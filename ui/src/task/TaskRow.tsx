import { ArrowRightLeft, Bot, CalendarPlus, CalendarX, ChevronRight, ListPlus, Paperclip, Pencil, Play, Repeat, RotateCcw, SkipForward, SquareTerminal, Trash2, Undo2 } from "lucide-react";
import { useState, type LiHTMLAttributes, type ReactNode } from "react";
import type { wire } from "../api";
import { StepList } from "../components/steps";
import { DueTag, PriorityFlag } from "../components/tags";
import TaskHover from "../components/TaskHover";
import { Checkbox, IconButton, Meter, cx, unassignedColor } from "../components/ui";
import { useDaemon } from "../daemon";
import { formatDuration, relativeDay } from "../format";
import { blockTimes, type PlanGroup } from "../plan";
import { projectPath } from "../shell/projects";
import { useTabs } from "../shell/tabs";
import { RowMenu, type MenuItem } from "./Menu";
import { stageChoices, type StatusTone, type TaskStatus, type TaskStatusKind } from "./status";
import { dayWords, useTaskUI } from "./TaskUI";

// One task row for every list in the window: a tick, the title (click to open the task form), what it is
// doing now in words, the facts that matter (project, today's time, priority, due, steps, repeats), and the
// same actions everywhere: Start, Hand off, and ⋯ for edit, move, and delete.

export interface TaskRowProps {
  task: wire.Task;
  /** Its blocks on today's plan, when it has some: the time, the minutes, carried over, skipped. */
  plan?: PlanGroup | null;
  /** Show its project in the meta line; off in lists that are already one project's. */
  showProject?: boolean;
  /** Extra left padding, to nest it under a sub-project. */
  indent?: number;
  /** A column before the tick, such as Today's time column. */
  lead?: ReactNode;
  /** The first row in its list: no hairline above it. */
  first?: boolean;
  /** For narrow lists (the side panel): one short meta line, Start and ⋯ only. */
  compact?: boolean;
  /** Screen-specific controls before the actions, such as Plan's start time and minutes. */
  controls?: ReactNode;
  /** Statuses the list already says by itself (the Inbox needn't say "To process" on every row). */
  quietStatus?: TaskStatusKind[];
  /** Attributes for the row itself, such as Plan's drag and drop. */
  liProps?: LiHTMLAttributes<HTMLLIElement>;
  /** How far the steps sit in from the row's padding, when there is a lead (default: Today's time column). */
  leadWidth?: number;
  /** Its steps show unfolded from the start, as in a goal's task list. */
  stepsOpen?: boolean;
}

const tones: Record<StatusTone, string> = {
  working: "bg-working/12 text-working",
  warning: "bg-warning/12 text-warning",
  info: "bg-info/12 text-info",
  success: "bg-success/12 text-success",
  accent: "bg-accent/12 text-accent-hover",
  muted: "bg-surface-3 text-ink-muted",
  done: "bg-surface-3 text-ink-faint",
};

/** What a task is doing now, as a small pill with a dot that pulses while something runs. */
export function StatusPill({ status, className }: { status: TaskStatus; className?: string }) {
  return (
    <span className={cx("inline-flex h-[18px] shrink-0 items-center gap-1.5 rounded-full px-1.5 text-[11px] font-medium whitespace-nowrap", tones[status.tone], className)}>
      <span className={cx("size-1.5 rounded-full bg-current", status.live && "animate-live motion-reduce:animate-none")} />
      {status.label}
    </span>
  );
}

export default function TaskRow({ task, plan, showProject = true, indent = 0, lead, first, compact, controls, quietStatus = [], liProps, leadWidth = 72, stepsOpen = false }: TaskRowProps) {
  const d = useDaemon();
  const ui = useTaskUI();
  const tabs = useTabs();
  const today = d.today();
  const status = ui.statusOf(task);
  // Today's plan knows every step, done ones too; otherwise only the open ones are loaded.
  const counted = !!plan?.steps.length;
  const steps = counted ? plan!.steps : ui.stepsOf(task);
  const [open, setOpen] = useState<boolean | null>(null);
  const unfolded = open ?? (stepsOpen || status.kind === "tracking");
  const isDone = task.status === "done" || !!plan?.done;
  // Lists of tasks carry the count; a task inside a plan block doesn't, so the window's open tasks fill it in.
  const attached = task.attachments || d.tasks.find((x) => x.id === task.id)?.attachments || 0;
  const project = d.projects.find((p) => p.id === task.project_id);
  const minutes = plan?.minutes || task.estimate_minutes || 0;
  const terminal = ui.agentTerminal(task);
  const showStatus = status.kind !== "todo" && !quietStatus.includes(status.kind);
  const stepsDone = steps.filter((s) => s.status === "done").length;
  const live = status.live || status.kind === "agent-finished";
  // The task you are tracking can go to an agent too; one an agent already works on can't.
  const handable = !isDone && (!status.live || status.kind === "tracking") && status.kind !== "delegated";
  // Plan rows can be on another day; everything that says "today" says that day instead.
  const onDay = plan ? dayWords(plan, today) : "today";

  const menu: MenuItem[] = isDone
    ? [
        { label: "Reopen", icon: RotateCcw, onSelect: () => ui.reopen(task) },
        { label: "Open the task", icon: Pencil, onSelect: () => ui.edit(task) },
        "divider",
        { label: "Delete", icon: Trash2, danger: true, onSelect: () => ui.remove(task) },
      ]
    : [
        { label: "Open the task", icon: Pencil, onSelect: () => ui.edit(task) },
        ...(status.kind === "delegated" || status.kind === "agent-stopped" || status.kind === "agent-finished"
          ? [{ label: "Take it back", icon: Undo2, onSelect: () => ui.takeBack(task) }]
          : handable
            ? [{ label: "Hand it to an agent", icon: Bot, onSelect: () => ui.handOff(task) }]
            : []),
        ...(steps.length === 0 ? [{ label: "Add steps", icon: ListPlus, onSelect: () => setOpen(true) }] : []),
        ...(plan
          ? plan.skipped
            ? [{ label: onDay === "today" ? "Put it back on today" : `Put it back ${onDay}`, icon: CalendarPlus, onSelect: () => ui.putBack(plan) }]
            : [
                { label: `Skip it ${onDay}`, icon: SkipForward, onSelect: () => ui.skipToday(plan) },
                { label: onDay === "today" ? "Take it off today's plan" : `Take it off the plan ${onDay}`, icon: CalendarX, onSelect: () => ui.removeFromToday(plan) },
              ]
          : []),
        { heading: "Move to" },
        ...stageChoices.filter((s) => s.value !== task.stage).map((s) => ({ label: s.label, icon: ArrowRightLeft, onSelect: () => ui.move(task, s.value) })),
        "divider",
        { label: "Delete", icon: Trash2, danger: true, onSelect: () => ui.remove(task) },
      ];

  const when =
    plan?.start != null && !controls ? (
      <span className="tabular-nums">{blockTimes(plan)}</span>
    ) : plan?.skipped ? (
      <span>Skipped {onDay}</span>
    ) : plan && plan.start == null && !isDone ? (
      <span className="text-idle">No room {onDay}</span>
    ) : task.start_day && task.start_day > today ? (
      <span className="tabular-nums">Starts {relativeDay(task.start_day, today)}</span>
    ) : null;

  return (
    <li {...liProps} className={cx(!first && "border-t border-chrome-line", live && "bg-surface-2/40", liProps?.className)}>
      <TaskHover task={task} className={cx("group flex items-start gap-3 pr-3 hover:bg-surface-2/40", compact ? "py-1.5" : "py-2", !!controls && "flex-wrap")}>
        <div className="flex shrink-0 items-start gap-3" style={{ paddingLeft: 16 + indent }}>
          {lead}
          <Checkbox checked={isDone} disabled={plan?.skipped} onChange={(on) => (on ? ui.complete(task) : ui.reopen(task))} label={isDone ? `Reopen ${task.title}` : `Complete ${task.title}`} />
        </div>
        {/* With a screen's own controls the title keeps 14rem; past that the controls wrap under it, to the right. */}
        <div className={cx("min-w-0 flex-1", !!controls && "basis-56")}>
          <div className="flex min-w-0 items-center gap-2">
            <button
              type="button"
              onClick={() => ui.edit(task)}
              title={task.title}
              className={cx("min-w-0 truncate text-left leading-5 hover:underline hover:decoration-line-3 hover:underline-offset-2", compact ? "text-[13px]" : "text-[13.5px]", isDone ? "text-ink-faint line-through" : "text-ink")}
            >
              {task.title}
            </button>
            {showStatus && !compact && <StatusPill status={status} />}
          </div>
          <div className="mt-0.5 flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-xs text-ink-subtle">
            {/* In a narrow list the status rides on the meta line, so the title keeps the width. */}
            {showStatus && compact && <StatusPill status={status} />}
            {showProject && (
              <span className="inline-flex min-w-0 items-center gap-1.5">
                <span className="size-[7px] shrink-0 rounded-[2px]" style={{ backgroundColor: project?.color ?? unassignedColor }} />
                <span className="truncate">{project ? projectPath(project, d.projects) : "No project"}</span>
              </span>
            )}
            {when}
            {!compact && <PriorityFlag priority={task.priority} />}
            {task.due_day && <DueTag day={task.due_day} today={today} done={isDone} />}
            {attached > 0 && (
              <button type="button" onClick={() => ui.edit(task)} className="-mx-1 inline-flex items-center gap-1 rounded px-1 tabular-nums hover:bg-surface-3 hover:text-ink" title="Context attached: open the task to see it">
                <Paperclip size={11} aria-hidden />
                {attached}
              </button>
            )}
            {!compact && (task.rrule || task.template_id) && (
              <span className="inline-flex items-center gap-1" title="Repeats">
                <Repeat size={11} aria-hidden />
                Repeats
              </span>
            )}
            {steps.length > 0 && (
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  setOpen(!unfolded);
                }}
                aria-expanded={unfolded}
                className="-mx-1 flex shrink-0 items-center gap-1.5 rounded px-1 tabular-nums hover:bg-surface-3 hover:text-ink"
              >
                <ChevronRight size={12} className={cx("transition-transform", unfolded && "rotate-90")} aria-hidden />
                {counted ? (
                  <>
                    <span className="w-8 shrink-0">
                      <Meter value={stepsDone / steps.length} height={4} label={`${stepsDone} of ${steps.length} steps done`} />
                    </span>
                    {steps.length === 1 ? (stepsDone ? "1 step, done" : "1 step") : `${stepsDone} of ${steps.length} steps`}
                  </>
                ) : (
                  `${steps.length} ${steps.length === 1 ? "step" : "steps"} to do`
                )}
              </button>
            )}
            {!compact && (plan?.rollover ?? 0) > 0 && (
              <span className="text-idle tabular-nums" title={`Carried over from earlier days ${plan!.rollover} times`}>
                Carried over {plan!.rollover}×
              </span>
            )}
            {!compact && !isDone && task.tracked_ms > 0 && <span className="tabular-nums">{formatDuration(task.tracked_ms)} tracked</span>}
          </div>
        </div>
        {controls}
        <span className="flex shrink-0 items-center gap-0.5 self-center">
          {!compact && !controls && minutes > 0 && <span className="mr-1.5 text-[12px] text-ink-subtle tabular-nums">{formatDuration(minutes * 60_000)}</span>}
          {terminal && (
            <IconButton size="sm" icon={SquareTerminal} label="Open its Claude session" onClick={() => tabs.open({ kind: "term", project: task.project_id ?? "", terminal })} />
          )}
          {handable && !compact && <IconButton size="sm" icon={Bot} label={`Hand ${task.title} to an agent`} onClick={() => ui.handOff(task)} />}
          {!isDone && status.kind !== "tracking" && !plan?.skipped && <IconButton size="sm" tone="secondary" icon={Play} label={`Start ${task.title}`} onClick={() => ui.start(task)} />}
          <RowMenu label={`More for ${task.title}`} items={menu} />
        </span>
      </TaskHover>
      {unfolded && (steps.length > 0 || (open && !isDone)) && (
        <div className="mr-4 mb-2" style={{ marginLeft: 16 + indent + (lead ? leadWidth : 0) }}>
          <StepList parent={task} steps={steps} adding={!isDone} />
        </div>
      )}
    </li>
  );
}
