import { useEffect, useMemo, useRef, useState } from "react";
import { App, isHub, wire } from "../api";
import type { BadgeTone } from "../components/ui";
import { useDaemon } from "../daemon";
import { useWork } from "../shell/work";

// The agentic timeline's data (plan.md 3.17): the day as a graph from gwend (GET /v1/days/{day}/graph), kept fresh as
// the plan, tasks, tracking, and Claude sessions change, and read again each minute so "heading to end" keeps moving.

export interface DayGraph {
  graph: wire.DayGraph | null;
  /** Why there is no graph, in a sentence: the hub can't plan, or the daemon refused. */
  error: string;
  byId: Map<string, wire.GraphTask>;
  reload: () => void;
}

export function useDayGraph(day: string): DayGraph {
  const d = useDaemon();
  const w = useWork();
  const [graph, setGraph] = useState<wire.DayGraph | null>(null);
  const [error, setError] = useState("");
  const [tick, setTick] = useState(0);
  const latest = useRef(0);

  // What agents are doing, as one key, so a session's step changing doesn't fetch again.
  const agents = w.sessions.map((s) => `${s.id}:${s.state}:${s.task_id}`).join(",");

  useEffect(() => {
    const id = window.setInterval(() => setTick((n) => n + 1), 60_000);
    return () => window.clearInterval(id);
  }, []);

  useEffect(() => {
    if (isHub) {
      setError("The day's graph is worked out on the laptop.");
      return;
    }
    const n = ++latest.current;
    // Changes come in bursts (a tick moves the plan and the task); read once they settle.
    const t = window.setTimeout(() => {
      App.GetDayGraph(day).then(
        (g) => {
          if (n !== latest.current) return;
          setGraph(g);
          setError("");
        },
        (e) => {
          if (n !== latest.current) return;
          setError(String((e as { message?: string })?.message ?? e));
        },
      );
    }, 250);
    return () => window.clearTimeout(t);
  }, [day, d.daysVersion, d.planVersion, d.tasksVersion, d.status?.state, d.status?.task_id, agents, tick]);

  const byId = useMemo(() => new Map((graph?.tasks ?? []).map((t) => [t.id, t])), [graph]);
  return { graph, error, byId, reload: () => setTick((n) => n + 1) };
}

export interface StateMeta {
  label: string;
  tone: BadgeTone;
  /** The colour of its dot, a CSS colour. */
  color: string;
}

export const stateMeta: Record<string, StateMeta> = {
  done: { label: "Done", tone: "working", color: "var(--color-success)" },
  waiting_on_you: { label: "Waiting on you", tone: "idle", color: "var(--color-warning)" },
  blocked: { label: "Blocked", tone: "danger", color: "var(--color-danger)" },
  at_risk: { label: "Running late", tone: "idle", color: "var(--color-warning)" },
  working: { label: "On it now", tone: "working", color: "var(--color-working)" },
  agent: { label: "With Claude", tone: "accent", color: "var(--color-info)" },
  ready: { label: "On track", tone: "neutral", color: "var(--color-ink-subtle)" },
  unplanned: { label: "Not planned", tone: "neutral", color: "var(--color-ink-faint)" },
};

export function metaOf(state: string): StateMeta {
  return stateMeta[state] ?? stateMeta.ready;
}

/** Agent states as the timeline names them. */
export const agentStateLabel: Record<string, string> = {
  starting: "Starting",
  working: "Working",
  needs_you: "Needs you",
  done: "Finished",
  idle: "Idle",
  error: "Stopped with an error",
  ended: "Ended",
};

export interface ChainNode {
  task: wire.GraphTask;
  depth: number;
}

/** Everything a task waits on in turn ("after") or that waits on it in turn ("before"), nearest first, each once. */
export function chain(byId: Map<string, wire.GraphTask>, id: string, dir: "after" | "before"): ChainNode[] {
  const out: ChainNode[] = [];
  const seen = new Set([id]);
  const walk = (from: string, depth: number) => {
    const t = byId.get(from);
    for (const next of (dir === "after" ? t?.waits_on : t?.blocks) ?? []) {
      const n = byId.get(next);
      if (!n || seen.has(next) || depth > 6) continue;
      seen.add(next);
      out.push({ task: n, depth });
      walk(next, depth + 1);
    }
  };
  walk(id, 0);
  return out;
}

/** Minutes as people say them: "40 min", "1 h 5 min". */
export function minutesWords(m: number): string {
  if (m < 60) return `${m} min`;
  return m % 60 === 0 ? `${m / 60} h` : `${Math.floor(m / 60)} h ${m % 60} min`;
}

/** The ask Gwen gets from the timeline: the day as a whole, or one task. */
export function gwenAsk(task?: wire.GraphTask): string {
  if (!task)
    return "Look at my day with day_graph. What's in my way or running late, and what should change? Fix what you can with the smallest changes (what waits on what, when things are planned, or a Claude session for a blocker), saying in a line what each change is for.";
  return `Look at my day with day_graph, at “${task.title}” (task ${task.id.slice(0, 8)}). ${task.reason ? `It says: ${task.reason}. ` : ""}What is holding it up, and what's the smallest change that gets it done today? Make the change if it's clear, saying in a line why.`;
}
