package learn

import (
	"context"
	"slices"
	"time"

	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
)

// plan is one write being decided. It reads the claims it may change once,
// changes copies of them, and adds new rows; commit turns that into one
// store.MemoryChange whose claim updates each carry the status read as
// WasStatus, so a claim changed meanwhile fails the whole write. A plan is
// used under Learner.mu.
type plan struct {
	l   *Learner
	ctx context.Context

	loaded map[string]*model.Claim // claims read, as changed so far
	orig   map[string]model.Claim  // the same claims as read
	order  []string                // loaded ids, in the order read
	added  []*model.Claim
	// actives caches which (key, subject) pairs have had their active claims
	// loaded.
	actives map[string]bool

	entities        []model.Entity
	conflicts       []model.Conflict
	conflictUpdates []store.ConflictUpdate
	links           []model.MemoryLink
	ledger          []store.LedgerItem
	result          Result
}

func (l *Learner) newPlan(ctx context.Context) *plan {
	return &plan{l: l, ctx: ctx, loaded: map[string]*model.Claim{}, orig: map[string]model.Claim{},
		actives: map[string]bool{}}
}

// track adds a claim read from the store, unless it is tracked already, and
// returns the tracked copy.
func (p *plan) track(c model.Claim) *model.Claim {
	if t, ok := p.loaded[c.ID]; ok {
		return t
	}
	cp := c
	p.loaded[c.ID], p.orig[c.ID] = &cp, c
	p.order = append(p.order, c.ID)
	return &cp
}

// claim returns a live claim, tracked.
func (p *plan) claim(id string) (*model.Claim, error) {
	if c, ok := p.loaded[id]; ok {
		return c, nil
	}
	for _, c := range p.added {
		if c.ID == id {
			return c, nil
		}
	}
	c, err := p.l.o.Repo.Claim(p.ctx, id)
	if err != nil {
		return nil, err
	}
	return p.track(c), nil
}

// subjectKey names a key about a subject.
func subjectKey(key string, subjectID *string) string {
	if subjectID == nil {
		return key + "\x00"
	}
	return key + "\x00" + *subjectID
}

func sameSubject(a, b *string) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

// activeOf returns the claims of key about subject that are active in this
// plan: those in the store and those added, as changed so far.
func (p *plan) activeOf(key string, subjectID *string) ([]*model.Claim, error) {
	if sk := subjectKey(key, subjectID); !p.actives[sk] {
		got, err := p.l.o.Repo.ClaimsByKey(p.ctx, key, subjectID, model.ClaimActive)
		if err != nil {
			return nil, err
		}
		for _, c := range got {
			p.track(c)
		}
		p.actives[sk] = true
	}
	var out []*model.Claim
	for _, id := range p.order {
		if c := p.loaded[id]; c.Key == key && sameSubject(c.SubjectID, subjectID) && c.Status == model.ClaimActive {
			out = append(out, c)
		}
	}
	for _, c := range p.added {
		if c.Key == key && sameSubject(c.SubjectID, subjectID) && c.Status == model.ClaimActive {
			out = append(out, c)
		}
	}
	return out, nil
}

// add adds a new claim and its ledger row.
func (p *plan) add(c *model.Claim) {
	p.added = append(p.added, c)
	p.ledger = append(p.ledger, ledgerRow("claim.commit", c))
	p.result.Learned++
}

// addEntity adds a new entity once.
func (p *plan) addEntity(e model.Entity) {
	if !slices.ContainsFunc(p.entities, func(x model.Entity) bool { return x.ID == e.ID }) {
		p.entities = append(p.entities, e)
	}
}

// link adds a link from a claim, once.
func (p *plan) link(claimID, toKind, toID, relation string) {
	if toID == "" || slices.ContainsFunc(p.links, func(l model.MemoryLink) bool {
		return l.FromID == claimID && l.ToKind == toKind && l.ToID == toID && l.Relation == relation
	}) {
		return
	}
	p.links = append(p.links, model.MemoryLink{ID: model.NewID(), FromKind: "claim", FromID: claimID,
		ToKind: toKind, ToID: toID, Relation: relation})
}

// setStatus moves a claim to a status other than superseded, with its
// ledger row.
func (p *plan) setStatus(c *model.Claim, status, action string, extra ...any) {
	c.Status, c.SupersededBy, c.SupersededAt = status, nil, nil
	p.ledger = append(p.ledger, ledgerRow(action, c, extra...))
}

// supersede marks old superseded by by, ending old's validity at end when
// that is earlier than its own end.
func (p *plan) supersede(old *model.Claim, by string, end *time.Time) {
	old.Status, old.SupersededBy = model.ClaimSuperseded, &by
	if end != nil && (old.ValidTo == nil || old.ValidTo.After(*end)) &&
		(old.ValidFrom == nil || !end.Before(*old.ValidFrom)) {
		e := *end
		old.ValidTo = &e
	}
	p.ledger = append(p.ledger, ledgerRow("claim.supersede", old, "by", by))
}

// openConflict opens a conflict between a and b unless one is open already.
func (p *plan) openConflict(a, b *model.Claim, reason string) error {
	if slices.ContainsFunc(p.conflicts, func(c model.Conflict) bool {
		return c.ClaimA == a.ID && c.ClaimB == b.ID || c.ClaimA == b.ID && c.ClaimB == a.ID
	}) {
		return nil
	}
	for _, c := range []*model.Claim{a, b} {
		if _, stored := p.loaded[c.ID]; !stored {
			continue
		}
		open, err := p.l.o.Repo.OpenConflictsOf(p.ctx, c.ID)
		if err != nil {
			return err
		}
		if slices.ContainsFunc(open, func(o model.Conflict) bool {
			return o.ClaimA == a.ID && o.ClaimB == b.ID || o.ClaimA == b.ID && o.ClaimB == a.ID
		}) {
			return nil
		}
	}
	cf := model.Conflict{ID: model.NewID(), ClaimA: a.ID, ClaimB: b.ID, Reason: memory.Cut(reason, 500),
		Sensitivity: memory.MoreSensitive(a.Sensitivity, b.Sensitivity), Status: model.ConflictOpen}
	p.conflicts = append(p.conflicts, cf)
	p.ledger = append(p.ledger, store.LedgerItem{Data: map[string]any{"action": "conflict.open",
		"conflict": cf.ID, "a": a.ID, "b": b.ID}})
	p.result.Conflicts = true
	return nil
}

// dismissConflicts dismisses the open conflicts a stored claim is in.
func (p *plan) dismissConflicts(id, reason string) error {
	open, err := p.l.o.Repo.OpenConflictsOf(p.ctx, id)
	if err != nil {
		return err
	}
	for _, c := range open {
		if slices.ContainsFunc(p.conflictUpdates, func(u store.ConflictUpdate) bool { return u.ID == c.ID }) {
			continue
		}
		p.conflictUpdates = append(p.conflictUpdates, store.ConflictUpdate{ID: c.ID, Status: model.ConflictDismissed})
		p.ledger = append(p.ledger, store.LedgerItem{Data: map[string]any{"action": "conflict.dismiss",
			"conflict": c.ID, "reason": reason}})
	}
	return nil
}

// validFrom is when a claim began to hold, for supersession: its valid_from,
// or else when it was said.
func validFrom(c *model.Claim) time.Time {
	if c.ValidFrom != nil {
		return *c.ValidFrom
	}
	return c.MentionedAt
}

// settle applies one-valued supersession (spec 5, 2.8) to n, an active
// claim, against the other active claims of its key and subject with a
// different hash: a claim older in both world time and when it was said is
// superseded by n; one newer in both supersedes n (a backfill); any other
// pair is a conflict, and both stay active.
func (p *plan) settle(n *model.Claim) error {
	if n.Cardinality != model.CardinalityOne || n.Status != model.ClaimActive {
		return nil
	}
	actives, err := p.activeOf(n.Key, n.SubjectID)
	if err != nil {
		return err
	}
	var others []*model.Claim
	for _, o := range actives {
		if o.ID != n.ID && o.ContentHash != n.ContentHash {
			others = append(others, o)
		}
	}
	vfN, mN := validFrom(n), n.MentionedAt
	bothEqual := func(o *model.Claim) bool { return validFrom(o).Equal(vfN) && o.MentionedAt.Equal(mN) }
	// A backfill: n is superseded by the earliest claim newer in both.
	var after *model.Claim
	for _, o := range others {
		if !vfN.After(validFrom(o)) && !mN.After(o.MentionedAt) && !bothEqual(o) {
			if after == nil || validFrom(o).Before(validFrom(after)) ||
				validFrom(o).Equal(validFrom(after)) && o.MentionedAt.Before(after.MentionedAt) {
				after = o
			}
		}
	}
	if after != nil {
		end := validFrom(after)
		p.supersede(n, after.ID, &end)
		return nil
	}
	for _, o := range others {
		if !vfN.Before(validFrom(o)) && !mN.Before(o.MentionedAt) && !bothEqual(o) {
			p.supersede(o, n.ID, &vfN)
			continue
		}
		label := n.Key
		if spec, ok := memory.Spec(n.Key); ok {
			label = spec.Label
		}
		if err := p.openConflict(o, n, label+": two values hold at once, and which is newer is not clear"); err != nil {
			return err
		}
	}
	return nil
}

func sameTime(a, b *time.Time) bool {
	return a == nil && b == nil || a != nil && b != nil && a.Equal(*b)
}

// commit applies the plan in one change: entities, new claims, the changes
// to the claims read, conflicts, links, ledger rows, and jobs (a project job
// for every new or changed claim and one files job). A plan that changes
// nothing writes nothing.
func (p *plan) commit() (Result, error) {
	ch := store.MemoryChange{Entities: p.entities, Conflicts: p.conflicts, ConflictUpdates: p.conflictUpdates,
		Links: p.links, Ledger: p.ledger}
	var changed []string
	for _, c := range p.added {
		ch.Claims = append(ch.Claims, *c)
		changed = append(changed, c.ID)
	}
	for _, id := range p.order {
		c, o := p.loaded[id], p.orig[id]
		u := store.ClaimUpdate{ID: id, WasStatus: o.Status}
		set := false
		switch {
		case c.Status == model.ClaimSuperseded && c.SupersededBy != nil &&
			(o.SupersededBy == nil || *o.SupersededBy != *c.SupersededBy):
			u.SupersededBy, set = c.SupersededBy, true
		case c.Status != o.Status:
			status := c.Status
			u.Status, set = &status, true
		}
		if !sameTime(c.ValidTo, o.ValidTo) {
			u.ValidTo, set = store.Nullable[time.Time]{Set: true, Value: c.ValidTo}, true
		}
		if c.Suppressed != o.Suppressed {
			s := c.Suppressed
			u.Suppressed, set = &s, true
		}
		if c.Trust != o.Trust {
			trust := c.Trust
			u.Trust, set = &trust, true
		}
		if c.Confidence != o.Confidence {
			conf := c.Confidence
			u.Confidence, set = &conf, true
		}
		if set {
			ch.Updates = append(ch.Updates, u)
			changed = append(changed, id)
		}
	}
	if len(ch.Claims) == 0 && len(ch.Updates) == 0 && len(ch.Conflicts) == 0 && len(ch.ConflictUpdates) == 0 &&
		len(ch.Links) == 0 {
		return Result{}, nil
	}
	for _, id := range changed {
		ch.Jobs = append(ch.Jobs, projectJob(id))
	}
	if len(changed) > 0 || len(ch.Conflicts) > 0 || len(ch.ConflictUpdates) > 0 {
		ch.Jobs = append(ch.Jobs, filesJob())
	}
	if err := p.l.o.Repo.Apply(p.ctx, ch); err != nil {
		return Result{}, err
	}
	res := p.result
	res.Claims = changed
	return res, nil
}
