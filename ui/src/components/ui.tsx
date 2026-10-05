import { Check, LoaderCircle, X, type LucideIcon } from "lucide-react";
import {
  useEffect,
  useId,
  useRef,
  type ButtonHTMLAttributes,
  type ComponentProps,
  type CSSProperties,
  type InputHTMLAttributes,
  type ReactNode,
  type SelectHTMLAttributes,
} from "react";

/** Joins class names, skipping the falsy ones. */
export function cx(...parts: (string | false | null | undefined)[]): string {
  return parts.filter(Boolean).join(" ");
}

type Tone = "primary" | "secondary" | "ghost" | "danger" | "destroy";
type Size = "sm" | "md" | "lg";

const tones: Record<Tone, string> = {
  primary: "bg-accent text-white hover:bg-accent-hover active:bg-accent-focus",
  secondary: "border border-line-strong bg-surface-2 text-ink hover:border-line-3 hover:bg-surface-3",
  ghost: "text-ink-subtle hover:bg-surface-2 hover:text-ink",
  danger: "border border-line-strong bg-surface-2 text-danger hover:border-danger/50 hover:bg-danger/10",
  destroy: "bg-danger text-white hover:bg-danger/85",
};
const sizes: Record<Size, string> = {
  sm: "h-7 gap-1.5 px-2.5 text-[13px]",
  md: "h-[34px] gap-2 px-3.5 text-sm",
  lg: "h-11 gap-2.5 px-5 text-[15px]",
};
const iconSizes: Record<Size, number> = { sm: 14, md: 16, lg: 18 };

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  tone?: Tone;
  size?: Size;
  icon?: LucideIcon;
  /** Tints only the icon, such as the break colour on Start break. */
  iconColor?: string;
  busy?: boolean;
}

export function Button({ tone = "secondary", size = "md", icon: Icon, iconColor, busy, className, children, type = "button", disabled, ...rest }: ButtonProps) {
  return (
    <button
      type={type}
      disabled={disabled || busy}
      className={cx(
        "inline-flex shrink-0 select-none items-center justify-center rounded-lg font-medium whitespace-nowrap transition-colors disabled:cursor-not-allowed disabled:opacity-45",
        tones[tone],
        sizes[size],
        className,
      )}
      {...rest}
    >
      {busy ? (
        <LoaderCircle size={iconSizes[size]} className="animate-spin" aria-hidden />
      ) : (
        Icon && <Icon size={iconSizes[size]} strokeWidth={2} aria-hidden style={iconColor ? { color: iconColor } : undefined} />
      )}
      {children}
    </button>
  );
}

export interface IconButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  icon: LucideIcon;
  /** Read aloud and shown as the tooltip. */
  label: string;
  tone?: Tone;
  size?: "sm" | "md" | "lg";
  /** A toggled-on look, such as a pinned plan item. */
  active?: boolean;
}

export function IconButton({ icon: Icon, label, tone = "ghost", size = "md", active, className, type = "button", ...rest }: IconButtonProps) {
  return (
    <button
      type={type}
      aria-label={label}
      title={label}
      aria-pressed={active}
      className={cx(
        "inline-grid shrink-0 place-items-center rounded-lg transition-colors disabled:cursor-not-allowed disabled:opacity-40",
        size === "sm" ? "size-7" : size === "lg" ? "size-11" : "size-[34px]",
        active ? "bg-accent/15 text-accent-hover hover:bg-accent/25" : tones[tone],
        className,
      )}
      {...rest}
    >
      <Icon size={size === "sm" ? 14 : size === "lg" ? 18 : 16} strokeWidth={2} aria-hidden />
    </button>
  );
}

/** A lifted panel: surface 1, hairline border, 12 px corners. */
export function Panel({
  title,
  icon: Icon,
  actions,
  children,
  className,
  bodyClassName,
}: {
  title?: ReactNode;
  icon?: LucideIcon;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
  bodyClassName?: string;
}) {
  const header = title != null || actions != null;
  return (
    <section className={cx("lift min-w-0 rounded-xl border border-line bg-surface-1", className)}>
      {header && (
        <header className="flex min-h-11 flex-wrap items-center justify-between gap-x-3 gap-y-2 px-5 pt-4">
          <h2 className="flex items-center gap-2 text-[15px] font-semibold tracking-[-0.01em] text-ink">
            {Icon && <Icon size={16} className="text-ink-subtle" aria-hidden />}
            {title}
          </h2>
          {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
        </header>
      )}
      <div className={cx(header ? "px-5 pt-3 pb-5" : "p-5", bodyClassName)}>{children}</div>
    </section>
  );
}

/** A screen's title row: the headline on the left, its actions on the right. */
export function PageHeader({ title, subtitle, actions }: { title: ReactNode; subtitle?: ReactNode; actions?: ReactNode }) {
  return (
    <header className="mb-6 flex flex-wrap items-end justify-between gap-x-6 gap-y-3">
      <div className="min-w-0">
        <h1 className="text-headline font-semibold text-ink">{title}</h1>
        {subtitle && <p className="mt-1 text-sm text-ink-subtle">{subtitle}</p>}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </header>
  );
}

/**
 * A labelled form control. Simple inputs sit inside a <label>; compound
 * controls (segmented, pickers) get a group instead, because a <label>
 * would click its first button.
 */
export function Field({
  label,
  hint,
  error,
  children,
  className,
  compound,
}: {
  label: ReactNode;
  hint?: ReactNode;
  error?: string;
  children: ReactNode;
  className?: string;
  compound?: boolean;
}) {
  const id = useId();
  const body = (
    <>
      <span id={id} className="text-[13px] font-medium text-ink-muted">
        {label}
      </span>
      {children}
      {error ? <span className="text-xs text-danger">{error}</span> : hint ? <span className="text-xs leading-relaxed text-ink-subtle">{hint}</span> : null}
    </>
  );
  const cls = cx("flex min-w-0 flex-col gap-1.5", className);
  return compound ? (
    <div role="group" aria-labelledby={id} className={cls}>
      {body}
    </div>
  ) : (
    <label className={cls}>{body}</label>
  );
}

export function Input({ className, ...rest }: InputHTMLAttributes<HTMLInputElement>) {
  return <input className={cx("field", className)} {...rest} />;
}

export function Select({ className, ...rest }: SelectHTMLAttributes<HTMLSelectElement>) {
  return <select className={cx("field", className)} {...rest} />;
}

export function TextArea({ className, ...rest }: ComponentProps<"textarea">) {
  return <textarea className={cx("field", className)} {...rest} />;
}

/** A select with a colour dot in front, for projects. */
export function DotSelect({ color, className, ...rest }: SelectHTMLAttributes<HTMLSelectElement> & { color: string }) {
  return (
    <span className={cx("relative inline-flex min-w-0", className)}>
      <span className="pointer-events-none absolute top-1/2 left-3 size-2.5 -translate-y-1/2 rounded-full" style={{ backgroundColor: color }} />
      <select className="field w-full !pl-8" {...rest} />
    </span>
  );
}

export function Toggle({ checked, onChange, label, disabled }: { checked: boolean; onChange: (on: boolean) => void; label: string; disabled?: boolean }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={cx(
        "relative inline-flex h-5 w-9 shrink-0 items-center rounded-full border transition-colors disabled:cursor-not-allowed disabled:opacity-45",
        checked ? "border-accent bg-accent" : "border-line-3 bg-surface-3",
      )}
    >
      <span className={cx("inline-block size-3.5 rounded-full bg-white transition-transform", checked ? "translate-x-[17px]" : "translate-x-[2px]")} />
    </button>
  );
}

/** A setting that is on or off: what it does on the left, the switch on the right. */
export function ToggleRow({ title, description, checked, onChange, disabled }: { title: string; description?: ReactNode; checked: boolean; onChange: (on: boolean) => void; disabled?: boolean }) {
  return (
    <div className="flex items-start justify-between gap-6 py-1">
      <div className="min-w-0">
        <div className="text-sm font-medium text-ink">{title}</div>
        {description && <div className="mt-0.5 text-[13px] text-ink-subtle">{description}</div>}
      </div>
      <Toggle checked={checked} onChange={onChange} label={title} disabled={disabled} />
    </div>
  );
}

/** A round check for tasks, or a square one for picking from a list. */
export function Checkbox({
  checked,
  onChange,
  label,
  color,
  square,
  disabled,
}: {
  checked: boolean;
  onChange: (on: boolean) => void;
  label: string;
  color?: string;
  square?: boolean;
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      role="checkbox"
      aria-checked={checked}
      aria-label={label}
      title={label}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={cx(
        "grid size-[18px] shrink-0 place-items-center border transition-colors disabled:cursor-not-allowed disabled:opacity-45",
        square ? "rounded-[5px]" : "rounded-full",
        checked ? "border-transparent text-white" : "border-line-3 bg-transparent hover:border-ink-subtle",
      )}
      style={checked ? { backgroundColor: color ?? "var(--color-accent)" } : undefined}
    >
      {checked && <Check size={12} strokeWidth={3} aria-hidden />}
    </button>
  );
}

export interface SegmentOption<T> {
  value: T;
  label: ReactNode;
  icon?: LucideIcon;
  title?: string;
}

/** Pill tabs: the selected one is lifted onto surface 3. */
export function Segmented<T extends string | number>({
  value,
  onChange,
  options,
  size = "md",
  className,
  label,
}: {
  value: T;
  onChange: (v: T) => void;
  options: SegmentOption<T>[];
  size?: "sm" | "md";
  className?: string;
  label?: string;
}) {
  return (
    <div role="radiogroup" aria-label={label} className={cx("inline-flex max-w-full items-center gap-0.5 overflow-x-auto rounded-full border border-line bg-canvas p-0.5", className)}>
      {options.map((o) => {
        const on = o.value === value;
        const Icon = o.icon;
        return (
          <button
            key={String(o.value)}
            type="button"
            role="radio"
            aria-checked={on}
            title={o.title}
            onClick={() => onChange(o.value)}
            className={cx(
              "inline-flex shrink-0 items-center gap-1.5 rounded-full font-medium whitespace-nowrap transition-colors",
              size === "sm" ? "h-6 px-2.5 text-xs" : "h-7 px-3 text-[13px]",
              on ? "bg-surface-3 text-ink shadow-[inset_0_0_0_1px_var(--color-line-strong)]" : "text-ink-subtle hover:text-ink",
            )}
          >
            {Icon && <Icon size={14} aria-hidden />}
            {o.label}
          </button>
        );
      })}
    </div>
  );
}

type BadgeTone = "neutral" | "accent" | "working" | "idle" | "break" | "danger";
const badgeTones: Record<BadgeTone, string> = {
  neutral: "bg-surface-3 text-ink-muted",
  accent: "bg-accent/15 text-accent-hover",
  working: "bg-working/15 text-working",
  idle: "bg-idle/15 text-idle",
  break: "bg-break/15 text-break",
  danger: "bg-danger/15 text-danger",
};

export function Badge({ tone = "neutral", icon: Icon, children, className, title }: { tone?: BadgeTone; icon?: LucideIcon; children: ReactNode; className?: string; title?: string }) {
  return (
    <span title={title} className={cx("inline-flex h-5 shrink-0 items-center gap-1 rounded-full px-2 text-xs font-medium whitespace-nowrap", badgeTones[tone], className)}>
      {Icon && <Icon size={12} strokeWidth={2.25} aria-hidden />}
      {children}
    </span>
  );
}

/** A colour dot; it breathes while something is live. */
export function Dot({ color, size = 8, live, className, title }: { color: string; size?: number; live?: boolean; className?: string; title?: string }) {
  return (
    <span
      title={title}
      className={cx("inline-block shrink-0 rounded-full", live && "animate-live", className)}
      style={{ width: size, height: size, backgroundColor: color }}
    />
  );
}

/** A thin bar for progress, with an optional marker such as where today should be. */
export function Meter({
  value,
  color = "var(--color-accent)",
  marker,
  markerLabel,
  height = 6,
  label,
  className,
}: {
  value: number;
  color?: string;
  marker?: number | null;
  markerLabel?: string;
  height?: number;
  label: string;
  className?: string;
}) {
  const v = clamp01(value);
  return (
    <div
      role="meter"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={Math.round(v * 100)}
      className={cx("relative w-full rounded-full bg-surface-3", className)}
      style={{ height }}
    >
      <div className="h-full rounded-full transition-[width] duration-500" style={{ width: `${v * 100}%`, backgroundColor: color }} />
      {marker != null && (
        <div
          title={markerLabel}
          className="absolute -top-[3px] -bottom-[3px] w-[2px] rounded-full bg-ink-muted"
          style={{ left: `calc(${clamp01(marker) * 100}% - 1px)` }}
        />
      )}
    </div>
  );
}

export function clamp01(n: number): number {
  return Number.isFinite(n) ? Math.max(0, Math.min(1, n)) : 0;
}

/** An empty list or screen: what is missing and the one action that fills it. */
export function Empty({ icon: Icon, title, children, action, className }: { icon?: LucideIcon; title: string; children?: ReactNode; action?: ReactNode; className?: string }) {
  return (
    <div className={cx("flex flex-col items-center justify-center gap-1.5 rounded-lg border border-dashed border-line-strong px-6 py-9 text-center", className)}>
      {Icon && (
        <span className="mb-1.5 grid size-10 place-items-center rounded-full bg-surface-3 text-ink-subtle">
          <Icon size={18} aria-hidden />
        </span>
      )}
      <p className="text-sm font-medium text-ink">{title}</p>
      {children && <div className="max-w-sm text-[13px] leading-relaxed text-ink-subtle">{children}</div>}
      {action && <div className="mt-3 flex flex-wrap justify-center gap-2">{action}</div>}
    </div>
  );
}

/** A note inside a panel: warnings, hints, and results that need attention. */
export function Callout({ tone = "neutral", icon: Icon, children, action, className }: { tone?: "neutral" | "idle" | "danger" | "working" | "accent"; icon?: LucideIcon; children: ReactNode; action?: ReactNode; className?: string }) {
  const look = {
    neutral: "border-line-strong bg-surface-2 text-ink-muted",
    idle: "border-idle/30 bg-idle/10 text-idle",
    danger: "border-danger/30 bg-danger/10 text-danger",
    working: "border-working/30 bg-working/10 text-working",
    accent: "border-accent/35 bg-accent/10 text-accent-hover",
  }[tone];
  return (
    <div className={cx("flex items-start gap-3 rounded-lg border px-3.5 py-2.5 text-[13px] leading-relaxed", look, className)}>
      {Icon && <Icon size={16} className="mt-0.5 shrink-0" aria-hidden />}
      <div className="min-w-0 flex-1">{children}</div>
      {action && <div className="shrink-0">{action}</div>}
    </div>
  );
}

// Escape closes only the topmost dialog.
const openDialogs: number[] = [];
let nextDialog = 1;

export function Modal({
  title,
  description,
  onClose,
  children,
  footer,
  size = "md",
}: {
  title: string;
  description?: ReactNode;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
  size?: "sm" | "md" | "lg";
}) {
  const close = useRef(onClose);
  close.current = onClose;
  useEffect(() => {
    const id = nextDialog++;
    openDialogs.push(id);
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && openDialogs[openDialogs.length - 1] === id) {
        e.preventDefault();
        close.current();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
      openDialogs.splice(openDialogs.indexOf(id), 1);
    };
  }, []);
  const width = { sm: "max-w-[26rem]", md: "max-w-[34rem]", lg: "max-w-[44rem]" }[size];
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4" onMouseDown={() => close.current()}>
      <div
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className={cx("animate-pop lift flex max-h-[88vh] w-full flex-col overflow-hidden rounded-2xl border border-line-strong bg-surface-1 shadow-2xl shadow-black", width)}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <header className="flex items-start justify-between gap-4 px-6 pt-5 pb-4">
          <div className="min-w-0">
            <h2 className="text-[17px] font-semibold tracking-[-0.01em] text-ink">{title}</h2>
            {description && <div className="mt-1 text-[13px] leading-relaxed text-ink-subtle">{description}</div>}
          </div>
          <IconButton icon={X} label="Close" size="sm" onClick={() => close.current()} className="-mt-0.5 -mr-2" />
        </header>
        <div className="min-h-0 flex-1 overflow-y-auto px-6 pb-6">{children}</div>
        {footer && <footer className="flex flex-wrap items-center justify-end gap-2 border-t border-line bg-surface-2 px-6 py-3">{footer}</footer>}
      </div>
    </div>
  );
}

/** A full-window message while there is nothing else to show. */
export function Splash({ children }: { children: ReactNode }) {
  return <div className="flex h-full items-center justify-center p-8 text-sm text-ink-subtle">{children}</div>;
}

/** The colour a state is shown in, matching the tray icons. */
export const stateColor: Record<string, string> = {
  off: "#62666d",
  working: "#27a644",
  idle_pending: "#f2c94c",
  break_auto: "#4ea7fc",
  break_manual: "#4ea7fc",
};

export const stateLabel: Record<string, string> = {
  off: "Not clocked in",
  working: "Working",
  idle_pending: "Idle (still counting)",
  break_auto: "On break (automatic)",
  break_manual: "On break",
};

/** The colour of an unassigned project's time. */
export const unassignedColor = "#8a8f98";

export const priorities: { value: number; label: string; color: string }[] = [
  { value: 1, label: "Low", color: "#62666d" },
  { value: 2, label: "Normal", color: "#8a8f98" },
  { value: 3, label: "High", color: "#f2c94c" },
  { value: 4, label: "Urgent", color: "#eb5757" },
];

export function priorityOf(p: number) {
  return priorities.find((x) => x.value === p) ?? priorities[1];
}

/** Style for a coloured swatch, such as a project's. */
export function swatch(color: string): CSSProperties {
  return { backgroundColor: color };
}
