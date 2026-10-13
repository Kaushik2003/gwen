package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/memory/files"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

func init() {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// fakeModel is a scripted llm.Memory: Extract finds each scripted fact's
// quote in the items and answers with those found; the other jobs answer
// nothing unless set.
type fakeModel struct {
	mu         sync.Mutex
	facts      []llm.ExtractedFact
	extractErr error
	contradict func(llm.ContradictRequest) (llm.ContradictOutput, error)
	observe    func(llm.ObserveRequest) (llm.ObserveOutput, error)

	extracts, contradicts, insights, observes int
}

func (m *fakeModel) Name() string { return "fake/model" }

func (m *fakeModel) Extract(_ context.Context, req llm.ExtractRequest) (llm.ExtractOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.extracts++
	if m.extractErr != nil {
		return llm.ExtractOutput{}, m.extractErr
	}
	out := llm.ExtractOutput{Facts: []llm.ExtractedFact{}}
	for _, f := range m.facts {
		for _, it := range req.Items {
			if memory.ContainsQuote(it.Text, f.Quote) {
				f.Source = it.Ref
				out.Facts = append(out.Facts, f)
				break
			}
		}
	}
	return out, nil
}

func (m *fakeModel) Verify(_ context.Context, req llm.VerifyRequest) (llm.VerifyOutput, error) {
	out := llm.VerifyOutput{}
	for _, c := range req.Checks {
		out.Verdicts = append(out.Verdicts, llm.Verdict{Ref: c.Ref, Supported: "yes"})
	}
	return out, nil
}

func (m *fakeModel) Observe(_ context.Context, req llm.ObserveRequest) (llm.ObserveOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observes++
	if m.observe != nil {
		return m.observe(req)
	}
	return llm.ObserveOutput{}, nil
}

func (m *fakeModel) Reflect(_ context.Context, req llm.ReflectRequest) (llm.ReflectOutput, error) {
	return llm.ReflectOutput{Log: req.Log}, nil
}

func (m *fakeModel) Contradict(_ context.Context, req llm.ContradictRequest) (llm.ContradictOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.contradicts++
	if m.contradict != nil {
		return m.contradict(req)
	}
	return llm.ContradictOutput{}, nil
}

func (m *fakeModel) Insight(context.Context, llm.InsightRequest) (llm.InsightOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.insights++
	return llm.InsightOutput{}, nil
}

func (m *fakeModel) count(n *int) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return *n
}

// fakeLane stands in for the vector index, Mem0, or Supermemory: it keeps
// records by id and records every call.
type fakeLane struct {
	mu         sync.Mutex
	name       memory.LaneName
	items      map[string]memory.Record
	ext        map[string]string // a Mem0 id -> the claim id it holds
	upserted   []string
	deleted    []string
	cleared    int
	closed     bool
	upsertErr  error
	deleteErr  error
	healthErr  error
	hits       []memory.Hit
	onStored   func(claimID, externalID string)
	searchHits int
}

func newFakeLane(name memory.LaneName) *fakeLane {
	return &fakeLane{name: name, items: map[string]memory.Record{}, ext: map[string]string{}}
}

func (l *fakeLane) Name() memory.LaneName { return l.name }

func (l *fakeLane) Search(context.Context, memory.Query) ([]memory.Hit, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.searchHits++
	return slices.Clone(l.hits), nil
}

func (l *fakeLane) Upsert(_ context.Context, recs []memory.Record) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.upsertErr != nil {
		return l.upsertErr
	}
	for _, r := range recs {
		l.items[r.ID] = r
		l.upserted = append(l.upserted, r.ID)
		if l.onStored != nil {
			id := bareClaimID(r.ID)
			l.ext["m-"+id] = r.ID
			l.mu.Unlock()
			l.onStored(id, "m-"+id)
			l.mu.Lock()
		}
	}
	return nil
}

func (l *fakeLane) Delete(_ context.Context, ids []string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.deleteErr != nil {
		return l.deleteErr
	}
	for _, id := range ids {
		l.deleted = append(l.deleted, id)
		delete(l.items, id)
		if key, ok := l.ext[id]; ok {
			delete(l.items, key)
			delete(l.ext, id)
		}
	}
	return nil
}

func (l *fakeLane) Health(context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.healthErr
}

func (l *fakeLane) Count(context.Context) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.items), nil
}

func (l *fakeLane) Refs(context.Context) ([]string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, 0, len(l.items))
	for id := range l.items {
		out = append(out, id)
	}
	slices.Sort(out)
	return out, nil
}

func (l *fakeLane) Clear(context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cleared++
	l.items = map[string]memory.Record{}
	return nil
}

func (l *fakeLane) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	return nil
}

// has reports whether the lane holds id.
func (l *fakeLane) has(id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, ok := l.items[id]
	return ok
}

func (l *fakeLane) deletes() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.deleted)
}

func (l *fakeLane) set(fn func(l *fakeLane)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fn(l)
}

// fakeFolder stands in for the memory folder.
type fakeFolder struct {
	mu        sync.Mutex
	dir       string
	changes   []files.Change // the next Scan's
	scanErr   error
	projected [][]memory.ClaimView
	forgot    int
}

func (f *fakeFolder) Dir() string { return f.dir }

func (f *fakeFolder) Scan(context.Context) ([]files.Change, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.scanErr != nil {
		return nil, f.scanErr
	}
	out := f.changes
	f.changes = nil
	return out, nil
}

func (f *fakeFolder) Project(_ context.Context, claims []memory.ClaimView, _ []model.Entity) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.projected = append(f.projected, claims)
	return nil
}

func (f *fakeFolder) ForgetHistory(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forgot++
	return nil
}

// last is the texts of the last projection.
func (f *fakeFolder) last() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.projected) == 0 {
		return nil
	}
	var out []string
	for _, v := range f.projected[len(f.projected)-1] {
		out = append(out, v.Text)
	}
	return out
}

// t0 is the fixture's start: Thursday 1 October 2026, noon UTC.
var t0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// fx is a Service over a fresh store with a fake clock, a fake model, and
// fake lanes.
type fx struct {
	t   *testing.T
	ctx context.Context
	clk *clock.Fake
	db  *store.DB
	r   store.Repos
	m   *fakeModel

	mu      sync.Mutex
	cfg     config.Config
	noModel bool
	idle    bool
	events  []wire.MemoryChanged
	built   map[string]int
	vec     *fakeLane
	mem0    *fakeLane
	sm      *fakeLane
	folder  *fakeFolder

	s *Service
}

func newFx(t *testing.T) *fx {
	t.Helper()
	clk := clock.NewFake(t0)
	db := testutil.NewDBWithClock(t, clk)
	cfg := config.Defaults()
	cfg.Memory.Verify = false
	f := &fx{t: t, ctx: context.Background(), clk: clk, db: db, r: store.NewRepos(db), m: &fakeModel{}, cfg: cfg,
		built: map[string]int{}}
	s, err := New(Options{DB: db, Repos: f.r, Clock: clk, Loc: time.UTC, DataDir: t.TempDir(), CredDir: t.TempDir(),
		Config: f.config, RecallScale: 100,
		LLM: func(config.LLM) (llm.Memory, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.noModel {
				return nil, errors.New("no model is set up")
			}
			return f.m, nil
		},
		Publish: func(name string, data any) {
			require.Equal(t, wire.EventMemoryChanged, name)
			f.mu.Lock()
			defer f.mu.Unlock()
			f.events = append(f.events, data.(wire.MemoryChanged))
		},
		Idle: func(context.Context) bool {
			f.mu.Lock()
			defer f.mu.Unlock()
			return f.idle
		}})
	require.NoError(t, err)
	s.make = factories{
		vec: func(_, _, _ string) (vecLane, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.built["vec"]++
			f.vec = newFakeLane(memory.LaneVec)
			return f.vec, nil
		},
		mem0: func(_, _ string, onStored func(claimID, externalID string)) memory.Projection {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.built["mem0"]++
			f.mem0 = newFakeLane(memory.LaneMem0)
			f.mem0.onStored = onStored
			return f.mem0
		},
		sm: func(_, _ string) docLane {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.built["supermemory"]++
			f.sm = newFakeLane(memory.LaneSupermemory)
			return f.sm
		},
		files: func(dir string) folder {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.built["files"]++
			f.folder = &fakeFolder{dir: dir}
			return f.folder
		},
	}
	t.Cleanup(func() { s.Close() })
	f.s = s
	return f
}

func (f *fx) config() config.Config {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cfg
}

// setMemory changes the [memory] section.
func (f *fx) setMemory(fn func(m *config.Memory)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(&f.cfg.Memory)
}

// allLanes turns every lane on.
func (f *fx) allLanes() {
	f.setMemory(func(m *config.Memory) {
		m.EmbedEndpoint, m.EmbedModel = "http://127.0.0.1:11434", "nomic-embed-text"
		m.Mem0URL, m.SupermemoryURL = "http://127.0.0.1:8888", "http://127.0.0.1:6767"
		m.FilesDir = "/tmp/gwen-memory-test"
	})
	f.s.lanesNow()
}

func (f *fx) setIdle(idle bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.idle = idle
}

// changes is the memory_changed events so far.
func (f *fx) changes() []wire.MemoryChanged {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.events)
}

// turn stores a chat turn ("" starts a thread) and tells the service.
func (f *fx) turn(thread, user, reply string, forget ...string) (string, model.ChatMessage, model.ChatMessage) {
	f.t.Helper()
	now := f.clk.Now()
	th, msgs, err := f.r.Chats.Append(f.ctx, thread, []model.ChatMessage{
		{Role: model.ChatUser, Text: user, At: now}, {Role: model.ChatAssistant, Text: reply, At: now}})
	require.NoError(f.t, err)
	f.s.ChatTurn(f.ctx, memory.ChatTurn{ThreadID: th.ID, User: msgs[0], Reply: msgs[1], Forget: forget})
	return th.ID, msgs[0], msgs[1]
}

// fact is a scripted fact about the user.
func fact(key, value, text, quote string) llm.ExtractedFact {
	return llm.ExtractedFact{Key: key, Subject: memory.SubjectUser, Value: jsonString(value), Text: text, Quote: quote,
		Sensitivity: model.SensitivityPersonal, Confidence: 0.9}
}

func jsonString(s string) []byte {
	return []byte(`"` + strings.ReplaceAll(s, `"`, `\"`) + `"`)
}

// claims are the live claims in statuses (all when none), hidden ones too.
func (f *fx) claims(statuses ...string) []model.Claim {
	f.t.Helper()
	got, err := f.r.Memory.Claims(f.ctx, store.ClaimFilter{Statuses: statuses, IncludeSuppressed: true})
	require.NoError(f.t, err)
	return got
}

// only is the one live claim of key.
func (f *fx) only(key string) model.Claim {
	f.t.Helper()
	var out []model.Claim
	for _, c := range f.claims() {
		if c.Key == key {
			out = append(out, c)
		}
	}
	require.Len(f.t, out, 1, "claims of %s", key)
	return out[0]
}

// jobs counts jobs by state.
func (f *fx) jobs() map[string]int {
	f.t.Helper()
	got, err := f.r.MemoryState.JobCounts(f.ctx)
	require.NoError(f.t, err)
	return got
}

// remember commits words as the user's own, with no model.
func (f *fx) remember(text string) model.Claim {
	f.t.Helper()
	f.mu.Lock()
	no := f.noModel
	f.noModel = true
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.noModel = no
		f.mu.Unlock()
	}()
	claims, err := f.s.Remember(f.ctx, memory.RememberRequest{Text: text, Channel: model.ChannelUI})
	require.NoError(f.t, err)
	require.Len(f.t, claims, 1)
	return claims[0]
}
