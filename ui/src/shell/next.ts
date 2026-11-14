import { useEffect, useState } from "react";
import { App, wire } from "../api";
import { useDaemon, useTick } from "../daemon";

/** The next plan block or calendar event today, for the status bar and Today; not the task you are already on. */
export function useNext(): { title: string; start: number; taskId?: string } | null {
  const d = useDaemon();
  const [plan, setPlan] = useState<wire.Plan | null>(null);
  const today = d.today();
  useTick(30_000);
  useEffect(() => {
    let live = true;
    App.GetPlan(today).then(
      (p) => live && setPlan(p),
      () => live && setPlan(null),
    );
    return () => {
      live = false;
    };
  }, [today, d.planVersion, d.tasksVersion]);
  const now = d.now();
  const st = d.status;
  const current = st && st.state !== "off" ? st.task_id : null;
  const blocks = [
    ...(plan?.items ?? []).filter((it) => it.start_at != null && it.status === "planned" && it.start_at > now && it.task.id !== current).map((it) => ({ title: it.task.title, start: it.start_at!, taskId: it.task.id })),
    ...(plan?.events ?? []).filter((e) => e.start_at > now).map((e) => ({ title: e.title, start: e.start_at })),
  ];
  return blocks.sort((a, b) => a.start - b.start)[0] ?? null;
}

