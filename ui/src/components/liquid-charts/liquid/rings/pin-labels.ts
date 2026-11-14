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
 * Labels on pins, for tilted rings. Seen from the side, slabs of different heights hide each other
 * and angles can't be read, so every slab names itself on a stem: a back slab's label stands in a
 * row above the ring, its stem rising straight up from the slab's top; a front slab's hangs in a row
 * below it, its stem dropping from the front face's lower edge. A stem never crosses the ring.
 *
 * Stems are vertical. Where labels in a row would collide they first slide along their slab (the
 * stem moves with them and stays straight), then stagger into a second row, and only then take one
 * square elbow (up, across, up) with the run on a shelf just under the row. What still doesn't fit
 * is dropped, lowest priority first, so a narrow card keeps the slab in focus and the largest few.
 * Pure layout over screen px; the charts project the anchors through the tilt camera every frame.
 */

import { clamp, settle, type Rect } from "@/components/liquid-charts/liquid/core/geometry";
import type { Point } from "@/components/liquid-charts/liquid/core/arc";
import { dodge } from "@/components/liquid-charts/liquid/core/labels";
import { segmentsCross } from "./leader-labels";

/** Which way a pin's stem runs from its slab: up to a row above the ring, or down to one below. */
export type PinSide = "up" | "down";

export type PinRequest = {
  id: string;
  side: PinSide;
  /** The stem's preferred foot, plot px: a back slab's highest visible point (its top face's far
   * edge, at its angular middle) or a front slab's lowest (its front face's lower edge). */
  x: number;
  y: number;
  /** More places the stem may stand along the same edge, so a label that has to slide keeps a
   * straight stem. Any order; the foot follows the edge between them. */
  ridge?: readonly Point[];
  /** Label size, px. */
  width: number;
  height: number;
  /** Higher keeps its label when a row runs out of room; Infinity for the slab in focus. ≤ 0 is
   * never shown. */
  priority: number;
};

export type PinLabel = {
  id: string;
  visible: boolean;
  side: PinSide;
  /** 0 is the row nearest the ring, 1 the staggered row beyond it. */
  row: number;
  /** The label box's top-left, plot px. */
  left: number;
  top: number;
  /** The stem, from its foot on the slab to a few px short of the label: two points when straight,
   * four with an elbow (along, across, along). Every segment is vertical or horizontal. */
  points: Point[];
};

export type PinOptions = {
  /** Where labels may sit, plot px (leave room for anything along the edges). */
  bounds: Rect;
  /** The ring's highest point on screen (smallest y): the upper rows end above it. */
  ceiling: number;
  /** The ring's lowest point on screen (largest y): the lower rows start below it. */
  floor: number;
  /** Px between labels in a row. */
  gap?: number;
  /** Px between the two rows on a side. */
  rowGap?: number;
  /** Px between the ring and the nearest row. */
  lift?: number;
  /** Px a stem stops short of its label. */
  clearance?: number;
  /** Rows each side may use (1 or 2). */
  rows?: number;
  /** Show at most this many labels, the highest priorities, besides the one in focus (priority
   * Infinity), which is always one more, so the rest stay put while the focus moves. */
  limit?: number;
  /** Px a stem keeps inside its label's ends, and clear of another row's label. */
  inset?: number;
};

type Item = {
  index: number;
  request: PinRequest;
  /** Preferred foot x, and the range the foot may slide over. */
  p: number;
  a: number;
  b: number;
  ridge: Point[];
};

type Placed = { left: number; row: number; points: Point[]; elbow: boolean; foot: number };

/** The foot's y where the edge passes `x` (linear between ridge points, flat beyond them). */
function footY(item: Item, x: number) {
  const ridge = item.ridge;
  if (ridge.length < 2) return item.request.y;
  if (x <= ridge[0].x) return ridge[0].y;
  for (let k = 1; k < ridge.length; k++) {
    const q = ridge[k - 1],
      r = ridge[k];
    if (x <= r.x) return r.x - q.x < 1e-9 ? r.y : q.y + ((x - q.x) / (r.x - q.x)) * (r.y - q.y);
  }
  return ridge[ridge.length - 1].y;
}

/** True when an axis-aligned segment passes through the box (grown by `pad`). */
function segmentHitsBox(p: Point, q: Point, box: Rect, pad: number) {
  const left = Math.min(p.x, q.x),
    right = Math.max(p.x, q.x),
    top = Math.min(p.y, q.y),
    bottom = Math.max(p.y, q.y);
  return (
    right > box.left - pad &&
    left < box.right + pad &&
    bottom > box.top - pad &&
    top < box.bottom + pad
  );
}

/**
 * Lays out pins for a tilted ring: one entry per request, in request order. Each side is solved on
 * its own: a single row with every stem straight if it can be; otherwise the cheapest of a few
 * two-row staggers and elbowed single rows that keep every stem clear of every other stem and label;
 * otherwise the lowest-priority label goes and it tries again.
 */
export function layoutPinLabels(
  requests: readonly PinRequest[],
  {
    bounds,
    ceiling,
    floor,
    gap = 12,
    rowGap = 6,
    lift = 16,
    clearance = 4,
    rows = 2,
    limit = Infinity,
    inset = 6,
  }: PinOptions,
): PinLabel[] {
  // The labels worth showing: positive priority, the strongest `limit` of them.
  const ranked = requests
    .map((_, i) => i)
    .filter((i) => requests[i].priority > 0 && requests[i].width > 0)
    .sort((a, b) => requests[b].priority - requests[a].priority || a - b);
  const focused = ranked.filter((i) => requests[i].priority === Infinity);
  const rest = ranked.filter((i) => requests[i].priority !== Infinity);
  const kept = new Set([
    ...focused,
    ...rest.slice(0, Math.max(0, Math.floor(Math.min(limit, 1e6)))),
  ]);

  const result: PinLabel[] = requests.map((request) => ({
    id: request.id,
    visible: false,
    side: request.side,
    row: 0,
    left: settle(request.x - request.width / 2),
    top: settle(request.side === "up" ? ceiling - lift - request.height : floor + lift),
    points: [
      { x: settle(request.x), y: settle(request.y) },
      { x: settle(request.x), y: settle(request.y) },
    ],
  }));

  for (const side of ["up", "down"] as const) {
    let items: Item[] = [];
    requests.forEach((request, index) => {
      if (request.side !== side || !kept.has(index)) return;
      const ridge = [...(request.ridge ?? []), { x: request.x, y: request.y }].sort(
        (m, n) => m.x - n.x,
      );
      items.push({
        index,
        request,
        p: request.x,
        a: ridge[0].x,
        b: ridge[ridge.length - 1].x,
        ridge,
      });
    });
    items.sort((m, n) => m.p - n.p || m.index - n.index);
    if (!items.length) continue;

    // The rows: as near the ring as the bounds allow, every label in a row on one line.
    const rowHeight = Math.max(...items.map((item) => item.request.height));
    const up = side === "up";
    const near = up
      ? clamp(ceiling - lift, bounds.top + rowHeight, bounds.bottom)
      : clamp(floor + lift, bounds.top, bounds.bottom - rowHeight);
    const far = up ? near - rowHeight - rowGap : near + rowHeight + rowGap;
    const farFits =
      rows > 1 && (up ? far - rowHeight >= bounds.top : far + rowHeight <= bounds.bottom);
    /** Where a row's labels line up: their bottoms above the ring, their tops below it. */
    const edgeOf = (row: number) => (row === 0 ? near : far);
    const topOf = (row: number, height: number) => (up ? edgeOf(row) - height : edgeOf(row));
    const endOf = (row: number) => (up ? edgeOf(row) + clearance : edgeOf(row) - clearance);
    /** The shelf an elbow runs along: just under the near row, on the ring's side of it. */
    const shelf = up ? near + clearance + 5 : near - clearance - 5;

    /** A straight stem's x for a label at `left`, or null when no foot reaches under it. */
    const straight = (item: Item, left: number) => {
      const w = item.request.width;
      const room = Math.min(inset, w / 3);
      const lo = Math.max(item.a, left + room),
        hi = Math.min(item.b, left + w - room);
      return lo > hi + 1e-6 ? null : clamp(item.p, lo, Math.max(lo, hi));
    };

    const stemFor = (item: Item, left: number, row: number): Placed | null => {
      const w = item.request.width;
      const s = straight(item, left);
      const end = endOf(row);
      if (s !== null) {
        const foot = { x: s, y: footY(item, s) };
        return { left, row, elbow: false, foot: s, points: [foot, { x: s, y: end }] };
      }
      // One square elbow, on the near row only: along from the foot to the shelf, across under the
      // label, then along into it.
      if (row !== 0) return null;
      const room = Math.min(inset, w / 3);
      const s0 = clamp(left + w / 2, item.a, item.b);
      const y0 = footY(item, s0);
      if (up ? y0 < shelf + 3 : y0 > shelf - 3) return null;
      const e = clamp(s0, left + room, left + w - room);
      return {
        left,
        row,
        elbow: true,
        foot: s0,
        points: [
          { x: s0, y: y0 },
          { x: s0, y: shelf },
          { x: e, y: shelf },
          { x: e, y: end },
        ],
      };
    };

    /** Lays out one assignment of items to rows; null when it can't be done cleanly. */
    const build = (assignment: readonly number[]): Map<number, Placed> | null => {
      const placed = new Map<number, Placed>();
      const farItems = items.filter((_, k) => assignment[k] === 1);
      const farStems: number[] = [];
      if (farItems.length) {
        const spread = dodge(
          farItems.map((item) => ({ desired: item.p, height: item.request.width })),
          bounds.left,
          bounds.right,
          gap,
        );
        if (!spread.fits) return null;
        for (let k = 0; k < farItems.length; k++) {
          const stem = stemFor(farItems[k], spread.tops[k], 1);
          if (!stem) return null;
          placed.set(farItems[k].index, stem);
          farStems.push(stem.foot);
        }
      }
      // The near row, in runs between the far row's stems, which pass between its labels.
      let lo = bounds.left,
        run: Item[] = [];
      const settleRun = (hi: number) => {
        if (!run.length) return true;
        const spread = dodge(
          run.map((item) => ({ desired: item.p, height: item.request.width })),
          lo,
          hi,
          gap,
        );
        if (!spread.fits) return false;
        for (let k = 0; k < run.length; k++) {
          const stem = stemFor(run[k], spread.tops[k], 0);
          if (!stem) return false;
          placed.set(run[k].index, stem);
        }
        run = [];
        return true;
      };
      let far = 0;
      for (let k = 0; k < items.length; k++) {
        if (assignment[k] === 1) {
          if (!settleRun(farStems[far] - inset)) return null;
          lo = farStems[far] + inset;
          far++;
        } else run.push(items[k]);
      }
      if (!settleRun(bounds.right)) return null;
      return placed;
    };

    /** Clean means no stem crosses another stem or passes through another label. */
    const clean = (placed: Map<number, Placed>) => {
      const entries = [...placed.entries()];
      const boxes = entries.map(([index, spot]) => {
        const { width, height } = requests[index];
        const top = topOf(spot.row, height);
        return { left: spot.left, top, right: spot.left + width, bottom: top + height };
      });
      for (let m = 0; m < entries.length; m++) {
        const a = entries[m][1].points;
        for (let n = 0; n < entries.length; n++) {
          if (n === m) continue;
          for (let k = 0; k < a.length - 1; k++)
            if (segmentHitsBox(a[k], a[k + 1], boxes[n], 1)) return false;
          if (n < m) continue;
          const b = entries[n][1].points;
          for (let k = 0; k < a.length - 1; k++)
            for (let j = 0; j < b.length - 1; j++) {
              if (segmentsCross(a[k], a[k + 1], b[j], b[j + 1])) return false;
              // Two runs along the same shelf that overlap.
              const flatA = Math.abs(a[k].y - a[k + 1].y) < 1e-6,
                flatB = Math.abs(b[j].y - b[j + 1].y) < 1e-6;
              if (
                flatA &&
                flatB &&
                Math.abs(a[k].y - b[j].y) < 1e-6 &&
                Math.min(Math.max(a[k].x, a[k + 1].x), Math.max(b[j].x, b[j + 1].x)) >
                  Math.max(Math.min(a[k].x, a[k + 1].x), Math.min(b[j].x, b[j + 1].x)) + 1
              )
                return false;
            }
        }
      }
      return true;
    };

    /** What a layout costs to read: stems off their slab's middle, labels off their stems, far
     * rows, and above all elbows. */
    const cost = (placed: Map<number, Placed>) => {
      let total = 0;
      for (const item of items) {
        const spot = placed.get(item.index)!;
        const centre = spot.left + item.request.width / 2;
        total += 0.5 * Math.abs(spot.foot - item.p) + 0.15 * Math.abs(centre - spot.foot);
        if (spot.row === 1) total += 30;
        if (spot.elbow) total += 80 + 0.3 * Math.abs(spot.points[2].x - spot.points[1].x);
      }
      return total;
    };

    for (;;) {
      const assignments: number[][] = [items.map(() => 0)];
      if (farFits && items.length > 1 && items.length <= 7) {
        // Few enough to try every way of sharing them between the two rows.
        for (let mask = 1; mask < 1 << items.length; mask++)
          assignments.push(items.map((_, k) => (mask >> k) & 1));
      } else if (farFits && items.length > 1) {
        assignments.push(items.map((_, k) => k % 2));
        assignments.push(items.map((_, k) => 1 - (k % 2)));
        // Greedy: each label in the near row where it fits beside the last, else the far row.
        let nearRight = -Infinity,
          farRight = -Infinity;
        assignments.push(
          items.map((item) => {
            const left = item.p - item.request.width / 2;
            if (left >= nearRight + gap) {
              nearRight = left + item.request.width;
              return 0;
            }
            if (left >= farRight + gap && item.p > nearRight + inset) {
              farRight = left + item.request.width;
              return 1;
            }
            nearRight = Math.max(nearRight, left + item.request.width);
            return 0;
          }),
        );
      }
      let best: Map<number, Placed> | null = null,
        bestCost = Infinity;
      const seen = new Set<string>();
      for (const assignment of assignments) {
        const key = assignment.join("");
        if (seen.has(key)) continue;
        seen.add(key);
        const placed = build(assignment);
        if (!placed) continue;
        // Cost first: it's cheap, and only a layout that would win needs checking for crossings.
        const c = cost(placed);
        if (c < bestCost - 1e-6 && clean(placed)) {
          best = placed;
          bestCost = c;
        }
      }
      if (best) {
        for (const item of items) {
          const spot = best.get(item.index)!;
          const request = item.request;
          result[item.index] = {
            id: request.id,
            visible: true,
            side,
            row: spot.row,
            left: settle(spot.left),
            top: settle(topOf(spot.row, request.height)),
            points: spot.points.map((q) => ({ x: settle(q.x), y: settle(q.y) })),
          };
        }
        break;
      }
      // Too many for the room: drop the weakest (the latest-listed of equals) and try again.
      let weakest = 0;
      items.forEach((item, k) => {
        const w = items[weakest];
        if (
          item.request.priority < w.request.priority ||
          (item.request.priority === w.request.priority && item.index > w.index)
        )
          weakest = k;
      });
      items = items.filter((_, k) => k !== weakest);
      if (!items.length) break;
    }
  }
  return result;
}

/* ——— Anchors on tilted slabs ——— */

/** A slab as the ring draws it this frame: angles clockwise from 12 o'clock, including any turn. */
export type PinSlab = { start: number; end: number; radius: number; width: number; height: number };
/** Screen position of the point at `angle` and `radius` from the ring's centre, `z` above the
 * floor (a `RingFrame`'s `at`). */
export type PinProjector = (angle: number, radius: number, z?: number) => Point;

/**
 * Where a tilted slab's pin stands. A slab mostly at the back (the half nearer 12 o'clock, which
 * the camera sees from the front) points up from its top face's far edge; one mostly at the front
 * points down from its front face's lower edge. The preferred foot is the middle of the slab's
 * stretch on that side (or `along` it); `ridge` is the rest of that edge, kept `inset` px from the
 * slab's ends.
 * Null for a slab with no length.
 */
export function slabPin(
  at: PinProjector,
  slab: PinSlab,
  {
    side,
    along = 0.5,
    inset = 8,
    samples = 9,
  }: {
    /** Force the side instead of taking the one holding more of the slab. */
    side?: PinSide;
    /** Where along its stretch the preferred foot stands: 0 its start, 1 its end, 0.5 the middle. */
    along?: number;
    inset?: number;
    samples?: number;
  } = {},
): { side: PinSide; x: number; y: number; ridge: Point[] } | null {
  const span = slab.end - slab.start;
  if (!(span > 1e-4)) return null;
  // Which side holds more of the slab, and the longest unbroken stretch of it there.
  const steps = 48;
  const backAt = (k: number) => Math.cos(slab.start + (span * (k + 0.5)) / steps) > 0;
  let back = 0;
  for (let k = 0; k < steps; k++) if (backAt(k)) back++;
  const chosen: PinSide = side ?? (back * 2 >= steps ? "up" : "down");
  let bestFrom = 0,
    bestLength = 0;
  for (let k = 0; k < steps; ) {
    if (backAt(k) !== (chosen === "up")) {
      k++;
      continue;
    }
    let j = k;
    while (j < steps && backAt(j) === (chosen === "up")) j++;
    if (j - k > bestLength) {
      bestFrom = k;
      bestLength = j - k;
    }
    k = j;
  }
  let from = slab.start,
    to = slab.end;
  if (bestLength > 0 && bestLength < steps) {
    from = slab.start + (span * bestFrom) / steps;
    to = slab.start + (span * (bestFrom + bestLength)) / steps;
  }
  const outer = slab.radius + slab.width / 2;
  // The top face's far edge (just inside its rim), or the front face's foot.
  const r = chosen === "up" ? outer - Math.min(2, slab.width * 0.1) : outer;
  const z = chosen === "up" ? slab.height : 0;
  const trim = Math.min(inset / Math.max(1, outer), (to - from) * 0.3);
  const a0 = from + trim,
    a1 = to - trim;
  const ridge: Point[] = [];
  const n = Math.max(2, samples);
  for (let k = 0; k < n; k++) {
    const angle = a0 + ((a1 - a0) * k) / (n - 1);
    const q = at(angle, r, z);
    ridge.push({ x: q.x, y: q.y });
  }
  const foot = at(a0 + (a1 - a0) * Math.max(0, Math.min(1, along)), r, z);
  return { side: chosen, x: foot.x, y: foot.y, ridge };
}

/** The screen extent of tilted slabs: the highest point of any top (`ceiling`), the lowest point
 * of any foot (`floor`), and the leftmost and rightmost points. */
export function slabExtent(at: PinProjector, slabs: readonly PinSlab[], samples = 9) {
  let ceiling = Infinity,
    floor = -Infinity,
    left = Infinity,
    right = -Infinity;
  for (const slab of slabs) {
    if (!(slab.end - slab.start > 1e-4)) continue;
    for (let k = 0; k < samples; k++) {
      const angle = slab.start + ((slab.end - slab.start) * k) / (samples - 1);
      for (const radius of [slab.radius - slab.width / 2, slab.radius + slab.width / 2])
        for (const z of [0, slab.height]) {
          const q = at(angle, radius, z);
          ceiling = Math.min(ceiling, q.y);
          floor = Math.max(floor, q.y);
          left = Math.min(left, q.x);
          right = Math.max(right, q.x);
        }
    }
  }
  return { ceiling, floor, left, right };
}

/* ——— Framing a tilted ring with room for its pins ——— */

/** Px one row of pins takes: a line of labels and the gap to the next. */
export const PIN_ROW = 22;

/**
 * How far a tilted ring reaches on screen from its centre, per px of its outer radius: up to the
 * back of a slab `rise` outer radii tall, and down to the front of its floor, for the glass camera
 * (`GlassCamera`) at `elevation` radians from `distance` outer radii.
 */
export function tiltReach(rise: number, elevation: number, distance: number) {
  const s = Math.sin(elevation),
    c = Math.cos(elevation);
  return {
    up: (distance * (s + rise * c)) / Math.max(1e-6, distance + c - rise * s),
    down: (distance * s) / Math.max(1e-6, distance - c),
  };
}

/** A slab as a frame sees it: its angles (clockwise from 12 o'clock, any turn included), its
 * height and its outer edge, both in outer radii (the edge defaults to the ring's own). */
export type PinnedSlab = { start: number; end: number; rise: number; radius?: number };

/**
 * How far given slabs reach on screen from the ring's centre, per px of its outer radius: up to the
 * highest point of any slab's top, down to the lowest point of the floor under them.
 */
export function slabsReach(slabs: readonly PinnedSlab[], elevation: number, distance: number) {
  const s = Math.sin(elevation),
    c = Math.cos(elevation);
  // The glass camera's screen y (per outer radius) of the point at `angle`, `r` out, `z` up.
  const y = (angle: number, r: number, z: number) => {
    const dy = -Math.cos(angle) * r;
    return (distance * (dy * s - z * c)) / Math.max(1e-6, distance - dy * c - z * s);
  };
  let up = 0,
    down = 0;
  for (const slab of slabs) {
    const n = Math.max(2, Math.ceil(((slab.end - slab.start) / Math.PI) * 24));
    for (let k = 0; k <= n; k++) {
      const angle = slab.start + ((slab.end - slab.start) * k) / n;
      const r = slab.radius ?? 1;
      up = Math.max(up, -y(angle, r, slab.rise));
      down = Math.max(down, y(angle, r, 0));
    }
  }
  return { up, down };
}

/**
 * Frames a tilted ring in a plot `height` px tall with room for its pins: `rows` rows above it
 * (under `top` px kept for a note and a view switch) and below it. Returns the largest outer radius
 * that fits (at most `max`) and the screen y its centre should land on, so the ring, its slabs and
 * the rows sit centred in the plot. Without `slabs`, it allows for a slab `rise` outer radii tall at
 * the very back.
 */
export function framePinned(
  height: number,
  {
    rise,
    slabs,
    elevation,
    distance,
    max,
    top = 30,
    rows = 1,
    lift = 16,
  }: {
    /** The tallest slab, in outer radii. */
    rise: number;
    /** The slabs as they will stand, for a tighter frame. */
    slabs?: readonly PinnedSlab[];
    elevation: number;
    /** The camera's distance, in outer radii. */
    distance: number;
    max: number;
    top?: number;
    /** Rows each side, or above and below separately. */
    rows?: number | { above: number; below: number };
    lift?: number;
  },
) {
  const worst = tiltReach(rise, elevation, distance);
  const { above: rowsAbove, below: rowsBelow } =
    typeof rows === "number" ? { above: rows, below: rows } : rows;
  const reach = slabs?.length ? slabsReach(slabs, elevation, distance) : worst;
  const above = top + lift + rowsAbove * PIN_ROW,
    below = lift + rowsBelow * PIN_ROW + 2;
  const room = Math.max(1, height - above - below);
  const outer = Math.max(1, Math.min(max, room / (reach.up + reach.down)));
  const slack = Math.max(0, room - outer * (reach.up + reach.down));
  return { outer, y: above + outer * reach.up + slack / 2 };
}

/** A pin's stem as an SVG `points` attribute, on half pixels so a 1px hairline stays crisp. */
export const pinPoints = (points: readonly Point[]) =>
  points.map((q) => `${Math.floor(q.x) + 0.5},${Math.floor(q.y) + 0.5}`).join(" ");
