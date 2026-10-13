package learn

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
)

// The profile card (spec 5, 11).
const (
	profileTokens = 800
	profileHeader = "What you know about them (cite [pN] when you rely on one):"
)

// Sections of the profile card, in order.
var profileSections = []string{"About them:", "People:", "Other:"}

// profileLine is a claim on the card.
type profileLine struct {
	c       model.Claim
	section int
	group   string // the entity's name, for people and other
	text    string // the line after its ref
}

// Profile compiles the profile card for day from the active claims that
// hold now, leaving out hidden and sensitive ones, with no model: the same
// claims make the same card. Lines are chosen by trust, then use, then
// recency, until the card is about 800 tokens, and shown in section order.
// The card is stored and returned; with no claims its text is "".
func (l *Learner) Profile(ctx context.Context, day string) (model.ProfileCard, error) {
	now := l.now()
	claims, err := l.o.Repo.Claims(ctx, store.ClaimFilter{Statuses: []string{model.ClaimActive},
		ExcludeSensitive: true, ValidAt: &now})
	if err != nil {
		return model.ProfileCard{}, fmt.Errorf("profile card: %w", err)
	}
	text, refs, err := l.compileProfile(ctx, claims)
	if err != nil {
		return model.ProfileCard{}, fmt.Errorf("profile card: %w", err)
	}
	card, err := l.o.State.AddProfile(ctx, model.ProfileCard{Day: day, Text: text, Refs: refs})
	if err != nil {
		return model.ProfileCard{}, fmt.Errorf("profile card: %w", err)
	}
	return card, nil
}

func (l *Learner) compileProfile(ctx context.Context, claims []model.Claim) (string, map[string]string, error) {
	refs := map[string]string{}
	if len(claims) == 0 {
		return "", refs, nil
	}
	open, err := l.o.Repo.Conflicts(ctx, model.ConflictOpen)
	if err != nil {
		return "", nil, err
	}
	disputed := map[string]bool{}
	for _, c := range open {
		disputed[c.ClaimA], disputed[c.ClaimB] = true, true
	}
	entities, err := l.o.Repo.Entities(ctx, "")
	if err != nil {
		return "", nil, err
	}
	byID := map[string]model.Entity{}
	for _, e := range entities {
		byID[e.ID] = e
	}
	ids := make([]string, len(claims))
	for i, c := range claims {
		ids[i] = c.ID
	}
	usage, err := l.o.State.Usage(ctx, ids)
	if err != nil {
		return "", nil, err
	}
	var lines []profileLine
	for _, c := range claims {
		ln := profileLine{c: c, text: c.Text}
		if c.SubjectID != nil {
			e, ok := byID[*c.SubjectID]
			if !ok {
				continue
			}
			ln.section, ln.group = 2, e.Name
			if e.Kind == memory.EntityPerson {
				ln.section = 1
			}
		}
		if c.ValidFrom != nil {
			ln.text += " (since " + c.ValidFrom.In(l.loc).Format("Jan 2006") + ")"
		}
		if disputed[c.ID] {
			ln.text += " (disputed)"
		}
		lines = append(lines, ln)
	}
	// Choose by rank until the budget is spent.
	rank := slices.Clone(lines)
	slices.SortFunc(rank, func(a, b profileLine) int {
		return cmp.Or(cmp.Compare(memory.Tier(a.c.Trust), memory.Tier(b.c.Trust)),
			cmp.Compare(usage[b.c.ID].Uses, usage[a.c.ID].Uses), b.c.RecordedAt.Compare(a.c.RecordedAt),
			strings.Compare(a.c.ID, b.c.ID))
	})
	tokens := memory.Tokens(profileHeader)
	chosen := map[string]bool{}
	opened := map[int]bool{}
	for _, ln := range rank {
		cost := memory.Tokens("- [p00] "+ln.text) + 1
		if !opened[ln.section] {
			cost += memory.Tokens(profileSections[ln.section]) + 2
		}
		if tokens+cost > profileTokens {
			break
		}
		tokens += cost
		chosen[ln.c.ID], opened[ln.section] = true, true
	}
	lines = slices.DeleteFunc(lines, func(ln profileLine) bool { return !chosen[ln.c.ID] })
	slices.SortFunc(lines, profileOrder)
	var b strings.Builder
	b.WriteString(profileHeader)
	section := -1
	for i, ln := range lines {
		if ln.section != section {
			section = ln.section
			b.WriteString("\n\n" + profileSections[section])
		}
		ref := fmt.Sprintf("p%d", i+1)
		refs[ref] = memory.Ref(memory.RefClaim, ln.c.ID)
		b.WriteString("\n- [" + ref + "] " + ln.text)
	}
	if len(lines) == 0 {
		return "", refs, nil
	}
	return b.String(), refs, nil
}

// keyIndex is a key's place in memory.Keys, after them all when it is not
// there.
func keyIndex(key string) int {
	if i := slices.IndexFunc(memory.Keys, func(k memory.KeySpec) bool { return k.Key == key }); i >= 0 {
		return i
	}
	return len(memory.Keys)
}

// profileOrder is the order of the card: by section; people and other
// things grouped by name; in a group, a person's relation first, then
// one-valued keys in vocabulary order, then many-valued ones newest first.
func profileOrder(a, b profileLine) int {
	if c := cmp.Compare(a.section, b.section); c != 0 {
		return c
	}
	if c := cmp.Or(cmp.Compare(strings.ToLower(a.group), strings.ToLower(b.group)), cmp.Compare(a.group, b.group),
		cmp.Compare(subjectOf(a.c), subjectOf(b.c))); c != 0 {
		return c
	}
	place := func(c model.Claim) int {
		switch {
		case c.Key == "person.relation":
			return 0
		case c.Cardinality == model.CardinalityOne:
			return 1
		}
		return 2
	}
	if c := cmp.Compare(place(a.c), place(b.c)); c != 0 {
		return c
	}
	if a.c.Cardinality == model.CardinalityOne {
		if c := cmp.Compare(keyIndex(a.c.Key), keyIndex(b.c.Key)); c != 0 {
			return c
		}
	}
	return cmp.Or(b.c.RecordedAt.Compare(a.c.RecordedAt), strings.Compare(a.c.ID, b.c.ID))
}

func subjectOf(c model.Claim) string {
	if c.SubjectID == nil {
		return ""
	}
	return *c.SubjectID
}
