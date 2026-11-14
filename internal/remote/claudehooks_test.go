package remote

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// The hooks switch edits a file that is the person's own, Claude Code's
// ~/.claude/settings.json (plan.md 3.14.6): every other key keeps its place,
// every other hook stays as it is, and a file Gwen can't read as JSON is
// never touched.

const (
	w27cGwen     = "/home/user/.local/bin/gwen"
	w27cGwenHook = w27cGwen + " hook"
)

// w27cSettings is a settings file with hooks of the person's own, indented
// the way Claude Code writes it.
const w27cSettings = `{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "model": "opus",
  "permissions": {
    "allow": [
      "Bash(go test:*)"
    ],
    "deny": []
  },
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "~/bin/guard.sh",
            "timeout": 5
          }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "notify-send 'Claude stopped'"
          }
        ]
      }
    ],
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "log-prompt"
          }
        ]
      }
    ]
  },
  "statusLine": {
    "type": "command",
    "command": "~/.claude/statusline.sh"
  },
  "enabledPlugins": {
    "notes@market": true
  }
}
`

// w27cWriteSettings writes a settings file in a fresh folder.
func w27cWriteSettings(t *testing.T, content string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".claude", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), mode))
	return path
}

// w27cKeys are an object's keys in the file's order.
func w27cKeys(t *testing.T, raw []byte) []string {
	t.Helper()
	o, err := decodeOrdered(raw)
	require.NoError(t, err)
	return o.keys
}

// w27cGroups are an event's hook groups, decoded.
func w27cGroups(t *testing.T, file []byte, event string) []any {
	t.Helper()
	var doc struct {
		Hooks map[string][]any `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal(file, &doc))
	return doc.Hooks[event]
}

func w27cDecode(t *testing.T, s string) any {
	t.Helper()
	var v any
	require.NoError(t, json.Unmarshal([]byte(s), &v))
	return v
}

func TestSetHooksKeepsForeignHooksAndKeyOrder(t *testing.T) {
	t.Parallel()
	path := w27cWriteSettings(t, w27cSettings, 0o644)
	require.Equal(t, HooksState{}, ReadHooks(path, w27cGwen))

	require.NoError(t, SetHooks(path, w27cGwen, true))
	on, err := os.ReadFile(path)
	require.NoError(t, err)

	require.Equal(t, []string{"$schema", "model", "permissions", "hooks", "statusLine", "enabledPlugins"}, w27cKeys(t, on))
	top, err := decodeOrdered(on)
	require.NoError(t, err)
	hooks, _ := top.get("hooks")
	require.Equal(t, []string{"PreToolUse", "Stop", "UserPromptSubmit", "SessionStart", "PostToolUse", "PermissionRequest", "SessionEnd"},
		w27cKeys(t, hooks), "the person's events keep their place and Gwen's new ones follow")

	// The person's hooks come first and unchanged; Gwen's follow.
	gwenAsync := w27cDecode(t, `{"hooks":[{"type":"command","command":"`+w27cGwenHook+`","async":true}]}`)
	require.Equal(t, []any{
		w27cDecode(t, `{"matcher":"Bash","hooks":[{"type":"command","command":"~/bin/guard.sh","timeout":5}]}`),
		gwenAsync,
	}, w27cGroups(t, on, "PreToolUse"))
	require.Equal(t, []any{
		w27cDecode(t, `{"hooks":[{"type":"command","command":"notify-send 'Claude stopped'"}]}`),
		gwenAsync,
	}, w27cGroups(t, on, "Stop"))
	require.Equal(t, []any{w27cDecode(t, `{"hooks":[{"type":"command","command":"log-prompt"}]}`)}, w27cGroups(t, on, "UserPromptSubmit"),
		"an event Gwen doesn't use is left alone")
	require.Equal(t, []any{gwenAsync}, w27cGroups(t, on, "SessionStart"))
	require.Equal(t, []any{gwenAsync}, w27cGroups(t, on, "SessionEnd"))
	require.Equal(t, []any{w27cDecode(t, `{"matcher":"Edit|Write|MultiEdit|NotebookEdit","hooks":[{"type":"command","command":"`+w27cGwenHook+`","async":true}]}`)},
		w27cGroups(t, on, "PostToolUse"))
	require.Equal(t, []any{w27cDecode(t, `{"hooks":[{"type":"command","command":"`+w27cGwenHook+`","timeout":900}]}`)},
		w27cGroups(t, on, "PermissionRequest"), "the permission hook waits for Gwen, longer than Gwen ever holds one")

	// Every other key is as it was.
	var before, after map[string]any
	require.NoError(t, json.Unmarshal([]byte(w27cSettings), &before))
	require.NoError(t, json.Unmarshal(on, &after))
	delete(before, "hooks")
	delete(after, "hooks")
	require.Equal(t, before, after)

	require.Equal(t, HooksState{On: true}, ReadHooks(path, w27cGwen))
	backup, err := os.ReadFile(path + ".gwen-backup")
	require.NoError(t, err)
	require.Equal(t, w27cSettings, string(backup), "the first change keeps a copy of the file as it was")
	st, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o644), st.Mode().Perm(), "the file keeps its mode")

	// On again changes nothing: no second entry, and the copy stays the
	// original.
	require.NoError(t, SetHooks(path, w27cGwen, true))
	again, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(on), string(again))
	backup, err = os.ReadFile(path + ".gwen-backup")
	require.NoError(t, err)
	require.Equal(t, w27cSettings, string(backup))

	// Off gives back the file as it was.
	require.NoError(t, SetHooks(path, w27cGwen, false))
	off, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, w27cSettings, string(off))
	require.Equal(t, HooksState{}, ReadHooks(path, w27cGwen))

	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, entries, 2, "no temporary file is left behind: %v", entries)
}

// A hook command full of shell is common (2>&1, &&, <file). Gwen rewrites
// the file, but the person's commands must read as they wrote them.
func TestSetHooksKeepsShellCharactersReadable(t *testing.T) {
	t.Parallel()
	const settings = `{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "make lint 2>&1 | tee /tmp/lint.log && echo ok < /dev/null"
          }
        ]
      }
    ]
  }
}
`
	path := w27cWriteSettings(t, settings, 0o600)
	require.NoError(t, SetHooks(path, w27cGwen, true))
	require.NoError(t, SetHooks(path, w27cGwen, false))
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.JSONEq(t, settings, string(got), "the command means the same")
	require.Equal(t, settings, string(got), "the person's own commands are kept as written, with no \\u003c-style escapes")
}

// Off takes out Gwen's entries only: one beside the person's in the same
// group, a group of Gwen's alone, and the event once nothing is left. A
// gwen at another path, or a hook that isn't a command, is not Gwen's.
func TestSetHooksOffRemovesOnlyGwensEntries(t *testing.T) {
	t.Parallel()
	settings := `{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup",
        "hooks": [
          {"type": "command", "command": "/usr/local/bin/audit start"},
          {"type": "command", "command": "` + w27cGwenHook + `", "async": true}
        ]
      },
      {
        "hooks": [
          {"type": "command", "command": "` + w27cGwenHook + `", "async": true}
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {"type": "command", "command": "/usr/bin/gwen hook", "async": true},
          {"type": "prompt", "command": "` + w27cGwenHook + `"}
        ]
      }
    ],
    "SessionEnd": [
      {
        "hooks": [
          {"type": "command", "command": "` + w27cGwenHook + `", "async": true}
        ]
      }
    ]
  },
  "theme": "dark"
}
`
	path := w27cWriteSettings(t, settings, 0o600)
	require.False(t, ReadHooks(path, w27cGwen).On, "three events are not all six")

	require.NoError(t, SetHooks(path, w27cGwen, false))
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.JSONEq(t, `{
  "hooks": {
    "SessionStart": [
      {"matcher": "startup", "hooks": [{"type": "command", "command": "/usr/local/bin/audit start"}]}
    ],
    "Stop": [
      {
        "hooks": [
          {"type": "command", "command": "/usr/bin/gwen hook", "async": true},
          {"type": "prompt", "command": "`+w27cGwenHook+`"}
        ]
      }
    ]
  },
  "theme": "dark"
}`, string(got))
	require.Equal(t, []string{"hooks", "theme"}, w27cKeys(t, got))

	// Once no hook is left at all, the hooks key goes too.
	path = w27cWriteSettings(t, `{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "`+w27cGwenHook+`"}]}]}, "model": "opus"}`, 0o600)
	require.NoError(t, SetHooks(path, w27cGwen, false))
	got, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "{\n  \"model\": \"opus\"\n}\n", string(got))
}

// A file Gwen can't read as Claude Code's settings is left exactly as it is,
// and the switch says why (plan.md 3.14.6).
func TestSetHooksLeavesInvalidJSONUntouched(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, content string }{
		{"cut short", "{\n  \"model\": \"opus\",\n"},
		{"comments", "{\n  // my model\n  \"model\": \"opus\"\n}\n"},
		{"trailing comma", `{"model": "opus",}`},
		{"two objects", `{} {}`},
		{"not an object", `["hooks"]`},
		{"hooks not an object", `{"hooks": []}`},
		{"an event not a list", `{"hooks": {"Stop": {"type": "command"}}}`},
		{"empty", ``},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			for _, on := range []bool{true, false} {
				path := w27cWriteSettings(t, tt.content, 0o600)
				require.Error(t, SetHooks(path, w27cGwen, on))
				got, err := os.ReadFile(path)
				require.NoError(t, err)
				require.Equal(t, tt.content, string(got))
				entries, err := os.ReadDir(filepath.Dir(path))
				require.NoError(t, err)
				require.Len(t, entries, 1, "no copy and no temporary file")
			}
		})
	}

	path := w27cWriteSettings(t, "{\n  \"model\": \"opus\",\n", 0o600)
	st := ReadHooks(path, w27cGwen)
	require.False(t, st.On)
	require.Equal(t, "Claude Code's settings file isn't valid JSON, so Gwen left it alone.", st.Note)
}

func TestSetHooksWithoutASettingsFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".claude", "settings.json")

	// Off with no file makes none.
	require.NoError(t, SetHooks(path, w27cGwen, false))
	_, err := os.Stat(filepath.Dir(path))
	require.ErrorIs(t, err, os.ErrNotExist)

	require.NoError(t, SetHooks(path, w27cGwen, true))
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, []string{"hooks"}, w27cKeys(t, got))
	require.True(t, ReadHooks(path, w27cGwen).On)
	st, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), st.Mode().Perm())
	_, err = os.Stat(path + ".gwen-backup")
	require.ErrorIs(t, err, os.ErrNotExist, "there was nothing to keep a copy of")
}

func TestReadHooksShowsTheFilesRealState(t *testing.T) {
	t.Parallel()
	require.Equal(t, HooksState{}, ReadHooks(filepath.Join(t.TempDir(), "settings.json"), w27cGwen), "no file: off, nothing to say")

	path := w27cWriteSettings(t, `{"disableAllHooks": true}`, 0o600)
	require.NoError(t, SetHooks(path, w27cGwen, true))
	require.Equal(t, HooksState{On: true, Note: "Claude Code hooks are turned off in your settings."}, ReadHooks(path, w27cGwen))

	// Gwen's hooks at another path are not this Gwen's.
	require.False(t, ReadHooks(path, "/opt/gwen/bin/gwen").On)

	// One event missing is off: the switch shows the file's real state.
	path = w27cWriteSettings(t, `{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "`+w27cGwenHook+`"}]}]}}`, 0o600)
	require.Equal(t, HooksState{}, ReadHooks(path, w27cGwen))
}
