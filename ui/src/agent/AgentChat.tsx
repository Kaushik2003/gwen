import {
  Bot,
  Check,
  ChevronDown,
  ChevronRight,
  CircleAlert,
  CircleX,
  Clock,
  FileText,
  Globe,
  ListChecks,
  LoaderCircle,
  MessageSquarePlus,
  Pencil,
  SendHorizontal,
  ShieldCheck,
  Square,
  SquareTerminal,
  Trash2,
  Undo2,
  X,
  type LucideIcon,
} from "lucide-react";
import { useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { App, wire } from "../api";
import { ApprovalCard } from "../components/ApprovalCard";
import Face from "../components/Face";
import { useConfirm, useToast } from "../components/feedback";
import { Button, IconButton, cx } from "../components/ui";
import { useDictation } from "../components/Voice";
import { useDaemon } from "../daemon";
import { formatTime } from "../format";
import { Markdown } from "../mobile/Markdown";
import { BoardMenu, type MenuEntry } from "../screens/BoardMenu";
import { ClaudeMark } from "../shell/bits";
import { useTabs } from "../shell/tabs";
import { useWork } from "../shell/work";
import { autonomyChoices, autonomyLabel, busyState, focusLabel, focusOf, lastChat, openChatEvent, rememberChat, useAgentChat, useAgentChats, type Place } from "./agent";

// Gwen's agent in a chat (plan.md 3.16): her replies as she writes them, every tool call as a quiet row under her text,
// the approvals she waits on pinned above the box, the sessions she started as cards, and chips saying what she will
// see with your next message. The side panel shows the open project's chats; Gwen › Chat shows any chat.

const projectStarters = ["What's left in this project?", "Review my uncommitted changes", "Fix the failing tests", "Plan today's work here and get it started"];
const everywhereStarters = ["What should I do first today?", "Process my inbox", "Which project needs me most this week?", "Plan the rest of my day"];

export default function AgentChat({
  place,
  chatId,
  onChat,
  screen,
  headerStart,
}: {
  /** The project the chats belong to, or "everywhere". */
  place: Place;
  /** Controlled by Gwen › Chat; left out, the chat last open in the place is shown. */
  chatId?: string | null;
  onChat?: (id: string | null) => void;
  /** Drawn on Gwen › Chat: more room, no chat switcher (the screen lists the chats). */
  screen?: boolean;
  headerStart?: ReactNode;
}) {
  const d = useDaemon();
  const tabs = useTabs();
  const w = useWork();
  const notify = useToast();
  const confirm = useConfirm();
  const controlled = chatId !== undefined;
  const [own, setOwn] = useState<string | null>(() => lastChat(place));
  // A new chat you asked for (New chat, or after deleting one): kept empty until your first message makes it, rather
  // than replaced by the newest chat of the place.
  const fresh = useRef(false);
  // The chat this panel just made from your first message: the place's list doesn't hold it until it reloads, and
  // until then it is not "gone", so the newest older chat must not take its place.
  const made = useRef<string | null>(null);
  const id = controlled ? chatId : own;
  const setId = (next: string | null) => {
    fresh.current = next === null;
    rememberChat(place, next);
    if (controlled) onChat?.(next);
    else setOwn(next);
  };
  useEffect(() => {
    fresh.current = false;
    if (!controlled) setOwn(lastChat(place));
  }, [place, controlled]);
  // Another part of the window started a chat here (Ask Gwen on the timeline): show it.
  useEffect(() => {
    if (controlled) return;
    const show = (e: Event) => {
      const { place: at, id: chat } = (e as CustomEvent<{ place: Place; id: string }>).detail;
      if (at !== place) return;
      made.current = chat;
      fresh.current = false;
      setOwn(chat);
      reloadChats();
    };
    window.addEventListener(openChatEvent, show);
    return () => window.removeEventListener(openChatEvent, show);
  });

  const { chats, loaded, reload: reloadChats } = useAgentChats(place, !screen);
  // The chat last open, else the newest one of the place.
  useEffect(() => {
    if (controlled || !loaded || fresh.current) return;
    if (own && (own === made.current || chats.some((c) => c.id === own))) return;
    setOwn(chats[0]?.id ?? null);
  }, [controlled, loaded, chats, own]);
  const live = useAgentChat(id ?? null);
  useEffect(() => {
    if (live.missing && id) setId(null);
  }, [live.missing]);

  const project = place !== "everywhere" ? (d.projects.find((p) => p.id === place) ?? null) : null;
  const detail = live.detail;
  const chat = detail?.chat;
  const state = chat?.state ?? "idle";
  const busy = busyState(state);
  const name = d.config?.llm.assistant_name || "Gwen";

  const [text, setText] = useState("");
  const [sending, setSending] = useState<string | null>(null);
  const input = useRef<HTMLTextAreaElement>(null);
  const before = useRef("");
  const voice = useDictation({
    onStart: () => {
      before.current = text;
      input.current?.focus();
    },
    onText: (said) => setText(said ? (before.current.trim() ? `${before.current.trimEnd()} ${said}` : said) : before.current),
  });

  // The open tab, offered as a chip: a file, a terminal, or the history. × leaves it out of the next message.
  const active = tabs.activeTab;
  const termName = active.kind === "term" ? w.terminals.find((t) => t.address === active.terminal)?.name : undefined;
  const focus = focusLabel(active, termName);
  const [dropFocus, setDropFocus] = useState(false);
  useEffect(() => setDropFocus(false), [active]);

  async function send(raw: string) {
    const msg = raw.trim();
    if (!msg || sending) return;
    setSending(msg);
    setText("");
    try {
      let target = id;
      if (!target) {
        const c = await App.CreateAgentChat(wire.CreateAgentChat.createFrom({ project_id: project ? project.id : null }));
        target = c.id;
        made.current = c.id;
        setId(c.id);
        reloadChats();
      }
      await App.SendAgentMessage(target, wire.AgentMessageRequest.createFrom({ text: msg, focus: dropFocus ? undefined : focusOf(active) }));
      setDropFocus(false);
      live.reload();
    } catch (e) {
      setText(msg);
      d.fail(e);
    } finally {
      setSending(null);
    }
  }

  const stop = () => id && App.StopAgentChat(id).then(live.reload, d.fail);
  const remove = async () => {
    if (!id || !chat) return;
    const ok = await confirm({
      title: `Delete “${chat.title}”?`,
      body: "Its messages are erased, and what Gwen learned from it is forgotten. Changes she made stay.",
      confirm: "Delete chat",
      danger: true,
    });
    if (!ok) return;
    try {
      await App.DeleteAgentChat(id);
      setId(null);
      reloadChats();
    } catch (e) {
      d.fail(e);
    }
  };
  const setAutonomy = (v: string) =>
    project && d.act(() => App.PatchProject(project.id, wire.PatchProjectRequest.createFrom({ agent_autonomy: v }))).then((p) => p && notify(`Gwen is ${autonomyLabel(v || d.config?.agent?.autonomy || "balanced").toLowerCase()} in ${project.name}`));

  // Keep the newest in view while you are at the bottom.
  const scroller = useRef<HTMLDivElement>(null);
  const atBottom = useRef(true);
  useLayoutEffect(() => {
    const el = scroller.current;
    if (el && atBottom.current) el.scrollTop = el.scrollHeight;
  }, [detail, live.partial, sending]);

  const switcher: MenuEntry[] = [
    ...chats.map((c): MenuEntry => ({ label: c.title || "New chat", checked: c.id === id, run: () => setId(c.id) })),
    ...(chats.length ? [{ kind: "divider" } as MenuEntry] : []),
    { label: "New chat", run: () => setId(null) },
    ...(id ? [{ label: "Delete this chat", run: remove } as MenuEntry] : []),
  ];
  const autonomy = project?.agent_autonomy || "";
  const autonomyMenu: MenuEntry[] = [
    { label: `Default (${autonomyLabel(d.config?.agent?.autonomy || "balanced")})`, checked: !autonomy, run: () => setAutonomy("") },
    { kind: "divider" },
    ...autonomyChoices.map((c): MenuEntry => ({ label: `${c.label}: ${c.hint}`, checked: autonomy === c.value, run: () => setAutonomy(c.value) })),
  ];
  const starters = project ? projectStarters : everywhereStarters;
  const empty = !detail || (detail.messages.length === 0 && !sending && detail.queue.length === 0);

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className={cx("flex h-12 shrink-0 items-center gap-2 border-b border-chrome-line", screen ? "px-4" : "pr-2 pl-3")}>
        {headerStart}
        <Face mood={state === "error" ? "shocked" : busy ? "thinking" : "smiling"} react talking={state === "thinking" && !!live.partial} className="size-7 shrink-0" />
        <div className="min-w-0 flex-1">
          {screen ? (
            <div className="truncate text-[13.5px] font-semibold text-ink">{chat?.title || "New chat"}</div>
          ) : (
            <BoardMenu
              label="Chats"
              entries={switcher}
              align="start"
              trigger={({ open, toggle, ref }) => (
                <button
                  ref={ref}
                  type="button"
                  onClick={toggle}
                  aria-haspopup="menu"
                  aria-expanded={open}
                  title={project ? `${project.name}'s chats` : "Chats for everywhere"}
                  className="-ml-1 flex max-w-full min-w-0 items-center gap-1 rounded-md px-1 py-0.5 text-left hover:bg-surface-2"
                >
                  <span className="min-w-0 truncate text-[13px] font-semibold text-ink">{chat?.title || (id ? "…" : "New chat")}</span>
                  <ChevronDown size={13} className="shrink-0 text-ink-subtle" aria-hidden />
                </button>
              )}
            />
          )}
          <StateLine state={state} name={name} queued={chat?.queued ?? 0} pending={detail?.approvals.length ?? 0} where={project?.name} />
        </div>
        {project && (
          <BoardMenu
            label="How freely Gwen acts here"
            entries={autonomyMenu}
            trigger={({ open, toggle, ref }) => (
              <button
                ref={ref}
                type="button"
                onClick={toggle}
                aria-haspopup="menu"
                aria-expanded={open}
                title="How freely Gwen acts in this project"
                aria-label={`Autonomy here: ${autonomyLabel(autonomy || d.config?.agent?.autonomy || "balanced")}`}
                className={cx("flex h-7 shrink-0 items-center gap-1 rounded-md text-[11.5px] font-medium text-ink-muted hover:text-ink", screen ? "border border-line px-2 hover:border-line-3" : "w-7 justify-center hover:bg-surface-2")}
              >
                <ShieldCheck size={screen ? 12 : 14} aria-hidden />
                {screen && autonomyLabel(autonomy || d.config?.agent?.autonomy || "balanced")}
              </button>
            )}
          />
        )}
        {busy && <IconButton size="sm" icon={Square} label="Stop" onClick={stop} />}
        {screen && id && <IconButton size="sm" icon={Trash2} label="Delete this chat" onClick={remove} />}
        {!screen && <IconButton size="sm" icon={MessageSquarePlus} label="New chat" onClick={() => setId(null)} />}
      </div>

      {detail && <JobsBar steps={detail.steps} projectId={project?.id} />}

      <div
        ref={scroller}
        onScroll={(e) => {
          const el = e.currentTarget;
          atBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 60;
        }}
        className={cx("flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto", screen ? "px-6 py-5" : "px-3.5 py-3.5")}
      >
        {empty ? (
          <EmptyChat name={name} project={project?.name} starters={starters} onPick={send} disabled={!!sending} />
        ) : (
          <Timeline detail={detail!} partial={live.partial} sending={sending} name={name} folder={project?.folder} projectId={project?.id} onUndo={(s) => App.UndoAgentStep(s.id).then(() => notify("Undone"), d.fail)} />
        )}
      </div>

      <div className={cx("flex shrink-0 flex-col gap-2 border-t border-chrome-line", screen ? "px-5 pt-3 pb-4" : "px-3 pt-2.5 pb-3")}>
        {!!detail?.approvals.length && (
          <div className={cx("-mx-1 flex flex-col gap-2 overflow-y-auto px-1", screen ? "max-h-[55vh]" : "max-h-[45vh]")}>
            {detail.approvals.map((a) => (
          <ApprovalCard
            key={a.id}
            approval={a}
            compact={!screen}
            projectName={project?.name}
            onDecide={(decision, remember, note) =>
              App.DecideApproval(a.id, wire.DecideApprovalRequest.createFrom({ decision, remember, payload: note ? { note } : undefined })).then(live.reload, d.fail)
            }
          />
            ))}
          </div>
        )}
        {chat?.error && !busy && (
          <p className="flex items-start gap-2 rounded-lg border border-danger/30 bg-danger/[0.06] px-3 py-2 text-[12px] text-ink-muted" role="alert">
            <CircleAlert size={14} className="mt-px shrink-0 text-danger" aria-hidden />
            <span className="min-w-0">{chat.error} Your next message starts her again.</span>
          </p>
        )}
        {voice.panel}
        <div className="flex flex-wrap items-center gap-1.5">
          <Chip color={project?.color} label={project ? project.name : "Everywhere"} title={project ? `She works in ${project.folder || "no folder yet"}` : "This chat belongs to no project"} />
          {focus && !dropFocus && <Chip icon={active.kind === "term" ? SquareTerminal : FileText} label={focus} title="She sees what you have open" onRemove={() => setDropFocus(true)} />}
        </div>
        <form
          className={cx("flex flex-col rounded-[10px] border bg-surface-2 transition-colors focus-within:border-accent", voice.listening ? "border-accent/60" : "border-line-strong hover:border-line-3")}
          onSubmit={(e) => {
            e.preventDefault();
            send(text);
          }}
        >
          <textarea
            ref={input}
            rows={2}
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                send(text);
              }
            }}
            placeholder={busy ? "She's working: what you send next waits its turn" : project ? `Ask ${name} to do something in ${project.name}` : `Tell ${name} what you need`}
            aria-label={`Message to ${name}`}
            maxLength={32000}
            readOnly={voice.listening}
            className="max-h-48 min-h-[2.75rem] w-full resize-none bg-transparent px-3 pt-2.5 text-[13.5px] leading-5 text-ink placeholder:text-ink-faint focus:outline-none"
          />
          <div className="flex items-center gap-1 px-1.5 pb-1.5">
            <span className="min-w-0 flex-1 truncate pl-1.5 text-[11px] text-ink-faint">{voice.listening ? "Words appear as you talk · Enter sends · Esc cancels" : "Enter sends · Shift+Enter for a new line"}</span>
            {voice.button}
            <IconButton type="submit" size="sm" tone="primary" icon={SendHorizontal} label="Send" disabled={!text.trim() || !!sending} />
          </div>
        </form>
      </div>
    </div>
  );
}

/** Under the chat's name: what she is doing. */
function StateLine({ state, name, queued, pending, where }: { state: string; name: string; queued: number; pending: number; where?: string }) {
  let body: ReactNode;
  if (pending > 0 || state === "waiting")
    body = (
      <span className="text-warning">
        Waiting for your OK{pending > 1 ? ` (${pending})` : ""}
      </span>
    );
  else if (state === "starting") body = <>Starting Claude Code…</>;
  else if (state === "thinking") body = <>{name} is working</>;
  else if (state === "error") body = <span className="text-danger">Stopped with an error</span>;
  else body = <>{where ? `Works in ${where}` : "For everywhere"}</>;
  return (
    <div className="flex min-w-0 items-center gap-1.5 text-[11.5px] text-ink-subtle">
      {busyState(state) && pending === 0 && <LoaderCircle size={11} className="shrink-0 animate-spin" aria-hidden />}
      <span className="truncate">{body}</span>
      {queued > 0 && <span className="shrink-0 text-ink-faint">· {queued} waiting</span>}
    </div>
  );
}

function Chip({ label, color, icon: Icon, title, onRemove }: { label: string; color?: string; icon?: LucideIcon; title?: string; onRemove?: () => void }) {
  return (
    <span title={title} className="inline-flex h-6 max-w-[14rem] min-w-0 items-center gap-1.5 rounded-md border border-line bg-surface-1 pr-1 pl-2 text-[11.5px] text-ink-muted">
      {color ? <span className="size-2 shrink-0 rounded-[2px]" style={{ backgroundColor: color }} aria-hidden /> : Icon ? <Icon size={12} className="shrink-0" aria-hidden /> : null}
      <span className="min-w-0 truncate">{label}</span>
      {onRemove ? (
        <button type="button" onClick={onRemove} aria-label={`Leave ${label} out of the next message`} className="grid size-4 shrink-0 place-items-center rounded text-ink-faint hover:bg-surface-3 hover:text-ink">
          <X size={11} aria-hidden />
        </button>
      ) : (
        <span className="w-1" />
      )}
    </span>
  );
}

function EmptyChat({ name, project, starters, onPick, disabled }: { name: string; project?: string; starters: string[]; onPick: (s: string) => void; disabled: boolean }) {
  return (
    <div className="my-auto flex flex-col items-center gap-4 px-2 py-6 text-center">
      <p className="max-w-sm text-[12.5px] leading-relaxed text-ink-subtle">
        {project
          ? `Tell ${name} what you want done in ${project}. She looks at the code, plans it with you, and hands the work to Claude sessions she follows: you see each job here and hear when it's done.`
          : `Ask ${name} anything about your day, tasks and projects. She changes them for you, and can get work going in any project.`}
      </p>
      <div className="flex w-full max-w-sm flex-col gap-1.5">
        {starters.map((s) => (
          <button
            key={s}
            type="button"
            disabled={disabled}
            onClick={() => onPick(s)}
            className="rounded-lg border border-line bg-surface-1 px-3 py-2 text-left text-[12.5px] text-ink-muted transition-colors hover:border-line-strong hover:text-ink disabled:opacity-50"
          >
            {s}
          </button>
        ))}
      </div>
    </div>
  );
}

/** The conversation in time order: your messages, what she did, the news of the jobs she runs, her replies, and her
 * reply so far while she writes it. */
function Timeline({
  detail,
  partial,
  sending,
  name,
  folder,
  projectId,
  onUndo,
}: {
  detail: wire.AgentChatDetail;
  partial: string;
  sending: string | null;
  name: string;
  folder?: string;
  projectId?: string;
  onUndo: (s: wire.AgentStep) => void;
}) {
  const { messages, steps } = detail;
  // Messages and steps, merged by time; steps in a row form one block.
  const items = useMemo(() => {
    type Item = { at: number; message?: wire.AssistantMessage; index?: number; step?: wire.AgentStep };
    const all: Item[] = [...messages.map((m, i) => ({ at: m.at ?? 0, message: m, index: i })), ...steps.map((st) => ({ at: st.started_at, step: st }))];
    all.sort((a, b) => a.at - b.at || (a.message ? -1 : 1));
    const out: ({ kind: "message"; m: wire.AssistantMessage; key: string } | { kind: "steps"; steps: wire.AgentStep[]; key: string })[] = [];
    for (const it of all) {
      if (it.message) out.push({ kind: "message", m: it.message, key: it.message.id ?? `m${it.index}` });
      else {
        const last = out[out.length - 1];
        if (last?.kind === "steps" && !isEvent(it.step!) && !isEvent(last.steps[last.steps.length - 1])) last.steps.push(it.step!);
        else out.push({ kind: "steps", steps: [it.step!], key: it.step!.id });
      }
    }
    return out;
  }, [messages, steps]);
  const busy = busyState(detail.chat.state);
  // The latest news of each job, for a card whose session the window no longer lists.
  const news = useMemo(() => {
    const m = new Map<string, string>();
    for (const st of steps) if (isEvent(st)) m.set(field(st, "session_id"), field(st, "kind"));
    return m;
  }, [steps]);
  // Her face beside a reply that follows something else; a run of replies shares one.
  const follows = (i: number) => {
    const prev = items[i - 1];
    return prev?.kind === "message" && prev.m.role !== "user";
  };
  return (
    <>
      {items.map((it, i) =>
        it.kind === "message" ? (
          it.m.role === "user" ? (
            <UserBubble key={it.key} text={it.m.text} at={it.m.at} />
          ) : (
            <Reply key={it.key} text={it.m.text} at={it.m.at} name={name} avatar={!follows(i)} />
          )
        ) : (
          <Steps key={it.key} steps={it.steps} folder={folder} projectId={projectId} onUndo={onUndo} news={news} />
        ),
      )}
      {busy && <Working partial={partial} name={name} />}
      {sending && <UserBubble text={sending} pending />}
      {detail.queue.map((q, i) => (
        <UserBubble key={`q${i}`} text={q} queued />
      ))}
    </>
  );
}

function UserBubble({ text, at, pending, queued }: { text: string; at?: number; pending?: boolean; queued?: boolean }) {
  return (
    <div className="flex max-w-[88%] flex-col items-end gap-0.5 self-end">
      <div className={cx("rounded-xl rounded-tr-sm bg-accent/15 px-3 py-2 text-[13px] leading-relaxed whitespace-pre-wrap text-ink", (pending || queued) && "opacity-60")}>{text}</div>
      {queued ? <span className="px-1 text-[10.5px] text-ink-faint">Waits for her to finish</span> : at ? <span className="px-1 text-[10.5px] text-ink-faint tabular-nums">{formatTime(at)}</span> : null}
    </div>
  );
}

function Reply({ text, at, name, avatar }: { text: string; at?: number; name: string; avatar?: boolean }) {
  return (
    <div className="flex min-w-0 gap-2.5">
      <span className="mt-0.5 w-6 shrink-0">{avatar && <Face mood="smiling" className="size-6" />}</span>
      <div className="agent-md flex min-w-0 flex-1 flex-col gap-0.5">
        <Markdown text={text} />
        {at ? (
          <span className="text-[10.5px] text-ink-faint tabular-nums">
            {name} · {formatTime(at)}
          </span>
        ) : null}
      </div>
    </div>
  );
}

/** She is on it: her reply so far, or a quiet line until she writes. */
function Working({ partial, name }: { partial: string; name: string }) {
  if (partial.trim())
    return (
      <div className="agent-md min-w-0">
        <Markdown text={partial} />
        <span className="ml-0.5 inline-block h-3 w-1.5 animate-live rounded-[1px] bg-ink-subtle align-middle motion-reduce:animate-none" aria-hidden />
      </div>
    );
  return (
    <p className="flex items-center gap-2 text-[12px] text-ink-subtle" role="status">
      <LoaderCircle size={12} className="animate-spin" aria-hidden />
      {name} is thinking
    </p>
  );
}

const readLike = (s: wire.AgentStep) => (s.capability === "fs.read" || s.capability === "gwen.read" || s.capability === "app.read") && s.status === "done";

/** A turn's tool calls: reads folded into one line, everything else one row each. */
function Steps({ steps, folder, projectId, onUndo, news }: { steps: wire.AgentStep[]; folder?: string; projectId?: string; onUndo: (s: wire.AgentStep) => void; news?: Map<string, string> }) {
  const groups: (wire.AgentStep | wire.AgentStep[])[] = [];
  for (const s of steps) {
    const last = groups[groups.length - 1];
    if (readLike(s) && Array.isArray(last)) last.push(s);
    else groups.push(readLike(s) ? [s] : s);
  }
  return (
    <ul className="flex flex-col gap-0.5 border-l border-line pl-2.5" aria-label="What she did">
      {groups.map((g) => (Array.isArray(g) ? <ReadGroup key={g[0].id} steps={g} /> : <StepRow key={g.id} step={g} folder={folder} projectId={projectId} onUndo={onUndo} news={news} />))}
    </ul>
  );
}

function ReadGroup({ steps }: { steps: wire.AgentStep[] }) {
  const [open, setOpen] = useState(false);
  if (steps.length === 1) return <StepRow step={steps[0]} />;
  const files = steps.filter((s) => s.capability === "fs.read").length;
  const n = (k: number, one: string, many: string) => `${k} ${k === 1 ? one : many}`;
  const label = files === steps.length ? `Read ${n(files, "file", "files")}` : files === 0 ? `Looked at ${n(steps.length, "thing", "things")}` : `Read ${n(files, "file", "files")} and looked at ${n(steps.length - files, "thing", "things")}`;
  return (
    <li>
      <button type="button" onClick={() => setOpen((o) => !o)} aria-expanded={open} className="flex items-center gap-1.5 py-0.5 text-[12px] text-ink-subtle hover:text-ink">
        {open ? <ChevronDown size={12} aria-hidden /> : <ChevronRight size={12} aria-hidden />}
        <FileText size={12} aria-hidden />
        {label}
      </button>
      {open && (
        <ul className="ml-4 flex flex-col">
          {steps.map((s) => (
            <StepRow key={s.id} step={s} />
          ))}
        </ul>
      )}
    </li>
  );
}

const toolIcon = (s: wire.AgentStep): LucideIcon => {
  const t = s.tool;
  if (t === "Bash" || t === "BashOutput") return SquareTerminal;
  if (t === "Edit" || t === "MultiEdit" || t === "Write" || t === "NotebookEdit") return Pencil;
  if (t === "WebFetch" || t === "WebSearch") return Globe;
  if (t === "Task" || t === "Agent") return Bot;
  if (t.startsWith("mcp__gwen__")) return ListChecks;
  return FileText;
};

const input = (s: wire.AgentStep) => (s.input ?? {}) as Record<string, unknown>;

/** One tool call: what it was, how it went, its output when there is some to see, and Undo for Gwen's own changes. */
function StepRow({ step: s, folder, projectId, onUndo, news }: { step: wire.AgentStep; folder?: string; projectId?: string; onUndo?: (s: wire.AgentStep) => void; news?: Map<string, string> }) {
  const tabs = useTabs();
  const [open, setOpen] = useState(false);
  if (s.tool === "mcp__gwen__session_start") return <JobCard step={s} projectId={projectId} latest={news?.get(sessionOf(s) ?? "")} />;
  if (isEvent(s)) return <JobEvent step={s} projectId={projectId} />;
  const Icon = toolIcon(s);
  const failed = s.status === "failed" || s.status === "denied";
  const showOutput = !!s.output && (s.tool === "Bash" || failed);
  const path = typeof input(s).file_path === "string" ? (input(s).file_path as string) : "";
  const rel = folder && path.startsWith(folder + "/") ? path.slice(folder.length + 1) : "";
  const gwen = s.tool.startsWith("mcp__gwen__");
  return (
    <li className="min-w-0">
      <div className="group flex min-w-0 items-start gap-1.5 py-0.5 text-[12px]">
        <StatusMark status={s.status} />
        <Icon size={12} className="mt-[3px] shrink-0 text-ink-subtle" aria-hidden />
        <button
          type="button"
          onClick={() => (showOutput ? setOpen((o) => !o) : rel && projectId ? tabs.open({ kind: "file", project: projectId, path: rel }) : undefined)}
          className={cx("min-w-0 text-left break-words", failed ? "text-danger" : "text-ink-muted", (showOutput || (rel && projectId)) && "hover:text-ink")}
          title={rel ? "Open the file" : undefined}
        >
          {gwen && s.output && s.status === "done" ? stripMark(s.output) : s.title}
          {failed && s.output && !showOutput && <span className="text-ink-subtle"> · {s.output}</span>}
        </button>
        {s.can_undo && onUndo && (
          <button type="button" onClick={() => onUndo(s)} className="ml-auto inline-flex shrink-0 items-center gap-1 rounded px-1 text-[11px] text-ink-subtle hover:bg-surface-2 hover:text-ink">
            <Undo2 size={11} aria-hidden />
            Undo
          </button>
        )}
      </div>
      {showOutput && open && <pre className="my-1 ml-5 max-h-48 overflow-auto rounded-md border border-line bg-canvas/60 px-2 py-1.5 font-mono text-[11px] leading-[1.5] whitespace-pre-wrap text-ink-muted">{s.output}</pre>}
    </li>
  );
}

function StatusMark({ status }: { status: string }) {
  switch (status) {
    case "running":
      return <LoaderCircle size={12} className="mt-[3px] shrink-0 animate-spin text-info" aria-label="Running" />;
    case "waiting":
      return <Clock size={12} className="mt-[3px] shrink-0 text-warning" aria-label="Waiting for your OK" />;
    case "done":
      return <Check size={12} strokeWidth={2.6} className="mt-[3px] shrink-0 text-success" aria-label="Done" />;
    case "denied":
      return <CircleX size={12} className="mt-[3px] shrink-0 text-danger" aria-label="Not allowed" />;
    case "failed":
      return <CircleX size={12} className="mt-[3px] shrink-0 text-danger" aria-label="Failed" />;
  }
  return <span className="mt-[3px] size-3 shrink-0 rounded-full border border-line-3" aria-label="Stopped" />;
}

// A job is a Claude session she started (mcp__gwen__session_start). Its step's output carries the session's id as
// "[session:<id>]", which the daemon writes so a card always finds its own session.
const sessionOf = (s: wire.AgentStep) => /\[session:([^\]]+)\]/.exec(s.output ?? "")?.[1];
const stripMark = (t: string) => t.replace(/\s*\[session:[^\]]+\]/, "");
const isEvent = (s: wire.AgentStep) => s.tool === "gwen:session";
const field = (s: wire.AgentStep, k: string) => (typeof input(s)[k] === "string" ? (input(s)[k] as string) : "");

/** A job's live state in a few words, and its tone: from its session, else from the chat's latest news of it. */
function jobState(session: wire.CodeSession | undefined, latest?: string): { label: string; tone: "info" | "warning" | "success" | "muted"; live: boolean } {
  if (!session && latest === "finished") return { label: "Finished", tone: "success", live: false };
  if (!session && latest === "ended") return { label: "Ended", tone: "muted", live: false };
  switch (session?.state) {
    case "starting":
      return { label: "Starting", tone: "info", live: true };
    case "working":
      return { label: "Working", tone: "info", live: true };
    case "needs_you":
      return { label: "Needs you", tone: "warning", live: true };
    case "done":
    case "idle":
      return { label: "Finished", tone: "success", live: false };
    case "error":
      return { label: "Stopped with an error", tone: "muted", live: false };
    case "ended":
      return { label: "Ended", tone: "muted", live: false };
  }
  return { label: "Starting", tone: "info", live: true };
}

const toneText = { info: "text-info", warning: "text-warning", success: "text-success", muted: "text-ink-subtle" };

function elapsed(from: number, now: number) {
  const m = Math.max(0, Math.round((now - from) / 60000));
  return m < 1 ? "just now" : m < 60 ? `${m} min` : `${Math.floor(m / 60)} h ${m % 60} min`;
}

/** A job she handed to Claude: its title and task, what Claude is doing now, and Open. */
function JobCard({ step: s, projectId, latest }: { step: wire.AgentStep; projectId?: string; latest?: string }) {
  const w = useWork();
  const tabs = useTabs();
  const d = useDaemon();
  const [more, setMore] = useState(false);
  const id = sessionOf(s);
  const session = id ? w.sessions.find((x) => x.id === id) : undefined;
  const title = field(s, "title") || "A job for Claude";
  const brief = field(s, "brief");
  const taskId = field(s, "task_id");
  const task = taskId ? d.tasks.find((t) => t.id === taskId || t.id.startsWith(taskId)) : undefined;
  const failed = s.status === "failed" || s.status === "denied";
  const asking = s.status === "waiting";
  const st = jobState(session, latest);
  const now = d.now();
  const doing = session?.state === "needs_you" ? session.waiting?.summary || "an answer from you" : session?.step || session?.last_message || "";
  return (
    <li className={cx("my-1 flex flex-col gap-2 rounded-xl border px-3.5 py-3", failed ? "border-danger/30 bg-danger/[0.04]" : st.tone === "warning" ? "border-warning/40 bg-warning/[0.05]" : "border-line bg-surface-1")}>
      <div className="flex min-w-0 items-center gap-2">
        <span className="grid size-6 shrink-0 place-items-center rounded-md bg-[#D97757]/15">
          <ClaudeMark size={13} />
        </span>
        <div className="min-w-0 flex-1">
          <div className="truncate text-[13px] font-semibold text-ink">{title}</div>
          <div className="truncate text-[11.5px] text-ink-subtle">{task ? `For “${task.title}”` : "Handed to Claude"}</div>
        </div>
        {session?.terminal && session.state !== "ended" && (
          <Button size="sm" icon={SquareTerminal} onClick={() => tabs.open({ kind: "term", project: projectId ?? "", terminal: session.terminal })}>
            {session.state === "needs_you" ? "Answer" : "Open"}
          </Button>
        )}
      </div>
      {failed ? (
        <p className="text-[12px] text-danger">{stripMark(s.output) || "It didn't start."}</p>
      ) : asking ? (
        <p className="text-[12px] text-warning">Waits for your OK to start</p>
      ) : (
        <div className="flex min-w-0 items-center gap-2 text-[12px]">
          <span className={cx("inline-flex shrink-0 items-center gap-1.5 font-medium", toneText[st.tone])}>
            {st.live ? <span className={cx("size-1.5 rounded-full bg-current", st.tone !== "warning" && "animate-live motion-reduce:animate-none")} /> : <Check size={12} strokeWidth={2.6} aria-hidden />}
            {st.label}
          </span>
          {doing && <span className="min-w-0 truncate text-ink-muted">{doing}</span>}
          {session && <span className="ml-auto shrink-0 text-[11px] text-ink-faint tabular-nums">{elapsed(session.started_at, now)}</span>}
        </div>
      )}
      {brief && (
        <button type="button" onClick={() => setMore((m) => !m)} className="text-left text-[11.5px] leading-relaxed text-ink-subtle hover:text-ink-muted">
          <span className={cx(!more && "line-clamp-2")}>{brief}</span>
        </button>
      )}
    </li>
  );
}

/** News of a job: Claude finished, needs you, or stopped. */
function JobEvent({ step: s, projectId }: { step: wire.AgentStep; projectId?: string }) {
  const w = useWork();
  const tabs = useTabs();
  const [open, setOpen] = useState(false);
  const id = field(s, "session_id");
  const session = id ? w.sessions.find((x) => x.id === id) : undefined;
  const kind = field(s, "kind");
  // A question that was answered since is history: quiet, and its details folded.
  const waiting = kind === "needs" && session?.state === "needs_you";
  const tone = waiting ? "text-warning" : kind === "finished" ? "text-success" : "text-ink-subtle";
  return (
    <li className="my-0.5 flex min-w-0 flex-col">
      <div className="flex min-w-0 items-center gap-2 text-[12px]">
        <ClaudeMark size={12} />
        <button type="button" onClick={() => s.output && setOpen((o) => !o)} className={cx("min-w-0 truncate text-left font-medium", tone)}>
          {s.title}
        </button>
        {waiting && session?.terminal && (
          <Button size="sm" tone="ghost" onClick={() => tabs.open({ kind: "term", project: projectId ?? "", terminal: session.terminal })}>
            Answer
          </Button>
        )}
      </div>
      {s.output && (open || waiting) && <p className="mt-0.5 ml-5 text-[11.5px] leading-relaxed whitespace-pre-wrap text-ink-subtle">{s.output}</p>}
    </li>
  );
}

/** The jobs of this chat that are still going, always in view under the header. */
function JobsBar({ steps, projectId }: { steps: wire.AgentStep[]; projectId?: string }) {
  const w = useWork();
  const tabs = useTabs();
  const jobs = steps
    .filter((s) => s.tool === "mcp__gwen__session_start" && sessionOf(s))
    .map((s) => ({ s, session: w.sessions.find((x) => x.id === sessionOf(s)) }))
    .filter((j) => j.session && ["starting", "working", "needs_you"].includes(j.session.state));
  if (jobs.length === 0) return null;
  return (
    <div className="flex shrink-0 flex-wrap items-center gap-1.5 border-b border-chrome-line bg-surface-1/60 px-3 py-2">
      <span className="mr-0.5 text-[11px] font-semibold tracking-[0.06em] text-ink-subtle uppercase">Jobs</span>
      {jobs.map(({ s, session }) => {
        const st = jobState(session);
        return (
          <button
            key={s.id}
            type="button"
            onClick={() => session?.terminal && tabs.open({ kind: "term", project: projectId ?? "", terminal: session.terminal })}
            title={session?.step || session?.last_message || ""}
            className={cx("inline-flex h-6 max-w-[16rem] min-w-0 items-center gap-1.5 rounded-md border bg-surface-1 px-2 text-[11.5px] hover:border-line-3", st.tone === "warning" ? "border-warning/50 text-warning" : "border-line text-ink-muted")}
          >
            <span className={cx("size-1.5 shrink-0 rounded-full bg-current", st.tone === "info" && "animate-live text-info motion-reduce:animate-none")} />
            <span className="min-w-0 truncate">{field(s, "title") || "Claude job"}</span>
            {st.tone === "warning" && <span className="shrink-0 font-medium">needs you</span>}
          </button>
        );
      })}
    </div>
  );
}
