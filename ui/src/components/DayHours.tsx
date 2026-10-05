import { RotateCcw } from "lucide-react";
import { useEffect, useState } from "react";
import { App, wire } from "../api";
import { useDaemon } from "../daemon";
import { formatDuration, goDuration, minutesOf, parseDuration } from "../format";
import { clockOf } from "../rrule";
import DurationInput from "./DurationInput";
import TimeInput from "./TimeInput";
import { Button, Field } from "./ui";

/**
 * A day's own hours (docs/06-planner.md#day-hours): when its work starts and
 * how much time it gives tasks. Untouched, both show what the day has now;
 * a change is applied with one button, which replans the day.
 */
export default function DayHours({ plan, onPlan }: { plan: wire.Plan; onPlan: (p: wire.Plan) => void }) {
  const d = useDaemon();
  const hours = plan.hours ?? null;
  const savedStart = clockOf(plan.window.start_minute);
  const savedWork = goDuration((hours?.work_minutes ?? plan.capacity_minutes) * 60_000);
  const [start, setStart] = useState<string | null>(null); // edited, else null
  const [work, setWork] = useState<string | null>(null); // a Go duration, edited, else null
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    setStart(null);
    setWork(null);
  }, [plan.day, hours?.start_minute, hours?.work_minutes, plan.window.start_minute]);

  const changed = (start !== null && start !== savedStart) || (work !== null && parseDuration(work) !== parseDuration(savedWork));
  const usualStart = d.config?.planner.day_start ?? "09:00";
  const target = parseDuration(d.config?.tracking.daily_target ?? "8h");

  async function save(next: { start_minute: number | null; work_minutes: number | null }) {
    setBusy(true);
    const p = await d.act(() => App.SetDayHours(wire.SetDayHoursRequest.createFrom({ day: plan.day, ...next })));
    setBusy(false);
    if (p) onPlan(p);
  }
  const apply = () =>
    save({
      start_minute: start !== null ? minutesOf(start) : (hours?.start_minute ?? null),
      work_minutes: work !== null ? Math.round(parseDuration(work) / 60_000) : (hours?.work_minutes ?? null),
    });

  return (
    <div className="flex flex-wrap items-end gap-x-5 gap-y-3">
      <Field label="Start at">
        <TimeInput className="w-20" value={start ?? savedStart} onCommit={setStart} aria-label="Start at" title="When this day's work starts, 24-hour" />
      </Field>
      <Field label="Work for" compound>
        <DurationInput value={work ?? savedWork} onChange={setWork} label="Time for tasks" />
      </Field>
      {changed ? (
        <div className="flex items-center gap-2 pb-px">
          <Button tone="primary" onClick={apply} busy={busy}>
            Replan with these hours
          </Button>
          <Button
            tone="ghost"
            disabled={busy}
            onClick={() => {
              setStart(null);
              setWork(null);
            }}
          >
            Cancel
          </Button>
        </div>
      ) : (
        hours && (
          <Button tone="ghost" icon={RotateCcw} onClick={() => save({ start_minute: null, work_minutes: null })} busy={busy} className="mb-px">
            Usual hours
          </Button>
        )
      )}
      <p className="min-w-56 flex-1 self-center text-xs leading-relaxed text-ink-subtle">
        {hours
          ? `Set for this day. Usually from ${usualStart}, with the time worked out from your ${formatDuration(target)} target.`
          : `Usual hours: from ${usualStart}, with the time for tasks worked out from your ${formatDuration(target)} target and commitments. Change either for this day alone.`}
      </p>
    </div>
  );
}
