package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/timeengine"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// testServer is a Server over a fresh store, served on a socket, with a fake
// tracker and a scripted assistant. Its clock reads 2026-09-15 08:00 UTC.
type testServer struct {
	*Server
	c       *client.Client
	planner *fakePlanner
	ctx     context.Context
}

// newTestServer starts a Server; mem nil runs no memory.
func newTestServer(t *testing.T, mem memory.Memory) *testServer {
	t.Helper()
	clk := clock.NewFake(testutil.T0.Add(-time.Hour))
	db := testutil.NewDBWithClock(t, clk)
	cfg := config.Defaults()
	cfg.LLM.Provider = config.ProviderAnthropic
	p := &fakePlanner{}
	s := &Server{DB: db, Repos: store.NewRepos(db), Tracker: &fakeTracker{cfg: cfg}, Hub: NewHub(clk), Clock: clk,
		Loc: time.UTC, Memory: mem, LLM: func(config.LLM, llm.Self) (llm.Planner, error) { return p, nil }}
	sock := testutil.SocketPath(t)
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		s.Hub.Close()
		_ = srv.Close()
	})
	return &testServer{Server: s, c: client.New(sock), planner: p, ctx: context.Background()}
}

// apiCode requires err to be an API error with code.
func apiCode(t *testing.T, err error, code string) *client.APIError {
	t.Helper()
	var ae *client.APIError
	require.ErrorAs(t, err, &ae)
	require.Equal(t, code, ae.Code, ae.Message)
	return ae
}

type fakeTracker struct {
	mu  sync.Mutex
	cfg config.Config
}

func (f *fakeTracker) Do(context.Context, func(time.Time) timeengine.Input) (wire.Status, error) {
	return wire.Status{}, nil
}
func (f *fakeTracker) Status(context.Context) (wire.Status, error) { return wire.Status{}, nil }
func (f *fakeTracker) Snapshot(context.Context) (timeengine.Snapshot, error) {
	return timeengine.Snapshot{}, nil
}
func (f *fakeTracker) Config() config.Config {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cfg
}
func (f *fakeTracker) SetConfig(_ context.Context, c config.Config) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cfg = c
	return nil
}

// fakePlanner answers assistant turns with reply and keeps what was sent.
type fakePlanner struct {
	mu    sync.Mutex
	reply llm.AssistantOutput
	reqs  []llm.AssistantRequest
}

var errNotUsed = errors.New("not used in these tests")

func (p *fakePlanner) Breakdown(context.Context, llm.BreakdownRequest) (llm.BreakdownOutput, error) {
	return llm.BreakdownOutput{}, errNotUsed
}
func (p *fakePlanner) DayPlan(context.Context, llm.DayPlanRequest) (llm.DayPlanOutput, error) {
	return llm.DayPlanOutput{}, errNotUsed
}
func (p *fakePlanner) Retro(context.Context, llm.RetroRequest) (llm.RetroOutput, error) {
	return llm.RetroOutput{}, errNotUsed
}
func (p *fakePlanner) Name() string { return "fake" }
func (p *fakePlanner) Assistant(_ context.Context, req llm.AssistantRequest) (llm.AssistantOutput, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reqs = append(p.reqs, req)
	return p.reply, nil
}

func (p *fakePlanner) answer(reply string, cites, forget []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reply = llm.AssistantOutput{Reply: reply, Mood: "smiling", Actions: []llm.Action{}, Cites: cites, Forget: forget}
}

func (p *fakePlanner) last() llm.AssistantRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.reqs[len(p.reqs)-1]
}

// fakeMemory is a scripted memory.Memory that also resolves refs. It keeps
// the arguments it was called with.
type fakeMemory struct {
	mu sync.Mutex
	// err, when set, is what every method that can fail returns.
	err error

	note     memory.Note
	noteErr  error
	noteReqs []memory.NoteRequest
	card     model.ProfileCard
	cardDays []string
	obs      map[string]model.ChatObservation
	turns    []memory.ChatTurn

	claims   []memory.ClaimView
	filters  []store.ClaimFilter
	queries  []memory.Query
	remember []memory.RememberRequest
	edits    []memory.EditRequest
	statuses []string
	rebuilt  []memory.LaneName
	added    []memory.AddResourceRequest
	learned  map[string][]wire.MemoryRef
}

var _ memory.Memory = (*fakeMemory)(nil)
var _ MemoryResolver = (*fakeMemory)(nil)

func (f *fakeMemory) lock() func() {
	f.mu.Lock()
	return f.mu.Unlock
}

func (f *fakeMemory) claim(id string) (memory.ClaimView, bool) {
	for _, c := range f.claims {
		if c.ID == id {
			return c, true
		}
	}
	return memory.ClaimView{}, false
}

func (f *fakeMemory) Note(_ context.Context, req memory.NoteRequest) (memory.Note, error) {
	defer f.lock()()
	f.noteReqs = append(f.noteReqs, req)
	return f.note, f.noteErr
}

func (f *fakeMemory) Recall(_ context.Context, q memory.Query) (memory.Packet, error) {
	defer f.lock()()
	f.queries = append(f.queries, q)
	return memory.Packet{Intent: memory.IntentLookup, Tokens: 12, Degraded: []memory.LaneName{memory.LaneVec},
			Hits: []memory.Hit{{Ref: "claim:" + claimA, Kind: "claim", Key: "user.employer", Text: "Works at Acme",
				Source: "chat “Work”, 3 Sep", At: testutil.T0, Score: 0.5, Lanes: []memory.LaneName{memory.LaneClaims}}}},
		f.err
}

func (f *fakeMemory) Get(context.Context, string, *string, time.Time) (model.Claim, error) {
	return model.Claim{}, store.ErrNotFound
}

func (f *fakeMemory) Explain(_ context.Context, ref string) (memory.Explanation, error) {
	defer f.lock()()
	if f.err != nil {
		return memory.Explanation{}, f.err
	}
	kind, id, _ := memory.ParseRef(ref)
	v, ok := f.claim(id)
	if kind != memory.RefClaim || !ok {
		return memory.Explanation{}, store.Fail(store.ErrNotFound, "nothing is remembered as %s", ref)
	}
	return memory.Explanation{Ref: ref, Claim: &v, Quote: v.Span, History: []memory.ClaimView{v},
		Context: []model.ChatMessage{{ID: "m", Role: llm.RoleUser, Text: "I work at Acme", At: testutil.T0,
			Actions: []byte("[]")}}}, nil
}

func (f *fakeMemory) Claims(_ context.Context, flt store.ClaimFilter) ([]memory.ClaimView, error) {
	defer f.lock()()
	f.filters = append(f.filters, flt)
	return slices.Clone(f.claims), f.err
}

func (f *fakeMemory) Entities(context.Context) ([]model.Entity, error) {
	return []model.Entity{{ID: entityAnna, Kind: "person", Name: "Anna"}}, f.err
}

func (f *fakeMemory) Conflicts(_ context.Context, status string) ([]memory.ConflictView, error) {
	defer f.lock()()
	f.statuses = append(f.statuses, status)
	a, _ := f.claim(claimA)
	b, _ := f.claim(claimB)
	return []memory.ConflictView{{Conflict: model.Conflict{ID: conflictID, ClaimA: claimA, ClaimB: claimB,
		Reason: "two employers", Status: model.ConflictOpen}, A: a, B: b}}, f.err
}

func (f *fakeMemory) Profile(_ context.Context, day string) (model.ProfileCard, error) {
	defer f.lock()()
	f.cardDays = append(f.cardDays, day)
	if f.card.Text == "" {
		return model.ProfileCard{}, store.ErrNotFound
	}
	return f.card, nil
}

func (f *fakeMemory) Observation(_ context.Context, threadID string) (model.ChatObservation, bool, error) {
	defer f.lock()()
	o, ok := f.obs[threadID]
	return o, ok, nil
}

func (f *fakeMemory) Status(context.Context) memory.Status {
	slept := testutil.T0
	return memory.Status{Enabled: true, Learning: true, Claims: map[string]int{"active": 2}, Conflicts: 1,
		Lanes:     []memory.LaneStatus{{Lane: memory.LaneVec, Enabled: true, OK: false, Items: -1, Detail: "down"}},
		LastSlept: &slept, Ledger: memory.LedgerStatus{Rows: 9, Verified: true}}
}

func (f *fakeMemory) ChatTurn(_ context.Context, t memory.ChatTurn) {
	defer f.lock()()
	f.turns = append(f.turns, t)
}

func (f *fakeMemory) Remember(_ context.Context, req memory.RememberRequest) ([]model.Claim, error) {
	defer f.lock()()
	f.remember = append(f.remember, req)
	a, _ := f.claim(claimA)
	return []model.Claim{a.Claim}, f.err
}

func (f *fakeMemory) Edit(_ context.Context, _ string, req memory.EditRequest) (model.Claim, error) {
	defer f.lock()()
	f.edits = append(f.edits, req)
	b, _ := f.claim(claimB)
	return b.Claim, f.err
}

func (f *fakeMemory) change(id, status string) (model.Claim, error) {
	defer f.lock()()
	v, ok := f.claim(id)
	if !ok {
		return model.Claim{}, store.Fail(store.ErrNotFound, "no claim %s", id)
	}
	if status != "" {
		v.Status = status
	}
	return v.Claim, f.err
}

func (f *fakeMemory) Confirm(_ context.Context, id string) (model.Claim, error) {
	return f.change(id, model.ClaimActive)
}
func (f *fakeMemory) Reject(_ context.Context, id string) (model.Claim, error) {
	return f.change(id, model.ClaimRejected)
}
func (f *fakeMemory) Suppress(_ context.Context, id string, on bool) (model.Claim, error) {
	c, err := f.change(id, "")
	c.Suppressed = on
	return c, err
}

func (f *fakeMemory) Forget(_ context.Context, id string) (memory.ForgetReceipt, error) {
	if _, err := f.change(id, ""); err != nil {
		return memory.ForgetReceipt{}, err
	}
	return memory.ForgetReceipt{Claims: 1, Lanes: []memory.LaneReceipt{{Lane: memory.LaneClaims, OK: true},
		{Lane: memory.LaneMem0, OK: false, Note: "Mem0 did not answer; it will be retried"}}}, nil
}

func (f *fakeMemory) ForgetAll(context.Context) (memory.ForgetReceipt, error) {
	return memory.ForgetReceipt{Claims: 2, Episodes: 1, Lanes: []memory.LaneReceipt{}}, f.err
}

func (f *fakeMemory) Resolve(_ context.Context, id, resolution string) (memory.ConflictView, error) {
	cs, _ := f.Conflicts(context.Background(), "")
	c := cs[0]
	c.ID, c.Status, c.Resolution = id, model.ConflictResolved, &resolution
	return c, f.err
}

func (f *fakeMemory) AddResource(_ context.Context, req memory.AddResourceRequest) (model.Resource, error) {
	defer f.lock()()
	f.added = append(f.added, req)
	return model.Resource{ID: resourceID, Title: "Lease", Path: req.Path, Mime: "application/pdf", Size: 10,
		AddedAt: testutil.T0}, f.err
}

func (f *fakeMemory) Resources(context.Context) ([]model.Resource, error) {
	return []model.Resource{{ID: resourceID, Title: "Lease", Path: "/home/u/lease.pdf", AddedAt: testutil.T0}}, f.err
}

func (f *fakeMemory) RemoveResource(context.Context, string) (memory.ForgetReceipt, error) {
	return memory.ForgetReceipt{Episodes: 1, Lanes: []memory.LaneReceipt{}}, f.err
}

func (f *fakeMemory) Rebuild(_ context.Context, lane memory.LaneName) error {
	defer f.lock()()
	f.rebuilt = append(f.rebuilt, lane)
	return f.err
}

func (f *fakeMemory) Sleep(context.Context) (memory.SleepReport, error) {
	return memory.SleepReport{Learned: 3, Profile: true}, f.err
}

func (f *fakeMemory) Export(context.Context) (memory.Export, error) {
	defer f.lock()()
	ex := memory.Export{ExportedAt: testutil.T0, Conflicts: []model.Conflict{{ID: conflictID, ClaimA: claimA,
		ClaimB: claimGone, Status: model.ConflictOpen}}}
	for _, c := range f.claims {
		ex.Claims = append(ex.Claims, c.Claim)
	}
	return ex, f.err
}

func (f *fakeMemory) ResolveRef(_ context.Context, ref string) (wire.MemoryRef, error) {
	return wire.MemoryRef{Ref: ref, Kind: "claim", Text: "resolved " + ref, Source: "the fake"}, nil
}

func (f *fakeMemory) Learned(_ context.Context, ids []string) (map[string][]wire.MemoryRef, error) {
	defer f.lock()()
	out := map[string][]wire.MemoryRef{}
	for _, id := range ids {
		if l, ok := f.learned[id]; ok {
			out[id] = l
		}
	}
	return out, nil
}

// Fixed ids of the fake memory's rows.
const (
	claimA     = "11111111-1111-4111-8111-111111111111"
	claimB     = "22222222-2222-4222-8222-222222222222"
	claimGone  = "33333333-3333-4333-8333-333333333333"
	entityAnna = "44444444-4444-4444-8444-444444444444"
	conflictID = "55555555-5555-4555-8555-555555555555"
	resourceID = "66666666-6666-4666-8666-666666666666"
)

// newFakeMemory holds two claims: where they work (active, from a chat) and
// a candidate.
func newFakeMemory() *fakeMemory {
	from := testutil.T0.Add(-30 * 24 * time.Hour)
	return &fakeMemory{obs: map[string]model.ChatObservation{}, claims: []memory.ClaimView{
		{Claim: model.Claim{ID: claimA, Key: "user.employer", Value: []byte(`"Acme"`), Text: "Works at Acme",
			Status: model.ClaimActive, Trust: model.TrustOwnerSaid, Sensitivity: model.SensitivityPersonal,
			Confidence: 0.9, ValidFrom: &from, MentionedAt: testutil.T0, RecordedAt: testutil.T0,
			SourceKind: model.SourceMessage, SourceID: "77777777-7777-4777-8777-777777777777", Span: "I work at Acme"},
			Label: "Works at", Source: "chat “Work”, 15 Sep", ThreadID: "t1"},
		{Claim: model.Claim{ID: claimB, Key: "user.home", Value: []byte(`"Pune"`), Text: "Lives in Pune",
			Status: model.ClaimCandidate, Trust: model.TrustInferred, Sensitivity: model.SensitivityPersonal,
			MentionedAt: testutil.T0, RecordedAt: testutil.T0, SourceKind: model.SourceUserUI},
			Label: "Lives in", Source: "your edit", Disputed: true},
	}}
}

// plainMemory is a memory that does not resolve refs itself.
type plainMemory struct{ memory.Memory }
