package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/kzark/gwen/internal/config"
)

// claudeCode runs the Claude Code CLI in print mode, so the user's Claude
// subscription pays instead of an API key (docs/07-integrations.md#claude-code).
type claudeCode struct {
	path  string // the CLI
	model string
	// scope is systemd-run when the CLI must run in a systemd scope of its
	// own, outside the memory limits of gwend.service; empty runs it directly.
	scope string
	dir   string
}

// apiKeyEnv are the variables that would make the CLI bill an API key rather
// than the subscription; the CLI never sees them.
var apiKeyEnv = []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"}

// maxOutput bounds what is kept of the CLI's stdout and stderr.
const maxOutput = 4 << 20

func newClaudeCode(cfg config.LLM) (claudeCode, error) {
	path, err := FindCommand(cfg.Command)
	if err != nil {
		return claudeCode{}, &unavailable{fmt.Sprintf("the Claude Code CLI %q was not found; install Claude Code, "+
			"or give its path with gwen setup llm", cfg.Command)}
	}
	c := claudeCode{path: path, model: cfg.Model, dir: os.TempDir()}
	if os.Getenv("INVOCATION_ID") != "" { // set by systemd for the units it runs
		if sr, err := exec.LookPath("systemd-run"); err == nil {
			c.scope = sr
		}
	}
	return c, nil
}

// FindCommand resolves llm.command: an absolute path is used as it is, and a
// bare name is looked up on PATH and then in ~/.local/bin, where the Claude
// Code installer puts claude.
func FindCommand(command string) (string, error) {
	path, err := exec.LookPath(command)
	if err == nil || strings.ContainsRune(command, '/') || !errors.Is(err, exec.ErrNotFound) {
		return path, err
	}
	home, herr := os.UserHomeDir()
	if herr != nil {
		return "", err
	}
	return exec.LookPath(filepath.Join(home, ".local", "bin", command))
}

// cliResult is the one JSON object the CLI prints with --output-format json.
type cliResult struct {
	Type             string          `json:"type"`
	IsError          bool            `json:"is_error"`
	Result           string          `json:"result"`
	StructuredOutput json.RawMessage `json:"structured_output"`
}

func (c claudeCode) complete(ctx context.Context, p prompt, user string) (string, error) {
	args := []string{"-p", "--output-format", "json"}
	if c.model != "" {
		args = append(args, "--model", c.model)
	}
	args = append(args, "--system-prompt", p.system, "--json-schema", p.schema, "--tools", "",
		"--strict-mcp-config", "--setting-sources", "", "--disable-slash-commands", "--no-session-persistence")
	name := c.path
	if c.scope != "" {
		name, args = c.scope, append([]string{"--user", "--scope", "--quiet", "--collect",
			"--description=Gwen LLM request", "--", c.path}, args...)
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = c.dir
	// The daemon's environment, with PWD matching Dir, less the API keys.
	cmd.Env = slices.DeleteFunc(cmd.Environ(), func(kv string) bool {
		key, _, _ := strings.Cut(kv, "=")
		return slices.Contains(apiKeyEnv, key)
	})
	cmd.Stdin = strings.NewReader(user)
	stdout, stderr := &capped{max: maxOutput}, &capped{max: maxOutput}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	// The CLI and anything it starts share a process group, killed together.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	runErr := cmd.Run()
	if err := ctx.Err(); err != nil {
		return "", err
	}

	var res cliResult
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &res); err != nil || res.Type != "result" {
		if line := lastLine(stderr.String()); line != "" {
			return "", fmt.Errorf("claude_code: %s", line)
		}
		if runErr == nil {
			runErr = errors.New("the CLI's output is not its result JSON")
		}
		return "", fmt.Errorf("claude_code: %w", runErr)
	}
	switch {
	case res.IsError:
		msg := strings.TrimSpace(res.Result)
		if msg == "" {
			msg = "the CLI reported an error"
		}
		return "", fmt.Errorf("claude_code: %s", msg)
	case len(res.StructuredOutput) > 0 && string(res.StructuredOutput) != "null":
		return string(res.StructuredOutput), nil
	case strings.TrimSpace(res.Result) != "":
		return res.Result, nil
	}
	return "", errors.New("claude_code: the reply had no text")
}

// lastLine is the last non-blank line of s, trimmed.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// capped keeps the first max bytes written to it and discards the rest, so a
// runaway CLI cannot grow the daemon.
type capped struct {
	bytes.Buffer
	max int
}

func (c *capped) Write(p []byte) (int, error) {
	if room := c.max - c.Len(); room > 0 {
		c.Buffer.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}
