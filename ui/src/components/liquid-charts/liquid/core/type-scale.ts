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
 * How large a chart sets its type, as a multiple of the web sizes in `type.module.css`. On a page
 * the roles are sized for a dashboard (labels at 13 px); a chart that becomes a picture on a slide
 * or a poster needs them a step larger, and so does every estimate that fits labels before they are
 * measured (`TYPE` in format.ts). Both read this one number: the CSS through `--lc-type-scale`, the
 * estimates through `typeScale()`.
 *
 * Set it before a chart mounts, and set `--lc-type-scale` to the same value on an element around
 * the chart: the estimates and the drawn text then agree.
 */

/** The most a host may scale chart type. */
export const MAX_TYPE_SCALE = 2;

let scale = 1;

/** The current type scale: 1 on a page, larger for presentation pictures. */
export function typeScale(): number {
  return scale;
}

/** Set chart type `next` times the web sizes until the returned function is called. Nested calls
 * restore in order. A scale outside (0, `MAX_TYPE_SCALE`] is refused, not clamped. */
export function setTypeScale(next: number): () => void {
  if (!(next > 0 && next <= MAX_TYPE_SCALE)) {
    throw new RangeError(`Type scale must be above 0 and at most ${MAX_TYPE_SCALE}, not ${next}.`);
  }
  const previous = scale;
  scale = next;
  return () => {
    scale = previous;
  };
}
