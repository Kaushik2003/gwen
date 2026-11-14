import { useEffect, useRef, type ReactNode } from "react";
import type { Session, SessionState } from "./api";

/** "2m", "3h", "Tue": how long ago, short enough for a pill. */
export function ago(ms: number, now = Date.now()): string {
  const s = Math.max(0, Math.round((now - ms) / 1000));
  if (s < 45) return "now";
  if (s < 3600) return `${Math.round(s / 60)}m`;
  if (s < 86400) return `${Math.round(s / 3600)}h`;
  return new Date(ms).toLocaleDateString(undefined, { weekday: "short" });
}

/** "just now", "12 min ago", "yesterday", "3 days ago": how long ago, in a sentence. */
export function agoWords(ms: number, now = Date.now()): string {
  const s = Math.max(0, Math.round((now - ms) / 1000));
  if (s < 45) return "just now";
  if (s < 3600) return `${Math.round(s / 60)} min ago`;
  const day = (t: number) => Math.floor((t - new Date(t).getTimezoneOffset() * 60_000) / 86_400_000);
  const days = day(now) - day(ms);
  if (days === 0) return `${Math.round(s / 3600)}h ago`;
  if (days === 1) return "yesterday";
  if (days < 7) return `${days} days ago`;
  if (days < 14) return "last week";
  return new Date(ms).toLocaleDateString(undefined, { day: "numeric", month: "short" });
}

/** "4m", "2h", "3d": how long something has lasted, for "Working · 4m" and "idle 2h". */
export function lasted(ms: number, now = Date.now()): string {
  const m = Math.max(0, Math.round((now - ms) / 60_000));
  if (m < 1) return "now";
  if (m < 60) return `${m}m`;
  if (m < 60 * 24) return `${Math.round(m / 60)}h`;
  return `${Math.round(m / 60 / 24)}d`;
}

/** The time of day, as the phone writes it. */
export function clock(ms: number): string {
  return new Date(ms).toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" });
}

export const STATE_WORDS: Record<SessionState, string> = {
  starting: "Starting", working: "Working", needs_you: "Needs you", done: "Done", idle: "Idle", error: "Error", ended: "Ended",
};

export const MODE_WORDS: Record<string, string> = {
  default: "Ask before edits", acceptEdits: "Accept edits", plan: "Plan mode", auto: "Auto", bypassPermissions: "Skip permissions", dontAsk: "Don't ask",
};

/** What Claude wants, from the tool it asks for: "Wants to run a command". */
export function wants(tool: string): string {
  switch (tool) {
    case "Bash":
      return "run a command";
    case "Edit":
    case "MultiEdit":
    case "NotebookEdit":
      return "edit a file";
    case "Write":
      return "write a file";
    case "Read":
      return "read a file";
    case "WebFetch":
      return "fetch a web page";
    case "WebSearch":
      return "search the web";
  }
  return tool ? `use ${tool}` : "go ahead";
}

/** One line saying what a session is doing or waiting for. */
export function sessionLine(s: Session): string {
  if (s.notice) return s.notice;
  switch (s.state) {
    case "needs_you":
      if (s.need === "permission") return s.waiting?.summary ? `Wants to run: ${s.waiting.summary}` : "Needs your permission";
      if (s.need === "question") return s.waiting?.question || "Has a question for you";
      return "Waiting for you";
    case "working":
      return s.step || "Working…";
    case "starting":
      return "Starting Claude…";
    case "error":
      return s.last_message || "Stopped with an error";
    case "ended":
      return s.last_message || "Ended";
  }
  return s.last_message || "Ready for the next task";
}

/** "~/work/dashboard-app · main": where a session runs. */
export function where(cwd: string, branch: string): string {
  return tilde(cwd) + (branch ? ` · ${branch}` : "");
}

/** A sheet that slides up from the bottom; tapping outside or Escape closes it. */
export function Sheet({ title, label, onClose, children }: { title?: string; label?: string; onClose: () => void; children: ReactNode }) {
  const close = useRef(onClose);
  close.current = onClose;
  useEffect(() => {
    const key = (e: KeyboardEvent) => e.key === "Escape" && close.current();
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, []);
  return (
    <div className="scrim" onClick={onClose}>
      <div className="sheet" role="dialog" aria-modal="true" aria-label={label ?? title} onClick={(e) => e.stopPropagation()}>
        <div className="grip" />
        {title && <h2>{title}</h2>}
        {children}
      </div>
    </div>
  );
}

export function Switch({ on, onChange, label }: { on: boolean; onChange: (on: boolean) => void; label: string }) {
  return <button type="button" role="switch" aria-checked={on} aria-label={label} className={`switch ${on ? "on" : ""}`} onClick={() => onChange(!on)} />;
}

/**
 * The bar at the bottom of a screen (a composer, the key bar, the Start button). It tells the page how tall it is, so the banner for
 * another session and short notes sit just above it instead of over the header's buttons.
 */
export function Dock({ className = "", children }: { className?: string; children: ReactNode }) {
  const ref = useRef<HTMLElement>(null);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const root = document.documentElement.style;
    const set = () => root.setProperty("--dock-h", `${el.offsetHeight}px`);
    const ro = new ResizeObserver(set);
    ro.observe(el);
    set();
    return () => {
      ro.disconnect();
      root.removeProperty("--dock-h");
    };
  }, []);
  return (
    <footer ref={ref} className={`dock ${className}`}>
      {children}
    </footer>
  );
}

/** The last part of a path, for titles. */
export function baseName(path: string): string {
  return path.replace(/\/+$/, "").split("/").pop() || path;
}

/** A home folder path written with ~, as people say it. */
export function tilde(path: string, roots: string[] = []): string {
  const home = roots.find((r) => /^\/home\/[^/]+$/.test(r)) ?? path.match(/^\/home\/[^/]+/)?.[0];
  return home && (path === home || path.startsWith(home + "/")) ? "~" + path.slice(home.length) : path;
}
