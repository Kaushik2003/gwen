// Package supermemory is the Supermemory lane: the documents the user gave
// Gwen, kept in a local Supermemory server that searches them by meaning and
// builds a profile from them. The lane is optional and off by default; every
// call fails soft, with an error the caller logs or reports as a degraded
// lane. Only resources go here, never claims or chats.
//
// # The API, as verified on 2026-10-12
//
// Sources read (the docs pages are Mintlify Markdown with the OpenAPI spec
// embedded):
//   - https://supermemory.ai/docs/self-hosting/overview.md and /self-hosting/quickstart.md
//   - https://supermemory.ai/docs/api-reference/overview.md (legacy v3/v4 reference)
//   - https://supermemory.ai/docs/api-reference/ingest/add-document.md (POST /v3/documents)
//   - https://supermemory.ai/docs/api-reference/ingest/delete-document-by-id-or-customid.md
//   - https://supermemory.ai/docs/api-reference/documents/search-documents.md (POST /v3/search)
//   - https://supermemory.ai/docs/api-reference/recall-search/search-memory-entries.md (POST /v4/search)
//   - https://supermemory.ai/docs/api-reference/documents/list-documents.md (POST /v3/documents/list)
//   - https://supermemory.ai/docs/api-reference/profiles/get-user-profile.md (POST /v4/profile)
//   - https://supermemory.ai/docs/migration/api-v5.md and the v5 pages under /docs/v5/api-reference/
//   - https://api.github.com/repos/supermemoryai/supermemory/releases (server versions)
//
// Server: the self-hosted binary serves http://localhost:6767. Server
// v0.0.8 (17 Aug 2026, the latest stable) speaks only the legacy v3/v4 API;
// v0.0.9 (a release candidate since 12 Oct 2026) adds v5. The docs mark v3
// and v4 deprecated, to be shut down on the hosted platform on 31 Dec 2026.
// So the lane speaks v3/v4 and, when a server no longer has those routes
// (404, 405, or 410 on the document list), v5.
//
// Auth: "Authorization: Bearer sm_..." on every request (the key is printed
// on the server's first boot). Errors are {"error": "...", "details"?: "..."}.
//
// Legacy (v3/v4):
//   - POST /v3/documents {"content", "containerTag"?, "customId"?, "metadata"?: {string: string|number|bool|[string]},
//     "documentDate"?: "YYYY-MM-DD or ISO 8601", "dreaming"?, "taskType"?} -> {"id", "status"}. customId and
//     containerTag: at most 100 characters, letters, digits, "-", "_", and "."; the container must also match
//     ^[a-zA-Z0-9_:-]+$ where it is searched. Adding with a customId again replaces that document.
//   - DELETE /v3/documents/{id or customId} -> 204; 404 when there is no such document.
//   - POST /v3/search {"q", "containerTag", "limit" (1-100), "onlyMatchingChunks"?, "chunkThreshold"?} ->
//     {"results": [{"documentId", "title", "metadata", "score", "createdAt", "chunks": [{"content", "position",
//     "score", "isRelevant"}]}], "timing", "total"}. Chunks name their document and position here.
//   - POST /v4/search {"q", "containerTag", "searchMode": memories|hybrid|documents, "limit"} returns chunk
//     results with only "id", "chunk", "metadata", and "similarity", which do not name the document; so
//     documents are searched with /v3/search.
//   - POST /v3/documents/list {"containerTags": [...], "limit", "page"} -> {"memories": [documents],
//     "pagination": {"currentPage", "limit", "totalItems", "totalPages"}}.
//   - POST /v4/profile {"containerTag"} -> {"profile": {"static": [string], "dynamic": [string], "buckets"?}}.
//   - POST /v4/memories/list {"containerTags"} lists extracted memories; not used.
//
// v5 (namespace in the path; options in the JSON body; list paging in the query):
//   - POST /ns/{ns}/document {"content", "id"?, "metadata"?, "date"?} -> {"id", "status"}. Adding with an
//     existing id adds to that document rather than replacing it.
//   - DELETE /ns/{ns}/document {"ids": [up to 100]} -> {"count", "errors": [{"id", "error"}]}.
//   - POST /ns/{ns}/search {"query", "searchMode": hybrid|memories|chunks, "limit", "include": {"documents"}}
//     -> {"results": [{"id", "memory"|"chunk", "metadata", "similarity", "included": {"document": {"id",
//     "title", "metadata", "system": {"createdAt"}}}}], "searchTime"}.
//   - POST /ns/{ns}/list/documents?page=1&limit=1 {} -> {"documents", "chunks", "memories", "pagination":
//     {"totalItems", ...}}.
//   - POST /ns/{ns}/profile {} -> {"profile": {"static": [{"id", "memory"}], "dynamic": [{"id", "memory"}]}}.
//
// Not verified: whether v0.0.9 still serves v3/v4 (the lane does not depend on
// it), how v5 answers for a namespace that has nothing yet (a 404 there is
// read as empty), and whether chunk results carry the document's metadata in
// v5 (the lane also reads included.document.metadata).
//
// Known server problems this lane works around or leaves to the operator:
//   - customId collisions overwrite documents (supermemory #1547): the id is
//     the resource's UUID, and an id that is not a valid customId is refused
//     rather than rewritten, so two resources never share one.
//   - Search returns nothing without a containerTag (#1704) and nothing when
//     limit is over 100 (#1740): both are always set, limit at most 100.
//   - Text with no ASCII characters is silently not indexed (#1740). Gwen's
//     own episode search covers such documents.
//   - The server binds every interface (#1441) and keeps its data in the
//     directory it was started from unless SUPERMEMORY_DATA_DIR is set (#1420),
//     and sends telemetry unless SUPERMEMORY_DISABLE_TELEMETRY=1. These are set
//     where the server is started; this lane defaults to the loopback address.
package supermemory

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
)

// DefaultURL is the local server's address.
const DefaultURL = "http://127.0.0.1:6767"

// DefaultContainer is the container (namespace) Gwen's documents live in.
const DefaultContainer = "gwen"

// MetaResourceID is the metadata key holding a document's resource id.
const MetaResourceID = "gwen_resource_id"

// defaultCandidates is how many hits Search returns when the query does not
// say; maxLimit is the most the server returns (and more returns nothing).
const (
	defaultCandidates = 40
	maxLimit          = 100
)

// maxReply bounds what is read of a reply.
const maxReply = 16 << 20

// customID is what the server accepts as a customId.
var customID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)

// The API dialects.
const (
	apiUnknown int32 = iota
	apiLegacy        // v3/v4
	apiV5
)

// Options configure a Lane.
type Options struct {
	URL       string // DefaultURL when empty
	APIKey    string // sent as a bearer token; "" sends none
	Container string // DefaultContainer when empty
	// HTTPClient makes the requests; nil is a client with a one-minute
	// timeout. Callers bound searches with their context.
	HTTPClient *http.Client
}

// Lane is the Supermemory lane. It is safe for concurrent use.
type Lane struct {
	url       string
	key       string
	container string
	http      *http.Client
	api       atomic.Int32 // the dialect the server speaks, once known
}

var _ memory.Projection = (*Lane)(nil)

// New returns a lane talking to the server at o.URL. It does not connect.
func New(o Options) *Lane {
	l := &Lane{url: strings.TrimRight(cmp.Or(o.URL, DefaultURL), "/"), key: o.APIKey,
		container: cmp.Or(o.Container, DefaultContainer), http: o.HTTPClient}
	if l.http == nil {
		l.http = &http.Client{Timeout: time.Minute}
	}
	return l
}

// Name is memory.LaneSupermemory.
func (l *Lane) Name() memory.LaneName { return memory.LaneSupermemory }

// Upsert sends each resource's text as a document of the container, under
// its resource id, with the id in the metadata (and the title, from
// Meta["title"]) and the document's date (ValidFrom, else At). Records that
// are sensitive (Meta["sensitivity"]) or have no text are skipped: sensitive
// content never leaves the device. An id the server would not take as is is
// refused, not rewritten, so two resources never share a document.
func (l *Lane) Upsert(ctx context.Context, recs []memory.Record) error {
	var todo []memory.Record
	for _, r := range recs {
		if r.Meta["sensitivity"] == model.SensitivitySensitive || strings.TrimSpace(r.Text) == "" ||
			(r.Kind != "" && r.Kind != "resource") {
			continue
		}
		todo = append(todo, r)
	}
	if len(todo) == 0 {
		return nil
	}
	api, err := l.dialect(ctx, false)
	if err != nil {
		return err
	}
	var errs []error
	for _, r := range todo {
		id := bareID(r.ID)
		if !customID.MatchString(id) {
			errs = append(errs, fmt.Errorf("supermemory: %q cannot be a document id", id))
			continue
		}
		meta := map[string]string{MetaResourceID: id}
		if t := r.Meta["title"]; t != "" {
			meta["title"] = t
		}
		date := ""
		if r.ValidFrom != nil {
			date = r.ValidFrom.UTC().Format(time.RFC3339)
		} else if !r.At.IsZero() {
			date = r.At.UTC().Format(time.RFC3339)
		}
		var body map[string]any
		var path string
		if api == apiV5 {
			path = l.ns("/document")
			body = map[string]any{"content": r.Text, "id": id, "metadata": meta}
			if date != "" {
				body["date"] = date
			}
		} else {
			path = "/v3/documents"
			body = map[string]any{"content": r.Text, "containerTag": l.container, "customId": id, "metadata": meta}
			if date != "" {
				body["documentDate"] = date
			}
		}
		if err := l.call(ctx, http.MethodPost, path, body, nil); err != nil {
			l.lost(api, err)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Delete drops documents by resource id. One already gone is not an error.
// It asks the server which API it speaks first, so a missing route is never
// mistaken for a missing document.
func (l *Lane) Delete(ctx context.Context, ids []string) error {
	var todo []string
	for _, id := range ids {
		if id = bareID(id); id != "" {
			todo = append(todo, id)
		}
	}
	if len(todo) == 0 {
		return nil
	}
	api, err := l.dialect(ctx, true)
	if err != nil {
		return err
	}
	var errs []error
	if api == apiLegacy {
		for _, id := range todo {
			err := l.call(ctx, http.MethodDelete, "/v3/documents/"+url.PathEscape(id), nil, nil)
			if err != nil && !isStatus(err, http.StatusNotFound) {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	}
	for batch := range slices.Chunk(todo, 100) {
		var res struct {
			Errors []struct {
				ID    string `json:"id"`
				Error string `json:"error"`
			} `json:"errors"`
		}
		err := l.call(ctx, http.MethodDelete, l.ns("/document"), map[string]any{"ids": batch}, &res)
		if isStatus(err, http.StatusNotFound) {
			continue
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, e := range res.Errors {
			if !strings.Contains(strings.ToLower(e.Error), "not found") {
				errs = append(errs, fmt.Errorf("supermemory: delete %s: %s", e.ID, e.Error))
			}
		}
	}
	return errors.Join(errs...)
}

// Search finds the passages of the documents nearest q.Text in meaning. Each
// hit is a passage: Ref doc:<resource id>#c<position> (or #<chunk id> when the
// server gives no position), Kind "resource", Text the passage, trust
// external. Documents Gwen did not add are left out.
func (l *Lane) Search(ctx context.Context, q memory.Query) ([]memory.Hit, error) {
	text := strings.TrimSpace(q.Text)
	if text == "" {
		return nil, nil
	}
	n := q.Budget.Candidates
	if n <= 0 {
		n = defaultCandidates
	}
	n = min(n, maxLimit)
	api, err := l.dialect(ctx, false)
	if err != nil {
		return nil, err
	}
	var passages []passage
	if api == apiV5 {
		passages, err = l.searchV5(ctx, text, n)
	} else {
		passages, err = l.searchLegacy(ctx, text, n)
	}
	if err != nil {
		l.lost(api, err)
		return nil, err
	}
	slices.SortStableFunc(passages, func(a, b passage) int { return cmp.Compare(b.score, a.score) })
	var hits []memory.Hit
	seen := map[string]bool{}
	for _, p := range passages {
		ref := memory.Ref(memory.RefResource, p.doc)
		if seen[ref+p.part] || slices.Contains(q.Exclude, ref) || slices.Contains(q.Exclude, ref+p.part) {
			continue
		}
		seen[ref+p.part] = true
		h := memory.Hit{Ref: ref + p.part, Kind: "resource", Text: p.text, Score: p.score, At: p.at,
			Trust: model.TrustExternal, Lanes: []memory.LaneName{memory.LaneSupermemory}}
		if p.title != "" {
			h.Source = "document “" + p.title + "”"
		}
		hits = append(hits, h)
		if len(hits) == n {
			break
		}
	}
	return hits, nil
}

// passage is a chunk of a document found by a search.
type passage struct {
	doc   string // the resource id
	part  string // "#c<position>" or "#<chunk id>", or ""
	text  string
	title string
	score float64
	at    time.Time
}

func (l *Lane) searchLegacy(ctx context.Context, text string, n int) ([]passage, error) {
	body := map[string]any{"q": text, "containerTag": l.container, "limit": n, "onlyMatchingChunks": true}
	var res struct {
		Results []map[string]any `json:"results"`
	}
	if err := l.call(ctx, http.MethodPost, "/v3/search", body, &res); err != nil {
		return nil, err
	}
	var out []passage
	for _, r := range res.Results {
		doc := resourceID(r["metadata"], r["customId"])
		if doc == "" {
			continue
		}
		title, at, score := str(r["title"]), parseTime(str(r["createdAt"])), num(r["score"])
		chunks, _ := r["chunks"].([]any)
		for _, c := range chunks {
			ch, _ := c.(map[string]any)
			content := strings.TrimSpace(str(ch["content"]))
			if content == "" || ch["isRelevant"] == false {
				continue
			}
			p := passage{doc: doc, text: content, title: title, at: at, score: score}
			if s, ok := ch["score"].(float64); ok {
				p.score = s
			}
			if pos, ok := ch["position"].(float64); ok {
				p.part = "#c" + strconv.Itoa(int(pos))
			}
			out = append(out, p)
		}
	}
	return out, nil
}

func (l *Lane) searchV5(ctx context.Context, text string, n int) ([]passage, error) {
	body := map[string]any{"query": text, "searchMode": "chunks", "limit": n,
		"include": map[string]bool{"documents": true}}
	var res struct {
		Results []map[string]any `json:"results"`
	}
	err := l.call(ctx, http.MethodPost, l.ns("/search"), body, &res)
	if isStatus(err, http.StatusNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []passage
	for _, r := range res.Results {
		var docMeta any
		var title, created string
		if inc, ok := r["included"].(map[string]any); ok {
			if d, ok := inc["document"].(map[string]any); ok {
				docMeta, title = d["metadata"], str(d["title"])
				if sys, ok := d["system"].(map[string]any); ok {
					created = str(sys["createdAt"])
				}
			}
		}
		doc := resourceID(r["metadata"], r["customId"])
		if doc == "" {
			doc = resourceID(docMeta, nil)
		}
		content := strings.TrimSpace(str(r["chunk"]))
		if doc == "" || content == "" {
			continue
		}
		p := passage{doc: doc, text: content, title: title, score: num(r["similarity"]), at: parseTime(created)}
		if pos, ok := r["position"].(float64); ok {
			p.part = "#c" + strconv.Itoa(int(pos))
		} else if id := str(r["id"]); id != "" {
			p.part = "#" + id
		}
		out = append(out, p)
	}
	return out, nil
}

// Count is how many documents the container holds; the free server takes
// at most 10,000.
func (l *Lane) Count(ctx context.Context) (int, error) {
	api, err := l.dialect(ctx, false)
	if err != nil {
		return 0, err
	}
	var res struct {
		Pagination struct {
			TotalItems *float64 `json:"totalItems"`
		} `json:"pagination"`
		Memories  []any `json:"memories"`
		Documents []any `json:"documents"`
	}
	if api == apiV5 {
		err = l.call(ctx, http.MethodPost, l.ns("/list/documents")+"?page=1&limit=1", map[string]any{}, &res)
		if isStatus(err, http.StatusNotFound) {
			return 0, nil
		}
	} else {
		err = l.call(ctx, http.MethodPost, "/v3/documents/list",
			map[string]any{"containerTags": []string{l.container}, "limit": 1, "page": 1}, &res)
	}
	if err != nil {
		l.lost(api, err)
		return 0, err
	}
	if res.Pagination.TotalItems != nil {
		return int(*res.Pagination.TotalItems), nil
	}
	return len(res.Memories) + len(res.Documents), nil
}

// Profile returns what Supermemory has made of the documents: its static
// facts, then its dynamic ones, each once. They are external content: the
// caller treats each as a quarantined candidate.
func (l *Lane) Profile(ctx context.Context) ([]string, error) {
	api, err := l.dialect(ctx, false)
	if err != nil {
		return nil, err
	}
	var res struct {
		Profile struct {
			Static  []any `json:"static"`
			Dynamic []any `json:"dynamic"`
		} `json:"profile"`
	}
	if api == apiV5 {
		err = l.call(ctx, http.MethodPost, l.ns("/profile"), map[string]any{}, &res)
	} else {
		err = l.call(ctx, http.MethodPost, "/v4/profile", map[string]any{"containerTag": l.container}, &res)
	}
	if isStatus(err, http.StatusNotFound) {
		l.lost(api, err)
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var facts []string
	seen := map[string]bool{}
	for _, f := range append(res.Profile.Static, res.Profile.Dynamic...) {
		s := str(f)
		if m, ok := f.(map[string]any); ok {
			s = cmp.Or(str(m["memory"]), str(m["content"]), str(m["text"]))
		}
		if s = strings.TrimSpace(s); s != "" && !seen[s] {
			seen[s] = true
			facts = append(facts, s)
		}
	}
	return facts, nil
}

// Health is nil when the server answers with the key given.
func (l *Lane) Health(ctx context.Context) error {
	_, err := l.Count(ctx)
	return err
}

// dialect is the API the server speaks: the one found before, or, when fresh
// or not known yet, what the document list says now. A server without the
// legacy list is taken to speak v5.
func (l *Lane) dialect(ctx context.Context, fresh bool) (int32, error) {
	if api := l.api.Load(); api != apiUnknown && !fresh {
		return api, nil
	}
	err := l.call(ctx, http.MethodPost, "/v3/documents/list",
		map[string]any{"containerTags": []string{l.container}, "limit": 1, "page": 1}, nil)
	switch {
	case err == nil:
		l.api.Store(apiLegacy)
		return apiLegacy, nil
	case routeMissing(err):
		l.api.Store(apiV5)
		return apiV5, nil
	}
	return apiUnknown, err
}

// lost forgets the dialect when a legacy route has gone, as after an upgrade
// to a server that only speaks v5, so the next call asks again.
func (l *Lane) lost(api int32, err error) {
	if api == apiLegacy && routeMissing(err) {
		l.api.CompareAndSwap(apiLegacy, apiUnknown)
	}
}

// ns is a v5 path in the container's namespace.
func (l *Lane) ns(path string) string { return "/ns/" + url.PathEscape(l.container) + path }

// StatusError is a reply the server refused with.
type StatusError struct {
	Status  int
	Message string
}

func (e *StatusError) Error() string {
	switch e.Status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "supermemory did not accept the API key: " + e.Message
	case http.StatusPaymentRequired:
		return "supermemory is out of credits or over its document limit: " + e.Message
	}
	return fmt.Sprintf("supermemory: %d %s", e.Status, e.Message)
}

// isStatus reports whether err is a refusal with status.
func isStatus(err error, status int) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Status == status
}

// routeMissing reports whether err says the server has no such route.
func routeMissing(err error) bool {
	var se *StatusError
	return errors.As(err, &se) &&
		(se.Status == http.StatusNotFound || se.Status == http.StatusMethodNotAllowed || se.Status == http.StatusGone)
}

// call sends one request and decodes the reply's JSON into out, when out is
// not nil and the reply has a body.
func (l *Lane) call(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, l.url+path, rd)
	if err != nil {
		return fmt.Errorf("supermemory: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if l.key != "" {
		req.Header.Set("Authorization", "Bearer "+l.key)
	}
	resp, err := l.http.Do(req)
	if err != nil {
		return fmt.Errorf("reach supermemory: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxReply))
	if err != nil {
		return fmt.Errorf("supermemory: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &StatusError{Status: resp.StatusCode, Message: errorText(b, resp.Status)}
	}
	if out == nil || len(bytes.TrimSpace(b)) == 0 {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("supermemory: unreadable reply: %w", err)
	}
	return nil
}

// errorText is why the server refused, from {"error", "details"}.
func errorText(body []byte, status string) string {
	var e struct {
		Error   any    `json:"error"`
		Message string `json:"message"`
		Details string `json:"details"`
	}
	if json.Unmarshal(body, &e) == nil {
		msg := cmp.Or(str(e.Error), e.Message)
		if m, ok := e.Error.(map[string]any); ok {
			msg = cmp.Or(msg, str(m["message"]))
		}
		if msg != "" && e.Details != "" {
			msg += ": " + e.Details
		}
		if msg != "" {
			return memory.Cut(msg, 200)
		}
	}
	if s := strings.TrimSpace(string(body)); s != "" {
		return memory.Cut(s, 200)
	}
	return status
}

// resourceID is the resource id a search result names: gwen_resource_id in
// its metadata, else its customId; "" when neither.
func resourceID(meta, custom any) string {
	if m, ok := meta.(map[string]any); ok {
		if id := str(m[MetaResourceID]); id != "" {
			return id
		}
	}
	return str(custom)
}

// bareID is a resource id without a "doc:" prefix or a part.
func bareID(id string) string {
	id, _ = strings.CutPrefix(id, memory.RefResource+":")
	id, _, _ = strings.Cut(id, "#")
	return id
}

// str is a JSON string, or "" for anything else.
func str(v any) string {
	s, _ := v.(string)
	return s
}

// num is a JSON number, or 0 for anything else.
func num(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case string:
		f, _ := strconv.ParseFloat(t, 64)
		return f
	}
	return 0
}

// parseTime reads an RFC 3339 instant; zero for anything else.
func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}
