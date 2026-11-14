import { Search } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { App } from "../api";
import Face from "../components/Face";
import { cx } from "../components/ui";
import { useDaemon } from "../daemon";
import { useTaskUI } from "../task/TaskUI";
import { useTracking } from "../tracking";
import { startShell } from "../work/terminals";
import { Swatch, TermIcon } from "./bits";
import { startClaude } from "./ProjectHome";
import { projectChoices, projectPath } from "./projects";
import { tabProject, useTabs, type Tab } from "./tabs";
import { useWork } from "./work";

// Ctrl K (plan.md 3.15, board 16): every screen, project, terminal, task,
// file of the project in front, command, and setting, plus "Ask Gwen".

interface Item {
  id: string;
  group: string;
  label: string;
  hint?: string;
  taskProject?: string;
  icon?: ReactNode;
  mono?: boolean;
  run: () => void;
}

const settingsSections: [string, string][] = [
  ["connections", "All connections"],
  ["calendar", "Google Calendar"],
  ["phone", "Phone and sessions"],
  ["sync", "Sync hub"],
  ["tracking", "Tracking"],
  ["nudges", "Nudges"],
  ["planner", "Planner"],
  ["methods", "Methods"],
  ["ai", "AI, voice, personality"],
  ["memory", "Memory"],
  ["appearance", "Appearance"],
  ["advanced", "Advanced"],
  ["about", "About"],
];

/** How well text matches a query: higher is better, 0 is no match. Words may come in any order. */
function score(text: string, q: string): number {
  const t = text.toLowerCase();
  let total = 0;
  for (const word of q.toLowerCase().split(/\s+/).filter(Boolean)) {
    const at = t.indexOf(word);
    if (at < 0) return 0;
    total += at === 0 ? 3 : t[at - 1] === " " || t[at - 1] === "/" ? 2 : 1;
  }
  return total || 1;
}

export default function Palette({ onClose }: { onClose: () => void }) {
  const d = useDaemon();
  const w = useWork();
  const t = useTracking();
  const ui = useTaskUI();
  const tabs = useTabs();
  const [q, setQ] = useState("");
  const [at, setAt] = useState(0);
  const [files, setFiles] = useState<string[]>([]);
  const list = useRef<HTMLDivElement>(null);
  const project = d.projects.find((p) => p.id === tabProject(tabs.activeTab));
  const projects = useMemo(() => projectChoices(d.projects), [d.projects]);
  const today = d.today();

  useEffect(() => {
    if (!project?.folder) return;
    let live = true;
    App.RepoTree(project.folder).then(
      (tree) => live && setFiles((tree.files ?? []).map((f) => f.path)),
      () => {},
    );
    return () => {
      live = false;
    };
  }, [project?.folder]);

  const items = useMemo<Item[]>(() => {
    const open = (tab: Tab) => () => tabs.open(tab);
    const out: Item[] = [];
    const screens: [string, Tab][] = [
      ["Today", { kind: "today" }],
      ["Inbox › Needs you", { kind: "screen", screen: "inbox", sub: "needs" }],
      ["Inbox › To process", { kind: "screen", screen: "inbox", sub: "captured" }],
      ["Gwen › Chat", { kind: "screen", screen: "gwen", sub: "chat" }],
      ["Gwen › Memory", { kind: "screen", screen: "gwen", sub: "memory" }],
      ["Plan › Day", { kind: "screen", screen: "plan", sub: "day" }],
      ["Plan › Board", { kind: "screen", screen: "plan", sub: "board" }],
      ["Plan › Goals", { kind: "screen", screen: "plan", sub: "goals" }],
      ["Review › Week", { kind: "screen", screen: "review", sub: "week" }],
      ["Review › Stats", { kind: "screen", screen: "review", sub: "stats" }],
      ["Review › History", { kind: "screen", screen: "review", sub: "history" }],
      ["Manage projects", { kind: "screen", screen: "projects" }],
    ];
    for (const [label, tab] of screens) out.push({ id: `screen:${label}`, group: "Screens", label, run: open(tab) });
    for (const [id, label] of settingsSections) out.push({ id: `settings:${id}`, group: "Settings", label: `Settings › ${label}`, run: open({ kind: "screen", screen: "settings", param: id }) });
    for (const { id, label } of projects) {
      const p = d.projects.find((p) => p.id === id);
      if (!p) continue;
      out.push({ id: `project:${p.id}`, group: "Projects", label, hint: "open its home", icon: <Swatch color={p.color} />, run: open({ kind: "project", project: p.id }) });
      if (p.folder) {
        out.push({ id: `claude:${p.id}`, group: "Commands", label: `New Claude session in ${label}`, icon: <TermIcon claude />, run: () => startClaude(p.folder, p.id, {}, (proj, terminal) => tabs.open({ kind: "term", project: proj, terminal })).then(w.refreshSessions, d.fail) });
        out.push({ id: `shell:${p.id}`, group: "Commands", label: `New shell in ${label}`, icon: <TermIcon claude={false} />, run: () => startShell(p.folder, p.id).then((x) => tabs.open({ kind: "term", project: p.id, terminal: x.address }), d.fail) });
        out.push({ id: `files:${p.id}`, group: "Commands", label: `Files of ${label}`, run: open({ kind: "file", project: p.id, path: "" }) });
        out.push({ id: `history:${p.id}`, group: "Commands", label: `History of ${label}`, run: open({ kind: "history", project: p.id }) });
      }
      out.push({ id: `track:${p.id}`, group: "Commands", label: `Start tracking ${label}`, run: () => (t.state === "off" ? t.clockIn(p.id, null) : t.switchTo(p.id, null)) });
    }
    for (const x of w.terminals) {
      const name = projectPath(d.projects.find((p) => p.id === x.project), d.projects);
      out.push({ id: `term:${x.address}`, group: "Terminals", label: name ? `${name} · ${x.name}` : x.name, icon: <TermIcon claude={x.claude} />, run: open({ kind: "term", project: x.project ?? "", terminal: x.address }) });
    }
    const tasks = d.tasks.filter((x) => x.status === "open" && !x.parent_id && !x.rrule && (!x.occurrence_day || x.occurrence_day <= today));
    for (const p of [...projects, { id: "", label: "No project" }]) {
      for (const x of tasks.filter((x) => (x.project_id ?? "") === p.id)) {
        out.push({ id: `task:${x.id}`, group: "Tasks", taskProject: p.id, label: x.title, hint: t.isTracking(x.id) ? "Tracking" : "Start", run: () => ui.start(x) });
      }
    }
    if (project) for (const f of files) out.push({ id: `file:${f}`, group: "Files", label: f, hint: project.name, mono: true, run: open({ kind: "file", project: project.id, path: f }) });
    if (t.state === "off") out.push({ id: "clock-in", group: "Commands", label: "Clock in", run: () => t.clockIn(d.lastAttribution.project_id, d.lastAttribution.task_id) });
    if (t.working) out.push({ id: "break", group: "Commands", label: "Start a break", run: t.breakStart });
    if (t.onBreak) out.push({ id: "back", group: "Commands", label: "End the break", run: t.breakEnd });
    if (t.state !== "off") {
      out.push({ id: "clock-out", group: "Commands", label: "Clock out", run: t.clockOut });
      out.push({ id: "snooze", group: "Commands", label: "Snooze nudges", run: t.snooze });
    }
    return out;
  }, [d, w, t, ui, tabs, project, projects, today, files]);

  const shown = useMemo(() => {
    const query = q.trim();
    const groups = ["Projects", "Terminals", "Screens", "Tasks", "Files", "Commands", "Settings"];
    const limit: Record<string, number> = { Files: 8, Tasks: 6, Commands: 6, Settings: 4 };
    let ranked: Item[];
    if (!query) ranked = items.filter((i) => i.group === "Screens" || i.group === "Projects" || i.group === "Terminals");
    else
      ranked = items
        .map((i) => ({ i, s: score(i.label, query) }))
        .filter((x) => x.s > 0)
        .sort((a, b) => b.s - a.s)
        .map((x) => x.i);
    const out: Item[] = [];
    for (const g of groups) {
      const matches = ranked.filter((i) => i.group === g).slice(0, query ? (limit[g] ?? 6) : 12);
      if (g === "Tasks") for (const p of [...projects, { id: "", label: "No project" }]) out.push(...matches.filter((i) => i.taskProject === p.id));
      else out.push(...matches);
    }
    if (query)
      out.push({
        id: "ask",
        group: "Gwen",
        label: `Ask Gwen about "${query}"`,
        hint: "Ctrl Enter",
        icon: <Face mood="smiling" className="size-4" />,
        run: () => tabs.open({ kind: "screen", screen: "gwen", sub: "chat", param: query }),
      });
    return out;
  }, [items, q, tabs, projects]);

  useEffect(() => setAt(0), [q]);
  useEffect(() => {
    list.current?.querySelector(`[data-at="${at}"]`)?.scrollIntoView({ block: "nearest" });
  }, [at]);

  function run(i: Item | undefined) {
    if (!i) return;
    onClose();
    i.run();
  }

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center bg-black/55 pt-[12vh]" onMouseDown={onClose}>
      <div role="dialog" aria-label="Everything" className="animate-pop w-[600px] max-w-[92vw] overflow-hidden rounded-[14px] border border-line-strong bg-surface-1 shadow-2xl shadow-black" onMouseDown={(e) => e.stopPropagation()}>
        <div className="flex h-12 items-center gap-2.5 border-b border-line px-3.5">
          <Search size={16} aria-hidden className="text-ink-subtle" />
          <input
            autoFocus
            value={q}
            onChange={(e) => setQ(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Escape") onClose();
              else if (e.key === "ArrowDown") (e.preventDefault(), setAt((v) => Math.min(v + 1, shown.length - 1)));
              else if (e.key === "ArrowUp") (e.preventDefault(), setAt((v) => Math.max(v - 1, 0)));
              else if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) (e.preventDefault(), run(shown.find((i) => i.id === "ask")));
              else if (e.key === "Enter") (e.preventDefault(), run(shown[at]));
            }}
            aria-label="Search everything"
            placeholder="Screens, projects, sessions, tasks, files, commands…"
            className="min-w-0 flex-1 bg-transparent text-[15px] text-ink outline-none placeholder:text-ink-faint"
          />
          <span className="text-[11px] text-ink-subtle">Esc closes</span>
        </div>
        <div ref={list} className="max-h-[56vh] overflow-y-auto px-1.5 pt-1 pb-2">
          {shown.length === 0 && <p className="px-3 py-6 text-center text-ink-subtle">Nothing matches.</p>}
          {shown.map((i, n) => (
            <div key={i.id}>
              {(n === 0 || shown[n - 1].group !== i.group || shown[n - 1].taskProject !== i.taskProject) && (
                <div className="truncate px-2.5 pt-2 pb-0.5 text-[11px] font-medium text-ink-subtle">
                  {i.group === "Tasks" ? `Tasks · ${projects.find((p) => p.id === i.taskProject)?.label ?? "No project"}` : i.group}
                </div>
              )}
              <button
                type="button"
                data-at={n}
                onMouseMove={() => setAt(n)}
                onClick={() => run(i)}
                className={cx("flex h-8 w-full items-center gap-2.5 rounded-[7px] px-2.5 text-left text-[12.5px]", n === at ? "bg-surface-3 text-ink" : "text-ink-muted")}
              >
                {i.icon}
                <span className={cx("min-w-0 flex-1 truncate", i.mono && "font-mono text-[12px]")}>{i.label}</span>
                {i.hint && <span className="shrink-0 text-[11.5px] text-ink-subtle">{i.hint}</span>}
              </button>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
