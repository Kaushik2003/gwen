package wire

// Request bodies. The server decodes them with DisallowUnknownFields. Commands
// with no parameters take {} and have no type here.
//
// In create bodies a nullable field is a pointer, and absent means null. In
// PATCH bodies every field is optional: a nullable column is an Optional, so
// that null can clear it, and any other field is a pointer, nil when absent.

// ClockInRequest is the POST /v1/day/clock-in body. Both fields nullable.
type ClockInRequest struct {
	ProjectID *string `json:"project_id"`
	TaskID    *string `json:"task_id"`
}

// SwitchRequest is the POST /v1/session/switch body. Both fields nullable.
type SwitchRequest struct {
	ProjectID *string `json:"project_id"`
	TaskID    *string `json:"task_id"`
}

// PatchDayRequest is the PATCH /v1/days/{day} body.
type PatchDayRequest struct {
	TargetSeconds *int    `json:"target_seconds,omitempty"`
	Note          *string `json:"note,omitempty"`
	Rev           *int64  `json:"rev,omitempty"`
}

// CreateSegmentRequest is the POST /v1/segments body. EndedAt is required; it
// is a pointer so that a missing value can be reported as invalid_request.
type CreateSegmentRequest struct {
	Day       string  `json:"day"`
	Kind      string  `json:"kind"`
	ProjectID *string `json:"project_id"`
	TaskID    *string `json:"task_id"`
	StartedAt int64   `json:"started_at"`
	EndedAt   *int64  `json:"ended_at"`
}

// PatchSegmentRequest is the PATCH /v1/segments/{id} body.
type PatchSegmentRequest struct {
	Kind      *string          `json:"kind,omitempty"`
	ProjectID Optional[string] `json:"project_id,omitzero" ts_type:"string | null"`
	TaskID    Optional[string] `json:"task_id,omitzero" ts_type:"string | null"`
	StartedAt *int64           `json:"started_at,omitempty"`
	EndedAt   *int64           `json:"ended_at,omitempty"`
	Rev       *int64           `json:"rev,omitempty"`
}

// SplitSegmentRequest is the POST /v1/segments/{id}/split body.
type SplitSegmentRequest struct {
	At int64 `json:"at"`
}

// CreateProjectRequest is the POST /v1/projects body. A nil Color asks the
// daemon to pick one from the palette.
type CreateProjectRequest struct {
	Name  string  `json:"name"`
	Color *string `json:"color,omitempty"`
}

// PatchProjectRequest is the PATCH /v1/projects/{id} body.
type PatchProjectRequest struct {
	Name     *string `json:"name,omitempty"`
	Color    *string `json:"color,omitempty"`
	Archived *bool   `json:"archived,omitempty"`
	Rev      *int64  `json:"rev,omitempty"`
}

// CreateTaskRequest is the POST /v1/tasks body. Nil Notes and Priority take the
// defaults "" and 2.
type CreateTaskRequest struct {
	ProjectID       *string `json:"project_id"`
	Title           string  `json:"title"`
	Notes           *string `json:"notes,omitempty"`
	Priority        *int    `json:"priority,omitempty"`
	DueDay          *string `json:"due_day"`
	EstimateMinutes *int    `json:"estimate_minutes"`
	GoalID          *string `json:"goal_id,omitempty"`
	Quantity        *int    `json:"quantity,omitempty"`
	RRule           *string `json:"rrule,omitempty"`
}

// PatchTaskRequest is the PATCH /v1/tasks/{id} body.
type PatchTaskRequest struct {
	ProjectID       Optional[string] `json:"project_id,omitzero" ts_type:"string | null"`
	Title           *string          `json:"title,omitempty"`
	Notes           *string          `json:"notes,omitempty"`
	Priority        *int             `json:"priority,omitempty"`
	DueDay          Optional[string] `json:"due_day,omitzero" ts_type:"string | null"`
	EstimateMinutes Optional[int]    `json:"estimate_minutes,omitzero" ts_type:"number | null"`
	GoalID          Optional[string] `json:"goal_id,omitzero" ts_type:"string | null"`
	Quantity        Optional[int]    `json:"quantity,omitzero" ts_type:"number | null"`
	RRule           Optional[string] `json:"rrule,omitzero" ts_type:"string | null"`
	Rev             *int64           `json:"rev,omitempty"`
}

// CompleteTaskRequest is the POST /v1/tasks/{id}/complete body. A nil
// QuantityDone records the task's quantity.
type CompleteTaskRequest struct {
	QuantityDone *int `json:"quantity_done,omitempty"`
}

// ConfigPatch is the PATCH /v1/config body: a partial Config, nested, as
// section name → key → JSON value, e.g. {"tracking": {"soft_idle": "10s"}}.
type ConfigPatch map[string]map[string]any

// TaskQuery is the query of GET /v1/tasks. Zero values are the defaults: any
// project and goal, status "open", no due bound, templates excluded.
type TaskQuery struct {
	ProjectID string `json:"project_id"`
	GoalID    string `json:"goal_id"`
	Status    string `json:"status"`     // "open", "done", or "all"
	DueBefore string `json:"due_before"` // exclusive
	Templates bool   `json:"templates"`  // list templates alongside the other tasks
}

// Values of the archived query parameter of GET /v1/projects.
const (
	ArchivedFalse = "false"
	ArchivedTrue  = "true"
	ArchivedAll   = "all"
)

// CreateGoalRequest is the POST /v1/goals body. A nil Unit takes "".
type CreateGoalRequest struct {
	Title          string  `json:"title"`
	Kind           string  `json:"kind"`
	Unit           *string `json:"unit,omitempty"`
	TargetQuantity *int    `json:"target_quantity"`
	MinutesPerUnit *int    `json:"minutes_per_unit"`
	ProjectID      *string `json:"project_id"`
	StartDay       string  `json:"start_day"`
	DueDay         string  `json:"due_day"`
}

// PatchGoalRequest is the PATCH /v1/goals/{id} body. Kind cannot change.
type PatchGoalRequest struct {
	Title          *string          `json:"title,omitempty"`
	Kind           *string          `json:"kind,omitempty"`
	Unit           *string          `json:"unit,omitempty"`
	TargetQuantity Optional[int]    `json:"target_quantity,omitzero" ts_type:"number | null"`
	MinutesPerUnit Optional[int]    `json:"minutes_per_unit,omitzero" ts_type:"number | null"`
	ProjectID      Optional[string] `json:"project_id,omitzero" ts_type:"string | null"`
	StartDay       *string          `json:"start_day,omitempty"`
	DueDay         *string          `json:"due_day,omitempty"`
	Status         *string          `json:"status,omitempty"`
	Rev            *int64           `json:"rev,omitempty"`
}

// CreateCommitmentRequest is the POST /v1/commitments body. A nil
// CountsTowardTarget takes true.
type CreateCommitmentRequest struct {
	Title              string  `json:"title"`
	ProjectID          *string `json:"project_id"`
	RRule              string  `json:"rrule"`
	StartMinute        *int    `json:"start_minute"`
	DurationMinutes    int     `json:"duration_minutes"`
	CountsTowardTarget *bool   `json:"counts_toward_target,omitempty"`
	ActiveFrom         string  `json:"active_from"`
	ActiveUntil        *string `json:"active_until"`
}

// PatchCommitmentRequest is the PATCH /v1/commitments/{id} body.
type PatchCommitmentRequest struct {
	Title              *string          `json:"title,omitempty"`
	ProjectID          Optional[string] `json:"project_id,omitzero" ts_type:"string | null"`
	RRule              *string          `json:"rrule,omitempty"`
	StartMinute        Optional[int]    `json:"start_minute,omitzero" ts_type:"number | null"`
	DurationMinutes    *int             `json:"duration_minutes,omitempty"`
	CountsTowardTarget *bool            `json:"counts_toward_target,omitempty"`
	ActiveFrom         *string          `json:"active_from,omitempty"`
	ActiveUntil        Optional[string] `json:"active_until,omitzero" ts_type:"string | null"`
	Rev                *int64           `json:"rev,omitempty"`
}

// GeneratePlanRequest is the POST /v1/plan/generate body.
type GeneratePlanRequest struct {
	Day string `json:"day"`
}

// PatchPlanItemRequest is the PATCH /v1/plan/items/{id} body. Status is only
// planned or skipped. Changing StartAt, Position, or PlannedMinutes pins the
// item.
type PatchPlanItemRequest struct {
	StartAt        Optional[int64] `json:"start_at,omitzero" ts_type:"number | null"`
	Position       *int            `json:"position,omitempty"`
	Pinned         *bool           `json:"pinned,omitempty"`
	PlannedMinutes *int            `json:"planned_minutes,omitempty"`
	Status         *string         `json:"status,omitempty"`
	Rev            *int64          `json:"rev,omitempty"`
}
