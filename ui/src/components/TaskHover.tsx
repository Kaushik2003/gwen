import { Clock, Hourglass, ListChecks, Target } from "lucide-react";
import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import type { wire } from "../api";
import { useDaemon } from "../daemon";
import { formatDuration } from "../format";
import { DueTag, EffortTag, PriorityFlag, ProjectTag, StageTag, StartTag } from "./tags";

const openDelay = 350;
const width = 320;

/**
 * Shows a task's details in a floating card while the pointer rests on its
 * row. The card lives in a portal, so a scrolling panel never clips it.
 */
export default function TaskHover({ task, children, className }: { task: wire.Task; children: ReactNode; className?: string }) {
  const row = useRef<HTMLDivElement>(null);
  const timer = useRef<number | undefined>(undefined);
  const [open, setOpen] = useState(false);
  useEffect(() => () => window.clearTimeout(timer.current), []);
  return (
    <div
      ref={row}
      className={className}
      onMouseEnter={() => {
        window.clearTimeout(timer.current);
        timer.current = window.setTimeout(() => setOpen(true), openDelay);
      }}
      onMouseLeave={() => {
        window.clearTimeout(timer.current);
        setOpen(false);
      }}
      onMouseDown={() => {
        window.clearTimeout(timer.current);
        setOpen(false);
      }}
    >
      {children}
      {open && row.current && createPortal(<Card task={task} anchor={row.current} />, document.body)}
    </div>
  );
}

/** The card, beside the row: right of it when there is room, else left, kept inside the window. */
function Card({ task: given, anchor }: { task: wire.Task; anchor: HTMLElement }) {
  const d = useDaemon();
  const card = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState<{ left: number; top: number } | null>(null);
  // The open list has the freshest copy; its steps are the open ones.
  const task = d.tasks.find((x) => x.id === given.id) ?? given;
  const project = d.projects.find((p) => p.id === task.project_id);
  const steps = d.tasks.filter((x) => x.parent_id === task.id);
  const today = d.today();

  useLayoutEffect(() => {
    const r = anchor.getBoundingClientRect();
    const h = card.current?.offsetHeight ?? 200;
    const gap = 10;
    const right = r.right + gap + width <= window.innerWidth - 8;
    const left = right ? r.right + gap : Math.max(8, r.left - gap - width);
    // Beside a wide row there is no room either side: drop below it instead.
    const beside = right || r.left - gap - width >= 8;
    const top = beside ? Math.min(Math.max(8, r.top - 6), window.innerHeight - h - 8) : Math.min(r.bottom + 6, window.innerHeight - h - 8);
    setPos({ left: beside ? left : Math.min(r.left + 24, window.innerWidth - width - 8), top });
  }, [anchor]);

  const facts: [ReactNode, ReactNode][] = [];
  if (task.estimate_minutes) facts.push([<Hourglass size={12} aria-hidden />, `${formatDuration(task.estimate_minutes * 60_000)} estimated`]);
  if (task.tracked_ms > 0) facts.push([<Clock size={12} aria-hidden />, `${formatDuration(task.tracked_ms)} tracked`]);
  if (task.quantity) facts.push([<Target size={12} aria-hidden />, `${task.quantity_done ?? 0} of ${task.quantity} done`]);
  if (steps.length > 0) facts.push([<ListChecks size={12} aria-hidden />, `${steps.length} ${steps.length === 1 ? "step" : "steps"} left`]);

  return (
    <div
      ref={card}
      role="tooltip"
      className="pointer-events-none fixed z-50 rounded-xl border border-line-strong bg-surface-2 p-3.5 shadow-[0_12px_40px_rgb(0_0_0/0.55)]"
      style={{ width, left: pos?.left ?? -9999, top: pos?.top ?? -9999 }}
    >
      <div className="text-[14px] leading-snug font-semibold text-ink">{task.title}</div>
      <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1">
        <ProjectTag project={project} />
        <StageTag task={task} />
        <PriorityFlag priority={task.priority} />
        <EffortTag effort={task.effort} />
        {task.due_day && <DueTag day={task.due_day} today={today} done={task.status === "done"} />}
        <StartTag task={task} today={today} />
      </div>
      {facts.length > 0 && (
        <ul className="mt-2.5 grid grid-cols-2 gap-x-3 gap-y-1">
          {facts.map(([icon, text], i) => (
            <li key={i} className="flex items-center gap-1.5 text-xs text-ink-muted tabular-nums">
              <span className="text-ink-faint">{icon}</span>
              {text}
            </li>
          ))}
        </ul>
      )}
      {task.notes.trim() && <p className="mt-2.5 line-clamp-6 border-t border-line pt-2.5 text-[12.5px] leading-relaxed whitespace-pre-line text-ink-subtle">{task.notes.trim()}</p>}
      {steps.length > 0 && (
        <ul className="mt-2.5 flex flex-col gap-1 border-t border-line pt-2.5">
          {steps.slice(0, 5).map((s) => (
            <li key={s.id} className="flex items-center gap-2 text-xs text-ink-muted">
              <span className="size-1.5 shrink-0 rounded-full bg-ink-faint" />
              <span className="truncate">{s.title}</span>
            </li>
          ))}
          {steps.length > 5 && <li className="text-xs text-ink-faint">and {steps.length - 5} more</li>}
        </ul>
      )}
    </div>
  );
}
