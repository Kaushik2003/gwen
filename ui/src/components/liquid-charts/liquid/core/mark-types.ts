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
import type { Rgb } from "./colour";

/* The marks every bar-like and bead-like layer draws, as data: what a chart lays out and what a
 * layer reports back each frame. Flat and glass layers share them, so they live apart from either
 * renderer. */

/** Which way a bar grows from its base. */
export type Direction = "up" | "down" | "right" | "left";

/** A convex host shell in plot CSS pixels, in boundary order. Adjacent duplicate points
 * represent a triangular end. Omitted shells retain the legacy rectangular solid. */
export type BarShell = readonly [
  Readonly<{ x: number; y: number }>,
  Readonly<{ x: number; y: number }>,
  Readonly<{ x: number; y: number }>,
  Readonly<{ x: number; y: number }>,
];
export type BarPaintOutcome = {
  key: string;
  revision: number;
  status: "native" | "host";
  reason?: string;
};

/** Where one bar should be. Charts lay these out for the plot's current size, in plot px. */
export type BarMark = {
  key: string;
  /** Actual optional sloped shell, never an enclosing rectangle or clipping mask. */
  shell?: BarShell;
  /** Caller revision returned by onPaint; ordinary native charts may omit it. */
  revision?: number;
  /** Centre across the bar's axis, and the baseline it grows from. */
  cross: number;
  base: number;
  thickness: number;
  length: number;
  direction: Direction;
  tint: Rgb;
  /** 1 lit in its tint, 0 smoked graphite (out of focus, or a comparison). */
  lit: number;
  /** Seconds after the entrance starts that this bar begins to fill. */
  delay?: number;
  /** How full the glass is, 0–1 (default 1). Below 1 the bar is a container holding liquid to
   * that level: a progress capsule or a capacity bar. Changes ease. */
  level?: number;
  /** Etch a pattern into the glass: `true` for the whole bar, or the share of its length (0–1,
   * from the base) where the pattern begins. For a different state of the same quantity:
   * projected, estimated or incomplete periods, or the stretch beyond a limit. */
  pattern?: boolean | number;
  /** Engraved reference marks (a target, a pace, a plan): up to four shares of the bar's length,
   * from its base. Reference marks live in the glass, never on top of it. A share plus 2 is an
   * axis graduation tick, from the bar's near rim; plus 4, a graduation ring across it. Both are
   * cut quieter than reference marks. */
  marks?: readonly number[];
  /** Where a bar new to the layout starts before it springs to its place: a drill zooming a tile
   * out of its parent, a node sliding in. Without it a new bar appears in place and fills. */
  from?: Partial<Pick<BarMark, "cross" | "base" | "thickness" | "length">>;
};
/** A bar as drawn this frame: its eased length, tone and entrance fill. */
export type BarFrame = BarMark & { current: number; tone: number; fill: number };

/** Where one bead should be. Charts lay these out for the plot's current size, in plot px. */
export type DotMark = {
  key: string;
  /** Centre and radius. */
  x: number;
  y: number;
  r: number;
  tint: Rgb;
  /** 1 lit in its tint, 0 smoked graphite (out of focus, or a comparison). */
  lit: number;
  /** Seconds after the entrance starts that this bead begins to fill. */
  delay?: number;
  /** How full the bead is, 0–1 (default 1). Below 1 it holds liquid to that level. */
  level?: number;
  /** Etch a pattern into the glass, for another state of the same quantity (projected,
   * estimated, beyond a limit): `true` for the whole bead, or the share (from the bottom) where
   * it begins. */
  pattern?: boolean | number;
  /** Where a bead new to the canvas starts, so it springs out from there (children out of the
   * parent that opened, beads in from where they were). Default: its own place. */
  from?: { x: number; y: number; r?: number };
  /** Follow the target exactly instead of springing to it (a node under the finger, a pan, a
   * force layout playing back). The radius still springs. */
  immediate?: boolean;
  /** Tracked and sprung, and in `onFrame`, but not drawn (joints that links meet). */
  hidden?: boolean;
};
/** A bead as drawn this frame: its sprung centre and radius, tone and entrance fill. */
export type DotFrame = DotMark & {
  cx: number;
  cy: number;
  radius: number;
  tone: number;
  fill: number;
  /** Entrance, 0–1 (always 1 for the `pour` entrance, whose beads stand empty and fill). */
  shown: number;
};
