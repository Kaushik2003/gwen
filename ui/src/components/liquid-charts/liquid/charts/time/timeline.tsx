"use client";
/*
Liquid Charts by Ojieame · https://liquid.ojieame.design · nathan@ojieame.design

Liquid Charts
Copyright (c) 2026 NATEPAY, LLC. This file is source-available under the Liquid Charts
Source-Available Licence 1.0, not under MIT: https://liquid.ojieame.design/docs/licence

Free to use and change in anything you make, commercial products and client work included, as
long as the Liquid credit stays visible where the charts show it and this notice stays. You may
not redistribute or sell it as a chart library, component library, UI kit, template, registry
or other developer tool.

The notices below, and any later in this file, are kept word for word. The work each one
names keeps its own licence: the MIT notices below apply only to portions derived from the
projects they name, and a font keeps its font licence.

Liquid Charts adaptation. Original foundation: Evil Charts by Gurbinder.
Upstream: ecbd6a5db7b25f9a7e070c02b2ef728c378eeb5c
MIT License

Copyright (c) 2026 Gurbinder

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

Includes shadcn-derived primitives:
MIT License

Copyright (c) 2023 shadcn

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

*/

import {
  arrowHead,
  dateRangeText,
  DAY,
  dependencyEnds,
  dependencyPath,
  durationText,
  endText,
  fitsInBar,
  overrunShare,
  relatedTasks,
  slip,
  taskAt,
  taskInNextLane,
  taskRect,
  tickVisibility,
  TIMELINE_DENSITY,
  timelineDomain,
  timelineGeometry,
  timelineInsets,
  timelineModel,
  timeValue,
  zoomTicks,
  type ParsedTask,
  type TimeDensity,
  type TimelineGeometry,
  type TimelineModel,
  type TimelineTask,
  type TimelineUnit,
  type TimelineZoom,
} from "./layout";
import {
  ChartFrame,
  ChartState,
  cssToRgb,
  EventMarker,
  Figure,
  mutedText,
  noop,
  placeTooltip,
  seriesColour,
  stagger,
  textWidth,
  TYPE,
  useActive,
  useChartFrame,
  type BarFrame,
  type BarMark,
  type ChartConfig,
  type Insets,
  type LabelMode,
  type Rgb,
  type StyleSpec,
  type Theme,
} from "@/components/liquid-charts/liquid/core";
import {
  forwardRef,
  useRef,
  type CSSProperties,
  type ForwardedRef,
  type KeyboardEvent,
  type ReactNode,
} from "react";
import { BarLayer, DiamondLayer, type DiamondMark } from "./marks";
import { usePageTheme } from "@/components/liquid-charts/liquid/core/page-theme";
import { cssModule } from "@/components/liquid-charts/liquid/core/css-module";
import Roles from "@/components/liquid-charts/liquid/core/type.module.css";
import Core from "@/components/liquid-charts/liquid/core/core.module.css";
import { LiquidCredit } from "@/components/liquid-charts/liquid/credit";
import Styles from "./time.module.css";

const roles = cssModule(Roles, "lc-type-");
const core = cssModule(Core, "lc-core-");
const styles = cssModule(Styles, "lc-time-");

export type { TimelineTask, TimelineUnit, TimelineZoom };

export type TimelineProps = {
  /** Spans and milestones. Tasks sharing a `lane` share a row; `group` heads a run of lanes. */
  tasks: readonly TimelineTask[];
  /** Colours by each task's `series` (default its group, then its lane). */
  config: ChartConfig;
  /** The axis's unit: hours, days, Monday weeks, months or quarters. `auto` (the default) suits
   * the span. */
  unit?: TimelineUnit;
  /** @deprecated Use `unit`. The old name for the axis's unit (it never zoomed); `unit` wins when
   * both are set. */
  zoom?: TimelineUnit;
  /** The span shown; tasks running past it fade at the edge. Default: every task, with air. */
  window?: readonly [Date | number | string, Date | number | string];
  /** Today (or now): a quiet line with a tag, and what "reached" and "due" are measured from. */
  today?: Date | number | string | null;
  todayLabel?: string;
  /** Light or dark. Without one, the page's: `data-theme` or a `dark` class on `<html>`, else the system setting. @default the page's */
  theme?: Theme;
  /** Row height follows the density; `height` fits the rows to a height instead. */
  height?: number;
  density?: TimeDensity;
  /** The task in focus (an index into `tasks`). */
  active?: number | null;
  onActiveChange?: (index: number | null) => void;
  /** `auto` names bars that share a row; `all` names every bar and shows progress; `none`. */
  labels?: LabelMode;
  tooltip?: boolean;
  /** Forces glass or flat bars and milestones. */
  material?: "glass" | "flat";
  timeZone?: string;
  loading?: boolean;
  empty?: ReactNode;
  error?: ReactNode;
  replay?: number;
  label: string;
  /** One sentence for the focused task (an index into `tasks`), read aloud. */
  describe?: (index: number) => string;
  className?: string;
};

const FALLBACK: Rgb = [0.56, 0.56, 0.56];

/**
 * A timeline or Gantt chart: tasks as horizontal glass bars on a time axis, in lanes and groups
 * with their names beside them. A task's progress is liquid in its bar (the bar is a container,
 * full when done, empty glass before it starts) and the work not yet done is etched; where a task
 * ran past its plan, the planned end is engraved across the glass. Dependencies are quiet flat elbows with rounded corners; milestones are small glass
 * diamonds, full once reached and clear glass while ahead; today is a quiet line. Focus lights a
 * task and the tasks it waits on and that wait on it, and turns the rest graphite. Left and Right
 * step through tasks, Up and Down move between rows.
 */
export function Timeline({
  tasks,
  config,
  unit: unitProp,
  zoom,
  window,
  today = null,
  todayLabel = "Today",
  theme: themeProp,
  height,
  density = "standard",
  active: activeProp,
  onActiveChange,
  labels = "auto",
  tooltip = true,
  material,
  timeZone = "UTC",
  loading = false,
  empty,
  error,
  replay = 0,
  label,
  describe,
  className,
}: TimelineProps) {
  const theme = usePageTheme(themeProp);
  const valid = tasks.filter((t) => Number.isFinite(timeValue(t.start)));
  const toModel = valid.map((t) => tasks.indexOf(t));
  const [activeOuter, setActiveOuter] = useActive(activeProp, onActiveChange);
  const active =
    activeOuter === null
      ? null
      : toModel.indexOf(activeOuter) < 0
        ? null
        : toModel.indexOf(activeOuter);
  const setActive = (index: number | null) =>
    setActiveOuter(index === null ? null : toModel[index]);
  const tip = useRef<HTMLDivElement>(null);

  const model = timelineModel(valid);
  const now = today === null ? null : timeValue(today);
  const todayAt = now !== null && Number.isFinite(now) ? now : null;
  const win =
    window && Number.isFinite(timeValue(window[0])) && Number.isFinite(timeValue(window[1]))
      ? ([timeValue(window[0]), timeValue(window[1])] as const)
      : null;
  const unit = unitProp ?? zoom ?? "auto";
  const domain = timelineDomain(model.extent, { window: win, today: todayAt, zoom: unit });
  const frameInsets = {
    top:
      todayAt !== null && todayAt >= domain[0] && todayAt <= domain[1]
        ? 26
        : density === "tile"
          ? 8
          : 12,
    bottom: density === "tile" ? 24 : 30,
  };
  const tracks = model.lanes.reduce((n, lane) => n + lane.tracks, 0);
  const headerRows = model.lanes.filter(
    (lane, i) => lane.group !== null && (i === 0 || model.lanes[i - 1].group !== lane.group),
  ).length;
  const rowHeight = height
    ? Math.max(
        18,
        Math.min(
          44,
          (height -
            frameInsets.top -
            frameInsets.bottom -
            headerRows * TIMELINE_DENSITY[density].header) /
            Math.max(1, tracks),
        ),
      )
    : TIMELINE_DENSITY[density].row;
  const geometryFor = (width: number): TimelineGeometry => {
    const insets: Insets = {
      ...timelineInsets({
        lanes: model.lanes.map((lane) => lane.label),
        width,
        density,
        today: false,
      }),
      ...frameInsets,
    };
    return timelineGeometry(model, { width, insets, domain, density, rowHeight });
  };
  const frameHeight = geometryFor(720).height;

  const related = relatedTasks(model, active);
  const colourOf = (task: ParsedTask) => seriesColour(config, task.series, theme);
  const tintOf = (task: ParsedTask) => cssToRgb(colourOf(task)) ?? FALLBACK;
  const order = model.order;
  const slotOf = (index: number) => order.indexOf(index);
  const pace = stagger(order.length);
  const litOf = (task: ParsedTask) => (active === null || related.has(task.index) ? 1 : 0);
  const clipped = model.tasks.some((t) => t.start < domain[0] || t.end > domain[1]);
  const year = new Date(todayAt ?? model.extent[0]).getUTCFullYear();

  /* A bar is a container: liquid to the share done, the rest of the planned work etched (the same
   * quantity, not yet real), and where a task ran past its plan, the plan's end engraved across
   * the glass. Untracked tasks are full. */
  const bars = (width: number): BarMark[] => {
    const g = geometryFor(width);
    return g.shapes.flatMap((s) => {
      const t = model.tasks[s.index];
      if (s.milestone) return [];
      const plan = overrunShare(t);
      const done = t.progress ?? 1;
      return [
        {
          key: t.id,
          cross: s.cross,
          base: s.base,
          length: s.length,
          thickness: s.thickness,
          direction: "right" as const,
          tint: tintOf(t),
          lit: litOf(t),
          level: done,
          pattern: done < 1 ? done : undefined,
          marks: plan === null ? undefined : [plan],
          delay: slotOf(t.index) * pace,
        },
      ];
    });
  };
  /** A milestone is reached once marked done, or once today has passed it; with no today, every
   * moment shown has happened. Reached diamonds are full glass; ones ahead are clear and etched. */
  const reached = (t: ParsedTask) =>
    t.progress !== null ? t.progress >= 1 : todayAt === null || t.start <= todayAt;
  const diamonds = (width: number): DiamondMark[] => {
    const g = geometryFor(width);
    return g.shapes.flatMap((s) => {
      const t = model.tasks[s.index];
      if (!s.milestone) return [];
      const full = reached(t);
      return [
        {
          key: t.id,
          cx: s.base,
          cy: s.cross,
          size: s.size,
          tint: tintOf(t),
          lit: litOf(t),
          level: full ? 1 : 0,
          pattern: full ? undefined : 0,
          delay: 0.2 + slotOf(t.index) * pace,
        },
      ];
    });
  };

  const onFrame = (_bars: BarFrame[], width: number) => {
    const card = tip.current;
    if (!card || active === null) return;
    const g = geometryFor(width);
    const r = taskRect(g.shapes[active]);
    placeTooltip(
      card,
      { left: r.left - 4, right: r.right + 4, top: r.top - 4, bottom: r.bottom + 4 },
      {
        left: 4,
        top: g.plot.top,
        right: width - 4,
        bottom: g.plot.bottom,
      },
      { prefer: "row" },
    );
  };

  /* ——— States ——— */
  if (error || loading || !model.tasks.length) {
    const kind = error ? "error" : loading ? "loading" : "empty";
    return (
      <ChartFrame
        credit={<LiquidCredit />}
        theme={theme}
        height={height ?? 220}
        insets={{ left: 0, right: 0, ...frameInsets }}
        count={1}
        active={null}
        onActiveChange={noop}
        pick={() => null}
        label={label}
        className={className}
      >
        <ChartState
          kind={kind}
          shape="rows"
          bars={5}
          message={kind === "error" ? (error === true ? undefined : error) : empty}
        />
      </ChartFrame>
    );
  }

  /* ——— Reading aloud and the data table ——— */
  const range = (t: ParsedTask) => dateRangeText(t.start, t.end, { timeZone, year });
  const lateness = (t: ParsedTask) => {
    const late = slip(t);
    if (!Number.isFinite(late) || Math.abs(late) < 60_000) return null;
    const words = durationText(Math.abs(late));
    return late > 0 ? `${words} late` : `${words} early`;
  };
  const describeTask = (i: number) => {
    const t = model.tasks[i];
    const lane = model.lanes[t.lane];
    const parts = [
      t.milestone ? `milestone on ${range(t)}` : `${range(t)}, ${durationText(t.end - t.start)}`,
    ];
    if (!t.milestone && t.progress !== null) parts.push(`${Math.round(t.progress * 100)}% done`);
    if (t.milestone && todayAt !== null) parts.push(reached(t) ? "reached" : "ahead");
    const late = lateness(t);
    if (late) parts.push(late);
    if (t.after.length) parts.push(`after ${t.after.map((j) => model.tasks[j].name).join(", ")}`);
    const where = lane.label !== t.name ? ` (${lane.label})` : "";
    return `${t.name}${where}: ${parts.join(", ")}.`;
  };
  const columns = ["Task", "Lane", "Dates", "Duration", "Progress", "After"];
  const tableRows = model.order.map((i) => {
    const t = model.tasks[i];
    return [
      t.name,
      model.lanes[t.lane].label,
      range(t),
      t.milestone ? "Milestone" : durationText(t.end - t.start),
      t.progress === null ? "—" : `${Math.round(t.progress * 100)}%`,
      t.after.map((j) => model.tasks[j].name).join(", ") || "—",
    ];
  });

  const spans = model.tasks.filter((t) => !t.milestone);
  const spec: StyleSpec = {
    geometry: "cartesian",
    marks: ["solid", "stroke"],
    axis: "time",
    measure: "amount",
    series: 1,
    markCount: spans.length,
    density,
  };

  /* Up and Down move between rows; Left and Right step through tasks (the frame's own keys). */
  const onKeyDownCapture = (event: KeyboardEvent<HTMLDivElement>) => {
    if (active === null || (event.key !== "ArrowUp" && event.key !== "ArrowDown")) return;
    const next = taskInNextLane(model, active, event.key === "ArrowUp" ? -1 : 1);
    event.preventDefault();
    event.stopPropagation();
    if (next !== null) setActive(next);
  };

  return (
    <div className={`${styles.timeline} ${className ?? ""}`} onKeyDownCapture={onKeyDownCapture}>
      <ChartFrame
        credit={<LiquidCredit />}
        theme={theme}
        height={frameHeight}
        insets={{ left: 0, right: 0, ...frameInsets }}
        count={order.length}
        active={active === null ? null : slotOf(active)}
        onActiveChange={(slot) => setActive(slot === null ? null : order[slot])}
        pick={(x, y, size) => {
          const index = taskAt(model, geometryFor(size.width), x, y);
          return index === null ? null : slotOf(index);
        }}
        label={label}
        describe={(slot) => (describe ? describe(toModel[order[slot]]) : describeTask(order[slot]))}
        table={{ caption: label, columns, rows: tableRows }}
        className={styles.frame}
      >
        <Grid
          geometryFor={geometryFor}
          model={model}
          domain={domain}
          unit={unit}
          timeZone={timeZone}
          active={active}
          today={todayAt}
          todayLabel={todayLabel}
          labels={labels}
        />
        <Sized geometryFor={geometryFor} clipped={clipped}>
          {(g) => (
            <>
              <Connectors
                geometryFor={geometryFor}
                model={model}
                active={active}
                key={`c${replay}`}
              />
              <BarLayer
                theme={theme}
                replay={replay}
                spec={spec}
                material={material}
                smallest={g?.shapes[0]?.thickness}
                layout={(width) => bars(width)}
                onFrame={(frames, width) => onFrame(frames, width)}
              />
              <DiamondLayer
                theme={theme}
                replay={replay}
                layout={(width) => diamonds(width)}
                flat={material === "flat"}
              />
            </>
          )}
        </Sized>
        <BarNames geometryFor={geometryFor} model={model} labels={labels} active={active} />
        {tooltip && active !== null && (
          <TaskTooltip
            ref={tip}
            task={model.tasks[active]}
            model={model}
            colour={colourOf(model.tasks[active])}
            range={range(model.tasks[active])}
            lateness={lateness(model.tasks[active])}
            today={todayAt}
          />
        )}
      </ChartFrame>
    </div>
  );
}

/** Children that need the geometry, with the layers faded at the plot's edges when tasks run past
 * the window (so a clipped bar reads as running on, not as starting there). */
function Sized({
  geometryFor,
  clipped,
  children,
}: {
  geometryFor: (width: number) => TimelineGeometry;
  clipped: boolean;
  children: (g: TimelineGeometry | null) => ReactNode;
}) {
  const { size } = useChartFrame();
  const g = size.width ? geometryFor(size.width) : null;
  const left = g ? g.plot.left : 0,
    right = g ? g.plot.right : 0;
  const mask = g
    ? clipped
      ? `linear-gradient(90deg, transparent ${left - 6}px, #000 ${left + 18}px, #000 ${right - 18}px, transparent ${right + 6}px)`
      : `linear-gradient(90deg, transparent ${left - 14}px, #000 ${left - 2}px)`
    : undefined;
  return (
    <div
      className={styles.marks}
      style={mask ? ({ maskImage: mask, WebkitMaskImage: mask } as CSSProperties) : undefined}
    >
      {children(g)}
    </div>
  );
}

/* ——— Rows, groups, the time axis and today ——— */

function Grid({
  geometryFor,
  model,
  domain,
  unit,
  timeZone,
  active,
  today,
  todayLabel,
  labels,
}: {
  geometryFor: (width: number) => TimelineGeometry;
  model: TimelineModel;
  domain: readonly [number, number];
  unit: TimelineUnit;
  timeZone: string;
  active: number | null;
  today: number | null;
  todayLabel: string;
  /** The bar names' mode: the today line steps round any name it would cross. */
  labels: LabelMode;
}) {
  const { size } = useChartFrame();
  if (!size.width) return null;
  const g = geometryFor(size.width);
  const ticks = zoomTicks(domain, unit, { width: g.plot.right - g.plot.left, timeZone });
  const shown = tickVisibility(ticks, g.x);
  const activeLane = active === null ? null : model.tasks[active].lane;
  const litLanes = new Set(
    active === null ? [] : [...relatedTasks(model, active)].map((i) => model.tasks[i].lane),
  );
  const labelWidth = g.plot.left - 14;
  return (
    <>
      {ticks.map((tick, i) => {
        const x = Math.round(g.x(tick.value)) + 0.5;
        if (x < g.plot.left - 0.5 || x > g.plot.right + 0.5) return null;
        return (
          <span
            key={tick.value}
            className={styles.vgrid}
            style={{ left: x, top: g.plot.top, height: g.plot.bottom - g.plot.top }}
            aria-hidden="true"
          >
            {shown[i] && <span className={`${styles.vtick} ${roles.label}`}>{tick.label}</span>}
          </span>
        );
      })}
      {activeLane !== null && (
        <span
          className={styles.rowBand}
          style={{
            top: g.lanes[activeLane].top,
            height: g.lanes[activeLane].bottom - g.lanes[activeLane].top,
            left: 0,
            right: 0,
          }}
          aria-hidden="true"
        />
      )}
      {g.headers.map((header) => (
        <span
          key={header.label}
          className={styles.groupHeader}
          style={{ top: header.top, height: header.height }}
          aria-hidden="true"
        >
          <span className={`${roles.caption} ${styles.groupName}`}>{header.label}</span>
        </span>
      ))}
      {model.lanes.map((lane, i) => (
        <span
          key={lane.key}
          className={`${styles.rowLabel} ${roles.label}`}
          data-active={activeLane === i}
          data-dim={active !== null && !litLanes.has(i)}
          style={{
            top: (g.lanes[i].top + g.lanes[i].bottom) / 2,
            maxWidth: Math.max(40, labelWidth),
          }}
          title={lane.label}
          aria-hidden="true"
        >
          {lane.label}
        </span>
      ))}
      {today !== null && today >= domain[0] && today <= domain[1] && (
        <TodayLine
          x={Math.round(g.x(today)) + 0.5}
          label={todayLabel}
          names={barNames(g, model, labels, active)}
        />
      )}
    </>
  );
}

/** Today's hairline, open where it would cross a bar's name. */
function TodayLine({ x, label, names }: { x: number; label: string; names: readonly BarName[] }) {
  const clear = names
    .filter((name) => name.x - 4 <= x && name.x + name.width + 4 >= x)
    .map((name) => [name.y - 9, name.y + 9] as const);
  return <EventMarker x={x} label={label} clear={clear} />;
}

/* ——— Dependencies: quiet flat elbows, lit with the task in focus ——— */

function Connectors({
  geometryFor,
  model,
  active,
}: {
  geometryFor: (width: number) => TimelineGeometry;
  model: TimelineModel;
  active: number | null;
}) {
  const { size } = useChartFrame();
  if (!size.width) return null;
  const g = geometryFor(size.width);
  const links: { key: string; d: string; head: string; lit: boolean; dim: boolean }[] = [];
  const chain = relatedTasks(model, active);
  for (const t of model.tasks)
    for (const j of t.after) {
      const from = g.shapes[j],
        to = g.shapes[t.index];
      const ends = dependencyEnds(from, to);
      const down = to.cross >= from.cross;
      const laneFrom = g.lanes[model.tasks[j].lane];
      const between = down ? laneFrom.bottom : laneFrom.top;
      const lit = active !== null && chain.has(t.index) && chain.has(j);
      links.push({
        key: `${j}-${t.index}`,
        d: dependencyPath(ends.from, { x: ends.to.x - 1.5, y: ends.to.y }, { between }),
        head: arrowHead(ends.to.x - 1.5, ends.to.y),
        lit,
        dim: active !== null && !lit,
      });
    }
  if (!links.length) return null;
  return (
    <svg className={styles.connectors} aria-hidden="true">
      {links.map((link, i) => (
        <g
          key={link.key}
          className={styles.connector}
          data-lit={link.lit}
          data-dim={link.dim}
          style={{ animationDelay: `${0.7 + i * 0.03}s` }}
        >
          <path d={link.d} />
          <path d={link.head} />
        </g>
      ))}
    </svg>
  );
}

/* ——— Names on bars that share a row ——— */

type BarName = {
  key: string;
  x: number;
  y: number;
  /** The name's width, px. */
  width: number;
  text: string;
  inside: boolean;
  dim: boolean;
};

/** Where each bar's name sits: inside its bar when it fits, else just past its end when the track
 * has room there. */
function barNames(
  g: TimelineGeometry,
  model: TimelineModel,
  labels: LabelMode,
  active: number | null,
): BarName[] {
  const items: BarName[] = [];
  if (labels === "none") return items;
  const related = relatedTasks(model, active);
  for (const lane of model.lanes) {
    const shared =
      lane.tasks.length > 1 ||
      (lane.tasks.length === 1 && model.tasks[lane.tasks[0]].name !== lane.label);
    if (labels === "auto" && !shared) continue;
    for (const i of lane.tasks) {
      const t = model.tasks[i];
      const s = g.shapes[i];
      const r = taskRect(s);
      const text =
        labels === "all" && !t.milestone && t.progress !== null && t.progress < 1
          ? `${t.name} · ${Math.round(t.progress * 100)}%`
          : t.name;
      // The next task in the same track bounds a label set beside its bar.
      const next = lane.tasks
        .filter(
          (k) => k !== i && model.tasks[k].track === t.track && model.tasks[k].start >= t.start,
        )
        .map((k) => taskRect(g.shapes[k]).left);
      const limit = Math.min(g.plot.right, ...next);
      const w = textWidth(text, TYPE.caption);
      const visibleLeft = Math.max(r.left, g.plot.left);
      if (!t.milestone && fitsInBar(text, r.right - visibleLeft))
        items.push({
          key: t.id,
          x: visibleLeft + 9,
          y: s.cross,
          width: w,
          text,
          inside: true,
          dim: active !== null && !related.has(i),
        });
      else if (r.right + 8 + w <= limit - 6 && r.right + 8 >= g.plot.left)
        items.push({
          key: t.id,
          x: r.right + 8,
          y: s.cross,
          width: w,
          text,
          inside: false,
          dim: active !== null && !related.has(i),
        });
    }
  }
  return items;
}

function BarNames({
  geometryFor,
  model,
  labels,
  active,
}: {
  geometryFor: (width: number) => TimelineGeometry;
  model: TimelineModel;
  labels: LabelMode;
  active: number | null;
}) {
  const { size } = useChartFrame();
  if (!size.width || labels === "none") return null;
  const items = barNames(geometryFor(size.width), model, labels, active);
  return (
    <>
      {items.map((item) => (
        <span
          key={item.key}
          className={`${styles.barName} ${roles.caption}`}
          data-inside={item.inside}
          data-dim={item.dim}
          style={{ left: item.x, top: item.y }}
          aria-hidden="true"
        >
          {item.text}
        </span>
      ))}
    </>
  );
}

/* ——— The tooltip: dates, duration, progress, the plan and what it waits on ——— */

const TaskTooltip = forwardRef(function TaskTooltip(
  {
    task,
    model,
    colour,
    range,
    lateness,
    today,
  }: {
    task: ParsedTask;
    model: TimelineModel;
    colour: string;
    range: string;
    lateness: string | null;
    today: number | null;
  },
  ref: ForwardedRef<HTMLDivElement>,
) {
  const lane = model.lanes[task.lane];
  const group = lane.group === null ? null : model.groups[lane.group].label;
  const where = [group !== task.name ? group : null, lane.label !== task.name ? lane.label : null]
    .filter(Boolean)
    .join(" · ");
  const late = slip(task);
  let status: string | null = null;
  if (task.milestone && today !== null) {
    const reached = (task.progress ?? 0) >= 1 || task.start <= today;
    const days = Math.round((task.start - today) / DAY);
    status = reached ? "reached" : days <= 0 ? "due today" : `due in ${durationText(days * DAY)}`;
  }
  // The card's keys live here, not in a legend: what the hatching, the tick and the diamond mean.
  const tick = overrunShare(task) !== null;
  return (
    <div
      ref={ref}
      className={`${core.tooltip} ${styles.taskTip}`}
      style={{ "--tint": colour } as CSSProperties}
      aria-hidden="true"
    >
      {where && <span className={`${core.tooltipTitle} ${roles.casing}`}>{where}</span>}
      <span className={styles.taskTitle}>
        <span className={styles.taskSwatch} data-milestone={task.milestone} aria-hidden="true" />
        <span className={roles.label}>{task.name}</span>
      </span>
      <span className={styles.tipRow}>
        <span className={`${roles.label} ${core.tooltipLabel}`}>{range}</span>
        {!task.milestone && (
          <span className={`${roles.caption} ${mutedText}`}>
            {durationText(task.end - task.start)}
          </span>
        )}
      </span>
      {!task.milestone && task.progress !== null && (
        <span className={styles.progressRow}>
          <span className={styles.progressTrack} aria-hidden="true">
            <span style={{ width: `${task.progress * 100}%` }} />
          </span>
          <Figure value={task.progress} kind="percent" decimals={0} className={roles.figureS} />
          <span className={`${roles.caption} ${mutedText}`}>
            {task.progress < 1 ? "done, hatched still to do" : "done"}
          </span>
        </span>
      )}
      {task.milestone && (
        <span className={`${roles.caption} ${core.tooltipLabel}`}>
          Milestone{status ? ` · ${status}` : ""}
        </span>
      )}
      {lateness && (
        <span className={styles.slip} data-late={late > 0}>
          <span className={styles.slipMark} aria-hidden="true" />
          <span className={roles.caption}>
            {lateness} against plan
            {task.plannedEnd !== null && (
              <span className={mutedText}>
                {" "}
                (planned to end {endText(task.plannedEnd)}
                {tick ? ", at the tick" : ""})
              </span>
            )}
          </span>
        </span>
      )}
      {task.after.length > 0 && (
        <span className={`${roles.caption} ${mutedText}`}>
          After {task.after.map((j) => model.tasks[j].name).join(", ")}
        </span>
      )}
      {task.note && <span className={`${roles.caption} ${core.tooltipLabel}`}>{task.note}</span>}
    </div>
  );
});
