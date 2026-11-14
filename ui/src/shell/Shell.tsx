import { Bell, CalendarDays, ChartColumn, Columns2, FileCode, FolderKanban, GitCommitHorizontal, Inbox, PanelRight, Plus, SlidersHorizontal, Smartphone, Sparkles, Sun, X, type LucideIcon } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { App, onEvent } from "../api";
import { BriefingProvider } from "../components/Briefing";
import { TaskUIProvider } from "../task/TaskUI";
import { cx, stateColor } from "../components/ui";
import { useDaemon, useTick } from "../daemon";
import { clockFace, formatTime } from "../format";
import { NavContext } from "../nav";
import { useTracking } from "../tracking";
import { Kbd, Scroll, StateGlyph, Swatch, TermIcon } from "./bits";
import { useNext } from "./next";
import Palette from "./Palette";
import { HubScreen } from "./screens";
import SidePanel, { PanelProvider, usePanel } from "./SidePanel";
import Sidebar from "./Sidebar";
import { parseTarget, screenLabel, screenTab, tabId, tabProject, useTabs, type ScreenId, type Tab } from "./tabs";
import TodayBoard from "./TodayBoard";
import { useWork, WorkProvider } from "./work";
import ProjectHome from "./ProjectHome";
import TermTab from "./TermTab";
import FileTab from "./FileTab";
import HistoryTab from "./HistoryTab";

// The Gwen window (plan.md 3.15, board 1): sidebar, tabs, an optional side
// panel, and a status bar on every screen.

export default function Shell() {
  return (
    <WorkProvider>
      <PanelProvider>
        <BriefingProvider>
          <TaskUIProvider>
            <Frame />
          </TaskUIProvider>
        </BriefingProvider>
      </PanelProvider>
    </WorkProvider>
  );
}

const screenIcon: Record<ScreenId, LucideIcon> = {
  inbox: Inbox,
  gwen: Sparkles,
  plan: CalendarDays,
  review: ChartColumn,
  settings: SlidersHorizontal,
  projects: FolderKanban,
};

/** Whether a key event comes from inside a terminal, where Ctrl W, Ctrl K, and Ctrl \ belong to the program. */
function inTerminal(e: KeyboardEvent): boolean {
  return e.target instanceof Element && !!e.target.closest(".xterm");
}

/**
 * Tells gwend while this window has your focus (again every 20 s, since gwend forgets it after 45), so nudges then
 * show under Needs you here alone, without the panel card or a desktop notification on top.
 */
function useFocusReport() {
  useEffect(() => {
    let timer: number | undefined;
    const tell = (focused: boolean) => {
      try {
        App.WindowFocus(focused).catch(() => {});
      } catch {
        // A window without the binding (an older gwen-ui) just keeps the old behaviour.
      }
    };
    const check = () => {
      const focused = document.hasFocus() && document.visibilityState === "visible";
      tell(focused);
      window.clearInterval(timer);
      timer = focused ? window.setInterval(() => tell(true), 20_000) : undefined;
    };
    check();
    window.addEventListener("focus", check);
    window.addEventListener("blur", check);
    document.addEventListener("visibilitychange", check);
    return () => {
      window.clearInterval(timer);
      window.removeEventListener("focus", check);
      window.removeEventListener("blur", check);
      document.removeEventListener("visibilitychange", check);
      tell(false);
    };
  }, []);
}

function Frame() {
  const w = useWork();
  const tabs = useTabs();
  const panel = usePanel();
  const [palette, setPalette] = useState(false);

  // A target from `gwen-ui --open`, at launch or handed to the running window.
  const openTarget = useCallback(
    (target: string) => {
      const t = parseTarget(target);
      if (!t) return;
      if ("tab" in t) return tabs.open(t.tab);
      const address = "session" in t ? (w.sessionById(t.session)?.terminal ?? "") : t.terminal;
      if (!address) {
        if ("session" in t) w.refreshSessions();
        return tabs.open({ kind: "screen", screen: "inbox", sub: "needs" });
      }
      const term = w.terminals.find((x) => x.address === address);
      tabs.open({ kind: "term", project: term?.project ?? "", terminal: address });
    },
    [tabs, w],
  );
  const opener = useRef(openTarget);
  opener.current = openTarget;
  useEffect(() => {
    App.StartScreen().then((s) => s && opener.current(s), () => {});
    return onEvent("navigate", (target: string) => opener.current(String(target ?? "")));
  }, []);

  // The screens' own links, go("board", "matrix"), open tabs.
  const go = useCallback((screen: string, param: string | null = null) => {
    const t = screenTab(screen, param);
    if (t) tabs.open(t);
  }, [tabs]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const ctrl = e.ctrlKey || e.metaKey;
      if (!ctrl || e.altKey) return;
      const term = inTerminal(e);
      const key = e.key.toLowerCase();
      // Handled keys stop here, before a terminal sees them: this listener runs in the capture phase.
      const take = () => (e.preventDefault(), e.stopPropagation());
      if ((key === "k" && !e.shiftKey && !term) || (key === "p" && e.shiftKey)) {
        take();
        setPalette((v) => !v);
      } else if ((key === "w" && !e.shiftKey && !term) || (key === "w" && e.shiftKey)) {
        take();
        tabs.close(tabs.active);
      } else if (e.key === "Tab") {
        take();
        tabs.cycle(e.shiftKey ? -1 : 1);
      } else if (/^[1-9]$/.test(e.key) && !e.shiftKey) {
        const t = tabs.tabs[Number(e.key) - 1];
        if (t) {
          take();
          tabs.focus(tabId(t));
        }
      } else if (e.key === "\\" && !term) {
        take();
        panel.toggle();
      }
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [tabs, panel]);

  useFollowTabs();
  useFocusReport();

  const nav = useMemo(() => {
    const a = tabs.activeTab;
    const screen = a.kind === "screen" ? (a.sub ?? a.screen) : a.kind;
    return { screen, param: a.kind === "screen" ? (a.param ?? null) : null, go };
  }, [tabs.activeTab, go]);

  return (
    <NavContext.Provider value={nav}>
      <div className="flex h-full flex-col bg-canvas text-[13px]">
        <div className="flex min-h-0 flex-1">
          <Sidebar onSearch={() => setPalette(true)} />
          <main className="flex min-w-0 flex-1 flex-col">
            <TabBar />
            <div className="relative min-h-0 flex-1">
              {tabs.tabs.map((t) => {
                const id = tabId(t);
                const on = id === tabs.active;
                // Terminals stay mounted while their tab is open, so their screens and scrollback survive a tab switch.
                if (!on && t.kind !== "term") return null;
                return (
                  <div key={id} className={cx("absolute inset-0", !on && "invisible")} aria-hidden={!on}>
                    <TabBody tab={t} active={on} />
                  </div>
                );
              })}
            </div>
          </main>
          {panel.open && <SidePanel />}
        </div>
        <StatusBar onPalette={() => setPalette(true)} />
      </div>
      {palette && <Palette onClose={() => setPalette(false)} />}
    </NavContext.Provider>
  );
}

function TabBody({ tab, active }: { tab: Tab; active: boolean }) {
  switch (tab.kind) {
    case "today":
      return <Scroll wide>{<TodayBoard />}</Scroll>;
    case "screen":
      return <HubScreen tab={tab} />;
    case "project":
      return <ProjectHome project={tab.project} view={tab.view} />;
    case "term":
      return <TermTab tab={tab} active={active} />;
    case "file":
      return <FileTab tab={tab} />;
    case "history":
      return <HistoryTab project={tab.project} />;
  }
}

function TabBar() {
  const d = useDaemon();
  const w = useWork();
  const tabs = useTabs();
  const panel = usePanel();
  const active = tabs.activeTab;

  function label(t: Tab): { icon: ReactNode; text: string; title?: string; glyph?: ReactNode } {
    const project = (id: string) => d.projects.find((p) => p.id === id);
    switch (t.kind) {
      case "today":
        return { icon: <Sun size={14} strokeWidth={1.9} aria-hidden />, text: "Today" };
      case "screen": {
        const Icon = screenIcon[t.screen];
        return { icon: <Icon size={14} strokeWidth={1.9} aria-hidden />, text: screenLabel[t.screen] };
      }
      case "project": {
        const p = project(t.project);
        return { icon: <Swatch color={p?.color ?? "#62666d"} />, text: p?.name ?? "Project" };
      }
      case "term": {
        const term = w.terminals.find((x) => x.address === t.terminal);
        const p = project(t.project);
        const name = term?.name ?? t.terminal;
        return {
          icon: <TermIcon claude={term?.claude ?? false} />,
          text: p ? `${p.name} · ${name}` : name,
          glyph: term ? <StateGlyph state={term.session?.state ?? term.status} unread={term.session?.unread} shell={!term.claude} /> : null,
        };
      }
      case "file":
        return { icon: <FileCode size={14} strokeWidth={1.9} aria-hidden className="text-ink-subtle" />, text: t.path.split("/").pop() ?? t.path, title: t.path };
      case "history":
        return { icon: <GitCommitHorizontal size={14} strokeWidth={1.9} aria-hidden />, text: `History · ${project(t.project)?.name ?? ""}` };
    }
  }

  return (
    <nav aria-label="Open tabs" className="flex h-11 shrink-0 items-center gap-1 border-b border-chrome-line bg-chrome-deep px-2.5">
      <div className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto [scrollbar-width:none]">
        {tabs.tabs.map((t) => {
          const id = tabId(t);
          const on = id === tabs.active;
          const l = label(t);
          return (
            <div
              key={id}
              className={cx(
                "group flex h-[30px] max-w-60 shrink-0 items-center rounded-[7px]",
                on ? "bg-surface-2 text-ink shadow-[inset_0_0_0_1px_var(--color-line)]" : "text-ink-muted hover:bg-surface-1 hover:text-ink",
              )}
            >
              <button
                type="button"
                aria-current={on ? "page" : undefined}
                title={l.title ?? l.text}
                onClick={() => tabs.focus(id)}
                onAuxClick={(e) => e.button === 1 && tabs.close(id)}
                className={cx("flex h-full min-w-0 items-center gap-2 pl-3", t.kind === "today" ? "pr-3" : "pr-1", on && "font-medium")}
              >
                {l.icon}
                <span className="truncate">{l.text}</span>
                {l.glyph}
              </button>
              {t.kind !== "today" && (
                <button
                  type="button"
                  aria-label={`Close ${l.text}`}
                  onClick={() => tabs.close(id)}
                  className={cx("mr-1 grid size-5 place-items-center rounded text-ink-subtle hover:bg-surface-3 hover:text-ink", !on && "opacity-0 group-hover:opacity-100 focus:opacity-100")}
                >
                  <X size={12} aria-hidden />
                </button>
              )}
            </div>
          );
        })}
        <button
          type="button"
          aria-label="New tab"
          title="Open anything (Ctrl K)"
          onClick={() => window.dispatchEvent(new KeyboardEvent("keydown", { key: "k", ctrlKey: true }))}
          className="grid size-7 shrink-0 place-items-center rounded-[7px] text-ink-subtle hover:bg-surface-1 hover:text-ink"
        >
          <Plus size={15} aria-hidden />
        </button>
      </div>
      {active.kind === "term" && (
        <button
          type="button"
          aria-label={active.split ? "Close the split" : "Split this tab"}
          title={active.split ? "Close the split" : "Split: a shell beside this terminal"}
          onClick={() => window.dispatchEvent(new CustomEvent("gwen:split", { detail: tabId(active) }))}
          className={cx("grid size-[30px] place-items-center rounded-[7px] hover:bg-surface-1 hover:text-ink", active.split ? "text-ink" : "text-ink-subtle")}
        >
          <Columns2 size={16} strokeWidth={1.8} aria-hidden />
        </button>
      )}
      <button
        type="button"
        aria-label={panel.open ? "Hide the side panel (Ctrl \\)" : "Show the side panel (Ctrl \\)"}
        title={panel.open ? "Hide the side panel (Ctrl \\)" : "Show the side panel (Ctrl \\)"}
        aria-pressed={panel.open}
        onClick={panel.toggle}
        className={cx("grid size-[30px] place-items-center rounded-[7px] hover:bg-surface-1 hover:text-ink", panel.open ? "text-ink" : "text-ink-subtle")}
      >
        <PanelRight size={16} strokeWidth={1.8} aria-hidden />
      </button>
    </nav>
  );
}

function StatusBar({ onPalette }: { onPalette: () => void }) {
  const d = useDaemon();
  const w = useWork();
  const t = useTracking();
  const tabs = useTabs();
  const next = useNext();
  useTick(1000);
  const st = d.status;
  const now = d.now();
  const worked = st ? (st.today?.worked_ms ?? 0) + (t.working && st.today ? now - st.server_now_at : 0) : 0;
  const project = d.projects.find((p) => p.id === st?.project_id);
  const label = !st || st.state === "off" ? "Off the clock" : t.onBreak ? "On break" : st.state === "idle_pending" ? "Idle" : "Working";
  const phone = w.remote?.published && (w.remote.devices ?? 0) > 0;
  const mins = next ? Math.round((next.start - now) / 60_000) : 0;

  return (
    <footer className="flex h-[30px] shrink-0 items-center gap-[18px] overflow-hidden border-t border-chrome-line bg-chrome-deep px-3.5 text-[12px] whitespace-nowrap text-ink-muted">
      <span className="inline-flex items-center gap-1.5">
        <span className="size-[7px] rounded-full" style={{ background: stateColor[st?.state ?? "off"] }} />
        {label}
        {st && st.state !== "off" && (
          <>
            <span className="font-mono text-ink tabular-nums">{clockFace(worked)}</span>· {project?.name ?? "Unassigned"}
          </>
        )}
      </span>
      {next && (
        <span className="inline-flex min-w-0 items-center gap-1.5">
          <Bell size={13} aria-hidden />
          <span className="max-w-64 truncate">{next.title}</span>
          {mins <= 90 ? `in ${Math.max(mins, 0)} min` : `at ${formatTime(next.start)}`}
        </span>
      )}
      {w.needs.length > 0 && (
        <button type="button" onClick={() => tabs.open({ kind: "screen", screen: "inbox", sub: "needs" })} className="inline-flex items-center gap-1.5 text-warning hover:underline">
          <span className="size-[7px] rounded-full bg-warning" />
          {w.needs.length} need{w.needs.length === 1 ? "s" : ""} you
        </button>
      )}
      {w.core === "down" && <span className="text-ink-subtle">Terminals are off: {w.coreMessage || "Gwen's terminal server isn't running"}</span>}
      <span className="ml-auto inline-flex items-center gap-1.5">
        <Smartphone size={13} aria-hidden />
        {phone ? "Phone connected" : w.remote?.published ? "Phone ready" : "Phone off"}
      </span>
      <button type="button" onClick={onPalette} className="inline-flex items-center gap-1.5 hover:text-ink">
        <Kbd>Ctrl K</Kbd>
        Everything
      </button>
    </footer>
  );
}

/**
 * Your time follows your tabs (plan.md 3.15): while tracking is on and the window has focus, a project's tab that stays in front for
 * 30 seconds switches tracking to that project, with the terminal's linked task when it has one.
 */
function useFollowTabs() {
  const d = useDaemon();
  const t = useTracking();
  const w = useWork();
  const tabs = useTabs();
  const [focused, setFocused] = useState(() => document.hasFocus());
  useEffect(() => {
    const on = () => setFocused(true);
    const off = () => setFocused(false);
    window.addEventListener("focus", on);
    window.addEventListener("blur", off);
    return () => {
      window.removeEventListener("focus", on);
      window.removeEventListener("blur", off);
    };
  }, []);

  const follow = d.config?.tracking.follow_tabs !== false;
  const project = tabProject(tabs.activeTab);
  const active = tabs.activeTab;
  const task = active.kind === "term" ? (w.terminals.find((x) => x.address === active.terminal)?.task ?? "") : "";
  const current = d.status?.project_id ?? null;
  const currentTask = d.status?.task_id ?? null;
  const want = !!project && follow && focused && t.working && (project !== current || (!!task && task !== currentTask));

  const switchTo = useRef(t.switchTo);
  switchTo.current = t.switchTo;
  useEffect(() => {
    if (!want || !project) return;
    const timer = window.setTimeout(() => switchTo.current(project, task || (project === current ? currentTask : null)), 30_000);
    return () => window.clearTimeout(timer);
  }, [want, project, task, current, currentTask]);
}
