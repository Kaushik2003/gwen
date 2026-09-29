package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/activity"
	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/notify"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/timeengine"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

func init() {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// fakeMonitor lets a test inject activity events.
type fakeMonitor struct {
	name       string
	mu         sync.Mutex
	cur        chan activity.Event
	thresholds [][]time.Duration
}

func (m *fakeMonitor) Name() string { return m.name }

func (m *fakeMonitor) Start(ctx context.Context, th []time.Duration) (<-chan activity.Event, error) {
	out := make(chan activity.Event)
	m.mu.Lock()
	m.cur = out
	m.thresholds = append(m.thresholds, th)
	m.mu.Unlock()
	return out, nil
}

// emit hands an event to the loop; it returns once the loop has taken it.
func (m *fakeMonitor) emit(ev activity.Event) {
	m.mu.Lock()
	ch := m.cur
	m.mu.Unlock()
	ch <- ev
}

func (m *fakeMonitor) starts() [][]time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([][]time.Duration{}, m.thresholds...)
}

// fakeNotifier records what the loop sends.
type fakeNotifier struct {
	mu        sync.Mutex
	sent      []notify.Notification
	withdrawn []string
	configs   int
	actions   chan notify.ActionEvent
}

func (n *fakeNotifier) Send(x notify.Notification) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent = append(n.sent, x)
}

func (n *fakeNotifier) Withdraw(kind string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.withdrawn = append(n.withdrawn, kind)
}

func (n *fakeNotifier) Actions() <-chan notify.ActionEvent { return n.actions }

func (n *fakeNotifier) SetConfig(config.Config) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.configs++
}

func (n *fakeNotifier) Test(context.Context) wire.NotifyTestResult {
	return wire.NotifyTestResult{Desktop: wire.NotifyOK, Phone: wire.NotifyDisabled}
}

func (n *fakeNotifier) kinds() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	var out []string
	for _, x := range n.sent {
		out = append(out, x.Kind)
	}
	return out
}

// daemon is a gwend running in-process on a fake clock.
type daemon struct {
	t       *testing.T
	ctx     context.Context
	clk     *clock.Fake
	mon     *fakeMonitor
	notif   *fakeNotifier
	c       *client.Client
	loop    *loop
	dataDir string
	cfgPath string
	socket  string
	stop    func() error
}

// t0 is the daemon's clock at start: 2026-09-15 08:00 UTC.
var t0 = testutil.At("08:00")

type setup struct {
	dataDir   string
	socket    string
	wrapRepos func(store.Repos) store.Repos
	monName   string
	calendar  bool
	llm       bool
}

func startDaemon(t *testing.T, s setup) *daemon {
	t.Helper()
	return startDaemonWith(t, s, nil)
}

// startDaemonWith is startDaemon with a last adjustment of the options.
func startDaemonWith(t *testing.T, s setup, adjust func(*options)) *daemon {
	t.Helper()
	dir := t.TempDir()
	if s.dataDir == "" {
		s.dataDir = filepath.Join(dir, "data")
	}
	if s.socket == "" {
		s.socket = testutil.SocketPath(t)
	}
	if s.monName == "" {
		s.monName = "wayland"
	}
	d := &daemon{
		t: t, ctx: context.Background(), clk: clock.NewFake(t0),
		mon:     &fakeMonitor{name: s.monName},
		notif:   &fakeNotifier{actions: make(chan notify.ActionEvent)},
		dataDir: s.dataDir, cfgPath: filepath.Join(dir, "config.toml"), socket: s.socket,
	}
	ready := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	o := options{
		dataDir: d.dataDir, configPath: d.cfgPath, socketPath: d.socket, version: "1.0.0-test",
		clk: d.clk, loc: time.UTC, level: new(slog.LevelVar),
		monitor:   func(context.Context, clock.Clock) activity.ActivityMonitor { return d.mon },
		notifier:  func(config.Config, string) (notifier, []string) { return d.notif, nil },
		wrapRepos: s.wrapRepos,
		calendar:  s.calendar,
		llm:       s.llm,
		ready:     ready,
		onLoop:    func(l *loop) { d.loop = l },
	}
	if adjust != nil {
		adjust(&o)
	}
	go func() { done <- run(ctx, o) }()
	require.NoError(t, <-ready)
	var once sync.Once
	var stopErr error
	d.stop = func() error {
		once.Do(func() {
			cancel()
			stopErr = <-done
		})
		return stopErr
	}
	t.Cleanup(func() { d.stop() })
	d.c = client.New(d.socket)
	return d
}

// sync waits until the loop has processed everything handed to it so far.
func (d *daemon) sync() {
	d.t.Helper()
	_, err := d.loop.Do(d.ctx, func(at time.Time) timeengine.Input { return timeengine.Tick{At: at} })
	require.NoError(d.t, err)
}

func (d *daemon) status() *wire.Status {
	d.t.Helper()
	st, err := d.c.Status(d.ctx)
	require.NoError(d.t, err)
	return st
}

func (d *daemon) project(name string) *wire.Project {
	d.t.Helper()
	p, err := d.c.CreateProject(d.ctx, wire.CreateProjectRequest{Name: name})
	require.NoError(d.t, err)
	return p
}

// apiErr asserts err is an API error with code, returning it.
func apiErr(t *testing.T, err error, code string) *client.APIError {
	t.Helper()
	var ae *client.APIError
	require.ErrorAs(t, err, &ae, "want %s", code)
	require.Equal(t, code, ae.Code, ae.Message)
	require.NotEmpty(t, ae.Message)
	return ae
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	return info.Mode().Perm()
}
