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
/** Plot-space geometry shared by the core. All values are CSS px, y pointing down. */

export type Rect = { left: number; top: number; right: number; bottom: number };
export type Insets = { left: number; top: number; right: number; bottom: number };
export type Size = { width: number; height: number };

export const rectWidth = (r: Rect) => r.right - r.left;
export const rectHeight = (r: Rect) => r.bottom - r.top;

/** True when two rects overlap once each is grown by `gap`. Touching edges do not overlap. */
export function overlaps(a: Rect, b: Rect, gap = 0): boolean {
  return (
    a.left < b.right + gap &&
    b.left < a.right + gap &&
    a.top < b.bottom + gap &&
    b.top < a.bottom + gap
  );
}

/** The plot rect inside a chart of `size` with `insets` reserved for axes and labels. */
export function plotRect(size: Size, insets: Insets): Rect {
  return {
    left: insets.left,
    top: insets.top,
    right: Math.max(insets.left, size.width - insets.right),
    bottom: Math.max(insets.top, size.height - insets.bottom),
  };
}

/** Rounds to 1/100 px so server and client layouts agree. */
export const settle = (value: number) => Math.round(value * 100) / 100;

/** `value` held within `[lo, hi]`. */
export const clamp = (value: number, lo: number, hi: number) => Math.max(lo, Math.min(hi, value));
/** `t` held within `[0, 1]`. */
export const clamp01 = (t: number) => (t <= 0 ? 0 : t >= 1 ? 1 : t);
/** A cubic ease-out over `t` in `[0, 1]` (held there): the entrances' curve. */
export const easeOut = (t: number) => 1 - (1 - clamp01(t)) ** 3;
/** A real number: not NaN, not infinite, not missing. */
export const finite = (value: unknown): value is number =>
  typeof value === "number" && Number.isFinite(value);
/** Does nothing: a handler a chart passes where it has nothing to do. */
export const noop = () => {};

export type BarDirection = "up" | "down" | "left" | "right";

/** A bar's rect at a given length (its current, eased length while it springs). */
export function barRect(
  bar: { cross: number; base: number; thickness: number; direction: BarDirection },
  length: number,
): Rect {
  const half = bar.thickness / 2;
  const l = Math.max(0, length);
  switch (bar.direction) {
    case "up":
      return {
        left: bar.cross - half,
        right: bar.cross + half,
        top: bar.base - l,
        bottom: bar.base,
      };
    case "down":
      return {
        left: bar.cross - half,
        right: bar.cross + half,
        top: bar.base,
        bottom: bar.base + l,
      };
    case "right":
      return {
        left: bar.base,
        right: bar.base + l,
        top: bar.cross - half,
        bottom: bar.cross + half,
      };
    case "left":
      return {
        left: bar.base - l,
        right: bar.base,
        top: bar.cross - half,
        bottom: bar.cross + half,
      };
  }
}
