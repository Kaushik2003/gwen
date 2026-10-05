package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/planner"
)

// Limits of a block placed by hand.
const (
	MinScheduled = 5   // minutes
	MaxScheduled = 720 // minutes
)

func (r planRepo) Schedule(ctx context.Context, taskID, day string, start *time.Time, minutes int, env PlanEnv) (PlanEntry, Changes, error) {
	d, err := r.parseDay(day, env)
	if err != nil {
		return PlanEntry{}, Changes{}, err
	}
	today := env.today()
	if d.Before(today) {
		return PlanEntry{}, Changes{}, FailField(ErrInvalid, "day", "a day in the past cannot be planned")
	}
	if minutes < MinScheduled || minutes > MaxScheduled {
		return PlanEntry{}, Changes{}, FailField(ErrInvalid, "planned_minutes", "planned_minutes must be %d to %d",
			MinScheduled, MaxScheduled)
	}
	if start != nil {
		if start.Before(d.Midnight(env.Loc)) || !start.Before(d.AddDays(1).Midnight(env.Loc)) {
			return PlanEntry{}, Changes{}, FailField(ErrInvalid, "start_at", "start_at must be on %s", d)
		}
		at := start.Truncate(time.Minute)
		start = &at
	}
	var (
		e  PlanEntry
		ch Changes
	)
	err = r.db.InTx(ctx, func(tx *sql.Tx) error {
		t, err := getTask(ctx, tx, taskID)
		if err != nil {
			return err
		}
		switch {
		case t.Status != model.TaskOpen:
			return FailField(ErrInvalid, "task_id", "%q is done; reopen it to plan it", t.Title)
		case t.IsTemplate():
			return FailField(ErrInvalid, "task_id", "a recurring task is planned through its occurrences")
		}
		if d == today {
			if err := r.rollover(ctx, tx, today, &ch); err != nil {
				return err
			}
		}
		replanToday := d == today
		const qPlannedFrom = `SELECT ` + planItemCols + ` FROM plan_items WHERE task_id = ? AND status = 'planned'
			AND deleted_at IS NULL AND day >= ?`
		old, err := queryPlanItems(ctx, tx, qPlannedFrom, t.ID, today.String())
		if err != nil {
			return err
		}
		for _, it := range old {
			if err := softDelete(ctx, tx, r.db, "plan_items", it.ID); err != nil {
				return err
			}
			ch.plan(it.Day)
			replanToday = replanToday || it.Day == today.String()
		}
		// Putting a task on the calendar processes it (Getting Things Done).
		if t.Stage != model.StageTodo && t.Stage != model.StageDoing {
			t.Stage = model.StageTodo
			if err := writeTask(ctx, tx, r.db, &t); err != nil {
				return err
			}
			ch.task(t.ID)
		}
		var position int
		const qCountDay = `SELECT count(*) FROM plan_items WHERE day = ? AND deleted_at IS NULL`
		if err := tx.QueryRowContext(ctx, qCountDay, d.String()).Scan(&position); err != nil {
			return err
		}
		latest, err := latestBefore(ctx, tx, d, []string{t.ID})
		if err != nil {
			return err
		}
		var prev *model.PlanItem
		if it, ok := latest[t.ID]; ok {
			prev = &it
		}
		it := model.PlanItem{ID: model.NewID(), Day: d.String(), TaskID: t.ID, Planned: time.Duration(minutes) * time.Minute,
			StartAt: start, Position: position, Status: model.PlanPlanned, Pinned: true}
		it.RolloverCount, it.RolledFromID = planner.RolloverCount(prev)
		if err := insertPlanItem(ctx, tx, r.db, &it); err != nil {
			return err
		}
		ch.plan(d.String())
		if replanToday {
			if err := r.generate(ctx, tx, today, env, &ch); err != nil {
				return err
			}
		}
		const qGetPlanItem = `SELECT ` + planItemCols + ` FROM plan_items WHERE id = ?`
		if it, err = scanPlanItem(tx.QueryRowContext(ctx, qGetPlanItem, it.ID)); err != nil {
			return err
		}
		entries, err := withTasks(ctx, tx, []model.PlanItem{it})
		if err != nil || len(entries) == 0 {
			return fmt.Errorf("read the scheduled block: %w", err)
		}
		e = entries[0]
		return nil
	})
	if err != nil {
		return PlanEntry{}, Changes{}, fmt.Errorf("schedule task %s: %w", taskID, err)
	}
	return e, ch, nil
}

func (r planRepo) Unschedule(ctx context.Context, taskID, day string, env PlanEnv) (Changes, error) {
	today := env.today()
	if day != "" {
		d, err := r.parseDay(day, env)
		if err != nil {
			return Changes{}, err
		}
		if d.Before(today) {
			return Changes{}, FailField(ErrInvalid, "day", "a day in the past cannot change")
		}
	}
	var ch Changes
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := getTask(ctx, tx, taskID); err != nil {
			return err
		}
		q := `SELECT ` + planItemCols + ` FROM plan_items WHERE task_id = ? AND status = 'planned'
			AND deleted_at IS NULL AND day >= ?`
		args := []any{taskID, today.String()}
		if day != "" {
			q += ` AND day = ?`
			args = append(args, day)
		}
		items, err := queryPlanItems(ctx, tx, q, args...)
		if err != nil {
			return err
		}
		for i := range items {
			it := &items[i]
			ch.plan(it.Day)
			if it.Day == today.String() {
				// Skipped rather than removed, so regenerating leaves it off today.
				it.Status = model.PlanSkipped
				if err := writePlanItem(ctx, tx, r.db, it); err != nil {
					return err
				}
				continue
			}
			if err := softDelete(ctx, tx, r.db, "plan_items", it.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Changes{}, fmt.Errorf("unschedule task %s: %w", taskID, err)
	}
	return ch, nil
}

func (r planRepo) Refresh(ctx context.Context, env PlanEnv) (Changes, error) {
	var ch Changes
	today := env.today()
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		if err := r.rollover(ctx, tx, today, &ch); err != nil {
			return err
		}
		return r.generate(ctx, tx, today, env, &ch)
	})
	if err != nil {
		return Changes{}, fmt.Errorf("refresh today's plan: %w", err)
	}
	return ch, nil
}
