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
 * The liquid in a glass mark, simulated: a viscous, gooey liquid whose surface moves point by
 * point. Plain class, no DOM; the mark hooks step it each frame and hand `rows` to the shaders.
 *
 * - The level is a mass on a spring. It launches when the mark's delay passes, rises, lands and,
 *   in a full mark, hits the tip and sags back before settling.
 * - The surface is a height field over the liquid's top: NX cells across the mark (wall to wall)
 *   by NZ through its depth (front face to back), each with its own height and velocity, pulled
 *   towards its neighbours (the damped wave equation with velocity diffusion for viscosity, as in
 *   height-field water). Walls reflect. The field holds deviations from the level that always sum
 *   to zero, so at rest the level reads exactly the value.
 * - Liquid wells up unevenly: faster through the middle, where the walls drag least, and in slow
 *   drifting lumps. The unevenness enters as volume where it rises and the field carries it out.
 * - Cells that reach a full mark's tip splash back. The pointer drags the surface as it sweeps and
 *   a press (the pointer or keyboard focus arriving) sinks the column and drops a dip under it.
 * - When the liquid falls back it leaves a coat on the glass above it, which drains unevenly.
 *
 * Marks thinner than `FIELD_MIN` px keep the level's spring and coat but no field: there the
 * surface is too small to see.
 */

import { OPTICAL_SURFACE } from "@/components/liquid-charts/ui/chart-optical-shader";
import { clamp } from "@/components/liquid-charts/liquid/core/geometry";

const smooth = (x: number) => {
  const t = clamp(x, 0, 1);
  return t * t * (3 - 2 * t);
};

/** Cells across the mark and through its depth (the layout both renderers read). */
export const NX = OPTICAL_SURFACE.columns;
export const NZ = OPTICAL_SURFACE.depth;
/** Rows the shaders read per mark: NZ rows of heights, the coat, then (flow, moving, surface). */
export const FIELD_ROWS = OPTICAL_SURFACE.rows;
export const FIELD_LENGTH = NX * FIELD_ROWS;
/** Px the meniscus climbs the walls at rest. */
export const MENISCUS = OPTICAL_SURFACE.meniscus;
/** Marks thinner than this (px) have no simulated surface. */
export const FIELD_MIN = 10;

const LEVEL = { hz: 1.15, zeta: 0.55, launch: 0.7, restitution: 0.22 };
/** Wave speed (px/s), damping (1/s), viscosity (px²/s), how unevenly it enters (share of the
 * level's speed), how much of that is the viscous middle rather than lumps, how fast the lumps
 * change (Hz), and how much speed a cell keeps when it splashes off the tip. */
const FLUID = { speed: 55, damping: 2.4, viscosity: 26, uneven: 0.16, middle: 0.75, lumps: 0.9 };
const SPLASH = 0.15;
/** A sweep's drag (per px/s of the pointer; for marks wider than `DRAG_WIDTH` px it scales down,
 * so a wide pool ripples about as many px as a bar), a press's dip (px/s) and its sink (px/s). */
const DRAG = 0.5,
  DRAG_WIDTH = 48,
  DROP = 110,
  SINK = 150;
/** The coat's drain (px/s) and its most px, and how many times faster it drains once the pointer
 * that stirred it has left. */
const DRAIN = 14,
  COAT_MAX = 40,
  DRAIN_AWAY = 3;
/** Still to the eye: the surface's largest deviation (px) and speed (px/s) under which it rests.
 * Below a quarter pixel nothing shows, and chasing it kept loops running for seconds. */
const STILL = { peak: 0.25, energy: 2 };
/** Rest. Once nothing drives the surface (no push, no press, the level slower than `level` px/s)
 * for `after` s, ripples smaller than `visible` px relax away at up to `rate` per s: a wide pool's
 * longest slosh is overdamped and otherwise creeps on for ten seconds or more. Motion you can see
 * is left to the fluid. After an input, every tail fades out by `cap` s (at `fade` per s). */
const REST = { level: 25, after: 0.3, visible: 1, rate: 5, cap: 1.5, fade: 8 };
/** Share of the level's travel the inner cloud travels with it (returning as it lands). */
const FLOW = 0.5;

export type LiquidInput = {
  /** Where the level should be (px along the mark); 0 before the mark's delay. */
  target: number;
  /** Width across the mark and depth through it (px). */
  thickness: number;
  depth: number;
  /** The end of a full mark (px): the liquid splashes off it instead of running past. */
  ceiling?: number;
  /** The pointer near the mark: where across it (-1 to 1, wall to wall) and its speed across the
   * mark (px/s) times its proximity (0–1). */
  pointer?: { s: number; push: number } | null;
};

/** Seeded value noise in time, so the lumps drift and change smoothly. */
function noise(seed: number, t: number) {
  const i = Math.floor(t),
    f = t - i;
  const r = (n: number) => {
    const x = Math.sin(n * 127.1 + seed * 311.7) * 43758.5453;
    return x - Math.floor(x);
  };
  const u = f * f * (3 - 2 * f);
  return r(i) * (1 - u) + r(i + 1) * u;
}

export class LiquidField {
  /** The level (px along the mark) and its speed (px/s). */
  front = 0;
  velocity = 0;
  launched = false;
  /** Deviations from the level, [z * NX + x], px, and their vertical velocities. */
  readonly height = new Float32Array(NX * NZ);
  readonly speed = new Float32Array(NX * NZ);
  /** Px the coat reaches above the level, per column. */
  readonly wet = new Float32Array(NX);
  private clock = 0;
  private still = true;
  /** Seconds since anything drove the surface, and since the last input (a push or a press);
   * `capped` while that input is the latest disturbance (new data or a replay clears it). */
  private quiet = 0;
  private sinceInput = 0;
  private capped = false;
  private aim = 0;
  private scratch = new Float32Array(NX * NZ);
  private inflow = new Float32Array(NX);
  /** The hardest the level landed on the tip, and a cell splashed off it, since `takeImpact`. */
  private landed = 0;
  private splashed = 0;
  private impact = { land: 0, splash: 0 };
  /** The inflow's three lumps at the current substep: where across the mark, and how strong. */
  private centres = new Float64Array(3);
  private strengths = new Float64Array(3);
  /** How fast each column's coat drains, relative to the rest: fixed by the seed. */
  private readonly runs: Float64Array;

  constructor(private seed = 1) {
    this.runs = new Float64Array(NX);
    for (let x = 0; x < NX; x++) this.runs[x] = 0.6 + 0.8 * noise(seed + x * 1.37, x * 0.37);
  }

  /** Writes what the shaders read (FIELD_LENGTH floats) at `offset`. */
  rows(out: Float32Array, offset = 0, rest = this.front) {
    out.set(this.height, offset);
    out.set(this.wet, offset + NX * NZ);
    const misc = offset + NX * (NZ + 1);
    out[misc] = (this.front - rest) * FLOW;
    out[misc + 1] = this.still ? 0 : 1;
    // This mark has a liquid surface (its meniscus shows even at rest).
    out[misc + 2] = 1;
  }

  /** A fresh copy of `rows`, for renderers that keep one per mark. */
  field(rest = this.front) {
    const out = new Float32Array(FIELD_LENGTH);
    this.rows(out, 0, rest);
    return out;
  }

  /** Whether the surface is flat and the coat gone (the level may still move). */
  get flat() {
    return this.still;
  }

  reset() {
    this.front = this.velocity = 0;
    this.launched = false;
    this.quiet = this.sinceInput = this.aim = 0;
    this.capped = false;
    this.height.fill(0);
    this.speed.fill(0);
    this.wet.fill(0);
    this.still = true;
    this.landed = this.splashed = 0;
  }

  /** The hardest impacts (px/s) since the last call, and clears them: the level landing on a full
   * mark's tip, and the surface splashing off it. Taken after stepping, since a long frame steps
   * in parts; read it before the next call. */
  takeImpact(): Readonly<{ land: number; splash: number }> {
    // One object, reused: this runs for every mark on every frame.
    this.impact.land = this.landed;
    this.impact.splash = this.splashed;
    this.landed = this.splashed = 0;
    return this.impact;
  }

  /** Lands at the target with everything still (reduced motion, or a snap). */
  settle(target: number) {
    this.reset();
    this.front = target;
    this.launched = target > 0;
  }

  /** The pointer or keyboard focus arrives on the mark (s across it, 0 for the keyboard). */
  press(s = 0, thickness = FIELD_MIN) {
    this.quiet = this.sinceInput = 0;
    this.capped = true;
    if (this.launched) this.velocity -= SINK;
    if (thickness < FIELD_MIN) return;
    const at = ((clamp(s, -1, 1) + 1) / 2) * (NX - 1);
    for (let z = 0; z < NZ; z++)
      for (let x = 0; x < NX; x++)
        this.speed[z * NX + x] -= DROP * Math.exp(-(((x - at) / 2.2) ** 2));
    balance(this.speed);
    this.still = false;
  }

  /** Advances by `dt` seconds; returns whether anything still moves. A long frame is integrated in
   * steps of at most 50 ms (the substep limits below assume no more), so it plays at its real
   * speed instead of in slow motion. */
  step(dt: number, input: LiquidInput): boolean {
    if (dt > 0.05) {
      const n = Math.ceil(dt / 0.05);
      let moving = false;
      for (let k = 0; k < n; k++) moving = this.step(dt / n, input);
      return moving;
    }
    const { target, thickness, depth, ceiling, pointer } = input;
    if (dt <= 0) return !this.still || this.velocity !== 0;
    const w = 2 * Math.PI * LEVEL.hz;
    if (!this.launched && target > 0) {
      this.launched = true;
      this.velocity = Math.max(this.velocity, LEVEL.launch * w * (target - this.front));
    }
    const simulated = thickness >= FIELD_MIN;
    const dx = Math.max(thickness / NX, 0.5),
      dz = Math.max(depth / NZ, 0.5);
    const k = 1 / (dx * dx) + 1 / (dz * dz);
    // Substeps keep the wave equation and the diffusion stable at this cell size.
    const limit = Math.min(0.45 / (FLUID.speed * Math.sqrt(k)), 0.2 / (FLUID.viscosity * k));
    const steps = simulated ? Math.min(24, Math.max(2, Math.ceil(dt / limit))) : 2;
    const h = dt / steps;
    const c2 = FLUID.speed * FLUID.speed;
    const push = simulated ? (pointer?.push ?? 0) : 0;
    const pointerAt = pointer ? ((clamp(pointer.s, -1, 1) + 1) / 2) * (NX - 1) : 0;
    // The drag ripples a wide pool about as many px as a bar: its cells are wider, so the same
    // push moves more liquid.
    const reach = Math.min(1, DRAG_WIDTH / Math.max(thickness, 1));
    const { height, speed, scratch, inflow, centres, strengths } = this;
    const before = this.front;
    // The rest clocks. A new target (data, the entrance) is not the tail of an input.
    if (Math.abs(target - this.aim) > 0.5) this.capped = false;
    this.aim = target;
    if (push !== 0) {
      this.sinceInput = 0;
      this.capped = true;
    } else this.sinceInput += dt;
    this.quiet = push !== 0 || Math.abs(this.velocity) > REST.level ? 0 : this.quiet + dt;
    for (let n = 0; n < steps; n++) {
      this.clock += h;
      // The level: a mass on a spring that rebounds off a full mark's tip.
      this.velocity += (w * w * (target - this.front) - 2 * LEVEL.zeta * w * this.velocity) * h;
      this.front += this.velocity * h;
      if (ceiling !== undefined && this.front > ceiling && this.velocity > 0) {
        this.front = ceiling;
        if (this.velocity > this.landed) this.landed = this.velocity;
        this.velocity *= -LEVEL.restitution;
      }
      if (!simulated) continue;
      // Uneven inflow: faster through the middle and in slow lumps, by the level's speed.
      const rate = Math.abs(this.velocity) * FLUID.uneven;
      if (rate > 0.01) {
        // The lumps drift in time, not across the mark: sampled once a substep for every column.
        for (let j = 0; j < 3; j++) {
          centres[j] = (noise(this.seed + j * 7.3, this.clock * 0.9 + j) * 2 - 1) * 0.8;
          strengths[j] = noise(this.seed + j * 3.1 + 11, this.clock * FLUID.lumps + j * 1.7);
        }
        let mean = 0;
        for (let x = 0; x < NX; x++) {
          const s = (x / (NX - 1)) * 2 - 1;
          let q = FLUID.middle * (1 - s * s);
          for (let j = 0; j < 3; j++)
            q +=
              strengths[j] * (1 - FLUID.middle) * 1.6 * Math.exp(-(((s - centres[j]) / 0.28) ** 2));
          inflow[x] = q;
          mean += q;
        }
        mean /= NX;
        const sign = Math.sign(this.velocity);
        for (let x = 0; x < NX; x++) {
          const add = (inflow[x] - mean) * rate * sign * h;
          for (let z = 0; z < NZ; z++) height[z * NX + x] += add * (1 - 0.15 * (z / (NZ - 1)));
        }
        this.still = false;
      }
      // The pointer drags the surface: liquid piles up ahead of it and drops behind.
      if (push !== 0) {
        for (let x = 0; x < NX; x++) {
          const d = (x - pointerAt) / 3;
          const force = -DRAG * reach * push * 2 * d * Math.exp(-d * d) * FLUID.speed;
          for (let z = 0; z < NZ; z++) speed[z * NX + x] += force * h;
        }
        this.still = false;
      }
      if (this.still) continue;
      // The damped wave equation with velocity diffusion; walls reflect (zero flux).
      for (let z = 0; z < NZ; z++)
        for (let x = 0; x < NX; x++) {
          const i = z * NX + x;
          const l = x > 0 ? i - 1 : i,
            r = x < NX - 1 ? i + 1 : i,
            f = z > 0 ? i - NX : i,
            b = z < NZ - 1 ? i + NX : i;
          const lap =
            (height[l] + height[r] - 2 * height[i]) / (dx * dx) +
            (height[f] + height[b] - 2 * height[i]) / (dz * dz);
          const vlap =
            (speed[l] + speed[r] - 2 * speed[i]) / (dx * dx) +
            (speed[f] + speed[b] - 2 * speed[i]) / (dz * dz);
          scratch[i] =
            speed[i] + (c2 * lap + FLUID.viscosity * vlap - FLUID.damping * speed[i]) * h;
        }
      speed.set(scratch);
      const floor = -Math.max(this.front, 0);
      for (let i = 0; i < NX * NZ; i++) {
        height[i] += speed[i] * h;
        // Never below the base, and a full mark's tip splashes cells back down.
        if (height[i] < floor) {
          height[i] = floor;
          if (speed[i] < 0) speed[i] = 0;
        }
        if (ceiling !== undefined && this.front + height[i] > ceiling) {
          height[i] = ceiling - this.front;
          if (speed[i] > 0) {
            if (speed[i] > this.splashed) this.splashed = speed[i];
            speed[i] *= -SPLASH;
          }
        }
      }
    }
    if (simulated && !this.still) {
      balance(height);
      // Balancing may lift a cell back past the tip; the tip wins.
      if (ceiling !== undefined)
        for (let i = 0; i < NX * NZ; i++) height[i] = Math.min(height[i], ceiling - this.front);
    }
    // Rest: relax what the eye can't see once nothing drives the surface, and fade every tail of
    // an input out by REST.cap s after it. Uniform scaling keeps the field's sum at zero.
    const tail = this.capped ? smooth((this.sinceInput - REST.cap + 0.5) / 0.5) : 0;
    if (simulated && !this.still) {
      let peak = 0;
      for (let i = 0; i < NX * NZ; i++) peak = Math.max(peak, Math.abs(height[i]));
      const relax = smooth((this.quiet - REST.after) / 0.4) * (1 - smooth(peak / REST.visible));
      const rate = Math.max(REST.rate * relax, REST.fade * tail);
      if (rate > 0) {
        const keep = Math.exp(-rate * dt);
        for (let i = 0; i < NX * NZ; i++) {
          height[i] *= keep;
          speed[i] *= keep;
        }
      }
    }
    const drain = DRAIN * (!pointer && this.capped ? DRAIN_AWAY : 1) * (1 + 4 * tail);
    // The coat: per column, what the liquid left on the glass, staying put while the level moves
    // under it and draining in uneven runs.
    const moved = this.front - before;
    let coated = false;
    for (let x = 0; x < NX; x++) {
      let top = 0;
      if (simulated && !this.still) {
        top = -Infinity;
        for (let z = 0; z < NZ; z++) top = Math.max(top, height[z * NX + x]);
      }
      this.wet[x] = Math.min(
        COAT_MAX,
        Math.max(top, this.wet[x] - moved - drain * this.runs[x] * dt),
      );
      if (this.wet[x] - top < 0.3) this.wet[x] = top > 0 ? top : 0;
      else coated = true;
    }

    let moving = coated;
    if (Math.abs(target - this.front) < 0.15 && Math.abs(this.velocity) < 2) {
      this.front = target;
      this.velocity = 0;
    } else moving = true;
    if (simulated && !this.still) {
      let peak = 0,
        energy = 0;
      for (let i = 0; i < NX * NZ; i++) {
        peak = Math.max(peak, Math.abs(height[i]));
        energy = Math.max(energy, Math.abs(speed[i]));
      }
      if (
        peak < STILL.peak &&
        energy < STILL.energy &&
        push === 0 &&
        !coated &&
        this.velocity === 0
      ) {
        height.fill(0);
        speed.fill(0);
        this.wet.fill(0);
        this.still = true;
      } else moving = true;
    }
    return moving;
  }
}

/** Keeps a field's sum at zero, so the level stays true. */
function balance(field: Float32Array) {
  let sum = 0;
  for (let k = 0; k < field.length; k++) sum += field[k];
  const mean = sum / field.length;
  for (let k = 0; k < field.length; k++) field[k] -= mean;
}

/** A cheap fingerprint of a field, for renderers that skip unchanged frames. */
export function fieldPrint(field: Float32Array | undefined) {
  if (!field) return 0;
  let sum = 0;
  for (let k = 0; k < field.length; k++) sum += field[k] * ((k % 7) + 1);
  return Math.round(sum * 1000);
}
