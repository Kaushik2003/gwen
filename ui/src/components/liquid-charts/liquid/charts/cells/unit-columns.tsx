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
  cssToRgb,
  ENTER,
  figureWidth,
  formatValue,
  labelDelay,
  noop,
  placeLabels,
  placeTooltip,
  plotRect,
  resolveStyle,
  seriesColour,
  textWidth,
  TooltipFoot,
  TYPE,
  useActive,
  useChartFrame,
  type BarMark,
  type ChartConfig,
  type Density,
  type Insets,
  type LabelMode,
  type Rgb,
  type Theme,
  type ValueFormat,
} from "@/components/liquid-charts/liquid/core";
import { Fig, plain, UnitLayer, useIsomorphicLayoutEffect, useStyleWarnings } from "./cell-parts";
import { columnUnits, unitColumnsGeometry, unitCounts, type UnitColumnsGeometry } from "./units";
import { cellLabelPriorities, thinLabels, wrapLines } from "./layout";
import { useRef, type CSSProperties, type ReactNode } from "react";
import { usePageTheme } from "@/components/liquid-charts/liquid/core/page-theme";
import { cssModule } from "@/components/liquid-charts/liquid/core/css-module";
import { cssRgb } from "@/components/liquid-charts/liquid/core/colour-scale";
import Roles from "@/components/liquid-charts/liquid/core/type.module.css";
import { LiquidCredit } from "@/components/liquid-charts/liquid/credit";
import Styles from "./cells.module.css";

const roles = cssModule(Roles, "lc-type-");
const styles = cssModule(Styles, "lc-cells-");

export type UnitColumnsProps<Row extends Record<string, unknown>> = {
  data: readonly Row[];
  /** Colours: one entry per category (keyed by its name) or one for `valueKey`. */
  config: ChartConfig;
  categoryKey: keyof Row & string;
  valueKey: keyof Row & string;
  /** Projected or not yet counted, stacked on top as etched units. */
  projectedKey?: keyof Row & string;
  categoryTitle?: (row: Row, index: number) => string;
  /** What one square stands for (10 respondents, say). */
  unit: number;
  /** One square's thing, singular and plural ("respondent", "respondents"). */
  unitLabel?: readonly [string, string];
  /** Squares across each column; chosen to make squares as large as the plot allows by default. */
  across?: number;
  /** Light or dark. Without one, the page's: `data-theme` or a `dark` class on `<html>`, else the system setting. @default the page's */
  theme?: Theme;
  height?: number;
  density?: Density;
  active?: number | null;
  onActiveChange?: (index: number | null) => void;
  /** Counts over the columns: all (default), auto, ends, none. */
  labels?: LabelMode;
  tooltip?: boolean;
  format?: ValueFormat;
  material?: "glass" | "flat";
  loading?: boolean;
  empty?: ReactNode;
  error?: ReactNode;
  replay?: number;
  label: string;
  describe?: (index: number) => string;
  className?: string;
};

const HEIGHT: Record<Density, number> = { tile: 170, standard: 280, hero: 340 };
const INSETS: Insets = { left: 0, right: 0, top: 26, bottom: 40 };
const FALLBACK: Rgb = [0.56, 0.56, 0.56];
const num = (value: unknown) => (typeof value === "number" && Number.isFinite(value) ? value : 0);

/**
 * A waffle bar chart: each category a column of glass squares, one square per `unit`, stacked
 * from the bottom, the top square filled to the fraction left over, projected units etched on
 * top. Hover or step through the columns (the rest smoke to graphite); counts over each column,
 * exact figures in the tooltip and the data table.
 */
export function UnitColumns<Row extends Record<string, unknown>>({
  data,
  config,
  categoryKey,
  valueKey,
  projectedKey,
  categoryTitle,
  unit,
  unitLabel,
  across,
  theme: themeProp,
  height,
  density = "standard",
  active: activeProp,
  onActiveChange,
  labels = "all",
  tooltip = true,
  format = { kind: "number" },
  material,
  loading = false,
  empty,
  error,
  replay = 0,
  label,
  describe,
  className,
}: UnitColumnsProps<Row>) {
  const theme = usePageTheme(themeProp);
  const [active, setActive] = useActive(activeProp, onActiveChange);
  const names = data.map((row) => String(row[categoryKey] ?? ""));
  const values = data.map((row) => num(row[valueKey]));
  const projected = data.map((row) => (projectedKey ? num(row[projectedKey]) : 0));
  const columns = unitCounts(values, unit, projected);
  const squares = columns.map(columnUnits);
  const total = values.reduce((a, b) => a + b, 0);
  const frameHeight = height ?? HEIGHT[density];
  const colourKey = (i: number) => (config[names[i]] ? names[i] : valueKey);
  const colours = names.map((_, i) => seriesColour(config, colourKey(i), theme));
  const tints = colours.map((colour) => cssToRgb(colour) ?? FALLBACK);
  const geometry = (w: number, h: number) =>
    unitColumnsGeometry({
      counts: squares.map((s) => s.length),
      plot: plotRect({ width: w, height: h }, INSETS),
      across,
    });
  const title = (i: number) => categoryTitle?.(data[i], i) ?? names[i];
  const measure = plain(config[valueKey]?.label, valueKey);
  const unitNote = `1 square = ${formatValue(unit, { ...format, signed: false })}${unitLabel ? ` ${unit === 1 ? unitLabel[0] : unitLabel[1]}` : ""}`;
  const markCount = squares.reduce((n, s) => n + s.length, 0);
  // The square size the plot would give, for the style rules (a guess before the frame measures).
  const guess = geometry(560, frameHeight).size;
  const style = resolveStyle({
    geometry: "cartesian",
    marks: ["solid"],
    measure: "amount",
    markCount,
    smallestMark: guess,
    material,
  });
  useStyleWarnings(style.warnings, "UnitColumns");
  const glass = style.materials.solid === "glass";

  const wrapperClass = `${styles.root} ${className ?? ""}`;
  const state = error ? "error" : loading ? "loading" : !data.length || !markCount ? "empty" : null;
  if (state)
    return (
      <ChartScope theme={theme} className={wrapperClass}>
        <ChartFrame
          credit={<LiquidCredit />}
          theme={theme}
          height={frameHeight}
          insets={INSETS}
          count={1}
          active={null}
          onActiveChange={noop}
          pick={() => null}
          label={label}
        >
          {state === "loading" ? (
            <ChartState
              kind="loading"
              skeleton={{ grid: [6, Math.max(3, Math.min(8, data.length || 5)) * 2] }}
            />
          ) : (
            <ChartState
              kind={state}
              message={state === "error" ? (error === true ? undefined : error) : empty}
            />
          )}
        </ChartFrame>
      </ChartScope>
    );

  const marks = (w: number, h: number): BarMark[] => {
    const g = geometry(w, h);
    const rows = Math.max(1, ...squares.map((s) => Math.ceil(s.length / g.across)));
    const span = rows + names.length * g.across;
    return squares.flatMap((column, i) =>
      column.map((square, k) => {
        const r = g.rect(i, k);
        const row = Math.floor(k / g.across),
          col = i * g.across + (k % g.across);
        return {
          key: `${i}-${k}`,
          cross: (r.left + r.right) / 2,
          base: r.bottom,
          thickness: g.size,
          length: g.size,
          direction: "up" as const,
          tint: tints[i],
          lit: active === null || active === i ? 1 : 0,
          level: square.level,
          pattern: square.projected ? true : undefined,
          delay: ((row + col) / Math.max(1, span)) * 0.75,
        };
      }),
    );
  };

  const formatted = (v: number) => formatValue(v, format);
  const describeColumn = (i: number) =>
    `${title(i)}: ${formatted(values[i])} ${measure.toLowerCase()}, ${Math.round((total ? values[i] / total : 0) * 1000) / 10}% of ${formatted(total)}${projected[i] ? `, ${formatted(projected[i])} more expected` : ""}.`;

  return (
    <ChartScope theme={theme} className={wrapperClass}>
      <ChartFrame
        credit={<LiquidCredit />}
        theme={theme}
        height={frameHeight}
        insets={INSETS}
        count={data.length}
        active={active}
        onActiveChange={setActive}
        pick={(x, y, size) => geometry(size.width, size.height).at(x, y)}
        label={label}
        describe={describe ?? describeColumn}
        table={{
          caption: `${label} (${unitNote})`,
          columns: ["Category", measure, "Share", ...(projectedKey ? ["Expected"] : [])],
          rows: data.map((_, i) => [
            title(i),
            formatted(values[i]),
            `${Math.round((total ? values[i] / total : 0) * 1000) / 10}%`,
            ...(projectedKey ? [formatted(projected[i])] : []),
          ]),
        }}
      >
        <UnitLayer
          theme={theme}
          replay={replay}
          material={glass ? "glass" : "flat"}
          layout={marks}
        />
        <ColumnLabels
          key={replay}
          geometry={geometry}
          names={names}
          values={values}
          squares={squares.map((s) => s.length)}
          labels={labels}
          active={active}
          format={format}
        />
        {tooltip && active !== null && data[active] && (
          <ColumnTooltip
            geometry={geometry}
            index={active}
            units={squares[active].length}
            title={title(active)}
            measure={measure}
            value={values[active]}
            projected={projected[active]}
            share={total ? values[active] / total : 0}
            total={total}
            colour={cssRgb(tints[active])}
            format={format}
            unitNote={unitNote}
          />
        )}
      </ChartFrame>
    </ChartScope>
  );
}

function ColumnLabels({
  geometry,
  names,
  values,
  squares,
  labels,
  active,
  format,
}: {
  geometry: (width: number, height: number) => UnitColumnsGeometry;
  names: readonly string[];
  values: readonly number[];
  squares: readonly number[];
  labels: LabelMode;
  active: number | null;
  format: ValueFormat;
}) {
  const { size } = useChartFrame();
  if (!size.width) return null;
  const g = geometry(size.width, size.height);
  const priorities = cellLabelPriorities(values, labels, active);
  const placed = placeLabels(
    values.map((v, i) => ({
      id: String(i),
      x: g.columns[i].centre,
      y: g.top(i, squares[i]),
      width: figureWidth(formatValue(v, format), TYPE.figure) * (active === i ? 1.22 : 1),
      height: 18,
      side: "above" as const,
      offset: 6,
      flip: false,
      priority: priorities[i],
    })),
    { bounds: { left: 0, top: 0, right: size.width, bottom: size.height }, gap: 6 },
  );
  // Category names wrap onto two lines under their column; only when even that cannot fit are
  // they thinned (never rotated or cut).
  const slot = size.width / Math.max(1, names.length) - 10;
  const wrapped = names.map((name) => wrapLines(name, slot, TYPE.label));
  const wraps = wrapped.every(
    (lines) => lines.length <= 2 && lines.every((line) => textWidth(line, TYPE.label) <= slot),
  );
  const thin = wraps
    ? null
    : thinLabels({
        centres: g.columns.map((c) => c.centre),
        extents: names.map((name) => textWidth(name, TYPE.label)),
        lo: 0,
        hi: size.width,
        active,
        gap: 10,
      });
  const base = size.height - INSETS.bottom;
  return (
    <div className={styles.axes} aria-hidden="true">
      {values.map((v, i) => (
        <span
          key={`v${i}`}
          className={styles.columnValue}
          data-active={active === i}
          data-dim={active !== null && active !== i}
          data-figure-active={active === i}
          style={
            {
              left: g.columns[i].centre,
              top: g.top(i, squares[i]) - 6,
              visibility: placed[i].visible ? "visible" : "hidden",
              "--delay": `${Math.round(labelDelay(i * 0.06, ENTER.small) * 1000)}ms`,
            } as CSSProperties
          }
        >
          <Fig value={v} format={format} className={`${roles.figureS} ${roles.lift}`} />
        </span>
      ))}
      {names.map((name, i) => (
        <span
          key={`n${i}`}
          className={`${styles.columnName} ${roles.label}`}
          data-shown={thin ? thin.visible[i] : true}
          data-active={active === i}
          data-wrap={!thin}
          style={
            thin
              ? { left: thin.starts[i], top: base + 8 }
              : { left: g.columns[i].centre, top: base + 8, width: Math.max(0, slot) }
          }
        >
          {thin
            ? name
            : wrapped[i].map((line, k) => (
                <span key={k} className={styles.columnLine}>
                  {line}
                </span>
              ))}
        </span>
      ))}
    </div>
  );
}

function ColumnTooltip({
  geometry,
  index,
  units,
  title,
  measure,
  value,
  projected,
  share,
  total,
  colour,
  format,
  unitNote,
}: {
  geometry: (width: number, height: number) => UnitColumnsGeometry;
  index: number;
  units: number;
  title: string;
  measure: string;
  value: number;
  projected: number;
  share: number;
  total: number;
  colour: string;
  format: ValueFormat;
  unitNote: string;
}) {
  const tip = useRef<HTMLDivElement>(null);
  const { size } = useChartFrame();
  useIsomorphicLayoutEffect(() => {
    const element = tip.current;
    if (!element || !size.width) return;
    const g = geometry(size.width, size.height);
    const c = g.columns[index];
    const anchor = {
      left: c.left,
      right: c.left + c.width,
      top: g.top(index, units) - 26,
      bottom: size.height - INSETS.bottom,
    };
    const bounds = { left: 4, top: 4, right: size.width - 4, bottom: size.height - INSETS.bottom };
    // Over the column when there is room (it covers nothing there), else beside it.
    if (anchor.top - 8 - element.offsetHeight >= bounds.top)
      placeTooltip(element, anchor, bounds, { prefer: "above" });
    else placeTooltip(element, anchor, bounds);
  });
  return (
    <ChartTooltip
      ref={tip}
      title={title}
      rows={[
        {
          label: measure,
          color: colour,
          value: <Fig value={value} format={format} />,
        },
      ]}
      footer={
        <>
          <TooltipFoot>
            {Math.round(share * 1000) / 10}% of {formatValue(total, format)}
          </TooltipFoot>
          {projected > 0 && (
            <TooltipFoot>
              <Fig value={projected} format={format} signed className="" /> expected, etched
            </TooltipFoot>
          )}
          <TooltipFoot>{unitNote}</TooltipFoot>
        </>
      }
    />
  );
}
