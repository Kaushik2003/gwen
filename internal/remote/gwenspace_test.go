package remote

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveScreen(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, tab, sub string
		ok             bool
	}{
		{"", "today", "", true},
		{"today", "today", "", true},
		{"assistant", "gwen", "", true},
		{"inbox", "tasks", "inbox", true},
		{"board", "tasks", "board", true},
		{"projects", "tasks", "projects", true},
		{"goals", "plan", "goals", true},
		{"weekly", "review", "weekly", true},
		{"stats", "review", "stats", true},
		{"history", "review", "history", true},
		{"gwen", "gwen", "", true},
		{"tasks", "tasks", "", true},
		{"plan", "plan", "", true},
		{"review", "review", "", true},
		{"connections", "connections", "", true},
		{"memory", "memory", "", true},
		{"settings", "settings", "", true},
		{"unknown", "", "", false},
	} {
		name := tc.name
		if name == "" {
			name = "default"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tab, sub, ok := resolveScreen(tc.name)
			require.Equal(t, tc.ok, ok)
			require.Equal(t, tc.tab, tab)
			require.Equal(t, tc.sub, sub)
		})
	}
}

func TestModScreen(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, root, want string
	}{
		{"gwen pane", `{"type":"row","children":[{"content":{"mod":"gwen","props":{"screen":"today"}}}]}`, "today"},
		{"nested gwen pane", `{"type":"row","children":[{"type":"column","children":[{"content":{"mod":"gwen","props":{"screen":"settings"}}}]}]}`, "settings"},
		{"terminal only", `{"type":"row","children":[{"content":{"terminal":"term-1"}}]}`, ""},
		{"other mod", `{"type":"row","children":[{"content":{"mod":"files","props":{"screen":"today"}}}]}`, ""},
		{"missing props", `{"type":"row","children":[{"content":{"mod":"gwen"}}]}`, ""},
		{"empty screen", `{"type":"row","children":[{"content":{"mod":"gwen","props":{"screen":""}}}]}`, ""},
		{"invalid screen type", `{"type":"row","children":[{"content":{"mod":"gwen","props":{"screen":42}}}]}`, ""},
		{"invalid json", `{"type":"row",`, ""},
		{"empty root", ``, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, modScreen(json.RawMessage(tc.root)))
		})
	}
}

func TestGwenSpace(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, document, space string
		tabs                  map[string]string
	}{
		{
			name: "first space with gwen tabs and first duplicate",
			document: `{"spaces":[
				{"id":"shell-space","name":"Gwen","tabs":[
					{"id":"shell","root":{"type":"row","children":[{"content":{"terminal":"term-1"}}]}}
				]},
				{"id":"chosen-space","name":"Renamed","tabs":[
					{"id":"files","root":{"type":"row","children":[{"content":{"mod":"files","props":{"screen":"today"}}}]}},
					{"id":"today-first","root":{"type":"row","children":[{"content":{"mod":"gwen","props":{"screen":"today"}}}]}},
					{"id":"today-duplicate","root":{"type":"row","children":[{"content":{"mod":"gwen","props":{"screen":"today"}}}]}},
					{"id":"settings","root":{"type":"row","children":[{"content":{"mod":"gwen","props":{"screen":"settings"}}}]}}
				]},
				{"id":"later-space","tabs":[
					{"id":"later-today","root":{"type":"row","children":[{"content":{"mod":"gwen","props":{"screen":"today"}}}]}},
					{"id":"later-memory","root":{"type":"row","children":[{"content":{"mod":"gwen","props":{"screen":"memory"}}}]}}
				]}
			]}`,
			space: "chosen-space",
			tabs:  map[string]string{"today": "today-first", "settings": "settings"},
		},
		{
			name: "no gwen tabs",
			document: `{"spaces":[
				{"id":"empty","tabs":[]},
				{"id":"shell-space","name":"Gwen","tabs":[
					{"id":"shell","root":{"type":"row","children":[{"content":{"terminal":"term-1"}}]}}
				]},
				{"id":"files-space","tabs":[
					{"id":"files","root":{"type":"row","children":[{"content":{"mod":"files","props":{"screen":"today"}}}]}}
				]}
			]}`,
		},
		{name: "empty spaces", document: `{"spaces":[]}`},
		{name: "missing spaces", document: `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var doc struct {
				Spaces []docSpace `json:"spaces"`
			}
			require.NoError(t, json.Unmarshal([]byte(tc.document), &doc))
			space, tabs := gwenSpace(doc.Spaces)
			require.Equal(t, tc.space, space)
			require.Equal(t, tc.tabs, tabs)
		})
	}
}

func TestInStep(t *testing.T) {
	t.Parallel()
	doc := func(spaces ...docSpace) []docSpace { return spaces }
	space := func(id string, tabs ...string) docSpace {
		sp := docSpace{ID: id}
		for _, tab := range tabs {
			sp.Tabs = append(sp.Tabs, docTab{ID: tab})
		}
		return sp
	}
	for _, tc := range []struct {
		name string
		doc  []docSpace
		live map[string][]string
		want bool
	}{
		{"same spaces and tabs", doc(space("gwen", "today", "tasks"), space("code", "shell")),
			map[string][]string{"gwen": {"tasks", "today"}, "code": {"shell"}}, true},
		{"nothing on either side", nil, map[string][]string{}, true},
		// A core that has just started holds nothing until the window saves.
		{"core holds nothing yet", nil, map[string][]string{"gwen": {"today"}}, false},
		{"a space made just now is not saved yet", doc(space("code", "shell")),
			map[string][]string{"gwen": {"today"}, "code": {"shell"}}, false},
		{"a tab made just now is not saved yet", doc(space("gwen", "today")),
			map[string][]string{"gwen": {"today", "tasks"}}, false},
		{"a closed space is still saved", doc(space("gwen", "today"), space("code", "shell")),
			map[string][]string{"code": {"shell"}}, false},
		{"a tab was swapped for another", doc(space("gwen", "today", "old")),
			map[string][]string{"gwen": {"today", "new"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, inStep(tc.doc, tc.live))
		})
	}
}
