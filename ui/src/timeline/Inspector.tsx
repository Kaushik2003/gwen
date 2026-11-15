import { ArrowDownRight, ArrowUpLeft, Bot, Check, Hand, Pencil, Play, Plus, Sparkles, SquareTerminal, X } from "lucide-react";
import { useState } from "react";
import { App, wire } from "../api";
import type { FixedBlock } from "../components/Timeline";
import { Badge, Button, IconButton, Select, cx, unassignedColor } from "../components/ui";
import { useDaemon } from "../daemon";
import { formatDuration, formatTime } from "../format";
import { useTabs } from "../shell/tabs";
import { useWork } from "../shell/work";
import { useTaskUI } from "../task/TaskUI";
import { useAskGwen } from "./ask";
import { agentStateLabel, chain, gwenAsk, metaOf, minutesWords, type ChainNode, type DayGraph } from "./graph";

// The panel under the agentic timeline: what the bar in focus is, why it is where it is, everything it waits on and
// everything waiting on it, and what you can do about it. With nothing in focus, the day in a few lines.

export type Picked =
  | { kind: "task"; id: string }
  | { kind: "agent"; id: string }
  | { kind: "tracked"; segment: wire.Segment }
  | { kind: "fixed"; block: FixedBlock };

const shell = "min-w-0 rounded-lg border border-line bg-surface-2/40 px-3 py-2.5";

export function Inspector({
  picked,
  graph,
  onShow,
  onClose,
  colors,
  names,
}: {
  picked: Picked;
  graph: DayGraph;
  onShow: (p: Picked) => void;
  onClose: () => void;
  colors: Map<string | null, string>;
  names: Map<string | null, string>;
}) {
  if (picked.kind === "task") {
    const t = graph.byId.get(picked.id);
    if (!t) return <DayOverview graph={graph} onShow={onShow} />;
    return <TaskPanel task={t} graph={graph} onShow={onShow} onClose={onClose} colors={colors} names={names} />;
  }
  if (picked.kind === "agent") {
    const a = graph.graph?.agents.find((x) => x.session_id === picked.id);
    if (!a) return <DayOverview graph={graph} onShow={onShow} />;
    return <AgentPanel lane={a} graph={graph} onShow={onShow} onClose={onClose} />;
  }
  if (picked.kind === "tracked") {
    const s = picked.segment;
    const end = s.ended_at ?? Date.now();
    return (
      <section aria-label="Tracked time" className={shell}>
        <Head title={s.kind === "work" ? (names.get(s.project_id ?? null) ?? "Unassigned") : "Break"} color={s.kind === "work" ? (colors.get(s.project_id ?? null) ?? unassignedColor) : "var(--color-break)"} onClose={onClose} />
        <p className="text-[12px] text-ink-subtle tabular-nums">
          Tracked {formatTime(s.started_at)} – {s.ended_at ? formatTime(s.ended_at) : "now"} · {formatDuration(end - s.started_at)}
        </p>
      </section>
    );
  }
  const f = picked.block;
  return (
    <section aria-label="Fixed time" className={shell}>
      <Head title={f.title} color="#8a8f98" onClose={onClose} />
      <p className="text-[12px] text-ink-subtle tabular-nums">
        Fixed {formatTime(f.start)} – {formatTime(f.start + f.minutes * 60_000)} · planned around
      </p>
    </section>
  );
}

function Head({ title, color, badge, onClose }: { title: string; color: string; badge?: React.ReactNode; onClose: () => void }) {
  return (
    <div className="mb-1 flex items-center gap-2">
      <span className="size-2 shrink-0 rounded-[3px]" style={{ background: color }} aria-hidden />
      <h3 className="min-w-0 flex-1 truncate text-[13px] font-semibold text-ink">{title}</h3>
      {badge}
      <IconButton icon={X} label="Close" size="sm" onClick={onClose} />
    </div>
  );
}

function TaskPanel({
  task: t,
  graph,
  onShow,
  onClose,
  colors,
  names,
}: {
  task: wire.GraphTask;
  graph: DayGraph;
  onShow: (p: Picked) => void;
  onClose: () => void;
  colors: Map<string | null, string>;
  names: Map<string | null, string>;
}) {
  const d = useDaemon();
  const ui = useTaskUI();
  const ask = useAskGwen();
  const meta = metaOf(t.state);
  const full = d.tasks.find((x) => x.id === t.id) ?? null;
  const after = chain(graph.byId, t.id, "after");
  const before = chain(graph.byId, t.id, "before");
  const critical = graph.graph?.critical.includes(t.id) ?? false;
  const project = names.get(t.project_id ?? null);

  const setWaits = (ids: string[]) => {
    const rev = full?.rev ?? t.rev;
    d.act(() => App.PatchTask(t.id, wire.PatchTaskRequest.createFrom({ waits_on: ids, rev }))).then((ok) => ok && graph.reload());
  };
  // What it can wait on: open tasks other than itself and what already waits on it (that would be a loop).
  const loops = new Set([t.id, ...before.map((n) => n.task.id)]);
  const choices = d.tasks.filter((x) => !x.parent_id && x.status === "open" && !x.rrule && !loops.has(x.id) && !t.waits_on.includes(x.id));
  const [adding, setAdding] = useState(false);

  const planned = t.planned_start != null && t.planned_end != null ? `${formatTime(t.planned_start)} – ${formatTime(t.planned_end)}` : null;
  const heading = t.projected_end != null && t.state !== "done" ? formatTime(t.projected_end) : null;
  const est = t.estimate_minutes ? t.estimate_minutes * 60_000 : 0;

  return (
    <section aria-label={t.title} className={shell}>
      <Head
        title={t.title}
        color={colors.get(t.project_id ?? null) ?? unassignedColor}
        onClose={onClose}
        badge={
          <Badge tone={meta.tone} className="shrink-0">
            {meta.label}
          </Badge>
        }
      />
      <p className="text-[12px] text-ink-subtle">
        {[project, planned && `planned ${planned}`, heading && (t.slip_minutes >= 10 ? `heading to end ${heading}, ${minutesWords(t.slip_minutes)} late` : `ends around ${heading}`)]
          .filter(Boolean)
          .join(" · ")}
      </p>
      <p className="text-[12px] text-ink-subtle tabular-nums">
        {t.tracked_ms > 0 ? `${formatDuration(t.tracked_ms)} tracked${est ? ` of ${formatDuration(est)}` : ""}` : `Not started${est ? ` · ${formatDuration(est)} estimated` : ""}`}
        {critical && <span className="text-ink-muted"> · on the day's longest chain</span>}
      </p>
      {t.reason && (
        <p className="mt-1.5 rounded-md px-2 py-1.5 text-[12px] leading-snug text-ink" style={{ background: `color-mix(in srgb, ${meta.color} 12%, transparent)` }}>
          {t.reason}
        </p>
      )}

      <div className="mt-2 grid gap-2 @lg:grid-cols-2">
        <ChainList
          title="Waits on"
          icon={ArrowUpLeft}
          nodes={after}
          empty="Nothing: it can start any time"
          onShow={(id) => onShow({ kind: "task", id })}
          onRemove={(id) => setWaits(t.waits_on.filter((w) => w !== id))}
          direct={t.waits_on}
        />
        <ChainList title="Holds up" icon={ArrowDownRight} nodes={before} empty="Nothing waits on it" onShow={(id) => onShow({ kind: "task", id })} />
      </div>

      {full && t.status === "open" && (
        <div className="mt-2">
          {adding ? (
            <div className="flex items-center gap-1.5">
              <Select
                autoFocus
                aria-label="A task it waits on"
                defaultValue=""
                className="h-7 min-w-0 flex-1 text-[12px]"
                onChange={(e) => {
                  if (e.target.value) setWaits([...t.waits_on, e.target.value]);
                  setAdding(false);
                }}
              >
                <option value="" disabled>
                  It waits on…
                </option>
                {choices.map((x) => (
                  <option key={x.id} value={x.id}>
                    {x.title}
                    {x.project_id ? ` · ${names.get(x.project_id) ?? ""}` : ""}
                  </option>
                ))}
              </Select>
              <IconButton icon={X} label="Cancel" size="sm" onClick={() => setAdding(false)} />
            </div>
          ) : (
            <button type="button" className="inline-flex items-center gap-1 text-[12px] text-ink-subtle hover:text-ink" onClick={() => setAdding(true)} disabled={!choices.length}>
              <Plus size={12} aria-hidden /> It waits on another task
            </button>
          )}
        </div>
      )}

      <div className="mt-2.5 flex flex-wrap items-center gap-1.5">
        {full && t.status === "open" && t.state !== "working" && (
          <Button
            size="sm"
            icon={Play}
            onClick={() => ui.start(full)}
            disabled={t.state === "blocked" || (t.state === "waiting_on_you" && !t.session_id)}
            title={t.state === "blocked" || (t.state === "waiting_on_you" && !t.session_id) ? "It waits on open work" : undefined}
          >
            Start
          </Button>
        )}
        {full && t.status === "open" && (
          <Button size="sm" icon={Check} onClick={() => ui.complete(full)}>
            Done
          </Button>
        )}
        {full && t.status === "open" && !t.session_id && (
          <Button size="sm" icon={Bot} onClick={() => ui.handOff(full)}>
            Hand to Claude
          </Button>
        )}
        {t.session_id && <OpenSession id={t.session_id} />}
        {full && (
          <Button size="sm" tone="ghost" icon={Pencil} onClick={() => ui.edit(full)}>
            Edit
          </Button>
        )}
        {t.status === "open" && (
          <Button size="sm" tone="ghost" icon={Sparkles} onClick={() => ask(gwenAsk(t))}>
            Ask Gwen
          </Button>
        )}
      </div>
    </section>
  );
}

function ChainList({
  title,
  icon: Icon,
  nodes,
  empty,
  onShow,
  onRemove,
  direct,
}: {
  title: string;
  icon: typeof ArrowUpLeft;
  nodes: ChainNode[];
  empty: string;
  onShow: (id: string) => void;
  onRemove?: (id: string) => void;
  direct?: string[];
}) {
  return (
    <div className="min-w-0">
      <div className="mb-0.5 flex items-center gap-1 text-[11px] font-medium text-ink-faint">
        <Icon size={11} aria-hidden />
        {title}
      </div>
      {nodes.length === 0 ? (
        <p className="text-[12px] text-ink-faint">{empty}</p>
      ) : (
        <ul className="flex flex-col">
          {nodes.map(({ task, depth }) => {
            const m = metaOf(task.state);
            return (
              <li key={task.id} className="group flex items-center gap-1" style={{ paddingLeft: depth * 12 }}>
                <button type="button" onClick={() => onShow(task.id)} className="flex min-w-0 flex-1 items-center gap-1.5 rounded px-1 py-0.5 text-left hover:bg-surface-3/60" title={task.reason || m.label}>
                  <span className="size-1.5 shrink-0 rounded-full" style={{ background: m.color }} aria-hidden />
                  <span className={cx("truncate text-[12px]", task.state === "done" ? "text-ink-faint line-through" : "text-ink-muted")}>{task.title}</span>
                  <span className="shrink-0 text-[11px] text-ink-faint">{m.label}</span>
                </button>
                {onRemove && depth === 0 && direct?.includes(task.id) && (
                  <IconButton icon={X} label={`It no longer waits on ${task.title}`} size="sm" className="opacity-0 group-hover:opacity-100 focus-visible:opacity-100" onClick={() => onRemove(task.id)} />
                )}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

function OpenSession({ id }: { id: string }) {
  const w = useWork();
  const tabs = useTabs();
  const s = w.sessions.find((x) => x.id === id);
  if (!s?.terminal) return null;
  const project = w.terminals.find((x) => x.address === s.terminal)?.project ?? "";
  return (
    <Button size="sm" icon={SquareTerminal} onClick={() => tabs.open({ kind: "term", project, terminal: s.terminal })}>
      {s.state === "needs_you" ? "Answer Claude" : "Open session"}
    </Button>
  );
}

function AgentPanel({ lane, graph, onShow, onClose }: { lane: wire.AgentLane; graph: DayGraph; onShow: (p: Picked) => void; onClose: () => void }) {
  const task = lane.task_id ? graph.byId.get(lane.task_id) : undefined;
  const sum = (state: string) => lane.spans.filter((s) => s.state === state).reduce((n, s) => n + ((s.end_at ?? Date.now()) - s.start_at), 0);
  const worked = sum("working") + sum("starting");
  const waited = sum("needs_you");
  const color = lane.state === "needs_you" ? "var(--color-warning)" : lane.state === "error" ? "var(--color-danger)" : "var(--color-info)";
  return (
    <section aria-label={`Claude in ${lane.name}`} className={shell}>
      <Head
        title={`Claude in ${lane.name}`}
        color={color}
        onClose={onClose}
        badge={
          <Badge tone={lane.state === "needs_you" ? "idle" : lane.state === "working" ? "accent" : "neutral"} className="shrink-0">
            {agentStateLabel[lane.state] ?? lane.state}
          </Badge>
        }
      />
      <p className="text-[12px] text-ink-subtle tabular-nums">
        Worked {formatDuration(worked)}
        {waited > 0 && ` · waited on you ${formatDuration(waited)}`}
      </p>
      {lane.question && (
        <p className="mt-1.5 flex items-start gap-1.5 rounded-md bg-warning/12 px-2 py-1.5 text-[12px] leading-snug text-ink">
          <Hand size={13} className="mt-px shrink-0 text-warning" aria-hidden />
          {lane.question}
        </p>
      )}
      {task && (
        <p className="mt-1.5 text-[12px] text-ink-subtle">
          On{" "}
          <button type="button" className="text-ink-muted hover:text-ink hover:underline" onClick={() => onShow({ kind: "task", id: task.id })}>
            {task.title}
          </button>
          {task.blocks.length > 0 && `, which ${task.blocks.length === 1 ? "1 task waits" : `${task.blocks.length} tasks wait`} on`}
        </p>
      )}
      <div className="mt-2.5 flex flex-wrap gap-1.5">
        <OpenSession id={lane.session_id} />
      </div>
    </section>
  );
}

/** The day in a few lines, with nothing in focus. */
export function DayOverview({ graph, onShow }: { graph: DayGraph; onShow: (p: Picked) => void }) {
  const g = graph.graph;
  const tasks = g?.tasks ?? [];
  const count = (s: string) => tasks.filter((t) => t.state === s).length;
  const working = g?.agents.filter((a) => a.state === "working" || a.state === "starting") ?? [];
  const waiting = g?.agents.filter((a) => a.state === "needs_you") ?? [];
  const planned = tasks.filter((t) => t.planned.length > 0);
  const last = planned.reduce<wire.GraphTask | null>((m, t) => (t.projected_end != null && t.status === "open" && (!m || t.projected_end > (m.projected_end ?? 0)) ? t : m), null);
  return (
    <section aria-label="Your day" className={shell}>
      <h3 className="mb-1.5 text-[11px] font-semibold tracking-wide text-ink-faint uppercase">How it hangs together</h3>
      {!g ? (
        <p className="text-[12px] text-ink-subtle">{graph.error || "Reading your day…"}</p>
      ) : (
        <ul className="flex flex-col gap-1 text-[12px] text-ink-muted">
          <li>
            {count("done")} of {planned.length} planned done
            {count("at_risk") > 0 && ` · ${count("at_risk")} running late`}
            {count("blocked") > 0 && ` · ${count("blocked")} blocked`}
          </li>
          {last?.projected_end != null && <li>Your planned work heads to end around {formatTime(last.projected_end)}.</li>}
          {working.length > 0 && <li>Claude is working in {working.map((a) => a.name).join(", ")}.</li>}
          {waiting.map((a) => (
            <li key={a.session_id}>
              <button type="button" className="text-left text-warning hover:underline" onClick={() => onShow({ kind: "agent", id: a.session_id })}>
                Claude in {a.name} is waiting for you
              </button>
            </li>
          ))}
          <li className="text-ink-faint">Hover a bar to read it here; click to keep it.</li>
        </ul>
      )}
    </section>
  );
}
