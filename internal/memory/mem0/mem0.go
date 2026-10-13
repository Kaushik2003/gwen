// Package mem0 is the Mem0 lane: a copy of Gwen's verified claims in a
// self-hosted Mem0 server, which recalls them by meaning, and Mem0's own
// extractor run as a second opinion on chats (Shadow). The lane is optional
// and off by default; every call fails soft, with an error the caller logs or
// reports as a degraded lane.
//
// # The API, as verified on 2026-10-12
//
// Sources read:
//   - https://raw.githubusercontent.com/mem0ai/mem0/main/server/main.py (the REST server)
//   - https://raw.githubusercontent.com/mem0ai/mem0/main/server/auth.py (verify_auth)
//   - https://raw.githubusercontent.com/mem0ai/mem0/main/mem0/memory/main.py (Memory.add, search, get_all, delete)
//   - https://docs.mem0.ai/open-source/features/rest-api (endpoint list, auth, ports)
//
// Server: FastAPI, no /v1 prefix. Compose serves it on http://localhost:8888;
// raw Docker and uvicorn on 8000.
//
// Auth (server/auth.py verify_auth): on by default. An API key (per-user
// "m0sk_..." or the server's ADMIN_API_KEY) goes in the X-API-Key header.
// "Authorization: Bearer" is only for the dashboard's JWTs and is checked
// first, so a key must never be sent as a bearer token. With AUTH_DISABLED
// set, no header is needed. A refusal is 401 {"detail": "..."}.
//
// POST /memories, body MemoryCreate:
//
//	{"messages": [{"role": "user"|"assistant", "content": "..."}], "user_id": "...",
//	 "agent_id"?: "...", "run_id"?: "...", "metadata"?: {...}, "infer"?: bool,
//	 "expiration_date"?: "YYYY-MM-DD", "memory_type"?: "...", "prompt"?: "..."}
//
// At least one of user_id, agent_id, run_id is required (else 400). infer
// defaults to true. With infer false each non-system message is stored as is,
// one memory per message, with the metadata plus "role". The reply is
// {"results": [{"id": "<uuid>", "memory": "...", "event": "ADD", ...}]}.
// Since v3 the extractor is ADD-only (no UPDATE or DELETE events) and dedupes
// only by MD5 of the exact text against the 10 nearest memories in scope.
//
// POST /search, body {"query": "...", "filters": {"user_id": "..."}, "top_k"?: n,
// "threshold"?: f, "explain"?: bool, "show_expired"?: bool}. Top-level user_id
// is deprecated; filters must name a user_id, agent_id, or run_id. top_k
// defaults to 20, threshold to 0.1. The reply is {"results": [{"id", "memory",
// "score", "hash", "created_at", "updated_at", "user_id", "role", "metadata":
// {caller's keys}}]}: the caller's metadata comes back nested under "metadata".
//
// GET /memories?user_id=...&top_k=n (0 to 1000; get_all's default is 20)
// lists a scope: {"results": [items as in search, without "score"]}.
//
// DELETE /memories/{id}: {"message": "Memory deleted successfully"}; an
// unknown id is 404 {"detail": "Memory with id ... not found"}.
//
// There is no health route: the docs say to hit "/" or "/docs". Health here
// lists one memory of the scope, which also checks the key and the store.
//
// Not verified: Mem0's managed platform (api.mem0.ai, "Token" auth, /v1 and
// /v2 routes) is a different API and is not supported here.
//
// Known behaviour this lane works around or leaves to the operator:
//   - ADD-only: adding a claim again stores a second copy, so Upsert lists
//     what the scope holds and keeps or replaces the copy of each claim.
//   - Telemetry: the server sends PostHog telemetry unless MEM0_TELEMETRY=false
//     is set in its environment (setting it in code was reported not to work,
//     mem0 #2062). The daemon cannot set it for a server it does not start.
//   - Deleting a memory leaves its words in the server's history database
//     (Memory._delete_memory records the old text with event DELETE), and an
//     inferring add saves the messages there too. Only POST /reset (admin)
//     clears it, so forgetting cannot fully reach Mem0's history.
package mem0

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
)

// DefaultUserID is the Mem0 user whose memories are Gwen's claims.
const DefaultUserID = "gwen"

// ShadowUserID is the scratch Mem0 user Shadow extracts under; what it
// stores there is deleted again at once.
const ShadowUserID = "gwen-shadow"

// Metadata keys stored with each claim.
const (
	MetaClaimID   = "gwen_claim_id"
	MetaKey       = "key"
	MetaValidFrom = "valid_from"
)

// defaultCandidates is how many hits Search returns when the query does not
// say; maxCandidates is the most it asks for.
const (
	defaultCandidates = 40
	maxCandidates     = 100
)

// listLimit is the most memories one listing returns (the server's cap).
const listLimit = 1000

// maxReply bounds what is read of a reply.
const maxReply = 16 << 20

// Options configure a Lane.
type Options struct {
	URL    string // the server, such as http://127.0.0.1:8888
	APIKey string // sent as X-API-Key; "" sends none (a server with AUTH_DISABLED)
	UserID string // DefaultUserID when empty
	// HTTPClient makes the requests; nil is a client with a two-minute
	// timeout. Callers bound searches with their context.
	HTTPClient *http.Client
	// OnStored is told the Mem0 id a claim is stored under, after each
	// Upsert of it, so the service can record it in memory_backends.
	OnStored func(claimID, externalID string)
}

// Lane is the Mem0 lane. It is safe for concurrent use.
type Lane struct {
	url      string
	key      string
	user     string
	http     *http.Client
	onStored func(claimID, externalID string)
}

var _ memory.Projection = (*Lane)(nil)

// New returns a lane talking to the server at o.URL. It does not connect.
func New(o Options) *Lane {
	l := &Lane{url: strings.TrimRight(o.URL, "/"), key: o.APIKey, user: o.UserID, http: o.HTTPClient,
		onStored: o.OnStored}
	if l.user == "" {
		l.user = DefaultUserID
	}
	if l.http == nil {
		l.http = &http.Client{Timeout: 2 * time.Minute}
	}
	return l
}

// Name is memory.LaneMem0.
func (l *Lane) Name() memory.LaneName { return memory.LaneMem0 }

// Upsert stores each claim's text with inference off, under its claim id, so
// Mem0 holds it word for word. A claim already stored with the same words is
// kept; one stored with other words is replaced. Records that are sensitive
// (Meta["sensitivity"]) or have no words are skipped: sensitive claims never
// leave the device. OnStored is told each claim's Mem0 id. The scope is read
// once per call, at most 1,000 memories (the server's cap); past that a claim
// may be stored twice.
func (l *Lane) Upsert(ctx context.Context, recs []memory.Record) error {
	var todo []memory.Record
	for _, r := range recs {
		if r.Meta["sensitivity"] == model.SensitivitySensitive || strings.TrimSpace(r.Text) == "" ||
			(r.Kind != "" && r.Kind != "claim") {
			continue
		}
		todo = append(todo, r)
	}
	if len(todo) == 0 {
		return nil
	}
	items, err := l.list(ctx, l.user)
	if err != nil {
		return err
	}
	stored := map[string][]item{} // claim id -> its copies
	for _, it := range items {
		if id := it.meta(MetaClaimID); id != "" {
			stored[id] = append(stored[id], it)
		}
	}
	var errs []error
	for _, r := range todo {
		claimID := bareID(r.ID)
		text := strings.TrimSpace(r.Text)
		keep := ""
		var drop []string
		for _, it := range stored[claimID] {
			if keep == "" && strings.TrimSpace(it.Memory) == text {
				keep = it.ID
			} else {
				drop = append(drop, it.ID)
			}
		}
		if err := l.Delete(ctx, drop); err != nil {
			errs = append(errs, err)
			continue
		}
		if keep == "" {
			if keep, err = l.add(ctx, claimID, text, r); err != nil {
				errs = append(errs, err)
				continue
			}
		}
		stored[claimID] = []item{{ID: keep, Memory: text}}
		if l.onStored != nil {
			l.onStored(claimID, keep)
		}
	}
	return errors.Join(errs...)
}

// add stores one claim and returns its Mem0 id.
func (l *Lane) add(ctx context.Context, claimID, text string, r memory.Record) (string, error) {
	meta := map[string]string{MetaClaimID: claimID}
	if r.Key != "" {
		meta[MetaKey] = r.Key
	}
	if r.ValidFrom != nil {
		meta[MetaValidFrom] = r.ValidFrom.UTC().Format(time.RFC3339)
	}
	body := map[string]any{
		"messages": []Message{{Role: "user", Content: text}},
		"user_id":  l.user,
		"infer":    false,
		"metadata": meta,
	}
	var raw json.RawMessage
	if err := l.call(ctx, http.MethodPost, "/memories", nil, body, &raw); err != nil {
		return "", err
	}
	items, err := decodeItems(raw)
	if err != nil {
		return "", fmt.Errorf("mem0: %w", err)
	}
	for _, it := range items {
		if it.ID != "" {
			return it.ID, nil
		}
	}
	return "", errors.New("mem0: the server stored nothing")
}

// Delete drops memories by their Mem0 ids. One already gone is not an error.
func (l *Lane) Delete(ctx context.Context, ids []string) error {
	var errs []error
	for _, id := range ids {
		if id == "" {
			continue
		}
		err := l.call(ctx, http.MethodDelete, "/memories/"+url.PathEscape(id), nil, nil, nil)
		if err != nil && !isStatus(err, http.StatusNotFound) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Search asks Mem0 for the claims nearest q.Text in meaning. Hits are
// claim:<id> refs, best first, with Mem0's score; memories Gwen did not store
// (without a claim id) are left out.
func (l *Lane) Search(ctx context.Context, q memory.Query) ([]memory.Hit, error) {
	text := strings.TrimSpace(q.Text)
	if text == "" {
		return nil, nil
	}
	n := q.Budget.Candidates
	if n <= 0 {
		n = defaultCandidates
	}
	n = min(n, maxCandidates)
	body := map[string]any{"query": text, "filters": map[string]string{"user_id": l.user}, "top_k": n}
	var raw json.RawMessage
	if err := l.call(ctx, http.MethodPost, "/search", nil, body, &raw); err != nil {
		return nil, err
	}
	items, err := decodeItems(raw)
	if err != nil {
		return nil, fmt.Errorf("mem0: %w", err)
	}
	var hits []memory.Hit
	seen := map[string]bool{}
	for _, it := range items {
		id := it.meta(MetaClaimID)
		if id == "" {
			continue
		}
		ref := memory.Ref(memory.RefClaim, id)
		if seen[ref] || slices.Contains(q.Exclude, ref) {
			continue
		}
		seen[ref] = true
		h := memory.Hit{Ref: ref, Kind: "claim", Key: it.meta(MetaKey), Text: it.Memory, Score: it.Score,
			At: parseTime(it.CreatedAt), Lanes: []memory.LaneName{memory.LaneMem0}}
		if vf := parseTime(it.meta(MetaValidFrom)); !vf.IsZero() {
			h.ValidFrom = &vf
		}
		hits = append(hits, h)
		if len(hits) == n {
			break
		}
	}
	return hits, nil
}

// Health is nil when the server answers for Gwen's user with the key given.
func (l *Lane) Health(ctx context.Context) error {
	q := url.Values{"user_id": {l.user}, "top_k": {"1"}}
	return l.call(ctx, http.MethodGet, "/memories", q, nil, nil)
}

// Message is a chat message for Shadow.
type Message struct {
	Role    string `json:"role"` // "user" or "assistant"
	Content string `json:"content"`
}

// Shadow runs Mem0's own extractor over messages as a second opinion: it adds
// them with inference on under ShadowUserID, in a run of their own so nothing
// earlier colours the result, returns the facts Mem0 found, and deletes them
// again. Anything an earlier Shadow failed to delete is deleted first. The
// facts are only candidates: the caller verifies each against the messages.
// The caller must not pass sensitive messages.
func (l *Lane) Shadow(ctx context.Context, messages []Message) ([]string, error) {
	var msgs []Message
	for _, m := range messages {
		if strings.TrimSpace(m.Content) != "" && (m.Role == "user" || m.Role == "assistant") {
			msgs = append(msgs, m)
		}
	}
	if len(msgs) == 0 {
		return nil, nil
	}
	if left, err := l.list(ctx, ShadowUserID); err == nil {
		if err := l.Delete(ctx, ids(left)); err != nil {
			slog.Warn("mem0: shadow memories left behind", "err", err)
		}
	}
	body := map[string]any{"messages": msgs, "user_id": ShadowUserID, "run_id": "shadow-" + uuid.NewString(),
		"infer": true}
	var raw json.RawMessage
	if err := l.call(ctx, http.MethodPost, "/memories", nil, body, &raw); err != nil {
		return nil, err
	}
	items, err := decodeItems(raw)
	if err != nil {
		return nil, fmt.Errorf("mem0: %w", err)
	}
	var facts, made []string
	seen := map[string]bool{}
	for _, it := range items {
		switch strings.ToUpper(it.Event) {
		case "", "ADD", "UPDATE":
		default:
			continue
		}
		made = append(made, it.ID)
		if f := strings.TrimSpace(it.Memory); f != "" && !seen[f] {
			seen[f] = true
			facts = append(facts, f)
		}
	}
	// A cleanup that fails is retried by the next Shadow; the facts stand.
	if err := l.Delete(ctx, made); err != nil {
		slog.Warn("mem0: shadow memories left behind", "err", err)
	}
	return facts, nil
}

// list returns every memory of a Mem0 user, up to the server's cap.
func (l *Lane) list(ctx context.Context, user string) ([]item, error) {
	q := url.Values{"user_id": {user}, "top_k": {strconv.Itoa(listLimit)}}
	var raw json.RawMessage
	if err := l.call(ctx, http.MethodGet, "/memories", q, nil, &raw); err != nil {
		return nil, err
	}
	items, err := decodeItems(raw)
	if err != nil {
		return nil, fmt.Errorf("mem0: %w", err)
	}
	return items, nil
}

// StatusError is a reply the server refused with.
type StatusError struct {
	Status  int
	Message string
}

func (e *StatusError) Error() string {
	switch e.Status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "mem0 did not accept the API key: " + e.Message
	}
	return fmt.Sprintf("mem0: %d %s", e.Status, e.Message)
}

// isStatus reports whether err is a refusal with status.
func isStatus(err error, status int) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Status == status
}

// call sends one request and decodes the reply's JSON into out, when out is
// not nil.
func (l *Lane) call(ctx context.Context, method, path string, query url.Values, body, out any) error {
	if l.url == "" {
		return errors.New("mem0: no server address is set")
	}
	u := l.url + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return fmt.Errorf("mem0: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if l.key != "" {
		req.Header.Set("X-API-Key", l.key)
	}
	resp, err := l.http.Do(req)
	if err != nil {
		return fmt.Errorf("reach mem0: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxReply))
	if err != nil {
		return fmt.Errorf("mem0: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &StatusError{Status: resp.StatusCode, Message: detail(b, resp.Status)}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("mem0: unreadable reply: %w", err)
	}
	return nil
}

// detail is why the server refused, from FastAPI's {"detail": ...}.
func detail(body []byte, status string) string {
	var e struct {
		Detail  any    `json:"detail"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil {
		if s, ok := e.Detail.(string); ok && s != "" {
			return s
		}
		if e.Detail != nil {
			if b, err := json.Marshal(e.Detail); err == nil {
				return memory.Cut(string(b), 200)
			}
		}
		if s := cmp.Or(e.Message, e.Error); s != "" {
			return s
		}
	}
	if s := strings.TrimSpace(string(body)); s != "" {
		return memory.Cut(s, 200)
	}
	return status
}

// item is a memory as the server returns it, read tolerantly.
type item struct {
	ID        string
	Memory    string
	Event     string
	Score     float64
	CreatedAt string
	Metadata  map[string]any
	top       map[string]any // the whole object, for metadata at the top level
}

// meta is a metadata value as text: from "metadata", or from the item itself
// for servers that flatten it.
func (it item) meta(key string) string {
	for _, m := range []map[string]any{it.Metadata, it.top} {
		if v, ok := m[key]; ok {
			if s := text(v); s != "" {
				return s
			}
		}
	}
	return ""
}

// decodeItems reads a list of memories: {"results": [...]}, a bare array,
// or {"memories": [...]}. Unknown fields are ignored and odd types tolerated.
func decodeItems(raw json.RawMessage) ([]item, error) {
	var list []map[string]any
	if err := json.Unmarshal(raw, &list); err != nil {
		var wrap struct {
			Results  []map[string]any `json:"results"`
			Memories []map[string]any `json:"memories"`
		}
		if err := json.Unmarshal(raw, &wrap); err != nil {
			return nil, fmt.Errorf("unreadable list of memories: %w", err)
		}
		list = wrap.Results
		if list == nil {
			list = wrap.Memories
		}
	}
	items := make([]item, 0, len(list))
	for _, m := range list {
		it := item{ID: text(m["id"]), Memory: text(firstOf(m["memory"], m["data"], m["text"])),
			Event: text(m["event"]), CreatedAt: text(m["created_at"]), top: m}
		it.Metadata, _ = m["metadata"].(map[string]any)
		switch s := m["score"].(type) {
		case float64:
			it.Score = s
		case string:
			it.Score, _ = strconv.ParseFloat(s, 64)
		}
		items = append(items, it)
	}
	return items, nil
}

// text is a JSON value as text: strings as they are, numbers and booleans
// formatted, anything else "".
func text(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	}
	return ""
}

// firstOf is the first of vs that is not nil.
func firstOf(vs ...any) any {
	for _, v := range vs {
		if v != nil {
			return v
		}
	}
	return nil
}

// parseTime reads an RFC 3339 instant or a YYYY-MM-DD day; zero for anything
// else.
func parseTime(s string) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	if t, err := time.Parse("2006-01-02T15:04:05.999999", s); err == nil {
		return t // Python's isoformat without a zone
	}
	if t, err := time.Parse(time.DateOnly, s); err == nil {
		return t
	}
	return time.Time{}
}

// bareID is a claim id without a "claim:" prefix.
func bareID(id string) string {
	id, _ = strings.CutPrefix(id, memory.RefClaim+":")
	return id
}

// ids are the Mem0 ids of items.
func ids(items []item) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}
