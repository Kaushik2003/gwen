import { Archive, Bot, CalendarClock, ChevronDown, Circle, CircleCheck, CircleDot, Flame, Hand, Hourglass, Inbox, Pencil, Play, Plus, Trash, Trash2, Undo2, type LucideIcon } from "lucide-react";
import { useEffect, useLayoutEffect, useMemo, useRef, useState, type DragEvent, type KeyboardEvent, type ReactNode, type RefObject } from "react";
import { App, wire } from "../api";
import { useToast } from "../components/feedback";
import { stages } from "../components/ScheduleInput";
import { DueTag, PriorityFlag } from "../components/tags";
import { Button, Field, Input, Modal, cx, unassignedColor } from "../components/ui";
import { useDaemon } from "../daemon";
import { addDays, dayOf, daysBetween, formatClock, formatDate, formatDuration, relativeDay } from "../format";
import { useNav } from "../nav";
import { useClearOfSheet } from "../shell/HandOff";
import { parentOf, projectChoices, projectPath } from "../shell/projects";
import { useTabs } from "../shell/tabs";
import { RowMenu, type MenuItem } from "../task/Menu";
import { stageChoices, stageName, type TaskStatusKind } from "../task/status";
import { StatusPill } from "../task/TaskRow";
import { useTaskUI } from "../task/TaskUI";
import { BoardMenu, type MenuEntry } from "./BoardMenu";

// Plan › Board (plan.md 3.15, board 9): every task by stage as a Kanban board you drag cards across, or as the Eisenhower
// matrix of urgent against important, filtered by project. A card says when an agent is on it; clicking one opens the task
// form, the same one every other screen uses.

/** What a dragged card carries: never text, so no text field takes it as typing. */
const dragType = "application/x-gwen-task";

interface Column {
  id: string;
  label: string;
  hint: string;
  icon: LucideIcon;
  /** What an empty column says. */
  empty: string;
}

const columnIcons: Record<string, LucideIcon> = { inbox: Inbox, todo: Circle, doing: CircleDot, waiting: Hourglass, someday: Archive };
const emptyText: Record<string, string> = { inbox: "Nothing to sort", todo: "Nothing ready yet", doing: "Nothing in progress", waiting: "Nothing waiting", someday: "Nothing parked" };

/** The board's columns: the stages, then Done. */
const columns: Column[] = [
  ...stages.map((s) => ({ id: s.value, label: stageName(s.value), hint: s.hint, icon: columnIcons[s.value] ?? Circle, empty: emptyText[s.value] ?? "Nothing here" })),
  { id: "done", label: stageName("done"), hint: "Finished this week", icon: CircleCheck, empty: "Nothing finished this week" },
];

/** Where a task sits on the board. */
function columnOf(t: wire.Task): string {
  return t.status === "done" ? "done" : t.stage || "todo";
}

/** Eisenhower: urgent is due within two days; important is priority 3 or 4, or tied to a goal. */
export function quadrantOf(t: wire.Task, today: string): "do" | "schedule" | "delegate" | "drop" {
  const urgent = !!t.due_day && t.due_day <= addDays(today, 2);
  const important = t.priority >= 3 || !!t.goal_id;
  return urgent ? (important ? "do" : "delegate") : important ? "schedule" : "drop";
}

type View = "kanban" | "matrix";

/** What the cards read besides the task: its steps and the filter's project. */
interface Context {
  steps: Map<string, { done: number; all: number }>;
  /** The project the board is filtered to, whose name the cards need not repeat. */
  scoped: string;
}

/** The board; with `fixed` it is one project's (with its sub-projects), as on that project's home, and has no project filter. */
export default function Board({ fixed }: { fixed?: string } = {}) {
  const d = useDaemon();
  const ui = useTaskUI();
  const nav = useNav();
  const param = fixed ? null : nav.param;
  const tabs = useTabs();
  const [view, setView] = useState<View>(param === "matrix" ? "matrix" : "kanban");
  // The filter lives in the tab's param ("project:<id>", "project:none"), so a project's Board link opens on that project
  // and opening it again from another project changes it. "" is every project, "none" tasks without one.
  const linked = fixed ?? (param?.startsWith("project:") ? param.slice("project:".length) : "");
  const [project, setFilter] = useState(linked);
  useEffect(() => setFilter(linked), [linked]);
  const setProject = (v: string) => {
    if (fixed) return;
    setFilter(v);
    tabs.update("screen:plan", { kind: "screen", screen: "plan", sub: "board", param: v ? `project:${v}` : null });
  };
  const [doneAll, setDoneAll] = useState<wire.Task[] | null>(null);
  const [doneFailed, setDoneFailed] = useState(false);
  const [reload, setReload] = useState(0);
  const root = useRef<HTMLDivElement>(null);
  const clear = useClearOfSheet(root, !!ui.handing);
  const today = d.today();

  useEffect(() => {
    let live = true;
    App.ListTasks(wire.TaskQuery.createFrom({ status: "done" })).then(
      (l) => {
        if (!live) return;
        setDoneAll(l.tasks ?? []);
        setDoneFailed(false);
      },
      () => live && setDoneFailed(true),
    );
    return () => {
      live = false;
    };
  }, [d.tasksVersion, reload]);
  // A filter on a project that was archived or deleted would show nothing, with no way to tell why.
  useEffect(() => {
    if (!fixed && project && project !== "none" && d.projects.length > 0 && !d.projects.some((p) => p.id === project)) setProject("");
  }, [project, d.projects]);

  const scope = useMemo(() => {
    if (!project || project === "none") return null;
    return new Set([project, ...d.projects.filter((p) => parentOf(p) === project).map((p) => p.id)]);
  }, [project, d.projects]);

  // A repeating task's copy for a later day is not work yet: only today's is on the board. Done keeps this week's.
  const tasks = useMemo(() => {
    const since = d.now() - 7 * 86_400_000;
    const done = (doneAll ?? []).filter((t) => !t.parent_id && (t.done_at ?? 0) >= since);
    return [...d.tasks.filter((t) => !t.parent_id && !(t.occurrence_day && t.occurrence_day > today)), ...done].filter(
      (t) => !project || (project === "none" ? !t.project_id : scope!.has(t.project_id ?? "")),
    );
  }, [d.tasks, d.now, doneAll, project, scope, today]);

  const ctx = useMemo<Context>(() => {
    const steps = new Map<string, { done: number; all: number }>();
    const count = (t: wire.Task, done: boolean) => {
      if (!t.parent_id) return;
      const s = steps.get(t.parent_id) ?? { done: 0, all: 0 };
      s.all++;
      if (done) s.done++;
      steps.set(t.parent_id, s);
    };
    d.tasks.forEach((t) => count(t, false));
    (doneAll ?? []).forEach((t) => count(t, true));
    return { steps, scoped: project };
  }, [d.tasks, doneAll, project]);

  const open = tasks.filter((t) => t.status !== "done").length;
  const finished = tasks.length - open;
  const where = project === "none" ? " with no project" : project ? ` in ${projectPath(d.projects.find((p) => p.id === project), d.projects) || "this project"}` : "";
  const summary =
    view === "matrix"
      ? "Urgent means due within two days; important means high priority or tied to a goal."
      : `${open === 0 ? "Nothing open" : `${open} open`}${where}${doneAll ? ` · ${finished === 0 ? "nothing" : finished} finished this week` : ""}`;

  return (
    <div ref={root} className="flex flex-col gap-3.5" style={clear ? { paddingRight: clear } : undefined}>
      <header className="flex flex-wrap items-center gap-2">
        <p className="min-w-0 flex-[1_1_12rem] truncate text-[12.5px] text-ink-subtle" title={summary}>
          {summary}
        </p>
        {fixed ? (
          <Button size="sm" tone="ghost" onClick={() => tabs.open({ kind: "screen", screen: "plan", sub: "board", param: null })}>
            All projects' board
          </Button>
        ) : (
          <ProjectFilter value={project} onChange={setProject} />
        )}
        <ViewSwitch value={view} onChange={setView} />
        <Button tone="primary" size="sm" icon={Plus} onClick={() => ui.create({ project: project === "none" ? "" : project })}>
          New task
        </Button>
      </header>
      {view === "kanban" ? (
        <Kanban tasks={tasks} ctx={ctx} onAdd={(stage) => ui.create({ project: project === "none" ? "" : project, stage })} done={doneAll ? "ok" : doneFailed ? "failed" : "loading"} onRetry={() => setReload((n) => n + 1)} />
      ) : (
        <Matrix tasks={tasks.filter((t) => t.status !== "done" && t.stage !== "someday")} today={today} ctx={ctx} />
      )}
    </div>
  );
}

/** Which project the board shows: every one, those with none, or one project with its sub-projects. */
function ProjectFilter({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const d = useDaemon();
  const choices = projectChoices(d.projects);
  const current = d.projects.find((p) => p.id === value);
  const label = value === "none" ? "No project" : current ? projectPath(current, d.projects) : "All projects";
  const entries: MenuEntry[] = [
    { label: "All projects", checked: !value, run: () => onChange("") },
    { label: "No project", checked: value === "none", run: () => onChange("none") },
    ...(choices.length ? [{ kind: "divider" } satisfies MenuEntry] : []),
    ...choices.map((c): MenuEntry => ({ label: c.label, color: d.projects.find((p) => p.id === c.id)?.color, checked: value === c.id, run: () => onChange(c.id) })),
  ];
  return (
    <BoardMenu
      label="Project"
      entries={entries}
      align="end"
      trigger={({ open, toggle, ref }) => (
        <button
          ref={ref}
          type="button"
          onClick={toggle}
          aria-haspopup="menu"
          aria-expanded={open}
          title="Show one project's tasks"
          className={cx(
            "inline-flex h-7 max-w-64 min-w-0 items-center gap-1.5 rounded-lg border px-2.5 text-[13px] font-medium transition-colors",
            open ? "border-line-3 bg-surface-3 text-ink" : "border-line-strong bg-surface-2 text-ink hover:border-line-3 hover:bg-surface-3",
          )}
        >
          {current && <span aria-hidden className="size-[7px] shrink-0 rounded-[2px]" style={{ background: current.color }} />}
          <span className="min-w-0 truncate">{label}</span>
          <ChevronDown size={13} className="shrink-0 text-ink-subtle" aria-hidden />
        </button>
      )}
    />
  );
}

function ViewSwitch({ value, onChange }: { value: View; onChange: (v: View) => void }) {
  const options: [View, string][] = [
    ["kanban", "Kanban"],
    ["matrix", "Eisenhower"],
  ];
  return (
    <div role="radiogroup" aria-label="View" className="flex rounded-md border border-line bg-surface-2 p-0.5">
      {options.map(([v, label]) => (
        <button
          key={v}
          type="button"
          role="radio"
          aria-checked={value === v}
          onClick={() => onChange(v)}
          className={cx("h-6 rounded-[5px] px-2.5 text-[12px] font-medium", value === v ? "bg-surface-4 text-ink" : "text-ink-subtle hover:text-ink")}
        >
          {label}
        </button>
      ))}
    </div>
  );
}

/** Moves a task to a column: a stage, or done; out of done reopens it. Waiting asks who it waits on. */
function useMove() {
  const d = useDaemon();
  const ui = useTaskUI();
  const toast = useToast();
  const [asking, setAsking] = useState<wire.Task | null>(null);
  const [who, setWho] = useState("");
  const pending = useRef(new Map<string, { rev: number; run: (task: wire.Task) => void }>());

  // Reopening publishes a new revision through the daemon; moving must use that revision.
  useEffect(() => {
    for (const task of d.tasks) {
      const next = pending.current.get(task.id);
      if (!next || task.status === "done" || task.rev <= next.rev) continue;
      pending.current.delete(task.id);
      next.run(task);
    }
  }, [d.tasks]);

  function afterReopen(t: wire.Task, run: (task: wire.Task) => void) {
    if (t.status !== "done") return run(t);
    const next = { rev: t.rev, run };
    pending.current.set(t.id, next);
    // A failed reopen must not move the task if it is reopened much later elsewhere.
    window.setTimeout(() => {
      if (pending.current.get(t.id) === next) pending.current.delete(t.id);
    }, 30_000);
    ui.reopen(t);
  }

  function move(t: wire.Task, to: string) {
    if (columnOf(t) === to) return;
    if (to === "done") return ui.complete(t);
    if (to === "waiting" && !t.delegated_to) {
      setWho("");
      setAsking(t);
      return;
    }
    afterReopen(t, (task) => ui.move(task, to));
  }

  const dialog = asking && (
    <Modal
      title={`Who is "${asking.title}" waiting on?`}
      size="sm"
      onClose={() => setAsking(null)}
      footer={
        <>
          <Button onClick={() => setAsking(null)}>Cancel</Button>
          <Button
            tone="primary"
            onClick={() => {
              const to = who.trim();
              afterReopen(asking, (task) => {
                d.act(() => App.PatchTask(task.id, wire.PatchTaskRequest.createFrom({ stage: "waiting", delegated_to: to, rev: task.rev }))).then((ok) => {
                  if (ok) toast(to ? `Moved to ${stageName("waiting")}: waiting on ${to}` : `Moved to ${stageName("waiting")}`);
                });
              });
              setAsking(null);
            }}
          >
            Move to {stageName("waiting")}
          </Button>
        </>
      }
    >
      <Field label="Person or thing" hint="Leave it empty when it is simply blocked.">
        <Input value={who} onChange={(e) => setWho(e.target.value)} placeholder="Alex" autoFocus maxLength={200} />
      </Field>
    </Modal>
  );
  return { move, dialog };
}

/**
 * The height a column may take: down to the window's foot, less the status bar (30), the page's bottom padding (32), room for the
 * sideways scrollbar (12), and the hint line under the board with the gap above it (14), so the board and its hint fit the window.
 */
function useFitHeight(ref: RefObject<HTMLElement | null>, foot: RefObject<HTMLElement | null>): number {
  const [h, setH] = useState(0);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const scroller = el.closest<HTMLElement>(".overflow-y-auto");
    const measure = () => {
      const top = el.getBoundingClientRect().top + (scroller?.scrollTop ?? 0);
      const below = 30 + 32 + 14 + 12 + (foot.current?.offsetHeight ?? 18);
      setH(Math.max(280, Math.floor(window.innerHeight - top - below)));
    };
    measure();
    const seen = new ResizeObserver(measure);
    if (el.parentElement?.parentElement) seen.observe(el.parentElement.parentElement);
    if (foot.current) seen.observe(foot.current);
    window.addEventListener("resize", measure);
    return () => {
      seen.disconnect();
      window.removeEventListener("resize", measure);
    };
  }, [ref, foot]);
  return h;
}

/** Which columns are out of sight to the right, and whether there is more to either side, as the board scrolls sideways. */
function useSideways(ref: RefObject<HTMLElement | null>) {
  const [s, setS] = useState({ left: false, right: false, hidden: [] as string[] });
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const measure = () => {
      const box = el.getBoundingClientRect();
      // A column counts as out of sight when less than two thirds of it shows.
      const hidden = [...el.querySelectorAll<HTMLElement>("[data-col]")]
        .filter((c) => {
          const r = c.getBoundingClientRect();
          return Math.min(r.right, box.right) - Math.max(r.left, box.left) < (r.width * 2) / 3;
        })
        .map((c) => c.dataset.col!);
      const next = { left: el.scrollLeft > 2, right: el.scrollLeft + el.clientWidth < el.scrollWidth - 2, hidden };
      setS((p) => (p.left === next.left && p.right === next.right && p.hidden.join() === hidden.join() ? p : next));
    };
    measure();
    const seen = new ResizeObserver(measure);
    seen.observe(el);
    if (el.firstElementChild) seen.observe(el.firstElementChild);
    el.addEventListener("scroll", measure, { passive: true });
    return () => {
      seen.disconnect();
      el.removeEventListener("scroll", measure);
    };
  }, [ref]);
  return s;
}

/** "Someday and Done (finished this week)". */
function listColumns(ids: string[]): string {
  const names = ids.map((id) => (id === "done" ? `${stageName(id)} (finished this week)` : stageName(id)));
  return names.length <= 1 ? names.join("") : `${names.slice(0, -1).join(", ")} and ${names[names.length - 1]}`;
}

function Kanban({
  tasks,
  ctx,
  onAdd,
  done,
  onRetry,
}: {
  tasks: wire.Task[];
  ctx: Context;
  onAdd: (stage: string) => void;
  done: "loading" | "ok" | "failed";
  onRetry: () => void;
}) {
  const { move, dialog } = useMove();
  const [over, setOver] = useState<string | null>(null);
  const [dragging, setDragging] = useState<string | null>(null);
  const byId = useMemo(() => new Map(tasks.map((t) => [t.id, t])), [tasks]);
  const row = useRef<HTMLDivElement>(null);
  const foot = useRef<HTMLParagraphElement>(null);
  const fit = useFitHeight(row, foot);
  const side = useSideways(row);
  // A card moved from the keyboard keeps the focus in its new column.
  const refocus = useRef<{ id: string; col: string } | null>(null);
  useEffect(() => {
    const r = refocus.current;
    if (!r) return;
    const el = row.current?.querySelector<HTMLElement>(`[data-col="${r.col}"] [data-card="${r.id}"]`);
    if (el) {
      el.focus();
      refocus.current = null;
    }
  });

  function drop(e: DragEvent, to: string) {
    e.preventDefault();
    setOver(null);
    setDragging(null);
    const t = byId.get(e.dataTransfer.getData(dragType));
    if (t) move(t, to);
  }

  function shift(t: wire.Task, by: -1 | 1) {
    const at = columns.findIndex((c) => c.id === columnOf(t));
    const to = columns[at + by];
    if (!to) return;
    refocus.current = { id: t.id, col: to.id };
    // A move that fails, or waits on a question, must not pull the focus back later.
    window.setTimeout(() => {
      if (refocus.current?.id === t.id) refocus.current = null;
    }, 3000);
    move(t, to.id);
  }

  return (
    <>
      <div className="relative -mr-7">
        {/* Each column has 4 px of padding for its drop highlight, so the row steps out by as much to keep the cards in line. */}
        <div ref={row} className="-ml-1 flex items-stretch gap-0.5 overflow-x-auto pr-6">
          {columns.map((c) => {
            const cards = tasks
              .filter((t) => columnOf(t) === c.id)
              .sort((a, b) => (c.id === "done" ? (b.done_at ?? 0) - (a.done_at ?? 0) : b.priority - a.priority || (a.due_day ?? "9").localeCompare(b.due_day ?? "9")));
            return (
              <section
                key={c.id}
                data-col={c.id}
                aria-label={c.label}
                onDragEnter={(e) => {
                  if (!dragging) return;
                  e.preventDefault();
                  setOver(c.id);
                }}
                onDragOver={(e) => {
                  if (!dragging) return;
                  e.preventDefault();
                  e.dataTransfer.dropEffect = "move";
                  if (over !== c.id) setOver(c.id);
                }}
                onDragLeave={(e) => {
                  // Only when the pointer leaves the column, not as it crosses into a card: WebKit gives no relatedTarget here.
                  const r = e.currentTarget.getBoundingClientRect();
                  if (e.clientX < r.left || e.clientX >= r.right || e.clientY < r.top || e.clientY >= r.bottom) setOver((o) => (o === c.id ? null : o));
                }}
                onDrop={(e) => drop(e, c.id)}
                className={cx(
                  "flex max-w-73 min-w-46 flex-[1_0_11.5rem] flex-col gap-2 rounded-xl p-1 transition-[background-color,box-shadow]",
                  over === c.id && "bg-accent/[0.04] shadow-[inset_0_0_0_1px_color-mix(in_srgb,var(--color-accent)_50%,transparent)]",
                )}
                style={fit ? { maxHeight: fit } : undefined}
              >
                <header className="shrink-0 px-0.5">
                  <div className="flex h-[22px] items-center gap-1.5">
                    <h2 className="truncate text-[12.5px] font-semibold text-ink">{c.label}</h2>
                    <span className="text-[12.5px] text-ink-subtle tabular-nums">{c.id === "done" && done !== "ok" ? "" : cards.length}</span>
                    {c.id !== "done" && (
                      <button
                        type="button"
                        aria-label={`New task in ${c.label}`}
                        title={`New task in ${c.label}`}
                        onClick={() => onAdd(c.id)}
                        className="-mr-1 ml-auto grid size-[22px] shrink-0 place-items-center rounded-[5px] text-ink-subtle transition-colors hover:bg-surface-2 hover:text-ink"
                      >
                        <Plus size={14} aria-hidden />
                      </button>
                    )}
                  </div>
                  <p className="truncate text-[11px] text-ink-subtle" title={c.hint}>
                    {c.hint}
                  </p>
                </header>
                <div className="-m-[3px] flex min-h-0 flex-col gap-2 overflow-y-auto p-[3px]">
                  {cards.map((t) => (
                    <BoardCard
                      key={t.id}
                      task={t}
                      ctx={ctx}
                      dragging={dragging === t.id}
                      onDragStart={(e) => {
                        e.dataTransfer.setData(dragType, t.id);
                        e.dataTransfer.effectAllowed = "move";
                        setDragging(t.id);
                      }}
                      onDragEnd={() => {
                        setDragging(null);
                        setOver(null);
                      }}
                      onMove={(to) => move(t, to)}
                      onShift={(by) => shift(t, by)}
                    />
                  ))}
                  {cards.length === 0 &&
                    (c.id === "done" && done === "failed" ? (
                      <div className="flex flex-col items-start gap-1.5 rounded-[10px] border border-line px-3 py-2.5 text-[11.5px] text-ink-subtle">
                        Couldn't load finished tasks.
                        <Button size="sm" onClick={onRetry}>
                          Try again
                        </Button>
                      </div>
                    ) : c.id === "done" && done === "loading" ? (
                      <div role="status" className="h-14 animate-pulse rounded-[10px] bg-surface-1" aria-label="Loading finished tasks" />
                    ) : (
                      <div
                        className={cx(
                          "grid h-14 shrink-0 place-items-center rounded-[10px] border border-dashed text-[11.5px]",
                          dragging ? "border-line-strong text-ink-subtle" : "border-line text-ink-faint",
                        )}
                      >
                        {dragging ? "Drop here" : c.empty}
                      </div>
                    ))}
                </div>
              </section>
            );
          })}
        </div>
        {side.left && <span aria-hidden className="pointer-events-none absolute top-0 bottom-0 -left-1 w-10 bg-linear-to-l from-transparent to-canvas" />}
        {side.right && <span aria-hidden className="pointer-events-none absolute top-0 right-0 bottom-0 w-14 bg-linear-to-r from-transparent to-canvas" />}
      </div>
      <p ref={foot} className="text-[11.5px] text-ink-subtle">
        Drag a card to move it, or press Shift and an arrow key on it. Click a card to open it; its ⋯ menu has Move to, Start, Hand off, and Delete.
        {side.hidden.length > 0 && ` Scroll sideways for ${listColumns(side.hidden)}.`}
      </p>
      {dialog}
    </>
  );
}

/** A card's or a row's facts after its title: project, priority, when, length, and steps. */
function Facts({ task: t, ctx }: { task: wire.Task; ctx: Context }) {
  const d = useDaemon();
  const today = d.today();
  const project = d.projects.find((p) => p.id === t.project_id);
  const done = t.status === "done";
  const steps = ctx.steps.get(t.id);
  const facts: ReactNode[] = [];
  if (!project || project.id !== ctx.scoped)
    facts.push(
      <span key="p" className="inline-flex min-w-0 items-center gap-1" title={project ? projectPath(project, d.projects) : "No project"}>
        <span aria-hidden className="size-[7px] shrink-0 rounded-[2px]" style={{ background: project?.color ?? unassignedColor }} />
        <span className="truncate">{project ? projectPath(project, d.projects) : "No project"}</span>
      </span>,
    );
  if (!done && t.priority !== 2) facts.push(<PriorityFlag key="pr" priority={t.priority} />);
  // A finished task is not late any more.
  if (t.due_day && !done) facts.push(<DueTag key="due" day={t.due_day} today={today} />);
  if (!done && t.start_day && (t.start_day > today || t.start_minute != null)) {
    const n = daysBetween(today, t.start_day);
    const day = n === 0 ? "Today" : n === 1 ? "Tomorrow" : formatDate(t.start_day, false);
    facts.push(<span key="st">{t.start_minute != null ? `${day} ${formatClock(t.start_minute)}` : n > 1 ? `From ${day}` : day}</span>);
  }
  if (t.estimate_minutes) facts.push(<span key="est" className="tabular-nums">{formatDuration(t.estimate_minutes * 60_000)}</span>);
  if (steps && steps.all > 0) facts.push(<span key="steps" className="tabular-nums">{`${steps.done} of ${steps.all} ${steps.all === 1 ? "step" : "steps"}`}</span>);
  if (done && t.done_at) facts.push(<span key="dn">Finished {relativeDay(dayOf(t.done_at, d.config?.tracking.day_rollover ?? "04:00"), today)}</span>);
  return facts.length ? <>{facts}</> : null;
}

/** The small square buttons in a card's hover bar. */
function BarButton({ icon: Icon, label, onClick }: { icon: LucideIcon; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      onClick={onClick}
      className="grid size-6 place-items-center rounded-[5px] text-ink-muted transition-colors hover:bg-surface-4 hover:text-ink"
    >
      <Icon size={13} strokeWidth={2.2} aria-hidden />
    </button>
  );
}

function BoardCard({
  task: t,
  ctx,
  dragging,
  onDragStart,
  onDragEnd,
  onMove,
  onShift,
}: {
  task: wire.Task;
  ctx: Context;
  dragging: boolean;
  onDragStart: (e: DragEvent) => void;
  onDragEnd: () => void;
  onMove: (to: string) => void;
  onShift: (by: -1 | 1) => void;
}) {
  const ui = useTaskUI();
  const tabs = useTabs();
  const done = t.status === "done";
  const status = ui.statusOf(t);
  const tracking = status.kind === "tracking";
  const terminal = ui.agentTerminal(t);
  const col = columnOf(t);
  const frog = t.effort === 3 && !done;
  const canStart = !done && !tracking;
  // Hand off and Take it back show exactly where a task row shows them.
  const canHand = !done && !status.live && status.kind !== "delegated";
  const canTakeBack = !done && (status.kind === "delegated" || status.kind === "agent-stopped" || status.kind === "agent-finished");

  const items: MenuItem[] = [
    { heading: "Move to" },
    ...columns.filter((c) => c.id !== col).map((c) => ({ label: stageChoices.find((s) => s.value === c.id)?.label ?? c.label, icon: c.icon, onSelect: () => onMove(c.id) })),
    "divider",
    ...(canStart ? [{ label: "Start", icon: Play, onSelect: () => ui.start(t) }] : []),
    { label: "Open the task", icon: Pencil, onSelect: () => ui.edit(t) },
    ...(canTakeBack ? [{ label: "Take it back", icon: Undo2, onSelect: () => ui.takeBack(t) }] : canHand ? [{ label: "Hand it to an agent", icon: Bot, onSelect: () => ui.handOff(t) }] : []),
    "divider",
    { label: "Delete", icon: Trash2, danger: true, onSelect: () => ui.remove(t) },
  ];

  function onKey(e: KeyboardEvent<HTMLDivElement>) {
    if (e.target !== e.currentTarget) return;
    const el = e.currentTarget;
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      ui.edit(t);
    } else if (e.shiftKey && (e.key === "ArrowLeft" || e.key === "ArrowRight")) {
      e.preventDefault();
      onShift(e.key === "ArrowLeft" ? -1 : 1);
    } else if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      const next = e.key === "ArrowDown" ? el.nextElementSibling : el.previousElementSibling;
      if (next instanceof HTMLElement && next.dataset.card) next.focus();
    } else if (e.key === "ArrowLeft" || e.key === "ArrowRight") {
      e.preventDefault();
      // The next column with a card in it.
      for (let s = el.closest("[data-col]"); s; ) {
        s = e.key === "ArrowLeft" ? s.previousElementSibling : s.nextElementSibling;
        const first = s?.querySelector<HTMLElement>("[data-card]");
        if (first) return first.focus();
      }
    }
  }

  return (
    <div
      data-card={t.id}
      role="button"
      tabIndex={0}
      aria-label={t.title}
      draggable
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
      onClick={() => ui.edit(t)}
      onKeyDown={onKey}
      className={cx(
        "group relative shrink-0 cursor-pointer rounded-[10px] border px-[11px] py-2.5 text-left transition-[border-color,background-color,opacity] focus-visible:outline-offset-0 active:cursor-grabbing",
        tracking ? "border-working/45 bg-surface-1" : "border-line bg-surface-1 hover:border-line-strong",
        dragging && "opacity-40",
      )}
    >
      <div className="flex items-start gap-1.5">
        {frog && <Flame size={13} className="mt-[2px] shrink-0 text-danger" aria-label="Hard: do it first" role="img" />}
        <span className={cx("min-w-0 flex-1 text-[12.5px] leading-[1.4] [overflow-wrap:anywhere]", done ? "text-ink-subtle line-through" : frog ? "font-medium text-ink" : "text-ink")}>{t.title}</span>
      </div>
      <FactsRow>
        <Facts task={t} ctx={ctx} />
      </FactsRow>
      {!saidByColumn.includes(status.kind) &&
        (terminal ? (
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation();
              tabs.open({ kind: "term", project: t.project_id ?? "", terminal });
            }}
            aria-label="Open its Claude session"
            title="Open its Claude session"
            className={cx(stripClass, "w-full text-left hover:underline")}
          >
            <StatusPill status={status} />
          </button>
        ) : (
          <div className={stripClass}>
            <StatusPill status={status} />
          </div>
        ))}
      <div
        onClick={(e) => e.stopPropagation()}
        className={cx(
          "absolute top-1.5 right-1.5 flex items-center gap-px rounded-md border border-line-strong bg-surface-3 p-0.5 shadow-lg shadow-black/40 transition-opacity",
          dragging ? "hidden" : "pointer-events-none opacity-0 group-focus-within:pointer-events-auto group-focus-within:opacity-100 group-hover:pointer-events-auto group-hover:opacity-100 has-[[aria-expanded=true]]:pointer-events-auto has-[[aria-expanded=true]]:opacity-100",
        )}
      >
        {canStart && <BarButton icon={Play} label={`Start ${t.title}`} onClick={() => ui.start(t)} />}
        {canHand && <BarButton icon={Bot} label={`Hand ${t.title} to an agent`} onClick={() => ui.handOff(t)} />}
        <RowMenu label={`More for ${t.title}`} items={items} className="size-6! rounded-[5px]! hover:bg-surface-4 [&_svg]:size-[13px]" />
      </div>
    </div>
  );
}

/** Statuses a card's column already says; its foot line is for what the column can't: tracking, an agent on it, who it waits on. */
const saidByColumn: TaskStatusKind[] = ["done", "inbox", "todo", "doing", "waiting", "someday"];

/** The line at a card's foot that says what it is doing. */
const stripClass = "mt-2 flex items-start gap-1.5 border-t border-chrome-line pt-[7px] text-[11px] leading-[1.35]";

/** The facts line under a card's title; nothing when there are none. */
function FactsRow({ children }: { children: ReactNode }) {
  return <div className="mt-[7px] flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-ink-subtle empty:hidden">{children}</div>;
}

const quadrants = [
  { id: "do", title: "Do first", sub: "Urgent and important", icon: Flame, tone: "text-danger" },
  { id: "schedule", title: "Schedule", sub: "Important, not urgent", icon: CalendarClock, tone: "text-info" },
  { id: "delegate", title: "Delegate", sub: "Urgent, not important", icon: Hand, tone: "text-warning" },
  { id: "drop", title: "Drop", sub: `Neither: move it to ${stageName("someday")}, or delete it`, icon: Trash, tone: "text-ink-subtle" },
] as const;

function Matrix({ tasks, today, ctx }: { tasks: wire.Task[]; today: string; ctx: Context }) {
  const ui = useTaskUI();
  const tabs = useTabs();
  const { move, dialog } = useMove();
  return (
    <>
      <div className="grid items-start gap-3 @2xl:grid-cols-2">
        {quadrants.map((q) => {
          const list = tasks.filter((t) => quadrantOf(t, today) === q.id).sort((a, b) => (a.due_day ?? "9").localeCompare(b.due_day ?? "9") || b.priority - a.priority);
          const Icon = q.icon;
          return (
            <section key={q.id} aria-label={q.title} className="lift min-w-0 overflow-hidden rounded-xl border border-line bg-surface-1">
              <header className="flex h-11 items-center gap-2 border-b border-chrome-line pr-3 pl-4">
                <Icon size={14} className={cx("shrink-0", q.tone)} aria-hidden />
                <h2 className="shrink-0 text-[13px] font-semibold text-ink">{q.title}</h2>
                <span className="text-[12.5px] text-ink-subtle tabular-nums">{list.length}</span>
                <span className="ml-auto min-w-0 truncate text-[12px] text-ink-subtle">{q.sub}</span>
              </header>
              {list.length === 0 ? (
                <p className="px-4 py-4 text-[12.5px] text-ink-faint">Nothing here.</p>
              ) : (
                <ul className="max-h-[26rem] overflow-y-auto">
                  {list.map((t, i) => {
                    const terminal = ui.agentTerminal(t);
                    const status = ui.statusOf(t);
                    const tracking = status.kind === "tracking";
                    return (
                      <li key={t.id} className={cx("group flex items-center gap-2 py-2 pr-2.5 pl-4 hover:bg-surface-2/40", i > 0 && "border-t border-chrome-line")}>
                        <button type="button" onClick={() => ui.edit(t)} className="min-w-0 flex-1 text-left">
                          <span className="flex items-center gap-1.5">
                            {t.effort === 3 && <Flame size={13} className="shrink-0 text-danger" aria-label="Hard: do it first" role="img" />}
                            <span className="truncate text-[13px] text-ink">{t.title}</span>
                          </span>
                          <span className="mt-0.5 flex flex-wrap items-center gap-x-2.5 gap-y-1 text-[12px] text-ink-subtle empty:hidden">
                            <Facts task={t} ctx={ctx} />
                            {status.kind !== "todo" && <StatusPill status={status} />}
                          </span>
                        </button>
                        <span className="flex shrink-0 items-center gap-1">
                          <span className="flex items-center gap-1 opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100">
                            {terminal && <Button size="sm" tone="ghost" onClick={() => tabs.open({ kind: "term", project: t.project_id ?? "", terminal })}>Open</Button>}
                            {!status.live && status.kind !== "delegated" && <BarIcon icon={Bot} label={`Hand ${t.title} to an agent`} onClick={() => ui.handOff(t)} />}
                            <BarIcon icon={CircleCheck} label={`Complete ${t.title}`} onClick={() => ui.complete(t)} />
                          </span>
                          {q.id === "do" && !tracking && (
                            <Button size="sm" tone="ghost" icon={Play} onClick={() => ui.start(t)}>
                              Start
                            </Button>
                          )}
                          {q.id === "schedule" && (
                            <Button size="sm" tone="ghost" icon={CalendarClock} onClick={() => ui.edit(t)}>
                              Schedule
                            </Button>
                          )}
                          {q.id === "delegate" && t.stage !== "waiting" && (
                            <Button size="sm" tone="ghost" icon={Hand} onClick={() => move(t, "waiting")}>
                              Delegate
                            </Button>
                          )}
                          {q.id === "drop" && (
                            <Button size="sm" tone="ghost" icon={Archive} onClick={() => move(t, "someday")}>
                              {stageName("someday")}
                            </Button>
                          )}
                        </span>
                      </li>
                    );
                  })}
                </ul>
              )}
            </section>
          );
        })}
      </div>
      {dialog}
    </>
  );
}

/** A row's quiet icon button, 28 px like the rest of the window's small buttons. */
function BarIcon({ icon: Icon, label, onClick }: { icon: LucideIcon; label: string; onClick: () => void }) {
  return (
    <button type="button" aria-label={label} title={label} onClick={onClick} className="grid size-7 place-items-center rounded-lg text-ink-subtle transition-colors hover:bg-surface-3 hover:text-ink">
      <Icon size={14} aria-hidden />
    </button>
  );
}
