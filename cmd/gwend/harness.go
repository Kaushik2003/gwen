package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/kzark/gwen/internal/api"
	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/connect"
	"github.com/kzark/gwen/internal/gcal"
	"github.com/kzark/gwen/internal/harness"
	"github.com/kzark/gwen/internal/store"
)

// newHarness builds the run engine with this device's connectors: the
// desktop and the calendar cache (plan.md 3.7, 3.8, W19). It
// recovers the runs a restart interrupted.
func newHarness(ctx context.Context, repos store.Repos, srv *api.Server, cal *gcal.Service, clk clock.Clock, publish func(string, any)) *harness.Engine {
	home, _ := os.UserHomeDir()
	today := func() string { return clk.Now().In(srv.Loc).Format(time.DateOnly) }
	reg := connect.NewRegistry(
		connect.Desktop(nil, srv.StartTracking, home),
		connect.Calendar(calendarCache{repos: repos, cal: cal}, srv.Loc, today),
	)
	routineOf := func(ctx context.Context, subject string) (store.Routine, bool, error) {
		return srv.RoutineOf(ctx, subject)
	}
	e := harness.New(harness.Options{Runs: repos.Runs, Registry: reg, Clock: clk, Publish: publish, Scopes: srv.SubjectChain},
		harness.Routine(reg, routineOf))
	if err := e.Recover(ctx); err != nil {
		slog.Warn("runs: recover", "err", err)
	}
	return e
}

// calendarCache reads the calendar sync's busy cache for calendar.read.
type calendarCache struct {
	repos store.Repos
	cal   *gcal.Service
}

func (c calendarCache) Busy(ctx context.Context, from, to time.Time) ([]connect.Busy, error) {
	busy, err := c.repos.Calendar.Busy(ctx, from, to)
	if err != nil {
		return nil, err
	}
	out := make([]connect.Busy, len(busy))
	for i, b := range busy {
		out[i] = connect.Busy{Title: b.Title, Start: b.Start, End: b.End}
	}
	return out, nil
}

func (c calendarCache) Status(ctx context.Context) connect.Status {
	if c.cal == nil {
		return connect.Status{State: connect.NeedsSetup, Message: "Calendar sync is off in this daemon"}
	}
	if err := c.cal.Available(); err != nil {
		return connect.Status{State: connect.NeedsSetup, Message: "Set up Google Calendar in Settings › Calendar"}
	}
	st := c.cal.Status(ctx)
	switch {
	case !st.Enabled || !st.Connected:
		return connect.Status{State: connect.NeedsSetup, Message: "Connect Google Calendar in Settings › Calendar"}
	case st.LastError != "":
		return connect.Status{State: connect.Failed, Message: "The last sync failed; what Gwen read may be old"}
	}
	return connect.Status{State: connect.Connected}
}
