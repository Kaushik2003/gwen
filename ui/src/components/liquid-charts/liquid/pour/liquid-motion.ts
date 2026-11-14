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
import { clamp } from "@/components/liquid-charts/liquid/core/geometry";
/**
 * The liquid's motion, generalised from the approved area (presentation/area-focus): a pour that
 * fills a data-shaped mould from the left, a pinned spring field that ripples towards the pointer
 * and rings like liquid, and springs that carry the surface between data. Plain classes with no
 * DOM, stepped by the chart's animation loop and unit-tested on their own. None of them runs when
 * the reader prefers reduced motion: the chart settles them instead.
 */

const smooth = (x: number) => {
  const t = clamp(x, 0, 1);
  return t * t * (3 - 2 * t);
};

/* ——— The pour ——— */

const STEP = 1 / 240;
// Subtracting browser timestamps can leave an exact step a fraction of a nanosecond short.
// Absorb only that arithmetic residue so the same elapsed frames never lose a physics step.
const STEP_ROUNDING = 1e-9;
const PROFILES = {
  pour: { speed: 1.25, slowdown: 0.45, climb: 0.26 },
  gentle: { speed: 0.85, slowdown: 0.4, climb: 0.34 },
} as const;
/** Leading-edge length, as a share of the plot width, stretching with speed. */
const NOSE_BASE = 0.035,
  NOSE_PER_SPEED = 0.06,
  NOSE_MAX = 0.14;
const RUSH_SECONDS = 0.2,
  RUSH_CLIMB = 0.12;

/**
 * The entrance: liquid fills the mould from the left. `front` (0–1 of the plot width) is where the
 * leading edge meets the floor; behind it, one `nose` length back, the surface is exactly the data
 * line, so the reveal never shows a value the data does not have. The pour slows as it spreads,
 * then its edge climbs the right wall until the body meets the mould everywhere. Inspecting or new
 * data hurries it without cutting to the last frame.
 */
export class Pour {
  front = 0;
  speed = 0;
  nose = NOSE_BASE;
  active = true;
  rushing = false;
  private wallAge = 0;
  private arrivalNose = 0;
  private climbFrom = 0;
  private rushSpeed = 0;
  private accumulator = 0;

  /** Monotonic 0–1 progress across the travel and the final climb. */
  get progress() {
    if (!this.active) return 1;
    const climb = this.arrivalNose > 0 ? clamp(1 - this.nose / this.arrivalNose, 0, 1) : 0;
    return this.front * 0.85 + climb * 0.15;
  }

  /** Share of the width behind which the surface is exactly the data. */
  get settled() {
    return this.active ? Math.max(0, this.front - this.nose) : 1;
  }

  rush() {
    if (!this.active || this.rushing) return;
    this.rushing = true;
    this.rushSpeed = Math.max(this.speed, (1 - this.front) / RUSH_SECONDS);
    if (this.front === 1) {
      this.climbFrom = this.nose;
      this.wallAge = 0;
    }
  }

  finish() {
    this.active = false;
    this.front = 1;
    this.speed = 0;
    this.nose = 0;
  }

  /** Advances by `dt` seconds in fixed sub-steps; returns whether it is still pouring. */
  step(dt: number, gentle = false) {
    if (!this.active) return false;
    const profile = gentle ? PROFILES.gentle : PROFILES.pour;
    this.accumulator += Math.min(Math.max(0, dt), 1 / 30);
    while (this.accumulator + STEP_ROUNDING >= STEP && this.active) {
      this.accumulator = Math.max(0, this.accumulator - STEP);
      if (this.front < 1) {
        let target = profile.speed * (1 - profile.slowdown * this.front);
        if (this.rushing) target = Math.max(target, this.rushSpeed);
        this.speed += (target - this.speed) * (1 - Math.exp(-(this.rushing ? 30 : 14) * STEP));
        this.front = Math.min(1, this.front + this.speed * STEP);
        const nose = Math.min(NOSE_MAX, NOSE_BASE + NOSE_PER_SPEED * this.speed);
        // A rushed edge may not stretch, or settled data would slide back under it.
        this.nose = this.rushing ? Math.min(this.nose, nose) : nose;
        if (this.front === 1) this.arrivalNose = this.climbFrom = this.nose;
      } else {
        this.speed = 0;
        this.wallAge += STEP;
        const climb = this.rushing ? RUSH_CLIMB : profile.climb;
        this.nose = this.climbFrom * (1 - smooth(this.wallAge / climb));
        if (this.wallAge >= climb) this.finish();
      }
    }
    return this.active;
  }
}

/* ——— The ripple ——— */

/**
 * The spring field runs in the approved chart's model space, 342 units wide and 192 tall, with its
 * 73 nodes, so the tuning carries over to any size: the chart maps x and y in and offsets out.
 */
export const MODEL_WIDTH = 342;
export const MODEL_HEIGHT = 192;
export const RIPPLE_NODES = 73;
/** The furthest the surface may move, model units (about 4% of the plot's height). */
export const MAX_DISPLACEMENT = 8;
/** Ripple amplitude per model unit/ms of pointer speed. A resting pointer adds none. */
const RIPPLE_GAIN = 9;
// A light pull back to the data with strong neighbour coupling and low damping lets disturbances
// travel, reflect off the pinned points and ring like liquid.
const RESTORE = 70,
  COUPLING = 230,
  DAMPING = 10;

/**
 * Which nodes are held still: the ones nearest each data point while points are well apart (at
 * least eight nodes, as in the approved seven-day chart, so a ripple reflects off them and the
 * surface always passes through the data), else only the two walls, so denser data still ripples.
 */
export function ripplePins(points: readonly number[], nodes = RIPPLE_NODES): boolean[] {
  const pins = new Array<boolean>(nodes).fill(false);
  pins[0] = pins[nodes - 1] = true;
  const sorted = points.filter((x) => Number.isFinite(x)).sort((a, b) => a - b);
  let sparse = sorted.length > 1;
  for (let i = 1; i < sorted.length; i++)
    if ((sorted[i] - sorted[i - 1]) * (nodes - 1) < 8) sparse = false;
  if (sparse) for (const x of sorted) pins[Math.round(clamp(x, 0, 1) * (nodes - 1))] = true;
  return pins;
}

export class Ripple {
  offsets: number[];
  velocity: number[];
  pins: boolean[];
  private pointer: { x: number; y: number; speed: number; age: number } | null = null;

  constructor(pins: readonly boolean[] = ripplePins([])) {
    this.pins = [...pins];
    this.offsets = new Array<number>(this.pins.length).fill(0);
    this.velocity = new Array<number>(this.pins.length).fill(0);
  }

  get nodes() {
    return this.pins.length;
  }

  setPins(pins: readonly boolean[]) {
    if (pins.length !== this.pins.length) {
      this.offsets = new Array<number>(pins.length).fill(0);
      this.velocity = new Array<number>(pins.length).fill(0);
    }
    this.pins = [...pins];
  }

  /** The pointer at model (x, y), moving at `speed` model units per ms. */
  inspect(x: number, y: number, speed: number) {
    this.pointer = { x, y, speed: clamp(speed, 0, 2.5), age: 0 };
  }

  release() {
    this.pointer = null;
  }

  /** A gentle slosh across the whole surface (when the data settles after a change). */
  kick(amount: number) {
    const n = this.nodes;
    for (let i = 0; i < n; i++)
      if (!this.pins[i]) this.velocity[i] += amount * Math.sin(((i % 12) / 12) * Math.PI) * 12;
  }

  settle() {
    this.offsets.fill(0);
    this.velocity.fill(0);
    this.pointer = null;
  }

  /**
   * Advances by `dt` seconds. `surface(i)` is the resting surface's model y at node i (so the
   * pull fades with the pointer's distance above or below it). Returns whether anything moves.
   */
  step(dt: number, surface: (node: number) => number) {
    dt = clamp(dt, 1 / 240, 1 / 30);
    const n = this.nodes;
    if (this.pointer) this.pointer.age += dt;
    const ripple = this.pointer
      ? clamp(RIPPLE_GAIN * this.pointer.speed * Math.exp(-this.pointer.age * 7), 0, 9)
      : 0;
    const forcing = ripple > 0.05;
    let moving = forcing;
    const steps = 4,
      h = dt / steps;
    const forces = new Array<number>(n).fill(0);
    for (let sub = 0; sub < steps; sub++) {
      for (let i = 0; i < n; i++) {
        if (this.pins[i]) {
          forces[i] = 0;
          continue;
        }
        let target = 0;
        if (forcing && this.pointer) {
          const x = (i / (n - 1)) * MODEL_WIDTH;
          const distance = (x - this.pointer.x) / 26;
          const proximity = Math.exp(-(((this.pointer.y - surface(i)) / 120) ** 2));
          target = ripple * (1 - 2 * distance ** 2) * Math.exp(-(distance ** 2)) * proximity;
        }
        const left = this.offsets[Math.max(0, i - 1)],
          right = this.offsets[Math.min(n - 1, i + 1)];
        forces[i] =
          RESTORE * (target - this.offsets[i]) +
          COUPLING * (left + right - 2 * this.offsets[i]) -
          DAMPING * this.velocity[i];
      }
      for (let i = 0; i < n; i++) {
        if (this.pins[i]) {
          this.offsets[i] = 0;
          this.velocity[i] = 0;
          continue;
        }
        this.velocity[i] += forces[i] * h;
        this.offsets[i] = clamp(
          this.offsets[i] + this.velocity[i] * h,
          -MAX_DISPLACEMENT,
          MAX_DISPLACEMENT,
        );
      }
    }
    for (let i = 0; i < n; i++) {
      if (!forcing && Math.abs(this.offsets[i]) < 0.002 && Math.abs(this.velocity[i]) < 0.015) {
        this.offsets[i] = 0;
        this.velocity[i] = 0;
      }
      if (Math.abs(this.velocity[i]) > 0.015 || this.offsets[i] !== 0) moving = true;
    }
    if (!forcing && this.pointer && this.pointer.age > 1) this.pointer = null;
    return moving;
  }

  /** The offset at `t` (0–1 across the plot), model units, linearly between nodes. */
  offsetAt(t: number) {
    const n = this.nodes;
    const s = clamp(t, 0, 1) * (n - 1);
    const i = Math.min(n - 2, Math.floor(s));
    const f = s - i;
    return this.offsets[i] * (1 - f) + this.offsets[i + 1] * f;
  }

  get still() {
    return this.offsets.every((offset) => offset === 0) && !this.pointer;
  }
}

/* ——— Springs between data ——— */

/**
 * Springs that carry a grid of values (`[layer][point]`) to new data; the same stiffness the bars
 * use. A change of shape (points or layers added or removed) snaps, since there is nothing to
 * spring from. NaN (a gap) snaps too.
 */
export class Springs {
  values: number[][] = [];
  target: number[][] = [];
  private velocity: number[][] = [];

  set(next: readonly (readonly number[])[]) {
    const reshaped =
      next.length !== this.values.length ||
      next.some((row, k) => row.length !== this.values[k].length);
    this.target = next.map((row) => [...row]);
    if (reshaped) {
      this.values = next.map((row) => [...row]);
      this.velocity = next.map((row) => row.map(() => 0));
      return;
    }
    next.forEach((row, k) =>
      row.forEach((v, i) => {
        if (!Number.isFinite(v) || !Number.isFinite(this.values[k][i])) {
          this.values[k][i] = v;
          this.velocity[k][i] = 0;
        }
      }),
    );
  }

  settle() {
    this.values = this.target.map((row) => [...row]);
    this.velocity = this.target.map((row) => row.map(() => 0));
  }

  /** Advances by `dt` seconds; returns whether any value still moves. `scale` is the size of one
   * unit of change that counts as visible (the domain's span), so settling is scale-free. */
  step(dt: number, scale = 1) {
    dt = clamp(dt, 1 / 240, 1 / 30);
    const steps = 4,
      h = dt / steps;
    const rest = Math.abs(scale) * 1e-4 || 1e-4;
    let moving = false;
    this.values.forEach((row, k) =>
      row.forEach((value, i) => {
        const target = this.target[k][i];
        if (!Number.isFinite(target) || !Number.isFinite(value)) return;
        let v = value,
          velocity = this.velocity[k][i];
        for (let s = 0; s < steps; s++) {
          velocity += (110 * (target - v) - 21 * velocity) * h;
          v += velocity * h;
        }
        if (Math.abs(target - v) < rest && Math.abs(velocity) < rest * 10) {
          v = target;
          velocity = 0;
        } else moving = true;
        row[i] = v;
        this.velocity[k][i] = velocity;
      }),
    );
    return moving;
  }
}
