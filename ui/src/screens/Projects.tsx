import { Archive, ArchiveRestore, CheckSquare, ChevronDown, ChevronRight, FolderKanban, Layers, ListChecks, ListPlus, Pencil, Play, Plus, Radio, Repeat, SlidersHorizontal, Target, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import { App, wire } from "../api";
import { projectPalette } from "../components/color";
import { useCompleteTask } from "../components/complete";
import DurationInput from "../components/DurationInput";
import { useConfirm, useToast } from "../components/feedback";
import Recurrence from "../components/Recurrence";
import ScheduleInput, { atOn, efforts, stages, type Block } from "../components/ScheduleInput";
import StartInput from "../components/StartInput";
import { StepList } from "../components/steps";
import { DueTag, EffortTag, PriorityFlag, ProjectTag, StageTag, StartTag } from "../components/tags";
import { Badge, Button, Checkbox, Dot, Empty, Field, IconButton, Input, Modal, PageHeader, Panel, Segmented, Select, TextArea, Toggle, cx, priorities } from "../components/ui";
import { useDaemon } from "../daemon";
import { formatDuration, goDuration, parseDuration } from "../format";
import { describeRule } from "../rrule";
import { useTracking } from "../tracking";

export default function Projects() {
  const d = useDaemon();
  const confirm = useConfirm();
  const notify = useToast();
  const done = useCompleteTask();
  const [projects, setProjects] = useState<wire.Project[]>([]);
  const [goals, setGoals] = useState<wire.Goal[]>([]);
  const [showArchived, setShowArchived] = useState(false);
  const [newName, setNewName] = useState("");
  const [selected, setSelected] = useState<string>("");
  const [status, setStatus] = useState("open");
  const [tasks, setTasks] = useState<wire.Task[] | null>(null);
  const [editing, setEditing] = useState<wire.Task | "new" | null>(null);
  const [editingProject, setEditingProject] = useState<wire.Project | null>(null);
  // Select mode: the rows' controls pick tasks for one bulk delete.
  const [picking, setPicking] = useState(false);
  const [picked, setPicked] = useState<Set<string>>(new Set());
  const [openQueues, setOpenQueues] = useState<Set<string>>(new Set());
  const [stepping, setStepping] = useState<Set<string>>(new Set());

  useEffect(() => {
    App.ListProjects(showArchived ? "all" : "false").then((l) => setProjects(l.projects), d.fail);
  }, [showArchived, d.projectsVersion, d.fail]);
  // Every status is fetched, so a parent's done steps still show under it.
  useEffect(() => {
    App.ListTasks(
      wire.TaskQuery.createFrom({
        project_id: selected,
        status: "all",
        due_before: "",
        templates: true,
      }),
    ).then((l) => setTasks(l.tasks), d.fail);
  }, [selected, d.tasksVersion, d.fail]);
  useEffect(() => {
    App.ListGoals("all").then((l) => setGoals(l.goals), d.fail);
  }, [d.goalsVersion, d.fail]);
  useEffect(() => {
    setPicked(new Set());
  }, [selected, status, picking]);

  const today = d.today();
  const patchProject = (p: wire.Project, fields: Record<string, unknown>) => d.act(() => App.PatchProject(p.id, wire.PatchProjectRequest.createFrom({ ...fields, rev: p.rev })));

  const all = tasks ?? [];
  const goalOf = (x: wire.Task) => goals.find((g) => g.id === x.goal_id);
  const listed = new Set(all.map((x) => x.id));
  const stepsOf = (id: string) => all.filter((x) => x.parent_id === id);
  // A quantity goal's open items wait in its queue, folded into one row.
  const isItem = (x: wire.Task) => goalOf(x)?.kind === "quantity" && !x.rrule && !x.template_id && !x.parent_id && x.status === "open";
  // Counts leave out steps and lined-up items, which live under their parent or goal.
  const openCount = (id?: string) => d.tasks.filter((x) => (id === undefined || x.project_id === id) && !x.parent_id && !isItem(x)).length;
  const shown = (x: wire.Task) => status === "all" || x.status === status;
  const top = all.filter((x) => !(x.parent_id && listed.has(x.parent_id)) && !isItem(x) && shown(x));
  const queues = goals
    .map((g) => ({
      goal: g,
      items: all.filter((x) => isItem(x) && x.goal_id === g.id),
    }))
    .filter((q) => q.items.length > 0 && status !== "done");
  const visible = [...top.flatMap((x) => [x, ...stepsOf(x.id)]), ...queues.flatMap((q) => (openQueues.has(q.goal.id) ? q.items : []))];

  async function addProject(e: React.FormEvent) {
    e.preventDefault();
    const color = projectPalette[projects.length % projectPalette.length];
    const p = await d.act(() => App.CreateProject(wire.CreateProjectRequest.createFrom({ name: newName.trim(), color })));
    if (p) {
      setNewName("");
      setSelected(p.id);
    }
  }
  async function removeProject(p: wire.Project) {
    const n = openCount(p.id);
    const g = goals.filter((x) => x.project_id === p.id).length;
    const parts = [n > 0 && `${n} open ${n === 1 ? "task" : "tasks"}`, g > 0 && `${g} ${g === 1 ? "goal" : "goals"}`].filter(Boolean).join(" and ");
    const ok = await confirm({
      title: `Delete ${p.name}?`,
      body: `${parts ? `Its ${parts} go too, with everything planned for them. ` : ""}Time already tracked on it stays in your history.`,
      confirm: "Delete project",
      danger: true,
    });
    if (ok && (await d.run(() => App.DeleteProject(p.id)))) {
      if (selected === p.id) setSelected("");
      notify(`Deleted ${p.name}`, "success");
    }
  }
  async function removeTask(x: wire.Task) {
    const steps = stepsOf(x.id).length;
    const g = goalOf(x);
    let what = "Time already tracked on it stays in your history.";
    if (x.rrule) what = "It stops repeating, and its open copies leave your plan. " + what;
    else if (x.template_id && g?.kind === "quantity" && steps > 0) what = `Its ${steps} ${g.unit || "units"} go back to the goal's queue for the next session.`;
    else if (steps > 0) what = `Its ${steps} ${steps === 1 ? "step goes" : "steps go"} too. ` + what;
    const ok = await confirm({
      title: `Delete ${x.title}?`,
      body: what,
      confirm: "Delete task",
      danger: true,
    });
    if (ok) d.run(() => App.DeleteTask(x.id));
  }
  async function removePicked() {
    const ids = [...picked];
    const ok = await confirm({
      title: `Delete ${ids.length} ${ids.length === 1 ? "task" : "tasks"}?`,
      body: "Their steps and repeats go too. Time already tracked stays in your history.",
      confirm: `Delete ${ids.length}`,
      danger: true,
    });
    if (!ok) return;
    const out = await d.act(() => App.DeleteTasks(wire.DeleteTasksRequest.createFrom({ ids })));
    if (!out) return;
    notify(`Deleted ${out.task_ids.length} ${out.task_ids.length === 1 ? "task" : "tasks"}`, "success");
    setPicking(false);
  }
  const pick = (id: string) =>
    setPicked((p) => {
      const n = new Set(p);
      if (n.has(id)) n.delete(id);
      else n.add(id);
      return n;
    });
  const flip = (set: Set<string>, id: string) => {
    const n = new Set(set);
    if (n.has(id)) n.delete(id);
    else n.add(id);
    return n;
  };

  const row = (x: wire.Task, step = false) => (
    <TaskRow
      key={x.id}
      x={x}
      step={step}
      goal={goalOf(x)}
      project={!selected ? (projects.find((p) => p.id === x.project_id) ?? d.projects.find((p) => p.id === x.project_id)) : undefined}
      today={today}
      picking={picking}
      picked={picked.has(x.id)}
      onPick={() => pick(x.id)}
      onComplete={(on) => (on ? done.complete(x, stepsOf(x.id).length > 0) : done.reopen(x))}
      onEdit={() => setEditing(x)}
      onDelete={() => removeTask(x)}
      onStep={step || x.rrule ? undefined : () => setStepping((s) => flip(s, x.id))}
    />
  );

  const current = projects.find((p) => p.id === selected);
  return (
    <div className="flex flex-col gap-5">
      <PageHeader title="Projects & tasks" subtitle="Projects colour your time. Tasks are what you plan and track." />
      <div className="grid items-start gap-5 @3xl:grid-cols-[15rem_minmax(0,1fr)]">
        <Panel
          title="Projects"
          icon={FolderKanban}
          bodyClassName="px-2 pt-1 pb-3"
          actions={
            <label className="flex items-center gap-2 text-xs text-ink-subtle">
              Archived
              <Toggle checked={showArchived} onChange={setShowArchived} label="Show archived projects" />
            </label>
          }
        >
          <ul className="flex flex-col gap-0.5">
            <ProjectItem name="All projects" color="var(--color-ink-faint)" selected={selected === ""} onSelect={() => setSelected("")} count={openCount()} />
            {projects.map((p) => (
              <ProjectItem
                key={p.id}
                name={p.name}
                color={p.color}
                archived={!!p.archived_at}
                selected={selected === p.id}
                onSelect={() => setSelected(p.id)}
                count={openCount(p.id)}
                onEdit={() => setEditingProject(p)}
              />
            ))}
          </ul>
          <form className="mt-3 flex gap-2 px-1" onSubmit={addProject}>
            <Input className="min-w-0 flex-1" placeholder="New project" value={newName} onChange={(e) => setNewName(e.target.value)} aria-label="New project name" />
            <IconButton icon={Plus} label="Add project" tone="secondary" type="submit" disabled={!newName.trim()} />
          </form>
        </Panel>

        <Panel
          title={current ? current.name : "All tasks"}
          icon={ListChecks}
          bodyClassName="px-2 pt-2 pb-3"
          actions={
            picking ? (
              <>
                <span className="text-xs text-ink-subtle tabular-nums">{picked.size} selected</span>
                <Button size="sm" tone="ghost" onClick={() => setPicked(picked.size === visible.length ? new Set() : new Set(visible.map((x) => x.id)))}>
                  {picked.size === visible.length && visible.length > 0 ? "Select none" : "Select all"}
                </Button>
                <Button size="sm" tone="destroy" icon={Trash2} disabled={picked.size === 0} onClick={removePicked}>
                  Delete {picked.size || ""}
                </Button>
                <Button size="sm" onClick={() => setPicking(false)}>
                  Done
                </Button>
              </>
            ) : (
              <>
                {current && (
                  <>
                    <IconButton icon={Pencil} label={`Edit ${current.name}`} size="sm" onClick={() => setEditingProject(current)} />
                    <IconButton
                      icon={current.archived_at ? ArchiveRestore : Archive}
                      label={current.archived_at ? "Unarchive" : "Archive: hide it from pickers"}
                      size="sm"
                      onClick={() =>
                        patchProject(current, {
                          archived: !current.archived_at,
                        })
                      }
                    />
                    <IconButton icon={Trash2} label={`Delete ${current.name}`} size="sm" onClick={() => removeProject(current)} />
                  </>
                )}
                <Segmented
                  value={status}
                  onChange={setStatus}
                  size="sm"
                  label="Show"
                  options={[
                    { value: "open", label: "Open" },
                    { value: "done", label: "Done" },
                    { value: "all", label: "All" },
                  ]}
                />
                {all.length > 0 && (
                  <Button size="sm" icon={CheckSquare} onClick={() => setPicking(true)}>
                    Select
                  </Button>
                )}
                <Button size="sm" tone="primary" icon={Plus} onClick={() => setEditing("new")}>
                  New task
                </Button>
              </>
            )
          }
        >
          {tasks && top.length === 0 && queues.length === 0 ? (
            <Empty
              icon={ListChecks}
              title={status === "done" ? "Nothing done here yet" : "No tasks"}
              className="mx-2"
              action={
                <Button icon={Plus} onClick={() => setEditing("new")}>
                  New task
                </Button>
              }
            >
              {status !== "done" && "Add a task, or let the assistant break a goal into tasks."}
            </Empty>
          ) : (
            <ul className="flex flex-col">
              {top.map((x) => {
                const steps = stepsOf(x.id);
                return (
                  <li key={x.id}>
                    <ul>{row(x)}</ul>
                    {picking
                      ? steps.length > 0 && <ul className="ml-[1.85rem] border-l border-line pl-2">{steps.map((s) => row(s, true))}</ul>
                      : (steps.length > 0 || stepping.has(x.id)) && <StepList parent={x} steps={steps} adding={stepping.has(x.id) || steps.length > 0} className="mb-1.5" />}
                  </li>
                );
              })}
              {queues.map(({ goal: g, items }) => {
                const open = openQueues.has(g.id);
                const unit = g.unit || "units";
                const units = items.reduce((n, x) => n + (x.quantity ?? 1), 0);
                return (
                  <li key={`queue-${g.id}`} className="mt-1">
                    <button
                      type="button"
                      onClick={() => setOpenQueues((s) => flip(s, g.id))}
                      aria-expanded={open}
                      className="flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left hover:bg-surface-2/60"
                    >
                      {open ? <ChevronDown size={16} className="text-ink-subtle" aria-hidden /> : <ChevronRight size={16} className="text-ink-subtle" aria-hidden />}
                      <Layers size={15} className="text-accent-hover" aria-hidden />
                      <span className="min-w-0 flex-1 truncate text-sm text-ink">Lined up for {g.title}</span>
                      <span className="text-xs text-ink-subtle tabular-nums">
                        {units} {unit}
                        {g.progress.per_session ? `, about ${Math.ceil(units / g.progress.per_session)} sessions` : ""}
                      </span>
                    </button>
                    {open && (
                      <ul className="ml-[1.85rem] border-l border-line pl-2">
                        <li className="px-3 pb-1 text-xs text-ink-subtle">Each day's session takes the next ones from the top. Edit or delete any of them.</li>
                        {items.map((x) => row(x, true))}
                      </ul>
                    )}
                  </li>
                );
              })}
            </ul>
          )}
        </Panel>
      </div>
      {editing && (
        <TaskForm
          task={editing === "new" ? null : editing}
          projects={projects}
          goals={goals.filter((g) => g.status === "active" || g.id === (editing === "new" ? "" : editing.goal_id))}
          defaultProject={selected}
          onClose={() => setEditing(null)}
        />
      )}
      {editingProject && <ProjectForm project={editingProject} onClose={() => setEditingProject(null)} />}
      {done.dialog}
    </div>
  );
}

/** One task in the list: its controls, or a pick box in select mode. */
function TaskRow({
  x,
  step,
  goal,
  project,
  today,
  picking,
  picked,
  onPick,
  onComplete,
  onEdit,
  onDelete,
  onStep,
}: {
  x: wire.Task;
  step: boolean;
  goal?: wire.Goal;
  project?: wire.Project;
  today: string;
  picking: boolean;
  picked: boolean;
  onPick: () => void;
  onComplete: (on: boolean) => void;
  onEdit: () => void;
  onDelete: () => void;
  onStep?: () => void;
}) {
  const t = useTracking();
  const isDone = x.status === "done";
  const template = !!x.rrule;
  const tracking = t.isTracking(x.id);
  return (
    <li className={cx("group flex items-start gap-3 rounded-lg px-3", step ? "py-1.5" : "py-2.5", tracking ? "bg-surface-2" : "hover:bg-surface-2/60")}>
      <span className="pt-0.5">
        {picking ? (
          <Checkbox square checked={picked} onChange={onPick} label={`Select ${x.title}`} />
        ) : template ? (
          <span className="grid size-4.5 place-items-center text-ink-subtle" title="Repeating: each day's copy is what you complete">
            <Repeat size={14} aria-hidden />
          </span>
        ) : (
          <Checkbox checked={isDone} onChange={onComplete} label={isDone ? `Reopen ${x.title}` : `Complete ${x.title}`} />
        )}
      </span>
      <button type="button" className="min-w-0 flex-1 text-left" onClick={picking ? onPick : onEdit} tabIndex={-1}>
        <div className={cx(step ? "text-[13px]" : "text-sm", isDone ? "text-ink-faint line-through" : "text-ink")}>{x.title}</div>
        {x.notes && <div className={cx("mt-0.5 text-xs whitespace-pre-line text-ink-subtle", step ? "line-clamp-2" : "line-clamp-1")}>{x.notes}</div>}
        {!step && (
          <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1">
            {project && <ProjectTag project={project} />}
            {goal && (
              <span className="inline-flex items-center gap-1 text-xs text-ink-subtle">
                <Target size={12} aria-hidden />
                {goal.title}
                {x.quantity != null && ` (${x.quantity} ${goal.unit || "units"})`}
              </span>
            )}
            {template && <Badge icon={Repeat}>{describeRule(x.rrule!)}</Badge>}
            {!isDone && <StartTag task={x} today={today} />}
            {x.due_day && <DueTag day={x.due_day} today={today} done={isDone} />}
            <PriorityFlag priority={x.priority} />
            <EffortTag effort={x.effort} />
            <StageTag task={x} />
            {x.estimate_minutes != null && <span className="text-xs text-ink-subtle">{formatDuration(x.estimate_minutes * 60_000)} estimated</span>}
          </div>
        )}
      </button>
      {(x.tracked_ms > 0 || (step && x.estimate_minutes != null)) && (
        <span className="shrink-0 pt-0.5 text-right text-xs text-ink-subtle tabular-nums" title={x.tracked_ms > 0 ? "Tracked on this task" : "Estimated"}>
          {formatDuration(x.tracked_ms > 0 ? x.tracked_ms : x.estimate_minutes! * 60_000)}
        </span>
      )}
      {!picking && (
        <span className="flex shrink-0 items-center gap-0.5">
          {!template &&
            !isDone &&
            !step &&
            (tracking ? (
              <span className="grid size-7 place-items-center text-working" title="Tracking now">
                <Radio size={15} aria-hidden />
              </span>
            ) : (
              <IconButton icon={Play} label={`Start ${x.title}`} size="sm" onClick={() => t.startTask(x)} />
            ))}
          {onStep && !isDone && <IconButton icon={ListPlus} label={`Add a step to ${x.title}`} size="sm" onClick={onStep} />}
          <IconButton icon={Pencil} label={`Edit ${x.title}`} size="sm" onClick={onEdit} />
          <IconButton icon={Trash2} label={`Delete ${x.title}`} size="sm" onClick={onDelete} />
        </span>
      )}
    </li>
  );
}

function ProjectItem({
  name,
  color,
  selected,
  onSelect,
  count,
  archived,
  onEdit,
}: {
  name: string;
  color: string;
  selected: boolean;
  onSelect: () => void;
  count: number;
  archived?: boolean;
  onEdit?: () => void;
}) {
  return (
    <li className={cx("group flex items-center rounded-md", selected ? "bg-surface-2 shadow-[inset_0_0_0_1px_var(--color-line)]" : "hover:bg-surface-2/60")}>
      <button type="button" onClick={onSelect} aria-current={selected || undefined} className="flex h-9 min-w-0 flex-1 items-center gap-2.5 px-2.5 text-left">
        <Dot color={color} size={9} />
        <span className={cx("min-w-0 flex-1 truncate text-sm", archived ? "text-ink-faint" : selected ? "text-ink" : "text-ink-muted")}>{name}</span>
        {archived ? <span className="text-xs text-ink-faint">archived</span> : count > 0 && <span className="text-xs text-ink-faint tabular-nums">{count}</span>}
      </button>
      {onEdit && <IconButton icon={Pencil} label={`Edit ${name}`} size="sm" onClick={onEdit} className="mr-1 opacity-0 group-hover:opacity-100 focus-visible:opacity-100" />}
    </li>
  );
}

function ProjectForm({ project: p, onClose }: { project: wire.Project; onClose: () => void }) {
  const d = useDaemon();
  const [name, setName] = useState(p.name);
  const [color, setColor] = useState(p.color);
  async function save() {
    const ok = await d.act(() =>
      App.PatchProject(
        p.id,
        wire.PatchProjectRequest.createFrom({
          name: name.trim(),
          color,
          rev: p.rev,
        }),
      ),
    );
    if (ok) onClose();
  }
  return (
    <Modal
      title="Edit project"
      size="sm"
      onClose={onClose}
      footer={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button tone="primary" onClick={save} disabled={!name.trim()}>
            Save project
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-5">
        <Field label="Name">
          <Input value={name} onChange={(e) => setName(e.target.value)} autoFocus />
        </Field>
        <Field label="Colour" compound>
          <div className="flex flex-wrap items-center gap-2">
            {projectPalette.map((c) => (
              <button
                key={c}
                type="button"
                aria-label={c}
                aria-pressed={color.toLowerCase() === c}
                onClick={() => setColor(c)}
                className={cx("size-7 rounded-full transition-transform hover:scale-110", color.toLowerCase() === c && "ring-2 ring-ink ring-offset-2 ring-offset-surface-1")}
                style={{ backgroundColor: c }}
              />
            ))}
            <label className="ml-1 flex items-center gap-2 text-xs text-ink-subtle" title="Any colour">
              <input type="color" className="swatch" value={color} onChange={(e) => setColor(e.target.value)} />
              Custom
            </label>
          </div>
        </Field>
      </div>
    </Modal>
  );
}

export function TaskForm({
  task,
  projects,
  goals,
  defaultProject,
  defaultStage,
  onClose,
}: {
  task: wire.Task | null;
  projects: wire.Project[];
  goals: wire.Goal[];
  defaultProject: string;
  defaultStage?: string;
  onClose: () => void;
}) {
  const d = useDaemon();
  const today = d.today();
  const [title, setTitle] = useState(task?.title ?? "");
  // A new task starts today; how long it takes and the rest wait under More.
  const [startDay, setStartDay] = useState(task ? (task.start_day ?? "") : today);
  const [startMinute, setStartMinute] = useState<number | null>(task?.start_minute ?? null);
  const [more, setMore] = useState(!!task);
  const [project, setProject] = useState(task ? (task.project_id ?? "") : defaultProject);
  const [goal, setGoal] = useState(task?.goal_id ?? "");
  const [priority, setPriority] = useState(task?.priority ?? 2);
  const [due, setDue] = useState(task?.due_day ?? "");
  const [estimate, setEstimate] = useState(goDuration((task?.estimate_minutes ?? 0) * 60_000));
  const [quantity, setQuantity] = useState(task?.quantity != null ? String(task.quantity) : "");
  const [notes, setNotes] = useState(task?.notes ?? "");
  const [rule, setRule] = useState(task?.rrule ?? "");
  const [effort, setEffort] = useState(task?.effort ?? 0);
  const [stage, setStage] = useState(task?.stage ?? defaultStage ?? "todo");
  const [delegatedTo, setDelegatedTo] = useState(task?.delegated_to ?? "");
  // A time set by hand: a pinned block on the plan.
  const [block, setBlock] = useState<Block | null>(null);
  const [saving, setSaving] = useState(false);
  // Occurrences of a repeating task cannot repeat themselves.
  const canRepeat = !task?.template_id;
  const goalKind = goals.find((g) => g.id === goal)?.kind;

  async function save() {
    if (!title.trim()) return;
    setSaving(true);
    const minutes = Math.round(parseDuration(estimate) / 60_000);
    const at = startDay && startMinute != null ? startMinute : null;
    const blockMinutes = block ? Math.round(parseDuration(block.length) / 60_000) : 0;
    const schedule = block && blockMinutes > 0 ? { day: block.day, start_at: atOn(block.day, block.minute), planned_minutes: blockMinutes } : null;
    const fields = {
      title,
      start_day: startDay || null,
      start_minute: at,
      project_id: project || null,
      priority,
      due_day: due || null,
      estimate_minutes: minutes > 0 ? minutes : null,
      notes,
      goal_id: goal || null,
      quantity: goalKind === "quantity" && Number(quantity) > 0 ? Number(quantity) : null,
      effort: effort || null,
      stage,
      delegated_to: stage === "waiting" ? delegatedTo : "",
    };
    const repeat = canRepeat && rule !== (task?.rrule ?? "") ? { rrule: rule || null } : {};
    const ok = task
      ? await d.act(() =>
          App.PatchTask(
            task.id,
            wire.PatchTaskRequest.createFrom({
              ...fields,
              ...repeat,
              rev: task.rev,
            }),
          ),
        )
      : await d.act(() =>
          App.CreateTask(
            wire.CreateTaskRequest.createFrom({
              ...fields,
              effort: effort || undefined,
              start_day: startDay || undefined,
              start_minute: at ?? undefined,
              rrule: rule || undefined,
              schedule: rule ? undefined : (schedule ?? undefined),
            }),
          ),
        );
    // Gwen adds new and changed tasks to today's plan by itself; a time set by hand on an existing task moves it there.
    if (ok && task && schedule && !rule) await d.act(() => App.ScheduleTask(wire.ScheduleRequest.createFrom({ task_id: task.id, ...schedule })));
    setSaving(false);
    if (ok) onClose();
  }

  return (
    <Modal
      title={task ? "Edit task" : "New task"}
      size={more ? "lg" : "md"}
      onClose={onClose}
      footer={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button tone="primary" onClick={save} disabled={!title.trim()} busy={saving}>
            {task ? "Save task" : "Create task"}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-5">
        <Field label="Title">
          <Input
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                save();
              }
            }}
            placeholder="Call the bank"
            autoFocus
          />
        </Field>
        {!block && <Field label="From" compound hint={startMinute != null ? undefined : "Add a time if it should start at one."}>
          <StartInput
            day={startDay}
            minute={startMinute}
            today={today}
            onChange={(day, minute) => {
              setStartDay(day);
              setStartMinute(minute);
            }}
          />
        </Field>}
        <Field label="How long" compound hint={parseDuration(estimate) > 0 ? "Gwen plans time for it." : "Leave at 0 for a quick to-do you just tick off. Set a time to have Gwen plan it."}>
          <DurationInput
            value={estimate}
            onChange={(v) => {
              setEstimate(v);
              if (block && parseDuration(v) > 0) setBlock({ ...block, length: v });
            }}
            label="How long"
          />
        </Field>
        {!rule && (
          <Field
            label={task ? "Put it on the plan" : "When"}
            compound
            hint={block ? "Blocked on your plan at exactly this time; everything else is planned around it." : task ? "Set a time to move it there; it leaves any other day it was on." : "Gwen puts it on today's plan by urgency, or set the time yourself."}
          >
            <ScheduleInput value={block} today={today} onChange={(b) => setBlock(b && !block && parseDuration(estimate) > 0 ? { ...b, length: estimate } : b)} />
          </Field>
        )}
        <Field label="How hard" compound hint={effort === 3 ? "Eat the frog: hard work goes first in your day, in your prime time when you set one." : undefined}>
          <Segmented value={effort} onChange={setEffort} label="How hard" className="self-start" options={efforts.map((e) => ({ value: e.value, label: e.label }))} />
        </Field>
        {!more ? (
          <Button tone="ghost" size="sm" icon={SlidersHorizontal} className="self-start" onClick={() => setMore(true)}>
            More: notes, project, goal, priority, due date, stage, repeat
          </Button>
        ) : (
          <>
          <Field label="Notes">
            <TextArea rows={2} value={notes} onChange={(e) => setNotes(e.target.value)} />
          </Field>
          <div className="grid grid-cols-2 gap-4">
            <Field label="Project">
              <Select value={project} onChange={(e) => setProject(e.target.value)}>
                <option value="">None</option>
                {projects
                  .filter((p) => !p.archived_at || p.id === project)
                  .map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                    </option>
                  ))}
              </Select>
            </Field>
            <Field label="Goal" hint={goals.length === 0 ? "No active goals." : undefined}>
              <Select value={goal} onChange={(e) => setGoal(e.target.value)}>
                <option value="">None</option>
                {goals.map((g) => (
                  <option key={g.id} value={g.id}>
                    {g.title}
                  </option>
                ))}
              </Select>
            </Field>
            <Field label="Priority" compound className="col-span-2">
              <Segmented
                value={priority}
                onChange={setPriority}
                label="Priority"
                className="self-start"
                options={priorities.map((p) => ({
                  value: p.value,
                  label: (
                    <span
                      style={{
                        color: p.value === priority ? p.color : undefined,
                      }}
                    >
                      {p.label}
                    </span>
                  ),
                }))}
              />
            </Field>
            <Field label="Due">
              <Input type="date" value={due} onChange={(e) => setDue(e.target.value)} />
            </Field>
            <Field label="Stage" hint={stages.find((s) => s.value === stage)?.hint}>
              <Select value={stage} onChange={(e) => setStage(e.target.value)}>
                {stages.map((s) => (
                  <option key={s.value} value={s.value}>
                    {s.label}
                  </option>
                ))}
              </Select>
            </Field>
            {stage === "waiting" && (
              <Field label="Waiting on" hint="Who you handed it to, or what it waits for.">
                <Input value={delegatedTo} onChange={(e) => setDelegatedTo(e.target.value)} placeholder="Alex" maxLength={200} />
              </Field>
            )}
            {goalKind === "quantity" && (
              <Field label="Covers how many of the goal" hint="Completing it asks how many you actually did.">
                <Input type="number" min="1" value={quantity} onChange={(e) => setQuantity(e.target.value)} className="w-28" />
              </Field>
            )}
          </div>
          {canRepeat && (
            <Field label="Repeats" compound hint={rule ? "A repeating task puts a fresh copy on each matching day's plan." : undefined}>
              <Recurrence value={rule} onChange={setRule} anchorDay={startDay || today} allowNone />
            </Field>
          )}
          </>
        )}
      </div>
    </Modal>
  );
}
