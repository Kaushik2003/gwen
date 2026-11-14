package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/herdr"
	"github.com/kzark/gwen/internal/wire"
	"github.com/spf13/cobra"
)

// `gwen hook` is what Claude Code runs for the events Gwen listens to
// (plan.md 3.14.6). It must never get in Claude's way: it prints nothing,
// always exits 0, and outside Gwen Terminal it does nothing at all. The one
// thing it may print is the decision of a permission request the phone
// answered.

const (
	hookInputLimit = 1 << 20
	hookPostWait   = 500 * time.Millisecond
	// A held permission waits for the phone for up to ten minutes; Claude
	// Code gives this hook fifteen.
	hookPermissionWait = 14 * time.Minute
	hookToolInputLimit = 8 << 10
)

func (c *cli) hookCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "hook",
		Short:  "Tell Gwen about a Claude Code event (Claude Code runs this)",
		Hidden: true,
		Args:   cobra.ArbitraryArgs,
		// The root's setup needs nothing from here, and a failure there must
		// not reach Claude Code.
		PersistentPreRunE: func(*cobra.Command, []string) error { return nil },
		RunE: func(cmd *cobra.Command, _ []string) error {
			runHook(cmd.Context(), c.stdin, c.stdout)
			return nil
		},
	}
}

func runHook(ctx context.Context, stdin io.Reader, stdout io.Writer) {
	terminal := os.Getenv("HC_TERMINAL_ID")
	// Gwen's own agent reports to her directly, never through hooks (plan.md 3.16).
	if terminal == "" || os.Getenv("GWEN_AGENT") != "" || !inGwenTerminal() {
		return
	}
	data, err := io.ReadAll(io.LimitReader(stdin, hookInputLimit))
	if err != nil {
		return
	}
	var in struct {
		Event          string           `json:"hook_event_name"`
		SessionID      string           `json:"session_id"`
		TranscriptPath string           `json:"transcript_path"`
		Cwd            string           `json:"cwd"`
		PermissionMode string           `json:"permission_mode"`
		ToolName       string           `json:"tool_name"`
		ToolInput      map[string]any   `json:"tool_input"`
		LastMessage    string           `json:"last_assistant_message"`
		Source         string           `json:"source"`
		Reason         string           `json:"reason"`
		Suggestions    []map[string]any `json:"permission_suggestions"`
	}
	if json.Unmarshal(data, &in) != nil || in.Event == "" {
		return
	}
	ev := wire.CodeHookEvent{Event: in.Event, SessionID: in.SessionID, TranscriptPath: in.TranscriptPath, Cwd: in.Cwd,
		PermissionMode: in.PermissionMode, ToolName: in.ToolName, LastMessage: clip(in.LastMessage, 300), Source: in.Source,
		Reason: in.Reason, Terminal: terminal, PID: os.Getppid(), Sandbox: os.Getenv("GWEN_SANDBOX") == "1"}
	for _, key := range []string{"command", "file_path", "notebook_path"} {
		if s, ok := in.ToolInput[key].(string); ok && s != "" {
			ev.ToolSummary = clip(s, 200)
			break
		}
	}
	wait := hookPostWait
	if in.Event == "PermissionRequest" || in.ToolName == "AskUserQuestion" {
		// Only these carry the whole tool input: Gwen shows it to you on the
		// phone. A prompt's text is never sent.
		if raw, err := json.Marshal(in.ToolInput); err == nil && len(raw) <= hookToolInputLimit {
			ev.ToolInput = in.ToolInput
		}
		ev.Suggestions = in.Suggestions
	}
	if in.Event == "PermissionRequest" {
		wait = hookPermissionWait
	}
	paths, err := config.DefaultPaths()
	if err != nil {
		return
	}
	body, err := json.Marshal(ev)
	if err != nil {
		return
	}
	socket := filepath.Join(paths.RuntimeDir, "hook.sock")
	hc := &http.Client{Timeout: wait, Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: hookPostWait}
			return d.DialContext(ctx, "unix", socket)
		},
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://gwend/v1/code/hook", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	var reply wire.CodeHookReply
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&reply) == nil && reply.Output != "" {
		_, _ = io.WriteString(stdout, reply.Output+"\n")
	}
}

// inGwenTerminal reports whether this terminal belongs to Gwen's own core.
// A terminal of a Herdr you run for yourself has HC_TERMINAL_ID too, and
// is none of Gwen's business.
func inGwenTerminal() bool {
	socket := os.Getenv("HC_TERMINAL_SOCKET")
	if socket == "" {
		return false
	}
	if socket == herdr.DefaultSocket() {
		return true
	}
	// A core socket set in [remote] core_socket.
	paths, err := config.DefaultPaths()
	if err != nil {
		return false
	}
	cfg, err := config.Load(paths.ConfigFile)
	return err == nil && cfg.Remote.CoreSocket != "" && socket == cfg.Remote.CoreSocket
}

// clip cuts s to at most n characters.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
