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
import {
  OPTICAL_SURFACE,
  opticalFragmentSource,
  opticalVertexSource,
} from "./chart-optical-shader";
import type {
  ChartOpticalScene,
  ChartOpticalResult,
  ChartOpticalMark,
} from "./chart-optical-types";
import { opticalFieldGrid, rasterizeOpticalField } from "./chart-optical-field";
import { parseMaterialColor } from "./chart-material-policy";
import type { OpticalField } from "./chart-optical-field";
export type * from "./chart-optical-types";

type Color = readonly [number, number, number, number];
/** A mask on the GPU, with the translation and uniform scale it was rasterized at, so a mark that
 * has moved or grown since can still be drawn from it. */
type TextureEntry = {
  texture: WebGLTexture;
  field: OpticalField;
  bytes: number;
  tx: number;
  ty: number;
  scale: number;
  /** Approximate keys that point here (see `Painter.near`). */
  near: string[];
};
type Client = {
  id: number;
  canvas: HTMLCanvasElement;
  last: ChartOpticalScene | null;
  disposed: boolean;
  paint: (scene: ChartOpticalScene) => ChartOpticalResult;
  report: (result: ChartOpticalResult) => void;
};
type Painter = {
  doc: Document;
  canvas: HTMLCanvasElement;
  mask: HTMLCanvasElement;
  gl: WebGL2RenderingContext;
  program: WebGLProgram | null;
  buffer: WebGLBuffer | null;
  palette: WebGLTexture | null;
  uniforms: Record<string, WebGLUniformLocation | null>;
  /** Masks by exact geometry, least recently used first. */
  entries: Map<string, TextureEntry>;
  /** A moved or growing mark's approximate key (its geometry without its place, or with its size
   * in 2px steps) to the exact key of the latest mask that answers it. */
  near: Map<string, string>;
  colors: Map<string, Color | null>;
  clients: Set<Client>;
  bytes: number;
  lost: boolean;
  reason: string;
  idle: ReturnType<typeof setTimeout> | undefined;
};
const painters = new WeakMap<Document, Painter>();
const pathIds = new WeakMap<object, number>();
let nextClient = 1,
  nextPath = 1;
/** Mask budget at one device pixel per CSS px; it grows with the pixel count (4× at DPR 2). */
const CACHE_BYTES = 32 * 1024 * 1024;
const BACKING_PIXELS = 16 * 1024 * 1024;
/** A painter with no charts left is kept this long, so a page switch reuses its context. */
const IDLE_MS = 30_000;
/** After motion ends, the exact masks replace the approximate ones this much later. */
const REFINE_MS = 80;
const DEV = process.env.NODE_ENV !== "production";
const TAU = Math.PI * 2;
const IDENTITY = [1, 0, 0, 1, 0, 0] as const;
/** Shared backing only grows; oversized unions retain native paint rather than churn GPU allocations. */
export function opticalBackingPlan(
  currentWidth: number,
  currentHeight: number,
  width: number,
  height: number,
  maxSide = 8192,
): { width: number; height: number; sourceY: number } | null {
  if (
    ![currentWidth, currentHeight, width, height, maxSide].every(Number.isFinite) ||
    width <= 0 ||
    height <= 0 ||
    maxSide <= 0
  )
    return null;
  const nextWidth = Math.max(1, Math.ceil(currentWidth), Math.ceil(width));
  const nextHeight = Math.max(1, Math.ceil(currentHeight), Math.ceil(height));
  if (nextWidth > maxSide || nextHeight > maxSide || nextWidth * nextHeight > BACKING_PIXELS)
    return null;
  return { width: nextWidth, height: nextHeight, sourceY: nextHeight - Math.ceil(height) };
}

const NAMES = [
  "uField",
  "uPalette",
  "uSize",
  "uRatio",
  "uFieldRect",
  "uFieldGain",
  "uBounds",
  "uRange",
  "uColor",
  "uHasPalette",
  "uColorDirection",
  "uTone",
  "uGlow",
  "uTheme",
  "uDepth",
  "uBevel",
  "uShell",
  "uFill",
  "uOrigin",
  "uAngular",
  "uAngularCenter",
  "uKind",
  "uDepthAxis",
  "uDepthReverse",
  "uShapeKind",
  "uShape",
  "uAngles",
  "uOpacity",
  "uFocus",
  "uHatch",
  "uPattern",
  "uMarks",
  "uSeed",
  "uTip",
  "uLiquidSurface[0]",
  "uExact",
  "uGrid",
  "uFieldFormat",
  "uExactShape[0]",
];
const FLAT_SURFACE = new Float32Array(OPTICAL_SURFACE.columns * OPTICAL_SURFACE.rows);
/** A cheap fingerprint of a liquid surface, so unchanged frames are still skipped. */
function fieldPrint(field: Float32Array) {
  let sum = 0;
  for (let k = 0; k < field.length; k++) sum += field[k] * ((k % 7) + 1);
  return Math.round(sum * 1000);
}
const bounded = (n: number | undefined, fallback = 1) =>
  Math.max(0, Math.min(1, Number.isFinite(n) ? n! : fallback));
const pathKey = (path: string | Path2D) => {
  if (typeof path === "string") return path;
  let id = pathIds.get(path);
  if (!id) {
    id = nextPath++;
    pathIds.set(path, id);
  }
  return `path:${id}`;
};
/** Thousandths of a px past the whole px: where a mask's texels fall under its edges. */
const phase = (value: number) => Math.round((((value % 1) + 1) % 1) * 1000) % 1000;
const hundredths = (value: number) => Math.round(value * 100) / 100;

/** As core `sectorPath` clamps a sector's corner rounds so they fit inside it (0: too small to
 * round, drawn sharp). The two must agree for an exact sector to match its path. */
export function opticalSectorCorner(
  inner: number,
  outer: number,
  start: number,
  end: number,
  corner = 0,
) {
  inner = Math.max(0, Math.min(inner, outer));
  const s = Math.sin(Math.min((end - start) / 2, Math.PI / 2));
  let k = Math.max(0, Math.min(corner, (outer - inner) / 2));
  if (s < 1) k = Math.min(k, (outer * s) / (1 + s) - 0.01);
  if (inner > 0 && s < 1) k = Math.min(k, (inner * s) / (1 - s));
  k = Math.max(0, k);
  return k < 0.05 ? 0 : k;
}

type Exact = { kind: 1 | 2 | 3; shape: Float32Array };
/** The shader's description of an exact shape (see `uExactShape`), or null to build a mask. */
export function opticalExactShape(mark: ChartOpticalMark): Exact | null {
  const shape = mark.shape;
  if (!shape?.exact || mark.kind !== "glass" || mark.clip || mark.clips?.length) return null;
  const out = new Float32Array(24);
  if (shape.kind === "circle") {
    if (!(shape.r > 0) || ![shape.cx, shape.cy].every(Number.isFinite)) return null;
    out.set([shape.cx, shape.cy, shape.r]);
    return { kind: 3, shape: out };
  }
  if (shape.kind === "rect") {
    if (!(shape.width > 0 && shape.height > 0) || ![shape.x, shape.y].every(Number.isFinite))
      return null;
    const k = Math.max(0, Math.min(shape.radius ?? 0, shape.width / 2, shape.height / 2));
    out.set([
      shape.x + shape.width / 2,
      shape.y + shape.height / 2,
      shape.width / 2,
      shape.height / 2,
      k,
    ]);
    return { kind: 1, shape: out };
  }
  const outer = shape.outerRadius,
    inner = Math.max(0, Math.min(shape.innerRadius, outer));
  // The interval the sector covers, whichever way it sweeps (the tip's light reads the direction
  // from the shape's own angles).
  const a0 = Math.min(shape.startAngle, shape.endAngle),
    a1 = Math.max(shape.startAngle, shape.endAngle),
    span = a1 - a0;
  if (!(outer > 0) || !(span > 1e-6) || span >= TAU - 1e-6) return null;
  if (![shape.cx, shape.cy, inner, a0].every(Number.isFinite)) return null;
  const k = opticalSectorCorner(inner, outer, a0, a1, shape.corner);
  // A pie's rounds stop at its outer corners; its point at the centre stays sharp.
  if (inner <= 0 && k > 0) return null;
  const ri = inner + k,
    ro = outer - k;
  const tOut = Math.asin(Math.min(1, k / ro)),
    tIn = ri > 0 ? Math.asin(Math.min(1, k / ri)) : 0;
  const at = (r: number, a: number) => [Math.sin(a) * r, -Math.cos(a) * r];
  out.set([
    shape.cx,
    shape.cy,
    ri,
    ro,
    ...at(ro, a0 + tOut),
    ...at(ro, a1 - tOut),
    ...at(ri, a0 + tIn),
    ...at(ri, a1 - tIn),
    a0 + tOut,
    span - 2 * tOut,
    a0 + tIn,
    span - 2 * tIn,
    Math.cos(a0),
    Math.sin(a0),
    -Math.cos(a1),
    -Math.sin(a1),
    k,
    span > Math.PI ? 1 : 0,
  ]);
  return { kind: 2, shape: out };
}

type Placement = { tx: number; ty: number; linear: readonly number[]; scale: number };
/** A mark's transform as its translation and the rest; a uniform scale also reads as one number. */
function placement(mark: ChartOpticalMark): Placement {
  const [a, b, c, d, tx, ty] = mark.transform ?? IDENTITY;
  return { tx, ty, linear: [a, b, c, d], scale: b === 0 && c === 0 && a === d && a > 0 ? a : 0 };
}
/** Whether only the transform places the mask's content: no clip paths or contours of their own
 * in scene coordinates, which would not move with it. */
const movable = (mark: ChartOpticalMark) =>
  !mark.clips?.length && !mark.upper?.length && !mark.lower?.length;
/** How the shader reads a mask: distance alone (RG) unless it needs depth down the run, which an
 * area and a glass lit by its outline (no rect or sector to light it, filled from an edge) do. */
const maskChannels = (mark: ChartOpticalMark): 2 | 4 =>
  mark.kind === "glass" &&
  (mark.shape?.kind === "rect" || mark.shape?.kind === "sector" || mark.fillRadial)
    ? 2
    : 4;

/** The exact key of a mark's mask: its geometry relative to its transform's translation (so a
 * whole-px move keeps it) and the translation's sub-px phase. */
export function opticalGeometryKey(mark: ChartOpticalMark, ratio: number) {
  return maskKeys(mark, ratio).exact;
}
function maskKeys(mark: ChartOpticalMark, ratio: number) {
  const { tx, ty, linear, scale } = placement(mark);
  const b = mark.bounds;
  const own = movable(mark);
  // Content fixed in the scene ties the mask to its absolute place.
  const ox = own ? tx : 0,
    oy = own ? ty : 0;
  const shared = [
    mark.id,
    mark.cacheKey ?? [
      pathKey(mark.path),
      mark.clips?.map((clip) => [pathKey(clip.path), clip.transform]),
    ],
    mark.clip && [mark.clip.x - ox, mark.clip.y - oy, mark.clip.width, mark.clip.height],
    mark.fillRule,
    mark.upper,
    mark.lower,
    mark.kind === "area" ? [mark.fillOrigin, mark.depthAxis, mark.depthReverse] : undefined,
    maskChannels(mark),
    ratio,
  ];
  const place = [b.x - ox, b.y - oy, b.width, b.height];
  const exact = JSON.stringify([
    ...shared,
    place,
    linear,
    own ? [phase(tx), phase(ty)] : mark.transform,
  ]);
  if (!own) return { exact, move: null, grow: null };
  return {
    exact,
    // The same outline anywhere: drawn from its mask, offset (sub-px off its texels until refined).
    move: JSON.stringify([...shared, place.map(hundredths), linear, "move"]),
    // A uniformly scaled outline, one mask per 2px of size, drawn scaled until refined.
    grow: scale
      ? JSON.stringify([
          ...shared,
          [(b.x - tx) / scale, (b.y - ty) / scale, b.width / scale, b.height / scale].map(
            hundredths,
          ),
          "grow",
          Math.round(Math.max(b.width, b.height) / 2),
        ])
      : null,
  };
}
function resultFor(
  scene: ChartOpticalScene,
  reason: string,
  available = false,
): ChartOpticalResult {
  return {
    available,
    visible: false,
    rendered: [],
    fallback: scene.marks.map((mark) => ({ id: mark.id, reason })),
  };
}
/** While the shared context is lost, a chart keeps its last frame on screen: nothing is cleared or
 * hidden, and the restore repaints it (announced by a `chart-optical-status` event). */
const lostFrame = (result: ChartOpticalResult) =>
  !result.available && result.fallback.every((entry) => entry.reason === "context-lost");
/** Marks the canvas with the frame's outcome. Its visibility is only ever "hidden" (nothing drawn: the
 * chart falls back) or cleared, which leaves it to the layer it sits in: the core keeps a glass layer
 * hidden while flat marks stand in, and an inline "visible" would show the canvas through it. */
function emit(canvas: HTMLCanvasElement, result: ChartOpticalResult) {
  if (!lostFrame(result)) canvas.style.visibility = result.visible ? "" : "hidden";
  canvas.dataset.opticalRenderer = result.available ? "webgl-shared" : "native-fallback";
  canvas.dataset.opticalMarks = String(result.rendered.length);
  canvas.dataset.opticalFallback = result.fallback
    .map((entry) => `${entry.id}:${entry.reason}`)
    .join(";");
  try {
    canvas.dispatchEvent(new CustomEvent("chart-optical-status", { detail: result }));
  } catch {
    /* Minimal host/test canvases need not implement CustomEvent. */
  }
}
function texture(
  painter: Painter,
  width: number,
  height: number,
  data: Uint8Array,
  channels: 2 | 4 = 4,
) {
  const { gl } = painter,
    next = gl.createTexture();
  if (!next) throw new Error("texture-allocation");
  gl.bindTexture(gl.TEXTURE_2D, next);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
  gl.pixelStorei(gl.UNPACK_ALIGNMENT, 1);
  if (channels === 2)
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.RG8, width, height, 0, gl.RG, gl.UNSIGNED_BYTE, data);
  else gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, width, height, 0, gl.RGBA, gl.UNSIGNED_BYTE, data);
  return next;
}
function release(painter: Painter, key: string, entry: TextureEntry) {
  painter.gl.deleteTexture(entry.texture);
  painter.bytes -= entry.bytes;
  painter.entries.delete(key);
  for (const near of entry.near) if (painter.near.get(near) === key) painter.near.delete(near);
}
// Nothing a painter holds is made inside a chart's scope: its context handlers are module
// functions that find it through their canvas, so a page's first chart is never retained by the
// shared painter after it unmounts (a minifier may inline the factory into a chart's closure).
const byCanvas = new WeakMap<EventTarget, Painter>();
function initializePainter(painter: Painter) {
  const gl = painter.gl;
  if (painter.program) gl.deleteProgram(painter.program);
  if (painter.buffer) gl.deleteBuffer(painter.buffer);
  if (painter.palette) gl.deleteTexture(painter.palette);
  painter.program = null;
  painter.buffer = null;
  painter.palette = null;
  const program = gl.createProgram();
  if (!program) {
    painter.reason = "program-allocation";
    return false;
  }
  for (const [type, source] of [
    [gl.VERTEX_SHADER, opticalVertexSource],
    [gl.FRAGMENT_SHADER, opticalFragmentSource],
  ] as const) {
    const shader = gl.createShader(type);
    if (!shader) {
      gl.deleteProgram(program);
      painter.reason = "shader-allocation";
      return false;
    }
    gl.shaderSource(shader, source);
    gl.compileShader(shader);
    if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
      painter.reason = `shader-compile: ${gl.getShaderInfoLog(shader) ?? "unknown"}`;
      gl.deleteShader(shader);
      gl.deleteProgram(program);
      return false;
    }
    gl.attachShader(program, shader);
    gl.deleteShader(shader);
  }
  gl.linkProgram(program);
  if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
    painter.reason = `shader-link: ${gl.getProgramInfoLog(program) ?? "unknown"}`;
    gl.deleteProgram(program);
    return false;
  }
  const buffer = gl.createBuffer();
  if (!buffer) {
    gl.deleteProgram(program);
    painter.reason = "buffer-allocation";
    return false;
  }
  gl.useProgram(program);
  gl.bindBuffer(gl.ARRAY_BUFFER, buffer);
  gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 3, -1, -1, 3]), gl.STATIC_DRAW);
  const attribute = gl.getAttribLocation(program, "position");
  gl.enableVertexAttribArray(attribute);
  gl.vertexAttribPointer(attribute, 2, gl.FLOAT, false, 0, 0);
  painter.program = program;
  painter.buffer = buffer;
  painter.uniforms = Object.fromEntries(
    NAMES.map((name) => [name, gl.getUniformLocation(program, name)]),
  );
  painter.palette = texture(painter, 1, 1, new Uint8Array([255, 255, 255, 255]));
  painter.reason = "";
  return true;
}
function onContextLost(event: Event) {
  const painter = event.currentTarget && byCanvas.get(event.currentTarget);
  if (!painter) return;
  event.preventDefault();
  painter.lost = true;
  painter.reason = "context-lost";
  // WebGL destroys every object on loss. These handles belong to the old
  // context generation: even deleting them after restore is an invalid operation.
  // Drop references now; initialize creates only new-generation resources.
  painter.program = null;
  painter.buffer = null;
  painter.palette = null;
  painter.uniforms = {};
  painter.entries.clear();
  painter.near.clear();
  painter.bytes = 0;
  for (const client of painter.clients)
    if (client.last) client.report(resultFor(client.last, "context-lost"));
}
function onContextRestored(event: Event) {
  const painter = event.currentTarget && byCanvas.get(event.currentTarget);
  if (!painter) return;
  painter.lost = false;
  if (!initializePainter(painter)) {
    for (const client of painter.clients)
      if (client.last) client.report(resultFor(client.last, painter.reason));
    return;
  }
  for (const client of painter.clients)
    if (client.last && !client.disposed) client.report(client.paint(client.last));
}
function destroyPainter(painter: Painter) {
  const { gl, canvas } = painter;
  if (painter.idle !== undefined) clearTimeout(painter.idle);
  painter.idle = undefined;
  canvas.removeEventListener("webglcontextlost", onContextLost);
  canvas.removeEventListener("webglcontextrestored", onContextRestored);
  byCanvas.delete(canvas);
  for (const entry of painter.entries.values()) gl.deleteTexture(entry.texture);
  painter.entries.clear();
  painter.near.clear();
  painter.clients.clear();
  if (painter.palette) gl.deleteTexture(painter.palette);
  if (painter.buffer) gl.deleteBuffer(painter.buffer);
  if (painter.program) gl.deleteProgram(painter.program);
  painter.palette = painter.buffer = painter.program = null;
  if (painters.get(painter.doc) === painter) painters.delete(painter.doc);
  try {
    gl.getExtension("WEBGL_lose_context")?.loseContext();
  } catch {
    /* Disposed context may already be lost. */
  }
}
function expireIdle(painter: Painter) {
  painter.idle = undefined;
  if (!painter.clients.size) destroyPainter(painter);
}
function createPainter(doc: Document): Painter | null {
  const existing = painters.get(doc);
  if (existing) return existing;
  const canvas = doc.createElement("canvas"),
    mask = doc.createElement("canvas");
  let gl: WebGL2RenderingContext | null;
  try {
    gl = canvas.getContext("webgl2", {
      alpha: true,
      premultipliedAlpha: true,
      antialias: false,
      preserveDrawingBuffer: true,
      powerPreference: "low-power",
    });
  } catch {
    return null;
  }
  if (!gl) return null;
  const painter: Painter = {
    doc,
    canvas,
    mask,
    gl,
    program: null,
    buffer: null,
    palette: null,
    uniforms: {},
    entries: new Map(),
    near: new Map(),
    colors: new Map(),
    clients: new Set(),
    bytes: 0,
    lost: false,
    reason: "",
    idle: undefined,
  };
  byCanvas.set(canvas, painter);
  canvas.addEventListener("webglcontextlost", onContextLost);
  canvas.addEventListener("webglcontextrestored", onContextRestored);
  try {
    initializePainter(painter);
  } catch (error) {
    painter.reason = error instanceof Error ? error.message : "initialization";
  }
  painters.set(doc, painter);
  return painter;
}
/** Let a painter with no charts go now rather than after its idle wait (tests, teardown). */
export function releaseIdleOpticalPainter(doc: Document) {
  const painter = painters.get(doc);
  if (painter && !painter.clients.size) destroyPainter(painter);
}
function color(painter: Painter, value: string): Color | null {
  const parsed = parseMaterialColor(value);
  if (parsed) return parsed;
  if (painter.colors.has(value)) return painter.colors.get(value)!;
  let resolved: Color | null = null;
  try {
    const context = painter.mask.getContext("2d", { willReadFrequently: true });
    if (context) {
      context.save();
      context.setTransform(1, 0, 0, 1, 0, 0);
      context.clearRect(0, 0, 1, 1);
      context.fillStyle = "#010203";
      context.fillStyle = value;
      if (context.fillStyle !== "#010203") {
        context.fillRect(0, 0, 1, 1);
        const pixel = context.getImageData(0, 0, 1, 1).data;
        resolved = [pixel[0], pixel[1], pixel[2], pixel[3] / 255];
      }
      context.restore();
    }
  } catch {
    /* Unknown color retains native provider fill. */
  }
  painter.colors.set(value, resolved);
  return resolved;
}
/** Where a mask draws now: its field rect, distance gain and texels per px. */
type Mapped = { entry: TextureEntry; x: number; y: number; w: number; h: number; gain: number };
function fieldFor(
  painter: Painter,
  client: Client,
  mark: ChartOpticalMark,
  ratio: number,
  approximate: boolean,
): Mapped & { approximated: boolean } {
  const keys = maskKeys(mark, ratio);
  const prefix = `${client.id}:`;
  const key = prefix + keys.exact;
  const at = placement(mark);
  const map = (entry: TextureEntry, approximated: boolean) => {
    const gain = at.scale && entry.scale ? at.scale / entry.scale : 1;
    const f = entry.field;
    return {
      entry,
      x: at.tx + (f.x - entry.tx) * gain,
      y: at.ty + (f.y - entry.ty) * gain,
      w: f.cssWidth * gain,
      h: f.cssHeight * gain,
      gain,
      approximated,
    };
  };
  const touch = (exactKey: string, entry: TextureEntry) => {
    painter.entries.delete(exactKey);
    painter.entries.set(exactKey, entry);
  };
  const hit = painter.entries.get(key);
  if (hit) {
    touch(key, hit);
    return map(hit, false);
  }
  // A moving or growing mark keeps drawing from a mask it already has until it holds still.
  if (approximate)
    for (const near of [keys.move, keys.grow]) {
      if (!near) continue;
      const exactKey = painter.near.get(prefix + near);
      const entry = exactKey && painter.entries.get(exactKey);
      if (entry) {
        touch(exactKey, entry);
        return map(entry, true);
      }
    }
  const channels = maskChannels(mark);
  const field = rasterizeOpticalField(mark, painter.mask, ratio, channels);
  if (!field.ink) throw new Error("empty-mask");
  const budget = CACHE_BYTES * ratio * ratio;
  if (field.pixels.byteLength > budget) throw new Error("mask-budget");
  for (const [stale, entry] of painter.entries) {
    if (painter.bytes + field.pixels.byteLength <= budget) break;
    release(painter, stale, entry);
  }
  const entry: TextureEntry = {
    texture: texture(painter, field.width, field.height, field.pixels, channels),
    field,
    bytes: field.pixels.byteLength,
    tx: at.tx,
    ty: at.ty,
    scale: at.scale,
    near: [],
  };
  // GPU owns pixels after upload. Cache geometry metadata only; restore rebuilds native masks.
  field.pixels = new Uint8Array(0);
  painter.entries.set(key, entry);
  painter.bytes += entry.bytes;
  for (const near of [keys.move, keys.grow])
    if (near) {
      painter.near.set(prefix + near, key);
      entry.near.push(prefix + near);
    }
  return map(entry, false);
}
function palette(painter: Painter, mark: ChartOpticalMark) {
  const stops = mark.colors?.length
    ? [...mark.colors]
    : mark.palette?.map((value, index, all) => ({
        offset: index / Math.max(1, all.length - 1),
        color: value,
      }));
  if (!stops?.length) return false;
  stops.sort((a, b) => a.offset - b.offset);
  const resolved = stops.map((stop) => ({
    offset: bounded(stop.offset),
    color: color(painter, stop.color),
  }));
  if (resolved.some((stop) => !stop.color)) throw new Error("palette-color");
  const data = new Uint8Array(256 * 4);
  for (let i = 0; i < 256; i++) {
    const t = i / 255;
    let upper = resolved.findIndex((stop) => stop.offset >= t);
    if (upper < 0) upper = resolved.length - 1;
    const b = resolved[upper],
      a = resolved[Math.max(0, upper - 1)],
      mix = Math.max(0, Math.min(1, (t - a.offset) / Math.max(0.000001, b.offset - a.offset)));
    for (let channel = 0; channel < 4; channel++)
      data[i * 4 + channel] = Math.round(
        (a.color![channel] + (b.color![channel] - a.color![channel]) * mix) *
          (channel === 3 ? 255 : 1),
      );
  }
  const gl = painter.gl;
  gl.activeTexture(gl.TEXTURE1);
  gl.bindTexture(gl.TEXTURE_2D, painter.palette);
  gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, 256, 1, 0, gl.RGBA, gl.UNSIGNED_BYTE, data);
  return true;
}
/** Draws one mark; true when it drew from an approximate mask (moved or grown since it was built). */
function drawMark(
  painter: Painter,
  client: Client,
  scene: ChartOpticalScene,
  mark: ChartOpticalMark,
  ratio: number,
  approximate: boolean,
  exact: Exact | null,
) {
  const { gl, uniforms: u } = painter;
  const b = mark.bounds;
  if (![b.x, b.y, b.width, b.height].every(Number.isFinite) || b.width <= 0 || b.height <= 0)
    throw new Error("empty-geometry");
  const tint = color(painter, mark.color);
  if (!tint) throw new Error("unresolved-color");
  let approximated = false;
  // An exact shape needs no mask: the shader evaluates it on the grid a mask would cover.
  let rect: { x: number; y: number; w: number; h: number },
    grid: [number, number, number],
    gain = 1,
    range = 256;
  if (exact) {
    const g = opticalFieldGrid(b, ratio);
    rect = { x: g.x, y: g.y, w: g.cssWidth, h: g.cssHeight };
    grid = [g.x, g.y, g.scale];
    gl.uniform4fv(u["uExactShape[0]"], exact.shape);
  } else {
    const mapped = fieldFor(painter, client, mark, ratio, approximate);
    approximated = mapped.approximated;
    rect = mapped;
    gain = mapped.gain;
    grid = [mapped.x, mapped.y, mapped.entry.field.scale / gain];
    gl.activeTexture(gl.TEXTURE0);
    gl.bindTexture(gl.TEXTURE_2D, mapped.entry.texture);
    gl.uniform1i(u.uFieldFormat, mapped.entry.field.channels === 2 ? 1 : 0);
    range = mapped.entry.field.range;
  }
  gl.uniform1i(u.uExact, exact ? 1 : 0);
  gl.uniform3f(u.uGrid, grid[0], grid[1], grid[2]);
  gl.uniform1f(u.uFieldGain, gain);
  const hasPalette = palette(painter, mark);
  gl.uniform1i(u.uField, 0);
  gl.uniform1i(u.uPalette, 1);
  gl.uniform2f(u.uSize, scene.width, scene.height);
  gl.uniform1f(u.uRatio, ratio);
  gl.uniform4f(u.uFieldRect, rect.x, rect.y, rect.w, rect.h);
  gl.uniform4f(u.uBounds, b.x, b.y, b.width, b.height);
  gl.uniform1f(u.uRange, range);
  gl.uniform4f(u.uColor, tint[0] / 255, tint[1] / 255, tint[2] / 255, tint[3]);
  gl.uniform1f(u.uHasPalette, hasPalette ? 1 : 0);
  gl.uniform1f(u.uColorDirection, mark.colorDirection === "vertical" ? 1 : 0);
  gl.uniform1f(u.uTone, bounded(mark.tone));
  gl.uniform1f(u.uGlow, Math.max(0, mark.glow ?? 1));
  gl.uniform1f(u.uTheme, scene.theme === "light" ? 1 : 0);
  const small = Math.max(1, Math.min(b.width, b.height)),
    depth = Math.max(2, Math.min(78, mark.depth ?? small * 0.6)),
    bevel = Math.max(0.4, Math.min(depth * 0.49, mark.bevel ?? Math.min(6, small * 0.05)));
  gl.uniform1f(u.uDepth, depth);
  gl.uniform1f(u.uBevel, bevel);
  gl.uniform1f(u.uShell, Math.min(1.6, depth * 0.25));
  gl.uniform1f(u.uFill, bounded(mark.fill));
  let origin = mark.fillOrigin;
  const radial = mark.fillRadial ?? null;
  if (!origin && mark.shape?.kind === "sector")
    origin = {
      cx: mark.shape.cx,
      cy: mark.shape.cy,
      start: mark.shape.startAngle,
      end: mark.shape.endAngle,
      clockwise: mark.shape.endAngle >= mark.shape.startAngle,
    };
  gl.uniform1i(
    u.uOrigin,
    radial
      ? 5
      : typeof origin === "object"
        ? 4
        : origin === "top"
          ? 1
          : origin === "left"
            ? 2
            : origin === "right"
              ? 3
              : 0,
  );
  if (radial) {
    // Radial: the centre, then (inner radius, depth from inner to outer).
    const inner = Math.max(0, radial.inner ?? 0);
    gl.uniform2f(u.uAngularCenter, radial.cx, radial.cy);
    gl.uniform4f(u.uAngular, inner, Math.max(1, radial.outer - inner), 0, 0);
  } else if (typeof origin === "object") {
    gl.uniform2f(u.uAngularCenter, origin.cx, origin.cy);
    gl.uniform4f(
      u.uAngular,
      origin.start,
      Math.abs(origin.end - origin.start),
      origin.clockwise === false ? -1 : 1,
      0,
    );
  }
  gl.uniform1i(u.uKind, mark.kind === "area" ? 1 : 0);
  gl.uniform1f(u.uDepthAxis, mark.depthAxis === "x" ? 1 : 0);
  const depthReverse =
    mark.depthReverse ??
    (mark.depthAxis === "x" ? mark.fillOrigin === "left" : mark.fillOrigin === "top");
  gl.uniform1f(u.uDepthReverse, depthReverse ? 1 : 0);
  let shapeKind = 0,
    shape = [0, 0, 0, 0],
    tip = 0;
  if (mark.shape?.kind === "rect") {
    const q = mark.shape;
    shapeKind = 1;
    shape = [q.x + q.width / 2, q.y + q.height, q.width / 2, q.height];
  } else if (mark.shape?.kind === "sector") {
    const q = mark.shape;
    shapeKind = 2;
    shape = [q.cx, q.cy, (q.innerRadius + q.outerRadius) / 2, (q.outerRadius - q.innerRadius) / 2];
    gl.uniform2f(u.uAngles, q.startAngle, q.endAngle);
    tip = (q.tip ?? (radial ? "outer" : "end")) === "outer" ? 1 : 0;
  } else if (mark.shape?.kind === "circle") {
    const q = mark.shape;
    shapeKind = 3;
    shape = [q.cx, q.cy, q.r, 0];
  }
  gl.uniform1i(u.uShapeKind, shapeKind);
  gl.uniform1f(u.uTip, tip);
  gl.uniform4f(u.uShape, shape[0], shape[1], shape[2], shape[3]);
  gl.uniform1f(u.uOpacity, bounded(mark.opacity));
  gl.uniform3f(u.uFocus, mark.focus?.x ?? 0, mark.focus?.y ?? 0, bounded(mark.focus?.amount, 0));
  gl.uniform1f(u.uHatch, mark.hatch === false ? 0 : 1);
  const engraved = [0, 1, 2, 3].map((k) => {
    const at = mark.marks?.[k];
    if (at === undefined) return -1;
    // A graduation keeps its +2 (a tick) or +4 (a ring) (chart-optical-shader.ts).
    const kind = at > 3.5 ? 4 : at > 1.5 ? 2 : 0;
    return kind + Math.max(0, Math.min(1, at - kind));
  });
  gl.uniform4f(u.uMarks, engraved[0], engraved[1], engraved[2], engraved[3]);
  gl.uniform2f(
    u.uPattern,
    mark.pattern === undefined ? 0 : 1,
    Math.max(0, Math.min(1, mark.pattern ?? 0)),
  );
  gl.uniform4fv(u["uLiquidSurface[0]"], mark.field ?? FLAT_SURFACE);
  gl.uniform1f(u.uSeed, [...mark.id].reduce((sum, letter) => sum + letter.charCodeAt(0), 0) % 97);
  const x = Math.max(0, Math.floor(rect.x * ratio)),
    y = Math.max(0, Math.floor((scene.height - rect.y - rect.h) * ratio));
  const right = Math.min(painter.canvas.width, Math.ceil((rect.x + rect.w) * ratio)),
    top = Math.min(painter.canvas.height, Math.ceil((scene.height - rect.y) * ratio));
  if (right <= x || top <= y) throw new Error("offscreen");
  gl.scissor(x, y, right - x, top - y);
  gl.drawArrays(gl.TRIANGLES, 0, 3);
  // No per-mark getError: it blocks on the GPU (about 70 µs a call). Development checks once a
  // paint; production relies on webglcontextlost.
  return approximated;
}

/** One shared offscreen WebGL context per document; each chart receives a stable 2D bitmap.
 * No RAF, no mark cap, no SVG filters. Native fill may hide ONLY for returned rendered IDs.
 * Exact shapes (circle, rect, sector) need no mask. Path geometry changes rebuild exact masks,
 * except that a mark moving or growing by its transform draws from the mask it has and gets its
 * exact mask once it holds still; color/fill/tone/focus changes reuse the distance field.
 */
export type ChartOpticalRenderer = {
  draw(scene: ChartOpticalScene): ChartOpticalResult;
  invalidate?: () => void;
  destroy(): void;
};
export function createChartOpticalRenderer(canvas: HTMLCanvasElement): ChartOpticalRenderer {
  let painter: Painter | null = null,
    attempted = false,
    previous = "",
    previousResult: ChartOpticalResult | null = null,
    refine: ReturnType<typeof setTimeout> | undefined;
  const client: Client = {
    id: nextClient++,
    canvas,
    last: null,
    disposed: false,
    paint: (scene) => paint(scene, false),
    report: (result) => {
      previousResult = result;
      emit(canvas, result);
    },
  };
  const settle = () => {
    if (refine !== undefined) clearTimeout(refine);
    refine = undefined;
  };
  function paint(scene: ChartOpticalScene, exactOnly: boolean): ChartOpticalResult {
    client.last = scene;
    if (client.disposed) return resultFor(scene, "disposed");
    if (
      ![scene.width, scene.height].every(Number.isFinite) ||
      scene.width <= 0 ||
      scene.height <= 0
    )
      return resultFor(scene, "canvas-bounds");
    if (!attempted) {
      attempted = true;
      try {
        painter = createPainter(canvas.ownerDocument ?? document);
      } catch {
        painter = null;
      }
      if (painter) {
        painter.clients.add(client);
        if (painter.idle !== undefined) clearTimeout(painter.idle);
        painter.idle = undefined;
      }
    }
    if (!painter?.program || painter.lost || painter.gl.isContextLost())
      return resultFor(scene, painter?.reason || "webgl-unavailable");
    const ratio = Math.min(
      2,
      Math.max(1, scene.dpr ?? canvas.ownerDocument?.defaultView?.devicePixelRatio ?? 1),
    );
    const exacts = scene.marks.map(opticalExactShape);
    const key = JSON.stringify([
      scene.width,
      scene.height,
      ratio,
      scene.theme,
      scene.marks.map((mark, index) => [
        exacts[index] ? [mark.id, mark.bounds] : opticalGeometryKey(mark, ratio),
        mark.color,
        mark.colors,
        mark.palette,
        mark.colorDirection,
        mark.kind,
        mark.shape,
        mark.fill,
        mark.fillOrigin,
        mark.fillRadial,
        mark.tone,
        mark.opacity,
        mark.focus,
        mark.depth,
        mark.bevel,
        mark.hatch,
        mark.pattern,
        mark.marks,
        mark.glow,
        mark.field && fieldPrint(mark.field),
      ]),
    ]);
    if (!exactOnly && key === previous && previousResult?.available) return previousResult;
    const output = canvas.getContext("2d");
    if (!output) return resultFor(scene, "canvas-2d-unavailable");
    const { gl } = painter;
    const width = Math.max(1, Math.round(scene.width * ratio)),
      height = Math.max(1, Math.round(scene.height * ratio));
    const maxSide = Math.min(8192, Number(gl.getParameter(gl.MAX_RENDERBUFFER_SIZE)) || 8192);
    const backing = opticalBackingPlan(
      painter.canvas.width,
      painter.canvas.height,
      width,
      height,
      maxSide,
    );
    if (!backing) return resultFor(scene, "backing-budget");
    if (painter.canvas.width !== backing.width) painter.canvas.width = backing.width;
    if (painter.canvas.height !== backing.height) painter.canvas.height = backing.height;
    if (canvas.width !== width) canvas.width = width;
    if (canvas.height !== height) canvas.height = height;
    gl.viewport(0, 0, width, height);
    gl.useProgram(painter.program);
    gl.enable(gl.SCISSOR_TEST);
    gl.scissor(0, 0, width, height);
    gl.clearColor(0, 0, 0, 0);
    gl.clear(gl.COLOR_BUFFER_BIT);
    gl.enable(gl.BLEND);
    gl.blendFunc(gl.ONE, gl.ONE_MINUS_SRC_ALPHA);
    gl.activeTexture(gl.TEXTURE1);
    gl.bindTexture(gl.TEXTURE_2D, painter.palette);
    const result: ChartOpticalResult = {
      available: true,
      visible: false,
      rendered: [],
      fallback: [],
    };
    let approximated = false;
    for (const [index, mark] of scene.marks.entries()) {
      try {
        if (drawMark(painter, client, scene, mark, ratio, !exactOnly, exacts[index]))
          approximated = true;
        result.rendered.push(mark.id);
      } catch (error) {
        result.fallback.push({
          id: mark.id,
          reason: error instanceof Error ? error.message : "mark-render",
        });
      }
    }
    gl.disable(gl.SCISSOR_TEST);
    if (painter.lost || gl.isContextLost()) return resultFor(scene, "context-lost");
    if (DEV && result.rendered.length) {
      const error = gl.getError();
      if (error !== gl.NO_ERROR) {
        // One check a paint cannot say which mark failed, so none of them claims the glass.
        for (const id of result.rendered) result.fallback.push({ id, reason: `webgl-${error}` });
        result.rendered = [];
      }
    }
    try {
      output.setTransform(1, 0, 0, 1, 0, 0);
      output.clearRect(0, 0, width, height);
      output.drawImage(painter.canvas, 0, backing.sourceY, width, height, 0, 0, width, height);
    } catch {
      return resultFor(scene, "canvas-transfer");
    }
    result.visible = result.rendered.length > 0;
    previous = key;
    previousResult = result;
    settle();
    // Drawn from masks built for another place or size: once the marks hold still, draw exact.
    if (approximated) {
      refine = setTimeout(() => {
        refine = undefined;
        if (!client.disposed && client.last) client.report(paint(client.last, true));
      }, REFINE_MS);
    }
    return result;
  }
  return {
    draw(scene: ChartOpticalScene) {
      let result: ChartOpticalResult;
      try {
        result = paint(scene, false);
      } catch (error) {
        result = resultFor(
          scene,
          error instanceof Error ? `renderer-exception: ${error.message}` : "renderer-exception",
        );
      }
      if (!lostFrame(result)) canvas.style.visibility = result.visible ? "" : "hidden";
      canvas.dataset.opticalRenderer = result.available ? "webgl-shared" : "native-fallback";
      canvas.dataset.opticalMarks = String(result.rendered.length);
      canvas.dataset.opticalFallback = result.fallback
        .map((entry) => `${entry.id}:${entry.reason}`)
        .join(";");
      return result;
    },
    invalidate() {
      previous = "";
      previousResult = null;
    },
    destroy() {
      if (client.disposed) return;
      client.disposed = true;
      settle();
      canvas.style.visibility = "hidden";
      if (painter) {
        for (const [key, entry] of painter.entries)
          if (key.startsWith(`${client.id}:`)) release(painter, key, entry);
        painter.clients.delete(client);
        if (!painter.clients.size && painter.idle === undefined) {
          // Keep the context, program and backing for the next chart (a page switch); let them
          // go only if none arrives.
          painter.idle = setTimeout(expireIdle, IDLE_MS, painter);
          (painter.idle as unknown as { unref?: () => void }).unref?.();
        }
      }
    },
  };
}
