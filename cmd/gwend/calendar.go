package main

import (
	"context"
	"time"

	"github.com/kzark/gwen/internal/api"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/gcal"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/timeengine"
	"github.com/kzark/gwen/internal/wire"
)

// newCalendar builds the Google Calendar sync. It syncs Debounce after every
// plan or configuration change, and every attempt is reported as integration_changed, with
// plan_changed for the days a pulled event moved.
func newCalendar(db *store.DB, repos store.Repos, o options, l *loop, srv *api.Server, hub *api.Hub) *gcal.Service {
	cal := gcal.New(gcal.Options{
		DB: db, Repo: repos.Calendar, Clock: o.clk, CredDir: config.CredentialsDir(o.dataDir), Loc: o.loc,
		Config: l.Config,
		DayOf:  func(t time.Time) string { return timeengine.ConfigFrom(l.Config(), o.loc).DayOf(t) },
		OnAttempt: func(ch gcal.Changes) {
			api.PublishChanges(hub, store.Changes{PlanDays: ch.PlanDays})
			srv.PublishIntegration(context.Background())
		},
		Endpoint: o.calendarEndpoint,
	})
	hub.OnPublish(func(name string) {
		if name == wire.EventPlanChanged || name == wire.EventConfigChanged { // a busy calendar may be new
			cal.PlanChanged()
		}
	})
	return cal
}
