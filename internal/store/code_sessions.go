package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Code session states (plan.md 3.14.5).
const (
	CodeStarting = "starting"
	CodeWorking  = "working"
	CodeNeedsYou = "needs_you"
	CodeDone     = "done"
	CodeIdle     = "idle"
	CodeError    = "error"
	CodeEnded    = "ended"
)

// Why a session needs you.
const (
	NeedPermission = "permission"
	NeedQuestion   = "question"
	NeedOther      = "other"
)

// CodeSession is a Claude Code session in a herdr-core terminal
// (00011_remote.sql).
type CodeSession struct {
	ID              string
	Terminal        string // the core's address, "" once the terminal is gone
	ClaudeSessionID string
	Cwd             string
	Sandbox         bool
	Args            []string // the flags Claude was started with, for Resume
	PID             int
	TranscriptPath  string
	State           string
	Need            string
	Step            string
	LastMessage     string
	PermissionMode  string
	// StartStatus is the output of `git status --porcelain -z` when the
	// session started, keyed "files": the paths already changed then.
	StartStatus map[string]bool
	// Touched are the files the session edited, in order, without repeats.
	Touched   []string
	Pinned    bool
	SeenAt    *time.Time
	StartedAt time.Time
	UpdatedAt time.Time
	EndedAt   *time.Time
}

// CodeSessionRepo keeps the code sessions.
type CodeSessionRepo interface {
	// Save inserts the session or replaces every column of it.
	Save(ctx context.Context, s CodeSession) error
	Get(ctx context.Context, id string) (CodeSession, error)
	// List returns the sessions that have not ended, and those that ended
	// at or after endedSince, newest first.
	List(ctx context.Context, endedSince time.Time) ([]CodeSession, error)
	// EndedWithClaude returns the ended sessions of one Claude conversation.
	EndedWithClaude(ctx context.Context, claudeSessionID string) ([]CodeSession, error)
	// LastOfClaude returns the newest session of a Claude conversation, or
	// ErrNotFound.
	LastOfClaude(ctx context.Context, claudeSessionID string) (CodeSession, error)
	Delete(ctx context.Context, id string) error
	// Prune deletes sessions that ended before cutoff.
	Prune(ctx context.Context, cutoff time.Time) (int64, error)
	// KeepCommitSummary stores Gwen's summary of a commit a Claude
	// conversation made, replacing an older one.
	KeepCommitSummary(ctx context.Context, claudeSessionID, commit, summary string, at time.Time) error
	// CommitSummaries returns the stored summaries of the commits these
	// conversations made, by CommitKey.
	CommitSummaries(ctx context.Context, claudeSessionIDs []string) (map[string]string, error)
}

// CommitKey is how CommitSummaries keys a conversation's commit.
func CommitKey(claudeSessionID, commit string) string { return claudeSessionID + " " + commit }

type codeSessionRepo struct{ db *DB }

// NewCodeSessionRepo returns the SQLite CodeSessionRepo.
func NewCodeSessionRepo(db *DB) CodeSessionRepo { return codeSessionRepo{db} }

const codeSessionCols = `id, terminal, claude_session_id, cwd, sandbox, args, pid, transcript_path, state, need, step,
	last_message, permission_mode, start_status, touched, pinned, seen_at, started_at, updated_at, ended_at`

func scanCodeSession(s scanner) (CodeSession, error) {
	var (
		c                     CodeSession
		args, status, touched string
		pid, seen, ended      sql.NullInt64
		started, updated      int64
	)
	if err := s.Scan(&c.ID, &c.Terminal, &c.ClaudeSessionID, &c.Cwd, &c.Sandbox, &args, &pid, &c.TranscriptPath, &c.State,
		&c.Need, &c.Step, &c.LastMessage, &c.PermissionMode, &status, &touched, &c.Pinned, &seen, &started, &updated,
		&ended); err != nil {
		return CodeSession{}, err
	}
	c.PID = int(pid.Int64)
	c.SeenAt, c.EndedAt = timePtr(seen), timePtr(ended)
	c.StartedAt, c.UpdatedAt = timeOf(started), timeOf(updated)
	c.Args, c.Touched, c.StartStatus = []string{}, []string{}, map[string]bool{}
	_ = json.Unmarshal([]byte(args), &c.Args)
	_ = json.Unmarshal([]byte(touched), &c.Touched)
	_ = json.Unmarshal([]byte(status), &c.StartStatus)
	return c, nil
}

func (r codeSessionRepo) Save(ctx context.Context, s CodeSession) error {
	args, _ := json.Marshal(append([]string{}, s.Args...))
	touched, _ := json.Marshal(append([]string{}, s.Touched...))
	status := []byte("{}")
	if len(s.StartStatus) > 0 {
		status, _ = json.Marshal(s.StartStatus)
	}
	var pid sql.NullInt64
	if s.PID != 0 {
		pid = sql.NullInt64{Int64: int64(s.PID), Valid: true}
	}
	const qSaveCodeSession = `INSERT INTO code_sessions (` + codeSessionCols + `)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET terminal = excluded.terminal, claude_session_id = excluded.claude_session_id,
			cwd = excluded.cwd, sandbox = excluded.sandbox, args = excluded.args, pid = excluded.pid,
			transcript_path = excluded.transcript_path, state = excluded.state, need = excluded.need,
			step = excluded.step, last_message = excluded.last_message, permission_mode = excluded.permission_mode,
			start_status = excluded.start_status, touched = excluded.touched, pinned = excluded.pinned,
			seen_at = excluded.seen_at, started_at = excluded.started_at, updated_at = excluded.updated_at,
			ended_at = excluded.ended_at`
	_, err := r.db.sql.ExecContext(ctx, qSaveCodeSession, s.ID, s.Terminal, s.ClaudeSessionID, s.Cwd, s.Sandbox, string(args),
		pid, s.TranscriptPath, s.State, s.Need, s.Step, s.LastMessage, s.PermissionMode, string(status), string(touched),
		s.Pinned, nullMillis(s.SeenAt), s.StartedAt.UnixMilli(), s.UpdatedAt.UnixMilli(), nullMillis(s.EndedAt))
	if err != nil {
		return fmt.Errorf("save code session: %w", classify(err))
	}
	return nil
}

func (r codeSessionRepo) Get(ctx context.Context, id string) (CodeSession, error) {
	const qGetCodeSession = `SELECT ` + codeSessionCols + ` FROM code_sessions WHERE id = ?`
	s, err := scanCodeSession(r.db.sql.QueryRowContext(ctx, qGetCodeSession, id))
	if err != nil {
		return CodeSession{}, fmt.Errorf("get code session: %w", notFound(err, "session", id))
	}
	return s, nil
}

func (r codeSessionRepo) query(ctx context.Context, what, q string, args ...any) ([]CodeSession, error) {
	rows, err := r.db.sql.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	defer rows.Close()
	out := []CodeSession{}
	for rows.Next() {
		s, err := scanCodeSession(rows)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", what, err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r codeSessionRepo) List(ctx context.Context, endedSince time.Time) ([]CodeSession, error) {
	const qListCodeSessions = `SELECT ` + codeSessionCols + ` FROM code_sessions
		WHERE ended_at IS NULL OR ended_at >= ? ORDER BY updated_at DESC`
	return r.query(ctx, "list code sessions", qListCodeSessions, endedSince.UnixMilli())
}

func (r codeSessionRepo) EndedWithClaude(ctx context.Context, claudeSessionID string) ([]CodeSession, error) {
	const qEndedWithClaude = `SELECT ` + codeSessionCols + ` FROM code_sessions
		WHERE claude_session_id = ? AND ended_at IS NOT NULL`
	return r.query(ctx, "ended code sessions", qEndedWithClaude, claudeSessionID)
}

func (r codeSessionRepo) LastOfClaude(ctx context.Context, claudeSessionID string) (CodeSession, error) {
	const qLastOfClaude = `SELECT ` + codeSessionCols + ` FROM code_sessions
		WHERE claude_session_id = ? ORDER BY started_at DESC LIMIT 1`
	s, err := scanCodeSession(r.db.sql.QueryRowContext(ctx, qLastOfClaude, claudeSessionID))
	if err != nil {
		return CodeSession{}, fmt.Errorf("last code session: %w", notFound(err, "session of conversation", claudeSessionID))
	}
	return s, nil
}

func (r codeSessionRepo) Delete(ctx context.Context, id string) error {
	const qDeleteCodeSession = `DELETE FROM code_sessions WHERE id = ?`
	if _, err := r.db.sql.ExecContext(ctx, qDeleteCodeSession, id); err != nil {
		return fmt.Errorf("delete code session: %w", err)
	}
	return nil
}

func (r codeSessionRepo) Prune(ctx context.Context, cutoff time.Time) (int64, error) {
	const qPruneCodeSessions = `DELETE FROM code_sessions WHERE ended_at IS NOT NULL AND ended_at < ?`
	res, err := r.db.sql.ExecContext(ctx, qPruneCodeSessions, cutoff.UnixMilli())
	if err != nil {
		return 0, fmt.Errorf("prune code sessions: %w", err)
	}
	return res.RowsAffected()
}

func (r codeSessionRepo) KeepCommitSummary(ctx context.Context, claudeSessionID, commit, summary string, at time.Time) error {
	const qKeepCommitSummary = `INSERT INTO commit_summaries (claude_session_id, commit_short, summary, made_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (claude_session_id, commit_short) DO UPDATE SET summary = excluded.summary, made_at = excluded.made_at`
	if _, err := r.db.sql.ExecContext(ctx, qKeepCommitSummary, claudeSessionID, commit, summary, at.UnixMilli()); err != nil {
		return fmt.Errorf("keep commit summary: %w", classify(err))
	}
	return nil
}

func (r codeSessionRepo) CommitSummaries(ctx context.Context, claudeSessionIDs []string) (map[string]string, error) {
	out := map[string]string{}
	if len(claudeSessionIDs) == 0 {
		return out, nil
	}
	args := make([]any, len(claudeSessionIDs))
	for i, id := range claudeSessionIDs {
		args[i] = id
	}
	q := `SELECT claude_session_id, commit_short, summary FROM commit_summaries WHERE claude_session_id IN (?` +
		strings.Repeat(", ?", len(args)-1) + `)`
	rows, err := r.db.sql.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("commit summaries: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, commit, summary string
		if err := rows.Scan(&id, &commit, &summary); err != nil {
			return nil, fmt.Errorf("commit summaries: %w", err)
		}
		out[CommitKey(id, commit)] = summary
	}
	return out, rows.Err()
}
