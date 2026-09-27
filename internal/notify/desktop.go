package notify

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	notificationsDest  = "org.freedesktop.Notifications"
	notificationsPath  = dbus.ObjectPath("/org/freedesktop/Notifications")
	notificationsIface = "org.freedesktop.Notifications"
)

// ErrUnavailable is returned when the backend has nothing to deliver to.
var ErrUnavailable = errors.New("desktop notifications are unavailable")

// notifyBus is the slice of org.freedesktop.Notifications the desktop backend
// uses.
type notifyBus interface {
	Capabilities(ctx context.Context) ([]string, error)
	Notify(ctx context.Context, replaces uint32, summary, body string, actions []string, hints map[string]dbus.Variant, expire int32) (uint32, error)
	Close(ctx context.Context, id uint32) error
	// Signals delivers ActionInvoked and NotificationClosed.
	Signals() (<-chan *dbus.Signal, error)
}

// Desktop sends notifications over org.freedesktop.Notifications. Repeats of
// a kind replace the previous notification rather than stack.
type Desktop struct {
	bus     notifyBus
	actions chan ActionEvent

	mu       sync.Mutex
	caps     []string
	capsRead bool
	last     map[string]uint32 // kind → the id last returned
	kindOf   map[uint32]string
	first    map[string]string // kind → its first action, for "default"
}

// NewDesktop connects to the session bus. Without a bus or a notification
// service it logs a Warn and returns a backend whose sends do nothing.
func NewDesktop() *Desktop {
	b, err := newSessionNotifyBus()
	if err != nil {
		slog.Warn("desktop notifications unavailable", "err", err)
		return newDesktop(nil)
	}
	return newDesktop(b)
}

func newDesktop(bus notifyBus) *Desktop {
	d := &Desktop{bus: bus, actions: make(chan ActionEvent, 16), last: map[string]uint32{},
		kindOf: map[uint32]string{}, first: map[string]string{}}
	if bus != nil {
		sigs, err := bus.Signals()
		if err != nil {
			slog.Warn("notification actions unavailable", "err", err)
		} else {
			go d.listen(sigs)
		}
	}
	return d
}

// Name is "desktop".
func (d *Desktop) Name() string { return "desktop" }

// Available reports whether a notification service was found.
func (d *Desktop) Available() bool { return d.bus != nil }

// Actions delivers clicked buttons; "default" becomes the kind's first action.
func (d *Desktop) Actions() <-chan ActionEvent { return d.actions }

// Notify shows n. The idle and break_long nudges persist until dismissed.
func (d *Desktop) Notify(ctx context.Context, n Notification) error {
	if d.bus == nil {
		if n.Kind == KindTest {
			return ErrUnavailable
		}
		return nil
	}
	caps, err := d.capabilities(ctx)
	if err != nil {
		return err
	}
	var actions []string
	if slices.Contains(caps, "actions") && len(n.Actions) > 0 {
		for _, a := range n.Actions {
			actions = append(actions, a.ID, a.Label)
		}
		actions = append(actions, "default", n.Actions[0].Label)
	}
	expire := int32(-1)
	if n.Kind == "idle" || n.Kind == "break_long" {
		expire = 0
	}
	hints := map[string]dbus.Variant{
		"urgency":       dbus.MakeVariant(byte(1)),
		"desktop-entry": dbus.MakeVariant("gwen"),
	}
	d.mu.Lock()
	replaces := d.last[n.Kind]
	d.mu.Unlock()
	id, err := d.bus.Notify(ctx, replaces, n.Title, n.Body, actions, hints, expire)
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.last[n.Kind] = id
	d.kindOf[id] = n.Kind
	if len(n.Actions) > 0 {
		d.first[n.Kind] = n.Actions[0].ID
	}
	d.mu.Unlock()
	return nil
}

// Withdraw closes the kind's notification if one was shown.
func (d *Desktop) Withdraw(ctx context.Context, kind string) error {
	d.mu.Lock()
	id, ok := d.last[kind]
	delete(d.last, kind)
	d.mu.Unlock()
	if !ok || d.bus == nil {
		return nil
	}
	return d.bus.Close(ctx, id)
}

func (d *Desktop) capabilities(ctx context.Context) ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.capsRead {
		return d.caps, nil
	}
	caps, err := d.bus.Capabilities(ctx)
	if err != nil {
		return nil, err
	}
	d.caps, d.capsRead = caps, true
	return caps, nil
}

func (d *Desktop) listen(sigs <-chan *dbus.Signal) {
	for sig := range sigs {
		if len(sig.Body) < 2 {
			continue
		}
		id, _ := sig.Body[0].(uint32)
		switch sig.Name {
		case notificationsIface + ".ActionInvoked":
			action, _ := sig.Body[1].(string)
			d.mu.Lock()
			kind, ok := d.kindOf[id]
			if action == "default" {
				action = d.first[kind]
			}
			d.mu.Unlock()
			if ok && action != "" {
				d.actions <- ActionEvent{Kind: kind, Action: action}
			}
		case notificationsIface + ".NotificationClosed":
			d.mu.Lock()
			if kind, ok := d.kindOf[id]; ok {
				delete(d.kindOf, id)
				if d.last[kind] == id {
					delete(d.last, kind)
				}
			}
			d.mu.Unlock()
		}
	}
}

// sessionNotifyBus implements notifyBus on the session bus.
type sessionNotifyBus struct {
	conn *dbus.Conn
	obj  dbus.BusObject
}

func newSessionNotifyBus() (*sessionNotifyBus, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, err
	}
	var has bool
	if err := conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, notificationsDest).Store(&has); err != nil {
		return nil, err
	}
	if !has {
		return nil, errors.New("no notification service on the session bus")
	}
	return &sessionNotifyBus{conn: conn, obj: conn.Object(notificationsDest, notificationsPath)}, nil
}

func (b *sessionNotifyBus) Capabilities(ctx context.Context) ([]string, error) {
	var caps []string
	err := b.obj.CallWithContext(ctx, notificationsIface+".GetCapabilities", 0).Store(&caps)
	return caps, err
}

func (b *sessionNotifyBus) Notify(ctx context.Context, replaces uint32, summary, body string, actions []string,
	hints map[string]dbus.Variant, expire int32) (uint32, error) {
	if actions == nil {
		actions = []string{}
	}
	var id uint32
	err := b.obj.CallWithContext(ctx, notificationsIface+".Notify", 0,
		"Gwen", replaces, "gwen", summary, body, actions, hints, expire).Store(&id)
	return id, err
}

func (b *sessionNotifyBus) Close(ctx context.Context, id uint32) error {
	return b.obj.CallWithContext(ctx, notificationsIface+".CloseNotification", 0, id).Err
}

func (b *sessionNotifyBus) Signals() (<-chan *dbus.Signal, error) {
	for _, member := range []string{"ActionInvoked", "NotificationClosed"} {
		if err := b.conn.AddMatchSignal(dbus.WithMatchInterface(notificationsIface), dbus.WithMatchMember(member)); err != nil {
			return nil, err
		}
	}
	ch := make(chan *dbus.Signal, 16)
	b.conn.Signal(ch)
	return ch, nil
}
