import { BellOff, CalendarClock, CircleCheck, Coffee, ListTodo, Play, Plus, Radio, RefreshCw, ScrollText, Sparkles, Square, Target, TriangleAlert } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { App, wire } from "../api";
import { useBriefing } from "../components/Briefing";
import { useCompleteTask } from "../components/complete";
import { GoalMeter, PaceChip, goalAmount } from "../components/goal";
import ProgressRing from "../components/ProgressRing";
import { StepList, stepsDone } from "../components/steps";
import { ProjectTag } from "../components/tags";
import Timeline, { type FixedBlock, type PlannedBlock } from "../components/Timeline";
import { Badge, Button, Callout, Checkbox, Dot, DotSelect, Empty, IconButton, Input, PageHeader, Panel, Select, cx, stateColor, stateLabel, unassignedColor } from "../components/ui";
import { useDaemon, useTick } from "../daemon";
import { clockFace, formatDuration, formatLongDate, formatTime, instantOn, parseDuration } from "../format";
import { useNav } from "../nav";
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
  const [planning, setPlanning] = useState(false);
  const [todo, setTodo] = useState("");
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
  const projectTasks = d.tasks.filter((x) => !projectId || x.project_id === projectId || !x.project_id);
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
    }));

  async function planDay() {
    setPlanning(true);
    await d.act(() => App.GeneratePlan(wire.GeneratePlanRequest.createFrom({ day: today })));
    setPlanning(false);
  }

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
    if (!title) return;
    if (await d.act(() => App.CreateTask(wire.CreateTaskRequest.createFrom({ title, start_day: today })))) setTodo("");
  }

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title={formatLongDate(today)}
        actions={
          <Button icon={ScrollText} onClick={openBriefing}>
            Briefing
          </Button>
        }
      />

      {st.warnings.map((w) => (
        <Callout key={w} tone="idle" icon={TriangleAlert}>
          {w}
        </Callout>
      ))}

      <section className="lift overflow-hidden rounded-2xl border border-line bg-surface-1">
        <div className="flex flex-col gap-8 p-6 md:p-7 @3xl:flex-row @3xl:items-center">
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2.5 text-sm font-medium" style={{ color }}>
              <Dot color={color} live={state === "working"} size={9} />
              {stateLabel[state]}
              {state !== "off" && <span className="font-normal text-ink-subtle">{t.onBreak ? `for ${formatDuration(elapsed)}` : `since ${formatTime(st.state_since_at)}`}</span>}
            </div>

            {state === "off" ? (
              <div className="mt-3 text-[44px] leading-tight font-semibold tracking-[-0.03em] text-ink-subtle">Off the clock</div>
            ) : (
              <div className={cx("mt-2 text-display font-semibold tabular-nums", t.onBreak ? "text-ink-subtle" : "text-ink")} aria-label={`${formatDuration(worked)} worked today`}>
                {clockFace(worked).slice(0, -3)}
                <span className="text-ink-faint">{clockFace(worked).slice(-3)}</span>
              </div>
            )}

            {state === "idle_pending" && st.idle_since_at && (
              <Callout
                tone="idle"
                className="mt-4"
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

            <div className="mt-5 flex flex-wrap items-center gap-2">
              <DotSelect color={colors.get(projectId) ?? unassignedColor} value={projectId ?? ""} onChange={(e) => chooseProject(e.target.value || null)} aria-label="Project" className="w-52">
                <option value="">Unassigned</option>
                {d.projects.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </DotSelect>
              <Select value={taskId ?? ""} onChange={(e) => chooseTask(e.target.value || null)} aria-label="Task" className="w-64 max-w-full">
                <option value="">No task</option>
                {projectTasks.map((x) => (
                  <option key={x.id} value={x.id}>
                    {x.title}
                  </option>
                ))}
              </Select>
            </div>

            <div className="mt-5 flex flex-wrap items-center gap-2">
              {state === "off" && (
                <Button tone="primary" size="lg" icon={Play} onClick={() => t.clockIn(pick.project_id, pick.task_id)}>
                  Clock in
                </Button>
              )}
              {t.onBreak && (
                <Button tone="primary" size="lg" icon={Play} onClick={t.breakEnd}>
                  End break
                </Button>
              )}
              {(state === "working" || state === "break_auto") && (
                <Button size="lg" icon={Coffee} iconColor={stateColor.break_manual} onClick={t.breakStart}>
                  {state === "break_auto" ? "Stay on break" : "Start break"}
                </Button>
              )}
              {state !== "off" && (
                <Button size="lg" icon={Square} iconColor="var(--color-danger)" onClick={t.clockOut}>
                  Clock out
                </Button>
              )}
              {state !== "off" && <IconButton icon={BellOff} label="Snooze nudges" tone="secondary" size="lg" onClick={t.snooze} />}
            </div>
            {st.snoozed_until_at != null && st.snoozed_until_at > now && (
              <p className="mt-3 flex items-center gap-1.5 text-xs text-ink-subtle">
                <BellOff size={12} aria-hidden />
                Nudges snoozed for {formatDuration(st.snoozed_until_at - now)}
              </p>
            )}
          </div>

          <div className="flex shrink-0 flex-col items-center gap-4 @3xl:pr-2">
            <ProgressRing fraction={target > 0 ? worked / target : 0} color={met ? stateColor.working : color} label="Worked against the daily target">
              <span className="text-[30px] leading-none font-semibold tracking-[-0.03em] text-ink tabular-nums">{Math.round(target > 0 ? (worked / target) * 100 : 0)}%</span>
              <span className="mt-1.5 text-xs text-ink-subtle">of {formatDuration(target)}</span>
            </ProgressRing>
            <dl className="grid grid-cols-2 gap-x-6 gap-y-0.5 text-center">
              <dt className="text-xs text-ink-subtle">Worked</dt>
              <dt className="text-xs text-ink-subtle">Break</dt>
              <dd className="text-sm font-semibold text-ink tabular-nums">{formatDuration(worked)}</dd>
              <dd className="text-sm font-semibold text-ink tabular-nums">{formatDuration(breakMs)}</dd>
            </dl>
            {met && (
              <Badge tone="working" icon={CircleCheck}>
                Target met
              </Badge>
            )}
          </div>
        </div>
        <div className="border-t border-line bg-canvas/40 px-6 py-4 md:px-7">
          <Timeline segments={day?.segments ?? []} colors={colors} names={names} now={now} live from={from} to={to} planned={planned} fixed={fixed} />
        </div>
      </section>

      <div className="grid gap-5 @3xl:grid-cols-[minmax(0,1.45fr)_minmax(0,1fr)]">
        <Panel
          title="Today's plan"
          icon={CalendarClock}
          actions={
            <>
              {items.length > 0 && <span className="text-xs text-ink-subtle tabular-nums">{formatDuration(plannedLeft * 60_000)} left</span>}
              <Button size="sm" tone="ghost" icon={Sparkles} iconColor="var(--color-accent-hover)" onClick={() => go("plan", "chat")}>
                Plan with AI
              </Button>
              {items.length > 0 && (
                <Button size="sm" tone="ghost" onClick={() => go("plan")}>
                  Open plan
                </Button>
              )}
            </>
          }
        >
          {plan && items.length === 0 ? (
            <Empty
              icon={CalendarClock}
              title="Nothing planned yet"
              action={
                <Button tone="primary" icon={RefreshCw} onClick={planDay} busy={planning}>
                  Plan my day
                </Button>
              }
            >
              Gwen fills the day from your goals and open tasks, around your commitments.
            </Empty>
          ) : (
            <ul className="-mx-2 flex flex-col">
              {items.map((it) => {
                const isDone = it.status === "done";
                const skipped = it.status === "skipped";
                const tracking = t.isTracking(it.task_id);
                const project = d.projects.find((p) => p.id === it.task.project_id);
                const steps = it.steps ?? [];
                return (
                  <li key={it.id} className={cx("rounded-lg", tracking && "bg-surface-2")}>
                    <div className="flex items-center gap-3 px-2 py-2">
                      <Checkbox
                        checked={isDone}
                        disabled={skipped}
                        onChange={(on) => (on ? done.complete(it.task, steps.length > 0) : done.reopen(it.task))}
                        label={isDone ? `Reopen ${it.task.title}` : `Complete ${it.task.title}`}
                      />
                      <div className="min-w-0 flex-1">
                        <div className={cx("truncate text-sm", isDone ? "text-ink-faint line-through" : skipped ? "text-ink-faint" : "text-ink")}>{it.task.title}</div>
                        <div className="mt-0.5 flex min-w-0 items-center gap-3">
                          <ProjectTag project={project} />
                          {it.start_at != null && <span className="text-xs text-ink-subtle tabular-nums">{formatTime(it.start_at)}</span>}
                          {steps.length > 0 && <span className="text-xs text-ink-subtle tabular-nums">{stepsDone(steps)}</span>}
                          {it.rollover_count > 0 && <Badge tone="idle">Carried over {it.rollover_count}×</Badge>}
                          {skipped && <Badge>Skipped</Badge>}
                        </div>
                      </div>
                      <span className="text-xs text-ink-subtle tabular-nums">{formatDuration(it.planned_minutes * 60_000)}</span>
                      {tracking ? (
                        <Badge tone="working" icon={Radio}>
                          Tracking
                        </Badge>
                      ) : (
                        !isDone && !skipped && <IconButton icon={Play} label={`Start ${it.task.title}`} tone="secondary" size="sm" onClick={() => t.startTask(it.task)} />
                      )}
                    </div>
                    {steps.length > 0 && <StepList parent={it.task} steps={steps} className="mb-2" />}
                  </li>
                );
              })}
            </ul>
          )}
        </Panel>

        <div className="flex min-w-0 flex-col gap-5">
          <Panel
            title="To-do"
            icon={ListTodo}
            actions={
              <Button size="sm" tone="ghost" icon={Plus} onClick={() => setAdding(true)}>
                New task
              </Button>
            }
          >
            <form className="flex gap-2" onSubmit={addTodo}>
              <Input className="min-w-0 flex-1" placeholder="Add a quick to-do, then Enter" value={todo} onChange={(e) => setTodo(e.target.value)} aria-label="New to-do" />
              <IconButton icon={Plus} label="Add to-do" tone="secondary" type="submit" disabled={!todo.trim()} />
            </form>
            {todos.length > 0 && (
              <ul className="-mx-2 mt-2 flex flex-col">
                {todos.map((x) => (
                  <li key={x.id} className="flex items-center gap-3 rounded-lg px-2 py-1.5 hover:bg-surface-2/60">
                    <Checkbox checked={false} onChange={() => done.complete(x, false)} label={`Complete ${x.title}`} />
                    <span className="min-w-0 flex-1 truncate text-sm text-ink">{x.title}</span>
                    {x.start_minute != null && x.start_day === today && <span className="text-xs text-ink-subtle tabular-nums">{clockOf(x.start_minute)}</span>}
                    {x.due_day && x.due_day <= today && <span className={cx("text-xs", x.due_day < today ? "text-danger" : "text-idle")}>{x.due_day < today ? "Late" : "Due today"}</span>}
                  </li>
                ))}
              </ul>
            )}
          </Panel>

          <Panel title="Where today went">
            {byTotal === 0 ? (
              <p className="text-[13px] text-ink-subtle">No work tracked yet today.</p>
            ) : (
              <>
                <div className="flex h-2.5 overflow-hidden rounded-full bg-surface-3">
                  {byProject.map((p) => (
                    <span
                      key={p.project_id ?? "none"}
                      title={`${p.name}: ${formatDuration(p.worked_ms)}`}
                      style={{
                        width: `${(p.worked_ms / byTotal) * 100}%`,
                        backgroundColor: p.color,
                      }}
                    />
                  ))}
                </div>
                <ul className="mt-4 flex flex-col gap-2.5">
                  {byProject.map((p) => (
                    <li key={p.project_id ?? "none"} className="flex items-center gap-2.5 text-sm">
                      <Dot color={p.color} />
                      <span className="min-w-0 flex-1 truncate text-ink-muted">{p.name}</span>
                      <span className="font-medium text-ink tabular-nums">{formatDuration(p.worked_ms)}</span>
                      <span className="w-10 text-right text-xs text-ink-subtle tabular-nums">{Math.round((p.worked_ms / byTotal) * 100)}%</span>
                    </li>
                  ))}
                </ul>
              </>
            )}
          </Panel>

          <Panel
            title="Goals"
            icon={Target}
            actions={
              goals.length > 0 && (
                <Button size="sm" tone="ghost" onClick={() => go("goals")}>
                  All goals
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
              <ul className="flex flex-col gap-4">
                {shownGoals.map((g) => (
                  <li key={g.id} className="flex flex-col gap-2">
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
