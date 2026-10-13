package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// LegacySyncSchema is the schema version assumed of a hub or a client that
// does not say its own: the last one before schema negotiation, whose
// gwend refuses rows of a table it does not know.
const LegacySyncSchema int64 = 9

// maxClockSkew is how far in the future a synced row's updated_at may be;
// a row dated later is skipped, since it would win every later write.
const maxClockSkew = time.Hour

// Sync modes (gwen-harness/harness.md section 5.5).
const (
	// modeLWW rows are replaced by a newer write: the higher
	// (updated_at, device_id) wins.
	modeLWW = "lww"
	// modeAppend rows are written once. Only a tombstone changes a stored
	// row, and only while that row is live.
	modeAppend = "append"
)

// notSensitive keeps sensitive memory on the device it was learned on.
const notSensitive = `sensitivity <> 'sensitive'`

// syncTable is one synced table.
type syncTable struct {
	name string
	// cols are the table's columns but the envelope, id first.
	cols []string
	// since is the schema version that started syncing the table: a hub or
	// client at an older version is never sent its rows.
	since int64
	mode  string
	// pushWhere, when set, is the SQL condition a row must meet to leave
	// the device; the others are never pushed or pulled.
	pushWhere string
}

// columns are every column the table syncs, the envelope last.
func (t syncTable) columns() []string { return append(append([]string{}, t.cols...), envelope...) }

// syncTables are the synced tables in dependency order: a row's parents are
// in an earlier table, or earlier in its own table by created_at. Their
// columns are the complete set of each (docs/03-data-model.md#sync-envelope).
var syncTables = []syncTable{
	{name: "projects", since: 1, mode: modeLWW, cols: []string{"id", "name", "color", "archived_at"}},
	{name: "goals", since: 1, mode: modeLWW, cols: []string{"id", "title", "kind", "unit", "target_quantity",
		"minutes_per_unit", "project_id", "start_day", "due_day", "status", "daily_minutes", "specific", "measurable",
		"assignable", "realistic"}},
	{name: "tasks", since: 1, mode: modeLWW, cols: []string{"id", "project_id", "title", "notes", "status", "priority",
		"due_day", "estimate_minutes", "done_at", "goal_id", "quantity", "quantity_done", "rrule", "template_id",
		"occurrence_day", "parent_id", "start_day", "start_minute", "stage", "effort", "delegated_to"}},
	{name: "commitments", since: 1, mode: modeLWW, cols: []string{"id", "title", "project_id", "rrule", "start_minute",
		"duration_minutes", "counts_toward_target", "active_from", "active_until"}},
	{name: "work_days", since: 1, mode: modeLWW, cols: []string{"id", "day", "tz", "clocked_in_at", "clocked_out_at",
		"target_seconds", "note"}},
	{name: "segments", since: 1, mode: modeLWW, cols: []string{"id", "work_day_id", "kind", "source", "project_id",
		"task_id", "started_at", "ended_at", "truncated"}},
	{name: "plan_items", since: 1, mode: modeLWW, cols: []string{"id", "day", "task_id", "planned_minutes", "start_at",
		"position", "status", "pinned", "rolled_from_id", "rollover_count"}},
	{name: "day_hours", since: 1, mode: modeLWW, cols: []string{"id", "day", "start_minute", "work_minutes"}},
	{name: "weekly_reviews", since: 1, mode: modeLWW, cols: []string{"id", "week_start", "went_well", "went_badly",
		"energy", "decisions", "improvements", "checklist"}},
	{name: "energy_logs", since: 1, mode: modeLWW, cols: []string{"id", "at", "level"}},

	{name: "chat_threads", since: memoryVersion, mode: modeAppend, cols: []string{"id", "title"}},
	{name: "chat_messages", since: memoryVersion, mode: modeAppend, cols: []string{"id", "thread_id", "role", "text",
		"at", "mood", "attitude", "actions", "memory"}},
	{name: "self_versions", since: memoryVersion, mode: modeAppend, cols: []string{"id", "attitude", "note", "since",
		"reason", "thread_id"}},
	{name: "entities", since: memoryVersion, mode: modeLWW, cols: []string{"id", "kind", "name", "aliases"}},
	{name: "episodes", since: memoryVersion, mode: modeAppend, pushWhere: notSensitive, cols: []string{"id", "kind",
		"channel", "speaker", "title", "body", "source", "at", "sensitivity", "content_hash"}},
	{name: "claims", since: memoryVersion, mode: modeLWW, pushWhere: notSensitive, cols: []string{"id", "key",
		"subject_id", "value", "text", "cardinality", "status", "trust", "sensitivity", "confidence", "valid_from",
		"valid_to", "occurred_at", "mentioned_at", "recorded_at", "superseded_at", "superseded_by", "suppressed",
		"source_kind", "source_id", "span", "source_device_id", "channel", "extractor", "content_hash"}},
	{name: "conflicts", since: memoryVersion, mode: modeLWW, pushWhere: notSensitive, cols: []string{"id", "claim_a",
		"claim_b", "reason", "sensitivity", "status", "resolution", "resolved_at"}},
	{name: "memory_links", since: memoryVersion, mode: modeLWW, cols: []string{"id", "from_kind", "from_id", "to_kind",
		"to_id", "relation"}},
}

// SyncTables are the names of the synced tables, in dependency order.
var SyncTables = func() []string {
	out := make([]string, len(syncTables))
	for i, t := range syncTables {
		out[i] = t.name
	}
	return out
}()

// SyncTablesBetween returns the names of the synced tables that a schema
// version after after, and up to through, started syncing, in dependency
// order: SyncTablesBetween(0, v) is everything a node at v can take.
func SyncTablesBetween(after, through int64) []string {
	var out []string
	for _, t := range syncTables {
		if t.since > after && t.since <= through {
			out = append(out, t.name)
		}
	}
	return out
}

// envelope columns end every synced table.
var envelope = []string{"created_at", "updated_at", "deleted_at", "device_id", "rev"}

// Row is one synced row, keyed by column name. Values are int64, float64,
// string, or nil.
type Row map[string]any

// Rows are synced rows by table.
type Rows map[string][]Row

// Count is the number of rows.
func (r Rows) Count() int {
	n := 0
	for _, rows := range r {
		n += len(rows)
	}
	return n
}

// DecodeRows reads Rows from JSON, keeping integers exact. A number with a
// fraction or an exponent is a float64.
func DecodeRows(b []byte) (Rows, error) {
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	var raw map[string][]map[string]any
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	out := Rows{}
	for table, rows := range raw {
		for _, r := range rows {
			row := Row{}
			for k, v := range r {
				switch v := v.(type) {
				case json.Number:
					if n, err := v.Int64(); err == nil {
						row[k] = n
						break
					}
					if !strings.ContainsAny(string(v), ".eE") {
						return nil, fmt.Errorf("%s.%s: %q is not an integer", table, k, v)
					}
					f, err := v.Float64()
					if err != nil {
						return nil, fmt.Errorf("%s.%s: %q is not a number", table, k, v)
					}
					row[k] = f
				case string, nil:
					row[k] = v
				default:
					return nil, fmt.Errorf("%s.%s: values must be numbers, strings, or null", table, k)
				}
			}
			out[table] = append(out[table], row)
		}
	}
	return out, nil
}

// Applied is what an apply wrote.
type Applied struct {
	Applied, Ignored int
	// IDs are the applied row ids by table.
	IDs map[string][]string
}

// SyncRepo reads and writes rows for the sync hub and its clients
// (docs/07-integrations.md#sync-hub).
type SyncRepo interface {
	// Changed returns the synced rows with updated_at at or after since, of
	// the named tables (nil is every table), in SyncTables order and by
	// created_at within a table. Rows a table keeps on the device, such as
	// sensitive claims, are left out.
	Changed(ctx context.Context, since time.Time, tables []string) ([]TableRow, error)
	// Apply writes rows in one transaction with deferred foreign keys: by
	// last-writer-wins on (updated_at, device_id), or, for an append-only
	// table, only when the row is new or tombstones a live one. Rows of a
	// table this database does not sync, rows dated more than an hour
	// ahead, and rows that violate a constraint are skipped and counted as
	// ignored. A row needs its id, created_at, updated_at, rev, and
	// device_id; other columns it lacks take their defaults, and columns the
	// table does not have are dropped. On the hub, every applied row
	// replaces its hub_changes entry.
	Apply(ctx context.Context, rows Rows, hub bool) (Applied, error)
	// Pull returns the rows whose hub_changes seq is after cursor and whose
	// last writer is not deviceID, in seq order, at most limit, with the new
	// cursor and whether more remain. Rows of tables that schema, the
	// client's schema version, does not sync are skipped, and the cursor
	// still moves past them.
	Pull(ctx context.Context, deviceID string, cursor int64, limit int, schema int64) (Rows, int64, bool, error)
	// Days are the days whose work day or segments, and whose plan, the
	// applied rows touched.
	Days(ctx context.Context, a Applied) (workDays, planDays []string, err error)
}

// TableRow is a row with its table.
type TableRow struct {
	Table string
	Row   Row
}

type syncRepo struct{ db *DB }

// NewSyncRepo returns the SQLite SyncRepo.
func NewSyncRepo(db *DB) SyncRepo { return syncRepo{db} }

// table returns a synced table that this database has.
func (r syncRepo) table(name string) (syncTable, bool) {
	for _, t := range syncTables {
		if t.name == name {
			return t, t.since <= r.db.SchemaVersion()
		}
	}
	return syncTable{}, false
}

// selectRows is the query for a table's synced rows matching where, which
// must not be empty.
func selectRows(t syncTable, where string) string {
	if t.pushWhere != "" {
		where = "(" + where + ") AND (" + t.pushWhere + ")"
	}
	// The table and column names come from syncTables, never from a request.
	return `SELECT ` + strings.Join(t.columns(), ", ") + ` FROM ` + t.name + ` WHERE ` + where
}

func scanRow(rows *sql.Rows, cols []string) (Row, error) {
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}
	r := Row{}
	for i, c := range cols {
		switch v := vals[i].(type) {
		case []byte:
			r[c] = string(v)
		default:
			r[c] = v
		}
	}
	return r, nil
}

func (r syncRepo) Changed(ctx context.Context, since time.Time, tables []string) ([]TableRow, error) {
	var out []TableRow
	for _, t := range syncTables {
		if _, ok := r.table(t.name); !ok || (tables != nil && !slices.Contains(tables, t.name)) {
			continue
		}
		cols := t.columns()
		q := selectRows(t, `updated_at >= ?`) + ` ORDER BY created_at, id`
		rows, err := r.db.sql.QueryContext(ctx, q, since.UnixMilli())
		if err != nil {
			return nil, fmt.Errorf("changed %s: %w", t.name, err)
		}
		for rows.Next() {
			row, err := scanRow(rows, cols)
			if err != nil {
				rows.Close()
				return nil, fmt.Errorf("changed %s: %w", t.name, err)
			}
			out = append(out, TableRow{Table: t.name, Row: row})
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// checkRow validates a pushed row's envelope: a string id and device_id, and
// integer created_at, updated_at, and rev. The table's constraints check the
// rest when the row is written.
func checkRow(table string, row Row) error {
	for _, c := range []string{"id", "device_id", "created_at", "updated_at", "rev"} {
		v, ok := row[c]
		if !ok {
			return fmt.Errorf("a %s row lacks %s", table, c)
		}
		switch c {
		case "id", "device_id":
			if _, ok := v.(string); !ok {
				return fmt.Errorf("a %s row's %s must be a string", table, c)
			}
		default:
			if _, ok := v.(int64); !ok {
				return fmt.Errorf("a %s row's %s must be an integer", table, c)
			}
		}
	}
	return nil
}

func (r syncRepo) Apply(ctx context.Context, rows Rows, hub bool) (Applied, error) {
	res := Applied{IDs: map[string][]string{}}
	for table, rs := range rows {
		if _, ok := r.table(table); !ok {
			// A newer device's table: its rows wait for this one to be updated.
			if len(rs) > 0 {
				slog.Warn("synced rows of an unknown table skipped", "table", table, "rows", len(rs))
				res.Ignored += len(rs)
			}
			continue
		}
		for _, row := range rs {
			if err := checkRow(table, row); err != nil {
				return Applied{}, FailField(ErrInvalid, "rows", "%v", err)
			}
		}
	}
	horizon := r.db.Now().Add(maxClockSkew).UnixMilli()
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `PRAGMA defer_foreign_keys = ON`); err != nil {
			return err
		}
		for _, t := range syncTables {
			if _, ok := r.table(t.name); !ok {
				continue
			}
			var future []any
			for _, row := range rows[t.name] {
				if row["updated_at"].(int64) > horizon {
					future = append(future, row["id"])
					continue
				}
				ok, err := applyRow(ctx, tx, t, row)
				if err != nil {
					return err
				}
				if !ok {
					res.Ignored++
					continue
				}
				res.Applied++
				id := row["id"].(string)
				res.IDs[t.name] = append(res.IDs[t.name], id)
				if hub {
					const qHubChange = `INSERT OR REPLACE INTO hub_changes (table_name, row_id, device_id) VALUES (?, ?, ?)`
					if _, err := tx.ExecContext(ctx, qHubChange, t.name, id, row["device_id"]); err != nil {
						return err
					}
				}
			}
			if len(future) > 0 {
				slog.Warn("synced rows dated in the future skipped", "table", t.name, "rows", len(future),
					"first_id", future[0])
				res.Ignored += len(future)
			}
		}
		return nil
	})
	if err != nil {
		return Applied{}, fmt.Errorf("apply synced rows: %w", err)
	}
	return res, nil
}

// applyRow writes one row if its table's mode lets it, inside a savepoint so
// that a constraint violation skips just this row. It reports whether the
// row was written.
func applyRow(ctx context.Context, tx *sql.Tx, t syncTable, row Row) (bool, error) {
	var cols, sets []string
	var args []any
	for _, c := range t.columns() {
		v, ok := row[c]
		if !ok {
			continue
		}
		cols = append(cols, c)
		args = append(args, v)
		if c != "id" {
			sets = append(sets, c+" = excluded."+c)
		}
	}
	where := `(excluded.updated_at, excluded.device_id) > (` + t.name + `.updated_at, ` + t.name + `.device_id)`
	if t.mode == modeAppend {
		where = `excluded.deleted_at IS NOT NULL AND ` + t.name + `.deleted_at IS NULL`
	}
	// The table and column names come from syncTables, never from the request.
	q := `INSERT INTO ` + t.name + ` (` + strings.Join(cols, ", ") + `) VALUES (` + placeholders(len(cols)) + `)
		ON CONFLICT (id) DO UPDATE SET ` + strings.Join(sets, ", ") + ` WHERE ` + where
	if _, err := tx.ExecContext(ctx, `SAVEPOINT sync_row`); err != nil {
		return false, err
	}
	res, err := tx.ExecContext(ctx, q, args...)
	if err != nil {
		if _, rbErr := tx.ExecContext(ctx, `ROLLBACK TO sync_row`); rbErr != nil {
			return false, rbErr
		}
		if _, rbErr := tx.ExecContext(ctx, `RELEASE sync_row`); rbErr != nil {
			return false, rbErr
		}
		if violatesConstraint(err) {
			slog.Warn("synced row skipped", "table", t.name, "id", row["id"], "err", err)
			return false, nil
		}
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `RELEASE sync_row`); err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// violatesConstraint reports whether err is a constraint a row broke: a
// check, NOT NULL, uniqueness, or, in a STRICT table, a value of the wrong
// type.
func violatesConstraint(err error) bool {
	var se *sqlite.Error
	return errors.As(err, &se) && se.Code()&0xff == sqlite3.SQLITE_CONSTRAINT
}

func (r syncRepo) Pull(ctx context.Context, deviceID string, cursor int64, limit int,
	schema int64) (Rows, int64, bool, error) {
	const qChanges = `SELECT seq, table_name, row_id FROM hub_changes WHERE seq > ? AND device_id <> ?
		ORDER BY seq LIMIT ?`
	rows, err := r.db.sql.QueryContext(ctx, qChanges, cursor, deviceID, limit+1)
	if err != nil {
		return nil, 0, false, fmt.Errorf("pull: %w", err)
	}
	type change struct {
		seq       int64
		table, id string
	}
	var changes []change
	for rows.Next() {
		var c change
		if err := rows.Scan(&c.seq, &c.table, &c.id); err != nil {
			rows.Close()
			return nil, 0, false, fmt.Errorf("pull: %w", err)
		}
		changes = append(changes, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, false, err
	}
	more := len(changes) > limit
	if more {
		changes = changes[:limit]
	}
	out := Rows{}
	for _, c := range changes {
		cursor = c.seq
		t, ok := r.table(c.table)
		if !ok || t.since > schema {
			continue
		}
		cols := t.columns()
		rs, err := r.db.sql.QueryContext(ctx, selectRows(t, `id = ?`), c.id)
		if err != nil {
			return nil, 0, false, fmt.Errorf("pull %s: %w", c.table, err)
		}
		for rs.Next() {
			row, err := scanRow(rs, cols)
			if err != nil {
				rs.Close()
				return nil, 0, false, err
			}
			out[c.table] = append(out[c.table], row)
		}
		err = rs.Err()
		rs.Close()
		if err != nil {
			return nil, 0, false, fmt.Errorf("pull %s: %w", c.table, err)
		}
	}
	return out, cursor, more, nil
}

// ParseCursor reads a pull cursor; "" is the start.
func ParseCursor(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0, FailField(ErrInvalid, "cursor", "cursor must be one a pull returned")
	}
	return n, nil
}

func (r syncRepo) Days(ctx context.Context, a Applied) (workDays, planDays []string, err error) {
	distinct := func(q string, ids []string) ([]string, error) {
		if len(ids) == 0 {
			return nil, nil
		}
		return queryIDs(ctx, r.db.sql, fmt.Sprintf(q, placeholders(len(ids))), anys(ids)...)
	}
	const qWorkDays = `SELECT DISTINCT day FROM work_days WHERE id IN (%s) ORDER BY day`
	const qSegmentDays = `SELECT DISTINCT d.day FROM segments s JOIN work_days d ON d.id = s.work_day_id
		WHERE s.id IN (%s) ORDER BY d.day`
	const qPlanDays = `SELECT DISTINCT day FROM plan_items WHERE id IN (%s) ORDER BY day`
	wd, err := distinct(qWorkDays, a.IDs["work_days"])
	if err != nil {
		return nil, nil, err
	}
	sd, err := distinct(qSegmentDays, a.IDs["segments"])
	if err != nil {
		return nil, nil, err
	}
	for _, d := range sd {
		if !slices.Contains(wd, d) {
			wd = append(wd, d)
		}
	}
	slices.Sort(wd)
	if planDays, err = distinct(qPlanDays, a.IDs["plan_items"]); err != nil {
		return nil, nil, err
	}
	const qHoursDays = `SELECT DISTINCT day FROM day_hours WHERE id IN (%s) ORDER BY day`
	hd, err := distinct(qHoursDays, a.IDs["day_hours"])
	for _, d := range hd {
		if !slices.Contains(planDays, d) {
			planDays = append(planDays, d)
		}
	}
	slices.Sort(planDays)
	return wd, planDays, err
}
