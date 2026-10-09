import { CalendarCheck2, CircleAlert, Clock, MessageSquarePlus, SendHorizontal, Sparkles, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { App, apiError, wire } from "../api";
import { useDaemon } from "../daemon";
import { formatClock, formatDuration, formatTime } from "../format";
import type { ChatMessage, DayPlanOutput, ProposedBlock, RunError } from "../llm";
import { AiFailed, AiProgress, AiUnavailable, providerName } from "./ai";
import Face from "./Face";
import { useToast } from "./feedback";
import { ProjectTag } from "./tags";
import { Badge, Button, Callout, Checkbox, IconButton, Panel, TextArea, cx } from "./ui";

/** Messages to start from, sent with one click. */
const starters = ["Plan my day", "I'm tired today, keep it light", "Start at 14:00 and work 3 hours", "Most urgent work first, with breaks"];

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

/** "from 14:00 · 3h for tasks" for a change of hours. */
export function hoursText(h: DayPlanOutput["hours"]): string {
  if (!h) return "";
  const parts: string[] = [];
  if (h.start_minute != null) parts.push(`from ${formatClock(h.start_minute)}`);
  if (h.work_minutes != null) parts.push(h.work_minutes === 0 ? "no time for tasks" : `${formatDuration(h.work_minutes * 60_000)} for tasks`);
  return parts.join(" · ");
}

/**
 * Plan → Plan with AI (docs/08-clients.md#v3-additions): a conversation about
 * one day that ends in a plan. The latest proposal is listed with a checkbox
 * per block and previewed on the timeline through onPreview; nothing changes
 * until Apply plan.
 */
export default function PlanChat({ day, plan, onPreview, onClose }: { day: string; plan: wire.Plan; onPreview: (blocks: ProposedBlock[] | null) => void; onClose: () => void }) {
  const d = useDaemon();
  const notify = useToast();
  const [run, setRun] = useState<wire.LlmRun | null>(null); // the conversation's latest answer
  const [picked, setPicked] = useState<Set<number>>(new Set());
  const [text, setText] = useState("");
  const [asking, setAsking] = useState<{ message: string; since: number } | null>(null);
  const [failure, setFailure] = useState<string | null>(null);
  const [unavailable, setUnavailable] = useState<string | null>(null);
  const [refused, setRefused] = useState<string | null>(null);
  const [applying, setApplying] = useState(false);
  const scroller = useRef<HTMLDivElement>(null);

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
        (r) => live && r.kind === "day_plan" && r.subject_id === day && (r.status === "ok" || r.status === "accepted") && adopt(r),
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
  useEffect(() => {
    const el = scroller.current;
    if (el) el.scrollTop = el.scrollHeight;
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

  async function apply() {
    if (!run) return;
    setApplying(true);
    const done = await d.act(() => App.AcceptLLMRun(run.id, wire.AcceptRunRequest.createFrom({ indexes: [...picked].sort((a, b) => a - b) })));
    setApplying(false);
    if (!done) return;
    setRun(wire.LlmRun.createFrom({ ...run, status: "accepted" }));
    notify(picked.size ? `Planned ${picked.size} ${picked.size === 1 ? "block" : "blocks"} from the chat` : "Cleared the day's plan", "success");
  }

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

  return (
    <Panel
      title="Plan with AI"
      icon={Sparkles}
      actions={
        <>
          <span className="text-xs text-ink-subtle">{providerName(d.config?.llm)}</span>
          {(run || failure) && (
            <Button size="sm" tone="ghost" icon={MessageSquarePlus} onClick={startOver} disabled={!!asking}>
              New chat
            </Button>
          )}
          <IconButton icon={X} label="Close the chat" size="sm" onClick={onClose} />
        </>
      }
    >
      <div className="grid gap-5 @4xl:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)]">
        <div className="flex min-w-0 flex-col gap-3">
          <div ref={scroller} className="flex max-h-[min(24rem,46vh)] min-h-44 flex-col gap-3 overflow-y-auto rounded-lg border border-line bg-canvas/40 p-4" aria-live="polite">
            {messages.length === 0 && !asking ? (
              <div className="my-auto flex flex-col items-center gap-4 py-2 text-center">
                <p className="max-w-sm text-[13px] leading-relaxed text-ink-subtle">
                  Say how you want the day to go: when you start, how long you have, what matters most, how you feel. The plan comes from your open tasks, and nothing changes until you apply it.
                </p>
                <div className="flex flex-wrap justify-center gap-2">
                  {starters.map((s) => (
                    <button
                      key={s}
                      type="button"
                      onClick={() => send(s)}
                      className="rounded-full border border-line-strong bg-surface-2 px-3 py-1.5 text-xs text-ink-muted transition-colors hover:border-line-3 hover:text-ink"
                    >
                      {s}
                    </button>
                  ))}
                </div>
              </div>
            ) : (
              messages.map((m, i) => <Bubble key={i} role={m.role} text={m.text} />)
            )}
            {asking && <Bubble role="user" text={asking.message} pending />}
          </div>
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
          <form
            className="flex items-end gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              send(text);
            }}
          >
            <TextArea
              rows={2}
              value={text}
              onChange={(e) => setText(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !e.shiftKey) {
                  e.preventDefault();
                  send(text);
                }
              }}
              placeholder={messages.length ? "Ask for a change, such as: move the reading after lunch" : "How should today go?"}
              aria-label="Message to the assistant"
              maxLength={2000}
              disabled={!!asking}
              className="min-w-0 flex-1 resize-none"
            />
            <Button type="submit" tone="primary" icon={SendHorizontal} busy={!!asking} disabled={!text.trim()}>
              {failure ? "Send again" : "Send"}
            </Button>
          </form>
          <p className="text-xs text-ink-faint">Enter sends, Shift+Enter starts a new line.</p>
        </div>

        <div className="flex min-w-0 flex-col overflow-hidden rounded-lg border border-line">
          <div className="flex items-center gap-2 border-b border-line bg-surface-2 px-4 py-2.5">
            <CalendarCheck2 size={15} className="text-ink-subtle" aria-hidden />
            <span className="text-[13px] font-medium text-ink">Proposed plan</span>
            {run && !open && (
              <Badge tone="working" className="ml-auto">
                Applied
              </Badge>
            )}
            {open && (
              <span className={cx("ml-auto text-xs tabular-nums", total > room ? "text-idle" : "text-ink-subtle")}>
                {formatDuration(total * 60_000)} of {formatDuration(room * 60_000)}
              </span>
            )}
          </div>
          {!run ? (
            <p className="m-auto max-w-64 px-4 py-10 text-center text-[13px] leading-relaxed text-ink-subtle">The blocks the assistant proposes show here, and on the timeline above.</p>
          ) : (
            <>
              <ul className="max-h-[min(22rem,40vh)] divide-y divide-line overflow-y-auto">
                {blocks.map((b, i) => (
                  <li key={i} className={cx("flex items-start gap-3 px-4 py-2.5", open && "hover:bg-surface-2/60")}>
                    <span className="pt-0.5">
                      <Checkbox square checked={picked.has(i)} disabled={!open} onChange={() => toggle(i)} label={`Pick ${b.title}`} />
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className={cx("block truncate text-sm", picked.has(i) || !open ? "text-ink" : "text-ink-subtle")} title={b.title}>
                        {b.title}
                      </span>
                      <span className="mt-0.5 flex min-w-0 flex-wrap items-center gap-x-3 gap-y-0.5 text-xs text-ink-subtle">
                        <span className="text-ink-muted tabular-nums">
                          {formatTime(b.start_at)}–{formatTime(b.start_at + b.planned_minutes * 60_000)}
                        </span>
                        <ProjectTag project={d.projects.find((p) => p.id === b.project_id)} />
                        <span className="inline-flex items-center gap-1 tabular-nums">
                          <Clock size={12} aria-hidden />
                          {formatDuration(b.planned_minutes * 60_000)}
                        </span>
                      </span>
                    </span>
                  </li>
                ))}
                {blocks.length === 0 && <li className="px-4 py-6 text-center text-[13px] text-ink-subtle">Nothing planned: a day off.</li>}
              </ul>
              {change && (
                <p className="border-t border-line px-4 py-2.5 text-xs text-ink-muted">
                  Also sets this day's hours: <span className="text-ink">{change}</span>
                </p>
              )}
              <div className="mt-auto flex items-center justify-end gap-2 border-t border-line bg-surface-2 px-4 py-2.5">
                {open ? (
                  <Button tone="primary" icon={CalendarCheck2} onClick={apply} busy={applying} disabled={blocks.length > 0 && picked.size === 0}>
                    {blocks.length === 0 ? "Clear the plan" : picked.size === blocks.length ? "Apply plan" : `Apply ${picked.size} of ${blocks.length}`}
                  </Button>
                ) : (
                  <span className="text-xs text-ink-subtle">On your plan. Keep talking to change it again.</span>
                )}
              </div>
            </>
          )}
        </div>
      </div>
    </Panel>
  );
}

function Bubble({ role, text, pending }: { role: "user" | "assistant"; text: string; pending?: boolean }) {
  const user = role === "user";
  return (
    <div className={cx("flex items-end gap-2.5", user && "flex-row-reverse")}>
      {!user && <Face mood="smiling" breathe={false} className="size-14" />}
      <div
        className={cx(
          "max-w-[85%] rounded-xl px-3.5 py-2.5 text-[13px] leading-relaxed whitespace-pre-line",
          user ? "rounded-br-sm bg-accent/15 text-ink" : "rounded-bl-sm border border-line bg-surface-2 text-ink-muted",
          pending && "opacity-70",
        )}
      >
        {text}
      </div>
    </div>
  );
}
