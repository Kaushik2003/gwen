import { AudioLines, CircleAlert, CircleCheck, CircleX, Download, Drama, LoaderCircle, MessageSquarePlus, PhoneOff, SendHorizontal, Settings as SettingsIcon, Square, Volume2, VolumeX } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { App, apiError, isHub, wire } from "../api";
import { useDaemon } from "../daemon";
import { formatTime } from "../format";
import { useNav } from "../nav";
import { attitudeOf, type AssistantMessage, type AssistantOutput, type Mood, type RunError } from "../llm";
import { AiFailed, AiProgress, AiUnavailable, providerName } from "./ai";
import Call, { type CallPhase } from "./Call";
import Face from "./Face";
import { useAloud, useSpeech } from "./Speech";
import { Badge, Button, Callout, IconButton, TextArea, cx } from "./ui";
import { DownloadProgress, useDictation } from "./Voice";

/** How long a pause in hands-free talk ends what you are saying. */
const pauseMs = 1600;

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
 * reply lists what it changed. Replies can be read aloud in Gwen's voice,
 * and Talk makes it a spoken conversation, shown as a call (Call): the mic
 * listens, a pause sends what you said, she answers aloud, then listens
 * again. With call, it is only that call, started at once, as in the panel
 * widget's talk window; ending it calls onHangUp.
 */
export default function AssistantChat({
  initial,
  onInitialUsed,
  call,
  onHangUp,
}: {
  initial?: string | null;
  onInitialUsed?: () => void;
  call?: boolean;
  onHangUp?: () => void;
}) {
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
  const speech = useSpeech();
  const { go } = useNav();
  const [aloud, setAloud] = useAloud();
  const [talking, setTalking] = useState(false);
  const talkingRef = useRef(false);
  // The reply being read aloud, by its time, and its utterance.
  const [saying, setSaying] = useState<{ id: number; at: number } | null>(null);
  const [offerSpeech, setOfferSpeech] = useState(false);
  // How she has chosen to treat you, which outlives the chat.
  const [attitude, setAttitude] = useState("");
  const quiet = useRef(0); // sends what was said once a pause in talk lasts
  // What the box held when the mic opened; the words said follow it.
  const before = useRef("");
  const voice = useDictation(
    {
      onStart: () => {
        before.current = text;
        speech.stop(); // the mic would hear her
        input.current?.focus();
      },
      onText: (said, final) => {
        setText(after(before.current, said));
        window.clearTimeout(quiet.current);
        if (final) {
          input.current?.focus();
          if (!said && talkingRef.current) endTalk(); // cancelled, or heard nothing
        } else if (talkingRef.current && said.trim()) quiet.current = window.setTimeout(() => submitRef.current(), pauseMs);
      },
    },
    !!asking,
  );

  useEffect(() => {
    if (!isHub) App.AssistantSelf().then((s) => setAttitude(s.attitude), () => {});
  }, []);

  // A call starts as soon as the mic and her voice are known.
  const called = useRef(false);
  useEffect(() => {
    if (!call || called.current || !voice.known || !speech.status) return;
    called.current = true;
    startTalk();
  }, [call, voice.known, speech.status]);

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
  const last = messages.filter((m) => m.role === "assistant").at(-1);
  // The header's face: what is happening now, else the last reply's.
  const mood: Mood = asking ? "thinking" : failure || unavailable ? "crying" : refused ? "annoyed" : last ? turnMood(last) : "smiling";
  const phase: CallPhase = !talking ? "idle" : voice.listening ? "listening" : saying ? (speech.sounding === saying.id ? "speaking" : "preparing") : "thinking";
  // What she is saying in the call, else her last answer.
  const spoken = (saying && messages.find((m) => m.at === saying.at)) || last || null;
  useEffect(() => {
    const el = scroller.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [messages.length, asking]);

  /** Reads a reply aloud; in talk, the mic opens again once she is done. */
  async function read(m: AssistantMessage) {
    let ended = false;
    const id = await speech.say(m.text, (error) => {
      ended = true;
      setSaying((s) => (s?.at === m.at ? null : s));
      if (!talkingRef.current) return;
      if (error) endTalk();
      else listen();
    });
    if (id && !ended) setSaying({ id, at: m.at });
    else if (!id && talkingRef.current) endTalk();
  }

  async function listen() {
    if (!(await voice.start())) endTalk();
  }

  function startTalk() {
    if (!speech.ready) return setOfferSpeech(true);
    talkingRef.current = true;
    setTalking(true);
    if (!asking) listen();
  }

  function endTalk() {
    talkingRef.current = false;
    setTalking(false);
    window.clearTimeout(quiet.current);
    if (voice.listening) voice.cancel();
    speech.stop();
  }

  function toggleAloud() {
    if (aloud) {
      setAloud(false);
      speech.stop();
    } else if (speech.ready) setAloud(true);
    else setOfferSpeech(true);
  }

  async function installSpeech() {
    try {
      await speech.install();
      setOfferSpeech(false);
      setAloud(true);
    } catch (e) {
      d.fail(e);
    }
  }

  // Talk and speech end with the chat.
  useEffect(() => () => window.clearTimeout(quiet.current), []);

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
        if (talkingRef.current) endTalk();
      } else {
        setRun(r);
        remember(r.id);
        const reply = ((r.output as AssistantOutput).messages ?? []).at(-1);
        if (reply?.attitude) setAttitude(reply.attitude);
        if (reply?.role === "assistant" && (aloud || talkingRef.current)) read(reply);
      }
    } catch (e) {
      const err = apiError(e);
      setText(m);
      if (talkingRef.current) endTalk();
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
  const submitRef = useRef(submit);
  submitRef.current = submit;

  function startOver() {
    endTalk();
    remember(null);
    setRun(null);
    setFailure(null);
    setRefused(null);
    setText("");
  }

  // Why something stopped, or what her voice needs, wherever it is shown.
  const notices = (
    <>
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
      {speech.error && (
        <Callout tone="danger" icon={CircleAlert}>
          {name} couldn't speak: {speech.error}
        </Callout>
      )}
      {offerSpeech &&
        speech.status &&
        speech.provider &&
        (!speech.status.player ? (
          <Callout tone="idle" icon={CircleAlert}>
            {name}'s voice plays through PipeWire's player, which isn't installed. Install the <span className="text-ink">pipewire-utils</span> package, then try again.
          </Callout>
        ) : speech.provider.installing ? (
          <DownloadProgress progress={speech.progress} size={speech.provider.download_mb} what={`${name}'s voice`} />
        ) : !speech.provider.installed ? (
          <Callout
            tone="accent"
            icon={Volume2}
            action={
              <Button size="sm" tone="primary" icon={Download} onClick={installSpeech}>
                Download
              </Button>
            }
          >
            Hear {name} talk back. Her voice needs a one-time {speech.provider.download_mb} MB download, then works offline, on this computer.
          </Callout>
        ) : (
          speech.provider.key &&
          !speech.provider.has_key && (
            <Callout
              tone="accent"
              icon={Volume2}
              action={
                <Button size="sm" tone="primary" icon={SettingsIcon} onClick={() => (call ? App.OpenDashboard("settings") : go("settings", "ai"))}>
                  Add key
                </Button>
              }
            >
              Hear {name} talk back. Her {speech.provider.name} voice needs an API key, added in Settings → AI.
            </Callout>
          )
        ))}
    </>
  );

  const callView = (
    <Call
      name={name}
      mood={mood}
      phase={phase}
      saying={saying?.id ?? 0}
      heard={asking ? asking.message : text}
      reply={spoken}
      onSend={submit}
      onInterrupt={speech.stop}
      onEnd={() => {
        endTalk();
        onHangUp?.();
      }}
      onStart={startTalk}
      className={call ? "h-full" : "min-h-[min(40rem,70vh)]"}
    >
      {!talking && (
        <>
          {notices}
          {voice.panel}
        </>
      )}
    </Call>
  );
  if (call) return callView;

  return (
    <div className="lift flex flex-col overflow-hidden rounded-xl border border-line bg-surface-1">
      <div className="flex items-center gap-3 border-b border-line px-5 py-3">
        {!talking && <Face mood={mood} react talking={!!saying && speech.sounding === saying.id} className="-my-3 size-14" />}
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2 text-sm font-semibold text-ink">
            {name}
            {attitudeOf(attitude) && (
              <Badge tone={attitude === "angry" ? "danger" : "accent"} title={attitudeOf(attitude)!.means}>
                {attitudeOf(attitude)!.label}
              </Badge>
            )}
          </div>
          <div className="text-xs text-ink-subtle">{providerName(d.config?.llm)} · changes your tasks and plan as you talk</div>
        </div>
        {!isHub && (
          <>
            <IconButton icon={aloud ? Volume2 : VolumeX} label={aloud ? "Stop reading replies aloud" : "Read replies aloud"} tone={aloud ? "secondary" : "ghost"} onClick={toggleAloud} disabled={talking} />
            <Button size="sm" tone={talking ? "destroy" : "secondary"} icon={talking ? PhoneOff : AudioLines} onClick={talking ? endTalk : startTalk}>
              {talking ? "End talk" : "Talk"}
            </Button>
          </>
        )}
        {(run || failure) && (
          <Button size="sm" tone="ghost" icon={MessageSquarePlus} onClick={startOver} disabled={!!asking}>
            New chat
          </Button>
        )}
      </div>

      {talking ? (
        callView
      ) : (
        <>
          <div ref={scroller} className="flex max-h-[min(34rem,58vh)] min-h-72 flex-col gap-4 overflow-y-auto px-5 py-5" aria-live="polite">
            {messages.length === 0 && !asking ? (
              <div className="my-auto flex flex-col items-center gap-4 py-4 text-center">
                <Face mood="winking" react className="size-40" />
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
              messages.map((m, i) => (
                <Turn
                  key={i}
                  name={name}
                  message={m}
                  latest={m === last}
                  reading={saying?.at === m.at ? (speech.sounding === saying.id ? "sounding" : "preparing") : null}
                  onRead={speech.ready && m.role === "assistant" ? () => read(m) : undefined}
                  onStop={speech.stop}
                />
              ))
            )}
            {asking && <Turn message={{ role: "user", text: asking.message, at: asking.since }} pending />}
          </div>

          <div className="flex flex-col gap-3 border-t border-line bg-surface-2/40 px-5 py-4">
            {asking && <AiProgress since={asking.since} doing={`${name} is thinking`} />}
            {notices}
            {saying && <Speaking name={name} sounding={speech.sounding === saying.id} onStop={speech.stop} />}
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
        </>
      )}
    </div>
  );
}

/** The box's earlier text, then the words said. */
function after(before: string, said: string): string {
  if (!said) return before;
  return before.trim() ? `${before.trimEnd()} ${said}` : said;
}

/** The face of an assistant turn: the one it chose, else one for what it did. */
function turnMood(m: AssistantMessage): Mood {
  if (m.mood) return m.mood;
  const actions = m.actions ?? [];
  return actions.some((a) => !a.ok) ? "shocked" : actions.length ? "happy" : "smiling";
}

/** She is speaking, or about to: with a way to stop her. */
function Speaking({ name, sounding, onStop }: { name: string; sounding: boolean; onStop: () => void }) {
  return (
    <div className="flex items-center gap-3 rounded-lg border border-accent/30 bg-accent/[0.06] px-3.5 py-2" role="status">
      {sounding ? <Volume2 size={14} className="animate-live text-accent-hover" aria-hidden /> : <LoaderCircle size={14} className="animate-spin text-accent-hover" aria-hidden />}
      <span className="flex-1 text-[13px] text-ink-muted">{sounding ? `${name} is speaking` : `${name} is finding her words`}</span>
      <Button size="sm" tone="ghost" icon={Square} onClick={onStop}>
        Stop
      </Button>
    </div>
  );
}

function Turn({
  name,
  message,
  latest,
  pending,
  reading,
  onRead,
  onStop,
}: {
  name?: string;
  message: AssistantMessage;
  /** Her latest answer, whose face still breathes. */
  latest?: boolean;
  pending?: boolean;
  reading?: "preparing" | "sounding" | null;
  onRead?: () => void;
  onStop?: () => void;
}) {
  const user = message.role === "user";
  const actions = message.actions ?? [];
  const switched = attitudeOf(message.attitude);
  return (
    <div className={cx("flex items-start gap-2.5", user && "flex-row-reverse")}>
      {!user && <Face mood={turnMood(message)} talking={reading === "sounding"} breathe={latest} className="-mt-2 size-14" />}
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
        {switched && (
          <div className="flex items-center gap-2 px-1 text-xs text-ink-subtle" title={switched.means}>
            <Drama size={13} className="shrink-0 text-accent-hover" aria-hidden />
            <span>
              {name} is <span className="font-medium text-ink">{switched.label.toLowerCase()}</span> with you now
            </span>
          </div>
        )}
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
        {message.at > 0 && !pending && (
          <span className="flex items-center gap-1.5 px-1 text-[11px] text-ink-faint tabular-nums">
            {formatTime(message.at)}
            {onRead && (
              <button
                type="button"
                onClick={reading ? onStop : onRead}
                aria-label={reading ? "Stop reading" : "Read aloud"}
                title={reading ? "Stop reading" : "Read aloud"}
                className={cx("rounded p-0.5 transition-colors hover:text-ink", reading ? "text-accent-hover" : "text-ink-faint")}
              >
                {reading === "preparing" ? <LoaderCircle size={12} className="animate-spin" /> : reading ? <Square size={12} /> : <Volume2 size={12} />}
              </button>
            )}
          </span>
        )}
      </div>
    </div>
  );
}
