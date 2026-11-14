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

import {
  forwardRef,
  useCallback,
  useEffect,
  useRef,
  type ForwardedRef,
  type ReactNode,
} from "react";
import { placeLabels, type LabelRequest } from "./labels";
import { cssModule } from "./css-module";
import Styles from "./core.module.css";
import type { Rect } from "./geometry";
import Roles from "./type.module.css";

const styles = cssModule(Styles, "lc-core-");
const roles = cssModule(Roles, "lc-type-");

/** How much a focused figure grows. Matches `.lift` in type.module.css. */
export const LIFT = 1.22;
/** Class for secondary text inside a label or tooltip. */
export const mutedText = styles.muted;

export type ValueLabelRequest = Omit<LabelRequest, "width" | "height"> & {
  /** The label is lifted (in focus): it is placed at its grown size. */
  lifted?: boolean;
};

/**
 * Value labels that place themselves. Each label is measured once (and again whenever its content
 * changes size); `place` runs the collision solver with those sizes and moves the labels, hiding
 * the ones that would collide, overflow or sit on an obstacle. Call it from the mark layer's
 * `onFrame` so labels ride the marks as they spring. A label with priority 0 is simply hidden: it
 * is not measured, solved or moved (it fades out where it stood).
 */
export function useValueLabels() {
  const elements = useRef(new Map<string, HTMLDivElement>());
  const sizes = useRef(new Map<string, { width: number; height: number }>());
  const observer = useRef<ResizeObserver | null>(null);

  useEffect(() => {
    observer.current = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const id = (entry.target as HTMLElement).dataset.labelId;
        const box = entry.borderBoxSize?.[0];
        if (id && box) sizes.current.set(id, { width: box.inlineSize, height: box.blockSize });
      }
    });
    for (const element of elements.current.values()) observer.current.observe(element);
    return () => observer.current?.disconnect();
  }, []);

  const ref = useCallback(
    (id: string) => (element: HTMLDivElement | null) => {
      const previous = elements.current.get(id);
      if (previous && previous !== element) observer.current?.unobserve(previous);
      if (!element) {
        elements.current.delete(id);
        return;
      }
      element.dataset.labelId = id;
      elements.current.set(id, element);
      observer.current?.observe(element);
    },
    [],
  );

  const place = useCallback(
    (
      requests: readonly ValueLabelRequest[],
      /** `gap`: the least px between two labels, or a label and an obstacle (default 2). */
      options: { bounds: Rect; obstacles?: readonly Rect[]; gap?: number },
    ) => {
      const measured: LabelRequest[] = [];
      const natural: { width: number; height: number }[] = [];
      for (const request of requests) {
        const element = elements.current.get(request.id);
        if (!(request.priority > 0)) {
          if (element && element.dataset.shown !== "false") element.dataset.shown = "false";
          continue;
        }
        let size = sizes.current.get(request.id);
        if (!size && element) {
          size = { width: element.offsetWidth, height: element.offsetHeight };
          sizes.current.set(request.id, size);
        }
        const { width, height } = size ?? { width: 0, height: 0 };
        const k = request.lifted ? LIFT : 1;
        natural.push({ width, height });
        measured.push({ ...request, width: width * k, height: height * k });
      }
      placeLabels(measured, options).forEach((placed, i) => {
        const element = elements.current.get(placed.id);
        if (!element) return;
        // The solver placed the grown box; the element keeps its natural box and scales about the
        // edge nearest its mark, so shift it to sit inside the grown one.
        const grown = measured[i],
          { width, height } = natural[i];
        const spareX = grown.width - width,
          spareY = grown.height - height;
        const x = placed.side === "left" ? spareX : placed.side === "right" ? 0 : spareX / 2;
        const y = placed.side === "above" ? spareY : placed.side === "below" ? 0 : spareY / 2;
        element.style.transform = `translate(${placed.left + x}px, ${placed.top + y}px)`;
        element.dataset.side = placed.side;
        element.dataset.shown = String(placed.visible);
        element.style.visibility = "visible";
      });
    },
    [],
  );

  return { ref, place };
}

/** One value label. Its content is usually a `Figure` with the lift role. */
export const ValueLabel = forwardRef(function ValueLabel(
  {
    active = false,
    children,
  }: {
    active?: boolean;
    children: ReactNode;
  },
  ref: ForwardedRef<HTMLDivElement>,
) {
  return (
    <div ref={ref} className={styles.label} data-figure-active={active} aria-hidden="true">
      {children}
    </div>
  );
});

/**
 * A value label set as the chart's key value: its figure (pass one with a large role, such as
 * `figureL`) in the first ink, and a short note under it ("Highest month") in the quiet ink. It lifts
 * on focus like any value label. Use it as a `ValueLabel`'s content for the one value that matters.
 */
export function KeyValue({
  figure,
  note,
  className,
}: {
  figure: ReactNode;
  note?: ReactNode;
  /** A class of the host's own, for styling the key value from outside the chart. */
  className?: string;
}) {
  return (
    <span className={`${styles.keyValue} ${roles.lift} ${className ?? ""}`}>
      {figure}
      {note ? <span className={`${styles.keyNote} ${roles.micro}`}>{note}</span> : null}
    </span>
  );
}
