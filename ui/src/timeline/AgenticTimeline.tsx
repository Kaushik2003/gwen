import { Sparkles } from "lucide-react";
import { useMemo, useState } from "react";
import type { wire } from "../api";
import { Timeline as LiquidTimeline, type TimelineTask } from "../components/liquid-charts/liquid/charts/time";
import type { ChartConfig } from "../components/liquid-charts/liquid/core";
import type { FixedBlock } from "../components/Timeline";
import { Button, Segmented, cx, unassignedColor } from "../components/ui";
import { useDaemon } from "../daemon";
import { formatTime } from "../format";
import { useAskGwen } from "./ask";
import { agentStateLabel, gwenAsk, metaOf, useDayGraph } from "./graph";
import { DayOverview, Inspector, type Picked } from "./Inspector";

// Today against the clock, as one picture of how the day hangs together (plan.md 3.17): your plan with what waits on
// what drawn as arrows, what you tracked, your fixed time, and each Claude session's day, in liquid glass. A planned
// block fills as you work on it; one pushed later by what it waits on moves, and one running long shows where its plan
// ended. Hover a bar to read it below, click to keep it there; the panel says why it is where it is, follows its whole
// chain both ways, and changes it: what it waits on, start, done, hand to Claude, or ask Gwen to sort it out.

const hour = 3_600_000;
const kindLabel: Record<string, string> = { work: "Work", break_manual: "Break", break_auto: "Automatic break" };
const mergeToleranceMs = 1_000;
const timeZone = Intl.DateTimeFormat().resolvedOptions().timeZone;

type Zoom = "day" | "now";
const zoomKey = "gwen.timeline.zoom";

/** What a bar on the chart stands for. */
type Ref =
  | { key: string; kind: "task"; id: string }
  | { key: string; kind: "agent"; id: string }
  | { key: string; kind: "tracked"; segment: wire.Segment }
  | { key: string; kind: "fixed"; block: FixedBlock };

const stateSeries: Record<string, { label: string; color: string }> = {
  blocked: { label: "Blocked", color: "var(--color-danger)" },
  waiting_on_you: { label: "Waiting on you", color: "var(--color-warning)" },
  agent_working: { label: "Claude working", color: "var(--color-info)" },
  agent_needs: { label: "Claude needs you", color: "var(--color-warning)" },
  agent_error: { label: "Claude stopped", color: "var(--color-danger)" },
  break: { label: "Break", color: "var(--color-break)" },
  fixed: { label: "Fixed time", color: "#8a8f98" },
};

export default function AgenticTimeline({
  segments,
  colors,
  names,
  fixed,
}: {
  segments: wire.Segment[];
  colors: Map<string | null, string>;
  names: Map<string | null, string>;
  fixed: FixedBlock[];
}) {
  const d = useDaemon();
  const day = d.today();
  const g = useDayGraph(day);
  const ask = useAskGwen();
  const [zoom, setZoomState] = useState<Zoom>(() => {
    try {
      return localStorage.getItem(zoomKey) === "now" ? "now" : "day";
    } catch {
      return "day";
    }
  });
  const setZoom = (z: Zoom) => {
    setZoomState(z);
    try {
      localStorage.setItem(zoomKey, z);
    } catch {
      // storage is a convenience
    }
  };
  const [activeKey, setActiveKey] = useState<string | null>(null);
  const [picked, setPicked] = useState<Picked | null>(null);

  const now = d.now();
  const minute = Math.floor(now / 60_000) * 60_000;

  const { tasks, refs, config, window } = useMemo(() => {
    const config: ChartConfig = {};
    for (const [key, s] of Object.entries(stateSeries)) config[key] = { label: s.label, colors: { dark: [s.color], light: [s.color] } };
    const projectSeries = (projectId: string | null | undefined) => {
      const key = `project:${projectId ?? "none"}`;
      const color = colors.get(projectId ?? null) ?? unassignedColor;
      config[key] ??= { label: names.get(projectId ?? null) ?? "Unassigned", colors: { dark: [color], light: [color] } };
      return key;
    };
    const tasks: TimelineTask[] = [];
    const refs: Ref[] = [];
    const push = (t: TimelineTask, r: Ref) => {
      tasks.push(t);
      refs.push(r);
    };
    const graph = g.graph;

    // Your plan: each block of each task, filled by what you tracked on it, after what it waits on.
    const lastBlock = new Map<string, string>();
    const firstBlock = new Map<string, string>();
    for (const t of graph?.tasks ?? []) {
      const timed = t.planned.filter((b) => b.start_at != null);
      if (!timed.length) continue;
      firstBlock.set(t.id, `blk:${timed[0].item_id}`);
      lastBlock.set(t.id, `blk:${timed[timed.length - 1].item_id}`);
    }
    for (const t of graph?.tasks ?? []) {
      const timed = t.planned.filter((b) => b.start_at != null);
      if (!timed.length) continue;
      // Pushed later by what it waits on (or its time came and went): its blocks move with it. One you're on now is
      // drawn from when you started it, early or late.
      const off = t.state !== "done" && t.projected_start != null && t.planned_start != null ? t.projected_start - t.planned_start : 0;
      const moved = t.state === "working" ? off : Math.max(0, off);
      let left = t.tracked_ms;
      const series = t.state === "blocked" ? "blocked" : t.state === "waiting_on_you" ? "waiting_on_you" : projectSeries(t.project_id);
      timed.forEach((b, i) => {
        const ms = b.minutes * 60_000;
        const fill = b.status === "done" ? 1 : Math.max(0, Math.min(1, left / ms));
        left -= ms;
        const shift = b.status === "done" ? 0 : moved;
        const start = b.start_at! + shift;
        let end = start + ms;
        let plannedEnd: number | undefined;
        if (i === timed.length - 1 && t.projected_end != null && t.projected_end > end && (t.state === "working" || t.state === "at_risk")) {
          if (shift === 0) plannedEnd = end; // ran long: the plan's end engraved on the glass
          end = t.projected_end;
        }
        const after = i === 0 ? t.waits_on.map((w) => lastBlock.get(w)).filter((x): x is string => !!x) : [`blk:${timed[i - 1].item_id}`];
        push(
          {
            id: `blk:${b.item_id}`,
            name: t.title,
            lane: "Plan",
            group: "You",
            series,
            start,
            end,
            progress: t.state === "done" ? 1 : fill,
            plannedEnd,
            after,
            note: t.reason || metaOf(t.state).label,
          },
          { key: `task:${t.id}:${b.item_id}`, kind: "task", id: t.id },
        );
      });
    }

    for (const f of fixed) {
      push(
        { id: `fixed:${f.id}`, name: f.title, lane: "Busy", group: "You", series: "fixed", start: f.start, end: f.start + f.minutes * 60_000, note: "Fixed time" },
        { key: `fixed:${f.id}`, kind: "fixed", block: f },
      );
    }

    // What you tracked: one bar per unbroken run of a project.
    let run: { task: TimelineTask; project: string | null; end: number } | null = null;
    for (const s of segments) {
      const end = s.ended_at ?? Math.max(minute, s.started_at + 60_000);
      if (s.kind !== "work") {
        run = null;
        push(
          { id: `seg:${s.id}`, name: kindLabel[s.kind] ?? s.kind, lane: "Tracked", group: "You", series: "break", start: s.started_at, end },
          { key: `seg:${s.id}`, kind: "tracked", segment: s },
        );
        continue;
      }
      const project = s.project_id ?? null;
      if (run && run.project === project && Math.abs(s.started_at - run.end) <= mergeToleranceMs) {
        run.task.end = end;
        run.end = end;
        continue;
      }
      const task: TimelineTask = { id: `seg:${s.id}`, name: names.get(project) ?? "Unassigned", lane: "Tracked", group: "You", series: projectSeries(project), start: s.started_at, end };
      push(task, { key: `seg:${s.id}`, kind: "tracked", segment: s });
      run = { task, project, end };
    }

    // Each Claude session: when it worked and when it waited on you.
    const lanesSeen = new Map<string, number>();
    for (const a of graph?.agents ?? []) {
      const n = (lanesSeen.get(a.name) ?? 0) + 1;
      lanesSeen.set(a.name, n);
      const lane = n === 1 ? a.name : `${a.name} ${n}`;
      const task = a.task_id ? g.byId.get(a.task_id) : undefined;
      a.spans.forEach((sp, i) => {
        const series = sp.state === "working" || sp.state === "starting" ? "agent_working" : sp.state === "needs_you" ? "agent_needs" : sp.state === "error" ? "agent_error" : null;
        if (!series) return;
        const end = sp.end_at ?? minute;
        push(
          {
            id: `agent:${a.session_id}:${i}`,
            name: sp.state === "needs_you" ? "Needs you" : (task?.title ?? agentStateLabel[sp.state] ?? sp.state),
            lane,
            group: "Agents",
            series,
            start: sp.start_at,
            end: Math.max(end, sp.start_at + 60_000),
            note: sp.state === "needs_you" && sp.end_at == null && a.question ? a.question : task ? `On “${task.title}”` : undefined,
          },
          { key: `agent:${a.session_id}:${i}`, kind: "agent", id: a.session_id },
        );
      });
    }

    // The day's work and now, to the half hour with a little air, so the bars get the room; or the hours around now.
    const half = hour / 2;
    const span = (lo: number, hi: number) => [Math.floor(lo / half) * half, Math.ceil(hi / half) * half] as const;
    let window: readonly [number, number];
    if (zoom === "now") {
      window = span(minute - 2 * hour, minute + 3 * hour);
    } else {
      const starts = [minute, ...tasks.map((t) => t.start as number)];
      const ends = [minute + hour, ...tasks.map((t) => (t.end ?? t.start) as number)];
      const [lo, hi] = span(Math.min(...starts) - 20 * 60_000, Math.max(...ends) + 20 * 60_000);
      window = [lo, Math.max(lo + 2 * hour, hi)];
    }
    return { tasks, refs, config, window };
  }, [g.graph, g.byId, segments, fixed, colors, names, minute, zoom]);

  const activeIndex = activeKey === null ? null : refs.findIndex((r) => r.key === activeKey);
  const activeRef = activeIndex !== null && activeIndex >= 0 ? refs[activeIndex] : null;
  const focus: Picked | null = activeRef
    ? activeRef.kind === "task"
      ? { kind: "task", id: activeRef.id }
      : activeRef.kind === "agent"
        ? { kind: "agent", id: activeRef.id }
        : activeRef.kind === "tracked"
          ? { kind: "tracked", segment: activeRef.segment }
          : { kind: "fixed", block: activeRef.block }
    : picked;

  /** Shows a task (or a session) in the panel, and lights it on the chart when it has a bar there. */
  const show = (p: Picked | null) => {
    setPicked(p);
    if (p && (p.kind === "task" || p.kind === "agent")) {
      const r = refs.find((x) => x.kind === p.kind && x.id === p.id);
      setActiveKey(r?.key ?? null);
    } else setActiveKey(null);
  };

  const issues = g.graph?.issues ?? [];
  const late = g.graph?.tasks.filter((t) => t.state === "at_risk").length ?? 0;

  return (
    <div className="@container">
      <div className="mb-2 flex flex-wrap items-center gap-2">
        <Segmented<Zoom>
          size="sm"
          label="Zoom"
          value={zoom}
          onChange={setZoom}
          options={[
            { value: "day", label: "Day" },
            { value: "now", label: "Around now" },
          ]}
        />
        <span className="min-w-0 flex-1 truncate text-[12px] text-ink-subtle">
          {g.error
            ? g.error
            : issues.length
              ? `${issues.length} ${issues.length === 1 ? "thing needs" : "things need"} a look${late ? ` · ${late} running late` : ""}`
              : g.graph
                ? "Nothing in your way"
                : ""}
        </span>
        <Button size="sm" icon={Sparkles} onClick={() => ask(gwenAsk())} title="Gwen reads how your day hangs together and fixes what's in the way">
          {issues.length ? "Sort out my day" : "Ask Gwen"}
        </Button>
      </div>
      <LiquidTimeline
        tasks={tasks}
        config={config}
        window={window}
        unit="hours"
        today={minute}
        todayLabel={formatTime(now)}
        timeZone={timeZone}
        theme="dark"
        tooltip={false}
        active={activeIndex !== null && activeIndex >= 0 ? activeIndex : null}
        onActiveChange={(i) => {
          setActiveKey(i === null ? null : (refs[i]?.key ?? null));
          if (i !== null) setPicked(null);
        }}
        empty="Nothing planned, tracked, or with Claude yet"
        label="Today against the clock: your plan with what waits on what, your tracked time, your fixed time, and each Claude session. Use Left and Right to move between bars, Up and Down to change row; the panel below reads the bar in focus."
        describe={(i) => {
          const r = refs[i];
          if (r?.kind === "task") {
            const t = g.byId.get(r.id);
            return t ? `${t.title}: ${metaOf(t.state).label}. ${t.reason}` : "";
          }
          const t = tasks[i];
          return `${t.name}, ${formatTime(t.start as number)} to ${formatTime((t.end ?? t.start) as number)}`;
        }}
      />
      <div className={cx("mt-3 grid gap-3", "@3xl:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]")}>
        {focus ? (
          <Inspector picked={focus} graph={g} onShow={show} onClose={() => show(null)} colors={colors} names={names} />
        ) : (
          <DayOverview graph={g} onShow={show} />
        )}
        <IssueList issues={issues} graph={g.graph} onShow={show} />
      </div>
    </div>
  );
}

function IssueList({ issues, graph, onShow }: { issues: wire.GraphIssue[]; graph: wire.DayGraph | null; onShow: (p: Picked) => void }) {
  const [all, setAll] = useState(false);
  const shown = all ? issues : issues.slice(0, 4);
  const critical = graph?.critical ?? [];
  const title = (id: string) => graph?.tasks.find((t) => t.id === id)?.title ?? "";
  return (
    <section aria-label="In your way" className="min-w-0 rounded-lg border border-line bg-surface-2/40 px-3 py-2.5">
      <h3 className="mb-1.5 text-[11px] font-semibold tracking-wide text-ink-faint uppercase">In your way</h3>
      {issues.length === 0 ? (
        <p className="text-[12px] text-ink-subtle">Nothing blocked or running late.</p>
      ) : (
        <ul className="flex flex-col gap-1">
          {shown.map((is) => (
            <li key={`${is.kind}:${is.task_id}`}>
              <button type="button" onClick={() => onShow({ kind: "task", id: is.task_id })} className="flex w-full items-start gap-2 rounded-md px-1.5 py-1 text-left hover:bg-surface-3/60">
                <span className="mt-1.5 size-1.5 shrink-0 rounded-full" style={{ background: metaOf(is.kind).color }} aria-hidden />
                <span className="min-w-0 text-[12px] leading-snug text-ink-muted">{is.text}</span>
              </button>
            </li>
          ))}
          {issues.length > 4 && (
            <li>
              <button type="button" onClick={() => setAll(!all)} className="px-1.5 text-[12px] text-ink-subtle hover:text-ink">
                {all ? "Fewer" : `${issues.length - 4} more`}
              </button>
            </li>
          )}
        </ul>
      )}
      {critical.length >= 2 && (
        <p className="mt-2 border-t border-line pt-2 text-[11px] leading-snug text-ink-subtle">
          <span className="font-medium text-ink-muted">Longest chain: </span>
          {critical.map((id, i) => (
            <span key={id}>
              {i > 0 && " → "}
              <button type="button" className="hover:text-ink hover:underline" onClick={() => onShow({ kind: "task", id })}>
                {title(id)}
              </button>
            </span>
          ))}
          {graph && (() => {
            const last = graph.tasks.find((t) => t.id === critical[critical.length - 1]);
            return last?.projected_end ? ` · done around ${formatTime(last.projected_end)}` : "";
          })()}
        </p>
      )}
    </section>
  );
}

