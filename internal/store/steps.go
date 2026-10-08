package store

import (
	"context"
	"database/sql"
	"errors"
	"slices"

	"github.com/kzark/gwen/internal/model"
)

// Steps, a quantity goal's items and sessions, and the cascades between them
// (docs/06-planner.md#sessions-and-steps and #deleting).

// stepsOf returns the live steps of the parents, by created_at then id.
func stepsOf(ctx context.Context, q Querier, parentIDs ...string) ([]model.Task, error) {
	if len(parentIDs) == 0 {
		return []model.Task{}, nil
	}
	qSteps := `SELECT ` + taskCols + ` FROM tasks WHERE deleted_at IS NULL AND parent_id IN (` +
		placeholders(len(parentIDs)) + `) ORDER BY created_at, id`
	return queryTasks(ctx, q, qSteps, anys(parentIDs)...)
}

// isSession reports whether t is a session: an occurrence of a live quantity
// goal's template.
func isSession(ctx context.Context, q Querier, t model.Task) (bool, error) {
	if t.TemplateID == nil || t.GoalID == nil {
		return false, nil
	}
	g, err := getGoal(ctx, q, *t.GoalID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return g.Kind == model.GoalQuantity, nil
}

// defaultUnit gives an item or session step without a quantity one unit,
// so it counts when done.
func defaultUnit(ctx context.Context, q Querier, t *model.Task) error {
	if t.Quantity != nil || t.GoalID == nil || t.RRule != nil || t.TemplateID != nil {
		return nil
	}
	g, err := getGoal(ctx, q, *t.GoalID)
	if err != nil || g.Kind != model.GoalQuantity {
		return err
	}
	one := 1
	t.Quantity = &one
	return nil
}

// unitsOf is the summed quantity of tasks, each counting 1 without one.
func unitsOf(tasks []model.Task) int {
	n := 0
	for _, t := range tasks {
		if t.Quantity == nil {
			n++
		} else {
			n += *t.Quantity
		}
	}
	return n
}

// validStep checks a step's parent and copies the parent's project, and a
// session's goal, onto it.
func validStep(ctx context.Context, q Querier, t *model.Task) error {
	if *t.ParentID == t.ID {
		return FailField(ErrInvalid, "parent_id", "a task cannot be its own step")
	}
	parent, err := getTask(ctx, q, *t.ParentID)
	if errors.Is(err, ErrNotFound) {
		return FailField(ErrNotFound, "parent_id", "no task %s", *t.ParentID)
	}
	if err != nil {
		return err
	}
	switch {
	case parent.IsStep():
		return FailField(ErrInvalid, "parent_id", "a step cannot have steps of its own")
	case parent.IsTemplate():
		return FailField(ErrInvalid, "parent_id", "a repeating task has no steps; add them to one day's copy")
	case t.RRule != nil:
		return FailField(ErrInvalid, "rrule", "a step cannot repeat")
	case t.TemplateID != nil:
		return FailField(ErrInvalid, "parent_id", "a repeating task's copy cannot be a step")
	}
	own, err := stepsOf(ctx, q, t.ID)
	if err != nil {
		return err
	}
	if len(own) > 0 {
		return FailField(ErrInvalid, "parent_id", "a task with steps cannot become a step")
	}
	t.ProjectID = parent.ProjectID
	session, err := isSession(ctx, q, parent)
	if err != nil {
		return err
	}
	if session {
		t.GoalID = parent.GoalID
	}
	return nil
}

// moveSteps keeps the steps of t in t's project.
func moveSteps(ctx context.Context, tx *sql.Tx, db *DB, t model.Task) error {
	steps, err := stepsOf(ctx, tx, t.ID)
	if err != nil {
		return err
	}
	for _, s := range steps {
		if equalPtr(s.ProjectID, t.ProjectID) {
			continue
		}
		s.ProjectID = t.ProjectID
		if err := writeTask(ctx, tx, db, &s); err != nil {
			return err
		}
	}
	return nil
}

func equalPtr[T comparable](a, b *T) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

// detach makes the steps top-level tasks again; a session's open steps so
// return to its goal's queue.
func detach(ctx context.Context, tx *sql.Tx, db *DB, steps []model.Task, ch *Changes) error {
	for _, s := range steps {
		s.ParentID = nil
		if err := writeTask(ctx, tx, db, &s); err != nil {
			return err
		}
		ch.task(s.ID)
	}
	return nil
}

func openOnly(tasks []model.Task) []model.Task {
	return slices.DeleteFunc(slices.Clone(tasks), func(t model.Task) bool { return t.Status != model.TaskOpen })
}

// queueOf returns a goal's open items, front first.
func queueOf(ctx context.Context, q Querier, goalID string) ([]model.Task, error) {
	const qQueue = `SELECT ` + taskCols + ` FROM tasks WHERE deleted_at IS NULL AND status = 'open' AND goal_id = ?
		AND rrule IS NULL AND template_id IS NULL AND parent_id IS NULL ORDER BY created_at, id`
	return queryTasks(ctx, q, qQueue, goalID)
}

// fill gives an open session items from the front of its goal's queue until
// its steps cover its quantity.
func fill(ctx context.Context, tx *sql.Tx, db *DB, session model.Task, ch *Changes) error {
	if session.Quantity == nil || session.GoalID == nil || session.Status != model.TaskOpen {
		return nil
	}
	steps, err := stepsOf(ctx, tx, session.ID)
	if err != nil {
		return err
	}
	have := unitsOf(steps)
	if have >= *session.Quantity {
		return nil
	}
	queue, err := queueOf(ctx, tx, *session.GoalID)
	if err != nil {
		return err
	}
	for _, it := range queue {
		if have >= *session.Quantity {
			break
		}
		it.ParentID, it.ProjectID = &session.ID, session.ProjectID
		if err := writeTask(ctx, tx, db, &it); err != nil {
			return err
		}
		ch.task(it.ID)
		have += unitsOf([]model.Task{it})
	}
	return nil
}

// refill deals a goal's queue out again after leftovers came back to it: the
// open sessions from today on that nothing was done in yet let their steps
// go, and the open sessions fill anew, earliest first, so the oldest items
// lead.
func refill(ctx context.Context, tx *sql.Tx, db *DB, goalID, today string, ch *Changes) error {
	const qComingSessions = `SELECT ` + taskCols + ` FROM tasks WHERE deleted_at IS NULL AND status = 'open'
		AND goal_id = ? AND template_id IS NOT NULL AND occurrence_day >= ? ORDER BY occurrence_day, id`
	sessions, err := queryTasks(ctx, tx, qComingSessions, goalID, today)
	if err != nil {
		return err
	}
	for _, s := range sessions {
		steps, err := stepsOf(ctx, tx, s.ID)
		if err != nil {
			return err
		}
		if len(openOnly(steps)) < len(steps) {
			continue // work started on it stays where it is
		}
		if err := detach(ctx, tx, db, steps, ch); err != nil {
			return err
		}
	}
	return fillGoal(ctx, tx, db, goalID, ch)
}

// fillGoal fills the open sessions of a quantity goal, earliest first.
func fillGoal(ctx context.Context, tx *sql.Tx, db *DB, goalID string, ch *Changes) error {
	g, err := getGoal(ctx, tx, goalID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil || g.Kind != model.GoalQuantity {
		return err
	}
	const qOpenSessions = `SELECT ` + taskCols + ` FROM tasks WHERE deleted_at IS NULL AND status = 'open'
		AND goal_id = ? AND template_id IS NOT NULL ORDER BY occurrence_day, id`
	sessions, err := queryTasks(ctx, tx, qOpenSessions, goalID)
	if err != nil {
		return err
	}
	for _, s := range sessions {
		if err := fill(ctx, tx, db, s, ch); err != nil {
			return err
		}
	}
	return nil
}

// completeTask completes t with its cascades: a session with steps counts
// nothing itself, and the last open step completes its parent. Steps never
// move: a session done early keeps its open steps until its day is over,
// when rollover returns them to the queue (settleSessions), so that reopening
// it, as after a mistaken tick, gives back everything it held. A done task is
// returned unchanged.
func completeTask(ctx context.Context, tx *sql.Tx, db *DB, t model.Task, quantityDone *int, ch *Changes) (model.Task, error) {
	if t.IsTemplate() {
		return t, FailField(ErrInvalid, "id", "a recurring template is never completed; complete its occurrence")
	}
	if t.Status == model.TaskDone {
		return t, nil
	}
	steps, err := stepsOf(ctx, tx, t.ID)
	if err != nil {
		return t, err
	}
	session, err := isSession(ctx, tx, t)
	if err != nil {
		return t, err
	}
	now := db.Now()
	t.Status, t.DoneAt, t.QuantityDone = model.TaskDone, &now, t.Quantity
	if session && len(steps) > 0 {
		zero := 0 // its steps count instead
		t.QuantityDone = &zero
	}
	if quantityDone != nil {
		t.QuantityDone = quantityDone
	}
	if err := writeTask(ctx, tx, db, &t); err != nil {
		return t, err
	}
	days, err := setItemStatus(ctx, tx, db, t.ID, model.PlanPlanned, model.PlanDone, "")
	if err != nil {
		return t, err
	}
	for _, d := range days {
		ch.plan(d)
	}
	changed, err := settleGoal(ctx, tx, db, t.GoalID, true)
	if err != nil {
		return t, err
	}
	ch.Goals = ch.Goals || changed
	if t.ParentID == nil {
		return t, nil
	}
	parent, err := getTask(ctx, tx, *t.ParentID)
	if errors.Is(err, ErrNotFound) || (err == nil && parent.Status != model.TaskOpen) {
		return t, nil
	}
	if err != nil {
		return t, err
	}
	siblings, err := stepsOf(ctx, tx, parent.ID)
	if err != nil {
		return t, err
	}
	if len(openOnly(siblings)) > 0 {
		return t, nil
	}
	ch.task(parent.ID)
	_, err = completeTask(ctx, tx, db, parent, nil, ch)
	return t, err
}

// reopenTask reopens t, and its parent when that is done. An open task is
// returned unchanged.
func reopenTask(ctx context.Context, tx *sql.Tx, db *DB, t model.Task, today string, ch *Changes) (model.Task, error) {
	if t.Status == model.TaskOpen {
		return t, nil
	}
	t.Status, t.DoneAt, t.QuantityDone = model.TaskOpen, nil, nil
	if err := writeTask(ctx, tx, db, &t); err != nil {
		return t, err
	}
	days, err := setItemStatus(ctx, tx, db, t.ID, model.PlanDone, model.PlanPlanned, today)
	if err != nil {
		return t, err
	}
	for _, d := range days {
		ch.plan(d)
	}
	changed, err := settleGoal(ctx, tx, db, t.GoalID, false)
	if err != nil {
		return t, err
	}
	ch.Goals = ch.Goals || changed
	if t.ParentID == nil {
		return t, nil
	}
	parent, err := getTask(ctx, tx, *t.ParentID)
	if errors.Is(err, ErrNotFound) || (err == nil && parent.Status != model.TaskDone) {
		return t, nil
	}
	if err != nil {
		return t, err
	}
	ch.task(parent.ID)
	_, err = reopenTask(ctx, tx, db, parent, today, ch)
	return t, err
}

// deleteTask soft-deletes t with its planned items and its steps, detaching
// a session's steps instead, and a template's open occurrences
// (docs/06-planner.md#deleting).
func deleteTask(ctx context.Context, tx *sql.Tx, db *DB, t model.Task, ch *Changes) error {
	if t.DeletedAt != nil || slices.Contains(ch.Deleted, t.ID) {
		return nil
	}
	// Whether t is a session depends on its goal, so ask before anything goes.
	session, err := isSession(ctx, tx, t)
	if err != nil {
		return err
	}
	if err := softDelete(ctx, tx, db, "tasks", t.ID); err != nil {
		return err
	}
	ch.deleted(t.ID)
	if err := deletePlanned(ctx, tx, db, t.ID, ch); err != nil {
		return err
	}
	steps, err := stepsOf(ctx, tx, t.ID)
	if err != nil {
		return err
	}
	if session {
		if err := detach(ctx, tx, db, steps, ch); err != nil {
			return err
		}
	} else {
		for _, s := range steps {
			if err := deleteTask(ctx, tx, db, s, ch); err != nil {
				return err
			}
		}
	}
	if !t.IsTemplate() {
		return nil
	}
	const qOpenOccurrences = `SELECT ` + taskCols + ` FROM tasks WHERE deleted_at IS NULL AND status = 'open'
		AND template_id = ? ORDER BY occurrence_day, id`
	occurrences, err := queryTasks(ctx, tx, qOpenOccurrences, t.ID)
	if err != nil {
		return err
	}
	for _, o := range occurrences {
		if err := deleteTask(ctx, tx, db, o, ch); err != nil {
			return err
		}
	}
	return nil
}

// deletePlanned soft-deletes a task's planned items, on any day.
func deletePlanned(ctx context.Context, tx *sql.Tx, db *DB, taskID string, ch *Changes) error {
	const qPlannedItems = `SELECT ` + planItemCols + ` FROM plan_items WHERE deleted_at IS NULL
		AND status = 'planned' AND task_id = ?`
	items, err := queryPlanItems(ctx, tx, qPlannedItems, taskID)
	if err != nil {
		return err
	}
	for _, it := range items {
		if err := softDelete(ctx, tx, db, "plan_items", it.ID); err != nil {
			return err
		}
		ch.plan(it.Day)
	}
	return nil
}

// deleteGoal soft-deletes a goal and its session template, and with
// openTasks its open items and steps (docs/06-planner.md#deleting).
func deleteGoal(ctx context.Context, tx *sql.Tx, db *DB, g model.Goal, openTasks bool, ch *Changes) error {
	// The template goes first, while the goal is live, so its sessions
	// detach their steps rather than deleting them.
	const qTemplates = `SELECT ` + taskCols + ` FROM tasks WHERE deleted_at IS NULL AND goal_id = ?
		AND rrule IS NOT NULL ORDER BY created_at, id`
	templates, err := queryTasks(ctx, tx, qTemplates, g.ID)
	if err != nil {
		return err
	}
	for _, t := range templates {
		if err := deleteTask(ctx, tx, db, t, ch); err != nil {
			return err
		}
	}
	if err := softDelete(ctx, tx, db, "goals", g.ID); err != nil {
		return err
	}
	ch.Goals = true
	if !openTasks {
		return nil
	}
	const qOpenGoalTasks = `SELECT ` + taskCols + ` FROM tasks WHERE deleted_at IS NULL AND status = 'open'
		AND goal_id = ? ORDER BY created_at, id`
	open, err := queryTasks(ctx, tx, qOpenGoalTasks, g.ID)
	if err != nil {
		return err
	}
	for _, t := range open {
		if err := deleteTask(ctx, tx, db, t, ch); err != nil {
			return err
		}
	}
	return nil
}
