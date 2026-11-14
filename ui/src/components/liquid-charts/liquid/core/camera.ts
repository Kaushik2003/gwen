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
 * The camera a chart's marks are seen through: straight down onto the floor (the flat chart, an
 * orthographic view in CSS px) or tilted, through a perspective camera. Pure maths: labels, picking
 * and the tilted ring's renderer all project with the same mapping.
 */

export type Vec3 = readonly [number, number, number];
/** A perspective view looking down at the floor. `tilt` 0 is the straight-on flat view. */
export type GlassCamera = {
  tilt: number;
  /** Radians above the floor the camera looks down from; π/2 is straight down (the flat view). */
  elevation: number;
  /** Px from the camera to its target; smaller means stronger perspective. */
  distance: number;
  /** Screen point the target lands on. */
  anchor: { x: number; y: number };
};
export type GlassView = {
  /** 0 for the orthographic straight-on view; otherwise the pinhole's focal length in px. */
  focal: number;
  position: Vec3;
  forward: Vec3;
  right: Vec3;
  up: Vec3;
  anchor: { x: number; y: number };
};
const dot = (a: Vec3, b: Vec3) => a[0] * b[0] + a[1] * b[1] + a[2] * b[2];

/** The camera looking at `target` (the ring's centre on the floor). The floor is the plane the
 * slabs stand on; at tilt 0 the view is straight down onto it, which is the flat chart. */
export function glassView(target: Vec3, camera?: GlassCamera): GlassView {
  const flat = { x: target[0], y: target[1] };
  if (!camera || camera.tilt <= 0) {
    return {
      focal: 0,
      position: target,
      forward: [0, 0, -1],
      right: [1, 0, 0],
      up: [0, -1, 0],
      anchor: flat,
    };
  }
  const e = camera.elevation,
    d = camera.distance;
  const forward: Vec3 = [0, -Math.cos(e), -Math.sin(e)];
  return {
    focal: d,
    position: [target[0] - forward[0] * d, target[1] - forward[1] * d, target[2] - forward[2] * d],
    forward,
    right: [1, 0, 0],
    up: [0, -Math.sin(e), Math.cos(e)],
    anchor: camera.anchor,
  };
}

/** Screen position of a world point; the shader traces exactly this mapping in reverse. */
export function projectPoint(view: GlassView, point: Vec3) {
  if (!view.focal) return { x: point[0], y: point[1] };
  const v: Vec3 = [
    point[0] - view.position[0],
    point[1] - view.position[1],
    point[2] - view.position[2],
  ];
  const depth = Math.max(dot(v, view.forward), 1e-3);
  return {
    x: Math.round((view.anchor.x + (view.focal * dot(v, view.right)) / depth) * 100) / 100,
    y: Math.round((view.anchor.y - (view.focal * dot(v, view.up)) / depth) * 100) / 100,
  };
}

/** Where the ray through a screen point meets the horizontal plane z = level. */
export function pointOnPlane(view: GlassView, x: number, y: number, level: number) {
  if (!view.focal) return { x, y };
  const sx = x - view.anchor.x,
    sy = y - view.anchor.y;
  const ray = [0, 1, 2].map(
    (k) => view.forward[k] * view.focal + view.right[k] * sx - view.up[k] * sy,
  );
  if (Math.abs(ray[2]) < 1e-6) return null;
  const t = (level - view.position[2]) / ray[2];
  if (t <= 0) return null;
  return { x: view.position[0] + ray[0] * t, y: view.position[1] + ray[1] * t };
}

/** How bars are seen: straight on (`flat`), turned to show each bar's side (`side`), or from a
 * corner and above (`isometric`). */
export type BarView = "flat" | "side" | "isometric";

/**
 * A parallel view of bars that keeps their front plane true to the plot, so axes, labels and hover
 * stay where the straight-on chart has them: a point `d` px behind the front lands `x * d` px to
 * the right and `y * d` px down the screen. `turn` turns each bar about its upright axis (radians),
 * so it shows a corner, with its front edge on the front plane. With no perspective a bar keeps its
 * size at any depth, and the front edge's height is the value.
 */
export type BarSlant = { x: number; y: number; turn: number };

export const BAR_SLANTS: Record<Exclude<BarView, "flat">, BarSlant> = {
  // The right side shows, its top edge running back and a little down: seen from just below the
  // tops, so no top face covers the reading.
  side: { x: 0.3, y: 0.2, turn: 0 },
  // Turned to a corner and seen from above: a shallow diamond on top of each bar.
  isometric: { x: 0, y: -0.34, turn: Math.PI / 4 },
};

/** Screen position of a world point seen through a slant (z is 0 on the front plane, negative
 * behind it). */
export function slantPoint(slant: BarSlant, point: Vec3) {
  return { x: point[0] - slant.x * point[2], y: point[1] - slant.y * point[2] };
}

/**
 * A bar's solid seen through a slant, for a bar `thickness` px wide face on: its square footprint's
 * side, how far it reaches behind the front plane, and the room it takes past its face-on rect, to
 * the right and above its top. A turned bar's diagonal spans the thickness, so it is as wide on
 * screen as the face-on bar.
 */
export function slantedSolid(slant: BarSlant, thickness: number) {
  const spread = Math.abs(Math.cos(slant.turn)) + Math.abs(Math.sin(slant.turn));
  const side = thickness / spread;
  const reach = side * spread;
  return {
    side,
    reach,
    right: Math.max(slant.x, 0) * reach,
    above: Math.max(-slant.y, 0) * reach,
  };
}
