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
/** Liquid glass for every chart other than the area: edge-lit acrylic solids, seen straight on
 * (as in Metalab's Robinhood charts). Light enters through each solid's edges, strongest at its
 * tip (the value end), and scatters inward: pale and milky where it enters, deepening through
 * the saturated colour to a dark core. By default the view is orthographic and straight on, so no
 * side or top faces show and depth comes only from that light; solids sit in CSS pixels, so labels
 * placed in plain CSS coordinates line up exactly. A ring can also be seen tilted, through a
 * perspective camera, with its slabs extruded up from the floor and lit through their tops. Bars can
 * be seen at a slant (`BarSlant`): a parallel view that keeps their front plane true, so their sides
 * and tops show while labels and axes stay where they were. */
import { FIELD_LENGTH, FIELD_ROWS, fieldPrint, MENISCUS, NX, NZ } from "./liquid-field";
import { glassView, type BarSlant, type GlassCamera, type Vec3 } from "@/components/liquid-charts/liquid/core/camera";
import { isStaticGlass, sizeCanvas } from "@/components/liquid-charts/liquid/core/presence";
import { pixelRatio } from "@/components/liquid-charts/liquid/core/render-scale";
import type { Rgb, Theme } from "@/components/liquid-charts/liquid/core/colour";

import { validFunnelShell, funnelShellBounds } from "@/components/liquid-charts/liquid/core/funnel-shell";
import type { BarShell } from "@/components/liquid-charts/liquid/core/mark-types";

export type { Rgb, Theme };
export type GlassPaintResult = { painted: boolean; reason?: string };
export {
  glassView,
  pointOnPlane,
  projectPoint,
  type GlassCamera,
  type GlassView,
  type Vec3,
} from "@/components/liquid-charts/liquid/core/camera";
export type GlassBar = {
  /** Actual convex outline, in canvas CSS pixels; absent for legacy bars. */
  shell?: BarShell;
  /** Centre across the bar's axis (x for vertical bars, y for horizontal), CSS px. */
  x: number;
  /** Where the bar starts along its axis: the baseline it grows from. */
  base: number;
  /** Thickness across the axis, and length along it. */
  width: number;
  height: number;
  /** Which way the bar grows from its base; light enters at its value end. Defaults to up. */
  direction?: "up" | "down" | "right" | "left";
  /** Share of the shell filled with liquid, from the bottom. */
  fill: number;
  /** Etch a pattern into the glass from this share of the bar's length to its end (0 = the whole
   * bar). Marks a different state of the same quantity: projected, estimated, over a limit. */
  pattern?: number;
  /** Up to four engraved reference marks (a target, a pace, a plan), as shares of the bar's
   * length from its base. They glow in the liquid's colour once it passes them. A share plus 2 is
   * a graduation tick from the bar's near rim, plus 4 a graduation ring across it: an axis's
   * graduations are cut quieter than reference marks. */
  marks?: readonly number[];
  tint: Rgb;
  /** 1 shows the tint; 0 the theme's smoky grey (inactive). */
  tone: number;
  /** The liquid's simulated surface (`LiquidField.rows`); without it the front is flat. */
  field?: Float32Array;
};
export type GlassSegment = {
  /** Clockwise radians from 12 o'clock. */
  start: number;
  end: number;
  /** Share of the slab filled, flowing clockwise from its start. */
  fill: number;
  tint: Rgb;
  tone: number;
  /** Px the slab lifts out from the ring when selected (it moves outward by 0.3 of this). */
  pop: number;
  /** Px the slab stands above the floor; defaults to the ring's depth (the flat view). */
  height?: number;
  /** Concentric rings: this slab's own middle radius and thickness, px (default the ring's). One
   * canvas then draws every track, with true depth between them when tilted. */
  radius?: number;
  width?: number;
  /** The liquid's simulated surface (`LiquidField.rows`); without it the front is flat. */
  field?: Float32Array;
};
export type GlassScene =
  | {
      kind: "bars";
      theme: Theme;
      depth: number;
      bevel: number;
      bars: GlassBar[];
      /** Px at the canvas's edge over which the glow round the glass fades out: the canvas's bleed
       * past the plot, so the glow never stops at the canvas's edge (a tinted panel on a light
       * page, where the bars' halos add up). 0 (the default) keeps it to the edge. */
      margin?: number;
      /** Seen at a slant, with their sides showing; straight on without one. Columns only: every
       * bar grows up or down. The scene's depth is then each bar's side (a square footprint). */
      slant?: BarSlant;
      /** The floor the bars stand on, at canvas y: nothing shows below it, and the glass fades out
       * over `fade` px above it. */
      floor?: { y: number; fade: number };
      /** Etch the edges of empty glass with a dashed line, so an empty bar or a container's empty
       * part reads as its outline. */
      edges?: boolean;
    }
  | {
      kind: "ring";
      theme: Theme;
      ring: { x: number; y: number; radius: number; width: number; depth: number; gap: number };
      bevel: number;
      segments: GlassSegment[];
      camera?: GlassCamera;
    };

/** Solids one canvas draws. Bars stop at 24 (`RAYMARCH_LIMIT`), one ring at 24 (`RING_LIMIT`);
 * concentric rings may use them all. */
export const GLASS_LIMIT = 48;
const MAX = GLASS_LIMIT;
/** A solid's glass wall, px: the liquid shows only behind the front wall and before the back one,
 * each with an edge of 0.8 px it fades in over. */
export const GLASS_WALL = 1.6;
const vertexSource = `#version 300 es
in vec2 position;
void main() { gl_Position = vec4(position, 0.0, 1.0); }
`;
const fragmentSource = `#version 300 es
precision highp float;
uniform vec2 size;
uniform float ratio;
uniform int mode;
uniform int count;
uniform bool hostShells;
uniform vec4 shapeA[${MAX}];
uniform vec4 shapeB[${MAX}];
uniform vec4 tints[${MAX}];
// Up to four engraved reference marks per bar, as shares of its length from the base; -1 is none.
uniform vec4 marks[${MAX}];
// Each solid's simulated liquid surface: ${FIELD_ROWS} rows of ${NX} (${NZ} rows of heights through the
// depth, px from the level; the coat; then flow, moving and whether it has a surface).
uniform highp sampler2D field;
uniform vec4 ring;
uniform vec4 solid;
uniform vec3 grey;
uniform float theme;
uniform bool staticGlass;
uniform float focal;
uniform float lift;
uniform vec3 camPos;
uniform vec3 camDir;
uniform vec3 camRight;
uniform vec3 camUp;
uniform vec2 anchor;
uniform vec3 boxMin;
uniform vec3 boxMax;
// Bars at a slant: a point d px behind the front plane shows (x, y) * d px across and down; z turns
// each bar about its upright axis. All zero is straight on.
uniform vec3 slant;
// The floor's canvas y and the px over which the glass fades out above it; no fade at 0.
uniform vec2 floorFade;
// Dashed edges etched into empty glass.
uniform bool edged;
// Px at the canvas's edge over which the glow round the glass fades out (its bleed); 0 keeps it.
uniform float margin;
out vec4 outputColour;

const float PI = 3.14159265359;
const float STEP = 1.25;
// The cosine of glass's critical angle (index 1.5): refracted light crosses a wall at least this
// steeply.
const float CROSS = .745;
// How strongly an axis's graduations are engraved: a faint cut in the glass, visible when looked
// for, never a drawn line. .65 is a shade stronger, .28 barely there; the optical renderer
// (chart-optical-shader.ts) uses the same.
const float ETCH = .45;
// A photo studio the glass reflects (y points down the screen): a broad soft box overhead, a
// strip to the right, a dim fill to the left and a dark floor. Narrow bevels sweep across it,
// so their highlights land as crisp lines that change around a ring.
vec3 studio(vec3 r, float light) {
  float overhead = smoothstep(.4, .62, -r.y) * smoothstep(.85, .6, abs(r.x));
  float right = smoothstep(.48, .64, r.x) * smoothstep(.72, .38, abs(r.y));
  float left = smoothstep(.55, .72, -r.x) * smoothstep(.72, .38, abs(r.y));
  float front = smoothstep(.8, 1., r.z);
  float ground = smoothstep(.12, .5, r.y);
  float base = mix(.035, 1., light) * (1. - ground * mix(.7, .08, light));
  return vec3(base + overhead * mix(5.5, .5, light) + right * mix(2.8, .3, light)
              + left * mix(.9, .15, light) + front * .1);
}

float roundBox(vec3 q, float r) {
  q += r;
  return length(max(q, 0.)) + min(max(q.x, max(q.y, q.z)), 0.) - r;
}
float wrapAngle(float a) { return mod(a + PI, 2. * PI) - PI; }
// The angle round the ring, clockwise from 12 o'clock. On the row through the ring's centre l.y is
// exactly zero, and some GPUs return the opposite half-turn for atan(x, -0.), which drew a dark
// seam across the ring at 3 and 9 o'clock; a zero l.y is nudged just below the centre line.
float ringAngle(vec2 l) { return atan(l.x, abs(l.y) < 1e-4 ? -1e-4 : -l.y); }

// Bars: A = (centre x, centre y, half width, half height); B = (fill, direction, -, -), where
// direction 0 grows up, 1 down, 2 right, 3 left from the anchored end.
// Ring slabs: A = (start, end, pop, fill); B = (height, radius, half width, -), where a radius or
// half width of 0 is the ring's own (concentric rings give each slab its own). solid = (depth,
// bevel, shell, half width). The floor is z = -depth; a slab stands from there to its height.
float slabRadius(int i) { return shapeB[i].y > 0. ? shapeB[i].y : ring.z; }
float slabHalf(int i) { return shapeB[i].z > 0. ? shapeB[i].z : solid.w; }
bool slanted() { return slant.x != 0. || slant.y != 0.; }
// p in bar i's own frame: a bar turned about its upright axis stands square there, its front face
// at z = 0 and its back at z = -depth. In the world its nearest corner touches the front plane.
vec3 barFrame(vec3 p, int i) {
  if (slant.z == 0.) return p;
  float c = cos(slant.z), s = sin(slant.z), back = solid.x * .5;
  float centre = -(shapeA[i].z * abs(s) + back * abs(c));
  vec2 d = vec2(p.x - shapeA[i].x, p.z - centre);
  return vec3(shapeA[i].x + c * d.x + s * d.y, p.y, -s * d.x + c * d.y - back);
}
// Optional host shells occupy extra rows after the unchanged liquid field. Only
// the scene guard uses a uniform; no per-mark uniform arrays are added.
float shellValue(int i, int column) {
  return texelFetch(field, ivec2(column, ${FIELD_ROWS * MAX} + i), 0).r;
}
bool hasHostShell(int i) {
  // Uniform branch: legacy scenes never read the optional shell texture rows.
  if (!hostShells) return false;
  return shellValue(i, 8) > .5;
}
vec2 shellPoint(int i, int vertex) {
  return vec2(shellValue(i, vertex * 2), shellValue(i, vertex * 2 + 1));
}
float hostDistance(vec2 p, int i) {
  float squared = 1e30;
  bool positive = false, negative = false;
  for (int k = 0; k < 4; k++) {
    vec2 a = shellPoint(i, k), e = shellPoint(i, (k + 1) % 4) - a, v = p - a;
    float span = dot(e, e);
    // A collapsed end is a triangle, not a zero-length normal or a NaN.
    if (span <= 0.) continue;
    vec2 q = v - e * clamp(dot(v, e) / span, 0., 1.);
    squared = min(squared, dot(q, q));
    float cross = e.x * v.y - e.y * v.x;
    positive = positive || cross > 0.;
    negative = negative || cross < 0.;
  }
  return sqrt(squared) * (positive && negative ? 1. : -1.);
}
float shell(vec3 p, int i) {
  vec4 a = shapeA[i];
  float depth = solid.x;
  if (mode == 0) {
    if (hasHostShell(i)) {
      // Extrude the actual polygon. Bevel the front/back edges inside that outline;
      // its mid-depth silhouette (including a triangular tip) stays exact.
      float bevel = max(0., min(solid.y, depth * .5 - .01));
      vec2 q = vec2(hostDistance(p.xy, i), abs(p.z + depth * .5) - depth * .5) + bevel;
      return length(max(q, 0.)) + min(max(q.x, q.y), 0.) - bevel;
    }
    vec3 q = abs(barFrame(p, i) - vec3(a.x, a.y, -depth * .5)) - vec3(a.z, a.w, depth * .5);
    return roundBox(q, min(solid.y, min(a.z, min(a.w, depth * .5)) - .01));
  }
  float mid = (a.x + a.y) * .5;
  vec2 outward = vec2(sin(mid), -cos(mid));
  vec3 l = p - vec3(ring.xy + outward * a.z * .3, a.z);
  float r = length(l.xy);
  float radius = slabRadius(i), halfW = slabHalf(i);
  float off = abs(wrapAngle(ringAngle(l.xy) - mid));
  float halfSpan = max((a.y - a.x) * .5 - ring.w / radius, .002);
  float h = max(shapeB[i].x, 1.);
  vec3 q = vec3(abs(r - radius) - halfW, r * sin(clamp(off - halfSpan, -PI * .5, PI * .5)),
                abs(l.z + depth - h * .5) - h * .5);
  // A slab thinner than the ring keeps its bevel in proportion.
  float bevel = shapeB[i].z > 0. ? min(solid.y, halfW * .4) : solid.y;
  return roundBox(q, min(bevel, h * .5 - .01));
}
float map(vec3 p, out int id) {
  float best = 1e5;
  id = 0;
  for (int i = 0; i < ${MAX}; i++) {
    if (i >= count) break;
    float d = shell(p, i);
    if (d < best) {
      best = d;
      id = i;
    }
  }
  return best;
}
// Signed distances (negative inside) from solid i's front and back walls, a slab's top and
// floor, ignoring its sides. Every ray enters through the front or a side, never the back.
vec2 faceDistances(vec3 p, int i) {
  if (mode == 0) {
    float z = barFrame(p, i).z;
    return vec2(z, -solid.x - z);
  }
  float z = p.z - shapeA[i].z + solid.x;
  return vec2(z - max(shapeB[i].x, 1.), -z);
}
// Straight on, the solids' outline in the screen's plane: each one's distance at its mid-depth,
// where its sides stand straight.
float planar(vec2 px) {
  float best = 1e5;
  for (int i = 0; i < ${MAX}; i++) {
    if (i >= count) break;
    float z = mode == 0 ? -solid.x * .5 : shapeA[i].z - solid.x + max(shapeB[i].x, 1.) * .5;
    best = min(best, shell(vec3(px, z), i));
  }
  return best;
}
// A bar seen along its own axis: how far p is from the anchored end, the bar's length, and how
// far p is from the side away from the key light and the side toward it.
void barAxes(vec3 p, int i, out float u, out float len, out float near, out float far,
             out float across) {
  vec4 a = shapeA[i];
  float dir = shapeB[i].y;
  p = barFrame(p, i);
  if (dir < 1.5) {
    len = 2. * a.w;
    across = a.z;
    u = dir < .5 ? a.y + a.w - p.y : p.y - (a.y - a.w);
    near = p.x - (a.x - a.z);
    far = a.x + a.z - p.x;
  } else {
    len = 2. * a.z;
    across = a.w;
    u = dir < 2.5 ? p.x - (a.x - a.z) : a.x + a.z - p.x;
    near = a.y + a.w - p.y;
    far = p.y - (a.y - a.w);
  }
  if (hasHostShell(i)) {
    float coordinate = dir < 1.5 ? p.y : p.x;
    float low = 1e30, high = -1e30;
    for (int k = 0; k < 4; k++) {
      vec2 start = shellPoint(i, k), end = shellPoint(i, (k + 1) % 4);
      float from = dir < 1.5 ? start.y : start.x;
      float to = dir < 1.5 ? end.y : end.x;
      if (abs(to - from) < 1e-7) continue;
      float t = (coordinate - from) / (to - from);
      if (t < 0. || t > 1.) continue;
      float edge = dir < 1.5 ? mix(start.x, end.x, t) : mix(start.y, end.y, t);
      low = min(low, edge);
      high = max(high, edge);
    }
    if (high >= low) {
      across = max((high - low) * .5, .01);
      near = dir < 1.5 ? p.x - low : high - p.y;
      far = dir < 1.5 ? high - p.x : p.y - low;
    }
  }
}
float hash(vec3 p) {
  p = fract(p * .3183099 + .1);
  p *= 17.;
  return fract(p.x * p.y * p.z * (p.x + p.y + p.z));
}
float cloud(vec3 x) {
  vec3 i = floor(x), f = fract(x);
  f = f * f * (3. - 2. * f);
  return mix(mix(mix(hash(i), hash(i + vec3(1, 0, 0)), f.x),
                 mix(hash(i + vec3(0, 1, 0)), hash(i + vec3(1, 1, 0)), f.x), f.y),
             mix(mix(hash(i + vec3(0, 0, 1)), hash(i + vec3(1, 0, 1)), f.x),
                 mix(hash(i + vec3(0, 1, 1)), hash(i + vec3(1, 1, 1)), f.x), f.y), f.z);
}
const int NX = ${NX}, NZ = ${NZ}, ROWS = ${FIELD_ROWS};
// Solid i's simulated surface at x (0 to NX - 1, wall to wall) in row r, px; and its extras
// (0 flow, 1 moving, 2 has a surface).
float fieldAt(int i, float x, int r) {
  float fx = clamp(x, 0., float(NX - 1));
  int x0 = int(floor(fx)), x1 = min(x0 + 1, NX - 1), row = i * ROWS + r;
  return mix(texelFetch(field, ivec2(x0, row), 0).r, texelFetch(field, ivec2(x1, row), 0).r,
             fx - float(x0));
}
float fieldExtra(int i, int k) { return texelFetch(field, ivec2(k, i * ROWS + NZ + 1), 0).r; }
// The meniscus: px the liquid climbs the walls, keeping the volume.
float wallRise(float s, float halfW) {
  return ${MENISCUS.toFixed(2)} * (exp(-(1. - abs(s)) * halfW / 2.2) - min(2.2 / max(halfW, 1.), 1.));
}
float density3(vec3 q, int i) {
  if (staticGlass) return 1.;
  vec4 a = shapeA[i];
  vec3 l = q / 64.;
  if (mode == 0) {
    float dir = shapeB[i].y;
    vec2 anchor = dir < .5 ? vec2(a.x, a.y + a.w) : dir < 1.5 ? vec2(a.x, a.y - a.w)
                : dir < 2.5 ? vec2(a.x - a.z, a.y) : vec2(a.x + a.z, a.y);
    // The cloud travels with the liquid while it pours, then settles back.
    vec2 grow = dir < .5 ? vec2(0., -1.) : dir < 1.5 ? vec2(0., 1.) : dir < 2.5 ? vec2(1., 0.) : vec2(-1., 0.);
    anchor += grow * fieldExtra(i, 0);
    l = (q - vec3(anchor, 0.)) / (dir < 1.5 ? vec3(46., 150., 48.) : vec3(150., 46., 48.));
  } else {
    l.z -= fieldExtra(i, 0) / 64.;
  }
  l += float(i) * 7.31;
  return .78 + .44 * (cloud(l) * .75 + cloud(l * 2.1 + 3.7) * .25);
}
vec2 liquidFront(vec3 p, int i);
float frontWave(vec3 p, int i);
// Light entering through the solid's edges, in the plane of its face. The tip (the value end, or
// the liquid's surface while it fills) lets in the most, the long sides less (the right, toward
// the key light, more than the left) and the anchored end a little. Each falls off with the
// distance it travels through the gel, so the core is where light has come furthest.
float edgeLight(vec3 p, int i) {
  vec4 a = shapeA[i];
  if (mode == 0) {
    float u, len, near, far, across;
    barAxes(p, i, u, len, near, far, across);
    float tip = max(min(liquidFront(p, i).x, len - u), 0.), anchor = max(u, 0.);
    float reach = 22. + .26 * len, side = across * .3;
    return 2.3 * exp(-tip / reach) + .8 * exp(-max(near, 0.) / side)
         + 1.1 * exp(-max(far, 0.) / side) + .55 * exp(-anchor / (side * 1.2));
  }
  float mid = (a.x + a.y) * .5;
  vec2 outward = vec2(sin(mid), -cos(mid));
  vec2 l = p.xy - (ring.xy + outward * a.z * .3);
  float r = length(l), span = a.y - a.x;
  float along = mod(ringAngle(l) - a.x + .1, 2. * PI) - .1;
  float radius = slabRadius(i), halfW = slabHalf(i);
  float tip = max((a.w * span - along) * r + frontWave(p, i), 0.), anchor = max(along * r, 0.);
  float outer = max(radius + halfW - r, 0.), inner = max(r - radius + halfW, 0.);
  // The outer rim catches more light where it faces the key light, up and to the right.
  float facing = .55 + .45 * max(dot(l / max(r, 1.), normalize(vec2(.55, -.81))), 0.);
  float reach = 26. + .24 * span * radius, side = halfW * .75;
  // Tilted, the top face is the tip: light pours in through it and deepens down the walls.
  float h = max(shapeB[i].x, 1.);
  float below = max(-solid.x + a.z + h - p.z, 0.);
  // Tilted, the rims light less and the top more, so walls keep their saturated colour.
  float edges = 2.1 * exp(-tip / reach) + 1.1 * facing * exp(-outer / side)
              + .7 * exp(-inner / side) + .4 * exp(-anchor / side);
  // On white the walls give up even more rim light, so pale tops sit over saturated sides.
  return edges * mix(1., mix(.6, .38, step(.5, theme)), lift)
       + lift * .75 * exp(-below / (4. + h * .3));
}
vec3 normalAt(vec3 p, int i) {
  const vec2 k = vec2(1., -1.);
  float h = .35;
  return normalize(k.xyy * shell(p + k.xyy * h, i) + k.yyx * shell(p + k.yyx * h, i)
                 + k.yxy * shell(p + k.yxy * h, i) + k.xxx * shell(p + k.xxx * h, i));
}
// Where p lies across solid i: s from -1 to 1 wall to wall, px from the middle to a wall, and
// px of liquid along the fill (the surface fades in over the first px of an empty glass).
vec3 acrossOf(vec3 p, int i) {
  vec4 a = shapeA[i];
  if (mode == 0) {
    float u, len, near, far, across;
    barAxes(p, i, u, len, near, far, across);
    return vec3(clamp((near - across) / max(across, .5), -1., 1.), across, shapeB[i].x * (len + 2.));
  }
  float mid = (a.x + a.y) * .5;
  vec2 l = p.xy - (ring.xy + vec2(sin(mid), -cos(mid)) * a.z * .3);
  float r = length(l), halfW = slabHalf(i);
  return vec3(clamp((r - slabRadius(i)) / max(halfW, .5), -1., 1.), halfW, a.w * (a.y - a.x) * r);
}
// The surface's offset at p on the front face (px along the fill); 0 without a surface.
float frontWave(vec3 p, int i) {
  if (fieldExtra(i, 2) < .5) return 0.;
  vec3 c = acrossOf(p, i);
  float x = (c.x + 1.) * .5 * float(NX - 1);
  return (fieldAt(i, x, 0) + wallRise(c.x, c.y)) * clamp(c.z / 6., 0., 1.);
}
// Signed px distance from the liquid's front (positive inside the liquid), and how far along
// the solid the point lies (0 at the start of the fill direction, 1 at its end), given the
// surface's offset there.
vec2 liquidFrontAt(vec3 p, int i, float wave) {
  vec4 a = shapeA[i];
  if (mode == 0) {
    float u, len, near, far, across;
    barAxes(p, i, u, len, near, far, across);
    // Outlined at a slant, an empty glass holds no film at its foot: seen from above, its whole
    // floor would show it.
    if (edged && shapeB[i].x <= 0.) return vec2(-1e3, clamp(u / len, 0., 1.));
    return vec2(shapeB[i].x * (len + 2.) + wave - u, clamp(u / len, 0., 1.));
  }
  float mid = (a.x + a.y) * .5;
  vec2 outward = vec2(sin(mid), -cos(mid));
  vec2 l = p.xy - (ring.xy + outward * a.z * .3);
  float span = a.y - a.x;
  float along = mod(ringAngle(l) - a.x + .1, 2. * PI) - .1;
  float front = a.w * (span + .02);
  return vec2((front - along) * length(l) + wave, clamp(along / span, 0., 1.));
}
vec2 liquidFront(vec3 p, int i) { return liquidFrontAt(p, i, frontWave(p, i)); }

float hitFacing;
// The colour and coverage of solid id where the ray meets it at p, and how far the ray runs
// inside it before leaving (so a second trace can carry on to whatever lies behind).
vec4 surface(vec3 p, vec3 rd, int id, float light, out float exitT) {
  exitT = 64. * STEP;
    vec3 n = normalAt(p, id);
    vec4 tint = tints[id];
    float tone = tint.a;
    vec3 colour = mix(mix(grey, tint.rgb, .1 * (1. - light)), tint.rgb, tone);
    // Walk into the solid for its coverage: path in the liquid, path in the bare wall, and the
    // liquid's surface where the walk crosses it (only visible while it fills).
    float inLiquid = 0., inShell = 0., meniscus = 0.;
    // The simulated surface along this ray. Straight on, every sample shares the ray's place
    // across the solid, so its height at each depth row, and the coat above it, are read once.
    float rows[NZ];
    float coat = 0.;
    bool surfaced = fieldExtra(id, 2) > .5;
    if (surfaced) {
      vec3 c = acrossOf(p, id);
      float fx = (c.x + 1.) * .5 * float(NX - 1);
      float fade = clamp(c.z / 6., 0., 1.), rise = wallRise(c.x, c.y);
      for (int r = 0; r < NZ; r++) rows[r] = (fieldAt(id, fx, r) + rise) * fade;
      coat = fieldAt(id, fx, NZ) * fade;
    } else {
      for (int r = 0; r < NZ; r++) rows[r] = 0.;
    }
    // Tilted or slanted, the walk follows the ray as the glass bends it, toward the surface's
    // normal: a ray that only clips a rounded rim runs on into the solid, as light does, instead of
    // leaving it at once as a thin, see-through sliver.
    // Outlined at a slant, glass entered above its liquid is walked straight: bent down through a
    // rounded top, the walk would find the liquid far below and speckle the corner with it, where
    // straight it meets the liquid's top face just where it lies.
    bool above = edged && mode == 0 && liquidFront(p, id).x < -2.;
    vec3 walk = (focal > 0. || slanted()) && !above ? refract(rd, n, 1. / 1.5) : rd;
    for (int k = 0; k < 64; k++) {
      vec3 q = p + walk * (float(k) + .5) * STEP;
      float d = shell(q, id);
      if (d > 0.) {
        exitT = (float(k) + .5) * STEP;
        break;
      }
      // Through the depth the surface is a band, not a line: front and back rows differ.
      float dz = clamp(-(mode == 0 ? barFrame(q, id) : q).z / max(solid.x, 1.), 0., 1.) * float(NZ - 1);
      int r0 = int(floor(dz));
      float here = mix(rows[r0], rows[min(r0 + 1, NZ - 1)], dz - float(r0));
      vec2 front = liquidFrontAt(q, id, here);
      // The liquid fills the glass out to its sides; only the front and back walls (a slab's top
      // and floor) hold it in. Seen edge-on, a side wall shows the liquid behind it, as refraction
      // shows it in a real glass, not a band of bare glass (a straight walk would run down the
      // wall). Light entering bends to within glass's critical angle, so even a ray grazing a
      // tilted top is through it in a few px.
      vec2 faces = faceDistances(q, id);
      float crossed = max(min(faces.x, -CROSS * (float(k) + .5) * STEP), faces.y);
      float inside = smoothstep(-solid.z + .8, -solid.z - .8, crossed);
      float liquid = inside * smoothstep(-.8, .8, front.x);
      // The coat the liquid left on the glass above it: a thin, translucent film.
      float film = coat > .3 ? inside * (1. - liquid) * smoothstep(-.8, .8, front.x - here + coat) : 0.;
      inLiquid += STEP * (liquid + film * .07);
      // Only the glass wall itself scatters; wall behind the liquid is hidden by it.
      inShell += STEP * (1. - liquid) * step(-solid.z - .8, d) * exp(-inLiquid / 3.);
      meniscus += STEP * exp(-front.x * front.x / 2.2) * step(crossed, -solid.z);
    }
    // Edge light through the gel, with a faint cloudiness so no two solids are identical.
    float light3 = edgeLight(p, id) * mix(.86, 1.14, (density3(p, id) - .78) / .44);
    float entered = 1. - exp(-light3 * mix(1.15, 1.3, light));
    // Dark cards: light that has come far has lost all but the colour's deepest tone, so the core
    // is dark and saturated; where it enters it is pale and milky.
    float deepest = mix(2.4, 4.2, tone);
    vec3 night = pow(max(colour, vec3(.02)), vec3(mix(deepest, .34, entered)));
    night *= mix(mix(.4, .16, tone), 1., entered) * mix(.8, 1., tone);
    // Light cards stay high-key: the core is the pure, vivid colour and the light coming in
    // dilutes it toward white at the edges and the tip. Nothing goes muddy.
    vec3 vivid = pow(max(colour, vec3(.02)), vec3(mix(1.15, 1.3, tone)));
    vec3 day = mix(vivid, mix(colour, vec3(1.), mix(.8, .6, tone)), pow(entered, 1.8));
    vec3 body = mix(night, day, light);
    vec3 milk = mix(sqrt(colour), vec3(1.), .45) * mix(mix(.6, 1., light), 1., tone);
    body = mix(body, milk, (1. - exp(-max(inShell - 12., 0.) / 9.)) * .3);
    // The meniscus glints. Looking down on the surface (at a slant), the walk runs along it and the
    // glint builds up over its whole face, so it is held to what a face-on view gives.
    if (!staticGlass)
      body += mix(sqrt(colour), vec3(1.), .5) * min(meniscus / 5., 1.) * (slanted() ? .18 : .5);
    // An etched pattern: diagonal lanes frosted into the gel, where light scatters and the colour
    // turns milky. Lanes are anchored to the bar and sized to its thickness, so every bar's
    // pattern reads alike, and the rest of the material (rims, bevel, glow) is untouched.
    float etched = 0.;
    if (mode == 0 && shapeB[id].w > .5) {
      float u, len, near, far, across;
      barAxes(p, id, u, len, near, far, across);
      float region = smoothstep(shapeB[id].z * len - .6, shapeB[id].z * len + .6, u);
      float period = clamp(across * .62, 5.5, 12.);
      float lane = abs(fract((u - near) / period) - .5) * period;
      float etch = (1. - smoothstep(period * .2 - .5, period * .2 + .5, lane)) * region;
      // A fine seam where the pattern begins, so a projected tail or an overflow reads as meant.
      float from = shapeB[id].z * len;
      etch = max(etch, exp(-(u - from) * (u - from) / .9) * step(.5, from));
      etched = etch;
      vec3 frost = mix(mix(sqrt(colour), vec3(1.), .35), vec3(1.), light * .55);
      body = mix(body, mix(body, vec3(dot(body, vec3(.3333))), .22), region * .6);
      body = mix(body, frost, etch * mix(.42, .62, light));
    }
    // Glass reflects the studio by Schlick's Fresnel term; every state keeps this, so inactive
    // graphite still has its silver rims.
    vec3 V = -rd;
    float facing = max(dot(n, V), 0.);
    hitFacing = facing;
    // At a slant a bar's side is a broad face seen at a glance, not a bevel: it keeps more of its
    // colour than the angle alone would leave it, so the side reads as the same glass.
    float look = slanted() ? mix(facing, 1., .5) : facing;
    float fresnel = min(.045 + .955 * pow(1. - look, 5.), .24);
    vec3 env = studio(reflect(rd, n), light);
    vec3 lit = min(body * (1. - fresnel) + env * fresnel * mix(.45, 1., light), vec3(1.));
    // On white, the bevel turns away from the room and reads as a fine darker edge, which is what
    // keeps a pale solid from dissolving into the card.
    lit = mix(lit, lit * vec3(.85), light * (1. - smoothstep(.45, .95, look)));
    // Liquid is near-opaque; a bare wall is nearly clear face-on and only builds up where the
    // view grazes it, so an empty shell reads as an outline of glass.
    float liquidCover = 1. - exp(-inLiquid / 3.);
    float wallCover = (1. - exp(-inShell * inShell / 1100.)) * (1. - liquidCover);
    // Outlined, empty glass has no milky walls: they build up at its rims and double the pen line.
    if (edged) wallCover = 0.;
    vec3 wall = min(milk * mix(.25, .9, light) + env * fresnel, vec3(1.));
    float glassAlpha = clamp(fresnel * mix(1.6, .55, light), 0., 1.) * (1. - liquidCover - wallCover);
    vec3 glass = mix(min(env, vec3(1.)), vec3(.74), light);
    float filled = mode == 0 ? shapeB[id].x : shapeA[id].w;
    glassAlpha *= mix(1., 1. - filled, light);
    // Where the ray only clips the very edge, the rim is thinner than a pixel and would speckle.
    glassAlpha *= smoothstep(.6, 3., exitT);
    if (edged) {
      // Outlined, empty glass is shaded as a drawing shades a box: the top catches the light from
      // above, the front a little less, the sides least. Its volume reads on light cards and dark,
      // quieter than the liquid, with no reflected rims to double the pen line.
      float catches = clamp(dot(n, normalize(vec3(.1, -1., .75))) * .5 + .5, 0., 1.);
      glass = mix(mix(vec3(.06), vec3(.32), catches), mix(vec3(.84), vec3(.99), catches), light);
      glassAlpha = (1. - liquidCover) * .3 * smoothstep(.6, 3., exitT);
    }
  vec4 result = vec4(lit * liquidCover + wall * wallCover + glass * glassAlpha,
                     liquidCover + wallCover + glassAlpha);
  // Etched lanes show in the empty glass too, as frost in the bare wall, so planned or projected
  // work that holds no liquid yet still reads as etched.
  float bare = etched * smoothstep(.6, 3., exitT) * (1. - liquidCover);
  if (bare > 0.) {
    vec3 frosted = mix(mix(sqrt(colour), vec3(1.), .15), mix(colour, vec3(.47), .4), light);
    float a = bare * mix(.34, .3, light);
    result.rgb = result.rgb * (1. - a) + frosted * a;
    result.a = result.a * (1. - a) + a;
  }
  // Engraved reference marks (a target, a pace, a plan). Engravings in edge-lit acrylic catch the
  // light travelling through the sheet and glow, brightest towards the rims where it enters. Once
  // the liquid has passed a mark it glows in the liquid's colour; before that it is a pale frosted
  // groove in the empty glass. Darker flanks keep the groove crisp on pale glass.
  if (mode == 0) {
    float u, len, near, far, across;
    barAxes(p, id, u, len, near, far, across);
    float level = shapeB[id].x * (len + 2.);
    float rim = .72 + .28 * exp(-min(max(near, 0.), max(far, 0.)) / max(across * .35, .5));
    for (int k = 0; k < 4; k++) {
      vec4 m = marks[id];
      float at = k == 0 ? m.x : k == 1 ? m.y : k == 2 ? m.z : m.w;
      if (at < 0.) continue;
      // A share past 2 is a graduation: a short tick cut in from the near rim, like a measuring
      // cylinder's, rather than a ring across the bar; past 4, a graduation ring. Either is cut
      // quieter than a reference mark.
      float reach = 1.;
      bool graduated = at > 1.5;
      if (at > 3.5) {
        at -= 4.;
      } else if (at > 1.5) {
        at -= 2.;
        float from = near;
        // Turned to a corner, the near rim runs back from the front plane, so a tick there shows
        // above its value. The tick is cut across the front corner instead, the one edge on the
        // plane the axis is read on: a chevron along both faces, its point at the value.
        if (slant.z != 0.) {
          vec3 l = barFrame(p, id);
          vec2 edge = vec2(shapeA[id].x + shapeA[id].z * sign(sin(slant.z)), cos(slant.z) > 0. ? 0. : -solid.x);
          from = length(l.xz - edge) * .8;
        }
        reach = 1. - smoothstep(.2, .3, from / max(2. * across, 1.));
        if (reach <= 0.) continue;
      }
      float d = u - at * len;
      bool reached = level >= at * len - .5;
      if (graduated) {
        // An axis's graduation is a faint engraving: a soft groove a shade off the glass around
        // it, with no bright edge and no halo. Passed, it takes the liquid's own colour, a little
        // lighter on dark cards and a little deeper on light ones; before that, a pale frost.
        float groove = exp(-d * d / .5) * reach * ETCH;
        vec3 cut = reached ? mix(mix(colour, vec3(1.), .4), colour * .8, light) : mix(vec3(.8), vec3(.5), light);
        float a = groove * (reached ? mix(.3, .32, light) : mix(.22, .2, light));
        result.rgb = result.rgb * (1. - a) + cut * a;
        result.a = result.a * (1. - a) + a;
        continue;
      }
      float core = exp(-d * d / .32) + .3 * exp(-d * d / 4.);
      float flank = exp(-(abs(d) - 1.35) * (abs(d) - 1.35) / .3) * reach;
      vec3 glow = reached ? mix(mix(sqrt(colour), vec3(1.), .55), vec3(1.), light * .6)
                          : mix(vec3(.88), vec3(.52), light);
      float a = clamp(core * rim, 0., 1.) * (reached ? mix(.9, .85, light) : mix(.7, .58, light)) * reach;
      // A passed mark scatters a little light into the liquid around it, so it reads as lit.
      float halo = reached ? exp(-d * d / 14.) * mix(.22, .08, light) * reach : 0.;
      result.rgb *= 1. - flank * mix(.18, .16, light) * result.a;
      result.rgb += mix(sqrt(colour), vec3(1.), .4) * halo * result.a;
      result.rgb = result.rgb * (1. - a) + glow * a;
      result.a = result.a * (1. - a) + a;
    }
  }
  return result;
}
float mapExcept(vec3 p, int skip, out int id) {
  float best = 1e5;
  id = -1;
  for (int i = 0; i < ${MAX}; i++) {
    if (i >= count) break;
    if (i == skip) continue;
    float d = shell(p, i);
    if (d < best) {
      best = d;
      id = i;
    }
  }
  return best;
}

// Bars at a slant, outlined like a dashed box: each edge a face toward the viewer touches, as a
// one-px pen line in screen space with even dashes that begin and end on its corners. Only the
// empty glass is outlined (above the liquid), and glass standing in front hides the line.
// A corner of bar i: k's bits pick right (1) or left, bottom (2) or top, back (4) or front. It sits
// on the middle of the rounded bevel, where the glass's edge is.
vec3 corner(int i, int k) {
  vec4 a = shapeA[i];
  float depth = solid.x, back = depth * .5;
  float inset = min(solid.y, min(a.z, min(a.w, back)) - .01) * .29;
  float lx = (k & 1) == 1 ? a.z - inset : -a.z + inset;
  float ly = (k & 2) == 2 ? a.w - inset : -a.w + inset;
  float lz = (k & 4) == 4 ? -depth + inset : -inset;
  float c = cos(slant.z), s = sin(slant.z);
  float centre = slant.z == 0. ? -back : -(a.z * abs(s) + back * abs(c));
  return vec3(a.x + c * lx - s * (lz + back), a.y + ly, centre + s * lx + c * (lz + back));
}
vec2 onScreen(vec3 w) { return w.xy - slant.xy * w.z; }
// Whether the face on an axis's high or low side (axis 0 x, 1 y, 2 z) faces the viewer.
bool toward(int axis, bool high) {
  vec3 n = axis == 0 ? vec3(high ? 1. : -1., 0., 0.)
         : axis == 1 ? vec3(0., high ? 1. : -1., 0.)
                     : vec3(0., 0., high ? -1. : 1.);
  float c = cos(slant.z), s = sin(slant.z);
  vec3 w = vec3(c * n.x - s * n.z, n.y, s * n.x + c * n.z);
  return dot(w, vec3(slant.xy, 1.)) > 1e-4;
}
const ivec2 EDGES[12] = ivec2[12](ivec2(0, 1), ivec2(2, 3), ivec2(4, 5), ivec2(6, 7),
  ivec2(0, 2), ivec2(1, 3), ivec2(4, 6), ivec2(5, 7), ivec2(0, 4), ivec2(1, 5), ivec2(2, 6),
  ivec2(3, 7));
vec4 pen(vec2 px, vec3 ro, vec3 rd, int hit, float hitT, float light) {
  float ink = 0.;
  vec3 tone = vec3(0.);
  for (int i = 0; i < ${MAX}; i++) {
    if (i >= count) break;
    vec4 a = shapeA[i];
    // Far from this bar's box on screen: nothing to draw.
    vec2 lo = vec2(1e5), hi = vec2(-1e5);
    for (int k = 0; k < 8; k++) {
      vec2 q = onScreen(corner(i, k));
      lo = min(lo, q);
      hi = max(hi, q);
    }
    if (any(lessThan(px, lo - 2.)) || any(greaterThan(px, hi + 2.))) continue;
    // The liquid's level, px from the base along the bar.
    float len = 2. * a.w, level = shapeB[i].x * (len + 2.);
    bool up = shapeB[i].y < .5;
    for (int e = 0; e < 12; e++) {
      int k0 = EDGES[e].x, k1 = EDGES[e].y, along = (k0 ^ k1) == 1 ? 0 : (k0 ^ k1) == 2 ? 1 : 2;
      // The edge shows when a face it bounds faces the viewer: the two axes it doesn't run along.
      bool shows = false;
      for (int axis = 0; axis < 3; axis++) {
        if (axis == along) continue;
        int bit = axis == 0 ? 1 : axis == 1 ? 2 : 4;
        if (toward(axis, (k0 & bit) == bit)) shows = true;
      }
      if (!shows) continue;
      vec3 w0 = corner(i, k0), w1 = corner(i, k1);
      vec2 s0 = onScreen(w0), s1 = onScreen(w1), span = s1 - s0;
      float length2 = max(dot(span, span), 1e-6), extent = sqrt(length2);
      float f = clamp(dot(px - s0, span) / length2, 0., 1.);
      // How far along the edge this pixel lies (unclamped), and how far off it.
      vec2 way = span / extent;
      float run = dot(px - s0, way), across = abs(way.x * (px.y - s0.y) - way.y * (px.x - s0.x));
      if (across > 1.5 || run < -2. || run > extent + 2.) continue;
      vec3 w = mix(w0, w1, f);
      // Another bar's glass in front of the line, or level with it, hides it: where two bars meet,
      // no sliver of the hidden one's edge peeks out.
      if (hit >= 0 && hit != i && dot(w - ro, rd) > hitT - .75) continue;
      // Seen from the side, every front stands on the front plane: a line behind it is hidden
      // wherever another bar's front covers it, with a px to spare where bars touch. Where two
      // fronts touch, their shared edge is drawn once, by the bar on the right.
      if (slant.z == 0.) {
        bool behind = w.z < -solid.y * .29 - 1.;
        bool covered = false;
        for (int j = 0; j < ${MAX}; j++) {
          if (j >= count) break;
          vec4 b = shapeA[j];
          if (j != i && (behind || b.x > a.x) && all(lessThan(abs(px - b.xy), b.zw + 1.5)))
            covered = true;
        }
        if (covered) continue;
      }
      // A skinny pen line in dashes of hand-drawn lengths, each with round ends. Every cell of the
      // edge holds one dash, its length and place drawn from the edge's own seed, so the pattern
      // never shimmers as the chart moves.
      float cells = max(1., floor(extent / 5.5 + .5)), cell = extent / cells, radius = .32;
      float gapTo = 1e5, nearest = floor(run / cell);
      for (int dk = -1; dk <= 1; dk++) {
        float k = nearest + float(dk);
        if (k < 0. || k >= cells) continue;
        float h1 = fract(sin(dot(vec2(k, float(i * 12 + e)), vec2(12.9898, 78.233))) * 43758.5453);
        float h2 = fract(h1 * 91.7 + .31);
        float size = mix(1., cell * .74, h1);
        float centre = (k + .5) * cell + (h2 - .5) * (cell - size) * .8;
        float off = max(abs(run - centre) - max(size * .5 - radius, 0.), 0.);
        gapTo = min(gapTo, sqrt(off * off + across * across) - radius);
      }
      float line = clamp(.5 - gapTo * ratio, 0., 1.);
      // Only above the liquid, easing in over its surface; further back, fainter.
      float u = up ? a.y + a.w - w.y : w.y - (a.y - a.w);
      line *= smoothstep(level - .5, level + 3., u);
      line *= mix(1., .45, clamp(-w.z / max(solid.x * 1.42, 1.), 0., 1.));
      if (line > ink) {
        ink = line;
        vec3 colour = mix(grey, tints[i].rgb, tints[i].a);
        // Pale on dark cards; on light ones a soft grey, or a pastel of the bar's colour.
        tone = mix(mix(sqrt(colour), vec3(1.), .45), mix(vec3(.62), mix(colour, vec3(1.), .15), .55 * tints[i].a), light);
      }
    }
  }
  return vec4(tone, ink * mix(.55, .7, light));
}

// One ray through a screen point: the glass it meets (premultiplied colour and coverage), with
// whatever stands behind a thin edge laid underneath. edge is set when the ray only clipped a
// solid's edge, where a single sample per pixel would stair-step.
vec4 trace(vec2 px, float light, out vec3 ro, out vec3 rd, out int id, out bool edge,
           out float clear, out float facing, out float hitT) {
  // Straight on, every ray runs parallel into the screen, so only front faces and bevels show;
  // tilted, rays leave a pinhole camera. Either way they are clipped to the scene's box.
  ro = vec3(px, boxMax.z);
  rd = vec3(0., 0., -1.);
  if (focal > 0.) {
    vec2 s = px - anchor;
    ro = camPos;
    rd = normalize(camDir * focal + camRight * s.x - camUp * s.y);
  } else if (slanted()) {
    // At a slant every ray still runs parallel, back and against the slant, so the front plane
    // (z = 0) stays where it is straight on.
    rd = normalize(vec3(-slant.xy, -1.));
    ro = vec3(px + slant.xy * boxMax.z, boxMax.z);
  }
  vec3 safe = vec3(abs(rd.x) < 1e-6 ? 1e-6 : rd.x, abs(rd.y) < 1e-6 ? 1e-6 : rd.y,
                   abs(rd.z) < 1e-6 ? 1e-6 : rd.z);
  vec3 t0 = (boxMin - ro) / safe, t1 = (boxMax - ro) / safe;
  vec3 tlo = min(t0, t1), thi = max(t0, t1);
  float t = max(max(max(tlo.x, tlo.y), tlo.z), 0.), tEnd = min(min(thi.x, thi.y), thi.z);
  id = -1;
  edge = false;
  hitT = 1e5;
  // The closest the ray passes to a solid, in screen px (a ray's neighbours spread by t / focal).
  clear = 1e5;
  facing = 1.;
  for (int k = 0; k < 128; k++) {
    if (t > tEnd) break;
    int i;
    float d = map(ro + rd * t, i);
    if (d < .04) {
      id = i;
      break;
    }
    clear = min(clear, focal > 0. ? d * focal / max(t, 1.) : d);
    t += max(d * .9, .3);
    if (t > tEnd) break;
  }
  if (id < 0) return vec4(0.);
  hitT = t;
  float exitT;
  vec4 near = surface(ro + rd * t, rd, id, light, exitT);
  facing = hitFacing;
  edge = exitT < 4.;
  // A thin edge is only partly covered, so whatever stands behind it must show through, not the
  // card: carry the ray on past this solid and lay the next one underneath.
  if (near.a < .985) {
    float t2 = t + exitT + .6;
    int behind = -1;
    for (int k = 0; k < 48; k++) {
      if (t2 > tEnd) break;
      int i;
      float d = mapExcept(ro + rd * t2, id, i);
      if (d < .04) {
        behind = i;
        break;
      }
      t2 += max(d * .9, .3);
    }
    if (behind >= 0) {
      float exit2;
      vec4 far = surface(ro + rd * t2, rd, behind, light, exit2);
      // Outlined bars seen through empty glass are veiled, as through frosted glass, so a bar's
      // side behind its neighbour's empty container reads as behind it. Only liquid shows through:
      // a neighbour's bare glass behind (its corner, its rims) would only speckle.
      if (edged) far *= liquidFront(ro + rd * t2, behind).x > -1. ? .5 : 0.;
      near.rgb += far.rgb * (1. - near.a);
      near.a += far.a * (1. - near.a);
    }
  }
  return near;
}

// Extra samples, in device pixels, taken only where a ray clipped an edge.
const vec2 JITTER[4] = vec2[4](vec2(0.), vec2(.375, .125), vec2(-.125, .375), vec2(-.25, -.375));
// Eight rays across a device pixel (one to a row and a column), where a silhouette crosses it.
const vec2 SPREAD[8] = vec2[8](vec2(-.4375, .0625), vec2(-.3125, -.3125), vec2(-.1875, .3125),
  vec2(-.0625, -.1875), vec2(.0625, .4375), vec2(.1875, -.4375), vec2(.3125, .1875),
  vec2(.4375, -.0625));

void main() {
  vec2 px = vec2(gl_FragCoord.x / ratio, size.y - gl_FragCoord.y / ratio);
  float light = step(.5, theme);
  vec3 ro, rd;
  int id;
  vec4 sum = vec4(0.);
  float samples = 0.;
  float cover = 1.;
  // How far the ray through this pixel ran before it met glass (none: very far).
  float hitT = 1e5, spare;
  vec2 at = px;
  bool angled = focal > 0. || slanted();
  if (!angled) {
    // Straight on, the outline is known exactly: a pixel it cuts is covered by its share of one
    // device px and shaded from half a device px inside it, so the cut is as clean as a vector
    // path's and carries the glass's own colour, never a rim of what lies past the edge.
    float plan = planar(px), inset = .5 / ratio;
    cover = clamp(.5 - plan * ratio, 0., 1.);
    if (cover > 0. && plan > -inset) {
      vec2 e = vec2(.25, 0.);
      vec2 g = vec2(planar(px + e.xy) - planar(px - e.xy), planar(px + e.yx) - planar(px - e.yx));
      at = px - g / max(length(g), 1e-5) * (plan + inset);
    }
  }
  if (cover <= 0.) {
    ro = vec3(px, boxMax.z);
    rd = vec3(0., 0., -1.);
    id = -1;
    samples = 1.;
  } else {
    vec3 o, r;
    int i;
    bool edge;
    float clear, facing;
    vec4 c = trace(at, light, o, r, i, edge, clear, facing, hitT);
    ro = o;
    rd = r;
    id = i;
    // Tilted or slanted, a ray that passes within a device px of a silhouette, or meets the
    // surface edge-on, sits on an outline: eight rays across the pixel share it out.
    bool silhouette = angled && (i < 0 ? clear < 1.5 / ratio : facing < .3);
    if (silhouette) {
      for (int k = 0; k < 8; k++) {
        vec4 s = trace(px + SPREAD[k] / ratio, light, o, r, i, edge, clear, facing, spare);
        sum += s;
        samples += 1.;
      }
    } else if (!edge || cover < 1.) {
      // Away from edges one sample is exact; stop there.
      sum = c;
      samples = 1.;
    } else {
      sum = c;
      samples = 1.;
      for (int k = 1; k < 4; k++) {
        sum += trace(at + JITTER[k] / ratio, light, o, r, i, edge, clear, facing, spare);
        samples += 1.;
      }
    }
  }
  vec3 premultiplied = sum.rgb / samples * cover;
  float alpha = sum.a / samples * cover;
  // The card's shadow and glow fade out before the canvas ends, so neither stops at its edge.
  float fade = margin > 0.
    ? smoothstep(0., margin, min(min(px.x, size.x - px.x), min(px.y, size.y - px.y)))
    : 1.;
  // Only the card around the glass takes the shadow; the glass's own soft edges stay clean.
  if (theme > .5 && (id < 0 || cover < 1.)) {
    vec3 mid = ro + rd * ((-solid.x * .5 - ro.z) / rd.z);
    float shade = 0.;
    for (int i = 0; i < ${MAX}; i++) {
      if (i >= count) break;
      float fill = mode == 0 ? shapeB[i].x : shapeA[i].w;
      shade += exp(-max(shell(mid - vec3(0., 10., 0.), i), 0.) / 12.) * min(fill * 1.5, 1.);
    }
    shade = min(shade, 1.) * .035 * (1. - alpha) * fade;
    alpha += shade;
  }
  // Coloured glass glows into the card around it: light on dark cards, a pastel halo on white. The
  // halo keeps as close to the glass on white as the light does on dark: a longer reach there added
  // up across the bars into a tinted panel.
  {
    vec3 mid = ro + rd * ((-solid.x * .5 - ro.z) / rd.z);
    vec3 glow = vec3(0.);
    float bloom = 0.;
    for (int i = 0; i < ${MAX}; i++) {
      if (i >= count) break;
      float fill = mode == 0 ? shapeB[i].x : shapeA[i].w;
      float w = exp(-max(shell(mid, i), 0.) / 16.) * tints[i].a * min(fill * 1.5, 1.);
      glow += tints[i].rgb * w;
      bloom += w;
    }
    glow /= max(bloom, 1e-4);
    bloom = min(bloom, 1.) * mix(.05, .1, light) * fade;
    glow = mix(glow, vec3(1.), light * .12);
    premultiplied += glow * bloom * (1. - alpha);
    alpha += bloom * (1. - alpha);
  }
  // The dashed outline of empty glass, drawn as a pen draws it, over the glass.
  if (edged && mode == 0) {
    vec4 ink = pen(px, ro, rd, id, hitT, light);
    premultiplied = premultiplied * (1. - ink.a) + ink.rgb * ink.a;
    alpha = alpha * (1. - ink.a) + ink.a;
  }
  // Bars standing on a floor: nothing below it, and the glass fades toward it, never so far that
  // a short bar's level is lost.
  if (floorFade.y > 0.) {
    float stand = step(px.y, floorFade.x) * mix(.3, 1., smoothstep(floorFade.x, floorFade.x - floorFade.y, px.y));
    premultiplied *= stand;
    alpha *= stand;
  }
  float noise = fract(52.9829189 * fract(dot(gl_FragCoord.xy, vec2(.06711056, .00583715)))) - .5;
  outputColour = vec4(premultiplied + noise / 255. * alpha, alpha);
}
`;

const NAMES = [
  "size",
  "ratio",
  "mode",
  "count",
  "hostShells",
  "shapeA[0]",
  "shapeB[0]",
  "tints[0]",
  "marks[0]",
  "field",
  "ring",
  "solid",
  "grey",
  "theme",
  "staticGlass",
  "focal",
  "lift",
  "camPos",
  "camDir",
  "camRight",
  "camUp",
  "anchor",
  "boxMin",
  "boxMax",
  "margin",
  "slant",
  "floorFade",
  "edged",
];
// Graphite: neutral smoke on dark cards, neutral silver on light ones (no tint in the greys).
const GREY: Record<Theme, Rgb> = { dark: [0.63, 0.63, 0.63], light: [0.8, 0.8, 0.8] };

type Painter = {
  gl: WebGL2RenderingContext;
  canvas: HTMLCanvasElement;
  program: WebGLProgram | null;
  buffer: WebGLBuffer | null;
  location: Record<string, WebGLUniformLocation | null>;
  /** Bumped whenever the program is rebuilt, so every client redraws. */
  generation: number;
};

function contextFor(canvas: HTMLCanvasElement) {
  try {
    return canvas.getContext("webgl2", {
      alpha: true,
      premultipliedAlpha: true,
      antialias: false,
      powerPreference: "low-power",
    });
  } catch {
    return null;
  }
}

function painterFor(canvas: HTMLCanvasElement): Painter | null {
  const gl = contextFor(canvas);
  if (!gl) return null;
  const painter: Painter = { gl, canvas, program: null, buffer: null, location: {}, generation: 0 };
  return build(painter) ? painter : null;
}

function build(painter: Painter) {
  const { gl } = painter;
  const next = gl.createProgram();
  if (!next) return false;
  for (const [type, source] of [
    [gl.VERTEX_SHADER, vertexSource],
    [gl.FRAGMENT_SHADER, fragmentSource],
  ] as const) {
    const shader = gl.createShader(type);
    if (!shader) return false;
    gl.shaderSource(shader, source);
    gl.compileShader(shader);
    if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
      console.warn("Liquid glass shader:", gl.getShaderInfoLog(shader));
      return false;
    }
    gl.attachShader(next, shader);
    gl.deleteShader(shader);
  }
  gl.linkProgram(next);
  if (!gl.getProgramParameter(next, gl.LINK_STATUS)) return false;
  painter.program = next;
  gl.useProgram(next);
  painter.buffer = gl.createBuffer();
  gl.bindBuffer(gl.ARRAY_BUFFER, painter.buffer);
  gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 3, -1, -1, 3]), gl.STATIC_DRAW);
  const position = gl.getAttribLocation(next, "position");
  gl.enableVertexAttribArray(position);
  gl.vertexAttribPointer(position, 2, gl.FLOAT, false, 0, 0);
  painter.location = Object.fromEntries(
    NAMES.map((name) => [name, gl.getUniformLocation(next, name)]),
  );
  painter.generation++;
  return true;
}

/* Charts that opt in share one context per document and copy each frame out to their own 2D
 * canvas, so a page full of charts never meets the browser's cap on live WebGL contexts. */
const sharedPainters = new WeakMap<Document, Painter | null>();
const sharedClients = new Set<() => void>();

/* The context handlers live at module level and find their painter through the event's surface.
 * A closure made inside `sharedPainter` shares the scope it is made in (after minification, the
 * first `createLiquidGlass(canvas)` call), so the shared surface kept that chart's detached page
 * alive for the life of the document. */
const surfacePainters = new WeakMap<EventTarget, Painter>();
function onContextLost(event: Event) {
  event.preventDefault();
  sharedClients.forEach((redraw) => redraw());
}
function onContextRestored(event: Event) {
  const painter = event.currentTarget ? surfacePainters.get(event.currentTarget) : undefined;
  if (painter && build(painter)) sharedClients.forEach((redraw) => redraw());
}

function sharedPainter(doc: Document): Painter | null {
  if (sharedPainters.has(doc)) return sharedPainters.get(doc) ?? null;
  const surface = doc.createElement("canvas");
  const painter = painterFor(surface);
  if (painter) {
    surfacePainters.set(surface, painter);
    surface.addEventListener("webglcontextlost", onContextLost);
    surface.addEventListener("webglcontextrestored", onContextRestored);
  }
  sharedPainters.set(doc, painter);
  return painter;
}

/** Draws glass into `canvas`, only when the scene changes. With `shared`, the canvas is a plain
 * 2D canvas fed from one context per page; without it, the canvas owns a WebGL2 context. Returns
 * null without WebGL2. */
export function createLiquidGlass(
  canvas: HTMLCanvasElement,
  {
    shared = false,
    onPaint,
  }: {
    shared?: boolean;
    onPaint?: (result: GlassPaintResult) => void;
  } = {},
) {
  const painter = shared ? sharedPainter(canvas.ownerDocument) : painterFor(canvas);
  const output = shared && painter ? canvas.getContext("2d") : null;
  if (!painter || (shared && !output)) return null;
  const p = painter;
  const gl = p.gl;
  let width = 1,
    height = 1,
    ratio = 1,
    drawn = "",
    last: GlassScene | null = null,
    // Whether the canvas shows this renderer's last scene: an engine that draws into the same
    // canvas takes it over through `invalidate`.
    showing = false;
  const shapeA = new Float32Array(MAX * 4),
    shapeB = new Float32Array(MAX * 4),
    tints = new Float32Array(MAX * 4),
    marks = new Float32Array(MAX * 4),
    heights = new Float32Array(FIELD_LENGTH * MAX + NX * MAX);
  // The solids' simulated surfaces, one block of rows per solid, in a float texture. A restored
  // context bumps the painter's generation and every old texture is gone with it; comparing the
  // generation is free, where `gl.isTexture` per draw blocked on the GPU process.
  let fieldTexture: WebGLTexture | null = null,
    fieldGeneration = -1;

  function resize(nextWidth: number, nextHeight: number) {
    const nextRatio = pixelRatio(2);
    if (nextWidth === width && nextHeight === height && nextRatio === ratio) return;
    width = nextWidth;
    height = nextHeight;
    ratio = nextRatio;
    // A lost context can't repaint until the restore: keep the picture through the resize.
    sizeCanvas(
      canvas,
      Math.max(1, Math.round(width * ratio)),
      Math.max(1, Math.round(height * ratio)),
      !!output && gl.isContextLost(),
    );
    canvas.style.width = `${width}px`;
    canvas.style.height = `${height}px`;
    drawn = "";
    // Resizing cleared the backing store: repaint the last scene now, inside the resize, so the
    // canvas is never presented blank while the next frame lays out the new size.
    if (showing && last) draw(last);
  }

  function outcome(painted: boolean, reason?: string): GlassPaintResult {
    if (!painted) drawn = "";
    const result = reason ? { painted, reason } : { painted };
    onPaint?.(result);
    return result;
  }
  function draw(scene: GlassScene): GlassPaintResult {
    last = scene;
    if (!p.program || gl.isContextLost()) return outcome(false, "context-unavailable");
    if (
      scene.kind === "bars" &&
      scene.bars.some((bar) => bar.shell && (!validFunnelShell(bar.shell) || !(scene.depth > 0)))
    )
      return outcome(false, "invalid-shell");
    const { location } = p;
    const staticGlass = isStaticGlass(canvas);
    const key = JSON.stringify(
      [scene, width, height, ratio, p.generation, staticGlass],
      (_, value) => (value instanceof Float32Array ? fieldPrint(value) : value),
    );
    if (key === drawn && showing) return outcome(true);
    const items = scene.kind === "bars" ? scene.bars : scene.segments;
    const count = Math.min(MAX, items.length);
    let hostShells = false;
    shapeA.fill(0);
    shapeB.fill(0);
    tints.fill(0);
    marks.fill(-1);
    heights.fill(0);
    items.slice(0, count).forEach((item, i) => {
      if (item.field && !staticGlass)
        heights.set(item.field.subarray(0, FIELD_LENGTH), i * FIELD_LENGTH);
    });
    if (scene.kind === "bars") {
      scene.bars.slice(0, count).forEach((bar, i) => {
        const row = FIELD_LENGTH * MAX + i * NX;
        if (bar.shell) {
          hostShells = true;
          bar.shell.forEach((point, vertex) => heights.set([point.x, point.y], row + vertex * 2));
          heights[row + 8] = 1;
        } else heights[row + 8] = 0;
        const length = Math.max(bar.height, 1),
          half = bar.width / 2,
          direction = bar.direction ?? "up";
        const bounds = bar.shell ? funnelShellBounds(bar.shell) : undefined;
        const centre = bounds
          ? [
              bounds.x + bounds.width / 2,
              bounds.y + bounds.height / 2,
              bounds.width / 2,
              bounds.height / 2,
            ]
          : direction === "up"
            ? [bar.x, bar.base - length / 2, half, length / 2]
            : direction === "down"
              ? [bar.x, bar.base + length / 2, half, length / 2]
              : direction === "right"
                ? [bar.base + length / 2, bar.x, length / 2, half]
                : [bar.base - length / 2, bar.x, length / 2, half];
        shapeA.set(centre, i * 4);
        shapeB.set(
          [
            bar.fill,
            ["up", "down", "right", "left"].indexOf(direction),
            Math.max(0, Math.min(1, bar.pattern ?? 0)),
            bar.pattern === undefined ? 0 : 1,
          ],
          i * 4,
        );
        tints.set([...bar.tint, bar.tone], i * 4);
        bar.marks?.slice(0, 4).forEach((at, k) => {
          // A graduation keeps its +2 (a tick) or +4 (a ring).
          const kind = at > 3.5 ? 4 : at > 1.5 ? 2 : 0;
          marks[i * 4 + k] = kind + Math.max(0, Math.min(1, at - kind));
        });
      });
    } else {
      scene.segments.slice(0, count).forEach((segment, i) => {
        shapeA.set([segment.start, segment.end, segment.pop, segment.fill], i * 4);
        shapeB.set(
          [
            segment.height ?? scene.ring.depth,
            segment.radius ?? 0,
            segment.width === undefined ? 0 : segment.width / 2,
            0,
          ],
          i * 4,
        );
        tints.set([...segment.tint, segment.tone], i * 4);
      });
    }
    const depth = scene.kind === "bars" ? scene.depth : scene.ring.depth;
    const backingWidth = canvas.width,
      backingHeight = canvas.height;
    if (output) {
      // The shared surface only grows; each client renders into its bottom-left corner.
      if (p.canvas.width < backingWidth) p.canvas.width = backingWidth;
      if (p.canvas.height < backingHeight) p.canvas.height = backingHeight;
    }
    gl.viewport(0, 0, backingWidth, backingHeight);
    gl.useProgram(p.program);
    gl.uniform2f(location.size, width, height);
    gl.uniform1f(location.ratio, ratio);
    gl.uniform1i(location.mode, scene.kind === "bars" ? 0 : 1);
    gl.uniform1i(location.count, count);
    gl.uniform1i(location.hostShells, hostShells ? 1 : 0);
    gl.uniform4fv(location["shapeA[0]"], shapeA);
    gl.uniform4fv(location["shapeB[0]"], shapeB);
    gl.uniform4fv(location["tints[0]"], tints);
    gl.uniform4fv(location["marks[0]"], marks);
    if (!fieldTexture || fieldGeneration !== p.generation) {
      fieldTexture = gl.createTexture();
      fieldGeneration = p.generation;
      gl.bindTexture(gl.TEXTURE_2D, fieldTexture);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
    }
    gl.activeTexture(gl.TEXTURE0);
    gl.bindTexture(gl.TEXTURE_2D, fieldTexture);
    gl.pixelStorei(gl.UNPACK_ALIGNMENT, 1);
    // The shell rows go up only with a host shell: a native scene uploads what it always has.
    gl.texImage2D(
      gl.TEXTURE_2D,
      0,
      gl.R32F,
      NX,
      (FIELD_ROWS + (hostShells ? 1 : 0)) * MAX,
      0,
      gl.RED,
      gl.FLOAT,
      heights,
    );
    gl.uniform1i(location.field, 0);
    const halfWidth = scene.kind === "ring" ? scene.ring.width / 2 : 0;
    gl.uniform4f(location.solid, depth, scene.bevel, GLASS_WALL, halfWidth);
    if (scene.kind === "ring") {
      const { x, y, radius, gap } = scene.ring;
      gl.uniform4f(location.ring, x, y, radius, gap / 2);
    }
    gl.uniform3f(location.grey, ...GREY[scene.theme]);
    gl.uniform1f(location.theme, scene.theme === "light" ? 1 : 0);
    gl.uniform1i(location.staticGlass, staticGlass ? 1 : 0);
    // Camera and the box every ray is clipped to.
    let top = 2;
    let box: [Vec3, Vec3] = [
      [0, 0, -depth - 2],
      [width, height, 2],
    ];
    let view = glassView([0, 0, -depth]);
    if (scene.kind === "ring") {
      const { x, y, radius, width: ringWidth } = scene.ring;
      let reach = radius + ringWidth / 2 + 24;
      for (const segment of scene.segments) {
        top = Math.max(top, -depth + segment.pop + (segment.height ?? depth) + 2);
        // Concentric slabs reach as far as the widest track, and a popped slab moves out.
        reach = Math.max(
          reach,
          (segment.radius ?? radius) + (segment.width ?? ringWidth) / 2 + segment.pop * 0.3 + 24,
        );
      }
      box = [
        [x - reach, y - reach, -depth - 2],
        [x + reach, y + reach, top],
      ];
      view = glassView([x, y, -depth], scene.camera);
    }
    gl.uniform1f(location.focal, view.focal);
    gl.uniform1f(location.lift, scene.kind === "ring" ? (scene.camera?.tilt ?? 0) : 0);
    gl.uniform3f(location.camPos, ...view.position);
    gl.uniform3f(location.camDir, ...view.forward);
    gl.uniform3f(location.camRight, ...view.right);
    gl.uniform3f(location.camUp, ...view.up);
    gl.uniform2f(location.anchor, view.anchor.x, view.anchor.y);
    // Straight on, the box spans the whole canvas, as the flat view always has. A turned bar
    // reaches further back than its depth: its diagonal.
    if (!view.focal) {
      const turn = scene.kind === "bars" ? (scene.slant?.turn ?? 0) : 0;
      box[0] = [0, 0, -depth * (Math.abs(Math.cos(turn)) + Math.abs(Math.sin(turn))) - 2];
      box[1] = [width, height, box[1][2]];
    }
    const slant = scene.kind === "bars" && scene.slant ? scene.slant : { x: 0, y: 0, turn: 0 };
    gl.uniform3f(location.slant, slant.x, slant.y, slant.turn);
    const floor = scene.kind === "bars" ? scene.floor : undefined;
    gl.uniform2f(location.floorFade, floor?.y ?? 0, floor?.fade ?? 0);
    gl.uniform1i(location.edged, scene.kind === "bars" && scene.edges ? 1 : 0);
    gl.uniform3f(location.boxMin, ...box[0]);
    gl.uniform3f(location.boxMax, ...box[1]);
    gl.uniform1f(location.margin, scene.kind === "bars" ? (scene.margin ?? 0) : 0);
    gl.drawArrays(gl.TRIANGLES, 0, 3);
    if (onPaint && (gl.isContextLost() || gl.getError() !== gl.NO_ERROR))
      return outcome(false, "draw-failed");
    try {
      if (output) {
        output.clearRect(0, 0, backingWidth, backingHeight);
        output.drawImage(
          p.canvas,
          0,
          p.canvas.height - backingHeight,
          backingWidth,
          backingHeight,
          0,
          0,
          backingWidth,
          backingHeight,
        );
      }
    } catch {
      return outcome(false, "canvas-transfer");
    }
    showing = true;
    drawn = key;
    return outcome(true);
  }

  const redraw = () => {
    if (last && showing) draw(last);
  };
  const lost = (event: Event) => {
    event.preventDefault();
    outcome(false, "context-lost");
  };
  const restored = () => {
    if (build(p)) redraw();
  };
  if (shared) sharedClients.add(redraw);
  else {
    canvas.addEventListener("webglcontextlost", lost);
    canvas.addEventListener("webglcontextrestored", restored);
  }
  return {
    resize,
    draw,
    /** Forget the last drawn scene, so the next draw repaints even if nothing changed (after
     * something else has drawn into or cleared the canvas). Until then a resize or a restored
     * context leaves the canvas to whoever drew it. */
    invalidate() {
      drawn = "";
      showing = false;
    },
    destroy() {
      if (fieldTexture && !gl.isContextLost()) gl.deleteTexture(fieldTexture);
      fieldTexture = null;
      if (shared) {
        sharedClients.delete(redraw);
        output?.clearRect(0, 0, canvas.width, canvas.height);
        return;
      }
      canvas.removeEventListener("webglcontextlost", lost);
      canvas.removeEventListener("webglcontextrestored", restored);
      if (p.program) gl.deleteProgram(p.program);
      if (p.buffer) gl.deleteBuffer(p.buffer);
    },
  };
}
