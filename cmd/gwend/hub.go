package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/kzark/gwen/internal/api"
	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/store"
	gsync "github.com/kzark/gwen/internal/sync"
	"github.com/kzark/gwen/internal/timeengine"
	"github.com/kzark/gwen/internal/wire"
	"github.com/kzark/gwen/ui"
)

// hubOptions is everything runHub needs.
type hubOptions struct {
	dataDir    string
	configPath string
	listen     string
	version    string
	clk        clock.Clock
	loc        *time.Location
	level      *slog.LevelVar
	// ready, if set, receives the bound address, or "" on a startup error.
	ready chan<- string
}

// errHub answers anything on the hub that needs a tracking device.
var errHub = errors.New("the sync hub does not track time")

// hubTracker stands in for the engine loop on the hub, which has none: it
// serves only the configuration the read-only API reads.
type hubTracker struct{ cfg config.Config }

func (h hubTracker) Do(context.Context, func(time.Time) timeengine.Input) (wire.Status, error) {
	return wire.Status{}, errHub
}
func (h hubTracker) Status(context.Context) (wire.Status, error) { return wire.Status{}, errHub }
func (h hubTracker) Snapshot(context.Context) (timeengine.Snapshot, error) {
	return timeengine.Snapshot{State: timeengine.Off}, nil
}
func (h hubTracker) Config() config.Config                          { return h.cfg }
func (h hubTracker) SetConfig(context.Context, config.Config) error { return errHub }

// runHub is gwend --hub (docs/07-integrations.md#sync-hub): the hub API, the
// sign-in, the read-only API, and the dashboard over its own database, with
// no activity monitor, engine, or notifier.
func runHub(ctx context.Context, o hubOptions) (err error) {
	defer func() {
		if o.ready != nil && err != nil {
			o.ready <- ""
		}
	}()
	cfg, err := config.Load(o.configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	o.level.Set(cfg.Log.Level)
	db, err := store.Open(ctx, o.dataDir, o.clk)
	if err != nil {
		return fmt.Errorf("the database is not usable: %w", err)
	}
	defer db.Close()
	repos := store.NewRepos(db)
	hub := &gsync.Hub{DB: db, Repo: repos.Sync, CredDir: config.CredentialsDir(o.dataDir), Clock: o.clk}
	srv := &api.Server{DB: db, Repos: repos, Tracker: hubTracker{cfg}, Hub: api.NewHub(o.clk), Clock: o.clk,
		Loc: o.loc, ConfigPath: o.configPath, Version: o.version, PID: os.Getpid()}
	dashboard, err := fs.Sub(ui.HubAssets, "dist-hub")
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("/sync/v1/", hub.Handler())
	mux.HandleFunc("POST /login", hub.Login)
	mux.Handle("/v1/", hub.Require(srv.ReadOnlyHandler()))
	mux.Handle("/", spa(dashboard))

	ln, err := net.Listen("tcp", o.listen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", o.listen, err)
	}
	httpSrv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	serveErr := make(chan error, 1)
	go func() { serveErr <- httpSrv.Serve(ln) }()
	slog.Info("hub listening", "addr", ln.Addr().String(), "schema_version", db.SchemaVersion())
	if o.ready != nil {
		o.ready <- ln.Addr().String()
	}
	select {
	case <-ctx.Done():
	case err := <-serveErr:
		slog.Error("hub server stopped", "err", err)
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdown); err != nil {
		slog.Warn("shutdown", "err", err)
	}
	slog.Info("hub stopped")
	return nil
}

// spa serves the dashboard's files, and its index for any other path so
// that the page can route itself.
func spa(files fs.FS) http.Handler {
	fileServer := http.FileServerFS(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := path.Clean(r.URL.Path)[1:]
		if name == "" {
			name = "index.html"
		}
		if _, err := fs.Stat(files, name); err != nil {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
