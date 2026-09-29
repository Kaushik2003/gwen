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
	Capacity CapacityResult
	// Existing are the live items on Day whose task is live.
	Existing []PlanItem
	// Tasks are the live open tasks; Generate builds the pool from them.
	Tasks    []Task
	Tracked  map[string]time.Duration // all-time tracked work per task
	Progress map[string]GoalProgress  // active goals
	// Latest is each task's most recent live item on a day before Day.
	Latest map[string]PlanItem
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

	pool := []Task{}
	counts := map[string]int{}
	rolledFrom := map[string]*string{}
	score := map[string]float64{}
	for _, t := range in.Tasks {
		if !t.live() || !t.open() || t.IsTemplate() || keptTasks[t.ID] {
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
		counts[t.ID], rolledFrom[t.ID] = RolloverCount(latest)
		score[t.ID] = Urgency(t, in.Day, in.Progress, counts[t.ID])
		pool = append(pool, t)
	}
	sortByUrgency(pool, score)

	var fresh []PlanItemDraft
	for _, t := range pool {
		remaining := RemainingMinutes(t, in.Tracked[t.ID])
		for blocks := 0; blocks < BlocksPerTask && remaining > 0 && capacityLeft >= MinBlock; blocks++ {
			block := min(remaining, MaxBlock, capacityLeft) / BlockStep * BlockStep
			if block < MinBlock {
				break
			}
			fresh = append(fresh, PlanItemDraft{TaskID: t.ID, Planned: time.Duration(block) * time.Minute,
				RolloverCount: counts[t.ID], RolledFromID: rolledFrom[t.ID]})
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
	for _, d := range fresh {
		d.Position = len(out)
		if at, ok := earliestFit(free, d.Planned); ok {
			d.StartAt = &at
			free = subtract(free, Interval{at, at.Add(held(d.Planned))})
		}
		out = append(out, d)
	}
	return out
}

// held is how long a block occupies its slot: a block of 90 minutes, or a
// longer one the user set, also holds the 10-minute gap after it.
func held(planned time.Duration) time.Duration {
	if planned >= MaxBlock*time.Minute {
		return planned + LongBlockGap
	}
	return planned
}
