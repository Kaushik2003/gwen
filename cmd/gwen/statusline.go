package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/remote"
	"github.com/kzark/gwen/internal/wire"
	"github.com/spf13/cobra"
)

// `gwen statusline` is the status line of each Claude session Gwen starts
// (plan.md 3.15). Claude Code gives a status line the only copy of your
// usage limits, so this sends them to Gwen for the sidebar's meter, then runs
// your own status line with the same input and prints what it prints. Like
// `gwen hook` it never gets in Claude's way: it always exits 0, and when Gwen
// is not running your status line still shows.

const (
	statusInputLimit = 1 << 20
	statusPostWait   = 300 * time.Millisecond
	statusOwnWait    = 10 * time.Second
	// statusNested is set for your own status line, so a settings file that
	// names `gwen statusline` itself does not run it again and again.
	statusNested = "GWEN_STATUSLINE"
)

func (c *cli) statusLineCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "statusline",
		Short:             "Claude Code's status line in sessions Gwen starts (Claude Code runs this)",
		Hidden:            true,
		Args:              cobra.ArbitraryArgs,
		PersistentPreRunE: func(*cobra.Command, []string) error { return nil },
		RunE: func(cmd *cobra.Command, _ []string) error {
			runStatusLine(cmd.Context(), c.stdin, c.stdout)
			return nil
		},
	}
}

func runStatusLine(ctx context.Context, stdin io.Reader, stdout io.Writer) {
	if os.Getenv(statusNested) != "" || os.Getenv("GWEN_AGENT") != "" {
		return
	}
	data, err := io.ReadAll(io.LimitReader(stdin, statusInputLimit))
	if err != nil {
		return
	}
	var in struct {
		Cwd       string `json:"cwd"`
		Workspace struct {
			CurrentDir string `json:"current_dir"`
			ProjectDir string `json:"project_dir"`
		} `json:"workspace"`
		RateLimits struct {
			FiveHour *statusWindow `json:"five_hour"`
			SevenDay *statusWindow `json:"seven_day"`
		} `json:"rate_limits"`
	}
	_ = json.Unmarshal(data, &in)
	var wg sync.WaitGroup
	if usage := statusUsage(in.RateLimits.FiveHour, in.RateLimits.SevenDay); usage != nil {
		wg.Go(func() { postStatusUsage(ctx, usage) })
	}
	dir := in.Workspace.ProjectDir
	if dir == "" {
		dir = in.Cwd
	}
	runOwnStatusLine(ctx, dir, data, stdout)
	wg.Wait()
}

// statusWindow is one limit as Claude Code gives it: resets_at is in Unix
// seconds.
type statusWindow struct {
	UsedPercentage *float64 `json:"used_percentage"`
	ResetsAt       *float64 `json:"resets_at"`
}

func (w *statusWindow) wire() *wire.UsageWindow {
	if w == nil || w.UsedPercentage == nil {
		return nil
	}
	out := &wire.UsageWindow{UsedPercent: *w.UsedPercentage}
	if w.ResetsAt != nil && *w.ResetsAt > 0 {
		out.ResetsAt = int64(*w.ResetsAt * 1000)
	}
	return out
}

// statusUsage is the reading to send, nil when Claude Code gave none (an API
// key instead of a plan, or before the first answer).
func statusUsage(five, week *statusWindow) *wire.ClaudeUsage {
	u := &wire.ClaudeUsage{FiveHour: five.wire(), SevenDay: week.wire()}
	if u.FiveHour == nil && u.SevenDay == nil {
		return nil
	}
	return u
}

// postStatusUsage sends the reading to gwend's hook socket from inside the
// terminal, as `gwen hook` does, and gives up quickly.
func postStatusUsage(ctx context.Context, usage *wire.ClaudeUsage) {
	terminal := os.Getenv("HC_TERMINAL_ID")
	if terminal == "" || !inGwenTerminal() {
		return
	}
	paths, err := config.DefaultPaths()
	if err != nil {
		return
	}
	body, err := json.Marshal(wire.CodeHookEvent{Event: wire.HookStatusLine, Terminal: terminal, PID: os.Getppid(), Usage: usage,
		Sandbox: os.Getenv("GWEN_SANDBOX") == "1"})
	if err != nil {
		return
	}
	socket := filepath.Join(paths.RuntimeDir, "hook.sock")
	hc := &http.Client{Timeout: statusPostWait, Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: statusPostWait}
			return d.DialContext(ctx, "unix", socket)
		},
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://gwend/v1/code/hook", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if resp, err := hc.Do(req); err == nil {
		resp.Body.Close()
	}
}

// runOwnStatusLine runs the status line your settings name for dir with the
// same input, and copies what it prints.
func runOwnStatusLine(ctx context.Context, dir string, input []byte, stdout io.Writer) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	own := remote.OwnStatusLine(home, dir)
	command, _ := own["command"].(string)
	if typ, _ := own["type"].(string); typ != "command" || strings.TrimSpace(command) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, statusOwnWait)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		cmd.Dir = dir // as Claude Code runs it, in the session's folder
	}
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = stdout
	cmd.Env = append(os.Environ(), statusNested+"=1")
	_ = cmd.Run()
}
