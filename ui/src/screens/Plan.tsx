import { CalendarClock, CalendarDays, ChevronLeft, ChevronRight, GripVertical, Pin, Play, Radio, RefreshCw, SkipForward, Sparkles, Undo2 } from "lucide-react";
import { useEffect, useMemo, useState, type DragEvent } from "react";
import { App, readOnly, wire } from "../api";
import { useCompleteTask } from "../components/complete";
import DayHours from "../components/DayHours";
import PlanChat from "../components/PlanChat";
import { StepList, stepsDone } from "../components/steps";
import { ProjectTag } from "../components/tags";
import TimeInput from "../components/TimeInput";
import Timeline, { type FixedBlock, type PlannedBlock } from "../components/Timeline";
import { Badge, Button, Checkbox, Empty, IconButton, Input, Meter, PageHeader, Panel, cx, unassignedColor } from "../components/ui";
import { useDaemon } from "../daemon";
import { addDays, clockValue, formatClock, formatDuration, formatLongDate, formatTime, instantOn, minutesOf, relativeDay } from "../format";
import type { ProposedBlock } from "../llm";
import { useNav } from "../nav";
import { blockTimes, eventBlocks, groupPlan, leadItem, type PlanGroup } from "../plan";
import { clockOf, occursOn } from "../rrule";
import { useTracking } from "../tracking";

const minutes = (m: number) => formatDuration(m * 60_000);

export default function Plan() {
  const d = useDaemon();
  const t = useTracking();
  const done = useCompleteTask();
  const today = d.today();
  const { param } = useNav();
  const [day, setDay] = useState(today);
  const [chat, setChat] = useState(param === "chat");
  const [preview, setPreview] = useState<ProposedBlock[] | null>(null);
  const [plan, setPlan] = useState<wire.Plan | null>(null);
  const [commitments, setCommitments] = useState<wire.Commitment[]>([]);
  const [tracked, setTracked] = useState<wire.Segment[]>([]);
  // Dragging moves a task: these are task ids, and "end" is the drop zone after the last row.
  const [dragging, setDragging] = useState<string | null>(null);
  const [target, setTarget] = useState<string | null>(null);
  const [generating, setGenerating] = useState(false);
  const [cleared, setCleared] = useState(0);

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
  // Plan with AI from Today or Assistant opens the chat for today.
  useEffect(() => {
    if (param === "chat") {
      setDay(today);
      setChat(true);
    }
  }, [param]);
  useEffect(() => {
    let live = true;
    if (day > today) setTracked([]);
    else
      App.GetDay(day).then(
        (x) => live && setTracked(x.segments),
        () => live && setTracked([]),
      );
    return () => {
      live = false;
    };
  }, [day, today, d.daysVersion]);

  const colors = useMemo(() => {
    const m = new Map<string | null, string>([[null, unassignedColor]]);
    d.projects.forEach((p) => m.set(p.id, p.color));
    return m;
  }, [d.projects]);
  const names = useMemo(() => {
    const m = new Map<string | null, string>([[null, "Unassigned"]]);
    d.projects.forEach((p) => m.set(p.id, p.name));
    return m;
  }, [d.projects]);

  const past = day < today || readOnly; // nothing past, and nothing on the hub, can change
  const rollover = d.config?.tracking.day_rollover ?? "04:00";
  const onDay = commitments.filter((c) => occursOn(c, day));
  const items = plan?.items ?? [];
  const groups = useMemo(() => groupPlan(items), [items]);
  // Skipping and pinning go through one block and apply to all the task's blocks on the day.
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
  const rel = relativeDay(day, today);

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Plan"
        subtitle={["today", "tomorrow", "yesterday"].includes(rel) ? `${formatLongDate(day)} (${rel})` : formatLongDate(day)}
        actions={
          <>
            <div className="flex items-center gap-1">
              <IconButton icon={ChevronLeft} label="Previous day" tone="secondary" onClick={() => setDay(addDays(day, -1))} />
              <Input type="date" value={day} onChange={(e) => e.target.value && setDay(e.target.value)} aria-label="Day" className="w-40" />
              <IconButton icon={ChevronRight} label="Next day" tone="secondary" onClick={() => setDay(addDays(day, 1))} />
            </div>
            {day !== today && <Button onClick={() => setDay(today)}>Today</Button>}
            {!past && !chat && (
              <Button icon={Sparkles} iconColor="var(--color-accent-hover)" onClick={() => setChat(true)}>
                Plan with AI
              </Button>
            )}
            {!past && day > today && items.length === 0 && (
              <Button tone="primary" icon={RefreshCw} onClick={generate} busy={generating}>
                Plan this day
              </Button>
            )}
          </>
        }
      />

      <section className="lift rounded-2xl border border-line bg-surface-1">
        <div className="flex flex-wrap items-end gap-x-10 gap-y-3 px-6 pt-5">
          <Figure value={minutes(planned)} label="planned" />
          <Figure value={minutes(capacity)} label="room in the day" />
          <Figure value={minutes(Math.max(0, capacity - planned))} label="free" muted={capacity - planned <= 0} />
          {over && <p className="ml-auto self-center text-[13px] text-idle">{minutes(planned - capacity)} more than the day has room for. Skip or shorten something.</p>}
        </div>
        <div className="px-6 pt-4">
          <Meter value={capacity > 0 ? planned / capacity : planned > 0 ? 1 : 0} color={over ? "var(--color-idle)" : "var(--color-accent)"} height={8} label="Planned against the room in the day" />
        </div>
        {!past && plan && (
          <div className="px-6 pt-5">
            <DayHours plan={plan} onPlan={setPlan} />
          </div>
        )}
        {onDay.length + events.length > 0 && (
          <div className="flex flex-wrap gap-2 px-6 pt-4">
            {onDay.map((c) => (
              <span key={c.id} className="hatched-grey inline-flex items-center gap-2 rounded-md border border-line px-2.5 py-1 text-xs text-ink-muted">
                <span className="font-medium text-ink">{c.title}</span>
                <span className="tabular-nums">{c.start_minute == null ? "any time" : `${formatClock(c.start_minute)}–${formatClock(c.start_minute + c.duration_minutes)}`}</span>
                <span className="text-ink-subtle">{minutes(c.duration_minutes)}</span>
              </span>
            ))}
            {events.map((e) => (
              <span key={e.id} title="From Google Calendar" className="hatched-grey inline-flex items-center gap-2 rounded-md border border-line px-2.5 py-1 text-xs text-ink-muted">
                <CalendarDays size={12} className="text-ink-subtle" aria-hidden />
                <span className="font-medium text-ink">{e.title}</span>
                <span className="tabular-nums">{`${formatTime(e.start)}–${formatTime(e.start + e.minutes * 60_000)}`}</span>
                <span className="text-ink-subtle">{minutes(e.minutes)}</span>
              </span>
            ))}
          </div>
        )}
        <div className="mt-5 border-t border-line px-6 py-4">
          <Timeline
            segments={tracked}
            colors={colors}
            names={names}
            now={d.now()}
            live={day === today}
            from={shownFrom}
            to={shownTo}
            planned={blocks}
            fixed={fixed}
          />
          {preview && (
            <p className="mt-2 flex items-center gap-1.5 text-xs text-ink-subtle">
              <Sparkles size={12} className="text-accent-hover" aria-hidden />
              The plan lane shows the AI's proposal. Apply it below to keep it.
            </p>
          )}
        </div>
      </section>

      {chat && !past && plan && (
        <PlanChat
          key={day}
          day={day}
          plan={plan}
          onPreview={setPreview}
          onClose={() => {
            setChat(false);
            setPreview(null);
          }}
        />
      )}

      <Panel title="Tasks" icon={CalendarClock} bodyClassName="px-3 pt-2 pb-4">
        {plan && items.length === 0 ? (
          <Empty
            icon={CalendarClock}
            title={past ? "Nothing was planned" : day > today ? "Not planned yet" : "Nothing to plan yet"}
            className="mx-2"
            action={
              !past &&
              day > today && (
                <Button tone="primary" icon={RefreshCw} onClick={generate} busy={generating}>
                  Plan this day
                </Button>
              )
            }
          >
            {!past &&
              (day > today
                ? "Gwen fills the day from your goals and open tasks, around your commitments, and keeps it up to date from then on."
                : "Tasks with a time estimate and your goals' sessions show up here by themselves, timed around your commitments.")}
          </Empty>
        ) : (
          <ul className="flex flex-col" onDragLeave={(e) => e.currentTarget === e.target && setTarget(null)}>
            {groups.map((g, i) => {
              const id = g.task.id;
              const isDone = g.done;
              const tracking = t.isTracking(id);
              const movable = !past && !isDone && !g.skipped;
              const lead = leadItem(g);
              return (
                <li
                  key={id}
                  draggable={movable}
                  onDragStart={(e) => {
                    setDragging(id);
                    e.dataTransfer.effectAllowed = "move";
                    e.dataTransfer.setData("application/x-gwen-task", id);
                  }}
                  onDragEnd={() => {
                    setDragging(null);
                    setTarget(null);
                  }}
                  onDragOver={(e) => !past && dragOver(e, id)}
                  onDrop={(e) => drop(e, i)}
                  className={cx(
                    "group rounded-lg border-t-2 transition-colors",
                    target === id ? "border-accent" : "border-transparent",
                    dragging === id && "opacity-40",
                    tracking ? "bg-surface-2" : "hover:bg-surface-2/60",
                  )}
                >
                  <div className="flex items-center gap-2.5 px-2 py-2">
                    {!past && (
                      <button
                        type="button"
                        disabled={!movable}
                        className="grid size-6 shrink-0 cursor-grab place-items-center rounded text-ink-faint hover:text-ink-subtle disabled:cursor-default disabled:opacity-30"
                        aria-label={`Move ${g.task.title}: drag, or press the up and down arrows`}
                        title="Drag to change the order, or press the up and down arrows. Gwen times the day in that order."
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
                    )}
                    <Checkbox
                      checked={isDone}
                      disabled={past || g.skipped}
                      onChange={(on) => (on ? done.complete(g.task, g.steps.length > 0) : done.reopen(g.task))}
                      label={isDone ? `Reopen ${g.task.title}` : `Complete ${g.task.title}`}
                    />
                    <div className="min-w-0 flex-1">
                      <div className={cx("truncate text-sm", isDone ? "text-ink-faint line-through" : g.skipped ? "text-ink-faint" : "text-ink")}>{g.task.title}</div>
                      <div className="mt-0.5 flex min-w-0 items-center gap-3">
                        <ProjectTag project={d.projects.find((p) => p.id === g.task.project_id)} />
                        {g.items.length > 1 && g.start != null && <span className="text-xs text-ink-subtle tabular-nums">{blockTimes(g)}</span>}
                        {g.pinned && (
                          <span className="inline-flex items-center gap-1 text-xs text-accent-hover" title="A time you set: the rest of the day flows around it">
                            <Pin size={11} aria-hidden />
                            Fixed time
                          </span>
                        )}
                        {g.steps.length > 0 && <span className="text-xs text-ink-subtle tabular-nums">{stepsDone(g.steps)}</span>}
                        {g.rollover > 0 && <Badge tone="idle">Carried over {g.rollover}×</Badge>}
                        {g.skipped && <Badge>Skipped</Badge>}
                        {!past && !isDone && !g.skipped && g.start == null && <span className="text-xs text-idle">No room left today</span>}
                      </div>
                    </div>
                    {past || isDone ? (
                      <span className="w-36 text-right text-xs text-ink-subtle tabular-nums">
                        {g.start != null && `${formatTime(g.start)}  `}
                        {minutes(g.minutes)}
                      </span>
                    ) : (
                      <>
                        <TimeInput
                          key={`${id}-t${cleared}`}
                          allowEmpty
                          className="w-20 shrink-0"
                          aria-label={`Start time of ${g.task.title}`}
                          title={g.pinned ? "Start time you set, 24-hour. Clear it to let Gwen time it." : "Start time, 24-hour. Set one to fix it there."}
                          disabled={g.skipped}
                          value={g.start != null ? clockValue(g.start) : ""}
                          onCommit={(v) => setTime(g, v)}
                        />
                        <label className="relative shrink-0">
                          <Input
                            type="number"
                            min="5"
                            className="w-22 pr-10 text-right tabular-nums"
                            aria-label={`Minutes for ${g.task.title}`}
                            key={`${id}-m${g.minutes}`}
                            disabled={g.skipped}
                            defaultValue={g.minutes}
                            onBlur={(e) => {
                              const m = Number(e.target.value);
                              if (m > 0 && m !== g.minutes) patch(lead, { planned_minutes: m });
                            }}
                          />
                          <span className="pointer-events-none absolute top-1/2 right-2.5 -translate-y-1/2 text-xs text-ink-faint">min</span>
                        </label>
                        <IconButton
                          icon={Pin}
                          label={g.pinned ? "Let Gwen time it" : "Fix it at this time"}
                          size="sm"
                          active={g.pinned}
                          disabled={g.skipped}
                          onClick={() => patch(lead, { pinned: !g.pinned })}
                        />
                        <IconButton
                          icon={g.skipped ? Undo2 : SkipForward}
                          label={g.skipped ? "Put back on the plan" : "Skip for this day"}
                          size="sm"
                          onClick={() => patch(lead, { status: g.skipped ? "planned" : "skipped" })}
                        />
                        {day === today &&
                          (tracking ? (
                            <span className="grid size-7 place-items-center text-working" title="Tracking now">
                              <Radio size={15} aria-hidden />
                            </span>
                          ) : (
                            <IconButton icon={Play} label={`Start ${g.task.title}`} tone="secondary" size="sm" disabled={g.skipped} onClick={() => t.startTask(g.task)} />
                          ))}
                      </>
                    )}
                  </div>
                  {g.steps.length > 0 && <StepList parent={g.task} steps={g.steps} adding={!past} className="mb-2" />}
                </li>
              );
            })}
            {dragging && (
              <li
                onDragOver={(e) => dragOver(e, "end")}
                onDrop={(e) => drop(e, groups.length)}
                className={cx("grid h-10 place-items-center rounded-lg border border-dashed text-xs", target === "end" ? "border-accent text-ink-subtle" : "border-line text-ink-faint")}
              >
                Last
              </li>
            )}
          </ul>
        )}
        {groups.length > 1 && !past && (
          <p className="mt-2 px-2 text-xs text-ink-subtle">
            Drag tasks into the order you want to do them. Gwen times the day in that order and keeps it current as you go; a time you set by hand stays fixed.
          </p>
        )}
      </Panel>
      {done.dialog}
    </div>
  );
}

function Figure({ value, label, muted }: { value: string; label: string; muted?: boolean }) {
  return (
    <div>
      <div className={cx("text-title font-semibold tabular-nums", muted ? "text-ink-subtle" : "text-ink")}>{value}</div>
      <div className="text-xs text-ink-subtle">{label}</div>
    </div>
  );
}
