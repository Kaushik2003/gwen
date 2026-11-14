package main

import "github.com/kzark/gwen/internal/wire"

// The harness (plan.md 3.7, W19): runs, approvals, grants, and connections.
// Approvals are decided only here, by a click (section 7).

func (a *App) ListRuns(statuses []string, limit int) (*wire.RunList, error) {
	return call(a.api.ListRuns(a.ctx, statuses, limit))
}
func (a *App) GetRun(id string) (*wire.RunDetail, error) { return call(a.api.GetRun(a.ctx, id)) }
func (a *App) StartRun(id string) (*wire.Run, error)     { return call(a.api.StartRun(a.ctx, id)) }
func (a *App) CancelRun(id string) (*wire.Run, error)    { return call(a.api.CancelRun(a.ctx, id)) }
func (a *App) ResumeRun(id string) (*wire.Run, error)    { return call(a.api.ResumeRun(a.ctx, id)) }
func (a *App) CreateRun(req wire.CreateRunRequest) (*wire.Run, error) {
	return call(a.api.CreateRun(a.ctx, req))
}
func (a *App) ListApprovals(status string) (*wire.ApprovalList, error) {
	return call(a.api.ListApprovals(a.ctx, status))
}
func (a *App) DecideApproval(id string, req wire.DecideApprovalRequest) (*wire.Approval, error) {
	return call(a.api.DecideApproval(a.ctx, id, req))
}
func (a *App) ListGrants() (*wire.GrantList, error) { return call(a.api.ListGrants(a.ctx)) }
func (a *App) DeleteGrant(id string) error          { return wrap(a.api.DeleteGrant(a.ctx, id)) }
func (a *App) ListConnections() (*wire.ConnectionList, error) {
	return call(a.api.ListConnections(a.ctx))
}
func (a *App) GetRoutine(subject string) (*wire.Routine, error) {
	return call(a.api.GetRoutine(a.ctx, subject))
}
func (a *App) PutRoutine(subject string, req wire.PutRoutineRequest) (*wire.Routine, error) {
	return call(a.api.PutRoutine(a.ctx, subject, req))
}
func (a *App) GetResources(subject string) (*wire.ResourceList, error) {
	return call(a.api.GetResources(a.ctx, subject))
}
func (a *App) PutResources(subject string, req wire.PutResourcesRequest) (*wire.ResourceList, error) {
	return call(a.api.PutResources(a.ctx, subject, req))
}
func (a *App) GetProjectDir(id string) (*wire.ProjectDir, error) {
	return call(a.api.GetProjectDir(a.ctx, id))
}
func (a *App) PutProjectDir(id string, req wire.ProjectDir) (*wire.ProjectDir, error) {
	return call(a.api.PutProjectDir(a.ctx, id, req))
}
func (a *App) MessageRun(id string, req wire.RunMessageRequest) (*wire.Run, error) {
	return call(a.api.MessageRun(a.ctx, id, req))
}
func (a *App) ConnectConnection(id string) error { return wrap(a.api.ConnectConnection(a.ctx, id)) }
func (a *App) DisconnectConnection(id string) error {
	return wrap(a.api.DisconnectConnection(a.ctx, id))
}
