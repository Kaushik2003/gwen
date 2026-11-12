import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { ApiError, api, follow, setOnUnpaired, type Session, type SessionList, type Spaces, type Terminal, type Theme } from "./api";

export type SessionTab = "terminal" | "chat" | "changes";
export interface Route {
  name: "inbox" | "spaces" | "new" | "session" | "terminal" | "pair";
  id?: string;
  tab?: SessionTab;
}

function parseRoute(): Route {
  const parts = location.pathname.split("/").filter(Boolean);
  switch (parts[0]) {
    case "pair":
      return { name: "pair" };
    case "spaces":
      return { name: "spaces" };
    case "new":
      return { name: "new" };
    case "s":
      if (parts[1]) return { name: "session", id: decodeURIComponent(parts[1]), tab: (parts[2] as SessionTab) || undefined };
      break;
    case "t":
      if (parts[1]) return { name: "terminal", id: decodeURIComponent(parts[1]) };
  }
  return { name: "inbox" };
}

function pathOf(r: Route): string {
  switch (r.name) {
    case "session":
      return `/s/${encodeURIComponent(r.id!)}${r.tab ? "/" + r.tab : ""}`;
    case "terminal":
      return `/t/${encodeURIComponent(r.id!)}`;
    case "inbox":
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
}

const Ctx = createContext<Store | null>(null);
export const useStore = () => useContext(Ctx)!;

const KEPT = "gwen.last";

/** Sets the app's colours from Gwen Terminal's theme, so the phone looks like the laptop. */
export function applyTheme(t: Theme) {
  const s = document.documentElement.style;
  const vars: Record<string, string> = {
    "--bg": t.background, "--fg": t.foreground, "--muted": t.muted, "--surface": t.surface, "--border": t.border, "--accent": t.accent,
    "--on-accent": t.background, "--surface-2": `color-mix(in srgb, ${t.foreground} 9%, ${t.background})`,
    "--working": t.ansi[3], "--blocked": t.ansi[1], "--done": t.ansi[4], "--ok": t.ansi[2], "--idle": t.muted,
  };
  for (const [k, v] of Object.entries(vars)) s.setProperty(k, v);
  document.documentElement.toggleAttribute("data-light", !t.dark);
  document.querySelector('meta[name="theme-color"]')?.setAttribute("content", t.background);
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

  const go = useCallback((r: Route, replace = false) => {
    if (replace) history.replaceState({ d: depth.current }, "", pathOf(r));
    else history.pushState({ d: ++depth.current }, "", pathOf(r));
    setRoute(r);
  }, []);
  const back = useCallback(() => {
    // Opened straight from a push link there is nothing to go back to: go home instead.
    if (depth.current > 0) history.back();
    else go({ name: "inbox" }, true);
  }, [go]);
  useEffect(() => {
    const pop = (e: PopStateEvent) => {
      depth.current = e.state?.d ?? 0;
      setRoute(parseRoute());
    };
    window.addEventListener("popstate", pop);
    return () => window.removeEventListener("popstate", pop);
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
      setById(Object.fromEntries(list.sessions.map((s) => [s.id, s])));
      setMeta({ core: list.core, message: list.message, machine: list.machine });
      setSpaces(sp);
      applyTheme(sp.theme);
      setOnline(true);
      setHeardAt(Date.now());
      try {
        localStorage.setItem(KEPT, JSON.stringify({ at: Date.now(), list, spaces: sp }));
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
  }, []);

  useEffect(() => {
    setOnUnpaired(() => setRoute((r) => (r.name === "pair" ? r : (history.replaceState({}, "", "/pair"), { name: "pair" }))));
    // What the laptop last said shows at once, and stays on screen when it can't be reached.
    try {
      const kept = JSON.parse(localStorage.getItem(KEPT) ?? "null");
      if (kept) {
        setById(Object.fromEntries(kept.list.sessions.map((s: Session) => [s.id, s])));
        setMeta({ core: kept.list.core, message: kept.list.message, machine: kept.list.machine });
        setSpaces(kept.spaces);
        applyTheme(kept.spaces.theme);
        setHeardAt(kept.at);
      }
    } catch {
      // nothing kept
    }
    if (parseRoute().name === "pair") return;
    let spacesTimer = 0;
    const stop = follow({
      session: (s) => {
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
        if (up) refresh();
      },
    });
    return () => {
      stop();
      window.clearTimeout(spacesTimer);
    };
  }, [refresh]);

  const upsert = useCallback((s: Session) => setById((m) => ({ ...m, [s.id]: s })), []);
  const sessions = useMemo(() => Object.values(byId).sort((a, b) => b.updated_at - a.updated_at), [byId]);
  const terminals = useMemo(() => {
    const out: Record<string, Terminal> = {};
    for (const sp of spaces?.spaces ?? []) for (const tab of sp.tabs) for (const t of tab.terminals) out[t.terminal] = t;
    for (const t of spaces?.loose ?? []) out[t.terminal] = t;
    for (const s of sessions) for (const h of s.helpers) out[h.terminal] = h;
    return out;
  }, [spaces, sessions]);

  const value = useMemo<Store>(
    () => ({ route, go, back, sessions, meta, spaces, terminals, online, heardAt, refresh, upsert, toast }),
    [route, go, back, sessions, meta, spaces, terminals, online, heardAt, refresh, upsert, toast],
  );
  return (
    <Ctx.Provider value={value}>
      {children}
      {toastText && <div className="toast" role="status">{toastText}</div>}
    </Ctx.Provider>
  );
}
