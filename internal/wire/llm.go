package wire

import "encoding/json"

// LLM run kinds and statuses.
const (
	RunBreakdown = "breakdown"
	RunRetro     = "retro"
	RunDayPlan   = "day_plan"
	RunAssistant = "assistant"
	RunOK        = "ok"
	RunFailed    = "failed"
	RunAccepted  = "accepted"
	RunRejected  = "rejected"
)

// LlmRun is an llm_runs row. Output is a JSON object whose shape depends on
// Kind (docs/07-integrations.md#llm-adapter).
type LlmRun struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	SubjectID string          `json:"subject_id"`
	Status    string          `json:"status"`
	Output    json.RawMessage `json:"output" ts_type:"any"`
	CreatedAt int64           `json:"created_at"`
	UpdatedAt int64           `json:"updated_at"`
}

// BreakdownOutput is an ok breakdown's Output.
type BreakdownOutput struct {
	Tasks []ProposedTask `json:"tasks"`
}

// ProposedTask is one task of a breakdown.
type ProposedTask struct {
	Title           string `json:"title"`
	Notes           string `json:"notes"`
	EstimateMinutes int    `json:"estimate_minutes"`
	DueDay          string `json:"due_day"`
	Priority        int    `json:"priority"`
	Quantity        *int   `json:"quantity"`
}

// RetroOutput is a retro's Output; GeneratedBy is "llm" or "rules".
type RetroOutput struct {
	Markdown    string `json:"markdown"`
	GeneratedBy string `json:"generated_by"`
}

// DayPlanOutput is an ok day plan's Output: the conversation including the
// reply, the proposed blocks, and the change of hours, nil for none
// (docs/07-integrations.md#day-plan).
type DayPlanOutput struct {
	Messages []ChatMessage   `json:"messages"`
	Items    []ProposedBlock `json:"items"`
	Hours    *DayHours       `json:"hours"`
}

// ChatMessage is a turn of a day plan conversation; Role is "user" or
// "assistant".
type ChatMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// ProposedBlock is a block of a day plan.
type ProposedBlock struct {
	TaskID         string  `json:"task_id"`
	Title          string  `json:"title"`
	ProjectID      *string `json:"project_id"`
	StartAt        int64   `json:"start_at"`
	PlannedMinutes int     `json:"planned_minutes"`
}

// PlanChatRequest is the POST /v1/plan/chat body; RunID continues a
// conversation, nil starts one.
type PlanChatRequest struct {
	Day     string  `json:"day"`
	Message string  `json:"message"`
	RunID   *string `json:"run_id"`
}

// RunError is a failed run's Output.
type RunError struct {
	Error string `json:"error"`
}

// BreakdownRequest is the POST /v1/goals/{id}/breakdown body.
type BreakdownRequest struct {
	Instructions string `json:"instructions"`
	// Sessions is how many upcoming sessions a quantity goal's breakdown
	// fills, 1 to 14; 0 means 7.
	Sessions int `json:"sessions,omitempty"`
}

// RetroRequest is the POST /v1/retro body.
type RetroRequest struct {
	WeekStart string `json:"week_start"`
}

// AcceptRunRequest is the POST /v1/llm/runs/{id}/accept body.
type AcceptRunRequest struct {
	Indexes []int `json:"indexes"`
}
