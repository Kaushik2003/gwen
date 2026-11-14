package term

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInstallPutsTheThemesInPlace(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	changed, err := Install(home)
	require.NoError(t, err)
	require.True(t, changed)
	for _, f := range []string{"themes/ghost.json", "themes/tide.json", "themes/ember.json"} {
		require.FileExists(t, filepath.Join(home, f))
	}
	require.NoFileExists(t, filepath.Join(home, "keys.json"), "a missing keys.json is the window's to write")
	require.NoDirExists(t, filepath.Join(home, "mods"))

	changed, err = Install(home)
	require.NoError(t, err)
	require.False(t, changed, "nothing differs the second time")

	// A changed theme is put back as the build has it.
	ghost := filepath.Join(home, "themes/ghost.json")
	require.NoError(t, os.WriteFile(ghost, []byte("broken"), 0o644))
	changed, err = Install(home)
	require.NoError(t, err)
	require.True(t, changed)
	b, err := os.ReadFile(ghost)
	require.NoError(t, err)
	require.True(t, json.Valid(b))
}

func TestInstallRemovesTheOldMod(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	old := filepath.Join(home, "mods", "gwen")
	require.NoError(t, os.MkdirAll(filepath.Join(old, "lib"), 0o755))
	for _, f := range []string{"main.js", "mod.json", "lib/gwen.js", "lib/gpui-kit.d.ts"} {
		require.NoError(t, os.WriteFile(filepath.Join(old, f), []byte("x"), 0o644))
	}
	other := filepath.Join(home, "mods", "agents", "main.js")
	require.NoError(t, os.MkdirAll(filepath.Dir(other), 0o755))
	require.NoError(t, os.WriteFile(other, []byte("x"), 0o644))

	changed, err := Install(home)
	require.NoError(t, err)
	require.True(t, changed)
	require.NoDirExists(t, old)
	require.FileExists(t, other, "other mods stay")
	entries, err := os.ReadDir(home)
	require.NoError(t, err)
	for _, e := range entries {
		require.NotContains(t, e.Name(), ".gwen-mod-", "the staging folder is removed")
	}
}

func TestInstallKeepsAModOfTheSameName(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	mine := filepath.Join(home, "mods", "gwen", "main.js")
	require.NoError(t, os.MkdirAll(filepath.Dir(mine), 0o755))
	require.NoError(t, os.WriteFile(mine, []byte("x"), 0o644))
	_, err := Install(home)
	require.NoError(t, err)
	require.FileExists(t, mine, "a gwen mod without Gwen's bridge is the person's own")
}

func TestInstallRemovesTheOldKey(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		in, want string
		changed  bool
	}{
		"first of several": {`{
  "global": {
    "ctrl-shift-g": "space.select:1",
    "alt-1": "space.select:1"
  }
}
`, `{
  "global": {
    "alt-1": "space.select:1"
  }
}
`, true},
		"last of several": {`{
  "global": {
    "alt-1": "space.select:1",
    "ctrl-shift-g": "space.select:1"
  },
  "terminal": {}
}
`, `{
  "global": {
    "alt-1": "space.select:1"
  },
  "terminal": {}
}
`, true},
		"the only one":    {"{\"global\": {\n    \"ctrl-shift-g\": \"space.select:1\"}}", `{"global": {}}`, true},
		"changed by hand": {`{"global": {"ctrl-shift-g": "tab.new"}}`, `{"global": {"ctrl-shift-g": "tab.new"}}`, false},
		"no global":       {`{"terminal": {}}`, `{"terminal": {}}`, false},
		"broken":          {`{"global": `, `{"global": `, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			p := filepath.Join(home, "keys.json")
			require.NoError(t, os.WriteFile(p, []byte(tc.in), 0o644))
			changed, err := removeOldKey(home)
			require.NoError(t, err)
			require.Equal(t, tc.changed, changed)
			b, err := os.ReadFile(p)
			require.NoError(t, err)
			require.Equal(t, tc.want, string(b))
			// Running again changes nothing.
			changed, err = removeOldKey(home)
			require.NoError(t, err)
			require.False(t, changed)
		})
	}
}
