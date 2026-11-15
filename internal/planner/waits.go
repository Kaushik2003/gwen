package planner

import (
	"slices"
	"time"
)

// Dependencies (plan.md 3.17): a task can wait on other tasks. The planner
// keeps a task off the day while something it waits on is open and not on
// the plan too, orders it after what it waits on, and starts it no earlier
// than their last block ends. A task waited on that is done, deleted, or
// missing blocks nothing.

// openWaits is, for each open task, the open tasks among tasks it waits on.
func openWaits(tasks []Task) map[string][]string {
	open := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		if t.live() && t.open() {
			open[t.ID] = true
		}
	}
	out := map[string][]string{}
	for _, t := range tasks {
		if !open[t.ID] {
			continue
		}
		for _, w := range t.WaitsOn {
			if open[w] && w != t.ID {
				out[t.ID] = append(out[t.ID], w)
			}
		}
	}
	return out
}

// afterWaits moves each new block after every block of the tasks it waits
// on, keeping the order otherwise. Kept items stay where they are.
func afterWaits(seq []PlanItemDraft, waits map[string][]string) []PlanItemDraft {
	if len(waits) == 0 {
		return seq
	}
	left := map[string]int{}
	for _, d := range seq {
		left[d.TaskID]++
	}
	ready := func(d PlanItemDraft) bool {
		if d.ExistingID != "" {
			return true
		}
		for _, w := range waits[d.TaskID] {
			if left[w] > 0 {
				return false
			}
		}
		return true
	}
	out := make([]PlanItemDraft, 0, len(seq))
	var pending []PlanItemDraft
	emit := func(d PlanItemDraft) {
		out = append(out, d)
		left[d.TaskID]--
		// Whatever waited on it may be free now; earliest first.
		for moved := true; moved; {
			moved = false
			for i, p := range pending {
				if ready(p) {
					pending = slices.Delete(pending, i, i+1)
					out = append(out, p)
					left[p.TaskID]--
					moved = true
					break
				}
			}
		}
	}
	for _, d := range seq {
		if ready(d) {
			emit(d)
		} else {
			pending = append(pending, d)
		}
	}
	return append(out, pending...) // only a loop leaves any, and the store refuses loops
}

// waitsEnd is the latest end of the blocks placed so far of the tasks id
// waits on, or the zero time.
func waitsEnd(ends map[string]time.Time, waits []string) time.Time {
	var at time.Time
	for _, w := range waits {
		if e := ends[w]; e.After(at) {
			at = e
		}
	}
	return at
}
