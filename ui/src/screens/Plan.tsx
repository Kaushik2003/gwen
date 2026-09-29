import { useEffect, useState } from "react";
import { App, readOnly, wire } from "../api";
import { Button, Card, Input } from "../components/ui";
import { useDaemon } from "../daemon";
import { addDays, formatDuration, formatTime, instantOn } from "../format";
import { clockOf, occursOn } from "../rrule";

const minutes = (m: number) => formatDuration(m * 60_000);

export default function Plan() {
  const d = useDaemon();
  const today = d.today();
  const [day, setDay] = useState(today);
  const [plan, setPlan] = useState<wire.Plan | null>(null);
  const [commitments, setCommitments] = useState<wire.Commitment[]>([]);
  const [dragging, setDragging] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    App.GetPlan(day).then((p) => live && setPlan(p), d.fail);
    return () => {
      live = false;
    };
  }, [day, d.planVersion, d.tasksVersion, d.fail]);
  useEffect(() => {
    App.ListCommitments().then((l) => setCommitments(l.commitments), d.fail);
  }, [d.goalsVersion, d.fail]);

  const past = day < today || readOnly; // nothing past, and nothing on the hub, can change
  const rollover = d.config?.tracking.day_rollover ?? "04:00";
  const onDay = commitments.filter((c) => occursOn(c, day));
  const patch = (it: wire.PlanItem, fields: Record<string, unknown>) =>
    d.act(() => App.PatchPlanItem(it.id, wire.PatchPlanItemRequest.createFrom({ ...fields, rev: it.rev })));

  const capacity = plan?.capacity_minutes ?? 0;
  const planned = plan?.planned_minutes ?? 0;
  const over = planned > capacity;

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center gap-2">
        <Button onClick={() => setDay(addDays(day, -1))}>←</Button>
        <Input type="date" value={day} onChange={(e) => e.target.value && setDay(e.target.value)} />
        <Button onClick={() => setDay(addDays(day, 1))}>→</Button>
        {day !== today && <Button onClick={() => setDay(today)}>Today</Button>}
        <div className="flex-1" />
        {!past && (
          <Button tone="primary" onClick={() => d.act(() => App.GeneratePlan(wire.GeneratePlanRequest.createFrom({ day })))}>
            Regenerate
          </Button>
        )}
      </div>

      <Card title="Capacity">
        <div className="mb-2 flex justify-between text-sm">
          <span>
            <span className="font-semibold">{minutes(planned)}</span> planned
          </span>
          <span className="text-zinc-500">{minutes(capacity)} available</span>
        </div>
        <div className="h-3 overflow-hidden rounded-full bg-zinc-200 dark:bg-zinc-800">
          <div
            className={`h-full rounded-full ${over ? "bg-amber-500" : "bg-emerald-500"}`}
            style={{ width: `${capacity > 0 ? Math.min(100, (planned / capacity) * 100) : planned > 0 ? 100 : 0}%` }}
          />
        </div>
        {over && <p className="mt-2 text-xs text-amber-600">More is planned than the day has room for.</p>}
      </Card>

      {onDay.length > 0 && (
        <Card title="Commitments">
          <ul className="flex flex-col gap-2">
            {onDay.map((c) => (
              <li key={c.id} className="flex items-center gap-3 rounded-md bg-zinc-100 px-3 py-2 text-sm dark:bg-zinc-800">
                <span className="w-28 tabular-nums text-zinc-500">
                  {c.start_minute == null ? "any time" : `${clockOf(c.start_minute)}–${clockOf(c.start_minute + c.duration_minutes)}`}
                </span>
                <span className="flex-1">{c.title}</span>
                <span className="text-xs text-zinc-500">{minutes(c.duration_minutes)}</span>
              </li>
            ))}
          </ul>
        </Card>
      )}

      <Card title="Plan">
        {plan && plan.items.length === 0 && (
          <p className="text-sm text-zinc-500">
            {day < today || readOnly ? "Nothing was planned." : "Nothing planned. Regenerate to fill the day."}
          </p>
        )}
        <ul className="flex flex-col gap-1">
          {plan?.items.map((it) => {
            const done = it.status === "done";
            const skipped = it.status === "skipped";
            return (
              <li
                key={it.id}
                draggable={!past}
                onDragStart={() => setDragging(it.id)}
                onDragEnd={() => setDragging(null)}
                onDragOver={(e) => dragging && dragging !== it.id && e.preventDefault()}
                onDrop={() => {
                  const from = plan.items.find((x) => x.id === dragging);
                  if (from) patch(from, { position: it.position });
                  setDragging(null);
                }}
                className={`flex items-center gap-3 rounded-md px-2 py-1.5 text-sm ring-1 ring-transparent ${
                  dragging === it.id ? "opacity-50" : ""
                } ${past ? "" : "cursor-grab hover:ring-zinc-200 dark:hover:ring-zinc-700"}`}
              >
                <Input
                  type="time"
                  className="w-24"
                  disabled={past}
                  key={`${it.id}-${it.start_at}`}
                  defaultValue={it.start_at != null ? formatTime(it.start_at) : ""}
                  onBlur={(e) => {
                    const v = e.target.value;
                    const at = v ? instantOn(day, v, rollover) : null;
                    if (at !== (it.start_at ?? null)) patch(it, { start_at: at });
                  }}
                />
                <span className={`flex-1 ${done ? "text-zinc-400 line-through" : skipped ? "text-zinc-400" : ""}`}>{it.task.title}</span>
                {it.rollover_count > 0 && (
                  <span className="rounded bg-amber-100 px-1.5 text-xs text-amber-800 dark:bg-amber-950 dark:text-amber-300">
                    rolled {it.rollover_count}×
                  </span>
                )}
                <Input
                  type="number"
                  min="1"
                  className="w-20 text-right"
                  disabled={past}
                  key={`${it.id}-m${it.planned_minutes}`}
                  defaultValue={it.planned_minutes}
                  onBlur={(e) => {
                    const m = Number(e.target.value);
                    if (m > 0 && m !== it.planned_minutes) patch(it, { planned_minutes: m });
                  }}
                />
                <span className="w-6 text-xs text-zinc-500">min</span>
                {!readOnly && (
                  <Button
                    disabled={past}
                    onClick={() => patch(it, { pinned: !it.pinned })}
                    className={it.pinned ? "text-emerald-700 dark:text-emerald-300" : ""}
                  >
                    {it.pinned ? "Pinned" : "Pin"}
                  </Button>
                )}
                {!done && !readOnly && (
                  <Button disabled={past} onClick={() => patch(it, { status: skipped ? "planned" : "skipped" })}>
                    {skipped ? "Unskip" : "Skip"}
                  </Button>
                )}
              </li>
            );
          })}
        </ul>
        {plan && plan.items.length > 1 && !past && (
          <p className="mt-3 text-xs text-zinc-500">Drag items to reorder. Moving, resizing, or reordering an item pins it.</p>
        )}
      </Card>
    </div>
  );
}
