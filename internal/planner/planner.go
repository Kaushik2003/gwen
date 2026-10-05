// Package planner turns goals, tasks, and commitments into a plan for a day,
// as specified in docs/06-planner.md. Like the time engine it is pure: plain
// functions over values, with no store access, no clock reads, and no I/O.
// The store loads the inputs, calls these, and writes the results in one
// transaction.
package planner

import (
	"cmp"
	"math"
	"slices"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/planner/civil"
)

// Planning constants (docs/06-planner.md#generating-a-plan). They are not
// configuration.
const (
	MinBlock        = 25 // minutes
	MaxBlock        = 90 // minutes
	BlockStep       = 5  // minutes
	BlocksPerTask   = 2
	DefaultEstimate = 60 // minutes, for a task with none
	LongBlockGap    = 10 * time.Minute
)

// Urgency weights (docs/06-planner.md#urgency). They are not configuration.
const (
	WeightDue  = 0.40
	WeightPace = 0.25
	WeightPrio = 0.20
	WeightAge  = 0.15
)

// Goal pace values.
const (
	PaceAhead   = "ahead"
	PaceOnTrack = "on_track"
	PaceBehind  = "behind"
)

// Goal, Commitment, and PlanItem are the stored rows.
type (
	Goal       = model.Goal
	Commitment = model.Commitment
	PlanItem   = model.PlanItem
)

// Task is a stored task with the day its created_at belongs to, which anchors
// a template's recurrence.
type Task struct {
	model.Task
	Anchor civil.Day
}

func (t Task) live() bool { return t.DeletedAt == nil }

func (t Task) open() bool { return t.Status == model.TaskOpen }

func (t Task) isOccurrence() bool { return t.TemplateID != nil }

// startsAfter reports whether the task's start_day is after d.
func (t Task) startsAfter(d civil.Day) bool {
	return t.StartDay != nil && civil.MustParse(*t.StartDay).After(d)
}

// startOn is the minute after midnight the task may start at on d, or false
// when it has no start time on d.
func (t Task) startOn(d civil.Day) (int, bool) {
	if t.StartMinute == nil || t.StartDay == nil || civil.MustParse(*t.StartDay) != d {
		return 0, false
	}
	return *t.StartMinute, true
}

// due is the task's due day, or false when it has none.
func (t Task) due() (civil.Day, bool) {
	if t.DueDay == nil {
		return civil.Day{}, false
	}
	return civil.MustParse(*t.DueDay), true
}

// GoalProgress is a goal's progress on a day (docs/06-planner.md#goals-and-progress).
type GoalProgress struct {
	Target, Done, Remaining int
	DaysTotal, DaysElapsed  int
	SessionsLeft            int
	RequiredPerDay          float64
	ActualPerDay            float64
	ExpectedByNow           float64
	Pace                    string
	ProjectedFinish         *civil.Day
	// PerSession is the units a session holds, set only for a quantity goal
	// with daily minutes.
	PerSession *int
	Reachable  int
	// NeededDaily and NeededDue are the fixes for a goal that does not fit:
	// the minutes a day that would, and the due day that would.
	NeededDaily *int
	NeededDue   *civil.Day
	LinedUp     int // units in open items and session steps
}

// FeasibilityHorizon is how far ahead NeededDue is looked for, in days.
const FeasibilityHorizon = 3 * 366

// IsItem reports whether t is an item of a live quantity goal in goals: a
// unit waiting for a session, never planned on its own
// (docs/06-planner.md#sessions-and-steps).
func IsItem(t model.Task, goals map[string]Goal) bool {
	if t.GoalID == nil || t.RRule != nil || t.TemplateID != nil || t.ParentID != nil {
		return false
	}
	g, ok := goals[*t.GoalID]
	return ok && g.Kind == model.GoalQuantity
}

// ownPlan reports whether t is planned and reminded on its own: neither a
// step nor an item.
func ownPlan(t model.Task, goals map[string]Goal) bool { return t.ParentID == nil && !IsItem(t, goals) }

// units is the quantity a task covers, 1 when it has none.
func units(t model.Task) int {
	if t.Quantity == nil {
		return 1
	}
	return *t.Quantity
}

// Progress computes g's progress on today from the tasks linked to it. tasks
// may hold other tasks; only live ones with g's id count.
func Progress(g Goal, tasks []Task, today civil.Day) GoalProgress {
	start, due := civil.MustParse(g.StartDay), civil.MustParse(g.DueDay)
	var p GoalProgress
	var template *Task
	for i, t := range tasks {
		if !t.live() || t.GoalID == nil || *t.GoalID != g.ID {
			continue
		}
		if t.IsTemplate() {
			template = &tasks[i]
			continue
		}
		if g.Kind == model.GoalQuantity && t.open() && !t.isOccurrence() {
			p.LinedUp += units(t.Task)
		}
		if g.Kind == model.GoalTasks {
			p.Target++
			if t.Status == model.TaskDone {
				p.Done++
			}
		} else if t.Status == model.TaskDone && t.QuantityDone != nil {
			p.Done += *t.QuantityDone
		}
	}
	if g.Kind == model.GoalQuantity && g.TargetQuantity != nil {
		p.Target = *g.TargetQuantity
	}
	p.Remaining = max(p.Target-p.Done, 0)
	p.DaysTotal = start.DaysUntil(due) + 1
	p.DaysElapsed = min(max(start.DaysUntil(today)+1, 0), p.DaysTotal)

	from := today
	if from.Before(start) {
		from = start // nothing is planned before the goal starts
	}
	var rule *Rule
	if template != nil {
		if r, err := ParseRule(*template.RRule); err == nil {
			rule = &r
		}
	}
	if !from.After(due) {
		if g.Kind == model.GoalTasks {
			p.SessionsLeft = from.DaysUntil(due) + 1
		} else if rule != nil {
			p.SessionsLeft = len(rule.Between(template.Anchor, from, due))
		}
	}
	p.RequiredPerDay = float64(p.Remaining)
	if p.SessionsLeft > 0 {
		p.RequiredPerDay = float64(p.Remaining) / float64(p.SessionsLeft)
	}
	p.ActualPerDay = float64(p.Done) / float64(max(p.DaysElapsed, 1))
	p.ExpectedByNow = float64(p.Target) * float64(p.DaysElapsed) / float64(p.DaysTotal)

	ratio := 1.0
	if p.ExpectedByNow > 0 {
		ratio = float64(p.Done) / p.ExpectedByNow
	}
	switch {
	case ratio >= 1.1:
		p.Pace = PaceAhead
	case ratio < 0.9:
		p.Pace = PaceBehind
	default:
		p.Pace = PaceOnTrack
	}

	switch {
	case p.Remaining == 0:
		p.ProjectedFinish = &today
	case p.ActualPerDay > 0:
		d := today.AddDays(int(math.Ceil(float64(p.Remaining)/p.ActualPerDay)) - 1)
		p.ProjectedFinish = &d
	}

	p.Reachable = p.Target
	if g.Kind != model.GoalQuantity || g.Daily == nil || g.PerUnit == nil {
		return p
	}
	per := max(1, int(*g.Daily / *g.PerUnit))
	p.PerSession = &per
	p.Reachable = min(p.Target, p.Done+per*p.SessionsLeft)
	if p.Reachable >= p.Target {
		return p
	}
	if p.SessionsLeft > 0 {
		perUnit := int(*g.PerUnit / time.Minute)
		need := int(math.Ceil(p.RequiredPerDay)) * perUnit
		need = (need + BlockStep - 1) / BlockStep * BlockStep
		p.NeededDaily = &need
	}
	if rule != nil {
		p.NeededDue = nthSession(*rule, template.Anchor, from, (p.Remaining+per-1)/per)
	}
	return p
}

// nthSession is the n-th day on or after from that rule matches, or nil when
// none falls within FeasibilityHorizon days.
func nthSession(rule Rule, anchor, from civil.Day, n int) *civil.Day {
	for i := range FeasibilityHorizon {
		d := from.AddDays(i)
		if rule.Until != nil && d.After(*rule.Until) {
			return nil
		}
		if !d.Before(anchor) && rule.Matches(anchor, d) {
			if n--; n == 0 {
				return &d
			}
		}
	}
	return nil
}

// SessionQuantity is the quantity of g's session occurrence on day, from the
// progress computed with today = day, and false when no occurrence is due
// (docs/06-planner.md#goals-and-progress).
func SessionQuantity(g Goal, p GoalProgress, day civil.Day) (int, bool) {
	if g.Status != model.GoalActive || p.Remaining == 0 || day.Before(civil.MustParse(g.StartDay)) {
		return 0, false
	}
	q := max(1, int(math.Ceil(p.RequiredPerDay)))
	if p.PerSession != nil {
		q = min(q, *p.PerSession)
	}
	return q, true
}

// SessionSlot is an upcoming session that still needs items.
type SessionSlot struct {
	Day     civil.Day
	Units   int
	Minutes int
}

// Sessions lists the next n session days from today that still need items
// for g, given its session template, its progress today, and the units
// already lined up, which fill the first sessions
// (docs/06-planner.md#goals-and-progress).
func Sessions(g Goal, template Task, p GoalProgress, today civil.Day, linedUp, n int) []SessionSlot {
	if g.Kind != model.GoalQuantity || g.PerUnit == nil || template.RRule == nil {
		return nil
	}
	rule, err := ParseRule(*template.RRule)
	if err != nil {
		return nil
	}
	start, due := civil.MustParse(g.StartDay), civil.MustParse(g.DueDay)
	from := today
	if from.Before(start) {
		from = start
	}
	days := rule.Between(template.Anchor, from, due)
	remaining := p.Remaining
	var out []SessionSlot
	for i, d := range days {
		if remaining <= 0 || len(out) == n {
			break
		}
		q := int(math.Ceil(float64(remaining) / float64(len(days)-i)))
		if p.PerSession != nil {
			q = min(q, *p.PerSession)
		}
		q = max(q, 1)
		remaining -= q
		covered := min(linedUp, q)
		linedUp -= covered
		if q -= covered; q > 0 {
			out = append(out, SessionSlot{Day: d, Units: q, Minutes: q * int(*g.PerUnit/time.Minute)})
		}
	}
	return out
}

// Urgency scores a candidate task on day. progress holds the active goals
// only, so a task whose goal is missing from it has no pace factor.
func Urgency(t Task, day civil.Day, progress map[string]GoalProgress, rolloverCount int) float64 {
	due := 0.0
	if d, ok := t.due(); ok {
		due = clamp(1-float64(day.DaysUntil(d))/14, 0, 1)
	}
	pace := 0.0
	if t.GoalID != nil {
		if p, ok := progress[*t.GoalID]; ok && p.RequiredPerDay > 0 {
			pace = clamp((p.RequiredPerDay-p.ActualPerDay)/p.RequiredPerDay, 0, 1)
		}
	}
	prio := float64(t.Priority-1) / 3
	age := clamp(float64(rolloverCount)/5, 0, 1)
	return WeightDue*due + WeightPace*pace + WeightPrio*prio + WeightAge*age
}

func clamp(x, lo, hi float64) float64 { return min(max(x, lo), hi) }

// sortByUrgency orders tasks by score, descending, breaking ties by due_day
// ascending with nulls last, then priority descending, then created_at, then id.
func sortByUrgency(tasks []Task, score map[string]float64) {
	slices.SortStableFunc(tasks, func(a, b Task) int {
		if c := cmp.Compare(score[b.ID], score[a.ID]); c != 0 {
			return c
		}
		ad, aok := a.due()
		bd, bok := b.due()
		switch {
		case aok && bok:
			if c := ad.Compare(bd); c != 0 {
				return c
			}
		case aok != bok:
			if aok {
				return -1
			}
			return 1
		}
		if c := cmp.Compare(b.Priority, a.Priority); c != 0 {
			return c
		}
		if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
}

// RemainingMinutes is the work left on a task: its estimate, 60 minutes when
// it has none, less all its tracked time, and never below 25.
func RemainingMinutes(t Task, tracked time.Duration) int {
	estimate := DefaultEstimate
	if t.Estimate != nil {
		estimate = int(*t.Estimate / time.Minute)
	}
	return max(estimate-int(tracked/time.Minute), MinBlock)
}

// RolloverCount is a task's rollover count on a day given its most recent
// earlier plan item: one more than that item's when it rolled, else 0. rolled
// is that item's id when it rolled.
func RolloverCount(latest *PlanItem) (count int, rolled *string) {
	if latest == nil || latest.Status != model.PlanRolled {
		return 0, nil
	}
	id := latest.ID
	return latest.RolloverCount + 1, &id
}
