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
	DB         *store.DB
	Repos      store.Repos
	Tracker    Tracker
	Hub        *Hub
	Notify     NotifyTester
	Clock      clock.Clock
	Loc        *time.Location // the device's zone, recorded on work days that edits create
	ConfigPath string
	Version    string
	PID        int
}

// Handler returns the router for every route.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	route := func(pattern string, h func(w http.ResponseWriter, r *http.Request) error) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if err := h(w, r); err != nil {
				s.fail(w, r, err)
			}
		})
	}
	route("GET /v1/health", s.health)
	route("GET /v1/status", s.status)
	route("GET /v1/events", s.events)

	route("POST /v1/day/clock-in", s.clockIn)
	route("POST /v1/day/clock-out", s.command(func(at time.Time) timeengine.Input { return timeengine.ClockOut{At: at} }))
	route("POST /v1/break/start", s.command(func(at time.Time) timeengine.Input { return timeengine.BreakStart{At: at} }))
	route("POST /v1/break/end", s.command(func(at time.Time) timeengine.Input { return timeengine.BreakEnd{At: at} }))
	route("POST /v1/session/switch", s.switchTo)
	route("POST /v1/nudge/snooze", s.command(func(at time.Time) timeengine.Input { return timeengine.Snooze{At: at} }))

	route("GET /v1/days", s.listDays)
	route("GET /v1/days/{day}", s.getDay)
	route("PATCH /v1/days/{day}", s.patchDay)
	route("POST /v1/segments", s.createSegment)
	route("PATCH /v1/segments/{id}", s.patchSegment)
	route("POST /v1/segments/{id}/split", s.splitSegment)
	route("DELETE /v1/segments/{id}", s.deleteSegment)

	route("GET /v1/projects", s.listProjects)
	route("POST /v1/projects", s.createProject)
	route("GET /v1/projects/{id}", s.getProject)
	route("PATCH /v1/projects/{id}", s.patchProject)
	route("DELETE /v1/projects/{id}", s.deleteProject)

	route("GET /v1/tasks", s.listTasks)
	route("POST /v1/tasks", s.createTask)
	route("GET /v1/tasks/{id}", s.getTask)
	route("PATCH /v1/tasks/{id}", s.patchTask)
	route("POST /v1/tasks/{id}/complete", s.completeTask)
	route("POST /v1/tasks/{id}/reopen", s.reopenTask)
	route("DELETE /v1/tasks/{id}", s.deleteTask)

	route("GET /v1/stats/summary", s.statsSummary)
	route("GET /v1/stats/heatmap", s.statsHeatmap)

	route("GET /v1/goals", s.listGoals)
	route("POST /v1/goals", s.createGoal)
	route("GET /v1/goals/{id}", s.getGoal)
	route("PATCH /v1/goals/{id}", s.patchGoal)
	route("DELETE /v1/goals/{id}", s.deleteGoal)
	route("GET /v1/commitments", s.listCommitments)
	route("POST /v1/commitments", s.createCommitment)
	route("PATCH /v1/commitments/{id}", s.patchCommitment)
	route("DELETE /v1/commitments/{id}", s.deleteCommitment)
	route("GET /v1/plan", s.getPlan)
	route("POST /v1/plan/generate", s.generatePlan)
	route("PATCH /v1/plan/items/{id}", s.patchPlanItem)
	route("GET /v1/briefing", s.briefing)

	route("GET /v1/config", s.getConfig)
	route("PATCH /v1/config", s.patchConfig)
	route("POST /v1/notify/test", s.notifyTest)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.fail(w, r, errNotFoundRoute)
	})
	return mux
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
