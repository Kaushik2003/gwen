package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kzark/gwen/internal/model"
)

// minSegment is the shortest live closed segment (invariant 7).
const minSegment = time.Second

// NewSegment is a closed segment added by hand. EndedAt is required.
type NewSegment struct {
	Day               string
	Kind              string
	ProjectID, TaskID *string
	StartedAt         time.Time
	EndedAt           *time.Time
}

// DayDefaults fill in a work day that a segment write creates.
type DayDefaults struct {
	TZ            string
	TargetSeconds int
}

// SegmentPatch changes a closed segment; unset fields are unchanged.
type SegmentPatch struct {
	Kind      *string
	ProjectID Nullable[string]
	TaskID    Nullable[string]
	StartedAt *time.Time
	EndedAt   *time.Time
	Rev       *int64
}

// SegmentRepo reads and writes segments, including the engine's OpenSegment
// and CloseSegment effects.
//
// Hand edits (Create, Update, Split, Delete) apply to closed segments only;
// the open segment belongs to the engine. They keep every invariant of
// docs/03-data-model.md#invariants, may not end in the future, and treat the
// open segment as running on indefinitely, so no edit can collide with where
// the engine will close it. The effects trust the engine for overlaps, so that
// a bad edit can never leave tracking unable to move on.
type SegmentRepo interface {
	// Current returns the open segment, or nil.
	Current(ctx context.Context) (*model.Segment, error)
	Get(ctx context.Context, id string) (model.Segment, error)
	// ListByWorkDay returns a work day's live segments by started_at.
	ListByWorkDay(ctx context.Context, workDayID string) ([]model.Segment, error)
	// LatestEnd is the latest ended_at among a work day's closed live segments.
	LatestEnd(ctx context.Context, workDayID string) (*time.Time, error)
	// LatestWork is the most recent live work segment, or nil.
	LatestWork(ctx context.Context) (*model.Segment, error)
	// Create adds a closed segment with source edit, creating a closed work
	// day with the defaults when the day has none.
	Create(ctx context.Context, s NewSegment, defaults DayDefaults) (model.Segment, error)
	Update(ctx context.Context, id string, p SegmentPatch) (model.Segment, error)
	// Split cuts a closed segment at at; the first half keeps the id and the
	// second half is a new segment with source edit.
	Split(ctx context.Context, id string, at time.Time) ([2]model.Segment, error)
	Delete(ctx context.Context, id string) error
	// OpenSegment inserts the open segment on the open work day.
	OpenSegment(ctx context.Context, tx *sql.Tx, kind, source string, project, task *string, at time.Time) (model.Segment, error)
	// CloseSegment ends the open segment at at, tombstoning it instead when it
	// would be shorter than 1000 ms.
	CloseSegment(ctx context.Context, tx *sql.Tx, at time.Time, truncated bool) (model.Segment, error)
}

type segmentRepo struct{ db *DB }

// NewSegmentRepo returns the SQLite SegmentRepo.
func NewSegmentRepo(db *DB) SegmentRepo { return segmentRepo{db} }

const segmentCols = `id, work_day_id, kind, source, project_id, task_id, started_at, ended_at, truncated, ` + envelopeCols

func scanSegment(s scanner) (model.Segment, error) {
	var (
		g             model.Segment
		project, task sql.NullString
		start         int64
		end           sql.NullInt64
		truncated     int
		env           envelopeScan
	)
	dest := append([]any{&g.ID, &g.WorkDayID, &g.Kind, &g.Source, &project, &task, &start, &end, &truncated},
		env.dest()...)
	if err := s.Scan(dest...); err != nil {
		return model.Segment{}, err
	}
	g.ProjectID, g.TaskID = stringPtr(project), stringPtr(task)
	g.StartedAt, g.EndedAt, g.Truncated = timeOf(start), timePtr(end), truncated == 1
	g.Envelope = env.envelope()
	return g, nil
}

func querySegments(ctx context.Context, q Querier, query string, args ...any) ([]model.Segment, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Segment{}
	for rows.Next() {
		g, err := scanSegment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r segmentRepo) Current(ctx context.Context) (*model.Segment, error) {
	return currentSegment(ctx, r.db.sql)
}

func currentSegment(ctx context.Context, q Querier) (*model.Segment, error) {
	const qCurrentSegment = `SELECT ` + segmentCols + ` FROM segments WHERE ended_at IS NULL AND deleted_at IS NULL`
	g, err := scanSegment(q.QueryRowContext(ctx, qCurrentSegment))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("current segment: %w", err)
	}
	return &g, nil
}

func (r segmentRepo) Get(ctx context.Context, id string) (model.Segment, error) {
	return getSegment(ctx, r.db.sql, id)
}

func getSegment(ctx context.Context, q Querier, id string) (model.Segment, error) {
	const qGetSegment = `SELECT ` + segmentCols + ` FROM segments WHERE id = ? AND deleted_at IS NULL`
	g, err := scanSegment(q.QueryRowContext(ctx, qGetSegment, id))
	if err != nil {
		return model.Segment{}, fmt.Errorf("get segment: %w", notFound(err, "segment", id))
	}
	return g, nil
}

func (r segmentRepo) ListByWorkDay(ctx context.Context, workDayID string) ([]model.Segment, error) {
	const qSegmentsOfDay = `SELECT ` + segmentCols + ` FROM segments
		WHERE work_day_id = ? AND deleted_at IS NULL ORDER BY started_at, id`
	out, err := querySegments(ctx, r.db.sql, qSegmentsOfDay, workDayID)
	if err != nil {
		return nil, fmt.Errorf("list segments: %w", err)
	}
	return out, nil
}

func (r segmentRepo) LatestEnd(ctx context.Context, workDayID string) (*time.Time, error) {
	const qLatestEnd = `SELECT MAX(ended_at) FROM segments WHERE work_day_id = ? AND deleted_at IS NULL`
	var latest sql.NullInt64
	if err := r.db.sql.QueryRowContext(ctx, qLatestEnd, workDayID).Scan(&latest); err != nil {
		return nil, fmt.Errorf("latest segment end: %w", err)
	}
	return timePtr(latest), nil
}

func (r segmentRepo) LatestWork(ctx context.Context) (*model.Segment, error) {
	const qLatestWork = `SELECT ` + segmentCols + ` FROM segments WHERE kind = 'work' AND deleted_at IS NULL
		ORDER BY started_at DESC, id DESC LIMIT 1`
	out, err := querySegments(ctx, r.db.sql, qLatestWork)
	if err != nil {
		return nil, fmt.Errorf("latest work segment: %w", err)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return &out[0], nil
}

func (r segmentRepo) Create(ctx context.Context, ns NewSegment, defaults DayDefaults) (model.Segment, error) {
	if !validDay(ns.Day) {
		return model.Segment{}, FailField(ErrInvalid, "day", "day must be a date like 2026-09-15")
	}
	if ns.EndedAt == nil {
		return model.Segment{}, FailField(ErrInvalid, "ended_at", "ended_at is required")
	}
	if model.IsBreakKind(ns.Kind) && (ns.ProjectID != nil || ns.TaskID != nil) {
		field := "project_id"
		if ns.ProjectID == nil {
			field = "task_id"
		}
		return model.Segment{}, FailField(ErrInvalid, field, "a break has no project or task")
	}
	g := model.Segment{
		ID: model.NewID(), Kind: ns.Kind, Source: model.SourceEdit, ProjectID: ns.ProjectID, TaskID: ns.TaskID,
		StartedAt: ns.StartedAt.Truncate(time.Millisecond), EndedAt: truncPtr(ns.EndedAt),
	}
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		if err := r.validate(ctx, tx, &g); err != nil {
			return err
		}
		w, err := workDayByDay(ctx, tx, ns.Day)
		switch {
		case errors.Is(err, ErrNotFound):
			w = model.WorkDay{Day: ns.Day, TZ: defaults.TZ, ClockedInAt: g.StartedAt, ClockedOutAt: g.EndedAt,
				Target: time.Duration(defaults.TargetSeconds) * time.Second}
			if err := insertWorkDay(ctx, tx, r.db, &w); err != nil {
				return err
			}
		case err != nil:
			return err
		}
		g.WorkDayID = w.ID
		if err := r.checkOverlap(ctx, tx, g); err != nil {
			return err
		}
		if err := r.widen(ctx, tx, w, g); err != nil {
			return err
		}
		return r.insert(ctx, tx, &g)
	})
	if err != nil {
		return model.Segment{}, fmt.Errorf("create segment: %w", err)
	}
	return g, nil
}

func (r segmentRepo) Update(ctx context.Context, id string, p SegmentPatch) (model.Segment, error) {
	var g model.Segment
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		if g, err = r.closed(ctx, tx, id); err != nil {
			return err
		}
		if err := checkRev(p.Rev, g.Rev); err != nil {
			return err
		}
		if p.Kind != nil {
			g.Kind = *p.Kind
		}
		if p.ProjectID.Set {
			g.ProjectID = p.ProjectID.Value
		}
		if p.TaskID.Set {
			g.TaskID = p.TaskID.Value
		}
		if model.IsBreakKind(g.Kind) {
			g.ProjectID, g.TaskID = nil, nil
		}
		if p.StartedAt != nil {
			g.StartedAt = p.StartedAt.Truncate(time.Millisecond)
		}
		if p.EndedAt != nil {
			g.EndedAt = truncPtr(p.EndedAt)
		}
		if err := r.validate(ctx, tx, &g); err != nil {
			return err
		}
		if err := r.checkOverlap(ctx, tx, g); err != nil {
			return err
		}
		w, err := getWorkDayTx(ctx, tx, g.WorkDayID)
		if err != nil {
			return err
		}
		if err := r.widen(ctx, tx, w, g); err != nil {
			return err
		}
		return r.write(ctx, tx, &g)
	})
	if err != nil {
		return model.Segment{}, fmt.Errorf("update segment %s: %w", id, err)
	}
	return g, nil
}

func (r segmentRepo) Split(ctx context.Context, id string, at time.Time) ([2]model.Segment, error) {
	var halves [2]model.Segment
	at = at.Truncate(time.Millisecond)
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		first, err := r.closed(ctx, tx, id)
		if err != nil {
			return err
		}
		if at.Sub(first.StartedAt) < minSegment || first.EndedAt.Sub(at) < minSegment {
			return FailField(ErrInvalid, "at", "the split must leave both halves at least 1 s long")
		}
		second := first
		second.ID, second.Source, second.StartedAt, second.Truncated = model.NewID(), model.SourceEdit, at, false
		first.EndedAt = &at
		if err := r.write(ctx, tx, &first); err != nil {
			return err
		}
		if err := r.insert(ctx, tx, &second); err != nil {
			return err
		}
		halves = [2]model.Segment{first, second}
		return nil
	})
	if err != nil {
		return halves, fmt.Errorf("split segment %s: %w", id, err)
	}
	return halves, nil
}

func (r segmentRepo) Delete(ctx context.Context, id string) error {
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := r.closed(ctx, tx, id); err != nil {
			return err
		}
		now := r.db.Now().UnixMilli()
		const qDeleteSegment = `UPDATE segments SET deleted_at = ?, updated_at = ?, device_id = ?, rev = rev + 1
			WHERE id = ?`
		_, err := tx.ExecContext(ctx, qDeleteSegment, now, now, r.db.DeviceID(), id)
		return err
	})
	if err != nil {
		return fmt.Errorf("delete segment %s: %w", id, err)
	}
	return nil
}

func (r segmentRepo) OpenSegment(ctx context.Context, tx *sql.Tx, kind, source string, project, task *string, at time.Time) (model.Segment, error) {
	w, err := currentWorkDay(ctx, tx)
	if err != nil {
		return model.Segment{}, err
	}
	if w == nil {
		return model.Segment{}, Fail(ErrConflict, "no work day is open")
	}
	g := model.Segment{ID: model.NewID(), WorkDayID: w.ID, Kind: kind, Source: source, ProjectID: project,
		TaskID: task, StartedAt: at.Truncate(time.Millisecond)}
	if at.Before(w.ClockedInAt) {
		w.ClockedInAt = g.StartedAt
		if err := writeWorkDay(ctx, tx, r.db, w); err != nil {
			return model.Segment{}, err
		}
	}
	if err := r.insert(ctx, tx, &g); err != nil {
		return model.Segment{}, fmt.Errorf("open segment: %w", err)
	}
	return g, nil
}

func (r segmentRepo) CloseSegment(ctx context.Context, tx *sql.Tx, at time.Time, truncated bool) (model.Segment, error) {
	g, err := currentSegment(ctx, tx)
	if err != nil {
		return model.Segment{}, err
	}
	if g == nil {
		return model.Segment{}, Fail(ErrConflict, "no segment is open")
	}
	end := at.Truncate(time.Millisecond)
	if end.Before(g.StartedAt) {
		end = g.StartedAt
	}
	g.EndedAt, g.Truncated = &end, truncated
	if end.Sub(g.StartedAt) < minSegment {
		now := r.db.Now()
		g.DeletedAt = &now // too short to keep (invariant 7)
	}
	if err := r.write(ctx, tx, g); err != nil {
		return model.Segment{}, fmt.Errorf("close segment %s: %w", g.ID, err)
	}
	return *g, nil
}

// closed loads a live segment for a hand edit, which the open segment refuses.
func (r segmentRepo) closed(ctx context.Context, tx *sql.Tx, id string) (model.Segment, error) {
	g, err := getSegment(ctx, tx, id)
	if err != nil {
		return model.Segment{}, err
	}
	if g.EndedAt == nil {
		return model.Segment{}, Fail(ErrConflict, "the open segment belongs to the tracker")
	}
	return g, nil
}

// validate checks a hand-edited segment's kind, times, and attribution. A task
// without a project takes the task's project, as the tracking commands do.
func (r segmentRepo) validate(ctx context.Context, q Querier, g *model.Segment) error {
	if !model.ValidKind(g.Kind) {
		return FailField(ErrInvalid, "kind", "kind must be work, break_auto, or break_manual")
	}
	end := *g.EndedAt
	switch {
	case !end.After(g.StartedAt):
		return FailField(ErrInvalid, "ended_at", "a segment must end after it starts")
	case end.Sub(g.StartedAt) < minSegment:
		return FailField(ErrInvalid, "ended_at", "a segment must be at least 1 s long")
	case end.After(r.db.Now()):
		return FailField(ErrInvalid, "ended_at", "a segment cannot end in the future")
	}
	if g.TaskID != nil {
		t, err := getTask(ctx, q, *g.TaskID)
		if err != nil {
			return FailField(ErrNotFound, "task_id", "no task %s", *g.TaskID)
		}
		switch {
		case g.ProjectID == nil:
			g.ProjectID = t.ProjectID
		case t.ProjectID != nil && *t.ProjectID != *g.ProjectID:
			return FailField(ErrConflict, "task_id", "the task belongs to another project")
		}
	}
	if g.ProjectID != nil {
		return projectExists(ctx, q, *g.ProjectID, "project_id")
	}
	return nil
}

// checkOverlap refuses a segment overlapping another live segment of its work
// day; touching is fine, and the open segment runs on indefinitely.
func (r segmentRepo) checkOverlap(ctx context.Context, q Querier, g model.Segment) error {
	const qOverlap = `SELECT count(*) FROM segments WHERE work_day_id = ? AND deleted_at IS NULL AND id <> ?
		AND started_at < ? AND (ended_at IS NULL OR ended_at > ?)`
	var n int
	err := q.QueryRowContext(ctx, qOverlap, g.WorkDayID, g.ID, g.EndedAt.UnixMilli(), g.StartedAt.UnixMilli()).Scan(&n)
	if err != nil {
		return err
	}
	if n > 0 {
		return FailField(ErrConflict, "started_at", "segments may not overlap")
	}
	return nil
}

// widen grows a work day's bounds to contain g (invariant 5).
func (r segmentRepo) widen(ctx context.Context, tx *sql.Tx, w model.WorkDay, g model.Segment) error {
	changed := false
	if g.StartedAt.Before(w.ClockedInAt) {
		w.ClockedInAt, changed = g.StartedAt, true
	}
	if w.ClockedOutAt != nil && g.EndedAt.After(*w.ClockedOutAt) {
		w.ClockedOutAt, changed = g.EndedAt, true
	}
	if !changed {
		return nil
	}
	return writeWorkDay(ctx, tx, r.db, &w)
}

func (r segmentRepo) insert(ctx context.Context, tx *sql.Tx, g *model.Segment) error {
	now := r.db.Now()
	g.Envelope = model.Envelope{CreatedAt: now, UpdatedAt: now, DeviceID: r.db.DeviceID(), Rev: 1}
	const qInsertSegment = `INSERT INTO segments (id, work_day_id, kind, source, project_id, task_id, started_at,
		ended_at, truncated, created_at, updated_at, device_id, rev) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`
	_, err := tx.ExecContext(ctx, qInsertSegment, g.ID, g.WorkDayID, g.Kind, g.Source, nullString(g.ProjectID),
		nullString(g.TaskID), g.StartedAt.UnixMilli(), nullMillis(g.EndedAt), boolInt(g.Truncated),
		now.UnixMilli(), now.UnixMilli(), g.DeviceID)
	return classify(err)
}

// write stores every column of g, including deleted_at, with a new envelope write.
func (r segmentRepo) write(ctx context.Context, tx *sql.Tx, g *model.Segment) error {
	now := r.db.Now()
	g.UpdatedAt, g.DeviceID, g.Rev = now, r.db.DeviceID(), g.Rev+1
	const qUpdateSegment = `UPDATE segments SET kind = ?, source = ?, project_id = ?, task_id = ?, started_at = ?,
		ended_at = ?, truncated = ?, deleted_at = ?, updated_at = ?, device_id = ?, rev = ? WHERE id = ?`
	_, err := tx.ExecContext(ctx, qUpdateSegment, g.Kind, g.Source, nullString(g.ProjectID), nullString(g.TaskID),
		g.StartedAt.UnixMilli(), nullMillis(g.EndedAt), boolInt(g.Truncated), nullMillis(g.DeletedAt),
		now.UnixMilli(), g.DeviceID, g.Rev, g.ID)
	return classify(err)
}

func getWorkDayTx(ctx context.Context, q Querier, id string) (model.WorkDay, error) {
	const qGetWorkDayTx = `SELECT ` + workDayCols + ` FROM work_days WHERE id = ? AND deleted_at IS NULL`
	w, err := scanWorkDay(q.QueryRowContext(ctx, qGetWorkDayTx, id))
	if err != nil {
		return model.WorkDay{}, notFound(err, "work day", id)
	}
	return w, nil
}

func truncPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := t.Truncate(time.Millisecond)
	return &v
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
