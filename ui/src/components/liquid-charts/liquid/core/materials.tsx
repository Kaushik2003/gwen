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

import { createContext, useContext, useEffect, type ComponentType, type ReactNode } from "react";
import type { BarFrame, BarMark } from "./mark-types";
import type { RingGlass } from "./ring-types";
import type { Theme } from "./colour";
import type { BarView } from "./camera";

/*
 * The materials a chart can draw in. The core draws flat and light; glass, liquid and tilt come in
 * a material pack (`liquid-core-pro`, free with the Liquid credit), handed down through
 * `MaterialsProvider`. Without a pack every chart draws in the core's own materials, and a chart
 * that asks for glass outright says so once in development.
 *
 * The core never loads a pack itself: a pack is passed in, by `<LiquidMaterials>` from the
 * materials or by a chart that brings its own.
 */

/** Where every mark should be for the plot's current size, in plot px. */
export type MarkLayout = (width: number, height: number) => BarMark[];
/** Every mark as drawn this frame, for labels and tooltips that follow them. */
export type MarkFrames = (bars: BarFrame[], width: number, height: number) => void;

/** What every bar mark layer takes, flat or glass. */
export type LayerProps = {
  theme: Theme;
  /** Change to replay the entrance. */
  replay?: number;
  layout: MarkLayout;
  onFrame?: MarkFrames;
  /** A class for the layer's box (its stacking, say). */
  className?: string;
};

/** How the glass is cut: its depth (a multiple of the thinnest mark), its bevel, px, and how far
 * the canvas reaches past the plot, px, so the glow fades before it ends (thin marks need more
 * than the default 24). */
export type GlassCut = { depth?: number; bevel?: number; bleed?: number };

/** A glass layer for bars: what a pack's `BarLayer` takes. */
export type GlassLayerProps = LayerProps &
  GlassCut & {
    /** Once, when the glass can't draw here (no WebGL), so the chart draws flat instead. */
    onUnavailable?: () => void;
    /** When the browser restores a lost canvas (a GPU reset), which leaves it blank. */
    onRestored?: () => void;
    /** Kept mounted (and drawing) but not shown, while a flat layer stands in. */
    hidden?: boolean;
    /** Straight on, or columns at a slant (their sides showing); flat marks are always straight on. */
    view?: BarView;
  };

/** A mark's liquid as a layer drives it: its front (px along the mark) chases where the level
 * should be. The glass pack's is simulated: it lags, sags when the mark is pressed, splashes off a full mark's
 * end and moves point by point across it (`rows`, the surface the glass reads). */
export type MarkLiquid = {
  readonly front: number;
  reset(): void;
  settle(rest: number): void;
  press(across?: number, thickness?: number): void;
  step(
    dt: number,
    input: {
      target: number;
      thickness: number;
      depth: number;
      ceiling?: number;
      pointer?: { s: number; push: number } | null;
    },
  ): boolean;
  rows(out: Float32Array, offset?: number, rest?: number): void;
  /** The hardest the level landed, and splashed, since the last read. */
  takeImpact(): Readonly<{ land: number; splash: number }>;
};

/** The materials beyond the core's own. Every part is optional; a chart draws flat where its
 * pack has nothing. */
export type MaterialPack = {
  /** Named in development notes. */
  name: string;
  /** Bars, columns, stacks, capsules and tiles: the glass layer `MarkLayer` mounts in place of
   * the flat one. */
  BarLayer?: ComponentType<GlassLayerProps>;
  /** Rings, sectors and tracks: glass on the ring's canvas (null where it can't draw). */
  ring?: (canvas: HTMLCanvasElement) => RingGlass | null;
  /** What a chart drawn with this pack shows beside its plot: the Liquid credit, which the
   * source-available materials carry and a Pro build leaves empty. */
  Credit?: ComponentType;
};

/** The core alone: flat and light. */
export const FREE_MATERIALS: MaterialPack = { name: "Liquid Charts" };

const Materials = createContext<MaterialPack>(FREE_MATERIALS);

/** Hands a material pack to every chart inside. The materials (`liquid-core-pro`) wrap this as
 * `<LiquidMaterials>`. */
export function MaterialsProvider({ pack, children }: { pack: MaterialPack; children: ReactNode }) {
  return <Materials.Provider value={pack}>{children}</Materials.Provider>;
}

/** The material pack in reach: the provider's, or the core alone. */
export const useMaterials = () => useContext(Materials);

const noted = new Set<string>();

/** Once per page and material, outside production: a chart asked for a material (`material="glass"`,
 * a tilted view) that this project doesn't have, and draws in the core's instead. Call it from an
 * effect. */
export function noteMissingMaterial(material: "glass" | "liquid" | "tilt") {
  if (process.env.NODE_ENV === "production" || noted.has(material)) return;
  noted.add(material);
  const instead = material === "tilt" ? "draws untilted" : "draws flat";
  console.info(
    `Liquid Charts: ${material} comes with the materials (liquid-core-pro, free), so this chart ${instead}. Install them and wrap the app in <LiquidMaterials>: https://liquid.ojieame.design/start`,
  );
}

/** Notes once, in development, that a chart asked outright (`asked`) for a material it can't draw
 * here because no pack brings it (`has` false). */
export function useMaterialNote(
  material: "glass" | "liquid" | "tilt",
  asked: boolean,
  has: boolean,
) {
  useEffect(() => {
    if (asked && !has) noteMissingMaterial(material);
  }, [material, asked, has]);
}
