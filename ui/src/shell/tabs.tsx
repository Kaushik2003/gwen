import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";

// The Gwen window's tabs (plan.md 3.15). Today is always first and cannot
// close; every other tab is a screen, a project's home, a terminal, a file,
// or a project's history. Open tabs are kept in the webview's storage, which
// is only a convenience: losing them loses nothing.

/** The screens with a place in the sidebar, and Settings. */
export type ScreenId = "inbox" | "gwen" | "plan" | "review" | "settings" | "projects";

/** The views of a project's home (board 7); none is Overview. */
export type ProjectView = "overview" | "board" | "sessions" | "time" | "settings";

export type Tab =
  | { kind: "today" }
  | { kind: "screen"; screen: ScreenId; sub?: string; param?: string | null }
  | { kind: "project"; project: string; view?: ProjectView }
  | { kind: "term"; project: string; terminal: string; split?: string }
  | { kind: "file"; project: string; path: string }
  | { kind: "history"; project: string };

export function tabId(t: Tab): string {
  switch (t.kind) {
    case "today":
      return "today";
    case "screen":
      return `screen:${t.screen}`;
    case "project":
      return `project:${t.project}`;
    case "term":
      return `term:${t.terminal}`;
    case "file":
      return `file:${t.project}:${t.path}`;
    case "history":
      return `history:${t.project}`;
  }
}

/** The project a tab belongs to, for the side panel and for following your tabs with tracking. */
export function tabProject(t: Tab | undefined): string | null {
  if (!t) return null;
  return t.kind === "project" || t.kind === "term" || t.kind === "file" || t.kind === "history" ? t.project : null;
}

/** Sub-screens of each hub screen, in order; the first is the default. */
export const subScreens: Record<ScreenId, { id: string; label: string }[]> = {
  inbox: [
    { id: "needs", label: "Needs you" },
    { id: "captured", label: "To process" },
  ],
  gwen: [
    { id: "chat", label: "Chat" },
    { id: "memory", label: "Memory" },
  ],
  plan: [
    { id: "day", label: "Day" },
    { id: "board", label: "Board" },
    { id: "goals", label: "Goals" },
  ],
  review: [
    { id: "week", label: "Week" },
    { id: "stats", label: "Stats" },
    { id: "history", label: "History" },
  ],
  settings: [],
  projects: [],
};

export const screenLabel: Record<ScreenId, string> = {
  inbox: "Inbox",
  gwen: "Gwen",
  plan: "Plan",
  review: "Review",
  settings: "Settings",
  projects: "Projects",
};

/**
 * The tab an old screen name opens, with what the screen reads as its param. These are the names
 * the tray, the panel widget, `gwen open`, and the screens' own links (go("board", "matrix")) use.
 */
export function screenTab(screen: string, param: string | null = null): Tab | null {
  const s = (sc: ScreenId, sub?: string): Tab => ({ kind: "screen", screen: sc, sub, param });
  switch (screen) {
    case "":
    case "today":
      return { kind: "today" };
    case "inbox":
    case "tasks":
      return s("inbox", "captured");
    case "needs":
    case "work":
      return s("inbox", "needs");
    case "gwen":
    case "assistant":
    case "chat":
      return s("gwen", "chat");
    case "memory":
      return s("gwen", "memory");
    case "plan":
      return s("plan", "day");
    case "board":
      return s("plan", "board");
    case "goals":
      return s("plan", "goals");
    case "review":
    case "weekly":
    case "week":
      return s("review", "week");
    case "stats":
      return s("review", "stats");
    case "history":
      return s("review", "history");
    case "projects":
      return s("projects");
    case "settings":
      return s("settings");
    case "connections":
      return { kind: "screen", screen: "settings", param: "connections" };
  }
  if (screen.startsWith("settings/")) return { kind: "screen", screen: "settings", param: screen.slice("settings/".length) };
  return null;
}

/** What `gwen-ui --open TARGET` asks for, before sessions and terminals are resolved to projects. */
export type Target = { tab: Tab } | { session: string } | { terminal: string } | null;

/**
 * Gwen › Chat on one chat of her agent. The param carries the moment it was asked for, so opening the same chat again
 * (from Needs you, after you moved to another chat) selects it again.
 */
export function agentChatTab(chat: string): Tab {
  return { kind: "screen", screen: "gwen", sub: "chat", param: `agent:${chat}#${Date.now()}` };
}

export function parseTarget(target: string): Target {
  const t = target.trim();
  if (t.startsWith("project:")) return { tab: { kind: "project", project: t.slice(8) } };
  if (t.startsWith("session:")) return { session: t.slice(8) };
  if (t.startsWith("terminal:")) return { terminal: t.slice(9) };
  if (t.startsWith("agent:")) return { tab: agentChatTab(t.slice(6)) };
  const tab = screenTab(t);
  return tab ? { tab } : null;
}

interface TabsState {
  tabs: Tab[];
  active: string;
}

interface Tabs extends TabsState {
  activeTab: Tab;
  /** Opens a tab, or focuses it when it is already open (taking the new sub-screen or param). */
  open: (t: Tab, opts?: { background?: boolean }) => void;
  close: (id: string) => void;
  focus: (id: string) => void;
  /** Changes an open tab in place, e.g. a hub's sub-screen or a terminal's split. */
  update: (id: string, t: Tab) => void;
  /** Focuses the next (1) or previous (-1) tab. */
  cycle: (by: 1 | -1) => void;
}

const TabsContext = createContext<Tabs | null>(null);

/** The window's tabs, or null in a window without them (the hub). */
export function useTabsMaybe(): Tabs | null {
  return useContext(TabsContext);
}

export function useTabs(): Tabs {
  const t = useContext(TabsContext);
  if (!t) throw new Error("useTabs outside TabsProvider");
  return t;
}

const storageKey = "gwen.tabs.v1";
const today: Tab = { kind: "today" };

function load(): TabsState {
  try {
    const raw = localStorage.getItem(storageKey);
    if (raw) {
      const v = JSON.parse(raw) as TabsState;
      if (Array.isArray(v.tabs) && typeof v.active === "string") {
        const tabs = [today, ...v.tabs.filter((t) => t && typeof t.kind === "string" && t.kind !== "today")];
        return { tabs, active: tabs.some((t) => tabId(t) === v.active) ? v.active : "today" };
      }
    }
  } catch {
    // storage is a convenience
  }
  return { tabs: [today], active: "today" };
}

export function TabsProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<TabsState>(load);

  useEffect(() => {
    try {
      localStorage.setItem(storageKey, JSON.stringify(state));
    } catch {
      // storage is a convenience
    }
  }, [state]);

  const open = useCallback((t: Tab, opts?: { background?: boolean }) => {
    const id = tabId(t);
    setState((s) => {
      const at = s.tabs.findIndex((x) => tabId(x) === id);
      const tabs = at >= 0 ? s.tabs.map((x, i) => (i === at ? { ...x, ...t } : x)) : [...s.tabs, t];
      return { tabs, active: opts?.background ? s.active : id };
    });
  }, []);

  const close = useCallback((id: string) => {
    if (id === "today") return;
    setState((s) => {
      const at = s.tabs.findIndex((x) => tabId(x) === id);
      if (at < 0) return s;
      const tabs = s.tabs.filter((_, i) => i !== at);
      const active = s.active === id ? tabId(tabs[Math.min(at, tabs.length - 1)] ?? today) : s.active;
      return { tabs, active };
    });
  }, []);

  const focus = useCallback((id: string) => setState((s) => (s.tabs.some((x) => tabId(x) === id) ? { ...s, active: id } : s)), []);

  const update = useCallback(
    (id: string, t: Tab) => setState((s) => ({ ...s, tabs: s.tabs.map((x) => (tabId(x) === id ? t : x)), active: s.active === id ? tabId(t) : s.active })),
    [],
  );

  const cycle = useCallback(
    (by: 1 | -1) =>
      setState((s) => {
        const at = s.tabs.findIndex((x) => tabId(x) === s.active);
        const next = s.tabs[(at + by + s.tabs.length) % s.tabs.length];
        return { ...s, active: tabId(next) };
      }),
    [],
  );

  const value = useMemo<Tabs>(() => {
    const activeTab = state.tabs.find((t) => tabId(t) === state.active) ?? today;
    return { ...state, activeTab, open, close, focus, update, cycle };
  }, [state, open, close, focus, update, cycle]);

  return <TabsContext.Provider value={value}>{children}</TabsContext.Provider>;
}
