package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/herdr"
	"github.com/kzark/gwen/internal/remote"
	"github.com/kzark/gwen/internal/wire"
	"github.com/kzark/gwen/term"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Terminals in the Gwen window (plan.md 3.15). herdr-core session `gwen`
// runs them and the window draws them: gwen-ui is a client of Gwen's own
// core, as Gwen Terminal was. It lists the core's terminals, starts shells,
// and attaches as a viewer that may type, passing output to the page as Wails
// events and typing and grid size back. It never connects to the default
// core, which belongs to a Herdr of your own.
//
// Events, all from the Go side:
//   - term:changed (no payload, at most every 150 ms): terminals came, went,
//     or changed status, title, labels, or foreground program; refetch
//     TermList. It also fires when the core comes or goes.
//   - term:data:<view> (string): output bytes, base64.
//   - term:size:<view> ({cols, rows}): another viewer set the terminal's grid,
//     or this one took it back; draw at that size.
//   - term:closed:<view> ({reason}): the view ended: exited or killed (the
//     terminal ended), lost (the connection to the core dropped), or another
//     reason herdr-core gives (server_stop, signaled, ...).

// Labels Gwen keeps on a terminal (plan.md 3.15).
const (
	projectLabel = "gwen.project"
	taskLabel    = "gwen.task"
)

// Event names.
const (
	eventTermChanged = "term:changed"
	eventTermData    = "term:data:"
	eventTermSize    = "term:size:"
	eventTermClosed  = "term:closed:"
)

const (
	// viewHistory is how many scrollback lines a view shows first.
	viewHistory = 5000
	// maxBatch is the most output one term:data event carries.
	maxBatch = 256 << 10
	// changeDelay coalesces term:changed.
	changeDelay = 150 * time.Millisecond
	// coreStartWait is how long TermStart waits for a core it started.
	coreStartWait = 8 * time.Second
	// corePoll is how often TermStart looks for that core.
	corePoll = 200 * time.Millisecond
)

// reconnectDelays are the waits before each attempt to follow the core
// again: 1, 2, 5, then every 10 seconds (plan.md 3.14.5).
var reconnectDelays = []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second}

var (
	errCoreDown     = errors.New("herdr-core isn't running")
	errCoreOutdated = errors.New("herdr-core is out of date")
	errCoreNoStart  = errors.New("herdr-core didn't start")
	errNoView       = errors.New("no such terminal view")
)

// TermInfo is one terminal of Gwen's core, as the sidebar and tabs show it.
type TermInfo struct {
	Address string `json:"address"`
	Name    string `json:"name"`
	// Cwd is the foreground program's folder when known, else where the
	// terminal started.
	Cwd string `json:"cwd"`
	// Status is what the program reported through OSC 7501: working,
	// blocked, idle, done, error, or "".
	Status string `json:"status"`
	App    string `json:"app"`
	// SessionID, Project and Task are the labels gwen.session, gwen.project
	// and gwen.task.
	SessionID string `json:"session_id"`
	Project   string `json:"project"`
	Task      string `json:"task"`
	// Claude says Claude Code runs in it now.
	Claude bool `json:"claude"`
	// Parent is the terminal that started it, for a helper an agent opened.
	Parent string `json:"parent"`
	Cols   int    `json:"cols"`
	Rows   int    `json:"rows"`
	// Live says it has a program; a kept terminal that is being given a new
	// shell has none for a moment.
	Live bool `json:"live"`
}

// TermList is every live terminal on this laptop, and whether the core runs.
type TermList struct {
	Core      string     `json:"core"` // ok or down
	Message   string     `json:"message"`
	Terminals []TermInfo `json:"terminals"`
}

// TermStartRequest starts a terminal: Argv in Cwd, the login shell when
// Argv is empty.
type TermStartRequest struct {
	Cwd     string   `json:"cwd"`
	Name    string   `json:"name"`
	Project string   `json:"project"`
	Argv    []string `json:"argv"`
}

var (
	termsOnce sync.Once
	termsHub  *terminals
)

// terms is the window's one connection to the core, made on first use.
func (a *App) terms() *terminals {
	termsOnce.Do(func() {
		home, _ := os.UserHomeDir()
		emit := func(name string, data ...any) {
			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, name, data...)
			}
		}
		termsHub = newTerminals(clock.Real(), herdr.DefaultSocket(), emit)
		termsHub.start = func(ctx context.Context) error { return remote.StartCore(ctx, home) }
	})
	return termsHub
}

// startTerminals follows the core for the window's lifetime, so the page
// hears term:changed. The main window's OnStartup calls it once.
func (a *App) startTerminals(ctx context.Context) {
	t := a.terms()
	if t.started.Swap(true) {
		return
	}
	go t.notify(ctx)
	go t.watch(ctx)
}

func (a *App) termCtx(d time.Duration) (context.Context, context.CancelFunc) {
	parent := a.ctx
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, d)
}

// TermList lists the live terminals of Gwen's core on this laptop. When no
// core runs, Core is "down" and Message says why.
func (a *App) TermList() (*TermList, error) {
	ctx, cancel := a.termCtx(10 * time.Second)
	defer cancel()
	return a.terms().list(ctx), nil
}

// TermStart starts a terminal and returns it, starting the core first when
// none runs.
func (a *App) TermStart(req TermStartRequest) (*TermInfo, error) {
	ctx, cancel := a.termCtx(30 * time.Second)
	defer cancel()
	info, err := a.terms().run(ctx, req)
	return info, termError(err)
}

// TermLabel sets labels on a terminal; an empty value removes one. A task
// link is gwen.task.
func (a *App) TermLabel(address string, labels map[string]string) error {
	ctx, cancel := a.termCtx(10 * time.Second)
	defer cancel()
	return termError(a.terms().label(ctx, address, labels))
}

// TermEnd ends a shell terminal and its program. A terminal that runs a
// Claude session is refused: that session is ended through gwend.
func (a *App) TermEnd(address string) error {
	ctx, cancel := a.termCtx(20 * time.Second)
	defer cancel()
	return termError(a.terms().end(ctx, address))
}

// TermAttach opens a view of a terminal at the page's grid, as a viewer that
// may type, and returns its id. Output waits until the page's first
// TermResize or TermInput for the view, which it sends once it listens to
// the view's events, so the first paint is never lost.
func (a *App) TermAttach(address string, cols, rows int) (string, error) {
	ctx, cancel := a.termCtx(10 * time.Second)
	defer cancel()
	id, err := a.terms().attach(ctx, address, cols, rows)
	return id, termError(err)
}

// TermInput types bytes, base64, into a view.
func (a *App) TermInput(viewID, b64 string) error {
	return termError(a.terms().input(viewID, b64))
}

// TermResize announces a view's grid, which makes it the viewer whose size
// the terminal follows.
func (a *App) TermResize(viewID string, cols, rows int) error {
	return termError(a.terms().resize(viewID, cols, rows))
}

// TermDetach closes a view. The terminal keeps running.
func (a *App) TermDetach(viewID string) { a.terms().detach(viewID) }

// stream is the part of a herdr-core attach stream a view uses.
type stream interface {
	Next(ctx context.Context) (herdr.Frame, error)
	Credit(n int)
	Write(p []byte) error
	Resize(cols, rows int) error
	Close()
}

// terminals is the window's connection to the core: one lazily dialled
// client, shared by every call and view, made again when it drops.
type terminals struct {
	clk    clock.Clock
	socket string
	dial   func(ctx context.Context, socket string) (*herdr.Client, error)
	start  func(ctx context.Context) error
	emit   func(name string, data ...any)

	started atomic.Bool
	dialMu  sync.Mutex // one dial at a time
	startMu sync.Mutex // one core start at a time

	mu     sync.Mutex
	c      *herdr.Client
	views  map[string]*termView
	nextID uint64

	kick    chan struct{} // a new connection: follow it now
	changed chan struct{} // a change for term:changed
}

func newTerminals(clk clock.Clock, socket string, emit func(name string, data ...any)) *terminals {
	return &terminals{clk: clk, socket: socket, dial: herdr.Dial, emit: emit,
		start: func(context.Context) error { return remote.ErrNoCore },
		views: map[string]*termView{}, kick: make(chan struct{}, 1), changed: make(chan struct{}, 1)}
}

// client returns the shared connection, dialling when there is none or it
// dropped; fresh says it was just made.
func (t *terminals) client(ctx context.Context) (c *herdr.Client, fresh bool, err error) {
	t.dialMu.Lock()
	defer t.dialMu.Unlock()
	t.mu.Lock()
	c = t.c
	t.mu.Unlock()
	if c != nil {
		select {
		case <-c.Done():
		default:
			return c, false, nil
		}
	}
	dialCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	c, err = t.dial(dialCtx, t.socket)
	cancel()
	if err != nil {
		return nil, false, fmt.Errorf("%w: %w", errCoreDown, err)
	}
	if h := c.Hello(); h.Generation != herdr.Generation || !herdr.VersionAtLeast(h.Version, term.CoreVersion()) {
		c.Close()
		return nil, false, fmt.Errorf("%w: it is %s, Gwen needs %s", errCoreOutdated, h.Version, term.CoreVersion())
	}
	t.mu.Lock()
	t.c = c
	t.mu.Unlock()
	return c, true, nil
}

// conn is client for a call: a new connection wakes the watcher, so the page
// hears about changes on it at once.
func (t *terminals) conn(ctx context.Context) (*herdr.Client, error) {
	c, fresh, err := t.client(ctx)
	if fresh {
		select {
		case t.kick <- struct{}{}:
		default:
		}
	}
	return c, err
}

// call runs f on the shared connection, and once more on a new one when that
// connection turns out to be gone, as it is for a moment after the core
// restarts. Only calls that may run twice use it.
func (t *terminals) call(ctx context.Context, f func(c *herdr.Client) error) error {
	for try := 0; ; try++ {
		c, err := t.conn(ctx)
		if err != nil {
			return err
		}
		err = f(c)
		if try > 0 || !errors.Is(err, herdr.ErrClosed) {
			return err
		}
		t.mu.Lock()
		if t.c == c {
			t.c = nil
		}
		t.mu.Unlock()
		c.Close()
	}
}

// poke asks for a term:changed.
func (t *terminals) poke() {
	select {
	case t.changed <- struct{}{}:
	default:
	}
}

// notify sends term:changed changeDelay after the first change of a burst;
// changes while it waits are covered by the same event.
func (t *terminals) notify(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.changed:
		}
		timer := t.clk.NewTimer(changeDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C():
		}
		select {
		case <-t.changed:
		default:
		}
		t.emit(eventTermChanged)
	}
}

// watch follows the core until ctx ends, reconnecting after 1, 2, 5, then
// every 10 seconds, or at once when a call made a new connection.
func (t *terminals) watch(ctx context.Context) {
	attempt := 0
	for ctx.Err() == nil {
		if t.follow(ctx) {
			attempt = 0
		}
		timer := t.clk.NewTimer(reconnectDelays[min(attempt, len(reconnectDelays)-1)])
		attempt++
		select {
		case <-ctx.Done():
		case <-timer.C():
		case <-t.kick:
		}
		timer.Stop()
	}
}

// follow subscribes to the core's events on the shared connection and turns
// them into term:changed and term:size until the connection drops. It
// reports whether it subscribed.
func (t *terminals) follow(ctx context.Context) bool {
	c, _, err := t.client(ctx)
	if err != nil {
		return false
	}
	// A failed subscription or a stream that ended leaves the connection to
	// client, which dials again once it has dropped.
	events, _, err := c.Subscribe(ctx)
	if err != nil {
		return false
	}
	t.poke()
	for {
		ev, err := events.Next(ctx)
		if err != nil {
			break
		}
		t.event(ev)
	}
	if ctx.Err() == nil {
		t.poke()
	}
	return true
}

// event handles one core event.
func (t *terminals) event(ev herdr.Event) {
	switch ev.Type {
	case "size":
		var size herdr.Size
		if json.Unmarshal(ev.Data, &size) == nil {
			t.resized(ev.Terminal, size)
		}
	case "created", "restarted", "removed", "finished", "lost", "status", "title", "labels", "foreground", "missed":
		t.poke()
	}
}

// list is TermList.
func (t *terminals) list(ctx context.Context) *TermList {
	out := &TermList{Core: "ok", Terminals: []TermInfo{}}
	var terms []herdr.Terminal
	err := t.call(ctx, func(c *herdr.Client) (err error) {
		terms, err = c.List(ctx)
		return err
	})
	if err != nil {
		out.Core, out.Message = "down", coreMessage(err)
		return out
	}
	out.Terminals = termInfos(terms)
	return out
}

// coreMessage says why there are no terminals to show.
func coreMessage(err error) string {
	switch {
	case errors.Is(err, errCoreOutdated):
		return "Gwen's terminal server is out of date. Run make term."
	case errors.Is(err, errCoreDown):
		return "Gwen's terminal server isn't running. Starting a terminal starts it."
	}
	return "Gwen's terminal server isn't answering."
}

// termInfos are the live terminals on this laptop, by address.
func termInfos(terms []herdr.Terminal) []TermInfo {
	out := []TermInfo{}
	for _, t := range terms {
		if t.State == "live" && t.Local() {
			out = append(out, termInfo(t))
		}
	}
	sort.Slice(out, func(i, j int) bool { return addressLess(out[i].Address, out[j].Address) })
	return out
}

// addressLess orders t2 before t10.
func addressLess(a, b string) bool {
	x, errX := strconv.Atoi(strings.TrimPrefix(a, "t"))
	y, errY := strconv.Atoi(strings.TrimPrefix(b, "t"))
	if errX == nil && errY == nil && x != y {
		return x < y
	}
	return a < b
}

// termInfo is a core terminal as the page shows it.
func termInfo(t herdr.Terminal) TermInfo {
	info := TermInfo{Address: t.Address, Name: remote.TerminalName(t), Cwd: t.Cwd, Claude: remote.IsClaude(t), Parent: t.Parent,
		Cols: t.Cols, Rows: t.Rows, Live: t.Live(),
		SessionID: t.Labels[remote.SessionLabel], Project: t.Labels[projectLabel], Task: t.Labels[taskLabel]}
	if fg := t.Foreground; fg != nil && fg.Cwd != "" {
		info.Cwd = fg.Cwd
	}
	st := remote.ClaudeStatus(t)
	if st == nil {
		st = t.Status
	}
	if st != nil {
		info.Status, info.App = st.State, st.App
	}
	return info
}

// terminalName is herdr-core's rule for a terminal name.
var terminalName = regexp.MustCompile(`^[a-z0-9_.-]{1,32}$`)

// runParams checks a start request and turns it into terminal.run's
// parameters; shell is the login shell ($SHELL).
func runParams(req TermStartRequest, shell string) (herdr.RunParams, error) {
	cwd := strings.TrimSpace(req.Cwd)
	if !filepath.IsAbs(cwd) {
		return herdr.RunParams{}, invalid("Choose a folder to start the terminal in.")
	}
	cwd = filepath.Clean(cwd)
	if st, err := os.Stat(cwd); err != nil || !st.IsDir() {
		return herdr.RunParams{}, invalid(fmt.Sprintf("The folder %s isn't there.", cwd))
	}
	if req.Name != "" && !terminalName.MatchString(req.Name) {
		return herdr.RunParams{}, invalid("A terminal's name is 1 to 32 small letters, digits, dots, dashes, or underscores.")
	}
	argv := req.Argv
	if len(argv) == 0 || argv[0] == "" {
		if shell == "" {
			shell = "/bin/bash"
		}
		argv = []string{shell}
	}
	p := herdr.RunParams{Argv: argv, Cwd: cwd, Name: req.Name, Keep: true, NoParent: true}
	if req.Project != "" {
		p.Labels = map[string]string{projectLabel: req.Project}
	}
	return p, nil
}

func invalid(msg string) error {
	return &hostError{Code: wire.CodeInvalidRequest, Message: msg, Details: map[string]any{}}
}

// run is TermStart.
func (t *terminals) run(ctx context.Context, req TermStartRequest) (*TermInfo, error) {
	p, err := runParams(req, os.Getenv("SHELL"))
	if err != nil {
		return nil, err
	}
	c, err := t.conn(ctx)
	if errors.Is(err, errCoreDown) {
		c, err = t.startCore(ctx)
	}
	if err != nil {
		return nil, err
	}
	rec, err := c.Run(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("start a terminal: %w", err)
	}
	info := termInfo(rec)
	return &info, nil
}

// startCore starts Gwen's core the way gwend does and waits for it to answer.
func (t *terminals) startCore(ctx context.Context) (*herdr.Client, error) {
	t.startMu.Lock()
	defer t.startMu.Unlock()
	// Someone else may have started it while this call waited.
	if c, err := t.conn(ctx); !errors.Is(err, errCoreDown) {
		return c, err
	}
	startErr := t.start(ctx)
	if errors.Is(startErr, remote.ErrNoCore) {
		return nil, startErr
	}
	deadline := t.clk.Now().Add(coreStartWait)
	for {
		timer := t.clk.NewTimer(corePoll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("start herdr-core: %w", ctx.Err())
		case <-timer.C():
		}
		c, err := t.conn(ctx)
		if !errors.Is(err, errCoreDown) {
			return c, err
		}
		if !t.clk.Now().Before(deadline) {
			if startErr != nil {
				return nil, fmt.Errorf("%w: %w", errCoreNoStart, startErr)
			}
			return nil, errCoreNoStart
		}
	}
}

// label is TermLabel.
func (t *terminals) label(ctx context.Context, address string, labels map[string]string) error {
	if address == "" || len(labels) == 0 {
		return invalid("Say which terminal and which labels.")
	}
	if err := t.call(ctx, func(c *herdr.Client) error { return c.Label(ctx, address, labels) }); err != nil {
		return fmt.Errorf("label %s: %w", address, err)
	}
	return nil
}

// end is TermEnd: terminal.kill, which ends a kept terminal for good.
func (t *terminals) end(ctx context.Context, address string) error {
	if address == "" {
		return invalid("Say which terminal to end.")
	}
	var rec herdr.Terminal
	err := t.call(ctx, func(c *herdr.Client) (err error) {
		rec, err = c.Get(ctx, address)
		return err
	})
	if err != nil {
		return fmt.Errorf("end %s: %w", address, err)
	}
	if remote.IsClaude(rec) || rec.Labels[remote.SessionLabel] != "" {
		return &hostError{Code: wire.CodeConflict, Message: "A Claude session runs in this terminal. End the session instead.", Details: map[string]any{}}
	}
	if err := t.call(ctx, func(c *herdr.Client) error {
		return c.Call(ctx, "terminal.kill", map[string]any{"terminal": address}, nil)
	}); err != nil {
		return fmt.Errorf("end %s: %w", address, err)
	}
	return nil
}

// termView is one open view of a terminal.
type termView struct {
	id       string
	terminal string
	s        stream
	cancel   context.CancelFunc

	ready     chan struct{} // closed once the page listens
	readyOnce sync.Once

	mu        sync.Mutex
	announced herdr.Size // the grid this view last told the core
	shown     herdr.Size // the grid the page draws at
	detached  bool
}

// listen lets output through: the page is listening to the view's events.
func (v *termView) listen() { v.readyOnce.Do(func() { close(v.ready) }) }

func validGrid(cols, rows int) bool { return cols > 0 && rows > 0 && cols <= 0xFFFF && rows <= 0xFFFF }

// attach is TermAttach.
func (t *terminals) attach(ctx context.Context, address string, cols, rows int) (string, error) {
	if address == "" || !validGrid(cols, rows) {
		return "", invalid("Say which terminal, and a grid of at least one row and column.")
	}
	var out *herdr.Attached
	err := t.call(ctx, func(c *herdr.Client) (err error) {
		out, err = c.Attach(ctx, herdr.AttachParams{Terminal: address, History: viewHistory, Cols: cols, Rows: rows})
		return err
	})
	if err != nil {
		return "", fmt.Errorf("attach %s: %w", address, err)
	}
	addr := out.Terminal.Address
	if addr == "" {
		addr = address
	}
	return t.open(addr, out, herdr.Size{Cols: cols, Rows: rows}), nil
}

// open registers a view of s and starts passing its output on.
func (t *terminals) open(terminal string, s stream, size herdr.Size) string {
	pumpCtx, cancel := context.WithCancel(context.Background())
	t.mu.Lock()
	t.nextID++
	id := "v" + strconv.FormatUint(t.nextID, 10)
	v := &termView{id: id, terminal: terminal, s: s, cancel: cancel, ready: make(chan struct{}), announced: size, shown: size}
	t.views[id] = v
	t.mu.Unlock()
	go func() {
		defer cancel()
		pump(pumpCtx, v, t.emit, func() {
			t.mu.Lock()
			if t.views[id] == v {
				delete(t.views, id)
			}
			t.mu.Unlock()
		})
	}()
	return id
}

func (t *terminals) view(id string) (*termView, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	v := t.views[id]
	if v == nil {
		return nil, errNoView
	}
	return v, nil
}

// input is TermInput.
func (t *terminals) input(id, b64 string) error {
	v, err := t.view(id)
	if err != nil {
		return err
	}
	b, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return invalid("Typed text must be base64.")
	}
	v.listen()
	if len(b) == 0 {
		return nil
	}
	if err := v.s.Write(b); err != nil {
		return fmt.Errorf("type into %s: %w", v.terminal, err)
	}
	return nil
}

// resize is TermResize.
func (t *terminals) resize(id string, cols, rows int) error {
	if !validGrid(cols, rows) {
		return invalid("A terminal has at least one row and column.")
	}
	v, err := t.view(id)
	if err != nil {
		return err
	}
	size := herdr.Size{Cols: cols, Rows: rows}
	v.mu.Lock()
	v.announced, v.shown = size, size
	v.mu.Unlock()
	v.listen()
	if err := v.s.Resize(cols, rows); err != nil {
		return fmt.Errorf("resize %s: %w", v.terminal, err)
	}
	return nil
}

// detach is TermDetach.
func (t *terminals) detach(id string) {
	t.mu.Lock()
	v := t.views[id]
	delete(t.views, id)
	t.mu.Unlock()
	if v == nil {
		return
	}
	v.mu.Lock()
	v.detached = true
	v.mu.Unlock()
	v.cancel()
	v.s.Close()
}

// resized tells the views of a terminal its grid changed. The page draws at
// the terminal's real grid: another viewer's while that one is active, its
// own again once it types or resizes.
func (t *terminals) resized(terminal string, size herdr.Size) {
	if !validGrid(size.Cols, size.Rows) {
		return
	}
	t.mu.Lock()
	var views []*termView
	for _, v := range t.views {
		if v.terminal == terminal {
			views = append(views, v)
		}
	}
	t.mu.Unlock()
	for _, v := range views {
		select {
		case <-v.ready:
		default:
			continue // the page draws at the size it attached with
		}
		v.mu.Lock()
		changed := v.shown != size && !v.detached
		v.shown = size
		v.mu.Unlock()
		if changed {
			t.emit(eventTermSize+v.id, size)
		}
	}
}

// termClosed is term:closed's payload.
type termClosed struct {
	Reason string `json:"reason"`
}

func isData(f herdr.Frame) bool { return f.Resize == nil && f.Closed == nil }

// pump passes a view's output to the page once it listens, in order: bytes
// as term:data, merging what has already arrived up to maxBatch, then
// returning credit for exactly the bytes passed on; a grid as term:size; and
// the end as term:closed. It returns when the view ends or ctx does, and
// calls forget before the page hears that the view closed.
func pump(ctx context.Context, v *termView, emit func(name string, data ...any), forget func()) {
	defer forget()
	select {
	case <-v.ready:
	case <-ctx.Done():
		return
	}
	// A done context makes Next return only what has already arrived.
	arrived, stop := context.WithCancel(context.Background())
	stop()
	var held *herdr.Frame
	for {
		var f herdr.Frame
		if held != nil {
			f, held = *held, nil
		} else {
			var err error
			if f, err = v.s.Next(ctx); err != nil {
				if ctx.Err() == nil {
					forget()
					emit(eventTermClosed+v.id, termClosed{Reason: "lost"})
				}
				return
			}
		}
		switch {
		case f.Closed != nil:
			v.mu.Lock()
			detached := v.detached
			v.mu.Unlock()
			forget()
			if !detached {
				emit(eventTermClosed+v.id, termClosed{Reason: f.Closed.Reason})
			}
			return
		case f.Resize != nil:
			v.mu.Lock()
			v.shown = *f.Resize
			v.mu.Unlock()
			emit(eventTermSize+v.id, *f.Resize)
		default:
			data := f.Data
			for len(data) < maxBatch {
				g, err := v.s.Next(arrived)
				if err != nil {
					break
				}
				if !isData(g) {
					held = &g
					break
				}
				data = append(data, g.Data...)
			}
			if len(data) == 0 {
				continue
			}
			emit(eventTermData+v.id, base64.StdEncoding.EncodeToString(data))
			v.s.Credit(len(data))
		}
	}
}

// termError is a failure as the page reads it: a code and a sentence.
func termError(err error) error {
	if err == nil {
		return nil
	}
	var he *hostError
	if errors.As(err, &he) {
		return he
	}
	fail := func(code, msg string) error { return &hostError{Code: code, Message: msg, Details: map[string]any{}} }
	switch {
	case errors.Is(err, remote.ErrNoCore):
		return fail(wire.CodeUnavailable, "Gwen's terminal server isn't installed. Run make term.")
	case errors.Is(err, errCoreOutdated), errors.Is(err, errCoreDown):
		return fail(wire.CodeUnavailable, coreMessage(err))
	case errors.Is(err, errCoreNoStart):
		return fail(wire.CodeUnavailable, "Gwen's terminal server didn't start.")
	case errors.Is(err, errNoView):
		return fail(wire.CodeNotFound, "That terminal view is closed.")
	case errors.Is(err, herdr.ErrClosed):
		return fail(wire.CodeUnavailable, "Lost the terminal server.")
	case herdr.IsKind(err, "not_found"), herdr.IsKind(err, "finished"):
		return fail(wire.CodeNotFound, "That terminal has ended.")
	case herdr.IsKind(err, "name_taken"):
		return fail(wire.CodeConflict, "Another terminal already has that name.")
	case herdr.IsKind(err, "unavailable"):
		return fail(wire.CodeUnavailable, "That terminal is restarting. Try again in a moment.")
	case herdr.IsKind(err, "spawn_failed"):
		return fail(wire.CodeInvalidRequest, "That program couldn't start.")
	}
	var ce *herdr.Error
	if errors.As(err, &ce) {
		return fail(wire.CodeConflict, ce.Message)
	}
	return fail(wire.CodeInternal, err.Error())
}
