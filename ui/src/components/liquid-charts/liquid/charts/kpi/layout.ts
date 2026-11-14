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
 * The KPI family's layout maths: levels against a capacity, overflow, thresholds and bands,
 * targets, deltas, meter segments and capacity rows. Pure functions (no React, no DOM), so every
 * reading the components draw is unit-tested.
 */

import { formatValue as coreFormatValue } from "@/components/liquid-charts/liquid/core/format";
import { clampValue as clamp } from "@/components/liquid-charts/liquid/drag/drag-handle";
import type { FigureKind } from "@/components/liquid-charts/liquid/core/figure";
import { finite } from "@/components/liquid-charts/liquid/core/geometry";

export type KpiFormat = { kind: FigureKind; decimals?: number; signed?: boolean };
export type KpiDensity = "tile" | "standard" | "hero";
/** Which way is good for this number: revenue goes up, churn goes down, storage just grows. */
export type GoodDirection = "up" | "down" | "none";

/** A reading that changes at a share of the capacity: at 0.9, a 90% full disk turns `key`. */
export type Threshold = { at: number; key: string; label?: string };
/** A range of values with its own colour and name: an SLA's breach zone, a plan's overspend. */
export type Band = { from: number; to: number; key: string; label?: string };

export type Level = {
  /** (value − min) / (max − min), unclamped: 1.12 is 12% over. */
  ratio: number;
  /** How full the container is, 0–1. */
  level: number;
  over: boolean;
  /** Value above the capacity, in value units (0 when within). */
  excess: number;
  /** Excess as a share of the capacity span; Infinity when the span is zero. */
  excessRatio: number;
};

/**
 * A value as a level in a container from `min` to `max`. A missing or non-finite value reads as
 * empty; a negative one is empty too (the ratio stays negative so callers can say so). A zero
 * capacity is full as soon as anything is in it, and everything in it is excess.
 */
export function levelOf(value: number, max: number, min = 0): Level {
  if (!finite(value) || !finite(max) || !finite(min))
    return { ratio: 0, level: 0, over: false, excess: 0, excessRatio: 0 };
  const span = max - min;
  const excess = Math.max(0, value - max);
  const over = value > max;
  if (span <= 0) {
    const ratio = value > min ? 1 : 0;
    return { ratio, level: ratio, over, excess, excessRatio: excess > 0 ? Infinity : 0 };
  }
  const ratio = (value - min) / span;
  return { ratio, level: clamp(ratio, 0, 1), over, excess, excessRatio: excess / span };
}

/** The threshold in force at `ratio`: the highest one it has reached, or null. Order-free. */
export function thresholdFor(
  ratio: number,
  thresholds: readonly Threshold[] = [],
): Threshold | null {
  if (!Number.isFinite(ratio)) return null;
  let hit: Threshold | null = null;
  for (const threshold of thresholds)
    if (ratio >= threshold.at && (!hit || threshold.at > hit.at)) hit = threshold;
  return hit;
}

/** The band a value falls in: [from, to), with the topmost band also holding its own `to`. */
export function bandFor(value: number, bands: readonly Band[] = []): Band | null {
  if (!Number.isFinite(value) || bands.length === 0) return null;
  let top = bands[0];
  for (const band of bands) if (band.to > top.to) top = band;
  for (const band of bands)
    if (value >= band.from && (value < band.to || (band === top && value === band.to))) return band;
  return null;
}

export type LevelScale = {
  /** The track's top value: the capacity, or past it when the value is over. */
  top: number;
  /** How far the liquid fills the capsule, 0–1. */
  fill: number;
  /** Where the capacity sits along the capsule, 0–1 (1, its end, within capacity). */
  capacity: number;
  over: boolean;
};

/**
 * The scale a level is drawn on. Within capacity the capsule is the capacity and the liquid fills
 * to the level. Over it, the capsule stays one piece: the scale runs on to the value (plus
 * `headroom`, a share of the value's span, default none), the liquid fills the whole way, and
 * `capacity` says where the limit falls: a flat hairline there, and the etched pattern from there
 * to the end (style rule 10). A missing value is empty.
 */
export function levelScale(value: number, max: number, min = 0, headroom = 0): LevelScale {
  const level = levelOf(value, max, min);
  if (!level.over) return { top: max, fill: level.level, capacity: 1, over: false };
  const span = (value - min) * (1 + Math.max(0, headroom));
  return {
    top: min + span,
    fill: (value - min) / span,
    capacity: clamp((max - min) / span, 0, 1),
    over: true,
  };
}

/** Where a value sits along a track of `length` px, clamped to it; null when the value is
 * missing or the range is empty. */
export function positionOf(
  value: number | null | undefined,
  min: number,
  max: number,
  length: number,
): number | null {
  if (!finite(value) || !(max > min) || !(length >= 0)) return null;
  return clamp(((value - min) / (max - min)) * length, 0, length);
}

export type Delta = {
  /** current − comparison. */
  change: number;
  /** change / |comparison|; null when the comparison is zero. */
  relative: number | null;
  direction: "up" | "down" | "flat";
  /** Good when it moved the way that is good; neutral when it did not move. */
  tone: "good" | "bad" | "neutral";
};

/** The change from a comparison to the current value, and whether it is good news. Null when
 * either is missing. */
export function deltaOf(
  current: number,
  comparison: number | null | undefined,
  good: GoodDirection = "up",
  epsilon = 1e-9,
): Delta | null {
  if (!finite(current) || !finite(comparison)) return null;
  const change = current - comparison;
  const direction = Math.abs(change) <= epsilon ? "flat" : change > 0 ? "up" : "down";
  const relative = comparison === 0 ? null : change / Math.abs(comparison);
  const tone =
    direction === "flat" || good === "none" ? "neutral" : direction === good ? "good" : "bad";
  return { change: direction === "flat" ? 0 : change, relative, direction, tone };
}

export type Segment = {
  index: number;
  /** The segment's value range. */
  from: number;
  to: number;
  /** How much of it the value fills, 0–1. */
  fill: number;
  /** Lit in a segmented display: the value fills at least half of it. */
  on: boolean;
  /** The band its midpoint falls in. */
  band: Band | null;
};

/** A meter's `count` equal segments from `min` to `max`, each with its fill and band. */
export function meterSegments(
  value: number,
  {
    min = 0,
    max,
    count,
    bands,
  }: { min?: number; max: number; count: number; bands?: readonly Band[] },
): Segment[] {
  const n = Math.max(1, Math.floor(Number.isFinite(count) ? count : 1));
  const span = max - min;
  const step = span > 0 ? span / n : 0;
  return Array.from({ length: n }, (_, index) => {
    const from = min + step * index;
    const to = index === n - 1 ? max : from + step;
    let fill = 0;
    if (Number.isFinite(value))
      fill = step > 0 ? clamp((value - from) / step, 0, 1) : value > min ? 1 : 0;
    return { index, from, to, fill, on: fill >= 0.5, band: bandFor((from + to) / 2, bands) };
  });
}

/** `count` equal slots along `length` px with `gap` px between them. The gap gives way in a
 * short track so each slot keeps at least 60% of its share. */
export function segmentSlots(
  length: number,
  count: number,
  gap: number,
): { start: number; size: number }[] {
  const n = Math.max(1, Math.floor(count));
  const room = Math.max(0, length);
  const space = n > 1 ? Math.min(gap, ((room / n) * 0.4 * n) / (n - 1)) : 0;
  const size = (room - space * (n - 1)) / n;
  return Array.from({ length: n }, (_, i) => ({ start: i * (size + space), size }));
}

/** Where a straight-line pace puts a goal after `elapsed` of `period`: the plan to date. */
export function paceTo(elapsed: number, period: number, goal: number): number {
  if (!(period > 0)) return goal;
  return goal * clamp(elapsed / period, 0, 1);
}

export type CapacityRow = {
  /** The row's top (its readout line), px. */
  top: number;
  /** The capsule's top and centre, px. */
  capsule: number;
  centre: number;
  bottom: number;
};

/** Stacked capacity rows: a readout line above each capsule, `gap` px between rows. */
export function rowsLayout(
  count: number,
  {
    thickness,
    readout,
    gap,
    inner = 8,
  }: {
    thickness: number;
    readout: number;
    gap: number;
    inner?: number;
  },
): { rows: CapacityRow[]; height: number } {
  const rows: CapacityRow[] = [];
  let y = 0;
  for (let i = 0; i < Math.max(0, count); i++) {
    const capsule = y + readout + inner;
    rows.push({ top: y, capsule, centre: capsule + thickness / 2, bottom: capsule + thickness });
    y = capsule + thickness + gap;
  }
  return { rows, height: rows.length ? rows[rows.length - 1].bottom : 0 };
}

/** The row under `y`: each row owns half the gap on either side; null outside them all. */
export function rowAt(y: number, rows: readonly CapacityRow[]): number | null {
  if (!rows.length || !Number.isFinite(y)) return null;
  for (let i = 0; i < rows.length; i++) {
    const before = i === 0 ? rows[0].top : (rows[i - 1].bottom + rows[i].top) / 2;
    const after = i === rows.length - 1 ? rows[i].bottom : (rows[i].bottom + rows[i + 1].top) / 2;
    if (y >= before && y <= after) return i;
  }
  return null;
}

/** Sizes per density: capsule thickness, gap between rows, readout line height. */
export const DENSITY: Record<
  KpiDensity,
  { thickness: number; gap: number; readout: number; figure: "figureS" | "figureM" | "figureL" }
> = {
  tile: { thickness: 12, gap: 14, readout: 18, figure: "figureS" },
  standard: { thickness: 22, gap: 20, readout: 22, figure: "figureM" },
  hero: { thickness: 40, gap: 24, readout: 30, figure: "figureL" },
};

/** A value as plain text in the same format a `Figure` draws (the core's `formatValue`, the same
 * Intl options), with a true minus and a dash for a missing value. For aria labels, live
 * announcements and data tables, where a component won't do. */
export function formatValue(value: number, format: KpiFormat): string {
  if (!Number.isFinite(value)) return "—";
  return coreFormatValue(value, format).replace(/^-/, "−");
}

/** A one-line reading of a level, for screen readers: "$1.84M of $2.2M, 84%, near capacity". */
export function levelText(
  value: number,
  max: number,
  format: KpiFormat,
  {
    min = 0,
    target,
    targetLabel = "target",
    status,
  }: {
    min?: number;
    target?: number | null;
    targetLabel?: string;
    status?: string | null;
  } = {},
): string {
  const level = levelOf(value, max, min);
  const parts = [`${formatValue(value, format)} of ${formatValue(max, format)}`];
  if (max > min) parts.push(formatValue(level.ratio, { kind: "percent" }));
  if (finite(target)) parts.push(`${targetLabel} ${formatValue(target, format)}`);
  if (level.over) parts.push(`over by ${formatValue(level.excess, format)}`);
  if (status) parts.push(status.toLowerCase());
  return parts.join(", ");
}

/**
 * Where a row list's tooltip goes, never covering the focused row's readout or capsule:
 * - below the row, centred on its liquid end, when it fits inside the plot (plus `sink` px of
 *   room under it);
 * - else above the row, where it may rise `rise` px past the plot's top (over the card body,
 *   never the card's header);
 * - else beside the liquid end (to its left when there is room, so the right-aligned figures stay
 *   clear), level with the capsule's top, clamped into the room there is.
 */
export function rowTooltipPlacement(
  tip: { width: number; height: number },
  row: { end: number; top: number; capsule: number; bottom: number },
  plot: { width: number; height: number },
  { gap = 10, rise = 0, sink = 0 }: { gap?: number; rise?: number; sink?: number } = {},
): { x: number; y: number; side: "below" | "above" | "beside" } {
  const centred = clamp(row.end - tip.width / 2, 0, Math.max(0, plot.width - tip.width));
  const below = row.bottom + gap;
  if (below + tip.height <= plot.height + sink) return { x: centred, y: below, side: "below" };
  const above = row.top - gap - tip.height;
  if (above >= -rise) return { x: centred, y: above, side: "above" };
  const left = row.end - gap - tip.width;
  const x = clamp(left >= 0 ? left : row.end + gap, 0, Math.max(0, plot.width - tip.width));
  const y = clamp(row.capsule - 4, -rise, Math.max(-rise, plot.height + sink - tip.height));
  return { x, y, side: "beside" };
}

/**
 * Which gap of a segmented meter a reference falls in: the boundary between segments `index - 1`
 * and `index` nearest the value (1 to count − 1). References at or past either end snap to the
 * first or last gap; a missing value or an empty range has none.
 */
export function referenceGap(
  value: number | null | undefined,
  { min = 0, max, count }: { min?: number; max: number; count: number },
): number | null {
  const n = Math.floor(count);
  if (!finite(value) || !(max > min) || n < 2) return null;
  return clamp(Math.round(((value - min) / (max - min)) * n), 1, n - 1);
}

/** A point on a level's scale as a share of the capsule's length, for an engraved mark; null when
 * the point is missing. */
export function markShare(value: number | null | undefined, min: number, top: number) {
  if (!finite(value) || !(top > min)) return null;
  return clamp((value - min) / (top - min), 0, 1);
}
