import { App, wire } from "./api";
import { useDaemon } from "./daemon";

/**
 * The tracking commands, with the same visibility rules as the tray menu
 * (docs/08-clients.md#tray), shared by Today, the status dock, and every list
 * with a start button.
 */
export function useTracking() {
  const d = useDaemon();
  const state = d.status?.state ?? "off";
  const working = state === "working" || state === "idle_pending";
  const onBreak = state === "break_auto" || state === "break_manual";

  const clockIn = (project_id: string | null, task_id: string | null) =>
    d.act(() => App.ClockIn(wire.ClockInRequest.createFrom({ project_id, task_id })));
  const switchTo = (project_id: string | null, task_id: string | null) =>
    d.act(() => App.Switch(wire.SwitchRequest.createFrom({ project_id, task_id })));

  return {
    state,
    working,
    onBreak,
    canStartBreak: working || state === "break_auto",
    clockIn,
    switchTo,
    /** Starts tracking a task: clocks in, switches to it, or ends the break on it. */
    async startTask(t: { id: string; project_id?: string | null }) {
      const project = t.project_id ?? null;
      if (state === "off") return clockIn(project, t.id);
      const s = await switchTo(project, t.id);
      if (s && onBreak) return d.act(() => App.BreakEnd());
      return s;
    },
    /** Whether the clock is running on this task right now. */
    isTracking: (taskId: string) => working && d.status?.task_id === taskId,
    breakStart: () => d.act(() => App.BreakStart()),
    breakEnd: () => d.act(() => App.BreakEnd()),
    snooze: () => d.act(() => App.Snooze()),
    clockOut: () => d.act(() => App.ClockOut()),
  };
}
