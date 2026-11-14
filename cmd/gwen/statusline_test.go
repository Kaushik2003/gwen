package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kzark/gwen/internal/herdr"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// statusInput is what Claude Code gives a status line, cut down.
const statusInput = `{"cwd":"/code","model":{"display_name":"Opus"},` +
	`"rate_limits":{"five_hour":{"used_percentage":42.5,"resets_at":1794560000},"seven_day":{"used_percentage":12}}}`

// statusOwn sets your own status line in the test's home.
func statusOwn(t *testing.T, command string) {
	t.Helper()
	dir := filepath.Join(os.Getenv("HOME"), ".claude")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	b, err := json.Marshal(map[string]any{"statusLine": map[string]any{"type": "command", "command": command}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "settings.json"), b, 0o644))
}

// In a terminal of Gwen's core it sends the limits to gwend, and either way
// it prints what your own status line prints for the same input.
func TestStatusLineSendsTheLimitsAndShowsYours(t *testing.T) {
	hookSocket := w27bHookEnv(t)
	w27bSetenv(t, "CLAUDE_CONFIG_DIR", "")
	w27bSetenv(t, statusNested, "")
	w27bSetenv(t, "HC_TERMINAL_SOCKET", herdr.DefaultSocket())
	w27bSetenv(t, "HC_TERMINAL_ID", "t7")
	events := w27bFakeGwend(t, hookSocket)
	statusOwn(t, `jq -r .model.display_name; echo "$GWEN_STATUSLINE"`)

	var out bytes.Buffer
	runStatusLine(t.Context(), strings.NewReader(statusInput), &out)
	require.Equal(t, "Opus\n1\n", out.String(), "your status line, told it runs inside Gwen's")

	var ev wire.CodeHookEvent
	select {
	case body := <-events:
		require.NoError(t, json.Unmarshal(body, &ev))
	default:
		t.Fatal("the limits did not reach gwend")
	}
	require.Equal(t, wire.CodeHookEvent{Event: wire.HookStatusLine, Terminal: "t7", PID: os.Getppid(), Usage: &wire.ClaudeUsage{
		FiveHour: &wire.UsageWindow{UsedPercent: 42.5, ResetsAt: 1_794_560_000_000}, SevenDay: &wire.UsageWindow{UsedPercent: 12}}}, ev)
}

// Outside Gwen's core, without limits (an API key), or with gwend down, it
// only shows your status line; with none set it prints nothing.
func TestStatusLineWithoutGwen(t *testing.T) {
	hookSocket := w27bHookEnv(t)
	w27bSetenv(t, "CLAUDE_CONFIG_DIR", "")
	w27bSetenv(t, statusNested, "")
	w27bSetenv(t, "HC_TERMINAL_ID", "t7")
	w27bSetenv(t, "HC_TERMINAL_SOCKET", "/run/user/1000/herdr-core/default.sock") // your own Herdr
	events := w27bFakeGwend(t, hookSocket)

	var out bytes.Buffer
	runStatusLine(t.Context(), strings.NewReader(statusInput), &out)
	require.Empty(t, out.String(), "no status line of yours")

	statusOwn(t, "cat >/dev/null; echo mine")
	out.Reset()
	runStatusLine(t.Context(), strings.NewReader(statusInput), &out)
	require.Equal(t, "mine\n", out.String())
	select {
	case <-events:
		t.Fatal("a terminal of your own Herdr told gwend")
	default:
	}

	// A settings file that names gwen statusline itself does not loop.
	w27bSetenv(t, statusNested, "1")
	out.Reset()
	runStatusLine(t.Context(), strings.NewReader(statusInput), &out)
	require.Empty(t, out.String())
}
