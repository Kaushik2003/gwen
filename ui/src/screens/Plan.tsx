import { CalendarClock, ChevronLeft, ChevronRight, GripVertical, Pin, Play, Radio, RefreshCw, SkipForward, Sparkles, Undo2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
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
import { addDays, formatDuration, formatLongDate, formatTime, instantOn, minutesOf, relativeDay } from "../format";
import type { ProposedBlock } from "../llm";
import { useNav } from "../nav";
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
  const [dragging, setDragging] = useState<string | null>(null);
  const [target, setTarget] = useState<string | null>(null);
  const [generating, setGenerating] = useState(false);

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
  const patch = (it: wire.PlanItem, fields: Record<string, unknown>) => d.act(() => App.PatchPlanItem(it.id, wire.PatchPlanItemRequest.createFrom({ ...fields, rev: it.rev })));
  const moveTo = (it: wire.PlanItem, other: wire.PlanItem | undefined) => other && other.id !== it.id && patch(it, { position: other.position });

  async function generate() {
    setGenerating(true);
    await d.act(() => App.GeneratePlan(wire.GeneratePlanRequest.createFrom({ day })));
    setGenerating(false);
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
  const fixed: FixedBlock[] = onDay
    .filter((c) => c.start_minute != null)
    .map((c) => ({
      id: c.id,
      title: c.title,
      start: instantOn(day, clockOf(c.start_minute!), rollover),
      minutes: c.duration_minutes,
    }));
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
            {!past && (
              <Button tone="primary" icon={RefreshCw} onClick={generate} busy={generating}>
                {items.length ? "Regenerate" : "Plan this day"}
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
        {onDay.length > 0 && (
          <div className="flex flex-wrap gap-2 px-6 pt-4">
            {onDay.map((c) => (
              <span key={c.id} className="hatched-grey inline-flex items-center gap-2 rounded-md border border-line px-2.5 py-1 text-xs text-ink-muted">
                <span className="font-medium text-ink">{c.title}</span>
                <span className="tabular-nums">{c.start_minute == null ? "any time" : `${clockOf(c.start_minute)}–${clockOf(c.start_minute + c.duration_minutes)}`}</span>
                <span className="text-ink-subtle">{minutes(c.duration_minutes)}</span>
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
            from={instantOn(day, shownStart, rollover)}
            to={instantOn(day, d.config?.planner.day_end ?? "23:00", rollover)}
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
            title={past ? "Nothing was planned" : "Nothing planned yet"}
            className="mx-2"
            action={
              !past && (
                <Button tone="primary" icon={RefreshCw} onClick={generate} busy={generating}>
                  Plan this day
                </Button>
              )
            }
          >
            {!past && "Gwen fills the day from your goals and open tasks, around your commitments."}
          </Empty>
        ) : (
          <ul className="flex flex-col">
            {items.map((it, i) => {
              const isDone = it.status === "done";
              const skipped = it.status === "skipped";
              const tracking = t.isTracking(it.task_id);
              return (
                <li
                  key={it.id}
                  draggable={!past}
                  onDragStart={(e) => {
                    setDragging(it.id);
                    e.dataTransfer.effectAllowed = "move";
                    e.dataTransfer.setData("text/plain", it.id);
                  }}
                  onDragEnd={() => {
                    setDragging(null);
                    setTarget(null);
                  }}
                  onDragOver={(e) => {
                    if (dragging && dragging !== it.id) {
                      e.preventDefault();
                      setTarget(it.id);
                    }
                  }}
                  onDrop={(e) => {
                    e.preventDefault();
                    const from = items.find((x) => x.id === dragging);
                    if (from) moveTo(from, it);
                    setDragging(null);
                    setTarget(null);
                  }}
                  className={cx(
                    "group rounded-lg border-t-2 transition-colors",
                    target === it.id ? "border-accent" : "border-transparent",
                    dragging === it.id && "opacity-40",
                    tracking ? "bg-surface-2" : "hover:bg-surface-2/60",
                  )}
                >
                  <div className="flex items-center gap-2.5 px-2 py-2">
                    {!past && (
                      <button
                        type="button"
                        className="grid size-6 shrink-0 cursor-grab place-items-center rounded text-ink-faint hover:text-ink-subtle"
                        aria-label={`Move ${it.task.title}: drag, or press the up and down arrows`}
                        title="Drag to reorder, or press the up and down arrows"
                        onKeyDown={(e) => {
                          if (e.key === "ArrowUp" || e.key === "ArrowDown") {
                            e.preventDefault();
                            moveTo(it, items[i + (e.key === "ArrowUp" ? -1 : 1)]);
                          }
                        }}
                      >
                        <GripVertical size={14} aria-hidden />
                      </button>
                    )}
                    <Checkbox
                      checked={isDone}
                      disabled={past || skipped}
                      onChange={(on) => (on ? done.complete(it.task, (it.steps ?? []).length > 0) : done.reopen(it.task))}
                      label={isDone ? `Reopen ${it.task.title}` : `Complete ${it.task.title}`}
                    />
                    <div className="min-w-0 flex-1">
                      <div className={cx("truncate text-sm", isDone ? "text-ink-faint line-through" : skipped ? "text-ink-faint" : "text-ink")}>{it.task.title}</div>
                      <div className="mt-0.5 flex min-w-0 items-center gap-3">
                        <ProjectTag project={d.projects.find((p) => p.id === it.task.project_id)} />
                        {it.pinned && (
                          <span className="inline-flex items-center gap-1 text-xs text-accent-hover">
                            <Pin size={11} aria-hidden />
                            Pinned
                          </span>
                        )}
                        {(it.steps ?? []).length > 0 && <span className="text-xs text-ink-subtle tabular-nums">{stepsDone(it.steps)}</span>}
                        {it.rollover_count > 0 && <Badge tone="idle">Carried over {it.rollover_count}×</Badge>}
                        {skipped && <Badge>Skipped</Badge>}
                      </div>
                    </div>
                    {past ? (
                      <span className="w-36 text-right text-xs text-ink-subtle tabular-nums">
                        {it.start_at != null && `${formatTime(it.start_at)}  `}
                        {minutes(it.planned_minutes)}
                      </span>
                    ) : (
                      <>
                        <TimeInput
                          allowEmpty
                          className="w-20 shrink-0"
                          aria-label={`Start time of ${it.task.title}`}
                          title="Start time, 24-hour. Empty lets the planner choose."
                          value={it.start_at != null ? formatTime(it.start_at) : ""}
                          onCommit={(v) => {
                            const at = v ? instantOn(day, v, rollover) : null;
                            if (at !== (it.start_at ?? null)) patch(it, { start_at: at });
                          }}
                        />
                        <label className="relative shrink-0">
                          <Input
                            type="number"
                            min="1"
                            className="w-22 pr-10 text-right tabular-nums"
                            aria-label={`Minutes for ${it.task.title}`}
                            key={`${it.id}-m${it.planned_minutes}`}
                            defaultValue={it.planned_minutes}
                            onBlur={(e) => {
                              const m = Number(e.target.value);
                              if (m > 0 && m !== it.planned_minutes) patch(it, { planned_minutes: m });
                            }}
                          />
                          <span className="pointer-events-none absolute top-1/2 right-2.5 -translate-y-1/2 text-xs text-ink-faint">min</span>
                        </label>
                        <IconButton icon={Pin} label={it.pinned ? "Unpin" : "Pin, so regenerating keeps it"} size="sm" active={it.pinned} onClick={() => patch(it, { pinned: !it.pinned })} />
                        {!isDone && (
                          <IconButton
                            icon={skipped ? Undo2 : SkipForward}
                            label={skipped ? "Put back on the plan" : "Skip for this day"}
                            size="sm"
                            onClick={() =>
                              patch(it, {
                                status: skipped ? "planned" : "skipped",
                              })
                            }
                          />
                        )}
                        {day === today &&
                          (tracking ? (
                            <span className="grid size-7 place-items-center text-working" title="Tracking now">
                              <Radio size={15} aria-hidden />
                            </span>
                          ) : (
                            <IconButton icon={Play} label={`Start ${it.task.title}`} tone="secondary" size="sm" disabled={isDone || skipped} onClick={() => t.startTask(it.task)} />
                          ))}
                      </>
                    )}
                  </div>
                  {(it.steps ?? []).length > 0 && <StepList parent={it.task} steps={it.steps} adding={!past} className="mb-2" />}
                </li>
              );
            })}
          </ul>
        )}
        {items.length > 1 && !past && <p className="mt-2 px-2 text-xs text-ink-subtle">Moving, resizing, or reordering a task pins it, so regenerating keeps it where you put it.</p>}
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
