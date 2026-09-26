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

// Task is a tasks row plus the computed TrackedMs. The v2 columns are always
// null in v1.
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
}

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
	DayStart string `json:"day_start" toml:"day_start"`
	DayEnd   string `json:"day_end" toml:"day_end"`
	Buffer   string `json:"buffer" toml:"buffer"`
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
	Timeout  string `json:"timeout" toml:"timeout"`
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
