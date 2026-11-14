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
"use client";

import { pixelRatio } from "@/components/liquid-charts/liquid/core/render-scale";
import { chartPresence, frameDelta, lostContext, substeps } from "@/components/liquid-charts/liquid/core/presence";
import { createChartOpticalRenderer, type ChartOpticalMark } from "@/components/liquid-charts/ui/chart-optical";
import { focusPresses, trackPointer } from "@/components/liquid-charts/liquid/core/liquid-pointer";
import { FIELD_LENGTH, LiquidField } from "@/components/liquid-charts/liquid/glass/liquid-field";
import type { Rgb, Theme } from "@/components/liquid-charts/liquid/glass/liquid-glass";
import { cssRgb as css } from "@/components/liquid-charts/liquid/core/colour-scale";
import { clamp01, settle } from "@/components/liquid-charts/liquid/core/geometry";
import { GRAPHITE, mixRgb } from "@/components/liquid-charts/liquid/core/colour";
import { liquidFeel } from "@/components/liquid-charts/liquid/core/liquid-feel";
import { useEffect, useRef, useState } from "react";

import type { DotFrame, DotMark } from "@/components/liquid-charts/liquid/core/mark-types";

export type { DotFrame, DotMark };

const rgba = ([r, g, b]: Rgb, a: number) =>
  `rgba(${Math.round(r * 255)}, ${Math.round(g * 255)}, ${Math.round(b * 255)}, ${a})`;
/** Ease out with a little overshoot, for beads growing in. */
const easeBack = (t: number) => {
  const x = clamp01(t) - 1;
  return 1 + 2.2 * x * x * x + 1.2 * x * x;
};

/** On white, glass that small reads pastel: beads under 10px deepen their pigment (up to 42%
 * at 4px and below), so small beads keep their colour. Dark cards need no help. */
export function beadPigment(tint: Rgb, r: number, theme: Theme): Rgb {
  if (theme !== "light" || r >= 10) return tint;
  const k = 1 - 0.42 * Math.min(1, (10 - Math.max(r, 3)) / 6);
  return [tint[0] * k, tint[1] * k, tint[2] * k];
}
const circlePath = (x: number, y: number, r: number) =>
  `M${x - r} ${y}A${r} ${r} 0 1 0 ${x + r} ${y}A${r} ${r} 0 1 0 ${x - r} ${y}Z`;

/** Glass depth, bevel and glow for a bead of radius `r`. Shallower than a bar of the same width
 * (light crosses a bead's whole diameter, so a bar's depth turns its centre dark), with a bevel
 * that rounds the rim; small beads glow less, so a dense scatter doesn't build a haze. */
export function beadSolid(r: number, depth = 0.35) {
  // Small beads need relatively more glass to hold their colour (thin glass reads pale on white).
  const solid = Math.max(
    2,
    Math.min(24, settle(Math.max(2 * r * depth, Math.min(2 * r * 0.9, 8)))),
  );
  const small = Math.max(0, Math.min(1, (r - 4) / 20));
  return {
    depth: solid,
    bevel: Math.max(0.4, settle(Math.min(solid * 0.45, r * 0.5))),
    glow: 0.25 + 0.75 * small * small,
  };
}

/** The bead under a point, or the nearest one within `reach` px of its rim; null when none is.
 * For a few thousand beads; beyond that, index them (a Delaunay or grid). */
export function nearestDot<T extends { cx: number; cy: number; radius: number }>(
  dots: readonly T[],
  x: number,
  y: number,
  reach = 12,
): T | null {
  let best: T | null = null,
    distance = Infinity;
  for (const dot of dots) {
    const gap = Math.hypot(dot.cx - x, dot.cy - y) - dot.radius;
    if (gap < distance) {
      distance = gap;
      best = dot;
    }
  }
  return distance <= reach ? best : null;
}

/**
 * One glass canvas for round marks: scatter points, bubbles, beeswarms, packed circles and
 * outliers. Beads spring to new positions and sizes, fade between tint and graphite, and hold a
 * simulated liquid (`LiquidField`) that pours in from the bottom on entrance, lands and settles,
 * ripples under a sweeping pointer and sags when the bead comes into focus (beads under 10px
 * keep the pour without a surface). `onFrame` receives every bead's current geometry so labels
 * and tooltips can follow it. It draws only while something moves.
 *
 * Every bead goes through the optical renderer (no count limit; the page shares one GPU context).
 * Position and size changes rebuild a bead's mask, so the entrance animates only the fill and
 * tone. The canvas reaches `bleed` px past the plot so glows fade out. `available` turns false
 * without WebGL2, so the chart can draw flat instead.
 *
 * For trees and networks: the `grow` entrance grows each bead in at its delay (with a little
 * overshoot) and fills it as it grows, with no liquid surface; per bead, `from`, `immediate` and
 * `hidden`; an `onFrame` that returns true keeps frames coming (links still easing); `wake()`
 * draws again when the layout changed outside React; and `engine: "flat"` draws the same beads
 * flat on a 2D canvas (an open, empty bead reads as a faint disc with a rim). The engine and the
 * entrance are fixed for the canvas's life.
 */
export function useDotMarks({
  theme,
  layout,
  onFrame,
  depth = 0.35,
  glow = 1,
  replay = 0,
  bleed = 24,
  engine = "glass",
  entrance = "pour",
}: {
  theme: Theme;
  /** `now` (ms) is the chart's presence clock: held while it is off screen or waiting to enter, so
   * an entrance timed from it starts with the beads' own. */
  layout: (width: number, height: number, now: number) => DotMark[];
  /** Every bead as drawn this frame (`now` on the same clock as `layout`'s). Return true to keep
   * frames coming (something else eases). */
  onFrame?: (dots: DotFrame[], width: number, height: number, now: number) => boolean | void;
  /** Glass depth as a multiple of each bead's diameter. */
  depth?: number;
  /** Scales every bead's halo and shadow (1 = the automatic amount for its size). */
  glow?: number;
  /** Change to replay the entrance. */
  replay?: number;
  bleed?: number;
  /** Glass beads (default), or flat ones on a 2D canvas. */
  engine?: "glass" | "flat";
  /** `pour` (default): beads stand empty and liquid pours in. `grow`: beads grow in and fill. */
  entrance?: "pour" | "grow";
}) {
  const box = useRef<HTMLDivElement>(null);
  const canvas = useRef<HTMLCanvasElement>(null);
  const [available, setAvailable] = useState(true);
  const live = useRef({ theme, depth, glow, replay, layout, onFrame, wake: () => {} });

  useEffect(() => {
    Object.assign(live.current, { theme, depth, glow, replay, layout, onFrame });
    live.current.wake();
  });

  useEffect(() => {
    const surface = canvas.current,
      root = box.current;
    if (!surface || !root) return;
    const optical = engine === "glass" ? createChartOpticalRenderer(surface) : null;
    const flat = engine === "flat" ? surface.getContext("2d") : null;
    const pour = entrance === "pour";
    const shared = live.current;
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    const edge = bleed;
    Object.assign(surface.style, {
      left: `${-edge}px`,
      top: `${-edge}px`,
      width: `calc(100% + ${edge * 2}px)`,
      height: `calc(100% + ${edge * 2}px)`,
    });
    let width = 0,
      height = 0,
      canvasWidth = 0,
      canvasHeight = 0,
      ratio = 1;
    // As in useBarMarks: a whole number of CSS px, placed on a whole device pixel, or the browser
    // resamples the canvas and every rim softens.
    const offset = { x: edge, y: edge };
    const observer = new ResizeObserver(() => {
      const r = root.getBoundingClientRect();
      // A flat canvas keeps at most 2 device px per CSS px; glass sizes its own backing store.
      ratio = flat ? pixelRatio(2) : pixelRatio();
      const fraction = (value: number) => ((((value * ratio) % 1) + 1) % 1) / ratio;
      width = r.width;
      height = r.height;
      offset.x = edge + fraction(r.left);
      offset.y = edge + fraction(r.top);
      canvasWidth = Math.ceil(width + offset.x + edge);
      canvasHeight = Math.ceil(height + offset.y + edge);
      if (flat) {
        surface.width = Math.max(1, Math.round(canvasWidth * ratio));
        surface.height = Math.max(1, Math.round(canvasHeight * ratio));
      }
      // The optical renderer sizes the backing store from these; the CSS size must match it
      // exactly, or the browser stretches the bitmap and every rim blurs.
      surface.style.width = `${canvasWidth}px`;
      surface.style.height = `${canvasHeight}px`;
      surface.style.left = `${-offset.x}px`;
      surface.style.top = `${-offset.y}px`;
      wake();
    });
    observer.observe(root);
    // The pointer stirs the liquid; a grown bead holds none.
    const pointer = pour ? trackPointer(root, () => wake()) : null;
    const presence = chartPresence(root, () => wake(), { redraw });
    // The optical renderer announces a restored context on the canvas: pick the motion back up.
    const onOptical = (event: Event) => {
      if ((event as CustomEvent<{ available?: boolean }>).detail?.available) wake();
    };
    surface.addEventListener("chart-optical-status", onOptical);
    const presses = focusPresses();
    const feel = liquidFeel(root);
    type Spring = { value: number; velocity: number };
    const state = new Map<
      string,
      {
        x: Spring;
        y: Spring;
        r: Spring;
        tone: number;
        level: number;
        liquid: LiquidField | null;
        field: Float32Array | null;
      }
    >();
    let seeds = 0;
    const spring = (s: Spring, target: number, dt: number, rest = 0.2) => {
      for (let k = 0, n = substeps(dt); k < n; k++) {
        s.velocity += (110 * (target - s.value) - 21 * s.velocity) * (dt / n);
        s.value += s.velocity * (dt / n);
      }
      if (Math.abs(target - s.value) < rest && Math.abs(s.velocity) < 1) {
        s.value = target;
        s.velocity = 0;
        return false;
      }
      return true;
    };
    let replayed = shared.replay,
      start = presence.clock(performance.now()),
      raf = 0,
      last = 0;

    function step(frameTime: number) {
      raf = 0;
      if (!width || !height) return;
      // Offscreen or in a hidden tab: keep every bead as it is and draw nothing until it is back.
      if (!presence.active) {
        last = 0;
        return;
      }
      // Time runs on the presence clock at its real rate: a long frame stutters, and past a quarter
      // second the motion snaps to its end.
      const now = presence.clock(frameTime);
      const { dt, snap: long } = frameDelta(now, last);
      // A settled frame (print, export) lands every motion at its end.
      const snap = long || presence.settling;
      last = now;
      if (shared.replay !== replayed) {
        replayed = shared.replay;
        start = now;
        for (const dot of state.values()) dot.liquid?.reset();
      }
      const elapsed = (now - start) / 1000;
      let moving = false;
      const seen = new Set<string>();
      const targets = shared.layout(width, height, now);
      // Beads that just came into focus (hover or keyboard) are pressed.
      const joined = reduced || !pour ? [] : presses(targets);
      const frames: DotFrame[] = targets.map((target) => {
        seen.add(target.key);
        const level = clamp01(target.level ?? 1);
        let dot = state.get(target.key);
        if (!dot) {
          const from = target.from;
          dot = {
            x: { value: from?.x ?? target.x, velocity: 0 },
            y: { value: from?.y ?? target.y, velocity: 0 },
            r: { value: from?.r ?? target.r, velocity: 0 },
            tone: target.lit,
            level,
            liquid: pour ? new LiquidField(++seeds) : null,
            field: pour ? new Float32Array(FIELD_LENGTH) : null,
          };
          state.set(target.key, dot);
        }
        if (reduced || snap) {
          dot.x = { value: target.x, velocity: 0 };
          dot.y = { value: target.y, velocity: 0 };
          dot.r = { value: target.r, velocity: 0 };
          dot.tone = target.lit;
          dot.level = level;
        } else {
          if (target.immediate) {
            dot.x = { value: target.x, velocity: 0 };
            dot.y = { value: target.y, velocity: 0 };
          } else {
            if (spring(dot.x, target.x, dt)) moving = true;
            if (spring(dot.y, target.y, dt)) moving = true;
          }
          if (spring(dot.r, target.r, dt, 0.1)) moving = true;
          if (!pour) {
            dot.level += (level - dot.level) * (1 - Math.exp(-7 * dt));
            if (Math.abs(level - dot.level) > 0.002) moving = true;
            else dot.level = level;
          }
          dot.tone += (target.lit - dot.tone) * (1 - Math.exp(-11 * dt));
          if (Math.abs(target.lit - dot.tone) > 0.004) moving = true;
          else dot.tone = target.lit;
        }
        if (!dot.liquid || !dot.field) {
          // Grow: in at the delay with a little overshoot, filling as it grows.
          const t = (elapsed - 0.1 - (target.delay ?? 0)) / 0.55;
          const shown = reduced ? 1 : easeBack(t);
          if (!reduced && t < 1) moving = true;
          return {
            ...target,
            cx: dot.x.value,
            cy: dot.y.value,
            radius: Math.max(0.5, dot.r.value * Math.max(0, shown)),
            tone: dot.tone,
            fill: clamp01(t * 1.6) * dot.level,
            shown: clamp01(shown),
          };
        }
        // The liquid pours in from the bottom once the bead's delay passes; a full bead's top is
        // the glass it lands against.
        const radius = Math.max(0.5, dot.r.value),
          span = radius * 2 + 2,
          rest = level * span;
        const launched = elapsed >= 0.25 + (target.delay ?? 0);
        if (reduced || (snap && launched)) dot.liquid.settle(rest);
        else {
          if (!launched) moving = true;
          let near: { s: number; push: number } | null = null;
          const p = pointer?.current;
          if (p) {
            const across = p.x - dot.x.value,
              along = p.y - (dot.y.value + radius - dot.liquid.front);
            const proximity =
              Math.exp(-((across / (radius * 2.8)) ** 2)) * Math.exp(-((along / 80) ** 2));
            const push = p.vx * proximity;
            near = {
              s: Math.max(-1, Math.min(1, across / radius)),
              push: Math.abs(push) < 2 ? 0 : push,
            };
          }
          if (joined.includes(target.key)) {
            dot.liquid.press(near?.s ?? 0, radius * 2);
            feel.press(target.key, near !== null);
          }
          const liquidMoving = dot.liquid.step(dt, {
            target: launched ? rest : 0,
            thickness: radius * 2,
            depth: beadSolid(radius, shared.depth).depth,
            ceiling: level > 0.999 ? span + 1 : undefined,
            pointer: near,
          });
          feel.after(target.key, dot.liquid);
          if (liquidMoving) moving = true;
        }
        dot.liquid.rows(dot.field, 0, rest);
        return {
          ...target,
          cx: dot.x.value,
          cy: dot.y.value,
          radius,
          tone: dot.tone,
          fill: Math.max(0, dot.liquid.front / span),
          shown: 1,
        };
      });
      for (const key of state.keys()) if (!seen.has(key)) state.delete(key);
      if (pointer?.settle(dt)) moving = true;
      const drawn = frames.filter(
        (dot) => !dot.hidden && (pour || (dot.shown > 0.01 && dot.radius > 0.6)),
      );
      if (flat) drawFlat(flat, drawn);
      const result = optical?.draw({
        width: canvasWidth,
        height: canvasHeight,
        dpr: pixelRatio(),
        theme: shared.theme,
        reducedMotion: reduced,
        marks: drawn.map((dot): ChartOpticalMark => {
          const cx = settle(dot.cx + offset.x),
            cy = settle(dot.cy + offset.y),
            r = settle(dot.radius);
          const solid = beadSolid(r, shared.depth);
          return {
            id: dot.key,
            kind: "glass",
            path: circlePath(cx, cy, r),
            bounds: { x: cx - r, y: cy - r, width: 2 * r, height: 2 * r },
            color: css(beadPigment(dot.tint, r, shared.theme)),
            tone: dot.tone,
            fill: dot.fill,
            fillOrigin: "bottom",
            pattern:
              dot.pattern === undefined || dot.pattern === false
                ? undefined
                : dot.pattern === true
                  ? 0
                  : dot.pattern,
            // The path is this circle exactly, so the glass needs no mask as it moves or grows.
            shape: { kind: "circle", cx, cy, r, exact: true },
            depth: solid.depth,
            bevel: solid.bevel,
            glow: solid.glow * shared.glow,
            field: state.get(dot.key)?.field ?? undefined,
          };
        }),
      });
      // A lost context is temporary: hold the last frame until the restore repaints it.
      if (result && lostContext(result)) {
        last = 0;
        return;
      }
      if (result && !result.available) {
        setAvailable(false);
        return;
      }
      if (shared.onFrame?.(frames, width, height, now)) moving = true;
      // It loops only on screen once entered (about a third visible); else it holds this frame.
      if (moving && presence.running) raf = requestAnimationFrame(step);
      else last = 0;
    }
    function drawFlat(ctx: CanvasRenderingContext2D, dots: readonly DotFrame[]) {
      ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
      ctx.clearRect(0, 0, canvasWidth, canvasHeight);
      ctx.translate(offset.x, offset.y);
      const dark = shared.theme === "dark";
      const graphite = GRAPHITE[shared.theme];
      for (const dot of dots) {
        const colour = mixRgb(graphite, dot.tint, dot.tone);
        const r = dot.radius;
        // An empty bead is a faint disc; a full one solid.
        ctx.beginPath();
        ctx.arc(dot.cx, dot.cy, r, 0, Math.PI * 2);
        ctx.fillStyle = rgba(colour, 0.18 + 0.72 * clamp01(dot.fill));
        ctx.fill();
        if (r > 2.5) {
          ctx.lineWidth = 1;
          ctx.strokeStyle = rgba(
            mixRgb(colour, dark ? [1, 1, 1] : [0.09, 0.09, 0.09], dark ? 0.25 : 0.22),
            0.7,
          );
          ctx.beginPath();
          ctx.arc(dot.cx, dot.cy, r - 0.5, 0, Math.PI * 2);
          ctx.stroke();
        }
      }
    }
    function wake() {
      if (!raf) raf = requestAnimationFrame(step);
    }
    /** Draws a frame now (print, export). */
    function redraw() {
      cancelAnimationFrame(raf);
      raf = 0;
      step(performance.now());
    }
    shared.wake = wake;
    wake();
    return () => {
      cancelAnimationFrame(raf);
      observer.disconnect();
      pointer?.stop();
      presence.stop();
      surface.removeEventListener("chart-optical-status", onOptical);
      optical?.destroy();
      shared.wake = () => {};
    };
    // The engine, entrance and bleed are fixed for the canvas's life (remount to change them).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return { box, canvas, available, wake: () => live.current.wake() };
}
