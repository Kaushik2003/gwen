package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/model"
)

// runsVersion is the schema version that added the harness tables.
const runsVersion = 16

// Run statuses (plan.md 3.7).
const (
	RunDraft     = "draft"
	RunRunning   = "running"
	RunWaiting   = "waiting"
	RunDone      = "done"
	RunFailed    = "failed"
	RunCancelled = "cancelled"
)

// Step statuses.
const (
	StepPending = "pending"
	StepRunning = "running"
	StepWaiting = "waiting"
	StepDone    = "done"
	StepFailed  = "failed"
	StepSkipped = "skipped"
)

// Approval statuses.
const (
	ApprovalPending  = "pending"
	ApprovalApproved = "approved"
	ApprovalRejected = "rejected"
	ApprovalExpired  = "expired"
)

// Run is one piece of work Gwen does with you. It stays on this machine.
type Run struct {
	ID      string
	Kind    string
	Subject string
	Title   string
	Brief   string
	Status  string
	// Reason says why it waits or failed: queued, approval, interrupted,
	// your reply, or an error.
	Reason      string
	ExternalRef string
	Result      json.RawMessage
	CreatedAt   time.Time
	UpdatedAt   time.Time
	StartedAt   *time.Time
	EndedAt     *time.Time
}

// RunStep is one step of a run.
type RunStep struct {
	ID         string
	RunID      string
	Position   int
	Title      string
	Capability string
	Risk       string
	Status     string
	Input      json.RawMessage
	Output     json.RawMessage
	Error      string
	StartedAt  *time.Time
	EndedAt    *time.Time
}

// RunEvent is one line of a run's log.
type RunEvent struct {
	Seq    int64
	RunID  string
	StepID string
	Kind   string
	Data   json.RawMessage
	At     time.Time
}

// Approval is a step waiting for your decision, with exactly what it will
// do: a run's (RunID) or a chat's (ThreadID, W32), never both. Pattern
// narrows what a remembered grant covers; GrantScopes are the scopes
// "remember" may pick.
type Approval struct {
	ID          string
	RunID       string
	ThreadID    string
	StepID      string
	Capability  string
	Risk        string
	Pattern     string
	Summary     string
	Payload     json.RawMessage
	GrantScopes []string
	Status      string
	CreatedAt   time.Time
	DecidedAt   *time.Time
}

// Grant relaxes or tightens a capability, with a pattern or none, for one
// scope. Decision is allow or ask.
type Grant struct {
	ID         string
	Capability string
	Pattern    string
	Scope      string
	Decision   string
	CreatedAt  time.Time
	ExpiresAt  *time.Time
}

// RunRepo keeps runs, their steps, log, and approvals, and the grants. All
// of it is device-local.
type RunRepo interface {
	CreateRun(ctx context.Context, r Run) (Run, error)
	Run(ctx context.Context, id string) (Run, error)
	// Runs lists runs newest first, with these statuses (all when empty).
	Runs(ctx context.Context, statuses []string, limit int) ([]Run, error)
	// UpdateRun writes a run's title, status, reason, result, and times.
	UpdateRun(ctx context.Context, r Run) error
	// AddStep appends a step after the run's last one.
	AddStep(ctx context.Context, s RunStep) (RunStep, error)
	Steps(ctx context.Context, runID string) ([]RunStep, error)
	Step(ctx context.Context, id string) (RunStep, error)
	UpdateStep(ctx context.Context, s RunStep) error
	// AddEvent adds a line to a run's log. data must hold no secret.
	AddEvent(ctx context.Context, runID, stepID, kind string, data any) (RunEvent, error)
	// Events are the newest limit lines of a run's log, oldest first.
	Events(ctx context.Context, runID string, limit int) ([]RunEvent, error)
	// Interrupt is restart recovery: every running run, and every run that
	// waited for an approval, waits with the reason interrupted; their
	// running or waiting steps wait, and pending approvals expire. It returns
	// the runs it moved.
	Interrupt(ctx context.Context) ([]Run, error)

	CreateApproval(ctx context.Context, a Approval) (Approval, error)
	Approval(ctx context.Context, id string) (Approval, error)
	// Approvals lists approvals newest first, with this status (all when "").
	Approvals(ctx context.Context, status string, limit int) ([]Approval, error)
	// Decide moves a pending approval to approved, rejected, or expired,
	// with an edited payload when given; one that is not pending is a
	// conflict.
	Decide(ctx context.Context, id, status string, payload json.RawMessage) (Approval, error)
	// RunApproved reports whether a run has an approved write.
	RunApproved(ctx context.Context, runID string) (bool, error)

	Grants(ctx context.Context) ([]Grant, error)
	// Grant is one grant.
	Grant(ctx context.Context, id string) (Grant, error)
	// PutGrant sets the grant for a capability, pattern, and scope.
	PutGrant(ctx context.Context, g Grant) (Grant, error)
	DeleteGrant(ctx context.Context, id string) error
}

type runRepo struct{ db *DB }

// NewRunRepo returns the SQLite RunRepo.
func NewRunRepo(db *DB) RunRepo { return runRepo{db} }

const runCols = `id, kind, subject, title, brief, status, reason, external_ref, result, created_at, updated_at, started_at, ended_at`

func scanRun(s scanner) (Run, error) {
	var (
		r                Run
		result           string
		created, updated int64
		started, ended   sql.NullInt64
	)
	if err := s.Scan(&r.ID, &r.Kind, &r.Subject, &r.Title, &r.Brief, &r.Status, &r.Reason, &r.ExternalRef, &result,
		&created, &updated, &started, &ended); err != nil {
		return Run{}, err
	}
	r.Result = json.RawMessage(result)
	r.CreatedAt, r.UpdatedAt, r.StartedAt, r.EndedAt = timeOf(created), timeOf(updated), timePtr(started), timePtr(ended)
	return r, nil
}

func rawOr(v json.RawMessage) string {
	if len(v) == 0 || !json.Valid(v) {
		return "{}"
	}
	return string(v)
}

func (r runRepo) CreateRun(ctx context.Context, run Run) (Run, error) {
	now := r.db.Now()
	run.ID, run.CreatedAt, run.UpdatedAt = model.NewID(), now, now
	if run.Status == "" {
		run.Status = RunDraft
	}
	run.Title = strings.TrimSpace(run.Title)
	if run.Kind == "" || run.Title == "" || len([]rune(run.Title)) > 200 {
		return Run{}, FailField(ErrInvalid, "title", "a run needs a kind and a title of 1 to 200 characters")
	}
	if len(run.Brief) > 16000 {
		return Run{}, FailField(ErrInvalid, "brief", "a brief is at most 16,000 characters")
	}
	run.Result = json.RawMessage(rawOr(run.Result))
	const q = `INSERT INTO runs (` + runCols + `) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := r.db.sql.ExecContext(ctx, q, run.ID, run.Kind, run.Subject, run.Title, run.Brief, run.Status, run.Reason,
		run.ExternalRef, string(run.Result), now.UnixMilli(), now.UnixMilli(), nullMillis(run.StartedAt), nullMillis(run.EndedAt))
	if err != nil {
		return Run{}, fmt.Errorf("create run: %w", classify(err))
	}
	return run, nil
}

func (r runRepo) Run(ctx context.Context, id string) (Run, error) {
	run, err := scanRun(r.db.sql.QueryRowContext(ctx, `SELECT `+runCols+` FROM runs WHERE id = ?`, id))
	if err != nil {
		return Run{}, fmt.Errorf("get run: %w", notFound(err, "run", id))
	}
	return run, nil
}

func (r runRepo) Runs(ctx context.Context, statuses []string, limit int) ([]Run, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q, args := `SELECT `+runCols+` FROM runs`, []any{}
	if len(statuses) > 0 {
		q += ` WHERE status IN (?` + strings.Repeat(", ?", len(statuses)-1) + `)`
		args = anys(statuses)
	}
	q += ` ORDER BY created_at DESC, id LIMIT ?`
	rows, err := r.db.sql.QueryContext(ctx, q, append(args, limit)...)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list runs: %w", err)
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (r runRepo) UpdateRun(ctx context.Context, run Run) error {
	run.UpdatedAt = r.db.Now()
	const q = `UPDATE runs SET title = ?, status = ?, reason = ?, external_ref = ?, result = ?, updated_at = ?, started_at = ?,
		ended_at = ? WHERE id = ?`
	res, err := r.db.sql.ExecContext(ctx, q, run.Title, run.Status, run.Reason, run.ExternalRef, rawOr(run.Result),
		run.UpdatedAt.UnixMilli(), nullMillis(run.StartedAt), nullMillis(run.EndedAt), run.ID)
	if err != nil {
		return fmt.Errorf("update run: %w", classify(err))
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Fail(ErrNotFound, "no run %s", run.ID)
	}
	return nil
}

const stepCols = `id, run_id, position, title, capability, risk, status, input, output, error, started_at, ended_at`

func scanStep(s scanner) (RunStep, error) {
	var (
		st             RunStep
		in, out        string
		started, ended sql.NullInt64
	)
	if err := s.Scan(&st.ID, &st.RunID, &st.Position, &st.Title, &st.Capability, &st.Risk, &st.Status, &in, &out, &st.Error,
		&started, &ended); err != nil {
		return RunStep{}, err
	}
	st.Input, st.Output, st.StartedAt, st.EndedAt = json.RawMessage(in), json.RawMessage(out), timePtr(started), timePtr(ended)
	return st, nil
}

func (r runRepo) AddStep(ctx context.Context, st RunStep) (RunStep, error) {
	st.ID = model.NewID()
	if st.Status == "" {
		st.Status = StepPending
	}
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(position) + 1, 0) FROM run_steps WHERE run_id = ?`, st.RunID).
			Scan(&st.Position); err != nil {
			return err
		}
		const q = `INSERT INTO run_steps (` + stepCols + `) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
		_, err := tx.ExecContext(ctx, q, st.ID, st.RunID, st.Position, st.Title, st.Capability, st.Risk, st.Status,
			rawOr(st.Input), rawOr(st.Output), st.Error, nullMillis(st.StartedAt), nullMillis(st.EndedAt))
		return classify(err)
	})
	if err != nil {
		return RunStep{}, fmt.Errorf("add step: %w", err)
	}
	st.Input, st.Output = json.RawMessage(rawOr(st.Input)), json.RawMessage(rawOr(st.Output))
	return st, nil
}

func (r runRepo) Steps(ctx context.Context, runID string) ([]RunStep, error) {
	rows, err := r.db.sql.QueryContext(ctx, `SELECT `+stepCols+` FROM run_steps WHERE run_id = ? ORDER BY position`, runID)
	if err != nil {
		return nil, fmt.Errorf("list steps: %w", err)
	}
	defer rows.Close()
	out := []RunStep{}
	for rows.Next() {
		st, err := scanStep(rows)
		if err != nil {
			return nil, fmt.Errorf("list steps: %w", err)
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func (r runRepo) Step(ctx context.Context, id string) (RunStep, error) {
	st, err := scanStep(r.db.sql.QueryRowContext(ctx, `SELECT `+stepCols+` FROM run_steps WHERE id = ?`, id))
	if err != nil {
		return RunStep{}, fmt.Errorf("get step: %w", notFound(err, "step", id))
	}
	return st, nil
}

func (r runRepo) UpdateStep(ctx context.Context, st RunStep) error {
	const q = `UPDATE run_steps SET title = ?, status = ?, input = ?, output = ?, error = ?, started_at = ?, ended_at = ? WHERE id = ?`
	_, err := r.db.sql.ExecContext(ctx, q, st.Title, st.Status, rawOr(st.Input), rawOr(st.Output), st.Error,
		nullMillis(st.StartedAt), nullMillis(st.EndedAt), st.ID)
	if err != nil {
		return fmt.Errorf("update step: %w", classify(err))
	}
	return nil
}

func (r runRepo) AddEvent(ctx context.Context, runID, stepID, kind string, data any) (RunEvent, error) {
	raw, err := json.Marshal(data)
	if err != nil || data == nil {
		raw = []byte("{}")
	}
	ev := RunEvent{RunID: runID, StepID: stepID, Kind: kind, Data: raw, At: r.db.Now()}
	res, err := r.db.sql.ExecContext(ctx, `INSERT INTO run_events (run_id, step_id, kind, data, at) VALUES (?, ?, ?, ?, ?)`,
		runID, sql.NullString{String: stepID, Valid: stepID != ""}, kind, string(raw), ev.At.UnixMilli())
	if err != nil {
		return RunEvent{}, fmt.Errorf("add run event: %w", classify(err))
	}
	ev.Seq, _ = res.LastInsertId()
	return ev, nil
}

func (r runRepo) Events(ctx context.Context, runID string, limit int) ([]RunEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	const q = `SELECT seq, run_id, COALESCE(step_id, ''), kind, data, at FROM
		(SELECT * FROM run_events WHERE run_id = ? ORDER BY seq DESC LIMIT ?) ORDER BY seq`
	rows, err := r.db.sql.QueryContext(ctx, q, runID, limit)
	if err != nil {
		return nil, fmt.Errorf("list run events: %w", err)
	}
	defer rows.Close()
	out := []RunEvent{}
	for rows.Next() {
		var (
			ev   RunEvent
			data string
			at   int64
		)
		if err := rows.Scan(&ev.Seq, &ev.RunID, &ev.StepID, &ev.Kind, &data, &at); err != nil {
			return nil, fmt.Errorf("list run events: %w", err)
		}
		ev.Data, ev.At = json.RawMessage(data), timeOf(at)
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (r runRepo) Interrupt(ctx context.Context) ([]Run, error) {
	var moved []Run
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT `+runCols+` FROM runs WHERE status = ? OR (status = ? AND reason = 'approval')`,
			RunRunning, RunWaiting)
		if err != nil {
			return err
		}
		for rows.Next() {
			run, err := scanRun(rows)
			if err != nil {
				rows.Close()
				return err
			}
			moved = append(moved, run)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		now := r.db.Now().UnixMilli()
		for i := range moved {
			moved[i].Status, moved[i].Reason = RunWaiting, "interrupted"
			if _, err := tx.ExecContext(ctx, `UPDATE runs SET status = ?, reason = ?, updated_at = ? WHERE id = ?`,
				RunWaiting, "interrupted", now, moved[i].ID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE run_steps SET status = ? WHERE run_id = ? AND status IN (?, ?)`,
				StepWaiting, moved[i].ID, StepRunning, StepWaiting); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO run_events (run_id, kind, data, at) VALUES (?, 'interrupted', '{}', ?)`,
				moved[i].ID, now); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE approvals SET status = ?, decided_at = ? WHERE status = ?`, ApprovalExpired, now,
			ApprovalPending)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("interrupt runs: %w", err)
	}
	return moved, nil
}

const approvalCols = `id, COALESCE(run_id, ''), COALESCE(thread_id, ''), step_id, capability, risk, pattern, summary, payload,
	grant_scopes, status, created_at, decided_at`

func scanApproval(s scanner) (Approval, error) {
	var (
		a               Approval
		payload, scopes string
		created         int64
		decided         sql.NullInt64
	)
	if err := s.Scan(&a.ID, &a.RunID, &a.ThreadID, &a.StepID, &a.Capability, &a.Risk, &a.Pattern, &a.Summary, &payload, &scopes,
		&a.Status, &created, &decided); err != nil {
		return Approval{}, err
	}
	if err := json.Unmarshal([]byte(scopes), &a.GrantScopes); err != nil || a.GrantScopes == nil {
		a.GrantScopes = []string{}
	}
	a.Payload, a.CreatedAt, a.DecidedAt = json.RawMessage(payload), timeOf(created), timePtr(decided)
	return a, nil
}

func (r runRepo) CreateApproval(ctx context.Context, a Approval) (Approval, error) {
	if (a.RunID == "") == (a.ThreadID == "") {
		return Approval{}, Fail(ErrInvalid, "an approval belongs to a run or to a chat")
	}
	a.ID, a.Status, a.CreatedAt = model.NewID(), ApprovalPending, r.db.Now()
	a.Payload = json.RawMessage(rawOr(a.Payload))
	if a.GrantScopes == nil {
		a.GrantScopes = []string{}
	}
	scopes, _ := json.Marshal(a.GrantScopes)
	const q = `INSERT INTO approvals (id, run_id, thread_id, step_id, capability, risk, pattern, summary, payload, grant_scopes,
		status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := r.db.sql.ExecContext(ctx, q, a.ID, sql.NullString{String: a.RunID, Valid: a.RunID != ""},
		sql.NullString{String: a.ThreadID, Valid: a.ThreadID != ""}, a.StepID, a.Capability, a.Risk, a.Pattern, a.Summary,
		string(a.Payload), string(scopes), a.Status, a.CreatedAt.UnixMilli()); err != nil {
		return Approval{}, fmt.Errorf("create approval: %w", classify(err))
	}
	return a, nil
}

func (r runRepo) Approval(ctx context.Context, id string) (Approval, error) {
	a, err := scanApproval(r.db.sql.QueryRowContext(ctx, `SELECT `+approvalCols+` FROM approvals WHERE id = ?`, id))
	if err != nil {
		return Approval{}, fmt.Errorf("get approval: %w", notFound(err, "approval", id))
	}
	return a, nil
}

func (r runRepo) Approvals(ctx context.Context, status string, limit int) ([]Approval, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q, args := `SELECT `+approvalCols+` FROM approvals`, []any{}
	if status != "" {
		q, args = q+` WHERE status = ?`, append(args, status)
	}
	rows, err := r.db.sql.QueryContext(ctx, q+` ORDER BY created_at DESC, id LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, fmt.Errorf("list approvals: %w", err)
	}
	defer rows.Close()
	out := []Approval{}
	for rows.Next() {
		a, err := scanApproval(rows)
		if err != nil {
			return nil, fmt.Errorf("list approvals: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r runRepo) Decide(ctx context.Context, id, status string, payload json.RawMessage) (Approval, error) {
	var a Approval
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		a, err = scanApproval(tx.QueryRowContext(ctx, `SELECT `+approvalCols+` FROM approvals WHERE id = ?`, id))
		if err != nil {
			return notFound(err, "approval", id)
		}
		if a.Status != ApprovalPending {
			return Fail(ErrConflict, "that approval was already %s", a.Status)
		}
		now := r.db.Now()
		a.Status, a.DecidedAt = status, &now
		if len(payload) > 0 {
			a.Payload = payload
		}
		_, err = tx.ExecContext(ctx, `UPDATE approvals SET status = ?, payload = ?, decided_at = ? WHERE id = ?`, a.Status,
			rawOr(a.Payload), now.UnixMilli(), a.ID)
		return classify(err)
	})
	if err != nil {
		return Approval{}, fmt.Errorf("decide approval: %w", err)
	}
	return a, nil
}

func (r runRepo) RunApproved(ctx context.Context, runID string) (bool, error) {
	var n int
	err := r.db.sql.QueryRowContext(ctx, `SELECT count(*) FROM approvals WHERE run_id = ? AND status = ? AND risk = 'write'`,
		runID, ApprovalApproved).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("run approved: %w", err)
	}
	return n > 0, nil
}

const grantCols = `id, capability, pattern, scope, decision, created_at, expires_at`

func scanGrant(s scanner) (Grant, error) {
	var (
		g       Grant
		created int64
		expires sql.NullInt64
	)
	if err := s.Scan(&g.ID, &g.Capability, &g.Pattern, &g.Scope, &g.Decision, &created, &expires); err != nil {
		return Grant{}, err
	}
	g.CreatedAt, g.ExpiresAt = timeOf(created), timePtr(expires)
	return g, nil
}

func (r runRepo) Grants(ctx context.Context) ([]Grant, error) {
	rows, err := r.db.sql.QueryContext(ctx, `SELECT `+grantCols+` FROM grants WHERE expires_at IS NULL OR expires_at > ?
		ORDER BY capability, pattern, scope`, r.db.Now().UnixMilli())
	if err != nil {
		return nil, fmt.Errorf("list grants: %w", err)
	}
	defer rows.Close()
	out := []Grant{}
	for rows.Next() {
		g, err := scanGrant(rows)
		if err != nil {
			return nil, fmt.Errorf("list grants: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r runRepo) Grant(ctx context.Context, id string) (Grant, error) {
	g, err := scanGrant(r.db.sql.QueryRowContext(ctx, `SELECT `+grantCols+` FROM grants WHERE id = ?`, id))
	if err != nil {
		return Grant{}, fmt.Errorf("get grant: %w", notFound(err, "grant", id))
	}
	return g, nil
}

func (r runRepo) PutGrant(ctx context.Context, g Grant) (Grant, error) {
	if g.Capability == "" || g.Scope == "" || (g.Decision != "allow" && g.Decision != "ask") {
		return Grant{}, Fail(ErrInvalid, "a grant needs a capability, a scope, and allow or ask")
	}
	g.ID, g.CreatedAt = model.NewID(), r.db.Now()
	const q = `INSERT INTO grants (` + grantCols + `) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (capability, pattern, scope) DO UPDATE SET decision = excluded.decision, expires_at = excluded.expires_at
		RETURNING id, created_at`
	var created int64
	if err := r.db.sql.QueryRowContext(ctx, q, g.ID, g.Capability, g.Pattern, g.Scope, g.Decision, g.CreatedAt.UnixMilli(),
		nullMillis(g.ExpiresAt)).Scan(&g.ID, &created); err != nil {
		return Grant{}, fmt.Errorf("put grant: %w", classify(err))
	}
	g.CreatedAt = timeOf(created)
	return g, nil
}

func (r runRepo) DeleteGrant(ctx context.Context, id string) error {
	res, err := r.db.sql.ExecContext(ctx, `DELETE FROM grants WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete grant: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Fail(ErrNotFound, "no grant %s", id)
	}
	return nil
}
