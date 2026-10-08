import { BellOff, CalendarClock, CircleCheck, Coffee, ListTodo, Play, Plus, Radio, ScrollText, Sparkles, Square, Target, TriangleAlert } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { App, wire } from "../api";
import { useBriefing } from "../components/Briefing";
import { useCompleteTask } from "../components/complete";
import { GoalMeter, PaceChip, goalAmount } from "../components/goal";
import ProgressRing from "../components/ProgressRing";
import { StepList, stepsDone } from "../components/steps";
import TaskHover from "../components/TaskHover";
import { ProjectTag } from "../components/tags";
import Timeline, { type FixedBlock, type PlannedBlock } from "../components/Timeline";
import { Badge, Button, Callout, Checkbox, Dot, DotSelect, Empty, IconButton, Input, Panel, Select, cx, stateColor, stateLabel, unassignedColor } from "../components/ui";
import { useDaemon, useTick } from "../daemon";
import { clockFace, formatClock, formatDuration, formatLongDate, formatTime, instantOn, parseDuration } from "../format";
import { useNav } from "../nav";
import { blockTimes, eventBlocks, groupPlan } from "../plan";
import { clockOf, occursOn } from "../rrule";
import { useTracking } from "../tracking";
import { TaskForm } from "./Projects";

export default function Today() {
  const d = useDaemon();
  const t = useTracking();
  const openBriefing = useBriefing();
  const { go } = useNav();
  const done = useCompleteTask();
  useTick(1000);
  const st = d.status;
  const today = d.today();
  const [day, setDay] = useState<wire.DayDetail | null>(null);
  const [plan, setPlan] = useState<wire.Plan | null>(null);
  const [goals, setGoals] = useState<wire.Goal[]>([]);
  const [commitments, setCommitments] = useState<wire.Commitment[]>([]);
  // What Clock in starts with: the last project and task, until another is picked.
  const [pick, setPick] = useState(d.lastAttribution);
  const [todo, setTodo] = useState("");
  const addingTodo = useRef(false); // one to-do per Enter, however fast it is pressed
  const [adding, setAdding] = useState(false);

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

  if (!st) return null;
  const now = d.now();
  const state = st.state;
  const color = stateColor[state];
  const elapsed = st.open_segment ? now - st.open_segment.started_at : 0;
  const summary = st.today ?? day?.summary;
  const worked = (summary?.worked_ms ?? 0) + (t.working && st.today ? now - st.server_now_at : 0);
  const breakMs = (summary?.break_ms ?? 0) + (t.onBreak && st.today ? now - st.server_now_at : 0);
  const target = (summary?.target_seconds ?? 0) * 1000 || parseDuration(d.config?.tracking.daily_target ?? "8h");
  const met = target > 0 && worked >= target;

  const projectId = state === "off" ? pick.project_id : (st.project_id ?? null);
  const taskId = state === "off" ? pick.task_id : (st.task_id ?? null);
  const projectTasks = d.tasks.filter((x) => (!projectId || x.project_id === projectId || !x.project_id) && !(x.occurrence_day && x.occurrence_day > today));
  const chooseProject = (id: string | null) => (state === "off" ? setPick({ project_id: id, task_id: null }) : t.switchTo(id, null));
  const chooseTask = (id: string | null) => {
    const task = d.tasks.find((x) => x.id === id);
    const project = task?.project_id ?? projectId;
    if (state === "off") setPick({ project_id: project, task_id: id });
    else t.switchTo(project, id);
  };

  const rollover = d.config?.tracking.day_rollover ?? "04:00";
  const from = instantOn(today, d.config?.planner.day_start ?? "09:00", rollover);
  const to = instantOn(today, d.config?.planner.day_end ?? "23:00", rollover);
  const items = plan?.items ?? [];
  const groups = useMemo(() => groupPlan(plan?.items ?? []), [plan]);
  const planned: PlannedBlock[] = items
    .filter((it) => it.start_at != null && it.status !== "skipped")
    .map((it) => ({
      id: it.id,
      title: it.task.title,
      start: it.start_at!,
      minutes: it.planned_minutes,
      color: colors.get(it.task.project_id ?? null) ?? unassignedColor,
      done: it.status === "done",
    }));
  const fixed: FixedBlock[] = commitments
    .filter((c) => c.start_minute != null && occursOn(c, today))
    .map((c) => ({
      id: c.id,
      title: c.title,
      start: instantOn(today, clockOf(c.start_minute!), rollover),
      minutes: c.duration_minutes,
    }))
    .concat(eventBlocks(plan, from, to));

  const byProject = day?.summary.by_project ?? [];
  const byTotal = byProject.reduce((sum, p) => sum + p.worked_ms, 0);
  const shownGoals = [...goals].sort((a, b) => rank(a.progress.pace) - rank(b.progress.pace)).slice(0, 4);
  const plannedLeft = items.filter((it) => it.status === "planned").reduce((sum, it) => sum + it.planned_minutes, 0);
  // Quick to-dos: open tasks with no time to plan, for today or earlier, timed ones first.
  const quantityGoals = new Set(goals.filter((g) => g.kind === "quantity").map((g) => g.id));
  const todos = d.tasks
    .filter((x) => x.estimate_minutes == null && !x.parent_id && !(x.goal_id && quantityGoals.has(x.goal_id)))
    .filter((x) => (!x.start_day || x.start_day <= today) && (!x.occurrence_day || x.occurrence_day <= today))
    .sort((a, b) => (a.start_minute ?? 1440) - (b.start_minute ?? 1440));

  async function addTodo(e: React.FormEvent) {
    e.preventDefault();
    const title = todo.trim();
    if (!title || addingTodo.current) return;
    addingTodo.current = true;
    if (await d.act(() => App.CreateTask(wire.CreateTaskRequest.createFrom({ title, start_day: today })))) setTodo("");
    addingTodo.current = false;
  }

  return (
    // From two columns up the page fits the window: the panels scroll inside
    // themselves, so everything is in view at once.
    <div className="flex flex-col gap-4 @2xl:h-[calc(100dvh-3.5rem)] @2xl:min-h-[540px]">
      <header className="flex shrink-0 flex-wrap items-center justify-between gap-3">
        <h1 className="text-[22px] font-semibold tracking-[-0.02em] text-ink">{formatLongDate(today)}</h1>
        <div className="flex items-center gap-2">
          <span className="text-sm text-ink-subtle tabular-nums">{formatTime(now)}</span>
          <Button size="sm" icon={ScrollText} onClick={openBriefing}>
            Briefing
          </Button>
        </div>
      </header>

      {st.warnings.map((w) => (
        <p key={w} className="flex shrink-0 items-center gap-2 rounded-lg border border-idle/25 bg-idle/8 px-3 py-1.5 text-xs text-idle">
          <TriangleAlert size={13} aria-hidden className="shrink-0" />
          {w}
        </p>
      ))}

      <section className="lift shrink-0 overflow-hidden rounded-2xl border border-line bg-surface-1">
        <div className="grid items-center gap-x-8 gap-y-4 px-6 py-5 @2xl:grid-cols-[auto_minmax(0,1fr)]">
          <div className="flex min-w-0 items-center gap-5">
            <ProgressRing fraction={target > 0 ? worked / target : 0} color={met ? stateColor.working : color} size={84} stroke={7} label="Worked against the daily target">
              <span className="text-[17px] leading-none font-semibold tracking-[-0.02em] text-ink tabular-nums">{Math.round(target > 0 ? (worked / target) * 100 : 0)}%</span>
              <span className="mt-0.5 text-[10px] text-ink-subtle">of {formatDuration(target)}</span>
            </ProgressRing>
            <div className="min-w-0">
              <div className="flex items-center gap-2 text-[13px] font-medium whitespace-nowrap" style={{ color }}>
                <Dot color={color} live={state === "working"} />
                {stateLabel[state]}
                {state !== "off" && <span className="font-normal text-ink-subtle">{t.onBreak ? `for ${formatDuration(elapsed)}` : `since ${formatTime(st.state_since_at)}`}</span>}
              </div>
              {state === "off" ? (
                <div className="text-[30px] leading-tight font-semibold tracking-[-0.03em] whitespace-nowrap text-ink-subtle">Off the clock</div>
              ) : (
                <div
                  className={cx("text-[40px] leading-tight font-semibold tracking-[-0.04em] tabular-nums", t.onBreak ? "text-ink-subtle" : "text-ink")}
                  aria-label={`${formatDuration(worked)} worked today`}
                >
                  {clockFace(worked).slice(0, -3)}
                  <span className="text-ink-faint">{clockFace(worked).slice(-3)}</span>
                </div>
              )}
              <div className="flex items-center gap-2 text-xs whitespace-nowrap text-ink-subtle tabular-nums">
                <span>Break {formatDuration(breakMs)}</span>
                <span className="text-ink-faint">·</span>
                {met ? (
                  <span className="inline-flex items-center gap-1 text-working">
                    <CircleCheck size={12} aria-hidden />
                    Target met
                  </span>
                ) : (
                  <span>{formatDuration(target - worked)} to go</span>
                )}
                {st.snoozed_until_at != null && st.snoozed_until_at > now && (
                  <span className="inline-flex items-center gap-1" title={`Nudges snoozed for ${formatDuration(st.snoozed_until_at - now)}`}>
                    <BellOff size={12} aria-hidden />
                    {formatDuration(st.snoozed_until_at - now)}
                  </span>
                )}
              </div>
            </div>
          </div>

          <div className="flex min-w-0 flex-col gap-2.5">
            <div className="flex min-w-0 gap-2">
              <DotSelect color={colors.get(projectId) ?? unassignedColor} value={projectId ?? ""} onChange={(e) => chooseProject(e.target.value || null)} aria-label="Project" className="w-40 shrink-0">
                <option value="">Unassigned</option>
                {d.projects.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </DotSelect>
              <Select value={taskId ?? ""} onChange={(e) => chooseTask(e.target.value || null)} aria-label="Task" className="min-w-0 flex-1">
                <option value="">No task</option>
                {projectTasks.map((x) => (
                  <option key={x.id} value={x.id}>
                    {x.title}
                  </option>
                ))}
              </Select>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              {state === "off" && (
                <Button tone="primary" icon={Play} onClick={() => t.clockIn(pick.project_id, pick.task_id)}>
                  Clock in
                </Button>
              )}
              {t.onBreak && (
                <Button tone="primary" icon={Play} onClick={t.breakEnd}>
                  End break
                </Button>
              )}
              {(state === "working" || state === "break_auto") && (
                <Button icon={Coffee} iconColor={stateColor.break_manual} onClick={t.breakStart}>
                  {state === "break_auto" ? "Stay on break" : "Start break"}
                </Button>
              )}
              {state !== "off" && (
                <Button icon={Square} iconColor="var(--color-danger)" onClick={t.clockOut}>
                  Clock out
                </Button>
              )}
              {state !== "off" && <IconButton icon={BellOff} label="Snooze nudges" tone="secondary" onClick={t.snooze} />}
            </div>
          </div>
        </div>

        {state === "idle_pending" && st.idle_since_at && (
          <Callout
            tone="idle"
            className="mx-6 mb-4"
            action={
              <Button size="sm" icon={Coffee} onClick={t.breakStart}>
                Count it as a break
              </Button>
            }
          >
            No keyboard or mouse for {formatDuration(now - st.idle_since_at)}. It still counts as work until{" "}
            {d.config?.tracking.hard_idle ? formatDuration(parseDuration(d.config.tracking.hard_idle)) : "the limit"}, then becomes a break.
          </Callout>
        )}
        <div className="border-t border-line bg-canvas/40 px-6 py-3">
          <Timeline segments={day?.segments ?? []} colors={colors} names={names} now={now} live from={from} to={to} planned={planned} fixed={fixed} />
        </div>
      </section>

      <div className="grid min-h-0 flex-1 gap-4 @2xl:grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)] @6xl:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_minmax(0,0.85fr)]">
        <Panel
          title="Today's plan"
          icon={CalendarClock}
          className="flex min-h-64 flex-col"
          bodyClassName="min-h-0 flex-1 overflow-y-auto"
          actions={
            <>
              {items.length > 0 && <span className="text-xs text-ink-subtle tabular-nums">{formatDuration(plannedLeft * 60_000)} left</span>}
              <Button size="sm" tone="ghost" icon={Sparkles} iconColor="var(--color-accent-hover)" onClick={() => go("plan", "chat")}>
                Plan with AI
              </Button>
              {items.length > 0 && (
                <Button size="sm" tone="ghost" onClick={() => go("plan")}>
                  Open
                </Button>
              )}
            </>
          }
        >
          {plan && items.length === 0 ? (
            <Empty icon={CalendarClock} title="Nothing to plan yet">
              Tasks with a time estimate and your goals' sessions show up here by themselves, timed around your commitments.
            </Empty>
          ) : (
            <ul className="-mx-2 flex flex-col">
              {groups.map((g) => {
                const isDone = g.done;
                const tracking = t.isTracking(g.task.id);
                const project = d.projects.find((p) => p.id === g.task.project_id);
                return (
                  <li key={g.task.id} className={cx("rounded-lg", tracking && "bg-surface-2")}>
                    <TaskHover task={g.task} className="flex items-center gap-3 rounded-lg px-2 py-1.5 hover:bg-surface-2/60">
                      <Checkbox
                        checked={isDone}
                        disabled={g.skipped}
                        onChange={(on) => (on ? done.complete(g.task, g.steps.length > 0) : done.reopen(g.task))}
                        label={isDone ? `Reopen ${g.task.title}` : `Complete ${g.task.title}`}
                      />
                      <div className="min-w-0 flex-1">
                        <div className={cx("truncate text-sm", isDone ? "text-ink-faint line-through" : g.skipped ? "text-ink-faint" : "text-ink")}>{g.task.title}</div>
                        <div className="mt-0.5 flex min-w-0 items-center gap-3">
                          <ProjectTag project={project} />
                          {g.start != null && <span className="text-xs text-ink-subtle tabular-nums">{blockTimes(g)}</span>}
                          {g.steps.length > 0 && <span className="text-xs text-ink-subtle tabular-nums">{stepsDone(g.steps)}</span>}
                          {g.rollover > 0 && <Badge tone="idle">Carried over {g.rollover}×</Badge>}
                          {g.skipped && <Badge>Skipped</Badge>}
                          {!isDone && !g.skipped && g.start == null && <span className="text-xs text-idle">No room left today</span>}
                        </div>
                      </div>
                      <span className="text-xs text-ink-subtle tabular-nums">{formatDuration(g.minutes * 60_000)}</span>
                      {tracking ? (
                        <Badge tone="working" icon={Radio}>
                          Tracking
                        </Badge>
                      ) : (
                        !isDone && !g.skipped && <IconButton icon={Play} label={`Start ${g.task.title}`} tone="secondary" size="sm" onClick={() => t.startTask(g.task)} />
                      )}
                    </TaskHover>
                    {g.steps.length > 0 && <StepList parent={g.task} steps={g.steps} adding={tracking} className="mb-2" />}
                  </li>
                );
              })}
            </ul>
          )}
        </Panel>

        <div className="flex min-h-0 min-w-0 flex-col gap-4">
          <Panel
            title="To-do"
            icon={ListTodo}
            className="flex min-h-36 flex-[3] flex-col"
            bodyClassName="flex min-h-0 flex-1 flex-col"
            actions={
              <Button size="sm" tone="ghost" icon={Plus} onClick={() => setAdding(true)}>
                New task
              </Button>
            }
          >
            <form className="flex shrink-0 gap-2" onSubmit={addTodo}>
              <Input className="min-w-0 flex-1" placeholder="Add a quick to-do, then Enter" value={todo} onChange={(e) => setTodo(e.target.value)} aria-label="New to-do" />
              <IconButton icon={Plus} label="Add to-do" tone="secondary" type="submit" disabled={!todo.trim()} />
            </form>
            {todos.length > 0 ? (
              <ul className="-mx-2 mt-2 flex min-h-0 flex-1 flex-col overflow-y-auto">
                {todos.map((x) => {
                  const steps = d.tasks.filter((s) => s.parent_id === x.id);
                  return (
                    <li key={x.id}>
                      <TaskHover task={x} className="flex items-center gap-3 rounded-lg px-2 py-1.5 hover:bg-surface-2/60">
                        <Checkbox checked={false} onChange={() => done.complete(x, steps.length > 0)} label={`Complete ${x.title}`} />
                        <span className="min-w-0 flex-1 truncate text-sm text-ink">{x.title}</span>
                        {x.start_minute != null && x.start_day === today && <span className="text-xs text-ink-subtle tabular-nums">{formatClock(x.start_minute)}</span>}
                        {x.due_day && x.due_day <= today && <span className={cx("text-xs", x.due_day < today ? "text-danger" : "text-idle")}>{x.due_day < today ? "Late" : "Due today"}</span>}
                      </TaskHover>
                      {steps.length > 0 && <StepList parent={x} steps={steps} adding={false} className="mb-1.5" />}
                    </li>
                  );
                })}
              </ul>
            ) : (
              <p className="mt-3 text-[13px] text-ink-subtle">Nothing left to tick off.</p>
            )}
          </Panel>

          <Panel
            title="Goals"
            icon={Target}
            className="flex min-h-36 flex-[2] flex-col"
            bodyClassName="min-h-0 flex-1 overflow-y-auto"
            actions={
              goals.length > 0 && (
                <Button size="sm" tone="ghost" onClick={() => go("goals")}>
                  All
                </Button>
              )
            }
          >
            {goals.length === 0 ? (
              <Empty
                title="No goals yet"
                action={
                  <Button icon={Target} onClick={() => go("goals")}>
                    Set a goal
                  </Button>
                }
              >
                A goal such as 300 problems by December turns into daily sessions on your plan.
              </Empty>
            ) : (
              <ul className="flex flex-col gap-3.5">
                {shownGoals.map((g) => (
                  <li key={g.id} className="flex flex-col gap-1.5">
                    <div className="flex items-center gap-2">
                      <span className="min-w-0 flex-1 truncate text-sm text-ink">{g.title}</span>
                      <PaceChip pace={g.progress.pace} />
                    </div>
                    <GoalMeter g={g} today={today} />
                    <span className="text-xs text-ink-subtle tabular-nums">{goalAmount(g)}</span>
                  </li>
                ))}
              </ul>
            )}
          </Panel>
        </div>

        <Panel title="Where today went" className="hidden min-h-0 flex-col @6xl:flex" bodyClassName="min-h-0 flex-1 overflow-y-auto">
          {byTotal === 0 ? (
            <p className="text-[13px] text-ink-subtle">No work tracked yet today.</p>
          ) : (
            <>
              <div className="flex h-2 overflow-hidden rounded-full bg-surface-3">
                {byProject.map((p) => (
                  <span key={p.project_id ?? "none"} style={{ width: `${(p.worked_ms / byTotal) * 100}%`, backgroundColor: p.color }} />
                ))}
              </div>
              <ul className="mt-3.5 flex flex-col gap-2">
                {byProject.map((p) => (
                  <li key={p.project_id ?? "none"} className="flex items-center gap-2.5 text-[13px]">
                    <Dot color={p.color} />
                    <span className="min-w-0 flex-1 truncate text-ink-muted">{p.name}</span>
                    <span className="font-medium text-ink tabular-nums">{formatDuration(p.worked_ms)}</span>
                    <span className="w-9 text-right text-xs text-ink-subtle tabular-nums">{Math.round((p.worked_ms / byTotal) * 100)}%</span>
                  </li>
                ))}
              </ul>
            </>
          )}
        </Panel>
      </div>
      {adding && <TaskForm task={null} projects={d.projects} goals={goals} defaultProject="" onClose={() => setAdding(false)} />}
      {done.dialog}
    </div>
  );
}

/** Goals behind pace come first. */
function rank(pace: string): number {
  return pace === "behind" ? 0 : pace === "on_track" ? 1 : 2;
}
