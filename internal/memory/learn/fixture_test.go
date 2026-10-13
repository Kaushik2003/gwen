package learn

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/stretchr/testify/require"
)

// fakeModel is a scripted llm.Memory: each job runs its function when set
// and otherwise returns nothing, and every request is recorded.
type fakeModel struct {
	mu         sync.Mutex
	extract    func(llm.ExtractRequest) (llm.ExtractOutput, error)
	verify     func(llm.VerifyRequest) (llm.VerifyOutput, error)
	observe    func(llm.ObserveRequest) (llm.ObserveOutput, error)
	reflect    func(llm.ReflectRequest) (llm.ReflectOutput, error)
	contradict func(llm.ContradictRequest) (llm.ContradictOutput, error)
	insight    func(llm.InsightRequest) (llm.InsightOutput, error)

	extracts    []llm.ExtractRequest
	verifies    []llm.VerifyRequest
	observes    []llm.ObserveRequest
	reflects    []llm.ReflectRequest
	contradicts []llm.ContradictRequest
	insights    []llm.InsightRequest
}

func (m *fakeModel) Name() string { return "fake/model" }

func (m *fakeModel) Extract(_ context.Context, req llm.ExtractRequest) (llm.ExtractOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.extracts = append(m.extracts, req)
	if m.extract == nil {
		return llm.ExtractOutput{Facts: []llm.ExtractedFact{}}, nil
	}
	return m.extract(req)
}

func (m *fakeModel) Verify(_ context.Context, req llm.VerifyRequest) (llm.VerifyOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.verifies = append(m.verifies, req)
	if m.verify == nil {
		out := llm.VerifyOutput{}
		for _, c := range req.Checks {
			out.Verdicts = append(out.Verdicts, llm.Verdict{Ref: c.Ref, Supported: "yes"})
		}
		return out, nil
	}
	return m.verify(req)
}

func (m *fakeModel) Observe(_ context.Context, req llm.ObserveRequest) (llm.ObserveOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observes = append(m.observes, req)
	if m.observe == nil {
		return llm.ObserveOutput{}, nil
	}
	return m.observe(req)
}

func (m *fakeModel) Reflect(_ context.Context, req llm.ReflectRequest) (llm.ReflectOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reflects = append(m.reflects, req)
	if m.reflect == nil {
		return llm.ReflectOutput{Log: req.Log}, nil
	}
	return m.reflect(req)
}

func (m *fakeModel) Contradict(_ context.Context, req llm.ContradictRequest) (llm.ContradictOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.contradicts = append(m.contradicts, req)
	if m.contradict == nil {
		return llm.ContradictOutput{}, nil
	}
	return m.contradict(req)
}

func (m *fakeModel) Insight(_ context.Context, req llm.InsightRequest) (llm.InsightOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.insights = append(m.insights, req)
	if m.insight == nil {
		return llm.InsightOutput{}, nil
	}
	return m.insight(req)
}

// t0 is the fixture's start: Thursday 1 October 2026, noon UTC.
var t0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// fx is a Learner over a fresh store with a fake clock and a fake model.
type fx struct {
	t       *testing.T
	ctx     context.Context
	clk     *clock.Fake
	db      *store.DB
	r       store.Repos
	m       *fakeModel
	noModel bool
	verify  bool
	data    []string
	l       *Learner
}

func newFx(t *testing.T) *fx {
	t.Helper()
	clk := clock.NewFake(t0)
	db := testutil.NewDBWithClock(t, clk)
	f := &fx{t: t, ctx: context.Background(), clk: clk, db: db, r: store.NewRepos(db), m: &fakeModel{}}
	f.l = New(Options{Repo: f.r.Memory, State: f.r.MemoryState, DB: db,
		Model: func() (llm.Memory, error) {
			if f.noModel {
				return nil, errors.New("no model is set up")
			}
			return f.m, nil
		},
		Clock: clk, Loc: time.UTC, Verify: func() bool { return f.verify },
		Data: func(context.Context) ([]string, error) { return f.data, nil }})
	return f
}

// jv is the JSON of a string value.
func jv(s string) json.RawMessage { return jsonString(s) }

// say appends a message to a thread ("" starts one) and returns it.
func (f *fx) say(thread, role, text string, at time.Time) model.ChatMessage {
	f.t.Helper()
	_, msgs, err := f.r.Chats.Append(f.ctx, thread, []model.ChatMessage{{Role: role, Text: text, At: at}})
	require.NoError(f.t, err)
	return msgs[0]
}

// fact is a fact about the user from item i1.
func fact(key, value, text, quote string) llm.ExtractedFact {
	return llm.ExtractedFact{Key: key, Subject: memory.SubjectUser, Value: jv(value), Text: text, Quote: quote,
		Source: "i1", Confidence: 0.9}
}

// from makes a fact a candidate sourced to a chat message.
func from(fct llm.ExtractedFact, msg model.ChatMessage) Candidate {
	speaker := model.SpeakerUser
	if msg.Role == model.ChatAssistant {
		speaker = model.SpeakerGwen
	}
	return Candidate{Fact: fct, SourceKind: model.SourceMessage, SourceID: msg.ID, SourceText: msg.Text,
		Speaker: speaker, Channel: model.ChannelChat, At: msg.At, Extractor: "fake/model#m1"}
}

// propose proposes candidates that must commit.
func (f *fx) propose(cands ...Candidate) Result {
	f.t.Helper()
	res, err := f.l.Propose(f.ctx, cands)
	require.NoError(f.t, err)
	return res
}

// claim returns a claim that must exist.
func (f *fx) claim(id string) model.Claim {
	f.t.Helper()
	c, err := f.r.Memory.Claim(f.ctx, id)
	require.NoError(f.t, err)
	return c
}

// claims returns the live claims in statuses (all when none), suppressed
// ones included, oldest recorded first.
func (f *fx) claims(statuses ...string) []model.Claim {
	f.t.Helper()
	got, err := f.r.Memory.Claims(f.ctx, store.ClaimFilter{Statuses: statuses, IncludeSuppressed: true})
	require.NoError(f.t, err)
	for i, j := 0, len(got)-1; i < j; i, j = i+1, j-1 {
		got[i], got[j] = got[j], got[i]
	}
	return got
}

// only returns the one live claim of key.
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

// actions lists the ledger's actions in order.
func (f *fx) actions() []string {
	f.t.Helper()
	rows, err := f.r.Memory.Ledger(f.ctx, 0, 0)
	require.NoError(f.t, err)
	out := make([]string, len(rows))
	for i, r := range rows {
		var d struct {
			Action string `json:"action"`
		}
		require.NoError(f.t, json.Unmarshal(r.Data, &d))
		out[i] = d.Action
	}
	return out
}

// jobs lists the pending jobs as "kind subject".
func (f *fx) jobs() []string {
	f.t.Helper()
	const qPending = `SELECT kind, subject FROM memory_jobs WHERE state = 'pending' ORDER BY id`
	rows, err := f.db.SQL().QueryContext(f.ctx, qPending)
	require.NoError(f.t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k, s string
		require.NoError(f.t, rows.Scan(&k, &s))
		out = append(out, k+" "+s)
	}
	require.NoError(f.t, rows.Err())
	return out
}

// cursor reads a rowid cursor.
func (f *fx) cursor(key string) int64 {
	f.t.Helper()
	n, err := f.l.getCursor(f.ctx, key)
	require.NoError(f.t, err)
	return n
}

// seed stores a claim as it is, filling what it leaves out: an active
// one-valued owner_said claim about the user said at t0.
func (f *fx) seed(c model.Claim) model.Claim {
	f.t.Helper()
	if c.ID == "" {
		c.ID = model.NewID()
	}
	if c.Cardinality == "" {
		c.Cardinality = model.CardinalityOne
	}
	if c.Status == "" {
		c.Status = model.ClaimActive
	}
	if c.Trust == "" {
		c.Trust = model.TrustOwnerSaid
	}
	if c.Sensitivity == "" {
		c.Sensitivity = model.SensitivityPersonal
	}
	if c.Confidence == 0 {
		c.Confidence = 0.9
	}
	if c.MentionedAt.IsZero() {
		c.MentionedAt = t0
	}
	if c.SourceKind == "" {
		c.SourceKind, c.SourceID = model.SourceMessage, model.NewID()
	}
	if c.Channel == "" {
		c.Channel = model.ChannelChat
	}
	if c.Value == nil {
		c.Value = jv(c.Text)
	}
	c.ContentHash = memory.ClaimHash(c.SubjectID, c.Key, c.Value)
	require.NoError(f.t, f.r.Memory.Apply(f.ctx, store.MemoryChange{Claims: []model.Claim{c}}))
	return f.claim(c.ID)
}

// day is local midnight of a date in the fixture's zone.
func day(s string) time.Time {
	d, err := time.ParseInLocation(time.DateOnly, s, time.UTC)
	if err != nil {
		panic(err)
	}
	return d
}

func tp(t time.Time) *time.Time { return &t }
