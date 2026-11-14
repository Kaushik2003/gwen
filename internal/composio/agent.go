package composio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Gwen's agent reaches any app connected to Composio (plan.md 3.16): Accounts
// lists them, FindTools finds the tools for a job with Composio's own plan and
// pitfalls, ToolSchema says what a tool takes, and Run runs one. What a tool
// may do without asking is the agent's policy (ToolKind), not this package's.

// Account is an app connected to Composio.
type Account struct {
	App    string `json:"app"`               // toolkit slug, e.g. "slack"
	Name   string `json:"account,omitempty"` // the account's own name, e.g. "you (Acme)"
	Status string `json:"status"`            // "active" or "expired"
}

// UseCase is one of Composio's answers to a search: the tools for a job and
// how to use them.
type UseCase struct {
	UseCase  string   `json:"use_case"`
	Apps     []string `json:"apps"`
	Plan     []string `json:"plan"`
	Pitfalls []string `json:"pitfalls,omitempty"`
	Tools    []string `json:"tools"`
	Related  []string `json:"related_tools,omitempty"`
}

// ToolInfo is a tool with the input it takes (a JSON schema, trimmed).
type ToolInfo struct {
	Tool  string          `json:"tool"`
	Input json.RawMessage `json:"input_schema"`
}

// Found is what FindTools found.
type Found struct {
	UseCases []UseCase  `json:"use_cases"`
	Tools    []ToolInfo `json:"tools"`
	// Connected are the found apps that are connected; the rest must be
	// connected by the user first.
	Connected []string `json:"connected_apps"`
}

// maxSchema caps one tool's schema, maxRun a tool's answer, in bytes.
const (
	maxSchema = 6000
	maxRun    = 12000
)

var (
	slugPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{2,120}$`)
	appsPattern = regexp.MustCompile(`^[a-z0-9_]+(,[a-z0-9_]+)*$`)
)

// checkSlug refuses what isn't a tool slug, and Composio's own tools (its
// remote workbench and shell run code with every connected account).
func checkSlug(slug string) error {
	if !slugPattern.MatchString(slug) {
		return &ToolError{Msg: fmt.Sprintf("%q isn't a tool name; tool names look like SLACK_SEND_MESSAGE.", slug)}
	}
	if strings.HasPrefix(slug, "COMPOSIO_") {
		return &ToolError{Msg: "Gwen doesn't run Composio's own tools (its workbench and remote shell); use an app's tools."}
	}
	return nil
}

// toolkitOf is the app a tool slug belongs to, as best told from its name.
func toolkitOf(slug string) string {
	app, _, _ := strings.Cut(slug, "_")
	return strings.ToLower(app)
}

// Accounts lists every app connected to Composio, active ones first.
func (c *Client) Accounts(ctx context.Context) ([]Account, error) {
	out, tmp, err := c.command(ctx, "", "connections", "list")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	i := bytes.IndexByte(out, '{')
	var raw map[string][]struct {
		Status      string `json:"status"`
		DisplayName string `json:"display_name"`
	}
	if i < 0 || json.NewDecoder(bytes.NewReader(out[i:])).Decode(&raw) != nil {
		if loginNeeded(string(out)) {
			return nil, &UnavailableError{Msg: "Composio is logged out on this computer. Run composio login in a terminal, then try again."}
		}
		return nil, &UnavailableError{Msg: "Composio's list of connected apps couldn't be read."}
	}
	var list []Account
	for app, accs := range raw {
		a := Account{App: app}
		for _, acc := range accs {
			switch strings.ToUpper(acc.Status) {
			case "ACTIVE":
				if a.Status != StatusActive {
					a.Status, a.Name = StatusActive, acc.DisplayName
				}
			case "EXPIRED", "INACTIVE", "FAILED":
				if a.Status == "" {
					a.Status = StatusExpired
				}
			}
		}
		if a.Status != "" {
			list = append(list, a)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if (list[i].Status == StatusActive) != (list[j].Status == StatusActive) {
			return list[i].Status == StatusActive
		}
		return list[i].App < list[j].App
	})
	return list, nil
}

// FindTools asks Composio which tools do a job, in words ("send a Slack
// message to #general"); apps narrows it to some toolkits ("slack,gmail").
func (c *Client) FindTools(ctx context.Context, query, apps string) (*Found, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, &ToolError{Msg: "say what you want to do, e.g. \"send a Slack message\"."}
	}
	args := []string{"search", query, "--limit", "3"}
	if apps = strings.TrimSpace(apps); apps != "" {
		if !appsPattern.MatchString(apps) {
			return nil, &ToolError{Msg: "apps is a comma-separated list of app slugs, e.g. \"slack,gmail\"."}
		}
		args = append(args, "--toolkits", apps)
	}
	out, tmp, err := c.command(ctx, "", args...)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	i := bytes.IndexByte(out, '{')
	var raw struct {
		Results []struct {
			UseCase  string   `json:"use_case"`
			Plan     []string `json:"recommended_plan_steps"`
			Pitfalls []string `json:"known_pitfalls"`
			Primary  []string `json:"primary_tool_slugs"`
			Related  []string `json:"related_tool_slugs"`
			Toolkits []string `json:"toolkits"`
		} `json:"results"`
		Schemas struct {
			Primary map[string]string `json:"primary"`
		} `json:"tool_schemas"`
		Connected []string `json:"connected_toolkits"`
	}
	if i < 0 || json.NewDecoder(bytes.NewReader(out[i:])).Decode(&raw) != nil {
		if loginNeeded(string(out)) {
			return nil, &UnavailableError{Msg: "Composio is logged out on this computer. Run composio login in a terminal, then try again."}
		}
		return nil, &ToolError{Msg: "Composio gave an answer Gwen couldn't read."}
	}
	f := &Found{Connected: raw.Connected}
	if f.Connected == nil {
		f.Connected = []string{}
	}
	seen := map[string]bool{}
	for _, r := range raw.Results {
		f.UseCases = append(f.UseCases, UseCase{UseCase: r.UseCase, Apps: r.Toolkits, Plan: r.Plan, Pitfalls: r.Pitfalls,
			Tools: r.Primary, Related: r.Related})
		for _, slug := range r.Primary {
			if seen[slug] || checkSlug(slug) != nil {
				continue
			}
			seen[slug] = true
			if schema, err := cachedSchema(raw.Schemas.Primary[slug], slug); err == nil {
				f.Tools = append(f.Tools, ToolInfo{Tool: slug, Input: schema})
			}
		}
	}
	return f, nil
}

// cachedSchema reads a tool's schema from the file the search wrote, only
// from Composio's own definitions folder and only the file of that tool.
func cachedSchema(path, slug string) (json.RawMessage, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".composio", "tool_definitions")
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		path = filepath.Join(home, rest)
	}
	if path == "" || filepath.Clean(path) != filepath.Join(dir, slug+".json") {
		path = filepath.Join(dir, slug+".json")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var def struct {
		InputSchema json.RawMessage `json:"inputSchema"`
	}
	if err := json.Unmarshal(b, &def); err != nil || len(def.InputSchema) == 0 {
		return nil, errors.New("no schema")
	}
	return trimSchema(def.InputSchema), nil
}

// ToolSchema is the input a tool takes.
func (c *Client) ToolSchema(ctx context.Context, slug string) (json.RawMessage, error) {
	if err := checkSlug(slug); err != nil {
		return nil, err
	}
	if s, err := cachedSchema("", slug); err == nil {
		return s, nil
	}
	app := toolkitOf(slug)
	out, tmp, err := c.command(ctx, app, "execute", slug, "--get-schema")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	i := bytes.IndexByte(out, '{')
	var def struct {
		InputSchema json.RawMessage `json:"inputSchema"`
	}
	if i < 0 || json.NewDecoder(bytes.NewReader(out[i:])).Decode(&def) != nil || len(def.InputSchema) == 0 {
		return nil, &ToolError{App: app, Msg: fmt.Sprintf("there is no tool %s; find tools with app_search.", slug)}
	}
	return trimSchema(def.InputSchema), nil
}

// trimSchema drops what only a person's form needs (titles, examples, form
// labels) and shortens descriptions until the schema fits maxSchema.
func trimSchema(raw json.RawMessage) json.RawMessage {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return raw
	}
	var b []byte
	for _, n := range []int{400, 160, 60, 0} {
		b, _ = json.Marshal(slim(v, n))
		if len(b) <= maxSchema {
			break
		}
	}
	return b
}

func slim(v any, desc int) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			switch k {
			case "title", "examples", "human_parameter_name", "human_parameter_description", "file_uploadable":
				continue
			case "description":
				if s, ok := e.(string); ok {
					if desc == 0 {
						continue
					}
					e = cut(oneLine(s), desc)
				}
			}
			out[k] = slim(e, desc)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = slim(e, desc)
		}
		return out
	}
	return v
}

// Run runs one tool with its arguments (a JSON object) and returns its data.
// An answer over maxRun comes back cut, as text, saying so.
func (c *Client) Run(ctx context.Context, slug string, args json.RawMessage) (json.RawMessage, error) {
	if err := checkSlug(slug); err != nil {
		return nil, err
	}
	var obj map[string]any
	if len(bytes.TrimSpace(args)) == 0 {
		obj = map[string]any{}
	} else if err := json.Unmarshal(args, &obj); err != nil || obj == nil {
		return nil, &ToolError{App: toolkitOf(slug), Msg: "arguments must be a JSON object."}
	}
	var data json.RawMessage
	if err := c.execute(ctx, toolkitOf(slug), slug, obj, &data); err != nil {
		return nil, err
	}
	if len(data) <= maxRun {
		return data, nil
	}
	cutAt := maxRun
	for cutAt > 0 && !utf8Start(data[cutAt]) {
		cutAt--
	}
	b, _ := json.Marshal(map[string]any{
		"cut":  fmt.Sprintf("The answer was %d KB, too long to read whole; this is its start. Ask for less: a smaller limit, fewer fields, or a filter.", len(data)/1000),
		"data": string(data[:cutAt]),
	})
	return b, nil
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }

// ToolKind tells from a tool's name what it does: "read" (gets, lists,
// searches), "delete" (deletes, removes, trashes), or "write" (anything
// else: sends, creates, changes, and names it doesn't know).
func ToolKind(slug string) string {
	words := strings.Split(slug, "_")
	if len(words) > 1 {
		words = words[1:] // the app's name
	}
	has := func(set map[string]bool) bool {
		for _, w := range words {
			if set[w] {
				return true
			}
		}
		return false
	}
	switch {
	case has(deleteWords):
		return "delete"
	case has(writeWords):
		return "write"
	case has(readWords):
		return "read"
	}
	return "write"
}

func wordSet(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(s) {
		m[w] = true
	}
	return m
}

var (
	readWords   = wordSet("GET LIST FETCH SEARCH FIND READ RETRIEVE QUERY VIEW DESCRIBE COUNT")
	deleteWords = wordSet("DELETE REMOVE TRASH DESTROY PURGE ERASE WIPE EMPTY CLEAR REVOKE UNINSTALL")
	// writeWords outweigh a read word in the same name (SEARCH_AND_REPLY).
	// Words that are also nouns in read tools' names (RUN, COMMENT, DRAFT,
	// SCHEDULE) are left out; a name without a read word asks anyway.
	writeWords = wordSet("SEND CREATE POST UPDATE REPLY ADD SET PUT PATCH INSERT UPSERT EDIT MODIFY WRITE UPLOAD SHARE " +
		"INVITE MOVE COPY MERGE FORWARD ARCHIVE UNARCHIVE UNSTAR MARK EXECUTE TRIGGER DISPATCH CANCEL REOPEN ASSIGN " +
		"UNASSIGN APPROVE REJECT REACT UNPIN JOIN LEAVE KICK BAN PUBLISH SUBSCRIBE UNSUBSCRIBE FOLLOW UNFOLLOW " +
		"TRANSFER RENAME UNLOCK ENABLE DISABLE")
)
