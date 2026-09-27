// Package timeengine turns activity events and user commands into segments,
// as specified in docs/05-time-engine.md. It is pure: no I/O, no goroutines,
// no clock reads, and no logging. Every input carries its own instant, and a
// Decision lists the effects the caller commits before calling Accept.
package timeengine

import (
	"errors"
	"time"

	"github.com/kzark/gwen/internal/config"
)

// PresenceThreshold is the fixed idle threshold the monitor also reports, so
// that a return from any break is seen within moments. The engine ignores it.
const PresenceThreshold = 5 * time.Second

// staleNudge is how late a deadline that only nudges may be processed and still
// fire; older ones are skipped (05-time-engine.md#inputs).
const staleNudge = 60 * time.Second

// breakActiveAfter is how long a manual break must last before typing during
// it nudges.
const breakActiveAfter = 60 * time.Second

// quickRestart bounds how long the daemon may have been gone for an open
// segment to survive recovery untouched.
const quickRestart = 60 * time.Second

// ErrInvalidState is matched by the error Decide returns for a command the
// current state does not allow.
var ErrInvalidState = errors.New("invalid state for this operation")

// Config is what the engine needs from the configuration.
type Config struct {
	SoftIdle      time.Duration
	HardIdle      time.Duration
	BreakReminder time.Duration
	Repeat        time.Duration
	Snooze        time.Duration
	DayRollover   config.TimeOfDay
	DailyTarget   time.Duration
	// Location is the device's zone. Days and rollovers are computed on its
	// wall clock, and its name is written to work_days.tz, so it must carry an
	// IANA name rather than "Local".
	Location *time.Location
}

// ConfigFrom takes the engine's settings from the configuration.
func ConfigFrom(c config.Config, loc *time.Location) Config {
	return Config{
		SoftIdle:      c.Tracking.SoftIdle,
		HardIdle:      c.Tracking.HardIdle,
		BreakReminder: c.Nudge.BreakReminder,
		Repeat:        c.Nudge.Repeat,
		Snooze:        c.Nudge.Snooze,
		DayRollover:   c.Tracking.DayRollover,
		DailyTarget:   c.Tracking.DailyTarget,
		Location:      loc,
	}
}

// Thresholds are the idle durations the activity monitor must report,
// ascending: presence, soft, and hard.
func (c Config) Thresholds() []time.Duration {
	out := []time.Duration{PresenceThreshold}
	for _, d := range []time.Duration{c.SoftIdle, c.HardIdle} {
		if d != out[len(out)-1] {
			out = append(out, d)
		}
	}
	return out
}

func (c Config) location() *time.Location {
	if c.Location == nil {
		return time.UTC
	}
	return c.Location
}

// Engine is the tracking state machine. It is not safe for concurrent use; the
// daemon drives it from a single goroutine.
type Engine struct {
	cur      Snapshot
	recovery []Effect
}

// Decision is the outcome of one input: the snapshot to adopt and the effects
// to apply first, in order, in one store transaction.
type Decision struct {
	Next    Snapshot
	Effects []Effect
}

// Decide computes the result of an input without changing the engine. For a
// command the state does not allow it returns an error matching
// ErrInvalidState, whose message is fit to show the user.
func (e *Engine) Decide(in Input) (Decision, error) {
	d := decider{s: e.cur, shownBefore: e.cur.nudges.shown}
	if err := d.apply(in); err != nil {
		return Decision{}, err
	}
	return Decision{Next: d.s, Effects: d.collapse()}, nil
}

// Accept makes a decision current. The daemon calls it only after the
// decision's effects have been committed.
func (e *Engine) Accept(d Decision) { e.cur = d.Next }

// NextDeadline is the earliest instant at which the engine needs a Tick.
func (e *Engine) NextDeadline() (time.Time, bool) {
	dl, _, ok := e.cur.nextDeadline()
	return dl, ok
}

// Status returns the current snapshot.
func (e *Engine) Status() Snapshot { return e.cur }

// Config returns the configuration in effect.
func (e *Engine) Config() Config { return e.cur.cfg }

// RecoveryEffects are the effects New derived from the stored state; the
// daemon commits them before anything else.
func (e *Engine) RecoveryEffects() []Effect { return e.recovery }

// DayOf is the day an instant belongs to: the wall-clock date of t minus the
// day rollover (docs/05-time-engine.md#day-boundaries).
func (c Config) DayOf(t time.Time) string { return dayOf(t, c) }
