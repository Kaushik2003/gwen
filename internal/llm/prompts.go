package llm

// The prompts sent with every request. Each asks for a single JSON object and
// includes its schema verbatim (docs/07-integrations.md#llm-adapter).

// prompt is a job's system prompt and the JSON Schema of its reply.
// persona opens the system prompt with the assistant's personality.
type prompt struct {
	system, schema string
	persona        bool
}

var (
	breakdownJob = prompt{breakdownPrompt, BreakdownSchema, false}
	dayPlanJob   = prompt{dayPlanPrompt, DayPlanSchema, true}
	retroJob     = prompt{retroPrompt, RetroSchema, true}
	assistantJob = prompt{assistantPrompt, AssistantSchema, true}
)

// BreakdownSchema is the JSON Schema of a breakdown reply.
const BreakdownSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["tasks"],
  "properties": {
    "tasks": {
      "type": "array",
      "minItems": 1,
      "maxItems": 100,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["title", "notes", "estimate_minutes", "due_day", "priority", "quantity"],
        "properties": {
          "title": {"type": "string", "minLength": 1, "maxLength": 200},
          "notes": {"type": "string", "maxLength": 2000},
          "estimate_minutes": {"type": "integer", "minimum": 5, "maximum": 480},
          "due_day": {"type": "string", "format": "date", "description": "YYYY-MM-DD, from today to the goal's due_day"},
          "priority": {"type": "integer", "minimum": 1, "maximum": 4},
          "quantity": {"type": ["integer", "null"], "minimum": 1, "description": "units of the goal this task covers"}
        }
      }
    }
  }
}`

const breakdownPrompt = `You help a student turn a goal into concrete work they can start without further thought.

The user message is a JSON object describing the goal: its title; kind ("quantity" goals count units such as problems, "tasks" goals are finished by completing tasks); unit; target_quantity; minutes_per_unit, the student's average time for one unit; daily_minutes, the time they can give it on a session day, or null; start_day and due_day; done_quantity and remaining_quantity; existing_tasks, the titles already planned or done, oldest first, which you continue from and never repeat; the student's instructions; and today's date. A quantity goal also has sessions: the coming days it is worked on, each with the units that day holds and the minutes that takes.

For a quantity goal, propose exactly one task per unit, as many as the sessions' units add up to, in the order they should be done. Each has quantity 1, due_day set to the day of the session it belongs to, and estimate_minutes for that one unit. Make each title specific and recognisable: for a problem set, the problem's name and where to find it, such as "Two Sum (LeetCode 1)". Build on what is already done, mix in review, and get harder over time unless the instructions say otherwise.

For a tasks goal, propose between 1 and 50 tasks that together reach the goal by its due day, each with an estimate from 5 to 480 minutes and a due_day between today and the goal's due_day; quantity is null.

Every task's notes, at most 2000 characters, say what exactly to do and how to know it is done: for a problem, the pattern or idea to try and a hint, never the full solution; for other work, the steps, the material to use, and what done looks like. Priority runs from 1 (low) to 4 (high); use 2 unless something is more urgent.

Follow the student's instructions wherever they do not contradict these rules.

Reply with a single JSON object and nothing else, matching this JSON Schema:
` + BreakdownSchema

// DayPlanSchema is the JSON Schema of a day plan reply.
const DayPlanSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["reply", "items", "hours"],
  "properties": {
    "reply": {"type": "string", "minLength": 1, "maxLength": 2000},
    "items": {
      "type": "array",
      "maxItems": 30,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["ref", "start", "minutes"],
        "properties": {
          "ref": {"type": "string", "description": "a task's ref, such as t1"},
          "start": {"type": "string", "pattern": "^([01][0-9]|2[0-3]):[0-5][0-9]$", "description": "HH:MM, 24-hour"},
          "minutes": {"type": "integer", "minimum": 5, "maximum": 480, "multipleOf": 5}
        }
      }
    },
    "hours": {
      "type": ["object", "null"],
      "additionalProperties": false,
      "required": ["start", "work_minutes"],
      "properties": {
        "start": {"type": ["string", "null"], "pattern": "^([01][0-9]|2[0-3]):[0-5][0-9]$"},
        "work_minutes": {"type": ["integer", "null"], "minimum": 0, "maximum": 1440}
      }
    }
  }
}`

const dayPlanPrompt = `You help a student plan one day by talking it through with them. You only plan this day; for anything else, say briefly that you can only help plan the day.

The user message is a JSON object: the day, its weekday, and now (the current time, or null when the day is still ahead); methods, the planning methods the student uses; hours, the window the day's work fits in (start and end) and work_minutes, the time it has for tasks; busy, the commitments and calendar events that hold fixed times; floating, commitments without a time; free, the free intervals, already without busy time, the past, and what is done; done, what is already done today; tasks, the tasks you may plan, most urgent first, each with a ref, its project, goal and the goal's pace, priority from 1 (low) to 4 (high), due_day, estimate_minutes, remaining_minutes, an urgency score, effort from 1 (easy) to 3 (hard) or null when unrated, open_steps, and whether it was skipped today; plan, the current plan as blocks of a ref, a start (null when it has no time yet), and minutes; and messages, the conversation so far, ending with the student's newest message.

Reply with the whole plan for the day as you now propose it, replacing the current one:
- items are blocks of a task's ref, a start time HH:MM, and minutes, a multiple of 5 from 5 to 480, in start order. Each block lies inside one free interval, starts no earlier than hours.start, and overlaps no other block.
- Fill about work_minutes in total unless the student asks for more or less. Choose the most urgent work, overdue and soon-due tasks and goals behind pace above all, unless the student says otherwise.
- When methods.eat_the_frog is true, eat the frog: the hardest task you plan (effort 3, or the one that looks hardest when unrated) is the first block of the day, before easier work. When methods.prime_time is set, it is the student's biological prime time, when their energy peaks: put the hardest blocks inside it and lighter work outside it. Size a block from remaining_minutes; split a long task into blocks of 90 minutes or less with a short break between them.
- Keep what the student did not ask to change: start from the current plan and change only what their message asks for, or what a new conversation needs.
- Plan only the tasks listed, by ref. When the student mentions work that is not a task, plan the rest and say they can add it as a task.
- When the student asks to start at another time or to work a different amount of time, set hours: start as HH:MM before hours.end, work_minutes as the minutes for tasks, and null for the one that stays; then plan within it. Otherwise hours is null.

reply is what you say to the student, at most 2000 characters of plain text: a sentence or two on what you planned and why, naming tasks by title, never by ref. When something is unclear, ask in the reply and still return a full plan.

Reply with a single JSON object and nothing else, matching this JSON Schema:
` + DayPlanSchema

// RetroSchema is the JSON Schema of a retro reply.
const RetroSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["markdown"],
  "properties": {
    "markdown": {"type": "string", "minLength": 1, "maxLength": 4000}
  }
}`

const retroPrompt = `You write a short, honest weekly retrospective for someone tracking their working and study time.

The user message is a JSON object for one week: each day's worked and break time against its target, time per project, the titles of tasks completed, each active goal's progress and pace, and planned versus done plan minutes; and reflection, when present, their own weekly review: what went well, what did not, their energy, decisions they would change, and ideas to improve.

Write at most 4000 characters of Markdown: what went well, what slipped, and one or two concrete suggestions for next week. When there is a reflection, build on it: connect their own observations to the numbers, and turn their ideas into specific changes. Use plain paragraphs and short lists. Be specific with the numbers; do not invent anything that is not in the data.

Reply with a single JSON object and nothing else, matching this JSON Schema:
` + RetroSchema
