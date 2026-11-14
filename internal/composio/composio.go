// Package composio attaches things from other apps to tasks: an email, a
// calendar event, a GitHub issue or pull request, a Notion page. It runs the
// `composio` command already installed and logged in on this computer, so
// Gwen holds no app tokens or Composio key of its own; Composio keeps them.
//
// Search finds items to pick from, and Fetch takes one item's text at the
// moment it is attached (a snapshot), which agents and Gwen chat read later
// without calling the app again.
package composio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// AppInfo is an app Gwen can attach from.
type AppInfo struct{ Slug, Name string }

// Apps Gwen can attach from, in picker order.
var Apps = []AppInfo{{Slug: "gmail", Name: "Gmail"}, {Slug: "googlecalendar", Name: "Google Calendar"}, {Slug: "github", Name: "GitHub"}, {Slug: "notion", Name: "Notion"}}

// Connection statuses.
const (
	StatusActive      = "active"
	StatusExpired     = "expired"
	StatusMissing     = "missing"
	StatusUnavailable = "unavailable"
)

// Connection is one app's state in Composio.
type Connection struct {
	App    string // slug
	Name   string
	Status string // "active", "expired", "missing" (not connected), "unavailable" (no CLI / not logged in)
}

// Item is one search result.
type Item struct {
	App     string
	Ref     string // the id Fetch takes (message id, event id, "owner/repo#123", page id)
	Title   string
	URL     string // a link a person can open in the browser
	Snippet string // one or two lines for the picker
	When    *time.Time
}

// Snapshot is an item with its text, kept on the task.
type Snapshot struct {
	Item
	Body string // plain text, at most maxBody runes
}

// maxBody caps a snapshot's text.
const maxBody = 20000

// defaultTimeout bounds one composio command when the client sets none.
const defaultTimeout = 45 * time.Second

// ErrUnavailable means the composio command is missing or not logged in.
// Errors that mean it carry a message saying which; test with errors.Is.
var ErrUnavailable = errors.New("composio isn't set up on this computer")

// UnavailableError is ErrUnavailable with the reason in words.
type UnavailableError struct{ Msg string }

func (e *UnavailableError) Error() string        { return e.Msg }
func (e *UnavailableError) Is(target error) bool { return target == ErrUnavailable }

// ErrNotConnected means the app isn't connected to Composio, or its sign-in
// expired. Test with errors.As.
type ErrNotConnected struct{ App string }

func (e ErrNotConnected) Error() string {
	return fmt.Sprintf("%s isn't connected to Composio, or its sign-in expired. Connect it again and try once more.", appName(e.App))
}

// ToolError is an app's own answer that something failed, in words.
type ToolError struct {
	App string
	Msg string
}

func (e *ToolError) Error() string { return fmt.Sprintf("%s: %s", appName(e.App), e.Msg) }

// runner runs a command and returns what it printed. It is a field so the
// package can run without the real composio command.
type runner func(ctx context.Context, env []string, name string, args ...string) (stdout, stderr []byte, err error)

// Client runs the composio command.
type Client struct {
	Bin     string        // default: "composio" on PATH, then ~/.local/bin and ~/.composio
	Timeout time.Duration // per command; default 45s
	run     runner
	mu      sync.Mutex
	hints   map[string]hint
	clock   func() time.Time
}

func execRunner(ctx context.Context, env []string, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	cmd.Stdin = nil
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	return out.Bytes(), errb.Bytes(), err
}

// bin finds the composio command. A user service often has a short PATH, so
// the usual install places are tried after it.
func (c *Client) bin() string {
	if c.Bin != "" {
		return c.Bin
	}
	if p, err := exec.LookPath("composio"); err == nil {
		return p
	}
	if home, err := os.UserHomeDir(); err == nil {
		for _, p := range []string{filepath.Join(home, ".local", "bin", "composio"), filepath.Join(home, ".composio", "composio")} {
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
	}
	return "composio"
}

// command runs `composio args...` with its own private TMPDIR: the command
// writes large answers to a file there, which is read and then removed with
// the folder, so no app content stays behind in /tmp.
func (c *Client) command(ctx context.Context, app string, args ...string) (stdout []byte, tmp string, err error) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	tmp, err = os.MkdirTemp("", "gwen-composio-")
	if err != nil {
		return nil, "", err
	}
	env := append(os.Environ(), "TMPDIR="+tmp, "NO_COLOR=1")
	run := c.run
	if run == nil {
		run = execRunner
	}
	start := time.Now()
	out, errOut, err := run(ctx, env, c.bin(), args...)
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
	}
	slog.Info("composio call", "cmd", cmd, "tool", toolOf(args), "ms", time.Since(start).Milliseconds(), "ok", err == nil)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			os.RemoveAll(tmp)
			return nil, "", &UnavailableError{Msg: "Composio isn't installed on this computer. Install the composio command and log in to it to attach from other apps."}
		}
		if ctx.Err() == context.DeadlineExceeded {
			os.RemoveAll(tmp)
			return nil, "", &ToolError{App: app, Msg: "it took too long to answer. Try again in a moment."}
		}
		// A failed tool still prints its JSON answer and exits 1; that answer says more.
		if bytes.IndexByte(out, '{') < 0 {
			os.RemoveAll(tmp)
			if loginNeeded(string(out) + string(errOut)) {
				return nil, "", &UnavailableError{Msg: "Composio is logged out on this computer. Run composio login in a terminal, then try again."}
			}
			return nil, "", &ToolError{App: app, Msg: firstLine(string(errOut) + string(out))}
		}
	}
	return out, tmp, nil
}

// answer is what `composio execute` prints. A large answer is written to a
// file instead and the printed one names it.
type answer struct {
	Successful     bool            `json:"successful"`
	Data           json.RawMessage `json:"data"`
	Error          any             `json:"error"`
	Slug           string          `json:"slug"`
	StoredInFile   bool            `json:"storedInFile"`
	OutputFilePath string          `json:"outputFilePath"`
}

// execute runs one tool with its arguments and decodes its data into out.
func (c *Client) execute(ctx context.Context, app, tool string, args map[string]any, out any) error {
	data, err := json.Marshal(args)
	if err != nil {
		return err
	}
	stdout, tmp, err := c.command(ctx, app, "execute", tool, "-d", string(data))
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	a, err := decodeAnswer(stdout)
	if err != nil {
		return &ToolError{App: app, Msg: "Composio gave an answer Gwen couldn't read."}
	}
	if a.StoredInFile && a.OutputFilePath != "" {
		// Only read the file the command wrote into our own folder.
		if rel, err := filepath.Rel(tmp, a.OutputFilePath); err != nil || strings.HasPrefix(rel, "..") {
			return &ToolError{App: app, Msg: "Composio put its answer somewhere Gwen won't read."}
		}
		b, err := os.ReadFile(a.OutputFilePath)
		if err != nil {
			return &ToolError{App: app, Msg: "Composio's answer went missing."}
		}
		if a, err = decodeAnswer(b); err != nil {
			return &ToolError{App: app, Msg: "Composio gave an answer Gwen couldn't read."}
		}
	}
	if !a.Successful {
		return toolFailure(app, a)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(a.Data, out); err != nil {
		return &ToolError{App: app, Msg: "the answer wasn't in the shape Gwen expects."}
	}
	return nil
}

// decodeAnswer reads the JSON object in the command's output, past anything
// printed before it.
func decodeAnswer(b []byte) (*answer, error) {
	i := bytes.IndexByte(b, '{')
	if i < 0 {
		return nil, errors.New("no JSON in output")
	}
	var a answer
	if err := json.NewDecoder(bytes.NewReader(b[i:])).Decode(&a); err != nil {
		return nil, err
	}
	return &a, nil
}

// toolFailure turns a failed answer into an error a person can read.
func toolFailure(app string, a *answer) error {
	msg := errorText(a.Error)
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(a.Slug, "NoActiveConnection"), strings.Contains(low, "no active connection"), strings.Contains(low, "composio link"),
		strings.Contains(low, "expired"), strings.Contains(low, "not connected"):
		return ErrNotConnected{App: app}
	case loginNeeded(msg):
		return &UnavailableError{Msg: "Composio is logged out on this computer. Run composio login in a terminal, then try again."}
	case strings.Contains(low, "not found"), strings.Contains(low, "404"):
		return &ToolError{App: app, Msg: "that item wasn't found. It may have been deleted, or it isn't shared with you."}
	}
	if msg == "" {
		msg = "something went wrong."
	}
	return &ToolError{App: app, Msg: msg}
}

// errorText is the first readable line of an error field, which is a string
// or an object depending on the tool.
func errorText(v any) string {
	switch e := v.(type) {
	case nil:
		return ""
	case string:
		// Some tools pass the app's own JSON error through as text.
		if t := strings.TrimSpace(e); strings.HasPrefix(t, "{") {
			var inner any
			if json.Unmarshal([]byte(t), &inner) == nil {
				if s := errorText(inner); s != "" {
					return s
				}
			}
		}
		return firstLine(e)
	case map[string]any:
		for _, k := range []string{"message", "error", "detail", "error_description"} {
			if s := errorText(e[k]); s != "" {
				return s
			}
		}
		return ""
	}
	b, _ := json.Marshal(v)
	return firstLine(string(b))
}

func loginNeeded(s string) bool {
	low := strings.ToLower(s)
	return strings.Contains(low, "composio login") || strings.Contains(low, "not logged in") || strings.Contains(low, "unauthenticated") || strings.Contains(low, "invalid api key")
}

// firstLine is the first non-empty line, cut to a sentence's length.
func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return cut(l, 200)
		}
	}
	return ""
}

// Connections reports each of Apps' state in Composio. When Composio can't be
// reached every app is "unavailable" and the error says why.
func (c *Client) Connections(ctx context.Context) ([]Connection, error) {
	list := make([]Connection, len(Apps))
	for i, a := range Apps {
		list[i] = Connection{App: a.Slug, Name: a.Name, Status: StatusMissing}
	}
	unavailable := func(err error) ([]Connection, error) {
		for i := range list {
			list[i].Status = StatusUnavailable
		}
		return list, err
	}
	out, tmp, err := c.command(ctx, "", "connections", "list")
	if err != nil {
		if !errors.Is(err, ErrUnavailable) {
			err = &UnavailableError{Msg: "Composio didn't answer: " + err.Error()}
		}
		return unavailable(err)
	}
	defer os.RemoveAll(tmp)
	i := bytes.IndexByte(out, '{')
	var accounts map[string][]struct {
		Status string `json:"status"`
	}
	if i < 0 || json.NewDecoder(bytes.NewReader(out[i:])).Decode(&accounts) != nil {
		if loginNeeded(string(out)) {
			return unavailable(&UnavailableError{Msg: "Composio is logged out on this computer. Run composio login in a terminal, then try again."})
		}
		return unavailable(&UnavailableError{Msg: "Composio's list of connected apps couldn't be read."})
	}
	for i, a := range Apps {
		for _, acc := range accounts[a.Slug] {
			switch strings.ToUpper(acc.Status) {
			case "ACTIVE":
				list[i].Status = StatusActive
			case "EXPIRED", "INACTIVE", "FAILED":
				if list[i].Status != StatusActive {
					list[i].Status = StatusExpired
				}
			}
		}
	}
	return list, nil
}

// Search finds an app's items matching query, at most limit of them. An empty
// query lists recent ones: the latest mail, upcoming events, issues and pull
// requests you're involved in, recently edited pages.
func (c *Client) Search(ctx context.Context, app, query string, limit int) ([]Item, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 25 {
		limit = 25
	}
	query = strings.TrimSpace(query)
	var items []Item
	var err error
	switch app {
	case "gmail":
		items, err = c.searchGmail(ctx, query, limit)
	case "googlecalendar":
		items, err = c.searchCalendar(ctx, query, limit)
	case "github":
		items, err = c.searchGitHub(ctx, query, limit)
	case "notion":
		items, err = c.searchNotion(ctx, query, limit)
	default:
		return nil, unknownApp(app)
	}
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].unescape()
	}
	return items, nil
}

// unescape turns HTML entities the apps leave in titles and previews ("&#39;")
// into the characters they stand for.
func (it *Item) unescape() {
	it.Title = html.UnescapeString(it.Title)
	it.Snippet = html.UnescapeString(it.Snippet)
}

// Fetch takes one item's text now, for keeping on a task.
func (c *Client) Fetch(ctx context.Context, app, ref string) (*Snapshot, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, &ToolError{App: app, Msg: "no item was picked."}
	}
	var s *Snapshot
	var err error
	switch app {
	case "gmail":
		s, err = c.fetchGmail(ctx, ref)
	case "googlecalendar":
		s, err = c.fetchCalendar(ctx, ref)
	case "github":
		s, err = c.fetchGitHub(ctx, ref)
	case "notion":
		s, err = c.fetchNotion(ctx, ref)
	default:
		return nil, unknownApp(app)
	}
	if err != nil {
		return nil, err
	}
	s.unescape()
	s.Body = capRunes(strings.TrimSpace(html.UnescapeString(s.Body)), maxBody)
	return s, nil
}

var linkURL = regexp.MustCompile(`https://[^\s"'<>]+`)

// LinkURL asks Composio for the page that connects (or reconnects) an app,
// for the person to open in their browser. When Composio can't give one
// without asking questions, the error says to run `composio link <app>`.
func (c *Client) LinkURL(ctx context.Context, app string) (string, error) {
	if !known(app) {
		return "", unknownApp(app)
	}
	out, tmp, err := c.command(ctx, app, "link", app, "--no-wait", "--no-browser")
	if tmp != "" {
		defer os.RemoveAll(tmp)
	}
	if err == nil {
		// The JSON form names the URL in a field; the text form prints it on its own.
		if a, derr := decodeAnswer(out); derr == nil && a.Error != nil && !a.Successful {
			err = &ToolError{App: app, Msg: errorText(a.Error)}
		} else if u := linkURL.FindString(string(out)); u != "" {
			return strings.TrimRight(u, ".,);"), nil
		}
	}
	if errors.Is(err, ErrUnavailable) {
		return "", err
	}
	return "", &ToolError{App: app, Msg: fmt.Sprintf("Composio couldn't make a sign-in link here. Run composio link %s in a terminal to connect it.", app)}
}

func known(slug string) bool {
	for _, a := range Apps {
		if a.Slug == slug {
			return true
		}
	}
	return false
}

func appName(slug string) string {
	for _, a := range Apps {
		if a.Slug == slug {
			return a.Name
		}
	}
	if slug == "" {
		return "Composio"
	}
	return slug
}

func unknownApp(app string) error {
	return &ToolError{App: app, Msg: "Gwen can't attach from this app yet."}
}

func toolOf(args []string) string {
	if len(args) > 1 && args[0] == "execute" {
		return args[1]
	}
	return ""
}
