"use client";
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

import { useCallback, useEffect, useRef, type ReactNode } from "react";
import type { ChartConfig } from "@/components/liquid-charts/liquid/core/colour";
import type { FigureKind } from "@/components/liquid-charts/liquid/core/figure";
import { numberFormat } from "@/components/liquid-charts/liquid/core/format";

/** How a chart sets its numbers: the `Figure` kind, decimals and an explicit plus sign. */
export type RingFormat = {
  kind: FigureKind;
  decimals?: number;
  signed?: boolean;
  currency?: string;
};

/** A value as plain text, set the way `Figure` sets it (for aria labels and data tables). */
export function formatText(value: number, format: RingFormat = { kind: "number" }) {
  const { kind, decimals = 0, signed = false, currency = "USD" } = format;
  const options: Intl.NumberFormatOptions =
    kind === "currency"
      ? {
          style: "currency",
          currency,
          minimumFractionDigits: decimals,
          maximumFractionDigits: decimals,
        }
      : kind === "currencyCompact"
        ? {
            style: "currency",
            currency,
            notation: "compact",
            // Compact figures drop trailing zeros; WebKit otherwise keeps a currency's two.
            minimumFractionDigits: 0,
            maximumFractionDigits: decimals,
          }
        : kind === "percent"
          ? { style: "percent", minimumFractionDigits: decimals, maximumFractionDigits: decimals }
          : kind === "compact"
            ? { notation: "compact", minimumFractionDigits: 0, maximumFractionDigits: decimals }
            : { minimumFractionDigits: decimals, maximumFractionDigits: decimals };
  const text = numberFormat(options).format(Math.abs(value));
  return `${value < 0 ? "−" : signed ? "+" : ""}${text}`;
}

/** A series' name from its config, as text when the label is a string. */
export function labelText(config: ChartConfig, key: string, fallback?: string) {
  const label = config[key]?.label;
  return typeof label === "string" || typeof label === "number" ? String(label) : (fallback ?? key);
}
export function labelNode(config: ChartConfig, key: string, fallback?: string): ReactNode {
  return config[key]?.label ?? fallback ?? key;
}

/** Measured label boxes, kept current by one ResizeObserver; read them every frame. */
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
    for (const element of elements.current.values()) observer.current.observe(element);
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
