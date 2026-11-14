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
import type { Rect } from "./geometry";

/**
 * The focused item's box as its chart last placed the tooltip beside it, so a pointer layer can
 * frame or follow the item without asking each family for its geometry. It is kept per plot (the
 * tooltip's offset parent, which the box is measured in) with the tooltip's tint, and it lapses
 * when the tooltip leaves the plot.
 */
type Anchor = { rect: Rect; tip: HTMLElement };
const anchors = new WeakMap<Element, Anchor>();
const watchers = new WeakMap<Element, () => void>();

export function noteFocusAnchor(tip: HTMLElement, rect: Rect) {
  const plot = tip.offsetParent;
  if (!plot) return;
  const held = anchors.get(plot);
  if (held && held.tip === tip) {
    const r = held.rect;
    if (
      r.left === rect.left &&
      r.top === rect.top &&
      r.right === rect.right &&
      r.bottom === rect.bottom
    )
      return;
    r.left = rect.left;
    r.top = rect.top;
    r.right = rect.right;
    r.bottom = rect.bottom;
  } else anchors.set(plot, { rect: { ...rect }, tip });
  watchers.get(plot)?.();
}

/** Calls `wake` whenever the focused item's box in `plot` moves; returns the undo. */
export function watchFocusAnchor(plot: Element, wake: () => void) {
  watchers.set(plot, wake);
  return () => {
    if (watchers.get(plot) === wake) watchers.delete(plot);
  };
}

/** The focused item's box in `plot` px and its series tint, or null when nothing is in focus. */
export function focusAnchor(plot: Element): { rect: Rect; tint: string | null } | null {
  const held = anchors.get(plot);
  if (!held) return null;
  if (!held.tip.isConnected || held.tip.offsetParent !== plot) {
    anchors.delete(plot);
    return null;
  }
  return { rect: held.rect, tint: held.tip.style.getPropertyValue("--tint") || null };
}
