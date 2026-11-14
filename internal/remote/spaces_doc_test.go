package remote

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/herdr"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// The phone's spaces come from herdr-core's "herdr-app" document, whose
// format belongs to Herdr (plan.md 3.14.7): the parser is tested against a
// recorded document and skips what it doesn't know. The fixture has rows,
// columns, a stack, a mod block, a floating pane, agent and tab blocks, and
// fields Gwen never reads at every level.

func w27cHerdrDoc(t *testing.T) json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "herdr-app", "document.json"))
	require.NoError(t, err)
	return data
}

func TestParseSpacesReadsARecordedDocument(t *testing.T) {
	t.Parallel()
	spaces := parseSpaces(w27cHerdrDoc(t))
	type tab struct{ ID, Name string }
	type space struct {
		ID, Name, Folder string
		Tabs             []tab
	}
	var got []space
	for _, sp := range spaces {
		s := space{ID: sp.ID, Name: sp.Name, Folder: sp.Folder}
		for _, tb := range sp.Tabs {
			s.Tabs = append(s.Tabs, tab{tb.ID, tb.Name})
		}
		got = append(got, s)
	}
	require.Equal(t, []space{
		{ID: "s1", Name: "gwen", Folder: "/home/user/code/gwen", Tabs: []tab{{"s1-tab1", "edit"}, {"s1-tab2", ""}}},
		{ID: "s2", Folder: "/home/user/notes", Tabs: []tab{{"s2-tab1", "agents"}}},
		{ID: "s3", Name: "scratch"},
	}, got)

	// A tab's terminals in layout order: children in their own order, then
	// what floats over them. A mod, an agent block and a tab block name no
	// terminal; a terminal shown twice counts once.
	require.Equal(t, []string{"t3", "t1", "t2", "t9"}, layoutTerminals(spaces[0].Tabs[0].Root))
	require.Equal(t, []string{"t4"}, layoutTerminals(spaces[0].Tabs[1].Root))
	require.Equal(t, []string{"t1"}, layoutTerminals(spaces[1].Tabs[0].Root))
}

// A document Gwen can't read gives no spaces rather than an error: the
// phone then lists every terminal as loose.
func TestParseSpacesSkipsWhatItCantRead(t *testing.T) {
	t.Parallel()
	for _, doc := range []string{``, `null`, `{}`, `{"spaces": null}`, `not json`, `{"spaces": {"s1": {}}}`, `[]`} {
		require.Empty(t, parseSpaces(json.RawMessage(doc)), doc)
	}
	for _, root := range []string{``, `null`, `"t1"`, `{"type": "block", "content": {"terminal": ""}}`, `{"terminal": 7}`} {
		require.Empty(t, layoutTerminals(json.RawMessage(root)), root)
	}
	// A layout node of a kind Herdr adds later still gives up its terminals.
	require.Equal(t, []string{"t5", "t6"}, layoutTerminals(json.RawMessage(
		`{"type": "carousel", "slides": [{"type": "block", "content": {"terminal": "t5"}}, {"terminal": "t6"}]}`)))
}

// Without a project that has a folder, the phone's Spaces screen is Gwen
// Terminal's own spaces from the document, with the terminals that are live;
// the rest of this machine's live terminals are loose.
func TestSpacesFromTheHerdrAppDocument(t *testing.T) {
	// Theme reads Gwen Terminal's settings: point it at an empty folder.
	t.Setenv("HERDR_APP_HOME", t.TempDir())
	clk := clock.NewFake(testutil.At("09:00:00"))
	db := testutil.NewDBWithClock(t, clk)
	svc, err := New(Options{DB: db, Repos: store.NewRepos(db), Clock: clk, Config: config.Defaults, Home: t.TempDir()})
	require.NoError(t, err)

	svc.mu.Lock()
	svc.spaces = parseSpaces(w27cHerdrDoc(t))
	svc.terms = map[string]herdr.Terminal{
		"t1":    {Address: "t1", State: "live", Name: "editor", Cwd: "/home/user/code/gwen", Cols: 120, Rows: 40},
		"t2":    {Address: "t2", State: "live", Title: "tail -f gwend.log"},
		"t3":    {Address: "t3", State: "live", Foreground: &herdr.Foreground{Name: "claude", Cwd: "/home/user/code/gwen/ui"}, Status: &herdr.Status{State: "working", App: "claude"}},
		"t4":    {Address: "t4", State: "finished"},
		"t9":    {Address: "t9", State: "live", Argv: []string{"/usr/bin/htop"}},
		"t5":    {Address: "t5", State: "live", Name: "no tab"},
		"t6":    {Address: "t6", State: "live", Parent: "t3"},
		"t8":    {Address: "t8", State: "finished"},
		"pi/t7": {Address: "pi/t7", State: "live"},
	}
	svc.byTerminal["t3"] = &session{CodeSession: store.CodeSession{ID: "cs_session003"}}
	svc.mu.Unlock()

	got := svc.Spaces(t.Context())
	type tab struct {
		ID, Name  string
		Terminals []string
	}
	type space struct {
		ID, Name, Folder string
		Tabs             []tab
	}
	var spaces []space
	for _, sp := range got.Spaces {
		s := space{ID: sp.ID, Name: sp.Name, Folder: sp.Folder, Tabs: []tab{}}
		for _, tb := range sp.Tabs {
			ts := []string{}
			for _, term := range tb.Terminals {
				ts = append(ts, term.Terminal)
			}
			s.Tabs = append(s.Tabs, tab{tb.ID, tb.Name, ts})
		}
		spaces = append(spaces, s)
	}
	require.Equal(t, []space{
		{ID: "s1", Name: "gwen", Folder: "/home/user/code/gwen", Tabs: []tab{
			{"s1-tab1", "edit", []string{"t3", "t1", "t2", "t9"}},
			// A tab without a name is called by its place; a finished
			// terminal is not shown.
			{"s1-tab2", "2", []string{}},
		}},
		// A space without a name is called by its folder.
		{ID: "s2", Name: "notes", Folder: "/home/user/notes", Tabs: []tab{{"s2-tab1", "agents", []string{"t1"}}}},
		{ID: "s3", Name: "scratch", Tabs: []tab{}},
	}, spaces)

	// Loose: live, on this machine, and not a helper of another terminal.
	require.Equal(t, []wire.CodeTerminal{{Terminal: "t5", Name: "no tab"}}, got.Loose)

	// Each terminal carries its name, folder, status and session.
	edit := got.Spaces[0].Tabs[0].Terminals
	require.Equal(t, wire.CodeTerminal{Terminal: "t3", Name: "claude", Cwd: "/home/user/code/gwen/ui", Status: "working", App: "claude",
		SessionID: "cs_session003"}, edit[0])
	require.Equal(t, wire.CodeTerminal{Terminal: "t1", Name: "editor", Cwd: "/home/user/code/gwen", Cols: 120, Rows: 40}, edit[1])
	require.Equal(t, "tail -f gwend.log", edit[2].Name)
	require.Equal(t, "htop", edit[3].Name)
}
