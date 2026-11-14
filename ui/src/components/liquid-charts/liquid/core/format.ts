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
/**
 * Formatting and text estimates every chart needs before anything is measured (from the drivers
 * family, with the same tables the bar and area families keep):
 * - `textWidth` and `figureWidth` for Sofia Sans Semi Condensed, so insets fit up front.
 * - `formatValue` (plain text for screen readers and tables) and `tickFormatter` (axis labels).
 * - `valueTicks`, `niceStep` and `alignedDomain`: a second value scale whose ticks land on the
 *   first scale's gridlines, so a dual axis never needs a second set of lines.
 */

import { ticks, type Domain } from "./scale";
import type { FigureKind } from "./figure";
import { typeScale } from "./type-scale";
import { finite } from "./geometry";

export type ValueFormat = {
  kind: FigureKind;
  decimals?: number;
  signed?: boolean;
  /** A unit after the number ("h", "pts"), set small and quiet. */
  unit?: string;
  /** The ISO 4217 code a currency kind is in. Default USD. */
  currency?: string;
};

/* Advance widths in em for Sofia Sans Semi Condensed, measured in the browser and rounded up, so
 * estimates err towards room. */
const EM: Record<string, number> = {
  ",": 0.25,
  ".": 0.25,
  " ": 0.21,
  $: 0.6,
  "%": 0.9,
  "−": 0.58,
  "-": 0.47,
  "+": 0.58,
  "–": 0.5,
  I: 0.25,
  J: 0.26,
  M: 0.74,
  W: 0.82,
  f: 0.36,
  i: 0.23,
  j: 0.23,
  l: 0.23,
  m: 0.8,
  r: 0.35,
  t: 0.38,
  w: 0.7,
};
const emOf = (char: string) =>
  EM[char] ?? (/[0-9]/.test(char) ? 0.56 : /[A-Z]/.test(char) ? 0.6 : 0.52);

/** An estimate of a line of chart text's width, px, at `size` px. The frame sets the face (or its
 * fallback, sized to the same average advance) and no letter-spacing, so this holds on any page. */
export function textWidth(text: string, size: number): number {
  let em = 0;
  for (const char of text) em += emOf(char);
  return em * size;
}

/** An estimate of a `Figure`'s width: its currency sign and unit (%, K, M, B) are set at 0.7 em. */
export function figureWidth(text: string, size: number): number {
  let px = 0;
  for (const char of text) px += textWidth(char, /[$£€¥%KMB]/.test(char) ? size * 0.7 : size);
  return px;
}

/** Type sizes of the roles the axes and labels use, at the current type scale (type-scale.ts), so
 * estimates match what the roles draw. */
export const TYPE = {
  get micro() {
    return 11.5 * typeScale();
  },
  get caption() {
    return 12.5 * typeScale();
  },
  get label() {
    return 13 * typeScale();
  },
  get figure() {
    return 14 * typeScale();
  },
};

/** The line height every chart frame sets (core.module.css `.frame`), as a multiple of the size. */
export const LINE_HEIGHT = 1.5;
/** The height of one line of chart text at `size` px: the box a label takes in the DOM. */
export const lineBox = (size: number) => size * LINE_HEIGHT;

const formats = new Map<string, Intl.NumberFormat>();
/** An en-US `Intl.NumberFormat`, made once per set of options for the page: building one costs far
 * more than using it, and charts format values every frame. */
export function numberFormat(options: Intl.NumberFormatOptions): Intl.NumberFormat {
  const key = JSON.stringify(options);
  let format = formats.get(key);
  if (!format) formats.set(key, (format = new Intl.NumberFormat("en-US", options)));
  return format;
}

const intl = (format: ValueFormat, compact: boolean) => {
  const decimals = format.decimals ?? 0;
  const kind = format.kind;
  if (kind === "percent")
    return numberFormat({
      style: "percent",
      minimumFractionDigits: compact ? 0 : decimals,
      maximumFractionDigits: decimals,
    });
  const currency = kind === "currency" || kind === "currencyCompact";
  return numberFormat({
    ...(currency ? { style: "currency", currency: format.currency ?? "USD" } : {}),
    ...(compact || kind === "compact" || kind === "currencyCompact"
      ? // WebKit pads compact fractions ("$6.0K") unless the minimum is pinned.
        {
          notation: "compact",
          minimumFractionDigits: 0,
          maximumFractionDigits: compact ? 1 : decimals,
        }
      : { minimumFractionDigits: decimals, maximumFractionDigits: decimals }),
  });
};

/** A value as plain text for screen readers and the data table ("-$6.2K", "+12.5%", "7.5 h"). */
export function formatValue(value: number, format: ValueFormat): string {
  if (!finite(value)) return "no data";
  const text = intl(format, false).format(Math.abs(value));
  const sign = value < 0 ? "-" : format.signed && value > 0 ? "+" : "";
  return sign + text + (format.unit ? ` ${format.unit}` : "");
}

/**
 * Tick labels for a value axis: compact where the numbers are large ("$40K", "12K"), whole
 * percents, the unit when there is one, and a true minus. Zero keeps its currency sign.
 */
export function tickFormatter(format: ValueFormat, domain: Domain): (value: number) => string {
  const peak = Math.max(Math.abs(domain[0]), Math.abs(domain[1]));
  const span = Math.abs(domain[1] - domain[0]);
  const kind = format.kind;
  const compact =
    kind === "compact" ||
    kind === "currencyCompact" ||
    (kind === "currency" && peak >= 1000) ||
    (kind === "number" && peak >= 10000);
  // Small spans (a 1–5 scale, hours of a day) keep the decimals they need.
  const decimals = kind === "number" && span <= 5 ? Math.min(1, format.decimals ?? 0) : 0;
  const number =
    kind === "percent"
      ? numberFormat({
          style: "percent",
          maximumFractionDigits: span < 0.05 ? 1 : 0,
        })
      : intl({ kind, decimals, currency: format.currency }, compact);
  return (value) => {
    const text = number.format(Math.abs(value)) + (format.unit ? ` ${format.unit}` : "");
    return value < 0 ? `−${text}` : text;
  };
}

/** About `count` round ticks inside the domain, plus the domain's low end when it is below zero. */
export function valueTicks(domain: Domain, count = 4): number[] {
  const values = ticks(domain, count);
  if (domain[0] < 0 && !values.includes(domain[0])) values.unshift(domain[0]);
  return [...new Set(values)].sort((a, b) => a - b);
}

/** The smallest round step (1, 2, 2.5 or 5 × a power of ten) at least `raw`. Zero for none. */
export function niceStep(raw: number): number {
  if (!finite(raw) || raw <= 0) return 0;
  const power = 10 ** Math.floor(Math.log10(raw));
  for (const m of [1, 2, 2.5, 5, 10]) {
    const step = m * power;
    // Tolerate float noise just above a round step.
    if (step >= raw * (1 - 1e-9)) return step;
  }
  return 10 * power;
}

/**
 * A second scale whose ticks sit on the first scale's gridlines. `primary` is the first scale's
 * ticks (evenly spaced, ascending, its domain's ends included); the result holds `values` and
 * shares the first scale's zero line, so one set of gridlines serves both axes. With no finite
 * values it returns a 0–1 scale on the same lines.
 */
export function alignedDomain(
  values: readonly number[],
  primary: readonly number[],
): { domain: [number, number]; ticks: number[] } {
  const n = primary.length - 1;
  if (n < 1) return { domain: [0, 1], ticks: [0, 1] };
  const step = (primary[n] - primary[0]) / n;
  // How many intervals lie above and below zero on the first scale.
  const below = Math.max(0, Math.round(-primary[0] / step));
  const above = n - below;
  let hi = 0,
    lo = 0;
  for (const v of values) {
    if (!finite(v)) continue;
    hi = Math.max(hi, v);
    lo = Math.min(lo, v);
  }
  const need = Math.max(above > 0 ? hi / above : 0, below > 0 ? -lo / below : 0);
  // Values the first scale has no room for on their side still widen the step, so they fit.
  const overflow = (above === 0 && hi > 0) || (below === 0 && lo < 0);
  const s = niceStep(overflow ? Math.max(hi, -lo) : need) || 1 / Math.max(1, above);
  const ticks = Array.from({ length: n + 1 }, (_, i) => (i - below) * s);
  return { domain: [ticks[0], ticks[n]], ticks };
}

/**
 * Entrance timing, s, shared by every family so a page's charts arrive on one beat (the frame sets
 * the same values as CSS variables for CSS entrances: `--enter-delay`, `--enter-mark`,
 * `--enter-label`, `--enter-label-delay`, `--enter-settled`, `--enter-tooltip`):
 * - `delay` before the first mark moves; `mark` for one mark's entrance (a bar growing, a ring
 *   sweeping, a ribbon pouring); `small` for a small mark's (a bead, a dot, a cell).
 * - The marks stagger over at most `run` (`stagger`), so a chart lands by about
 *   `delay + run + mark`; a shape revealed in one sweep (a radial line) takes `run + mark`.
 * - A label shows once its mark is `labelAt` of the way in, and fades in over `label`.
 * - The tooltip appears over `tooltip` (`--enter-tooltip`).
 */
export const ENTER = {
  delay: 0.25,
  mark: 0.8,
  small: 0.6,
  run: 0.6,
  labelAt: 0.6,
  label: 0.3,
  tooltip: 0.16,
} as const;

/** Entrance stagger per slot, s: brisk for a few marks, the whole run under `ENTER.run`. */
export const stagger = (count: number) => Math.min(0.07, ENTER.run / Math.max(1, count));

/** When, s after the entrance starts, a mark that starts after `delay` (and takes `duration`) is
 * far enough in for its label: for labels that fade in by a CSS delay rather than per frame. */
export const labelDelay = (delay = 0, duration: number = ENTER.mark) =>
  delay + ENTER.delay + ENTER.labelAt * duration;

/** Whether a mark has come far enough in for its label (`ENTER.labelAt`), so labels arrive with
 * their marks rather than before them. Pass its entrance progress, 0 to 1 (a bead's growth), or a
 * bar's frame, whose liquid fills towards its level. */
export function arrived(progress: number | { fill: number; level?: number }): boolean {
  if (typeof progress === "number") return progress >= ENTER.labelAt;
  const level = progress.level ?? 1;
  return level <= 0 || progress.fill / level >= ENTER.labelAt;
}
