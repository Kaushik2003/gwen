"use client";
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

import { resolveStyle, type BarFrame, type BarMark, type Density } from "@/components/liquid-charts/liquid/core";
import { useEffect, useLayoutEffect, useState, type RefObject } from "react";

/* The KPI family draws its marks through the core's mark layer (glass, with `level` for liquid in
 * a container, or flat SVG when the style or the device says flat). These are the family's choices
 * for it: which material a chart draws with, and how its thin marks sit in the layer. */

export type Layout = (width: number, height: number) => BarMark[];
export type OnFrame = (bars: BarFrame[], width: number, height: number) => void;

/** KPI marks are thin: the glass canvas bleeds 64px (the core's default is 24) so the glow fades
 * before the canvas ends instead of showing as a band around the capsule, and flat corners stay at
 * 2px. Spread into the core `MarkLayer`. */
export const KPI_LAYER = { bleed: 64, flatRadius: 2 } as const;

const useIsomorphicLayoutEffect = typeof window === "undefined" ? useEffect : useLayoutEffect;

/** An element's size, kept current by a ResizeObserver. Zero until measured. */
export function useSize(ref: RefObject<HTMLElement | null>) {
  const [size, setSize] = useState({ width: 0, height: 0 });
  useIsomorphicLayoutEffect(() => {
    const element = ref.current;
    if (!element) return;
    const observer = new ResizeObserver(([entry]) => {
      const { width, height } = entry.contentRect;
      setSize((s) => (s.width === width && s.height === height ? s : { width, height }));
    });
    observer.observe(element);
    return () => observer.disconnect();
  }, [ref]);
  return size;
}

/** The material a level chart draws with: liquid in glass by default, flat when forced, when
 * the marks are too small to read as glass, or when the style says so. */
export function levelMaterial({
  material,
  density,
  thickness,
}: {
  material?: "glass" | "flat";
  density?: Density;
  thickness: number;
}): "liquid" | "flat" {
  const style = resolveStyle({
    geometry: "cartesian",
    marks: ["solid", "reference"],
    measure: "level",
    density,
    smallestMark: thickness,
    material: material === "flat" ? "flat" : undefined,
  });
  return style.materials.solid === "flat" ? "flat" : "liquid";
}

/** The material a segmented meter draws with: glass segments, or flat. */
export function meterMaterial({
  material,
  density,
  segments,
  smallest,
}: {
  material?: "glass" | "flat";
  density?: Density;
  segments: number;
  smallest: number;
}): "glass" | "flat" {
  const style = resolveStyle({
    geometry: "cartesian",
    marks: ["solid", "reference"],
    precise: true,
    density,
    markCount: segments,
    smallestMark: smallest,
    material: material === "flat" ? "flat" : undefined,
  });
  return style.materials.solid === "flat" ? "flat" : "glass";
}
