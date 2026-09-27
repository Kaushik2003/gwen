package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/model"
)

// Nullable is a patch value for a nullable column: unset leaves the column
// alone, and a set nil Value clears it.
type Nullable[T any] struct {
	Set   bool
	Value *T
}

// TaskFilter selects tasks. Zero values are the defaults: any project, open
// tasks, no due bound.
type TaskFilter struct {
	ProjectID string
	Status    string // "open", "done", or "all"
	DueBefore string // exclusive YYYY-MM-DD
}

// NewTask is a task to create. Nil Notes and Priority take "" and 2.
type NewTask struct {
	ProjectID       *string
	Title           string
	Notes           *string
	Priority        *int
	DueDay          *string
	EstimateMinutes *int
}

// TaskPatch changes a task; unset fields are unchanged.
type TaskPatch struct {
	ProjectID       Nullable[string]
	Title           *string
	Notes           *string
	Priority        *int
	DueDay          Nullable[string]
	EstimateMinutes Nullable[int]
	Rev             *int64
}

// TaskRepo reads and writes tasks.
type TaskRepo interface {
	// List returns live tasks, ordered by due_day with nulls last, then
	// priority descending, then created_at.
	List(ctx context.Context, f TaskFilter) ([]model.Task, error)
	Get(ctx context.Context, id string) (model.Task, error)
	Create(ctx context.Context, t NewTask) (model.Task, error)
	Update(ctx context.Context, id string, p TaskPatch) (model.Task, error)
	// Complete and Reopen are idempotent: a task already in the target status
	// is returned unchanged.
	Complete(ctx context.Context, id string) (model.Task, error)
	Reopen(ctx context.Context, id string) (model.Task, error)
	Delete(ctx context.Context, id string) error
	// Tracked returns all-time work on each task, the open segment counting up
	// to now. Tasks without work are absent.
	Tracked(ctx context.Context, ids []string, now time.Time) (map[string]time.Duration, error)
}

type taskRepo struct{ db *DB }

// NewTaskRepo returns the SQLite TaskRepo.
func NewTaskRepo(db *DB) TaskRepo { return taskRepo{db} }

const taskCols = `id, project_id, title, notes, status, priority, due_day, estimate_minutes, done_at, ` + envelopeCols

func scanTask(s scanner) (model.Task, error) {
	var (
		t                model.Task
		project, due     sql.NullString
		estimate, doneAt sql.NullInt64
		env              envelopeScan
	)
	dest := append([]any{&t.ID, &project, &t.Title, &t.Notes, &t.Status, &t.Priority, &due, &estimate, &doneAt},
		env.dest()...)
	if err := s.Scan(dest...); err != nil {
		return model.Task{}, err
	}
	t.ProjectID, t.DueDay, t.DoneAt = stringPtr(project), stringPtr(due), timePtr(doneAt)
	if estimate.Valid {
		d := time.Duration(estimate.Int64) * time.Minute
		t.Estimate = &d
	}
	t.Envelope = env.envelope()
	return t, nil
}

func (r taskRepo) List(ctx context.Context, f TaskFilter) ([]model.Task, error) {
	q := `SELECT ` + taskCols + ` FROM tasks WHERE deleted_at IS NULL`
	var args []any
	switch f.Status {
	case "", model.TaskOpen:
		q += ` AND status = 'open'`
	case model.TaskDone:
		q += ` AND status = 'done'`
	case "all":
	default:
		return nil, FailField(ErrInvalid, "status", "status must be open, done, or all")
	}
	if f.ProjectID != "" {
		q += ` AND project_id = ?`
		args = append(args, f.ProjectID)
	}
	if f.DueBefore != "" {
		if !validDay(f.DueBefore) {
			return nil, FailField(ErrInvalid, "due_before", "due_before must be a date like 2026-09-15")
		}
		q += ` AND due_day < ?`
		args = append(args, f.DueBefore)
	}
	const qTaskOrder = ` ORDER BY due_day IS NULL, due_day, priority DESC, created_at, id`
	rows, err := r.db.sql.QueryContext(ctx, q+qTaskOrder, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	out := []model.Task{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("list tasks: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r taskRepo) Get(ctx context.Context, id string) (model.Task, error) {
	return getTask(ctx, r.db.sql, id)
}

func getTask(ctx context.Context, q Querier, id string) (model.Task, error) {
	const qGetTask = `SELECT ` + taskCols + ` FROM tasks WHERE id = ? AND deleted_at IS NULL`
	t, err := scanTask(q.QueryRowContext(ctx, qGetTask, id))
	if err != nil {
		return model.Task{}, fmt.Errorf("get task: %w", notFound(err, "task", id))
	}
	return t, nil
}

func (r taskRepo) Create(ctx context.Context, nt NewTask) (model.Task, error) {
	now := r.db.Now()
	t := model.Task{
		ID:        model.NewID(),
		ProjectID: nt.ProjectID,
		Title:     nt.Title,
		Status:    model.TaskOpen,
		Priority:  2,
		DueDay:    nt.DueDay,
		Envelope:  model.Envelope{CreatedAt: now, UpdatedAt: now, DeviceID: r.db.DeviceID(), Rev: 1},
	}
	if nt.Notes != nil {
		t.Notes = *nt.Notes
	}
	if nt.Priority != nil {
		t.Priority = *nt.Priority
	}
	if nt.EstimateMinutes != nil {
		d := time.Duration(*nt.EstimateMinutes) * time.Minute
		t.Estimate = &d
	}
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		if err := validTask(ctx, tx, &t); err != nil {
			return err
		}
		const qInsertTask = `INSERT INTO tasks (id, project_id, title, notes, status, priority, due_day,
			estimate_minutes, done_at, created_at, updated_at, device_id, rev)
			VALUES (?, ?, ?, ?, 'open', ?, ?, ?, NULL, ?, ?, ?, 1)`
		_, err := tx.ExecContext(ctx, qInsertTask, t.ID, nullString(t.ProjectID), t.Title, t.Notes, t.Priority,
			nullString(t.DueDay), estimateMinutes(t.Estimate), now.UnixMilli(), now.UnixMilli(), t.DeviceID)
		return classify(err)
	})
	if err != nil {
		return model.Task{}, fmt.Errorf("create task: %w", err)
	}
	return t, nil
}

func estimateMinutes(d *time.Duration) sql.NullInt64 {
	if d == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*d / time.Minute), Valid: true}
}

// validTask checks and normalizes a task's fields before a write.
func validTask(ctx context.Context, q Querier, t *model.Task) error {
	t.Title = strings.TrimSpace(t.Title)
	if n := len([]rune(t.Title)); n < 1 || n > 200 {
		return FailField(ErrInvalid, "title", "a task title must be 1 to 200 characters")
	}
	if t.Priority < 1 || t.Priority > 4 {
		return FailField(ErrInvalid, "priority", "priority must be 1 to 4")
	}
	if t.DueDay != nil && !validDay(*t.DueDay) {
		return FailField(ErrInvalid, "due_day", "due_day must be a date like 2026-09-15")
	}
	if t.Estimate != nil && *t.Estimate < time.Minute {
		return FailField(ErrInvalid, "estimate_minutes", "estimate_minutes must be positive")
	}
	if t.ProjectID != nil {
		return projectExists(ctx, q, *t.ProjectID, "project_id")
	}
	return nil
}

func (r taskRepo) Update(ctx context.Context, id string, p TaskPatch) (model.Task, error) {
	var t model.Task
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		if t, err = getTask(ctx, tx, id); err != nil {
			return err
		}
		if err := checkRev(p.Rev, t.Rev); err != nil {
			return err
		}
		if p.ProjectID.Set {
			t.ProjectID = p.ProjectID.Value
		}
		if p.Title != nil {
			t.Title = *p.Title
		}
		if p.Notes != nil {
			t.Notes = *p.Notes
		}
		if p.Priority != nil {
			t.Priority = *p.Priority
		}
		if p.DueDay.Set {
			t.DueDay = p.DueDay.Value
		}
		if p.EstimateMinutes.Set {
			t.Estimate = nil
			if p.EstimateMinutes.Value != nil {
				d := time.Duration(*p.EstimateMinutes.Value) * time.Minute
				t.Estimate = &d
			}
		}
		if err := validTask(ctx, tx, &t); err != nil {
			return err
		}
		return r.write(ctx, tx, &t)
	})
	if err != nil {
		return model.Task{}, fmt.Errorf("update task %s: %w", id, err)
	}
	return t, nil
}

// write stores every column of t with a new envelope write.
func (r taskRepo) write(ctx context.Context, tx *sql.Tx, t *model.Task) error {
	now := r.db.Now()
	t.UpdatedAt, t.DeviceID, t.Rev = now, r.db.DeviceID(), t.Rev+1
	const qUpdateTask = `UPDATE tasks SET project_id = ?, title = ?, notes = ?, status = ?, priority = ?,
		due_day = ?, estimate_minutes = ?, done_at = ?, updated_at = ?, device_id = ?, rev = ? WHERE id = ?`
	_, err := tx.ExecContext(ctx, qUpdateTask, nullString(t.ProjectID), t.Title, t.Notes, t.Status, t.Priority,
		nullString(t.DueDay), estimateMinutes(t.Estimate), nullMillis(t.DoneAt), now.UnixMilli(), t.DeviceID,
		t.Rev, t.ID)
	return classify(err)
}

func (r taskRepo) Complete(ctx context.Context, id string) (model.Task, error) {
	return r.setStatus(ctx, id, model.TaskDone)
}

func (r taskRepo) Reopen(ctx context.Context, id string) (model.Task, error) {
	return r.setStatus(ctx, id, model.TaskOpen)
}

func (r taskRepo) setStatus(ctx context.Context, id, status string) (model.Task, error) {
	var t model.Task
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		if t, err = getTask(ctx, tx, id); err != nil {
			return err
		}
		if t.Status == status {
			return nil
		}
		t.Status, t.DoneAt = status, nil
		if status == model.TaskDone {
			now := r.db.Now()
			t.DoneAt = &now
		}
		return r.write(ctx, tx, &t)
	})
	if err != nil {
		return model.Task{}, fmt.Errorf("set task %s %s: %w", id, status, err)
	}
	return t, nil
}

func (r taskRepo) Delete(ctx context.Context, id string) error {
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := getTask(ctx, tx, id); err != nil {
			return err
		}
		now := r.db.Now().UnixMilli()
		const qDeleteTask = `UPDATE tasks SET deleted_at = ?, updated_at = ?, device_id = ?, rev = rev + 1 WHERE id = ?`
		_, err := tx.ExecContext(ctx, qDeleteTask, now, now, r.db.DeviceID(), id)
		return err
	})
	if err != nil {
		return fmt.Errorf("delete task %s: %w", id, err)
	}
	return nil
}

func (r taskRepo) Tracked(ctx context.Context, ids []string, now time.Time) (map[string]time.Duration, error) {
	out := map[string]time.Duration{}
	if len(ids) == 0 {
		return out, nil
	}
	qTracked := `SELECT task_id, SUM(COALESCE(ended_at, ?) - started_at) FROM segments
		WHERE kind = 'work' AND deleted_at IS NULL AND task_id IN (` + placeholders(len(ids)) + `)
		GROUP BY task_id`
	args := []any{now.UnixMilli()}
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := r.db.sql.QueryContext(ctx, qTracked, args...)
	if err != nil {
		return nil, fmt.Errorf("tracked time: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var ms int64
		if err := rows.Scan(&id, &ms); err != nil {
			return nil, fmt.Errorf("tracked time: %w", err)
		}
		out[id] = time.Duration(ms) * time.Millisecond
	}
	return out, rows.Err()
}
