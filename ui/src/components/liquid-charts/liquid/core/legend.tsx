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

import { forwardRef, type ForwardedRef, type ReactNode } from "react";
import type { HeightMeaning } from "./style";
import { cssModule } from "./css-module";
import Styles from "./core.module.css";
import Roles from "./type.module.css";

const styles = cssModule(Styles, "lc-core-");
const roles = cssModule(Roles, "lc-type-");

export type LegendEntry = {
  key: string;
  label: ReactNode;
  color: string;
  /** Drawn quieter: a comparison series, such as last period. */
  muted?: boolean;
};

/** Series names with their swatches, as plain text. With `onToggle`, each name is also a button that
 * hides its series; a hidden one fades. */
export function Legend({
  items,
  hidden,
  onToggle,
}: {
  items: readonly LegendEntry[];
  hidden?: ReadonlySet<string>;
  onToggle?: (key: string) => void;
}) {
  return (
    <div className={styles.legend}>
      {items.map((item) => {
        const swatch = (
          <span
            className={styles.swatch}
            data-muted={item.muted}
            style={{ background: item.color }}
            aria-hidden="true"
          />
        );
        return onToggle ? (
          <button
            key={item.key}
            type="button"
            className={`${styles.legendItem} ${roles.caption}`}
            aria-pressed={!hidden?.has(item.key)}
            onClick={() => onToggle(item.key)}
          >
            {swatch}
            {item.label}
          </button>
        ) : (
          <span key={item.key} className={`${styles.legendItem} ${roles.caption}`}>
            {swatch}
            {item.label}
          </span>
        );
      })}
    </div>
  );
}

/** What a tilted slab's height stands for, in words: "value", "level", or the second measure's name. */
export function heightNoteText(meaning: HeightMeaning, measureLabel?: string) {
  if (meaning === "depth") return "level";
  if (meaning === "second-measure") return measureLabel ?? "second measure";
  return "value";
}

/**
 * The key a tilted ring carries for its third dimension: one quiet line of plain text along the
 * plot's top ("Height · Growth vs Q2"), with no icon. It starts hidden; the chart fades it in with
 * the tilt by writing `opacity` through `ref` each frame. Shared by the donut, radial bars, rose and
 * sunburst so all four say it the same way.
 */
export const HeightNote = forwardRef(function HeightNote(
  {
    meaning,
    measureLabel,
    shown,
  }: {
    meaning: HeightMeaning;
    measureLabel?: string;
    /** True while the chart is tilted, so assistive tech reads it only then. */
    shown: boolean;
  },
  ref: ForwardedRef<HTMLDivElement>,
) {
  return (
    <div ref={ref} className={`${styles.heightNote} ${roles.caption}`} aria-hidden={!shown}>
      Height · {heightNoteText(meaning, measureLabel)}
    </div>
  );
});
