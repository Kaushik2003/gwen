import type { wire } from "../api";
import { formatDuration, formatTime } from "../format";

/** Today's segments on one horizontal bar, coloured by project; breaks are hatched. */
export default function Timeline({ segments, colors, now }: { segments: wire.Segment[]; colors: Map<string | null, string>; now: number }) {
  if (segments.length === 0) return <p className="text-sm text-zinc-500">Nothing tracked yet today.</p>;
  const start = segments[0].started_at;
  const end = Math.max(now, ...segments.map((s) => s.ended_at ?? now));
  const span = Math.max(end - start, 1);
  return (
    <div>
      <div className="relative h-8 w-full overflow-hidden rounded-md bg-zinc-100 dark:bg-zinc-800">
        {segments.map((s) => {
          const segEnd = s.ended_at ?? now;
          const left = ((s.started_at - start) / span) * 100;
          const width = Math.max(((segEnd - s.started_at) / span) * 100, 0.4);
          const isBreak = s.kind !== "work";
          return (
            <div
              key={s.id}
              title={`${s.kind} ${formatTime(s.started_at)}–${s.ended_at ? formatTime(s.ended_at) : "now"} · ${formatDuration(segEnd - s.started_at)}`}
              className={`absolute top-0 h-full ${isBreak ? "hatched" : ""}`}
              style={{ left: `${left}%`, width: `${width}%`, backgroundColor: isBreak ? undefined : colors.get(s.project_id ?? null) }}
            />
          );
        })}
      </div>
      <div className="mt-1 flex justify-between text-xs text-zinc-500">
        <span>{formatTime(start)}</span>
        <span>{formatTime(end)}</span>
      </div>
    </div>
  );
}
