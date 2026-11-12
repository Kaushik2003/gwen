// Voice: the phone records while you hold the mic, turns it into 16 kHz mono 16-bit PCM here, and the laptop turns that into text
// with its own speech model. Nothing is sent anywhere else, and the text is only ever put in front of you to review (plan.md 3.14.10).

const RATE = 16000;
const MAX_SECONDS = 60;

export interface Recording {
  /** Stops recording and returns the audio, or null when it was too short to hold a word. */
  stop: () => Promise<ArrayBuffer | null>;
  cancel: () => void;
}

export async function record(onLimit: () => void): Promise<Recording> {
  if (!navigator.mediaDevices?.getUserMedia) throw new Error("This browser can't record sound.");
  let stream: MediaStream;
  try {
    stream = await navigator.mediaDevices.getUserMedia({ audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true } });
  } catch {
    throw new Error("Allow the microphone for this page to talk to Gwen.");
  }
  const ctx = new AudioContext({ sampleRate: RATE });
  await ctx.audioWorklet.addModule("/recorder.js");
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
  return {
    cancel: end,
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
