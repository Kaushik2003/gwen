import { CircleAlert, Download, LoaderCircle, Mic, Square } from "lucide-react";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { App, apiError, isHub } from "../api";
import { useTick } from "../daemon";
import type { main } from "../wailsjs/go/models";
import { EventsOn } from "../wailsjs/runtime/runtime";
import { useToast } from "./feedback";
import { Button, Callout, IconButton, Meter, cx } from "./ui";

/** A recording stops by itself after two minutes, as the host keeps no more. */
const maxSeconds = 120;

/** How many recent levels the listening waveform shows, at ten a second. */
const bars = 28;

/** Voice input's state from the host, kept fresh across every mic and Settings. */
export function useVoice() {
  const [status, setStatus] = useState<main.VoiceStatus | null>(null);
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null);
  useEffect(() => {
    if (isHub) return;
    let live = true;
    const refresh = () => App.VoiceStatus().then((s) => live && setStatus(s), () => {});
    refresh();
    const offChanged = EventsOn("voice:changed", refresh);
    const offProgress = EventsOn("voice:progress", setProgress);
    return () => {
      live = false;
      offChanged();
      offProgress();
    };
  }, []);
  return { status, progress: status?.installing ? progress : null };
}

/** The model download as a person reads it: "42 of 99 MB". */
export function DownloadProgress({ progress, size, what = "the voice model" }: { progress: { done: number; total: number } | null; size: number; what?: string }) {
  const mb = (n: number) => Math.round(n / 2 ** 20);
  return (
    <div className="flex flex-col gap-2" role="status">
      <div className="flex items-center gap-2 text-[13px] text-ink-muted">
        <LoaderCircle size={14} className="animate-spin text-accent-hover" aria-hidden />
        Downloading {what}
        <span className="ml-auto text-xs text-ink-faint tabular-nums">{progress ? `${mb(progress.done)} of ${mb(progress.total)} MB` : `${size} MB`}</span>
      </div>
      <Meter value={progress ? progress.done / progress.total : 0} label={`Downloading ${what}`} height={4} />
    </div>
  );
}

/** What a dictation tells the message box it fills. */
export interface DictationHandlers {
  /** Listening began: remember what the box held. */
  onStart?: () => void;
  /** Everything said so far, as it grows; final once listening ends, "" when cancelled. */
  onText: (said: string, final: boolean) => void;
}

/**
 * Speak instead of typing: the mic starts listening, the text grows as you
 * talk, phrase by phrase, and pressing the mic again ends it, all on this
 * computer (internal/voice). The button goes beside a message box, the panel
 * above it, where it shows the live waveform, the one-time download, or what
 * went wrong. finish ends listening and resolves to everything said, or null
 * when that failed; start and cancel are the mic's own, for hands-free talk,
 * and known whether the mic's state is in, so start can be trusted.
 */
export function useDictation(
  handlers: DictationHandlers,
  disabled?: boolean,
): { button: ReactNode; panel: ReactNode; listening: boolean; known: boolean; finish: () => Promise<string | null>; start: () => Promise<boolean>; cancel: () => void } {
  const { status, progress } = useVoice();
  const notify = useToast();
  const [phase, setPhase] = useState<"idle" | "listening" | "transcribing">("idle");
  const [since, setSince] = useState(0);
  const [levels, setLevels] = useState<number[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [offer, setOffer] = useState(false);
  const phaseRef = useRef(phase);
  phaseRef.current = phase;
  const on = useRef(handlers);
  on.current = handlers;

  useEffect(() => {
    if (phase !== "listening") return;
    const off = EventsOn("voice:level", (l: number) => setLevels((ls) => [...ls.slice(1 - bars), l]));
    // A late update after stop would undo the final text, so only while listening.
    const offText = EventsOn("voice:text", (said: string) => phaseRef.current === "listening" && on.current.onText(said, false));
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        cancel();
      }
    };
    window.addEventListener("keydown", onKey);
    const limit = window.setTimeout(stop, maxSeconds * 1000);
    return () => {
      off();
      offText();
      window.removeEventListener("keydown", onKey);
      window.clearTimeout(limit);
    };
  }, [phase]);

  // A recording does not outlive the screen it was started on.
  useEffect(() => () => void (phaseRef.current === "listening" && App.CancelListening()), []);

  /** Opens the mic, resolving to whether it is listening. */
  async function start(): Promise<boolean> {
    setError(null);
    setOffer(!status?.recorder || !status.installed);
    if (!status?.recorder || !status.installed) return false;
    try {
      await App.StartListening();
      on.current.onStart?.();
      setLevels([]);
      setSince(Date.now());
      phaseRef.current = "listening";
      setPhase("listening");
      return true;
    } catch (e) {
      setError(apiError(e).message);
      return false;
    }
  }

  async function stop(): Promise<string | null> {
    if (phaseRef.current !== "listening") return null;
    phaseRef.current = "transcribing";
    setPhase("transcribing");
    let said: string | null = null;
    try {
      said = await App.StopListening();
      on.current.onText(said, true);
      if (!said) setError("Didn't catch any words. Check that the right microphone is on in your sound settings, then try again.");
    } catch (e) {
      setError(apiError(e).message);
    }
    setPhase("idle");
    return said;
  }

  function cancel() {
    App.CancelListening();
    phaseRef.current = "idle";
    setPhase("idle");
    on.current.onText("", true);
  }

  async function install() {
    setError(null);
    try {
      await App.InstallVoice();
      setOffer(false);
      notify("Voice input is ready. Press the mic and speak.", "success");
    } catch (e) {
      setError(apiError(e).message);
    }
  }

  const listening = phase === "listening";
  const known = !!status;
  if (isHub) return { button: null, panel: null, listening, known, finish: stop, start, cancel };

  const button = (
    <IconButton
      icon={phase === "transcribing" ? LoaderCircle : listening ? Square : Mic}
      label={listening ? "Stop listening" : phase === "transcribing" ? "Finishing the text" : "Speak instead of typing"}
      tone={listening ? "destroy" : "secondary"}
      onClick={listening ? stop : start}
      disabled={!status || status.installing || phase === "transcribing" || (disabled && !listening)}
      className={cx(phase === "transcribing" && "[&>svg]:animate-spin")}
    />
  );

  let panel: ReactNode = null;
  if (listening) panel = <Listening since={since} levels={levels} onCancel={cancel} />;
  else if (status?.installing) panel = <DownloadProgress progress={progress} size={status.download_mb} />;
  else if (error) panel = <Callout tone="danger" icon={CircleAlert}>{error}</Callout>;
  else if (offer && status && !status.recorder)
    panel = (
      <Callout tone="idle" icon={CircleAlert}>
        Voice input records through PipeWire's recorder, which isn't installed. Install the <span className="text-ink">pipewire-utils</span> package, then try again.
      </Callout>
    );
  else if (offer && status && !status.installed)
    panel = (
      <Callout
        tone="accent"
        icon={Mic}
        action={
          <Button size="sm" tone="primary" icon={Download} onClick={install}>
            Download
          </Button>
        }
      >
        Talk instead of typing. Voice input needs a one-time {status.download_mb} MB download, then works offline: your voice never leaves this computer.
      </Callout>
    );
  return { button, panel, listening, known, finish: stop, start, cancel };
}

/** The live waveform while the mic is open, with how long it has been. */
function Listening({ since, levels, onCancel }: { since: number; levels: number[]; onCancel: () => void }) {
  useTick(1000);
  const secs = Math.max(0, Math.floor((Date.now() - since) / 1000));
  const shown = [...Array(Math.max(0, bars - levels.length)).fill(0), ...levels];
  return (
    <div className="flex items-center gap-3 rounded-lg border border-danger/30 bg-danger/[0.06] px-3.5 py-2.5" role="status">
      <span className="size-2 shrink-0 animate-live rounded-full bg-danger" aria-hidden />
      <span className="text-[13px] text-ink-muted">Listening</span>
      <div className="flex h-6 min-w-0 flex-1 items-center gap-[3px]" aria-hidden>
        {shown.map((l, i) => (
          <span key={i} className="w-[3px] shrink-0 rounded-full bg-danger/70 transition-[height] duration-100" style={{ height: `${Math.max(12, l * 100)}%` }} />
        ))}
      </div>
      <span className="text-xs text-ink-faint tabular-nums">
        {Math.floor(secs / 60)}:{String(secs % 60).padStart(2, "0")}
      </span>
      <button type="button" onClick={onCancel} className="text-xs text-ink-subtle underline-offset-2 hover:text-ink hover:underline">
        Cancel (Esc)
      </button>
    </div>
  );
}
