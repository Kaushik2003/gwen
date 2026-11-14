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
import { clamp } from "@/components/liquid-charts/liquid/core/geometry";
/**
 * Range maths for brushing, zooming and panning: pure functions on spans (a `[from, to]` pair in
 * domain units: data indices, band edges, epoch ms or plain values), shared by the core brush and
 * every chart that zooms. No DOM and no React, so all of it is unit-tested.
 *
 * - Spans: `clampSpan` (inside an extent, at least a minimum width), `zoomSpan` (about a point, so
 *   the value under the pointer stays put), `panSpan`, `resetSpan`, `isZoomed`.
 * - Input: `wheelScale` (mouse wheel and trackpad pinch to a zoom factor), `valueAt` / `pxOf`.
 * - Snapping: `snapValue`, `snapSpan` (to whole bands or points, or to the nearest data x).
 * - The brush: `brushHit` (which part a press lands on), `dragSpan` (a drag from a press),
 *   `keySpan` (the brush's keyboard), `brushLabels` (where its bound readouts sit).
 * - Motion: `springValues`, one step of the critically damped spring domains ride.
 */

export type Span = readonly [number, number];

/** The span with its ends in order (from ≤ to). */
export function ordered(span: Span): [number, number] {
  return span[0] <= span[1] ? [span[0], span[1]] : [span[1], span[0]];
}

/**
 * The span kept inside `extent` and at least `minSpan` wide (never wider than the extent). A span
 * that sticks out slides back in and keeps its width; one narrower than the minimum grows about
 * its centre. Non-finite input returns the extent.
 */
export function clampSpan(span: Span, extent: Span, minSpan = 0): [number, number] {
  const [e0, e1] = ordered(extent);
  if (!Number.isFinite(span[0]) || !Number.isFinite(span[1])) return [e0, e1];
  const full = e1 - e0;
  let [a, b] = ordered(span);
  const least = Math.min(Math.max(0, minSpan), full);
  if (b - a < least) {
    const centre = (a + b) / 2;
    a = centre - least / 2;
    b = centre + least / 2;
  }
  if (b - a >= full) return [e0, e1];
  if (a < e0) {
    b += e0 - a;
    a = e0;
  }
  if (b > e1) {
    a -= b - e1;
    b = e1;
  }
  return [a, b];
}

/**
 * Zooms about `anchor` (a value, usually the one under the pointer): the width scales by `scale`
 * (below 1 zooms in, above 1 out) and the anchor keeps its place on screen, then the span is
 * clamped to the extent and the minimum width.
 */
export function zoomSpan(
  span: Span,
  scale: number,
  anchor: number,
  extent: Span,
  minSpan = 0,
): [number, number] {
  const [a, b] = ordered(span);
  const [e0, e1] = ordered(extent);
  const width = b - a;
  const full = e1 - e0;
  if (!(scale > 0) || !Number.isFinite(scale)) return clampSpan([a, b], extent, minSpan);
  const next = clamp(width * scale, Math.min(Math.max(0, minSpan), full), full);
  // An anchor outside the span counts as its nearest edge.
  const t = width > 0 && Number.isFinite(anchor) ? clamp((anchor - a) / width, 0, 1) : 0.5;
  const from = a + t * width - t * next;
  return clampSpan([from, from + next], extent, minSpan);
}

/** The span moved by `delta`, kept inside the extent with its width. */
export function panSpan(span: Span, delta: number, extent: Span): [number, number] {
  const [a, b] = ordered(span);
  const d = Number.isFinite(delta) ? delta : 0;
  return clampSpan([a + d, b + d], extent);
}

/** The whole extent: what a reset returns to. */
export const resetSpan = (extent: Span): [number, number] => ordered(extent);

/** True when the span shows less than the whole extent (beyond a relative `tolerance`). */
export function isZoomed(span: Span | null | undefined, extent: Span, tolerance = 1e-6): boolean {
  if (!span) return false;
  const [a, b] = ordered(span);
  const [e0, e1] = ordered(extent);
  const slack = (e1 - e0) * tolerance;
  return a > e0 + slack || b < e1 - slack;
}

/**
 * A wheel event as a zoom factor (below 1 zooms in). Trackpad pinches arrive as wheel events with
 * `ctrlKey` and small pixel deltas, mouse wheels as larger pixel or line steps; both feel even, and
 * one event never zooms by more than e (about 2.7×).
 */
export function wheelScale({
  deltaY,
  deltaMode = 0,
  ctrlKey = false,
}: {
  deltaY: number;
  deltaMode?: number;
  ctrlKey?: boolean;
}): number {
  if (!Number.isFinite(deltaY) || deltaY === 0) return 1;
  const px = deltaMode === 1 ? deltaY * 16 : deltaMode === 2 ? deltaY * 400 : deltaY;
  return Math.exp(clamp(px * (ctrlKey ? 0.01 : 0.0024), -1, 1));
}

/** The value at `px` along a plot axis running from `p0` to `p1` px over `span`. */
export function valueAt(px: number, [p0, p1]: Span, span: Span): number {
  return p1 === p0
    ? (span[0] + span[1]) / 2
    : span[0] + ((px - p0) / (p1 - p0)) * (span[1] - span[0]);
}

/** The px of `value` along a plot axis running from `p0` to `p1` px over `span`. */
export function pxOf(value: number, [p0, p1]: Span, span: Span): number {
  return span[1] === span[0]
    ? (p0 + p1) / 2
    : p0 + ((value - span[0]) / (span[1] - span[0])) * (p1 - p0);
}

/* ——— Snapping ——— */

/** How a brush snaps: `integer` to whole bands or point indices, a sorted list of values to the
 * nearest data x, a function of your own, or nothing. */
export type Snap = "integer" | readonly number[] | ((value: number) => number) | null | undefined;

/** A value snapped `nearest` (default), `up` (the smallest snap point at least it) or `down`. */
export function snapValue(
  value: number,
  snap: Snap,
  mode: "nearest" | "up" | "down" = "nearest",
): number {
  if (!snap || !Number.isFinite(value)) return value;
  if (snap === "integer")
    return mode === "up"
      ? Math.ceil(value - 1e-9)
      : mode === "down"
        ? Math.floor(value + 1e-9)
        : Math.round(value);
  if (typeof snap === "function") return snap(value);
  if (!snap.length) return value;
  // Binary search for the first snap point at or above the value.
  let lo = 0,
    hi = snap.length;
  while (lo < hi) {
    const mid = (lo + hi) >> 1;
    if (snap[mid] < value) lo = mid + 1;
    else hi = mid;
  }
  const above = snap[Math.min(lo, snap.length - 1)];
  const below = snap[Math.max(0, lo - 1)];
  if (mode === "up") return lo >= snap.length ? snap[snap.length - 1] : above;
  if (mode === "down") return above === value ? above : below;
  return Math.abs(value - below) <= Math.abs(above - value) ? below : above;
}

/**
 * A span with both ends snapped, inside the extent and at least `minSpan` wide: when snapping
 * collapses it, the far end steps out to the next snap point that clears the minimum (or the near
 * end steps back, against the extent's end).
 */
export function snapSpan(span: Span, snap: Snap, extent: Span, minSpan = 0): [number, number] {
  const [e0, e1] = ordered(extent);
  const [c0, c1] = clampSpan(span, extent, minSpan);
  if (!snap) return [c0, c1];
  let a = clamp(snapValue(c0, snap), e0, e1);
  let b = clamp(snapValue(c1, snap), e0, e1);
  const least = Math.min(minSpan, e1 - e0);
  if (b - a < least - 1e-9) {
    const grown = snapValue(a + least, snap, "up");
    if (grown <= e1 + 1e-9 && grown - a >= least - 1e-9) b = grown;
    else {
      b = clamp(snapValue(e1, snap, "down"), e0, e1);
      a = clamp(snapValue(b - least, snap, "down"), e0, e1);
    }
  }
  return [a, b];
}

/* ——— The brush ——— */

/** Which part of a brush a press lands on. */
export type BrushGrip = "from" | "to" | "inside" | "outside";

/**
 * The part of the brush under `px`: a handle within `reach` px of a window edge (the nearer one
 * when the window is narrow), the window itself, or outside it. `window` is the window's edges in
 * px, or null when nothing is selected.
 */
export function brushHit(px: number, window: Span | null, reach = 12): BrushGrip {
  if (!window) return "outside";
  const [w0, w1] = ordered(window);
  const d0 = Math.abs(px - w0),
    d1 = Math.abs(px - w1);
  const width = w1 - w0;
  // A narrow window keeps the middle half of itself for moving, so it can still be dragged.
  if (width < reach * 3 && px > w0 + width / 4 && px < w1 - width / 4) return "inside";
  if (Math.min(d0, d1) <= reach) return d0 <= d1 ? "from" : "to";
  return px > w0 && px < w1 ? "inside" : "outside";
}

/** A drag in progress: the part pressed, the value under the press, and the range then. */
export type BrushDrag = { grip: BrushGrip; origin: number; start: Span | null };

/**
 * The range a drag makes at `value`: from outside a new window between the press and the pointer;
 * inside, the window moved by the distance dragged; on a handle, that edge moved (dragged past the
 * other, the two swap). Snapped, inside the extent and at least `minSpan` wide.
 */
export function dragSpan(
  drag: BrushDrag,
  value: number,
  extent: Span,
  { minSpan = 0, snap }: { minSpan?: number; snap?: Snap } = {},
): [number, number] {
  const [e0, e1] = ordered(extent);
  const v = clamp(value, e0, e1);
  const start = drag.start ? ordered(drag.start) : ([e0, e1] as [number, number]);
  if (drag.grip === "outside") {
    // A new window grows from the press towards the pointer, at least the minimum wide.
    const least = Math.min(minSpan, e1 - e0);
    const o = clamp(drag.origin, e0, e1);
    const span: Span =
      Math.abs(v - o) >= least
        ? [Math.min(o, v), Math.max(o, v)]
        : v < o
          ? [o - least, o]
          : [o, o + least];
    return snapSpan(span, snap, extent, minSpan);
  }
  if (drag.grip === "inside") {
    const width = start[1] - start[0];
    let a = clamp(start[0] + (value - drag.origin), e0, e1 - width);
    if (snap) a = clamp(snapValue(a, snap), e0, e1 - width);
    const b = snap && typeof snap !== "string" ? snapValue(a + width, snap) : a + width;
    return snapSpan([a, b], snap, extent, minSpan);
  }
  const fixed = drag.grip === "from" ? start[1] : start[0];
  const least = Math.min(minSpan, e1 - e0);
  // The moving edge keeps the minimum away from the fixed one, on whichever side it is.
  if (v >= fixed) {
    const b = Math.max(v, fixed + least);
    return snapSpan(b > e1 ? [e1 - least, e1] : [fixed, b], snap, extent, minSpan);
  }
  const a = Math.min(v, fixed - least);
  return snapSpan(a < e0 ? [e0, e0 + least] : [a, fixed], snap, extent, minSpan);
}

/**
 * The brush's keyboard: Left and Right move the window by `step`, Shift with them moves its right
 * edge (narrower or wider), PageUp and PageDown move it by its own width, Home and End take it to
 * the ends, `+` and `-` narrow or widen it about its centre, and Escape clears it (null). Returns
 * undefined for keys the brush doesn't use, and for moves that need a window when there is none.
 */
export function keySpan(
  range: Span | null,
  key: string,
  {
    shift = false,
    extent,
    step,
    minSpan = 0,
    snap,
  }: { shift?: boolean; extent: Span; step: number; minSpan?: number; snap?: Snap },
): [number, number] | null | undefined {
  const [e0, e1] = ordered(extent);
  if (key === "Escape") return range ? null : undefined;
  const current = range ? ordered(range) : null;
  const width = current ? current[1] - current[0] : e1 - e0;
  const done = (span: Span) => snapSpan(span, snap, extent, minSpan);
  switch (key) {
    case "ArrowLeft":
    case "ArrowRight": {
      const sign = key === "ArrowRight" ? 1 : -1;
      if (shift) {
        const [a, b] = current ?? [e0, e1];
        const next = clamp(b + sign * step, a + Math.min(minSpan, e1 - e0), e1);
        return done([a, next]);
      }
      if (!current) return undefined;
      return done(panSpan(current, sign * step, extent));
    }
    case "PageUp":
    case "PageDown":
      if (!current) return undefined;
      return done(panSpan(current, (key === "PageDown" ? 1 : -1) * width, extent));
    case "Home":
      return current ? done([e0, e0 + width]) : undefined;
    case "End":
      return current ? done([e1 - width, e1]) : undefined;
    case "+":
    case "=":
    case "-":
    case "_": {
      const [a, b] = current ?? [e0, e1];
      const scale = key === "+" || key === "=" ? 0.8 : 1.25;
      return done(zoomSpan([a, b], scale, (a + b) / 2, extent, minSpan));
    }
    default:
      return undefined;
  }
}

/** Where a brush's two bound readouts sit, px from the strip's left: each outside its handle
 * where it fits (from on the left, to on the right), inside the window where it doesn't, and one
 * merged readout centred over the window when the two inside would touch. */
export type BrushLabelLayout =
  | { merged: false; from: number; to: number; fromInside: boolean; toInside: boolean }
  | { merged: true; left: number };

/**
 * Lays out the bound readouts for a window from `from` to `to` px. `widths` are the two readouts'
 * widths and `merged` the width of the single "from – to" readout; `bounds` the px they must stay
 * within; `gap` the air between a readout and its handle.
 */
export function brushLabels({
  from,
  to,
  widths: [fromWidth, toWidth],
  merged,
  bounds: [left, right],
  gap = 8,
}: {
  from: number;
  to: number;
  widths: readonly [number, number];
  merged: number;
  bounds: Span;
  gap?: number;
}): BrushLabelLayout {
  const [a, b] = ordered([from, to]);
  const fromInside = a - gap - fromWidth < left;
  const toInside = b + gap + toWidth > right;
  const fromLeft = fromInside ? a + gap : a - gap - fromWidth;
  const toLeft = toInside ? b - gap - toWidth : b + gap;
  const fromRight = fromLeft + fromWidth;
  // Two readouts that would touch (or cross a handle) become one, centred over the window.
  if (
    fromRight + 6 > toLeft ||
    (fromInside && fromRight > b - gap) ||
    (toInside && toLeft < a + gap)
  )
    return {
      merged: true,
      left: clamp((a + b) / 2 - merged / 2, left, Math.max(left, right - merged)),
    };
  return { merged: false, from: fromLeft, to: toLeft, fromInside, toInside };
}

/* ——— Motion ——— */

/** A set of numbers riding springs (a domain's ends, say), with their velocities. */
export type SpringState = { value: number[]; velocity: number[] };

/**
 * One step of a critically damped spring for every number towards `target` (stiffness 170,
 * damping 26 by default: about a third of a second, no overshoot to speak of). Each number comes
 * to rest within `rest[i]` (default a ten-thousandth of its distance at the start, at least 1e-6)
 * and snaps there. Returns true while any number still moves. A state of a different length snaps.
 */
export function springValues(
  state: SpringState,
  target: readonly number[],
  dt: number,
  {
    stiffness = 170,
    damping = 26,
    rest,
  }: { stiffness?: number; damping?: number; rest?: readonly number[] } = {},
): boolean {
  if (state.value.length !== target.length) {
    state.value = [...target];
    state.velocity = target.map(() => 0);
    return false;
  }
  const h = clamp(dt, 0, 1 / 20) / 4;
  let moving = false;
  for (let i = 0; i < target.length; i++) {
    let v = state.value[i],
      velocity = state.velocity[i];
    const goal = target[i];
    if (!Number.isFinite(goal) || !Number.isFinite(v)) {
      state.value[i] = goal;
      state.velocity[i] = 0;
      continue;
    }
    for (let k = 0; k < 4; k++) {
      velocity += (stiffness * (goal - v) - damping * velocity) * h;
      v += velocity * h;
    }
    const tolerance = rest?.[i] ?? 1e-6;
    if (Math.abs(goal - v) <= tolerance && Math.abs(velocity) <= tolerance * 20) {
      v = goal;
      velocity = 0;
    } else moving = true;
    state.value[i] = v;
    state.velocity[i] = velocity;
  }
  return moving;
}
