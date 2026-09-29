package wire

import "encoding/json"

// LLM run kinds and statuses.
const (
	RunBreakdown = "breakdown"
	RunRetro     = "retro"
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

// RunError is a failed run's Output.
type RunError struct {
	Error string `json:"error"`
}

// BreakdownRequest is the POST /v1/goals/{id}/breakdown body.
type BreakdownRequest struct {
	Instructions string `json:"instructions"`
}

// RetroRequest is the POST /v1/retro body.
type RetroRequest struct {
	WeekStart string `json:"week_start"`
}

// AcceptRunRequest is the POST /v1/llm/runs/{id}/accept body.
type AcceptRunRequest struct {
	Indexes []int `json:"indexes"`
}
