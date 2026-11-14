import type { wire } from "../api";

// What a task is doing right now, decided in one place so every screen says the same thing.
// The order matters: a finished task is done whatever else is true; the task you are tracking
// is "Tracking" even if it was in the inbox a minute ago; an agent working on it outranks its stage.

export type TaskStatusKind =
  | "done"
  | "tracking"
  | "agent-needs"
  | "agent-working"
  | "agent-finished"
  | "agent-stopped"
  | "delegated"
  | "waiting"
  | "inbox"
  | "doing"
  | "someday"
  | "todo";

export type StatusTone = "done" | "working" | "warning" | "info" | "success" | "muted" | "accent";

export interface TaskStatus {
  kind: TaskStatusKind;
  /** A few words: "Tracking", "Claude needs you", "To process". */
  label: string;
  tone: StatusTone;
  /** True for statuses that mean something is running now (tracking, an agent at work). */
  live: boolean;
}

/** A live session working on the task, as the shell sees it: its state and who it is. */
export interface AgentOnTask {
  state: string;
  name: string;
}

/** A stage as a heading or label says it, everywhere. */
export const stageLabels: Record<string, string> = { inbox: "To process", todo: "To do", doing: "In progress", waiting: "Waiting", someday: "Someday", done: "Done" };
export function stageName(stage: string): string {
  return stageLabels[stage] ?? "To do";
}
/** The stages in the order you work through them, for pickers. */
export const stageOrder = ["inbox", "todo", "doing", "waiting", "someday"] as const;
/** Stage names as a pick-list shows them ("Move to In progress"). */
export const stageChoices: { value: string; label: string }[] = [
  { value: "inbox", label: "Inbox" },
  { value: "todo", label: "To do" },
  { value: "doing", label: "In progress" },
  { value: "waiting", label: "Waiting" },
  { value: "someday", label: "Someday" },
];

export function taskStatus(task: wire.Task, ctx: { tracking: boolean; agent?: AgentOnTask | null }): TaskStatus {
  if (task.status === "done") return { kind: "done", label: "Done", tone: "done", live: false };
  if (ctx.tracking) return { kind: "tracking", label: "Tracking", tone: "working", live: true };
  const a = ctx.agent;
  if (a) {
    if (a.state === "needs_you" || a.state === "blocked") return { kind: "agent-needs", label: `${a.name} needs you`, tone: "warning", live: true };
    if (a.state === "working" || a.state === "starting") return { kind: "agent-working", label: `${a.name} is working`, tone: "info", live: true };
    if (a.state === "done") return { kind: "agent-finished", label: `${a.name} finished`, tone: "success", live: false };
    return { kind: "agent-stopped", label: `${a.name} stopped`, tone: "muted", live: false };
  }
  if (task.stage === "waiting") return task.delegated_to ? { kind: "delegated", label: `With ${task.delegated_to}`, tone: "info", live: false } : { kind: "waiting", label: "Waiting", tone: "muted", live: false };
  if (task.stage === "inbox") return { kind: "inbox", label: "To process", tone: "accent", live: false };
  if (task.stage === "doing") return { kind: "doing", label: "In progress", tone: "info", live: false };
  if (task.stage === "someday") return { kind: "someday", label: "Someday", tone: "muted", live: false };
  return { kind: "todo", label: "To do", tone: "muted", live: false };
}
