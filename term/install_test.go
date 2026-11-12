package term

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func TestInstallPutsTheModAndThemesInPlace(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	changed, err := Install(home)
	require.NoError(t, err)
	require.True(t, changed)
	for _, f := range []string{"mods/gwen/main.js", "mods/gwen/app.js", "mods/gwen/mod.json", "mods/gwen/lib/gwen.js", "themes/ghost.json", "themes/tide.json", "themes/ember.json"} {
		require.FileExists(t, filepath.Join(home, f))
	}
	require.NoFileExists(t, filepath.Join(home, "keys.json"), "a missing keys.json is the window's to write")

	changed, err = Install(home)
	require.NoError(t, err)
	require.False(t, changed, "nothing differs the second time")

	// The window writes type files for editors into a mod folder: no difference.
	require.NoError(t, os.WriteFile(filepath.Join(home, "mods/gwen/jsconfig.json"), []byte("{}"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(home, "mods/gwen/lib/gpui-kit.d.ts"), []byte("x"), 0o644))
	changed, err = Install(home)
	require.NoError(t, err)
	require.False(t, changed, "files the window writes are not Gwen's to remove")

	// A changed or extra file is put back as the build has it.
	main := filepath.Join(home, "mods/gwen/main.js")
	require.NoError(t, os.WriteFile(main, []byte("broken"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(home, "mods/gwen/stray.js"), []byte("x"), 0o644))
	changed, err = Install(home)
	require.NoError(t, err)
	require.True(t, changed)
	b, err := os.ReadFile(main)
	require.NoError(t, err)
	require.NotEqual(t, "broken", string(b))
	require.NoFileExists(t, filepath.Join(home, "mods/gwen/stray.js"))
	entries, err := os.ReadDir(home)
	require.NoError(t, err)
	for _, e := range entries {
		require.NotContains(t, e.Name(), ".gwen-mod-", "the staging folder is removed")
	}
}

func TestInstallAddsTheKeyOnce(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ in, want string }{
		"some keys": {`{
  "global": {
    "alt-1": "space.select:1"
  }
}
`, `{
  "global": {
    "ctrl-shift-g": "space.select:1",
    "alt-1": "space.select:1"
  }
}
`},
		"empty global": {`{"global": {}}`, "{\"global\": {\n    \"ctrl-shift-g\": \"space.select:1\"}}"},
		"taken":        {`{"global": {"ctrl-shift-g": "tab.new"}}`, `{"global": {"ctrl-shift-g": "tab.new"}}`},
		"no global":    {`{"terminal": {}}`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			p := filepath.Join(home, "keys.json")
			require.NoError(t, os.WriteFile(p, []byte(tc.in), 0o644))
			_, err := Install(home)
			require.NoError(t, err)
			b, err := os.ReadFile(p)
			require.NoError(t, err)
			if tc.want != "" {
				require.Equal(t, tc.want, string(b))
			}
			var keys map[string]map[string]string
			require.NoError(t, json.Unmarshal(b, &keys), "still valid JSON")
			if name != "taken" {
				require.Equal(t, "space.select:1", keys["global"]["ctrl-shift-g"])
			}
			// Running again changes nothing.
			_, err = Install(home)
			require.NoError(t, err)
			again, err := os.ReadFile(p)
			require.NoError(t, err)
			require.Equal(t, string(b), string(again))
		})
	}
}

// `gwen-term check` writes type files into the folder it checks, so a build
// made after checking the mod in the repo holds them: they are not installed,
// and they make no difference.
func TestInstallLeavesOutTypeFilesInTheBuild(t *testing.T) {
	t.Parallel()
	build := fstest.MapFS{
		"mods/gwen/main.js":              {Data: []byte("export {}")},
		"mods/gwen/lib/kit.js":           {Data: []byte("kit")},
		"mods/gwen/jsconfig.json":        {Data: []byte("{}")},
		"mods/gwen/gpui-kit.d.ts":        {Data: []byte("types")},
		"mods/gwen/lib/gpui-kit.d.ts":    {Data: []byte("types")},
		"mods/gwen/.cache/anything.json": {Data: []byte("x")},
	}
	dst := filepath.Join(t.TempDir(), "mods", "gwen")
	changed, err := installTree(build, "mods/gwen", dst)
	require.NoError(t, err)
	require.True(t, changed)
	require.FileExists(t, filepath.Join(dst, "main.js"))
	require.FileExists(t, filepath.Join(dst, "lib", "kit.js"))
	for _, f := range []string{"jsconfig.json", "gpui-kit.d.ts", "lib/gpui-kit.d.ts", ".cache"} {
		require.NoFileExists(t, filepath.Join(dst, f))
		require.NoDirExists(t, filepath.Join(dst, f))
	}
	changed, err = installTree(build, "mods/gwen", dst)
	require.NoError(t, err)
	require.False(t, changed, "the type files in the build make no difference")
}
