package wire

import "encoding/json"

// Event names on GET /v1/events (docs/04-api-contract.md#events).
const (
	EventStateChanged    = "state_changed"
	EventDayChanged      = "day_changed"
	EventNudgeFired      = "nudge_fired"
	EventProjectsChanged = "projects_changed"
	EventTasksChanged    = "tasks_changed"
	EventConfigChanged   = "config_changed"
)

// Event is one Server-Sent Events frame. Data is the frame's JSON, undecoded.
type Event struct {
	ID   int64           `json:"id"`
	Name string          `json:"name"`
	Data json.RawMessage `json:"data" ts_type:"any"`
}

// DayChanged is the data of day_changed.
type DayChanged struct {
	Day string `json:"day"`
}

// NudgeFired is the data of nudge_fired.
type NudgeFired struct {
	Kind  string `json:"kind"`
	At    int64  `json:"at"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

// ProjectsChanged is the data of projects_changed.
type ProjectsChanged struct{}

// TasksChanged is the data of tasks_changed.
type TasksChanged struct {
	TaskIDs []string `json:"task_ids"`
}
