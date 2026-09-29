package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/planner"
	"github.com/kzark/gwen/internal/planner/civil"
)

// Goal status filters for listing goals.
const GoalsAll = "all"

// SessionRule is the recurrence of a new quantity goal's session template.
const SessionRule = "FREQ=DAILY"

// sessionPriority is the priority of a session template.
const sessionPriority = 3

// NewGoal is a goal to create. A nil Unit takes "".
type NewGoal struct {
	Title          string
	Kind           string
	Unit           *string
	TargetQuantity *int
	MinutesPerUnit *int
	ProjectID      *string
	StartDay       string
	DueDay         string
}

// GoalPatch changes a goal; unset fields are unchanged.
type GoalPatch struct {
	Title          *string
	Kind           *string
	Unit           *string
	TargetQuantity Nullable[int]
	MinutesPerUnit Nullable[int]
	ProjectID      Nullable[string]
	StartDay       *string
	DueDay         *string
	Status         *string
	Rev            *int64
}

// GoalRepo reads and writes goals.
type GoalRepo interface {
	// List returns live goals with the status ("" means active, or "all"),
	// by due_day.
	List(ctx context.Context, status string) ([]model.Goal, error)
	Get(ctx context.Context, id string) (model.Goal, error)
	// Create inserts a goal and, for a quantity goal, its session template in
	// the same transaction.
	Create(ctx context.Context, g NewGoal) (model.Goal, Changes, error)
	Update(ctx context.Context, id string, p GoalPatch) (model.Goal, error)
	// Delete soft-deletes the goal, its session template, and the template's
	// open occurrences.
	Delete(ctx context.Context, id string) (Changes, error)
}

type goalRepo struct{ db *DB }

// NewGoalRepo returns the SQLite GoalRepo.
func NewGoalRepo(db *DB) GoalRepo { return goalRepo{db} }

const goalCols = `id, title, kind, unit, target_quantity, minutes_per_unit, project_id, start_day, due_day, status, ` +
	envelopeCols

func scanGoal(s scanner) (model.Goal, error) {
	var (
		g               model.Goal
		target, perUnit sql.NullInt64
		project         sql.NullString
		env             envelopeScan
	)
	dest := append([]any{&g.ID, &g.Title, &g.Kind, &g.Unit, &target, &perUnit, &project, &g.StartDay, &g.DueDay,
		&g.Status}, env.dest()...)
	if err := s.Scan(dest...); err != nil {
		return model.Goal{}, err
	}
	g.TargetQuantity, g.ProjectID = intPtr(target), stringPtr(project)
	if perUnit.Valid {
		d := time.Duration(perUnit.Int64) * time.Minute
		g.PerUnit = &d
	}
	g.Envelope = env.envelope()
	return g, nil
}

func queryGoals(ctx context.Context, q Querier, query string, args ...any) ([]model.Goal, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Goal{}
	for rows.Next() {
		g, err := scanGoal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r goalRepo) List(ctx context.Context, status string) ([]model.Goal, error) {
	q := `SELECT ` + goalCols + ` FROM goals WHERE deleted_at IS NULL`
	var args []any
	switch status {
	case "":
		status = model.GoalActive
		fallthrough
	case model.GoalActive, model.GoalDone, model.GoalAbandoned:
		q += ` AND status = ?`
		args = append(args, status)
	case GoalsAll:
	default:
		return nil, FailField(ErrInvalid, "status", "status must be active, done, abandoned, or all")
	}
	out, err := queryGoals(ctx, r.db.sql, q+` ORDER BY due_day, created_at, id`, args...)
	if err != nil {
		return nil, fmt.Errorf("list goals: %w", err)
	}
	return out, nil
}

func (r goalRepo) Get(ctx context.Context, id string) (model.Goal, error) {
	return getGoal(ctx, r.db.sql, id)
}

func getGoal(ctx context.Context, q Querier, id string) (model.Goal, error) {
	const qGetGoal = `SELECT ` + goalCols + ` FROM goals WHERE id = ? AND deleted_at IS NULL`
	g, err := scanGoal(q.QueryRowContext(ctx, qGetGoal, id))
	if err != nil {
		return model.Goal{}, fmt.Errorf("get goal: %w", notFound(err, "goal", id))
	}
	return g, nil
}

func minutesPtr(n *int) *time.Duration {
	if n == nil {
		return nil
	}
	d := time.Duration(*n) * time.Minute
	return &d
}

func (r goalRepo) Create(ctx context.Context, ng NewGoal) (model.Goal, Changes, error) {
	now := r.db.Now()
	g := model.Goal{
		ID: model.NewID(), Title: ng.Title, Kind: ng.Kind, TargetQuantity: ng.TargetQuantity,
		PerUnit: minutesPtr(ng.MinutesPerUnit), ProjectID: ng.ProjectID, StartDay: ng.StartDay, DueDay: ng.DueDay,
		Status:   model.GoalActive,
		Envelope: model.Envelope{CreatedAt: now, UpdatedAt: now, DeviceID: r.db.DeviceID(), Rev: 1},
	}
	if ng.Unit != nil {
		g.Unit = *ng.Unit
	}
	var ch Changes
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		if err := validGoal(ctx, tx, &g); err != nil {
			return err
		}
		const qInsertGoal = `INSERT INTO goals (id, title, kind, unit, target_quantity, minutes_per_unit, project_id,
			start_day, due_day, status, created_at, updated_at, device_id, rev)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`
		if _, err := tx.ExecContext(ctx, qInsertGoal, g.ID, g.Title, g.Kind, g.Unit, nullInt(g.TargetQuantity),
			estimateMinutes(g.PerUnit), nullString(g.ProjectID), g.StartDay, g.DueDay, g.Status,
			now.UnixMilli(), now.UnixMilli(), g.DeviceID); err != nil {
			return classify(err)
		}
		ch.Goals = true
		if g.Kind != model.GoalQuantity {
			return nil
		}
		rule := SessionRule
		t := model.Task{ID: model.NewID(), ProjectID: g.ProjectID, Title: g.Title, Status: model.TaskOpen,
			Priority: sessionPriority, GoalID: &g.ID, RRule: &rule, Envelope: g.Envelope}
		ch.task(t.ID)
		return insertTask(ctx, tx, &t)
	})
	if err != nil {
		return model.Goal{}, Changes{}, fmt.Errorf("create goal: %w", err)
	}
	return g, ch, nil
}

// validGoal checks and normalizes a goal's fields before a write.
func validGoal(ctx context.Context, q Querier, g *model.Goal) error {
	g.Title = strings.TrimSpace(g.Title)
	if n := len([]rune(g.Title)); n < 1 || n > 200 {
		return FailField(ErrInvalid, "title", "a goal title must be 1 to 200 characters")
	}
	switch g.Kind {
	case model.GoalQuantity:
		if g.TargetQuantity == nil || *g.TargetQuantity < 1 {
			return FailField(ErrInvalid, "target_quantity", "a quantity goal needs a positive target_quantity")
		}
		if g.PerUnit == nil || *g.PerUnit < time.Minute {
			return FailField(ErrInvalid, "minutes_per_unit", "a quantity goal needs a positive minutes_per_unit")
		}
	case model.GoalTasks:
		if g.TargetQuantity != nil {
			return FailField(ErrInvalid, "target_quantity", "a tasks goal has no target_quantity")
		}
		if g.PerUnit != nil {
			return FailField(ErrInvalid, "minutes_per_unit", "a tasks goal has no minutes_per_unit")
		}
	default:
		return FailField(ErrInvalid, "kind", "kind must be quantity or tasks")
	}
	switch g.Status {
	case model.GoalActive, model.GoalDone, model.GoalAbandoned:
	default:
		return FailField(ErrInvalid, "status", "status must be active, done, or abandoned")
	}
	if !validDay(g.StartDay) {
		return FailField(ErrInvalid, "start_day", "start_day must be a date like 2026-09-15")
	}
	if !validDay(g.DueDay) {
		return FailField(ErrInvalid, "due_day", "due_day must be a date like 2026-09-15")
	}
	if g.DueDay < g.StartDay {
		return FailField(ErrInvalid, "due_day", "due_day must not be before start_day")
	}
	if g.ProjectID != nil {
		return projectExists(ctx, q, *g.ProjectID, "project_id")
	}
	return nil
}

func (r goalRepo) Update(ctx context.Context, id string, p GoalPatch) (model.Goal, error) {
	var g model.Goal
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		if g, err = getGoal(ctx, tx, id); err != nil {
			return err
		}
		if err := checkRev(p.Rev, g.Rev); err != nil {
			return err
		}
		if p.Kind != nil && *p.Kind != g.Kind {
			return FailField(ErrInvalid, "kind", "a goal's kind cannot change")
		}
		if p.Title != nil {
			g.Title = *p.Title
		}
		if p.Unit != nil {
			g.Unit = *p.Unit
		}
		if p.TargetQuantity.Set {
			g.TargetQuantity = p.TargetQuantity.Value
		}
		if p.MinutesPerUnit.Set {
			g.PerUnit = minutesPtr(p.MinutesPerUnit.Value)
		}
		if p.ProjectID.Set {
			g.ProjectID = p.ProjectID.Value
		}
		if p.StartDay != nil {
			g.StartDay = *p.StartDay
		}
		if p.DueDay != nil {
			g.DueDay = *p.DueDay
		}
		if p.Status != nil {
			g.Status = *p.Status
		}
		if err := validGoal(ctx, tx, &g); err != nil {
			return err
		}
		return writeGoal(ctx, tx, r.db, &g)
	})
	if err != nil {
		return model.Goal{}, fmt.Errorf("update goal %s: %w", id, err)
	}
	return g, nil
}

// writeGoal stores every column of g with a new envelope write.
func writeGoal(ctx context.Context, tx *sql.Tx, db *DB, g *model.Goal) error {
	now := db.Now()
	g.UpdatedAt, g.DeviceID, g.Rev = now, db.DeviceID(), g.Rev+1
	const qUpdateGoal = `UPDATE goals SET title = ?, unit = ?, target_quantity = ?, minutes_per_unit = ?,
		project_id = ?, start_day = ?, due_day = ?, status = ?, updated_at = ?, device_id = ?, rev = ? WHERE id = ?`
	_, err := tx.ExecContext(ctx, qUpdateGoal, g.Title, g.Unit, nullInt(g.TargetQuantity), estimateMinutes(g.PerUnit),
		nullString(g.ProjectID), g.StartDay, g.DueDay, g.Status, now.UnixMilli(), g.DeviceID, g.Rev, g.ID)
	return classify(err)
}

func (r goalRepo) Delete(ctx context.Context, id string) (Changes, error) {
	var ch Changes
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := getGoal(ctx, tx, id); err != nil {
			return err
		}
		now, dev := r.db.Now().UnixMilli(), r.db.DeviceID()
		const qDeleteGoal = `UPDATE goals SET deleted_at = ?, updated_at = ?, device_id = ?, rev = rev + 1 WHERE id = ?`
		if _, err := tx.ExecContext(ctx, qDeleteGoal, now, now, dev, id); err != nil {
			return err
		}
		ch.Goals = true
		const qSessionTasks = `SELECT id FROM tasks WHERE deleted_at IS NULL AND (
			(goal_id = ?1 AND rrule IS NOT NULL) OR
			(status = 'open' AND template_id IN (SELECT id FROM tasks WHERE goal_id = ?1 AND rrule IS NOT NULL)))`
		ids, err := queryIDs(ctx, tx, qSessionTasks, id)
		if err != nil {
			return err
		}
		for _, tid := range ids {
			ch.task(tid)
			if err := softDelete(ctx, tx, r.db, "tasks", tid); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Changes{}, fmt.Errorf("delete goal %s: %w", id, err)
	}
	return ch, nil
}

// settleGoal keeps a goal's status in step with its progress after a task
// linked to it is completed or reopened: completing the last unit marks an
// active goal done, and reopening one marks a done goal active. It reports
// whether the goal changed.
func settleGoal(ctx context.Context, tx *sql.Tx, db *DB, goalID *string, completed bool) (bool, error) {
	if goalID == nil {
		return false, nil
	}
	g, err := getGoal(ctx, tx, *goalID)
	if err != nil {
		return false, nil // a deleted goal has no status to keep
	}
	tasks, err := goalTasks(ctx, tx, []string{g.ID})
	if err != nil {
		return false, err
	}
	// Remaining does not depend on the day or on template anchors.
	remaining := planner.Progress(g, planTasks(tasks, nil), civil.MustParse(g.StartDay)).Remaining
	switch {
	case completed && g.Status == model.GoalActive && remaining == 0:
		g.Status = model.GoalDone
	case !completed && g.Status == model.GoalDone && remaining > 0:
		g.Status = model.GoalActive
	default:
		return false, nil
	}
	return true, writeGoal(ctx, tx, db, &g)
}

// goalTasks returns the live tasks linked to the goals, templates included.
func goalTasks(ctx context.Context, q Querier, goalIDs []string) ([]model.Task, error) {
	if len(goalIDs) == 0 {
		return []model.Task{}, nil
	}
	qGoalTasks := `SELECT ` + taskCols + ` FROM tasks WHERE deleted_at IS NULL AND goal_id IN (` +
		placeholders(len(goalIDs)) + `) ORDER BY created_at, id`
	out, err := queryTasks(ctx, q, qGoalTasks, anys(goalIDs)...)
	if err != nil {
		return nil, fmt.Errorf("goal tasks: %w", err)
	}
	return out, nil
}

// softDelete tombstones one row of a synced table named by the store itself.
func softDelete(ctx context.Context, tx *sql.Tx, db *DB, table, id string) error {
	now := db.Now().UnixMilli()
	q := `UPDATE ` + table + ` SET deleted_at = ?, updated_at = ?, device_id = ?, rev = rev + 1
		WHERE id = ? AND deleted_at IS NULL`
	_, err := tx.ExecContext(ctx, q, now, now, db.DeviceID(), id)
	return err
}

func queryIDs(ctx context.Context, q Querier, query string, args ...any) ([]string, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func anys(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
