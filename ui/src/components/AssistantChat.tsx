import { CircleAlert, CircleCheck, CircleX, MessageSquarePlus, SendHorizontal, Sparkles } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { App, apiError, wire } from "../api";
import { useDaemon } from "../daemon";
import { formatTime } from "../format";
import type { AssistantMessage, AssistantOutput, RunError } from "../llm";
import { AiFailed, AiProgress, AiUnavailable, providerName } from "./ai";
import { Button, Callout, IconButton, TextArea, cx } from "./ui";
import { useDictation } from "./Voice";

/** Things to say with one click. */
const starters = [
  "What should I do first today?",
  "Process my inbox",
  "I have an exam on Friday, help me prepare",
  "Move my hardest task to my prime time",
  "Sort my tasks with the Eisenhower matrix",
  "Turn my goal into a SMART goal",
];

const runKey = "gwen.assistant.run";

function remember(id: string | null) {
  try {
    if (id) localStorage.setItem(runKey, id);
    else localStorage.removeItem(runKey);
  } catch {
    // storage is a convenience
  }
}

function recall(): string | null {
  try {
    return localStorage.getItem(runKey);
  } catch {
    return null;
  }
}

/**
 * Assistant → Chat: talk in plain words, and the assistant creates, moves,
 * reschedules, and reassigns tasks, and sets goals, as it answers. Each
 * reply lists what it changed.
 */
export default function AssistantChat({ initial, onInitialUsed }: { initial?: string | null; onInitialUsed?: () => void }) {
  const d = useDaemon();
  const [run, setRun] = useState<wire.LlmRun | null>(null);
  const [text, setText] = useState("");
  const [asking, setAsking] = useState<{ message: string; since: number } | null>(null);
  const [failure, setFailure] = useState<string | null>(null);
  const [unavailable, setUnavailable] = useState<string | null>(null);
  const [refused, setRefused] = useState<string | null>(null);
  const scroller = useRef<HTMLDivElement>(null);
  const input = useRef<HTMLTextAreaElement>(null);
  const name = d.config?.llm.assistant_name || "Gwen";
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

  useEffect(() => {
    let live = true;
    const id = recall();
    if (id)
      App.GetLLMRun(id).then(
        (r) => live && r.kind === "assistant" && r.status === "ok" && setRun(r),
        () => remember(null),
      );
    return () => {
      live = false;
    };
  }, []);

  useEffect(() => {
    if (initial) {
      setText(initial);
      input.current?.focus();
      onInitialUsed?.();
    }
  }, [initial]);

  const messages: AssistantMessage[] = run ? ((run.output as AssistantOutput).messages ?? []) : [];
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
      const r = await App.AssistantChat(wire.AssistantChatRequest.createFrom({ message: m, run_id: run?.id ?? null }));
      if (r.status === "failed") {
        setFailure((r.output as RunError).error);
        setText(m);
      } else {
        setRun(r);
        remember(r.id);
      }
    } catch (e) {
      const err = apiError(e);
      setText(m);
      if (err.code === "unavailable") setUnavailable(err.message);
      else if (err.code === "invalid_request") setRefused(err.message);
      else d.fail(e);
    } finally {
      setAsking(null);
      input.current?.focus();
    }
  }

  /** Sends the box, or while the mic is open, everything said once it stops. */
  async function submit() {
    if (!voice.listening) return send(text);
    const said = await voice.finish();
    if (said != null) send(after(before.current, said));
  }

  function startOver() {
    remember(null);
    setRun(null);
    setFailure(null);
    setRefused(null);
    setText("");
  }

  return (
    <div className="lift flex flex-col overflow-hidden rounded-xl border border-line bg-surface-1">
      <div className="flex items-center gap-3 border-b border-line px-5 py-3">
        <span className="grid size-8 place-items-center rounded-full bg-accent/15">
          <Sparkles size={16} className="text-accent-hover" aria-hidden />
        </span>
        <div className="min-w-0 flex-1">
          <div className="text-sm font-semibold text-ink">{name}</div>
          <div className="text-xs text-ink-subtle">{providerName(d.config?.llm)} · changes your tasks and plan as you talk</div>
        </div>
        {(run || failure) && (
          <Button size="sm" tone="ghost" icon={MessageSquarePlus} onClick={startOver} disabled={!!asking}>
            New chat
          </Button>
        )}
      </div>

      <div ref={scroller} className="flex max-h-[min(34rem,58vh)] min-h-72 flex-col gap-4 overflow-y-auto px-5 py-5" aria-live="polite">
        {messages.length === 0 && !asking ? (
          <div className="my-auto flex flex-col items-center gap-4 py-4 text-center">
            <p className="max-w-md text-[13px] leading-relaxed text-ink-subtle">
              Talk to {name} like you would to a person. Tell it about new work, ask what to do next, or say "move the report to tomorrow afternoon": it adds, moves, reschedules, and reassigns tasks for you, and tells you what it changed.
            </p>
            <div className="flex max-w-xl flex-wrap justify-center gap-2">
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
          messages.map((m, i) => <Turn key={i} message={m} />)
        )}
        {asking && <Turn message={{ role: "user", text: asking.message, at: asking.since }} pending />}
      </div>

      <div className="flex flex-col gap-3 border-t border-line bg-surface-2/40 px-5 py-4">
        {asking && <AiProgress since={asking.since} doing={`${name} is thinking`} />}
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
        {voice.panel}
        <form
          className="flex items-end gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            submit();
          }}
        >
          <TextArea
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
            placeholder={messages.length ? "Reply, or ask for a change" : `Tell ${name} what's on your mind`}
            aria-label={`Message to ${name}`}
            maxLength={2000}
            disabled={!!asking}
            readOnly={voice.listening}
            className="min-w-0 flex-1 resize-none"
          />
          {voice.button}
          <IconButton type="submit" tone="primary" icon={SendHorizontal} label="Send" disabled={!text.trim() || !!asking} />
        </form>
        <p className="text-xs text-ink-faint">
          {voice.listening ? "Talk, and your words appear as you go. Enter sends them, Esc cancels." : `Enter sends, Shift+Enter starts a new line${voice.button ? ", the mic lets you speak instead" : ""}.`}
        </p>
      </div>
    </div>
  );
}

/** The box's earlier text, then the words said. */
function after(before: string, said: string): string {
  if (!said) return before;
  return before.trim() ? `${before.trimEnd()} ${said}` : said;
}

function Turn({ message, pending }: { message: AssistantMessage; pending?: boolean }) {
  const user = message.role === "user";
  const actions = message.actions ?? [];
  return (
    <div className={cx("flex items-start gap-2.5", user && "flex-row-reverse")}>
      {!user && (
        <span className="mt-0.5 grid size-7 shrink-0 place-items-center rounded-full bg-accent/15">
          <Sparkles size={14} className="text-accent-hover" aria-hidden />
        </span>
      )}
      <div className={cx("flex max-w-[85%] min-w-0 flex-col gap-2", user && "items-end")}>
        <div
          className={cx(
            "rounded-xl px-3.5 py-2.5 text-[13px] leading-relaxed whitespace-pre-line",
            user ? "rounded-tr-sm bg-accent/15 text-ink" : "rounded-tl-sm border border-line bg-surface-2 text-ink-muted",
            pending && "opacity-70",
          )}
        >
          {message.text}
        </div>
        {actions.length > 0 && (
          <ul className="flex flex-col gap-1">
            {actions.map((a, i) => (
              <li key={i} className={cx("flex items-start gap-2 rounded-md border px-2.5 py-1.5 text-xs", a.ok ? "border-working/25 bg-working/8 text-ink" : "border-danger/25 bg-danger/8 text-ink-muted")}>
                {a.ok ? <CircleCheck size={13} className="mt-px shrink-0 text-working" aria-hidden /> : <CircleX size={13} className="mt-px shrink-0 text-danger" aria-hidden />}
                <span className="min-w-0">
                  {a.summary}
                  {!a.ok && a.error && <span className="text-ink-subtle"> — not done: {a.error}</span>}
                </span>
              </li>
            ))}
          </ul>
        )}
        {message.at > 0 && !pending && <span className="px-1 text-[11px] text-ink-faint tabular-nums">{formatTime(message.at)}</span>}
      </div>
    </div>
  );
}
