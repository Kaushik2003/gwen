package remote

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/herdr"
	"github.com/kzark/gwen/internal/herdr/herdrtest"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// w27dTimeout bounds every wait in these tests. It only guards against a
// hang: nothing here waits for time to pass.
const w27dTimeout = 10 * time.Second

// w27dNoClaude is the pid given to a fake Claude in a terminal's foreground:
// larger than any real pid, so nothing could ever signal a real process.
const w27dNoClaude = 1 << 30

// w27dEvents records what the Service publishes and wakes the tests that
// wait for it to change.
type w27dEvents struct {
	mu      sync.Mutex
	changed chan struct{}
	removed []string
}

func (e *w27dEvents) publish(name string, data any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if r, ok := data.(wire.CodeSessionRemoved); ok && name == wire.EventCodeSessionRemoved {
		e.removed = append(e.removed, r.ID)
	}
	close(e.changed)
	e.changed = make(chan struct{})
}

// wait blocks until cond holds, checking again after every publish. cond
// runs without e.mu, so it may call the Service.
func (e *w27dEvents) wait(t *testing.T, what string, cond func() bool) {
	t.Helper()
	timer := time.NewTimer(w27dTimeout)
	defer timer.Stop()
	for {
		e.mu.Lock()
		changed := e.changed
		e.mu.Unlock()
		if cond() {
			return
		}
		select {
		case <-changed:
		case <-timer.C:
			t.Fatalf("gave up after %v waiting for %s", w27dTimeout, what)
		}
	}
}

func (e *w27dEvents) removedIDs() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string{}, e.removed...)
}

// w27dLaunch is one program the Service started through systemd-run.
type w27dLaunch struct {
	Unit string
	Argv []string
}

// w27dSetup is what a rig changes from its defaults.
type w27dSetup struct {
	core herdrtest.Options
	// gwenBin is the gwen CLI the Service knows; "" leaves it unset.
	gwenBin string
	// launchErr is what every Launch returns.
	launchErr error
	// seed are rows in code_sessions before the Service starts.
	seed []store.CodeSession
}

// w27dRig is a Service following a fake herdr-core, with a fake clock, a
// temporary home, and a recorder for what it publishes and launches.
type w27dRig struct {
	t      *testing.T
	core   *herdrtest.Core
	svc    *Service
	clk    *clock.Fake
	repos  store.Repos
	home   string
	events *w27dEvents
	away   atomic.Bool
	// eventFirst makes terminal.run reply only once the Service has applied
	// the new terminal's created event (see run).
	eventFirst atomic.Bool
	// docRead gets a signal each time the core is asked for a document.
	docRead chan struct{}

	mu       sync.Mutex
	launches []w27dLaunch
}

// w27dNewRig starts a Service on a fresh fake core. It stops when the test
// ends, before the core and the database close.
func w27dNewRig(t *testing.T, o w27dSetup) *w27dRig {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	clk := clock.NewFake(testutil.At("09:00:00"))
	db := testutil.NewDBWithClock(t, clk)
	core := herdrtest.New(t, o.core)
	r := &w27dRig{t: t, core: core, clk: clk, repos: store.NewRepos(db), home: home,
		events: &w27dEvents{changed: make(chan struct{})}, docRead: make(chan struct{}, 1)}
	r.eventFirst.Store(true)
	for _, row := range o.seed {
		require.NoError(t, r.repos.CodeSessions.Save(t.Context(), row))
	}

	cfg := config.Defaults()
	// Never ask logind to keep this machine awake, never push, and never
	// look for the real core: Dial goes to the fake one.
	cfg.Remote.KeepAwake = config.KeepAwakeNever
	cfg.Remote.Push = false
	cfg.Remote.CoreSocket = core.Socket()
	svc, err := New(Options{DB: db, Repos: r.repos, Clock: clk, Config: func() config.Config { return cfg },
		Publish: r.events.publish, Away: r.away.Load, Home: home, GwenBin: o.gwenBin,
		DataDir: filepath.Join(home, "data"),
		Launch: func(_ context.Context, unit string, argv []string) error {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.launches = append(r.launches, w27dLaunch{Unit: unit, Argv: append([]string{}, argv...)})
			return o.launchErr
		},
		Dial: func(ctx context.Context, _ string) (*herdr.Client, error) { return herdr.Dial(ctx, core.Socket()) },
	})
	require.NoError(t, err)
	r.svc = svc
	// Installed after r.svc is set: the core's lock orders the two.
	core.Handle("terminal.run", r.run)
	core.Handle("doc.get", func(call herdrtest.Call) (any, error) {
		select {
		case r.docRead <- struct{}{}:
		default:
		}
		return call.Builtin()
	})
	// A label is applied, and its event sent, by the time this wakes the
	// waiters (see claude).
	core.Handle("terminal.label", func(call herdrtest.Call) (any, error) {
		defer r.events.publish("", nil)
		return call.Builtin()
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(w27dTimeout):
			t.Errorf("the Service did not stop within %v", w27dTimeout)
		}
	})
	return r
}

// run is the rig's terminal.run. The terminal runs as this process, so
// hooks can be sent from "inside" it: the pid check walks /proc from the
// sender upwards. With eventFirst it replies only once the Service has
// applied the terminal's created event, the order in which Start keeps its
// card (see TestStartKeepsItsCardWhenTheReplyComesFirst for the other).
func (r *w27dRig) run(call herdrtest.Call) (any, error) {
	var p herdr.RunParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	rec := herdr.Terminal{Name: p.Name, Argv: p.Argv, Cwd: p.Cwd, Labels: p.Labels, Keep: p.Keep, PID: os.Getpid()}
	if p.Size != nil {
		rec.Cols, rec.Rows = p.Size.Cols, p.Size.Rows
	}
	rec = r.core.AddTerminal(rec)
	if r.eventFirst.Load() {
		// The document event after created makes the Service's loop read
		// the document, by which time it knows the terminal. This runs on
		// the core's goroutine, so it gives up quietly instead of failing.
		r.core.SetDoc("herdr-app", map[string]any{"spaces": []any{}})
		timer := time.NewTimer(w27dTimeout)
		defer timer.Stop()
		for !r.knows(rec.Address) {
			select {
			case <-r.docRead:
			case <-timer.C:
				return rec, nil
			}
		}
	}
	return rec, nil
}

// knows reports whether the Service has applied a terminal's created event.
func (r *w27dRig) knows(addr string) bool {
	r.svc.mu.Lock()
	defer r.svc.mu.Unlock()
	_, ok := r.svc.terms[addr]
	return ok
}

// w27dReady is a rig whose Service is connected to its core.
func w27dReady(t *testing.T) *w27dRig {
	t.Helper()
	r := w27dNewRig(t, w27dSetup{})
	r.waitCore("ok")
	return r
}

// waitCore waits until the Service says the core is ok, down or outdated.
func (r *w27dRig) waitCore(state string) {
	r.t.Helper()
	r.events.wait(r.t, "the core to be "+state, func() bool { return r.svc.Sessions().Core == state })
}

// sync returns once the Service has applied every event the core sent so
// far. Events are applied in order on one loop, and a change to the
// herdr-app document makes that loop read it back (doc.get), so when the
// read arrives everything before it is done.
func (r *w27dRig) sync() {
	r.t.Helper()
	n := len(r.core.Calls("doc.get"))
	r.core.SetDoc("herdr-app", map[string]any{"spaces": []any{}})
	r.core.WaitCalls("doc.get", n+1)
}

// dir makes a folder under the rig's home, where sessions may start.
func (r *w27dRig) dir(name string) string {
	r.t.Helper()
	dir := filepath.Join(r.home, "code", name)
	require.NoError(r.t, os.MkdirAll(dir, 0o755))
	return dir
}

// claude adds a terminal running Claude Code in dir, as this process so
// hooks from it count, and returns its address and its card.
func (r *w27dRig) claude(dir string) (addr, id string) {
	r.t.Helper()
	t1 := r.core.AddTerminal(herdr.Terminal{Cwd: dir, PID: os.Getpid(), Argv: []string{"bash"},
		Foreground: &herdr.Foreground{PID: w27dNoClaude, Name: "claude", Argv: []string{"/home/u/.local/bin/claude"}, Cwd: dir}})
	r.sync()
	card := r.cardAt(t1.Address)
	require.NotNil(r.t, card, "Claude in %s gets a card", t1.Address)
	// The card's label comes back from the core as a labels event, which
	// makes the Service look at the terminal again: let that settle, so it
	// never lands in the middle of a test.
	r.events.wait(r.t, "the session label on "+t1.Address, func() bool {
		rec, _ := r.core.Terminal(t1.Address)
		return rec.Labels[SessionLabel] == card.ID
	})
	r.sync()
	return t1.Address, card.ID
}

// status is a status Claude Code reports through herdr-core (OSC 7501).
func (r *w27dRig) status(addr, state string) {
	r.t.Helper()
	r.core.SetStatus(addr, &herdr.Status{State: state, App: "claude-code"})
	r.sync()
}

// hook sends a hook event from inside terminal addr.
func (r *w27dRig) hook(addr string, ev wire.CodeHookEvent) wire.CodeHookReply {
	ev.Terminal = addr
	return r.svc.HandleHook(r.t.Context(), ev, os.Getpid())
}

// card is one card as the API shows it.
func (r *w27dRig) card(id string) wire.CodeSession {
	r.t.Helper()
	card, err := r.svc.Session(id)
	require.NoError(r.t, err)
	return card
}

// cardAt is the card of terminal addr, nil when it has none.
func (r *w27dRig) cardAt(addr string) *wire.CodeSession {
	for _, card := range r.svc.Sessions().Sessions {
		if card.Terminal == addr {
			return &card
		}
	}
	return nil
}

// cardsOf are the listed cards of one Claude conversation.
func (r *w27dRig) cardsOf(claudeID string) []wire.CodeSession {
	var out []wire.CodeSession
	for _, card := range r.svc.Sessions().Sessions {
		if card.ClaudeSessionID == claudeID {
			out = append(out, card)
		}
	}
	return out
}

// runs are the terminal.run calls the core received.
func (r *w27dRig) runs() []herdr.RunParams {
	r.t.Helper()
	var out []herdr.RunParams
	for _, call := range r.core.Calls("terminal.run") {
		var p herdr.RunParams
		require.NoError(r.t, call.Decode(&p))
		out = append(out, p)
	}
	return out
}

func (r *w27dRig) launched() []w27dLaunch {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]w27dLaunch{}, r.launches...)
}

// w27dCode is the wire error code of a Service error, "" for another error.
func w27dCode(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// w27dArmed waits until n fake timers are armed: the goroutine under test
// has reached the point where it waits on the clock.
func w27dArmed(t *testing.T, clk *clock.Fake, n int) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		clk.BlockUntil(n)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(w27dTimeout):
		t.Fatalf("gave up after %v waiting for %d armed timers", w27dTimeout, n)
	}
}

// w27dRecv takes one value from ch.
func w27dRecv[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(w27dTimeout):
		t.Fatalf("gave up after %v waiting for %s", w27dTimeout, what)
	}
	var zero T
	return zero
}

func w27dPtr[T any](v T) *T { return &v }
