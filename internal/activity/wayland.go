package activity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kzark/gwen/internal/activity/extidle"
	"github.com/kzark/gwen/internal/clock"
	"github.com/rajveermalviya/go-wayland/wayland/client"
)

const (
	notifierIface = "ext_idle_notifier_v1"
	seatIface     = "wl_seat"
	// notifierVersion is the highest ext_idle_notifier_v1 version spoken; from
	// version 2, input idle notifications ignore idle inhibitors, so a playing
	// video is not presence.
	notifierVersion = 2
)

// waylandIdle speaks ext_idle_notifier_v1 in process. Each run opens its own
// connection, and every call on it happens on the run's goroutine; the
// client is not safe for concurrent use.
type waylandIdle struct {
	clk clock.Clock
}

func probeWayland(_ context.Context, clk clock.Clock) (idleBackend, error) {
	conn, err := dialWayland()
	if err != nil {
		return nil, err
	}
	defer conn.close()
	if conn.notifier.name == 0 {
		return nil, fmt.Errorf("compositor does not advertise %s", notifierIface)
	}
	if conn.seat.name == 0 {
		return nil, errors.New("compositor advertises no seat")
	}
	return waylandIdle{clk: clk}, nil
}

func (waylandIdle) name() string { return NameWayland }

func (w waylandIdle) run(ctx context.Context, thresholds []time.Duration, emit func(Event)) error {
	conn, err := dialWayland()
	if err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { conn.close() })
	defer stop()
	defer conn.close()

	seat := client.NewSeat(conn.ctx())
	if err := bind(conn.registry, conn.seat.name, seatIface, 1, seat); err != nil {
		return fmt.Errorf("bind seat: %w", err)
	}
	version := min(conn.notifier.version, notifierVersion)
	notifier := extidle.NewIdleNotifier(conn.ctx())
	if err := bind(conn.registry, conn.notifier.name, notifierIface, version, notifier); err != nil {
		return fmt.Errorf("bind idle notifier: %w", err)
	}
	t := &tracker{clk: w.clk, emit: emit}
	for _, th := range thresholds {
		ms := uint32(max(th.Milliseconds(), 1))
		var n *extidle.IdleNotification
		if version >= 2 {
			n, err = notifier.GetInputIdleNotification(ms, seat)
		} else {
			n, err = notifier.GetIdleNotification(ms, seat)
		}
		if err != nil {
			return fmt.Errorf("idle notification for %s: %w", th, err)
		}
		th := th
		n.SetIdledHandler(func(extidle.IdleNotificationIdledEvent) { t.idled(th) })
		n.SetResumedHandler(func(extidle.IdleNotificationResumedEvent) { t.resumed() })
	}
	for {
		if err := conn.dispatch(); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
	}
}

// global is an advertised Wayland global; name 0 means absent.
type global struct {
	name, version uint32
}

type waylandConn struct {
	display  *client.Display
	registry *client.Registry
	notifier global
	seat     global
	fatal    error
}

// dialWayland connects to $WAYLAND_DISPLAY (wayland-0 when unset) and
// collects the globals Gwen needs with one round trip.
func dialWayland() (*waylandConn, error) {
	display, err := client.Connect("")
	if err != nil {
		return nil, fmt.Errorf("connect to wayland: %w", err)
	}
	c := &waylandConn{display: display}
	display.SetErrorHandler(func(e client.DisplayErrorEvent) {
		c.fatal = fmt.Errorf("wayland protocol error %d: %s", e.Code, e.Message)
	})
	if c.registry, err = display.GetRegistry(); err != nil {
		c.close()
		return nil, fmt.Errorf("wayland registry: %w", err)
	}
	c.registry.SetGlobalHandler(func(e client.RegistryGlobalEvent) {
		switch e.Interface {
		case notifierIface:
			c.notifier = global{e.Name, e.Version}
		case seatIface:
			if c.seat.name == 0 {
				c.seat = global{e.Name, e.Version}
			}
		}
	})
	if err := c.roundtrip(); err != nil {
		c.close()
		return nil, err
	}
	return c, nil
}

func (c *waylandConn) ctx() *client.Context { return c.display.Context() }

func (c *waylandConn) dispatch() error {
	if err := c.ctx().Dispatch(); err != nil {
		return err
	}
	return c.fatal
}

// roundtrip dispatches until the server has handled every request sent so far.
func (c *waylandConn) roundtrip() error {
	cb, err := c.display.Sync()
	if err != nil {
		return fmt.Errorf("wayland sync: %w", err)
	}
	done := false
	cb.SetDoneHandler(func(client.CallbackDoneEvent) { done = true })
	for !done {
		if err := c.dispatch(); err != nil {
			return fmt.Errorf("wayland round trip: %w", err)
		}
	}
	return nil
}

func (c *waylandConn) close() { c.ctx().Close() }

// bind sends wl_registry.bind. It replaces client.Registry.Bind, whose string
// encoder writes the padded length where the protocol wants the length with
// the terminating NUL; libwayland rejects that unless the two happen to match,
// as they do for "wl_seat" and not for "ext_idle_notifier_v1".
func bind(r *client.Registry, name uint32, iface string, version uint32, id client.Proxy) error {
	return r.Context().WriteMsg(bindRequest(r.ID(), name, iface, version, id.ID()), nil)
}

// bindRequest encodes wl_registry.bind (opcode 0): name, the interface as a
// Wayland string, version, and the new object's id.
func bindRequest(registry, name uint32, iface string, version, id uint32) []byte {
	strLen := len(iface) + 1 // with the terminating NUL
	padded := client.PaddedLen(strLen)
	size := 8 + 4 + 4 + padded + 4 + 4
	buf := make([]byte, size)
	client.PutUint32(buf[0:4], registry)
	client.PutUint32(buf[4:8], uint32(size<<16)) // opcode 0
	client.PutUint32(buf[8:12], name)
	client.PutUint32(buf[12:16], uint32(strLen))
	copy(buf[16:], iface) // the NUL and padding are already zero
	off := 16 + padded
	client.PutUint32(buf[off:off+4], version)
	client.PutUint32(buf[off+4:off+8], id)
	return buf
}
