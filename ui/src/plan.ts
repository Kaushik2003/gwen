import { wire } from "./api";
import type { FixedBlock } from "./components/Timeline";
import { formatTime } from "./format";

/**
 * One task's part of a day's plan. The planner may give a task more than one
 * block (long work is split, so are AI plans), but a task is one row: one
 * checkbox, one list of steps.
 */
export interface PlanGroup {
  task: wire.Task;
  steps: wire.Task[];
  /** The task's blocks on the day, in the order they start. */
  items: wire.PlanItem[];
  /** When its first block that is not skipped starts, or null when none has a time. */
  start: number | null;
  /** Minutes in its blocks that are planned or done. */
  minutes: number;
  done: boolean;
  /** Every block is skipped: the task is off this day. */
  skipped: boolean;
  /** A block keeps a time set by hand; the rest of the day flows around it. */
  pinned: boolean;
  rollover: number;
  /** Where it stands in the day's order. */
  position: number;
}

const byStart = (a: { start_at?: number | null; position: number }, b: { start_at?: number | null; position: number }) =>
  (a.start_at ?? Infinity) - (b.start_at ?? Infinity) || a.position - b.position;

/** A day's plan as one group per task, in the order the day runs: by start, those without a time last. */
export function groupPlan(items: wire.PlanItem[]): PlanGroup[] {
  const byTask = new Map<string, wire.PlanItem[]>();
  for (const it of items) byTask.set(it.task_id, [...(byTask.get(it.task_id) ?? []), it]);
  const groups = [...byTask.values()].map((blocks): PlanGroup => {
    const sorted = [...blocks].sort(byStart);
    const live = sorted.filter((it) => it.status !== "skipped");
    return {
      task: sorted[0].task,
      steps: sorted[0].steps ?? [],
      items: sorted,
      start: live.find((it) => it.start_at != null)?.start_at ?? null,
      minutes: live.reduce((n, it) => n + it.planned_minutes, 0),
      done: sorted[0].task.status === "done",
      skipped: live.length === 0,
      pinned: sorted.some((it) => it.pinned && it.status === "planned"),
      rollover: Math.max(...sorted.map((it) => it.rollover_count)),
      position: Math.min(...sorted.map((it) => it.position)),
    };
  });
  return groups.sort((a, b) => (a.start ?? Infinity) - (b.start ?? Infinity) || a.position - b.position);
}

/** "14:00", or "14:00 · 15:40" for a task planned in more than one block. */
export function blockTimes(g: PlanGroup): string {
  return g.items
    .filter((it) => it.status !== "skipped" && it.start_at != null)
    .map((it) => formatTime(it.start_at!))
    .join(" · ");
}

/** The block a change to the group goes through: its first that is still planned, else its first. */
export function leadItem(g: PlanGroup): wire.PlanItem {
  return g.items.find((it) => it.status === "planned") ?? g.items[0];
}

/** The plan's calendar events as fixed blocks, cut to [from, to] so an all-day event does not stretch the timeline. */
export function eventBlocks(plan: wire.Plan | null, from: number, to: number): FixedBlock[] {
  return (plan?.events ?? [])
    .map((e) => ({ e, start: Math.max(e.start_at, from), end: Math.min(e.end_at, to) }))
    .filter(({ start, end }) => end > start)
    .map(({ e, start, end }, i) => ({ id: `event-${i}`, title: e.title || "Busy", start, minutes: (end - start) / 60_000 }));
}
