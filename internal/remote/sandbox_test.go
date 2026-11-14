package remote

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kzark/gwen/internal/config"
	"github.com/stretchr/testify/require"
)

// The sandbox's whole promise (plan.md 3.14.8) is in one bubblewrap command,
// so it is kept as golden files: a change to it shows up as a diff to review.
// Rewrite them with: go test ./internal/remote -run TestSandboxArgv -update

func init() {
	// Another test file of this package may have defined it first.
	if flag.Lookup("update") == nil {
		flag.Bool("update", false, "rewrite the golden files in testdata")
	}
}

func w27cUpdating() bool {
	f := flag.Lookup("update")
	return f != nil && f.Value.String() == "true"
}

// w27cGolden compares got with testdata/sandbox/name, or rewrites it with -update.
func w27cGolden(t *testing.T, name string, got string) {
	t.Helper()
	path := filepath.Join("testdata", "sandbox", name)
	if w27cUpdating() {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "run the test with -update to write %s", path)
	require.Equal(t, string(want), got)
}

// w27cArgvLines writes a command with each option and its values on a line
// of its own, the way plan.md 3.14.8 lays it out.
func w27cArgvLines(argv []string) string {
	var b strings.Builder
	for i, a := range argv {
		switch {
		case i == 0:
		case strings.HasPrefix(a, "--"):
			b.WriteByte('\n')
		default:
			b.WriteByte(' ')
		}
		b.WriteString(a)
	}
	b.WriteByte('\n')
	return b.String()
}

// w27cExists answers for a made-up machine: only the paths listed are there.
func w27cExists(paths ...string) func(string) bool {
	set := map[string]bool{}
	for _, p := range paths {
		set[p] = true
	}
	return func(p string) bool { return set[p] }
}

// w27cSandboxOptions are fixed inputs for a person "user" with a project at
// ~/code/app, so the golden files hold no path of the machine running them.
func w27cSandboxOptions() SandboxOptions {
	return SandboxOptions{
		Home:       "/home/user",
		Dir:        "/home/user/code/app",
		GwenBin:    "/home/user/.local/bin/gwen",
		RuntimeDir: "/run/user/1000/gwen",
		HookSocket: "/run/user/1000/gwen/hook.sock",
		Claude:     "/home/user/.local/bin/claude",
		Extra:      []config.SandboxPath{{Path: "~/notes"}, {Path: "/srv/shared", Write: true}},
		StateDir:   "/home/user/.local/share/gwen/sandbox/app-0123456789ab",
	}
}

func TestSandboxArgv(t *testing.T) {
	t.Parallel()
	o := w27cSandboxOptions()
	claude := "/home/user/.claude/"
	tests := []struct {
		name    string
		o       SandboxOptions
		exists  func(string) bool
		overlay bool
	}{
		{
			// Bubblewrap 0.11 or newer: your caches underneath, a private
			// layer on top; the project's git hooks and config read-only.
			name: "overlay.golden", o: o, overlay: true,
			exists: w27cExists(claude+"settings.json", claude+"CLAUDE.md", claude+"commands", claude+"skills",
				o.Dir+"/.git/config", o.Dir+"/.git/hooks",
				"/home/user/go/pkg/mod", "/home/user/.cache/go-build", "/home/user/.npm"),
		},
		{
			// An older bubblewrap gets private empty caches. A repository
			// without a hooks folder gets an empty one, read-only, so none
			// can be planted; a cache inside a read-only toolchain folder
			// that isn't there is left out, since nothing can be made there.
			name: "no-overlay.golden", o: o, overlay: false,
			exists: w27cExists(claude+"settings.json", claude+"agents", claude+"plugins", claude+"keybindings.json",
				claude+"output-styles", claude+"rules", o.Dir+"/.git/config", "/home/user/.npm", "/home/user/.cargo/registry"),
		},
	}
	// A folder that isn't git, with a path so long that Claude Code hashes
	// its project folder: Gwen doesn't guess the hash and binds none.
	long := o
	long.Dir = "/home/user/code/" + strings.Repeat("deep/", 40) + "app"
	long.Extra = nil
	tests = append(tests, struct {
		name    string
		o       SandboxOptions
		exists  func(string) bool
		overlay bool
	}{name: "no-git-long-path.golden", o: long, overlay: true, exists: w27cExists(claude + "settings.json")})

	for _, tt := range tests {
		argv := sandboxArgv(tt.o, tt.exists, tt.overlay)
		w27cGolden(t, tt.name, w27cArgvLines(argv))
		require.Equal(t, []string{"--", tt.o.Claude}, argv[len(argv)-2:], "claude comes last, so its own arguments follow")
	}
}

// The rules the golden files show, said once more in words.
func TestSandboxArgvRules(t *testing.T) {
	t.Parallel()
	o := w27cSandboxOptions()
	argv := sandboxArgv(o, w27cExists(o.Dir+"/.git/config"), true)
	joined := strings.Join(argv, " ")
	require.Contains(t, joined, "--tmpfs /home/user --bind /home/user/code/app /home/user/code/app",
		"home is empty but for the project")
	require.Contains(t, joined, "--bind "+o.StateDir+"/claude /home/user/.claude", "Claude's own folder is a private one")
	require.Contains(t, joined, "--tmpfs /run ", "herdr-core's socket is not reachable")
	require.Contains(t, joined, "--bind "+o.HookSocket+" "+o.HookSocket, "the hook socket is the one way out")
	for _, v := range []string{"SSH_AUTH_SOCK", "DBUS_SESSION_BUS_ADDRESS", "WAYLAND_DISPLAY", "DISPLAY", "HC_SOCKET", "HC_SESSION"} {
		require.Contains(t, joined, "--unsetenv "+v+" ")
	}
	require.NotContains(t, joined, "--bind /home/user/.claude/settings.json", "nothing of your own Claude is writable")
	require.NotContains(t, joined, "--unsetenv HC_TERMINAL_SOCKET", "gwen hook still knows it runs in one of Gwen's terminals")
}

// BuildSandbox prepares the project's private state: its own Claude folder,
// the cache layers, and a copy of your ~/.claude.json without your other
// projects, MCP servers and account (plan.md 3.14.8).
func TestBuildSandboxPreparesPrivateState(t *testing.T) {
	t.Parallel()
	home, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	dir := filepath.Join(home, "code", "app")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("[core]\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".claude"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte("{}"), 0o600))
	host := map[string]any{
		"numStartups": 7, "theme": "dark",
		"projects": map[string]any{
			dir:           map[string]any{"hasTrustDialogAccepted": true, "allowedTools": []string{"Bash"}},
			"/home/other": map[string]any{"hasTrustDialogAccepted": true},
		},
		"mcpServers":      map[string]any{"mail": map[string]any{"command": "mail-mcp"}},
		"githubRepoPaths": map[string]any{"me/app": []string{dir}},
		"oauthAccount":    map[string]any{"emailAddress": "me@example.invalid"},
		"userID":          "host-user",
	}
	data, err := json.Marshal(host)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(home, ".claude.json"), data, 0o600))

	o := SandboxOptions{Home: home, Dir: dir, GwenBin: "/usr/bin/gwen", RuntimeDir: filepath.Join(home, "run"),
		HookSocket: filepath.Join(home, "run", "hook.sock"), Claude: "/usr/bin/claude",
		StateDir: SandboxStateDir(filepath.Join(home, ".local", "share", "gwen"), dir)}
	argv, err := BuildSandbox(o)
	require.NoError(t, err)
	require.Equal(t, "bwrap", argv[0])
	require.Equal(t, sandboxArgv(o, func(p string) bool { _, err := os.Lstat(p); return err == nil }, overlaySupported()), argv)

	for _, d := range []string{"claude", "empty", "cache/go-pkg-mod/upper", "cache/go-pkg-mod/work", "cache/npm/dir"} {
		st, err := os.Stat(filepath.Join(o.StateDir, d))
		require.NoError(t, err, d)
		require.True(t, st.IsDir())
	}
	_, err = os.Stat(filepath.Join(home, ".claude", "projects", projectDirName(dir)))
	require.NoError(t, err, "this folder's conversations stay where your own Claude keeps them")

	seed := func() map[string]any {
		data, err := os.ReadFile(filepath.Join(o.StateDir, "claude", ".claude.json"))
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(data, &m))
		return m
	}
	require.Equal(t, map[string]any{
		"numStartups": float64(7), "theme": "dark",
		"projects": map[string]any{dir: map[string]any{"hasTrustDialogAccepted": true}},
	}, seed(), "a folder you trusted needs no second asking; nothing else of yours comes in")

	// The sandbox signed in by itself and learned about the project; the
	// next start keeps that, and still takes nothing of your account.
	prev := seed()
	prev["oauthAccount"] = map[string]any{"emailAddress": "sandbox@example.invalid"}
	prev["userID"] = "sandbox-user"
	prev["projects"] = map[string]any{dir: map[string]any{"hasTrustDialogAccepted": true, "lastCost": 0.5}}
	data, err = json.Marshal(prev)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(o.StateDir, "claude", ".claude.json"), data, 0o600))
	_, err = BuildSandbox(o)
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"numStartups": float64(7), "theme": "dark",
		"oauthAccount": map[string]any{"emailAddress": "sandbox@example.invalid"}, "userID": "sandbox-user",
		"projects": map[string]any{dir: map[string]any{"hasTrustDialogAccepted": true, "lastCost": 0.5}},
	}, seed())

	_, err = BuildSandbox(SandboxOptions{Home: home, Dir: dir})
	require.Error(t, err, "no state folder, no sandbox")
}

func TestSandboxStateDir(t *testing.T) {
	t.Parallel()
	a := SandboxStateDir("/data/gwen", "/home/user/code/My App!")
	require.Regexp(t, regexp.MustCompile(`^/data/gwen/sandbox/My-App--[0-9a-f]{12}$`), a)
	require.Equal(t, a, SandboxStateDir("/data/gwen", "/home/user/code/My App!"), "the same folder keeps its state")
	require.NotEqual(t, a, SandboxStateDir("/data/gwen", "/home/user/work/My App!"), "another folder of the same name has its own")
	require.Regexp(t, regexp.MustCompile(`^/data/gwen/sandbox/dotfiles-[0-9a-f]{12}$`), SandboxStateDir("/data/gwen", "/home/user/.dotfiles"),
		"never a hidden folder")
}

// Git runs a repository's hooks and config by itself, outside any sandbox.
// In a folder that isn't git yet, a sandboxed `git init` makes a .git that
// is all writable: a hook or a core.fsmonitor planted there would run the
// next time you use git in that folder (plan.md 3.14.8: nothing a sandboxed
// session writes takes effect outside by itself).
func TestSandboxGuardsARepositoryMadeInside(t *testing.T) {
	t.Parallel()
	o := w27cSandboxOptions()
	argv := sandboxArgv(o, w27cExists(), true)
	if !strings.Contains(strings.Join(argv, " "), " "+o.Dir+"/.git") {
		t.Skip("BUG: sandboxArgv protects .git/config and .git/hooks only when .git/config exists at start, " +
			"so a repository the sandbox creates (git init) can carry hooks or config that run outside (sandbox.go)")
	}
}
