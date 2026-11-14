package agent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Spec is how a chat's Claude Code process starts (plan.md 3.16).
type Spec struct {
	// Claude is the CLI; Scope is systemd-run when the CLI must run in a
	// scope of its own, outside gwend.service's limits ("" runs it directly).
	Claude string
	Scope  string
	Dir    string
	// Session is the Claude session id; Resume continues it.
	Session string
	Resume  bool
	Model   string
	// MCPConfig and Prompt are the files of Gwen's MCP server and the
	// system prompt.
	MCPConfig string
	Prompt    string
	// Wrap runs the CLI through a wrapper, such as `gwen sandbox`; nil runs
	// it as it is.
	Wrap []string
}

// chatTools are Claude Code's own tools in Gwen's chat: reading and
// searching only. She orchestrates; every change and command goes to a
// Claude session she starts (plan.md 3.16).
const chatTools = "Read,Glob,Grep,WebFetch,WebSearch"

// args are the CLI's arguments, checked against Claude Code 2.1.323. Gwen's
// own prompt replaces Claude Code's, so she speaks and acts as Gwen.
func (s Spec) args() []string {
	a := []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose",
		"--include-partial-messages"}
	if s.Resume {
		a = append(a, "--resume", s.Session)
	} else {
		a = append(a, "--session-id", s.Session)
	}
	if s.Model != "" {
		a = append(a, "--model", s.Model)
	}
	return append(a, "--setting-sources", "", "--tools", chatTools, "--strict-mcp-config", "--mcp-config", s.MCPConfig,
		"--allowedTools", "mcp__gwen", "--permission-mode", "default", "--permission-prompts", "host",
		"--permission-prompt-tool", "stdio", "--system-prompt-file", s.Prompt)
}

// apiKeyEnv would make the CLI bill an API key rather than the subscription.
var apiKeyEnv = []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"}

// process is a running Claude Code process speaking stream-json.
type process struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stderr *tail
	mu     sync.Mutex // writes
	done   chan struct{}
	err    error
}

// start runs the CLI and reads its stdout line by line into onLine until it
// ends; then onExit gets how it ended.
func start(s Spec, onLine func([]byte), onExit func(error, string)) (*process, error) {
	argv := []string{s.Claude}
	if len(s.Wrap) > 0 {
		argv = append([]string{}, s.Wrap...) // the wrapper finds the CLI itself
	}
	argv = append(argv, s.args()...)
	name, args := argv[0], argv[1:]
	if s.Scope != "" {
		name, args = s.Scope, append([]string{"--user", "--scope", "--quiet", "--collect",
			"--description=Gwen's agent", "--"}, argv...)
	}
	cmd := exec.Command(name, args...)
	cmd.Dir = s.Dir
	env := slices.DeleteFunc(os.Environ(), func(kv string) bool {
		key, _, _ := strings.Cut(kv, "=")
		return slices.Contains(apiKeyEnv, key) || key == "PWD"
	})
	cmd.Env = append(env, "PWD="+s.Dir, "GWEN_AGENT=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	p := &process{cmd: cmd, stdin: stdin, stderr: &tail{max: 4 << 10}, done: make(chan struct{})}
	cmd.Stderr = p.stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start Claude Code: %w", err)
	}
	go func() {
		r := bufio.NewReader(stdout)
		for {
			line, err := r.ReadBytes('\n')
			if len(bytes.TrimSpace(line)) > 0 {
				onLine(line)
			}
			if err != nil {
				break
			}
		}
		p.err = cmd.Wait()
		close(p.done)
		onExit(p.err, p.stderr.String())
	}()
	// Claude Code waits for the host to introduce itself (checked).
	if err := p.send(map[string]any{"type": "control_request", "request_id": "init",
		"request": map[string]any{"subtype": "initialize", "hooks": nil}}); err != nil {
		p.kill()
		return nil, err
	}
	return p, nil
}

// send writes one JSON line to the process.
func (p *process) send(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	_, err = p.stdin.Write(append(b, '\n'))
	return err
}

// user sends a message of yours: the context note, then your text.
func (p *process) user(note, text string) error {
	content := []map[string]string{}
	if note != "" {
		content = append(content, map[string]string{"type": "text", "text": note})
	}
	content = append(content, map[string]string{"type": "text", "text": text})
	return p.send(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": content}})
}

// answer answers a permission request: allow with the input to use, or deny
// with a message the model reads as the tool's error.
func (p *process) answer(requestID string, allow bool, input json.RawMessage, message string) error {
	resp := map[string]any{"behavior": "deny", "message": message}
	if allow {
		resp = map[string]any{"behavior": "allow", "updatedInput": input}
	}
	return p.send(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success",
		"request_id": requestID, "response": resp}})
}

// interrupt ends the turn under way.
func (p *process) interrupt() error {
	return p.send(map[string]any{"type": "control_request", "request_id": fmt.Sprintf("int-%d", time.Now().UnixNano()),
		"request": map[string]any{"subtype": "interrupt"}})
}

// close ends the process: stdin closes, so the CLI finishes and exits; if it
// is still there after a while it is killed.
func (p *process) close() {
	p.mu.Lock()
	_ = p.stdin.Close()
	p.mu.Unlock()
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
		p.kill()
	}
}

// kill stops the process and everything it started.
func (p *process) kill() {
	if p.cmd.Process != nil {
		_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
	}
}

// tail keeps the last max bytes written to it.
type tail struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (t *tail) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, b...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(b), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

// streamMsg is one line of the CLI's stream-json output, as far as Gwen
// reads it.
type streamMsg struct {
	Type      string          `json:"type"`
	Subtype   string          `json:"subtype"`
	SessionID string          `json:"session_id"`
	RequestID string          `json:"request_id"`
	Request   json.RawMessage `json:"request"`
	Message   *struct {
		ID      string  `json:"id"`
		Content []block `json:"content"`
	} `json:"message"`
	Event *struct {
		Type  string `json:"type"`
		Delta *struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
	} `json:"event"`
	ParentToolUseID *string         `json:"parent_tool_use_id"`
	IsError         bool            `json:"is_error"`
	Result          string          `json:"result"`
	Usage           json.RawMessage `json:"usage"`
	TotalCostUSD    float64         `json:"total_cost_usd"`
	DurationMS      int64           `json:"duration_ms"`
	ToolUseResult   json.RawMessage `json:"tool_use_result"`
}

// block is a content block of a message.
type block struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	Text      string          `json:"text"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

// resultText is a tool result's text: a string, or the text blocks of a list.
func (b block) resultText() string {
	var s string
	if json.Unmarshal(b.Content, &s) == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(b.Content, &parts) != nil {
		return ""
	}
	var out []string
	for _, p := range parts {
		if p.Type == "text" {
			out = append(out, p.Text)
		}
	}
	return strings.Join(out, "\n")
}

// permissionRequest is a can_use_tool request.
type permissionRequest struct {
	Subtype     string          `json:"subtype"`
	ToolName    string          `json:"tool_name"`
	Input       json.RawMessage `json:"input"`
	ToolUseID   string          `json:"tool_use_id"`
	BlockedPath string          `json:"blocked_path"`
	Description string          `json:"description"`
}
