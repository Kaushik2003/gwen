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
import { createLightLines, type LightLine } from "@/components/liquid-charts/liquid/core/light-lines";
import { chartPresence, frameDelta } from "@/components/liquid-charts/liquid/core/presence";
import { cssRgb as css } from "@/components/liquid-charts/liquid/core/colour-scale";
import { flatColour } from "@/components/liquid-charts/liquid/rings/ring-marks";
import type { Rgb, Theme } from "@/components/liquid-charts/liquid/core/colour";
import { settle } from "@/components/liquid-charts/liquid/core/geometry";
import { useEffect, useRef, useState } from "react";
import type { Box } from "./layout";
import { Motion } from "./motion";

/** One piece of glass, in plot px: any closed path (an arc, a sector, a radar's polygon). */
export type GlassShape = {
  id: string;
  path: string;
  /** The path's tight bounds; the glass mask is rasterised at this size, so keep it small. */
  bounds: Box;
  tint: Rgb;
  /** 1 lit in its tint, 0 smoked graphite (out of focus). */
  tone: number;
  /** How much of the glass is filled with liquid, 0–1, along `sweep` (the entrance). */
  fill?: number;
  /** An angular fill from `start` to `end` round (cx, cy): arcs sweep in clockwise. */
  sweep?: { cx: number; cy: number; start: number; end: number; clockwise?: boolean };
  /** A fill that grows from (cx, cy) outward, `inner` to `outer` px (rose slices, a radar's
   * polygon); the glass then takes its tip light at its outer edge. Wins over `sweep`. */
  radial?: { cx: number; cy: number; inner?: number; outer: number };
  /** The sector the path traces, so the glass lights its rims as a ring segment's (its tip at the
   * end of its sweep, or along its outer arc when it fills radially). */
  sector?: {
    cx: number;
    cy: number;
    inner: number;
    outer: number;
    start: number;
    end: number;
    /** The rounds the path was drawn with (core `sectorPath`'s `corner`). Given, the glass is
     * evaluated from the sector itself and needs no mask, however it springs. */
    corner?: number;
  };
  /** Engraved reference marks, shares (0–1) of the sweep. */
  marks?: readonly number[];
  /** The etched pattern from this share of the sweep to its end. */
  pattern?: number;
  opacity?: number;
  depth?: number;
  bevel?: number;
  /** The flat twin's path (default `path`); flat has no liquid, so it draws what is filled. */
  flat?: string;
};

export type LayerClock = {
  width: number;
  height: number;
  motion: Motion;
  /** Seconds since the entrance began (or was replayed). */
  time: number;
  reduced: boolean;
  /** What draws the glass this frame. */
  engine: "optical" | "svg";
};

export type LayerScene = {
  shapes: readonly GlassShape[];
  lines?: readonly LightLine[];
  /** Keep animating even if no spring moves (an entrance still running). */
  moving?: boolean;
};

export type LayerPaintResult = {
  engine: "optical" | "svg";
  rendered: readonly string[];
  fallback: readonly { id: string; reason: string }[];
};

const SVG = "http://www.w3.org/2000/svg";

/**
 * The radial family's mark layer: one glass canvas fed by the optical renderer (arbitrary closed
 * paths, so arcs, sectors and polygons all get the approved edge-lit glass), its flat SVG twin for
 * `material="flat"` or a page without WebGL, and a canvas of the core's light lines above them.
 * `frame` runs every animation frame while anything moves: it reads springs from the `motion`
 * store, places labels, and returns the shapes and lines to draw. Nothing redraws at rest.
 *
 * The glass canvas reaches `bleed` px past the plot so the glow fades out, and sits on a whole
 * device pixel at a whole number of CSS px, so its backing store maps 1:1 to the screen.
 */
export function useRadialLayer({
  theme,
  material = "glass",
  replay = 0,
  bleed = 40,
  frame,
  onPaint,
}: {
  theme: Theme;
  material?: "glass" | "flat";
  replay?: number;
  bleed?: number;
  frame: (clock: LayerClock) => LayerScene;
  /** Actual submitted shapes, separate from the frame's requested geometry. */
  onPaint?: (result: LayerPaintResult) => void;
}) {
  const box = useRef<HTMLDivElement>(null);
  const canvas = useRef<HTMLCanvasElement>(null);
  const lights = useRef<HTMLCanvasElement>(null);
  const svg = useRef<SVGSVGElement>(null);
  const [available, setAvailable] = useState(true);
  const live = useRef({ theme, material, replay, frame, onPaint, wake: () => {} });

  useEffect(() => {
    Object.assign(live.current, { theme, material, replay, frame, onPaint });
    live.current.wake();
  });

  useEffect(() => {
    const surface = canvas.current,
      root = box.current;
    if (!root) return;
    const shared = live.current;
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    const light = lights.current ? createLightLines(lights.current) : null;
    let optical: ReturnType<typeof createChartOpticalRenderer> | null = null;
    let opticalFailed = false;
    const edge = bleed;
    let width = 0,
      height = 0,
      canvasWidth = 0,
      canvasHeight = 0;
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
      if (surface) {
        surface.style.left = `${-offset.x}px`;
        surface.style.top = `${-offset.y}px`;
        surface.style.width = `${canvasWidth}px`;
        surface.style.height = `${canvasHeight}px`;
      }
      light?.resize(width, height);
      lightKey = "";
      lightPainted = false;
      optical?.invalidate?.();
      wake();
    });
    observer.observe(root);
    // Drawn only near the screen, looping only on it; the entrance runs once the chart is a third
    // in view, on a clock held while it is away.
    const presence = chartPresence(root, () => wake(), { redraw });

    const motion = new Motion();
    let lightPainted = false;
    let replayed = shared.replay,
      start = presence.clock(performance.now()),
      raf = 0,
      before = 0,
      lightKey = "",
      engine: "optical" | "svg" | null = null;
    const paths = new Map<string, SVGPathElement>();
    /** Each shape's path last frame, to tell when many are reshaping at once. */
    const previous = new Map<string, string>();

    function step(frameTime: number) {
      raf = 0;
      if (!width || !height) return;
      if (!presence.active) {
        before = 0;
        return;
      }
      const now = presence.clock(frameTime);
      // Past a quarter second (or in a settled frame, for print and export) every spring and ease
      // lands at its target instead of playing in slow motion.
      const { dt, snap } = frameDelta(now, before);
      const jump = snap || presence.settling;
      before = now;
      if (shared.replay !== replayed) {
        replayed = shared.replay;
        start = now;
        motion.reset();
      }
      const next: "optical" | "svg" =
        shared.material === "glass" && surface && !opticalFailed ? "optical" : "svg";
      if (next !== engine) {
        if (engine === "optical") {
          optical?.destroy();
          optical = null;
        }
        if (engine === "svg") {
          paths.forEach((path) => path.remove());
          paths.clear();
        }
        engine = next;
      }
      motion.begin(dt, reduced || jump);
      const scene = shared.frame({
        width,
        height,
        motion,
        time: reduced || presence.settling ? 1e6 : (now - start) / 1000,
        reduced: reduced || presence.settling,
        engine,
      });
      motion.end();
      let painted: LayerPaintResult = { engine, rendered: [], fallback: [] };

      if (engine === "optical" && surface) {
        optical ??= createChartOpticalRenderer(surface);
        const ox = settle(offset.x),
          oy = settle(offset.y);
        // Every reshaped piece of glass that needs a mask has it rebuilt. While many reshape at once
        // (springs between data), the glass is drawn at one device pixel per CSS px, a quarter of
        // the work, and comes back sharp the frame it settles. An exact sector (its corner given,
        // and a hole or no rounds) needs no mask.
        let reshaped = 0;
        for (const shape of scene.shapes) {
          const before = previous.get(shape.id);
          const exact =
            shape.sector?.corner !== undefined &&
            (shape.sector.inner > 0 || shape.sector.corner < 0.05);
          if (before !== undefined && before !== shape.path && !exact) reshaped++;
          previous.set(shape.id, shape.path);
        }
        const busy = reshaped >= 6 && !reduced;
        const result = optical.draw({
          width: canvasWidth,
          height: canvasHeight,
          dpr: busy ? 1 : pixelRatio(),
          theme: shared.theme,
          marks: scene.shapes
            .filter((shape) => shape.path && shape.bounds.width > 0 && shape.bounds.height > 0)
            .map(
              (shape): ChartOpticalMark => ({
                id: shape.id,
                kind: "glass",
                path: shape.path,
                transform: [1, 0, 0, 1, ox, oy],
                fillRule: "evenodd",
                bounds: {
                  x: shape.bounds.x + ox,
                  y: shape.bounds.y + oy,
                  width: shape.bounds.width,
                  height: shape.bounds.height,
                },
                color: css(shape.tint),
                tone: settle(shape.tone),
                fill: settle(shape.fill ?? 1),
                fillRadial: shape.radial
                  ? {
                      cx: shape.radial.cx + ox,
                      cy: shape.radial.cy + oy,
                      inner: shape.radial.inner,
                      outer: shape.radial.outer,
                    }
                  : undefined,
                fillOrigin: shape.sweep
                  ? {
                      cx: shape.sweep.cx + ox,
                      cy: shape.sweep.cy + oy,
                      start: shape.sweep.start,
                      end: shape.sweep.end,
                      clockwise: shape.sweep.clockwise ?? true,
                    }
                  : undefined,
                shape: shape.sector
                  ? {
                      kind: "sector",
                      cx: shape.sector.cx + ox,
                      cy: shape.sector.cy + oy,
                      innerRadius: shape.sector.inner,
                      outerRadius: shape.sector.outer,
                      startAngle: shape.sector.start,
                      endAngle: shape.sector.end,
                      corner: shape.sector.corner,
                      exact: shape.sector.corner !== undefined,
                    }
                  : undefined,
                marks: shape.marks,
                pattern: shape.pattern,
                opacity: shape.opacity,
                depth: shape.depth,
                bevel: shape.bevel,
              }),
            ),
        });
        painted = { engine, rendered: result.rendered, fallback: result.fallback };
        if (!result.available) {
          // No optical glass on this device: draw flat from the next frame.
          opticalFailed = true;
          setAvailable(false);
          motion.moving = true;
        }
        // Drawn soft this frame: come back once more to draw it sharp.
        if (busy) motion.moving = true;
      } else if (engine === "svg" && svg.current) {
        const layer = svg.current;
        const seen = new Set<string>();
        scene.shapes.forEach((shape, i) => {
          seen.add(shape.id);
          let path = paths.get(shape.id);
          if (!path) {
            path = document.createElementNS(SVG, "path");
            path.setAttribute("fill-rule", "evenodd");
            paths.set(shape.id, path);
          }
          if (layer.children[i] !== path) layer.insertBefore(path, layer.children[i] ?? null);
          const d = shape.flat ?? shape.path;
          if (path.getAttribute("d") !== d) path.setAttribute("d", d);
          path.setAttribute("fill", flatColour(shape.tint, shape.tone, shared.theme));
          const fade = shape.flat === undefined ? (shape.fill ?? 1) : 1;
          path.setAttribute("opacity", String(settle((shape.opacity ?? 1) * fade)));
        });
        for (const [id, path] of paths)
          if (!seen.has(id)) {
            path.remove();
            paths.delete(id);
          }
        painted = { engine, rendered: [...seen], fallback: [] };
      }

      let lightFailure = "Native light canvas is unavailable.";
      if (light) {
        const lines = scene.lines ?? [];
        const key = JSON.stringify([shared.theme, lines]);
        if (key !== lightKey) {
          lightKey = key;
          const draw = () =>
            light.draw({
              theme: shared.theme,
              lines,
              scrub: null,
              focus: null,
              reveal: 1,
              left: 0,
            });
          if (shared.onPaint) {
            try {
              lightPainted = draw();
            } catch (error) {
              lightPainted = false;
              lightFailure = error instanceof Error ? error.message : "Native light paint failed.";
            }
          } else lightPainted = draw();
        }
      }
      if (shared.onPaint) {
        const keys = (scene.lines ?? []).map((line) => line.key);
        const fallback = [
          ...painted.fallback,
          ...(!lightPainted ? keys.map((id) => ({ id, reason: lightFailure })) : []),
        ];
        const failed = new Set(fallback.map((item) => item.id));
        const rendered = [...new Set([...painted.rendered, ...(lightPainted ? keys : [])])].filter(
          (id) => !failed.has(id),
        );
        shared.onPaint({ engine, rendered, fallback });
      }
      if ((motion.moving || scene.moving) && presence.running) raf = requestAnimationFrame(step);
      else before = 0;
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
      presence.stop();
      optical?.destroy();
      paths.forEach((path) => path.remove());
      paths.clear();
      shared.wake = () => {};
    };
    // The bleed is fixed for the canvas's life.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return { box, canvas, lights, svg, available };
}
