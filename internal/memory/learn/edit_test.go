package learn

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/stretchr/testify/require"
)

func TestRememberWithAModel(t *testing.T) {
	f := newFx(t)
	f.verify = true
	f.m.extract = func(req llm.ExtractRequest) (llm.ExtractOutput, error) {
		return llm.ExtractOutput{Facts: []llm.ExtractedFact{
			{Key: "user.employer", Subject: "user", Value: jv("Acme"), Text: "Works at Acme", Quote: "work at Acme",
				Source: "i1", Confidence: 0.7},
			{Key: "pref.food", Subject: "user", Value: jv("sushi"), Text: "Likes sushi", Quote: "love sushi",
				Source: "i1", Confidence: 0.7},
		}}, nil
	}
	claims, err := f.l.Remember(f.ctx, "  I work at Acme and love sushi ", model.ChannelCLI)
	require.NoError(t, err)
	require.Len(t, claims, 2)
	require.Empty(t, f.m.verifies, "the user's own words are not checked")
	req := f.m.extracts[0]
	require.True(t, req.Explicit)
	require.Equal(t, []llm.ExtractItem{{Ref: "i1", Speaker: "user", At: "2026-10-01 12:00",
		Text: "I work at Acme and love sushi"}}, req.Items)
	ep, err := f.r.Memory.Episode(f.ctx, claims[0].SourceID)
	require.NoError(t, err)
	require.Equal(t, model.EpisodeNote, ep.Kind)
	require.Equal(t, model.ChannelCLI, ep.Channel)
	require.Equal(t, "I work at Acme and love sushi", ep.Body)
	for _, c := range claims {
		require.Equal(t, model.TrustOwnerEdit, c.Trust)
		require.Equal(t, model.ClaimActive, c.Status)
		require.Equal(t, model.SourceEpisode, c.SourceKind)
		require.Equal(t, model.ChannelCLI, c.Channel)
		require.Equal(t, "fake/model#m1", c.Extractor)
		require.Equal(t, 1.0, c.Confidence)
	}
	// Words that only corroborate a candidate return it.
	gwen := f.say("", model.ChatAssistant, "You live in Oslo, right?", t0)
	f.propose(from(fact("user.home", "Oslo", "Lives in Oslo", "live in Oslo"), gwen))
	cand := f.only("user.home")
	f.m.extract = func(req llm.ExtractRequest) (llm.ExtractOutput, error) {
		return llm.ExtractOutput{Facts: []llm.ExtractedFact{{Key: "user.home", Subject: "user", Value: jv("Oslo"),
			Text: "Lives in Oslo", Quote: "I live in Oslo", Source: "i1"}}}, nil
	}
	claims, err = f.l.Remember(f.ctx, "I live in Oslo", "")
	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.Equal(t, cand.ID, claims[0].ID)
	require.Equal(t, model.ClaimActive, claims[0].Status)
}

func TestRememberWithoutAModel(t *testing.T) {
	long := strings.Repeat("I keep my spare keys with the neighbour downstairs. ", 10)
	for name, setup := range map[string]func(f *fx){
		"no model": func(f *fx) { f.noModel = true },
		"no facts": func(f *fx) {},
		"a model error": func(f *fx) {
			f.m.extract = func(llm.ExtractRequest) (llm.ExtractOutput, error) { return llm.ExtractOutput{}, errors.New("down") }
		},
		"facts dropped": func(f *fx) {
			f.m.extract = func(llm.ExtractRequest) (llm.ExtractOutput, error) {
				return llm.ExtractOutput{Facts: []llm.ExtractedFact{fact("user.fact", "x", "Made up", "not said")}}, nil
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFx(t)
			setup(f)
			claims, err := f.l.Remember(f.ctx, long, model.ChannelUI)
			require.NoError(t, err)
			require.Len(t, claims, 1)
			c := claims[0]
			cut := memory.Cut(long, rememberChars)
			require.Equal(t, "user.fact", c.Key)
			require.Equal(t, cut, c.Text)
			require.JSONEq(t, string(jv(cut)), string(c.Value))
			require.Equal(t, strings.TrimSuffix(cut, "…"), c.Span)
			require.Equal(t, model.TrustOwnerEdit, c.Trust)
			require.Equal(t, model.ClaimActive, c.Status)
			require.Equal(t, extractorUser, c.Extractor)
		})
	}
}

func TestRememberRefuses(t *testing.T) {
	f := newFx(t)
	for text, channel := range map[string]string{
		"my card number is 4111 1111 1111 1111": model.ChannelUI,
		"   ":                                   model.ChannelUI,
		"I like tea":                            model.ChannelChat,
	} {
		_, err := f.l.Remember(f.ctx, text, channel)
		require.ErrorIs(t, err, store.ErrInvalid, text)
	}
	eps, err := f.r.Memory.EpisodesAfter(f.ctx, 0, 0)
	require.NoError(t, err)
	require.Empty(t, eps, "a refused secret is not stored")
}

func TestEditAValue(t *testing.T) {
	f := newFx(t)
	old := f.seed(model.Claim{Key: "user.employer", Value: jv("Acme"), Text: "Works at ACME as an intern",
		ValidFrom: tp(day("2026-09-01"))})
	f.clk.Advance(time.Hour)
	v := "Globex"
	c, err := f.l.Edit(f.ctx, old.ID, memory.EditRequest{Value: &v})
	require.NoError(t, err)
	require.Equal(t, "Works at Globex as an intern", c.Text)
	require.JSONEq(t, `"Globex"`, string(c.Value))
	require.Equal(t, model.TrustOwnerEdit, c.Trust)
	require.Equal(t, model.ClaimActive, c.Status)
	require.Equal(t, model.CardinalityOne, c.Cardinality)
	require.Equal(t, extractorUser, c.Extractor)
	require.Equal(t, model.ChannelUI, c.Channel)
	require.Equal(t, c.Text, c.Span)
	require.Equal(t, day("2026-09-01"), *c.ValidFrom)
	require.Equal(t, memory.ClaimHash(nil, "user.employer", jv("Globex")), c.ContentHash)
	ep, err := f.r.Memory.Episode(f.ctx, c.SourceID)
	require.NoError(t, err)
	require.Equal(t, model.EpisodeEdit, ep.Kind)
	require.Equal(t, `"Works at ACME as an intern" -> "Works at Globex as an intern"`, ep.Body)
	old = f.claim(old.ID)
	require.Equal(t, model.ClaimSuperseded, old.Status)
	require.Equal(t, c.ID, *old.SupersededBy)
	require.Nil(t, old.ValidTo, "a correction keeps the old claim's time")
	require.Equal(t, []string{"claim.commit", "claim.supersede"}, f.actions())
	require.Equal(t, []string{"project claim:" + c.ID, "project claim:" + old.ID, "files "}, f.jobs())

	// A value its text does not hold makes a text from the key's label.
	other := f.seed(model.Claim{Key: "user.home", Value: jv("Lisbon"), Text: "Moved to Portugal last year"})
	v = "Porto"
	c, err = f.l.Edit(f.ctx, other.ID, memory.EditRequest{Value: &v})
	require.NoError(t, err)
	require.Equal(t, "Lives in Porto", c.Text)

	// A superseded claim cannot be edited.
	_, err = f.l.Edit(f.ctx, old.ID, memory.EditRequest{Text: "Works at Initech"})
	require.ErrorIs(t, err, store.ErrConflict)
}

func TestEditText(t *testing.T) {
	f := newFx(t)
	old := f.seed(model.Claim{Key: "pref.food", Cardinality: model.CardinalityMany, Value: jv("coffee"),
		Text: "Likes coffee", ValidFrom: tp(day("2026-01-01")), ValidTo: tp(day("2026-06-01"))})
	empty, until := "", "2026-12-31"
	c, err := f.l.Edit(f.ctx, old.ID, memory.EditRequest{Text: "  Likes cold brew  ", ValidFrom: &empty, ValidTo: &until})
	require.NoError(t, err)
	require.Equal(t, "Likes cold brew", c.Text)
	require.JSONEq(t, `"Likes cold brew"`, string(c.Value))
	require.Nil(t, c.ValidFrom)
	require.Equal(t, day("2027-01-01"), *c.ValidTo)
	// A one-valued claim given text, not a value, takes the text as both.
	name := f.seed(model.Claim{Key: "user.name", Value: jv("Sam"), Text: "Their name is Sam"})
	c, err = f.l.Edit(f.ctx, name.ID, memory.EditRequest{Text: "Samuel"})
	require.NoError(t, err)
	require.JSONEq(t, `"Samuel"`, string(c.Value))
	bad := "next week"
	_, err = f.l.Edit(f.ctx, c.ID, memory.EditRequest{Text: "Sam", ValidFrom: &bad})
	require.ErrorIs(t, err, store.ErrInvalid)
	_, err = f.l.Edit(f.ctx, c.ID, memory.EditRequest{Text: "  "})
	require.ErrorIs(t, err, store.ErrInvalid)
	_, err = f.l.Edit(f.ctx, c.ID, memory.EditRequest{Text: "password: hunter22"})
	require.ErrorIs(t, err, store.ErrInvalid)
	_, err = f.l.Edit(f.ctx, model.NewID(), memory.EditRequest{Text: "x"})
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestConfirm(t *testing.T) {
	f := newFx(t)
	old := f.seed(model.Claim{Key: "user.employer", Text: "Works at Acme", MentionedAt: t0.Add(-48 * time.Hour)})
	cand := f.seed(model.Claim{Key: "user.employer", Text: "Works at Globex", Status: model.ClaimCandidate,
		Trust: model.TrustInferred})
	c, err := f.l.Confirm(f.ctx, cand.ID)
	require.NoError(t, err)
	require.Equal(t, model.ClaimActive, c.Status)
	require.Equal(t, model.TrustOwnerEdit, c.Trust, "the user vouched for it")
	require.Equal(t, model.TrustOwnerEdit, f.claim(cand.ID).Trust)
	old = f.claim(old.ID)
	require.Equal(t, model.ClaimSuperseded, old.Status)
	require.Equal(t, cand.ID, *old.SupersededBy)
	require.Equal(t, t0, *old.ValidTo)
	require.Equal(t, []string{"claim.confirm", "claim.supersede"}, f.actions())
	// Confirming an active claim changes nothing.
	again, err := f.l.Confirm(f.ctx, cand.ID)
	require.NoError(t, err)
	require.Equal(t, c.Rev, again.Rev)
	_, err = f.l.Confirm(f.ctx, old.ID)
	require.ErrorIs(t, err, store.ErrConflict)
}

// conflicted makes two active claims of user.employer in an open conflict.
func (f *fx) conflicted() (a, b model.Claim, cf model.Conflict) {
	f.t.Helper()
	a = f.seed(model.Claim{Key: "user.employer", Text: "Works at Acme"})
	b = f.seed(model.Claim{Key: "user.employer", Text: "Works at Globex"})
	cf = model.Conflict{ID: model.NewID(), ClaimA: a.ID, ClaimB: b.ID, Reason: "two employers",
		Sensitivity: model.SensitivityPersonal, Status: model.ConflictOpen}
	require.NoError(f.t, f.r.Memory.Apply(f.ctx, store.MemoryChange{Conflicts: []model.Conflict{cf}}))
	return a, b, cf
}

func TestRejectDismissesConflicts(t *testing.T) {
	f := newFx(t)
	a, b, cf := f.conflicted()
	c, err := f.l.Reject(f.ctx, a.ID)
	require.NoError(t, err)
	require.Equal(t, model.ClaimRejected, c.Status)
	got, err := f.r.Memory.Conflict(f.ctx, cf.ID)
	require.NoError(t, err)
	require.Equal(t, model.ConflictDismissed, got.Status)
	require.Equal(t, model.ClaimActive, f.claim(b.ID).Status)
	require.Equal(t, []string{"claim.reject", "conflict.dismiss"}, f.actions())
	require.Equal(t, []string{"project claim:" + a.ID, "files "}, f.jobs())
	// Rejecting it again changes nothing.
	_, err = f.l.Reject(f.ctx, a.ID)
	require.NoError(t, err)
	require.Len(t, f.actions(), 2)
}

func TestSuppress(t *testing.T) {
	f := newFx(t)
	c := f.seed(model.Claim{Key: "user.home", Text: "Lives in Oslo"})
	got, err := f.l.Suppress(f.ctx, c.ID, true)
	require.NoError(t, err)
	require.True(t, got.Suppressed)
	require.Equal(t, model.ClaimActive, got.Status)
	got, err = f.l.Suppress(f.ctx, c.ID, false)
	require.NoError(t, err)
	require.False(t, got.Suppressed)
	require.Equal(t, []string{"claim.suppress", "claim.unsuppress"}, f.actions())
}

func TestForget(t *testing.T) {
	f := newFx(t)
	a, _, cf := f.conflicted()
	before, err := f.l.Forget(f.ctx, a.ID)
	require.NoError(t, err)
	require.Equal(t, "Works at Acme", before.Text)
	_, err = f.r.Memory.Claim(f.ctx, a.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	got, err := f.r.Memory.Conflict(f.ctx, cf.ID)
	require.NoError(t, err)
	require.Equal(t, model.ConflictDismissed, got.Status, "the store dismisses the forgotten claim's conflicts")
	require.Equal(t, []string{"conflict.dismiss", "claim.forget"}, f.actions())
	require.Equal(t, []string{"project claim:" + a.ID, "files "}, f.jobs())
	_, err = f.l.Forget(f.ctx, a.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestResolve(t *testing.T) {
	for _, tc := range []struct {
		resolution string
		a, b       string
	}{
		{model.ResolveKeepA, model.ClaimActive, model.ClaimRejected},
		{model.ResolveKeepB, model.ClaimRejected, model.ClaimActive},
		{model.ResolveBoth, model.ClaimActive, model.ClaimActive},
		{model.ResolveNeither, model.ClaimRejected, model.ClaimRejected},
	} {
		t.Run(tc.resolution, func(t *testing.T) {
			f := newFx(t)
			a, b, cf := f.conflicted()
			got, err := f.l.Resolve(f.ctx, cf.ID, tc.resolution)
			require.NoError(t, err)
			require.Equal(t, model.ConflictResolved, got.Status)
			require.Equal(t, tc.resolution, *got.Resolution)
			require.NotNil(t, got.ResolvedAt)
			require.Equal(t, tc.a, f.claim(a.ID).Status)
			require.Equal(t, tc.b, f.claim(b.ID).Status)
			require.Equal(t, "conflict.resolve", f.actions()[0])
			_, err = f.l.Resolve(f.ctx, cf.ID, tc.resolution)
			require.ErrorIs(t, err, store.ErrConflict)
		})
	}
	f := newFx(t)
	_, _, cf := f.conflicted()
	_, err := f.l.Resolve(f.ctx, cf.ID, "maybe")
	require.ErrorIs(t, err, store.ErrInvalid)
}
