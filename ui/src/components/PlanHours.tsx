import { RotateCcw } from "lucide-react";
import { useEffect, useState } from "react";
import { App, wire } from "../api";
import { useDaemon } from "../daemon";
import { formatClock, formatDuration, formatHHMM, minutesOf, parseDuration } from "../format";
import { clockOf } from "../rrule";
import TimeInput from "./TimeInput";
import { Button } from "./ui";

const minutes = (m: number) => formatDuration(m * 60_000);

/**
 * The day's hours in one line, as Plan › Day shows them above its timeline: "Day hours 9:00 am to 11:00 pm · keep 30m
 * free". A day with hours of its own says how long it gives tasks; the usual hours keep the planner's buffer free.
 */
export function hoursLine(plan: wire.Plan, buffer: string): string {
  const work = plan.hours?.work_minutes;
  const keep = parseDuration(buffer);
  const tail = work != null ? (work === 0 ? "no time for tasks" : `${minutes(work)} for tasks`) : keep > 0 ? `keep ${formatDuration(keep)} free` : "";
  return [`Day hours ${formatClock(plan.window.start_minute)} to ${formatClock(plan.window.end_minute)}`, tail, plan.hours && "set for this day"].filter(Boolean).join(" · ");
}

/**
 * A day's own hours (docs/06-planner.md#day-hours), opened from Change hours: when its work starts and how much time
 * it gives tasks. Untouched, both show what the day has now; a change is applied with one button, which replans the day.
 */
export default function PlanHours({ plan, onPlan, onClose }: { plan: wire.Plan; onPlan: (p: wire.Plan) => void; onClose: () => void }) {
  const d = useDaemon();
  const hours = plan.hours ?? null;
  const savedStart = clockOf(plan.window.start_minute);
  const savedWork = hours?.work_minutes ?? plan.capacity_minutes;
  const [start, setStart] = useState<string | null>(null); // edited, else null
  const [h, setH] = useState<string | null>(null); // the hours and minutes typed, else null
  const [m, setM] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    setStart(null);
    setH(null);
    setM(null);
  }, [plan.day, hours?.start_minute, hours?.work_minutes, plan.window.start_minute]);

  const shownH = h ?? String(Math.floor(savedWork / 60));
  const shownM = m ?? String(savedWork % 60);
  const workEdited = h !== null || m !== null;
  const work = (Number(shownH) || 0) * 60 + (Number(shownM) || 0);
  const changed = (start !== null && start !== savedStart) || (workEdited && work !== savedWork);
  const usualStart = formatHHMM(d.config?.planner.day_start ?? "09:00");
  const target = formatDuration(parseDuration(d.config?.tracking.daily_target ?? "8h"));

  async function save(next: { start_minute: number | null; work_minutes: number | null }) {
    setBusy(true);
    const p = await d.act(() => App.SetDayHours(wire.SetDayHoursRequest.createFrom({ day: plan.day, ...next })));
    setBusy(false);
    if (p) {
      onPlan(p);
      onClose();
    }
  }
  const apply = () =>
    save({
      start_minute: start !== null ? minutesOf(start) : (hours?.start_minute ?? null),
      work_minutes: workEdited ? work : (hours?.work_minutes ?? null),
    });
  const reset = () => {
    setStart(null);
    setH(null);
    setM(null);
  };

  return (
    <div className="mt-3 rounded-lg border border-line bg-canvas/40 px-3 py-2.5">
      <div className="flex flex-wrap items-center gap-x-5 gap-y-2">
        <label className="flex items-center gap-2 text-[12px] text-ink-muted">
          Start at
          <TimeInput
            className="!h-7 w-[84px] !border-line !bg-canvas !px-1.5 font-mono !text-[12px]"
            value={start ?? savedStart}
            onCommit={setStart}
            aria-label="Start at"
            title="When this day's work starts. Up and down arrows step it by 15 minutes."
          />
        </label>
        <div role="group" aria-label="Time for tasks" className="flex items-center gap-1.5 text-[12px] text-ink-muted">
          <span className="mr-0.5">Work for</span>
          <Amount value={shownH} onChange={setH} label="Time for tasks, hours" unit="h" />
          <Amount value={shownM} onChange={setM} label="Time for tasks, minutes" unit="m" />
        </div>
        <div className="ml-auto flex items-center gap-1.5">
          {changed ? (
            <>
              <Button size="sm" tone="ghost" disabled={busy} onClick={reset}>
                Cancel
              </Button>
              <Button size="sm" tone="primary" onClick={apply} busy={busy}>
                Replan with these hours
              </Button>
            </>
          ) : (
            hours && (
              <Button size="sm" tone="ghost" icon={RotateCcw} onClick={() => save({ start_minute: null, work_minutes: null })} busy={busy}>
                Usual hours
              </Button>
            )
          )}
        </div>
      </div>
      <p className="mt-2 text-[11.5px] leading-relaxed text-ink-subtle">
        {hours
          ? `Set for this day. Usually from ${usualStart}, with the time for tasks worked out from your ${target} target.`
          : `Usually from ${usualStart}, with the time for tasks worked out from your ${target} target and commitments. Change either for this day alone.`}
      </p>
    </div>
  );
}

/** A small number box with its unit after it, for hours or minutes. */
function Amount({ value, onChange, label, unit }: { value: string; onChange: (v: string) => void; label: string; unit: string }) {
  return (
    <span className="inline-flex items-center gap-1">
      <input
        className="field !h-7 w-11 !border-line !bg-canvas !px-1.5 text-right font-mono !text-[12px] tabular-nums"
        inputMode="numeric"
        aria-label={label}
        value={value}
        onChange={(e) => onChange(e.target.value.replace(/[^\d]/g, "").slice(0, 3))}
      />
      <span className="text-ink-subtle">{unit}</span>
    </span>
  );
}
