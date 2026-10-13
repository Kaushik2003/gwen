package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/stretchr/testify/require"
)

// TestChatTurnLearns follows a turn: the user asks to be remembered, so the
// worker extracts at once; the claim and both messages reach the vector
// lane; the note and the profile card carry the claim; memory_changed is
// published.
func TestChatTurnLearns(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.allLanes()
	f.m.facts = []llm.ExtractedFact{fact("user.employer", "Acme", "Works at Acme", "I work at Acme")}
	thread, user, reply := f.turn("", "Please remember that I work at Acme now.", "Noted, Acme it is.")

	require.Equal(t, map[string]int{model.JobPending: 3}, f.jobs(), "both messages and the chat's log are queued")
	f.s.Tick(f.ctx)
	require.Equal(t, 1, f.m.count(&f.m.extracts), "a remember cue extracts at once")
	c := f.only("user.employer")
	require.Equal(t, model.ClaimActive, c.Status)
	require.Equal(t, model.TrustOwnerSaid, c.Trust)
	require.Equal(t, user.ID, c.SourceID)

	ref := memory.Ref(memory.RefClaim, c.ID)
	require.True(t, f.vec.has(ref), "the claim is embedded")
	require.True(t, f.vec.has(memory.Ref(memory.RefMessage, user.ID)))
	require.True(t, f.vec.has(memory.Ref(memory.RefMessage, reply.ID)))
	require.True(t, f.mem0.has(ref), "the claim is in Mem0")
	b, err := f.r.MemoryState.Backend(f.ctx, "mem0", c.ID)
	require.NoError(t, err)
	require.Equal(t, "m-"+c.ID, b.ExternalID)
	require.Equal(t, model.BackendSynced, b.Status)
	require.Contains(t, f.folder.last(), "Works at Acme", "the folder is written")
	require.Equal(t, map[string]int{model.JobPending: 1, model.JobDone: 5}, f.jobs(),
		"the chat's log waits ten seconds; the rest ran")

	evs := f.changes()
	require.NotEmpty(t, evs)
	require.Equal(t, []string{c.ID}, evs[0].ClaimIDs)

	note, err := f.s.Note(f.ctx, memory.NoteRequest{Text: "where do I work?", ThreadID: thread, RefStart: 3,
		AsOf: f.clk.Now()})
	require.NoError(t, err)
	require.Contains(t, note.Text, "[m3] Works at Acme")
	require.Equal(t, ref, note.Refs["m3"])

	card, err := f.s.Profile(f.ctx, "2026-10-01")
	require.NoError(t, err)
	require.Contains(t, card.Text, "Works at Acme")

	// The chat's log is folded once it is due, and only with a model.
	f.clk.Advance(11 * time.Second)
	f.s.Tick(f.ctx)
	require.Equal(t, map[string]int{model.JobDone: 6}, f.jobs())
}

// TestChatTurnForget hides the claims a reply was asked to stop bringing up.
func TestChatTurnForget(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	c := f.remember("My sister Anna lives in Porto")
	f.turn("", "Stop bringing up my sister.", "I won't bring it up again.", memory.Ref(memory.RefClaim, c.ID))
	got, err := f.r.Memory.Claim(f.ctx, c.ID)
	require.NoError(t, err)
	require.True(t, got.Suppressed)
	evs := f.changes()
	require.Equal(t, []string{c.ID}, evs[len(evs)-1].ClaimIDs)
}

// TestWorkerRunsInBackground: Run requeues jobs left running, ticks at
// start, and wakes for a chat turn.
func TestWorkerRunsInBackground(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.allLanes()
	require.NoError(t, f.r.MemoryState.Enqueue(f.ctx, store.NewJob{Kind: model.JobProfile, Subject: "2026-10-01"}))
	_, err := f.r.MemoryState.NextJob(f.ctx, f.clk.Now())
	require.NoError(t, err)
	require.Equal(t, map[string]int{model.JobRunning: 1}, f.jobs(), "a job a crash left running")

	ctx, cancel := context.WithCancel(f.ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		f.s.Run(ctx)
	}()
	require.Eventually(t, func() bool { return f.jobs()[model.JobDone] == 1 }, 5*time.Second, 10*time.Millisecond)

	f.m.facts = []llm.ExtractedFact{fact("user.home", "Lisbon", "Lives in Lisbon", "I live in Lisbon")}
	f.turn("", "Remember: I live in Lisbon.", "Lisbon, got it.")
	require.Eventually(t, func() bool {
		claims, err := f.r.Memory.Claims(f.ctx, store.ClaimFilter{KeyPrefix: "user.home"})
		return err == nil && len(claims) == 1
	}, 5*time.Second, 10*time.Millisecond)
	cancel()
	<-done
}

// TestJobBackoff retries a failing job after 1, 5, and 30 minutes and 2
// hours, and fails it for good at the sixth attempt.
func TestJobBackoff(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.allLanes()
	f.vec.set(func(l *fakeLane) { l.upsertErr = errors.New("embedding server down") })
	c := f.remember("I play the cello")
	f.s.Tick(f.ctx)
	job := func() model.MemoryJob {
		t.Helper()
		var j model.MemoryJob
		err := f.db.SQL().QueryRowContext(f.ctx, `SELECT state, attempts, run_after, last_error FROM memory_jobs
			WHERE kind = 'project' AND subject = ?`, memory.Ref(memory.RefClaim, c.ID)).Scan(&j.State, &j.Attempts,
			new(int64), &j.LastError)
		require.NoError(t, err)
		return j
	}
	require.Equal(t, model.JobPending, job().State)
	require.Equal(t, 1, job().Attempts)
	require.Contains(t, job().LastError, "embedding server down")
	for i, wait := range []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 2 * time.Hour} {
		f.clk.Advance(wait - time.Second)
		f.s.Tick(f.ctx)
		require.Equal(t, i+1, job().Attempts, "not before its backoff")
		f.clk.Advance(time.Second)
		f.s.Tick(f.ctx)
		require.Equal(t, i+2, job().Attempts)
	}
	require.Equal(t, model.JobFailed, job().State, "the sixth failure is final")
}

// TestExtractBackoff waits 5 and then 30 minutes after model errors, says
// why in the status, and starts afresh after a success.
func TestExtractBackoff(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.m.extractErr = errors.New("rate limited")
	f.turn("", "Remember that I work at Acme.", "Sure.")
	f.s.Tick(f.ctx)
	require.Equal(t, 1, f.m.count(&f.m.extracts))
	st := f.s.Status(f.ctx)
	require.True(t, st.Learning)
	require.Equal(t, laneGwen, st.Lanes[0].Lane)
	require.Contains(t, st.Lanes[0].Detail, "rate limited")

	f.s.Wake(true)
	f.clk.Advance(4 * time.Minute)
	f.s.Tick(f.ctx)
	require.Equal(t, 1, f.m.count(&f.m.extracts), "backing off")
	f.clk.Advance(time.Minute)
	f.s.Tick(f.ctx)
	require.Equal(t, 2, f.m.count(&f.m.extracts))
	f.clk.Advance(29 * time.Minute)
	f.s.Tick(f.ctx)
	require.Equal(t, 2, f.m.count(&f.m.extracts), "the second wait is 30 minutes")

	f.m.mu.Lock()
	f.m.extractErr = nil
	f.m.facts = []llm.ExtractedFact{fact("user.employer", "Acme", "Works at Acme", "I work at Acme")}
	f.m.mu.Unlock()
	f.clk.Advance(time.Minute)
	f.s.Tick(f.ctx)
	require.Equal(t, 3, f.m.count(&f.m.extracts))
	f.only("user.employer")
	st = f.s.Status(f.ctx)
	require.NotContains(t, st.Lanes[0].Detail, "failed")
}

// TestSleepOncePerDay consolidates after 10 idle minutes, once a day, and
// in the evening even while working.
func TestSleepOncePerDay(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	slept := func() int { return f.m.count(&f.m.contradicts) }
	f.remember("I like jazz")
	f.remember("I like cycling")

	f.setIdle(true)
	f.s.Tick(f.ctx)
	f.clk.Advance(9 * time.Minute)
	f.s.Tick(f.ctx)
	require.Zero(t, slept(), "idle for 9 minutes")
	f.clk.Advance(time.Minute)
	f.s.Tick(f.ctx)
	require.Equal(t, 1, slept(), "idle for 10 minutes")
	last, err := store.GetLocalTime(f.ctx, f.db.SQL(), store.KeyMemoryLastSlept)
	require.NoError(t, err)
	require.True(t, f.clk.Now().Equal(last))
	f.clk.Advance(time.Hour)
	f.s.Tick(f.ctx)
	require.Equal(t, 1, slept(), "once a day")

	// The next day, working: not until 22:00.
	f.setIdle(false)
	f.clk.Set(time.Date(2026, 10, 2, 21, 59, 0, 0, time.UTC))
	f.s.Tick(f.ctx)
	require.Equal(t, 1, slept())
	f.clk.Advance(time.Minute)
	f.s.Tick(f.ctx)
	require.Equal(t, 2, slept(), "the evening")
	card, err := f.r.MemoryState.LatestProfile(f.ctx)
	require.NoError(t, err)
	require.Equal(t, "2026-10-02", card.Day)
}

// TestSleepReport runs consolidation on demand: duplicates merged, a
// contradiction opened, lanes reconciled.
func TestSleepReport(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.allLanes()
	// Two claims that say the same thing in other values, and one against them.
	f.m.facts = []llm.ExtractedFact{fact("pref.music", "jazz", "Likes jazz", "I like jazz"),
		fact("pref.music", "jazz music", "Likes jazz", "I like jazz music")}
	got, err := f.s.Remember(f.ctx, memory.RememberRequest{Text: "I like jazz"})
	require.NoError(t, err)
	a := got[0]
	f.clk.Advance(time.Minute)
	_, err = f.s.Remember(f.ctx, memory.RememberRequest{Text: "I like jazz music"})
	require.NoError(t, err)
	require.Len(t, f.claims(model.ClaimActive), 2)
	f.clk.Advance(time.Minute)
	b := f.remember("I dislike jazz")
	f.m.contradict = func(req llm.ContradictRequest) (llm.ContradictOutput, error) {
		refs := map[string]string{}
		for _, c := range req.Claims {
			refs[c.Text] = c.Ref
		}
		return llm.ContradictOutput{Conflicts: []llm.Contradiction{{A: refs[a.Text], B: refs[b.Text],
			Reason: "both cannot hold"}}}, nil
	}
	gone := memory.Ref(memory.RefClaim, "00000000-0000-4000-8000-00000000dead")
	f.vec.set(func(l *fakeLane) { l.items[gone] = memory.Record{ID: gone} })

	rep, err := f.s.Sleep(f.ctx)
	require.NoError(t, err)
	require.Equal(t, 1, rep.Merged)
	require.Equal(t, 1, rep.Conflicts)
	require.True(t, rep.Profile)
	require.GreaterOrEqual(t, rep.Reconciled, 1)
	require.False(t, f.vec.has(gone), "an item whose row is gone is dropped")
	cs, err := f.s.Conflicts(f.ctx, model.ConflictOpen)
	require.NoError(t, err)
	require.Len(t, cs, 1)
	require.Equal(t, "Likes jazz", cs[0].A.Text)
	require.True(t, cs[0].A.Disputed)
	evs := f.changes()
	require.True(t, evs[len(evs)-1].Conflicts)
}

// TestDisabled: with memory off, methods answer ErrDisabled, the note is
// empty, chat turns do nothing, and the worker idles.
func TestDisabled(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.setMemory(func(m *config.Memory) { m.Enabled = false })
	note, err := f.s.Note(f.ctx, memory.NoteRequest{Text: "where do I work?"})
	require.NoError(t, err)
	require.Empty(t, note.Text)
	f.turn("", "Remember that I work at Acme.", "Sure.")
	require.Empty(t, f.jobs())
	f.s.Tick(f.ctx)
	require.Zero(t, f.m.count(&f.m.extracts))

	_, err = f.s.Remember(f.ctx, memory.RememberRequest{Text: "I like tea"})
	require.ErrorIs(t, err, memory.ErrDisabled)
	_, err = f.s.Claims(f.ctx, store.ClaimFilter{})
	require.ErrorIs(t, err, memory.ErrDisabled)
	_, err = f.s.Profile(f.ctx, "2026-10-01")
	require.ErrorIs(t, err, memory.ErrDisabled)
	_, err = f.s.Forget(f.ctx, "x")
	require.ErrorIs(t, err, memory.ErrDisabled)
	_, err = f.s.Sleep(f.ctx)
	require.ErrorIs(t, err, memory.ErrDisabled)
	_, ok, err := f.s.Observation(f.ctx, "x")
	require.NoError(t, err)
	require.False(t, ok)
	st := f.s.Status(f.ctx)
	require.False(t, st.Enabled)
	require.False(t, st.Learning)
	require.Empty(t, st.Lanes)
}

// TestLearningPaused: with learning off or no model, the status says why
// and words to remember are kept as typed.
func TestLearningPaused(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.setMemory(func(m *config.Memory) { m.Learn = config.LearnOff })
	st := f.s.Status(f.ctx)
	require.True(t, st.Enabled)
	require.False(t, st.Learning)
	require.Equal(t, "learning paused: learning from chats is off", st.Lanes[0].Detail)
	claims, err := f.s.Remember(f.ctx, memory.RememberRequest{Text: "I take my coffee black"})
	require.NoError(t, err)
	require.Equal(t, "user.fact", claims[0].Key)
	require.Zero(t, f.m.count(&f.m.extracts))

	f.setMemory(func(m *config.Memory) {
		m.Learn, m.LocalEndpoint, m.LocalModel = config.LearnLocal, "http://127.0.0.1:8080/v1", "qwen"
	})
	var asked config.LLM
	f.s.o.LLM = func(c config.LLM) (llm.Memory, error) { asked = c; return f.m, nil }
	ok, detail := f.s.learning()
	require.True(t, ok)
	require.Equal(t, "learning with fake/model", detail)
	require.Equal(t, config.LLM{Provider: config.ProviderOpenAICompatible, Endpoint: "http://127.0.0.1:8080/v1",
		Model: "qwen", Timeout: 2 * time.Minute}, asked)
}

// TestLanesFollowConfig builds a lane when its settings change, closes the
// vector index it replaces, and keeps vendor lanes off the network while
// privacy is local.
func TestLanesFollowConfig(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	ls := f.s.lanesNow()
	require.Nil(t, ls.vec)
	require.Nil(t, ls.mem0)
	require.Nil(t, ls.files)

	f.allLanes()
	f.s.lanesNow()
	require.Equal(t, map[string]int{"vec": 1, "mem0": 1, "supermemory": 1, "files": 1}, f.built, "built once")
	old := f.vec
	f.setMemory(func(m *config.Memory) { m.EmbedModel = "mxbai-embed-large" })
	f.s.lanesNow()
	require.Equal(t, 2, f.built["vec"])
	require.True(t, old.closed, "the old index is closed")

	f.setMemory(func(m *config.Memory) { m.Mem0URL = "https://mem0.example.com" })
	ls = f.s.lanesNow()
	require.Nil(t, ls.mem0, "privacy local keeps memory on this device")
	st := f.s.Status(f.ctx)
	require.Contains(t, laneOf(st, memory.LaneMem0).Detail, "privacy keeps memory on this device")
	f.setMemory(func(m *config.Memory) { m.Privacy = config.PrivacyCloudOK })
	ls = f.s.lanesNow()
	require.NotNil(t, ls.mem0)

	f.setMemory(func(m *config.Memory) { m.FilesDir = "~/Gwen/memory" })
	ls = f.s.lanesNow()
	require.True(t, strings.HasSuffix(ls.files.Dir(), "/Gwen/memory"))
	require.True(t, strings.HasPrefix(ls.files.Dir(), "/"))

	f.vec.set(func(l *fakeLane) { l.healthErr = errors.New("connection refused") })
	require.True(t, laneOf(f.s.Status(f.ctx), memory.LaneVec).OK, "health is cached for a minute")
	f.clk.Advance(time.Minute)
	st = f.s.Status(f.ctx)
	vs := laneOf(st, memory.LaneVec)
	require.True(t, vs.Enabled)
	require.False(t, vs.OK)
	require.Equal(t, "connection refused", vs.Detail)
	f.vec.set(func(l *fakeLane) { l.healthErr = nil })
	require.False(t, laneOf(f.s.Status(f.ctx), memory.LaneVec).OK, "health is cached for a minute")
	f.clk.Advance(time.Minute)
	require.True(t, laneOf(f.s.Status(f.ctx), memory.LaneVec).OK)

	f.setMemory(func(m *config.Memory) { m.EmbedEndpoint = "" })
	ls = f.s.lanesNow()
	require.Nil(t, ls.vec)
	require.False(t, laneOf(f.s.Status(f.ctx), memory.LaneVec).Enabled)
}

func laneOf(st memory.Status, name memory.LaneName) memory.LaneStatus {
	for _, l := range st.Lanes {
		if l.Lane == name {
			return l
		}
	}
	return memory.LaneStatus{}
}

// TestStatus counts claims, jobs, and the ledger, and reports the lanes.
func TestStatus(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.allLanes()
	f.remember("I like jazz")
	f.s.Tick(f.ctx)
	st := f.s.Status(f.ctx)
	require.True(t, st.Enabled)
	require.True(t, st.Learning)
	require.Equal(t, map[string]int{model.ClaimActive: 1}, st.Claims)
	require.Equal(t, map[string]int{model.JobDone: 3}, st.Jobs)
	require.True(t, st.Ledger.Verified)
	require.Positive(t, st.Ledger.Rows)
	require.NotNil(t, st.Profile)
	var names []memory.LaneName
	for _, l := range st.Lanes {
		names = append(names, l.Lane)
	}
	require.Equal(t, []memory.LaneName{laneGwen, memory.LaneVec, memory.LaneMem0, memory.LaneSupermemory,
		memory.LaneFiles}, names)
	require.Equal(t, 1, st.Lanes[0].Items)
	require.Equal(t, 1, laneOf(st, memory.LaneVec).Items)
	require.Equal(t, 1, laneOf(st, memory.LaneMem0).Items)
	require.Equal(t, -1, laneOf(st, memory.LaneFiles).Items)

	f.sm.set(func(l *fakeLane) {
		for i := range smWarn {
			l.items[string(rune(i))] = memory.Record{}
		}
	})
	require.Contains(t, laneOf(f.s.Status(f.ctx), memory.LaneSupermemory).Detail, "8000 of the 10000 documents")
}

// TestProfileFresh compiles the card again only after claims change or the
// day turns.
func TestProfileFresh(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	card, err := f.s.Profile(f.ctx, "2026-10-01")
	require.NoError(t, err)
	require.Empty(t, card.Text)
	again, err := f.s.Profile(f.ctx, "2026-10-01")
	require.NoError(t, err)
	require.Equal(t, card.ID, again.ID, "the stored card is reused")

	f.remember("I like jazz")
	card, err = f.s.Profile(f.ctx, "2026-10-01")
	require.NoError(t, err)
	require.Contains(t, card.Text, "I like jazz")
	next, err := f.s.Profile(f.ctx, "2026-10-02")
	require.NoError(t, err)
	require.NotEqual(t, card.ID, next.ID)
	require.Equal(t, "2026-10-02", next.Day)
}
