import { Plus } from "lucide-react";
import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";
import type { wire } from "../api";
import AgentChat from "../agent/AgentChat";
import { useAgentStatus } from "../agent/agent";
import AssistantChat from "../components/AssistantChat";
import { Button, Empty, cx } from "../components/ui";
import { useDaemon } from "../daemon";
import { formatAgo } from "../format";
import TaskRow from "../task/TaskRow";
import { useTaskUI } from "../task/TaskUI";
import { stageOrder } from "../task/status";
import Changes from "../work/Changes";
import FileTree from "../work/FileTree";
import { StateGlyph, TermIcon } from "./bits";
import { parentOf } from "./projects";
import { tabProject, useTabs } from "./tabs";
import { useWork } from "./work";

// The side panel (plan.md 3.15): on Today and Gwen's screens, Gwen and
// Activity; in a project, its Changes, Files, Tasks, and Gwen.

type PanelTab = "gwen" | "activity" | "changes" | "files" | "tasks";

/** Where the panel is shown: Today and screens, a project's tabs, a wide tab (history, files) that wants the room, or Gwen's
 * own chat screen, which has its own right column and so starts with the panel closed. */
type Context = "day" | "project" | "wide" | "gwen";
const closedAt: Record<Context, boolean> = { day: true, project: true, wide: false, gwen: false };

interface PanelState {
  open: boolean;
  toggle: () => void;
  tab: (project: boolean) => PanelTab;
  setTab: (project: boolean, t: PanelTab) => void;
}

const PanelContext = createContext<PanelState | null>(null);
const storageKey = "gwen.panel.v2";

interface Saved {
  open: Record<Context, boolean>;
  day: PanelTab;
  project: PanelTab;
}

function load(): Saved {
  try {
    const v = JSON.parse(localStorage.getItem(storageKey) ?? "null");
    if (v && v.open && typeof v.open.day === "boolean") return v;
  } catch {
    // storage is a convenience
  }
  return { open: { ...closedAt }, day: "gwen", project: "changes" };
}

export function PanelProvider({ children }: { children: ReactNode }) {
  const tabs = useTabs();
  const a = tabs.activeTab;
  const context: Context =
    a.kind === "history" || (a.kind === "file" && !a.path) ? "wide" : tabProject(a) ? "project" : a.kind === "screen" && a.screen === "gwen" && (a.sub ?? "chat") === "chat" ? "gwen" : "day";
  const [s, setS] = useState(load);
  const persist = (v: typeof s) => {
    try {
      localStorage.setItem(storageKey, JSON.stringify(v));
    } catch {
      // storage is a convenience
    }
    return v;
  };
  const save = (v: typeof s) => setS(persist(v));
  const toggle = useCallback(() => setS((v) => persist({ ...v, open: { ...v.open, [context]: !(v.open[context] ?? closedAt[context]) } })), [context]);
  const value = useMemo<PanelState>(
    () => ({
      open: s.open[context] ?? closedAt[context],
      toggle,
      tab: (project) => (project ? s.project : s.day),
      setTab: (project, t) => save({ ...s, [project ? "project" : "day"]: t }),
    }),
    [s, toggle],
  );
  return <PanelContext.Provider value={value}>{children}</PanelContext.Provider>;
}

export function usePanel(): PanelState {
  const p = useContext(PanelContext);
  if (!p) throw new Error("usePanel outside PanelProvider");
  return p;
}

const widthKey = "gwen.panel.width";
const defaultWidth = 340;
const minWidth = 280;
const maxWidth = () => Math.max(minWidth, Math.min(760, Math.round(window.innerWidth * 0.6)));
const clampWidth = (w: number) => Math.round(Math.min(maxWidth(), Math.max(minWidth, w)));

function savedWidth(): number {
  try {
    const w = Number(localStorage.getItem(widthKey));
    return w > 0 ? clampWidth(w) : defaultWidth;
  } catch {
    return defaultWidth; // storage is a convenience
  }
}

/** The panel's left edge: drag it to make the panel wider or narrower, double-click for the usual width; arrows work too. */
function ResizeEdge({ width, onWidth }: { width: number; onWidth: (w: number) => void }) {
  const [dragging, setDragging] = useState(false);
  return (
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label="Side panel width: drag, or use the arrow keys"
      aria-valuenow={width}
      aria-valuemin={minWidth}
      aria-valuemax={maxWidth()}
      tabIndex={0}
      title="Drag to resize · double-click for the usual width"
      onPointerDown={(e) => {
        e.preventDefault();
        (e.target as HTMLElement).setPointerCapture(e.pointerId);
        setDragging(true);
      }}
      onPointerMove={(e) => dragging && onWidth(clampWidth(window.innerWidth - e.clientX))}
      onPointerUp={() => setDragging(false)}
      onPointerCancel={() => setDragging(false)}
      onDoubleClick={() => onWidth(defaultWidth)}
      onKeyDown={(e) => {
        if (e.key === "ArrowLeft") onWidth(clampWidth(width + 24));
        else if (e.key === "ArrowRight") onWidth(clampWidth(width - 24));
        else return;
        e.preventDefault();
      }}
      className="group absolute inset-y-0 -left-[3px] z-20 w-[7px] cursor-col-resize touch-none focus:outline-none"
    >
      <span className={cx("absolute inset-y-0 left-[3px] w-px transition-colors", dragging ? "bg-accent" : "group-hover:bg-line-3 group-focus-visible:bg-accent")} />
    </div>
  );
}

export default function SidePanel() {
  const [width, setWidthState] = useState(savedWidth);
  const setWidth = (w: number) => {
    setWidthState(w);
    try {
      localStorage.setItem(widthKey, String(w));
    } catch {
      // storage is a convenience
    }
  };
  const d = useDaemon();
  const tabs = useTabs();
  const w = useWork();
  const panel = usePanel();
  const projectId = tabProject(tabs.activeTab);
  const project = d.projects.find((p) => p.id === projectId) ?? null;
  const inProject = !!project;
  const projectIds = new Set(project ? [project.id, ...d.projects.filter((p) => parentOf(p) === project.id && !p.archived_at).map((p) => p.id)] : []);
  const tasks = d.tasks.filter((t) => !t.parent_id && projectIds.has(t.project_id ?? ""));
  const choices: { id: PanelTab; label: string; count?: number }[] = inProject
    ? [
        { id: "changes", label: "Changes" },
        { id: "files", label: "Files" },
        { id: "tasks", label: "Tasks", count: tasks.length },
        { id: "gwen", label: "Gwen" },
      ]
    : [
        { id: "gwen", label: "Gwen" },
        { id: "activity", label: "Activity", count: w.sessions.filter((s) => s.state !== "ended").length },
      ];
  const want = panel.tab(inProject);
  // A project without a folder has no files or changes to show, so its panel opens on its tasks.
  const folderless = inProject && !project.folder && (want === "changes" || want === "files");
  const tab = folderless ? "tasks" : choices.some((c) => c.id === want) ? want : choices[0].id;
  const selectedFile = tabs.activeTab.kind === "file" ? tabs.activeTab.path : undefined;

  return (
    <aside aria-label="Side panel" className="relative flex shrink-0 flex-col border-l border-chrome-line bg-chrome" style={{ width }}>
      <ResizeEdge width={width} onWidth={setWidth} />
      <div role="tablist" className="flex h-11 shrink-0 items-center gap-0.5 border-b border-chrome-line px-2.5">
        {choices.map((c) => (
          <button
            key={c.id}
            type="button"
            role="tab"
            aria-selected={tab === c.id}
            onClick={() => panel.setTab(inProject, c.id)}
            className={cx("flex h-7 items-center gap-1.5 rounded-[7px] px-2.5 font-medium", tab === c.id ? "bg-surface-2 text-ink" : "text-ink-subtle hover:text-ink")}
          >
            {c.label}
            {!!c.count && <span className="rounded-full bg-surface-3 px-1.5 text-[10.5px] text-ink-muted tabular-nums">{c.count}</span>}
          </button>
        ))}
      </div>
      <div className="min-h-0 flex-1">
        {tab === "gwen" && <GwenTab project={project?.id ?? null} />}
        {tab === "activity" && <Activity />}
        {tab === "changes" && project && (project.folder ? <Changes folder={project.folder} onOpenFile={(path) => tabs.open({ kind: "file", project: project.id, path })} /> : <NoFolder />)}
        {tab === "files" &&
          project &&
          (project.folder ? <FileTree folder={project.folder} selected={selectedFile} onOpen={(path) => tabs.open({ kind: "file", project: project.id, path })} /> : <NoFolder />)}
        {tab === "tasks" && project && <ProjectTasks project={project} tasks={tasks} />}
      </div>
    </aside>
  );
}

/** Gwen: her agent with the open project's chats (or the chats for everywhere), else the planning assistant with why. */
function GwenTab({ project }: { project: string | null }) {
  const status = useAgentStatus();
  if (!status) return null;
  if (!status.available)
    return (
      <div className="flex h-full min-h-0 flex-col">
        {status.enabled && status.reason && <p className="shrink-0 border-b border-chrome-line px-3.5 py-2 text-[11.5px] leading-snug text-ink-subtle">{status.reason}</p>}
        <div className="min-h-0 flex-1">
          <AssistantChat panel />
        </div>
      </div>
    );
  return <AgentChat key={project ?? "everywhere"} place={project ?? "everywhere"} />;
}

function NoFolder() {
  return (
    <Empty title="No folder yet" className="px-6 py-10">
      Give this project its folder on its home tab, and its files and changes show here.
    </Empty>
  );
}

/** A session's state as a few words, for a session with no step or message to show. */
const sessionSays: Record<string, string> = {
  starting: "Starting up",
  working: "Working",
  needs_you: "Needs you",
  done: "Finished",
  idle: "Waiting for you",
  error: "Stopped with an error",
  ended: "Ended",
};

/** Activity: the sessions on this laptop, newest change first. */
function Activity() {
  const d = useDaemon();
  const w = useWork();
  const tabs = useTabs();
  const now = d.now();
  const list = [...w.sessions].sort((a, b) => b.updated_at - a.updated_at).slice(0, 30);
  if (list.length === 0)
    return (
      <Empty title="No sessions yet" className="px-6 py-10">
        Claude sessions you start in a project show here as they work.
      </Empty>
    );
  return (
    <ul className="h-full overflow-y-auto py-1">
      {list.map((s) => {
        const project = w.projectOf(s.cwd);
        const name = d.projects.find((p) => p.id === project)?.name ?? s.name;
        return (
          <li key={s.id}>
            <button
              type="button"
              disabled={!s.terminal || s.state === "ended"}
              onClick={() => tabs.open({ kind: "term", project: project ?? "", terminal: s.terminal })}
              className="flex w-full items-start gap-2.5 px-4 py-2.5 text-left hover:bg-surface-1 disabled:hover:bg-transparent"
            >
              <TermIcon claude size={14} />
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="truncate font-medium">{name}</span>
                  <StateGlyph state={s.state} unread={s.unread} />
                  <span className="ml-auto shrink-0 text-[11px] text-ink-subtle">{formatAgo(s.updated_at, now)}</span>
                </div>
                <div className="mt-0.5 truncate text-[12px] text-ink-muted">{s.state === "ended" ? sessionSays.ended : s.step || s.last_message || sessionSays[s.state] || "No update yet"}</div>
              </div>
            </button>
          </li>
        );
      })}
    </ul>
  );
}

/** Tasks: this project's and its sub-projects' open tasks, with the shared task actions. */
function ProjectTasks({ project, tasks }: { project: wire.Project; tasks: wire.Task[] }) {
  const ui = useTaskUI();
  const stages = ["doing", "todo", ...stageOrder.filter((stage) => stage !== "doing" && stage !== "todo")];
  const sorted = [...tasks].sort((a, b) => stages.indexOf(a.stage) - stages.indexOf(b.stage) || b.priority - a.priority);

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center gap-2 border-b border-chrome-line px-3 py-2.5">
        <span className="min-w-0 flex-1 truncate text-[12px] text-ink-subtle">{tasks.length === 0 ? "No open tasks" : `${tasks.length} open`}</span>
        <Button size="sm" tone="primary" icon={Plus} onClick={() => ui.create({ project: project.id })}>
          New task
        </Button>
      </div>
      <ul className="min-h-0 flex-1 overflow-y-auto py-1">
        {sorted.map((x, i) => (
          <TaskRow key={x.id} task={x} compact showProject={x.project_id !== project.id} first={i === 0} />
        ))}
        {tasks.length === 0 && <li className="px-4 py-6 text-[12.5px] text-ink-subtle">No open tasks in {project.name}.</li>}
      </ul>
    </div>
  );
}
