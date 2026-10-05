import { useEffect, useState } from "react";
import { goDuration, parseDuration } from "../format";
import { cx } from "./ui";

type Unit = "h" | "m" | "s";
const unitMs: Record<Unit, number> = { h: 3_600_000, m: 60_000, s: 1000 };
const unitName: Record<Unit, string> = { h: "hours", m: "minutes", s: "seconds" };

/** The duration split over units: the first takes the whole of the larger ones. */
function split(ms: number, units: Unit[]): string[] {
  let rest = ms;
  return units.map((u, i) => {
    const n = i === units.length - 1 ? Math.round(rest / unitMs[u]) : Math.floor(rest / unitMs[u]);
    rest -= n * unitMs[u];
    return String(n);
  });
}

/**
 * A duration as numbers with units, such as 7 h 30 m, edited as a Go
 * duration string ("7h30m") the configuration stores.
 */
export default function DurationInput({
  value,
  onChange,
  units = ["h", "m"],
  label,
  disabled,
}: {
  value: string;
  onChange: (v: string) => void;
  units?: Unit[];
  label: string;
  disabled?: boolean;
}) {
  const ms = parseDuration(value);
  const [parts, setParts] = useState(() => split(ms, units));
  const shown = parts.reduce((sum, p, i) => sum + (Number(p) || 0) * unitMs[units[i]], 0);
  // Only an outside change of the value resets what is being typed.
  useEffect(() => {
    if (shown !== ms) setParts(split(ms, units));
  }, [ms]);

  function edit(i: number, text: string) {
    const next = parts.map((p, j) => (j === i ? text.replace(/[^\d]/g, "") : p));
    setParts(next);
    onChange(goDuration(next.reduce((sum, p, j) => sum + (Number(p) || 0) * unitMs[units[j]], 0)));
  }

  return (
    <div role="group" aria-label={label} className={cx("flex items-center gap-1.5", disabled && "opacity-50")}>
      {units.map((u, i) => (
        <label key={u} className="relative">
          <input
            className="field w-19 pr-8 text-right tabular-nums"
            inputMode="numeric"
            value={parts[i]}
            disabled={disabled}
            aria-label={`${label}, ${unitName[u]}`}
            onChange={(e) => edit(i, e.target.value)}
            onBlur={() => setParts(split(parseDuration(value), units))}
          />
          <span className="pointer-events-none absolute top-1/2 right-2.5 -translate-y-1/2 text-xs text-ink-faint">{u}</span>
        </label>
      ))}
    </div>
  );
}
