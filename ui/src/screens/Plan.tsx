import { CalendarDays, ChevronLeft, ChevronRight, CircleAlert, GripVertical, ListChecks, LoaderCircle, Pin, RefreshCw, Repeat, Sparkles, type LucideIcon } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type DragEvent, type ReactNode } from "react";
import { App, apiError, isHub, readOnly, wire } from "../api";
import PlanChat from "../components/PlanChat";
import PlanHours, { hoursLine } from "../components/PlanHours";
import TimeInput from "../components/TimeInput";
import Timeline, { type FixedBlock, type PlannedBlock } from "../components/Timeline";
import { Badge, Button, Callout, Checkbox, IconButton, Input, Meter, cx, unassignedColor } from "../components/ui";
import { useDaemon } from "../daemon";
import { addDays, clockValue, formatClock, formatDuration, formatLongDate, formatTime, instantOn, minutesOf, relativeDay } from "../format";
import type { ProposedBlock } from "../llm";
import { useNav } from "../nav";
import { eventBlocks, groupPlan, leadItem, type PlanGroup } from "../plan";
import { clockOf, occursOn } from "../rrule";
import { Card, Scroll, Swatch } from "../shell/bits";
import { useClearOfSheet } from "../shell/HandOff";
import { projectPath } from "../shell/projects";
import TaskRow from "../task/TaskRow";
import { useTaskUI } from "../task/TaskUI";
import { useTracking } from "../tracking";

// Plan › Day (plan.md 3.15, board 8): the day's numbers, hours, calendar and timeline in one card, the day's tasks in
// the order Gwen times them, and Plan with Gwen beside them, whose proposal shows on the plan lane until it is applied.

/** "1h 30m", or "0m" for none, where formatDuration would say "0s". */
const minutes = (m: number) => (m > 0 ? formatDuration(m * 60_000) : "0m");

export default function Plan() {
  const d = useDaemon();
  const t = useTracking();
  const today = d.today();
  const { param } = useNav();
  const name = d.config?.llm.assistant_name || "Gwen";
  const [day, setDay] = useState(today);
  const [chat, setChat] = useState(param === "chat");
  const [preview, setPreview] = useState<ProposedBlock[] | null>(null);
  const [plan, setPlan] = useState<wire.Plan | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [reload, setReload] = useState(0);
  const [commitments, setCommitments] = useState<wire.Commitment[]>([]);
  const [goals, setGoals] = useState<Map<string, string>>(new Map());
  const [tracked, setTracked] = useState<wire.Segment[]>([]);
  // The day's totals by project name the ones no longer in the list (archived or deleted) on the timeline.
  const [trackedBy, setTrackedBy] = useState<wire.ProjectTotal[]>([]);
  // Dragging moves a task: these are task ids, and "end" is the drop zone after the last row.
  const [dragging, setDragging] = useState<string | null>(null);
  const [target, setTarget] = useState<string | null>(null);
  const [generating, setGenerating] = useState(false);
  const [cleared, setCleared] = useState(0);
  const [editHours, setEditHours] = useState(false);
  const [unfolded, setUnfolded] = useState<Record<string, boolean>>({});
  const shownDay = useRef<string | null>(null);
  shownDay.current = plan?.day ?? null;

  useEffect(() => {
    let live = true;
    App.GetPlan(day).then(
      (p) => {
        if (!live) return;
        setPlan(p);
        setLoadError(null);
      },
      (e) => {
        if (!live) return;
        if (shownDay.current === day) d.fail(e); // a refresh failed: keep the plan showing, and say so
        else setLoadError(apiError(e).message);
      },
    );
    return () => {
      live = false;
    };
  }, [day, d.planVersion, d.tasksVersion, reload]);
  useEffect(() => {
    App.ListCommitments().then((l) => setCommitments(l.commitments), d.fail);
  }, [d.goalsVersion, d.fail]);
  useEffect(() => {
    App.ListGoals("active").then(
      (l) => setGoals(new Map(l.goals.map((g) => [g.id, g.title]))),
      () => setGoals(new Map()),
    );
  }, [d.goalsVersion]);
  // Plan with Gwen from Today or the assistant opens the chat for today.
  useEffect(() => {
    if (param === "chat") {
      setDay(today);
      setChat(true);
    }
  }, [param]);
  useEffect(() => setEditHours(false), [day]);
  useEffect(() => {
    let live = true;
    if (day > today) (setTracked([]), setTrackedBy([]));
    else
      App.GetDay(day).then(
        (x) => live && (setTracked(x.segments), setTrackedBy(x.summary.by_project)),
        () => live && (setTracked([]), setTrackedBy([])),
      );
    return () => {
      live = false;
    };
  }, [day, today, d.daysVersion]);

  const colors = useMemo(() => {
    const m = new Map<string | null, string>([[null, unassignedColor]]);
    d.projects.forEach((p) => m.set(p.id, p.color));
    trackedBy.forEach((p) => p.project_id && !m.has(p.project_id) && m.set(p.project_id, p.color));
    return m;
  }, [d.projects, trackedBy]);
  const names = useMemo(() => {
    const m = new Map<string | null, string>([[null, "Unassigned"]]);
    d.projects.forEach((p) => m.set(p.id, projectPath(p, d.projects)));
    trackedBy.forEach((p) => p.project_id && !m.has(p.project_id) && m.set(p.project_id, p.name));
    return m;
  }, [d.projects, trackedBy]);

  const past = day < today || readOnly; // nothing past, and nothing on the hub, can change
  const rollover = d.config?.tracking.day_rollover ?? "04:00";
  const onDay = commitments.filter((c) => occursOn(c, day));
  const items = plan?.items ?? [];
  const groups = useMemo(() => groupPlan(items.filter((it) => !it.task.parent_id)), [items]);
  // Duration and pinning changes go through one block and apply to all the task's blocks on the day.
  const patch = (it: wire.PlanItem, fields: Record<string, unknown>) => d.act(() => App.PatchPlanItem(it.id, wire.PatchPlanItemRequest.createFrom({ ...fields, rev: it.rev })));

  /**
   * Puts a task before the row at index (groups.length: last); the planner times the day in the new order. Rows with
   * a fixed time, done, or skipped keep their place in time, so the task goes before the next row Gwen still times.
   */
  async function move(taskId: string, index: number) {
    const before = groups.slice(index).find((g) => g.task.id !== taskId && !g.pinned && !g.done && !g.skipped);
    const p = await d.act(() => App.MovePlanTask(wire.MovePlanTaskRequest.createFrom({ day, task_id: taskId, before_task_id: before?.task.id })));
    if (p) setPlan(p);
  }
  /** A time set by hand: the task's whole time on the day, as one block that stays there. */
  function setTime(g: PlanGroup, hhmm: string) {
    if (!hhmm) {
      if (g.pinned) patch(leadItem(g), { pinned: false });
      else setCleared((n) => n + 1); // nothing to clear: show its time again
      return;
    }
    const planned = Math.min(720, Math.max(5, g.minutes || g.task.estimate_minutes || 30));
    d.act(() => App.ScheduleTask(wire.ScheduleRequest.createFrom({ task_id: g.task.id, day, start_at: instantOn(day, hhmm, rollover), planned_minutes: planned })));
  }

  async function generate() {
    setGenerating(true);
    await d.act(() => App.GeneratePlan(wire.GeneratePlanRequest.createFrom({ day })));
    setGenerating(false);
  }
  function dragOver(e: DragEvent, id: string) {
    if (dragging && dragging !== id) {
      e.preventDefault();
      setTarget(id);
    }
  }
  function drop(e: DragEvent, index: number) {
    e.preventDefault();
    if (dragging) move(dragging, index);
    setDragging(null);
    setTarget(null);
  }
  function closeChat() {
    setChat(false);
    setPreview(null);
  }

  const capacity = plan?.capacity_minutes ?? 0;
  const planned = plan?.planned_minutes ?? 0;
  const over = planned > capacity;
  const itemBlock = (it: wire.PlanItem): PlannedBlock => ({
    id: it.id,
    title: it.task.title,
    start: it.start_at!,
    minutes: it.planned_minutes,
    color: colors.get(it.task.project_id ?? null) ?? unassignedColor,
    done: it.status === "done",
  });
  // While the chat proposes a plan, the plan lane previews it in place of the planned items.
  const blocks: PlannedBlock[] = preview
    ? [
        ...items.filter((it) => it.start_at != null && it.status === "done").map(itemBlock),
        ...preview.map((b, i) => ({
          id: `preview-${i}`,
          title: b.title,
          start: b.start_at,
          minutes: b.planned_minutes,
          color: colors.get(b.project_id) ?? unassignedColor,
          done: false,
          proposed: true,
        })),
      ]
    : items.filter((it) => it.start_at != null && it.status !== "skipped").map(itemBlock);
  const usualStart = d.config?.planner.day_start ?? "09:00";
  const shownStart = plan ? clockOf(Math.min(minutesOf(usualStart), plan.window.start_minute)) : usualStart;
  const shownFrom = instantOn(day, shownStart, rollover);
  const shownTo = instantOn(day, d.config?.planner.day_end ?? "23:00", rollover);
  const events = eventBlocks(plan, shownFrom, shownTo);
  const fixed: FixedBlock[] = onDay
    .filter((c) => c.start_minute != null)
    .map((c) => ({
      id: c.id,
      title: c.title,
      start: instantOn(day, clockOf(c.start_minute!), rollover),
      minutes: c.duration_minutes,
    }))
    .concat(events);
  const chatOpen = chat && !past && !!plan && plan.day === day;
  const failed = loadError && (!plan || plan.day !== day);

  const header = (
    <header className="flex flex-wrap items-center gap-x-3 gap-y-2">
      <h1 className="mr-auto text-[22px] font-semibold tracking-[-0.01em]">Plan</h1>
      <div className="flex items-center gap-1">
        {day !== today && (
          <Button size="sm" tone="ghost" onClick={() => setDay(today)}>
            Today
          </Button>
        )}
        <IconButton icon={ChevronLeft} label="Previous day" size="sm" onClick={() => setDay(addDays(day, -1))} />
        <DayPicker day={day} today={today} onPick={setDay} />
        <IconButton icon={ChevronRight} label="Next day" size="sm" onClick={() => setDay(addDays(day, 1))} />
      </div>
      {!past && (
        <Button
          size="sm"
          tone={chatOpen ? "primary" : "secondary"}
          icon={Sparkles}
          iconColor={chatOpen ? undefined : "var(--color-accent-hover)"}
          aria-pressed={chatOpen}
          onClick={() => (chatOpen ? closeChat() : setChat(true))}
        >
          Plan with {name}
        </Button>
      )}
    </header>
  );

  let body: ReactNode;
  if (failed)
    body = (
      <Callout
        tone="danger"
        icon={CircleAlert}
        action={
          <Button size="sm" onClick={() => setReload((n) => n + 1)}>
            Try again
          </Button>
        }
      >
        Couldn't load the plan for {formatLongDate(day)}. {loadError}
      </Callout>
    );
  else if (!plan || plan.day !== day)
    body = (
      <section aria-label="The day" className="lift flex items-center gap-2.5 rounded-xl border border-line bg-surface-1 px-[18px] py-5 text-[13px] text-ink-subtle" role="status">
        <LoaderCircle size={15} className="animate-spin" aria-hidden />
        Loading the plan for {formatLongDate(day)}…
      </section>
    );
  else
    body = (
      <>
        <section aria-label="The day" className="lift min-w-0 rounded-xl border border-line bg-surface-1 px-[18px] pt-3.5 pb-4">
          <div className="flex flex-wrap items-end gap-x-7 gap-y-3">
            <Figure value={minutes(planned)} label="planned" />
            <Figure value={minutes(capacity)} label={day === today ? "room left in the day" : "room in the day"} />
            {over ? (
              <Figure value={minutes(planned - capacity)} label="more than fits" tone="warning" title="More than the day has room for. Skip or shorten something." />
            ) : (
              <Figure value={minutes(capacity - planned)} label="free" tone={capacity - planned > 0 ? "success" : "muted"} />
            )}
            <div className="min-w-48 flex-[1_1_220px] pb-0.5">
              <Meter
                value={capacity > 0 ? planned / capacity : planned > 0 ? 1 : 0}
                color={over ? "var(--color-warning)" : "var(--color-accent)"}
                height={8}
                label="Planned against the room in the day"
              />
              <div className="mt-1.5 flex flex-wrap items-baseline justify-between gap-x-3 gap-y-0.5 text-[11.5px] text-ink-subtle">
                <span>{hoursLine(plan, d.config?.planner.buffer ?? "")}</span>
                {!past && (
                  <button
                    type="button"
                    aria-expanded={editHours}
                    onClick={() => setEditHours(!editHours)}
                    className="text-ink-muted underline decoration-line-3 underline-offset-2 hover:text-ink hover:decoration-ink-subtle"
                  >
                    {editHours ? "Close" : "Change hours"}
                  </button>
                )}
              </div>
            </div>
          </div>
          {editHours && !past && <PlanHours plan={plan} onPlan={setPlan} onClose={() => setEditHours(false)} />}
          {(onDay.length + events.length > 0 || preview) && (
            <div className="mt-3 flex flex-wrap gap-1.5">
              {onDay.map((c) => (
                <Chip key={c.id} icon={Repeat} title="A commitment: the day is planned around it">
                  <span className="text-ink">{c.title}</span> · {c.start_minute == null ? "any time" : formatClock(c.start_minute)} · {minutes(c.duration_minutes)}
                </Chip>
              ))}
              {events.map((e) => (
                <Chip key={e.id} icon={CalendarDays} title="From your calendar: the day is planned around it">
                  <span className="text-ink">{e.title}</span> · {formatTime(e.start)} · {minutes(e.minutes)}
                </Chip>
              ))}
              {preview && (
                <span className="inline-flex min-h-5 items-center gap-1.5 rounded-[10px] bg-accent/12 px-2 py-0.5 text-[11px] leading-4 text-accent-hover">
                  <Sparkles size={11} className="shrink-0" aria-hidden />
                  {name}'s proposal is showing on the plan lane. Apply it to keep it.
                </span>
              )}
            </div>
          )}
          <div className="mt-3.5">
            <Timeline segments={tracked} colors={colors} names={names} now={d.now()} live={day === today} from={shownFrom} to={shownTo} planned={blocks} fixed={fixed} />
          </div>
        </section>

        <Card
          title="Tasks"
          icon={ListChecks}
          bodyClassName=""
          meta={
            groups.length > 1 &&
            !past && (
              <span title={`Drag tasks into the order you want to do them. ${name} times the day in that order and keeps it current as you go; a time you set by hand stays fixed.`}>
                Drag to reorder; {name} re-times the day
              </span>
            )
          }
        >
          {groups.length === 0 ? (
            <div className="flex flex-col items-start gap-1 px-4 py-5">
              <p className="text-[13px] font-medium text-ink">{past ? "Nothing was planned" : day > today ? "Not planned yet" : "Nothing to plan yet"}</p>
              {!past && (
                <p className="max-w-lg text-[12.5px] leading-relaxed text-ink-subtle">
                  {day > today
                    ? `${name} fills the day from your goals and open tasks, around your commitments, and keeps it up to date from then on.`
                    : "Tasks with a time estimate and your goals' sessions show up here by themselves, timed around your commitments."}
                </p>
              )}
              {!past && day > today && (
                <Button size="sm" tone="primary" icon={RefreshCw} onClick={generate} busy={generating} className="mt-2.5">
                  Plan this day
                </Button>
              )}
            </div>
          ) : (
            <ul onDragLeave={(e) => e.currentTarget === e.target && setTarget(null)}>
              {groups.map((g, i) => {
                const id = g.task.id;
                const isDone = g.done;
                const trackedMs = g.task.tracked_ms ?? 0;
                if (!past) {
                  const movable = !isDone && !g.skipped;
                  const lead = leadItem(g);
                  const controls = g.skipped ? (
                    <PutBack group={g} />
                  ) : isDone ? (
                    trackedMs > 0 && <Badge tone="working">Tracked {formatDuration(trackedMs)}</Badge>
                  ) : (
                    <>
                      <TimeInput
                        key={`${id}-t${cleared}`}
                        allowEmpty
                        className="!h-7 w-[78px] !border-line !bg-canvas !px-1 font-mono !text-[12px]"
                        aria-label={`Start time of ${g.task.title}`}
                        title={g.pinned ? `The start time you set. Clear it to let ${name} time it.` : `${name} times it for you. Set a start time only to fix it there.`}
                        value={g.start != null ? clockValue(g.start) : ""}
                        onCommit={(v) => setTime(g, v)}
                      />
                      <input
                        type="number"
                        min="5"
                        className="field !h-7 w-12 !border-line !bg-canvas !px-1.5 text-right font-mono !text-[12px] tabular-nums"
                        aria-label={`Minutes for ${g.task.title}`}
                        key={`${id}-m${g.minutes}`}
                        defaultValue={g.minutes}
                        onKeyDown={(e) => e.key === "Enter" && e.currentTarget.blur()}
                        onBlur={(e) => {
                          const m = Number(e.target.value);
                          if (m > 0 && m !== g.minutes) patch(lead, { planned_minutes: m });
                        }}
                      />
                      <span className="mr-1 text-[11px] text-ink-subtle">min</span>
                      <IconButton
                        icon={Pin}
                        label={g.pinned ? `Let ${name} time it` : g.start == null ? "Set a start time to fix it there" : "Fix it at this time"}
                        size="sm"
                        active={g.pinned}
                        disabled={!g.pinned && g.start == null}
                        onClick={() => patch(lead, { pinned: !g.pinned })}
                      />
                    </>
                  );
                  return (
                    <TaskRow
                      key={id}
                      task={g.task}
                      plan={g}
                      first={i === 0}
                      leadWidth={26}
                      lead={
                        <button
                          type="button"
                          disabled={!movable}
                          className="grid h-6 w-3.5 shrink-0 cursor-grab place-items-center rounded text-ink-faint hover:text-ink-subtle disabled:cursor-default disabled:opacity-30"
                          aria-label={`Move ${g.task.title}: drag, or press the up and down arrows`}
                          title={`Drag to change the order, or press the up and down arrows. ${name} times the day in that order.`}
                          onKeyDown={(e) => {
                            if (e.key === "ArrowUp" && i > 0) {
                              e.preventDefault();
                              move(id, i - 1);
                            } else if (e.key === "ArrowDown" && i < groups.length - 1) {
                              e.preventDefault();
                              move(id, i + 2);
                            }
                          }}
                        >
                          <GripVertical size={14} aria-hidden />
                        </button>
                      }
                      // In a narrow column (Plan with Gwen open beside it) the time and minutes wrap under the title, to the right.
                      controls={<span className="ml-auto flex shrink-0 items-center gap-1 self-center">{controls}</span>}
                      liProps={{
                        draggable: movable,
                        onDragStart: (e) => {
                          setDragging(id);
                          e.dataTransfer.effectAllowed = "move";
                          e.dataTransfer.setData("application/x-gwen-task", id);
                        },
                        onDragEnd: () => {
                          setDragging(null);
                          setTarget(null);
                        },
                        onDragOver: (e) => dragOver(e, id),
                        onDrop: (e) => drop(e, i),
                        className: cx(
                          target === id && "shadow-[inset_0_2px_0_0_var(--color-accent)]",
                          dragging === id && "opacity-40",
                        ),
                      }}
                    />
                  );
                }

                const tracking = t.isTracking(id);
                const noRoom = !isDone && !g.skipped && g.start == null;
                const project = d.projects.find((p) => p.id === g.task.project_id);
                const goal = g.task.goal_id ? goals.get(g.task.goal_id) : undefined;
                const timed = g.items.filter((it) => it.status !== "skipped" && it.start_at != null);
                const stepsOpen = unfolded[id] ?? tracking;
                const stepsTicked = g.steps.filter((s) => s.status === "done").length;

                const meta: ReactNode[] = [
                  <span key="project" className="inline-flex min-w-0 items-center gap-1.5">
                    <Swatch color={project?.color ?? unassignedColor} size={7} />
                    <span className="truncate">{project ? projectPath(project, d.projects) : "No project"}</span>
                  </span>,
                ];
                if (goal) meta.push(<span key="goal" className="truncate">Goal: {goal}</span>);
                if (timed.length > 1) meta.push(<span key="times" className="tabular-nums">in blocks at {timed.map((it) => formatTime(it.start_at!)).join(", ")}</span>);
                else if (g.start != null)
                  meta.push(
                    <span key="times" className="tabular-nums">
                      {formatTime(g.start)} to {formatTime(g.start + g.minutes * 60_000)}
                    </span>,
                  );
                if (g.task.quantity) meta.push(<span key="qty" className="tabular-nums">{`${g.task.quantity_done ?? 0} of ${g.task.quantity} done`}</span>);
                if (g.steps.length > 0)
                  meta.push(
                    <button
                      key="steps"
                      type="button"
                      aria-expanded={stepsOpen}
                      onClick={() => setUnfolded((u) => ({ ...u, [id]: !stepsOpen }))}
                      className="-mx-1 inline-flex items-center gap-1 rounded px-1 tabular-nums hover:bg-surface-3 hover:text-ink"
                    >
                      <ChevronRight size={11} className={cx("transition-transform", stepsOpen && "rotate-90")} aria-hidden />
                      {stepsTicked} of {g.steps.length} steps
                    </button>,
                  );
                if (g.pinned)
                  meta.push(
                    <span key="pinned" className="inline-flex items-center gap-1 text-accent-hover" title="A time you set: the rest of the day flows around it">
                      <Pin size={10} aria-hidden />
                      fixed time
                    </span>,
                  );
                if (noRoom) meta.push(<span key="room" className="text-idle">No room today</span>);
                if (g.skipped) meta.push(<span key="skipped">Skipped today</span>);

                return (
                  <li
                    key={id}
                    className={cx(
                      i > 0 && "border-t border-chrome-line",
                      tracking ? "bg-surface-2/50" : "hover:bg-surface-2/40",
                    )}
                  >
                    <div className="flex min-h-11 flex-wrap items-center gap-x-2.5 gap-y-1.5 py-1.5 pr-3.5 pl-2">
                      <span className="w-3.5 shrink-0" />
                      <Checkbox checked={isDone} disabled onChange={() => {}} label={g.task.title} />
                      <div className="min-w-0 flex-[1_1_13rem]">
                        <div
                          className={cx("truncate text-[13px] leading-5", isDone || g.skipped ? "text-ink-subtle line-through decoration-ink-faint" : "text-ink", tracking && "font-medium")}
                          title={g.task.title}
                        >
                          {g.task.title}
                        </div>
                        <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-0.5 text-xs leading-[18px] text-ink-subtle">
                          {meta}
                          {g.rollover > 0 && (
                            <span
                              className="text-idle tabular-nums"
                              title={`Carried over from earlier days ${g.rollover} ${g.rollover === 1 ? "time" : "times"}`}
                            >
                              Carried over {g.rollover}×
                            </span>
                          )}
                        </div>
                      </div>
                      <div className="ml-auto flex shrink-0 items-center gap-1">
                        {trackedMs > 0 && <Badge tone="working">Tracked {formatDuration(trackedMs)}</Badge>}
                        <span className="w-14 text-right font-mono text-[12px] text-ink-subtle">{minutes(g.minutes)}</span>
                      </div>
                    </div>
                    {stepsOpen && g.steps.length > 0 && (
                      <div className="mr-3.5 mb-2 ml-8">
                        <ul className="ml-[1.85rem] flex flex-col border-l border-line pl-2">
                          {g.steps.map((s) => (
                            <li key={s.id} className="flex items-start gap-2.5 px-2 py-1.5">
                              <span className="pt-px"><Checkbox checked={s.status === "done"} disabled onChange={() => {}} label={s.title} /></span>
                              <div className="min-w-0 flex-1">
                                <div className={cx("text-[13px] leading-snug", s.status === "done" ? "text-ink-faint line-through" : "text-ink")}>{s.title}</div>
                                {s.notes && <p className="mt-0.5 text-xs leading-relaxed whitespace-pre-line text-ink-subtle">{s.notes}</p>}
                              </div>
                              {s.estimate_minutes != null && <span className="pt-0.5 text-xs text-ink-faint tabular-nums">{minutes(s.estimate_minutes)}</span>}
                            </li>
                          ))}
                        </ul>
                      </div>
                    )}
                  </li>
                );
              })}
              {!past && dragging && (
                <li
                  onDragOver={(e) => dragOver(e, "end")}
                  onDrop={(e) => drop(e, groups.length)}
                  className={cx(
                    "mx-3 my-2 grid h-9 place-items-center rounded-md border border-dashed text-[12px]",
                    target === "end" ? "border-accent text-ink-subtle" : "border-line-strong text-ink-faint",
                  )}
                >
                  Move to the end
                </li>
              )}
            </ul>
          )}
        </Card>
      </>
    );

  const page = (
    <div className="flex flex-col gap-4">
      {header}
      {body}
    </div>
  );
  // The hub shows the screen inside its own scrolling page, read-only, so it has no chat beside it.
  if (isHub) return page;
  return (
    <PlanWindow>
      <div className="min-w-0 flex-1">
        <Scroll>{page}</Scroll>
      </div>
      {chatOpen && <PlanChat key={day} day={day} plan={plan} onPreview={setPreview} onClose={closeChat} />}
    </PlanWindow>
  );
}

// The hub has no TaskUI provider; only the desktop window mounts these controls.
function PlanWindow({ children }: { children: ReactNode }) {
  const ui = useTaskUI();
  const root = useRef<HTMLDivElement>(null);
  const clear = useClearOfSheet(root, !!ui.handing);
  return <div ref={root} className="flex h-full min-h-0" style={clear ? { paddingRight: clear } : undefined}>{children}</div>;
}

function PutBack({ group }: { group: PlanGroup }) {
  const ui = useTaskUI();
  return <Button size="sm" onClick={() => ui.putBack(group)}>Put back</Button>;
}

/** One of the day's numbers: the figure, then what it is. */
function Figure({ value, label, tone, title }: { value: string; label: string; tone?: "success" | "warning" | "muted"; title?: string }) {
  return (
    <div title={title}>
      <div
        className={cx(
          "font-mono text-[22px] leading-7 font-semibold tracking-[-0.02em] tabular-nums",
          tone === "success" ? "text-success" : tone === "warning" ? "text-warning" : tone === "muted" ? "text-ink-subtle" : "text-ink",
        )}
      >
        {value}
      </div>
      <div className="text-[12px] text-ink-subtle">{label}</div>
    </div>
  );
}

/** Something the day is planned around, such as a commitment or a calendar event. */
function Chip({ icon: Icon, title, children }: { icon: LucideIcon; title: string; children: ReactNode }) {
  return (
    <span title={title} className="inline-flex h-5 max-w-full items-center gap-1.5 truncate rounded-full bg-surface-2 px-2 text-[11px] text-ink-muted tabular-nums">
      <Icon size={11} className="shrink-0 text-ink-subtle" aria-hidden />
      <span className="truncate">{children}</span>
    </span>
  );
}

/** The day being planned, as a button that opens a calendar to pick another, with shortcuts to the days around today. */
function DayPicker({ day, today, onPick }: { day: string; today: string; onPick: (day: string) => void }) {
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const away = (e: MouseEvent) => !box.current?.contains(e.target as Node) && setOpen(false);
    const key = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", away);
    window.addEventListener("keydown", key);
    return () => {
      document.removeEventListener("mousedown", away);
      window.removeEventListener("keydown", key);
    };
  }, [open]);
  const pick = (d: string) => {
    onPick(d);
    setOpen(false);
  };
  return (
    <div ref={box} className="relative">
      <Button size="sm" aria-haspopup="dialog" aria-expanded={open} title="Pick a day" onClick={() => setOpen(!open)}>
        {formatLongDate(day)}
        <span className="font-normal text-ink-subtle">· {relativeDay(day, today)}</span>
      </Button>
      {open && (
        <div role="dialog" aria-label="Pick a day" className="absolute top-full right-0 z-30 mt-1.5 flex w-60 flex-col gap-2 rounded-lg border border-line-strong bg-surface-2 p-2 shadow-lg shadow-black/40">
          <Input type="date" value={day} onChange={(e) => e.target.value && onPick(e.target.value)} aria-label="Day" className="!h-8 w-full !text-[13px]" autoFocus />
          <div className="grid grid-cols-3 gap-1">
            <Button size="sm" tone="ghost" onClick={() => pick(addDays(today, -1))}>
              Yesterday
            </Button>
            <Button size="sm" tone="ghost" onClick={() => pick(today)}>
              Today
            </Button>
            <Button size="sm" tone="ghost" onClick={() => pick(addDays(today, 1))}>
              Tomorrow
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
