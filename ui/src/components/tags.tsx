import { CalendarDays, Clock, Flag, Gauge } from "lucide-react";
import type { wire } from "../api";
import { daysBetween, formatDate } from "../format";
import { clockOf } from "../rrule";
import { stageLabel } from "./ScheduleInput";
import { Dot, cx, priorityOf, unassignedColor } from "./ui";

/** A project's dot and name; "Unassigned" without one. */
export function ProjectTag({ project, className }: { project?: wire.Project | null; className?: string }) {
  return (
    <span className={cx("inline-flex min-w-0 items-center gap-1.5 text-xs text-ink-subtle", className)}>
      <Dot color={project?.color ?? unassignedColor} size={7} />
      <span className="truncate">{project?.name ?? "Unassigned"}</span>
    </span>
  );
}

/** When a task is due, in red once it is late. */
export function DueTag({ day, today, done }: { day: string; today: string; done?: boolean }) {
  const n = daysBetween(today, day);
  const text = n === 0 ? "Due today" : n === 1 ? "Due tomorrow" : n < 0 ? `${-n} ${n === -1 ? "day" : "days"} late` : `Due ${formatDate(day)}`;
  const color = done ? "text-ink-faint" : n < 0 ? "text-danger" : n === 0 ? "text-idle" : "text-ink-subtle";
  return (
    <span className={cx("inline-flex shrink-0 items-center gap-1 text-xs whitespace-nowrap", color)} title={day}>
      <CalendarDays size={12} aria-hidden />
      {text}
    </span>
  );
}

/** A flag in the priority's colour; normal priority shows nothing. */
export function PriorityFlag({ priority, always }: { priority: number; always?: boolean }) {
  if (priority === 2 && !always) return null;
  const p = priorityOf(priority);
  return (
    <span className="inline-flex shrink-0 items-center gap-1 text-xs" style={{ color: p.color }} title={`${p.label} priority`}>
      <Flag size={12} aria-hidden fill={priority >= 3 ? p.color : "none"} />
      {p.label}
    </span>
  );
}

/** When a task starts: shown for a start time, or a start day still ahead. */
export function StartTag({ task, today }: { task: wire.Task; today: string }) {
  if (!task.start_day || (task.start_day <= today && task.start_minute == null)) return null;
  const n = daysBetween(today, task.start_day);
  const day = n === 0 ? "Today" : n === 1 ? "Tomorrow" : n < 0 ? formatDate(task.start_day) : `From ${formatDate(task.start_day)}`;
  return (
    <span className="inline-flex shrink-0 items-center gap-1 text-xs whitespace-nowrap text-ink-subtle" title={task.start_day}>
      <Clock size={12} aria-hidden />
      {day}
      {task.start_minute != null && ` ${clockOf(task.start_minute)}`}
    </span>
  );
}

const effortStyle: Record<number, { label: string; color: string }> = {
  1: { label: "Easy", color: "#4cb782" },
  2: { label: "Medium", color: "#8a8f98" },
  3: { label: "Hard", color: "#eb5757" },
};

/** How hard a task is; a hard one is a frog, done first. Unrated shows nothing. */
export function EffortTag({ effort }: { effort?: number | null }) {
  if (!effort) return null;
  const e = effortStyle[effort];
  return (
    <span className="inline-flex shrink-0 items-center gap-1 text-xs whitespace-nowrap" style={{ color: e.color }} title={effort === 3 ? "Hard: a frog, planned first" : `${e.label} effort`}>
      <Gauge size={12} aria-hidden />
      {e.label}
    </span>
  );
}

/** A stage other than To do, and who a waiting task waits on. */
export function StageTag({ task }: { task: wire.Task }) {
  if (task.status === "done" || task.stage === "todo" || !task.stage) return null;
  const label = task.stage === "waiting" && task.delegated_to ? `Waiting on ${task.delegated_to}` : stageLabel(task.stage);
  return (
    <span className="inline-flex h-5 shrink-0 items-center rounded-full bg-surface-3 px-2 text-xs whitespace-nowrap text-ink-muted" title={label}>
      {label}
    </span>
  );
}
