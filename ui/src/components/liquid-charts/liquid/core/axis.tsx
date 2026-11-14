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

import { useLayoutEffect, useRef, type ReactNode } from "react";
import { placeCategoryNames } from "./labels";
import { textWidth, TYPE } from "./format";
import { cssModule } from "./css-module";
import Styles from "./core.module.css";
import Roles from "./type.module.css";

const styles = cssModule(Styles, "lc-core-");
const roles = cssModule(Roles, "lc-type-");

/** How a value axis is drawn: `full`, every tick with its unit; `quiet`, solid hairlines with the
 * unit on the top tick only and zero unlabelled; `etched`, the quiet ticks' numbers with no lines
 * across the plot, the marks carrying the graduations in their glass (glass bars); `ruler`, the quiet ticks with three short marks between each pair at the plot's edge, a ruler's
 * graduations; `none`, the zero line alone, for a chart whose values are labelled on the marks. */
export type ValueAxis = "full" | "quiet" | "etched" | "ruler" | "none";

/** The digits of a tick and its sign, without its currency, unit or percent: "$300K" is "300",
 * "−$20K" is "−20", "40%" is "40". */
export function bareTick(text: string): string {
  const digits = /[\d][\d.,]*/.exec(text)?.[0];
  if (!digits) return text;
  return (/^[−-]/.test(text) ? "−" : "") + digits;
}

/** A tick without its unit: the currency sign, a trailing percent or a unit after a space go, the
 * digits and any compact suffix stay, so "$200K" is "200K" and "40%" is "40". */
function unitless(text: string): string {
  return text
    .replace(/[$£€¥]/g, "")
    .replace(/%$/, "")
    .replace(/\s[A-Za-z]+$/, "");
}

/**
 * A value axis' tick text for `mode`, made from its full format. Quiet and etched ticks name the
 * unit once, on the top tick ("$300K", then "200K", "100K"), keep every tick's compact suffix so no
 * figure reads smaller than it is, and leave zero to the baseline. `none` labels nothing. Charts size
 * their insets with this too, so a quiet or absent axis gives its room back.
 */
export function axisTicks(
  values: readonly number[],
  format: (value: number) => string,
  mode: ValueAxis = "full",
): (value: number) => string {
  if (mode === "full") return format;
  if (mode === "none") return () => "";
  const top = values.length ? Math.max(...values) : 0;
  return (value) => {
    if (value === 0) return "";
    const text = format(value);
    return value === top ? text : unitless(text);
  };
}

/** A quiet axis keeps about three lines: the top tick and every other one below it, and zero. */
export function quietLines(values: readonly number[]): readonly number[] {
  if (values.length <= 3) return values;
  const keep = new Set([...values].sort((a, b) => b - a).filter((_, i) => i % 2 === 0));
  if (values.includes(0)) keep.add(0);
  return values.filter((value) => keep.has(value));
}

/** A value tick as a figure, set the way value labels are: a currency sign and a compact suffix
 * (K, M, B, %) or a unit after a space smaller and quieter than the digits ("$300K", "40%",
 * "400 ms"). Text that isn't one number (a time, a name) is left as it is. */
const TICK = /^([−-]?)([$£€¥]?)(\d[\d,]*(?:\.\d+)?)([KMBT%]?)(\s[A-Za-z]+)?$/;
export function TickText({ text }: { text: ReactNode }) {
  const match = typeof text === "string" ? TICK.exec(text) : null;
  if (!match) return <>{text}</>;
  const [, sign, before, digits, suffix, unit] = match;
  return (
    <>
      {sign}
      {before && <span className={styles.tickUnit}>{before}</span>}
      {digits}
      {suffix && <span className={styles.tickUnit}>{suffix}</span>}
      {unit && <span className={styles.tickUnit}>{unit}</span>}
    </>
  );
}

/** Horizontal gridlines across the plot, each with its tick to the left. `position` maps a value
 * to frame px; the zero line is solid. A `quiet` axis draws about three solid hairlines; an
 * `etched` one keeps every tick's number but no line (the marks carry them); with `none` only the
 * zero line is left. */
export function Gridlines({
  values,
  position,
  format,
  axis = "y",
  mode = "full",
  graduations,
}: {
  values: readonly number[];
  position: (value: number) => number;
  format: (value: number) => ReactNode;
  /** `y` (the default): lines across the plot with ticks at the left. `x`: lines up the plot with
   * ticks under it; string ticks that would collide are thinned. */
  axis?: "x" | "y";
  mode?: ValueAxis;
  /** A ruler's marks run between these ticks (default: the ticks drawn), so a tick that gives way
   * to a label leaves its graduations evenly spaced. */
  graduations?: readonly number[];
}) {
  const shown =
    mode === "none"
      ? values.filter((value) => value === 0)
      : mode === "quiet"
        ? quietLines(values)
        : values;
  if (axis === "x") return <XGridlines values={shown} position={position} format={format} />;
  // A ruler's three marks between each pair of ticks.
  const sorted = [...(graduations ?? shown)].sort((a, b) => a - b);
  const pairs = sorted.slice(1).map((value, i) => [sorted[i], value] as const);
  return (
    <>
      {mode === "ruler" &&
        pairs.flatMap(([low, high]) =>
          [1, 2, 3].map((step) => (
            <span
              key={`mark-${low}-${step}`}
              className={styles.rulerMark}
              data-half={step === 2}
              style={{ top: position(low + ((high - low) * step) / 4) }}
              aria-hidden="true"
            />
          )),
        )}
      {shown.map((value) => (
        <span
          key={value}
          className={styles.gridline}
          data-zero={value === 0}
          data-quiet={mode === "quiet" || mode === "ruler"}
          data-etched={mode === "etched"}
          style={{ top: position(value) }}
          aria-hidden="true"
        >
          <span className={`${styles.tick} ${roles.micro}`}>
            <TickText text={format(value)} />
          </span>
        </span>
      ))}
    </>
  );
}

/** Vertical gridlines with ticks under the plot; string ticks that would collide are thinned. */
function XGridlines({
  values,
  position,
  format,
}: {
  values: readonly number[];
  position: (value: number) => number;
  format: (value: number) => ReactNode;
}) {
  const ticks = values.map((value) => ({ value, x: position(value), text: format(value) }));
  const shown: boolean[] = [];
  for (let i = 0, end = -Infinity; i < ticks.length; i++) {
    const { x, text } = ticks[i];
    const half = typeof text === "string" ? textWidth(text, TYPE.micro) / 2 : 0;
    shown.push(x - half >= end + 8);
    if (shown[i]) end = x + half;
  }
  return (
    <>
      {ticks.map(({ value, x, text }, i) => (
        <span
          key={value}
          className={styles.gridline}
          data-axis="x"
          data-zero={value === 0}
          style={{ left: Math.round(x) - 0.5 }}
          aria-hidden="true"
        >
          {shown[i] && <span className={`${styles.tick} ${roles.micro}`}>{text}</span>}
        </span>
      ))}
    </>
  );
}

/** A category's name, with a trailing year ("Jan 2025", "Q1 2026") set apart, so a chart can set
 * the year on a line of its own under the name (Liquid's editorial axis); elsewhere it reads as
 * written. Any other text is left as it is. */
export function NameText({ text }: { text: ReactNode }) {
  const match = typeof text === "string" ? /^(.*\S)\s('?\d{4})$/.exec(text) : null;
  if (!match) return <>{text}</>;
  return (
    <>
      {match[1]}
      <span className={styles.nameGap}> </span>
      <span className={styles.nameYear}>{match[2]}</span>
    </>
  );
}

/** Category names: under each column (`bottom`, centred on `position`) or beside each row
 * (`left`, at the frame's left edge). The focused one brightens; with `dim`, the rest fade. The
 * `strong` one (the category of a called-out value) is set in the first ink at rest. */
/** A run of categories that belong together (the months of a year), named once under them. */
export type CategoryGroup = { label: string; from: number; to: number };

export function CategoryAxis({
  labels,
  position,
  orientation = "bottom",
  active = null,
  strong = null,
  dim = false,
  pill = false,
  groups,
  step = 0,
  contain = false,
}: {
  labels: readonly ReactNode[];
  position: (index: number) => number;
  orientation?: "bottom" | "left";
  active?: number | null;
  strong?: number | null;
  dim?: boolean;
  /** The focused name sits in a small glass pill. */
  pill?: boolean;
  /** Runs of categories named once under their names (bottom names only): a tier of years under
   * months. Each spans its categories' slots, `step` wide. */
  groups?: readonly CategoryGroup[];
  step?: number;
  /** Measure bottom names in their actual face and keep them inside the frame. Group bands
   * remain tied to their original slots; only the text moves. */
  contain?: boolean;
}) {
  const grouped = orientation === "bottom" && groups && groups.length > 0;
  const starts = new Set(grouped ? groups.map((group) => group.from) : []);
  const root = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const element = root.current;
    if (!element) return;
    const names = Array.from(element.querySelectorAll<HTMLElement>("[data-category-index]"));
    if (!contain || orientation !== "bottom") {
      names.forEach((name, i) => {
        name.style.left = orientation === "bottom" ? `${position(i)}px` : "";
        name.style.visibility =
          name.style.maxWidth =
          name.style.overflow =
          name.style.textOverflow =
            "";
      });
      return;
    }
    const place = () => {
      const width = element.clientWidth;
      if (width <= 2) return;
      const widths = names.map((name) => {
        name.style.maxWidth = `${width - 2}px`;
        // A name longer than the entire frame gets an explicit ellipsis, never smaller type.
        // Ordinary names keep overflow visible so their separate year line remains intact.
        name.style.overflow = name.scrollWidth > name.clientWidth + 1 ? "hidden" : "";
        name.style.textOverflow = "ellipsis";
        return name.textContent ? name.offsetWidth : 0;
      });
      const placed = placeCategoryNames(
        names.map((_, i) => position(i)),
        widths,
        width,
        active,
      );
      names.forEach((name, i) => {
        name.style.left = `${placed[i].left + widths[i] / 2}px`;
        name.style.visibility = placed[i].visible ? "" : "hidden";
      });
    };
    place();
    const observer = new ResizeObserver(place);
    observer.observe(element);
    names.forEach((name) => observer.observe(name));
    return () => observer.disconnect();
  }, [contain, orientation, labels, position, active]);
  return (
    <div
      ref={root}
      className={styles.categories}
      data-orientation={orientation}
      data-groups={grouped ? "" : undefined}
      aria-hidden="true"
    >
      {grouped &&
        groups.map((group, i) => (
          <span
            key={`g${i}`}
            className={styles.group}
            data-band={i % 2 === 0 ? "even" : "odd"}
            style={{
              left: position(group.from) - step / 2,
              width: position(group.to) - position(group.from) + step,
            }}
          >
            <span className={`${styles.groupLabel} ${roles.micro}`}>{group.label}</span>
          </span>
        ))}
      {labels.map((label, i) => (
        <span
          key={i}
          data-category-index={i}
          className={`${styles.category} ${roles.label}`}
          data-active={active === i}
          data-strong={strong === i}
          data-group-start={starts.has(i) ? "" : undefined}
          data-pill={pill && active === i}
          data-dim={dim && active !== null && active !== i}
          style={orientation === "bottom" ? { left: position(i) } : { top: position(i) }}
        >
          <NameText text={label} />
        </span>
      ))}
    </div>
  );
}
