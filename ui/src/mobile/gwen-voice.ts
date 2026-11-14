import { useCallback, useEffect, useRef, useState, type MouseEvent, type PointerEvent as ReactPointerEvent } from "react";
import { ApiError, api } from "./api";
import { audioContext, record, type Recording } from "./voice";

// Talking to Gwen (the phone redo, P5): hold the mic and talk; letting go sends what you said, sliding left first cancels it. A quick
// tap keeps listening until you tap again, and then the words wait in the field for you to check. While you speak, the laptop's
// speech model hears what you have said so far every few seconds, so the words show as you go.

/** How far left a held finger slides to cancel, in CSS pixels. */
const CANCEL_PX = 70;
/** How often the words so far are heard again while you speak. */
const INTERIM_MS = 2200;

export interface Talk {
  /** Listening, or turning the last words into text. */
  state: "" | "listening" | "hearing";
  /** Started by a tap: listening until the next tap (or Done). */
  tapped: boolean;
  /** The finger slid far enough left that letting go cancels. */
  armed: boolean;
  /** What she heard so far. */
  interim: string;
  seconds: number;
  /** Spread on the mic button. */
  bind: {
    disabled: boolean;
    onPointerDown: (e: ReactPointerEvent<HTMLElement>) => void;
    onPointerMove: (e: ReactPointerEvent<HTMLElement>) => void;
    onPointerUp: () => void;
    onPointerCancel: () => void;
    onContextMenu: (e: MouseEvent) => void;
    style: { touchAction: "none" };
  };
  /** Tap mode: stop, and put the words in the field. */
  done: () => void;
  /** Tap mode: stop and send at once. */
  send: () => void;
  cancel: () => void;
}

export function useTalk(on: { send: (text: string) => void; draft: (text: string) => void; error: (message: string) => void }, disabled = false): Talk {
  const [state, setState] = useState<Talk["state"]>("");
  const [tapped, setTapped] = useState(false);
  const [armed, setArmed] = useState(false);
  const [interim, setInterim] = useState("");
  const [seconds, setSeconds] = useState(0);
  const rec = useRef<Recording | null>(null);
  const starting = useRef(false);
  const tapMode = useRef(false);
  const wantStop = useRef<"" | "send" | "draft" | "cancel">("");
  const pressedAt = useRef(0);
  const startX = useRef(0);
  const armedNow = useRef(false);
  const askedAt = useRef(0);
  const generation = useRef(0);
  const cb = useRef(on);
  cb.current = on;

  const reset = () => {
    tapMode.current = false;
    setTapped(false);
    setArmed(false);
    armedNow.current = false;
    setInterim("");
    setSeconds(0);
  };

  const cancel = useCallback(() => {
    generation.current++;
    wantStop.current = "cancel";
    rec.current?.cancel();
    rec.current = null;
    setState("");
    reset();
  }, []);
  useEffect(() => () => rec.current?.cancel(), []);

  const finish = useCallback(async (kind: "send" | "draft") => {
    wantStop.current = kind;
    const r = rec.current;
    if (!r) return; // still asking for the microphone; start finishes when it arrives
    rec.current = null;
    const gen = ++generation.current;
    const silent = r.seconds() === 0;
    const pcm = await r.stop();
    reset();
    if (!pcm) {
      setState("");
      cb.current.error(silent ? "The microphone gave no sound. Check that this page may use it, then try again." : "Too short to hear a word: hold the mic while you talk.");
      return;
    }
    setState("hearing");
    try {
      const { text } = await api.transcribe(pcm);
      if (gen !== generation.current) return;
      if (!text?.trim()) cb.current.error("Gwen didn't catch any words.");
      else if (kind === "send") cb.current.send(text.trim());
      else cb.current.draft(text.trim());
    } catch (e) {
      cb.current.error(e instanceof ApiError ? e.message : "Voice didn't work this time.");
    } finally {
      if (gen === generation.current) setState("");
    }
  }, []);

  const start = useCallback(async () => {
    if (rec.current || starting.current) return;
    wantStop.current = "";
    starting.current = true;
    const ctx = audioContext(); // in the tap, before anything waits
    askedAt.current = Date.now();
    try {
      const r = await record(ctx, () => finish(tapMode.current ? "draft" : "send"));
      // Read again after the wait: a finger lifted (or a cancel) while the microphone was being asked for.
      const stopWith = wantStop.current as "" | "send" | "draft" | "cancel";
      if (stopWith && Date.now() - askedAt.current > 1500) {
        // The phone asked whether Gwen may use the microphone, and the finger was gone by the answer.
        r.cancel();
        reset();
        if (stopWith !== "cancel") cb.current.error("The microphone is on now: hold the mic while you talk.");
        return;
      }
      if (stopWith === "cancel") {
        r.cancel();
        return;
      }
      rec.current = r;
      setState("listening");
      if (stopWith) finish(stopWith);
    } catch (e) {
      reset();
      cb.current.error((e as Error).message);
    } finally {
      starting.current = false;
    }
  }, [finish]);

  // While listening: the clock, and the words so far, heard again every few seconds (one at a time).
  useEffect(() => {
    if (state !== "listening") return;
    const gen = generation.current;
    let busy = false;
    const tick = window.setInterval(() => setSeconds(Math.floor(rec.current?.seconds() ?? 0)), 250);
    const hear = window.setInterval(() => {
      const r = rec.current;
      if (!r || busy) return;
      const pcm = r.peek();
      if (!pcm) return;
      busy = true;
      api
        .transcribe(pcm)
        .then(({ text }) => gen === generation.current && rec.current && text?.trim() && setInterim(text.trim()))
        .catch(() => {})
        .finally(() => (busy = false));
    }, INTERIM_MS);
    return () => {
      window.clearInterval(tick);
      window.clearInterval(hear);
    };
  }, [state]);

  const bind: Talk["bind"] = {
    disabled: disabled || state === "hearing",
    onPointerDown: (e) => {
      e.preventDefault();
      if (tapMode.current) return void finish("draft"); // the second tap
      try {
        e.currentTarget.setPointerCapture(e.pointerId);
      } catch {
        // a browser without capture still works, only the slide is less sure
      }
      pressedAt.current = Date.now();
      startX.current = e.clientX;
      armedNow.current = false;
      setArmed(false);
      start();
    },
    onPointerMove: (e) => {
      if (!pressedAt.current || tapMode.current) return;
      const now = e.clientX - startX.current < -CANCEL_PX;
      if (now !== armedNow.current) {
        armedNow.current = now;
        setArmed(now);
      }
    },
    onPointerUp: () => {
      if (!pressedAt.current) return;
      const held = Date.now() - pressedAt.current;
      pressedAt.current = 0;
      if (armedNow.current) return cancel();
      if (held < 350 && (rec.current || starting.current)) {
        // A quick tap: keep listening until the next one.
        tapMode.current = true;
        setTapped(true);
        return;
      }
      finish("send");
    },
    onPointerCancel: () => {
      if (!pressedAt.current || tapMode.current) return;
      pressedAt.current = 0;
      cancel();
    },
    onContextMenu: (e) => e.preventDefault(),
    style: { touchAction: "none" },
  };
  return { state, tapped, armed, interim, seconds, bind, done: () => finish("draft"), send: () => finish("send"), cancel };
}
