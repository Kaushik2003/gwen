package timeengine

import (
	"fmt"
	"time"
)

// State is the engine state (docs/05-time-engine.md#states).
type State string

// The engine states.
const (
	Off         State = "off"
	Working     State = "working"
	IdlePending State = "idle_pending"
	BreakAuto   State = "break_auto"
	BreakManual State = "break_manual"
)

func (s State) isBreak() bool { return s == BreakAuto || s == BreakManual }

// Nudge kinds (docs/05-time-engine.md#nudges).
const (
	NudgeIdle        = "idle"
	NudgeBreakLong   = "break_long"
	NudgeBreakActive = "break_active"
	NudgeClockIn     = "clock_in"
)

// Notification action ids (docs/07-integrations.md#notifications).
const (
	ActionBack     = "back"
	ActionBreak    = "break"
	ActionSnooze   = "snooze"
	ActionEndBreak = "end_break"
	ActionClockIn  = "clock_in"
)

// Input is something that happened: a user command, an activity event, a
// deadline tick, or a configuration change. Each carries its own instant.
type Input interface {
	instant() time.Time
	trigger() string
}

// ClockIn starts or reopens the day's work day. Nil ids mean unassigned.
type ClockIn struct {
	ProjectID, TaskID *string
	At                time.Time
}

// ClockOut closes the open segment and the work day.
type ClockOut struct{ At time.Time }

// BreakStart starts a manual break.
type BreakStart struct{ At time.Time }

// BreakEnd ends any break.
type BreakEnd struct{ At time.Time }

// Switch changes the attribution. Nil ids mean unassigned.
type Switch struct {
	ProjectID, TaskID *string
	At                time.Time
}

// Snooze suppresses nudges for nudge.snooze.
type Snooze struct{ At time.Time }

// Idle reports that input stopped Threshold ago.
type Idle struct {
	Threshold time.Duration
	At        time.Time
}

// Active reports input after an Idle.
type Active struct{ At time.Time }

// Locked reports that the session locked.
type Locked struct{ At time.Time }

// Unlocked reports that the session unlocked.
type Unlocked struct{ At time.Time }

// Suspend reports that the machine is about to sleep.
type Suspend struct{ At time.Time }

// Resume reports that the machine woke.
type Resume struct{ At time.Time }

// Tick is the daemon's deadline timer firing.
type Tick struct{ At time.Time }

// ConfigChanged adopts new settings; idle_since is kept.
type ConfigChanged struct {
	Config Config
	At     time.Time
}

func (i ClockIn) instant() time.Time       { return i.At }
func (i ClockOut) instant() time.Time      { return i.At }
func (i BreakStart) instant() time.Time    { return i.At }
func (i BreakEnd) instant() time.Time      { return i.At }
func (i Switch) instant() time.Time        { return i.At }
func (i Snooze) instant() time.Time        { return i.At }
func (i Idle) instant() time.Time          { return i.At }
func (i Active) instant() time.Time        { return i.At }
func (i Locked) instant() time.Time        { return i.At }
func (i Unlocked) instant() time.Time      { return i.At }
func (i Suspend) instant() time.Time       { return i.At }
func (i Resume) instant() time.Time        { return i.At }
func (i Tick) instant() time.Time          { return i.At }
func (i ConfigChanged) instant() time.Time { return i.At }

func (ClockIn) trigger() string       { return "clock_in" }
func (ClockOut) trigger() string      { return "clock_out" }
func (BreakStart) trigger() string    { return "break_start" }
func (BreakEnd) trigger() string      { return "break_end" }
func (Switch) trigger() string        { return "switch" }
func (Snooze) trigger() string        { return "snooze" }
func (Idle) trigger() string          { return "idle" }
func (Active) trigger() string        { return "active" }
func (Locked) trigger() string        { return "locked" }
func (Unlocked) trigger() string      { return "unlocked" }
func (Suspend) trigger() string       { return "suspend" }
func (Resume) trigger() string        { return "resume" }
func (Tick) trigger() string          { return "tick" }
func (ConfigChanged) trigger() string { return "config_changed" }

// InputForAction maps a notification action to the input it stands for:
// back → Active, break → BreakStart, snooze → Snooze, end_break → BreakEnd,
// and clock_in → ClockIn with the attribution in s. ok is false for an
// unknown action.
func InputForAction(action string, at time.Time, s Snapshot) (in Input, ok bool) {
	switch action {
	case ActionBack:
		return Active{At: at}, true
	case ActionBreak:
		return BreakStart{At: at}, true
	case ActionSnooze:
		return Snooze{At: at}, true
	case ActionEndBreak:
		return BreakEnd{At: at}, true
	case ActionClockIn:
		return ClockIn{ProjectID: s.ProjectID, TaskID: s.TaskID, At: at}, true
	}
	return nil, false
}

// Effect is something the daemon does for a decision. Store effects are
// applied in order in one transaction; Notify and Withdraw run after commit.
type Effect interface{ effect() }

// StartDay reopens the live work day for Day if one exists, else inserts one.
type StartDay struct {
	Day           string
	TZ            string
	TargetSeconds int
	At            time.Time
}

// CloseDay sets clocked_out_at on the open work day.
type CloseDay struct{ At time.Time }

// OpenSegment inserts an open segment on the open work day.
type OpenSegment struct {
	Kind, Source      string
	ProjectID, TaskID *string
	At                time.Time
}

// CloseSegment sets ended_at on the open segment; the store tombstones it if
// it would be shorter than 1000 ms.
type CloseSegment struct {
	At        time.Time
	Truncated bool
}

// Notify sends a nudge; At is when it fired.
type Notify struct {
	Kind, Title, Body string
	Actions           []Action
	At                time.Time
}

// Action is a notification button.
type Action struct{ ID, Label string }

// Withdraw closes a nudge of the given kind if it is still shown.
type Withdraw struct{ Kind string }

// RecordTransition inserts a row into engine_events.
type RecordTransition struct {
	Trigger, From, To string
	At                time.Time
	Data              map[string]any
}

func (StartDay) effect()         {}
func (CloseDay) effect()         {}
func (OpenSegment) effect()      {}
func (CloseSegment) effect()     {}
func (Notify) effect()           {}
func (Withdraw) effect()         {}
func (RecordTransition) effect() {}

// Snapshot is the engine's state. The exported fields describe it; the rest
// is nudge bookkeeping and the configuration in effect.
type Snapshot struct {
	State State
	Since time.Time // when State was entered
	// ProjectID and TaskID are the attribution of the open work segment, or
	// the one work resumes on.
	ProjectID, TaskID *string
	IdleSince         *time.Time // last input instant; set only in idle_pending
	SnoozedUntil      *time.Time // idle and break_long nudges dropped until then
	Locked            bool
	Day, TZ           string   // the open work day; empty when off
	Segment           *Segment // the open segment; nil when off

	cfg    Config
	nudges nudgeState
}

// Segment describes the open segment.
type Segment struct {
	Kind, Source      string
	ProjectID, TaskID *string
	StartedAt         time.Time
}

type nudgeState struct {
	shown           shownSet
	nextBreakLong   time.Time // zero outside breaks
	breakLongFired  bool
	breakActiveSent bool
	promptedDay     string // the day the clock_in nudge last fired for
	clockedOutDay   string // the day the user last clocked out of
}

// shownSet records which nudge kinds are showing: sent and not withdrawn.
type shownSet struct{ idle, breakLong, breakActive, clockIn bool }

func (s *shownSet) ptr(kind string) *bool {
	switch kind {
	case NudgeIdle:
		return &s.idle
	case NudgeBreakLong:
		return &s.breakLong
	case NudgeBreakActive:
		return &s.breakActive
	case NudgeClockIn:
		return &s.clockIn
	}
	panic("timeengine: unknown nudge kind " + kind)
}

func (s shownSet) has(kind string) bool { return *s.ptr(kind) }

var allNudges = []string{NudgeIdle, NudgeBreakLong, NudgeBreakActive, NudgeClockIn}

// stateError is a command the state does not allow.
type stateError struct {
	state State
	op    string
}

func (e *stateError) Error() string {
	var where string
	switch e.state {
	case IdlePending:
		where = "while idle"
	case BreakAuto, BreakManual:
		where = "during a break"
	default:
		where = "while " + string(e.state)
	}
	return fmt.Sprintf("cannot %s %s", e.op, where)
}

func (e *stateError) Is(target error) bool { return target == ErrInvalidState }
