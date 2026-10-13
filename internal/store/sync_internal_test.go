package store

import (
	"context"
	"io/fs"
	"maps"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Every synced table syncs all of its columns, so a row loses nothing on the
// way to another device.
func TestSyncTablesHaveEveryColumn(t *testing.T) {
	t.Parallel()
	migrations, err := fs.Sub(embedded, "migrations")
	require.NoError(t, err)
	db := openTest(t, t.TempDir(), migrations)
	for _, st := range syncTables {
		cols, err := queryIDs(context.Background(), db.sql, `SELECT name FROM pragma_table_info(?)`, st.name)
		require.NoError(t, err)
		require.ElementsMatch(t, cols, st.columns(), st.name)
		require.Equal(t, "id", st.cols[0], st.name)
		require.Contains(t, []string{modeLWW, modeAppend}, st.mode, st.name)
		require.LessOrEqual(t, st.since, db.SchemaVersion(), st.name)
	}
}

// A database whose schema predates a synced table neither sends, takes, nor
// serves that table's rows, and the rest of sync goes on.
func TestSyncSkipsTablesTheSchemaLacks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openTest(t, t.TempDir(), migrationsUpTo(t, int(LegacySyncSchema)))
	require.Equal(t, LegacySyncSchema, db.SchemaVersion())
	r := NewSyncRepo(db)
	withEnvelope := func(row Row) Row {
		maps.Copy(row, Row{"created_at": int64(1), "updated_at": int64(10), "deleted_at": nil, "device_id": "dev-b",
			"rev": int64(1)})
		return row
	}
	const (
		project = "00000000-0000-7000-8000-00000000000a"
		thread  = "00000000-0000-7000-8000-0000000000c1"
	)
	res, err := r.Apply(ctx, Rows{
		"projects":     {withEnvelope(Row{"id": project, "name": "P", "color": "#10b981", "archived_at": nil})},
		"chat_threads": {withEnvelope(Row{"id": thread, "title": "Chat"})},
	}, true)
	require.NoError(t, err)
	require.Equal(t, 1, res.Applied)
	require.Equal(t, 1, res.Ignored, "chat syncs from schema 10")
	require.Equal(t, []string{project}, res.IDs["projects"])

	_, err = db.sql.Exec(`INSERT INTO chat_threads (id, title, created_at, updated_at, device_id, rev)
		VALUES (?, 'Chat', 1, 10, 'dev-b', 1)`, thread)
	require.NoError(t, err)
	changed, err := r.Changed(ctx, time.UnixMilli(0), nil)
	require.NoError(t, err)
	require.Len(t, changed, 1)
	require.Equal(t, "projects", changed[0].Table)

	_, err = db.sql.Exec(`INSERT INTO hub_changes (table_name, row_id, device_id) VALUES ('chat_threads', ?, 'dev-b')`,
		thread)
	require.NoError(t, err)
	rows, cursor, more, err := r.Pull(ctx, "dev-z", 0, 10, 10)
	require.NoError(t, err)
	require.False(t, more)
	require.Equal(t, int64(2), cursor)
	require.Equal(t, 1, rows.Count())
	require.Len(t, rows["projects"], 1)
}
