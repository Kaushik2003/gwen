package remote

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/herdr"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/stretchr/testify/require"
)

// A session's tab opens in the Gwen window (plan.md 3.15): gwend runs
// gwen-ui --open through systemd-run, under a unit name of systemd's choosing
// so a second open never collides with the window already running.
func TestOpenWindowRunsGwenUI(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, ".local", "bin")
	require.NoError(t, os.MkdirAll(bin, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bin, "gwen-ui"), []byte("#!/bin/sh\n"), 0o755))
	t.Setenv("PATH", t.TempDir())

	clk := clock.NewFake(testutil.At("09:00:00"))
	db := testutil.NewDBWithClock(t, clk)
	var unit string
	var argv []string
	svc, err := New(Options{DB: db, Repos: store.NewRepos(db), Clock: clk, Config: config.Defaults, Home: home,
		Launch: func(_ context.Context, u string, a []string) error {
			unit, argv = u, a
			return nil
		}})
	require.NoError(t, err)

	require.NoError(t, svc.openWindow(t.Context(), "terminal:t7"))
	require.Equal(t, "", unit)
	require.Equal(t, []string{filepath.Join(bin, "gwen-ui"), "--open", "terminal:t7"}, argv)
}

// From W16b each project with a folder is a space on the phone, with a tab per
// terminal inside its folder or labelled with it; the deepest folder wins.
func TestSpacesFromProjects(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	clk := clock.NewFake(testutil.At("09:00:00"))
	db := testutil.NewDBWithClock(t, clk)
	repos := store.NewRepos(db)
	svc, err := New(Options{DB: db, Repos: repos, Clock: clk, Config: config.Defaults, Home: home})
	require.NoError(t, err)
	gwen, err := repos.Projects.Create(t.Context(), "gwen", nil, nil)
	require.NoError(t, err)
	_, err = repos.Projects.SetFolder(t.Context(), gwen.ID, "/code/gwen")
	require.NoError(t, err)
	ui, err := repos.Projects.Create(t.Context(), "gwen ui", nil, nil)
	require.NoError(t, err)
	_, err = repos.Projects.SetFolder(t.Context(), ui.ID, "/code/gwen/ui")
	require.NoError(t, err)
	_, err = repos.Projects.Create(t.Context(), "no folder", nil, nil)
	require.NoError(t, err)

	svc.mu.Lock()
	svc.terms = map[string]herdr.Terminal{
		"t1": {Address: "t1", State: "live", Cwd: "/code/gwen/internal"},
		"t2": {Address: "t2", State: "live", Cwd: "/code/gwen/ui/src"},
		"t3": {Address: "t3", State: "live", Cwd: "/tmp", Labels: map[string]string{ProjectLabel: gwen.ID}},
		"t4": {Address: "t4", State: "live", Cwd: "/elsewhere"},
		"t5": {Address: "t5", State: "finished", Cwd: "/code/gwen"},
	}
	svc.mu.Unlock()

	got := svc.Spaces(t.Context())
	tabs := map[string][]string{}
	for _, sp := range got.Spaces {
		for _, tab := range sp.Tabs {
			tabs[sp.Name] = append(tabs[sp.Name], tab.ID)
		}
	}
	require.Equal(t, map[string][]string{"gwen": {"t1", "t3"}, "gwen ui": {"t2"}}, tabs)
	require.Len(t, got.Spaces, 2, "a project without a folder is no space")
	require.Len(t, got.Loose, 1)
	require.Equal(t, "t4", got.Loose[0].Terminal)
}
