package store

import (
	"context"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/stretchr/testify/require"
)

// 00005_day_plans.sql keeps a v4 database's LLM runs and allows day plans.
func TestDayPlansMigrationOnV4Data(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	v4 := openTest(t, dir, migrationsUpTo(t, 4))
	require.Equal(t, int64(4), v4.SchemaVersion())
	_, err := v4.SQL().Exec(`INSERT INTO llm_runs (id, kind, subject_id, status, output, created_at, updated_at)
		VALUES ('00000000-0000-7000-8000-000000000001', 'retro', '2026-09-07', 'ok', '{"markdown": "Quiet."}', 1, 2)`)
	require.NoError(t, err)
	require.NoError(t, v4.Close())

	db, err := Open(ctx, dir, clock.NewFake(time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.Equal(t, int64(7), db.SchemaVersion())
	r := NewRepos(db)
	run, err := r.LLMRuns.Get(ctx, "00000000-0000-7000-8000-000000000001")
	require.NoError(t, err)
	require.Equal(t, "retro", run.Kind)
	require.JSONEq(t, `{"markdown": "Quiet."}`, string(run.Output))
	_, err = r.LLMRuns.Create(ctx, "day_plan", "2026-09-20", "ok", map[string]any{})
	require.NoError(t, err)
	_, err = r.LLMRuns.Create(ctx, "chat", "2026-09-20", "ok", map[string]any{})
	require.Error(t, err, "the kind is still checked")
}
