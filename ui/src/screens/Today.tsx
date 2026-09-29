import { useEffect, useMemo, useState } from "react";
import { App, wire } from "../api";
import ProgressRing from "../components/ProgressRing";
import Timeline from "../components/Timeline";
import { useBriefing } from "../components/Briefing";
import { Button, Card, Select, stateColor, stateLabel } from "../components/ui";
import { useDaemon, useTick } from "../daemon";
import { formatDuration } from "../format";

export default function Today() {
  const d = useDaemon();
  const openBriefing = useBriefing();
  useTick(1000);
  const st = d.status;
  const [day, setDay] = useState<wire.DayDetail | null>(null);

  const todayDay = d.today();
  useEffect(() => {
    let live = true;
    App.GetDay(todayDay).then(
      (x) => live && setDay(x),
      () => live && setDay(null), // no work day yet
    );
    return () => {
      live = false;
    };
  }, [todayDay, d.daysVersion, st?.state]);

  const colors = useMemo(() => {
    const m = new Map<string | null, string>([[null, "#6b7280"]]);
    d.projects.forEach((p) => m.set(p.id, p.color));
    day?.summary.by_project.forEach((p) => m.set(p.project_id ?? null, p.color));
    return m;
  }, [d.projects, day]);

  if (!st) return null;
  const now = d.now();
  const state = st.state;
  const counting = state === "working" || state === "idle_pending";
  const elapsed = st.open_segment ? now - st.open_segment.started_at : 0;
  const worked = (st.today?.worked_ms ?? 0) + (counting ? now - st.server_now_at : 0);
  const target = (st.today?.target_seconds ?? 0) * 1000;
  const color = stateColor[state];
  const projectTasks = d.tasks.filter((t) => !st.project_id || t.project_id === st.project_id || t.project_id === null);

  const clockIn = () => d.act(() => App.ClockIn(wire.ClockInRequest.createFrom({ project_id: st.project_id, task_id: st.task_id })));
  const switchTo = (project: string | null, task: string | null) =>
    d.act(() => App.Switch(wire.SwitchRequest.createFrom({ project_id: project, task_id: task })));

  return (
    <div className="flex flex-col gap-4">
      {st.warnings.map((w) => (
        <div key={w} className="rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-900 ring-1 ring-amber-200 dark:bg-amber-950 dark:text-amber-200 dark:ring-amber-900">
          {w}
        </div>
      ))}
      <div className="grid grid-cols-[1fr_auto] gap-4">
        <Card>
          <div className="text-sm font-medium" style={{ color }}>
            {stateLabel[state]}
          </div>
          <div className="my-2 font-mono text-6xl font-semibold tabular-nums" style={{ color }}>
            {state === "off" ? "—" : clockFace(elapsed)}
          </div>
          {state === "idle_pending" && st.idle_since_at && (
            <p className="text-sm text-amber-600">No input for {formatDuration(now - st.idle_since_at)}</p>
          )}
          <div className="mt-3 flex flex-wrap items-center gap-2">
            <Select value={st.project_id ?? ""} onChange={(e) => switchTo(e.target.value || null, null)} disabled={state === "off"}>
              <option value="">Unassigned</option>
              {d.projects.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </Select>
            <Select value={st.task_id ?? ""} onChange={(e) => switchTo(st.project_id ?? null, e.target.value || null)} disabled={state === "off"}>
              <option value="">No task</option>
              {projectTasks.map((t) => (
                <option key={t.id} value={t.id}>
                  {t.title}
                </option>
              ))}
            </Select>
          </div>
          <div className="mt-4 flex flex-wrap gap-2">
            {state === "off" && (
              <Button tone="primary" onClick={clockIn}>
                Clock in
              </Button>
            )}
            {(state === "working" || state === "idle_pending" || state === "break_auto") && (
              <Button onClick={() => d.act(() => App.BreakStart())}>Start break</Button>
            )}
            {(state === "break_auto" || state === "break_manual") && (
              <Button tone="primary" onClick={() => d.act(() => App.BreakEnd())}>
                End break
              </Button>
            )}
            {state !== "off" && <Button onClick={() => d.act(() => App.Snooze())}>Snooze nudges</Button>}
            {state !== "off" && (
              <Button tone="danger" onClick={() => d.act(() => App.ClockOut())}>
                Clock out
              </Button>
            )}
            <Button onClick={openBriefing}>Briefing</Button>
          </div>
          {st.snoozed_until_at && st.snoozed_until_at > now && (
            <p className="mt-2 text-xs text-zinc-500">Nudges snoozed for {formatDuration(st.snoozed_until_at - now)}</p>
          )}
        </Card>
        <Card className="flex flex-col items-center justify-center">
          <ProgressRing fraction={target > 0 ? worked / target : 0} color="#10b981" label={`of ${formatDuration(target)}`} />
          <p className="mt-2 text-sm">
            <span className="font-semibold">{formatDuration(worked)}</span> worked ·{" "}
            {formatDuration((st.today?.break_ms ?? 0) + (state.startsWith("break") ? now - st.server_now_at : 0))} break
          </p>
        </Card>
      </div>
      <Card title="Timeline">
        <Timeline segments={day?.segments ?? []} colors={colors} now={now} />
      </Card>
      <Card title="By project">
        {(day?.summary.by_project ?? []).length === 0 && <p className="text-sm text-zinc-500">No work yet.</p>}
        <ul className="flex flex-col gap-2">
          {day?.summary.by_project.map((p) => (
            <li key={p.project_id ?? "none"} className="flex items-center gap-2 text-sm">
              <span className="inline-block h-3 w-3 rounded-sm" style={{ backgroundColor: p.color }} />
              <span className="flex-1">{p.name}</span>
              <span className="tabular-nums">{formatDuration(p.worked_ms)}</span>
            </li>
          ))}
        </ul>
      </Card>
    </div>
  );
}

/** "3:12:05" for the big timer. */
function clockFace(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  return `${h}:${String(m).padStart(2, "0")}:${String(s % 60).padStart(2, "0")}`;
}
