import { CalendarClock, ChevronRight, ListChecks, Radio, Sparkles, SquareTerminal, Target } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { App, wire } from "../api";
import { GoalMeter, PaceChip, goalAmount } from "../components/goal";
import AgenticTimeline from "../timeline/AgenticTimeline";
import type { FixedBlock } from "../components/Timeline";
import { Badge, Button, IconButton, cx, unassignedColor } from "../components/ui";
import { useDaemon } from "../daemon";
import { formatDuration, formatTime, instantOn } from "../format";
import { useNav } from "../nav";
import { eventBlocks, groupPlan, type PlanGroup } from "../plan";
import { clockOf, occursOn } from "../rrule";
import { Card, ClaudeMark, Eyebrow, StateGlyph } from "../shell/bits";
import { projectTree } from "../shell/projects";
import { useTabs } from "../shell/tabs";
import { useTaskUI } from "../task/TaskUI";
import TaskRow from "../task/TaskRow";
import { useWork, type Term } from "../shell/work";
import { useTracking } from "../tracking";

// Today's plan, below now and next (plan.md 3.15, board 11): your day on a timeline with where the
// time went under it, the agenda, and goals. The board in shell/TodayBoard.tsx lays them out.

export interface TodayData {
  day: wire.DayDetail | null;
  plan: wire.Plan | null;
  goals: wire.Goal[];
  commitments: wire.Commitment[];
  colors: Map<string | null, string>;
  names: Map<string | null, string>;
  /** The planner's day, as instants. */
  from: number;
  to: number;
}

/** Everything Today's cards read, loaded once for the board. */
export function useToday(): TodayData {
  const d = useDaemon();
  const st = d.status;
  const today = d.today();
  const [day, setDay] = useState<wire.DayDetail | null>(null);
  const [plan, setPlan] = useState<wire.Plan | null>(null);
  const [goals, setGoals] = useState<wire.Goal[]>([]);
  const [commitments, setCommitments] = useState<wire.Commitment[]>([]);

  useEffect(() => {
    let live = true;
    App.GetDay(today).then(
      (x) => live && setDay(x),
      () => live && setDay(null), // no work day yet
    );
    return () => {
      live = false;
    };
  }, [today, d.daysVersion, st?.state]);
  useEffect(() => {
    let live = true;
    App.GetPlan(today).then(
      (p) => live && setPlan(p),
      () => live && setPlan(null),
    );
    return () => {
      live = false;
    };
  }, [today, d.planVersion, d.tasksVersion]);
  useEffect(() => {
    App.ListGoals("active").then(
      (l) => setGoals(l.goals),
      () => setGoals([]),
    );
  }, [d.goalsVersion, d.tasksVersion]);
  useEffect(() => {
    App.ListCommitments().then(
      (l) => setCommitments(l.commitments),
      () => setCommitments([]),
    );
  }, [d.goalsVersion]);
  const { colors, names } = useMemo(() => {
    const colors = new Map<string | null, string>([[null, unassignedColor]]);
    const names = new Map<string | null, string>([[null, "Unassigned"]]);
    d.projects.forEach((p) => (colors.set(p.id, p.color), names.set(p.id, p.name)));
    day?.summary.by_project.forEach((p) => (colors.set(p.project_id ?? null, p.color), names.set(p.project_id ?? null, p.name)));
    return { colors, names };
  }, [d.projects, day]);

  const rollover = d.config?.tracking.day_rollover ?? "04:00";
  const from = instantOn(today, d.config?.planner.day_start ?? "09:00", rollover);
  const to = instantOn(today, d.config?.planner.day_end ?? "23:00", rollover);
  return { day, plan, goals, commitments, colors, names, from, to };
}

const noSegments: wire.Segment[] = [];

/** The day's fixed blocks: commitments that fall today and calendar events. */
function fixedBlocks(data: TodayData, today: string, rollover: string): FixedBlock[] {
  return data.commitments
    .filter((c) => c.start_minute != null && occursOn(c, today))
    .map((c) => ({ id: c.id, title: c.title, start: instantOn(today, clockOf(c.start_minute!), rollover), minutes: c.duration_minutes }))
    .concat(eventBlocks(data.plan, data.from, data.to));
}

/** Your day: how it hangs together against the clock (the agentic timeline, plan.md 3.17); where the time went is the header's total and the legend under it. */
export function DayCard({ data, className }: { data: TodayData; className?: string }) {
  const d = useDaemon();
  const t = useTracking();
  const st = d.status;
  const today = d.today();
  const rollover = d.config?.tracking.day_rollover ?? "04:00";
  // Kept between renders, so the timeline works its bars out again only when the day's data changes.
  const fixed = useMemo(() => fixedBlocks(data, today, rollover), [data.commitments, data.plan, data.from, data.to, today, rollover]);
  const segments = data.day?.segments ?? noSegments;
  const live = st && st.state !== "off" && !t.onBreak;
  const byProject = data.day?.summary.by_project ?? [];
  const total = byProject.reduce((sum, p) => sum + p.worked_ms, 0);
  const breaks = data.day?.summary.break_ms ?? 0;
  const share = (ms: number) => (total > 0 ? Math.round((ms / total) * 100) : 0);

  return (
    <Card
      title="Your day"
      icon={CalendarClock}
      className={className}
      bodyClassName="px-4 pt-3 pb-3.5"
      meta={
        total > 0 ? (
          <>
            <span className="font-semibold text-ink">{formatDuration(total)}</span> tracked
          </>
        ) : (
          "Nothing tracked yet"
        )
      }
      actions={
        live && (
          <Badge tone="working" icon={Radio}>
            Live
          </Badge>
        )
      }
    >
      <AgenticTimeline segments={segments} colors={data.colors} names={data.names} fixed={fixed} />
      {(total > 0 || breaks > 0) && (
        <ul aria-label="Where today went" className="mt-3 flex flex-wrap gap-x-4 gap-y-1.5 text-[12px]">
          {byProject.map((p) => (
            <li key={p.project_id ?? "none"} className="flex min-w-0 items-center gap-1.5">
              <span className="size-2 shrink-0 rounded-[2px]" style={{ backgroundColor: p.color }} />
              <span className="truncate text-ink-muted">{p.name}</span>
              <span className="font-medium text-ink tabular-nums">{formatDuration(p.worked_ms)}</span>
              <span className="text-ink-subtle tabular-nums">{share(p.worked_ms)}%</span>
            </li>
          ))}
          {breaks > 0 && (
            <li className="flex items-center gap-1.5">
              <span className="hatched size-2 shrink-0 rounded-[2px]" />
              <span className="text-ink-muted">Break</span>
              <span className="font-medium text-ink tabular-nums">{formatDuration(breaks)}</span>
            </li>
          )}
        </ul>
      )}
    </Card>
  );
}

type AgendaRow = { kind: "task"; group: PlanGroup; start: number | null } | { kind: "event"; block: FixedBlock; start: number };
type AgendaView = "projects" | "time";
const viewKey = "gwen.agenda.view";
const foldKey = "gwen.agenda.folded";

function stored<T>(key: string, fallback: T): T {
  try {
    const v = localStorage.getItem(key);
    return v ? (JSON.parse(v) as T) : fallback;
  } catch {
    return fallback; // storage is a convenience
  }
}
function store(key: string, v: unknown) {
  try {
    localStorage.setItem(key, JSON.stringify(v));
  } catch {
    // storage is a convenience
  }
}

/**
 * The agenda, Today's main list: every open task by project and sub-project with its steps and the
 * Claude sessions working there, or today's plan in time order. Tick, start, or hand off from either.
 */
export function AgendaCard({ data, className }: { data: TodayData; className?: string }) {
  const d = useDaemon();
  const { go } = useNav();
  const [view, setView] = useState<AgendaView>(() => stored(viewKey, "projects"));
  const today = d.today();
  // Top-level open tasks, not counting copies of repeating tasks for later days (the list leaves those out too).
  const open = d.tasks.filter((x) => !x.parent_id && !(x.occurrence_day && x.occurrence_day > today)).length;
  const pick = (v: AgendaView) => {
    setView(v);
    store(viewKey, v);
  };

  return (
    <Card
      title="Agenda"
      icon={ListChecks}
      count={open}
      className={className}
      bodyClassName=""
      actions={
        <>
          <div role="tablist" aria-label="Agenda view" className="mr-1 flex rounded-md border border-line bg-surface-2 p-0.5">
            {(
              [
                ["projects", "By project"],
                ["time", "By time"],
              ] as const
            ).map(([v, label]) => (
              <button
                key={v}
                type="button"
                role="tab"
                aria-selected={view === v}
                onClick={() => pick(v)}
                className={cx("h-6 rounded-[5px] px-2 text-[12px] font-medium", view === v ? "bg-surface-4 text-ink" : "text-ink-subtle hover:text-ink")}
              >
                {label}
              </button>
            ))}
          </div>
          <IconButton size="sm" icon={Sparkles} label="Plan the day with Gwen" onClick={() => go("plan", "chat")} />
          <IconButton size="sm" icon={CalendarClock} label="Open the plan" onClick={() => go("plan")} />
        </>
      }
    >
      {view === "projects" ? <ProjectAgenda data={data} /> : <TimeAgenda data={data} />}
    </Card>
  );
}

/** By project: each project (and its sub-projects) with its live Claude sessions, then its open tasks, today's planned ones first. */
function ProjectAgenda({ data }: { data: TodayData }) {
  const d = useDaemon();
  const t = useTracking();
  const w = useWork();
  const tabs = useTabs();
  const [folded, setFolded] = useState<Record<string, boolean>>(() => stored(foldKey, {}));
  const planned = new Map(groupPlan(data.plan?.items ?? []).map((g) => [g.task.id, g]));
  const trackedOf = new Map((data.day?.summary.by_project ?? []).map((p) => [p.project_id ?? "", p.worked_ms]));
  const today = d.today();
  const { go } = useNav();
  const liveIds = new Set(d.projects.filter((p) => !p.archived_at).map((p) => p.id));
  // A quantity goal lines up its items as tasks; the ones not on today's plan fold into one line per project.
  const quantityGoals = new Map(data.goals.filter((g) => g.kind === "quantity").map((g) => [g.id, g]));
  const lined = (x: wire.Task) => !!x.goal_id && quantityGoals.has(x.goal_id) && !x.rrule && !planned.has(x.id) && !t.isTracking(x.id);
  // Top-level open tasks, without copies of repeating tasks for later days; a task in an archived project counts as having none.
  const all = d.tasks.filter((x) => !x.parent_id && !(x.occurrence_day && x.occurrence_day > today));
  const projectOf = (x: wire.Task) => (x.project_id && liveIds.has(x.project_id) ? x.project_id : null);
  const roots = all.filter((x) => !lined(x));
  const linedOf = (id: string | null) => {
    const byGoal = new Map<string, number>();
    for (const x of all) if (lined(x) && projectOf(x) === id) byGoal.set(x.goal_id!, (byGoal.get(x.goal_id!) ?? 0) + 1);
    return [...byGoal.entries()].map(([goal, n]) => ({ goal: quantityGoals.get(goal)!, n }));
  };
  const rank = (x: wire.Task) => {
    const g = planned.get(x.id);
    return [g?.start != null ? 0 : g ? 1 : 2, g?.start ?? 0, -x.priority, x.due_day ?? "9999-12-31", x.title] as const;
  };
  const byRank = (a: wire.Task, b: wire.Task) => {
    const [x, y] = [rank(a), rank(b)];
    for (let i = 0; i < x.length; i++) if (x[i] !== y[i]) return x[i] < y[i] ? -1 : 1;
    return 0;
  };
  const tasksOf = (id: string | null) => roots.filter((x) => projectOf(x) === id).sort(byRank);
  const sessionsOf = (id: string) => w.termsOf(id).filter((x) => x.claude);
  const fold = (id: string) => {
    const next = { ...folded, [id]: !folded[id] };
    setFolded(next);
    store(foldKey, next);
  };

  const live = d.projects.filter((p) => !p.archived_at);
  const tree = projectTree(live)
    .map((n) => ({ ...n, children: n.children.filter((c) => tasksOf(c.id).length + sessionsOf(c.id).length + linedOf(c.id).length > 0) }))
    .filter((n) => tasksOf(n.project.id).length + sessionsOf(n.project.id).length + linedOf(n.project.id).length + n.children.length > 0);
  const loose = tasksOf(null);

  const looseLined = linedOf(null);
  if (tree.length === 0 && loose.length === 0 && looseLined.length === 0)
    return (
      <div className="flex flex-col items-start gap-1 px-4 py-5">
        <p className="text-[13px] font-medium text-ink">Nothing open</p>
        <p className="max-w-md text-[12.5px] text-ink-subtle">Add a task in New task and it shows here under its project.</p>
      </div>
    );

  const taskRow = (x: wire.Task, depth: number, first: boolean) => <TaskRow key={x.id} task={x} plan={planned.get(x.id)} showProject={false} indent={depth === 0 ? 0 : 20} first={first} />;

  const sessionRow = (s: Term, depth: number) => (
    <li key={s.address} className={cx("flex items-center gap-2.5 border-t border-chrome-line py-1.5 pr-3 first:border-t-0", depth === 0 ? "pl-4" : "pl-9")}>
      <span className="grid w-[18px] shrink-0 place-items-center">
        <ClaudeMark size={12} />
      </span>
      <span className="min-w-0 truncate text-[13px] text-ink">{s.name}</span>
      <StateGlyph state={s.session?.state ?? s.status} unread={s.session?.unread} />
      <span className="min-w-0 flex-1 truncate text-[12px] text-ink-subtle">{s.session?.step || s.session?.last_message || ""}</span>
      <Button size="sm" tone="ghost" icon={SquareTerminal} onClick={() => tabs.open({ kind: "term", project: s.project ?? "", terminal: s.address })}>
        Open
      </Button>
    </li>
  );

  const header = (p: wire.Project, depth: number, count: number) => {
    const isFolded = !!folded[p.id];
    const tracked = trackedOf.get(p.id) ?? 0;
    const plannedMin = tasksOf(p.id).reduce((n, x) => n + (planned.get(x.id)?.minutes ?? 0), 0);
    return (
      <div className={cx("flex h-9 items-center gap-2 pr-3", depth === 0 ? "border-t border-chrome-line bg-surface-2/40 pl-2.5" : "pl-7")}>
        <button type="button" onClick={() => fold(p.id)} aria-expanded={!isFolded} aria-label={isFolded ? `Show ${p.name}` : `Fold ${p.name}`} className="grid size-5 place-items-center rounded text-ink-subtle hover:bg-surface-3 hover:text-ink">
          <ChevronRight size={13} className={cx("transition-transform", !isFolded && "rotate-90")} aria-hidden />
        </button>
        <span className="size-2.5 shrink-0 rounded-[3px]" style={{ backgroundColor: p.color }} />
        <button type="button" onClick={() => tabs.open({ kind: "project", project: p.id })} className={cx("min-w-0 truncate hover:underline", depth === 0 ? "text-[13px] font-semibold text-ink" : "text-[12.5px] font-medium text-ink-muted")}>
          {p.name}
        </button>
        <span className="shrink-0 text-[11.5px] text-ink-subtle tabular-nums">{count}</span>
        <span className="ml-auto flex shrink-0 items-center gap-3 text-[11.5px] text-ink-subtle tabular-nums">
          {plannedMin > 0 && <span>{formatDuration(plannedMin * 60_000)} today</span>}
          {tracked > 0 && <span className="text-ink-muted">{formatDuration(tracked)} tracked</span>}
        </span>
      </div>
    );
  };

  // A goal's items not on today's plan: one quiet line that opens the goal.
  const linedRow = (id: string | null, depth: number, first: boolean) =>
    linedOf(id).map(({ goal, n }, i) => (
      <li key={`lined-${goal.id}`} className={cx("flex items-center gap-2.5 py-1.5 pr-3 text-[12px] text-ink-subtle", (!first || i > 0) && "border-t border-chrome-line", depth === 0 ? "pl-4" : "pl-9")}>
        <Target size={13} className="ml-0.5 shrink-0" aria-hidden />
        <span className="min-w-0 flex-1 truncate">
          {n} more lined up for {goal.title}
        </span>
        <Button size="sm" tone="ghost" onClick={() => go("goals")}>
          Open goal
        </Button>
      </li>
    ));
  const block = (p: wire.Project, depth: number) => {
    const tasks = tasksOf(p.id);
    const sessions = sessionsOf(p.id);
    return {
      tasks,
      sessions,
      body: [...sessions.map((s) => sessionRow(s, depth)), ...tasks.map((x, i) => taskRow(x, depth, i === 0 && sessions.length === 0)), ...linedRow(p.id, depth, tasks.length + sessions.length === 0)],
    };
  };

  return (
    <div>
      {tree.map(({ project, children }, i) => {
        const own = block(project, 0);
        const all = own.tasks.length + children.reduce((n, c) => n + tasksOf(c.id).length, 0);
        return (
          <section key={project.id} aria-label={project.name} className={cx(i === 0 && "[&>div:first-child]:border-t-0")}>
            {header(project, 0, all)}
            {!folded[project.id] && (
              <>
                {own.body.length > 0 && <ul className="border-t border-chrome-line">{own.body}</ul>}
                {children.map((c) => {
                  const sub = block(c, 1);
                  return (
                    <section key={c.id} aria-label={c.name} className="border-t border-chrome-line">
                      {header(c, 1, sub.tasks.length)}
                      {!folded[c.id] && sub.body.length > 0 && <ul className="border-t border-chrome-line">{sub.body}</ul>}
                    </section>
                  );
                })}
              </>
            )}
          </section>
        );
      })}
      {loose.length + looseLined.length > 0 && (
        <section aria-label="No project">
          <div className={cx("flex h-9 items-center gap-2 bg-surface-2/40 pr-3 pl-2.5", tree.length > 0 && "border-t border-chrome-line")}>
            <button type="button" onClick={() => fold("")} aria-expanded={!folded[""]} aria-label={folded[""] ? "Show tasks with no project" : "Fold tasks with no project"} className="grid size-5 place-items-center rounded text-ink-subtle hover:bg-surface-3 hover:text-ink">
              <ChevronRight size={13} className={cx("transition-transform", !folded[""] && "rotate-90")} aria-hidden />
            </button>
            <span className="size-2.5 shrink-0 rounded-[3px] border border-line-3" />
            <span className="text-[13px] font-semibold text-ink-muted">No project</span>
            <span className="text-[11.5px] text-ink-subtle tabular-nums">{loose.length}</span>
          </div>
          {!folded[""] && (
            <ul className="border-t border-chrome-line">
              {loose.map((x, i) => taskRow(x, 0, i === 0))}
              {linedRow(null, 0, loose.length === 0)}
            </ul>
          )}
        </section>
      )}
    </div>
  );
}

/** By time: today's plan in time order with a line at now, the time down the left; calendar events are single quiet lines. */
function TimeAgenda({ data }: { data: TodayData }) {
  const d = useDaemon();
  const now = d.now();
  const today = d.today();
  const items = data.plan?.items ?? [];
  const fixed = fixedBlocks(data, today, d.config?.tracking.day_rollover ?? "04:00");
  const rows: AgendaRow[] = [
    ...groupPlan(items).map((group): AgendaRow => ({ kind: "task", group, start: group.start })),
    ...fixed.map((block): AgendaRow => ({ kind: "event", block, start: block.start })),
  ];
  const timed = rows.filter((r) => r.start != null).sort((a, b) => a.start! - b.start!);
  const untimed = rows.filter((r) => r.start == null);
  // Now sits before the first row that starts after it.
  const nowAt = timed.findIndex((r) => r.start! > now);
  const showNow = timed.length > 0 && timed[0].start! <= now;

  const row = (r: AgendaRow, i: number, marker: boolean) => {
    const line = cx(i > 0 && !marker && "border-t border-chrome-line");
    if (r.kind === "event") {
      const e = r.block;
      const past = e.start + e.minutes * 60_000 <= now;
      return (
        <li key={`event-${e.id}`} className={cx("flex items-center gap-3 px-4 py-2 text-ink-subtle", line, past && "opacity-60")}>
          <span className="w-[60px] shrink-0 text-[12px] whitespace-nowrap tabular-nums">{formatTime(e.start)}</span>
          <CalendarClock size={15} className="mx-px shrink-0" aria-label="Calendar" />
          <span className="min-w-0 flex-1 truncate text-[13px]">{e.title}</span>
          <span className="shrink-0 text-[12px] tabular-nums">{formatDuration(e.minutes * 60_000)}</span>
        </li>
      );
    }
    const g = r.group;
    return (
      <TaskRow
        key={g.task.id}
        task={g.task}
        plan={g}
        first={i === 0 || marker}
        lead={<span className="w-[60px] shrink-0 pt-px text-[12px] whitespace-nowrap text-ink-subtle tabular-nums">{g.start != null ? formatTime(g.start) : "—"}</span>}
      />
    );
  };

  if (data.plan && rows.length === 0)
    return (
      <div className="flex flex-col items-start gap-1 px-4 py-5">
        <p className="text-[13px] font-medium text-ink">Nothing on today yet</p>
        <p className="max-w-md text-[12.5px] text-ink-subtle">Tasks with a time estimate and your goals' sessions show up here by themselves, timed around your calendar.</p>
      </div>
    );
  return (
    <>
      <ol>
        {timed.map((r, i) => {
          const marker = showNow && i === (nowAt === -1 ? timed.length : nowAt);
          return [marker && <NowLine key="now" now={now} />, row(r, i, marker)];
        })}
        {showNow && nowAt === -1 && <NowLine now={now} />}
      </ol>
      {untimed.length > 0 && (
        <>
          <div className="flex items-baseline gap-2 border-t border-chrome-line bg-surface-2/30 px-4 py-2">
            <Eyebrow>No room left today</Eyebrow>
            <span className="text-[11.5px] text-ink-subtle tabular-nums">{untimed.length}</span>
          </div>
          <ol className="border-t border-chrome-line">{untimed.map((r, i) => row(r, i, false))}</ol>
        </>
      )}
    </>
  );
}

/** Now, as a line across the agenda between what is past and what is to come. */
function NowLine({ now }: { now: number }) {
  return (
    <li aria-label={`Now, ${formatTime(now)}`} className="flex items-center gap-3 px-4 py-1">
      <span className="w-[60px] shrink-0 text-[11px] font-semibold whitespace-nowrap text-accent-hover tabular-nums">{formatTime(now)}</span>
      <span className="size-1.5 shrink-0 rounded-full bg-accent" />
      <span className="h-px flex-1 bg-accent/50" />
    </li>
  );
}

/** Goals, the ones behind pace first: each with its meter and where it stands. */
export function GoalsCard({ goals, className }: { goals: wire.Goal[]; className?: string }) {
  const d = useDaemon();
  const { go } = useNav();
  const ui = useTaskUI();
  const today = d.today();
  const shown = [...goals].sort((a, b) => rank(a.progress.pace) - rank(b.progress.pace)).slice(0, 4);
  return (
    <Card
      title="Goals"
      icon={Target}
      count={goals.length}
      className={className}
      bodyClassName=""
      actions={
        goals.length > 0 && (
          <Button size="sm" tone="ghost" onClick={() => go("goals")}>
            All
          </Button>
        )
      }
    >
      {goals.length === 0 ? (
        <div className="flex flex-col items-start gap-2.5 px-4 py-3.5">
          <p className="text-[12.5px] text-ink-subtle">A goal such as 300 problems by December turns into daily sessions on your plan.</p>
          <Button size="sm" icon={Target} onClick={() => ui.createGoal()}>
            Set a goal
          </Button>
        </div>
      ) : (
        <ul>
          {shown.map((g, i) => (
            <li key={g.id} className={cx("flex flex-col gap-2 px-4 py-3", i > 0 && "border-t border-chrome-line")}>
              <div className="flex items-center gap-2">
                <span className="min-w-0 flex-1 truncate text-[13px] font-medium text-ink">{g.title}</span>
                <PaceChip pace={g.progress.pace} />
              </div>
              <GoalMeter g={g} today={today} height={5} />
              <span className="text-[11.5px] text-ink-subtle tabular-nums">{goalAmount(g)}</span>
            </li>
          ))}
          {goals.length > shown.length && (
            <li className="border-t border-chrome-line px-4 py-2 text-[12px] text-ink-subtle">
              {goals.length - shown.length} more in Goals
            </li>
          )}
        </ul>
      )}
    </Card>
  );
}

/** Goals behind pace come first. */
function rank(pace: string): number {
  return pace === "behind" ? 0 : pace === "on_track" ? 1 : 2;
}
