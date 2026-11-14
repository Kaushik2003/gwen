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
)

// Gwen's own files in Gwen Terminal's home (plan.md 3.15 and 5.2): her three
// palettes as themes, so Gwen Terminal can wear the Gwen window's colours.
// They ship inside gwend; Install puts them in place when they differ.
//
// The first W16 also put a `gwen` mod (Gwen's screens) and a key that showed
// its space there. That mod is gone, so Install removes what it left behind.

//go:embed themes/*.json
var themeFiles embed.FS

// oldMod is the folder of the first W16's mod under the home's mods/, and
// oldModMark the file that tells it apart from a mod of the same name the
// person made themselves.
const (
	oldMod     = "gwen"
	oldModMark = "lib/gwen.js"
)

// oldKey and oldKeyAction are the binding the first W16 added to keys.json.
const (
	oldKey       = "ctrl-shift-g"
	oldKeyAction = "space.select:1"
)

// Install brings home (Gwen Terminal's folder, such as ~/.config/gwen-term)
// up to date with this build's themes and removes the first W16's mod and
// key. It reports whether it changed anything.
func Install(home string) (bool, error) {
	mod, err := removeOldMod(home)
	if err != nil {
		return false, fmt.Errorf("remove the old Gwen mod: %w", err)
	}
	themes, err := installThemes(home)
	if err != nil {
		return mod, fmt.Errorf("install Gwen's themes: %w", err)
	}
	key, err := removeOldKey(home)
	if err != nil {
		return mod || themes, fmt.Errorf("remove the old Gwen key: %w", err)
	}
	return mod || themes || key, nil
}

// removeOldMod deletes mods/gwen when it is the first W16's mod.
func removeOldMod(home string) (bool, error) {
	dir := filepath.Join(home, "mods", oldMod)
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(oldModMark))); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	// Renamed away first, so the window, which reloads a mod as its files
	// change, never sees half of it.
	tmp, err := os.MkdirTemp(home, ".gwen-mod-")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(tmp)
	if err := os.Rename(dir, filepath.Join(tmp, oldMod)); err != nil {
		return false, err
	}
	return true, nil
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

// oldKeyEntry finds the first W16's binding in keys.json with the comma and
// space that go with it: the entry and the comma after it, or else the comma
// before it, or else the entry alone.
var oldKeyEntry = []*regexp.Regexp{
	regexp.MustCompile(`"` + regexp.QuoteMeta(oldKey) + `"\s*:\s*"` + regexp.QuoteMeta(oldKeyAction) + `"\s*,\s*`),
	regexp.MustCompile(`,\s*"` + regexp.QuoteMeta(oldKey) + `"\s*:\s*"` + regexp.QuoteMeta(oldKeyAction) + `"`),
	regexp.MustCompile(`\s*"` + regexp.QuoteMeta(oldKey) + `"\s*:\s*"` + regexp.QuoteMeta(oldKeyAction) + `"`),
}

// removeOldKey takes the first W16's binding out of keys.json, as text so
// the rest of the file keeps its order and spacing. A binding the person
// changed, a missing file, and a broken one are left alone.
func removeOldKey(home string) (bool, error) {
	p := filepath.Join(home, "keys.json")
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	var keys struct {
		Global map[string]json.RawMessage `json:"global"`
	}
	if json.Unmarshal(b, &keys) != nil {
		return false, nil // the person's file, broken: the window reports it, and Gwen leaves it alone
	}
	var action string
	if raw, ok := keys.Global[oldKey]; !ok || json.Unmarshal(raw, &action) != nil || action != oldKeyAction {
		return false, nil
	}
	for _, re := range oldKeyEntry {
		loc := re.FindIndex(b)
		if loc == nil {
			continue
		}
		out := append(append([]byte{}, b[:loc[0]]...), b[loc[1]:]...)
		if json.Valid(out) {
			return true, writeAtomic(p, out)
		}
	}
	// Spelled in a way the patterns miss: rewrite the file without it.
	var all map[string]json.RawMessage
	if err := json.Unmarshal(b, &all); err != nil {
		return false, nil
	}
	delete(keys.Global, oldKey)
	raw, err := json.Marshal(keys.Global)
	if err != nil {
		return false, err
	}
	all["global"] = raw
	out, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return false, err
	}
	return true, writeAtomic(p, append(out, '\n'))
}

func writeAtomic(p string, b []byte) error {
	tmp := p + ".gwen-new"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
