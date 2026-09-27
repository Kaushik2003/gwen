import type { wire } from "../api";
import { formatDay, formatDuration } from "../format";

/** A year of days as a Monday-first grid, shaded against the daily target. */
export default function Heatmap({ heatmap, targetMs }: { heatmap: wire.Heatmap; targetMs: number }) {
  const worked = new Map(heatmap.days.map((d) => [d.day, d.worked_ms]));
  const jan1 = new Date(heatmap.year, 0, 1);
  const start = new Date(heatmap.year, 0, 1 - ((jan1.getDay() + 6) % 7));
  const cells: { x: number; y: number; day: string; ms: number; inYear: boolean }[] = [];
  for (let i = 0; ; i++) {
    const d = new Date(start.getFullYear(), start.getMonth(), start.getDate() + i);
    if (d.getFullYear() > heatmap.year && d.getDay() === 1) break;
    const day = formatDay(d);
    cells.push({ x: Math.floor(i / 7), y: i % 7, day, ms: worked.get(day) ?? 0, inYear: d.getFullYear() === heatmap.year });
  }
  const shade = (ms: number) => {
    if (ms <= 0 || targetMs <= 0) return ms > 0 ? 1 : 0;
    return Math.min(1, ms / targetMs);
  };
  const size = 12;
  const weeks = cells[cells.length - 1].x + 1;
  return (
    <svg viewBox={`0 0 ${weeks * (size + 2) + 30} ${7 * (size + 2)}`} className="w-full">
      {["Mon", "", "Wed", "", "Fri", "", ""].map((l, i) => (
        <text key={i} x="0" y={i * (size + 2) + size - 2} className="fill-zinc-500 text-[9px]">
          {l}
        </text>
      ))}
      {cells
        .filter((c) => c.inYear)
        .map((c) => {
          const s = shade(c.ms);
          return (
            <rect
              key={c.day}
              x={30 + c.x * (size + 2)}
              y={c.y * (size + 2)}
              width={size}
              height={size}
              rx="2"
              fill={s === 0 ? undefined : "#10b981"}
              fillOpacity={s === 0 ? undefined : 0.2 + 0.8 * s}
              className={s === 0 ? "fill-zinc-200 dark:fill-zinc-800" : ""}
            >
              <title>{`${c.day}: ${formatDuration(c.ms)}`}</title>
            </rect>
          );
        })}
    </svg>
  );
}
