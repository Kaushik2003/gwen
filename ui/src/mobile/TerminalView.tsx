import { Terminal as XTerm } from "@xterm/xterm";
import { ArrowDown, ArrowLeft, ArrowRight, ArrowUp, Keyboard } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { openView, type Theme, type View } from "./api";
import { useStore } from "./store";

const FONT_SIZE = 13;
const MIN_SCALE = 0.5;

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

/** The key bar above the keyboard, as in Herdr's phone app: the keys a phone keyboard lacks. */
const KEYS: { label: string; key: string; icon?: typeof ArrowUp }[] = [
  { label: "Esc", key: "esc" },
  { label: "Tab", key: "tab" },
  { label: "⇧Tab", key: "shift+tab" },
  { label: "Left", key: "left", icon: ArrowLeft },
  { label: "Up", key: "up", icon: ArrowUp },
  { label: "Down", key: "down", icon: ArrowDown },
  { label: "Right", key: "right", icon: ArrowRight },
  { label: "Enter", key: "enter" },
];

/**
 * The live terminal, drawn at the laptop's grid size and fitted to the phone's width. The phone only watches and types through the
 * API: it never becomes the viewer whose size the terminal follows, so the laptop's tab keeps its size (plan.md 3.14.7).
 */
export function TerminalView({ terminal }: { terminal: string }) {
  const { spaces } = useStore();
  const theme = spaces?.theme;
  const wrap = useRef<HTMLDivElement>(null);
  const holder = useRef<HTMLDivElement>(null);
  const term = useRef<XTerm | null>(null);
  const view = useRef<View | null>(null);
  const ctrl = useRef(false);
  const zoom = useRef(1);
  const [ctrlOn, setCtrlOn] = useState(false);
  const [note, setNote] = useState("Opening…");
  const [attempt, setAttempt] = useState(0);

  const setCtrl = (on: boolean) => {
    ctrl.current = on;
    setCtrlOn(on);
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
      fontFamily: '"Geist Mono Variable", "Geist Mono", ui-monospace, Menlo, monospace', fontSize: FONT_SIZE, lineHeight: 1.15, cursorBlink: false,
      scrollback: 2000, allowProposedApi: false, convertEol: false, theme: theme ? xtermTheme(theme) : undefined,
    });
    term.current = t;
    t.open(holder.current);
    setNote("Opening…");
    const v = openView(terminal, {
      data: (bytes) => {
        setNote("");
        t.write(bytes);
      },
      size: (cols, rows) => {
        t.resize(cols, rows);
        requestAnimationFrame(fit);
      },
      closed: (reason) => setNote(reason === "lost" ? "The connection dropped." : reason === "exited" || reason === "killed" ? "This terminal has ended." : "This terminal closed."),
    });
    view.current = v;
    // xterm follows the modes the program set, so what it hands over is already what a real terminal would send.
    const typed = t.onData((data) => {
      if (ctrl.current && data.length === 1) {
        const c = data.toLowerCase().charCodeAt(0);
        setCtrl(false);
        if (c >= 97 && c <= 122) return v.text(String.fromCharCode(c & 0x1f));
      }
      v.text(data);
    });
    const ro = new ResizeObserver(fit);
    if (wrap.current) ro.observe(wrap.current);
    return () => {
      ro.disconnect();
      typed.dispose();
      v.close();
      t.dispose();
      term.current = null;
      view.current = null;
    };
  }, [terminal, attempt]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (theme && term.current) term.current.options.theme = xtermTheme(theme);
    if (theme) wrap.current?.style.setProperty("--term-bg", theme.background);
  }, [theme]);

  // Pinch to zoom: two fingers change the scale; one finger scrolls.
  const pinch = useRef<{ d: number; z: number } | null>(null);
  const dist = (e: React.TouchEvent) => Math.hypot(e.touches[0].clientX - e.touches[1].clientX, e.touches[0].clientY - e.touches[1].clientY);

  const dropped = note === "The connection dropped.";
  return (
    <>
      <div
        className="term-wrap"
        ref={wrap}
        onTouchStart={(e) => {
          if (e.touches.length === 2) pinch.current = { d: dist(e), z: zoom.current };
        }}
        onTouchMove={(e) => {
          if (e.touches.length !== 2 || !pinch.current) return;
          zoom.current = Math.max(1, Math.min(4, (pinch.current.z * dist(e)) / pinch.current.d));
          fit();
        }}
        onTouchEnd={() => (pinch.current = null)}
      >
        <div className="term-scale" ref={holder} />
        {note && (
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
        )}
      </div>
      <div className="keys" role="toolbar" aria-label="Keys">
        <button aria-label="Show the keyboard" onClick={() => term.current?.focus()}>
          <Keyboard size={16} />
        </button>
        <button className={ctrlOn ? "on" : ""} aria-pressed={ctrlOn} {...press(() => setCtrl(!ctrl.current))}>
          Ctrl
        </button>
        {KEYS.map((k) => (
          <button
            key={k.key}
            aria-label={k.label}
            {...press(() => {
              const t = term.current;
              if (t) view.current?.text(keyBytes(k.key, t.modes.applicationCursorKeysMode));
              setCtrl(false);
            })}
          >
            {k.icon ? <k.icon size={15} /> : k.label}
          </button>
        ))}
      </div>
    </>
  );
}
