package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/stretchr/testify/require"
)

var t0 = time.Date(2026, 9, 15, 9, 0, 0, 0, time.UTC)

func openTest(t *testing.T, dir string, migrations fs.FS) *DB {
	t.Helper()
	db, err := open(context.Background(), dir, clock.NewFake(t0), migrations)
	require.NoError(t, err)
	return db
}

func initOnly(t *testing.T) fstest.MapFS {
	t.Helper()
	body, err := embedded.ReadFile("migrations/00001_init.sql")
	require.NoError(t, err)
	return fstest.MapFS{"00001_init.sql": {Data: body}}
}

func tableExists(t *testing.T, q Querier, name string) bool {
	t.Helper()
	var n int
	err := q.QueryRowContext(context.Background(),
		`SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = ?`, name).Scan(&n)
	require.NoError(t, err)
	return n == 1
}

func TestOpenEmptyDirectory(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "data")
	db, err := Open(ctx, dir, clock.NewFake(t0))
	require.NoError(t, err)

	var mode string
	require.NoError(t, db.SQL().QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode))
	require.Equal(t, "wal", mode)

	// foreign_keys is per connection; check several pooled connections at once.
	conns := make([]*sql.Conn, 4)
	for i := range conns {
		c, err := db.SQL().Conn(ctx)
		require.NoError(t, err)
		conns[i] = c
		var fk int
		require.NoError(t, c.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk))
		require.Equal(t, 1, fk)
	}
	for _, c := range conns {
		require.NoError(t, c.Close())
	}

	for _, table := range []string{"local_state", "projects", "tasks", "work_days", "segments", "engine_events",
		"goals", "commitments", "plan_items"} {
		require.True(t, tableExists(t, db.SQL(), table), table)
	}
	require.Equal(t, int64(2), db.SchemaVersion())

	id := db.DeviceID()
	require.Len(t, id, 36)
	require.NoError(t, db.Close())

	again, err := Open(ctx, dir, clock.NewFake(t0.Add(time.Hour)))
	require.NoError(t, err)
	t.Cleanup(func() { again.Close() })
	require.Equal(t, id, again.DeviceID(), "device_id must not change after reopening")

	_, err = os.Stat(filepath.Join(dir, backupPrefix+"0"))
	require.True(t, errors.Is(err, fs.ErrNotExist), "a new database has nothing to back up")
}

func TestOpenFailsOnUnwritableDirectory(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	require.NoError(t, os.Chmod(parent, 0o500))
	t.Cleanup(func() { os.Chmod(parent, 0o700) })
	if os.Getuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	_, err := Open(context.Background(), filepath.Join(parent, "data"), clock.NewFake(t0))
	require.Error(t, err)
}

func TestBackupBeforePendingMigration(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db := openTest(t, dir, initOnly(t))
	_, err := db.SQL().Exec(`INSERT INTO projects (id, name, color, created_at, updated_at, device_id, rev)
		VALUES ('00000000-0000-7000-8000-000000000001', 'Internship', '#10b981', 1, 1, 'dev', 1)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	next := initOnly(t)
	next["00002_test.sql"] = &fstest.MapFile{Data: []byte("-- +goose Up\nCREATE TABLE added_by_two (x INTEGER) STRICT;\n")}
	db = openTest(t, dir, next)
	t.Cleanup(func() { db.Close() })
	require.Equal(t, int64(2), db.SchemaVersion())
	require.True(t, tableExists(t, db.SQL(), "added_by_two"))

	bak, err := sql.Open("sqlite", "file:"+filepath.Join(dir, backupPrefix+"1")+"?mode=ro")
	require.NoError(t, err)
	t.Cleanup(func() { bak.Close() })
	require.False(t, tableExists(t, bak, "added_by_two"), "backup must be taken before the migration applies")
	var name string
	require.NoError(t, bak.QueryRow(`SELECT name FROM projects`).Scan(&name))
	require.Equal(t, "Internship", name)

	info, err := os.Stat(filepath.Join(dir, backupPrefix+"1"))
	require.NoError(t, err)
	require.Equal(t, fs.FileMode(0o600), info.Mode().Perm())
}

func TestPruneBackupsKeepsTwoMostRecent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, name := range []string{backupPrefix + "1", backupPrefix + "2", backupPrefix + "10", "gwen.db", "other"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, 0o600))
	}
	require.NoError(t, pruneBackups(dir))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	require.ElementsMatch(t, []string{backupPrefix + "2", backupPrefix + "10", "gwen.db", "other"}, names)
}

func TestInTx(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openTest(t, t.TempDir(), initOnly(t))
	t.Cleanup(func() { db.Close() })
	set := func(tx *sql.Tx, v string) error { return SetLocal(ctx, tx, KeyHeartbeatAt, v, t0) }

	require.NoError(t, db.InTx(ctx, func(tx *sql.Tx) error { return set(tx, "1") }))
	v, err := GetLocal(ctx, db.SQL(), KeyHeartbeatAt)
	require.NoError(t, err)
	require.Equal(t, "1", v)

	boom := errors.New("boom")
	err = db.InTx(ctx, func(tx *sql.Tx) error {
		require.NoError(t, set(tx, "2"))
		return boom
	})
	require.ErrorIs(t, err, boom)
	v, err = GetLocal(ctx, db.SQL(), KeyHeartbeatAt)
	require.NoError(t, err)
	require.Equal(t, "1", v, "a failed transaction must roll back")

	require.Panics(t, func() {
		_ = db.InTx(ctx, func(tx *sql.Tx) error {
			require.NoError(t, set(tx, "3"))
			panic("bug")
		})
	})
	v, err = GetLocal(ctx, db.SQL(), KeyHeartbeatAt)
	require.NoError(t, err)
	require.Equal(t, "1", v, "a panicking transaction must roll back")
}

func TestLocalState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openTest(t, t.TempDir(), initOnly(t))
	t.Cleanup(func() { db.Close() })
	q := db.SQL()

	_, err := GetLocal(ctx, q, KeyShutdownAt)
	require.ErrorIs(t, err, ErrNotFound)

	at := t0.Add(1500 * time.Millisecond)
	require.NoError(t, SetLocalTime(ctx, q, KeyShutdownAt, at, db.Now()))
	got, err := GetLocalTime(ctx, q, KeyShutdownAt)
	require.NoError(t, err)
	require.True(t, got.Equal(at))

	require.NoError(t, SetLocal(ctx, q, KeyShutdownAt, "garbage", db.Now()))
	_, err = GetLocalTime(ctx, q, KeyShutdownAt)
	require.ErrorIs(t, err, ErrInvalid)

	require.NoError(t, DeleteLocal(ctx, q, KeyShutdownAt))
	require.NoError(t, DeleteLocal(ctx, q, KeyShutdownAt), "deleting an absent key is fine")
	_, err = GetLocal(ctx, q, KeyShutdownAt)
	require.ErrorIs(t, err, ErrNotFound)

	require.ErrorIs(t, SetLocal(ctx, q, "favourite_colour", "green", db.Now()), ErrInvalid)
	require.ErrorIs(t, SetLocal(ctx, q, KeyDeviceID, "forged", db.Now()), ErrInvalid)
	require.ErrorIs(t, DeleteLocal(ctx, q, KeyDeviceID), ErrInvalid)

	var updated int64
	require.NoError(t, q.QueryRowContext(ctx, `SELECT updated_at FROM local_state WHERE key = ?`, KeyDeviceID).Scan(&updated))
	require.Equal(t, t0.UnixMilli(), updated)
}

func TestClassify(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openTest(t, t.TempDir(), initOnly(t))
	t.Cleanup(func() { db.Close() })
	const insertProject = `INSERT INTO projects (id, name, color, created_at, updated_at, device_id, rev)
		VALUES (?, ?, ?, 1, 1, 'dev', 1)`
	_, err := db.SQL().ExecContext(ctx, insertProject, "00000000-0000-7000-8000-000000000001", "A", "#3b82f6")
	require.NoError(t, err)

	tests := []struct {
		name string
		run  func() error
		want error
	}{
		{
			name: "unique",
			run: func() error {
				_, err := db.SQL().ExecContext(ctx, insertProject, "00000000-0000-7000-8000-000000000002", "a", "#3b82f6")
				return err
			},
			want: ErrConflict,
		},
		{
			name: "primary key",
			run: func() error {
				_, err := db.SQL().ExecContext(ctx, insertProject, "00000000-0000-7000-8000-000000000001", "B", "#3b82f6")
				return err
			},
			want: ErrConflict,
		},
		{
			name: "check",
			run: func() error {
				_, err := db.SQL().ExecContext(ctx, insertProject, "00000000-0000-7000-8000-000000000003", "C", "blue")
				return err
			},
			want: ErrInvalid,
		},
		{
			name: "foreign key",
			run: func() error {
				_, err := db.SQL().ExecContext(ctx, `INSERT INTO tasks (id, project_id, title, created_at, updated_at, device_id, rev)
					VALUES ('00000000-0000-7000-8000-000000000009', '00000000-0000-7000-8000-00000000dead', 'T', 1, 1, 'dev', 1)`)
				return err
			},
			want: ErrNotFound,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			require.Error(t, err)
			require.ErrorIs(t, classify(err), tc.want)
		})
	}
	other := errors.New("other")
	require.Same(t, other, classify(other))
}

func TestError(t *testing.T) {
	t.Parallel()
	err := fmt.Errorf("patch segment %s: %w", "abc", FailField(ErrConflict, "started_at", "segments may not overlap"))
	require.ErrorIs(t, err, ErrConflict)
	require.NotErrorIs(t, err, ErrInvalid)
	var se *Error
	require.ErrorAs(t, err, &se)
	require.Equal(t, "segments may not overlap", se.Message)
	require.Equal(t, "started_at", se.Field)
	require.Equal(t, "patch segment abc: segments may not overlap", err.Error())

	plain := Fail(ErrNotFound, "no project %s", "2c3d4e5f")
	require.ErrorIs(t, plain, ErrNotFound)
	require.Equal(t, "no project 2c3d4e5f", plain.Error())
}
