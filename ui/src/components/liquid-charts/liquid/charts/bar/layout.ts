/*
Liquid Charts by Ojieame · https://liquid.ojieame.design · nathan@ojieame.design

Liquid Charts adaptation. Original foundation: Evil Charts by Gurbinder.
Upstream: ecbd6a5db7b25f9a7e070c02b2ef728c378eeb5c
MIT License

Copyright (c) 2026 Gurbinder
Copyright (c) 2026 NATEPAY, LLC (Liquid Charts additions)

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
 * The bar family's layout maths: pure functions, no React and no DOM, so every decision the chart
 * makes (stacking, sorting, domains, geometry, insets, which labels show) can be unit-tested.
 *
 * Values are read into a `[series][category]` grid. `barModel` turns the grid into segments in
 * value units (stacked or side by side, sorted or not); `barGeometry` turns segments into bars in
 * plot px for one size. Charts call both every frame, which is cheap.
 */

import {
  band,
  grouped,
  linear,
  niceDomain,
  ticks,
  type Band,
  type Domain,
  type Linear,
} from "@/components/liquid-charts/liquid/core/scale";
import { labelPriorities, namedVisibility, type LabelMode } from "@/components/liquid-charts/liquid/core/labels";
import type { Insets, Rect } from "@/components/liquid-charts/liquid/core/geometry";
import { clamp, finite } from "@/components/liquid-charts/liquid/core/geometry";
import type { FigureKind } from "@/components/liquid-charts/liquid/core/figure";
import { numberFormat } from "@/components/liquid-charts/liquid/core/format";

export type BarLayout = "vertical" | "horizontal";
export type BarStack = "none" | "stacked" | "percent";
export type BarSort = "none" | "ascending" | "descending";
export type BarDensity = "tile" | "standard" | "hero";
export type BarDirection = "up" | "down" | "left" | "right";
export type BarHighlight = "max" | "min" | number;
export type BarFormat = {
  kind: FigureKind;
  decimals?: number;
  signed?: boolean;
  currency?: string;
};
export type BarRow = Record<string, unknown>;

/** A number from a data cell; anything else (missing, null, text) is NaN, which draws nothing. */
export const cell = (value: unknown): number =>
  typeof value === "number" ? value : typeof value === "string" && value.trim() ? +value : NaN;

/** The `[series][category]` grid for `keys` over `rows`. */
export function seriesValues(rows: readonly BarRow[], keys: readonly string[]): number[][] {
  return keys.map((key) => rows.map((row) => cell(row[key])));
}

/* ——— Model: segments in value units ——— */

/**
 * How a bar meets its target, by style rule 9 (reference marks live in the glass): short of the
 * target, the bar is a container as long as the target, filled to the value (`level`); at or past
 * it, the bar runs to the value with the target engraved (`marks`). A missing or zero target, or one
 * on the other side of zero, leaves the bar as it is.
 */
export function targetFit(
  value: number,
  target: number,
): { extent: number; level?: number; marks?: number[] } {
  if (!finite(value) || !finite(target) || target === 0 || value * target < 0)
    return { extent: value };
  if (Math.abs(value) < Math.abs(target)) return { extent: target, level: value / target };
  return { extent: value, marks: [target / value] };
}

export type BarSegment = {
  /** Stable across sorts and data changes, so a bar springs rather than jumps. */
  key: string;
  /** The row this segment belongs to. */
  category: number;
  /** Its display position after sorting. */
  slot: number;
  /** Index into the series; -1 for the comparison series. */
  series: number;
  /** Side-by-side position inside the slot. */
  group: number;
  /** The data value. */
  raw: number;
  /** The drawn value: the raw value, or its share of the category in a 100% stack. */
  value: number;
  /** Where the segment starts and ends along the value axis. */
  start: number;
  end: number;
  /** True for the segment that sits on zero (no stack gap before it). */
  first: boolean;
  /** Where the glass ends, in value units: the value, or the target when the value falls short
   * (the bar is then a container as long as the target). */
  extent: number;
  /** How full the container is (value / target), when the value falls short of its target. */
  level?: number;
  /** Engraved target, as a share of the bar's length, when the value reaches or passes it. */
  marks?: number[];
};

export type BarModel = {
  /** Display position → row index. */
  order: number[];
  segments: BarSegment[];
  /** Bars side by side in each slot. */
  groups: number;
  domain: [number, number];
  /** By row: the net of the series (sum when stacked, or the sum across series otherwise). */
  totals: number[];
  /** By row: the sum of the magnitudes, the whole a 100% stack divides by. */
  wholes: number[];
  /** True when any drawn value is below zero. */
  negative: boolean;
};

export type BarModelInput = {
  /** `[series][category]`; non-finite values are missing. */
  values: readonly (readonly number[])[];
  /** A comparison series drawn as a graphite ghost beside the live bars. */
  compare?: readonly number[] | null;
  stack?: BarStack;
  sort?: BarSort;
  /** Targets by row, which the domain includes. Side-by-side bars meet theirs in the glass (see
   * `targetFit`); stacks only make room for them. */
  targets?: readonly number[];
  /** A fixed value domain; otherwise a nice one is found. */
  domain?: Domain;
  /** The tick count the nice domain is rounded for (default 10, which keeps bars tall). */
  domainCount?: number;
};

export function barModel({
  values,
  compare = null,
  stack = "none",
  sort = "none",
  targets = [],
  domain,
  domainCount = 10,
}: BarModelInput): BarModel {
  const count = Math.max(0, ...values.map((row) => row.length), compare?.length ?? 0);
  const totals: number[] = [];
  const wholes: number[] = [];
  for (let c = 0; c < count; c++) {
    let net = 0,
      whole = 0;
    for (const row of values) {
      const v = row[c];
      if (!finite(v)) continue;
      net += v;
      whole += Math.abs(v);
    }
    totals.push(net);
    wholes.push(whole);
  }

  const order = Array.from({ length: count }, (_, i) => i);
  if (sort !== "none") {
    const sign = sort === "descending" ? -1 : 1;
    // Array sort is stable, so ties keep their data order.
    order.sort((a, b) => sign * (totals[a] - totals[b]));
  }
  const slotOf = new Array<number>(count);
  order.forEach((category, slot) => (slotOf[category] = slot));

  const stacked = stack !== "none";
  const live = stacked ? 1 : Math.max(1, values.length);
  const groups = live + (compare ? 1 : 0);
  const offset = compare ? 1 : 0;
  const segments: BarSegment[] = [];
  let negative = false;

  for (let c = 0; c < count; c++) {
    const slot = slotOf[c];
    if (compare && finite(compare[c])) {
      const v = compare[c];
      if (v < 0) negative = true;
      segments.push({
        key: `compare-${c}`,
        category: c,
        slot,
        series: -1,
        group: 0,
        raw: v,
        value: v,
        start: 0,
        end: v,
        first: true,
        extent: v,
      });
    }
    let up = 0,
      down = 0;
    values.forEach((row, s) => {
      const raw = row[c];
      if (!finite(raw)) return;
      const value = stack === "percent" ? (wholes[c] ? raw / wholes[c] : 0) : raw;
      if (value < 0) negative = true;
      if (!stacked) {
        segments.push({
          key: `${s}-${c}`,
          category: c,
          slot,
          series: s,
          group: offset + s,
          raw,
          value,
          start: 0,
          end: value,
          first: true,
          ...targetFit(value, targets[c] ?? NaN),
        });
        return;
      }
      // A zero segment in a stack has no height; drawing it would leave a sliver.
      if (value === 0) return;
      const from = value > 0 ? up : down;
      const to = from + value;
      if (value > 0) up = to;
      else down = to;
      segments.push({
        key: `${s}-${c}`,
        category: c,
        slot,
        series: s,
        group: offset,
        raw,
        value,
        start: from,
        end: to,
        first: from === 0,
        extent: to,
      });
    });
  }

  let resolved: [number, number];
  if (domain) resolved = [domain[0], domain[1]];
  else if (stack === "percent" && !negative) resolved = [0, 1];
  else {
    const extents = segments.flatMap((segment) => [segment.start, segment.end]);
    resolved = niceDomain([...extents, ...targets.filter(finite)], { count: domainCount });
  }
  return { order, segments, groups, domain: resolved, totals, wholes, negative };
}

/* ——— Geometry: bars in plot px ——— */

export type BarGeometryOptions = {
  width: number;
  height: number;
  insets: Insets;
  layout?: BarLayout;
  /** The thickest a single bar may be, px (default 34 for columns, 26 for rows). */
  maxThickness?: number;
  /** Px between side-by-side bars (default 6). */
  gap?: number;
  /** Px between stacked segments (default 2). */
  stackGap?: number;
  /** Share of each slot left between slots (default 0.4 for groups, 0.44 for single bars). */
  paddingInner?: number;
};

export type BarRect = {
  key: string;
  segment: BarSegment;
  /** Centre across the bar, the px it grows from, and its length and thickness. */
  cross: number;
  base: number;
  length: number;
  thickness: number;
  direction: BarDirection;
};

export type BarGeometry = {
  /** Slots along the category axis (x for columns, y for rows). */
  slots: Band;
  /** The value axis (y for columns, x for rows). */
  value: Linear;
  /** The px of zero (or of the domain's nearer end when zero lies outside it). */
  zero: number;
  /** Each bar's thickness, and the width the bars of one slot take together. */
  thickness: number;
  groupWidth: number;
  /** Centre offset of each side-by-side group from its slot's centre. */
  offset: (group: number) => number;
  plot: Rect;
  bars: BarRect[];
};

export function barGeometry(model: BarModel, options: BarGeometryOptions): BarGeometry {
  const { width, height, insets, layout = "vertical", gap = 6, stackGap = 2 } = options;
  const vertical = layout === "vertical";
  const plot: Rect = {
    left: insets.left,
    top: insets.top,
    right: Math.max(insets.left, width - insets.right),
    bottom: Math.max(insets.top, height - insets.bottom),
  };
  const cap = options.maxThickness ?? (vertical ? 34 : 26);
  const paddingInner = options.paddingInner ?? (model.groups > 1 ? 0.4 : 0.44);
  const slots = band(
    model.order.length,
    vertical ? [plot.left, plot.right] : [plot.top, plot.bottom],
    { paddingInner, paddingOuter: 0 },
  );
  const value = linear(model.domain, vertical ? [plot.bottom, plot.top] : [plot.left, plot.right]);
  const [lo, hi] = [Math.min(...model.domain), Math.max(...model.domain)];
  const at = (v: number) => value(clamp(v, lo, hi));
  const groupWidth = Math.max(
    0,
    Math.min(slots.bandwidth, model.groups * cap + (model.groups - 1) * gap),
  );
  const group = grouped(groupWidth, model.groups, gap);

  const bars = model.segments.map((segment): BarRect => {
    const positive = segment.extent >= segment.start;
    const direction: BarDirection = vertical
      ? positive
        ? "up"
        : "down"
      : positive
        ? "right"
        : "left";
    const a = at(segment.start),
      b = at(segment.extent);
    const span = Math.abs(b - a);
    const shift = segment.first ? 0 : Math.min(stackGap, span / 2);
    const growth = direction === "up" || direction === "left" ? -1 : 1;
    return {
      key: segment.key,
      segment,
      cross: slots.centre(segment.slot) + group.offset(segment.group),
      base: a + growth * shift,
      length: Math.max(0, span - shift),
      thickness: group.thickness,
      direction,
    };
  });
  return {
    slots,
    value,
    zero: at(0),
    thickness: group.thickness,
    groupWidth,
    offset: group.offset,
    plot,
    bars,
  };
}

export { barRect } from "@/components/liquid-charts/liquid/core/geometry";

/** The smallest rect holding all of `rects`, or null for none. */
export function unionRect(rects: readonly Rect[]): Rect | null {
  if (!rects.length) return null;
  return rects.reduce((a, b) => ({
    left: Math.min(a.left, b.left),
    top: Math.min(a.top, b.top),
    right: Math.max(a.right, b.right),
    bottom: Math.max(a.bottom, b.bottom),
  }));
}

/**
 * Where a value label anchors beyond a bar: at its tip, or past its target tick when the tick
 * reaches further, so the label never sits on the tick. `reach` is the target's px, if any.
 */
export function tipAnchor(
  rect: Rect,
  direction: BarDirection,
  cross: number,
  reach: number | null = null,
): { x: number; y: number; side: "above" | "below" | "left" | "right" } {
  const far = reach !== null && Number.isFinite(reach);
  switch (direction) {
    case "up":
      return { x: cross, y: far ? Math.min(rect.top, reach - 1) : rect.top, side: "above" };
    case "down":
      return { x: cross, y: far ? Math.max(rect.bottom, reach + 1) : rect.bottom, side: "below" };
    case "right":
      return { x: far ? Math.max(rect.right, reach + 1) : rect.right, y: cross, side: "right" };
    case "left":
      return { x: far ? Math.min(rect.left, reach - 1) : rect.left, y: cross, side: "left" };
  }
}

/** True when a label of `width` × `height` fits inside a segment with a little air. */
export const fitsInside = (rect: Rect, width: number, height: number, air = 4) =>
  rect.right - rect.left >= width + air && rect.bottom - rect.top >= height + air / 2;

export { rowTooltipPosition } from "@/components/liquid-charts/liquid/core/tooltip-position";

/* ——— Projected periods: an etched pattern from where the projection begins ——— */

/**
 * Where a row's projection begins, as a share of its bar from the base: `true` is the whole bar
 * (0), a number is the share (0–1) the actual part fills, `false` or missing (or 1 and beyond) is
 * no projection. Null means an actual, complete row.
 */
export function projectionStart(value: boolean | number | null | undefined): number | null {
  if (value === true) return 0;
  if (typeof value !== "number" || !Number.isFinite(value) || value >= 1) return null;
  return Math.max(0, value);
}

/**
 * The core's `BarMark.pattern` for one segment of a row projected from `from` (a share of the
 * row's bar). `reach` is how far the row's bar runs on this segment's side, in value units: the
 * segment's own end for side-by-side bars, the stack's end for stacks. A segment wholly past the
 * start is patterned whole (`true`), one wholly before it not at all (`undefined`), and the one the
 * start falls in from that share of its own length.
 */
export function segmentPattern(
  segment: Pick<BarSegment, "start" | "end">,
  from: number | null,
  reach: number,
): boolean | number | undefined {
  if (from === null) return undefined;
  const span = segment.end - segment.start;
  if (!span) return undefined;
  const t = (from * reach - segment.start) / span;
  if (t <= 1e-6) return true;
  if (t >= 1 - 1e-6) return undefined;
  return t;
}

/** How far each row's bar runs up (or right) and down (or left), in value units. */
export function rowReach(model: BarModel): { up: number; down: number }[] {
  const reach = model.totals.map(() => ({ up: 0, down: 0 }));
  for (const segment of model.segments) {
    if (segment.series < 0) continue;
    const r = reach[segment.category];
    r.up = Math.max(r.up, segment.end);
    r.down = Math.min(r.down, segment.end);
  }
  return reach;
}

/* ——— Labels: which bars carry a value, and in what order they win room ——— */

export type LabelTarget = {
  id: string;
  /** The row and the display position it labels. */
  category: number;
  slot: number;
  /** The bar whose tip (or centre) the label rides. */
  markKey: string;
  /** The value the label shows. */
  value: number;
  /** Sits inside the segment, at its centre (100% stacks), instead of beyond the tip. */
  centre: boolean;
};

/**
 * One label per row in single-series, stacked (the total, at the stack's far end) and 100% stacks
 * (the lead series' share, inside its segment); one per bar when several series sit side by side.
 * The comparison series is never labelled. Targets come back in display order.
 */
export function labelTargets(model: BarModel, stack: BarStack = "none"): LabelTarget[] {
  const bySlot = [...model.segments].sort((a, b) => a.slot - b.slot || a.series - b.series);
  const liveSeries = new Set(bySlot.filter((s) => s.series >= 0).map((s) => s.series)).size;
  const targets: LabelTarget[] = [];
  if (stack === "none") {
    for (const segment of bySlot) {
      if (segment.series < 0) continue;
      targets.push({
        id: liveSeries > 1 ? segment.key : String(segment.category),
        category: segment.category,
        slot: segment.slot,
        markKey: segment.key,
        value: segment.value,
        centre: false,
      });
    }
    return targets;
  }
  for (let slot = 0; slot < model.order.length; slot++) {
    const category = model.order[slot];
    const own = bySlot.filter((s) => s.category === category && s.series >= 0);
    if (!own.length) continue;
    if (stack === "percent") {
      const lead = own.reduce((a, b) => (b.series < a.series ? b : a));
      targets.push({
        id: String(category),
        category,
        slot,
        markKey: lead.key,
        value: lead.value,
        centre: true,
      });
      continue;
    }
    const total = model.totals[category];
    // The total sits at the far end of the side it leans to.
    const tip =
      total >= 0
        ? own
            .filter((s) => s.end > 0)
            .reduce<BarSegment | null>((a, b) => (!a || b.end > a.end ? b : a), null)
        : own
            .filter((s) => s.end < 0)
            .reduce<BarSegment | null>((a, b) => (!a || b.end < a.end ? b : a), null);
    if (!tip) continue;
    targets.push({
      id: String(category),
      category,
      slot,
      markKey: tip.key,
      value: total,
      centre: false,
    });
  }
  return targets;
}

/** The row to highlight: the largest or smallest total, or a given row; null when there is none. */
export function resolveHighlight(
  totals: readonly number[],
  highlight: BarHighlight | undefined,
  exclude: readonly boolean[] = [],
): number | null {
  if (highlight === undefined) return null;
  if (typeof highlight === "number")
    return Number.isInteger(highlight) && highlight >= 0 && highlight < totals.length
      ? highlight
      : null;
  let best: number | null = null;
  totals.forEach((value, i) => {
    if (!finite(value) || exclude[i]) return;
    if (best === null || (highlight === "max" ? value > totals[best] : value < totals[best]))
      best = i;
  });
  return best;
}

/**
 * Label priorities for `targets` (see `labelPriorities` in the core):
 * - `auto` reads the best, worst and latest *actual* values: excluded rows (projected periods)
 *   never count as best or worst, and the latest is the last row that is not excluded.
 * - A highlighted row carries the label: it outranks the rest, and in `auto` it is the only one
 *   labelled at rest. `none` stays empty.
 * - With a row in focus its labels come first; the rest hide (`hideOnFocus`, the default for
 *   columns, where the tooltip sits among them) or stay where they fit (rows).
 */
export function labelPlan(
  targets: readonly LabelTarget[],
  {
    mode,
    active = null,
    highlight = null,
    exclude = [],
    hideOnFocus = true,
  }: {
    mode: LabelMode;
    active?: number | null;
    highlight?: number | null;
    exclude?: readonly boolean[];
    hideOnFocus?: boolean;
  },
): number[] {
  const values = targets.map((t) => (mode === "auto" && exclude[t.category] ? NaN : t.value));
  const priorities = labelPriorities(values, mode);
  if (mode === "auto" && !priorities.includes(1)) {
    for (let i = values.length - 1; i >= 0; i--)
      if (finite(values[i])) {
        if (priorities[i] === 0) priorities[i] = 1;
        break;
      }
  }
  return targets.map((target, i) => {
    if (active !== null && (hideOnFocus || target.category === active))
      return target.category === active ? Infinity : 0;
    if (highlight !== null && mode !== "none") {
      if (target.category === highlight) return 10;
      return mode === "auto" ? 0 : priorities[i];
    }
    return priorities[i];
  });
}

/* ——— Axes: ticks, insets and which category names fit ——— */

/** About `count` round ticks, plus the domain's low end when it is below zero (the loss floor). */
export function valueTicks(domain: Domain, count = 3): number[] {
  const values = ticks(domain, count);
  if (domain[0] < 0 && !values.includes(domain[0])) values.unshift(domain[0]);
  return [...new Set(values)].sort((a, b) => a - b);
}

/* Advance widths in em for Sofia Sans Semi Condensed, measured in the browser and rounded up, so
 * estimates err towards room. */
const EM: Record<string, number> = {
  ",": 0.25,
  ".": 0.25,
  " ": 0.21,
  $: 0.6,
  "%": 0.9,
  "−": 0.58,
  "-": 0.47,
  "+": 0.58,
  I: 0.25,
  J: 0.26,
  M: 0.74,
  W: 0.82,
  f: 0.36,
  i: 0.23,
  j: 0.23,
  l: 0.23,
  m: 0.8,
  r: 0.35,
  t: 0.38,
  w: 0.7,
};
const emOf = (char: string) =>
  EM[char] ?? (/[0-9]/.test(char) ? 0.56 : /[A-Z]/.test(char) ? 0.6 : 0.52);

/** An estimate of a line of chart text's width, px, at `size` px. */
export function textWidth(text: string, size: number): number {
  let em = 0;
  for (const char of text) em += emOf(char);
  return em * size;
}

/** An estimate of a `Figure`'s width: its currency sign and unit (%, K, M, B) are set at 0.7 em. */
export function figureWidth(text: string, size: number): number {
  let px = 0;
  for (const char of text) px += textWidth(char, /[$%KMB]/.test(char) ? size * 0.7 : size);
  return px;
}

/** Type sizes of the roles the axes use (micro for ticks, label for categories, figure S). */
export const TYPE = { micro: 11.5, label: 13, figure: 14 } as const;
/** Px between a tick label and the plot, as the core's gridlines set it. */
const TICK_GAP = 10;

export type InsetsInput = {
  layout: BarLayout;
  /** Formatted value ticks (columns) and category names. */
  ticks: readonly string[];
  categories: readonly string[];
  labels: LabelMode;
  /** The widest value label, formatted, for rows (they sit beyond the bar's end). */
  valueLabel?: string;
  negative?: boolean;
  density?: BarDensity;
  /** Reserve room for a single target's label beside the plot. */
  targetLabel?: string | null;
  /** Value ticks under rows. */
  valueAxis?: boolean;
  /** Labels sit inside the bars (100% stacks), so no room is kept beyond them. */
  inside?: boolean;
};

const widest = (texts: readonly string[], size: number) =>
  texts.reduce((w, text) => Math.max(w, textWidth(text, size)), 0);

/**
 * Insets that fit the axes: columns reserve the widest tick on the left, a line of category names
 * below, room above the tallest bar for its (lifted) label, and room under the lowest loss for its
 * label; rows reserve the widest category name on the left and the widest value label on the right.
 */
export function barInsets({
  layout,
  ticks: tickLabels,
  categories,
  labels,
  valueLabel = "",
  negative = false,
  density = "standard",
  targetLabel = null,
  valueAxis = false,
  inside = false,
}: InsetsInput): Insets {
  const tile = density === "tile";
  const labelled = labels !== "none" && !inside;
  // A value label's line box is 24px; lifted on focus it grows by 1.22, and sits 6px off the bar.
  const lifted = Math.ceil(24 * 1.22);
  if (layout === "vertical") {
    const tickWidth = Math.ceil(widest(tickLabels, TYPE.micro));
    const target = targetLabel ? Math.ceil(figureWidth(targetLabel, TYPE.figure)) + 14 : 0;
    const names = tile ? 24 : 30;
    // Loss labels hang under the lowest bar and must clear the names (the chart keeps labels 22px,
    // or 18px in a tile, above the frame's bottom).
    const lossRoom = 6 + lifted + (tile ? 18 : 22) - names;
    return {
      left: Math.max(tile ? 20 : 28, tickWidth + TICK_GAP + 4),
      right: Math.max(tile ? 6 : 12, target),
      top: labelled ? lifted + 6 : tile ? 8 : 12,
      bottom: names + (negative && labelled ? lossRoom : 0),
    };
  }
  const name = Math.ceil(widest(categories, TYPE.label));
  const value = labelled ? Math.ceil(figureWidth(valueLabel, TYPE.figure) * 1.22) + 14 : 0;
  return {
    left: name + 16 + (negative ? value : 0),
    right: Math.max(12, value + 6),
    top: tile ? 4 : 8,
    bottom: (tile ? 4 : 8) + (valueAxis ? 22 : 0) + (targetLabel ? 22 : 0),
  };
}

/**
 * Which category names to show when they would collide: every `step`-th name, the smallest step at
 * which each shown name clears its shown neighbours with a little air. `nameWidth` is every name's
 * width (or one width for all), so a long name thins its row only where it really collides. The
 * focused one always shows, and shown names too close to it step aside while it does.
 */
export function categoryLabelVisibility(
  count: number,
  slotStep: number,
  nameWidth: number | readonly number[],
  active: number | null = null,
  air = 10,
): boolean[] {
  const width = (i: number) => (typeof nameWidth === "number" ? nameWidth : (nameWidth[i] ?? 0));
  const clear = (a: number, b: number) =>
    Math.abs(b - a) * slotStep >= (width(a) + width(b)) / 2 + air;
  // Names the host left empty were thinned on purpose: show the named ones that have room.
  if (Array.isArray(nameWidth) && nameWidth.some((w) => w === 0) && nameWidth.some((w) => w > 0))
    return namedVisibility(
      Array.from({ length: count }, (_, i) => (width(i) > 0 ? "x" : "")),
      clear,
    );
  let step = 1;
  if (slotStep > 0)
    while (
      step < count &&
      !Array.from({ length: count - step }, (_, i) => i).every(
        (i) => i % step !== 0 || clear(i, i + step),
      )
    )
      step++;
  const visible = Array.from({ length: count }, (_, i) => i % step === 0);
  if (active !== null && active >= 0 && active < count && !visible[active]) {
    for (let i = 0; i < count; i++) if (visible[i] && !clear(i, active)) visible[i] = false;
    visible[active] = true;
  }
  return visible;
}

/* ——— Formats ——— */

const intl = (format: BarFormat, compact: boolean) => {
  const decimals = format.decimals ?? 0;
  const kind = format.kind;
  if (kind === "percent")
    return numberFormat({
      style: "percent",
      minimumFractionDigits: compact ? 0 : decimals,
      maximumFractionDigits: decimals,
    });
  const currency = kind === "currency" || kind === "currencyCompact";
  return numberFormat({
    ...(currency ? { style: "currency", currency: "USD" } : {}),
    ...(compact || kind === "compact" || kind === "currencyCompact"
      ? // WebKit pads compact fractions ("$6.0K") unless the minimum is pinned.
        {
          notation: "compact",
          minimumFractionDigits: 0,
          maximumFractionDigits: compact ? 1 : decimals,
        }
      : { minimumFractionDigits: decimals, maximumFractionDigits: decimals }),
  });
};

/** A value as plain text for screen readers and the data table ("-$6.2K", "+12.5%"). */
export function formatValue(value: number, format: BarFormat): string {
  if (!finite(value)) return "no data";
  const text = intl(format, false).format(Math.abs(value));
  const sign = value < 0 ? "-" : format.signed && value > 0 ? "+" : "";
  return sign + text;
}

/** Preserve the fractional places of the actual axis values, including a loss floor beside
 * integer ticks. Ignore insignificant binary tails, but retain scientific-notation exponents.
 */
function tickPrecision(values: readonly number[], percent: boolean): number {
  const places = values.filter(finite).reduce((most, value) => {
    const [coefficient, exponent = "0"] = (value * (percent ? 100 : 1)).toPrecision(15).split("e");
    const fraction = (coefficient.split(".")[1] ?? "").replace(/0+$/, "").length;
    return Math.max(most, fraction - Number(exponent));
  }, 0);
  return Math.min(20, places);
}

/**
 * Tick labels for a value axis: compact where the numbers are large ("$40K", "12K"), whole
 * percents, and a true minus. Zero keeps its currency sign.
 */
export function tickFormatter(
  format: BarFormat,
  domain: Domain,
  values: readonly number[] = valueTicks(domain),
): (value: number) => string {
  const peak = Math.max(Math.abs(domain[0]), Math.abs(domain[1]));
  const kind = format.kind;
  const compact =
    kind === "compact" ||
    kind === "currencyCompact" ||
    (kind === "currency" && peak >= 1000) ||
    (kind === "number" && peak >= 10000);
  const currency = kind === "currency" || kind === "currencyCompact";
  const options: Intl.NumberFormatOptions =
    kind === "percent"
      ? { style: "percent" }
      : {
          ...(currency ? { style: "currency", currency: format.currency ?? "USD" } : {}),
          ...(compact ? { notation: "compact" } : {}),
        };
  const ticks = [...new Set(values.filter(finite))];
  // Integer input can still need fractional ticks (all-zero bars use a 0–1 axis).
  // Increase only axis precision, preserving distinct currency/percent/compact labels too.
  let decimals = Math.max(compact ? 1 : 0, tickPrecision(ticks, kind === "percent"));
  let number = numberFormat({
    ...options,
    minimumFractionDigits: 0,
    maximumFractionDigits: decimals,
  });
  const label = (value: number) => `${value < 0 ? "−" : ""}${number.format(Math.abs(value))}`;
  while (decimals < 20 && new Set(ticks.map(label)).size < ticks.length) {
    number = numberFormat({
      ...options,
      minimumFractionDigits: 0,
      maximumFractionDigits: ++decimals,
    });
  }
  return label;
}

/* ——— Defaults by density ——— */

export const BAR_FALLBACK = [0.57, 0.57, 0.57] as const; // bar-chart.tsx's FALLBACK
export const DENSITY = {
  tile: { height: 168, ticks: 2, column: 20, row: 16 },
  standard: { height: 280, ticks: 3, column: 34, row: 26 },
  hero: { height: 360, ticks: 4, column: 48, row: 32 },
} as const satisfies Record<
  BarDensity,
  { height: number; ticks: number; column: number; row: number }
>;

/** Entrance stagger per slot, s: brisk for a few bars, and the whole run under about 0.6 s. */
export const stagger = (count: number) => Math.min(0.07, 0.6 / Math.max(1, count));
