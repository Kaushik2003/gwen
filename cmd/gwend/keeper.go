package main

import (
	"context"
	"time"

	"github.com/kzark/gwen/internal/api"
	"github.com/kzark/gwen/internal/clock"
)

// keepEvery is how often the plan keeper brings today's plan up to date.
// Planned blocks are timed on a 5-minute grid, so every three steps of it
// keeps the plan current without moving calendar events every few minutes.
const keepEvery = 15 * time.Minute

// keepPlan brings today's plan up to date at every keepEvery boundary until
// ctx ends: blocks left in the past move to the time left, and a new day gets
// its plan without anyone opening it. Writes and the plan_changed events
// they cause happen only when something moved.
func keepPlan(ctx context.Context, clk clock.Clock, srv *api.Server) {
	t := clk.NewTimer(untilNext(clk.Now(), keepEvery))
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C():
			srv.Replan(ctx)
			t.Reset(untilNext(clk.Now(), keepEvery))
		}
	}
}

// untilNext is how long from now until the next multiple of every, a second
// past it so the grid has turned.
func untilNext(now time.Time, every time.Duration) time.Duration {
	return now.Truncate(every).Add(every).Sub(now) + time.Second
}
