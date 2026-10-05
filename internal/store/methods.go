package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/planner/civil"
)

// ReviewInput is a week's reflection to save; nil fields are unchanged.
type ReviewInput struct {
	WeekStart    string
	WentWell     *string
	WentBadly    *string
	Energy       *string
	Decisions    *string
	Improvements *string
	Checklist    *int
}

// ReviewRepo reads and writes weekly reviews.
type ReviewRepo interface {
	// Get returns the review of the week starting weekStart, a Monday, or
	// ErrNotFound when it has none.
	Get(ctx context.Context, weekStart string) (model.WeeklyReview, error)
	// Latest returns the most recent review of a week starting before
	// weekStart, or ErrNotFound.
	Latest(ctx context.Context, before string) (model.WeeklyReview, error)
	// Save creates or updates the week's review.
	Save(ctx context.Context, in ReviewInput) (model.WeeklyReview, error)
}

// EnergyRepo reads and writes energy check-ins.
type EnergyRepo interface {
	Log(ctx context.Context, at time.Time, level int) (model.EnergyLog, error)
	// List returns the check-ins from from up to to, oldest first.
	List(ctx context.Context, from, to time.Time) ([]model.EnergyLog, error)
	Delete(ctx context.Context, id string) error
}

type reviewRepo struct{ db *DB }

// NewReviewRepo returns the SQLite ReviewRepo.
func NewReviewRepo(db *DB) ReviewRepo { return reviewRepo{db} }

const reviewCols = `id, week_start, went_well, went_badly, energy, decisions, improvements, checklist, ` + envelopeCols

func scanReview(s scanner) (model.WeeklyReview, error) {
	var (
		r   model.WeeklyReview
		env envelopeScan
	)
	dest := append([]any{&r.ID, &r.WeekStart, &r.WentWell, &r.WentBadly, &r.Energy, &r.Decisions, &r.Improvements,
		&r.Checklist}, env.dest()...)
	if err := s.Scan(dest...); err != nil {
		return model.WeeklyReview{}, err
	}
	r.Envelope = env.envelope()
	return r, nil
}

// MaxReviewText is the longest answer of a review, in characters.
const MaxReviewText = 4000

func validWeek(weekStart string) error {
	d, err := civil.Parse(weekStart)
	if err != nil || d.Weekday() != time.Monday {
		return FailField(ErrInvalid, "week_start", "week_start must be a Monday like 2026-09-14")
	}
	return nil
}

func (r reviewRepo) Get(ctx context.Context, weekStart string) (model.WeeklyReview, error) {
	if err := validWeek(weekStart); err != nil {
		return model.WeeklyReview{}, err
	}
	return getReview(ctx, r.db.sql, weekStart)
}

func getReview(ctx context.Context, q Querier, weekStart string) (model.WeeklyReview, error) {
	const qGetReview = `SELECT ` + reviewCols + ` FROM weekly_reviews WHERE week_start = ? AND deleted_at IS NULL`
	rv, err := scanReview(q.QueryRowContext(ctx, qGetReview, weekStart))
	if err != nil {
		return model.WeeklyReview{}, fmt.Errorf("get review: %w", notFound(err, "review of the week", weekStart))
	}
	return rv, nil
}

func (r reviewRepo) Latest(ctx context.Context, before string) (model.WeeklyReview, error) {
	const qLatestReview = `SELECT ` + reviewCols + ` FROM weekly_reviews WHERE week_start < ? AND deleted_at IS NULL
		ORDER BY week_start DESC LIMIT 1`
	rv, err := scanReview(r.db.sql.QueryRowContext(ctx, qLatestReview, before))
	if err != nil {
		return model.WeeklyReview{}, fmt.Errorf("latest review: %w", notFound(err, "review before", before))
	}
	return rv, nil
}

func (r reviewRepo) Save(ctx context.Context, in ReviewInput) (model.WeeklyReview, error) {
	if err := validWeek(in.WeekStart); err != nil {
		return model.WeeklyReview{}, err
	}
	var rv model.WeeklyReview
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		rv, err = getReview(ctx, tx, in.WeekStart)
		fresh := isNotFound(err)
		if err != nil && !fresh {
			return err
		}
		if fresh {
			rv = model.WeeklyReview{ID: model.NewID(), WeekStart: in.WeekStart}
		}
		for _, f := range []struct {
			key      string
			to, from *string
		}{{"went_well", &rv.WentWell, in.WentWell}, {"went_badly", &rv.WentBadly, in.WentBadly},
			{"energy", &rv.Energy, in.Energy}, {"decisions", &rv.Decisions, in.Decisions},
			{"improvements", &rv.Improvements, in.Improvements}} {
			if f.from == nil {
				continue
			}
			v := strings.TrimSpace(*f.from)
			if len([]rune(v)) > MaxReviewText {
				return FailField(ErrInvalid, f.key, "%s is at most %d characters", f.key, MaxReviewText)
			}
			*f.to = v
		}
		if in.Checklist != nil {
			if *in.Checklist < 0 {
				return FailField(ErrInvalid, "checklist", "checklist must not be negative")
			}
			rv.Checklist = *in.Checklist
		}
		now := r.db.Now()
		if fresh {
			rv.Envelope = model.Envelope{CreatedAt: now, UpdatedAt: now, DeviceID: r.db.DeviceID(), Rev: 1}
			const qInsertReview = `INSERT INTO weekly_reviews (id, week_start, went_well, went_badly, energy, decisions,
				improvements, checklist, created_at, updated_at, device_id, rev) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`
			_, err := tx.ExecContext(ctx, qInsertReview, rv.ID, rv.WeekStart, rv.WentWell, rv.WentBadly, rv.Energy,
				rv.Decisions, rv.Improvements, rv.Checklist, now.UnixMilli(), now.UnixMilli(), rv.DeviceID)
			return classify(err)
		}
		rv.UpdatedAt, rv.DeviceID, rv.Rev = now, r.db.DeviceID(), rv.Rev+1
		const qUpdateReview = `UPDATE weekly_reviews SET went_well = ?, went_badly = ?, energy = ?, decisions = ?,
			improvements = ?, checklist = ?, updated_at = ?, device_id = ?, rev = ? WHERE id = ?`
		_, err = tx.ExecContext(ctx, qUpdateReview, rv.WentWell, rv.WentBadly, rv.Energy, rv.Decisions, rv.Improvements,
			rv.Checklist, now.UnixMilli(), rv.DeviceID, rv.Rev, rv.ID)
		return classify(err)
	})
	if err != nil {
		return model.WeeklyReview{}, fmt.Errorf("save review of %s: %w", in.WeekStart, err)
	}
	return rv, nil
}

type energyRepo struct{ db *DB }

// NewEnergyRepo returns the SQLite EnergyRepo.
func NewEnergyRepo(db *DB) EnergyRepo { return energyRepo{db} }

const energyCols = `id, at, level, ` + envelopeCols

func (r energyRepo) Log(ctx context.Context, at time.Time, level int) (model.EnergyLog, error) {
	if level < 1 || level > 5 {
		return model.EnergyLog{}, FailField(ErrInvalid, "level", "level must be 1 to 5")
	}
	now := r.db.Now()
	e := model.EnergyLog{ID: model.NewID(), At: at.Truncate(time.Millisecond), Level: level,
		Envelope: model.Envelope{CreatedAt: now, UpdatedAt: now, DeviceID: r.db.DeviceID(), Rev: 1}}
	const qInsertEnergy = `INSERT INTO energy_logs (id, at, level, created_at, updated_at, device_id, rev)
		VALUES (?, ?, ?, ?, ?, ?, 1)`
	if _, err := r.db.sql.ExecContext(ctx, qInsertEnergy, e.ID, e.At.UnixMilli(), e.Level, now.UnixMilli(),
		now.UnixMilli(), e.DeviceID); err != nil {
		return model.EnergyLog{}, fmt.Errorf("log energy: %w", classify(err))
	}
	return e, nil
}

func (r energyRepo) List(ctx context.Context, from, to time.Time) ([]model.EnergyLog, error) {
	const qEnergy = `SELECT ` + energyCols + ` FROM energy_logs WHERE deleted_at IS NULL AND at >= ? AND at < ?
		ORDER BY at, id`
	rows, err := r.db.sql.QueryContext(ctx, qEnergy, from.UnixMilli(), to.UnixMilli())
	if err != nil {
		return nil, fmt.Errorf("list energy: %w", err)
	}
	defer rows.Close()
	out := []model.EnergyLog{}
	for rows.Next() {
		var (
			e   model.EnergyLog
			at  int64
			env envelopeScan
		)
		if err := rows.Scan(append([]any{&e.ID, &at, &e.Level}, env.dest()...)...); err != nil {
			return nil, fmt.Errorf("list energy: %w", err)
		}
		e.At, e.Envelope = time.UnixMilli(at), env.envelope()
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r energyRepo) Delete(ctx context.Context, id string) error {
	return r.db.InTx(ctx, func(tx *sql.Tx) error {
		var n int
		const qHasEnergy = `SELECT count(*) FROM energy_logs WHERE id = ? AND deleted_at IS NULL`
		if err := tx.QueryRowContext(ctx, qHasEnergy, id).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return Fail(ErrNotFound, "no energy check-in %s", id)
		}
		return softDelete(ctx, tx, r.db, "energy_logs", id)
	})
}
