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

import {
  noteMissingMaterial,
  useMaterials,
  type GlassCut,
  type GlassLayerProps,
  type LayerProps,
} from "./materials";
import type { BarView } from "./camera";
import { useCallback, useEffect, useRef, useState, type ComponentType, type Ref } from "react";
import Styles from "./mark-layers.module.css";
import { useFlatMarks } from "./flat-marks";
import { cssModule } from "./css-module";

const styles = cssModule(Styles, "lc-mark-layers-");

export type { MarkFrames, MarkLayout } from "./materials";

/* The mark layers every bar-like chart draws through: glass where a material pack brings it and
 * WebGL and the marks' size allow, flat SVG otherwise. Both paths take one `layout(width, height)`
 * and report the same `onFrame`, so labels, tooltips and axes follow the marks the same way on
 * either. */

/** The box a mark hook draws into: a layer over the plot holding the hook's canvas (glass) or
 * svg (flat). For a family with its own mark hook (light lines, say) as much as for the bars. */
export function MarkSurface({
  box,
  canvas,
  svg,
  material,
  hidden = false,
  className,
}: {
  box: Ref<HTMLDivElement>;
  canvas?: Ref<HTMLCanvasElement>;
  svg?: Ref<SVGSVGElement>;
  /** Named on the canvas, for export's note on what SVG can't express. */
  material?: string;
  /** Kept mounted (and drawing) but not shown. */
  hidden?: boolean;
  className?: string;
}) {
  return (
    <div
      ref={box}
      className={className ? `${styles.layer} ${className}` : styles.layer}
      data-hidden={hidden || undefined}
    >
      {canvas && (
        <canvas
          ref={canvas}
          className={styles.canvas}
          data-material={material}
          aria-hidden="true"
        />
      )}
      {svg && <svg ref={svg} className={styles.svg} aria-hidden="true" />}
    </div>
  );
}

/** The flat path: the same marks as SVG, with the same springs and entrance. */
export function FlatLayer({
  handover,
  radius,
  className,
  ...props
}: LayerProps & {
  /** Read once on mount: true when this layer takes over marks the glass already drew, so it
   * skips the entrance. */
  handover?: { readonly current: boolean };
  /** Corner radius, px. */
  radius?: number;
}) {
  const { box, svg } = useFlatMarks({ ...props, radius, entrance: handover });
  return <MarkSurface box={box} svg={svg} className={className} />;
}

export type FlatLayerProps = Parameters<typeof FlatLayer>[0];

/**
 * Glass marks where there is glass, or flat ones, from one layout.
 *
 * - The glass comes from `Glass`, or else from the material pack in reach (`useMaterials`).
 *   Without either, the marks draw flat.
 * - `flat`: draw flat for good (the material or the style says so outright). The glass never mounts.
 * - `fallback`: draw flat for now (marks too thin at this width). The glass stays mounted and
 *   drawing underneath, hidden, so widening again shows it at once instead of replaying its pour.
 * - `asked`: the chart asked for glass outright (its `material` prop), so drawing flat for want of
 *   a pack is worth a note in development.
 * - `view`: columns at a slant, their sides showing. Only the glass draws it; flat marks stay
 *   straight on, and asking without a pack is worth a note too.
 *
 * Flat taking over marks the glass already drew skips its entrance. A restored canvas (after a GPU
 * reset) remounts the glass, since the renderer skips redrawing a scene it thinks is on screen.
 */
export function MarkLayer({
  flat = false,
  fallback = false,
  asked = false,
  depth,
  bevel,
  bleed,
  flatRadius,
  onUnavailable,
  view = "flat",
  Flat = FlatLayer,
  Glass,
  ...layer
}: LayerProps &
  GlassCut & {
    flat?: boolean;
    fallback?: boolean;
    asked?: boolean;
    /** The flat marks' corner radius, px. */
    flatRadius?: number;
    /** Straight on, or columns at a slant (glass only). */
    view?: BarView;
    /** Once, when WebGL turns out to be missing (the marks then draw flat). */
    onUnavailable?: () => void;
    /** The flat path, for a family whose flat marks draw differently. */
    Flat?: ComponentType<FlatLayerProps>;
    /** The glass path, for a chart that brings its own (the families that draw in glass pass liquid-core-pro's). */
    Glass?: ComponentType<GlassLayerProps>;
  }) {
  const pack = useMaterials();
  const GlassMarks = Glass ?? pack.BarLayer;
  const [unavailable, setUnavailable] = useState(false);
  const [generation, setGeneration] = useState(0);
  const glassShown = useRef(false);
  const markUnavailable = useCallback(() => {
    setUnavailable(true);
    onUnavailable?.();
  }, [onUnavailable]);
  const restored = useCallback(() => setGeneration((g) => g + 1), []);
  const { onFrame } = layer;
  useEffect(() => {
    if (asked && !GlassMarks) noteMissingMaterial("glass");
    if (view !== "flat" && !GlassMarks) noteMissingMaterial("tilt");
  }, [asked, view, GlassMarks]);
  const glass = Boolean(GlassMarks) && !flat && !unavailable;
  return (
    <>
      {glass && GlassMarks && (
        <GlassMarks
          key={generation}
          theme={layer.theme}
          replay={layer.replay}
          layout={layer.layout}
          className={layer.className}
          depth={depth}
          bevel={bevel}
          bleed={bleed}
          view={view}
          hidden={fallback}
          onFrame={
            fallback
              ? undefined
              : (bars, width, height) => {
                  if (bars.length) glassShown.current = true;
                  onFrame?.(bars, width, height);
                }
          }
          onUnavailable={markUnavailable}
          onRestored={restored}
        />
      )}
      {(!glass || fallback) && <Flat {...layer} radius={flatRadius} handover={glassShown} />}
    </>
  );
}
