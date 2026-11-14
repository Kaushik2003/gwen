import { MoreHorizontal, type LucideIcon } from "lucide-react";
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { cx } from "../components/ui";

// A row's ⋯ menu. It opens in a portal at the button, so a card that clips its content never cuts it off.

export type MenuItem = { label: string; onSelect: () => void; icon?: LucideIcon; danger?: boolean; disabled?: boolean } | { heading: string } | "divider";

export function RowMenu({ label, items, className }: { label: string; items: MenuItem[]; className?: string }) {
  const [open, setOpen] = useState(false);
  const [at, setAt] = useState<{ top: number; right: number; up: boolean } | null>(null);
  const button = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);

  useLayoutEffect(() => {
    if (!open || !button.current) return;
    const r = button.current.getBoundingClientRect();
    const up = r.bottom + 280 > window.innerHeight;
    setAt({ top: up ? r.top - 4 : r.bottom + 4, right: window.innerWidth - r.right, up });
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const close = (e: Event) => {
      if (e instanceof KeyboardEvent ? e.key === "Escape" : !menu.current?.contains(e.target as Node) && !button.current?.contains(e.target as Node)) setOpen(false);
    };
    const away = () => setOpen(false);
    document.addEventListener("pointerdown", close, true);
    document.addEventListener("keydown", close, true);
    window.addEventListener("scroll", away, true);
    window.addEventListener("resize", away);
    return () => {
      document.removeEventListener("pointerdown", close, true);
      document.removeEventListener("keydown", close, true);
      window.removeEventListener("scroll", away, true);
      window.removeEventListener("resize", away);
    };
  }, [open]);

  return (
    <>
      <button
        ref={button}
        type="button"
        aria-label={label}
        title={label}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={(e) => {
          e.stopPropagation();
          setOpen(!open);
        }}
        className={cx("grid size-7 shrink-0 place-items-center rounded-lg text-ink-subtle hover:bg-surface-2 hover:text-ink", open && "bg-surface-2 text-ink", className)}
      >
        <MoreHorizontal size={15} aria-hidden />
      </button>
      {open &&
        at &&
        createPortal(
          <div
            ref={menu}
            role="menu"
            className="fixed z-50 min-w-48 rounded-lg border border-line-3 bg-surface-2 p-1 shadow-lg shadow-black/40"
            style={{ right: at.right, ...(at.up ? { bottom: window.innerHeight - at.top } : { top: at.top }) }}
          >
            {items.map((it, i) =>
              it === "divider" ? (
                <div key={i} className="my-1 h-px bg-line" />
              ) : "heading" in it ? (
                <div key={i} className="px-2.5 pt-1.5 pb-1 text-[11px] font-semibold tracking-[0.06em] text-ink-subtle uppercase">
                  {it.heading}
                </div>
              ) : (
                <button
                  key={i}
                  type="button"
                  role="menuitem"
                  disabled={it.disabled}
                  onClick={(e) => {
                    e.stopPropagation();
                    setOpen(false);
                    it.onSelect();
                  }}
                  className={cx(
                    "flex h-8 w-full items-center gap-2.5 rounded-md px-2.5 text-left text-[13px] disabled:opacity-40",
                    it.danger ? "text-danger hover:bg-danger/10" : "text-ink hover:bg-surface-3",
                  )}
                >
                  {it.icon && <it.icon size={14} className="shrink-0 text-ink-subtle" aria-hidden />}
                  {it.label}
                </button>
              ),
            )}
          </div>,
          document.body,
        )}
    </>
  );
}
