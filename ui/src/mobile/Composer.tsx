import { ArrowUp, ImagePlus, Mic } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { ApiError, api, idem } from "./api";
import { useStore } from "./store";
import { record, type Recording } from "./voice";

/**
 * Compose: what you write is pasted into the terminal and submitted as one message. A photo is saved beside the terminal on the
 * laptop and its path is added to the text. Hold the mic to talk; the words land here for you to check and send.
 */
export function Composer({ terminal, send, placeholder, disabled }: { terminal: string; send: (text: string, key: string) => Promise<string | void>; placeholder: string; disabled?: string }) {
  const { toast } = useStore();
  const [text, setText] = useState("");
  const [busy, setBusy] = useState("");
  const [recording, setRecording] = useState(false);
  const rec = useRef<Recording | null>(null);
  const wantStop = useRef(false);
  const key = useRef(idem());
  const area = useRef<HTMLTextAreaElement>(null);
  const file = useRef<HTMLInputElement>(null);

  useEffect(() => {
    const el = area.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(132, el.scrollHeight)}px`;
  }, [text]);
  useEffect(() => () => rec.current?.cancel(), []);

  const submit = async () => {
    const body = text.trim();
    if (!body || busy) return;
    // The laptop waits for you to finish typing in that terminal before it pastes; say so if it takes a moment.
    const slow = window.setTimeout(() => setBusy("Waiting for you to finish typing on the laptop"), 1500);
    setBusy("Sending");
    try {
      const said = await send(body, key.current);
      setText("");
      key.current = idem();
      if (said) toast(said);
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "That didn't go through.");
    } finally {
      window.clearTimeout(slow);
      setBusy("");
    }
  };

  const attach = async (f: File | undefined) => {
    if (!f) return;
    if (f.size > 8 * 1024 * 1024) return toast("That file is too large; 8 MB at most.");
    setBusy("Sending the photo to the laptop");
    try {
      const { path } = await api.put(terminal, f);
      setText((t) => (t && !t.endsWith(" ") && !t.endsWith("\n") ? t + " " : t) + path + " ");
      area.current?.focus();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "The photo didn't go through.");
    } finally {
      setBusy("");
      if (file.current) file.current.value = "";
    }
  };

  const stopTalking = async () => {
    wantStop.current = true;
    const r = rec.current;
    if (!r) return; // still asking for the microphone; startTalking stops it when it arrives
    rec.current = null;
    setRecording(false);
    const pcm = await r.stop();
    if (!pcm) return;
    setBusy("Turning that into text");
    try {
      const { text: heard } = await api.transcribe(pcm);
      if (heard) setText((t) => (t && !/\s$/.test(t) ? t + " " : t) + heard);
      else toast("Gwen didn't catch any words.");
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "Voice didn't work this time.");
    } finally {
      setBusy("");
    }
  };

  const startTalking = async () => {
    if (rec.current || busy) return;
    wantStop.current = false;
    try {
      const r = await record(() => stopTalking());
      rec.current = r;
      setRecording(true);
      if (wantStop.current) stopTalking();
    } catch (e) {
      toast((e as Error).message);
    }
  };

  if (disabled) return <div className="hint" style={{ padding: "12px 14px calc(var(--safe-bottom) + 12px)", borderTop: "1px solid var(--border)" }}>{disabled}</div>;
  return (
    <>
      {(busy || recording) && <div className="hint">{recording ? "Listening… let go to stop" : busy + "…"}</div>}
      <div className="composer">
        <input ref={file} type="file" accept="image/*" hidden onChange={(e) => attach(e.target.files?.[0])} />
        <button className="round" aria-label="Attach a photo" disabled={!!busy} onClick={() => file.current?.click()}>
          <ImagePlus size={19} />
        </button>
        <textarea ref={area} rows={1} value={text} placeholder={placeholder} onChange={(e) => setText(e.target.value)} />
        {text.trim() ? (
          <button className="round send" aria-label="Send" disabled={!!busy} onClick={submit}>
            <ArrowUp size={20} />
          </button>
        ) : (
          <button
            className={`round ${recording ? "rec" : ""}`}
            aria-label="Hold to talk"
            disabled={!!busy}
            onPointerDown={(e) => (e.preventDefault(), startTalking())}
            onPointerUp={stopTalking}
            onPointerCancel={stopTalking}
            onContextMenu={(e) => e.preventDefault()}
            style={{ touchAction: "none" }}
          >
            <Mic size={19} />
          </button>
        )}
      </div>
    </>
  );
}
