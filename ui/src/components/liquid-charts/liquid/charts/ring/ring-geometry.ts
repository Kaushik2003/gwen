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
 * Ring geometry: the pure maths behind donuts, pies, semi donuts, progress rings and gauges.
 *
 * Angles are clockwise radians from 12 o'clock, the convention the glass renderer uses, so a point
 * at angle a and radius r sits at (cx + r·sin a, cy − r·cos a). Everything here is plain functions
 * of numbers, safe to call every animation frame and on the server, and unit-tested.
 */

import {
  angleOf,
  normAngle,
  polar,
  sectorPath,
  TAU,
  trimmed,
  wrapAngle,
  type Point,
} from "@/components/liquid-charts/liquid/core/arc";
import { framePinned } from "@/components/liquid-charts/liquid/rings/pin-labels";
import { clamp, settle } from "@/components/liquid-charts/liquid/core/geometry";

// The circle primitives, leader labels and the outward tooltip now live in the core; they are
// re-exported here so existing imports of this module keep working.
export { angleOf, normAngle, polar, sectorPath, TAU, trimmed, wrapAngle, type Point };
export {
  layoutOutsideLabels,
  leadersCross,
  segmentsCross,
  underRect,
  type OutsideLabel,
  type OutsideLabelRequest,
} from "@/components/liquid-charts/liquid/rings/leader-labels";
export {
  clearRingTooltipPosition,
  coversRing,
  outwardTooltipPosition as ringTooltipPosition,
} from "@/components/liquid-charts/liquid/core/tooltip-position";
import type { GlassCamera } from "@/components/liquid-charts/liquid/core/camera";
import type { Density } from "@/components/liquid-charts/liquid/core/style";

const finite = (value: number) => (Number.isFinite(value) && value > 0 ? value : 0);
/** How much of the circle a ring chart spans: the whole of it, or the upper half. */
export type Arc = "full" | "semi";
export type ArcSpan = { from: number; span: number };
export const arcSpan = (arc: Arc): ArcSpan =>
  arc === "semi" ? { from: -Math.PI / 2, span: Math.PI } : { from: 0, span: TAU };

export type RingAngle = {
  start: number;
  end: number;
  mid: number;
  /** The value's true share of the total (not its drawn share, which `minAngle` may raise). */
  share: number;
};

/**
 * Angles for values round an arc, clockwise from `from`, each spanning its share of `span`.
 * Missing, negative and zero values span nothing (start = end) but keep their place, so a hidden
 * segment can spring shut where it stood. A non-zero value smaller than `minAngle` is raised to it,
 * taken proportionally from the larger ones, so a tiny slice stays visible and can be pointed at.
 */
export function ringAngles(
  values: readonly number[],
  { from = 0, span = TAU, minAngle = 0 }: { from?: number; span?: number; minAngle?: number } = {},
): RingAngle[] {
  const clean = values.map(finite);
  const total = clean.reduce((sum, value) => sum + value, 0);
  if (total <= 0) return clean.map(() => ({ start: from, end: from, mid: from, share: 0 }));
  const positive = clean.filter((value) => value > 0).length;
  const floor = Math.min(Math.max(0, minAngle), span / positive);
  const spans = clean.map((value) => (value / total) * span);
  // Raise small slices to the floor; shrink the rest to pay for it. Shrinking can push another
  // slice under the floor, so repeat until none moves.
  const raised = new Set<number>();
  for (let pass = 0; pass < clean.length; pass++) {
    let changed = false;
    spans.forEach((value, i) => {
      if (clean[i] > 0 && !raised.has(i) && value < floor) {
        raised.add(i);
        changed = true;
      }
    });
    if (!changed && pass > 0) break;
    const budget = span - raised.size * floor;
    const free = clean.reduce((sum, value, i) => (raised.has(i) ? sum : sum + value), 0);
    clean.forEach((value, i) => {
      spans[i] = value <= 0 ? 0 : raised.has(i) ? floor : (value / (free || 1)) * budget;
    });
    if (!changed) break;
  }
  let angle = from;
  return clean.map((value, i) => {
    const start = angle;
    angle += spans[i];
    return { start, end: angle, mid: (start + angle) / 2, share: value / total };
  });
}

/** The index of the segment that holds `angle`, or null in a gap past the arc's end. */
export function segmentAt(
  segments: readonly { start: number; end: number }[],
  angle: number,
): number | null {
  for (let i = 0; i < segments.length; i++) {
    const { start, end } = segments[i];
    if (end - start <= 1e-9) continue;
    if (normAngle(angle - start) < end - start) return i;
  }
  return null;
}

/** Whether a label of `width` × `height` fits on a segment spanning `span` radians, centred on
 * the ring's middle `radius`, in a ring `ringWidth` wide with `gap` px between segments. */
export function insideLabelFits(
  span: number,
  radius: number,
  ringWidth: number,
  width: number,
  height: number,
  gap = 0,
) {
  // The chord at the label's inner edge is the narrowest place it must fit.
  const r = Math.max(0, radius - height / 2);
  const chord = span >= Math.PI ? 2 * r : 2 * r * Math.sin(span / 2);
  return width + 8 <= chord - gap && height + 4 <= ringWidth;
}

/* ——— Layouts ——— */

export type RingFit = {
  cx: number;
  cy: number;
  /** The ring's outer and inner edges, and its middle. */
  outer: number;
  inner: number;
  radius: number;
  ringWidth: number;
};

/**
 * A ring that fits a plot of `width` × `height`, leaving `side` px each side for labels and `top`
 * and `bottom` px above and below. `inner` is the hole as a share of the outer radius (0 for a pie).
 * A semi arc sits on its baseline, so it spans the width and only the upper half of the height.
 */
export function fitRing(
  width: number,
  height: number,
  {
    arc = "full",
    inner = 0.62,
    side = 0,
    top = 0,
    bottom = 0,
    max = Infinity,
    min = 24,
  }: {
    arc?: Arc;
    inner?: number;
    side?: number;
    top?: number;
    bottom?: number;
    max?: number;
    min?: number;
  } = {},
): RingFit {
  const across = (width - side * 2) / 2;
  const tall = arc === "semi" ? height - top - bottom : (height - top - bottom) / 2;
  const outer = Math.max(min, Math.min(across, tall, max));
  const cx = width / 2;
  const cy = arc === "semi" ? height - bottom : top + (height - top - bottom) / 2;
  const hole = clamp(inner, 0, 0.94) * outer;
  const ringWidth = outer - hole;
  return {
    cx: settle(cx),
    cy: settle(cy),
    outer: settle(outer),
    inner: settle(hole),
    radius: settle(outer - ringWidth / 2),
    ringWidth: settle(ringWidth),
  };
}

/** What a tilted slab's height shows, mapped into [low, high] px: the value's share, or a second
 * measure such as growth (whose lowest value maps to `low`, and zero never above a positive). */
export function tiltHeights(
  measure: readonly number[],
  low: number,
  high: number,
  { fromZero = true }: { fromZero?: boolean } = {},
): number[] {
  const values = measure.map((value) => (Number.isFinite(value) ? value : 0));
  if (!values.length) return [];
  let lo = Math.min(...values),
    hi = Math.max(...values);
  if (fromZero) {
    lo = Math.min(lo, 0);
    hi = Math.max(hi, 0);
  }
  if (hi - lo < 1e-12) return values.map(() => settle((low + high) / 2));
  return values.map((value) => settle(low + ((value - lo) / (hi - lo)) * (high - low)));
}

/* ——— Gauge ——— */

export type GaugeBand = { to: number };

/** The needle's angle for `value` on a gauge spanning `sweep` radians centred on 12 o'clock,
 * from `min` at its left end to `max` at its right. Values outside the domain pin to the ends. */
export function gaugeAngle(value: number, [min, max]: readonly [number, number], sweep: number) {
  const t = max === min || !Number.isFinite(value) ? 0 : clamp((value - min) / (max - min), 0, 1);
  return -sweep / 2 + t * sweep;
}

/** Each band's angles on the gauge. Bands run from the domain's minimum (or the previous band's
 * `to`) to their own `to`, clamped into the domain and kept in order. */
export function gaugeBands(
  bands: readonly GaugeBand[],
  domain: readonly [number, number],
  sweep: number,
): (RingAngle & { from: number; to: number })[] {
  let from = domain[0];
  return bands.map((band) => {
    const to = clamp(Math.max(band.to, from), domain[0], domain[1]);
    const start = gaugeAngle(from, domain, sweep),
      end = gaugeAngle(to, domain, sweep);
    const result = {
      start,
      end,
      mid: (start + end) / 2,
      share: domain[1] === domain[0] ? 0 : (to - from) / (domain[1] - domain[0]),
      from,
      to,
    };
    from = to;
    return result;
  });
}

/** The band that holds `value` (the last band whose range reaches it), or null for no bands. */
export function bandOf(
  value: number,
  bands: readonly { from: number; to: number }[],
): number | null {
  if (!bands.length) return null;
  for (let i = 0; i < bands.length; i++) if (value <= bands[i].to) return i;
  return bands.length - 1;
}

/** Decimals a gauge's reading needs to tell values apart across its domain: none from a span of
 * 10 up (0–100, 0–50), two on 0–1, so a share-of-one gauge never reads "1" for 0.87. At most 3. */
export function gaugeDecimals([min, max]: readonly [number, number]): number {
  const span = Math.abs(max - min);
  if (!Number.isFinite(span) || span <= 0 || span >= 10) return 0;
  return Math.max(0, Math.min(3, Math.ceil(2 - Math.log10(span) - 1e-9)));
}

/** The bands whose `to` falls outside the domain (0–100 thresholds on a 0–1 gauge, say): they are
 * clamped onto the dial's ends, so the gauge says so in development rather than rescaling. */
export function bandsOffDomain(
  bands: readonly { key?: string; to: number }[],
  [min, max]: readonly [number, number],
): { index: number; key?: string; to: number }[] {
  const lo = Math.min(min, max),
    hi = Math.max(min, max);
  return bands
    .map((band, index) => ({ index, key: band.key, to: band.to }))
    .filter(({ to }) => Number.isFinite(to) && (to < lo - 1e-9 || to > hi + 1e-9));
}

/** The fewest decimals (up to `most`) that set a scale stop exactly: 0.6 → 1, 1 → 0, 0.25 → 2. */
export function stopDecimals(value: number, most: number): number {
  for (let d = 0; d < most; d++) {
    const k = 10 ** d;
    if (Math.abs(Math.round(value * k) / k - value) < 1e-9) return d;
  }
  return most;
}

/* ——— Progress rings ——— */

export type ProgressArc = {
  /** Value over goal, unclamped (1.24 for 124%). */
  progress: number;
  /** The first lap, from the ring's start. */
  end: number;
  /** Past the goal, the second lap's end; null until the goal is reached. Capped at a full lap. */
  overflow: number | null;
};

/** How far round a progress ring goes: one lap to the goal, then a second lap riding on top of the
 * first for anything over it. A missing or zero goal reads as no progress. */
export function progressArc(value: number, goal: number): ProgressArc {
  const progress = goal > 0 && Number.isFinite(value) ? Math.max(0, value / goal) : 0;
  return {
    progress,
    end: Math.min(progress, 1) * TAU,
    overflow: progress > 1 ? Math.min(progress - 1, 1) * TAU : null,
  };
}

/** Concentric rings inside `outer`, outermost first: each ring's middle radius and the shared
 * thickness. The thickness gives way so the innermost ring keeps a hole of `hole` × outer. */
export function progressRingLayout(
  count: number,
  outer: number,
  { thickness, gap = 4, hole = 0.3 }: { thickness?: number; gap?: number; hole?: number } = {},
): { radii: number[]; thickness: number } {
  const n = Math.max(1, count);
  const room = outer * (1 - hole);
  const wanted = thickness ?? clamp(outer * (n === 1 ? 0.2 : 0.17), 6, 30);
  const width = Math.max(2, Math.min(wanted, (room - gap * (n - 1)) / n));
  return {
    thickness: settle(width),
    radii: Array.from({ length: count }, (_, i) => settle(outer - width / 2 - i * (width + gap))),
  };
}

/** The donut's tilt camera: radians above the floor, and its distance in outer radii. */
const ELEVATION = 0.5;
const DISTANCE = 3.6;

export type DonutGeometry = {
  fit: RingFit;
  tilt: {
    outer: number;
    radius: number;
    width: number;
    depth: number;
    low: number;
    high: number;
    gap: number;
    camera: GlassCamera;
  };
  label: { offset: number; reach: number };
};

/** The flat ring and its tilted view for a plot of `width` × `height`. Outside labels take
 * `side` px each side; the tilted ring fills the plot around the same centre. */
export function donutGeometry(
  width: number,
  height: number,
  {
    arc = "full",
    inner = 0.62,
    side = 0,
    density = "standard",
    gap = 6,
  }: { arc?: Arc; inner?: number; side?: number; density?: Density; gap?: number } = {},
): DonutGeometry {
  const tile = density === "tile";
  const pad = tile ? 4 : 12;
  const fit = fitRing(width, height, {
    arc,
    inner,
    side,
    top: arc === "semi" ? pad + 6 : pad,
    bottom: arc === "semi" ? 6 : pad,
    max: density === "hero" ? 200 : 170,
  });
  const share = fit.ringWidth / Math.max(1, fit.outer);
  const thick = Math.min(0.62, share * 1.12);
  const rise = Math.min(thick * 1.25, 0.62);
  // Tilted, every slab is labelled on a pin: a row above the ring (under the note and the view
  // switch) and a row below it. The camera frames the ring between the two.
  const framed = framePinned(height, {
    rise,
    elevation: ELEVATION,
    distance: DISTANCE,
    max: Math.min(width * 0.3, height * 0.47, 250),
    rows: tile ? 0 : 1,
    top: tile ? 0 : 30,
    lift: tile ? 0 : 16,
  });
  const outer = settle(Math.max(40, framed.outer));
  const ringWidth = settle(outer * thick);
  const radius = settle(outer - ringWidth / 2);
  const depth = settle(ringWidth * 0.7);
  const high = settle(Math.min(ringWidth * 1.25, outer * 0.62));
  return {
    fit,
    tilt: {
      outer,
      radius,
      width: ringWidth,
      depth,
      low: settle(Math.min(ringWidth * 0.3, 18)),
      high,
      gap: gap + 1,
      camera: {
        tilt: 1,
        elevation: ELEVATION,
        distance: settle(outer * DISTANCE),
        // The floor's centre sits so the ring, its tallest slab and the pins' rows are centred in
        // the plot.
        anchor: {
          x: fit.cx,
          y: settle(Math.min(height * 0.7, framed.y)),
        },
      },
    },
    label: { offset: tile ? 8 : 16, reach: tile ? 6 : 12 },
  };
}

/** The gauge's ring for a plot: centred across, its pivot low enough that the arc's top, the band
 * names over it and the reading under the pivot all fit. */
export function gaugeGeometry(
  width: number,
  height: number,
  sweep: number,
  { labelled = true, tile = false }: { labelled?: boolean; tile?: boolean } = {},
): RingFit & { reading: number } {
  const top = labelled ? 26 : 6;
  // Under the pivot: the arc's own ends (for sweeps past a half turn), and the reading.
  const reading = tile ? 30 : 62;
  const ends = Math.max(0, -Math.cos(sweep / 2));
  const side = labelled ? 44 : 8;
  const outer = Math.max(
    30,
    Math.min(width / 2 - side, height - top - reading, (height - top - 6) / (1 + ends), 170),
  );
  const ringWidth = settle(Math.max(10, outer * (tile ? 0.22 : 0.2)));
  const cy = settle(top + outer);
  return {
    cx: settle(width / 2),
    cy,
    outer: settle(outer),
    inner: settle(outer - ringWidth),
    radius: settle(outer - ringWidth / 2),
    ringWidth,
    reading,
  };
}

/** Concentric rings for the plot's size: their centre, each ring's middle radius and thickness. */
export function progressGeometry(
  width: number,
  height: number,
  count: number,
  { thickness, gap = 5 }: { thickness?: number; gap?: number } = {},
) {
  const outer = Math.max(20, Math.min(width, height) / 2 - 6);
  const { radii, thickness: t } = progressRingLayout(count, outer, { thickness, gap });
  return { cx: width / 2, cy: height / 2, outer, radii, thickness: t, gap };
}
