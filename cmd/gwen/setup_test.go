package main

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

// setupRecorder captures the side effects of a setup run.
type setupRecorder struct {
	mu      sync.Mutex
	runs    []string
	starts  []string
	outputs map[string]setupOutput // what each command line prints
	paths   map[string]string      // what each command resolves to
	env     setupEnv
	out     bytes.Buffer
}

// setupOutput is a command's stdout and error.
type setupOutput struct {
	stdout string
	err    error
}

func (r *setupRecorder) ran() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.runs...)
}

// setupNoStdin fails any read: --yes must not prompt.
type setupNoStdin struct{}

func (setupNoStdin) Read([]byte) (int, error) { return 0, errors.New("stdin must not be read") }

func setupTestEnv(t *testing.T, api client.API, stdin string) *setupRecorder {
	t.Helper()
	dir := t.TempDir()
	r := &setupRecorder{}
	in := strings.NewReader(stdin)
	r.env = setupEnv{
		api: func(string) client.API { return api },
		run: func(_ context.Context, name string, args ...string) error {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.runs = append(r.runs, name+" "+strings.Join(args, " "))
			return nil
		},
		output: func(_ context.Context, name string, args ...string) ([]byte, error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			line := name + " " + strings.Join(args, " ")
			r.runs = append(r.runs, line)
			o, ok := r.outputs[line]
			if !ok {
				return nil, errors.New("exec: " + name + ": not found")
			}
			return []byte(o.stdout), o.err
		},
		lookPath: func(command string) (string, error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			if path, ok := r.paths[command]; ok {
				return path, nil
			}
			return "", errors.New("exec: " + command + ": not found")
		},
		start: func(path string) error {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.starts = append(r.starts, path)
			return nil
		},
		stdin:      in,
		stdout:     &r.out,
		clk:        clock.NewFake(testutil.T0),
		configHome: filepath.Join(dir, "config"),
		shareDir:   filepath.Join(dir, "share"),
		exeDir:     "/home/u/.local/bin",
		credDir:    filepath.Join(dir, "credentials"),
	}
	if stdin == "" {
		r.env.stdin = setupNoStdin{}
	}
	return r
}

func setupExec(t *testing.T, r *setupRecorder, args ...string) error {
	t.Helper()
	cmd := setupCommand(func() (setupEnv, error) { return r.env, nil })
	cmd.SetArgs(args)
	cmd.SetOut(&r.out)
	cmd.SetErr(&r.out)
	return cmd.ExecuteContext(context.Background())
}

func setupConfig() *wire.Config {
	c := config.Defaults().Wire()
	return &c
}

func TestSetupService(t *testing.T) {
	t.Parallel()
	fake := clienttest.New().Returns("Health", &wire.Health{OK: true}, nil)
	r := setupTestEnv(t, fake, "")
	require.NoError(t, setupExec(t, r, "service"))
	require.Equal(t, []string{"systemctl --user enable --now gwend.service"}, r.ran())
	require.Contains(t, r.out.String(), "starts at login")

	r = setupTestEnv(t, fake, "")
	require.NoError(t, setupExec(t, r, "service", "--disable"))
	require.Equal(t, []string{"systemctl --user disable --now gwend.service"}, r.ran())
}

func TestSetupServiceWaitsFiveSeconds(t *testing.T) {
	t.Parallel()
	fake := clienttest.New().Returns("Health", nil, client.ErrDaemonNotRunning)
	r := setupTestEnv(t, fake, "")
	clk := r.env.clk.(*clock.Fake)
	done := make(chan error, 1)
	go func() { done <- setupExec(t, r, "service") }()
	for range 20 { // 20 × 250 ms = 5 s of fake time
		clk.BlockUntil(1)
		clk.Advance(250 * time.Millisecond)
	}
	err := <-done
	require.ErrorContains(t, err, "did not answer within 5 s")
	require.Len(t, fake.CallsTo("Health"), 21)
}

func TestSetupTracking(t *testing.T) {
	t.Parallel()
	fake := clienttest.New().Returns("GetConfig", setupConfig(), nil).Returns("PatchConfig", setupConfig(), nil)
	r := setupTestEnv(t, fake, "7h\n\n20m\n")
	require.NoError(t, setupExec(t, r, "tracking"))
	require.Equal(t, wire.ConfigPatch{"tracking": {"daily_target": "7h", "soft_idle": "3m", "hard_idle": "20m"}},
		fake.CallsTo("PatchConfig")[0].Args[0])
	require.Contains(t, r.out.String(), "Daily target [8h]: ")
}

func TestSetupTrackingReportsInvalidValues(t *testing.T) {
	t.Parallel()
	bad := &client.APIError{Status: 400, Code: wire.CodeInvalidRequest, Message: "tracking.soft_idle: must be less than tracking.hard_idle"}
	fake := clienttest.New().Returns("GetConfig", setupConfig(), nil).Returns("PatchConfig", nil, bad)
	r := setupTestEnv(t, fake, "\n30m\n\n")
	require.ErrorIs(t, setupExec(t, r, "tracking"), bad)
}

func TestSetupAutostart(t *testing.T) {
	t.Parallel()
	r := setupTestEnv(t, clienttest.New(), "")
	dst := filepath.Join(r.env.configHome, "autostart", "gwen-tray.desktop")

	// After make install-dev there is no packaged entry: write one for the
	// gwen-tray beside this gwen.
	require.NoError(t, setupExec(t, r, "autostart"))
	b, err := os.ReadFile(dst)
	require.NoError(t, err)
	require.Contains(t, string(b), "Exec=/home/u/.local/bin/gwen-tray\n")
	require.Contains(t, string(b), "X-GNOME-Autostart-enabled=true")
	require.Equal(t, []string{"/home/u/.local/bin/gwen-tray"}, r.starts)

	// A packaged install copies /usr/share/gwen/gwen-tray.desktop.
	require.NoError(t, os.MkdirAll(r.env.shareDir, 0o755))
	packaged, err := os.ReadFile("../../packaging/gwen-tray.desktop")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(r.env.shareDir, "gwen-tray.desktop"), packaged, 0o644))
	require.NoError(t, setupExec(t, r, "autostart"))
	b, err = os.ReadFile(dst)
	require.NoError(t, err)
	require.Equal(t, packaged, b)
	require.Equal(t, "gwen-tray", r.starts[1])

	require.NoError(t, setupExec(t, r, "autostart", "--disable"))
	_, err = os.Stat(dst)
	require.ErrorIs(t, err, fs.ErrNotExist)
	require.NoError(t, setupExec(t, r, "autostart", "--disable"), "safe to run again")
}

func TestSetupPhone(t *testing.T) {
	t.Parallel()
	fake := clienttest.New().
		Returns("GetConfig", setupConfig(), nil).
		Returns("PatchConfig", setupConfig(), nil).
		Returns("NotifyTest", &wire.NotifyTestResult{Desktop: "ok", Phone: "ok"}, nil)
	r := setupTestEnv(t, fake, "")
	require.NoError(t, setupExec(t, r, "phone", "--server", "http://pi.lan:8080", "--fallback", "https://ntfy.sh", "--token", "tk_x"))

	patch := fake.CallsTo("PatchConfig")[0].Args[0].(wire.ConfigPatch)
	topic := patch["ntfy"]["topic"].(string)
	require.Regexp(t, `^gwen-[a-z2-7]{24}$`, topic, "a topic is generated when none is set")
	require.Equal(t, "http://pi.lan:8080", patch["ntfy"]["server"])
	require.Equal(t, "https://ntfy.sh", patch["ntfy"]["fallback_server"])
	require.Equal(t, true, patch["nudge"]["phone"])

	token, err := config.ReadCredentialLine(r.env.credDir, config.CredNtfyToken)
	require.NoError(t, err)
	require.Equal(t, "tk_x", token)
	out := r.out.String()
	require.Contains(t, out, "http://pi.lan:8080/"+topic)
	require.Contains(t, out, "https://ntfy.sh/"+topic)
	require.Contains(t, out, "Test notification: desktop ok, phone ok.")
	require.Len(t, fake.CallsTo("NotifyTest"), 1)
}

func TestSetupPhoneKeepsAnExistingTopic(t *testing.T) {
	t.Parallel()
	cfg := setupConfig()
	cfg.Ntfy.Topic = "gwen-existingtopicexistingtop"
	fake := clienttest.New().Returns("GetConfig", cfg, nil).Returns("PatchConfig", cfg, nil).
		Returns("NotifyTest", &wire.NotifyTestResult{Desktop: "ok", Phone: "ok"}, nil)
	r := setupTestEnv(t, fake, "\n\n")
	require.NoError(t, setupExec(t, r, "phone"))
	patch := fake.CallsTo("PatchConfig")[0].Args[0].(wire.ConfigPatch)
	require.Equal(t, cfg.Ntfy.Topic, patch["ntfy"]["topic"])
	require.Equal(t, "https://ntfy.sh", patch["ntfy"]["server"], "the default")
	_, err := os.Stat(filepath.Join(r.env.credDir, config.CredNtfyToken))
	require.ErrorIs(t, err, fs.ErrNotExist, "no token given, none written")
}

func TestSetupYesRunsEveryStepWithoutReadingStdin(t *testing.T) {
	t.Parallel()
	fake := clienttest.New().
		Returns("Health", &wire.Health{OK: true}, nil).
		Returns("GetConfig", setupConfig(), nil).
		Returns("PatchConfig", setupConfig(), nil).
		Returns("NotifyTest", &wire.NotifyTestResult{Desktop: "ok", Phone: "ok"}, nil)
	r := setupTestEnv(t, fake, "") // stdin fails on read
	require.NoError(t, setupExec(t, r, "--yes"))
	require.Equal(t, []string{"systemctl --user enable --now gwend.service"}, r.ran())
	require.Len(t, fake.CallsTo("PatchConfig"), 2, "tracking and phone")
	require.Len(t, r.starts, 1, "the tray")
	out := r.out.String()
	require.Contains(t, out, "Daily target: 8h")
	require.Contains(t, out, "Gwen is set up.")
	require.Contains(t, out, "Tray:       starts at login")
}

func TestSetupStopsAtTheFirstFailure(t *testing.T) {
	t.Parallel()
	fake := clienttest.New()
	r := setupTestEnv(t, fake, "")
	r.env.run = func(context.Context, string, ...string) error { return errors.New("systemctl: no user session") }
	require.ErrorContains(t, setupExec(t, r, "--yes"), "no user session")
	require.Empty(t, fake.Calls())
}

func TestSetupTrayEntryMatchesThePackagedOne(t *testing.T) {
	t.Parallel()
	packaged, err := os.ReadFile("../../packaging/gwen-tray.desktop")
	require.NoError(t, err)
	require.Equal(t, string(packaged), strings.Replace(setupTrayEntry("/x/gwen-tray"), "Exec=/x/gwen-tray", "Exec=gwen-tray", 1))
}

func TestSetupCommandTree(t *testing.T) {
	t.Parallel()
	cmd := newSetupCmd()
	require.Equal(t, "setup", cmd.Name())
	var names []string
	for _, c := range cmd.Commands() {
		names = append(names, c.Name())
	}
	require.ElementsMatch(t, []string{"service", "tracking", "autostart", "phone", "calendar", "llm", "sync"}, names)
	require.NotNil(t, cmd.PersistentFlags().Lookup("yes"))

	env, err := setupDefaultEnv()
	require.NoError(t, err)
	require.Equal(t, "/usr/share/gwen", env.shareDir)
	require.NotEmpty(t, env.configHome)
}
