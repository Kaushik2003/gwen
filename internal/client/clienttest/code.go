package clienttest

import (
	"context"

	"github.com/kzark/gwen/internal/wire"
)

func (f *Fake) CodeSessions(context.Context) (*wire.CodeSessionList, error) {
	return result[*wire.CodeSessionList](f, "CodeSessions")
}

func (f *Fake) StartCodeSession(_ context.Context, req wire.StartCodeSession) (*wire.CodeSession, error) {
	return result[*wire.CodeSession](f, "StartCodeSession", req)
}

func (f *Fake) RemoteStatus(context.Context) (*wire.RemoteStatus, error) {
	return result[*wire.RemoteStatus](f, "RemoteStatus")
}

func (f *Fake) RemotePairCode(context.Context) (*wire.RemotePairCode, error) {
	return result[*wire.RemotePairCode](f, "RemotePairCode")
}

func (f *Fake) RemoteDevices(context.Context) (*wire.RemoteDevices, error) {
	return result[*wire.RemoteDevices](f, "RemoteDevices")
}

func (f *Fake) RevokeRemoteDevice(_ context.Context, id string) error {
	_, err := f.invoke("RevokeRemoteDevice", id)
	return err
}

func (f *Fake) SetRemoteHooks(_ context.Context, on bool) (*wire.RemoteStatus, error) {
	return result[*wire.RemoteStatus](f, "SetRemoteHooks", on)
}

func (f *Fake) ClaudeUsage(context.Context) (*wire.ClaudeUsage, error) {
	return result[*wire.ClaudeUsage](f, "ClaudeUsage")
}

func (f *Fake) CommitSessions(_ context.Context, dir string) (*wire.CommitSessions, error) {
	return result[*wire.CommitSessions](f, "CommitSessions", dir)
}

func (f *Fake) SummarizeCommit(_ context.Context, req wire.SummarizeCommit) (*wire.CommitSummary, error) {
	return result[*wire.CommitSummary](f, "SummarizeCommit", req)
}

func (f *Fake) CodeAnswer(_ context.Context, id string, req wire.CodeAnswer) (*wire.CodeAnswerResult, error) {
	return result[*wire.CodeAnswerResult](f, "CodeAnswer", id, req)
}

func (f *Fake) CodeMark(_ context.Context, id string, req wire.CodeMark) error {
	_, err := f.invoke("CodeMark", id, req)
	return err
}

func (f *Fake) CodeControl(_ context.Context, id string, req wire.CodeControl) error {
	_, err := f.invoke("CodeControl", id, req)
	return err
}

func (f *Fake) CodeConversations(_ context.Context, dir string) (*wire.CodeConversations, error) {
	return result[*wire.CodeConversations](f, "CodeConversations", dir)
}
