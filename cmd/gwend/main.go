// Command gwend is the Gwen daemon: the sole owner of the database, the
// activity monitor, the time engine, and the notifications, serving the API
// on a Unix socket (docs/02-architecture.md).
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/kzark/gwen/internal/activity"
	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"gopkg.in/natefinch/lumberjack.v2"
)

var version = "dev"

func main() {
	os.Exit(mainErr(os.Args[1:], os.Stderr))
}

func mainErr(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("gwend", flag.ContinueOnError)
	fs.SetOutput(stderr)
	foreground := fs.Bool("foreground", false, "also log readable text to stderr")
	dataDir := fs.String("data-dir", "", "database and credentials location (default $XDG_DATA_HOME/gwen)")
	configPath := fs.String("config", "", "config file (default $XDG_CONFIG_HOME/gwen/config.toml)")
	hub := fs.Bool("hub", false, "run as the sync hub")
	listen := fs.String("listen", ":7777", "hub listen address; only with --hub")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	listenSet := false
	fs.Visit(func(f *flag.Flag) { listenSet = listenSet || f.Name == "listen" })
	if listenSet && !*hub {
		fmt.Fprintln(stderr, "gwend: --listen is only valid with --hub")
		return 2
	}
	paths, err := config.DefaultPaths()
	if err != nil {
		fmt.Fprintln(stderr, "gwend:", err)
		return 1
	}
	if *dataDir == "" {
		*dataDir = paths.DataDir
	}
	if *configPath == "" {
		*configPath = paths.ConfigFile
	}
	level := new(slog.LevelVar)
	slog.SetDefault(slog.New(newLogHandler(paths.LogFile(), *foreground, stderr, level)))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	if *hub {
		err = runHub(ctx, hubOptions{dataDir: *dataDir, configPath: *configPath, listen: *listen, version: version,
			clk: clock.Real(), loc: localZone(), level: level})
		if err != nil {
			slog.Error("gwend --hub failed", "err", err)
			fmt.Fprintln(stderr, "gwend:", err)
			return 1
		}
		return 0
	}
	err = run(ctx, options{
		dataDir:    *dataDir,
		configPath: *configPath,
		socketPath: paths.SocketPath(),
		version:    version,
		clk:        clock.Real(),
		loc:        localZone(),
		level:      level,
		monitor:    func(ctx context.Context, clk clock.Clock) activity.ActivityMonitor { return activity.New(ctx, clk) },
		notifier:   defaultNotifier,
		calendar:   true,
		llm:        true,
		sync:       true,
	})
	if err != nil {
		slog.Error("gwend failed", "err", err)
		fmt.Fprintln(stderr, "gwend:", err)
		return 1
	}
	return 0
}

// newLogHandler writes JSON to the rotated log file, and readable text to
// stderr in the foreground (docs/CONVENTIONS.md#logging).
func newLogHandler(logFile string, foreground bool, stderr io.Writer, level *slog.LevelVar) slog.Handler {
	opts := &slog.HandlerOptions{Level: level}
	var handlers []slog.Handler
	if err := os.MkdirAll(filepath.Dir(logFile), 0o700); err == nil {
		rotated := &lumberjack.Logger{Filename: logFile, MaxSize: 10, MaxBackups: 3}
		handlers = append(handlers, slog.NewJSONHandler(rotated, opts))
	}
	if foreground || len(handlers) == 0 {
		handlers = append(handlers, slog.NewTextHandler(stderr, opts))
	}
	return slog.NewMultiHandler(handlers...)
}

// localZone is the device's zone with its IANA name, which work days record:
// $TZ, else the target of the /etc/localtime link, else UTC.
func localZone() *time.Location {
	name := strings.TrimPrefix(os.Getenv("TZ"), ":")
	if name == "" {
		if target, err := os.Readlink("/etc/localtime"); err == nil {
			if _, after, ok := strings.Cut(target, "zoneinfo/"); ok {
				name = after
			}
		}
	}
	if name != "" {
		if loc, err := time.LoadLocation(name); err == nil {
			return loc
		}
	}
	slog.Warn("cannot tell the local time zone; using UTC")
	return time.UTC
}
