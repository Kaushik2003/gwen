"use client";
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

/* Icons: Solar (alt-arrow-up-bold, alt-arrow-down-bold) by 480 Design, CC BY 4.0,
 * https://www.figma.com/community/file/1166831539721848736 */

import { createContext, forwardRef, useContext, type CSSProperties, type ForwardedRef, type ReactNode } from "react";
import { outwardTooltipPosition, rowTooltipPosition } from "./tooltip-position";
import { noteFocusAnchor } from "./focus-anchor";
import { cssModule } from "./css-module";
import Styles from "./core.module.css";
import type { Rect } from "./geometry";
import Roles from "./type.module.css";
import { ENTER } from "./format";

const styles = cssModule(Styles, "lc-core-");
const roles = cssModule(Roles, "lc-type-");

/** The side each tooltip last took beside its item, and how far past the edge it may lean (into
 * its gap, never onto the item) before it gives that side up. */
const sides = new WeakMap<HTMLElement, "right" | "left">();
const SIDE_SLACK = 8;

/**
 * Sets the tooltip beside the item in focus, never over it and never out of `bounds`:
 * - `right` (the default): to the item's right, or its left when there is no room; once on a
 *   side it keeps it while it still fits, so neighbouring items don't flip it back and forth.
 *   Its top level with the item's top, or with `foot`, its bottom level with the item's bottom
 *   (for losses).
 * - `above`: centred over the item, or under it when there is no room.
 * - `row`: above a thin row, else below, else beside it.
 * - `outward`: out past a ring along the segment's angle, covering as little of the ring as can be.
 * It enters in place, then glides between items, a forced change of side included (`moveTooltip`).
 */
export function placeTooltip(
  tip: HTMLElement,
  anchor: Rect,
  bounds: Rect,
  {
    prefer = "right",
    foot = false,
    ring,
  }: {
    /** `row` for thin stacked rows (above, below, then beside); `outward` for ring segments. */
    prefer?: "right" | "above" | "row" | "outward";
    foot?: boolean;
    /** For `outward`: the ring's centre, outer radius and the segment's angle. */
    ring?: { cx: number; cy: number; outer: number; angle: number };
  } = {},
) {
  noteFocusAnchor(tip, anchor);
  const w = tip.offsetWidth,
    h = tip.offsetHeight,
    gap = 12;
  let x: number, y: number;
  if (prefer === "row") {
    ({ x, y } = rowTooltipPosition(anchor, { width: w, height: h }, bounds));
  } else if (prefer === "outward" && ring) {
    const at = outwardTooltipPosition({ width: w, height: h }, ring, ring.angle, bounds);
    x = at.left;
    y = at.top;
  } else if (prefer === "right") {
    // Right of the item, else left; and once on a side the card keeps it while it still fits
    // (within a few px of the edge), so a pointer moving one way never sends it back the other.
    // A flip that is truly forced glides like any other move.
    const right = anchor.right + gap,
      left = anchor.left - gap - w;
    const fitsRight = (slack: number) => right + w <= bounds.right + slack;
    const fitsLeft = (slack: number) => left >= bounds.left - slack;
    let side = sides.get(tip) ?? "right";
    if (side === "right" ? !fitsRight(SIDE_SLACK) : !fitsLeft(SIDE_SLACK))
      side = side === "right" ? (fitsLeft(0) ? "left" : "right") : fitsRight(0) ? "right" : "left";
    sides.set(tip, side);
    x = side === "right" ? right : left;
    y = foot ? anchor.bottom - h : anchor.top - 4;
  } else {
    x = (anchor.left + anchor.right) / 2 - w / 2;
    y = anchor.top - gap - h;
    if (y < bounds.top) y = anchor.bottom + gap;
  }
  x = Math.max(bounds.left, Math.min(bounds.right - w, x));
  y = Math.max(bounds.top, Math.min(bounds.bottom - h, y));
  moveTooltip(tip, x, y);
}

/* ——— The glide: one spring for every tooltip ——— */

/** The spring's rate (1/s): critically damped, so it never overshoots, and settled to within 2% in
 * about 0.22 s. */
const GLIDE = 26;
const round2 = (v: number) => Math.round(v * 100) / 100;
/** Where each tooltip is headed (its transform), and the offset still to travel for those gliding.
 * The spring is the only owner of `translate`; the entrance animates opacity alone. */
const targets = new WeakMap<HTMLElement, { x: number; y: number }>();
const glides = new Map<HTMLElement, { ox: number; oy: number; vx: number; vy: number }>();
let raf = 0,
  lastFrame = 0;
let reduced: MediaQueryList | null | undefined;
const reducedMotion = () => {
  if (reduced === undefined)
    reduced =
      typeof matchMedia === "function" ? matchMedia("(prefers-reduced-motion: reduce)") : null;
  return reduced?.matches ?? false;
};
/** One rAF for every gliding tooltip; it stops as soon as the last one settles. */
function glide(now: number) {
  // A stalled main thread resumes the glide a frame or two on, rather than leaping to catch up.
  const dt = lastFrame ? Math.min(1 / 30, (now - lastFrame) / 1000) : 1 / 60;
  lastFrame = now;
  // The exact solution of a critically damped spring over dt, so 60 and 120 Hz glide alike.
  const e = Math.exp(-GLIDE * dt);
  for (const [tip, g] of glides) {
    if (!tip.isConnected) {
      glides.delete(tip);
      continue;
    }
    const ax = g.vx + GLIDE * g.ox,
      ay = g.vy + GLIDE * g.oy;
    g.ox = (g.ox + ax * dt) * e;
    g.oy = (g.oy + ay * dt) * e;
    g.vx = (g.vx - GLIDE * ax * dt) * e;
    g.vy = (g.vy - GLIDE * ay * dt) * e;
    if (Math.hypot(g.ox, g.oy) < 0.05 && Math.hypot(g.vx, g.vy) < 2) {
      glides.delete(tip);
      tip.style.translate = "";
    } else tip.style.translate = `${round2(g.ox)}px ${round2(g.oy)}px`;
  }
  if (glides.size) raf = requestAnimationFrame(glide);
  else {
    raf = 0;
    lastFrame = 0;
  }
}

/** The entrance: a short fade (opacity only) while the spring lifts the card its last 2px, so a
 * retarget during the entrance still glides. */
const RISE = 2;
const enter = (tip: HTMLElement) =>
  tip.animate?.([{ opacity: 0 }, { opacity: 1 }], {
    duration: ENTER.tooltip * 1000,
    easing: "cubic-bezier(0.2, 0.8, 0.2, 1)",
  });

const startGlide = (tip: HTMLElement, g: { ox: number; oy: number; vx: number; vy: number }) => {
  glides.set(tip, g);
  tip.style.translate = `${round2(g.ox)}px ${round2(g.oy)}px`;
  if (!raf) raf = requestAnimationFrame(glide);
};

/**
 * Moves a tooltip's top-left to (x, y) in its plot. Its first visible placement is its entrance: it
 * fades in where it lands, rising its last 2px. After that it glides on one shared, critically
 * damped spring, picking up where it is and how fast it is already moving, so a pointer sweeping
 * across marks (or arrow keys stepping through them) reads as one smooth glide, never steps, even
 * while it is still fading in. Shown again after hiding, it enters again at its new place; only a
 * jump across most of the plot fades rather than flies. Reduced motion lands at once, without the
 * fade. The transform always holds the target, so code that reads it back sees where the card is
 * going; the spring rides on the separate `translate`.
 */
export function moveTooltip(tip: HTMLElement, x: number, y: number) {
  x = round2(x);
  y = round2(y);
  const held = targets.get(tip);
  const shown = tip.style.visibility === "visible";
  tip.style.transform = `translate(${x}px, ${y}px)`;
  tip.style.visibility = "visible";
  if (reducedMotion()) {
    if (glides.delete(tip)) tip.style.translate = "";
    targets.set(tip, { x, y });
    return;
  }
  if (!held || !shown) {
    // The entrance, once per appearance (a first placement, or a card shown again).
    targets.set(tip, { x, y });
    enter(tip);
    startGlide(tip, { ox: 0, oy: RISE, vx: 0, vy: 0 });
    return;
  }
  if (held.x === x && held.y === y) return;
  const g = glides.get(tip) ?? { ox: 0, oy: 0, vx: 0, vy: 0 };
  // Where the card is now, relative to its new target.
  const ox = held.x + g.ox - x,
    oy = held.y + g.oy - y;
  held.x = x;
  held.y = y;
  const parent = tip.offsetParent as HTMLElement | null;
  const span = Math.max(parent?.clientWidth ?? 640, parent?.clientHeight ?? 0);
  if (Math.hypot(ox, oy) > Math.max(360, span * 0.75)) {
    // Across most of the plot: enter at the new place rather than fly over the chart.
    enter(tip);
    startGlide(tip, { ox: 0, oy: RISE, vx: 0, vy: 0 });
    return;
  }
  startGlide(tip, { ox, oy, vx: g.vx, vy: g.vy });
}

export type TooltipRow = { label: ReactNode; color: string; muted?: boolean; value: ReactNode };

/** More words for whichever tooltip is showing, from the page around the chart: a sentence on what
 * the item in focus means, or a note its author wrote. The shared card shows it under its foot, and
 * so does the area family's own, so a host adds it once, around the chart, without a prop on each
 * family. Candlestick and timeline draw cards of their own that don't show it yet. */
export const TooltipExtra = createContext<ReactNode>(null);

/** The compare card: a title, one row per series (swatch, name, figure), and an optional foot.
 * Its bottom light takes the focused series' colour: `tint`, else the first lit row's. */
export const ChartTooltip = forwardRef(function ChartTooltip(
  {
    title,
    rows,
    footer,
    tint,
  }: {
    title: ReactNode;
    rows: readonly TooltipRow[];
    footer?: ReactNode;
    tint?: string;
  },
  ref: ForwardedRef<HTMLDivElement>,
) {
  const light = tint ?? (rows.find((row) => !row.muted) ?? rows[0])?.color;
  const extra = useContext(TooltipExtra);
  return (
    <div
      ref={ref}
      className={styles.tooltip}
      style={light ? ({ "--tint": light } as CSSProperties) : undefined}
      aria-hidden="true"
    >
      <span className={`${styles.tooltipTitle} ${roles.casing}`}>{title}</span>
      {rows.map((row, i) => (
        <span key={i} className={styles.tooltipRow}>
          <span
            className={styles.swatch}
            data-muted={row.muted}
            style={{ background: row.color }}
          />
          <span className={`${styles.tooltipLabel} ${roles.label}`}>{row.label}</span>
          {row.value}
        </span>
      ))}
      {footer}
      {extra ? <span className={`${styles.tooltipExtra} ${roles.caption}`}>{extra}</span> : null}
    </div>
  );
});

/** The library's one change arrow (Solar's bold alt arrow, cropped to the glyph), in the text's
 * colour. `Delta` uses it; use it wherever a change needs its direction drawn. */
export function DeltaArrow({ direction }: { direction: "up" | "down" }) {
  return (
    <svg width="8" height="8" viewBox="4 4 16 16" aria-hidden="true">
      <path
        d={
          direction === "down"
            ? "M12.3704 15.8351L18.8001 9.20467C19.2013 8.79094 18.9581 8 18.4297 8H5.5703C5.04189 8 4.79869 8.79094 5.1999 9.20467L11.6296 15.8351C11.8427 16.0549 12.1573 16.0549 12.3704 15.8351Z"
            : "M12.3704 8.16485L18.8001 14.7953C19.2013 15.2091 18.9581 16 18.4297 16H5.5703C5.04189 16 4.79869 15.2091 5.1999 14.7953L11.6296 8.16485C11.8427 7.94505 12.1573 7.94505 12.3704 8.16485Z"
        }
        fill="currentColor"
      />
    </svg>
  );
}

/** A change against a comparison, with a true minus. The arrow follows the direction; the colour
 * follows whether that is good news (`better="down"` for measures where lower is better, such as
 * latency or churn). */
export function Delta({
  value,
  better = "up",
  children,
}: {
  value: number;
  better?: "up" | "down";
  children?: ReactNode;
}) {
  const direction = value < 0 ? "down" : "up";
  const good = value === 0 || (direction === "up") === (better === "up");
  return (
    <span className={`${styles.delta} ${roles.caption}`} data-trend={good ? "up" : "down"}>
      <DeltaArrow direction={direction} />
      {children ?? `${value < 0 ? "−" : "+"}${Math.abs(value * 100).toFixed(1)}%`}
    </span>
  );
}

export function TooltipFoot({ children }: { children: ReactNode }) {
  return <span className={`${styles.tooltipFoot} ${roles.caption}`}>{children}</span>;
}
