import { CircleAlert, X } from "lucide-react";
import { useEffect, useRef, useState, type Ref } from "react";
import { App, apiError, wire } from "../api";
import { useDaemon } from "../daemon";
import { formatClock, formatDate, formatDuration, formatTime, relativeDay } from "../format";
import type { ChatMessage, DayPlanOutput, ProposedBlock, RunError } from "../llm";
import { AiFailed, AiProgress, AiUnavailable, providerName } from "./ai";
import Face from "./Face";
import { useToast } from "./feedback";
import { Badge, Button, Callout, Checkbox, IconButton, cx, unassignedColor } from "./ui";
import { useDictation } from "./Voice";

/** Messages to start from, sent with one click. */
const starters = ["Plan my day", "I'm tired today, keep it light", "Start at 2 pm and work 3 hours", "Most urgent work first, with breaks"];

/** The day's latest run id, so reopening the day restores its conversation. */
function remember(day: string, id: string | null) {
  try {
    if (id) localStorage.setItem(`gwen.planChat.${day}`, id);
    else localStorage.removeItem(`gwen.planChat.${day}`);
  } catch {
    // storage is a convenience
  }
}

function recall(day: string): string | null {
  try {
    return localStorage.getItem(`gwen.planChat.${day}`);
  } catch {
    return null;
  }
}

/** "from 2:00 pm · 3h for tasks" for a change of hours. */
export function hoursText(h: DayPlanOutput["hours"]): string {
  if (!h) return "";
  const parts: string[] = [];
  if (h.start_minute != null) parts.push(`from ${formatClock(h.start_minute)}`);
  if (h.work_minutes != null) parts.push(h.work_minutes === 0 ? "no time for tasks" : `${formatDuration(h.work_minutes * 60_000)} for tasks`);
  return parts.join(" · ");
}

/** The box's earlier text, then the words said. */
function after(before: string, said: string): string {
  if (!said) return before;
  return before.trim() ? `${before.trimEnd()} ${said}` : said;
}

/**
 * Plan › Day › Plan with Gwen (docs/08-clients.md#v3-additions): a conversation about one day that ends in a plan,
 * in a column beside the day. The latest proposal is listed with a checkbox per block and previewed on the plan lane
 * through onPreview; nothing changes until it is applied.
 */
export default function PlanChat({ day, plan, onPreview, onClose }: { day: string; plan: wire.Plan; onPreview: (blocks: ProposedBlock[] | null) => void; onClose: () => void }) {
  const d = useDaemon();
  const notify = useToast();
  const name = d.config?.llm.assistant_name || "Gwen";
  const [run, setRun] = useState<wire.LlmRun | null>(null); // the conversation's latest answer
  const [picked, setPicked] = useState<Set<number>>(new Set());
  const [text, setText] = useState("");
  const [asking, setAsking] = useState<{ message: string; since: number } | null>(null);
  const [failure, setFailure] = useState<string | null>(null);
  const [unavailable, setUnavailable] = useState<string | null>(null);
  const [refused, setRefused] = useState<string | null>(null);
  const [applying, setApplying] = useState(false);
  const scroller = useRef<HTMLDivElement>(null);
  const last = useRef<HTMLDivElement>(null);
  const input = useRef<HTMLTextAreaElement>(null);
  // What the box held when the mic opened; the words said follow it.
  const before = useRef("");
  const voice = useDictation(
    {
      onStart: () => {
        before.current = text;
        input.current?.focus();
      },
      onText: (said, final) => {
        setText(after(before.current, said));
        if (final) input.current?.focus();
      },
    },
    !!asking,
  );

  function adopt(r: wire.LlmRun) {
    setRun(r);
    setPicked(new Set((r.output as DayPlanOutput).items.map((_, i) => i)));
    remember(day, r.id);
  }

  useEffect(() => {
    let live = true;
    const id = recall(day);
    if (id)
      App.GetLLMRun(id).then(
        (r) => live && r && r.kind === "day_plan" && r.subject_id === day && (r.status === "ok" || r.status === "accepted") && adopt(r),
        () => remember(day, null),
      );
    return () => {
      live = false;
    };
  }, [day]);

  const out = run ? (run.output as DayPlanOutput) : null;
  const messages: ChatMessage[] = out?.messages ?? [];
  const blocks = out?.items ?? [];
  const open = run?.status === "ok"; // proposed, not yet applied

  useEffect(() => onPreview(open ? blocks.filter((_, i) => picked.has(i)) : null), [run, picked]);
  useEffect(() => () => onPreview(null), []);
  // A new turn scrolls to it: the newest message at the top, the proposal under it.
  useEffect(() => {
    const el = scroller.current;
    if (!el) return;
    if (asking || !last.current) el.scrollTop = el.scrollHeight;
    else el.scrollTop = Math.max(0, last.current.offsetTop - 12);
  }, [messages.length, asking]);

  async function send(message: string) {
    const m = message.trim();
    if (!m || asking) return;
    setAsking({ message: m, since: Date.now() });
    setText("");
    setFailure(null);
    setUnavailable(null);
    setRefused(null);
    try {
      const r = await App.PlanChat(wire.PlanChatRequest.createFrom({ day, message: m, run_id: run?.id ?? null }));
      if (r.status === "failed") {
        setFailure((r.output as RunError).error);
        setText(m);
      } else adopt(r);
    } catch (e) {
      const err = apiError(e);
      setText(m);
      if (err.code === "unavailable") setUnavailable(err.message);
      else if (err.code === "invalid_request") setRefused(err.message);
      else d.fail(e);
    } finally {
      setAsking(null);
    }
  }

  /** Sends the box, or while the mic is open, everything said once it stops. */
  async function submit() {
    if (!voice.listening) return send(text);
    const said = await voice.finish();
    if (said != null) send(after(before.current, said));
  }

  async function apply() {
    if (!run) return;
    setApplying(true);
    const done = await d.act(() => App.AcceptLLMRun(run.id, wire.AcceptRunRequest.createFrom({ indexes: [...picked].sort((a, b) => a - b) })));
    setApplying(false);
    if (!done) return;
    setRun(wire.LlmRun.createFrom({ ...run, status: "accepted" }));
    notify(picked.size ? `Planned ${picked.size} ${picked.size === 1 ? "block" : "blocks"} from the chat` : "Cleared the day's plan", "success");
  }

  /** Drops the proposal, if one is waiting, and the conversation with it. */
  async function startOver() {
    if (run?.status === "ok") await d.run(() => App.RejectLLMRun(run.id));
    remember(day, null);
    setRun(null);
    setPicked(new Set());
    setFailure(null);
    setRefused(null);
    setText("");
  }

  const toggle = (i: number) =>
    setPicked((p) => {
      const n = new Set(p);
      if (n.has(i)) n.delete(i);
      else n.add(i);
      return n;
    });
  const total = blocks.reduce((sum, b, i) => sum + (picked.has(i) ? b.planned_minutes : 0), 0);
  const room = out?.hours?.work_minutes ?? plan.capacity_minutes;
  const change = hoursText(out?.hours ?? null);
  const rel = relativeDay(day, d.today());
  const dayWord = rel === "today" || rel === "tomorrow" ? rel : formatDate(day);
  const prime = !!d.config?.planner.prime_start;
  const followUps = ["Most urgent first, with breaks", prime ? "Hardest work in my prime time" : "Hardest work first", "Keep it lighter", "Leave the evening free"];
  const color = (b: ProposedBlock) => d.projects.find((p) => p.id === b.project_id)?.color ?? unassignedColor;
  const dur = (m: number) => formatDuration(m * 60_000);

  return (
    <aside aria-label={`Plan with ${name}`} className="@container flex w-[clamp(320px,32%,380px)] shrink-0 flex-col border-l border-chrome-line bg-chrome">
      <header className="flex h-11 shrink-0 items-center gap-2 border-b border-chrome-line pr-2 pl-3.5">
        <Face mood={asking ? "thinking" : "smiling"} breathe={false} className="size-6" />
        <h2 className="shrink-0 text-[13px] font-semibold text-ink" title={`Answers come from ${providerName(d.config?.llm)}`}>
          Plan with {name}
        </h2>
        <span className="hidden min-w-0 truncate text-[11.5px] text-ink-subtle @[22rem]:inline">{providerName(d.config?.llm)}</span>
        <span className="ml-auto flex shrink-0 items-center gap-1">
          {(run || failure) && (
            <Button size="sm" onClick={startOver} disabled={!!asking} className="!h-[26px]" title="Start over. A proposal you haven't applied is dropped.">
              New chat
            </Button>
          )}
          <IconButton icon={X} label={`Close Plan with ${name}`} size="sm" onClick={onClose} />
        </span>
      </header>

      <div ref={scroller} className="relative min-h-0 flex-1 overflow-y-auto">
        <div className="flex flex-col gap-2.5 px-4 pt-3.5" aria-live="polite">
          {messages.length === 0 && !asking ? (
            // Her opening line: what to tell her, and that nothing changes until a plan is applied.
            <div className="max-w-[90%] self-start rounded-xl rounded-bl-[4px] bg-surface-2 px-3 py-2 text-[13px] leading-relaxed text-ink">
              <p className="font-medium">How should {dayWord} go?</p>
              <p className="mt-0.5 text-ink-muted">
                Tell me when you start, how long you have, what matters most, or how you feel. I'll plan from your open tasks, and nothing changes until you apply it.
              </p>
            </div>
          ) : (
            messages.map((m, i) => <Bubble key={i} ref={i === messages.length - 1 ? last : undefined} role={m.role} text={m.text} />)
          )}
          {asking && <Bubble role="user" text={asking.message} pending />}
        </div>

        {(asking || unavailable || refused || failure) && (
          <div className="flex flex-col gap-2.5 px-4 pt-3">
            {asking && <AiProgress since={asking.since} doing="Planning your day" />}
            {unavailable && <AiUnavailable message={unavailable} />}
            {refused && (
              <Callout
                tone="neutral"
                icon={CircleAlert}
                action={
                  <Button size="sm" onClick={startOver}>
                    New chat
                  </Button>
                }
              >
                {refused[0].toUpperCase() + refused.slice(1)}.
              </Callout>
            )}
            {failure && <AiFailed error={failure} />}
          </div>
        )}

        {run && (
          <section aria-label="Proposed plan" className="mx-4 mt-3.5 rounded-xl border border-line bg-surface-1 px-3 pt-2.5 pb-3">
            <div className="flex items-center gap-2">
              <h3 className="text-[13px] font-semibold text-ink">Proposed plan</h3>
              {open ? (
                <span className={cx("ml-auto font-mono text-[12px] tabular-nums", total > room ? "text-warning" : "text-ink-muted")} title="Picked against the time the day has for tasks">
                  {dur(total)} of {dur(room)}
                </span>
              ) : (
                <Badge tone="working" className="ml-auto">
                  Applied
                </Badge>
              )}
            </div>
            {blocks.length === 0 ? (
              <p className="mt-1.5 text-[12.5px] text-ink-subtle">Nothing planned: a day off.</p>
            ) : (
              <ul className="mt-1.5 flex flex-col">
                {blocks.map((b, i) => {
                  const on = picked.has(i) || !open;
                  return (
                    <li
                      key={i}
                      onClick={(e) => open && !(e.target as HTMLElement).closest('[role="checkbox"]') && toggle(i)}
                      className={cx("-mx-1 flex items-center gap-2.5 rounded-md px-1 py-1.5", open && "cursor-pointer hover:bg-surface-2/60")}
                    >
                      <Checkbox square checked={picked.has(i)} disabled={!open} onChange={() => toggle(i)} label={`${picked.has(i) ? "Leave out" : "Keep"} ${b.title}`} />
                      <span className="min-w-0 flex-1">
                        <span className={cx("block truncate text-[12.5px] leading-[18px]", on ? "text-ink" : "text-ink-subtle")} title={b.title}>
                          {b.title}
                        </span>
                        <span className="flex items-center gap-1.5 text-[11px] leading-4 text-ink-subtle tabular-nums">
                          <span aria-hidden className="size-1.5 shrink-0 rounded-[2px]" style={{ background: color(b) }} />
                          {formatTime(b.start_at)} to {formatTime(b.start_at + b.planned_minutes * 60_000)}
                        </span>
                      </span>
                      <span className="shrink-0 font-mono text-[11.5px] text-ink-subtle">{dur(b.planned_minutes)}</span>
                    </li>
                  );
                })}
              </ul>
            )}
            {change && (
              <p className="mt-1.5 text-[11.5px] leading-relaxed text-ink-subtle">
                Also sets this day's hours: <span className="text-ink-muted">{change}</span>
              </p>
            )}
            {open ? (
              <div className="mt-2.5 flex gap-2">
                <Button tone="primary" size="sm" className="!h-8 flex-1" onClick={apply} busy={applying} disabled={blocks.length > 0 && picked.size === 0}>
                  {blocks.length === 0 ? "Clear the plan" : picked.size === blocks.length ? "Apply plan" : `Apply ${picked.size} of ${blocks.length}`}
                </Button>
                <Button size="sm" className="!h-8" onClick={startOver} disabled={applying} title="Drop this proposal and start a new chat">
                  Discard
                </Button>
              </div>
            ) : (
              <p className="mt-2 text-[12px] text-ink-subtle">On your plan. Keep talking to change it again.</p>
            )}
          </section>
        )}

        {!asking && (
          <div className="flex flex-wrap gap-1.5 px-4 pt-3.5 pb-4">
            {(messages.length ? followUps : starters).map((s) => (
              <button
                key={s}
                type="button"
                onClick={() => send(s)}
                className="h-[26px] rounded-full border border-line-strong bg-surface-2 px-2.5 text-[12px] text-ink-muted transition-colors hover:border-line-3 hover:text-ink"
              >
                {s}
              </button>
            ))}
          </div>
        )}
      </div>

      <div className="flex shrink-0 flex-col gap-2 border-t border-chrome-line px-4 pt-3 pb-3.5">
        {voice.panel}
        <form
          className={cx(
            "flex items-end gap-1 rounded-xl border bg-surface-1 py-1.5 pr-1.5 pl-3 transition-colors focus-within:border-accent",
            voice.listening ? "border-accent/60" : "border-line-strong hover:border-line-3",
            asking && "opacity-60",
          )}
          onSubmit={(e) => {
            e.preventDefault();
            submit();
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
                submit();
              }
            }}
            placeholder={messages.length ? "Ask for a change, such as: move the reading after lunch" : `How should ${dayWord} go?`}
            title="Enter sends, Shift+Enter starts a new line"
            aria-label={`Message to ${name}`}
            maxLength={2000}
            disabled={!!asking}
            readOnly={voice.listening}
            className="max-h-32 min-h-10 min-w-0 flex-1 resize-none self-center bg-transparent py-0.5 text-[13px] leading-5 text-ink placeholder:text-ink-faint focus:outline-none"
          />
          {voice.button}
          <Button type="submit" size="sm" className="!h-8" busy={!!asking} disabled={!text.trim() && !voice.listening}>
            {failure ? "Send again" : "Send"}
          </Button>
        </form>
      </div>
    </aside>
  );
}

function Bubble({ ref, role, text, pending }: { ref?: Ref<HTMLDivElement>; role: "user" | "assistant"; text: string; pending?: boolean }) {
  const user = role === "user";
  return (
    <div
      ref={ref}
      className={cx(
        "max-w-[90%] rounded-xl px-3 py-2 text-[13px] leading-relaxed whitespace-pre-line",
        user ? "self-end rounded-br-[4px] bg-accent/15 text-ink" : "self-start rounded-bl-[4px] bg-surface-2 text-ink",
        pending && "opacity-70",
      )}
    >
      {text}
    </div>
  );
}
