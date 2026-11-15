package planner_test

import (
	"testing"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/planner"
	"github.com/stretchr/testify/require"
)

func waitsOn(ids ...string) func(*planner.Task) { return func(t *planner.Task) { t.WaitsOn = ids } }

func TestGenerateWaits(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		capacity int
		frog     bool
		tasks    []planner.Task
		existing []planner.PlanItem
		want     []block
	}{
		{"a task comes after what it waits on, though more urgent", 600,
			false,
			[]planner.Task{task("app", 0, prio(4), waitsOn("api")), task("api", 1, prio(1))}, nil,
			[]block{{task: "api", minutes: 60, start: "09:00"}, {task: "app", minutes: 60, start: "10:00"}}},
		{"it starts after every block of what it waits on", 600, false,
			[]planner.Task{task("app", 0, prio(4), waitsOn("api")), task("api", 1, estimate(180)), task("docs", 2, estimate(30))},
			nil,
			[]block{{task: "api", minutes: 90, start: "09:00"}, {task: "api", minutes: 90, start: "10:40"},
				{task: "app", minutes: 60, start: "12:20"}, {task: "docs", minutes: 30, start: "13:20"}}},
		{"a chain keeps its order", 600, false,
			[]planner.Task{task("c", 0, prio(4), waitsOn("b")), task("b", 1, prio(3), waitsOn("a")), task("a", 2, prio(1))},
			nil,
			[]block{{task: "a", minutes: 60, start: "09:00"}, {task: "b", minutes: 60, start: "10:00"},
				{task: "c", minutes: 60, start: "11:00"}}},
		{"done work blocks nothing", 600, false,
			[]planner.Task{task("app", 0, waitsOn("api")), task("api", 1, done(nil))}, nil,
			[]block{{task: "app", minutes: 60, start: "09:00"}}},
		{"an open task off the plan keeps what waits on it off too", 600, false,
			[]planner.Task{task("app", 0, waitsOn("review")), task("review", 1, todo()), task("docs", 2)}, nil,
			[]block{{task: "docs", minutes: 60, start: "09:00"}}},
		{"when room runs out before the blocker, the waiting task goes too", 60, false,
			[]planner.Task{task("app", 0, prio(4), waitsOn("api")), task("api", 1, prio(1), estimate(60)), task("x", 2, prio(4))},
			nil,
			[]block{{task: "x", minutes: 60, start: "09:00"}}},
		{"eating the frog still keeps a hard task after what it waits on", 600, true,
			[]planner.Task{
				task("hard", 0, waitsOn("easy"), func(t *planner.Task) { t.Effort = ptr(model.EffortHard) }),
				task("easy", 1, func(t *planner.Task) { t.Effort = ptr(model.EffortEasy) }),
			}, nil,
			[]block{{task: "easy", minutes: 60, start: "09:00"}, {task: "hard", minutes: 60, start: "10:00"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := planner.Generate(planner.GenerateInput{
				Day: slotDay(), Capacity: planner.CapacityResult{Minutes: tc.capacity, Free: free("09:00", "23:00")},
				Tasks: tc.tasks, Existing: tc.existing, Frog: tc.frog,
			})
			require.Equal(t, tc.want, blocks(got))
		})
	}
}

func TestGenerateWaitsOnAPinnedBlock(t *testing.T) {
	t.Parallel()
	pinnedAt := at("13:00")
	existing := []planner.PlanItem{item("pinned", "api", "2026-09-15", model.PlanPlanned, func(it *planner.PlanItem) {
		it.Pinned, it.StartAt, it.Planned = true, &pinnedAt, mins(60)
	})}
	got := planner.Generate(planner.GenerateInput{
		Day: slotDay(), Capacity: planner.CapacityResult{Minutes: 600, Free: free("09:00", "23:00")},
		Tasks: []planner.Task{task("api", 0), task("app", 1, waitsOn("api"))}, Existing: existing,
	})
	require.Equal(t, []block{
		{task: "api", minutes: 60, start: "13:00", kept: "pinned"},
		{task: "app", minutes: 60, start: "14:00"},
	}, blocks(got))
}
