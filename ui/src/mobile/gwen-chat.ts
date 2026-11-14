import { useCallback, useEffect, useRef, useState } from "react";
import { api, type AgentChat, type AgentChatDetail, type AgentStatus, type AgentStep, type Approval } from "./api";
import { onGwen } from "./store";

// Gwen's agent on the phone, as the window's ui/src/agent/agent.ts keeps it: whether it can run, the chats, and one chat kept live
// by the events on the phone's stream (agent_changed, agent_text, agent_step, approval_requested, approval_decided).

interface AgentChanged { chat_id: string; state: string; error: string; queued: number; ended: boolean }

/** Where a chat belongs: a project's id, or "everywhere". */
export type Place = string;

/** True while she works on a turn. */
export const busyState = (state: string) => state === "thinking" || state === "starting" || state === "waiting";

/** Whether the agent can run on the laptop, and why not; null until known. */
export function useAgentStatus(): AgentStatus | null {
  const [status, setStatus] = useState<AgentStatus | null>(null);
  useEffect(() => {
    let live = true;
    const load = () =>
      api.agentStatus().then(
        (s) => live && setStatus(s),
        () => live && setStatus((old) => old ?? ({ enabled: false, available: false, reason: "The laptop didn't say whether Gwen's agent can run.", command: "", autonomy: "", live: 0 } as AgentStatus)),
      );
    load();
    const stop = onGwen((name, data) => name === "state_changed" && data === null && load());
    return () => {
      live = false;
      stop();
    };
  }, []);
  return status;
}

/** Every chat, newest first, kept fresh as chats change. */
export function useAgentChats(): { chats: AgentChat[]; loaded: boolean; reload: () => void } {
  const [chats, setChats] = useState<AgentChat[]>([]);
  const [loaded, setLoaded] = useState(false);
  const reload = useCallback(() => {
    api.agentChats().then(
      (l) => {
        setChats(l.chats ?? []);
        setLoaded(true);
      },
      () => setLoaded(true),
    );
  }, []);
  useEffect(() => {
    reload();
    const stop = onGwen((name, data) => {
      if (name === "agent_changed") {
        const e = data as AgentChanged;
        if (!e?.chat_id) return;
        setChats((list) => list.map((c) => (c.id === e.chat_id ? ({ ...c, state: e.state, error: e.error, queued: e.queued } as AgentChat) : c)));
        if (e.ended) reload();
      } else if (name === "approval_requested" || name === "approval_decided" || (name === "state_changed" && data === null)) reload();
    });
    return stop;
  }, [reload]);
  return { chats, loaded, reload };
}

export interface LiveChat {
  detail: AgentChatDetail | null;
  /** Her reply while she writes it. */
  partial: string;
  /** The chat is gone (deleted on the laptop). */
  missing: boolean;
  reload: () => void;
}

/** One chat, with its messages, steps, approvals and her reply as she writes it. */
export function useAgentChat(id: string | null): LiveChat {
  const [detail, setDetail] = useState<AgentChatDetail | null>(null);
  const [partial, setPartial] = useState("");
  const [missing, setMissing] = useState(false);
  const current = useRef(id);
  current.current = id;
  const reload = useCallback(() => {
    if (!id) return;
    api.agentChat(id).then(
      (c) => {
        if (current.current !== id) return;
        setDetail(c);
        setPartial(c.partial ?? "");
        setMissing(false);
      },
      (e) => current.current === id && (e as { status?: number })?.status === 404 && setMissing(true),
    );
  }, [id]);
  useEffect(() => {
    setDetail(null);
    setPartial("");
    setMissing(false);
    if (!id) return;
    reload();
    const stop = onGwen((name, data) => {
      switch (name) {
        case "agent_text":
          if (data?.chat_id === id) setPartial(data.text ?? "");
          break;
        case "agent_step": {
          const step = data?.step as AgentStep | undefined;
          if (data?.chat_id !== id || !step) return;
          setDetail((c) => {
            if (!c) return c;
            const at = c.steps.findIndex((s) => s.id === step.id);
            const steps = at < 0 ? [...c.steps, step] : c.steps.map((s, i) => (i === at ? step : s));
            return { ...c, steps } as AgentChatDetail;
          });
          break;
        }
        case "agent_changed": {
          const e = data as AgentChanged;
          if (e?.chat_id !== id) return;
          setDetail((c) => (c ? ({ ...c, chat: { ...c.chat, state: e.state, error: e.error, queued: e.queued } } as AgentChatDetail) : c));
          if (e.ended) {
            setPartial("");
            reload();
          } else reload();
          break;
        }
        case "approval_requested":
        case "approval_decided":
          if ((data as Approval)?.thread_id === id) reload();
          break;
        case "state_changed":
          if (data === null) reload(); // the stream came back: what changed meanwhile
          break;
      }
    });
    // A phone back from the background asks again: events may have been missed while it slept.
    const wake = () => document.visibilityState === "visible" && reload();
    document.addEventListener("visibilitychange", wake);
    return () => {
      stop();
      document.removeEventListener("visibilitychange", wake);
    };
  }, [id, reload]);
  return { detail, partial, missing, reload };
}

const keptKey = "gwen.phone.agent";

/** The chat open last on the phone: an id, null for a new chat in `place`, or undefined when there is none yet. */
export function keptChat(): { chat: string | null | undefined; place: Place } {
  try {
    const v = JSON.parse(localStorage.getItem(keptKey) ?? "{}");
    return { chat: typeof v.chat === "string" || v.chat === null ? v.chat : undefined, place: typeof v.place === "string" ? v.place : "everywhere" };
  } catch {
    return { chat: undefined, place: "everywhere" };
  }
}

export function keepChat(chat: string | null, place: Place) {
  try {
    localStorage.setItem(keptKey, JSON.stringify({ chat, place }));
  } catch {
    // storage is a convenience
  }
}
