import { Terminal as XTerm } from "@xterm/xterm";
import { CornerDownLeft, Keyboard, Mic, Square } from "lucide-react";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { ApiError, api, openView, type Theme, type View } from "./api";
import { useStore } from "./store";
import { Dock, Switch } from "./ui";
import { useHoldToTalk } from "./voice";

const FONT_SIZE = 13;
// The second spelling only differs so xterm measures the font again once it has loaded.
const MONO = '"JetBrains Mono Variable", "JetBrains Mono", ui-monospace, monospace';
const MONO_LOADED = '"JetBrains Mono Variable", "JetBrains Mono", ui-monospace, Menlo, monospace';
const MIN_SCALE = 0.5;
const DROPPED = "The connection dropped.";
const PAUSED = "Paused while you watched other terminals.";

function xtermTheme(t: Theme) {
  const a = t.ansi;
  return {
    background: t.background, foreground: t.foreground, cursor: t.cursor, cursorAccent: t.background, selectionBackground: t.selection,
    black: a[0], red: a[1], green: a[2], yellow: a[3], blue: a[4], magenta: a[5], cyan: a[6], white: a[7],
    brightBlack: a[8], brightRed: a[9], brightGreen: a[10], brightYellow: a[11], brightBlue: a[12], brightMagenta: a[13], brightCyan: a[14], brightWhite: a[15],
  };
}

/**
 * What a key of the bar sends. The keys go the same way as typed text, in order with it: herdr-core holds keys pressed through its
 * API until three seconds after anyone stops typing in that terminal, which would make Enter after a typed command wait.
 */
function keyBytes(key: string, appCursor: boolean): string {
  const arrow = (c: string) => (appCursor ? `\x1bO${c}` : `\x1b[${c}`);
  switch (key) {
    case "esc":
      return "\x1b";
    case "tab":
      return "\t";
    case "shift+tab":
      return "\x1b[Z";
    case "enter":
      return "\r";
    case "up":
      return arrow("A");
    case "down":
      return arrow("B");
    case "right":
      return arrow("C");
    case "left":
      return arrow("D");
  }
  return "";
}

/** A character with Ctrl held: Ctrl C is \x03. Null for a character Ctrl does nothing to. */
function ctrlOf(ch: string): string | null {
  const c = ch.toLowerCase().charCodeAt(0);
  if (c >= 97 && c <= 122) return String.fromCharCode(c & 0x1f);
  if ("@[\\]^_".includes(ch)) return String.fromCharCode(ch.charCodeAt(0) & 0x1f);
  if (ch === " ") return "\x00";
  return null;
}

/**
 * What the phone's keyboard types sits in a small hidden field before the cursor, after a little padding. Phone keyboards guess words
 * as you type (they "compose"), so the terminal can't wait for key presses: each change of the field is sent as the difference, the
 * removed letters as Backspace and the new ones as typed, the moment it happens. A corrected word then comes out right, and Backspace
 * still works when the field has nothing of yours left in it.
 */
const PAD = "  ";

/**
 * A key of the key bar. It must not take the focus from the terminal, or the phone's keyboard would close at every tap: a touch acts
 * when the finger lifts and never becomes a click, and a touch that slid along the bar is a scroll, not a press.
 */
function press(act: () => void) {
  let from: number | null = null;
  return {
    onTouchStart: (e: React.TouchEvent) => {
      from = e.touches[0].clientX;
    },
    onTouchEnd: (e: React.TouchEvent) => {
      e.preventDefault();
      const moved = from === null ? 0 : Math.abs(e.changedTouches[0].clientX - from);
      from = null;
      if (moved < 10) act();
    },
    onMouseDown: (e: React.MouseEvent) => e.preventDefault(),
    onClick: act,
  };
}

/** The key bar's first row after Stop: the keys a phone keyboard lacks. */
const KEYS: { label: string; key: string; name?: string }[] = [
  { label: "Esc", key: "esc", name: "Escape" },
  { label: "Tab", key: "tab" },
  { label: "↑", key: "up", name: "Up" },
  { label: "↓", key: "down", name: "Down" },
  { label: "←", key: "left", name: "Left" },
  { label: "→", key: "right", name: "Right" },
];

const DIRECT = "gwen.typeDirect";
/** Whether you type straight into terminals (for vim and the like) rather than a line at a time; the phone remembers it. */
function directKept(): boolean {
  try {
    return localStorage.getItem(DIRECT) === "1";
  } catch {
    return false;
  }
}
function keepDirect(on: boolean) {
  try {
    localStorage.setItem(DIRECT, on ? "1" : "0");
  } catch {
    // storage is off: the choice lasts until the page closes
  }
}

/** Where this terminal is among the panes of its tab, for "Pane 1 of 2" and the tab map. */
export interface PaneSpot { at: number; of: number; onMap: () => void }

/**
 * The live terminal, drawn at the laptop's grid size and fitted to the phone's width, then the key bar and the line you type at the
 * bottom (the phone redo, P4). What you type in the line goes when you press Enter, so the phone's keyboard and autocorrect can't
 * garble it; Type directly sends every key at once instead, for vim and the like. The phone only watches and types through the API:
 * it never becomes the viewer whose size the terminal follows, so the laptop's tab keeps its size (plan.md 3.14.7). The terminal
 * keeps the laptop theme's colours; the chrome around it does not.
 */
export function TerminalView({ terminal, pane, bar }: { terminal: string; pane?: PaneSpot; bar?: ReactNode }) {
  const { spaces, toast } = useStore();
  const theme = spaces?.theme;
  const area = useRef<HTMLDivElement>(null);
  const wrap = useRef<HTMLDivElement>(null);
  const holder = useRef<HTMLDivElement>(null);
  const term = useRef<XTerm | null>(null);
  const view = useRef<View | null>(null);
  const ctrl = useRef(false);
  const alt = useRef(false);
  const field = useRef<HTMLInputElement>(null);
  const lineField = useRef<HTMLInputElement>(null);
  const photo = useRef<HTMLInputElement>(null);
  const [direct, setDirect] = useState(directKept);
  const [line, setLine] = useState("");
  const [altOn, setAltOn] = useState(false);
  const [busy, setBusy] = useState(false);
  const talk = useHoldToTalk((heard) => setLine((l) => (l && !/\s$/.test(l) ? l + " " : l) + heard), toast);
  const was = useRef(PAD);
  const composing = useRef(false);
  const zoom = useRef(1);
  const [ctrlOn, setCtrlOn] = useState(false);
  const [note, setNote] = useState("Opening…");
  const [attempt, setAttempt] = useState(0);
  const [hint, setHint] = useState(true);

  const setCtrl = (on: boolean) => {
    ctrl.current = on;
    setCtrlOn(on);
  };
  const setAlt = (on: boolean) => {
    alt.current = on;
    setAltOn(on);
  };

  /** Scales the grid to the width of the screen; pinching zooms in from there. */
  const fit = () => {
    const el = holder.current, box = wrap.current, t = term.current;
    if (!el || !box || !t) return;
    const natural = t.element?.querySelector<HTMLElement>(".xterm-screen");
    if (!natural || !natural.offsetWidth) return;
    const width = natural.offsetWidth + 12, height = natural.offsetHeight + 8;
    // Fit the width, but never so small that the text can't be read: a very wide tab scrolls sideways instead.
    const scale = Math.max(MIN_SCALE, Math.min(1, box.clientWidth / width)) * zoom.current;
    el.style.transform = `scale(${scale})`;
    el.style.width = `${width}px`;
    el.style.height = `${height}px`;
    // The scaled box still takes its natural room in the page; pull the margins in so scrolling ends where the grid does.
    el.style.marginRight = `${width * (scale - 1)}px`;
    el.style.marginBottom = `${height * (scale - 1)}px`;
  };

  useEffect(() => {
    if (!holder.current) return;
    const t = new XTerm({
      fontFamily: MONO, fontSize: FONT_SIZE, lineHeight: 1.15, cursorBlink: false, disableStdin: true,
      scrollback: 2000, allowProposedApi: false, convertEol: false, theme: theme ? xtermTheme(theme) : undefined,
    });
    term.current = t;
    t.open(holder.current);
    // Typing goes through the field below; xterm's own text box must not bring up a keyboard of its own.
    t.textarea?.setAttribute("inputmode", "none");
    t.textarea?.setAttribute("tabindex", "-1");
    document.fonts?.load(`${FONT_SIZE}px "JetBrains Mono Variable"`).then(() => {
      if (term.current !== t) return;
      t.options.fontFamily = MONO_LOADED;
      requestAnimationFrame(fit);
    }, () => {});
    setNote("Opening…");
    const v = openView(terminal, {
      data: (bytes) => {
        setNote("");
        t.write(bytes, () => document.activeElement === field.current && showCursor());
      },
      size: (cols, rows) => {
        t.resize(cols, rows);
        requestAnimationFrame(fit);
      },
      // "closed": Gwen let this view go because the phone opened other terminals since; the terminal itself still runs.
      closed: (reason) => setNote(reason === "lost" ? DROPPED : reason === "exited" || reason === "killed" ? "This terminal has ended." : reason === "closed" ? PAUSED : "This terminal closed."),
    });
    view.current = v;
    const ro = new ResizeObserver(() => {
      fit();
      // The keyboard coming up shrinks the terminal: keep the line being typed in view.
      if (document.activeElement === field.current) showCursor();
    });
    if (wrap.current) ro.observe(wrap.current);
    return () => {
      ro.disconnect();
      v.close();
      t.dispose();
      term.current = null;
      view.current = null;
    };
  }, [terminal, attempt]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (theme && term.current) term.current.options.theme = xtermTheme(theme);
    if (theme) area.current?.style.setProperty("--term-bg", theme.background);
  }, [theme]);

  // Pinch to zoom: two fingers change the scale; one finger scrolls.
  const pinch = useRef<{ d: number; z: number } | null>(null);
  const dist = (e: React.TouchEvent) => Math.hypot(e.touches[0].clientX - e.touches[1].clientX, e.touches[0].clientY - e.touches[1].clientY);

  /** Scrolls the terminal so the cursor's line shows, when it is out of view. */
  const showCursor = () => {
    const t = term.current, box = wrap.current, el = holder.current;
    if (!t || !box || !el || !t.rows) return;
    const scale = el.getBoundingClientRect().height / Math.max(1, el.offsetHeight);
    const line = (el.offsetHeight / t.rows) * scale;
    const y = (t.buffer.active.cursorY + 1) * line;
    if (y > box.scrollTop + box.clientHeight || y - line < box.scrollTop) box.scrollTop = Math.max(0, y - box.clientHeight + line * 2);
  };

  /** Sends text as typed, with a held Ctrl or Alt applied to its first character. */
  const type = (text: string) => {
    if (!text) return;
    if (ctrl.current) {
      setCtrl(false);
      const c = ctrlOf(text[0]);
      if (c !== null) {
        view.current?.text(c);
        text = text.slice(1);
        // The field no longer matches the line: start it again.
        reset();
      }
    } else if (alt.current) {
      setAlt(false);
      view.current?.text("\x1b" + text[0]);
      text = text.slice(1);
      reset();
    }
    if (text) view.current?.text(text.replace(/\n/g, "\r"));
  };

  const reset = () => {
    const f = field.current;
    if (f && f.value !== PAD) f.value = PAD;
    was.current = PAD;
  };

  /** The field changed: send what was removed as Backspace and what was added as typed. */
  const changed = () => {
    const f = field.current;
    if (!f) return;
    const now = f.value, before = was.current;
    let same = 0;
    while (same < now.length && same < before.length && now[same] === before[same]) same++;
    was.current = now;
    const removed = before.length - same;
    if (removed > 0) view.current?.text("\x7f".repeat(removed));
    type(now.slice(same));
    // Refill the padding, or start over when the field grows long, but never in the middle of a word being composed.
    if (!composing.current && (now.length < PAD.length || now.length > 60 || !now.startsWith(PAD))) reset();
  };

  /** Keys that never change the field: Enter, and those of a real keyboard (arrows, Esc, Tab, Ctrl+letter). */
  const keyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    const t = term.current;
    if (e.nativeEvent.isComposing || e.keyCode === 229 || !t) return;
    let send = "";
    if (e.ctrlKey && !e.altKey && !e.metaKey && e.key.length === 1) send = ctrlOf(e.key) ?? "";
    else if (e.key === "Enter") send = "\r";
    else {
      const named: Record<string, string> = { Escape: "esc", Tab: e.shiftKey ? "shift+tab" : "tab", ArrowUp: "up", ArrowDown: "down", ArrowLeft: "left", ArrowRight: "right" };
      if (named[e.key]) send = keyBytes(named[e.key], t.modes.applicationCursorKeysMode);
    }
    if (!send) return;
    e.preventDefault();
    setCtrl(false);
    view.current?.text(send);
    reset();
  };

  const focusField = () => {
    setHint(false);
    // A line at a time: the tap brings up the keyboard on the line, not on the terminal.
    if (!direct) return lineField.current?.focus({ preventScroll: true });
    const f = field.current;
    if (!f) return;
    if (document.activeElement !== f) {
      reset();
      f.focus({ preventScroll: true });
    }
    requestAnimationFrame(showCursor);
  };

  const dropped = note === DROPPED || note === PAUSED;
  const key = (k: string) => {
    const t = term.current;
    if (t) view.current?.text(keyBytes(k, t.modes.applicationCursorKeysMode));
    setCtrl(false);
    setAlt(false);
    reset();
  };
  const stop = () => {
    view.current?.text("\x03");
    setCtrl(false);
    setAlt(false);
    reset();
  };
  /** The line goes as typed, then Enter a moment later, so a program that reads fast input as a paste still gets Enter as Enter. */
  const sendLine = () => {
    const v = view.current;
    if (!v || note) return toast(dropped ? "Not connected. Tap Reconnect first." : "The terminal isn't open yet.");
    if (line) v.text(line);
    window.setTimeout(() => v.text("\r"), line ? 40 : 0);
    setLine("");
  };
  /** In the line, a held Ctrl or Alt takes the next letter you type and sends it at once (Ctrl then c stops what runs). */
  const lineChanged = (next: string) => {
    if ((ctrl.current || alt.current) && next.length === line.length + 1 && next.startsWith(line)) {
      const ch = next.slice(line.length);
      if (ctrl.current) {
        const c = ctrlOf(ch);
        if (c !== null) view.current?.text(c);
      } else view.current?.text("\x1b" + ch);
      setCtrl(false);
      setAlt(false);
      return;
    }
    setLine(next);
  };
  const paste = async () => {
    let text = "";
    try {
      text = (await navigator.clipboard?.readText()) ?? "";
    } catch {
      // the phone may keep its clipboard to itself
    }
    if (!text) return toast(direct ? "The phone didn't share its clipboard." : "The phone didn't share its clipboard. Long-press the line to paste.");
    if (direct) view.current?.text(text.replace(/\n/g, "\r"));
    else setLine((l) => l + text.replace(/\s*\n\s*/g, " "));
  };
  const attach = async (f: File | undefined) => {
    if (!f) return;
    if (f.size > 8 * 1024 * 1024) return toast("That file is too large; 8 MB at most.");
    setBusy(true);
    try {
      const { path } = await api.put(terminal, f);
      if (direct) view.current?.text(path + " ");
      else setLine((l) => (l && !l.endsWith(" ") ? l + " " : l) + path + " ");
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "The photo didn't go through.");
    } finally {
      setBusy(false);
      if (photo.current) photo.current.value = "";
    }
  };
  const switchDirect = (on: boolean) => {
    setDirect(on);
    keepDirect(on);
    setCtrl(false);
    setAlt(false);
  };
  return (
    <>
      <div className={`term-area ${pane && pane.of > 1 ? "pills" : ""}`} ref={area}>
        <div
          className="term-wrap"
          ref={wrap}
          onTouchStart={(e) => {
            setHint(false);
            if (e.touches.length === 2) pinch.current = { d: dist(e), z: zoom.current };
          }}
          onTouchMove={(e) => {
            if (e.touches.length !== 2 || !pinch.current) return;
            zoom.current = Math.max(1, Math.min(4, (pinch.current.z * dist(e)) / pinch.current.d));
            fit();
          }}
          onTouchEnd={() => (pinch.current = null)}
          // Tap to type: a tap anywhere on the terminal brings up the phone's keyboard.
          onClick={focusField}
        >
          <div className="term-scale" ref={holder} />
        </div>
        <input
          ref={field}
          className="term-field"
          aria-label="Type into the terminal"
          defaultValue={PAD}
          autoComplete="off"
          autoCorrect="off"
          autoCapitalize="none"
          spellCheck={false}
          enterKeyHint="enter"
          onInput={changed}
          onKeyDown={keyDown}
          onCompositionStart={() => (composing.current = true)}
          onCompositionEnd={() => {
            composing.current = false;
            changed();
          }}
          onBlur={reset}
        />
        {pane && pane.of > 1 && (
          <div className="term-pills">
            <span>Pane {pane.at} of {pane.of} · swipe the header</span>
            <button className="hit" onClick={pane.onMap}>Tab map</button>
          </div>
        )}
        {note ? (
          <div className="term-note" style={dropped ? { pointerEvents: "auto" } : undefined}>
            <div className="col" style={{ gap: 12, alignItems: "center" }}>
              <span>{note}</span>
              {dropped && (
                <button className="btn sm" onClick={() => setAttempt((n) => n + 1)}>
                  Reconnect
                </button>
              )}
            </div>
          </div>
        ) : (
          hint && <span className="term-hint">{direct ? "Tap to type straight into it" : "Type below, Enter sends the line"} · pinch to zoom</span>
        )}
      </div>
      <Dock className="term">
        {bar}
        <div className="keys" role="toolbar" aria-label="Keys">
          <button className="stop" aria-label="Stop: Control C" {...press(stop)}>
            <Square size={11} fill="currentColor" strokeWidth={0} aria-hidden /> Stop
          </button>
          {KEYS.map((k) => (
            <button key={k.key} aria-label={k.name} {...press(() => key(k.key))}>
              {k.label}
            </button>
          ))}
        </div>
        <div className="keys second" role="toolbar" aria-label="More keys">
          <button aria-pressed={ctrlOn} {...press(() => (setAlt(false), setCtrl(!ctrl.current)))}>Ctrl</button>
          <button aria-pressed={altOn} {...press(() => (setCtrl(false), setAlt(!alt.current)))}>Alt</button>
          <button className="word" {...press(paste)}>Paste</button>
          <button className="word" disabled={busy} {...press(() => photo.current?.click())}>
            {busy ? <span className="spin sm" /> : "Photo"}
          </button>
          <span className="grow" />
          <label className="direct">
            <Switch on={direct} onChange={switchDirect} label="Type directly" />
            <span aria-hidden>Type directly</span>
          </label>
        </div>
        <input ref={photo} type="file" accept="image/*" hidden onChange={(e) => attach(e.target.files?.[0])} />
        {direct ? (
          <div className="t-line">
            <button className="t-direct" onClick={focusField}>
              <Keyboard size={18} aria-hidden />
              <span className="clip">{talk.state === "listening" ? "Listening…" : "Every key goes straight to the terminal"}</span>
            </button>
          </div>
        ) : (
          <form
            className="t-line"
            onSubmit={(e) => {
              e.preventDefault();
              sendLine();
            }}
          >
            <label className="t-input">
              <span className="mono" aria-hidden>$</span>
              <span className="sr">A line for the terminal</span>
              <input
                ref={lineField}
                value={line}
                onChange={(e) => lineChanged(e.target.value)}
                placeholder={
                  talk.state === "listening" ? (talk.tapped ? "Listening… tap the mic to stop" : "Listening… let go to stop")
                  : talk.state === "hearing" ? "Turning that into text…"
                  : ctrlOn ? "Ctrl is held: type a letter"
                  : altOn ? "Alt is held: type a key"
                  : "Type a line, Enter sends it"
                }
                autoComplete="off"
                autoCapitalize="none"
                spellCheck={false}
                enterKeyHint="send"
              />
            </label>
            <button type="button" className={`t-btn ${talk.state === "listening" ? "rec" : ""}`} aria-label="Hold to talk" {...talk.bind}>
              {talk.state === "hearing" ? <span className="spin" /> : <Mic size={20} />}
            </button>
            <button type="submit" className="t-btn send" aria-label={line ? "Send the line" : "Press Enter"}>
              <CornerDownLeft size={19} strokeWidth={2.2} aria-hidden />
            </button>
          </form>
        )}
      </Dock>
    </>
  );
}
