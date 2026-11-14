package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/connect"
)

// Capabilities of Gwen's agent (plan.md 3.16).
const (
	CapRead        = "fs.read"
	CapReadOutside = "fs.read_outside"
	CapWeb         = "web"
	CapGwenRead    = "gwen.read"
	CapGwenChange  = "gwen.change"
	// CapAppRead reads from an app connected to Composio (Gmail, Slack, ...).
	CapAppRead     = "app.read"
	CapEdit        = "fs.edit"
	CapShell       = "shell.run"
	CapSession     = "agent.session"
	CapExternal    = "external"
	CapDestructive = "destructive"
	CapFloor       = "floor"
	// CapOther is a tool the policy does not know, such as an MCP server's.
	CapOther = "tool"
)

// Verdict is what the policy says about one request.
type Verdict int

// Verdicts.
const (
	Allow Verdict = iota
	Ask
	// AskOnce asks, and an approval then covers the capability for the rest
	// of the chat.
	AskOnce
	Deny
)

// Request is one tool call as the policy sees it.
type Request struct {
	Capability string
	Risk       connect.Risk
	// Pattern narrows a remembered grant: a command's prefix, a folder.
	Pattern string
	// Grantable: a grant may cover it (one simple command, an edit, ...).
	Grantable bool
	// Plain: a command a word list can judge (Command.Plain). Trusted runs
	// only plain commands without asking.
	Plain bool
	// Title is the plain words a step and an approval show.
	Title string
	// Why says why it is floor, destructive, or external.
	Why string
}

// Place is where a chat works: its folder (may be ""), the user's home, Gwen's
// runtime folder, and the gwen binary, which the floor protects.
type Place struct {
	Folder  string
	Home    string
	Runtime string
	GwenBin string
}

// inside reports whether path is the folder or under it, both as written
// and with symbolic links followed, so a link inside the folder that points
// out of it does not count as inside.
func (p Place) inside(path string) bool {
	if p.Folder == "" || path == "" {
		return false
	}
	if !under(filepath.Clean(p.Folder), filepath.Clean(path)) {
		return false
	}
	folder, ok1 := resolve(p.Folder)
	real, ok2 := resolve(path)
	return ok1 && ok2 && under(folder, real)
}

func under(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}

// resolve is path with its symbolic links followed. A path that does not
// exist yet (a new file) resolves through its deepest existing parent; false
// when the path cannot be resolved.
func resolve(path string) (string, bool) {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return "", false
	}
	rest := ""
	for dir := path; ; dir = filepath.Dir(dir) {
		real, err := filepath.EvalSymlinks(dir)
		if err == nil {
			return filepath.Join(real, rest), true
		}
		if !os.IsNotExist(err) || dir == filepath.Dir(dir) {
			return "", false
		}
		rest = filepath.Join(filepath.Base(dir), rest)
	}
}

// floorOf is floorPath for a path both as written and resolved.
func (p Place) floorOf(path string) string {
	if why := floorPath(path, p.Runtime); why != "" {
		return why
	}
	if real, ok := resolve(path); ok {
		return floorPath(real, p.Runtime)
	}
	return ""
}

// short is a path as a step says it: relative inside the folder, with ~ for
// the home folder elsewhere.
func (p Place) short(path string) string {
	if p.inside(path) {
		if rel, err := filepath.Rel(p.Folder, path); err == nil {
			return rel
		}
	}
	if p.Home != "" && strings.HasPrefix(path, p.Home+"/") {
		return "~" + path[len(p.Home):]
	}
	return path
}

// toolInput is the fields of Claude Code's tool inputs the policy reads.
type toolInput struct {
	Command      string `json:"command"`
	FilePath     string `json:"file_path"`
	NotebookPath string `json:"notebook_path"`
	Path         string `json:"path"`
	Pattern      string `json:"pattern"`
	URL          string `json:"url"`
	Query        string `json:"query"`
	Description  string `json:"description"`
}

// Classify turns one of Claude Code's permission requests into a request:
// tool is its name, input its input, blocked the path it names as blocked.
func Classify(tool string, input json.RawMessage, blocked string, at Place) Request {
	var in toolInput
	_ = json.Unmarshal(input, &in)
	switch tool {
	case "Bash", "BashOutput":
		c := ClassifyCommand(in.Command, at.Home, at.Runtime, at.GwenBin)
		title := "Run `" + oneLine(in.Command, 120) + "`"
		switch c.Class {
		case classFloor:
			return Request{Capability: CapFloor, Title: title, Why: c.Why}
		case classDestructive:
			return Request{Capability: CapDestructive, Risk: connect.Destructive, Title: title, Why: c.Why}
		case classExternal:
			return Request{Capability: CapExternal, Risk: connect.External, Title: title, Why: c.Why}
		}
		return Request{Capability: CapShell, Risk: connect.Write, Pattern: c.Prefix, Grantable: c.Simple && c.Prefix != "",
			Plain: c.Plain, Title: title}
	case "Edit", "MultiEdit", "Write", "NotebookEdit":
		path := firstNonEmpty(in.FilePath, in.NotebookPath, blocked)
		verb := map[string]string{"Write": "Write", "NotebookEdit": "Edit notebook"}[tool]
		if verb == "" {
			verb = "Edit"
		}
		title := verb + " " + at.short(path)
		if why := at.floorOf(path); why != "" {
			return Request{Capability: CapFloor, Title: title, Why: why}
		}
		if !at.inside(path) {
			return Request{Capability: CapDestructive, Risk: connect.Destructive, Title: title,
				Why: "it writes outside the project's folder"}
		}
		return Request{Capability: CapEdit, Risk: connect.Write, Grantable: true, Title: title}
	case "Read", "Glob", "Grep", "LS", "NotebookRead":
		path := firstNonEmpty(blocked, in.FilePath, in.Path, in.NotebookPath)
		if path == "" && (tool == "Glob" || tool == "Grep" || tool == "LS") {
			path = at.Folder // a search without a path runs in the folder
		}
		title := "Read " + at.short(path)
		if tool == "Glob" || tool == "Grep" {
			title = "Search for " + oneLine(in.Pattern, 60)
			if p := at.short(path); p != "" && p != "." {
				title += " in " + p
			}
		}
		if why := at.floorOf(path); why != "" {
			return Request{Capability: CapFloor, Title: title, Why: why}
		}
		if at.inside(path) {
			return Request{Capability: CapRead, Risk: connect.Read, Title: title}
		}
		dir := path
		if tool == "Read" || tool == "NotebookRead" {
			dir = filepath.Dir(path)
		}
		return Request{Capability: CapReadOutside, Risk: connect.Read, Pattern: filepath.Clean(dir), Grantable: true,
			Title: title}
	case "WebFetch":
		return Request{Capability: CapWeb, Risk: connect.Read, Grantable: true, Title: "Fetch " + oneLine(in.URL, 120)}
	case "WebSearch":
		return Request{Capability: CapWeb, Risk: connect.Read, Grantable: true, Title: "Search the web for " +
			oneLine(in.Query, 100)}
	case "Task", "Agent", "TodoWrite", "TodoRead", "ExitPlanMode", "EnterPlanMode", "ToolSearch", "Skill":
		return Request{Capability: CapRead, Risk: connect.Read, Title: tool}
	}
	if strings.HasPrefix(tool, "mcp__gwen__") {
		// Allowed in Claude Code and checked by gwend at the call (Manager.Tool).
		return Request{Capability: CapGwenRead, Risk: connect.Read, Title: strings.TrimPrefix(tool, "mcp__gwen__")}
	}
	return Request{Capability: CapOther, Risk: connect.Write, Pattern: tool, Grantable: true, Title: "Use " + tool}
}

// Decide is the policy table of plan.md 3.16: what a request does under an
// autonomy, before grants.
func Decide(r Request, autonomy string) Verdict {
	switch r.Capability {
	case CapFloor:
		return Deny
	case CapExternal, CapDestructive:
		return Ask
	case CapRead, CapGwenRead, CapAppRead:
		return Allow
	}
	switch autonomy {
	case config.AutonomyTrusted:
		// The floor reads words; a command that can hide words from it asks.
		if r.Capability == CapShell && !r.Plain {
			return Ask
		}
		// A fetch can carry out what she read (mail, messages) in its URL,
		// so the web asks once per chat even here.
		if r.Capability == CapWeb {
			return AskOnce
		}
		return Allow
	case config.AutonomyCautious:
		return Ask
	}
	switch r.Capability { // balanced
	case CapGwenChange:
		return Allow
	case CapWeb, CapEdit, CapSession:
		return AskOnce
	}
	return Ask
}

// GrantScopes are the scopes an approval of r may be remembered for.
func GrantScopes(r Request, chat, project string) []string {
	if !r.Grantable || r.Risk == connect.External || r.Risk == connect.Destructive {
		return nil
	}
	var out []string
	switch r.Capability {
	case CapWeb, CapEdit, CapSession:
		out = append(out, "chat:"+chat)
	}
	if project != "" {
		out = append(out, "project:"+project)
	}
	return out
}

func firstNonEmpty(s ...string) string {
	for _, x := range s {
		if x != "" {
			return x
		}
	}
	return ""
}

// oneLine is s on one line, cut to n characters.
func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// riskWords is a risk as a card says it.
func riskWords(r Request) string {
	switch r.Capability {
	case CapExternal:
		return fmt.Sprintf("Leaves this computer: %s", r.Why)
	case CapDestructive:
		return fmt.Sprintf("Can't be undone: %s", r.Why)
	}
	return ""
}
