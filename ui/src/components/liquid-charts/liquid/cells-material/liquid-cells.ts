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
"use client";

import { pixelRatio } from "@/components/liquid-charts/liquid/core/render-scale";
import { chartPresence, frameDelta, sizeCanvas, substeps } from "@/components/liquid-charts/liquid/core/presence";
import { dimmed, INK, luminance, SURFACE } from "@/components/liquid-charts/liquid/core/colour-scale";
import { focusPresses, trackPointer } from "@/components/liquid-charts/liquid/core/liquid-pointer";
import type { Rgb, Theme } from "@/components/liquid-charts/liquid/core/colour";
import type { Rect } from "@/components/liquid-charts/liquid/core/geometry";
import { useEffect, useRef, useState } from "react";

/**
 * Liquid cells: the liquid for marks whose colour is the value (heatmap and calendar cells, map
 * tiles and regions). Each cell is a small pool filled with liquid in its exact scale colour. The
 * material's light lives only where the liquid meets the glass: a meniscus a few px deep under the
 * surface (on a region, under every edge that faces up), and a fine wall at the edges. Past about
 * 4 px from the surface and 2 px from the walls the body is the scale colour itself, untouched,
 * so the colour still reads true. On light cards the meniscus is a paper-white sheen and the wall
 * a deeper edge of the cell's own pigment.
 *
 * Etching carries meaning, never decoration: a cell with no value is empty glass with frosted
 * lanes; an `etched` cell (projected, incomplete, outside a target) has the same lanes inside its
 * liquid.
 *
 * The liquid pours on entrance (each pool rises from its floor after its delay, hits the brim and
 * settles with a short slosh) and ripples under the pointer: a sweep or a press sends a wave out
 * across neighbouring pools, which dip and tilt as it passes. `level` below 1 fills a pool part
 * way instead (the value told twice: colour and level).
 *
 * Every pool on a canvas is one instanced draw call on one WebGL2 context shared by every chart on
 * the page, copied to the chart's own 2D canvas. Regions (`rings`) keep their edges on a small
 * float texture that is uploaded only when the geometry changes; their distance field is solved
 * per pixel. Without WebGL2 `available` is false and the chart draws flat.
 */

export type LiquidCellShape =
  | { kind: "rect"; rect: Rect; radius: number }
  | { kind: "rings"; rings: readonly (readonly (readonly [number, number])[])[] };

export type LiquidCell = {
  key: string;
  shape: LiquidCellShape;
  /** The exact scale colour; null for no value (empty, etched glass). */
  colour: Rgb | null;
  /** 1 lit, 0 dimmed towards graphite. Eases. */
  lit: number;
  /** How full, 0–1. Default 1: to the brim. */
  level?: number;
  /** Another state of the same value (projected, incomplete, outside a target): frosted lanes
   * inside the liquid. */
  etched?: boolean;
  /** Seconds after the entrance starts that this pool pours. */
  delay?: number;
  /** In focus: lifted (a rect grows by `grow` px, a region scales about its anchor), ringed, and
   * its meniscus burns. */
  focus?: boolean;
  grow?: number;
  /** A quiet inner ring (today). */
  mark?: boolean;
  /** The point a region lifts about. Default: its bounds' centre. */
  anchor?: readonly [number, number];
};

export type LiquidCellsScene = {
  theme: Theme;
  /** Where plot px start on the canvas, CSS px. */
  offset: { x: number; y: number };
  pools: readonly LiquidPoolFrame[];
  /** Pointer waves: x, y (plot px), start time and strength (px). */
  drops: readonly { x: number; y: number; t: number; amp: number }[];
  time: number;
};

/** A pool as drawn this frame. */
export type LiquidPoolFrame = {
  shape: LiquidCellShape;
  colour: Rgb;
  presence: number;
  level: number;
  tilt: number;
  appear: number;
  focus: number;
  tone: number;
  etched: boolean;
  mark: boolean;
  markLight: boolean;
  /** Rect growth (px) or region lift (scale) from focus. */
  grow: number;
  lift: number;
  anchor: readonly [number, number];
};

const EDGE_WIDTH = 2048;
/** Floats per instance: six vec4 attributes. */
const STRIDE = 24;
const MAX_DROPS = 4;

const poolVertex = `#version 300 es
precision highp float;
in vec2 aCorner;
in vec4 iBox;
in vec4 iShape;
in vec4 iColour;
in vec4 iState;
in vec4 iKind;
in vec4 iAnchor;
uniform vec2 uSize;
uniform vec2 uOffset;
uniform vec4 uDrops[${MAX_DROPS}];
uniform float uTime;
out vec2 vPos;
flat out vec4 vShape;
flat out vec4 vColour;
flat out vec4 vState;
flat out vec4 vKind;
flat out vec4 vAnchor;
flat out vec2 vWave;

// The pointer's waves at a point: packets running outward from each drop, fading as they go.
float wave(vec2 at) {
  float h = 0.;
  for (int i = 0; i < ${MAX_DROPS}; i++) {
    vec4 d = uDrops[i];
    float age = uTime - d.z;
    if (d.w <= 0. || age < 0. || age > 2.2) continue;
    float r = length(at - d.xy);
    float front = 150. * age;
    float env = exp(-pow((r - front) / 34., 2.)) * exp(-age * 2.4) * d.w;
    h += env * cos((r - front) / 8.);
  }
  return h;
}

void main() {
  vec2 a = iAnchor.xy;
  float lift = iAnchor.z;
  vec2 corner = mix(iBox.xy, iBox.zw, aCorner);
  vPos = a + (corner - a) * lift;
  float cy = (iShape.y + iShape.w) * .5;
  float hl = wave(vec2(iShape.x, cy)), hr = wave(vec2(iShape.z, cy));
  // The surface dips by the mean and tilts by the difference across the pool.
  vWave = vec2(abs(hl + hr) * .5, (hr - hl) * .5);
  vShape = iShape;
  vColour = iColour;
  vState = iState;
  vKind = iKind;
  vAnchor = iAnchor;
  vec2 canvasPx = vPos + uOffset;
  gl_Position = vec4(canvasPx.x / uSize.x * 2. - 1., 1. - canvasPx.y / uSize.y * 2., 0., 1.);
}`;

const poolFragment = `#version 300 es
precision highp float;
precision highp sampler2D;
in vec2 vPos;
flat in vec4 vShape;
flat in vec4 vColour;
flat in vec4 vState;
flat in vec4 vKind;
flat in vec4 vAnchor;
flat in vec2 vWave;
uniform sampler2D uEdges;
uniform float uTheme;
uniform float uRatio;
uniform vec3 uInk;
out vec4 outColour;

float boxSd(vec2 p, vec4 r, float radius) {
  vec2 c = (r.xy + r.zw) * .5;
  vec2 h = max((r.zw - r.xy) * .5, vec2(0.));
  float k = min(radius, min(h.x, h.y));
  vec2 q = abs(p - c) - h + k;
  return length(max(q, 0.)) + min(max(q.x, q.y), 0.) - k;
}

// A region's signed distance (even-odd), and the vertical distance up to its boundary above.
vec2 regionSd(vec2 p, int start, int count) {
  float d = 1e9, up = 1e9;
  bool inside = false;
  for (int i = 0; i < 8192; i++) {
    if (i >= count) break;
    int k = start + i;
    vec4 e = texelFetch(uEdges, ivec2(k % ${EDGE_WIDTH}, k / ${EDGE_WIDTH}), 0);
    vec2 a = e.xy, b = e.zw;
    vec2 pa = p - a, ba = b - a;
    float h = clamp(dot(pa, ba) / max(dot(ba, ba), 1e-6), 0., 1.);
    d = min(d, length(pa - ba * h));
    if ((a.y > p.y) != (b.y > p.y) && p.x < (b.x - a.x) * (p.y - a.y) / (b.y - a.y) + a.x)
      inside = !inside;
    if ((a.x > p.x) != (b.x > p.x)) {
      float y = a.y + (b.y - a.y) * (p.x - a.x) / (b.x - a.x);
      if (y <= p.y) up = min(up, p.y - y);
    }
  }
  return vec2(inside ? -d : d, up);
}

vec2 shapeSd(vec2 q) {
  if (vKind.x < .5) return vec2(boxSd(q, vShape, vKind.y), q.y - vShape.y);
  return regionSd(q, int(vKind.y + .5), int(vKind.z + .5));
}

float lanes(vec2 q, float period, float width) {
  float lane = abs(fract((q.x + q.y) / period) - .5) * period;
  return 1. - smoothstep(width * .5, width * .5 + .7, lane);
}

void main() {
  bool light = uTheme > .5;
  float px = 1. / uRatio;
  vec2 a = vAnchor.xy;
  float lift = vAnchor.z;
  vec2 q = a + (vPos - a) / lift;
  vec2 s = shapeSd(q) * lift;
  // Regions sit half a px in from their shared borders, so neighbours part on a hairline.
  float edge = s.x + (vKind.x < .5 ? 0. : .5);
  float aa = .55 * px + .12;
  float inside = smoothstep(aa, -aa, edge);
  float focus = vState.w;
  float appear = vState.z;
  // Outside the shape only a lifted pool's shadow draws.
  if (inside <= 0. && focus <= .001) discard;

  float flags = vKind.w;
  bool etched = mod(flags, 2.) >= 1.;
  bool mark = mod(floor(flags / 2.), 2.) >= 1.;
  bool markLight = mod(floor(flags / 4.), 2.) >= 1.;
  float wall = max(-edge, 0.);
  float presence = vColour.a;
  float tone = vAnchor.w;
  vec3 C = vColour.rgb;

  // The surface: the level (with its slosh and the pointer's wave) or, at the brim, the shape's
  // own top edges, whichever is nearer.
  float cx = (vShape.x + vShape.z) * .5, hw = max((vShape.z - vShape.x) * .5, 1.);
  float level = vState.x;
  float surfaceY = mix(vShape.w, vShape.y, level) + vWave.x + (vState.y + vWave.y) * (q.x - cx) / hw;
  float under = min((q.y - surfaceY) * lift, s.y);
  // The meniscus: liquid climbs the glass, so in a rect the surface sits a hair below the brim in
  // the middle and meets it at the walls.
  if (vKind.x < .5) {
    float side = min(q.x - vShape.x, vShape.z - q.x) * lift;
    under -= .9 * (1. - exp(-max(side, 0.) / 2.2)) * smoothstep(.0, 1.5, (q.y - vShape.y) * lift + 1.);
  }
  float liquid = smoothstep(-aa, aa, under) * presence;

  // Light only where liquid meets glass. The surface's crest catches the light; just under it
  // the surface bends the light away (a darker band); the walls are a fine deeper edge. Past
  // about 4 px from the surface and 2 px from the walls the body is the exact colour.
  // Larger pools carry a broader meniscus (up to about twice a cell's), so a district reads as
  // liquid too; the body beyond it is still exact.
  float k = clamp(min(vShape.z - vShape.x, vShape.w - vShape.y) / 44., 1., 2.);
  float u = max(under, 0.) / k;
  float wk = wall / k;
  float crest = exp(-u / .75) * (1. - smoothstep(1.4, 2.4, u));
  float band = exp(-pow((u - 2.1) / .9, 2.)) * (1. - smoothstep(3.4, 4.2, u));
  float rim = exp(-wk / .6) * (1. - smoothstep(1.3, 2.1, wk)) * (1. - crest);
  float strength = ((light ? .62 : .5) * mix(.55, 1., tone) + .3 * focus);
  vec3 lit = mix(C, vec3(1.), crest * strength);
  lit = mix(lit, C * (light ? .84 : .8), band * (light ? .34 : .36));
  lit = mix(lit, C * (light ? .7 : .74), rim * (light ? .5 : .42));
  if (etched) {
    float l = lanes(q, 5., 1.) * smoothstep(.5, 2.5, under);
    lit = mix(lit, mix(C, vec3(1.), light ? .7 : .58), l * (light ? .62 : .5));
  }

  // The glass above the liquid (or an empty cell): a faint pane, a hairline wall, and on dark
  // cards a soft bloom of the liquid's colour just over its surface.
  float hair = 1. - smoothstep(.25, .25 + 1.1 * px + .2, wall);
  float pane = light ? .035 : .045;
  float glassA = pane + hair * (light ? .14 : .15);
  vec3 glassC = uInk * glassA;
  if (presence < .999) {
    float l = lanes(q, 4., 1.) * (1. - presence);
    glassC += uInk * l * (light ? .16 : .17);
    glassA += l * (light ? .16 : .17);
  }
  if (!light && presence > 0.) {
    float bloom = exp(-max(-under, 0.) / 3.) * .2 * presence * (1. - liquid);
    glassC += C * bloom;
    glassA += bloom;
  }

  vec3 colour = lit * liquid + glassC * (1. - liquid);
  float alpha = liquid + glassA * (1. - liquid);
  colour *= inside;
  alpha *= inside;

  // Today: a quiet inner ring.
  if (mark) {
    float inset = clamp(min(vShape.z - vShape.x, vShape.w - vShape.y) / 7., 1.5, 2.5);
    float ring = (1. - smoothstep(.3, .3 + px + .2, abs(wall - inset - .6))) * inside;
    vec3 ringInk = markLight ? vec3(.95) : vec3(.08);
    colour = mix(colour, ringInk * alpha, ring * .8);
  }
  // In focus: an ink ring just inside the edge, and a soft shadow under the lifted pool.
  if (focus > .001) {
    float ring = (1. - smoothstep(.4, .4 + px + .2, abs(wall - .75))) * inside * focus;
    colour = mix(colour, uInk * alpha, ring * .92);
    float shadowSd = shapeSd(q - vec2(0., 3. * focus) / lift).x * lift;
    // The shadow is black, so only its alpha adds.
    float shade = focus * (light ? .22 : .6) * exp(-max(shadowSd, 0.) / 5.) * (1. - inside);
    alpha += shade * (1. - alpha);
  }
  colour *= appear;
  alpha *= appear;
  float noise = fract(52.9829189 * fract(dot(gl_FragCoord.xy, vec2(.06711056, .00583715)))) - .5;
  outColour = vec4(colour + noise / 255. * alpha, alpha);
}`;

/* ——— The density field: a liquid poured under a dense scatter ——— */

const densityVertex = `#version 300 es
in vec2 aCorner;
uniform vec4 uPlot;
uniform vec2 uSize;
out vec2 vUv;
void main() {
  vUv = aCorner;
  vec2 p = mix(uPlot.xy, uPlot.zw, aCorner);
  gl_Position = vec4(p.x / uSize.x * 2. - 1., 1. - p.y / uSize.y * 2., 0., 1.);
}`;

const densityFragment = `#version 300 es
precision highp float;
precision highp sampler2D;
in vec2 vUv;
uniform sampler2D uField;
uniform vec3 uColourA;
uniform vec3 uColourB;
uniform float uSeries;
uniform float uTheme;
uniform float uShore;
uniform float uRatio;
out vec4 outColour;

// One series' liquid: depth is density; a meniscus lights the shoreline, and faint terraces
// mark equal depths, so the pool reads as a relief of where points crowd.
vec4 pool(float depth, vec3 C, bool light) {
  float w = max(fwidth(depth), 1e-4);
  float inside = smoothstep(-.7, .7, (depth - uShore) / w);
  if (inside <= 0.) return vec4(0.);
  float fromShore = max(depth - uShore, 0.) / w / uRatio;
  float men = exp(-fromShore / 1.3) * (1. - smoothstep(2.5, 4.5, fromShore));
  float d = clamp((depth - uShore) / max(1. - uShore, 1e-3), 0., 1.);
  float terrace = 0.;
  for (int i = 1; i <= 3; i++) {
    float t = uShore + (1. - uShore) * float(i) * .25;
    float dist = abs(depth - t) / w / uRatio;
    terrace = max(terrace, 1. - smoothstep(.3, 1.1, dist));
  }
  float body = light ? mix(.07, .2, pow(d, .75)) : mix(.09, .3, pow(d, .75));
  vec3 lit = light ? C : mix(C, vec3(1.), .08 * d);
  float a = body;
  vec3 col = lit * a;
  float shoreA = light ? .5 : .55;
  vec3 shoreC = light ? C * .7 : mix(C, vec3(1.), .4);
  col = mix(col, shoreC * shoreA, men * .9);
  a = mix(a, shoreA, men * .9);
  float ta = light ? .16 : .14;
  vec3 tc = light ? C * .7 : mix(C, vec3(1.), .3);
  col = mix(col, tc * max(a, ta), terrace * .7);
  a = mix(a, max(a, ta), terrace * .7);
  return vec4(col, a) * inside;
}

void main() {
  bool light = uTheme > .5;
  vec2 f = texture(uField, vUv).rg;
  vec4 a = pool(f.r, uColourA, light);
  vec4 b = uSeries > 1.5 ? pool(f.g, uColourB, light) : vec4(0.);
  vec4 o = b + a * (1. - b.a);
  float noise = fract(52.9829189 * fract(dot(gl_FragCoord.xy, vec2(.06711056, .00583715)))) - .5;
  outColour = vec4(o.rgb + noise / 255. * o.a, o.a);
}`;

type Program = { program: WebGLProgram; location: Record<string, WebGLUniformLocation | null> };
type Painter = {
  gl: WebGL2RenderingContext;
  canvas: HTMLCanvasElement;
  pools: Program;
  density: Program;
  quad: WebGLBuffer;
  generation: number;
};

function compile(gl: WebGL2RenderingContext, vertex: string, fragment: string, names: string[]) {
  const program = gl.createProgram();
  if (!program) return null;
  for (const [type, source] of [
    [gl.VERTEX_SHADER, vertex],
    [gl.FRAGMENT_SHADER, fragment],
  ] as const) {
    const shader = gl.createShader(type);
    if (!shader) return null;
    gl.shaderSource(shader, source);
    gl.compileShader(shader);
    if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
      console.warn("Liquid cells shader:", gl.getShaderInfoLog(shader));
      return null;
    }
    gl.attachShader(program, shader);
    gl.deleteShader(shader);
  }
  gl.bindAttribLocation(program, 0, "aCorner");
  gl.linkProgram(program);
  if (!gl.getProgramParameter(program, gl.LINK_STATUS)) return null;
  const location = Object.fromEntries(
    names.map((name) => [name, gl.getUniformLocation(program, name)]),
  );
  return { program, location };
}

function build(gl: WebGL2RenderingContext) {
  const pools = compile(gl, poolVertex, poolFragment, [
    "uSize",
    "uOffset",
    "uDrops[0]",
    "uTime",
    "uEdges",
    "uTheme",
    "uRatio",
    "uInk",
  ]);
  const density = compile(gl, densityVertex, densityFragment, [
    "uPlot",
    "uSize",
    "uField",
    "uColourA",
    "uColourB",
    "uSeries",
    "uTheme",
    "uShore",
    "uRatio",
  ]);
  const quad = gl.createBuffer();
  if (!pools || !density || !quad) return null;
  gl.bindBuffer(gl.ARRAY_BUFFER, quad);
  gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([0, 0, 1, 0, 0, 1, 1, 1]), gl.STATIC_DRAW);
  return { pools, density, quad };
}

const painters = new WeakMap<Document, Painter | null>();
const clients = new Set<() => void>();
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
  let gl: WebGL2RenderingContext | null = null;
  try {
    gl = surface.getContext("webgl2", {
      alpha: true,
      premultipliedAlpha: true,
      antialias: false,
      powerPreference: "low-power",
    });
  } catch {
    gl = null;
  }
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

/** Sizes the 2D output canvas and keeps the shared surface large enough. */
function makeOutput(canvas: HTMLCanvasElement, gl: WebGL2RenderingContext) {
  let width = 1,
    height = 1,
    ratio = 1;
  return {
    get width() {
      return width;
    },
    get height() {
      return height;
    },
    get ratio() {
      return ratio;
    },
    /** Returns true when the size changed. */
    resize(nextWidth: number, nextHeight: number) {
      const nextRatio = pixelRatio(2);
      if (nextWidth === width && nextHeight === height && nextRatio === ratio) return false;
      width = nextWidth;
      height = nextHeight;
      ratio = nextRatio;
      sizeCanvas(
        canvas,
        Math.max(1, Math.round(width * ratio)),
        Math.max(1, Math.round(height * ratio)),
        gl.isContextLost(),
      );
      canvas.style.width = `${width}px`;
      canvas.style.height = `${height}px`;
      return true;
    },
  };
}

function copyOut(painter: Painter, output: CanvasRenderingContext2D, w: number, h: number) {
  output.drawImage(painter.canvas, 0, painter.canvas.height - h, w, h, 0, 0, w, h);
}

function prepare(painter: Painter, w: number, h: number) {
  const gl = painter.gl;
  if (painter.canvas.width < w) painter.canvas.width = w;
  if (painter.canvas.height < h) painter.canvas.height = h;
  gl.viewport(0, 0, w, h);
  gl.disable(gl.SCISSOR_TEST);
  gl.clearColor(0, 0, 0, 0);
  gl.clear(gl.COLOR_BUFFER_BIT);
  gl.enable(gl.BLEND);
  gl.blendFunc(gl.ONE, gl.ONE_MINUS_SRC_ALPHA);
}

/** Packs regions' edges for the shader, in plot px. Returns the texels and each ring set's
 * offset and count. */
function packEdges(pools: readonly LiquidPoolFrame[], into: Float32Array<ArrayBuffer>) {
  let total = 0;
  for (const pool of pools)
    if (pool.shape.kind === "rings")
      for (const ring of pool.shape.rings) total += ring.length > 2 ? ring.length : 0;
  const rows = Math.max(1, Math.ceil(total / EDGE_WIDTH));
  const size = rows * EDGE_WIDTH * 4;
  const texels: Float32Array<ArrayBuffer> = into.length >= size ? into : new Float32Array(size);
  const spans: { start: number; count: number }[] = [];
  let k = 0;
  for (const pool of pools) {
    if (pool.shape.kind !== "rings") {
      spans.push({ start: 0, count: 0 });
      continue;
    }
    const start = k;
    for (const ring of pool.shape.rings) {
      if (ring.length < 3) continue;
      for (let i = 0; i < ring.length; i++) {
        const a = ring[i],
          b = ring[(i + 1) % ring.length];
        texels[k * 4] = a[0];
        texels[k * 4 + 1] = a[1];
        texels[k * 4 + 2] = b[0];
        texels[k * 4 + 3] = b[1];
        k++;
      }
    }
    spans.push({ start, count: k - start });
  }
  return { texels, rows, spans };
}

function boundsOf(shape: LiquidCellShape): Rect {
  if (shape.kind === "rect") return shape.rect;
  let left = Infinity,
    top = Infinity,
    right = -Infinity,
    bottom = -Infinity;
  for (const ring of shape.rings)
    for (const [x, y] of ring) {
      if (x < left) left = x;
      if (x > right) right = x;
      if (y < top) top = y;
      if (y > bottom) bottom = y;
    }
  return left > right ? { left: 0, top: 0, right: 0, bottom: 0 } : { left, top, right, bottom };
}

/** A liquid cells renderer for `canvas` (a 2D canvas fed from the page's shared WebGL2 context).
 * Returns null without WebGL2; the chart then draws flat. */
export function createLiquidCells(canvas: HTMLCanvasElement) {
  const painter = sharedPainter(canvas.ownerDocument);
  const output = painter ? canvas.getContext("2d") : null;
  if (!painter || !output) return null;
  const p = painter;
  const gl = p.gl;
  const size = makeOutput(canvas, gl);
  let vao: WebGLVertexArrayObject | null = null,
    instances: WebGLBuffer | null = null,
    edges: WebGLTexture | null = null,
    generation = -1,
    capacity = 0,
    packed = new Float32Array(0),
    edgeTexels = new Float32Array(0),
    edgeRows = 0,
    edgeKey: readonly unknown[] = [],
    spans: { start: number; count: number }[] = [],
    last: LiquidCellsScene | null = null;
  const drops = new Float32Array(MAX_DROPS * 4);

  function setup() {
    vao = gl.createVertexArray();
    instances = gl.createBuffer();
    edges = gl.createTexture();
    gl.bindVertexArray(vao);
    gl.bindBuffer(gl.ARRAY_BUFFER, p.quad);
    gl.enableVertexAttribArray(0);
    gl.vertexAttribPointer(0, 2, gl.FLOAT, false, 0, 0);
    gl.bindBuffer(gl.ARRAY_BUFFER, instances);
    const names = ["iBox", "iShape", "iColour", "iState", "iKind", "iAnchor"];
    names.forEach((name, i) => {
      const at = gl.getAttribLocation(p.pools.program, name);
      if (at < 0) return;
      gl.enableVertexAttribArray(at);
      gl.vertexAttribPointer(at, 4, gl.FLOAT, false, STRIDE * 4, i * 16);
      gl.vertexAttribDivisor(at, 1);
    });
    gl.bindVertexArray(null);
    gl.bindTexture(gl.TEXTURE_2D, edges);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
    capacity = 0;
    edgeRows = 0;
    edgeKey = [];
    generation = p.generation;
  }

  function draw(scene: LiquidCellsScene, after?: (ctx: CanvasRenderingContext2D) => void) {
    last = scene;
    if (gl.isContextLost()) return;
    const w = canvas.width,
      h = canvas.height;
    output!.setTransform(1, 0, 0, 1, 0, 0);
    output!.clearRect(0, 0, w, h);
    const count = scene.pools.length;
    if (count) {
      if (generation !== p.generation || !vao) setup();
      // Edges: re-packed only when a region's geometry is new.
      const key = scene.pools.map((pool) =>
        pool.shape.kind === "rings" ? pool.shape.rings : null,
      );
      const same = key.length === edgeKey.length && key.every((k, i) => k === edgeKey[i]);
      if (!same) {
        edgeKey = key;
        const packedEdges = packEdges(scene.pools, edgeTexels);
        edgeTexels = packedEdges.texels;
        spans = packedEdges.spans;
        gl.bindTexture(gl.TEXTURE_2D, edges);
        gl.pixelStorei(gl.UNPACK_ALIGNMENT, 4);
        if (packedEdges.rows !== edgeRows) {
          gl.texImage2D(
            gl.TEXTURE_2D,
            0,
            gl.RGBA32F,
            EDGE_WIDTH,
            packedEdges.rows,
            0,
            gl.RGBA,
            gl.FLOAT,
            edgeTexels,
          );
          edgeRows = packedEdges.rows;
        } else
          gl.texSubImage2D(
            gl.TEXTURE_2D,
            0,
            0,
            0,
            EDGE_WIDTH,
            packedEdges.rows,
            gl.RGBA,
            gl.FLOAT,
            edgeTexels,
          );
      }
      if (packed.length < count * STRIDE)
        packed = new Float32Array(Math.ceil(count * 1.25) * STRIDE);
      for (let i = 0; i < count; i++) {
        const pool = scene.pools[i];
        const b = boundsOf(pool.shape);
        const o = i * STRIDE;
        const rect = pool.shape.kind === "rect";
        const g = rect ? pool.grow : 0;
        const shape = {
          left: b.left - g,
          top: b.top - g,
          right: b.right + g,
          bottom: b.bottom + g,
        };
        // Lifted pools reach out for their shadow; the rest only for anti-aliasing.
        const margin = pool.focus > 0.001 ? 14 : 1.5;
        packed[o] = shape.left - margin;
        packed[o + 1] = shape.top - margin;
        packed[o + 2] = shape.right + margin;
        packed[o + 3] = shape.bottom + margin;
        packed[o + 4] = shape.left;
        packed[o + 5] = shape.top;
        packed[o + 6] = shape.right;
        packed[o + 7] = shape.bottom;
        packed[o + 8] = pool.colour[0];
        packed[o + 9] = pool.colour[1];
        packed[o + 10] = pool.colour[2];
        packed[o + 11] = pool.presence;
        packed[o + 12] = pool.level;
        packed[o + 13] = pool.tilt;
        packed[o + 14] = pool.appear;
        packed[o + 15] = pool.focus;
        packed[o + 16] = rect ? 0 : 1;
        packed[o + 17] = rect ? (pool.shape as { radius: number }).radius : spans[i].start;
        packed[o + 18] = rect ? 0 : spans[i].count;
        packed[o + 19] = (pool.etched ? 1 : 0) + (pool.mark ? 2 : 0) + (pool.markLight ? 4 : 0);
        packed[o + 20] = pool.anchor[0];
        packed[o + 21] = pool.anchor[1];
        packed[o + 22] = rect ? 1 : pool.lift;
        packed[o + 23] = pool.tone;
      }
      gl.bindBuffer(gl.ARRAY_BUFFER, instances);
      if (capacity < count) {
        capacity = packed.length / STRIDE;
        gl.bufferData(gl.ARRAY_BUFFER, packed, gl.DYNAMIC_DRAW);
      } else gl.bufferSubData(gl.ARRAY_BUFFER, 0, packed, 0, count * STRIDE);
      drops.fill(0);
      scene.drops.slice(-MAX_DROPS).forEach((d, i) => drops.set([d.x, d.y, d.t, d.amp], i * 4));

      prepare(p, w, h);
      gl.useProgram(p.pools.program);
      const l = p.pools.location;
      gl.uniform2f(l.uSize, size.width, size.height);
      gl.uniform2f(l.uOffset, scene.offset.x, scene.offset.y);
      gl.uniform4fv(l["uDrops[0]"], drops);
      gl.uniform1f(l.uTime, scene.time);
      gl.activeTexture(gl.TEXTURE0);
      gl.bindTexture(gl.TEXTURE_2D, edges);
      gl.uniform1i(l.uEdges, 0);
      gl.uniform1f(l.uTheme, scene.theme === "light" ? 1 : 0);
      gl.uniform1f(l.uRatio, size.ratio);
      gl.uniform3f(l.uInk, ...INK[scene.theme]);
      gl.bindVertexArray(vao);
      gl.drawArraysInstanced(gl.TRIANGLE_STRIP, 0, 4, count);
      gl.bindVertexArray(null);
      gl.disable(gl.BLEND);
      copyOut(p, output!, w, h);
    }
    if (after) {
      output!.setTransform(size.ratio, 0, 0, size.ratio, 0, 0);
      after(output!);
      output!.setTransform(1, 0, 0, 1, 0, 0);
    }
  }

  const redraw = () => {
    if (last) draw(last);
  };
  clients.add(redraw);
  return {
    resize(width: number, height: number) {
      if (size.resize(width, height) && last) draw(last);
    },
    draw,
    get ratio() {
      return size.ratio;
    },
    destroy() {
      clients.delete(redraw);
      if (!gl.isContextLost()) {
        if (vao) gl.deleteVertexArray(vao);
        if (instances) gl.deleteBuffer(instances);
        if (edges) gl.deleteTexture(edges);
      }
      output!.clearRect(0, 0, canvas.width, canvas.height);
    },
  };
}

export type LiquidCellsRenderer = NonNullable<ReturnType<typeof createLiquidCells>>;

/* ——— The hook ——— */

type Spring = { value: number; velocity: number };
type CellState = {
  rgb: [number, number, number];
  velocity: [number, number, number];
  tone: number;
  focus: number;
  presence: number;
  level: Spring;
  launched: boolean;
  /** Slosh: its amplitude (px of tilt), phase and frequency. */
  slosh: number;
  phase: number;
  hz: number;
};

/** The level's spring (snappier than a mark's: cells are small) and how it bounces off the brim. */
const LEVEL = { hz: 1.9, zeta: 0.62, restitution: 0.24 };
const DROP_EVERY = 0.11;

/**
 * One canvas of liquid cells. Cells pour in on entrance, colours spring between data (critically
 * damped, so a colour never passes its target), cells out of focus dim towards graphite, and the
 * pointer's waves run across neighbouring pools. It draws only while something moves, and pauses
 * offscreen. Layouts are in plot px; the canvas reaches `bleed` px past the plot for the focus
 * shadow. `overlay` draws plain 2D strokes over the liquid (a range's outline), in plot px.
 * `available` is false without WebGL2: draw the flat cells instead.
 */
export function useLiquidCells({
  theme,
  layout,
  overlay,
  replay = 0,
  bleed = 16,
}: {
  theme: Theme;
  layout: (width: number, height: number) => LiquidCell[];
  overlay?: (ctx: CanvasRenderingContext2D, width: number, height: number, settled: number) => void;
  replay?: number;
  bleed?: number;
}) {
  const box = useRef<HTMLDivElement>(null);
  const canvas = useRef<HTMLCanvasElement>(null);
  const [available, setAvailable] = useState(true);
  const live = useRef({ theme, layout, overlay, replay, wake: () => {} });

  useEffect(() => {
    Object.assign(live.current, { theme, layout, overlay, replay });
    live.current.wake();
  });

  useEffect(() => {
    const surface = canvas.current,
      root = box.current;
    if (!surface || !root) return;
    const renderer = createLiquidCells(surface);
    if (!renderer) {
      setAvailable(false);
      return;
    }
    const shared = live.current;
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    let width = 0,
      height = 0;
    const offset = { x: bleed, y: bleed };
    const observer = new ResizeObserver(() => {
      const r = root.getBoundingClientRect();
      const ratio = pixelRatio(2);
      const fraction = (value: number) => ((((value * ratio) % 1) + 1) % 1) / ratio;
      width = r.width;
      height = r.height;
      offset.x = bleed + fraction(r.left);
      offset.y = bleed + fraction(r.top);
      Object.assign(surface.style, {
        position: "absolute",
        left: `${-offset.x}px`,
        top: `${-offset.y}px`,
      });
      renderer.resize(Math.ceil(width + offset.x + bleed), Math.ceil(height + offset.y + bleed));
      wake();
    });
    observer.observe(root);
    const pointer = trackPointer(root, () => wake(), 12);
    const presence = chartPresence(root, () => wake(), { redraw });
    const presses = focusPresses();
    const state = new Map<string, CellState>();
    const drops: { x: number; y: number; t: number; amp: number }[] = [];
    let replayed = shared.replay,
      start = presence.clock(performance.now()),
      raf = 0,
      last = 0,
      lastDrop = -1,
      seeds = 0,
      // What the canvas shows once at rest, so a wake that changes nothing (a slow pointer) draws
      // nothing.
      rested: { layout: unknown; theme: Theme; width: number; height: number } | null = null;
    const omega = 15;
    const w = 2 * Math.PI * LEVEL.hz;

    function stepLevel(s: CellState, target: number, h: number) {
      s.level.velocity +=
        (w * w * (target - s.level.value) - 2 * LEVEL.zeta * w * s.level.velocity) * h;
      s.level.value += s.level.velocity * h;
      // The brim: liquid can't rise past it, so it bounces back and sloshes.
      if (s.level.value > 1 && s.level.velocity > 0) {
        s.level.value = 1;
        s.slosh = Math.max(s.slosh, Math.min(2.2, Math.abs(s.level.velocity) * 0.5));
        s.level.velocity *= -LEVEL.restitution;
      }
    }

    function frame(frameTime: number) {
      raf = 0;
      if (!width || !height) return;
      if (!presence.active) {
        last = 0;
        return;
      }
      const now = presence.clock(frameTime);
      // The real time since the last frame. Past a quarter second the motion lands at its end, and
      // a settled frame (print, export) lands all of it.
      const { dt, snap } = frameDelta(now, last);
      const jump = snap || presence.settling;
      last = now;
      if (shared.replay !== replayed) {
        replayed = shared.replay;
        start = now;
        for (const s of state.values()) {
          s.level = { value: 0, velocity: 0 };
          s.launched = false;
          s.slosh = 0;
        }
      }
      const time = now / 1000;
      const elapsed = (now - start) / 1000;
      const t = shared.theme;
      let moving = false;
      // The entrance runs on the presence clock, which holds until about a third is on screen.
      const cells = shared.layout(width, height);
      const seen = new Set<string>();
      const ease = (k: number) => 1 - Math.exp(-k * dt);
      const joined = reduced
        ? []
        : presses(cells.map((cell) => ({ key: cell.key, lit: cell.focus ? 1 : cell.lit })));

      // Pointer waves: a sweep drops one every so often, scaled by its speed; a press drops one at
      // the cell it lands on.
      const ptr = pointer.current;
      if (!reduced && ptr) {
        const speed = Math.hypot(ptr.vx, ptr.vy);
        if (speed > 80 && time - lastDrop > DROP_EVERY) {
          drops.push({ x: ptr.x, y: ptr.y, t: time, amp: Math.min(2.4, speed / 380) });
          lastDrop = time;
        }
      }
      const frames: (LiquidPoolFrame & { order: number })[] = [];
      cells.forEach((cell, index) => {
        seen.add(cell.key);
        let s = state.get(cell.key);
        const target = cell.colour;
        if (!s) {
          const c = target ?? SURFACE[t];
          s = {
            rgb: [c[0], c[1], c[2]],
            velocity: [0, 0, 0],
            tone: cell.lit,
            focus: cell.focus ? 1 : 0,
            presence: target ? 1 : 0,
            level: { value: 0, velocity: 0 },
            launched: false,
            slosh: 0,
            phase: (++seeds * 2.399) % (Math.PI * 2),
            hz: 1.6 + ((seeds * 0.618) % 1) * 0.8,
          };
          state.set(cell.key, s);
        }
        const fill = target ? Math.max(0, Math.min(1, cell.level ?? 1)) : 0;
        const b = boundsOf(cell.shape);
        if (joined.includes(cell.key))
          drops.push({ x: (b.left + b.right) / 2, y: (b.top + b.bottom) / 2, t: time, amp: 2.4 });
        if (reduced || jump) {
          if (target) s.rgb = [target[0], target[1], target[2]];
          s.velocity = [0, 0, 0];
          s.tone = cell.lit;
          s.focus = cell.focus ? 1 : 0;
          s.presence = target ? 1 : 0;
          if (reduced || presence.settling || elapsed >= 0.15 + (cell.delay ?? 0)) {
            s.level = { value: fill, velocity: 0 };
            s.launched = true;
            s.slosh = 0;
          }
        } else {
          if (target)
            for (let i = 0; i < 3; i++) {
              // In steps of at most 50 ms, so a long frame stays stable.
              for (let k = 0, n = substeps(dt); k < n; k++) {
                s.velocity[i] +=
                  (omega * omega * (target[i] - s.rgb[i]) - 2 * omega * s.velocity[i]) * (dt / n);
                s.rgb[i] += s.velocity[i] * (dt / n);
              }
              if (Math.abs(target[i] - s.rgb[i]) > 0.002 || Math.abs(s.velocity[i]) > 0.01)
                moving = true;
              else {
                s.rgb[i] = target[i];
                s.velocity[i] = 0;
              }
            }
          const present = target ? 1 : 0;
          s.presence += (present - s.presence) * ease(9);
          if (Math.abs(present - s.presence) > 0.004) moving = true;
          else s.presence = present;
          s.tone += (cell.lit - s.tone) * ease(12);
          if (Math.abs(cell.lit - s.tone) > 0.004) moving = true;
          else s.tone = cell.lit;
          const focus = cell.focus ? 1 : 0;
          s.focus += (focus - s.focus) * ease(20);
          if (Math.abs(focus - s.focus) > 0.004) moving = true;
          else s.focus = focus;
          const launched = elapsed >= 0.15 + (cell.delay ?? 0);
          if (!launched) moving = true;
          else {
            s.launched = true;
            const n = substeps(dt);
            for (let k = 0; k < n; k++) stepLevel(s, fill, dt / n);
            if (Math.abs(fill - s.level.value) < 0.002 && Math.abs(s.level.velocity) < 0.01) {
              s.level.value = fill;
              s.level.velocity = 0;
            } else moving = true;
            // The slosh decays; under a third of a px it is gone.
            s.slosh *= Math.exp(-3.4 * dt);
            if (s.slosh > 0.3) moving = true;
            else s.slosh = 0;
          }
        }
        const colour = dimmed(s.rgb, t, (1 - s.tone) * 0.78);
        const appear =
          reduced || presence.settling
            ? 1
            : Math.max(0, Math.min(1, (elapsed - 0.05 - (cell.delay ?? 0)) / 0.25));
        if (appear < 1) moving = true;
        const span = Math.max(b.right - b.left, b.bottom - b.top, 1);
        frames.push({
          order: index,
          shape: cell.shape,
          colour: s.presence > 0.001 ? colour : SURFACE[t],
          presence: s.presence,
          level: s.launched ? s.level.value : 0,
          tilt: s.slosh * Math.sin(time * 2 * Math.PI * s.hz + s.phase),
          appear,
          focus: s.focus,
          tone: s.tone,
          etched: !!cell.etched,
          mark: !!cell.mark,
          markLight: luminance(s.presence > 0.5 ? colour : SURFACE[t]) <= 0.36,
          grow: (cell.grow ?? 0) * s.focus,
          lift: 1 + Math.min(0.08, 5 / span) * s.focus,
          anchor: cell.anchor ?? [(b.left + b.right) / 2, (b.top + b.bottom) / 2],
        });
      });
      for (const key of state.keys()) if (!seen.has(key)) state.delete(key);
      // Lifted pools draw last, over their neighbours.
      frames.sort((a, b) => a.focus - b.focus || a.order - b.order);
      while (drops.length && time - drops[0].t > 2.2) drops.shift();
      if (drops.length) moving = true;
      if (pointer.settle(dt)) moving = true;
      if (
        !moving &&
        !presence.settling &&
        rested &&
        rested.layout === shared.layout &&
        rested.theme === t &&
        rested.width === width &&
        rested.height === height
      ) {
        last = 0;
        return;
      }
      rested = moving ? null : { layout: shared.layout, theme: t, width, height };
      let settled = 1;
      for (const f of frames) if (f.appear < settled) settled = f.appear;
      renderer!.draw(
        { theme: t, offset, pools: frames, drops, time },
        shared.overlay
          ? (ctx) => {
              ctx.translate(offset.x, offset.y);
              shared.overlay!(ctx, width, height, settled);
            }
          : undefined,
      );
      if (moving && presence.running) raf = requestAnimationFrame(frame);
      else last = 0;
    }
    function wake() {
      if (!raf) raf = requestAnimationFrame(frame);
    }
    /** Draws a frame now (print, export). */
    function redraw() {
      cancelAnimationFrame(raf);
      raf = 0;
      frame(performance.now());
    }
    shared.wake = wake;
    wake();
    return () => {
      cancelAnimationFrame(raf);
      observer.disconnect();
      pointer.stop();
      presence.stop();
      renderer.destroy();
      shared.wake = () => {};
    };
    // The bleed is fixed for the canvas's life.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return { box, canvas, available };
}

/* ——— Density ——— */

export type LiquidDensityScene = {
  theme: Theme;
  /** The field's rectangle on the canvas, CSS px. */
  rect: Rect;
  /** Per cell, row-major from the top left: series A's and B's depth, 0–1. */
  field: Float32Array;
  columns: number;
  rows: number;
  colours: readonly Rgb[];
  /** The shoreline's depth (0–1): lower spreads the liquid wider. */
  shore: number;
};

/** A liquid density renderer for `canvas`, on the same shared context as the cells. */
export function createLiquidDensity(canvas: HTMLCanvasElement) {
  const painter = sharedPainter(canvas.ownerDocument);
  const output = painter ? canvas.getContext("2d") : null;
  if (!painter || !output) return null;
  const p = painter;
  const gl = p.gl;
  const size = makeOutput(canvas, gl);
  let texture: WebGLTexture | null = null,
    vao: WebGLVertexArrayObject | null = null,
    generation = -1,
    dims = "",
    last: LiquidDensityScene | null = null;

  function draw(scene: LiquidDensityScene) {
    last = scene;
    if (gl.isContextLost()) return;
    const w = canvas.width,
      h = canvas.height;
    output!.setTransform(1, 0, 0, 1, 0, 0);
    output!.clearRect(0, 0, w, h);
    if (scene.columns < 2 || scene.rows < 2) return;
    if (generation !== p.generation || !texture) {
      texture = gl.createTexture();
      gl.bindTexture(gl.TEXTURE_2D, texture);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
      vao = gl.createVertexArray();
      gl.bindVertexArray(vao);
      gl.bindBuffer(gl.ARRAY_BUFFER, p.quad);
      gl.enableVertexAttribArray(0);
      gl.vertexAttribPointer(0, 2, gl.FLOAT, false, 0, 0);
      gl.bindVertexArray(null);
      generation = p.generation;
      dims = "";
    }
    gl.activeTexture(gl.TEXTURE0);
    gl.bindTexture(gl.TEXTURE_2D, texture);
    gl.pixelStorei(gl.UNPACK_ALIGNMENT, 4);
    const d = `${scene.columns}x${scene.rows}`;
    if (d !== dims) {
      gl.texImage2D(
        gl.TEXTURE_2D,
        0,
        gl.RG16F,
        scene.columns,
        scene.rows,
        0,
        gl.RG,
        gl.FLOAT,
        scene.field,
      );
      dims = d;
    } else
      gl.texSubImage2D(
        gl.TEXTURE_2D,
        0,
        0,
        0,
        scene.columns,
        scene.rows,
        gl.RG,
        gl.FLOAT,
        scene.field,
      );
    prepare(p, w, h);
    gl.useProgram(p.density.program);
    const l = p.density.location;
    gl.uniform4f(l.uPlot, scene.rect.left, scene.rect.top, scene.rect.right, scene.rect.bottom);
    gl.uniform2f(l.uSize, size.width, size.height);
    gl.uniform1i(l.uField, 0);
    gl.uniform3f(l.uColourA, ...(scene.colours[0] ?? ([0.6, 0.6, 0.6] as const)));
    gl.uniform3f(
      l.uColourB,
      ...(scene.colours[1] ?? scene.colours[0] ?? ([0.6, 0.6, 0.6] as const)),
    );
    gl.uniform1f(l.uSeries, Math.min(2, scene.colours.length));
    gl.uniform1f(l.uTheme, scene.theme === "light" ? 1 : 0);
    gl.uniform1f(l.uShore, scene.shore);
    gl.uniform1f(l.uRatio, size.ratio);
    gl.bindVertexArray(vao);
    gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4);
    gl.bindVertexArray(null);
    gl.disable(gl.BLEND);
    copyOut(p, output!, w, h);
  }
  const redraw = () => {
    if (last) draw(last);
  };
  clients.add(redraw);
  return {
    resize(width: number, height: number) {
      if (size.resize(width, height) && last) draw(last);
    },
    draw,
    destroy() {
      clients.delete(redraw);
      if (!gl.isContextLost()) {
        if (texture) gl.deleteTexture(texture);
        if (vao) gl.deleteVertexArray(vao);
      }
      output!.clearRect(0, 0, canvas.width, canvas.height);
    },
  };
}

/**
 * A density field from points: binned on a grid of `cell` px and blurred by `sigma` px, per
 * series (two at most), as depth 0–1 against the deepest cell of either. Pure, for tests.
 */
export function densityField(
  points: readonly { x: number; y: number; series: number }[],
  rect: Rect,
  { cell = 4, sigma = 10 }: { cell?: number; sigma?: number } = {},
  into?: { field: Float32Array; scratch: Float32Array },
) {
  const columns = Math.max(2, Math.ceil((rect.right - rect.left) / cell) + 1);
  const rows = Math.max(2, Math.ceil((rect.bottom - rect.top) / cell) + 1);
  const n = columns * rows * 2;
  const field = into && into.field.length === n ? into.field : new Float32Array(n);
  const scratch = into && into.scratch.length === n ? into.scratch : new Float32Array(n);
  field.fill(0);
  for (const pt of points) {
    const fx = (pt.x - rect.left) / cell,
      fy = (pt.y - rect.top) / cell;
    const x0 = Math.floor(fx),
      y0 = Math.floor(fy);
    const tx = fx - x0,
      ty = fy - y0;
    const ch = pt.series > 0 ? 1 : 0;
    // Bilinear splat, so a point's mass doesn't snap to the grid.
    for (const [dx, dy, wgt] of [
      [0, 0, (1 - tx) * (1 - ty)],
      [1, 0, tx * (1 - ty)],
      [0, 1, (1 - tx) * ty],
      [1, 1, tx * ty],
    ] as const) {
      const x = x0 + dx,
        y = y0 + dy;
      if (x < 0 || y < 0 || x >= columns || y >= rows) continue;
      field[(y * columns + x) * 2 + ch] += wgt;
    }
  }
  const s = sigma / cell;
  const radius = Math.max(1, Math.ceil(s * 3));
  const kernel = new Float32Array(radius * 2 + 1);
  let sum = 0;
  for (let i = -radius; i <= radius; i++)
    sum += kernel[i + radius] = Math.exp(-(i * i) / (2 * s * s));
  for (let i = 0; i < kernel.length; i++) kernel[i] /= sum;
  // Separable blur: rows into scratch, then columns back into the field.
  for (let y = 0; y < rows; y++)
    for (let x = 0; x < columns; x++)
      for (let ch = 0; ch < 2; ch++) {
        let acc = 0;
        for (let k = -radius; k <= radius; k++) {
          const xx = x + k;
          if (xx < 0 || xx >= columns) continue;
          acc += field[(y * columns + xx) * 2 + ch] * kernel[k + radius];
        }
        scratch[(y * columns + x) * 2 + ch] = acc;
      }
  let peak = 0;
  for (let y = 0; y < rows; y++)
    for (let x = 0; x < columns; x++)
      for (let ch = 0; ch < 2; ch++) {
        let acc = 0;
        for (let k = -radius; k <= radius; k++) {
          const yy = y + k;
          if (yy < 0 || yy >= rows) continue;
          acc += scratch[(yy * columns + x) * 2 + ch] * kernel[k + radius];
        }
        field[(y * columns + x) * 2 + ch] = acc;
        if (acc > peak) peak = acc;
      }
  if (peak > 0) for (let i = 0; i < n; i++) field[i] /= peak;
  return { field, scratch, columns, rows, cell };
}
