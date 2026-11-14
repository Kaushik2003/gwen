package wire

// Gwen on the phone (plan.md 3.14): Claude Code sessions in Gwen Terminal,
// every terminal there, and pairing. The phone app, the dashboard and the CLI
// share these types.

// Event names on GET /v1/code/events, and on GET /v1/events of the main
// socket.
const (
	// EventCodeSession carries a CodeSession that changed.
	EventCodeSession = "code_session"
	// EventCodeSessionRemoved carries CodeSessionRemoved.
	EventCodeSessionRemoved = "code_session_removed"
	// EventCodeSpaces says the spaces, tabs or terminals changed; it carries
	// CodeSpacesChanged.
	EventCodeSpaces = "code_spaces"
	// EventRemoteChanged says pairing or publishing changed; it carries
	// RemoteChanged.
	EventRemoteChanged = "remote_changed"
	// EventCodeUsage carries ClaudeUsage: a newer reading of Claude's usage
	// limits.
	EventCodeUsage = "code_usage"
)

// Code session states.
const (
	CodeStarting = "starting"
	CodeWorking  = "working"
	CodeNeedsYou = "needs_you"
	CodeDone     = "done"
	CodeIdle     = "idle"
	CodeError    = "error"
	CodeEnded    = "ended"
)

// CodeSession is a Claude Code session in Gwen Terminal.
type CodeSession struct {
	ID string `json:"id"`
	// Terminal is its terminal in Gwen Terminal ("t7"), "" once it is gone.
	Terminal string `json:"terminal"`
	// Name is the folder's name.
	Name    string `json:"name"`
	Cwd     string `json:"cwd"`
	Branch  string `json:"branch"`
	Sandbox bool   `json:"sandbox"`
	State   string `json:"state"`
	// Need says why it needs you: permission, question, or other.
	Need string `json:"need"`
	// Step is what Claude is doing, e.g. "Editing main.go".
	Step        string `json:"step"`
	LastMessage string `json:"last_message"`
	// PermissionMode is default, acceptEdits, plan, auto, or "" when unknown.
	PermissionMode  string `json:"permission_mode"`
	ClaudeSessionID string `json:"claude_session_id"`
	Pinned          bool   `json:"pinned"`
	// Unread: it finished or asked for you and nobody has opened it since.
	Unread bool `json:"unread"`
	// Window: it has a tab in Gwen Terminal.
	Window bool `json:"window"`
	// Notice is a one-line note about the session, e.g. "Couldn't open Gwen
	// Terminal" or "Moved back to the laptop"; "" for none.
	Notice string `json:"notice"`
	// Waiting is the permission or question it is waiting on, when known.
	Waiting *CodeWaiting `json:"waiting"`
	// Helpers are the terminals this agent started, by address.
	Helpers []CodeTerminal `json:"helpers"`
	// Changes totals what the session changed, for its card; nil until
	// Gwen has counted.
	Changes   *CodeStat `json:"changes"`
	StartedAt int64     `json:"started_at"`
	UpdatedAt int64     `json:"updated_at"`
	EndedAt   *int64    `json:"ended_at"`
}

// CodeStat totals a session's changes: files, and lines added and removed,
// leaving out files that were already changed when it started.
type CodeStat struct {
	Files   int `json:"files"`
	Added   int `json:"added"`
	Removed int `json:"removed"`
}

// What a session is waiting on.
const (
	WaitingPermission = "permission"
	WaitingQuestion   = "question"
)

// CodeWaiting is a permission request or a question shown to the phone.
type CodeWaiting struct {
	Kind string `json:"kind"`
	// Held: Claude is waiting on Gwen, so the phone's answer decides. When
	// false the dialog shows on the laptop and the phone presses its keys.
	Held bool `json:"held"`
	// Tool and Summary say what Claude wants to do: "Bash" and the command,
	// "Edit" and the file.
	Tool    string `json:"tool"`
	Summary string `json:"summary"`
	// Question is the question's text, for a question.
	Question string `json:"question"`
	// Options are the choices: for a held permission allow, always (one per
	// suggestion) and deny; otherwise the numbered lines of the dialog.
	Options []CodeOption `json:"options"`
	// Note says why the phone cannot answer, e.g. "Answer in the Terminal
	// tab or on the laptop"; "" when it can.
	Note string `json:"note"`
	// HeldUntil is when a held request goes back to the laptop.
	HeldUntil *int64 `json:"held_until"`
}

// CodeOption is one choice of a CodeWaiting. Key is what to send back.
type CodeOption struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// Kind is allow, always, deny, or option.
	Kind string `json:"kind"`
}

// CodeSessionList is the body of GET /v1/code/sessions.
type CodeSessionList struct {
	Sessions []CodeSession `json:"sessions"`
	// Core says whether Gwen Terminal's core is reachable: ok, down, or
	// outdated. Message explains anything but ok.
	Core    string `json:"core"`
	Message string `json:"message"`
	// Machine is the laptop's name, for the phone's header.
	Machine string `json:"machine"`
}

// CodeSessionRemoved is the data of code_session_removed.
type CodeSessionRemoved struct {
	ID string `json:"id"`
}

// CodeSpacesChanged is the data of code_spaces.
type CodeSpacesChanged struct{}

// RemoteChanged is the data of remote_changed.
type RemoteChanged struct{}

// StartCodeSession is the body of POST /v1/code/sessions.
type StartCodeSession struct {
	Dir string `json:"dir"`
	// Create makes Dir's last path element; GitInit runs git init in it.
	Create  bool `json:"create,omitempty"`
	GitInit bool `json:"git_init,omitempty"`
	// Sandbox is nil to keep the mode a resumed conversation last ran in
	// (Normal for a new one).
	Sandbox *bool  `json:"sandbox,omitempty"`
	Prompt  string `json:"prompt,omitempty"`
	// Window is nil for the default: open a tab on the laptop.
	Window *bool `json:"window,omitempty"`
	// Resume is a Claude session id to continue.
	Resume string `json:"resume,omitempty"`
	// PermissionMode starts Claude Code in a permission mode: acceptEdits,
	// auto, or plan; "" keeps Claude Code's own default. Gwen's agent starts
	// a Trusted project's jobs in auto (plan.md 3.16).
	PermissionMode string `json:"permission_mode,omitempty"`
	// Name names the session ("" names it after its folder).
	Name string `json:"name,omitempty"`
}

// CodePrompt is the body of POST .../prompt and of POST
// /v1/code/terminals/{t}/send. Idem is a client UUID: the same one within 10
// minutes is not sent twice.
type CodePrompt struct {
	Text string `json:"text"`
	Idem string `json:"idem"`
}

// Results of a prompt.
const (
	PromptSent   = "sent"
	PromptQueued = "queued"
)

// CodePromptResult says how a prompt went in: sent, or queued behind what
// Claude is doing.
type CodePromptResult struct {
	Result string `json:"result"`
}

// Control actions.
const (
	ControlInterrupt    = "interrupt"
	ControlCycleMode    = "cycle_mode"
	ControlCompact      = "compact"
	ControlClear        = "clear"
	ControlEnd          = "end"
	ControlOpenOnLaptop = "open_on_laptop"
)

// CodeControl is the body of POST .../control.
type CodeControl struct {
	Action string `json:"action"`
}

// CodeAnswer is the body of POST .../answer: the Key of the chosen option,
// and for a denial an optional message to Claude.
type CodeAnswer struct {
	Key     string `json:"key"`
	Message string `json:"message,omitempty"`
}

// CodeAnswerResult says whether the answer went through; when it did not,
// Message says what to do.
type CodeAnswerResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// CodeMark is the body of POST .../mark; absent fields are unchanged.
type CodeMark struct {
	Pinned *bool `json:"pinned,omitempty"`
	Seen   *bool `json:"seen,omitempty"`
}

// CodeSpaces is the body of GET /v1/code/spaces: Gwen Terminal's spaces.
type CodeSpaces struct {
	Spaces []CodeSpace `json:"spaces"`
	// Loose are terminals with no tab: started without a window.
	Loose []CodeTerminal `json:"loose"`
	// Theme is Gwen Terminal's colours, for the phone.
	Theme CodeTheme `json:"theme"`
}

// CodeSpace is one space and its tabs.
type CodeSpace struct {
	ID     string    `json:"id"`
	Name   string    `json:"name"`
	Folder string    `json:"folder"`
	Tabs   []CodeTab `json:"tabs"`
}

// CodeTab is one tab and the terminals in its panes, in layout order.
type CodeTab struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Terminals []CodeTerminal `json:"terminals"`
}

// CodeTerminal is one terminal in Gwen Terminal.
type CodeTerminal struct {
	Terminal string `json:"terminal"`
	// Name is what to call it: its name, title, or program.
	Name string `json:"name"`
	Cwd  string `json:"cwd"`
	// Status is what its program reports (idle, working, done, blocked,
	// error), "" for a plain shell.
	Status string `json:"status"`
	App    string `json:"app"`
	// SessionID is its code session, "" when it is not Claude.
	SessionID string `json:"session_id"`
	Cols      int    `json:"cols"`
	Rows      int    `json:"rows"`
}

// CodeTheme is the colours of Gwen Terminal's active theme, as #rrggbb.
type CodeTheme struct {
	Name       string `json:"name"`
	Dark       bool   `json:"dark"`
	Background string `json:"background"`
	Foreground string `json:"foreground"`
	Cursor     string `json:"cursor"`
	Selection  string `json:"selection"`
	Accent     string `json:"accent"`
	Surface    string `json:"surface"`
	Border     string `json:"border"`
	Muted      string `json:"muted"`
	// ANSI are the 16 terminal colours, black to bright white.
	ANSI []string `json:"ansi"`
}

// CodePut is the reply of POST /v1/code/terminals/{t}/put: where the file
// landed on the laptop.
type CodePut struct {
	Path string `json:"path"`
}

// CodeNewTerminal is the reply of POST /v1/code/spaces/{id}/terminal.
type CodeNewTerminal struct {
	Terminal string `json:"terminal"`
}

// Transcript event kinds.
const (
	TranscriptYou    = "you"
	TranscriptClaude = "claude"
	TranscriptTool   = "tool"
	TranscriptNotice = "notice"
)

// CodeTranscriptEvent is one line of a session's conversation.
type CodeTranscriptEvent struct {
	// Cursor is the line's byte offset in the transcript; page with it.
	Cursor int64  `json:"cursor"`
	Kind   string `json:"kind"`
	At     int64  `json:"at"`
	// Text is the prompt, Claude's Markdown, the tool's one-line summary,
	// or the notice.
	Text string `json:"text"`
	// Tool is the tool's name, for a tool event.
	Tool string `json:"tool"`
	// OK is false when the tool failed.
	OK bool `json:"ok"`
	// Added and Removed are the line counts of an edit.
	Added   int `json:"added"`
	Removed int `json:"removed"`
	// Output is the last lines of a failed command.
	Output string `json:"output"`
}

// CodeTranscript is the body of GET .../transcript. Events are oldest first.
type CodeTranscript struct {
	Events []CodeTranscriptEvent `json:"events"`
	// Before pages back: pass it as ?before=; nil at the start.
	Before *int64 `json:"before"`
	// After is the cursor to poll with ?after= for newer events.
	After int64 `json:"after"`
}

// CodeChange is one changed file.
type CodeChange struct {
	Path string `json:"path"`
	// OldPath is where a renamed file was.
	OldPath string `json:"old_path"`
	// Status is added, modified, deleted, renamed, or untracked.
	Status  string `json:"status"`
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	Binary  bool   `json:"binary"`
	// BeforeSession: the file was already changed when the session started.
	BeforeSession bool `json:"before_session"`
}

// CodeChanges is the body of GET .../changes.
type CodeChanges struct {
	// Git is false outside a git folder: Files then lists what the session
	// edited, without diffs.
	Git    bool   `json:"git"`
	Branch string `json:"branch"`
	// Head is the short hash of the last commit the files are compared
	// with; "" before the first commit.
	Head  string       `json:"head"`
	Files []CodeChange `json:"files"`
}

// CodeDiff is the body of GET .../changes/diff.
type CodeDiff struct {
	Path      string `json:"path"`
	Diff      string `json:"diff"`
	Truncated bool   `json:"truncated"`
	Binary    bool   `json:"binary"`
	// Size is a binary file's size in bytes.
	Size int64 `json:"size"`
}

// CodeFolder is one folder of the folder picker.
type CodeFolder struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Git    bool   `json:"git"`
	Branch string `json:"branch"`
}

// CodeFolders is the body of GET /v1/code/folders and of
// GET /v1/code/recent-folders (Path and Parent are then "").
type CodeFolders struct {
	Path string `json:"path"`
	// Parent is the folder above, "" at a root.
	Parent  string       `json:"parent"`
	Folders []CodeFolder `json:"folders"`
	// Roots are the folders a session may start in.
	Roots []string `json:"roots"`
}

// CodeConversation is one earlier Claude conversation in a folder.
type CodeConversation struct {
	ClaudeSessionID string `json:"claude_session_id"`
	// Title is its name, or else its first message.
	Title    string `json:"title"`
	ActiveAt int64  `json:"active_at"`
	Sandbox  bool   `json:"sandbox"`
	// SessionID is the code session it is running in now, "" when it is not.
	SessionID string `json:"session_id"`
}

// CommitSession is a commit a Claude conversation made, and that
// conversation (plan.md 3.15, History).
type CommitSession struct {
	// Commit is the short commit id git printed.
	Commit string `json:"commit"`
	// At is when git printed it, Unix ms; 0 when not known.
	At              int64  `json:"at"`
	ClaudeSessionID string `json:"claude_session_id"`
	// Title is the conversation's own name, "" when it has none.
	Title       string `json:"title"`
	FirstPrompt string `json:"first_prompt"`
	Cwd         string `json:"cwd"`
	Sandbox     bool   `json:"sandbox"`
	// SessionID is the code session it is running in now, "" when it is not.
	SessionID string `json:"session_id"`
	// Summary is Gwen's summary of the commit, "" until you ask for one.
	Summary string `json:"summary"`
}

// CommitSessions is the body of GET /v1/code/commit-sessions?dir=.
type CommitSessions struct {
	Commits []CommitSession `json:"commits"`
}

// SummarizeCommit is the body of POST /v1/code/commit-sessions/summary.
type SummarizeCommit struct {
	Dir             string `json:"dir"`
	ClaudeSessionID string `json:"claude_session_id"`
	Commit          string `json:"commit"`
	// Subject is the commit's first line, which the model is given too.
	Subject string `json:"subject"`
}

// CommitSummary is the reply of POST /v1/code/commit-sessions/summary.
type CommitSummary struct {
	Summary string `json:"summary"`
}

// CodeConversations is the body of GET /v1/code/conversations.
type CodeConversations struct {
	Conversations []CodeConversation `json:"conversations"`
}

// CodeTranscription is the reply of POST /v1/code/transcribe.
type CodeTranscription struct {
	Text string `json:"text"`
}

// CodeHookEvent is the body of POST /v1/code/hook: what `gwen hook` keeps
// of a Claude Code hook event (plan.md 3.14.6).
type CodeHookEvent struct {
	Event          string `json:"event"`
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	PermissionMode string `json:"permission_mode"`
	ToolName       string `json:"tool_name"`
	// ToolSummary is the command (first 200 characters) or the file path.
	ToolSummary string `json:"tool_summary"`
	LastMessage string `json:"last_message"`
	Source      string `json:"source"`
	Reason      string `json:"reason"`
	// ToolInput and Suggestions are sent for PermissionRequest and
	// AskUserQuestion only.
	ToolInput   map[string]any   `json:"tool_input,omitempty" ts_type:"Record<string, any>"`
	Suggestions []map[string]any `json:"suggestions,omitempty" ts_type:"Record<string, any>[]"`
	// Terminal is HC_TERMINAL_ID; PID is Claude's process.
	Terminal string `json:"terminal"`
	PID      int    `json:"pid"`
	Sandbox  bool   `json:"sandbox"`
	// Usage is sent with the event StatusLine only, by `gwen statusline`.
	Usage *ClaudeUsage `json:"usage,omitempty"`
}

// HookStatusLine is the event `gwen statusline` sends: not one of Claude
// Code's hooks, but the usage limits Claude Code gives a status line.
const HookStatusLine = "StatusLine"

// ClaudeUsage is how much of your Claude plan's usage limits is used, as
// Claude Code last told the status line of a session Gwen started
// (GET /v1/code/usage). A window is nil when Claude Code did not report it;
// At is when the reading was taken, 0 when Gwen has none yet.
type ClaudeUsage struct {
	FiveHour *UsageWindow `json:"five_hour"`
	SevenDay *UsageWindow `json:"seven_day"`
	At       int64        `json:"at"`
}

// UsageWindow is one usage limit: the share used, 0 to 100, and when it
// resets (Unix ms, 0 when not known).
type UsageWindow struct {
	UsedPercent float64 `json:"used_percent"`
	ResetsAt    int64   `json:"resets_at"`
}

// CodeHookReply answers a hook. Output, when not empty, is what `gwen hook`
// prints for Claude Code: the decision of a held permission.
type CodeHookReply struct {
	Output string `json:"output"`
}

// RemoteStatus is the body of GET /v1/remote/status.
type RemoteStatus struct {
	Enabled bool `json:"enabled"`
	// Tailscale is ok, missing (not installed), stopped, or logged_out.
	Tailscale string `json:"tailscale"`
	// Host is this laptop's tailnet name, "" when unknown.
	Host string `json:"host"`
	// Certificates: the tailnet has HTTPS certificates on.
	Certificates bool `json:"certificates"`
	// Published: Tailscale Serve publishes the remote socket on Port.
	Published bool   `json:"published"`
	Port      int    `json:"port"`
	URL       string `json:"url"`
	// Owner is the Tailscale login allowed in, "" before the first turn-on.
	Owner string `json:"owner"`
	// Socket is the remote socket, for the publish command.
	Socket string `json:"socket"`
	// Hooks: Claude Code tells Gwen about its sessions. HooksNote explains
	// when it cannot, e.g. hooks are turned off in Claude's settings.
	Hooks     bool   `json:"hooks"`
	HooksNote string `json:"hooks_note"`
	// Core is ok, down, or outdated, with its version.
	Core        string `json:"core"`
	CoreVersion string `json:"core_version"`
	// Terminal: gwen-term is installed. Voice: the speech models are there.
	Terminal bool `json:"terminal"`
	Voice    bool `json:"voice"`
	Sandbox  bool `json:"sandbox"`
	// SandboxSignedIn: sandboxed sessions have their own sign-in token.
	SandboxSignedIn bool `json:"sandbox_signed_in"`
	Devices         int  `json:"devices"`
}

// RemotePairCode is the reply of POST /v1/remote/pair-code.
type RemotePairCode struct {
	Code      string `json:"code"`
	URL       string `json:"url"`
	ExpiresAt int64  `json:"expires_at"`
}

// RemoteDevice is a paired phone.
type RemoteDevice struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	CreatedAt  int64  `json:"created_at"`
	LastSeenAt *int64 `json:"last_seen_at"`
}

// RemoteDevices is the body of GET /v1/remote/devices.
type RemoteDevices struct {
	Devices []RemoteDevice `json:"devices"`
}

// RemoteHooks is the body of POST /v1/remote/hooks.
type RemoteHooks struct {
	On bool `json:"on"`
}

// RemotePair is the body of POST /pair on the remote socket.
type RemotePair struct {
	Code string `json:"code"`
	Name string `json:"name"`
}
