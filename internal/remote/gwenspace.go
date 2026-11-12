package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"time"

	"github.com/kzark/gwen/internal/wire"
	"github.com/kzark/gwen/term"
)

// The Gwen space (plan.md 3.15): the first space of the Gwen window, whose
// tabs are Gwen's screens. Each tab is one pane of the `gwen` mod with
// props {screen}. gwend makes the space when the window first opens, and
// brings back a screen's tab when it is asked for after being closed.

// GwenMod is the mod that draws Gwen's screens.
const GwenMod = "gwen"

// gwenSpaceName is what the space is called in the sidebar.
const gwenSpaceName = "Gwen"

// gwenScreen is one tab of the Gwen space.
type gwenScreen struct{ id, name string }

// gwenScreens are the tabs in order (plan.md 5.3). Work joins with W21.
var gwenScreens = []gwenScreen{
	{"today", "Today"}, {"gwen", "Gwen"}, {"tasks", "Tasks"}, {"plan", "Plan"}, {"review", "Review"},
	{"connections", "Connections"}, {"memory", "Memory"}, {"settings", "Settings"},
}

// screenAliases maps the names the tray, the panel widget, and old links use
// to a tab and the sub-screen inside it.
var screenAliases = map[string][2]string{
	"":          {"today", ""},
	"assistant": {"gwen", ""},
	"inbox":     {"tasks", "inbox"},
	"board":     {"tasks", "board"},
	"projects":  {"tasks", "projects"},
	"goals":     {"plan", "goals"},
	"weekly":    {"review", "weekly"},
	"stats":     {"review", "stats"},
	"history":   {"review", "history"},
}

// resolveScreen is the tab a screen name opens and the sub-screen inside
// it, or false for a name Gwen does not know.
func resolveScreen(name string) (tab, sub string, ok bool) {
	if a, found := screenAliases[name]; found {
		return a[0], a[1], true
	}
	for _, sc := range gwenScreens {
		if sc.id == name {
			return name, "", true
		}
	}
	return "", "", false
}

// modScreen is the screen a tab shows when its only content is the gwen
// mod, "" otherwise.
func modScreen(root json.RawMessage) string {
	var found string
	var walk func(v any)
	walk = func(v any) {
		switch n := v.(type) {
		case map[string]any:
			if mod, _ := n["mod"].(string); mod == GwenMod {
				if props, ok := n["props"].(map[string]any); ok {
					if sc, _ := props["screen"].(string); sc != "" && found == "" {
						found = sc
					}
				}
			}
			for _, inner := range n {
				walk(inner)
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
	return found
}

// gwenSpace finds the Gwen space in the window's document: the first space
// with a tab showing one of Gwen's screens. It returns its id and the tab
// of each screen it has.
func gwenSpace(spaces []docSpace) (id string, tabs map[string]string) {
	for _, sp := range spaces {
		tabs = map[string]string{}
		for _, tab := range sp.Tabs {
			if sc := modScreen(tab.Root); sc != "" {
				if _, dup := tabs[sc]; !dup {
					tabs[sc] = tab.ID
				}
			}
		}
		if len(tabs) > 0 {
			return sp.ID, tabs
		}
	}
	return "", nil
}

// layout is the window's layout as the core holds it, once that copy is in
// step with what the window shows. The window saves it to the core a moment
// after each change, and a core that has just started holds none until the
// window first saves: a Gwen space made from a copy that is behind would be
// a second one. It waits up to wait for the copy to catch up, then fails
// rather than guess.
func (s *Service) layout(ctx context.Context, wait time.Duration) ([]docSpace, error) {
	deadline := time.Now().Add(wait)
	for {
		spaces, err := s.coreLayout(ctx)
		if err == nil {
			var live map[string][]string
			if live, err = s.liveLayout(ctx); err != nil {
				return nil, err
			}
			if inStep(spaces, live) {
				return spaces, nil
			}
			err = fail(wire.CodeUnavailable, "the Gwen window hasn't saved its layout yet; try again in a moment")
		}
		if !time.Now().Before(deadline) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
}

// coreLayout reads the window's layout from the core now. Without the
// service's connection (the core started a moment ago and the next try to
// connect is still waiting), it connects for this one read.
func (s *Service) coreLayout(ctx context.Context) ([]docSpace, error) {
	s.mu.Lock()
	c := s.core
	s.mu.Unlock()
	if c == nil {
		dialCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		once, err := s.dial(dialCtx)
		cancel()
		if err != nil {
			return nil, fail(wire.CodeUnavailable, "the Gwen window's core isn't answering")
		}
		defer once.Close()
		c = once
	}
	doc, err := c.Doc(ctx, "herdr-app")
	if err != nil {
		return nil, fail(wire.CodeUnavailable, "the Gwen window's core didn't give its layout: %v", err)
	}
	return parseSpaces(doc), nil
}

// liveLayout is what the window shows now: the tab ids of each of this
// computer's spaces, by space id.
func (s *Service) liveLayout(ctx context.Context) (map[string][]string, error) {
	reply, err := s.termRequest(ctx, map[string]any{"ls": true})
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(reply)
	if err != nil {
		return nil, err
	}
	var w struct {
		Spaces []struct {
			ID      string `json:"id"`
			Machine string `json:"machine"`
			Tabs    []struct {
				ID string `json:"id"`
			} `json:"tabs"`
		} `json:"spaces"`
	}
	if err := json.Unmarshal(raw, &w); err != nil {
		return nil, fmt.Errorf("the Gwen window listed its spaces in a way Gwen can't read: %w", err)
	}
	live := make(map[string][]string, len(w.Spaces))
	for _, sp := range w.Spaces {
		if sp.Machine != "" {
			continue // another machine's space lives in that machine's core
		}
		tabs := make([]string, 0, len(sp.Tabs))
		for _, t := range sp.Tabs {
			tabs = append(tabs, t.ID)
		}
		live[sp.ID] = tabs
	}
	return live, nil
}

// inStep reports whether the core's copy of the layout has exactly the
// spaces and tabs the window shows.
func inStep(doc []docSpace, live map[string][]string) bool {
	if len(doc) != len(live) {
		return false
	}
	for _, sp := range doc {
		tabs, ok := live[sp.ID]
		if !ok || len(tabs) != len(sp.Tabs) {
			return false
		}
		shown := make(map[string]bool, len(tabs))
		for _, id := range tabs {
			shown[id] = true
		}
		for _, t := range sp.Tabs {
			if !shown[t.ID] {
				return false
			}
		}
	}
	return true
}

// ensureGwenSpace makes the Gwen space with all its tabs when the window has
// none, or else the tab named by want when it was closed, and returns the
// space and its tabs. It waits up to wait for the window's layout (layout).
func (s *Service) ensureGwenSpace(ctx context.Context, want string, wait time.Duration) (space string, tabs map[string]string, err error) {
	spaces, err := s.layout(ctx, wait)
	if err != nil {
		return "", nil, err
	}
	space, tabs = gwenSpace(spaces)
	created := false
	shell := ""
	if space == "" {
		reply, err := s.termDo(ctx, "space.new", map[string]any{"name": gwenSpaceName, "folder": s.home})
		if err != nil {
			return "", nil, err
		}
		space, _ = reply["space"].(string)
		shell, _ = reply["tab"].(string)
		if space == "" {
			return "", nil, fail(wire.CodeInternal, "the Gwen window made no space")
		}
		tabs = map[string]string{}
		created = true
	}
	for i, sc := range gwenScreens {
		if _, ok := tabs[sc.id]; ok || (!created && sc.id != want) {
			continue
		}
		reply, err := s.termDo(ctx, "tab.new", map[string]any{"space": space, "name": sc.name, "mod": GwenMod,
			"props": map[string]any{"screen": sc.id}})
		if err != nil {
			return "", nil, err
		}
		tab, _ := reply["tab"].(string)
		tabs[sc.id] = tab
		if !created && tab != "" {
			// A tab that was closed comes back where it was: after the screens before it that are still open.
			index := 1
			for _, before := range gwenScreens[:i] {
				if _, ok := tabs[before.id]; ok {
					index++
				}
			}
			_, _ = s.termDo(ctx, "tab.move", map[string]any{"id": tab, "index": index})
		}
	}
	if created {
		if shell != "" {
			// space.new starts with a shell tab; the Gwen space has only Gwen's screens.
			_, _ = s.termDo(ctx, "tab.close", map[string]any{"id": shell, "confirmed": true})
		}
		_, _ = s.termDo(ctx, "space.move", map[string]any{"id": space, "index": 1})
	}
	return space, tabs, nil
}

// OpenGwen shows one of Gwen's screens in the Gwen window: it starts the
// window when it is closed, makes the Gwen space or the screen's tab when
// they are missing, selects them, and brings the window to the front.
func (s *Service) OpenGwen(ctx context.Context, screen string) error {
	tab, sub, ok := resolveScreen(screen)
	if !ok {
		return fail(wire.CodeInvalidRequest, "Gwen has no screen called %q", screen)
	}
	s.gwenMu.Lock()
	defer s.gwenMu.Unlock()
	if err := s.ensureWindow(ctx); err != nil {
		return err
	}
	s.windowSeen.Store(true)
	// A window started just now restores its layout first: wait for it.
	space, tabs, err := s.ensureGwenSpace(ctx, tab, 8*time.Second)
	if err != nil {
		s.windowSeen.Store(false) // the watcher tries again
		return err
	}
	if _, err := s.termDo(ctx, "space.select", map[string]any{"id": space}); err != nil {
		return err
	}
	if id := tabs[tab]; id != "" {
		if _, err := s.termDo(ctx, "tab.select", map[string]any{"id": id}); err != nil {
			return err
		}
	}
	if sub != "" {
		// The mod's own command: the shown screen switches to its sub-screen.
		_, _ = s.termDo(ctx, GwenMod+".show:"+sub, map[string]any{})
	}
	s.raiseWindow(ctx)
	return nil
}

// raiseWindow brings the open window to the front: a second gwen-term asks
// the running one to raise itself and exits.
func (s *Service) raiseWindow(ctx context.Context) {
	if s.opts.Launch != nil {
		return
	}
	bin := termBin(s.home)
	if bin == "" {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, bin).Run()
}

// watchWindow makes the Gwen space the first time the window is seen open
// since it was last closed, so a window started by hand has it too.
func (s *Service) watchWindow(ctx context.Context) {
	open := s.termOpen(ctx)
	if !open {
		s.windowSeen.Store(false)
		return
	}
	if s.windowSeen.Swap(true) {
		return
	}
	// The window writes its keys.json on its first start, after gwend put
	// Gwen's files in place: add her key now that it is there.
	if _, err := term.Install(termHome(s.home)); err != nil {
		slog.Warn("Gwen's screens in the Gwen window", "err", err)
	}
	s.gwenMu.Lock()
	defer s.gwenMu.Unlock()
	if _, _, err := s.ensureGwenSpace(ctx, "", time.Second); err != nil {
		// A window that has just started saves its layout a moment later:
		// try again on the next tick, and say so only when it keeps failing.
		s.windowSeen.Store(false)
		if s.spaceMisses++; s.spaceMisses == 5 {
			slog.Warn("the Gwen space", "err", err)
		}
		return
	}
	s.spaceMisses = 0
}
