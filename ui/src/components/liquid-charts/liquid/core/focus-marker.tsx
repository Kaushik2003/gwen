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
  type CSSProperties,
  type ForwardedRef,
} from "react";
import { cssModule } from "./css-module";
import Styles from "./core.module.css";

const styles = cssModule(Styles, "lc-core-");

/**
 * The focus point on lines, areas and spectra: a black hole. Its centre is a void in the card's own
 * colour, so the line seems to fall into it; round it a crisp photon ring of white-hot light in the
 * series colour, a soft corona, and a faint lensing ring just outside. On light cards it is a white
 * dimple with an ink ring and a tinted shadow. `latest` is a smaller one whose corona breathes, for
 * the newest value; `point` is a quiet head for lollipops and dumbbells. Position it with `x`/`y`
 * (frame px), or move it every frame through `ref` with a translate.
 */
export const FocusMarker = forwardRef(function FocusMarker(
  {
    x,
    y,
    colour,
    variant = "focus",
    visible = true,
  }: {
    x?: number;
    y?: number;
    /** The series colour (any CSS colour). */
    colour: string;
    /** `point` is a smaller head for lollipops and dumbbells; the focused one uses `focus`. */
    variant?: "focus" | "latest" | "point";
    visible?: boolean;
  },
  ref: ForwardedRef<HTMLSpanElement>,
) {
  // The latest marker's corona breathes only on screen: offscreen its animation is paused
  // (`data-onscreen`), so a page of line charts at rest does not restyle every frame.
  const own = useRef<HTMLSpanElement | null>(null);
  const attach = useCallback(
    (node: HTMLSpanElement | null) => {
      own.current = node;
      if (typeof ref === "function") ref(node);
      else if (ref) ref.current = node;
    },
    [ref],
  );
  useEffect(() => {
    const element = own.current;
    if (variant !== "latest" || !element || typeof IntersectionObserver === "undefined") return;
    const observer = new IntersectionObserver(([entry]) => {
      element.dataset.onscreen = String(entry.isIntersecting);
    });
    observer.observe(element);
    return () => observer.disconnect();
  }, [variant]);
  return (
    <span
      ref={attach}
      className={styles.focusMarker}
      data-variant={variant}
      data-visible={visible}
      aria-hidden="true"
      style={
        {
          "--marker": colour,
          ...(x !== undefined && y !== undefined ? { transform: `translate(${x}px, ${y}px)` } : {}),
        } as CSSProperties
      }
    />
  );
});
