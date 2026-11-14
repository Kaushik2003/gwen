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
 * The pointer as the liquid feels it: its place over a mark layer (in the layer's px) and its
 * smoothed velocity (px/s). Mark layers ignore the pointer (the chart's frame handles hover), so
 * this listens on the document and only wakes the layer when the pointer is near it. A pointer
 * that stops no longer pushes: `settle` lets its velocity fade.
 */
export type LiquidPointer = { x: number; y: number; vx: number; vy: number };

export function trackPointer(root: HTMLElement, wake: () => void, reach = 40) {
  let pointer: LiquidPointer | null = null,
    at = 0,
    screenX = 0,
    screenY = 0;
  const doc = root.ownerDocument;
  const move = (event: PointerEvent) => {
    const r = root.getBoundingClientRect();
    const x = event.clientX - r.left,
      y = event.clientY - r.top;
    // The velocity is the pointer's own motion on screen. A scroll carries the layer under a still
    // pointer (and the browser sends pointermoves for it); that must not stir the liquid or wake
    // the loop.
    const dx = event.clientX - screenX,
      dy = event.clientY - screenY;
    screenX = event.clientX;
    screenY = event.clientY;
    if (x < -reach || y < -reach || x > r.width + reach || y > r.height + reach) {
      if (pointer) {
        pointer = null;
        wake();
      }
      return;
    }
    const now = event.timeStamp || performance.now();
    const still = pointer !== null && dx === 0 && dy === 0;
    if (pointer && now > at) {
      const dt = Math.max(0.008, (now - at) / 1000);
      pointer = {
        x,
        y,
        vx: pointer.vx * 0.5 + (dx / dt) * 0.5,
        vy: pointer.vy * 0.5 + (dy / dt) * 0.5,
      };
    } else pointer = { x, y, vx: 0, vy: 0 };
    at = now;
    if (!still) wake();
  };
  const leave = () => {
    if (!pointer) return;
    pointer = null;
    wake();
  };
  doc.addEventListener("pointermove", move, { passive: true });
  doc.addEventListener("pointerleave", leave);
  return {
    get current() {
      return pointer;
    },
    /** Lets a resting pointer's velocity fade; returns whether it still pushes. */
    settle(dt: number) {
      if (!pointer) return false;
      const k = Math.exp(-12 * dt);
      pointer.vx *= k;
      pointer.vy *= k;
      return Math.abs(pointer.vx) + Math.abs(pointer.vy) > 4;
    },
    stop() {
      doc.removeEventListener("pointermove", move);
      doc.removeEventListener("pointerleave", leave);
    },
  };
}

/** Marks that just came into focus (hover or keyboard), to press. A mark is pressed when it is lit
 * and either was not before (focus moved onto it), or focus narrowed onto it: marks that were lit
 * with it have dimmed while it stayed lit. The second case is the common one where some marks are
 * never lit (a comparison drawn as graphite ghosts): there the focused bar was already lit at rest,
 * so only the narrowing shows it came into focus. Releasing focus (the lit set only grows) presses
 * nothing; nor do marks leaving the layout (hiding a series), nor the first frame. */
export function focusPresses() {
  let lit: Set<string> | null = null;
  return (marks: readonly { key: string; lit: number }[]) => {
    const next = new Set(marks.filter((mark) => mark.lit >= 0.5).map((mark) => mark.key));
    const before = lit;
    lit = next;
    if (!before || next.size === 0 || next.size === marks.length) return [];
    // Focus released (everything lit before is still lit, and more): nothing is pressed.
    if ([...before].every((key) => next.has(key))) return [];
    const joined = [...next].filter((key) => !before.has(key));
    if (joined.length) return joined;
    const present = new Set(marks.map((mark) => mark.key));
    const dimmed = [...before].some((key) => !next.has(key) && present.has(key));
    return dimmed ? [...next] : [];
  };
}

/**
 * What a frame takes from the pointer: the item under it on every move, and on a mouse's arrival
 * without a move. A hovering pointer can be put over a plot without moving: an overlay above the
 * chart goes away, or the viewport is re-emulated and restored (what a capture beyond the viewport
 * does). The browser then sends pointerout and pointerover but no pointermove, so a frame that
 * only took the item on moves would release it on the way out and sit unlit under a pointer that
 * is plainly over a bar. Only a mouse arrives that way: a finger or a pen arrives by tapping, which
 * pointerdown and click handle, and taking on its arrival would light, release and light again
 * across one tap. `quiet` says the pointer did not move on screen (the page scrolled, or content
 * changed, under a still pointer), so feedback keeps silent.
 */
export function pointerFocus() {
  let x = NaN,
    y = NaN;
  return (
    kind: "move" | "enter",
    event: { pointerType: string; screenX: number; screenY: number },
  ): { quiet: boolean } | null => {
    if (kind === "enter" && event.pointerType !== "mouse") return null;
    const quiet = event.screenX === x && event.screenY === y;
    x = event.screenX;
    y = event.screenY;
    return { quiet };
  };
}
