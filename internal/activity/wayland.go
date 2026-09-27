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
	if err := conn.registry.Bind(conn.seat.name, seatIface, 1, seat); err != nil {
		return fmt.Errorf("bind seat: %w", err)
	}
	version := min(conn.notifier.version, notifierVersion)
	notifier := extidle.NewIdleNotifier(conn.ctx())
	if err := conn.registry.Bind(conn.notifier.name, notifierIface, version, notifier); err != nil {
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
