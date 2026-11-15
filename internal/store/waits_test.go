package store_test

import (
	"testing"
	"time"

	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestTaskWaits(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	api, app, qa := f.task("API", nil), f.task("App", nil), f.task("QA", nil)

	got, err := f.r.Tasks.Update(f.ctx, app.ID, store.TaskPatch{WaitsOn: &[]string{api.ID, " " + api.ID, ""}})
	require.NoError(t, err)
	require.Equal(t, []string{api.ID}, got.WaitsOn, "trimmed, without repeats or blanks")
	read, err := f.r.Tasks.Get(f.ctx, app.ID)
	require.NoError(t, err)
	require.Equal(t, []string{api.ID}, read.WaitsOn)

	// Other edits keep it.
	title := "App screens"
	got, err = f.r.Tasks.Update(f.ctx, app.ID, store.TaskPatch{Title: &title})
	require.NoError(t, err)
	require.Equal(t, []string{api.ID}, got.WaitsOn)

	_, err = f.r.Tasks.Update(f.ctx, qa.ID, store.TaskPatch{WaitsOn: &[]string{app.ID}})
	require.NoError(t, err)

	t.Run("a loop is refused", func(t *testing.T) {
		_, err := f.r.Tasks.Update(f.ctx, api.ID, store.TaskPatch{WaitsOn: &[]string{qa.ID}})
		userErr(t, err, store.ErrConflict, "waits_on")
		require.Contains(t, err.Error(), "App screens")
	})
	t.Run("itself", func(t *testing.T) {
		_, err := f.r.Tasks.Update(f.ctx, api.ID, store.TaskPatch{WaitsOn: &[]string{api.ID}})
		userErr(t, err, store.ErrInvalid, "waits_on")
	})
	t.Run("a task that isn't there is dropped", func(t *testing.T) {
		got, err := f.r.Tasks.Update(f.ctx, api.ID, store.TaskPatch{WaitsOn: &[]string{"00000000-0000-0000-0000-000000000000"}})
		require.NoError(t, err)
		require.Empty(t, got.WaitsOn)
	})
	t.Run("on create", func(t *testing.T) {
		tk, _, err := f.r.Tasks.Create(f.ctx, store.NewTask{Title: "Ship", EstimateMinutes: testutil.Ptr(30),
			WaitsOn: []string{qa.ID, app.ID}})
		require.NoError(t, err)
		require.Equal(t, []string{qa.ID, app.ID}, tk.WaitsOn)
	})
	t.Run("cleared", func(t *testing.T) {
		got, err := f.r.Tasks.Update(f.ctx, qa.ID, store.TaskPatch{WaitsOn: &[]string{}})
		require.NoError(t, err)
		require.Empty(t, got.WaitsOn)
	})
}

func TestAgentSpans(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	start := testutil.AtOn("2026-09-20", "09:00")
	s := store.CodeSession{ID: "abcdefghijklm", Cwd: "/w/app", State: store.CodeStarting, StartedAt: start, UpdatedAt: start}
	save := func(state, need, task string, at time.Time) {
		t.Helper()
		s.State, s.Need, s.TaskID, s.UpdatedAt = state, need, task, at
		if state == store.CodeEnded {
			s.EndedAt = &at
		}
		require.NoError(t, f.r.CodeSessions.Save(f.ctx, s))
	}
	save(store.CodeStarting, "", "", start)
	save(store.CodeWorking, "", "", start.Add(time.Minute))
	save(store.CodeWorking, "", "", start.Add(5*time.Minute)) // a step, not a new state
	save(store.CodeWorking, "", "task-1", start.Add(6*time.Minute))
	save(store.CodeNeedsYou, store.NeedPermission, "task-1", start.Add(20*time.Minute))
	save(store.CodeWorking, "", "task-1", start.Add(25*time.Minute))

	spans, err := f.r.CodeSessions.Spans(f.ctx, start, start.Add(time.Hour))
	require.NoError(t, err)
	type row struct {
		state, need, task string
		from, to          int // minutes after 09:00; -1 still open
	}
	var got []row
	for _, sp := range spans {
		r := row{sp.State, sp.Need, sp.TaskID, int(sp.StartedAt.Sub(start) / time.Minute), -1}
		if sp.EndedAt != nil {
			r.to = int(sp.EndedAt.Sub(start) / time.Minute)
		}
		require.Equal(t, "/w/app", sp.Cwd)
		got = append(got, r)
	}
	require.Equal(t, []row{
		{store.CodeStarting, "", "", 0, 1},
		{store.CodeWorking, "", "", 1, 6},
		{store.CodeWorking, "", "task-1", 6, 20},
		{store.CodeNeedsYou, store.NeedPermission, "task-1", 20, 25},
		{store.CodeWorking, "", "task-1", 25, -1},
	}, got)

	save(store.CodeEnded, "", "task-1", start.Add(40*time.Minute))
	spans, err = f.r.CodeSessions.Spans(f.ctx, start.Add(30*time.Minute), start.Add(time.Hour))
	require.NoError(t, err)
	require.Len(t, spans, 1, "only what overlaps the window")
	require.Equal(t, start.Add(40*time.Minute), *spans[0].EndedAt, "ending closes the open span")

	require.NoError(t, f.r.CodeSessions.Delete(f.ctx, s.ID))
	spans, err = f.r.CodeSessions.Spans(f.ctx, start, start.Add(time.Hour))
	require.NoError(t, err)
	require.Empty(t, spans)
}
