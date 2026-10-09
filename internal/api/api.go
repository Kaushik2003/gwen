// Package api serves the local HTTP API of docs/04-api-contract.md on the
// daemon's socket: the handlers, the Server-Sent Events hub, and the one
// function that turns errors into error responses.
package api

import (
	"context"
	"net/http"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/timeengine"
	"github.com/kzark/gwen/internal/wire"
)

// Tracker is the daemon's engine loop as the API sees it. Handlers never
// change tracking state themselves; they hand the loop an input and wait.
type Tracker interface {
	// Do runs one engine input, built at the loop's current instant, and
	// returns the resulting status.
	Do(ctx context.Context, build func(at time.Time) timeengine.Input) (wire.Status, error)
	Status(ctx context.Context) (wire.Status, error)
	Snapshot(ctx context.Context) (timeengine.Snapshot, error)
	Config() config.Config
	// SetConfig applies a validated configuration to the running daemon.
	SetConfig(ctx context.Context, c config.Config) error
}

// NotifyTester sends the test notification of POST /v1/notify/test.
type NotifyTester interface {
	Test(ctx context.Context) wire.NotifyTestResult
}

// Server holds what the handlers need.
type Server struct {
	DB      *store.DB
	Repos   store.Repos
	Tracker Tracker
	Hub     *Hub
	Notify  NotifyTester
	// Calendar is the Google Calendar sync, nil when the daemon runs none.
	Calendar CalendarService
	// LLM builds the LLM adapter for the configuration in effect, nil when
	// the daemon runs none.
	LLM func(cfg config.LLM, self llm.Self) (llm.Planner, error)
	// Sync is the sync hub client, nil when the daemon runs none.
	Sync       SyncService
	Clock      clock.Clock
	Loc        *time.Location // the device's zone, recorded on work days that edits create
	ConfigPath string
	Version    string
	PID        int
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) error {
	writeJSON(w, http.StatusOK, wire.Health{OK: true, Version: s.Version, SchemaVersion: s.DB.SchemaVersion(), PID: s.PID})
	return nil
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) error {
	st, err := s.Tracker.Status(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, st)
	return nil
}

// now is the daemon's clock reading at millisecond precision.
func (s *Server) now() time.Time { return s.Clock.Now().Truncate(time.Millisecond) }

// today is the day the current instant belongs to.
func (s *Server) today() string {
	return timeengine.ConfigFrom(s.Tracker.Config(), s.Loc).DayOf(s.now())
}
