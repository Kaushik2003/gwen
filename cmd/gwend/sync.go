package main

import (
	"context"
	"log/slog"

	"github.com/kzark/gwen/internal/api"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/store"
	gsync "github.com/kzark/gwen/internal/sync"
	"github.com/kzark/gwen/internal/wire"
)

// newSyncClient builds the sync hub client. Pulled rows are announced with
// the invalidation events their tables call for, and every attempt as
// integration_changed.
func newSyncClient(db *store.DB, repos store.Repos, o options, l *loop, srv *api.Server, hub *api.Hub) *gsync.Client {
	return gsync.NewClient(gsync.ClientOptions{
		DB: db, Repo: repos.Sync, Clock: o.clk, CredDir: config.CredentialsDir(o.dataDir), Config: l.Config,
		OnApplied: func(a store.Applied) {
			if len(a.IDs["projects"]) > 0 {
				hub.Publish(wire.EventProjectsChanged, wire.ProjectsChanged{})
			}
			if ids := a.IDs["tasks"]; len(ids) > 0 {
				hub.Publish(wire.EventTasksChanged, wire.TasksChanged{TaskIDs: ids})
			}
			if len(a.IDs["goals"])+len(a.IDs["commitments"]) > 0 {
				hub.Publish(wire.EventGoalsChanged, wire.GoalsChanged{})
			}
			if len(a.IDs["claims"])+len(a.IDs["conflicts"])+len(a.IDs["entities"]) > 0 {
				claims := a.IDs["claims"]
				if claims == nil {
					claims = []string{}
				}
				hub.Publish(wire.EventMemoryChanged, wire.MemoryChanged{ClaimIDs: claims,
					Conflicts: len(a.IDs["conflicts"]) > 0})
			}
			onMemoryApplied(srv.Memory, a)
			workDays, planDays, err := repos.Sync.Days(context.Background(), a)
			if err != nil {
				slog.Warn("days of pulled rows", "err", err)
				return
			}
			for _, d := range workDays {
				hub.Publish(wire.EventDayChanged, wire.DayChanged{Day: d})
			}
			api.PublishChanges(hub, store.Changes{PlanDays: planDays})
		},
		OnAttempt: func() { srv.PublishIntegration(context.Background()) },
	})
}

// pullWatcher is a memory that catches up with rows sync applied.
type pullWatcher interface {
	Pulled(ctx context.Context, a store.Applied)
}

// onMemoryApplied is the memory's hook into sync. It sees every pulled page
// applied, so that the memory can catch up with what other devices wrote:
// index pulled claims, messages, and episodes, and drop forgotten ones from
// its lanes. a.IDs holds the applied ids by table. Without a memory service
// it does nothing.
func onMemoryApplied(m memory.Memory, a store.Applied) {
	if w, ok := m.(pullWatcher); ok {
		w.Pulled(context.Background(), a)
	}
}
