package api

import (
	"context"

	"github.com/kzark/gwen/internal/wire"
)

// syncStatus is the sync client's status for integration_changed.
func (s *Server) syncStatus(ctx context.Context) wire.SyncStatus {
	if s.Sync == nil {
		return wire.SyncStatus{}
	}
	return syncWire(s.Sync.Status(ctx))
}
