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
import { createChartOpticalRenderer, type ChartOpticalMark } from "@/components/liquid-charts/ui/chart-optical";
import { chartPresence, frameDelta, substeps } from "@/components/liquid-charts/liquid/core/presence";
import { focusPresses, trackPointer } from "@/components/liquid-charts/liquid/core/liquid-pointer";
import { createLiquidGlass, GLASS_LIMIT, GLASS_WALL, type Theme } from "./liquid-glass";
import { cssRgb as css } from "@/components/liquid-charts/liquid/core/colour-scale";
import { liquidFeel } from "@/components/liquid-charts/liquid/core/liquid-feel";
import { FIELD_LENGTH, LiquidField } from "./liquid-field";
import { settle } from "@/components/liquid-charts/liquid/core/geometry";
import { BAR_SLANTS, slantedSolid, type BarView } from "@/components/liquid-charts/liquid/core/camera";
import { useEffect, useRef, useState } from "react";

import { funnelShellBounds, funnelShellPath } from "@/components/liquid-charts/liquid/core/funnel-shell";
import type { BarShell, BarPaintOutcome } from "@/components/liquid-charts/liquid/core/mark-types";
import type { ChartOpticalResult } from "@/components/liquid-charts/ui/chart-optical";

import type { BarFrame, BarMark, Direction } from "@/components/liquid-charts/liquid/core/mark-types";

export type { BarFrame, BarMark, Direction };

/** The raymarched renderer draws up to this many bars; beyond it the optical renderer takes over. */
export const RAYMARCH_LIMIT = 24;
/** Bars at a slant (side, isometric) stay raymarched up to all the glass holds: the optical
 * renderer draws only straight on, so past this a slanted chart is drawn face on. */
export const SLANT_LIMIT = GLASS_LIMIT;

/** The shallowest a shelled solid (a funnel's stages) is drawn, px: its front and back walls with
 * their edges, and a body of liquid between them as deep as one of them. */
export const SHELL_MIN_DEPTH = 3 * (GLASS_WALL + 0.8);

/** A scene's glass depth: `depth` times its thinnest bar, so a thin bar keeps its proportions. A
 * funnel's stages are one solid whose last stage can be a sliver, the narrower the funnel the
 * thinner: drawn as shallow as that, its walls would meet and no stage would show its liquid. A
 * scene with shells is never shallower than `SHELL_MIN_DEPTH`. */
export function solidDepthOf(
  frames: readonly { thickness: number; shell?: unknown }[],
  depth: number,
) {
  const solid = Math.min(...frames.map((bar) => bar.thickness), 200) * depth;
  return frames.some((bar) => bar.shell) ? Math.max(solid, SHELL_MIN_DEPTH) : solid;
}

/** A bar's rect in canvas px, and a path with corners rounded to the bevel: the silhouette the
 * raymarched solids have, so both renderers draw the same shape. */
function barRect(bar: BarFrame, shift: { x: number; y: number }) {
  const length = Math.max(bar.current, 1),
    half = bar.thickness / 2;
  const vertical = bar.direction === "up" || bar.direction === "down";
  const cross = bar.cross + (vertical ? shift.x : shift.y),
    base = bar.base + (vertical ? shift.y : shift.x);
  if (bar.direction === "up")
    return { x: cross - half, y: base - length, width: bar.thickness, height: length };
  if (bar.direction === "down")
    return { x: cross - half, y: base, width: bar.thickness, height: length };
  if (bar.direction === "right")
    return { x: base, y: cross - half, width: length, height: bar.thickness };
  return { x: base - length, y: cross - half, width: length, height: bar.thickness };
}
function roundedPath(r: { x: number; y: number; width: number; height: number }, radius: number) {
  const k = Math.min(radius, r.width / 2, r.height / 2);
  const right = r.x + r.width,
    bottom = r.y + r.height;
  return `M${r.x + k} ${r.y}H${right - k}A${k} ${k} 0 0 1 ${right} ${r.y + k}V${bottom - k}A${k} ${k} 0 0 1 ${right - k} ${bottom}H${r.x + k}A${k} ${k} 0 0 1 ${r.x} ${bottom - k}V${r.y + k}A${k} ${k} 0 0 1 ${r.x + k} ${r.y}Z`;
}
const ORIGIN = { up: "bottom", down: "top", right: "left", left: "right" } as const;
const patternStart = (pattern: BarMark["pattern"]) =>
  pattern === undefined || pattern === false ? undefined : pattern === true ? 0 : pattern;

/**
 * One glass canvas for bars of any direction. Bars spring to new lengths and places (every edge;
 * a resize snaps them instead), fade between tint and graphite, and hold a simulated liquid
 * (`LiquidField`): on entrance it pours in from the base, lands and settles; its surface moves
 * point by point, ripples under a sweeping pointer, and sags and coats the glass when a bar comes
 * into focus. `onFrame` receives every bar's current geometry so labels, tooltips and axes can
 * follow it. It draws only while something moves.
 *
 * The canvas reaches `bleed` px past the plot on every side, so the glow fades out instead of
 * stopping at the plot's edge; layouts and frames stay in plot coordinates. Columns can be seen at
 * a slant (`view`), as square glass solids whose sides show: their fronts stay where the straight-on
 * bars are, so frames, labels and hover don't change. Up to 24 bars (48 at a slant) use the
 * raymarched renderer, more use the optical one; both share one GPU context per page. `available`
 * turns false without WebGL2, so the chart can draw flat instead.
 */
export function useBarMarks({
  theme,
  layout,
  onFrame,
  onPaint,
  depth = 0.6,
  bevel = 5,
  replay = 0,
  bleed = 24,
  view = "flat",
}: {
  theme: Theme;
  layout: (width: number, height: number) => BarMark[];
  onFrame?: (bars: BarFrame[], width: number, height: number) => void;
  /** Actual canvas draw outcomes, bound to the caller revision; not capability. */
  onPaint?: (outcomes: readonly BarPaintOutcome[]) => void;
  /** Glass depth as a multiple of the thinnest bar. */
  depth?: number;
  bevel?: number;
  /** Change to replay the entrance. */
  replay?: number;
  bleed?: number;
  /** Straight on, or at a slant: from the side, or from a corner and above. Columns only; rows
   * and more than `SLANT_LIMIT` bars stay straight on. The canvas's `data-view` says which. */
  view?: BarView;
}) {
  const box = useRef<HTMLDivElement>(null);
  const canvas = useRef<HTMLCanvasElement>(null);
  const [available, setAvailable] = useState(true);
  const live = useRef({
    theme,
    depth,
    bevel,
    replay,
    layout,
    onFrame,
    onPaint,
    view,
    wake: () => {},
  });

  useEffect(() => {
    Object.assign(live.current, { theme, depth, bevel, replay, layout, onFrame, onPaint, view });
    live.current.wake();
  });

  useEffect(() => {
    const surface = canvas.current,
      root = box.current;
    if (!surface || !root) return;
    const shared = live.current;
    let paintedFrames: BarFrame[] = [];
    const report = (painted: boolean, reason?: string, rendered?: readonly string[]) => {
      shared.onPaint?.(
        paintedFrames.map(
          (bar): BarPaintOutcome => ({
            key: bar.key,
            revision: bar.revision ?? 0,
            status: painted && (!rendered || rendered.includes(bar.key)) ? "native" : "host",
            ...(!painted || (rendered && !rendered.includes(bar.key))
              ? { reason: reason ?? "not-rendered" }
              : {}),
          }),
        ),
      );
      // The optional host-shell consumer restores host paint on failure. Do not leave a stale
      // canvas over that restored host. Ordinary native charts keep their existing behavior.
      if (shared.onPaint) surface.style.visibility = painted ? "" : "hidden";
    };
    const glass = createLiquidGlass(surface, {
      shared: true,
      onPaint: onPaint ? (result) => report(result.painted, result.reason) : undefined,
    });
    let optical: ReturnType<typeof createChartOpticalRenderer> | null = null;
    if (!glass) {
      setAvailable(false);
      if (!shared.onPaint) return;
      const unavailable = () => {
        const bounds = root.getBoundingClientRect();
        paintedFrames = shared
          .layout(bounds.width, bounds.height)
          .map((mark) => ({ ...mark, current: mark.length, tone: mark.lit, fill: 0 }));
        report(false, "webgl-unavailable");
      };
      shared.wake = unavailable;
      const observer = new ResizeObserver(unavailable);
      observer.observe(root);
      unavailable();
      return () => {
        observer.disconnect();
        shared.wake = () => {};
      };
    }
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
      canvasHeight = 0;
    // Where plot px (0, 0) sits on the canvas: the bleed, plus the fraction that puts the canvas on
    // a whole device pixel. The canvas is also a whole number of CSS px, so its backing store maps
    // 1:1 to the screen; a fractional size or position makes the browser resample it, which
    // softens and stair-steps every edge. (Found by the ring family.)
    const offset = { x: edge, y: edge };
    const observer = new ResizeObserver(() => {
      const r = root.getBoundingClientRect();
      const ratio = pixelRatio();
      const fraction = (value: number) => ((((value * ratio) % 1) + 1) % 1) / ratio;
      width = r.width;
      height = r.height;
      offset.x = edge + fraction(r.left);
      offset.y = edge + fraction(r.top);
      canvasWidth = Math.ceil(width + offset.x + edge);
      canvasHeight = Math.ceil(height + offset.y + edge);
      surface.style.left = `${-offset.x}px`;
      surface.style.top = `${-offset.y}px`;
      glass.resize(canvasWidth, canvasHeight);
      // Redrawn at once: a resize clears the canvas, and a frame later would show it blank.
      redraw();
    });
    observer.observe(root);
    const pointer = trackPointer(root, () => wake());
    const presence = chartPresence(root, () => wake(), { redraw });
    const presses = focusPresses();
    const feel = liquidFeel(root);
    type Spring = { value: number; velocity: number };
    const state = new Map<
      string,
      {
        length: number;
        velocity: number;
        tone: number;
        cross: Spring;
        base: Spring;
        thickness: Spring;
        liquid: LiquidField;
        field: Float32Array;
        shell?: { x: Spring; y: Spring }[];
      }
    >();
    let seeds = 0;
    // Every edge springs between data; a resize snaps the bars to their new places instead.
    const spring = (s: Spring, target: number, dt: number) => {
      for (let k = 0, n = substeps(dt); k < n; k++) {
        s.velocity += (110 * (target - s.value) - 21 * s.velocity) * (dt / n);
        s.value += s.velocity * (dt / n);
      }
      if (Math.abs(target - s.value) < 0.3 && Math.abs(s.velocity) < 1) {
        s.value = target;
        s.velocity = 0;
        return false;
      }
      return true;
    };
    let sized = "";
    let replayed = shared.replay,
      start = presence.clock(performance.now()),
      raf = 0,
      last = 0,
      engine: "raymarch" | "optical" = "raymarch";

    function step(frameTime: number) {
      raf = 0;
      if (!width || !height) return;
      // Offscreen or in a hidden tab: keep every bar as it is and draw nothing until it is back.
      if (!presence.active) {
        last = 0;
        return;
      }
      // Time runs on the presence clock (held while the chart can't be seen), at its real rate: a
      // long frame stutters, and past a quarter second the motion snaps to its end.
      const now = presence.clock(frameTime);
      const { dt, snap: long } = frameDelta(now, last);
      // A settled frame (print, export) lands every motion at its end.
      const snap = long || presence.settling;
      last = now;
      if (shared.replay !== replayed) {
        replayed = shared.replay;
        start = now;
        for (const bar of state.values()) bar.liquid.reset();
      }
      const elapsed = (now - start) / 1000;
      let moving = false;
      const size = `${width}x${height}`;
      const resized = size !== sized;
      sized = size;
      const targets = shared.layout(width, height);
      // At a slant, columns are square solids seen from the side or a corner.
      const upright = targets.every((t) => t.direction === "up" || t.direction === "down");
      const slant =
        shared.view !== "flat" &&
        upright &&
        targets.length > 0 &&
        targets.length <= SLANT_LIMIT &&
        !targets.some((t) => t.shell)
          ? BAR_SLANTS[shared.view]
          : undefined;
      // Bars that just came into focus (hover or keyboard) are pressed: the liquid sags and
      // ripples out from under the pointer.
      const joined = reduced || presence.staticGlass ? [] : presses(targets);
      const frames: BarFrame[] = targets.map((target) => {
        let bar = state.get(target.key);
        const level = Math.max(0, Math.min(1, target.level ?? 1));
        if (!bar) {
          const from = target.from ?? {};
          bar = {
            length: from.length ?? target.length,
            velocity: 0,
            tone: target.lit,
            cross: { value: from.cross ?? target.cross, velocity: 0 },
            base: { value: from.base ?? target.base, velocity: 0 },
            thickness: { value: from.thickness ?? target.thickness, velocity: 0 },
            liquid: new LiquidField(++seeds),
            field: new Float32Array(FIELD_LENGTH),
          };
          state.set(target.key, bar);
        }
        if (reduced || resized || snap) {
          for (const [edge, value] of [
            [bar.cross, target.cross],
            [bar.base, target.base],
            [bar.thickness, target.thickness],
          ] as const) {
            edge.value = value;
            edge.velocity = 0;
          }
        } else {
          if (spring(bar.cross, target.cross, dt)) moving = true;
          if (spring(bar.base, target.base, dt)) moving = true;
          if (spring(bar.thickness, target.thickness, dt)) moving = true;
        }
        if (reduced || snap || (resized && target.shell)) {
          bar.length = target.length;
          bar.velocity = 0;
          bar.tone = target.lit;
        } else {
          for (let k = 0, n = substeps(dt); k < n; k++) {
            bar.velocity += (110 * (target.length - bar.length) - 21 * bar.velocity) * (dt / n);
            bar.length += bar.velocity * (dt / n);
          }
          if (Math.abs(target.length - bar.length) < 0.3 && Math.abs(bar.velocity) < 1) {
            bar.length = target.length;
            bar.velocity = 0;
          } else moving = true;
          bar.tone += (target.lit - bar.tone) * (1 - Math.exp(-11 * dt));
          if (Math.abs(target.lit - bar.tone) > 0.004) moving = true;
          else bar.tone = target.lit;
        }
        // The liquid: it pours in once the bar's delay passes and settles at its level; a full bar's
        // tip is the glass it lands against.
        const span = Math.max(bar.length, 1) + 2,
          thickness = bar.thickness.value,
          rest = level * span;
        const vertical = target.direction === "up" || target.direction === "down";
        const launched = elapsed >= 0.25 + (target.delay ?? 0);
        if (reduced || presence.staticGlass || (snap && launched)) bar.liquid.settle(rest);
        else {
          if (!launched) moving = true;
          let near: { s: number; push: number } | null = null;
          const p = pointer.current;
          if (p) {
            const sign = target.direction === "up" || target.direction === "left" ? -1 : 1;
            const across = (vertical ? p.x : p.y) - bar.cross.value,
              along = (vertical ? p.y : p.x) - (bar.base.value + sign * bar.liquid.front);
            const proximity =
              Math.exp(-((across / (thickness * 1.4)) ** 2)) * Math.exp(-((along / 200) ** 2));
            const push = (vertical ? p.vx : -p.vy) * proximity;
            // s runs from the left wall to the right one, or from the bottom wall to the top one.
            const s = Math.max(-1, Math.min(1, ((vertical ? 1 : -1) * across) / (thickness / 2)));
            near = { s, push: Math.abs(push) < 2 ? 0 : push };
          }
          const reach = Math.min(1, span / Math.max(1, vertical ? height : width));
          if (joined.includes(target.key)) {
            bar.liquid.press(near?.s ?? 0, thickness);
            feel.press(target.key, near !== null, reach);
          }
          const liquidMoving = bar.liquid.step(dt, {
            target: launched ? rest : 0,
            thickness,
            depth: slant ? slantedSolid(slant, thickness).side : thickness * shared.depth,
            ceiling: level > 0.999 ? span + 1 : undefined,
            pointer: near,
          });
          feel.after(target.key, bar.liquid, reach);
          if (liquidMoving) moving = true;
        }
        bar.liquid.rows(bar.field, 0, rest);
        let shell: BarShell | undefined;
        if (target.shell) {
          bar.shell ??= target.shell.map((p) => ({
            x: { value: p.x, velocity: 0 },
            y: { value: p.y, velocity: 0 },
          }));
          shell = target.shell.map((p, i) => {
            const vertex = bar!.shell![i];
            for (const axis of ["x", "y"] as const) {
              if (reduced || resized || snap) {
                vertex[axis].value = p[axis];
                vertex[axis].velocity = 0;
              } else if (spring(vertex[axis], p[axis], dt)) moving = true;
            }
            return { x: vertex.x.value, y: vertex.y.value };
          }) as unknown as BarShell;
        } else bar.shell = undefined;
        return {
          ...target,
          cross: bar.cross.value,
          base: bar.base.value,
          thickness: bar.thickness.value,
          current: bar.length,
          tone: bar.tone,
          fill: Math.max(0, bar.liquid.front / span),
          ...(shell ? { shell } : {}),
        };
      });
      if (pointer.settle(dt)) moving = true;
      if (state.size > frames.length) {
        const keys = new Set(frames.map((bar) => bar.key));
        for (const key of state.keys()) if (!keys.has(key)) state.delete(key);
      }
      const thinnest = Math.min(...frames.map((bar) => bar.thickness), 200);
      // At a slant the depth is a square footprint's side; a turned bar's diagonal spans its
      // thickness, so it is as wide on screen as straight on.
      const footprint = (thickness: number) =>
        slant ? slantedSolid(slant, thickness).side : thickness;
      const solidDepth = settle(slant ? footprint(thinnest) : solidDepthOf(frames, shared.depth)),
        solidBevel = settle(Math.min(shared.bevel, footprint(thinnest) * 0.2));
      // Seen from the side, the bars stand on a floor at their base and fade toward it, as a
      // floor's shadow would take them; below it nothing shows (the bottom faces it hides).
      const grounded = slant && slant.y > 0 && frames.every((bar) => bar.direction === "up");
      const floor = grounded
        ? {
            y: settle(Math.max(...frames.map((bar) => bar.base)) + offset.y),
            fade: settle(Math.min(height * 0.3, Math.max(...frames.map((bar) => bar.current)))),
          }
        : undefined;
      // The view the bars are drawn in, on the canvas for hosts and tests: face on past a slant's
      // limit, for rows, and for shells.
      const drawn = slant ? shared.view : "flat";
      if (surface!.dataset.view !== drawn) surface!.dataset.view = drawn;
      const next = frames.length > (slant ? SLANT_LIMIT : RAYMARCH_LIMIT) ? "optical" : "raymarch";
      if (next !== engine) {
        engine = next;
        // The optical renderer paints this canvas now: the glass must not repaint over it.
        if (engine === "optical") glass!.invalidate();
        if (engine === "raymarch") {
          // The optical renderer hides its canvas when it lets go; take it back and repaint.
          optical?.destroy();
          optical = null;
          surface!.style.visibility = "";
          glass!.invalidate();
        }
      }
      paintedFrames = frames;
      if (engine === "raymarch") {
        glass!.draw({
          kind: "bars",
          theme: shared.theme,
          depth: solidDepth,
          bevel: solidBevel,
          // The glow fades out over the bleed, as it was made for.
          margin: edge,
          ...(slant ? { slant, edges: true } : {}),
          ...(floor ? { floor } : {}),
          bars: frames.map((bar) => {
            const vertical = bar.direction === "up" || bar.direction === "down";
            return {
              x: settle(bar.cross + (vertical ? offset.x : offset.y)),
              base: settle(bar.base + (vertical ? offset.y : offset.x)),
              width: settle(footprint(bar.thickness)),
              height: settle(Math.max(bar.current, 1)),
              direction: bar.direction,
              fill: bar.fill,
              tint: bar.tint,
              tone: settle(bar.tone),
              pattern: patternStart(bar.pattern),
              marks: bar.marks,
              field: presence.staticGlass ? undefined : state.get(bar.key)?.field,
              ...(bar.shell
                ? {
                    shell: bar.shell.map((p) => ({
                      x: p.x + offset.x,
                      y: p.y + offset.y,
                    })) as unknown as BarShell,
                  }
                : {}),
            };
          }),
        });
      } else {
        optical ??= createChartOpticalRenderer(surface!);
        const result = optical.draw({
          width: canvasWidth,
          height: canvasHeight,
          // The registry's ratio, so an export's scale reaches the optical glass too.
          dpr: pixelRatio(),
          theme: shared.theme,
          marks: frames.map((bar): ChartOpticalMark => {
            const shell = bar.shell?.map((p) => ({
              x: p.x + offset.x,
              y: p.y + offset.y,
            })) as unknown as BarShell | undefined;
            const r = shell ? funnelShellBounds(shell) : barRect(bar, offset);
            return {
              id: bar.key,
              kind: "glass",
              path: shell ? funnelShellPath(shell) : roundedPath(r, solidBevel),
              bounds: r,
              color: css(bar.tint),
              tone: bar.tone,
              fill: bar.fill,
              fillOrigin: ORIGIN[bar.direction],
              pattern: patternStart(bar.pattern),
              marks: bar.marks,
              field: presence.staticGlass ? undefined : state.get(bar.key)?.field,
              // The path is this rect with its corners rounded by the bevel: no mask needed.
              ...(shell
                ? {}
                : { shape: { kind: "rect" as const, ...r, radius: solidBevel, exact: true } }),
              depth: solidDepth,
              bevel: solidBevel,
            };
          }),
        });
        report(result.available, result.fallback[0]?.reason, result.rendered);
      }
      shared.onFrame?.(frames, width, height);
      // It loops only on screen once entered (about a third visible); else it holds this frame.
      if (moving && presence.running) raf = requestAnimationFrame(step);
      else last = 0;
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
    const opticalStatus = (event: Event) => {
      if (engine !== "optical") return;
      const result = (event as CustomEvent<ChartOpticalResult>).detail;
      report(result.available, result.fallback[0]?.reason, result.rendered);
    };
    surface.addEventListener("chart-optical-status", opticalStatus);
    shared.wake = wake;
    wake();
    return () => {
      cancelAnimationFrame(raf);
      observer.disconnect();
      surface.removeEventListener("chart-optical-status", opticalStatus);
      report(false, "disposed");
      pointer.stop();
      presence.stop();
      glass.destroy();
      optical?.destroy();
      shared.wake = () => {};
    };
    // The bleed is fixed for the canvas's life.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return { box, canvas, available };
}
