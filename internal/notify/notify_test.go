package notify

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

const topic = "gwen-abcdefghijklmnopqrstuvwx"

type captured struct {
	method, path, body string
	header             http.Header
}

// ntfyServer records requests and answers with status.
func ntfyServer(t *testing.T, status int) (*httptest.Server, *[]captured) {
	t.Helper()
	var mu sync.Mutex
	var got []captured
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, captured{r.Method, r.URL.Path, string(b), r.Header.Clone()})
		mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func ntfyWith(t *testing.T, server, fallback string) (*Ntfy, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	p := NewNtfy(nil, config.CredentialsDir(t.TempDir()))
	p.log = slog.New(slog.NewTextHandler(&logs, nil))
	c := config.Defaults()
	c.Ntfy = config.Ntfy{Server: server, FallbackServer: fallback, Topic: topic}
	p.SetConfig(c)
	return p, &logs
}

var idleNudge = Notification{Kind: "idle", Title: "Still there?", Body: "No input for 3m. It stops counting as work at 10m.",
	Actions: []Action{{"back", "I'm back"}, {"break", "Start break"}, {"snooze", "Snooze 10m"}}}

func TestNtfyRequest(t *testing.T) {
	t.Parallel()
	srv, got := ntfyServer(t, http.StatusOK)
	p, _ := ntfyWith(t, srv.URL, "")
	require.NoError(t, p.Notify(context.Background(), idleNudge))
	require.Len(t, *got, 1)
	r := (*got)[0]
	require.Equal(t, http.MethodPost, r.method)
	require.Equal(t, "/"+topic, r.path)
	require.Equal(t, idleNudge.Body, r.body)
	require.Equal(t, "Still there?", r.header.Get("Title"))
	require.Equal(t, "4", r.header.Get("Priority"))
	require.Equal(t, "hourglass_flowing_sand", r.header.Get("Tags"))
	require.Empty(t, r.header.Get("Authorization"), "no token file, no header")

	require.NoError(t, p.Notify(context.Background(), Notification{Kind: "break_long", Title: "On a break for 15m"}))
	require.Equal(t, "coffee", (*got)[1].header.Get("Tags"))

	require.NoError(t, config.WriteCredential(p.credDir, config.CredNtfyToken, []byte("tk_secret\n")))
	require.NoError(t, p.Notify(context.Background(), idleNudge))
	require.Equal(t, "Bearer tk_secret", (*got)[2].header.Get("Authorization"), "read at the moment of use")
}

func TestNtfyFallback(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		primary     int // 0: a refused connection
		wantFallbck bool
		wantErr     bool
	}{
		{"5xx falls back", http.StatusBadGateway, true, false},
		{"refused connection falls back", 0, true, false},
		{"4xx does not", http.StatusForbidden, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			primary := "http://" + refusedAddr(t)
			if tc.primary != 0 {
				srv, _ := ntfyServer(t, tc.primary)
				primary = srv.URL
			}
			fallback, got := ntfyServer(t, http.StatusOK)
			p, logs := ntfyWith(t, primary, fallback.URL)
			err := p.Notify(context.Background(), idleNudge)
			if tc.wantErr {
				require.Error(t, err)
				require.NotContains(t, err.Error(), topic)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantFallbck, len(*got) == 1)
			require.NotContains(t, logs.String(), topic, "the topic is never logged")
		})
	}
}

func TestNtfyErrorsHideTheTopic(t *testing.T) {
	t.Parallel()
	p, logs := ntfyWith(t, "http://"+refusedAddr(t), "")
	err := p.Notify(context.Background(), idleNudge)
	require.Error(t, err)
	require.NotContains(t, err.Error(), topic)
	require.NotContains(t, logs.String(), topic)

	cfg := cfgWith(false, true)
	cfg.Ntfy.Server = "http://" + refusedAddr(t) // never the real ntfy.sh
	d := NewDispatcher(fakeNotifier{name: "desktop"}, p, nil, cfg)
	d.log = slog.New(slog.NewTextHandler(logs, nil))
	d.Send(idleNudge)
	d.Wait()
	require.Contains(t, logs.String(), "notification failed")
	require.NotContains(t, logs.String(), topic)

	empty := NewNtfy(nil, t.TempDir())
	require.Error(t, empty.Notify(context.Background(), idleNudge), "no topic")
	require.NoError(t, empty.Withdraw(context.Background(), "idle"))
	require.Equal(t, "phone", empty.Name())
}

// refusedAddr is a local address nothing listens on.
func refusedAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

// fakeBus records org.freedesktop.Notifications calls.
type fakeBus struct {
	mu      sync.Mutex
	caps    []string
	nextID  uint32
	notifys []notifyCall
	closed  []uint32
	sigs    chan *dbus.Signal
}

type notifyCall struct {
	replaces      uint32
	summary, body string
	actions       []string
	hints         map[string]dbus.Variant
	expire        int32
}

func (b *fakeBus) Capabilities(context.Context) ([]string, error) { return b.caps, nil }

func (b *fakeBus) Notify(_ context.Context, replaces uint32, summary, body string, actions []string, hints map[string]dbus.Variant, expire int32) (uint32, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.notifys = append(b.notifys, notifyCall{replaces, summary, body, actions, hints, expire})
	if replaces != 0 {
		return replaces, nil
	}
	b.nextID++
	return b.nextID, nil
}

func (b *fakeBus) Close(_ context.Context, id uint32) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = append(b.closed, id)
	return nil
}

func (b *fakeBus) Signals() (<-chan *dbus.Signal, error) { return b.sigs, nil }

func TestDesktopNotify(t *testing.T) {
	t.Parallel()
	bus := &fakeBus{caps: []string{"body", "actions"}, sigs: make(chan *dbus.Signal, 4)}
	d := newDesktop(bus)
	ctx := context.Background()
	require.True(t, d.Available())
	require.Equal(t, "desktop", d.Name())

	require.NoError(t, d.Notify(ctx, idleNudge))
	require.NoError(t, d.Notify(ctx, idleNudge))
	require.NoError(t, d.Notify(ctx, Notification{Kind: "clock_in", Title: "Ready to start?", Actions: []Action{{"clock_in", "Clock in"}}}))

	c := bus.notifys
	require.Equal(t, uint32(0), c[0].replaces)
	require.Equal(t, uint32(1), c[1].replaces, "a repeat replaces rather than stacks")
	require.Equal(t, uint32(0), c[2].replaces, "another kind is its own notification")
	require.Equal(t, int32(0), c[0].expire, "idle is persistent")
	require.Equal(t, int32(-1), c[2].expire)
	require.Equal(t, []string{"back", "I'm back", "break", "Start break", "snooze", "Snooze 10m", "default", "I'm back"}, c[0].actions)
	require.Equal(t, dbus.MakeVariant(byte(1)), c[0].hints["urgency"])
	require.Equal(t, dbus.MakeVariant("gwen"), c[0].hints["desktop-entry"])
	require.Equal(t, "Still there?", c[0].summary)

	require.NoError(t, d.Withdraw(ctx, "idle"))
	require.NoError(t, d.Withdraw(ctx, "idle"), "withdrawing twice closes once")
	require.Equal(t, []uint32{1}, bus.closed)
}

func TestDesktopWithoutActionCapability(t *testing.T) {
	t.Parallel()
	bus := &fakeBus{caps: []string{"body"}, sigs: make(chan *dbus.Signal)}
	d := newDesktop(bus)
	require.NoError(t, d.Notify(context.Background(), idleNudge))
	require.Nil(t, bus.notifys[0].actions)
}

func TestDesktopActions(t *testing.T) {
	t.Parallel()
	bus := &fakeBus{caps: []string{"actions"}, sigs: make(chan *dbus.Signal, 8)}
	d := newDesktop(bus)
	require.NoError(t, d.Notify(context.Background(), idleNudge))
	bus.sigs <- &dbus.Signal{Name: notificationsIface + ".ActionInvoked", Body: []any{uint32(1), "break"}}
	bus.sigs <- &dbus.Signal{Name: notificationsIface + ".ActionInvoked", Body: []any{uint32(1), "default"}}
	bus.sigs <- &dbus.Signal{Name: notificationsIface + ".ActionInvoked", Body: []any{uint32(99), "back"}} // not ours
	bus.sigs <- &dbus.Signal{Name: notificationsIface + ".NotificationClosed", Body: []any{uint32(1), uint32(2)}}
	require.Equal(t, ActionEvent{Kind: "idle", Action: "break"}, <-d.Actions())
	require.Equal(t, ActionEvent{Kind: "idle", Action: "back"}, <-d.Actions(), "default is the kind's first action")
	require.Eventually(t, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		_, shown := d.last["idle"]
		return !shown
	}, 5*time.Second, time.Millisecond, "a closed notification is forgotten")
	require.NoError(t, d.Withdraw(context.Background(), "idle"))
	require.Empty(t, bus.closed, "nothing left to close")
}

func TestDesktopWithoutBus(t *testing.T) {
	t.Parallel()
	d := newDesktop(nil)
	require.False(t, d.Available())
	require.NoError(t, d.Notify(context.Background(), idleNudge), "a missing bus is a no-op")
	require.ErrorIs(t, d.Notify(context.Background(), Notification{Kind: KindTest}), ErrUnavailable)
	require.NoError(t, d.Withdraw(context.Background(), "idle"))
}

// fakeNotifier records notifications and can block.
type fakeNotifier struct {
	name  string
	err   error
	block chan struct{} // when set, Notify waits for it to close
	mu    *sync.Mutex
	got   *[]string
}

func newFake(name string) fakeNotifier {
	return fakeNotifier{name: name, mu: &sync.Mutex{}, got: &[]string{}}
}

func (f fakeNotifier) Name() string { return f.name }

func (f fakeNotifier) Notify(ctx context.Context, n Notification) error {
	if f.block != nil {
		<-f.block
	}
	if f.mu != nil {
		f.mu.Lock()
		*f.got = append(*f.got, n.Kind)
		f.mu.Unlock()
	}
	return f.err
}

func (f fakeNotifier) Withdraw(context.Context, string) error { return nil }

func (f fakeNotifier) kinds() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, *f.got...)
}

// cfgWith never points at a real server: tests make no network requests.
func cfgWith(desktop, phone bool) config.Config {
	c := config.Defaults()
	c.Nudge.Desktop, c.Nudge.Phone = desktop, phone
	c.Ntfy.Server = "http://127.0.0.1:9"
	c.Ntfy.Topic = topic
	return c
}

func TestDispatcherRouting(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		cfg            config.Config
		kind           string
		desktop, phone bool
	}{
		{"idle to both", cfgWith(true, true), "idle", true, true},
		{"break_long to both", cfgWith(true, true), "break_long", true, true},
		{"break_active is desktop only", cfgWith(true, true), "break_active", true, false},
		{"clock_in is desktop only", cfgWith(true, true), "clock_in", true, false},
		{"desktop off", cfgWith(false, true), "idle", false, true},
		{"phone off", cfgWith(true, false), "idle", true, false},
		{"no topic", func() config.Config { c := cfgWith(true, true); c.Ntfy.Topic = ""; return c }(), "idle", true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			desk, phone := newFake("desktop"), newFake("phone")
			d := NewDispatcher(desk, phone, nil, tc.cfg)
			d.Send(Notification{Kind: tc.kind})
			d.Wait()
			require.Equal(t, tc.desktop, len(desk.kinds()) == 1, "desktop")
			require.Equal(t, tc.phone, len(phone.kinds()) == 1, "phone")
		})
	}
}

func TestDispatcherBackendsDoNotBlockEachOther(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	slow := newFake("desktop")
	slow.block = release
	fast := newFake("phone")
	d := NewDispatcher(slow, fast, nil, cfgWith(true, true))
	d.Send(Notification{Kind: "idle"}) // returns at once although desktop blocks
	require.Eventually(t, func() bool { return len(fast.kinds()) == 1 }, 5*time.Second, time.Millisecond,
		"the phone is delivered while the desktop is stuck")
	close(release)
	d.Wait()
	require.Equal(t, []string{"idle"}, slow.kinds())
}

func TestDispatcherAbandonsAtTheTimeout(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	stuck := newFake("desktop")
	stuck.block = make(chan struct{}) // never released
	d := NewDispatcher(stuck, newFake("phone"), nil, cfgWith(true, false))
	d.log = slog.New(slog.NewTextHandler(&logs, nil))
	d.timeout = time.Nanosecond
	d.Send(Notification{Kind: "idle"})
	d.Wait() // returns: the stuck backend was abandoned
	require.Contains(t, logs.String(), "notification abandoned")
	require.Equal(t, "timed out", d.try(context.Background(), stuck, Notification{Kind: KindTest}))
}

func TestDispatcherTest(t *testing.T) {
	t.Parallel()
	ok, broken := newFake("desktop"), newFake("phone")
	broken.err = errors.New("ntfy ntfy.sh: 403 Forbidden")
	d := NewDispatcher(ok, broken, nil, cfgWith(true, true))
	require.Equal(t, wire.NotifyTestResult{Desktop: "ok", Phone: "ntfy ntfy.sh: 403 Forbidden"}, d.Test(context.Background()))
	require.Equal(t, []string{KindTest}, broken.kinds(), "the test ignores the phone-kind filter")

	d.SetConfig(cfgWith(false, false))
	require.Equal(t, wire.NotifyTestResult{Desktop: "disabled", Phone: "disabled"}, d.Test(context.Background()))
	noTopic := cfgWith(true, true)
	noTopic.Ntfy.Topic = ""
	d.SetConfig(noTopic)
	require.Equal(t, "disabled", d.Test(context.Background()).Phone)
}

func TestDispatcherForwardsConfigAndActions(t *testing.T) {
	t.Parallel()
	phone := NewNtfy(nil, t.TempDir())
	actions := make(chan ActionEvent, 1)
	d := NewDispatcher(newFake("desktop"), phone, actions, cfgWith(true, true))
	require.Equal(t, topic, phone.cfg.Topic, "the phone follows the configuration")
	actions <- ActionEvent{Kind: "idle", Action: "back"}
	require.Equal(t, ActionEvent{Kind: "idle", Action: "back"}, <-d.Actions())

	desk := &fakeBus{caps: []string{"actions"}, sigs: make(chan *dbus.Signal)}
	dd := NewDispatcher(newDesktop(desk), phone, nil, cfgWith(true, false))
	require.NoError(t, dd.desktop.Notify(context.Background(), idleNudge))
	dd.Withdraw("idle")
	dd.Wait()
	require.Equal(t, []uint32{1}, desk.closed)
}
