package store

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/planner"
	"github.com/kzark/gwen/internal/planner/civil"
)

const dayHoursCols = `id, day, start_minute, work_minutes, ` + envelopeCols

// dayHoursOn returns day's live hours, or nil when it has none.
func dayHoursOn(ctx context.Context, q Querier, day string) (*model.DayHours, error) {
	const qDayHours = `SELECT ` + dayHoursCols + ` FROM day_hours WHERE day = ? AND deleted_at IS NULL`
	var (
		h           model.DayHours
		start, work sql.NullInt64
		env         envelopeScan
	)
	err := q.QueryRowContext(ctx, qDayHours, day).Scan(append([]any{&h.ID, &h.Day, &start, &work}, env.dest()...)...)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("day hours of %s: %w", day, err)
	}
	h.StartMinute, h.Envelope = intPtr(start), env.envelope()
	if work.Valid {
		w := time.Duration(work.Int64) * time.Minute
		h.Work = &w
	}
	return &h, nil
}

// writeDayHours replaces day's hours, deleting its row when both are nil.
func writeDayHours(ctx context.Context, tx *sql.Tx, db *DB, day string, start, work *int) error {
	cur, err := dayHoursOn(ctx, tx, day)
	if err != nil {
		return err
	}
	now := db.Now().UnixMilli()
	switch {
	case start == nil && work == nil:
		if cur == nil {
			return nil
		}
		return softDelete(ctx, tx, db, "day_hours", cur.ID)
	case cur == nil:
		const qInsertDayHours = `INSERT INTO day_hours (id, day, start_minute, work_minutes, created_at, updated_at,
			device_id, rev) VALUES (?, ?, ?, ?, ?, ?, ?, 1)`
		_, err = tx.ExecContext(ctx, qInsertDayHours, model.NewID(), day, nullInt(start), nullInt(work), now, now,
			db.DeviceID())
	default:
		const qUpdateDayHours = `UPDATE day_hours SET start_minute = ?, work_minutes = ?, updated_at = ?,
			device_id = ?, rev = rev + 1 WHERE id = ?`
		_, err = tx.ExecContext(ctx, qUpdateDayHours, nullInt(start), nullInt(work), now, db.DeviceID(), cur.ID)
	}
	return classify(err)
}

// validHours checks a day's hours against docs/06-planner.md#day-hours.
func validHours(start, work *int, env PlanEnv) error {
	if start != nil && (*start < 0 || *start >= env.DayEnd) {
		return FailField(ErrInvalid, "start_minute", "the day must start before %02d:%02d, when planner.day_end ends it",
			env.DayEnd/60, env.DayEnd%60)
	}
	if work != nil && (*work < 0 || *work > 24*60) {
		return FailField(ErrInvalid, "work_minutes", "work_minutes must be 0 to 1440")
	}
	return nil
}

// plannable parses day and requires it to be today or later.
func (r planRepo) plannable(day string, env PlanEnv) (civil.Day, error) {
	d, err := r.parseDay(day, env)
	if err != nil {
		return civil.Day{}, err
	}
	if d.Before(env.today()) {
		return civil.Day{}, FailField(ErrInvalid, "day", "a day in the past cannot be planned")
	}
	return d, nil
}

func (r planRepo) SetHours(ctx context.Context, day string, start, work *int, env PlanEnv) (Plan, Changes, error) {
	d, err := r.plannable(day, env)
	if err != nil {
		return Plan{}, Changes{}, err
	}
	if err := validHours(start, work, env); err != nil {
		return Plan{}, Changes{}, err
	}
	var (
		p  Plan
		ch Changes
	)
	err = r.db.InTx(ctx, func(tx *sql.Tx) error {
		if err := writeDayHours(ctx, tx, r.db, d.String(), start, work); err != nil {
			return err
		}
		if d == env.today() {
			if err := r.rollover(ctx, tx, d, &ch); err != nil {
				return err
			}
		}
		// New hours weigh the day afresh: what fits is planned again.
		if err := r.generate(ctx, tx, d, env, false, &ch); err != nil {
			return err
		}
		var err error
		p, err = r.read(ctx, tx, d, env)
		return err
	})
	if err != nil {
		return Plan{}, Changes{}, fmt.Errorf("set hours of %s: %w", d, err)
	}
	ch.plan(d.String())
	return p, ch, nil
}

// DayPlanContext is what a day plan conversation plans from
// (docs/07-integrations.md#day-plan).
type DayPlanContext struct {
	Plan Plan
	// Free is the free time from the earlier of planner.day_start and the
	// window's start, less the times of the day's done items.
	Free        []planner.Interval
	Commitments []model.Commitment // those on the day
	Busy        []BusyInterval     // calendar events in the window
	Candidates  []Candidate        // the day's pool, most urgent first
}

// Candidate is a task of the pool with what a day plan request says of it.
type Candidate struct {
	planner.Candidate
	Remaining int          // minutes
	OpenSteps []model.Task // in order
	Goal      *model.Goal
	Pace      string // the goal's pace while it is active, else ""
	Skipped   bool   // it has a skipped item on the day
}

func (r planRepo) DayPlan(ctx context.Context, day string, env PlanEnv) (DayPlanContext, Changes, error) {
	d, err := r.plannable(day, env)
	if err != nil {
		return DayPlanContext{}, Changes{}, err
	}
	var (
		c  DayPlanContext
		ch Changes
	)
	err = r.db.InTx(ctx, func(tx *sql.Tx) error {
		if d == env.today() {
			if err := r.prepareToday(ctx, tx, env, &ch); err != nil {
				return err
			}
		} else if err := r.materialize(ctx, tx, d, env, &ch); err != nil {
			return err
		}
		in, onDay, err := r.generateInput(ctx, tx, d, env)
		if err != nil {
			return err
		}
		if c.Plan, err = r.read(ctx, tx, d, env); err != nil {
			return err
		}
		capIn, _, err := capacityInput(ctx, tx, d, env, nil)
		if err != nil {
			return err
		}
		for _, cm := range capIn.Commitments {
			if planner.OnDay(cm, d) {
				c.Commitments = append(c.Commitments, cm)
			}
		}
		window := planner.Interval{Start: d.At(capIn.DayStart, env.Loc), End: d.At(capIn.DayEnd, env.Loc)}
		for _, b := range c.Plan.Events {
			if b.Start.Before(window.End) && window.Start.Before(b.End) {
				c.Busy = append(c.Busy, b)
			}
		}
		skipped := map[string]bool{}
		for _, it := range onDay {
			switch {
			case it.Status == model.PlanDone && it.StartAt != nil:
				capIn.Busy = append(capIn.Busy, planner.Interval{Start: *it.StartAt, End: it.StartAt.Add(it.Planned)})
			case it.Status == model.PlanSkipped:
				skipped[it.TaskID] = true
			}
		}
		capIn.DayStart = min(env.DayStart, capIn.DayStart)
		c.Free = planner.Capacity(capIn).Free

		cands := planner.Candidates(in, nil)
		steps, err := stepsOf(ctx, tx, ids(cands, func(c planner.Candidate) string { return c.Task.ID })...)
		if err != nil {
			return err
		}
		open := map[string][]model.Task{}
		for _, s := range steps {
			if s.Status == model.TaskOpen {
				open[*s.ParentID] = append(open[*s.ParentID], s)
			}
		}
		for _, pc := range cands {
			t := pc.Task
			cand := Candidate{Candidate: pc, Remaining: planner.RemainingMinutes(t, in.Tracked[t.ID]),
				OpenSteps: open[t.ID], Skipped: skipped[t.ID]}
			if t.GoalID != nil {
				if g, ok := in.Goals[*t.GoalID]; ok {
					cand.Goal = &g
					if p, ok := in.Progress[g.ID]; ok && g.Status == model.GoalActive {
						cand.Pace = p.Pace
					}
				}
			}
			c.Candidates = append(c.Candidates, cand)
		}
		return nil
	})
	if err != nil {
		return DayPlanContext{}, Changes{}, fmt.Errorf("day plan for %s: %w", d, err)
	}
	return c, ch, nil
}

// DayPlanOutput is an ok day plan run's output, as it is stored.
type DayPlanOutput struct {
	Messages []json.RawMessage `json:"messages"`
	Items    []ProposedBlock   `json:"items"`
	Hours    *struct {
		StartMinute *int `json:"start_minute"`
		WorkMinutes *int `json:"work_minutes"`
	} `json:"hours"`
}

// ProposedBlock is a block of a day plan run's output.
type ProposedBlock struct {
	TaskID         string `json:"task_id"`
	StartAt        int64  `json:"start_at"`
	PlannedMinutes int    `json:"planned_minutes"`
}

// acceptDayPlan writes the blocks at indexes of a day plan run in tx
// (docs/07-integrations.md#day-plan) and returns the tasks it planned.
func acceptDayPlan(ctx context.Context, tx *sql.Tx, db *DB, run model.LLMRun, indexes []int, today string) ([]model.Task, Changes, error) {
	var ch Changes
	day := run.SubjectID
	if day < today {
		return nil, ch, FailField(ErrInvalid, "day", "%s has passed, so its plan cannot change", day)
	}
	var out DayPlanOutput
	if err := json.Unmarshal(run.Output, &out); err != nil {
		return nil, ch, fmt.Errorf("read run output: %w", err)
	}
	seen := map[int]bool{}
	for _, i := range indexes {
		if i < 0 || i >= len(out.Items) || seen[i] {
			return nil, ch, FailField(ErrInvalid, "indexes", "indexes must be distinct and from 0 to %d", len(out.Items)-1)
		}
		seen[i] = true
	}
	if h := out.Hours; h != nil && (h.StartMinute != nil || h.WorkMinutes != nil) {
		cur, err := dayHoursOn(ctx, tx, day)
		if err != nil {
			return nil, ch, err
		}
		start, work := h.StartMinute, h.WorkMinutes
		if cur != nil && start == nil {
			start = cur.StartMinute
		}
		if cur != nil && work == nil && cur.Work != nil {
			m := int(*cur.Work / time.Minute)
			work = &m
		}
		if err := writeDayHours(ctx, tx, db, day, start, work); err != nil {
			return nil, ch, err
		}
	}
	const qDayItems = `SELECT ` + planItemCols + ` FROM plan_items WHERE day = ? AND deleted_at IS NULL`
	onDay, err := queryPlanItems(ctx, tx, qDayItems, day)
	if err != nil {
		return nil, ch, err
	}
	var items []model.PlanItem
	for _, it := range onDay {
		if it.Status == model.PlanPlanned {
			if err := softDelete(ctx, tx, db, "plan_items", it.ID); err != nil {
				return nil, ch, err
			}
			continue
		}
		items = append(items, it)
	}
	picked := slices.Sorted(slices.Values(indexes))
	tasks, err := tasksByID(ctx, tx, ids(picked, func(i int) string { return out.Items[i].TaskID }))
	if err != nil {
		return nil, ch, err
	}
	byID := map[string]model.Task{}
	for _, t := range tasks {
		if t.DeletedAt == nil && t.Status == model.TaskOpen {
			byID[t.ID] = t
		}
	}
	taskIDs := ids(tasks, func(t model.Task) string { return t.ID })
	latest, err := latestBefore(ctx, tx, civil.MustParse(day), taskIDs)
	if err != nil {
		return nil, ch, err
	}
	tracked, err := trackedTime(ctx, tx, taskIDs, db.Now())
	if err != nil {
		return nil, ch, err
	}
	// A task with an estimate gets no more time than it has work left: the
	// blocks past that, in start order, are cut short or left out.
	left := map[string]int{}
	for _, t := range byID {
		if t.Estimate != nil {
			left[t.ID] = planner.RemainingMinutes(planner.Task{Task: t}, tracked[t.ID])
		}
	}
	fresh := map[string]bool{} // items to insert rather than update
	var planned []model.Task
	for _, i := range picked {
		b := out.Items[i]
		t, ok := byID[b.TaskID]
		if !ok {
			continue // done or deleted since the proposal
		}
		minutes := b.PlannedMinutes
		if room, capped := left[t.ID]; capped {
			if minutes = min(minutes, room) / planner.BlockStep * planner.BlockStep; minutes < planner.BlockStep {
				continue
			}
			left[t.ID] = room - minutes
		}
		var prev *model.PlanItem
		if it, ok := latest[t.ID]; ok {
			prev = &it
		}
		start := time.UnixMilli(b.StartAt)
		it := model.PlanItem{ID: model.NewID(), Day: day, TaskID: t.ID, Planned: time.Duration(minutes) * time.Minute,
			StartAt: &start, Status: model.PlanPlanned, Pinned: true}
		it.RolloverCount, it.RolledFromID = planner.RolloverCount(prev)
		fresh[it.ID] = true
		items = append(items, it)
		if !slices.ContainsFunc(planned, func(p model.Task) bool { return p.ID == t.ID }) {
			planned = append(planned, t)
		}
	}
	slices.SortStableFunc(items, func(a, b model.PlanItem) int {
		switch {
		case a.StartAt != nil && b.StartAt != nil:
			if c := a.StartAt.Compare(*b.StartAt); c != 0 {
				return c
			}
		case a.StartAt != nil:
			return -1
		case b.StartAt != nil:
			return 1
		}
		return cmp.Or(cmp.Compare(a.Position, b.Position), cmp.Compare(a.ID, b.ID))
	})
	for pos := range items {
		it := &items[pos]
		switch {
		case fresh[it.ID]:
			it.Position = pos
			err = insertPlanItem(ctx, tx, db, it)
		case it.Position != pos:
			it.Position = pos
			err = writePlanItem(ctx, tx, db, it)
		}
		if err != nil {
			return nil, ch, err
		}
	}
	ch.plan(day)
	return planned, ch, nil
}
