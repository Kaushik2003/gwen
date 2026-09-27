package activity

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/stretchr/testify/require"
)

var t0 = testutil.T0

// collect drains a monitor channel until it closes or n events arrive.
func collect(t *testing.T, ch <-chan Event, n int) []Event {
	t.Helper()
	var out []Event
	for len(out) < n {
		select {
		case ev, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out after %d of %d events", len(out), n)
		}
	}
	return out
}

func kinds(evs []Event) []string {
	var out []string
	for _, e := range evs {
		s := e.Kind.String()
		if e.Kind == Idle {
			s += " " + e.Threshold.String()
		}
		out = append(out, s)
	}
	return out
}

// fakeBackend emits a script of events and then waits for cancellation.
type fakeBackend struct {
	id     string
	script []Event
	starts int
	mu     sync.Mutex
}

func (f *fakeBackend) name() string { return f.id }

func (f *fakeBackend) run(ctx context.Context, _ []time.Duration, emit func(Event)) error {
	f.mu.Lock()
	f.starts++
	f.mu.Unlock()
	for _, e := range f.script {
		emit(e)
	}
	<-ctx.Done()
	return nil
}

func TestNewProbesInOrder(t *testing.T) {
	t.Parallel()
	var tried []string
	fail := func(name string) prober {
		return prober{name, func(context.Context) (idleBackend, error) {
			tried = append(tried, name)
			return nil, errors.New("unavailable")
		}}
	}
	ok := func(name string) prober {
		return prober{name, func(context.Context) (idleBackend, error) {
			tried = append(tried, name)
			return &fakeBackend{id: name}, nil
		}}
	}
	ctx := context.Background()
	clk := clock.NewFake(t0)

	m := newMonitor(ctx, clk, []prober{fail(NameWayland), ok(NameSwayidle), ok(NameDBusScreensaver)}, nil)
	require.Equal(t, NameSwayidle, m.Name())
	require.Equal(t, []string{NameWayland, NameSwayidle}, tried, "the first that initialises wins")

	tried = nil
	m = newMonitor(ctx, clk, []prober{fail(NameWayland), fail(NameSwayidle), fail(NameDBusScreensaver), fail(NameXprintidle)}, nil)
	require.Equal(t, NameNone, m.Name())
	require.Equal(t, []string{NameWayland, NameSwayidle, NameDBusScreensaver, NameXprintidle}, tried)

	require.Equal(t, []string{NameWayland, NameSwayidle, NameDBusScreensaver, NameXprintidle},
		func() []string {
			var names []string
			for _, p := range defaultProbers(clk) {
				names = append(names, p.name)
			}
			return names
		}(), "the documented probe order")
}

type fakeSession struct{ events []Event }

func (f fakeSession) run(ctx context.Context, emit func(Event)) {
	for _, e := range f.events {
		emit(e)
	}
	<-ctx.Done()
}

func TestMonitorMergesAndRestarts(t *testing.T) {
	t.Parallel()
	b := &fakeBackend{id: "fake", script: []Event{{Kind: Idle, Threshold: 5 * time.Second, At: t0}}}
	m := &monitor{clk: clock.NewFake(t0), idle: b, session: fakeSession{events: []Event{{Kind: Locked, At: t0}}}}

	for range 2 { // Start again after cancelling, as on a config change
		ctx, cancel := context.WithCancel(context.Background())
		ch, err := m.Start(ctx, []time.Duration{5 * time.Second})
		require.NoError(t, err)
		got := collect(t, ch, 2)
		require.ElementsMatch(t, []string{"idle 5s", "locked"}, kinds(got))
		cancel()
		_, open := <-ch
		for open {
			_, open = <-ch
		}
	}
	require.Equal(t, 2, b.starts)

	none := &monitor{clk: clock.NewFake(t0), session: fakeSession{events: []Event{{Kind: Suspend, At: t0}}}}
	require.Equal(t, NameNone, none.Name())
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := none.Start(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"suspend"}, kinds(collect(t, ch, 1)), "none still reports login1 events")
	cancel()
}

func TestTracker(t *testing.T) {
	t.Parallel()
	clk := clock.NewFake(t0)
	var got []Event
	tr := &tracker{clk: clk, emit: func(e Event) { got = append(got, e) }}
	tr.resumed() // no Idle yet: not an Active
	tr.idled(5 * time.Second)
	clk.Advance(time.Minute)
	tr.idled(3 * time.Minute)
	clk.Advance(time.Second)
	tr.resumed()
	tr.resumed() // every notification object resumes; only the first counts
	require.Equal(t, []string{"idle 5s", "idle 3m0s", "active"}, kinds(got))
	require.Equal(t, t0, got[0].At)
	require.Equal(t, t0.Add(61*time.Second), got[2].At)
}

func TestSwayidleArgsAndParse(t *testing.T) {
	t.Parallel()
	require.Equal(t, []string{"-w",
		"timeout", "5", "echo idle 5000", "resume", "echo active 5000",
		"timeout", "181", "echo idle 180500", "resume", "echo active 180500",
	}, swayidleArgs([]time.Duration{5 * time.Second, 180500 * time.Millisecond}))

	tests := []struct {
		line string
		idle bool
		th   time.Duration
		ok   bool
	}{
		{"idle 5000", true, 5 * time.Second, true},
		{"active 600000", false, 10 * time.Minute, true},
		{"  idle   180000 ", true, 3 * time.Minute, true},
		{"idle", false, 0, false},
		{"idle five", false, 0, false},
		{"idle 0", false, 0, false},
		{"sleep 5000", false, 0, false},
		{"", false, 0, false},
	}
	for _, tc := range tests {
		idle, th, ok := parseSwayidle(tc.line)
		require.Equal(t, tc.ok, ok, tc.line)
		require.Equal(t, tc.idle, idle, tc.line)
		require.Equal(t, tc.th, th, tc.line)
	}
}

// fakeRunner scripts external programs.
type fakeRunner struct {
	missing bool
	outputs []string // successive Output results
	outErr  error
	stream  string
	calls   int
	mu      sync.Mutex
}

func (f *fakeRunner) LookPath(string) error {
	if f.missing {
		return errors.New("not found")
	}
	return nil
}

func (f *fakeRunner) Output(context.Context, string, ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.outErr != nil {
		return nil, f.outErr
	}
	out := f.outputs[min(f.calls, len(f.outputs)-1)]
	f.calls++
	return []byte(out), nil
}

func (f *fakeRunner) Stream(context.Context, string, ...string) (io.ReadCloser, error) {
	if f.outErr != nil {
		return nil, f.outErr
	}
	return io.NopCloser(strings.NewReader(f.stream)), nil
}

func TestSwayidleBackend(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	clk := clock.NewFake(t0)
	_, err := probeSwayidle(context.Background(), clk, &fakeRunner{missing: true})
	require.Error(t, err)

	r := &fakeRunner{stream: "idle 5000\nidle 180000\nnoise\nactive 5000\nactive 180000\n"}
	b, err := probeSwayidle(context.Background(), clk, r)
	require.NoError(t, err)
	require.Equal(t, NameSwayidle, b.name())
	var got []Event
	err = b.run(context.Background(), []time.Duration{5 * time.Second, 3 * time.Minute}, func(e Event) { got = append(got, e) })
	require.ErrorContains(t, err, "swayidle exited")
	require.Equal(t, []string{"idle 5s", "idle 3m0s", "active"}, kinds(got))

	err = swayidleProc{clk: clk, exec: &fakeRunner{outErr: errors.New("boom")}}.run(context.Background(), nil, func(Event) {})
	require.ErrorContains(t, err, "start swayidle")

	t.Setenv("WAYLAND_DISPLAY", "")
	_, err = probeSwayidle(context.Background(), clk, r)
	require.Error(t, err)
}

func TestXprintidleParse(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]time.Duration{"0\n": 0, "12345\n": 12345 * time.Millisecond, " 7 ": 7 * time.Millisecond} {
		got, err := parseXprintidle(in)
		require.NoError(t, err, in)
		require.Equal(t, want, got)
	}
	for _, bad := range []string{"", "abc", "-5", "1.5"} {
		_, err := parseXprintidle(bad)
		require.Error(t, err, bad)
	}
}

func TestPollerComputesAtFromObservedIdle(t *testing.T) {
	t.Parallel()
	p := newPoller([]time.Duration{5 * time.Second, 3 * time.Minute, 10 * time.Minute})
	at := func(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }

	require.Empty(t, p.observe(2*time.Second, at(10)), "input 8 s in, 2 s idle")
	got := p.observe(12*time.Second, at(20))
	require.Equal(t, []Event{{Kind: Idle, Threshold: 5 * time.Second, At: at(13)}}, got,
		"At is last input + threshold, not the poll time")
	require.Empty(t, p.observe(22*time.Second, at(30)))
	got = p.observe(200*time.Second, at(208))
	require.Equal(t, []Event{{Kind: Idle, Threshold: 3 * time.Minute, At: at(8 + 180)}}, got)

	got = p.observe(time.Second, at(220))
	require.Equal(t, []Event{{Kind: Active, At: at(219)}}, got, "input since the last sample starts a new period")
	got = p.observe(700*time.Second, at(919))
	require.Equal(t, []Event{
		{Kind: Idle, Threshold: 5 * time.Second, At: at(224)},
		{Kind: Idle, Threshold: 3 * time.Minute, At: at(399)},
		{Kind: Idle, Threshold: 10 * time.Minute, At: at(819)},
	}, got, "a long gap fires every threshold with its own instant")

	// Input between samples is caught even when the idle time grew.
	q := newPoller([]time.Duration{5 * time.Second})
	require.Len(t, q.observe(6*time.Second, at(10)), 1)
	require.Equal(t, []Event{{Kind: Active, At: at(12)}, {Kind: Idle, Threshold: 5 * time.Second, At: at(17)}},
		q.observe(8*time.Second, at(20)))
}

func TestXprintidlePolls(t *testing.T) {
	t.Setenv("DISPLAY", ":0")
	clk := clock.NewFake(t0)
	r := &fakeRunner{outputs: []string{"1000\n", "1000\n", "11000\n", "500\n"}}
	b, err := probeXprintidle(context.Background(), clk, r)
	require.NoError(t, err)
	require.Equal(t, NameXprintidle, b.name())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan Event, 8)
	done := make(chan error, 1)
	go func() {
		done <- b.run(ctx, []time.Duration{5 * time.Second}, func(e Event) { events <- e })
	}()
	clk.BlockUntil(1)
	clk.Advance(0) // first sample: 1 s idle
	clk.BlockUntil(1)
	clk.Advance(pollInterval) // 11 s idle at t0+10s: last input t0−1s
	ev := <-events
	require.Equal(t, Event{Kind: Idle, Threshold: 5 * time.Second, At: t0.Add(4 * time.Second)}, ev)
	clk.BlockUntil(1)
	clk.Advance(pollInterval) // 0.5 s idle: input
	ev = <-events
	require.Equal(t, Active, ev.Kind)
	require.Equal(t, t0.Add(19500*time.Millisecond), ev.At)
	cancel()
	require.NoError(t, <-done)

	t.Setenv("DISPLAY", "")
	_, err = probeXprintidle(context.Background(), clk, r)
	require.Error(t, err)
	t.Setenv("DISPLAY", ":0")
	_, err = probeXprintidle(context.Background(), clk, &fakeRunner{outErr: errors.New("no X")})
	require.Error(t, err)
}

type fakeIdleSource struct {
	idle []time.Duration
	err  error
	n    int
}

func (f *fakeIdleSource) IdleTime(context.Context) (time.Duration, error) {
	if f.err != nil {
		return 0, f.err
	}
	d := f.idle[min(f.n, len(f.idle)-1)]
	f.n++
	return d, nil
}

func TestScreensaverBackend(t *testing.T) {
	t.Parallel()
	clk := clock.NewFake(t0)
	_, err := probeScreensaverWith(context.Background(), clk, &fakeIdleSource{err: errors.New("NotSupported")})
	require.Error(t, err, "KDE on Wayland answers NotSupported")

	src := &fakeIdleSource{idle: []time.Duration{0, 6 * time.Second}}
	b, err := probeScreensaverWith(context.Background(), clk, src)
	require.NoError(t, err)
	require.Equal(t, NameDBusScreensaver, b.name())

	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan Event, 4)
	done := make(chan error, 1)
	go func() { done <- b.run(ctx, []time.Duration{5 * time.Second}, func(e Event) { events <- e }) }()
	clk.BlockUntil(1)
	clk.Advance(0)
	ev := <-events
	require.Equal(t, Event{Kind: Idle, Threshold: 5 * time.Second, At: t0.Add(-time.Second)}, ev)
	cancel()
	require.NoError(t, <-done)

	failing := dbusScreensaver{clk: clk, src: &fakeIdleSource{err: errors.New("bus gone")}}
	err = failing.run(context.Background(), nil, func(Event) {})
	require.ErrorContains(t, err, "bus gone")
}

// fakeLoginBus scripts login1.
type fakeLoginBus struct {
	mu        sync.Mutex
	session   dbus.ObjectPath
	locked    bool
	sigs      chan *dbus.Signal
	inhibits  int
	released  int
	inhibitOK bool
}

type fakeInhibitor struct{ bus *fakeLoginBus }

func (f fakeInhibitor) Close() error {
	f.bus.mu.Lock()
	defer f.bus.mu.Unlock()
	f.bus.released++
	return nil
}

func (f *fakeLoginBus) Session() (dbus.ObjectPath, error) { return f.session, nil }

func (f *fakeLoginBus) LockedHint(dbus.ObjectPath) (bool, error) { return f.locked, nil }

func (f *fakeLoginBus) Inhibit() (io.Closer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.inhibitOK {
		return nil, errors.New("denied")
	}
	f.inhibits++
	return fakeInhibitor{f}, nil
}

func (f *fakeLoginBus) Signals(dbus.ObjectPath) (<-chan *dbus.Signal, func(), error) {
	return f.sigs, func() {}, nil
}

func (f *fakeLoginBus) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.inhibits, f.released
}

const session = dbus.ObjectPath("/org/freedesktop/login1/session/_32")

func signal(name string, path dbus.ObjectPath, body ...any) *dbus.Signal {
	return &dbus.Signal{Name: name, Path: path, Body: body}
}

func lockedHint(v bool) *dbus.Signal {
	return signal(propertiesIface+".PropertiesChanged", session, sessionIface,
		map[string]dbus.Variant{"LockedHint": dbus.MakeVariant(v)}, []string{})
}

func TestLogin1Translation(t *testing.T) {
	t.Parallel()
	bus := &fakeLoginBus{session: session, locked: true, sigs: make(chan *dbus.Signal, 32), inhibitOK: true}
	clk := clock.NewFake(t0)
	l := &login1{clk: clk, bus: bus}
	events := make(chan Event, 32)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		l.run(ctx, func(e Event) { events <- e })
		close(done)
	}()

	require.Equal(t, Locked, (<-events).Kind, "a session locked at start is reported")
	for _, s := range []*dbus.Signal{
		lockedHint(true), // no change
		signal(sessionIface+".Unlock", session),
		lockedHint(false), // no change
		signal(sessionIface+".Lock", "/org/freedesktop/login1/session/other"), // another session
		signal(sessionIface+".Lock", session),
		lockedHint(false),
		signal(propertiesIface+".PropertiesChanged", session, "org.freedesktop.login1.Seat", map[string]dbus.Variant{}),
		signal(managerIface+".PrepareForSleep", login1Path, true),
	} {
		bus.sigs <- s
	}
	got := []Event{<-events, <-events, <-events, <-events}
	require.Equal(t, []string{"unlocked", "locked", "unlocked", "suspend"}, kinds(got))

	suspend := got[3]
	require.NotNil(t, suspend.Ack)
	inhibits, released := bus.counts()
	require.Equal(t, 1, inhibits)
	require.Zero(t, released, "the inhibitor is held until the suspend is acknowledged")
	suspend.Ack()
	suspend.Ack()
	_, released = bus.counts()
	require.Equal(t, 1, released, "acknowledged once, released once")

	bus.sigs <- signal(managerIface+".PrepareForSleep", login1Path, false)
	resume := <-events
	require.Equal(t, Resume, resume.Kind)
	require.Nil(t, resume.Ack)
	inhibits, _ = bus.counts()
	require.Equal(t, 2, inhibits, "the inhibitor is re-taken on resume")

	cancel()
	<-done
	_, released = bus.counts()
	require.Equal(t, 2, released, "stopping releases the inhibitor")
}

func TestLogin1WithoutInhibitor(t *testing.T) {
	t.Parallel()
	bus := &fakeLoginBus{session: session, sigs: make(chan *dbus.Signal, 4)}
	l := &login1{clk: clock.NewFake(t0), bus: bus}
	events := make(chan Event, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go l.run(ctx, func(e Event) { events <- e })
	bus.sigs <- signal(managerIface+".PrepareForSleep", login1Path, true)
	ev := <-events
	require.Equal(t, Suspend, ev.Kind)
	ev.Ack() // nothing to release, and no panic
	close(bus.sigs)
}

func TestEventKindString(t *testing.T) {
	t.Parallel()
	for k, s := range map[EventKind]string{Idle: "idle", Active: "active", Locked: "locked", Unlocked: "unlocked",
		Suspend: "suspend", Resume: "resume", EventKind(0): "unknown"} {
		require.Equal(t, s, k.String())
	}
}

func TestBindRequestEncodesTheStringLength(t *testing.T) {
	t.Parallel()
	buf := bindRequest(2, 21, "ext_idle_notifier_v1", 2, 5)
	le := func(b []byte) uint32 { return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24 }
	require.Len(t, buf, 8+4+4+24+4+4)
	require.Equal(t, uint32(2), le(buf[0:4]), "sender: the registry")
	require.Equal(t, uint32(len(buf))<<16, le(buf[4:8]), "size and opcode 0")
	require.Equal(t, uint32(21), le(buf[8:12]), "global name")
	require.Equal(t, uint32(21), le(buf[12:16]), "string length counts the NUL, not the padding")
	require.Equal(t, "ext_idle_notifier_v1\x00\x00\x00\x00", string(buf[16:40]))
	require.Equal(t, uint32(2), le(buf[40:44]), "version")
	require.Equal(t, uint32(5), le(buf[44:48]), "new id")
}
