package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/activity"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/planner"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// Every v2 endpoint's happy and error paths. The daemon's clock reads
// 2026-09-15 08:00 UTC, before the 09:00 planning window opens.

func (n *fakeNotifier) bodies() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	var out []string
	for _, x := range n.sent {
		out = append(out, x.Body)
	}
	return out
}

func TestGoalEndpoints(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	next := stream(t, d)
	p := d.project("Study")
	g, err := d.c.CreateGoal(d.ctx, wire.CreateGoalRequest{Title: "300 problems", Kind: wire.GoalQuantity,
		Unit: testutil.Ptr("problems"), TargetQuantity: testutil.Ptr(300), MinutesPerUnit: testutil.Ptr(30),
		ProjectID: &p.ID, StartDay: testutil.Day0, DueDay: "2026-12-15"})
	require.NoError(t, err)
	require.Equal(t, wire.GoalActive, g.Status)
	require.Equal(t, 30, *g.MinutesPerUnit)
	require.Equal(t, 300, g.Progress.RemainingQuantity)
	require.InDelta(t, 300.0/92, g.Progress.RequiredPerDay, 1e-9)
	require.Equal(t, wire.PaceBehind, g.Progress.Pace, "day 1 expects 300/92 already")
	require.Nil(t, g.Progress.ProjectedFinishDay)
	var tc wire.TasksChanged
	require.NoError(t, json.Unmarshal(next(wire.EventTasksChanged).Data, &tc))
	require.Len(t, tc.TaskIDs, 1, "the session template")
	next(wire.EventGoalsChanged)

	got, err := d.c.GetGoal(d.ctx, g.ID)
	require.NoError(t, err)
	require.Equal(t, *g, *got)
	list, err := d.c.ListGoals(d.ctx, "")
	require.NoError(t, err)
	require.Len(t, list.Goals, 1)

	up, err := d.c.PatchGoal(d.ctx, g.ID, wire.PatchGoalRequest{Title: testutil.Ptr("250 problems"),
		TargetQuantity: wire.Some(250), Rev: &g.Rev})
	require.NoError(t, err)
	require.Equal(t, 250, up.Progress.RemainingQuantity)
	next(wire.EventGoalsChanged)

	templates, err := d.c.ListTasks(d.ctx, wire.TaskQuery{GoalID: g.ID, Templates: true})
	require.NoError(t, err)
	require.Len(t, templates.Tasks, 2, "the session template, and today's session, planned as the goal was made")
	plain, err := d.c.ListTasks(d.ctx, wire.TaskQuery{GoalID: g.ID})
	require.NoError(t, err)
	require.Len(t, plain.Tasks, 1)
	require.Equal(t, testutil.Day0, *plain.Tasks[0].OccurrenceDay)
	for _, tk := range templates.Tasks {
		if tk.ID != plain.Tasks[0].ID {
			require.Equal(t, "FREQ=DAILY", *tk.RRule)
		}
	}

	require.NoError(t, d.c.DeleteGoal(d.ctx, g.ID, false))
	next(wire.EventGoalsChanged)
	_, err = d.c.GetGoal(d.ctx, g.ID)
	apiErr(t, err, wire.CodeNotFound)

	t.Run("errors", func(t *testing.T) {
		_, err := d.c.CreateGoal(d.ctx, wire.CreateGoalRequest{Title: "x", Kind: wire.GoalQuantity,
			StartDay: testutil.Day0, DueDay: testutil.Day0})
		require.Equal(t, "target_quantity", apiErr(t, err, wire.CodeInvalidRequest).Details["field"])
		_, err = d.c.ListGoals(d.ctx, "paused")
		apiErr(t, err, wire.CodeInvalidRequest)
		_, err = d.c.PatchGoal(d.ctx, "nope", wire.PatchGoalRequest{})
		apiErr(t, err, wire.CodeNotFound)
		tg, err := d.c.CreateGoal(d.ctx, wire.CreateGoalRequest{Title: "Thesis", Kind: wire.GoalTasks,
			StartDay: testutil.Day0, DueDay: "2026-10-01"})
		require.NoError(t, err)
		_, err = d.c.PatchGoal(d.ctx, tg.ID, wire.PatchGoalRequest{Kind: testutil.Ptr(wire.GoalQuantity)})
		require.Equal(t, "kind", apiErr(t, err, wire.CodeInvalidRequest).Details["field"])
		_, err = d.c.PatchGoal(d.ctx, tg.ID, wire.PatchGoalRequest{Rev: testutil.Ptr(int64(9))})
		apiErr(t, err, wire.CodeConflict)
		apiErr(t, d.c.DeleteGoal(d.ctx, "nope", false), wire.CodeNotFound)
		_, err = d.c.GetGoal(d.ctx, "nope")
		apiErr(t, err, wire.CodeNotFound)
	})
}

func TestCommitmentEndpoints(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	next := stream(t, d)
	c, err := d.c.CreateCommitment(d.ctx, wire.CreateCommitmentRequest{Title: "Internship",
		RRule: "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR", StartMinute: testutil.Ptr(600), DurationMinutes: 300,
		ActiveFrom: "2026-09-01"})
	require.NoError(t, err)
	require.True(t, c.CountsTowardTarget)
	require.Nil(t, c.ActiveUntil)
	next(wire.EventGoalsChanged)

	list, err := d.c.ListCommitments(d.ctx)
	require.NoError(t, err)
	require.Equal(t, []wire.Commitment{*c}, list.Commitments)

	up, err := d.c.PatchCommitment(d.ctx, c.ID, wire.PatchCommitmentRequest{StartMinute: wire.Null[int](),
		CountsTowardTarget: testutil.Ptr(false), Rev: &c.Rev})
	require.NoError(t, err)
	require.Nil(t, up.StartMinute)
	require.False(t, up.CountsTowardTarget)
	next(wire.EventGoalsChanged)

	require.NoError(t, d.c.DeleteCommitment(d.ctx, c.ID))
	next(wire.EventGoalsChanged)

	_, err = d.c.CreateCommitment(d.ctx, wire.CreateCommitmentRequest{Title: "x", RRule: "FREQ=DAILY;COUNT=2",
		DurationMinutes: 60, ActiveFrom: "2026-09-01"})
	require.Equal(t, "rrule", apiErr(t, err, wire.CodeInvalidRequest).Details["field"])
	_, err = d.c.PatchCommitment(d.ctx, c.ID, wire.PatchCommitmentRequest{})
	apiErr(t, err, wire.CodeNotFound)
	apiErr(t, d.c.DeleteCommitment(d.ctx, c.ID), wire.CodeNotFound)
}

func TestPlanEndpoints(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	next := stream(t, d)
	tk, err := d.c.CreateTask(d.ctx, wire.CreateTaskRequest{Title: "Report", DueDay: testutil.Ptr(testutil.Day0),
		EstimateMinutes: testutil.Ptr(120)})
	require.NoError(t, err)
	next(wire.EventTasksChanged)

	plan, err := d.c.GetPlan(d.ctx, "")
	require.NoError(t, err)
	require.Equal(t, testutil.Day0, plan.Day)
	require.Equal(t, 450, plan.CapacityMinutes, "min(480, 840) − 30")
	require.Equal(t, 120, plan.PlannedMinutes)
	require.Len(t, plan.Items, 2)
	require.Equal(t, 90, plan.Items[0].PlannedMinutes)
	require.Equal(t, wire.Millis(testutil.At("09:00")), *plan.Items[0].StartAt)
	require.Equal(t, tk.ID, plan.Items[0].Task.ID)
	require.JSONEq(t, `{"day":"2026-09-15"}`, string(next(wire.EventPlanChanged).Data), "generated as the task was added")

	item := plan.Items[1]
	moved, err := d.c.PatchPlanItem(d.ctx, item.ID, wire.PatchPlanItemRequest{
		StartAt: wire.Some(wire.Millis(testutil.At("15:00"))), Rev: &item.Rev})
	require.NoError(t, err)
	require.True(t, moved.Pinned)
	require.Equal(t, wire.Millis(testutil.At("15:00")), *moved.StartAt)
	next(wire.EventPlanChanged)
	plan, err = d.c.GetPlan(d.ctx, "")
	require.NoError(t, err)
	require.Len(t, plan.Items, 1, "the day follows the edit at once: the pinned block is the task's time")

	regen, err := d.c.GeneratePlan(d.ctx, wire.GeneratePlanRequest{Day: testutil.Day0})
	require.NoError(t, err)
	require.Len(t, regen.Items, 1, "the pinned block is kept and the task is not planned again")
	require.Equal(t, item.ID, regen.Items[0].ID)

	done, err := d.c.CompleteTask(d.ctx, tk.ID, wire.CompleteTaskRequest{})
	require.NoError(t, err)
	require.Equal(t, "done", done.Status)
	next(wire.EventPlanChanged)
	after, err := d.c.GetPlan(d.ctx, testutil.Day0)
	require.NoError(t, err)
	require.Equal(t, wire.PlanDone, after.Items[0].Status)

	future, err := d.c.GeneratePlan(d.ctx, wire.GeneratePlanRequest{Day: "2026-09-16"})
	require.NoError(t, err)
	require.Empty(t, future.Items)

	t.Run("errors", func(t *testing.T) {
		_, err := d.c.GetPlan(d.ctx, "tomorrow")
		require.Equal(t, "day", apiErr(t, err, wire.CodeInvalidRequest).Details["field"])
		_, err = d.c.GeneratePlan(d.ctx, wire.GeneratePlanRequest{Day: "2026-09-14"})
		require.Equal(t, "day", apiErr(t, err, wire.CodeInvalidRequest).Details["field"])
		_, err = d.c.GeneratePlan(d.ctx, wire.GeneratePlanRequest{})
		apiErr(t, err, wire.CodeInvalidRequest)
		_, err = d.c.PatchPlanItem(d.ctx, item.ID, wire.PatchPlanItemRequest{Status: testutil.Ptr(wire.PlanRolled)})
		require.Equal(t, "status", apiErr(t, err, wire.CodeInvalidRequest).Details["field"])
		_, err = d.c.PatchPlanItem(d.ctx, item.ID, wire.PatchPlanItemRequest{Rev: testutil.Ptr(int64(1))})
		apiErr(t, err, wire.CodeConflict)
		_, err = d.c.PatchPlanItem(d.ctx, "nope", wire.PatchPlanItemRequest{})
		apiErr(t, err, wire.CodeNotFound)
	})
}

func TestTaskV2Endpoints(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	g, err := d.c.CreateGoal(d.ctx, wire.CreateGoalRequest{Title: "5 katas", Kind: wire.GoalQuantity,
		TargetQuantity: testutil.Ptr(5), MinutesPerUnit: testutil.Ptr(10), StartDay: testutil.Day0, DueDay: "2026-09-30"})
	require.NoError(t, err)
	tk, err := d.c.CreateTask(d.ctx, wire.CreateTaskRequest{Title: "Katas", GoalID: &g.ID, Quantity: testutil.Ptr(5)})
	require.NoError(t, err)
	require.Equal(t, g.ID, *tk.GoalID)
	require.Equal(t, 5, *tk.Quantity)
	require.Nil(t, tk.QuantityDone)

	next := stream(t, d)
	done, err := d.c.CompleteTask(d.ctx, tk.ID, wire.CompleteTaskRequest{QuantityDone: testutil.Ptr(5)})
	require.NoError(t, err)
	require.Equal(t, 5, *done.QuantityDone)
	next(wire.EventGoalsChanged)
	goal, err := d.c.GetGoal(d.ctx, g.ID)
	require.NoError(t, err)
	require.Equal(t, wire.GoalDone, goal.Status, "its last unit is done")
	require.Equal(t, testutil.Day0, *goal.Progress.ProjectedFinishDay)

	_, err = d.c.ReopenTask(d.ctx, tk.ID)
	require.NoError(t, err)
	next(wire.EventGoalsChanged)
	goal, err = d.c.GetGoal(d.ctx, g.ID)
	require.NoError(t, err)
	require.Equal(t, wire.GoalActive, goal.Status)

	// Creating the item refreshed today's plan, which made it a step of today's session.
	patched, err := d.c.PatchTask(d.ctx, tk.ID, wire.PatchTaskRequest{GoalID: wire.Null[string](), Quantity: wire.Null[int](),
		ParentID: wire.Null[string](), RRule: wire.Some("FREQ=WEEKLY;BYDAY=MO")})
	require.NoError(t, err)
	require.Nil(t, patched.GoalID)
	require.Equal(t, "FREQ=WEEKLY;BYDAY=MO", *patched.RRule)

	_, err = d.c.CompleteTask(d.ctx, tk.ID, wire.CompleteTaskRequest{})
	apiErr(t, err, wire.CodeInvalidRequest)
	_, err = d.c.CreateTask(d.ctx, wire.CreateTaskRequest{Title: "x", RRule: testutil.Ptr("FREQ=YEARLY")})
	require.Equal(t, "rrule", apiErr(t, err, wire.CodeInvalidRequest).Details["field"])
	_, err = d.c.CreateTask(d.ctx, wire.CreateTaskRequest{Title: "x", GoalID: testutil.Ptr("nope")})
	require.Equal(t, "goal_id", apiErr(t, err, wire.CodeNotFound).Details["field"])
	_, err = d.c.CompleteTask(d.ctx, tk.ID, wire.CompleteTaskRequest{QuantityDone: testutil.Ptr(-2)})
	apiErr(t, err, wire.CodeInvalidRequest)
}

func TestBriefingEndpointAndClockInNudge(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	_, err := d.c.CreateTask(d.ctx, wire.CreateTaskRequest{Title: "Due", DueDay: testutil.Ptr("2026-09-16"), EstimateMinutes: testutil.Ptr(60)})
	require.NoError(t, err)
	_, err = d.c.CreateGoal(d.ctx, wire.CreateGoalRequest{Title: "Thesis", Kind: wire.GoalTasks,
		StartDay: testutil.Day0, DueDay: "2026-10-01"})
	require.NoError(t, err)

	// While off, input prompts a clock-in with the briefing as its body.
	d.sync() // the loop has started the monitor
	d.mon.emit(activity.Event{Kind: activity.Active, At: d.clk.Now()})
	d.sync()
	require.Equal(t, []string{"clock_in"}, d.notif.kinds())
	require.Equal(t, []string{"1 planned today · 1 reminder"}, d.notif.bodies())

	b, err := d.c.Briefing(d.ctx)
	require.NoError(t, err)
	require.Equal(t, testutil.Day0, b.Day)
	require.Empty(t, b.Pending)
	require.Len(t, b.Today, 1)
	require.Equal(t, []wire.Reminder{{TaskID: b.Today[0].TaskID, Title: "Due", DueDay: "2026-09-16", DaysLeft: 1,
		Message: "Due tomorrow"}}, b.Reminders)
	require.Len(t, b.Goals, 1)
	require.Equal(t, "Thesis", b.Goals[0].Title)
}

func TestClockInNudgeFallsBackWithNothingPlanned(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	d.sync() // the loop has started the monitor
	d.mon.emit(activity.Event{Kind: activity.Active, At: d.clk.Now()})
	d.sync()
	require.Equal(t, []string{"Clock in to start tracking."}, d.notif.bodies())
}

func TestBriefingSummary(t *testing.T) {
	t.Parallel()
	require.Equal(t, "", briefingSummary(storeBriefing(0, 0, 0)))
	require.Equal(t, "2 pending from yesterday · 3 planned today · 4 reminders", briefingSummary(storeBriefing(2, 3, 4)))
	require.Equal(t, "1 pending from yesterday · 1 reminder", briefingSummary(storeBriefing(1, 0, 1)))
}

func storeBriefing(pending, planned, reminders int) store.Briefing {
	b := store.Briefing{}
	for range pending {
		b.Pending = append(b.Pending, store.PlanEntry{Item: model.PlanItem{Status: model.PlanRolled}})
	}
	for range planned {
		b.Today = append(b.Today, store.PlanEntry{Item: model.PlanItem{Status: model.PlanPlanned}})
	}
	b.Today = append(b.Today, store.PlanEntry{Item: model.PlanItem{Status: model.PlanDone}}) // not counted
	for range reminders {
		b.Reminders = append(b.Reminders, planner.Reminder{})
	}
	return b
}

func TestThePlanKeepsUpWithTheClock(t *testing.T) {
	t.Parallel()
	d := startDaemonWith(t, setup{}, func(o *options) { o.keeper = true })
	next := stream(t, d)
	_, err := d.c.CreateTask(d.ctx, wire.CreateTaskRequest{Title: "Report", EstimateMinutes: testutil.Ptr(60)})
	require.NoError(t, err)
	next(wire.EventPlanChanged)
	plan, err := d.c.GetPlan(d.ctx, "")
	require.NoError(t, err)
	require.Equal(t, wire.Millis(testutil.At("09:00")), *plan.Items[0].StartAt)

	// 09:31 and not started: the keeper moves the block to the time left,
	// with nobody asking.
	d.clk.Advance(91 * time.Minute)
	require.JSONEq(t, `{"day":"2026-09-15"}`, string(next(wire.EventPlanChanged).Data))
	plan, err = d.c.GetPlan(d.ctx, "")
	require.NoError(t, err)
	require.Equal(t, wire.Millis(testutil.At("09:35")), *plan.Items[0].StartAt)
}

func TestMovePlanTaskEndpoint(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	a, err := d.c.CreateTask(d.ctx, wire.CreateTaskRequest{Title: "A", EstimateMinutes: testutil.Ptr(60)})
	require.NoError(t, err)
	b, err := d.c.CreateTask(d.ctx, wire.CreateTaskRequest{Title: "B", EstimateMinutes: testutil.Ptr(60)})
	require.NoError(t, err)
	next := stream(t, d)

	plan, err := d.c.MovePlanTask(d.ctx, wire.MovePlanTaskRequest{Day: testutil.Day0, TaskID: b.ID, BeforeTaskID: &a.ID})
	require.NoError(t, err)
	require.Len(t, plan.Items, 2)
	require.Equal(t, []string{b.ID, a.ID}, []string{plan.Items[0].TaskID, plan.Items[1].TaskID})
	require.Equal(t, wire.Millis(testutil.At("09:00")), *plan.Items[0].StartAt, "B takes the first slot")
	require.Equal(t, wire.Millis(testutil.At("10:00")), *plan.Items[1].StartAt)
	require.JSONEq(t, `{"day":"2026-09-15"}`, string(next(wire.EventPlanChanged).Data))

	_, err = d.c.MovePlanTask(d.ctx, wire.MovePlanTaskRequest{Day: testutil.Day0})
	require.Equal(t, "task_id", apiErr(t, err, wire.CodeInvalidRequest).Details["field"])
	_, err = d.c.MovePlanTask(d.ctx, wire.MovePlanTaskRequest{TaskID: a.ID})
	require.Equal(t, "day", apiErr(t, err, wire.CodeInvalidRequest).Details["field"])
}
