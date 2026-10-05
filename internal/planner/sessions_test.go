package planner_test

import (
	"testing"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/planner"
	"github.com/stretchr/testify/require"
)

// timedGoal is the second worked example of
// docs/06-planner.md#goals-and-progress: 300 problems, 20 min each, 2 h a
// day, 2026-10-05 to 2026-11-10.
func timedGoal() planner.Goal {
	g := quantityGoal()
	g.PerUnit, g.Daily = ptr(mins(20)), ptr(mins(120))
	g.StartDay, g.DueDay = "2026-10-05", "2026-11-10"
	return g
}

func dailyTemplate() planner.Task {
	tmpl := sessionTemplate("FREQ=DAILY")
	tmpl.Anchor = day("2026-10-05")
	return tmpl
}

func TestProgressTimedWorkedExample(t *testing.T) {
	t.Parallel()
	g := timedGoal()
	p := planner.Progress(g, []planner.Task{dailyTemplate()}, day("2026-10-05"))
	require.Equal(t, 37, p.SessionsLeft)
	require.Equal(t, 6, *p.PerSession)
	require.Equal(t, 222, p.Reachable)
	require.Equal(t, 180, *p.NeededDaily, "ceil(300/37) × 20")
	require.Equal(t, day("2026-11-23"), *p.NeededDue, "the 50th session")
	q, ok := planner.SessionQuantity(g, p, day("2026-10-05"))
	require.True(t, ok)
	require.Equal(t, 6, q, "capped at what 2 h holds")

	g.Daily = ptr(mins(180))
	p = planner.Progress(g, []planner.Task{dailyTemplate()}, day("2026-10-05"))
	require.Equal(t, 300, p.Reachable, "3 h a day fits")
	require.Nil(t, p.NeededDaily)
	require.Nil(t, p.NeededDue)

	g.Daily = nil
	p = planner.Progress(g, []planner.Task{dailyTemplate()}, day("2026-10-05"))
	require.Nil(t, p.PerSession)
	require.Equal(t, 300, p.Reachable, "without a daily time nothing is capped")
}

func TestProgressLinedUp(t *testing.T) {
	t.Parallel()
	g := timedGoal()
	session := task("session", 1, forGoal("g"), func(t *planner.Task) { t.TemplateID = ptr("tmpl") })
	tasks := []planner.Task{
		dailyTemplate(), session,
		task("item", 2, forGoal("g")),
		task("pair", 3, forGoal("g"), func(t *planner.Task) { t.Quantity = ptr(2) }),
		task("step", 4, forGoal("g"), func(t *planner.Task) { t.ParentID = ptr("session") }),
		task("done step", 5, forGoal("g"), done(ptr(1)), func(t *planner.Task) { t.ParentID = ptr("session") }),
	}
	p := planner.Progress(g, tasks, day("2026-10-05"))
	require.Equal(t, 4, p.LinedUp, "open items and steps, not the session")
	require.Equal(t, 1, p.Done)
}

func TestSessions(t *testing.T) {
	t.Parallel()
	g := timedGoal()
	tmpl := dailyTemplate()
	p := planner.Progress(g, []planner.Task{tmpl}, day("2026-10-05"))

	slots := planner.Sessions(g, tmpl, p, day("2026-10-05"), 0, 3)
	require.Len(t, slots, 3)
	for i, s := range slots {
		require.Equal(t, day("2026-10-05").AddDays(i), s.Day)
		require.Equal(t, 6, s.Units)
		require.Equal(t, 120, s.Minutes)
	}

	slots = planner.Sessions(g, tmpl, p, day("2026-10-05"), 8, 3)
	require.Equal(t, []planner.SessionSlot{
		{Day: day("2026-10-06"), Units: 4, Minutes: 80},
		{Day: day("2026-10-07"), Units: 6, Minutes: 120},
		{Day: day("2026-10-08"), Units: 6, Minutes: 120},
	}, slots, "8 lined up fill the first session and part of the second")

	g.TargetQuantity = ptr(10)
	p = planner.Progress(g, []planner.Task{tmpl}, day("2026-10-05"))
	slots = planner.Sessions(g, tmpl, p, day("2026-10-05"), 0, 14)
	units := 0
	for _, s := range slots {
		units += s.Units
	}
	require.Equal(t, 10, units, "the slots stop once the remaining units are covered")
}

func TestGenerateSessionsAndSteps(t *testing.T) {
	t.Parallel()
	g := timedGoal()
	g.ID = "g"
	goals := map[string]planner.Goal{"g": g}
	session := task("session", 1, forGoal("g"), estimate(120), func(t *planner.Task) {
		t.TemplateID, t.OccurrenceDay = ptr("tmpl"), ptr("2026-09-15")
	})
	parent := task("parent", 2, estimate(60))
	step := task("step", 3, estimate(30), func(t *planner.Task) { t.ParentID = ptr("parent") })
	item := task("item", 4, forGoal("g"), estimate(20))
	drafts := planner.Generate(planner.GenerateInput{
		Day: slotDay(), Capacity: planner.CapacityResult{Minutes: 480, Free: free("09:00", "23:00")},
		Tasks: []planner.Task{session, parent, step, item}, Goals: goals,
	})
	byTask := map[string][]time.Duration{}
	for _, d := range drafts {
		byTask[d.TaskID] = append(byTask[d.TaskID], d.Planned)
	}
	require.Equal(t, []time.Duration{mins(120)}, byTask["session"], "one block of the daily time, past the 90 cap")
	require.Equal(t, []time.Duration{mins(60)}, byTask["parent"])
	require.NotContains(t, byTask, "step", "steps are planned through their parent")
	require.NotContains(t, byTask, "item", "items are planned through sessions")

	delete(goals, "g")
	drafts = planner.Generate(planner.GenerateInput{
		Day: slotDay(), Capacity: planner.CapacityResult{Minutes: 480, Free: free("09:00", "23:00")},
		Tasks: []planner.Task{item}, Goals: goals,
	})
	require.Len(t, drafts, 1, "once its goal is gone an item is an ordinary task")
}

func TestIsItem(t *testing.T) {
	t.Parallel()
	goals := map[string]planner.Goal{"q": {ID: "q", Kind: model.GoalQuantity}, "t": {ID: "t", Kind: model.GoalTasks}}
	for _, tc := range []struct {
		name string
		task model.Task
		want bool
	}{
		{"item", model.Task{GoalID: ptr("q")}, true},
		{"tasks goal", model.Task{GoalID: ptr("t")}, false},
		{"no goal", model.Task{}, false},
		{"step", model.Task{GoalID: ptr("q"), ParentID: ptr("p")}, false},
		{"session", model.Task{GoalID: ptr("q"), TemplateID: ptr("x")}, false},
		{"template", model.Task{GoalID: ptr("q"), RRule: ptr("FREQ=DAILY")}, false},
		{"deleted goal", model.Task{GoalID: ptr("gone")}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, planner.IsItem(tc.task, goals))
		})
	}
}
