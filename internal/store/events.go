package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kzark/gwen/internal/model"
)

// EngineEventRepo keeps engine_events, the audit trail of state transitions.
type EngineEventRepo interface {
	// Insert records a transition inside the decision's transaction.
	Insert(ctx context.Context, tx *sql.Tx, e model.EngineEvent) error
	// Prune deletes rows older than before and reports how many.
	Prune(ctx context.Context, before time.Time) (int64, error)
	// List returns rows from since onwards, oldest first.
	List(ctx context.Context, since time.Time) ([]model.EngineEvent, error)
}

type engineEventRepo struct{ db *DB }

// NewEngineEventRepo returns the SQLite EngineEventRepo.
func NewEngineEventRepo(db *DB) EngineEventRepo { return engineEventRepo{db} }

func (r engineEventRepo) Insert(ctx context.Context, tx *sql.Tx, e model.EngineEvent) error {
	data := e.Data
	if data == nil {
		data = map[string]any{}
	}
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("record transition: %w", err)
	}
	const qInsertEngineEvent = `INSERT INTO engine_events (at, trigger, from_state, to_state, data) VALUES (?, ?, ?, ?, ?)`
	if _, err := tx.ExecContext(ctx, qInsertEngineEvent, e.At.UnixMilli(), e.Trigger, e.FromState, e.ToState, string(b)); err != nil {
		return fmt.Errorf("record transition: %w", classify(err))
	}
	return nil
}

func (r engineEventRepo) Prune(ctx context.Context, before time.Time) (int64, error) {
	const qPruneEngineEvents = `DELETE FROM engine_events WHERE at < ?`
	res, err := r.db.sql.ExecContext(ctx, qPruneEngineEvents, before.UnixMilli())
	if err != nil {
		return 0, fmt.Errorf("prune engine events: %w", err)
	}
	return res.RowsAffected()
}

func (r engineEventRepo) List(ctx context.Context, since time.Time) ([]model.EngineEvent, error) {
	const qListEngineEvents = `SELECT id, at, trigger, from_state, to_state, data FROM engine_events
		WHERE at >= ? ORDER BY at, id`
	rows, err := r.db.sql.QueryContext(ctx, qListEngineEvents, since.UnixMilli())
	if err != nil {
		return nil, fmt.Errorf("list engine events: %w", err)
	}
	defer rows.Close()
	var out []model.EngineEvent
	for rows.Next() {
		var (
			e    model.EngineEvent
			at   int64
			data string
		)
		if err := rows.Scan(&e.ID, &at, &e.Trigger, &e.FromState, &e.ToState, &data); err != nil {
			return nil, fmt.Errorf("list engine events: %w", err)
		}
		e.At = timeOf(at)
		if err := json.Unmarshal([]byte(data), &e.Data); err != nil {
			return nil, fmt.Errorf("list engine events: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Repos bundles every repository over one database.
type Repos struct {
	Projects    ProjectRepo
	Tasks       TaskRepo
	WorkDays    WorkDayRepo
	Segments    SegmentRepo
	Stats       StatsRepo
	Events      EngineEventRepo
	Goals       GoalRepo
	Commitments CommitmentRepo
	Plans       PlanRepo
	Calendar    CalendarRepo
	LLMRuns     LLMRunRepo
	Sync        SyncRepo
}

// NewRepos returns the SQLite repositories over db.
func NewRepos(db *DB) Repos {
	return Repos{
		Projects:    NewProjectRepo(db),
		Tasks:       NewTaskRepo(db),
		WorkDays:    NewWorkDayRepo(db),
		Segments:    NewSegmentRepo(db),
		Stats:       NewStatsRepo(db),
		Events:      NewEngineEventRepo(db),
		Goals:       NewGoalRepo(db),
		Commitments: NewCommitmentRepo(db),
		Plans:       NewPlanRepo(db),
		Calendar:    NewCalendarRepo(db),
		LLMRuns:     NewLLMRunRepo(db),
		Sync:        NewSyncRepo(db),
	}
}
