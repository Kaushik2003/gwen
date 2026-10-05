package api

import (
	"context"
	"errors"
	"net/http"

	gsync "github.com/kzark/gwen/internal/sync"
	"github.com/kzark/gwen/internal/wire"
)

// SyncService is the sync hub client as the API sees it
// (docs/07-integrations.md#client).
type SyncService interface {
	// Available is nil when a hub and its token are configured.
	Available() error
	Status(ctx context.Context) gsync.Status
	Sync(ctx context.Context) (gsync.Status, error)
}

func syncWire(st gsync.Status) wire.SyncStatus {
	out := wire.SyncStatus{Configured: st.Configured, LastPushAt: wire.MillisPtr(st.LastPushAt),
		LastPullAt: wire.MillisPtr(st.LastPullAt)}
	if st.LastError != "" {
		out.LastError = &st.LastError
	}
	return out
}

// syncClient returns the client, or unavailable when no hub is configured.
func (s *Server) syncClient() (SyncService, error) {
	if s.Sync == nil {
		return nil, Unavailable("sync is not running in this daemon")
	}
	if err := s.Sync.Available(); err != nil {
		return nil, syncErr(err)
	}
	return s.Sync, nil
}

func syncErr(err error) error {
	if errors.Is(err, gsync.ErrUnavailable) {
		return Unavailable("%s", err.Error())
	}
	return err
}

func (s *Server) syncStatusHandler(w http.ResponseWriter, r *http.Request) error {
	c, err := s.syncClient()
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, syncWire(c.Status(r.Context())))
	return nil
}

func (s *Server) syncNow(w http.ResponseWriter, r *http.Request) error {
	var body struct{}
	if err := decode(r, &body); err != nil {
		return err
	}
	c, err := s.syncClient()
	if err != nil {
		return err
	}
	st, err := c.Sync(r.Context())
	if err != nil {
		return syncErr(err)
	}
	writeJSON(w, http.StatusOK, syncWire(st))
	return nil
}

// storedPlan is GET /v1/plan on the hub: stored items only, never rolled
// over or generated (docs/07-integrations.md#read-only-dashboard).
func (s *Server) storedPlan(w http.ResponseWriter, r *http.Request) error {
	p, err := s.Repos.Plans.Read(r.Context(), r.URL.Query().Get("day"), s.planEnv())
	if err != nil {
		return err
	}
	items, err := s.entriesWire(r.Context(), p.Items)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, planWire(p, items[0]))
	return nil
}
