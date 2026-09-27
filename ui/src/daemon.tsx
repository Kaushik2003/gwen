import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { App, apiError, isStale, onEvent, wire, type ApiError } from "./api";
import { dayOf } from "./format";

/** What every screen shares: the daemon's status and lists, kept fresh by the event stream. */
export interface Daemon {
  up: boolean | null; // null until the first health check
  status: wire.Status | null;
  config: wire.Config | null;
  projects: wire.Project[]; // live, not archived
  tasks: wire.Task[]; // open
  /** Bumped on day_changed, so screens showing days refetch. */
  daysVersion: number;
  tasksVersion: number;
  projectsVersion: number;
  notice: string | null;
  setNotice: (s: string | null) => void;
  /** The daemon's clock now: the local clock minus the skew at the last status. */
  now: () => number;
  today: () => string;
  refresh: () => Promise<void>;
  /** Runs an API call, adopting a returned Status and reporting failures. */
  act: <T>(call: () => Promise<T>) => Promise<T | undefined>;
  fail: (e: unknown) => ApiError;
}

const DaemonContext = createContext<Daemon | null>(null);

export function useDaemon(): Daemon {
  const d = useContext(DaemonContext);
  if (!d) throw new Error("useDaemon outside DaemonProvider");
  return d;
}

/** Re-renders every interval ms, for ticking timers. */
export function useTick(ms = 1000): number {
  const [t, setT] = useState(Date.now());
  useEffect(() => {
    const id = setInterval(() => setT(Date.now()), ms);
    return () => clearInterval(id);
  }, [ms]);
  return t;
}

export function DaemonProvider({ children }: { children: ReactNode }) {
  const [up, setUp] = useState<boolean | null>(null);
  const [status, setStatusState] = useState<wire.Status | null>(null);
  const [config, setConfig] = useState<wire.Config | null>(null);
  const [projects, setProjects] = useState<wire.Project[]>([]);
  const [tasks, setTasks] = useState<wire.Task[]>([]);
  const [daysVersion, setDaysVersion] = useState(0);
  const [tasksVersion, setTasksVersion] = useState(0);
  const [projectsVersion, setProjectsVersion] = useState(0);
  const [notice, setNotice] = useState<string | null>(null);
  const skew = useRef(0);
  const configRef = useRef<wire.Config | null>(null);

  const setStatus = useCallback((s: wire.Status) => {
    skew.current = Date.now() - s.server_now_at;
    setStatusState(s);
    setUp(true);
  }, []);

  const loadProjects = useCallback(async () => {
    const list = await App.ListProjects("false");
    setProjects(list.projects);
    setProjectsVersion((v) => v + 1);
  }, []);
  const loadTasks = useCallback(async () => {
    const list = await App.ListTasks(wire.TaskQuery.createFrom({ project_id: "", status: "open", due_before: "" }));
    setTasks(list.tasks);
    setTasksVersion((v) => v + 1);
  }, []);

  const refresh = useCallback(async () => {
    try {
      await App.Health();
      const [s, c] = await Promise.all([App.Status(), App.GetConfig()]);
      setStatus(s);
      configRef.current = c;
      setConfig(c);
      await Promise.all([loadProjects(), loadTasks()]);
      setDaysVersion((v) => v + 1);
    } catch (e) {
      if (apiError(e).code === "daemon_not_running") setUp(false);
      else setNotice(apiError(e).message);
    }
  }, [loadProjects, loadTasks, setStatus]);

  useEffect(() => {
    refresh();
    const offs = [
      onEvent("state_changed", (s: wire.Status) => setStatus(wire.Status.createFrom(s))),
      onEvent("day_changed", () => setDaysVersion((v) => v + 1)),
      onEvent("projects_changed", () => loadProjects().catch(() => {})),
      onEvent("tasks_changed", () => loadTasks().catch(() => {})),
      onEvent("config_changed", (c: wire.Config) => {
        configRef.current = c;
        setConfig(c);
      }),
      // No replay after a reconnect: everything cached is stale.
      onEvent("connected", () => refresh()),
      onEvent("disconnected", () =>
        App.Health().then(
          () => {},
          () => setUp(false),
        ),
      ),
    ];
    return () => offs.forEach((off) => off());
  }, [refresh, loadProjects, loadTasks, setStatus]);

  const now = useCallback(() => Date.now() - skew.current, []);
  const today = useCallback(() => {
    if (status?.work_day) return status.work_day.day;
    return dayOf(now(), configRef.current?.tracking.day_rollover ?? "04:00");
  }, [status, now]);

  const fail = useCallback(
    (e: unknown) => {
      const err = apiError(e);
      if (err.code === "daemon_not_running") setUp(false);
      else if (isStale(err)) {
        setNotice("Changed elsewhere — reloaded");
        refresh();
      } else setNotice(err.message);
      return err;
    },
    [refresh],
  );

  const act = useCallback(
    async <T,>(call: () => Promise<T>): Promise<T | undefined> => {
      try {
        const out = await call();
        if (out && typeof out === "object" && "state" in out && "server_now_at" in out) setStatus(out as unknown as wire.Status);
        return out;
      } catch (e) {
        fail(e);
        return undefined;
      }
    },
    [fail, setStatus],
  );

  const value: Daemon = {
    up, status, config, projects, tasks, daysVersion, tasksVersion, projectsVersion,
    notice, setNotice, now, today, refresh, act, fail,
  };
  return <DaemonContext.Provider value={value}>{children}</DaemonContext.Provider>;
}
