package store_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/stretchr/testify/require"
)

// W15: sessions, steps, items, and the delete cascades
// (docs/06-planner.md#sessions-and-steps and #deleting).

// dsaGoal is 12 problems of 20 minutes from today to 2026-09-23, 3 a day, with
// an hour a day: each session holds 3.
func (f *fx) dsaGoal() model.Goal {
	f.t.Helper()
	g, _ := f.goal(store.NewGoal{Title: "DSA", Kind: model.GoalQuantity, Unit: testutil.Ptr("problems"),
		TargetQuantity: testutil.Ptr(12), MinutesPerUnit: testutil.Ptr(20), DailyMinutes: testutil.Ptr(60),
		StartDay: fxToday, DueDay: "2026-09-23"})
	return g
}

// items adds n items to the goal, named p1 to pn.
func (f *fx) items(g model.Goal, n int) []model.Task {
	f.t.Helper()
	out := make([]model.Task, n)
	for i := range out {
		out[i] = f.newTask(store.NewTask{Title: fmt.Sprintf("p%d", i+1), GoalID: &g.ID})
	}
	return out
}

func (f *fx) get(id string) model.Task {
	f.t.Helper()
	t, err := f.r.Tasks.Get(f.ctx, id)
	require.NoError(f.t, err)
	return t
}

func (f *fx) complete(id string) store.TaskChange {
	f.t.Helper()
	ch, err := f.r.Tasks.Complete(f.ctx, id, nil)
	require.NoError(f.t, err)
	return ch
}

func (f *fx) progress(g model.Goal) (done, linedUp int) {
	f.t.Helper()
	p, err := f.r.Plans.Progress(f.ctx, []model.Goal{g}, f.env())
	require.NoError(f.t, err)
	return p[g.ID].Done, p[g.ID].LinedUp
}

// session returns today's only plan entry, which must be g's session.
func (f *fx) session(g model.Goal) store.PlanEntry {
	f.t.Helper()
	p := f.plan("")
	require.Len(f.t, p.Items, 1)
	e := p.Items[0]
	require.Equal(f.t, g.ID, *e.Task.GoalID)
	require.NotNil(f.t, e.Task.TemplateID)
	return e
}

func stepTitles(steps []model.Task) []string {
	out := []string{}
	for _, s := range steps {
		out = append(out, s.Title)
	}
	return out
}

func TestSessionFillsFromTheQueue(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	g := f.dsaGoal()
	f.items(g, 5)
	_, linedUp := f.progress(g)
	require.Equal(t, 5, linedUp)

	e := f.session(g)
	require.Equal(t, 3, *e.Task.Quantity)
	require.Equal(t, time.Hour, *e.Task.Estimate)
	require.Equal(t, time.Hour, e.Item.Planned, "the session is one block of its time")
	require.Equal(t, []string{"p1", "p2", "p3"}, stepTitles(e.Steps), "the front of the queue, in order")
	for _, s := range e.Steps {
		require.Equal(t, e.Task.ProjectID, s.ProjectID)
	}

	more := f.newTask(store.NewTask{Title: "p6", GoalID: &g.ID})
	require.Nil(t, f.get(more.ID).ParentID, "a full session takes no more")
}

func TestCompletingTheLastStepCompletesTheSession(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	g := f.dsaGoal()
	f.items(g, 3)
	e := f.session(g)

	f.complete(e.Steps[0].ID)
	f.complete(e.Steps[1].ID)
	require.Equal(t, model.TaskOpen, f.get(e.Task.ID).Status)
	ch := f.complete(e.Steps[2].ID)
	require.Contains(t, ch.TaskIDs, e.Task.ID)
	session := f.get(e.Task.ID)
	require.Equal(t, model.TaskDone, session.Status)
	require.Equal(t, 0, *session.QuantityDone, "its steps count instead")
	done, _ := f.progress(g)
	require.Equal(t, 3, done, "each step counts once")
	require.Equal(t, model.PlanDone, f.plan("").Items[0].Item.Status)

	_, err := f.r.Tasks.Reopen(f.ctx, e.Steps[2].ID, fxToday)
	require.NoError(t, err)
	require.Equal(t, model.TaskOpen, f.get(e.Task.ID).Status, "reopening a step reopens its session")
	done, _ = f.progress(g)
	require.Equal(t, 2, done)
}

func TestCompletingASessionEarlyReturnsItsSteps(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	g := f.dsaGoal()
	items := f.items(g, 4)
	e := f.session(g)
	f.complete(e.Steps[0].ID)

	ch := f.complete(e.Task.ID)
	require.Equal(t, 0, *ch.Task.QuantityDone)
	require.Contains(t, ch.TaskIDs, items[1].ID)
	require.Nil(t, f.get(items[1].ID).ParentID)
	require.Nil(t, f.get(items[2].ID).ParentID)
	require.Equal(t, e.Task.ID, *f.get(items[0].ID).ParentID, "a done step stays")
	done, linedUp := f.progress(g)
	require.Equal(t, 1, done)
	require.Equal(t, 3, linedUp, "p2 and p3 are back ahead of p4")

	queue, err := f.r.Tasks.List(f.ctx, store.TaskFilter{GoalID: g.ID})
	require.NoError(t, err)
	var open []string
	for _, tk := range queue {
		if tk.TemplateID == nil {
			open = append(open, tk.Title)
		}
	}
	require.Equal(t, []string{"p2", "p3", "p4"}, open)
}

func TestExpiredSessionsSettle(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	g := f.dsaGoal()
	f.items(g, 8)
	day1 := f.session(g)
	f.complete(day1.Steps[0].ID)

	f.clk.Advance(24 * time.Hour)
	f.plan("")
	settled := f.get(day1.Task.ID)
	require.Equal(t, model.TaskDone, settled.Status, "a session with a done step is completed, not deleted")
	require.Equal(t, 0, *settled.QuantityDone)
	day2 := f.plan("").Items
	require.Len(t, day2, 1)
	require.Equal(t, []string{"p2", "p3", "p4"}, stepTitles(day2[0].Steps), "yesterday's leftovers come first")

	f.clk.Advance(24 * time.Hour)
	f.plan("")
	_, err := f.r.Tasks.Get(f.ctx, day2[0].Task.ID)
	require.ErrorIs(t, err, store.ErrNotFound, "a session with nothing done expires")
	day3 := f.plan("").Items
	require.Len(t, day3, 1)
	require.Equal(t, []string{"p2", "p3", "p4"}, stepTitles(day3[0].Steps))
}

func TestStepsBelongToTheirParent(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	p := f.project("P")
	q := f.project("Q")
	parent := f.task("Read chapter 4", &p.ID)
	step := f.newTask(store.NewTask{Title: "4.1", ParentID: &parent.ID})
	require.Equal(t, p.ID, *step.ProjectID)
	require.Nil(t, step.GoalID)
	require.Equal(t, []string{"Read chapter 4"}, titles(f.plan("")), "steps are not planned")

	_, err := f.r.Tasks.Update(f.ctx, parent.ID, store.TaskPatch{ProjectID: store.Nullable[string]{Set: true, Value: &q.ID}})
	require.NoError(t, err)
	require.Equal(t, q.ID, *f.get(step.ID).ProjectID, "steps move with their parent")

	_, _, err = f.r.Tasks.Create(f.ctx, store.NewTask{Title: "4.1.1", ParentID: &step.ID})
	userErr(t, err, store.ErrInvalid, "parent_id")
	_, _, err = f.r.Tasks.Create(f.ctx, store.NewTask{Title: "x", ParentID: testutil.Ptr("nope")})
	userErr(t, err, store.ErrNotFound, "parent_id")
	daily := f.newTask(store.NewTask{Title: "daily", RRule: testutil.Ptr("FREQ=DAILY")})
	_, _, err = f.r.Tasks.Create(f.ctx, store.NewTask{Title: "x", ParentID: &daily.ID})
	userErr(t, err, store.ErrInvalid, "parent_id")
	_, err = f.r.Tasks.Update(f.ctx, parent.ID, store.TaskPatch{ParentID: store.Nullable[string]{Set: true, Value: &daily.ID}})
	userErr(t, err, store.ErrInvalid, "parent_id")

	f.complete(step.ID)
	require.Equal(t, model.TaskDone, f.get(parent.ID).Status, "the last step completes its parent")
}

func TestDeleteCascades(t *testing.T) {
	t.Parallel()

	t.Run("steps go with their task", func(t *testing.T) {
		t.Parallel()
		f := newFx(t)
		parent := f.task("parent", nil)
		step := f.newTask(store.NewTask{Title: "step", ParentID: &parent.ID})
		ch, err := f.r.Tasks.Delete(f.ctx, parent.ID)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{parent.ID, step.ID}, ch.Deleted)
	})

	t.Run("a template takes its open occurrences", func(t *testing.T) {
		t.Parallel()
		f := newFx(t)
		daily := f.newTask(store.NewTask{Title: "daily", RRule: testutil.Ptr("FREQ=DAILY")})
		require.Equal(t, []string{"daily"}, titles(f.plan("")))
		ch, err := f.r.Tasks.Delete(f.ctx, daily.ID)
		require.NoError(t, err)
		require.Len(t, ch.Deleted, 2)
		require.Contains(t, ch.PlanDays, fxToday)
		require.Empty(t, f.plan("").Items)
	})

	t.Run("a session's steps return to the queue", func(t *testing.T) {
		t.Parallel()
		f := newFx(t)
		g := f.dsaGoal()
		f.items(g, 3)
		e := f.session(g)
		f.complete(e.Steps[0].ID)
		ch, err := f.r.Tasks.Delete(f.ctx, e.Task.ID)
		require.NoError(t, err)
		require.Equal(t, []string{e.Task.ID}, ch.Deleted)
		for _, s := range e.Steps {
			require.Nil(t, f.get(s.ID).ParentID)
		}
		done, linedUp := f.progress(g)
		require.Equal(t, 1, done, "the done step still counts")
		require.Equal(t, 2, linedUp)
	})

	t.Run("a goal, with or without its tasks", func(t *testing.T) {
		t.Parallel()
		f := newFx(t)
		keep := f.dsaGoal()
		items := f.items(keep, 4)
		f.plan("")
		ch, err := f.r.Goals.Delete(f.ctx, keep.ID, false)
		require.NoError(t, err)
		require.Len(t, ch.Deleted, 2, "the template and today's session")
		for _, it := range items {
			kept := f.get(it.ID)
			require.Nil(t, kept.ParentID)
		}
		require.Len(t, f.plan("").Items, 4, "without their goal, items are ordinary tasks, planned again")

		drop := f.dsaGoal()
		gone := f.items(drop, 2)
		ch, err = f.r.Goals.Delete(f.ctx, drop.ID, true)
		require.NoError(t, err)
		for _, it := range gone {
			require.Contains(t, ch.Deleted, it.ID)
		}
	})

	t.Run("a project takes its goals", func(t *testing.T) {
		t.Parallel()
		f := newFx(t)
		p := f.project("Self improvement")
		g, _ := f.goal(store.NewGoal{Title: "DSA", Kind: model.GoalQuantity, TargetQuantity: testutil.Ptr(12),
			MinutesPerUnit: testutil.Ptr(20), ProjectID: &p.ID, StartDay: fxToday, DueDay: "2026-09-23"})
		f.items(g, 2)
		f.plan("")
		_, err := f.r.Projects.Delete(f.ctx, p.ID)
		require.NoError(t, err)
		_, err = f.r.Goals.Get(f.ctx, g.ID)
		require.ErrorIs(t, err, store.ErrNotFound)
		left, err := f.r.Tasks.List(f.ctx, store.TaskFilter{Status: "all", Templates: true})
		require.NoError(t, err)
		require.Empty(t, left)
		require.Empty(t, f.plan("").Items)
	})

	t.Run("the plan regenerates once its tasks are gone", func(t *testing.T) {
		t.Parallel()
		f := newFx(t)
		a := f.task("a", nil)
		require.Equal(t, []string{"a"}, titles(f.plan("")))
		f.task("b", nil)
		_, err := f.r.Tasks.Delete(f.ctx, a.ID)
		require.NoError(t, err)
		require.Equal(t, []string{"b"}, titles(f.plan("")))
	})

	t.Run("all or none", func(t *testing.T) {
		t.Parallel()
		f := newFx(t)
		a := f.task("a", nil)
		_, err := f.r.Tasks.Delete(f.ctx, a.ID, "nope")
		require.ErrorIs(t, err, store.ErrNotFound)
		f.get(a.ID)
	})
}

func TestAcceptFillsTodaysSession(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	g := f.dsaGoal()
	require.Empty(t, f.session(g).Steps, "nothing lined up yet")
	output := map[string]any{"tasks": []map[string]any{
		{"title": "Two Sum", "notes": "hash map", "estimate_minutes": 15, "due_day": fxToday, "priority": 2, "quantity": 1},
		{"title": "Valid Anagram", "notes": "", "estimate_minutes": 15, "due_day": fxToday, "priority": 2, "quantity": 1},
	}}
	run, err := f.r.LLMRuns.Create(f.ctx, model.RunBreakdown, g.ID, model.RunOK, output)
	require.NoError(t, err)
	tasks, _, err := f.r.LLMRuns.Accept(f.ctx, run.ID, []int{0, 1}, "")
	require.NoError(t, err)
	for _, tk := range tasks {
		require.Nil(t, tk.DueDay, "sessions schedule items")
		require.NotNil(t, tk.ParentID)
	}
	require.Equal(t, []string{"Two Sum", "Valid Anagram"}, stepTitles(f.session(g).Steps))
	require.Equal(t, "hash map", f.session(g).Steps[0].Notes)
}
