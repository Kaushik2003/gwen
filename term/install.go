package term

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// Gwen's own files in Gwen Terminal's home (plan.md 3.15): the `gwen` mod
// that draws Gwen's screens, her three palettes as themes, and the key that
// shows the Gwen space. They ship inside gwend, so the screens always match
// the daemon that answers them; Install puts them in place when they differ.

//go:embed all:mods/gwen
var gwenMod embed.FS

//go:embed themes/*.json
var themeFiles embed.FS

// ModName is the folder of Gwen's mod under the home's mods/.
const ModName = "gwen"

// GwenKey shows the Gwen space, which gwend keeps first in the sidebar.
const GwenKey = "ctrl-shift-g"

// gwenKeyAction is what GwenKey does.
const gwenKeyAction = "space.select:1"

// Install brings home (Gwen Terminal's folder, such as ~/.config/gwen-term)
// up to date with this build's mod and themes, and adds GwenKey to an
// existing keys.json that does not bind it. It reports whether it changed
// anything.
func Install(home string) (bool, error) {
	changed, err := installMod(home)
	if err != nil {
		return false, fmt.Errorf("install Gwen's mod: %w", err)
	}
	themes, err := installThemes(home)
	if err != nil {
		return changed, fmt.Errorf("install Gwen's themes: %w", err)
	}
	key, err := addKey(home)
	if err != nil {
		return changed || themes, fmt.Errorf("add Gwen's key: %w", err)
	}
	return changed || themes || key, nil
}

// installMod replaces mods/gwen when any of its files differ. The new copy
// is written beside it and swapped in with renames, so the window, which
// reloads a mod as its files change, never loads half of it.
func installMod(home string) (bool, error) {
	return installTree(gwenMod, "mods/gwen", filepath.Join(home, "mods", ModName))
}

// installTree makes dst hold exactly the files under root in fsys, leaving
// out the type files Gwen Terminal writes itself (written): `gwen-term check`
// writes them into the folder it checks, so a build can hold them too.
func installTree(fsys fs.FS, root, dst string) (bool, error) {
	home := filepath.Dir(filepath.Dir(dst))
	if same, err := sameTree(fsys, root, dst); err != nil {
		return false, err
	} else if same {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return false, err
	}
	tmp, err := os.MkdirTemp(home, ".gwen-mod-")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(tmp)
	err = fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if skip := skipped(p, root, d); skip != nil || written(d.Name()) {
			return skip
		}
		rel := p[len(root):]
		out := filepath.Join(tmp, "gwen", filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		return os.WriteFile(out, b, 0o644)
	})
	if err != nil {
		return false, err
	}
	old := filepath.Join(tmp, "old")
	if err := os.Rename(dst, old); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	if err := os.Rename(filepath.Join(tmp, "gwen"), dst); err != nil {
		_ = os.Rename(old, dst)
		return false, err
	}
	return true, nil
}

// sameTree reports whether dir on disk holds exactly the files of root in
// fsys, written files aside on both sides.
func sameTree(fsys fs.FS, root, dir string) (bool, error) {
	want := map[string]bool{}
	same := true
	err := fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !same {
			return err
		}
		if skip := skipped(p, root, d); skip != nil || d.IsDir() || written(d.Name()) {
			return skip
		}
		rel := p[len(root)+1:]
		want[rel] = true
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		have, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil || !bytes.Equal(have, b) {
			same = false
		}
		return nil
	})
	if err != nil || !same {
		return false, err
	}
	// A file the build no longer has makes the copy different too.
	extra := false
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		if written(d.Name()) {
			return nil
		}
		if !want[filepath.ToSlash(rel)] {
			extra = true
			return filepath.SkipAll
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil && !extra, err
}

// written reports whether Gwen Terminal writes a file of this name into a
// mod folder itself (its type files for editors), which is no difference.
func written(name string) bool {
	return name == "jsconfig.json" || strings.HasSuffix(name, ".d.ts") || strings.HasPrefix(name, ".")
}

// skipped is fs.SkipDir for a hidden folder below root, nil otherwise.
func skipped(p, root string, d fs.DirEntry) error {
	if d.IsDir() && p != root && strings.HasPrefix(d.Name(), ".") {
		return fs.SkipDir
	}
	return nil
}

// installThemes writes themes/<name>.json for each of Gwen's palettes that
// is missing or differs.
func installThemes(home string) (bool, error) {
	entries, err := themeFiles.ReadDir("themes")
	if err != nil {
		return false, err
	}
	changed := false
	for _, e := range entries {
		b, err := themeFiles.ReadFile(path.Join("themes", e.Name()))
		if err != nil {
			return changed, err
		}
		dst := filepath.Join(home, "themes", e.Name())
		if have, err := os.ReadFile(dst); err == nil && bytes.Equal(have, b) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return changed, err
		}
		if err := writeAtomic(dst, b); err != nil {
			return changed, err
		}
		changed = true
	}
	return changed, nil
}

// addKey binds GwenKey in an existing keys.json that leaves it free. A
// missing file is left alone: the window writes its defaults on first start,
// and the key is added on the next run.
func addKey(home string) (bool, error) {
	p := filepath.Join(home, "keys.json")
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(b, &keys); err != nil {
		return false, nil // the person's file, broken: the window reports it, and Gwen leaves it alone
	}
	global := map[string]json.RawMessage{}
	if raw, ok := keys["global"]; ok {
		if err := json.Unmarshal(raw, &global); err != nil {
			return false, nil
		}
	}
	if _, taken := global[GwenKey]; taken {
		return false, nil
	}
	// Inserted as text after "global": {, so the rest of the file keeps its order and spacing.
	if loc := globalOpen.FindIndex(b); loc != nil {
		rest := bytes.TrimLeft(b[loc[1]:], " \t\r\n")
		entry := "\n    \"" + GwenKey + "\": \"" + gwenKeyAction + "\""
		if len(rest) > 0 && rest[0] != '}' {
			entry += ","
		}
		out := append(append(append([]byte{}, b[:loc[1]]...), entry...), b[loc[1]:]...)
		return true, writeAtomic(p, out)
	}
	global[GwenKey] = json.RawMessage(`"` + gwenKeyAction + `"`)
	raw, err := json.Marshal(global)
	if err != nil {
		return false, err
	}
	keys["global"] = raw
	out, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return false, err
	}
	return true, writeAtomic(p, append(out, '\n'))
}

// globalOpen finds the start of keys.json's "global" section.
var globalOpen = regexp.MustCompile(`"global"\s*:\s*\{`)

func writeAtomic(p string, b []byte) error {
	tmp := p + ".gwen-new"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
