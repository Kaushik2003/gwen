package remote

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/herdr"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// ucService is a Service on its own database and home, with no core, and
// the events it publishes.
func ucService(t *testing.T, db *store.DB, clk *clock.Fake, home string) (*Service, *ucEvents) {
	t.Helper()
	evs := &ucEvents{}
	svc, err := New(Options{DB: db, Repos: store.NewRepos(db), Clock: clk, Config: config.Defaults, Home: home, Publish: evs.add})
	require.NoError(t, err)
	return svc, evs
}

type ucEvents struct {
	mu    sync.Mutex
	names []string
}

func (e *ucEvents) add(name string, _ any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.names = append(e.names, name)
}

func (e *ucEvents) count(name string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, have := range e.names {
		if have == name {
			n++
		}
	}
	return n
}

// The usage limits come from `gwen statusline` inside a terminal of Gwen's
// core, through the hook socket's checks; the reading is kept, so a restart
// still shows it, and screens hear of it only when it changes.
func TestStatusLineUsageIsKept(t *testing.T) {
	clk := clock.NewFake(testutil.At("09:00:00"))
	db := testutil.NewDBWithClock(t, clk)
	home := t.TempDir()
	svc, evs := ucService(t, db, clk, home)
	require.Zero(t, svc.Usage().At, "no reading yet")

	// The test process stands in for the terminal's program and the status line.
	me := os.Getpid()
	svc.mu.Lock()
	svc.terms = map[string]herdr.Terminal{"t1": {Address: "t1", State: "live", PID: me}}
	svc.mu.Unlock()
	reading := &wire.ClaudeUsage{FiveHour: &wire.UsageWindow{UsedPercent: 42.5, ResetsAt: 1_794_560_000_000}, SevenDay: &wire.UsageWindow{UsedPercent: 130}}
	svc.HandleHook(t.Context(), wire.CodeHookEvent{Event: wire.HookStatusLine, Terminal: "t1", Usage: reading}, me)

	got := svc.Usage()
	require.Equal(t, clk.Now().UnixMilli(), got.At)
	require.Equal(t, &wire.UsageWindow{UsedPercent: 42.5, ResetsAt: 1_794_560_000_000}, got.FiveHour)
	require.Equal(t, 100.0, got.SevenDay.UsedPercent, "a share is at most 100")
	require.Equal(t, 1, evs.count(wire.EventCodeUsage))
	require.Empty(t, svc.sessions, "a status line makes no session card")

	// The same numbers again: newer, but nothing to tell the screens.
	clk.Advance(time.Minute)
	svc.HandleHook(t.Context(), wire.CodeHookEvent{Event: wire.HookStatusLine, Terminal: "t1", Usage: reading}, me)
	require.Equal(t, clk.Now().UnixMilli(), svc.Usage().At)
	require.Equal(t, 1, evs.count(wire.EventCodeUsage))

	// From outside the terminal it does not count.
	svc.HandleHook(t.Context(), wire.CodeHookEvent{Event: wire.HookStatusLine, Terminal: "t1",
		Usage: &wire.ClaudeUsage{FiveHour: &wire.UsageWindow{UsedPercent: 1}}}, 1)
	require.Equal(t, 42.5, svc.Usage().FiveHour.UsedPercent)

	again, _ := ucService(t, db, clk, home)
	require.Equal(t, 42.5, again.Usage().FiveHour.UsedPercent, "kept across a restart")
}

// A session Gwen starts gets `gwen statusline` as its status line, with the
// padding and refresh of yours, the folder's own settings first.
func TestStatusLineSettings(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	clk := clock.NewFake(testutil.At("09:00:00"))
	db := testutil.NewDBWithClock(t, clk)
	home := t.TempDir()
	svc, _ := ucService(t, db, clk, home)
	require.Empty(t, svc.statusLineSettings(home), "no gwen binary, no status line")

	svc.opts.GwenBin = "/home/a b/.local/bin/gwen"
	var got map[string]map[string]any
	require.NoError(t, json.Unmarshal([]byte(svc.statusLineSettings(home)), &got))
	require.Equal(t, map[string]any{"type": "command", "command": `'/home/a b/.local/bin/gwen' statusline`}, got["statusLine"])

	require.NoError(t, os.MkdirAll(filepath.Join(home, ".claude"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".claude", "settings.json"),
		[]byte(`{"statusLine": {"type": "command", "command": "mine.sh", "padding": 2, "refreshInterval": 5}}`), 0o644))
	require.NoError(t, json.Unmarshal([]byte(svc.statusLineSettings(home)), &got))
	require.Equal(t, 2.0, got["statusLine"]["padding"])
	require.Equal(t, 5.0, got["statusLine"]["refreshInterval"])

	dir := filepath.Join(home, "code")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.local.json"),
		[]byte(`{"statusLine": {"type": "command", "command": "folder.sh", "refreshInterval": 9}}`), 0o644))
	require.Equal(t, "folder.sh", OwnStatusLine(home, dir)["command"])
	require.NoError(t, json.Unmarshal([]byte(svc.statusLineSettings(dir)), &got))
	require.Equal(t, 9.0, got["statusLine"]["refreshInterval"])
	require.NotContains(t, got["statusLine"], "padding")
}

// ucTranscript writes a Claude transcript of a conversation in cwd, kept
// where Claude Code keeps it under home, and returns its path.
func ucTranscript(t *testing.T, home, cwd, id string, lines ...string) string {
	t.Helper()
	dir := filepath.Join(home, ".claude", "projects", projectDirName(cwd))
	require.NoError(t, os.MkdirAll(dir, 0o755))
	p := filepath.Join(dir, id+".jsonl")
	require.NoError(t, os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644))
	return p
}

func ucPrompt(cwd, text, at string) string {
	b, _ := json.Marshal(map[string]any{"type": "user", "cwd": cwd, "timestamp": at, "message": map[string]any{"role": "user", "content": text}})
	return string(b)
}

func ucBash(id, command, at string) string {
	b, _ := json.Marshal(map[string]any{"type": "assistant", "timestamp": at, "message": map[string]any{"role": "assistant",
		"content": []any{map[string]any{"type": "tool_use", "id": id, "name": "Bash", "input": map[string]any{"command": command}}}}})
	return string(b)
}

func ucResult(id, output, at string, isError bool) string {
	b, _ := json.Marshal(map[string]any{"type": "user", "timestamp": at, "message": map[string]any{"role": "user",
		"content": []any{map[string]any{"type": "tool_result", "tool_use_id": id, "content": output, "is_error": isError}}}})
	return string(b)
}

func ucTitle(title string) string {
	b, _ := json.Marshal(map[string]any{"type": "custom-title", "customTitle": title})
	return string(b)
}

const (
	ucConvA = "11111111-1111-4111-8111-111111111111"
	ucConvB = "22222222-2222-4222-8222-222222222222"
	ucConvC = "33333333-3333-4333-8333-333333333333"
)

// A commit links to the conversation whose git commit printed it, in the
// project's folder or one inside it; a failed commit, another command, and a
// folder that only shares the name's start do not count. Reading again picks
// up what was added to a transcript.
func TestCommitSessionsFromTranscripts(t *testing.T) {
	clk := clock.NewFake(testutil.At("09:00:00"))
	db := testutil.NewDBWithClock(t, clk)
	home := t.TempDir()
	svc, _ := ucService(t, db, clk, home)
	repo := filepath.Join(home, "code", "gwen")
	ui := filepath.Join(repo, "ui")
	other := filepath.Join(home, "code", "gwen-term")
	for _, d := range []string{ui, other} {
		require.NoError(t, os.MkdirAll(d, 0o755))
	}

	a := ucTranscript(t, home, repo, ucConvA,
		ucPrompt(repo, "Fix the phone pairing", "2026-09-15T08:00:00Z"),
		ucTitle("gwen"), // Gwen names sessions after their folder: no name of its own
		ucBash("b1", `git add -A && git commit -m "fix(remote): end a phone's connections"`, "2026-09-15T08:10:00Z"),
		ucResult("b1", "[main 20dd8fb] fix(remote): end a phone's connections\n 2 files changed", "2026-09-15T08:10:01Z", false),
		ucBash("b2", `git commit -m "nothing"`, "2026-09-15T08:11:00Z"),
		ucResult("b2", "[main 99999aa] nothing", "2026-09-15T08:11:01Z", true),
		ucBash("b3", `cat notes.txt # about git commit`, "2026-09-15T08:12:00Z"),
	)
	ucTranscript(t, home, ui, ucConvB,
		ucPrompt(ui, "Make the history readable", "2026-09-15T09:00:00Z"),
		ucTitle("History polish"),
		ucBash("c1", `git commit -m "feat(ui): wrap long lines"`, "2026-09-15T09:30:00Z"),
		ucResult("c1", "[w16 (root-commit) abc1234] feat(ui): wrap long lines", "2026-09-15T09:30:02Z", false),
	)
	ucTranscript(t, home, other, ucConvC,
		ucPrompt(other, "Elsewhere", "2026-09-15T10:00:00Z"),
		ucBash("d1", `git commit -m "x"`, "2026-09-15T10:00:00Z"),
		ucResult("d1", "[main def5678] x", "2026-09-15T10:00:01Z", false),
	)

	got, err := svc.CommitSessions(t.Context(), repo)
	require.NoError(t, err)
	require.Len(t, got.Commits, 2)
	require.Equal(t, "abc1234", got.Commits[0].Commit, "newest first")
	require.Equal(t, ucConvB, got.Commits[0].ClaudeSessionID)
	require.Equal(t, "History polish", got.Commits[0].Title)
	require.Equal(t, ui, got.Commits[0].Cwd)
	require.Equal(t, wire.CommitSession{Commit: "20dd8fb", At: time.Date(2026, 9, 15, 8, 10, 1, 0, time.UTC).UnixMilli(), ClaudeSessionID: ucConvA,
		FirstPrompt: "Fix the phone pairing", Cwd: repo}, got.Commits[1])

	// The conversation goes on and commits again.
	f, err := os.OpenFile(a, os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = fmt.Fprintln(f, ucBash("b4", `git commit -am "docs: say so"`, "2026-09-15T11:00:00Z"))
	require.NoError(t, err)
	_, err = fmt.Fprintln(f, ucResult("b4", "[main 4444444] docs: say so", "2026-09-15T11:00:01Z", false))
	require.NoError(t, err)
	require.NoError(t, f.Close())
	got, err = svc.CommitSessions(t.Context(), repo)
	require.NoError(t, err)
	require.Len(t, got.Commits, 3)
	require.Equal(t, "4444444", got.Commits[0].Commit)

	// Gwen's summary is kept per commit.
	require.NoError(t, svc.KeepCommitSummary(t.Context(), ucConvA, "20dd8fb", "Unpairing a phone ends its connections."))
	got, err = svc.CommitSessions(t.Context(), repo)
	require.NoError(t, err)
	summaries := map[string]string{}
	for _, c := range got.Commits {
		summaries[c.Commit] = c.Summary
	}
	require.Equal(t, map[string]string{"4444444": "", "20dd8fb": "Unpairing a phone ends its connections.", "abc1234": ""}, summaries)

	// What led to a commit, for the model: the turns up to it.
	text, err := svc.CommitConversation(repo, ucConvA, "20dd8fb")
	require.NoError(t, err)
	require.Contains(t, text, "You: Fix the phone pairing")
	require.NotContains(t, text, "docs: say so", "nothing after the commit")
	_, err = svc.CommitConversation(repo, ucConvA, "abc1234")
	require.Error(t, err, "that conversation didn't make it")

	_, err = svc.CommitSessions(t.Context(), filepath.Join(home, ".hidden"))
	require.Error(t, err, "outside the folders Gwen may open")
}

// git commit -q prints no commit: git itself names the one made just before
// the call's result whose subject is in the command.
func TestCommitSessionsFindAQuietCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	clk := clock.NewFake(testutil.At("09:00:00"))
	db := testutil.NewDBWithClock(t, clk)
	home := t.TempDir()
	svc, _ := ucService(t, db, clk, home)
	repo := filepath.Join(home, "code", "quiet")
	require.NoError(t, os.MkdirAll(repo, 0o755))
	when := time.Date(2026, 9, 15, 8, 0, 0, 0, time.UTC)
	gitIn := func(at time.Time, args ...string) string {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		stamp := at.Format(time.RFC3339)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_AUTHOR_DATE="+stamp, "GIT_COMMITTER_DATE="+stamp,
			"GIT_AUTHOR_NAME=Ada", "GIT_AUTHOR_EMAIL=ada@example.com", "GIT_COMMITTER_NAME=Ada", "GIT_COMMITTER_EMAIL=ada@example.com")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		return strings.TrimSpace(string(out))
	}
	gitIn(when, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a"), 0o644))
	gitIn(when, "add", "a.txt")
	gitIn(when, "commit", "-q", "-m", "feat: the first one")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "b.txt"), []byte("b"), 0o644))
	gitIn(when.Add(30*time.Second), "add", "b.txt")
	gitIn(when.Add(30*time.Second), "commit", "-q", "-m", "fix: the quiet one")
	short := gitIn(when, "log", "-1", "--format=%h")

	ucTranscript(t, home, repo, ucConvA,
		ucPrompt(repo, "Commit it quietly", "2026-09-15T07:59:00Z"),
		ucBash("q1", `git add -A && git commit -q -m "fix: the quiet one"`, "2026-09-15T08:00:29Z"),
		ucResult("q1", "", "2026-09-15T08:00:31Z", false),
		ucBash("q2", `git commit -q -m "never made"`, "2026-09-15T08:05:00Z"),
		ucResult("q2", "", "2026-09-15T08:05:01Z", false),
	)
	got, err := svc.CommitSessions(t.Context(), repo)
	require.NoError(t, err)
	require.Len(t, got.Commits, 1, "the second call made no commit git knows of")
	require.Equal(t, short, got.Commits[0].Commit)
	require.Equal(t, ucConvA, got.Commits[0].ClaudeSessionID)
}
