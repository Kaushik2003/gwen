package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/kzark/gwen/internal/activity"
	"github.com/kzark/gwen/internal/api"
	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/gcal"
	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/notify"
	"github.com/kzark/gwen/internal/store"
	gsync "github.com/kzark/gwen/internal/sync"
	"github.com/kzark/gwen/internal/timeengine"
)

// engineEventsKept is how long engine_events rows are kept.
const engineEventsKept = 90 * 24 * time.Hour

// options is everything run needs; main fills it from the flags and the
// environment, and tests substitute fakes.
type options struct {
	dataDir    string
	configPath string
	socketPath string
	version    string
	clk        clock.Clock
	loc        *time.Location
	level      *slog.LevelVar
	monitor    func(ctx context.Context, clk clock.Clock) activity.ActivityMonitor
	notifier   func(cfg config.Config, credDir string) (notifier, []string)
	// calendar runs the Google Calendar sync (docs/07-integrations.md#google-calendar).
	calendar bool
	// llm runs the LLM adapter (docs/07-integrations.md#llm-adapter).
	llm bool
	// anthropicURL overrides the Messages API URL; tests point it at a fake.
	anthropicURL string
	// sync runs the sync hub client (docs/07-integrations.md#client).
	sync bool
	// keeper keeps today's plan up to date as time moves on (keepPlan).
	keeper bool
	// calendarEndpoint overrides the Calendar API URL; tests point it at a fake.
	calendarEndpoint string
	// wrapRepos lets tests make the store fail inside a decision.
	wrapRepos func(store.Repos) store.Repos
	// ready, if set, receives nil once the socket accepts connections, or the
	// startup error.
	ready chan<- error
	// onLoop, if set, sees the engine loop before it starts; tests use it.
	onLoop func(*loop)
}

func defaultNotifier(cfg config.Config, credDir string) (notifier, []string) {
	desktop := notify.NewDesktop()
	var warnings []string
	if !desktop.Available() {
		warnings = append(warnings, "desktop notifications are unavailable: no notification service was found")
	}
	return notify.NewDispatcher(desktop, notify.NewNtfy(nil, credDir), desktop.Actions(), cfg), warnings
}

// run is the daemon: open the store, recover, bind the socket, serve until
// ctx ends, then shut down cleanly.
func run(ctx context.Context, o options) (err error) {
	defer func() {
		if o.ready != nil && err != nil {
			o.ready <- err
		}
	}()
	cfg, err := config.Load(o.configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	o.level.Set(cfg.Log.Level)
	slog.Info("starting", "version", o.version, "config", cfg, "zone", o.loc.String())

	db, err := store.Open(ctx, o.dataDir, o.clk)
	if err != nil {
		return fmt.Errorf("the database is not usable: %w", err)
	}
	defer db.Close()
	repos := store.NewRepos(db)
	if o.wrapRepos != nil {
		repos = o.wrapRepos(repos)
	}
	if n, err := repos.Events.Prune(ctx, o.clk.Now().Add(-engineEventsKept)); err != nil {
		return err
	} else if n > 0 {
		slog.Info("pruned engine events", "rows", n)
	}

	engine, err := recoverEngine(ctx, db, repos, timeengine.ConfigFrom(cfg, o.loc), o.clk.Now().Truncate(time.Millisecond))
	if err != nil {
		return fmt.Errorf("recover: %w", err)
	}

	mon := o.monitor(ctx, o.clk)
	var warnings []string
	if mon.Name() == activity.NameNone {
		warnings = append(warnings, "idle detection is unavailable: tracking continues without automatic breaks")
	}
	n, notifyWarnings := o.notifier(cfg, config.CredentialsDir(o.dataDir))
	warnings = append(warnings, notifyWarnings...)

	hub := api.NewHub(o.clk)
	l := newLoop(o.clk, db, repos, engine, hub, n, mon, cfg, o.loc, o.level, warnings)
	if o.onLoop != nil {
		o.onLoop(l)
	}
	srv := &api.Server{DB: db, Repos: repos, Tracker: l, Hub: hub, Notify: n, Clock: o.clk, Loc: o.loc,
		ConfigPath: o.configPath, Version: o.version, PID: os.Getpid()}

	if o.llm {
		credDir := config.CredentialsDir(o.dataDir)
		srv.LLM = func(cfg config.LLM) (llm.Planner, error) {
			return llm.New(cfg, llm.Options{CredDir: credDir, AnthropicURL: o.anthropicURL})
		}
	}
	var syncer *gsync.Client
	if o.sync {
		syncer = newSyncClient(db, repos, o, l, srv, hub)
		srv.Sync = syncer
	}
	var cal *gcal.Service
	if o.calendar {
		cal = newCalendar(db, repos, o, l, srv, hub)
		srv.Calendar = cal
	}

	ln, err := bindSocket(ctx, o.socketPath)
	if err != nil {
		return err
	}
	loopCtx, stopLoop := context.WithCancel(context.Background())
	go l.run(loopCtx)
	if cal != nil {
		go cal.Run(loopCtx)
	}
	if syncer != nil {
		go syncer.Run(loopCtx)
	}
	if o.keeper {
		go keepPlan(loopCtx, o.clk, srv)
	}
	httpSrv := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	serveErr := make(chan error, 1)
	go func() { serveErr <- httpSrv.Serve(ln) }()
	slog.Info("listening", "socket", o.socketPath, "activity_backend", mon.Name())
	if o.ready != nil {
		o.ready <- nil
	}

	select {
	case <-ctx.Done():
	case err := <-serveErr:
		slog.Error("server stopped", "err", err)
	}
	hub.Close()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		slog.Warn("shutdown", "err", err)
	}
	stopLoop()
	<-l.done // the loop writes shutdown_at on its way out
	os.Remove(o.socketPath)
	slog.Info("stopped")
	return nil
}

// recoverEngine loads the stored state, builds the engine, and commits its
// recovery effects together with clearing shutdown_at. A failure is fatal.
func recoverEngine(ctx context.Context, db *store.DB, repos store.Repos, cfg timeengine.Config, now time.Time) (*timeengine.Engine, error) {
	rec := timeengine.Recovered{Now: now}
	var err error
	if rec.WorkDay, err = repos.WorkDays.Current(ctx); err != nil {
		return nil, err
	}
	if rec.Segment, err = repos.Segments.Current(ctx); err != nil {
		return nil, err
	}
	if rec.WorkDay != nil {
		if rec.LastEndedAt, err = repos.Segments.LatestEnd(ctx, rec.WorkDay.ID); err != nil {
			return nil, err
		}
	}
	for key, dst := range map[string]**time.Time{store.KeyHeartbeatAt: &rec.HeartbeatAt, store.KeyShutdownAt: &rec.ShutdownAt} {
		t, err := store.GetLocalTime(ctx, db.SQL(), key)
		switch {
		case err == nil:
			*dst = &t
		case !errors.Is(err, store.ErrNotFound):
			return nil, err
		}
	}
	if w, err := repos.Segments.LatestWork(ctx); err != nil {
		return nil, err
	} else if w != nil {
		rec.ProjectID, rec.TaskID = w.ProjectID, w.TaskID
	}
	if rec.ClosedDay, err = repos.WorkDays.LatestClosedDay(ctx); err != nil {
		return nil, err
	}

	engine := timeengine.New(cfg, rec)
	day := ""
	if rec.WorkDay != nil {
		day = rec.WorkDay.Day
	}
	err = db.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := applyEffects(ctx, tx, repos, day, engine.RecoveryEffects()); err != nil {
			return err
		}
		return store.DeleteLocal(ctx, tx, store.KeyShutdownAt)
	})
	if err != nil {
		return nil, err
	}
	slog.Info("recovered", "state", engine.Status().State, "effects", len(engine.RecoveryEffects()))
	return engine, nil
}

// bindSocket listens on the API socket, mode 0600 in a 0700 directory. A
// socket file with a live daemon behind it is an error; a stale one is
// replaced (docs/02-architecture.md#failure-and-degradation).
func bindSocket(ctx context.Context, path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("socket directory: %w", err)
	}
	if _, err := os.Stat(path); err == nil {
		probe, cancel := context.WithTimeout(ctx, 2*time.Second)
		h, err := client.New(path).Health(probe)
		cancel()
		if err == nil {
			return nil, fmt.Errorf("gwend is already running (pid %d) on %s", h.PID, path)
		}
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("remove stale socket: %w", err)
		}
		slog.Info("removed a stale socket", "socket", path)
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		return nil, fmt.Errorf("socket permissions: %w", err)
	}
	return ln, nil
}
