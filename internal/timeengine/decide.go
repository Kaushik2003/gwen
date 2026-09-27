package timeengine

import (
	"fmt"
	"time"

	"github.com/kzark/gwen/internal/model"
)

// decider builds one Decision from a copy of the snapshot. Within one
// transition, effects are ordered store effects, then RecordTransition, then
// Withdraw and Notify.
type decider struct {
	s           Snapshot
	effects     []Effect
	shownBefore shownSet
}

func (d *decider) emit(e Effect) { d.effects = append(d.effects, e) }

func (d *decider) invalid(op string) error { return &stateError{state: d.s.State, op: op} }

// apply processes every deadline due by the input's instant, then the input.
func (d *decider) apply(in Input) error {
	at := in.instant()
	wasOff := d.s.State == Off
	d.catchUp(at)
	if d.s.SnoozedUntil != nil && !at.Before(*d.s.SnoozedUntil) {
		d.s.SnoozedUntil = nil
	}
	switch in := in.(type) {
	case ClockIn:
		return d.clockIn(in)
	case ClockOut:
		return d.clockOut(at)
	case BreakStart:
		return d.breakStart(at)
	case BreakEnd:
		return d.breakEnd(at)
	case Switch:
		return d.switchTo(in.ProjectID, in.TaskID, at)
	case Snooze:
		if d.s.State == Off {
			return d.invalid("snooze nudges")
		}
		until := at.Add(d.s.cfg.Snooze)
		d.s.SnoozedUntil = &until
	case Idle:
		d.idle(in.Threshold, at)
	case Active:
		d.active(at)
	case Locked:
		d.locked(at)
	case Unlocked:
		d.unlocked(at)
	case Suspend:
		d.suspend(at)
	case Resume:
		// A Resume whose own catch-up ended the day (a suspend across the
		// rollover) is not a reason to prompt; the first input after it is.
		if d.s.State == Off && wasOff {
			d.promptClockIn(at)
		}
	case Tick:
		// Deadlines only.
	case ConfigChanged:
		d.configChanged(in.Config)
	}
	return nil
}

func (d *decider) clockIn(in ClockIn) error {
	if d.s.State != Off {
		return d.invalid("clock in")
	}
	at := in.At
	c := d.s.cfg
	day := dayOf(at, c)
	d.s.ProjectID, d.s.TaskID = in.ProjectID, in.TaskID
	d.s.Day, d.s.TZ = day, c.location().String()
	d.emit(StartDay{Day: day, TZ: d.s.TZ, TargetSeconds: int(c.DailyTarget / time.Second), At: at})
	d.open(model.KindWork, model.SourceUser, at)
	d.setState(Working, at, in.trigger(), at, map[string]any{
		"day": day, "project_id": in.ProjectID, "task_id": in.TaskID,
	})
	d.withdraw(NudgeClockIn)
	return nil
}

func (d *decider) clockOut(at time.Time) error {
	end := at
	switch d.s.State {
	case Off:
		return d.invalid("clock out")
	case IdlePending:
		// Confirming the idle time was not work: end at the last input.
		end = *d.s.IdleSince
	}
	d.close(end)
	d.emit(CloseDay{At: end})
	d.s.nudges.clockedOutDay = d.s.Day
	d.endDay(end, "clock_out", at)
	return nil
}

func (d *decider) breakStart(at time.Time) error {
	switch d.s.State {
	case Working:
		d.close(at)
		d.open(model.KindBreakManual, model.SourceUser, at)
		d.setState(BreakManual, at, "break_start", at, nil)
	case IdlePending:
		since := *d.s.IdleSince
		d.s.IdleSince = nil
		d.close(since)
		d.open(model.KindBreakManual, model.SourceUser, since)
		d.setState(BreakManual, since, "break_start", at, nil)
		d.withdraw(NudgeIdle)
	case BreakAuto:
		d.close(at)
		d.open(model.KindBreakManual, model.SourceUser, at)
		d.setState(BreakManual, at, "break_start", at, nil)
		d.withdraw(NudgeBreakLong)
	default:
		return d.invalid("start a break")
	}
	return nil
}

func (d *decider) breakEnd(at time.Time) error {
	if !d.s.State.isBreak() {
		return d.invalid("end a break")
	}
	d.close(at)
	d.open(model.KindWork, model.SourceUser, at)
	d.setState(Working, at, "break_end", at, nil)
	d.withdraw(NudgeBreakLong)
	d.withdraw(NudgeBreakActive)
	return nil
}

// switchTo changes the attribution. While working it splits the open segment;
// during a break it only changes what work resumes on. From idle_pending it is
// presence as well.
func (d *decider) switchTo(project, task *string, at time.Time) error {
	if d.s.State == Off {
		return d.invalid("switch")
	}
	wasIdle := d.s.State == IdlePending
	if !sameID(project, d.s.ProjectID) || !sameID(task, d.s.TaskID) {
		d.s.ProjectID, d.s.TaskID = project, task
		if d.s.State == Working || wasIdle {
			d.close(at)
			d.open(model.KindWork, model.SourceUser, at)
		}
	}
	if wasIdle {
		d.s.IdleSince = nil
		d.setState(Working, at, "switch", at, nil)
		d.withdraw(NudgeIdle)
	}
	return nil
}

func (d *decider) idle(threshold time.Duration, at time.Time) {
	c := d.s.cfg
	switch threshold {
	case c.SoftIdle:
		if d.s.State != Working {
			return
		}
		since := d.clampIdle(at.Add(-c.SoftIdle))
		d.s.IdleSince = &since
		d.setState(IdlePending, at, "idle", at, map[string]any{
			"threshold_ms": c.SoftIdle.Milliseconds(), "idle_since": since.UnixMilli(),
		})
		d.notify(NudgeIdle, at)
	case c.HardIdle:
		switch d.s.State {
		case Working:
			d.reclaim(d.clampIdle(at.Add(-c.HardIdle)), model.SourceIdle, "idle", at)
		case IdlePending:
			d.reclaim(*d.s.IdleSince, model.SourceIdle, "idle", at)
		}
	}
}

// clampIdle keeps idle_since no earlier than the open segment's start.
func (d *decider) clampIdle(t time.Time) time.Time {
	if d.s.Segment != nil && t.Before(d.s.Segment.StartedAt) {
		return d.s.Segment.StartedAt
	}
	return t
}

func (d *decider) active(at time.Time) {
	if d.s.Locked {
		return // input reaching the lock screen is not presence
	}
	switch d.s.State {
	case Off:
		d.promptClockIn(at)
	case IdlePending:
		d.s.IdleSince = nil
		d.setState(Working, at, "active", at, nil)
		d.withdraw(NudgeIdle)
	case BreakAuto:
		d.resumeWork(at, "active")
	case BreakManual:
		n := &d.s.nudges
		if !n.breakActiveSent && at.Sub(d.s.Segment.StartedAt) >= breakActiveAfter {
			n.breakActiveSent = true
			d.notify(NudgeBreakActive, at)
		}
	}
}

func (d *decider) locked(at time.Time) {
	d.s.Locked = true
	switch d.s.State {
	case Working:
		d.close(at)
		d.open(model.KindBreakAuto, model.SourceLock, at)
		d.setState(BreakAuto, at, "locked", at, nil)
	case IdlePending:
		d.reclaim(*d.s.IdleSince, model.SourceLock, "locked", at)
	}
}

func (d *decider) unlocked(at time.Time) {
	d.s.Locked = false
	switch d.s.State {
	case Off:
		d.promptClockIn(at)
	case BreakAuto:
		d.resumeWork(at, "unlocked")
	}
}

func (d *decider) suspend(at time.Time) {
	switch d.s.State {
	case Working:
		d.close(at)
		d.open(model.KindBreakAuto, model.SourceSuspend, at)
		d.setState(BreakAuto, at, "suspend", at, nil)
	case IdlePending:
		d.reclaim(*d.s.IdleSince, model.SourceSuspend, "suspend", at)
	}
}

func (d *decider) configChanged(c Config) {
	d.s.cfg = c
	n := &d.s.nudges
	if d.s.State.isBreak() && !n.breakLongFired {
		n.nextBreakLong = d.s.Segment.StartedAt.Add(c.BreakReminder)
	}
}

// reclaim ends work at the last input instant and starts an automatic break
// there (05-time-engine.md#retroactive-reclaim).
func (d *decider) reclaim(since time.Time, source, trigger string, at time.Time) {
	d.s.IdleSince = nil
	d.close(since)
	d.open(model.KindBreakAuto, source, since)
	d.setState(BreakAuto, since, trigger, at, map[string]any{"source": source})
	d.withdraw(NudgeIdle)
}

// resumeWork ends an automatic break on presence.
func (d *decider) resumeWork(at time.Time, trigger string) {
	d.close(at)
	d.open(model.KindWork, model.SourceActivity, at)
	d.setState(Working, at, trigger, at, nil)
	d.withdraw(NudgeBreakLong)
}

func (d *decider) promptClockIn(at time.Time) {
	day := dayOf(at, d.s.cfg)
	n := &d.s.nudges
	if day == n.promptedDay || day == n.clockedOutDay {
		return
	}
	n.promptedDay = day
	d.notify(NudgeClockIn, at)
}

// endDay finishes the work day at end: state off, nudges withdrawn, and their
// bookkeeping reset.
func (d *decider) endDay(end time.Time, trigger string, at time.Time) {
	d.s.IdleSince, d.s.SnoozedUntil = nil, nil
	d.s.Day, d.s.TZ = "", ""
	d.setState(Off, end, trigger, at, nil)
	for _, kind := range allNudges {
		d.withdraw(kind)
	}
	n := &d.s.nudges
	*n = nudgeState{promptedDay: n.promptedDay, clockedOutDay: n.clockedOutDay}
}

// open starts a segment. Work segments carry the attribution; breaks start the
// break's nudge bookkeeping.
func (d *decider) open(kind, source string, at time.Time) {
	var project, task *string
	if kind == model.KindWork {
		project, task = d.s.ProjectID, d.s.TaskID
	}
	d.emit(OpenSegment{Kind: kind, Source: source, ProjectID: project, TaskID: task, At: at})
	d.s.Segment = &Segment{Kind: kind, Source: source, ProjectID: project, TaskID: task, StartedAt: at}
	n := &d.s.nudges
	n.breakLongFired, n.breakActiveSent = false, false
	n.nextBreakLong = time.Time{}
	if kind != model.KindWork {
		n.nextBreakLong = at.Add(d.s.cfg.BreakReminder)
	}
}

func (d *decider) close(at time.Time) {
	if d.s.Segment == nil {
		return
	}
	d.emit(CloseSegment{At: at})
	d.s.Segment = nil
}

// setState moves to a state and records the transition when the state changes.
func (d *decider) setState(to State, since time.Time, trigger string, at time.Time, data map[string]any) {
	from := d.s.State
	d.s.State, d.s.Since = to, since
	if from != to {
		d.record(trigger, from, to, at, data)
	}
}

func (d *decider) record(trigger string, from, to State, at time.Time, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	d.emit(RecordTransition{Trigger: trigger, From: string(from), To: string(to), At: at, Data: data})
}

func (d *decider) withdraw(kind string) {
	shown := d.s.nudges.shown.ptr(kind)
	if *shown {
		*shown = false
		d.emit(Withdraw{Kind: kind})
	}
}

// notify sends a nudge unless snoozed; idle and break_long are dropped while
// at < snoozed_until.
func (d *decider) notify(kind string, at time.Time) {
	if (kind == NudgeIdle || kind == NudgeBreakLong) && d.s.SnoozedUntil != nil && at.Before(*d.s.SnoozedUntil) {
		return
	}
	*d.s.nudges.shown.ptr(kind) = true
	d.emit(d.nudge(kind, at))
}

// collapse drops a nudge that is withdrawn later in the same decision, and a
// Withdraw of something that was never shown.
func (d *decider) collapse() []Effect {
	var out []Effect
	notified := shownSet{}
	for i, e := range d.effects {
		switch e := e.(type) {
		case Notify:
			if withdrawnAfter(d.effects[i+1:], e.Kind) {
				continue
			}
			*notified.ptr(e.Kind) = true
		case Withdraw:
			if !d.shownBefore.has(e.Kind) && !notified.has(e.Kind) {
				continue
			}
		}
		out = append(out, e)
	}
	return out
}

func withdrawnAfter(effects []Effect, kind string) bool {
	for _, e := range effects {
		if w, ok := e.(Withdraw); ok && w.Kind == kind {
			return true
		}
	}
	return false
}

func sameID(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// dayOf is the day an instant belongs to: the wall-clock date of at minus the
// day rollover (05-time-engine.md#day-boundaries).
func dayOf(at time.Time, c Config) string {
	lt := at.In(c.location())
	y, m, dd := lt.Date()
	if lt.Hour()*60+lt.Minute() < int(c.DayRollover) {
		y, m, dd = time.Date(y, m, dd-1, 12, 0, 0, 0, c.location()).Date()
	}
	return fmt.Sprintf("%04d-%02d-%02d", y, m, dd)
}

// rolloverAfter is the rollover instant that ends day: the next day at
// day_rollover on the wall clock.
func rolloverAfter(day string, c Config) time.Time {
	t, err := time.Parse(model.DayLayout, day)
	if err != nil {
		panic("timeengine: malformed day " + day) // days are produced by dayOf
	}
	return c.DayRollover.On(t.Year(), t.Month(), t.Day()+1, c.location())
}
