package herdr_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kzark/gwen/internal/herdr"
	"github.com/kzark/gwen/internal/herdr/herdrtest"
	"github.com/stretchr/testify/require"
)

// Subscribe returns the terminals as they are, then every change after
// them in order; heartbeats, junk and events the snapshot already covers
// never reach the caller, and folding the rest with Apply keeps the picture
// right.
func TestEvents(t *testing.T) {
	t.Parallel()
	core := herdrtest.New(t, herdrtest.Options{})
	t1 := core.AddTerminal(herdr.Terminal{Name: "claude"})
	c := core.Connect()
	ctx := testCtx(t)

	events, snap, err := c.Subscribe(ctx)
	require.NoError(t, err)
	terms := map[string]herdr.Terminal{}
	for _, term := range snap {
		terms[term.Address] = term
	}
	require.Contains(t, terms, t1.Address)

	core.Heartbeat()
	core.EmitLine(`not json at all`)
	core.EmitLine(`{"jsonrpc":"2.0","method":"event","params":{"seq":1,"type":"title","terminal":"t1","data":{"title":"stale"}}}`)
	core.SetStatus(t1.Address, &herdr.Status{State: "working", App: "claude-code"})
	core.SetDoc("herdr-app", map[string]any{"spaces": []any{}})
	t2 := core.AddTerminal(herdr.Terminal{Name: "shell"})
	zero := 0
	core.Finish(t1.Address, herdr.CloseReason{Reason: "exited", ExitCode: &zero})
	core.Remove(t2.Address)

	want := []struct {
		typ      string
		terminal string
	}{
		{"status", t1.Address}, {"doc", ""}, {"created", t2.Address}, {"status", t1.Address}, {"finished", t1.Address},
		{"removed", t2.Address},
	}
	for i, w := range want {
		ev, err := events.Next(ctx)
		require.NoError(t, err)
		require.EqualValues(t, i+2, ev.Seq)
		require.Equal(t, w.typ, ev.Type)
		require.Equal(t, w.terminal, ev.Terminal)
		require.Equal(t, w.terminal != "", herdr.Apply(terms, ev), "Apply reports whether the event named a terminal")
		switch i {
		case 0:
			require.Equal(t, "working", terms[t1.Address].Status.State)
			require.True(t, terms[t1.Address].Reported)
		case 1:
			var d struct {
				Name string `json:"name"`
			}
			require.NoError(t, json.Unmarshal(ev.Data, &d))
			require.Equal(t, "herdr-app", d.Name)
		case 2:
			require.Equal(t, "shell", terms[t2.Address].Name)
		case 3:
			require.Nil(t, terms[t1.Address].Status, "the status is cleared just before finished")
		}
	}
	require.Equal(t, "finished", terms[t1.Address].State)
	require.False(t, terms[t1.Address].Live())
	require.NotContains(t, terms, t2.Address)
}

// `missed` replaces everything known with a fresh snapshot, and events it
// already covers are dropped after it.
func TestMissed(t *testing.T) {
	t.Parallel()
	core := herdrtest.New(t, herdrtest.Options{})
	t1 := core.AddTerminal(herdr.Terminal{})
	c := core.Connect()
	ctx := testCtx(t)
	events, _, err := c.Subscribe(ctx)
	require.NoError(t, err)

	core.SetTitle(t1.Address, "a") // seq 2
	core.Missed()
	core.EmitLine(`{"jsonrpc":"2.0","method":"event","params":{"seq":2,"type":"title","terminal":"t1","data":{"title":"again"}}}`)
	core.SetTitle(t1.Address, "b") // seq 3

	ev, err := events.Next(ctx)
	require.NoError(t, err)
	require.Equal(t, "title", ev.Type)
	ev, err = events.Next(ctx)
	require.NoError(t, err)
	require.Equal(t, "missed", ev.Type)
	require.EqualValues(t, 2, ev.Seq)
	require.Len(t, ev.Missed, 1)
	require.Equal(t, "a", ev.Missed[0].Title)
	ev, err = events.Next(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 3, ev.Seq, "the stale seq 2 was dropped")

	// A snapshot with no terminals still replaces everything: Missed is
	// empty, not nil, so a caller can tell it from an ordinary event.
	core.EmitLine(`{"jsonrpc":"2.0","method":"missed","params":{"seq":9,"terminals":null}}`)
	core.EmitLine(`{"jsonrpc":"2.0","method":"event","params":{"seq":9,"type":"title","terminal":"t1","data":{"title":"old"}}}`)
	core.EmitLine(`{"jsonrpc":"2.0","method":"event","params":{"seq":10,"type":"title","terminal":"t1","data":{"title":"new"}}}`)
	ev, err = events.Next(ctx)
	require.NoError(t, err)
	require.Equal(t, "missed", ev.Type)
	require.NotNil(t, ev.Missed)
	require.Empty(t, ev.Missed)
	ev, err = events.Next(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 10, ev.Seq)
}

// A subscriber that stops reading falls behind; herdr-core then drops its
// backlog and sends `missed` instead, keeping whole lines. The client must
// come out of it with every event it does get in order and the snapshot in
// between.
func TestMissedWhenFallingBehind(t *testing.T) {
	t.Parallel()
	core := herdrtest.New(t, herdrtest.Options{MaxPending: 64 << 10})
	t1 := core.AddTerminal(herdr.Terminal{})
	c := core.Connect()
	ctx := testCtx(t)
	events, _, err := c.Subscribe(ctx)
	require.NoError(t, err)

	// Nothing reads while 100 events of 16 KiB arrive: more than the
	// stream's credit (256 KiB plus the client's 1 MiB) and the backlog
	// allowed after it together.
	big := strings.Repeat("x", 16<<10)
	const sent = 100
	for range sent {
		core.SetTitle(t1.Address, big)
	}
	core.SetTurn(t1.Address, 99)

	var last uint64
	titles, missed := 0, 0
	for {
		ev, err := events.Next(ctx)
		require.NoError(t, err)
		require.Greater(t, ev.Seq, last, "events come in order")
		last = ev.Seq
		if ev.Type == "missed" {
			missed++
			require.Len(t, ev.Missed, 1)
			require.Equal(t, t1.Address, ev.Missed[0].Address)
			continue
		}
		if ev.Type == "turn" {
			break
		}
		require.Equal(t, "title", ev.Type)
		titles++
	}
	require.Positive(t, missed)
	require.Less(t, titles, sent, "the backlog was dropped")
	require.EqualValues(t, sent+2, last, "the last event came through")
}

// The client gives the event stream 1 MiB of credit when it subscribes and
// returns credit for every byte it reads, so the core keeps sending.
func TestEventCredit(t *testing.T) {
	t.Parallel()
	core := herdrtest.New(t, herdrtest.Options{})
	t1 := core.AddTerminal(herdr.Terminal{})
	c := core.Connect()
	ctx := testCtx(t)
	events, _, err := c.Subscribe(ctx)
	require.NoError(t, err)
	sub := core.WaitSubscribers(1)[0]
	sub.WaitCredited(1 << 20)

	for _, title := range []string{"a", "b", "c"} {
		core.SetTitle(t1.Address, title)
	}
	for range 3 {
		_, err := events.Next(ctx)
		require.NoError(t, err)
	}
	read := sub.Sent()
	sub.WaitCredited(1<<20 + read)
	require.Equal(t, 1<<20+read, sub.Credited())
}

// Apply folds each event type the way herdr-core's schema/README.md says.
func TestApply(t *testing.T) {
	t.Parallel()
	base := func() map[string]herdr.Terminal {
		return map[string]herdr.Terminal{"t1": {Address: "t1", State: "live", PID: 5, Labels: map[string]string{"a": "1"},
			Foreground: &herdr.Foreground{PID: 6, Name: "claude", Argv: []string{"claude"}}, Title: "old", Cols: 80, Rows: 24}}
	}
	t2 := `{"address":"t2","state":"live","pid":9,"name":"two"}`
	cases := []struct {
		name  string
		ev    herdr.Event
		named bool
		check func(t *testing.T, terms map[string]herdr.Terminal)
	}{
		{"created adds the record", herdr.Event{Type: "created", Terminal: "t2", Data: json.RawMessage(t2)}, true,
			func(t *testing.T, terms map[string]herdr.Terminal) { require.Equal(t, "two", terms["t2"].Name) }},
		{"created without a record adds nothing", herdr.Event{Type: "created", Terminal: "t2", Data: json.RawMessage(`{}`)}, true,
			func(t *testing.T, terms map[string]herdr.Terminal) { require.NotContains(t, terms, "t2") }},
		{"restarted replaces the record", herdr.Event{Type: "restarted", Terminal: "t1",
			Data: json.RawMessage(`{"address":"t1","state":"live","pid":7}`)}, true,
			func(t *testing.T, terms map[string]herdr.Terminal) {
				require.Equal(t, 7, terms["t1"].PID)
				require.Nil(t, terms["t1"].Foreground)
			}},
		{"removed drops it", herdr.Event{Type: "removed", Terminal: "t1", Data: json.RawMessage(`{}`)}, true,
			func(t *testing.T, terms map[string]herdr.Terminal) { require.NotContains(t, terms, "t1") }},
		{"labels replaces all labels", herdr.Event{Type: "labels", Terminal: "t1", Data: json.RawMessage(`{"labels":{"b":"2"}}`)}, true,
			func(t *testing.T, terms map[string]herdr.Terminal) {
				require.Equal(t, map[string]string{"b": "2"}, terms["t1"].Labels)
			}},
		{"labels with a record adds a terminal new to the view", herdr.Event{Type: "labels", Terminal: "t2",
			Data: json.RawMessage(`{"labels":{"b":"2"},"record":` + t2 + `}`)}, true,
			func(t *testing.T, terms map[string]herdr.Terminal) { require.Equal(t, 9, terms["t2"].PID) }},
		{"labels of an unknown terminal add nothing", herdr.Event{Type: "labels", Terminal: "t2", Data: json.RawMessage(`{"labels":{}}`)}, true,
			func(t *testing.T, terms map[string]herdr.Terminal) { require.NotContains(t, terms, "t2") }},
		{"status replaces status, records and reported", herdr.Event{Type: "status", Terminal: "t1",
			Data: json.RawMessage(`{"status":{"state":"blocked","app":"claude-code"},"records":{"x":{"state":"idle"}},"reported":true}`)}, true,
			func(t *testing.T, terms map[string]herdr.Terminal) {
				require.Equal(t, "blocked", terms["t1"].Status.State)
				require.Equal(t, "idle", terms["t1"].Records["x"].State)
				require.True(t, terms["t1"].Reported)
			}},
		{"a null status clears it", herdr.Event{Type: "status", Terminal: "t1", Data: json.RawMessage(`{"status":null,"records":{},"reported":true}`)}, true,
			func(t *testing.T, terms map[string]herdr.Terminal) { require.Nil(t, terms["t1"].Status) }},
		{"foreground sets the program", herdr.Event{Type: "foreground", Terminal: "t1",
			Data: json.RawMessage(`{"pid":8,"name":"vim","argv":["vim","x"],"cwd":"/src"}`)}, true,
			func(t *testing.T, terms map[string]herdr.Terminal) {
				require.Equal(t, []string{"vim", "x"}, terms["t1"].Foreground.Argv)
			}},
		{"a null foreground clears it", herdr.Event{Type: "foreground", Terminal: "t1", Data: json.RawMessage(`null`)}, true,
			func(t *testing.T, terms map[string]herdr.Terminal) { require.Nil(t, terms["t1"].Foreground) }},
		{"finished ends the program", herdr.Event{Type: "finished", Terminal: "t1", Data: json.RawMessage(`{"reason":"exited","exit_code":0}`)}, true,
			func(t *testing.T, terms map[string]herdr.Terminal) {
				require.Equal(t, "finished", terms["t1"].State)
				require.Zero(t, terms["t1"].PID)
				require.Nil(t, terms["t1"].Foreground)
			}},
		{"lost keeps a kept terminal live without a program", herdr.Event{Type: "lost", Terminal: "t1", Data: json.RawMessage(`{"reason":"signaled"}`)}, true,
			func(t *testing.T, terms map[string]herdr.Terminal) {
				require.Equal(t, "live", terms["t1"].State)
				require.Zero(t, terms["t1"].PID)
				require.False(t, terms["t1"].Live())
			}},
		{"title", herdr.Event{Type: "title", Terminal: "t1", Data: json.RawMessage(`{"title":"new"}`)}, true,
			func(t *testing.T, terms map[string]herdr.Terminal) { require.Equal(t, "new", terms["t1"].Title) }},
		{"size", herdr.Event{Type: "size", Terminal: "t1", Data: json.RawMessage(`{"cols":120,"rows":40}`)}, true,
			func(t *testing.T, terms map[string]herdr.Terminal) {
				require.Equal(t, 120, terms["t1"].Cols)
				require.Equal(t, 40, terms["t1"].Rows)
			}},
		{"turn", herdr.Event{Type: "turn", Terminal: "t1", Data: json.RawMessage(`{"turn":4,"state":"done"}`)}, true,
			func(t *testing.T, terms map[string]herdr.Terminal) { require.EqualValues(t, 4, terms["t1"].Turn) }},
		{"keep", herdr.Event{Type: "keep", Terminal: "t1", Data: json.RawMessage(`{"keep":true}`)}, true,
			func(t *testing.T, terms map[string]herdr.Terminal) { require.True(t, terms["t1"].Keep) }},
		{"a change to an unknown terminal adds nothing", herdr.Event{Type: "title", Terminal: "t9", Data: json.RawMessage(`{"title":"x"}`)}, true,
			func(t *testing.T, terms map[string]herdr.Terminal) { require.NotContains(t, terms, "t9") }},
		{"a doc event names no terminal", herdr.Event{Type: "doc", Data: json.RawMessage(`{"name":"herdr-app","version":2}`)}, false,
			func(t *testing.T, terms map[string]herdr.Terminal) { require.Equal(t, base(), terms) }},
		{"an unknown type changes nothing", herdr.Event{Type: "colour", Terminal: "t1", Data: json.RawMessage(`{}`)}, true,
			func(t *testing.T, terms map[string]herdr.Terminal) { require.Equal(t, base(), terms) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			terms := base()
			require.Equal(t, tc.named, herdr.Apply(terms, tc.ev))
			tc.check(t, terms)
		})
	}
}

// Live means a program runs in it now; Local means it is on this laptop.
func TestTerminalLiveLocal(t *testing.T) {
	t.Parallel()
	require.True(t, herdr.Terminal{State: "live", PID: 1}.Live())
	require.False(t, herdr.Terminal{State: "live"}.Live(), "lost, waiting for its new shell")
	require.False(t, herdr.Terminal{State: "finished", PID: 1}.Live())
	require.True(t, herdr.Terminal{Address: "t7"}.Local())
	require.False(t, herdr.Terminal{Address: "rohan/t7"}.Local())
}
