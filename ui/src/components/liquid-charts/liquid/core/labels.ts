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
 * Value labels that place themselves and never collide.
 *
 * Charts render labels as DOM elements, measure them, and call `placeLabels` every animation frame
 * with the mark positions. Labels are placed greedily in priority order: the item in focus first,
 * then the values worth reading (best, worst, latest), and a label that cannot find room is hidden
 * rather than drawn over another. Hidden labels keep a position so they can fade out in place.
 */

import { overlaps, settle, type Rect } from "./geometry";

export type LabelSide = "above" | "below" | "left" | "right" | "center";

export type LabelRequest = {
  id: string;
  /** Anchor point: the bar tip, point, or segment centroid the label belongs to. */
  x: number;
  y: number;
  /** Measured label size, px. */
  width: number;
  height: number;
  /** Where the label sits relative to the anchor. */
  side: LabelSide;
  /** Distance from the anchor to the label's near edge, px. Default 6. */
  offset?: number;
  /** Higher places first and wins collisions. Infinity for the item in focus. */
  priority: number;
  /** May the label move to the opposite side when it does not fit? Default true for above/below/left/right. */
  flip?: boolean;
};

export type PlacedLabel = {
  id: string;
  visible: boolean;
  left: number;
  top: number;
  side: LabelSide;
};

import type { LabelMode } from "./style";

export type { LabelMode };

const OPPOSITE: Record<LabelSide, LabelSide> = {
  above: "below",
  below: "above",
  left: "right",
  right: "left",
  center: "center",
};

/**
 * Writes the label's rect for `side` into `out` and reports whether it fits inside `bounds`. The
 * label slides along its free axis to stay inside (both axes for a centred label); on the side axis
 * it stays put, because moving it there would detach it from its anchor.
 */
function locate(r: LabelRequest, side: LabelSide, bounds: Rect, out: Rect): boolean {
  const offset = r.offset ?? 6;
  const { width: w, height: h } = r;
  let left = r.x - w / 2;
  let top = r.y - h / 2;
  if (side === "above") top = r.y - offset - h;
  else if (side === "below") top = r.y + offset;
  else if (side === "left") left = r.x - offset - w;
  else if (side === "right") left = r.x + offset;

  const vertical = side === "above" || side === "below";
  const horizontal = side === "left" || side === "right";
  if (!horizontal) left = Math.max(bounds.left, Math.min(left, bounds.right - w));
  if (!vertical) top = Math.max(bounds.top, Math.min(top, bounds.bottom - h));
  out.left = left;
  out.top = top;
  out.right = left + w;
  out.bottom = top + h;

  const fitsX = horizontal
    ? left >= bounds.left && left + w <= bounds.right
    : w <= bounds.right - bounds.left;
  const fitsY = vertical
    ? top >= bounds.top && top + h <= bounds.bottom
    : h <= bounds.bottom - bounds.top;
  return fitsX && fitsY;
}

function blocked(rect: Rect, placed: readonly Rect[], obstacles: readonly Rect[], gap: number) {
  for (const other of placed) if (overlaps(rect, other, gap)) return true;
  for (const other of obstacles) if (overlaps(rect, other, gap)) return true;
  return false;
}

/**
 * Places labels in descending priority (ties keep request order) and hides any that would leave
 * `bounds` or touch an already placed label or an obstacle within `gap` px, after trying the
 * opposite side once where allowed. Labels with priority ≤ 0 are hidden without being placed.
 * Returns one entry per request, in request order.
 */
export function placeLabels(
  requests: readonly LabelRequest[],
  options: { bounds: Rect; obstacles?: readonly Rect[]; gap?: number },
): PlacedLabel[] {
  const { bounds, obstacles = [], gap = 2 } = options;
  const rect: Rect = { left: 0, top: 0, right: 0, bottom: 0 };
  const result: PlacedLabel[] = new Array(requests.length);
  const order: number[] = [];

  // Every label starts hidden at its own side, which is where a hidden label fades out.
  for (let i = 0; i < requests.length; i++) {
    const r = requests[i];
    locate(r, r.side, bounds, rect);
    result[i] = {
      id: r.id,
      visible: false,
      left: settle(rect.left),
      top: settle(rect.top),
      side: r.side,
    };
    if (r.priority > 0) order.push(i);
  }
  // Array sort is stable, so equal priorities keep request order.
  order.sort((a, b) => {
    const pa = requests[a].priority;
    const pb = requests[b].priority;
    return pa === pb ? 0 : pb > pa ? 1 : -1;
  });

  const placed: Rect[] = [];
  for (const i of order) {
    const r = requests[i];
    const attempts = r.side !== "center" && r.flip !== false ? 2 : 1;
    for (let attempt = 0; attempt < attempts; attempt++) {
      const side = attempt === 0 ? r.side : OPPOSITE[r.side];
      if (!locate(r, side, bounds, rect) || blocked(rect, placed, obstacles, gap)) continue;
      placed.push({ ...rect });
      const label = result[i];
      label.visible = true;
      label.left = settle(rect.left);
      label.top = settle(rect.top);
      label.side = side;
      break;
    }
  }
  // The material scorecard's tally (see ChartFrame): labels hidden for want of room, by id.
  const tally = (globalThis as { __liquidScorecard?: { labels: Map<string, boolean> } })
    .__liquidScorecard?.labels;
  if (tally)
    for (let i = 0; i < requests.length; i++)
      if (requests[i].priority > 0) tally.set(requests[i].id, !result[i].visible);
      else tally.delete(requests[i].id);
  return result;
}

/** Measured bottom-axis names slide inside the frame before collisions are resolved. Keep edge
 * names where possible, and always give the inspected category first choice. */
export function placeCategoryNames(
  centres: readonly number[],
  widths: readonly number[],
  frameWidth: number,
  active: number | null = null,
) {
  return placeLabels(
    centres.map((x, i) => ({
      id: String(i),
      x,
      y: 0.5,
      width: widths[i] ?? 0,
      height: 1,
      side: "center" as const,
      priority: !widths[i] ? 0 : i === active ? 3 : i === 0 || i === centres.length - 1 ? 2 : 1,
    })),
    { bounds: { left: 1, right: frameWidth - 1, top: 0, bottom: 1 }, gap: 10 },
  );
}

/**
 * Priorities for a series of values: auto shows the best, worst and latest; all shows every value
 * (larger magnitude first when they collide); ends shows the first and last; none shows nothing.
 * The active index (if any) always gets Infinity. Missing (non-finite) values are never labelled
 * unless active.
 */
export function labelPriorities(
  values: readonly number[],
  mode: LabelMode,
  active?: number | null,
): number[] {
  const n = values.length;
  const priorities = new Array<number>(n).fill(0);
  if (mode === "all") {
    let peak = 0;
    for (const v of values) if (Number.isFinite(v)) peak = Math.max(peak, Math.abs(v));
    for (let i = 0; i < n; i++)
      if (Number.isFinite(values[i])) priorities[i] = 1 + Math.abs(values[i]) / (peak || 1);
  } else if (mode === "ends" && n > 0) {
    priorities[0] = 1;
    priorities[n - 1] = 1;
  } else if (mode === "auto" && n > 0) {
    // First occurrence wins ties, so a flat series has max and min on the same index.
    let max = -1;
    let min = -1;
    for (let i = 0; i < n; i++) {
      const v = values[i];
      if (!Number.isFinite(v)) continue;
      if (max < 0 || v > values[max]) max = i;
      if (min < 0 || v < values[min]) min = i;
    }
    if (max >= 0) {
      priorities[max] = 3;
      if (min !== max) priorities[min] = 2;
      if (priorities[n - 1] === 0 && Number.isFinite(values[n - 1])) priorities[n - 1] = 1;
    }
  }
  if (active != null && Number.isInteger(active) && active >= 0 && active < n)
    priorities[active] = Infinity;
  return priorities;
}

/* ——— One-axis dodge (from the line family): end labels, row labels ——— */

export type DodgeItem = {
  /** Where the label's centre wants to be, px. */
  desired: number;
  height: number;
};

/**
 * Stacks labels along one axis without overlap, each as near its desired centre as the others
 * allow, keeping their order, inside [lo, hi]. Overlapping labels merge into a cluster centred on
 * their members' mean, so a crowd spreads evenly about where it wants to be instead of being shoved
 * one way. Returns each label's top, in input order, and whether they all fit.
 */
export function dodge(
  items: readonly DodgeItem[],
  lo: number,
  hi: number,
  gap = 2,
): { tops: number[]; fits: boolean } {
  const order = items.map((_, i) => i).sort((a, b) => items[a].desired - items[b].desired || a - b);
  type Cluster = { members: number[]; top: number; height: number; offsets: number[] };
  const clusters: Cluster[] = [];
  const total =
    items.reduce((sum, item) => sum + item.height, 0) + gap * Math.max(0, items.length - 1);
  const fits = total <= hi - lo + 1e-9;
  const settleCluster = (c: Cluster) => {
    // The top that puts each member nearest its desired centre on average.
    let sum = 0;
    c.members.forEach((m, k) => (sum += items[m].desired - c.offsets[k] - items[m].height / 2));
    const ideal = sum / c.members.length;
    c.top = Math.max(lo, Math.min(ideal, hi - c.height));
    if (c.height > hi - lo) c.top = lo;
  };
  for (const i of order) {
    const c: Cluster = { members: [i], top: 0, height: items[i].height, offsets: [0] };
    settleCluster(c);
    clusters.push(c);
    while (clusters.length > 1) {
      const b = clusters[clusters.length - 1],
        a = clusters[clusters.length - 2];
      if (a.top + a.height + gap <= b.top + 1e-9) break;
      const shift = a.height + gap;
      a.members.push(...b.members);
      a.offsets.push(...b.offsets.map((o) => o + shift));
      a.height += gap + b.height;
      clusters.pop();
      settleCluster(a);
    }
  }
  const tops = new Array<number>(items.length);
  for (const c of clusters) c.members.forEach((m, k) => (tops[m] = c.top + c.offsets[k]));
  return { tops, fits };
}

/**
 * Which of a run of axis names to show when the host has already chosen them, leaving the rest
 * empty (a run of months named at its quarter starts): every named one, less any that would touch
 * the one before it. The last named one always shows (it is usually the latest), taking the place
 * of the one before if they would touch. `clear(a, b)` says whether names `a` and `b` have room.
 */
export function namedVisibility(
  names: readonly string[],
  clear: (a: number, b: number) => boolean,
): boolean[] {
  const visible = names.map(() => false);
  let last = -1;
  names.forEach((name, i) => {
    if (!name) return;
    if (last < 0 || clear(last, i)) {
      visible[i] = true;
      last = i;
    }
  });
  let end = names.length - 1;
  while (end >= 0 && !names[end]) end--;
  if (end >= 0 && !visible[end]) {
    for (let i = end - 1; i >= 0; i--)
      if (visible[i]) {
        if (!clear(i, end)) visible[i] = false;
        break;
      }
    visible[end] = true;
  }
  return visible;
}
