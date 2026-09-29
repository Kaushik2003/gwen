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

	if !today.After(due) {
		if g.Kind == model.GoalTasks {
			p.SessionsLeft = today.DaysUntil(due) + 1
		} else if template != nil {
			if r, err := ParseRule(*template.RRule); err == nil {
				p.SessionsLeft = len(r.Between(template.Anchor, today, due))
			}
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
	return p
}

// SessionQuantity is the quantity of g's session occurrence on day, from the
// progress computed with today = day, and false when no occurrence is due
// (docs/06-planner.md#goals-and-progress).
func SessionQuantity(g Goal, p GoalProgress, day civil.Day) (int, bool) {
	if g.Status != model.GoalActive || p.Remaining == 0 || day.Before(civil.MustParse(g.StartDay)) {
		return 0, false
	}
	return max(1, int(math.Ceil(p.RequiredPerDay))), true
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
