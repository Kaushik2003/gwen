package llm

// The prompts sent with every request. Each asks for a single JSON object and
// includes its schema verbatim (docs/07-integrations.md#llm-adapter).

// BreakdownSchema is the JSON Schema of a breakdown reply.
const BreakdownSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["tasks"],
  "properties": {
    "tasks": {
      "type": "array",
      "minItems": 1,
      "maxItems": 50,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["title", "notes", "estimate_minutes", "due_day", "priority", "quantity"],
        "properties": {
          "title": {"type": "string", "minLength": 1, "maxLength": 200},
          "notes": {"type": "string"},
          "estimate_minutes": {"type": "integer", "minimum": 5, "maximum": 480},
          "due_day": {"type": "string", "format": "date", "description": "YYYY-MM-DD, from today to the goal's due_day"},
          "priority": {"type": "integer", "minimum": 1, "maximum": 4},
          "quantity": {"type": ["integer", "null"], "minimum": 1, "description": "units of the goal this task covers"}
        }
      }
    }
  }
}`

const breakdownPrompt = `You help a student break a goal into concrete study tasks.

The user message is a JSON object describing the goal: its title, kind ("quantity" goals count units such as problems; "tasks" goals are finished by completing tasks), unit, target_quantity, minutes_per_unit, start_day, due_day, the titles of tasks already linked to it (do not repeat them), the user's instructions, and today's date.

Propose between 1 and 50 tasks that together reach the goal by its due day. Give each a short title, optional notes, an estimate in minutes from 5 to 480, a due_day between today and the goal's due_day, a priority from 1 (low) to 4 (high), and for quantity goals the number of units it covers (null otherwise).

Reply with a single JSON object and nothing else, matching this JSON Schema:
` + BreakdownSchema

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

The user message is a JSON object for one week: each day's worked and break time against its target, time per project, the titles of tasks completed, each active goal's progress and pace, and planned versus done plan minutes.

Write at most 4000 characters of Markdown: what went well, what slipped, and one or two concrete suggestions for next week. Use plain paragraphs and short lists. Be specific with the numbers; do not invent anything that is not in the data.

Reply with a single JSON object and nothing else, matching this JSON Schema:
` + RetroSchema
