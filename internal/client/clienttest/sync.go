package clienttest

import (
	"context"

	"github.com/kzark/gwen/internal/wire"
)

func (f *Fake) SyncStatus(context.Context) (*wire.SyncStatus, error) {
	return result[*wire.SyncStatus](f, "SyncStatus")
}

func (f *Fake) SyncNow(context.Context) (*wire.SyncStatus, error) {
	return result[*wire.SyncStatus](f, "SyncNow")
}
