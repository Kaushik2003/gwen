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
}

// PatchTaskRequest is the PATCH /v1/tasks/{id} body.
type PatchTaskRequest struct {
	ProjectID       Optional[string] `json:"project_id,omitzero" ts_type:"string | null"`
	Title           *string          `json:"title,omitempty"`
	Notes           *string          `json:"notes,omitempty"`
	Priority        *int             `json:"priority,omitempty"`
	DueDay          Optional[string] `json:"due_day,omitzero" ts_type:"string | null"`
	EstimateMinutes Optional[int]    `json:"estimate_minutes,omitzero" ts_type:"number | null"`
	Rev             *int64           `json:"rev,omitempty"`
}

// CompleteTaskRequest is the POST /v1/tasks/{id}/complete body.
type CompleteTaskRequest struct{}

// ConfigPatch is the PATCH /v1/config body: a partial Config, nested, as
// section name → key → JSON value, e.g. {"tracking": {"soft_idle": "10s"}}.
type ConfigPatch map[string]map[string]any

// TaskQuery is the query of GET /v1/tasks. Zero values are the defaults: any
// project, status "open", no due bound.
type TaskQuery struct {
	ProjectID string `json:"project_id"`
	Status    string `json:"status"`     // "open", "done", or "all"
	DueBefore string `json:"due_before"` // exclusive
}

// Values of the archived query parameter of GET /v1/projects.
const (
	ArchivedFalse = "false"
	ArchivedTrue  = "true"
	ArchivedAll   = "all"
)
