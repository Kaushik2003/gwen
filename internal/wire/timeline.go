package wire

// The agentic timeline (plan.md 3.17): a day's plan as a graph of what
// waits on what, where it is heading, and what the agents did.

// Graph task states, worst first after done.
const (
	GraphDone         = "done"
	GraphWaitingOnYou = "waiting_on_you" // a Claude session on it asks you something
	GraphBlocked      = "blocked"        // it waits on open work that won't be done in time today
	GraphAtRisk       = "at_risk"        // expected to end well after its plan
	GraphWorking      = "working"        // you are tracking it now
	GraphAgent        = "agent"          // a Claude session is working on it
	GraphReady        = "ready"          // on the plan, nothing in its way
	GraphUnplanned    = "unplanned"      // open, not on the day's plan
)

// DayGraph is the GET /v1/days/{day}/graph response.
type DayGraph struct {
	Day string `json:"day"`
	Now int64  `json:"now"`
	// From and To are the planner's day, as instants.
	From  int64       `json:"from"`
	To    int64       `json:"to"`
	Tasks []GraphTask `json:"tasks"`
	// Agents are the Claude sessions active on the day, with what state
	// they were in when.
	Agents []AgentLane `json:"agents"`
	// Issues are what needs a look, most pressing first.
	Issues []GraphIssue `json:"issues"`
	// Critical is the chain of waiting that ends last on the day, first to
	// last; empty when no task waits on another.
	Critical []string `json:"critical"`
}

// GraphTask is a task on the day: on its plan, waited on by something that
// is, or worked on by an agent.
type GraphTask struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	ProjectID       *string `json:"project_id"`
	Status          string  `json:"status"`
	Stage           string  `json:"stage"`
	DelegatedTo     string  `json:"delegated_to"`
	EstimateMinutes *int    `json:"estimate_minutes"`
	TrackedMs       int64   `json:"tracked_ms"`
	Rev             int64   `json:"rev"`
	// WaitsOn and Blocks are the tasks it waits on and that wait on it,
	// among Tasks.
	WaitsOn []string `json:"waits_on"`
	Blocks  []string `json:"blocks"`
	// Planned are its blocks on the day, by start.
	Planned      []GraphBlock `json:"planned"`
	PlannedStart *int64       `json:"planned_start"`
	PlannedEnd   *int64       `json:"planned_end"`
	// ProjectedStart and ProjectedEnd are where it is heading now, from
	// what is tracked and what it waits on; nil when that can't be said.
	ProjectedStart *int64 `json:"projected_start"`
	ProjectedEnd   *int64 `json:"projected_end"`
	// SlipMinutes is how far the projected end is past the planned one.
	SlipMinutes int    `json:"slip_minutes"`
	State       string `json:"state"`
	// Reason says why it is in its state, in a sentence; "" when ready.
	Reason string `json:"reason"`
	// SessionID is the Claude session working on it, "" for none.
	SessionID string `json:"session_id"`
}

// GraphBlock is one planned block of a task.
type GraphBlock struct {
	ItemID  string `json:"item_id"`
	StartAt *int64 `json:"start_at"`
	Minutes int    `json:"minutes"`
	Status  string `json:"status"`
	Pinned  bool   `json:"pinned"`
}

// AgentLane is a Claude session over the day.
type AgentLane struct {
	SessionID string  `json:"session_id"`
	Name      string  `json:"name"`
	Cwd       string  `json:"cwd"`
	ProjectID *string `json:"project_id"`
	TaskID    string  `json:"task_id"`
	// State and Need are its state now; Question is what it asks, when it
	// needs you.
	State    string      `json:"state"`
	Need     string      `json:"need"`
	Question string      `json:"question"`
	Spans    []AgentSpan `json:"spans"`
}

// AgentSpan is a stretch a session spent in one state; EndAt is nil while
// it is still in it.
type AgentSpan struct {
	State   string `json:"state"`
	Need    string `json:"need"`
	TaskID  string `json:"task_id"`
	StartAt int64  `json:"start_at"`
	EndAt   *int64 `json:"end_at"`
}

// GraphIssue is one thing on the day that needs a look.
type GraphIssue struct {
	// Kind is waiting_on_you, blocked, or at_risk.
	Kind   string `json:"kind"`
	TaskID string `json:"task_id"`
	// OtherID is the task in its way, "" for none.
	OtherID   string `json:"other_id"`
	SessionID string `json:"session_id"`
	Text      string `json:"text"`
}
