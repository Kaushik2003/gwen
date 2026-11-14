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
import type { ChartOpticalMark, OpticalPoint } from "./chart-optical-types";

export type OpticalField = {
  pixels: Uint8Array;
  width: number;
  height: number;
  x: number;
  y: number;
  cssWidth: number;
  cssHeight: number;
  scale: number;
  range: number;
  ink: boolean;
  /** 4: distance (RG, 16 bits), coverage (B) and depth along the run (A). 2: distance only, for a
   * shader that reads no depth and takes coverage from the distance (half the bytes). */
  channels: 2 | 4;
};
const INF = 1e20;
/** Squared Euclidean distance transform, linear in pixel count. */
export function opticalDistanceTransform(grid: Float32Array, width: number, height: number) {
  const size = Math.max(width, height),
    f = new Float64Array(size),
    d = new Float64Array(size);
  const v = new Int32Array(size),
    z = new Float64Array(size + 1);
  function line(n: number) {
    let k = 0;
    v[0] = 0;
    z[0] = -INF;
    z[1] = INF;
    for (let q = 1; q < n; q++) {
      let s = (f[q] + q * q - (f[v[k]] + v[k] * v[k])) / (2 * (q - v[k]));
      while (s <= z[k] && k > 0) {
        k--;
        s = (f[q] + q * q - (f[v[k]] + v[k] * v[k])) / (2 * (q - v[k]));
      }
      k++;
      v[k] = q;
      z[k] = s;
      z[k + 1] = INF;
    }
    k = 0;
    for (let q = 0; q < n; q++) {
      while (z[k + 1] < q) k++;
      d[q] = (q - v[k]) ** 2 + f[v[k]];
    }
  }
  for (let x = 0; x < width; x++) {
    for (let y = 0; y < height; y++) f[y] = grid[y * width + x];
    line(height);
    for (let y = 0; y < height; y++) grid[y * width + x] = d[y];
  }
  for (let y = 0; y < height; y++) {
    for (let x = 0; x < width; x++) f[x] = grid[y * width + x];
    line(width);
    for (let x = 0; x < width; x++) grid[y * width + x] = d[x];
  }
}

export function opticalFieldFromAlpha(
  alpha: Uint8ClampedArray | Uint8Array,
  width: number,
  height: number,
  scale = 1,
  range = 256,
  horizontal = false,
  reverse = false,
  channels: 2 | 4 = 4,
) {
  const inside = new Float32Array(width * height),
    outside = new Float32Array(width * height);
  const pixels = new Uint8Array(width * height * channels);
  let ink = false;
  for (let i = 0; i < inside.length; i++) {
    const a = alpha[i];
    inside[i] = a >= 128 ? 0 : INF;
    outside[i] = a >= 128 ? INF : 0;
    if (channels === 4) pixels[i * 4 + 2] = a;
    ink ||= a > 0;
  }
  opticalDistanceTransform(inside, width, height);
  opticalDistanceTransform(outside, width, height);
  for (let i = 0; i < inside.length; i++) {
    let distance = (Math.sqrt(inside[i]) - Math.sqrt(outside[i])) / scale;
    if (alpha[i] > 0 && alpha[i] < 255) distance = (0.5 - alpha[i] / 255) / scale;
    const encoded = Math.round(Math.max(0, Math.min(1, distance / (2 * range) + 0.5)) * 65535);
    pixels[i * channels] = encoded >> 8;
    pixels[i * channels + 1] = encoded & 255;
  }
  if (channels === 2) return { pixels, ink };
  // Local depth per connected run; each hole/disconnected band resets its own depth.
  const crossCount = horizontal ? height : width,
    alongCount = horizontal ? width : height;
  const index = (cross: number, along: number) =>
    horizontal ? cross * width + along : along * width + cross;
  for (let cross = 0; cross < crossCount; cross++) {
    let along = 0;
    while (along < alongCount) {
      while (along < alongCount && alpha[index(cross, along)] <= 16) along++;
      const first = along;
      while (along < alongCount && alpha[index(cross, along)] > 16) along++;
      const last = along;
      for (let position = first; position < last; position++) {
        const depth = (position - first + 0.5) / Math.max(1, last - first);
        pixels[index(cross, position) * 4 + 3] = Math.round((reverse ? 1 - depth : depth) * 255);
      }
    }
  }
  return { pixels, ink };
}
function boundaryAt(points: readonly OpticalPoint[], cross: number, horizontal: boolean) {
  if (!points.length) return NaN;
  const position = (point: OpticalPoint) => (horizontal ? point.y : point.x);
  const value = (point: OpticalPoint) => (horizontal ? point.x : point.y);
  if (cross <= position(points[0])) return value(points[0]);
  for (let i = 1; i < points.length; i++)
    if (position(points[i]) >= cross) {
      const a = points[i - 1],
        b = points[i];
      return (
        value(a) +
        (value(b) - value(a)) *
          ((cross - position(a)) / Math.max(0.00001, position(b) - position(a)))
      );
    }
  return value(points[points.length - 1]);
}

/** Analytic contours refine a single native band; they never merge separate raster runs. */
function refineContourDepth(
  pixels: Uint8Array,
  alpha: Uint8Array,
  width: number,
  height: number,
  x: number,
  y: number,
  scale: number,
  upper: readonly OpticalPoint[],
  lower: readonly OpticalPoint[],
  horizontal: boolean,
  reverse: boolean,
) {
  const crossCount = horizontal ? height : width;
  const alongCount = horizontal ? width : height;
  const crossOrigin = horizontal ? y : x;
  const alongOrigin = horizontal ? x : y;
  const index = (cross: number, along: number) =>
    horizontal ? cross * width + along : along * width + cross;
  for (let cross = 0; cross < crossCount; cross++) {
    const position = crossOrigin + (cross + 0.5) / scale;
    const start = boundaryAt(upper, position, horizontal);
    const end = boundaryAt(lower, position, horizontal);
    if (!Number.isFinite(start + end) || Math.abs(end - start) < 0.00001) continue;
    let runs = 0,
      first = 0,
      last = 0,
      along = 0;
    while (along < alongCount) {
      while (along < alongCount && alpha[index(cross, along)] <= 16) along++;
      if (along === alongCount) break;
      first = along;
      while (along < alongCount && alpha[index(cross, along)] > 16) along++;
      last = along;
      if (++runs > 1) break;
    }
    const low = Math.min(start, end),
      high = Math.max(start, end);
    const tolerance = 1.5 / scale;
    const matches =
      runs === 1 &&
      Math.abs(low - (alongOrigin + first / scale)) <= tolerance &&
      Math.abs(high - (alongOrigin + last / scale)) <= tolerance;
    if (matches) {
      for (let along = first; along < last; along++) {
        const depth = Math.max(
          0,
          Math.min(1, (alongOrigin + (along + 0.5) / scale - start) / (end - start)),
        );
        pixels[index(cross, along) * 4 + 3] = Math.round((reverse ? 1 - depth : depth) * 255);
      }
    } else if (end < start) {
      // Oriented contours can reverse a band, but cannot replace its separate run boundaries.
      for (let along = 0; along < alongCount; along++) {
        const i = index(cross, along);
        if (alpha[i] > 16) pixels[i * 4 + 3] = 255 - pixels[i * 4 + 3];
      }
    }
  }
}

/** The texel grid a mark's field covers: its bounds and 48px for the glow, at up to 2 texels per
 * px. Exact shapes are evaluated in the shader on this same grid, so they match a built mask. */
export function opticalFieldGrid(bounds: ChartOpticalMark["bounds"], ratio: number) {
  const padding = 48,
    x = Math.floor(bounds.x - padding),
    y = Math.floor(bounds.y - padding);
  const cssWidth = Math.ceil(bounds.width + padding * 2),
    cssHeight = Math.ceil(bounds.height + padding * 2);
  const scale = Math.min(2, Math.max(1, ratio), 1536 / cssWidth, 1536 / cssHeight);
  if (!(scale >= 0.25) || !(cssWidth > 0) || !(cssHeight > 0)) throw new Error("mask-bounds");
  const width = Math.max(1, Math.ceil(cssWidth * scale)),
    height = Math.max(1, Math.ceil(cssHeight * scale));
  return { x, y, scale, width, height, cssWidth: width / scale, cssHeight: height / scale };
}

export function rasterizeOpticalField(
  mark: ChartOpticalMark,
  canvas: HTMLCanvasElement,
  ratio: number,
  channels: 2 | 4 = 4,
): OpticalField {
  const { x, y, scale, width, height } = opticalFieldGrid(mark.bounds, ratio);
  const cssWidth = Math.ceil(mark.bounds.width + 96),
    cssHeight = Math.ceil(mark.bounds.height + 96);
  if (canvas.width !== width) canvas.width = width;
  if (canvas.height !== height) canvas.height = height;
  const ctx = canvas.getContext("2d", { willReadFrequently: true });
  if (!ctx) throw new Error("mask-context");
  const set = (matrix?: readonly number[]) => {
    ctx.setTransform(scale, 0, 0, scale, -x * scale, -y * scale);
    if (matrix) ctx.transform(matrix[0], matrix[1], matrix[2], matrix[3], matrix[4], matrix[5]);
  };
  set();
  ctx.clearRect(x, y, cssWidth, cssHeight);
  ctx.save();
  if (mark.clip) {
    ctx.beginPath();
    ctx.rect(mark.clip.x, mark.clip.y, mark.clip.width, mark.clip.height);
    ctx.clip();
  }
  for (const clip of mark.clips ?? []) {
    set(clip.transform);
    ctx.clip(typeof clip.path === "string" ? new Path2D(clip.path) : clip.path);
  }
  set(mark.transform);
  ctx.fillStyle = "white";
  ctx.fill(
    typeof mark.path === "string" ? new Path2D(mark.path) : mark.path,
    mark.fillRule ?? "nonzero",
  );
  ctx.restore();
  const image = ctx.getImageData(0, 0, width, height),
    alpha = new Uint8Array(width * height);
  for (let i = 0; i < alpha.length; i++) alpha[i] = image.data[i * 4 + 3];
  const range = 256;
  const horizontal = mark.kind === "area" && mark.depthAxis === "x";
  const reverse =
    mark.kind === "area" &&
    (mark.depthReverse ?? (horizontal ? mark.fillOrigin === "left" : mark.fillOrigin === "top"));
  const { pixels, ink } = opticalFieldFromAlpha(
    alpha,
    width,
    height,
    scale,
    range,
    horizontal,
    reverse,
    channels,
  );
  if (channels === 4 && mark.kind === "area" && mark.upper?.length && mark.lower?.length)
    refineContourDepth(
      pixels,
      alpha,
      width,
      height,
      x,
      y,
      scale,
      mark.upper,
      mark.lower,
      horizontal,
      reverse,
    );

  return {
    pixels,
    width,
    height,
    x,
    y,
    cssWidth: width / scale,
    cssHeight: height / scale,
    scale,
    range,
    ink,
    channels,
  };
}
