import { Check } from "lucide-react";
import { useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { cx } from "../components/ui";

// The project filter needs a custom trigger, project colours, and checked choices that RowMenu doesn't support.
// It closes on a click outside, Escape, Tab, a scroll, or a resize, and the arrow keys walk its items.

export type MenuEntry =
  | { kind: "divider" }
  | {
      kind?: "item";
      label: string;
      /** A project's colour square in front of the label. */
      color?: string;
      checked?: boolean;
      run: () => void;
    };

const menuWidth = 224;

/** A menu under its trigger: the trigger is whatever `trigger` draws, given whether the menu is open and how to toggle it. */
export function BoardMenu({
  label,
  entries,
  trigger,
  align = "end",
}: {
  label: string;
  entries: MenuEntry[];
  trigger: (p: { open: boolean; toggle: () => void; ref: (el: HTMLElement | null) => void }) => ReactNode;
  align?: "start" | "end";
}) {
  const [open, setOpen] = useState(false);
  const anchor = useRef<HTMLElement | null>(null);

  const close = (refocus: boolean) => {
    setOpen(false);
    if (refocus) anchor.current?.focus();
  };

  return (
    <>
      {trigger({
        open,
        toggle: () => setOpen((o) => !o),
        ref: (el) => {
          anchor.current = el;
        },
      })}
      {open && anchor.current && createPortal(<MenuList label={label} entries={entries} anchor={anchor.current} align={align} onClose={close} />, document.body)}
    </>
  );
}

function MenuList({ label, entries, anchor, align, onClose }: { label: string; entries: MenuEntry[]; anchor: HTMLElement; align: "start" | "end"; onClose: (refocus: boolean) => void }) {
  const box = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState<{ left: number; top: number } | null>(null);
  const closeRef = useRef(onClose);
  closeRef.current = onClose;

  useLayoutEffect(() => {
    const r = anchor.getBoundingClientRect();
    const h = box.current?.offsetHeight ?? 240;
    const left = Math.min(Math.max(8, align === "end" ? r.right - menuWidth : r.left), window.innerWidth - menuWidth - 8);
    // Below the trigger when it fits, else above it.
    const top = r.bottom + 4 + h <= window.innerHeight - 8 ? r.bottom + 4 : Math.max(8, r.top - 4 - h);
    setPos({ left, top });
    box.current?.querySelector<HTMLElement>('[role^="menuitem"]')?.focus({ preventScroll: true });
  }, [anchor, align]);

  useEffect(() => {
    const away = (e: MouseEvent) => {
      if (!box.current?.contains(e.target as Node) && !anchor.contains(e.target as Node)) closeRef.current(false);
    };
    const gone = (e: Event) => {
      if (!box.current?.contains(e.target as Node)) closeRef.current(false);
    };
    const resize = () => closeRef.current(false);
    window.addEventListener("mousedown", away);
    window.addEventListener("scroll", gone, true);
    window.addEventListener("resize", resize);
    return () => {
      window.removeEventListener("mousedown", away);
      window.removeEventListener("scroll", gone, true);
      window.removeEventListener("resize", resize);
    };
  }, [anchor]);

  function onKey(e: KeyboardEvent) {
    const items = [...(box.current?.querySelectorAll<HTMLElement>('[role^="menuitem"]') ?? [])];
    const at = items.indexOf(document.activeElement as HTMLElement);
    const go = (i: number) => items[(i + items.length) % items.length]?.focus({ preventScroll: true });
    if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
      closeRef.current(true);
    } else if (e.key === "Tab") {
      e.preventDefault();
      closeRef.current(true);
    } else if (e.key === "ArrowDown") {
      e.preventDefault();
      go(at + 1);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      go(at - 1);
    } else if (e.key === "Home") {
      e.preventDefault();
      go(0);
    } else if (e.key === "End") {
      e.preventDefault();
      go(items.length - 1);
    }
  }

  const checkable = entries.some((x) => (x.kind ?? "item") === "item" && "checked" in x && x.checked !== undefined);
  return (
    <div
      ref={box}
      role="menu"
      aria-label={label}
      onKeyDown={onKey}
      onClick={(e) => e.stopPropagation()}
      className="animate-pop fixed z-50 flex max-h-[min(26rem,calc(100vh-16px))] flex-col overflow-y-auto rounded-lg border border-line-strong bg-surface-2 p-1 text-[13px] shadow-xl shadow-black/50"
      style={{ width: menuWidth, left: pos?.left ?? -9999, top: pos?.top ?? -9999 }}
    >
      {entries.map((x, i) => {
        if (x.kind === "divider") return <div key={i} className="mx-1 my-1 h-px shrink-0 bg-line" />;
        return (
          <button
            key={i}
            type="button"
            role={checkable ? "menuitemradio" : "menuitem"}
            aria-checked={checkable ? !!x.checked : undefined}
            onClick={() => {
              closeRef.current(false);
              x.run();
            }}
            className={cx(
              "flex h-[30px] w-full shrink-0 items-center gap-2 rounded-md pr-2 pl-2.5 text-left outline-none hover:bg-surface-3 focus-visible:bg-surface-3",
              x.checked ? "text-ink" : "text-ink-muted hover:text-ink focus-visible:text-ink",
            )}
          >
            {x.color && <span aria-hidden className="size-[7px] shrink-0 rounded-[2px]" style={{ background: x.color }} />}
            <span className="min-w-0 flex-1 truncate">{x.label}</span>
            {x.checked && <Check size={13} className="shrink-0 text-ink-subtle" aria-hidden />}
          </button>
        );
      })}
    </div>
  );
}
