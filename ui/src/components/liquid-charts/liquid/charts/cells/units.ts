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
 * Pure maths for unit charts: how many units each category fills in a waffle (in order, the
 * total kept, a partial unit at the end of the filled run), where each square sits in the grid,
 * and unit columns (a waffle bar chart: units stacked per category, bottom up).
 */

import type { Rect } from "@/components/liquid-charts/liquid/core/geometry";
import { isValue } from "./layout";

export type UnitAllocation = {
  /** What one unit stands for. */
  unitValue: number;
  /** Per unit, in fill order: its category (−1 for an empty unit) and how full it is, 0–1. */
  units: { category: number; level: number }[];
  /** Units each category shows (a partial unit counts its level). */
  shown: number[];
};

const amount = (value: unknown) => (isValue(value) && value > 0 ? value : 0);

/**
 * Fills `units` squares with the categories in order. One unit stands for `total / units`
 * (`total` defaults to the values' sum, so they fill the grid). Boundaries between categories are
 * rounded cumulatively, which keeps the order and the total; the end of the filled run keeps its
 * fraction as a partly filled unit. Values past the total are cut at the grid's end.
 */
export function allocateUnits(
  values: readonly unknown[],
  { units, total }: { units: number; total?: number },
): UnitAllocation {
  const n = Math.max(0, Math.floor(units));
  const clean = values.map(amount);
  const sum = clean.reduce((a, b) => a + b, 0);
  const whole = total !== undefined && isValue(total) && total > 0 ? total : sum || 1;
  const unitValue = n ? whole / n : whole;
  const out: UnitAllocation = {
    unitValue,
    units: Array.from({ length: n }, () => ({ category: -1, level: 0 })),
    shown: clean.map(() => 0),
  };
  if (!n || !sum) return out;
  let last = -1;
  clean.forEach((v, k) => {
    if (v > 0) last = k;
  });
  let running = 0,
    start = 0;
  for (let k = 0; k <= last; k++) {
    running += clean[k];
    const edge = Math.min(n, running / unitValue);
    if (k < last) {
      const end = Math.max(start, Math.min(n, Math.round(edge)));
      for (let u = start; u < end; u++) out.units[u] = { category: k, level: 1 };
      out.shown[k] = end - start;
      start = end;
      continue;
    }
    let full = Math.floor(edge + 1e-9);
    let fraction = edge - full;
    if (fraction > 1 - 1e-9) {
      full += 1;
      fraction = 0;
    }
    if (full < start) {
      fraction = Math.max(0, edge - start);
      full = start;
    }
    for (let u = start; u < full; u++) out.units[u] = { category: k, level: 1 };
    out.shown[k] = full - start;
    if (fraction > 1e-6 && full < n) {
      out.units[full] = { category: k, level: fraction };
      out.shown[k] += fraction;
    }
  }
  return out;
}

/** The unit a category's tooltip anchors on: its last square, or the remainder's first (the
 * first unit not completely filled). −1 when there is none. */
export function anchorUnit(alloc: UnitAllocation, category: number | "remainder"): number {
  if (category === "remainder") return alloc.units.findIndex((u) => u.category < 0 || u.level < 1);
  let last = -1;
  alloc.units.forEach((u, i) => {
    if (u.category === category) last = i;
  });
  return last;
}

export type WaffleOrder = "rows" | "columns";

/** A unit's row and column: `rows` reads like text from the top left; `columns` fills each column
 * from the bottom, left to right, like a level rising. */
export function unitCell(unit: number, rows: number, columns: number, order: WaffleOrder = "rows") {
  if (order === "columns") {
    const column = Math.floor(unit / rows);
    return { row: rows - 1 - (unit % rows), column };
  }
  return { row: Math.floor(unit / columns), column: unit % columns };
}

export type WaffleGeometry = {
  size: number;
  gap: number;
  /** The grid's size, px. */
  width: number;
  height: number;
  /** A unit's square, relative to the grid's top left. */
  rect: (unit: number) => Rect;
  /** The unit under a point (relative to the grid), or null. */
  at: (x: number, y: number) => number | null;
};

/** The gap between glass units: enough for each square's rim to read. */
export const unitGap = (size: number) => Math.max(2, Math.min(6, Math.round(size * 0.16)));

/** Whole-px square units filling at most `width` × `height`, at most `maxSize` px each. */
export function waffleGeometry({
  rows,
  columns,
  width,
  height,
  order = "rows",
  maxSize = 40,
}: {
  rows: number;
  columns: number;
  width: number;
  height: number;
  order?: WaffleOrder;
  maxSize?: number;
}): WaffleGeometry {
  const r = Math.max(1, rows),
    c = Math.max(1, columns);
  let size = Math.floor(Math.min(width / c, height / r));
  let gap = unitGap(size);
  for (let i = 0; i < 3; i++) {
    size = Math.max(
      1,
      Math.min(maxSize, Math.floor(Math.min((width + gap) / c, (height + gap) / r) - gap)),
    );
    gap = unitGap(size);
  }
  const pitch = size + gap;
  return {
    size,
    gap,
    width: c * pitch - gap,
    height: r * pitch - gap,
    rect: (unit) => {
      const { row, column } = unitCell(unit, r, c, order);
      return {
        left: column * pitch,
        top: row * pitch,
        right: column * pitch + size,
        bottom: row * pitch + size,
      };
    },
    at: (x, y) => {
      const column = Math.floor((x + gap / 2) / pitch),
        row = Math.floor((y + gap / 2) / pitch);
      if (column < 0 || column >= c || row < 0 || row >= r) return null;
      return order === "columns" ? column * r + (r - 1 - row) : row * c + column;
    },
  };
}

/* ——— Unit columns ——— */

export type UnitColumn = {
  /** Units the actual value fills (fractional), and projected units after it (whole). */
  actual: number;
  projected: number;
};

/** Each category's units: `value / unit`, and its projected units rounded to whole squares. */
export function unitCounts(
  values: readonly unknown[],
  unit: number,
  projected: readonly unknown[] = [],
): UnitColumn[] {
  const per = isValue(unit) && unit > 0 ? unit : 1;
  return values.map((v, i) => ({
    actual: amount(v) / per,
    projected: Math.round(amount(projected[i]) / per),
  }));
}

/** The squares a column draws, bottom up: full units, the partial top unit (its level), then
 * projected units (etched). */
export function columnUnits(column: UnitColumn): { level: number; projected: boolean }[] {
  const out: { level: number; projected: boolean }[] = [];
  const full = Math.floor(column.actual + 1e-9);
  const fraction = column.actual - full;
  for (let k = 0; k < full; k++) out.push({ level: 1, projected: false });
  if (fraction > 1e-6) out.push({ level: fraction > 1 - 1e-9 ? 1 : fraction, projected: false });
  for (let k = 0; k < column.projected; k++) out.push({ level: 1, projected: true });
  return out;
}

export type UnitColumnsGeometry = {
  /** Units across each column. */
  across: number;
  size: number;
  gap: number;
  /** Each category's column: its left edge, width and centre, px. */
  columns: { left: number; width: number; centre: number }[];
  /** The k-th unit from the bottom of a category's column. */
  rect: (category: number, k: number) => Rect;
  /** The top of a category's column holding `units` squares. */
  top: (category: number, units: number) => number;
  /** The category whose column slot holds `x`, or null. */
  at: (x: number, y: number) => number | null;
};

/**
 * Where unit columns go in `plot`: every category a column `across` units wide (chosen, from 1 to
 * 8, to make the squares as large as the plot allows, unless given), units filling bottom up, row
 * by row. Columns are centred in equal slots; squares are whole px, at most `maxSize`.
 */
export function unitColumnsGeometry({
  counts,
  plot,
  across: fixed,
  maxSize = 28,
}: {
  /** Squares per category (see `columnUnits`). */
  counts: readonly number[];
  plot: Rect;
  across?: number;
  maxSize?: number;
}): UnitColumnsGeometry {
  const n = Math.max(1, counts.length);
  const tallest = Math.max(1, ...counts);
  const width = Math.max(0, plot.right - plot.left),
    height = Math.max(0, plot.bottom - plot.top);
  const fit = (across: number) => {
    let size = 12,
      gap = unitGap(size);
    for (let i = 0; i < 4; i++) {
      const slotGap = Math.max(12, Math.round(size * 1.1));
      const slot = (width - slotGap * (n - 1)) / n;
      const rows = Math.ceil(tallest / across);
      const byWidth = (slot + gap) / across - gap;
      const byHeight = (height + gap) / rows - gap;
      size = Math.max(1, Math.min(maxSize, Math.floor(Math.min(byWidth, byHeight))));
      gap = unitGap(size);
    }
    return { size, gap };
  };
  let across = fixed && fixed > 0 ? Math.round(fixed) : 1;
  let best = fit(across);
  if (!fixed)
    for (let a = 2; a <= 8; a++) {
      const next = fit(a);
      if (next.size > best.size) {
        best = next;
        across = a;
      }
    }
  const { size, gap } = best;
  const pitch = size + gap;
  const columnWidth = across * pitch - gap;
  const slot = width / n;
  const columns = Array.from({ length: n }, (_, i) => {
    const centre = plot.left + slot * (i + 0.5);
    const left = Math.round(centre - columnWidth / 2);
    return { left, width: columnWidth, centre: left + columnWidth / 2 };
  });
  const base = Math.round(plot.bottom);
  return {
    across,
    size,
    gap,
    columns,
    rect: (category, k) => {
      const row = Math.floor(k / across),
        col = k % across;
      const left = columns[category].left + col * pitch;
      const bottom = base - row * pitch;
      return { left, top: bottom - size, right: left + size, bottom };
    },
    top: (_, units) => base - Math.ceil(Math.max(0, units) / across) * pitch + gap,
    at: (x, y) => {
      if (y < plot.top - 8 || y > plot.bottom + 8) return null;
      const i = Math.floor((x - plot.left) / slot);
      return i >= 0 && i < counts.length ? i : null;
    },
  };
}
