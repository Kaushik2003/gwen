import type { ReactNode } from "react";
import { clamp01 } from "./ui";

/** A ring filled to fraction, with whatever is passed drawn in its middle. */
export default function ProgressRing({
  fraction,
  color,
  size = 176,
  stroke = 12,
  label,
  children,
}: {
  fraction: number;
  color: string;
  size?: number;
  stroke?: number;
  label: string;
  children?: ReactNode;
}) {
  const r = (size - stroke) / 2;
  const c = 2 * Math.PI * r;
  const f = clamp01(fraction);
  return (
    <div
      role="meter"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={Math.round(f * 100)}
      className="relative grid shrink-0 place-items-center"
      style={{ width: size, height: size }}
    >
      <svg viewBox={`0 0 ${size} ${size}`} className="absolute inset-0 -rotate-90" aria-hidden>
        <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke="var(--color-surface-3)" strokeWidth={stroke} />
        {f > 0 && (
          <circle
            cx={size / 2}
            cy={size / 2}
            r={r}
            fill="none"
            stroke={color}
            strokeWidth={stroke}
            strokeLinecap="round"
            strokeDasharray={`${f * c} ${c}`}
            style={{ transition: "stroke-dasharray 600ms ease, stroke 300ms" }}
          />
        )}
      </svg>
      <div className="relative flex flex-col items-center text-center">{children}</div>
    </div>
  );
}
