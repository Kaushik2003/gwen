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
  formatValue,
  noop,
  resolveStyle,
  seriesColour,
  TooltipFoot,
  useActive,
  useChartFrame,
  type BarMark,
  type ChartConfig,
  type Density,
  type Rgb,
  type Theme,
  type ValueFormat,
} from "@/components/liquid-charts/liquid/core";
import {
  allocateUnits,
  anchorUnit,
  unitCell,
  waffleGeometry,
  type UnitAllocation,
  type WaffleGeometry,
  type WaffleOrder,
} from "./units";
import {
  Fig,
  placeAt,
  plain,
  UnitLayer,
  useIsomorphicLayoutEffect,
  useStyleWarnings,
  useWidth,
} from "./cell-parts";
import { leastCover, quadrantTooltipPosition, sweepDelay } from "./layout";
import { useRef, type CSSProperties, type ReactNode } from "react";
import { usePageTheme } from "@/components/liquid-charts/liquid/core/page-theme";
import { cssModule } from "@/components/liquid-charts/liquid/core/css-module";
import { cssRgb } from "@/components/liquid-charts/liquid/core/colour-scale";
import Roles from "@/components/liquid-charts/liquid/core/type.module.css";
import { LiquidCredit } from "@/components/liquid-charts/liquid/credit";
import Styles from "./cells.module.css";

const roles = cssModule(Roles, "lc-type-");
const styles = cssModule(Styles, "lc-cells-");

export type WaffleItem = {
  /** The config key that names and colours this category. */
  key: string;
  value: number;
  /** Projected, estimated or not yet final: its units are etched (style rule 10). */
  projected?: boolean;
};

export type WaffleProps = {
  /** Categories in fill order. */
  data: readonly WaffleItem[];
  config: ChartConfig;
  /** What the whole grid stands for. Default: the values' sum (they fill the grid). */
  total?: number;
  rows?: number;
  columns?: number;
  /** `rows` reads from the top left like text; `columns` rises from the bottom like a level. */
  order?: WaffleOrder;
  /** Names the empty units ("Unsold"), so they can be focused and read; `false` leaves them out. */
  remainder?: string | false;
  /** What one unit is, singular and plural ("seat", "seats"), for the tooltip and legend note. */
  unitLabel?: readonly [string, string];
  /** Light or dark. Without one, the page's: `data-theme` or a `dark` class on `<html>`, else the system setting. @default the page's */
  theme?: Theme;
  /** The grid's height; the squares take the largest whole size that fits. */
  height?: number;
  density?: Density;
  /** The category in focus (an index into `data`, then the remainder). */
  active?: number | null;
  onActiveChange?: (index: number | null) => void;
  tooltip?: boolean;
  /** A legend of exact counts beside the grid (or under it when narrow). Beside the grid, its
   * row for the category in focus is the readout, so no card covers the squares. */
  legend?: boolean;
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

const HEIGHT: Record<Density, number> = { tile: 150, standard: 248, hero: 320 };
const FALLBACK: Rgb = [0.56, 0.56, 0.56];

type Category = {
  key: string;
  name: string;
  value: number;
  projected: boolean;
  colour: string;
  tint: Rgb;
  remainder: boolean;
};

/**
 * A unit chart: a grid of glass squares (10 × 10 by default) filled in order by each category's
 * share, the end of the filled run holding a partly filled unit, projected units etched. Hover a
 * square or a legend row to light that category's units (the rest smoke to graphite); the arrows
 * step through categories. Tooltip, legend and data table give the exact counts.
 */
export function Waffle({
  data,
  config,
  total,
  rows = 10,
  columns = 10,
  order = "rows",
  remainder = "Remaining",
  unitLabel,
  theme: themeProp,
  height,
  density = "standard",
  active: activeProp,
  onActiveChange,
  tooltip = true,
  legend = true,
  format = { kind: "number" },
  material,
  loading = false,
  empty,
  error,
  replay = 0,
  label,
  describe,
  className,
}: WaffleProps) {
  const theme = usePageTheme(themeProp);
  const [active, setActive] = useActive(activeProp, onActiveChange);
  const [outer, width] = useWidth();
  const units = Math.max(0, rows * columns);
  const values = data.map((d) => (Number.isFinite(d.value) && d.value > 0 ? d.value : 0));
  const sum = values.reduce((a, b) => a + b, 0);
  const whole = total !== undefined && total > 0 ? total : sum;
  const alloc = allocateUnits(values, { units, total: whole || undefined });
  const left = Math.max(0, whole - sum);
  const categories: Category[] = data.map((d, i) => {
    const colour = seriesColour(config, d.key, theme);
    return {
      key: d.key,
      name: plain(config[d.key]?.label, d.key),
      value: values[i],
      projected: Boolean(d.projected),
      colour,
      tint: cssToRgb(colour) ?? FALLBACK,
      remainder: false,
    };
  });
  const lastTint = categories[categories.length - 1]?.tint ?? FALLBACK;
  const hasEmpty = alloc.units.some((u) => u.category < 0 || u.level < 1);
  if (remainder !== false && left > 0 && hasEmpty)
    categories.push({
      key: "__remainder",
      name: remainder,
      value: left,
      projected: false,
      colour: "var(--chart-ink-3)",
      tint: lastTint,
      remainder: true,
    });
  const remainderIndex = categories.findIndex((c) => c.remainder);

  const frameHeight = height ?? HEIGHT[density];
  const fitted = waffleGeometry({ rows, columns, width: width || 400, height: frameHeight, order });
  const side = legend && width - fitted.width - 28 >= 190;
  const geometry = (w: number, h: number) =>
    waffleGeometry({ rows, columns, width: w, height: h, order });
  const style = resolveStyle({
    geometry: "cartesian",
    marks: ["solid"],
    measure: "amount",
    markCount: units,
    smallestMark: fitted.size,
    material,
  });
  useStyleWarnings(style.warnings, "Waffle");
  const glass = style.materials.solid === "glass";
  const share = (value: number) => (whole ? value / whole : 0);
  const formatted = (value: number) => formatValue(value, format);
  const perSquare = formatValue(alloc.unitValue, {
    ...format,
    decimals: Number.isInteger(alloc.unitValue) ? 0 : Math.max(1, format.decimals ?? 0),
    signed: false,
  });
  const unitNote = unitLabel
    ? `1 square = ${perSquare} ${alloc.unitValue === 1 ? unitLabel[0] : unitLabel[1]}`
    : alloc.unitValue !== 1
      ? `1 square = ${perSquare}`
      : null;

  const wrapperClass = `${styles.root} ${className ?? ""}`;
  const state = error ? "error" : loading ? "loading" : !units || !whole ? "empty" : null;
  if (state)
    return (
      <ChartScope theme={theme} className={wrapperClass}>
        <div ref={outer}>
          <div style={{ width: Math.max(fitted.width, 120) }}>
            <ChartFrame
              credit={<LiquidCredit />}
              theme={theme}
              height={fitted.height || frameHeight}
              insets={{ left: 0, top: 0, right: 0, bottom: 0 }}
              count={1}
              active={null}
              onActiveChange={noop}
              pick={() => null}
              label={label}
            >
              {state === "loading" ? (
                <ChartState kind="loading" skeleton={{ grid: [rows, columns] }} />
              ) : (
                <ChartState
                  kind={state}
                  message={state === "error" ? (error === true ? undefined : error) : empty}
                />
              )}
            </ChartFrame>
          </div>
        </div>
      </ChartScope>
    );

  const categoryOf = (unit: number | null) => {
    if (unit === null || unit < 0 || unit >= units) return null;
    const c = alloc.units[unit].category;
    if (c >= 0 && alloc.units[unit].level >= 0.5) return c;
    if (c >= 0) return remainderIndex >= 0 ? remainderIndex : c;
    return remainderIndex >= 0 ? remainderIndex : null;
  };
  const marks = (w: number, h: number): BarMark[] => {
    const g = geometry(w, h);
    return alloc.units.map((unit, u) => {
      const r = g.rect(u);
      const c = unit.category;
      const { row, column } = unitCell(u, rows, columns, order);
      const filled = c >= 0;
      const mine = filled ? active === c : active === remainderIndex && remainderIndex >= 0;
      return {
        key: `u${u}`,
        cross: (r.left + r.right) / 2,
        base: r.bottom,
        thickness: g.size,
        length: g.size,
        direction: "up",
        tint: filled ? categories[c].tint : lastTint,
        lit: active === null ? (filled ? 1 : 0) : mine ? 1 : 0,
        level: filled ? unit.level : 0,
        pattern: filled && categories[c].projected ? true : undefined,
        delay: sweepDelay(row, column, rows, columns, 0.6),
      };
    });
  };

  const describeCategory = (i: number) => {
    const c = categories[i];
    return `${c.name}: ${formatted(c.value)}, ${Math.round(share(c.value) * 1000) / 10}% of ${formatted(whole)}${c.projected ? ", projected" : ""}.`;
  };

  return (
    <ChartScope theme={theme} className={wrapperClass}>
      <div ref={outer} className={styles.waffle} data-side={side}>
        <div className={styles.waffleGrid} style={{ width: fitted.width }}>
          <ChartFrame
            credit={<LiquidCredit />}
            theme={theme}
            height={fitted.height}
            insets={{ left: 0, top: 0, right: 0, bottom: 0 }}
            count={categories.length}
            active={active}
            onActiveChange={setActive}
            pick={(x, y, size) => categoryOf(geometry(size.width, size.height).at(x, y))}
            label={label}
            describe={describe ?? describeCategory}
            table={{
              // What a square stands for is read here and in the tooltip, never printed on the card.
              caption: unitNote ? `${label} (${unitNote})` : label,
              columns: ["Category", "Count", "Share"],
              rows: categories.map((c) => [
                c.name + (c.projected ? " (projected)" : ""),
                formatted(c.value),
                `${Math.round(share(c.value) * 1000) / 10}%`,
              ]),
            }}
          >
            <UnitLayer
              theme={theme}
              replay={replay}
              material={glass ? "glass" : "flat"}
              layout={marks}
            />
            {tooltip && !side && active !== null && categories[active] && (
              <WaffleTooltip
                geometry={geometry}
                alloc={alloc}
                index={active}
                category={categories[active]}
                share={share(categories[active].value)}
                whole={whole}
                format={format}
                unitNote={unitNote}
              />
            )}
          </ChartFrame>
        </div>
        {legend && (
          <ul className={styles.waffleLegend} onPointerLeave={() => setActive(null)}>
            {categories.map((c, i) => (
              <li
                key={c.key}
                className={styles.waffleItem}
                data-active={active === i}
                data-dim={active !== null && active !== i}
                onPointerEnter={() => setActive(i)}
              >
                <span
                  className={
                    c.remainder
                      ? styles.emptySwatch
                      : c.projected
                        ? styles.etchedSwatch
                        : styles.glassSwatch
                  }
                  style={
                    c.remainder
                      ? undefined
                      : ({
                          "--swatch": c.colour,
                          ...(c.projected ? { backgroundColor: c.colour } : {}),
                        } as CSSProperties)
                  }
                  aria-hidden="true"
                />
                <span className={`${styles.waffleName} ${roles.label}`}>{c.name}</span>
                <span className={styles.waffleCount}>
                  <Fig value={c.value} format={format} className={roles.figureS} />
                  <span className={`${roles.micro} ${styles.waffleShare}`}>
                    {Math.round(share(c.value) * 100)}%
                  </span>
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </ChartScope>
  );
}

function WaffleTooltip({
  geometry,
  alloc,
  index,
  category,
  share,
  whole,
  format,
  unitNote,
}: {
  geometry: (width: number, height: number) => WaffleGeometry;
  alloc: UnitAllocation;
  index: number;
  category: Category;
  share: number;
  whole: number;
  format: ValueFormat;
  unitNote: string | null;
}) {
  const tip = useRef<HTMLDivElement>(null);
  const { size } = useChartFrame();
  useIsomorphicLayoutEffect(() => {
    const element = tip.current;
    if (!element || !size.width) return;
    const g = geometry(size.width, size.height);
    // Wherever covers least of the category's own squares: a corner of the grid, or off the
    // corner of the square where its run ends.
    const own = alloc.units.flatMap((u, i) =>
      (category.remainder ? u.category < 0 || u.level < 1 : u.category === index)
        ? [g.rect(i)]
        : [],
    );
    const unit = anchorUnit(alloc, category.remainder ? "remainder" : index);
    if (unit < 0) return;
    const r = g.rect(unit);
    const w = element.offsetWidth,
      h = element.offsetHeight;
    const near = quadrantTooltipPosition(
      { left: r.left - 2, top: r.top - 2, right: r.right + 2, bottom: r.bottom + 2 },
      { width: w, height: h },
      { left: 0, top: 0, right: size.width, bottom: size.height },
      8,
    );
    placeAt(
      element,
      leastCover(
        [
          near,
          { x: size.width - w, y: size.height - h },
          { x: 0, y: size.height - h },
          { x: size.width - w, y: 0 },
          { x: 0, y: 0 },
        ],
        { width: w, height: h },
        { left: 0, top: 0, right: size.width, bottom: size.height },
        own,
      ),
    );
  });
  return (
    <ChartTooltip
      ref={tip}
      title={category.projected ? `${category.name} · projected` : category.name}
      rows={[
        {
          label: `${Math.round(share * 1000) / 10}% of ${formatValue(whole, format)}`,
          color: category.remainder ? "var(--chart-ink-3)" : cssRgb(category.tint),
          muted: category.remainder,
          value: <Fig value={category.value} format={format} />,
        },
      ]}
      footer={unitNote ? <TooltipFoot>{unitNote}</TooltipFoot> : undefined}
    />
  );
}
