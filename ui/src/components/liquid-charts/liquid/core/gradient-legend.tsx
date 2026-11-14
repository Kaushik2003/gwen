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

import { cssRgb, gradientCss, type ColourScale } from "./colour-scale";
import type { CSSProperties, ReactNode } from "react";
import Styles from "./gradient-legend.module.css";
import type { ValueFormat } from "./format";
import { cssModule } from "./css-module";
import Roles from "./type.module.css";
import type { Rgb } from "./colour";
import { Figure } from "./figure";

const styles = cssModule(Styles, "lc-gradient-legend-");
const roles = cssModule(Roles, "lc-type-");

function Fig({
  value,
  format,
  signed,
  className,
}: {
  value: number;
  format: ValueFormat;
  signed?: boolean;
  className: string;
}) {
  return (
    <Figure
      value={value}
      kind={format.kind}
      decimals={format.decimals}
      signed={signed ?? format.signed}
      unit={format.unit}
      className={className}
    />
  );
}

/**
 * A colour scale as a bar: its gradient (linear in value, so a diverging midpoint sits where it
 * truly is), the domain's ends, the midpoint named, and a marker that glides to the value in
 * focus. An etched key names missing values when there are any. Plain text throughout; the chart
 * itself carries the accessible reading, so the legend is hidden from assistive tech.
 */
export function GradientLegend({
  scale,
  format,
  name,
  value,
  lowLabel,
  highLabel,
  midLabel,
  midValue = true,
  missing = false,
  extra,
  background,
  compact = false,
  thin = false,
}: {
  scale: ColourScale;
  format: ValueFormat;
  name?: ReactNode;
  /** The value in focus (the marker), or null. */
  value: number | null;
  /** Names for the two sides of a diverging scale. */
  lowLabel?: ReactNode;
  highLabel?: ReactNode;
  /** What the midpoint means ("SLO"). */
  midLabel?: ReactNode;
  /** Show the midpoint's value after its name. Default true. */
  midValue?: boolean;
  missing?: boolean;
  /** More keys after the bar (today, a streak). */
  extra?: ReactNode;
  /** Replaces the gradient (glass and beads show strength in one tint, not colour). */
  background?: string;
  /** A shorter bar, for tile-sized charts. */
  compact?: boolean;
  /** A thinner, squarer bar (6px, 1px corners) that sits quietly under a map. */
  thin?: boolean;
}) {
  const [lo, hi] = scale.domain;
  const mid = scale.midpoint;
  const at = value === null ? null : scale.offset(value);
  const tick = `${roles.micro} ${styles.tick}`;
  return (
    <div className={styles.legend} data-compact={compact} data-thin={thin} aria-hidden="true">
      {name && <span className={`${roles.caption} ${styles.name}`}>{name}</span>}
      <span className={styles.scale}>
        {lowLabel && <span className={`${roles.micro} ${styles.side}`}>{lowLabel}</span>}
        <Fig value={lo} format={format} signed={false} className={tick} />
        <span className={styles.track}>
          <span
            className={styles.bar}
            style={{ background: background ?? gradientCss(scale.gradient()) }}
          />
          {mid !== null && (
            <span className={styles.mid} style={{ left: `${scale.offset(mid) * 100}%` }}>
              {(midValue || midLabel) && (
                <span className={`${roles.micro} ${styles.midLabel}`}>
                  {midLabel}
                  {midValue && (
                    <>
                      {midLabel ? " " : ""}
                      <Fig value={mid} format={format} signed={false} className={roles.micro} />
                    </>
                  )}
                </span>
              )}
            </span>
          )}
          <span
            className={styles.marker}
            data-shown={at !== null}
            style={{ left: `${(at ?? 0.5) * 100}%` } as CSSProperties}
          >
            {value !== null && (
              <span className={styles.value}>
                <Fig value={value} format={format} className={roles.micro} />
              </span>
            )}
          </span>
        </span>
        <Fig value={hi} format={format} signed={false} className={tick} />
        {highLabel && <span className={`${roles.micro} ${styles.side}`}>{highLabel}</span>}
      </span>
      {missing && (
        <span className={`${roles.caption} ${styles.key}`}>
          <span className={styles.etched} />
          No data
        </span>
      )}
      {extra}
    </div>
  );
}

/** One tint's strength, faint to full, for a legend bar when the colour itself is constant (glass
 * levels, beads). */
export const strengthCss = (tint: Rgb, faint = 0.1) =>
  `linear-gradient(90deg, ${cssRgb(tint, faint)}, ${cssRgb(tint)})`;
