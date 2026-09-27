package timeengine

import (
	"time"

	"github.com/kzark/gwen/internal/model"
)

// Recovered is the stored state the daemon loads at startup.
type Recovered struct {
	Now time.Time
	// WorkDay is the open work day, and Segment its open segment, if any.
	WorkDay *model.WorkDay
	Segment *model.Segment
	// LastEndedAt is the latest ended_at among the open work day's closed
	// live segments.
	LastEndedAt *time.Time
	HeartbeatAt *time.Time
	ShutdownAt  *time.Time
	// ProjectID and TaskID are the attribution of the most recent live work
	// segment, which work resumes on.
	ProjectID, TaskID *string
	// ClosedDay is the day of the latest closed live work day, so that the
	// clock_in nudge does not prompt again on a day already clocked out.
	ClosedDay string
}

// New returns an engine in the state recovered from rec, per
// docs/05-time-engine.md#crash-recovery. The effects of recovery are in
// RecoveryEffects.
func New(cfg Config, rec Recovered) *Engine {
	d := decider{s: Snapshot{
		State:     Off,
		Since:     rec.Now,
		ProjectID: rec.ProjectID,
		TaskID:    rec.TaskID,
		cfg:       cfg,
	}}
	d.s.nudges.clockedOutDay = rec.ClosedDay
	d.recover(rec)
	return &Engine{cur: d.s, recovery: d.effects}
}

func (d *decider) recover(rec Recovered) {
	wd := rec.WorkDay
	if wd == nil {
		return // rule 1
	}
	d.s.Day, d.s.TZ = wd.Day, wd.TZ

	var lastSeen *time.Time
	switch {
	case rec.ShutdownAt != nil:
		lastSeen = rec.ShutdownAt
	case rec.HeartbeatAt != nil:
		lastSeen = rec.HeartbeatAt
	}
	from := Off
	if seg := rec.Segment; seg != nil {
		if lastSeen == nil || lastSeen.Before(seg.StartedAt) {
			lastSeen = &seg.StartedAt
		}
		from = stateForKind(seg.Kind)
		d.s.Segment = &Segment{Kind: seg.Kind, Source: seg.Source, ProjectID: seg.ProjectID,
			TaskID: seg.TaskID, StartedAt: seg.StartedAt}
		if seg.Kind == model.KindWork {
			d.s.ProjectID, d.s.TaskID = seg.ProjectID, seg.TaskID
		}
		if rec.Now.Sub(*lastSeen) <= quickRestart {
			// Rule 2: a quick restart keeps the segment open.
			d.s.State, d.s.Since = from, seg.StartedAt
			if seg.Kind != model.KindWork {
				d.s.nudges.nextBreakLong = seg.StartedAt.Add(d.s.cfg.BreakReminder)
			}
			return
		}
		// Rule 3: close it where the daemon was last seen.
		d.emit(CloseSegment{At: *lastSeen, Truncated: rec.ShutdownAt == nil})
		d.s.Segment = nil
	}

	// Rule 4: the work day is open with no open segment.
	gap := wd.ClockedInAt
	for _, t := range []*time.Time{lastSeen, rec.LastEndedAt} {
		if t != nil && t.After(gap) {
			gap = *t
		}
	}
	data := map[string]any{"gap_start": gap.UnixMilli(), "truncated": rec.Segment != nil && rec.ShutdownAt == nil}
	if dayOf(rec.Now, d.s.cfg) != wd.Day {
		d.emit(CloseDay{At: gap})
		d.s.Day, d.s.TZ = "", ""
		d.s.Since = gap
		d.record("recovery", from, Off, rec.Now, data)
		return
	}
	d.open(model.KindBreakAuto, model.SourceRecovery, gap)
	d.s.State, d.s.Since = BreakAuto, gap
	d.record("recovery", from, BreakAuto, rec.Now, data)
}

func stateForKind(kind string) State {
	switch kind {
	case model.KindBreakAuto:
		return BreakAuto
	case model.KindBreakManual:
		return BreakManual
	}
	return Working
}
