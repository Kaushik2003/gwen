package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/model"
	"github.com/stretchr/testify/require"
)

// 00002_planner.sql applies to a v1 database holding data, and the v1
// repositories read and write that data unchanged afterwards.
func TestPlannerMigrationOnV1Data(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	v1 := openTest(t, dir, initOnly(t))
	require.Equal(t, int64(1), v1.SchemaVersion())
	day := func(h int) int64 { return time.Date(2026, 9, 15, h, 0, 0, 0, time.UTC).UnixMilli() }
	for _, q := range []string{
		`INSERT INTO projects (id, name, color, created_at, updated_at, device_id, rev)
			VALUES ('00000000-0000-7000-8000-000000000001', 'Internship', '#10b981', 1, 1, 'dev', 1)`,
		`INSERT INTO tasks (id, project_id, title, priority, due_day, estimate_minutes, created_at, updated_at, device_id, rev)
			VALUES ('00000000-0000-7000-8000-000000000002', '00000000-0000-7000-8000-000000000001', 'Report', 3,
			'2026-09-20', 90, 1, 1, 'dev', 1)`,
	} {
		_, err := v1.SQL().Exec(q)
		require.NoError(t, err)
	}
	_, err := v1.SQL().Exec(`INSERT INTO work_days (id, day, tz, clocked_in_at, clocked_out_at, target_seconds,
		created_at, updated_at, device_id, rev) VALUES ('00000000-0000-7000-8000-000000000003', '2026-09-15', 'UTC',
		?, ?, 28800, 1, 1, 'dev', 1)`, day(9), day(12))
	require.NoError(t, err)
	_, err = v1.SQL().Exec(`INSERT INTO segments (id, work_day_id, kind, source, project_id, task_id, started_at,
		ended_at, created_at, updated_at, device_id, rev) VALUES ('00000000-0000-7000-8000-000000000004',
		'00000000-0000-7000-8000-000000000003', 'work', 'user', '00000000-0000-7000-8000-000000000001',
		'00000000-0000-7000-8000-000000000002', ?, ?, 1, 1, 'dev', 1)`, day(9), day(12))
	require.NoError(t, err)
	require.NoError(t, v1.Close())

	db, err := Open(ctx, dir, clock.NewFake(time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.Equal(t, int64(2), db.SchemaVersion())
	_, err = os.Stat(filepath.Join(dir, backupPrefix+"1"))
	require.NoError(t, err, "the v1 database is backed up first")

	r := NewRepos(db)
	tk, err := r.Tasks.Get(ctx, "00000000-0000-7000-8000-000000000002")
	require.NoError(t, err)
	require.Equal(t, "Report", tk.Title)
	require.Equal(t, 90*time.Minute, *tk.Estimate)
	require.Nil(t, tk.GoalID)
	require.Nil(t, tk.Quantity)
	require.Nil(t, tk.QuantityDone)
	require.Nil(t, tk.RRule)
	require.Nil(t, tk.TemplateID)
	require.Nil(t, tk.OccurrenceDay)

	listed, err := r.Tasks.List(ctx, TaskFilter{})
	require.NoError(t, err)
	require.Len(t, listed, 1)
	tracked, err := r.Tasks.Tracked(ctx, []string{tk.ID}, db.Now())
	require.NoError(t, err)
	require.Equal(t, 3*time.Hour, tracked[tk.ID])

	sum, err := r.Stats.DaySummary(ctx, "00000000-0000-7000-8000-000000000003", db.Now())
	require.NoError(t, err)
	require.Equal(t, 3*time.Hour, sum.Worked)
	require.Equal(t, "Internship", sum.ByProject[0].Name)

	done, err := r.Tasks.Complete(ctx, tk.ID, nil)
	require.NoError(t, err)
	require.Equal(t, model.TaskDone, done.Task.Status)
	require.Nil(t, done.Task.QuantityDone, "a task without a quantity records none")
	require.Empty(t, done.PlanDays)
	require.False(t, done.Goals)

	p, err := r.Projects.Create(ctx, "Study", nil)
	require.NoError(t, err)
	require.NoError(t, r.Projects.Delete(ctx, p.ID))
}
