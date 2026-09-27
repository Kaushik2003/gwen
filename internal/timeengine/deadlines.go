package timeengine

import (
	"time"

	"github.com/kzark/gwen/internal/model"
)

type deadlineKind int

// Kinds in the order they are processed when due at the same instant.
const (
	hardIdle deadlineKind = iota
	rollover
	breakLong
)

// nextDeadline is the earliest pending deadline: the hard idle threshold in
// idle_pending, the day rollover whenever clocked in, and break_long during a
// break, moved to the end of any snooze it falls inside.
func (s Snapshot) nextDeadline() (time.Time, deadlineKind, bool) {
	var (
		best  time.Time
		kind  deadlineKind
		found bool
	)
	consider := func(t time.Time, k deadlineKind) {
		if !found || t.Before(best) || (t.Equal(best) && k < kind) {
			best, kind, found = t, k, true
		}
	}
	if s.State == IdlePending {
		consider(s.IdleSince.Add(s.cfg.HardIdle), hardIdle)
	}
	if s.State != Off {
		consider(rolloverAfter(s.Day, s.cfg), rollover)
	}
	if s.State.isBreak() && !s.nudges.nextBreakLong.IsZero() {
		t := s.nudges.nextBreakLong
		if s.SnoozedUntil != nil && t.Before(*s.SnoozedUntil) {
			t = *s.SnoozedUntil
		}
		consider(t, breakLong)
	}
	return best, kind, found
}

// catchUp processes every deadline at or before at, in time order, as if a Tick
// had arrived at each. Every step moves the next deadline forward, so the loop
// ends; the bound only guards against a bug.
func (d *decider) catchUp(at time.Time) {
	for range 1_000_000 {
		dl, kind, ok := d.s.nextDeadline()
		if !ok || dl.After(at) {
			return
		}
		d.fire(kind, dl, at)
	}
	panic("timeengine: deadlines did not converge")
}

func (d *decider) fire(kind deadlineKind, dl, at time.Time) {
	switch kind {
	case hardIdle:
		d.reclaim(*d.s.IdleSince, model.SourceIdle, "tick", dl)
	case rollover:
		d.rollover(dl)
	case breakLong:
		n := &d.s.nudges
		if at.Sub(dl) > staleNudge {
			// Resuming after hours asleep must not produce a burst of nudges.
			n.nextBreakLong = at.Add(d.s.cfg.Repeat)
			return
		}
		d.notify(NudgeBreakLong, dl)
		n.nextBreakLong = dl.Add(d.s.cfg.Repeat)
		n.breakLongFired = true
	}
}

// rollover closes the day at the rollover instant itself. Work continues on
// the new day; a break ends the day (05-time-engine.md#day-boundaries).
func (d *decider) rollover(at time.Time) {
	c := d.s.cfg
	from, oldDay := d.s.State, d.s.Day
	d.close(at)
	d.emit(CloseDay{At: at})
	if !from.isBreak() {
		day := dayOf(at, c)
		d.s.IdleSince = nil
		d.s.Day, d.s.TZ = day, c.location().String()
		d.emit(StartDay{Day: day, TZ: d.s.TZ, TargetSeconds: int(c.DailyTarget / time.Second), At: at})
		d.open(model.KindWork, model.SourceActivity, at)
		d.s.State, d.s.Since = Working, at
		d.record("tick", from, Working, at, map[string]any{"rollover_from": oldDay, "rollover_to": day})
		d.withdraw(NudgeIdle)
		return
	}
	d.endDay(at, "tick", at)
}
