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

func (f *Fake) OpenApp(_ context.Context, screen string) error {
	_, err := f.invoke("OpenApp", screen)
	return err
}
