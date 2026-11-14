import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { ApiError, api, type Approval, type DayStatus, type Plan, type Project, type Task } from "./api";
import { onGwen, useStore } from "./store";

// Your day on the phone (the phone redo): what you track, today's plan, your open tasks and projects, and the approvals of
// Gwen's agent that wait for you. Kept fresh by Gwen's events on the phone's stream; every screen reads it from useDay.

interface Day {
  status: DayStatus | null;
  plan: Plan | null;
  /** Open tasks, newest first. */
  tasks: Task[];
  projects: Project[];
  /** Approvals of Gwen's agent waiting for you. */
  approvals: Approval[];
  /** null until the first answer. */
  loaded: boolean;
  reload: () => void;
  /** Runs a change and shows its failure as a toast; resolves to whether it worked. */
  act: (fn: () => Promise<unknown>, done?: string) => Promise<boolean>;
  project: (id: string | null | undefined) => Project | undefined;
}

const Ctx = createContext<Day | null>(null);
export const useDay = () => useContext(Ctx)!;

export function DayProvider({ children }: { children: ReactNode }) {
  const { toast } = useStore();
  const [status, setStatus] = useState<DayStatus | null>(null);
  const [plan, setPlan] = useState<Plan | null>(null);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [approvals, setApprovals] = useState<Approval[]>([]);
  const [loaded, setLoaded] = useState(false);

  // Each part loads on its own, so one route the laptop can't answer (an older gwend) doesn't empty the rest.
  const load = useCallback(async (parts: Set<string>) => {
    const jobs: Promise<unknown>[] = [];
    if (parts.has("status")) jobs.push(api.status().then(setStatus));
    if (parts.has("plan")) jobs.push(api.plan().then(setPlan));
    if (parts.has("tasks")) jobs.push(api.tasks({ status: "open" }).then((l) => setTasks(l.tasks ?? [])));
    if (parts.has("projects")) jobs.push(api.projects().then((l) => setProjects((l.projects ?? []).filter((p) => !p.archived_at))));
    if (parts.has("approvals")) jobs.push(api.approvals().then((l) => setApprovals((l.approvals ?? []).filter((a) => a.status === "pending"))));
    await Promise.allSettled(jobs);
    setLoaded(true);
  }, []);

  // Events come in bursts (a switch changes the state, the plan and the tasks): gather them for a moment.
  const pending = useRef(new Set<string>());
  const timer = useRef(0);
  const want = useCallback(
    (...parts: string[]) => {
      for (const p of parts) pending.current.add(p);
      window.clearTimeout(timer.current);
      timer.current = window.setTimeout(() => {
        const next = pending.current;
        pending.current = new Set();
        load(next);
      }, 150);
    },
    [load],
  );
  const reload = useCallback(() => want("status", "plan", "tasks", "projects", "approvals"), [want]);

  useEffect(() => {
    reload();
    const stop = onGwen((name, data) => {
      switch (name) {
        case "state_changed":
          if (data && typeof data === "object" && "state" in data) setStatus(data as DayStatus);
          else want("status");
          want("plan");
          break;
        case "day_changed":
          want("status", "plan");
          break;
        case "tasks_changed":
          want("tasks", "plan");
          break;
        case "plan_changed":
          want("plan");
          break;
        case "projects_changed":
          want("projects");
          break;
        case "approval_requested":
        case "approval_decided":
          want("approvals");
          break;
      }
    });
    // A phone back from the background asks again: events may have been missed while it slept.
    const wake = () => document.visibilityState === "visible" && reload();
    document.addEventListener("visibilitychange", wake);
    return () => {
      stop();
      document.removeEventListener("visibilitychange", wake);
      window.clearTimeout(timer.current);
    };
  }, [reload, want]);

  const act = useCallback(
    async (fn: () => Promise<unknown>, done?: string) => {
      try {
        await fn();
        if (done) toast(done);
        return true;
      } catch (e) {
        toast(e instanceof ApiError ? e.message : "That didn't work.");
        return false;
      }
    },
    [toast],
  );
  const project = useCallback((id: string | null | undefined) => (id ? projects.find((p) => p.id === id) : undefined), [projects]);

  const value = useMemo<Day>(
    () => ({ status, plan, tasks, projects, approvals, loaded, reload, act, project }),
    [status, plan, tasks, projects, approvals, loaded, reload, act, project],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}
