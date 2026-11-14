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
/** Exact native geometry and optical state. No chart scales or interaction are owned here. */
export type OpticalMatrix = readonly [number, number, number, number, number, number];
export type OpticalBounds = { x: number; y: number; width: number; height: number };
export type OpticalPoint = { x: number; y: number };
/** `exact`: the mark's path traces this shape exactly (a rect with its corners rounded by `radius`,
 * a sector with its corners rounded by `corner`, clamped as core `sectorPath` clamps them), so the
 * renderer evaluates its distance field in the shader and builds no mask: moving, growing or
 * hovered, it costs nothing on the CPU. Without it the shape only lights the glass and the path's
 * mask is the geometry. Ignored for areas, clipped marks, and a pie (no inner radius) with rounded
 * corners. */
export type OpticalShape =
  | ({ kind: "rect"; radius?: number; exact?: boolean } & OpticalBounds)
  | { kind: "circle"; cx: number; cy: number; r: number; exact?: boolean }
  | {
      kind: "sector";
      cx: number;
      cy: number;
      innerRadius: number;
      outerRadius: number;
      /** From the sweep's start to its end: an end before the start sweeps anticlockwise. */
      startAngle: number;
      endAngle: number;
      /** Where light enters strongest: `end` (default) at the end of the sweep, as a ring
       * segment's value end; `outer` along the outer arc, as a rose slice's. `fillRadial`
       * implies `outer`. */
      tip?: "end" | "outer";
      corner?: number;
      exact?: boolean;
    };
export type OpticalFillOrigin =
  | "bottom"
  | "top"
  | "left"
  | "right"
  | { cx: number; cy: number; start: number; end: number; clockwise?: boolean };
/** A fill that grows from a centre outward, from `inner` (default 0) to `outer` px from (cx, cy). */
export type OpticalRadialFill = { cx: number; cy: number; inner?: number; outer: number };
export type ChartOpticalMark = {
  id: string;
  path: string | Path2D;
  bounds: OpticalBounds;
  color: string;
  kind: "glass" | "area";
  /** Give a path in local coordinates and its place as this matrix's translation (and its size
   * as a uniform scale), and its mask survives motion: a move reuses it, and a mark growing by a
   * uniform scale reuses one mask per 2px of size; the exact mask is rebuilt once it holds still. */
  transform?: OpticalMatrix;
  fillRule?: CanvasFillRule;
  opacity?: number;
  tone?: number;
  fill?: number;
  fillOrigin?: OpticalFillOrigin;
  /** Glass only: the liquid grows from the centre outward instead (a rose slice, a radar's
   * polygon), and takes its tip light at its outer edge. Overrides `fillOrigin`. */
  fillRadial?: OpticalRadialFill;
  shape?: OpticalShape;
  /** Local thickness axis, independent of entrance/flow direction. */
  depthAxis?: "x" | "y";
  depthReverse?: boolean;
  /** Optional oriented depth boundaries in scene pixels, sorted by cross-axis
   * (x for vertical thickness; y for horizontal). Used only where they match one
   * native raster run. Swapping boundaries reverses depth; depthReverse inverts it. */
  upper?: readonly OpticalPoint[];
  lower?: readonly OpticalPoint[];
  clip?: OpticalBounds;
  clips?: readonly { path: string | Path2D; transform?: OpticalMatrix }[];
  focus?: { x: number; y: number; amount: number };
  palette?: readonly string[];
  colors?: readonly { offset: number; color: string }[];
  colorDirection?: "horizontal" | "vertical";
  hatch?: boolean;
  /** Glass only: etch a pattern from this share (0–1) of the mark's fill direction to its end,
   * matching the raymarched renderer. */
  pattern?: number;
  /** Glass only: up to four engraved reference marks, as shares (0–1) of the fill direction. */
  marks?: readonly number[];
  /** Geometry fingerprint only. Must change when path/clip/transform changes. */
  cacheKey?: string;
  depth?: number;
  bevel?: number;
  /** Glass only: scales the halo and the drop shadow (default 1); dense marks use less so
   * their glows don't add up to a haze. */
  glow?: number;
  /** Glass only: the liquid's simulated surface (see OPTICAL_SURFACE in chart-optical-shader);
   * without it the liquid's front is flat. */
  field?: Float32Array;
};
export type ChartOpticalScene = {
  width: number;
  height: number;
  /** Device px per CSS px, at most 2. Callers pass the registry's `pixelRatio()`, so an export's
   * scale reaches the optical glass; without it, the screen's. */
  dpr?: number;
  theme: "dark" | "light";
  marks: readonly ChartOpticalMark[];
  reducedMotion?: boolean;
};
export type ChartOpticalResult = {
  available: boolean;
  visible: boolean;
  rendered: string[];
  fallback: { id: string; reason: string }[];
};
