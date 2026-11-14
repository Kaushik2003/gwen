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

import { createChartOpticalRenderer, type ChartOpticalMark } from "@/components/liquid-charts/ui/chart-optical";
import type { RingGlass, RingMark, RingScene } from "@/components/liquid-charts/liquid/core/ring-types";
import { sectorPath, TAU, trimmed } from "@/components/liquid-charts/liquid/core/arc";
import { cssRgb as css } from "@/components/liquid-charts/liquid/core/colour-scale";
import { lostContext } from "@/components/liquid-charts/liquid/core/presence";
import { pixelRatio } from "@/components/liquid-charts/liquid/core/render-scale";
import { FIELD_LENGTH, LiquidField } from "./liquid-field";
import { settle } from "@/components/liquid-charts/liquid/core/geometry";
import { createLiquidGlass } from "./liquid-glass";

const patternStart = (pattern: RingMark["pattern"]) =>
  pattern === undefined || pattern === false ? undefined : pattern === true ? 0 : pattern;

/**
 * Glass for ring marks, on one canvas: the raymarched ring (edge-lit glass, the
 * only engine that tilts, with true depth between concentric tracks) and the optical painter for
 * round ends and more segments than the raymarched ring takes. Both share one GPU context per page.
 * Each segment holds a simulated liquid that chases the entrance, sags when pressed and splashes
 * off a full segment's end. Null without WebGL2, so the ring draws flat.
 */
export function createRingGlass(canvas: HTMLCanvasElement): RingGlass | null {
  const glass = createLiquidGlass(canvas, { shared: true });
  if (!glass) return null;
  let optical: ReturnType<typeof createChartOpticalRenderer> | null = null;
  let last: RingScene["engine"] | null = null;
  const dropOptical = () => {
    optical?.destroy();
    optical = null;
  };
  return {
    resize: (width, height) => glass.resize(width, height),
    invalidate() {
      glass.invalidate();
      dropOptical();
      last = null;
    },
    destroy() {
      glass.destroy();
      dropOptical();
    },
    liquid: (seed) => new LiquidField(seed),
    fieldLength: FIELD_LENGTH,
    paint(scene) {
      // Another engine paints the canvas now: the one before must not repaint over it.
      if (scene.engine !== last) {
        if (last === "raymarch") glass.invalidate();
        if (last === "optical") dropOptical();
        last = scene.engine;
      }
      const { offset, ownTracks } = scene;
      const { radius, width: ringWidth, depth, gap, bevel } = scene.ring;
      const ring = scene.ring;
      if (scene.engine === "raymarch") {
        // The camera works in plot px; the shader reads canvas px, which start at the offset.
        const lens = scene.camera && {
          ...scene.camera,
          anchor: { x: scene.camera.anchor.x + offset.x, y: scene.camera.anchor.y + offset.y },
        };
        glass.draw({
          kind: "ring",
          theme: scene.theme,
          ring: {
            x: settle(ring.cx + offset.x),
            y: settle(ring.cy + offset.y),
            radius: settle(radius),
            width: settle(ringWidth),
            depth: settle(depth),
            gap: settle(gap),
          },
          bevel: settle(Math.min(bevel, ringWidth * 0.2)),
          camera: lens,
          segments: scene.segments.map((segment) => ({
            start: settle(segment.start * 1e3) / 1e3,
            end: settle(segment.end * 1e3) / 1e3,
            fill: settle(segment.fill * 1e3) / 1e3,
            tint: segment.tint,
            tone: settle(segment.tone),
            pop: settle(segment.pop),
            height: settle(segment.height),
            ...(ownTracks
              ? { radius: settle(segment.radius), width: settle(segment.width) }
              : undefined),
            field: segment.field,
          })),
        });
        return "drawn";
      }
      optical ??= createChartOpticalRenderer(canvas);
      const result = optical.draw({
        width: scene.size.width,
        height: scene.size.height,
        dpr: pixelRatio(),
        theme: scene.theme,
        marks: scene.segments.map((segment): ChartOpticalMark => {
          const ccx = ring.cx + offset.x,
            ccy = ring.cy + offset.y;
          const inner = segment.radius - segment.width / 2,
            outer = segment.radius + segment.width / 2;
          const base = {
            id: segment.key,
            kind: "glass" as const,
            fillRule: "evenodd" as const,
            bounds: { x: ccx - outer, y: ccy - outer, width: outer * 2, height: outer * 2 },
            color: css(segment.tint),
            tone: segment.tone,
            fill: segment.fill,
            pattern: patternStart(segment.pattern),
            // Bevel and depth in the proportions of the approved donut, so the rim catches a
            // continuous highlight rather than a hairline.
            depth: settle(Math.max(segment.width * 0.7, 12)),
            bevel: settle(Math.min(5, segment.width * 0.27)),
          };
          if (segment.end - segment.start >= TAU - 1e-6) {
            // A complete ring: two circles, filled even-odd, with no sector hint, so the
            // lighting has no tip and the ring no seam where it closes. It sweeps in from its
            // start, then drops the sweep once full.
            return {
              ...base,
              path: sectorPath(ccx, ccy, inner, outer, segment.start, segment.start + TAU),
              fillOrigin:
                segment.fill < 0.999
                  ? {
                      cx: ccx,
                      cy: ccy,
                      start: segment.start,
                      end: segment.start + TAU,
                      clockwise: true,
                    }
                  : undefined,
            };
          }
          const [a0, a1] = trimmed(segment.start, segment.end, gap, segment.radius);
          const corner = segment.round ? segment.width / 2 : bevel;
          return {
            ...base,
            path: sectorPath(ccx, ccy, inner, outer, a0, a1, corner),
            fillOrigin: { cx: ccx, cy: ccy, start: a0, end: a1, clockwise: true },
            field: segment.field,
            shape: {
              kind: "sector",
              cx: ccx,
              cy: ccy,
              innerRadius: inner,
              outerRadius: outer,
              startAngle: a0,
              endAngle: a1,
              // The path is this sector with `corner` rounds (sectorPath): no mask needed.
              corner,
              exact: true,
            },
          };
        }),
      });
      if (lostContext(result)) return "lost";
      return result.available ? "drawn" : "unavailable";
    },
  };
}
