// Package herdrtest is a fake herdr-core for tests (plan.md 6.3, W27). It
// listens on a Unix socket in a temporary folder and speaks the protocol of
// herdr-core's schema/README.md as internal/herdr uses it: the hello line,
// plain JSON-RPC (one message per line), framed mode after a hello with
// framed true, events with a snapshot, `missed` and heartbeats, and attach
// streams with credit, the 4 MiB repaint, RESIZE frames and close reasons.
// Its flow control copies herdr-core's own (crates/herdr-core/src/wire.rs):
// replies go first, each stream sends only within its credit, 16 KiB at a
// time, and streams take turns, so a stalled stream never holds up calls.
//
// # Starting one
//
//	core := herdrtest.New(t, herdrtest.Options{})
//	t1 := core.AddTerminal(herdr.Terminal{Name: "claude", Cwd: "/src"})
//	c := core.Connect() // herdr.Dial(ctx, core.Socket()), closed when the test ends
//	events, terms, err := c.Subscribe(ctx) // terms holds t1
//	core.SetStatus(t1.Address, &herdr.Status{State: "working", App: "claude-code"})
//	ev, err := events.Next(ctx) // the status event
//
// Options sets what hello reports (Generation, Version, Features; by default
// the generation and version Gwen accepts) and the flow control limits. To
// point code that dials by path at it, use Socket(); for remote.Options.Dial,
// pass func(ctx context.Context, _ string) (*herdr.Client, error) {
// return herdr.Dial(ctx, core.Socket()) }.
//
// # Terminals, events and documents
//
// AddTerminal, SetStatus, SetForeground, SetLabels, SetTitle, SetTurn,
// Resize, Finish, Lose and Remove change the fake's terminals and send the
// event herdr-core would send to every subscriber; Update makes any other
// change with an event of your choosing. SetScreen sets what terminal.read
// returns and what a paint shows (Paint). SetDoc writes a document and sends
// its doc event. Emit sends an arbitrary event, EmitLine any raw line (a
// stale seq, junk), Missed a `missed` with a fresh snapshot, and Heartbeat a
// heartbeat. A subscriber that falls more than MaxPending behind gets
// `missed` on its own, as with the real core. Event filters
// (terminal, children_of, labels) are ignored: every subscriber sees all.
//
// # Calls
//
// Every request is recorded, hello included: Calls(method) returns them and
// WaitCalls(method, n) waits for the nth. The built-in methods are hello,
// events.subscribe, terminal.attach, terminal.list, terminal.get,
// terminal.run, terminal.label, terminal.send, terminal.keys, terminal.read,
// terminal.interrupt, terminal.put, doc.get and doc.set. Handle replaces any
// of them, or adds a method; the handler may return a herdr-core error with
// Err, a json.RawMessage to send raw, or fall back to the built-in with
// call.Builtin(). Handlers run side by side, one goroutine per call, so one
// may block on a channel to model a slow call:
//
//	core.Handle("terminal.send", func(call herdrtest.Call) (any, error) {
//		if sends++; sends == 1 {
//			return nil, herdrtest.Err("blocked", "t1 is asking something")
//		}
//		return call.Builtin()
//	})
//
// # Attach streams
//
// Each terminal.attach opens a Stream, the core's end of the view.
// WaitStreams(terminal, n) returns the first n opened on a terminal, in
// order. Output(terminal, bytes) is program output to every viewer of it;
// Stream.Push writes to one viewer. Both respect credit (Options.Window to
// start with, then what the client returns), and a viewer more than
// Options.MaxPending behind gets a fresh Paint instead of the backlog. What
// the client did is recorded: Input (typed bytes), Credited, Resizes (its
// RESIZE frames) and Detached (it sent CLOSE, which the core answers with an
// empty CLOSE); Sent and Queued say what went out and what waits for credit.
// Stream.Finish closes it with a reason once its queued output is out (a
// program that ended), Stream.Close closes it at once, and a zero
// herdr.CloseReason sends an empty CLOSE. The last active typing viewer
// sets the terminal's size, as in herdr-core: a typing attach, or typing or
// RESIZE on it, with another grid resizes the terminal, sends a size event,
// and gives observers that asked for resize notifications a RESIZE and a
// fresh paint. Observers never type or resize: Core.Input(terminal) holds
// only what typing viewers sent. Subscribers returns the framed event
// subscriptions as Streams too (Terminal ""), for their Credited and Sent.
//
// # Connections
//
// DropConnections ends every connection, as when the core goes away for a
// moment; Stop closes the socket too, so dialing fails, until Start;
// Restart is a new core process (sequence numbers start again). Connections
// and WaitConnections count open connections.
//
// # Waiting
//
// Every Wait method blocks until its condition holds and fails the test
// given to New after Options.Timeout (10 seconds by default) as a guard, so
// call them from that test's goroutine; in parallel subtests, make one core
// per subtest. Everything else is safe from any goroutine, except that
// Update's change function runs with the core locked and must not call it.
package herdrtest

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/herdr"
	"github.com/kzark/gwen/term"
)

// Flow control defaults, as in herdr-core's wire.rs.
const (
	// DefaultWindow is the credit a stream starts with.
	DefaultWindow = 256 << 10
	// DefaultMaxPending is how far a viewer may fall behind before it gets
	// a repaint, or an event subscriber `missed`, instead of the backlog.
	DefaultMaxPending = 4 << 20
	// chunk is the largest DATA frame of a stream.
	chunk = 16 << 10
)

// Options configures a fake core. The zero value is a core Gwen accepts.
type Options struct {
	// Generation is the protocol generation hello reports; 0 means
	// herdr.Generation. A hello that names another generation is refused
	// with kind unsupported_generation, as herdr-core does.
	Generation int
	// Version is the version hello reports; "" means term/core.version.
	Version string
	// Features is what hello reports; nil means resize_notifications.
	Features []string
	// Window is the credit each stream starts with; 0 means DefaultWindow.
	Window int
	// MaxPending is the backlog past which a viewer gets a repaint and a
	// subscriber `missed`; 0 means DefaultMaxPending.
	MaxPending int
	// Timeout bounds every Wait method; 0 means 10 seconds.
	Timeout time.Duration
}

// Call is one request the core received.
type Call struct {
	// Conn is the connection it came on: 1 for the first accepted, and so on.
	Conn   int
	Method string
	Params json.RawMessage

	id   json.RawMessage
	core *Core
	cn   *conn
}

// Decode unmarshals the call's params into v.
func (c Call) Decode(v any) error {
	if len(c.Params) == 0 {
		return nil
	}
	return json.Unmarshal(c.Params, v)
}

// Builtin runs the fake's own handling of the call, for a Handler that only
// changes some calls.
func (c Call) Builtin() (any, error) { return c.core.builtin(c) }

// Handler answers one call. A non-nil error becomes a JSON-RPC error (a
// *herdr.Error keeps its code and kind; any other error is internal); the
// result is marshalled as JSON, and a json.RawMessage is sent as it is.
type Handler func(call Call) (result any, err error)

// Err is a herdr-core error of the given kind (not_found, blocked,
// unavailable, conflict, ...) for a Handler to return.
func Err(kind, message string) *herdr.Error {
	e := &herdr.Error{Code: -32000, Message: message}
	e.Data.Kind = kind
	return e
}

// Core is a fake herdr-core. Its methods are safe for concurrent use; the
// Wait methods must run on the test's goroutine.
type Core struct {
	t      testing.TB
	opts   Options
	socket string
	wg     sync.WaitGroup

	mu       sync.Mutex
	changed  chan struct{} // closed and replaced on every change
	ln       net.Listener
	conns    map[*conn]struct{}
	nconn    int
	handlers map[string]Handler
	calls    []Call
	terms    map[string]*terminal
	order    []string // addresses in the order they were added
	nextTerm int
	seq      uint64
	docs     map[string]*document
	streams  []*Stream // attach streams in the order they opened
	subs     []*Stream // framed event subscriptions
	activity int64     // counts viewer activity, to find the last active viewer
	closed   bool
}

// New starts a fake core on a socket in a temporary folder; it stops when
// the test ends.
func New(t testing.TB, o Options) *Core {
	t.Helper()
	if o.Generation == 0 {
		o.Generation = herdr.Generation
	}
	if o.Version == "" {
		o.Version = term.CoreVersion()
	}
	if o.Features == nil {
		o.Features = []string{"resize_notifications"}
	}
	if o.Window <= 0 {
		o.Window = DefaultWindow
	}
	if o.MaxPending <= 0 {
		o.MaxPending = DefaultMaxPending
	}
	if o.Timeout <= 0 {
		o.Timeout = 10 * time.Second
	}
	// A socket path may not pass 108 bytes, which a long test name can.
	socket := filepath.Join(t.TempDir(), "core.sock")
	if len(socket) > 100 {
		dir, err := os.MkdirTemp("", "hc")
		if err != nil {
			t.Fatalf("herdrtest: socket folder: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(dir) })
		socket = filepath.Join(dir, "core.sock")
	}
	c := &Core{t: t, opts: o, socket: socket, changed: make(chan struct{}), conns: map[*conn]struct{}{},
		handlers: map[string]Handler{}, terms: map[string]*terminal{}, docs: map[string]*document{}}
	c.Start()
	t.Cleanup(c.shutdown)
	return c
}

// Socket is the path the core listens on.
func (c *Core) Socket() string { return c.socket }

// Connect dials the core with herdr.Dial and fails the test if it can't. The
// client is closed when the test ends.
func (c *Core) Connect() *herdr.Client {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), c.opts.Timeout)
	defer cancel()
	cl, err := herdr.Dial(ctx, c.socket)
	if err != nil {
		c.t.Fatalf("herdrtest: dial: %v", err)
	}
	c.t.Cleanup(cl.Close)
	return cl
}

// Start listens on the socket again after Stop.
func (c *Core) Start() {
	c.t.Helper()
	ln, err := net.Listen("unix", c.socket)
	if err != nil {
		c.t.Fatalf("herdrtest: listen: %v", err)
	}
	c.mu.Lock()
	c.ln = ln
	c.mu.Unlock()
	c.wg.Add(1)
	go c.accept(ln)
}

// Stop closes the socket and every connection: dialing fails until Start.
func (c *Core) Stop() {
	c.mu.Lock()
	ln := c.ln
	c.ln = nil
	c.mu.Unlock()
	if ln != nil {
		ln.Close()
	}
	c.DropConnections()
}

// Restart is a new core process: every connection ends, and sequence
// numbers start again from 0. Terminals and documents stay.
func (c *Core) Restart() {
	c.t.Helper()
	c.Stop()
	c.mu.Lock()
	c.seq = 0
	c.mu.Unlock()
	c.Start()
}

// DropConnections ends every open connection, and the core lets go of each
// before it returns; the socket stays open, so clients can dial again.
func (c *Core) DropConnections() {
	c.mu.Lock()
	conns := slices.Collect(maps.Keys(c.conns))
	c.mu.Unlock()
	for _, cn := range conns {
		cn.gone()
	}
}

// Connections is how many connections are open.
func (c *Core) Connections() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.conns)
}

// WaitConnections waits until exactly n connections are open.
func (c *Core) WaitConnections(n int) {
	c.t.Helper()
	c.waitUntil(fmt.Sprintf("%d open connections", n), func() bool { return len(c.conns) == n })
}

// Handle answers method with h from now on; nil restores the built-in.
func (c *Core) Handle(method string, h Handler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if h == nil {
		delete(c.handlers, method)
		return
	}
	c.handlers[method] = h
}

// Calls returns the calls received of method, or of every method when it is
// "", in the order they arrived.
func (c *Core) Calls(method string) []Call {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.callsLocked(method)
}

func (c *Core) callsLocked(method string) []Call {
	var out []Call
	for _, call := range c.calls {
		if method == "" || call.Method == method {
			out = append(out, call)
		}
	}
	return out
}

// WaitCalls waits until n calls of method ("" for any) have arrived and
// returns them all.
func (c *Core) WaitCalls(method string, n int) []Call {
	c.t.Helper()
	var out []Call
	c.waitUntil(fmt.Sprintf("%d %q calls", n, method), func() bool {
		out = c.callsLocked(method)
		return len(out) >= n
	})
	return out
}

// Streams returns the attach streams opened on a terminal, in order.
func (c *Core) Streams(terminal string) []*Stream {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.streamsLocked(terminal)
}

func (c *Core) streamsLocked(terminal string) []*Stream {
	var out []*Stream
	for _, s := range c.streams {
		if s.terminal == terminal {
			out = append(out, s)
		}
	}
	return out
}

// WaitStreams waits until n attach streams have opened on a terminal and
// returns them all, in order.
func (c *Core) WaitStreams(terminal string, n int) []*Stream {
	c.t.Helper()
	var out []*Stream
	c.waitUntil(fmt.Sprintf("%d streams on %s", n, terminal), func() bool {
		out = c.streamsLocked(terminal)
		return len(out) >= n
	})
	return out
}

// Subscribers returns the framed event subscriptions, in order, including
// those that ended.
func (c *Core) Subscribers() []*Stream {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.subs)
}

// WaitSubscribers waits until n framed event subscriptions have opened and
// returns them all.
func (c *Core) WaitSubscribers(n int) []*Stream {
	c.t.Helper()
	var out []*Stream
	c.waitUntil(fmt.Sprintf("%d event subscriptions", n), func() bool {
		out = slices.Clone(c.subs)
		return len(out) >= n
	})
	return out
}

// waitUntil blocks until cond, called with c.mu held, reports true.
func (c *Core) waitUntil(what string, cond func() bool) {
	c.t.Helper()
	timer := time.NewTimer(c.opts.Timeout)
	defer timer.Stop()
	for {
		c.mu.Lock()
		ok := cond()
		changed := c.changed
		c.mu.Unlock()
		if ok {
			return
		}
		select {
		case <-changed:
		case <-timer.C:
			c.t.Fatalf("herdrtest: gave up after %v waiting for %s", c.opts.Timeout, what)
		}
	}
}

// notifyLocked wakes every waiter.
func (c *Core) notifyLocked() {
	close(c.changed)
	c.changed = make(chan struct{})
}

func (c *Core) accept(ln net.Listener) {
	defer c.wg.Done()
	for {
		nc, err := ln.Accept()
		if err != nil {
			return
		}
		c.mu.Lock()
		if c.closed || c.ln != ln {
			c.mu.Unlock()
			nc.Close()
			return
		}
		c.nconn++
		cn := &conn{core: c, id: c.nconn, nc: nc, wake: make(chan struct{}, 1), done: make(chan struct{}),
			streams: map[uint32]*Stream{}, all: map[uint32]*Stream{}}
		c.conns[cn] = struct{}{}
		c.notifyLocked()
		c.mu.Unlock()
		c.wg.Add(2)
		go cn.writeLoop()
		go cn.readLoop()
	}
}

// shutdown stops the core when the test ends.
func (c *Core) shutdown() {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	c.Stop()
	c.wg.Wait()
}
