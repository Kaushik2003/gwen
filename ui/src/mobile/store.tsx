import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { ApiError, api, follow, setOnUnpaired, type GwenEvent, type Session, type SessionList, type Spaces, type Terminal } from "./api";

export type SessionTab = "terminal" | "chat" | "changes";
/** The screens: Today (home), Work and Gwen have the tab bar; the rest open over them. */
export interface Route {
  name: "today" | "work" | "gwen" | "new" | "session" | "terminal" | "pair";
  /** A session's id, a terminal's address, or Gwen's chat. */
  id?: string;
  tab?: SessionTab;
}

/** The screens the tab bar switches between. */
export const tabScreens = ["today", "work", "gwen"] as const;

function parseRoute(): Route {
  const parts = location.pathname.split("/").filter(Boolean);
  switch (parts[0]) {
    case "pair":
      return { name: "pair" };
    case "work":
    case "spaces":
    case "inbox":
      return { name: "work" };
    case "gwen":
      return { name: "gwen", id: parts[1] ? decodeURIComponent(parts[1]) : undefined };
    case "new":
      return { name: "new" };
    case "s":
      if (parts[1]) return { name: "session", id: decodeURIComponent(parts[1]), tab: (parts[2] as SessionTab) || undefined };
      break;
    case "t":
      if (parts[1]) return { name: "terminal", id: decodeURIComponent(parts[1]) };
  }
  return { name: "today" };
}

function pathOf(r: Route): string {
  switch (r.name) {
    case "session":
      return `/s/${encodeURIComponent(r.id!)}${r.tab ? "/" + r.tab : ""}`;
    case "terminal":
      return `/t/${encodeURIComponent(r.id!)}`;
    case "gwen":
      return r.id ? `/gwen/${encodeURIComponent(r.id)}` : "/gwen";
    case "today":
      return "/";
  }
  return "/" + r.name;
}

interface Store {
  route: Route;
  go: (r: Route, replace?: boolean) => void;
  back: () => void;
  sessions: Session[];
  meta: Pick<SessionList, "core" | "message" | "machine">;
  spaces: Spaces | null;
  /** Every terminal of Gwen Terminal by address. */
  terminals: Record<string, Terminal>;
  /** null until the first answer; false when the laptop can't be reached. */
  online: boolean | null;
  /** When the lists were last heard from the laptop. */
  heardAt: number;
  refresh: () => void;
  upsert: (s: Session) => void;
  toast: (text: string) => void;
  /** The short note on screen, "" for none. */
  toastText: string;
  /** When the phone saw a session reach the state it is in, for "Working · 4m"; undefined when it doesn't know. */
  since: (s: Session) => number | undefined;
  /** The newest session that asked for you or finished while the app was open. */
  alert: Alert | null;
  dismissAlert: () => void;
}

const Ctx = createContext<Store | null>(null);
export const useStore = () => useContext(Ctx)!;

// Gwen's own events from the one event stream, for the screens that show your day and her chats.
const gwenListeners = new Set<(name: GwenEvent, data: any) => void>();
/** Calls fn with each of Gwen's events (state_changed, tasks_changed, agent_step, …); returns a function that stops. */
export function onGwen(fn: (name: GwenEvent, data: any) => void): () => void {
  gwenListeners.add(fn);
  return () => gwenListeners.delete(fn);
}

const KEPT = "gwen.last";
const TAB = "gwen.tab";

/** The session tab used last: the phone opens the next session on it too (plan.md 3.14.13). */
export function lastTab(): SessionTab {
  try {
    const t = localStorage.getItem(TAB);
    if (t === "terminal" || t === "chat" || t === "changes") return t;
  } catch {
    // storage is off
  }
  // The approved design opens a session on its Chat: what Claude did, in plain steps, and its questions.
  return "chat";
}
export function keepTab(t: SessionTab) {
  try {
    localStorage.setItem(TAB, t);
  } catch {
    // storage is off: the tab is simply not kept
  }
}

/** A session that just asked for you or finished, shown as a banner while the app is open. */
export interface Alert { id: string; name: string; text: string; urgent: boolean }

/** What the banner says when a session reaches a state; states missing here make no banner. */
function alertOf(s: Session): Alert | null {
  const a = (text: string, urgent: boolean): Alert => ({ id: s.id, name: s.name, text, urgent });
  switch (s.state) {
    case "needs_you":
      return a(s.need === "permission" ? "needs your permission" : s.need === "question" ? "has a question" : "needs you", true);
    case "error":
      return a("stopped with an error", true);
    case "done":
      return a("finished", false);
  }
  return null;
}

const SENT = "gwen.sent";
interface Sent { text: string; at: number }

/** Remembers a message this phone sent, so the chat can say "From your phone" under it. */
export function noteSent(id: string, text: string) {
  try {
    const all: Record<string, Sent[]> = JSON.parse(localStorage.getItem(SENT) ?? "{}");
    all[id] = [...(all[id] ?? []), { text: text.trim(), at: Date.now() }].slice(-30);
    // Only the sessions written to lately are kept.
    const keep = Object.entries(all).sort((a, b) => b[1][b[1].length - 1].at - a[1][a[1].length - 1].at).slice(0, 40);
    localStorage.setItem(SENT, JSON.stringify(Object.fromEntries(keep)));
  } catch {
    // storage is off: the chat just doesn't say where a message came from
  }
}

/** Whether a message of the conversation is one this phone sent: the same words, written down within ten minutes of sending. */
export function sentHere(id: string, text: string, at: number): boolean {
  try {
    const all: Record<string, Sent[]> = JSON.parse(localStorage.getItem(SENT) ?? "{}");
    const t = text.trim();
    return (all[id] ?? []).some((m) => m.text === t && at >= m.at - 60_000 && at - m.at < 600_000);
  } catch {
    return false;
  }
}

export function Provider({ children }: { children: ReactNode }) {
  const [route, setRoute] = useState<Route>(parseRoute);
  const [byId, setById] = useState<Record<string, Session>>({});
  const [meta, setMeta] = useState<Store["meta"]>({ core: "ok", message: "", machine: "" });
  const [spaces, setSpaces] = useState<Spaces | null>(null);
  const [online, setOnline] = useState<boolean | null>(null);
  const [heardAt, setHeardAt] = useState(0);
  const [toastText, setToastText] = useState("");
  const toastTimer = useRef(0);
  const depth = useRef(0);
  const [alert, setAlert] = useState<Alert | null>(null);
  const alertTimer = useRef(0);
  // The state each session was last seen in, so a banner shows only when one changes.
  const states = useRef<Record<string, string>>({});
  // When each session reached its state, as far as the phone saw it.
  const sinceAt = useRef<Record<string, number>>({});
  const routeNow = useRef(route);
  routeNow.current = route;

  const go = useCallback((r: Route, replace = false) => {
    if (replace) history.replaceState({ d: depth.current }, "", pathOf(r));
    else history.pushState({ d: ++depth.current }, "", pathOf(r));
    setRoute(r);
  }, []);
  const back = useCallback(() => {
    // Opened straight from a push link there is nothing to go back to: go home instead.
    if (depth.current > 0) history.back();
    else go({ name: "today" }, true);
  }, [go]);
  useEffect(() => {
    const pop = (e: PopStateEvent) => {
      depth.current = e.state?.d ?? 0;
      setRoute(parseRoute());
    };
    window.addEventListener("popstate", pop);
    return () => window.removeEventListener("popstate", pop);
  }, []);

  const dismissAlert = useCallback(() => {
    window.clearTimeout(alertTimer.current);
    setAlert(null);
  }, []);
  /** Notes a session's state. `live` is a change that just came in on the event stream; one found by asking again may be older. */
  const noticeChange = useCallback((s: Session, live = false) => {
    const before = states.current[s.id];
    states.current[s.id] = s.state;
    if (before === undefined || before === s.state) return;
    sinceAt.current[s.id] = live ? Date.now() : s.updated_at;
    const a = alertOf(s);
    const r = routeNow.current;
    // The session on screen says it itself.
    if (!a || (r.name === "session" && r.id === s.id)) return;
    setAlert(a);
    window.clearTimeout(alertTimer.current);
    alertTimer.current = window.setTimeout(() => setAlert(null), a.urgent ? 12_000 : 6_000);
  }, []);

  const toast = useCallback((text: string) => {
    setToastText(text);
    window.clearTimeout(toastTimer.current);
    toastTimer.current = window.setTimeout(() => setToastText(""), 3200);
  }, []);

  const loading = useRef(false);
  const again = useRef(false);
  const refresh = useCallback(async () => {
    if (loading.current) {
      again.current = true;
      return;
    }
    loading.current = true;
    try {
      const [list, sp] = await Promise.all([api.sessions(), api.spaces()]);
      for (const s of list.sessions) noticeChange(s);
      setById(Object.fromEntries(list.sessions.map((s) => [s.id, s])));
      setMeta({ core: list.core, message: list.message, machine: list.machine });
      setSpaces(sp);
      setOnline(true);
      setHeardAt(Date.now());
      try {
        localStorage.setItem(KEPT, JSON.stringify({ at: Date.now(), list, spaces: sp, since: sinceAt.current }));
      } catch {
        // storage is full or off: the list is simply not kept
      }
    } catch (e) {
      if (e instanceof ApiError && e.code === "offline") setOnline(false);
    } finally {
      loading.current = false;
      if (again.current) {
        again.current = false;
        refresh();
      }
    }
  }, [noticeChange]);

  useEffect(() => {
    setOnUnpaired(() => setRoute((r) => (r.name === "pair" ? r : (history.replaceState({}, "", "/pair"), { name: "pair" }))));
    // What the laptop last said shows at once, and stays on screen when it can't be reached.
    try {
      const kept = JSON.parse(localStorage.getItem(KEPT) ?? "null");
      if (kept) {
        setById(Object.fromEntries(kept.list.sessions.map((s: Session) => [s.id, s])));
        setMeta({ core: kept.list.core, message: kept.list.message, machine: kept.list.machine });
        setSpaces(kept.spaces);
        setHeardAt(kept.at);
        for (const s of kept.list.sessions as Session[]) states.current[s.id] = s.state;
        sinceAt.current = kept.since ?? {};
      }
    } catch {
      // nothing kept
    }
    if (parseRoute().name === "pair") return;
    let spacesTimer = 0;
    const stop = follow({
      session: (s) => {
        noticeChange(s, true);
        setById((m) => ({ ...m, [s.id]: s }));
        setHeardAt(Date.now());
      },
      removed: (id) => setById(({ [id]: _, ...rest }) => rest),
      spaces: () => {
        window.clearTimeout(spacesTimer);
        spacesTimer = window.setTimeout(refresh, 250);
      },
      online: (up) => {
        setOnline(up);
        if (up) {
          refresh();
          // Reconnected: what changed meanwhile is fetched again.
          for (const fn of gwenListeners) fn("state_changed", null);
        }
      },
      gwen: (name, data) => {
        // A project made or given a folder is a new space: shells and Claude can start there.
        if (name === "projects_changed") {
          window.clearTimeout(spacesTimer);
          spacesTimer = window.setTimeout(refresh, 250);
        }
        for (const fn of gwenListeners) fn(name, data);
      },
    });
    return () => {
      stop();
      window.clearTimeout(spacesTimer);
    };
  }, [refresh, noticeChange]);

  const upsert = useCallback((s: Session) => {
    noticeChange(s, true);
    setById((m) => ({ ...m, [s.id]: s }));
  }, [noticeChange]);
  const since = useCallback((s: Session) => (states.current[s.id] === s.state ? sinceAt.current[s.id] : undefined), []);
  const sessions = useMemo(() => Object.values(byId).sort((a, b) => b.updated_at - a.updated_at), [byId]);
  const terminals = useMemo(() => {
    const out: Record<string, Terminal> = {};
    for (const sp of spaces?.spaces ?? []) for (const tab of sp.tabs) for (const t of tab.terminals) out[t.terminal] = t;
    for (const t of spaces?.loose ?? []) out[t.terminal] = t;
    for (const s of sessions) for (const h of s.helpers) out[h.terminal] = h;
    return out;
  }, [spaces, sessions]);

  const value = useMemo<Store>(
    () => ({ route, go, back, sessions, meta, spaces, terminals, online, heardAt, refresh, upsert, toast, toastText, since, alert, dismissAlert }),
    [route, go, back, sessions, meta, spaces, terminals, online, heardAt, refresh, upsert, toast, toastText, since, alert, dismissAlert],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}
