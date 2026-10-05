package store

import (
	"context"
	"testing"
	"testing/fstest"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/stretchr/testify/require"
)

// migrationsUpTo is the first n embedded migrations.
func migrationsUpTo(t *testing.T, n int) fstest.MapFS {
	t.Helper()
	entries, err := embedded.ReadDir("migrations")
	require.NoError(t, err)
	out := fstest.MapFS{}
	for _, e := range entries[:n] {
		body, err := embedded.ReadFile("migrations/" + e.Name())
		require.NoError(t, err)
		out[e.Name()] = &fstest.MapFile{Data: body}
	}
	return out
}

// 00004_sessions.sql applies to a v3 database holding a goal and its tasks,
// which read back with no daily time and no steps.
func TestSessionsMigrationOnV3Data(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	v3 := openTest(t, dir, migrationsUpTo(t, 3))
	require.Equal(t, int64(3), v3.SchemaVersion())
	for _, q := range []string{
		`INSERT INTO goals (id, title, kind, unit, target_quantity, minutes_per_unit, start_day, due_day,
			created_at, updated_at, device_id, rev) VALUES ('00000000-0000-7000-8000-000000000001', 'DSA',
			'quantity', 'problems', 300, 30, '2026-09-15', '2026-12-15', 1, 1, 'dev', 1)`,
		`INSERT INTO tasks (id, title, goal_id, rrule, created_at, updated_at, device_id, rev)
			VALUES ('00000000-0000-7000-8000-000000000002', 'DSA', '00000000-0000-7000-8000-000000000001',
			'FREQ=DAILY', 1, 1, 'dev', 1)`,
		`INSERT INTO tasks (id, title, goal_id, quantity, created_at, updated_at, device_id, rev)
			VALUES ('00000000-0000-7000-8000-000000000003', 'Arrays set', '00000000-0000-7000-8000-000000000001',
			5, 1, 1, 'dev', 1)`,
	} {
		_, err := v3.SQL().Exec(q)
		require.NoError(t, err)
	}
	require.NoError(t, v3.Close())

	db, err := Open(ctx, dir, clock.NewFake(time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.Equal(t, int64(7), db.SchemaVersion())

	r := NewRepos(db)
	g, err := r.Goals.Get(ctx, "00000000-0000-7000-8000-000000000001")
	require.NoError(t, err)
	require.Nil(t, g.Daily)
	tk, err := r.Tasks.Get(ctx, "00000000-0000-7000-8000-000000000003")
	require.NoError(t, err)
	require.Nil(t, tk.ParentID)
	require.Equal(t, 5, *tk.Quantity)
	_, err = r.Goals.Update(ctx, g.ID, GoalPatch{DailyMinutes: Nullable[int]{Set: true, Value: new(int)}})
	require.ErrorIs(t, err, ErrInvalid, "a daily time is 5 minutes at least")
}
