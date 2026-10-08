package wire

// Engine states, the values of Status.State (docs/05-time-engine.md#states).
const (
	StateOff         = "off"
	StateWorking     = "working"
	StateIdlePending = "idle_pending"
	StateBreakAuto   = "break_auto"
	StateBreakManual = "break_manual"
)

// How a ProjectTotal renders time with no project.
const (
	UnassignedName  = "Unassigned"
	UnassignedColor = "#6b7280"
)

// Status is the engine snapshot.
type Status struct {
	State           string      `json:"state"`
	StateSinceAt    int64       `json:"state_since_at"`
	WorkDay         *WorkDay    `json:"work_day"`
	OpenSegment     *Segment    `json:"open_segment"`
	ProjectID       *string     `json:"project_id"`
	TaskID          *string     `json:"task_id"`
	IdleSinceAt     *int64      `json:"idle_since_at"`
	SnoozedUntilAt  *int64      `json:"snoozed_until_at"`
	Today           *DaySummary `json:"today"`
	ServerNowAt     int64       `json:"server_now_at"`
	ActivityBackend string      `json:"activity_backend"`
	Warnings        []string    `json:"warnings"`
}

// WorkDay is a work_days row.
type WorkDay struct {
	ID            string `json:"id"`
	Day           string `json:"day"`
	TZ            string `json:"tz"`
	ClockedInAt   int64  `json:"clocked_in_at"`
	ClockedOutAt  *int64 `json:"clocked_out_at"`
	TargetSeconds int    `json:"target_seconds"`
	Note          string `json:"note"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
	Rev           int64  `json:"rev"`
}

// Segment is a segments row.
type Segment struct {
	ID        string  `json:"id"`
	WorkDayID string  `json:"work_day_id"`
	Kind      string  `json:"kind"`
	Source    string  `json:"source"`
	ProjectID *string `json:"project_id"`
	TaskID    *string `json:"task_id"`
	StartedAt int64   `json:"started_at"`
	EndedAt   *int64  `json:"ended_at"`
	Truncated bool    `json:"truncated"`
	CreatedAt int64   `json:"created_at"`
	UpdatedAt int64   `json:"updated_at"`
	Rev       int64   `json:"rev"`
}

// DaySummary is the totals for one work day.
type DaySummary struct {
	Day           string         `json:"day"`
	TargetSeconds int            `json:"target_seconds"`
	WorkedMs      int64          `json:"worked_ms"`
	BreakMs       int64          `json:"break_ms"`
	TargetMet     bool           `json:"target_met"`
	ByProject     []ProjectTotal `json:"by_project"`
}

// ProjectTotal is worked time on one project; ProjectID is null for unassigned.
type ProjectTotal struct {
	ProjectID *string `json:"project_id"`
	Name      string  `json:"name"`
	Color     string  `json:"color"`
	WorkedMs  int64   `json:"worked_ms"`
}

// Project is a projects row.
type Project struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Color      string `json:"color"`
	ArchivedAt *int64 `json:"archived_at"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
	Rev        int64  `json:"rev"`
}

// Task is a tasks row plus the computed TrackedMs.
type Task struct {
	ID              string  `json:"id"`
	ProjectID       *string `json:"project_id"`
	Title           string  `json:"title"`
	Notes           string  `json:"notes"`
	Status          string  `json:"status"`
	Priority        int     `json:"priority"`
	DueDay          *string `json:"due_day"`
	EstimateMinutes *int    `json:"estimate_minutes"`
	DoneAt          *int64  `json:"done_at"`
	CreatedAt       int64   `json:"created_at"`
	UpdatedAt       int64   `json:"updated_at"`
	Rev             int64   `json:"rev"`
	TrackedMs       int64   `json:"tracked_ms"`
	GoalID          *string `json:"goal_id"`
	Quantity        *int    `json:"quantity"`
	QuantityDone    *int    `json:"quantity_done"`
	RRule           *string `json:"rrule"`
	TemplateID      *string `json:"template_id"`
	OccurrenceDay   *string `json:"occurrence_day"`
	ParentID        *string `json:"parent_id"`
	StartDay        *string `json:"start_day"`
	StartMinute     *int    `json:"start_minute"`
	Stage           string  `json:"stage"`
	Effort          *int    `json:"effort"`
	DelegatedTo     string  `json:"delegated_to"`
}

// Task stages and efforts.
const (
	StageInbox   = "inbox"
	StageTodo    = "todo"
	StageDoing   = "doing"
	StageWaiting = "waiting"
	StageSomeday = "someday"
)

// Health is the GET /v1/health response.
type Health struct {
	OK            bool   `json:"ok"`
	Version       string `json:"version"`
	SchemaVersion int64  `json:"schema_version"`
	PID           int    `json:"pid"`
}

// DayList is the GET /v1/days response.
type DayList struct {
	Days []DaySummary `json:"days"`
}

// DayDetail is the GET /v1/days/{day} response; Segments are by started_at.
type DayDetail struct {
	WorkDay  WorkDay    `json:"work_day"`
	Summary  DaySummary `json:"summary"`
	Segments []Segment  `json:"segments"`
}

// SegmentList is the POST /v1/segments/{id}/split response.
type SegmentList struct {
	Segments []Segment `json:"segments"`
}

// ProjectList is the GET /v1/projects response.
type ProjectList struct {
	Projects []Project `json:"projects"`
}

// TaskList is the GET /v1/tasks response.
type TaskList struct {
	Tasks []Task `json:"tasks"`
}

// StatsSummary is the GET /v1/stats/summary response. Streaks are all-time.
type StatsSummary struct {
	From          string         `json:"from"`
	To            string         `json:"to"`
	WorkedMs      int64          `json:"worked_ms"`
	BreakMs       int64          `json:"break_ms"`
	DaysTracked   int            `json:"days_tracked"`
	DaysTargetMet int            `json:"days_target_met"`
	AvgWorkedMs   int64          `json:"avg_worked_ms"`
	ByProject     []ProjectTotal `json:"by_project"`
	CurrentStreak int            `json:"current_streak"`
	LongestStreak int            `json:"longest_streak"`
}

// Heatmap is the GET /v1/stats/heatmap response.
type Heatmap struct {
	Year int          `json:"year"`
	Days []HeatmapDay `json:"days"`
}

// HeatmapDay is worked time on one day with worked_ms > 0.
type HeatmapDay struct {
	Day      string `json:"day"`
	WorkedMs int64  `json:"worked_ms"`
}

// Values of the NotifyTestResult fields other than a backend's error message.
const (
	NotifyOK       = "ok"
	NotifyDisabled = "disabled"
)

// NotifyTestResult is the POST /v1/notify/test response.
type NotifyTestResult struct {
	Desktop string `json:"desktop"`
	Phone   string `json:"phone"`
}

// Config mirrors config.toml (docs/03-data-model.md#configuration). Durations
// and HH:MM times stay strings. It is also the string form internal/config
// reads and writes, hence the toml tags.
type Config struct {
	Tracking TrackingConfig `json:"tracking" toml:"tracking"`
	Nudge    NudgeConfig    `json:"nudge" toml:"nudge"`
	Ntfy     NtfyConfig     `json:"ntfy" toml:"ntfy"`
	Planner  PlannerConfig  `json:"planner" toml:"planner"`
	Calendar CalendarConfig `json:"calendar" toml:"calendar"`
	LLM      LLMConfig      `json:"llm" toml:"llm"`
	Sync     SyncConfig     `json:"sync" toml:"sync"`
	Log      LogConfig      `json:"log" toml:"log"`
}

// TrackingConfig is the [tracking] section.
type TrackingConfig struct {
	DailyTarget string `json:"daily_target" toml:"daily_target"`
	SoftIdle    string `json:"soft_idle" toml:"soft_idle"`
	HardIdle    string `json:"hard_idle" toml:"hard_idle"`
	DayRollover string `json:"day_rollover" toml:"day_rollover"`
}

// NudgeConfig is the [nudge] section.
type NudgeConfig struct {
	Desktop       bool   `json:"desktop" toml:"desktop"`
	Phone         bool   `json:"phone" toml:"phone"`
	BreakReminder string `json:"break_reminder" toml:"break_reminder"`
	Repeat        string `json:"repeat" toml:"repeat"`
	Snooze        string `json:"snooze" toml:"snooze"`
}

// NtfyConfig is the [ntfy] section.
type NtfyConfig struct {
	Server         string `json:"server" toml:"server"`
	FallbackServer string `json:"fallback_server" toml:"fallback_server"`
	Topic          string `json:"topic" toml:"topic"`
}

// PlannerConfig is the [planner] section.
type PlannerConfig struct {
	DayStart   string `json:"day_start" toml:"day_start"`
	DayEnd     string `json:"day_end" toml:"day_end"`
	Buffer     string `json:"buffer" toml:"buffer"`
	EatTheFrog bool   `json:"eat_the_frog" toml:"eat_the_frog"`
	PrimeStart string `json:"prime_start" toml:"prime_start"` // HH:MM, or "" for none
	PrimeEnd   string `json:"prime_end" toml:"prime_end"`
}

// CalendarConfig is the [calendar] section.
type CalendarConfig struct {
	Enabled       bool     `json:"enabled" toml:"enabled"`
	Name          string   `json:"name" toml:"name"`
	BusyCalendars []string `json:"busy_calendars" toml:"busy_calendars"`
}

// LLMConfig is the [llm] section.
type LLMConfig struct {
	Provider string `json:"provider" toml:"provider"`
	Model    string `json:"model" toml:"model"`
	Endpoint string `json:"endpoint" toml:"endpoint"`
	Command  string `json:"command" toml:"command"`
	Timeout  string `json:"timeout" toml:"timeout"`
	// The assistant's name, personality preset, and the user's own words
	// added to its prompt.
	AssistantName string `json:"assistant_name" toml:"assistant_name"`
	Personality   string `json:"personality" toml:"personality"`
	Instructions  string `json:"instructions" toml:"instructions"`
}

// SyncConfig is the [sync] section.
type SyncConfig struct {
	HubURL   string `json:"hub_url" toml:"hub_url"`
	Interval string `json:"interval" toml:"interval"`
}

// LogConfig is the [log] section.
type LogConfig struct {
	Level string `json:"level" toml:"level"`
}

// Goal kinds and statuses.
const (
	GoalQuantity  = "quantity"
	GoalTasks     = "tasks"
	GoalActive    = "active"
	GoalDone      = "done"
	GoalAbandoned = "abandoned"
	GoalsAll      = "all"
)

// Goal pace values.
const (
	PaceAhead   = "ahead"
	PaceOnTrack = "on_track"
	PaceBehind  = "behind"
)

// Plan item statuses.
const (
	PlanPlanned = "planned"
	PlanDone    = "done"
	PlanRolled  = "rolled"
	PlanSkipped = "skipped"
)

// Goal is a goals row with its progress today.
type Goal struct {
	ID             string       `json:"id"`
	Title          string       `json:"title"`
	Kind           string       `json:"kind"`
	Unit           string       `json:"unit"`
	TargetQuantity *int         `json:"target_quantity"`
	MinutesPerUnit *int         `json:"minutes_per_unit"`
	DailyMinutes   *int         `json:"daily_minutes"`
	ProjectID      *string      `json:"project_id"`
	StartDay       string       `json:"start_day"`
	DueDay         string       `json:"due_day"`
	Status         string       `json:"status"`
	Specific       string       `json:"specific"`
	Measurable     string       `json:"measurable"`
	Assignable     string       `json:"assignable"`
	Realistic      string       `json:"realistic"`
	CreatedAt      int64        `json:"created_at"`
	UpdatedAt      int64        `json:"updated_at"`
	Rev            int64        `json:"rev"`
	Progress       GoalProgress `json:"progress"`
}

// GoalProgress is a goal's progress today. For a tasks goal the quantities
// count tasks.
type GoalProgress struct {
	DoneQuantity       int     `json:"done_quantity"`
	RemainingQuantity  int     `json:"remaining_quantity"`
	RequiredPerDay     float64 `json:"required_per_day"`
	ActualPerDay       float64 `json:"actual_per_day"`
	Pace               string  `json:"pace"`
	ProjectedFinishDay *string `json:"projected_finish_day"`
	PerSession         *int    `json:"per_session"`
	ReachableQuantity  int     `json:"reachable_quantity"`
	NeededDailyMinutes *int    `json:"needed_daily_minutes"`
	NeededDueDay       *string `json:"needed_due_day"`
	LinedUp            int     `json:"lined_up"`
}

// GoalList is the GET /v1/goals response.
type GoalList struct {
	Goals []Goal `json:"goals"`
}

// Commitment is a commitments row.
type Commitment struct {
	ID                 string  `json:"id"`
	Title              string  `json:"title"`
	ProjectID          *string `json:"project_id"`
	RRule              string  `json:"rrule"`
	StartMinute        *int    `json:"start_minute"`
	DurationMinutes    int     `json:"duration_minutes"`
	CountsTowardTarget bool    `json:"counts_toward_target"`
	ActiveFrom         string  `json:"active_from"`
	ActiveUntil        *string `json:"active_until"`
	CreatedAt          int64   `json:"created_at"`
	UpdatedAt          int64   `json:"updated_at"`
	Rev                int64   `json:"rev"`
}

// CommitmentList is the GET /v1/commitments response.
type CommitmentList struct {
	Commitments []Commitment `json:"commitments"`
}

// PlanItem is a plan_items row with its task.
type PlanItem struct {
	ID             string  `json:"id"`
	Day            string  `json:"day"`
	TaskID         string  `json:"task_id"`
	PlannedMinutes int     `json:"planned_minutes"`
	StartAt        *int64  `json:"start_at"`
	Position       int     `json:"position"`
	Status         string  `json:"status"`
	Pinned         bool    `json:"pinned"`
	RolledFromID   *string `json:"rolled_from_id"`
	RolloverCount  int     `json:"rollover_count"`
	CreatedAt      int64   `json:"created_at"`
	UpdatedAt      int64   `json:"updated_at"`
	Rev            int64   `json:"rev"`
	Task           Task    `json:"task"`
	Steps          []Task  `json:"steps"`
}

// Plan is a day's plan; Items are by position. PlannedMinutes sums the items
// planned or done. Hours is nil when the day has no hours of its own.
type Plan struct {
	Day             string     `json:"day"`
	CapacityMinutes int        `json:"capacity_minutes"`
	PlannedMinutes  int        `json:"planned_minutes"`
	Window          PlanWindow `json:"window"`
	Hours           *DayHours  `json:"hours"`
	Items           []PlanItem `json:"items"`
	// Events are the busy calendars' events the day plans around.
	Events []CalendarEvent `json:"events"`
}

// PlanWindow is the window capacity is computed over, in minutes after local
// midnight (docs/06-planner.md#capacity).
type PlanWindow struct {
	StartMinute int `json:"start_minute"`
	EndMinute   int `json:"end_minute"`
}

// DayHours is a day's own start and time for tasks; a nil field is the usual
// value (docs/06-planner.md#day-hours).
type DayHours struct {
	StartMinute *int `json:"start_minute"`
	WorkMinutes *int `json:"work_minutes"`
}

// Briefing is the GET /v1/briefing response.
type Briefing struct {
	Day       string     `json:"day"`
	Pending   []PlanItem `json:"pending"`
	Today     []PlanItem `json:"today"`
	Reminders []Reminder `json:"reminders"`
	Goals     []Goal     `json:"goals"`
}

// Reminder is a nudge about a task's due day.
type Reminder struct {
	TaskID   string `json:"task_id"`
	Title    string `json:"title"`
	DueDay   string `json:"due_day"`
	DaysLeft int    `json:"days_left"`
	Message  string `json:"message"`
}
