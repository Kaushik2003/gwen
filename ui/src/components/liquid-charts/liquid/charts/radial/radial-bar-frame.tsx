"use client";
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

import { resolveStyle, type Density, type View } from "@/components/liquid-charts/liquid/core/style";
import { useLayoutEffect, useRef, type CSSProperties } from "react";
import type { GlassShape, LayerClock } from "./use-radial-layer";
import { cssModule } from "@/components/liquid-charts/liquid/core/css-module";
import type { Rgb } from "@/components/liquid-charts/liquid/core/colour";
import { sectorBounds, type TrackGlass } from "./layout";
import { sectorPath } from "@/components/liquid-charts/liquid/core/arc";
import Styles from "./radial.module.css";
import { easeInOut } from "./motion";

const styles = cssModule(Styles, "lc-radial-");

export function radialBarStyle({
  count,
  density,
  material,
  heightMeaning = "none",
  view,
}: {
  count: number;
  density: Density;
  material: "glass" | "flat";
  heightMeaning?: "value" | "second-measure" | "none";
  view?: View;
}) {
  return resolveStyle({
    geometry: "circular",
    marks: ["solid", "reference"],
    measure: "amount",
    segments: count,
    density,
    material,
    ...(heightMeaning === "none" ? {} : { heightMeaning, view }),
  });
}

/** Already laid-out native geometry. Values/domain/order remain the caller's layout work. */
export type RadialBarFrameInput = {
  key: string;
  position: number;
  cx: number;
  cy: number;
  radius: number;
  width: number;
  start: number;
  end: number;
  /** Native bars sweep clockwise from `start`; an adapter's host may run the other way (a radial
   * bar chart from 0° to 360°), its `end` then before its `start`. Default true. */
  clockwise?: boolean;
  tint: Rgb;
  lit: boolean;
  /** Native bars default to full rounds; an adapter supplies the observed host corner. */
  corner?: number;
  targetAngle?: number | null;
};

/** The original RadialBars flat frame policy, shared by the chart and geometry adapters.
 * Spring names, entrance/stagger, cutoffs, morph, target etching and optical constants are
 * unchanged. The caller owns the existing radial layer clock and tilt handoff state. */
export function radialBarFrame(
  { motion, time }: Pick<LayerClock, "motion" | "time">,
  bars: readonly RadialBarFrameInput[],
  { tilted = false, handed = false }: { tilted?: boolean; handed?: boolean } = {},
) {
  const morph = motion.spring("morph", tilted || handed ? 1 : 0, { rest: 0.002 });
  const shapes: GlassShape[] = [];
  const glass: TrackGlass[] = [];
  const ends: number[] = [];
  const fills: number[] = [];
  let entering = false;
  bars.forEach((bar, i) => {
    const { key, position: p, cx, cy, radius: r, width: w, start } = bar;
    const end = motion.spring(`end:${key}`, bar.end, { rest: 1e-4 });
    const tone = motion.ease(`tone:${key}`, bar.lit ? 1 : 0);
    const fill = easeInOut((time - 0.2 - p * 0.09) / 1.2);
    if (fill < 1) entering = true;
    // The bar's end on its own side of `start` (+1 clockwise, -1 the other way), whatever the
    // spring overshoots; the sector drawn is the interval between them, filled from `start`.
    const turn = bar.clockwise === false ? -1 : 1;
    const a1 = turn > 0 ? Math.max(start, end) : Math.min(start, end);
    const span = (a1 - start) * turn;
    const filled = start + (a1 - start) * fill;
    const from = Math.min(start, a1),
      to = Math.max(start, a1);
    ends[i] = a1;
    fills[i] = fill;
    glass[p] = { radius: r, width: w, start, end: filled };
    if (span < 1e-3 || handed) return;
    const inner = r - w / 2,
      outer = r + w / 2;
    const round = bar.corner ?? w / 2;
    const corner = round + (Math.min(3, w * 0.2) - round) * morph;
    const targetAngle = bar.targetAngle ?? null;
    const along = targetAngle === null ? null : (targetAngle - start) * turn;
    const marks =
      along !== null && along > 1e-4 && along <= span + 1e-6 ? [along / span] : undefined;
    shapes.push({
      id: key,
      path: sectorPath(cx, cy, inner, outer, from, to, corner),
      bounds: sectorBounds(cx, cy, inner, outer, from, to),
      tint: bar.tint,
      tone,
      fill,
      sweep: { cx, cy, start, end: a1, ...(turn < 0 && { clockwise: false }) },
      // From `start` to the tip, so the glass lights the tip at the sweep's end either way.
      sector: { cx, cy, inner, outer, start, end: a1, corner },
      marks,
      depth: Math.max(w * 0.7, 10),
      bevel: Math.min(5, w * 0.27),
      flat:
        fill <= 0.001
          ? ""
          : sectorPath(
              cx,
              cy,
              inner,
              outer,
              Math.min(start, filled),
              Math.max(start, filled),
              corner,
            ),
    });
  });
  return {
    shapes,
    glass,
    ends,
    fills,
    morph,
    handoff: tilted && !handed && morph >= 0.998,
    flatShown: morph < 0.02 && !handed,
    moving: entering || (!tilted && handed),
  };
}

export type RadialBarTrackInput = {
  cx: number;
  cy: number;
  inner: number;
  outer: number;
  start: number;
  end: number;
  corner?: number;
};

export function radialBarTrackPath(sector: RadialBarTrackInput) {
  const { cx, cy, inner, outer, start, end, corner = (outer - inner) / 2 } = sector;
  return sectorPath(cx, cy, inner, outer, start, end, corner);
}

/** The original etched track, shared without copying its colour/focus CSS into adapters. */
export function RadialBarTrack({
  sector,
  colour,
  active,
  onPaint,
}: {
  sector: RadialBarTrackInput;
  colour: string;
  active: boolean;
  /** A committed SVG path, independent of the optical foreground layer's receipt. */
  onPaint?: (painted: boolean) => void;
}) {
  const ref = useRef<SVGPathElement>(null);
  const path = radialBarTrackPath(sector);
  useLayoutEffect(() => {
    if (!onPaint) return;
    onPaint(!!ref.current?.isConnected && ref.current.getAttribute("d") === path);
    return () => onPaint(false);
  }, [path, colour, active, onPaint]);
  return (
    <path
      ref={ref}
      className={styles.track}
      data-active={active}
      style={{ "--track": colour } as CSSProperties}
      d={path}
    />
  );
}
