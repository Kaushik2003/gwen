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
 * Scales, nice domains and ticks.
 *
 * Value axes use d3's tick logic (through `@visx/scale`) so domains and ticks land on the round
 * numbers people expect. The scales themselves are plain functions, cheap to create on every render
 * and safe to call every animation frame.
 */

import { scaleLinear } from "@visx/scale";

export type Domain = readonly [number, number];

/**
 * A nice value domain covering the values. includeZero (default true) keeps 0 inside, so bars
 * and filled areas start from it. Returns [0, 1] for empty input; widens a zero-width domain.
 * `count` (default 5) is the tick count the domain is rounded for, so its ends land on ticks.
 */
export function niceDomain(
  values: readonly number[],
  options: { includeZero?: boolean; count?: number } = {},
): [number, number] {
  const { includeZero = true, count = 5 } = options;
  let lo = Infinity;
  let hi = -Infinity;
  for (const v of values) {
    if (!Number.isFinite(v)) continue;
    if (v < lo) lo = v;
    if (v > hi) hi = v;
  }
  if (lo > hi) return [0, 1];
  if (includeZero) {
    lo = Math.min(lo, 0);
    hi = Math.max(hi, 0);
  }
  // A single value sits in the middle of a domain half its size either way; zero gets [0, 1].
  if (lo === hi) {
    if (lo === 0) return [0, 1];
    const pad = Math.abs(lo) / 2;
    lo -= pad;
    hi += pad;
  }
  const [a, b] = scaleLinear({ domain: [lo, hi], nice: count }).domain();
  return [a, b];
}

/** About `count` (default 5) round tick values inside the domain, including 0 when it lies inside. */
export function ticks(domain: Domain, count = 5): number[] {
  return scaleLinear({ domain: [domain[0], domain[1]] }).ticks(count);
}

export type Linear = ((value: number) => number) & {
  domain: Domain;
  range: Domain;
  invert: (px: number) => number;
};

/**
 * Maps a value domain onto a px range without clamping. A reversed range (a y axis, bottom to top)
 * works as is. A zero-width domain or range maps to the middle of the other, as d3 does.
 */
export function linear(domain: Domain, range: Domain): Linear {
  const [d0, d1] = domain;
  const [r0, r1] = range;
  const dSpan = d1 - d0;
  const rSpan = r1 - r0;
  return Object.assign((value: number) => r0 + (dSpan ? (value - d0) / dSpan : 0.5) * rSpan, {
    domain: [d0, d1] as const,
    range: [r0, r1] as const,
    invert: (px: number) => d0 + (rSpan ? (px - r0) / rSpan : 0.5) * dSpan,
  });
}

export type Band = {
  count: number;
  /** Px per slot. */
  step: number;
  /** Px per band after inner padding. */
  bandwidth: number;
  /** The band's start px. */
  start: (index: number) => number;
  /** The band's centre px. */
  centre: (index: number) => number;
  /** The index under a px position, or null outside the bands' extent. */
  indexAt: (px: number) => number | null;
};

/**
 * `count` equal bands across `range`, with d3 scaleBand's padding: paddingInner (default 0.2) is
 * the share of each step left between bands, paddingOuter (default 0.1) the share of a step left
 * before the first and after the last. A reversed range puts index 0 at the range's start.
 */
export function band(
  count: number,
  range: Domain,
  options: { paddingInner?: number; paddingOuter?: number } = {},
): Band {
  const { paddingInner = 0.2, paddingOuter = 0.1 } = options;
  const [r0, r1] = range;
  const reverse = r1 < r0;
  const lo = Math.min(r0, r1);
  const hi = Math.max(r0, r1);
  const step = (hi - lo) / Math.max(1, count - paddingInner + paddingOuter * 2);
  const first = lo + (hi - lo - step * (count - paddingInner)) / 2;
  const bandwidth = step * (1 - paddingInner);
  const slot = (index: number) => (reverse ? count - 1 - index : index);
  const start = (index: number) => first + step * slot(index);
  return {
    count,
    step,
    bandwidth,
    start,
    centre: (index) => start(index) + bandwidth / 2,
    // Each slot is a whole step centred on its band, so a pointer between two bars picks the
    // nearer one; the outer padding belongs to the end bands.
    indexAt: (px) => {
      if (count < 1 || hi === lo || px < lo || px > hi) return null;
      const i = Math.floor((px - first + (step - bandwidth) / 2) / step);
      return slot(Math.max(0, Math.min(count - 1, i)));
    },
  };
}

/**
 * Positions for `groups` side-by-side bars inside one band, separated by `gap` px (default 6).
 * Returns each sub-bar's centre offset from the band centre and the sub-bar thickness. In a narrow
 * band the gap shrinks so the bars keep at least half of the band between them.
 */
export function grouped(
  bandwidth: number,
  groups: number,
  gap = 6,
): { thickness: number; offset: (index: number) => number } {
  const n = Math.max(1, groups);
  const space = n > 1 ? Math.min(gap, bandwidth / (2 * (n - 1))) : 0;
  const thickness = Math.max(0, (bandwidth - space * (n - 1)) / n);
  return { thickness, offset: (index) => (index - (n - 1) / 2) * (thickness + space) };
}

export type TimeUnit = "minute" | "hour" | "day" | "week" | "month" | "quarter" | "year";

/**
 * Round time ticks across a span (epoch ms), with labels that suit the span: times of day within
 * a day (24-hour), days within a few months, months beyond. UTC unless `timeZone` says otherwise.
 * A forced `unit` steps by hours, days, weeks from Monday, months on the 1st, quarters ("Q3 2026")
 * or years, `every` units apart.
 */
export function timeTicks(
  [from, to]: Domain,
  count = 6,
  {
    timeZone = "UTC",
    locale = "en-GB",
    unit,
    every = 1,
    years,
  }: {
    timeZone?: string;
    locale?: string;
    /** Force the unit (a Gantt zoomed to weeks, an axis in quarters) instead of choosing one. */
    unit?: TimeUnit;
    /** With `unit`: ticks every this many units. */
    every?: number;
    /** Which month and quarter ticks name their year (when the label does not already show it).
     * By default months never do and quarters do on the first tick and each Q1; `"turn"` names it
     * where the year turns (January, Q1), `"every"` on every tick. */
    years?: "turn" | "every";
  } = {},
): { value: number; label: string }[] {
  const span = Math.abs(to - from);
  if (!Number.isFinite(span) || span <= 0) return [];
  const minute = 60_000,
    hour = 60 * minute,
    day = 24 * hour;
  const n = Math.max(1, Math.round(every));
  const forced: Record<TimeUnit, number> = {
    minute: minute * n,
    hour: hour * n,
    day: day * n,
    week: 7 * day * n,
    month: 30 * day * n,
    quarter: 91 * day * n,
    year: 365 * day * n,
  };
  const steps = [
    minute,
    5 * minute,
    15 * minute,
    30 * minute,
    hour,
    3 * hour,
    6 * hour,
    12 * hour,
    day,
    2 * day,
    7 * day,
    14 * day,
    30 * day,
    91 * day,
    182 * day,
    365 * day,
  ];
  const step = unit ? forced[unit] : (steps.find((s) => span / s <= count) ?? 365 * day);
  const format = new Intl.DateTimeFormat(
    locale,
    step < day
      ? { hour: "2-digit", minute: "2-digit", hourCycle: "h23", timeZone }
      : step < 30 * day
        ? { day: "numeric", month: "short", timeZone }
        : { month: "short", year: step >= 365 * day ? "numeric" : undefined, timeZone },
  );
  const lo = Math.min(from, to),
    hi = Math.max(from, to);
  const out: { value: number; label: string }[] = [];
  if (step >= 30 * day) {
    // Calendar months: ticks on the first of the month, every 1, 3, 6 or 12 months (UTC).
    const months = unit
      ? n * (unit === "quarter" ? 3 : unit === "year" ? 12 : 1)
      : Math.max(1, Math.round(step / (30 * day)));
    const d = new Date(lo);
    const y = d.getUTCFullYear();
    let m = d.getUTCMonth() + (Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), 1) < lo ? 1 : 0);
    m = Math.ceil(m / months) * months;
    for (let value = Date.UTC(y, m, 1); value <= hi; value = Date.UTC(y, (m += months), 1)) {
      const at = new Date(value);
      const turn = at.getUTCMonth() === 0;
      const year = ` ${at.getUTCFullYear()}`;
      const label =
        unit === "quarter"
          ? `Q${Math.floor(at.getUTCMonth() / 3) + 1}${years === "every" || turn || (!years && out.length === 0) ? year : ""}`
          : unit === "year"
            ? String(at.getUTCFullYear())
            : `${format.format(at)}${step < 365 * day && (years === "every" || (years === "turn" && turn)) ? year : ""}`;
      out.push({ value, label });
    }
    return out;
  }
  // Weeks start on Monday; days and shorter steps on round multiples of the step.
  let first = Math.ceil(lo / step) * step;
  if (step % (7 * day) === 0) {
    const midnight = Math.ceil(lo / day) * day;
    const weekday = (new Date(midnight).getUTCDay() + 6) % 7;
    first = midnight + ((7 - weekday) % 7) * day;
  }
  for (let value = first; value <= hi; value += step)
    out.push({ value, label: format.format(new Date(value)) });
  return out;
}
