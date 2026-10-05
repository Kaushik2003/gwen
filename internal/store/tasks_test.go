package store_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestTaskCreateDefaultsAndValidation(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	p := f.project("P")
	tk, _, err := f.r.Tasks.Create(f.ctx, store.NewTask{Title: "  Read chapter 3 ", ProjectID: &p.ID})
	require.NoError(t, err)
	require.Equal(t, "Read chapter 3", tk.Title)
	require.Equal(t, "", tk.Notes)
	require.Equal(t, 2, tk.Priority)
	require.Equal(t, model.TaskOpen, tk.Status)
	require.Nil(t, tk.DueDay)
	require.Nil(t, tk.Estimate)
	require.True(t, tk.IsTodo(), "a task with no estimate is a to-do")
	require.Nil(t, tk.StartDay)
	require.Nil(t, tk.StartMinute)
	require.Nil(t, tk.DoneAt)
	require.Equal(t, int64(1), tk.Rev)

	full, _, err := f.r.Tasks.Create(f.ctx, store.NewTask{Title: "x", Notes: testutil.Ptr("n"), Priority: testutil.Ptr(4),
		DueDay: testutil.Ptr("2026-09-30"), EstimateMinutes: testutil.Ptr(90)})
	require.NoError(t, err)
	require.Equal(t, 90*time.Minute, *full.Estimate)
	got, err := f.r.Tasks.Get(f.ctx, full.ID)
	require.NoError(t, err)
	require.Equal(t, full, got)

	tests := []struct {
		name     string
		task     store.NewTask
		sentinel error
		field    string
	}{
		{"empty title", store.NewTask{Title: " "}, store.ErrInvalid, "title"},
		{"priority", store.NewTask{Title: "x", Priority: testutil.Ptr(5)}, store.ErrInvalid, "priority"},
		{"due day", store.NewTask{Title: "x", DueDay: testutil.Ptr("2026-02-30")}, store.ErrInvalid, "due_day"},
		{"estimate", store.NewTask{Title: "x", EstimateMinutes: testutil.Ptr(0)}, store.ErrInvalid, "estimate_minutes"},
		{"start day", store.NewTask{Title: "x", StartDay: testutil.Ptr("2026-13-01")}, store.ErrInvalid, "start_day"},
		{"start after due", store.NewTask{Title: "x", StartDay: testutil.Ptr("2026-09-30"), DueDay: testutil.Ptr("2026-09-29")}, store.ErrInvalid, "start_day"},
		{"start time without a day", store.NewTask{Title: "x", StartMinute: testutil.Ptr(600)}, store.ErrInvalid, "start_minute"},
		{"start time", store.NewTask{Title: "x", StartDay: testutil.Ptr("2026-09-30"), StartMinute: testutil.Ptr(1440)}, store.ErrInvalid, "start_minute"},
		{"unknown project", store.NewTask{Title: "x", ProjectID: testutil.Ptr("nope")}, store.ErrNotFound, "project_id"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := f.r.Tasks.Create(f.ctx, tc.task)
			userErr(t, err, tc.sentinel, tc.field)
		})
	}
}

func TestTaskListOrderAndFilters(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	p := f.project("P")
	mk := func(title string, due *string, prio int, project *string) model.Task {
		tk, _, err := f.r.Tasks.Create(f.ctx, store.NewTask{Title: title, DueDay: due, Priority: &prio, ProjectID: project})
		require.NoError(t, err)
		f.clk.Advance(time.Second)
		return tk
	}
	noDueLow := mk("no due, low", nil, 1, nil)
	noDueHigh := mk("no due, high", nil, 4, &p.ID)
	lateLow := mk("late, low", testutil.Ptr("2026-10-01"), 1, nil)
	soonLow := mk("soon, low", testutil.Ptr("2026-09-20"), 1, &p.ID)
	soonHigh := mk("soon, high", testutil.Ptr("2026-09-20"), 3, nil)
	soonHighLater := mk("soon, high, created later", testutil.Ptr("2026-09-20"), 3, nil)
	done := mk("done", nil, 2, nil)
	_, err := f.r.Tasks.Complete(f.ctx, done.ID, nil)
	require.NoError(t, err)

	ids := func(tasks []model.Task) []string {
		out := []string{}
		for _, t := range tasks {
			out = append(out, t.Title)
		}
		return out
	}
	open, err := f.r.Tasks.List(f.ctx, store.TaskFilter{})
	require.NoError(t, err)
	require.Equal(t, []string{soonHigh.Title, soonHighLater.Title, soonLow.Title, lateLow.Title, noDueHigh.Title, noDueLow.Title}, ids(open),
		"due_day ascending with nulls last, then priority descending, then created_at")

	all, err := f.r.Tasks.List(f.ctx, store.TaskFilter{Status: "all"})
	require.NoError(t, err)
	require.Len(t, all, 7)
	doneOnly, err := f.r.Tasks.List(f.ctx, store.TaskFilter{Status: "done"})
	require.NoError(t, err)
	require.Equal(t, []string{done.Title}, ids(doneOnly))
	ofP, err := f.r.Tasks.List(f.ctx, store.TaskFilter{ProjectID: p.ID})
	require.NoError(t, err)
	require.Equal(t, []string{soonLow.Title, noDueHigh.Title}, ids(ofP))
	before, err := f.r.Tasks.List(f.ctx, store.TaskFilter{DueBefore: "2026-10-01"})
	require.NoError(t, err)
	require.Equal(t, []string{soonHigh.Title, soonHighLater.Title, soonLow.Title}, ids(before), "due_before is exclusive")

	_, err = f.r.Tasks.List(f.ctx, store.TaskFilter{Status: "later"})
	userErr(t, err, store.ErrInvalid, "status")
	_, err = f.r.Tasks.List(f.ctx, store.TaskFilter{DueBefore: "soon"})
	userErr(t, err, store.ErrInvalid, "due_before")
}

func TestTaskUpdateCompleteReopenDelete(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	p := f.project("P")
	tk, _, err := f.r.Tasks.Create(f.ctx, store.NewTask{Title: "t", ProjectID: &p.ID, DueDay: testutil.Ptr("2026-09-21"),
		EstimateMinutes: testutil.Ptr(30)})
	require.NoError(t, err)

	f.clk.Advance(time.Minute)
	up, err := f.r.Tasks.Update(f.ctx, tk.ID, store.TaskPatch{
		ProjectID:       store.Nullable[string]{Set: true},
		DueDay:          store.Nullable[string]{Set: true},
		EstimateMinutes: store.Nullable[int]{Set: true, Value: testutil.Ptr(45)},
		Title:           testutil.Ptr("renamed"),
		Notes:           testutil.Ptr("notes"),
		Priority:        testutil.Ptr(1),
		Rev:             &tk.Rev,
	})
	require.NoError(t, err)
	require.Nil(t, up.ProjectID, "null clears")
	require.Nil(t, up.DueDay)
	require.Equal(t, 45*time.Minute, *up.Estimate)
	require.Equal(t, "renamed", up.Title)
	require.Equal(t, int64(2), up.Rev)
	require.Equal(t, fxNow.Add(time.Minute), up.UpdatedAt)

	cleared, err := f.r.Tasks.Update(f.ctx, tk.ID, store.TaskPatch{EstimateMinutes: store.Nullable[int]{Set: true}})
	require.NoError(t, err)
	require.Nil(t, cleared.Estimate)

	_, err = f.r.Tasks.Update(f.ctx, tk.ID, store.TaskPatch{Rev: &tk.Rev})
	userErr(t, err, store.ErrConflict, "rev")
	_, err = f.r.Tasks.Update(f.ctx, tk.ID, store.TaskPatch{Priority: testutil.Ptr(0)})
	userErr(t, err, store.ErrInvalid, "priority")

	completed, err := f.r.Tasks.Complete(f.ctx, tk.ID, nil)
	require.NoError(t, err)
	done := completed.Task
	require.Equal(t, model.TaskDone, done.Status)
	require.Equal(t, fxNow.Add(time.Minute), *done.DoneAt)
	again, err := f.r.Tasks.Complete(f.ctx, tk.ID, nil)
	require.NoError(t, err)
	require.Equal(t, done, again.Task, "complete is idempotent")

	reopened, err := f.r.Tasks.Reopen(f.ctx, tk.ID, testutil.Day0)
	require.NoError(t, err)
	open := reopened.Task
	require.Equal(t, model.TaskOpen, open.Status)
	require.Nil(t, open.DoneAt)
	require.Equal(t, done.Rev+1, open.Rev)
	againOpen, err := f.r.Tasks.Reopen(f.ctx, tk.ID, testutil.Day0)
	require.NoError(t, err)
	require.Equal(t, open, againOpen.Task, "reopen is idempotent")

	_, err = f.r.Tasks.Delete(f.ctx, tk.ID)
	require.NoError(t, err)
	_, err = f.r.Tasks.Get(f.ctx, tk.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = f.r.Tasks.Delete(f.ctx, tk.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = f.r.Tasks.Complete(f.ctx, tk.ID, nil)
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = f.r.Tasks.Update(f.ctx, tk.ID, store.TaskPatch{})
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestTaskTracked(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	p := f.project("P")
	tk := f.task("t", &p.ID)
	other := f.task("other", nil)
	f.startDay("09:00")
	require.NoError(t, f.tx(func(tx *sql.Tx) error {
		_, err := f.r.Segments.OpenSegment(f.ctx, tx, model.KindWork, model.SourceUser, &p.ID, &tk.ID, at("09:00"))
		return err
	}))
	f.close("10:00")
	require.NoError(t, f.tx(func(tx *sql.Tx) error {
		_, err := f.r.Segments.OpenSegment(f.ctx, tx, model.KindWork, model.SourceUser, &p.ID, &tk.ID, at("11:00"))
		return err
	}))

	got, err := f.r.Tasks.Tracked(f.ctx, []string{tk.ID, other.ID}, at("11:30"))
	require.NoError(t, err)
	require.Equal(t, map[string]time.Duration{tk.ID: 90 * time.Minute}, got, "the open segment counts up to now")

	empty, err := f.r.Tasks.Tracked(f.ctx, nil, at("11:30"))
	require.NoError(t, err)
	require.Empty(t, empty)
}
