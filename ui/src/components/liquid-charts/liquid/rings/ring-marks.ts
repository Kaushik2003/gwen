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
"use client";

import { pixelRatio } from "@/components/liquid-charts/liquid/core/render-scale";
import type {
  RingEngine,
  RingFrame,
  RingGlass,
  RingLayout,
  RingMark,
  RingSegmentFrame,
  RingShape,
  RingTilt,
} from "@/components/liquid-charts/liquid/core/ring-types";
import {
  angleOf,
  normAngle,
  polar,
  sectorPath,
  TAU,
  trimmed,
  wrapAngle,
} from "@/components/liquid-charts/liquid/core/arc";
import { glassView, projectPoint, type GlassCamera } from "@/components/liquid-charts/liquid/core/camera";
import { chartPresence, frameDelta, substeps } from "@/components/liquid-charts/liquid/core/presence";
import { focusPresses, trackPointer } from "@/components/liquid-charts/liquid/core/liquid-pointer";
import { useMaterials, type MarkLiquid } from "@/components/liquid-charts/liquid/core/materials";
import { cssRgb as css } from "@/components/liquid-charts/liquid/core/colour-scale";
import { clamp01, settle } from "@/components/liquid-charts/liquid/core/geometry";
import { liquidFeel } from "@/components/liquid-charts/liquid/core/liquid-feel";
import type { Rgb, Theme } from "@/components/liquid-charts/liquid/core/colour";
import { useEffect, useId, useRef, useState } from "react";

/* The ring marks as data live in the core (`ring-types.ts`), where the material pack and the
 * ring glass read them too; this module re-exports them so a chart imports everything ring from
 * here. */
export type {
  RingEngine,
  RingFrame,
  RingGlass,
  RingLayout,
  RingMark,
  RingScene,
  RingSegmentFrame,
  RingShape,
  RingTilt,
} from "@/components/liquid-charts/liquid/core/ring-types";

/** The raymarched renderer draws up to this many segments in one ring. */
export const RING_LIMIT = 24;
/** …and up to this many marks on concentric tracks (each with its own radius or width). */
export const CONCENTRIC_LIMIT = 48;

const easeInOut = (t: number) => {
  const u = clamp01(t);
  return u * u * (3 - 2 * u);
};
const mix = (a: number, b: number, t: number) => a + (b - a) * t;
const patternStart = (pattern: RingMark["pattern"]) =>
  pattern === undefined || pattern === false ? undefined : pattern === true ? 0 : pattern;
const SVG = "http://www.w3.org/2000/svg";
/** Flat graphite, per theme: the SVG twin of the glass's smoked tone. */
const FLAT_GREY: Record<Theme, Rgb> = { dark: [0.26, 0.26, 0.26], light: [0.81, 0.81, 0.81] };

/** A segment's flat (SVG) colour at a tone: its tint, eased towards graphite as it leaves focus. */
export function flatColour(tint: Rgb, tone: number, theme: Theme) {
  const grey = FLAT_GREY[theme];
  const t = clamp01(tone);
  return css([mix(grey[0], tint[0], t), mix(grey[1], tint[1], t), mix(grey[2], tint[2], t)]);
}

/** True when some mark sits on its own track (its radius or width differs from the ring's). */
export function isConcentric(
  marks: readonly RingMark[],
  ring: Pick<RingShape, "radius" | "width">,
) {
  return marks.some(
    (mark) =>
      (mark.radius !== undefined && Math.abs(mark.radius - ring.radius) > 0.01) ||
      (mark.width !== undefined && Math.abs(mark.width - ring.width) > 0.01),
  );
}

/** The engine that can draw these marks: SVG when glass is off or unavailable; the optical
 * renderer for round ends (progress rings) or more segments than the raymarched ring takes (24 on
 * one ring, `CONCENTRIC_LIMIT` on concentric tracks); the raymarched ring otherwise, which is the
 * only engine that tilts. */
export function ringEngine(
  marks: readonly RingMark[],
  ring: Pick<RingShape, "radius" | "width">,
  glass: boolean,
): RingEngine {
  if (!glass) return "svg";
  if (marks.some((mark) => mark.round)) return "optical";
  const limit = isConcentric(marks, ring) ? CONCENTRIC_LIMIT : RING_LIMIT;
  return marks.length > limit ? "optical" : "raymarch";
}

/** A mark's middle radius and thickness at a camera progress (0 flat … 1 tilted): its own flat
 * values (or the ring's), moving to its tilted ones (or the flat ones scaled as the ring's are). */
export function markGeometry(
  mark: Pick<RingMark, "radius" | "width" | "tilted">,
  ring: Pick<RingShape, "radius" | "width">,
  tilt: Pick<RingTilt, "radius" | "width"> | undefined,
  camera: number,
) {
  const radius = mark.radius ?? ring.radius,
    width = mark.width ?? ring.width;
  if (!tilt) return { radius, width };
  const tiltRadius =
    mark.tilted?.radius ??
    (mark.radius === undefined
      ? tilt.radius
      : (radius * tilt.radius) / Math.max(1e-6, ring.radius));
  const tiltWidth =
    mark.tilted?.width ??
    (mark.width === undefined ? tilt.width : (width * tilt.width) / Math.max(1e-6, ring.width));
  return { radius: mix(radius, tiltRadius, camera), width: mix(width, tiltWidth, camera) };
}

/** The eased camera for a transition progress `camera` (0–1) from the flat view to `tilt`, as the
 * approved donut does it: perspective closes in as the camera pitches, so the flat ring becomes
 * the tilted one without a cut. */
export function lensFor(
  camera: number,
  flat: { cx: number; cy: number },
  tilt: RingTilt,
): GlassCamera | undefined {
  if (camera <= 0) return undefined;
  const c = tilt.camera;
  return {
    tilt: Math.round(camera * 1e4) / 1e4,
    elevation: mix(Math.PI / 2, c.elevation, camera),
    distance: c.distance / Math.max(camera * camera, c.distance / 20000),
    anchor: { x: mix(flat.cx, c.anchor.x, camera), y: mix(flat.cy, c.anchor.y, camera) },
  };
}

type State = {
  start: number;
  end: number;
  vStart: number;
  vEnd: number;
  pop: number;
  tone: number;
  height: number;
  vHeight: number;
  level: number;
  /** Continuous rings: radius and width (flat and tilted) on springs, how present the mark is
   * (0 arriving or gone … 1 there), seconds it still waits before filling in, and the last mark
   * it was, for drawing it while it drains out. */
  radius: number;
  vRadius: number;
  width: number;
  vWidth: number;
  tiltRadius: number;
  vTiltRadius: number;
  tiltWidth: number;
  vTiltWidth: number;
  presence: number;
  wait: number;
  leaving: boolean;
  mark: RingMark;
  /** The slab's simulated liquid, chasing the sweep, and its surface for the shaders. */
  liquid: MarkLiquid;
  field: Float32Array;
};

/** A segment's liquid where there is no glass: its front is simply where the level is. */
function stillLiquid(): MarkLiquid {
  let front = 0;
  return {
    get front() {
      return front;
    },
    reset: () => (front = 0),
    settle: (rest) => (front = rest),
    press: () => {},
    step: (_dt, { target }) => {
      front = target;
      return false;
    },
    rows: () => {},
    takeImpact: () => ({ land: 0, splash: 0 }),
  };
}

/** One step of the glass's spring (stiffness 110, damping 21); returns true while it moves. */
function springStep(
  s: Record<string, number>,
  key: string,
  velocity: string,
  target: number,
  dt: number,
  rest = 0.02,
) {
  for (let k = 0, n = substeps(dt); k < n; k++) {
    s[velocity] += (110 * (target - s[key]) - 21 * s[velocity]) * (dt / n);
    s[key] += s[velocity] * (dt / n);
  }
  if (Math.abs(target - s[key]) < rest && Math.abs(s[velocity]) < rest * 10) {
    s[key] = target;
    s[velocity] = 0;
    return false;
  }
  return true;
}

/**
 * One glass canvas for a ring of segments: donuts, pies, semi donuts, gauges and concentric
 * progress rings. Segments spring to new angles, sweep in clockwise on entrance, pop out, and fade
 * between tint and graphite. A ring can turn to a tilted view: the camera pitches over the first
 * 60% of the move, the ring turns the short way to bring a slab to the back, and slabs rise on
 * staggered springs to their heights; going back runs it in reverse. `onFrame` hands back the
 * current geometry and a projector so labels and tooltips follow the camera. It draws only while
 * something moves.
 *
 * The glass comes from `glass`, or else the material pack in reach (`useMaterials().ring`, from
 * the materials). With it, the canvas reaches `bleed` px past the plot so the glow fades out; one
 * ring of up to 24 segments, or up to `CONCENTRIC_LIMIT` marks on concentric tracks
 * (`RingMark.radius`/`width`), is raymarched: the edge-lit glass, and the only engine that tilts,
 * with true depth between tracks. Round ends (progress rings) use the optical painter. Without
 * glass, with `material` "flat", or without WebGL2, the same frames draw into the `svg` layer
 * instead and `available` turns false.
 *
 * A layout marked `continuous` keeps one canvas through changes of structure (a sunburst's drill):
 * tracks spring to new radii, late marks sweep in on their own and leaving ones retract, so
 * nothing replays the entrance. Remounting the hook still replays it, so keep one layer mounted and
 * hand it changing geometry rather than choosing canvases by key.
 */
export function useRingMarks({
  theme,
  layout,
  onFrame,
  replay = 0,
  bleed = 48,
  material = "glass",
  glass: ownGlass,
}: {
  theme: Theme;
  layout: (width: number, height: number) => RingLayout;
  onFrame?: (frame: RingFrame) => void;
  /** Change to replay the entrance. */
  replay?: number;
  bleed?: number;
  material?: "glass" | "flat";
  /** The glass, for a chart that brings its own (the families that draw in glass pass liquid-core-pro's). */
  glass?: (canvas: HTMLCanvasElement) => RingGlass | null;
}) {
  const pack = useMaterials();
  const makeGlass = ownGlass ?? pack.ring;
  const box = useRef<HTMLDivElement>(null);
  const canvas = useRef<HTMLCanvasElement>(null);
  const svg = useRef<SVGSVGElement>(null);
  const shade = useRef<SVGSVGElement>(null);
  const last = useRef<RingFrame | null>(null);
  /** Where plot px (0, 0) sits on the canvas: the bleed, plus the fraction that puts the canvas on
   * whole device pixels. */
  const origin = useRef({ x: bleed, y: bleed });
  const uid = useId().replace(/[^a-zA-Z0-9_-]/g, "");
  // No glass in reach means none on the first render either, so a tilt switch never flashes up.
  const [available, setAvailable] = useState(() => Boolean(makeGlass));
  const live = useRef({ theme, replay, layout, onFrame, material, wake: () => {} });

  useEffect(() => {
    Object.assign(live.current, { theme, replay, layout, onFrame, material });
    live.current.wake();
  });

  useEffect(() => {
    const surface = canvas.current,
      root = box.current;
    if (!root) return;
    const shared = live.current;
    const glass = surface && makeGlass ? makeGlass(surface) : null;
    setAvailable(Boolean(glass));
    let glassFailed = false;
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    const edge = bleed;
    if (surface)
      Object.assign(surface.style, {
        left: `${-edge}px`,
        top: `${-edge}px`,
        width: `calc(100% + ${edge * 2}px)`,
        height: `calc(100% + ${edge * 2}px)`,
      });
    let width = 0,
      height = 0,
      canvasWidth = 0,
      canvasHeight = 0;
    const offset = origin.current;
    // The canvas is a whole number of CSS px and starts on a whole device pixel, so its backing
    // store maps 1:1 to the screen. A fractional size or position would make the browser resample
    // it, which softens and stair-steps every edge.
    const observer = new ResizeObserver(() => {
      const r = root.getBoundingClientRect();
      const ratio = pixelRatio();
      const fraction = (value: number) => ((((value * ratio) % 1) + 1) % 1) / ratio;
      width = r.width;
      height = r.height;
      offset.x = edge + fraction(r.left);
      offset.y = edge + fraction(r.top);
      canvasWidth = Math.ceil(width + offset.x + edge);
      canvasHeight = Math.ceil(height + offset.y + edge);
      if (surface) {
        surface.style.left = `${-offset.x}px`;
        surface.style.top = `${-offset.y}px`;
      }
      glass?.resize(canvasWidth, canvasHeight);
      wake();
    });
    observer.observe(root);
    // The pointer stirs the glass's liquid; without glass nothing listens for it.
    const pointer = glass ? trackPointer(root, () => wake()) : null;
    const presence = chartPresence(root, () => wake(), { redraw });
    // The optical renderer announces a restored context on the canvas: pick the motion back up.
    const onOptical = (event: Event) => {
      if ((event as CustomEvent<{ available?: boolean }>).detail?.available) wake();
    };
    surface?.addEventListener("chart-optical-status", onOptical);
    const presses = focusPresses();
    const feel = liquidFeel(root);
    let seeds = 0;
    const state = new Map<string, State>();
    let replayed = shared.replay,
      start = presence.clock(performance.now()),
      raf = 0,
      before = 0,
      phase = 0,
      turn = 0,
      engine: RingEngine | null = null,
      // Continuous rings: whether the entrance's marks have been laid out (later marks arrive on
      // their own), and the plot size last frame (a resize snaps geometry instead of springing).
      primed = false,
      lastWidth = 0,
      lastHeight = 0;
    const paths = new Map<string, SVGPathElement[]>();
    const shadows = new Map<string, { clip: SVGPathElement; cast: SVGPathElement }>();

    function clearSvg() {
      paths.forEach((group) => group.forEach((path) => path.remove()));
      paths.clear();
    }

    function step(frameTime: number) {
      raf = 0;
      if (!width || !height) return;
      // Offscreen or in a hidden tab: keep every segment as it is and draw nothing until it is
      // back. Time runs on the presence clock, held meanwhile.
      if (!presence.active) {
        before = 0;
        return;
      }
      const now = presence.clock(frameTime);
      // The real time since the last frame; past a quarter second (or in a settled frame, for
      // print and export) the motion lands at its end instead of playing in slow motion.
      const { dt, snap } = frameDelta(now, before);
      const jump = snap || presence.settling;
      before = now;
      if (shared.replay !== replayed) {
        replayed = shared.replay;
        start = now;
        primed = false;
        for (const s of state.values()) s.liquid.reset();
      }
      const elapsed = (now - start) / 1000;
      const target = shared.layout(width, height);
      const { ring, marks } = target;
      const continuous = Boolean(target.continuous);
      const resized = width !== lastWidth || height !== lastHeight;
      lastWidth = width;
      lastHeight = height;
      const nextEngine = ringEngine(
        marks,
        ring,
        shared.material === "glass" && Boolean(glass) && !glassFailed,
      );
      if (nextEngine !== engine) {
        // Another engine paints the canvas now: the glass must not repaint over it.
        if (engine === "raymarch" || engine === "optical") glass?.invalidate();
        if (engine === "svg") clearSvg();
        if (nextEngine !== "svg" && surface) surface.style.visibility = "";
        if (nextEngine === "svg" && surface) {
          surface.getContext("2d")?.clearRect(0, 0, surface.width, surface.height);
        }
        engine = nextEngine;
      }
      const tilt = engine === "raymarch" ? target.tilt : undefined;
      let moving = false;

      // The view: one phase from flat (0) to tilted (1), run at a steady pace.
      const goalPhase = target.view === "tilt" && tilt ? 1 : 0;
      phase +=
        Math.sign(goalPhase - phase) *
        Math.min(reduced || jump ? 1 : dt / 0.9, Math.abs(goalPhase - phase));
      if (phase !== goalPhase) moving = true;
      const camera = easeInOut(phase / 0.6);
      // Tilted, the ring turns the short way to bring the chosen slab to the back.
      const home = goalPhase ? (target.turn ?? 0) : 0;
      const goalTurn = home + TAU * Math.round((turn - home) / TAU);
      turn += (goalTurn - turn) * (reduced || jump ? 1 : 1 - Math.exp(-7 * dt));
      if (Math.abs(goalTurn - turn) > 1e-3) moving = true;
      else turn = goalTurn;

      const flatDepth = ring.depth ?? ring.width * 0.7;
      const depth = tilt ? mix(flatDepth, tilt.depth, camera) : flatDepth;
      const radius = tilt ? mix(ring.radius, tilt.radius, camera) : ring.radius;
      const ringWidth = tilt ? mix(ring.width, tilt.width, camera) : ring.width;
      const gap = tilt
        ? mix(ring.gap ?? 0, tilt.gap ?? (ring.gap ?? 0) + 1, camera)
        : (ring.gap ?? 0);
      const bevelFlat = ring.bevel ?? Math.min(6, ring.width * 0.12);
      const bevel = tilt
        ? mix(bevelFlat, tilt.bevel ?? Math.min(5, tilt.width * 0.08), camera)
        : bevelFlat;
      const sweepFrom = target.sweep?.from ?? Math.min(...marks.map((mark) => mark.start), 0);
      const sweepSpan = target.sweep?.span ?? TAU;
      const sweep = (delay: number) =>
        reduced ? sweepSpan : easeInOut((elapsed - 0.3 - delay) / 1.6) * sweepSpan;
      if (!reduced && sweep(Math.max(0, ...marks.map((mark) => mark.delay ?? 0))) < sweepSpan)
        moving = true;

      // Slabs that just came into focus (hover or keyboard) are pressed: their liquid sags back.
      const joined = reduced || presence.staticGlass ? [] : presses(marks);
      const segments: RingSegmentFrame[] = marks.map((mark, index) => {
        let s = state.get(mark.key);
        const level = clamp01(mark.level ?? 1);
        const lowHeight = tilt?.low ?? flatDepth;
        // Where the mark's track is, flat and tilted (its own, or the ring's).
        const flatGoal = markGeometry(mark, ring, tilt, 0);
        const tiltGoal = markGeometry(mark, ring, tilt, 1);
        if (!s) {
          // A mark that arrives after the entrance of a continuous ring starts from `from`.
          const late = continuous && primed;
          const from = late ? mark.from : undefined;
          const scaleR = tiltGoal.radius / Math.max(1e-6, flatGoal.radius),
            scaleW = tiltGoal.width / Math.max(1e-6, flatGoal.width);
          s = {
            start: from?.start ?? mark.start,
            end: from?.end ?? mark.end,
            vStart: 0,
            vEnd: 0,
            pop: mark.pop ?? 0,
            tone: mark.lit,
            height: flatDepth,
            vHeight: 0,
            level,
            radius: from?.radius ?? flatGoal.radius,
            vRadius: 0,
            width: from?.width ?? flatGoal.width,
            vWidth: 0,
            tiltRadius: from?.radius === undefined ? tiltGoal.radius : from.radius * scaleR,
            vTiltRadius: 0,
            tiltWidth: from?.width === undefined ? tiltGoal.width : from.width * scaleW,
            vTiltWidth: 0,
            presence: late && !reduced ? 0 : 1,
            wait: late && !reduced ? (mark.delay ?? 0) : 0,
            leaving: false,
            mark,
            liquid: glass ? glass.liquid(++seeds) : stillLiquid(),
            field: new Float32Array(glass?.fieldLength ?? 0),
          };
          state.set(mark.key, s);
        }
        s.mark = mark;
        s.leaving = false;
        if (continuous) {
          // Geometry springs to its new track; a resize (or reduced motion) snaps it there.
          const geometry = s as unknown as Record<string, number>;
          const goals: [string, string, number][] = [
            ["radius", "vRadius", flatGoal.radius],
            ["width", "vWidth", flatGoal.width],
            ["tiltRadius", "vTiltRadius", tiltGoal.radius],
            ["tiltWidth", "vTiltWidth", tiltGoal.width],
          ];
          for (const [key, velocity, goal] of goals) {
            if (reduced || resized || jump) {
              geometry[key] = goal;
              geometry[velocity] = 0;
            } else if (springStep(geometry, key, velocity, goal, dt)) moving = true;
          }
          // A late mark waits for its delay, then fills in.
          if (reduced || jump) s.presence = 1;
          else if (s.wait > 0) {
            s.wait -= dt;
            moving = true;
          } else if (s.presence < 1) {
            s.presence = Math.min(1, s.presence + dt / 0.8);
            moving = true;
          }
        }
        if (reduced || jump) {
          s.start = mark.start;
          s.end = mark.end;
          s.vStart = s.vEnd = 0;
          s.pop = mark.pop ?? 0;
          s.tone = mark.lit;
          s.level = level;
        } else {
          // Angles spring; pop, tone and level ease.
          for (let k = 0, n = substeps(dt); k < n; k++) {
            const h = dt / n;
            s.vStart += (110 * (mark.start - s.start) - 21 * s.vStart) * h;
            s.vEnd += (110 * (mark.end - s.end) - 21 * s.vEnd) * h;
            s.start += s.vStart * h;
            s.end += s.vEnd * h;
          }
          if (
            Math.abs(mark.start - s.start) < 1e-4 &&
            Math.abs(mark.end - s.end) < 1e-4 &&
            Math.abs(s.vStart) < 1e-3 &&
            Math.abs(s.vEnd) < 1e-3
          ) {
            s.start = mark.start;
            s.end = mark.end;
            s.vStart = s.vEnd = 0;
          } else moving = true;
          const k = 1 - Math.exp(-11 * dt);
          s.pop += ((mark.pop ?? 0) - s.pop) * k;
          s.tone += (mark.lit - s.tone) * k;
          s.level += (level - s.level) * (1 - Math.exp(-7 * dt));
          if (
            Math.abs((mark.pop ?? 0) - s.pop) > 0.03 ||
            Math.abs(mark.lit - s.tone) > 0.004 ||
            Math.abs(level - s.level) > 0.002
          )
            moving = true;
          else {
            s.pop = mark.pop ?? 0;
            s.tone = mark.lit;
            s.level = level;
          }
        }
        // Heights rise in a wave out from the back of the ring, each on its own spring.
        const mid = (mark.start + mark.end) / 2;
        const away = Math.abs(wrapAngle(mid + home)) / Math.PI;
        const rise = easeInOut((phase - 0.3 - away * 0.22) / 0.45);
        const goal = mix(depth, mark.height ?? lowHeight, rise);
        if (reduced || jump || phase === 0) {
          s.height = goal;
          s.vHeight = 0;
        } else {
          for (let k = 0, n = substeps(dt); k < n; k++) {
            s.vHeight += (240 * (goal - s.height) - 19 * s.vHeight) * (dt / n);
            s.height += s.vHeight * (dt / n);
          }
          if (Math.abs(goal - s.height) < 0.05 && Math.abs(s.vHeight) < 0.5) {
            s.height = goal;
            s.vHeight = 0;
          } else moving = true;
        }
        const entrance = clamp01(
          (sweep(mark.delay ?? 0) - (mark.start - sweepFrom)) /
            Math.max(1e-6, mark.end - mark.start),
        );
        const track = continuous
          ? {
              radius: mix(s.radius, s.tiltRadius, camera),
              width: mix(s.width, s.tiltWidth, camera),
            }
          : markGeometry(mark, ring, tilt, camera);
        // A late mark sweeps in clockwise, glass and liquid together, so no empty shell shows.
        const grown = easeInOut(s.presence);
        // The liquid chases the sweep: it lags, lands against a full slab's end and sags back, and
        // its surface moves point by point across the track.
        const length = Math.max(1, (s.end - s.start) * grown * track.radius);
        const rest = entrance * level * length;
        if (reduced || jump || s.presence < 1) s.liquid.settle(rest);
        else {
          let near: { s: number; push: number } | null = null;
          const p = pointer?.current;
          if (p && !tilt) {
            const dx = p.x - ring.cx,
              dy = p.y - ring.cy;
            const r = Math.hypot(dx, dy);
            const angle = (Math.atan2(dx, -dy) + TAU) % TAU;
            const front = s.start + turn + (s.liquid.front / length) * (s.end - s.start);
            let gap = Math.abs(angle - (((front % TAU) + TAU) % TAU));
            gap = Math.min(gap, TAU - gap) * track.radius;
            const radial = (p.vx * dx + p.vy * dy) / Math.max(r, 1);
            const proximity =
              Math.exp(-((gap / (track.width * 2)) ** 2)) *
              Math.exp(-(((r - track.radius) / track.width) ** 2));
            const push = radial * proximity;
            near = {
              s: Math.max(-1, Math.min(1, (r - track.radius) / (track.width / 2))),
              push: Math.abs(push) < 2 ? 0 : push,
            };
          }
          if (joined.includes(mark.key)) {
            s.liquid.press(near?.s ?? 0, track.width);
            feel.press(mark.key, near !== null);
          }
          const liquidMoving = s.liquid.step(dt, {
            target: rest,
            thickness: track.width,
            depth: Math.max(track.width * 0.7, 1),
            ceiling: level > 0.999 && entrance >= 1 ? length + 1 : undefined,
            pointer: near,
          });
          feel.after(mark.key, s.liquid);
          if (liquidMoving) moving = true;
        }
        s.liquid.rows(s.field, 0, rest);
        return {
          ...mark,
          index,
          start: s.start + turn,
          end: s.start + (s.end - s.start) * grown + turn,
          pop: s.pop,
          height: s.height,
          tone: s.tone,
          fill: Math.max(0, s.liquid.front / length),
          radius: track.radius,
          width: track.width,
        };
      });
      if (marks.length) primed = true;
      if (pointer?.settle(dt)) moving = true;
      // Keys that left the layout are forgotten; on a continuous ring they drain out first, drawn
      // where they were but no longer part of the frame (nothing can point at them).
      const leaving: RingSegmentFrame[] = [];
      if (state.size > marks.length) {
        const keys = new Set(marks.map((mark) => mark.key));
        for (const [key, s] of state) {
          if (keys.has(key)) continue;
          if (!continuous || reduced || jump) {
            state.delete(key);
            continue;
          }
          s.leaving = true;
          s.presence = Math.max(0, s.presence - dt / 0.4);
          if (s.presence <= 0) {
            state.delete(key);
            continue;
          }
          moving = true;
          // It retracts the way it came in, back to its start.
          leaving.push({
            ...s.mark,
            index: -1,
            start: s.start + turn,
            end: s.start + (s.end - s.start) * easeInOut(s.presence) + turn,
            pop: s.pop,
            height: s.height,
            tone: s.tone,
            fill: s.level,
            radius: mix(s.radius, s.tiltRadius, camera),
            width: mix(s.width, s.tiltWidth, camera),
          });
        }
      }
      // The raymarched ring draws each slab on its own track when any mark has one.
      const ownTracks = continuous || isConcentric(marks, ring);

      // The camera works in plot px; the view reads canvas px, which start at the offset.
      const plotLens = tilt ? lensFor(camera, ring, tilt) : undefined;
      const lens = plotLens && {
        ...plotLens,
        anchor: { x: plotLens.anchor.x + offset.x, y: plotLens.anchor.y + offset.y },
      };
      const floor = -depth;
      const view = glassView([ring.cx + offset.x, ring.cy + offset.y, floor], lens);
      // Segments narrower than their padding are not drawn (a hidden one springing shut).
      const drawn = [...segments, ...leaving].filter(
        (segment) => (segment.end - segment.start) * segment.radius > gap + 0.5,
      );

      if (engine !== "svg" && glass) {
        const result = glass.paint({
          engine,
          theme: shared.theme,
          size: { width: canvasWidth, height: canvasHeight },
          offset,
          ring: { cx: ring.cx, cy: ring.cy, radius, width: ringWidth, depth, gap, bevel },
          camera: plotLens,
          ownTracks,
          segments: drawn.map((segment) => ({
            ...segment,
            field:
              !presence.staticGlass && segment.index >= 0
                ? state.get(segment.key)?.field
                : undefined,
          })),
        });
        if (result === "lost") {
          // The context is lost for now: keep the last frame until the restore repaints it.
          moving = false;
        } else if (result === "unavailable") {
          // No glass on this device: draw flat from the next frame.
          glassFailed = true;
          setAvailable(false);
          moving = true;
        }
      } else if (engine === "svg" && svg.current) {
        drawSvg(svg.current, drawn, ring, gap, bevel, shared.theme);
      }
      if (shade.current) drawShadows(shade.current, drawn, ring, shared.theme);

      const frame: RingFrame = {
        width,
        height,
        ring: { cx: ring.cx, cy: ring.cy, radius, width: ringWidth, depth, gap },
        segments,
        phase,
        camera,
        turn,
        floor,
        view,
        engine,
        project: (x, y, z = floor) => {
          const at = projectPoint(view, [x + offset.x, y + offset.y, z]);
          return { x: settle(at.x - offset.x), y: settle(at.y - offset.y) };
        },
        at: (angle, r, z = 0) => {
          const point = polar(ring.cx, ring.cy, r, angle);
          const at = projectPoint(view, [point.x + offset.x, point.y + offset.y, floor + z]);
          return { x: settle(at.x - offset.x), y: settle(at.y - offset.y) };
        },
      };
      last.current = frame;
      shared.onFrame?.(frame);
      // It loops only on screen once entered (about a third visible); else it holds this frame.
      if (moving && presence.running) raf = requestAnimationFrame(step);
      else before = 0;
    }

    function drawSvg(
      layer: SVGSVGElement,
      drawn: RingSegmentFrame[],
      ring: RingShape,
      gap: number,
      bevel: number,
      theme: Theme,
    ) {
      const keys = new Set(drawn.map((segment) => segment.key));
      for (const [key, group] of paths)
        if (!keys.has(key)) {
          group.forEach((path) => path.remove());
          paths.delete(key);
        }
      const hatch = `${uid}-hatch`;
      if (!layer.querySelector("pattern")) {
        // The flat twin of the etched glass: fine diagonal lanes.
        const defs = document.createElementNS(SVG, "defs");
        defs.innerHTML = `<pattern id="${hatch}" width="6" height="6" patternUnits="userSpaceOnUse" patternTransform="rotate(45)"><rect width="2" height="6" fill="white" opacity="0.4"/></pattern>`;
        layer.prepend(defs);
      }
      for (const segment of drawn) {
        let group = paths.get(segment.key);
        if (!group) {
          group = [0, 1, 2].map(() => {
            const path = document.createElementNS(SVG, "path");
            path.setAttribute("fill-rule", "evenodd");
            return path;
          });
          group[2].setAttribute("fill", `url(#${hatch})`);
          layer.append(...group);
          paths.set(segment.key, group);
        }
        const [shell, liquid, etch] = group;
        const mid = (segment.start + segment.end) / 2;
        const shift = segment.pop * 0.3;
        const cx = ring.cx + Math.sin(mid) * shift,
          cy = ring.cy - Math.cos(mid) * shift;
        const inner = segment.radius - segment.width / 2,
          outer = segment.radius + segment.width / 2;
        const full = segment.end - segment.start >= TAU - 1e-6;
        const [a0, a1] = full
          ? [segment.start, segment.start + TAU]
          : trimmed(segment.start, segment.end, gap, segment.radius);
        const corner = segment.round ? segment.width / 2 : Math.min(bevel, segment.width * 0.2);
        const colour = flatColour(segment.tint, segment.tone, theme);
        shell.setAttribute("d", sectorPath(cx, cy, inner, outer, a0, a1, corner));
        shell.setAttribute("fill", colour);
        shell.setAttribute("opacity", segment.fill >= 0.999 ? "0" : "0.16");
        const end = a0 + (a1 - a0) * segment.fill;
        const filled =
          segment.fill <= 0.001 ? "" : sectorPath(cx, cy, inner, outer, a0, end, corner);
        liquid.setAttribute("d", filled);
        liquid.setAttribute("fill", colour);
        const from = patternStart(segment.pattern);
        etch.setAttribute(
          "d",
          from === undefined || segment.fill <= 0.001
            ? ""
            : sectorPath(cx, cy, inner, outer, a0 + (a1 - a0) * from, end, corner),
        );
      }
    }

    /** A soft shadow beyond the end of each mark that asks for one, falling on the band under it
     * (and clipped off the mark itself), so a lap on top reads as lying on the lap below. */
    function drawShadows(
      layer: SVGSVGElement,
      drawn: RingSegmentFrame[],
      ring: RingShape,
      theme: Theme,
    ) {
      const blur = `${uid}-blur`;
      if (!layer.querySelector("filter")) {
        const defs = document.createElementNS(SVG, "defs");
        defs.innerHTML = `<filter id="${blur}" x="-50%" y="-50%" width="200%" height="200%"><feGaussianBlur stdDeviation="3"/></filter>`;
        layer.prepend(defs);
      }
      const wanted = new Set<string>();
      for (const segment of drawn) {
        if (!segment.shadow || segment.fill <= 0.001) continue;
        wanted.add(segment.key);
        let shadow = shadows.get(segment.key);
        if (!shadow) {
          const clipPath = document.createElementNS(SVG, "clipPath");
          clipPath.id = `${uid}-clip-${shadows.size}`;
          const clip = document.createElementNS(SVG, "path");
          clip.setAttribute("clip-rule", "evenodd");
          clipPath.append(clip);
          const cast = document.createElementNS(SVG, "path");
          cast.setAttribute("clip-path", `url(#${clipPath.id})`);
          cast.setAttribute("filter", `url(#${blur})`);
          layer.querySelector("defs")?.append(clipPath);
          layer.append(cast);
          shadow = { clip, cast };
          shadows.set(segment.key, shadow);
        }
        const r = segment.radius,
          w = segment.width;
        const inner = r - w / 2,
          outer = r + w / 2;
        const end = segment.start + (segment.end - segment.start) * segment.fill;
        const corner = segment.round ? w / 2 : 0;
        // Clip: the band under the mark, less the mark itself.
        shadow.clip.setAttribute(
          "d",
          sectorPath(ring.cx, ring.cy, inner + 0.5, outer - 0.5, 0, TAU) +
            sectorPath(ring.cx, ring.cy, inner, outer, segment.start, end, corner),
        );
        // The cast: the mark's last stretch, nudged forward along the ring.
        const reach = Math.min(end - segment.start, (w * 1.4) / r);
        const nudge = ((6 / r) * 180) / Math.PI;
        shadow.cast.setAttribute(
          "d",
          sectorPath(ring.cx, ring.cy, inner + 1, outer - 1, end - reach, end, corner),
        );
        shadow.cast.setAttribute(
          "transform",
          `rotate(${settle(nudge)} ${settle(ring.cx)} ${settle(ring.cy)})`,
        );
        shadow.cast.setAttribute("fill", "black");
        shadow.cast.setAttribute(
          "opacity",
          String(
            settle(
              (theme === "dark" ? 0.9 : 0.5) *
                Math.min(1, segment.fill * 4) *
                (0.4 + 0.6 * segment.tone),
            ),
          ),
        );
      }
      for (const [key, shadow] of shadows) if (!wanted.has(key)) shadow.cast.setAttribute("d", "");
    }

    function wake() {
      if (!raf) raf = requestAnimationFrame(step);
    }
    /** Draws a frame now (print, export). */
    function redraw() {
      cancelAnimationFrame(raf);
      raf = 0;
      step(performance.now());
    }
    shared.wake = wake;
    wake();
    return () => {
      cancelAnimationFrame(raf);
      observer.disconnect();
      pointer?.stop();
      presence.stop();
      surface?.removeEventListener("chart-optical-status", onOptical);
      glass?.destroy();
      clearSvg();
      shared.wake = () => {};
    };
    // The bleed is fixed for the canvas's life; new glass (a pack arriving) starts it afresh.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [makeGlass]);

  /** The segment (by mark index) under a plot point in the current frame, or null. Flat, it is
   * the annulus and angle under the point; tilted, the slab whose top the ray meets nearest. */
  const pick = (x: number, y: number, slack = 8): number | null => {
    const frame = last.current;
    if (!frame) return null;
    const { ring, segments } = frame;
    if (frame.camera <= 0) {
      for (const segment of segments) {
        if (segment.end - segment.start <= 1e-6) continue;
        const mid = (segment.start + segment.end) / 2;
        const shift = segment.pop * 0.3;
        const cx = ring.cx + Math.sin(mid) * shift,
          cy = ring.cy - Math.cos(mid) * shift;
        const r = Math.hypot(x - cx, y - cy);
        if (Math.abs(r - segment.radius) > segment.width / 2 + slack) continue;
        if (normAngle(angleOf(cx, cy, x, y) - segment.start) < segment.end - segment.start)
          return segment.index;
      }
      return null;
    }
    // Tilted, a ray leaves the camera through the point and meets each slab's top, or its outer
    // or inner wall; the nearest hit names the slab.
    const view = frame.view;
    const sx = x + origin.current.x - view.anchor.x,
      sy = y + origin.current.y - view.anchor.y;
    const eye = view.position;
    const dir = [0, 1, 2].map(
      (k) => view.forward[k] * view.focal + view.right[k] * sx - view.up[k] * sy,
    );
    let best: number | null = null,
      nearest = Infinity;
    for (const segment of segments) {
      if (segment.end - segment.start <= 1e-6) continue;
      const mid = (segment.start + segment.end) / 2;
      const cx = ring.cx + origin.current.x + Math.sin(mid) * segment.pop * 0.3,
        cy = ring.cy + origin.current.y - Math.cos(mid) * segment.pop * 0.3;
      const bottom = frame.floor + segment.pop,
        top = bottom + segment.height;
      const within = (t: number, onTop: boolean) => {
        if (t <= 0 || t >= nearest) return false;
        const px = eye[0] + dir[0] * t,
          py = eye[1] + dir[1] * t,
          pz = eye[2] + dir[2] * t;
        if (!onTop && (pz < bottom - 1 || pz > top + 1)) return false;
        const r = Math.hypot(px - cx, py - cy);
        if (Math.abs(r - segment.radius) > segment.width / 2 + (onTop ? slack : 1)) return false;
        return normAngle(angleOf(cx, cy, px, py) - segment.start) < segment.end - segment.start;
      };
      const hits: [number, boolean][] = [];
      if (Math.abs(dir[2]) > 1e-9) hits.push([(top - eye[2]) / dir[2], true]);
      // Walls: where the ray meets the cylinders through the slab's outer and inner edges.
      const a = dir[0] * dir[0] + dir[1] * dir[1];
      const ox = eye[0] - cx,
        oy = eye[1] - cy;
      for (const radius of [
        segment.radius + segment.width / 2,
        segment.radius - segment.width / 2,
      ]) {
        if (a < 1e-12 || radius <= 0) continue;
        const b = 2 * (ox * dir[0] + oy * dir[1]),
          c = ox * ox + oy * oy - radius * radius;
        const disc = b * b - 4 * a * c;
        if (disc < 0) continue;
        const root = Math.sqrt(disc);
        hits.push([(-b - root) / (2 * a), false], [(-b + root) / (2 * a), false]);
      }
      for (const [t, onTop] of hits)
        if (within(t, onTop)) {
          nearest = t;
          best = segment.index;
        }
    }
    return best;
  };

  return { box, canvas, svg, shade, available, pick, frame: last };
}
