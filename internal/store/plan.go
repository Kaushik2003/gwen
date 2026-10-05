package store

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
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
	// Frog places the hardest work first (planner.eat_the_frog).
	Frog bool
	// Prime is the biological prime time, minutes after midnight, or nil.
	Prime *[2]int
}

// Today is the day the current instant belongs to.
func (e PlanEnv) Today() string { return e.DayOf(e.Now) }

func (e PlanEnv) today() civil.Day { return civil.MustParse(e.Today()) }

// PlanEntry is a plan item with its task and the task's live steps.
type PlanEntry struct {
	Item  model.PlanItem
	Task  model.Task
	Steps []model.Task
}

// Plan is a day's plan.
type Plan struct {
	Day        string
	Capacity   int // minutes
	Planned    int // minutes over items planned or done
	Start, End int // the capacity window, minutes after local midnight
	Hours      *model.DayHours
	Items      []PlanEntry
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
	// Read returns the stored plan for day ("" means today) and never rolls
	// over or generates; the sync hub serves plans this way.
	Read(ctx context.Context, day string, env PlanEnv) (Plan, error)
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
	// Preview computes the progress g would have today without writing it:
	// with the tasks and session rule of the live goal existingID, or as a
	// new goal with a daily session when existingID is "".
	Preview(ctx context.Context, g model.Goal, existingID string, env PlanEnv) (planner.GoalProgress, error)
	// Slots lists the next n sessions of a quantity goal that still need
	// items, with the goal's progress today.
	Slots(ctx context.Context, g model.Goal, n int, env PlanEnv) ([]planner.SessionSlot, planner.GoalProgress, error)
	// SetHours replaces day's hours, a nil field meaning the usual value and
	// both nil none, then regenerates its plan (docs/06-planner.md#day-hours).
	SetHours(ctx context.Context, day string, startMinute, workMinutes *int, env PlanEnv) (Plan, Changes, error)
	// DayPlan prepares day as GET /v1/plan does and returns what a day plan
	// conversation plans from (docs/07-integrations.md#day-plan).
	DayPlan(ctx context.Context, day string, env PlanEnv) (DayPlanContext, Changes, error)
	// Schedule puts a task on day at start (nil leaves it to the planner's
	// order) for minutes, pinned: the user's own time for it. Its other
	// planned blocks from today on are removed, so scheduling also moves a
	// task. Today's plan is then regenerated around it.
	Schedule(ctx context.Context, taskID, day string, start *time.Time, minutes int, env PlanEnv) (PlanEntry, Changes, error)
	// Unschedule removes a task's planned blocks from today on, or on day
	// only when day is not "", and regenerates today's plan.
	Unschedule(ctx context.Context, taskID, day string, env PlanEnv) (Changes, error)
	// Refresh regenerates today's plan after tasks changed, so new and edited
	// tasks show up without asking. It keeps pinned items.
	Refresh(ctx context.Context, env PlanEnv) (Changes, error)
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

// planTasks wraps tasks for the planner, anchoring templates at their
// start_day, else the day their created_at belongs to. A nil dayOf leaves
// anchors zero, for callers that need no recurrence.
func planTasks(tasks []model.Task, dayOf func(time.Time) string) []planner.Task {
	out := make([]planner.Task, len(tasks))
	for i, t := range tasks {
		out[i] = planner.Task{Task: t}
		switch {
		case !t.IsTemplate():
		case t.StartDay != nil:
			out[i].Anchor = civil.MustParse(*t.StartDay)
		case dayOf != nil:
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

func (r planRepo) Read(ctx context.Context, day string, env PlanEnv) (Plan, error) {
	d, err := r.parseDay(day, env)
	if err != nil {
		return Plan{}, err
	}
	p, err := r.read(ctx, r.db.sql, d, env)
	if err != nil {
		return Plan{}, fmt.Errorf("plan for %s: %w", d, err)
	}
	return p, nil
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
	const qCountItems = `SELECT count(*) FROM plan_items p JOIN tasks t ON t.id = p.task_id
		WHERE p.day = ? AND p.deleted_at IS NULL AND t.deleted_at IS NULL`
	var n int
	if err := tx.QueryRowContext(ctx, qCountItems, today.String()).Scan(&n); err != nil {
		return fmt.Errorf("count plan items: %w", err)
	}
	planned, err := GetLocal(ctx, tx, KeyPlannedDay)
	if err != nil && !isNotFound(err) {
		return err
	}
	if planned != today.String() {
		return r.generate(ctx, tx, today, env, ch)
	}
	if n > 0 {
		return nil
	}
	// Generated today but empty: try again, since work may fit now, but a plan
	// that stays empty changed nothing. Announcing it would have every client
	// that rereads on plan_changed read again, forever.
	announced := slices.Contains(ch.PlanDays, today.String())
	if err := r.generate(ctx, tx, today, env, ch); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, qCountItems, today.String()).Scan(&n); err != nil {
		return fmt.Errorf("count plan items: %w", err)
	}
	if n == 0 && !announced {
		ch.PlanDays = slices.DeleteFunc(ch.PlanDays, func(d string) bool { return d == today.String() })
	}
	return nil
}

// rollover settles the planned items of earlier days and expires overdue
// occurrences (docs/06-planner.md#rollover).
func (r planRepo) rollover(ctx context.Context, tx *sql.Tx, today civil.Day, ch *Changes) error {
	if err := r.settleSessions(ctx, tx, today, ch); err != nil {
		return fmt.Errorf("rollover: %w", err)
	}
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
		t, err := getTask(ctx, tx, id)
		if err != nil {
			return fmt.Errorf("rollover: %w", err)
		}
		if err := deleteTask(ctx, tx, r.db, t, ch); err != nil {
			return fmt.Errorf("rollover: %w", err)
		}
	}
	return nil
}

// settleSessions returns the open steps of expired sessions to their goals'
// queues and completes those with a done step, whose steps already count.
func (r planRepo) settleSessions(ctx context.Context, tx *sql.Tx, today civil.Day, ch *Changes) error {
	const qExpiredSessions = `SELECT ` + taskCols + ` FROM tasks WHERE deleted_at IS NULL AND status = 'open'
		AND template_id IS NOT NULL AND goal_id IS NOT NULL AND due_day < ? ORDER BY occurrence_day, id`
	expired, err := queryTasks(ctx, tx, qExpiredSessions, today.String())
	if err != nil {
		return err
	}
	for _, t := range expired {
		session, err := isSession(ctx, tx, t)
		if err != nil {
			return err
		}
		if !session {
			continue
		}
		steps, err := stepsOf(ctx, tx, t.ID)
		if err != nil {
			return err
		}
		if err := detach(ctx, tx, r.db, openOnly(steps), ch); err != nil {
			return err
		}
		if len(openOnly(steps)) == len(steps) {
			continue // nothing done: it expires
		}
		zero := 0
		if _, err := completeTask(ctx, tx, r.db, t, &zero, ch); err != nil {
			return err
		}
		ch.task(t.ID)
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
		if tmpl.StartMinute != nil {
			occ.StartDay, occ.StartMinute = &day, tmpl.StartMinute
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
		if err := fill(ctx, tx, r.db, occ, ch); err != nil {
			return fmt.Errorf("materialize %s on %s: %w", tmpl.ID, day, err)
		}
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

// generateInput loads the planner's input for d, with every live item on d
// (Existing holds those whose task is live).
func (r planRepo) generateInput(ctx context.Context, q Querier, d civil.Day, env PlanEnv) (planner.GenerateInput, []model.PlanItem, error) {
	const qDayItems = `SELECT ` + planItemCols + ` FROM plan_items WHERE day = ? AND deleted_at IS NULL`
	onDay, err := queryPlanItems(ctx, q, qDayItems, d.String())
	if err != nil {
		return planner.GenerateInput{}, nil, err
	}
	const qOpenTasks = `SELECT ` + taskCols + ` FROM tasks WHERE deleted_at IS NULL AND status = 'open'
		ORDER BY created_at, id`
	open, err := queryTasks(ctx, q, qOpenTasks)
	if err != nil {
		return planner.GenerateInput{}, nil, err
	}
	live, err := liveTaskIDs(ctx, q, ids(onDay, func(it model.PlanItem) string { return it.TaskID }))
	if err != nil {
		return planner.GenerateInput{}, nil, err
	}
	in := planner.GenerateInput{Day: d, Loc: env.Loc, Tasks: planTasks(open, env.DayOf), Frog: env.Frog,
		Prime: env.Prime, Elsewhere: map[string]bool{}}
	const qElsewhere = `SELECT DISTINCT task_id FROM plan_items WHERE deleted_at IS NULL AND status = 'planned'
		AND pinned = 1 AND day <> ? AND day >= ?`
	elsewhere, err := queryIDs(ctx, q, qElsewhere, d.String(), env.Today())
	if err != nil {
		return planner.GenerateInput{}, nil, err
	}
	for _, id := range elsewhere {
		in.Elsewhere[id] = true
	}
	for _, it := range onDay {
		if live[it.TaskID] {
			in.Existing = append(in.Existing, it)
		}
	}
	capacity, err := r.capacity(ctx, q, d, env, nil)
	if err != nil {
		return planner.GenerateInput{}, nil, err
	}
	in.Capacity = capacity.CapacityResult
	openIDs := ids(open, func(t model.Task) string { return t.ID })
	if in.Tracked, err = trackedTime(ctx, q, openIDs, env.Now); err != nil {
		return planner.GenerateInput{}, nil, err
	}
	if in.Progress, err = r.activeProgress(ctx, q, d, env); err != nil {
		return planner.GenerateInput{}, nil, err
	}
	if in.Latest, err = latestBefore(ctx, q, d, openIDs); err != nil {
		return planner.GenerateInput{}, nil, err
	}
	if in.Goals, err = liveGoals(ctx, q); err != nil {
		return planner.GenerateInput{}, nil, err
	}
	return in, onDay, nil
}

// generate runs steps 1 to 9 of docs/06-planner.md#generating-a-plan for d,
// after rollover.
func (r planRepo) generate(ctx context.Context, tx *sql.Tx, d civil.Day, env PlanEnv, ch *Changes) error {
	if err := r.materialize(ctx, tx, d, env, ch); err != nil {
		return err
	}
	in, onDay, err := r.generateInput(ctx, tx, d, env)
	if err != nil {
		return fmt.Errorf("generate: %w", err)
	}
	drafts := planner.Generate(in)
	if d == env.today() {
		if err := SetLocal(ctx, tx, KeyPlannedDay, d.String(), r.db.Now()); err != nil {
			return fmt.Errorf("generate: %w", err)
		}
	}

	ch.plan(d.String())
	kept := map[string]planner.PlanItemDraft{}
	for _, dr := range drafts {
		if dr.ExistingID != "" {
			kept[dr.ExistingID] = dr
		}
	}
	// An item the plan drops is reused for a new block of its task, so that
	// regenerating rewrites rows rather than piling up deleted ones.
	reusable := map[string][]*model.PlanItem{}
	for i := range onDay {
		it := &onDay[i]
		dr, ok := kept[it.ID]
		switch {
		case !ok:
			reusable[it.TaskID] = append(reusable[it.TaskID], it)
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
		if old := reusable[dr.TaskID]; len(old) > 0 && old[0].Status == model.PlanPlanned && !old[0].Pinned {
			it := old[0]
			reusable[dr.TaskID] = old[1:]
			if it.Planned == dr.Planned && it.Position == dr.Position && sameTime(it.StartAt, dr.StartAt) {
				continue
			}
			it.Planned, it.StartAt, it.Position = dr.Planned, dr.StartAt, dr.Position
			if err := writePlanItem(ctx, tx, r.db, it); err != nil {
				return fmt.Errorf("generate: %w", err)
			}
			continue
		}
		it := model.PlanItem{ID: model.NewID(), Day: d.String(), TaskID: dr.TaskID, Planned: dr.Planned,
			StartAt: dr.StartAt, Position: dr.Position, Status: model.PlanPlanned, RolledFromID: dr.RolledFromID,
			RolloverCount: dr.RolloverCount}
		if err := insertPlanItem(ctx, tx, r.db, &it); err != nil {
			return fmt.Errorf("generate: %w", err)
		}
	}
	for _, old := range reusable {
		for _, it := range old {
			if err := softDelete(ctx, tx, r.db, "plan_items", it.ID); err != nil {
				return fmt.Errorf("generate: %w", err)
			}
		}
	}
	return nil
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
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

// dayCapacity is a day's capacity with the window and hours it came from.
type dayCapacity struct {
	planner.CapacityResult
	Start, End int // the window, minutes after local midnight
	Hours      *model.DayHours
}

// capacity computes d's capacity. commitments, when nil, are loaded.
func (r planRepo) capacity(ctx context.Context, q Querier, d civil.Day, env PlanEnv, commitments []model.Commitment) (dayCapacity, error) {
	in, hours, err := capacityInput(ctx, q, d, env, commitments)
	if err != nil {
		return dayCapacity{}, err
	}
	return dayCapacity{CapacityResult: planner.Capacity(in), Start: in.DayStart, End: in.DayEnd, Hours: hours}, nil
}

// capacityInput loads Capacity's input for d under its day hours, which it
// also returns. Busy time is loaded from the earlier of planner.day_start and
// the window's start, so the input serves that wider window too.
func capacityInput(ctx context.Context, q Querier, d civil.Day, env PlanEnv, commitments []model.Commitment) (planner.CapacityInput, *model.DayHours, error) {
	if commitments == nil {
		var err error
		if commitments, err = listCommitments(ctx, q); err != nil {
			return planner.CapacityInput{}, nil, err
		}
	}
	hours, err := dayHoursOn(ctx, q, d.String())
	if err != nil {
		return planner.CapacityInput{}, nil, err
	}
	in := planner.CapacityInput{
		Day: d, Loc: env.Loc, Today: d == env.today(), Now: env.Now, DayStart: env.DayStart, DayEnd: env.DayEnd,
		Buffer: env.Buffer, Target: env.DailyTarget, Commitments: commitments,
	}
	if hours != nil {
		if hours.StartMinute != nil {
			in.DayStart = *hours.StartMinute
		}
		in.Work = hours.Work
	}
	if env.Busy {
		busy, err := busyBetween(ctx, q, d.At(min(env.DayStart, in.DayStart), env.Loc), d.At(env.DayEnd, env.Loc))
		if err != nil {
			return planner.CapacityInput{}, nil, err
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
			var since time.Time // all of today's work, unless the day has its own time for tasks
			if in.Work != nil {
				since = d.At(in.DayStart, env.Loc)
			}
			if in.WorkedToday, err = projectWork(ctx, q, w.ID, since, env.Now); err != nil {
				return planner.CapacityInput{}, nil, err
			}
		case !isNotFound(err):
			return planner.CapacityInput{}, nil, err
		}
	}
	return in, hours, nil
}

// projectWork is a work day's work by project from since (the zero time for
// all of it), the open segment counting up to now.
func projectWork(ctx context.Context, q Querier, workDayID string, since, now time.Time) ([]planner.ProjectWork, error) {
	var from int64
	if !since.IsZero() {
		from = since.UnixMilli()
	}
	const qProjectWork = `SELECT project_id, SUM(MAX(0, COALESCE(ended_at, ?) - MAX(started_at, ?))) FROM segments
		WHERE work_day_id = ? AND kind = 'work' AND deleted_at IS NULL GROUP BY project_id`
	rows, err := q.QueryContext(ctx, qProjectWork, now.UnixMilli(), from, workDayID)
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

// liveGoals returns every live goal by id.
func liveGoals(ctx context.Context, q Querier) (map[string]model.Goal, error) {
	goals, err := queryGoals(ctx, q, `SELECT `+goalCols+` FROM goals WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("live goals: %w", err)
	}
	out := make(map[string]model.Goal, len(goals))
	for _, g := range goals {
		out[g.ID] = g
	}
	return out, nil
}

// previewID stands for a goal that does not exist yet.
const previewID = "preview"

func (r planRepo) Preview(ctx context.Context, g model.Goal, existingID string, env PlanEnv) (planner.GoalProgress, error) {
	today := env.today()
	g.ID, g.Title, g.Status, g.ProjectID = previewID, previewID, model.GoalActive, nil
	if existingID != "" {
		existing, err := getGoal(ctx, r.db.sql, existingID)
		if err != nil {
			return planner.GoalProgress{}, err
		}
		g.ID, g.Status = existing.ID, existing.Status
	}
	if err := validGoal(ctx, r.db.sql, &g); err != nil {
		return planner.GoalProgress{}, err
	}
	if existingID == "" {
		rule := SessionRule
		var tasks []planner.Task
		if g.Kind == model.GoalQuantity {
			tasks = []planner.Task{{Task: model.Task{ID: previewID, GoalID: &g.ID, RRule: &rule}, Anchor: today}}
		}
		return planner.Progress(g, tasks, today), nil
	}
	tasks, err := goalTasks(ctx, r.db.sql, []string{existingID})
	if err != nil {
		return planner.GoalProgress{}, err
	}
	return planner.Progress(g, planTasks(tasks, env.DayOf), today), nil
}

func (r planRepo) Slots(ctx context.Context, g model.Goal, n int, env PlanEnv) ([]planner.SessionSlot, planner.GoalProgress, error) {
	today := env.today()
	tasks, err := goalTasks(ctx, r.db.sql, []string{g.ID})
	if err != nil {
		return nil, planner.GoalProgress{}, err
	}
	pts := planTasks(tasks, env.DayOf)
	p := planner.Progress(g, pts, today)
	for _, t := range pts {
		if t.IsTemplate() {
			return planner.Sessions(g, t, p, today, p.LinedUp, n), p, nil
		}
	}
	return nil, p, nil
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
	p := Plan{Day: d.String(), Capacity: capacity.Minutes, Start: capacity.Start, End: capacity.End,
		Hours: capacity.Hours, Items: entries}
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
	steps, err := stepsOf(ctx, q, ids(tasks, func(t model.Task) string { return t.ID })...)
	if err != nil {
		return nil, fmt.Errorf("plan steps: %w", err)
	}
	stepsByParent := map[string][]model.Task{}
	for _, s := range steps {
		stepsByParent[*s.ParentID] = append(stepsByParent[*s.ParentID], s)
	}
	out := []PlanEntry{}
	for _, it := range items {
		if t, ok := byID[it.TaskID]; ok {
			out = append(out, PlanEntry{Item: it, Task: t, Steps: stepsByParent[t.ID]})
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
		e = PlanEntry{Item: it, Task: entries[0].Task, Steps: entries[0].Steps}
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
	goals, err := liveGoals(ctx, q)
	if err != nil {
		return nil, err
	}
	return planner.Reminders(planner.ReminderInput{
		Today: today, Tasks: planTasks(tasks, nil), Tracked: tracked, PlannedToday: planned,
		DayCapacity: capacity, Progress: progress, RolloverCounts: counts, Goals: goals,
	}), nil
}
