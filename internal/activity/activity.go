// Package activity reports keyboard and pointer idleness and session
// lifecycle events: the ActivityMonitor port of docs/02-architecture.md. The
// signal is input-or-no-input only; nothing about what the user types or
// which app is focused is ever read.
package activity

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/kzark/gwen/internal/clock"
)

// EventKind is what happened.
type EventKind int

// The event kinds.
const (
	Idle EventKind = iota + 1
	Active
	Locked
	Unlocked
	Suspend
	Resume
)

func (k EventKind) String() string {
	switch k {
	case Idle:
		return "idle"
	case Active:
		return "active"
	case Locked:
		return "locked"
	case Unlocked:
		return "unlocked"
	case Suspend:
		return "suspend"
	case Resume:
		return "resume"
	}
	return "unknown"
}

// Event is one activity or session event.
type Event struct {
	Kind      EventKind
	Threshold time.Duration // set on Idle: the threshold that fired
	At        time.Time
	// Ack is set on Suspend. The daemon calls it once the suspend is
	// committed, which releases the sleep inhibitor. It is safe to call more
	// than once.
	Ack func()
}

// ActivityMonitor emits activity and session events.
type ActivityMonitor interface {
	// Start emits events until ctx is cancelled, then closes the channel.
	// thresholds are the idle durations to report, ascending. Start may be
	// called again once the previous run's context is cancelled.
	Start(ctx context.Context, thresholds []time.Duration) (<-chan Event, error)
	Name() string
}

// Backend names, the activity_backend values of docs/04-api-contract.md#status.
const (
	NameWayland         = "wayland"
	NameSwayidle        = "swayidle"
	NameDBusScreensaver = "dbus_screensaver"
	NameXprintidle      = "xprintidle"
	NameNone            = "none"
)

// idleBackend reports Idle and Active.
type idleBackend interface {
	name() string
	// run emits events through emit until ctx is done.
	run(ctx context.Context, thresholds []time.Duration, emit func(Event)) error
}

// prober tries to initialise a backend.
type prober struct {
	name  string
	probe func(ctx context.Context) (idleBackend, error)
}

// sessionSource reports lock, unlock, suspend, and resume.
type sessionSource interface {
	run(ctx context.Context, emit func(Event))
}

// New probes the idle backends in the order of docs/02-architecture.md —
// Wayland, swayidle, the D-Bus screensaver, xprintidle — and returns a monitor
// on the first that initialises, merged with the login1 session events. With
// no idle backend it returns the none monitor, which emits only login1 events,
// so tracking goes on without automatic breaks.
func New(ctx context.Context, clk clock.Clock) ActivityMonitor {
	var session sessionSource
	if l, err := newLogin1(clk); err != nil {
		slog.Warn("session events unavailable", "err", err)
	} else {
		session = l
	}
	return newMonitor(ctx, clk, defaultProbers(clk), session)
}

func defaultProbers(clk clock.Clock) []prober {
	return []prober{
		{NameWayland, func(ctx context.Context) (idleBackend, error) { return probeWayland(ctx, clk) }},
		{NameSwayidle, func(ctx context.Context) (idleBackend, error) { return probeSwayidle(ctx, clk, execRunner{}) }},
		{NameDBusScreensaver, func(ctx context.Context) (idleBackend, error) { return probeScreensaver(ctx, clk) }},
		{NameXprintidle, func(ctx context.Context) (idleBackend, error) { return probeXprintidle(ctx, clk, execRunner{}) }},
	}
}

func newMonitor(ctx context.Context, clk clock.Clock, probers []prober, session sessionSource) *monitor {
	m := &monitor{clk: clk, session: session}
	for _, p := range probers {
		b, err := p.probe(ctx)
		if err != nil {
			slog.Debug("idle backend unavailable", "backend", p.name, "err", err)
			continue
		}
		m.idle = b
		slog.Info("idle backend selected", "backend", b.name())
		return m
	}
	slog.Warn("no idle backend: tracking continues without automatic breaks")
	return m
}

// monitor merges an idle backend (nil for none) with the session events.
type monitor struct {
	clk     clock.Clock
	idle    idleBackend
	session sessionSource
}

func (m *monitor) Name() string {
	if m.idle == nil {
		return NameNone
	}
	return m.idle.name()
}

func (m *monitor) Start(ctx context.Context, thresholds []time.Duration) (<-chan Event, error) {
	out := make(chan Event, 16)
	emit := func(ev Event) {
		select {
		case out <- ev:
		case <-ctx.Done():
		}
	}
	var wg sync.WaitGroup
	if m.idle != nil {
		wg.Go(func() {
			if err := m.idle.run(ctx, thresholds, emit); err != nil && ctx.Err() == nil {
				slog.Warn("idle backend stopped", "backend", m.idle.name(), "err", err)
			}
		})
	}
	if m.session != nil {
		wg.Go(func() { m.session.run(ctx, emit) })
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out, nil
}

// tracker turns per-threshold idle and resume notifications into events: one
// Idle per threshold per idle period, and one Active on the first input after
// any Idle (docs/05-time-engine.md#what-the-engine-requires-of-the-activity-monitor).
type tracker struct {
	mu       sync.Mutex
	clk      clock.Clock
	emit     func(Event)
	idleSeen bool
}

func (t *tracker) idled(threshold time.Duration) {
	t.mu.Lock()
	t.idleSeen = true
	t.mu.Unlock()
	t.emit(Event{Kind: Idle, Threshold: threshold, At: t.clk.Now()})
}

func (t *tracker) resumed() {
	t.mu.Lock()
	send := t.idleSeen
	t.idleSeen = false
	t.mu.Unlock()
	if send {
		t.emit(Event{Kind: Active, At: t.clk.Now()})
	}
}
