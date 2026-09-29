package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/planner"
	"github.com/kzark/gwen/internal/planner/civil"
)

// PlanEnv is the current instant and the settings the planner runs with.
type PlanEnv struct {
	Now time.Time
	Loc *time.Location
	// DayOf is the day an instant belongs to (docs/05-time-engine.md#day-boundaries).
	DayOf            func(time.Time) string
	DayStart, DayEnd int // planner.day_start and day_end, minutes after midnight
	Buffer           time.Duration
	DailyTarget      time.Duration
	// Busy subtracts calendar busy time from capacity; it is set while the
	// calendar is enabled.
	Busy bool
}

// Today is the day the current instant belongs to.
func (e PlanEnv) Today() string { return e.DayOf(e.Now) }

func (e PlanEnv) today() civil.Day { return civil.MustParse(e.Today()) }

// PlanEntry is a plan item with its task.
type PlanEntry struct {
	Item model.PlanItem
	Task model.Task
}

// Plan is a day's plan.
type Plan struct {
	Day      string
	Capacity int // minutes
	Planned  int // minutes over items planned or done
	Items    []PlanEntry
}

// GoalProgress is a goal with its progress today.
type GoalProgress struct {
	Goal     model.Goal
	Progress planner.GoalProgress
}

// Briefing is what to start today (docs/06-planner.md#briefing).
type Briefing struct {
	Day       string
	Pending   []PlanEntry
	Today     []PlanEntry
	Reminders []planner.Reminder
	Goals     []GoalProgress
}

// PlanItemPatch changes a plan item; unset fields are unchanged.
type PlanItemPatch struct {
	StartAt        Nullable[time.Time]
	Position       *int
	Pinned         *bool
	PlannedMinutes *int
	Status         *string // planned or skipped
	Rev            *int64
}

// PlanRepo runs the planner over the store: every call loads its inputs,
// runs the pure planner, and writes the results in one transaction.
type PlanRepo interface {
	// Get returns the plan for day ("" means today). For today it first runs
	// rollover and, when today has no live items, generates the plan.
	Get(ctx context.Context, day string, env PlanEnv) (Plan, Changes, error)
	// Generate regenerates the plan for today or a later day.
	Generate(ctx context.Context, day string, env PlanEnv) (Plan, Changes, error)
	// UpdateItem edits an item. Changing start_at, position, or
	// planned_minutes also pins it.
	UpdateItem(ctx context.Context, id string, p PlanItemPatch) (PlanEntry, error)
	// Briefing runs rollover and implicit generation for today and returns
	// the briefing.
	Briefing(ctx context.Context, env PlanEnv) (Briefing, Changes, error)
	// Progress computes the goals' progress today.
	Progress(ctx context.Context, goals []model.Goal, env PlanEnv) (map[string]planner.GoalProgress, error)
}

type planRepo struct{ db *DB }

// NewPlanRepo returns the SQLite PlanRepo.
func NewPlanRepo(db *DB) PlanRepo { return planRepo{db} }

const planItemCols = `id, day, task_id, planned_minutes, start_at, position, status, pinned, rolled_from_id,
	rollover_count, ` + envelopeCols

func scanPlanItem(s scanner) (model.PlanItem, error) {
	var (
		it              model.PlanItem
		planned         int64
		start           sql.NullInt64
		pinned          int
		rolledFrom      sql.NullString
		env             envelopeScan
		position, count int
	)
	dest := append([]any{&it.ID, &it.Day, &it.TaskID, &planned, &start, &position, &it.Status, &pinned, &rolledFrom,
		&count}, env.dest()...)
	if err := s.Scan(dest...); err != nil {
		return model.PlanItem{}, err
	}
	it.Planned, it.StartAt, it.Position = time.Duration(planned)*time.Minute, timePtr(start), position
	it.Pinned, it.RolledFromID, it.RolloverCount = pinned == 1, stringPtr(rolledFrom), count
	it.Envelope = env.envelope()
	return it, nil
}

func queryPlanItems(ctx context.Context, q Querier, query string, args ...any) ([]model.PlanItem, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.PlanItem{}
	for rows.Next() {
		it, err := scanPlanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func insertPlanItem(ctx context.Context, tx *sql.Tx, db *DB, it *model.PlanItem) error {
	now := db.Now()
	it.Envelope = model.Envelope{CreatedAt: now, UpdatedAt: now, DeviceID: db.DeviceID(), Rev: 1}
	const qInsertPlanItem = `INSERT INTO plan_items (id, day, task_id, planned_minutes, start_at, position, status,
		pinned, rolled_from_id, rollover_count, created_at, updated_at, device_id, rev)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`
	_, err := tx.ExecContext(ctx, qInsertPlanItem, it.ID, it.Day, it.TaskID, int64(it.Planned/time.Minute),
		nullMillis(it.StartAt), it.Position, it.Status, boolInt(it.Pinned), nullString(it.RolledFromID),
		it.RolloverCount, now.UnixMilli(), now.UnixMilli(), it.DeviceID)
	return classify(err)
}

// writePlanItem stores every column of it with a new envelope write.
func writePlanItem(ctx context.Context, tx *sql.Tx, db *DB, it *model.PlanItem) error {
	now := db.Now()
	it.UpdatedAt, it.DeviceID, it.Rev = now, db.DeviceID(), it.Rev+1
	const qUpdatePlanItem = `UPDATE plan_items SET planned_minutes = ?, start_at = ?, position = ?, status = ?,
		pinned = ?, updated_at = ?, device_id = ?, rev = ? WHERE id = ?`
	_, err := tx.ExecContext(ctx, qUpdatePlanItem, int64(it.Planned/time.Minute), nullMillis(it.StartAt), it.Position,
		it.Status, boolInt(it.Pinned), now.UnixMilli(), it.DeviceID, it.Rev, it.ID)
	return classify(err)
}

// setItemStatus moves a task's live items from one status to another, on day
// only when day is not "", and returns the days it changed.
func setItemStatus(ctx context.Context, tx *sql.Tx, db *DB, taskID, from, to, day string) ([]string, error) {
	q := `SELECT ` + planItemCols + ` FROM plan_items WHERE task_id = ? AND status = ? AND deleted_at IS NULL`
	args := []any{taskID, from}
	if day != "" {
		q += ` AND day = ?`
		args = append(args, day)
	}
	items, err := queryPlanItems(ctx, tx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("plan items of task %s: %w", taskID, err)
	}
	var days []string
	for i := range items {
		items[i].Status = to
		if err := writePlanItem(ctx, tx, db, &items[i]); err != nil {
			return nil, err
		}
		days = append(days, items[i].Day)
	}
	return days, nil
}

// planTasks wraps tasks for the planner, anchoring templates at the day their
// created_at belongs to. A nil dayOf leaves anchors zero, for callers that
// need no recurrence.
func planTasks(tasks []model.Task, dayOf func(time.Time) string) []planner.Task {
	out := make([]planner.Task, len(tasks))
	for i, t := range tasks {
		out[i] = planner.Task{Task: t}
		if dayOf != nil && t.IsTemplate() {
			out[i].Anchor = civil.MustParse(dayOf(t.CreatedAt))
		}
	}
	return out
}

func taskMap(tasks []planner.Task) map[string]planner.Task {
	m := make(map[string]planner.Task, len(tasks))
	for _, t := range tasks {
		m[t.ID] = t
	}
	return m
}

func ids[T any](xs []T, id func(T) string) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = id(x)
	}
	return out
}

// tasksByID loads tasks by id, deleted ones included.
func tasksByID(ctx context.Context, q Querier, taskIDs []string) ([]model.Task, error) {
	if len(taskIDs) == 0 {
		return []model.Task{}, nil
	}
	qTasksByID := `SELECT ` + taskCols + ` FROM tasks WHERE id IN (` + placeholders(len(taskIDs)) + `)`
	return queryTasks(ctx, q, qTasksByID, anys(taskIDs)...)
}

func (r planRepo) parseDay(day string, env PlanEnv) (civil.Day, error) {
	if day == "" {
		return env.today(), nil
	}
	d, err := civil.Parse(day)
	if err != nil {
		return civil.Day{}, FailField(ErrInvalid, "day", "day must be a date like 2026-09-15")
	}
	return d, nil
}

func (r planRepo) Get(ctx context.Context, day string, env PlanEnv) (Plan, Changes, error) {
	d, err := r.parseDay(day, env)
	if err != nil {
		return Plan{}, Changes{}, err
	}
	var (
		p  Plan
		ch Changes
	)
	err = r.db.InTx(ctx, func(tx *sql.Tx) error {
		if d == env.today() {
			if err := r.prepareToday(ctx, tx, env, &ch); err != nil {
				return err
			}
		}
		var err error
		p, err = r.read(ctx, tx, d, env)
		return err
	})
	if err != nil {
		return Plan{}, Changes{}, fmt.Errorf("plan for %s: %w", d, err)
	}
	return p, ch, nil
}

func (r planRepo) Generate(ctx context.Context, day string, env PlanEnv) (Plan, Changes, error) {
	d, err := r.parseDay(day, env)
	if err != nil {
		return Plan{}, Changes{}, err
	}
	today := env.today()
	if d.Before(today) {
		return Plan{}, Changes{}, FailField(ErrInvalid, "day", "a day in the past cannot be planned")
	}
	var (
		p  Plan
		ch Changes
	)
	err = r.db.InTx(ctx, func(tx *sql.Tx) error {
		if d == today {
			if err := r.rollover(ctx, tx, today, &ch); err != nil {
				return err
			}
		}
		if err := r.generate(ctx, tx, d, env, &ch); err != nil {
			return err
		}
		var err error
		p, err = r.read(ctx, tx, d, env)
		return err
	})
	if err != nil {
		return Plan{}, Changes{}, fmt.Errorf("generate plan for %s: %w", d, err)
	}
	return p, ch, nil
}

// prepareToday runs rollover for today and generates today's plan when it has
// no live items.
func (r planRepo) prepareToday(ctx context.Context, tx *sql.Tx, env PlanEnv, ch *Changes) error {
	today := env.today()
	if err := r.rollover(ctx, tx, today, ch); err != nil {
		return err
	}
	const qCountItems = `SELECT count(*) FROM plan_items WHERE day = ? AND deleted_at IS NULL`
	var n int
	if err := tx.QueryRowContext(ctx, qCountItems, today.String()).Scan(&n); err != nil {
		return fmt.Errorf("count plan items: %w", err)
	}
	if n > 0 {
		return nil
	}
	return r.generate(ctx, tx, today, env, ch)
}

// rollover settles the planned items of earlier days and expires overdue
// occurrences (docs/06-planner.md#rollover).
func (r planRepo) rollover(ctx context.Context, tx *sql.Tx, today civil.Day, ch *Changes) error {
	const qStale = `SELECT ` + planItemCols + ` FROM plan_items
		WHERE deleted_at IS NULL AND status = 'planned' AND day < ?`
	items, err := queryPlanItems(ctx, tx, qStale, today.String())
	if err != nil {
		return fmt.Errorf("rollover: %w", err)
	}
	tasks, err := tasksByID(ctx, tx, ids(items, func(it model.PlanItem) string { return it.TaskID }))
	if err != nil {
		return fmt.Errorf("rollover: %w", err)
	}
	const qExpired = `SELECT ` + taskCols + ` FROM tasks WHERE deleted_at IS NULL AND status = 'open'
		AND template_id IS NOT NULL AND due_day < ?`
	expired, err := queryTasks(ctx, tx, qExpired, today.String())
	if err != nil {
		return fmt.Errorf("rollover: %w", err)
	}
	res := planner.Rollover(items, taskMap(planTasks(append(tasks, expired...), nil)), today)
	byID := map[string]*model.PlanItem{}
	for i := range items {
		byID[items[i].ID] = &items[i]
	}
	for _, c := range res.Changes {
		it := byID[c.ItemID]
		it.Status = c.Status
		if err := writePlanItem(ctx, tx, r.db, it); err != nil {
			return fmt.Errorf("rollover: %w", err)
		}
		ch.plan(c.Day)
	}
	for _, id := range res.Expire {
		if err := softDelete(ctx, tx, r.db, "tasks", id); err != nil {
			return fmt.Errorf("rollover: %w", err)
		}
		ch.task(id)
	}
	return nil
}

// materialize creates the occurrences of every live template that recurs on
// d and has none there yet, even a deleted one.
func (r planRepo) materialize(ctx context.Context, tx *sql.Tx, d civil.Day, env PlanEnv, ch *Changes) error {
	const qTemplates = `SELECT ` + taskCols + ` FROM tasks WHERE deleted_at IS NULL AND rrule IS NOT NULL
		ORDER BY created_at, id`
	templates, err := queryTasks(ctx, tx, qTemplates)
	if err != nil {
		return fmt.Errorf("materialize: %w", err)
	}
	for _, tmpl := range planTasks(templates, env.DayOf) {
		rule, err := planner.ParseRule(*tmpl.RRule)
		if err != nil || !rule.Matches(tmpl.Anchor, d) {
			continue
		}
		const qHasOccurrence = `SELECT count(*) FROM tasks WHERE template_id = ? AND occurrence_day = ?`
		var n int
		if err := tx.QueryRowContext(ctx, qHasOccurrence, tmpl.ID, d.String()).Scan(&n); err != nil {
			return fmt.Errorf("materialize: %w", err)
		}
		if n > 0 {
			continue
		}
		now := r.db.Now()
		due, day := rule.OccurrenceDue(tmpl.Anchor, d).String(), d.String()
		occ := model.Task{
			ID: model.NewID(), ProjectID: tmpl.ProjectID, Title: tmpl.Title, Notes: tmpl.Notes, Status: model.TaskOpen,
			Priority: tmpl.Priority, DueDay: &due, Estimate: tmpl.Estimate, GoalID: tmpl.GoalID,
			TemplateID: &tmpl.ID, OccurrenceDay: &day,
			Envelope: model.Envelope{CreatedAt: now, UpdatedAt: now, DeviceID: r.db.DeviceID(), Rev: 1},
		}
		if tmpl.GoalID != nil {
			ok, err := r.session(ctx, tx, *tmpl.GoalID, d, env, &occ)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
		}
		if err := insertTask(ctx, tx, &occ); err != nil {
			return fmt.Errorf("materialize %s on %s: %w", tmpl.ID, day, err)
		}
		ch.task(occ.ID)
	}
	return nil
}

// session sizes an occurrence of a quantity goal's template on d, and reports
// false when no session is due (docs/06-planner.md#goals-and-progress).
func (r planRepo) session(ctx context.Context, tx *sql.Tx, goalID string, d civil.Day, env PlanEnv, occ *model.Task) (bool, error) {
	g, err := getGoal(ctx, tx, goalID)
	if err != nil || g.Kind != model.GoalQuantity {
		return err == nil, nil // a template of a deleted goal is deleted with it
	}
	tasks, err := goalTasks(ctx, tx, []string{g.ID})
	if err != nil {
		return false, err
	}
	q, ok := planner.SessionQuantity(g, planner.Progress(g, planTasks(tasks, env.DayOf), d), d)
	if !ok {
		return false, nil
	}
	estimate := time.Duration(q) * *g.PerUnit
	occ.Quantity, occ.Estimate = &q, &estimate
	return true, nil
}

// generate runs steps 1 to 9 of docs/06-planner.md#generating-a-plan for d,
// after rollover.
func (r planRepo) generate(ctx context.Context, tx *sql.Tx, d civil.Day, env PlanEnv, ch *Changes) error {
	if err := r.materialize(ctx, tx, d, env, ch); err != nil {
		return err
	}
	const qDayItems = `SELECT ` + planItemCols + ` FROM plan_items WHERE day = ? AND deleted_at IS NULL`
	onDay, err := queryPlanItems(ctx, tx, qDayItems, d.String())
	if err != nil {
		return fmt.Errorf("generate: %w", err)
	}
	const qOpenTasks = `SELECT ` + taskCols + ` FROM tasks WHERE deleted_at IS NULL AND status = 'open'
		ORDER BY created_at, id`
	open, err := queryTasks(ctx, tx, qOpenTasks)
	if err != nil {
		return fmt.Errorf("generate: %w", err)
	}
	live, err := liveTaskIDs(ctx, tx, ids(onDay, func(it model.PlanItem) string { return it.TaskID }))
	if err != nil {
		return fmt.Errorf("generate: %w", err)
	}
	var existing []model.PlanItem
	for _, it := range onDay {
		if live[it.TaskID] {
			existing = append(existing, it)
		}
	}
	capacity, err := r.capacity(ctx, tx, d, env, nil)
	if err != nil {
		return err
	}
	openIDs := ids(open, func(t model.Task) string { return t.ID })
	tracked, err := trackedTime(ctx, tx, openIDs, env.Now)
	if err != nil {
		return err
	}
	progress, err := r.activeProgress(ctx, tx, d, env)
	if err != nil {
		return err
	}
	latest, err := latestBefore(ctx, tx, d, openIDs)
	if err != nil {
		return err
	}
	drafts := planner.Generate(planner.GenerateInput{
		Day: d, Capacity: capacity, Existing: existing, Tasks: planTasks(open, env.DayOf),
		Tracked: tracked, Progress: progress, Latest: latest,
	})

	ch.plan(d.String())
	kept := map[string]planner.PlanItemDraft{}
	for _, dr := range drafts {
		if dr.ExistingID != "" {
			kept[dr.ExistingID] = dr
		}
	}
	for i := range onDay {
		it := &onDay[i]
		dr, ok := kept[it.ID]
		switch {
		case !ok:
			if err := softDelete(ctx, tx, r.db, "plan_items", it.ID); err != nil {
				return fmt.Errorf("generate: %w", err)
			}
		case dr.Position != it.Position:
			it.Position = dr.Position
			if err := writePlanItem(ctx, tx, r.db, it); err != nil {
				return fmt.Errorf("generate: %w", err)
			}
		}
	}
	for _, dr := range drafts {
		if dr.ExistingID != "" {
			continue
		}
		it := model.PlanItem{ID: model.NewID(), Day: d.String(), TaskID: dr.TaskID, Planned: dr.Planned,
			StartAt: dr.StartAt, Position: dr.Position, Status: model.PlanPlanned, RolledFromID: dr.RolledFromID,
			RolloverCount: dr.RolloverCount}
		if err := insertPlanItem(ctx, tx, r.db, &it); err != nil {
			return fmt.Errorf("generate: %w", err)
		}
	}
	return nil
}

// liveTaskIDs reports which of the tasks are live.
func liveTaskIDs(ctx context.Context, q Querier, taskIDs []string) (map[string]bool, error) {
	tasks, err := tasksByID(ctx, q, taskIDs)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, t := range tasks {
		if t.DeletedAt == nil {
			out[t.ID] = true
		}
	}
	return out, nil
}

// latestBefore is each task's most recent live item on a day before d.
func latestBefore(ctx context.Context, q Querier, d civil.Day, taskIDs []string) (map[string]model.PlanItem, error) {
	out := map[string]model.PlanItem{}
	if len(taskIDs) == 0 {
		return out, nil
	}
	qLatest := `SELECT ` + planItemCols + ` FROM plan_items WHERE deleted_at IS NULL AND day < ?
		AND task_id IN (` + placeholders(len(taskIDs)) + `) ORDER BY day DESC, position, id`
	items, err := queryPlanItems(ctx, q, qLatest, append([]any{d.String()}, anys(taskIDs)...)...)
	if err != nil {
		return nil, fmt.Errorf("latest plan items: %w", err)
	}
	for _, it := range items {
		if _, ok := out[it.TaskID]; !ok {
			out[it.TaskID] = it
		}
	}
	return out, nil
}

// capacity computes d's capacity. commitments, when nil, are loaded.
func (r planRepo) capacity(ctx context.Context, q Querier, d civil.Day, env PlanEnv, commitments []model.Commitment) (planner.CapacityResult, error) {
	if commitments == nil {
		var err error
		if commitments, err = listCommitments(ctx, q); err != nil {
			return planner.CapacityResult{}, err
		}
	}
	in := planner.CapacityInput{
		Day: d, Loc: env.Loc, Today: d == env.today(), Now: env.Now, DayStart: env.DayStart, DayEnd: env.DayEnd,
		Buffer: env.Buffer, Target: env.DailyTarget, Commitments: commitments,
	}
	if env.Busy {
		busy, err := busyBetween(ctx, q, d.At(env.DayStart, env.Loc), d.At(env.DayEnd, env.Loc))
		if err != nil {
			return planner.CapacityResult{}, err
		}
		for _, b := range busy {
			in.Busy = append(in.Busy, planner.Interval{Start: b.Start, End: b.End})
		}
	}
	if in.Today {
		w, err := workDayByDay(ctx, q, d.String())
		switch {
		case err == nil:
			in.Target = w.Target
			if in.WorkedToday, err = projectWork(ctx, q, w.ID, env.Now); err != nil {
				return planner.CapacityResult{}, err
			}
		case !isNotFound(err):
			return planner.CapacityResult{}, err
		}
	}
	return planner.Capacity(in), nil
}

// projectWork is a work day's work by project, the open segment counting up to now.
func projectWork(ctx context.Context, q Querier, workDayID string, now time.Time) ([]planner.ProjectWork, error) {
	const qProjectWork = `SELECT project_id, SUM(COALESCE(ended_at, ?) - started_at) FROM segments
		WHERE work_day_id = ? AND kind = 'work' AND deleted_at IS NULL GROUP BY project_id`
	rows, err := q.QueryContext(ctx, qProjectWork, now.UnixMilli(), workDayID)
	if err != nil {
		return nil, fmt.Errorf("work by project: %w", err)
	}
	defer rows.Close()
	var out []planner.ProjectWork
	for rows.Next() {
		var project sql.NullString
		var ms int64
		if err := rows.Scan(&project, &ms); err != nil {
			return nil, fmt.Errorf("work by project: %w", err)
		}
		out = append(out, planner.ProjectWork{ProjectID: stringPtr(project), Worked: time.Duration(ms) * time.Millisecond})
	}
	return out, rows.Err()
}

// activeProgress is the progress of every active goal on d.
func (r planRepo) activeProgress(ctx context.Context, q Querier, d civil.Day, env PlanEnv) (map[string]planner.GoalProgress, error) {
	const qActiveGoals = `SELECT ` + goalCols + ` FROM goals WHERE deleted_at IS NULL AND status = 'active'
		ORDER BY due_day, created_at, id`
	goals, err := queryGoals(ctx, q, qActiveGoals)
	if err != nil {
		return nil, fmt.Errorf("active goals: %w", err)
	}
	return progressOf(ctx, q, goals, d, env)
}

func progressOf(ctx context.Context, q Querier, goals []model.Goal, d civil.Day, env PlanEnv) (map[string]planner.GoalProgress, error) {
	tasks, err := goalTasks(ctx, q, ids(goals, func(g model.Goal) string { return g.ID }))
	if err != nil {
		return nil, err
	}
	pts := planTasks(tasks, env.DayOf)
	out := make(map[string]planner.GoalProgress, len(goals))
	for _, g := range goals {
		out[g.ID] = planner.Progress(g, pts, d)
	}
	return out, nil
}

func (r planRepo) Progress(ctx context.Context, goals []model.Goal, env PlanEnv) (map[string]planner.GoalProgress, error) {
	return progressOf(ctx, r.db.sql, goals, env.today(), env)
}

// read returns d's plan: its live items whose task is live, by position.
func (r planRepo) read(ctx context.Context, q Querier, d civil.Day, env PlanEnv) (Plan, error) {
	capacity, err := r.capacity(ctx, q, d, env, nil)
	if err != nil {
		return Plan{}, err
	}
	const qPlanItems = `SELECT ` + planItemCols + ` FROM plan_items WHERE day = ? AND deleted_at IS NULL
		ORDER BY position, id`
	items, err := queryPlanItems(ctx, q, qPlanItems, d.String())
	if err != nil {
		return Plan{}, fmt.Errorf("read plan: %w", err)
	}
	entries, err := withTasks(ctx, q, items)
	if err != nil {
		return Plan{}, err
	}
	p := Plan{Day: d.String(), Capacity: capacity.Minutes, Items: entries}
	for _, e := range entries {
		if e.Item.Status == model.PlanPlanned || e.Item.Status == model.PlanDone {
			p.Planned += int(e.Item.Planned / time.Minute)
		}
	}
	return p, nil
}

// withTasks pairs items with their tasks, dropping items whose task is deleted.
func withTasks(ctx context.Context, q Querier, items []model.PlanItem) ([]PlanEntry, error) {
	tasks, err := tasksByID(ctx, q, ids(items, func(it model.PlanItem) string { return it.TaskID }))
	if err != nil {
		return nil, fmt.Errorf("plan tasks: %w", err)
	}
	byID := map[string]model.Task{}
	for _, t := range tasks {
		if t.DeletedAt == nil {
			byID[t.ID] = t
		}
	}
	out := []PlanEntry{}
	for _, it := range items {
		if t, ok := byID[it.TaskID]; ok {
			out = append(out, PlanEntry{Item: it, Task: t})
		}
	}
	return out, nil
}

func (r planRepo) UpdateItem(ctx context.Context, id string, p PlanItemPatch) (PlanEntry, error) {
	var e PlanEntry
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		const qGetPlanItem = `SELECT ` + planItemCols + ` FROM plan_items WHERE id = ? AND deleted_at IS NULL`
		it, err := scanPlanItem(tx.QueryRowContext(ctx, qGetPlanItem, id))
		if err != nil {
			return notFound(err, "plan item", id)
		}
		entries, err := withTasks(ctx, tx, []model.PlanItem{it})
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			return Fail(ErrNotFound, "no plan item %s", id)
		}
		if err := checkRev(p.Rev, it.Rev); err != nil {
			return err
		}
		if p.Pinned != nil {
			it.Pinned = *p.Pinned
		}
		if p.StartAt.Set {
			it.StartAt, it.Pinned = truncPtr(p.StartAt.Value), true
		}
		if p.Position != nil {
			if *p.Position < 0 {
				return FailField(ErrInvalid, "position", "position must not be negative")
			}
			it.Position, it.Pinned = *p.Position, true
		}
		if p.PlannedMinutes != nil {
			if *p.PlannedMinutes < 1 {
				return FailField(ErrInvalid, "planned_minutes", "planned_minutes must be positive")
			}
			it.Planned, it.Pinned = time.Duration(*p.PlannedMinutes)*time.Minute, true
		}
		if p.Status != nil {
			if *p.Status != model.PlanPlanned && *p.Status != model.PlanSkipped {
				return FailField(ErrInvalid, "status", "status must be planned or skipped")
			}
			it.Status = *p.Status
		}
		if err := writePlanItem(ctx, tx, r.db, &it); err != nil {
			return err
		}
		e = PlanEntry{Item: it, Task: entries[0].Task}
		return nil
	})
	if err != nil {
		return PlanEntry{}, fmt.Errorf("update plan item %s: %w", id, err)
	}
	return e, nil
}

func (r planRepo) Briefing(ctx context.Context, env PlanEnv) (Briefing, Changes, error) {
	var (
		b  Briefing
		ch Changes
	)
	today := env.today()
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		if err := r.prepareToday(ctx, tx, env, &ch); err != nil {
			return err
		}
		plan, err := r.read(ctx, tx, today, env)
		if err != nil {
			return err
		}
		b = Briefing{Day: today.String(), Today: plan.Items}
		if b.Pending, err = r.pending(ctx, tx); err != nil {
			return err
		}
		const qActiveGoals = `SELECT ` + goalCols + ` FROM goals WHERE deleted_at IS NULL AND status = 'active'
			ORDER BY due_day, created_at, id`
		goals, err := queryGoals(ctx, tx, qActiveGoals)
		if err != nil {
			return err
		}
		progress, err := progressOf(ctx, tx, goals, today, env)
		if err != nil {
			return err
		}
		for _, g := range goals {
			b.Goals = append(b.Goals, GoalProgress{Goal: g, Progress: progress[g.ID]})
		}
		b.Reminders, err = r.reminders(ctx, tx, env, plan, progress)
		return err
	})
	if err != nil {
		return Briefing{}, Changes{}, fmt.Errorf("briefing: %w", err)
	}
	if b.Goals == nil {
		b.Goals = []GoalProgress{}
	}
	return b, ch, nil
}

// pending is, for each open task, its most recent rolled item when nothing
// later has been planned for it.
func (r planRepo) pending(ctx context.Context, q Querier) ([]PlanEntry, error) {
	const qRolledTasksItems = `SELECT ` + planItemCols + ` FROM plan_items WHERE deleted_at IS NULL AND task_id IN
		(SELECT task_id FROM plan_items WHERE status = 'rolled' AND deleted_at IS NULL)`
	items, err := queryPlanItems(ctx, q, qRolledTasksItems)
	if err != nil {
		return nil, fmt.Errorf("pending: %w", err)
	}
	tasks, err := tasksByID(ctx, q, ids(items, func(it model.PlanItem) string { return it.TaskID }))
	if err != nil {
		return nil, fmt.Errorf("pending: %w", err)
	}
	return withTasks(ctx, q, planner.Pending(items, taskMap(planTasks(tasks, nil))))
}

// reminders computes the briefing's reminders for today.
func (r planRepo) reminders(ctx context.Context, q Querier, env PlanEnv, plan Plan, progress map[string]planner.GoalProgress) ([]planner.Reminder, error) {
	today := env.today()
	const qDueTasks = `SELECT ` + taskCols + ` FROM tasks WHERE deleted_at IS NULL AND status = 'open'
		AND rrule IS NULL AND due_day IS NOT NULL ORDER BY created_at, id`
	tasks, err := queryTasks(ctx, q, qDueTasks)
	if err != nil {
		return nil, fmt.Errorf("reminders: %w", err)
	}
	taskIDs := ids(tasks, func(t model.Task) string { return t.ID })
	tracked, err := trackedTime(ctx, q, taskIDs, env.Now)
	if err != nil {
		return nil, err
	}
	latest, err := latestBefore(ctx, q, today, taskIDs)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for id, it := range latest {
		counts[id], _ = planner.RolloverCount(&it)
	}
	planned := map[string]bool{}
	for _, e := range plan.Items {
		planned[e.Task.ID] = true
	}
	commitments, err := listCommitments(ctx, q)
	if err != nil {
		return nil, err
	}
	capacity := make([]int, 14)
	for i := range capacity {
		c, err := r.capacity(ctx, q, today.AddDays(i), env, commitments)
		if err != nil {
			return nil, err
		}
		capacity[i] = c.Minutes
	}
	return planner.Reminders(planner.ReminderInput{
		Today: today, Tasks: planTasks(tasks, nil), Tracked: tracked, PlannedToday: planned,
		DayCapacity: capacity, Progress: progress, RolloverCounts: counts,
	}), nil
}
