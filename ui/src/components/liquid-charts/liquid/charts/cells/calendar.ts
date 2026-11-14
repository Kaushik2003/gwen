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
/**
 * Pure calendar maths for `CalendarHeatmap`: days as UTC midnights, weeks as columns starting on
 * Monday, month labels from the core's `timeTicks`, a layout that folds a year into two halves
 * (or scrolls inside its card) when it is narrow, keyboard steps by day and week, streaks, and the
 * outline round a range of days.
 */

import type { Rect } from "@/components/liquid-charts/liquid/core/geometry";
import { timeTicks } from "@/components/liquid-charts/liquid/core/scale";
import { isValue } from "./layout";

export const DAY = 86_400_000;

export type DayInput = string | number | Date;

/** A day as its UTC midnight (epoch ms). Strings are "YYYY-MM-DD"; numbers are epoch ms (floored
 * to their UTC day); a Date counts by its local calendar date, as people mean it. */
export function parseDay(day: DayInput): number {
  if (typeof day === "number") return Math.floor(day / DAY) * DAY;
  if (day instanceof Date) return Date.UTC(day.getFullYear(), day.getMonth(), day.getDate());
  const match = /^(\d{4})-(\d{2})-(\d{2})/.exec(day);
  if (match) return Date.UTC(Number(match[1]), Number(match[2]) - 1, Number(match[3]));
  const parsed = Date.parse(day);
  return Number.isFinite(parsed) ? Math.floor(parsed / DAY) * DAY : NaN;
}

/** "YYYY-MM-DD" for a UTC midnight. */
export const dayKey = (ms: number) => new Date(ms).toISOString().slice(0, 10);

/** Weekday with Monday as 0 and Sunday as 6. */
export const weekdayOf = (ms: number) => (new Date(ms).getUTCDay() + 6) % 7;

export type CalendarData =
  | readonly { date: DayInput; value: number | null | undefined }[]
  | Readonly<Record<string, number | null | undefined>>;

export type CalendarModel = {
  /** First and last day shown, UTC midnights, inclusive. */
  from: number;
  to: number;
  days: number;
  /** One value per day from `from`; null where there is no value. */
  values: (number | null)[];
  /** The first day's weekday (Monday 0): blank cells before it in the first week. */
  lead: number;
  weeks: number;
  week: (index: number) => number;
  weekday: (index: number) => number;
  /** The day in a week column and weekday row, or null outside the range. */
  dayAt: (week: number, weekday: number) => number | null;
  date: (index: number) => number;
  indexOf: (day: DayInput) => number | null;
};

const entries = (data: CalendarData): [number, unknown][] =>
  Array.isArray(data)
    ? (data as readonly { date: DayInput; value: unknown }[]).map((d) => [
        parseDay(d.date),
        d.value,
      ])
    : Object.entries(data).map(([key, value]) => [parseDay(key), value]);

/**
 * The days a calendar shows. `to` defaults to the latest day in the data; `from` to the first of
 * the month `months - 1` before `to` when `months` is given, else the day 52 weeks before (a year
 * of 365 days). Values on the same day add up; days with no value are null.
 */
export function calendarModel({
  data,
  from,
  to,
  months,
}: {
  data: CalendarData;
  from?: DayInput;
  to?: DayInput;
  months?: number;
}): CalendarModel {
  const known = entries(data).filter(([day]) => Number.isFinite(day));
  let end = to !== undefined ? parseDay(to) : NaN;
  if (!Number.isFinite(end))
    end = known.length
      ? Math.max(...known.map(([day]) => day))
      : from !== undefined
        ? parseDay(from) + 364 * DAY
        : 0;
  let start = from !== undefined ? parseDay(from) : NaN;
  if (!Number.isFinite(start)) {
    if (months && months > 0) {
      const d = new Date(end);
      start = Date.UTC(d.getUTCFullYear(), d.getUTCMonth() - (Math.round(months) - 1), 1);
    } else start = end - 364 * DAY;
  }
  if (start > end) [start, end] = [end, start];
  const days = Math.round((end - start) / DAY) + 1;
  const values: (number | null)[] = new Array(days).fill(null);
  for (const [day, value] of known) {
    const i = Math.round((day - start) / DAY);
    if (i < 0 || i >= days || !isValue(value)) continue;
    values[i] = (values[i] ?? 0) + value;
  }
  const lead = weekdayOf(start);
  const weeks = Math.ceil((lead + days) / 7);
  return {
    from: start,
    to: end,
    days,
    values,
    lead,
    weeks,
    week: (i) => Math.floor((i + lead) / 7),
    weekday: (i) => (i + lead) % 7,
    dayAt: (week, weekday) => {
      if (weekday < 0 || weekday > 6) return null;
      const i = week * 7 + weekday - lead;
      return i >= 0 && i < days ? i : null;
    },
    date: (i) => start + i * DAY,
    indexOf: (day) => {
      const i = Math.round((parseDay(day) - start) / DAY);
      return Number.isFinite(i) && i >= 0 && i < days ? i : null;
    },
  };
}

/* ——— Months ——— */

export type MonthTick = {
  /** The 1st of the month, UTC midnight. */
  value: number;
  /** Short month name ("Oct"). */
  label: string;
  year: number;
  month: number;
  /** The week column the label starts at: the 1st's week when most of that week is in the new
   * month (the 1st falls Monday to Thursday), else the next. */
  week: number;
};

/** Month starts inside the range, from the core's calendar-aligned `timeTicks`. */
export function monthTicks(model: CalendarModel): MonthTick[] {
  const span = model.to + DAY - 1 - model.from;
  const count = Math.max(1, Math.ceil(span / (30 * DAY)));
  return timeTicks([model.from, model.to + DAY - 1], count, { timeZone: "UTC", locale: "en-US" })
    .map((tick) => {
      const d = new Date(tick.value);
      const first = Math.round((tick.value - model.from) / DAY);
      const week = model.week(first) + (weekdayOf(tick.value) > 3 ? 1 : 0);
      return {
        value: tick.value,
        label: tick.label.replace(/\s*\d{4}$/, "").replace(/\.$/, ""),
        year: d.getUTCFullYear(),
        month: d.getUTCMonth(),
        week,
      };
    })
    .filter((tick) => tick.week < model.weeks);
}

/* ——— Layout ——— */

export type CalendarBand = {
  /** Week columns [from, to). */
  from: number;
  to: number;
  /** The band's first row, and its month labels' row, px. */
  top: number;
  labelTop: number;
};

export type CalendarLayout = {
  cell: number;
  gap: number;
  pitch: number;
  /** The first column's left edge, px (after the weekday labels). */
  left: number;
  bands: CalendarBand[];
  folded: boolean;
  /** The content is wider than the container: it scrolls inside its card. */
  scroll: boolean;
  /** Content size, px. */
  width: number;
  height: number;
  rect: (index: number) => Rect;
  /** The day under a point, or null (blank or outside). */
  at: (x: number, y: number) => number | null;
  /** The band a week column sits in. */
  bandOf: (week: number) => number;
};

export type CalendarFold = "auto" | "never" | "always";

const calendarGap = (cell: number) => (cell >= 15 ? 3 : cell >= 8 ? 2 : 1);

/** The largest whole-px square cell that fits `weeks` columns in `room` px, capped at `max`. */
function fitCell(room: number, weeks: number, max: number) {
  let cell = Math.floor(room / Math.max(1, weeks));
  for (let i = 0; i < 4; i++) {
    const gap = calendarGap(cell);
    cell = Math.min(max, Math.floor((room + gap) / Math.max(1, weeks)) - gap);
  }
  return Math.max(1, cell);
}

/** The week to fold at: the month start nearest the middle, so each half begins on a month. */
export function foldWeek(model: CalendarModel): number {
  const middle = Math.ceil(model.weeks / 2);
  const ticks = monthTicks(model).filter((t) => t.week > 0 && t.week < model.weeks);
  if (!ticks.length) return middle;
  return ticks.reduce((best, t) =>
    Math.abs(t.week - middle) < Math.abs(best.week - middle) ? t : best,
  ).week;
}

/**
 * Where every day goes at this width: one band of week columns while cells stay at least
 * `minCell` px, else (with `fold` auto) two bands, each starting on a month, else cells at
 * `minCell` that scroll inside the card. Cells are whole-px squares no larger than `maxCell`.
 */
export function calendarLayout({
  model,
  width,
  fold = "auto",
  left = 30,
  monthRow = 18,
  minCell = 9,
  maxCell = 18,
  bandGap = 16,
}: {
  model: CalendarModel;
  width: number;
  fold?: CalendarFold;
  left?: number;
  monthRow?: number;
  minCell?: number;
  maxCell?: number;
  bandGap?: number;
}): CalendarLayout {
  const room = Math.max(0, Math.floor(width - left));
  const single = fitCell(room, model.weeks, maxCell);
  const canFold = model.weeks > 1;
  const folded =
    canFold && (fold === "always" || (fold === "auto" && single < minCell && model.weeks > 8));
  const split = folded ? foldWeek(model) : model.weeks;
  const widest = folded ? Math.max(split, model.weeks - split) : model.weeks;
  let cell = folded ? fitCell(room, widest, maxCell) : single;
  let scroll = false;
  if (cell < minCell) {
    cell = minCell;
    scroll = true;
  }
  const gap = calendarGap(cell);
  const pitch = cell + gap;
  const bandHeight = 7 * pitch - gap;
  const bands: CalendarBand[] = [];
  const ranges = folded
    ? [
        [0, split],
        [split, model.weeks],
      ]
    : [[0, model.weeks]];
  let y = 0;
  for (const [from, to] of ranges) {
    bands.push({ from, to, labelTop: y, top: y + monthRow });
    y += monthRow + bandHeight + bandGap;
  }
  const height = y - bandGap;
  const bandOf = (week: number) => (folded && week >= split ? 1 : 0);
  const rect = (index: number): Rect => {
    const week = model.week(index),
      weekday = model.weekday(index);
    const band = bands[bandOf(week)];
    const x = left + (week - band.from) * pitch,
      top = band.top + weekday * pitch;
    return { left: x, top, right: x + cell, bottom: top + cell };
  };
  return {
    cell,
    gap,
    pitch,
    left,
    bands,
    folded,
    scroll,
    width: left + widest * pitch - gap,
    height,
    rect,
    bandOf,
    at: (x, py) => {
      for (const band of bands) {
        if (py < band.top - gap / 2 || py >= band.top + 7 * pitch - gap / 2) continue;
        const column = Math.floor((x - left + gap / 2) / pitch);
        if (column < 0 || column >= band.to - band.from) return null;
        const weekday = Math.floor((py - band.top + gap / 2) / pitch);
        return model.dayAt(band.from + column, weekday);
      }
      return null;
    },
  };
}

/* ——— Keyboard ——— */

/**
 * The next day for a key: Left and Right step a day, Up and Down a week, Home and End go to the
 * first and last day, Page Up and Page Down a month (four weeks). Moves stop at the ends. From
 * nothing in focus any move starts at the last day (the most recent). Escape clears (null);
 * other keys are not handled (undefined).
 */
export function calendarStep(
  index: number | null,
  key: string,
  days: number,
): number | null | undefined {
  if (key === "Escape") return null;
  const moves: Record<string, number> = {
    ArrowLeft: -1,
    ArrowRight: 1,
    ArrowUp: -7,
    ArrowDown: 7,
    PageUp: -28,
    PageDown: 28,
  };
  if (days < 1) return key in moves || key === "Home" || key === "End" ? null : undefined;
  if (key === "Home") return 0;
  if (key === "End") return days - 1;
  if (!(key in moves)) return undefined;
  if (index === null || index < 0 || index >= days) return days - 1;
  return Math.max(0, Math.min(days - 1, index + moves[key]));
}

/* ——— Streaks and ranges ——— */

export type Streak = { from: number; to: number; length: number };

/** The longest run of consecutive days that pass `test` (by default, a value above zero); the
 * most recent wins a tie. Indices are day offsets. Null when no day passes. */
export function longestStreak(
  values: readonly (number | null)[],
  test: (value: number) => boolean = (v) => v > 0,
): Streak | null {
  let best: Streak | null = null,
    start = -1;
  for (let i = 0; i <= values.length; i++) {
    const v = values[i];
    const pass = i < values.length && isValue(v) && test(v);
    if (pass && start < 0) start = i;
    if (!pass && start >= 0) {
      const length = i - start;
      if (!best || length >= best.length) best = { from: start, to: i - 1, length };
      start = -1;
    }
  }
  return best;
}

/** The run of passing days that ends on `at` (by default the last day), or null. */
export function currentStreak(
  values: readonly (number | null)[],
  at = values.length - 1,
  test: (value: number) => boolean = (v) => v > 0,
): Streak | null {
  let from = at;
  const pass = (i: number) => {
    const v = values[i];
    return isValue(v) && test(v);
  };
  if (at < 0 || at >= values.length || !pass(at)) return null;
  while (from > 0 && pass(from - 1)) from--;
  return { from, to: at, length: at - from + 1 };
}

export type Segment = { x1: number; y1: number; x2: number; y2: number };

/**
 * The outline round the days [from, to] (day offsets, inclusive): one segment along every side of
 * a day in the range whose neighbour on that side is not (or sits in the other band), set in the
 * middle of the gap, so the segments join into a stepped contour.
 */
export function rangeOutline(
  model: CalendarModel,
  layout: CalendarLayout,
  from: number,
  to: number,
): Segment[] {
  const lo = Math.max(0, Math.min(from, to)),
    hi = Math.min(model.days - 1, Math.max(from, to));
  const inside = (i: number | null) => i !== null && i >= lo && i <= hi;
  const half = layout.gap / 2;
  const out: Segment[] = [];
  for (let i = lo; i <= hi; i++) {
    const week = model.week(i),
      weekday = model.weekday(i);
    const r = layout.rect(i);
    const band = layout.bandOf(week);
    const sameBand = (w: number) => layout.bandOf(w) === band;
    const left = sameBand(week - 1) ? model.dayAt(week - 1, weekday) : null;
    const right = sameBand(week + 1) ? model.dayAt(week + 1, weekday) : null;
    const up = weekday > 0 ? model.dayAt(week, weekday - 1) : null;
    const down = weekday < 6 ? model.dayAt(week, weekday + 1) : null;
    const l = r.left - half,
      t = r.top - half,
      rr = r.right + half,
      b = r.bottom + half;
    if (!inside(up)) out.push({ x1: l, y1: t, x2: rr, y2: t });
    if (!inside(down)) out.push({ x1: l, y1: b, x2: rr, y2: b });
    if (!inside(left)) out.push({ x1: l, y1: t, x2: l, y2: b });
    if (!inside(right)) out.push({ x1: rr, y1: t, x2: rr, y2: b });
  }
  return out;
}
