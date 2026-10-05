import { TrendingDown, TrendingUp, Minus } from "lucide-react";
import type { wire } from "../api";
import { daysBetween } from "../format";
import { Badge, Meter, clamp01 } from "./ui";

const pace: Record<string, { label: string; tone: "working" | "accent" | "idle"; color: string; icon: typeof Minus }> = {
  ahead: { label: "Ahead", tone: "working", color: "var(--color-working)", icon: TrendingUp },
  on_track: { label: "On track", tone: "accent", color: "var(--color-accent)", icon: Minus },
  behind: { label: "Behind", tone: "idle", color: "var(--color-idle)", icon: TrendingDown },
};

function paceOf(p: string) {
  return pace[p] ?? pace.on_track;
}

export function PaceChip({ pace: p }: { pace: string }) {
  const x = paceOf(p);
  return (
    <Badge tone={x.tone} icon={x.icon}>
      {x.label}
    </Badge>
  );
}

/** "12 / 300 problems" for a quantity goal, "3 / 8 tasks" for a tasks goal. */
export function goalAmount(g: wire.Goal): string {
  const p = g.progress;
  const unit = g.kind === "tasks" ? "tasks" : g.unit;
  return `${p.done_quantity} / ${p.done_quantity + p.remaining_quantity}${unit ? " " + unit : ""}`;
}

export function goalFraction(g: wire.Goal): number {
  const total = g.progress.done_quantity + g.progress.remaining_quantity;
  return total > 0 ? g.progress.done_quantity / total : 0;
}

/** How far through its days the goal is today: where an even pace would have it. */
export function evenPaceMark(g: wire.Goal, today: string): number | null {
  if (today < g.start_day || today > g.due_day) return null;
  const days = daysBetween(g.start_day, g.due_day) + 1;
  return clamp01((daysBetween(g.start_day, today) + 1) / days);
}

/** Done against total, in the pace colour, with a mark where an even pace would be today. */
export function GoalMeter({ g, today, height = 6 }: { g: wire.Goal; today: string; height?: number }) {
  return (
    <Meter
      value={goalFraction(g)}
      color={paceOf(g.progress.pace).color}
      marker={g.status === "active" ? evenPaceMark(g, today) : null}
      markerLabel="Where an even pace would be today"
      height={height}
      label={`${g.title}: ${goalAmount(g)}`}
    />
  );
}
