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
"use client";

import type { Theme } from "./colour";
import { createContext } from "react";

/*
 * What a frame can be lent from outside it: the glass cursor, and sounds and haptics. The frame
 * ships with neither; `<ChartCursor>` (`liquid-senses`) provides them to every chart inside it, so
 * no chart takes a prop for them and a project without that slice has a plain frame.
 *
 * Apart from frame.tsx on purpose: this module carries no stylesheet, so a page that imports
 * `<ChartCursor>` early (the hub, the landing showcase, the docs previews) doesn't pull the frame's
 * stylesheets ahead of the rest and change which of two equally specific rules wins.
 */
export type FrameSenses = {
  /** The glass cursor: attach it to a plot; returns the undo. */
  cursor?: { attach(plot: HTMLElement, theme: Theme): () => void };
  /** Sounds and haptics for one plot: a cue as the focus moves (`quiet` when the page scrolled
   * under a still pointer), as an item is pinned (`on`) and let go, and `prime` on a press, since
   * browsers open audio only then. */
  feedback?(plot: HTMLElement): {
    focus(quiet: boolean): void;
    pin(on: boolean): void;
    prime(): void;
    stop(): void;
  };
};

/** The senses lent to every frame inside; null outside a `<ChartCursor>`. */
export const FrameSensesContext = createContext<FrameSenses | null>(null);
/** Lends `FrameSenses` to every frame inside; `<ChartCursor>` wraps this. */
export const FrameSensesProvider = FrameSensesContext.Provider;
