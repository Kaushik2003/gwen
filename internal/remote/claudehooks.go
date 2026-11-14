package remote

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// hookEvents are the Claude Code events Gwen listens to, in the order they
// are written (plan.md 3.14.6). matcher is "" for none.
var hookEvents = []struct{ name, matcher string }{
	{"SessionStart", ""}, {"PreToolUse", ""}, {"PostToolUse", "Edit|Write|MultiEdit|NotebookEdit"},
	{"PermissionRequest", ""}, {"Stop", ""}, {"SessionEnd", ""},
}

// permissionTimeout is how long Claude Code waits for the PermissionRequest
// hook, in seconds: longer than Gwen ever holds one.
const permissionTimeout = 900

// HooksState is what ~/.claude/settings.json says about Gwen's hooks.
type HooksState struct {
	// On: every event has Gwen's hook.
	On bool
	// Note says why the hooks cannot work, "" when they can.
	Note string
}

// ClaudeSettingsPath is Claude Code's user settings file.
func ClaudeSettingsPath(home string) string { return filepath.Join(home, ".claude", "settings.json") }

// HookCommand is the command Claude Code runs for an event.
func HookCommand(gwenBin string) string { return gwenBin + " hook" }

// ReadHooks reports whether Gwen's hooks are in the settings file.
func ReadHooks(path, gwenBin string) HooksState {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return HooksState{}
	}
	if err != nil {
		return HooksState{Note: "Gwen can't read Claude Code's settings file."}
	}
	var doc struct {
		Hooks           map[string][]hookGroup `json:"hooks"`
		DisableAllHooks bool                   `json:"disableAllHooks"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return HooksState{Note: "Claude Code's settings file isn't valid JSON, so Gwen left it alone."}
	}
	on := true
	for _, ev := range hookEvents {
		on = on && hasHook(doc.Hooks[ev.name], HookCommand(gwenBin))
	}
	st := HooksState{On: on}
	if doc.DisableAllHooks {
		st.Note = "Claude Code hooks are turned off in your settings."
	}
	return st
}

type hookGroup struct {
	Matcher string      `json:"matcher,omitempty"`
	Hooks   []hookEntry `json:"hooks"`
}

type hookEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

func hasHook(groups []hookGroup, command string) bool {
	for _, g := range groups {
		for _, h := range g.Hooks {
			if h.Type == "command" && h.Command == command {
				return true
			}
		}
	}
	return false
}

// SetHooks adds or removes Gwen's hooks in the settings file. Every other
// key keeps its place and every other hook stays as it is. The first change
// keeps a copy at <path>.gwen-backup. A file that is not valid JSON is left
// alone and reported.
func SetHooks(path, gwenBin string, on bool) error {
	data, err := os.ReadFile(path)
	missing := errors.Is(err, fs.ErrNotExist)
	if err != nil && !missing {
		return fmt.Errorf("read Claude Code's settings: %w", err)
	}
	if missing {
		if !on {
			return nil
		}
		data = []byte("{}")
	}
	doc, err := decodeOrdered(data)
	if err != nil {
		return errors.New("the settings file of Claude Code isn't valid JSON, so Gwen left it alone")
	}
	hooks := &ordered{}
	if raw, ok := doc.get("hooks"); ok {
		if hooks, err = decodeOrdered(raw); err != nil {
			return errors.New("the hooks in Claude Code's settings aren't an object, so Gwen left them alone")
		}
	}
	command := HookCommand(gwenBin)
	for _, ev := range hookEvents {
		var groups []json.RawMessage
		if raw, ok := hooks.get(ev.name); ok {
			if err := json.Unmarshal(raw, &groups); err != nil {
				return fmt.Errorf("the %s hooks in Claude Code's settings aren't a list, so Gwen left them alone", ev.name)
			}
		}
		groups = withoutCommand(groups, command)
		if on {
			entry := map[string]any{"type": "command", "command": command}
			if ev.name == "PermissionRequest" {
				entry["timeout"] = permissionTimeout
			} else {
				entry["async"] = true
			}
			group := map[string]any{"hooks": []any{entry}}
			if ev.matcher != "" {
				group["matcher"] = ev.matcher
			}
			raw, _ := marshalPlain(group)
			groups = append(groups, raw)
		}
		if len(groups) == 0 {
			hooks.remove(ev.name)
			continue
		}
		raw, _ := marshalPlain(groups)
		hooks.set(ev.name, raw)
	}
	if len(hooks.keys) == 0 {
		doc.remove("hooks")
	} else {
		raw, err := hooks.encode()
		if err != nil {
			return err
		}
		doc.set("hooks", raw)
	}
	out, err := doc.encode()
	if err != nil {
		return err
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, out, "", "  "); err != nil {
		return err
	}
	pretty.WriteByte('\n')
	if !missing {
		backup := path + ".gwen-backup"
		if _, err := os.Stat(backup); errors.Is(err, fs.ErrNotExist) {
			if err := os.WriteFile(backup, data, 0o600); err != nil {
				return fmt.Errorf("back up Claude Code's settings: %w", err)
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".settings-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(pretty.Bytes()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	mode := fs.FileMode(0o600)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// withoutCommand drops Gwen's entries: a group whose only hooks are command
// goes, and a group with other hooks loses just that one.
func withoutCommand(groups []json.RawMessage, command string) []json.RawMessage {
	out := groups[:0:0]
	for _, raw := range groups {
		g, err := decodeOrdered(raw)
		if err != nil {
			out = append(out, raw)
			continue
		}
		var entries []json.RawMessage
		if h, ok := g.get("hooks"); !ok || json.Unmarshal(h, &entries) != nil {
			out = append(out, raw)
			continue
		}
		kept := entries[:0:0]
		for _, e := range entries {
			var h hookEntry
			if json.Unmarshal(e, &h) == nil && h.Type == "command" && h.Command == command {
				continue
			}
			kept = append(kept, e)
		}
		if len(kept) == len(entries) {
			out = append(out, raw)
			continue
		}
		if len(kept) == 0 {
			continue
		}
		h, _ := marshalPlain(kept)
		g.set("hooks", h)
		if enc, err := g.encode(); err == nil {
			out = append(out, enc)
		}
	}
	return out
}

// ordered is a JSON object that keeps its keys in order, so that rewriting
// one key of a settings file leaves the rest where the person put it.
type ordered struct {
	keys   []string
	values map[string]json.RawMessage
}

func decodeOrdered(data []byte) (*ordered, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, errors.New("not an object")
	}
	o := &ordered{values: map[string]json.RawMessage{}}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, errors.New("bad key")
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		if _, dup := o.values[key]; !dup {
			o.keys = append(o.keys, key)
		}
		o.values[key] = v
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, errors.New("trailing data")
	}
	return o, nil
}

func (o *ordered) get(key string) (json.RawMessage, bool) {
	if o.values == nil {
		return nil, false
	}
	v, ok := o.values[key]
	return v, ok
}

func (o *ordered) set(key string, v json.RawMessage) {
	if o.values == nil {
		o.values = map[string]json.RawMessage{}
	}
	if _, ok := o.values[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.values[key] = v
}

func (o *ordered) remove(key string) {
	if _, ok := o.values[key]; !ok {
		return
	}
	delete(o.values, key)
	for i, k := range o.keys {
		if k == key {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

func (o *ordered) encode() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		key, err := marshalPlain(k)
		if err != nil {
			return nil, err
		}
		b.Write(key)
		b.WriteByte(':')
		b.Write(o.values[k])
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// marshalPlain is json.Marshal without HTML escaping: a hook command such as
// `a 2>&1 && b` stays as the person wrote it instead of turning into \u003e
// and \u0026.
func marshalPlain(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}
