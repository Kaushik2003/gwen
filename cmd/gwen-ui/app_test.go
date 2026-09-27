package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/client/clienttest"
	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

func TestAppMirrorsTheClient(t *testing.T) {
	t.Parallel()
	api := reflect.TypeFor[client.API]()
	app := reflect.TypeFor[*App]()
	ctx := reflect.TypeFor[context.Context]()
	for i := range api.NumMethod() {
		m := api.Method(i)
		if m.Name == "Events" {
			continue
		}
		got, ok := app.MethodByName(m.Name)
		require.True(t, ok, "App has no %s", m.Name)
		var want []reflect.Type
		for j := range m.Type.NumIn() {
			if in := m.Type.In(j); in != ctx {
				want = append(want, in)
			}
		}
		var have []reflect.Type
		for j := 1; j < got.Type.NumIn(); j++ { // skip the receiver
			have = append(have, got.Type.In(j))
		}
		require.Equal(t, want, have, "%s parameters", m.Name)
		for j := range m.Type.NumOut() {
			require.Equal(t, m.Type.Out(j), got.Type.Out(j), "%s results", m.Name)
		}
	}
}

func TestAppPassesCallsThrough(t *testing.T) {
	t.Parallel()
	pid := "p1"
	f := clienttest.New().
		Returns("ClockIn", &wire.Status{State: wire.StateWorking}, nil).
		Returns("DeleteTask", nil, nil)
	a := &App{ctx: context.Background(), api: f}
	st, err := a.ClockIn(wire.ClockInRequest{ProjectID: &pid})
	require.NoError(t, err)
	require.Equal(t, wire.StateWorking, st.State)
	require.Equal(t, []any{wire.ClockInRequest{ProjectID: &pid}}, f.CallsTo("ClockIn")[0].Args)
	require.NoError(t, a.DeleteTask("t1"))
	require.Equal(t, []any{"t1"}, f.CallsTo("DeleteTask")[0].Args)
}

func TestAppErrorsAreJSON(t *testing.T) {
	t.Parallel()
	decode := func(err error) hostError {
		t.Helper()
		var he hostError
		require.NoError(t, json.Unmarshal([]byte(err.Error()), &he))
		return he
	}
	f := clienttest.New().
		Returns("PatchConfig", nil, &client.APIError{Status: 400, Code: wire.CodeInvalidRequest,
			Message: "tracking.soft_idle: must be less than tracking.hard_idle", Details: map[string]any{"key": "tracking.soft_idle"}}).
		Returns("Status", nil, client.ErrDaemonNotRunning).
		Returns("Health", nil, errors.New("boom"))
	a := &App{ctx: context.Background(), api: f}

	_, err := a.PatchConfig(wire.ConfigPatch{"tracking": {"soft_idle": "1h"}})
	require.Equal(t, hostError{Code: wire.CodeInvalidRequest, Message: "tracking.soft_idle: must be less than tracking.hard_idle",
		Details: map[string]any{"key": "tracking.soft_idle"}}, decode(err))
	_, err = a.Status()
	require.Equal(t, codeDaemonNotRunning, decode(err).Code)
	_, err = a.Health()
	require.Equal(t, hostError{Code: wire.CodeInternal, Message: "boom", Details: map[string]any{}}, decode(err))
	require.NoError(t, wrap(nil))
}

// runner records commands instead of running them.
type runner struct {
	mu   sync.Mutex
	runs [][]string
	err  error
}

func (r *runner) run(_ context.Context, name string, args ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs = append(r.runs, append([]string{name}, args...))
	return r.err
}

func (r *runner) start(name string, args ...string) error {
	return r.run(context.Background(), name, args...)
}

func (r *runner) all() [][]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([][]string(nil), r.runs...)
}

func newHost(t *testing.T, api client.API) (*host, *runner, *clock.Fake) {
	dir := t.TempDir()
	r := &runner{}
	clk := clock.NewFake(testutil.T0)
	return &host{api: api, clk: clk, run: r.run, start: r.start,
		configHome: filepath.Join(dir, "config"), shareDir: filepath.Join(dir, "share"),
		exeDir: filepath.Join(dir, "bin"), credDir: filepath.Join(dir, "credentials")}, r, clk
}

func TestSetCredential(t *testing.T) {
	t.Parallel()
	h, _, _ := newHost(t, clienttest.New())
	require.NoError(t, h.setCredential(config.CredLLMAPIKey, "sk-test"))
	got, err := config.ReadCredentialLine(h.credDir, config.CredLLMAPIKey)
	require.NoError(t, err)
	require.Equal(t, "sk-test", got)
	info, err := os.Stat(filepath.Join(h.credDir, config.CredLLMAPIKey))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	require.ErrorContains(t, h.setCredential("../gwen.toml", "x"), "not a credential")
}

func TestOpenURL(t *testing.T) {
	t.Parallel()
	h, r, _ := newHost(t, clienttest.New())
	require.NoError(t, h.openURL("https://accounts.google.com/o/oauth2/auth?x=1"))
	require.Error(t, h.openURL("file:///etc/passwd"))
	require.Error(t, h.openURL("javascript:alert(1)"))
	require.Equal(t, [][]string{{"xdg-open", "https://accounts.google.com/o/oauth2/auth?x=1"}}, r.all())
}

func TestEnableServiceWaitsForTheDaemon(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	up := false
	f := clienttest.New().On("Health", func(...any) (any, error) {
		mu.Lock()
		defer mu.Unlock()
		if !up {
			return nil, client.ErrDaemonNotRunning
		}
		return &wire.Health{OK: true}, nil
	})
	h, r, clk := newHost(t, f)
	done := make(chan error, 1)
	go func() { done <- h.enableService(context.Background()) }()
	clk.BlockUntil(1)
	clk.Advance(250 * time.Millisecond)
	clk.BlockUntil(1)
	mu.Lock()
	up = true
	mu.Unlock()
	clk.Advance(250 * time.Millisecond)
	require.NoError(t, <-done)
	require.Equal(t, [][]string{{"systemctl", "--user", "enable", "--now", "gwend.service"}}, r.all())
}

func TestEnableServiceGivesUp(t *testing.T) {
	t.Parallel()
	h, _, clk := newHost(t, clienttest.New().Returns("Health", nil, client.ErrDaemonNotRunning))
	done := make(chan error, 1)
	go func() { done <- h.enableService(context.Background()) }()
	for range 20 {
		clk.BlockUntil(1)
		clk.Advance(250 * time.Millisecond)
	}
	require.ErrorContains(t, <-done, "did not answer within 5 s")
}

func TestEnableServiceReportsSystemctl(t *testing.T) {
	t.Parallel()
	h, r, _ := newHost(t, clienttest.New())
	r.err = errors.New("systemctl: exit status 1: Unit gwend.service not found.")
	require.ErrorContains(t, h.enableService(context.Background()), "not found")
}

func TestAutostart(t *testing.T) {
	t.Parallel()
	h, r, _ := newHost(t, clienttest.New())
	require.False(t, h.autostart())

	// Installed from a package: the packaged entry is copied.
	packaged := "[Desktop Entry]\nExec=gwen-tray\n"
	require.NoError(t, os.MkdirAll(h.shareDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(h.shareDir, "gwen-tray.desktop"), []byte(packaged), 0o644))
	require.NoError(t, h.setAutostart(true))
	require.True(t, h.autostart())
	got, err := os.ReadFile(h.autostartPath())
	require.NoError(t, err)
	require.Equal(t, packaged, string(got))

	require.NoError(t, h.setAutostart(false))
	require.False(t, h.autostart())
	require.NoError(t, h.setAutostart(false), "removing twice is fine")

	// After make install-dev: an entry for the tray beside gwen-ui.
	require.NoError(t, os.Remove(filepath.Join(h.shareDir, "gwen-tray.desktop")))
	require.NoError(t, h.setAutostart(true))
	got, err = os.ReadFile(h.autostartPath())
	require.NoError(t, err)
	tray := filepath.Join(h.exeDir, "gwen-tray")
	require.Contains(t, string(got), "Exec="+tray+"\n")
	require.Equal(t, [][]string{{"gwen-tray"}, {tray}}, r.all())
}
