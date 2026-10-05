import type { wire } from "../api";
import { formatDate, formatDay, formatDuration } from "../format";

const monthNames = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
const levels = [0, 0.25, 0.5, 0.75, 1];

/** A year of days as a Monday-first grid, shaded by worked time against the daily target. */
export default function Heatmap({ heatmap, targetMs, today }: { heatmap: wire.Heatmap; targetMs: number; today: string }) {
  const worked = new Map(heatmap.days.map((d) => [d.day, d.worked_ms]));
  const jan1 = new Date(heatmap.year, 0, 1);
  const start = new Date(heatmap.year, 0, 1 - ((jan1.getDay() + 6) % 7));
  const cells: { x: number; y: number; day: string; ms: number; month: number; date: number }[] = [];
  for (let i = 0; ; i++) {
    const d = new Date(start.getFullYear(), start.getMonth(), start.getDate() + i);
    if (d.getFullYear() > heatmap.year && d.getDay() === 1) break;
    if (d.getFullYear() !== heatmap.year) continue;
    const day = formatDay(d);
    cells.push({ x: Math.floor(i / 7), y: i % 7, day, ms: worked.get(day) ?? 0, month: d.getMonth(), date: d.getDate() });
  }
  const level = (ms: number) => {
    if (ms <= 0) return 0;
    if (targetMs <= 0) return 1;
    return Math.min(4, Math.max(1, Math.ceil((ms / targetMs) * 4)));
  };
  const size = 11;
  const gap = 3;
  const left = 28;
  const top = 16;
  const weeks = cells[cells.length - 1].x + 1;
  const months = cells.filter((c) => c.date === 1);
  const tracked = cells.filter((c) => c.ms > 0).length;

  return (
    <div>
      <div className="overflow-x-auto">
        <svg viewBox={`0 0 ${left + weeks * (size + gap)} ${top + 7 * (size + gap)}`} className="w-full min-w-160" role="img" aria-label={`${heatmap.year}: ${tracked} days tracked`}>
          {months.map((c) => (
            <text key={c.month} x={left + c.x * (size + gap)} y={10} fontSize={10} fill="#62666d">
              {monthNames[c.month]}
            </text>
          ))}
          {["Mon", "", "Wed", "", "Fri", "", ""].map((l, i) => (
            <text key={i} x={0} y={top + i * (size + gap) + size - 2} fontSize={9} fill="#62666d">
              {l}
            </text>
          ))}
          {cells.map((c) => {
            const lv = level(c.ms);
            return (
              <rect
                key={c.day}
                x={left + c.x * (size + gap)}
                y={top + c.y * (size + gap)}
                width={size}
                height={size}
                rx={2.5}
                fill={lv === 0 ? "#18191a" : "#27a644"}
                fillOpacity={lv === 0 ? 1 : 0.22 + 0.195 * lv}
                stroke={c.day === today ? "#f7f8f8" : "none"}
                strokeWidth={c.day === today ? 1.25 : 0}
              >
                <title>{`${formatDate(c.day)}: ${c.ms > 0 ? formatDuration(c.ms) : "nothing tracked"}`}</title>
              </rect>
            );
          })}
        </svg>
      </div>
      <div className="mt-3 flex items-center justify-end gap-1.5 text-[11px] text-ink-faint">
        <span className="mr-1">Less</span>
        {levels.map((l, i) => (
          <span
            key={l}
            className="inline-block size-2.75 rounded-[2.5px]"
            style={{ backgroundColor: i === 0 ? "#18191a" : "#27a644", opacity: i === 0 ? 1 : 0.22 + 0.195 * i }}
          />
        ))}
        <span className="ml-1">More, against the daily target</span>
      </div>
    </div>
  );
}
