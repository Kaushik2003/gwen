package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/planner"
)

// NewCommitment is a commitment to create. A nil CountsTowardTarget takes true.
type NewCommitment struct {
	Title              string
	ProjectID          *string
	RRule              string
	StartMinute        *int
	DurationMinutes    int
	CountsTowardTarget *bool
	ActiveFrom         string
	ActiveUntil        *string
}

// CommitmentPatch changes a commitment; unset fields are unchanged.
type CommitmentPatch struct {
	Title              *string
	ProjectID          Nullable[string]
	RRule              *string
	StartMinute        Nullable[int]
	DurationMinutes    *int
	CountsTowardTarget *bool
	ActiveFrom         *string
	ActiveUntil        Nullable[string]
	Rev                *int64
}

// CommitmentRepo reads and writes commitments.
type CommitmentRepo interface {
	// List returns live commitments, fixed ones by start_minute before
	// floating ones, then by title.
	List(ctx context.Context) ([]model.Commitment, error)
	Get(ctx context.Context, id string) (model.Commitment, error)
	Create(ctx context.Context, c NewCommitment) (model.Commitment, error)
	Update(ctx context.Context, id string, p CommitmentPatch) (model.Commitment, error)
	Delete(ctx context.Context, id string) error
}

type commitmentRepo struct{ db *DB }

// NewCommitmentRepo returns the SQLite CommitmentRepo.
func NewCommitmentRepo(db *DB) CommitmentRepo { return commitmentRepo{db} }

const commitmentCols = `id, title, project_id, rrule, start_minute, duration_minutes, counts_toward_target,
	active_from, active_until, ` + envelopeCols

func scanCommitment(s scanner) (model.Commitment, error) {
	var (
		c              model.Commitment
		project, until sql.NullString
		start          sql.NullInt64
		duration       int64
		counts         int
		env            envelopeScan
	)
	dest := append([]any{&c.ID, &c.Title, &project, &c.RRule, &start, &duration, &counts, &c.ActiveFrom, &until},
		env.dest()...)
	if err := s.Scan(dest...); err != nil {
		return model.Commitment{}, err
	}
	c.ProjectID, c.ActiveUntil, c.StartMinute = stringPtr(project), stringPtr(until), intPtr(start)
	c.Duration, c.CountsTowardTarget = time.Duration(duration)*time.Minute, counts == 1
	c.Envelope = env.envelope()
	return c, nil
}

func listCommitments(ctx context.Context, q Querier) ([]model.Commitment, error) {
	const qListCommitments = `SELECT ` + commitmentCols + ` FROM commitments WHERE deleted_at IS NULL
		ORDER BY start_minute IS NULL, start_minute, title COLLATE NOCASE, id`
	rows, err := q.QueryContext(ctx, qListCommitments)
	if err != nil {
		return nil, fmt.Errorf("list commitments: %w", err)
	}
	defer rows.Close()
	out := []model.Commitment{}
	for rows.Next() {
		c, err := scanCommitment(rows)
		if err != nil {
			return nil, fmt.Errorf("list commitments: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r commitmentRepo) List(ctx context.Context) ([]model.Commitment, error) {
	return listCommitments(ctx, r.db.sql)
}

func (r commitmentRepo) Get(ctx context.Context, id string) (model.Commitment, error) {
	return getCommitment(ctx, r.db.sql, id)
}

func getCommitment(ctx context.Context, q Querier, id string) (model.Commitment, error) {
	const qGetCommitment = `SELECT ` + commitmentCols + ` FROM commitments WHERE id = ? AND deleted_at IS NULL`
	c, err := scanCommitment(q.QueryRowContext(ctx, qGetCommitment, id))
	if err != nil {
		return model.Commitment{}, fmt.Errorf("get commitment: %w", notFound(err, "commitment", id))
	}
	return c, nil
}

func (r commitmentRepo) Create(ctx context.Context, nc NewCommitment) (model.Commitment, error) {
	now := r.db.Now()
	c := model.Commitment{
		ID: model.NewID(), Title: nc.Title, ProjectID: nc.ProjectID, RRule: nc.RRule, StartMinute: nc.StartMinute,
		Duration: time.Duration(nc.DurationMinutes) * time.Minute, CountsTowardTarget: true,
		ActiveFrom: nc.ActiveFrom, ActiveUntil: nc.ActiveUntil,
		Envelope: model.Envelope{CreatedAt: now, UpdatedAt: now, DeviceID: r.db.DeviceID(), Rev: 1},
	}
	if nc.CountsTowardTarget != nil {
		c.CountsTowardTarget = *nc.CountsTowardTarget
	}
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		if err := validCommitment(ctx, tx, &c); err != nil {
			return err
		}
		const qInsertCommitment = `INSERT INTO commitments (id, title, project_id, rrule, start_minute,
			duration_minutes, counts_toward_target, active_from, active_until, created_at, updated_at, device_id, rev)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`
		_, err := tx.ExecContext(ctx, qInsertCommitment, c.ID, c.Title, nullString(c.ProjectID), c.RRule,
			nullInt(c.StartMinute), int64(c.Duration/time.Minute), boolInt(c.CountsTowardTarget), c.ActiveFrom,
			nullString(c.ActiveUntil), now.UnixMilli(), now.UnixMilli(), c.DeviceID)
		return classify(err)
	})
	if err != nil {
		return model.Commitment{}, fmt.Errorf("create commitment: %w", err)
	}
	return c, nil
}

// validCommitment checks and normalizes a commitment's fields before a write.
func validCommitment(ctx context.Context, q Querier, c *model.Commitment) error {
	c.Title = strings.TrimSpace(c.Title)
	if n := len([]rune(c.Title)); n < 1 || n > 200 {
		return FailField(ErrInvalid, "title", "a commitment title must be 1 to 200 characters")
	}
	if _, err := planner.ParseRule(c.RRule); err != nil {
		return FailField(ErrInvalid, "rrule", "%v", err)
	}
	if c.StartMinute != nil && (*c.StartMinute < 0 || *c.StartMinute > 1439) {
		return FailField(ErrInvalid, "start_minute", "start_minute must be 0 to 1439")
	}
	if c.Duration < time.Minute || c.Duration > 24*time.Hour {
		return FailField(ErrInvalid, "duration_minutes", "duration_minutes must be 1 to 1440")
	}
	if !validDay(c.ActiveFrom) {
		return FailField(ErrInvalid, "active_from", "active_from must be a date like 2026-09-15")
	}
	if c.ActiveUntil != nil {
		if !validDay(*c.ActiveUntil) {
			return FailField(ErrInvalid, "active_until", "active_until must be a date like 2026-09-15")
		}
		if *c.ActiveUntil < c.ActiveFrom {
			return FailField(ErrInvalid, "active_until", "active_until must not be before active_from")
		}
	}
	if c.ProjectID != nil {
		return projectExists(ctx, q, *c.ProjectID, "project_id")
	}
	return nil
}

func (r commitmentRepo) Update(ctx context.Context, id string, p CommitmentPatch) (model.Commitment, error) {
	var c model.Commitment
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		if c, err = getCommitment(ctx, tx, id); err != nil {
			return err
		}
		if err := checkRev(p.Rev, c.Rev); err != nil {
			return err
		}
		if p.Title != nil {
			c.Title = *p.Title
		}
		if p.ProjectID.Set {
			c.ProjectID = p.ProjectID.Value
		}
		if p.RRule != nil {
			c.RRule = *p.RRule
		}
		if p.StartMinute.Set {
			c.StartMinute = p.StartMinute.Value
		}
		if p.DurationMinutes != nil {
			c.Duration = time.Duration(*p.DurationMinutes) * time.Minute
		}
		if p.CountsTowardTarget != nil {
			c.CountsTowardTarget = *p.CountsTowardTarget
		}
		if p.ActiveFrom != nil {
			c.ActiveFrom = *p.ActiveFrom
		}
		if p.ActiveUntil.Set {
			c.ActiveUntil = p.ActiveUntil.Value
		}
		if err := validCommitment(ctx, tx, &c); err != nil {
			return err
		}
		now := r.db.Now()
		c.UpdatedAt, c.DeviceID, c.Rev = now, r.db.DeviceID(), c.Rev+1
		const qUpdateCommitment = `UPDATE commitments SET title = ?, project_id = ?, rrule = ?, start_minute = ?,
			duration_minutes = ?, counts_toward_target = ?, active_from = ?, active_until = ?, updated_at = ?,
			device_id = ?, rev = ? WHERE id = ?`
		_, err = tx.ExecContext(ctx, qUpdateCommitment, c.Title, nullString(c.ProjectID), c.RRule,
			nullInt(c.StartMinute), int64(c.Duration/time.Minute), boolInt(c.CountsTowardTarget), c.ActiveFrom,
			nullString(c.ActiveUntil), now.UnixMilli(), c.DeviceID, c.Rev, c.ID)
		return classify(err)
	})
	if err != nil {
		return model.Commitment{}, fmt.Errorf("update commitment %s: %w", id, err)
	}
	return c, nil
}

func (r commitmentRepo) Delete(ctx context.Context, id string) error {
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := getCommitment(ctx, tx, id); err != nil {
			return err
		}
		return softDelete(ctx, tx, r.db, "commitments", id)
	})
	if err != nil {
		return fmt.Errorf("delete commitment %s: %w", id, err)
	}
	return nil
}
