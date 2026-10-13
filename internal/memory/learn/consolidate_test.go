package learn

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/stretchr/testify/require"
)

// chat adds n messages to a thread ("" starts one), alternating user and
// assistant, a minute apart from start, and returns the thread and them.
func (f *fx) chat(thread string, n int, start time.Time, text func(i int) string) (string, []model.ChatMessage) {
	f.t.Helper()
	var out []model.ChatMessage
	for i := range n {
		role := model.ChatUser
		if i%2 == 1 {
			role = model.ChatAssistant
		}
		m := f.say(thread, role, text(i), start.Add(time.Duration(i)*time.Minute))
		thread = m.ThreadID
		out = append(out, m)
	}
	return thread, out
}

func (f *fx) observe(thread string) bool {
	f.t.Helper()
	ok, err := f.l.Observe(f.ctx, thread)
	require.NoError(f.t, err)
	return ok
}

func TestObserveFoldsOldMessages(t *testing.T) {
	f := newFx(t)
	short := func(i int) string { return fmt.Sprintf("message %d", i+1) }
	thread, msgs := f.chat("", 24, t0.Add(-5*time.Hour), short)
	require.False(t, f.observe(thread), "24 messages stay verbatim")
	require.Empty(t, f.m.observes)

	_, more := f.chat(thread, 6, t0.Add(-time.Hour), short)
	msgs = append(msgs, more...)
	f.m.observe = func(req llm.ObserveRequest) (llm.ObserveOutput, error) {
		return llm.ObserveOutput{Observations: []llm.Observation{
			{Date: "2026-10-01", Priority: "high", Text: "They decided to move to Rome"},
			{Date: "2026-10-01", Priority: "low", Text: "Small talk"}}}, nil
	}
	require.True(t, f.observe(thread))
	require.Len(t, f.m.observes, 1)
	req := f.m.observes[0]
	require.Equal(t, "2026-10-01", req.Today)
	require.Empty(t, req.Log)
	require.Len(t, req.Messages, 22)
	require.Equal(t, llm.ObserveMessage{Role: "user", At: "2026-10-01 07:00", Text: "message 1"}, req.Messages[0])
	require.Equal(t, "assistant", req.Messages[1].Role)
	o, err := f.r.MemoryState.Observation(f.ctx, thread)
	require.NoError(t, err)
	require.Equal(t, msgs[21].ID, o.ThroughID)
	require.Equal(t, msgs[21].At, o.ThroughAt)
	require.Equal(t, "- 2026-10-01 [high] They decided to move to Rome\n- 2026-10-01 [low] Small talk", o.Log)
	require.Zero(t, o.Reflections)
	// Eight are left after the log: nothing to fold.
	require.False(t, f.observe(thread))
	require.Len(t, f.m.observes, 1)

	// Many words fold too, even with few messages.
	big := func(int) string { return strings.Repeat("long ", 2500) }
	thread2, _ := f.chat("", 10, t0.Add(-time.Hour), big)
	f.m.observe = nil
	require.True(t, f.observe(thread2))
	require.Len(t, f.m.observes[1].Messages, 2)
	require.Len(t, f.m.observes[1].Messages[0].Text, len(memory.Cut(big(0), itemChars)))

	// A deleted thread is not an error.
	_, _ = f.chat(thread, 30, t0, short)
	require.NoError(t, f.r.Chats.Delete(f.ctx, thread))
	require.False(t, f.observe(thread))
}

func TestObserveReflectsALongLog(t *testing.T) {
	f := newFx(t)
	thread, _ := f.chat("", 25, t0.Add(-5*time.Hour), func(i int) string { return fmt.Sprintf("m%d", i) })
	line := "- 2026-09-30 [medium] " + strings.Repeat("x", 77)
	var lines []string
	for range 120 {
		lines = append(lines, line)
	}
	old := strings.Join(lines, "\n") // 11,999 characters
	require.Len(t, old, 11999)
	first, err := f.r.Memory.MessagesBefore(f.ctx, thread, t0.Add(-5*time.Hour+30*time.Second), 1)
	require.NoError(t, err)
	require.NoError(t, f.r.MemoryState.SetObservation(f.ctx, model.ChatObservation{ThreadID: thread,
		ThroughID: first[0].ID, ThroughAt: first[0].At, Log: old}))
	// 24 messages after the log: not yet.
	require.False(t, f.observe(thread))
	f.chat(thread, 1, t0, func(int) string { return "one more" })
	f.m.observe = func(llm.ObserveRequest) (llm.ObserveOutput, error) {
		return llm.ObserveOutput{Observations: []llm.Observation{{Date: "2026-10-01", Priority: "high",
			Text: "They start a new job on Monday"}}}, nil
	}
	f.m.reflect = func(req llm.ReflectRequest) (llm.ReflectOutput, error) {
		require.True(t, strings.HasSuffix(req.Log, "\n- 2026-10-01 [high] They start a new job on Monday"))
		return llm.ReflectOutput{Log: "- 2026-10-01 [high] Starts a new job on Monday"}, nil
	}
	require.True(t, f.observe(thread))
	require.Len(t, f.m.reflects, 1)
	o, err := f.r.MemoryState.Observation(f.ctx, thread)
	require.NoError(t, err)
	require.Equal(t, "- 2026-10-01 [high] Starts a new job on Monday", o.Log)
	require.Equal(t, 1, o.Reflections)
	// A reflection that is still too long is capped, oldest lines first.
	require.Equal(t, "b\nc", capLog("a\nb\nc", 3))
}

func TestObserveNeedsAModel(t *testing.T) {
	f := newFx(t)
	thread, _ := f.chat("", 30, t0.Add(-time.Hour), func(i int) string { return "hi" })
	f.noModel = true
	_, err := f.l.Observe(f.ctx, thread)
	require.ErrorIs(t, err, memory.ErrDisabled)
	f.noModel = false
	ok, err := f.l.Observe(f.ctx, model.NewID())
	require.NoError(t, err)
	require.False(t, ok)
}

func TestContradict(t *testing.T) {
	f := newFx(t)
	acme := f.seed(model.Claim{Key: "user.employer", Text: "Works at Acme"})
	f.clk.Advance(time.Second)
	mit := f.seed(model.Claim{Key: "user.school", Text: "Studies full time at MIT"})
	f.clk.Advance(time.Second)
	rome := f.seed(model.Claim{Key: "user.home", Text: "Lives in Rome", ValidTo: tp(day("2026-12-01"))})
	refOf := func(req llm.ContradictRequest, text string) string {
		for _, c := range req.Claims {
			if c.Text == text {
				return c.Ref
			}
		}
		return ""
	}
	f.m.contradict = func(req llm.ContradictRequest) (llm.ContradictOutput, error) {
		require.Len(t, req.Claims, 3)
		require.Equal(t, llm.ContradictItem{Ref: "k1", Key: "user.home", Subject: "user", Text: "Lives in Rome",
			Until: "2026-11-30"}, req.Claims[0])
		// Rome stops being active before the conflicts are opened.
		_, err := f.l.Reject(f.ctx, rome.ID)
		require.NoError(t, err)
		return llm.ContradictOutput{Conflicts: []llm.Contradiction{
			{A: refOf(req, "Works at Acme"), B: refOf(req, "Studies full time at MIT"), Reason: "full time at both"},
			{A: refOf(req, "Works at Acme"), B: refOf(req, "Lives in Rome"), Reason: "Acme is in Oslo"},
		}}, nil
	}
	n, err := f.l.Contradict(f.ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	open, err := f.r.Memory.Conflicts(f.ctx, model.ConflictOpen)
	require.NoError(t, err)
	require.Len(t, open, 1)
	require.Equal(t, acme.ID, open[0].ClaimA)
	require.Equal(t, mit.ID, open[0].ClaimB)
	require.Equal(t, "full time at both", open[0].Reason)
	// A pair once in a conflict, in any status, is not opened again.
	_, err = f.l.Resolve(f.ctx, open[0].ID, model.ResolveBoth)
	require.NoError(t, err)
	f.m.contradict = func(req llm.ContradictRequest) (llm.ContradictOutput, error) {
		return llm.ContradictOutput{Conflicts: []llm.Contradiction{
			{A: refOf(req, "Studies full time at MIT"), B: refOf(req, "Works at Acme")}}}, nil
	}
	n, err = f.l.Contradict(f.ctx)
	require.NoError(t, err)
	require.Zero(t, n)
	// With fewer than two claims, the model is not asked.
	g := newFx(t)
	g.seed(model.Claim{Key: "user.home", Text: "Lives in Rome"})
	n, err = g.l.Contradict(g.ctx)
	require.NoError(t, err)
	require.Zero(t, n)
	require.Empty(t, g.m.contradicts)
}

func TestDedupe(t *testing.T) {
	f := newFx(t)
	keep := f.seed(model.Claim{Key: "user.fact", Cardinality: model.CardinalityMany, Value: jv("blue bike"),
		Text: "Has a blue bike"})
	f.clk.Advance(time.Minute)
	dup := f.seed(model.Claim{Key: "user.fact", Cardinality: model.CardinalityMany, Value: jv("a blue bike"),
		Text: "has a  Blue bike"})
	evidence := model.NewID()
	require.NoError(t, f.r.Memory.Apply(f.ctx, store.MemoryChange{Links: []model.MemoryLink{{ID: model.NewID(),
		FromKind: "claim", FromID: dup.ID, ToKind: "message", ToID: evidence, Relation: "evidence"}}}))
	f.clk.Advance(time.Minute)
	other := f.seed(model.Claim{Key: "user.fact", Cardinality: model.CardinalityMany, Value: jv("a red car"),
		Text: "Has a red car"})
	n, err := f.l.Dedupe(f.ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	got := f.claim(dup.ID)
	require.Equal(t, model.ClaimSuperseded, got.Status)
	require.Equal(t, keep.ID, *got.SupersededBy)
	require.Equal(t, model.ClaimActive, f.claim(keep.ID).Status)
	require.Equal(t, model.ClaimActive, f.claim(other.ID).Status)
	links, err := f.r.Memory.LinksFrom(f.ctx, keep.ID)
	require.NoError(t, err)
	var to []string
	for _, l := range links {
		require.Equal(t, "evidence", l.Relation)
		to = append(to, l.ToID)
	}
	require.ElementsMatch(t, []string{dup.SourceID, evidence}, to)
	n, err = f.l.Dedupe(f.ctx)
	require.NoError(t, err)
	require.Zero(t, n)
}

func TestInsights(t *testing.T) {
	f := newFx(t)
	f.seed(model.Claim{Key: "user.role", Text: "Works as a backend intern"})
	n, err := f.l.Insights(f.ctx)
	require.NoError(t, err)
	require.Zero(t, n)
	require.Empty(t, f.m.insights, "no data, no question")

	f.data = []string{"Worked over target 6 of the last 7 days", "Most focus time 08:00 to 11:00"}
	f.m.insight = func(req llm.InsightRequest) (llm.InsightOutput, error) {
		return llm.InsightOutput{Insights: []llm.Insight{{Key: "pref.work", Value: jv("mornings"),
			Text: "Works best in the morning", Basis: "Most focus time 08:00 to 11:00", Confidence: 0.7}}}, nil
	}
	n, err = f.l.Insights(f.ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, llm.InsightRequest{Today: "2026-10-01", Known: []string{"Works as a backend intern"},
		Data: f.data}, f.m.insights[0])
	c := f.only("pref.work")
	require.Equal(t, model.ClaimCandidate, c.Status)
	require.Equal(t, model.TrustInferred, c.Trust)
	require.Equal(t, model.SourceData, c.SourceKind)
	require.Equal(t, "insight:2026-10-01", c.SourceID)
	require.Equal(t, "Most focus time 08:00 to 11:00", c.Span)
	require.Equal(t, channelData, c.Channel)

	// At most once a week.
	f.clk.Advance(6 * 24 * time.Hour)
	n, err = f.l.Insights(f.ctx)
	require.NoError(t, err)
	require.Zero(t, n)
	require.Len(t, f.m.insights, 1)

	// A rejected insight is not proposed again, on any day.
	_, err = f.l.Reject(f.ctx, c.ID)
	require.NoError(t, err)
	f.clk.Advance(2 * 24 * time.Hour)
	n, err = f.l.Insights(f.ctx)
	require.NoError(t, err)
	require.Zero(t, n)
	require.Len(t, f.m.insights, 2)
	last, err := store.GetLocalTime(f.ctx, f.db.SQL(), store.KeyMemoryLastInsight)
	require.NoError(t, err)
	require.Equal(t, t0.Add(8*24*time.Hour), last.UTC())
}
