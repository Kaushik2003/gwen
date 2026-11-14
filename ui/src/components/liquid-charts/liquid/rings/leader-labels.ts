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
 * Labels that sit outside their marks and point back with leader lines: round the outside of a
 * ring, in two columns that never cross. Pure layout, unit-tested through the ring family. (From
 * the ring family.)
 */

import { clamp, overlaps, settle, type Rect } from "@/components/liquid-charts/liquid/core/geometry";
import { normAngle, polar, type Point } from "@/components/liquid-charts/liquid/core/arc";

export type OutsideLabelRequest = {
  id: string;
  /** The segment's middle angle. */
  angle: number;
  width: number;
  height: number;
  /** Higher keeps its label when a side runs out of room. ≤ 0 is never shown. */
  priority: number;
};
export type OutsideLabel = {
  id: string;
  visible: boolean;
  side: "left" | "right";
  /** The label box's top-left, in plot px. */
  left: number;
  top: number;
  /** The leader: from the ring's edge, to the elbow, to the label. */
  points: [Point, Point, Point];
};

type Side = {
  index: number;
  desired: number;
  height: number;
  top: number;
};

/** Stacks boxes in their given order without overlap, each as near its desired centre as the
 * others allow, inside [lo, hi]. Boxes above `centre` give way upwards and those below give way
 * downwards, so leaders lean away from the ring rather than across it. False when they don't fit. */
function stack(items: Side[], lo: number, hi: number, gap: number, centre: number): boolean {
  if (!items.length) return true;
  const need = items.reduce((sum, item) => sum + item.height, 0) + gap * (items.length - 1);
  if (need > hi - lo) return false;
  items.forEach((item) => (item.top = item.desired - item.height / 2));
  // Settle collisions outward from the equator: the lower group pushes down, the upper pushes up.
  const split = items.findIndex((item) => item.desired >= centre);
  const pivot = split < 0 ? items.length : split;
  for (let i = pivot + 1; i < items.length; i++)
    items[i].top = Math.max(items[i].top, items[i - 1].top + items[i - 1].height + gap);
  for (let i = pivot - 2; i >= 0; i--)
    items[i].top = Math.min(items[i].top, items[i + 1].top - gap - items[i].height);
  if (pivot > 0 && pivot < items.length) {
    const above = items[pivot - 1],
      below = items[pivot];
    const overlap = above.top + above.height + gap - below.top;
    if (overlap > 0) {
      // Split the difference across the equator, then let each group give way again.
      above.top -= overlap / 2;
      below.top += overlap / 2;
      for (let i = pivot + 1; i < items.length; i++)
        items[i].top = Math.max(items[i].top, items[i - 1].top + items[i - 1].height + gap);
      for (let i = pivot - 2; i >= 0; i--)
        items[i].top = Math.min(items[i].top, items[i + 1].top - gap - items[i].height);
    }
  }
  // Inside the bounds: push down from the top, then up from the bottom.
  for (let i = 0; i < items.length; i++) {
    const floor = i === 0 ? lo : items[i - 1].top + items[i - 1].height + gap;
    items[i].top = Math.max(items[i].top, floor);
  }
  for (let i = items.length - 1; i >= 0; i--) {
    const ceiling = i === items.length - 1 ? hi : items[i + 1].top - gap;
    items[i].top = Math.min(items[i].top, ceiling - items[i].height);
  }
  return items[0].top >= lo - 0.01;
}

/** True when segments ab and cd cross (touching at an end does not count). */
export function segmentsCross(a: Point, b: Point, c: Point, d: Point): boolean {
  const cross = (o: Point, p: Point, q: Point) =>
    (p.x - o.x) * (q.y - o.y) - (p.y - o.y) * (q.x - o.x);
  const d1 = cross(c, d, a),
    d2 = cross(c, d, b),
    d3 = cross(a, b, c),
    d4 = cross(a, b, d);
  return d1 * d2 < -1e-9 && d3 * d4 < -1e-9;
}

/** The closest a segment comes to (cx, cy). */
function nearest(a: Point, b: Point, cx: number, cy: number) {
  const dx = b.x - a.x,
    dy = b.y - a.y;
  const length = dx * dx + dy * dy;
  const t = length ? clamp(((cx - a.x) * dx + (cy - a.y) * dy) / length, 0, 1) : 0;
  return Math.hypot(a.x + t * dx - cx, a.y + t * dy - cy);
}

/** True when two leader polylines cross, or a leader cuts back into the ring. */
export function leadersCross(a: readonly Point[], b: readonly Point[]): boolean {
  for (let i = 0; i < a.length - 1; i++)
    for (let j = 0; j < b.length - 1; j++)
      if (segmentsCross(a[i], a[i + 1], b[j], b[j + 1])) return true;
  return false;
}

/**
 * Labels outside a ring, in a column on each side, each joined to its segment by a leader: out from
 * the ring's edge to an elbow, then level to the label. Labels keep their segments' order down each
 * side, so leaders never cross; where a side runs out of room the lowest-priority labels are hidden.
 * Any leader that would still cross another or cut back into the ring hides its label too.
 */
export function layoutOutsideLabels(
  requests: readonly OutsideLabelRequest[],
  {
    cx,
    cy,
    radius,
    bounds,
    offset = 16,
    reach = 12,
    gap = 4,
  }: {
    cx: number;
    cy: number;
    /** The ring's outer edge. */
    radius: number;
    bounds: Rect;
    /** Px from the ring's edge out to the elbows' circle. */
    offset?: number;
    /** Px of level line between the elbows' column and the labels. */
    reach?: number;
    gap?: number;
  },
): OutsideLabel[] {
  const r1 = radius + 2,
    r2 = radius + offset;
  const result: OutsideLabel[] = requests.map((request) => {
    const side = Math.sin(request.angle) >= -1e-9 ? "right" : "left";
    const anchor = polar(cx, cy, r1, request.angle);
    return {
      id: request.id,
      visible: false,
      side,
      left: anchor.x,
      top: anchor.y - request.height / 2,
      points: [anchor, anchor, anchor],
    };
  });
  const hidden = new Set<number>();
  requests.forEach((request, i) => request.priority <= 0 && hidden.add(i));

  const solve = () => {
    const visible = new Set<number>();
    for (const side of ["left", "right"] as const) {
      const sign = side === "right" ? 1 : -1;
      // The label column: past the elbows, and inside the bounds.
      const column = cx + sign * (r2 + reach);
      const indices = requests
        .map((_, i) => i)
        .filter((i) => result[i].side === side && !hidden.has(i));
      // Add labels by priority, keeping each only if the side still stacks.
      const byPriority = [...indices].sort(
        (a, b) => requests[b].priority - requests[a].priority || a - b,
      );
      let kept: Side[] = [];
      for (const i of byPriority) {
        const request = requests[i];
        const fits =
          side === "right"
            ? column + request.width <= bounds.right + 0.01
            : column - request.width >= bounds.left - 0.01;
        if (!fits) continue;
        const desired = polar(cx, cy, r2, request.angle).y;
        const trial = [...kept, { index: i, desired, height: request.height, top: 0 }];
        // Down each side, labels keep their segments' order: clockwise on the right, the
        // reverse on the left, which is top to bottom either way.
        trial.sort((a, b) =>
          side === "right"
            ? normAngle(requests[a.index].angle) - normAngle(requests[b.index].angle)
            : normAngle(requests[b.index].angle) - normAngle(requests[a.index].angle),
        );
        const lo = Math.max(bounds.top, cy - r2 - offset),
          hi = Math.min(bounds.bottom, cy + r2 + offset);
        if (stack(trial, lo, hi, gap, cy)) kept = trial;
      }
      stack(
        kept,
        Math.max(bounds.top, cy - r2 - offset),
        Math.min(bounds.bottom, cy + r2 + offset),
        gap,
        cy,
      );
      for (const item of kept) {
        const request = requests[item.index];
        const label = result[item.index];
        const middle = item.top + item.height / 2;
        const anchor = polar(cx, cy, r1, request.angle);
        const dy = middle - cy;
        // The elbow sits on the elbows' circle at the label's height, and never inside the
        // anchor's own reach, so the first leg always leans outward.
        const out = Math.max(
          Math.sqrt(Math.max(0, r2 * r2 - dy * dy)),
          Math.abs(anchor.x - cx) + 6,
        );
        const elbow = { x: cx + sign * Math.min(out, r2 + reach - 2), y: middle };
        const end = { x: column - sign * 2, y: middle };
        label.points = [
          { x: settle(anchor.x), y: settle(anchor.y) },
          { x: settle(elbow.x), y: settle(elbow.y) },
          { x: settle(end.x), y: settle(end.y) },
        ];
        label.left = settle(side === "right" ? column : column - request.width);
        label.top = settle(item.top);
        visible.add(item.index);
      }
    }
    return visible;
  };

  // Hide the weaker label of any pair whose leaders still cross, or any leader that dips into the
  // ring, and lay out again. Each pass hides one label, so this ends.
  for (let pass = 0; pass <= requests.length; pass++) {
    const visible = [...solve()];
    let worst = -1;
    for (const i of visible) {
      const [a, b] = result[i].points;
      if (nearest(a, b, cx, cy) < radius - 0.5) {
        worst = i;
        break;
      }
    }
    for (let x = 0; worst < 0 && x < visible.length; x++)
      for (let y = x + 1; y < visible.length; y++) {
        const i = visible[x],
          j = visible[y];
        if (leadersCross(result[i].points, result[j].points)) {
          worst = requests[i].priority < requests[j].priority ? i : j;
          break;
        }
      }
    if (worst < 0) {
      visible.forEach((i) => (result[i].visible = true));
      break;
    }
    hidden.add(worst);
  }
  return result;
}

/** Label rects that overlap `rect`, by index; used to fade labels a tooltip sits over. */
export const underRect = (rects: readonly Rect[], rect: Rect, gap = 2) =>
  rects.map((other) => overlaps(other, rect, gap));
