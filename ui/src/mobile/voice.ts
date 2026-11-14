// Voice: the phone records while you hold the mic, turns it into 16 kHz mono 16-bit PCM here, and the laptop turns that into text
// with its own speech model. Nothing is sent anywhere else, and the text is only ever put in front of you to review (plan.md 3.14.10).

import { useCallback, useEffect, useRef, useState, type MouseEvent, type PointerEvent as ReactPointerEvent } from "react";
import { ApiError, api } from "./api";

const RATE = 16000;
const MAX_SECONDS = 60;

export interface Recording {
  /** Stops recording and returns the audio, or null when it was too short to hold a word. */
  stop: () => Promise<ArrayBuffer | null>;
  cancel: () => void;
  /** Seconds of sound recorded so far. */
  seconds: () => number;
  /** The audio so far, still recording: for showing the words while you speak. Null while too short. */
  peek: () => ArrayBuffer | null;
}

/**
 * The audio context for one recording. It is made and resumed in the tap itself: a phone's browser keeps a context made later (after
 * the microphone was granted) suspended, and a suspended context records nothing, so the recording came back empty.
 */
export function audioContext(): AudioContext {
  let ctx: AudioContext;
  try {
    ctx = new AudioContext({ sampleRate: RATE });
  } catch {
    ctx = new AudioContext(); // a browser that can't record at 16 kHz records at its own rate, brought down after
  }
  ctx.resume().catch(() => {});
  return ctx;
}

export async function record(ctx: AudioContext, onLimit: () => void): Promise<Recording> {
  if (!navigator.mediaDevices?.getUserMedia) {
    ctx.close();
    throw new Error(window.isSecureContext ? "This browser can't record sound." : "Voice needs Gwen's https address; open the phone app from the pairing link.");
  }
  let stream: MediaStream;
  try {
    stream = await navigator.mediaDevices.getUserMedia({ audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true } });
  } catch (e) {
    ctx.close();
    const name = (e as DOMException)?.name;
    throw new Error(name === "NotFoundError" ? "This phone has no microphone Gwen can use." : name === "NotReadableError" ? "Another app is using the microphone." : "Allow the microphone for this page (the lock by the address), then hold the mic again.");
  }
  try {
    await ctx.audioWorklet.addModule("/recorder.js");
    if (ctx.state !== "running") await ctx.resume();
  } catch {
    stream.getTracks().forEach((t) => t.stop());
    ctx.close();
    throw new Error("This browser can't record sound here.");
  }
  const source = ctx.createMediaStreamSource(stream);
  const node = new AudioWorkletNode(ctx, "gwen-recorder");
  const chunks: Float32Array[] = [];
  let samples = 0;
  let limit = false;
  node.port.onmessage = (e: MessageEvent<Float32Array>) => {
    if (limit) return;
    chunks.push(e.data);
    samples += e.data.length;
    if (samples >= MAX_SECONDS * ctx.sampleRate) {
      limit = true;
      onLimit();
    }
  };
  source.connect(node);
  const end = () => {
    source.disconnect();
    node.disconnect();
    stream.getTracks().forEach((t) => t.stop());
    ctx.close();
  };
  const joined = () => {
    const all = new Float32Array(samples);
    let at = 0;
    for (const c of chunks) {
      all.set(c.subarray(0, Math.min(c.length, samples - at)), at);
      at += c.length;
      if (at >= samples) break;
    }
    return all;
  };
  return {
    cancel: end,
    seconds: () => samples / ctx.sampleRate,
    peek: () => {
      const rate = ctx.sampleRate;
      if (samples < rate * 0.5) return null;
      const all = joined();
      return toPCM(rate === RATE ? all : resample(all, rate));
    },
    stop: async () => {
      const rate = ctx.sampleRate;
      end();
      if (samples < rate * 0.3) return null;
      const all = new Float32Array(samples);
      let at = 0;
      for (const c of chunks) {
        all.set(c, at);
        at += c.length;
      }
      return toPCM(rate === RATE ? all : resample(all, rate));
    },
  };
}

/** Browsers that ignore the asked-for rate record at their own; bring that down to 16 kHz. */
function resample(input: Float32Array, from: number): Float32Array {
  const ratio = from / RATE;
  const out = new Float32Array(Math.floor(input.length / ratio));
  for (let i = 0; i < out.length; i++) {
    const start = Math.floor(i * ratio), end = Math.min(input.length, Math.floor((i + 1) * ratio));
    let sum = 0;
    for (let j = start; j < end; j++) sum += input[j];
    out[i] = sum / Math.max(1, end - start);
  }
  return out;
}

function toPCM(samples: Float32Array): ArrayBuffer {
  const n = Math.min(samples.length, MAX_SECONDS * RATE);
  const out = new DataView(new ArrayBuffer(n * 2));
  for (let i = 0; i < n; i++) {
    const s = Math.max(-1, Math.min(1, samples[i]));
    out.setInt16(i * 2, s < 0 ? s * 0x8000 : s * 0x7fff, true);
  }
  return out.buffer;
}

/**
 * The mic button: hold it while you talk and let go, or tap it once to start and again to stop. Then the laptop turns the words into
 * text, which goes to onText for you to check. Spread `bind` on the button; `state` says whether it is listening (and `tapped`: until
 * the next tap) or still turning the words into text.
 */
export function useHoldToTalk(onText: (text: string) => void, onError: (message: string) => void, disabled = false) {
  const [state, setState] = useState<"" | "listening" | "hearing">("");
  const [tapped, setTapped] = useState(false);
  const rec = useRef<Recording | null>(null);
  const wantStop = useRef(false);
  const starting = useRef(false);
  const toggle = useRef(false);
  const pressedAt = useRef(0);
  const askedAt = useRef(0);
  const listeningAt = useRef(0);
  const said = useRef(onText);
  const failed = useRef(onError);
  said.current = onText;
  failed.current = onError;
  useEffect(() => () => rec.current?.cancel(), []);

  const stop = useCallback(async () => {
    wantStop.current = true;
    toggle.current = false;
    setTapped(false);
    const r = rec.current;
    if (!r) return; // still asking for the microphone; start stops it when it arrives
    rec.current = null;
    setState("");
    const silent = r.seconds() === 0 && Date.now() - listeningAt.current > 1000;
    const pcm = await r.stop();
    if (!pcm) {
      failed.current(silent ? "The microphone gave no sound. Check that this page may use it, then try again." : "Too short to hear a word: hold the mic while you talk, or tap it to start and again to stop.");
      return;
    }
    setState("hearing");
    try {
      const { text } = await api.transcribe(pcm);
      if (text) said.current(text);
      else failed.current("Gwen didn't catch any words.");
    } catch (e) {
      failed.current(e instanceof ApiError ? e.message : "Voice didn't work this time.");
    } finally {
      setState("");
    }
  }, []);

  const start = useCallback(async () => {
    if (rec.current || starting.current) return;
    wantStop.current = false;
    starting.current = true;
    const ctx = audioContext(); // in the tap, before anything waits
    askedAt.current = Date.now();
    try {
      const r = await record(ctx, () => stop());
      if (wantStop.current && Date.now() - askedAt.current > 1500) {
        // The phone asked whether Gwen may use the microphone, and the finger was gone by the answer.
        r.cancel();
        failed.current("The microphone is on now: hold the mic while you talk.");
        return;
      }
      rec.current = r;
      listeningAt.current = Date.now();
      setState("listening");
      if (wantStop.current) stop();
    } catch (e) {
      toggle.current = false;
      setTapped(false);
      failed.current((e as Error).message);
    } finally {
      starting.current = false;
    }
  }, [stop]);

  const bind = {
    disabled: disabled || state === "hearing",
    onPointerDown: (e: ReactPointerEvent) => {
      e.preventDefault();
      if (toggle.current) return void stop(); // the second tap
      pressedAt.current = Date.now();
      start();
    },
    onPointerUp: () => {
      if (!pressedAt.current) return;
      const held = Date.now() - pressedAt.current;
      pressedAt.current = 0;
      if (held < 350 && (rec.current || starting.current)) {
        // A quick tap: keep listening until the next one.
        toggle.current = true;
        setTapped(true);
        return;
      }
      stop();
    },
    onPointerCancel: () => {
      if (!toggle.current) stop();
    },
    onContextMenu: (e: MouseEvent) => e.preventDefault(),
    style: { touchAction: "none" } as const,
  };
  return { state, tapped, bind };
}
