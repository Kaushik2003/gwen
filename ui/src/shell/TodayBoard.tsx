import { Bot, CircleAlert, Coffee, Play, Plus, ScrollText, Sparkles, SquareTerminal, Target, TriangleAlert } from "lucide-react";
import { useRef } from "react";
import { wire } from "../api";
import { useBriefing } from "../components/Briefing";
import { Button, cx, stateColor } from "../components/ui";
import { useDaemon, useTick } from "../daemon";
import { clockFace, formatDuration, formatLongDate, formatTime, instantOn } from "../format";
import { useNav } from "../nav";
import { AgendaCard, DayCard, GoalsCard, useToday } from "../screens/Today";
import { useTracking } from "../tracking";
import { Card, Eyebrow } from "./bits";
import { useTaskUI } from "../task/TaskUI";
import { useClearOfSheet } from "./HandOff";
import NeedsYou from "./NeedsYou";
import { useNext } from "./next";
import { projectChoices } from "./projects";
import { useTabs } from "./tabs";
import { useWork } from "./work";

// Today (plan.md 3.15, board 1), laid out by what matters most:
//   - the agenda, every open task by project with its steps and sessions, owns the right column from the top;
//   - your day, the timeline, runs the full width above both columns;
//   - on the left: what needs you, now and next as one compact card with Switch and Start in it, one slim
//     bar to add a task or a goal (each opens its full form as a popup), and goals.
// In one column (a narrow window) the agenda comes right after now and next.

export default function TodayBoard() {
  const d = useDaemon();
  const w = useWork();
  const { go } = useNav();
  const openBriefing = useBriefing();
  const data = useToday();
  // The hand-off panel opens over the right edge, where the agenda is; the board makes room for it.
  const root = useRef<HTMLDivElement>(null);
  const pad = useClearOfSheet(root, !!useTaskUI().handing);
  // The header reads to the minute. Only the Now and Next rows tick each second, so the timeline and the
  // agenda don't render again every second.
  useTick(15_000);
  const st = d.status;
  const today = d.today();
  const now = d.now();
  // Until the end of the work day, which after midnight is still yesterday's until the rollover.
  const left = instantOn(today, d.config?.planner.day_end ?? "23:00", d.config?.tracking.day_rollover ?? "04:00") - now;

  return (
    <div ref={root} className="flex flex-col gap-5" style={pad ? { paddingRight: pad } : undefined}>
      <header className="flex flex-wrap items-end gap-x-4 gap-y-3">
        <div className="min-w-64 flex-1">
          <h1 className="text-[22px] font-semibold tracking-[-0.01em]">{formatLongDate(today)}</h1>
          <p className="mt-1 text-[13px] text-ink-muted">
            {formatTime(now)}
            {left > 0 && ` · ${formatDuration(left)} left in your day`}
            {w.needs.length > 0 && ` · ${w.needs.length} thing${w.needs.length === 1 ? "" : "s"} need${w.needs.length === 1 ? "s" : ""} you`}
          </p>
        </div>
        <div className="flex gap-2">
          <Button icon={ScrollText} onClick={openBriefing}>
            Briefing
          </Button>
          <Button icon={Sparkles} onClick={() => go("plan", "chat")}>
            Plan the rest with Gwen
          </Button>
        </div>
      </header>

      {st?.warnings.map((x) => (
        <p key={x} className="-mt-1 flex items-center gap-2 rounded-lg border border-idle/25 bg-idle/8 px-3 py-1.5 text-xs text-idle">
          <TriangleAlert size={13} aria-hidden className="shrink-0" />
          {x}
        </p>
      ))}

      <DayCard data={data} />

      {/* One column: each part in its order. Two columns: the left stack keeps that order, the agenda takes the right. */}
      <div className="flex flex-col gap-5 @3xl:grid @3xl:grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)] @3xl:items-start">
        <div className="contents @3xl:flex @3xl:min-w-0 @3xl:flex-col @3xl:gap-5">
          <Card title="Needs you" icon={CircleAlert} count={w.needs.length} countTone="warning" bodyClassName="" className="order-0" meta={w.needs.length > 0 ? "Answer here, in the session, or on your phone" : undefined}>
            <NeedsYou limit={3} compact />
          </Card>
          <MomentCard plan={data.plan} className="order-1" />
          <AddBar className="order-3" />
          <GoalsCard goals={data.goals} className="order-5" />
        </div>
        <AgendaCard data={data} className="order-2" />
      </div>

    </div>
  );
}

/** Add a task or a goal: one slim bar; each opens the window's one form for it as a popup. */
function AddBar({ className }: { className?: string }) {
  const ui = useTaskUI();
  return (
    <section aria-label="Add" className={cx("lift flex min-w-0 items-center gap-1 rounded-xl border border-line bg-surface-1 p-1.5", className)}>
      <button
        type="button"
        onClick={() => ui.create()}
        className="group flex h-9 min-w-0 flex-1 items-center gap-2.5 rounded-lg px-2.5 text-left text-[13px] font-medium text-ink-muted hover:bg-surface-2 hover:text-ink"
      >
        <span className="grid size-5 shrink-0 place-items-center rounded-md bg-accent/15 text-accent-hover group-hover:bg-accent/25">
          <Plus size={13} aria-hidden />
        </span>
        <span className="shrink-0 whitespace-nowrap">New task</span>
        <span className="hidden min-w-0 truncate text-[12px] font-normal text-ink-faint @md:inline">Gwen fits it into your day</span>
      </button>
      <span className="h-5 w-px shrink-0 bg-line" aria-hidden />
      <button type="button" onClick={() => ui.createGoal()} className="flex h-9 shrink-0 items-center gap-2 rounded-lg px-2.5 text-[13px] font-medium whitespace-nowrap text-ink-muted hover:bg-surface-2 hover:text-ink">
        <Target size={14} className="text-ink-subtle" aria-hidden />
        New goal
      </button>
    </section>
  );
}

/** Now and next in one card: two compact rows, with switching and starting right in them. */
function MomentCard({ plan, className }: { plan: wire.Plan | null; className?: string }) {
  return (
    <section aria-label="Now and next" className={cx("lift min-w-0 overflow-hidden rounded-xl border border-line bg-surface-1", className)}>
      <NowRow plan={plan} />
      <NextRow />
    </section>
  );
}

/** Now: what your time goes to, how long, and Switch (or Clock in, or the session doing it with you). */
function NowRow({ plan }: { plan: wire.Plan | null }) {
  useTick(1000);
  const d = useDaemon();
  const w = useWork();
  const t = useTracking();
  const tabs = useTabs();
  const ui = useTaskUI();
  const st = d.status;
  if (!st) return <div className="h-[3.75rem]" />;
  const now = d.now();
  const project = d.projects.find((p) => p.id === st.project_id);
  const task = d.tasks.find((x) => x.id === st.task_id);
  const elapsed = st.open_segment ? now - st.open_segment.started_at : 0;
  const session = project ? (w.termsOf(project.id).find((x) => x.claude && (!task || x.task === task.id)) ?? w.termsOf(project.id).find((x) => x.claude)) : undefined;
  const item = task ? plan?.items.find((it) => it.task.id === task.id && it.status !== "skipped") : undefined;
  const planned = (item?.planned_minutes ?? 0) * 60_000;
  const tracked = task ? t.trackedNow(task) : 0;
  const off = st.state === "off";

  const title = off ? "Nothing is being tracked" : (task?.title ?? project?.name ?? "Unassigned");
  const sub = off
    ? "Clock in when you start."
    : t.onBreak
      ? "On a break"
      : [task && project?.name, session ? `with Claude in "${session.name}"` : null, planned > 0 ? `${formatDuration(tracked)} of ${formatDuration(planned)}` : null].filter(Boolean).join(" · ") ||
        "Pick what this time is for";

  return (
    <div className="relative flex flex-wrap items-center gap-x-3 gap-y-2 px-4 py-3">
      <span className={cx("size-2 shrink-0 rounded-full", st.state === "working" && "animate-live motion-reduce:animate-none")} style={{ background: off ? "var(--color-ink-faint)" : stateColor[st.state] }} />
      <div className="min-w-0 flex-1 basis-40">
        <div className="flex items-baseline gap-2">
          <Eyebrow className="shrink-0">Now</Eyebrow>
          <span className="truncate text-[14px] font-semibold text-ink">{title}</span>
        </div>
        <div className="truncate text-[12px] text-ink-subtle">{sub}</div>
      </div>
      {!off && <span className="font-mono text-[13px] text-ink tabular-nums">{clockFace(elapsed)}</span>}
      <div className="flex shrink-0 items-center gap-1.5">
        {off ? (
          <Button size="sm" tone="primary" icon={Play} onClick={() => t.clockIn(d.lastAttribution.project_id, d.lastAttribution.task_id)}>
            Clock in
          </Button>
        ) : (
          <>
            {t.onBreak && (
              <Button size="sm" icon={Coffee} onClick={() => t.breakEnd()}>
                Back to work
              </Button>
            )}
            {session && project && (
              <Button size="sm" icon={SquareTerminal} onClick={() => tabs.open({ kind: "term", project: project.id, terminal: session.address })}>
                Session
              </Button>
            )}
            {task && task.status !== "done" && session?.task !== task.id && (
              <Button size="sm" icon={Bot} onClick={() => ui.handOff(task)} title={`Hand “${task.title}” to Claude`}>
                Hand to Claude
              </Button>
            )}
            <SwitchTask current={task?.id} />
          </>
        )}
      </div>
      {planned > 0 && !off && (
        <span className="absolute inset-x-0 bottom-0 h-[2px] bg-surface-3" aria-hidden>
          <span className="block h-full bg-working" style={{ width: `${Math.min(100, (tracked / planned) * 100)}%` }} />
        </span>
      )}
    </div>
  );
}

/** Switch: every open task, grouped by project, in one compact picker; picking one starts it as Start does everywhere. */
function SwitchTask({ current }: { current?: string }) {
  const d = useDaemon();
  const ui = useTaskUI();
  const today = d.today();
  const choices = projectChoices(d.projects);
  // Today's and earlier, not copies of repeating tasks for later days.
  const roots = d.tasks.filter((x) => !x.parent_id && x.id !== current && !(x.occurrence_day && x.occurrence_day > today));
  const loose = roots.filter((x) => !x.project_id);
  return (
    <select
      aria-label="Switch to another task"
      value=""
      onChange={(e) => {
        const next = d.tasks.find((x) => x.id === e.target.value);
        if (next) ui.start(next);
      }}
      className="field !h-7 w-32 !text-[12.5px]"
    >
      <option value="">Switch…</option>
      {choices.map((c) => {
        const tasks = roots.filter((x) => x.project_id === c.id);
        return (
          tasks.length > 0 && (
            <optgroup key={c.id} label={c.label}>
              {tasks.slice(0, 30).map((x) => (
                <option key={x.id} value={x.id}>
                  {x.title}
                </option>
              ))}
            </optgroup>
          )
        );
      })}
      {loose.length > 0 && (
        <optgroup label="No project">
          {loose.slice(0, 30).map((x) => (
            <option key={x.id} value={x.id}>
              {x.title}
            </option>
          ))}
        </optgroup>
      )}
    </select>
  );
}

/** Next: the next block or event, when, and Start. */
function NextRow() {
  useTick(1000);
  const d = useDaemon();
  const t = useTracking();
  const tabs = useTabs();
  const next = useNext();
  const now = d.now();
  const task = next?.taskId ? d.tasks.find((x) => x.id === next.taskId) : undefined;
  const project = d.projects.find((p) => p.id === task?.project_id);
  const mins = next ? Math.round((next.start - now) / 60_000) : 0;

  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-2 border-t border-chrome-line px-4 py-2.5">
      <span className="size-2 shrink-0 rounded-full border border-line-3" />
      <div className="min-w-0 flex-1 basis-40">
        <div className="flex items-baseline gap-2">
          <Eyebrow className="shrink-0">Next</Eyebrow>
          <span className="truncate text-[13.5px] font-medium text-ink">{next ? next.title : "Nothing else planned today"}</span>
        </div>
        <div className="truncate text-[12px] text-ink-subtle">
          {next
            ? [`${formatTime(next.start)}, ${mins < 60 ? `in ${Math.max(mins, 0)} min` : `in ${formatDuration(next.start - now)}`}`, task?.estimate_minutes ? formatDuration(task.estimate_minutes * 60_000) : null, project?.name ?? (task ? null : "from your calendar")]
                .filter(Boolean)
                .join(" · ")
            : "Plan the rest of the day, or start something from the agenda."}
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-1.5">
        {task && !t.isTracking(task.id) && (
          <Button size="sm" tone="primary" icon={Play} onClick={() => t.startTask(task)}>
            Start now
          </Button>
        )}
        {!next && (
          <Button size="sm" onClick={() => tabs.open({ kind: "screen", screen: "plan", sub: "day" })}>
            Open your plan
          </Button>
        )}
      </div>
    </div>
  );
}
