package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/connect"
	"github.com/kzark/gwen/internal/harness"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// The approval round trip at API level (plan.md 3.7, 3.10): a routine run
// reaches a step that needs you, the approval shows up in the list and on the
// event stream, you decide it with the client, and the run goes on, with what
// you decided: an edited payload, a remembered grant, a refusal. Everything
// goes through the real client and routes, as the dashboard does; the same
// calls are what clienttest.Fake stands in for in the CLI tests.

type runsDraft struct {
	Text string `json:"text"`
}

type runsMail struct {
	To   string `json:"to"`
	Body string `json:"body"`
}

// runsOutbox is a connector with a write capability (draft.create) and an
// external one (mail.send); it keeps the inputs its calls received.
type runsOutbox struct {
	mu     sync.Mutex
	drafts []string
	sent   []string
}

func (*runsOutbox) ID() string   { return "outbox" }
func (*runsOutbox) Name() string { return "Test outbox" }
func (*runsOutbox) Status(context.Context) connect.Status {
	return connect.Status{State: connect.Connected}
}

func (o *runsOutbox) Capabilities() []connect.Capability {
	record := func(into *[]string) func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return func(_ context.Context, input json.RawMessage) (json.RawMessage, error) {
			o.mu.Lock()
			defer o.mu.Unlock()
			*into = append(*into, string(input))
			return json.RawMessage(`{"done":true}`), nil
		}
	}
	return []connect.Capability{
		{
			Name: "draft.create", Title: "Save a draft", Risk: connect.Write,
			Check: connect.Checked(func(in *runsDraft) error {
				if strings.TrimSpace(in.Text) == "" {
					return errors.New("a draft needs text")
				}
				return nil
			}),
			Summary: func(input json.RawMessage) string {
				var in runsDraft
				_ = json.Unmarshal(input, &in)
				return "Save a draft: " + in.Text
			},
			Invoke: record(&o.drafts),
		},
		{
			Name: "mail.send", Title: "Send an email", Risk: connect.External,
			Check: connect.Checked(func(in *runsMail) error {
				if !strings.Contains(in.To, "@") {
					return errors.New("give an email address")
				}
				return nil
			}),
			Summary: func(input json.RawMessage) string {
				var in runsMail
				_ = json.Unmarshal(input, &in)
				return "Send an email to " + in.To
			},
			Invoke: record(&o.sent),
		},
	}
}

func (o *runsOutbox) calls() (drafts, sent []string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.drafts...), append([]string(nil), o.sent...)
}

// runsServer is a Server with a run engine over the outbox, on a socket, and
// the events its client has received.
type runsServer struct {
	*Server
	c   *client.Client
	box *runsOutbox
	ctx context.Context

	mu     sync.Mutex
	events []wire.Event
}

const (
	standup    = "event:local/standup"
	notesInput = `{"text":"Standup notes"}`
	mailInput  = `{"to":"team@example.com","body":"Notes attached"}`
)

func newRunsServer(t *testing.T) *runsServer {
	t.Helper()
	clk := clock.NewFake(testutil.T0.Add(-time.Hour))
	db := testutil.NewDBWithClock(t, clk)
	s := &Server{DB: db, Repos: store.NewRepos(db), Tracker: &fakeTracker{cfg: config.Defaults()}, Hub: NewHub(clk), Clock: clk, Loc: time.UTC}
	box := &runsOutbox{}
	reg := connect.NewRegistry(box)
	s.Harness = harness.New(harness.Options{Runs: s.Repos.Runs, Registry: reg, Clock: clk, Publish: s.Hub.Publish, Scopes: s.SubjectChain},
		harness.Routine(reg, s.RoutineOf))
	sock := testutil.SocketPath(t)
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		s.Hub.Close()
		_ = srv.Close()
	})
	t.Cleanup(s.Harness.Close) // before the server and the store go
	rs := &runsServer{Server: s, c: client.New(sock), box: box, ctx: context.Background()}

	stream, err := rs.c.Events(rs.ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = stream.Close() })
	go func() {
		for {
			ev, err := stream.Next()
			if err != nil {
				return
			}
			rs.mu.Lock()
			rs.events = append(rs.events, ev)
			rs.mu.Unlock()
		}
	}()
	return rs
}

// routine saves the standup routine: save a draft, then send it.
func (rs *runsServer) routine(t *testing.T) {
	t.Helper()
	got, err := rs.c.PutRoutine(rs.ctx, standup, wire.PutRoutineRequest{Steps: []wire.RoutineStep{
		{Capability: "draft.create", Input: json.RawMessage(notesInput)},
		{Capability: "mail.send", Input: json.RawMessage(mailInput)},
	}})
	require.NoError(t, err)
	require.Len(t, got.Steps, 2)
}

func (rs *runsServer) start(t *testing.T) wire.Run {
	t.Helper()
	run, err := rs.c.CreateRun(rs.ctx, wire.CreateRunRequest{Kind: harness.KindRoutine, Subject: standup, Title: "Standup", Start: true})
	require.NoError(t, err)
	require.Equal(t, store.RunRunning, run.Status)
	return *run
}

// pending waits until exactly n approvals wait for you and returns them.
func (rs *runsServer) pending(t *testing.T, n int) []wire.Approval {
	t.Helper()
	var list *wire.ApprovalList
	require.Eventually(t, func() bool {
		var err error
		list, err = rs.c.ListApprovals(rs.ctx, store.ApprovalPending)
		return err == nil && len(list.Approvals) == n
	}, 10*time.Second, 5*time.Millisecond, "waiting for %d pending approvals", n)
	return list.Approvals
}

// waitRun waits until a run has this status (and, when it ended, has written
// its last log line) and returns it with its steps, log, and approvals.
func (rs *runsServer) waitRun(t *testing.T, id, status string) *wire.RunDetail {
	t.Helper()
	var d *wire.RunDetail
	require.Eventually(t, func() bool {
		var err error
		d, err = rs.c.GetRun(rs.ctx, id)
		if err != nil || d.Run.Status != status {
			return false
		}
		if status == store.RunWaiting || status == store.RunRunning {
			return true
		}
		return len(d.Log) > 0 && d.Log[len(d.Log)-1].Kind == status && d.Log[len(d.Log)-1].StepID == ""
	}, 10*time.Second, 5*time.Millisecond, "waiting for the run to be %s", status)
	return d
}

// waitStep waits until a run's step is in this status.
func (rs *runsServer) waitStep(t *testing.T, id string, position int, status string) *wire.RunDetail {
	t.Helper()
	var d *wire.RunDetail
	require.Eventually(t, func() bool {
		var err error
		d, err = rs.c.GetRun(rs.ctx, id)
		return err == nil && len(d.Steps) > position && d.Steps[position].Status == status
	}, 10*time.Second, 5*time.Millisecond, "waiting for step %d to be %s", position, status)
	return d
}

// eventsNamed are the events of this name received so far, decoded into T.
func eventsNamed[T any](t *testing.T, rs *runsServer, name string) []T {
	t.Helper()
	rs.mu.Lock()
	defer rs.mu.Unlock()
	var out []T
	for _, ev := range rs.events {
		if ev.Name == name {
			var v T
			require.NoError(t, json.Unmarshal(ev.Data, &v))
			out = append(out, v)
		}
	}
	return out
}

func logKinds(d *wire.RunDetail) []string {
	var kinds []string
	for _, ev := range d.Log {
		kinds = append(kinds, ev.Kind)
	}
	return kinds
}

func decide(rs *runsServer, id, decision, remember string, payload string) (*wire.Approval, error) {
	req := wire.DecideApprovalRequest{Decision: decision, Remember: remember}
	if payload != "" {
		req.Payload = json.RawMessage(payload)
	}
	return rs.c.DecideApproval(rs.ctx, id, req)
}

// A write step asks, you edit it and say "remember" for the subject, and it
// goes on with your edit; the send that follows asks again, can't be
// remembered, and goes out only after you approve it.
func TestApprovalRoundTrip(t *testing.T) {
	rs := newRunsServer(t)
	rs.routine(t)
	run := rs.start(t)

	// The step waits, and the approval says exactly what it will do.
	approvals := rs.pending(t, 1)
	draft := approvals[0]
	require.Equal(t, run.ID, draft.RunID)
	require.Equal(t, "Standup", draft.RunTitle)
	require.Equal(t, "draft.create", draft.Capability)
	require.Equal(t, string(connect.Write), draft.Risk)
	require.Equal(t, "Save a draft: Standup notes", draft.Summary)
	require.JSONEq(t, notesInput, string(draft.Payload))
	require.Equal(t, []string{standup}, draft.GrantScopes, "a write can be remembered for its subject")
	require.Equal(t, store.ApprovalPending, draft.Status)
	require.Nil(t, draft.DecidedAt)
	require.Equal(t, draft.CreatedAt+harness.ApprovalTTL.Milliseconds(), draft.ExpiresAt, "a pending approval lapses after a day")

	d := rs.waitRun(t, run.ID, store.RunWaiting)
	require.Equal(t, harness.ReasonApproval, d.Run.Reason)
	require.Equal(t, store.StepWaiting, rs.waitStep(t, run.ID, 0, store.StepWaiting).Steps[0].Status)
	require.Len(t, d.Approvals, 1)
	require.Equal(t, draft.ID, d.Approvals[0].ID)
	require.Contains(t, logKinds(d), "approval_requested")
	drafts, sent := rs.box.calls()
	require.Empty(t, drafts, "nothing is done before you decide")
	require.Empty(t, sent)
	requested := eventsNamed[wire.Approval](t, rs, wire.EventApprovalRequested)
	require.Len(t, requested, 1, "the dashboard hears about it")
	require.Equal(t, draft.ID, requested[0].ID)

	// Decisions that make no sense are refused, and the approval keeps waiting.
	for name, tt := range map[string]struct{ decision, remember, payload string }{
		"not a decision":             {"maybe", "", ""},
		"remember for another scope": {"approve", "event:local/other", ""},
		"remember with a rejection":  {"reject", standup, ""},
		"an edit the call refuses":   {"approve", "", `{"text":"  "}`},
		"an edit that doesn't fit":   {"approve", "", `{"words":"x"}`},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := decide(rs, draft.ID, tt.decision, tt.remember, tt.payload)
			apiCode(t, err, wire.CodeInvalidRequest)
		})
	}
	_, err := decide(rs, "no-such-approval", "approve", "", "")
	apiCode(t, err, wire.CodeNotFound)
	require.Len(t, rs.pending(t, 1), 1)
	require.Equal(t, store.RunWaiting, rs.waitRun(t, run.ID, store.RunWaiting).Run.Status)
	drafts, _ = rs.box.calls()
	require.Empty(t, drafts)
	grants, err := rs.c.ListGrants(rs.ctx)
	require.NoError(t, err)
	require.Empty(t, grants.Grants, "a refused decision remembers nothing")

	// Approve with an edit, remembered for the subject.
	decided, err := decide(rs, draft.ID, "approve", standup, `{"text":"Edited notes"}`)
	require.NoError(t, err)
	require.Equal(t, store.ApprovalApproved, decided.Status)
	require.NotNil(t, decided.DecidedAt)
	require.JSONEq(t, `{"text":"Edited notes"}`, string(decided.Payload))

	// The run goes on with your edit, and stops at the send.
	mail := rs.pending(t, 1)[0]
	require.NotEqual(t, draft.ID, mail.ID)
	drafts, sent = rs.box.calls()
	require.Len(t, drafts, 1)
	require.JSONEq(t, `{"text":"Edited notes"}`, drafts[0], "the step ran with what you edited")
	require.Empty(t, sent)
	require.Equal(t, "mail.send", mail.Capability)
	require.Equal(t, string(connect.External), mail.Risk)
	require.Equal(t, "Send an email to team@example.com", mail.Summary)
	require.JSONEq(t, mailInput, string(mail.Payload))
	require.Empty(t, mail.GrantScopes, "a send can't be remembered")
	grants, err = rs.c.ListGrants(rs.ctx)
	require.NoError(t, err)
	require.Len(t, grants.Grants, 1)
	require.Equal(t, wire.Grant{ID: grants.Grants[0].ID, Capability: "draft.create", Scope: standup, Decision: string(connect.Allow),
		CreatedAt: grants.Grants[0].CreatedAt}, grants.Grants[0])

	// Approving a send once, with "remember" or not, never makes a grant for it.
	for _, scope := range []string{standup, "mail.send", "anywhere"} {
		_, err = decide(rs, mail.ID, "approve", scope, "")
		apiCode(t, err, wire.CodeInvalidRequest)
	}
	rs.waitRun(t, run.ID, store.RunWaiting)
	_, sent = rs.box.calls()
	require.Empty(t, sent, "the send waits for you")

	_, err = decide(rs, mail.ID, "approve", "", "")
	require.NoError(t, err)
	d = rs.waitRun(t, run.ID, store.RunDone)
	require.Empty(t, d.Run.Reason)
	_, sent = rs.box.calls()
	require.Len(t, sent, 1)
	require.JSONEq(t, mailInput, sent[0])
	require.Len(t, d.Steps, 2)
	require.Equal(t, store.StepDone, d.Steps[0].Status)
	require.Equal(t, store.StepDone, d.Steps[1].Status)
	require.Equal(t, []string{
		"approval_requested", "approved", "call", "call_done", "step_done",
		"approval_requested", "approved", "call", "call_done", "step_done", "done",
	}, logKinds(d))
	grants, err = rs.c.ListGrants(rs.ctx)
	require.NoError(t, err)
	require.Len(t, grants.Grants, 1, "approving the send added no grant")

	// A decided approval can't be decided again, and nothing is pending.
	_, err = decide(rs, mail.ID, "reject", "", "")
	apiCode(t, err, wire.CodeConflict)
	_, err = decide(rs, draft.ID, "approve", "", "")
	apiCode(t, err, wire.CodeConflict)
	rs.pending(t, 0)
	for status, want := range map[string]int{store.ApprovalApproved: 2, store.ApprovalRejected: 0, store.ApprovalExpired: 0} {
		list, err := rs.c.ListApprovals(rs.ctx, status)
		require.NoError(t, err)
		require.Len(t, list.Approvals, want, status)
	}
	_, err = rs.c.ListApprovals(rs.ctx, "someday")
	apiCode(t, err, wire.CodeInvalidRequest)

	// The stream told the dashboard each step of it.
	require.Eventually(t, func() bool { return len(eventsNamed[wire.Approval](t, rs, wire.EventApprovalDecided)) == 2 },
		10*time.Second, 5*time.Millisecond)
	for _, a := range eventsNamed[wire.Approval](t, rs, wire.EventApprovalDecided) {
		require.Equal(t, store.ApprovalApproved, a.Status)
	}
	require.Len(t, eventsNamed[wire.Approval](t, rs, wire.EventApprovalRequested), 2)
	changed := eventsNamed[wire.RunChanged](t, rs, wire.EventRunChanged)
	require.NotEmpty(t, changed)
	require.Equal(t, wire.RunChanged{ID: run.ID, Kind: harness.KindRoutine, Status: store.RunDone}, changed[len(changed)-1])
}

// What you remembered lets the same step go on without asking, but never the
// send; a rejection skips the step and leaves the run done, with nothing
// sent; removing the grant makes the step ask again; and cancelling a run
// that waits ends its approval.
func TestApprovalsRememberRejectAndCancel(t *testing.T) {
	rs := newRunsServer(t)
	rs.routine(t)

	// First run: approve the draft for the subject, reject the send.
	first := rs.start(t)
	_, err := decide(rs, rs.pending(t, 1)[0].ID, "approve", standup, "")
	require.NoError(t, err)
	_, err = decide(rs, rs.pending(t, 1)[0].ID, "reject", "", "")
	require.NoError(t, err)
	d := rs.waitRun(t, first.ID, store.RunDone)
	require.Equal(t, store.StepDone, d.Steps[0].Status)
	require.Equal(t, store.StepSkipped, d.Steps[1].Status, "a step you said no to is skipped")
	require.Equal(t, harness.ErrRejected.Error(), d.Steps[1].Error)
	require.Contains(t, logKinds(d), "rejected")
	drafts, sent := rs.box.calls()
	require.Len(t, drafts, 1)
	require.Empty(t, sent, "a rejected send is never made")

	// Second run: the draft goes ahead on the grant; only the send asks.
	second := rs.start(t)
	mail := rs.pending(t, 1)[0]
	require.Equal(t, "mail.send", mail.Capability)
	require.Equal(t, second.ID, mail.RunID)
	d = rs.waitStep(t, second.ID, 1, store.StepWaiting)
	require.Equal(t, store.StepDone, d.Steps[0].Status, "the remembered draft didn't ask again")
	drafts, _ = rs.box.calls()
	require.Len(t, drafts, 2)
	_, err = decide(rs, mail.ID, "reject", "", "")
	require.NoError(t, err)
	rs.waitRun(t, second.ID, store.RunDone)
	_, sent = rs.box.calls()
	require.Empty(t, sent)
	rejected, err := rs.c.ListApprovals(rs.ctx, store.ApprovalRejected)
	require.NoError(t, err)
	require.Len(t, rejected.Approvals, 2)

	// Remove the grant: the draft asks again, and Cancel ends what waits.
	grants, err := rs.c.ListGrants(rs.ctx)
	require.NoError(t, err)
	require.Len(t, grants.Grants, 1)
	require.NoError(t, rs.c.DeleteGrant(rs.ctx, grants.Grants[0].ID))
	apiCode(t, rs.c.DeleteGrant(rs.ctx, grants.Grants[0].ID), wire.CodeNotFound)
	grants, err = rs.c.ListGrants(rs.ctx)
	require.NoError(t, err)
	require.Empty(t, grants.Grants)

	third := rs.start(t)
	require.Equal(t, "draft.create", rs.pending(t, 1)[0].Capability, "without the grant the draft asks again")
	cancelled, err := rs.c.CancelRun(rs.ctx, third.ID)
	require.NoError(t, err)
	require.Equal(t, store.RunCancelled, cancelled.Status)
	rs.pending(t, 0)
	expired, err := rs.c.ListApprovals(rs.ctx, store.ApprovalExpired)
	require.NoError(t, err)
	require.Len(t, expired.Approvals, 1, "cancelling the run ended its approval")
	_, err = decide(rs, expired.Approvals[0].ID, "approve", "", "")
	apiCode(t, err, wire.CodeConflict)
	drafts, sent = rs.box.calls()
	require.Len(t, drafts, 2, "the cancelled run made no draft")
	require.Empty(t, sent)
}
