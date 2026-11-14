package wire

import "encoding/json"

// Gwen's agent (plan.md 3.16, W32): the chat that works in your projects.

// Events of Gwen's agent.
const (
	// EventAgentChanged carries AgentChanged: a chat's state or queue moved.
	EventAgentChanged = "agent_changed"
	// EventAgentText carries AgentText: her reply so far, at most one every
	// 50 ms per chat.
	EventAgentText = "agent_text"
	// EventAgentStep carries AgentStepEvent: a tool call began, waits, or ended.
	EventAgentStep = "agent_step"
)

// Agent chat states.
const (
	AgentIdle     = "idle"
	AgentStarting = "starting"
	AgentThinking = "thinking"
	AgentWaiting  = "waiting"
	AgentError    = "error"
)

// AgentChat is one of Gwen's chats: a project's, or one for everywhere.
type AgentChat struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// ProjectID is nil for a chat for everywhere.
	ProjectID *string `json:"project_id"`
	// State is idle, starting, thinking (she works), waiting (on you), or error.
	State string `json:"state"`
	Error string `json:"error"`
	// Queued is how many of your messages wait for her to finish.
	Queued   int   `json:"queued"`
	Messages int   `json:"messages"`
	LastAt   int64 `json:"last_at"`
	// Pending is how many approvals wait for you.
	Pending int `json:"pending"`
}

// AgentChatList is GET /v1/agent/chats.
type AgentChatList struct {
	Chats []AgentChat `json:"chats"`
}

// AgentStep is one tool call, as the chat shows it.
type AgentStep struct {
	ID        string `json:"id"`
	ToolUseID string `json:"tool_use_id"`
	Turn      int    `json:"turn"`
	Position  int    `json:"position"`
	Tool      string `json:"tool"`
	// Capability is what the policy saw it as (fs.edit, shell.run, ...);
	// Risk is read, local, write, external, or destructive.
	Capability string          `json:"capability"`
	Risk       string          `json:"risk"`
	Title      string          `json:"title"`
	Input      json.RawMessage `json:"input" ts_type:"any"`
	Output     string          `json:"output"`
	// Status is running, waiting, done, failed, denied, or cancelled.
	Status     string  `json:"status"`
	ApprovalID *string `json:"approval_id"`
	CanUndo    bool    `json:"can_undo"`
	StartedAt  int64   `json:"started_at"`
	EndedAt    *int64  `json:"ended_at"`
}

// AgentChatDetail is GET /v1/agent/chats/{id}: the chat, its messages and
// steps, the approvals waiting in it, and her reply so far while she works.
type AgentChatDetail struct {
	Chat      AgentChat          `json:"chat"`
	Messages  []AssistantMessage `json:"messages"`
	Steps     []AgentStep        `json:"steps"`
	Approvals []Approval         `json:"approvals"`
	Queue     []string           `json:"queue"`
	// Partial is the reply she is writing now, "" when none.
	Partial string `json:"partial"`
}

// CreateAgentChat is the POST /v1/agent/chats body.
type CreateAgentChat struct {
	// ProjectID is nil for a chat for everywhere.
	ProjectID *string `json:"project_id"`
}

// PatchAgentChat is the PATCH /v1/agent/chats/{id} body.
type PatchAgentChat struct {
	Title string `json:"title"`
}

// AgentFocus is what you have open in the window when you send a message.
type AgentFocus struct {
	// Kind is today, screen, project, file, terminal, or history.
	Kind      string `json:"kind"`
	Screen    string `json:"screen,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Path      string `json:"path,omitempty"`
	Terminal  string `json:"terminal,omitempty"`
	// Selection is text selected in the side panel, if any.
	Selection string `json:"selection,omitempty"`
}

// AgentMessageRequest is the POST /v1/agent/chats/{id}/messages body.
type AgentMessageRequest struct {
	Text  string      `json:"text"`
	Focus *AgentFocus `json:"focus,omitempty"`
}

// AgentStatus is GET /v1/agent/status: whether the agent can run, and why not.
type AgentStatus struct {
	Enabled bool `json:"enabled"`
	// Available: Claude Code is found and the provider is claude_code.
	Available bool `json:"available"`
	// Reason says why it can't run, "" when it can.
	Reason   string `json:"reason"`
	Command  string `json:"command"`
	Autonomy string `json:"autonomy"`
	Live     int    `json:"live"`
}

// AgentChanged is the data of agent_changed.
type AgentChanged struct {
	ChatID string `json:"chat_id"`
	State  string `json:"state"`
	Error  string `json:"error"`
	Queued int    `json:"queued"`
	// Ended: a turn ended, so messages and steps should be read again.
	Ended bool `json:"ended"`
}

// AgentText is the data of agent_text: the reply so far.
type AgentText struct {
	ChatID string `json:"chat_id"`
	Text   string `json:"text"`
}

// AgentStepEvent is the data of agent_step.
type AgentStepEvent struct {
	ChatID string    `json:"chat_id"`
	Step   AgentStep `json:"step"`
}

// AgentTool is one of Gwen's tools, as `gwen mcp` lists it.
type AgentTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema" ts_type:"any"`
}

// AgentToolList is GET /v1/agent/tools on agent.sock.
type AgentToolList struct {
	Tools []AgentTool `json:"tools"`
}

// AgentToolResult is what POST /v1/agent/tools/{name} answers: a summary
// for the model and the chat, data for the model, and the change it made,
// which the chat can undo.
type AgentToolResult struct {
	OK      bool            `json:"ok"`
	Summary string          `json:"summary"`
	Data    json.RawMessage `json:"data,omitempty" ts_type:"any"`
	Change  string          `json:"change,omitempty"`
	Error   string          `json:"error,omitempty"`
}
