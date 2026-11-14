package api

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kzark/gwen/internal/agent"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/connect"
	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/planner/civil"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/timeengine"
	"github.com/kzark/gwen/internal/wire"
)

// Gwen's tools for her agent (plan.md 3.16): read and change your tasks,
// plans, goals, and projects through the same code the planning assistant
// uses (applyAction), track time, remember, open things, and start or
// message Claude sessions. `gwen mcp` serves them to Claude Code.

// agentTool is one tool: its schema, capability, and how it runs.
type agentTool struct {
	name, description string
	capability        string
	risk              connect.Risk
	schema            string
	run               func(s *Server, ctx context.Context, thread model.ChatThread, in toolArgs) (wire.AgentToolResult, json.RawMessage, error)
}

// toolArgs is every field any tool takes.
type toolArgs struct {
	ID              string          `json:"id"`
	TaskID          string          `json:"task_id"`
	ProjectID       *string         `json:"project_id"`
	GoalID          *string         `json:"goal_id"`
	Title           *string         `json:"title"`
	Name            *string         `json:"name"`
	Notes           *string         `json:"notes"`
	Priority        *int            `json:"priority"`
	DueDay          *string         `json:"due_day"`
	StartDay        *string         `json:"start_day"`
	EstimateMinutes *int            `json:"estimate_minutes"`
	Effort          *int            `json:"effort"`
	Stage           *string         `json:"stage"`
	DelegatedTo     *string         `json:"delegated_to"`
	Steps           []string        `json:"steps"`
	Day             *string         `json:"day"`
	Start           *string         `json:"start"`
	Minutes         *int            `json:"minutes"`
	Status          string          `json:"status"`
	Text            string          `json:"text"`
	Query           string          `json:"query"`
	From            string          `json:"from"`
	To              string          `json:"to"`
	Specific        *string         `json:"specific"`
	Measurable      *string         `json:"measurable"`
	Assignable      *string         `json:"assignable"`
	Realistic       *string         `json:"realistic"`
	Attitude        *string         `json:"attitude"`
	Note            *string         `json:"note"`
	URL             string          `json:"url"`
	Folder          string          `json:"folder"`
	Tab             string          `json:"tab"`
	Brief           string          `json:"brief"`
	Sandbox         *bool           `json:"sandbox"`
	SessionID       string          `json:"session_id"`
	Turns           int             `json:"turns"`
	Path            string          `json:"path"`
	Everywhere      bool            `json:"everywhere"`
	Tool            string          `json:"tool"`
	App             string          `json:"app"`
	Arguments       json.RawMessage `json:"arguments"`
}

// undoOp is how a change is taken back.
type undoOp struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Day  string `json:"day,omitempty"`
	// Task is the task as it was before an update.
	Task *model.Task `json:"task,omitempty"`
}

func undoJSON(op undoOp) json.RawMessage {
	b, _ := json.Marshal(op)
	return b
}

const (
	schemaNone    = `{"type":"object","properties":{}}`
	schemaID      = `{"type":"object","properties":{"id":{"type":"string","description":"The task's id, or a unique prefix of 8 or more characters"}},"required":["id"]}`
	schemaTaskSet = `"title":{"type":"string"},"notes":{"type":"string"},"project_id":{"type":"string","description":"A project id (prefix of 8+ is fine); \"\" for none"},"goal_id":{"type":"string"},"priority":{"type":"integer","minimum":1,"maximum":4,"description":"1 is the most important"},"due_day":{"type":"string","description":"YYYY-MM-DD, \"\" to clear"},"start_day":{"type":"string","description":"YYYY-MM-DD, \"\" to clear"},"estimate_minutes":{"type":"integer","description":"0 clears it"},"effort":{"type":"integer","minimum":0,"maximum":3,"description":"1 easy, 2 medium, 3 hard"},"stage":{"type":"string","enum":["inbox","todo","doing","waiting","someday"]},"delegated_to":{"type":"string"}`
)

// agentTools is the list `gwen mcp` serves, in order.
var agentTools = append([]agentTool{
	{name: "context", capability: agent.CapGwenRead, risk: connect.Read, schema: schemaNone, run: (*Server).toolContext,
		description: "What the user has open and is doing right now: tracking, the open tab, the project's tasks, sessions and git state."},
	{name: "tasks", capability: agent.CapGwenRead, risk: connect.Read, run: (*Server).toolTasks,
		schema:      `{"type":"object","properties":{"project_id":{"type":"string","description":"A project (with its sub-projects); leave out for this chat's project, or every project in a chat for everywhere"},"everywhere":{"type":"boolean","description":"Every project, whatever the chat"},"status":{"type":"string","enum":["open","done","all"]},"stage":{"type":"string"},"text":{"type":"string","description":"Words the title or notes contain"}}}`,
		description: "List tasks, with ids, stage, project, due day, estimate, priority, and steps."},
	{name: "task", capability: agent.CapGwenRead, risk: connect.Read, schema: schemaID, run: (*Server).toolTask,
		description: "Read one task with its notes and steps."},
	{name: "plan", capability: agent.CapGwenRead, risk: connect.Read, run: (*Server).toolPlan,
		schema:      `{"type":"object","properties":{"day":{"type":"string","description":"YYYY-MM-DD; today when left out"}}}`,
		description: "A day's plan: the planned blocks with their times, and the calendar's busy times."},
	{name: "goals", capability: agent.CapGwenRead, risk: connect.Read, schema: schemaNone, run: (*Server).toolGoals,
		description: "The active goals with their progress and pace."},
	{name: "projects", capability: agent.CapGwenRead, risk: connect.Read, schema: schemaNone, run: (*Server).toolProjects,
		description: "Every project with its folder on this computer and its sub-projects."},
	{name: "time", capability: agent.CapGwenRead, risk: connect.Read, run: (*Server).toolTime,
		schema:      `{"type":"object","properties":{"from":{"type":"string","description":"YYYY-MM-DD"},"to":{"type":"string","description":"YYYY-MM-DD"}}}`,
		description: "Time tracked per project over a range of days (the last 7 when left out)."},
	{name: "sessions", capability: agent.CapGwenRead, risk: connect.Read, run: (*Server).toolSessions,
		schema:      `{"type":"object","properties":{"project_id":{"type":"string"},"everywhere":{"type":"boolean"}}}`,
		description: "Claude sessions of this chat's project (or of every project): id, state, what it is doing, its task."},
	{name: "session_transcript", capability: agent.CapGwenRead, risk: connect.Read, run: (*Server).toolTranscript,
		schema:      `{"type":"object","properties":{"session_id":{"type":"string"},"turns":{"type":"integer","description":"How many of the last turns (default 6)"}},"required":["session_id"]}`,
		description: "The last turns of a Claude session's conversation."},
	{name: "recall", capability: agent.CapGwenRead, risk: connect.Read, run: (*Server).toolRecall,
		schema:      `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`,
		description: "Search Gwen's memory of the user for what bears on a question."},
	{name: "task_create", capability: agent.CapGwenChange, risk: connect.Local, run: (*Server).toolTaskCreate,
		schema:      `{"type":"object","properties":{` + schemaTaskSet + `,"steps":{"type":"array","items":{"type":"string"},"description":"Steps (subtasks), in order"}},"required":["title"]}`,
		description: "Add a task. Gwen fits it into the day by herself when it has an estimate. In a project chat the task goes to that project unless project_id says otherwise."},
	{name: "task_update", capability: agent.CapGwenChange, risk: connect.Local, run: (*Server).toolTaskUpdate,
		schema:      `{"type":"object","properties":{"id":{"type":"string"},` + schemaTaskSet + `,"steps":{"type":"array","items":{"type":"string"},"description":"Steps to add"}},"required":["id"]}`,
		description: "Change a task: any field, its stage or project, or add steps. Only the fields given change."},
	{name: "task_complete", capability: agent.CapGwenChange, risk: connect.Local, schema: schemaID, run: (*Server).toolTaskComplete,
		description: "Tick a task off."},
	{name: "task_schedule", capability: agent.CapGwenChange, risk: connect.Local, run: (*Server).toolTaskSchedule,
		schema:      `{"type":"object","properties":{"id":{"type":"string"},"day":{"type":"string","description":"YYYY-MM-DD; today when left out"},"start":{"type":"string","description":"HH:MM, 24-hour"},"minutes":{"type":"integer"}},"required":["id"]}`,
		description: "Pin a task on a day's plan, at a time if given."},
	{name: "task_unschedule", capability: agent.CapGwenChange, risk: connect.Local, run: (*Server).toolTaskUnschedule,
		schema:      `{"type":"object","properties":{"id":{"type":"string"},"day":{"type":"string","description":"YYYY-MM-DD; every day when left out"}},"required":["id"]}`,
		description: "Take a task off the plan."},
	{name: "project_create", capability: agent.CapGwenChange, risk: connect.Local, run: (*Server).toolProjectCreate,
		schema:      `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`,
		description: "Add a project."},
	{name: "goal_create", capability: agent.CapGwenChange, risk: connect.Local, run: (*Server).toolGoalCreate,
		schema:      `{"type":"object","properties":{"title":{"type":"string"},"project_id":{"type":"string"},"start_day":{"type":"string"},"due_day":{"type":"string"},"specific":{"type":"string"},"measurable":{"type":"string"},"assignable":{"type":"string"},"realistic":{"type":"string"}},"required":["title","due_day"]}`,
		description: "Add a goal (a set of tasks to finish by a day), with its SMART parts."},
	{name: "goal_update", capability: agent.CapGwenChange, risk: connect.Local, run: (*Server).toolGoalUpdate,
		schema:      `{"type":"object","properties":{"id":{"type":"string"},"title":{"type":"string"},"due_day":{"type":"string"},"specific":{"type":"string"},"measurable":{"type":"string"},"assignable":{"type":"string"},"realistic":{"type":"string"}},"required":["id"]}`,
		description: "Change a goal."},
	{name: "track", capability: agent.CapGwenChange, risk: connect.Local, run: (*Server).toolTrack,
		schema:      `{"type":"object","properties":{"task_id":{"type":"string"},"project_id":{"type":"string"}}}`,
		description: "Start tracking time on a task or project (clocks in when the user is off)."},
	{name: "remember", capability: agent.CapGwenChange, risk: connect.Local, run: (*Server).toolRemember,
		schema:      `{"type":"object","properties":{"text":{"type":"string","description":"What to remember, in the user's words"}},"required":["text"]}`,
		description: "Remember something the user asked you to remember."},
	{name: "set_attitude", capability: agent.CapGwenChange, risk: connect.Local, run: (*Server).toolAttitude,
		schema:      `{"type":"object","properties":{"attitude":{"type":"string"},"note":{"type":"string","description":"Your note to yourself on why and what works with them"}}}`,
		description: "Change how you treat the user from now on (your attitude) and your note to yourself."},
	{name: "open", capability: agent.CapGwenChange, risk: connect.Local, run: (*Server).toolOpen,
		schema:      `{"type":"object","properties":{"url":{"type":"string"},"folder":{"type":"string"},"tab":{"type":"string","description":"A tab in Gwen's window: project:<id>, session:<id>, terminal:<address>, or a screen (today, plan, board, goals, inbox)"}}}`,
		description: "Open a link in the browser, a folder, or a tab in Gwen's window."},
	{name: "task_delete", capability: agent.CapDestructive, risk: connect.Destructive, schema: schemaID, run: (*Server).toolTaskDelete,
		description: "Delete a task with its steps. Always asks the user."},
	{name: "goal_delete", capability: agent.CapDestructive, risk: connect.Destructive, schema: schemaID, run: (*Server).toolGoalDelete,
		description: "Delete a goal. Always asks the user."},
	{name: "session_start", capability: agent.CapSession, risk: connect.Write, run: (*Server).toolSessionStart,
		schema:      `{"type":"object","properties":{"title":{"type":"string","description":"The job in a few words, e.g. Fix the mobile layout"},"brief":{"type":"string","description":"What to do, why, where (files, folders, commands), and how it will know it is done"},"project_id":{"type":"string","description":"This chat's project when left out"},"task_id":{"type":"string","description":"The task it works on; it goes to Waiting on Claude"},"sandbox":{"type":"boolean"}},"required":["title","brief"]}`,
		description: "Hand a job to a new Claude session in a project's folder: every change, command, test, or build goes through one. It runs in its own terminal the user can watch; you hear when it finishes or needs the user."},
	{name: "changes", capability: agent.CapGwenRead, risk: connect.Read, run: (*Server).toolChanges,
		schema:      `{"type":"object","properties":{"session_id":{"type":"string","description":"A Claude session: what it changed since it started; leave out for the project's uncommitted changes"},"path":{"type":"string","description":"One file: its diff"}}}`,
		description: "What changed: a session's files (with +/- counts) since it started, or the project's uncommitted files; with path, that file's diff."},
	{name: "session_send", capability: agent.CapSession, risk: connect.Write, run: (*Server).toolSessionSend,
		schema:      `{"type":"object","properties":{"session_id":{"type":"string"},"text":{"type":"string"}},"required":["session_id","text"]}`,
		description: "Send a message to a Claude session that is idle or done."},
}, agentAppTools...)

func findTool(name string) (agentTool, bool) {
	i := slices.IndexFunc(agentTools, func(t agentTool) bool { return t.name == name })
	if i < 0 {
		return agentTool{}, false
	}
	return agentTools[i], true
}

// Tools implements agent.Sources.
func (s *Server) Tools() []wire.AgentTool {
	out := make([]wire.AgentTool, len(agentTools))
	for i, t := range agentTools {
		out[i] = wire.AgentTool{Name: t.name, Description: t.description, InputSchema: json.RawMessage(t.schema)}
	}
	return out
}

// ToolRequest implements agent.Sources: a tool's capability and risk, which
// for app_run depend on the app tool it runs.
func (s *Server) ToolRequest(name string, input json.RawMessage) (agent.Request, bool) {
	t, ok := findTool(name)
	if !ok {
		return agent.Request{}, false
	}
	switch name {
	case "app_run":
		return agentAppRequest(input), true
	case "open":
		// A link can carry out what she read (mail, messages) to anyone's
		// server, so each one asks; a folder or a tab of Gwen's stays local.
		var in toolArgs
		_ = json.Unmarshal(input, &in)
		if in.URL != "" {
			return agent.Request{Capability: agent.CapExternal, Risk: connect.External,
				Why: "the page it opens learns whatever the link holds"}, true
		}
	case "task_delete":
		return agent.Request{Capability: t.capability, Risk: t.risk, Why: "the task and its steps are gone for good"}, true
	case "goal_delete":
		return agent.Request{Capability: t.capability, Risk: t.risk, Why: "the goal is gone for good"}, true
	}
	return agent.Request{Capability: t.capability, Risk: t.risk}, true
}

// ToolTitle implements agent.Sources: a call in a few words.
func (s *Server) ToolTitle(name string, input json.RawMessage) string {
	var in toolArgs
	_ = json.Unmarshal(input, &in)
	title := func(p *string) string {
		if p == nil {
			return ""
		}
		return " “" + strings.TrimSpace(*p) + "”"
	}
	switch name {
	case "task_create":
		return "Add task" + title(in.Title)
	case "task_update":
		return "Change task" + title(in.Title)
	case "task_complete":
		return "Complete a task"
	case "task_schedule":
		return "Put a task on the plan"
	case "task_unschedule":
		return "Take a task off the plan"
	case "project_create":
		return "Add project" + title(in.Name)
	case "goal_create":
		return "Add goal" + title(in.Title)
	case "goal_update":
		return "Change a goal"
	case "track":
		return "Start tracking"
	case "remember":
		return "Remember: " + agentCut(in.Text, 60)
	case "set_attitude":
		return "Change her attitude"
	case "open":
		return "Open " + cmp.Or(in.URL, in.Folder, in.Tab)
	case "task_delete":
		return "Delete a task"
	case "goal_delete":
		return "Delete a goal"
	case "session_start":
		return "Hand to Claude: " + agentCut(cmp.Or(deref(in.Title), in.Brief), 80)
	case "session_send":
		return "Tell Claude: " + agentCut(in.Text, 80)
	case "changes":
		return "Check what changed"
	case "tasks":
		return "List tasks"
	case "session_transcript":
		return "Read a session's transcript"
	case "apps":
		return "Check the connected apps"
	case "app_search":
		if in.Tool != "" {
			return "Look up " + appWords(in.Tool) + "’s " + toolWords(in.Tool)
		}
		return "Find app tools to " + agentCut(in.Query, 60)
	case "app_run":
		return appWords(in.Tool) + ": " + toolWords(in.Tool)
	}
	return strings.ReplaceAll(cmp.Or(name, "tool"), "_", " ")
}

func agentCut(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// CallTool implements agent.Sources.
func (s *Server) CallTool(ctx context.Context, thread model.ChatThread, name string, input json.RawMessage) (wire.AgentToolResult, json.RawMessage, error) {
	t, ok := findTool(name)
	if !ok {
		return wire.AgentToolResult{}, nil, fmt.Errorf("there is no tool %q", name)
	}
	var in toolArgs
	if err := json.Unmarshal(input, &in); err != nil {
		return wire.AgentToolResult{OK: false, Error: "the input isn't valid: " + err.Error()}, nil, nil
	}
	res, undo, err := t.run(s, ctx, thread, in)
	if err != nil {
		return wire.AgentToolResult{OK: false, Error: actionError(err)}, nil, nil
	}
	res.OK = true
	return res, undo, nil
}

// Undo implements agent.Sources.
func (s *Server) Undo(ctx context.Context, raw json.RawMessage) error {
	var op undoOp
	if err := json.Unmarshal(raw, &op); err != nil {
		return err
	}
	switch op.Kind {
	case "task_delete":
		ch, err := s.Repos.Tasks.Delete(ctx, op.ID)
		if err != nil {
			return err
		}
		s.publishChanges(ch)
		s.Hub.Publish(wire.EventTasksChanged, wire.TasksChanged{TaskIDs: []string{op.ID}})
	case "task_reopen":
		ch, err := s.Repos.Tasks.Reopen(ctx, op.ID, s.today())
		if err != nil {
			return err
		}
		s.publishChanges(ch.Changes)
		s.Hub.Publish(wire.EventTasksChanged, wire.TasksChanged{TaskIDs: []string{op.ID}})
	case "task_restore":
		t := op.Task
		if t == nil {
			return errors.New("nothing to restore")
		}
		est := store.Nullable[int]{Set: true}
		if t.Estimate != nil {
			m := int(*t.Estimate / time.Minute)
			est.Value = &m
		}
		eff := store.Nullable[int]{Set: true, Value: t.Effort}
		_, err := s.Repos.Tasks.Update(ctx, op.ID, store.TaskPatch{Title: &t.Title, Notes: &t.Notes, Priority: &t.Priority,
			ProjectID: store.Nullable[string]{Set: true, Value: t.ProjectID}, GoalID: store.Nullable[string]{Set: true, Value: t.GoalID},
			DueDay: store.Nullable[string]{Set: true, Value: t.DueDay}, StartDay: store.Nullable[string]{Set: true, Value: t.StartDay},
			EstimateMinutes: est, Effort: eff, Stage: &t.Stage, DelegatedTo: &t.DelegatedTo})
		if err != nil {
			return err
		}
		s.Hub.Publish(wire.EventTasksChanged, wire.TasksChanged{TaskIDs: []string{op.ID}})
		s.replan(ctx)
	case "unschedule":
		ch, err := s.Repos.Plans.Unschedule(ctx, op.ID, op.Day, s.planEnv())
		if err != nil {
			return err
		}
		s.publishChanges(ch)
	case "project_delete":
		ch, err := s.Repos.Projects.Delete(ctx, op.ID)
		if err != nil {
			return err
		}
		s.publishChanges(ch)
		s.Hub.Publish(wire.EventProjectsChanged, wire.ProjectsChanged{})
	case "goal_delete":
		ch, err := s.Repos.Goals.Delete(ctx, op.ID, false)
		if err != nil {
			return err
		}
		s.publishChanges(ch)
	default:
		return fmt.Errorf("can't undo %q", op.Kind)
	}
	return nil
}

// resolve finds a row by its id or a unique prefix of 8 or more characters.
func resolve[T any](items []T, id func(T) string, prefix, kind string) (T, error) {
	var zero T
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return zero, fmt.Errorf("give the %s's id", kind)
	}
	var found []T
	for _, it := range items {
		if id(it) == prefix {
			return it, nil
		}
		if len(prefix) >= 8 && strings.HasPrefix(id(it), prefix) {
			found = append(found, it)
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return zero, fmt.Errorf("there is no %s %s", kind, prefix)
	}
	return zero, fmt.Errorf("%s matches more than one %s; give more of its id", prefix, kind)
}

func (s *Server) findTask(ctx context.Context, id string) (model.Task, error) {
	tasks, err := s.Repos.Tasks.List(ctx, store.TaskFilter{Status: "all", Templates: true})
	if err != nil {
		return model.Task{}, err
	}
	return resolve(tasks, func(t model.Task) string { return t.ID }, id, "task")
}

func (s *Server) findProject(ctx context.Context, id string) (model.Project, error) {
	projects, err := s.Repos.Projects.List(ctx, wire.ArchivedAll)
	if err != nil {
		return model.Project{}, err
	}
	return resolve(projects, func(p model.Project) string { return p.ID }, id, "project")
}

func (s *Server) findGoal(ctx context.Context, id string) (model.Goal, error) {
	goals, err := s.Repos.Goals.List(ctx, "")
	if err != nil {
		return model.Goal{}, err
	}
	return resolve(goals, func(g model.Goal) string { return g.ID }, id, "goal")
}

// act runs one planning-assistant action with ids resolved to refs.
func (s *Server) act(ctx context.Context, a llm.Action, refs assistantRefs) (wire.ActionResult, error) {
	res := s.applyActions(ctx, []llm.Action{a}, refs)
	if len(res) == 0 {
		return wire.ActionResult{}, errors.New("nothing happened")
	}
	if !res[0].OK {
		return res[0], errors.New(res[0].Error)
	}
	return res[0], nil
}

// linkRefs puts a project and a goal in refs, under their ids, for an action.
func (s *Server) linkRefs(ctx context.Context, refs assistantRefs, a *llm.Action, project, goal *string) error {
	if project != nil {
		if *project == "" {
			a.Project = project
		} else {
			p, err := s.findProject(ctx, *project)
			if err != nil {
				return err
			}
			refs[p.ID] = ref{refProject, p.ID, p.Name}
			a.Project = &p.ID
		}
	}
	if goal != nil {
		if *goal == "" {
			a.Goal = goal
		} else {
			g, err := s.findGoal(ctx, *goal)
			if err != nil {
				return err
			}
			refs[g.ID] = ref{refGoal, g.ID, g.Title}
			a.Goal = &g.ID
		}
	}
	return nil
}

func (s *Server) addSteps(ctx context.Context, parent model.Task, steps []string) error {
	for _, title := range steps {
		if title = strings.TrimSpace(title); title == "" {
			continue
		}
		if _, _, err := s.Repos.Tasks.Create(ctx, store.NewTask{Title: title, ParentID: &parent.ID, ProjectID: parent.ProjectID}); err != nil {
			return err
		}
	}
	if len(steps) > 0 {
		s.Hub.Publish(wire.EventTasksChanged, wire.TasksChanged{TaskIDs: []string{parent.ID}})
	}
	return nil
}

func (s *Server) toolTaskCreate(ctx context.Context, thread model.ChatThread, in toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	refs := assistantRefs{}
	key := "new"
	a := llm.Action{Type: llm.ActCreateTask, Title: in.Title, Notes: in.Notes, Priority: in.Priority, DueDay: in.DueDay,
		StartDay: in.StartDay, EstimateMinutes: in.EstimateMinutes, Effort: in.Effort, Stage: in.Stage,
		DelegatedTo: in.DelegatedTo, Key: &key}
	project := in.ProjectID
	if project == nil && thread.ProjectID != nil {
		project = thread.ProjectID
	}
	if err := s.linkRefs(ctx, refs, &a, project, in.GoalID); err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	res, err := s.act(ctx, a, refs)
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	if res.TaskID != nil {
		if t, err := s.Repos.Tasks.Get(ctx, *res.TaskID); err == nil {
			if err := s.addSteps(ctx, t, in.Steps); err != nil {
				return wire.AgentToolResult{}, nil, err
			}
		}
	}
	data, _ := json.Marshal(map[string]any{"task_id": res.TaskID})
	return wire.AgentToolResult{Summary: res.Summary, Data: data}, undoJSON(undoOp{Kind: "task_delete", ID: deref(res.TaskID)}), nil
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (s *Server) toolTaskUpdate(ctx context.Context, _ model.ChatThread, in toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	t, err := s.findTask(ctx, in.ID)
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	before := t
	refs := assistantRefs{t.ID: ref{refTask, t.ID, t.Title}}
	a := llm.Action{Type: llm.ActUpdateTask, Ref: &t.ID, Title: in.Title, Notes: in.Notes, Priority: in.Priority,
		DueDay: in.DueDay, StartDay: in.StartDay, EstimateMinutes: in.EstimateMinutes, Effort: in.Effort, Stage: in.Stage,
		DelegatedTo: in.DelegatedTo}
	if err := s.linkRefs(ctx, refs, &a, in.ProjectID, in.GoalID); err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	changed := in.Title != nil || in.Notes != nil || in.Priority != nil || in.DueDay != nil || in.StartDay != nil ||
		in.EstimateMinutes != nil || in.Effort != nil || in.Stage != nil || in.DelegatedTo != nil || in.ProjectID != nil || in.GoalID != nil
	summary := fmt.Sprintf("Updated %q", t.Title)
	if changed {
		res, err := s.act(ctx, a, refs)
		if err != nil {
			return wire.AgentToolResult{}, nil, err
		}
		summary = res.Summary
	}
	if err := s.addSteps(ctx, t, in.Steps); err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	if len(in.Steps) > 0 {
		summary += fmt.Sprintf(", %d steps added", len(in.Steps))
	}
	var undo json.RawMessage
	if changed {
		undo = undoJSON(undoOp{Kind: "task_restore", ID: t.ID, Task: &before})
	}
	return wire.AgentToolResult{Summary: summary}, undo, nil
}

func (s *Server) toolTaskComplete(ctx context.Context, _ model.ChatThread, in toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	t, err := s.findTask(ctx, in.ID)
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	res, err := s.act(ctx, llm.Action{Type: llm.ActCompleteTask, Ref: &t.ID}, assistantRefs{t.ID: ref{refTask, t.ID, t.Title}})
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	return wire.AgentToolResult{Summary: res.Summary}, undoJSON(undoOp{Kind: "task_reopen", ID: t.ID}), nil
}

func (s *Server) toolTaskSchedule(ctx context.Context, _ model.ChatThread, in toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	t, err := s.findTask(ctx, in.ID)
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	res, err := s.act(ctx, llm.Action{Type: llm.ActScheduleTask, Ref: &t.ID, Day: in.Day, Start: in.Start, Minutes: in.Minutes},
		assistantRefs{t.ID: ref{refTask, t.ID, t.Title}})
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	day := s.today()
	if in.Day != nil && *in.Day != "" {
		day = *in.Day
	}
	return wire.AgentToolResult{Summary: res.Summary}, undoJSON(undoOp{Kind: "unschedule", ID: t.ID, Day: day}), nil
}

func (s *Server) toolTaskUnschedule(ctx context.Context, _ model.ChatThread, in toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	t, err := s.findTask(ctx, in.ID)
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	res, err := s.act(ctx, llm.Action{Type: llm.ActUnschedule, Ref: &t.ID, Day: in.Day}, assistantRefs{t.ID: ref{refTask, t.ID, t.Title}})
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	return wire.AgentToolResult{Summary: res.Summary}, nil, nil
}

func (s *Server) toolProjectCreate(ctx context.Context, _ model.ChatThread, in toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	res, err := s.act(ctx, llm.Action{Type: llm.ActCreateProject, Name: in.Name}, assistantRefs{})
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	data, _ := json.Marshal(map[string]any{"project_id": res.ProjectID})
	return wire.AgentToolResult{Summary: res.Summary, Data: data}, undoJSON(undoOp{Kind: "project_delete", ID: deref(res.ProjectID)}), nil
}

func (s *Server) toolGoalCreate(ctx context.Context, thread model.ChatThread, in toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	refs := assistantRefs{}
	a := llm.Action{Type: llm.ActCreateGoal, Title: in.Title, StartDay: in.StartDay, DueDay: in.DueDay, Specific: in.Specific,
		Measurable: in.Measurable, Assignable: in.Assignable, Realistic: in.Realistic}
	project := in.ProjectID
	if project == nil && thread.ProjectID != nil {
		project = thread.ProjectID
	}
	if err := s.linkRefs(ctx, refs, &a, project, nil); err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	res, err := s.act(ctx, a, refs)
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	return wire.AgentToolResult{Summary: res.Summary}, undoJSON(undoOp{Kind: "goal_delete", ID: deref(res.GoalID)}), nil
}

func (s *Server) toolGoalUpdate(ctx context.Context, _ model.ChatThread, in toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	g, err := s.findGoal(ctx, in.ID)
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	res, err := s.act(ctx, llm.Action{Type: llm.ActUpdateGoal, Ref: &g.ID, Title: in.Title, DueDay: in.DueDay, Specific: in.Specific,
		Measurable: in.Measurable, Assignable: in.Assignable, Realistic: in.Realistic}, assistantRefs{g.ID: ref{refGoal, g.ID, g.Title}})
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	return wire.AgentToolResult{Summary: res.Summary}, nil, nil
}

func (s *Server) toolTaskDelete(ctx context.Context, _ model.ChatThread, in toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	t, err := s.findTask(ctx, in.ID)
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	ch, err := s.Repos.Tasks.Delete(ctx, t.ID)
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	s.publishChanges(ch)
	s.Hub.Publish(wire.EventTasksChanged, wire.TasksChanged{TaskIDs: []string{t.ID}})
	return wire.AgentToolResult{Summary: fmt.Sprintf("Deleted %q", t.Title)}, nil, nil
}

func (s *Server) toolGoalDelete(ctx context.Context, _ model.ChatThread, in toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	g, err := s.findGoal(ctx, in.ID)
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	ch, err := s.Repos.Goals.Delete(ctx, g.ID, false)
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	s.publishChanges(ch)
	return wire.AgentToolResult{Summary: fmt.Sprintf("Deleted the goal %q", g.Title)}, nil, nil
}

func (s *Server) toolTrack(ctx context.Context, _ model.ChatThread, in toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	var project, task *string
	label := "no project"
	if in.TaskID != "" {
		t, err := s.findTask(ctx, in.TaskID)
		if err != nil {
			return wire.AgentToolResult{}, nil, err
		}
		task, label = &t.ID, t.Title
	}
	if in.ProjectID != nil && *in.ProjectID != "" {
		p, err := s.findProject(ctx, *in.ProjectID)
		if err != nil {
			return wire.AgentToolResult{}, nil, err
		}
		project = &p.ID
		if task == nil {
			label = p.Name
		}
	}
	project, task, err := s.attribution(ctx, project, task)
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	st, err := s.Tracker.Status(ctx)
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	if st.State == "off" {
		_, err = s.Tracker.Do(ctx, func(at time.Time) timeengine.Input {
			return timeengine.ClockIn{ProjectID: project, TaskID: task, At: at}
		})
	} else {
		_, err = s.Tracker.Do(ctx, func(at time.Time) timeengine.Input {
			return timeengine.Switch{ProjectID: project, TaskID: task, At: at}
		})
	}
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	return wire.AgentToolResult{Summary: "Tracking " + label}, nil, nil
}

func (s *Server) toolRemember(ctx context.Context, _ model.ChatThread, in toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	if s.Memory == nil {
		return wire.AgentToolResult{}, nil, errors.New("memory is off on this device")
	}
	claims, err := s.Memory.Remember(ctx, memory.RememberRequest{Text: in.Text, Channel: model.ChannelUI})
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	return wire.AgentToolResult{Summary: fmt.Sprintf("Remembered %d %s", len(claims), plural(len(claims), "thing", "things"))}, nil, nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func (s *Server) toolAttitude(ctx context.Context, _ model.ChatThread, in toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	self, switched, changed, err := s.nextSelf(ctx, in.Attitude, in.Note)
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	if !changed {
		return wire.AgentToolResult{Summary: "Nothing to change"}, nil, nil
	}
	if _, err := s.Repos.Self.Add(ctx, self); err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	if switched != "" {
		return wire.AgentToolResult{Summary: "Now " + switched}, nil, nil
	}
	return wire.AgentToolResult{Summary: "Kept a note to herself"}, nil, nil
}

func (s *Server) toolOpen(ctx context.Context, _ model.ChatThread, in toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	switch {
	case in.Tab != "":
		if s.Code == nil {
			return wire.AgentToolResult{}, nil, errors.New("the window can't be opened from here")
		}
		if err := s.Code.OpenWindow(ctx, in.Tab); err != nil {
			return wire.AgentToolResult{}, nil, err
		}
		return wire.AgentToolResult{Summary: "Opened " + in.Tab}, nil, nil
	case in.URL != "" || in.Folder != "":
		if s.Harness == nil {
			return wire.AgentToolResult{}, nil, errors.New("links can't be opened on this device")
		}
		name, payload := "browser.open", map[string]any{"urls": []string{in.URL}}
		if in.URL == "" {
			name, payload = "folder.open", map[string]any{"path": in.Folder}
		}
		c, _, ok := s.Harness.Registry().Capability(name)
		if !ok {
			return wire.AgentToolResult{}, nil, fmt.Errorf("can't %s here", strings.ReplaceAll(name, ".", " "))
		}
		raw, _ := json.Marshal(payload)
		if checked, err := c.Check(raw); err == nil {
			raw = checked
		} else {
			return wire.AgentToolResult{}, nil, err
		}
		if _, err := c.Invoke(ctx, raw); err != nil {
			return wire.AgentToolResult{}, nil, err
		}
		return wire.AgentToolResult{Summary: "Opened " + cmp.Or(in.URL, in.Folder)}, nil, nil
	}
	return wire.AgentToolResult{}, nil, errors.New("say what to open: url, folder, or tab")
}

func (s *Server) toolSessionStart(ctx context.Context, thread model.ChatThread, in toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	if s.Code == nil {
		return wire.AgentToolResult{}, nil, errors.New("sessions can't start on this device")
	}
	projectID := deref(thread.ProjectID)
	if in.ProjectID != nil && *in.ProjectID != "" {
		projectID = *in.ProjectID
	}
	if projectID == "" {
		return wire.AgentToolResult{}, nil, errors.New("say which project the session works in")
	}
	p, err := s.findProject(ctx, projectID)
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	if p.Folder == "" {
		return wire.AgentToolResult{}, nil, fmt.Errorf("%s has no folder on this computer yet; ask the user to choose one", p.Name)
	}
	if strings.TrimSpace(in.Brief) == "" {
		return wire.AgentToolResult{}, nil, errors.New("give the session a brief")
	}
	var task *model.Task
	if in.TaskID != "" {
		t, err := s.findTask(ctx, in.TaskID)
		if err != nil {
			return wire.AgentToolResult{}, nil, err
		}
		task = &t
	}
	window := false
	// A Trusted project's jobs run in Claude's auto mode: its own classifier
	// approves routine work and asks only about risky steps (plan.md 3.16).
	mode := ""
	if cmp.Or(p.AgentAutonomy, s.Tracker.Config().Agent.Autonomy) == config.AutonomyTrusted {
		mode = "auto"
	}
	sess, err := s.Code.StartInProject(ctx, wire.StartCodeSession{Dir: p.Folder, Prompt: in.Brief, Window: &window, Sandbox: in.Sandbox,
		PermissionMode: mode, Name: deref(in.Title)})
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	summary := "Started a Claude session in " + p.Name
	if task != nil {
		if sess.Terminal != "" {
			if err := s.Code.LinkTask(ctx, sess.Terminal, task.ID); err != nil {
				summary += " (the task link didn't take)"
			}
		}
		stage, who := model.StageWaiting, "Claude"
		if _, err := s.Repos.Tasks.Update(ctx, task.ID, store.TaskPatch{Stage: &stage, DelegatedTo: &who}); err == nil {
			s.Hub.Publish(wire.EventTasksChanged, wire.TasksChanged{TaskIDs: []string{task.ID}})
		}
		summary += " on " + fmt.Sprintf("%q", task.Title)
	}
	data, _ := json.Marshal(map[string]any{"session_id": sess.ID, "terminal": sess.Terminal, "title": deref(in.Title)})
	return wire.AgentToolResult{Summary: summary, Data: data}, nil, nil
}

func (s *Server) toolSessionSend(ctx context.Context, _ model.ChatThread, in toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	if s.Code == nil {
		return wire.AgentToolResult{}, nil, errors.New("sessions aren't reachable on this device")
	}
	sess, err := s.Code.Session(in.SessionID)
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	switch sess.State {
	case "working", "starting":
		return wire.AgentToolResult{}, nil, errors.New("that session is working; wait until it is done")
	case "needs_you":
		return wire.AgentToolResult{}, nil, errors.New("that session waits on the user; only they can answer it")
	}
	res, err := s.Code.Prompt(ctx, sess.ID, wire.CodePrompt{Text: in.Text})
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	return wire.AgentToolResult{Summary: "Sent to the session (" + res.Result + ")"}, nil, nil
}

// --- reads

func toolData(v any) wire.AgentToolResult {
	b, _ := json.Marshal(v)
	return wire.AgentToolResult{Summary: "", Data: b}
}

func (s *Server) toolContext(ctx context.Context, thread model.ChatThread, _ toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	var project *model.Project
	if thread.ProjectID != nil {
		if p, err := s.Repos.Projects.Get(ctx, *thread.ProjectID); err == nil {
			project = &p
		}
	}
	note := s.Note(ctx, thread, project, nil, "")
	return wire.AgentToolResult{Summary: "Read what's going on now", Data: mustJSON(map[string]string{"context": note})}, nil, nil
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// projectScope is a project with its sub-projects.
func (s *Server) projectScope(ctx context.Context, id string) (map[string]bool, error) {
	projects, err := s.Repos.Projects.List(ctx, wire.ArchivedAll)
	if err != nil {
		return nil, err
	}
	scope := map[string]bool{id: true}
	for _, p := range projects {
		if p.ParentID != nil && *p.ParentID == id {
			scope[p.ID] = true
		}
	}
	return scope, nil
}

func (s *Server) toolTasks(ctx context.Context, thread model.ChatThread, in toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	status := cmp.Or(in.Status, "open")
	tasks, err := s.Repos.Tasks.List(ctx, store.TaskFilter{Status: status})
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	var scope map[string]bool
	projectID := ""
	if in.ProjectID != nil {
		projectID = *in.ProjectID
	} else if thread.ProjectID != nil && !in.Everywhere {
		projectID = *thread.ProjectID
	}
	if projectID != "" {
		p, err := s.findProject(ctx, projectID)
		if err != nil {
			return wire.AgentToolResult{}, nil, err
		}
		if scope, err = s.projectScope(ctx, p.ID); err != nil {
			return wire.AgentToolResult{}, nil, err
		}
	}
	names := s.projectNames(ctx)
	steps := map[string]int{}
	for _, t := range tasks {
		if t.ParentID != nil {
			steps[*t.ParentID]++
		}
	}
	today := s.today()
	text := strings.ToLower(strings.TrimSpace(in.Text))
	type row struct {
		ID       string  `json:"id"`
		Title    string  `json:"title"`
		Stage    string  `json:"stage"`
		Status   string  `json:"status,omitempty"`
		Project  string  `json:"project,omitempty"`
		Due      *string `json:"due,omitempty"`
		Estimate int     `json:"estimate_minutes,omitempty"`
		Priority int     `json:"priority"`
		Steps    int     `json:"open_steps,omitempty"`
		With     string  `json:"waiting_on,omitempty"`
	}
	out := []row{}
	for _, t := range tasks {
		if t.ParentID != nil || (t.OccurrenceDay != nil && *t.OccurrenceDay > today) {
			continue
		}
		if scope != nil && (t.ProjectID == nil || !scope[*t.ProjectID]) {
			continue
		}
		if in.Stage != nil && *in.Stage != "" && t.Stage != *in.Stage {
			continue
		}
		if text != "" && !strings.Contains(strings.ToLower(t.Title+" "+t.Notes), text) {
			continue
		}
		r := row{ID: t.ID[:8], Title: t.Title, Stage: t.Stage, Due: t.DueDay, Priority: t.Priority, Steps: steps[t.ID], With: t.DelegatedTo}
		if status != "open" {
			r.Status = t.Status
		}
		if t.ProjectID != nil {
			r.Project = names[*t.ProjectID]
		}
		if t.Estimate != nil {
			r.Estimate = int(*t.Estimate / time.Minute)
		}
		out = append(out, r)
		if len(out) == 80 {
			break
		}
	}
	res := toolData(map[string]any{"tasks": out})
	res.Summary = fmt.Sprintf("Listed %d %s", len(out), plural(len(out), "task", "tasks"))
	return res, nil, nil
}

func (s *Server) projectNames(ctx context.Context) map[string]string {
	names := map[string]string{}
	if projects, err := s.Repos.Projects.List(ctx, wire.ArchivedAll); err == nil {
		for _, p := range projects {
			names[p.ID] = p.Name
		}
	}
	return names
}

func (s *Server) toolTask(ctx context.Context, _ model.ChatThread, in toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	t, err := s.findTask(ctx, in.ID)
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	all, err := s.Repos.Tasks.List(ctx, store.TaskFilter{Status: "all"})
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	type step struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Done  bool   `json:"done"`
	}
	steps := []step{}
	for _, x := range all {
		if x.ParentID != nil && *x.ParentID == t.ID {
			steps = append(steps, step{x.ID[:8], x.Title, x.Status == model.TaskDone})
		}
	}
	data := map[string]any{"id": t.ID, "title": t.Title, "notes": t.Notes, "stage": t.Stage, "status": t.Status,
		"priority": t.Priority, "due": t.DueDay, "start_day": t.StartDay, "project": s.projectNames(ctx)[deref(t.ProjectID)],
		"waiting_on": t.DelegatedTo, "steps": steps}
	if t.Estimate != nil {
		data["estimate_minutes"] = int(*t.Estimate / time.Minute)
	}
	res := toolData(data)
	res.Summary = fmt.Sprintf("Read %q", t.Title)
	return res, nil, nil
}

func (s *Server) toolPlan(ctx context.Context, _ model.ChatThread, in toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	day := s.today()
	if in.Day != nil && *in.Day != "" {
		if _, err := civil.Parse(*in.Day); err != nil {
			return wire.AgentToolResult{}, nil, fmt.Errorf("%q is not a day like 2026-10-05", *in.Day)
		}
		day = *in.Day
	}
	env := s.planEnv()
	d := civil.MustParse(day)
	plan, err := s.Repos.Plans.Read(ctx, day, env)
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	type block struct {
		Task    string `json:"task"`
		TaskID  string `json:"task_id"`
		Start   string `json:"start,omitempty"`
		Minutes int    `json:"minutes"`
		Status  string `json:"status"`
	}
	blocks := []block{}
	for _, e := range plan.Items {
		b := block{Task: e.Task.Title, TaskID: e.Task.ID[:8], Minutes: int(e.Item.Planned / time.Minute), Status: e.Item.Status}
		if e.Item.StartAt != nil {
			b.Start = s.dayClock(d, *e.Item.StartAt)
		}
		blocks = append(blocks, b)
	}
	busy := []map[string]string{}
	if dc, _, err := s.Repos.Plans.DayPlan(ctx, day, env); err == nil {
		for _, b := range dc.Busy {
			busy = append(busy, map[string]string{"title": cmp.Or(b.Title, "Busy"), "start": s.dayClock(d, b.Start), "end": s.dayClock(d, b.End)})
		}
	}
	res := toolData(map[string]any{"day": day, "blocks": blocks, "busy": busy})
	res.Summary = fmt.Sprintf("Read the plan for %s", s.dayText(d))
	return res, nil, nil
}

func (s *Server) toolGoals(ctx context.Context, _ model.ChatThread, _ toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	goals, err := s.Repos.Goals.List(ctx, model.GoalActive)
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	progress, err := s.Repos.Plans.Progress(ctx, goals, s.planEnv())
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	out := []map[string]any{}
	for _, g := range goals {
		pr := progress[g.ID]
		out = append(out, map[string]any{"id": g.ID[:8], "title": g.Title, "kind": g.Kind, "due": g.DueDay, "done": pr.Done,
			"remaining": pr.Remaining, "pace": pr.Pace, "unit": g.Unit})
	}
	res := toolData(map[string]any{"goals": out})
	res.Summary = fmt.Sprintf("Read %d %s", len(out), plural(len(out), "goal", "goals"))
	return res, nil, nil
}

func (s *Server) toolProjects(ctx context.Context, _ model.ChatThread, _ toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	projects, err := s.Repos.Projects.List(ctx, wire.ArchivedFalse)
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	names := map[string]string{}
	for _, p := range projects {
		names[p.ID] = p.Name
	}
	out := []map[string]any{}
	for _, p := range projects {
		row := map[string]any{"id": p.ID[:8], "name": p.Name, "folder": p.Folder}
		if p.ParentID != nil {
			row["under"] = names[*p.ParentID]
		}
		out = append(out, row)
	}
	res := toolData(map[string]any{"projects": out})
	res.Summary = fmt.Sprintf("Read %d projects", len(out))
	return res, nil, nil
}

func (s *Server) toolTime(ctx context.Context, _ model.ChatThread, in toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	today := civil.MustParse(s.today())
	from, to := cmp.Or(in.From, today.AddDays(-6).String()), cmp.Or(in.To, today.String())
	sum, err := s.Repos.Stats.Summary(ctx, from, to, today.String(), s.now())
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	out := []map[string]any{}
	for _, p := range sum.ByProject {
		out = append(out, map[string]any{"project": p.Name, "minutes": int(p.Worked / time.Minute)})
	}
	res := toolData(map[string]any{"from": from, "to": to, "worked_minutes": int(sum.Worked / time.Minute), "by_project": out})
	res.Summary = "Read the time tracked"
	return res, nil, nil
}

func (s *Server) toolSessions(ctx context.Context, thread model.ChatThread, in toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	if s.Code == nil {
		return toolData(map[string]any{"sessions": []any{}}), nil, nil
	}
	folder := ""
	projectID := deref(thread.ProjectID)
	if in.ProjectID != nil {
		projectID = *in.ProjectID
	}
	if projectID != "" && !in.Everywhere {
		if p, err := s.findProject(ctx, projectID); err == nil {
			folder = p.Folder
		}
	}
	out := []map[string]any{}
	for _, cs := range s.Code.Sessions().Sessions {
		if cs.State == "ended" {
			continue
		}
		if folder != "" && cs.Cwd != folder && !strings.HasPrefix(cs.Cwd, folder+"/") {
			continue
		}
		out = append(out, map[string]any{"id": cs.ID, "name": cs.Name, "folder": cs.Cwd, "state": cs.State,
			"doing": cmp.Or(cs.Step, agentCut(cs.LastMessage, 200)), "sandbox": cs.Sandbox})
	}
	res := toolData(map[string]any{"sessions": out})
	res.Summary = fmt.Sprintf("Read %d %s", len(out), plural(len(out), "session", "sessions"))
	return res, nil, nil
}

func (s *Server) toolTranscript(_ context.Context, _ model.ChatThread, in toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	if s.Code == nil {
		return wire.AgentToolResult{}, nil, errors.New("sessions aren't reachable on this device")
	}
	n := in.Turns
	if n <= 0 || n > 30 {
		n = 6
	}
	tr, err := s.Code.Transcript(in.SessionID, 0, 0, n)
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	res := toolData(tr)
	res.Summary = "Read a session's last turns"
	return res, nil, nil
}

func (s *Server) toolRecall(ctx context.Context, thread model.ChatThread, in toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	if s.Memory == nil {
		return wire.AgentToolResult{}, nil, errors.New("memory is off on this device")
	}
	n, err := s.Memory.Note(ctx, memory.NoteRequest{Text: in.Query, ThreadID: thread.ID, AsOf: s.now(),
		Profile: memory.ProfileFrontier, RefStart: 1})
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	res := toolData(map[string]string{"memory": cmp.Or(n.Text, "Nothing in memory bears on that.")})
	res.Summary = "Searched her memory"
	return res, nil, nil
}

// cutText is s cut to n bytes on a character boundary, its lines kept.
func cutText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "\n[cut]"
}

// toolChanges is what a session changed, or the project's uncommitted files,
// and one file's diff.
func (s *Server) toolChanges(ctx context.Context, thread model.ChatThread, in toolArgs) (wire.AgentToolResult, json.RawMessage, error) {
	if in.SessionID != "" {
		if s.Code == nil {
			return wire.AgentToolResult{}, nil, errors.New("sessions aren't reachable on this device")
		}
		if in.Path != "" {
			d, err := s.Code.Diff(ctx, in.SessionID, in.Path)
			if err != nil {
				return wire.AgentToolResult{}, nil, err
			}
			res := toolData(map[string]any{"path": d.Path, "diff": cutText(d.Diff, 12000), "truncated": d.Truncated || len(d.Diff) > 12000})
			res.Summary = "Read the diff of " + d.Path
			return res, nil, nil
		}
		ch, err := s.Code.Changes(ctx, in.SessionID)
		if err != nil {
			return wire.AgentToolResult{}, nil, err
		}
		res := toolData(ch)
		res.Summary = fmt.Sprintf("Read what the session changed (%d %s)", len(ch.Files), plural(len(ch.Files), "file", "files"))
		return res, nil, nil
	}
	if thread.ProjectID == nil {
		return wire.AgentToolResult{}, nil, errors.New("say which session; this chat has no project folder")
	}
	p, err := s.Repos.Projects.Get(ctx, *thread.ProjectID)
	if err != nil {
		return wire.AgentToolResult{}, nil, err
	}
	if p.Folder == "" {
		return wire.AgentToolResult{}, nil, fmt.Errorf("%s has no folder on this computer", p.Name)
	}
	args := []string{"-C", p.Folder, "diff", "--stat", "HEAD"}
	if in.Path != "" {
		args = []string{"-C", p.Folder, "diff", "HEAD", "--", in.Path}
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, "git", args...).Output()
	if err != nil {
		return wire.AgentToolResult{}, nil, fmt.Errorf("git couldn't read %s's changes", p.Name)
	}
	status, _ := exec.CommandContext(cctx, "git", "-C", p.Folder, "status", "--short").Output()
	res := toolData(map[string]string{"status": cutText(string(status), 4000), "diff": cutText(string(out), 12000)})
	res.Summary = "Read the project's uncommitted changes"
	return res, nil, nil
}

// Sessions implements agent.Sources.
func (s *Server) Sessions() []wire.CodeSession {
	if s.Code == nil {
		return nil
	}
	return s.Code.Sessions().Sessions
}
