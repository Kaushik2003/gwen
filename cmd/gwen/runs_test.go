package main

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/client/clienttest"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// Approvals happen only by a click in the window (plan.md 3.7, section 7):
// `gwen run` lists what waits and what you remembered, and has no way to
// decide. The round trip here runs through clienttest.Fake with a small state
// behind it, standing in for the daemon: the CLI lists the approval, the
// window decides it, the CLI no longer lists it.

const (
	approvalDraft = "0a1b2c3d-0000-4000-8000-00000000d001"
	approvalMail  = "0a1b2c3d-0000-4000-8000-00000000d002"
)

var (
	draftApproval = wire.Approval{ID: approvalDraft, RunID: "r1", RunTitle: "Standup", Capability: "draft.create", Risk: "write",
		Summary: "Save a draft: Standup notes", Payload: json.RawMessage(`{"text":"Standup notes"}`), Status: "pending",
		GrantScopes: []string{"event:local/standup"}}
	mailApproval = wire.Approval{ID: approvalMail, RunID: "r1", RunTitle: "Standup", Capability: "mail.send", Risk: "external",
		Summary: "Send an email to team@example.com", Payload: json.RawMessage(`{"to":"team@example.com"}`), Status: "pending",
		GrantScopes: []string{}}
)

// approvalsDaemon is a fake daemon's approvals: ListApprovals and
// DecideApproval over one list, as the real routes behave.
func approvalsDaemon(approvals ...wire.Approval) *clienttest.Fake {
	var mu sync.Mutex
	return clienttest.New().
		On("ListApprovals", func(a ...any) (any, error) {
			mu.Lock()
			defer mu.Unlock()
			out := &wire.ApprovalList{Approvals: []wire.Approval{}}
			for _, ap := range approvals {
				if status := a[0].(string); status == "" || ap.Status == status {
					out.Approvals = append(out.Approvals, ap)
				}
			}
			return out, nil
		}).
		On("DecideApproval", func(a ...any) (any, error) {
			mu.Lock()
			defer mu.Unlock()
			req := a[1].(wire.DecideApprovalRequest)
			for i := range approvals {
				if approvals[i].ID == a[0].(string) && approvals[i].Status == "pending" {
					approvals[i].Status = map[string]string{"approve": "approved", "reject": "rejected"}[req.Decision]
					ap := approvals[i]
					return &ap, nil
				}
			}
			return nil, &client.APIError{Status: 409, Code: wire.CodeConflict, Message: "that approval was already decided", Details: map[string]any{}}
		})
}

func TestRunApprovalsListsWhatWaitsAndNeverDecides(t *testing.T) {
	f := approvalsDaemon(draftApproval, mailApproval)

	stdout, stderr, code := runCLI(t, f, "run", "approvals")
	require.Equal(t, exitOK, code, stderr)
	require.Contains(t, stdout, approvalDraft[:8]+"  Save a draft: Standup notes (write)")
	require.Contains(t, stdout, approvalMail[:8]+"  Send an email to team@example.com (external)")
	require.Contains(t, stdout, "Approve or reject them in Gwen › Settings › What Gwen can do.")
	require.Equal(t, [][]any{{"pending"}}, argsOf(f.CallsTo("ListApprovals")), "it asks for what is pending")

	// The window decides: the draft is approved, the send rejected.
	_, err := f.DecideApproval(context.Background(), approvalDraft, wire.DecideApprovalRequest{Decision: "approve", Remember: "event:local/standup"})
	require.NoError(t, err)
	_, err = f.DecideApproval(context.Background(), approvalMail, wire.DecideApprovalRequest{Decision: "reject"})
	require.NoError(t, err)
	f.Reset()

	stdout, stderr, code = runCLI(t, f, "run", "approvals")
	require.Equal(t, exitOK, code, stderr)
	require.Equal(t, "Nothing is waiting for you.\n", stdout)
	require.Empty(t, f.CallsTo("DecideApproval"), "listing never decides")

	// Decided approvals can't be decided again.
	_, err = f.DecideApproval(context.Background(), approvalDraft, wire.DecideApprovalRequest{Decision: "reject"})
	require.ErrorContains(t, err, "already decided")
}

func TestRunApprovalsJSONIsTheWireList(t *testing.T) {
	f := approvalsDaemon(draftApproval)
	stdout, stderr, code := runCLI(t, f, "--json", "run", "approvals")
	require.Equal(t, exitOK, code, stderr)
	var got wire.ApprovalList
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Equal(t, []wire.Approval{draftApproval}, got.Approvals)
}

// There is no command that approves or rejects, and the ones that exist never
// call DecideApproval, whatever they are given.
func TestRunHasNoWayToDecide(t *testing.T) {
	for _, args := range [][]string{
		{"run", "approve", approvalDraft},
		{"run", "reject", approvalDraft},
		{"run", "decide", approvalDraft, "approve"},
		{"run", "approvals", approvalDraft},
		{"run", "approvals", "--approve"},
		{"run", "grants", "--remember"},
	} {
		t.Run(args[1]+" "+args[len(args)-1], func(t *testing.T) {
			f := approvalsDaemon(draftApproval)
			f.Returns("ListGrants", &wire.GrantList{Grants: []wire.Grant{}}, nil)
			_, _, code := runCLI(t, f, args...)
			require.Equal(t, exitUsage, code)
			require.Empty(t, f.CallsTo("DecideApproval"))
		})
	}
	f := approvalsDaemon(draftApproval)
	f.Returns("ListGrants", &wire.GrantList{Grants: []wire.Grant{}}, nil)
	for _, args := range [][]string{{"run", "approvals"}, {"run", "grants"}, {"run", "ls"}} {
		f.Returns("ListRuns", &wire.RunList{Runs: []wire.Run{}}, nil)
		_, stderr, code := runCLI(t, f, args...)
		require.Equal(t, exitOK, code, stderr)
	}
	require.Empty(t, f.CallsTo("DecideApproval"))
}

// The grants it lists are what "remember" made.
func TestRunGrantsListsWhatYouRemembered(t *testing.T) {
	grantID := "0a1b2c3d-0000-4000-8000-00000000e001"
	f := clienttest.New().Returns("ListGrants", &wire.GrantList{Grants: []wire.Grant{
		{ID: grantID, Capability: "draft.create", Scope: "event:local/standup", Decision: "allow"},
	}}, nil)
	stdout, stderr, code := runCLI(t, f, "run", "grants")
	require.Equal(t, exitOK, code, stderr)
	require.Contains(t, stdout, grantID[:8])
	require.Contains(t, stdout, "allow")
	require.Contains(t, stdout, "draft.create")
	require.Contains(t, stdout, "event:local/standup")
}

func argsOf(calls []clienttest.Call) [][]any {
	out := make([][]any, len(calls))
	for i, c := range calls {
		out[i] = c.Args
	}
	return out
}
