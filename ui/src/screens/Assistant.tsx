import { History, LoaderCircle, MessageSquarePlus, Plus, Trash2 } from "lucide-react";
import { useCallback, useEffect, useLayoutEffect, useRef, useState, type CSSProperties, type RefObject } from "react";
import AgentChat from "../agent/AgentChat";
import { busyState, lastChat, rememberChat, useAgentChats, useAgentStatus } from "../agent/agent";
import { App, wire } from "../api";
import AssistantChat, { type ChatControl, type ChatState } from "../components/AssistantChat";
import { Button, IconButton, cx } from "../components/ui";
import { useDaemon } from "../daemon";
import { addDays, formatDate, formatDay, formatTime } from "../format";
import { threadOf } from "../memory";
import { useNav } from "../nav";
import WithGwen from "./AssistantSide";

/** How many past chats are listed at a time. */
const pastPage = 50;

/**
 * Gwen › Chat (plan.md 3.15, board 12): the past chats on the left; the chat, with her face and attitude on top,
 * what she changed and remembered under each reply, starters, read aloud, and Talk; and beside it what else she
 * does with you: the AI she runs on, planning the day, breaking a goal into tasks, the weekly retro, and the
 * sessions she watches. Another screen can open the chat with a message ready to send, or open one of the chats.
 */
export default function Assistant() {
  const status = useAgentStatus();
  if (!status) return null;
  return status.available ? <AgentScreen /> : <PlanningChat />;
}

/** The chat of Gwen's agent named by a param "agent:<id>", from Needs you and other screens. */
function agentChatOf(param: string | null): string | null {
  return param?.startsWith("agent:") ? param.slice("agent:".length).split("#")[0] || null : null;
}

/**
 * Gwen › Chat with her agent (plan.md 3.16): every chat on the left, the chats for everywhere first and then each
 * project's, with a spinner while she works and a dot while she waits for you; the chat on the right; and on a wide
 * window what else she does with you.
 */
function AgentScreen() {
  const d = useDaemon();
  const { param } = useNav();
  const { chats, reload } = useAgentChats("all");
  const [selected, setSelected] = useState<string | null>(() => agentChatOf(param) ?? lastChat("all"));
  // Where a new chat goes until its first message makes it.
  const [newPlace, setNewPlace] = useState("everywhere");
  useEffect(() => {
    const id = agentChatOf(param);
    if (id) setSelected(id);
  }, [param]);
  const [showList, setShowList] = useState(false);
  const [page, height] = useFill();
  const current = chats.find((c) => c.id === selected);
  const place = current ? (current.project_id ?? "everywhere") : newPlace;
  const choose = (id: string | null) => {
    setSelected(id);
    rememberChat("all", id);
    setShowList(false);
    reload();
  };
  const start = (where: string) => {
    setNewPlace(where);
    choose(null);
  };
  const groups = [
    { id: "everywhere", name: "Everywhere", color: "", chats: chats.filter((c) => !c.project_id) },
    ...d.projects
      .map((p) => ({ id: p.id, name: p.name, color: p.color, chats: chats.filter((c) => c.project_id === p.id) }))
      .filter((g) => g.chats.length > 0 || g.id === newPlace),
  ];
  const now = d.now();

  return (
    <div ref={page} className="grid items-start gap-5 @[90rem]:grid-cols-[minmax(0,1fr)_18.5rem]" style={height ? ({ "--fill": `${height}px` } as CSSProperties) : undefined}>
      <section aria-label="Chat" className="lift relative flex h-[calc(var(--fill)-4rem)] min-h-[30rem] min-w-0 overflow-hidden rounded-xl border border-line bg-surface-1 @[90rem]:h-(--fill)">
        <nav
          aria-label="Chats"
          className={cx(
            showList ? "flex" : "hidden",
            "absolute inset-y-0 left-0 z-10 w-64 shrink-0 flex-col border-r border-chrome-line bg-surface-1 @3xl:static @3xl:z-auto @3xl:flex",
          )}
        >
          <div className="flex h-12 shrink-0 items-center gap-2 border-b border-chrome-line pr-2 pl-3.5">
            <span className="flex-1 text-[13px] font-semibold">Chats</span>
            <Button size="sm" tone="ghost" icon={MessageSquarePlus} onClick={() => start("everywhere")}>
              New
            </Button>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto py-1.5">
            {groups.map((g) => (
              <div key={g.id} className="pb-1.5">
                <div className="group flex items-center gap-2 px-3.5 pt-2 pb-1">
                  {g.color ? <span className="size-2 shrink-0 rounded-[2px]" style={{ backgroundColor: g.color }} aria-hidden /> : <span className="size-2 shrink-0 rounded-[2px] border border-line-3" aria-hidden />}
                  <span className="min-w-0 flex-1 truncate text-[11px] font-semibold tracking-[.04em] text-ink-subtle uppercase">{g.name}</span>
                  <IconButton size="sm" icon={Plus} label={`New chat in ${g.name}`} onClick={() => start(g.id)} className="opacity-0 group-hover:opacity-100 focus-visible:opacity-100" />
                </div>
                {g.chats.length === 0 && g.id === "everywhere" && <p className="px-3.5 pb-1 text-[11.5px] text-ink-faint">No chats yet</p>}
                <ul className="flex flex-col gap-px px-1.5">
                  {g.chats.map((c) => (
                    <li key={c.id}>
                      <button
                        type="button"
                        onClick={() => choose(c.id)}
                        aria-current={c.id === selected || undefined}
                        className={cx("flex w-full min-w-0 items-center gap-2 rounded-md px-2 py-1.5 text-left", c.id === selected ? "bg-surface-2 text-ink" : "text-ink-muted hover:bg-surface-2/60 hover:text-ink")}
                      >
                        <span className="min-w-0 flex-1 truncate text-[12.5px]">{c.title || "New chat"}</span>
                        {c.pending > 0 ? (
                          <span className="size-[7px] shrink-0 rounded-full bg-warning" title="Waits for your OK" />
                        ) : busyState(c.state) ? (
                          <LoaderCircle size={11} className="shrink-0 animate-spin text-info" aria-label="Working" />
                        ) : (
                          <span className="shrink-0 text-[10.5px] text-ink-faint tabular-nums">{lastWhen(c.last_at, now)}</span>
                        )}
                      </button>
                    </li>
                  ))}
                  {!selected && g.id === newPlace && (
                    <li className="rounded-md bg-surface-2 px-2 py-1.5 text-[12.5px] text-ink">New chat</li>
                  )}
                </ul>
              </div>
            ))}
          </div>
        </nav>
        <div className="min-w-0 flex-1">
          <AgentChat
            key={selected ?? `new:${newPlace}`}
            screen
            place={place}
            chatId={selected}
            onChat={choose}
            headerStart={<IconButton icon={History} label={showList ? "Hide the chats" : "Chats"} active={showList} onClick={() => setShowList((v) => !v)} className="-ml-1.5 @3xl:hidden" />}
          />
        </div>
      </section>
      <WithGwen className="hidden @[90rem]:block @[90rem]:max-h-(--fill) @[90rem]:overflow-y-auto" />
    </div>
  );
}

/** Gwen › Chat with the planning assistant, when her agent can't run here. */
function PlanningChat() {
  const { param } = useNav();
  const [initial, setInitial] = useState<string | null>(threadOf(param) ? null : param);
  const [thread, setThread] = useState<string | null>(threadOf(param));
  const handled = useRef(param);
  useEffect(() => {
    if (param === handled.current) return;
    handled.current = param;
    const t = threadOf(param);
    if (t) setThread(t);
    else if (param) setInitial(param);
  }, [param]);

  const control = useRef<ChatControl | null>(null);
  const [chat, setChat] = useState<ChatState>({ thread: null, messages: 0, busy: false });
  // On a narrow window the past chats open over the chat, from a button in its header.
  const [showPast, setShowPast] = useState(false);
  const [page, height] = useFill();

  return (
    <div ref={page} className="grid items-start gap-5 @5xl:grid-cols-[minmax(0,1fr)_18.5rem]" style={height ? ({ "--fill": `${height}px` } as CSSProperties) : undefined}>
      {/* On a narrow window the cards go under the chat, which stops short so their headers show. */}
      <section aria-label="Chat" className="lift relative flex h-[calc(var(--fill)-4rem)] min-h-[30rem] min-w-0 overflow-hidden rounded-xl border border-line bg-surface-1 @5xl:h-(--fill)">
        <PastChats
          chat={chat}
          control={control}
          onDone={() => setShowPast(false)}
          className={cx(showPast ? "flex" : "hidden", "absolute inset-y-0 left-0 z-10 @3xl:static @3xl:z-auto @3xl:flex")}
        />
        <div className="min-w-0 flex-1">
          <AssistantChat
            screen
            control={control}
            onState={setChat}
            initial={initial}
            onInitialUsed={() => setInitial(null)}
            openThread={thread}
            headerExtra={<IconButton icon={History} label={showPast ? "Hide past chats" : "Past chats"} active={showPast} onClick={() => setShowPast((v) => !v)} className="-ml-1.5 @3xl:hidden" />}
          />
        </div>
      </section>
      <WithGwen className="@5xl:max-h-(--fill) @5xl:overflow-y-auto" />
    </div>
  );
}

/**
 * The height left for the chat in the scrolling page it sits in, so it fills the window and scrolls inside
 * rather than the page; the page scrolls only for what sits below it on a narrow window.
 */
function useFill() {
  const ref = useRef<HTMLDivElement>(null);
  const [height, setHeight] = useState<number | null>(null);
  useLayoutEffect(() => {
    const el = ref.current;
    let page = el?.parentElement ?? null;
    while (page && !/(auto|scroll)/.test(getComputedStyle(page).overflowY)) page = page.parentElement;
    if (!el || !page) return;
    const scroller = page;
    const measure = () => {
      const top = el.getBoundingClientRect().top - scroller.getBoundingClientRect().top + scroller.scrollTop;
      const bottom = parseFloat(getComputedStyle(el.parentElement!).paddingBottom) || 0;
      setHeight(Math.floor(scroller.clientHeight - top - bottom));
    };
    measure();
    const watch = new ResizeObserver(measure);
    watch.observe(scroller);
    return () => watch.disconnect();
  }, []);
  return [ref, height] as const;
}

/** When a chat last moved: "now", "2:31 pm" today, "yesterday", then its date. */
function lastWhen(at: number, now: number): string {
  if (now - at < 120_000) return "now";
  const today = formatDay(new Date(now));
  const day = formatDay(new Date(at));
  if (day === today) return formatTime(at);
  if (day === addDays(today, -1)) return "yesterday";
  return formatDate(day, false);
}

/** Past chats: the most recent first, the one on screen marked. Open one, start a new one, or delete one. */
function PastChats({ chat, control, onDone, className }: { chat: ChatState; control: RefObject<ChatControl | null>; onDone: () => void; className?: string }) {
  const d = useDaemon();
  const name = d.config?.llm.assistant_name || "Gwen";
  const [list, setList] = useState<{ threads: wire.ChatThread[]; more: boolean } | null>(null);
  const [failed, setFailed] = useState(false);
  const [older, setOlder] = useState(false);
  // How many are listed, so the list read again after a reply keeps the older ones shown.
  const count = useRef(pastPage);

  const load = useCallback(() => {
    let live = true;
    App.ListThreads(count.current, 0).then(
      ({ threads }) => {
        if (!live) return;
        setList({ threads, more: threads.length === count.current });
        setFailed(false);
      },
      () => live && setFailed(true),
    );
    return () => {
      live = false;
    };
  }, []);
  // The list follows the chat on screen: a new chat joins it, and each reply moves its chat to the top.
  useEffect(load, [chat.thread, chat.messages]);

  async function showOlder() {
    if (!list) return;
    setOlder(true);
    try {
      const { threads } = await App.ListThreads(pastPage, list.threads.length);
      count.current = list.threads.length + pastPage;
      setList((p) => {
        if (!p) return p;
        const seen = new Set(p.threads.map((t) => t.id));
        return { threads: [...p.threads, ...threads.filter((t) => !seen.has(t.id))], more: threads.length === pastPage };
      });
    } catch (e) {
      d.fail(e);
    } finally {
      setOlder(false);
    }
  }

  async function forget(t: wire.ChatThread) {
    if (await control.current?.forget(t)) setList((p) => p && { ...p, threads: p.threads.filter((x) => x.id !== t.id) });
  }

  const now = Date.now();
  return (
    <nav aria-label="Past chats" className={cx("w-[236px] shrink-0 flex-col border-r border-chrome-line bg-surface-1", className)}>
      <div className="flex h-12 shrink-0 items-center gap-2 pr-2.5 pl-4">
        <h2 className="text-[12px] font-medium text-ink-subtle">Past chats</h2>
        <Button
          size="sm"
          icon={MessageSquarePlus}
          className="ml-auto"
          disabled={chat.busy || !chat.thread}
          onClick={() => {
            control.current?.startOver();
            onDone();
          }}
        >
          New chat
        </Button>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto px-2 pb-2">
        {list === null ? (
          failed ? (
            <div className="flex flex-col items-start gap-2 px-2 py-3 text-[12.5px] text-ink-subtle">
              Couldn't load your chats.
              <Button
                size="sm"
                onClick={() => {
                  setFailed(false);
                  load();
                }}
              >
                Try again
              </Button>
            </div>
          ) : (
            <p className="flex items-center gap-2 px-2 py-3 text-[12.5px] text-ink-subtle" role="status">
              <LoaderCircle size={13} className="animate-spin" aria-hidden />
              Loading your chats…
            </p>
          )
        ) : (
          <>
            <ul className="flex flex-col gap-0.5">
              {!chat.thread && (
                <li aria-current="true" className="rounded-lg bg-surface-2 px-3 py-2 ring-1 ring-line ring-inset">
                  <span className="block text-[13px] text-ink">New chat</span>
                  <span className="mt-0.5 block text-[11px] text-ink-subtle">Starts with what you say first</span>
                </li>
              )}
              {list.threads.map((t) => {
                const on = t.id === chat.thread;
                const title = t.title || "Untitled chat";
                return (
                  <li key={t.id} className="group relative">
                    <button
                      type="button"
                      title={title}
                      aria-current={on ? "true" : undefined}
                      disabled={chat.busy}
                      onClick={() => {
                        if (!on) control.current?.open(t.id);
                        onDone();
                      }}
                      className={cx(
                        "flex w-full min-w-0 flex-col rounded-lg py-2 pr-9 pl-3 text-left transition-colors disabled:cursor-not-allowed",
                        on ? "bg-surface-2 ring-1 ring-line ring-inset" : "hover:bg-surface-2/60 disabled:hover:bg-transparent",
                      )}
                    >
                      <span className={cx("w-full truncate text-[13px]", on ? "text-ink" : "text-ink-muted group-hover:text-ink")}>{title}</span>
                      <span className="mt-0.5 text-[11px] text-ink-subtle tabular-nums">
                        {lastWhen(t.last_at, now)} · {t.messages} {t.messages === 1 ? "message" : "messages"}
                      </span>
                    </button>
                    <IconButton
                      size="sm"
                      icon={Trash2}
                      label={`Delete "${title}"`}
                      disabled={chat.busy}
                      onClick={() => forget(t)}
                      className="absolute top-1/2 right-1 -translate-y-1/2 opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
                    />
                  </li>
                );
              })}
            </ul>
            {list.threads.length === 0 && <p className="px-3 py-3 text-[12.5px] leading-relaxed text-ink-subtle">No chats yet. What you say to {name} is kept here, and she remembers it in later chats.</p>}
            {list.more && (
              <Button size="sm" tone="ghost" className="mt-1 ml-1" onClick={showOlder} busy={older}>
                Show older chats
              </Button>
            )}
          </>
        )}
      </div>
      {!!list?.threads.length && (
        <p className="shrink-0 border-t border-chrome-line px-4 py-2.5 text-[11px] leading-snug text-ink-faint">{name} recalls these chats when they come up again. Deleting one erases it.</p>
      )}
    </nav>
  );
}
