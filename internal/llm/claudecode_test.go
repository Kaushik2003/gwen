package llm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/llm"
	"github.com/stretchr/testify/require"
)

// fakeCLI is a stand-in for the Claude Code CLI: a shell script that records
// how it was run into dir and replies with what the test put there.
type fakeCLI struct {
	t    *testing.T
	dir  string
	path string
}

func newFakeCLI(t *testing.T) *fakeCLI {
	t.Helper()
	dir := t.TempDir()
	f := &fakeCLI{t: t, dir: dir, path: filepath.Join(dir, "claude")}
	script := fmt.Sprintf(`#!/bin/sh
d=%q
printf '%%s\0' "$@" > "$d/args"
cat > "$d/stdin"
pwd -P > "$d/cwd"
printf '%%s' "${ANTHROPIC_API_KEY-unset} ${ANTHROPIC_AUTH_TOKEN-unset} ${GWEN_TEST_KEPT-unset} $PWD" > "$d/env"
[ -f "$d/hang" ] && exec sleep 30
[ -f "$d/stderr" ] && cat "$d/stderr" >&2
[ -f "$d/reply" ] && cat "$d/reply"
exit $(cat "$d/exit" 2>/dev/null || echo 0)
`, dir)
	require.NoError(t, os.WriteFile(f.path, []byte(script), 0o755))
	return f
}

func (f *fakeCLI) write(name, content string) {
	f.t.Helper()
	require.NoError(f.t, os.WriteFile(filepath.Join(f.dir, name), []byte(content), 0o644))
}

// reply makes the CLI print a result object.
func (f *fakeCLI) reply(res map[string]any) {
	f.t.Helper()
	res["type"] = "result"
	b, err := json.Marshal(res)
	require.NoError(f.t, err)
	f.write("reply", string(b)+"\n")
}

func (f *fakeCLI) read(name string) string {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.dir, name))
	require.NoError(f.t, err)
	return string(b)
}

func (f *fakeCLI) args() []string {
	return strings.Split(strings.TrimSuffix(f.read("args"), "\x00"), "\x00")
}

func (f *fakeCLI) adapter(timeout time.Duration) llm.Planner {
	f.t.Helper()
	cfg := config.Defaults().LLM
	cfg.Provider, cfg.Command = config.ProviderClaudeCode, f.path
	if timeout > 0 {
		cfg.Timeout = timeout
	}
	a, err := llm.New(cfg, llm.Options{CredDir: withKey(f.t, "")})
	require.NoError(f.t, err)
	return a
}

// notUnderSystemd clears INVOCATION_ID for the test, as outside a unit.
func notUnderSystemd(t *testing.T) {
	t.Setenv("INVOCATION_ID", "")
	os.Unsetenv("INVOCATION_ID")
}

func TestClaudeCodeInvocation(t *testing.T) {
	notUnderSystemd(t)
	t.Setenv("ANTHROPIC_API_KEY", "sk-must-not-reach-the-cli")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "must-not-either")
	t.Setenv("GWEN_TEST_KEPT", "kept")
	f := newFakeCLI(t)
	var tasks any
	require.NoError(t, json.Unmarshal([]byte(goodBreakdown), &tasks))
	f.reply(map[string]any{"subtype": "success", "is_error": false, "result": "not this", "structured_output": tasks})
	a := f.adapter(0)
	require.Equal(t, "claude_code", a.Name())

	out, err := a.Breakdown(context.Background(), goalReq())
	require.NoError(t, err)
	require.Len(t, out.Tasks, 2, "structured_output is preferred over result")
	require.Equal(t, "Graphs: BFS set", out.Tasks[0].Title)

	args := f.args()
	require.Len(t, args, 16)
	require.Equal(t, []string{"-p", "--output-format", "json", "--model", "claude-sonnet-5", "--system-prompt"}, args[:6])
	require.Contains(t, args[6], llm.BreakdownSchema, "the prompt carries the schema verbatim")
	require.Equal(t, []string{"--json-schema", llm.BreakdownSchema, "--tools", "", "--strict-mcp-config",
		"--setting-sources", "", "--disable-slash-commands", "--no-session-persistence"}, args[7:])

	var sent map[string]any
	require.NoError(t, json.Unmarshal([]byte(f.read("stdin")), &sent))
	require.Equal(t, "300 problems", sent["title"])
	require.Equal(t, "focus on graphs", sent["instructions"])
	require.Len(t, sent, 13, "the breakdown fields and nothing else")

	tmp, err := filepath.EvalSymlinks(os.TempDir())
	require.NoError(t, err)
	require.Equal(t, tmp, strings.TrimSpace(f.read("cwd")))
	require.Equal(t, "unset unset kept "+os.TempDir(), f.read("env"), "no API keys; PWD matches the directory")
	_, err = os.Stat(filepath.Join(f.dir, "scope-args"))
	require.ErrorIs(t, err, os.ErrNotExist)

	f.reply(map[string]any{"is_error": false, "result": `{"markdown": "A good week."}`})
	retro, err := a.Retro(context.Background(), llm.RetroRequest{WeekStart: "2026-09-14"})
	require.NoError(t, err)
	require.Equal(t, llm.RetroOutput{Markdown: "A good week.", GeneratedBy: llm.GeneratedByLLM}, retro,
		"result is used when there is no structured_output")
	require.Equal(t, llm.RetroSchema, f.args()[8])
}

func TestClaudeCodeRunsInAScopeUnderSystemd(t *testing.T) {
	t.Setenv("INVOCATION_ID", "4b0d4c4a1f6e4f0b9e0d3c2b1a090807")
	f := newFakeCLI(t)
	bin := t.TempDir()
	scope := fmt.Sprintf(`#!/bin/sh
printf '%%s\0' "$@" > %q
while [ "$1" != "--" ]; do shift; done
shift
exec "$@"
`, filepath.Join(f.dir, "scope-args"))
	require.NoError(t, os.WriteFile(filepath.Join(bin, "systemd-run"), []byte(scope), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	f.reply(map[string]any{"is_error": false, "result": goodBreakdown})

	_, err := f.adapter(0).Breakdown(context.Background(), goalReq())
	require.NoError(t, err)
	got := strings.Split(strings.TrimSuffix(f.read("scope-args"), "\x00"), "\x00")
	require.Equal(t, []string{"--user", "--scope", "--quiet", "--collect", "--description=Gwen LLM request", "--",
		f.path, "-p"}, got[:8])
	require.Equal(t, got[7:], f.args(), "the CLI gets the same arguments inside the scope")
}

func TestClaudeCodeFailures(t *testing.T) {
	notUnderSystemd(t)
	tests := []struct {
		name    string
		setup   func(f *fakeCLI)
		timeout time.Duration
		want    string
	}{
		{
			name: "is_error",
			setup: func(f *fakeCLI) {
				f.reply(map[string]any{"subtype": "success", "is_error": true, "result": "Not logged in · Please run /login"})
				f.write("exit", "1")
			},
			want: "claude_code: Not logged in · Please run /login",
		},
		{
			name: "a non-zero exit without the result",
			setup: func(f *fakeCLI) {
				f.write("stderr", "Usage: claude [options]\nerror: unknown option '--json-schema'\n")
				f.write("exit", "1")
			},
			want: "claude_code: error: unknown option '--json-schema'",
		},
		{
			name:  "output that is not the result",
			setup: func(f *fakeCLI) { f.write("reply", "Hello!\n") },
			want:  "claude_code: the CLI's output is not its result JSON",
		},
		{
			name:  "no text",
			setup: func(f *fakeCLI) { f.reply(map[string]any{"is_error": false, "result": " "}) },
			want:  "claude_code: the reply had no text",
		},
		{
			name:    "the timeout",
			setup:   func(f *fakeCLI) { f.write("hang", "") },
			timeout: 50 * time.Millisecond,
			want:    "the claude_code request timed out after 50ms",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeCLI(t)
			tc.setup(f)
			a := f.adapter(tc.timeout)
			_, err := a.Breakdown(context.Background(), goalReq())
			require.EqualError(t, err, tc.want)

			retro, err := a.Retro(context.Background(), llm.RetroRequest{WeekStart: "2026-09-14"})
			require.Error(t, err, "the reason is still reported")
			require.Equal(t, llm.GeneratedByRules, retro.GeneratedBy, "a retro always succeeds")
		})
	}
}

func TestClaudeCodeCommandLookup(t *testing.T) {
	home, empty := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", empty)
	cfg := config.Defaults().LLM
	cfg.Provider = config.ProviderClaudeCode

	_, err := llm.New(cfg, llm.Options{CredDir: withKey(t, "")})
	require.ErrorIs(t, err, llm.ErrUnavailable, "not on PATH and not in ~/.local/bin")
	require.ErrorContains(t, err, `the Claude Code CLI "claude" was not found`)

	installed := filepath.Join(home, ".local", "bin", "claude")
	require.NoError(t, os.MkdirAll(filepath.Dir(installed), 0o755))
	require.NoError(t, os.WriteFile(installed, []byte("#!/bin/sh\n"), 0o755))
	path, err := llm.FindCommand("claude")
	require.NoError(t, err)
	require.Equal(t, installed, path, "the installer's location is the fallback")
	_, err = llm.New(cfg, llm.Options{CredDir: withKey(t, "")})
	require.NoError(t, err)

	onPath := filepath.Join(empty, "claude")
	require.NoError(t, os.WriteFile(onPath, []byte("#!/bin/sh\n"), 0o755))
	path, err = llm.FindCommand("claude")
	require.NoError(t, err)
	require.Equal(t, onPath, path, "PATH comes first")

	_, err = llm.FindCommand(filepath.Join(home, "nowhere", "claude"))
	require.Error(t, err, "an absolute path is used as it is")
}
