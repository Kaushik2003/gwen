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
import { pixelRatio } from "@/components/liquid-charts/liquid/core/render-scale";
import { isStaticGlass, sizeCanvas } from "@/components/liquid-charts/liquid/core/presence";
import type { Rgb, Theme } from "@/components/liquid-charts/liquid/core/colour";

/**
 * The liquid itself, generalised from the approved area's colour field (presentation/area-focus,
 * area-colour-field.ts) to any data, size and scale, up to eight layers, the pour, and an etched
 * projection.
 *
 * Each layer's top is a row of evenly spaced surface samples in a float texture; every fragment
 * rebuilds the surfaces above and below it with the same cubic the SVG edge uses, so the two meet
 * exactly. Under each surface the colour is lit just below the meniscus and deepens towards the
 * floor; above the top surface a soft bloom of the liquid's own colour; the body dissolves into the
 * walls; fine diagonal hatching sits inside. Focus flares the top surface round the focused x. A
 * projected stretch is etched: denser frosted lanes in the same material from where the projection
 * begins. Light theme has its own authored branch: pigment at the surface dissolving into paper.
 *
 * The liquid pours from a floor: by default up from the plot's bottom, left to right. It can also
 * hang from a ceiling (`orientation: "down"`), rise sideways from a wall (`"left"`, `"right"`), rise
 * both ways about a spine (`mirror`, a violin), pour onto a floor that moves along the run
 * (`floors`, a stream's baseline or a threshold curve), or bend round a circle (`polar`: the run is
 * the angle and the height the radius). However it is turned, the shading is the same, so the lit
 * surface always faces away from the floor. Missing samples break a run cleanly, and liquid with no
 * height draws nothing at all, not even its bloom.
 *
 * Every chart on a page draws through one WebGL2 context and copies its frame to its own 2D canvas
 * (the request spectrum's pattern), so a page of charts never meets the browser's context cap.
 */

/** Which way the liquid rises from its floor. `up` (the default) stands on the plot's bottom;
 * `down` hangs from its top; `right` rises from its left wall and `left` from its right wall. */
export type LiquidOrientation = "up" | "down" | "left" | "right";

/** A circle for the liquid to bend round. The run (x along the plot, from `plot.left` to
 * `plot.right`) becomes the angle, clockwise from `from` (radians from 12 o'clock) through `span`;
 * the height (y, from `plot.bottom` up to `plot.top`) becomes the radius, from `inner` (the floor)
 * out to `outer`. Canvas px. For even light make the plot about `span` × the middle radius wide and
 * `outer − inner` tall. A full turn (`span` = 2π) joins itself at the seam; there are no walls. */
export type LiquidPolar = {
  cx: number;
  cy: number;
  inner: number;
  outer: number;
  from: number;
  span: number;
};

export type LiquidScene = {
  theme: Theme;
  /** The plot in canvas CSS px: the walls and the floor. */
  plot: { left: number; top: number; right: number; bottom: number };
  /** Up to `LIQUID_LAYERS` layers, bottom first; each its top surface at evenly spaced samples
   * along the run (NaN in gaps). Canvas px across the run: y for `up` and `down`, x for `left` and
   * `right`. */
  surfaces: readonly ArrayLike<number>[];
  /** The first sample's position along the run (x for `up`/`down`, y for `left`/`right`) and the
   * step between samples, canvas px. A negative step runs the other way (right to left, or bottom
   * to top), and the pour follows it. Liquid lies only between the first and last sample, so the
   * plot's walls can sit past the data, clear of it. */
  x0: number;
  step: number;
  /** Optional explicit positions in run order (same length as each surface).
   * Nondecreasing in the liquid frame; duplicates retain vertical steps. Uses
   * linear boundary segments instead of the legacy uniform cubic. */
  xs?: ArrayLike<number>;
  /** Colour stops per layer (a gradient along the run when there are several). */
  colours: readonly (readonly Rgb[])[];
  /** The pour's leading edge along the run and its nose length, canvas px (nose 0 once poured). */
  tip: number;
  nose: number;
  /** The focused position along the run, canvas px, and how present its light is (0–1). */
  focusX: number;
  focus: number;
  /** Where an etched projection begins along the run, canvas px, or null. */
  project: number | null;
  /** Which way the liquid rises from its floor. Default `up`. */
  orientation?: LiquidOrientation;
  /** Rise both ways from the floor, symmetric about it (a violin about its spine). */
  mirror?: boolean;
  /** For a mirrored body: how far past the spine the light's floor sits, as a share of the tallest
   * point, so the halves meet in liquid instead of fading to nothing at the spine. Default 0.55. */
  spineDepth?: number;
  /** A floor per sample (same spacing as the surfaces), canvas px across the run; NaN is a gap.
   * Default: the plot's floor (its bottom for `up`, top for `down`, left for `right`, right for
   * `left`). */
  floors?: ArrayLike<number> | null;
  /** Bend the liquid round a circle (`orientation` is then ignored). */
  polar?: LiquidPolar | null;
  /** Per sample, px the coat the liquid left on the glass reaches above the top surface (it
   * falls back and the coat drains). Absent or 0, no coat. */
  coat?: ArrayLike<number> | null;
};

/** How a scene is drawn onto the renderer's canvas. */
export type LiquidDrawOptions = {
  /** Only this part of the canvas (CSS px) is poured and copied: cheaper for a small shape on a
   * large canvas. Default the whole canvas. */
  region?: { left: number; top: number; right: number; bottom: number } | null;
  /** Composite onto what the canvas already shows, through its 2D context's current blend mode,
   * alpha and clip, instead of replacing it: several pours on one canvas (ridges over each other,
   * overlaid densities). The owner then redraws everything itself after a resize. */
  keep?: boolean;
  /** Confirm the draw with `gl.getError()` (a round trip to the GPU process), for a caller that
   * reports paint. Native pours never ask: without it a draw costs what it always has. */
  verify?: boolean;
};

/** How many layers one canvas pours. */
export const LIQUID_LAYERS = 8;
const MAX_LAYERS = LIQUID_LAYERS;
const MAX_STOPS = 4;
const MISSING = -1e5;
const TURN = Math.PI * 2;
/** The hatching's and the etching's lane periods, px. */
const HATCH = 18;
const ETCH = 7;
/** Beyond this the projection sentinels mean "all" or "none" whichever way the run goes. */
const FAR = 1e5;

const vertexSource = `#version 300 es
in vec2 position;
void main() { gl_Position = vec4(position, 0., 1.); }`;

const fragmentSource = `#version 300 es
precision highp float;
precision highp sampler2D;
uniform vec2 uSize;
uniform float uRatio;
uniform vec3 uMapU;
uniform vec3 uMapV;
uniform vec4 uPlot;
uniform int uLayers;
uniform int uSamples;
uniform float uX0;
uniform float uStep;
uniform sampler2D uSurface;
uniform sampler2D uAbscissae;
uniform float uExplicit;
// The coat above the top surface, px per sample (only read when uCoated is set).
uniform sampler2D uCoat;
uniform float uCoated;
uniform vec3 uStops[${MAX_LAYERS * MAX_STOPS}];
uniform int uStopCount[${MAX_LAYERS}];
uniform float uTip;
uniform float uNose;
uniform float uFocusX;
uniform float uFocus;
uniform float uProject;
uniform float uTheme;
uniform bool uStaticGlass;
uniform float uMirror;
uniform float uExt;
uniform vec4 uPolar;
uniform vec2 uTurn;
uniform float uWrap;
uniform vec2 uLanes;
uniform float uFloors;
out vec4 outputColour;
// How much of this device pixel lies on the inside of an edge d px away (positive inside): a box
// one device px wide, so an outline is as fine as a vector path's at any pixel ratio.
float cover(float d) { return clamp(.5 + d * uRatio, 0., 1.); }

// Everything below works in the liquid's own frame: x runs along the plot in the pour's
// direction, y grows towards the floor. Each texel holds a sample's surface, its floor, and the
// first and last sample of the run it belongs to.
vec4 texel(int k, int i) {
  return texelFetch(uSurface, ivec2(clamp(i, 0, uSamples - 1), k), 0);
}

// The coat above the top surface at x, px.
float coatAt(float x);
// A full turn joins itself: positions past either end come round from the other.
float wrapped(float x) {
  return uWrap > 0. ? uPlot.x + mod(x - uPlot.x, uWrap) : x;
}
float abscissa(int i) {
  return uExplicit > .5 ? texelFetch(uAbscissae, ivec2(clamp(i, 0, uSamples - 1), 0), 0).r : uX0 + float(i) * uStep;
}
// Upper-bound lookup retains the right side of a vertical step. Sixteen
// iterations cover the renderer's texture width without a per-pixel scan.
int explicitIndex(float x) {
  int lo = 0, hi = uSamples;
  for (int n = 0; n < 16; n++) {
    if (lo >= hi) break;
    int mid = (lo + hi) / 2;
    if (abscissa(mid) <= x) lo = mid + 1;
    else hi = mid;
  }
  return clamp(lo - 1, 0, uSamples - 1);
}
float coatAt(float x) {
  if (uCoated < .5) return 0.;
  if (uExplicit > .5) {
    float w = wrapped(x);
    int i = explicitIndex(w);
    int j = min(i + 1, uSamples - 1);
    float t = clamp((w - abscissa(i)) / max(1e-6, abscissa(j) - abscissa(i)), 0., 1.);
    return mix(texelFetch(uCoat, ivec2(i, 0), 0).r, texelFetch(uCoat, ivec2(j, 0), 0).r, t);
  }
  float s = (wrapped(x) - uX0) / uStep;
  int i = clamp(int(floor(s)), 0, uSamples - 2);
  float t = clamp(s - float(i), 0., 1.);
  return mix(texelFetch(uCoat, ivec2(i, 0), 0).r, texelFetch(uCoat, ivec2(i + 1, 0), 0).r, t);
}

// A layer's surface and the floor under it at x, by the same cubic as the SVG edge: Catmull-Rom
// tangents, one-sided at the ends of a run (round the seam on a full turn). The surface is -1e5
// in a gap.
vec2 curvesAt(int k, float x) {
  if (uExplicit > .5) {
    float w = wrapped(x);
    if (w < abscissa(0) || w > abscissa(uSamples - 1)) return vec2(-1e5, uPlot.w);
    int i = explicitIndex(w);
    vec2 a = texel(k, i).rg;
    float from = abscissa(i);
    if (w == from || i == uSamples - 1) return a;
    vec2 b = texel(k, i + 1).rg;
    if (a.x < -1e4 || b.x < -1e4) return vec2(-1e5, uPlot.w);
    vec2 c = mix(a, b, (w - from) / max(1e-6, abscissa(i + 1) - from));
    if (uFloors < .5) c.y = uPlot.w;
    return c;
  }
  float s = (wrapped(x) - uX0) / uStep;
  int last = uSamples - 1;
  int i = int(floor(s));
  float t = s - float(i);
  if (i >= last) { i = last - 1; t = 1.; }
  if (i < 0) { i = 0; t = 0.; }
  vec2 a = texel(k, i).rg;
  vec2 b = texel(k, i + 1).rg;
  vec2 c;
  if (a.x < -1e4 || b.x < -1e4) {
    // Exactly on a run's end.
    if (a.x > -1e4 && t < 1e-3) c = a;
    else if (b.x > -1e4 && t > .999) c = b;
    else return vec2(-1e5, uPlot.w);
  } else {
    vec2 before = i > 0 ? texel(k, i - 1).rg : uWrap > 0. ? texel(k, last - 1).rg : vec2(-1e5);
    vec2 after = i + 2 <= last ? texel(k, i + 2).rg : uWrap > 0. ? texel(k, 1).rg : vec2(-1e5);
    if (before.x < -1e4) before = 2. * a - b;
    if (after.x < -1e4) after = 2. * b - a;
    vec2 m0 = (b - before) * .5;
    vec2 m1 = (after - a) * .5;
    float t2 = t * t;
    float t3 = t2 * t;
    c = (2. * t3 - 3. * t2 + 1.) * a + (t3 - 2. * t2 + t) * m0 + (-2. * t3 + 3. * t2) * b + (t3 - t2) * m1;
  }
  // Without floors the floor is the plot's, exactly.
  if (uFloors < .5) c.y = uPlot.w;
  return c;
}

// The run of known samples round x in layer k, as [first, last] along the run; empty when x is in
// a gap.
vec2 runAt(int k, float x) {
  float w = wrapped(x);
  int i = uExplicit > .5 ? explicitIndex(w) : clamp(int(floor((w - uX0) / uStep)), 0, uSamples - 1);
  vec4 here = texel(k, i);
  if (here.r < -1e4) {
    // Just before a run's first sample.
    vec4 next = texel(k, i + 1);
    if (i + 1 > uSamples - 1 || next.r < -1e4) return vec2(1., -1.);
    here = next;
  }
  float shift = x - w;
  if (uExplicit > .5) return vec2(abscissa(int(here.b)) + shift, abscissa(int(here.a)) + shift);
  return vec2(uX0 + here.b * uStep + shift, uX0 + here.a * uStep + shift);
}

// The pour lowers every surface inside its rounded leading edge (a quarter ellipse) to the floor.
float poured(float y, float x, float base) {
  if (uNose <= .001) return y;
  float u = clamp((uTip - x) / uNose, 0., 1.);
  return base - (base - y) * sqrt(u * (2. - u));
}

// A surface as poured, held to its floor, and the spine under it (-1e5 in a gap).
vec2 pouredAt(int k, float x) {
  vec2 c = curvesAt(k, x);
  if (c.x < -1e4) return c;
  float base = c.y + uExt;
  return vec2(min(poured(c.x, x, base), base), c.y);
}

// How much liquid there is to light: none where it has no height, all from a little over half a px.
float presenceOf(float height) { return smoothstep(0., .6, height); }
// The glow fades sooner, as the liquid thins to a thread: a tail lying along its floor does not
// draw a line of light.
float glowOf(float height) { return smoothstep(0., 1.25, height); }

// pow(0, 0) is undefined in GLSL, so integer powers multiply.
float ipow(float b, int e) {
  float r = 1.;
  for (int i = 0; i < 4; i++) {
    if (i >= e) break;
    r *= b;
  }
  return r;
}

vec3 colourAt(int k, float t) {
  int n = uStopCount[k];
  if (n <= 1) return uStops[k * 4];
  float u = clamp(t, 0., 1.);
  float s = 1. - u;
  vec3 c = vec3(0.);
  float binomial = 1.;
  for (int i = 0; i < 4; i++) {
    if (i >= n) break;
    c += binomial * ipow(u, i) * ipow(s, n - 1 - i) * uStops[k * 4 + i];
    binomial = binomial * float(n - 1 - i) / float(i + 1);
  }
  return c;
}

void main() {
  vec2 frag = vec2(gl_FragCoord.x / uRatio, uSize.y - gl_FragCoord.y / uRatio);
  bool polar = uPolar.w > uPolar.z;
  vec2 p;
  if (polar) {
    // Angle along the run, radius up from the floor.
    vec2 d = frag - uPolar.xy;
    // A zero d.y (the row through the centre) is nudged below it: some GPUs return the opposite
    // half-turn for atan(x, -0.), a seam across the dial at 3 and 9 o'clock.
    float a = mod(atan(d.x, abs(d.y) < 1e-4 ? -1e-4 : -d.y) - uTurn.x, 6.28318531);
    float width = uPlot.z - uPlot.x;
    float u;
    if (a <= uTurn.y) u = uPlot.x + a / uTurn.y * width;
    else {
      float past = a - uTurn.y, before = 6.28318531 - a;
      u = past < before ? uPlot.z + past / uTurn.y * width : uPlot.x - before / uTurn.y * width;
    }
    float v = uPlot.w - (length(d) - uPolar.z) / (uPolar.w - uPolar.z) * (uPlot.w - uPlot.y);
    p = vec2(u, v);
  } else p = vec2(dot(uMapU, vec3(frag, 1.)), dot(uMapV, vec3(frag, 1.)));

  float left = uPlot.x;
  float right = min(uPlot.z, uTip);
  if (uLayers < 1) {
    outputColour = vec4(0.);
    return;
  }
  int last = uLayers - 1;
  // The stretch this pixel can draw in: its run, inside the walls and behind the pour.
  vec2 run = runAt(last, p.x);
  float lo = run.x, hi = run.y;
  if (uWrap <= 0. || uTip < uPlot.z) {
    lo = max(lo, left);
    hi = min(hi, right);
  }
  if (lo > hi || p.x < lo - 1. || p.x > hi + 1.) {
    outputColour = vec4(0.);
    return;
  }
  float x = clamp(p.x, lo, hi);
  float spine = curvesAt(last, x).y;
  if (uMirror > .5 && p.y > spine) p.y = 2. * spine - p.y;
  float base = spine + uExt;
  if (p.y > base + .5) {
    outputColour = vec4(0.);
    return;
  }
  float tops[${MAX_LAYERS}];
  for (int k = 0; k < ${MAX_LAYERS}; k++) {
    tops[k] = base;
    if (k < uLayers) {
      float y = curvesAt(k, x).x;
      tops[k] = y < -1e4 ? -1e5 : min(poured(y, x, base), base);
    }
  }
  float top = tops[last];
  if (top < -1e4) {
    outputColour = vec4(0.);
    return;
  }
  float presence = presenceOf(spine - top);
  // The coat: a thin translucent film of the liquid left on the glass above the surface.
  float coat = coatAt(x);
  float film = coat > .3 ? cover(p.y - (top - coat)) * cover(-(p.y - top)) : 0.;
  // Focus is shown by the liquid itself, as the line material shows it: the surface flares in a
  // short, soft stretch round the focused x, with only a faint, shallow glow beneath it. No marks.
  float fromFocus = p.x - uFocusX;
  if (uWrap > 0.) fromFocus = mod(fromFocus + uWrap * .5, uWrap) - uWrap * .5;
  float flare = uFocus * exp(-(fromFocus / 30.) * (fromFocus / 30.));
  float focus = uFocus * exp(-(fromFocus / 40.) * (fromFocus / 40.));
  // At the focus the bloom grows brighter and a little taller: the surface burns there.
  float reach = 6.5 + 4. * flare;
  // How far the bloom can be seen: 18 px, more where the surface burns.
  float bloomReach = 18. + 10. * flare;
  float raw = p.y - top;
  if (raw < -bloomReach && film <= 0.) {
    // Far above its own surface a pixel can still be in the bloom of liquid beside it (a steep
    // wall, or the liquid's end over a dry stretch): look either side before giving up.
    bool near = false;
    for (int i = -2; i <= 2; i++) {
      if (i == 0) continue;
      float sx = clamp(x + float(i) * 7.5, lo, hi);
      vec2 there = pouredAt(last, sx);
      if (there.x > -1e4 && there.y - there.x > 0.
          && length(vec2(sx - x, max(there.x - p.y, 0.))) < bloomReach + 4.) near = true;
    }
    if (!near) {
      outputColour = vec4(0.);
      return;
    }
  }
  // Distance to the surface keeps the light even on slopes. Below it, along the local normal;
  // above it (the bloom), the nearest point of the surface round it (a polyline through samples
  // either side), so a steep corner does not throw a fin of light into the air and a steep wall's
  // glow is smooth. Each piece of surface glows as much as there is liquid under it, so where
  // there is none there is no glow.
  float dist;
  float glow = glowOf(spine - top);
  if (raw < 0.) {
    // Its own column first, exactly; then a polyline through points on a fixed 2.5 px grid (the
    // same for every pixel, so the glow has no grain).
    float d = presence > 0. ? -raw : 1e5;
    glow *= exp(-(raw / reach) * (raw / reach));
    vec2 at = vec2(x, p.y);
    vec2 before = vec2(0.);
    float beforeLit = 0.;
    bool joined = false;
    float grid = floor(x / 2.5) * 2.5;
    int span = flare > .01 ? 11 : 7;
    for (int i = -11; i <= 12; i++) {
      if (i < -span || i > span + 1) continue;
      // Held to the run, so its ends (a wall, a gap) are on the line too.
      float sx = clamp(grid + float(i) * 2.5, lo, hi);
      vec2 there = pouredAt(last, sx);
      if (there.x < -1e4) {
        joined = false;
        continue;
      }
      vec2 point = vec2(sx, there.x);
      float lit = glowOf(there.y - there.x);
      if (joined && max(lit, beforeLit) > 0.) {
        vec2 along = point - before;
        float h = clamp(dot(at - before, along) / max(dot(along, along), 1e-6), 0., 1.);
        float to = length(at - before - along * h);
        float there = mix(beforeLit, lit, h);
        if (there > 0.) {
          d = min(d, to);
          glow = max(glow, there * exp(-(to / reach) * (to / reach)));
        }
      }
      before = point;
      beforeLit = lit;
      joined = true;
    }
    dist = -d;
  } else {
    float ax = min(x + 1., hi), bx = max(x - 1., lo);
    float ahead = pouredAt(last, ax).x;
    float behind = pouredAt(last, bx).x;
    float slope = ahead < -1e4 || behind < -1e4 ? 0. : (ahead - behind) * .5;
    dist = raw / sqrt(1. + slope * slope);
  }
  float t = clamp((x - left) / max(1., right - left), 0., 1.);
  // Round a circle the walls sit beyond the span: nothing dims inside it.
  float wallDistance = polar ? 1e5 : min(p.x - left, right - p.x);
  float wallEdge = cover(p.x - lo) * cover(hi - p.x);
  float walls = mix(.12, 1., smoothstep(0., 40., wallDistance));
  float rim = mix(.55, 1., smoothstep(0., 24., wallDistance));
  // Depth is measured from a softened surface (a short weighted average of the height across x,
  // within the run), so a sharp dip does not let light run deeper in its own column and draw a
  // streak. On gentle data the average equals the surface itself.
  float soft = (base - top) * .4;
  float weight = .4;
  for (int i = -2; i <= 2; i++) {
    if (i == 0) continue;
    float sx = clamp(x + float(i) * 14., lo, hi);
    vec2 there = pouredAt(last, sx);
    if (there.x < -1e4) continue;
    float w = i == -1 || i == 1 ? .2 : .1;
    soft += (there.y + uExt - there.x) * w;
    weight += w;
  }
  soft /= weight;
  float softTop = base - soft;
  float depth = clamp((p.y - softTop) / max(1., soft), 0., 1.);
  float pool = focus * exp(-max(p.y - min(top, softTop), 0.) / 16.) * cover(dist) * .5;
  bool light = uTheme > .5;
  bool etched = p.x >= uProject;

  vec3 premultiplied = vec3(0.);
  float alpha = 0.;
  vec3 topColour = colourAt(last, t);
  // Each layer, top down: the share of this pixel inside it, and its own shading.
  float below = 1.;
  for (int k = ${MAX_LAYERS - 1}; k >= 0; k--) {
    if (k > last) continue;
    float layerTop = tops[k];
    float layerBottom = k == 0 ? base : tops[k - 1];
    float edge = k == last ? dist : p.y - layerTop;
    float inside = cover(edge) * cover(layerBottom - p.y + (k == 0 ? 1. : 0.));
    if (inside <= 0.) continue;
    vec3 colour = colourAt(k, t);
    float local = clamp((p.y - layerTop) / max(1., layerBottom - layerTop), 0., 1.);
    float under = max(p.y - layerTop, 0.);
    // Light caught just under the surface (fainter at the interfaces inside a stack).
    // Static glass keeps a narrow optical edge instead of a broad liquid meniscus.
    float meniscus = (uStaticGlass ? .45 * exp(-under / 1.2) : exp(-under / 5.)) * (k == last ? 1. : .7);
    // The flare lives in the top surface's meniscus, a few px deep.
    float burn = k == last ? flare * exp(-under / 6.) : 0.;
    float body;
    if (uLayers == 1) body = .85 * pow(1. - depth, 2.1);
    else body = mix(.3, .82, pow(1. - local, 1.25)) * (1. - .3 * depth);
    float a;
    vec3 lit;
    if (light) {
      float wash = uLayers == 1 ? .46 * pow(1. - depth, 1.65) : mix(.2, .5, pow(1. - local, 1.25)) * (1. - .25 * depth);
      if (etched) wash *= .8;
      float skin = meniscus * .56 * rim;
      a = max(max(wash, pool * .32) * walls, max(skin, burn * .95)) * wallEdge;
      lit = mix(colour, vec3(.96), meniscus * .16);
      // On paper the surface burns as richer ink.
      lit = mix(lit, mix(colour, vec3(.08), .38), burn);
    } else {
      if (etched) body *= .8;
      lit = mix(colour, mix(colour, vec3(1.), .38), meniscus * .65);
      lit = mix(lit, mix(colour, vec3(1.), .3), pool * .55);
      lit = mix(lit, mix(colour, vec3(1.), .88), burn);
      a = max(max(body, pool * .6) * walls, max(meniscus * .92 * rim, burn)) * wallEdge;
    }
    // Fine diagonal hatching inside the material, fading in below the surface and out with depth.
    float lane = abs(fract((p.x + p.y) / uLanes.x) - .5) * uLanes.x;
    float hatch = (1. - smoothstep(.35, 1.3, lane)) * smoothstep(4., 36., under)
                * pow(1. - depth, .7) * (.07 + .05 * focus) * walls * wallEdge;
    if (uStaticGlass) hatch = 0.;
    // An etched projection: denser frosted lanes in the same material.
    if (etched) {
      float etch = abs(fract((p.x + p.y) / uLanes.y) - .5) * uLanes.y;
      float frost = (1. - smoothstep(.5, 1.5, etch)) * smoothstep(1., 10., under)
                  * mix(.3, .14, depth) * walls * wallEdge;
      hatch = max(hatch, frost * (light ? .9 : 1.));
    }
    vec3 lanes = light ? mix(colour, vec3(1.), .6) : mix(colour, vec3(1.), .55);
    vec3 shaded = lit * a;
    shaded = lanes * hatch + shaded * (1. - hatch);
    a = hatch + a * (1. - hatch);
    float share = inside * below;
    premultiplied += shaded * share;
    alpha += a * share;
    below *= 1. - inside;
  }
  // Liquid with no height draws nothing.
  premultiplied *= presence;
  alpha *= presence;
  if (film > 0.) {
    float fa = film * (light ? .16 : .22) * wallEdge * presence;
    premultiplied += mix(topColour, vec3(1.), light ? .35 : .2) * fa * (1. - alpha);
    alpha += fa * (1. - alpha);
  }
  // A soft bloom above the top surface in the liquid's own colour, brighter at the selection.
  if (!light && !uStaticGlass) {
    float outside = 1. - cover(dist);
    float bloom = glow * outside * (.26 + 1.1 * flare) * rim * wallEdge;
    premultiplied += topColour * bloom * (1. - alpha);
    alpha += bloom * (1. - alpha);
  }
  // Less than one output level of stable dither prevents 8-bit banding.
  float noise = fract(52.9829189 * fract(dot(gl_FragCoord.xy, vec2(.06711056, .00583715)))) - .5;
  outputColour = vec4(premultiplied + noise / 255. * alpha, alpha);
}
`;

const NAMES = [
  "uSize",
  "uRatio",
  "uMapU",
  "uMapV",
  "uPlot",
  "uLayers",
  "uSamples",
  "uX0",
  "uStep",
  "uSurface",
  "uAbscissae",
  "uExplicit",
  "uCoat",
  "uCoated",
  "uStops[0]",
  "uStopCount[0]",
  "uTip",
  "uNose",
  "uFocusX",
  "uFocus",
  "uProject",
  "uTheme",
  "uStaticGlass",
  "uMirror",
  "uExt",
  "uPolar",
  "uTurn",
  "uWrap",
  "uLanes",
  "uFloors",
];

/* ——— The liquid's own frame (pure, so it is tested without a GPU) ——— */

/** How a scene maps into the liquid's own frame: x along the run in the pour's direction, y
 * growing towards the floor (the frame every scene is shaded in). */
export type LiquidFrame = {
  /** Canvas px → frame: `u = mapU·[x, y, 1]`, `v = mapV·[x, y, 1]` (unused round a circle). */
  mapU: [number, number, number];
  mapV: [number, number, number];
  /** The plot in the frame: the walls left and right, the floor at the bottom. */
  plot: { left: number; top: number; right: number; bottom: number };
  /** A canvas position across the run (a surface or a floor) → the frame's y. */
  across: (value: number) => number;
  /** A canvas position along the run → the frame's x. */
  along: (value: number) => number;
  /** 1, or −1 when the samples run backwards. */
  sign: 1 | -1;
};

export function liquidFrame(
  scene: Pick<LiquidScene, "plot" | "step" | "orientation" | "polar">,
): LiquidFrame {
  const { left: L, top: T, right: R, bottom: B } = scene.plot;
  if (scene.polar)
    return {
      mapU: [1, 0, 0],
      mapV: [0, 1, 0],
      plot: { left: L, top: T, right: R, bottom: B },
      across: (value) => value,
      along: (value) => value,
      sign: 1,
    };
  const s: 1 | -1 = scene.step < 0 ? -1 : 1;
  const sideways = scene.orientation === "left" || scene.orientation === "right";
  const [a, b] = sideways ? [T, B] : [L, R];
  const runExtent = { left: Math.min(s * a, s * b), right: Math.max(s * a, s * b) };
  const along = (value: number) => s * value;
  switch (scene.orientation) {
    case "down":
      return {
        mapU: [s, 0, 0],
        mapV: [0, -1, T + B],
        plot: { ...runExtent, top: T, bottom: B },
        across: (value) => T + B - value,
        along,
        sign: s,
      };
    case "right":
      return {
        mapU: [0, s, 0],
        mapV: [-1, 0, L + R],
        plot: { ...runExtent, top: L, bottom: R },
        across: (value) => L + R - value,
        along,
        sign: s,
      };
    case "left":
      return {
        mapU: [0, s, 0],
        mapV: [1, 0, 0],
        plot: { ...runExtent, top: L, bottom: R },
        across: (value) => value,
        along,
        sign: s,
      };
    default:
      return {
        mapU: [s, 0, 0],
        mapV: [0, 1, 0],
        plot: { ...runExtent, top: T, bottom: B },
        across: (value) => value,
        along,
        sign: s,
      };
  }
}

/**
 * The runs of known samples in a row: for each sample its run's first and last index (NaN for a
 * missing sample). On a full turn (`wrap`) a run that reaches both ends continues round the seam:
 * the first run starts before 0 and the last ends past the end, by the turn's length in samples.
 */
export function liquidRuns(
  known: ArrayLike<boolean>,
  { wrap = false }: { wrap?: boolean } = {},
): { start: Float32Array; end: Float32Array } {
  const n = known.length;
  const start = new Float32Array(n).fill(NaN),
    end = new Float32Array(n).fill(NaN);
  let from = -1;
  for (let j = 0; j <= n; j++) {
    const here = j < n && known[j];
    if (here && from < 0) from = j;
    if (!here && from >= 0) {
      for (let i = from; i < j; i++) {
        start[i] = from;
        end[i] = j - 1;
      }
      from = -1;
    }
  }
  if (wrap && n > 1 && known[0] && known[n - 1]) {
    const period = n - 1;
    const firstEnd = end[0],
      lastStart = start[n - 1];
    if (lastStart === 0) {
      // One run all the way round.
      start.fill(-period);
      end.fill(2 * period);
    } else {
      for (let i = 0; i <= firstEnd; i++) start[i] = lastStart - period;
      for (let i = lastStart; i < n; i++) end[i] = firstEnd + period;
    }
  }
  return { start, end };
}

/**
 * Packs a scene's layers into the texture the shader reads, in the liquid's frame: per sample its
 * surface, floor, and run (first and last index). A sample is missing (and breaks the run) when its
 * surface or its floor is not finite. Returns the packed texels and the tallest liquid (frame px,
 * top layer over its floor), for a mirrored body's depth.
 */
export function packLiquid(
  scene: Pick<LiquidScene, "surfaces" | "floors" | "polar">,
  frame: LiquidFrame,
  layers: number,
  samples: number,
  into?: Float32Array<ArrayBuffer>,
): { texels: Float32Array<ArrayBuffer>; tallest: number } {
  const texels =
    into && into.length === samples * layers * 4 ? into : new Float32Array(samples * layers * 4);
  const wrap = !!scene.polar && scene.polar.span >= TURN - 1e-6;
  const floorOf = (j: number) => {
    const f = scene.floors?.[j];
    return scene.floors
      ? Number.isFinite(f)
        ? frame.across(f as number)
        : NaN
      : frame.plot.bottom;
  };
  const floors = Array.from({ length: samples }, (_, j) => floorOf(j));
  let tallest = 0;
  for (let k = 0; k < layers; k++) {
    const row = scene.surfaces[k];
    const known = Array.from(
      { length: samples },
      (_, j) => Number.isFinite(row[j]) && Number.isFinite(floors[j]),
    );
    const { start, end } = liquidRuns(known, { wrap });
    for (let j = 0; j < samples; j++) {
      const at = (k * samples + j) * 4;
      const surface = known[j] ? frame.across(row[j]) : MISSING;
      texels[at] = surface;
      texels[at + 1] = Number.isFinite(floors[j]) ? floors[j] : MISSING;
      texels[at + 2] = known[j] ? start[j] : 0;
      texels[at + 3] = known[j] ? end[j] : -1;
      if (k === layers - 1 && known[j]) tallest = Math.max(tallest, floors[j] - surface);
    }
  }
  return { texels, tallest };
}

/** A position along the run in the liquid's frame; the projection's far sentinels keep their
 * meaning (everything etched, or nothing) whichever way the run goes. */
function alongFrame(frame: LiquidFrame, value: number | null, fallback: number) {
  if (value === null || !Number.isFinite(value)) return fallback;
  if (value <= -FAR) return -1e6;
  if (value >= FAR) return 1e6;
  return frame.along(value);
}

type Painter = {
  gl: WebGL2RenderingContext;
  canvas: HTMLCanvasElement;
  program: WebGLProgram;
  location: Record<string, WebGLUniformLocation | null>;
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
    // A denied or lost graphics service must not stop the chart: it draws flat instead.
    return null;
  }
}

function build(gl: WebGL2RenderingContext) {
  const program = gl.createProgram();
  if (!program) return null;
  for (const [type, source] of [
    [gl.VERTEX_SHADER, vertexSource],
    [gl.FRAGMENT_SHADER, fragmentSource],
  ] as const) {
    const shader = gl.createShader(type);
    if (!shader) return null;
    gl.shaderSource(shader, source);
    gl.compileShader(shader);
    if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
      console.warn("Liquid shader:", gl.getShaderInfoLog(shader));
      return null;
    }
    gl.attachShader(program, shader);
    gl.deleteShader(shader);
  }
  gl.linkProgram(program);
  if (!gl.getProgramParameter(program, gl.LINK_STATUS)) return null;
  gl.useProgram(program);
  const buffer = gl.createBuffer();
  gl.bindBuffer(gl.ARRAY_BUFFER, buffer);
  gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 3, -1, -1, 3]), gl.STATIC_DRAW);
  const position = gl.getAttribLocation(program, "position");
  gl.enableVertexAttribArray(position);
  gl.vertexAttribPointer(position, 2, gl.FLOAT, false, 0, 0);
  const location = Object.fromEntries(
    NAMES.map((name) => [name, gl.getUniformLocation(program, name)]),
  );
  return { program, location };
}

const painters = new WeakMap<Document, Painter | null>();
const clients = new Set<() => void>();

/* The context handlers live at module level and find their painter through the event's surface.
 * A closure made inside `sharedPainter` shares the scope it is made in (after minification, the
 * first `createLiquidRenderer(canvas)` call), so the shared surface kept that chart's detached
 * page alive for the life of the document. */
const surfacePainters = new WeakMap<EventTarget, Painter>();
function onContextLost(event: Event) {
  event.preventDefault();
}
function onContextRestored(event: Event) {
  const painter = event.currentTarget ? surfacePainters.get(event.currentTarget) : undefined;
  const rebuilt = painter && build(painter.gl);
  if (!painter || !rebuilt) return;
  Object.assign(painter, rebuilt);
  painter.generation++;
  clients.forEach((redraw) => redraw());
}

function sharedPainter(doc: Document): Painter | null {
  if (painters.has(doc)) return painters.get(doc) ?? null;
  const surface = doc.createElement("canvas");
  const gl = contextFor(surface);
  const built = gl && build(gl);
  const painter = gl && built ? { gl, canvas: surface, ...built, generation: 0 } : null;
  if (painter) {
    surfacePainters.set(surface, painter);
    surface.addEventListener("webglcontextlost", onContextLost);
    surface.addEventListener("webglcontextrestored", onContextRestored);
  }
  painters.set(doc, painter);
  return painter;
}

/** True when this page can draw liquid (WebGL2 with float textures). Cheap after the first call. */
export function liquidAvailable(doc: Document = document) {
  return sharedPainter(doc) !== null;
}

/**
 * A liquid renderer for `canvas` (a plain 2D canvas fed from the page's shared WebGL2 context).
 * Returns null without WebGL2, and the chart then draws flat. While the context is lost the canvas
 * keeps its last frame (`lost` is true); on the restore the renderer repaints its last scene, and
 * `onRestore` tells an owner that pours several scenes (`keep`) to repaint them all.
 */
export function createLiquidRenderer(
  canvas: HTMLCanvasElement,
  { onRestore }: { onRestore?: () => void } = {},
) {
  const painter = sharedPainter(canvas.ownerDocument);
  const output = painter ? canvas.getContext("2d") : null;
  if (!painter || !output) return null;
  const p = painter;
  const gl = p.gl;
  const maxTextureSamples = gl.getParameter(gl.MAX_TEXTURE_SIZE) as number;
  let texture: WebGLTexture | null = null,
    xTexture: WebGLTexture | null = null,
    xPacked = new Float32Array(0),
    xLegacy = false,
    coatTexture: WebGLTexture | null = null,
    coatPacked = new Float32Array(0),
    generation = -1,
    textureSize = "",
    width = 1,
    height = 1,
    ratio = 1,
    packed = new Float32Array(0),
    last: LiquidScene | null = null,
    lastOptions: LiquidDrawOptions | undefined;
  const stops = new Float32Array(MAX_LAYERS * MAX_STOPS * 3);
  const counts = new Int32Array(MAX_LAYERS);

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
      gl.isContextLost(),
    );
    canvas.style.width = `${width}px`;
    canvas.style.height = `${height}px`;
    redraw();
  }

  function draw(scene: LiquidScene, options?: LiquidDrawOptions) {
    last = scene;
    const staticGlass = isStaticGlass(canvas);
    // Geometry is settled by presence; remove transient lighting and fluid residue too.
    if (staticGlass) scene = { ...scene, coat: null, focus: 0, nose: 0, tip: Infinity };
    lastOptions = options;
    if (gl.isContextLost()) return false;
    const layers = Math.min(MAX_LAYERS, scene.surfaces.length);
    const samples = layers ? scene.surfaces[0].length : 0;
    const backingWidth = canvas.width,
      backingHeight = canvas.height;
    // The part of the backing store to pour, in device px.
    const region = options?.region;
    const x0 = region ? Math.max(0, Math.floor(region.left * ratio)) : 0,
      y0 = region ? Math.max(0, Math.floor(region.top * ratio)) : 0,
      x1 = region ? Math.min(backingWidth, Math.ceil(region.right * ratio)) : backingWidth,
      y1 = region ? Math.min(backingHeight, Math.ceil(region.bottom * ratio)) : backingHeight;
    if (!options?.keep) output!.clearRect(0, 0, backingWidth, backingHeight);
    if (!layers || samples < 2 || x1 <= x0 || y1 <= y0) return false;
    if (scene.xs && (scene.xs.length !== samples || samples > maxTextureSamples || samples > 65535))
      return false;
    if (generation !== p.generation || !texture) {
      texture = gl.createTexture();
      gl.bindTexture(gl.TEXTURE_2D, texture);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
      coatTexture = gl.createTexture();
      gl.bindTexture(gl.TEXTURE_2D, coatTexture);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
      xTexture = gl.createTexture();
      xLegacy = false;
      gl.bindTexture(gl.TEXTURE_2D, xTexture);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
      generation = p.generation;
      textureSize = "";
    }
    const frame = liquidFrame(scene);
    const { texels, tallest } = packLiquid(scene, frame, layers, samples, packed);
    packed = texels;
    gl.activeTexture(gl.TEXTURE0);
    gl.bindTexture(gl.TEXTURE_2D, texture);
    gl.pixelStorei(gl.UNPACK_ALIGNMENT, 4);
    const size = `${samples}x${layers}`;
    if (size !== textureSize) {
      gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA32F, samples, layers, 0, gl.RGBA, gl.FLOAT, packed);
      textureSize = size;
    } else gl.texSubImage2D(gl.TEXTURE_2D, 0, 0, 0, samples, layers, gl.RGBA, gl.FLOAT, packed);

    stops.fill(0);
    counts.fill(1);
    for (let k = 0; k < layers; k++) {
      const colours = scene.colours[k]?.length ? scene.colours[k] : [[0.56, 0.56, 0.56] as const];
      counts[k] = Math.min(MAX_STOPS, colours.length);
      for (let i = 0; i < counts[k]; i++) stops.set(colours[i], (k * MAX_STOPS + i) * 3);
    }

    const polar = scene.polar ?? null;
    const plot = frame.plot;
    const wrap = polar && polar.span >= TURN - 1e-6 ? plot.right - plot.left : 0;
    // On a full turn the lanes divide the turn exactly, so they meet themselves at the seam.
    const lanes = (period: number) =>
      wrap ? wrap / Math.max(1, Math.round(wrap / period)) : period;

    // The shared surface only grows; each client renders into its bottom-left corner.
    if (p.canvas.width < backingWidth) p.canvas.width = backingWidth;
    if (p.canvas.height < backingHeight) p.canvas.height = backingHeight;
    gl.viewport(0, 0, backingWidth, backingHeight);
    gl.useProgram(p.program);
    const { location } = p;
    gl.uniform2f(location.uSize, width, height);
    gl.uniform1f(location.uRatio, ratio);
    gl.uniform3f(location.uMapU, ...frame.mapU);
    gl.uniform3f(location.uMapV, ...frame.mapV);
    gl.uniform4f(location.uPlot, plot.left, plot.top, plot.right, plot.bottom);
    gl.uniform1i(location.uLayers, layers);
    gl.uniform1i(location.uSamples, samples);
    gl.uniform1f(location.uX0, frame.along(scene.x0));
    gl.uniform1f(location.uStep, Math.max(1e-3, Math.abs(scene.step)));
    gl.uniform1i(location.uSurface, 0);
    const explicit = !!scene.xs && scene.xs.length === samples;
    // Always bind a complete float texture, including after another client used
    // explicit geometry. Legacy clients keep their original interpolation path, and their one
    // texel goes up once, not on every frame of a pour.
    gl.activeTexture(gl.TEXTURE2);
    gl.bindTexture(gl.TEXTURE_2D, xTexture);
    if (explicit) {
      if (xPacked.length !== samples) xPacked = new Float32Array(samples);
      for (let j = 0; j < samples; j++) xPacked[j] = frame.along(scene.xs![j]);
      gl.texImage2D(gl.TEXTURE_2D, 0, gl.R32F, samples, 1, 0, gl.RED, gl.FLOAT, xPacked);
      xLegacy = false;
    } else if (!xLegacy) {
      gl.texImage2D(gl.TEXTURE_2D, 0, gl.R32F, 1, 1, 0, gl.RED, gl.FLOAT, new Float32Array(1));
      xLegacy = true;
    }
    gl.activeTexture(gl.TEXTURE0);
    gl.uniform1i(location.uAbscissae, 2);
    gl.uniform1f(location.uExplicit, explicit ? 1 : 0);
    // The coat, when there is one: px per sample, on its own small texture.
    const coat = scene.coat;
    const coated =
      !!coat && coat.length === samples && Array.prototype.some.call(coat, (c: number) => c > 0.3);
    if (coated) {
      if (coatPacked.length !== samples) coatPacked = new Float32Array(samples);
      for (let j = 0; j < samples; j++) coatPacked[j] = Number.isFinite(coat[j]) ? coat[j] : 0;
      gl.activeTexture(gl.TEXTURE1);
      gl.bindTexture(gl.TEXTURE_2D, coatTexture);
      gl.pixelStorei(gl.UNPACK_ALIGNMENT, 4);
      gl.texImage2D(gl.TEXTURE_2D, 0, gl.R32F, samples, 1, 0, gl.RED, gl.FLOAT, coatPacked);
      gl.activeTexture(gl.TEXTURE0);
    }
    gl.uniform1i(location.uCoat, 1);
    gl.uniform1f(location.uCoated, coated ? 1 : 0);
    gl.uniform3fv(location["uStops[0]"], stops);
    gl.uniform1iv(location["uStopCount[0]"], counts);
    gl.uniform1f(location.uTip, alongFrame(frame, scene.tip, 1e6));
    gl.uniform1f(location.uNose, scene.nose);
    gl.uniform1f(location.uFocusX, alongFrame(frame, scene.focusX, 0));
    gl.uniform1f(location.uFocus, scene.focus);
    gl.uniform1f(location.uProject, alongFrame(frame, scene.project, 1e6));
    gl.uniform1f(location.uTheme, scene.theme === "light" ? 1 : 0);
    gl.uniform1i(location.uStaticGlass, staticGlass ? 1 : 0);
    const mirror = !!scene.mirror && !polar;
    gl.uniform1f(location.uMirror, mirror ? 1 : 0);
    gl.uniform1f(location.uExt, mirror ? (scene.spineDepth ?? 0.55) * tallest : 0);
    if (polar) {
      gl.uniform4f(
        location.uPolar,
        polar.cx,
        polar.cy,
        polar.inner,
        Math.max(polar.outer, polar.inner + 1e-3),
      );
      gl.uniform2f(location.uTurn, polar.from, Math.max(1e-6, Math.min(TURN, polar.span)));
    } else {
      gl.uniform4f(location.uPolar, 0, 0, 0, 0);
      gl.uniform2f(location.uTurn, 0, TURN);
    }
    gl.uniform1f(location.uWrap, wrap);
    gl.uniform2f(location.uLanes, lanes(HATCH), lanes(ETCH));
    gl.uniform1f(location.uFloors, scene.floors ? 1 : 0);
    if (region) {
      gl.enable(gl.SCISSOR_TEST);
      gl.scissor(x0, backingHeight - y1, x1 - x0, y1 - y0);
    }
    gl.clearColor(0, 0, 0, 0);
    gl.clear(gl.COLOR_BUFFER_BIT);
    gl.drawArrays(gl.TRIANGLES, 0, 3);
    if (region) gl.disable(gl.SCISSOR_TEST);
    const copy = () =>
      output!.drawImage(
        p.canvas,
        x0,
        p.canvas.height - backingHeight + y0,
        x1 - x0,
        y1 - y0,
        x0,
        y0,
        x1 - x0,
        y1 - y0,
      );
    if (options?.keep) {
      // Through the context's own blend mode, alpha and clip, but at device px.
      output!.save();
      output!.setTransform(1, 0, 0, 1, 0, 0);
      copy();
      output!.restore();
    } else copy();
    return !gl.isContextLost() && (!options?.verify || gl.getError() === gl.NO_ERROR);
  }

  // A canvas that keeps several pours is redrawn by its owner.
  const redraw = () => {
    if (last && !lastOptions?.keep) draw(last, lastOptions);
    onRestore?.();
  };
  clients.add(redraw);
  return {
    resize,
    draw,
    /** The shared context is lost: draws do nothing and the canvas keeps its last frame. */
    get lost() {
      return gl.isContextLost();
    },
    destroy() {
      clients.delete(redraw);
      if (texture && !gl.isContextLost()) gl.deleteTexture(texture);
      if (coatTexture && !gl.isContextLost()) gl.deleteTexture(coatTexture);
      if (xTexture && !gl.isContextLost()) gl.deleteTexture(xTexture);
      output!.clearRect(0, 0, canvas.width, canvas.height);
    },
  };
}

export type LiquidRenderer = NonNullable<ReturnType<typeof createLiquidRenderer>>;
