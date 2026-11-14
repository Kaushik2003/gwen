// Package harness runs the work Gwen does with you (plan.md 3.7): a run of
// some kind goes through steps; each step reaches the world only through
// Exec, where the permission policy is checked at the moment of the call and
// an approval waits for your click. At most two runs execute at once; a
// restart leaves the running ones waiting, and nothing with a side effect is
// re-run until you press Resume.
package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/connect"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/subject"
	"github.com/kzark/gwen/internal/wire"
)

// Limits.
const (
	// MaxRunning is how many runs execute at once; more wait as queued drafts.
	MaxRunning = 2
	// ApprovalTTL is how long a pending approval waits before it expires.
	ApprovalTTL = 24 * time.Hour
)

// Reasons a run waits or stays a draft.
const (
	ReasonQueued      = "queued"
	ReasonApproval    = "approval"
	ReasonInterrupted = "interrupted"
	ReasonReply       = "reply"
)

// StepSpec is a step a kind asks for.
type StepSpec struct {
	Title      string
	Capability string
	Input      json.RawMessage
}

// StepResult is what a step did; Add are further steps it asks for, after
// the last one.
type StepResult struct {
	Output json.RawMessage
	Add    []StepSpec
}

// Kind is one kind of run: "routine", and later "email_review" and
// "code".
type Kind interface {
	Name() string
	// Begin returns the first steps.
	Begin(ctx context.Context, r store.Run) ([]StepSpec, error)
	// Step carries out one step.
	Step(ctx context.Context, r store.Run, s store.RunStep, x Exec) (StepResult, error)
}

// Messenger is a kind that takes your replies while it runs (W23's code
// runs discuss their plan).
type Messenger interface {
	Message(ctx context.Context, r store.Run, text string, x Exec) error
}

// Exec is the only way a kind touches the world.
type Exec interface {
	// Invoke calls a capability; the policy is checked here, and an
	// approval waits for you.
	Invoke(ctx context.Context, capability string, input json.RawMessage) (json.RawMessage, error)
	// Ask waits for your decision on something that is not one capability
	// call (W23's plan approval).
	Ask(ctx context.Context, a ApprovalRequest) (Decision, error)
	// Log adds a line to the run's log. data must hold no secret.
	Log(kind string, data any)
}

// ApprovalRequest is what an approval card shows.
type ApprovalRequest struct {
	Capability string
	Risk       connect.Risk
	Summary    string
	Payload    json.RawMessage
}

// Decision is your answer to an approval; Payload is set when you edited it.
type Decision struct {
	Approved bool
	Payload  json.RawMessage
}

// Errors a step or a call reports.
var (
	// ErrRejected: you said no to an approval.
	ErrRejected = errors.New("you said no")
	// ErrExpired: an approval was not decided in time.
	ErrExpired = errors.New("the approval expired")
	// ErrRefused: the policy never allows this call.
	ErrRefused = errors.New("not allowed")
)

// moves are the allowed run status changes (plan.md 3.7); every other is
// refused.
var moves = map[string][]string{
	store.RunDraft:   {store.RunRunning, store.RunCancelled},
	store.RunRunning: {store.RunWaiting, store.RunDone, store.RunFailed, store.RunCancelled},
	store.RunWaiting: {store.RunRunning, store.RunCancelled},
}

// CanMove reports whether a run may go from one status to another.
func CanMove(from, to string) bool { return slices.Contains(moves[from], to) }

// Options configure an Engine.
type Options struct {
	Runs     store.RunRepo
	Registry *connect.Registry
	Clock    clock.Clock
	Publish  func(name string, data any)
	// Scopes are the scopes a run's calls are about for grants: its
	// subject and that subject's parents. Nil uses the subject alone.
	Scopes func(ctx context.Context, subject string) []string
}

// Engine runs runs.
type Engine struct {
	o         Options
	kinds     map[string]Kind
	approvals *Approvals

	mu     sync.Mutex
	active map[string]context.CancelFunc
	done   map[string]chan struct{}
	closed bool
	wg     sync.WaitGroup
}

// New builds an engine with these kinds.
func New(o Options, kinds ...Kind) *Engine {
	if o.Publish == nil {
		o.Publish = func(string, any) {}
	}
	e := &Engine{o: o, kinds: map[string]Kind{}, active: map[string]context.CancelFunc{}, done: map[string]chan struct{}{},
		approvals: NewApprovals(o.Runs, o.Clock, o.Publish, o.Registry)}
	for _, k := range kinds {
		e.kinds[k.Name()] = k
	}
	return e
}

// Kinds are the names of the kinds this engine runs.
func (e *Engine) Kinds() []string {
	out := make([]string, 0, len(e.kinds))
	for k := range e.kinds {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// Registry is the engine's connectors.
func (e *Engine) Registry() *connect.Registry { return e.o.Registry }

// Approvals is the approvals service runs share with Gwen's agent (W32).
func (e *Engine) Approvals() *Approvals { return e.approvals }

// Recover is restart recovery: every run that was running waits with the
// reason interrupted, for you to Resume or Cancel; the runs that were queued
// for a slot start.
func (e *Engine) Recover(ctx context.Context) error {
	moved, err := e.o.Runs.Interrupt(ctx)
	for _, r := range moved {
		e.changed(r)
	}
	if err == nil {
		// Runs queued for a slot when Gwen stopped start now that the interrupted ones hold none.
		e.next(ctx)
	}
	return err
}

// Close cancels every executing run and waits for them to stop; their runs
// are left as a restart would find them.
func (e *Engine) Close() {
	e.mu.Lock()
	e.closed = true
	for _, cancel := range e.active {
		cancel()
	}
	e.mu.Unlock()
	e.wg.Wait()
}

func (e *Engine) isClosed() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.closed
}

// Create makes a draft run.
func (e *Engine) Create(ctx context.Context, kind, subj, title, brief, externalRef string) (store.Run, error) {
	if _, ok := e.kinds[kind]; !ok {
		return store.Run{}, store.FailField(store.ErrInvalid, "kind", "%q is not a kind of run; the kinds are %v", kind, e.Kinds())
	}
	if subj != "" {
		if _, err := subject.Parse(subj); err != nil {
			return store.Run{}, store.FailField(store.ErrInvalid, "subject", "%v", err)
		}
	}
	r, err := e.o.Runs.CreateRun(ctx, store.Run{Kind: kind, Subject: subj, Title: title, Brief: brief, ExternalRef: externalRef})
	if err != nil {
		return store.Run{}, err
	}
	e.changed(r)
	return r, nil
}

// CreateWith makes a draft run that starts with these steps, so its kind's
// Begin is not asked for any.
func (e *Engine) CreateWith(ctx context.Context, kind, subj, title, brief, externalRef string, steps []StepSpec) (store.Run, error) {
	r, err := e.Create(ctx, kind, subj, title, brief, externalRef)
	if err != nil {
		return store.Run{}, err
	}
	if err := e.add(ctx, r.ID, steps); err != nil {
		return store.Run{}, err
	}
	return r, nil
}

// Wait returns once a run that executes now stops executing (it ended, or
// it was left for a restart), or ctx ends; a run that does not execute
// returns at once.
func (e *Engine) Wait(ctx context.Context, id string) {
	e.mu.Lock()
	ch, ok := e.done[id]
	e.mu.Unlock()
	if !ok {
		return
	}
	select {
	case <-ch:
	case <-ctx.Done():
	}
}

// Start starts a draft run, or keeps it queued while MaxRunning run.
func (e *Engine) Start(ctx context.Context, id string) (store.Run, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	r, err := e.o.Runs.Run(ctx, id)
	if err != nil {
		return store.Run{}, err
	}
	if r.Status != store.RunDraft {
		return store.Run{}, store.Fail(store.ErrConflict, "the run is %s, not a draft", r.Status)
	}
	return e.launch(ctx, r)
}

// launch starts r if there is room, else marks it queued. e.mu is held.
func (e *Engine) launch(ctx context.Context, r store.Run) (store.Run, error) {
	if e.closed {
		return store.Run{}, store.Fail(store.ErrConflict, "Gwen is shutting down")
	}
	running, err := e.o.Runs.Runs(ctx, []string{store.RunRunning}, MaxRunning+1)
	if err != nil {
		return store.Run{}, err
	}
	if len(running) >= MaxRunning {
		// A started draft, or a resumed waiting run, waits for a slot as queued; next starts it.
		if (r.Status == store.RunDraft || r.Status == store.RunWaiting) && r.Reason != ReasonQueued {
			r.Reason = ReasonQueued
			if err := e.o.Runs.UpdateRun(ctx, r); err != nil {
				return store.Run{}, err
			}
			e.changed(r)
		}
		return r, nil
	}
	if !CanMove(r.Status, store.RunRunning) {
		return store.Run{}, store.Fail(store.ErrConflict, "a %s run can't start", r.Status)
	}
	now := e.o.Clock.Now()
	r.Status, r.Reason = store.RunRunning, ""
	if r.StartedAt == nil {
		r.StartedAt = &now
	}
	if err := e.o.Runs.UpdateRun(ctx, r); err != nil {
		return store.Run{}, err
	}
	e.changed(r)
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	e.active[r.ID] = cancel
	e.done[r.ID] = make(chan struct{})
	e.wg.Add(1)
	go e.execute(runCtx, r)
	return r, nil
}

// Cancel stops a run: an executing one through its context, which sends a
// Claude Code process SIGINT; a draft or waiting one at once.
func (e *Engine) Cancel(ctx context.Context, id string) (store.Run, error) {
	e.mu.Lock()
	cancel, executing := e.active[id]
	e.mu.Unlock()
	if executing {
		_, _ = e.o.Runs.AddEvent(ctx, id, "", "cancel_asked", nil)
		cancel()
		// A step stops at its context; give it a moment, so the answer says cancelled.
		wait, stop := context.WithTimeout(ctx, 3*time.Second)
		e.Wait(wait, id)
		stop()
		return e.o.Runs.Run(ctx, id)
	}
	r, err := e.o.Runs.Run(ctx, id)
	if err != nil {
		return store.Run{}, err
	}
	if !CanMove(r.Status, store.RunCancelled) {
		return store.Run{}, store.Fail(store.ErrConflict, "the run already ended (%s)", r.Status)
	}
	if err := e.finish(ctx, &r, store.RunCancelled, ""); err != nil {
		return store.Run{}, err
	}
	e.approvals.expireRun(ctx, r.ID)
	return r, nil
}

// Resume continues a waiting run that was interrupted: the kind's waiting
// steps run again from the start, which you asked for by pressing Resume.
func (e *Engine) Resume(ctx context.Context, id string) (store.Run, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, executing := e.active[id]; executing {
		return store.Run{}, store.Fail(store.ErrConflict, "the run is already going")
	}
	r, err := e.o.Runs.Run(ctx, id)
	if err != nil {
		return store.Run{}, err
	}
	if r.Status != store.RunWaiting {
		return store.Run{}, store.Fail(store.ErrConflict, "only a waiting run can be resumed; this one is %s", r.Status)
	}
	steps, err := e.o.Runs.Steps(ctx, id)
	if err != nil {
		return store.Run{}, err
	}
	// A run already queued for a slot was resumed before: pressing Resume again changes nothing.
	if r.Reason != ReasonQueued {
		for _, s := range steps {
			if s.Status == store.StepWaiting || s.Status == store.StepRunning {
				s.Status = store.StepPending
				if err := e.o.Runs.UpdateStep(ctx, s); err != nil {
					return store.Run{}, err
				}
			}
		}
		_, _ = e.o.Runs.AddEvent(ctx, id, "", "resumed", nil)
	}
	return e.launch(ctx, r)
}

// Message hands your reply to a run whose kind takes replies.
func (e *Engine) Message(ctx context.Context, id, text string) (store.Run, error) {
	r, err := e.o.Runs.Run(ctx, id)
	if err != nil {
		return store.Run{}, err
	}
	m, ok := e.kinds[r.Kind].(Messenger)
	if !ok {
		return store.Run{}, store.Fail(store.ErrConflict, "a %s run doesn't take replies", r.Kind)
	}
	if r.Status != store.RunWaiting && r.Status != store.RunRunning {
		return store.Run{}, store.Fail(store.ErrConflict, "the run is %s", r.Status)
	}
	_, _ = e.o.Runs.AddEvent(ctx, id, "", "message", map[string]int{"chars": len([]rune(text))})
	if err := m.Message(ctx, r, text, &exec{e: e, run: r}); err != nil {
		return store.Run{}, err
	}
	return e.o.Runs.Run(ctx, id)
}

// Decide records your decision on a pending approval and hands it to the
// step or chat that waits for it; remember makes an allow grant for one of
// its grant scopes.
func (e *Engine) Decide(ctx context.Context, id string, approve bool, payload json.RawMessage, remember string) (store.Approval, error) {
	return e.approvals.Decide(ctx, id, approve, payload, remember)
}

// execute carries out a run's steps in order until none is left.
func (e *Engine) execute(ctx context.Context, r store.Run) {
	defer e.wg.Done()
	bg := context.WithoutCancel(ctx)
	status, reason := e.steps(ctx, r)
	if ctx.Err() != nil {
		status, reason = store.RunCancelled, ""
		e.mu.Lock()
		closed := e.closed
		e.mu.Unlock()
		if closed {
			// Shutting down: leave it running, for Recover to find on the next start.
			status = ""
		}
	}
	e.mu.Lock()
	delete(e.active, r.ID)
	done := e.done[r.ID]
	delete(e.done, r.ID)
	e.mu.Unlock()
	defer close(done)
	if status != "" {
		cur, err := e.o.Runs.Run(bg, r.ID)
		if err == nil {
			if err := e.finish(bg, &cur, status, reason); err != nil {
				slog.Error("finish run", "run", r.ID, "err", err)
			}
		}
		if status == store.RunCancelled {
			e.approvals.expireRun(bg, r.ID)
		}
	}
	e.next(bg)
}

// next starts queued runs, the oldest first, until there is no room: the
// drafts that were started and the waiting runs that were resumed while the
// engine was full.
func (e *Engine) next(ctx context.Context) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return
	}
	waiting, err := e.o.Runs.Runs(ctx, []string{store.RunDraft, store.RunWaiting}, 500)
	if err != nil {
		return
	}
	for i := len(waiting) - 1; i >= 0; i-- { // oldest first
		if waiting[i].Reason != ReasonQueued {
			continue
		}
		started, err := e.launch(ctx, waiting[i])
		if err != nil {
			slog.Warn("start queued run", "run", waiting[i].ID, "err", err)
			continue
		}
		if started.Status != store.RunRunning {
			return // full
		}
	}
}

// steps runs the run's pending steps and returns how it ended.
func (e *Engine) steps(ctx context.Context, r store.Run) (string, string) {
	kind := e.kinds[r.Kind]
	all, err := e.o.Runs.Steps(ctx, r.ID)
	if err != nil {
		return store.RunFailed, err.Error()
	}
	if len(all) == 0 {
		specs, err := kind.Begin(ctx, r)
		if err != nil {
			e.log(ctx, r.ID, "", "failed", map[string]string{"error": err.Error()})
			return store.RunFailed, err.Error()
		}
		if err := e.add(ctx, r.ID, specs); err != nil {
			return store.RunFailed, err.Error()
		}
	}
	for ctx.Err() == nil {
		all, err := e.o.Runs.Steps(ctx, r.ID)
		if err != nil {
			return store.RunFailed, err.Error()
		}
		i := slices.IndexFunc(all, func(s store.RunStep) bool { return s.Status == store.StepPending })
		if i < 0 {
			failed := 0
			for _, s := range all {
				if s.Status == store.StepFailed {
					failed++
				}
			}
			if failed > 0 {
				return store.RunFailed, fmt.Sprintf("%d of %d steps failed", failed, len(all))
			}
			return store.RunDone, ""
		}
		s := all[i]
		now := e.o.Clock.Now()
		s.Status, s.StartedAt, s.Error = store.StepRunning, &now, ""
		_ = e.o.Runs.UpdateStep(ctx, s)
		x := &exec{e: e, run: r, step: s.ID}
		res, err := kind.Step(ctx, r, s, x)
		end := e.o.Clock.Now()
		s.EndedAt = &end
		bg := context.WithoutCancel(ctx)
		switch {
		case ctx.Err() != nil && e.isClosed():
			// Gwen is stopping: the step waits for Resume after the restart, and runs again only then.
			s.Status = store.StepWaiting
		case ctx.Err() != nil:
			s.Status, s.Error = store.StepFailed, "cancelled"
		case errors.Is(err, ErrRejected) || errors.Is(err, ErrExpired):
			s.Status, s.Error = store.StepSkipped, err.Error()
		case err != nil:
			// A partial failure keeps what did happen, such as the links that opened.
			s.Status, s.Error, s.Output = store.StepFailed, err.Error(), res.Output
		default:
			s.Status, s.Output = store.StepDone, res.Output
		}
		_ = e.o.Runs.UpdateStep(bg, s)
		e.log(bg, r.ID, s.ID, "step_"+s.Status, map[string]string{"title": s.Title, "error": s.Error})
		e.changedID(bg, r.ID)
		if err == nil && len(res.Add) > 0 {
			if err := e.add(ctx, r.ID, res.Add); err != nil {
				return store.RunFailed, err.Error()
			}
		}
	}
	return store.RunCancelled, ""
}

// add appends steps, each with its capability's risk.
func (e *Engine) add(ctx context.Context, runID string, specs []StepSpec) error {
	for _, sp := range specs {
		risk := ""
		if sp.Capability != "" {
			if c, _, ok := e.o.Registry.Capability(sp.Capability); ok {
				risk = string(c.Risk)
			}
		}
		if _, err := e.o.Runs.AddStep(ctx, store.RunStep{RunID: runID, Title: sp.Title, Capability: sp.Capability, Risk: risk,
			Input: sp.Input}); err != nil {
			return err
		}
	}
	return nil
}

// finish ends a run.
func (e *Engine) finish(ctx context.Context, r *store.Run, status, reason string) error {
	if !CanMove(r.Status, status) {
		return store.Fail(store.ErrConflict, "a %s run can't become %s", r.Status, status)
	}
	now := e.o.Clock.Now()
	r.Status, r.Reason, r.EndedAt = status, reason, &now
	if err := e.o.Runs.UpdateRun(ctx, *r); err != nil {
		return err
	}
	e.log(ctx, r.ID, "", status, map[string]string{"reason": reason})
	e.changed(*r)
	return nil
}

// setStatus moves an executing run between running and waiting.
func (e *Engine) setStatus(ctx context.Context, runID, status, reason string) {
	r, err := e.o.Runs.Run(ctx, runID)
	if err != nil || r.Status == status || !CanMove(r.Status, status) {
		return
	}
	r.Status, r.Reason = status, reason
	if err := e.o.Runs.UpdateRun(ctx, r); err == nil {
		e.changed(r)
	}
}

func (e *Engine) log(ctx context.Context, runID, stepID, kind string, data any) {
	if _, err := e.o.Runs.AddEvent(ctx, runID, stepID, kind, data); err != nil {
		slog.Warn("run log", "run", runID, "err", err)
	}
}

func (e *Engine) changed(r store.Run) {
	e.o.Publish(wire.EventRunChanged, wire.RunChanged{ID: r.ID, Kind: r.Kind, Status: r.Status, Reason: r.Reason, ExternalRef: r.ExternalRef})
}

func (e *Engine) changedID(ctx context.Context, id string) {
	if r, err := e.o.Runs.Run(ctx, id); err == nil {
		e.changed(r)
	}
}

// exec is one step's Exec.
type exec struct {
	e    *Engine
	run  store.Run
	step string
}

func (x *exec) Log(kind string, data any) {
	x.e.log(context.Background(), x.run.ID, x.step, kind, data)
}

func (x *exec) scopes(ctx context.Context) []string {
	if x.run.Subject == "" {
		return nil
	}
	if x.e.o.Scopes != nil {
		return x.e.o.Scopes(ctx, x.run.Subject)
	}
	return []string{x.run.Subject}
}

func (x *exec) Invoke(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
	capab, _, ok := x.e.o.Registry.Capability(name)
	if !ok {
		return nil, fmt.Errorf("there is no capability %s", name)
	}
	input, err := capab.Check(input)
	if err != nil {
		return nil, err
	}
	scopes := x.scopes(ctx)
	if capab.Scope != nil {
		if s := capab.Scope(input); s != "" {
			scopes = append(scopes, s)
		}
	}
	grants, err := x.e.o.Runs.Grants(ctx)
	if err != nil {
		return nil, err
	}
	approved, err := x.e.o.Runs.RunApproved(ctx, x.run.ID)
	if err != nil {
		return nil, err
	}
	call := connect.Call{Capability: name, Risk: capab.Risk, Scopes: scopes, Approved: approved}
	for _, g := range grants {
		// A run's calls have no pattern: grants with one are for Gwen's agent (W32).
		if g.Pattern == "" {
			call.Grants = append(call.Grants, connect.Grant{Capability: g.Capability, Pattern: g.Pattern, Scope: g.Scope,
				Decision: connect.Decision(g.Decision)})
		}
	}
	summary := capab.Summary(input)
	switch connect.Decide(call) {
	case connect.Refuse:
		x.Log("refused", map[string]string{"capability": name})
		return nil, fmt.Errorf("%w: %s is not offered", ErrRefused, name)
	case connect.Ask:
		d, err := x.ask(ctx, Request{Capability: name, Risk: capab.Risk, Summary: summary, Payload: input, GrantScopes: offer(x.run.Subject, capab, input)})
		if err != nil {
			return nil, err
		}
		if !d.Approved {
			return nil, ErrRejected
		}
		if len(d.Payload) > 0 {
			input = d.Payload
			summary = capab.Summary(input)
		}
	}
	x.Log("call", map[string]string{"capability": name, "summary": summary})
	out, err := capab.Invoke(ctx, input)
	if err != nil {
		x.Log("call_failed", map[string]string{"capability": name, "error": err.Error()})
		if len(out) > 0 {
			return out, err
		}
		return nil, err
	}
	x.Log("call_done", map[string]string{"capability": name})
	return out, nil
}

func (x *exec) Ask(ctx context.Context, req ApprovalRequest) (Decision, error) {
	return x.ask(ctx, Request{Capability: req.Capability, Risk: req.Risk, Summary: req.Summary, Payload: req.Payload})
}

// ask waits for your decision on one of this run's steps: the step and the
// run wait meanwhile, and run again after.
func (x *exec) ask(ctx context.Context, req Request) (Decision, error) {
	e := x.e
	req.RunID, req.StepID = x.run.ID, x.step
	if st, err := e.o.Runs.Step(ctx, x.step); err == nil {
		st.Status = store.StepWaiting
		_ = e.o.Runs.UpdateStep(ctx, st)
	}
	e.setStatus(ctx, x.run.ID, store.RunWaiting, ReasonApproval)
	x.Log("approval_requested", map[string]string{"capability": req.Capability, "summary": req.Summary})
	d, err := e.approvals.Ask(ctx, req)
	if ctx.Err() != nil {
		return d, err
	}
	bg := context.WithoutCancel(ctx)
	if st, err := e.o.Runs.Step(bg, x.step); err == nil && st.Status == store.StepWaiting {
		st.Status = store.StepRunning
		_ = e.o.Runs.UpdateStep(bg, st)
	}
	e.setStatus(bg, x.run.ID, store.RunRunning, "")
	switch {
	case err != nil:
		x.Log("approval_expired", map[string]string{"capability": req.Capability})
	case d.Approved:
		x.Log("approved", map[string]string{"capability": req.Capability})
	default:
		x.Log("rejected", map[string]string{"capability": req.Capability})
	}
	return d, err
}

// offer are the scopes an approval of a run's call may be remembered for:
// its subject, and the folder the call works in.
func offer(subj string, c connect.Capability, input json.RawMessage) []string {
	if !connect.Grantable(c.Risk) {
		return nil
	}
	var out []string
	if subj != "" {
		out = append(out, subj)
	}
	if c.Scope != nil {
		if f := c.Scope(input); f != "" {
			out = append(out, f)
		}
	}
	return out
}
