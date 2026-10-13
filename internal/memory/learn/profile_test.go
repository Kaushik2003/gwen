package learn

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/stretchr/testify/require"
)

func TestProfileWithNoClaims(t *testing.T) {
	f := newFx(t)
	card, err := f.l.Profile(f.ctx, "2026-10-01")
	require.NoError(t, err)
	require.Empty(t, card.Text)
	require.Empty(t, card.Refs)
	latest, err := f.r.MemoryState.LatestProfile(f.ctx)
	require.NoError(t, err)
	require.Equal(t, card.ID, latest.ID)
	require.Equal(t, "2026-10-01", latest.Day)
}

func TestProfileSectionsAndMarks(t *testing.T) {
	f := newFx(t)
	anna := model.Entity{ID: model.NewID(), Kind: memory.EntityPerson, Name: "Anna", Aliases: []string{}}
	bob := model.Entity{ID: model.NewID(), Kind: memory.EntityPerson, Name: "bob", Aliases: []string{}}
	acme := model.Entity{ID: model.NewID(), Kind: memory.EntityOrg, Name: "Acme", Aliases: []string{}}
	require.NoError(t, f.r.Memory.Apply(f.ctx, store.MemoryChange{Entities: []model.Entity{anna, bob, acme}}))
	at := func(m int) time.Time { return t0.Add(time.Duration(m) * time.Minute) }
	ids := map[string]string{}
	add := func(c model.Claim) {
		c = f.seed(c)
		ids[c.Text] = c.ID
	}
	add(model.Claim{Key: "pref.music", Cardinality: model.CardinalityMany, Text: "Likes jazz", RecordedAt: at(1)})
	add(model.Claim{Key: "pref.food", Cardinality: model.CardinalityMany, Text: "Likes sushi", RecordedAt: at(2)})
	add(model.Claim{Key: "user.employer", Text: "Works at Acme", ValidFrom: tp(day("2026-09-01")), RecordedAt: at(3)})
	add(model.Claim{Key: "user.name", Text: "Their name is Sam", Trust: model.TrustOwnerEdit, RecordedAt: at(4)})
	add(model.Claim{Key: "user.school", Text: "Studies at MIT", RecordedAt: at(5)})
	add(model.Claim{Key: "user.school", Text: "Studies at Stanford", RecordedAt: at(6)})
	add(model.Claim{Key: "person.fact", SubjectID: &bob.ID, Cardinality: model.CardinalityMany,
		Text: "Bob plays chess", RecordedAt: at(7)})
	add(model.Claim{Key: "person.birthday", SubjectID: &anna.ID, Text: "Anna's birthday is 3 May", RecordedAt: at(8)})
	add(model.Claim{Key: "person.relation", SubjectID: &anna.ID, Text: "Anna is their sister", RecordedAt: at(9)})
	add(model.Claim{Key: "org.fact", SubjectID: &acme.ID, Cardinality: model.CardinalityMany,
		Text: "Acme makes rockets", RecordedAt: at(10)})
	// Left out: sensitive, hidden, not confirmed, no longer true, superseded.
	add(model.Claim{Key: "user.health", Cardinality: model.CardinalityMany, Text: "Has asthma",
		Sensitivity: model.SensitivitySensitive})
	add(model.Claim{Key: "user.home", Text: "Lives in Oslo", Suppressed: true})
	add(model.Claim{Key: "user.role", Text: "Works as a designer", Status: model.ClaimCandidate,
		Trust: model.TrustInferred})
	add(model.Claim{Key: "user.plan", Cardinality: model.CardinalityMany, Text: "Was in Rome",
		ValidTo: tp(day("2026-08-10"))})
	conflict := model.Conflict{ID: model.NewID(), ClaimA: ids["Studies at MIT"], ClaimB: ids["Studies at Stanford"],
		Sensitivity: model.SensitivityPersonal, Status: model.ConflictOpen}
	require.NoError(t, f.r.Memory.Apply(f.ctx, store.MemoryChange{Conflicts: []model.Conflict{conflict}}))

	card, err := f.l.Profile(f.ctx, "2026-10-01")
	require.NoError(t, err)
	require.Equal(t, `What you know about them (cite [pN] when you rely on one):

About them:
- [p1] Their name is Sam
- [p2] Works at Acme (since Sep 2026)
- [p3] Studies at Stanford (disputed)
- [p4] Studies at MIT (disputed)
- [p5] Likes sushi
- [p6] Likes jazz

People:
- [p7] Anna is their sister
- [p8] Anna's birthday is 3 May
- [p9] Bob plays chess

Other:
- [p10] Acme makes rockets`, card.Text)
	require.Len(t, card.Refs, 10)
	require.Equal(t, "claim:"+ids["Their name is Sam"], card.Refs["p1"])
	require.Equal(t, "claim:"+ids["Acme makes rockets"], card.Refs["p10"])

	// The same claims make the same card.
	again, err := f.l.Profile(f.ctx, "2026-10-01")
	require.NoError(t, err)
	require.NotEqual(t, card.ID, again.ID)
	require.Equal(t, card.Text, again.Text)
	require.Equal(t, card.Refs, again.Refs)
}

func TestProfileBudget(t *testing.T) {
	f := newFx(t)
	text := func(i int) string {
		return fmt.Sprintf("Fact number %02d about them, written out at some length so that the card fills up fast", i)
	}
	var edits, used, newest, oldest []string
	for i := range 60 {
		c := f.seed(model.Claim{Key: "user.fact", Cardinality: model.CardinalityMany, Text: text(i),
			RecordedAt: t0.Add(time.Duration(i) * time.Minute)})
		switch {
		case i < 3:
			oldest = append(oldest, c.Text)
		case i >= 57:
			newest = append(newest, c.Text)
		}
		if i >= 3 && i < 5 {
			used = append(used, c.Text)
			require.NoError(t, f.r.MemoryState.TouchClaims(f.ctx, []string{c.ID}, t0))
		}
	}
	for i := range 3 {
		c := f.seed(model.Claim{Key: "user.fact", Cardinality: model.CardinalityMany, Text: text(60 + i),
			Trust: model.TrustOwnerEdit, RecordedAt: t0.Add(-time.Duration(i+1) * time.Hour)})
		edits = append(edits, c.Text)
	}
	card, err := f.l.Profile(f.ctx, "2026-10-01")
	require.NoError(t, err)
	require.LessOrEqual(t, memory.Tokens(card.Text), profileTokens)
	require.Greater(t, memory.Tokens(card.Text), profileTokens-60, "the budget is used, to within two lines")
	require.Less(t, len(card.Refs), 63)
	for _, s := range append(append(edits, used...), newest...) {
		require.Contains(t, card.Text, s)
	}
	for _, s := range oldest {
		require.NotContains(t, card.Text, s)
	}
	// Refs are numbered in the order shown.
	lines := strings.Split(card.Text, "\n")
	require.True(t, strings.HasPrefix(lines[3], "- [p1] "))
	require.True(t, strings.HasPrefix(lines[len(lines)-1], fmt.Sprintf("- [p%d] ", len(card.Refs))))
}
