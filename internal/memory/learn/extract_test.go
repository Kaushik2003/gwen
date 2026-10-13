package learn

import (
	"errors"
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

// rowids returns the rowids of the messages after rowid, in order.
func (f *fx) rowids(after int64) []int64 {
	f.t.Helper()
	rows, err := f.r.Memory.MessagesAfter(f.ctx, after, 0)
	require.NoError(f.t, err)
	out := make([]int64, len(rows))
	for i, r := range rows {
		out[i] = r.Rowid
	}
	return out
}

func (f *fx) extract(force bool) Result {
	f.t.Helper()
	res, err := f.l.Extract(f.ctx, force)
	require.NoError(f.t, err)
	return res
}

func TestExtractRunsWhenDue(t *testing.T) {
	f := newFx(t)
	msg := f.say("", model.ChatUser, "hello there", t0.Add(-time.Minute))
	items, tokens, err := f.l.Pending(f.ctx)
	require.NoError(t, err)
	require.Equal(t, 1, items)
	require.Equal(t, memory.Tokens("hello there")+tokensPerItem, tokens)
	// Too little, too new, and no cue: nothing is read.
	f.extract(false)
	require.Empty(t, f.m.extracts)
	require.Zero(t, f.cursor(store.KeyMemoryMsgCursor))
	_, err = store.GetLocalTime(f.ctx, f.db.SQL(), store.KeyMemoryLastLearned)
	require.ErrorIs(t, err, store.ErrNotFound)
	// Forced, it is read.
	f.extract(true)
	require.Len(t, f.m.extracts, 1)
	require.Equal(t, f.rowids(0)[0], f.cursor(store.KeyMemoryMsgCursor))
	learned, err := store.GetLocalTime(f.ctx, f.db.SQL(), store.KeyMemoryLastLearned)
	require.NoError(t, err)
	require.Equal(t, t0, learned.UTC())

	// A cue to remember.
	f.say(msg.ThreadID, model.ChatUser, "Don’t forget I’m vegetarian", t0)
	f.extract(false)
	require.Len(t, f.m.extracts, 2)
	// A message waiting over an hour.
	f.say(msg.ThreadID, model.ChatAssistant, "ok", t0.Add(-2*time.Hour))
	f.extract(false)
	require.Len(t, f.m.extracts, 3)
	// Enough words.
	f.say(msg.ThreadID, model.ChatUser, strings.Repeat("word ", 600), t0)
	f.extract(false)
	require.Len(t, f.m.extracts, 3)
	f.say(msg.ThreadID, model.ChatUser, strings.Repeat("word ", 600), t0)
	f.extract(false)
	require.Len(t, f.m.extracts, 4)
	require.Len(t, f.m.extracts[3].Items, 2)
	items, _, err = f.l.Pending(f.ctx)
	require.NoError(t, err)
	require.Zero(t, items)
}

func TestExtractRequest(t *testing.T) {
	f := newFx(t)
	anna := model.Entity{ID: model.NewID(), Kind: memory.EntityPerson, Name: "Anna", Aliases: []string{"Annie"}}
	require.NoError(t, f.r.Memory.Apply(f.ctx, store.MemoryChange{Entities: []model.Entity{anna}}))
	f.seed(model.Claim{Key: "user.employer", Text: "Works at Acme", ValidFrom: tp(day("2026-09-01"))})
	f.seed(model.Claim{Key: "person.relation", SubjectID: &anna.ID, Text: "Anna is their sister"})
	f.seed(model.Claim{Key: "pref.music", Cardinality: model.CardinalityMany, Text: "Likes jazz"})
	f.seed(model.Claim{Key: "user.health", Cardinality: model.CardinalityMany, Text: "Has asthma",
		Sensitivity: model.SensitivitySensitive})
	f.seed(model.Claim{Key: "user.home", Text: "Lives in Rome", Status: model.ClaimCandidate,
		Trust: model.TrustInferred})
	first := f.say("", model.ChatUser, "one", t0.Add(-time.Hour))
	for i, text := range []string{"two", "three", "four", "five"} {
		role := model.ChatUser
		if i%2 == 0 {
			role = model.ChatAssistant
		}
		f.say(first.ThreadID, role, text, t0.Add(-time.Hour+time.Duration(i+1)*time.Minute))
	}
	f.extract(true)
	f.say(first.ThreadID, model.ChatUser, "six", t0.Add(-time.Minute))
	f.extract(true)
	require.Len(t, f.m.extracts, 2)
	req := f.m.extracts[1]
	require.Equal(t, "2026-10-01", req.Today)
	require.Equal(t, "2026-10-01T12:00:00Z", req.Now)
	require.Equal(t, "UTC", req.Timezone)
	require.False(t, req.Explicit)
	require.Len(t, req.Keys, len(memory.Keys))
	require.Equal(t, llm.ExtractKey{Key: "user.name", Label: "Name", Cardinality: "one", About: "user"}, req.Keys[0])
	require.Equal(t, []llm.ExtractEntity{{Ref: "e1", Kind: "person", Name: "Anna", Aliases: []string{"Annie"}}},
		req.Entities)
	// Active claims only, sensitive ones left out; one-valued ones first.
	require.Len(t, req.Known, 3)
	refs := []string{req.Known[0].Ref, req.Known[1].Ref, req.Known[2].Ref}
	require.Equal(t, []string{"k1", "k2", "k3"}, refs)
	for i := range req.Known[:2] {
		req.Known[i].Ref = ""
	}
	require.ElementsMatch(t, []llm.ExtractKnown{
		{Key: "user.employer", Subject: "user", Text: "Works at Acme", Since: "2026-09-01"},
		{Key: "person.relation", Subject: "e1", Text: "Anna is their sister"},
	}, req.Known[:2])
	require.Equal(t, llm.ExtractKnown{Ref: "k3", Key: "pref.music", Subject: "user", Text: "Likes jazz"}, req.Known[2])
	require.Equal(t, []llm.ExtractItem{
		{Ref: "c1", Speaker: "gwen", At: "2026-10-01 11:01", Text: "two"},
		{Ref: "c2", Speaker: "user", At: "2026-10-01 11:02", Text: "three"},
		{Ref: "c3", Speaker: "gwen", At: "2026-10-01 11:03", Text: "four"},
		{Ref: "c4", Speaker: "user", At: "2026-10-01 11:04", Text: "five"},
	}, req.Context)
	require.Equal(t, []llm.ExtractItem{{Ref: "i1", Speaker: "user", At: "2026-10-01 11:59", Text: "six"}}, req.Items)
}

func TestExtractCommitsFactsAndAppliesTheSpeakerRule(t *testing.T) {
	f := newFx(t)
	q := f.say("", model.ChatAssistant, "Should I note that you work at Acme now?", t0.Add(-2*time.Hour))
	a := f.say(q.ThreadID, model.ChatUser, "Yes, I work at Acme now. My sister Anna lives in Rome.",
		t0.Add(-2*time.Hour+time.Minute))
	f.m.extract = func(req llm.ExtractRequest) (llm.ExtractOutput, error) {
		return llm.ExtractOutput{Facts: []llm.ExtractedFact{
			{Key: "user.employer", Subject: "user", Value: jv("Acme"), Text: "Works at Acme", Quote: "work at Acme",
				Source: "i1", Confidence: 0.8},
			{Key: "person.home", Subject: "person:Anna", Value: jv("Rome"), Text: "Anna lives in Rome",
				Quote: "Anna lives in Rome", Source: "i2", Confidence: 0.9},
			{Key: "person.relation", Subject: "person:Anna", Value: jv("sister"), Text: "Anna is their sister",
				Quote: "My sister Anna", Source: "i2", Confidence: 0.9},
		}}, nil
	}
	res := f.extract(false)
	require.Equal(t, 3, res.Learned)
	require.Len(t, res.Claims, 3)
	emp := f.only("user.employer")
	require.Equal(t, a.ID, emp.SourceID, "Gwen's words the user said too are the user's")
	require.Equal(t, model.TrustOwnerSaid, emp.Trust)
	require.Equal(t, model.ClaimActive, emp.Status)
	require.Equal(t, "fake/model#m1", emp.Extractor)
	home := f.only("person.home")
	rel := f.only("person.relation")
	require.Equal(t, *home.SubjectID, *rel.SubjectID, "one new entity for both facts")
	require.Equal(t, f.rowids(0)[1], f.cursor(store.KeyMemoryMsgCursor))
}

func TestExtractBatchesAndCursors(t *testing.T) {
	f := newFx(t)
	first := f.say("", model.ChatUser, "message 1", t0.Add(-3*time.Hour))
	for i := 2; i <= 70; i++ {
		f.say(first.ThreadID, model.ChatUser, fmt.Sprintf("message %d", i), t0.Add(-3*time.Hour+time.Duration(i)*time.Second))
	}
	rows := f.rowids(0)
	calls := 0
	f.m.extract = func(req llm.ExtractRequest) (llm.ExtractOutput, error) {
		calls++
		if calls == 2 {
			return llm.ExtractOutput{}, errors.New("model down")
		}
		return llm.ExtractOutput{}, nil
	}
	_, err := f.l.Extract(f.ctx, false)
	require.ErrorContains(t, err, "model down")
	require.Len(t, f.m.extracts, 2)
	require.Len(t, f.m.extracts[0].Items, 60)
	require.Empty(t, f.m.extracts[0].Context)
	require.Len(t, f.m.extracts[1].Items, 10)
	require.Len(t, f.m.extracts[1].Context, 4)
	require.Equal(t, "message 57", f.m.extracts[1].Context[0].Text)
	require.Equal(t, rows[59], f.cursor(store.KeyMemoryMsgCursor), "the first batch's cursor stays")
	_, err = store.GetLocalTime(f.ctx, f.db.SQL(), store.KeyMemoryLastLearned)
	require.NoError(t, err)
	// The failed batch is read again, and only it.
	f.extract(false)
	require.Len(t, f.m.extracts, 3)
	require.Equal(t, "message 61", f.m.extracts[2].Items[0].Text)
	require.Equal(t, rows[69], f.cursor(store.KeyMemoryMsgCursor))
}

func TestExtractCursorWaitsForEveryThread(t *testing.T) {
	f := newFx(t)
	a1 := f.say("", model.ChatUser, "A one", t0.Add(-2*time.Hour))
	b1 := f.say("", model.ChatUser, "B one", t0.Add(-2*time.Hour))
	f.say(a1.ThreadID, model.ChatUser, "A two", t0.Add(-2*time.Hour))
	f.say(b1.ThreadID, model.ChatUser, "B two", t0.Add(-2*time.Hour))
	rows := f.rowids(0)
	f.m.extract = func(req llm.ExtractRequest) (llm.ExtractOutput, error) {
		if strings.HasPrefix(req.Items[0].Text, "B") {
			return llm.ExtractOutput{}, errors.New("model down")
		}
		return llm.ExtractOutput{}, nil
	}
	_, err := f.l.Extract(f.ctx, true)
	require.Error(t, err)
	require.Equal(t, rows[0], f.cursor(store.KeyMemoryMsgCursor), "B's first message holds the cursor back")
	f.m.extract = nil
	f.extract(true)
	require.Equal(t, rows[3], f.cursor(store.KeyMemoryMsgCursor))
	var read []string
	for _, req := range f.m.extracts[2:] {
		for _, it := range req.Items {
			read = append(read, it.Text)
		}
	}
	require.ElementsMatch(t, []string{"B one", "B two", "A two"}, read)
}

func TestExtractWithoutAModel(t *testing.T) {
	f := newFx(t)
	f.noModel = true
	f.say("", model.ChatUser, "hello", t0)
	_, err := f.l.Extract(f.ctx, true)
	require.ErrorIs(t, err, memory.ErrDisabled)
	require.Zero(t, f.cursor(store.KeyMemoryMsgCursor))
	// With nothing due, there is nothing to say.
	_, err = f.l.Extract(f.ctx, false)
	require.NoError(t, err)
}

func TestExtractReadsNotesAndSkipsTheRest(t *testing.T) {
	f := newFx(t)
	note, err := f.r.Memory.AddEpisode(f.ctx, model.Episode{Kind: model.EpisodeNote, Channel: model.ChannelCLI,
		Speaker: model.SpeakerUser, Body: "I am allergic to peanuts", At: t0.Add(-2 * time.Hour),
		Sensitivity: model.SensitivityPersonal})
	require.NoError(t, err)
	_, err = f.r.MemoryState.AddResource(f.ctx, model.Resource{Title: "Lease", Path: "/tmp/lease.txt",
		ContentHash: "x", AddedAt: t0}, model.Episode{Kind: model.EpisodeResource, Channel: model.ChannelDoc,
		Speaker: model.SpeakerExternal, Body: "The tenant is allergic to cats", At: t0,
		Sensitivity: model.SensitivityPersonal})
	require.NoError(t, err)
	f.noModel = true
	_, err = f.l.Remember(f.ctx, "My bike is blue", model.ChannelUI)
	require.NoError(t, err)
	f.noModel = false
	f.m.extract = func(req llm.ExtractRequest) (llm.ExtractOutput, error) {
		return llm.ExtractOutput{Facts: []llm.ExtractedFact{{Key: "user.health", Subject: "user",
			Value: jv("peanut allergy"), Text: "Is allergic to peanuts", Quote: "allergic to peanuts", Source: "i1",
			Confidence: 0.9}}}, nil
	}
	f.extract(false)
	require.Len(t, f.m.extracts, 1)
	req := f.m.extracts[0]
	require.True(t, req.Explicit)
	require.Equal(t, []llm.ExtractItem{{Ref: "i1", Speaker: "user", At: "2026-10-01 10:00",
		Text: "I am allergic to peanuts"}}, req.Items)
	c := f.only("user.health")
	require.Equal(t, note.ID, c.SourceID)
	require.Equal(t, model.TrustOwnerEdit, c.Trust)
	require.Equal(t, model.ClaimActive, c.Status)
	require.Equal(t, model.SensitivitySensitive, c.Sensitivity)
	require.Equal(t, model.ChannelCLI, c.Channel)
	eps, err := f.r.Memory.EpisodesAfter(f.ctx, 0, 0)
	require.NoError(t, err)
	require.Len(t, eps, 3)
	require.Equal(t, eps[2].Rowid, f.cursor(store.KeyMemoryEpCursor))
}
