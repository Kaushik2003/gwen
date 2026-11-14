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
 * Radial geometry: the pure maths behind radial bars, radar, radial lines and areas, and polar
 * area (rose) charts.
 *
 * Angles are clockwise radians from 12 o'clock, the convention of the core's arc helpers and the
 * glass renderers, so a point at angle a and radius r sits at (cx + r·sin a, cy − r·cos a).
 * Everything here is a plain function of numbers, safe every animation frame and on the server,
 * and unit-tested in tests/liquid-radial-*.test.ts.
 */

import {
  placeLabels,
  type LabelRequest,
  type LabelSide,
  type PlacedLabel,
} from "@/components/liquid-charts/liquid/core/labels";
import { clamp, finite, overlaps, settle, type Rect } from "@/components/liquid-charts/liquid/core/geometry";
import { normAngle, polar, TAU, wrapAngle, type Point } from "@/components/liquid-charts/liquid/core/arc";
import { niceDomain, timeTicks } from "@/components/liquid-charts/liquid/core/scale";
import type { RingTilt } from "@/components/liquid-charts/liquid/rings/ring-marks";
import { framePinned } from "@/components/liquid-charts/liquid/rings/pin-labels";

export { normAngle, polar, TAU, wrapAngle, type Point };

export type Box = { x: number; y: number; width: number; height: number };
export type Size = { width: number; height: number };

/** A box as a rect. */
export const boxRect = (box: Box): Rect => ({
  left: box.x,
  top: box.y,
  right: box.x + box.width,
  bottom: box.y + box.height,
});

/**
 * The tight bounding box of a ring sector between radii `inner` and `outer` from `a0` to `a1`
 * (clockwise, a1 ≥ a0): its four corners, plus the outer rim wherever the sector passes a
 * cardinal direction. An `inner` of 0 includes the centre. Keeping glass masks this small is what
 * keeps the optical renderer cheap.
 */
export function sectorBounds(
  cx: number,
  cy: number,
  inner: number,
  outer: number,
  a0: number,
  a1: number,
): Box {
  const points: Point[] = [];
  const span = Math.max(0, a1 - a0);
  if (span >= TAU - 1e-6)
    return { x: cx - outer, y: cy - outer, width: outer * 2, height: outer * 2 };
  points.push(polar(cx, cy, outer, a0), polar(cx, cy, outer, a1));
  if (inner > 0) points.push(polar(cx, cy, inner, a0), polar(cx, cy, inner, a1));
  else points.push({ x: cx, y: cy });
  for (let k = 0; k < 4; k++) {
    const cardinal = (k * Math.PI) / 2;
    // The first cardinal at or after a0.
    const at = a0 + normAngle(cardinal - a0);
    if (at <= a1 + 1e-9) points.push(polar(cx, cy, outer, at));
  }
  const xs = points.map((p) => p.x),
    ys = points.map((p) => p.y);
  const x = Math.min(...xs),
    y = Math.min(...ys);
  return {
    x: settle(x),
    y: settle(y),
    width: settle(Math.max(...xs) - x),
    height: settle(Math.max(...ys) - y),
  };
}

/** The value domain a radial scale reads against: from zero (or the data's low end when
 * `includeZero` is false) to a round top. Empty or all-missing data gives [0, 1]. */
export function radialDomain(
  values: readonly (number | null | undefined)[],
  { includeZero = true, count = 4 }: { includeZero?: boolean; count?: number } = {},
): [number, number] {
  return niceDomain(values.filter(finite), { includeZero, count });
}

/* ——— Radial bars ——— */

export type RadialArc = "full" | "semi";

export type RadialBarsGeometry = {
  cx: number;
  cy: number;
  /** The outermost track's outer edge and the innermost track's inner edge. */
  outer: number;
  inner: number;
  /** Where the tracks start and how far they run (radians, clockwise). */
  from: number;
  span: number;
  /** Each track's middle radius, by position (0 is the outermost). */
  radii: number[];
  thickness: number;
  gap: number;
};

/**
 * Concentric tracks for a plot of `width` × `height`. A full chart runs each track three quarters
 * of the way round from 12 o'clock, leaving the top-left quadrant open for the names at each
 * track's start; a semi chart runs over the top from 9 o'clock to 3 o'clock and sits on its
 * baseline, with `bottom` px under it for the names. Thickness gives way so the innermost track
 * keeps a hole of `hole` × outer; `labelReach` is the widest start label plus its gap, which may
 * push the circle right so a long name still fits in the open quadrant.
 */
export function radialBarsGeometry(
  width: number,
  height: number,
  count: number,
  {
    arc = "full",
    thickness,
    gap: gapOption,
    hole = 0.24,
    pad = 22,
    bottom = 40,
    labelReach = 0,
    max = 190,
  }: {
    arc?: RadialArc;
    thickness?: number;
    gap?: number;
    hole?: number;
    /** Px kept clear round the outside (for the scale's tick labels). */
    pad?: number;
    /** Semi only: px under the baseline. */
    bottom?: number;
    labelReach?: number;
    max?: number;
  } = {},
): RadialBarsGeometry {
  const n = Math.max(1, count);
  const gap = gapOption ?? (n >= 5 ? 4 : 6);
  const semi = arc === "semi";
  let outer: number, cx: number, cy: number;
  if (semi) {
    outer = Math.max(24, Math.min(width / 2 - pad, height - bottom - pad, max));
    cx = width / 2;
    cy = pad + outer;
  } else {
    outer = Math.max(24, Math.min(height / 2 - pad, width / 2 - pad, max));
    // The start labels sit left of 12 o'clock; push the circle right when they would not fit.
    cx = Math.max(width / 2, Math.min(width - pad - outer, labelReach + 4));
    cy = height / 2;
  }
  const room = outer * (1 - clamp(hole, 0, 0.9));
  const wanted = thickness ?? clamp((room - gap * (n - 1)) / n, 6, n === 1 ? 34 : 26);
  // Tracks give way first, then the gaps, so even a great many tracks stay inside the circle.
  let t = Math.max(1, Math.min(wanted, (room - gap * (n - 1)) / n));
  if (t * n > room) t = room / n;
  const g = n > 1 ? Math.max(0, Math.min(gap, (room - t * n) / (n - 1))) : 0;
  const radii = Array.from({ length: count }, (_, i) => settle(outer - t / 2 - i * (t + g)));
  return {
    cx: settle(cx),
    cy: settle(cy),
    outer: settle(outer),
    inner: settle(outer - n * t - (n - 1) * g),
    from: semi ? -Math.PI / 2 : 0,
    span: semi ? Math.PI : Math.PI * 1.5,
    radii,
    thickness: settle(t),
    gap: settle(g),
  };
}

/** The angle a value reaches on a track: its share of `max` along the span, clamped to the span
 * (a negative or missing value stays at the start). */
export function barAngle(value: number, max: number, from: number, span: number) {
  const share = max > 0 && finite(value) ? clamp(value / max, 0, 1) : 0;
  return from + share * span;
}

/** Track positions: `descending` puts the largest value outermost (ties keep data order);
 * `data` keeps the data's order, first outermost. Returns each datum's position. */
export function trackOrder(
  values: readonly number[],
  order: "descending" | "data" = "descending",
): number[] {
  const indices = values.map((_, i) => i);
  if (order === "descending")
    indices.sort(
      (a, b) =>
        (finite(values[b]) ? values[b] : -Infinity) - (finite(values[a]) ? values[a] : -Infinity) ||
        a - b,
    );
  const position = new Array<number>(values.length);
  indices.forEach((datum, p) => (position[datum] = p));
  return position;
}

/** The track (by position) under a point: the annulus whose middle is nearest, within half a
 * track and half a gap. The whole lap counts, so a short bar is as easy to point at as a long one. */
export function trackAt(
  geometry: Pick<RadialBarsGeometry, "cx" | "cy" | "radii" | "thickness" | "gap">,
  x: number,
  y: number,
  slack = 2,
): number | null {
  const r = Math.hypot(x - geometry.cx, y - geometry.cy);
  let best: number | null = null,
    distance = Infinity;
  geometry.radii.forEach((radius, i) => {
    const d = Math.abs(r - radius);
    if (d <= geometry.thickness / 2 + geometry.gap / 2 + slack && d < distance) {
      distance = d;
      best = i;
    }
  });
  return best;
}

/** A track's glass, for keeping labels off it: its middle radius, width and drawn angles. */
export type TrackGlass = { radius: number; width: number; start: number; end: number };

export type ArcLabelRequest = {
  id: string;
  /** The track this label belongs to (its index in `glass`), its middle radius and where its arc
   * ends. The label slides along the track from just past the end. */
  track: number;
  radius: number;
  angle: number;
  /** How far along the track the label may slide (an angle), usually the track's span end. */
  limit: number;
  width: number;
  height: number;
  /** Higher places first. ≤ 0 is never shown. */
  priority: number;
};
export type ArcLabel = {
  id: string;
  visible: boolean;
  left: number;
  top: number;
  /** The angle of the label's centre along its track. */
  angle: number;
};

/** True when any sample of the box lies on glass: inside one of the tracks' annuli and within its
 * drawn angles. Corners, edge midpoints and the centre are checked. */
export function boxOnGlass(
  box: Rect,
  glass: readonly TrackGlass[],
  cx: number,
  cy: number,
  skip = -1,
): boolean {
  const xs = [box.left, (box.left + box.right) / 2, box.right];
  const ys = [box.top, (box.top + box.bottom) / 2, box.bottom];
  for (const x of xs)
    for (const y of ys) {
      const r = Math.hypot(x - cx, y - cy);
      const a = normAngle(Math.atan2(x - cx, cy - y));
      for (let i = 0; i < glass.length; i++) {
        if (i === skip) continue;
        const g = glass[i];
        if (g.end - g.start <= 1e-6) continue;
        if (Math.abs(r - g.radius) > g.width / 2) continue;
        if (normAngle(a - g.start) <= g.end - g.start) return true;
      }
    }
  return false;
}

/**
 * Value labels at the ends of radial bars, set level (never rotated) and kept clear of each other:
 * each starts just past its bar's end in its own track and, where it would touch another label, an
 * obstacle, another track's glass or the bounds, slides further along the track in small steps up
 * to `limit`, then tries straight out from the bar's end (a full outermost bar's value goes past
 * the rim). Highest priority places first; a label with nowhere to go is hidden (the tooltip still
 * reads it). Its own glass counts too, so a label never sits on its bar.
 */
export function placeArcLabels(
  requests: readonly ArcLabelRequest[],
  {
    cx,
    cy,
    glass,
    bounds,
    obstacles = [],
    gap = 6,
    step = 4,
    inset = 2.5,
  }: {
    cx: number;
    cy: number;
    glass: readonly TrackGlass[];
    bounds: Rect;
    obstacles?: readonly Rect[];
    /** Px between the bar's end and the label's near edge. */
    gap?: number;
    /** Px each slide moves the label along its track. */
    step?: number;
    /** Px of a label's box that may graze glass (the empty space above and below its glyphs). */
    inset?: number;
  },
): ArcLabel[] {
  const result: ArcLabel[] = requests.map((r) => {
    const at = polar(cx, cy, r.radius, r.angle);
    return {
      id: r.id,
      visible: false,
      left: settle(at.x - r.width / 2),
      top: settle(at.y - r.height / 2),
      angle: r.angle,
    };
  });
  const order = requests
    .map((_, i) => i)
    .filter((i) => requests[i].priority > 0)
    .sort((a, b) => requests[b].priority - requests[a].priority || a - b);
  const placed: Rect[] = [];
  for (const i of order) {
    const r = requests[i];
    const radius = Math.max(1, r.radius);
    // The box's half extent along the track at angle a, so its near edge sits `gap` past the end.
    const along = (a: number) =>
      (Math.abs(Math.cos(a)) * r.width) / 2 + (Math.abs(Math.sin(a)) * r.height) / 2;
    let a = r.angle + (gap + along(r.angle)) / radius;
    a = r.angle + (gap + along(a)) / radius;
    const tries = Math.max(1, Math.ceil(((r.limit - a) * radius) / step) + 1);
    // Candidates: sliding along the track from just past the end, then (for a bar whose track is
    // full to its limit, usually the outermost) straight out from its end, past the rim.
    const candidates: { angle: number; radius: number }[] = [];
    for (let k = 0; k < Math.min(tries, 400); k++) {
      const angle = Math.min(a + (k * step) / radius, Math.max(a, r.limit));
      candidates.push({ angle, radius });
      if (angle >= r.limit) break;
    }
    const along0 = along(r.angle);
    const across =
      (Math.abs(Math.sin(r.angle)) * r.width) / 2 + (Math.abs(Math.cos(r.angle)) * r.height) / 2;
    for (let k = 1; k <= 8; k++)
      candidates.push({
        angle: r.angle - Math.min(along0, 12) / radius,
        radius: radius + across + k * step,
      });
    for (const candidate of candidates) {
      const centre = polar(cx, cy, candidate.radius, candidate.angle);
      const box: Rect = {
        left: centre.x - r.width / 2,
        top: centre.y - r.height / 2,
        right: centre.x + r.width / 2,
        bottom: centre.y + r.height / 2,
      };
      const inside =
        box.left >= bounds.left &&
        box.right <= bounds.right &&
        box.top >= bounds.top &&
        box.bottom <= bounds.bottom;
      const clear =
        inside &&
        !placed.some((other) => overlaps(box, other, 2)) &&
        !obstacles.some((other) => overlaps(box, other, 2)) &&
        !boxOnGlass(
          {
            left: box.left + inset,
            top: box.top + inset,
            right: box.right - inset,
            bottom: box.bottom - inset,
          },
          glass,
          cx,
          cy,
        );
      if (clear) {
        placed.push(box);
        result[i] = {
          id: r.id,
          visible: true,
          left: settle(box.left),
          top: settle(box.top),
          angle: settle(candidate.angle * 1e4) / 1e4,
        };
        break;
      }
    }
  }
  return result;
}

/** Labels at the start of each track, right-aligned just left of 12 o'clock in the open quadrant
 * (full charts). Returns each label's box; `widths`/`heights` by position. */
export function startLabelBoxes(
  geometry: Pick<RadialBarsGeometry, "cx" | "cy" | "radii">,
  sizes: readonly Size[],
  gap = 8,
): Rect[] {
  return geometry.radii.map((radius, i) => {
    const { width, height } = sizes[i] ?? { width: 0, height: 0 };
    const right = geometry.cx - gap;
    const middle = geometry.cy - radius;
    return {
      left: settle(right - width),
      top: settle(middle - height / 2),
      right: settle(right),
      bottom: settle(middle + height / 2),
    };
  });
}

/* ——— Radar ——— */

/** Equal spokes, the first at `from` (12 o'clock), clockwise. */
export const spokeAngles = (count: number, from = 0) =>
  Array.from({ length: Math.max(0, count) }, (_, i) => from + (i / Math.max(1, count)) * TAU);

/** How an axis label sits against the point where it meets the circle: to the right of it on the
 * right half, left of it on the left half, centred at the top and bottom. */
export function axisAnchor(angle: number) {
  const s = Math.sin(angle),
    c = Math.cos(angle);
  const horizontal: "start" | "middle" | "end" = s > 0.2 ? "start" : s < -0.2 ? "end" : "middle";
  const vertical: "top" | "middle" | "bottom" = c > 0.35 ? "bottom" : c < -0.35 ? "top" : "middle";
  return { horizontal, vertical };
}

/** The box of an axis label of `size` whose anchor point is `at`, aligned by `axisAnchor`. */
export function axisLabelBox(at: Point, angle: number, size: Size): Rect {
  const { horizontal, vertical } = axisAnchor(angle);
  const left =
    horizontal === "start"
      ? at.x
      : horizontal === "end"
        ? at.x - size.width
        : at.x - size.width / 2;
  const top =
    vertical === "bottom" ? at.y - size.height : vertical === "top" ? at.y : at.y - size.height / 2;
  return { left, top, right: left + size.width, bottom: top + size.height };
}

/**
 * The largest radar that fits a plot with its axis labels round the outside: each label sits
 * `gap` px beyond the circle along its spoke, aligned by `axisAnchor`, and every label must stay
 * `pad` px inside the plot. The circle is centred; `max` caps it.
 */
export function radarGeometry(
  width: number,
  height: number,
  labels: readonly Size[],
  {
    gap = 12,
    pad = 4,
    max = 200,
    min = 24,
    from = 0,
  }: {
    gap?: number;
    pad?: number;
    max?: number;
    min?: number;
    from?: number;
  } = {},
): { cx: number; cy: number; radius: number } {
  const cx = width / 2,
    cy = height / 2;
  let radius = Math.min(max, width / 2 - pad - gap, height / 2 - pad - gap);
  const angles = spokeAngles(labels.length, from);
  labels.forEach((size, i) => {
    const a = angles[i];
    const s = Math.sin(a),
      c = Math.cos(a);
    const { horizontal, vertical } = axisAnchor(a);
    // Horizontal: the label's far edge from the centre, as a function of R.
    const halfW = horizontal === "middle" ? size.width / 2 : size.width;
    if (Math.abs(s) > 1e-6)
      radius = Math.min(radius, (width / 2 - pad - halfW) / Math.abs(s) - gap);
    else radius = Math.min(radius, width / 2 - pad - halfW);
    const halfH = vertical === "middle" ? size.height / 2 : size.height;
    if (Math.abs(c) > 1e-6)
      radius = Math.min(radius, (height / 2 - pad - halfH) / Math.abs(c) - gap);
    else radius = Math.min(radius, height / 2 - pad - halfH);
  });
  return { cx: settle(cx), cy: settle(cy), radius: settle(Math.max(min, radius)) };
}

/**
 * Axis labels round a radar: each at its spoke, `gap` px out and slid back inside `bounds` if it
 * would leave them; where one would touch a label already placed it steps further out along its
 * spoke (up to `reach` px more), and if it still collides it is hidden. Labels place in their given
 * order, so pass the important ones first.
 */
export function placeAxisLabels(
  angles: readonly number[],
  sizes: readonly Size[],
  {
    cx,
    cy,
    radius,
    gap = 12,
    reach = 24,
    bounds,
  }: { cx: number; cy: number; radius: number; gap?: number; reach?: number; bounds?: Rect },
): { left: number; top: number; visible: boolean }[] {
  const placed: Rect[] = [];
  // A label that would leave the bounds slides back inside them: a criterion is never dropped for
  // want of a few px.
  const inside = (box: Rect): Rect => {
    if (!bounds) return box;
    const dx =
      box.left < bounds.left
        ? bounds.left - box.left
        : box.right > bounds.right
          ? bounds.right - box.right
          : 0;
    const dy =
      box.top < bounds.top
        ? bounds.top - box.top
        : box.bottom > bounds.bottom
          ? bounds.bottom - box.bottom
          : 0;
    return {
      left: box.left + dx,
      top: box.top + dy,
      right: box.right + dx,
      bottom: box.bottom + dy,
    };
  };
  return angles.map((angle, i) => {
    const size = sizes[i] ?? { width: 0, height: 0 };
    let first: Rect | null = null;
    for (let extra = 0; extra <= reach; extra += 4) {
      const box = inside(axisLabelBox(polar(cx, cy, radius + gap + extra, angle), angle, size));
      first ??= box;
      if (!placed.some((other) => overlaps(box, other, 3))) {
        placed.push(box);
        return { left: settle(box.left), top: settle(box.top), visible: true };
      }
    }
    const fallback = first ?? { left: cx, top: cy, right: cx, bottom: cy };
    return { left: settle(fallback.left), top: settle(fallback.top), visible: false };
  });
}

/** Each value's radius on a radar of `radius` reading `domain` from its centre; values beyond the
 * domain stop at its edge, missing values are NaN. */
export function radarRadii(
  values: readonly (number | null | undefined)[],
  domain: readonly [number, number],
  radius: number,
): number[] {
  const [d0, d1] = domain;
  return values.map((v) =>
    finite(v) ? clamp(((v - d0) / (d1 - d0 || 1)) * radius, 0, radius) : Number.NaN,
  );
}

/** A closed outline through each spoke's point at `radii` (NaN radii are left out, so the outline
 * runs straight between their neighbours), scaled by `scale` from the centre (the bloom). Empty
 * when fewer than two points remain. */
export function radarPath(
  cx: number,
  cy: number,
  angles: readonly number[],
  radii: readonly number[],
  scale = 1,
): string {
  const points = angles
    .map((a, i) => (Number.isFinite(radii[i]) ? polar(cx, cy, radii[i] * scale, a) : null))
    .filter((p): p is Point => p !== null);
  if (points.length < 2) return "";
  return `${points.map((p, i) => `${i ? "L" : "M"}${settle(p.x)} ${settle(p.y)}`).join("")}Z`;
}

/** The spoke nearest a point's angle from the centre. */
export function spokeAt(angles: readonly number[], cx: number, cy: number, x: number, y: number) {
  if (!angles.length) return null;
  const a = Math.atan2(x - cx, cy - y);
  let best = 0,
    distance = Infinity;
  angles.forEach((angle, i) => {
    const d = Math.abs(wrapAngle(a - angle));
    if (d < distance) {
      distance = d;
      best = i;
    }
  });
  return best;
}

/** The order to read series at one spoke: the focused one first, then highest first; missing
 * values last. Returns series indices. */
export function readingOrder(values: readonly (number | null | undefined)[], focus: number | null) {
  return values
    .map((_, i) => i)
    .sort((a, b) => {
      if (a === focus) return -1;
      if (b === focus) return 1;
      const va = finite(values[a]) ? values[a] : -Infinity;
      const vb = finite(values[b]) ? values[b] : -Infinity;
      return vb - va || a - b;
    });
}

/** The series whose point on a spoke is nearest `r` px from the centre, within `slack` px. */
export function seriesAt(radii: readonly number[], r: number, slack = 22): number | null {
  let best: number | null = null,
    distance = Infinity;
  radii.forEach((radius, i) => {
    if (!Number.isFinite(radius)) return;
    const d = Math.abs(radius - r);
    if (d <= slack && d < distance) {
      distance = d;
      best = i;
    }
  });
  return best;
}

/** A closed polygon's area (shoelace), for drawing the larger of two fills underneath. */
export function polygonArea(angles: readonly number[], radii: readonly number[]) {
  let area = 0;
  const points = angles
    .map((a, i) => (Number.isFinite(radii[i]) ? polar(0, 0, radii[i], a) : null))
    .filter((p): p is Point => p !== null);
  for (let i = 0; i < points.length; i++) {
    const p = points[i],
      q = points[(i + 1) % points.length];
    area += p.x * q.y - q.x * p.y;
  }
  return Math.abs(area) / 2;
}

/* ——— Radial time: a year or a day wrapped round ——— */

export type Period = "year" | "day" | "week";
const DAY = 86_400_000;

/** The UTC start of the period holding `time`: 1 January, midnight, or Monday midnight. */
export function periodStart(time: number, period: Period): number {
  const d = new Date(time);
  if (period === "year") return Date.UTC(d.getUTCFullYear(), 0, 1);
  const midnight = Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate());
  if (period === "day") return midnight;
  return midnight - ((d.getUTCDay() + 6) % 7) * DAY;
}

/** The length of the period holding `time`, ms (a leap year has 366 days). */
export function periodLength(time: number, period: Period): number {
  if (period === "day") return DAY;
  if (period === "week") return 7 * DAY;
  const y = new Date(time).getUTCFullYear();
  return Date.UTC(y + 1, 0, 1) - Date.UTC(y, 0, 1);
}

/** How far through its period a time is, 0–1. */
export function periodFraction(time: number, period: Period): number {
  if (!Number.isFinite(time)) return Number.NaN;
  const start = periodStart(time, period);
  return clamp((time - start) / periodLength(time, period), 0, 1 - 1e-12);
}

/** The angle of a time on a clock face of its period, from `from` (12 o'clock). */
export const periodAngle = (time: number, period: Period, from = 0) =>
  from + periodFraction(time, period) * TAU;

/**
 * Ticks round the rim for a period: month starts for a year (named in the middle of each month),
 * every three hours for a day (named at the tick), each day for a week. Built on the core's
 * `timeTicks`, so months are calendar months and labels follow the locale.
 */
export function periodTicks(
  reference: number,
  period: Period,
  {
    from = 0,
    hours = 3,
    locale = "en-GB",
  }: { from?: number; hours?: number; locale?: string } = {},
): { value: number; label: string; angle: number; labelAngle: number }[] {
  const start = periodStart(reference, period);
  const length = periodLength(reference, period);
  const angle = (value: number) => from + ((value - start) / length) * TAU;
  if (period === "year") {
    // Thirteen, not twelve: a year is a little over twelve 30-day steps.
    const ticks = timeTicks([start, start + length - 1], 13, { locale });
    return ticks.map((tick, i) => {
      const next = ticks[i + 1]?.value ?? start + length;
      return {
        value: tick.value,
        label: tick.label,
        angle: angle(tick.value),
        labelAngle: angle((tick.value + next) / 2),
      };
    });
  }
  if (period === "week") {
    const names = new Intl.DateTimeFormat(locale, { weekday: "short", timeZone: "UTC" });
    return Array.from({ length: 7 }, (_, i) => {
      const value = start + i * DAY;
      return {
        value,
        label: names.format(new Date(value)),
        angle: angle(value),
        labelAngle: angle(value + DAY / 2),
      };
    });
  }
  const count = Math.max(1, Math.round(24 / Math.max(1, hours)));
  return timeTicks([start, start + length - 1], count, { locale }).map((tick) => ({
    value: tick.value,
    label: tick.label,
    angle: angle(tick.value),
    labelAngle: angle(tick.value),
  }));
}

const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
const WEEKDAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
const two = (n: number) => String(n).padStart(2, "0");

/** A moment's name for its period, in UTC and spelled out here rather than by the locale, so the
 * server's data table matches every browser's: "18 Oct 2026", "14:30", "Mon 14:30". */
export function momentLabel(time: number, period: Period): string {
  if (!Number.isFinite(time)) return "";
  const d = new Date(time);
  const clock = `${two(d.getUTCHours())}:${two(d.getUTCMinutes())}`;
  if (period === "day") return clock;
  if (period === "week") return `${WEEKDAYS[d.getUTCDay()]} ${clock}`;
  return `${d.getUTCDate()} ${MONTHS[d.getUTCMonth()]} ${d.getUTCFullYear()}`;
}

/** A radius for each value: `domain` mapped onto [inner, outer]; values outside stop at the ends. */
export function radialScale(domain: readonly [number, number], inner: number, outer: number) {
  const [d0, d1] = domain;
  return (value: number) =>
    finite(value)
      ? inner + clamp((value - d0) / (d1 - d0 || 1), 0, 1) * (outer - inner)
      : Number.NaN;
}

export type RadialSample = { fraction: number; value: number };

/**
 * A smooth radial curve through points at `fractions` (0–1 round the period) with `values` (null
 * or NaN for a gap), sampled every `resolution` of a turn so it follows the circle instead of
 * cutting chords across it. The curve is a monotone cubic in angle, so it never overshoots a data
 * point. Runs are split at gaps; when `closed` and the data meets itself across 12 o'clock, the
 * last run joins the first. Returns one array of samples per run.
 */
export function radialSamples(
  fractions: readonly number[],
  values: readonly (number | null | undefined)[],
  { closed = false, resolution = 1 / 360 }: { closed?: boolean; resolution?: number } = {},
): RadialSample[][] {
  const points = fractions
    .map((f, i) => ({ f, v: values[i] }))
    .filter((p) => Number.isFinite(p.f))
    .sort((a, b) => a.f - b.f);
  // Runs of defined values.
  const runs: { f: number; v: number }[][] = [];
  let run: { f: number; v: number }[] = [];
  for (const p of points) {
    if (finite(p.v)) run.push({ f: p.f, v: p.v });
    else if (run.length) {
      runs.push(run);
      run = [];
    }
  }
  if (run.length) runs.push(run);
  if (!runs.length) return [];
  // Joined across 12 o'clock: the first and last points are both defined and the step between
  // them is no bigger than the data's usual step.
  const allDefined = points.length > 2 && points.every((p) => finite(p.v));
  const typical =
    points.length > 1 ? (points[points.length - 1].f - points[0].f) / (points.length - 1) : 1;
  const wrapGap = points.length ? 1 - points[points.length - 1].f + points[0].f : 1;
  const wraps = closed && allDefined && wrapGap <= typical * 1.5 + 1e-9;
  if (wraps) {
    // One periodic run: pad with neighbours from the other side so the tangents are continuous.
    const only = runs[0];
    const n = only.length;
    const extended = [
      { f: only[n - 2].f - 1, v: only[n - 2].v },
      { f: only[n - 1].f - 1, v: only[n - 1].v },
      ...only,
      { f: only[0].f + 1, v: only[0].v },
      { f: only[1].f + 1, v: only[1].v },
    ];
    const curve = monotone(extended);
    const out: RadialSample[] = [];
    const start = only[0].f;
    const steps = Math.max(2, Math.ceil(1 / resolution));
    for (let k = 0; k <= steps; k++) {
      const f = start + k / steps;
      out.push({ fraction: f, value: curve(f) });
    }
    return [out];
  }
  return runs.map((points) => {
    if (points.length === 1) return [{ fraction: points[0].f, value: points[0].v }];
    const curve = monotone(points);
    const first = points[0].f,
      last = points[points.length - 1].f;
    const steps = Math.max(1, Math.ceil((last - first) / resolution));
    return Array.from({ length: steps + 1 }, (_, k) => {
      const f = first + ((last - first) * k) / steps;
      return { fraction: f, value: curve(f) };
    });
  });
}

/** A monotone cubic (Fritsch–Carlson) through sorted points: never overshoots between them. */
function monotone(points: readonly { f: number; v: number }[]) {
  const n = points.length;
  const dx = new Array<number>(n - 1),
    slope = new Array<number>(n - 1);
  for (let i = 0; i < n - 1; i++) {
    dx[i] = points[i + 1].f - points[i].f || 1e-9;
    slope[i] = (points[i + 1].v - points[i].v) / dx[i];
  }
  const m = new Array<number>(n);
  m[0] = slope[0];
  m[n - 1] = slope[n - 2];
  for (let i = 1; i < n - 1; i++) {
    if (slope[i - 1] * slope[i] <= 0) m[i] = 0;
    else {
      const w1 = 2 * dx[i] + dx[i - 1],
        w2 = dx[i] + 2 * dx[i - 1];
      m[i] = (w1 + w2) / (w1 / slope[i - 1] + w2 / slope[i]);
    }
  }
  return (f: number) => {
    if (f <= points[0].f) return points[0].v;
    if (f >= points[n - 1].f) return points[n - 1].v;
    let i = 0;
    let lo = 0,
      hi = n - 2;
    while (lo <= hi) {
      const mid = (lo + hi) >> 1;
      if (points[mid].f <= f) {
        i = mid;
        lo = mid + 1;
      } else hi = mid - 1;
    }
    const h = dx[i],
      t = (f - points[i].f) / h;
    const t2 = t * t,
      t3 = t2 * t;
    return (
      (2 * t3 - 3 * t2 + 1) * points[i].v +
      (t3 - 2 * t2 + t) * h * m[i] +
      (-2 * t3 + 3 * t2) * points[i + 1].v +
      (t3 - t2) * h * m[i + 1]
    );
  };
}

/** SVG path data for sampled runs, mapped through `toPoint`, keeping samples up to `until` (a
 * fraction of the turn from each run's start: the entrance drawing round). */
export function samplesPath(
  runs: readonly RadialSample[][],
  toPoint: (sample: RadialSample) => Point,
  { until = Infinity, close = false }: { until?: number; close?: boolean } = {},
): string {
  let d = "";
  for (const run of runs) {
    const kept = run.filter((s) => s.fraction <= until + 1e-9);
    if (kept.length < 2) continue;
    kept.forEach((sample, k) => {
      const p = toPoint(sample);
      d += `${k ? "L" : "M"}${settle(p.x)} ${settle(p.y)}`;
    });
    if (close && kept.length === run.length) d += "Z";
  }
  return d;
}

/** The index of the defined point nearest `fraction` round the circle, or null. */
export function nearestFraction(
  fractions: readonly number[],
  fraction: number,
  defined?: readonly boolean[],
): number | null {
  let best: number | null = null,
    distance = Infinity;
  fractions.forEach((f, i) => {
    if (!Number.isFinite(f) || (defined && !defined[i])) return;
    const d = Math.abs(f - fraction);
    const around = Math.min(d, 1 - d);
    if (around < distance - 1e-12) {
      distance = around;
      best = i;
    }
  });
  return best;
}

/** The sides a label at `angle` on a circle tries, in order: outward first, then round the circle
 * towards the open side, then inward. */
export function outwardSides(angle: number): LabelSide[] {
  const s = Math.sin(angle),
    c = Math.cos(angle);
  if (Math.abs(c) >= Math.abs(s)) {
    const first: LabelSide = c > 0 ? "above" : "below";
    const across: LabelSide = s >= 0 ? "right" : "left";
    return [first, across, across === "right" ? "left" : "right", c > 0 ? "below" : "above"];
  }
  const first: LabelSide = s > 0 ? "right" : "left";
  const across: LabelSide = c >= 0 ? "above" : "below";
  return [first, across, across === "above" ? "below" : "above", s > 0 ? "left" : "right"];
}

/**
 * Value labels round a circle: the core solver places every label on its first side, then each
 * label that found no room tries its other sides in turn (round the circle before inward), clear
 * of everything already placed. Returns one entry per request, in request order.
 */
export function placeRound(
  requests: readonly (LabelRequest & { sides: readonly LabelSide[] })[],
  options: { bounds: Rect; obstacles?: readonly Rect[]; gap?: number },
): PlacedLabel[] {
  const first = placeLabels(
    requests.map((r) => ({ ...r, side: r.sides[0] ?? r.side, flip: false })),
    options,
  );
  const taken: Rect[] = first.flatMap((spot, k) =>
    spot.visible
      ? [
          {
            left: spot.left,
            top: spot.top,
            right: spot.left + requests[k].width,
            bottom: spot.top + requests[k].height,
          },
        ]
      : [],
  );
  return first.map((spot, k) => {
    const r = requests[k];
    if (spot.visible || r.priority <= 0) return spot;
    for (const side of r.sides.slice(1)) {
      const [trial] = placeLabels([{ ...r, side, flip: false }], {
        ...options,
        obstacles: [...(options.obstacles ?? []), ...taken],
      });
      if (trial.visible) {
        taken.push({
          left: trial.left,
          top: trial.top,
          right: trial.left + r.width,
          bottom: trial.top + r.height,
        });
        return trial;
      }
    }
    return spot;
  });
}

/** The plot's radial chart: centred, with `rim` px outside for the tick labels round the rim. */
export function clockGeometry(
  width: number,
  height: number,
  { rim = 30, hole = 0.3, max = 190 }: { rim?: number; hole?: number; max?: number } = {},
) {
  const outer = Math.max(30, Math.min(width / 2 - rim, height / 2 - rim, max));
  return {
    cx: settle(width / 2),
    cy: settle(height / 2),
    outer: settle(outer),
    inner: settle(outer * clamp(hole, 0, 0.8)),
  };
}

/* ——— Polar area (rose) ——— */

export type PolarScale = "area" | "radius";

/**
 * A slice's outer radius. With `area` (the default, for amounts), the slice's area grows in
 * proportion to its value, so the radius grows with the square root: r = √(inner² + (outer² −
 * inner²)·v/max). With `radius` (for rates and scores, when told to), the radius itself is linear.
 * Negative and missing values sit at the inner radius.
 */
export function polarRadius(
  value: number,
  max: number,
  inner: number,
  outer: number,
  scale: PolarScale = "area",
): number {
  const share = max > 0 && finite(value) ? clamp(value / max, 0, 1) : 0;
  if (scale === "radius") return inner + share * (outer - inner);
  return Math.sqrt(inner * inner + (outer * outer - inner * inner) * share);
}

/** Equal slices round the circle from `from`, each with its middle angle. */
export function sliceAngles(
  count: number,
  from = 0,
): { start: number; end: number; mid: number }[] {
  const n = Math.max(0, count);
  return Array.from({ length: n }, (_, i) => {
    const start = from + (i / n) * TAU,
      end = from + ((i + 1) / n) * TAU;
    return { start, end, mid: (start + end) / 2 };
  });
}

/** The slice under a point, by angle, when the point is inside the slice's radius (plus `slack`)
 * or anywhere out to `reach` px from the centre. */
export function sliceAt(
  slices: readonly { start: number; end: number }[],
  cx: number,
  cy: number,
  x: number,
  y: number,
  reach: number,
): number | null {
  const r = Math.hypot(x - cx, y - cy);
  if (r > reach || !slices.length) return null;
  const a = normAngle(Math.atan2(x - cx, cy - y));
  for (let i = 0; i < slices.length; i++) {
    const { start, end } = slices[i];
    if (normAngle(a - start) < end - start) return i;
  }
  return null;
}

/** The rose's circle for a plot: centred, leaving `side` px each side for leader labels and `rim`
 * px above and below. */
export function roseGeometry(
  width: number,
  height: number,
  {
    side = 0,
    rim = 18,
    hole = 0.12,
    max = 180,
  }: {
    side?: number;
    rim?: number;
    hole?: number;
    max?: number;
  } = {},
) {
  const outer = Math.max(28, Math.min(width / 2 - side, height / 2 - rim, max));
  return {
    cx: settle(width / 2),
    cy: settle(height / 2),
    outer: settle(outer),
    inner: settle(outer * clamp(hole, 0, 0.6)),
  };
}

/** How many slices apart the rim names sit so they breathe: 1 (every slice) where they fit with
 * room to spare, otherwise the smallest step that divides the slices evenly (every 2nd, 3rd…). */
export function rimStep(count: number, radius: number, labelWidth: number): number {
  if (count <= 0) return 1;
  const arc = (TAU * Math.max(1, radius)) / count;
  // A name needs its own width and as much again of air, or a ring of names reads as a fence.
  const need = Math.max(1, Math.ceil((labelWidth * 1.8) / Math.max(1, arc)));
  for (let step = need; step <= count; step++) if (count % step === 0) return step;
  return count;
}

/**
 * The rose's tilted view. Flat, the slices close into an even ring (`inner` to `outer`, in the flat
 * plot); tilted, that ring is seen from the side, each slab standing from `low` to `high` px by its
 * value, through a camera that keeps the tallest slab in the plot (the donut's proportions).
 */
export function roseTilt(
  width: number,
  height: number,
  flat: { cx: number; cy: number; outer: number },
  {
    rows = 1,
    slabs,
  }: {
    rows?: number;
    /** The slabs as they will stand: their angles and heights as a share of the tallest (0–1). */
    slabs?: readonly { start: number; end: number; share: number }[];
  } = {},
): { inner: number; outer: number; low: number; high: number; tilt: RingTilt } {
  const inner = settle(flat.outer * 0.42);
  const outer = flat.outer;
  const share = (outer - inner) / Math.max(1, outer);
  const thick = Math.min(0.5, share * 0.78);
  // Every slab is labelled on a pin, in `rows` rows above the ring and below it; the camera frames
  // the ring between them.
  const rise = Math.min(thick * 1.7, 0.72);
  const framed = framePinned(height, {
    rise,
    slabs: slabs?.map((slab) => ({
      start: slab.start,
      end: slab.end,
      rise: Math.max(0, Math.min(1, slab.share)) * rise,
    })),
    elevation: 0.52,
    distance: 3.6,
    max: Math.min(width * 0.3, height * 0.4, 220),
    top: 34,
    rows,
  });
  const tiltOuter = settle(Math.max(40, framed.outer));
  const ringWidth = settle(tiltOuter * thick);
  const high = settle(Math.min(ringWidth * 1.7, tiltOuter * 0.72));
  const low = settle(Math.min(ringWidth * 0.12, 6));
  return {
    inner,
    outer,
    low,
    high,
    tilt: {
      radius: settle(tiltOuter - ringWidth / 2),
      width: ringWidth,
      depth: settle(ringWidth * 0.5),
      low,
      gap: 3,
      bevel: 3,
      camera: {
        tilt: 1,
        elevation: 0.52,
        distance: settle(tiltOuter * 3.6),
        anchor: { x: settle(flat.cx), y: settle(Math.min(height * 0.62, framed.y)) },
      },
    },
  };
}

/**
 * Radial bars seen from the side: the same concentric tracks, each bar a glass slab standing from
 * `low` to `high` px, through the rose's camera. The open part of the circle (the top-left quadrant
 * of a full chart, the bottom half of a semi) turns to the front (`turn`), so the camera looks into
 * the tracks and no outer slab stands in front of an inner one's start.
 */
export function radialBarsTilt(
  width: number,
  height: number,
  flat: Pick<RadialBarsGeometry, "outer" | "radii" | "thickness" | "gap" | "from" | "span">,
  {
    rows = 1,
    slabs,
  }: {
    rows?: number | { above: number; below: number };
    /** The bars as they will stand: their angles (before the turn), heights as a share of the
     * tallest (0–1) and outer edges as a share of the flat outer radius. */
    slabs?: readonly { start: number; end: number; share: number; radius: number }[];
  } = {},
): { radii: number[]; thickness: number; low: number; high: number; turn: number; tilt: RingTilt } {
  // The middle of the open part of the circle comes round to 6 o'clock, the front.
  const gapMiddle = flat.from + flat.span + (TAU - flat.span) / 2;
  const turn = settle(wrapAngle(Math.PI - gapMiddle) * 1e4) / 1e4;
  // Every slab is labelled on a pin, in a row above the ring and a row below it; the camera frames
  // the tracks between them, allowing for the tallest slab at the back.
  const most = Math.max(40, Math.min(width * 0.36, height * 0.42, 220));
  const riseAt = (outer: number) => {
    const t = (flat.thickness + flat.gap * 0.5) * (outer / Math.max(1, flat.outer));
    return Math.min(outer * 0.34, Math.max(4, t) * 1.8 + 14) / outer;
  };
  const standing = (rise: number) =>
    slabs?.map((slab) => ({
      start: slab.start + turn,
      end: slab.end + turn,
      rise: Math.max(0, Math.min(1, slab.share)) * rise,
      radius: slab.radius,
    }));
  let framed = framePinned(height, {
    rise: riseAt(most),
    slabs: standing(riseAt(most)),
    elevation: 0.9,
    distance: 3.6,
    max: most,
    rows,
  });
  const rise = riseAt(Math.max(40, framed.outer));
  framed = framePinned(height, {
    rise,
    slabs: standing(rise),
    elevation: 0.9,
    distance: 3.6,
    max: most,
    rows,
  });
  const tiltOuter = settle(Math.max(40, framed.outer));
  const scale = tiltOuter / Math.max(1, flat.outer);
  const radii = flat.radii.map((r) => settle(r * scale));
  // Tilted, a track takes half its gap: thicker slabs carry more light through their tops.
  const thickness = settle(Math.max(4, (flat.thickness + flat.gap * 0.5) * scale));
  const high = settle(Math.min(tiltOuter * 0.34, thickness * 1.8 + 14));
  const low = settle(Math.min(thickness * 0.35, 5));
  return {
    radii,
    thickness,
    low,
    high,
    turn,
    tilt: {
      radius: radii[0] ?? tiltOuter - thickness / 2,
      width: thickness,
      depth: settle(Math.max(thickness * 0.6, 6)),
      low,
      gap: 0,
      bevel: settle(Math.min(3, thickness * 0.2)),
      // Steeper than the rose's camera, so it sees into the inner tracks over the outer ones.
      camera: {
        tilt: 1,
        elevation: 0.9,
        distance: settle(tiltOuter * 3.6),
        anchor: { x: settle(width / 2), y: settle(framed.y) },
      },
    },
  };
}

/**
 * Slab heights for a tilted view, `low` to `high` px. With `max`, each value is its share of it
 * (clamped). Otherwise the values are a second measure, read from zero (or from the smallest when
 * some are below zero) to the largest. Missing values sit at the middle height.
 */
export function tiltHeights(
  values: readonly (number | null | undefined)[],
  low: number,
  high: number,
  max?: number,
): number[] {
  const middle = (low + high) / 2;
  if (max !== undefined)
    return values.map((v) =>
      finite(v) ? low + (max > 0 ? clamp(v / max, 0, 1) : 0) * (high - low) : middle,
    );
  const known = values.filter(finite);
  if (!known.length) return values.map(() => middle);
  const lo = Math.min(0, ...known),
    hi = Math.max(0, ...known);
  if (hi - lo < 1e-12) return values.map(() => middle);
  return values.map((v) => (finite(v) ? low + ((v - lo) / (hi - lo)) * (high - low) : middle));
}
