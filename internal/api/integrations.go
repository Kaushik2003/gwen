package api

import (
	"context"

	"github.com/kzark/gwen/internal/wire"
)

// syncStatus is the sync hub client's status for integration_changed; this
// daemon runs no sync client, so sync is not configured.
func (s *Server) syncStatus(context.Context) wire.SyncStatus {
	return wire.SyncStatus{}
}
