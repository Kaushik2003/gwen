"use client";
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

import {
  ChartFrame,
  ChartScope,
  ChartState,
  ChartTooltip,
  ENTER,
  formatValue,
  labelDelay,
  mutedText,
  noop,
  plotRect,
  resolveStyle,
  textWidth,
  TooltipFoot,
  TYPE,
  useActive,
  useChartFrame,
  type BarMark,
  type ChartConfig,
  type Density,
  type DotMark,
  type Insets,
  type LabelMode,
  type Theme,
  type ValueFormat,
} from "@/components/liquid-charts/liquid/core";
import {
  cellGap,
  cellIndex,
  cellLabelPriorities,
  cellOf,
  fitsInCell,
  gridGeometry,
  heatmapHeight,
  heatmapInsets,
  isValue,
  quadrantTooltipPosition,
  step2D,
  sweepDelay,
  thinLabels,
  type CellValue,
  type ColumnLabels,
  type Grid,
} from "./layout";
import {
  CellKey,
  CellLayer,
  Fig,
  FlatCellLayer,
  placeAt,
  plain,
  UnitLayer,
  useIsomorphicLayoutEffect,
  useStyleWarnings,
  useWidth,
} from "./cell-parts";
import {
  colourScale,
  configColours,
  cssRgb,
  inkOn,
  scaleKeys,
  type ColourScale,
  type HeatScale,
} from "@/components/liquid-charts/liquid/core/colour-scale";
import { useRef, type CSSProperties, type KeyboardEvent, type ReactNode } from "react";
import { GradientLegend, strengthCss } from "@/components/liquid-charts/liquid/core/gradient-legend";
import { usePageTheme } from "@/components/liquid-charts/liquid/core/page-theme";
import { cssModule } from "@/components/liquid-charts/liquid/core/css-module";
import Roles from "@/components/liquid-charts/liquid/core/type.module.css";
import { LiquidCredit } from "@/components/liquid-charts/liquid/credit";
import { beadRadius, DotCells } from "./dot-cells";
import type { FlatCell } from "./cell-canvas";
import Styles from "./cells.module.css";

const roles = cssModule(Roles, "lc-type-");
const styles = cssModule(Styles, "lc-cells-");

export type HeatmapShape = "rect" | "circle";

export type HeatmapProps = {
  /** Values by row, then column: `data[row][column]`. Null or missing is "no data". */
  data: readonly (readonly CellValue[])[];
  /** Row names, top to bottom, and column names, left to right. */
  rows: readonly string[];
  columns: readonly string[];
  /** Longer names for the tooltip and screen readers ("Tuesday" for "Tue"). */
  rowTitle?: (row: number) => string;
  columnTitle?: (column: number) => string;
  /** What a row and a column are, for the data table ("Day", "Hour"). */
  rowName?: string;
  columnName?: string;
  /** Colours. The scale's key (or two keys, for a diverging scale) name the stops. */
  config: ChartConfig;
  scale?: HeatScale;
  /** For a diverging scale: whether values above the midpoint are good news ("up", the default)
   * or bad ("down", for error rates and latency). Colours the tooltip's note. */
  better?: "up" | "down";
  /** What the midpoint means ("SLO"), in the legend and tooltip. */
  midpointLabel?: string;
  /** `rect` cells coloured by value (flat), or `circle`: glass beads sized by value. */
  shape?: HeatmapShape;
  /** By default (`liquid`) each cell is a pool of liquid in its exact scale colour, lit only at
   * its surface and walls. `glass` opts small matrices into glass cells whose liquid level is the
   * value (the colour then only says which side of a diverging midpoint it is on); `flat` draws
   * plain flat cells. */
  material?: "liquid" | "flat" | "glass";
  /** Liquid cells: `brim` (default) fills every pool to the brim, so the colour alone is the
   * value; `level` also fills each pool to its value's strength (distance from the midpoint on a
   * diverging scale), telling it twice. Best on sequential scales. */
  fill?: "brim" | "level";
  /** Cells drawn etched, with frosted lanes in their liquid: values outside a target, projected
   * or incomplete. Etching carries meaning; it is never decoration. */
  etched?: (value: number, row: number, column: number) => boolean;
  /** What etched cells mean, for the legend ("Over SLO"). */
  etchedLabel?: ReactNode;
  /** Light or dark. Without one, the page's: `data-theme` or a `dark` class on `<html>`, else the system setting. @default the page's */
  theme?: Theme;
  /** Fixed frame height; by default cells take `aspect` at the chart's width. */
  height?: number;
  /** Cell height over width when the height is automatic. Default 1 (square). */
  aspect?: number;
  density?: Density;
  /** The cell in focus as a row-major index (`row * columns.length + column`). */
  active?: number | null;
  onActiveChange?: (index: number | null) => void;
  /** Values printed in cells that fit them: auto (highest and lowest), all, ends, none. */
  labels?: LabelMode;
  columnLabels?: ColumnLabels;
  tooltip?: boolean;
  /** A note under the tooltip's value for a cell. */
  tooltipFoot?: (row: number, column: number, value: number | null) => ReactNode;
  format?: ValueFormat;
  /** The measure’s name in the tooltip and legend. Default: the sequential key’s label ("Value" for a diverging scale, whose keys name its sides). */
  valueName?: string;
  legend?: boolean;
  loading?: boolean;
  empty?: ReactNode;
  error?: ReactNode;
  replay?: number;
  label: string;
  describe?: (index: number) => string;
  className?: string;
};

/** Cell height bounds per density when the height follows the width. */
const CELLS: Record<Density, { min: number; max: number }> = {
  tile: { min: 10, max: 22 },
  standard: { min: 14, max: 44 },
  hero: { min: 18, max: 60 },
};

/**
 * A rows × columns matrix of cells whose colour is the value (style rule 1): each cell a small
 * pool of liquid in its exact colour on a sequential or diverging scale from `ChartConfig`, lit
 * only at its surface and walls, with hairline gaps. Hovering lights the cell's row and column and
 * dims the rest towards graphite; the arrows step in two dimensions; the legend's marker follows
 * the value in focus. Missing values are etched empty glass; `etched` cells (outside a target,
 * projected) carry frosted lanes in their liquid. `material="glass"` and `shape="circle"` trade
 * colour for a liquid level or a bead's size.
 */
export function Heatmap({
  data,
  rows,
  columns,
  rowTitle,
  columnTitle,
  rowName = "Row",
  columnName = "Column",
  config,
  scale = { kind: "sequential" },
  better = "up",
  midpointLabel,
  shape = "rect",
  material,
  fill = "brim",
  etched,
  etchedLabel,
  theme: themeProp,
  height,
  aspect = 1,
  density = "standard",
  active: activeProp,
  onActiveChange,
  labels = "auto",
  columnLabels = "bottom",
  tooltip = true,
  tooltipFoot,
  format = { kind: "number" },
  valueName,
  legend = true,
  loading = false,
  empty,
  error,
  replay = 0,
  label,
  describe,
  className,
}: HeatmapProps) {
  const theme = usePageTheme(themeProp);
  const [active, setActive] = useActive(activeProp, onActiveChange);
  const [outer, width] = useWidth();
  const nRows = rows.length,
    nCols = columns.length,
    count = nRows * nCols;
  const values: (number | null)[] = Array.from({ length: count }, (_, i) => {
    const { row, column } = cellOf(i, nCols);
    const v = data[row]?.[column];
    return isValue(v) ? v : null;
  });
  const keys = scaleKeys(scale, config);
  const colours = colourScale({
    scale,
    values,
    low: scale.kind === "diverging" ? configColours(config, keys[0], theme) : [],
    high: configColours(config, keys[scale.kind === "diverging" ? 1 : 0], theme),
    theme,
  });
  // A diverging scale's keys name its sides, not the measure.
  const measure =
    valueName ?? (scale.kind === "diverging" ? "Value" : plain(config[keys[0]]?.label, "Value"));

  const insets: Insets = heatmapInsets({ rows, columnLabels });
  const caps = CELLS[density];
  const frameHeight =
    height ??
    heatmapHeight({
      rows: nRows,
      columns: nCols,
      width: width || 560,
      insets,
      aspect,
      min: caps.min,
      max: caps.max,
    });
  const grid = (w: number, h: number) =>
    gridGeometry({ rows: nRows, columns: nCols, plot: plotRect({ width: w, height: h }, insets) });

  const circle = shape === "circle";
  const approx = width
    ? Math.min(
        (width - insets.left - insets.right) / Math.max(1, nCols),
        (frameHeight - insets.top - insets.bottom) / Math.max(1, nRows),
      )
    : undefined;
  const style = resolveStyle({
    geometry: "cartesian",
    marks: [circle ? "point" : "cell"],
    colour: circle ? undefined : scale.kind,
    markCount: count,
    smallestMark: approx,
    material,
  });
  useStyleWarnings(style.warnings, "Heatmap");
  const glass = !circle && style.materials.cell === "glass";
  const liquid = !circle && style.materials.cell === "liquid";

  const focus = active !== null && active >= 0 && active < count ? cellOf(active, nCols) : null;
  const lit = (row: number, column: number) =>
    focus === null || row === focus.row || column === focus.column ? 1 : 0;
  const delay = (row: number, column: number) => sweepDelay(row, column, nRows, nCols);
  const missingTint = colours.tint(colours.domain[1]);

  /** A level never empties a valued pool: a thin film keeps its colour in view. */
  const levelOf = (v: number) => 0.08 + 0.92 * Math.min(1, Math.abs(colours.intensity(v)));
  const flatLayout = (w: number, h: number): FlatCell[] => {
    const g = grid(w, h);
    return values.map((v, i) => {
      const { row, column } = cellOf(i, nCols);
      const r = g.rect(row, column);
      const cw = r.right - r.left,
        ch = r.bottom - r.top;
      return {
        key: `c${i}`,
        x: r.left,
        y: r.top,
        w: cw,
        h: ch,
        colour: v === null ? null : circle ? colours.tint(v) : colours.colour(v),
        level: v === null || fill !== "level" ? 1 : levelOf(v),
        etched: liquid && v !== null && !!etched?.(v, row, column),
        lit: lit(row, column),
        delay: delay(row, column),
        focus: active === i,
        grow: Math.min(cw, ch) >= 16 ? g.gap + 1 : g.gap,
        circle:
          circle && v !== null
            ? beadRadius(colours.intensity(v), { w: cw, h: ch }) / (Math.min(cw, ch) / 2)
            : undefined,
      };
    });
  };
  const glassLayout = (w: number, h: number): BarMark[] => {
    const g = grid(w, h);
    return values.map((v, i) => {
      const { row, column } = cellOf(i, nCols);
      const r = g.rect(row, column);
      return {
        key: `c${i}`,
        cross: (r.left + r.right) / 2,
        base: r.bottom,
        thickness: r.right - r.left,
        length: r.bottom - r.top,
        direction: "up",
        tint: v === null ? missingTint : colours.tint(v),
        lit: lit(row, column),
        level: v === null ? 0 : Math.abs(colours.intensity(v)),
        pattern: v === null ? true : undefined,
        delay: delay(row, column),
      };
    });
  };
  const dotLayout = (w: number, h: number): DotMark[] => {
    const g = grid(w, h);
    return values.map((v, i) => {
      const { row, column } = cellOf(i, nCols);
      const r = g.rect(row, column);
      const cell = { w: r.right - r.left, h: r.bottom - r.top };
      const radius = v === null ? beadRadius(0.12, cell) : beadRadius(colours.intensity(v), cell);
      return {
        key: `c${i}`,
        x: (r.left + r.right) / 2,
        y: (r.top + r.bottom) / 2,
        r: active === i ? Math.min(radius * 1.12, cell.w / 2, cell.h / 2) : radius,
        tint: v === null ? missingTint : colours.tint(v),
        lit: lit(row, column),
        level: v === null ? 0 : 1,
        pattern: v === null ? true : undefined,
        delay: delay(row, column),
      };
    });
  };

  const rowLong = (row: number) => rowTitle?.(row) ?? rows[row] ?? "";
  const columnLong = (column: number) => columnTitle?.(column) ?? columns[column] ?? "";
  const formatted = (v: number | null) => (v === null ? "no data" : formatValue(v, format));
  const describeCell = (index: number) => {
    const { row, column } = cellOf(index, nCols);
    return `${rowLong(row)}, ${columnLong(column)}: ${measure} ${formatted(values[index])}.`;
  };

  const anyValue = values.some((v) => v !== null);
  const state = error ? "error" : loading ? "loading" : !count || !anyValue ? "empty" : null;
  const wrapperClass = `${styles.root} ${className ?? ""}`;

  if (state)
    return (
      <ChartScope theme={theme} className={wrapperClass}>
        <div ref={outer}>
          <ChartFrame
            credit={<LiquidCredit />}
            theme={theme}
            height={frameHeight}
            insets={insets}
            count={1}
            active={null}
            onActiveChange={noop}
            pick={() => null}
            label={label}
          >
            {state === "loading" ? (
              <ChartState
                kind="loading"
                skeleton={{
                  grid: [Math.max(1, Math.min(nRows, 8)), Math.max(1, Math.min(nCols, 24))],
                }}
              />
            ) : (
              <ChartState
                kind={state}
                message={state === "error" ? (error === true ? undefined : error) : empty}
              />
            )}
          </ChartFrame>
        </div>
      </ChartScope>
    );

  const onKeyDownCapture = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Escape") return;
    const next = step2D(active, event.key, nRows, nCols, { ctrl: event.ctrlKey || event.metaKey });
    if (next === undefined) return;
    event.preventDefault();
    event.stopPropagation();
    if (next !== active) setActive(next);
  };

  const priorities = cellLabelPriorities(values, labels, tooltip ? null : active);
  const activeValue = active !== null && active >= 0 && active < count ? values[active] : null;

  return (
    <ChartScope theme={theme} className={wrapperClass}>
      <div ref={outer} onKeyDownCapture={onKeyDownCapture}>
        <ChartFrame
          credit={<LiquidCredit />}
          theme={theme}
          height={frameHeight}
          insets={insets}
          count={count}
          active={active}
          onActiveChange={setActive}
          pick={(x, y, size) => {
            const hit = grid(size.width, size.height).at(x, y);
            return hit ? cellIndex(hit.row, hit.column, nCols) : null;
          }}
          label={label}
          describe={describe ?? describeCell}
          table={{
            caption: label,
            columns: [`${rowName} / ${columnName}`, ...columns.map((_, c) => columnLong(c))],
            rows: rows.map((_, r) => [
              rowLong(r),
              ...columns.map((_, c) => formatted(values[cellIndex(r, c, nCols)])),
            ]),
          }}
        >
          {circle ? (
            <DotCells theme={theme} replay={replay} layout={dotLayout} flat={flatLayout} />
          ) : glass ? (
            <UnitLayer theme={theme} replay={replay} material="glass" layout={glassLayout} />
          ) : liquid ? (
            <CellLayer theme={theme} replay={replay} layout={flatLayout} />
          ) : (
            <FlatCellLayer theme={theme} replay={replay} layout={flatLayout} />
          )}
          <HeatAxes
            grid={grid}
            rows={rows}
            columns={columns}
            focus={focus}
            columnLabels={columnLabels}
          />
          {!circle && (
            <CellValues
              key={replay}
              grid={grid}
              values={values}
              colours={colours}
              glass={glass}
              priorities={priorities}
              format={format}
              columns={nCols}
              lit={lit}
            />
          )}
          {tooltip && active !== null && focus !== null && (
            <CellTooltip
              grid={grid}
              row={focus.row}
              column={focus.column}
              title={`${rowLong(focus.row)} · ${columnLong(focus.column)}`}
              measure={measure}
              value={values[active]}
              colour={
                values[active] === null
                  ? null
                  : cssRgb(
                      glass || circle
                        ? colours.tint(values[active]!)
                        : colours.colour(values[active]!),
                    )
              }
              format={format}
              foot={tooltipFoot?.(focus.row, focus.column, values[active])}
              colours={colours}
              values={values}
              better={better}
              midpointLabel={midpointLabel}
            />
          )}
        </ChartFrame>
      </div>
      {legend && (
        <div className={styles.legendRow} style={{ paddingLeft: Math.max(0, insets.left - 2) }}>
          <GradientLegend
            scale={colours}
            format={format}
            name={measure}
            value={activeValue}
            midLabel={midpointLabel}
            missing={values.some((v) => v === null)}
            background={glass || circle ? strengthBar(colours) : undefined}
            compact={density === "tile"}
            extra={
              etchedLabel && etched && liquid ? (
                <CellKey mark="lanes" colour={etchedColour(values, colours, etched, nCols)}>
                  {etchedLabel}
                </CellKey>
              ) : undefined
            }
          />
        </div>
      )}
    </ChartScope>
  );
}

/** The colour of the strongest etched cell, for its legend key. */
function etchedColour(
  values: readonly (number | null)[],
  colours: ColourScale,
  etched: (value: number, row: number, column: number) => boolean,
  columns: number,
) {
  let best: number | null = null;
  values.forEach((v, i) => {
    const { row, column } = cellOf(i, columns);
    if (
      v !== null &&
      etched(v, row, column) &&
      (best === null || Math.abs(colours.intensity(v)) > Math.abs(colours.intensity(best)))
    )
      best = v;
  });
  return best === null ? undefined : colours.colour(best);
}

/** For glass and beads the colour is constant per side; the legend bar shows strength instead. */
function strengthBar(colours: ColourScale) {
  const [lo, hi] = colours.domain;
  if (colours.kind === "diverging" && colours.midpoint !== null) {
    const at = colours.offset(colours.midpoint) * 100;
    return `linear-gradient(90deg, ${cssRgb(colours.tint(lo))}, ${cssRgb(colours.tint(lo), 0.08)} ${at}%, ${cssRgb(colours.tint(hi), 0.08)} ${at}%, ${cssRgb(colours.tint(hi))})`;
  }
  return strengthCss(colours.tint(hi), 0.08);
}

/* ——— Row and column names, thinned by priority ——— */

function HeatAxes({
  grid,
  rows,
  columns,
  focus,
  columnLabels,
}: {
  grid: (width: number, height: number) => Grid;
  rows: readonly string[];
  columns: readonly string[];
  focus: { row: number; column: number } | null;
  columnLabels: ColumnLabels;
}) {
  const { size } = useChartFrame();
  if (!size.width) return null;
  const g = grid(size.width, size.height);
  const columnThin = thinLabels({
    centres: columns.map((_, c) => g.xs[c] + g.widths[c] / 2),
    extents: columns.map((name) => textWidth(name, TYPE.micro) + 2),
    lo: 0,
    hi: size.width,
    active: focus?.column ?? null,
    gap: 6,
  });
  const lineHeight = 15;
  const rowThin = thinLabels({
    centres: rows.map((_, r) => g.ys[r] + g.heights[r] / 2),
    extents: rows.map(() => lineHeight),
    lo: 0,
    hi: size.height,
    active: focus?.row ?? null,
    gap: 1,
  });
  return (
    <div className={styles.axes} aria-hidden="true">
      {columnLabels !== "none" &&
        columns.map((name, c) => (
          <span
            key={`c${c}`}
            className={`${styles.columnLabel} ${roles.micro}`}
            data-shown={columnThin.visible[c]}
            data-active={focus?.column === c}
            data-dim={focus !== null && focus.column !== c}
            style={{
              left: columnThin.starts[c],
              top: columnLabels === "top" ? Math.max(0, g.bounds.top - 21) : g.bounds.bottom + 7,
            }}
          >
            {name}
          </span>
        ))}
      {rows.map((name, r) => (
        <span
          key={`r${r}`}
          className={`${styles.rowLabel} ${roles.label}`}
          data-shown={rowThin.visible[r]}
          data-active={focus?.row === r}
          data-dim={focus !== null && focus.row !== r}
          style={{
            right: size.width - g.bounds.left + 10,
            top: rowThin.starts[r] + lineHeight / 2,
          }}
        >
          {name}
        </span>
      ))}
    </div>
  );
}

/* ——— Values inside the cells that fit them ——— */

function CellValues({
  grid,
  values,
  colours,
  glass,
  priorities,
  format,
  columns,
  lit,
}: {
  grid: (width: number, height: number) => Grid;
  values: readonly (number | null)[];
  colours: ColourScale;
  glass: boolean;
  priorities: readonly number[];
  format: ValueFormat;
  columns: number;
  lit: (row: number, column: number) => number;
}) {
  const { size, theme } = useChartFrame();
  if (!size.width) return null;
  const g = grid(size.width, size.height);
  const rows = Math.max(1, values.length / Math.max(1, columns));
  return (
    <>
      {values.map((v, i) => {
        if (v === null || !priorities[i]) return null;
        const { row, column } = cellOf(i, columns);
        const r = g.rect(row, column);
        if (!fitsInCell(r, formatValue(v, format), TYPE.micro)) return null;
        const ink = glass ? (theme === "dark" ? "light" : "dark") : inkOn(colours.colour(v));
        return (
          <span
            key={i}
            className={`${styles.cellValue} ${roles.micro}`}
            data-ink={ink}
            data-dim={lit(row, column) === 0}
            style={
              {
                left: (r.left + r.right) / 2,
                top: (r.top + r.bottom) / 2,
                "--delay": `${Math.round(labelDelay(sweepDelay(row, column, rows, columns), ENTER.small) * 1000)}ms`,
              } as CSSProperties
            }
            aria-hidden="true"
          >
            <Fig value={v} format={format} className={roles.micro} />
          </span>
        );
      })}
    </>
  );
}

/* ——— Tooltip: off the cell's corner, clear of its row and column ——— */

function CellTooltip({
  grid,
  row,
  column,
  title,
  measure,
  value,
  colour,
  format,
  foot,
  colours,
  values,
  better,
  midpointLabel,
}: {
  grid: (width: number, height: number) => Grid;
  row: number;
  column: number;
  title: string;
  measure: string;
  value: number | null;
  colour: string | null;
  format: ValueFormat;
  foot?: ReactNode;
  colours: ColourScale;
  values: readonly (number | null)[];
  better: "up" | "down";
  midpointLabel?: string;
}) {
  const tip = useRef<HTMLDivElement>(null);
  const { size } = useChartFrame();
  useIsomorphicLayoutEffect(() => {
    const element = tip.current;
    if (!element || !size.width) return;
    const g = grid(size.width, size.height);
    const r = g.rect(row, column);
    const grow = cellGap(Math.min(r.right - r.left, r.bottom - r.top)) + 1;
    placeAt(
      element,
      quadrantTooltipPosition(
        { left: r.left - grow, top: r.top - grow, right: r.right + grow, bottom: r.bottom + grow },
        { width: element.offsetWidth, height: element.offsetHeight },
        { left: 2, top: 2, right: size.width - 2, bottom: size.height - 2 },
      ),
    );
  });
  let note: ReactNode = foot;
  if (note === undefined && value !== null) {
    if (colours.kind === "diverging" && colours.midpoint !== null) {
      const diff = value - colours.midpoint;
      const good = diff === 0 || diff > 0 === (better === "up");
      note = (
        <span className={`${roles.caption} ${styles.tipNote}`} data-good={good}>
          {diff === 0 ? (
            <>At {midpointLabel ?? "the midpoint"}</>
          ) : (
            <>
              <Fig value={Math.abs(diff)} format={format} signed={false} className="" />{" "}
              {diff > 0 ? "above" : "below"} {midpointLabel ?? "the midpoint"}
            </>
          )}
        </span>
      );
    } else {
      const finite = values.filter((v): v is number => v !== null);
      const max = Math.max(...finite),
        min = Math.min(...finite);
      if (finite.length > 2 && value === max) note = <TooltipFoot>Highest</TooltipFoot>;
      else if (finite.length > 2 && value === min) note = <TooltipFoot>Lowest</TooltipFoot>;
    }
  }
  return (
    <ChartTooltip
      ref={tip}
      title={title}
      rows={[
        {
          label: measure,
          color: colour ?? "var(--chart-ink-3)",
          muted: value === null,
          value:
            value === null ? (
              <span className={`${roles.caption} ${mutedText}`}>No data</span>
            ) : (
              <Fig value={value} format={format} />
            ),
        },
      ]}
      footer={note}
    />
  );
}
