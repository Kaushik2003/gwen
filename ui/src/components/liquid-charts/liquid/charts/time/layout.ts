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
/**
 * The time family's layout maths: pure functions, no React and no DOM, so every decision the
 * candlestick and the timeline make can be unit-tested.
 *
 * - Candles sit on a band scale over trading sessions, not over calendar time: a weekend or a
 *   holiday is not a gap in the market, so it is not a gap on the axis. The axis is labelled where
 *   calendar months or Monday weeks begin (the core's `timeTicks`), at the first session on or
 *   after each one.
 * - The timeline packs tasks into lanes and tracks, maps time to px for a zoom level, routes
 *   dependencies as quiet elbows with rounded corners, and works out which tasks a focus lights.
 */

import {
  band,
  linear,
  niceDomain,
  timeTicks,
  type Band,
  type Linear,
  type TimeUnit,
} from "@/components/liquid-charts/liquid/core/scale";
import type { Insets, Rect } from "@/components/liquid-charts/liquid/core/geometry";
import { textWidth, TYPE } from "@/components/liquid-charts/liquid/core/format";
import type { LabelMode } from "@/components/liquid-charts/liquid/core/labels";
import type { Density } from "@/components/liquid-charts/liquid/core/style";

export type TimeRow = Record<string, unknown>;
export type TimeDensity = Density;

import { clamp, finite } from "@/components/liquid-charts/liquid/core/geometry";
import { DAY, HOUR, MINUTE } from "./units";

export { DAY, HOUR, MINUTE };

const r2 = (value: number) => Math.round(value * 100) / 100;

/** A number from a data cell; anything else (missing, null, text) is NaN. */
export const cell = (value: unknown): number =>
  typeof value === "number" ? value : typeof value === "string" && value.trim() ? +value : NaN;

/** A time in ms from a cell: a Date, a number of ms, or a date string. NaN otherwise. */
export function timeValue(value: unknown): number {
  if (value instanceof Date) return value.getTime();
  if (typeof value === "number") return value;
  if (typeof value === "string" && value.trim()) return Date.parse(value);
  return NaN;
}

/* ——————————————————————————— Candles ——————————————————————————— */

export type Candle = {
  time: number;
  open: number;
  high: number;
  low: number;
  close: number;
  /** NaN when the data has no volume. */
  volume: number;
};

export type CandleKeys = {
  date: string;
  open: string;
  high: string;
  low: string;
  close: string;
  volume?: string;
};

/** Candles from rows. A session missing any price keeps its place on the axis but draws nothing. */
export function readCandles(rows: readonly TimeRow[], keys: CandleKeys): Candle[] {
  return rows.map((row) => ({
    time: timeValue(row[keys.date]),
    open: cell(row[keys.open]),
    high: cell(row[keys.high]),
    low: cell(row[keys.low]),
    close: cell(row[keys.close]),
    volume: keys.volume ? cell(row[keys.volume]) : NaN,
  }));
}

/** True when every price of the session is known. */
export const complete = (c: Candle) =>
  finite(c.open) && finite(c.high) && finite(c.low) && finite(c.close);

/**
 * A trailing simple moving average over `window` values: the mean of this value and the
 * `window − 1` before it, NaN until there are that many, and NaN wherever one of them is missing,
 * so the line never invents a value. A window of 1 or less is the values themselves.
 */
export function movingAverage(values: readonly number[], window: number): number[] {
  const n = Math.max(1, Math.floor(window));
  const out = new Array<number>(values.length).fill(NaN);
  let sum = 0,
    run = 0;
  for (let i = 0; i < values.length; i++) {
    const v = values[i];
    if (!finite(v)) {
      sum = 0;
      run = 0;
      continue;
    }
    sum += v;
    run++;
    if (run > n) {
      sum -= values[i - n];
      run = n;
    }
    if (run === n) out[i] = sum / n;
  }
  return out;
}

export type CandleModel = {
  /** The sessions shown, oldest first. */
  candles: Candle[];
  /** Index in the full data of the first session shown. */
  offset: number;
  /** The moving average at each session shown (history before the range feeds it). */
  average: number[];
  /** Close against the previous session's close, as a share (NaN without a previous close). */
  change: number[];
  priceDomain: [number, number];
  volumeDomain: [number, number];
  /** The session with the highest high, the lowest low and the newest close, or null. */
  high: number | null;
  low: number | null;
  latest: number | null;
  hasVolume: boolean;
  /** A window in session units (a brushed view, fractional while it springs): where it starts
   * past the first session shown, and how many sessions wide it is. Absent, the sessions shown
   * fill the plot. */
  view?: { from: number; span: number };
};

/**
 * What the candlestick shows: the latest `range` sessions (all when null), a moving average over
 * `average` sessions fed by any history before the range, each session's change on the one before
 * (also from history), and domains that hold every wick and the average.
 *
 * A `window` shows the sessions under it instead: session i spans [i, i + 1), so [10, 30) is the
 * eleventh to the thirtieth; a fractional window (a view springing between ranges) takes every
 * session it touches and records how far it reaches into them. `priceDomain` overrides the price
 * domain (a springing domain).
 */
export function candleModel(
  all: readonly Candle[],
  {
    range = null,
    average = 0,
    window = null,
    priceDomain,
  }: {
    range?: number | null;
    average?: number;
    window?: readonly [number, number] | null;
    priceDomain?: readonly [number, number];
  } = {},
): CandleModel {
  const span =
    window && finite(window[0]) && finite(window[1])
      ? ([
          clamp(Math.min(window[0], window[1]), 0, all.length),
          clamp(Math.max(window[0], window[1]), 0, all.length),
        ] as const)
      : null;
  const first = span ? Math.min(all.length, Math.floor(span[0] + 1e-9)) : 0;
  const end = span ? Math.max(first, Math.ceil(span[1] - 1e-9)) : 0;
  const count = span
    ? end - first
    : range === null || !finite(range)
      ? all.length
      : clamp(Math.floor(range), 0, all.length);
  const offset = span ? first : all.length - count;
  const candles = all.slice(offset, offset + count);
  const closes = all.map((c) => (complete(c) ? c.close : NaN));
  const ma =
    average > 0
      ? movingAverage(closes, average).slice(offset, offset + count)
      : candles.map(() => NaN);
  const change = candles.map((c, i) => {
    const before = closes[offset + i - 1];
    return complete(c) && finite(before) && before !== 0 ? (c.close - before) / before : NaN;
  });
  let high: number | null = null,
    low: number | null = null,
    latest: number | null = null;
  const prices: number[] = [];
  let peakVolume = 0,
    hasVolume = false;
  candles.forEach((c, i) => {
    if (finite(c.volume) && c.volume >= 0) {
      hasVolume = true;
      peakVolume = Math.max(peakVolume, c.volume);
    }
    if (!complete(c)) return;
    prices.push(c.high, c.low);
    if (high === null || c.high > candles[high].high) high = i;
    if (low === null || c.low < candles[low].low) low = i;
    latest = i;
  });
  for (const v of ma) if (finite(v)) prices.push(v);
  return {
    candles,
    offset,
    average: ma,
    change,
    priceDomain: priceDomain
      ? [priceDomain[0], priceDomain[1]]
      : niceDomain(prices, { includeZero: false, count: 6 }),
    volumeDomain: niceDomain([peakVolume], { count: 4 }),
    high,
    low,
    latest,
    hasVolume,
    ...(span ? { view: { from: span[0] - offset, span: Math.max(1e-6, span[1] - span[0]) } } : {}),
  };
}

/**
 * The band scale for a window of sessions: d3 scaleBand's padding over a fractional number of
 * bands, shifted by `from`, so a whole window lays out exactly as `band` does and a springing one
 * slides between. `indexAt` finds only sessions whose centre lies inside the range.
 */
export function windowBand(
  count: number,
  range: readonly [number, number],
  { from, span }: { from: number; span: number },
  {
    paddingInner = 0.32,
    paddingOuter = 0.28,
  }: { paddingInner?: number; paddingOuter?: number } = {},
): Band {
  const [lo, hi] = range[0] <= range[1] ? range : [range[1], range[0]];
  const step = (hi - lo) / Math.max(1e-9, span - paddingInner + paddingOuter * 2);
  const first = lo + (hi - lo - step * (span - paddingInner)) / 2;
  const bandwidth = step * (1 - paddingInner);
  const start = (index: number) => first + step * (index - from);
  const centre = (index: number) => start(index) + bandwidth / 2;
  return {
    count,
    step,
    bandwidth,
    start,
    centre,
    indexAt: (px) => {
      if (count < 1 || hi === lo || px < lo || px > hi) return null;
      let i = Math.floor((px - first + (step - bandwidth) / 2) / step + from);
      i = clamp(i, 0, count - 1);
      // Step inwards past sessions whose centre falls outside the range.
      while (i > 0 && centre(i) > hi) i--;
      while (i < count - 1 && centre(i) < lo) i++;
      return i;
    },
  };
}

/**
 * Axis labels for sessions: calendar ticks (months, or Monday weeks for a short range, from the
 * core's `timeTicks`) each placed on the first session on or after it, so a holiday Monday labels
 * the Tuesday. Two ticks that land on one session keep the first.
 */
export function sessionTicks(
  times: readonly number[],
  width: number,
  { timeZone = "UTC", locale = "en-US" }: { timeZone?: string; locale?: string } = {},
): { index: number; label: string }[] {
  const known = times.filter(finite);
  if (known.length < 2) return [];
  const from = known[0],
    to = known[known.length - 1];
  // At least three: two ticks over a few months can both land outside it, leaving one month name.
  const count = Math.max(3, Math.floor(width / 110));
  const ticks = timeTicks([from, to], count, { timeZone, locale });
  // Weeks and days are labelled with the session's own date (a holiday Monday's tick reads as the
  // Tuesday it lands on); months keep the month's name.
  const daily =
    ticks.length > 1 && ticks[1].value - ticks[0].value < 28 * DAY
      ? new Intl.DateTimeFormat(locale, { month: "short", day: "numeric", timeZone })
      : null;
  const out: { index: number; label: string }[] = [];
  let i = 0;
  for (const tick of ticks) {
    while (i < times.length && !(times[i] >= tick.value)) i++;
    if (i >= times.length) break;
    if (out.length && out[out.length - 1].index === i) continue;
    out.push({ index: i, label: daily ? daily.format(times[i]) : tick.label });
  }
  return out;
}

/** Which session labels show when they would collide: every one that clears the last shown by its
 * width plus some air; the focused session's label always shows and its neighbours step aside. */
export function sessionLabelVisibility(
  ticks: readonly { index: number; label: string }[],
  centre: (index: number) => number,
  air = 14,
): boolean[] {
  let right = -Infinity;
  return ticks.map((tick) => {
    const half = textWidth(tick.label, TYPE.label) / 2;
    const x = centre(tick.index);
    if (x - half < right + air) return false;
    right = x + half;
    return true;
  });
}

export type CandleDensity = Density;

export const CANDLE_DENSITY = {
  tile: { height: 200, thickness: 10, ticks: 2 },
  standard: { height: 340, thickness: 18, ticks: 4 },
  hero: { height: 400, thickness: 22, ticks: 4 },
} as const satisfies Record<CandleDensity, { height: number; thickness: number; ticks: number }>;

/** Room round the plot: price ticks on the left, the latest price's tag on the right, the date
 * axis below, and lanes for event tags (plus the period's high label) above. */
export function candleInsets({
  ticks,
  latest,
  lanes = 0,
  density = "standard",
}: {
  ticks: readonly string[];
  /** The latest price as text, for its tag. */
  latest?: string;
  /** Event label lanes to make room for. */
  lanes?: number;
  density?: CandleDensity;
}): Insets {
  const tile = density === "tile";
  const tickWidth = ticks.reduce((w, t) => Math.max(w, textWidth(t, TYPE.micro)), 0);
  return {
    left: Math.ceil(Math.max(tile ? 22 : 30, tickWidth + 14)),
    right: latest ? Math.ceil(textWidth(latest, TYPE.micro) + 24) : tile ? 8 : 12,
    top: lanes > 0 ? 8 + lanes * 20 : tile ? 8 : 12,
    bottom: tile ? 22 : 28,
  };
}

export type CandleShape = {
  index: number;
  cross: number;
  up: boolean;
  /** Price px. */
  open: number;
  close: number;
  high: number;
  low: number;
  /** The glass body: grows from the open towards the close (up for a rise, down for a fall). */
  base: number;
  length: number;
  direction: "up" | "down";
  /** The volume bar under it, or null. */
  volume: { base: number; length: number } | null;
};

export type CandleGeometry = {
  plot: Rect;
  /** The price pane and the volume pane (null without volume). */
  price: Rect;
  volume: Rect | null;
  slots: Band;
  thickness: number;
  /** Price and volume to px. */
  y: Linear;
  v: Linear;
  shapes: (CandleShape | null)[];
};

/** A doji (open equal to close) still shows as a sliver of glass this thick, px. */
export const DOJI = 1.5;

/**
 * Where every candle goes for a size: a band per session across the plot, prices in the upper
 * pane and volume in a quieter pane under it (about a fifth of the plot), with room at the price
 * pane's top and bottom for the high and low labels.
 */
export function candleGeometry(
  model: CandleModel,
  {
    width,
    height,
    insets,
    maxThickness = 18,
    volumeShare = 0.2,
    gap = 14,
    labelRoom = 18,
  }: {
    width: number;
    height: number;
    insets: Insets;
    maxThickness?: number;
    volumeShare?: number;
    gap?: number;
    /** Px kept clear above the highest wick and below the lowest, for their labels. */
    labelRoom?: number;
  },
): CandleGeometry {
  const plot: Rect = {
    left: insets.left,
    top: insets.top,
    right: Math.max(insets.left + 1, width - insets.right),
    bottom: Math.max(insets.top + 1, height - insets.bottom),
  };
  const h = plot.bottom - plot.top;
  const volumeHeight = model.hasVolume ? Math.max(18, Math.round(h * volumeShare)) : 0;
  const volume: Rect | null = model.hasVolume
    ? { left: plot.left, right: plot.right, top: plot.bottom - volumeHeight, bottom: plot.bottom }
    : null;
  const price: Rect = {
    left: plot.left,
    right: plot.right,
    top: plot.top,
    bottom: volume ? volume.top - gap : plot.bottom,
  };
  const n = model.candles.length;
  const slots = model.view
    ? windowBand(n, [plot.left, plot.right], model.view)
    : band(n, [plot.left, plot.right], { paddingInner: 0.32, paddingOuter: 0.28 });
  const thickness = Math.max(1, Math.min(maxThickness, slots.bandwidth));
  const room = Math.min(labelRoom, (price.bottom - price.top) / 6);
  const y = linear(model.priceDomain, [price.bottom - room, price.top + room]);
  const v = linear(model.volumeDomain, [volume ? volume.bottom : plot.bottom, volume?.top ?? 0]);
  const shapes = model.candles.map((c, index): CandleShape | null => {
    if (!complete(c)) return null;
    // In a window, a session whose centre lies outside the plot is out of view.
    if (model.view) {
      const x = slots.centre(index);
      if (x < plot.left - 0.5 || x > plot.right + 0.5) return null;
    }
    const up = c.close >= c.open;
    const yo = y(c.open),
      yc = y(c.close);
    let base = yo,
      length = Math.abs(yo - yc);
    if (length < DOJI) {
      // Centre a doji's sliver on its price.
      base = (yo + yc) / 2 + (up ? DOJI / 2 : -DOJI / 2);
      length = DOJI;
    }
    const vol =
      volume && finite(c.volume) && c.volume > 0
        ? { base: volume.bottom, length: Math.max(1, volume.bottom - v(c.volume)) }
        : null;
    return {
      index,
      cross: r2(slots.centre(index)),
      up,
      open: r2(yo),
      close: r2(yc),
      high: r2(y(c.high)),
      low: r2(y(c.low)),
      base: r2(base),
      length: r2(length),
      direction: up ? "up" : "down",
      volume: vol && { base: r2(vol.base), length: r2(vol.length) },
    };
  });
  return { plot, price, volume, slots, thickness, y, v, shapes };
}

/** The candle's full extent, wick to wick, as a rect (the tooltip's anchor). */
export function candleExtent(shape: CandleShape, thickness: number): Rect {
  const half = Math.max(thickness, 6) / 2;
  return {
    left: shape.cross - half,
    right: shape.cross + half,
    top: Math.min(shape.high, shape.low),
    bottom: Math.max(shape.high, shape.low),
  };
}

/** The wick above the body and the wick below it, as y ranges (empty when the body reaches). */
export function wicks(shape: CandleShape) {
  const top = Math.min(shape.open, shape.close),
    bottom = Math.max(shape.open, shape.close);
  return {
    upper: shape.high < top - 0.25 ? ([shape.high, top] as const) : null,
    lower: shape.low > bottom + 0.25 ? ([bottom, shape.low] as const) : null,
  };
}

/**
 * Which candles carry a label at rest: `auto` and `all` mark the period's high (above its wick)
 * and low (below its wick); `ends` marks the first and the latest close; `none` nothing. The
 * focused candle's figures are in the tooltip, so it never takes a label.
 */
export function candleLabelPlan(
  model: CandleModel,
  mode: LabelMode,
): {
  id: string;
  index: number;
  value: number;
  side: "above" | "below";
  kind: "high" | "low" | "close";
}[] {
  if (mode === "none" || !model.candles.length) return [];
  if (mode === "ends") {
    const first = model.candles.findIndex(complete);
    const out = [];
    if (first >= 0)
      out.push({
        id: "first",
        index: first,
        value: model.candles[first].close,
        side: "above" as const,
        kind: "close" as const,
      });
    if (model.latest !== null && model.latest !== first)
      out.push({
        id: "latest",
        index: model.latest,
        value: model.candles[model.latest].close,
        side: "above" as const,
        kind: "close" as const,
      });
    return out;
  }
  const out = [];
  if (model.high !== null)
    out.push({
      id: "high",
      index: model.high,
      value: model.candles[model.high].high,
      side: "above" as const,
      kind: "high" as const,
    });
  if (model.low !== null && model.low !== model.high)
    out.push({
      id: "low",
      index: model.low,
      value: model.candles[model.low].low,
      side: "below" as const,
      kind: "low" as const,
    });
  return out;
}

/** The session index for an event given by index or by time: the first session on or after it,
 * or null when it falls outside the sessions shown. */
export function sessionIndex(times: readonly number[], at: number): number | null {
  if (!finite(at) || !times.length) return null;
  for (let i = 0; i < times.length; i++)
    if (times[i] >= at) return times[i] - at < 7 * DAY ? i : null;
  return null;
}

/** An SVG polyline through points, a subpath per run of finite ys. */
export function polylinePath(xs: readonly number[], ys: readonly number[]): string {
  let d = "";
  let open = false;
  for (let i = 0; i < Math.min(xs.length, ys.length); i++) {
    if (!finite(ys[i]) || !finite(xs[i])) {
      open = false;
      continue;
    }
    d += `${open ? "L" : "M"}${r2(xs[i])} ${r2(ys[i])}`;
    open = true;
  }
  return d;
}

/* ——————————————————————————— Timeline ——————————————————————————— */

export type TimelineTask = {
  id: string;
  name: string;
  /** The row it sits in; tasks sharing a lane share a row (packed into tracks when they overlap).
   * Default: the task's own row, named after it. */
  lane?: string;
  /** A heading over a run of lanes. */
  group?: string;
  start: Date | number | string;
  /** Exclusive end. Omitted (or equal to the start) for a milestone. */
  end?: Date | number | string;
  /** Draw as a milestone even with an end. */
  milestone?: boolean;
  /** Share done, 0–1: the bar is a container holding liquid to it. Omitted means not tracked
   * (drawn full). For a milestone, 1 means reached. */
  progress?: number;
  /** The planned end. When the task runs past it, the plan's end is engraved across the bar. */
  plannedEnd?: Date | number | string;
  /** Ids of the tasks this one waits on. */
  after?: readonly string[];
  /** The config key that colours it. Default: its group, then its lane. */
  series?: string;
  /** A line for the tooltip. */
  note?: string;
};

export type ParsedTask = {
  index: number;
  id: string;
  name: string;
  lane: number;
  track: number;
  start: number;
  end: number;
  milestone: boolean;
  progress: number | null;
  plannedEnd: number | null;
  /** Indices of the tasks it waits on, and of the ones waiting on it. */
  after: number[];
  before: number[];
  series: string;
  note?: string;
};

export type TimelineLane = {
  key: string;
  label: string;
  group: number | null;
  tracks: number;
  tasks: number[];
};

export type TimelineModel = {
  tasks: ParsedTask[];
  lanes: TimelineLane[];
  groups: { key: string; label: string; lanes: number[] }[];
  /** Keyboard order: lane by lane, earliest first. Task indices. */
  order: number[];
  /** The extent of every task (and its plan), ms. */
  extent: [number, number];
};

/**
 * Packs intervals into as few tracks as keeps them apart: each, earliest first, goes into the first
 * track whose last interval has ended by its start (plus `gap`). Returns each interval's track.
 */
export function packTracks(
  intervals: readonly { start: number; end: number }[],
  gap = 0,
): number[] {
  const order = intervals
    .map((_, i) => i)
    .sort(
      (a, b) =>
        intervals[a].start - intervals[b].start || intervals[a].end - intervals[b].end || a - b,
    );
  const ends: number[] = [];
  const out = new Array<number>(intervals.length).fill(0);
  for (const i of order) {
    const { start, end } = intervals[i];
    let track = ends.findIndex((e) => e + gap <= start);
    if (track < 0) {
      track = ends.length;
      ends.push(-Infinity);
    }
    ends[track] = Math.max(start, end);
    out[i] = track;
  }
  return out;
}

/**
 * Tasks read into lanes, groups and tracks. Lanes and groups keep the order they first appear in;
 * a task with no lane gets its own. A task with no valid start is dropped; an end before its start
 * is read as a milestone at the start. Dependencies on unknown ids are ignored.
 */
export function timelineModel(input: readonly TimelineTask[]): TimelineModel {
  const valid = input.filter((t) => finite(timeValue(t.start)));
  const groupKeys: string[] = [];
  const laneKeys: string[] = [];
  const lanes: TimelineLane[] = [];
  const tasks: ParsedTask[] = valid.map((t, index) => {
    const start = timeValue(t.start);
    const rawEnd = t.end === undefined ? NaN : timeValue(t.end);
    const milestone = !!t.milestone || !finite(rawEnd) || rawEnd <= start;
    const laneKey = t.lane ?? `task:${t.id}`;
    let lane = laneKeys.indexOf(laneKey);
    const groupKey = t.group ?? null;
    let group = groupKey === null ? -1 : groupKeys.indexOf(groupKey);
    if (groupKey !== null && group < 0) {
      group = groupKeys.length;
      groupKeys.push(groupKey);
    }
    if (lane < 0) {
      lane = laneKeys.length;
      laneKeys.push(laneKey);
      lanes.push({
        key: laneKey,
        label: t.lane ?? t.name,
        group: group < 0 ? null : group,
        tracks: 1,
        tasks: [],
      });
    }
    lanes[lane].tasks.push(index);
    const planned = t.plannedEnd === undefined ? NaN : timeValue(t.plannedEnd);
    const progress =
      t.progress === undefined || !finite(t.progress) ? null : clamp(t.progress, 0, 1);
    return {
      index,
      id: t.id,
      name: t.name,
      lane,
      track: 0,
      start,
      end: milestone ? start : rawEnd,
      milestone,
      progress,
      plannedEnd: finite(planned) ? planned : null,
      after: [],
      before: [],
      series: t.series ?? t.group ?? t.lane ?? t.id,
      note: t.note,
    };
  });
  const byId = new Map(tasks.map((t) => [t.id, t.index]));
  valid.forEach((t, i) => {
    for (const id of t.after ?? []) {
      const j = byId.get(id);
      if (j === undefined || j === i || tasks[i].after.includes(j)) continue;
      tasks[i].after.push(j);
      tasks[j].before.push(i);
    }
  });
  // Lanes in group order (lanes without a group first, in their own order), then tracks.
  const order = lanes
    .map((_, i) => i)
    .sort((a, b) => {
      const ga = lanes[a].group ?? -1,
        gb = lanes[b].group ?? -1;
      return ga - gb || a - b;
    });
  const sortedLanes = order.map((i) => lanes[i]);
  const laneIndex = new Map(order.map((old, now) => [old, now]));
  for (const t of tasks) t.lane = laneIndex.get(t.lane)!;
  // A milestone takes a little time either side (about its diamond's width on a chart of the
  // whole span), so it never sits on the end of a bar in the same track.
  let first = Infinity,
    final = -Infinity;
  for (const t of tasks) {
    first = Math.min(first, t.start);
    final = Math.max(final, t.end);
  }
  const room = final > first ? (final - first) * 0.022 : 0;
  for (const lane of sortedLanes) {
    // Spans first, so a sequence of work stays on one track; then each milestone goes into the
    // first track where it touches nothing.
    const spans = lane.tasks.filter((i) => !tasks[i].milestone);
    const placed: { start: number; end: number; track: number }[] = [];
    packTracks(spans.map((i) => ({ start: tasks[i].start, end: tasks[i].end }))).forEach(
      (track, k) => {
        tasks[spans[k]].track = track;
        placed.push({ start: tasks[spans[k]].start, end: tasks[spans[k]].end, track });
      },
    );
    for (const i of lane.tasks) {
      const t = tasks[i];
      if (!t.milestone) continue;
      const lo = t.start - room,
        hi = t.start + room;
      let track = 0;
      while (placed.some((p) => p.track === track && p.start < hi && lo < p.end)) track++;
      t.track = track;
      placed.push({ start: lo, end: hi, track });
    }
    lane.tracks = Math.max(1, ...lane.tasks.map((i) => tasks[i].track + 1));
  }
  const groups = groupKeys.map((key) => ({
    key,
    label: key,
    lanes: sortedLanes.flatMap((lane, i) => (lane.group === groupKeys.indexOf(key) ? [i] : [])),
  }));
  const keyboard = tasks
    .map((t) => t.index)
    .sort((a, b) => {
      const ta = tasks[a],
        tb = tasks[b];
      return ta.lane - tb.lane || ta.track - tb.track || ta.start - tb.start || a - b;
    });
  let lo = Infinity,
    hi = -Infinity;
  for (const t of tasks) {
    lo = Math.min(lo, t.start);
    hi = Math.max(hi, t.end, t.plannedEnd ?? -Infinity);
  }
  return {
    tasks,
    lanes: sortedLanes,
    groups,
    order: keyboard,
    extent: lo <= hi ? [lo, hi] : [0, DAY],
  };
}

/** The unit of a timeline's time axis: hours, days, Monday weeks, months or quarters. `auto` suits
 * the span. */
export type TimelineUnit = "auto" | "hours" | "days" | "weeks" | "months" | "quarters";
/** @deprecated Renamed `TimelineUnit`: it sets the time axis's unit, not a zoom. */
export type TimelineZoom = TimelineUnit;

/** The time a zoom level's tick spans, roughly, ms. */
const ZOOM_STEP: Record<Exclude<TimelineZoom, "auto">, number> = {
  hours: HOUR,
  days: DAY,
  weeks: 7 * DAY,
  months: 30.44 * DAY,
  quarters: 91.3 * DAY,
};

/**
 * The span shown: `window` when given, else every task (and its plan, and today) with a little air
 * either side (a share of the span, at least half a tick of the zoom level).
 */
export function timelineDomain(
  extent: readonly [number, number],
  {
    window,
    today,
    zoom = "auto",
  }: { window?: readonly [number, number] | null; today?: number | null; zoom?: TimelineZoom } = {},
): [number, number] {
  if (window && finite(window[0]) && finite(window[1]) && window[1] > window[0])
    return [window[0], window[1]];
  let lo = extent[0],
    hi = extent[1];
  if (today !== null && today !== undefined && finite(today)) {
    lo = Math.min(lo, today);
    hi = Math.max(hi, today);
  }
  if (!(hi > lo)) {
    const pad = zoom === "auto" ? DAY : ZOOM_STEP[zoom];
    return [lo - pad, hi + pad];
  }
  const span = hi - lo;
  const pad = Math.max(span * 0.03, zoom === "auto" ? 0 : ZOOM_STEP[zoom] * 0.35);
  return [lo - pad, hi + pad];
}

/** Ms of the calendar parts (y, m, d, h, minute) of `ms` in a time zone, read as UTC. */
function zoned(ms: number, timeZone: string) {
  if (timeZone === "UTC") {
    const d = new Date(ms);
    return {
      year: d.getUTCFullYear(),
      month: d.getUTCMonth(),
      day: d.getUTCDate(),
      hour: d.getUTCHours(),
      minute: d.getUTCMinutes(),
      weekday: d.getUTCDay(),
    };
  }
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone,
    year: "numeric",
    month: "numeric",
    day: "numeric",
    hour: "numeric",
    minute: "numeric",
    weekday: "short",
    hourCycle: "h23",
  }).formatToParts(new Date(ms));
  const get = (type: string) => parts.find((p) => p.type === type)?.value ?? "0";
  const weekdays = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
  return {
    year: +get("year"),
    month: +get("month") - 1,
    day: +get("day"),
    hour: +get("hour") % 24,
    minute: +get("minute"),
    weekday: weekdays.indexOf(get("weekday")),
  };
}

const ZOOM_UNIT: Record<Exclude<TimelineZoom, "auto">, [TimeUnit, number]> = {
  hours: ["hour", 1],
  days: ["day", 1],
  weeks: ["week", 1],
  months: ["month", 1],
  quarters: ["quarter", 1],
};

/** Steps the timeline's axis can take, finest first: [unit, multiple, rough ms]. */
const STEPS: [TimeUnit, number, number][] = [
  ["minute", 5, 5 * MINUTE],
  ["minute", 10, 10 * MINUTE],
  ["minute", 15, 15 * MINUTE],
  ["minute", 30, 30 * MINUTE],
  ["hour", 1, HOUR],
  ["hour", 2, 2 * HOUR],
  ["hour", 3, 3 * HOUR],
  ["hour", 6, 6 * HOUR],
  ["hour", 12, 12 * HOUR],
  ["day", 1, DAY],
  ["day", 2, 2 * DAY],
  ["week", 1, 7 * DAY],
  ["week", 2, 14 * DAY],
  ["month", 1, 30.44 * DAY],
  ["quarter", 1, 91.3 * DAY],
  ["month", 6, 182.6 * DAY],
  ["year", 1, 365.25 * DAY],
];

/** The core's `timeTicks` for a forced unit, labelled as a Gantt reads: "14:30" (24-hour),
 * "Oct 12", "Oct" ("Jan 2027" when a year turns), "Q4 2026" and "2027". */
const calendarTicks = (
  domain: readonly [number, number],
  unit: TimeUnit,
  every: number,
  timeZone: string,
) =>
  timeTicks(domain, 6, {
    unit,
    every,
    timeZone,
    locale: "en-US",
    years: unit === "quarter" ? "every" : "turn",
  });

/**
 * Ticks for a zoom level: hours, days, Monday weeks, months or quarters (calendar aligned, from
 * the core's `timeTicks`). `auto` takes the finest step that leaves about one tick per 96px.
 */
export function zoomTicks(
  domain: readonly [number, number],
  zoom: TimelineZoom,
  { width = 600, timeZone = "UTC" }: { width?: number; timeZone?: string } = {},
): { value: number; label: string }[] {
  const [from, to] = domain;
  if (!(to > from) || !finite(from) || !finite(to)) return [];
  if (zoom !== "auto") {
    const [unit, step] = ZOOM_UNIT[zoom];
    return calendarTicks(domain, unit, step, timeZone);
  }
  const wanted = Math.max(2, Math.floor(width / 96));
  const [unit, step] =
    STEPS.find(([, , ms]) => (to - from) / ms <= wanted) ?? STEPS[STEPS.length - 1];
  return calendarTicks(domain, unit, step, timeZone);
}

/** Which ticks keep their label: every k-th, where k fits the widest label with some air. */
export function tickVisibility(
  ticks: readonly { value: number; label: string }[],
  x: (value: number) => number,
  air = 16,
): boolean[] {
  if (ticks.length < 2) return ticks.map(() => true);
  const widest = Math.max(...ticks.map((t) => textWidth(t.label, TYPE.label)));
  const pitch = Math.abs(x(ticks[1].value) - x(ticks[0].value));
  const k = pitch > 0 ? Math.max(1, Math.ceil((widest + air) / pitch)) : 1;
  return ticks.map((_, i) => i % k === 0);
}

export const TIMELINE_DENSITY = {
  tile: { row: 24, header: 22 },
  standard: { row: 32, header: 28 },
  hero: { row: 36, header: 30 },
} as const satisfies Record<TimeDensity, { row: number; header: number }>;

/** Room for the row labels on the left (capped so the bars keep most of a narrow card), the time
 * axis below, and the today tag above. */
export function timelineInsets({
  lanes,
  width,
  density = "standard",
  today = false,
}: {
  lanes: readonly string[];
  width: number;
  density?: TimeDensity;
  today?: boolean;
}): Insets {
  const tile = density === "tile";
  const widest = lanes.reduce((w, label) => Math.max(w, textWidth(label, TYPE.label)), 0);
  const cap = Math.max(84, Math.round(width * (width < 560 ? 0.34 : 0.26)));
  return {
    left: Math.ceil(Math.min(cap, widest + 22)),
    right: tile ? 10 : 16,
    top: today ? 26 : tile ? 8 : 12,
    bottom: tile ? 24 : 30,
  };
}

export type TaskShape = {
  index: number;
  /** Row centre (the bar's cross position), and the bar's start and length along x. */
  cross: number;
  base: number;
  length: number;
  thickness: number;
  milestone: boolean;
  /** For a milestone: the diamond's centre and size (corner to corner). */
  size: number;
};

export type TimelineGeometry = {
  plot: Rect;
  x: Linear;
  rowHeight: number;
  /** Each lane's band, top to bottom. */
  lanes: { top: number; bottom: number }[];
  /** Group headings: where each sits. */
  headers: { label: string; top: number; height: number }[];
  shapes: TaskShape[];
  /** The total height the rows need (insets included), px. */
  height: number;
};

/**
 * Where every row, bar and milestone goes. Rows are `rowHeight` per track; a group heading takes a
 * row of its own above its lanes. Bars are about half a row thick; a milestone is a diamond a
 * little larger than a bar is thick. Times outside the domain map beyond the plot (the chart fades
 * the edges, so a bar that runs on reads as running on).
 */
export function timelineGeometry(
  model: TimelineModel,
  {
    width,
    insets,
    domain,
    density = "standard",
    rowHeight,
  }: {
    width: number;
    insets: Insets;
    domain: readonly [number, number];
    density?: TimeDensity;
    rowHeight?: number;
  },
): TimelineGeometry {
  const d = TIMELINE_DENSITY[density];
  const row = rowHeight ?? d.row;
  const header = d.header;
  const left = insets.left,
    right = Math.max(insets.left + 1, width - insets.right);
  let y = insets.top;
  const lanes: { top: number; bottom: number }[] = [];
  const headers: { label: string; top: number; height: number }[] = [];
  let group: number | null | undefined = undefined;
  for (const lane of model.lanes) {
    if (lane.group !== group) {
      group = lane.group;
      if (group !== null) {
        headers.push({ label: model.groups[group].label, top: y, height: header });
        y += header;
      }
    }
    lanes.push({ top: y, bottom: y + row * lane.tracks });
    y += row * lane.tracks;
  }
  const plot: Rect = { left, right, top: insets.top, bottom: Math.max(y, insets.top + row) };
  const x = linear([domain[0], domain[1]], [left, right]);
  const thickness = Math.max(6, Math.round(row * 0.5));
  const size = Math.round(thickness * 1.3);
  const shapes = model.tasks.map((t): TaskShape => {
    const lane = lanes[t.lane];
    const cross = r2(lane.top + row * (t.track + 0.5));
    if (t.milestone) {
      const at = r2(x(t.start));
      return { index: t.index, cross, base: at, length: 0, thickness, milestone: true, size };
    }
    const base = x(t.start);
    return {
      index: t.index,
      cross,
      base: r2(base),
      length: r2(Math.max(4, x(t.end) - base)),
      thickness,
      milestone: false,
      size,
    };
  });
  return {
    plot,
    x,
    rowHeight: row,
    lanes,
    headers,
    shapes,
    height: plot.bottom + insets.bottom,
  };
}

/** A task's rect: the bar, or the diamond's bounding square. */
export function taskRect(shape: TaskShape): Rect {
  if (shape.milestone) {
    const h = shape.size / 2;
    return {
      left: shape.base - h,
      right: shape.base + h,
      top: shape.cross - h,
      bottom: shape.cross + h,
    };
  }
  const h = shape.thickness / 2;
  return {
    left: shape.base,
    right: shape.base + shape.length,
    top: shape.cross - h,
    bottom: shape.cross + h,
  };
}

/**
 * The task under a point: the lane under y, then the task in it whose rect holds x, else the
 * nearest one within `reach` px (so thin bars and small diamonds are easy to find). Null between
 * lanes, outside the plot, or far from every task.
 */
export function taskAt(
  model: TimelineModel,
  geometry: TimelineGeometry,
  px: number,
  py: number,
  reach = 28,
): number | null {
  const lane = geometry.lanes.findIndex((l) => py >= l.top && py < l.bottom);
  if (lane < 0 || px < geometry.plot.left - 6 || px > geometry.plot.right + 6) return null;
  const track = Math.floor((py - geometry.lanes[lane].top) / geometry.rowHeight);
  let best: number | null = null,
    distance = Infinity;
  for (const i of model.lanes[lane].tasks) {
    const t = model.tasks[i];
    if (t.track !== track && model.lanes[lane].tracks > 1) continue;
    const r = taskRect(geometry.shapes[i]);
    const dx = px < r.left ? r.left - px : px > r.right ? px - r.right : 0;
    if (dx < distance) {
      distance = dx;
      best = i;
    }
  }
  return distance <= reach ? best : null;
}

/** From a task, the nearest task (in time) in the lane above or below, for Up and Down. */
export function taskInNextLane(model: TimelineModel, from: number, step: -1 | 1): number | null {
  const t = model.tasks[from];
  if (!t) return null;
  const middle = (task: ParsedTask) => (task.start + task.end) / 2;
  for (let lane = t.lane + step; lane >= 0 && lane < model.lanes.length; lane += step) {
    const candidates = model.lanes[lane].tasks;
    if (!candidates.length) continue;
    let best = candidates[0];
    for (const i of candidates)
      if (
        Math.abs(middle(model.tasks[i]) - middle(t)) <
        Math.abs(middle(model.tasks[best]) - middle(t))
      )
        best = i;
    return best;
  }
  return null;
}

/** The tasks a focus lights: the task itself, what it waits on and what waits on it. */
export function relatedTasks(model: TimelineModel, index: number | null): Set<number> {
  const lit = new Set<number>();
  const t = index === null ? undefined : model.tasks[index];
  if (!t) return lit;
  lit.add(t.index);
  for (const j of t.after) lit.add(j);
  for (const j of t.before) lit.add(j);
  return lit;
}

/** Where a dependency leaves a task (the end of a bar, the right corner of a diamond) and where it
 * enters one (the start, or the left corner). */
export function dependencyEnds(from: TaskShape, to: TaskShape) {
  return {
    from: {
      x: from.milestone ? from.base + from.size / 2 : from.base + from.length,
      y: from.cross,
    },
    to: { x: to.milestone ? to.base - to.size / 2 : to.base, y: to.cross },
  };
}

/**
 * A dependency as a quiet elbow with rounded corners: out of the earlier task to the right, down
 * (or up) to the later task's row, and into its start. When the later task starts before there is
 * room to turn (it overlaps, or starts right away), the line loops: out, down to between the rows,
 * back left past the start, down, and in. Corners are arcs of `radius` px, never wider than half
 * the run they join. Ends in a small arrowhead at the later task.
 */
export function dependencyPath(
  from: { x: number; y: number },
  to: { x: number; y: number },
  { lead = 8, radius = 5, between }: { lead?: number; radius?: number; between?: number } = {},
): string {
  const dy = to.y - from.y;
  const sy = dy >= 0 ? 1 : -1;
  const p = (x: number, y: number) => `${r2(x)} ${r2(y)}`;
  if (Math.abs(dy) < 0.5) return `M${p(from.x, from.y)}H${r2(to.x)}`;
  const room = to.x - from.x;
  if (room >= lead * 2) {
    // One step: right, down, right.
    const turn = from.x + Math.min(lead + 4, room / 2);
    const r = Math.max(0, Math.min(radius, Math.abs(dy) / 2, turn - from.x, to.x - turn));
    return (
      `M${p(from.x, from.y)}H${r2(turn - r)}` +
      `Q${p(turn, from.y)} ${p(turn, from.y + sy * r)}` +
      `V${r2(to.y - sy * r)}` +
      `Q${p(turn, to.y)} ${p(turn + r, to.y)}` +
      `H${r2(to.x)}`
    );
  }
  // A loop: right, down to between the rows, left past the start, down, right.
  const out = from.x + lead;
  const back = to.x - lead;
  const mid = between ?? from.y + dy / 2;
  const r = Math.max(
    0,
    Math.min(
      radius,
      Math.abs(mid - from.y) / 2,
      Math.abs(to.y - mid) / 2,
      (out - back) / 2 || radius,
    ),
  );
  return (
    `M${p(from.x, from.y)}H${r2(out - r)}` +
    `Q${p(out, from.y)} ${p(out, from.y + sy * r)}` +
    `V${r2(mid - sy * r)}` +
    `Q${p(out, mid)} ${p(out - r, mid)}` +
    `H${r2(back + r)}` +
    `Q${p(back, mid)} ${p(back, mid + sy * r)}` +
    `V${r2(to.y - sy * r)}` +
    `Q${p(back, to.y)} ${p(back + r, to.y)}` +
    `H${r2(to.x)}`
  );
}

/** A small open arrowhead pointing right, its tip at (x, y). */
export function arrowHead(x: number, y: number, size = 3.5): string {
  return `M${r2(x - size)} ${r2(y - size)}L${r2(x)} ${r2(y)}L${r2(x - size)} ${r2(y + size)}`;
}

/** A diamond (a square on its corner) centred on (cx, cy), `size` corner to corner, its corners
 * rounded by `radius` px, as an SVG path. */
export function diamondPath(cx: number, cy: number, size: number, radius = 2): string {
  const h = size / 2;
  // Each corner is cut back along both edges by k and joined with a quadratic through the corner.
  const k = Math.min(radius * Math.SQRT2, h * 0.45) / Math.SQRT2;
  const corners: [number, number][] = [
    [cx, cy - h],
    [cx + h, cy],
    [cx, cy + h],
    [cx - h, cy],
  ];
  let d = "";
  corners.forEach(([x, y], i) => {
    const [px, py] = corners[(i + 3) % 4];
    const [nx, ny] = corners[(i + 1) % 4];
    const len = Math.hypot(px - x, py - y);
    const ax = x + ((px - x) / len) * k * Math.SQRT2,
      ay = y + ((py - y) / len) * k * Math.SQRT2;
    const bx = x + ((nx - x) / len) * k * Math.SQRT2,
      by = y + ((ny - y) / len) * k * Math.SQRT2;
    d += `${i === 0 ? "M" : "L"}${r2(ax)} ${r2(ay)}Q${r2(x)} ${r2(y)} ${r2(bx)} ${r2(by)}`;
  });
  return d + "Z";
}

/** Where along a bar (a share, 0–1) its planned end falls, when the bar runs past it; else null. */
export function overrunShare(task: Pick<ParsedTask, "start" | "end" | "plannedEnd" | "milestone">) {
  if (task.milestone || task.plannedEnd === null) return null;
  if (!(task.end > task.plannedEnd) || task.end <= task.start) return null;
  return clamp((task.plannedEnd - task.start) / (task.end - task.start), 0, 1);
}

/** How late a task runs against its plan, ms (negative when early, NaN with no plan). */
export function slip(task: Pick<ParsedTask, "end" | "plannedEnd" | "milestone" | "start">) {
  if (task.plannedEnd === null) return NaN;
  return (task.milestone ? task.start : task.end) - task.plannedEnd;
}

/** A duration in words: minutes and hours under a day, days beyond ("45 min", "1 h 05 min",
 * "12 days", "1 day"). */
export function durationText(ms: number): string {
  if (!finite(ms) || ms < 0) return "";
  if (ms < HOUR) return `${Math.round(ms / MINUTE)} min`;
  if (ms < DAY) {
    const h = Math.floor(ms / HOUR),
      m = Math.round((ms - h * HOUR) / MINUTE);
    return m ? `${h} h ${String(m).padStart(2, "0")} min` : `${h} h`;
  }
  const days = Math.round(ms / DAY);
  return `${days} ${days === 1 ? "day" : "days"}`;
}

/** True when a time falls on midnight in the time zone (a date without a time of day). */
export function isMidnight(ms: number, timeZone = "UTC") {
  const z = zoned(ms, timeZone);
  return z.hour === 0 && z.minute === 0;
}

/**
 * A task's dates as people write them. Whole days read inclusively (an exclusive end of Oct 17
 * reads "Oct 12 – Oct 16"); times of day read as times ("14:02 – 14:51"). A milestone is its date.
 * The year shows when the dates are not in `year`.
 */
export function dateRangeText(
  start: number,
  end: number,
  { timeZone = "UTC", year }: { timeZone?: string; year?: number } = {},
): string {
  const days = isMidnight(start, timeZone) && isMidnight(end, timeZone);
  const showYear =
    year !== undefined &&
    (zoned(start, timeZone).year !== year || zoned(end, timeZone).year !== year);
  const date = new Intl.DateTimeFormat("en-US", {
    month: "short",
    day: "numeric",
    year: showYear ? "numeric" : undefined,
    timeZone,
  });
  const time = new Intl.DateTimeFormat("en-US", {
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
    timeZone,
  });
  if (end <= start)
    return days ? date.format(start) : `${date.format(start)}, ${time.format(start)}`;
  if (days) {
    const last = end - DAY;
    return last <= start ? date.format(start) : `${date.format(start)} – ${date.format(last)}`;
  }
  const sameDay = zoned(start, timeZone).day === zoned(end, timeZone).day && end - start < DAY;
  return sameDay
    ? `${time.format(start)} – ${time.format(end)}`
    : `${date.format(start)}, ${time.format(start)} – ${date.format(end)}, ${time.format(end)}`;
}

/** An exclusive end as people write it: the last whole day for dates ("Oct 16" for an end of
 * Oct 17 at midnight), the time of day otherwise. */
export function endText(end: number, { timeZone = "UTC" }: { timeZone?: string } = {}): string {
  const date = new Intl.DateTimeFormat("en-US", { month: "short", day: "numeric", timeZone });
  if (isMidnight(end, timeZone)) return date.format(end - DAY);
  const time = new Intl.DateTimeFormat("en-US", {
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
    timeZone,
  });
  return `${date.format(end)}, ${time.format(end)}`;
}

/** The fit of a name inside a bar: the label shows inside when it fits with padding. */
export function fitsInBar(name: string, length: number, padding = 10) {
  return textWidth(name, TYPE.caption) + padding * 2 <= length;
}
