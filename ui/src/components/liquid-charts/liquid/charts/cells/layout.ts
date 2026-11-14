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
 * Pure layout maths for the cells family: whole-pixel grids, axis labels that thin out by
 * priority (never rotated or cut), 2D keyboard stepping, a tooltip that sits off the lit row and
 * column, the diagonal entrance sweep, and which cells carry a value label.
 */

import { placeLabels, type LabelMode } from "@/components/liquid-charts/liquid/core/labels";
import type { Insets, Rect } from "@/components/liquid-charts/liquid/core/geometry";
import { textWidth, TYPE } from "@/components/liquid-charts/liquid/core/format";

export type CellValue = number | null | undefined;

export const isValue = (value: unknown): value is number =>
  typeof value === "number" && Number.isFinite(value);

/* ——— Whole-pixel grids ——— */

/** The hairline between cells, whole px: wider for big cells, one px for tiny ones. */
export const cellGap = (size: number) => (size >= 30 ? 3 : size >= 11 ? 2 : 1);

/**
 * Splits `length` px from `start` into `count` cells with `gap` px between, every edge on a whole
 * px. Cells differ by at most 1px (the remainder is spread evenly), the first starts at `start`
 * and the last ends at `start + length`. The gap shrinks to keep cells at least as wide as it.
 */
export function splitWhole(
  start: number,
  length: number,
  count: number,
  gap: number,
): { starts: number[]; sizes: number[]; gap: number } {
  const s = Math.round(start),
    span = Math.max(0, Math.floor(length));
  if (count < 1) return { starts: [], sizes: [], gap };
  const g = Math.max(0, Math.min(Math.round(gap), Math.floor(span / count / 2)));
  const edge = (i: number) => s + Math.round((i * (span + g)) / count);
  const starts: number[] = [],
    sizes: number[] = [];
  for (let i = 0; i < count; i++) {
    starts.push(edge(i));
    sizes.push(Math.max(0, edge(i + 1) - edge(i) - g));
  }
  return { starts, sizes, gap: g };
}

/** The cell under `px` along one split axis: each cell owns half the gap on either side. */
export function indexAlong(
  split: { starts: readonly number[]; sizes: readonly number[]; gap: number },
  px: number,
): number | null {
  const n = split.starts.length;
  if (!n) return null;
  const half = split.gap / 2;
  if (px < split.starts[0] - half || px > split.starts[n - 1] + split.sizes[n - 1] + half)
    return null;
  for (let i = n - 1; i >= 0; i--) if (px >= split.starts[i] - half) return i;
  return 0;
}

export type Grid = {
  rows: number;
  columns: number;
  gap: number;
  xs: number[];
  widths: number[];
  ys: number[];
  heights: number[];
  /** The grid's outer rect. */
  bounds: Rect;
  rect: (row: number, column: number) => Rect;
  /** The cell under a point, or null outside the grid. */
  at: (x: number, y: number) => { row: number; column: number } | null;
};

/** A rows × columns grid filling `plot`, every edge on a whole px. */
export function gridGeometry({
  rows,
  columns,
  plot,
  gap,
}: {
  rows: number;
  columns: number;
  plot: Rect;
  gap?: number;
}): Grid {
  const width = Math.max(0, plot.right - plot.left),
    height = Math.max(0, plot.bottom - plot.top);
  const g = gap ?? cellGap(Math.min(width / Math.max(1, columns), height / Math.max(1, rows)) || 0);
  const across = splitWhole(plot.left, width, columns, g);
  const down = splitWhole(plot.top, height, rows, g);
  const used = Math.min(across.gap, down.gap);
  const x = across.gap === used ? across : splitWhole(plot.left, width, columns, used);
  const y = down.gap === used ? down : splitWhole(plot.top, height, rows, used);
  const bounds: Rect = {
    left: x.starts[0] ?? Math.round(plot.left),
    top: y.starts[0] ?? Math.round(plot.top),
    right: columns ? x.starts[columns - 1] + x.sizes[columns - 1] : Math.round(plot.left),
    bottom: rows ? y.starts[rows - 1] + y.sizes[rows - 1] : Math.round(plot.top),
  };
  return {
    rows,
    columns,
    gap: used,
    xs: x.starts,
    widths: x.sizes,
    ys: y.starts,
    heights: y.sizes,
    bounds,
    rect: (row, column) => ({
      left: x.starts[column],
      top: y.starts[row],
      right: x.starts[column] + x.sizes[column],
      bottom: y.starts[row] + y.sizes[row],
    }),
    at: (px, py) => {
      const column = indexAlong(x, px),
        row = indexAlong(y, py);
      return column === null || row === null ? null : { row, column };
    },
  };
}

/* ——— Heatmap frame ——— */

export type ColumnLabels = "top" | "bottom" | "none";

/** Insets that fit the row names on the left and the column names above or below. */
export function heatmapInsets({
  rows,
  columnLabels = "bottom",
  rowLabels = true,
}: {
  rows: readonly string[];
  columnLabels?: ColumnLabels;
  rowLabels?: boolean;
}): Insets {
  const widest = rowLabels
    ? rows.reduce((w, name) => Math.max(w, textWidth(name, TYPE.label)), 0)
    : 0;
  return {
    left: rowLabels && rows.length ? Math.ceil(Math.min(160, widest) + 12) : 2,
    right: 2,
    top: columnLabels === "top" ? 24 : 4,
    bottom: columnLabels === "bottom" ? 26 : 4,
  };
}

/**
 * The frame height that gives cells `aspect` (height over width) at this width, with the cell
 * height kept between `min` and `max` px.
 */
export function heatmapHeight({
  rows,
  columns,
  width,
  insets,
  aspect = 1,
  min = 14,
  max = 44,
}: {
  rows: number;
  columns: number;
  width: number;
  insets: Insets;
  aspect?: number;
  min?: number;
  max?: number;
}): number {
  const cellWidth = Math.max(0, width - insets.left - insets.right) / Math.max(1, columns);
  const cell = Math.max(min, Math.min(max, cellWidth * aspect));
  return Math.round(insets.top + insets.bottom + cell * Math.max(1, rows));
}

/** A cell's index in a row-major matrix, and back. */
export const cellIndex = (row: number, column: number, columns: number) => row * columns + column;
export const cellOf = (index: number, columns: number) => ({
  row: Math.floor(index / Math.max(1, columns)),
  column: index % Math.max(1, columns),
});

/** Long-form records as a matrix: rows and columns in first-seen order unless given; repeated
 * pairs add up; pairs with no record are null. */
export function toMatrix<T extends Record<string, unknown>>(
  records: readonly T[],
  rowKey: keyof T & string,
  columnKey: keyof T & string,
  valueKey: keyof T & string,
  order: { rows?: readonly string[]; columns?: readonly string[] } = {},
): { rows: string[]; columns: string[]; data: (number | null)[][] } {
  const rows = [...(order.rows ?? [])],
    columns = [...(order.columns ?? [])];
  if (!order.rows || !order.columns)
    for (const record of records) {
      const r = String(record[rowKey]),
        c = String(record[columnKey]);
      if (!order.rows && !rows.includes(r)) rows.push(r);
      if (!order.columns && !columns.includes(c)) columns.push(c);
    }
  const data = rows.map(() => columns.map((): number | null => null));
  for (const record of records) {
    const r = rows.indexOf(String(record[rowKey])),
      c = columns.indexOf(String(record[columnKey]));
    const v = record[valueKey];
    if (r < 0 || c < 0 || !isValue(v)) continue;
    data[r][c] = (data[r][c] ?? 0) + v;
  }
  return { rows, columns, data };
}

/* ——— Axis labels: thinned by priority, never rotated ——— */

/** Round label steps, smallest first: every label, every 2nd, 3rd, 4th, 6th, 12th… */
export const LABEL_STEPS = [1, 2, 3, 4, 6, 12, 24, 48, 96, 192] as const;

/**
 * Which of a run of labels along one axis to show, and where. Labels are tried at the smallest
 * round step at which they clear each other (counted from the first), so they stay evenly spaced;
 * the focused label always shows, and step labels it would touch give way to it. Labels slide
 * inside `[lo, hi]` rather than being cut. `centres` are the labels' wanted centres along the
 * axis, `extents` their lengths along it (text width on a horizontal axis, line height on a
 * vertical one). Returns each label's visibility and start (left or top) px.
 */
export function thinLabels({
  centres,
  extents,
  lo,
  hi,
  active = null,
  gap = 8,
  steps = LABEL_STEPS,
  priority,
}: {
  centres: readonly number[];
  extents: readonly number[];
  lo: number;
  hi: number;
  active?: number | null;
  gap?: number;
  steps?: readonly number[];
  /** Overrides the step: a label's own priority (higher wins collisions, ≤ 0 hides). */
  priority?: (index: number) => number;
}): { visible: boolean[]; starts: number[]; step: number } {
  const n = centres.length;
  const clears = (step: number) => {
    for (let i = 0; i + step < n; i += step) {
      const j = i + step;
      if (Math.abs(centres[j] - centres[i]) < (extents[i] + extents[j]) / 2 + gap) return false;
    }
    return true;
  };
  const step = steps.find(clears) ?? steps[steps.length - 1] ?? 1;
  const placed = placeLabels(
    centres.map((centre, i) => ({
      id: String(i),
      x: centre,
      y: 0,
      width: extents[i],
      height: 1,
      side: "center" as const,
      priority: active === i ? Infinity : priority ? priority(i) : i % step === 0 ? 2 : 0,
    })),
    { bounds: { left: lo, right: hi, top: -1, bottom: 1 }, gap },
  );
  return {
    visible: placed.map((label) => label.visible),
    starts: placed.map((label) => label.left),
    step,
  };
}

/* ——— Keyboard ——— */

/**
 * The next cell for a key in a rows × columns grid, row-major. Arrows move one cell and stop at
 * the edges; Home and End go to the row's ends (with Ctrl or Meta, the grid's); Page Up and Page
 * Down to the column's ends. From nothing in focus any move starts at the first cell. Escape
 * clears (null); other keys are not handled (undefined).
 */
export function step2D(
  index: number | null,
  key: string,
  rows: number,
  columns: number,
  { ctrl = false }: { ctrl?: boolean } = {},
): number | null | undefined {
  if (key === "Escape") return null;
  const handled = [
    "ArrowLeft",
    "ArrowRight",
    "ArrowUp",
    "ArrowDown",
    "Home",
    "End",
    "PageUp",
    "PageDown",
  ];
  if (!handled.includes(key)) return undefined;
  if (rows < 1 || columns < 1) return null;
  const last = rows * columns - 1;
  if (index === null || index < 0 || index > last) return key === "End" && ctrl ? last : 0;
  let row = Math.floor(index / columns),
    column = index % columns;
  if (key === "ArrowLeft") column = Math.max(0, column - 1);
  else if (key === "ArrowRight") column = Math.min(columns - 1, column + 1);
  else if (key === "ArrowUp") row = Math.max(0, row - 1);
  else if (key === "ArrowDown") row = Math.min(rows - 1, row + 1);
  else if (key === "Home") {
    column = 0;
    if (ctrl) row = 0;
  } else if (key === "End") {
    column = columns - 1;
    if (ctrl) row = rows - 1;
  } else if (key === "PageUp") row = 0;
  else if (key === "PageDown") row = rows - 1;
  return row * columns + column;
}

/* ——— Tooltip: off the lit row and column ——— */

const fitsIn = (x: number, y: number, w: number, h: number, b: Rect) =>
  x >= b.left - 1e-9 && y >= b.top - 1e-9 && x + w <= b.right + 1e-9 && y + h <= b.bottom + 1e-9;
const overlapArea = (x: number, y: number, w: number, h: number, r: Rect) =>
  Math.max(0, Math.min(x + w, r.right) - Math.max(x, r.left)) *
  Math.max(0, Math.min(y + h, r.bottom) - Math.max(y, r.top));

/**
 * Where a tooltip goes for a cell in a matrix: diagonally off its corner (above right, below
 * right, above left, below left), so it covers neither the cell nor the lit row and column. When
 * no corner fits, beside the cell (right, left, above, below); when nothing fits, wherever covers
 * least of the cell, inside `bounds`. Returns the card's top-left.
 */
export function quadrantTooltipPosition(
  anchor: Rect,
  size: { width: number; height: number },
  bounds: Rect,
  gap = 10,
): { x: number; y: number } {
  const { width: w, height: h } = size;
  const right = anchor.right + gap,
    left = anchor.left - gap - w,
    above = anchor.top - gap - h,
    below = anchor.bottom + gap;
  const midX = (anchor.left + anchor.right) / 2 - w / 2,
    midY = (anchor.top + anchor.bottom) / 2 - h / 2;
  const candidates: [number, number][] = [
    [right, above],
    [right, below],
    [left, above],
    [left, below],
    [right, midY],
    [left, midY],
    [midX, above],
    [midX, below],
  ];
  const clampX = (x: number) => Math.max(bounds.left, Math.min(bounds.right - w, x));
  const clampY = (y: number) => Math.max(bounds.top, Math.min(bounds.bottom - h, y));
  for (const [x, y] of candidates) if (fitsIn(x, y, w, h, bounds)) return { x, y };
  let best = { x: clampX(candidates[0][0]), y: clampY(candidates[0][1]) },
    least = Infinity;
  for (const [x, y] of candidates) {
    const cx = clampX(x),
      cy = clampY(y);
    const cover = overlapArea(cx, cy, w, h, anchor);
    if (cover < least - 1e-9) {
      least = cover;
      best = { x: cx, y: cy };
    }
  }
  return best;
}

/**
 * Of candidate top-left positions for a card of `size`, the one that covers the least of the
 * `avoid` rects (the marks in focus), each clamped inside `bounds` first; the earliest wins a tie.
 */
export function leastCover(
  candidates: readonly { x: number; y: number }[],
  size: { width: number; height: number },
  bounds: Rect,
  avoid: readonly Rect[],
): { x: number; y: number } {
  const { width: w, height: h } = size;
  let best = { x: bounds.left, y: bounds.top },
    least = Infinity;
  for (const c of candidates) {
    const x = Math.max(bounds.left, Math.min(bounds.right - w, c.x)),
      y = Math.max(bounds.top, Math.min(bounds.bottom - h, c.y));
    let cover = 0;
    for (const r of avoid) cover += overlapArea(x, y, w, h, r);
    if (cover < least - 1e-9) {
      least = cover;
      best = { x, y };
    }
  }
  return best;
}

/* ——— Motion ——— */

/** The diagonal entrance: a cell's delay, s, so the sweep runs from the top-left corner to the
 * bottom-right in `span` seconds. */
export function sweepDelay(
  row: number,
  column: number,
  rows: number,
  columns: number,
  span = 0.55,
) {
  const diagonal = Math.max(1, rows + columns - 2);
  return ((row + column) / diagonal) * span;
}

/* ——— Value labels in cells ——— */

/**
 * Which cells carry their value at rest: `auto` the highest and lowest, `all` every value
 * (larger first where they compete), `ends` the first and last values, `none` nothing. The cell
 * in focus always does. Missing values never do unless focused. Returns a priority per cell.
 */
export function cellLabelPriorities(
  values: readonly CellValue[],
  mode: LabelMode,
  active: number | null = null,
): number[] {
  const n = values.length;
  const priorities = new Array<number>(n).fill(0);
  if (mode === "all") {
    let peak = 0;
    for (const v of values) if (isValue(v)) peak = Math.max(peak, Math.abs(v));
    values.forEach((v, i) => {
      if (isValue(v)) priorities[i] = 1 + Math.abs(v) / (peak || 1);
    });
  } else if (mode === "auto") {
    let max = -1,
      min = -1;
    values.forEach((v, i) => {
      if (!isValue(v)) return;
      if (max < 0 || v > (values[max] as number)) max = i;
      if (min < 0 || v < (values[min] as number)) min = i;
    });
    if (max >= 0) priorities[max] = 3;
    if (min >= 0 && min !== max) priorities[min] = 2;
  } else if (mode === "ends") {
    const first = values.findIndex(isValue);
    let last = -1;
    for (let i = n - 1; i >= 0; i--)
      if (isValue(values[i])) {
        last = i;
        break;
      }
    if (first >= 0) priorities[first] = 1;
    if (last >= 0) priorities[last] = 1;
  }
  if (active !== null && active >= 0 && active < n) priorities[active] = Infinity;
  return priorities;
}

/** Words wrapped greedily into lines no wider than `maxWidth` px at `size` (a word wider than the
 * line keeps a line of its own). */
export function wrapLines(text: string, maxWidth: number, size: number = TYPE.label): string[] {
  const lines: string[] = [];
  for (const word of text.split(/\s+/).filter(Boolean)) {
    const last = lines[lines.length - 1];
    if (last !== undefined && textWidth(`${last} ${word}`, size) <= maxWidth)
      lines[lines.length - 1] = `${last} ${word}`;
    else lines.push(word);
  }
  return lines;
}

/** A bead's radius for a cell: the area follows the value's strength (0–1), from a small floor so
 * the faintest value is still a bead, up to the largest round mark the cell holds. */
export function beadRadius(strength: number, cell: { w: number; h: number }) {
  const max = Math.max(1, Math.min(cell.w, cell.h) / 2 - 1);
  const floor = Math.min(2, max * 0.2);
  const s = Math.max(0, Math.min(1, Math.abs(strength)));
  return floor + (max - floor) * Math.sqrt(s);
}

/** Whether a label of `text` fits inside a cell, with air round it. */
export function fitsInCell(rect: Rect, text: string, size: number = TYPE.micro, air = 4) {
  return (
    textWidth(text, size) + air * 2 <= rect.right - rect.left &&
    size * 1.15 + air <= rect.bottom - rect.top
  );
}
