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
/** What the feel reads off a mark's liquid: its impacts since the last read (the glass pack's
 * simulated liquid reports them; a still one never lands). */
type Impacts = { takeImpact(): Readonly<{ land: number; splash: number }> };

/**
 * The liquid's moments, for feedback that keeps time with it (a sound, a phone's tap, the glass
 * cursor's give):
 * - `press`: focus arrives on a mark and its column sags (pointer or keyboard);
 * - `land`: the level, refilling after a press, lands back on a full mark's tip;
 * - `splash`: the surface splashes off that tip.
 *
 * Mark layers report them from their frame loops at the moment the liquid moves, through
 * `liquidFeel`; a chart's frame listens on its plot with `onLiquid`. Landings and splashes count
 * only within a second of a press on the same mark, so an entrance or new data never sounds.
 * A report is queued, not heard inside the loop: the queue is delivered once the frame has drawn
 * (a microtask after the loop's callback), in the same frame, each listener on its own, so one
 * that reads layout, starts audio or throws never lands between canvas writes. With no listener a
 * report is dropped after a few parent hops, and nothing here ever keeps a loop running.
 */
export type LiquidMoment = {
  kind: "press" | "land" | "splash";
  /** How hard, 0..1. */
  strength: number;
  /** The mark's reach as a share of the layer (0 at the foot, 1 at the far edge), if known. */
  height?: number;
  /** The pointer pressed it (not the keyboard). */
  pointer: boolean;
};
type Listener = (moment: LiquidMoment) => void;

const listeners = new WeakMap<Element, Set<Listener>>();

/** Listens for the liquid moments of every mark layer inside `element`; returns the undo. */
export function onLiquid(element: Element, listener: Listener) {
  let set = listeners.get(element);
  if (!set) listeners.set(element, (set = new Set()));
  set.add(listener);
  return () => {
    set.delete(listener);
    if (!set.size) listeners.delete(element);
  };
}

/** The nearest ancestor of `from` (or itself) that listens. */
function listening(from: Element | null) {
  for (let at = from; at; at = at.parentElement) {
    const set = listeners.get(at);
    if (set) return set;
  }
  return null;
}

/** Moments waiting for the frame to finish drawing, and their listeners. One array, reused. */
const queue: { set: Set<Listener>; moment: LiquidMoment }[] = [];
function deliver() {
  const due = queue.splice(0);
  for (const { set, moment } of due)
    for (const listener of set)
      try {
        listener(moment);
      } catch (error) {
        // One listener failing must not silence the rest, nor the next frame's.
        setTimeout(() => {
          throw error;
        });
      }
}

/** Tells the nearest listening ancestor of `from` about a moment, once the frame has drawn. */
export function emitLiquid(from: Element | null, moment: LiquidMoment) {
  const set = listening(from);
  if (!set) return;
  if (!queue.length) queueMicrotask(deliver);
  queue.push({ set, moment });
}

/** Impacts (px/s) below these are the liquid at rest, not a moment. A press (150 px/s down on
 * the level, ζ 0.55) brings it back onto a full mark's tip at about 6 to 8 px/s, some 0.6 s on;
 * splashes that size come off the tip as it settles, and only a pointer's sweep throws harder. */
const LAND_MIN = 3;
const SPLASH_MIN = 90;
/** A landing this hard (px/s) is full strength. */
const LAND_FULL = 10;
/** How long after a press its landing or splash still belongs to it (ms). */
const AFTER_PRESS = 1000;

/**
 * A mark layer's reporter: `press` where a mark is pressed, `after` once its liquid has stepped
 * (every frame, so its impacts are taken and cleared).
 */
export function liquidFeel(root: Element) {
  const pressed = new Map<string, { at: number; pointer: boolean; splashed: boolean }>();
  return {
    press(key: string, pointer: boolean, height?: number) {
      pressed.set(key, { at: performance.now(), pointer, splashed: false });
      emitLiquid(root, { kind: "press", strength: pointer ? 1 : 0.6, height, pointer });
    },
    after(key: string, liquid: Impacts, height?: number) {
      const impact = liquid.takeImpact();
      if (impact.land < LAND_MIN && impact.splash < SPLASH_MIN) return;
      const press = pressed.get(key);
      if (!press || performance.now() - press.at > AFTER_PRESS) return;
      const { pointer } = press;
      if (impact.land >= LAND_MIN) {
        pressed.delete(key);
        emitLiquid(root, {
          kind: "land",
          strength: Math.min(1, impact.land / LAND_FULL),
          height,
          pointer,
        });
      } else if (!press.splashed) {
        // One splash per press: the surface throws many small ones as it settles.
        press.splashed = true;
        emitLiquid(root, {
          kind: "splash",
          strength: Math.min(1, impact.splash / 400),
          height,
          pointer,
        });
      }
    },
  };
}
