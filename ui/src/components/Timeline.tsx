import type { wire } from "../api";
import { formatDuration, formatTime } from "../format";
import { alpha, over } from "./color";

export interface PlannedBlock {
  id: string;
  title: string;
  start: number;
  minutes: number;
  color: string;
  done: boolean;
}

export interface FixedBlock {
  id: string;
  title: string;
  start: number;
  minutes: number;
}

const kindLabel: Record<string, string> = { work: "Work", break_manual: "Break", break_auto: "Automatic break" };

/**
 * One day against the clock: what was planned on the top lane, what was
 * tracked below it, coloured by project with breaks hatched, and a line at
 * now while the day is live.
 */
export default function Timeline({
  segments,
  colors,
  names,
  now,
  live,
  from,
  to,
  planned = [],
  fixed = [],
  onSegment,
}: {
  segments: wire.Segment[];
  colors: Map<string | null, string>;
  names: Map<string | null, string>;
  now: number;
  /** The day is today: open segments run to now, and now is marked. */
  live: boolean;
  /** The span to show at least, such as the planner's day start and end. */
  from: number;
  to: number;
  planned?: PlannedBlock[];
  fixed?: FixedBlock[];
  onSegment?: (s: wire.Segment) => void;
}) {
  const ends = (s: wire.Segment) => s.ended_at ?? (live ? now : s.started_at);
  const starts = [from, ...segments.map((s) => s.started_at), ...planned.map((p) => p.start), ...fixed.map((f) => f.start)];
  const stops = [
    to,
    ...segments.map(ends),
    ...planned.map((p) => p.start + p.minutes * 60_000),
    ...fixed.map((f) => f.start + f.minutes * 60_000),
    ...(live ? [now] : []),
  ];
  const hour = 3_600_000;
  const start = floorHour(Math.min(...starts));
  const end = Math.max(start + hour, ceilHour(Math.max(...stops)));
  const span = end - start;
  const pct = (t: number) => `${((t - start) / span) * 100}%`;
  const width = (a: number, b: number) => `${Math.max(((b - a) / span) * 100, 0.35)}%`;

  const hours = Math.round(span / hour);
  const step = hours <= 12 ? 1 : hours <= 20 ? 2 : 3;
  const ticks: number[] = [];
  for (let t = start; t <= end; t += hour) ticks.push(t);
  const showPlan = planned.length + fixed.length > 0;
  const nowIn = live && now >= start && now <= end;

  const grid = (
    <>
      {ticks.slice(1, -1).map((t) => (
        <span key={t} className="absolute top-0 bottom-0 w-px bg-line/70" style={{ left: pct(t) }} />
      ))}
      {nowIn && <span className="absolute -top-1 -bottom-1 z-10 w-0.5 rounded-full bg-ink/80" style={{ left: `calc(${pct(now)} - 1px)` }} />}
    </>
  );

  return (
    <div className="select-none">
      <div className="relative ml-16 h-5 text-[11px] text-ink-faint tabular-nums">
        {ticks.map((t, i) =>
          i % step === 0 ? (
            <span key={t} className={`absolute top-0 whitespace-nowrap ${i === 0 ? "" : i === ticks.length - 1 ? "-translate-x-full" : "-translate-x-1/2"}`} style={{ left: pct(t) }}>
              {formatTime(t).replace(":00 ", " ")}
            </span>
          ) : null,
        )}
      </div>
      {showPlan && (
        <Lane label="Plan">
          {grid}
          {fixed.map((f) => (
            <span
              key={f.id}
              title={`${f.title}: ${formatTime(f.start)}–${formatTime(f.start + f.minutes * 60_000)}`}
              className="hatched-grey absolute top-1 bottom-1 overflow-hidden rounded-sm px-1.5 text-[11px] leading-4.5 text-ink-subtle"
              style={{ left: pct(f.start), width: width(f.start, f.start + f.minutes * 60_000) }}
            >
              {f.title}
            </span>
          ))}
          {planned.map((p) => (
            <span
              key={p.id}
              title={`${p.title}: ${formatTime(p.start)}, ${formatDuration(p.minutes * 60_000)}${p.done ? " (done)" : ""}`}
              className="absolute top-1 bottom-1 overflow-hidden rounded-sm px-1.5 text-[11px] leading-4.5 font-medium whitespace-nowrap text-ink-muted"
              style={{
                left: pct(p.start),
                width: width(p.start, p.start + p.minutes * 60_000),
                backgroundColor: over(p.color, p.done ? 0.4 : 0.2),
                boxShadow: `inset 0 0 0 1px ${alpha(p.color, 0.7)}`,
              }}
            >
              {p.title}
            </span>
          ))}
        </Lane>
      )}
      <Lane label="Tracked" tall>
        {grid}
        {segments.map((s) => {
          const e = ends(s);
          const work = s.kind === "work";
          const who = work ? names.get(s.project_id ?? null) ?? "Unassigned" : kindLabel[s.kind] ?? s.kind;
          const Tag = onSegment ? "button" : "span";
          return (
            <Tag
              key={s.id}
              type={onSegment ? "button" : undefined}
              onClick={onSegment ? () => onSegment(s) : undefined}
              title={`${who}: ${formatTime(s.started_at)}–${s.ended_at ? formatTime(s.ended_at) : "now"}, ${formatDuration(e - s.started_at)}`}
              className={`absolute top-1 bottom-1 rounded-sm ${work ? "" : "hatched"} ${onSegment ? "cursor-pointer hover:brightness-125" : ""}`}
              style={{ left: pct(s.started_at), width: width(s.started_at, e), backgroundColor: work ? colors.get(s.project_id ?? null) : undefined }}
            />
          );
        })}
        {segments.length === 0 && <span className="absolute inset-0 flex items-center px-3 text-xs text-ink-faint">Nothing tracked yet</span>}
      </Lane>
    </div>
  );
}

function Lane({ label, tall, children }: { label: string; tall?: boolean; children: React.ReactNode }) {
  return (
    <div className="mt-1.5 flex items-center">
      <span className="w-16 shrink-0 text-[11px] font-medium text-ink-faint">{label}</span>
      <div className={`relative flex-1 rounded-md bg-surface-2 ${tall ? "h-9" : "h-7"}`}>{children}</div>
    </div>
  );
}

function floorHour(ms: number): number {
  const d = new Date(ms);
  d.setMinutes(0, 0, 0);
  return d.getTime();
}

function ceilHour(ms: number): number {
  const f = floorHour(ms);
  return f === ms ? ms : f + 3_600_000;
}
