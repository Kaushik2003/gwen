import { Archive, ArchiveRestore, CheckSquare, ChevronDown, ChevronRight, ExternalLink, FolderKanban, FolderOpen, FolderPlus, Layers, ListChecks, Pencil, Plus, Repeat, Search, SlidersHorizontal, SquareKanban, Target, Trash2, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { App, wire } from "../api";
import { projectPalette } from "../components/color";
import DurationInput from "../components/DurationInput";
import { useConfirm, useToast } from "../components/feedback";
import Recurrence from "../components/Recurrence";
import ScheduleInput, { atOn, efforts, stages, type Block } from "../components/ScheduleInput";
import StartInput from "../components/StartInput";
import { StepList } from "../components/steps";
import { DueTag, EffortTag, PriorityFlag, ProjectTag, StartTag } from "../components/tags";
import { Badge, Button, Checkbox, Field, IconButton, Input, Modal, Segmented, Select, TextArea, Toggle, cx, priorities } from "../components/ui";
import { useDaemon } from "../daemon";
import { useNav } from "../nav";
import { Card, ClaudeMark, Eyebrow, tildify, useHome } from "../shell/bits";
import { useClearOfSheet } from "../shell/HandOff";
import { parentOf, projectChoices, projectPath, projectTree } from "../shell/projects";
import { useTabs } from "../shell/tabs";
import { useWork, type Term } from "../shell/work";
import { AttachmentsField, type PendingAttachment } from "../task/Attachments";
import { RowMenu, type MenuItem } from "../task/Menu";
import TaskRow, { StatusPill } from "../task/TaskRow";
import { stageChoices } from "../task/status";
import { useTaskUI } from "../task/TaskUI";
import { addDays, formatDate, formatDuration, goDuration, parseDuration, relativeDay } from "../format";
import { describeRule } from "../rrule";

// Manage projects (plan.md 3.15): every project with its folder, this week's time and live sessions on the left; the
// one you pick on the right, with what it adds up to, its folder and sub-projects, and its tasks. Tasks use the window's
// one task row and task form (ui/src/task); projects are made and edited in one project form.

/** Which projects the list is showing: "" is every project, "none" tasks without one, else a project id. */
type Scope = string;

export default function Projects() {
  const d = useDaemon();
  const confirm = useConfirm();
  const notify = useToast();
  const ui = useTaskUI();
  const w = useWork();
  const tabs = useTabs();
  const home = useHome();
  const { param } = useNav();
  const page = useRef<HTMLDivElement>(null);
  const sheetPad = useClearOfSheet(page, !!ui.handing);
  const [projects, setProjects] = useState<wire.Project[]>([]);
  const [goals, setGoals] = useState<wire.Goal[]>([]);
  const [showArchived, setShowArchived] = useState(false);
  // The pick lives in the tab's param, so a link can open a project here and the tab remembers it.
  const [selected, setSelected] = useState<Scope>(param ?? "");
  useEffect(() => setSelected(param ?? ""), [param]);
  const select = (id: Scope) => {
    setSelected(id);
    tabs.update("screen:projects", { kind: "screen", screen: "projects", param: id || null });
  };
  const [status, setStatus] = useState("open");
  const [query, setQuery] = useState("");
  const [tasks, setTasks] = useState<wire.Task[] | null>(null);
  const [form, setForm] = useState<{ project: wire.Project | null; parent?: string } | null>(null);
  const [week, setWeek] = useState<Map<string, number>>(new Map());
  // Select mode: the rows' controls pick tasks for one bulk move or delete.
  const [picking, setPicking] = useState(false);
  const [picked, setPicked] = useState<Set<string>>(new Set());
  const [openQueues, setOpenQueues] = useState<Set<string>>(new Set());
  const today = d.today();

  // Archived ones too, so their tasks still have a name; the list hides them until asked.
  useEffect(() => {
    App.ListProjects("all").then((l) => setProjects(l.projects), d.fail);
  }, [d.projectsVersion, d.fail]);
  // Every status is fetched, so a parent's done steps still show under it.
  useEffect(() => {
    App.ListTasks(wire.TaskQuery.createFrom({ project_id: "", status: "all", due_before: "", templates: true })).then((l) => setTasks(l.tasks), d.fail);
  }, [d.tasksVersion, d.fail]);
  useEffect(() => {
    App.ListGoals("all").then((l) => setGoals(l.goals), d.fail);
  }, [d.goalsVersion, d.fail]);
  // Time worked in the last seven days, per project.
  useEffect(() => {
    let live = true;
    App.StatsSummary(addDays(today, -6), today).then(
      (s) => live && setWeek(new Map((s.by_project ?? []).map((p) => [p.project_id ?? "", p.worked_ms]))),
      () => live && setWeek(new Map()),
    );
    return () => {
      live = false;
    };
  }, [today, d.daysVersion]);
  useEffect(() => {
    setPicked(new Set());
  }, [selected, status, picking, query]);

  const known = [...projects, ...d.projects.filter((p) => !projects.some((x) => x.id === p.id))];
  const liveProjects = known.filter((p) => !p.archived_at);
  const archived = known.filter((p) => p.archived_at);
  const current = known.find((p) => p.id === selected);
  // A project with its sub-projects: what every count and total on this screen adds up.
  const scopeOf = (id: Scope): Set<string> | null => (!id || id === "none" ? null : new Set([id, ...known.filter((p) => parentOf(p) === id).map((p) => p.id)]));
  const inScope = (x: { project_id?: string | null }, id: Scope) => {
    if (!id) return true;
    if (id === "none") return !x.project_id || !known.some((p) => p.id === x.project_id);
    return scopeOf(id)!.has(x.project_id ?? "");
  };
  const goalOf = (x: wire.Task) => goals.find((g) => g.id === x.goal_id);
  // A quantity goal's open items wait in its queue, folded into one row.
  const isItem = (x: wire.Task) => goalOf(x)?.kind === "quantity" && !x.rrule && !x.template_id && !x.parent_id && x.status === "open";
  // Counts leave out steps, lined-up items and copies of repeating tasks for later days.
  const openOf = (id: Scope) => d.tasks.filter((x) => inScope(x, id) && !x.parent_id && !isItem(x) && !(x.occurrence_day && x.occurrence_day > today));
  const weekOf = (id: Scope) => (id === "none" ? (week.get("") ?? 0) : id ? [...scopeOf(id)!].reduce((n, p) => n + (week.get(p) ?? 0), 0) : [...week.values()].reduce((n, v) => n + v, 0));
  const sessionsOf = (id: Scope) => (id && id !== "none" ? [...scopeOf(id)!].flatMap((p) => w.termsOf(p).filter((t) => t.claude)) : []);
  const since = d.now() - 7 * 86_400_000;
  const doneOf = (id: Scope) => (tasks ?? []).filter((x) => inScope(x, id) && !x.parent_id && x.status === "done" && (x.done_at ?? 0) >= since).length;
  const goalsIn = (id: Scope) => goals.filter((g) => g.status === "active" && (id ? (id === "none" ? !g.project_id : scopeOf(id)!.has(g.project_id ?? "")) : true));

  // The task list: the picked scope, the status filter, and the search.
  const q = query.trim().toLowerCase();
  const matches = (x: wire.Task) => !q || x.title.toLowerCase().includes(q) || (x.notes ?? "").toLowerCase().includes(q);
  const all = (tasks ?? []).filter((x) => inScope(x, selected) && matches(x));
  const stepsOf = (id: string) => (tasks ?? []).filter((x) => x.parent_id === id);
  const shown = (x: wire.Task) => status === "all" || x.status === status;
  // A repeating task's copies sit under it, so the task and today's copy never read as two of the same task.
  const shownIds = new Set(all.filter(shown).map((x) => x.id));
  const copiesOf = (id: string) => all.filter((x) => !x.parent_id && x.template_id === id && shown(x)).sort((a, b) => (a.occurrence_day ?? "").localeCompare(b.occurrence_day ?? ""));
  const top = all.filter((x) => !x.parent_id && !(x.template_id && shownIds.has(x.template_id)) && !isItem(x) && shown(x));
  const queues = goals.map((g) => ({ goal: g, items: all.filter((x) => isItem(x) && x.goal_id === g.id) })).filter((q) => q.items.length > 0 && status !== "done");
  const visible = [...top.flatMap((x) => [x, ...stepsOf(x.id), ...copiesOf(x.id).flatMap((c) => [c, ...stepsOf(c.id)])]), ...queues.flatMap((q) => (openQueues.has(q.goal.id) ? q.items : []))];
  const copyLabel = (c: wire.Task) => {
    if (!c.occurrence_day) return undefined;
    const rel = relativeDay(c.occurrence_day, today);
    return ["today", "tomorrow", "yesterday"].includes(rel) ? rel[0].toUpperCase() + rel.slice(1) : formatDate(c.occurrence_day);
  };

  const patchProject = (p: wire.Project, fields: Record<string, unknown>) => d.act(() => App.PatchProject(p.id, wire.PatchProjectRequest.createFrom({ ...fields, rev: p.rev })));
  async function chooseFolder(p: wire.Project) {
    const path = await App.PickProjectFolder(p.folder || home).catch((e) => {
      d.fail(e);
      return "";
    });
    if (path && (await patchProject(p, { folder: path }))) notify(`${p.name} now lives in ${tildify(path, home)}`, "success");
  }
  async function toggleArchive(p: wire.Project) {
    if (p.archived_at) {
      if (await patchProject(p, { archived: false })) notify(`${p.name} is back`, "success");
      return;
    }
    const ok = await confirm({ title: `Archive ${p.name}?`, body: "It leaves the sidebar and the pickers. Its tasks, time, and folder stay, and you can bring it back here.", confirm: "Archive" });
    if (ok && (await patchProject(p, { archived: true }))) {
      tabs.close(`project:${p.id}`);
      if (!showArchived && selected === p.id) select("");
      notify(`Archived ${p.name}`, "success");
    }
  }
  async function removeProject(p: wire.Project) {
    const n = d.tasks.filter((x) => x.project_id === p.id && !x.parent_id).length;
    const g = goals.filter((x) => x.project_id === p.id).length;
    const subs = known.filter((x) => parentOf(x) === p.id).length;
    const parts = [n > 0 && `${n} open ${n === 1 ? "task" : "tasks"}`, g > 0 && `${g} ${g === 1 ? "goal" : "goals"}`].filter(Boolean).join(" and ");
    const ok = await confirm({
      title: `Delete ${p.name}?`,
      body: `${parts ? `Its ${parts} go too, with everything planned for them. ` : ""}${subs > 0 ? `Its ${subs} ${subs === 1 ? "sub-project moves" : "sub-projects move"} to the top level. ` : ""}Time already tracked on it stays in your history.`,
      confirm: "Delete project",
      danger: true,
    });
    if (ok && (await d.run(() => App.DeleteProject(p.id)))) {
      tabs.close(`project:${p.id}`);
      if (selected === p.id) select("");
      notify(`Deleted ${p.name}`, "success");
    }
  }
  const openBoard = (id: Scope) => tabs.open({ kind: "screen", screen: "plan", sub: "board", param: id ? `project:${id}` : null });
  const projectMenu = (p: wire.Project): MenuItem[] => [
    ...(p.archived_at
      ? []
      : [
          { label: "Open the project", icon: ExternalLink, onSelect: () => tabs.open({ kind: "project", project: p.id }) },
          { label: "Open its board", icon: SquareKanban, onSelect: () => openBoard(p.id) },
          { label: "New task here", icon: Plus, onSelect: () => ui.create({ project: p.id }) },
          "divider" as const,
        ]),
    { label: "Edit name, colour, place", icon: Pencil, onSelect: () => setForm({ project: p }) },
    ...(!parentOf(p) && !p.archived_at ? [{ label: "Add a sub-project", icon: FolderPlus, onSelect: () => setForm({ project: null, parent: p.id }) }] : []),
    { label: p.folder ? "Change its folder" : "Choose its folder", icon: FolderOpen, onSelect: () => chooseFolder(p) },
    "divider",
    { label: p.archived_at ? "Bring it back" : "Archive", icon: p.archived_at ? ArchiveRestore : Archive, onSelect: () => toggleArchive(p) },
    { label: "Delete", icon: Trash2, danger: true, onSelect: () => removeProject(p) },
  ];

  // Select mode: move or delete many at once. Steps follow their task, so only top-level tasks move.
  const pickedTasks = (tasks ?? []).filter((x) => picked.has(x.id));
  async function movePicked(fields: Record<string, unknown>, where: string) {
    const list = pickedTasks.filter((x) => !x.parent_id && x.status === "open");
    let n = 0;
    for (const x of list) if (await d.act(() => App.PatchTask(x.id, wire.PatchTaskRequest.createFrom({ ...fields, rev: x.rev })))) n++;
    if (n > 0) notify(`Moved ${n} ${n === 1 ? "task" : "tasks"} to ${where}`, "success");
    setPicking(false);
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
  const flip = (set: Set<string>, id: string) => {
    const n = new Set(set);
    if (n.has(id)) n.delete(id);
    else n.add(id);
    return n;
  };

  // Ordinary tasks are the window's one task row; repeat templates, lined-up goal items and select mode keep their own.
  const row = (x: wire.Task, step = false, label?: string, first = false) =>
    !picking && !x.rrule && !isItem(x) ? (
      <TaskRow key={x.id} task={x} showProject={!selected || x.project_id !== selected} first={first} quietStatus={status === "done" ? ["done"] : undefined} lead={label && <span className="w-[60px] shrink-0 truncate pt-0.5 text-xs text-ink-subtle">{label}</span>} />
    ) : (
      <SpecialTaskRow
        key={x.id}
        x={x}
        step={step}
        first={first}
        label={label}
        goal={goalOf(x)}
        project={known.find((p) => p.id === x.project_id)}
        showProject={!selected || x.project_id !== selected}
        today={today}
        picking={picking}
        picked={picked.has(x.id)}
        onPick={() => setPicked((s) => flip(s, x.id))}
      />
    );

  const rowFacts = (id: Scope) => ({ open: openOf(id).length, week: weekOf(id), sessions: sessionsOf(id) });
  const noProject = openOf("none").length + (week.get("") ?? 0);
  const tree = projectTree(liveProjects);
  const filteredTree = q ? tree.filter((n) => [n.project, ...n.children].some((p) => p.name.toLowerCase().includes(q)) || openOf(n.project.id).some(matches)) : tree;
  const newTaskProject = selected === "none" ? "" : selected;

  return (
    <div ref={page} className="flex flex-col gap-5" style={{ paddingRight: sheetPad || undefined }}>
      <header className="flex flex-wrap items-end gap-x-4 gap-y-3">
        <div className="min-w-64 flex-1">
          <h1 className="text-[22px] font-semibold tracking-[-0.01em]">Projects</h1>
          <p className="mt-1 text-[13px] text-ink-muted tabular-nums">
            {liveProjects.length} {liveProjects.length === 1 ? "project" : "projects"} · {openOf("").length} open {openOf("").length === 1 ? "task" : "tasks"}
            {weekOf("") > 0 && ` · ${formatDuration(weekOf(""))} worked this week`}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <label className="relative flex items-center">
            <Search size={14} className="pointer-events-none absolute left-2.5 text-ink-subtle" aria-hidden />
            <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Find a project or task" aria-label="Find a project or task" className="field !h-8 w-56 !pl-8 !text-[13px]" />
          </label>
          <Button icon={FolderPlus} tone="primary" onClick={() => setForm({ project: null })}>
            New project
          </Button>
        </div>
      </header>

      <div className="grid items-start gap-5 @3xl:grid-cols-[19rem_minmax(0,1fr)]">
        <Card
          title="Projects"
          icon={FolderKanban}
          count={liveProjects.length}
          className="@3xl:sticky @3xl:top-0"
          bodyClassName="p-1.5"
          actions={
            <label className="flex items-center gap-2 pr-1 text-[12px] text-ink-subtle">
              Archived
              <Toggle checked={showArchived} onChange={setShowArchived} label="Show archived projects" />
            </label>
          }
        >
          <ul className="flex flex-col gap-px">
            <ProjectRow name="All projects" color="var(--color-ink-faint)" selected={selected === ""} onSelect={() => select("")} facts={rowFacts("")} detail="Every project" />
            {filteredTree.flatMap((n) => [n.project, ...n.children]).map((p) => (
              <ProjectRow
                key={p.id}
                sub={!!parentOf(p) && liveProjects.some((x) => x.id === parentOf(p))}
                name={p.name}
                color={p.color}
                selected={selected === p.id}
                onSelect={() => select(p.id)}
                facts={rowFacts(p.id)}
                detail={p.folder ? tildify(p.folder, home) : "No folder"}
                mono={!!p.folder}
                menu={projectMenu(p)}
              />
            ))}
            {noProject > 0 && <ProjectRow name="No project" color="transparent" outline selected={selected === "none"} onSelect={() => select("none")} facts={rowFacts("none")} detail="Not in any project" />}
          </ul>
          {liveProjects.length === 0 && (
            <div className="flex flex-col items-start gap-2 px-2.5 pt-2 pb-2.5">
              <p className="text-[12.5px] text-ink-subtle">A project colours your time and holds its tasks, folder and Claude sessions.</p>
              <Button size="sm" icon={FolderPlus} onClick={() => setForm({ project: null })}>
                New project
              </Button>
            </div>
          )}
          {showArchived && (
            <div className="mt-1.5 border-t border-chrome-line pt-1.5">
              <Eyebrow className="block px-2.5 pt-1 pb-1.5">Archived</Eyebrow>
              {archived.length === 0 ? (
                <p className="px-2.5 pb-1.5 text-[12px] text-ink-subtle">Nothing archived.</p>
              ) : (
                <ul className="flex flex-col gap-px">
                  {archived.map((p) => (
                    <ProjectRow key={p.id} name={p.name} color={p.color} archived selected={selected === p.id} onSelect={() => select(p.id)} facts={rowFacts(p.id)} detail="Archived" menu={projectMenu(p)} />
                  ))}
                </ul>
              )}
            </div>
          )}
        </Card>

        <div className="flex min-w-0 flex-col gap-5">
          <ProjectSummary
            project={current}
            scope={selected}
            projects={known}
            open={openOf(selected)}
            done={doneOf(selected)}
            week={weekOf(selected)}
            sessions={sessionsOf(selected)}
            goals={goalsIn(selected)}
            home={home}
            menu={current ? projectMenu(current) : undefined}
            onEdit={() => current && setForm({ project: current })}
            onFolder={() => current && chooseFolder(current)}
            onSubProject={() => current && setForm({ project: null, parent: current.id })}
            onSelect={select}
            onBoard={() => openBoard(selected === "none" ? "none" : selected)}
          />

          <Card
            title={status === "done" ? "Done" : status === "all" ? "All tasks" : "Open tasks"}
            icon={ListChecks}
            count={top.length + queues.length}
            bodyClassName=""
            actions={
              picking ? (
                <>
                  <span className="px-1 text-[12px] text-ink-subtle tabular-nums">{picked.size} picked</span>
                  <Button size="sm" tone="ghost" onClick={() => setPicked(picked.size === visible.length ? new Set() : new Set(visible.map((x) => x.id)))}>
                    {picked.size === visible.length && visible.length > 0 ? "None" : "All"}
                  </Button>
                  <select
                    aria-label="Move the picked tasks to a project"
                    value=""
                    disabled={!pickedTasks.some((x) => !x.parent_id && x.status === "open")}
                    onChange={(e) => {
                      const v = e.target.value;
                      if (!v) return;
                      const to = v === "none" ? null : v;
                      movePicked({ project_id: to }, to ? projectPath(known.find((p) => p.id === to), known) : "no project");
                    }}
                    className="field !h-7 w-32 !text-[12.5px]"
                  >
                    <option value="">Move to…</option>
                    <option value="none">No project</option>
                    {projectChoices(liveProjects).map((c) => (
                      <option key={c.id} value={c.id}>
                        {c.label}
                      </option>
                    ))}
                  </select>
                  <select
                    aria-label="Move the picked tasks to a stage"
                    value=""
                    disabled={!pickedTasks.some((x) => !x.parent_id && x.status === "open")}
                    onChange={(e) => e.target.value && movePicked({ stage: e.target.value }, stageChoices.find((s) => s.value === e.target.value)?.label ?? e.target.value)}
                    className="field !h-7 w-28 !text-[12.5px]"
                  >
                    <option value="">Stage…</option>
                    {stageChoices.map((s) => (
                      <option key={s.value} value={s.value}>
                        {s.label}
                      </option>
                    ))}
                  </select>
                  <Button size="sm" tone="destroy" icon={Trash2} disabled={picked.size === 0} onClick={removePicked}>
                    Delete
                  </Button>
                  <Button size="sm" onClick={() => setPicking(false)}>
                    Done
                  </Button>
                </>
              ) : (
                <>
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
                  {all.length > 0 && <IconButton size="sm" icon={CheckSquare} label="Pick tasks to move or delete" onClick={() => setPicking(true)} />}
                  <Button size="sm" tone="primary" icon={Plus} onClick={() => ui.create({ project: newTaskProject })}>
                    New task
                  </Button>
                </>
              )
            }
          >
            {tasks && top.length === 0 && queues.length === 0 ? (
              <div className="flex flex-col items-start gap-2.5 px-4 py-5">
                <p className="text-[13px] font-medium text-ink">{q ? `Nothing matches “${query.trim()}”` : status === "done" ? "Nothing done here yet" : "No open tasks"}</p>
                {!q && status !== "done" && (
                  <>
                    <p className="max-w-md text-[12.5px] text-ink-subtle">Add one and Gwen fits it into your day, or ask Gwen to break a goal into tasks.</p>
                    <Button size="sm" icon={Plus} onClick={() => ui.create({ project: newTaskProject })}>
                      New task
                    </Button>
                  </>
                )}
              </div>
            ) : (
              <ul className="flex flex-col">
                {top.map((x, i) => {
                  if (!picking && !x.rrule) return row(x, false, undefined, i === 0);
                  const steps = stepsOf(x.id);
                  const copies = copiesOf(x.id);
                  return (
                    <li key={x.id} className={cx(i > 0 && "border-t border-chrome-line")}>
                      <ul>{row(x, false, undefined, true)}</ul>
                      {copies.length > 0 && (
                        <ul className="ml-[1.85rem] border-l border-line pl-2">
                          {copies.map((c, j) => {
                            if (!picking) return row(c, true, copyLabel(c), j === 0);
                            const cs = stepsOf(c.id);
                            return (
                              <li key={c.id}>
                                <ul>{row(c, true, copyLabel(c), j === 0)}</ul>
                                {cs.length > 0 && <ul className="ml-[1.85rem] border-l border-line pl-2">{cs.map((s, k) => row(s, true, undefined, k === 0))}</ul>}
                              </li>
                            );
                          })}
                        </ul>
                      )}
                      {picking
                        ? steps.length > 0 && <ul className="ml-[1.85rem] border-l border-line pl-2">{steps.map((s, k) => row(s, true, undefined, k === 0))}</ul>
                        : steps.length > 0 && <StepList parent={x} steps={steps} adding className="mb-1.5" />}
                    </li>
                  );
                })}
                {queues.map(({ goal: g, items }, i) => {
                  const open = openQueues.has(g.id);
                  const unit = g.unit || "units";
                  const units = items.reduce((n, x) => n + (x.quantity ?? 1), 0);
                  const sessions = g.progress.per_session ? Math.ceil(units / g.progress.per_session) : 0;
                  return (
                    <li key={`queue-${g.id}`} className={cx((top.length > 0 || i > 0) && "border-t border-chrome-line")}>
                      <button type="button" onClick={() => setOpenQueues((s) => flip(s, g.id))} aria-expanded={open} className="flex w-full items-center gap-3 py-2.5 pr-3 pl-4 text-left hover:bg-surface-2/40">
                        {open ? <ChevronDown size={16} className="text-ink-subtle" aria-hidden /> : <ChevronRight size={16} className="text-ink-subtle" aria-hidden />}
                        <Layers size={15} className="text-accent-hover" aria-hidden />
                        <span className="min-w-0 flex-1 truncate text-sm text-ink">Lined up for {g.title}</span>
                        <span className="text-xs text-ink-subtle tabular-nums">
                          {units} {unit}
                          {sessions > 0 ? `, about ${sessions} ${sessions === 1 ? "session" : "sessions"}` : ""}
                        </span>
                      </button>
                      {open && (
                        <ul className="ml-[1.85rem] border-l border-line pl-2">
                          <li className="pr-3 pb-1 pl-4 text-xs text-ink-subtle">Each day's session takes the next ones from the top. Edit or delete any of them.</li>
                          {items.map((x) => row(x, true))}
                        </ul>
                      )}
                    </li>
                  );
                })}
              </ul>
            )}
          </Card>
        </div>
      </div>
      {form && <ProjectForm project={form.project} parent={form.parent} onClose={() => setForm(null)} onCreated={(p) => select(p.id)} />}
    </div>
  );
}

/** What the picked project adds up to: open work, what got done and worked this week, its folder, sub-projects, goals and sessions. */
function ProjectSummary({
  project: p,
  scope,
  projects,
  open,
  done,
  week,
  sessions,
  goals,
  home,
  menu,
  onEdit,
  onFolder,
  onSubProject,
  onSelect,
  onBoard,
}: {
  project?: wire.Project;
  scope: Scope;
  projects: wire.Project[];
  open: wire.Task[];
  done: number;
  week: number;
  sessions: Term[];
  goals: wire.Goal[];
  home: string;
  menu?: MenuItem[];
  onEdit: () => void;
  onFolder: () => void;
  onSubProject: () => void;
  onSelect: (id: Scope) => void;
  onBoard: () => void;
}) {
  const tabs = useTabs();
  const { go } = useNav();
  const doing = open.filter((x) => x.stage === "doing").length;
  const estimate = open.reduce((n, x) => n + (x.estimate_minutes ?? 0), 0);
  const children = p ? projects.filter((x) => parentOf(x) === p.id && !x.archived_at) : [];
  const parent = p ? projects.find((x) => x.id === parentOf(p)) : undefined;
  const needs = sessions.filter((s) => s.session?.state === "needs_you" || s.session?.state === "blocked").length;
  const title = p ? p.name : scope === "none" ? "No project" : "All projects";

  return (
    <section aria-label={title} className="lift min-w-0 overflow-hidden rounded-xl border border-line bg-surface-1">
      <header className="flex min-h-11 flex-wrap items-center gap-x-2 gap-y-1.5 border-b border-chrome-line py-2 pr-2.5 pl-4">
        {p ? <span className="size-3 shrink-0 rounded-[3px]" style={{ backgroundColor: p.color }} aria-hidden /> : <FolderKanban size={14} className="shrink-0 text-ink-subtle" aria-hidden />}
        {parent && (
          <button type="button" onClick={() => onSelect(parent.id)} className="shrink-0 text-[13px] text-ink-subtle hover:text-ink">
            {parent.name} ›
          </button>
        )}
        <h2 className="min-w-0 truncate text-[14px] font-semibold text-ink">{title}</h2>
        {p?.archived_at && <Badge>Archived</Badge>}
        <div className="ml-auto flex shrink-0 items-center gap-1">
          {p && !p.archived_at && (
            <Button size="sm" tone="ghost" icon={ExternalLink} onClick={() => tabs.open({ kind: "project", project: p.id })}>
              Open
            </Button>
          )}
          {!p?.archived_at && (
            <Button size="sm" tone="ghost" icon={SquareKanban} onClick={onBoard}>
              Board
            </Button>
          )}
          {p && (
            <Button size="sm" tone="ghost" icon={Pencil} onClick={onEdit}>
              Edit
            </Button>
          )}
          {menu && <RowMenu label={`More for ${p?.name}`} items={menu} />}
        </div>
      </header>
      <dl className="grid grid-cols-2 gap-px bg-chrome-line @xl:grid-cols-4">
        <Fact label="Open" value={String(open.length)} sub={doing > 0 ? `${doing} in progress` : open.length > 0 ? "none started" : "all clear"} />
        <Fact label="Done this week" value={String(done)} sub="last 7 days" />
        <Fact label="Worked this week" value={week > 0 ? formatDuration(week) : "—"} sub={p && children.length > 0 ? "with sub-projects" : "last 7 days"} />
        <Fact label="Estimated left" value={estimate > 0 ? formatDuration(estimate * 60_000) : "—"} sub={estimate > 0 ? "Gwen plans it in" : "no estimates"} />
      </dl>
      {p && (
        <div className="flex flex-col gap-2.5 border-t border-chrome-line px-4 py-3 text-[12.5px]">
          <div className="flex min-w-0 items-center gap-2.5">
            <span className="w-20 shrink-0 text-ink-subtle">Folder</span>
            {p.folder ? <span className="min-w-0 truncate font-mono text-[12px] text-ink-muted">{tildify(p.folder, home)}</span> : <span className="text-ink-subtle">None yet: sessions and files need one</span>}
            <Button size="sm" tone="ghost" icon={FolderOpen} onClick={onFolder} className="ml-auto shrink-0">
              {p.folder ? "Change" : "Choose"}
            </Button>
          </div>
          {!parentOf(p) && !p.archived_at && (
            <div className="flex min-w-0 flex-wrap items-center gap-1.5">
              <span className="w-20 shrink-0 text-ink-subtle">Sub-projects</span>
              {children.map((c) => (
                <button key={c.id} type="button" onClick={() => onSelect(c.id)} className="inline-flex h-6 items-center gap-1.5 rounded-md border border-line bg-surface-2 px-2 text-[12px] text-ink-muted hover:border-line-3 hover:text-ink">
                  <span className="size-2 rounded-[2px]" style={{ backgroundColor: c.color }} aria-hidden />
                  {c.name}
                </button>
              ))}
              <Button size="sm" tone="ghost" icon={FolderPlus} onClick={onSubProject}>
                Add
              </Button>
            </div>
          )}
          {(goals.length > 0 || sessions.length > 0) && (
            <div className="flex min-w-0 flex-wrap items-center gap-x-4 gap-y-1.5">
              {goals.length > 0 && (
                <button type="button" onClick={() => go("goals")} className="inline-flex min-w-0 items-center gap-1.5 text-ink-muted hover:text-ink">
                  <Target size={13} className="shrink-0 text-ink-subtle" aria-hidden />
                  <span className="truncate">{goals.length === 1 ? goals[0].title : `${goals.length} goals`}</span>
                </button>
              )}
              {sessions.length > 0 && (
                <button type="button" onClick={() => tabs.open({ kind: "project", project: p.id })} className="inline-flex items-center gap-1.5 text-ink-muted hover:text-ink">
                  <ClaudeMark size={12} />
                  {sessions.length} Claude {sessions.length === 1 ? "session" : "sessions"} open
                  {needs > 0 && <span className="text-warning">· {needs} need{needs === 1 ? "s" : ""} you</span>}
                </button>
              )}
            </div>
          )}
        </div>
      )}
    </section>
  );
}

function Fact({ label, value, sub }: { label: string; value: string; sub: string }) {
  return (
    <div className="flex min-w-0 flex-col gap-0.5 bg-surface-1 px-4 py-3">
      <dt className="text-[11.5px] text-ink-subtle">{label}</dt>
      <dd className="text-[17px] font-semibold text-ink tabular-nums">{value}</dd>
      <dd className="truncate text-[11.5px] text-ink-subtle">{sub}</dd>
    </div>
  );
}

/** Repeat templates, queued goal items, and the pick boxes in select mode. */
function SpecialTaskRow({
  x,
  step,
  first,
  label,
  goal,
  project,
  showProject,
  today,
  picking,
  picked,
  onPick,
}: {
  x: wire.Task;
  step: boolean;
  /** The first row in its list: no hairline above it. */
  first: boolean;
  /** Before the title, such as the day of a repeating task's copy. */
  label?: string;
  goal?: wire.Goal;
  project?: wire.Project;
  showProject: boolean;
  today: string;
  picking: boolean;
  picked: boolean;
  onPick: () => void;
}) {
  const ui = useTaskUI();
  const isDone = x.status === "done";
  const template = !!x.rrule;
  const status = ui.statusOf(x);
  const tracking = status.kind === "tracking";
  return (
    <li className={cx("group flex items-start gap-3 pr-3 pl-4", !first && "border-t border-chrome-line", step ? "py-1.5" : "py-2", tracking ? "bg-surface-2/40" : "hover:bg-surface-2/40")}>
      <span className="flex shrink-0">
        {picking ? (
          <Checkbox square checked={picked} onChange={onPick} label={`Select ${x.title}`} />
        ) : template ? (
          <span className="grid size-4.5 place-items-center text-ink-subtle" title="Repeating: each day's copy is what you complete">
            <Repeat size={14} aria-hidden />
          </span>
        ) : (
          <Checkbox checked={isDone} onChange={(on) => on ? ui.complete(x) : ui.reopen(x)} label={isDone ? `Reopen ${x.title}` : `Complete ${x.title}`} />
        )}
      </span>
      <button type="button" className="min-w-0 flex-1 text-left" onClick={picking ? onPick : () => ui.edit(x)}>
        <div className={cx("leading-5", step ? "text-[13px]" : "text-[13.5px]", isDone ? "text-ink-faint line-through" : "text-ink")}>
          {label && <span className="mr-1.5 text-xs font-medium text-ink-subtle">{label}</span>}
          {x.title}
        </div>
        {x.notes && <div className={cx("mt-0.5 text-xs whitespace-pre-line text-ink-subtle", step ? "line-clamp-2" : "line-clamp-1")}>{x.notes}</div>}
        {step && showProject && <ProjectTag project={project} />}
        {!step && (
          <div className="mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-1">
            {showProject && <ProjectTag project={project} />}
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
            {status.kind !== "todo" && <StatusPill status={status} />}
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
          <IconButton icon={Pencil} label={`Edit ${x.title}`} size="sm" onClick={() => ui.edit(x)} />
          <IconButton icon={Trash2} label={`Delete ${x.title}`} size="sm" onClick={() => ui.remove(x)} />
        </span>
      )}
    </li>
  );
}

/** A project in the list: its colour, name, live sessions and open count, with its folder and this week's time under it. */
function ProjectRow({
  name,
  color,
  selected,
  onSelect,
  facts,
  detail,
  mono,
  sub,
  archived,
  outline,
  menu,
}: {
  name: string;
  color: string;
  selected: boolean;
  onSelect: () => void;
  facts: { open: wire.Task[] | number; week: number; sessions: Term[] } | { open: number; week: number; sessions: Term[] };
  /** The second line: its folder, or what the row stands for. */
  detail: string;
  mono?: boolean;
  /** A sub-project, drawn indented under its project. */
  sub?: boolean;
  archived?: boolean;
  /** No colour of its own (No project): an outlined square. */
  outline?: boolean;
  menu?: MenuItem[];
}) {
  const open = typeof facts.open === "number" ? facts.open : facts.open.length;
  const needs = facts.sessions.some((s) => s.session?.state === "needs_you" || s.session?.state === "blocked");
  return (
    <li className={cx("group relative flex items-center rounded-lg", selected ? "bg-surface-2 shadow-[inset_0_0_0_1px_var(--color-line)]" : "hover:bg-surface-2/60")}>
      {sub && <span aria-hidden className="absolute top-0 bottom-0 left-[15px] w-px bg-line" />}
      <button type="button" onClick={onSelect} aria-current={selected || undefined} className={cx("flex min-w-0 flex-1 items-start gap-2.5 py-2 text-left", sub ? "pr-2 pl-7" : "pr-2 pl-2.5")}>
        <span className={cx("mt-[4px] size-2.5 shrink-0 rounded-[3px]", outline && "border border-line-3")} style={{ backgroundColor: color }} aria-hidden />
        <span className="min-w-0 flex-1">
          <span className="flex min-w-0 items-center gap-1.5">
            <span className={cx("truncate text-[13px] font-medium", archived ? "text-ink-faint" : selected ? "text-ink" : "text-ink-muted group-hover:text-ink")}>{name}</span>
            {facts.sessions.length > 0 && (
              <span className={cx("inline-flex shrink-0 items-center gap-1 text-[11px] tabular-nums", needs ? "text-warning" : "text-ink-subtle")} title={`${facts.sessions.length} Claude ${facts.sessions.length === 1 ? "session" : "sessions"} open`}>
                <ClaudeMark size={10} />
                {facts.sessions.length}
              </span>
            )}
          </span>
          <span className="mt-0.5 flex min-w-0 items-center gap-1.5 text-[11.5px] text-ink-subtle">
            <span className={cx("min-w-0 truncate", mono && "font-mono text-[11px]")}>{detail}</span>
            {facts.week > 0 && <span className="shrink-0 tabular-nums">· {formatDuration(facts.week)} this week</span>}
          </span>
        </span>
        {open > 0 && <span className={cx("mt-px shrink-0 text-[11.5px] text-ink-subtle tabular-nums", menu && "group-hover:invisible")}>{open}</span>}
      </button>
      {menu && <RowMenu label={`More for ${name}`} items={menu} className="absolute top-1.5 right-1.5 opacity-0 group-hover:opacity-100 focus-visible:opacity-100 aria-expanded:opacity-100" />}
    </li>
  );
}

/** The one project form: a new project (at the top level, or under one) or an existing one's name, place, colour and folder. */
/** A project's fields and saving them, shared by the New and Edit project popup and the Settings view of a project's home. */
export function useProjectForm(p: wire.Project | null, under?: string) {
  const d = useDaemon();
  const home = useHome();
  const [name, setName] = useState(p?.name ?? "");
  const [color, setColor] = useState(p?.color ?? projectPalette[d.projects.length % projectPalette.length]);
  const [parent, setParent] = useState(p ? (parentOf(p) ?? "") : (under ?? ""));
  const [folder, setFolder] = useState(p?.folder ?? "");
  // How freely Gwen's agent acts in it on this machine: "" follows Settings › Gwen › Agent (plan.md 3.16).
  const [autonomy, setAutonomy] = useState(p?.agent_autonomy ?? "");
  const [saving, setSaving] = useState(false);
  const reset = () => {
    if (!p) return;
    setName(p.name);
    setColor(p.color);
    setParent(parentOf(p) ?? "");
    setFolder(p.folder ?? "");
    setAutonomy(p.agent_autonomy ?? "");
  };
  // A change saved anywhere else (its folder from the header, say) shows here too.
  useEffect(reset, [p?.rev]); // eslint-disable-line react-hooks/exhaustive-deps
  const dirty = !p || name.trim() !== p.name || color !== p.color || parent !== (parentOf(p) ?? "") || folder !== (p.folder ?? "") || autonomy !== (p.agent_autonomy ?? "");
  // One level: a project with sub-projects stays at the top, and only top-level projects can hold others.
  const hasChildren = !!p && d.projects.some((x) => parentOf(x) === p.id);
  const parents = d.projects.filter((x) => x.id !== p?.id && !parentOf(x) && (!x.archived_at || x.id === parent));
  async function pick() {
    const path = await App.PickProjectFolder(folder || home).catch((e) => {
      d.fail(e);
      return "";
    });
    if (path) setFolder(path);
  }
  async function save(): Promise<wire.Project | null> {
    if (!name.trim() || saving) return null;
    setSaving(true);
    const out = p
      ? await d.act(() =>
          App.PatchProject(
            p.id,
            wire.PatchProjectRequest.createFrom({
              name: name.trim(),
              color,
              // An empty choice moves it to the top level, which the API takes as null.
              ...(parent !== (parentOf(p) ?? "") ? { parent_id: parent || null } : {}),
              ...(folder !== (p.folder ?? "") ? { folder } : {}),
              ...(autonomy !== (p.agent_autonomy ?? "") ? { agent_autonomy: autonomy } : {}),
              rev: p.rev,
            }),
          ),
        )
      : await d.act(() => App.CreateProject(wire.CreateProjectRequest.createFrom({ name: name.trim(), color, parent_id: parent || undefined, folder: folder || undefined })));
    // A new project takes its autonomy once it exists: it is device-local, like the folder.
    const done = !p && out && autonomy ? ((await d.act(() => App.PatchProject(out.id, wire.PatchProjectRequest.createFrom({ agent_autonomy: autonomy })))) ?? out) : out;
    setSaving(false);
    return done || null;
  }
  return { project: p, name, setName, color, setColor, parent, setParent, folder, setFolder, autonomy, setAutonomy, saving, dirty, hasChildren, parents, pick, save, reset };
}

const agentHint: Record<string, string> = {
  "": "Follows Settings › Gwen › Agent.",
  cautious: "She asks before every change, tasks and plans included.",
  balanced: "Edits ask once per chat; each command asks, with “Always in this project”.",
  trusted: "Edits and commands run freely; the web asks once per chat; pushes, sends, links and deletes ask every time.",
};

/** Name, where it sits, folder, Gwen's autonomy and colour: the fields of useProjectForm. */
export function ProjectFields({ form: f, onEnter, autoFocus }: { form: ReturnType<typeof useProjectForm>; onEnter: () => void; autoFocus?: boolean }) {
  const home = useHome();
  return (
    <div className="flex flex-col gap-5">
      <Field label="Name">
        <Input
          value={f.name}
          maxLength={120}
          onChange={(e) => f.setName(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              onEnter();
            }
          }}
          placeholder="Thesis"
          autoFocus={autoFocus}
        />
      </Field>
      <Field label="Sits under" hint={f.hasChildren ? `${f.project!.name} has sub-projects, so it stays a top-level project.` : "A sub-project keeps its own tasks, time, folder and sessions; its project groups them."}>
        <Select value={f.parent} onChange={(e) => f.setParent(e.target.value)} disabled={f.hasChildren}>
          <option value="">Nothing: a top-level project</option>
          {f.parents.map((x) => (
            <option key={x.id} value={x.id}>
              {x.name}
            </option>
          ))}
        </Select>
      </Field>
      <Field label="Folder" compound hint="Where its code or files live on this computer: its terminals, Claude sessions, files and history open there.">
        <div className="flex min-w-0 items-center gap-2">
          <span className={cx("min-w-0 flex-1 truncate rounded-lg border border-line bg-surface-2 px-3 py-1.5 text-[12.5px]", f.folder ? "font-mono text-ink-muted" : "text-ink-subtle")}>{f.folder ? tildify(f.folder, home) : "No folder"}</span>
          <Button size="sm" icon={FolderOpen} onClick={f.pick}>
            {f.folder ? "Change" : "Choose"}
          </Button>
          {f.folder && <IconButton size="sm" icon={X} label="Forget the folder" onClick={() => f.setFolder("")} />}
        </div>
      </Field>
      <Field label="Gwen's agent here" compound hint={agentHint[f.autonomy] ?? agentHint[""]}>
        <Segmented
          value={f.autonomy}
          onChange={f.setAutonomy}
          label="How freely Gwen's agent acts in this project"
          className="self-start"
          options={[
            { value: "", label: "Default" },
            { value: "cautious", label: "Cautious" },
            { value: "balanced", label: "Balanced" },
            { value: "trusted", label: "Trusted" },
          ]}
        />
      </Field>
      <Field label="Colour" compound>
        <div className="flex flex-wrap items-center gap-2">
          {projectPalette.map((c, i) => (
            <button
              key={c}
              type="button"
              aria-label={`Colour ${i + 1}`}
              aria-pressed={f.color.toLowerCase() === c}
              onClick={() => f.setColor(c)}
              className={cx("size-7 rounded-full transition-transform hover:scale-110", f.color.toLowerCase() === c && "ring-2 ring-ink ring-offset-2 ring-offset-surface-1")}
              style={{ backgroundColor: c }}
            />
          ))}
          <label className="ml-1 flex items-center gap-2 text-xs text-ink-subtle" title="Any colour">
            <input type="color" className="swatch" value={f.color} onChange={(e) => f.setColor(e.target.value)} aria-label="Custom colour" />
            Custom
          </label>
        </div>
      </Field>
    </div>
  );
}

function ProjectForm({ project: p, parent: under, onClose, onCreated }: { project: wire.Project | null; parent?: string; onClose: () => void; onCreated?: (p: wire.Project) => void }) {
  const f = useProjectForm(p, under);
  async function save() {
    const out = await f.save();
    if (!out) return;
    if (!p) onCreated?.(out);
    onClose();
  }
  return (
    <Modal
      title={p ? "Edit project" : f.parent ? `New sub-project` : "New project"}
      size="sm"
      onClose={onClose}
      footer={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button tone="primary" onClick={save} disabled={!f.name.trim()} busy={f.saving}>
            {p ? "Save project" : "Create project"}
          </Button>
        </>
      }
    >
      <ProjectFields form={f} onEnter={save} autoFocus />
    </Modal>
  );
}

function TaskProjectPicker({ projects, current, value, onChange }: { projects: wire.Project[]; current: string; value: string; onChange: (value: string) => void }) {
  const d = useDaemon();
  const [fetched, setFetched] = useState<wire.Project | null>(null);
  const known = projects.find((p) => p.id === current) ?? d.projects.find((p) => p.id === current);
  useEffect(() => {
    if (!current || known) return;
    let live = true;
    // An archived project is no longer in the lists; ask for its name. One that's gone just reads as archived.
    App.GetProject(current).then(
      (p) => live && setFetched(p),
      () => {},
    );
    return () => {
      live = false;
    };
  }, [current, known, d.projectsVersion]);
  // Its current project stays a choice even when archived, so opening the form never quietly drops it.
  const choices = projectChoices(projects);
  if (current && !choices.some((p) => p.id === current)) {
    const project = known ?? (fetched?.id === current ? fetched : undefined);
    choices.push({ id: current, label: project ? `${projectPath(project, [...projects, ...d.projects])} (archived)` : "An archived project" });
  }
  return (
    <Field label="Project">
      <Select value={value} onChange={(e) => onChange(e.target.value)}>
        <option value="">No project</option>
        {choices.map((p) => <option key={p.id} value={p.id}>{p.label}</option>)}
      </Select>
    </Field>
  );
}

export function TaskForm({
  task,
  projects,
  goals,
  defaultProject,
  defaultStage,
  defaultGoal,
  defaultRule,
  defaultMany,
  onClose,
  inline,
}: {
  task: wire.Task | null;
  projects: wire.Project[];
  goals: wire.Goal[];
  defaultProject: string;
  defaultStage?: string;
  /** A new task for this goal, such as one added from the goal's card. */
  defaultGoal?: string;
  /** A new task that repeats, such as a goal's daily task. */
  defaultRule?: string;
  /** Opens on several new tasks at once, one title per line, such as a goal's set of tasks. */
  defaultMany?: boolean;
  /** Saved or cleared; a modal closes, an inline form starts over. */
  onClose: () => void;
  /** Drawn in place (Today's New task card) instead of as a modal: the project shows up front, and Cancel becomes Clear. */
  inline?: boolean;
}) {
  const d = useDaemon();
  const today = d.today();
  const [title, setTitle] = useState(task?.title ?? "");
  // A new task starts today; how long it takes and the rest wait under More.
  const [startDay, setStartDay] = useState(task ? (task.start_day ?? "") : today);
  const [startMinute, setStartMinute] = useState<number | null>(task?.start_minute ?? null);
  const [more, setMore] = useState(!!task || !!defaultGoal || !!defaultRule);
  const [project, setProject] = useState(task ? (task.project_id ?? "") : defaultProject);
  const [goal, setGoal] = useState(task ? (task.goal_id ?? "") : (defaultGoal ?? ""));
  const [priority, setPriority] = useState(task?.priority ?? 2);
  const [due, setDue] = useState(task?.due_day ?? "");
  const [estimate, setEstimate] = useState(goDuration((task?.estimate_minutes ?? 0) * 60_000));
  const [steps, setSteps] = useState("");
  // Attachments added before the task exists; they are saved right after it.
  const [pending, setPending] = useState<PendingAttachment[]>([]);
  const [quantity, setQuantity] = useState(task?.quantity != null ? String(task.quantity) : "");
  const [notes, setNotes] = useState(task?.notes ?? "");
  const [rule, setRule] = useState(task ? (task.rrule ?? "") : (defaultRule ?? ""));
  const [effort, setEffort] = useState(task?.effort ?? 0);
  const [stage, setStage] = useState(task?.stage ?? defaultStage ?? "todo");
  const [delegatedTo, setDelegatedTo] = useState(task?.delegated_to ?? "");
  // A time set by hand: a pinned block on the plan.
  const [block, setBlock] = useState<Block | null>(null);
  const [saving, setSaving] = useState(false);
  // Enter and the button both save: one save at a time, so a quick double press never makes the task twice.
  const inFlight = useRef(false);
  const createdTask = useRef<wire.Task | null>(null);
  // Several new tasks at once: one title per line, all with the settings below. Steps and a set time are per task, so they wait.
  const [many, setMany] = useState(!task && !!defaultMany);
  const lines = title.split(/\r?\n/).map((l) => l.trim()).filter(Boolean);
  const [fetchedGoal, setFetchedGoal] = useState<wire.Goal | null>(null);
  const currentGoalId = task?.goal_id;
  const listedGoal = goals.find((g) => g.id === currentGoalId);
  useEffect(() => {
    if (!currentGoalId || listedGoal) return;
    let live = true;
    // A finished goal isn't in the active list; fetch it so the form still names it.
    App.GetGoal(currentGoalId).then(
      (g) => live && setFetchedGoal(g),
      () => {},
    );
    return () => {
      live = false;
    };
  }, [currentGoalId, listedGoal, d.goalsVersion]);
  const availableGoals = goals.filter((g) => g.status === "active" || g.id === currentGoalId);
  if (!listedGoal && fetchedGoal && fetchedGoal.id === currentGoalId) availableGoals.push(fetchedGoal);
  // Occurrences of a repeating task cannot repeat themselves.
  const canRepeat = !task?.template_id;
  const goalKind = availableGoals.find((g) => g.id === goal)?.kind ?? (goal === currentGoalId && task?.quantity != null ? "quantity" : undefined);

  async function save() {
    if (!title.trim() || inFlight.current) return;
    inFlight.current = true;
    setSaving(true);
    if (many && !task) return saveMany();
    const current = task ?? createdTask.current;
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
    const repeat = canRepeat && rule !== (current?.rrule ?? "") ? { rrule: rule || null } : {};
    const ok = current
      ? await d.act(() =>
          App.PatchTask(
            current.id,
            wire.PatchTaskRequest.createFrom({
              ...fields,
              ...repeat,
              rev: current.rev,
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
    if (ok && !task) {
      createdTask.current = ok;
      const titles = steps.split(/\r?\n/).map((s) => s.trim()).filter(Boolean);
      for (let i = 0; i < titles.length; i++) {
        const step = await d.act(() => App.CreateTask(wire.CreateTaskRequest.createFrom({ title: titles[i], parent_id: ok.id, project_id: ok.project_id ?? null })));
        if (!step) {
          setSteps(titles.slice(i).join("\n"));
          setSaving(false);
          inFlight.current = false;
          return;
        }
      }
      setSteps("");
      for (let i = 0; i < pending.length; i++) {
        const { key: _, snippet: __, ...req } = pending[i];
        const att = await d.act(() => App.CreateAttachment(ok.id, wire.CreateAttachmentRequest.createFrom(req)));
        if (!att) {
          setPending(pending.slice(i));
          setSaving(false);
          inFlight.current = false;
          return;
        }
      }
    }
    // Gwen adds new and changed tasks to today's plan by itself; a time set by hand on an existing task moves it there.
    if (ok && current && schedule && !rule) await d.act(() => App.ScheduleTask(wire.ScheduleRequest.createFrom({ task_id: current.id, ...schedule })));
    setSaving(false);
    if (ok) onClose();
    else inFlight.current = false; // a failed save may be tried again; a closed form never saves twice
  }

  /** Creates one task per line, in order; on a failure the lines not yet made stay in the box to try again. */
  async function saveMany() {
    const minutes = Math.round(parseDuration(estimate) / 60_000);
    for (let i = 0; i < lines.length; i++) {
      const ok = await d.act(() =>
        App.CreateTask(
          wire.CreateTaskRequest.createFrom({
            title: lines[i],
            start_day: startDay || undefined,
            start_minute: startDay && startMinute != null ? startMinute : undefined,
            project_id: project || null,
            priority,
            due_day: due || null,
            estimate_minutes: minutes > 0 ? minutes : null,
            notes,
            goal_id: goal || null,
            quantity: goalKind === "quantity" && Number(quantity) > 0 ? Number(quantity) : null,
            effort: effort || undefined,
            stage,
            delegated_to: stage === "waiting" ? delegatedTo : "",
            rrule: rule || undefined,
          }),
        ),
      );
      if (!ok) {
        setTitle(lines.slice(i).join("\n"));
        setSaving(false);
        inFlight.current = false;
        return;
      }
    }
    setSaving(false);
    onClose();
  }

  const footer = (
    <>
      <Button onClick={onClose} disabled={inline && !title}>
        {inline ? "Clear" : "Cancel"}
      </Button>
      <Button tone="primary" onClick={save} disabled={!title.trim()} busy={saving}>
        {task || createdTask.current ? "Save task" : many ? (lines.length > 1 ? `Create ${lines.length} tasks` : "Create task") : "Create task"}
      </Button>
    </>
  );
  // Several at once is for new tasks only; going back to one keeps a single line.
  const canMany = !task && !createdTask.current && !inline;
  const body = (
      <div className="flex flex-col gap-5">
        <Field
          label={many ? "Tasks" : "Title"}
          compound={canMany}
          hint={
            canMany && (many || lines.length <= 1) ? (
              <span className="flex flex-wrap items-baseline gap-x-2">
                {many && <span>One per line, each with the settings below. Add steps to any of them afterwards.</span>}
                {(!many || lines.length <= 1) && (
                  <button type="button" className="font-medium text-accent-hover hover:underline" onClick={() => setMany(!many)}>
                    {many ? "Just one task" : "Add several at once"}
                  </button>
                )}
              </span>
            ) : undefined
          }
        >
          {many ? (
            <TextArea
              rows={5}
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
                  e.preventDefault();
                  save();
                }
              }}
              placeholder={"Read chapter 1\nDo the chapter 1 exercises\nRead chapter 2"}
              aria-label="Tasks, one per line"
              disabled={saving}
              autoFocus
            />
          ) : (
            <Input
              value={title}
              maxLength={300}
              onChange={(e) => setTitle(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  save();
                }
              }}
              placeholder="Call the bank"
              autoFocus={!inline}
            />
          )}
        </Field>
        {inline && <TaskProjectPicker projects={projects} current={task ? (task.project_id ?? "") : defaultProject} value={project} onChange={setProject} />}
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
        {!many && <Field label="Steps" compound={!!task} hint={!task && createdTask.current ? "Task created. Save again to add the remaining steps." : undefined}>
          {task ? <StepList parent={task} steps={d.tasks.filter((s) => s.parent_id === task.id)} adding className="!ml-0" /> : <TextArea rows={inline ? 2 : 3} value={steps} onChange={(e) => setSteps(e.target.value)} placeholder="One step per line" disabled={saving} />}
        </Field>}
        {!many && (
          <Field label="Context" compound hint="Optional: a link, a note, a file, or an email, event, issue, or page from your apps. An agent you hand the task to gets it in its brief.">
            <AttachmentsField taskId={task?.id ?? null} pending={pending} onPending={setPending} />
          </Field>
        )}
        {!rule && !many && (
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
            {inline ? "More: notes, goal, priority, due, repeat" : "More: notes, project, goal, priority, due date, stage, repeat"}
          </Button>
        ) : (
          <>
          <Field label="Notes">
            <TextArea rows={2} value={notes} onChange={(e) => setNotes(e.target.value)} />
          </Field>
          <div className="grid grid-cols-2 gap-4">
            {!inline && <TaskProjectPicker projects={projects} current={task ? (task.project_id ?? "") : defaultProject} value={project} onChange={setProject} />}
            <Field label="Goal" hint={availableGoals.length === 0 && !currentGoalId ? "No active goals." : undefined}>
              <Select value={goal} onChange={(e) => setGoal(e.target.value)}>
                <option value="">None</option>
                {currentGoalId && !availableGoals.some((g) => g.id === currentGoalId) && <option value={currentGoalId}>Current goal (finished)</option>}
                {availableGoals.map((g) => (
                  <option key={g.id} value={g.id}>
                    {g.title}{g.status !== "active" && " (finished)"}
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
                {stageChoices.map((s) => (
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
  );
  if (inline)
    return (
      <div className="flex flex-col gap-4">
        {body}
        <div className="flex justify-end gap-2 border-t border-chrome-line pt-3">{footer}</div>
      </div>
    );
  return (
    <Modal title={task ? "Edit task" : many ? "New tasks" : "New task"} size={more ? "lg" : "md"} onClose={onClose} footer={footer}>
      {body}
    </Modal>
  );
}
