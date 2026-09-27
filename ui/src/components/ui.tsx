import type { ButtonHTMLAttributes, ReactNode, SelectHTMLAttributes, InputHTMLAttributes } from "react";

type Tone = "primary" | "plain" | "danger";

const tones: Record<Tone, string> = {
  primary: "bg-emerald-600 text-white hover:bg-emerald-500 disabled:bg-emerald-600/50",
  plain:
    "bg-white text-zinc-800 ring-1 ring-zinc-300 hover:bg-zinc-100 dark:bg-zinc-800 dark:text-zinc-100 dark:ring-zinc-700 dark:hover:bg-zinc-700",
  danger: "bg-white text-red-600 ring-1 ring-red-300 hover:bg-red-50 dark:bg-zinc-800 dark:text-red-400 dark:ring-red-900",
};

export function Button({ tone = "plain", className = "", ...rest }: ButtonHTMLAttributes<HTMLButtonElement> & { tone?: Tone }) {
  return (
    <button
      className={`rounded-md px-3 py-1.5 text-sm font-medium transition disabled:cursor-not-allowed ${tones[tone]} ${className}`}
      {...rest}
    />
  );
}

export function Card({ title, children, className = "", actions }: { title?: ReactNode; children: ReactNode; className?: string; actions?: ReactNode }) {
  return (
    <section className={`rounded-xl bg-white p-4 shadow-sm ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800 ${className}`}>
      {(title || actions) && (
        <div className="mb-3 flex items-center justify-between gap-2">
          <h2 className="text-sm font-semibold text-zinc-500 dark:text-zinc-400">{title}</h2>
          <div className="flex gap-2">{actions}</div>
        </div>
      )}
      {children}
    </section>
  );
}

const field =
  "rounded-md border border-zinc-300 bg-white px-2 py-1 text-sm dark:border-zinc-700 dark:bg-zinc-800 focus:outline-none focus:ring-2 focus:ring-emerald-500";

export function Select(props: SelectHTMLAttributes<HTMLSelectElement>) {
  return <select {...props} className={`${field} ${props.className ?? ""}`} />;
}

export function Input(props: InputHTMLAttributes<HTMLInputElement>) {
  return <input {...props} className={`${field} ${props.className ?? ""}`} />;
}

export function Label({ text, error, children }: { text: string; error?: string; children: ReactNode }) {
  return (
    <label className="flex flex-col gap-1 text-sm">
      <span className="text-zinc-600 dark:text-zinc-400">{text}</span>
      {children}
      {error && <span className="text-xs text-red-600 dark:text-red-400">{error}</span>}
    </label>
  );
}

export function Modal({ title, onClose, children }: { title: string; onClose: () => void; children: ReactNode }) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40" onClick={onClose}>
      <div
        className="w-[28rem] max-w-[90vw] rounded-xl bg-white p-5 shadow-xl dark:bg-zinc-900"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="mb-4 text-lg font-semibold">{title}</h2>
        {children}
      </div>
    </div>
  );
}

/** The colour a state is shown in, matching the tray icons. */
export const stateColor: Record<string, string> = {
  off: "#6b7280",
  working: "#10b981",
  idle_pending: "#f59e0b",
  break_auto: "#3b82f6",
  break_manual: "#3b82f6",
};

export const stateLabel: Record<string, string> = {
  off: "Not clocked in",
  working: "Working",
  idle_pending: "Idle — still counting",
  break_auto: "On break (automatic)",
  break_manual: "On break",
};
