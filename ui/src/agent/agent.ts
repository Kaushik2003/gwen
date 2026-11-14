import { useCallback, useEffect, useRef, useState } from "react";
import { App, onEvent, wire } from "../api";
import { useDaemon } from "../daemon";
import { tabProject, type Tab } from "../shell/tabs";

// Gwen's agent (plan.md 3.16, W32) as the window sees it: whether it can run, the chats of a place, and one chat kept
// live by the daemon's events. The chat itself is AgentChat; Gwen › Chat and the side panel both use it.

/** The daemon's agent events (internal/wire/agent.go). */
interface AgentChanged {
  chat_id: string;
  state: string;
  error: string;
  queued: number;
  ended: boolean;
}
interface AgentText {
  chat_id: string;
  text: string;
}
interface AgentStepEvent {
  chat_id: string;
  step: wire.AgentStep;
}

/** Where chats belong: a project's id, "everywhere" (no project), or "all". */
export type Place = string;

/** Whether the agent can run here, and why not; null until known. */
export function useAgentStatus(): wire.AgentStatus | null {
  const d = useDaemon();
  const [status, setStatus] = useState<wire.AgentStatus | null>(null);
  useEffect(() => {
    let live = true;
    App.AgentStatus().then(
      (s) => live && setStatus(s),
      () => live && setStatus(wire.AgentStatus.createFrom({ enabled: false, available: false, reason: "Gwen's agent doesn't run on this device." })),
    );
    return () => {
      live = false;
    };
  }, [d.config?.agent?.enabled, d.config?.llm.provider, d.config?.llm.command]);
  return status;
}

/** The chats of a place, newest first, kept fresh as chats change. */
export function useAgentChats(place: Place, enabled = true): { chats: wire.AgentChat[]; loaded: boolean; reload: () => void } {
  const [chats, setChats] = useState<wire.AgentChat[]>([]);
  const [loaded, setLoaded] = useState(false);
  const reload = useCallback(() => {
    if (!enabled) return;
    App.ListAgentChats(place === "all" ? "" : place).then(
      (l) => {
        setChats(l.chats ?? []);
        setLoaded(true);
      },
      () => setLoaded(true),
    );
  }, [place, enabled]);
  useEffect(() => {
    setLoaded(false);
    reload();
    // A state change patches the row; a turn's end or an approval reads them again.
    const offs = [
      onEvent("agent_changed", (e: AgentChanged) => {
        if (!e?.chat_id) return;
        setChats((list) => list.map((c) => (c.id === e.chat_id ? { ...c, state: e.state, error: e.error, queued: e.queued } : c)));
        if (e.ended) reload();
      }),
      onEvent("approval_requested", reload),
      onEvent("approval_decided", reload),
      onEvent("connected", reload),
    ];
    return () => offs.forEach((off) => off());
  }, [reload]);
  return { chats, loaded, reload };
}

export interface LiveChat {
  detail: wire.AgentChatDetail | null;
  /** Her reply while she writes it. */
  partial: string;
  missing: boolean;
  reload: () => void;
}

/** One chat, with its messages, steps, approvals and her reply as she writes it. */
export function useAgentChat(id: string | null): LiveChat {
  const [detail, setDetail] = useState<wire.AgentChatDetail | null>(null);
  const [partial, setPartial] = useState("");
  const [missing, setMissing] = useState(false);
  const current = useRef(id);
  current.current = id;
  const reload = useCallback(() => {
    if (!id) return;
    App.GetAgentChat(id).then(
      (c) => {
        if (current.current !== id) return;
        setDetail(c);
        setPartial(c.partial ?? "");
        setMissing(false);
      },
      () => current.current === id && setMissing(true),
    );
  }, [id]);
  useEffect(() => {
    setDetail(null);
    setPartial("");
    setMissing(false);
    if (!id) return;
    reload();
    const offs = [
      onEvent("agent_text", (e: AgentText) => e?.chat_id === id && setPartial(e.text ?? "")),
      onEvent("agent_step", (e: AgentStepEvent) => {
        if (e?.chat_id !== id || !e.step) return;
        setDetail((c) => {
          if (!c) return c;
          const at = c.steps.findIndex((s) => s.id === e.step.id);
          const steps = at < 0 ? [...c.steps, e.step] : c.steps.map((s, i) => (i === at ? e.step : s));
          return { ...c, steps } as wire.AgentChatDetail;
        });
      }),
      onEvent("agent_changed", (e: AgentChanged) => {
        if (e?.chat_id !== id) return;
        setDetail((c) => (c ? ({ ...c, chat: { ...c.chat, state: e.state, error: e.error, queued: e.queued } } as wire.AgentChatDetail) : c));
        if (e.ended) {
          setPartial("");
          reload();
        } else if (e.queued !== undefined) reload();
      }),
      onEvent("approval_requested", (a: wire.Approval) => a?.thread_id === id && reload()),
      onEvent("approval_decided", (a: wire.Approval) => a?.thread_id === id && reload()),
      onEvent("connected", reload),
    ];
    return () => offs.forEach((off) => off());
  }, [id, reload]);
  return { detail, partial, missing, reload };
}

/** What the window shows, as the agent's focus for a message. */
export function focusOf(tab: Tab): wire.AgentFocus {
  switch (tab.kind) {
    case "today":
      return wire.AgentFocus.createFrom({ kind: "today" });
    case "screen":
      return wire.AgentFocus.createFrom({ kind: "screen", screen: [tab.screen, tab.sub].filter(Boolean).join(" › ") });
    case "project":
      return wire.AgentFocus.createFrom({ kind: "project", project_id: tab.project });
    case "file":
      return wire.AgentFocus.createFrom({ kind: "file", project_id: tab.project, path: tab.path });
    case "term":
      return wire.AgentFocus.createFrom({ kind: "terminal", project_id: tab.project, terminal: tab.terminal });
    case "history":
      return wire.AgentFocus.createFrom({ kind: "history", project_id: tab.project });
  }
}

/** A tab's focus in a few words, for its chip; null when there is nothing worth a chip. */
export function focusLabel(tab: Tab, terminalName?: string): string | null {
  switch (tab.kind) {
    case "file":
      return tab.path.split("/").pop() || tab.path;
    case "term":
      return terminalName || "Terminal";
    case "history":
      return "History";
  }
  return null;
}

/** The place of a tab: its project, or everywhere. */
export function placeOfTab(tab: Tab): Place {
  return tabProject(tab) || "everywhere";
}

const lastKey = "gwen.agent.last";

/** The chat last open in a place, kept in the webview's storage (a convenience). */
export function lastChat(place: Place): string | null {
  try {
    const v = JSON.parse(localStorage.getItem(lastKey) ?? "{}");
    return typeof v[place] === "string" ? v[place] : null;
  } catch {
    return null;
  }
}

export function rememberChat(place: Place, id: string | null) {
  try {
    const v = JSON.parse(localStorage.getItem(lastKey) ?? "{}");
    if (id) v[place] = id;
    else delete v[place];
    localStorage.setItem(lastKey, JSON.stringify(v));
  } catch {
    // storage is a convenience
  }
}

/** Autonomy as the chat's menu says it. */
export const autonomyChoices: { value: string; label: string; hint: string }[] = [
  { value: "cautious", label: "Cautious", hint: "Asks before every change and every job" },
  { value: "balanced", label: "Balanced", hint: "Asks before the first job in a chat; Claude asks in its session" },
  { value: "trusted", label: "Trusted", hint: "Starts jobs freely; Claude works in auto mode" },
];

export function autonomyLabel(v: string): string {
  return autonomyChoices.find((c) => c.value === v)?.label ?? "Balanced";
}

/** True while she works on a turn. */
export function busyState(state: string): boolean {
  return state === "thinking" || state === "starting" || state === "waiting";
}
