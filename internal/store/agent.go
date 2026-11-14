package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/kzark/gwen/internal/model"
)

// agentVersion is the schema version that added Gwen's agent (00017_agent.sql).
const agentVersion = 17

// Agent chat states (plan.md 3.16).
const (
	AgentIdle     = "idle"
	AgentStarting = "starting"
	AgentThinking = "thinking"
	AgentWaiting  = "waiting"
	AgentError    = "error"
)

// Agent step statuses.
const (
	AgentStepRunning   = "running"
	AgentStepWaiting   = "waiting"
	AgentStepDone      = "done"
	AgentStepFailed    = "failed"
	AgentStepDenied    = "denied"
	AgentStepCancelled = "cancelled"
)

// The most a step keeps of its input and output (plan.md 3.16).
const (
	MaxStepInput  = 16 << 10
	MaxStepOutput = 2 << 10
)

// AgentChat is the Claude Code session behind a chat on this machine.
type AgentChat struct {
	ThreadID        string
	ClaudeSessionID string
	Cwd             string
	State           string
	Error           string
	Usage           json.RawMessage
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// AgentStep is one tool call of Gwen's agent, as the chat shows it.
type AgentStep struct {
	ID         string
	ThreadID   string
	ToolUseID  string
	Turn       int
	Position   int
	Tool       string
	Capability string
	Risk       string
	Title      string
	Input      json.RawMessage
	Output     string
	Status     string
	ApprovalID *string
	Undo       json.RawMessage
	StartedAt  time.Time
	EndedAt    *time.Time
}

// AgentRepo keeps the device-local half of Gwen's agent: the session behind
// each chat and the steps it took (00017_agent.sql).
type AgentRepo interface {
	// Chat returns a chat's session on this machine.
	Chat(ctx context.Context, threadID string) (AgentChat, error)
	// PutChat stores a chat's session, replacing what was there.
	PutChat(ctx context.Context, c AgentChat) error
	// SetState sets a chat's state and its error ("" for none).
	SetState(ctx context.Context, threadID, state, errText string) error
	// Steps returns a chat's steps in the order they were taken.
	Steps(ctx context.Context, threadID string) ([]AgentStep, error)
	// Step returns one step.
	Step(ctx context.Context, id string) (AgentStep, error)
	// StepOfToolUse returns the step of a chat's tool call, ErrNotFound when
	// none has its id yet.
	StepOfToolUse(ctx context.Context, threadID, toolUseID string) (AgentStep, error)
	// PutStep stores a step, replacing what was there; its input and output
	// are cut to what a step keeps.
	PutStep(ctx context.Context, st AgentStep) error
	// Interrupted makes every chat idle and cancels the steps left running or
	// waiting, when the daemon starts.
	Interrupted(ctx context.Context) error
	// Delete removes what this machine kept of a chat.
	Delete(ctx context.Context, threadID string) error
}

type agentRepo struct{ db *DB }

// NewAgentRepo returns the SQLite AgentRepo.
func NewAgentRepo(db *DB) AgentRepo { return agentRepo{db} }

const agentChatCols = `thread_id, claude_session_id, cwd, state, error, usage, created_at, updated_at`

func scanAgentChat(s scanner) (AgentChat, error) {
	var (
		c                AgentChat
		usage            string
		created, updated int64
	)
	if err := s.Scan(&c.ThreadID, &c.ClaudeSessionID, &c.Cwd, &c.State, &c.Error, &usage, &created, &updated); err != nil {
		return AgentChat{}, err
	}
	c.Usage, c.CreatedAt, c.UpdatedAt = json.RawMessage(usage), timeOf(created), timeOf(updated)
	return c, nil
}

func (r agentRepo) Chat(ctx context.Context, threadID string) (AgentChat, error) {
	c, err := scanAgentChat(r.db.sql.QueryRowContext(ctx, `SELECT `+agentChatCols+` FROM agent_chats WHERE thread_id = ?`, threadID))
	if err != nil {
		return AgentChat{}, fmt.Errorf("agent chat: %w", notFound(err, "chat", threadID))
	}
	return c, nil
}

func (r agentRepo) PutChat(ctx context.Context, c AgentChat) error {
	now := r.db.Now()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	usage := string(c.Usage)
	if usage == "" || !json.Valid(c.Usage) {
		usage = "{}"
	}
	const q = `INSERT INTO agent_chats (` + agentChatCols + `) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (thread_id) DO UPDATE SET claude_session_id = excluded.claude_session_id, cwd = excluded.cwd,
		state = excluded.state, error = excluded.error, usage = excluded.usage, updated_at = excluded.updated_at`
	_, err := r.db.sql.ExecContext(ctx, q, c.ThreadID, c.ClaudeSessionID, c.Cwd, c.State, c.Error, usage,
		c.CreatedAt.UnixMilli(), now.UnixMilli())
	if err != nil {
		return fmt.Errorf("put agent chat: %w", classify(err))
	}
	return nil
}

func (r agentRepo) SetState(ctx context.Context, threadID, state, errText string) error {
	const q = `UPDATE agent_chats SET state = ?, error = ?, updated_at = ? WHERE thread_id = ?`
	if _, err := r.db.sql.ExecContext(ctx, q, state, errText, r.db.Now().UnixMilli(), threadID); err != nil {
		return fmt.Errorf("agent chat state: %w", classify(err))
	}
	return nil
}

const agentStepCols = `id, thread_id, tool_use_id, turn, position, tool, capability, risk, title, input, output, status,
	approval_id, undo, started_at, ended_at`

func scanAgentStep(s scanner) (AgentStep, error) {
	var (
		st          AgentStep
		input, undo string
		approval    sql.NullString
		started     int64
		ended       sql.NullInt64
	)
	if err := s.Scan(&st.ID, &st.ThreadID, &st.ToolUseID, &st.Turn, &st.Position, &st.Tool, &st.Capability, &st.Risk,
		&st.Title, &input, &st.Output, &st.Status, &approval, &undo, &started, &ended); err != nil {
		return AgentStep{}, err
	}
	st.Input, st.Undo, st.ApprovalID = json.RawMessage(input), json.RawMessage(undo), stringPtr(approval)
	st.StartedAt, st.EndedAt = timeOf(started), timePtr(ended)
	return st, nil
}

func (r agentRepo) Steps(ctx context.Context, threadID string) ([]AgentStep, error) {
	rows, err := r.db.sql.QueryContext(ctx, `SELECT `+agentStepCols+` FROM agent_steps WHERE thread_id = ?
		ORDER BY turn, position, started_at`, threadID)
	if err != nil {
		return nil, fmt.Errorf("agent steps: %w", err)
	}
	defer rows.Close()
	out := []AgentStep{}
	for rows.Next() {
		st, err := scanAgentStep(rows)
		if err != nil {
			return nil, fmt.Errorf("agent steps: %w", err)
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func (r agentRepo) Step(ctx context.Context, id string) (AgentStep, error) {
	st, err := scanAgentStep(r.db.sql.QueryRowContext(ctx, `SELECT `+agentStepCols+` FROM agent_steps WHERE id = ?`, id))
	if err != nil {
		return AgentStep{}, fmt.Errorf("agent step: %w", notFound(err, "step", id))
	}
	return st, nil
}

func (r agentRepo) StepOfToolUse(ctx context.Context, threadID, toolUseID string) (AgentStep, error) {
	st, err := scanAgentStep(r.db.sql.QueryRowContext(ctx, `SELECT `+agentStepCols+` FROM agent_steps
		WHERE thread_id = ? AND tool_use_id = ? ORDER BY started_at DESC LIMIT 1`, threadID, toolUseID))
	if err != nil {
		return AgentStep{}, fmt.Errorf("agent step: %w", notFound(err, "step", toolUseID))
	}
	return st, nil
}

func (r agentRepo) PutStep(ctx context.Context, st AgentStep) error {
	if st.ID == "" {
		st.ID = model.NewID()
	}
	if st.StartedAt.IsZero() {
		st.StartedAt = r.db.Now()
	}
	input := string(st.Input)
	if input == "" || !json.Valid(st.Input) {
		input = "{}"
	}
	if len(input) > MaxStepInput {
		// Too big to keep whole: what it was, cut, as a string the chat can still show.
		cut, _ := json.Marshal(map[string]string{"cut": cutText(input, MaxStepInput-64)})
		input = string(cut)
	}
	undo := string(st.Undo)
	if undo == "" || !json.Valid(st.Undo) {
		undo = "{}"
	}
	const q = `INSERT INTO agent_steps (` + agentStepCols + `) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET tool_use_id = excluded.tool_use_id, turn = excluded.turn,
		position = excluded.position, tool = excluded.tool, capability = excluded.capability, risk = excluded.risk,
		title = excluded.title, input = excluded.input, output = excluded.output, status = excluded.status,
		approval_id = excluded.approval_id, undo = excluded.undo, ended_at = excluded.ended_at`
	_, err := r.db.sql.ExecContext(ctx, q, st.ID, st.ThreadID, st.ToolUseID, st.Turn, st.Position, st.Tool, st.Capability,
		st.Risk, st.Title, input, cutText(st.Output, MaxStepOutput), st.Status, st.ApprovalID, undo,
		st.StartedAt.UnixMilli(), nullMillis(st.EndedAt))
	if err != nil {
		return fmt.Errorf("put agent step: %w", classify(err))
	}
	return nil
}

func (r agentRepo) Interrupted(ctx context.Context) error {
	return r.db.InTx(ctx, func(tx *sql.Tx) error {
		now := r.db.Now().UnixMilli()
		if _, err := tx.ExecContext(ctx, `UPDATE agent_chats SET state = 'idle', updated_at = ? WHERE state <> 'idle'`, now); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE agent_steps SET status = 'cancelled', output = 'Gwen restarted', ended_at = ?
			WHERE status IN ('running', 'waiting')`, now)
		return err
	})
}

func (r agentRepo) Delete(ctx context.Context, threadID string) error {
	return r.db.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM agent_steps WHERE thread_id = ?`, threadID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM agent_chats WHERE thread_id = ?`, threadID)
		return err
	})
}

// cutText is s cut to at most n bytes on a character boundary.
func cutText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
