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
import type { GlassCamera, GlassView } from "./camera";
import type { MarkLiquid } from "./materials";
import type { Rgb, Theme } from "./colour";
import type { View } from "./style";

/* The ring marks as data: what a chart lays out, what a ring layer reports each frame, and the
 * contract a glass painter (the materials, `liquid-glass`) fills. Apart from the ring hook, so the core and
 * the glass can name them without carrying it. */

/** Where one ring segment should be, in data terms. Charts lay these out for the plot's current
 * size. Angles are clockwise radians from 12 o'clock. */
export type RingMark = {
  key: string;
  start: number;
  end: number;
  tint: Rgb;
  /** 1 lit in its tint, 0 smoked graphite (out of focus). */
  lit: number;
  /** Px the segment lifts out of the ring (it moves outward by 0.3 of this, as the glass does). */
  pop?: number;
  /** Px the slab stands above the floor in the tilt view; defaults to `tilt.low`. */
  height?: number;
  /** Seconds after the entrance sweep reaches this segment that it fills. */
  delay?: number;
  /** How full the glass is, 0–1 (default 1): below 1 it is a shell holding liquid to that share,
   * flowing clockwise from its start. Changes ease. */
  level?: number;
  /** Concentric rings: this mark's own middle radius and thickness. One raymarched canvas draws
   * every track (up to `CONCENTRIC_LIMIT` marks), with true depth between them when tilted. */
  radius?: number;
  width?: number;
  /** Concentric rings: this mark's radius and thickness in the tilted view. Defaults to its flat
   * ones scaled as the ring's are (`tilt.radius / ring.radius`, `tilt.width / ring.width`). */
  tilted?: { radius?: number; width?: number };
  /** Continuous rings only: where a mark that arrives after the entrance starts from. It springs
   * from here to its place while it sweeps in (a part sliding in from the hole on a drill). */
  from?: { start?: number; end?: number; radius?: number; width?: number };
  /** Round the ends fully (progress rings) instead of bevelling them. */
  round?: boolean;
  /** Etch the glass with the pattern that means "beyond a limit" (style rule 10): `true` for the
   * whole mark, or the share of it (from its start) where the pattern begins. Optical and flat
   * engines; the raymarched ring has no pattern. */
  pattern?: boolean | number;
  /** Cast a soft shadow from the mark's end onto whatever lies under it, so a lap riding on top
   * of another reads as lying on it. Needs the `shade` layer. */
  shadow?: boolean;
};

/** The ring every mark sits on, in plot px. */
export type RingShape = {
  cx: number;
  cy: number;
  /** The ring's middle radius and its width. */
  radius: number;
  width: number;
  /** Px between segments, at the middle radius. */
  gap?: number;
  bevel?: number;
  /** Glass depth; defaults to 0.7 × width. */
  depth?: number;
};

/** The same ring seen tilted: its size, slab depth and the camera. Slabs stand from `depth`
 * (flat) to their `height` as the view turns. */
export type RingTilt = {
  radius: number;
  width: number;
  depth: number;
  camera: GlassCamera;
  /** Default slab height when a mark has none. */
  low: number;
  gap?: number;
  bevel?: number;
};

export type RingLayout = {
  ring: RingShape;
  marks: RingMark[];
  tilt?: RingTilt;
  /** The view to move towards. Tilt needs `tilt` and a single raymarched ring. */
  view?: View;
  /** Radians the ring turns while tilted, to bring a slab to the back. */
  turn?: number;
  /** Where the entrance sweep starts and how far it runs (default: the marks' extent). */
  sweep?: { from: number; span: number };
  /** Keep the ring continuous while its marks change (a drill, a filter): each mark's radius
   * and width spring to new values as its angles do (a resize still snaps), a mark that arrives
   * after the entrance sweeps in on its own (from `from`, after its `delay`) instead of the whole
   * ring replaying, and a mark that leaves retracts to its start before it goes. Off by default:
   * new marks appear at once and leaving ones vanish. */
  continuous?: boolean;
};

/** A segment as drawn this frame. `start` and `end` include the ring's turn. */
export type RingSegmentFrame = RingMark & {
  index: number;
  pop: number;
  height: number;
  tone: number;
  /** Entrance fill × level, 0–1. */
  fill: number;
  radius: number;
  width: number;
};

export type RingFrame = {
  width: number;
  height: number;
  /** The ring now: between its flat and tilted geometry while the view turns. */
  ring: { cx: number; cy: number; radius: number; width: number; depth: number; gap: number };
  segments: RingSegmentFrame[];
  /** 0 flat … 1 tilted; the camera move takes its first 60%. */
  phase: number;
  /** The eased camera, 0 straight on … 1 fully tilted. */
  camera: number;
  turn: number;
  /** The floor's z; a slab's top is at floor + height. */
  floor: number;
  view: GlassView;
  engine: RingEngine;
  /** Screen position (plot px) of a world point. Flat, it is the point itself. */
  project: (x: number, y: number, z?: number) => { x: number; y: number };
  /** Screen position of the point at `angle` and `radius` from the ring's centre, at height z
   * above the floor (default 0, on it). */
  at: (angle: number, radius: number, z?: number) => { x: number; y: number };
};

export type RingEngine = "raymarch" | "optical" | "svg";

/** What a glass painter is handed each frame: the ring now and its segments, in plot px, with
 * where plot (0, 0) sits on the canvas. */
export type RingScene = {
  engine: Exclude<RingEngine, "svg">;
  theme: Theme;
  /** The canvas, px. */
  size: { width: number; height: number };
  offset: { x: number; y: number };
  ring: {
    cx: number;
    cy: number;
    radius: number;
    width: number;
    depth: number;
    gap: number;
    bevel: number;
  };
  /** The tilted camera, in plot px; undefined straight on. */
  camera?: GlassCamera;
  /** Each segment on its own track (concentric or continuous rings). */
  ownTracks: boolean;
  segments: readonly (RingSegmentFrame & { field?: Float32Array })[];
};

/** Glass for ring marks (the materials, `liquid-glass`): the raymarched ring, which tilts, and the optical
 * painter, with the simulated liquid inside. A material pack's `ring` makes one per canvas. */
export type RingGlass = {
  resize(width: number, height: number): void;
  /** "lost": the context is lost for now (keep the last frame); "unavailable": no glass on this
   * device (draw flat from now on). */
  paint(scene: RingScene): "drawn" | "lost" | "unavailable";
  /** Something else drew on the canvas: don't skip the next frame as already painted. */
  invalidate(): void;
  destroy(): void;
  /** A segment's liquid, and the floats of surface per segment the painter reads. */
  liquid(seed: number): MarkLiquid;
  fieldLength: number;
};
