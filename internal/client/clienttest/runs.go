package clienttest

import (
	"context"

	"github.com/kzark/gwen/internal/wire"
)

func (f *Fake) ListRuns(_ context.Context, statuses []string, limit int) (*wire.RunList, error) {
	return result[*wire.RunList](f, "ListRuns", statuses, limit)
}

func (f *Fake) CreateRun(_ context.Context, req wire.CreateRunRequest) (*wire.Run, error) {
	return result[*wire.Run](f, "CreateRun", req)
}

func (f *Fake) GetRun(_ context.Context, id string) (*wire.RunDetail, error) {
	return result[*wire.RunDetail](f, "GetRun", id)
}

func (f *Fake) StartRun(_ context.Context, id string) (*wire.Run, error) {
	return result[*wire.Run](f, "StartRun", id)
}

func (f *Fake) CancelRun(_ context.Context, id string) (*wire.Run, error) {
	return result[*wire.Run](f, "CancelRun", id)
}

func (f *Fake) ResumeRun(_ context.Context, id string) (*wire.Run, error) {
	return result[*wire.Run](f, "ResumeRun", id)
}

func (f *Fake) MessageRun(_ context.Context, id string, req wire.RunMessageRequest) (*wire.Run, error) {
	return result[*wire.Run](f, "MessageRun", id, req)
}

func (f *Fake) ListApprovals(_ context.Context, status string) (*wire.ApprovalList, error) {
	return result[*wire.ApprovalList](f, "ListApprovals", status)
}

func (f *Fake) DecideApproval(_ context.Context, id string, req wire.DecideApprovalRequest) (*wire.Approval, error) {
	return result[*wire.Approval](f, "DecideApproval", id, req)
}

func (f *Fake) GetResources(_ context.Context, subject string) (*wire.ResourceList, error) {
	return result[*wire.ResourceList](f, "GetResources", subject)
}

func (f *Fake) PutResources(_ context.Context, subject string, req wire.PutResourcesRequest) (*wire.ResourceList, error) {
	return result[*wire.ResourceList](f, "PutResources", subject, req)
}

func (f *Fake) GetRoutine(_ context.Context, subject string) (*wire.Routine, error) {
	return result[*wire.Routine](f, "GetRoutine", subject)
}

func (f *Fake) PutRoutine(_ context.Context, subject string, req wire.PutRoutineRequest) (*wire.Routine, error) {
	return result[*wire.Routine](f, "PutRoutine", subject, req)
}

func (f *Fake) GetProjectDir(_ context.Context, id string) (*wire.ProjectDir, error) {
	return result[*wire.ProjectDir](f, "GetProjectDir", id)
}

func (f *Fake) PutProjectDir(_ context.Context, id string, req wire.ProjectDir) (*wire.ProjectDir, error) {
	return result[*wire.ProjectDir](f, "PutProjectDir", id, req)
}

func (f *Fake) ListConnections(_ context.Context) (*wire.ConnectionList, error) {
	return result[*wire.ConnectionList](f, "ListConnections")
}

func (f *Fake) ConnectConnection(_ context.Context, id string) error {
	_, err := f.invoke("ConnectConnection", id)
	return err
}

func (f *Fake) DisconnectConnection(_ context.Context, id string) error {
	_, err := f.invoke("DisconnectConnection", id)
	return err
}

func (f *Fake) ListGrants(_ context.Context) (*wire.GrantList, error) {
	return result[*wire.GrantList](f, "ListGrants")
}

func (f *Fake) DeleteGrant(_ context.Context, id string) error {
	_, err := f.invoke("DeleteGrant", id)
	return err
}
