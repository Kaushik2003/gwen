package store_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/stretchr/testify/require"
)

func projectRow(id, name string, updated int64, device string) store.Row {
	return store.Row{"id": id, "name": name, "color": "#10b981", "archived_at": nil, "created_at": int64(1),
		"updated_at": updated, "deleted_at": nil, "device_id": device, "rev": int64(1)}
}

const pid = "00000000-0000-7000-8000-00000000000a"

func (f *fx) projectName(id string) string {
	f.t.Helper()
	var name string
	require.NoError(f.t, f.db.SQL().QueryRow(`SELECT name FROM projects WHERE id = ?`, id).Scan(&name))
	return name
}

func TestApplyIsLastWriterWins(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	apply := func(r store.Row) store.Applied {
		t.Helper()
		res, err := f.r.Sync.Apply(f.ctx, store.Rows{"projects": {r}}, true)
		require.NoError(t, err)
		return res
	}
	require.Equal(t, 1, apply(projectRow(pid, "First", 100, "dev-b")).Applied)
	require.Equal(t, 1, apply(projectRow(pid, "Newer", 200, "dev-b")).Applied, "newer wins")
	require.Equal(t, "Newer", f.projectName(pid))
	require.Equal(t, 1, apply(projectRow(pid, "Older", 150, "dev-z")).Ignored, "older is ignored")
	require.Equal(t, "Newer", f.projectName(pid))
	require.Equal(t, 1, apply(projectRow(pid, "Same time, lower device", 200, "dev-a")).Ignored)
	require.Equal(t, 1, apply(projectRow(pid, "Same time, higher device", 200, "dev-c")).Applied)
	require.Equal(t, "Same time, higher device", f.projectName(pid))
	require.Equal(t, 1, apply(projectRow(pid, "Same time, higher device", 200, "dev-c")).Ignored, "a re-push is a no-op")

	var n int
	require.NoError(t, f.db.SQL().QueryRow(`SELECT count(*) FROM hub_changes`).Scan(&n))
	require.Equal(t, 1, n, "one entry per row")
}

func TestApplySkipsAConstraintViolation(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	day := func(id, d string) store.Row {
		return store.Row{"id": id, "day": d, "tz": "UTC", "clocked_in_at": int64(1), "clocked_out_at": nil,
			"target_seconds": int64(28800), "note": "", "created_at": int64(1), "updated_at": int64(5),
			"deleted_at": nil, "device_id": "dev-b", "rev": int64(1)}
	}
	res, err := f.r.Sync.Apply(f.ctx, store.Rows{
		"work_days": {day("00000000-0000-7000-8000-000000000001", "2026-09-14"),
			day("00000000-0000-7000-8000-000000000002", "2026-09-15")}, // a second open day
		"projects": {projectRow(pid, "Kept", 1, "dev-b")},
	}, false)
	require.NoError(t, err)
	require.Equal(t, 2, res.Applied)
	require.Equal(t, 1, res.Ignored)
	require.Equal(t, []string{"00000000-0000-7000-8000-000000000001"}, res.IDs["work_days"])
	require.Equal(t, "Kept", f.projectName(pid))

	_, err = f.r.Sync.Apply(f.ctx, store.Rows{"secrets": {{"id": "x"}}}, false)
	userErr(t, err, store.ErrInvalid, "rows")
	bad := projectRow(pid, "x", 1, "d")
	delete(bad, "rev")
	_, err = f.r.Sync.Apply(f.ctx, store.Rows{"projects": {bad}}, false)
	userErr(t, err, store.ErrInvalid, "rows")
}

func TestPullPagesAndExcludesTheRequester(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	var rows []store.Row
	for i := range 5 {
		device := "dev-a"
		if i%2 == 1 {
			device = "dev-b"
		}
		rows = append(rows, projectRow("00000000-0000-7000-8000-00000000010"+string(rune('0'+i)), "P"+string(rune('0'+i)), int64(10+i), device))
	}
	_, err := f.r.Sync.Apply(f.ctx, store.Rows{"projects": rows}, true)
	require.NoError(t, err)

	got, cursor, more, err := f.r.Sync.Pull(f.ctx, "dev-b", 0, 2)
	require.NoError(t, err)
	require.True(t, more)
	require.Equal(t, []string{"P0", "P2"}, names(got))
	got, cursor, more, err = f.r.Sync.Pull(f.ctx, "dev-b", cursor, 2)
	require.NoError(t, err)
	require.False(t, more)
	require.Equal(t, []string{"P4"}, names(got))
	got, _, more, err = f.r.Sync.Pull(f.ctx, "dev-b", cursor, 2)
	require.NoError(t, err)
	require.False(t, more)
	require.Empty(t, got.Count())

	// Rewriting a row moves it to the end.
	_, err = f.r.Sync.Apply(f.ctx, store.Rows{"projects": {projectRow(rows[0]["id"].(string), "P0 again", 99, "dev-a")}}, true)
	require.NoError(t, err)
	got, _, _, err = f.r.Sync.Pull(f.ctx, "dev-b", cursor, 10)
	require.NoError(t, err)
	require.Equal(t, []string{"P0 again"}, names(got))
}

func names(r store.Rows) []string {
	out := []string{}
	for _, row := range r["projects"] {
		out = append(out, row["name"].(string))
	}
	return out
}

func TestChangedAndRoundTrip(t *testing.T) {
	t.Parallel()
	src := newFx(t)
	p := src.project("Study")
	src.clk.Advance(time.Minute)
	tk := src.newTask(store.NewTask{Title: "Read", ProjectID: &p.ID})
	src.plan("")

	changed, err := src.r.Sync.Changed(src.ctx, time.UnixMilli(0))
	require.NoError(t, err)
	var tables []string
	rows := store.Rows{}
	for _, tr := range changed {
		if len(tables) == 0 || tables[len(tables)-1] != tr.Table {
			tables = append(tables, tr.Table)
		}
		rows[tr.Table] = append(rows[tr.Table], tr.Row)
	}
	require.Equal(t, []string{"projects", "tasks", "plan_items"}, tables, "dependency order")
	later, err := src.r.Sync.Changed(src.ctx, src.clk.Now().Add(time.Second))
	require.NoError(t, err)
	require.Empty(t, later)

	// The rows survive JSON and apply to another database unchanged.
	b, err := json.Marshal(rows)
	require.NoError(t, err)
	decoded, err := store.DecodeRows(b)
	require.NoError(t, err)
	dst := newFx(t)
	res, err := dst.r.Sync.Apply(dst.ctx, decoded, false)
	require.NoError(t, err)
	require.Equal(t, rows.Count(), res.Applied)
	got, err := dst.r.Tasks.Get(dst.ctx, tk.ID)
	require.NoError(t, err)
	require.Equal(t, tk.Title, got.Title)
	require.Equal(t, src.db.DeviceID(), got.DeviceID, "the writer's device is kept")
	require.Equal(t, model.TaskOpen, got.Status)
	require.Equal(t, tk.CreatedAt, got.CreatedAt)
}
