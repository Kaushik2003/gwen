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
  ChartTooltip,
  cssToRgb,
  Figure,
  MarkLayer,
  moveTooltip,
  mutedText,
  SkeletonBlock,
  TooltipFoot,
  useActive,
  useChartFrame,
  type BarMark,
  type ChartConfig,
  type Rgb,
  type Theme,
} from "@/components/liquid-charts/liquid/core";
import {
  DENSITY,
  formatValue,
  levelOf,
  levelScale,
  levelText,
  markShare,
  rowAt,
  rowsLayout,
  rowTooltipPlacement,
  type KpiDensity,
  type KpiFormat,
  type Threshold,
} from "./layout";
import { KPI_LAYER, levelMaterial, type OnFrame } from "./kpi-marks";
import { useRef, type CSSProperties, type ReactNode } from "react";
import { cssModule } from "@/components/liquid-charts/liquid/core/css-module";
import Roles from "@/components/liquid-charts/liquid/core/type.module.css";
import { LiquidCredit } from "@/components/liquid-charts/liquid/credit";
import { GlassLayer } from "@/components/liquid-charts/liquid/glass";
import { KpiState, kpiState } from "./kpi-state";
import { levelReading } from "./progress-bar";
import Styles from "./kpi.module.css";

const roles = cssModule(Roles, "lc-type-");
const styles = cssModule(Styles, "lc-kpi-");

export type CapacityItem = {
  key: string;
  label: string;
  value: number;
  max: number;
  /** A point on this row's scale, engraved into its glass: a plan, a pace, a soft limit. */
  target?: number | null;
  /** A short note for the tooltip's foot ("Grows 1.2 TB a month"). */
  note?: ReactNode;
};

export type CapacityBarsProps = {
  data: readonly CapacityItem[];
  config: ChartConfig;
  /** The liquid's config key for rows without their own config entry. */
  colorKey?: string;
  theme: Theme;
  /** Shares of each row's capacity where its reading changes colour and says why. */
  thresholds?: readonly Threshold[];
  targetLabel?: string;
  density?: KpiDensity;
  active?: number | null;
  onActiveChange?: (index: number | null) => void;
  /** Each row's readout (name, value of capacity and share). */
  labels?: boolean;
  tooltip?: boolean;
  format?: KpiFormat;
  material?: "glass" | "flat";
  loading?: boolean;
  empty?: boolean | ReactNode;
  error?: boolean | ReactNode;
  replay?: number;
  label: string;
  /** One sentence per row, read aloud when it is in focus. */
  describe?: (item: CapacityItem, index: number) => string;
  className?: string;
};

const FALLBACK: Rgb = [0.56, 0.56, 0.56];
const INSETS = { left: 0, right: 0, top: 0, bottom: 0 };

/** Per-row geometry for a list of capacity bars, from the plot width. */
export function capacityGeometry(
  data: readonly CapacityItem[],
  density: KpiDensity,
  width: number,
) {
  const metrics = DENSITY[density];
  const { rows, height } = rowsLayout(data.length, {
    thickness: metrics.thickness,
    readout: metrics.readout,
    gap: metrics.gap + (density === "tile" ? 0 : 4),
    inner: density === "tile" ? 6 : 8,
  });
  return {
    rows,
    height,
    thickness: metrics.thickness,
    width,
    /** Each row's scale: within capacity, or run on past it to the value. */
    scales: data.map((item) => levelScale(item.value, item.max)),
  };
}

/** Each row's empty well, under the glass. */
function Wells({ data, density }: { data: readonly CapacityItem[]; density: KpiDensity }) {
  const { size } = useChartFrame();
  const g = capacityGeometry(data, density, size.width);
  return (
    <>
      {g.rows.map((row, i) => (
        <span
          key={data[i].key}
          className={styles.well}
          style={{ top: row.capsule, height: g.thickness }}
          aria-hidden="true"
        />
      ))}
    </>
  );
}

/** Row readouts: the name at the left, value of capacity and share at the right. A row's status
 * shows as its liquid's colour (or the etch past its capacity); its name is in the tooltip. */
function Readouts({
  data,
  density,
  format,
  colours,
}: {
  data: readonly CapacityItem[];
  density: KpiDensity;
  format: KpiFormat;
  colours: readonly string[];
}) {
  const { size, active } = useChartFrame();
  const g = capacityGeometry(data, density, size.width);
  const figure = density === "tile" ? roles.micro : roles.figureS;
  return (
    <>
      {data.map((item, i) => {
        const level = levelOf(item.value, item.max);
        return (
          <div key={item.key} aria-hidden="true">
            <div
              className={styles.rowReadout}
              data-figure-active={active === i}
              data-dim={active !== null && active !== i}
              style={
                {
                  top: g.rows[i].top,
                  height: DENSITY[density].readout,
                  "--kpi-liquid": colours[i],
                } as CSSProperties
              }
            >
              <span
                className={`${styles.rowName} ${density === "tile" ? roles.caption : roles.label}`}
              >
                {item.label}
              </span>
              <span className={`${styles.rowFigures} ${roles.lift}`}>
                <span className={styles.reading}>
                  <Figure value={item.value} {...format} className={figure} />
                  <span className={`${styles.of} ${roles.caption}`}>of</span>
                  <Figure
                    value={item.max}
                    {...format}
                    className={`${roles.caption} ${styles.quiet}`}
                  />
                  <Figure
                    value={level.ratio}
                    kind="percent"
                    className={`${roles.micro} ${mutedText}`}
                  />
                </span>
              </span>
            </div>
          </div>
        );
      })}
    </>
  );
}

/**
 * Several levels against their capacities, one capsule per row: volumes, seats, quotas by team.
 * Pointer and arrow keys move focus between rows (the rest turn graphite), a click pins a row,
 * and a tooltip beside the focused row gives what is free (or over) and the row's status.
 */
export function CapacityBars({
  data,
  config,
  colorKey,
  theme,
  thresholds,
  targetLabel = "Target",
  density = "standard",
  active: activeProp,
  onActiveChange,
  labels = true,
  tooltip = true,
  format = { kind: "number" },
  material,
  loading = false,
  empty = false,
  error = false,
  replay = 0,
  label,
  describe,
  className,
}: CapacityBarsProps) {
  const [active, setActive] = useActive(activeProp, onActiveChange);
  const tip = useRef<HTMLDivElement>(null);
  const state = kpiState({
    loading,
    empty: empty || (!loading && !error && data.length === 0),
    error,
  });
  const metrics = DENSITY[density];
  const flat = levelMaterial({ material, density, thickness: metrics.thickness }) === "flat";
  const readings = data.map((item) =>
    levelReading(levelOf(item.value, item.max), {
      config,
      colorKey: config[item.key] ? item.key : colorKey,
      thresholds,
      theme,
    }),
  );
  const colours = readings.map((r) => r.colour);
  const statuses = readings.map((r) => r.status);
  const shown = active !== null && active < data.length ? active : null;
  const height = data.length ? capacityGeometry(data, density, 0).height : 56;

  const layout = (width: number): BarMark[] => {
    const g = capacityGeometry(data, density, width);
    return data.map((item, i) => ({
      key: item.key,
      cross: g.rows[i].centre,
      base: 0,
      thickness: g.thickness,
      length: width,
      direction: "right",
      tint: cssToRgb(colours[i]) ?? FALLBACK,
      lit: shown === null || shown === i ? 1 : 0,
      level: g.scales[i].fill,
      // Past its capacity a row is etched, from the hairline to the end (style rule 10).
      pattern: g.scales[i].over ? g.scales[i].capacity : undefined,
      // The row's target is engraved into its glass (style rule 9); aligned targets across rows
      // form a quiet vertical rhythm.
      marks: (() => {
        const share = markShare(item.target, 0, g.scales[i].top);
        return share === null ? undefined : [share];
      })(),
      delay: i * 0.08,
    }));
  };

  const onFrame: OnFrame = (bars, width, h) => {
    const element = tip.current;
    if (!element || shown === null) return;
    const g = capacityGeometry(data, density, width);
    const bar = bars[shown];
    if (!bar) return;
    // The tooltip may rise over the card body above the plot, but not over the card's header.
    const plot = element.parentElement?.getBoundingClientRect();
    const card = element.closest("article");
    const header = card?.querySelector("header")?.getBoundingClientRect();
    const rise = plot && header ? Math.max(0, plot.top - header.bottom - 8) : 0;
    const sink =
      plot && card ? Math.max(0, card.getBoundingClientRect().bottom - plot.bottom - 6) : 0;
    const { x, y } = rowTooltipPlacement(
      { width: element.offsetWidth, height: element.offsetHeight },
      {
        end: bar.base + bar.current * Math.min(1, bar.fill || bar.level || 0),
        top: g.rows[shown].top,
        capsule: g.rows[shown].capsule,
        bottom: g.rows[shown].bottom,
      },
      { width, height: h },
      { rise, sink },
    );
    // Appear in place, then glide between rows (as the core's placeTooltip does).
    moveTooltip(element, Math.round(x), Math.round(y));
  };

  const item = shown !== null ? data[shown] : null;
  const itemLevel = item ? levelOf(item.value, item.max) : null;

  return (
    <div className={`${styles.capacity} ${className ?? ""}`} data-density={density}>
      <ChartFrame
        credit={<LiquidCredit />}
        theme={theme}
        height={height}
        insets={INSETS}
        count={state ? 0 : data.length}
        active={state ? null : shown}
        onActiveChange={setActive}
        pick={(_, y) => (state ? null : rowAt(y, capacityGeometry(data, density, 0).rows))}
        label={label}
        describe={(i) =>
          describe?.(data[i], i) ??
          `${data[i].label}: ${levelText(data[i].value, data[i].max, format, {
            target: data[i].target,
            targetLabel: targetLabel.toLowerCase(),
            status: statuses[i],
          })}.`
        }
        table={{
          caption: label,
          columns: ["Item", "Used", "Capacity", "Share"],
          rows: data.map((d) => [
            d.label,
            formatValue(d.value, format),
            formatValue(d.max, format),
            formatValue(levelOf(d.value, d.max).ratio, { kind: "percent" }),
          ]),
        }}
      >
        {state ? (
          <div
            className={styles.capacityState}
            data-kind={state.kind === "loading" ? "loading" : "message"}
          >
            {state.kind === "loading" ? (
              Array.from({ length: Math.max(1, Math.min(4, data.length || 3)) }, (_, i) => (
                <span key={i} className={styles.rowSkeleton}>
                  <SkeletonBlock />
                </span>
              ))
            ) : (
              <KpiState {...state} />
            )}
          </div>
        ) : (
          <>
            <div
              className={styles.capacityMarks}
              style={{ "--kpi-thickness": `${metrics.thickness}px` } as CSSProperties}
            >
              <Wells data={data} density={density} />
              <MarkLayer
                Glass={GlassLayer}
                {...KPI_LAYER}
                flat={flat}
                theme={theme}
                replay={replay}
                layout={layout}
                onFrame={tooltip ? onFrame : undefined}
              />
            </div>
            {labels && <Readouts data={data} density={density} format={format} colours={colours} />}
            {tooltip && item && itemLevel && (
              <ChartTooltip
                ref={tip}
                title={item.label}
                rows={[
                  {
                    label: itemLevel.over ? "Over by" : "Free",
                    color: colours[shown!],
                    value: (
                      <Figure
                        value={
                          itemLevel.over ? itemLevel.excess : Math.max(0, item.max - item.value)
                        }
                        {...format}
                        className={roles.figureS}
                      />
                    ),
                  },
                ]}
                footer={
                  item.note || statuses[shown!] ? (
                    <TooltipFoot>
                      {statuses[shown!]}
                      {statuses[shown!] && item.note ? " · " : null}
                      {item.note}
                    </TooltipFoot>
                  ) : undefined
                }
              />
            )}
          </>
        )}
      </ChartFrame>
    </div>
  );
}
