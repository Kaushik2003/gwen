package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kzark/gwen/internal/agent"
	"github.com/kzark/gwen/internal/composio"
	"github.com/kzark/gwen/internal/connect"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/wire"
)

// Gwen's agent reaches the user's apps through Composio (plan.md 3.16): apps
// lists what is connected, app_search finds the tools for a job, and app_run
// runs one. A tool that only reads runs without asking; one that sends,
// creates, or changes something asks every time, and one that deletes is
// destructive (agentAppRequest).

// AgentApps runs Composio's tools for Gwen's agent; *composio.Client is one.
type AgentApps interface {
	Accounts(ctx context.Context) ([]composio.Account, error)
	FindTools(ctx context.Context, query, apps string) (*composio.Found, error)
	ToolSchema(ctx context.Context, slug string) (json.RawMessage, error)
	Run(ctx context.Context, slug string, args json.RawMessage) (json.RawMessage, error)
}

var agentAppTools = []agentTool{
	{name: "apps", capability: agent.CapAppRead, risk: connect.Read, schema: schemaNone, run: (*Server).toolApps,
		description: "The apps the user connected to Composio (Gmail, Slack, GitHub, Calendar, Notion, ...), and whether each sign-in still works. Call it only when the user asks what is connected, or after an app failed with a sign-in error. Do not call it before app_search or app_run: app_run already says when an app is not connected."},
	{name: "app_search", capability: agent.CapAppRead, risk: connect.Read, run: (*Server).toolAppSearch,
		schema:      `{"type":"object","properties":{"query":{"type":"string","description":"The job in words, e.g. \"read the latest messages in a Slack channel\""},"app":{"type":"string","description":"App slugs to look in, comma-separated, e.g. \"slack\" or \"gmail,googlecalendar\""},"tool":{"type":"string","description":"A tool's name (e.g. SLACK_SEND_MESSAGE): just its input schema"}}}`,
		description: "Find the app tools for a job: Composio's plan, pitfalls, and each tool's name and input schema. Use it only when you do not already know which tool does the job. If you used the tool earlier in this chat, or the user named it, go straight to app_run. With tool, get one tool's input schema when you are unsure of its arguments."},
	{name: "app_run", capability: agent.CapExternal, risk: connect.External, run: (*Server).toolAppRun,
		schema:      `{"type":"object","properties":{"tool":{"type":"string","description":"The tool's name from app_search, e.g. SLACK_FETCH_CONVERSATION_HISTORY"},"arguments":{"type":"object","description":"Its input, as its schema says"}},"required":["tool"]}`,
		description: "Run one app tool as the user. Tools that only read (get, list, search, fetch) run at once; any that send, create, change, or delete ask the user every time. If you know the tool's name, run it without searching first; a failed call says what was wrong, so fix the arguments and retry, and look up the schema (app_search with tool) only if the message is not enough."},
}

// agentAppRequest is app_run's capability by its tool's name.
func agentAppRequest(input json.RawMessage) agent.Request {
	var in struct {
		Tool string `json:"tool"`
	}
	_ = json.Unmarshal(input, &in)
	app := appWords(in.Tool)
	switch composio.ToolKind(in.Tool) {
	case "read":
		return agent.Request{Capability: agent.CapAppRead, Risk: connect.Read}
	case "delete":
		return agent.Request{Capability: agent.CapDestructive, Risk: connect.Destructive,
			Why: "it deletes or removes something in " + app}
	}
	return agent.Request{Capability: agent.CapExternal, Risk: connect.External,
		Why: "it acts as you in " + app}
}

// appWords is the app of a tool's name, as a person says it.
func appWords(slug string) string {
	app, _, _ := strings.Cut(slug, "_")
	if names := map[string]string{"GMAIL": "Gmail", "SLACK": "Slack", "GITHUB": "GitHub", "GOOGLECALENDAR": "Google Calendar",
		"NOTION": "Notion", "GOOGLESHEETS": "Google Sheets", "GOOGLEDRIVE": "Google Drive", "GOOGLEDOCS": "Google Docs",
		"LINEAR": "Linear", "JIRA": "Jira"}; names[app] != "" {
		return names[app]
	}
	if app == "" {
		return "an app"
	}
	return strings.ToUpper(app[:1]) + strings.ToLower(app[1:])
}

// toolWords is a tool's name as words: SLACK_SEND_MESSAGE → "send message".
func toolWords(slug string) string {
	_, rest, ok := strings.Cut(slug, "_")
	if !ok {
		rest = slug
	}
	return strings.ToLower(strings.ReplaceAll(rest, "_", " "))
}

func (s *Server) noApps() error {
	return errors.New("the composio command isn't set up on this computer: the user must install it and log in")
}

// appError is a Composio failure in words the model can act on.
func appError(err error) error {
	var nc composio.ErrNotConnected
	if errors.As(err, &nc) {
		return fmt.Errorf("%s; ask the user to connect it (Settings, or `composio link %s` in a terminal)", err.Error(), nc.App)
	}
	return err
}

func (s *Server) toolApps(ctx context.Context, _ model.ChatThread, _ toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	if s.Apps == nil {
		return wire.AgentToolResult{}, nil, s.noApps()
	}
	list, err := s.Apps.Accounts(ctx)
	if err != nil {
		return wire.AgentToolResult{}, nil, appError(err)
	}
	if list == nil {
		list = []composio.Account{}
	}
	active := 0
	for _, a := range list {
		if a.Status == composio.StatusActive {
			active++
		}
	}
	res := toolData(map[string]any{"apps": list,
		"note": "Only connected (active) apps work. The user connects more, or reconnects an expired one, themselves."})
	res.Summary = fmt.Sprintf("Checked the connected apps (%d working)", active)
	return res, nil, nil
}

func (s *Server) toolAppSearch(ctx context.Context, _ model.ChatThread, in toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	if s.Apps == nil {
		return wire.AgentToolResult{}, nil, s.noApps()
	}
	if slug := strings.TrimSpace(in.Tool); slug != "" {
		schema, err := s.Apps.ToolSchema(ctx, slug)
		if err != nil {
			return wire.AgentToolResult{}, nil, appError(err)
		}
		res := toolData(composio.ToolInfo{Tool: slug, Input: schema})
		res.Summary = "Looked up " + appWords(slug) + "’s " + toolWords(slug)
		return res, nil, nil
	}
	found, err := s.Apps.FindTools(ctx, in.Query, in.App)
	if err != nil {
		return wire.AgentToolResult{}, nil, appError(err)
	}
	res := toolData(found)
	res.Summary = "Looked for app tools to " + agentCut(in.Query, 60)
	return res, nil, nil
}

func (s *Server) toolAppRun(ctx context.Context, _ model.ChatThread, in toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	if s.Apps == nil {
		return wire.AgentToolResult{}, nil, s.noApps()
	}
	slug := strings.TrimSpace(in.Tool)
	data, err := s.Apps.Run(ctx, slug, in.Arguments)
	if err != nil {
		return wire.AgentToolResult{}, nil, appError(err)
	}
	res := wire.AgentToolResult{Data: data}
	verb := "Read from"
	if composio.ToolKind(slug) != "read" {
		verb = "Did in"
	}
	res.Summary = fmt.Sprintf("%s %s: %s", verb, appWords(slug), toolWords(slug))
	return res, nil, nil
}
