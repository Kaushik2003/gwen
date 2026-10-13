import { CalendarClock, Radio } from "lucide-react";
import { useMemo, useState, type FocusEvent, type MouseEvent as ReactMouseEvent, type ReactNode } from "react";
import { createPortal } from "react-dom";
import type { wire } from "../api";
import { formatDuration, formatTime } from "../format";
import { alpha, over, readableOn } from "./color";
import { Dot } from "./ui";

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
const unassigned = "#8a8f98";

/** Width, as a fraction of the whole span, below which a block's own label would be too
 * cramped to read and starts crowding its neighbours; such blocks show as plain colour,
 * their title still reachable from the tooltip. */
const labelMinFraction = 0.035;

/** The width of the "Plan"/"Tracked" row label gutter, in rem, kept in sync with the `w-16`/
 * `ml-16` utilities below so the live line lines up with each lane's own track. */
const gutterRem = 4;

interface Tip {
  x: number;
  y: number;
  color?: string;
  title: string;
  sub: string;
}

/** One day against the clock: what was planned on the top lane, what was tracked below it,
 * coloured by project with breaks hatched, a legend naming who's who, and one crisp line at
 * now while the day is live. Hovering (or focusing) any block shows its time and duration. */
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
  const [tip, setTip] = useState<Tip | null>(null);
  const showTip = (e: ReactMouseEvent | FocusEvent, color: string | undefined, title: string, sub: string) => {
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
    const x = Math.min(Math.max(r.left + r.width / 2, 72), window.innerWidth - 72);
    setTip({ x, y: r.top, color, title, sub });
  };
  const hideTip = () => setTip(null);

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
  const frac = (a: number, b: number) => (b - a) / span;
  // The live line's left offset, past the row label gutter: a CSS calc so it still lines up
  // with each track exactly as the panel is resized, with no JS measurement needed.
  const nowFrac = Math.min(1, Math.max(0, (now - start) / span));
  const nowLeft = `calc(${gutterRem}rem + ${nowFrac} * (100% - ${gutterRem}rem))`;

  const hours = Math.round(span / hour);
  const step = hours <= 12 ? 1 : hours <= 20 ? 2 : 3;
  const ticks: number[] = [];
  for (let t = start; t <= end; t += hour) ticks.push(t);
  const halfTicks: number[] = step === 1 ? ticks.slice(0, -1).map((t) => t + hour / 2) : [];
  const showPlan = planned.length + fixed.length > 0;
  const nowIn = live && now >= start && now <= end;
  // The elapsed portion of the day reads as faintly travelled ground, so the eye finds "now"
  // without hunting for the line: no gradient, just one flat, barely-there wash.
  const pastWidth = live && nowIn ? `${nowFrac * 100}%` : null;

  const legend = useMemo(() => {
    const projects = new Map<string | null, string>();
    let hasBreak = false;
    for (const s of segments) {
      if (s.kind === "work") projects.set(s.project_id ?? null, colors.get(s.project_id ?? null) ?? unassigned);
      else hasBreak = true;
    }
    return { projects: [...projects.entries()], hasBreak };
  }, [segments, colors]);

  const grid = (
    <>
      {ticks.slice(1, -1).map((t) => (
        <span key={t} className="absolute top-0 bottom-0 w-px bg-line/70" style={{ left: pct(t) }} />
      ))}
      {halfTicks.map((t) => (
        <span key={t} className="absolute top-0 bottom-0 w-px bg-line/30" style={{ left: pct(t) }} />
      ))}
    </>
  );

  return (
    <div className="select-none">
      <div className="relative ml-16 h-5 text-[11px] text-ink-faint tabular-nums">
        {ticks.map((t, i) => {
          // The tick nearest now steps aside for the live time chip, rather than crowd it. The
          // first/last tick is right/left-aligned rather than centred, so it reaches further
          // in and needs a wider berth.
          const edge = i === 0 || i === ticks.length - 1;
          const tooCloseToNow = nowIn && Math.abs(t - now) / span < (edge ? 0.12 : 0.05);
          return i % step === 0 && !tooCloseToNow ? (
            <span key={t} className={`absolute top-0 whitespace-nowrap ${i === 0 ? "" : i === ticks.length - 1 ? "-translate-x-full" : "-translate-x-1/2"}`} style={{ left: pct(t) }}>
              {formatTime(t).replace(":00 ", " ")}
            </span>
          ) : null;
        })}
        {nowIn && (
          <span
            className="absolute top-0 -translate-x-1/2 rounded-full bg-accent/15 px-1.5 py-px font-semibold whitespace-nowrap text-accent-hover"
            style={{ left: pct(now) }}
          >
            {formatTime(now)}
          </span>
        )}
      </div>
      <div className="relative">
        {showPlan && (
          <Lane label="Plan" icon={CalendarClock} pastWidth={pastWidth}>
            {grid}
            {fixed.map((f) => {
              const stop = f.start + f.minutes * 60_000;
              const sub = `${formatTime(f.start)} – ${formatTime(stop)} · ${formatDuration(f.minutes * 60_000)}`;
              return (
                <span
                  key={f.id}
                  aria-label={`${f.title}: ${sub}`}
                  onMouseEnter={(e) => showTip(e, undefined, f.title, sub)}
                  onMouseLeave={hideTip}
                  className="hatched-grey absolute top-1 bottom-1 truncate rounded-[4px] px-1.5 text-[11px] leading-4.5 text-ink-subtle"
                  style={{ left: pct(f.start), width: width(f.start, stop), boxShadow: "0 0 0 1.5px var(--color-canvas)" }}
                >
                  {frac(f.start, stop) > labelMinFraction && f.title}
                </span>
              );
            })}
            {planned.map((p) => {
              const stop = p.start + p.minutes * 60_000;
              const sub = `${formatTime(p.start)} · ${formatDuration(p.minutes * 60_000)}${p.done ? " · done" : ""}`;
              return (
                <span
                  key={p.id}
                  aria-label={`${p.title}: ${sub}`}
                  onMouseEnter={(e) => showTip(e, p.color, p.title, sub)}
                  onMouseLeave={hideTip}
                  className="bar-sheen absolute top-1 bottom-1 truncate rounded-[4px] px-1.5 text-[11px] leading-4.5 font-medium text-ink-muted"
                  style={{
                    left: pct(p.start),
                    width: width(p.start, stop),
                    backgroundColor: over(p.color, p.done ? 0.4 : 0.2),
                    boxShadow: `0 0 0 1.5px var(--color-canvas), inset 0 0 0 1px ${alpha(p.color, 0.7)}`,
                  }}
                >
                  {frac(p.start, stop) > labelMinFraction && p.title}
                </span>
              );
            })}
          </Lane>
        )}
        <Lane label="Tracked" icon={Radio} tall pastWidth={pastWidth}>
          {grid}
          {segments.map((s) => {
            const e = ends(s);
            const work = s.kind === "work";
            const open = live && s.ended_at == null;
            const color = work ? colors.get(s.project_id ?? null) ?? unassigned : undefined;
            const who = work ? names.get(s.project_id ?? null) ?? "Unassigned" : kindLabel[s.kind] ?? s.kind;
            const sub = `${formatTime(s.started_at)} – ${open ? "now" : formatTime(e)} · ${formatDuration(e - s.started_at)}${open ? " · in progress" : ""}`;
            const Tag = onSegment ? "button" : "span";
            return (
              <Tag
                key={s.id}
                type={onSegment ? "button" : undefined}
                onClick={onSegment ? () => onSegment(s) : undefined}
                aria-label={`${who}: ${sub}`}
                onMouseEnter={(e) => showTip(e, color, who, sub)}
                onMouseLeave={hideTip}
                onFocus={onSegment ? (e) => showTip(e, color, who, sub) : undefined}
                onBlur={onSegment ? hideTip : undefined}
                className={`bar-sheen absolute top-1 bottom-1 overflow-hidden rounded-[4px] ${work ? "" : "hatched"} ${onSegment ? "cursor-pointer hover:brightness-125" : ""}`}
                style={{
                  left: pct(s.started_at),
                  width: width(s.started_at, e),
                  backgroundColor: color,
                  boxShadow: "0 0 0 1.5px var(--color-canvas)",
                }}
              >
                {frac(s.started_at, e) > labelMinFraction && (
                  <span className="truncate px-1.5 text-[11px] leading-[inherit] font-medium" style={{ color: work ? readableOn(color!) : undefined }}>
                    {who}
                  </span>
                )}
              </Tag>
            );
          })}
          {segments.length === 0 && <span className="absolute inset-0 flex items-center px-3 text-xs text-ink-faint">Nothing tracked yet</span>}
        </Lane>
        {nowIn && (
          <>
            <span className="pointer-events-none absolute -top-1.5 -bottom-1.5 z-10 w-px bg-accent" style={{ left: nowLeft }} />
            <span
              className="pointer-events-none absolute -top-[5px] z-10 size-2 -translate-x-1/2 rounded-full bg-accent ring-2 ring-canvas"
              style={{ left: nowLeft }}
            />
          </>
        )}
      </div>
      {segments.length > 0 && (legend.projects.length > 0 || legend.hasBreak) && (
        <div className="mt-2.5 ml-16 flex flex-wrap items-center gap-x-3.5 gap-y-1 text-[11px] text-ink-subtle">
          {legend.projects.map(([id, color]) => (
            <span key={id ?? "none"} className="inline-flex items-center gap-1.5">
              <Dot color={color} size={7} />
              {names.get(id) ?? "Unassigned"}
            </span>
          ))}
          {legend.hasBreak && (
            <span className="inline-flex items-center gap-1.5">
              <span className="hatched inline-block size-[9px] shrink-0 rounded-[2px]" />
              Break
            </span>
          )}
        </div>
      )}
      {tip &&
        createPortal(
          <div
            role="tooltip"
            className="pointer-events-none fixed z-50 rounded-lg border border-line-3 bg-surface-2 px-2.5 py-1.5 shadow-lg shadow-black/40"
            style={{ left: tip.x, top: tip.y - 8, transform: "translate(-50%, -100%)" }}
          >
            <div className="flex items-center gap-1.5 text-[12px] font-medium whitespace-nowrap text-ink">
              {tip.color && <span className="size-1.5 shrink-0 rounded-full" style={{ backgroundColor: tip.color }} />}
              {tip.title}
            </div>
            <div className="mt-0.5 text-[11px] whitespace-nowrap text-ink-subtle tabular-nums">{tip.sub}</div>
          </div>,
          document.body,
        )}
    </div>
  );
}

function Lane({
  label,
  icon: Icon,
  tall,
  pastWidth,
  children,
}: {
  label: string;
  icon: typeof CalendarClock;
  tall?: boolean;
  pastWidth?: string | null;
  children: ReactNode;
}) {
  return (
    <div className="mt-2 flex items-center">
      <span className="flex w-16 shrink-0 items-center gap-1.5 text-[11px] font-medium text-ink-faint">
        <Icon size={12} aria-hidden className="shrink-0" />
        {label}
      </span>
      <div className={`relative flex-1 overflow-hidden rounded-lg bg-surface-2 ${tall ? "h-11" : "h-9"}`}>
        {pastWidth && <span className="absolute inset-y-0 left-0 bg-white/[0.04]" style={{ width: pastWidth }} />}
        {children}
      </div>
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
