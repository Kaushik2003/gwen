package activity

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/kzark/gwen/internal/clock"
)

const (
	login1Dest      = "org.freedesktop.login1"
	login1Path      = dbus.ObjectPath("/org/freedesktop/login1")
	managerIface    = "org.freedesktop.login1.Manager"
	sessionIface    = "org.freedesktop.login1.Session"
	userIface       = "org.freedesktop.login1.User"
	propertiesIface = "org.freedesktop.DBus.Properties"
)

// loginBus is the slice of org.freedesktop.login1 Gwen uses.
type loginBus interface {
	// Session finds the user's graphical session.
	Session() (dbus.ObjectPath, error)
	LockedHint(session dbus.ObjectPath) (bool, error)
	// Inhibit takes a delay inhibitor for sleep; closing it releases it.
	Inhibit() (io.Closer, error)
	// Signals delivers Lock, Unlock, PropertiesChanged of the session, and the
	// manager's PrepareForSleep, until cancel is called.
	Signals(session dbus.ObjectPath) (sigs <-chan *dbus.Signal, cancel func(), err error)
}

// login1 reports lock, unlock, suspend, and resume, holding a sleep delay
// inhibitor so that the suspend instant is written before the machine sleeps.
type login1 struct {
	clk clock.Clock
	bus loginBus

	mu        sync.Mutex
	inhibitor io.Closer
}

func newLogin1(clk clock.Clock) (*login1, error) {
	b, err := newSystemLoginBus()
	if err != nil {
		return nil, err
	}
	return &login1{clk: clk, bus: b}, nil
}

func (l *login1) run(ctx context.Context, emit func(Event)) {
	session, err := l.bus.Session()
	if err != nil {
		slog.Warn("no login session: lock events unavailable", "err", err)
	}
	sigs, cancel, err := l.bus.Signals(session)
	if err != nil {
		slog.Warn("login1 signals unavailable", "err", err)
		return
	}
	defer cancel()
	l.takeInhibitor()
	defer l.release()

	locked := false
	setLocked := func(v bool) {
		if v == locked {
			return
		}
		locked = v
		kind := Unlocked
		if v {
			kind = Locked
		}
		emit(Event{Kind: kind, At: l.clk.Now()})
	}
	if session != "" {
		if hint, err := l.bus.LockedHint(session); err == nil {
			setLocked(hint)
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case sig, ok := <-sigs:
			if !ok {
				return
			}
			switch sig.Name {
			case sessionIface + ".Lock":
				if sig.Path == session {
					setLocked(true)
				}
			case sessionIface + ".Unlock":
				if sig.Path == session {
					setLocked(false)
				}
			case propertiesIface + ".PropertiesChanged":
				if hint, ok := lockedHintChange(sig); ok && sig.Path == session {
					setLocked(hint)
				}
			case managerIface + ".PrepareForSleep":
				if len(sig.Body) != 1 {
					continue
				}
				sleeping, _ := sig.Body[0].(bool)
				if sleeping {
					emit(Event{Kind: Suspend, At: l.clk.Now(), Ack: l.release})
				} else {
					l.takeInhibitor()
					emit(Event{Kind: Resume, At: l.clk.Now()})
				}
			}
		}
	}
}

// lockedHintChange extracts a LockedHint change from PropertiesChanged.
func lockedHintChange(sig *dbus.Signal) (bool, bool) {
	if len(sig.Body) < 2 {
		return false, false
	}
	if iface, _ := sig.Body[0].(string); iface != sessionIface {
		return false, false
	}
	changed, _ := sig.Body[1].(map[string]dbus.Variant)
	v, ok := changed["LockedHint"]
	if !ok {
		return false, false
	}
	hint, ok := v.Value().(bool)
	return hint, ok
}

func (l *login1) takeInhibitor() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inhibitor != nil {
		return
	}
	inh, err := l.bus.Inhibit()
	if err != nil {
		slog.Warn("sleep inhibitor unavailable: a suspend may be recorded late", "err", err)
		return
	}
	l.inhibitor = inh
}

// release closes the sleep inhibitor if held; it is the Suspend event's Ack.
func (l *login1) release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inhibitor != nil {
		l.inhibitor.Close()
		l.inhibitor = nil
	}
}

// systemLoginBus implements loginBus over the system bus.
type systemLoginBus struct {
	conn *dbus.Conn
	mgr  dbus.BusObject
}

func newSystemLoginBus() (*systemLoginBus, error) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, fmt.Errorf("system bus: %w", err)
	}
	return &systemLoginBus{conn: conn, mgr: conn.Object(login1Dest, login1Path)}, nil
}

// Session is the session in the user's Display property: gwend runs as a
// user service outside any session, so its own PID has none. $XDG_SESSION_ID
// is the fallback.
func (b *systemLoginBus) Session() (dbus.ObjectPath, error) {
	var user dbus.ObjectPath
	if err := b.mgr.Call(managerIface+".GetUser", 0, uint32(os.Getuid())).Store(&user); err == nil {
		v, err := b.conn.Object(login1Dest, user).GetProperty(userIface + ".Display")
		if err == nil {
			if display, ok := v.Value().([]any); ok && len(display) == 2 {
				if path, ok := display[1].(dbus.ObjectPath); ok && path != "/" && path != "" {
					return path, nil
				}
			}
		}
	}
	id := os.Getenv("XDG_SESSION_ID")
	if id == "" {
		return "", errors.New("the user has no graphical session and XDG_SESSION_ID is unset")
	}
	var path dbus.ObjectPath
	if err := b.mgr.Call(managerIface+".GetSession", 0, id).Store(&path); err != nil {
		return "", fmt.Errorf("GetSession %s: %w", id, err)
	}
	return path, nil
}

func (b *systemLoginBus) LockedHint(session dbus.ObjectPath) (bool, error) {
	v, err := b.conn.Object(login1Dest, session).GetProperty(sessionIface + ".LockedHint")
	if err != nil {
		return false, err
	}
	hint, _ := v.Value().(bool)
	return hint, nil
}

func (b *systemLoginBus) Inhibit() (io.Closer, error) {
	var fd dbus.UnixFD
	err := b.mgr.Call(managerIface+".Inhibit", 0, "sleep", "Gwen",
		"Record the suspend time before sleeping", "delay").Store(&fd)
	if err != nil {
		return nil, fmt.Errorf("Inhibit: %w", err)
	}
	return os.NewFile(uintptr(fd), "gwen-sleep-inhibitor"), nil
}

func (b *systemLoginBus) Signals(session dbus.ObjectPath) (<-chan *dbus.Signal, func(), error) {
	rules := [][]dbus.MatchOption{
		{dbus.WithMatchObjectPath(login1Path), dbus.WithMatchInterface(managerIface), dbus.WithMatchMember("PrepareForSleep")},
	}
	if session != "" {
		rules = append(rules,
			[]dbus.MatchOption{dbus.WithMatchObjectPath(session), dbus.WithMatchInterface(sessionIface), dbus.WithMatchMember("Lock")},
			[]dbus.MatchOption{dbus.WithMatchObjectPath(session), dbus.WithMatchInterface(sessionIface), dbus.WithMatchMember("Unlock")},
			[]dbus.MatchOption{dbus.WithMatchObjectPath(session), dbus.WithMatchInterface(propertiesIface), dbus.WithMatchMember("PropertiesChanged")},
		)
	}
	var added [][]dbus.MatchOption
	cancel := func() {
		for _, r := range added {
			_ = b.conn.RemoveMatchSignal(r...)
		}
	}
	for _, r := range rules {
		if err := b.conn.AddMatchSignal(r...); err != nil {
			cancel()
			return nil, nil, fmt.Errorf("subscribe to login1: %w", err)
		}
		added = append(added, r)
	}
	ch := make(chan *dbus.Signal, 16)
	b.conn.Signal(ch)
	return ch, func() {
		b.conn.RemoveSignal(ch)
		cancel()
	}, nil
}
