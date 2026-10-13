package api

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/planner/civil"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
)

// The memory routes (/v1/memory/...), which answer 503 while the daemon runs
// no memory, and every conversion of memory to wire.

// Memory route sizes.
const (
	DefaultClaims     = 200
	MaxClaims         = 500
	DefaultSelfHist   = 50
	MaxSelfHist       = 200
	maxRememberRunes  = 4000
	maxRecallRunes    = 500
	maxClaimTextRunes = 1000
	maxTitleRunes     = 200
	// ForgetAllConfirm is what POST /v1/memory/forget-all needs in confirm.
	ForgetAllConfirm = "forget everything"
	// memoryOff is what a request that needs memory hears while it is off.
	memoryOff = "memory is off; turn it on in Settings, Memory"
)

// MemoryResolver resolves memory refs for showing: what a reply cited and was
// asked to forget, and what was learned from each user message. The daemon's
// memory implements it beside memory.Memory (whose Resolve settles conflicts,
// hence ResolveRef); without one, refs are resolved from the store with a
// plain source.
type MemoryResolver interface {
	// ResolveRef resolves one ref; what it named being gone is Gone, not an
	// error.
	ResolveRef(ctx context.Context, ref string) (wire.MemoryRef, error)
	// Learned returns, by message id, the live claims learned from each
	// message, in any status but rejected.
	Learned(ctx context.Context, msgIDs []string) (map[string][]wire.MemoryRef, error)
}

// resolver is the memory when it resolves refs, else the store.
func (s *Server) resolver() MemoryResolver {
	if r, ok := s.Memory.(MemoryResolver); ok {
		return r
	}
	return storeResolver{repos: s.Repos}
}

// withMemory runs a memory handler, or answers that memory is off.
func (s *Server) withMemory(h func(w http.ResponseWriter, r *http.Request, m memory.Memory) error) func(
	http.ResponseWriter, *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		if s.Memory == nil {
			return Unavailable(memoryOff)
		}
		if err := h(w, r, s.Memory); err != nil {
			if errors.Is(err, memory.ErrDisabled) {
				return Unavailable(memoryOff)
			}
			return err
		}
		return nil
	}
}

func noClaim(id string) error {
	return &requestError{code: wire.CodeNotFound, status: http.StatusNotFound, msg: fmt.Sprintf("no claim %s", id)}
}

// memoryStatus is GET /v1/memory/status.
func (s *Server) memoryStatus(w http.ResponseWriter, r *http.Request, m memory.Memory) error {
	writeJSON(w, http.StatusOK, memoryStatusWire(m.Status(r.Context())))
	return nil
}

// listClaims is GET /v1/memory/claims (wire.MemoryClaimQuery as query
// parameters).
func (s *Server) listClaims(w http.ResponseWriter, r *http.Request, m memory.Memory) error {
	ctx := r.Context()
	q, f, err := claimQuery(r)
	if err != nil {
		return err
	}
	if q.Q == "" {
		views, err := m.Claims(ctx, f)
		if err != nil {
			return err
		}
		writeJSON(w, http.StatusOK, wire.MemoryClaimList{Claims: claimsWire(views)})
		return nil
	}
	// Words: the claims that share them, the best match first, within the
	// filter.
	hits, err := s.Repos.Memory.SearchClaims(ctx, q.Q, MaxClaims)
	if err != nil {
		return err
	}
	f.Limit, f.Offset = 0, 0
	views, err := m.Claims(ctx, f)
	if err != nil {
		return err
	}
	byID := make(map[string]memory.ClaimView, len(views))
	for _, v := range views {
		byID[v.ID] = v
	}
	var found []memory.ClaimView
	for _, h := range hits {
		if v, ok := byID[h.ID]; ok {
			found = append(found, v)
		}
	}
	found = found[min(len(found), q.Offset):]
	found = found[:min(len(found), q.Limit)]
	writeJSON(w, http.StatusOK, wire.MemoryClaimList{Claims: claimsWire(found)})
	return nil
}

// claimQuery reads wire.MemoryClaimQuery from the query string and the store
// filter it selects.
func claimQuery(r *http.Request) (wire.MemoryClaimQuery, store.ClaimFilter, error) {
	v := r.URL.Query()
	q := wire.MemoryClaimQuery{Status: v.Get("status"), Key: strings.TrimSpace(v.Get("key")),
		Subject: strings.TrimSpace(v.Get("subject")), Q: strings.TrimSpace(v.Get("q"))}
	var f store.ClaimFilter
	switch q.Status {
	case "":
		q.Status, f.Statuses = wire.ClaimActive, []string{model.ClaimActive}
	case "all":
	case model.ClaimCandidate, model.ClaimActive, model.ClaimQuarantined, model.ClaimRejected, model.ClaimSuperseded:
		f.Statuses = []string{q.Status}
	default:
		return q, f, badRequest("status", "status is all, candidate, active, quarantined, rejected, or superseded")
	}
	f.KeyPrefix = q.Key
	switch q.Subject {
	case "":
	case "user":
		f.SubjectSet = true
	default:
		subject := q.Subject
		f.Subject, f.SubjectSet = &subject, true
	}
	if utf8.RuneCountInString(q.Q) > maxRecallRunes {
		return q, f, badRequest("q", "a search is at most %d characters", maxRecallRunes)
	}
	if h := v.Get("hidden"); h != "" {
		b, err := strconv.ParseBool(h)
		if err != nil {
			return q, f, badRequest("hidden", "hidden is true or false")
		}
		q.Hidden = b
	}
	f.IncludeSuppressed = q.Hidden
	var err error
	if q.Limit, err = queryLimit(r, DefaultClaims, MaxClaims); err != nil {
		return q, f, err
	}
	if o := v.Get("offset"); o != "" {
		if q.Offset, err = strconv.Atoi(o); err != nil || q.Offset < 0 {
			return q, f, badRequest("offset", "offset is a number of claims, 0 or more")
		}
	}
	f.Limit, f.Offset = q.Limit, q.Offset
	return q, f, nil
}

// claimView is the claim id with what the dashboard shows beside it.
func claimView(ctx context.Context, m memory.Memory, id string) (memory.ClaimView, error) {
	ref := memory.Ref(memory.RefClaim, id)
	if _, _, ok := memory.ParseRef(ref); !ok {
		return memory.ClaimView{}, noClaim(id)
	}
	exp, err := m.Explain(ctx, ref)
	if errors.Is(err, store.ErrNotFound) || err == nil && exp.Claim == nil {
		return memory.ClaimView{}, noClaim(id)
	}
	if err != nil {
		return memory.ClaimView{}, err
	}
	return *exp.Claim, nil
}

// claimOut is a claim a change returned, as returned, with what the
// dashboard shows beside it when the memory can say.
func claimOut(ctx context.Context, m memory.Memory, c model.Claim) wire.MemoryClaim {
	v, err := claimView(ctx, m, c.ID)
	if err != nil {
		v = memory.ClaimView{}
	}
	v.Claim = c
	return claimWire(v)
}

// getClaim is GET /v1/memory/claims/{id}.
func (s *Server) getClaim(w http.ResponseWriter, r *http.Request, m memory.Memory) error {
	v, err := claimView(r.Context(), m, r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, claimWire(v))
	return nil
}

// editClaim is PATCH /v1/memory/claims/{id}: the user's correction, which
// supersedes the claim.
func (s *Server) editClaim(w http.ResponseWriter, r *http.Request, m memory.Memory) error {
	var req wire.EditClaimRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" && req.Value == nil && req.ValidFrom == nil && req.ValidTo == nil {
		return badRequest("text", "say what is true now: the text, the value, or when it was true")
	}
	if utf8.RuneCountInString(req.Text) > maxClaimTextRunes {
		return badRequest("text", "a claim is at most %d characters", maxClaimTextRunes)
	}
	if req.Value != nil {
		v := strings.TrimSpace(*req.Value)
		if v == "" || utf8.RuneCountInString(v) > maxClaimTextRunes {
			return badRequest("value", "a value is 1 to %d characters", maxClaimTextRunes)
		}
		req.Value = &v
	}
	for _, d := range []struct {
		field string
		day   *string
	}{{"valid_from", req.ValidFrom}, {"valid_to", req.ValidTo}} {
		if d.day != nil && *d.day != "" {
			if _, err := civil.Parse(*d.day); err != nil {
				return badRequest(d.field, "%s is a day like 2026-10-05, or empty for open", d.field)
			}
		}
	}
	c, err := m.Edit(r.Context(), r.PathValue("id"), memory.EditRequest{Text: req.Text, Value: req.Value,
		ValidFrom: req.ValidFrom, ValidTo: req.ValidTo})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, wire.MemoryClaimsResult{Claims: []wire.MemoryClaim{claimOut(r.Context(), m, c)}})
	return nil
}

// claimChange is a POST /v1/memory/claims/{id}/... that changes one claim.
func (s *Server) claimChange(change func(ctx context.Context, m memory.Memory, id string, r *http.Request) (model.Claim,
	error)) func(http.ResponseWriter, *http.Request, memory.Memory) error {
	return func(w http.ResponseWriter, r *http.Request, m memory.Memory) error {
		c, err := change(r.Context(), m, r.PathValue("id"), r)
		if err != nil {
			return err
		}
		writeJSON(w, http.StatusOK, claimOut(r.Context(), m, c))
		return nil
	}
}

func confirmClaim(ctx context.Context, m memory.Memory, id string, _ *http.Request) (model.Claim, error) {
	return m.Confirm(ctx, id)
}

func rejectClaim(ctx context.Context, m memory.Memory, id string, _ *http.Request) (model.Claim, error) {
	return m.Reject(ctx, id)
}

func hideClaim(ctx context.Context, m memory.Memory, id string, r *http.Request) (model.Claim, error) {
	var req wire.HideClaimRequest
	if err := decode(r, &req); err != nil {
		return model.Claim{}, err
	}
	return m.Suppress(ctx, id, req.Hidden)
}

// forgetClaim is DELETE /v1/memory/claims/{id}: the claim is erased
// everywhere, and the receipt says where.
func (s *Server) forgetClaim(w http.ResponseWriter, r *http.Request, m memory.Memory) error {
	rc, err := m.Forget(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, receiptWire(rc))
	return nil
}

// explainMemory is GET /v1/memory/explain?ref=.
func (s *Server) explainMemory(w http.ResponseWriter, r *http.Request, m memory.Memory) error {
	ref := strings.TrimSpace(r.URL.Query().Get("ref"))
	if _, _, ok := memory.ParseRef(ref); !ok {
		return badRequest("ref", "ref is claim:<id>, msg:<id>, ep:<id>, doc:<id>, or data:<name>")
	}
	exp, err := m.Explain(r.Context(), ref)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, explanationWire(exp))
	return nil
}

// remember is POST /v1/memory/remember: words the user typed to be
// remembered. ?channel=cli says they came from the terminal.
func (s *Server) remember(w http.ResponseWriter, r *http.Request, m memory.Memory) error {
	var req wire.RememberRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	text := strings.TrimSpace(req.Text)
	if n := utf8.RuneCountInString(text); n < 1 || n > maxRememberRunes {
		return badRequest("text", "what to remember is 1 to %d characters", maxRememberRunes)
	}
	channel := model.ChannelUI
	switch c := r.URL.Query().Get("channel"); c {
	case "", model.ChannelUI:
	case model.ChannelCLI:
		channel = c
	default:
		return badRequest("channel", "channel is ui or cli")
	}
	claims, err := m.Remember(r.Context(), memory.RememberRequest{Text: text, Channel: channel})
	if err != nil {
		return err
	}
	out := wire.MemoryClaimsResult{Claims: []wire.MemoryClaim{}}
	for _, c := range claims {
		out.Claims = append(out.Claims, claimOut(r.Context(), m, c))
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// knownLanes are the lanes a recall may name.
var knownLanes = []memory.LaneName{memory.LaneClaims, memory.LaneChats, memory.LaneEpisodes, memory.LaneEntity,
	memory.LaneTime, memory.LaneData, memory.LaneVec, memory.LaneMem0, memory.LaneSupermemory}

// recallMemory is POST /v1/memory/recall.
func (s *Server) recallMemory(w http.ResponseWriter, r *http.Request, m memory.Memory) error {
	var req wire.RecallRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	text := strings.TrimSpace(req.Query)
	if n := utf8.RuneCountInString(text); n < 1 || n > maxRecallRunes {
		return badRequest("query", "a query is 1 to %d characters", maxRecallRunes)
	}
	q := memory.Query{Text: text}
	if req.AsOf != "" {
		d, err := civil.Parse(req.AsOf)
		if err != nil {
			return badRequest("as_of", "as_of is a day like 2026-10-05, or empty for now")
		}
		// As of a day is as of its end: what held that day.
		q.AsOf = d.AddDays(1).Midnight(s.Loc).Add(-time.Millisecond)
	}
	for _, l := range req.Lanes {
		if !slices.Contains(knownLanes, memory.LaneName(l)) {
			return badRequest("lanes", "%q is not a lane", l)
		}
		q.Lanes = append(q.Lanes, memory.LaneName(l))
	}
	p, err := m.Recall(r.Context(), q)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, recallWire(p))
	return nil
}

// listConflicts is GET /v1/memory/conflicts?status= (open by default, all
// for every status).
func (s *Server) listConflicts(w http.ResponseWriter, r *http.Request, m memory.Memory) error {
	status := r.URL.Query().Get("status")
	switch status {
	case "":
		status = model.ConflictOpen
	case "all":
		status = ""
	case model.ConflictOpen, model.ConflictResolved, model.ConflictDismissed:
	default:
		return badRequest("status", "status is open, resolved, dismissed, or all")
	}
	cs, err := m.Conflicts(r.Context(), status)
	if err != nil {
		return err
	}
	out := wire.MemoryConflictList{Conflicts: make([]wire.MemoryConflict, len(cs))}
	for i, c := range cs {
		out.Conflicts[i] = conflictWire(c)
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// resolveConflict is POST /v1/memory/conflicts/{id}/resolve.
func (s *Server) resolveConflict(w http.ResponseWriter, r *http.Request, m memory.Memory) error {
	var req wire.ResolveConflictRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	switch req.Resolution {
	case wire.ResolveKeepA, wire.ResolveKeepB, wire.ResolveBoth, wire.ResolveNeither:
	default:
		return badRequest("resolution", "resolution is keep_a, keep_b, both, or neither")
	}
	c, err := m.Resolve(r.Context(), r.PathValue("id"), req.Resolution)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, conflictWire(c))
	return nil
}

// listEntities is GET /v1/memory/entities.
func (s *Server) listEntities(w http.ResponseWriter, r *http.Request, m memory.Memory) error {
	es, err := m.Entities(r.Context())
	if err != nil {
		return err
	}
	out := wire.MemoryEntityList{Entities: make([]wire.MemoryEntity, len(es))}
	for i, e := range es {
		out.Entities[i] = entityWire(e)
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// memoryProfile is GET /v1/memory/profile: today's profile card.
func (s *Server) memoryProfile(w http.ResponseWriter, r *http.Request, m memory.Memory) error {
	card, err := m.Profile(r.Context(), s.today())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, profileWire(card))
	return nil
}

// listResources is GET /v1/memory/resources.
func (s *Server) listResources(w http.ResponseWriter, r *http.Request, m memory.Memory) error {
	rs, err := m.Resources(r.Context())
	if err != nil {
		return err
	}
	out := wire.MemoryResourceList{Resources: make([]wire.MemoryResource, len(rs))}
	for i, res := range rs {
		out.Resources[i] = resourceWire(res)
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// addResource is POST /v1/memory/resources: a document on the daemon's
// device for Gwen to read and remember.
func (s *Server) addResource(w http.ResponseWriter, r *http.Request, m memory.Memory) error {
	var req wire.AddResourceRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	path, title := strings.TrimSpace(req.Path), strings.TrimSpace(req.Title)
	if path == "" || !filepath.IsAbs(path) {
		return badRequest("path", "path is the absolute path of a file on this device")
	}
	if utf8.RuneCountInString(title) > maxTitleRunes {
		return badRequest("title", "a title is at most %d characters", maxTitleRunes)
	}
	res, err := m.AddResource(r.Context(), memory.AddResourceRequest{Path: filepath.Clean(path), Title: title})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, resourceWire(res))
	return nil
}

// removeResource is DELETE /v1/memory/resources/{id}.
func (s *Server) removeResource(w http.ResponseWriter, r *http.Request, m memory.Memory) error {
	rc, err := m.RemoveResource(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, receiptWire(rc))
	return nil
}

// rebuildLanes are the derived lanes POST /v1/memory/rebuild rebuilds.
var rebuildLanes = []memory.LaneName{memory.LaneVec, memory.LaneFiles, memory.LaneMem0, memory.LaneSupermemory}

// rebuildMemory is POST /v1/memory/rebuild.
func (s *Server) rebuildMemory(w http.ResponseWriter, r *http.Request, m memory.Memory) error {
	var req wire.RebuildRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	if !slices.Contains(rebuildLanes, memory.LaneName(req.Lane)) {
		return badRequest("lane", "lane is vec, files, mem0, or supermemory")
	}
	if err := m.Rebuild(r.Context(), memory.LaneName(req.Lane)); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// sleepMemory is POST /v1/memory/sleep: consolidation now.
func (s *Server) sleepMemory(w http.ResponseWriter, r *http.Request, m memory.Memory) error {
	rep, err := m.Sleep(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, wire.SleepReport{Learned: rep.Learned, Merged: rep.Merged, Conflicts: rep.Conflicts,
		Insights: rep.Insights, Observed: rep.Observed, Reconciled: rep.Reconciled, Profile: rep.Profile})
	return nil
}

// exportMemory is GET /v1/memory/export.
func (s *Server) exportMemory(w http.ResponseWriter, r *http.Request, m memory.Memory) error {
	ex, err := m.Export(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, exportWire(ex))
	return nil
}

// ForgetAllRequest is the POST /v1/memory/forget-all body: Confirm must be
// ForgetAllConfirm.
type ForgetAllRequest struct {
	Confirm string `json:"confirm"`
}

// forgetAll is POST /v1/memory/forget-all: everything memory holds is
// erased; chats are kept.
func (s *Server) forgetAll(w http.ResponseWriter, r *http.Request, m memory.Memory) error {
	var req ForgetAllRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.Confirm != ForgetAllConfirm {
		return badRequest("confirm", "to erase all memory, confirm with %q", ForgetAllConfirm)
	}
	rc, err := m.ForgetAll(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, receiptWire(rc))
	return nil
}

// selfHistory is GET /v1/assistant/self/history?limit=: how the assistant
// chose to treat the user over time, newest first.
func (s *Server) selfHistory(w http.ResponseWriter, r *http.Request) error {
	limit, err := queryLimit(r, DefaultSelfHist, MaxSelfHist)
	if err != nil {
		return err
	}
	vs, err := s.Repos.Self.History(r.Context(), limit)
	if err != nil {
		return err
	}
	out := wire.SelfHistory{Versions: make([]wire.SelfVersion, len(vs))}
	for i, v := range vs {
		out.Versions[i] = wire.SelfVersion{ID: v.ID, Attitude: v.Attitude, Note: v.Note, Since: wire.MillisPtr(v.Since),
			Reason: v.Reason, ThreadID: v.ThreadID, At: wire.Millis(v.CreatedAt)}
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// resolveRefs resolves refs for showing, each once; one that fails to
// resolve is shown as its ref alone.
func (s *Server) resolveRefs(ctx context.Context, res MemoryResolver, refs []string,
	seen map[string]wire.MemoryRef) []wire.MemoryRef {
	if len(refs) == 0 {
		return nil
	}
	out := make([]wire.MemoryRef, 0, len(refs))
	for _, ref := range refs {
		got, ok := seen[ref]
		if !ok {
			var err error
			if got, err = res.ResolveRef(ctx, ref); err != nil {
				slog.Warn("memory ref unresolved", "ref", ref, "err", err)
				kind, _, _ := memory.ParseRef(ref)
				got = wire.MemoryRef{Ref: ref, Kind: refKindName(kind)}
			}
			seen[ref] = got
		}
		out = append(out, got)
	}
	return out
}

// refKindName is the kind of a ref as wire.MemoryRef says it.
func refKindName(kind string) string {
	switch kind {
	case memory.RefClaim:
		return "claim"
	case memory.RefMessage:
		return "message"
	case memory.RefEpisode:
		return "episode"
	case memory.RefResource:
		return "resource"
	case memory.RefData:
		return "data"
	}
	return ""
}

// storeResolver resolves refs from the store when the daemon runs no memory
// that does: sources are plain words.
type storeResolver struct{ repos store.Repos }

func (sr storeResolver) ResolveRef(ctx context.Context, ref string) (wire.MemoryRef, error) {
	kind, id, ok := memory.ParseRef(ref)
	out := wire.MemoryRef{Ref: ref, Kind: refKindName(kind)}
	if !ok {
		out.Gone = true
		return out, nil
	}
	var err error
	switch kind {
	case memory.RefClaim:
		var c model.Claim
		if c, err = sr.repos.Memory.Claim(ctx, id); err == nil {
			out.Text, out.Status = c.Text, c.Status
			out.Source, out.ThreadID = sr.claimSource(ctx, c)
		}
	case memory.RefMessage:
		var h model.ChatHit
		if h, err = sr.repos.Memory.Message(ctx, id); err == nil {
			out.Text, out.Source, out.ThreadID = memory.Cut(h.Text, 120), "chat “"+h.ThreadTitle+"”", h.ThreadID
		}
	case memory.RefEpisode:
		var e model.Episode
		if e, err = sr.repos.Memory.Episode(ctx, id); err == nil {
			out.Text, out.Source = cmp.Or(e.Title, memory.Cut(e.Body, 120)), episodeSource(e)
		}
	case memory.RefResource:
		var res model.Resource
		if res, err = sr.repos.MemoryState.Resource(ctx, id); err == nil {
			out.Text, out.Source = res.Title, "a document"
		}
	case memory.RefData:
		out.Text, out.Source = id, "your tracked data"
	}
	if errors.Is(err, store.ErrNotFound) {
		return wire.MemoryRef{Ref: ref, Kind: out.Kind, Gone: true}, nil
	}
	return out, err
}

func (sr storeResolver) Learned(ctx context.Context, msgIDs []string) (map[string][]wire.MemoryRef, error) {
	out := map[string][]wire.MemoryRef{}
	for _, id := range msgIDs {
		claims, err := sr.repos.Memory.ClaimsFrom(ctx, model.SourceMessage, id)
		if err != nil {
			return nil, err
		}
		for _, c := range claims {
			if c.Status == model.ClaimRejected {
				continue
			}
			out[id] = append(out[id], wire.MemoryRef{Ref: memory.Ref(memory.RefClaim, c.ID), Kind: "claim", Text: c.Text,
				Source: "this message", Status: c.Status})
		}
	}
	return out, nil
}

// claimSource is where a claim came from in plain words, and its chat.
func (sr storeResolver) claimSource(ctx context.Context, c model.Claim) (source, threadID string) {
	switch c.SourceKind {
	case model.SourceMessage, model.SourceMem0:
		if h, err := sr.repos.Memory.Message(ctx, c.SourceID); err == nil {
			return "chat “" + h.ThreadTitle + "”", h.ThreadID
		}
		return "a chat", ""
	case model.SourceEpisode:
		if e, err := sr.repos.Memory.Episode(ctx, c.SourceID); err == nil {
			return episodeSource(e), ""
		}
		return "a note", ""
	case model.SourceUserUI:
		return "your edit", ""
	case model.SourceData:
		return "your tracked data", ""
	case model.SourceSuperM:
		return "your documents (Supermemory)", ""
	case model.SourceClaim:
		return "other memories", ""
	}
	return "", ""
}

// episodeSource is where an episode came from in plain words.
func episodeSource(e model.Episode) string {
	switch {
	case e.Kind == model.EpisodeResource:
		return "a document"
	case e.Kind == model.EpisodeEdit:
		return "your edit"
	case e.Channel == model.ChannelCLI:
		return "a note in the terminal"
	}
	return "a note in the dashboard"
}

// Conversions of memory to wire.

func claimsWire(vs []memory.ClaimView) []wire.MemoryClaim {
	out := make([]wire.MemoryClaim, len(vs))
	for i, v := range vs {
		out[i] = claimWire(v)
	}
	return out
}

func claimWire(v memory.ClaimView) wire.MemoryClaim {
	c := v.Claim
	value := json.RawMessage(c.Value)
	if len(value) == 0 || !json.Valid(value) {
		value = json.RawMessage("null")
	}
	ref := ""
	switch c.SourceKind {
	case model.SourceMessage, model.SourceEpisode:
		if c.SourceID != "" {
			ref = memory.Ref(c.SourceKind, c.SourceID)
		}
	}
	return wire.MemoryClaim{ID: c.ID, Key: c.Key, Label: v.Label, Subject: v.Subject, SubjectID: c.SubjectID,
		Text: c.Text, Value: value, Status: c.Status, Trust: c.Trust, Sensitivity: c.Sensitivity,
		Confidence: c.Confidence, ValidFrom: wire.MillisPtr(c.ValidFrom), ValidTo: wire.MillisPtr(c.ValidTo),
		MentionedAt: wire.Millis(c.MentionedAt), RecordedAt: wire.Millis(c.RecordedAt), SupersededBy: c.SupersededBy,
		Hidden: c.Suppressed, Source: v.Source, SourceRef: ref, ThreadID: v.ThreadID, Quote: c.Span,
		Disputed: v.Disputed}
}

func conflictWire(c memory.ConflictView) wire.MemoryConflict {
	return wire.MemoryConflict{ID: c.ID, Reason: c.Reason, Status: c.Status, Resolution: c.Resolution,
		A: claimWire(c.A), B: claimWire(c.B), CreatedAt: wire.Millis(c.CreatedAt)}
}

func entityWire(e model.Entity) wire.MemoryEntity {
	return wire.MemoryEntity{ID: e.ID, Kind: e.Kind, Name: e.Name, Aliases: append([]string{}, e.Aliases...)}
}

func episodeWire(e model.Episode) wire.MemoryEpisode {
	return wire.MemoryEpisode{ID: e.ID, Kind: e.Kind, Channel: e.Channel, Speaker: e.Speaker, Title: e.Title,
		Body: e.Body, Source: e.Source, At: wire.Millis(e.At), Sensitivity: e.Sensitivity}
}

func resourceWire(r model.Resource) wire.MemoryResource {
	return wire.MemoryResource{ID: r.ID, Title: r.Title, Path: r.Path, Mime: r.Mime, Size: r.Size,
		AddedAt: wire.Millis(r.AddedAt), DocumentAt: wire.MillisPtr(r.DocumentAt)}
}

func profileWire(c model.ProfileCard) wire.ProfileCard {
	return wire.ProfileCard{Day: c.Day, Text: c.Text, CreatedAt: wire.Millis(c.CreatedAt)}
}

func receiptWire(r memory.ForgetReceipt) wire.ForgetReceipt {
	out := wire.ForgetReceipt{Claims: r.Claims, Episodes: r.Episodes, Lanes: make([]wire.LaneReceipt, len(r.Lanes))}
	for i, l := range r.Lanes {
		out.Lanes[i] = wire.LaneReceipt{Lane: string(l.Lane), OK: l.OK, Note: l.Note}
	}
	return out
}

func memoryStatusWire(st memory.Status) wire.MemoryStatus {
	out := wire.MemoryStatus{Enabled: st.Enabled, Learning: st.Learning, Claims: map[string]int{},
		Conflicts: st.Conflicts, Jobs: map[string]int{}, Lanes: make([]wire.MemoryLane, len(st.Lanes)),
		LastLearnedAt: wire.MillisPtr(st.LastLearned), LastSleptAt: wire.MillisPtr(st.LastSlept),
		LedgerRows: st.Ledger.Rows, LedgerOK: st.Ledger.Verified}
	for k, v := range st.Claims {
		out.Claims[k] = v
	}
	for k, v := range st.Jobs {
		out.Jobs[k] = v
	}
	for i, l := range st.Lanes {
		out.Lanes[i] = wire.MemoryLane{Lane: string(l.Lane), Enabled: l.Enabled, OK: l.OK, Items: l.Items, Detail: l.Detail}
	}
	if st.Profile != nil {
		p := profileWire(*st.Profile)
		out.Profile = &p
	}
	return out
}

func recallWire(p memory.Packet) wire.RecallResult {
	out := wire.RecallResult{Intent: p.Intent, Hits: make([]wire.MemoryHit, len(p.Hits)), Tokens: p.Tokens,
		Abstain: p.Abstain, Degraded: make([]string, len(p.Degraded))}
	for i, h := range p.Hits {
		lanes := make([]string, len(h.Lanes))
		for j, l := range h.Lanes {
			lanes[j] = string(l)
		}
		out.Hits[i] = wire.MemoryHit{Ref: h.Ref, Kind: h.Kind, Key: h.Key, Subject: h.Subject, Text: h.Text,
			Source: h.Source, At: wire.Millis(h.At), ValidFrom: wire.MillisPtr(h.ValidFrom),
			ValidTo: wire.MillisPtr(h.ValidTo), Trust: h.Trust, Sensitivity: h.Sensitivity, Disputed: h.Disputed,
			Score: h.Score, Lanes: lanes}
	}
	for i, l := range p.Degraded {
		out.Degraded[i] = string(l)
	}
	return out
}

func explanationWire(e memory.Explanation) wire.MemoryExplanation {
	out := wire.MemoryExplanation{Ref: e.Ref, Quote: e.Quote, Context: make([]wire.AssistantMessage, len(e.Context)),
		History: claimsWire(e.History), Links: make([]wire.MemoryLink, len(e.Links))}
	if e.Claim != nil {
		c := claimWire(*e.Claim)
		out.Claim = &c
	}
	for i, m := range e.Context {
		out.Context[i] = messageWire(m)
	}
	if e.Episode != nil {
		ep := episodeWire(*e.Episode)
		out.Episode = &ep
	}
	for i, l := range e.Links {
		out.Links[i] = wire.MemoryLink{ToKind: l.ToKind, ToID: l.ToID, Relation: l.Relation}
	}
	return out
}

func exportWire(e memory.Export) wire.MemoryExport {
	out := wire.MemoryExport{ExportedAt: wire.Millis(e.ExportedAt), Claims: make([]wire.MemoryClaim, len(e.Claims)),
		Entities: make([]wire.MemoryEntity, len(e.Entities)), Episodes: make([]wire.MemoryEpisode, len(e.Episodes)),
		Conflicts: make([]wire.MemoryConflict, len(e.Conflicts)), Resources: make([]wire.MemoryResource, len(e.Resources))}
	views := map[string]memory.ClaimView{}
	for i, c := range e.Claims {
		views[c.ID] = memory.ClaimView{Claim: c}
		out.Claims[i] = claimWire(views[c.ID])
	}
	for i, en := range e.Entities {
		out.Entities[i] = entityWire(en)
	}
	for i, ep := range e.Episodes {
		out.Episodes[i] = episodeWire(ep)
	}
	view := func(id string) memory.ClaimView {
		if v, ok := views[id]; ok {
			return v
		}
		return memory.ClaimView{Claim: model.Claim{ID: id}}
	}
	for i, c := range e.Conflicts {
		out.Conflicts[i] = conflictWire(memory.ConflictView{Conflict: c, A: view(c.ClaimA), B: view(c.ClaimB)})
	}
	for i, r := range e.Resources {
		out.Resources[i] = resourceWire(r)
	}
	return out
}
