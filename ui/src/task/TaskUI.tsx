import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { App, wire } from "../api";
import { useCompleteTask } from "../components/complete";
import { useConfirm, useToast } from "../components/feedback";
import { useDaemon } from "../daemon";
import { formatDate } from "../format";
import type { PlanGroup } from "../plan";
import { GoalForm } from "../screens/Goals";
import { TaskForm } from "../screens/Projects";
import { HandOffSheet } from "../shell/HandOff";
import { useWork } from "../shell/work";
import { useTracking } from "../tracking";
import { stageChoices, taskStatus, type AgentOnTask, type TaskStatus } from "./status";

// Everything you can do to a task, in one place for the whole window: the one task form (new and
// edit), the hand-off panel, completing with its steps and quantity questions, starting, moving
// between stages, and deleting. Screens call these instead of keeping their own modals and handlers.

export interface NewTaskDefaults {
  project?: string;
  stage?: string;
  /** The goal it is one of the tasks of. */
  goal?: string;
  /** How it repeats, such as a goal's daily task. */
  rrule?: string;
  /** Several tasks at once, one per line, such as a goal's set of tasks. */
  many?: boolean;
}

export interface TaskUI {
  /** Opens the task form on a task. */
  edit: (task: wire.Task) => void;
  /** Opens the task form for a new task, with these filled in. */
  create: (defaults?: NewTaskDefaults) => void;
  /** Opens the goal form for a new goal, over the whole window. */
  createGoal: () => void;
  /** Opens the hand-off panel for a task. */
  handOff: (task: wire.Task) => void;
  /** The task whose hand-off panel is open, so a page can keep its content clear of the panel. */
  handing: wire.Task | null;
  /** Ticks a task off; asks about its open steps or how many it covered when that matters. */
  complete: (task: wire.Task) => void;
  reopen: (task: wire.Task) => void;
  /** Starts tracking it; a task still in the inbox or to do moves to In progress, the same from every screen. */
  start: (task: wire.Task) => void;
  /** Skips its blocks on today's plan, or puts them back. */
  skipToday: (group: PlanGroup) => void;
  putBack: (group: PlanGroup) => void;
  /** Takes it off today's plan altogether; Gwen won't plan it today again. */
  removeFromToday: (group: PlanGroup) => void;
  /** Takes back a task handed to someone (or an agent whose session ended): it returns to To do. */
  takeBack: (task: wire.Task) => void;
  /** Moves it to another stage on the board (Inbox, To do, In progress, Waiting, Someday). */
  move: (task: wire.Task, stage: string) => void;
  /** Asks, then deletes it with its steps. */
  remove: (task: wire.Task) => void;
  /** The live Claude session working on it, if any. */
  agentOf: (task: wire.Task) => AgentOnTask | null;
  /** The terminal of that session, to open it. */
  agentTerminal: (task: wire.Task) => string | null;
  statusOf: (task: wire.Task) => TaskStatus;
  /** Its open steps (subtasks). */
  stepsOf: (task: wire.Task) => wire.Task[];
}

const Ctx = createContext<TaskUI | null>(null);

/** "today", or "on Fri 14 Nov" for a plan group on another day. */
export function dayWords(g: PlanGroup, today: string): string {
  const day = g.items[0]?.day ?? today;
  return day === today ? "today" : `on ${formatDate(day)}`;
}

export function TaskUIProvider({ children }: { children: ReactNode }) {
  const d = useDaemon();
  const w = useWork();
  const t = useTracking();
  const toast = useToast();
  const confirm = useConfirm();
  const done = useCompleteTask();
  const [editing, setEditing] = useState<{ task: wire.Task | null; defaults: NewTaskDefaults } | null>(null);
  const [handing, setHanding] = useState<wire.Task | null>(null);
  const [newGoal, setNewGoal] = useState(false);
  const [goals, setGoals] = useState<wire.Goal[]>([]);
  const completing = useRef(new Set<string>());

  // The form lists active goals, and the one the task already has.
  useEffect(() => {
    if (!editing) return;
    App.ListGoals("active").then(
      (l) => setGoals(l.goals),
      () => setGoals([]),
    );
  }, [editing, d.goalsVersion]);

  // A task deleted or finished elsewhere takes its open panel with it.
  useEffect(() => {
    if (handing && !d.tasks.some((x) => x.id === handing.id)) setHanding(null);
  }, [d.tasks, handing]);

  const stepsOf = useCallback((task: wire.Task) => d.tasks.filter((s) => s.parent_id === task.id), [d.tasks]);
  const agentTerm = useCallback((task: wire.Task) => w.terminals.find((x) => x.claude && x.task === task.id) ?? null, [w.terminals]);

  const ui = useMemo<TaskUI>(() => {
    const agentOf = (task: wire.Task): AgentOnTask | null => {
      const term = agentTerm(task);
      return term ? { state: term.session?.state ?? term.status, name: "Claude" } : null;
    };
    return {
      edit: (task) => setEditing({ task, defaults: {} }),
      create: (defaults = {}) => setEditing({ task: null, defaults }),
      createGoal: () => setNewGoal(true),
      handOff: (task) => setHanding(task),
      handing,
      complete: (task) => {
        // One completion per task at a time, however fast the box is clicked.
        if (completing.current.has(task.id)) return;
        completing.current.add(task.id);
        window.setTimeout(() => completing.current.delete(task.id), 1500);
        done.complete(task, stepsOf(task).length > 0);
      },
      reopen: (task) => done.reopen(task),
      start: async (task) => {
        const ok = await t.startTask(task);
        if (!ok || (task.stage !== "inbox" && task.stage !== "todo")) return;
        if (await d.act(() => App.PatchTask(task.id, wire.PatchTaskRequest.createFrom({ stage: "doing", rev: task.rev })))) {
          if (task.stage === "inbox") toast(`Started ${task.title}: it moved to In progress`);
        }
      },
      skipToday: (g) => {
        const live = g.items.filter((it) => it.status === "planned");
        Promise.all(live.map((it) => d.act(() => App.PatchPlanItem(it.id, wire.PatchPlanItemRequest.createFrom({ status: "skipped", rev: it.rev }))))).then(() =>
          toast(`Skipped ${g.task.title} ${dayWords(g, d.today())}`),
        );
      },
      putBack: (g) => {
        const skipped = g.items.filter((it) => it.status === "skipped");
        Promise.all(skipped.map((it) => d.act(() => App.PatchPlanItem(it.id, wire.PatchPlanItemRequest.createFrom({ status: "planned", rev: it.rev })))));
      },
      removeFromToday: (g) => {
        const day = g.items[0]?.day ?? d.today();
        d.act(() => App.UnscheduleTask(wire.UnscheduleRequest.createFrom({ task_id: g.task.id, day }))).then((ok) => ok && toast(`Took ${g.task.title} off the plan ${dayWords(g, d.today())}`));
      },
      takeBack: (task) => {
        d.act(() => App.PatchTask(task.id, wire.PatchTaskRequest.createFrom({ stage: "todo", delegated_to: "", rev: task.rev }))).then((ok) => ok && toast(`${task.title} is back with you`));
      },
      move: (task, stage) => {
        if (task.stage === stage) return;
        d.act(() => App.PatchTask(task.id, wire.PatchTaskRequest.createFrom({ stage, rev: task.rev }))).then((ok) => {
          if (ok) toast(`Moved to ${stageChoices.find((s) => s.value === stage)?.label ?? stage}`);
        });
      },
      remove: async (task) => {
        const steps = stepsOf(task).length;
        if (
          await confirm({
            title: `Delete ${task.title}?`,
            body: steps > 0 ? `Its ${steps} ${steps === 1 ? "step goes" : "steps go"} with it. Time already tracked stays in your day.` : "Time already tracked stays in your day.",
            confirm: "Delete task",
            danger: true,
          })
        )
          d.run(() => App.DeleteTask(task.id));
      },
      agentOf,
      agentTerminal: (task) => agentTerm(task)?.address ?? null,
      statusOf: (task) => taskStatus(task, { tracking: t.isTracking(task.id), agent: agentOf(task) }),
      stepsOf,
    };
  }, [d, t, toast, confirm, done, stepsOf, agentTerm, handing]);

  return (
    <Ctx.Provider value={ui}>
      {children}
      {editing && (
        <TaskForm
          task={editing.task}
          projects={d.projects}
          goals={goals.filter((g) => g.status === "active" || g.id === editing.task?.goal_id)}
          defaultProject={editing.defaults.project ?? ""}
          defaultStage={editing.defaults.stage}
          defaultGoal={editing.defaults.goal}
          defaultRule={editing.defaults.rrule}
          defaultMany={editing.defaults.many}
          onClose={() => setEditing(null)}
        />
      )}
      {handing && <HandOffSheet key={handing.id} task={handing} onClose={() => setHanding(null)} />}
      {newGoal && <GoalForm goal={null} onClose={() => setNewGoal(false)} />}
      {done.dialog}
    </Ctx.Provider>
  );
}

export function useTaskUI(): TaskUI {
  const v = useContext(Ctx);
  if (!v) throw new Error("useTaskUI outside TaskUIProvider");
  return v;
}
