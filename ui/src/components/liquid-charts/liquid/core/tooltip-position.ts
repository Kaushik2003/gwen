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
 * Where a tooltip goes, as pure functions: out past a ring along a segment's angle, or round a thin
 * row (above, below, then beside). `placeTooltip` in tooltip.tsx applies them. (From the ring and
 * bar families.)
 */

import { clamp, settle, type Rect } from "./geometry";
import { polar } from "./arc";

/**
 * Where a tooltip of `size` goes for the segment at `angle`: out past the ring along that angle,
 * its nearest corner towards the ring, and inside `bounds`. If the bounds push it back over the
 * ring, the side positions (right, left, above, below the ring) are tried and the one that covers
 * least of the ring, then lies nearest the segment, wins.
 */
export function outwardTooltipPosition(
  size: { width: number; height: number },
  ring: { cx: number; cy: number; outer: number },
  angle: number,
  bounds: Rect,
  gap = 12,
): { left: number; top: number } {
  const { width: w, height: h } = size;
  const { cx, cy, outer } = ring;
  const at = polar(cx, cy, outer + gap, angle);
  const dx = Math.sin(angle),
    dy = -Math.cos(angle);
  const place = (left: number, top: number) => ({
    left: clamp(left, bounds.left, Math.max(bounds.left, bounds.right - w)),
    top: clamp(top, bounds.top, Math.max(bounds.top, bounds.bottom - h)),
  });
  const along = (d: number, extent: number) => (d > 0.38 ? 0 : d < -0.38 ? -extent : -extent / 2);
  const candidates = [
    place(at.x + along(dx, w), at.y + along(dy, h)),
    place(cx + outer + gap, at.y - h / 2),
    place(cx - outer - gap - w, at.y - h / 2),
    place(at.x - w / 2, cy - outer - gap - h),
    place(at.x - w / 2, cy + outer + gap),
  ];
  const cover = (c: { left: number; top: number }) => {
    const nx = clamp(cx, c.left, c.left + w),
      ny = clamp(cy, c.top, c.top + h);
    return Math.max(0, outer + 2 - Math.hypot(nx - cx, ny - cy));
  };
  const distance = (c: { left: number; top: number }) =>
    Math.hypot(clamp(at.x, c.left, c.left + w) - at.x, clamp(at.y, c.top, c.top + h) - at.y);
  let best = candidates[0],
    score = Infinity;
  candidates.forEach((candidate, i) => {
    // Grazing the ring's rim by a few px is fine; covering its glass is not.
    const depth = cover(candidate);
    const value = Math.max(0, depth - 4) * 100 + depth * 4 + distance(candidate) + i * 0.01;
    if (value < score) {
      score = value;
      best = candidate;
    }
  });
  return { left: settle(best.left), top: settle(best.top) };
}

/**
 * Where a tooltip of `size` can sit beside a ring without covering any of it: out past the segment
 * at `angle` when there is room, else the spot in the free space round the ring nearest that
 * segment's outer edge. Null when nothing inside `bounds` clears the ring within `reach` px of the
 * segment (a ring that fills its card): the ring's centre reading then carries the focus. `ring`
 * should include anything popped out of it.
 */
export function clearRingTooltipPosition(
  size: { width: number; height: number },
  ring: { cx: number; cy: number; outer: number },
  angle: number,
  bounds: Rect,
  { gap = 12, reach = Infinity }: { gap?: number; reach?: number } = {},
): { left: number; top: number } | null {
  const { width: w, height: h } = size;
  const clear = { ...ring, outer: ring.outer + 2 };
  const first = outwardTooltipPosition(size, ring, angle, bounds, gap);
  if (!coversRing(first, size, clear)) return first;
  // Search the free space on a 4px grid (and flush with each edge) for the box nearest the
  // segment's outer edge that clears the ring.
  const at = polar(ring.cx, ring.cy, ring.outer + gap, angle);
  const steps = (lo: number, hi: number) => {
    const out: number[] = [];
    for (let v = lo; v < hi; v += 4) out.push(v);
    out.push(hi);
    return out;
  };
  let best: { left: number; top: number } | null = null,
    nearest = reach;
  for (const top of steps(bounds.top, bounds.bottom - h))
    for (const left of steps(bounds.left, bounds.right - w)) {
      if (coversRing({ left, top }, size, clear)) continue;
      const d = Math.hypot(clamp(at.x, left, left + w) - at.x, clamp(at.y, top, top + h) - at.y);
      if (d < nearest) {
        nearest = d;
        best = { left, top };
      }
    }
  return best && { left: settle(best.left), top: settle(best.top) };
}

/** True when a tooltip placed at `position` covers any of the ring's band (an annulus). */
export function coversRing(
  position: { left: number; top: number },
  size: { width: number; height: number },
  ring: { cx: number; cy: number; outer: number },
) {
  const nx = clamp(ring.cx, position.left, position.left + size.width),
    ny = clamp(ring.cy, position.top, position.top + size.height);
  return Math.hypot(nx - ring.cx, ny - ring.cy) < ring.outer;
}

/**
 * Where a tooltip goes for a row. Rows are thin and stacked close, so the card goes above
 * the row, else below it, else beside it (past either end), and when none fits (a tall card on a
 * short chart) on the roomier side, pinned to its edge, which covers the least of the row.
 * Returns the card's top-left in the same px as `anchor` and `bounds`.
 */
export function rowTooltipPosition(
  anchor: Rect,
  size: { width: number; height: number },
  bounds: Rect,
  gap = 12,
): { x: number; y: number } {
  const { width: w, height: h } = size;
  const centreY = (anchor.top + anchor.bottom) / 2 - h / 2;
  let x = (anchor.left + anchor.right) / 2 - w / 2;
  let y: number;
  if (anchor.top - gap - h >= bounds.top) y = anchor.top - gap - h;
  else if (anchor.bottom + gap + h <= bounds.bottom) y = anchor.bottom + gap;
  else if (anchor.right + gap + w <= bounds.right) {
    x = anchor.right + gap;
    y = centreY;
  } else if (anchor.left - gap - w >= bounds.left) {
    x = anchor.left - gap - w;
    y = centreY;
  } else
    y = anchor.top - bounds.top > bounds.bottom - anchor.bottom ? bounds.top : bounds.bottom - h;
  return {
    x: Math.max(bounds.left, Math.min(bounds.right - w, x)),
    y: Math.max(bounds.top, Math.min(bounds.bottom - h, y)),
  };
}
