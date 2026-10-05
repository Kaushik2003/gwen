import { ArrowLeft, ArrowRight, CalendarClock, CircleCheck, Clock, Columns3, Flame, Grid2x2, Hand, Pencil, Play, Plus, Trash, UserRound } from "lucide-react";
import { useEffect, useMemo, useState, type DragEvent } from "react";
import { App, wire } from "../api";
import { useCompleteTask } from "../components/complete";
import { stages } from "../components/ScheduleInput";
import { DueTag, EffortTag, PriorityFlag, ProjectTag } from "../components/tags";
import { Button, Field, IconButton, Input, Modal, PageHeader, Segmented, Select, cx } from "../components/ui";
import { useDaemon } from "../daemon";
import { addDays, formatDuration } from "../format";
import { useNav } from "../nav";
import { useTracking } from "../tracking";
import { TaskForm } from "./Projects";

/** The board's columns: the stages, then Done. */
const columns = [...stages.map((s) => ({ id: s.value, label: s.label, hint: s.hint })), { id: "done", label: "Done", hint: "Finished this week" }];

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

/**
 * Board: every task by stage, as a Kanban board you drag cards across, or as
 * the Eisenhower matrix of urgent against important.
 */
export default function Board() {
  const d = useDaemon();
  const { param } = useNav();
  const [view, setView] = useState<"kanban" | "matrix">(param === "matrix" ? "matrix" : "kanban");
  const [project, setProject] = useState("");
  const [done, setDone] = useState<wire.Task[]>([]);
  const [goals, setGoals] = useState<wire.Goal[]>([]);
  const [editing, setEditing] = useState<wire.Task | null>(null);
  const [adding, setAdding] = useState<string | null>(null);
  const today = d.today();

  useEffect(() => {
    const since = Date.now() - 7 * 86_400_000;
    App.ListTasks(wire.TaskQuery.createFrom({ status: "done" })).then((l) => setDone(l.tasks.filter((t) => (t.done_at ?? 0) >= since && !t.parent_id)), d.fail);
  }, [d.tasksVersion, d.fail]);
  useEffect(() => {
    App.ListGoals("active").then((l) => setGoals(l.goals), d.fail);
  }, [d.goalsVersion, d.fail]);

  const tasks = useMemo(
    () => [...d.tasks.filter((t) => !t.parent_id), ...done].filter((t) => !project || (project === "none" ? !t.project_id : t.project_id === project)),
    [d.tasks, done, project],
  );

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Board"
        subtitle={view === "kanban" ? "Drag cards between columns as work moves along. Done this week sits at the end." : "Urgent against important: do, schedule, delegate, or drop."}
        actions={
          <>
            <Select value={project} onChange={(e) => setProject(e.target.value)} aria-label="Project" className="w-44">
              <option value="">All projects</option>
              <option value="none">No project</option>
              {d.projects.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </Select>
            <Segmented
              value={view}
              onChange={setView}
              label="View"
              options={[
                { value: "kanban", label: "Kanban", icon: Columns3 },
                { value: "matrix", label: "Eisenhower", icon: Grid2x2 },
              ]}
            />
          </>
        }
      />
      {view === "kanban" ? <Kanban tasks={tasks} onEdit={setEditing} onAdd={setAdding} /> : <Matrix tasks={tasks.filter((t) => t.status !== "done" && t.stage !== "someday")} today={today} onEdit={setEditing} />}
      {(editing || adding) && (
        <TaskForm
          task={editing}
          projects={d.projects}
          goals={goals}
          defaultProject={project === "none" ? "" : project}
          defaultStage={adding ?? undefined}
          onClose={() => {
            setEditing(null);
            setAdding(null);
          }}
        />
      )}
    </div>
  );
}

/** Moves a task to a column: a stage, or done; out of done reopens it. */
function useMove() {
  const d = useDaemon();
  const complete = useCompleteTask();
  const [asking, setAsking] = useState<wire.Task | null>(null);
  const [who, setWho] = useState("");

  async function move(t: wire.Task, to: string) {
    if (columnOf(t) === to) return;
    if (to === "done") return complete.complete(t);
    if (t.status === "done" && !(await d.act(() => App.ReopenTask(t.id)))) return;
    if (to === "waiting" && !t.delegated_to) {
      setWho("");
      setAsking(t);
      return;
    }
    await d.act(() => App.PatchTask(t.id, wire.PatchTaskRequest.createFrom({ stage: to })));
  }

  const dialog = (
    <>
      {complete.dialog}
      {asking && (
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
                  d.act(() => App.PatchTask(asking.id, wire.PatchTaskRequest.createFrom({ stage: "waiting", delegated_to: who })));
                  setAsking(null);
                }}
              >
                Move to Waiting
              </Button>
            </>
          }
        >
          <Field label="Person or thing" hint="Leave empty when it is simply blocked.">
            <Input value={who} onChange={(e) => setWho(e.target.value)} placeholder="Alex" autoFocus maxLength={200} />
          </Field>
        </Modal>
      )}
    </>
  );
  return { move, dialog };
}

function Kanban({ tasks, onEdit, onAdd }: { tasks: wire.Task[]; onEdit: (t: wire.Task) => void; onAdd: (stage: string) => void }) {
  const { move, dialog } = useMove();
  const [over, setOver] = useState<string | null>(null);
  const [dragging, setDragging] = useState<string | null>(null);
  const byId = useMemo(() => new Map(tasks.map((t) => [t.id, t])), [tasks]);

  function drop(e: DragEvent, to: string) {
    e.preventDefault();
    setOver(null);
    const t = byId.get(e.dataTransfer.getData("text/plain"));
    if (t) move(t, to);
  }

  return (
    <>
      <div className="-mx-4 overflow-x-auto px-4 pb-3 md:-mx-8 md:px-8">
        <div className="grid grid-cols-[repeat(6,minmax(13rem,1fr))] gap-2.5">
          {columns.map((c, ci) => {
            const cards = tasks
              .filter((t) => columnOf(t) === c.id)
              .sort((a, b) => (c.id === "done" ? (b.done_at ?? 0) - (a.done_at ?? 0) : b.priority - a.priority || (a.due_day ?? "9").localeCompare(b.due_day ?? "9")));
            return (
              <section
                key={c.id}
                aria-label={c.label}
                onDragOver={(e) => {
                  e.preventDefault();
                  setOver(c.id);
                }}
                onDragLeave={() => setOver((o) => (o === c.id ? null : o))}
                onDrop={(e) => drop(e, c.id)}
                className={cx("flex h-[max(24rem,calc(100vh-13rem))] min-w-0 flex-col rounded-xl border bg-surface-1/60 transition-colors", over === c.id ? "border-accent bg-accent/8" : "border-line")}
              >
                <header className="shrink-0 px-3.5 pt-3 pb-2">
                  <div className="flex h-7 items-center gap-2">
                    <span className="truncate text-[13px] font-semibold whitespace-nowrap text-ink">{c.label}</span>
                    <span className="rounded-full bg-surface-3 px-1.5 text-xs text-ink-subtle tabular-nums">{cards.length}</span>
                    {c.id !== "done" && <IconButton icon={Plus} label={`New task in ${c.label}`} size="sm" className="-mr-1.5 ml-auto" onClick={() => onAdd(c.id)} />}
                  </div>
                  <p className="truncate text-[11px] text-ink-faint" title={c.hint}>
                    {c.hint}
                  </p>
                </header>
                <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto px-2 pb-2">
                  {cards.map((t) => (
                    <Card
                      key={t.id}
                      task={t}
                      dragging={dragging === t.id}
                      onDragStart={(e) => {
                        e.dataTransfer.setData("text/plain", t.id);
                        e.dataTransfer.effectAllowed = "move";
                        setDragging(t.id);
                      }}
                      onDragEnd={() => setDragging(null)}
                      onEdit={() => onEdit(t)}
                      onLeft={ci > 0 ? () => move(t, columns[ci - 1].id) : undefined}
                      onRight={ci < columns.length - 1 ? () => move(t, columns[ci + 1].id) : undefined}
                    />
                  ))}
                  {cards.length === 0 && <div className="grid flex-1 place-items-center rounded-lg border border-dashed border-line text-xs text-ink-faint">Drop here</div>}
                </div>
              </section>
            );
          })}
        </div>
      </div>
      {dialog}
    </>
  );
}

function Card({
  task: t,
  dragging,
  onDragStart,
  onDragEnd,
  onEdit,
  onLeft,
  onRight,
}: {
  task: wire.Task;
  dragging: boolean;
  onDragStart: (e: DragEvent) => void;
  onDragEnd: () => void;
  onEdit: () => void;
  onLeft?: () => void;
  onRight?: () => void;
}) {
  const d = useDaemon();
  const tr = useTracking();
  const project = d.projects.find((p) => p.id === t.project_id);
  const done = t.status === "done";
  const tracking = tr.isTracking(t.id);
  return (
    <article
      draggable
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
      onDoubleClick={onEdit}
      className={cx(
        "group relative flex shrink-0 cursor-grab flex-col gap-2 rounded-lg border bg-surface-2 p-2.5 pl-3 shadow-sm transition-[opacity,border-color] active:cursor-grabbing",
        tracking ? "border-working/60" : "border-line-strong hover:border-line-3",
        dragging && "opacity-40",
      )}
      style={project ? { boxShadow: `inset 3px 0 0 ${project.color}` } : undefined}
    >
      <div className="flex items-start gap-1.5">
        {t.effort === 3 && !done && <Flame size={13} className="mt-0.5 shrink-0 text-danger" aria-label="A frog: do it first" />}
        <span className={cx("min-w-0 flex-1 text-[13px] leading-snug", done ? "text-ink-subtle line-through" : "text-ink")}>{t.title}</span>
      </div>
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
        {project && <ProjectTag project={project} />}
        {t.due_day && <DueTag day={t.due_day} today={d.today()} done={done} />}
        <PriorityFlag priority={t.priority} />
        <EffortTag effort={t.effort} />
        {t.estimate_minutes != null && (
          <span className="inline-flex items-center gap-1 text-xs text-ink-subtle tabular-nums">
            <Clock size={12} aria-hidden />
            {formatDuration(t.estimate_minutes * 60_000)}
          </span>
        )}
        {t.stage === "waiting" && t.delegated_to && (
          <span className="inline-flex items-center gap-1 text-xs text-ink-subtle">
            <UserRound size={12} aria-hidden />
            {t.delegated_to}
          </span>
        )}
      </div>
      <div className="absolute right-1.5 bottom-1.5 flex items-center gap-0.5 rounded-md border border-line-strong bg-surface-3 p-0.5 opacity-0 shadow-lg shadow-black/40 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100">
        {onLeft && <IconButton icon={ArrowLeft} label="Move left" size="sm" onClick={onLeft} />}
        {onRight && <IconButton icon={ArrowRight} label="Move right" size="sm" onClick={onRight} />}
        {!done && !tracking && (
          <IconButton
            icon={Play}
            label="Start tracking it"
            size="sm"
            onClick={async () => {
              if (t.stage !== "doing") await d.act(() => App.PatchTask(t.id, wire.PatchTaskRequest.createFrom({ stage: "doing" })));
              tr.startTask(t);
            }}
          />
        )}
        {!done && <IconButton icon={Pencil} label="Edit" size="sm" onClick={onEdit} />}
      </div>
    </article>
  );
}

const quadrants = [
  { id: "do", title: "Do first", sub: "Urgent and important", icon: Flame, color: "#eb5757" },
  { id: "schedule", title: "Schedule", sub: "Important, not urgent", icon: CalendarClock, color: "#5e6ad2" },
  { id: "delegate", title: "Delegate", sub: "Urgent, not important", icon: Hand, color: "#f2994a" },
  { id: "drop", title: "Drop", sub: "Neither: someday, or delete", icon: Trash, color: "#8a8f98" },
] as const;

function Matrix({ tasks, today, onEdit }: { tasks: wire.Task[]; today: string; onEdit: (t: wire.Task) => void }) {
  const d = useDaemon();
  const tr = useTracking();
  const { move, dialog } = useMove();
  return (
    <>
      <div className="grid gap-3 @2xl:grid-cols-2">
        {quadrants.map((q) => {
          const list = tasks.filter((t) => quadrantOf(t, today) === q.id).sort((a, b) => (a.due_day ?? "9").localeCompare(b.due_day ?? "9") || b.priority - a.priority);
          const Icon = q.icon;
          return (
            <section key={q.id} className="lift flex min-h-64 flex-col rounded-xl border border-line bg-surface-1" style={{ borderTopColor: q.color, borderTopWidth: 3 }}>
              <header className="flex items-center gap-2.5 px-4 pt-3 pb-2">
                <Icon size={16} style={{ color: q.color }} aria-hidden />
                <div className="min-w-0 flex-1">
                  <div className="text-sm font-semibold text-ink">{q.title}</div>
                  <div className="text-xs text-ink-subtle">{q.sub}</div>
                </div>
                <span className="rounded-full bg-surface-3 px-2 text-xs text-ink-subtle tabular-nums">{list.length}</span>
              </header>
              <ul className="flex flex-col divide-y divide-line px-1 pb-2">
                {list.map((t) => (
                  <li key={t.id} className="group flex items-center gap-2 rounded-md px-3 py-2 hover:bg-surface-2/60">
                    <button type="button" onClick={() => onEdit(t)} className="min-w-0 flex-1 text-left">
                      <span className="block truncate text-[13px] text-ink">{t.title}</span>
                      <span className="mt-0.5 flex flex-wrap items-center gap-x-2 text-xs">
                        <ProjectTag project={d.projects.find((p) => p.id === t.project_id)} />
                        {t.due_day && <DueTag day={t.due_day} today={today} />}
                        <PriorityFlag priority={t.priority} />
                        <EffortTag effort={t.effort} />
                      </span>
                    </button>
                    <span className="flex shrink-0 gap-1">
                      {q.id === "do" && !tr.isTracking(t.id) && (
                        <Button size="sm" icon={Play} onClick={() => tr.startTask(t)}>
                          Start
                        </Button>
                      )}
                      {q.id === "schedule" && (
                        <Button size="sm" icon={CalendarClock} onClick={() => onEdit(t)}>
                          Schedule
                        </Button>
                      )}
                      {q.id === "delegate" && t.stage !== "waiting" && (
                        <Button size="sm" icon={Hand} onClick={() => move(t, "waiting")}>
                          Delegate
                        </Button>
                      )}
                      {q.id === "drop" && (
                        <Button size="sm" onClick={() => move(t, "someday")}>
                          Someday
                        </Button>
                      )}
                      <IconButton icon={CircleCheck} label="Done" size="sm" onClick={() => move(t, "done")} />
                    </span>
                  </li>
                ))}
                {list.length === 0 && <li className="px-3 py-6 text-center text-xs text-ink-faint">Nothing here.</li>}
              </ul>
            </section>
          );
        })}
      </div>
      {dialog}
    </>
  );
}
