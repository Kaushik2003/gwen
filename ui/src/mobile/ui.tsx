import { useEffect, type ReactNode } from "react";
import type { Session, SessionState } from "./api";

/** "2m", "3h", "Tue": how long ago, short enough for a list row. */
export function ago(ms: number, now = Date.now()): string {
  const s = Math.max(0, Math.round((now - ms) / 1000));
  if (s < 45) return "now";
  if (s < 3600) return `${Math.round(s / 60)}m`;
  if (s < 86400) return `${Math.round(s / 3600)}h`;
  return new Date(ms).toLocaleDateString(undefined, { weekday: "short" });
}

/** "just now", "5m ago", "Tue": how long ago, in a sentence. */
export function agoWords(ms: number): string {
  const a = ago(ms);
  return a === "now" ? "just now" : /^\d/.test(a) ? `${a} ago` : a;
}

/** The time of day, as the phone writes it. */
export function clock(ms: number): string {
  return new Date(ms).toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" });
}

export const STATE_WORDS: Record<SessionState, string> = {
  starting: "Starting", working: "Working", needs_you: "Needs you", done: "Done", idle: "Idle", error: "Error", ended: "Ended",
};

export const MODE_WORDS: Record<string, string> = {
  default: "Ask before edits", acceptEdits: "Accept edits", plan: "Plan", auto: "Auto", bypassPermissions: "Skip permissions", dontAsk: "Don't ask",
};

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

export function Dot({ state }: { state: string }) {
  return <span className={`dot ${state}`} aria-hidden />;
}

/** A sheet that slides up from the bottom; tapping outside closes it. */
export function Sheet({ title, onClose, children }: { title?: string; onClose: () => void; children: ReactNode }) {
  useEffect(() => {
    const key = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, [onClose]);
  return (
    <div className="scrim" onClick={onClose}>
      <div className="sheet" role="dialog" aria-label={title} onClick={(e) => e.stopPropagation()}>
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

/** The last part of a path, for titles. */
export function baseName(path: string): string {
  return path.replace(/\/+$/, "").split("/").pop() || path;
}

/** A home folder path written with ~, as people say it. */
export function tilde(path: string, roots: string[] = []): string {
  const home = roots.find((r) => /^\/home\/[^/]+$/.test(r)) ?? path.match(/^\/home\/[^/]+/)?.[0];
  return home && (path === home || path.startsWith(home + "/")) ? "~" + path.slice(home.length) : path;
}
