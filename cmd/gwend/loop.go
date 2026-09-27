package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"time"

	"github.com/kzark/gwen/internal/activity"
	"github.com/kzark/gwen/internal/api"
	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/notify"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/timeengine"
	"github.com/kzark/gwen/internal/wire"
)

// heartbeatInterval bounds the damage of a crash (05-time-engine.md#crash-recovery).
const heartbeatInterval = 15 * time.Second

// notifier is what the loop needs from notify.Dispatcher.
type notifier interface {
	Send(notify.Notification)
	Withdraw(kind string)
	Actions() <-chan notify.ActionEvent
	SetConfig(config.Config)
	Test(ctx context.Context) wire.NotifyTestResult
}

// loop is the one goroutine that owns all tracking state. Every transition
// happens on it, serially; the API hands it inputs and waits
// (docs/02-architecture.md#concurrency-model).
type loop struct {
	clk      clock.Clock
	db       *store.DB
	repos    store.Repos
	engine   *timeengine.Engine
	hub      *api.Hub
	notifier notifier
	monitor  activity.ActivityMonitor
	loc      *time.Location
	level    *slog.LevelVar
	warnings []string

	reqs chan request
	done chan struct{}

	mu   sync.Mutex // guards the copies readers use
	cfg  config.Config
	snap timeengine.Snapshot
}

type request struct {
	build func(at time.Time) timeengine.Input // an engine input, or
	cfg   *config.Config                      // a configuration change
	reply chan reply
}

type reply struct {
	status wire.Status
	err    error
}

var errStopped = errors.New("the daemon is shutting down")

func newLoop(clk clock.Clock, db *store.DB, repos store.Repos, engine *timeengine.Engine, hub *api.Hub,
	n notifier, mon activity.ActivityMonitor, cfg config.Config, loc *time.Location, level *slog.LevelVar, warnings []string) *loop {
	return &loop{
		clk: clk, db: db, repos: repos, engine: engine, hub: hub, notifier: n, monitor: mon, loc: loc,
		level: level, warnings: warnings, reqs: make(chan request), done: make(chan struct{}),
		cfg: cfg, snap: engine.Status(),
	}
}

// Do runs an input on the loop and waits for the resulting status.
func (l *loop) Do(ctx context.Context, build func(at time.Time) timeengine.Input) (wire.Status, error) {
	return l.send(ctx, request{build: build, reply: make(chan reply, 1)})
}

// SetConfig applies a configuration on the loop.
func (l *loop) SetConfig(ctx context.Context, c config.Config) error {
	_, err := l.send(ctx, request{cfg: &c, reply: make(chan reply, 1)})
	return err
}

func (l *loop) send(ctx context.Context, req request) (wire.Status, error) {
	select {
	case l.reqs <- req:
	case <-l.done:
		return wire.Status{}, errStopped
	case <-ctx.Done():
		return wire.Status{}, ctx.Err()
	}
	select {
	case r := <-req.reply:
		return r.status, r.err
	case <-ctx.Done():
		return wire.Status{}, ctx.Err()
	}
}

// Status reads the store with the latest snapshot; reads need not go through
// the loop.
func (l *loop) Status(ctx context.Context) (wire.Status, error) {
	return api.BuildStatus(ctx, l.repos, l.snapshot(), l.clk.Now().Truncate(time.Millisecond), l.monitor.Name(), l.warnings)
}

func (l *loop) Snapshot(context.Context) (timeengine.Snapshot, error) { return l.snapshot(), nil }

func (l *loop) snapshot() timeengine.Snapshot {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.snap
}

func (l *loop) Config() config.Config {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.cfg
}

func (l *loop) now() time.Time { return l.clk.Now().Truncate(time.Millisecond) }

func (l *loop) thresholds() []time.Duration {
	return timeengine.ConfigFrom(l.Config(), l.loc).Thresholds()
}

// run processes inputs until ctx ends, then records a clean shutdown.
func (l *loop) run(ctx context.Context) {
	defer close(l.done)
	monCtx, stopMon := context.WithCancel(ctx)
	events, err := l.monitor.Start(monCtx, l.thresholds())
	if err != nil {
		slog.Warn("activity monitor did not start", "err", err)
	}
	deadline := l.clk.NewTimer(time.Hour)
	defer deadline.Stop()
	l.arm(deadline)
	heartbeat := l.clk.NewTimer(heartbeatInterval)
	defer heartbeat.Stop()
	actions := l.notifier.Actions()

	for {
		select {
		case <-ctx.Done():
			stopMon()
			l.writeInstant(store.KeyShutdownAt)
			return
		case ev, ok := <-events:
			if !ok {
				events = nil
				break
			}
			l.onActivity(ctx, ev)
		case a, ok := <-actions:
			if !ok {
				actions = nil
				break
			}
			l.onAction(ctx, a)
		case req := <-l.reqs:
			if req.cfg != nil {
				stopMon()
				monCtx, stopMon = context.WithCancel(ctx)
				err := l.applyConfig(ctx, *req.cfg)
				var startErr error
				if events, startErr = l.monitor.Start(monCtx, l.thresholds()); startErr != nil {
					slog.Warn("activity monitor did not restart", "err", startErr)
				}
				req.reply <- reply{err: err}
				break
			}
			st, err := l.apply(ctx, req.build(l.now()))
			req.reply <- reply{status: st, err: err}
		case <-deadline.C():
			l.apply(ctx, timeengine.Tick{At: l.now()})
		case <-heartbeat.C():
			l.writeInstant(store.KeyHeartbeatAt)
			heartbeat.Reset(heartbeatInterval)
		}
		l.arm(deadline)
	}
}

// arm sets the deadline timer to the engine's next deadline.
func (l *loop) arm(t clock.Timer) {
	t.Stop()
	if dl, ok := l.engine.NextDeadline(); ok {
		t.Reset(max(dl.Sub(l.clk.Now()), 0))
	}
}

func (l *loop) onActivity(ctx context.Context, ev activity.Event) {
	var in timeengine.Input
	switch ev.Kind {
	case activity.Idle:
		in = timeengine.Idle{Threshold: ev.Threshold, At: ev.At}
	case activity.Active:
		in = timeengine.Active{At: ev.At}
	case activity.Locked:
		in = timeengine.Locked{At: ev.At}
	case activity.Unlocked:
		in = timeengine.Unlocked{At: ev.At}
	case activity.Suspend:
		l.writeInstant(store.KeyHeartbeatAt) // the suspend instant must reach disk
		in = timeengine.Suspend{At: ev.At}
	case activity.Resume:
		in = timeengine.Resume{At: ev.At}
	default:
		return
	}
	slog.Debug("activity", "kind", ev.Kind.String(), "threshold", ev.Threshold)
	if _, err := l.apply(ctx, in); err != nil {
		slog.Debug("activity event not applied", "kind", ev.Kind.String(), "err", err)
	}
	if ev.Ack != nil {
		ev.Ack() // after the commit: the machine may now sleep
	}
}

func (l *loop) onAction(ctx context.Context, a notify.ActionEvent) {
	in, ok := timeengine.InputForAction(a.Action, l.now(), l.engine.Status())
	if !ok {
		slog.Debug("unknown notification action", "kind", a.Kind, "action", a.Action)
		return
	}
	if _, err := l.apply(ctx, in); err != nil {
		slog.Debug("notification action dropped", "action", a.Action, "err", err)
	}
}

func (l *loop) applyConfig(ctx context.Context, c config.Config) error {
	l.mu.Lock()
	l.cfg = c
	l.mu.Unlock()
	l.level.Set(c.Log.Level)
	l.notifier.SetConfig(c)
	_, err := l.apply(ctx, timeengine.ConfigChanged{Config: timeengine.ConfigFrom(c, l.loc), At: l.now()})
	return err
}

// apply decides, commits the effects in one transaction, and only then accepts
// the decision, sends notifications, and emits events. On a failed commit the
// engine is unchanged and the caller gets an internal error.
func (l *loop) apply(ctx context.Context, in timeengine.Input) (wire.Status, error) {
	before := l.engine.Status()
	d, err := l.engine.Decide(in)
	if err != nil {
		return wire.Status{}, err
	}
	var days []string
	err = l.db.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		days, err = applyEffects(ctx, tx, l.repos, before.Day, d.Effects)
		return err
	})
	if err != nil {
		slog.Error("commit decision", "input", fmt.Sprintf("%T", in), "err", err)
		return wire.Status{}, fmt.Errorf("commit decision: %v", err) // never a store sentinel: internal
	}
	l.engine.Accept(d)
	l.mu.Lock()
	l.snap = d.Next
	l.mu.Unlock()

	for _, e := range d.Effects {
		switch e := e.(type) {
		case timeengine.Notify:
			n := l.nudge(ctx, e)
			l.notifier.Send(n)
			l.hub.Publish(wire.EventNudgeFired, wire.NudgeFired{Kind: n.Kind, At: wire.Millis(e.At), Title: n.Title, Body: n.Body})
			slog.Info("nudge", "kind", e.Kind)
		case timeengine.Withdraw:
			l.notifier.Withdraw(e.Kind)
		case timeengine.RecordTransition:
			slog.Info("transition", "trigger", e.Trigger, "from", e.From, "to", e.To)
		}
	}
	for _, day := range days {
		l.hub.Publish(wire.EventDayChanged, wire.DayChanged{Day: day})
	}
	st, err := l.Status(ctx)
	if err != nil {
		return wire.Status{}, err
	}
	if len(d.Effects) > 0 || !reflect.DeepEqual(before, d.Next) {
		l.hub.Publish(wire.EventStateChanged, st)
	}
	return st, nil
}

// nudge turns an engine notification into one for the dispatcher.
func (l *loop) nudge(_ context.Context, e timeengine.Notify) notify.Notification {
	n := notify.Notification{Kind: e.Kind, Title: e.Title, Body: e.Body}
	for _, a := range e.Actions {
		n.Actions = append(n.Actions, notify.Action{ID: a.ID, Label: a.Label})
	}
	return n
}

// applyEffects runs a decision's store effects in order inside tx and returns
// the days whose work day or segments they wrote.
func applyEffects(ctx context.Context, tx *sql.Tx, repos store.Repos, day string, effects []timeengine.Effect) ([]string, error) {
	var days []string
	touch := func(d string) {
		if d != "" && (len(days) == 0 || days[len(days)-1] != d) {
			days = append(days, d)
		}
	}
	for _, e := range effects {
		var err error
		switch e := e.(type) {
		case timeengine.StartDay:
			_, err = repos.WorkDays.StartDay(ctx, tx, e.Day, e.TZ, e.TargetSeconds, e.At)
			day = e.Day
			touch(day)
		case timeengine.CloseDay:
			_, err = repos.WorkDays.CloseDay(ctx, tx, e.At)
			touch(day)
		case timeengine.OpenSegment:
			_, err = repos.Segments.OpenSegment(ctx, tx, e.Kind, e.Source, e.ProjectID, e.TaskID, e.At)
			touch(day)
		case timeengine.CloseSegment:
			_, err = repos.Segments.CloseSegment(ctx, tx, e.At, e.Truncated)
			touch(day)
		case timeengine.RecordTransition:
			err = repos.Events.Insert(ctx, tx, model.EngineEvent{At: e.At, Trigger: e.Trigger,
				FromState: e.From, ToState: e.To, Data: e.Data})
		}
		if err != nil {
			return nil, err
		}
	}
	return days, nil
}

// writeInstant stores the current instant under a local-state key.
func (l *loop) writeInstant(key string) {
	now := l.now()
	if err := store.SetLocalTime(context.Background(), l.db.SQL(), key, now, now); err != nil {
		slog.Error("write local state", "key", key, "err", err)
	}
}
