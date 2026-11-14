import { ChevronDown, History, LoaderCircle, Mic, Plus, SendHorizontal, Square, X } from "lucide-react";
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import shockedFace from "../assets/gwen/shocked.png";
import smilingFace from "../assets/gwen/smiling.png";
import thinkingFace from "../assets/gwen/thinking.png";
import { ApiError, api, type AgentChat, type Approval } from "./api";
import { useDay } from "./day";
import { busyState, keepChat, keptChat, useAgentChat, useAgentChats, useAgentStatus, type Place } from "./gwen-chat";
import { Timeline } from "./gwen-timeline";
import { useTalk } from "./gwen-voice";
import "./gwen.css";
import { useStore } from "./store";
import { ago, Sheet } from "./ui";

// Gwen on the phone (the phone redo, P5): her agent's chats, the same ones as the window's Gwen › Chat. A place picker on top
// (everywhere, or a project) starts a chat there; the chats sheet lists them all. What she does shows as steps, what needs your OK
// as a card with Allow and Deny, and you talk to her by holding the mic.

const NAME = "Gwen";
const projectStarters = ["What's left in this project?", "Review my uncommitted changes", "Fix the failing tests", "Plan today's work here and get it started"];
const everywhereStarters = ["What should I do first today?", "Process my inbox", "Which project needs me most this week?", "Plan the rest of my day"];

export function Gwen({ chat }: { chat?: string }) {
  const status = useAgentStatus();
  if (!status) return <main className="g-screen" aria-busy="true" />;
  if (!status.available)
    return (
      <main className="g-screen">
        <div className="empty g-off">
          <img src={shockedFace} alt="" className="g-face big" />
          <h2>Gwen's agent can't run right now</h2>
          <p>{status.reason || (status.enabled ? "The laptop couldn't start it." : "It's turned off in Settings › Gwen › Agent on the laptop.")}</p>
        </div>
      </main>
    );
  return <AgentScreen initial={chat} />;
}

function AgentScreen({ initial }: { initial?: string }) {
  const { toast, go, route } = useStore();
  const day = useDay();
  const kept = useRef(keptChat()).current;
  // undefined: not chosen yet (the newest chat once the list is here); null: a new chat in newPlace until its first message.
  const [selected, setSelected] = useState<string | null | undefined>(initial ?? kept.chat);
  const [newPlace, setNewPlace] = useState<Place>(kept.place);
  const { chats, loaded, reload: reloadChats } = useAgentChats();
  useEffect(() => {
    if (selected === undefined && loaded) setSelected(chats[0]?.id ?? null);
  }, [selected, loaded, chats]);
  const id = selected ?? null;
  const live = useAgentChat(id);
  useEffect(() => {
    if (live.missing) setSelected(null);
  }, [live.missing]);

  const current: AgentChat | undefined = live.detail?.chat ?? chats.find((c) => c.id === id);
  const place: Place = id ? (current?.project_id ?? "everywhere") : newPlace;
  const project = place !== "everywhere" ? day.project(place) : undefined;
  const state = current?.state ?? "idle";
  const busy = busyState(state);
  const pending = live.detail?.approvals.length ?? current?.pending ?? 0;

  const choose = (c: string | null, where: Place = place) => {
    setSelected(c);
    keepChat(c, where);
    if (route.id) go({ name: "gwen" }, true); // a link to one chat is used once
  };
  const startNew = (where: Place) => {
    setNewPlace(where);
    choose(null, where);
  };

  const [text, setText] = useState("");
  const [sending, setSending] = useState<string | null>(null);
  // The field grows with what you write, up to six lines, and shrinks back once it is sent.
  const field = useRef<HTMLTextAreaElement>(null);
  useLayoutEffect(() => {
    const el = field.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 132)}px`;
  }, [text]);
  const send = useCallback(
    async (raw: string) => {
      const msg = raw.trim();
      if (!msg || sending) return;
      setSending(msg);
      setText("");
      try {
        let target = id;
        if (!target) {
          const c = await api.createAgentChat(place === "everywhere" ? null : place);
          target = c.id;
          setSelected(c.id);
          keepChat(c.id, place);
          reloadChats();
        }
        await api.sendAgent(target, msg);
        live.reload();
      } catch (e) {
        setText(msg);
        toast(e instanceof ApiError ? e.message : "Your message didn't reach the laptop.");
      } finally {
        setSending(null);
      }
    },
    [id, place, sending, live, reloadChats, toast],
  );
  const stop = () => id && api.stopAgent(id).then(live.reload, (e) => toast(e instanceof ApiError ? e.message : "She couldn't be stopped."));
  const decide = (a: Approval, approve: boolean, note?: string) =>
    api.decide(a.id, approve, note).then(
      () => {
        live.reload();
        day.reload();
      },
      (e) => toast(e instanceof ApiError ? e.message : "The laptop didn't get your answer."),
    );

  const talk = useTalk({ send, draft: (t) => setText((old) => (old.trim() ? `${old.trimEnd()} ${t}` : t)), error: toast }, !!sending);

  // Keep the newest in view while you are at the bottom.
  const scroller = useRef<HTMLDivElement>(null);
  const atBottom = useRef(true);
  useLayoutEffect(() => {
    const el = scroller.current;
    if (el && atBottom.current) el.scrollTop = el.scrollHeight;
  }, [live.detail, live.partial, sending]);

  const [sheet, setSheet] = useState<"" | "place" | "chats">("");
  const detail = live.detail;
  const empty = !id || (detail !== null && detail.messages.length === 0 && detail.steps.length === 0 && !sending && detail.queue.length === 0);
  const face = state === "error" ? shockedFace : busy ? thinkingFace : smilingFace;

  return (
    <main className="g-screen">
      <header className="g-head">
        <img src={face} alt="" className="g-face" />
        <div className="grow">
          <div className="g-name">{NAME}</div>
          <StateLine state={state} pending={pending} queued={current?.queued ?? 0} where={project?.name} />
        </div>
        <button type="button" className="g-place" onClick={() => setSheet("place")} aria-label={`Where this chat works: ${project?.name ?? "everywhere"}. Change it`}>
          {project?.color && <span className="g-dot" style={{ background: project.color }} aria-hidden />}
          <span className="clip">{project?.name ?? "Everywhere"}</span>
          <ChevronDown size={14} aria-hidden />
        </button>
        <button type="button" className="round" aria-label="Chats" onClick={() => setSheet("chats")}>
          <History size={19} aria-hidden />
        </button>
      </header>

      <div
        ref={scroller}
        className="g-scroll"
        onScroll={(e) => {
          const el = e.currentTarget;
          atBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
        }}
      >
        {id && !detail ? (
          <p className="g-thinking" role="status">
            <LoaderCircle size={14} className="g-spin" aria-hidden />
            Opening the chat
          </p>
        ) : empty && !sending ? (
          <Starters project={project?.name} onPick={send} disabled={!!sending} />
        ) : detail ? (
          <Timeline detail={detail} partial={live.partial} sending={sending} name={NAME} onDecide={decide} />
        ) : (
          <div className="g-you">
            <div className="g-bubble dim">{sending}</div>
            <span className="g-when">Sending…</span>
          </div>
        )}
        {current?.error && !busy && (
          <p className="g-error" role="alert">
            {current.error} Your next message starts her again.
          </p>
        )}
      </div>

      <footer className={`g-compose${talk.state ? " talking" : ""}`}>
        {talk.state ? (
          <div className={`g-talk${talk.armed ? " armed" : ""}`} role="status" aria-live="polite">
            <p className="g-heard">{talk.state === "hearing" ? talk.interim || "Turning it into words…" : talk.interim || "Listening…"}</p>
            <p className="g-talk-hint">
              {talk.state === "hearing"
                ? "Hearing what you said"
                : talk.armed
                  ? "Let go to cancel"
                  : talk.tapped
                    ? `Tap the mic when you're done · ${talk.seconds}s`
                    : `Release to send · slide left to cancel · ${talk.seconds}s`}
            </p>
            {talk.tapped && talk.state === "listening" && (
              <div className="g-talk-buttons">
                <button type="button" className="btn sm" onClick={talk.cancel}>
                  <X size={15} aria-hidden />
                  Cancel
                </button>
                <button type="button" className="btn sm primary" onClick={talk.send}>
                  Send now
                </button>
              </div>
            )}
          </div>
        ) : (
          <label className="g-field">
            <span className="sr">Message to {NAME}</span>
            <textarea
              ref={field}
              rows={1}
              value={text}
              maxLength={32000}
              onChange={(e) => setText(e.target.value)}
              placeholder={busy ? "Next message waits its turn" : `Tell ${NAME} what you need`}
            />
          </label>
        )}
        <div className="g-compose-buttons">
          <button type="button" className={`g-mic${talk.state === "listening" ? " on" : ""}${talk.armed ? " armed" : ""}`} aria-label={talk.tapped ? "Stop talking" : "Hold to talk, or tap to start"} {...talk.bind}>
            {talk.state === "hearing" ? <LoaderCircle size={22} className="g-spin" aria-hidden /> : <Mic size={22} aria-hidden />}
          </button>
          {!talk.state &&
            (busy && !text.trim() ? (
              // While she works and you have nothing to add, the send button stops her.
              <button type="button" className="round g-stop" aria-label="Stop her" onClick={stop}>
                <Square size={15} fill="currentColor" aria-hidden />
              </button>
            ) : (
              <button type="button" className="round send" aria-label="Send" disabled={!text.trim() || !!sending} onClick={() => send(text)}>
                <SendHorizontal size={19} aria-hidden />
              </button>
            ))}
        </div>
      </footer>

      {sheet === "place" && (
        <Sheet title="Where she works" onClose={() => setSheet("")}>
          <p className="note">Picking a place starts a new chat there. In a project she works in its folder and hands jobs to Claude there.</p>
          {[{ id: "everywhere", name: "Everywhere", color: "" }, ...day.projects.map((p) => ({ id: p.id, name: p.name, color: p.color }))].map((p) => (
            <button
              key={p.id}
              type="button"
              className={`item${p.id === place ? " on" : ""}`}
              onClick={() => {
                setSheet("");
                if (p.id !== place || id) startNew(p.id);
              }}
            >
              <span className="g-dot big" style={p.color ? { background: p.color } : undefined} aria-hidden />
              <span className="grow clip">{p.name}</span>
            </button>
          ))}
        </Sheet>
      )}
      {sheet === "chats" && <ChatsSheet chats={chats} selected={id} place={place} onClose={() => setSheet("")} onPick={(c) => (setSheet(""), choose(c.id, c.project_id ?? "everywhere"))} onNew={() => (setSheet(""), startNew(place))} />}
    </main>
  );
}

/** Under her name: what she is doing. */
function StateLine({ state, pending, queued, where }: { state: string; pending: number; queued: number; where?: string }) {
  let text: string;
  let tone = "";
  if (pending > 0 || state === "waiting") {
    text = `Waiting for your OK${pending > 1 ? ` (${pending})` : ""}`;
    tone = "warn";
  } else if (state === "starting") text = "Starting…";
  else if (state === "thinking") text = "Thinking";
  else if (state === "error") {
    text = "Stopped with an error";
    tone = "bad";
  } else text = where ? `Works in ${where}` : "For everywhere";
  return (
    <div className={`g-state clip ${tone}`}>
      {text}
      {queued > 0 && ` · ${queued} waiting`}
    </div>
  );
}

function Starters({ project, onPick, disabled }: { project?: string; onPick: (s: string) => void; disabled: boolean }) {
  return (
    <div className="g-starters">
      <p>
        {project
          ? `Tell ${NAME} what you want done in ${project}. She plans it with you and hands the work to Claude sessions you can follow from Work.`
          : `Ask ${NAME} anything about your day, tasks and projects. She changes them for you, and can get work going in any project.`}
      </p>
      {(project ? projectStarters : everywhereStarters).map((s) => (
        <button key={s} type="button" className="g-starter" disabled={disabled} onClick={() => onPick(s)}>
          {s}
        </button>
      ))}
    </div>
  );
}

/** Every chat, by place: everywhere first, then each project's; New chat starts one in the place you're in. */
function ChatsSheet({ chats, selected, place, onClose, onPick, onNew }: { chats: AgentChat[]; selected: string | null; place: Place; onClose: () => void; onPick: (c: AgentChat) => void; onNew: () => void }) {
  const day = useDay();
  const groups = [
    { id: "everywhere", name: "Everywhere", color: "", chats: chats.filter((c) => !c.project_id) },
    ...day.projects.map((p) => ({ id: p.id, name: p.name, color: p.color, chats: chats.filter((c) => c.project_id === p.id) })),
  ].filter((g) => g.chats.length > 0);
  const now = Date.now();
  return (
    <Sheet title="Chats" onClose={onClose}>
      <button type="button" className="item" onClick={onNew}>
        <Plus size={18} aria-hidden />
        <span className="grow">New chat {place !== "everywhere" ? `in ${day.project(place)?.name ?? "this project"}` : ""}</span>
      </button>
      {groups.length === 0 && <p className="note">No chats yet.</p>}
      {groups.map((g) => (
        <div key={g.id} className="g-group">
          <div className="g-group-name">
            <span className="g-dot" style={g.color ? { background: g.color } : undefined} aria-hidden />
            {g.name}
          </div>
          {g.chats.map((c) => (
            <button key={c.id} type="button" className={`item${c.id === selected ? " on" : ""}`} onClick={() => onPick(c)}>
              <span className="grow clip">{c.title || "New chat"}</span>
              {c.pending > 0 ? (
                <span className="g-badge">Needs your OK</span>
              ) : busyState(c.state) ? (
                <LoaderCircle size={15} className="g-spin g-info" aria-label="Working" />
              ) : (
                <span className="g-ago">{c.last_at ? ago(c.last_at, now) : ""}</span>
              )}
            </button>
          ))}
        </div>
      ))}
    </Sheet>
  );
}
