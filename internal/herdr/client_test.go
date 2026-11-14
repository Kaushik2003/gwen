package herdr_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/herdr"
	"github.com/kzark/gwen/internal/herdr/herdrtest"
	"github.com/kzark/gwen/term"
	"github.com/stretchr/testify/require"
)

// testCtx bounds a test's calls; the deadline is only a guard against a hang.
func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// waitDone fails the test unless ch closes; the timer is only a guard.
func waitDone(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

// older is a version just below v: its last non-zero part one less.
func older(v string) string {
	parts := strings.Split(v, ".")
	for i := len(parts) - 1; i >= 0; i-- {
		if n, err := strconv.Atoi(parts[i]); err == nil && n > 0 {
			parts[i] = strconv.Itoa(n - 1)
			return strings.Join(parts, ".")
		}
	}
	panic("no version below " + v)
}

// newer is v with its patch one more.
func newer(v string) string {
	parts := strings.Split(v, ".")
	n, _ := strconv.Atoi(parts[len(parts)-1])
	parts[len(parts)-1] = strconv.Itoa(n + 1)
	return strings.Join(parts, ".")
}

// Dial asks for framed mode and resize notifications, and hands back what
// the core said about itself exactly as it said it.
func TestDialReportsHello(t *testing.T) {
	t.Parallel()
	core := herdrtest.New(t, herdrtest.Options{Version: "0.9.1", Features: []string{"resize_notifications", "automatic_input"}})
	c := core.Connect()

	h := c.Hello()
	require.Equal(t, herdr.Generation, h.Generation)
	require.Equal(t, "0.9.1", h.Version)
	require.Equal(t, []string{"resize_notifications", "automatic_input"}, h.Features)
	require.Contains(t, h.Methods, "terminal.attach")

	var p struct {
		Framed   bool     `json:"framed"`
		Features []string `json:"features"`
	}
	require.NoError(t, core.WaitCalls("hello", 1)[0].Decode(&p))
	require.True(t, p.Framed, "attach needs framed mode")
	require.Contains(t, p.Features, "resize_notifications")

	// The connection is framed now: calls and streams work.
	_, err := c.List(testCtx(t))
	require.NoError(t, err)
}

// gwend and the Gwen window accept a core only when hello reports protocol
// generation 1 and a version at least term/core.version (plan.md 3.14.5);
// otherwise Settings › Phone says to run make term. Dial does not refuse by
// itself: remote.Service.follow and the window's terminals.client make this
// check on Hello(), so this pins the contract they rely on.
func TestHelloCheck(t *testing.T) {
	t.Parallel()
	want := term.CoreVersion()
	cases := []struct {
		name       string
		generation int
		version    string
		ok         bool
	}{
		{"the version Gwen needs", 1, want, true},
		{"a newer patch", 1, newer(want), true},
		{"with a leading v", 1, "v" + want, true},
		{"one release too old", 1, older(want), false},
		{"another generation", 2, want, false},
		{"another generation, newer", 2, newer(want), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			core := herdrtest.New(t, herdrtest.Options{Generation: tc.generation, Version: tc.version})
			h := core.Connect().Hello()
			require.Equal(t, tc.generation, h.Generation)
			require.Equal(t, tc.version, h.Version)
			ok := h.Generation == herdr.Generation && herdr.VersionAtLeast(h.Version, want)
			require.Equal(t, tc.ok, ok)
		})
	}
}

// A core that refuses hello, one that isn't there, and one that never
// answers all make Dial fail, and none leaves a connection open.
func TestDialFails(t *testing.T) {
	t.Parallel()
	t.Run("refused", func(t *testing.T) {
		t.Parallel()
		core := herdrtest.New(t, herdrtest.Options{})
		core.Handle("hello", func(herdrtest.Call) (any, error) {
			return nil, herdrtest.Err("unsupported_generation", "protocol generation 1 not supported")
		})
		_, err := herdr.Dial(testCtx(t), core.Socket())
		require.True(t, herdr.IsKind(err, "unsupported_generation"), "%v", err)
		core.WaitConnections(0)
	})
	t.Run("no core", func(t *testing.T) {
		t.Parallel()
		core := herdrtest.New(t, herdrtest.Options{})
		core.Stop()
		_, err := herdr.Dial(testCtx(t), core.Socket())
		require.ErrorContains(t, err, "herdr-core: connect")
		// The core comes back: dialing works again.
		core.Start()
		c, err := herdr.Dial(testCtx(t), core.Socket())
		require.NoError(t, err)
		c.Close()
	})
	t.Run("never answers", func(t *testing.T) {
		t.Parallel()
		core := herdrtest.New(t, herdrtest.Options{})
		release := make(chan struct{})
		var once sync.Once
		free := func() { once.Do(func() { close(release) }) }
		t.Cleanup(free)
		core.Handle("hello", func(herdrtest.Call) (any, error) {
			<-release
			return nil, errors.New("too late")
		})
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		_, err := herdr.Dial(ctx, core.Socket())
		require.ErrorIs(t, err, os.ErrDeadlineExceeded)
		// The client hung up: once the core gets round to it, the
		// connection ends.
		free()
		core.WaitConnections(0)
	})
}

// Core errors keep their kind, so callers can tell not_found from blocked;
// a reply that isn't the expected shape and a call past its deadline fail
// without breaking the connection.
func TestCallErrors(t *testing.T) {
	t.Parallel()
	core := herdrtest.New(t, herdrtest.Options{})
	c := core.Connect()
	ctx := testCtx(t)

	_, err := c.Get(ctx, "t9")
	require.True(t, herdr.IsKind(err, "not_found"), "%v", err)
	require.EqualError(t, err, "herdr-core: no terminal t9")
	require.False(t, herdr.IsKind(err, "blocked"))
	require.False(t, herdr.IsKind(errors.New("plain"), "not_found"))

	err = c.Call(ctx, "no.such", nil, nil)
	var e *herdr.Error
	require.ErrorAs(t, err, &e)
	require.Equal(t, -32601, e.Code)
	require.Equal(t, "unknown_method", e.Data.Kind)

	core.Handle("terminal.list", func(herdrtest.Call) (any, error) { return nil, errors.New("boom") })
	_, err = c.List(ctx)
	require.True(t, herdr.IsKind(err, "internal"), "%v", err)

	core.Handle("terminal.get", func(herdrtest.Call) (any, error) { return json.RawMessage(`"not a record"`), nil })
	_, err = c.Get(ctx, "t1")
	require.ErrorContains(t, err, "terminal.get: bad reply")

	// A call past its deadline gives up; the late reply is dropped.
	release := make(chan struct{})
	core.Handle("terminal.read", func(herdrtest.Call) (any, error) {
		<-release
		return map[string]any{"text": "late"}, nil
	})
	short, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	_, err = c.Read(short, "t1", 10)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	close(release)
	core.Handle("terminal.read", nil)
	t1 := core.AddTerminal(herdr.Terminal{})
	core.SetScreen(t1.Address, "on time")
	text, err := c.Read(ctx, t1.Address, 10)
	require.NoError(t, err)
	require.Equal(t, "on time", text)
}

// Calls run side by side: a send waiting on the person typing never holds
// up the rest of gwend's calls on the same connection.
func TestCallsRunSideBySide(t *testing.T) {
	t.Parallel()
	core := herdrtest.New(t, herdrtest.Options{})
	t1 := core.AddTerminal(herdr.Terminal{})
	c := core.Connect()
	ctx := testCtx(t)

	release := make(chan struct{})
	core.Handle("terminal.send", func(call herdrtest.Call) (any, error) {
		<-release
		return call.Builtin()
	})
	sent := make(chan error, 1)
	go func() {
		_, err := c.Send(ctx, t1.Address, "hello", false)
		sent <- err
	}()
	core.WaitCalls("terminal.send", 1)

	terms, err := c.List(ctx)
	require.NoError(t, err)
	require.Len(t, terms, 1)

	close(release)
	select {
	case err := <-sent:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("the send never finished")
	}
}

// When the connection goes, everything waiting on it ends at once: calls
// fail with ErrClosed, streams end as lost, the event stream reports EOF.
func TestConnectionLost(t *testing.T) {
	t.Parallel()
	core := herdrtest.New(t, herdrtest.Options{})
	t1 := core.AddTerminal(herdr.Terminal{})
	c := core.Connect()
	ctx := testCtx(t)

	events, _, err := c.Subscribe(ctx)
	require.NoError(t, err)
	view, err := c.Attach(ctx, herdr.AttachParams{Terminal: t1.Address, Observe: true, Cols: 80, Rows: 24})
	require.NoError(t, err)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	core.Handle("terminal.keys", func(herdrtest.Call) (any, error) {
		<-release
		return nil, nil
	})
	waiting := make(chan error, 1)
	go func() { waiting <- c.Keys(ctx, t1.Address, []string{"enter"}) }()
	core.WaitCalls("terminal.keys", 1)

	core.DropConnections()

	select {
	case err := <-waiting:
		require.ErrorIs(t, err, herdr.ErrClosed)
	case <-ctx.Done():
		t.Fatal("the waiting call never failed")
	}
	waitDone(t, c.Done(), "Done")
	f := lastFrame(t, view.Stream)
	require.Equal(t, "lost", f.Closed.Reason)
	_, err = events.Next(ctx)
	require.ErrorIs(t, err, io.EOF)
	_, err = c.List(ctx)
	require.ErrorIs(t, err, herdr.ErrClosed)
}

// After a reconnect the client takes a fresh snapshot (plan.md 3.14.5). A
// restarted core numbers its events from the start again, and the new
// subscription must take them even though the old one had seen higher
// numbers.
func TestReconnectFreshSnapshot(t *testing.T) {
	t.Parallel()
	core := herdrtest.New(t, herdrtest.Options{})
	ctx := testCtx(t)
	t1 := core.AddTerminal(herdr.Terminal{Name: "one"})

	c1 := core.Connect()
	ev1, snap, err := c1.Subscribe(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{t1.Address}, addresses(snap))
	t2 := core.AddTerminal(herdr.Terminal{Name: "two"})
	for i := range 3 {
		core.SetTitle(t1.Address, "title "+strconv.Itoa(i))
	}
	var last herdr.Event
	for range 4 {
		last, err = ev1.Next(ctx)
		require.NoError(t, err)
	}
	require.EqualValues(t, 5, last.Seq)

	core.Restart()
	waitDone(t, c1.Done(), "the old connection to end")
	_, err = ev1.Next(ctx)
	require.ErrorIs(t, err, io.EOF)

	// What changed while nobody was connected is in the new snapshot.
	t3 := core.AddTerminal(herdr.Terminal{Name: "three"})
	core.SetTitle(t2.Address, "while away")
	c2 := core.Connect()
	ev2, snap, err := c2.Subscribe(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{t1.Address, t2.Address, t3.Address}, addresses(snap))
	require.Equal(t, "while away", snap[1].Title)

	core.SetTitle(t1.Address, "after")
	ev, err := ev2.Next(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 3, ev.Seq, "numbered by the new core")
	require.Equal(t, "title", ev.Type)
	require.Equal(t, t1.Address, ev.Terminal)
}

func addresses(terms []herdr.Terminal) []string {
	var out []string
	for _, t := range terms {
		out = append(out, t.Address)
	}
	return out
}

// Each method sends the params herdr-core's schema names and reads its
// result: a renamed field here would silently do nothing in the real core.
func TestMethods(t *testing.T) {
	t.Parallel()
	core := herdrtest.New(t, herdrtest.Options{})
	c := core.Connect()
	ctx := testCtx(t)

	params := func(method string) string {
		calls := core.Calls(method)
		require.NotEmpty(t, calls, method)
		return string(calls[len(calls)-1].Params)
	}

	rec, err := c.Run(ctx, herdr.RunParams{Argv: []string{"claude", "--resume", "x"}, Cwd: "/src", Env: map[string]string{"A": "1"},
		Labels: map[string]string{"gwen.session": "cs_1"}, Name: "c1", Keep: true, Size: &herdr.Size{Cols: 100, Rows: 30}, NoParent: true})
	require.NoError(t, err)
	require.JSONEq(t, `{"argv":["claude","--resume","x"],"cwd":"/src","env":{"A":"1"},"labels":{"gwen.session":"cs_1"},
		"name":"c1","keep":true,"size":{"cols":100,"rows":30},"no_parent":true}`, params("terminal.run"))
	require.Equal(t, "t1", rec.Address)
	require.True(t, rec.Live())
	require.Equal(t, 100, rec.Cols)
	addr := rec.Address

	_, err = c.Run(ctx, herdr.RunParams{Argv: []string{"sh"}})
	require.NoError(t, err)
	require.JSONEq(t, `{"argv":["sh"]}`, params("terminal.run"), "empty options are left out")
	core.Finish("t2", herdr.CloseReason{Reason: "exited"})

	terms, err := c.List(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{addr}, addresses(terms), "finished terminals are not listed")
	require.JSONEq(t, `{}`, params("terminal.list"))

	got, err := c.Get(ctx, addr)
	require.NoError(t, err)
	require.Equal(t, "c1", got.Name)
	require.JSONEq(t, `{"terminal":"t1"}`, params("terminal.get"))

	require.NoError(t, c.Label(ctx, addr, map[string]string{"a": "1", "gwen.session": ""}))
	require.JSONEq(t, `{"terminal":"t1","labels":{"a":"1","gwen.session":""}}`, params("terminal.label"))
	got, _ = core.Terminal(addr)
	require.Equal(t, map[string]string{"a": "1"}, got.Labels, "an empty value removes the label")

	core.SetStatus(addr, &herdr.Status{State: "idle", App: "claude-code"})
	core.SetTurn(addr, 3)
	res, err := c.Send(ctx, addr, "fix the test", false)
	require.NoError(t, err)
	require.JSONEq(t, `{"terminal":"t1","text":"fix the test","force":false}`, params("terminal.send"))
	require.Equal(t, "idle", res.Status)
	require.EqualValues(t, 3, res.Turn)
	require.NotNil(t, res.Started)
	require.True(t, *res.Started)

	core.SetStatus(addr, &herdr.Status{State: "blocked", App: "claude-code"})
	_, err = c.Send(ctx, addr, "y", false)
	require.True(t, herdr.IsKind(err, "blocked"), "%v", err)
	_, err = c.Send(ctx, addr, "y", true)
	require.NoError(t, err)
	require.JSONEq(t, `{"terminal":"t1","text":"y","force":true}`, params("terminal.send"))

	require.NoError(t, c.Keys(ctx, addr, []string{"esc", "shift+tab", "enter"}))
	require.JSONEq(t, `{"terminal":"t1","keys":["esc","shift+tab","enter"]}`, params("terminal.keys"))

	core.SetScreen(addr, "one\ntwo\nthree")
	text, err := c.Read(ctx, addr, 2)
	require.NoError(t, err)
	require.Equal(t, "two\nthree", text)
	require.JSONEq(t, `{"terminal":"t1","lines":2}`, params("terminal.read"))

	outcome, err := c.Interrupt(ctx, addr)
	require.NoError(t, err)
	require.Equal(t, "stopped", outcome)
	require.JSONEq(t, `{"terminal":"t1","timeout":5}`, params("terminal.interrupt"))

	photo := []byte("\xff\xd8\xff\xe0 not really a jpeg \x00\x01")
	path, err := c.Put(ctx, addr, "photo.jpg", photo)
	require.NoError(t, err)
	require.Equal(t, "/tmp/herdrtest/t1/photo.jpg", path)
	require.JSONEq(t, `{"terminal":"t1","name":"photo.jpg","data":"`+base64.StdEncoding.EncodeToString(photo)+`"}`, params("terminal.put"))
	saved, ok := core.File(addr, "photo.jpg")
	require.True(t, ok)
	require.Equal(t, photo, saved)

	_, err = c.Doc(ctx, "herdr-app")
	require.NoError(t, err)
	require.JSONEq(t, `{"name":"herdr-app"}`, params("doc.get"))

	_, _, err = c.Subscribe(ctx)
	require.NoError(t, err)
	require.JSONEq(t, `{}`, params("events.subscribe"), "unfiltered: every terminal and document")
}

// Doc tells a document that was never written (or was deleted) from one
// with a value.
func TestDoc(t *testing.T) {
	t.Parallel()
	core := herdrtest.New(t, herdrtest.Options{})
	c := core.Connect()
	ctx := testCtx(t)

	v, err := c.Doc(ctx, "herdr-app")
	require.NoError(t, err)
	require.Nil(t, v)

	core.SetDoc("herdr-app", map[string]any{"spaces": []string{"work"}})
	v, err = c.Doc(ctx, "herdr-app")
	require.NoError(t, err)
	require.JSONEq(t, `{"spaces":["work"]}`, string(v))

	core.SetDoc("herdr-app", nil)
	v, err = c.Doc(ctx, "herdr-app")
	require.NoError(t, err)
	require.Nil(t, v)
}

// A subscription reply without a channel (plain mode's shape) can't carry
// events in framed mode, so Subscribe refuses it rather than wait forever.
func TestSubscribeWithoutChannel(t *testing.T) {
	t.Parallel()
	core := herdrtest.New(t, herdrtest.Options{})
	core.Handle("events.subscribe", func(herdrtest.Call) (any, error) {
		return map[string]any{"seq": 0, "terminals": []any{}}, nil
	})
	_, _, err := core.Connect().Subscribe(testCtx(t))
	require.ErrorContains(t, err, "no channel")
}
