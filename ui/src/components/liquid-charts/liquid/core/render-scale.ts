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
 * How many device pixels Liquid's canvases draw per CSS pixel. On screen it is the display's own
 * ratio, capped where a material would cost too much to draw; while an export scale is set it is
 * that scale, so a picture drawn for a larger output gets more material detail instead of a
 * stretched screen draw, and a standard-density screen still exports at the scale asked for.
 *
 * Canvases read it when they size themselves (on mount and on resize), so set the scale before a
 * chart mounts for the whole draw to use it.
 */

/** The most device px per CSS px an export may ask for. */
export const MAX_RENDER_SCALE = 4;

/**
 * The largest picture Liquid asks a browser to draw. Past these, browsers hand back a blank or
 * clamped canvas rather than an error: Safari on iPhone and iPad (where Canva also runs) refuses
 * any canvas over 4096 × 4096 px, and many GPUs won't draw a WebGL buffer wider than 8192 px. At
 * the area limit one canvas is 64 MB, and an export holds several.
 */
export const MAX_PICTURE_SIDE = 8192;
export const MAX_PICTURE_AREA = 4096 * 4096;

let exportScale: number | null = null;

/** Device px per CSS px for a canvas: the export scale while one is set, else the screen's ratio,
 * at most `cap`. */
export function pixelRatio(cap = Infinity): number {
  if (exportScale !== null) return exportScale;
  const screen = typeof window === "undefined" ? 1 : window.devicePixelRatio || 1;
  return Math.min(cap, screen);
}

/** Draw every canvas at `scale` device px per CSS px until the returned function is called. Nested
 * calls restore in order. A scale past `MAX_RENDER_SCALE` is refused, not clamped. */
export function setRenderScale(scale: number): () => void {
  if (!(scale > 0 && scale <= MAX_RENDER_SCALE)) {
    throw new RangeError(`Render scale must be above 0 and at most ${MAX_RENDER_SCALE}, not ${scale}.`);
  }
  const previous = exportScale;
  exportScale = scale;
  let restored = false;
  return () => {
    if (restored) return;
    restored = true;
    exportScale = previous;
  };
}

/** The largest scale up to `scale` at which a `width` × `height` CSS px picture stays within the
 * picture limits. Below `scale` only for pictures too big to draw at it. */
export function fitScale(width: number, height: number, scale: number): number {
  const side = MAX_PICTURE_SIDE / Math.max(1, width, height);
  const area = Math.sqrt(MAX_PICTURE_AREA / Math.max(1, width * height));
  return Math.min(scale, side, area);
}
