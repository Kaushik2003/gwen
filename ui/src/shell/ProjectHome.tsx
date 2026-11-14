import { Archive, Bot, FileCode, FolderOpen, GitBranch, GitCommitHorizontal, Plus, ShieldCheck, SquareTerminal } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { App, wire } from "../api";
import { projectPalette } from "../components/color";
import { GoalMeter, PaceChip, goalAmount } from "../components/goal";
import { useConfirm } from "../components/feedback";
import { Badge, Button, Empty, IconButton, Input, cx } from "../components/ui";
import { useDaemon } from "../daemon";
import { addDays, formatAgo, formatDuration } from "../format";
import Board from "../screens/Board";
import History from "../screens/History";
import { ProjectFields, useProjectForm } from "../screens/Projects";
import TaskRow from "../task/TaskRow";
import { useTaskUI } from "../task/TaskUI";
import { stageName, stageOrder, type TaskStatusKind } from "../task/status";
import type { repo } from "../wailsjs/go/models";
import { startShell } from "../work/terminals";
import { useClearOfSheet } from "./HandOff";
import { Scroll, StateGlyph, Swatch, TermIcon, tildify, useHome } from "./bits";
import { parentOf } from "./projects";
import { useTabs, type ProjectView } from "./tabs";
import { useWork } from "./work";

// A project's home (plan.md 3.15, board 7): its folder and branch and its
// time on top, then five views. Overview: its sessions, its tasks by stage,
// its goals, its recent commits, and its defaults. Board: its tasks on the
// board. Sessions: every session running and ended. Time: its days. Settings:
// its name, place, folder and colour, and archiving it.

const views: [ProjectView, string][] = [
  ["overview", "Overview"],
  ["board", "Board"],
  ["sessions", "Sessions"],
  ["time", "Time"],
  ["settings", "Settings"],
];

/** Starts a Claude session in a folder through gwend, and opens its tab when its terminal is known. */
export async function startClaude(
  folder: string,
  project: string,
  opts: { sandbox?: boolean; prompt?: string; resume?: string },
  open: (project: string, terminal: string) => void,
): Promise<wire.CodeSession> {
  const s = await App.StartCodeSession(wire.StartCodeSession.createFrom({ dir: folder, window: false, sandbox: opts.sandbox, prompt: opts.prompt || undefined, resume: opts.resume || undefined }));
  if (s.terminal) open(project, s.terminal);
  return s;
}

export default function ProjectHome({ project: id, view = "overview" }: { project: string; view?: ProjectView }) {
  const d = useDaemon();
  const w = useWork();
  const tabs = useTabs();
  const ui = useTaskUI();
  const home = useHome();
  const confirm = useConfirm();
  const root = useRef<HTMLDivElement>(null);
  const pad = useClearOfSheet(root, !!ui.handing);
  const p = d.projects.find((x) => x.id === id);
  const [tree, setTree] = useState<repo.Tree | null>(null);
  const [log, setLog] = useState<repo.Commit[]>([]);
  const [goals, setGoals] = useState<wire.Goal[]>([]);
  const [week, setWeek] = useState(0);
  const [busy, setBusy] = useState("");
  const [showAll, setShowAll] = useState<Record<string, boolean>>({});
  const [childName, setChildName] = useState("");
  const today = d.today();

  useEffect(() => {
    if (!p?.folder) return setTree(null);
    let live = true;
    App.RepoTree(p.folder).then(
      (x) => live && setTree(x),
      () => live && setTree(null),
    );
    App.RepoLog(p.folder, { limit: 5, skip: 0, search: "", author: "", path: "" } as repo.LogQuery).then(
      (l) => live && setLog(l.commits ?? []),
      () => live && setLog([]),
    );
    return () => {
      live = false;
    };
  }, [p?.folder]);
  useEffect(() => {
    App.ListGoals("active").then(
      (l) => setGoals(l.goals),
      () => setGoals([]),
    );
  }, [d.goalsVersion]);
  useEffect(() => {
    const start = addDays(today, -((new Date(today + "T12:00:00").getDay() + 6) % 7));
    // With its sub-projects, as its board and its Time view count it.
    const ids = new Set([id, ...d.projects.filter((x) => parentOf(x) === id).map((x) => x.id)]);
    App.StatsSummary(start, today).then(
      (s) => setWeek(s.by_project.filter((x) => ids.has(x.project_id ?? "")).reduce((n, x) => n + x.worked_ms, 0)),
      () => setWeek(0),
    );
  }, [id, today, d.daysVersion, d.projects]);

  if (!p)
    return (
      <Scroll>
        <Empty title="This project is gone">It was deleted or archived. Close this tab.</Empty>
      </Scroll>
    );
  const project = p;
  const parent = d.projects.find((x) => x.id === parentOf(project));
  const children = d.projects.filter((x) => parentOf(x) === project.id && !x.archived_at);

  const projectIds = new Set([id, ...children.map((x) => x.id)]);
  const todayMs = (d.status?.today?.by_project ?? []).filter((x) => projectIds.has(x.project_id ?? "")).reduce((n, x) => n + x.worked_ms, 0);
  const terms = w.termsOf(id);
  const allEnded = w.endedOf(id);
  const ended = allEnded.slice(0, 5);
  // Its open tasks and its sub-projects', Urgent first within each stage.
  const tasks = d.tasks.filter((x) => !x.parent_id && projectIds.has(x.project_id ?? "")).sort((a, b) => b.priority - a.priority);
  const stages = ["doing", "todo", ...stageOrder.filter((stage) => stage !== "doing" && stage !== "todo")];
  const projectGoals = goals.filter((g) => g.project_id === id);
  const changed = tree?.files.filter((f) => f.status).length ?? 0;
  const openTerm = (proj: string, terminal: string) => tabs.open({ kind: "term", project: proj, terminal });
  const show = (v: ProjectView) => tabs.update(`project:${id}`, { kind: "project", project: id, view: v === "overview" ? undefined : v });

  async function chooseFolder() {
    const path = await App.PickProjectFolder(project.folder || home).catch((e) => {
      d.fail(e);
      return "";
    });
    if (!path || !(await d.act(() => App.PatchProject(project.id, wire.PatchProjectRequest.createFrom({ folder: path }))))) return "";
    await d.refresh();
    return path;
  }
  async function newShell() {
    setBusy("shell");
    try {
      const path = project.folder || (await chooseFolder());
      if (!path) return;
      const term = await startShell(path, project.id);
      openTerm(project.id, term.address);
    } catch (e) {
      d.fail(e);
    } finally {
      setBusy("");
    }
  }
  async function newSession(sandbox: boolean, resume?: string) {
    setBusy(resume ?? (sandbox ? "sandbox" : "claude"));
    try {
      const path = project.folder || (await chooseFolder());
      if (!path) return;
      await startClaude(path, project.id, { sandbox, resume }, openTerm);
      w.refreshSessions();
    } catch (e) {
      d.fail(e);
    } finally {
      setBusy("");
    }
  }
  async function addChild(e: React.FormEvent) {
    e.preventDefault();
    const name = childName.trim();
    if (!name) return;
    const color = projectPalette[(d.projects.length + 1) % projectPalette.length];
    if (await d.act(() => App.CreateProject(wire.CreateProjectRequest.createFrom({ name, color, parent_id: project.id })))) setChildName("");
  }
  async function archive() {
    if (!(await confirm({ title: `Archive ${project.name}?`, body: "It leaves the sidebar. Its tasks, time, and folder stay, and you can bring it back from Manage projects.", confirm: "Archive" }))) return;
    await d.act(() => App.PatchProject(project.id, wire.PatchProjectRequest.createFrom({ archived: true, rev: project.rev })));
    tabs.close(`project:${project.id}`);
  }

  return (
    <Scroll wide>
      <div ref={root} className="flex flex-col gap-4" style={pad ? { paddingRight: pad } : undefined}>
        <header className="flex flex-wrap items-start gap-3">
          <div className="min-w-72 flex-1">
            {parent && (
              <button type="button" onClick={() => tabs.open({ kind: "project", project: parent.id })} className="mb-0.5 flex items-center gap-1.5 text-[12.5px] text-ink-subtle hover:text-ink">
                <Swatch color={parent.color} size={8} />
                {parent.name} ›
              </button>
            )}
            <h1 className="flex items-center gap-2.5 text-[22px] font-semibold tracking-[-0.01em]">
              <Swatch color={project.color} size={12} />
              {project.name}
            </h1>
            <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-[12.5px] text-ink-muted">
              {project.folder ? (
                <button type="button" onClick={chooseFolder} className="max-w-full text-left font-mono break-all hover:text-ink" title="Change its folder">
                  {tildify(project.folder, home)}
                </button>
              ) : (
                <span>No folder yet</span>
              )}
              <Button size="sm" tone="ghost" icon={FolderOpen} onClick={chooseFolder}>
                {project.folder ? "Change" : "Choose its folder"}
              </Button>
              {tree?.git && (
                <span className="inline-flex items-center gap-1 font-mono">
                  <GitBranch size={12} aria-hidden />
                  {tree.branch || "detached"}
                  {tree.ahead > 0 && <span className="text-success">↑{tree.ahead}</span>}
                  {tree.behind > 0 && <span className="text-warning">↓{tree.behind}</span>}
                </span>
              )}
            </div>
          </div>
          <div className="flex flex-wrap gap-2">
            {project.folder && (
              <>
                <Button icon={FileCode} onClick={() => tabs.open({ kind: "file", project: id, path: "" })}>
                  Files
                </Button>
                <Button icon={GitCommitHorizontal} onClick={() => tabs.open({ kind: "history", project: id })}>
                  History
                </Button>
              </>
            )}
            <Button icon={SquareTerminal} busy={busy === "shell"} onClick={newShell}>
              Shell
            </Button>
            <Button tone="primary" icon={Bot} busy={busy === "claude"} onClick={() => newSession(false)}>
              New Claude session
            </Button>
          </div>
        </header>

        <div className="grid gap-3 [grid-template-columns:repeat(auto-fit,minmax(170px,1fr))]">
          <Stat label="Today" value={formatDuration(todayMs)} />
          <Stat label="This week" value={formatDuration(week)} />
          <Stat label="Sessions running" value={String(terms.filter((x) => x.claude).length)} />
          <Stat label="Not committed" value={tree?.git ? `${changed} file${changed === 1 ? "" : "s"}` : "—"} />
        </div>

        <div role="tablist" aria-label="Project views" className="-mb-1 flex gap-0.5 border-b border-chrome-line">
          {views.map(([v, label]) => (
            <button
              key={v}
              type="button"
              role="tab"
              aria-selected={view === v}
              onClick={() => show(v)}
              className={cx("-mb-px h-8 border-b-2 px-3 text-[13px] font-medium", view === v ? "border-accent text-ink" : "border-transparent text-ink-subtle hover:text-ink")}
            >
              {label}
            </button>
          ))}
        </div>

        {view === "board" && <Board fixed={id} />}
        {view === "time" && <ProjectTime project={project} children={children} goals={projectGoals} />}
        {view === "settings" && <ProjectSettings project={project} onArchive={archive} />}
        {view === "sessions" && (
          <>
            <div className="flex flex-wrap items-center gap-2">
              <p className="min-w-0 flex-[1_1_12rem] text-[12.5px] text-ink-subtle">
                {terms.length === 0 && allEnded.length === 0
                  ? project.folder
                    ? "Nothing running and nothing ended today. Start a shell or a Claude session."
                    : "No sessions yet. Start a Claude session and Gwen asks which folder it works in."
                  : `${terms.length === 0 ? "Nothing running" : `${terms.length} running`}${allEnded.length > 0 ? ` · ${allEnded.length} ended in the last day` : ""}`}
              </p>
              <Button size="sm" icon={SquareTerminal} busy={busy === "shell"} onClick={newShell}>
                Shell
              </Button>
              <Button size="sm" icon={ShieldCheck} busy={busy === "sandbox"} onClick={() => newSession(true)}>
                New sandboxed session
              </Button>
              <Button size="sm" tone="primary" icon={Bot} busy={busy === "claude"} onClick={() => newSession(false)}>
                New Claude session
              </Button>
            </div>
            {terms.length > 0 && (
              <Card title="Running" action={<span className="text-[12px] text-ink-subtle">{terms.length}</span>}>
                <SessionRows terms={terms} ended={[]} project={id} busy={busy} onOpen={openTerm} onResume={newSession} empty="" />
              </Card>
            )}
            {allEnded.length > 0 && (
              <Card title="Ended" action={<span className="text-[12px] text-ink-subtle">in the last day</span>}>
                <SessionRows terms={[]} ended={allEnded} project={id} busy={busy} onOpen={openTerm} onResume={newSession} empty="" />
              </Card>
            )}
          </>
        )}

        {view === "overview" && (
          <div className="grid gap-4 @5xl:grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)]">
            <div className="flex min-w-0 flex-col gap-4">
              {!parent && (
                <Card title="Sub-projects" action={children.length > 0 && <span className="text-[12px] text-ink-subtle">{children.length}</span>}>
                  {children.length > 0 && (
                    <ul>
                      {children.map((c, i) => {
                        const open = d.tasks.filter((x) => x.project_id === c.id && !x.parent_id).length;
                        const ms = d.status?.today?.by_project.find((x) => x.project_id === c.id)?.worked_ms ?? 0;
                        return (
                          <li key={c.id} className={cx(i > 0 && "border-t border-chrome-line")}>
                            <button type="button" onClick={() => tabs.open({ kind: "project", project: c.id })} className="flex w-full items-center gap-2.5 px-4 py-2 text-left text-[13px] hover:bg-surface-2/40">
                              <Swatch color={c.color} size={8} />
                              <span className="min-w-0 flex-1 truncate">{c.name}</span>
                              <span className="text-[12px] text-ink-subtle tabular-nums">{open === 0 ? "No open tasks" : `${open} open`}</span>
                              {ms > 0 && <span className="w-14 text-right font-mono text-[12px] text-ink-muted">{formatDuration(ms)}</span>}
                              {w.termsOf(c.id).some((x) => x.claude) && <TermIcon claude />}
                            </button>
                          </li>
                        );
                      })}
                    </ul>
                  )}
                  <form onSubmit={addChild} className={cx("flex items-center gap-2 px-4 py-2.5", children.length > 0 && "border-t border-chrome-line")}>
                    <Input value={childName} onChange={(e) => setChildName(e.target.value)} placeholder={`Add a sub-project to ${project.name}, such as a client or an app`} aria-label="New sub-project" className="h-8 min-w-0 flex-1" />
                    <IconButton icon={Plus} label="Add sub-project" tone="secondary" size="sm" type="submit" disabled={!childName.trim()} />
                  </form>
                </Card>
              )}
              <Card
                title="Sessions running and recent"
                action={
                  allEnded.length > ended.length && (
                    <Button size="sm" tone="ghost" onClick={() => show("sessions")}>
                      All {allEnded.length} ended
                    </Button>
                  )
                }
              >
                <SessionRows terms={terms} ended={ended} project={id} busy={busy} onOpen={openTerm} onResume={newSession} empty={project.folder ? "Nothing running. Start a shell or a Claude session above." : "No sessions yet. Start a Claude session and Gwen asks which folder it works in."} />
              </Card>

              <Card
                title="Tasks"
                action={
                  <span className="flex items-center gap-1.5">
                    <Button size="sm" tone="ghost" onClick={() => show("board")}>
                      Board
                    </Button>
                    <Button size="sm" tone="primary" icon={Plus} onClick={() => ui.create({ project: id })}>
                      New task
                    </Button>
                  </span>
                }
              >
                <div className="divide-y divide-chrome-line">
                  {stages
                    .map((stage) => ({ stage, list: tasks.filter((x) => x.stage === stage) }))
                    .filter((g) => g.list.length > 0)
                    .map(({ stage, list }) => {
                      const all = showAll[stage];
                      return (
                        <div key={stage} className="min-w-0">
                          <div className="px-4 pt-3 pb-1.5 text-[11.5px] font-medium text-ink-subtle">
                            {stageName(stage)} · {list.length}
                          </div>
                          <ul>
                            {(all ? list : list.slice(0, 8)).map((x, i) => (
                              <TaskRow key={x.id} task={x} showProject={x.project_id !== id} first={i === 0} quietStatus={[stage as TaskStatusKind]} />
                            ))}
                            {list.length > 8 && (
                              <li className="border-t border-chrome-line">
                                <button type="button" onClick={() => setShowAll({ ...showAll, [stage]: !all })} className="w-full px-4 py-2 text-left text-[12px] text-ink-subtle hover:text-ink">
                                  {all ? "Show fewer" : `Show all ${list.length}`}
                                </button>
                              </li>
                            )}
                          </ul>
                        </div>
                      );
                    })}
                  {tasks.length === 0 && <p className="px-4 py-3 text-[12.5px] text-ink-subtle">No open tasks in {project.name}. New task adds one, and Gwen fits it into your day.</p>}
                </div>
              </Card>
            </div>

            <div className="flex min-w-0 flex-col gap-4">
              {projectGoals.length > 0 && (
                <Card title="Goals">
                  <ul className="px-4 py-3">
                    {projectGoals.map((g) => (
                      <li key={g.id} className="flex flex-col gap-1 py-1">
                        <span className="truncate">{g.title}</span>
                        <PaceChip pace={g.progress.pace} />
                      </li>
                    ))}
                  </ul>
                </Card>
              )}
              <Card
                title="Recent commits"
                action={
                  tree?.git && (
                    <Button size="sm" tone="ghost" onClick={() => tabs.open({ kind: "history", project: id })}>
                      Open history
                    </Button>
                  )
                }
              >
                {log.length === 0 ? (
                  <p className="px-4 py-3 text-[12.5px] text-ink-subtle">{project.folder ? (tree?.git ? "No commits yet." : "This folder isn't a git repository.") : "Choose a folder first."}</p>
                ) : (
                  <ul>
                    {log.map((c) => (
                      <li key={c.hash} className="flex items-baseline gap-2.5 border-t border-chrome-line px-4 py-2 first:border-t-0">
                        <span className="font-mono text-[11.5px] text-ink-subtle">{c.short}</span>
                        <span className="min-w-0 flex-1 truncate text-[12.5px]">{c.subject}</span>
                        <span className="shrink-0 text-[11px] text-ink-subtle">{formatAgo(c.when, d.now())}</span>
                      </li>
                    ))}
                  </ul>
                )}
              </Card>
              <Card title="Defaults">
                <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2 px-4 py-3 text-[12.5px]">
                  <dt className="text-ink-subtle">Folder</dt>
                  <dd className="flex min-w-0 items-center gap-2">
                    <span className="min-w-0 truncate font-mono" title={project.folder}>{project.folder ? tildify(project.folder, home) : "None"}</span>
                    <Button size="sm" tone="ghost" onClick={chooseFolder}>
                      {project.folder ? "Change" : "Choose"}
                    </Button>
                  </dd>
                  <dt className="text-ink-subtle">Your time</dt>
                  <dd>{d.config?.tracking.follow_tabs !== false ? "Tracked here while you're in its tabs" : "Not following your tabs (Settings › Tracking)"}</dd>
                  <dt className="text-ink-subtle">Colour</dt>
                  <dd className="flex items-center gap-2">
                    <Swatch color={project.color} size={10} />
                    <Button size="sm" tone="ghost" onClick={() => show("settings")}>
                      Change
                    </Button>
                  </dd>
                </dl>
                <div className={cx("flex gap-2 border-t border-chrome-line px-4 py-2.5")}>
                  <Button size="sm" tone="ghost" icon={ShieldCheck} busy={busy === "sandbox"} onClick={() => newSession(true)}>
                    New sandboxed session
                  </Button>
                  <Button size="sm" tone="ghost" icon={Archive} className="ml-auto" onClick={archive}>
                    Archive
                  </Button>
                </div>
              </Card>
            </div>
          </div>
        )}
      </div>
    </Scroll>
  );
}

/** Running terminals and ended Claude sessions, each with Open or Resume. */
function SessionRows({
  terms,
  ended,
  project,
  busy,
  onOpen,
  onResume,
  empty,
}: {
  terms: ReturnType<ReturnType<typeof useWork>["termsOf"]>;
  ended: wire.CodeSession[];
  project: string;
  busy: string;
  onOpen: (project: string, terminal: string) => void;
  onResume: (sandbox: boolean, resume?: string) => void;
  empty: string;
}) {
  const d = useDaemon();
  const home = useHome();
  if (terms.length === 0 && ended.length === 0) return empty ? <p className="px-4 py-3 text-[12.5px] text-ink-subtle">{empty}</p> : null;
  return (
    <ul>
      {terms.map((x) => (
        <li key={x.address} className="flex items-center gap-3 border-t border-chrome-line px-4 py-2.5 first:border-t-0">
          <TermIcon claude={x.claude} size={14} />
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <span className="truncate font-medium">{x.name}</span>
              {x.session?.state === "needs_you" && <Badge tone="idle">Needs you</Badge>}
              {x.session?.state === "working" && <Badge tone="break">Working</Badge>}
              {x.session?.sandbox && <Badge tone="neutral" icon={ShieldCheck}>Sandboxed</Badge>}
            </div>
            <div className="truncate text-[12px] text-ink-subtle">{x.claude ? x.session?.step || x.session?.last_message || "Claude" : tildify(x.cwd, home)}</div>
          </div>
          <StateGlyph state={x.session?.state ?? x.status} unread={x.session?.unread} shell={!x.claude} />
          <Button size="sm" onClick={() => onOpen(project, x.address)}>
            Open
          </Button>
        </li>
      ))}
      {ended.map((s) => (
        <li key={s.id} className="flex items-center gap-3 border-t border-chrome-line px-4 py-2.5 first:border-t-0">
          <TermIcon claude size={14} />
          <div className="min-w-0 flex-1">
            <div className="truncate font-medium text-ink-muted">{s.last_message ? s.last_message.slice(0, 80) : s.name}</div>
            <div className="text-[12px] text-ink-subtle">
              Ended {formatAgo(s.updated_at, d.now())}
              {s.sandbox && " · sandboxed"}
            </div>
          </div>
          {s.claude_session_id && (
            <Button size="sm" busy={busy === s.claude_session_id} onClick={() => onResume(s.sandbox, s.claude_session_id)}>
              Resume
            </Button>
          )}
        </li>
      ))}
    </ul>
  );
}

/** Time: its days, and its goals' meters; sub-projects count in and show in their own colours. */
function ProjectTime({ project, children, goals }: { project: wire.Project; children: wire.Project[]; goals: wire.Goal[] }) {
  const d = useDaemon();
  const scope = { ids: new Set([project.id, ...children.map((c) => c.id)]), name: project.name };
  return (
    <>
      <History scope={scope} />
      {goals.length > 0 && (
        <Card title="Goals">
          <ul>
            {goals.map((g) => (
              <li key={g.id} className="flex flex-col gap-1.5 border-t border-chrome-line px-4 py-3 first:border-t-0">
                <div className="flex items-center gap-2">
                  <span className="min-w-0 flex-1 truncate text-[13px] font-medium">{g.title}</span>
                  <span className="shrink-0 text-[12px] text-ink-subtle tabular-nums">{goalAmount(g)}</span>
                  <PaceChip pace={g.progress.pace} />
                </div>
                <GoalMeter g={g} today={d.today()} />
              </li>
            ))}
          </ul>
        </Card>
      )}
    </>
  );
}

/** Settings: the same fields as Manage projects' Edit project, saved in place, and archiving it. */
function ProjectSettings({ project, onArchive }: { project: wire.Project; onArchive: () => void }) {
  const d = useDaemon();
  const f = useProjectForm(project);
  const save = () => f.dirty && f.save();
  return (
    <div className="grid items-start gap-4 @5xl:grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)]">
      <Card
        title="Project"
        action={
          <span className="flex items-center gap-1.5">
            {f.dirty && (
              <Button size="sm" tone="ghost" onClick={f.reset}>
                Undo
              </Button>
            )}
            <Button size="sm" tone="primary" onClick={save} disabled={!f.dirty || !f.name.trim()} busy={f.saving}>
              Save
            </Button>
          </span>
        }
      >
        <div className="px-4 py-4">
          <ProjectFields form={f} onEnter={save} />
        </div>
      </Card>
      <div className="flex min-w-0 flex-col gap-4">
        <Card title="How it works">
          <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 px-4 py-3 text-[12.5px]">
            <dt className="text-ink-subtle">Your time</dt>
            <dd>{d.config?.tracking.follow_tabs !== false ? "Tracked here while you're in its tabs" : "Not following your tabs (Settings › Tracking)"}</dd>
            <dt className="text-ink-subtle">New sessions</dt>
            <dd>Claude Code in its folder, not sandboxed; the Sessions view also starts sandboxed ones</dd>
          </dl>
        </Card>
        <Card title="Archive">
          <div className="flex items-center gap-3 px-4 py-3">
            <p className="min-w-0 flex-1 text-[12.5px] text-ink-subtle">It leaves the sidebar. Its tasks, time and folder stay, and Manage projects brings it back.</p>
            <Button size="sm" icon={Archive} onClick={onArchive}>
              Archive
            </Button>
          </div>
        </Card>
      </div>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-xl border border-line bg-surface-1 px-4 py-3">
      <div className="text-[11.5px] text-ink-subtle">{label}</div>
      <div className="mt-0.5 text-[18px] font-semibold tabular-nums">{value}</div>
    </div>
  );
}

function Card({ title, action, children }: { title: string; action?: React.ReactNode; children: React.ReactNode }) {
  return (
    <section className="overflow-hidden rounded-xl border border-line bg-surface-1">
      <div className="flex h-11 items-center gap-2 border-b border-chrome-line px-4">
        <h2 className="text-[14px] font-semibold">{title}</h2>
        <div className="ml-auto">{action}</div>
      </div>
      {children}
    </section>
  );
}
