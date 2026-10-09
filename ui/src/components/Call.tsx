import { AudioLines, CircleCheck, Mic, PhoneOff, SendHorizontal } from "lucide-react";
import { useEffect, useRef, useState, type ReactNode } from "react";
import type { AssistantMessage, Mood } from "../llm";
import { EventsOn } from "../wailsjs/runtime/runtime";
import Face from "./Face";
import { Button, cx } from "./ui";

/** Where a spoken conversation is: idle before it starts or after it ends. */
export type CallPhase = "idle" | "listening" | "thinking" | "preparing" | "speaking";

const status: Record<CallPhase, (name: string) => string> = {
  idle: () => "Ready when you are",
  listening: () => "Listening",
  thinking: (name) => `${name} is thinking`,
  preparing: (name) => `${name} is finding her words`,
  speaking: (name) => `${name} is speaking`,
};

/**
 * Talk as a call: Gwen large and alone, glowing with whoever is talking —
 * your voice while she listens, hers while she answers, a slow pulse while
 * she thinks — with what is being said as a caption beneath her, and only
 * the controls a call needs. saying is the utterance being heard, whose
 * loudness moves her face.
 */
export default function Call({
  name,
  mood,
  phase,
  saying,
  heard,
  reply,
  onSend,
  onInterrupt,
  onEnd,
  onStart,
  className,
  children,
}: {
  name: string;
  mood: Mood;
  phase: CallPhase;
  saying: number;
  /** What you are saying, or last said. */
  heard: string;
  /** Her answer being said, or her last one. */
  reply: AssistantMessage | null;
  onSend: () => void;
  onInterrupt: () => void;
  onEnd: () => void;
  onStart?: () => void;
  className?: string;
  /** Shown above the controls, such as why the talk ended. */
  children?: ReactNode;
}) {
  const level = useCallLevel(phase, saying);
  const caption = useRef<HTMLDivElement>(null);
  const words = phase === "listening" || phase === "thinking" ? heard : (reply?.text ?? "");
  useEffect(() => {
    const el = caption.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [words]);

  // She leans into your voice while listening, and moves with her own.
  const faceLevel = phase === "speaking" ? level : phase === "listening" ? level * 0.25 : undefined;
  const glow = phase === "thinking" || phase === "preparing" ? null : phase === "idle" ? 0.15 : 0.25 + level * 0.75;
  const actions = phase === "speaking" || phase === "preparing" ? (reply?.actions ?? []).filter((a) => a.ok) : [];

  return (
    <div className={cx("relative isolate flex flex-col items-center overflow-hidden", className)}>
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0 -z-10 transition-[background] duration-500"
        style={{
          background: `radial-gradient(70% 55% at 50% 40%, color-mix(in srgb, var(--color-accent) ${phase === "idle" ? 8 : 16}%, transparent), transparent 72%)`,
        }}
      />

      <div className="mt-5 flex items-center gap-2 rounded-full border border-line-strong bg-surface-2/70 px-3 py-1 text-xs text-ink-muted" role="status">
        <span className={cx("size-1.5 rounded-full", phase === "listening" ? "animate-live bg-danger" : phase === "idle" ? "bg-ink-faint" : "animate-live bg-accent-hover")} aria-hidden />
        {status[phase](name)}
      </div>

      <div className="relative my-4 grid size-[min(19rem,46vh)] shrink-0 place-items-center">
        <div
          aria-hidden
          className={cx("absolute inset-[4%] rounded-full bg-accent/50 blur-3xl transition-[transform,opacity] duration-150 ease-out", glow === null && "animate-aura")}
          style={glow === null ? undefined : { opacity: glow, transform: `scale(${0.9 + level * 0.3})` }}
        />
        <div
          aria-hidden
          className="absolute inset-[6%] rounded-full border-2 border-accent-hover/50 transition-[transform,opacity] duration-100 ease-out"
          style={{ opacity: phase === "listening" || phase === "speaking" ? level * 0.9 : 0, transform: `scale(${1 + level * 0.16})` }}
        />
        <Face mood={mood} react level={faceLevel} className="relative size-[88%] drop-shadow-[0_18px_40px_rgba(94,106,210,0.35)]" label={`${name}, ${mood}`} />
      </div>

      <div className="text-title font-semibold text-ink">{name}</div>

      <div ref={caption} className="mt-3 max-h-36 w-full max-w-lg overflow-y-auto px-6 text-center text-[15px] leading-relaxed" aria-live="polite">
        {words ? (
          <p className={cx("whitespace-pre-line", phase === "listening" ? "text-ink" : phase === "thinking" ? "text-ink-subtle" : "text-ink-muted")}>{phase === "listening" || phase === "thinking" ? `“${words}”` : words}</p>
        ) : (
          <p className="text-ink-subtle">{phase === "listening" ? "Go ahead, I'm listening." : phase === "idle" ? `Talk to ${name} out loud: she answers in her own voice.` : ""}</p>
        )}
      </div>

      {actions.length > 0 && (
        <ul className="mt-3 flex max-w-lg flex-wrap justify-center gap-1.5 px-6">
          {actions.map((a, i) => (
            <li key={i} className="flex items-center gap-1.5 rounded-full border border-working/25 bg-working/8 px-2.5 py-1 text-xs text-ink">
              <CircleCheck size={12} className="shrink-0 text-working" aria-hidden />
              {a.summary}
            </li>
          ))}
        </ul>
      )}

      {children && <div className="mt-4 flex w-full max-w-lg flex-col gap-3 px-6">{children}</div>}

      <div className="mt-auto flex items-center justify-center gap-4 pt-5 pb-6">
        {phase === "idle" ? (
          onStart && (
            <Button tone="primary" icon={AudioLines} onClick={onStart}>
              Talk
            </Button>
          )
        ) : (
          <>
            {phase === "listening" && (
              <Button tone="secondary" icon={SendHorizontal} onClick={onSend} disabled={!heard.trim()}>
                Send now
              </Button>
            )}
            {(phase === "speaking" || phase === "preparing") && (
              <Button tone="secondary" icon={Mic} onClick={onInterrupt}>
                Cut in
              </Button>
            )}
            <button
              type="button"
              onClick={onEnd}
              aria-label="End talk"
              title="End talk"
              className="grid size-14 place-items-center rounded-full bg-danger text-white shadow-lg shadow-danger/25 transition-[transform,background-color] hover:scale-105 hover:bg-danger/90 active:scale-95"
            >
              <PhoneOff size={22} aria-hidden />
            </button>
          </>
        )}
      </div>
    </div>
  );
}

/** How loud whoever is talking is now, 0–1: you while listening, her while speaking. */
function useCallLevel(phase: CallPhase, saying: number): number {
  const [level, setLevel] = useState(0);
  useEffect(() => {
    setLevel(0);
    if (phase === "listening") return EventsOn("voice:level", (l: number) => setLevel(l));
    if (phase === "speaking") return EventsOn("speech:level", ({ id, level }: { id: number; level: number }) => id === saying && setLevel(level));
  }, [phase, saying]);
  return level;
}
