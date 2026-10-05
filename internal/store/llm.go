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
)

// ProposedTask is a task of a breakdown run's output, as it is stored.
type ProposedTask struct {
	Title           string `json:"title"`
	Notes           string `json:"notes"`
	EstimateMinutes int    `json:"estimate_minutes"`
	DueDay          string `json:"due_day"`
	Priority        int    `json:"priority"`
	Quantity        *int   `json:"quantity"`
}

// LLMRunRepo keeps llm_runs and the reads a retro needs
// (docs/07-integrations.md#llm-adapter).
type LLMRunRepo interface {
	Create(ctx context.Context, kind, subjectID, status string, output any) (model.LLMRun, error)
	Get(ctx context.Context, id string) (model.LLMRun, error)
	// Accept applies the picks at indexes of an ok run in one transaction
	// and marks it accepted. A breakdown's tasks are created in index order,
	// linked to its goal and the goal's project; a quantity goal's tasks are
	// items: they get no due day, and its open sessions are filled from them.
	// A day plan's blocks replace the day's planned items, unless the day is
	// before today (docs/07-integrations.md#day-plan).
	Accept(ctx context.Context, id string, indexes []int, today string) ([]model.Task, Changes, error)
	// Reject marks an ok run rejected.
	Reject(ctx context.Context, id string) (model.LLMRun, error)
	// PlanMinutes sums plan items on days from..to: planned counts every item
	// not skipped, done the done ones.
	PlanMinutes(ctx context.Context, from, to string) (planned, done int, err error)
	// Completed lists the titles of live tasks done in [from, to).
	Completed(ctx context.Context, from, to time.Time) ([]string, error)
}

type llmRunRepo struct{ db *DB }

// NewLLMRunRepo returns the SQLite LLMRunRepo.
func NewLLMRunRepo(db *DB) LLMRunRepo { return llmRunRepo{db} }

const llmRunCols = `id, kind, subject_id, status, output, created_at, updated_at`

func scanLLMRun(s scanner) (model.LLMRun, error) {
	var r model.LLMRun
	var output string
	var created, updated int64
	if err := s.Scan(&r.ID, &r.Kind, &r.SubjectID, &r.Status, &output, &created, &updated); err != nil {
		return model.LLMRun{}, err
	}
	r.Output, r.CreatedAt, r.UpdatedAt = []byte(output), timeOf(created), timeOf(updated)
	return r, nil
}

func (r llmRunRepo) Create(ctx context.Context, kind, subjectID, status string, output any) (model.LLMRun, error) {
	b, err := json.Marshal(output)
	if err != nil {
		return model.LLMRun{}, fmt.Errorf("create llm run: %w", err)
	}
	now := r.db.Now()
	run := model.LLMRun{ID: model.NewID(), Kind: kind, SubjectID: subjectID, Status: status, Output: b,
		CreatedAt: now, UpdatedAt: now}
	const qInsertRun = `INSERT INTO llm_runs (id, kind, subject_id, status, output, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`
	if _, err := r.db.sql.ExecContext(ctx, qInsertRun, run.ID, kind, subjectID, status, string(b),
		now.UnixMilli(), now.UnixMilli()); err != nil {
		return model.LLMRun{}, fmt.Errorf("create llm run: %w", classify(err))
	}
	return run, nil
}

func getLLMRun(ctx context.Context, q Querier, id string) (model.LLMRun, error) {
	const qGetRun = `SELECT ` + llmRunCols + ` FROM llm_runs WHERE id = ?`
	run, err := scanLLMRun(q.QueryRowContext(ctx, qGetRun, id))
	if err != nil {
		return model.LLMRun{}, fmt.Errorf("get llm run: %w", notFound(err, "run", id))
	}
	return run, nil
}

func (r llmRunRepo) Get(ctx context.Context, id string) (model.LLMRun, error) {
	return getLLMRun(ctx, r.db.sql, id)
}

func setRunStatus(ctx context.Context, tx *sql.Tx, db *DB, run *model.LLMRun, status string) error {
	run.Status, run.UpdatedAt = status, db.Now()
	const qSetRunStatus = `UPDATE llm_runs SET status = ?, updated_at = ? WHERE id = ?`
	_, err := tx.ExecContext(ctx, qSetRunStatus, status, run.UpdatedAt.UnixMilli(), run.ID)
	return err
}

// openRun loads a run that can still be accepted or rejected.
func openRun(ctx context.Context, tx *sql.Tx, id string) (model.LLMRun, error) {
	run, err := getLLMRun(ctx, tx, id)
	if err != nil {
		return model.LLMRun{}, err
	}
	if run.Status != model.RunOK {
		return model.LLMRun{}, Fail(ErrConflict, "the run is %s, so it cannot change", run.Status)
	}
	return run, nil
}

func (r llmRunRepo) Accept(ctx context.Context, id string, indexes []int, today string) ([]model.Task, Changes, error) {
	var (
		created []model.Task
		ch      Changes
	)
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		run, err := openRun(ctx, tx, id)
		if err != nil {
			return err
		}
		switch run.Kind {
		case model.RunDayPlan:
			if created, ch, err = acceptDayPlan(ctx, tx, r.db, run, indexes, today); err != nil {
				return err
			}
			return setRunStatus(ctx, tx, r.db, &run, model.RunAccepted)
		case model.RunBreakdown:
		default:
			return Fail(ErrInvalid, "only a breakdown or a day plan can be accepted")
		}
		var out struct {
			Tasks []ProposedTask `json:"tasks"`
		}
		if err := json.Unmarshal(run.Output, &out); err != nil {
			return fmt.Errorf("read run output: %w", err)
		}
		if len(indexes) == 0 {
			return FailField(ErrInvalid, "indexes", "pick at least one task")
		}
		seen := map[int]bool{}
		for _, i := range indexes {
			if i < 0 || i >= len(out.Tasks) || seen[i] {
				return FailField(ErrInvalid, "indexes", "indexes must be distinct and from 0 to %d", len(out.Tasks)-1)
			}
			seen[i] = true
		}
		g, err := getGoal(ctx, tx, run.SubjectID)
		if err != nil {
			return err
		}
		items := g.Kind == model.GoalQuantity
		for _, i := range slices.Sorted(slices.Values(indexes)) {
			p := out.Tasks[i]
			now := r.db.Now()
			estimate := time.Duration(p.EstimateMinutes) * time.Minute
			t := model.Task{ID: model.NewID(), ProjectID: g.ProjectID, Title: p.Title, Notes: p.Notes,
				Status: model.TaskOpen, Priority: p.Priority, DueDay: &p.DueDay, Estimate: &estimate,
				GoalID: &g.ID, Quantity: p.Quantity,
				Envelope: model.Envelope{CreatedAt: now, UpdatedAt: now, DeviceID: r.db.DeviceID(), Rev: 1}}
			if items {
				t.DueDay = nil // sessions schedule items, in order
				if err := defaultUnit(ctx, tx, &t); err != nil {
					return err
				}
			}
			if err := validTask(ctx, tx, &t); err != nil {
				return err
			}
			if err := insertTask(ctx, tx, &t); err != nil {
				return err
			}
			created = append(created, t)
		}
		if items {
			var ch Changes
			if err := fillGoal(ctx, tx, r.db, g.ID, &ch); err != nil {
				return err
			}
			if created, err = tasksByID(ctx, tx, ids(created, func(t model.Task) string { return t.ID })); err != nil {
				return err
			}
			slices.SortFunc(created, func(a, b model.Task) int {
				return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), cmp.Compare(a.ID, b.ID))
			})
		}
		ch = Changes{TaskIDs: ids(created, func(t model.Task) string { return t.ID }), Goals: true}
		return setRunStatus(ctx, tx, r.db, &run, model.RunAccepted)
	})
	if err != nil {
		return nil, Changes{}, fmt.Errorf("accept run %s: %w", id, err)
	}
	return created, ch, nil
}

func (r llmRunRepo) Reject(ctx context.Context, id string) (model.LLMRun, error) {
	var run model.LLMRun
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		if run, err = openRun(ctx, tx, id); err != nil {
			return err
		}
		return setRunStatus(ctx, tx, r.db, &run, model.RunRejected)
	})
	if err != nil {
		return model.LLMRun{}, fmt.Errorf("reject run %s: %w", id, err)
	}
	return run, nil
}

func (r llmRunRepo) PlanMinutes(ctx context.Context, from, to string) (planned, done int, err error) {
	const qPlanMinutes = `SELECT
		COALESCE(SUM(CASE WHEN status <> 'skipped' THEN planned_minutes END), 0),
		COALESCE(SUM(CASE WHEN status = 'done' THEN planned_minutes END), 0)
		FROM plan_items WHERE deleted_at IS NULL AND day BETWEEN ? AND ?`
	if err := r.db.sql.QueryRowContext(ctx, qPlanMinutes, from, to).Scan(&planned, &done); err != nil {
		return 0, 0, fmt.Errorf("plan minutes: %w", err)
	}
	return planned, done, nil
}

func (r llmRunRepo) Completed(ctx context.Context, from, to time.Time) ([]string, error) {
	const qCompleted = `SELECT title FROM tasks WHERE deleted_at IS NULL AND status = 'done'
		AND done_at >= ? AND done_at < ? ORDER BY done_at, id`
	titles, err := queryIDs(ctx, r.db.sql, qCompleted, from.UnixMilli(), to.UnixMilli())
	if err != nil {
		return nil, fmt.Errorf("completed tasks: %w", err)
	}
	if titles == nil {
		titles = []string{}
	}
	return titles, nil
}
