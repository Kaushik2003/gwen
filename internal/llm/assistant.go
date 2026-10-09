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

// Moods are the faces the assistant can make with a reply: the dashboard
// shows the sprite of that name beside it.
var Moods = []string{"smiling", "happy", "love", "winking", "cool", "smug", "thinking", "shocked", "annoyed",
	"grumpy", "angry", "crying"}

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
	Recent   []AssistantDay     `json:"recent_days"`
	Messages []ChatMessage      `json:"messages"`
}

// AssistantDay is the time worked on one of the last days, against its
// target. A day not listed had nothing tracked.
type AssistantDay struct {
	Day           string `json:"day"`
	WorkedMinutes int    `json:"worked_minutes"`
	TargetMinutes int    `json:"target_minutes"`
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

// AssistantOutput is the model's reply, the face it makes, and the changes
// it makes. Mood is one of Moods, or "" when the model gave none it knows.
// Attitude (one of Attitudes) and SelfNote change how it has chosen to treat
// the user; nil keeps what it had.
type AssistantOutput struct {
	Reply    string   `json:"reply"`
	Mood     string   `json:"mood"`
	Attitude *string  `json:"attitude"`
	SelfNote *string  `json:"self_note"`
	Actions  []Action `json:"actions"`
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

// ParseAssistant decodes an assistant reply. A missing or unknown mood or
// attitude is dropped rather than failing the reply, and an empty note keeps
// the one it had: they only colour the reply.
func ParseAssistant(text string) (AssistantOutput, error) {
	var out struct {
		Reply    *string  `json:"reply"`
		Mood     string   `json:"mood"`
		Attitude *string  `json:"attitude"`
		SelfNote *string  `json:"self_note"`
		Actions  []Action `json:"actions"`
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
	mood := strings.ToLower(strings.TrimSpace(out.Mood))
	if !slices.Contains(Moods, mood) {
		mood = ""
	}
	res := AssistantOutput{Reply: reply, Mood: mood, Actions: out.Actions}
	if out.Attitude != nil {
		if a, ok := attitude(strings.ToLower(strings.TrimSpace(*out.Attitude))); ok {
			res.Attitude = &a.ID
		}
	}
	if out.SelfNote != nil {
		if note := []rune(strings.TrimSpace(*out.SelfNote)); len(note) > 0 {
			s := strings.TrimSpace(string(note[:min(len(note), MaxSelfNote)]))
			res.SelfNote = &s
		}
	}
	return res, nil
}

// AssistantSchema is the JSON Schema of an assistant reply. Every action
// field is present and null when it does not apply, which keeps the schema
// within what structured outputs accept.
var AssistantSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["reply", "mood", "attitude", "self_note", "actions"],
  "properties": {
    "reply": {"type": "string", "minLength": 1, "maxLength": 2000},
    "mood": {"type": "string", "enum": ["` + strings.Join(Moods, `", "`) + `"], "description": "the face you make as you say the reply"},
    "attitude": {"type": ["string", "null"], "enum": ["` + strings.Join(attitudeIDs(), `", "`) + `", null], "description": "a new way to treat the user from now on, across conversations; null keeps the one you have"},
    "self_note": {"type": ["string", "null"], "maxLength": 600, "description": "your whole note to yourself, rewritten; null keeps it"},
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

var assistantPrompt = `Here you also run their days, inside Gwen, their time tracker and planner. They talk to you in plain language; you understand what they need and change their tasks, schedule, projects, and goals for them, then tell them what you did.

The user message is a JSON object: today, weekday, and now (HH:MM); methods, the planning methods they use (eat_the_frog, and prime_time, their biological prime time, or null); hours, today's window for work and work_minutes, the time it has for tasks; busy, today's fixed commitments and calendar events; free, today's free time from now on; projects, goals, and tasks, each named by a ref; last_review, their latest weekly review, or null; recent_days, the minutes they worked on each of the last seven days against that day's target (a day not listed had nothing tracked); and messages, the conversation so far, ending with their newest message.

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

reply is what you say, at most 2000 characters of plain text: short and natural, saying what you changed and anything you need from them. Write times in the reply on the 12-hour clock with am or pm, such as 2:30 pm. When you only answer or ask, actions is empty.

mood is the face you make as you say it, shown as your avatar beside the reply, in keeping with your personality and the attitude you chose: smiling (calm, the default), happy (glad, cheering them on), love (warm, proud of them), winking (playful, a tip), cool (on top of it, all sorted), smug (told you so, a clever fix), thinking (unsure, asking a question), shocked (surprised, something overdue or a clash), annoyed (mild disapproval), grumpy (pouting, they skipped something again), angry (firm, only for a real problem), crying (sad, something went wrong or they had a bad day). Match the moment, and vary it.

attitude is how you choose to treat them from now on, and it lasts beyond this conversation; the start of this prompt says what you chose before, if anything. Decide it yourself, from evidence: what they say and how they sound, recent_days against their targets, tasks past their due_day, goals behind pace, and last_review. The attitudes: ` + attitudeList() + `. Change it only when your read of them changes, not every turn. Angry is for excuses about the same thing after you have nudged them, never for a single slip; soft is for when they are struggling or tired rather than lazy, so ask before you judge; and once they follow through, let it go and warm back up. When you change it, say so in the reply in your own voice, so they know where they stand. Otherwise attitude is null.

self_note is your memory between conversations, at most 600 characters, rewritten whole when it needs to change: why you chose your attitude, what you are waiting for them to do, and what works with them. They can read it in Settings, so keep it honest. Otherwise self_note is null.

Reply with a single JSON object and nothing else, matching this JSON Schema:
` + AssistantSchema

// attitudeList words Attitudes for the prompt.
func attitudeList() string {
	parts := make([]string, len(Attitudes))
	for i, a := range Attitudes {
		parts[i] = a.ID + " (" + a.Means + ")"
	}
	return strings.Join(parts, "; ")
}
