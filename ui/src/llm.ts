// The shapes of an LlmRun's output (internal/wire/llm.go). The host passes
// output as untyped JSON, so Wails does not generate these.

export interface ProposedTask {
  title: string;
  notes: string;
  estimate_minutes: number;
  due_day: string;
  priority: number;
  quantity: number | null;
}

export interface BreakdownOutput {
  tasks: ProposedTask[];
}

export interface RetroOutput {
  markdown: string;
  generated_by: "llm" | "rules";
}

export interface ChatMessage {
  role: "user" | "assistant";
  text: string;
}

export interface ProposedBlock {
  task_id: string;
  title: string;
  project_id: string | null;
  start_at: number;
  planned_minutes: number;
}

/** An ok day plan run's output (docs/07-integrations.md#day-plan). */
export interface DayPlanOutput {
  messages: ChatMessage[];
  items: ProposedBlock[];
  hours: { start_minute: number | null; work_minutes: number | null } | null;
}

export interface RunError {
  error: string;
}

/** One change the assistant made, or tried to. */
export interface ActionResult {
  type: string;
  ok: boolean;
  summary: string;
  error?: string;
  task_id?: string;
  project_id?: string;
  goal_id?: string;
}

export interface AssistantMessage {
  role: "user" | "assistant";
  text: string;
  at: number;
  actions?: ActionResult[];
}

/** An ok assistant run's output: the whole conversation. */
export interface AssistantOutput {
  messages: AssistantMessage[];
}
