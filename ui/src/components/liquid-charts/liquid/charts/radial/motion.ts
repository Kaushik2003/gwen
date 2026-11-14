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
/**
 * Motion for radial charts: named springs and eases stepped once per animation frame, with the
 * stiffness every glass mark in the library uses (110, damping 21). A chart asks for the value it
 * wants under a key; the store answers with where that value is this frame and remembers whether
 * anything still moves, so the frame loop can stop. Keys a frame does not ask for are forgotten.
 * Reduced motion answers every target at once. Plain class, no DOM, unit-tested.
 */
export class Motion {
  private springs = new Map<string, { value: number; velocity: number }>();
  private eases = new Map<string, number>();
  private touched = new Set<string>();
  /** True when anything asked for this frame has not yet arrived. */
  moving = false;
  dt = 1 / 60;
  reduced = false;

  /** Start a frame: `dt` seconds since the last one. */
  begin(dt: number, reduced: boolean) {
    this.dt = Math.max(1 / 240, Math.min(0.05, dt));
    this.reduced = reduced;
    this.moving = false;
    this.touched.clear();
  }

  /** End a frame: forget keys nobody asked for. */
  end() {
    for (const key of this.springs.keys()) if (!this.touched.has(key)) this.springs.delete(key);
    for (const key of this.eases.keys()) if (!this.touched.has(key)) this.eases.delete(key);
  }

  /** A spring towards `target`. The first ask starts at `from` (default: the target itself). A
   * non-finite target snaps. `rest` is the distance that counts as arrived. */
  spring(
    key: string,
    target: number,
    { from, rest = 0.01 }: { from?: number; rest?: number } = {},
  ) {
    this.touched.add(key);
    let s = this.springs.get(key);
    if (!s) {
      s = { value: from ?? target, velocity: 0 };
      this.springs.set(key, s);
    }
    if (this.reduced || !Number.isFinite(target) || !Number.isFinite(s.value)) {
      s.value = target;
      s.velocity = 0;
      return target;
    }
    const steps = 4,
      h = this.dt / steps;
    for (let k = 0; k < steps; k++) {
      s.velocity += (110 * (target - s.value) - 21 * s.velocity) * h;
      s.value += s.velocity * h;
    }
    if (Math.abs(target - s.value) < rest && Math.abs(s.velocity) < rest * 10) {
      s.value = target;
      s.velocity = 0;
    } else this.moving = true;
    return s.value;
  }

  /** An exponential ease towards `target` at `rate` per second (11 is the glass's tone fade). */
  ease(
    key: string,
    target: number,
    { rate = 11, from, rest = 0.004 }: { rate?: number; from?: number; rest?: number } = {},
  ) {
    this.touched.add(key);
    let value = this.eases.get(key) ?? from ?? target;
    if (this.reduced || !Number.isFinite(target)) value = target;
    else {
      value += (target - value) * (1 - Math.exp(-rate * this.dt));
      if (Math.abs(target - value) <= rest) value = target;
      else this.moving = true;
    }
    this.eases.set(key, value);
    return value;
  }

  /** Forget everything (a replay starts from scratch). */
  reset() {
    this.springs.clear();
    this.eases.clear();
  }
}

/** Smoothstep, clamped. */
export const easeInOut = (t: number) => {
  const u = Math.max(0, Math.min(1, t));
  return u * u * (3 - 2 * u);
};
