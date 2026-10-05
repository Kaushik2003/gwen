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
)

// SyncTables are the synced tables in dependency order: a row's parents are
// in an earlier table, or earlier in its own table by created_at.
var SyncTables = []string{"projects", "goals", "tasks", "commitments", "work_days", "segments", "plan_items",
	"day_hours", "weekly_reviews", "energy_logs"}

// syncColumns is the complete column set of each synced table
// (docs/03-data-model.md#sync-envelope).
var syncColumns = map[string][]string{
	"projects": {"id", "name", "color", "archived_at"},
	"goals": {"id", "title", "kind", "unit", "target_quantity", "minutes_per_unit", "project_id", "start_day",
		"due_day", "status", "daily_minutes", "specific", "measurable", "assignable", "realistic"},
	"tasks": {"id", "project_id", "title", "notes", "status", "priority", "due_day", "estimate_minutes", "done_at",
		"goal_id", "quantity", "quantity_done", "rrule", "template_id", "occurrence_day", "parent_id", "start_day",
		"start_minute", "stage", "effort", "delegated_to"},
	"commitments": {"id", "title", "project_id", "rrule", "start_minute", "duration_minutes", "counts_toward_target",
		"active_from", "active_until"},
	"work_days": {"id", "day", "tz", "clocked_in_at", "clocked_out_at", "target_seconds", "note"},
	"segments": {"id", "work_day_id", "kind", "source", "project_id", "task_id", "started_at", "ended_at",
		"truncated"},
	"plan_items": {"id", "day", "task_id", "planned_minutes", "start_at", "position", "status", "pinned",
		"rolled_from_id", "rollover_count"},
	"day_hours": {"id", "day", "start_minute", "work_minutes"},
	"weekly_reviews": {"id", "week_start", "went_well", "went_badly", "energy", "decisions", "improvements",
		"checklist"},
	"energy_logs": {"id", "at", "level"},
}

// envelope columns end every synced table.
var envelope = []string{"created_at", "updated_at", "deleted_at", "device_id", "rev"}

func columnsOf(table string) []string {
	return append(append([]string{}, syncColumns[table]...), envelope...)
}

// Row is one synced row, keyed by column name. Values are int64, string, or nil.
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

// DecodeRows reads Rows from JSON, keeping integers exact.
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
					n, err := v.Int64()
					if err != nil {
						return nil, fmt.Errorf("%s.%s: %q is not an integer", table, k, v)
					}
					row[k] = n
				case string, nil:
					row[k] = v
				default:
					return nil, fmt.Errorf("%s.%s: values must be integers, strings, or null", table, k)
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
	// Changed returns every synced row with updated_at at or after since, in
	// SyncTables order and by created_at within a table.
	Changed(ctx context.Context, since time.Time) ([]TableRow, error)
	// Apply writes rows by last-writer-wins on (updated_at, device_id) in one
	// transaction with deferred foreign keys. A row that violates a
	// constraint is skipped and counted as ignored. On the hub, every applied
	// row replaces its hub_changes entry.
	Apply(ctx context.Context, rows Rows, hub bool) (Applied, error)
	// Pull returns the rows whose hub_changes seq is after cursor and whose
	// last writer is not deviceID, in seq order, at most limit, with the new
	// cursor and whether more remain.
	Pull(ctx context.Context, deviceID string, cursor int64, limit int) (Rows, int64, bool, error)
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

func (r syncRepo) Changed(ctx context.Context, since time.Time) ([]TableRow, error) {
	var out []TableRow
	for _, table := range SyncTables {
		cols := columnsOf(table)
		q := `SELECT ` + strings.Join(cols, ", ") + ` FROM ` + table + ` WHERE updated_at >= ? ORDER BY created_at, id`
		rows, err := r.db.sql.QueryContext(ctx, q, since.UnixMilli())
		if err != nil {
			return nil, fmt.Errorf("changed %s: %w", table, err)
		}
		for rows.Next() {
			row, err := scanRow(rows, cols)
			if err != nil {
				rows.Close()
				return nil, fmt.Errorf("changed %s: %w", table, err)
			}
			out = append(out, TableRow{Table: table, Row: row})
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// checkRow validates a pushed row's table and columns: exactly the schema's
// set, with an id, an updated_at, and a device_id.
func checkRow(table string, row Row) error {
	cols, ok := syncColumns[table]
	if !ok {
		return fmt.Errorf("%q is not a synced table", table)
	}
	all := append(append([]string{}, cols...), envelope...)
	if len(row) != len(all) {
		return fmt.Errorf("a %s row needs exactly the columns %s", table, strings.Join(all, ", "))
	}
	for _, c := range all {
		if _, ok := row[c]; !ok {
			return fmt.Errorf("a %s row lacks %s", table, c)
		}
	}
	if _, ok := row["id"].(string); !ok {
		return fmt.Errorf("a %s row's id must be a string", table)
	}
	if _, ok := row["updated_at"].(int64); !ok {
		return fmt.Errorf("a %s row's updated_at must be an integer", table)
	}
	if _, ok := row["device_id"].(string); !ok {
		return fmt.Errorf("a %s row's device_id must be a string", table)
	}
	return nil
}

func (r syncRepo) Apply(ctx context.Context, rows Rows, hub bool) (Applied, error) {
	res := Applied{IDs: map[string][]string{}}
	for table, rs := range rows {
		for _, row := range rs {
			if err := checkRow(table, row); err != nil {
				return Applied{}, FailField(ErrInvalid, "rows", "%v", err)
			}
		}
	}
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `PRAGMA defer_foreign_keys = ON`); err != nil {
			return err
		}
		for _, table := range SyncTables {
			for _, row := range rows[table] {
				ok, err := applyRow(ctx, tx, table, row)
				if err != nil {
					return err
				}
				if !ok {
					res.Ignored++
					continue
				}
				res.Applied++
				id := row["id"].(string)
				res.IDs[table] = append(res.IDs[table], id)
				if hub {
					const qHubChange = `INSERT OR REPLACE INTO hub_changes (table_name, row_id, device_id) VALUES (?, ?, ?)`
					if _, err := tx.ExecContext(ctx, qHubChange, table, id, row["device_id"]); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return Applied{}, fmt.Errorf("apply synced rows: %w", err)
	}
	return res, nil
}

// applyRow writes one row if it wins last-writer-wins, inside a savepoint so
// that a constraint violation skips just this row. It reports whether the row
// was written.
func applyRow(ctx context.Context, tx *sql.Tx, table string, row Row) (bool, error) {
	cols := columnsOf(table)
	args := make([]any, len(cols))
	sets := make([]string, 0, len(cols)-1)
	for i, c := range cols {
		args[i] = row[c]
		if c != "id" {
			sets = append(sets, c+" = excluded."+c)
		}
	}
	// The table and column names come from syncColumns, never from the request.
	q := `INSERT INTO ` + table + ` (` + strings.Join(cols, ", ") + `) VALUES (` + placeholders(len(cols)) + `)
		ON CONFLICT (id) DO UPDATE SET ` + strings.Join(sets, ", ") + `
		WHERE (excluded.updated_at, excluded.device_id) > (` + table + `.updated_at, ` + table + `.device_id)`
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
		if c := classify(err); errors.Is(c, ErrConflict) || errors.Is(c, ErrInvalid) {
			slog.Warn("synced row skipped", "table", table, "id", row["id"], "err", err)
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

func (r syncRepo) Pull(ctx context.Context, deviceID string, cursor int64, limit int) (Rows, int64, bool, error) {
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
		cols, ok := syncColumns[c.table]
		if !ok {
			continue
		}
		all := append(append([]string{}, cols...), envelope...)
		q := `SELECT ` + strings.Join(all, ", ") + ` FROM ` + c.table + ` WHERE id = ?`
		rs, err := r.db.sql.QueryContext(ctx, q, c.id)
		if err != nil {
			return nil, 0, false, fmt.Errorf("pull %s: %w", c.table, err)
		}
		for rs.Next() {
			row, err := scanRow(rs, all)
			if err != nil {
				rs.Close()
				return nil, 0, false, err
			}
			out[c.table] = append(out[c.table], row)
		}
		rs.Close()
		cursor = c.seq
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
