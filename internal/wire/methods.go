package wire

// ScheduleRequest is the POST /v1/plan/schedule body: put a task on day at
// StartAt (nil leaves the time to the planner) for PlannedMinutes, pinned.
// The task's other planned blocks from today on are removed, so this also
// moves a task.
type ScheduleRequest struct {
	TaskID         string `json:"task_id"`
	Day            string `json:"day"`
	StartAt        *int64 `json:"start_at"`
	PlannedMinutes int    `json:"planned_minutes"`
}

// UnscheduleRequest is the POST /v1/plan/unschedule body: take a task off
// the plan on Day, or from today on when Day is "".
type UnscheduleRequest struct {
	TaskID string `json:"task_id"`
	Day    string `json:"day"`
}

// Unscheduled is the POST /v1/plan/unschedule response: the days changed.
type Unscheduled struct {
	Days []string `json:"days"`
}

// WeeklyReview is a weekly_reviews row; a week without one reads as empty
// with an empty ID.
type WeeklyReview struct {
	ID           string `json:"id"`
	WeekStart    string `json:"week_start"`
	WentWell     string `json:"went_well"`
	WentBadly    string `json:"went_badly"`
	Energy       string `json:"energy"`
	Decisions    string `json:"decisions"`
	Improvements string `json:"improvements"`
	Checklist    int    `json:"checklist"`
	UpdatedAt    int64  `json:"updated_at"`
}

// SaveReviewRequest is the PUT /v1/reviews/{week_start} body; nil fields are
// unchanged.
type SaveReviewRequest struct {
	WentWell     *string `json:"went_well,omitempty"`
	WentBadly    *string `json:"went_badly,omitempty"`
	Energy       *string `json:"energy,omitempty"`
	Decisions    *string `json:"decisions,omitempty"`
	Improvements *string `json:"improvements,omitempty"`
	Checklist    *int    `json:"checklist,omitempty"`
}

// EnergyLog is an energy check-in: Level 1 (drained) to 5 (peak).
type EnergyLog struct {
	ID    string `json:"id"`
	At    int64  `json:"at"`
	Level int    `json:"level"`
}

// LogEnergyRequest is the POST /v1/energy body; a nil At is now.
type LogEnergyRequest struct {
	Level int    `json:"level"`
	At    *int64 `json:"at"`
}

// EnergyReport is the GET /v1/energy response: the check-ins of the last
// Days days, the average by hour of day, and the prime time they suggest.
type EnergyReport struct {
	Days  int          `json:"days"`
	Logs  []EnergyLog  `json:"logs"`
	Hours []EnergyHour `json:"hours"`
	// SuggestedStart and SuggestedEnd are HH:MM, or nil until there are
	// enough check-ins.
	SuggestedStart *string `json:"suggested_start"`
	SuggestedEnd   *string `json:"suggested_end"`
}

// EnergyHour is the average level of the check-ins in one hour of the day.
type EnergyHour struct {
	Hour    int     `json:"hour"`
	Average float64 `json:"average"`
	Count   int     `json:"count"`
}

// AssistantChatRequest is the POST /v1/assistant/chat body. ThreadID
// continues a chat, nil starts one; it is left out when nil, so a new chat
// also starts on a daemon from before chats were threads. RunID, sent by
// clients from before then, continues the chat of that run.
type AssistantChatRequest struct {
	Message  string  `json:"message"`
	ThreadID *string `json:"thread_id,omitempty"`
	RunID    *string `json:"run_id"`
}

// AssistantOutput is an assistant run's Output: the chat it belongs to, and
// the turn it added to it, the user's message and the reply with what it
// changed. A deleted chat's runs keep no messages. Usage is what the request
// used, when the provider said.
type AssistantOutput struct {
	ThreadID string             `json:"thread_id"`
	Messages []AssistantMessage `json:"messages"`
	Usage    *LlmUsage          `json:"usage,omitempty"`
}

// LlmUsage is what one LLM request used, in tokens: Input read in full,
// CacheRead read from the provider's prompt cache, and CacheWrite written to
// it.
type LlmUsage struct {
	Input      int `json:"input_tokens"`
	CacheRead  int `json:"cache_read_input_tokens"`
	CacheWrite int `json:"cache_creation_input_tokens"`
	Output     int `json:"output_tokens"`
}

// AssistantMessage is one turn of an assistant conversation. Mood is the face
// the assistant made with its turn, one of llm.Moods, or "" for none;
// Attitude is the one it switched to with this turn, or "" when it kept its own.
type AssistantMessage struct {
	ID       string         `json:"id,omitempty"`
	Role     string         `json:"role"`
	Text     string         `json:"text"`
	At       int64          `json:"at"`
	Mood     string         `json:"mood,omitempty"`
	Attitude string         `json:"attitude,omitempty"`
	Actions  []ActionResult `json:"actions,omitempty"`
}

// ChatThread is one chat with the assistant, titled by its first message.
// LastAt is when its newest message was said; Messages counts them.
type ChatThread struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Messages  int    `json:"messages"`
	CreatedAt int64  `json:"created_at"`
	LastAt    int64  `json:"last_at"`
}

// ChatThreadList is GET /v1/assistant/threads: a page of the chats, the
// most recently active first.
type ChatThreadList struct {
	Threads []ChatThread `json:"threads"`
}

// ChatThreadDetail is GET /v1/assistant/threads/{id}: a chat and its
// messages, oldest first.
type ChatThreadDetail struct {
	Thread   ChatThread         `json:"thread"`
	Messages []AssistantMessage `json:"messages"`
}

// ChatSearchResult is GET /v1/assistant/search: the messages that share
// words with the query, the best match first.
type ChatSearchResult struct {
	Hits []ChatHit `json:"hits"`
}

// ChatHit is a message that matched a search, and its chat.
type ChatHit struct {
	ThreadID    string           `json:"thread_id"`
	ThreadTitle string           `json:"thread_title"`
	Message     AssistantMessage `json:"message"`
}

// AssistantSelf is GET /v1/assistant/self: how the assistant chose, by
// itself, to treat the user. Attitude is one of llm.Attitudes, or "" before
// it chose one; Note is its own note on why; Since is when it chose, nil
// before then.
type AssistantSelf struct {
	Attitude string `json:"attitude"`
	Note     string `json:"note"`
	Since    *int64 `json:"since"`
}

// ActionResult is one change the assistant made, or tried to: Summary says
// what in words, and Error why it was not made.
type ActionResult struct {
	Type      string  `json:"type"`
	OK        bool    `json:"ok"`
	Summary   string  `json:"summary"`
	Error     string  `json:"error,omitempty"`
	TaskID    *string `json:"task_id,omitempty"`
	ProjectID *string `json:"project_id,omitempty"`
	GoalID    *string `json:"goal_id,omitempty"`
}
