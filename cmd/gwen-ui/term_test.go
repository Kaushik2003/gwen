package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/herdr"
	"github.com/kzark/gwen/internal/remote"
	"github.com/kzark/gwen/internal/wire"
	"github.com/kzark/gwen/term"
	"github.com/stretchr/testify/require"
)

var termT0 = time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)

type emitted struct {
	name string
	data any
}

// recorder is an emit that hands every event to the test.
func recorder() (func(string, ...any), chan emitted) {
	ch := make(chan emitted, 256)
	return func(name string, data ...any) {
		var d any
		if len(data) > 0 {
			d = data[0]
		}
		ch <- emitted{name, d}
	}, ch
}

func TestTermInfo(t *testing.T) {
	t.Parallel()
	shell := herdr.Terminal{Address: "t3", Title: "kzark@fedora: ~/src", Argv: []string{"/bin/bash"}, Cwd: "/home/k", State: "live", PID: 10,
		Cols: 120, Rows: 40, Labels: map[string]string{"gwen.project": "prj_1", "gwen.task": "tsk_2"},
		Foreground: &herdr.Foreground{PID: 10, Name: "bash", Argv: []string{"-bash"}, Cwd: "/home/k/src"}}
	require.Equal(t, TermInfo{Address: "t3", Name: "kzark@fedora: ~/src", Cwd: "/home/k/src", Project: "prj_1", Task: "tsk_2", Cols: 120, Rows: 40, Live: true},
		termInfo(shell))

	claude := herdr.Terminal{Address: "t4", Name: "fix-it", Argv: []string{"claude"}, Cwd: "/home/k/gwen", State: "live", PID: 11, Parent: "t1",
		Labels:     map[string]string{"gwen.session": "cs_abc"},
		Status:     &herdr.Status{State: "working", App: "other"},
		Records:    map[string]herdr.Status{"x": {State: "blocked", App: "claude-code"}},
		Foreground: &herdr.Foreground{PID: 12, Name: "claude", Argv: []string{"claude"}}}
	require.Equal(t, TermInfo{Address: "t4", Name: "fix-it", Cwd: "/home/k/gwen", Status: "blocked", App: "claude-code", SessionID: "cs_abc",
		Claude: true, Parent: "t1", Live: true}, termInfo(claude))

	// A kept terminal getting a new shell has no program for a moment.
	pending := herdr.Terminal{Address: "t5", Argv: []string{"/usr/bin/fish"}, State: "live", Status: &herdr.Status{State: "idle"}}
	require.Equal(t, TermInfo{Address: "t5", Name: "fish", Status: "idle"}, termInfo(pending))
}

func TestTermInfosKeepLiveLocalTerminalsInOrder(t *testing.T) {
	t.Parallel()
	got := termInfos([]herdr.Terminal{
		{Address: "t10", State: "live", PID: 1},
		{Address: "t2", State: "live", PID: 1},
		{Address: "t7", State: "finished"},
		{Address: "box/t3", State: "live", PID: 1},
		{Address: "t9", State: "live", PID: 1},
	})
	var addrs []string
	for _, info := range got {
		addrs = append(addrs, info.Address)
	}
	require.Equal(t, []string{"t2", "t9", "t10"}, addrs)
	require.Equal(t, []TermInfo{}, termInfos(nil))
}

func TestRunParams(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "notes.txt")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
	p, err := runParams(TermStartRequest{Cwd: dir + "/", Project: "prj_1"}, "/usr/bin/zsh")
	require.NoError(t, err)
	require.Equal(t, herdr.RunParams{Argv: []string{"/usr/bin/zsh"}, Cwd: dir, Labels: map[string]string{"gwen.project": "prj_1"}, Keep: true, NoParent: true}, p)

	p, err = runParams(TermStartRequest{Cwd: dir, Name: "build-1", Argv: []string{"make", "watch"}}, "")
	require.NoError(t, err)
	require.Equal(t, herdr.RunParams{Argv: []string{"make", "watch"}, Cwd: dir, Name: "build-1", Keep: true, NoParent: true}, p)

	p, err = runParams(TermStartRequest{Cwd: dir}, "")
	require.NoError(t, err)
	require.Equal(t, []string{"/bin/bash"}, p.Argv)

	for name, req := range map[string]TermStartRequest{
		"relative folder": {Cwd: "src"},
		"no folder":       {},
		"missing folder":  {Cwd: filepath.Join(dir, "gone")},
		"a file":          {Cwd: file},
		"bad name":        {Cwd: dir, Name: "Has Spaces"},
		"long name":       {Cwd: dir, Name: "a123456789012345678901234567890123"},
	} {
		_, err := runParams(req, "/bin/sh")
		var he *hostError
		require.ErrorAs(t, err, &he, name)
		require.Equal(t, wire.CodeInvalidRequest, he.Code, name)
	}
}

func TestTermError(t *testing.T) {
	t.Parallel()
	coreErr := func(kind string) error {
		e := &herdr.Error{Message: kind}
		e.Data.Kind = kind
		return e
	}
	for _, tc := range []struct {
		err  error
		code string
	}{
		{errCoreDown, wire.CodeUnavailable},
		{errCoreOutdated, wire.CodeUnavailable},
		{remote.ErrNoCore, wire.CodeUnavailable},
		{errNoView, wire.CodeNotFound},
		{herdr.ErrClosed, wire.CodeUnavailable},
		{coreErr("not_found"), wire.CodeNotFound},
		{coreErr("name_taken"), wire.CodeConflict},
		{coreErr("live"), wire.CodeConflict},
		{errors.New("boom"), wire.CodeInternal},
	} {
		var he *hostError
		require.ErrorAs(t, termError(tc.err), &he)
		require.Equal(t, tc.code, he.Code, tc.err.Error())
		require.NotEmpty(t, he.Message)
	}
	require.NoError(t, termError(nil))
	require.Contains(t, coreMessage(errCoreDown), "isn't running")
	require.Contains(t, coreMessage(errCoreOutdated), "make term")
}

// fakeStream is an attach stream the test fills.
type fakeStream struct {
	mu      sync.Mutex
	frames  []herdr.Frame
	credits []int
	written [][]byte
	resizes []herdr.Size
	closed  bool
	wake    chan struct{}
}

func newFakeStream(frames ...herdr.Frame) *fakeStream {
	return &fakeStream{frames: frames, wake: make(chan struct{}, 1)}
}

func (s *fakeStream) push(f herdr.Frame) {
	s.mu.Lock()
	s.frames = append(s.frames, f)
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *fakeStream) Next(ctx context.Context) (herdr.Frame, error) {
	for {
		s.mu.Lock()
		if len(s.frames) > 0 {
			f := s.frames[0]
			s.frames = s.frames[1:]
			s.mu.Unlock()
			return f, nil
		}
		s.mu.Unlock()
		select {
		case <-s.wake:
		case <-ctx.Done():
			return herdr.Frame{}, ctx.Err()
		}
	}
}

func (s *fakeStream) Credit(n int) {
	s.mu.Lock()
	s.credits = append(s.credits, n)
	s.mu.Unlock()
}

func (s *fakeStream) Write(p []byte) error {
	s.mu.Lock()
	s.written = append(s.written, append([]byte(nil), p...))
	s.mu.Unlock()
	return nil
}

func (s *fakeStream) Resize(cols, rows int) error {
	s.mu.Lock()
	s.resizes = append(s.resizes, herdr.Size{Cols: cols, Rows: rows})
	s.mu.Unlock()
	return nil
}

func (s *fakeStream) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.push(herdr.Frame{Closed: &herdr.CloseReason{Reason: "closed"}})
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestViewPassesOutputInOrderOnceThePageListens(t *testing.T) {
	t.Parallel()
	emit, events := recorder()
	terms := newTerminals(clock.NewFake(termT0), "", emit)
	s := newFakeStream(
		herdr.Frame{Data: []byte("ab")},
		herdr.Frame{Data: []byte("cd")},
		herdr.Frame{Resize: &herdr.Size{Cols: 100, Rows: 30}},
		herdr.Frame{Data: []byte("ef")},
	)
	id := terms.open("t1", s, herdr.Size{Cols: 80, Rows: 24})

	// Nothing reaches the page before it listens: the first paint waits.
	require.Empty(t, events)
	require.NoError(t, terms.resize(id, 90, 25))

	require.Equal(t, emitted{"term:data:" + id, b64("abcd")}, <-events)
	require.Equal(t, emitted{"term:size:" + id, herdr.Size{Cols: 100, Rows: 30}}, <-events)
	require.Equal(t, emitted{"term:data:" + id, b64("ef")}, <-events)

	require.NoError(t, terms.input(id, b64("ls\r")))
	require.NoError(t, terms.input(id, ""))
	var he *hostError
	require.ErrorAs(t, termError(terms.input(id, "not base64!")), &he)

	s.push(herdr.Frame{Closed: &herdr.CloseReason{Reason: "exited"}})
	require.Equal(t, emitted{"term:closed:" + id, termClosed{Reason: "exited"}}, <-events)
	// The view is forgotten by the time the page hears it closed.
	require.ErrorIs(t, terms.input(id, b64("x")), errNoView)
	require.ErrorIs(t, terms.resize(id, 80, 24), errNoView)

	s.mu.Lock()
	defer s.mu.Unlock()
	require.Equal(t, []int{4, 2}, s.credits, "credit is exactly what was passed on")
	require.Equal(t, [][]byte{[]byte("ls\r")}, s.written)
	require.Equal(t, []herdr.Size{{Cols: 90, Rows: 25}}, s.resizes)
}

func TestViewBatchesUpToTheLimit(t *testing.T) {
	t.Parallel()
	emit, events := recorder()
	terms := newTerminals(clock.NewFake(termT0), "", emit)
	chunk := make([]byte, maxBatch/2+1)
	s := newFakeStream(herdr.Frame{Data: chunk}, herdr.Frame{Data: chunk}, herdr.Frame{Data: chunk})
	id := terms.open("t1", s, herdr.Size{Cols: 80, Rows: 24})
	require.NoError(t, terms.input(id, ""))
	for range 2 {
		ev := <-events
		raw, err := base64.StdEncoding.DecodeString(ev.data.(string))
		require.NoError(t, err)
		require.LessOrEqual(t, len(raw), maxBatch+len(chunk))
	}
	s.push(herdr.Frame{Closed: &herdr.CloseReason{Reason: "killed"}})
	require.Equal(t, emitted{"term:closed:" + id, termClosed{Reason: "killed"}}, <-events)
	s.mu.Lock()
	defer s.mu.Unlock()
	require.Equal(t, []int{2 * len(chunk), len(chunk)}, s.credits)
}

func TestDetachClosesQuietly(t *testing.T) {
	t.Parallel()
	emit, events := recorder()
	terms := newTerminals(clock.NewFake(termT0), "", emit)
	s := newFakeStream()
	id := terms.open("t1", s, herdr.Size{Cols: 80, Rows: 24})
	require.NoError(t, terms.resize(id, 80, 24))
	terms.detach(id)
	terms.detach(id) // a second detach is nothing
	require.ErrorIs(t, terms.input(id, b64("x")), errNoView)
	s.mu.Lock()
	require.True(t, s.closed)
	s.mu.Unlock()
	// A view that is still listening hears a lost connection.
	s2 := newFakeStream()
	id2 := terms.open("t2", s2, herdr.Size{Cols: 80, Rows: 24})
	require.NoError(t, terms.resize(id2, 80, 24))
	s2.push(herdr.Frame{Closed: &herdr.CloseReason{Reason: "lost"}})
	require.Equal(t, emitted{"term:closed:" + id2, termClosed{Reason: "lost"}}, <-events)
	require.Empty(t, events, "the detached view said nothing")
}

func TestResizedFollowsTheTerminalsGrid(t *testing.T) {
	t.Parallel()
	emit, events := recorder()
	terms := newTerminals(clock.NewFake(termT0), "", emit)
	s := newFakeStream()
	id := terms.open("t1", s, herdr.Size{Cols: 80, Rows: 24})
	other := terms.open("t2", newFakeStream(), herdr.Size{Cols: 80, Rows: 24})

	// Before the page listens it draws at the size it attached with.
	terms.resized("t1", herdr.Size{Cols: 120, Rows: 40})
	require.Empty(t, events)

	require.NoError(t, terms.resize(id, 80, 24))
	require.NoError(t, terms.resize(other, 80, 24))
	terms.resized("t1", herdr.Size{Cols: 80, Rows: 24}) // its own grid
	terms.resized("t1", herdr.Size{Cols: 120, Rows: 40})
	require.Equal(t, emitted{"term:size:" + id, herdr.Size{Cols: 120, Rows: 40}}, <-events)
	terms.resized("t1", herdr.Size{Cols: 120, Rows: 40})
	terms.resized("t1", herdr.Size{Cols: 0, Rows: 40})
	// Typing makes it the active viewer again: the core goes back to its grid.
	terms.resized("t1", herdr.Size{Cols: 80, Rows: 24})
	require.Equal(t, emitted{"term:size:" + id, herdr.Size{Cols: 80, Rows: 24}}, <-events)
	terms.detach(id)
	terms.detach(other)
	require.Empty(t, events)
}

func TestTermChangedIsCoalesced(t *testing.T) {
	t.Parallel()
	emit, events := recorder()
	f := clock.NewFake(termT0)
	terms := newTerminals(f, "", emit)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go terms.notify(ctx)

	for _, typ := range []string{"status", "title", "size", "turn", "labels"} {
		terms.event(herdr.Event{Type: typ, Terminal: "t1", Data: json.RawMessage(`{}`)})
	}
	f.BlockUntil(1)
	f.Advance(changeDelay - time.Millisecond)
	require.Equal(t, 1, f.Armed())
	f.Advance(time.Millisecond)
	require.Equal(t, emitted{"term:changed", nil}, <-events)
	require.Zero(t, f.Armed(), "changes during the wait were covered")

	// size and turn alone change nothing the sidebar shows.
	terms.event(herdr.Event{Type: "size", Terminal: "t1", Data: json.RawMessage(`{"cols":9,"rows":9}`)})
	terms.event(herdr.Event{Type: "turn", Terminal: "t1", Data: json.RawMessage(`{"turn":3}`)})
	require.Zero(t, f.Armed())
	terms.event(herdr.Event{Type: "removed", Terminal: "t1"})
	f.BlockUntil(1)
	f.Advance(changeDelay)
	require.Equal(t, emitted{"term:changed", nil}, <-events)
}

func TestWatchBacksOff(t *testing.T) {
	t.Parallel()
	emit, _ := recorder()
	f := clock.NewFake(termT0)
	terms := newTerminals(f, "/nonexistent/gwen.sock", emit)
	dials := make(chan struct{}, 16)
	terms.dial = func(context.Context, string) (*herdr.Client, error) {
		dials <- struct{}{}
		return nil, errors.New("connection refused")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go terms.watch(ctx)

	<-dials
	for _, d := range []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 10 * time.Second} {
		f.BlockUntil(1)
		f.Advance(d - time.Millisecond)
		require.Equal(t, 1, f.Armed(), "no attempt before %s", d)
		f.Advance(time.Millisecond)
		<-dials
	}
	// A call that made a new connection wakes it at once.
	f.BlockUntil(1)
	terms.kick <- struct{}{}
	<-dials
}

// fakeCore is a herdr-core on a Unix socket: it says hello, answers calls
// with handle, and hands every stream frame from the client to the test.
type fakeCore struct {
	ln     net.Listener
	socket string
	handle func(method string, params json.RawMessage) (result any, then func(send func(channel uint32, kind byte, p []byte)), err *herdr.Error)
	frames chan coreFrame

	mu   sync.Mutex
	conn net.Conn
}

type coreFrame struct {
	channel uint32
	kind    byte
	payload []byte
}

func newFakeCore(t *testing.T, socket string, handle func(string, json.RawMessage) (any, func(func(uint32, byte, []byte)), *herdr.Error)) *fakeCore {
	t.Helper()
	ln, err := net.Listen("unix", socket)
	require.NoError(t, err)
	fc := &fakeCore{ln: ln, socket: socket, handle: handle, frames: make(chan coreFrame, 64)}
	t.Cleanup(func() {
		ln.Close()
		fc.mu.Lock()
		if fc.conn != nil {
			fc.conn.Close()
		}
		fc.mu.Unlock()
	})
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			fc.mu.Lock()
			fc.conn = conn
			fc.mu.Unlock()
			go fc.serve(conn)
		}
	}()
	return fc
}

func (fc *fakeCore) send(channel uint32, kind byte, p []byte) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	frame := make([]byte, 9+len(p))
	binary.LittleEndian.PutUint32(frame[0:4], channel)
	frame[4] = kind
	binary.LittleEndian.PutUint32(frame[5:9], uint32(len(p)))
	copy(frame[9:], p)
	_, _ = fc.conn.Write(frame)
}

func (fc *fakeCore) serve(conn net.Conn) {
	r := bufio.NewReader(conn)
	if _, err := r.ReadBytes('\n'); err != nil {
		return
	}
	hello, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 0, "result": map[string]any{"generation": herdr.Generation, "version": term.CoreVersion()}})
	if _, err := conn.Write(append(hello, '\n')); err != nil {
		return
	}
	var header [9]byte
	for {
		if _, err := io.ReadFull(r, header[:]); err != nil {
			return
		}
		channel, kind := binary.LittleEndian.Uint32(header[0:4]), header[4]
		payload := make([]byte, binary.LittleEndian.Uint32(header[5:9]))
		if _, err := io.ReadFull(r, payload); err != nil {
			return
		}
		if channel != 0 {
			fc.frames <- coreFrame{channel, kind, payload}
			continue
		}
		var req struct {
			ID     uint64          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(payload, &req) != nil {
			continue
		}
		result, then, cerr := fc.handle(req.Method, req.Params)
		msg := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		if cerr != nil {
			msg["error"] = cerr
		} else {
			msg["result"] = result
		}
		reply, _ := json.Marshal(msg)
		fc.send(0, 0, reply)
		if then != nil {
			then(fc.send)
		}
	}
}

func coreTerminals(t *testing.T, emit func(string, ...any)) (*terminals, string) {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "c.sock")
	terms := newTerminals(clock.NewFake(termT0), socket, emit)
	t.Cleanup(func() {
		terms.mu.Lock()
		defer terms.mu.Unlock()
		if terms.c != nil {
			terms.c.Close()
		}
	})
	return terms, socket
}

func TestTermsAgainstACore(t *testing.T) {
	t.Parallel()
	emit, events := recorder()
	terms, socket := coreTerminals(t, emit)
	ctx := context.Background()

	// No core: the list says so.
	list := terms.list(ctx)
	require.Equal(t, "down", list.Core)
	require.Contains(t, list.Message, "isn't running")
	require.Empty(t, list.Terminals)

	shell := herdr.Terminal{Address: "t1", Argv: []string{"/bin/bash"}, Cwd: "/tmp", State: "live", PID: 5, Cols: 80, Rows: 24}
	session := herdr.Terminal{Address: "t2", Argv: []string{"/bin/bash"}, State: "live", PID: 6, Labels: map[string]string{"gwen.session": "cs_1"}}
	var killed []string
	var mu sync.Mutex
	core := newFakeCore(t, socket, func(method string, params json.RawMessage) (any, func(func(uint32, byte, []byte)), *herdr.Error) {
		var p map[string]any
		_ = json.Unmarshal(params, &p)
		switch method {
		case "terminal.list":
			return map[string]any{"terminals": []herdr.Terminal{session, shell, {Address: "box/t1", State: "live", PID: 1}}}, nil, nil
		case "terminal.run":
			rec := shell
			rec.Address, rec.Cwd, rec.Argv = "t3", p["cwd"].(string), []string{p["argv"].([]any)[0].(string)}
			rec.Labels = map[string]string{}
			for k, v := range p["labels"].(map[string]any) {
				rec.Labels[k] = v.(string)
			}
			if p["keep"] != true || p["no_parent"] != true {
				return nil, nil, &herdr.Error{Message: "want keep and no_parent"}
			}
			return rec, nil, nil
		case "terminal.label":
			return map[string]any{}, nil, nil
		case "terminal.get":
			if p["terminal"] == "t2" {
				return session, nil, nil
			}
			return shell, nil, nil
		case "terminal.kill":
			mu.Lock()
			killed = append(killed, p["terminal"].(string))
			mu.Unlock()
			return shell, nil, nil
		case "terminal.attach":
			if p["observe"] != false || p["history"] != float64(viewHistory) || p["cols"] != float64(100) || p["rows"] != float64(30) {
				return nil, nil, &herdr.Error{Message: "bad attach"}
			}
			return map[string]any{"channel": 9, "terminal": shell, "resize_notifications": false},
				func(send func(uint32, byte, []byte)) {
					send(9, 0, []byte("first "))
					send(9, 0, []byte("paint"))
				}, nil
		}
		e := &herdr.Error{Code: -32601, Message: "no method " + method}
		return nil, nil, e
	})

	list = terms.list(ctx)
	require.Equal(t, "ok", list.Core)
	require.Equal(t, []string{"t1", "t2"}, []string{list.Terminals[0].Address, list.Terminals[1].Address})
	require.Equal(t, "cs_1", list.Terminals[1].SessionID)

	dir := t.TempDir()
	info, err := terms.run(ctx, TermStartRequest{Cwd: dir, Project: "prj_9"})
	require.NoError(t, err)
	require.Equal(t, "t3", info.Address)
	require.Equal(t, dir, info.Cwd)
	require.Equal(t, "prj_9", info.Project)

	require.NoError(t, terms.label(ctx, "t3", map[string]string{"gwen.task": "tsk_1"}))
	require.Error(t, terms.label(ctx, "", map[string]string{"gwen.task": "tsk_1"}))

	require.NoError(t, terms.end(ctx, "t1"))
	var he *hostError
	require.ErrorAs(t, termError(terms.end(ctx, "t2")), &he)
	require.Equal(t, wire.CodeConflict, he.Code)
	mu.Lock()
	require.Equal(t, []string{"t1"}, killed, "a session's terminal is never ended here")
	mu.Unlock()

	// A view: output waits until the page listens, credit goes back for
	// exactly what reached it, and typing goes in on the view's channel.
	id, err := terms.attach(ctx, "t1", 100, 30)
	require.NoError(t, err)
	require.NoError(t, terms.resize(id, 100, 30))
	got := ""
	for got != "first paint" {
		ev := <-events
		require.Equal(t, "term:data:"+id, ev.name)
		raw, err := base64.StdEncoding.DecodeString(ev.data.(string))
		require.NoError(t, err)
		got += string(raw)
	}
	require.NoError(t, terms.input(id, b64("ls\r")))
	credit, typed, resized := 0, "", false
	for credit < len("first paint") || typed != "ls\r" || !resized {
		f := <-core.frames
		require.Equal(t, uint32(9), f.channel)
		switch f.kind {
		case 0:
			typed += string(f.payload)
		case 1:
			credit += int(binary.LittleEndian.Uint32(f.payload))
		case 3:
			require.Equal(t, []byte{100, 0, 30, 0}, f.payload)
			resized = true
		}
	}
	require.Equal(t, len("first paint"), credit)

	core.send(9, 2, []byte(`{"reason":"exited","exit_code":0}`))
	require.Equal(t, emitted{"term:closed:" + id, termClosed{Reason: "exited"}}, <-events)
	require.ErrorIs(t, terms.input(id, b64("x")), errNoView)

	_, err = terms.attach(ctx, "t1", 0, 30)
	require.ErrorAs(t, err, &he)
	require.Equal(t, wire.CodeInvalidRequest, he.Code)

	// The connection drops: open views hear it, and the next call connects
	// again.
	id, err = terms.attach(ctx, "t1", 100, 30)
	require.NoError(t, err)
	require.NoError(t, terms.resize(id, 100, 30))
	core.mu.Lock()
	core.conn.Close()
	core.mu.Unlock()
	for ev := range events {
		if ev.name == "term:closed:"+id {
			require.Equal(t, termClosed{Reason: "lost"}, ev.data)
			break
		}
	}
	list = terms.list(ctx)
	require.Equal(t, "ok", list.Core, list.Message)
	require.Len(t, list.Terminals, 2)
}

func TestTermStartStartsTheCore(t *testing.T) {
	t.Parallel()
	emit, _ := recorder()
	terms, socket := coreTerminals(t, emit)
	f := terms.clk.(*clock.Fake)
	started := make(chan struct{}, 2)
	terms.start = func(context.Context) error {
		started <- struct{}{}
		return nil
	}
	done := make(chan error, 1)
	dir := t.TempDir()
	go func() {
		_, err := terms.run(context.Background(), TermStartRequest{Cwd: dir})
		done <- err
	}()
	<-started
	newFakeCore(t, socket, func(method string, params json.RawMessage) (any, func(func(uint32, byte, []byte)), *herdr.Error) {
		if method != "terminal.run" {
			return nil, nil, &herdr.Error{Message: "unexpected " + method}
		}
		return herdr.Terminal{Address: "t1", State: "live", PID: 3}, nil, nil
	})
	f.BlockUntil(1)
	f.Advance(corePoll)
	require.NoError(t, <-done)
	require.Empty(t, started, "the core started once")
	select {
	case <-terms.kick:
	default:
		t.Fatal("a new connection wakes the watcher")
	}
}

func TestTermStartGivesUpOnACoreThatNeverAnswers(t *testing.T) {
	t.Parallel()
	emit, _ := recorder()
	terms, _ := coreTerminals(t, emit)
	f := terms.clk.(*clock.Fake)
	terms.start = func(context.Context) error { return errors.New("systemd-run: no user session") }
	done := make(chan error, 1)
	dir := t.TempDir()
	go func() {
		_, err := terms.run(context.Background(), TermStartRequest{Cwd: dir})
		done <- err
	}()
	for range int(coreStartWait / corePoll) {
		f.BlockUntil(1)
		f.Advance(corePoll)
	}
	err := <-done
	require.ErrorIs(t, err, errCoreNoStart)
	require.ErrorContains(t, err, "no user session")

	// Without Gwen's core installed there is nothing to wait for.
	terms.start = func(context.Context) error { return remote.ErrNoCore }
	_, err = terms.run(context.Background(), TermStartRequest{Cwd: dir})
	require.ErrorIs(t, err, remote.ErrNoCore)
}
