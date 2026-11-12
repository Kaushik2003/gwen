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
	"sort"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/herdr"
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

// CoreBin is the herdr-core Gwen built for herself (`make term` installs it
// in ~/.local/lib/gwen, apart from any Herdr of yours), "" when there is none.
func CoreBin(home string) string {
	p := filepath.Join(home, ".local", "lib", "gwen", "herdr-core")
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return p
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

// launch starts a long-lived program as a transient user unit.
func (s *Service) launch(ctx context.Context, unit string, argv ...string) error {
	if s.opts.Launch != nil {
		return s.opts.Launch(ctx, unit, argv)
	}
	args := []string{"--user", "--collect", "--quiet"}
	if unit == coreUnit {
		// `hc server start` puts the core in the background and returns. As a
		// one-shot unit that stays active, what it left running is kept: the
		// core, and every terminal in it, must outlive the command and any
		// window. It gets a name of systemd's choosing, so a core that died
		// can always be started again.
		args = append(args, "-p", "Type=oneshot", "-p", "RemainAfterExit=yes", "--description", "Gwen's herdr-core")
	} else {
		args = append(args, "--unit", unit)
	}
	args = append(append(args, "--"), argv...)
	out, err := exec.CommandContext(ctx, "systemd-run", args...).CombinedOutput()
	if err != nil {
		// A unit left over from a crash blocks the name; clear it and retry once.
		if strings.Contains(string(out), "already") {
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

// spaceForDir is the id of the window's space whose folder is dir, "" for
// none.
func (s *Service) spaceForDir(dir string) string {
	for _, sp := range s.spacesDoc() {
		if sp.Folder != "" && filepath.Clean(ExpandHome(sp.Folder, s.home)) == dir {
			return sp.ID
		}
	}
	return ""
}

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

// Spaces lists Gwen Terminal's spaces, tabs and terminals, with its theme.
func (s *Service) Spaces() wire.CodeSpaces {
	theme := s.Theme()
	s.mu.Lock()
	defer s.mu.Unlock()
	out := wire.CodeSpaces{Spaces: []wire.CodeSpace{}, Loose: []wire.CodeTerminal{}, Theme: theme}
	placed := map[string]bool{}
	for _, sp := range s.spaces {
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
