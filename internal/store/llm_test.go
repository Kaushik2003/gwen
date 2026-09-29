package store_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestLLMRunAcceptAndReject(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	p := f.project("Study")
	ng := tasksGoal()
	ng.ProjectID = &p.ID
	g, _ := f.goal(ng)
	output := map[string]any{"tasks": []store.ProposedTask{
		{Title: "Outline", EstimateMinutes: 60, DueDay: "2026-09-25", Priority: 3},
		{Title: "Draft", Notes: "chapter 1", EstimateMinutes: 240, DueDay: "2026-10-10", Priority: 2, Quantity: testutil.Ptr(2)},
		{Title: "Skip me", EstimateMinutes: 30, DueDay: "2026-10-10", Priority: 1},
	}}
	run, err := f.r.LLMRuns.Create(f.ctx, model.RunBreakdown, g.ID, model.RunOK, output)
	require.NoError(t, err)
	got, err := f.r.LLMRuns.Get(f.ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, run, got)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(got.Output, &decoded))

	for _, bad := range [][]int{{}, {3}, {-1}, {0, 0}} {
		_, err := f.r.LLMRuns.Accept(f.ctx, run.ID, bad)
		userErr(t, err, store.ErrInvalid, "indexes")
	}
	tasks, err := f.r.LLMRuns.Accept(f.ctx, run.ID, []int{1, 0})
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	require.Equal(t, "Draft", tasks[0].Title)
	require.Equal(t, g.ID, *tasks[0].GoalID)
	require.Equal(t, p.ID, *tasks[0].ProjectID)
	require.Equal(t, 4*time.Hour, *tasks[0].Estimate)
	require.Equal(t, 2, *tasks[0].Quantity)
	require.Equal(t, "chapter 1", tasks[0].Notes)
	linked, err := f.r.Tasks.List(f.ctx, store.TaskFilter{GoalID: g.ID})
	require.NoError(t, err)
	require.Len(t, linked, 2)
	got, err = f.r.LLMRuns.Get(f.ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, model.RunAccepted, got.Status)
	_, err = f.r.LLMRuns.Accept(f.ctx, run.ID, []int{2})
	require.ErrorIs(t, err, store.ErrConflict, "accepted once")
	_, err = f.r.LLMRuns.Reject(f.ctx, run.ID)
	require.ErrorIs(t, err, store.ErrConflict)

	other, err := f.r.LLMRuns.Create(f.ctx, model.RunBreakdown, g.ID, model.RunOK, output)
	require.NoError(t, err)
	rejected, err := f.r.LLMRuns.Reject(f.ctx, other.ID)
	require.NoError(t, err)
	require.Equal(t, model.RunRejected, rejected.Status)

	failed, err := f.r.LLMRuns.Create(f.ctx, model.RunBreakdown, g.ID, model.RunFailed, map[string]string{"error": "x"})
	require.NoError(t, err)
	_, err = f.r.LLMRuns.Accept(f.ctx, failed.ID, []int{0})
	require.ErrorIs(t, err, store.ErrConflict)
	_, err = f.r.LLMRuns.Get(f.ctx, "nope")
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestRetroReads(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	a := f.newTask(store.NewTask{Title: "Report", EstimateMinutes: testutil.Ptr(60)})
	f.newTask(store.NewTask{Title: "Open", EstimateMinutes: testutil.Ptr(30)})
	f.plan("")
	_, err := f.r.Tasks.Complete(f.ctx, a.ID, nil)
	require.NoError(t, err)
	planned, done, err := f.r.LLMRuns.PlanMinutes(f.ctx, "2026-09-14", fxToday)
	require.NoError(t, err)
	require.Equal(t, [2]int{90, 60}, [2]int{planned, done})
	titles, err := f.r.LLMRuns.Completed(f.ctx, testutil.AtOn("2026-09-14", "00:00"), testutil.AtOn("2026-09-21", "00:00"))
	require.NoError(t, err)
	require.Equal(t, []string{"Report"}, titles)
}
