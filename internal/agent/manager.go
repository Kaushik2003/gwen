// Package agent is Gwen's agent (plan.md 3.16, W32): the chat that works in
// your projects. Each live chat is a Claude Code process speaking stream-json;
// Gwen answers every permission request it makes with her policy and your
// approvals, and serves her own tools to it through `gwen mcp`.
package agent

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/connect"
	"github.com/kzark/gwen/internal/harness"
	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
)

// Limits (plan.md 3.16).
const (
	MaxMessage = 32000
	maxQueue   = 5
	textEvery  = 50 * time.Millisecond
)

// Errors the API maps to responses.
var (
	ErrUnavailable = errors.New("the agent can't run")
	ErrBusy        = errors.New("too many messages are waiting; let her finish first")
	ErrNoToken     = errors.New("not a token of a running chat")
	ErrNoUndo      = errors.New("there is nothing to undo")
)

// Sources is what the agent needs from the rest of Gwen; internal/api
// implements it.
type Sources interface {
	// Persona is who she is, the name you gave her, your instructions, and
	// how she chose to treat you.
	Persona(ctx context.Context) string
	// ProfileCard is what she keeps in mind about you.
	ProfileCard(ctx context.Context) string
	// Place says where a chat works, for its system prompt; project is nil
	// for a chat for everywhere.
	Place(ctx context.Context, project *model.Project) string
	// Note is the context note in front of a message (plan.md 3.16).
	Note(ctx context.Context, thread model.ChatThread, project *model.Project, focus *wire.AgentFocus, text string) string
	// AddMessage stores a message of a chat.
	AddMessage(ctx context.Context, threadID, role, text string) (model.ChatMessage, error)
	// TurnDone tells the memory of a turn that ended.
	TurnDone(ctx context.Context, threadID string, user, reply model.ChatMessage)
	// Tools are Gwen's tools; ToolRequest is a call's capability, risk, and
	// why it is risky, which may depend on its input (app_run).
	Tools() []wire.AgentTool
	ToolRequest(name string, input json.RawMessage) (Request, bool)
	// ToolTitle is a call as a step says it.
	ToolTitle(name string, input json.RawMessage) string
	// CallTool runs one of Gwen's tools for a chat; undo is how to take its
	// change back, nil for none.
	CallTool(ctx context.Context, thread model.ChatThread, name string, input json.RawMessage) (wire.AgentToolResult, json.RawMessage, error)
	// Undo takes a change back.
	Undo(ctx context.Context, undo json.RawMessage) error
	// Sessions are the Claude sessions on this machine, to follow the ones
	// a chat started.
	Sessions() []wire.CodeSession
}

// Options are what a Manager runs with.
type Options struct {
	Repos     store.Repos
	Approvals *harness.Approvals
	Sources   Sources
	Publish   func(name string, data any)
	// Notify shows a desktop notification that an approval waits in a chat
	// (thread); nil shows none.
	Notify func(thread, title, body string)
	Config func() config.Config
	// Home is the user's home folder, Runtime Gwen's runtime folder
	// ($XDG_RUNTIME_DIR/gwen), DataDir Gwen's data folder, GwenBin the gwen
	// binary, and AgentSocket agent.sock, which `gwen mcp` reaches.
	Home        string
	Runtime     string
	DataDir     string
	GwenBin     string
	AgentSocket string
}

// Manager runs the chats' processes.
type Manager struct {
	o      Options
	mu     sync.Mutex
	chats  map[string]*chat
	tokens map[string]string // token → chat
	// waiting are chats whose message waits for a free process.
	waiting []string
	// jobs are the Claude sessions chats started, by session id, which
	// Gwen follows (plan.md 3.16).
	jobs   map[string]*job
	closed bool
}

type queued struct {
	text  string
	focus *wire.AgentFocus
	// system: an event for Gwen, not a message of yours; it is not stored
	// in the chat and the queue does not show it.
	system bool
}

// job is a Claude session a chat started, and the state Gwen last saw.
type job struct {
	chat  string
	title string
	state string
	// worked: it has been working since it last finished, so going idle now
	// is a finish (a session that waits on a question and then goes idle
	// has not done the job).
	worked bool
}

// chat is a chat while this daemon runs. The Manager's mu guards it.
type chat struct {
	thread  model.ChatThread
	project *model.Project
	place   Place

	proc      *process
	gen       int
	session   string
	resumable bool
	token     string
	files     []string
	lastUsed  time.Time

	state   string
	errText string
	busy    bool
	// stopped: you pressed Stop in this turn.
	stopped bool
	queue   []queued
	pending int

	turn      int
	position  int
	user      model.ChatMessage
	texts     []string
	partial   strings.Builder
	turnCtx   context.Context
	endTurn   context.CancelFunc
	idle      *time.Timer
	timeout   *time.Timer
	textTimer *time.Timer
	// gwenSteps are the steps of Gwen's tool calls not yet answered, by tool
	// and input, so the call that arrives at agent.sock finds its step.
	gwenSteps map[string][]string
}

// New builds a Manager; Start makes it ready.
func New(o Options) *Manager {
	if o.Publish == nil {
		o.Publish = func(string, any) {}
	}
	return &Manager{o: o, chats: map[string]*chat{}, tokens: map[string]string{}, jobs: map[string]*job{}}
}

// Start makes every chat idle after a restart (plan.md 3.16).
func (m *Manager) Start(ctx context.Context) error {
	return m.o.Repos.Agent.Interrupted(ctx)
}

// Close stops every process.
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	var procs []*process
	for _, c := range m.chats {
		if c.proc != nil {
			procs = append(procs, c.proc)
		}
		if c.endTurn != nil {
			c.endTurn()
		}
	}
	m.mu.Unlock()
	for _, p := range procs {
		p.kill()
	}
}

// Status says whether the agent can run, and why not.
func (m *Manager) Status() wire.AgentStatus {
	cfg := m.o.Config()
	st := wire.AgentStatus{Enabled: cfg.Agent.Enabled, Command: cfg.LLM.Command, Autonomy: cfg.Agent.Autonomy}
	_, st.Reason = m.claude(cfg)
	st.Available = st.Reason == ""
	m.mu.Lock()
	for _, c := range m.chats {
		if c.proc != nil {
			st.Live++
		}
	}
	m.mu.Unlock()
	return st
}

// claude is the CLI to run, or why the agent can't run.
func (m *Manager) claude(cfg config.Config) (string, string) {
	if !cfg.Agent.Enabled {
		return "", "Gwen's agent is turned off (Settings › Gwen › Agent)."
	}
	if cfg.LLM.Provider != config.ProviderClaudeCode {
		return "", "Gwen's agent runs on Claude Code: choose Claude Code as her AI in Settings › Gwen › AI."
	}
	path, err := llm.FindCommand(cfg.LLM.Command)
	if err != nil {
		return "", fmt.Sprintf("Claude Code (%s) isn't installed here.", cfg.LLM.Command)
	}
	return path, ""
}

// Send gives a chat your message. It starts a turn, or waits in the chat's
// queue while she works.
func (m *Manager) Send(ctx context.Context, threadID, text string, focus *wire.AgentFocus) error {
	text = strings.TrimSpace(text)
	if n := utf8.RuneCountInString(text); n < 1 || n > MaxMessage {
		return store.FailField(store.ErrInvalid, "text", "a message is 1 to %d characters", MaxMessage)
	}
	if _, why := m.claude(m.o.Config()); why != "" {
		return fmt.Errorf("%w: %s", ErrUnavailable, why)
	}
	thread, err := m.o.Repos.Chats.Get(ctx, threadID)
	if err != nil {
		return err
	}
	c, err := m.load(ctx, thread)
	if err != nil {
		return err
	}
	m.mu.Lock()
	if c.busy || len(c.queue) > 0 || slices.Contains(m.waiting, threadID) {
		if len(c.queue) >= maxQueue {
			m.mu.Unlock()
			return ErrBusy
		}
		c.queue = append(c.queue, queued{text: text, focus: focus})
		m.changedLocked(c, false)
		m.mu.Unlock()
		return nil
	}
	c.busy = true
	m.mu.Unlock()
	go m.run(c, queued{text: text, focus: focus})
	return nil
}

// load is the live chat of a thread, made when it is first used.
func (m *Manager) load(ctx context.Context, thread model.ChatThread) (*chat, error) {
	var project *model.Project
	if thread.ProjectID != nil {
		p, err := m.o.Repos.Projects.Get(ctx, *thread.ProjectID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		if err == nil {
			project = &p
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.chats[thread.ID]
	if c == nil {
		c = &chat{state: wire.AgentIdle, gwenSteps: map[string][]string{}}
		if ac, err := m.o.Repos.Agent.Chat(ctx, thread.ID); err == nil {
			c.session, c.resumable = ac.ClaudeSessionID, ac.ClaudeSessionID != ""
			if ac.State == wire.AgentError {
				c.state, c.errText = wire.AgentIdle, ac.Error
			}
		}
		if c.session == "" {
			c.session = uuid.NewString()
		}
		m.chats[thread.ID] = c
	}
	c.thread, c.project = thread, project
	c.place = Place{Home: m.o.Home, Runtime: m.o.Runtime, GwenBin: m.o.GwenBin}
	if project != nil {
		c.place.Folder = project.Folder
	}
	return c, nil
}

// autonomy is how freely the agent acts in a chat.
func (m *Manager) autonomy(c *chat) string {
	if c.project != nil && c.project.AgentAutonomy != "" {
		return c.project.AgentAutonomy
	}
	return m.o.Config().Agent.Autonomy
}

// run is one turn: it starts the process when there is none, stores your
// message, and sends it with its context note. The process's events carry
// the turn on; endTurn ends it.
func (m *Manager) run(c *chat, q queued) {
	ctx := context.Background()
	m.mu.Lock()
	c.errText = ""
	if c.proc == nil {
		if !m.roomLocked(c) {
			// Every process is busy: the message waits for one to finish.
			c.busy = false
			c.queue = append([]queued{q}, c.queue...)
			if !slices.Contains(m.waiting, c.thread.ID) {
				m.waiting = append(m.waiting, c.thread.ID)
			}
			m.changedLocked(c, false)
			m.mu.Unlock()
			return
		}
		c.state = wire.AgentStarting
	} else {
		c.state = wire.AgentThinking
	}
	if c.idle != nil {
		c.idle.Stop()
	}
	c.turnCtx, c.endTurn = context.WithCancel(ctx)
	c.texts, c.position = nil, 0
	c.partial.Reset()
	c.lastUsed = time.Now()
	m.changedLocked(c, false)
	needStart := c.proc == nil
	m.mu.Unlock()

	if needStart {
		if err := m.spawn(ctx, c); err != nil {
			m.fail(c, err.Error())
			return
		}
	}
	var user model.ChatMessage
	if !q.system {
		var err error
		if user, err = m.o.Sources.AddMessage(ctx, c.thread.ID, model.ChatUser, q.text); err != nil {
			m.fail(c, "Couldn't keep your message: "+err.Error())
			return
		}
	}
	turn := 1
	if t, err := m.o.Repos.Chats.Get(ctx, c.thread.ID); err == nil {
		turn = t.Messages
		m.mu.Lock()
		c.thread = t
		m.mu.Unlock()
	}
	noteText := q.text
	if q.system {
		noteText = ""
	}
	note := m.o.Sources.Note(ctx, c.thread, c.project, q.focus, noteText)
	m.mu.Lock()
	c.user, c.turn, c.state = user, turn, wire.AgentThinking
	proc := c.proc
	timeout := m.o.Config().Agent.TurnTimeout
	c.timeout = time.AfterFunc(timeout, func() { m.timedOut(c, proc) })
	m.changedLocked(c, false)
	m.mu.Unlock()
	if proc == nil {
		m.fail(c, "Claude Code stopped before your message reached it.")
		return
	}
	if err := proc.user(note, q.text); err != nil {
		m.fail(c, "Couldn't reach Claude Code: "+err.Error())
	}
}

// roomLocked makes room for one more process, stopping the one idle the
// longest when all are taken; false when none can stop. m.mu is held.
func (m *Manager) roomLocked(c *chat) bool {
	live, max := 0, m.o.Config().Agent.MaxLive
	var oldest *chat
	for _, x := range m.chats {
		if x.proc == nil || x == c {
			continue
		}
		live++
		if !x.busy && (oldest == nil || x.lastUsed.Before(oldest.lastUsed)) {
			oldest = x
		}
	}
	if live < max {
		return true
	}
	if oldest == nil {
		return false
	}
	p := oldest.proc
	oldest.proc = nil
	oldest.gen++
	m.dropTokenLocked(oldest)
	go p.close()
	return true
}

// spawn starts a chat's process: its system prompt, its MCP config with a
// token of its own, and the CLI.
func (m *Manager) spawn(ctx context.Context, c *chat) error {
	cfg := m.o.Config()
	claude, why := m.claude(cfg)
	if why != "" {
		return errors.New(why)
	}
	dir := c.place.Folder
	if dir == "" {
		dir = filepath.Join(m.o.DataDir, "agent", "everywhere")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("make the agent's folder: %w", err)
		}
	} else if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return fmt.Errorf("the project's folder %s isn't there", dir)
	}
	filesDir := filepath.Join(m.o.Runtime, "agent")
	if err := os.MkdirAll(filesDir, 0o700); err != nil {
		return fmt.Errorf("make the agent's runtime folder: %w", err)
	}
	token := newToken()
	persona := m.o.Sources.Persona(ctx)
	card := m.o.Sources.ProfileCard(ctx)
	place := m.o.Sources.Place(ctx, c.project)
	promptPath := filepath.Join(filesDir, c.thread.ID+".prompt.md")
	if err := os.WriteFile(promptPath, []byte(SystemPrompt(persona, card, place, m.o.Home, c.place.Folder, dir, time.Now())), 0o600); err != nil {
		return fmt.Errorf("write the agent's prompt: %w", err)
	}
	mcp, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"gwen": map[string]any{
		"type": "stdio", "command": m.o.GwenBin, "args": []string{"mcp"},
		"env": map[string]string{"GWEN_AGENT": "1", "GWEN_AGENT_TOKEN": token, "GWEN_AGENT_SOCKET": m.o.AgentSocket}}}})
	mcpPath := filepath.Join(filesDir, c.thread.ID+".mcp.json")
	if err := os.WriteFile(mcpPath, mcp, 0o600); err != nil {
		return fmt.Errorf("write the agent's MCP config: %w", err)
	}
	scope := ""
	if os.Getenv("INVOCATION_ID") != "" { // set by systemd for the units it runs
		if sr, err := exec.LookPath("systemd-run"); err == nil {
			scope = sr
		}
	}
	m.mu.Lock()
	c.gen++
	gen := c.gen
	spec := Spec{Claude: claude, Scope: scope, Dir: dir, Session: c.session, Resume: c.resumable, Model: cfg.LLM.Model,
		MCPConfig: mcpPath, Prompt: promptPath}
	m.mu.Unlock()
	p, err := start(spec, func(line []byte) { m.line(c, gen, line) }, func(err error, stderr string) { m.exited(c, gen, err, stderr) })
	if err != nil {
		os.Remove(promptPath)
		os.Remove(mcpPath)
		return err
	}
	m.mu.Lock()
	c.proc, c.token, c.files = p, token, []string{promptPath, mcpPath}
	m.tokens[token] = c.thread.ID
	m.mu.Unlock()
	if err := m.o.Repos.Agent.PutChat(ctx, store.AgentChat{ThreadID: c.thread.ID, ClaudeSessionID: c.session, Cwd: dir,
		State: wire.AgentThinking}); err != nil {
		slog.Warn("agent chat not kept", "chat", c.thread.ID, "err", err)
	}
	slog.Info("agent started", "chat", c.thread.ID, "dir", dir, "resume", spec.Resume)
	return nil
}

func newToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// dropTokenLocked forgets a chat's token and files. m.mu is held.
func (m *Manager) dropTokenLocked(c *chat) {
	delete(m.tokens, c.token)
	c.token = ""
	for _, f := range c.files {
		os.Remove(f)
	}
	c.files = nil
}

// line handles one line of a chat's process.
func (m *Manager) line(c *chat, gen int, raw []byte) {
	var msg streamMsg
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}
	m.mu.Lock()
	stale := gen != c.gen
	m.mu.Unlock()
	if stale {
		return
	}
	ctx := context.Background()
	switch msg.Type {
	case "system":
		if msg.Subtype == "init" && msg.SessionID != "" {
			m.mu.Lock()
			c.session, c.resumable = msg.SessionID, true
			m.mu.Unlock()
		}
	case "stream_event":
		if msg.ParentToolUseID != nil || msg.Event == nil || msg.Event.Type != "content_block_delta" ||
			msg.Event.Delta == nil || msg.Event.Delta.Type != "text_delta" {
			return
		}
		m.mu.Lock()
		c.partial.WriteString(msg.Event.Delta.Text)
		if c.textTimer == nil {
			c.textTimer = time.AfterFunc(textEvery, func() { m.publishText(c) })
		}
		m.mu.Unlock()
	case "assistant":
		if msg.Message == nil {
			return
		}
		for _, b := range msg.Message.Content {
			switch b.Type {
			case "text":
				if msg.ParentToolUseID == nil && strings.TrimSpace(b.Text) != "" {
					m.mu.Lock()
					c.texts = append(c.texts, strings.TrimSpace(b.Text))
					c.partial.Reset()
					m.mu.Unlock()
				}
			case "tool_use":
				m.toolUse(ctx, c, b)
			}
		}
	case "user":
		if msg.Message == nil {
			return
		}
		for _, b := range msg.Message.Content {
			if b.Type == "tool_result" {
				m.toolResult(ctx, c, b)
			}
		}
	case "control_request":
		var req permissionRequest
		_ = json.Unmarshal(msg.Request, &req)
		if req.Subtype == "can_use_tool" {
			go m.permission(c, gen, msg.RequestID, req)
			return
		}
		m.mu.Lock()
		p := c.proc
		m.mu.Unlock()
		if p != nil {
			_ = p.send(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "error",
				"request_id": msg.RequestID, "error": "Gwen doesn't handle " + req.Subtype}})
		}
	case "result":
		m.finish(c, msg)
	}
}

// publishText sends her reply so far.
func (m *Manager) publishText(c *chat) {
	m.mu.Lock()
	c.textTimer = nil
	text := c.replyLocked()
	id := c.thread.ID
	m.mu.Unlock()
	m.o.Publish(wire.EventAgentText, wire.AgentText{ChatID: id, Text: text})
}

// replyLocked is her reply so far. m.mu is held.
func (c *chat) replyLocked() string {
	parts := append([]string{}, c.texts...)
	if p := strings.TrimSpace(c.partial.String()); p != "" {
		parts = append(parts, p)
	}
	return strings.Join(parts, "\n\n")
}

// canonical is a JSON value written the same way however it came.
func canonical(raw json.RawMessage) string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return string(raw)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// toolUse records a tool call as a step.
func (m *Manager) toolUse(ctx context.Context, c *chat, b block) {
	if _, err := m.o.Repos.Agent.StepOfToolUse(ctx, c.thread.ID, b.ID); err == nil {
		return
	}
	var req Request
	if name, ok := strings.CutPrefix(b.Name, "mcp__gwen__"); ok {
		req, _ = m.o.Sources.ToolRequest(name, b.Input)
		req.Title = m.o.Sources.ToolTitle(name, b.Input)
	} else {
		m.mu.Lock()
		place := c.place
		m.mu.Unlock()
		req = Classify(b.Name, b.Input, "", place)
	}
	m.mu.Lock()
	st := store.AgentStep{ID: model.NewID(), ThreadID: c.thread.ID, ToolUseID: b.ID, Turn: c.turn, Position: c.position,
		Tool: b.Name, Capability: req.Capability, Risk: string(req.Risk), Title: req.Title, Input: b.Input,
		Status: store.AgentStepRunning, StartedAt: time.Now()}
	c.position++
	if name, ok := strings.CutPrefix(b.Name, "mcp__gwen__"); ok {
		key := name + " " + canonical(b.Input)
		c.gwenSteps[key] = append(c.gwenSteps[key], st.ID)
	}
	m.mu.Unlock()
	m.putStep(ctx, st)
}

// toolResult ends a tool call's step.
func (m *Manager) toolResult(ctx context.Context, c *chat, b block) {
	st, err := m.o.Repos.Agent.StepOfToolUse(ctx, c.thread.ID, b.ToolUseID)
	if err != nil {
		return
	}
	if st.Status == store.AgentStepDenied || st.Status == store.AgentStepCancelled {
		return
	}
	text := b.resultText()
	st.Status = store.AgentStepDone
	if b.IsError {
		st.Status = store.AgentStepFailed
	}
	// Gwen's own tools wrote their step's output when they ran (with the
	// session mark of a job), so Claude's copy of the result leaves it.
	if gwen := strings.HasPrefix(st.Tool, "mcp__gwen__"); !gwen || st.Output == "" {
		if out := outputOf(st.Tool, text, b.IsError); out != "" || !gwen {
			st.Output = out
		}
	}
	now := time.Now()
	st.EndedAt = &now
	m.putStep(ctx, st)
}

// outputOf is what a step keeps of a tool's result: never a file's contents
// from a read (plan.md 3.16).
func outputOf(tool, text string, isErr bool) string {
	text = strings.TrimSpace(text)
	if isErr {
		return cutTail(text, 1024)
	}
	switch tool {
	case "Read", "NotebookRead":
		return fmt.Sprintf("Read %d lines", strings.Count(text, "\n")+1)
	case "Glob", "Grep", "LS":
		n := 0
		for _, l := range strings.Split(text, "\n") {
			if strings.TrimSpace(l) != "" {
				n++
			}
		}
		return fmt.Sprintf("%d results", n)
	case "WebFetch", "WebSearch":
		return cutTail(text, 300)
	}
	if strings.HasPrefix(tool, "mcp__gwen__") {
		var r wire.AgentToolResult
		if json.Unmarshal([]byte(text), &r) == nil {
			if r.Error != "" {
				return r.Error
			}
			return r.Summary
		}
		return ""
	}
	return cutTail(text, store.MaxStepOutput)
}

// cutTail is the last n bytes of s, on a line or character boundary.
func cutTail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[len(s)-n:]
	if i := strings.IndexByte(s, '\n'); i >= 0 && i < 200 {
		return "…" + s[i:]
	}
	for len(s) > 0 && !utf8.RuneStart(s[0]) {
		s = s[1:]
	}
	return "…" + s
}

func (m *Manager) putStep(ctx context.Context, st store.AgentStep) {
	if err := m.o.Repos.Agent.PutStep(ctx, st); err != nil {
		slog.Warn("agent step not kept", "chat", st.ThreadID, "err", err)
		return
	}
	m.o.Publish(wire.EventAgentStep, wire.AgentStepEvent{ChatID: st.ThreadID, Step: StepWire(st)})
}

// StepWire is a step as the API shows it.
func StepWire(st store.AgentStep) wire.AgentStep {
	out := wire.AgentStep{ID: st.ID, ToolUseID: st.ToolUseID, Turn: st.Turn, Position: st.Position, Tool: st.Tool,
		Capability: st.Capability, Risk: st.Risk, Title: st.Title, Input: st.Input, Output: st.Output, Status: st.Status,
		ApprovalID: st.ApprovalID, CanUndo: len(st.Undo) > 2, StartedAt: st.StartedAt.UnixMilli()}
	if len(out.Input) == 0 {
		out.Input = json.RawMessage("{}")
	}
	if st.EndedAt != nil {
		e := st.EndedAt.UnixMilli()
		out.EndedAt = &e
	}
	return out
}

// permission answers one of Claude Code's permission requests.
func (m *Manager) permission(c *chat, gen int, requestID string, pr permissionRequest) {
	ctx := context.Background()
	m.mu.Lock()
	p, place, turnCtx := c.proc, c.place, c.turnCtx
	if gen != c.gen || p == nil {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	if strings.HasPrefix(pr.ToolName, "mcp__gwen__") {
		_ = p.answer(requestID, true, pr.Input, "") // checked by gwend at the call
		return
	}
	req := Classify(pr.ToolName, pr.Input, pr.BlockedPath, place)
	st, err := m.o.Repos.Agent.StepOfToolUse(ctx, c.thread.ID, pr.ToolUseID)
	if err != nil {
		m.mu.Lock()
		st = store.AgentStep{ID: model.NewID(), ThreadID: c.thread.ID, ToolUseID: pr.ToolUseID, Turn: c.turn,
			Position: c.position, Tool: pr.ToolName, Input: pr.Input, StartedAt: time.Now()}
		c.position++
		m.mu.Unlock()
	}
	st.Capability, st.Risk, st.Title = req.Capability, string(req.Risk), req.Title
	payload, _ := json.Marshal(map[string]any{"tool": pr.ToolName, "input": pr.Input, "folder": place.Folder, "why": req.Why})
	ok, message := m.gate(turnCtx, c, &st, req, payload)
	if ok {
		_ = p.answer(requestID, true, pr.Input, "")
		return
	}
	_ = p.answer(requestID, false, nil, message)
}

// gate decides a request: allowed by the policy or a grant, denied by the
// floor, or asked of you. While it waits, the step is waiting and the chat
// waits on you. It returns whether the call may go ahead and, when not, the
// message the model reads.
func (m *Manager) gate(ctx context.Context, c *chat, st *store.AgentStep, req Request, payload json.RawMessage) (bool, string) {
	bg := context.Background()
	m.mu.Lock()
	autonomy := m.autonomy(c)
	m.mu.Unlock()
	verdict := Decide(req, autonomy)
	scopes := []string{"chat:" + c.thread.ID}
	project := ""
	if c.thread.ProjectID != nil {
		project = *c.thread.ProjectID
		scopes = append(scopes, "project:"+project)
	}
	if (verdict == Ask || verdict == AskOnce) && req.Grantable && req.Risk != connect.External && req.Risk != connect.Destructive {
		if allowed, err := m.o.Approvals.Allowed(bg, req.Capability, req.Pattern, scopes); err == nil && allowed {
			verdict = Allow
		}
	}
	switch verdict {
	case Allow:
		st.Status = store.AgentStepRunning
		m.putStep(bg, *st)
		return true, ""
	case Deny:
		st.Status, st.Output = store.AgentStepDenied, "Gwen's rules never allow this: "+req.Why
		now := time.Now()
		st.EndedAt = &now
		m.putStep(bg, *st)
		return false, "Gwen's rules never allow this (" + req.Why + "). Don't try to do it another way; tell the user what you needed."
	}
	if ctx == nil {
		ctx = bg
	}
	st.Status = store.AgentStepWaiting
	m.putStep(bg, *st)
	m.mu.Lock()
	c.pending++
	c.state = wire.AgentWaiting
	m.changedLocked(c, false)
	m.mu.Unlock()
	summary := req.Title
	if w := riskWords(req); w != "" {
		summary += " · " + w
	}
	if m.o.Notify != nil {
		where := "a chat for everywhere"
		if c.project != nil {
			where = c.project.Name
		}
		m.o.Notify(c.thread.ID, "Gwen needs your OK in "+where, req.Title)
	}
	d, err := m.o.Approvals.Ask(ctx, harness.Request{ThreadID: c.thread.ID, StepID: st.ID, Capability: req.Capability,
		Risk: req.Risk, Pattern: req.Pattern, Summary: summary, Payload: payload,
		GrantScopes: GrantScopes(req, c.thread.ID, project)})
	m.mu.Lock()
	c.pending--
	if c.pending == 0 && c.busy {
		c.state = wire.AgentThinking
	}
	m.changedLocked(c, false)
	m.mu.Unlock()
	if row, ok := m.approvalOf(bg, st.ID); ok {
		st.ApprovalID = &row
	}
	now := time.Now()
	switch {
	case err != nil && errors.Is(err, context.Canceled):
		st.Status, st.Output, st.EndedAt = store.AgentStepCancelled, "Stopped", &now
		m.putStep(bg, *st)
		return false, "The user stopped this turn."
	case err != nil:
		st.Status, st.Output, st.EndedAt = store.AgentStepDenied, "No answer in time", &now
		m.putStep(bg, *st)
		return false, "No answer in time: the user didn't approve this."
	case !d.Approved:
		note := ""
		var p struct {
			Note string `json:"note"`
		}
		if json.Unmarshal(d.Payload, &p) == nil {
			note = strings.TrimSpace(p.Note)
		}
		st.Status, st.Output, st.EndedAt = store.AgentStepDenied, "You said no", &now
		if note != "" {
			st.Output += ": " + note
		}
		m.putStep(bg, *st)
		msg := "The user said no to this. Don't try to do it another way."
		if note != "" {
			msg += " Their note: " + note
		}
		return false, msg
	}
	if verdict == AskOnce {
		// Approved once: the rest of the chat doesn't ask again.
		if _, err := m.o.Repos.Runs.PutGrant(bg, store.Grant{Capability: req.Capability, Pattern: req.Pattern,
			Scope: "chat:" + c.thread.ID, Decision: string(connect.Allow)}); err != nil {
			slog.Warn("chat grant not kept", "chat", c.thread.ID, "err", err)
		}
	}
	st.Status = store.AgentStepRunning
	m.putStep(bg, *st)
	return true, ""
}

// approvalOf is the id of the newest approval of a step.
func (m *Manager) approvalOf(ctx context.Context, stepID string) (string, bool) {
	list, err := m.o.Repos.Runs.Approvals(ctx, "", 50)
	if err != nil {
		return "", false
	}
	for _, a := range list {
		if a.StepID == stepID {
			return a.ID, true
		}
	}
	return "", false
}

// Tool runs one of Gwen's tools for the chat a token belongs to (agent.sock):
// her policy first, then the tool, and the step with its undo.
func (m *Manager) Tool(ctx context.Context, token, name string, input json.RawMessage) (wire.AgentToolResult, error) {
	m.mu.Lock()
	id, ok := m.tokens[token]
	c := m.chats[id]
	m.mu.Unlock()
	if !ok || token == "" || c == nil {
		return wire.AgentToolResult{}, ErrNoToken
	}
	if len(input) == 0 {
		input = json.RawMessage("{}")
	}
	req, known := m.o.Sources.ToolRequest(name, input)
	if !known {
		return wire.AgentToolResult{OK: false, Error: fmt.Sprintf("Gwen has no tool %q", name)}, nil
	}
	key := name + " " + canonical(input)
	m.mu.Lock()
	var stepID string
	if ids := c.gwenSteps[key]; len(ids) > 0 {
		stepID, c.gwenSteps[key] = ids[0], ids[1:]
	}
	thread := c.thread
	turn := c.turn
	m.mu.Unlock()
	st, err := m.o.Repos.Agent.Step(ctx, stepID)
	if stepID == "" || err != nil {
		m.mu.Lock()
		st = store.AgentStep{ID: model.NewID(), ThreadID: thread.ID, Turn: turn, Position: c.position,
			Tool: "mcp__gwen__" + name, Input: input, StartedAt: time.Now()}
		c.position++
		m.mu.Unlock()
	}
	req.Grantable = req.Capability == CapSession
	req.Title = m.o.Sources.ToolTitle(name, input)
	st.Capability, st.Risk, st.Title = req.Capability, string(req.Risk), req.Title
	payload, _ := json.Marshal(map[string]any{"tool": name, "input": input})
	m.mu.Lock()
	turnCtx := c.turnCtx
	m.mu.Unlock()
	if ok, message := m.gate(turnCtx, c, &st, req, payload); !ok {
		return wire.AgentToolResult{OK: false, Error: message}, nil
	}
	res, undo, err := m.o.Sources.CallTool(ctx, thread, name, input)
	now := time.Now()
	st.EndedAt = &now
	if err != nil {
		res = wire.AgentToolResult{OK: false, Error: err.Error()}
	}
	if res.OK {
		st.Status, st.Output = store.AgentStepDone, res.Summary
		if len(undo) > 2 {
			st.Undo = undo
			res.Change = st.ID
		}
		if name == "session_start" {
			var d struct {
				SessionID string `json:"session_id"`
				Title     string `json:"title"`
			}
			if json.Unmarshal(res.Data, &d) == nil && d.SessionID != "" {
				// The chat's card finds its session by this mark.
				st.Output += "\n[session:" + d.SessionID + "]"
				m.mu.Lock()
				m.jobs[d.SessionID] = &job{chat: thread.ID, title: d.Title, state: wire.CodeStarting}
				m.mu.Unlock()
			}
		}
	} else {
		st.Status, st.Output = store.AgentStepFailed, res.Error
	}
	m.putStep(context.Background(), st)
	return res, nil
}

// Undo takes back the change a step made.
func (m *Manager) Undo(ctx context.Context, stepID string) (wire.AgentStep, error) {
	st, err := m.o.Repos.Agent.Step(ctx, stepID)
	if err != nil {
		return wire.AgentStep{}, err
	}
	if len(st.Undo) <= 2 {
		return wire.AgentStep{}, ErrNoUndo
	}
	if err := m.o.Sources.Undo(ctx, st.Undo); err != nil {
		return wire.AgentStep{}, err
	}
	st.Undo = json.RawMessage("{}")
	st.Output = "Undone: " + st.Output
	m.putStep(ctx, st)
	return StepWire(st), nil
}

// finish ends a turn on the CLI's result: her reply is stored, the memory
// hears of the turn, and the next queued message goes.
func (m *Manager) finish(c *chat, msg streamMsg) {
	ctx := context.Background()
	m.mu.Lock()
	reply := c.replyLocked()
	switch {
	case c.stopped:
		reply = strings.TrimSpace(reply + "\n\n_Stopped._")
	case reply == "":
		reply = strings.TrimSpace(msg.Result)
	}
	if msg.IsError && reply == "" {
		reply = "Something went wrong in Claude Code."
	}
	user := c.user
	m.stopTurnLocked(c)
	m.mu.Unlock()

	if reply != "" {
		if stored, err := m.o.Sources.AddMessage(ctx, c.thread.ID, model.ChatAssistant, reply); err != nil {
			slog.Warn("agent reply not kept", "chat", c.thread.ID, "err", err)
		} else if user.ID != "" {
			m.o.Sources.TurnDone(ctx, c.thread.ID, user, stored)
		}
	}
	usage, _ := json.Marshal(map[string]any{"cost_usd": msg.TotalCostUSD, "duration_ms": msg.DurationMS, "usage": msg.Usage})
	if ac, err := m.o.Repos.Agent.Chat(ctx, c.thread.ID); err == nil {
		ac.Usage, ac.State, ac.Error = usage, wire.AgentIdle, ""
		_ = m.o.Repos.Agent.PutChat(ctx, ac)
	}
	m.cancelOpen(ctx, c)
	m.next(c)
}

// stopTurnLocked ends a turn's timers and context. m.mu is held.
func (m *Manager) stopTurnLocked(c *chat) {
	if c.timeout != nil {
		c.timeout.Stop()
		c.timeout = nil
	}
	if c.textTimer != nil {
		c.textTimer.Stop()
		c.textTimer = nil
	}
	if c.endTurn != nil {
		c.endTurn()
		c.endTurn = nil
	}
	c.texts, c.user, c.stopped = nil, model.ChatMessage{}, false
	c.partial.Reset()
	c.gwenSteps = map[string][]string{}
}

// cancelOpen cancels a chat's steps left running or waiting.
func (m *Manager) cancelOpen(ctx context.Context, c *chat) {
	steps, err := m.o.Repos.Agent.Steps(ctx, c.thread.ID)
	if err != nil {
		return
	}
	now := time.Now()
	for _, st := range steps {
		if st.Status == store.AgentStepRunning || st.Status == store.AgentStepWaiting {
			st.Status, st.EndedAt = store.AgentStepCancelled, &now
			m.putStep(ctx, st)
		}
	}
}

// next makes a chat idle and sends its next queued message, or wakes a chat
// that waits for a process.
func (m *Manager) next(c *chat) {
	m.mu.Lock()
	c.busy, c.state, c.pending = false, wire.AgentIdle, 0
	var q *queued
	if len(c.queue) > 0 && c.proc != nil {
		q = &c.queue[0]
		c.queue = c.queue[1:]
		c.busy = true
	}
	if q == nil && c.proc != nil {
		p, gen := c.proc, c.gen
		if c.idle != nil {
			c.idle.Stop()
		}
		c.idle = time.AfterFunc(m.o.Config().Agent.IdleStop, func() { m.idleStop(c, p, gen) })
	}
	m.changedLocked(c, true)
	var wake *chat
	var wq queued
	if q == nil && len(m.waiting) > 0 {
		id := m.waiting[0]
		m.waiting = m.waiting[1:]
		if w := m.chats[id]; w != nil && len(w.queue) > 0 && !w.busy {
			wake, wq = w, w.queue[0]
			w.queue = w.queue[1:]
			w.busy = true
		}
	}
	m.mu.Unlock()
	if q != nil {
		go m.run(c, *q)
	}
	if wake != nil {
		go m.run(wake, wq)
	}
}

// idleStop ends a process that has had no turn for a while.
func (m *Manager) idleStop(c *chat, p *process, gen int) {
	m.mu.Lock()
	if c.proc != p || c.gen != gen || c.busy {
		m.mu.Unlock()
		return
	}
	c.proc = nil
	c.gen++
	m.dropTokenLocked(c)
	m.mu.Unlock()
	p.close()
	slog.Info("agent stopped while idle", "chat", c.thread.ID)
}

// timedOut interrupts a turn that ran too long.
func (m *Manager) timedOut(c *chat, p *process) {
	m.mu.Lock()
	if c.proc != p || !c.busy {
		m.mu.Unlock()
		return
	}
	c.texts = append(c.texts, "_Stopped: the turn ran longer than "+config.FormatDuration(m.o.Config().Agent.TurnTimeout)+"._")
	if c.endTurn != nil {
		c.endTurn()
	}
	m.mu.Unlock()
	_ = p.interrupt()
}

// Stop ends the turn under way and drops the queued messages.
func (m *Manager) Stop(threadID string) {
	m.mu.Lock()
	c := m.chats[threadID]
	if c == nil {
		m.mu.Unlock()
		return
	}
	c.queue = nil
	m.waiting = slices.DeleteFunc(m.waiting, func(id string) bool { return id == threadID })
	p := c.proc
	busy := c.busy
	c.stopped = busy
	if c.endTurn != nil {
		c.endTurn() // its approvals expire
	}
	m.changedLocked(c, false)
	m.mu.Unlock()
	if busy && p != nil {
		_ = p.interrupt()
	}
}

// Forget stops a chat's process and forgets it, when the chat is deleted.
func (m *Manager) Forget(ctx context.Context, threadID string) {
	m.mu.Lock()
	c := m.chats[threadID]
	var p *process
	if c != nil {
		p = c.proc
		c.proc = nil
		c.gen++
		if c.endTurn != nil {
			c.endTurn()
		}
		m.dropTokenLocked(c)
		delete(m.chats, threadID)
	}
	m.mu.Unlock()
	if p != nil {
		p.kill()
	}
	if err := m.o.Repos.Agent.Delete(ctx, threadID); err != nil {
		slog.Warn("agent chat not forgotten", "chat", threadID, "err", err)
	}
}

// exited handles a process that ended.
func (m *Manager) exited(c *chat, gen int, err error, stderr string) {
	m.mu.Lock()
	if gen != c.gen {
		m.mu.Unlock()
		return
	}
	busy := c.busy
	c.proc = nil
	c.gen++
	m.dropTokenLocked(c)
	m.mu.Unlock()
	if !busy {
		return
	}
	why := lastLine(stderr)
	if why == "" && err != nil {
		why = err.Error()
	}
	if why == "" {
		why = "it ended"
	}
	m.fail(c, "Claude Code stopped: "+why)
}

// fail ends a turn with an error the chat shows; the next message resumes.
func (m *Manager) fail(c *chat, why string) {
	ctx := context.Background()
	slog.Warn("agent turn failed", "chat", c.thread.ID, "why", why)
	m.mu.Lock()
	reply := c.replyLocked()
	user := c.user
	m.stopTurnLocked(c)
	m.mu.Unlock()
	if reply != "" {
		if stored, err := m.o.Sources.AddMessage(ctx, c.thread.ID, model.ChatAssistant, reply); err == nil && user.ID != "" {
			m.o.Sources.TurnDone(ctx, c.thread.ID, user, stored)
		}
	}
	m.cancelOpen(ctx, c)
	_ = m.o.Repos.Agent.SetState(ctx, c.thread.ID, wire.AgentError, why)
	m.mu.Lock()
	c.busy, c.pending = false, 0
	c.state, c.errText = wire.AgentError, why
	m.changedLocked(c, true)
	m.mu.Unlock()
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// changedLocked publishes a chat's state. m.mu is held.
func (m *Manager) changedLocked(c *chat, ended bool) {
	m.o.Publish(wire.EventAgentChanged, wire.AgentChanged{ChatID: c.thread.ID, State: c.state, Error: c.errText,
		Queued: len(c.queue), Ended: ended})
}

// Live is what this daemon knows of a chat right now: its state, error,
// queued messages, her reply so far, and the approvals waiting.
func (m *Manager) Live(threadID string) (state, errText string, queue []string, partial string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.chats[threadID]
	if c == nil {
		return wire.AgentIdle, "", []string{}, ""
	}
	queue = []string{}
	for _, q := range c.queue {
		if !q.system {
			queue = append(queue, q.text)
		}
	}
	if c.busy {
		partial = c.replyLocked()
	}
	return c.state, c.errText, queue, partial
}

// SessionsChanged follows the Claude sessions chats started: a finished job
// gives Gwen a turn to check it and report, and one that needs you or stops
// shows in its chat (plan.md 3.16). gwend calls it on code_session events.
func (m *Manager) SessionsChanged() {
	sessions := m.o.Sources.Sessions()
	byID := map[string]wire.CodeSession{}
	for _, cs := range sessions {
		byID[cs.ID] = cs
	}
	type event struct {
		j    job
		cs   wire.CodeSession
		kind string
	}
	var events []event
	m.mu.Lock()
	for id, j := range m.jobs {
		cs, ok := byID[id]
		if !ok {
			cs = wire.CodeSession{ID: id, State: wire.CodeEnded}
		}
		if cs.State == j.state {
			continue
		}
		j.state = cs.State
		switch {
		case cs.State == wire.CodeWorking:
			j.worked = true
		case cs.State == wire.CodeNeedsYou:
			events = append(events, event{*j, cs, "needs"})
		case (cs.State == wire.CodeDone || cs.State == wire.CodeIdle) && j.worked:
			j.worked = false
			events = append(events, event{*j, cs, "finished"})
		case cs.State == wire.CodeEnded || cs.State == wire.CodeError:
			events = append(events, event{*j, cs, "ended"})
			delete(m.jobs, id)
		}
	}
	m.mu.Unlock()
	for _, e := range events {
		m.jobEvent(e.j, e.cs, e.kind)
	}
}

// jobEvent puts a job's news in its chat, and on a finish starts Gwen's turn.
func (m *Manager) jobEvent(j job, cs wire.CodeSession, kind string) {
	ctx := context.Background()
	thread, err := m.o.Repos.Chats.Get(ctx, j.chat)
	if err != nil {
		return
	}
	c, err := m.load(ctx, thread)
	if err != nil {
		return
	}
	title := cmp.Or(j.title, cs.Name, "the job")
	last := strings.TrimSpace(cmp.Or(cs.LastMessage, cs.Step))
	st := store.AgentStep{ID: model.NewID(), ThreadID: thread.ID, Turn: thread.Messages + 1, Position: -1,
		Tool: "gwen:session", Capability: CapGwenRead, Risk: string(connect.Read), StartedAt: time.Now(),
		Input: mustJSON(map[string]string{"session_id": cs.ID, "kind": kind})}
	now := time.Now()
	switch kind {
	case "needs":
		what := "an answer"
		if cs.Waiting != nil && cs.Waiting.Summary != "" {
			what = cs.Waiting.Summary
		}
		st.Title, st.Output, st.Status = "Claude needs you: "+title, what, store.AgentStepWaiting
	case "finished":
		st.Title, st.Output, st.Status, st.EndedAt = "Claude finished: "+title, cutTail(last, 600), store.AgentStepDone, &now
	default:
		st.Title, st.Output, st.Status, st.EndedAt = "Claude stopped: "+title, cutTail(last, 600), store.AgentStepCancelled, &now
	}
	m.putStep(ctx, st)
	if kind != "finished" {
		return
	}
	q := queued{system: true, text: fmt.Sprintf("<gwen-event>\nThe Claude session you started for %q (session %s) "+
		"finished. Its last message:\n%s\n</gwen-event>\nCheck its work and tell the user in a few lines what it did, "+
		"whether it worked, and what is next.", title, cs.ID, cutTail(last, 2000))}
	if _, why := m.claude(m.o.Config()); why != "" {
		return
	}
	m.mu.Lock()
	if c.busy || len(c.queue) > 0 {
		c.queue = append(c.queue, q)
		m.mu.Unlock()
		return
	}
	c.busy = true
	m.mu.Unlock()
	go m.run(c, q)
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
