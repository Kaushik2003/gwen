// Package model holds the domain structs shared by every layer, one per table in
// docs/03-data-model.md. Go types follow the time-representation rules there:
// instants are time.Time, local dates are "YYYY-MM-DD" strings, durations are
// time.Duration, and nullable columns are pointers.
package model

import (
	"time"

	"github.com/google/uuid"
)

// Segment kinds (segments.kind).
const (
	KindWork        = "work"
	KindBreakAuto   = "break_auto"
	KindBreakManual = "break_manual"
)

// Segment sources (segments.source): what started the segment.
const (
	SourceUser     = "user"
	SourceActivity = "activity"
	SourceIdle     = "idle"
	SourceLock     = "lock"
	SourceSuspend  = "suspend"
	SourceRecovery = "recovery"
	SourceEdit     = "edit"
)

// Task statuses (tasks.status).
const (
	TaskOpen = "open"
	TaskDone = "done"
)

// Goal kinds (goals.kind) and statuses (goals.status).
const (
	GoalQuantity  = "quantity"
	GoalTasks     = "tasks"
	GoalActive    = "active"
	GoalDone      = "done"
	GoalAbandoned = "abandoned"
)

// Plan item statuses (plan_items.status).
const (
	PlanPlanned = "planned"
	PlanDone    = "done"
	PlanRolled  = "rolled"
	PlanSkipped = "skipped"
)

// DayLayout is the layout of every local calendar date column.
const DayLayout = "2006-01-02"

// IsBreakKind reports whether kind is one of the break kinds.
func IsBreakKind(kind string) bool {
	return kind == KindBreakAuto || kind == KindBreakManual
}

// ValidKind reports whether kind is a segment kind.
func ValidKind(kind string) bool {
	return kind == KindWork || IsBreakKind(kind)
}

// ValidSource reports whether source is a segment source.
func ValidSource(source string) bool {
	switch source {
	case SourceUser, SourceActivity, SourceIdle, SourceLock, SourceSuspend, SourceRecovery, SourceEdit:
		return true
	}
	return false
}

// NewID returns a new UUIDv7 in its 36-character lowercase canonical form.
func NewID() string {
	return uuid.Must(uuid.NewV7()).String()
}

// Envelope is the sync envelope ending every synced table.
type Envelope struct {
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
	DeviceID  string
	Rev       int64
}

// LocalState is a row of local_state.
type LocalState struct {
	Key       string
	Value     string
	UpdatedAt time.Time
}

// Project is a row of projects.
type Project struct {
	ID         string
	Name       string
	Color      string
	ArchivedAt *time.Time
	Envelope
}

// Task is a row of tasks.
type Task struct {
	ID        string
	ProjectID *string
	Title     string
	Notes     string
	Status    string
	Priority  int
	DueDay    *string
	Estimate  *time.Duration // estimate_minutes
	DoneAt    *time.Time
	// v2 columns. A task with an RRule is a template; its occurrences carry
	// TemplateID and OccurrenceDay.
	GoalID        *string
	Quantity      *int
	QuantityDone  *int
	RRule         *string
	TemplateID    *string
	OccurrenceDay *string
	Envelope
}

// IsTemplate reports whether the task is a recurring template.
func (t Task) IsTemplate() bool { return t.RRule != nil }

// WorkDay is a row of work_days.
type WorkDay struct {
	ID           string
	Day          string
	TZ           string
	ClockedInAt  time.Time
	ClockedOutAt *time.Time
	Target       time.Duration // target_seconds
	Note         string
	Envelope
}

// Segment is a row of segments.
type Segment struct {
	ID        string
	WorkDayID string
	Kind      string
	Source    string
	ProjectID *string
	TaskID    *string
	StartedAt time.Time
	EndedAt   *time.Time
	Truncated bool
	Envelope
}

// EngineEvent is a row of engine_events, the audit trail of state transitions.
type EngineEvent struct {
	ID        int64
	At        time.Time
	Trigger   string
	FromState string
	ToState   string
	Data      map[string]any
}

// Goal is a row of goals.
type Goal struct {
	ID             string
	Title          string
	Kind           string
	Unit           string
	TargetQuantity *int
	PerUnit        *time.Duration // minutes_per_unit
	ProjectID      *string
	StartDay       string
	DueDay         string
	Status         string
	Envelope
}

// Commitment is a row of commitments: recurring time that is not planned work.
type Commitment struct {
	ID                 string
	Title              string
	ProjectID          *string
	RRule              string
	StartMinute        *int          // minutes after local midnight; nil when floating
	Duration           time.Duration // duration_minutes
	CountsTowardTarget bool
	ActiveFrom         string
	ActiveUntil        *string
	Envelope
}

// PlanItem is a row of plan_items: one block of a task planned on a day.
type PlanItem struct {
	ID            string
	Day           string
	TaskID        string
	Planned       time.Duration // planned_minutes
	StartAt       *time.Time
	Position      int
	Status        string
	Pinned        bool
	RolledFromID  *string
	RolloverCount int
	Envelope
}

// LLM run kinds (llm_runs.kind) and statuses (llm_runs.status).
const (
	RunBreakdown = "breakdown"
	RunRetro     = "retro"
	RunOK        = "ok"
	RunFailed    = "failed"
	RunAccepted  = "accepted"
	RunRejected  = "rejected"
)

// LLMRun is a row of llm_runs: one LLM job and its JSON output.
type LLMRun struct {
	ID        string
	Kind      string
	SubjectID string
	Status    string
	Output    []byte // a JSON object
	CreatedAt time.Time
	UpdatedAt time.Time
}
