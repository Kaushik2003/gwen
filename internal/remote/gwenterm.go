package remote

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/herdr"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
)

// Gwen Terminal (gwen-term, a fork of Herdr's app) takes commands on a Unix
// socket in its home folder, one JSON line each way: what `gwen-term do`
// sends (plan.md 3.14.5).

// ErrNoWindow is Gwen Terminal not running, or not answering.
var ErrNoWindow = errors.New("the Gwen Terminal window isn't open")

// termHome is Gwen Terminal's folder: $HERDR_APP_HOME, else
// $XDG_CONFIG_HOME/gwen-term.
func termHome(home string) string {
	if h := os.Getenv("HERDR_APP_HOME"); h != "" {
		return h
	}
	if c := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(c) {
		return filepath.Join(c, "gwen-term")
	}
	return filepath.Join(home, ".config", "gwen-term")
}

// TermHome is Gwen Terminal's folder, where its mods, themes and keys live.
func TermHome(home string) string { return termHome(home) }

// TermInstalled reports whether gwen-term is installed.
func TermInstalled(home string) bool { return termBin(home) != "" }

// systemCore is where the rpm and deb put Gwen's herdr-core.
var systemCore = "/usr/lib/gwen/herdr-core"

// CoreBin is Gwen's own herdr-core, apart from any Herdr of yours: the one
// `make term` installs in ~/.local/lib/gwen, else the one the package
// installs in /usr/lib/gwen, "" when there is none.
func CoreBin(home string) string {
	for _, p := range []string{filepath.Join(home, ".local", "lib", "gwen", "herdr-core"), systemCore} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// termBin is the gwen-term executable, "" when it is not installed.
func termBin(home string) string {
	if p, err := exec.LookPath("gwen-term"); err == nil {
		return p
	}
	p := filepath.Join(home, ".local", "bin", "gwen-term")
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return p
	}
	return ""
}

// termRequest sends one request to the open window and returns its reply.
func (s *Service) termRequest(ctx context.Context, req map[string]any) (map[string]any, error) {
	var d net.Dialer
	dialCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	conn, err := d.DialContext(dialCtx, "unix", filepath.Join(termHome(s.home), "gwen-term.sock"))
	cancel()
	if err != nil {
		return nil, ErrNoWindow
	}
	defer conn.Close()
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 35*time.Second {
		deadline = time.Now().Add(35 * time.Second)
	}
	_ = conn.SetDeadline(deadline)
	line, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write(append(line, '\n')); err != nil {
		return nil, ErrNoWindow
	}
	r := bufio.NewReaderSize(conn, 1<<20)
	reply, err := r.ReadBytes('\n')
	if err != nil && len(reply) == 0 {
		return nil, ErrNoWindow
	}
	var out map[string]any
	if err := json.Unmarshal(reply, &out); err != nil {
		return nil, fmt.Errorf("the Gwen Terminal window answered something Gwen can't read: %w", err)
	}
	if msg, ok := out["error"].(string); ok && msg != "" {
		return out, fmt.Errorf("the Gwen Terminal window refused: %s", msg)
	}
	return out, nil
}

// termDo runs one app command, as `gwen-term do <command> <args>` does.
func (s *Service) termDo(ctx context.Context, command string, args map[string]any) (map[string]any, error) {
	return s.termRequest(ctx, map[string]any{"do": command, "args": args})
}

// termOpen reports whether the window answers.
func (s *Service) termOpen(ctx context.Context) bool {
	_, err := s.termRequest(ctx, map[string]any{"status": true})
	return err == nil
}

// ensureWindow starts Gwen Terminal when it is not running and waits up to
// ten seconds for it to answer. It is the one window gwend ever opens
// (plan.md 3.6), and it is started through systemd-run so it gets the
// desktop session's environment, not the service's.
func (s *Service) ensureWindow(ctx context.Context) error {
	if s.termOpen(ctx) {
		return nil
	}
	bin := termBin(s.home)
	if bin == "" {
		return errors.New("gwen-term isn't installed; run make term")
	}
	if err := s.launch(ctx, "gwen-term", bin); err != nil {
		return err
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
		if s.termOpen(ctx) {
			return nil
		}
	}
	return ErrNoWindow
}

// uiBin is the Gwen window's program, gwen-ui, "" when it isn't installed.
func uiBin(home string) string {
	if p, err := exec.LookPath("gwen-ui"); err == nil {
		return p
	}
	for _, p := range []string{filepath.Join(home, ".local", "bin", "gwen-ui"), "/usr/bin/gwen-ui"} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// openWindow shows a target (a screen, "project:<id>", "session:<id>", or
// "terminal:<address>") in the Gwen window (plan.md 3.15). gwen-ui keeps one
// window: started while it is open, it hands the target over and exits;
// otherwise it becomes the window. It runs through systemd-run so it gets
// the desktop session's environment, not the service's.
func (s *Service) openWindow(ctx context.Context, target string) error {
	bin := uiBin(s.home)
	if bin == "" {
		return errors.New("gwen-ui isn't installed")
	}
	return s.launch(ctx, "", bin, "--open", target)
}

// launch starts a long-lived program as a transient user unit; an empty
// unit lets systemd name it.
func (s *Service) launch(ctx context.Context, unit string, argv ...string) error {
	if s.opts.Launch != nil {
		return s.opts.Launch(ctx, unit, argv)
	}
	return systemdRun(ctx, unit, argv)
}

// ErrNoCore is Gwen's own herdr-core not installed.
var ErrNoCore = errors.New("herdr-core isn't installed for Gwen; run make term")

// StartCore starts Gwen's own herdr-core (session gwen) the way gwend does
// when a session needs one (plan.md 3.14.5): through systemd-run, so it gets
// the desktop session's environment and outlives the caller. It returns once
// the start command ran; the core's socket appears shortly after.
func StartCore(ctx context.Context, home string) error {
	hc := CoreBin(home)
	if hc == "" {
		return ErrNoCore
	}
	return systemdRun(ctx, coreUnit, coreArgv(hc))
}

// coreArgv starts the herdr-core at hc as Gwen's own core, never the one a
// Herdr of yours uses.
func coreArgv(hc string) []string {
	return []string{"env", "HC_SESSION=" + herdr.Session, hc, "server", "start"}
}

// systemdRun starts argv as a transient user unit.
func systemdRun(ctx context.Context, unit string, argv []string) error {
	args := []string{"--user", "--collect", "--quiet"}
	if unit == coreUnit {
		// `hc server start` puts the core in the background and returns. As a
		// one-shot unit that stays active, what it left running is kept: the
		// core, and every terminal in it, must outlive the command and any
		// window. It gets a name of systemd's choosing, so a core that died
		// can always be started again.
		args = append(args, "-p", "Type=oneshot", "-p", "RemainAfterExit=yes", "--description", "Gwen's herdr-core")
	} else if unit != "" {
		args = append(args, "--unit", unit)
	}
	args = append(append(args, "--"), argv...)
	out, err := exec.CommandContext(ctx, "systemd-run", args...).CombinedOutput()
	if err != nil {
		// A unit left over from a crash blocks the name; clear it and retry once.
		if unit != "" && strings.Contains(string(out), "already") {
			_ = exec.CommandContext(ctx, "systemctl", "--user", "reset-failed", unit+".service").Run()
			out, err = exec.CommandContext(ctx, "systemd-run", args...).CombinedOutput()
		}
		if err != nil {
			return fmt.Errorf("start %s: %s", unit, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// coreUnit names the launch of Gwen's own herdr-core.
const coreUnit = "gwen-core"

// replyTerminal finds the terminal a tab.new or space.new reply names.
var terminalAddress = regexp.MustCompile(`^t[0-9]+$`)

func replyTerminal(reply any) string {
	switch v := reply.(type) {
	case map[string]any:
		for _, key := range []string{"terminal", "address"} {
			if t, ok := v[key].(string); ok && terminalAddress.MatchString(t) {
				return t
			}
		}
		for _, inner := range v {
			if t := replyTerminal(inner); t != "" {
				return t
			}
		}
	case []any:
		for _, inner := range v {
			if t := replyTerminal(inner); t != "" {
				return t
			}
		}
	}
	return ""
}

// docSpace is one space of the app's shared document. The format belongs to
// Herdr: only the fields Gwen shows are read, and anything else is skipped.
type docSpace struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Folder string   `json:"folder"`
	Tabs   []docTab `json:"tabs"`
}

type docTab struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Root json.RawMessage `json:"root"`
}

// parseSpaces reads the "herdr-app" document.
func parseSpaces(doc json.RawMessage) []docSpace {
	var d struct {
		Spaces []docSpace `json:"spaces"`
	}
	if len(doc) == 0 || json.Unmarshal(doc, &d) != nil {
		return nil
	}
	return d.Spaces
}

// layoutTerminals are the terminals of a tab's layout tree, in order: every
// node whose content names a terminal, wherever it sits.
func layoutTerminals(root json.RawMessage) []string {
	var out []string
	seen := map[string]bool{}
	var walk func(v any)
	walk = func(v any) {
		switch n := v.(type) {
		case map[string]any:
			if t, ok := n["terminal"].(string); ok && t != "" && !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
			// Children first in their own order, then anything else that nests.
			if kids, ok := n["children"].([]any); ok {
				for _, k := range kids {
					walk(k)
				}
			}
			keys := make([]string, 0, len(n))
			for key := range n {
				if key != "children" {
					keys = append(keys, key)
				}
			}
			sort.Strings(keys)
			for _, key := range keys {
				walk(n[key])
			}
		case []any:
			for _, inner := range n {
				walk(inner)
			}
		}
	}
	var tree any
	if json.Unmarshal(root, &tree) == nil {
		walk(tree)
	}
	return out
}

func (s *Service) spacesDoc() []docSpace {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.spaces
}

// TerminalName is what to call a terminal in a list: its name, its title, its
// foreground program, or its command.
func TerminalName(t herdr.Terminal) string { return terminalName(t) }

// IsClaude reports whether Claude Code runs in t now (plan.md 3.14.5).
func IsClaude(t herdr.Terminal) bool { return isClaude(t) }

// ClaudeStatus is the status Claude Code reported in t, nil when none did.
func ClaudeStatus(t herdr.Terminal) *herdr.Status { return claudeStatus(t) }

// terminalName is what to call a terminal on the phone.
func terminalName(t herdr.Terminal) string {
	switch {
	case t.Name != "":
		return t.Name
	case t.Title != "":
		return t.Title
	case t.Foreground != nil && t.Foreground.Name != "":
		return t.Foreground.Name
	case len(t.Argv) > 0:
		return filepath.Base(t.Argv[0])
	}
	return t.Address
}

func (s *Service) wireTerminal(t herdr.Terminal) wire.CodeTerminal {
	out := wire.CodeTerminal{Terminal: t.Address, Name: terminalName(t), Cwd: t.Cwd, Cols: t.Cols, Rows: t.Rows}
	if t.Foreground != nil && t.Foreground.Cwd != "" {
		out.Cwd = t.Foreground.Cwd
	}
	if t.Status != nil {
		out.Status, out.App = t.Status.State, t.Status.App
	}
	if sess := s.byTerminal[t.Address]; sess != nil {
		out.SessionID = sess.ID
	}
	return out
}

// ProjectLabel marks a terminal the Gwen window started in a project
// (plan.md 3.15).
const ProjectLabel = "gwen.project"

// projectSpaces are the projects with a folder on this machine: from W16b
// each is a space on the phone (plan.md 3.15).
func (s *Service) projectSpaces(ctx context.Context) []model.Project {
	if s.repos.Projects == nil {
		return nil
	}
	ps, err := s.repos.Projects.List(ctx, store.ArchivedExclude)
	if err != nil {
		return nil
	}
	out := ps[:0]
	for _, p := range ps {
		if p.Folder != "" {
			out = append(out, p)
		}
	}
	return out
}

// Spaces lists the spaces, tabs and terminals for the phone, with the
// theme. Each project with a folder is a space with a tab per terminal in
// it (plan.md 3.15); while no project has one, Gwen Terminal's spaces are
// listed instead.
func (s *Service) Spaces(ctx context.Context) wire.CodeSpaces {
	theme := s.Theme()
	projects := s.projectSpaces(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	out := wire.CodeSpaces{Spaces: []wire.CodeSpace{}, Loose: []wire.CodeTerminal{}, Theme: theme}
	placed := map[string]bool{}
	if len(projects) > 0 {
		// The deepest folder wins, so a project inside another keeps its own terminals.
		byDepth := slices.Clone(projects)
		slices.SortStableFunc(byDepth, func(a, b model.Project) int { return len(b.Folder) - len(a.Folder) })
		owner := map[string]string{}
		for _, addr := range s.sortedTerminals() {
			t := s.terms[addr]
			if t.State != "live" || !t.Local() || t.Parent != "" {
				continue
			}
			if id := t.Labels[ProjectLabel]; id != "" {
				owner[addr] = id
				continue
			}
			cwd := s.wireTerminal(t).Cwd
			for _, p := range byDepth {
				if within(cwd, p.Folder) {
					owner[addr] = p.ID
					break
				}
			}
		}
		for _, p := range projects {
			ws := wire.CodeSpace{ID: p.ID, Name: p.Name, Folder: p.Folder, Tabs: []wire.CodeTab{}}
			for _, addr := range s.sortedTerminals() {
				if owner[addr] != p.ID {
					continue
				}
				wt := s.wireTerminal(s.terms[addr])
				placed[addr] = true
				ws.Tabs = append(ws.Tabs, wire.CodeTab{ID: addr, Name: wt.Name, Terminals: []wire.CodeTerminal{wt}})
			}
			out.Spaces = append(out.Spaces, ws)
		}
	}
	for _, sp := range s.spaces {
		if len(projects) > 0 {
			break
		}
		ws := wire.CodeSpace{ID: sp.ID, Name: sp.Name, Folder: sp.Folder, Tabs: []wire.CodeTab{}}
		if ws.Name == "" {
			ws.Name = filepath.Base(sp.Folder)
		}
		for i, tab := range sp.Tabs {
			wt := wire.CodeTab{ID: tab.ID, Name: tab.Name, Terminals: []wire.CodeTerminal{}}
			if wt.Name == "" {
				wt.Name = fmt.Sprint(i + 1)
			}
			for _, addr := range layoutTerminals(tab.Root) {
				t, ok := s.terms[addr]
				if !ok || t.State != "live" {
					continue
				}
				placed[addr] = true
				wt.Terminals = append(wt.Terminals, s.wireTerminal(t))
			}
			ws.Tabs = append(ws.Tabs, wt)
		}
		out.Spaces = append(out.Spaces, ws)
	}
	for _, addr := range s.sortedTerminals() {
		t := s.terms[addr]
		if placed[addr] || t.State != "live" || !t.Local() || t.Parent != "" {
			continue
		}
		out.Loose = append(out.Loose, s.wireTerminal(t))
	}
	return out
}

// OpenWindow shows a target in the Gwen window: a tab such as project:<id>,
// session:<id>, terminal:<address>, or a screen (Gwen's agent, plan.md 3.16).
func (s *Service) OpenWindow(ctx context.Context, target string) error {
	if strings.ContainsAny(target, " \t\n") || strings.HasPrefix(target, "-") {
		return fail(wire.CodeInvalidRequest, "that isn't something Gwen's window can open")
	}
	return s.openWindow(ctx, target)
}

// TaskLabel is the terminal label that links a terminal to a task.
const TaskLabel = "gwen.task"

// LinkTask links a terminal to a task, as Hand it to an agent does.
func (s *Service) LinkTask(ctx context.Context, terminal, taskID string) error {
	s.mu.Lock()
	c := s.core
	s.mu.Unlock()
	if c == nil {
		return fail(wire.CodeUnavailable, "Gwen's terminal server isn't running")
	}
	return c.Label(ctx, terminal, map[string]string{TaskLabel: taskID})
}
