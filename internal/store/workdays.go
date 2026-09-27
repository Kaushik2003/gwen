package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kzark/gwen/internal/model"
)

// WorkDayPatch changes a work day; nil fields are unchanged.
type WorkDayPatch struct {
	TargetSeconds *int
	Note          *string
	Rev           *int64
}

// WorkDayRepo reads and writes work days, including the engine's StartDay and
// CloseDay effects.
type WorkDayRepo interface {
	// Current returns the open work day, or nil.
	Current(ctx context.Context) (*model.WorkDay, error)
	Get(ctx context.Context, id string) (model.WorkDay, error)
	GetByDay(ctx context.Context, day string) (model.WorkDay, error)
	Update(ctx context.Context, day string, p WorkDayPatch) (model.WorkDay, error)
	// LatestClosedDay is the day of the latest closed work day, or "".
	LatestClosedDay(ctx context.Context) (string, error)
	// StartDay reopens the live work day for day if one exists, else inserts
	// one with the zone and target given.
	StartDay(ctx context.Context, tx *sql.Tx, day, tz string, targetSeconds int, at time.Time) (model.WorkDay, error)
	// CloseDay closes the open work day at at, which must have no open segment.
	CloseDay(ctx context.Context, tx *sql.Tx, at time.Time) (model.WorkDay, error)
}

type workDayRepo struct{ db *DB }

// NewWorkDayRepo returns the SQLite WorkDayRepo.
func NewWorkDayRepo(db *DB) WorkDayRepo { return workDayRepo{db} }

const workDayCols = `id, day, tz, clocked_in_at, clocked_out_at, target_seconds, note, ` + envelopeCols

func scanWorkDay(s scanner) (model.WorkDay, error) {
	var (
		w          model.WorkDay
		in, target int64
		out        sql.NullInt64
		env        envelopeScan
	)
	if err := s.Scan(append([]any{&w.ID, &w.Day, &w.TZ, &in, &out, &target, &w.Note}, env.dest()...)...); err != nil {
		return model.WorkDay{}, err
	}
	w.ClockedInAt, w.ClockedOutAt = timeOf(in), timePtr(out)
	w.Target = time.Duration(target) * time.Second
	w.Envelope = env.envelope()
	return w, nil
}

func (r workDayRepo) Current(ctx context.Context) (*model.WorkDay, error) {
	return currentWorkDay(ctx, r.db.sql)
}

func currentWorkDay(ctx context.Context, q Querier) (*model.WorkDay, error) {
	const qCurrentWorkDay = `SELECT ` + workDayCols + ` FROM work_days
		WHERE clocked_out_at IS NULL AND deleted_at IS NULL`
	w, err := scanWorkDay(q.QueryRowContext(ctx, qCurrentWorkDay))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("current work day: %w", err)
	}
	return &w, nil
}

func (r workDayRepo) Get(ctx context.Context, id string) (model.WorkDay, error) {
	const qGetWorkDay = `SELECT ` + workDayCols + ` FROM work_days WHERE id = ? AND deleted_at IS NULL`
	w, err := scanWorkDay(r.db.sql.QueryRowContext(ctx, qGetWorkDay, id))
	if err != nil {
		return model.WorkDay{}, fmt.Errorf("get work day: %w", notFound(err, "work day", id))
	}
	return w, nil
}

func (r workDayRepo) GetByDay(ctx context.Context, day string) (model.WorkDay, error) {
	return workDayByDay(ctx, r.db.sql, day)
}

func workDayByDay(ctx context.Context, q Querier, day string) (model.WorkDay, error) {
	if !validDay(day) {
		return model.WorkDay{}, FailField(ErrInvalid, "day", "day must be a date like 2026-09-15")
	}
	const qWorkDayByDay = `SELECT ` + workDayCols + ` FROM work_days WHERE day = ? AND deleted_at IS NULL`
	w, err := scanWorkDay(q.QueryRowContext(ctx, qWorkDayByDay, day))
	if err != nil {
		return model.WorkDay{}, fmt.Errorf("get work day: %w", notFound(err, "work day on", day))
	}
	return w, nil
}

func (r workDayRepo) Update(ctx context.Context, day string, p WorkDayPatch) (model.WorkDay, error) {
	var w model.WorkDay
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		if w, err = workDayByDay(ctx, tx, day); err != nil {
			return err
		}
		if err := checkRev(p.Rev, w.Rev); err != nil {
			return err
		}
		if p.TargetSeconds != nil {
			if *p.TargetSeconds < 0 {
				return FailField(ErrInvalid, "target_seconds", "target_seconds must not be negative")
			}
			w.Target = time.Duration(*p.TargetSeconds) * time.Second
		}
		if p.Note != nil {
			w.Note = *p.Note
		}
		return r.write(ctx, tx, &w)
	})
	if err != nil {
		return model.WorkDay{}, fmt.Errorf("update work day %s: %w", day, err)
	}
	return w, nil
}

// write stores every column of w with a new envelope write.
func (r workDayRepo) write(ctx context.Context, tx *sql.Tx, w *model.WorkDay) error {
	return writeWorkDay(ctx, tx, r.db, w)
}

func writeWorkDay(ctx context.Context, tx *sql.Tx, db *DB, w *model.WorkDay) error {
	now := db.Now()
	w.UpdatedAt, w.DeviceID, w.Rev = now, db.DeviceID(), w.Rev+1
	const qUpdateWorkDay = `UPDATE work_days SET clocked_in_at = ?, clocked_out_at = ?, target_seconds = ?,
		note = ?, updated_at = ?, device_id = ?, rev = ? WHERE id = ?`
	_, err := tx.ExecContext(ctx, qUpdateWorkDay, w.ClockedInAt.UnixMilli(), nullMillis(w.ClockedOutAt),
		int64(w.Target/time.Second), w.Note, now.UnixMilli(), w.DeviceID, w.Rev, w.ID)
	return classify(err)
}

func insertWorkDay(ctx context.Context, tx *sql.Tx, db *DB, w *model.WorkDay) error {
	now := db.Now()
	w.ID = model.NewID()
	w.Envelope = model.Envelope{CreatedAt: now, UpdatedAt: now, DeviceID: db.DeviceID(), Rev: 1}
	const qInsertWorkDay = `INSERT INTO work_days (id, day, tz, clocked_in_at, clocked_out_at, target_seconds, note,
		created_at, updated_at, device_id, rev) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`
	_, err := tx.ExecContext(ctx, qInsertWorkDay, w.ID, w.Day, w.TZ, w.ClockedInAt.UnixMilli(),
		nullMillis(w.ClockedOutAt), int64(w.Target/time.Second), w.Note, now.UnixMilli(), now.UnixMilli(), w.DeviceID)
	return classify(err)
}

func (r workDayRepo) LatestClosedDay(ctx context.Context) (string, error) {
	const qLatestClosedDay = `SELECT day FROM work_days WHERE clocked_out_at IS NOT NULL AND deleted_at IS NULL
		ORDER BY day DESC LIMIT 1`
	var day string
	err := r.db.sql.QueryRowContext(ctx, qLatestClosedDay).Scan(&day)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("latest closed day: %w", err)
	}
	return day, nil
}

func (r workDayRepo) StartDay(ctx context.Context, tx *sql.Tx, day, tz string, targetSeconds int, at time.Time) (model.WorkDay, error) {
	open, err := currentWorkDay(ctx, tx)
	if err != nil {
		return model.WorkDay{}, err
	}
	if open != nil {
		if open.Day == day {
			return *open, nil
		}
		return model.WorkDay{}, Fail(ErrConflict, "the work day %s is still open", open.Day)
	}
	w, err := workDayByDay(ctx, tx, day)
	switch {
	case err == nil:
		// Reopen; the time since the earlier clock-out is a gap.
		w.ClockedOutAt = nil
		if at.Before(w.ClockedInAt) {
			w.ClockedInAt = at
		}
		err = r.write(ctx, tx, &w)
	case errors.Is(err, ErrNotFound):
		w = model.WorkDay{Day: day, TZ: tz, ClockedInAt: at, Target: time.Duration(targetSeconds) * time.Second}
		err = insertWorkDay(ctx, tx, r.db, &w)
	}
	if err != nil {
		return model.WorkDay{}, fmt.Errorf("start day %s: %w", day, err)
	}
	return w, nil
}

func (r workDayRepo) CloseDay(ctx context.Context, tx *sql.Tx, at time.Time) (model.WorkDay, error) {
	w, err := currentWorkDay(ctx, tx)
	if err != nil {
		return model.WorkDay{}, err
	}
	if w == nil {
		return model.WorkDay{}, Fail(ErrConflict, "no work day is open")
	}
	open, err := currentSegment(ctx, tx)
	if err != nil {
		return model.WorkDay{}, err
	}
	if open != nil {
		return model.WorkDay{}, Fail(ErrConflict, "the work day still has an open segment")
	}
	// Keep every segment inside the day's bounds (invariant 5).
	end := at
	const qLatestEnd = `SELECT MAX(ended_at) FROM segments WHERE work_day_id = ? AND deleted_at IS NULL`
	var latest sql.NullInt64
	if err := tx.QueryRowContext(ctx, qLatestEnd, w.ID).Scan(&latest); err != nil {
		return model.WorkDay{}, fmt.Errorf("close day: %w", err)
	}
	if latest.Valid && timeOf(latest.Int64).After(end) {
		end = timeOf(latest.Int64)
	}
	if end.Before(w.ClockedInAt) {
		end = w.ClockedInAt
	}
	w.ClockedOutAt = &end
	if err := r.write(ctx, tx, w); err != nil {
		return model.WorkDay{}, fmt.Errorf("close day %s: %w", w.Day, err)
	}
	return *w, nil
}
