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
import {
  easeOut,
  ENTER,
  resolveStyle,
  type Density,
  type LightLine,
  type Rgb,
} from "@/components/liquid-charts/liquid/core";
import type { GlassShape, LayerClock, LayerScene } from "./use-radial-layer";
import { polar, polygonArea, radarPath } from "./layout";

/** Already laid out radii: the caller owns its scale and the meaning of missing vertices. */
export type RadarFrameSeries = {
  key: string;
  centre: { x: number; y: number };
  angles: readonly number[];
  radii: readonly number[];
  tint: Rgb;
  hidden?: boolean;
  fill?: boolean;
  /** Omit for the canonical chart's existing index-based spring identities. */
  pointKeys?: readonly string[];
  /** Original chart membership index, including hidden series, when known. */
  entranceIndex?: number;
};

export type RadarFrameInput = {
  series: readonly RadarFrameSeries[];
  lead: string | null;
  /** Complete visible membership, including host-retained marks with no native geometry. */
  visibleSeries?: number;
};
export type RadarFrameResult = {
  scene: LayerScene;
  /** Native focus-marker radii, including entrance bloom, in input spoke order. */
  drawn: Map<string, number[]>;
};

/** The native chart's fill/count and material resolution, shared with adapters. */
export const radarFilled = (fill: boolean, visibleSeries: number) => fill && visibleSeries <= 2;

export function resolveRadarStyle({
  fill = true,
  series,
  density = "standard",
  material = "flat",
}: {
  fill?: boolean;
  series: number;
  density?: Density;
  material?: "glass" | "flat";
}) {
  return resolveStyle({
    geometry: "polygon",
    marks: radarFilled(fill, series) ? ["solid", "stroke", "reference"] : ["stroke", "reference"],
    series,
    density,
    material,
  });
}

/** Extracted from Radar's drawFrame. This is the single owner of its springs,
 * stagger, radial fill, area/focus ordering, depth, bevel and light presence.
 * The existing radial layer owns the clock, reduced motion and surface lifecycle. */
export function drawRadarFrame(
  clock: LayerClock,
  { series, lead, visibleSeries }: RadarFrameInput,
): RadarFrameResult {
  const { motion, time, engine } = clock;
  const visible = series.filter((item) => !item.hidden);
  const count = visibleSeries ?? visible.length;
  let entering = false;
  const current = new Map<string, number[]>();
  const scales = new Map<string, number>();
  series.forEach((item, s) => {
    const { key } = item;
    const radii = item.radii.map((target, i) =>
      Number.isFinite(target)
        ? motion.spring(
            item.pointKeys ? `r:${JSON.stringify([key, item.pointKeys[i]])}` : `r:${key}:${i}`,
            target,
            { rest: 0.05 },
          )
        : target,
    );
    const bloom = easeOut((time - ENTER.delay - (item.entranceIndex ?? s) * 0.12) / ENTER.mark);
    if (bloom < 1) entering = true;
    current.set(key, radii);
    scales.set(key, bloom);
  });
  const drawn = new Map(
    [...current].map(([key, radii]) => [key, radii.map((r) => r * (scales.get(key) ?? 1))]),
  );
  const tones = Object.fromEntries(
    series.map(({ key, hidden }) => [
      key,
      motion.ease(`tone:${key}`, hidden ? 0 : lead === null || lead === key ? 1 : 0),
    ]),
  );
  const shapes: GlassShape[] = [];
  const lines: LightLine[] = [];
  // The larger outline underneath, so the smaller sits on it whole; focus on top.
  const order = visible
    .filter((item) => radarFilled(item.fill !== false, count))
    .sort((a, b) => {
      if (a.key === lead) return 1;
      if (b.key === lead) return -1;
      return (
        polygonArea(b.angles, current.get(b.key)!) - polygonArea(a.angles, current.get(a.key)!)
      );
    });
  for (const item of order) {
    const { key, centre, angles, tint } = item;
    const radii = current.get(key)!;
    const bloom = scales.get(key) ?? 1;
    // Glass keeps its outline and fills from the centre out; flat, the outline blooms.
    const scale = engine === "optical" ? 1 : bloom;
    const path = radarPath(centre.x, centre.y, angles, radii, scale);
    if (!path) continue;
    const points = angles
      .map((a, i) =>
        Number.isFinite(radii[i]) ? polar(centre.x, centre.y, radii[i] * scale, a) : null,
      )
      .filter((p): p is { x: number; y: number } => p !== null);
    const xs = points.map((p) => p.x),
      ys = points.map((p) => p.y);
    const x = Math.min(...xs),
      y = Math.min(...ys);
    shapes.push({
      id: key,
      path,
      bounds: {
        x,
        y,
        width: Math.max(1, Math.max(...xs) - x),
        height: Math.max(1, Math.max(...ys) - y),
      },
      tint,
      tone: tones[key],
      fill: engine === "optical" ? bloom : 1,
      radial: {
        cx: centre.x,
        cy: centre.y,
        outer: Math.max(1, ...radii.filter((r) => Number.isFinite(r))),
      },
      opacity:
        engine === "svg"
          ? 0.26
          : order.length > 1 && key === order[order.length - 1].key
            ? 0.82
            : 1,
      depth: 14,
      bevel: 4,
    });
    if (engine === "svg")
      lines.push({ key, path, colour: tint, width: 1.75, presence: 0.32 + 0.83 * tones[key] });
  }
  const outlines: LightLine[] = [];
  for (const { key, centre, angles, tint, fill } of visible) {
    if (radarFilled(fill !== false, count)) continue;
    const path = radarPath(centre.x, centre.y, angles, current.get(key)!, scales.get(key) ?? 1);
    if (!path) continue;
    outlines.push({
      key,
      path,
      colour: tint,
      width: 2,
      presence: lead === null ? 1 : 0.32 + 0.83 * tones[key],
    });
  }
  // The one in focus is drawn last, over the rest.
  outlines.sort((a, b) => (a.key === lead ? 1 : b.key === lead ? -1 : 0));
  lines.push(...outlines);
  return { scene: { shapes, lines, moving: entering }, drawn };
}
