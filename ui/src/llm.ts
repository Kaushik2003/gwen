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

export interface RunError {
  error: string;
}
