package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/wire"
)

// renderInterval re-renders the ticking timers; the view sends only what
// changed, so most ticks cost nothing.
const renderInterval = time.Second

// view shows a Menu; the systray adapter is the real one.
type view interface{ show(Menu) }

// runner starts programs without waiting; tests replace it.
type runner interface {
	start(name string, args ...string) error
}

// controller keeps the tray in step with the daemon without ever blocking the
// UI on it: API calls run on their own goroutines.
type controller struct {
	api  client.API
	clk  clock.Clock
	view view
	exec runner

	mu       sync.Mutex
	status   *wire.Status
	daemonUp bool
	projects []wire.Project
	skew     time.Duration // local clock minus the daemon's, at the last status
	// last is the attribution of the most recent non-off status, which Clock
	// in resumes; kept in memory only.
	lastProject, lastTask *string
	// energy is this session's last check-in, shown on the submenu.
	energy *wire.EnergyLog
}

// run follows the event stream and re-renders until ctx ends.
func (c *controller) run(ctx context.Context) {
	c.render()
	go client.Follower{
		API:          c.api,
		Clock:        c.clk,
		OnConnect:    func() { go c.refreshProjects(ctx) },
		OnEvent:      func(ev wire.Event) { c.onEvent(ctx, ev) },
		OnDisconnect: func(error) { c.setDown() },
	}.Run(ctx)
	t := c.clk.NewTimer(renderInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C():
			c.render()
			t.Reset(renderInterval)
		}
	}
}

func (c *controller) onEvent(ctx context.Context, ev wire.Event) {
	switch ev.Name {
	case wire.EventStateChanged:
		var st wire.Status
		if err := json.Unmarshal(ev.Data, &st); err != nil {
			slog.Warn("bad state_changed event", "err", err)
			return
		}
		c.setStatus(&st)
	case wire.EventProjectsChanged:
		go c.refreshProjects(ctx)
	}
}

func (c *controller) setStatus(st *wire.Status) {
	c.mu.Lock()
	c.status, c.daemonUp = st, true
	c.skew = c.clk.Now().Sub(wire.Time(st.ServerNowAt))
	if st.State != wire.StateOff {
		c.lastProject, c.lastTask = st.ProjectID, st.TaskID
	}
	c.mu.Unlock()
	c.render()
}

func (c *controller) setDown() {
	c.mu.Lock()
	c.daemonUp = false
	c.mu.Unlock()
	c.render()
}

func (c *controller) refreshProjects(ctx context.Context) {
	list, err := c.api.ListProjects(ctx, wire.ArchivedFalse)
	if err != nil {
		slog.Debug("list projects", "err", err)
		return
	}
	c.mu.Lock()
	c.projects = list.Projects
	c.mu.Unlock()
	c.render()
}

func (c *controller) menu() Menu {
	c.mu.Lock()
	defer c.mu.Unlock()
	m := menuFor(c.status, c.daemonUp, c.projects, c.clk.Now().Add(-c.skew))
	m.Energy, m.EnergyTitle = c.daemonUp, energyTitle(c.energy)
	return m
}

func (c *controller) render() { c.view.show(c.menu()) }

// command runs a tracking command off the UI goroutine and shows its result.
func (c *controller) command(call func(ctx context.Context) (*wire.Status, error)) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		st, err := call(ctx)
		switch {
		case errors.Is(err, client.ErrDaemonNotRunning):
			c.setDown()
		case err != nil:
			slog.Warn("tray command failed", "err", err)
		default:
			c.setStatus(st)
		}
	}()
}

func (c *controller) clockIn() {
	c.mu.Lock()
	req := wire.ClockInRequest{ProjectID: c.lastProject, TaskID: c.lastTask}
	c.mu.Unlock()
	c.command(func(ctx context.Context) (*wire.Status, error) { return c.api.ClockIn(ctx, req) })
}

func (c *controller) startBreak() { c.command(c.api.BreakStart) }
func (c *controller) endBreak()   { c.command(c.api.BreakEnd) }
func (c *controller) snooze()     { c.command(c.api.Snooze) }
func (c *controller) clockOut()   { c.command(c.api.ClockOut) }

// switchTo changes the project; the task is dropped, as the submenu lists
// projects only.
func (c *controller) switchTo(project *string) {
	c.command(func(ctx context.Context) (*wire.Status, error) {
		return c.api.Switch(ctx, wire.SwitchRequest{ProjectID: project})
	})
}

func (c *controller) openDashboard() { c.launch("gwen-ui") }

// openNowCard opens the now card, or closes it when it is open already.
func (c *controller) openNowCard() { c.launch("gwen-ui", "--now") }

// openScreen opens the dashboard on one screen, such as "assistant".
func (c *controller) openScreen(screen string) { c.launch("gwen-ui", "--screen", screen) }

// logEnergy records an energy check-in, 1 to 5, for biological prime time.
func (c *controller) logEnergy(level int) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		e, err := c.api.LogEnergy(ctx, wire.LogEnergyRequest{Level: level})
		switch {
		case errors.Is(err, client.ErrDaemonNotRunning):
			c.setDown()
		case err != nil:
			slog.Warn("energy check-in failed", "err", err)
		default:
			c.mu.Lock()
			c.energy = e
			c.mu.Unlock()
			c.render()
		}
	}()
}

func (c *controller) startGwen() { c.launch("systemctl", "--user", "start", "gwend.service") }

func (c *controller) launch(name string, args ...string) {
	if err := c.exec.start(name, args...); err != nil {
		slog.Warn("could not start", "program", name, "err", err)
	}
}
