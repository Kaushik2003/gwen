package store_test

import (
	"testing"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/stretchr/testify/require"
)

// fxToday is the day fxNow belongs to: a Sunday.
const fxToday = "2026-09-20"

// env is the planner environment at the fixture clock's current instant: UTC,
// a 04:00 day rollover, the default 09:00–23:00 window, 30 m buffer, and 8 h
// target.
func (f *fx) env() store.PlanEnv {
	return store.PlanEnv{
		Now: f.clk.Now(), Loc: time.UTC,
		DayOf:    func(t time.Time) string { return t.UTC().Add(-4 * time.Hour).Format(model.DayLayout) },
		DayStart: 9 * 60, DayEnd: 23 * 60, Buffer: 30 * time.Minute, DailyTarget: 8 * time.Hour,
	}
}

func (f *fx) goal(ng store.NewGoal) (model.Goal, store.Changes) {
	f.t.Helper()
	g, ch, err := f.r.Goals.Create(f.ctx, ng)
	require.NoError(f.t, err)
	return g, ch
}

func quantityGoal(start, due string) store.NewGoal {
	return store.NewGoal{Title: "300 problems", Kind: model.GoalQuantity, Unit: testutil.Ptr("problems"),
		TargetQuantity: testutil.Ptr(300), MinutesPerUnit: testutil.Ptr(30), StartDay: start, DueDay: due}
}

func tasksGoal() store.NewGoal {
	return store.NewGoal{Title: "Ship the thesis", Kind: model.GoalTasks, StartDay: "2026-09-01", DueDay: "2026-10-31"}
}

// newTask creates nt, giving a task other than a step an hour's estimate when
// it has none, so it is planned work rather than a to-do.
func (f *fx) newTask(nt store.NewTask) model.Task {
	f.t.Helper()
	if nt.EstimateMinutes == nil && nt.ParentID == nil {
		nt.EstimateMinutes = testutil.Ptr(60)
	}
	tk, _, err := f.r.Tasks.Create(f.ctx, nt)
	require.NoError(f.t, err)
	return tk
}

func (f *fx) plan(day string) store.Plan {
	f.t.Helper()
	p, _, err := f.r.Plans.Get(f.ctx, day, f.env())
	require.NoError(f.t, err)
	return p
}

func titles(p store.Plan) []string {
	out := []string{}
	for _, e := range p.Items {
		out = append(out, e.Task.Title)
	}
	return out
}

func TestGoalCreateMakesSessionTemplate(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	p := f.project("Study")
	ng := quantityGoal("2026-09-20", "2026-12-20")
	ng.ProjectID = &p.ID
	g, ch := f.goal(ng)
	require.Equal(t, model.GoalActive, g.Status)
	require.Equal(t, "problems", g.Unit)
	require.Equal(t, 30*time.Minute, *g.PerUnit)
	require.True(t, ch.Goals)
	require.Len(t, ch.TaskIDs, 1)

	tmpl, err := f.r.Tasks.Get(f.ctx, ch.TaskIDs[0])
	require.NoError(t, err)
	require.Equal(t, store.SessionRule, *tmpl.RRule)
	require.Equal(t, g.ID, *tmpl.GoalID)
	require.Equal(t, p.ID, *tmpl.ProjectID)
	require.Equal(t, 3, tmpl.Priority)
	require.Equal(t, g.Title, tmpl.Title)

	listed, err := f.r.Tasks.List(f.ctx, store.TaskFilter{})
	require.NoError(t, err)
	require.Empty(t, listed, "templates are not listed by default")
	listed, err = f.r.Tasks.List(f.ctx, store.TaskFilter{Templates: true, GoalID: g.ID})
	require.NoError(t, err)
	require.Len(t, listed, 1)

	got, err := f.r.Goals.Get(f.ctx, g.ID)
	require.NoError(t, err)
	require.Equal(t, g, got)

	tg, ch := f.goal(tasksGoal())
	require.Empty(t, ch.TaskIDs, "a tasks goal has no template")
	require.Equal(t, "", tg.Unit)
}

func TestGoalValidation(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	with := func(mut func(*store.NewGoal)) store.NewGoal {
		g := quantityGoal("2026-09-20", "2026-12-20")
		mut(&g)
		return g
	}
	tests := []struct {
		name     string
		goal     store.NewGoal
		sentinel error
		field    string
	}{
		{"title", with(func(g *store.NewGoal) { g.Title = "  " }), store.ErrInvalid, "title"},
		{"kind", with(func(g *store.NewGoal) { g.Kind = "habit" }), store.ErrInvalid, "kind"},
		{"quantity needs a target", with(func(g *store.NewGoal) { g.TargetQuantity = nil }), store.ErrInvalid, "target_quantity"},
		{"quantity needs minutes", with(func(g *store.NewGoal) { g.MinutesPerUnit = testutil.Ptr(0) }), store.ErrInvalid, "minutes_per_unit"},
		{"tasks takes no target", with(func(g *store.NewGoal) { g.Kind = model.GoalTasks; g.MinutesPerUnit = nil }), store.ErrInvalid, "target_quantity"},
		{"start day", with(func(g *store.NewGoal) { g.StartDay = "soon" }), store.ErrInvalid, "start_day"},
		{"due before start", with(func(g *store.NewGoal) { g.DueDay = "2026-09-19" }), store.ErrInvalid, "due_day"},
		{"project", with(func(g *store.NewGoal) { g.ProjectID = testutil.Ptr("nope") }), store.ErrNotFound, "project_id"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := f.r.Goals.Create(f.ctx, tc.goal)
			userErr(t, err, tc.sentinel, tc.field)
		})
	}
}

func TestGoalUpdateListDelete(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	g, _ := f.goal(quantityGoal("2026-09-20", "2026-12-20"))
	tg, _ := f.goal(tasksGoal())

	up, err := f.r.Goals.Update(f.ctx, g.ID, store.GoalPatch{Title: testutil.Ptr("250 problems"),
		TargetQuantity: store.Nullable[int]{Set: true, Value: testutil.Ptr(250)}, Rev: &g.Rev})
	require.NoError(t, err)
	require.Equal(t, 250, *up.TargetQuantity)
	require.Equal(t, int64(2), up.Rev)
	_, err = f.r.Goals.Update(f.ctx, g.ID, store.GoalPatch{Rev: &g.Rev})
	userErr(t, err, store.ErrConflict, "rev")
	_, err = f.r.Goals.Update(f.ctx, g.ID, store.GoalPatch{Kind: testutil.Ptr(model.GoalTasks)})
	userErr(t, err, store.ErrInvalid, "kind")
	_, err = f.r.Goals.Update(f.ctx, g.ID, store.GoalPatch{Status: testutil.Ptr("paused")})
	userErr(t, err, store.ErrInvalid, "status")
	same, err := f.r.Goals.Update(f.ctx, g.ID, store.GoalPatch{Kind: testutil.Ptr(model.GoalQuantity)})
	require.NoError(t, err, "restating the kind is fine")
	require.Equal(t, int64(3), same.Rev)

	_, err = f.r.Goals.Update(f.ctx, tg.ID, store.GoalPatch{Status: testutil.Ptr(model.GoalAbandoned)})
	require.NoError(t, err)
	active, err := f.r.Goals.List(f.ctx, "")
	require.NoError(t, err)
	require.Len(t, active, 1)
	require.Equal(t, g.ID, active[0].ID)
	abandoned, err := f.r.Goals.List(f.ctx, model.GoalAbandoned)
	require.NoError(t, err)
	require.Len(t, abandoned, 1)
	all, err := f.r.Goals.List(f.ctx, store.GoalsAll)
	require.NoError(t, err)
	require.Equal(t, []string{tg.ID, g.ID}, []string{all[0].ID, all[1].ID}, "by due_day")
	_, err = f.r.Goals.List(f.ctx, "paused")
	userErr(t, err, store.ErrInvalid, "status")

	// Deleting a goal takes its template and the template's open occurrences.
	f.plan("") // materializes today's session
	occ, err := f.r.Tasks.List(f.ctx, store.TaskFilter{GoalID: g.ID})
	require.NoError(t, err)
	require.Len(t, occ, 1)
	ch, err := f.r.Goals.Delete(f.ctx, g.ID, false)
	require.NoError(t, err)
	require.Len(t, ch.TaskIDs, 2)
	_, err = f.r.Goals.Get(f.ctx, g.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	left, err := f.r.Tasks.List(f.ctx, store.TaskFilter{Status: "all", Templates: true})
	require.NoError(t, err)
	require.Empty(t, left)
	_, err = f.r.Goals.Delete(f.ctx, g.ID, false)
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestCommitments(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	p := f.project("Internship")
	c, err := f.r.Commitments.Create(f.ctx, store.NewCommitment{Title: "Internship", ProjectID: &p.ID,
		RRule: "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR", StartMinute: testutil.Ptr(10 * 60), DurationMinutes: 300,
		ActiveFrom: "2026-09-01"})
	require.NoError(t, err)
	require.True(t, c.CountsTowardTarget, "counts by default")
	require.Equal(t, 5*time.Hour, c.Duration)
	floating, err := f.r.Commitments.Create(f.ctx, store.NewCommitment{Title: "Gym", RRule: "FREQ=DAILY",
		DurationMinutes: 60, CountsTowardTarget: testutil.Ptr(false), ActiveFrom: "2026-09-01",
		ActiveUntil: testutil.Ptr("2026-12-31")})
	require.NoError(t, err)
	require.False(t, floating.CountsTowardTarget)

	list, err := f.r.Commitments.List(f.ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"Internship", "Gym"}, []string{list[0].Title, list[1].Title}, "fixed before floating")

	up, err := f.r.Commitments.Update(f.ctx, c.ID, store.CommitmentPatch{StartMinute: store.Nullable[int]{Set: true},
		DurationMinutes: testutil.Ptr(240), Rev: &c.Rev})
	require.NoError(t, err)
	require.Nil(t, up.StartMinute)
	require.Equal(t, 4*time.Hour, up.Duration)
	_, err = f.r.Commitments.Update(f.ctx, c.ID, store.CommitmentPatch{Rev: &c.Rev})
	userErr(t, err, store.ErrConflict, "rev")

	for _, tc := range []struct {
		name  string
		patch store.CommitmentPatch
		field string
	}{
		{"rrule", store.CommitmentPatch{RRule: testutil.Ptr("FREQ=DAILY;COUNT=3")}, "rrule"},
		{"start", store.CommitmentPatch{StartMinute: store.Nullable[int]{Set: true, Value: testutil.Ptr(1440)}}, "start_minute"},
		{"duration", store.CommitmentPatch{DurationMinutes: testutil.Ptr(0)}, "duration_minutes"},
		{"until", store.CommitmentPatch{ActiveUntil: store.Nullable[string]{Set: true, Value: testutil.Ptr("2026-08-01")}}, "active_until"},
		{"title", store.CommitmentPatch{Title: testutil.Ptr("")}, "title"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.r.Commitments.Update(f.ctx, floating.ID, tc.patch)
			userErr(t, err, store.ErrInvalid, tc.field)
		})
	}

	require.NoError(t, f.r.Commitments.Delete(f.ctx, c.ID))
	_, err = f.r.Commitments.Get(f.ctx, c.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	require.ErrorIs(t, f.r.Commitments.Delete(f.ctx, c.ID), store.ErrNotFound)
}

func TestTaskV2Fields(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	g, _ := f.goal(tasksGoal())
	tk := f.newTask(store.NewTask{Title: "Chapter 1", GoalID: &g.ID, Quantity: testutil.Ptr(3)})
	require.Equal(t, g.ID, *tk.GoalID)
	require.Equal(t, 3, *tk.Quantity)
	byGoal, err := f.r.Tasks.List(f.ctx, store.TaskFilter{GoalID: g.ID})
	require.NoError(t, err)
	require.Len(t, byGoal, 1)

	weekly := f.newTask(store.NewTask{Title: "Review", RRule: testutil.Ptr("FREQ=WEEKLY")})
	require.True(t, weekly.IsTemplate())
	_, err = f.r.Tasks.Complete(f.ctx, weekly.ID, nil)
	userErr(t, err, store.ErrInvalid, "id")

	for _, tc := range []struct {
		name     string
		task     store.NewTask
		sentinel error
		field    string
	}{
		{"rrule", store.NewTask{Title: "x", RRule: testutil.Ptr("FREQ=HOURLY")}, store.ErrInvalid, "rrule"},
		{"quantity", store.NewTask{Title: "x", Quantity: testutil.Ptr(0)}, store.ErrInvalid, "quantity"},
		{"goal", store.NewTask{Title: "x", GoalID: testutil.Ptr("nope")}, store.ErrNotFound, "goal_id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := f.r.Tasks.Create(f.ctx, tc.task)
			userErr(t, err, tc.sentinel, tc.field)
		})
	}

	up, err := f.r.Tasks.Update(f.ctx, tk.ID, store.TaskPatch{GoalID: store.Nullable[string]{Set: true},
		Quantity: store.Nullable[int]{Set: true}})
	require.NoError(t, err)
	require.Nil(t, up.GoalID)
	require.Nil(t, up.Quantity)

	daily := f.newTask(store.NewTask{Title: "Flashcards", RRule: testutil.Ptr("FREQ=DAILY")})
	f.plan("")
	occs, err := f.r.Tasks.List(f.ctx, store.TaskFilter{})
	require.NoError(t, err)
	var occ model.Task
	for _, o := range occs {
		if o.TemplateID != nil && *o.TemplateID == daily.ID {
			occ = o
		}
	}
	require.NotEmpty(t, occ.ID)
	_, err = f.r.Tasks.Update(f.ctx, occ.ID, store.TaskPatch{RRule: store.Nullable[string]{Set: true, Value: testutil.Ptr("FREQ=DAILY")}})
	userErr(t, err, store.ErrInvalid, "rrule")
}

func TestCompletionSettlesGoals(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	g, _ := f.goal(tasksGoal())
	a := f.newTask(store.NewTask{Title: "a", GoalID: &g.ID})
	b := f.newTask(store.NewTask{Title: "b", GoalID: &g.ID})

	ch, err := f.r.Tasks.Complete(f.ctx, a.ID, nil)
	require.NoError(t, err)
	require.False(t, ch.Goals, "one task is still open")
	ch, err = f.r.Tasks.Complete(f.ctx, b.ID, nil)
	require.NoError(t, err)
	require.True(t, ch.Goals)
	got, err := f.r.Goals.Get(f.ctx, g.ID)
	require.NoError(t, err)
	require.Equal(t, model.GoalDone, got.Status)

	ch, err = f.r.Tasks.Reopen(f.ctx, b.ID, fxToday)
	require.NoError(t, err)
	require.True(t, ch.Goals)
	got, err = f.r.Goals.Get(f.ctx, g.ID)
	require.NoError(t, err)
	require.Equal(t, model.GoalActive, got.Status)

	// An abandoned goal stays abandoned.
	_, err = f.r.Goals.Update(f.ctx, g.ID, store.GoalPatch{Status: testutil.Ptr(model.GoalAbandoned)})
	require.NoError(t, err)
	ch, err = f.r.Tasks.Complete(f.ctx, b.ID, nil)
	require.NoError(t, err)
	require.False(t, ch.Goals)

	// A quantity goal counts quantity_done, which defaults to quantity.
	q, _ := f.goal(store.NewGoal{Title: "5 katas", Kind: model.GoalQuantity, TargetQuantity: testutil.Ptr(5),
		MinutesPerUnit: testutil.Ptr(10), StartDay: "2026-09-01", DueDay: "2026-09-30"})
	k1 := f.newTask(store.NewTask{Title: "k1", GoalID: &q.ID, Quantity: testutil.Ptr(3)})
	k2 := f.newTask(store.NewTask{Title: "k2", GoalID: &q.ID, Quantity: testutil.Ptr(3)})
	ch, err = f.r.Tasks.Complete(f.ctx, k1.ID, nil)
	require.NoError(t, err)
	require.Equal(t, 3, *ch.Task.QuantityDone)
	require.False(t, ch.Goals)
	_, err = f.r.Tasks.Complete(f.ctx, k2.ID, testutil.Ptr(-1))
	userErr(t, err, store.ErrInvalid, "quantity_done")
	ch, err = f.r.Tasks.Complete(f.ctx, k2.ID, testutil.Ptr(2))
	require.NoError(t, err)
	require.Equal(t, 2, *ch.Task.QuantityDone)
	require.True(t, ch.Goals, "3 + 2 reaches 5")
	ch, err = f.r.Tasks.Reopen(f.ctx, k2.ID, fxToday)
	require.NoError(t, err)
	require.Nil(t, ch.Task.QuantityDone)
	require.True(t, ch.Goals)
}

func TestPlanGeneratesTodayOnFirstRead(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	_, err := f.r.Commitments.Create(f.ctx, store.NewCommitment{Title: "Gym", RRule: "FREQ=DAILY",
		StartMinute: testutil.Ptr(18 * 60), DurationMinutes: 60, CountsTowardTarget: testutil.Ptr(false),
		ActiveFrom: "2026-09-01"})
	require.NoError(t, err)
	urgent := f.newTask(store.NewTask{Title: "urgent", DueDay: testutil.Ptr(fxToday), EstimateMinutes: testutil.Ptr(120)})
	f.newTask(store.NewTask{Title: "later", Priority: testutil.Ptr(1)})
	f.newTask(store.NewTask{Title: "templ", RRule: testutil.Ptr("FREQ=WEEKLY;BYDAY=MO")})

	p, ch, err := f.r.Plans.Get(f.ctx, "", f.env())
	require.NoError(t, err)
	require.Equal(t, []string{fxToday}, ch.PlanDays)
	// 12:00–18:00 and 19:00–23:00 free: 600 minutes; target 480 − 30 buffer.
	require.Equal(t, 450, p.Capacity)
	require.Equal(t, []string{"urgent", "urgent", "later"}, titles(p))
	require.Equal(t, 90*time.Minute, p.Items[0].Item.Planned)
	require.Equal(t, 30*time.Minute, p.Items[1].Item.Planned)
	require.Equal(t, testutil.AtOn(fxToday, "12:00"), *p.Items[0].Item.StartAt)
	require.Equal(t, testutil.AtOn(fxToday, "13:40"), *p.Items[1].Item.StartAt, "after the gap")
	require.Equal(t, 180, p.Planned)

	again, ch, err := f.r.Plans.Get(f.ctx, fxToday, f.env())
	require.NoError(t, err)
	require.Empty(t, ch.PlanDays, "a plan exists, so it is only read")
	require.Equal(t, p.Items[0].Item.ID, again.Items[0].Item.ID)

	// A manual edit pins the item, and regeneration keeps it.
	moved, err := f.r.Plans.UpdateItem(f.ctx, p.Items[2].Item.ID, store.PlanItemPatch{
		StartAt: store.Nullable[time.Time]{Set: true, Value: testutil.Ptr(testutil.AtOn(fxToday, "20:00"))},
		Rev:     &p.Items[2].Item.Rev})
	require.NoError(t, err)
	require.True(t, moved.Item.Pinned)
	require.Equal(t, "later", moved.Task.Title)
	_, err = f.r.Plans.UpdateItem(f.ctx, moved.Item.ID, store.PlanItemPatch{Rev: &p.Items[2].Item.Rev})
	userErr(t, err, store.ErrConflict, "rev")
	_, err = f.r.Plans.UpdateItem(f.ctx, moved.Item.ID, store.PlanItemPatch{Status: testutil.Ptr(model.PlanDone)})
	userErr(t, err, store.ErrInvalid, "status")
	_, err = f.r.Plans.UpdateItem(f.ctx, moved.Item.ID, store.PlanItemPatch{PlannedMinutes: testutil.Ptr(0)})
	userErr(t, err, store.ErrInvalid, "planned_minutes")
	_, err = f.r.Plans.UpdateItem(f.ctx, "nope", store.PlanItemPatch{})
	require.ErrorIs(t, err, store.ErrNotFound)

	skipped, err := f.r.Plans.UpdateItem(f.ctx, p.Items[1].Item.ID, store.PlanItemPatch{Status: testutil.Ptr(model.PlanSkipped)})
	require.NoError(t, err)
	require.False(t, skipped.Item.Pinned, "a status change alone does not pin")

	regen, _, err := f.r.Plans.Generate(f.ctx, fxToday, f.env())
	require.NoError(t, err)
	require.Equal(t, []string{"urgent", "later"}, titles(regen),
		"kept items by start_at; urgent has a kept item, so it is not planned again")
	require.Equal(t, skipped.Item.ID, regen.Items[0].Item.ID)
	require.Equal(t, moved.Item.ID, regen.Items[1].Item.ID)
	require.Equal(t, 1, regen.Items[1].Item.Position)
	require.Equal(t, 60, regen.Planned, "the skipped block no longer counts")

	_, _, err = f.r.Plans.Generate(f.ctx, "2026-09-19", f.env())
	userErr(t, err, store.ErrInvalid, "day")
	_, _, err = f.r.Plans.Get(f.ctx, "someday", f.env())
	userErr(t, err, store.ErrInvalid, "day")

	future, _, err := f.r.Plans.Generate(f.ctx, "2026-09-21", f.env())
	require.NoError(t, err)
	require.Equal(t, 450, future.Capacity, "a whole window, less the gym: min(480, 780) − 30")
	require.Contains(t, titles(future), "templ", "Monday's occurrence")

	// Deleting a task hides its items.
	_, err = f.r.Tasks.Delete(f.ctx, urgent.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"later"}, titles(f.plan(fxToday)))
}

func TestSessionOccurrences(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	g, _ := f.goal(quantityGoal(fxToday, "2026-12-20"))
	p := f.plan("")
	require.Len(t, p.Items, 2, "120 minutes is two blocks")
	occ := p.Items[0].Task
	require.Equal(t, 4, *occ.Quantity, "ceil(300/92)")
	require.Equal(t, 120*time.Minute, *occ.Estimate)
	require.Equal(t, fxToday, *occ.OccurrenceDay)
	require.Equal(t, fxToday, *occ.DueDay, "a daily occurrence is due the same day")
	require.Equal(t, g.ID, *occ.GoalID)
	require.Equal(t, 3, occ.Priority)

	// Regenerating does not materialize a second occurrence, even after the
	// first is deleted.
	_, _, err := f.r.Plans.Generate(f.ctx, fxToday, f.env())
	require.NoError(t, err)
	_, err = f.r.Tasks.Delete(f.ctx, occ.ID)
	require.NoError(t, err)
	_, _, err = f.r.Plans.Generate(f.ctx, fxToday, f.env())
	require.NoError(t, err)
	listed, err := f.r.Tasks.List(f.ctx, store.TaskFilter{GoalID: g.ID, Status: "all"})
	require.NoError(t, err)
	require.Empty(t, listed)

	// Completing it defaults quantity_done to 4; the next session stays at 4.
	tomorrow := f.plan("2026-09-21")
	require.Empty(t, tomorrow.Items, "only today generates on read")
	gen, _, err := f.r.Plans.Generate(f.ctx, "2026-09-21", f.env())
	require.NoError(t, err)
	require.Equal(t, 4, *gen.Items[0].Task.Quantity, "ceil(300/91)")

	abandoned, _ := f.goal(quantityGoal(fxToday, "2026-12-20"))
	_, err = f.r.Goals.Update(f.ctx, abandoned.ID, store.GoalPatch{Status: testutil.Ptr(model.GoalAbandoned)})
	require.NoError(t, err)
	future, _ := f.goal(quantityGoal("2026-10-01", "2026-12-20"))
	_, _, err = f.r.Plans.Generate(f.ctx, "2026-09-22", f.env())
	require.NoError(t, err)
	for _, id := range []string{abandoned.ID, future.ID} {
		listed, err := f.r.Tasks.List(f.ctx, store.TaskFilter{GoalID: id, Status: "all"})
		require.NoError(t, err)
		require.Empty(t, listed, "no sessions for an inactive or unstarted goal")
	}
}

func TestCompletionMovesPlanItems(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	tk := f.newTask(store.NewTask{Title: "essay", EstimateMinutes: testutil.Ptr(180)})
	p := f.plan("")
	require.Len(t, p.Items, 2)
	ch, err := f.r.Tasks.Complete(f.ctx, tk.ID, nil)
	require.NoError(t, err)
	require.Equal(t, []string{fxToday}, ch.PlanDays)
	for _, e := range f.plan("").Items {
		require.Equal(t, model.PlanDone, e.Item.Status)
	}
	require.Equal(t, 180, f.plan("").Planned, "done items still count as planned")
	ch, err = f.r.Tasks.Reopen(f.ctx, tk.ID, fxToday)
	require.NoError(t, err)
	require.Equal(t, []string{fxToday}, ch.PlanDays)
	for _, e := range f.plan("").Items {
		require.Equal(t, model.PlanPlanned, e.Item.Status)
	}
}

func TestRolloverOnTheNextDay(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	open := f.newTask(store.NewTask{Title: "open", EstimateMinutes: testutil.Ptr(60)})
	finished := f.newTask(store.NewTask{Title: "finished", EstimateMinutes: testutil.Ptr(60)})
	gone := f.newTask(store.NewTask{Title: "gone", EstimateMinutes: testutil.Ptr(60)})
	daily := f.newTask(store.NewTask{Title: "daily", RRule: testutil.Ptr("FREQ=DAILY")})
	day1 := f.plan("")
	require.Len(t, day1.Items, 4)
	_, err := f.r.Tasks.Complete(f.ctx, finished.ID, nil)
	require.NoError(t, err)
	_, err = f.r.Tasks.Delete(f.ctx, gone.ID)
	require.NoError(t, err)

	// Undo the completion's own marking so rollover has to decide.
	_, err = f.db.SQL().Exec(`UPDATE plan_items SET status = 'planned' WHERE task_id = ?`, finished.ID)
	require.NoError(t, err)

	f.clk.Advance(24 * time.Hour)
	day2, ch, err := f.r.Plans.Get(f.ctx, "", f.env())
	require.NoError(t, err)
	require.Contains(t, ch.PlanDays, fxToday)
	require.Contains(t, ch.PlanDays, "2026-09-21")

	statuses := map[string]string{}
	var all []store.Plan
	all = append(all, f.plan(fxToday))
	for _, e := range all[0].Items {
		statuses[e.Task.Title] = e.Item.Status
	}
	require.Equal(t, model.PlanRolled, statuses["open"])
	require.Equal(t, model.PlanDone, statuses["finished"])
	require.Len(t, statuses, 2, "items of deleted tasks are not returned")
	stored := func(taskID string) string {
		var status string
		require.NoError(t, f.db.SQL().QueryRow(`SELECT status FROM plan_items WHERE task_id = ?`, taskID).Scan(&status))
		return status
	}
	var goneItemDeleted bool
	require.NoError(t, f.db.SQL().QueryRow(`SELECT deleted_at IS NOT NULL FROM plan_items WHERE task_id = ?`,
		gone.ID).Scan(&goneItemDeleted))
	require.True(t, goneItemDeleted, "deleting a task deletes its planned items")
	var expired string
	require.NoError(t, f.db.SQL().QueryRow(`SELECT id FROM tasks WHERE template_id = ? AND deleted_at IS NOT NULL`,
		daily.ID).Scan(&expired))
	require.Equal(t, model.PlanSkipped, stored(expired), "yesterday's daily occurrence expired and was deleted")

	var rolled store.PlanEntry
	for _, e := range day2.Items {
		if e.Task.ID == open.ID {
			rolled = e
		}
	}
	require.Equal(t, 1, rolled.Item.RolloverCount)
	require.NotNil(t, rolled.Item.RolledFromID)
	require.Equal(t, []string{"daily", "open"}, titles(day2), "due today outranks one rollover")

	// Rollover is idempotent.
	_, ch, err = f.r.Plans.Get(f.ctx, "", f.env())
	require.NoError(t, err)
	require.Empty(t, ch.PlanDays)
}

func TestBriefing(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	g, _ := f.goal(tasksGoal())
	late := f.newTask(store.NewTask{Title: "late", DueDay: testutil.Ptr("2026-09-18"), GoalID: &g.ID})
	f.newTask(store.NewTask{Title: "tomorrow", DueDay: testutil.Ptr("2026-09-21")})
	big := f.newTask(store.NewTask{Title: "big", EstimateMinutes: testutil.Ptr(600), Priority: testutil.Ptr(4)})

	// Yesterday everything was planned and nothing was done.
	yesterday := f.env()
	yesterday.Now = testutil.AtOn("2026-09-19", "12:00")
	_, _, err := f.r.Plans.Get(f.ctx, "", yesterday)
	require.NoError(t, err)

	// Fill today so that "big" cannot be planned again.
	_, err = f.r.Commitments.Create(f.ctx, store.NewCommitment{Title: "All day", RRule: "FREQ=DAILY",
		DurationMinutes: 480, ActiveFrom: fxToday})
	require.NoError(t, err)

	b, _, err := f.r.Plans.Briefing(f.ctx, f.env())
	require.NoError(t, err)
	require.Equal(t, fxToday, b.Day)
	require.Empty(t, b.Today, "no capacity left today")
	require.Len(t, b.Pending, 3, "all three were planned yesterday and none fits today")
	require.Equal(t, big.ID, b.Pending[2].Task.ID)
	for _, e := range b.Pending {
		require.Equal(t, model.PlanRolled, e.Item.Status)
		require.Equal(t, "2026-09-19", e.Item.Day)
	}
	require.Len(t, b.Reminders, 2)
	require.Equal(t, late.ID, b.Reminders[0].TaskID)
	require.Equal(t, "Overdue by 2 days", b.Reminders[0].Message)
	require.Equal(t, "Due tomorrow", b.Reminders[1].Message)
	require.Len(t, b.Goals, 1)
	require.Equal(t, 1, b.Goals[0].Progress.Target)
	require.Equal(t, 1, b.Goals[0].Progress.Remaining)

	progress, err := f.r.Plans.Progress(f.ctx, []model.Goal{g}, f.env())
	require.NoError(t, err)
	require.Equal(t, b.Goals[0].Progress, progress[g.ID])
}

func TestTodosAndStarts(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	tomorrow := "2026-09-21"
	clock := func(day, hour int) time.Time { return time.Date(2026, 9, day, hour, 0, 0, 0, time.UTC) }
	todo, _, err := f.r.Tasks.Create(f.ctx, store.NewTask{Title: "call the bank", StartDay: testutil.Ptr(fxToday)})
	require.NoError(t, err)
	require.True(t, todo.IsTodo())
	f.newTask(store.NewTask{Title: "next", StartDay: &tomorrow})
	f.newTask(store.NewTask{Title: "at 15", EstimateMinutes: testutil.Ptr(120), StartDay: testutil.Ptr(fxToday),
		StartMinute: testutil.Ptr(15 * 60)})
	f.newTask(store.NewTask{Title: "anytime"})
	f.newTask(store.NewTask{Title: "daily", RRule: testutil.Ptr("FREQ=DAILY"), EstimateMinutes: testutil.Ptr(30),
		StartDay: &tomorrow, StartMinute: testutil.Ptr(20 * 60)})

	p := f.plan("")
	require.Equal(t, []string{"at 15", "anytime"}, titles(p), "no to-do, nothing starting later, timed work first")
	require.Equal(t, 120*time.Minute, p.Items[0].Item.Planned, "one block at the chosen time, past the 90-minute cap")
	require.Equal(t, clock(20, 15), *p.Items[0].Item.StartAt)
	require.Equal(t, clock(20, 12), *p.Items[1].Item.StartAt, "untimed work fills the earliest free time")

	next, _, err := f.r.Plans.Generate(f.ctx, tomorrow, f.env())
	require.NoError(t, err)
	require.Contains(t, titles(next), "next")
	require.Contains(t, titles(next), "daily", "a template recurs from its start day")
	for _, e := range next.Items {
		switch e.Task.Title {
		case "daily":
			require.Equal(t, tomorrow, *e.Task.StartDay)
			require.Equal(t, 20*60, *e.Task.StartMinute, "occurrences keep the template's time")
			require.Equal(t, clock(21, 20), *e.Item.StartAt)
		case "at 15":
			require.True(t, e.Item.StartAt.Before(clock(21, 15)), "a start time holds on its day only")
		}
	}
}
