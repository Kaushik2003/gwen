import { Check, SquareTerminal, type LucideIcon } from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";
import { App } from "../api";
import { cx } from "../components/ui";

// Small pieces the shell's parts share (plan.md 3.15).

/** Claude's mark in Gwen's lists: a small asterisk in Claude's orange. */
export function ClaudeMark({ size = 13, className }: { size?: number; className?: string }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" aria-hidden className={cx("shrink-0", className)}>
      <path d="M12 2v20M2 12h20M4.9 4.9l14.2 14.2M19.1 4.9 4.9 19.1" stroke="#D97757" strokeWidth="2.8" strokeLinecap="round" />
    </svg>
  );
}

/** What a terminal is doing, as a glyph: a dot when it needs you, a spinner while it works, a tick when it finished unread. */
export function StateGlyph({ state, unread, shell }: { state: string; unread?: boolean; shell?: boolean }) {
  if (state === "needs_you" || state === "blocked")
    return <span title="Needs you" className="size-[7px] shrink-0 rounded-full bg-warning shadow-[0_0_0_3px_rgba(242,179,61,.18)]" />;
  if (state === "working" || state === "starting")
    return <span title="Working" className="size-2 shrink-0 animate-spin rounded-full border-[1.6px] border-info/30 border-t-info motion-reduce:animate-none" />;
  if ((state === "done" && unread) || state === "done")
    return <Check size={13} strokeWidth={2.6} aria-label="Done" className={cx("shrink-0", unread ? "text-success" : "text-ink-faint")} />;
  if (state === "error") return <span title="Error" className="size-[7px] shrink-0 rounded-full bg-danger" />;
  if (shell) return null;
  return null;
}

/** A terminal's icon: Claude's mark for a session, a terminal for a shell. */
export function TermIcon({ claude, size = 13 }: { claude: boolean; size?: number }) {
  return claude ? <ClaudeMark size={size} /> : <SquareTerminal size={size} aria-hidden className="shrink-0 text-ink-subtle" />;
}

export function Kbd({ children }: { children: ReactNode }) {
  return <kbd className="rounded border border-line-strong px-1.5 py-px font-mono text-[11px] text-ink-muted">{children}</kbd>;
}

let home: string | null = null;
let homeWait: Promise<string> | null = null;

/** The user's home folder, for showing paths as "~/...". */
export function useHome(): string {
  const [h, setH] = useState(home ?? "");
  useEffect(() => {
    if (home !== null) return;
    const wait = (homeWait ??= App.HomeDir().then(
      (v: string) => (home = v),
      () => (home = ""),
    ));
    wait.then(setH);
  }, []);
  return h;
}

export function tildify(path: string, home: string): string {
  if (home && (path === home || path.startsWith(home + "/"))) return "~" + path.slice(home.length);
  return path;
}

/** A section label: small uppercase muted text, for eyebrows and sub-headings inside a card. */
export function Eyebrow({ children, className }: { children: ReactNode; className?: string }) {
  return <span className={cx("text-[11px] font-semibold tracking-[0.06em] text-ink-subtle uppercase", className)}>{children}</span>;
}

/**
 * Today's card: a 44 px header (icon, title, count, then meta and actions on the right) over a hairline,
 * then the body. bodyClassName replaces the default 16 by 12 padding; list cards pass "" and pad their rows.
 */
export function Card({
  title,
  icon: Icon,
  count,
  countTone,
  meta,
  actions,
  children,
  className,
  bodyClassName = "px-4 py-3",
}: {
  title: string;
  icon?: LucideIcon;
  count?: number;
  countTone?: "warning";
  meta?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
  bodyClassName?: string;
}) {
  return (
    <section aria-label={title} className={cx("lift min-w-0 overflow-hidden rounded-xl border border-line bg-surface-1", className)}>
      <header className="flex h-11 items-center gap-2 border-b border-chrome-line pr-2.5 pl-4">
        {Icon && <Icon size={14} className="shrink-0 text-ink-subtle" aria-hidden />}
        <h2 className="shrink-0 text-[13px] font-semibold whitespace-nowrap text-ink">{title}</h2>
        {!!count && (
          <span
            className={cx(
              "inline-flex h-[18px] min-w-[18px] items-center justify-center rounded-full px-1.5 text-[11px] font-semibold tabular-nums",
              countTone === "warning" ? "bg-warning/15 text-warning" : "bg-surface-3 text-ink-muted",
            )}
          >
            {count}
          </span>
        )}
        <div className="ml-auto flex shrink-0 items-center gap-1">
          {meta && <span className="px-1.5 text-[12px] text-ink-subtle tabular-nums">{meta}</span>}
          {actions}
        </div>
      </header>
      <div className={bodyClassName}>{children}</div>
    </section>
  );
}

/** A project's colour square. */
export function Swatch({ color, size = 8 }: { color: string; size?: number }) {
  return <span aria-hidden className="shrink-0 rounded-[2px]" style={{ width: size, height: size, background: color }} />;
}

/** A scrolling page, centred, as the old dashboard's screens expect. */
export function Scroll({ children, wide }: { children: ReactNode; wide?: boolean }) {
  return (
    <div className="h-full overflow-y-auto">
      <div className={cx("@container mx-auto w-full px-7 pt-[22px] pb-8", wide ? "max-w-[1400px]" : "max-w-290")}>{children}</div>
    </div>
  );
}

