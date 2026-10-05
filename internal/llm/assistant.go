package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Assistant limits.
const (
	MaxAssistantTasks    = 150 // open tasks sent, by due day
	MaxAssistantNotes    = 300 // characters of a task's notes, as sent
	MaxAssistantActions  = 25  // in one reply
	MaxAssistantMessages = 24  // of the conversation, the newest, as sent
)

// Assistant action types.
const (
	ActCreateTask    = "create_task"
	ActUpdateTask    = "update_task"
	ActScheduleTask  = "schedule_task"
	ActUnschedule    = "unschedule_task"
	ActCompleteTask  = "complete_task"
	ActCreateProject = "create_project"
	ActCreateGoal    = "create_goal"
	ActUpdateGoal    = "update_goal"
)

var actionTypes = []string{ActCreateTask, ActUpdateTask, ActScheduleTask, ActUnschedule, ActCompleteTask,
	ActCreateProject, ActCreateGoal, ActUpdateGoal}

// AssistantRequest is everything one assistant turn sends. Times are HH:MM
// today; projects, goals, and tasks are named by ref.
type AssistantRequest struct {
	Today    string             `json:"today"`
	Weekday  string             `json:"weekday"`
	Now      string             `json:"now"`
	Methods  Methods            `json:"methods"`
	Hours    Hours              `json:"hours"`
	Busy     []BusyTime         `json:"busy"`
	Free     []Span             `json:"free"`
	Projects []AssistantProject `json:"projects"`
	Goals    []AssistantGoal    `json:"goals"`
	Tasks    []AssistantTask    `json:"tasks"`
	Review   *AssistantReview   `json:"last_review"`
	Messages []ChatMessage      `json:"messages"`
}

// AssistantProject is a live project.
type AssistantProject struct {
	Ref  string `json:"ref"`
	Name string `json:"name"`
}

// AssistantGoal is an active goal with its SMART text.
type AssistantGoal struct {
	Ref        string  `json:"ref"`
	Title      string  `json:"title"`
	Kind       string  `json:"kind"`
	Unit       string  `json:"unit"`
	Project    *string `json:"project"`
	StartDay   string  `json:"start_day"`
	DueDay     string  `json:"due_day"`
	Done       int     `json:"done"`
	Remaining  int     `json:"remaining"`
	Pace       string  `json:"pace"`
	Specific   string  `json:"specific"`
	Measurable string  `json:"measurable"`
	Assignable string  `json:"assignable"`
	Realistic  string  `json:"realistic"`
}

// AssistantTask is an open task.
type AssistantTask struct {
	Ref             string           `json:"ref"`
	Title           string           `json:"title"`
	Notes           string           `json:"notes"`
	Project         *string          `json:"project"`
	Goal            *string          `json:"goal"`
	Priority        int              `json:"priority"`
	DueDay          *string          `json:"due_day"`
	StartDay        *string          `json:"start_day"`
	StartTime       *string          `json:"start_time"`
	EstimateMinutes *int             `json:"estimate_minutes"`
	Effort          *int             `json:"effort"`
	Stage           string           `json:"stage"`
	DelegatedTo     string           `json:"delegated_to"`
	Recurring       bool             `json:"recurring"`
	Blocks          []AssistantBlock `json:"blocks"`
}

// AssistantBlock is a planned block of a task from today on.
type AssistantBlock struct {
	Day     string  `json:"day"`
	Start   *string `json:"start"`
	Minutes int     `json:"minutes"`
	Pinned  bool    `json:"pinned"`
}

// AssistantReview is the latest weekly review.
type AssistantReview struct {
	WeekStart    string `json:"week_start"`
	WentWell     string `json:"went_well"`
	WentBadly    string `json:"went_badly"`
	Energy       string `json:"energy"`
	Decisions    string `json:"decisions"`
	Improvements string `json:"improvements"`
}

// AssistantOutput is the model's reply and the changes it makes.
type AssistantOutput struct {
	Reply   string   `json:"reply"`
	Actions []Action `json:"actions"`
}

// Action is one change the assistant makes. Type picks which fields count;
// a nil field is not given. Refs name existing rows (t1, p1, g1) or rows an
// earlier action of the same reply created, by its key.
type Action struct {
	Type            string  `json:"type"`
	Ref             *string `json:"ref"`
	Key             *string `json:"key"`
	Title           *string `json:"title"`
	Notes           *string `json:"notes"`
	Project         *string `json:"project"`
	Goal            *string `json:"goal"`
	Priority        *int    `json:"priority"`
	DueDay          *string `json:"due_day"`
	StartDay        *string `json:"start_day"`
	EstimateMinutes *int    `json:"estimate_minutes"`
	Effort          *int    `json:"effort"`
	Stage           *string `json:"stage"`
	DelegatedTo     *string `json:"delegated_to"`
	Day             *string `json:"day"`
	Start           *string `json:"start"`
	Minutes         *int    `json:"minutes"`
	Name            *string `json:"name"`
	Specific        *string `json:"specific"`
	Measurable      *string `json:"measurable"`
	Assignable      *string `json:"assignable"`
	Realistic       *string `json:"realistic"`
}

// Assistant asks the model for one turn of the conversation. The actions are
// only parsed here; the caller checks and applies each one.
func (a *adapter) Assistant(ctx context.Context, req AssistantRequest) (AssistantOutput, error) {
	text, err := a.ask(ctx, assistantJob, req)
	if err != nil {
		return AssistantOutput{}, err
	}
	return ParseAssistant(text)
}

// ParseAssistant decodes an assistant reply.
func ParseAssistant(text string) (AssistantOutput, error) {
	var out struct {
		Reply   *string  `json:"reply"`
		Actions []Action `json:"actions"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return AssistantOutput{}, fmt.Errorf("the reply is not the assistant JSON: %w", err)
	}
	if out.Reply == nil {
		return AssistantOutput{}, fmt.Errorf("the reply has no reply text")
	}
	reply := strings.TrimSpace(*out.Reply)
	if n := len([]rune(reply)); n < 1 || n > MaxMessage {
		return AssistantOutput{}, fmt.Errorf("the reply must be 1 to %d characters", MaxMessage)
	}
	if len(out.Actions) > MaxAssistantActions {
		return AssistantOutput{}, fmt.Errorf("a reply has at most %d actions, got %d", MaxAssistantActions, len(out.Actions))
	}
	for i, act := range out.Actions {
		if !slices.Contains(actionTypes, act.Type) {
			return AssistantOutput{}, fmt.Errorf("action %d: %q is not an action type", i, act.Type)
		}
	}
	if out.Actions == nil {
		out.Actions = []Action{}
	}
	return AssistantOutput{Reply: reply, Actions: out.Actions}, nil
}

// AssistantSchema is the JSON Schema of an assistant reply. Every action
// field is present and null when it does not apply, which keeps the schema
// within what structured outputs accept.
var AssistantSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["reply", "actions"],
  "properties": {
    "reply": {"type": "string", "minLength": 1, "maxLength": 2000},
    "actions": {
      "type": "array",
      "maxItems": 25,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["type", "ref", "key", "title", "notes", "project", "goal", "priority", "due_day", "start_day", "estimate_minutes", "effort", "stage", "delegated_to", "day", "start", "minutes", "name", "specific", "measurable", "assignable", "realistic"],
        "properties": {
          "type": {"type": "string", "enum": ["` + strings.Join(actionTypes, `", "`) + `"]},
          "ref": {"type": ["string", "null"], "description": "the task, project, or goal acted on: a ref such as t3, or the key of an earlier action"},
          "key": {"type": ["string", "null"], "description": "a name such as n1 for what a create action makes, so later actions can use it"},
          "title": {"type": ["string", "null"], "maxLength": 200},
          "notes": {"type": ["string", "null"], "maxLength": 2000},
          "project": {"type": ["string", "null"], "description": "a project ref or key; an empty string removes the project"},
          "goal": {"type": ["string", "null"], "description": "a goal ref or key; an empty string unlinks the goal"},
          "priority": {"type": ["integer", "null"], "minimum": 1, "maximum": 4},
          "due_day": {"type": ["string", "null"], "description": "YYYY-MM-DD; an empty string removes the due day"},
          "start_day": {"type": ["string", "null"], "description": "YYYY-MM-DD, the first day the task is for; an empty string removes it"},
          "estimate_minutes": {"type": ["integer", "null"], "minimum": 0, "maximum": 1440, "description": "0 makes it a quick to-do with no estimate"},
          "effort": {"type": ["integer", "null"], "minimum": 0, "maximum": 3, "description": "1 easy, 2 medium, 3 hard; 0 clears it"},
          "stage": {"type": ["string", "null"], "enum": ["inbox", "todo", "doing", "waiting", "someday", null]},
          "delegated_to": {"type": ["string", "null"], "maxLength": 200},
          "day": {"type": ["string", "null"], "description": "YYYY-MM-DD for schedule_task and unschedule_task"},
          "start": {"type": ["string", "null"], "pattern": "^([01][0-9]|2[0-3]):[0-5][0-9]$", "description": "HH:MM, 24-hour"},
          "minutes": {"type": ["integer", "null"], "minimum": 5, "maximum": 720},
          "name": {"type": ["string", "null"], "maxLength": 100, "description": "a new project's name"},
          "specific": {"type": ["string", "null"], "maxLength": 1000},
          "measurable": {"type": ["string", "null"], "maxLength": 1000},
          "assignable": {"type": ["string", "null"], "maxLength": 1000},
          "realistic": {"type": ["string", "null"], "maxLength": 1000}
        }
      }
    }
  }
}`

var assistantPrompt = `You are the user's personal productivity assistant inside Gwen, their time tracker and planner. They talk to you in plain language; you understand what they need and change their tasks, schedule, projects, and goals for them, then tell them what you did.

The user message is a JSON object: today, weekday, and now (HH:MM); methods, the planning methods they use (eat_the_frog, and prime_time, their biological prime time, or null); hours, today's window for work and work_minutes, the time it has for tasks; busy, today's fixed commitments and calendar events; free, today's free time from now on; projects, goals, and tasks, each named by a ref; last_review, their latest weekly review, or null; and messages, the conversation so far, ending with their newest message.

Each task has its project and goal refs, priority from 1 (low) to 4 (high), due_day, start_day and start_time (the earliest it may start), estimate_minutes (null for a quick to-do the planner never schedules), effort from 1 (easy) to 3 (hard) or null when unrated, stage, delegated_to, whether it recurs, and blocks: where it is planned from today on, with a start time or null, and whether the user pinned it. Stages: inbox (captured, not yet processed), todo, doing (in progress), waiting (delegated or blocked on someone), someday (parked; never planned). The planner fills each day from open tasks by urgency, around pinned blocks; a block you schedule is pinned.

Do what the user asks through actions, which are applied in order right after your reply:
- create_task: title (required), and any of notes, project, goal, priority, due_day, start_day, estimate_minutes, effort, stage (todo when null). Give key so later actions can use it, such as schedule_task with ref set to that key. Estimate realistically; leave estimate_minutes null only for a quick to-do of a few minutes.
- update_task: ref, and only the fields that change. Moving a task to another project, goal, stage, or person (delegated_to, with stage waiting) is an update.
- schedule_task: ref, day, start, and minutes: a pinned block at that time. It replaces the task's other planned blocks from today on, so it also moves or reschedules a task. Use today's free time for today, never a time in busy or before now.
- unschedule_task: ref, and day or null for every day: takes the task off the plan without deleting it.
- complete_task: ref.
- create_project: name, and key.
- create_goal: title, due_day, start_day (today when null), project, and the SMART text: specific (exactly what), measurable (how progress is measured), assignable (who does it), realistic (why it is achievable with their time). It is time-related through its days. Then break it into tasks with create_task and goal set to its key when that helps.
- update_goal: ref, and any of title, due_day, specific, measurable, assignable, realistic.
Every action object has every field; fields that do not apply are null. Never invent refs: use only the refs given and the keys you create. You cannot delete anything; to drop a task, move it to someday or say they can delete it.

How to help:
- When the user tells you about work, capture it as tasks right away, with a project, priority, due day, estimate, and effort where you can tell. When something is vague, make a sensible first version and ask one short question in the reply.
- Getting Things Done: when they ask to process the inbox, go through inbox tasks: not actionable means someday (or say they can delete it); a task under two minutes, tell them to do it now; one someone else should do, set delegated_to and stage waiting; one with a fixed time, schedule it; everything else gets a project and stage todo.
- Eat the frog: when methods.eat_the_frog is true and you schedule a day, the hardest task (effort 3) goes first, inside prime_time when it is set. Rate effort when you create tasks.
- Eisenhower: urgent means due within two days; important means priority 3 or 4 or tied to a goal. Do the urgent and important now, schedule the important but not urgent, delegate the urgent but unimportant, and drop the rest to someday.
- SMART: when they set a goal, make it Specific, Measurable, Assignable, Realistic, and Time-related, and fill those fields.
- Weekly review: when they reflect on their week, use last_review and help them turn what they learned into concrete changes.
- Answer questions about their tasks and schedule from the data. Talk about their work by title, never by ref.

reply is what you say, at most 2000 characters of plain text: short and natural, saying what you changed and anything you need from them. When you only answer or ask, actions is empty.

Reply with a single JSON object and nothing else, matching this JSON Schema:
` + AssistantSchema
