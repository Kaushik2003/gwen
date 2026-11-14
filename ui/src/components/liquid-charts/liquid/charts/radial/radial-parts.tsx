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

import { formatValue, type ValueFormat } from "@/components/liquid-charts/liquid/core/format";
import { useCallback, useEffect, useRef, type ReactNode } from "react";
import type { ChartConfig } from "@/components/liquid-charts/liquid/core/colour";
import { formatFigure } from "@/components/liquid-charts/liquid/core/figure";

export type { ValueFormat };

/** A series' or category's name from its config, as text when the label is a string. */
export function labelText(config: ChartConfig, key: string, fallback?: string) {
  const label = config[key]?.label;
  return typeof label === "string" || typeof label === "number" ? String(label) : (fallback ?? key);
}
export function labelNode(config: ChartConfig, key: string, fallback?: string): ReactNode {
  return config[key]?.label ?? fallback ?? key;
}

/** A value as plain text for the live region and the data table ("1,240 kWh", "82%"). */
export const plain = (value: number | null | undefined, format: ValueFormat) =>
  value === null || value === undefined ? "no data" : formatValue(value, format);

/** A value set as `Figure` sets it, for titles and chips ("−$6.2K", "412 ms"). */
export const figureText = (value: number, format: ValueFormat) =>
  formatFigure(value, {
    kind: format.kind,
    decimals: format.decimals,
    signed: format.signed,
    unit: format.unit,
  });

/** Measured boxes of labels, kept current by one ResizeObserver; read them every frame. */
export function useMeasured() {
  const elements = useRef(new Map<string, HTMLElement>());
  const sizes = useRef(new Map<string, { width: number; height: number }>());
  const observer = useRef<ResizeObserver | null>(null);
  useEffect(() => {
    observer.current = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const id = (entry.target as HTMLElement).dataset.measure;
        const box = entry.borderBoxSize?.[0];
        if (id && box) sizes.current.set(id, { width: box.inlineSize, height: box.blockSize });
      }
    });
    for (const element of Array.from(elements.current.values())) observer.current.observe(element);
    return () => observer.current?.disconnect();
  }, []);
  const ref = useCallback(
    (id: string) => (element: HTMLElement | null) => {
      const previous = elements.current.get(id);
      if (previous && previous !== element) observer.current?.unobserve(previous);
      if (!element) {
        elements.current.delete(id);
        sizes.current.delete(id);
        return;
      }
      element.dataset.measure = id;
      elements.current.set(id, element);
      observer.current?.observe(element);
    },
    [],
  );
  const size = useCallback((id: string) => {
    let known = sizes.current.get(id);
    const element = elements.current.get(id);
    if (!known && element) {
      known = { width: element.offsetWidth, height: element.offsetHeight };
      if (known.width) sizes.current.set(id, known);
    }
    return known ?? { width: 0, height: 0 };
  }, []);
  const element = useCallback((id: string) => elements.current.get(id) ?? null, []);
  return { ref, size, element };
}

/** Moves an absolutely placed element to (x, y) and shows or hides it, writing only on change. */
export function put(element: HTMLElement | null, x: number, y: number, shown: boolean) {
  if (!element) return;
  const transform = `translate(${Math.round(x * 100) / 100}px, ${Math.round(y * 100) / 100}px)`;
  if (element.style.transform !== transform) element.style.transform = transform;
  const value = String(shown);
  if (element.dataset.shown !== value) element.dataset.shown = value;
  if (element.style.visibility !== "visible") element.style.visibility = "visible";
}

/** The message a state shows, from a prop that may be `true` or a node. */
export const stateMessage = (value: boolean | ReactNode) =>
  typeof value === "boolean" ? undefined : value;
