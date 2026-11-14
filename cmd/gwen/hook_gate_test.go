package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/client/clienttest"
	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/herdr"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// Claude Code runs `gwen hook` for every event of every session, in Gwen
// Terminal or not (plan.md 3.14.6). Outside Gwen Terminal it must cost
// nothing: exit 0 at once, without reading the event, calling gwend, or
// opening the hook socket. A terminal of a Herdr you run for yourself has
// HC_TERMINAL_ID too, and must be left alone just the same.

// w27bHookEnv gives `gwen hook` folders of its own, so it never reads the
// laptop's config or finds the real gwend, and returns the hook socket's
// path. The runtime folder is short: a Unix socket path has a size limit.
func w27bHookEnv(t *testing.T) (hookSocket string) {
	t.Helper()
	runtime, err := os.MkdirTemp("", "gw")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(runtime) })
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("XDG_RUNTIME_DIR", runtime)
	t.Setenv("GWEN_SANDBOX", "")
	require.NoError(t, os.MkdirAll(filepath.Join(runtime, "gwen"), 0o700))
	return filepath.Join(runtime, "gwen", "hook.sock")
}

// w27bCoreSocket writes a config whose [remote] core_socket is socket.
func w27bCoreSocket(t *testing.T, socket string) {
	t.Helper()
	dir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "gwen")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	cfg := fmt.Sprintf("[remote]\ncore_socket = %q\n", socket)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte(cfg), 0o600))
}

// w27bSetenv sets name to value, or unsets it for the test when value is "".
func w27bSetenv(t *testing.T, name, value string) {
	t.Helper()
	t.Setenv(name, value)
	if value == "" {
		require.NoError(t, os.Unsetenv(name))
	}
}

// w27bBlockedStdin never gives a byte: a hook that tried to read the event
// would hang on it.
type w27bBlockedStdin struct {
	read chan struct{}
	done chan struct{}
}

func (r *w27bBlockedStdin) Read([]byte) (int, error) {
	select {
	case r.read <- struct{}{}:
	default:
	}
	<-r.done
	return 0, io.EOF
}

// w27bRunHook runs `gwen hook` in-process with stdin, and returns what it
// printed and its exit code. It fails the test if the hook does not return.
func w27bRunHook(t *testing.T, stdin io.Reader) (stdout, stderr string, code int, f *clienttest.Fake) {
	t.Helper()
	f = clienttest.New()
	var out, errb bytes.Buffer
	c := &cli{
		newAPI: func(string) client.API { return f },
		stdin:  stdin, stdout: &out, stderr: &errb,
		clk: clock.NewFake(now), loc: time.UTC,
		open: func(string) error { return nil },
	}
	done := make(chan int, 1)
	go func() { done <- c.execute(context.Background(), []string{"hook"}) }()
	select {
	case code = <-done:
	case <-time.After(10 * time.Second): // a failure guard only
		t.Fatal("gwen hook did not return")
	}
	return out.String(), errb.String(), code, f
}

// w27bNoDial reports whether nobody connected to ln: a connection the hook
// made before it returned waits in the listen queue, and a non-blocking
// accept takes it at once, or says the queue is empty.
func w27bNoDial(t *testing.T, ln *net.UnixListener) bool {
	t.Helper()
	raw, err := ln.SyscallConn()
	require.NoError(t, err)
	var (
		fd        int
		acceptErr error
	)
	require.NoError(t, raw.Control(func(lfd uintptr) {
		fd, _, acceptErr = syscall.Accept4(int(lfd), syscall.SOCK_CLOEXEC)
	}))
	if acceptErr == nil {
		syscall.Close(fd)
		return false
	}
	require.ErrorIs(t, acceptErr, syscall.EAGAIN)
	return true
}

func TestGwenHookOutsideGwenTerminalDoesNothing(t *testing.T) {
	cases := []struct {
		name string
		// env sets HC_TERMINAL_ID and HC_TERMINAL_SOCKET; runtime is
		// $XDG_RUNTIME_DIR.
		env func(t *testing.T, runtime string)
	}{
		{"no HC_TERMINAL_ID, as in Ghostty", func(t *testing.T, _ string) {
			w27bSetenv(t, "HC_TERMINAL_ID", "")
			w27bSetenv(t, "HC_TERMINAL_SOCKET", herdr.DefaultSocket())
		}},
		{"no HC_TERMINAL_ID nor HC_TERMINAL_SOCKET", func(t *testing.T, _ string) {
			w27bSetenv(t, "HC_TERMINAL_ID", "")
			w27bSetenv(t, "HC_TERMINAL_SOCKET", "")
		}},
		{"a terminal of a Herdr of your own", func(t *testing.T, runtime string) {
			w27bSetenv(t, "HC_TERMINAL_ID", "t1")
			w27bSetenv(t, "HC_TERMINAL_SOCKET", filepath.Join(runtime, "herdr-core", "default.sock"))
		}},
		{"a terminal that names no core", func(t *testing.T, _ string) {
			w27bSetenv(t, "HC_TERMINAL_ID", "t1")
			w27bSetenv(t, "HC_TERMINAL_SOCKET", "")
		}},
		{"a core other than the one in [remote] core_socket", func(t *testing.T, runtime string) {
			w27bCoreSocket(t, filepath.Join(runtime, "mine", "gwen.sock"))
			w27bSetenv(t, "HC_TERMINAL_ID", "t1")
			w27bSetenv(t, "HC_TERMINAL_SOCKET", filepath.Join(runtime, "herdr-core", "default.sock"))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hookSocket := w27bHookEnv(t)
			tc.env(t, os.Getenv("XDG_RUNTIME_DIR"))
			// A gwend that would see any attempt to reach it.
			ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: hookSocket, Net: "unix"})
			require.NoError(t, err)
			t.Cleanup(func() { ln.Close() })
			// First show that a connection would be seen.
			probe, err := net.Dial("unix", hookSocket)
			require.NoError(t, err)
			require.False(t, w27bNoDial(t, ln))
			probe.Close()
			require.True(t, w27bNoDial(t, ln))

			stdin := &w27bBlockedStdin{read: make(chan struct{}, 1), done: make(chan struct{})}
			t.Cleanup(func() { close(stdin.done) })
			stdout, stderr, code, f := w27bRunHook(t, stdin)
			require.Equal(t, exitOK, code)
			require.Empty(t, stdout)
			require.Empty(t, stderr)
			require.Empty(t, stdin.read, "the event is not even read")
			require.Empty(t, f.Calls(), "gwend's API is never called")
			require.True(t, w27bNoDial(t, ln), "the hook socket is never opened")
		})
	}
}

// w27bFakeGwend serves the hook socket and hands over the body of each
// event posted to it.
func w27bFakeGwend(t *testing.T, hookSocket string) <-chan []byte {
	t.Helper()
	ln, err := net.Listen("unix", hookSocket)
	require.NoError(t, err)
	events := make(chan []byte, 4)
	srv := &http.Server{ReadHeaderTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/code/hook" || err != nil {
			http.NotFound(w, r)
			return
		}
		events <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"output":""}`)
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { srv.Close() })
	return events
}

// The other side of the gate: inside Gwen Terminal (its core is Gwen's own,
// at the default place or at [remote] core_socket) the event reaches gwend,
// with the terminal and Claude's pid added and no prompt text.
func TestGwenHookInGwenTerminalTellsGwend(t *testing.T) {
	cases := []struct {
		name string
		env  func(t *testing.T, runtime string)
	}{
		{"Gwen's own core", func(t *testing.T, _ string) {
			w27bSetenv(t, "HC_TERMINAL_SOCKET", herdr.DefaultSocket())
		}},
		{"the core in [remote] core_socket", func(t *testing.T, runtime string) {
			core := filepath.Join(runtime, "mine", "gwen.sock")
			w27bCoreSocket(t, core)
			w27bSetenv(t, "HC_TERMINAL_SOCKET", core)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hookSocket := w27bHookEnv(t)
			tc.env(t, os.Getenv("XDG_RUNTIME_DIR"))
			w27bSetenv(t, "HC_TERMINAL_ID", "t7")
			events := w27bFakeGwend(t, hookSocket)

			in := `{"hook_event_name":"PreToolUse","session_id":"s1","cwd":"/code","tool_name":"Bash",` +
				`"tool_input":{"command":"go test ./..."},"prompt":"my secret prompt"}`
			stdout, stderr, code, f := w27bRunHook(t, strings.NewReader(in))
			require.Equal(t, exitOK, code)
			require.Empty(t, stdout)
			require.Empty(t, stderr)
			require.Empty(t, f.Calls(), "the hook talks to the hook socket, never the API")

			var body []byte
			select {
			case body = <-events:
			default:
				t.Fatal("the event did not reach gwend")
			}
			require.NotContains(t, string(body), "secret", "a prompt's text is never sent")
			var ev wire.CodeHookEvent
			require.NoError(t, json.Unmarshal(body, &ev))
			require.Equal(t, wire.CodeHookEvent{Event: "PreToolUse", SessionID: "s1", Cwd: "/code", ToolName: "Bash",
				ToolSummary: "go test ./...", Terminal: "t7", PID: os.Getppid()}, ev)
		})
	}
}

// With gwend down, the hook still exits 0 and prints nothing.
func TestGwenHookWithGwendDownStaysQuiet(t *testing.T) {
	w27bHookEnv(t)
	w27bSetenv(t, "HC_TERMINAL_ID", "t7")
	w27bSetenv(t, "HC_TERMINAL_SOCKET", herdr.DefaultSocket())
	stdout, stderr, code, _ := w27bRunHook(t, strings.NewReader(`{"hook_event_name":"Stop","session_id":"s1"}`))
	require.Equal(t, exitOK, code)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
}
