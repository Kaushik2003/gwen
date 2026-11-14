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
  deltaOf,
  formatValue,
  type Delta,
  type GoodDirection,
  type KpiDensity,
  type KpiFormat,
  type Threshold,
} from "./layout";
import {
  ChartScope,
  Delta as ChangeText,
  Figure,
  Legend,
  type ChartConfig,
  type LegendEntry,
  type Theme,
} from "@/components/liquid-charts/liquid/core";
import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
  type RefObject,
} from "react";
import { cssModule } from "@/components/liquid-charts/liquid/core/css-module";
import Roles from "@/components/liquid-charts/liquid/core/type.module.css";
import { KpiState, kpiState } from "./kpi-state";
import { ProgressBar } from "./progress-bar";
import Styles from "./kpi.module.css";

const roles = cssModule(Roles, "lc-type-");
const styles = cssModule(Styles, "lc-kpi-");

const useIsomorphicLayoutEffect = typeof window === "undefined" ? useEffect : useLayoutEffect;

/** A change to show beside a headline: against a comparison (last period, plan, target). */
export type StatDelta = {
  /** The value compared against. */
  comparison: number;
  /** What it is compared against: "vs September". */
  label?: ReactNode;
  /** Which way is good news. Default up. */
  good?: GoodDirection;
  /** Relative (percent change, the default), absolute (in `format`, plus `unit`), or points
   * (a change between two percentages, in percentage points). */
  mode?: DeltaMode;
  /** For absolute changes; defaults to the headline's format. */
  format?: KpiFormat;
  /** After an absolute change: " pts". */
  unit?: string;
  /** The current value, when it is not the headline itself. */
  value?: number;
};

export type DeltaMode = "relative" | "absolute" | "points";

/** @deprecated Insight chips are gone (a card carries one figure and at most one change); the
 * `insight` and `insights` props that take this are ignored and will be removed. */
export type StatInsight = { label: ReactNode; colorKey?: string };

/** An inline level under a headline: quota booked, budget spent. */
export type StatProgress = {
  value: number;
  max: number;
  target?: number | null;
  targetLabel?: string;
  thresholds?: readonly Threshold[];
  colorKey?: string;
  format?: KpiFormat;
  label?: string;
};

/** The words for a delta, for a screen reader: "up 4.2% vs September, good". */
export function deltaText(
  delta: Delta,
  {
    mode = "relative",
    format,
    unit = "",
    label,
  }: {
    mode?: DeltaMode;
    format: KpiFormat;
    unit?: string;
    label?: string;
  },
) {
  if (delta.direction === "flat") return `unchanged${label ? ` ${label}` : ""}`;
  const amount =
    mode === "points"
      ? `${formatValue(Math.abs(delta.change) * 100, { kind: "number", decimals: format.decimals ?? 1 })} points`
      : mode === "relative" && delta.relative !== null
        ? formatValue(Math.abs(delta.relative), { kind: "percent", decimals: 1 })
        : formatValue(Math.abs(delta.change), format) + unit;
  return `${delta.direction} ${amount}${label ? ` ${label}` : ""}`;
}

/**
 * A change against a comparison, as plain coloured text (the core's `Delta`): an arrow for its
 * direction, coloured by whether it is good news (churn falling is green), with a true minus. A
 * change that is neither good nor bad (`good: "none"`) reads in the quiet ink, and no change has no
 * arrow. The comparison (`children`, "vs September") is the first thing to go when the line is
 * short.
 */
export function StatChange({
  delta,
  mode = "relative",
  format,
  unit,
  children,
}: {
  delta: Delta;
  mode?: DeltaMode;
  format: KpiFormat;
  unit?: string;
  children?: ReactNode;
}) {
  const relative = mode === "relative" && delta.relative !== null;
  const points = mode === "points";
  // The core colours a change by its direction against `better`; here the tone decides, so a
  // neutral change is passed as good news and then quietened in CSS.
  const better =
    delta.tone === "bad"
      ? delta.direction === "down"
        ? "up"
        : "down"
      : delta.direction === "down"
        ? "down"
        : "up";
  return (
    <span className={styles.change} data-tone={delta.tone} data-direction={delta.direction}>
      <ChangeText value={delta.change} better={better}>
        {relative ? (
          <Figure value={delta.relative!} kind="percent" decimals={1} signed />
        ) : points ? (
          <span className={styles.changeAmount}>
            <Figure
              value={delta.change * 100}
              kind="number"
              decimals={format.decimals ?? 1}
              signed
            />
            <span className={styles.changeUnit}>pts</span>
          </span>
        ) : (
          <span className={styles.changeAmount}>
            <Figure value={delta.change} {...format} signed />
            {unit && <span className={styles.changeUnit}>{unit.trim()}</span>}
          </span>
        )}
        {children && <span className={styles.changeContext}>{children}</span>}
      </ChangeText>
    </span>
  );
}

/** A card's legend: the core's `Legend` for entries, or your own node, as a row under the plot. */
function StatLegend({ legend }: { legend: ReactNode | readonly LegendEntry[] }) {
  return (
    <div className={styles.legend}>
      {Array.isArray(legend) ? (
        <Legend items={legend as readonly LegendEntry[]} />
      ) : (
        (legend as ReactNode)
      )}
    </div>
  );
}

/** Keeps the figure's line to one line: when the label and change don't fit beside the figure,
 * the change drops its comparison first, and only then does the label truncate. */
function useFitLine(ref: RefObject<HTMLElement | null>, key: unknown) {
  useIsomorphicLayoutEffect(() => {
    const line = ref.current;
    if (!line) return;
    const fit = () => {
      delete line.dataset.tight;
      // Full widths (the label's untruncated), so the comparison drops before the label shrinks.
      const parts = Array.from(line.children) as HTMLElement[];
      const gap = parseFloat(getComputedStyle(line).columnGap) || 0;
      const need = parts.reduce((sum, el) => sum + el.scrollWidth, 0) + gap * (parts.length - 1);
      if (need > line.clientWidth + 1) line.dataset.tight = "";
    };
    const observer = new ResizeObserver(fit);
    observer.observe(line);
    for (const child of Array.from(line.children)) observer.observe(child);
    return () => observer.disconnect();
  }, [ref, key]);
}

function resolveDelta(value: number | undefined, spec: StatDelta | undefined) {
  if (!spec) return null;
  const current = spec.value ?? value;
  return current === undefined ? null : deltaOf(current, spec.comparison, spec.good ?? "up");
}

export type StatHeaderProps = {
  title: ReactNode;
  /** Context for screen readers ("Quarter to date · USD · week 9 of 13"); not shown. */
  subtitle?: ReactNode;
  value?: number;
  format?: KpiFormat;
  valueLabel?: ReactNode;
  /** Replaces the headline figure (a range, a ratio). */
  headline?: ReactNode;
  delta?: StatDelta;
  /** @deprecated Ignored: a card carries one figure and at most one change. Will be removed. */
  insights?: readonly StatInsight[];
  /** More on the figure's line, after the change (your own `Delta`, say). */
  chips?: ReactNode;
  /** Series names with their swatches. In a `StatCard` they sit in a row under the plot; a
   * header on its own closes with them. */
  legend?: ReactNode | readonly LegendEntry[];
  progress?: StatProgress;
  config?: ChartConfig;
  theme: Theme;
  density?: KpiDensity;
  loading?: boolean;
  replay?: number;
  headingLevel?: 2 | 3 | 4;
};

/**
 * The library's card header, kept minimal: a quiet title, a subtitle for screen readers only, and
 * one line holding the headline figure, its label and a change against a comparison. That line
 * never wraps: the change drops its comparison first, then the label truncates. An optional inline
 * level sits under it.
 */
export function StatHeader({
  title,
  subtitle,
  value,
  format = { kind: "number" },
  valueLabel,
  headline,
  delta,
  chips,
  legend,
  progress,
  config = {},
  theme,
  density = "standard",
  loading = false,
  replay,
  headingLevel = 2,
}: StatHeaderProps) {
  const Heading = `h${headingLevel}` as const;
  const change = resolveDelta(value, delta);
  const line = useRef<HTMLDivElement>(null);
  useFitLine(line, loading);
  const figure = density === "tile" ? roles.figureL : roles.figureXL;
  return (
    <header className={styles.header} data-density={density}>
      <Heading className={`${styles.title} ${roles.label}`}>{title}</Heading>
      {subtitle && <p className={styles.srOnly}>{subtitle}</p>}
      <div ref={line} className={styles.kpi}>
        {loading ? (
          <KpiState kind="loading" />
        ) : (
          <>
            {headline ??
              (value !== undefined && (
                <Figure value={value} {...format} className={`${figure} ${styles.headline}`} />
              ))}
            {valueLabel && (
              <span className={`${styles.kpiLabel} ${roles.caption}`}>{valueLabel}</span>
            )}
            {change && delta && (
              <StatChange
                delta={change}
                mode={delta.mode}
                format={delta.format ?? format}
                unit={delta.unit}
              >
                {delta.label}
              </StatChange>
            )}
            {chips}
          </>
        )}
      </div>
      {progress && !loading && (
        <div className={styles.headerProgress}>
          <ProgressBar
            {...progress}
            format={progress.format ?? format}
            config={config}
            theme={theme}
            density="tile"
            readout={false}
            replay={replay}
            label={progress.label ?? (typeof title === "string" ? title : "Progress")}
          />
        </div>
      )}
      {legend && <StatLegend legend={legend} />}
    </header>
  );
}

/** A card in the same frame as the showcase's story card: a square hairline card holding the
 * header, the chart, and any legend as a quiet row under the chart. */
export function StatCard({
  theme,
  wide = false,
  label,
  className,
  legend,
  children,
  ...header
}: StatHeaderProps & {
  wide?: boolean;
  /** The card's accessible name; defaults to the title when it is a string. */
  label?: string;
  className?: string;
  children?: ReactNode;
}) {
  return (
    <ChartScope
      theme={theme}
      className={`${styles.card} ${wide ? styles.wide : ""} ${className ?? ""}`}
    >
      <article
        className={styles.cardInner}
        aria-label={label ?? (typeof header.title === "string" ? header.title : undefined)}
      >
        <StatHeader theme={theme} {...header} />
        {children && <div className={styles.cardBody}>{children}</div>}
        {legend && <StatLegend legend={legend} />}
      </article>
    </ChartScope>
  );
}

export type StatTileProps = {
  /** What the number is: "Monthly recurring revenue". */
  label: string;
  value: number;
  format?: KpiFormat;
  /** A short qualifier after the figure: "users", "/ month". */
  unit?: ReactNode;
  delta?: StatDelta;
  /** @deprecated Ignored: a tile carries one figure and at most one change. Will be removed. */
  insight?: ReactNode | StatInsight;
  progress?: StatProgress;
  config?: ChartConfig;
  theme: Theme;
  density?: KpiDensity;
  loading?: boolean;
  empty?: boolean | ReactNode;
  error?: boolean | ReactNode;
  replay?: number;
  /** Replaces the generated accessible description. */
  describe?: string;
  className?: string;
};

/**
 * A headline tile for a grid of KPIs: the name, the figure and its change, with an optional inline
 * level. It is one focusable, described element; focus or hover lifts the figure
 * and a changed value is announced. `density="tile"` sets it compact for dense grids.
 */
export function StatTile({
  label,
  value,
  format = { kind: "number" },
  unit,
  delta,
  progress,
  config = {},
  theme,
  density = "tile",
  loading = false,
  empty = false,
  error = false,
  replay,
  describe,
  className,
}: StatTileProps) {
  const [active, setActive] = useState(false);
  const change = resolveDelta(value, delta);
  const state = kpiState({ loading, empty, error });
  const reading =
    describe ??
    [
      `${label}: ${formatValue(value, format)}`,
      change && delta
        ? deltaText(change, {
            mode: delta.mode,
            format: delta.format ?? format,
            unit: delta.unit,
            label: typeof delta.label === "string" ? delta.label : undefined,
          })
        : null,
    ]
      .filter(Boolean)
      .join(", ");
  const [spoken, setSpoken] = useState({ reading, announce: "" });
  if (spoken.reading !== reading) setSpoken({ reading, announce: reading });
  const figure =
    density === "hero" ? roles.figureXL : density === "standard" ? roles.figureXL : roles.figureL;

  return (
    <ChartScope theme={theme} className={`${styles.tile} ${className ?? ""}`}>
      <div
        className={styles.tileInner}
        data-density={density}
        data-figure-active={active}
        role="group"
        tabIndex={0}
        aria-label={state ? `${label}: ${state.kind}` : reading}
        onPointerEnter={() => setActive(true)}
        onPointerLeave={() => setActive(false)}
        onFocus={() => setActive(true)}
        onBlur={() => setActive(false)}
      >
        <span className={`${styles.tileLabel} ${roles.label}`}>{label}</span>
        {state ? (
          <div className={styles.tileState}>
            <KpiState {...state} />
          </div>
        ) : (
          <>
            <span className={styles.tileFigure}>
              <span className={`${styles.tileReading} ${roles.lift}`}>
                <Figure value={value} {...format} className={figure} />
                {unit && <span className={`${styles.tileUnit} ${roles.caption}`}>{unit}</span>}
              </span>
            </span>
            {change && delta && (
              <span className={styles.tileDelta}>
                <StatChange
                  delta={change}
                  mode={delta.mode}
                  format={delta.format ?? format}
                  unit={delta.unit}
                >
                  {delta.label}
                </StatChange>
              </span>
            )}
            {progress && (
              <div className={styles.tileFoot}>
                <ProgressBar
                  {...progress}
                  format={progress.format ?? format}
                  config={config}
                  theme={theme}
                  density="tile"
                  readout={false}
                  replay={replay}
                  label={progress.label ?? label}
                />
              </div>
            )}
          </>
        )}
        <span className={styles.srOnly} aria-live="polite">
          {spoken.announce}
        </span>
      </div>
    </ChartScope>
  );
}
