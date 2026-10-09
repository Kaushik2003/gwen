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

/** The faces the assistant makes with a reply (llm.Moods), each a sprite in assets/gwen. */
export type Mood = "smiling" | "happy" | "love" | "winking" | "cool" | "smug" | "thinking" | "shocked" | "annoyed" | "grumpy" | "angry" | "crying";

/** How the assistant chose, by itself, to treat the user for a while (llm.Attitudes). */
export type Attitude = "playful" | "focused" | "soft" | "competitive" | "protective" | "excited" | "quiet" | "angry";

/** Each attitude in words, with the face that goes with it. */
export const attitudes: Record<Attitude, { label: string; means: string; face: Mood }> = {
  playful: { label: "Playful", means: "Teasing and flirty: things are going fine.", face: "winking" },
  focused: { label: "Focused", means: "Businesslike: less banter, more getting it done.", face: "cool" },
  soft: { label: "Soft", means: "Gentle and supportive: she thinks you're having a hard time.", face: "love" },
  competitive: { label: "Competitive", means: "Daring you to aim higher.", face: "smug" },
  protective: { label: "Protective", means: "Slowing you down: rest, boundaries, no burnout.", face: "thinking" },
  excited: { label: "Excited", means: "Thrilled about a win or a new idea.", face: "happy" },
  quiet: { label: "Quiet", means: "Calm company: few words, no advice unless you ask.", face: "smiling" },
  angry: { label: "Angry", means: "Fed up with excuses: blunt until you follow through.", face: "angry" },
};

/** The attitude of a name, or null for none this build knows. */
export function attitudeOf(id: string | undefined | null) {
  return id && id in attitudes ? attitudes[id as Attitude] : null;
}

export interface AssistantMessage {
  role: "user" | "assistant";
  text: string;
  at: number;
  mood?: Mood;
  /** The attitude she switched to with this turn. */
  attitude?: Attitude;
  actions?: ActionResult[];
}

/** An ok assistant run's output: the whole conversation. */
export interface AssistantOutput {
  messages: AssistantMessage[];
}
