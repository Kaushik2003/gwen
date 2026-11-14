package harness

import (
	"context"
	"encoding/json"
	"slices"
	"sync"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/connect"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
)

// Request is one approval to ask for: a run's step (RunID) or a chat's
// (ThreadID, W32), never both. Pattern narrows what a remembered grant
// covers, such as a command's prefix; GrantScopes are the scopes you may
// pick to remember it for (never offered for external or destructive).
type Request struct {
	RunID       string
	ThreadID    string
	StepID      string
	Capability  string
	Risk        connect.Risk
	Pattern     string
	Summary     string
	Payload     json.RawMessage
	GrantScopes []string
}

// Approvals asks you, waits for your click, and keeps the grants. Runs reach
// it through Exec.Ask; Gwen's agent (W32) calls it directly with a chat.
type Approvals struct {
	runs    store.RunRepo
	clock   clock.Clock
	publish func(name string, data any)
	// check validates an edited payload for a capability it knows.
	check func(capability string, payload json.RawMessage) (json.RawMessage, bool, error)

	mu      sync.Mutex
	waiters map[string]chan Decision
}

// NewApprovals builds the approvals service.
func NewApprovals(runs store.RunRepo, clk clock.Clock, publish func(string, any), reg *connect.Registry) *Approvals {
	if publish == nil {
		publish = func(string, any) {}
	}
	a := &Approvals{runs: runs, clock: clk, publish: publish, waiters: map[string]chan Decision{}}
	a.check = func(capability string, payload json.RawMessage) (json.RawMessage, bool, error) {
		if reg == nil {
			return payload, false, nil
		}
		c, _, ok := reg.Capability(capability)
		if !ok {
			return payload, false, nil
		}
		out, err := c.Check(payload)
		return out, true, err
	}
	return a
}

// Ask stores a pending approval, sends approval_requested, and blocks until
// you decide, it expires after ApprovalTTL (ErrExpired), or ctx ends, which
// expires it too.
func (a *Approvals) Ask(ctx context.Context, req Request) (Decision, error) {
	if req.Risk == connect.External || req.Risk == connect.Destructive {
		req.GrantScopes = nil // nothing can remember these
	}
	row, err := a.runs.CreateApproval(ctx, store.Approval{RunID: req.RunID, ThreadID: req.ThreadID, StepID: req.StepID,
		Capability: req.Capability, Risk: string(req.Risk), Pattern: req.Pattern, Summary: req.Summary, Payload: req.Payload,
		GrantScopes: req.GrantScopes})
	if err != nil {
		return Decision{}, err
	}
	ch := make(chan Decision, 1)
	a.mu.Lock()
	a.waiters[row.ID] = ch
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.waiters, row.ID)
		a.mu.Unlock()
	}()
	a.event(ctx, wire.EventApprovalRequested, row)

	timer := a.clock.NewTimer(ApprovalTTL)
	defer timer.Stop()
	select {
	case d := <-ch:
		return d, nil
	case <-timer.C():
		a.expire(context.WithoutCancel(ctx), row.ID)
		return Decision{}, ErrExpired
	case <-ctx.Done():
		a.expire(context.WithoutCancel(ctx), row.ID)
		return Decision{}, ctx.Err()
	}
}

// Allowed reports whether grants let this call go ahead without asking:
// an allow grant for the capability, whose pattern equals pattern exactly
// and whose scope is one of scopes, and no ask grant that matches the same
// way (an ask always wins, whatever the order). Never consult it for an
// external or destructive call; those always ask.
func (a *Approvals) Allowed(ctx context.Context, capability, pattern string, scopes []string) (bool, error) {
	grants, err := a.runs.Grants(ctx)
	if err != nil {
		return false, err
	}
	allowed := false
	for _, g := range grants {
		if g.Capability != capability || g.Pattern != pattern || !slices.Contains(scopes, g.Scope) {
			continue
		}
		if g.Decision != string(connect.Allow) {
			return false, nil
		}
		allowed = true
	}
	return allowed, nil
}

// Decide records your decision and wakes the step that waits for it. An
// edited payload is checked by its capability; remember, with approve, makes
// an allow grant for one of the approval's grant scopes.
func (a *Approvals) Decide(ctx context.Context, id string, approve bool, payload json.RawMessage, remember string) (store.Approval, error) {
	row, err := a.runs.Approval(ctx, id)
	if err != nil {
		return store.Approval{}, err
	}
	if row.Status == store.ApprovalPending && a.clock.Now().After(row.CreatedAt.Add(ApprovalTTL)) {
		a.expire(ctx, id)
		return store.Approval{}, store.Fail(store.ErrConflict, "that approval expired")
	}
	if remember != "" {
		risk := connect.Risk(row.Risk)
		switch {
		case !approve:
			return store.Approval{}, store.FailField(store.ErrInvalid, "remember", "only an approval can be remembered")
		case !connect.Grantable(risk):
			return store.Approval{}, store.FailField(store.ErrInvalid, "remember", "a %s step can't be remembered; it asks every time", risk)
		case !slices.Contains(row.GrantScopes, remember):
			return store.Approval{}, store.FailField(store.ErrInvalid, "remember", "this approval can't be remembered for %q", remember)
		}
	}
	if len(payload) > 0 {
		if !json.Valid(payload) {
			return store.Approval{}, store.FailField(store.ErrInvalid, "payload", "the edited payload is not JSON")
		}
		checked, known, err := a.check(row.Capability, payload)
		if err != nil {
			return store.Approval{}, store.FailField(store.ErrInvalid, "payload", "%v", err)
		}
		if known {
			payload = checked
		}
	}
	status := store.ApprovalRejected
	if approve {
		status = store.ApprovalApproved
	}
	row, err = a.runs.Decide(ctx, id, status, payload)
	if err != nil {
		return store.Approval{}, err
	}
	if remember != "" {
		if _, err := a.runs.PutGrant(ctx, store.Grant{Capability: row.Capability, Pattern: row.Pattern, Scope: remember,
			Decision: string(connect.Allow)}); err != nil {
			return store.Approval{}, err
		}
	}
	a.event(ctx, wire.EventApprovalDecided, row)
	d := Decision{Approved: approve}
	if len(payload) > 0 {
		d.Payload = row.Payload
	}
	a.mu.Lock()
	ch, ok := a.waiters[id]
	delete(a.waiters, id)
	a.mu.Unlock()
	if ok {
		ch <- d
	}
	return row, nil
}

// expire lapses a pending approval and says so.
func (a *Approvals) expire(ctx context.Context, id string) {
	if row, err := a.runs.Decide(ctx, id, store.ApprovalExpired, nil); err == nil {
		a.event(ctx, wire.EventApprovalDecided, row)
	}
}

// expireRun lapses a run's pending approvals.
func (a *Approvals) expireRun(ctx context.Context, runID string) {
	pending, err := a.runs.Approvals(ctx, store.ApprovalPending, 500)
	if err != nil {
		return
	}
	for _, row := range pending {
		if row.RunID == runID {
			a.expire(ctx, row.ID)
		}
	}
}

func (a *Approvals) event(ctx context.Context, name string, row store.Approval) {
	title := ""
	if row.RunID != "" {
		if r, err := a.runs.Run(ctx, row.RunID); err == nil {
			title = r.Title
		}
	}
	a.publish(name, WireApproval(row, title))
}
