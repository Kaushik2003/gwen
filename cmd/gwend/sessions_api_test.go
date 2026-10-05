package main

import (
	"testing"

	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// W15: goal preview, deletes that cascade, and tracking that carries on
// when its task or project is deleted.

func TestGoalPreview(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	// The second worked example of docs/06-planner.md#goals-and-progress, on
	// a daemon whose today is 2026-09-15.
	p, err := d.c.PreviewGoal(d.ctx, wire.GoalPreviewRequest{Kind: wire.GoalQuantity,
		TargetQuantity: testutil.Ptr(300), MinutesPerUnit: testutil.Ptr(20), DailyMinutes: testutil.Ptr(120),
		StartDay: testutil.Day0, DueDay: "2026-10-21"})
	require.NoError(t, err)
	require.Equal(t, 6, *p.PerSession)
	require.Equal(t, 222, p.ReachableQuantity)
	require.Equal(t, 180, *p.NeededDailyMinutes)
	require.Equal(t, "2026-11-03", *p.NeededDueDay)

	fits, err := d.c.PreviewGoal(d.ctx, wire.GoalPreviewRequest{Kind: wire.GoalQuantity,
		TargetQuantity: testutil.Ptr(300), MinutesPerUnit: testutil.Ptr(20), DailyMinutes: testutil.Ptr(180),
		StartDay: testutil.Day0, DueDay: "2026-10-21"})
	require.NoError(t, err)
	require.Equal(t, 300, fits.ReachableQuantity)
	require.Nil(t, fits.NeededDailyMinutes)
	require.Nil(t, fits.NeededDueDay)

	_, err = d.c.PreviewGoal(d.ctx, wire.GoalPreviewRequest{Kind: wire.GoalQuantity,
		TargetQuantity: testutil.Ptr(300), StartDay: testutil.Day0, DueDay: "2026-10-21"})
	ae := apiErr(t, err, wire.CodeInvalidRequest)
	require.Equal(t, "minutes_per_unit", ae.Details["field"])
	_, err = d.c.PreviewGoal(d.ctx, wire.GoalPreviewRequest{GoalID: testutil.Ptr("nope"), Kind: wire.GoalTasks,
		StartDay: testutil.Day0, DueDay: "2026-10-21"})
	apiErr(t, err, wire.CodeNotFound)
}

func TestDeletingTheTrackedTaskCarriesOn(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	p := d.project("P")
	tk, err := d.c.CreateTask(d.ctx, wire.CreateTaskRequest{ProjectID: &p.ID, Title: "T"})
	require.NoError(t, err)
	step, err := d.c.CreateTask(d.ctx, wire.CreateTaskRequest{Title: "step", ParentID: &tk.ID})
	require.NoError(t, err)
	require.Equal(t, p.ID, *step.ProjectID, "a step lives in its parent's project")
	_, err = d.c.ClockIn(d.ctx, wire.ClockInRequest{TaskID: &tk.ID})
	require.NoError(t, err)

	require.NoError(t, d.c.DeleteTask(d.ctx, tk.ID))
	st := d.status()
	require.Equal(t, "working", st.State)
	require.Equal(t, p.ID, *st.ProjectID, "still on the project")
	require.Nil(t, st.TaskID)
	_, err = d.c.GetTask(d.ctx, step.ID)
	apiErr(t, err, wire.CodeNotFound)

	require.NoError(t, d.c.DeleteProject(d.ctx, p.ID))
	st = d.status()
	require.Equal(t, "working", st.State)
	require.Nil(t, st.ProjectID, "unassigned once the project is gone")
}

func TestBulkDeleteIsAllOrNothing(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	a, err := d.c.CreateTask(d.ctx, wire.CreateTaskRequest{Title: "a"})
	require.NoError(t, err)
	b, err := d.c.CreateTask(d.ctx, wire.CreateTaskRequest{Title: "b"})
	require.NoError(t, err)
	step, err := d.c.CreateTask(d.ctx, wire.CreateTaskRequest{Title: "b.1", ParentID: &b.ID})
	require.NoError(t, err)

	_, err = d.c.DeleteTasks(d.ctx, wire.DeleteTasksRequest{IDs: []string{a.ID, "nope"}})
	apiErr(t, err, wire.CodeNotFound)
	_, err = d.c.GetTask(d.ctx, a.ID)
	require.NoError(t, err, "nothing went")
	_, err = d.c.DeleteTasks(d.ctx, wire.DeleteTasksRequest{})
	apiErr(t, err, wire.CodeInvalidRequest)

	out, err := d.c.DeleteTasks(d.ctx, wire.DeleteTasksRequest{IDs: []string{a.ID, b.ID, step.ID}})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{a.ID, b.ID, step.ID}, out.TaskIDs)
	left, err := d.c.ListTasks(d.ctx, wire.TaskQuery{Status: "all"})
	require.NoError(t, err)
	require.Empty(t, left.Tasks)
}

func TestDeleteGoalWithItsTasks(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	g, err := d.c.CreateGoal(d.ctx, wire.CreateGoalRequest{Title: "DSA", Kind: wire.GoalQuantity,
		TargetQuantity: testutil.Ptr(10), MinutesPerUnit: testutil.Ptr(20), DailyMinutes: testutil.Ptr(60),
		StartDay: testutil.Day0, DueDay: "2026-09-20"})
	require.NoError(t, err)
	require.Equal(t, 60, *g.DailyMinutes)
	for _, title := range []string{"Two Sum", "Valid Anagram"} {
		_, err := d.c.CreateTask(d.ctx, wire.CreateTaskRequest{Title: title, GoalID: &g.ID})
		require.NoError(t, err)
	}
	got, err := d.c.GetGoal(d.ctx, g.ID)
	require.NoError(t, err)
	require.Equal(t, 2, got.Progress.LinedUp)
	plan, err := d.c.GetPlan(d.ctx, "")
	require.NoError(t, err)
	require.Len(t, plan.Items, 1, "one session, holding the items")
	require.Len(t, plan.Items[0].Steps, 2)

	_, err = d.c.ListGoals(d.ctx, "")
	require.NoError(t, err)
	require.NoError(t, d.c.DeleteGoal(d.ctx, g.ID, true))
	left, err := d.c.ListTasks(d.ctx, wire.TaskQuery{Status: "all", Templates: true})
	require.NoError(t, err)
	require.Empty(t, left.Tasks)
	plan, err = d.c.GetPlan(d.ctx, "")
	require.NoError(t, err)
	require.Empty(t, plan.Items)
}
