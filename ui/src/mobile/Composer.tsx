import { ArrowUp, ImagePlus, Mic } from "lucide-react";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { ApiError, api, idem } from "./api";
import { useStore } from "./store";
import { useHoldToTalk } from "./voice";

/** Chips that only fill the composer, for you to edit and send (plan.md 3.14.13). */
export interface Ask { label: string; text: string }

/**
 * Compose: what you write is pasted into the terminal and submitted as one message. A photo is saved beside the terminal on the
 * laptop and its path is added to the text; so is a pasted screenshot. Hold the mic to talk; the words land here for you to check.
 * It draws the two lines of a session's footer: the mode chip or the chips with a short note, then the field, the mic and Send.
 */
export function Composer({
  terminal, send, placeholder, disabled, chip, lead, asks, seed, autoFocus,
}: {
  terminal: string;
  /** Sends the text; what it returns is said in a short note. */
  send: (text: string, key: string) => Promise<string | void>;
  placeholder: string;
  /** Why nothing can be sent now, e.g. "Answer Claude first"; the field stays, greyed. */
  disabled?: string;
  /** Shown first on the top line, e.g. the permission mode. */
  chip?: ReactNode;
  /** An action shown before the chips, e.g. Stop. */
  lead?: ReactNode;
  /** Chips on the top line that put their text in the field. */
  asks?: Ask[];
  /** Text put in the box for you to edit and send; a new `n` puts it again. */
  seed?: { text: string; n: number };
  autoFocus?: boolean;
}) {
  const { toast } = useStore();
  const [text, setText] = useState("");
  const [busy, setBusy] = useState("");
  const talk = useHoldToTalk((heard) => setText((t) => (t && !/\s$/.test(t) ? t + " " : t) + heard), toast, !!busy || !!disabled);
  const recording = talk.state === "listening";
  const key = useRef(idem());
  const area = useRef<HTMLTextAreaElement>(null);
  const file = useRef<HTMLInputElement>(null);

  useEffect(() => {
    const el = area.current;
    if (!el) return;
    el.style.height = "auto";
    // An empty box stays one line: the placeholder is cut short rather than making it taller.
    if (text) el.style.height = `${Math.min(132, el.scrollHeight)}px`;
  }, [text]);
  const put = (t: string) => {
    setText(t);
    requestAnimationFrame(() => {
      const el = area.current;
      el?.focus();
      el?.setSelectionRange(t.length, t.length);
    });
  };
  useEffect(() => {
    if (seed) put(seed.text);
  }, [seed]);

  const submit = async () => {
    const body = text.trim();
    if (!body || busy || disabled) return;
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

  const note = recording ? (talk.tapped ? "Listening… tap the mic to stop" : "Listening… let go to stop") : talk.state === "hearing" ? "Turning that into text…" : busy ? busy + "…" : disabled ?? "";
  const off = !!disabled;
  return (
    <>
      {((asks && asks.length > 0) || lead) && !note ? (
        <div className="chips" role="group" aria-label="Ask Claude">
          {lead}
          {asks?.map((a) => (
            <button key={a.label} className="chip hit" disabled={off} onClick={() => put(a.text)}>{a.label}</button>
          ))}
          {chip}
        </div>
      ) : (
        (chip || note) && (
          <div className="dock-line">
            {chip}
            {note && <span className="clip" role="status">{note}</span>}
          </div>
        )
      )}
      <div className="compose">
        <div className={`input ${off ? "off" : ""}`}>
          <label className="sr" htmlFor={`say-${terminal}`}>Message to Claude</label>
          <textarea
            id={`say-${terminal}`}
            ref={area}
            rows={1}
            value={text}
            disabled={off}
            autoFocus={autoFocus}
            placeholder={placeholder}
            onChange={(e) => setText(e.target.value)}
            onPaste={(e) => {
              // A pasted screenshot goes to the laptop like a photo.
              const image = Array.from(e.clipboardData.files).find((f) => f.type.startsWith("image/"));
              if (image) {
                e.preventDefault();
                attach(image);
              }
            }}
          />
          <input ref={file} type="file" accept="image/*" hidden onChange={(e) => attach(e.target.files?.[0])} />
          <button className="attach" aria-label="Attach a photo" disabled={!!busy || off} onClick={() => file.current?.click()}>
            <ImagePlus size={18} />
          </button>
        </div>
        <button className={`round ${recording ? "rec" : ""}`} aria-label="Hold to talk" {...talk.bind}>
          {talk.state === "hearing" ? <span className="spin" /> : <Mic size={20} />}
        </button>
        <button className="round send" aria-label="Send" disabled={!!busy || off || !text.trim()} onClick={submit}>
          {busy === "Sending" ? <span className="spin" /> : <ArrowUp size={19} strokeWidth={2.2} />}
        </button>
      </div>
    </>
  );
}
