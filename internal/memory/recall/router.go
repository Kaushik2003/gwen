// Package recall is memory's read path (harness 4.4): the router reads what
// a query asks for, asks its lanes in parallel under deadlines, fuses their
// rankings, keeps only what may be recalled, and packs the memory note.
package recall

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
)

// DataLane answers questions about the user's tracked data (tasks, goals,
// time worked) by fixed queries; w is the time the query names, if any.
type DataLane interface {
	Search(ctx context.Context, q memory.Query, w *memory.TimeRange) ([]memory.Hit, error)
}

// Options configures a Router. Day is the local day of an instant
// (YYYY-MM-DD), aware of when the user's day rolls over.
type Options struct {
	Repo  store.MemoryRepo
	State store.MemoryStateRepo
	Chats store.ChatRepo
	Data  DataLane      // nil: no data lane
	Lanes []memory.Lane // extra lanes: vec, mem0, supermemory (any may be absent)
	Clock clock.Clock
	Loc   *time.Location
	Day   func(time.Time) string
	// Scale stretches every lane's deadline and a query's default deadline
	// by a factor (0 or 1 keeps them): a test under the race detector reads
	// SQLite many times slower than the deadlines expect.
	Scale float64
}

// Defaults of a query's budget, and how long each lane may take.
const (
	defaultCandidates = 40
	defaultDeadline   = 600 * time.Millisecond
	localDeadline     = 150 * time.Millisecond
	vecDeadline       = 400 * time.Millisecond
	vendorDeadline    = 500 * time.Millisecond
	// timeCap is how many messages and episodes the time lane reads.
	timeCap = 40
	// rrfK is the k of reciprocal rank fusion.
	rrfK = 60
	// hitChars is how much of a message, episode, or document a hit shows.
	hitChars = 300
	// sensitiveText stands in for a sensitive hit's words in a tainted run.
	sensitiveText = "(sensitive: ask to see it)"
	// asOfSlack is how far before now an as_of must be to read the past.
	asOfSlack = time.Minute
)

// Router is the read path. It is safe for concurrent use.
type Router struct {
	o Options

	mu    sync.RWMutex
	lanes []memory.Lane

	// deadlines are how long each lane may take, and wait how long a query
	// that does not say may take; tests lengthen them.
	deadlines map[memory.LaneName]time.Duration
	wait      time.Duration
}

// New returns a Router.
func New(o Options) *Router {
	if o.Clock == nil {
		o.Clock = clock.Real()
	}
	if o.Loc == nil {
		o.Loc = time.Local
	}
	if o.Day == nil {
		loc := o.Loc
		o.Day = func(t time.Time) string { return t.In(loc).Format(time.DateOnly) }
	}
	r := &Router{o: o, wait: defaultDeadline, deadlines: map[memory.LaneName]time.Duration{
		memory.LaneClaims: localDeadline, memory.LaneChats: localDeadline, memory.LaneEpisodes: localDeadline,
		memory.LaneEntity: localDeadline, memory.LaneTime: localDeadline, memory.LaneData: localDeadline,
		memory.LaneVec: vecDeadline, memory.LaneMem0: vendorDeadline, memory.LaneSupermemory: vendorDeadline,
	}}
	if o.Scale > 1 {
		for name, d := range r.deadlines {
			r.deadlines[name] = time.Duration(float64(d) * o.Scale)
		}
		r.wait = time.Duration(float64(r.wait) * o.Scale)
	}
	r.SetLanes(o.Lanes)
	return r
}

// SetLanes replaces the extra lanes, as when the configuration changes. Nil
// lanes are left out.
func (r *Router) SetLanes(lanes []memory.Lane) {
	var keep []memory.Lane
	for _, l := range lanes {
		if l != nil {
			keep = append(keep, l)
		}
	}
	r.mu.Lock()
	r.lanes = keep
	r.mu.Unlock()
}

func (r *Router) extraLanes() []memory.Lane {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Clone(r.lanes)
}

func (r *Router) deadline(name memory.LaneName) time.Duration {
	if d, ok := r.deadlines[name]; ok {
		return d
	}
	return vendorDeadline
}

// cand is a hit on its way through the router, with the row it came from.
type cand struct {
	hit       memory.Hit
	claim     *model.Claim
	msg       *model.ChatHit
	ep        *model.Episode
	conflicts []model.Conflict
}

// lane is one lane of a recall.
type lane struct {
	name memory.LaneName
	run  func(ctx context.Context) ([]cand, error)
}

// query is a recall's settled reading.
type query struct {
	memory.Query
	now     time.Time
	asOf    time.Time
	past    bool // asOf is in the past: superseded claims that held then may be recalled
	window  *memory.TimeRange
	intent  string
	exclude map[string]bool
}

// settle fills in a query's defaults and reads its window and intent.
func (r *Router) settle(q memory.Query) query {
	now := r.o.Clock.Now()
	s := query{Query: q, now: now, asOf: q.AsOf, exclude: map[string]bool{}}
	if s.asOf.IsZero() {
		s.asOf = now
	}
	s.past = s.asOf.Before(now.Add(-asOfSlack))
	if s.Budget.Candidates <= 0 {
		s.Budget.Candidates = defaultCandidates
	}
	if s.Budget.Deadline <= 0 {
		s.Budget.Deadline = r.wait
	}
	s.window = q.Window
	if s.window == nil {
		s.window = r.window(q.Text, now)
	}
	s.intent = q.Intent
	if s.intent == "" {
		s.intent = intentOf(q.Text, s.window)
	}
	for _, ref := range q.Exclude {
		s.exclude[ref] = true
	}
	return s
}

// window reads the time text names, with "today" the user's day: past
// midnight, before the day rolls over, today is still yesterday's date and
// runs until now.
func (r *Router) window(text string, now time.Time) *memory.TimeRange {
	anchor := now
	start, err := time.ParseInLocation(time.DateOnly, r.o.Day(now), r.o.Loc)
	if err == nil && !now.Before(start.AddDate(0, 0, 1)) {
		anchor = start.AddDate(0, 0, 1).Add(-time.Millisecond)
	}
	w := ParseWindow(text, anchor, r.o.Loc)
	if w != nil && !anchor.Equal(now) && !anchor.Before(w.From) && anchor.Before(w.To) && !now.Before(w.To) {
		w.To = now.Add(time.Millisecond)
	}
	return w
}

// Recall runs the read path for a query: its lanes in parallel, each under
// its deadline, fused by weighted reciprocal rank, filtered, deduplicated,
// and packed to the budget.
func (r *Router) Recall(ctx context.Context, q memory.Query) (memory.Packet, error) {
	p, _, err := r.recall(ctx, q)
	return p, err
}

// recall is Recall, also returning the packed hits with their rows.
func (r *Router) recall(ctx context.Context, q memory.Query) (memory.Packet, []*cand, error) {
	s := r.settle(q)
	lanes := r.plan(ctx, s)
	results, degraded := r.run(ctx, s, lanes)

	fused := fuse(s, lanes, results)
	var kept []*cand
	for _, c := range fused {
		ok, err := r.admit(ctx, s, c)
		if err != nil {
			return memory.Packet{}, nil, err
		}
		if ok {
			kept = append(kept, c)
		}
	}
	kept = dedupe(kept)
	if s.Run.Tainted || s.Run.WorldReach {
		for _, c := range kept {
			if c.hit.Sensitivity == model.SensitivitySensitive {
				c.hit.Text, c.hit.Value = sensitiveText, nil
			}
		}
	}

	p := memory.Packet{Intent: s.intent, Degraded: degraded, Hits: []memory.Hit{}}
	// Gwen says she doesn't remember only when every lane that could have
	// answered did answer: a lane that failed or ran out of time proves nothing.
	if (s.intent == memory.IntentLookup || s.intent == memory.IntentEpisodic) && !slices.ContainsFunc(degraded,
		func(l memory.LaneName) bool {
			return slices.Contains([]memory.LaneName{memory.LaneClaims, memory.LaneChats, memory.LaneEpisodes,
				memory.LaneEntity, memory.LaneTime}, l)
		}) {
		p.Abstain = !slices.ContainsFunc(kept, func(c *cand) bool {
			return c.hit.Kind == kindClaim || c.hit.Kind == kindMessage || c.hit.Kind == kindEpisode
		})
	}
	d := r.describer(ctx, s.now)
	var packed []*cand
	for _, c := range kept {
		n := memory.Tokens(c.hit.Text)
		if s.Budget.Tokens > 0 && p.Tokens+n > s.Budget.Tokens {
			break
		}
		if err := d.fill(c); err != nil {
			return memory.Packet{}, nil, err
		}
		p.Tokens += n
		p.Hits = append(p.Hits, c.hit)
		packed = append(packed, c)
	}
	return p, packed, nil
}

// plan picks the lanes a query asks: those of its Lanes when set, else all
// that weigh something for its intent.
func (r *Router) plan(ctx context.Context, s query) []lane {
	ents := sync.OnceValues(func() ([]model.Entity, error) { return r.namedEntities(ctx, s.Text) })
	all := []lane{
		{memory.LaneClaims, func(ctx context.Context) ([]cand, error) { return r.claimsLane(ctx, s, ents) }},
		{memory.LaneEntity, func(ctx context.Context) ([]cand, error) { return r.entityLane(ctx, s, ents) }},
		{memory.LaneChats, func(ctx context.Context) ([]cand, error) { return r.chatsLane(ctx, s) }},
		{memory.LaneEpisodes, func(ctx context.Context) ([]cand, error) { return r.episodesLane(ctx, s) }},
	}
	if s.window != nil {
		all = append(all, lane{memory.LaneTime, func(ctx context.Context) ([]cand, error) { return r.timeLane(ctx, s) }})
	}
	if r.o.Data != nil {
		all = append(all, lane{memory.LaneData, func(ctx context.Context) ([]cand, error) { return r.dataLane(ctx, s) }})
	}
	for _, l := range r.extraLanes() {
		all = append(all, lane{l.Name(), func(ctx context.Context) ([]cand, error) { return r.extraLane(ctx, s, l) }})
	}
	var out []lane
	for _, l := range all {
		if len(s.Lanes) > 0 && !slices.Contains(s.Lanes, l.name) {
			continue
		}
		if weight(s.intent, l.name) <= 0 {
			continue
		}
		out = append(out, l)
	}
	return out
}

// run asks the lanes concurrently. A lane that fails or misses its deadline
// gives nothing and is named in degraded.
func (r *Router) run(ctx context.Context, s query, lanes []lane) (results [][]cand, degraded []memory.LaneName) {
	ctx, cancel := context.WithTimeout(ctx, s.Budget.Deadline)
	defer cancel()
	results = make([][]cand, len(lanes))
	failed := make([]bool, len(lanes))
	var wg sync.WaitGroup
	for i, l := range lanes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lctx, cancel := context.WithTimeout(ctx, r.deadline(l.name))
			defer cancel()
			type answer struct {
				cands []cand
				err   error
			}
			ch := make(chan answer, 1)
			go func() {
				cands, err := l.run(lctx)
				ch <- answer{cands, err}
			}()
			select {
			case a := <-ch:
				results[i], failed[i] = a.cands, a.err != nil
			case <-lctx.Done():
				failed[i] = true
			}
			if failed[i] {
				results[i] = nil
			}
		}()
	}
	wg.Wait()
	for i, l := range lanes {
		if failed[i] {
			degraded = append(degraded, l.name)
		}
	}
	return results, degraded
}

// fuse merges the lanes' rankings: a hit's score is the sum over the lanes
// that found it of weight(intent, lane) / (60 + rank), rank from 1. The
// result is best first.
func fuse(s query, lanes []lane, results [][]cand) []*cand {
	byRef := map[string]*cand{}
	var order []*cand
	for i, l := range lanes {
		w := weight(s.intent, l.name)
		rank := 0
		seen := map[string]bool{}
		for _, c := range results[i] {
			ref := c.hit.Ref
			if ref == "" || seen[ref] || s.exclude[ref] {
				continue
			}
			seen[ref] = true
			rank++
			f, ok := byRef[ref]
			if !ok {
				f = &cand{hit: c.hit, claim: c.claim, msg: c.msg, ep: c.ep}
				f.hit.Score, f.hit.Lanes = 0, nil
				byRef[ref] = f
				order = append(order, f)
			}
			f.hit.Score += w / float64(rrfK+rank)
			f.hit.Lanes = append(f.hit.Lanes, l.name)
		}
	}
	slices.SortStableFunc(order, func(a, b *cand) int { return cmp.Compare(b.hit.Score, a.hit.Score) })
	return order
}

// admit reports whether a hit may be recalled: a claim must be active, or
// superseded but holding at a past as_of; it must hold at as_of and not be
// suppressed. A claim in an open conflict is marked disputed.
func (r *Router) admit(ctx context.Context, s query, c *cand) (bool, error) {
	if c.claim == nil {
		return true, nil
	}
	if !recallable(*c.claim, s.asOf, s.past) {
		return false, nil
	}
	conflicts, err := r.o.Repo.OpenConflictsOf(ctx, c.claim.ID)
	if err != nil {
		return false, err
	}
	c.conflicts, c.hit.Disputed = conflicts, len(conflicts) > 0
	return true, nil
}

// recallable reports whether a claim may be recalled as holding at asOf;
// past is set when asOf is in the past. A superseded claim with no end held
// until it was superseded.
func recallable(c model.Claim, asOf time.Time, past bool) bool {
	if c.Suppressed || c.DeletedAt != nil || !c.ValidAt(asOf) {
		return false
	}
	switch c.Status {
	case model.ClaimActive:
		return true
	case model.ClaimSuperseded:
		return past && (c.ValidTo != nil || c.SupersededAt == nil || asOf.Before(*c.SupersededAt))
	}
	return false
}

// dedupe drops message hits whose words are another hit's: a message that
// says what a claim, an episode, or a better message says.
func dedupe(cs []*cand) []*cand {
	others := map[string]bool{}
	for _, c := range cs {
		if c.hit.Kind != kindMessage {
			others[memory.Normalize(c.hit.Text)] = true
		}
	}
	seen := map[string]bool{}
	var out []*cand
	for _, c := range cs {
		if c.hit.Kind == kindMessage {
			t := memory.Normalize(c.hit.Text)
			if others[t] || seen[t] {
				continue
			}
			seen[t] = true
		}
		out = append(out, c)
	}
	return out
}

// isNotFound reports whether err is the store's ErrNotFound.
func isNotFound(err error) bool { return errors.Is(err, store.ErrNotFound) }
