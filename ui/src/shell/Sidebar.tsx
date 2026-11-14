import { BellOff, CalendarDays, ChartColumn, Coffee, Flame, FolderOpen, Inbox, Play, Plus, Search, SlidersHorizontal, Smartphone, Sparkles, Square, Sun, type LucideIcon } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { App, apiError, onEvent, wire } from "../api";
import BrandMark from "../components/BrandMark";
import ProgressRing from "../components/ProgressRing";
import { Button, DotSelect, Field, IconButton, Input, Modal, Select, cx, stateColor, stateLabel, unassignedColor } from "../components/ui";
import { useDaemon, useTick } from "../daemon";
import { addDays, clockFace, formatAgo, formatDuration, formatTime, parseDuration } from "../format";
import { useTracking } from "../tracking";
import { StateGlyph, Swatch, TermIcon, tildify, useHome } from "./bits";
import { projectChoices, projectPath, projectTree } from "./projects";
import { tabId, useTabs, type ScreenId, type Tab } from "./tabs";
import { useWork } from "./work";

// The sidebar (plan.md 3.15, board 1): the Now card, Gwen's places, your
// projects with their live sessions, Settings, and the phone.

const nav: { id: "today" | ScreenId; label: string; icon: LucideIcon; tab: Tab }[] = [
  { id: "today", label: "Today", icon: Sun, tab: { kind: "today" } },
  { id: "inbox", label: "Inbox", icon: Inbox, tab: { kind: "screen", screen: "inbox" } },
  { id: "gwen", label: "Gwen", icon: Sparkles, tab: { kind: "screen", screen: "gwen" } },
  { id: "plan", label: "Plan", icon: CalendarDays, tab: { kind: "screen", screen: "plan" } },
  { id: "review", label: "Review", icon: ChartColumn, tab: { kind: "screen", screen: "review" } },
];

export default function Sidebar({ onSearch }: { onSearch: () => void }) {
  const d = useDaemon();
  const w = useWork();
  const tabs = useTabs();
  const [adding, setAdding] = useState(false);
  const [showAll, setShowAll] = useState(false);
  const active = tabs.activeTab;
  const activeProject = active.kind === "project" || active.kind === "term" || active.kind === "file" || active.kind === "history" ? active.project : null;

  const worked = useMemo(() => new Map((d.status?.today?.by_project ?? []).map((p) => [p.project_id ?? "", p.worked_ms])), [d.status]);
  const projects = useMemo(() => {
    const score = (p: wire.Project) => (w.termsOf(p.id).length > 0 ? 4 : 0) + ((worked.get(p.id) ?? 0) > 0 ? 2 : 0) + (p.folder ? 1 : 0);
    const sorted = [...d.projects].sort((a, b) => score(b) - score(a) || a.name.localeCompare(b.name));
    // Sub-projects sit under their project, so only top-level ones are ranked and counted here.
    return projectTree(sorted);
  }, [d.projects, w, worked]);
  const shown = showAll ? projects : projects.slice(0, 8);

  const projectRow = (p: wire.Project, depth: number) => {
    const terms = w.termsOf(p.id);
    const ms = worked.get(p.id) ?? 0;
    const home: Tab = { kind: "project", project: p.id };
    const on = isActive(home);
    return (
      <div key={p.id}>
        <button
          type="button"
          aria-current={on ? "page" : undefined}
          onClick={() => tabs.open(home)}
          title={p.folder || "No folder yet"}
          className={cx(
            "flex w-full items-center gap-2.5 rounded-[7px] px-2.5 text-left",
            depth === 0 ? "h-7" : "h-[26px] text-[12.5px]",
            on ? "bg-surface-2 text-ink shadow-[inset_0_0_0_1px_var(--color-line)]" : activeProject === p.id ? "text-ink" : "text-ink-muted hover:bg-surface-1 hover:text-ink",
          )}
        >
          <Swatch color={p.color} size={depth === 0 ? 8 : 7} />
          <span className={cx("min-w-0 flex-1 truncate", depth === 0 && "font-medium")}>{p.name}</span>
          {ms > 0 && <span className="font-mono text-[11.5px] text-ink-subtle">{formatDuration(ms)}</span>}
        </button>
        {terms.length > 0 && (
          <div className="mb-1 ml-[17px] flex flex-col gap-px border-l border-line pl-1.5">
            {terms.map((t) => {
              const tab: Tab = { kind: "term", project: p.id, terminal: t.address };
              const tOn = isActive(tab);
              const state = t.session?.state ?? t.status;
              return (
                <button
                  key={t.address}
                  type="button"
                  aria-current={tOn ? "page" : undefined}
                  onClick={() => tabs.open(tab)}
                  className={cx(
                    "flex h-[26px] items-center gap-2 rounded-md px-2 text-left text-[12.5px]",
                    tOn ? "bg-surface-2 text-ink shadow-[inset_0_0_0_1px_var(--color-line)]" : "text-ink-muted hover:bg-surface-1 hover:text-ink",
                  )}
                >
                  <TermIcon claude={t.claude} />
                  <span className="min-w-0 flex-1 truncate">{t.name}</span>
                  <StateGlyph state={state} unread={t.session?.unread} shell={!t.claude} />
                </button>
              );
            })}
          </div>
        )}
      </div>
    );
  };

  const isActive = (t: Tab) => tabId(t) === tabs.active;

  return (
    <aside aria-label="Sidebar" className="flex w-64 shrink-0 flex-col border-r border-chrome-line bg-chrome px-2.5 pt-2 pb-2.5 text-[13px]">
      <div className="flex h-9 items-center gap-2 pr-1 pl-1.5">
        <BrandMark className="size-[22px]" />
        <span className="flex-1 text-[14px] font-semibold">Gwen</span>
        <IconButton icon={Search} label="Search everything (Ctrl K)" size="sm" onClick={onSearch} />
      </div>

      <NowCard />

      <nav aria-label="Gwen" className="flex flex-col gap-px">
        {nav.map((n) => {
          const on = n.id === "today" ? active.kind === "today" : active.kind === "screen" && active.screen === n.id;
          const count = n.id === "inbox" ? w.needs.length : 0;
          return (
            <button
              key={n.id}
              type="button"
              aria-current={on ? "page" : undefined}
              onClick={() => tabs.open(n.tab)}
              className={cx(
                "flex h-[30px] items-center gap-2.5 rounded-[7px] px-2.5 text-left font-medium",
                on ? "bg-surface-2 text-ink shadow-[inset_0_0_0_1px_var(--color-line)]" : "text-ink-muted hover:bg-surface-1 hover:text-ink",
              )}
            >
              <n.icon size={16} strokeWidth={1.8} aria-hidden />
              {n.label}
              {count > 0 && (
                <span className="ml-auto inline-flex h-[18px] min-w-[18px] items-center justify-center rounded-full bg-warning/15 px-1.5 text-[11px] font-semibold text-warning tabular-nums">
                  {count}
                </span>
              )}
            </button>
          );
        })}
      </nav>

      <div className="mt-[18px] mb-1.5 ml-2.5 flex items-center text-[11.5px] font-medium tracking-[.02em] text-ink-subtle">
        Projects
        <IconButton icon={Plus} label="New project" size="sm" className="ml-auto" onClick={() => setAdding(true)} />
      </div>

      <div className="-mr-1 flex min-h-0 flex-1 flex-col gap-px overflow-y-auto pr-1">
        {shown.map(({ project, children }) => (
          <div key={project.id}>
            {projectRow(project, 0)}
            {children.length > 0 && <div className="ml-[13px] flex flex-col border-l border-line/60 pl-1">{children.map((c) => projectRow(c, 1))}</div>}
          </div>
        ))}
        {projects.length > 8 && (
          <button type="button" onClick={() => setShowAll((v) => !v)} className="h-7 rounded-[7px] px-2.5 text-left text-[12px] text-ink-subtle hover:bg-surface-1 hover:text-ink">
            {showAll ? "Show fewer" : `Show all ${projects.length}`}
          </button>
        )}
        {projects.length === 0 && <p className="px-2.5 py-1 text-[12px] text-ink-subtle">No projects yet. Add one with +, and give it its folder.</p>}
        <button
          type="button"
          onClick={() => tabs.open({ kind: "screen", screen: "projects" })}
          className="h-7 rounded-[7px] px-2.5 text-left text-[12px] text-ink-subtle hover:bg-surface-1 hover:text-ink"
        >
          Manage projects
        </button>
      </div>

      <ClaudeUsage />
      <div className="mt-2 flex items-center gap-1 border-t border-chrome-line px-0.5 pt-2">
        <button
          type="button"
          aria-current={active.kind === "screen" && active.screen === "settings" ? "page" : undefined}
          onClick={() => tabs.open({ kind: "screen", screen: "settings" })}
          className={cx(
            "flex h-7 items-center gap-2 rounded-[7px] px-2",
            active.kind === "screen" && active.screen === "settings" ? "bg-surface-2 text-ink shadow-[inset_0_0_0_1px_var(--color-line)]" : "text-ink-muted hover:bg-surface-1 hover:text-ink",
          )}
        >
          <SlidersHorizontal size={15} strokeWidth={1.8} aria-hidden />
          Settings
        </button>
        <button
          type="button"
          onClick={() => tabs.open({ kind: "screen", screen: "settings", param: "phone" })}
          className="ml-auto inline-flex items-center gap-1.5 rounded-md px-1.5 py-1 text-[12px] text-ink-subtle hover:text-ink"
          title={w.remote?.published ? `${w.remote.devices} phone${w.remote.devices === 1 ? "" : "s"} paired` : "The phone app is off"}
        >
          <Smartphone size={14} strokeWidth={1.8} aria-hidden />
          Phone
          <span className={cx("size-1.5 rounded-full", w.remote?.published ? "bg-success" : "bg-off")} />
        </button>
      </div>

      {adding && <NewProject onClose={() => setAdding(false)} />}
    </aside>
  );
}

/** A reading older than this is drawn faded: the limits have likely moved since. */
const USAGE_STALE = 3 * 3_600_000;

/**
 * How much of your Claude plan's limits is used (board 1): the five-hour and weekly shares as
 * Claude Code last gave them to a session Gwen started, and a bar for the fuller of the two.
 * Nothing shows until there is a reading.
 */
function ClaudeUsage() {
  const now = useTick(60_000);
  const [usage, setUsage] = useState<wire.ClaudeUsage | null>(null);
  useEffect(() => {
    let live = true;
    const load = () =>
      App.ClaudeUsage().then(
        (u) => live && setUsage(u),
        (e) => live && apiError(e).code !== "unavailable" && setUsage(null),
      );
    load();
    const offs = [onEvent("code_usage", (u: wire.ClaudeUsage) => setUsage(u)), onEvent("connected", load)];
    return () => {
      live = false;
      offs.forEach((off) => off());
    };
  }, []);
  if (!usage?.at || (!usage.five_hour && !usage.seven_day)) return null;
  const five = usage.five_hour;
  const week = usage.seven_day;
  const fullest = Math.max(five?.used_percent ?? 0, week?.used_percent ?? 0);
  const stale = now - usage.at > USAGE_STALE;
  const pct = (w: wire.UsageWindow) => `${Math.round(w.used_percent)}%`;
  const resets = (label: string, w?: wire.UsageWindow | null) => (w?.resets_at ? `${label} resets at ${formatTime(w.resets_at)}` : "");
  const title = [`From a Claude session ${formatAgo(usage.at, now)}`, resets("The 5-hour limit", five), resets("The weekly limit", week)].filter(Boolean).join(". ");
  return (
    <div className={cx("mt-2 border-t border-chrome-line px-2 pt-2.5 pb-0.5", stale && "opacity-60")} title={title}>
      <div className="flex items-center text-[12px] text-ink-muted">
        Claude
        <span className="ml-auto font-mono text-[11.5px] text-ink-subtle">{[five && `5h ${pct(five)}`, week && `week ${pct(week)}`].filter(Boolean).join(" · ")}</span>
      </div>
      <div
        className="mt-1.5 h-1 overflow-hidden rounded-sm bg-surface-3"
        role="meter"
        aria-label="Claude usage, the fuller limit"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(fullest)}
      >
        <div className={cx("h-full rounded-sm", fullest >= 90 ? "bg-danger" : "bg-[#D97757]")} style={{ width: `${fullest}%` }} />
      </div>
    </div>
  );
}

/** The sidebar's Now card: tracking state, timer, project and task, and the two buttons you need most. */
function NowCard() {
  const d = useDaemon();
  const t = useTracking();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useTick(1000);
  const st = d.status;
  if (!st) return <div className="mx-0.5 mt-2 mb-3 h-[92px] rounded-[10px] border border-line bg-surface-1" />;
  const now = d.now();
  const color = stateColor[st.state] ?? stateColor.off;
  const worked = (st.today?.worked_ms ?? 0) + (t.working && st.today ? now - st.server_now_at : 0);
  const project = d.projects.find((p) => p.id === st.project_id);
  const task = d.tasks.find((x) => x.id === st.task_id);
  const label = st.state === "off" ? "Off the clock" : t.onBreak ? "On break" : st.state === "idle_pending" ? "Idle" : "Working";
  // Time with no project is "Unassigned"; a tracked task that simply has no project says so.
  const where = st.state === "off" ? "Start when you're ready" : [project ? projectPath(project, d.projects) : task ? "No project" : "Unassigned", task?.title].filter(Boolean).join(" · ");

  return (
    <div ref={ref} className="relative mx-0.5 mt-2 mb-3">
      <div className="rounded-[10px] border border-line bg-surface-1 px-3 py-[11px]">
        <button type="button" onClick={() => setOpen((v) => !v)} aria-expanded={open} aria-haspopup="dialog" className="block w-full text-left">
          <div className="flex items-center gap-2">
            <span className={cx("size-2 rounded-full", st.state === "working" && "animate-live motion-reduce:animate-none")} style={{ background: color }} />
            <span className="font-medium" style={{ color: st.state === "off" ? "var(--color-ink-muted)" : color }}>
              {label}
            </span>
            {st.state !== "off" && <span className="ml-auto font-mono font-medium tabular-nums">{clockFace(worked)}</span>}
          </div>
          <div className="mt-[3px] truncate text-[12px] text-ink-muted">{where}</div>
        </button>
        <div className="mt-2.5 flex gap-1.5">
          {st.state === "off" ? (
            <Button size="sm" tone="primary" icon={Play} className="flex-1" onClick={() => t.clockIn(d.lastAttribution.project_id, d.lastAttribution.task_id)}>
              Clock in
            </Button>
          ) : t.onBreak ? (
            <Button size="sm" icon={Play} className="flex-1" onClick={t.breakEnd}>
              Back to work
            </Button>
          ) : (
            <Button size="sm" icon={Coffee} iconColor={stateColor.break_manual} className="flex-1" onClick={t.breakStart}>
              Break
            </Button>
          )}
          {st.state !== "off" && <IconButton icon={Square} label="Clock out" tone="secondary" size="sm" onClick={t.clockOut} />}
        </div>
      </div>
      {open && <NowPopover anchor={ref} onClose={() => setOpen(false)} />}
    </div>
  );
}

/** The Now card opened (board 11): the day's ring, streak, what you track, and the rest of the tray's controls. */
function NowPopover({ anchor, onClose }: { anchor: React.RefObject<HTMLDivElement | null>; onClose: () => void }) {
  const d = useDaemon();
  const t = useTracking();
  const [streak, setStreak] = useState(0);
  const [energy, setEnergy] = useState(0);
  const [pick, setPick] = useState(d.lastAttribution);
  const logEnergy = async (level: number) => {
    if (await d.act(() => App.LogEnergy(wire.LogEnergyRequest.createFrom({ level })))) setEnergy(level);
  };
  useTick(1000);
  const today = d.today();

  useEffect(() => {
    App.StatsSummary(addDays(today, -6), today).then(
      (s) => setStreak(s.current_streak),
      () => setStreak(0),
    );
  }, [today, d.daysVersion]);
  useEffect(() => {
    const onDown = (e: MouseEvent) => anchor.current && !anchor.current.contains(e.target as Node) && onClose();
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("mousedown", onDown);
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("mousedown", onDown);
      window.removeEventListener("keydown", onKey);
    };
  }, [anchor, onClose]);

  const st = d.status;
  if (!st) return null;
  const now = d.now();
  const state = st.state;
  const worked = (st.today?.worked_ms ?? 0) + (t.working && st.today ? now - st.server_now_at : 0);
  const breakMs = (st.today?.break_ms ?? 0) + (t.onBreak && st.today ? now - st.server_now_at : 0);
  const target = (st.today?.target_seconds ?? 0) * 1000 || parseDuration(d.config?.tracking.daily_target ?? "8h");
  const met = target > 0 && worked >= target;
  const projectId = state === "off" ? pick.project_id : (st.project_id ?? null);
  const taskId = state === "off" ? pick.task_id : (st.task_id ?? null);
  const projects = projectChoices(d.projects);
  // Open top-level tasks of the chosen project (with its sub-projects'), and always the one being tracked.
  const tasks = d.tasks.filter(
    (x) =>
      x.id === taskId ||
      (x.status === "open" && !x.parent_id && !x.rrule && (!x.occurrence_day || x.occurrence_day <= today) && (!projectId || x.project_id === projectId || d.projects.some((p) => p.id === x.project_id && p.parent_id === projectId))),
  );
  const tracked = d.tasks.find((x) => x.id === taskId);
  const taskGroups = [...projects, { id: "", label: "No project" }]
    .map((p) => ({ ...p, tasks: tasks.filter((x) => (x.project_id ?? "") === p.id) }))
    .filter((p) => p.tasks.length > 0);
  const color = d.projects.find((p) => p.id === projectId)?.color ?? unassignedColor;
  const chooseProject = (id: string | null) => (state === "off" ? setPick({ project_id: id, task_id: null }) : t.switchTo(id, null));
  const chooseTask = (id: string | null) => {
    const project = d.tasks.find((x) => x.id === id)?.project_id ?? projectId;
    if (state === "off") setPick({ project_id: project, task_id: id });
    else t.switchTo(project, id);
  };

  return (
    <div role="dialog" aria-label="Now" className="animate-pop absolute top-full left-0 z-40 mt-2 w-[300px] rounded-xl border border-line-strong bg-surface-1 p-4 shadow-2xl shadow-black/60">
      <div className="flex items-center gap-4">
        <ProgressRing fraction={target > 0 ? worked / target : 0} color={met ? stateColor.working : "var(--color-accent)"} size={64} stroke={6} label="Worked against the daily target">
          <span className="text-[13px] leading-none font-semibold tabular-nums">{Math.round(target > 0 ? (worked / target) * 100 : 0)}%</span>
        </ProgressRing>
        <div className="min-w-0">
          <div className="text-[13px] font-medium" style={{ color: stateColor[state] }}>
            {stateLabel[state]}
          </div>
          <div className="font-mono text-[22px] leading-tight font-semibold tabular-nums">{clockFace(worked)}</div>
          <div className="text-[11.5px] text-ink-subtle tabular-nums">
            of {formatDuration(target)} · break {formatDuration(breakMs)}
            {state !== "off" && ` · since ${formatTime(st.state_since_at)}`}
          </div>
        </div>
      </div>
      <div className="mt-3 flex items-center gap-1.5 text-[12px] text-ink-muted">
        <Flame size={13} aria-hidden className={streak > 0 ? "text-idle" : "text-ink-faint"} />
        {streak} {streak === 1 ? "day" : "days"} in a row
        {st.snoozed_until_at != null && st.snoozed_until_at > now && (
          <span className="ml-auto inline-flex items-center gap-1">
            <BellOff size={12} aria-hidden />
            snoozed {formatDuration(st.snoozed_until_at - now)}
          </span>
        )}
      </div>
      <div className="mt-3 flex items-center gap-1.5 text-[12px] text-ink-muted">
        Energy
        <div role="group" aria-label="How's your energy?" className="ml-auto flex gap-1">
          {energyLevels.map((e, i) => (
            <button
              key={e}
              type="button"
              title={e}
              aria-label={`Energy: ${e}`}
              aria-pressed={energy === i + 1}
              onClick={() => logEnergy(i + 1)}
              className={cx(
                "grid size-6 place-items-center rounded-md border text-[11px] tabular-nums",
                energy === i + 1 ? "border-accent bg-accent-soft text-ink" : "border-line text-ink-muted hover:border-line-strong hover:text-ink",
              )}
            >
              {i + 1}
            </button>
          ))}
        </div>
      </div>
      <div className="mt-3 flex flex-col gap-2">
        <DotSelect color={color} value={projectId ?? ""} onChange={(e) => chooseProject(e.target.value || null)} aria-label="Project">
          <option value="">{tracked && !tracked.project_id ? "No project" : "Unassigned"}</option>
          {projects.map((p) => (
            <option key={p.id} value={p.id}>
              {p.label}
            </option>
          ))}
        </DotSelect>
        <Select value={taskId ?? ""} onChange={(e) => chooseTask(e.target.value || null)} aria-label="Task">
          <option value="">No task</option>
          {taskGroups.map((p) => (
            <optgroup key={p.id} label={p.label}>
              {p.tasks.map((x) => (
                <option key={x.id} value={x.id}>
                  {x.title}
                </option>
              ))}
            </optgroup>
          ))}
        </Select>
      </div>
      <div className="mt-3 flex flex-wrap gap-1.5">
        {state === "off" && (
          <Button size="sm" tone="primary" icon={Play} onClick={() => t.clockIn(pick.project_id, pick.task_id)}>
            Clock in
          </Button>
        )}
        {t.onBreak && (
          <Button size="sm" tone="primary" icon={Play} onClick={t.breakEnd}>
            End break
          </Button>
        )}
        {(state === "working" || state === "break_auto") && (
          <Button size="sm" icon={Coffee} iconColor={stateColor.break_manual} onClick={t.breakStart}>
            {state === "break_auto" ? "Stay on break" : "Break"}
          </Button>
        )}
        {state !== "off" && (
          <Button size="sm" icon={BellOff} onClick={t.snooze}>
            Snooze nudges
          </Button>
        )}
        {state !== "off" && (
          <Button size="sm" icon={Square} iconColor="var(--color-danger)" onClick={t.clockOut}>
            Clock out
          </Button>
        )}
      </div>
    </div>
  );
}

const energyLevels = ["Drained", "Low", "Okay", "Good", "Peak"];

/** New project: a name, a folder on this laptop, and a colour from the palette. */
export function NewProject({ onClose, folder: initialFolder = "" }: { onClose: () => void; folder?: string }) {
  const d = useDaemon();
  const tabs = useTabs();
  const home = useHome();
  const [name, setName] = useState(initialFolder ? (initialFolder.split("/").pop() ?? "") : "");
  const [folder, setFolder] = useState(initialFolder);
  const [busy, setBusy] = useState(false);

  async function browse() {
    const path = await App.PickProjectFolder(folder || home).catch(() => "");
    if (!path) return;
    setFolder(path);
    if (!name.trim()) setName(path.split("/").pop() ?? "");
  }
  async function create(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim() || busy) return;
    setBusy(true);
    const p = await d.act(() => App.CreateProject(wire.CreateProjectRequest.createFrom({ name: name.trim(), folder: folder || undefined })));
    setBusy(false);
    if (!p) return;
    await d.refresh();
    tabs.open({ kind: "project", project: p.id });
    onClose();
  }

  return (
    <Modal
      title="New project"
      description="A project is your work and its workbench: its folder's terminals, files, and history, with its tasks and time."
      onClose={onClose}
      size="sm"
      footer={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button tone="primary" type="submit" form="new-project" busy={busy} disabled={!name.trim()}>
            Create project
          </Button>
        </>
      }
    >
      <form id="new-project" onSubmit={create} className="flex flex-col gap-3">
        <Field label="Name">
          <Input autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="gwen" />
        </Field>
        <Field label="Folder" hint="Where its code lives on this laptop. You can add it later." compound>
          <div className="flex gap-2">
            <div className="flex h-9 min-w-0 flex-1 items-center truncate rounded-lg border border-line bg-canvas px-3 font-mono text-[12.5px] text-ink-muted">
              {folder ? tildify(folder, home) : "No folder"}
            </div>
            <Button icon={FolderOpen} onClick={browse}>
              Choose…
            </Button>
          </div>
        </Field>
      </form>
    </Modal>
  );
}
