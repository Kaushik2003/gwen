package planner

import (
	"cmp"
	"slices"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/planner/civil"
)

// StatusChange sets a plan item's status.
type StatusChange struct {
	ItemID string
	Day    string
	Status string
}

// RolloverResult is what Rollover decides.
type RolloverResult struct {
	Changes []StatusChange
	// Expire lists the expired occurrences to soft-delete.
	Expire []string
}

// Rollover settles the planned items of days before today
// (docs/06-planner.md#rollover). items may hold any plan items; only live,
// planned ones before today are considered. tasks maps task ids to tasks,
// deleted ones included; it must hold every task the items reference and
// every live open occurrence, which expire once today is past their due day.
func Rollover(items []PlanItem, tasks map[string]Task, today civil.Day) RolloverResult {
	var res RolloverResult
	expired := func(t Task) bool {
		if !t.live() || !t.open() || !t.isOccurrence() {
			return false
		}
		d, ok := t.due()
		return ok && d.Before(today)
	}
	for _, it := range items {
		if it.DeletedAt != nil || it.Status != model.PlanPlanned || !civil.MustParse(it.Day).Before(today) {
			continue
		}
		t, ok := tasks[it.TaskID]
		status := model.PlanRolled
		switch {
		case ok && t.live() && t.Status == model.TaskDone:
			status = model.PlanDone
		case !ok || !t.live() || expired(t):
			status = model.PlanSkipped
		}
		res.Changes = append(res.Changes, StatusChange{ItemID: it.ID, Day: it.Day, Status: status})
	}
	for id, t := range tasks {
		if expired(t) {
			res.Expire = append(res.Expire, id)
		}
	}
	slices.Sort(res.Expire)
	return res
}

// Pending is, for each open live task, its most recent rolled item, provided
// the task has no item on a later day. items are live plan items; the result
// is ordered by day, then position.
func Pending(items []PlanItem, tasks map[string]Task) []PlanItem {
	latest := map[string]PlanItem{}
	for _, it := range items {
		if it.DeletedAt != nil {
			continue
		}
		cur, ok := latest[it.TaskID]
		better := !ok || it.Day > cur.Day
		if ok && it.Day == cur.Day && it.Status == model.PlanRolled {
			better = cur.Status != model.PlanRolled || it.Position < cur.Position
		}
		if better {
			latest[it.TaskID] = it
		}
	}
	out := []PlanItem{}
	for id, it := range latest {
		t, ok := tasks[id]
		if ok && t.live() && t.open() && it.Status == model.PlanRolled {
			out = append(out, it)
		}
	}
	sortItems(out)
	return out
}

func sortItems(items []PlanItem) {
	slices.SortFunc(items, func(a, b PlanItem) int {
		return cmp.Or(cmp.Compare(a.Day, b.Day), cmp.Compare(a.Position, b.Position), cmp.Compare(a.ID, b.ID))
	})
}

// GenerateInput is everything Generate needs for day Day.
type GenerateInput struct {
	Day      civil.Day
	Loc      *time.Location // places tasks' start times on Day
	Capacity CapacityResult
	// Existing are the live items on Day whose task is live.
	Existing []PlanItem
	// Tasks are the live open tasks; Generate builds the pool from them.
	Tasks    []Task
	Tracked  map[string]time.Duration // all-time tracked work per task
	Progress map[string]GoalProgress  // active goals
	// Latest is each task's most recent live item on a day before Day.
	Latest map[string]PlanItem
	// Goals are the live goals, which tell items and sessions apart.
	Goals map[string]Goal
	// Elsewhere are the tasks pinned to another day from today on: the user
	// put them there, so this day leaves them alone.
	Elsewhere map[string]bool
	// Frog places the hardest work first (eat the frog).
	Frog bool
	// Prime is the day's biological prime time, minutes after midnight, or
	// nil: hard work goes there first.
	Prime *[2]int
}

// PlanItemDraft is an item of a generated plan. A kept item carries its id
// and changes only Position; a new one has ExistingID "".
type PlanItemDraft struct {
	ExistingID    string
	TaskID        string
	Planned       time.Duration
	StartAt       *time.Time
	Position      int
	RolledFromID  *string
	RolloverCount int
	notBefore     time.Time // its task's start time on the day, when it has one
	effort        int       // its task's effort, medium when unrated
}

// Keep reports whether regenerating keeps an item: pinned, or no longer planned.
func Keep(it PlanItem) bool { return it.Pinned || it.Status != model.PlanPlanned }

// Generate builds the plan for in.Day (docs/06-planner.md#generating-a-plan),
// steps 2 to 9: the kept items followed by the new ones, in position order.
// An existing item missing from the result is soft-deleted by the caller.
func Generate(in GenerateInput) []PlanItemDraft {
	var kept []PlanItem
	keptTasks := map[string]bool{}
	capacityLeft := in.Capacity.Minutes
	free := in.Capacity.Free
	for _, it := range in.Existing {
		if !Keep(it) {
			continue
		}
		kept = append(kept, it)
		keptTasks[it.TaskID] = true
		if it.Pinned && it.Status == model.PlanPlanned {
			capacityLeft -= int(it.Planned / time.Minute)
		}
		if it.Pinned && it.StartAt != nil {
			free = subtract(free, Interval{*it.StartAt, it.StartAt.Add(held(it.Planned))})
		}
	}

	var fresh []PlanItemDraft
	for _, c := range Candidates(in, keptTasks) {
		t := c.Task
		remaining := RemainingMinutes(t, in.Tracked[t.ID])
		blocks, maxBlock := BlocksPerTask, MaxBlock
		var notBefore time.Time
		if minute, ok := t.startOn(in.Day); ok {
			blocks, maxBlock, notBefore = 1, remaining, in.Day.At(minute, in.Loc) // the time the user chose
		} else if timedSession(t, in.Goals) {
			blocks, maxBlock = 1, remaining // the time a day the user chose, in one block
		}
		for n := 0; n < blocks && remaining > 0 && capacityLeft >= MinBlock; n++ {
			block := min(remaining, maxBlock, capacityLeft) / BlockStep * BlockStep
			if block < MinBlock {
				break
			}
			fresh = append(fresh, PlanItemDraft{TaskID: t.ID, Planned: time.Duration(block) * time.Minute,
				RolloverCount: c.RolloverCount, RolledFromID: c.RolledFromID, notBefore: notBefore,
				effort: effortOf(t.Task)})
			remaining -= block
			capacityLeft -= block
		}
	}

	slices.SortStableFunc(kept, func(a, b PlanItem) int {
		switch {
		case a.StartAt != nil && b.StartAt != nil:
			if c := a.StartAt.Compare(*b.StartAt); c != 0 {
				return c
			}
		case a.StartAt != nil:
			return -1
		case b.StartAt != nil:
			return 1
		}
		return cmp.Or(cmp.Compare(a.Position, b.Position), cmp.Compare(a.ID, b.ID))
	})
	out := make([]PlanItemDraft, 0, len(kept)+len(fresh))
	for _, it := range kept {
		out = append(out, PlanItemDraft{ExistingID: it.ID, TaskID: it.TaskID, Planned: it.Planned, StartAt: it.StartAt,
			Position: len(out), RolledFromID: it.RolledFromID, RolloverCount: it.RolloverCount})
	}
	if in.Frog || in.Prime != nil {
		// Eat the frog: after the work with a chosen time, the hardest first.
		slices.SortStableFunc(fresh, func(a, b PlanItemDraft) int {
			timed := func(d PlanItemDraft) int {
				if d.notBefore.IsZero() {
					return 1
				}
				return 0
			}
			return cmp.Or(cmp.Compare(timed(a), timed(b)), cmp.Compare(b.effort, a.effort))
		})
	}
	var prime []Interval
	if in.Prime != nil {
		prime = within(free, Interval{in.Day.At(in.Prime[0], in.Loc), in.Day.At(in.Prime[1], in.Loc)})
	}
	for _, d := range fresh {
		d.Position = len(out)
		if d.effort == model.EffortHard && prime != nil {
			// Hard work takes prime time when it fits there.
			if at, ok := earliestFit(prime, d.Planned, d.notBefore); ok {
				d.StartAt = &at
				cut := Interval{at, at.Add(held(d.Planned))}
				free, prime = subtract(free, cut), subtract(prime, cut)
				out = append(out, d)
				continue
			}
		}
		if at, ok := earliestFit(free, d.Planned, d.notBefore); ok {
			d.StartAt = &at
			cut := Interval{at, at.Add(held(d.Planned))}
			free = subtract(free, cut)
			if prime != nil {
				prime = subtract(prime, cut)
			}
		}
		out = append(out, d)
	}
	return out
}

// Candidate is a task of a day's pool, with its urgency and rollover fields.
type Candidate struct {
	Task          Task
	Urgency       float64
	RolloverCount int
	RolledFromID  *string
}

// Candidates is the pool of steps 4 to 6 of
// docs/06-planner.md#generating-a-plan for in.Day, most urgent first after
// the tasks with a start time on the day, without the tasks in exclude.
// To-dos are never candidates.
func Candidates(in GenerateInput, exclude map[string]bool) []Candidate {
	pool := []Task{}
	byID := map[string]Candidate{}
	score := map[string]float64{}
	for _, t := range in.Tasks {
		if !t.live() || !t.open() || t.IsTemplate() || t.IsTodo() || exclude[t.ID] || !ownPlan(t.Task, in.Goals) ||
			!t.Plannable() || in.Elsewhere[t.ID] {
			continue
		}
		if t.startsAfter(in.Day) {
			continue
		}
		if t.isOccurrence() {
			if civil.MustParse(*t.OccurrenceDay).After(in.Day) {
				continue
			}
			if d, ok := t.due(); ok && d.Before(in.Day) {
				continue
			}
		}
		var latest *PlanItem
		if it, ok := in.Latest[t.ID]; ok {
			latest = &it
		}
		c := Candidate{Task: t}
		c.RolloverCount, c.RolledFromID = RolloverCount(latest)
		c.Urgency = Urgency(t, in.Day, in.Progress, c.RolloverCount)
		byID[t.ID], score[t.ID] = c, c.Urgency
		pool = append(pool, t)
	}
	sortByUrgency(pool, score)
	slices.SortStableFunc(pool, func(a, b Task) int {
		am, aok := a.startOn(in.Day)
		bm, bok := b.startOn(in.Day)
		switch {
		case aok && bok:
			return cmp.Compare(am, bm)
		case aok:
			return -1
		case bok:
			return 1
		}
		return 0
	})
	out := make([]Candidate, len(pool))
	for i, t := range pool {
		out[i] = byID[t.ID]
	}
	return out
}

// effortOf is a task's effort, medium when unrated.
func effortOf(t model.Task) int {
	if t.Effort == nil {
		return model.EffortMedium
	}
	return *t.Effort
}

// within is the parts of free inside w.
func within(free []Interval, w Interval) []Interval {
	var out []Interval
	for _, f := range free {
		i := Interval{Start: maxTime(f.Start, w.Start), End: minTime(f.End, w.End)}
		if !i.empty() {
			out = append(out, i)
		}
	}
	return out
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// timedSession reports whether t is a session of a goal with daily minutes.
func timedSession(t Task, goals map[string]Goal) bool {
	if !t.isOccurrence() || t.GoalID == nil {
		return false
	}
	g, ok := goals[*t.GoalID]
	return ok && g.Kind == model.GoalQuantity && g.Daily != nil
}

// held is how long a block occupies its slot: a block of 90 minutes, or a
// longer one the user set, also holds the 10-minute gap after it.
func held(planned time.Duration) time.Duration {
	if planned >= MaxBlock*time.Minute {
		return planned + LongBlockGap
	}
	return planned
}
