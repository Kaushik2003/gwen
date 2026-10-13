package store_test

import (
	"database/sql"
	"encoding/json"
	"maps"
	"slices"
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

const (
	pid      = "00000000-0000-7000-8000-00000000000a"
	threadID = "00000000-0000-7000-8000-0000000000c1"
	msgID    = "00000000-0000-7000-8000-0000000000c2"
)

func threadRow(updated int64, deleted any) store.Row {
	return store.Row{"id": threadID, "title": "The cat", "created_at": int64(1), "updated_at": updated,
		"deleted_at": deleted, "device_id": "dev-b", "rev": int64(1)}
}

func messageRow(id, text string, updated int64, deleted any) store.Row {
	return store.Row{"id": id, "thread_id": threadID, "role": "user", "text": text, "at": int64(1), "mood": "",
		"attitude": "", "actions": "[]", "memory": "{}", "created_at": int64(1), "updated_at": updated,
		"deleted_at": deleted, "device_id": "dev-b", "rev": int64(1)}
}

func claimRow(id, sensitivity string, updated int64) store.Row {
	return store.Row{"id": id, "key": "user.pet", "subject_id": nil, "value": `"a cat"`, "text": "They have a cat, Miso",
		"cardinality": "many", "status": "active", "trust": "owner_said", "sensitivity": sensitivity,
		"confidence": 0.75, "valid_from": int64(1), "valid_to": nil, "occurred_at": nil, "mentioned_at": int64(1),
		"recorded_at": int64(2), "superseded_at": nil, "superseded_by": nil, "suppressed": int64(0),
		"source_kind": "msg", "source_id": msgID, "span": "My cat Miso", "source_device_id": "dev-b",
		"channel": "chat", "extractor": "test", "content_hash": "c0ffee", "created_at": int64(1),
		"updated_at": updated, "deleted_at": nil, "device_id": "dev-b", "rev": int64(1)}
}

func episodeRow(id, sensitivity string, updated int64) store.Row {
	return store.Row{"id": id, "kind": "note", "channel": "dashboard", "speaker": "user", "title": "",
		"body": "Miso sees the vet on Friday", "source": "", "at": int64(1), "sensitivity": sensitivity,
		"content_hash": "c0ffee", "created_at": int64(1), "updated_at": updated, "deleted_at": nil,
		"device_id": "dev-b", "rev": int64(1)}
}

func conflictRow(id, sensitivity string, updated int64) store.Row {
	return store.Row{"id": id, "claim_a": "00000000-0000-7000-8000-0000000000d1",
		"claim_b": "00000000-0000-7000-8000-0000000000d2", "reason": "two pets", "sensitivity": sensitivity,
		"status": "open", "resolution": nil, "resolved_at": nil, "created_at": int64(1), "updated_at": updated,
		"deleted_at": nil, "device_id": "dev-b", "rev": int64(1)}
}

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

	for _, c := range []string{"id", "created_at", "updated_at", "rev", "device_id"} {
		bad := projectRow(pid, "x", 1, "d")
		delete(bad, c)
		_, err = f.r.Sync.Apply(f.ctx, store.Rows{"projects": {bad}}, false)
		userErr(t, err, store.ErrInvalid, "rows")
		bad[c] = 1.5
		_, err = f.r.Sync.Apply(f.ctx, store.Rows{"projects": {bad}}, false)
		userErr(t, err, store.ErrInvalid, "rows")
	}
	bad := projectRow(pid, "x", 1, "d")
	bad["archived_at"] = "yesterday"
	res, err = f.r.Sync.Apply(f.ctx, store.Rows{"projects": {bad}}, false)
	require.NoError(t, err)
	require.Equal(t, 1, res.Ignored, "a value of the wrong type skips the row")
}

// Rows from a newer device keep sync working: a table this one does not
// have is skipped, a column it does not have is dropped, and a column the
// row lacks takes its default.
func TestApplyToleratesWhatItDoesNotKnow(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	extra := projectRow(pid, "Extra", 10, "dev-b")
	extra["flavor"] = "mint"
	noArchive := projectRow("00000000-0000-7000-8000-00000000000b", "No archive", 10, "dev-b")
	delete(noArchive, "archived_at")
	delete(noArchive, "deleted_at")
	noColor := projectRow("00000000-0000-7000-8000-00000000000c", "No color", 10, "dev-b")
	delete(noColor, "color")
	res, err := f.r.Sync.Apply(f.ctx, store.Rows{
		"projects":  {extra, noArchive, noColor},
		"secrets":   {{"id": "x"}, {"id": "y"}},
		"run_steps": {{"id": "z"}},
	}, true)
	require.NoError(t, err)
	require.Equal(t, 2, res.Applied)
	require.Equal(t, 4, res.Ignored, "two unknown tables' rows, and a row lacking a column with no default")
	require.Equal(t, []string{pid, noArchive["id"].(string)}, res.IDs["projects"])
	require.Equal(t, "Extra", f.projectName(pid))
	require.Equal(t, "No archive", f.projectName(noArchive["id"].(string)))

	// A message from before the memory column gets an empty one.
	old := messageRow(msgID, "My cat Miso is ill", 20, nil)
	delete(old, "memory")
	res, err = f.r.Sync.Apply(f.ctx, store.Rows{"chat_threads": {threadRow(20, nil)}, "chat_messages": {old}}, false)
	require.NoError(t, err)
	require.Equal(t, 2, res.Applied)
	var memory string
	require.NoError(t, f.db.SQL().QueryRow(`SELECT memory FROM chat_messages WHERE id = ?`, msgID).Scan(&memory))
	require.Equal(t, "{}", memory)
}

// A row dated more than an hour ahead would win every later write, so it is
// skipped until clocks agree.
func TestApplySkipsFutureRows(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	now := f.clk.Now()
	res, err := f.r.Sync.Apply(f.ctx, store.Rows{"projects": {
		projectRow(pid, "Far ahead", now.Add(2*time.Hour).UnixMilli(), "dev-b"),
		projectRow("00000000-0000-7000-8000-00000000000b", "Close", now.Add(30*time.Minute).UnixMilli(), "dev-b"),
	}}, true)
	require.NoError(t, err)
	require.Equal(t, 1, res.Applied)
	require.Equal(t, 1, res.Ignored)
	_, err = f.r.Projects.Get(f.ctx, pid)
	require.ErrorIs(t, err, store.ErrNotFound)
	require.Equal(t, "Close", f.projectName("00000000-0000-7000-8000-00000000000b"))
}

// Append-only rows are written once: a later edit is ignored, and only a
// tombstone changes them, erasing their words from search.
func TestApplyAppendOnlyRows(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	apply := func(rows store.Rows) store.Applied {
		t.Helper()
		res, err := f.r.Sync.Apply(f.ctx, rows, true)
		require.NoError(t, err)
		return res
	}
	matches := func(word string) int {
		t.Helper()
		var n int
		require.NoError(t, f.db.SQL().QueryRow(`SELECT count(*) FROM chat_messages_fts WHERE chat_messages_fts MATCH ?`,
			word).Scan(&n))
		return n
	}
	text := func() (string, bool) {
		t.Helper()
		var s string
		var deleted sql.NullInt64
		require.NoError(t, f.db.SQL().QueryRow(`SELECT text, deleted_at FROM chat_messages WHERE id = ?`, msgID).
			Scan(&s, &deleted))
		return s, deleted.Valid
	}
	require.Equal(t, 2, apply(store.Rows{"chat_threads": {threadRow(10, nil)},
		"chat_messages": {messageRow(msgID, "My cat Miso is ill", 10, nil)}}).Applied)
	require.Equal(t, 1, matches("miso"))

	edited := apply(store.Rows{"chat_messages": {messageRow(msgID, "My dog Rex is ill", 20, nil)}})
	require.Equal(t, 1, edited.Ignored, "a newer edit does not change a stored message")
	got, deleted := text()
	require.Equal(t, "My cat Miso is ill", got)
	require.False(t, deleted)

	require.Equal(t, 1, apply(store.Rows{"chat_messages": {messageRow(msgID, "", 30, int64(30))}}).Applied)
	got, deleted = text()
	require.Empty(t, got)
	require.True(t, deleted)
	require.Zero(t, matches("miso"), "the forgotten words are gone from search")

	require.Equal(t, 1, apply(store.Rows{"chat_messages": {messageRow(msgID, "", 40, int64(40))}}).Ignored,
		"a second tombstone changes nothing")
	require.Equal(t, 1, apply(store.Rows{"chat_messages": {messageRow(msgID, "My cat Miso is ill", 50, nil)}}).Ignored,
		"nothing brings a tombstoned row back")
	got, deleted = text()
	require.Empty(t, got)
	require.True(t, deleted)
	require.Zero(t, matches("miso"))
}

// Sensitive memory never leaves the device: it is not pushed, and a hub
// that somehow holds it does not serve it.
func TestSensitiveMemoryStaysOnTheDevice(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	const (
		personal  = "00000000-0000-7000-8000-0000000000d1"
		sensitive = "00000000-0000-7000-8000-0000000000d2"
	)
	_, err := f.r.Sync.Apply(f.ctx, store.Rows{
		"claims":    {claimRow(personal, "personal", 10), claimRow(sensitive, "sensitive", 10)},
		"episodes":  {episodeRow(personal, "personal", 10), episodeRow(sensitive, "sensitive", 10)},
		"conflicts": {conflictRow(personal, "personal", 10), conflictRow(sensitive, "sensitive", 10)},
	}, true)
	require.NoError(t, err)

	changed, err := f.r.Sync.Changed(f.ctx, time.UnixMilli(0), nil)
	require.NoError(t, err)
	var got []string
	for _, tr := range changed {
		got = append(got, tr.Table+" "+tr.Row["id"].(string))
	}
	require.Equal(t, []string{"episodes " + personal, "claims " + personal, "conflicts " + personal}, got)

	pulled, _, _, err := f.r.Sync.Pull(f.ctx, "dev-z", 0, 100, 10)
	require.NoError(t, err)
	require.Equal(t, 3, pulled.Count())
	for table, rows := range pulled {
		require.Len(t, rows, 1, table)
		require.Equal(t, personal, rows[0]["id"], table)
	}
}

// Changed reads only the tables it is asked for.
func TestChangedFiltersTables(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	_, err := f.r.Sync.Apply(f.ctx, store.Rows{"projects": {projectRow(pid, "P", 10, "dev-b")},
		"claims": {claimRow("00000000-0000-7000-8000-0000000000d1", "personal", 10)}}, false)
	require.NoError(t, err)
	tables := func(names []string) []string {
		t.Helper()
		changed, err := f.r.Sync.Changed(f.ctx, time.UnixMilli(0), names)
		require.NoError(t, err)
		out := []string{}
		for _, tr := range changed {
			out = append(out, tr.Table)
		}
		return out
	}
	require.Equal(t, []string{"projects", "claims"}, tables(nil))
	require.Equal(t, []string{"claims"}, tables([]string{"claims", "entities"}))
	require.Empty(t, tables([]string{}))
}

// The synced tables, in dependency order, and the schema version each
// started syncing at.
func TestSyncTables(t *testing.T) {
	t.Parallel()
	base := []string{"projects", "goals", "tasks", "commitments", "work_days", "segments", "plan_items", "day_hours",
		"weekly_reviews", "energy_logs"}
	memory := []string{"chat_threads", "chat_messages", "self_versions", "entities", "episodes", "claims", "conflicts",
		"memory_links"}
	require.Equal(t, append(append([]string{}, base...), memory...), store.SyncTables)
	require.Equal(t, base, store.SyncTablesBetween(0, store.LegacySyncSchema))
	require.Equal(t, memory, store.SyncTablesBetween(store.LegacySyncSchema, 10))
	require.Equal(t, store.SyncTables, store.SyncTablesBetween(0, 99))
	require.Empty(t, store.SyncTablesBetween(10, 99))
}

// A client gets only the tables its schema has; the rows it cannot take
// still move its cursor on.
func TestPullHoldsBackTablesTheClientLacks(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	_, err := f.r.Sync.Apply(f.ctx, store.Rows{
		"projects":      {projectRow(pid, "P", 10, "dev-a")},
		"chat_threads":  {threadRow(10, nil)},
		"chat_messages": {messageRow(msgID, "My cat Miso is ill", 10, nil)},
		"claims":        {claimRow("00000000-0000-7000-8000-0000000000d1", "personal", 10)},
	}, true)
	require.NoError(t, err)

	got, cursor, more, err := f.r.Sync.Pull(f.ctx, "dev-z", 0, 10, store.LegacySyncSchema)
	require.NoError(t, err)
	require.False(t, more)
	require.Equal(t, []string{"projects"}, tablesOf(got))
	require.Equal(t, int64(4), cursor, "past the rows it was not sent")

	got, cursor, more, err = f.r.Sync.Pull(f.ctx, "dev-z", 1, 2, store.LegacySyncSchema)
	require.NoError(t, err)
	require.True(t, more)
	require.Zero(t, got.Count())
	require.Equal(t, int64(3), cursor, "a page of nothing it can take still moves on")
	got, cursor, more, err = f.r.Sync.Pull(f.ctx, "dev-z", cursor, 2, store.LegacySyncSchema)
	require.NoError(t, err)
	require.False(t, more)
	require.Zero(t, got.Count())
	require.Equal(t, int64(4), cursor)

	got, cursor, _, err = f.r.Sync.Pull(f.ctx, "dev-z", 0, 10, 10)
	require.NoError(t, err)
	require.Equal(t, []string{"chat_messages", "chat_threads", "claims", "projects"}, tablesOf(got))
	require.Equal(t, int64(4), cursor)
}

func tablesOf(r store.Rows) []string {
	out := []string{}
	for table := range r {
		out = append(out, table)
	}
	slices.Sort(out)
	return out
}

// Every memory table survives the trip through JSON to another database,
// fractions included.
func TestMemoryRoundTrip(t *testing.T) {
	t.Parallel()
	src := newFx(t)
	const (
		claimID  = "00000000-0000-7000-8000-0000000000d1"
		entityID = "00000000-0000-7000-8000-0000000000e1"
		otherID  = "00000000-0000-7000-8000-0000000000e2"
	)
	env := func(r store.Row) store.Row {
		maps.Copy(r, store.Row{"created_at": int64(1), "updated_at": int64(10), "deleted_at": nil, "device_id": "dev-b",
			"rev": int64(1)})
		return r
	}
	claim := claimRow(claimID, "personal", 10)
	claim["subject_id"] = entityID
	_, err := src.r.Sync.Apply(src.ctx, store.Rows{
		"chat_threads":  {threadRow(10, nil)},
		"chat_messages": {messageRow(msgID, "My cat Miso is ill", 10, nil)},
		"self_versions": {env(store.Row{"id": otherID, "attitude": "warm", "note": "They like short answers",
			"since": int64(5), "reason": "chat", "thread_id": threadID})},
		"entities":  {env(store.Row{"id": entityID, "kind": "pet", "name": "Miso", "aliases": `["the cat"]`})},
		"episodes":  {episodeRow(otherID, "personal", 10)},
		"claims":    {claim},
		"conflicts": {conflictRow(otherID, "personal", 10)},
		"memory_links": {env(store.Row{"id": otherID, "from_kind": "claim", "from_id": claimID, "to_kind": "entity",
			"to_id": entityID, "relation": "about"})},
	}, false)
	require.NoError(t, err)

	changed, err := src.r.Sync.Changed(src.ctx, time.UnixMilli(0), nil)
	require.NoError(t, err)
	require.Len(t, changed, 8)
	rows := store.Rows{}
	for _, tr := range changed {
		rows[tr.Table] = append(rows[tr.Table], tr.Row)
	}
	require.InDelta(t, 0.75, rows["claims"][0]["confidence"], 0)
	b, err := json.Marshal(rows)
	require.NoError(t, err)
	decoded, err := store.DecodeRows(b)
	require.NoError(t, err)
	require.Equal(t, rows, decoded, "the JSON keeps every value and its type")

	dst := newFx(t)
	res, err := dst.r.Sync.Apply(dst.ctx, decoded, false)
	require.NoError(t, err)
	require.Equal(t, 8, res.Applied)
	again, err := dst.r.Sync.Changed(dst.ctx, time.UnixMilli(0), nil)
	require.NoError(t, err)
	require.Equal(t, changed, again)
}

func TestDecodeRowsNumbers(t *testing.T) {
	t.Parallel()
	rows, err := store.DecodeRows([]byte(`{"claims": [{"a": 7, "b": 0.25, "c": 1e2, "d": "x", "e": null}]}`))
	require.NoError(t, err)
	require.Equal(t, store.Row{"a": int64(7), "b": 0.25, "c": 100.0, "d": "x", "e": nil}, rows["claims"][0])
	_, err = store.DecodeRows([]byte(`{"claims": [{"a": 123456789012345678901234567890}]}`))
	require.ErrorContains(t, err, "not an integer")
	_, err = store.DecodeRows([]byte(`{"claims": [{"a": true}]}`))
	require.ErrorContains(t, err, "numbers, strings, or null")
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

	got, cursor, more, err := f.r.Sync.Pull(f.ctx, "dev-b", 0, 2, 10)
	require.NoError(t, err)
	require.True(t, more)
	require.Equal(t, []string{"P0", "P2"}, names(got))
	got, cursor, more, err = f.r.Sync.Pull(f.ctx, "dev-b", cursor, 2, 10)
	require.NoError(t, err)
	require.False(t, more)
	require.Equal(t, []string{"P4"}, names(got))
	got, _, more, err = f.r.Sync.Pull(f.ctx, "dev-b", cursor, 2, 10)
	require.NoError(t, err)
	require.False(t, more)
	require.Empty(t, got.Count())

	// Rewriting a row moves it to the end.
	_, err = f.r.Sync.Apply(f.ctx, store.Rows{"projects": {projectRow(rows[0]["id"].(string), "P0 again", 99, "dev-a")}}, true)
	require.NoError(t, err)
	got, _, _, err = f.r.Sync.Pull(f.ctx, "dev-b", cursor, 10, 10)
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

	changed, err := src.r.Sync.Changed(src.ctx, time.UnixMilli(0), nil)
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
	later, err := src.r.Sync.Changed(src.ctx, src.clk.Now().Add(time.Second), nil)
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
