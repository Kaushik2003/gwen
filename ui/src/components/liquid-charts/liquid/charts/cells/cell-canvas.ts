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

/**
 * Flat cells on one 2D canvas: the render path for colour-coded cells (style rule 1). Every edge
 * is snapped to a whole device pixel, so the hairline gaps stay crisp at any size; the canvas is
 * a whole number of CSS px placed on a whole device pixel, as the core's layers are. Cells sweep in
 * on entrance (each after its `delay`), colours spring between data, cells out of focus dim
 * towards graphite (keeping their pattern in cool greys), missing values are etched empty cells,
 * and the cell in focus lifts over its gap with a ring. It draws only while something moves.
 */

import { pixelRatio } from "@/components/liquid-charts/liquid/core/render-scale";
import { dimmed, INK, luminance, SURFACE } from "@/components/liquid-charts/liquid/core/colour-scale";
import { chartPresence, frameDelta, substeps } from "@/components/liquid-charts/liquid/core/presence";
import { clamp01, easeOut } from "@/components/liquid-charts/liquid/core/geometry";
import type { Rgb, Theme } from "@/components/liquid-charts/liquid/core";
import { ENTER } from "@/components/liquid-charts/liquid/core/format";
import type { Segment } from "./calendar";
import { useEffect, useRef } from "react";

export type FlatCell = {
  key: string;
  /** The cell's rect in plot px (whole px). */
  x: number;
  y: number;
  w: number;
  h: number;
  /** The cell's colour; null for no value (an etched, empty cell). */
  colour: Rgb | null;
  /** 1 lit, 0 dimmed towards graphite. Eases. */
  lit: number;
  /** Seconds after the entrance starts that this cell appears. */
  delay?: number;
  /** In focus: lifted over its gap by `grow` px, with a ring. */
  focus?: boolean;
  grow?: number;
  /** A quiet inner ring (today). */
  mark?: boolean;
  /** Draw a round mark instead, `circle` (0–1) of the largest radius the cell holds. */
  circle?: number;
  /** Liquid only: how full the pool is (0–1, default 1, to the brim). */
  level?: number;
  /** Liquid only: frosted lanes inside the liquid (incomplete, projected, outside a target). */
  etched?: boolean;
};

const rgba = ([r, g, b]: Rgb, a: number) =>
  `rgba(${Math.round(r * 255)},${Math.round(g * 255)},${Math.round(b * 255)},${Math.round(a * 1000) / 1000})`;

function roundRect(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  w: number,
  h: number,
  r: number,
) {
  const k = Math.max(0, Math.min(r, w / 2, h / 2));
  ctx.beginPath();
  if (typeof ctx.roundRect === "function") {
    ctx.roundRect(x, y, w, h, k);
    return;
  }
  ctx.moveTo(x + k, y);
  ctx.arcTo(x + w, y, x + w, y + h, k);
  ctx.arcTo(x + w, y + h, x, y + h, k);
  ctx.arcTo(x, y + h, x, y, k);
  ctx.arcTo(x, y, x + w, y, k);
  ctx.closePath();
}

type CellState = {
  rgb: [number, number, number];
  velocity: [number, number, number];
  tone: number;
  focus: number;
  presence: number;
};

export function useCellCanvas({
  theme,
  layout,
  overlay,
  replay = 0,
  radius = 2,
  bleed = 10,
}: {
  theme: Theme;
  layout: (width: number, height: number) => FlatCell[];
  /** Lines over the cells (a range's outline), plot px. */
  overlay?: (width: number, height: number) => readonly Segment[];
  replay?: number;
  /** Corner radius, CSS px (smaller cells get proportionally less). */
  radius?: number;
  bleed?: number;
}) {
  const box = useRef<HTMLDivElement>(null);
  const canvas = useRef<HTMLCanvasElement>(null);
  const live = useRef({ theme, layout, overlay, replay, radius, wake: () => {} });

  useEffect(() => {
    Object.assign(live.current, { theme, layout, overlay, replay, radius });
    live.current.wake();
  });

  useEffect(() => {
    const surface = canvas.current,
      root = box.current;
    const ctx = surface?.getContext("2d");
    if (!surface || !root || !ctx) return;
    const shared = live.current;
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    const edge = bleed;
    let width = 0,
      height = 0,
      ratio = 1;
    const offset = { x: edge, y: edge };
    const observer = new ResizeObserver(() => {
      const r = root.getBoundingClientRect();
      ratio = pixelRatio(3);
      const fraction = (value: number) => ((((value * ratio) % 1) + 1) % 1) / ratio;
      width = r.width;
      height = r.height;
      offset.x = edge + fraction(r.left);
      offset.y = edge + fraction(r.top);
      const w = Math.ceil(width + offset.x + edge),
        h = Math.ceil(height + offset.y + edge);
      surface.style.left = `${-offset.x}px`;
      surface.style.top = `${-offset.y}px`;
      surface.style.width = `${w}px`;
      surface.style.height = `${h}px`;
      surface.width = Math.round(w * ratio);
      surface.height = Math.round(h * ratio);
      // Redrawn at once: the resize cleared the canvas, and a frame later would show it blank.
      redraw();
    });
    observer.observe(root);
    // Drawn only near the screen, looping only on it; the entrance runs once the chart is a third
    // in view, on a clock held while it is away.
    const presence = chartPresence(root, () => wake(), { redraw });
    const state = new Map<string, CellState>();
    let replayed = shared.replay,
      start = presence.clock(performance.now()),
      raf = 0,
      last = 0;

    function step(frameTime: number) {
      raf = 0;
      if (!width || !height) return;
      if (!presence.active) {
        last = 0;
        return;
      }
      const now = presence.clock(frameTime);
      // The real time since the last frame; past a quarter second (or in a settled frame, for
      // print and export) the motion lands at its end instead of playing in slow motion.
      const { dt, snap } = frameDelta(now, last);
      const jump = snap || presence.settling;
      last = now;
      if (shared.replay !== replayed) {
        replayed = shared.replay;
        start = now;
      }
      const elapsed = (now - start) / 1000;
      const t = shared.theme;
      const ink = INK[t];
      let moving = false;
      const cells = shared.layout(width, height);
      const seen = new Set<string>();
      // Critically damped colour springs: no overshoot, so a colour never passes its target.
      const omega = 15;
      const ease = (k: number) => 1 - Math.exp(-k * dt);
      const X = (v: number) => Math.round((v + offset.x) * ratio);
      const Y = (v: number) => Math.round((v + offset.y) * ratio);
      ctx!.setTransform(1, 0, 0, 1, 0, 0);
      ctx!.clearRect(0, 0, surface!.width, surface!.height);
      const drawn: { cell: FlatCell; s: CellState; appear: number }[] = [];
      for (const cell of cells) {
        seen.add(cell.key);
        let s = state.get(cell.key);
        const target = cell.colour;
        if (!s) {
          const c = target ?? SURFACE[t];
          s = {
            rgb: [c[0], c[1], c[2]],
            velocity: [0, 0, 0],
            tone: cell.lit,
            focus: cell.focus ? 1 : 0,
            presence: target ? 1 : 0,
          };
          state.set(cell.key, s);
        }
        if (reduced || jump) {
          if (target) s.rgb = [target[0], target[1], target[2]];
          s.velocity = [0, 0, 0];
          s.tone = cell.lit;
          s.focus = cell.focus ? 1 : 0;
          s.presence = target ? 1 : 0;
        } else {
          if (target)
            for (let i = 0; i < 3; i++) {
              for (let k = 0, n = substeps(dt); k < n; k++) {
                s.velocity[i] +=
                  (omega * omega * (target[i] - s.rgb[i]) - 2 * omega * s.velocity[i]) * (dt / n);
                s.rgb[i] += s.velocity[i] * (dt / n);
              }
              if (Math.abs(target[i] - s.rgb[i]) > 0.002 || Math.abs(s.velocity[i]) > 0.01)
                moving = true;
              else {
                s.rgb[i] = target[i];
                s.velocity[i] = 0;
              }
            }
          const presence = target ? 1 : 0;
          s.presence += (presence - s.presence) * ease(9);
          if (Math.abs(presence - s.presence) > 0.004) moving = true;
          else s.presence = presence;
          s.tone += (cell.lit - s.tone) * ease(12);
          if (Math.abs(cell.lit - s.tone) > 0.004) moving = true;
          else s.tone = cell.lit;
          const focus = cell.focus ? 1 : 0;
          s.focus += (focus - s.focus) * ease(20);
          if (Math.abs(focus - s.focus) > 0.004) moving = true;
          else s.focus = focus;
        }
        const appear = reduced
          ? 1
          : easeOut((elapsed - ENTER.delay - (cell.delay ?? 0)) / ENTER.small);
        if (appear < 1) moving = true;
        drawn.push({ cell, s, appear });
      }
      for (const key of state.keys()) if (!seen.has(key)) state.delete(key);
      // Lifted cells draw last, over their neighbours.
      drawn.sort((a, b) => a.s.focus - b.s.focus);

      for (const { cell, s, appear } of drawn) {
        if (appear <= 0) continue;
        const grow = (cell.grow ?? 0) * s.focus;
        let l = X(cell.x - grow),
          tp = Y(cell.y - grow),
          r = X(cell.x + cell.w + grow),
          b = Y(cell.y + cell.h + grow);
        if (appear < 1) {
          // Grow from the centre on entrance.
          const k = 0.5 + 0.5 * appear;
          const cx = (l + r) / 2,
            cy = (tp + b) / 2;
          l = cx - (cx - l) * k;
          r = cx + (r - cx) * k;
          tp = cy - (cy - tp) * k;
          b = cy + (b - cy) * k;
        }
        const w = r - l,
          h = b - tp;
        if (w <= 0 || h <= 0) continue;
        const k = Math.min(shared.radius * ratio, Math.min(w, h) * 0.24);
        const colour = dimmed(s.rgb, t, (1 - s.tone) * 0.78);
        ctx!.globalAlpha = appear;
        if (s.focus > 0.01) {
          ctx!.save();
          ctx!.shadowColor = t === "dark" ? "rgba(0,0,0,0.65)" : "rgba(39,39,39,0.28)";
          ctx!.shadowBlur = 12 * ratio * s.focus;
          ctx!.shadowOffsetY = 3 * ratio * s.focus;
          roundRect(ctx!, l, tp, w, h, k);
          ctx!.fillStyle = rgba(s.presence > 0.5 ? colour : SURFACE[t], 1);
          ctx!.fill();
          ctx!.restore();
        }
        if (cell.circle !== undefined) {
          const radius = Math.max(0.5, (Math.min(w, h) / 2) * clamp01(cell.circle));
          ctx!.beginPath();
          ctx!.arc((l + r) / 2, (tp + b) / 2, radius, 0, Math.PI * 2);
          ctx!.fillStyle = rgba(colour, s.presence);
          ctx!.fill();
        } else if (s.presence > 0.001) {
          roundRect(ctx!, l, tp, w, h, k);
          ctx!.fillStyle = rgba(colour, s.presence);
          ctx!.fill();
        }
        if (s.presence < 0.999 && cell.circle === undefined) {
          // Etched: a faint pane with frosted diagonal lanes and a hairline rim.
          const a = (1 - s.presence) * (0.55 + 0.45 * s.tone);
          ctx!.save();
          roundRect(ctx!, l, tp, w, h, k);
          ctx!.fillStyle = rgba(ink, (t === "dark" ? 0.035 : 0.03) * a);
          ctx!.fill();
          ctx!.clip();
          ctx!.strokeStyle = rgba(ink, (t === "dark" ? 0.17 : 0.16) * a);
          ctx!.lineWidth = Math.max(1, ratio);
          const pitch = 4 * ratio;
          ctx!.beginPath();
          for (let x = l - h; x < r; x += pitch) {
            ctx!.moveTo(x, b);
            ctx!.lineTo(x + h, tp);
          }
          ctx!.stroke();
          ctx!.restore();
          roundRect(
            ctx!,
            l + 0.5 * ratio,
            tp + 0.5 * ratio,
            w - ratio,
            h - ratio,
            Math.max(0, k - 0.5 * ratio),
          );
          ctx!.strokeStyle = rgba(ink, (t === "dark" ? 0.12 : 0.13) * a);
          ctx!.lineWidth = ratio;
          ctx!.stroke();
        }
        if (cell.mark && cell.circle === undefined) {
          const on = s.presence > 0.5 ? colour : SURFACE[t];
          const tone = luminance(on) > 0.36 ? INK.light : INK.dark;
          const inset = Math.max(1.5, Math.min(2.5, Math.min(w, h) / ratio / 7)) * ratio;
          roundRect(
            ctx!,
            l + inset,
            tp + inset,
            w - inset * 2,
            h - inset * 2,
            Math.max(0, k - inset),
          );
          ctx!.strokeStyle = rgba(tone, 0.8);
          ctx!.lineWidth = 1.25 * ratio;
          ctx!.stroke();
        }
        if (s.focus > 0.01 && cell.circle === undefined) {
          const line = 1.5 * ratio;
          roundRect(
            ctx!,
            l + line / 2,
            tp + line / 2,
            w - line,
            h - line,
            Math.max(0, k - line / 2),
          );
          ctx!.strokeStyle = rgba(ink, 0.92 * s.focus);
          ctx!.lineWidth = line;
          ctx!.stroke();
        }
        ctx!.globalAlpha = 1;
      }

      const segments = shared.overlay?.(width, height) ?? [];
      if (segments.length) {
        const settled = Math.min(1, ...drawn.map((d) => d.appear));
        ctx!.globalAlpha = reduced ? 1 : settled;
        ctx!.beginPath();
        for (const seg of segments) {
          ctx!.moveTo((seg.x1 + offset.x) * ratio, (seg.y1 + offset.y) * ratio);
          ctx!.lineTo((seg.x2 + offset.x) * ratio, (seg.y2 + offset.y) * ratio);
        }
        ctx!.strokeStyle = rgba(ink, t === "dark" ? 0.85 : 0.8);
        ctx!.lineWidth = 1.5 * ratio;
        ctx!.lineCap = "square";
        ctx!.stroke();
        ctx!.globalAlpha = 1;
      }
      if (moving && presence.running) raf = requestAnimationFrame(step);
      else last = 0;
    }
    function wake() {
      if (!raf) raf = requestAnimationFrame(step);
    }
    /** Draws a frame now: after a resize, and for print and export. */
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
      presence.stop();
      shared.wake = () => {};
    };
    // The bleed is fixed for the canvas's life.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return { box, canvas };
}
