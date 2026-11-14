package harness

import (
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
)

// WireRun is a run as the API sends it.
func WireRun(r store.Run) wire.Run {
	return wire.Run{ID: r.ID, Kind: r.Kind, Subject: r.Subject, Title: r.Title, Brief: r.Brief, Status: r.Status, Reason: r.Reason,
		ExternalRef: r.ExternalRef, Result: r.Result, CreatedAt: r.CreatedAt.UnixMilli(), UpdatedAt: r.UpdatedAt.UnixMilli(),
		StartedAt: wire.MillisPtr(r.StartedAt), EndedAt: wire.MillisPtr(r.EndedAt)}
}

// WireStep is a step as the API sends it.
func WireStep(s store.RunStep) wire.RunStep {
	return wire.RunStep{ID: s.ID, Position: s.Position, Title: s.Title, Capability: s.Capability, Risk: s.Risk, Status: s.Status,
		Input: s.Input, Output: s.Output, Error: s.Error, StartedAt: wire.MillisPtr(s.StartedAt), EndedAt: wire.MillisPtr(s.EndedAt)}
}

// WireEvent is a log line as the API sends it.
func WireEvent(ev store.RunEvent) wire.RunEvent {
	return wire.RunEvent{Seq: ev.Seq, StepID: ev.StepID, Kind: ev.Kind, Data: ev.Data, At: ev.At.UnixMilli()}
}

// WireApproval is an approval as the API sends it, with its run's title.
func WireApproval(a store.Approval, runTitle string) wire.Approval {
	scopes := a.GrantScopes
	if scopes == nil {
		scopes = []string{}
	}
	return wire.Approval{ID: a.ID, RunID: a.RunID, RunTitle: runTitle, ThreadID: a.ThreadID, StepID: a.StepID, Capability: a.Capability,
		Risk: a.Risk, Pattern: a.Pattern, Summary: a.Summary, Payload: a.Payload, GrantScopes: scopes, Status: a.Status,
		CreatedAt: a.CreatedAt.UnixMilli(), DecidedAt: wire.MillisPtr(a.DecidedAt), ExpiresAt: a.CreatedAt.Add(ApprovalTTL).UnixMilli()}
}

// WireGrant is a grant as the API sends it.
func WireGrant(g store.Grant) wire.Grant {
	return wire.Grant{ID: g.ID, Capability: g.Capability, Pattern: g.Pattern, Scope: g.Scope, Decision: g.Decision,
		CreatedAt: g.CreatedAt.UnixMilli(), ExpiresAt: wire.MillisPtr(g.ExpiresAt)}
}
