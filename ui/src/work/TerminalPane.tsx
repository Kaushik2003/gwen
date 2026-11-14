import "@fontsource-variable/geist-mono";
import "@xterm/xterm/css/xterm.css";
import { Terminal as XTerm, type ITheme } from "@xterm/xterm";
import { useEffect, useRef, useState } from "react";
import { apiError } from "../api";
import { Button } from "../components/ui";
import * as Host from "../wailsjs/go/main/App";
import { ClipboardGetText, ClipboardSetText, EventsOn } from "../wailsjs/runtime/runtime";

// One terminal of Gwen's herdr-core, drawn in the Gwen window (plan.md 3.15). The pane attaches as a viewer that may type: output
// comes as `term:data:<view>` events (base64), typing and the grid go back through TermInput and TermResize. The grid follows the
// pane's size; while another viewer (Gwen Terminal) is the active one, the pane draws at that viewer's grid until it types or gets
// the focus again.

const FONT_FAMILY = '"Geist Mono Variable", "Geist Mono", ui-monospace, monospace';
const FONT_SIZE = 13;
const SCROLLBACK = 5000;
const RESIZE_DELAY = 60;

/** A theme's terminal colours: the window's canvas and ink, its accent as the cursor, its strong line colour as the selection, and
 * the 16 colours of its palette (term/themes/*.json, plan.md 5.2). */
const palettes: Record<string, { background: string; foreground: string; cursor: string; line: string; ansi: string[] }> = {
  ghost: {
    background: "#0d1012", foreground: "#f2f1ed", cursor: "#d72638", line: "60, 70, 77",
    ansi: ["#232a2e", "#e5384a", "#4fb88a", "#f2b33d", "#4d9bdb", "#c58ad6", "#5cc8c0", "#afa2a0", "#696160", "#ec6a77", "#7bcaa7", "#f5c66e", "#7ab4e4", "#d4a7e0", "#85d6d0", "#f2f1ed"],
  },
  tide: {
    background: "#0b0f14", foreground: "#e9eff5", cursor: "#3ec1af", line: "56, 70, 86",
    ansi: ["#1e2833", "#ef6b6b", "#86c66a", "#e9b65a", "#7fa9ee", "#b99ae8", "#62d5c5", "#b6c2ce", "#5e6c7a", "#f39090", "#a4d48f", "#eec883", "#9fbef2", "#cab3ee", "#89e0d4", "#e9eff5"],
  },
  ember: {
    background: "#110f0d", foreground: "#f4efe9", cursor: "#e9935a", line: "71, 63, 55",
    ansi: ["#28231e", "#f0656a", "#6cc58c", "#e6d16a", "#79b2e6", "#d59ad0", "#7ccfc4", "#cdc4b9", "#70665d", "#f48c8f", "#91d4a9", "#ecdc8f", "#9ac5ec", "#e0b3dc", "#9ddbd3", "#f4efe9"],
  },
};

function palette() {
  return palettes[document.documentElement.dataset.theme ?? ""] ?? palettes.ghost;
}

function xtermTheme(): ITheme {
  const p = palette(), a = p.ansi;
  return {
    background: p.background, foreground: p.foreground, cursor: p.cursor, cursorAccent: p.background,
    selectionBackground: `rgba(${p.line}, 0.85)`, selectionInactiveBackground: `rgba(${p.line}, 0.5)`,
    black: a[0], red: a[1], green: a[2], yellow: a[3], blue: a[4], magenta: a[5], cyan: a[6], white: a[7],
    brightBlack: a[8], brightRed: a[9], brightGreen: a[10], brightYellow: a[11], brightBlue: a[12], brightMagenta: a[13], brightCyan: a[14], brightWhite: a[15],
  };
}

/** The scrollbar of the terminal's scrollback, thin and in the theme's line colour. */
const CSS = `
.gwen-term .xterm { height: 100%; }
.gwen-term .xterm-viewport { scrollbar-width: thin; scrollbar-color: var(--color-line-strong) transparent; }
.gwen-term .xterm-viewport::-webkit-scrollbar { width: 8px; }
.gwen-term .xterm-viewport::-webkit-scrollbar-thumb { background: var(--color-line-strong); border-radius: 4px; }
.gwen-term .xterm-viewport::-webkit-scrollbar-track { background: transparent; }
`;

function decode(b64: string): Uint8Array {
  const s = atob(b64);
  const out = new Uint8Array(s.length);
  for (let i = 0; i < s.length; i++) out[i] = s.charCodeAt(i);
  return out;
}

const utf8 = new TextEncoder();

/** Typed text as a binary string (one char per UTF-8 byte), the form btoa takes. */
function binary(text: string): string {
  const b = utf8.encode(text);
  let out = "";
  for (let i = 0; i < b.length; i += 0x8000) out += String.fromCharCode(...b.subarray(i, i + 0x8000));
  return out;
}

type Grid = { cols: number; rows: number };

/** The grid that fits in el at the terminal's cell size, or null while el has no size (a hidden tab). */
function gridFor(t: XTerm, el: HTMLElement): Grid | null {
  // xterm keeps the cell size in its render service, as its fit addon reads it; the screen's own size is the fallback.
  const cell = (t as unknown as { _core?: { _renderService?: { dimensions?: { css?: { cell?: { width: number; height: number } } } } } })._core?._renderService?.dimensions?.css?.cell;
  let w = cell?.width ?? 0, h = cell?.height ?? 0;
  if (!(w > 0 && h > 0)) {
    const screen = t.element?.querySelector<HTMLElement>(".xterm-screen");
    if (!screen?.offsetWidth || !t.cols || !t.rows) return null;
    w = screen.offsetWidth / t.cols;
    h = screen.offsetHeight / t.rows;
  }
  const viewport = t.element?.querySelector<HTMLElement>(".xterm-viewport");
  const scrollbar = viewport ? viewport.offsetWidth - viewport.clientWidth : 0;
  const style = getComputedStyle(el);
  const width = el.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight) - scrollbar;
  const height = el.clientHeight - parseFloat(style.paddingTop) - parseFloat(style.paddingBottom);
  if (width <= 0 || height <= 0) return null;
  return { cols: Math.max(2, Math.floor(width / w)), rows: Math.max(1, Math.floor(height / h)) };
}

type Op = { kind: "input"; data: string } | { kind: "resize"; cols: number; rows: number };

/**
 * What goes back to the view, one call at a time: Wails runs each call on its own goroutine, so calls sent side by side could
 * arrive out of order. Typing that piles up while a call is out goes in the next one, and only the last grid counts.
 */
function outbox() {
  let view: string | null = null;
  let busy = false;
  let grid: Grid | null = null; // the last grid sent or queued
  const queue: Op[] = [];
  const pump = async () => {
    if (busy) return;
    busy = true;
    while (view && queue.length) {
      const op = queue.shift()!, v = view;
      try {
        if (op.kind === "input") await Host.TermInput(v, btoa(op.data));
        else await Host.TermResize(v, op.cols, op.rows);
      } catch {
        // The view closed; term:closed says so.
      }
    }
    busy = false;
  };
  return {
    /** The view is open and the page listens: its grid goes first, which lets its output through. */
    open(v: string, g: Grid) {
      view = v;
      grid = g;
      queue.unshift({ kind: "resize", ...g });
      void pump();
    },
    input(data: string) {
      const last = queue[queue.length - 1];
      if (last?.kind === "input") last.data += data;
      else queue.push({ kind: "input", data });
      void pump();
    },
    /** Sends a grid unless it is the one already sent; force sends it anyway, to take the terminal's grid back. */
    resize(g: Grid, force = false) {
      if (!force && grid && grid.cols === g.cols && grid.rows === g.rows) return;
      grid = g;
      const last = queue[queue.length - 1];
      if (last?.kind === "resize") Object.assign(last, g);
      else queue.push({ kind: "resize", ...g });
      void pump();
    },
    close() {
      view = null;
      queue.length = 0;
    },
  };
}

async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text);
  } catch {
    await ClipboardSetText(text).catch(() => false);
  }
}

/** The desktop's clipboard straight from the host, which never shows WebKit's paste prompt; the browser's as the fallback. */
async function pasteText(): Promise<string> {
  try {
    return await ClipboardGetText();
  } catch {
    return navigator.clipboard.readText().catch(() => "");
  }
}

type Closed = { kind: "ended" | "lost"; note?: string };

export default function TerminalPane({ terminal, focused, onClosed }: { terminal: string; focused: boolean; onClosed?: (reason: string) => void }) {
  const holder = useRef<HTMLDivElement>(null);
  const term = useRef<XTerm | null>(null);
  const onClosedRef = useRef(onClosed);
  onClosedRef.current = onClosed;
  const [closed, setClosed] = useState<Closed | null>(null);
  const [attempt, setAttempt] = useState(0);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    const el = holder.current;
    if (!el) return;
    let disposed = false;
    let view: string | null = null;
    let attaching = false;
    let over = false; // the view closed: only Reconnect attaches again
    let fitted: Grid | null = null;
    let timer = 0;
    let offs: (() => void)[] = [];
    let t: XTerm | null = null;
    let ro: ResizeObserver | null = null;
    let themes: MutationObserver | null = null;
    const out = outbox();
    setClosed(null);

    const close = (c: Closed, reason: string) => {
      view = null;
      over = true;
      out.close();
      offs.forEach((off) => off());
      offs = [];
      setClosed(c);
      onClosedRef.current?.(reason);
    };

    const attach = async (x: XTerm, g: Grid) => {
      attaching = true;
      try {
        const id = await Host.TermAttach(terminal, g.cols, g.rows);
        if (disposed) {
          void Host.TermDetach(id);
          return;
        }
        view = id;
        offs.push(
          EventsOn(`term:data:${id}`, (b64: string) => x.write(decode(b64))),
          EventsOn(`term:size:${id}`, (s: Grid) => {
            if (s?.cols > 0 && s?.rows > 0 && (s.cols !== x.cols || s.rows !== x.rows)) x.resize(s.cols, s.rows);
          }),
          EventsOn(`term:closed:${id}`, (c: { reason?: string }) => {
            const reason = c?.reason ?? "lost";
            close({ kind: reason === "exited" || reason === "killed" ? "ended" : "lost" }, reason);
          }),
        );
        // The grid may have changed while attaching.
        out.open(id, fitted ?? g);
      } catch (e) {
        if (disposed) return;
        const err = apiError(e);
        if (err.code === "not_found") close({ kind: "ended" }, "exited");
        else close({ kind: "lost", note: err.message }, "lost");
      } finally {
        attaching = false;
      }
    };

    /** Fits the grid to the pane: the first time it attaches, after that it tells the view. */
    const fit = () => {
      if (!t || disposed) return;
      const g = gridFor(t, el);
      if (!g) return;
      fitted = g;
      if (over) return;
      // Drawn at another grid (its own before, or another viewer's): the terminal must follow this one again.
      const moved = g.cols !== t.cols || g.rows !== t.rows;
      if (moved) t.resize(g.cols, g.rows);
      if (view) out.resize(g, moved);
      else if (!attaching) void attach(t, g);
    };

    void (async () => {
      // xterm measures its cell once it opens, so the font must be there first.
      await document.fonts.load(`${FONT_SIZE}px "Geist Mono Variable"`).catch(() => []);
      if (disposed) return;
      const x = new XTerm({
        fontFamily: FONT_FAMILY, fontSize: FONT_SIZE, lineHeight: 1, scrollback: SCROLLBACK, cursorBlink: false, allowProposedApi: false,
        convertEol: false, theme: xtermTheme(),
      });
      t = x;
      term.current = x;
      x.open(el);
      x.attachCustomKeyEventHandler((ev) => {
        if (ev.type !== "keydown") return true;
        const key = ev.key.toLowerCase();
        if (ev.ctrlKey && ev.shiftKey && !ev.altKey && key === "c") {
          ev.preventDefault();
          const selection = x.getSelection();
          if (selection) void copyText(selection);
          return false;
        }
        if (ev.ctrlKey && ev.shiftKey && !ev.altKey && key === "v") {
          ev.preventDefault();
          void pasteText().then((text) => text && x.paste(text));
          return false;
        }
        // The window's own keys go on to it: Ctrl+Tab and Ctrl+Shift+Tab cycle tabs, Ctrl+1 to 9 jump.
        if (ev.ctrlKey && !ev.altKey && (ev.key === "Tab" || (!ev.shiftKey && /^[1-9]$/.test(ev.key)))) return false;
        return true;
      });
      // xterm follows the modes the program set, so what it hands over is already what a real terminal would send. Its own
      // listeners go with it when it is disposed.
      x.onData((data) => out.input(binary(data)));
      x.onBinary((data) => out.input(data));
      // Back in front: draw at this pane's own grid again, and make it the grid the terminal follows.
      x.textarea?.addEventListener("focus", () => {
        if (fitted && view && (fitted.cols !== x.cols || fitted.rows !== x.rows)) {
          x.resize(fitted.cols, fitted.rows);
          out.resize(fitted, true);
        }
      });
      themes = new MutationObserver(() => (x.options.theme = xtermTheme()));
      themes.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
      ro = new ResizeObserver(() => {
        window.clearTimeout(timer);
        timer = window.setTimeout(fit, view ? RESIZE_DELAY : 0);
      });
      ro.observe(el);
      fit();
      setReady(true);
    })();

    return () => {
      disposed = true;
      window.clearTimeout(timer);
      ro?.disconnect();
      themes?.disconnect();
      offs.forEach((off) => off());
      out.close();
      if (view) void Host.TermDetach(view);
      t?.dispose();
      term.current = null;
      setReady(false);
    };
  }, [terminal, attempt]);

  useEffect(() => {
    if (focused && ready) term.current?.focus();
  }, [focused, ready]);

  return (
    <div className="gwen-term relative h-full w-full overflow-hidden bg-canvas">
      <style href="gwen-term" precedence="medium">
        {CSS}
      </style>
      <div ref={holder} className="absolute inset-0 pt-1.5 pl-2.5" />
      {closed && (
        <div className="absolute inset-0 flex items-center justify-center bg-canvas/75">
          <div className="flex max-w-sm flex-col items-center gap-3 px-6 text-center">
            <span className="text-sm text-ink-muted">{closed.kind === "ended" ? "This terminal ended" : "Lost the terminal server"}</span>
            {closed.kind === "lost" && closed.note && <span className="text-[13px] text-ink-subtle">{closed.note}</span>}
            {closed.kind === "lost" && (
              <Button size="sm" onClick={() => setAttempt((n) => n + 1)}>
                Reconnect
              </Button>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
