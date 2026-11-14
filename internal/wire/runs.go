package wire

import "encoding/json"

// Harness events (plan.md 3.10, W19).
const (
	// EventRunChanged carries RunChanged.
	EventRunChanged = "run_changed"
	// EventApprovalRequested and EventApprovalDecided carry an Approval.
	EventApprovalRequested = "approval_requested"
	EventApprovalDecided   = "approval_decided"
	// EventConnectionChanged carries ConnectionChanged.
	EventConnectionChanged = "connection_changed"
)

// Run is one piece of work Gwen does with you (plan.md 3.7). Status is
// draft, running, waiting, done, failed, or cancelled; Reason says why it
// waits or failed: queued, approval, interrupted, or what went wrong.
type Run struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	Title   string `json:"title"`
	Brief   string `json:"brief"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	// ExternalRef ties it to what started it, e.g. "chat:<thread id>".
	ExternalRef string          `json:"external_ref"`
	Result      json.RawMessage `json:"result" ts_type:"any"`
	CreatedAt   int64           `json:"created_at"`
	UpdatedAt   int64           `json:"updated_at"`
	StartedAt   *int64          `json:"started_at"`
	EndedAt     *int64          `json:"ended_at"`
}

// RunStep is one step of a run. Status is pending, running, waiting, done,
// failed, or skipped.
type RunStep struct {
	ID         string          `json:"id"`
	Position   int             `json:"position"`
	Title      string          `json:"title"`
	Capability string          `json:"capability"`
	Risk       string          `json:"risk"`
	Status     string          `json:"status"`
	Input      json.RawMessage `json:"input" ts_type:"any"`
	Output     json.RawMessage `json:"output" ts_type:"any"`
	Error      string          `json:"error"`
	StartedAt  *int64          `json:"started_at"`
	EndedAt    *int64          `json:"ended_at"`
}

// RunEvent is one line of a run's log.
type RunEvent struct {
	Seq    int64           `json:"seq"`
	StepID string          `json:"step_id"`
	Kind   string          `json:"kind"`
	Data   json.RawMessage `json:"data" ts_type:"any"`
	At     int64           `json:"at"`
}

// RunDetail is GET /v1/runs/{id}: the run, its steps, its newest log lines,
// and its approvals.
type RunDetail struct {
	Run       Run        `json:"run"`
	Steps     []RunStep  `json:"steps"`
	Log       []RunEvent `json:"log"`
	Approvals []Approval `json:"approvals"`
}

// RunList is GET /v1/runs.
type RunList struct {
	Runs []Run `json:"runs"`
}

// CreateRunRequest is POST /v1/runs. Start true starts it at once.
type CreateRunRequest struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject,omitempty"`
	Title   string `json:"title"`
	Brief   string `json:"brief,omitempty"`
	Start   bool   `json:"start,omitempty"`
}

// RunMessageRequest is POST /v1/runs/{id}/message: your reply to a run.
type RunMessageRequest struct {
	Text string `json:"text"`
}

// RunChanged is the data of run_changed.
type RunChanged struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Status      string `json:"status"`
	Reason      string `json:"reason"`
	ExternalRef string `json:"external_ref"`
}

// Approval is a step waiting for your decision (plan.md 3.7): Summary says
// in a sentence what it will do and Payload exactly what. It belongs to a run
// (RunID) or, from W32, to a chat (ThreadID); the other is "". GrantScopes
// are the scopes Remember may pick, which make a grant for Capability and
// Pattern. Status is pending, approved, rejected, or expired.
type Approval struct {
	ID          string          `json:"id"`
	RunID       string          `json:"run_id"`
	RunTitle    string          `json:"run_title"`
	ThreadID    string          `json:"thread_id"`
	StepID      string          `json:"step_id"`
	Capability  string          `json:"capability"`
	Risk        string          `json:"risk"`
	Pattern     string          `json:"pattern"`
	Summary     string          `json:"summary"`
	Payload     json.RawMessage `json:"payload" ts_type:"any"`
	GrantScopes []string        `json:"grant_scopes"`
	Status      string          `json:"status"`
	CreatedAt   int64           `json:"created_at"`
	DecidedAt   *int64          `json:"decided_at"`
	// ExpiresAt is when a pending approval lapses.
	ExpiresAt int64 `json:"expires_at"`
}

// ApprovalList is GET /v1/approvals.
type ApprovalList struct {
	Approvals []Approval `json:"approvals"`
}

// DecideApprovalRequest is POST /v1/approvals/{id}: Decision is approve or
// reject; Payload, when given, is the payload as you edited it. Remember,
// with approve, is one of the approval's grant scopes: it also makes an allow
// grant for that scope, never for an external or destructive risk.
type DecideApprovalRequest struct {
	Decision string          `json:"decision"`
	Payload  json.RawMessage `json:"payload,omitempty" ts_type:"any"`
	Remember string          `json:"remember,omitempty"`
}

// Grant lets a capability (with Pattern, or any input when "") go ahead
// without asking, or makes it ask, for one Scope: a subject ref, a folder,
// project:<id>, or chat:<id>. Decision is allow or ask.
type Grant struct {
	ID         string `json:"id"`
	Capability string `json:"capability"`
	Pattern    string `json:"pattern"`
	Scope      string `json:"scope"`
	Decision   string `json:"decision"`
	CreatedAt  int64  `json:"created_at"`
	ExpiresAt  *int64 `json:"expires_at"`
}

// GrantList is GET /v1/grants.
type GrantList struct {
	Grants []Grant `json:"grants"`
}

// Resource is a link, folder, file, or note attached to a subject.
type Resource struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
	Value string `json:"value"`
}

// ResourceList is GET and PUT /v1/resources?subject=.
type ResourceList struct {
	Subject   string     `json:"subject"`
	Resources []Resource `json:"resources"`
}

// PutResourcesRequest replaces a subject's resources, in order.
type PutResourcesRequest struct {
	Resources []NewResource `json:"resources"`
}

// NewResource is one resource of PutResourcesRequest.
type NewResource struct {
	Kind  string `json:"kind"`
	Title string `json:"title,omitempty"`
	Value string `json:"value"`
}

// RoutineStep is one "when it starts" step: a capability and its input.
type RoutineStep struct {
	Title      string          `json:"title,omitempty"`
	Capability string          `json:"capability"`
	Input      json.RawMessage `json:"input" ts_type:"any"`
}

// Routine is GET and PUT /v1/routines?subject=. Inherited: the steps are
// the parent's (a task's template, an event's series), since the subject has
// none of its own.
type Routine struct {
	Subject   string        `json:"subject"`
	Steps     []RoutineStep `json:"steps"`
	AutoStart bool          `json:"auto_start"`
	Inherited bool          `json:"inherited"`
}

// PutRoutineRequest sets a subject's routine; no steps removes it.
type PutRoutineRequest struct {
	Steps     []RoutineStep `json:"steps"`
	AutoStart bool          `json:"auto_start"`
}

// ProjectDir is GET and PUT /v1/projects/{id}/dir: the project's folder on
// this machine and its test command.
type ProjectDir struct {
	Path        string `json:"path"`
	TestCommand string `json:"test_command"`
}

// Capability is one thing a connection can do, with its risk.
type Capability struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	Risk  string `json:"risk"`
	Input string `json:"input"`
}

// Connection is one connector: State is connected, needs_setup, or error,
// with Message saying why. Linkable: it can be connected and disconnected
// from the dashboard.
type Connection struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	State        string       `json:"state"`
	Message      string       `json:"message"`
	Linkable     bool         `json:"linkable"`
	Capabilities []Capability `json:"capabilities"`
}

// ConnectionList is GET /v1/connections.
type ConnectionList struct {
	Connections []Connection `json:"connections"`
}

// ConnectionChanged is the data of connection_changed.
type ConnectionChanged struct {
	ID string `json:"id"`
}
