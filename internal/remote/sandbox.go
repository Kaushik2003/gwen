package remote

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kzark/gwen/internal/config"
)

// The sandbox of plan.md 3.14.8 runs Claude Code under bubblewrap. Its rule
// is that nothing a sandboxed session writes may take effect outside the
// sandbox by itself:
//
//   - Claude's own folder (~/.claude) inside the sandbox is a private one,
//     kept per project under Gwen's data folder. Your real one is never
//     writable: only your settings, instructions, commands, agents, skills
//     and plugins are shown, read-only, so a sandboxed Claude cannot plant a
//     hook, a command, or an MCP server that your own Claude would then run.
//   - It signs in separately (a token from `claude setup-token`, or its own
//     /login), so it never reads or rewrites your Claude's sign-in.
//   - The tool caches (Go modules, npm, pip, cargo, ...) are your real ones
//     underneath, read-only, with a private writable layer on top, so it
//     cannot leave something in a cache that a later build of yours trusts.
//   - Git's own hooks and config of the project are read-only, because git
//     runs them by itself the next time you use the repository.

// SandboxOptions is what the bubblewrap command is built from.
type SandboxOptions struct {
	Home string
	// Dir is the session's folder, the only part of home it may write
	// besides its own private state.
	Dir string
	// GwenBin is the gwen binary, so the hooks work inside.
	GwenBin string
	// RuntimeDir is $XDG_RUNTIME_DIR/gwen; HookSocket is the hook socket in
	// it, the only way out to the daemon.
	RuntimeDir string
	HookSocket string
	// Claude is the claude executable, resolved outside the sandbox.
	Claude string
	Extra  []config.SandboxPath
	// StateDir is this project's private sandbox state (SandboxStateDir):
	// Claude's own folder for sandboxed sessions here, and the writable
	// layers of the tool caches.
	StateDir string
}

// claudeShared is what a sandboxed Claude sees of your real ~/.claude, each
// read-only. Everything else in that folder, your sign-in and your other
// conversations among it, is not there at all.
var claudeShared = []string{"settings.json", "CLAUDE.md", "commands", "agents", "skills", "plugins", "keybindings.json",
	"output-styles", "rules"}

// sandboxReadOnly are your toolchains, visible read-only.
var sandboxReadOnly = []string{".local/bin", ".local/share/claude", "go", ".cargo/bin", ".rustup", ".nvm", ".bun", ".deno",
	".gitconfig", ".config/git"}

// sandboxCaches are the tool caches: yours underneath, a private layer on top.
var sandboxCaches = []string{"go/pkg/mod", ".cache/go-build", ".npm", ".cache/pip", ".cargo/registry", ".local/share/pnpm"}

// SandboxStateDir is where a project's sandbox keeps its private state,
// under Gwen's data folder: one folder per project folder.
func SandboxStateDir(dataDir, dir string) string {
	sum := sha256.Sum256([]byte(dir))
	name := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '-'
	}, filepath.Base(dir))
	return filepath.Join(dataDir, "sandbox", strings.TrimLeft(name, ".")+"-"+hex.EncodeToString(sum[:6]))
}

// claudeProjectDir is the folder Claude Code keeps a working folder's
// conversations in. For a path longer than 200 characters Claude Code adds
// a hash of its own, which Gwen does not guess: "" then.
func claudeProjectDir(dir string) string {
	name := projectDirName(dir)
	if len(name) > 200 {
		return ""
	}
	return name
}

var overlay struct {
	once sync.Once
	ok   bool
}

// overlaySupported reports whether this bubblewrap can stack a private
// writable layer over a read-only folder (0.11 and newer).
func overlaySupported() bool {
	overlay.once.Do(func() {
		out, _ := exec.Command("bwrap", "--help").CombinedOutput()
		overlay.ok = strings.Contains(string(out), "--overlay-src")
	})
	return overlay.ok
}

// BuildSandbox prepares a project's private sandbox state and returns the
// command that runs Claude in the sandbox: bwrap, its arguments, "--", and
// claude. The caller appends Claude's own arguments.
func BuildSandbox(o SandboxOptions) ([]string, error) {
	if o.StateDir == "" || o.Home == "" || o.Dir == "" {
		return nil, errors.New("the sandbox needs a folder and a place for its state")
	}
	exists := func(p string) bool {
		_, err := os.Lstat(p)
		return err == nil
	}
	dirs := []string{filepath.Join(o.StateDir, "claude"), filepath.Join(o.StateDir, "empty")}
	if proj := claudeProjectDir(o.Dir); proj != "" {
		// Conversations stay where your own Claude and Gwen look for them.
		dirs = append(dirs, filepath.Join(o.Home, ".claude", "projects", proj))
	}
	for _, rel := range sandboxCaches {
		priv := cacheDir(o.StateDir, rel)
		dirs = append(dirs, filepath.Join(priv, "upper"), filepath.Join(priv, "work"), filepath.Join(priv, "dir"))
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, fmt.Errorf("prepare the sandbox: %w", err)
		}
	}
	if err := seedClaudeConfig(o); err != nil {
		return nil, fmt.Errorf("prepare the sandbox: %w", err)
	}
	return sandboxArgv(o, exists, overlaySupported()), nil
}

func cacheDir(state, rel string) string {
	return filepath.Join(state, "cache", strings.ReplaceAll(strings.TrimLeft(rel, "."), "/", "-"))
}

// seedClaudeConfig writes the sandbox's own ~/.claude.json (which lives
// inside its private folder): a copy of yours without your other projects,
// your MCP servers, and your account, so Claude does not start as if it
// were new, plus what earlier sandboxed sessions of this project stored
// about this project and their own sign-in. Nothing ever flows back.
func seedClaudeConfig(o SandboxOptions) error {
	read := func(path string) map[string]json.RawMessage {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var m map[string]json.RawMessage
		if json.Unmarshal(data, &m) != nil {
			return nil
		}
		return m
	}
	projectOf := func(m map[string]json.RawMessage) map[string]json.RawMessage {
		var projects map[string]json.RawMessage
		if json.Unmarshal(m["projects"], &projects) != nil {
			return nil
		}
		var entry map[string]json.RawMessage
		if json.Unmarshal(projects[o.Dir], &entry) != nil {
			return nil
		}
		return entry
	}
	target := filepath.Join(o.StateDir, "claude", ".claude.json")
	host, prev := read(filepath.Join(o.Home, ".claude.json")), read(target)
	out := map[string]json.RawMessage{}
	for k, v := range host {
		switch k {
		case "projects", "mcpServers", "githubRepoPaths", "oauthAccount", "userID":
		default:
			out[k] = v
		}
	}
	// The sandbox's own sign-in, when it has one.
	for _, k := range []string{"oauthAccount", "userID"} {
		if v, ok := prev[k]; ok {
			out[k] = v
		}
	}
	entry := projectOf(prev)
	if entry == nil {
		entry = map[string]json.RawMessage{}
		// A folder you already trusted needs no second asking.
		if v, ok := projectOf(host)["hasTrustDialogAccepted"]; ok {
			entry["hasTrustDialogAccepted"] = v
		}
	}
	projects, err := json.Marshal(map[string]any{o.Dir: entry})
	if err != nil {
		return err
	}
	out["projects"] = projects
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	tmp := target + ".seed"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}

// sandboxArgv is the bubblewrap command. exists reports whether a path is
// there on this machine, and overlay whether bubblewrap can stack layers.
func sandboxArgv(o SandboxOptions, exists func(string) bool, overlay bool) []string {
	// Claude's own folder has the same path inside and outside the sandbox;
	// inside, it is the private one.
	claude := filepath.Join(o.Home, ".claude")
	argv := []string{"bwrap", "--die-with-parent", "--unshare-pid", "--unshare-ipc", "--unshare-uts",
		"--ro-bind", "/usr", "/usr", "--symlink", "usr/bin", "/bin", "--symlink", "usr/sbin", "/sbin",
		"--symlink", "usr/lib", "/lib", "--symlink", "usr/lib64", "/lib64",
		"--ro-bind", "/etc", "/etc", "--ro-bind-try", "/opt", "/opt", "--proc", "/proc", "--dev", "/dev",
		"--tmpfs", "/tmp", "--tmpfs", "/var/tmp",
		"--tmpfs", "/run", "--ro-bind-try", "/run/systemd/resolve", "/run/systemd/resolve",
		"--tmpfs", o.Home, "--bind", o.Dir, o.Dir,
		"--bind", filepath.Join(o.StateDir, "claude"), claude, "--setenv", "CLAUDE_CONFIG_DIR", claude}
	for _, name := range claudeShared {
		if p := filepath.Join(claude, name); exists(p) {
			argv = append(argv, "--ro-bind", p, p)
		}
	}
	if proj := claudeProjectDir(o.Dir); proj != "" {
		p := filepath.Join(claude, "projects", proj)
		argv = append(argv, "--bind", p, p)
	}
	// Git runs a repository's hooks and config by itself, outside any
	// sandbox, the next time you use the repository: neither may change.
	if git := filepath.Join(o.Dir, ".git"); exists(filepath.Join(git, "config")) {
		argv = append(argv, "--ro-bind", filepath.Join(git, "config"), filepath.Join(git, "config"))
		hooks := filepath.Join(git, "hooks")
		if exists(hooks) {
			argv = append(argv, "--ro-bind", hooks, hooks)
		} else {
			argv = append(argv, "--ro-bind", filepath.Join(o.StateDir, "empty"), hooks)
		}
	}
	argv = append(argv, "--ro-bind", o.GwenBin, o.GwenBin, "--dir", o.RuntimeDir, "--bind", o.HookSocket, o.HookSocket)
	for _, rel := range sandboxReadOnly {
		p := filepath.Join(o.Home, rel)
		argv = append(argv, "--ro-bind-try", p, p)
	}
	for _, rel := range sandboxCaches {
		p, priv := filepath.Join(o.Home, rel), cacheDir(o.StateDir, rel)
		switch {
		case exists(p) && overlay:
			argv = append(argv, "--overlay-src", p, "--overlay", filepath.Join(priv, "upper"), filepath.Join(priv, "work"), p)
		case exists(p) || !underReadOnly(rel):
			// No layering here, or no cache of yours yet: a private one.
			argv = append(argv, "--bind", filepath.Join(priv, "dir"), p)
		}
	}
	for _, e := range o.Extra {
		p := ExpandHome(e.Path, o.Home)
		if e.Write {
			argv = append(argv, "--bind", p, p)
		} else {
			argv = append(argv, "--ro-bind", p, p)
		}
	}
	return append(argv, "--unsetenv", "SSH_AUTH_SOCK", "--unsetenv", "DBUS_SESSION_BUS_ADDRESS",
		"--unsetenv", "WAYLAND_DISPLAY", "--unsetenv", "DISPLAY", "--unsetenv", "HC_SOCKET",
		// HC_TERMINAL_SOCKET stays: it is only the name by which `gwen hook`
		// knows this is one of Gwen's terminals; the socket itself is not here.
		"--unsetenv", "HC_SESSION",
		"--setenv", "GWEN_SANDBOX", "1", "--chdir", o.Dir, "--", o.Claude)
}

// underReadOnly reports whether a path under home sits inside a toolchain
// folder that is shown read-only, where no new folder can be made.
func underReadOnly(rel string) bool {
	for _, ro := range sandboxReadOnly {
		if strings.HasPrefix(rel, ro+"/") {
			return true
		}
	}
	return false
}

// ExpandHome turns "~" and "~/x" into paths under home.
func ExpandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		return filepath.Join(home, rest)
	}
	return path
}

// FindClaude is the claude executable Gwen starts: the one on PATH, else
// ~/.local/bin/claude.
func FindClaude(home string) string {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		p := filepath.Join(dir, "claude")
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return p
		}
	}
	return filepath.Join(home, ".local", "bin", "claude")
}
