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
/**
 * Circle geometry shared by every round chart: angles are clockwise radians from 12 o'clock (the
 * glass renderer's convention), so a point at angle a and radius r sits at (cx + r·sin a,
 * cy − r·cos a). Plain functions, safe every frame and on the server. (From the ring family.)
 */

import { settle } from "./geometry";

export const TAU = Math.PI * 2;
export type Point = { x: number; y: number };

/** The point at `angle` and `radius` from (cx, cy). */
export const polar = (cx: number, cy: number, radius: number, angle: number): Point => ({
  x: cx + Math.sin(angle) * radius,
  y: cy - Math.cos(angle) * radius,
});
/** The angle wrapped into (−π, π]: the shortest signed turn. */
export const wrapAngle = (angle: number) => angle - TAU * Math.round(angle / TAU);
/** The angle wrapped into [0, 2π). */
export const normAngle = (angle: number) => ((angle % TAU) + TAU) % TAU;
/** The clockwise angle of a point seen from (cx, cy). */
export const angleOf = (cx: number, cy: number, x: number, y: number) =>
  normAngle(Math.atan2(x - cx, cy - y));

/** A segment's drawn angles once the padding between segments is taken out: `gap` px at
 * `radius`, half from each side, as the glass renderer trims its slabs. A segment narrower than
 * its padding collapses onto its middle. */
export function trimmed(start: number, end: number, gap: number, radius: number): [number, number] {
  const trim = radius > 0 ? gap / 2 / radius : 0;
  if (end - start <= trim * 2) {
    const mid = (start + end) / 2;
    return [mid, mid];
  }
  return [start + trim, end - trim];
}

const fmt = (value: number) => String(settle(value));

/**
 * An SVG path for a ring sector between radii `inner` and `outer`, from angle `a0` to `a1`, with
 * its four corners rounded to `corner` px (clamped so the rounds fit). An `inner` of 0 is a pie
 * wedge. A span of a whole turn draws the closed ring (use `fill-rule: evenodd`). Corners round
 * inside the sector's angles, so the path never reaches past [a0, a1].
 */
export function sectorPath(
  cx: number,
  cy: number,
  inner: number,
  outer: number,
  a0: number,
  a1: number,
  corner = 0,
): string {
  inner = Math.max(0, Math.min(inner, outer));
  const span = a1 - a0;
  const p = (r: number, a: number) => {
    const at = polar(cx, cy, r, a);
    return `${fmt(at.x)} ${fmt(at.y)}`;
  };
  if (outer <= 0 || span <= 1e-6) return "";
  if (span >= TAU - 1e-6) {
    const ring = (r: number, sweep: 0 | 1) =>
      `M${p(r, a0)}A${fmt(r)} ${fmt(r)} 0 1 ${sweep} ${p(r, a0 + Math.PI)}A${fmt(r)} ${fmt(r)} 0 1 ${sweep} ${p(r, a0)}Z`;
    return inner > 0 ? `${ring(outer, 1)}${ring(inner, 0)}` : ring(outer, 1);
  }
  const half = Math.min(span / 2, Math.PI / 2);
  const s = Math.sin(half);
  let k = Math.max(0, Math.min(corner, (outer - inner) / 2));
  // Each round must fit inside the sector: tangent to its arc and its radial edge.
  if (s < 1) k = Math.min(k, (outer * s) / (1 + s) - 0.01);
  if (inner > 0 && s < 1) k = Math.min(k, (inner * s) / (1 - s));
  k = Math.max(0, k);
  const large = (a: number) => (a > Math.PI ? 1 : 0);
  if (k < 0.05) {
    if (inner <= 0)
      return `M${fmt(cx)} ${fmt(cy)}L${p(outer, a0)}A${fmt(outer)} ${fmt(outer)} 0 ${large(span)} 1 ${p(outer, a1)}Z`;
    return `M${p(outer, a0)}A${fmt(outer)} ${fmt(outer)} 0 ${large(span)} 1 ${p(outer, a1)}L${p(inner, a1)}A${fmt(inner)} ${fmt(inner)} 0 ${large(span)} 0 ${p(inner, a0)}Z`;
  }
  const r = fmt(k);
  const dOut = outer - k,
    tOut = Math.asin(Math.min(1, k / dOut)),
    edgeOut = dOut * Math.cos(tOut);
  let path = `M${p(outer, a0 + tOut)}A${fmt(outer)} ${fmt(outer)} 0 ${large(span - 2 * tOut)} 1 ${p(outer, a1 - tOut)}A${r} ${r} 0 0 1 ${p(edgeOut, a1)}`;
  if (inner <= 0) {
    path += `L${fmt(cx)} ${fmt(cy)}`;
  } else {
    const dIn = inner + k,
      tIn = Math.asin(Math.min(1, k / dIn)),
      edgeIn = dIn * Math.cos(tIn);
    path += `L${p(edgeIn, a1)}A${r} ${r} 0 0 1 ${p(inner, a1 - tIn)}A${fmt(inner)} ${fmt(inner)} 0 ${large(span - 2 * tIn)} 0 ${p(inner, a0 + tIn)}A${r} ${r} 0 0 1 ${p(edgeIn, a0)}`;
  }
  return `${path}L${p(edgeOut, a0)}A${r} ${r} 0 0 1 ${p(outer, a0 + tOut)}Z`;
}

/* ——— Outside labels with leader lines ——— */
