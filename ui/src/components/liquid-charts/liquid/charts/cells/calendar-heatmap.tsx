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
  formatValue,
  mutedText,
  noop,
  resolveStyle,
  textWidth,
  TooltipFoot,
  TYPE,
  useActive,
  useChartFrame,
  type BarMark,
  type ChartConfig,
  type Insets,
  type Theme,
  type ValueFormat,
} from "@/components/liquid-charts/liquid/core";
import {
  calendarLayout,
  calendarModel,
  calendarStep,
  DAY,
  dayKey,
  monthTicks,
  parseDay,
  rangeOutline,
  type CalendarData,
  type CalendarFold,
  type CalendarLayout,
  type CalendarModel,
  type DayInput,
} from "./calendar";
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
  scaleKeys,
  type ColourScale,
  type HeatScale,
} from "@/components/liquid-charts/liquid/core/colour-scale";
import { useEffect, useRef, type KeyboardEvent, type MouseEvent, type ReactNode } from "react";
import { GradientLegend, strengthCss } from "@/components/liquid-charts/liquid/core/gradient-legend";
import { quadrantTooltipPosition, sweepDelay, thinLabels } from "./layout";
import { usePageTheme } from "@/components/liquid-charts/liquid/core/page-theme";
import { cssModule } from "@/components/liquid-charts/liquid/core/css-module";
import Roles from "@/components/liquid-charts/liquid/core/type.module.css";
import { LiquidCredit } from "@/components/liquid-charts/liquid/credit";
import type { FlatCell } from "./cell-canvas";
import Styles from "./cells.module.css";

const roles = cssModule(Roles, "lc-type-");
const styles = cssModule(Styles, "lc-cells-");

export type CalendarRange = { from: DayInput; to: DayInput; label?: ReactNode };

export type CalendarHeatmapProps = {
  /** Values by day: `{ date, value }` rows or a `{ "YYYY-MM-DD": value }` map. Days with no value
   * are etched empty cells. */
  data: CalendarData;
  /** First and last day shown. Default: the 365 days to the latest day in the data, or `months`
   * calendar months to it. */
  from?: DayInput;
  to?: DayInput;
  months?: number;
  /** Marked quietly with an inner ring. */
  today?: DayInput;
  /** A streak or selection, outlined. */
  range?: CalendarRange | null;
  /** Lets readers select a range: Shift with the arrow keys, or Shift-click. */
  onRangeChange?: (range: { from: string; to: string } | null) => void;
  config: ChartConfig;
  scale?: HeatScale;
  better?: "up" | "down";
  midpointLabel?: string;
  /** By default (`liquid`) each day is a pool of liquid in its exact scale colour, lit only at its
   * surface and walls. `glass` for a few months of days: glass cells whose liquid level is the
   * value. `flat` draws plain flat cells. */
  material?: "liquid" | "flat" | "glass";
  /** Days drawn etched, frosted lanes in their liquid: by default today (still counting) and any
   * day after it. */
  etched?: (date: string, value: number | null) => boolean;
  /** Light or dark. Without one, the page's: `data-theme` or a `dark` class on `<html>`, else the system setting. @default the page's */
  theme?: Theme;
  /** Below the width a year needs, fold it into two halves (each starting on a month). */
  fold?: CalendarFold;
  /** Cell size bounds, px. Under `minCell` even folded, the calendar scrolls inside its card. */
  minCell?: number;
  maxCell?: number;
  /** The day in focus, as days from `from`. */
  active?: number | null;
  onActiveChange?: (index: number | null) => void;
  tooltip?: boolean;
  tooltipFoot?: (date: string, value: number | null) => ReactNode;
  format?: ValueFormat;
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

const LONG = new Intl.DateTimeFormat("en-GB", {
  weekday: "long",
  day: "numeric",
  month: "long",
  year: "numeric",
  timeZone: "UTC",
});
/** "Wednesday 14 October 2026", the same in every engine. */
export const longDate = (ms: number) =>
  LONG.formatToParts(new Date(ms))
    .filter((part) => part.type !== "literal")
    .map((part) => part.value)
    .join(" ");
const WEEKDAYS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

/** Days [from, to] of a range as offsets inside the model, clipped to it; null when it misses. */
function rangeIn(model: CalendarModel, range: CalendarRange | null | undefined) {
  if (!range) return null;
  const a = Math.round((parseDay(range.from) - model.from) / DAY),
    b = Math.round((parseDay(range.to) - model.from) / DAY);
  if (!Number.isFinite(a) || !Number.isFinite(b)) return null;
  const lo = Math.max(0, Math.min(a, b)),
    hi = Math.min(model.days - 1, Math.max(a, b));
  return lo > hi ? null : { from: lo, to: hi, length: hi - lo + 1 };
}

/**
 * A year (or a few months) of days as liquid cells in their exact colour by value: weeks in
 * columns starting on Monday, month names from the core's calendar ticks, today ringed quietly and
 * etched (still counting), a streak or selection outlined. Left and Right step a day, Up and Down
 * a week. Narrow cards fold the year into two halves; narrower still, it scrolls inside the card,
 * never past it.
 */
export function CalendarHeatmap({
  data,
  from,
  to,
  months,
  today,
  range,
  onRangeChange,
  config,
  scale = { kind: "sequential" },
  better = "up",
  midpointLabel,
  material,
  etched,
  theme: themeProp,
  fold = "auto",
  minCell = 9,
  maxCell = 18,
  active: activeProp,
  onActiveChange,
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
}: CalendarHeatmapProps) {
  const theme = usePageTheme(themeProp);
  const [active, setActive] = useActive(activeProp, onActiveChange);
  const [outerRef, width] = useWidth();
  const anchor = useRef<number | null>(null);
  const model = calendarModel({ data, from, to, months });
  const left = Math.ceil(textWidth("Wed", TYPE.micro) + 10);
  const layout = calendarLayout({ model, width: width || 760, fold, left, minCell, maxCell });
  const frameHeight = layout.height + 2;
  const insets: Insets = { left, top: 18, right: 0, bottom: 2 };
  const keys = scaleKeys(scale, config);
  const colours = colourScale({
    scale,
    values: model.values,
    low: scale.kind === "diverging" ? configColours(config, keys[0], theme) : [],
    high: configColours(config, keys[scale.kind === "diverging" ? 1 : 0], theme),
    theme,
  });
  // A diverging scale's keys name its sides, not the measure.
  const measure =
    valueName ?? (scale.kind === "diverging" ? "Value" : plain(config[keys[0]]?.label, "Value"));
  const todayIndex = today !== undefined ? model.indexOf(today) : null;
  const selected = rangeIn(model, range);
  const style = resolveStyle({
    geometry: "cartesian",
    marks: ["cell"],
    colour: scale.kind,
    markCount: model.days,
    smallestMark: layout.cell,
    material,
  });
  useStyleWarnings(style.warnings, "CalendarHeatmap");
  const glass = style.materials.cell === "glass";
  const liquid = style.materials.cell === "liquid";
  const delay = (i: number) =>
    sweepDelay(model.weekday(i), model.week(i), 7, Math.max(2, model.weeks), 0.8);

  // Keep the focused day in view when the calendar scrolls inside its card.
  useEffect(() => {
    const element = outerRef.current;
    if (!element || !layout.scroll || active === null || active < 0 || active >= model.days) return;
    const r = layout.rect(active);
    if (r.left - 24 < element.scrollLeft) element.scrollLeft = Math.max(0, r.left - 24);
    else if (r.right + 24 > element.scrollLeft + element.clientWidth)
      element.scrollLeft = r.right + 24 - element.clientWidth;
  });

  const wrapperClass = `${styles.root} ${className ?? ""}`;
  const anyValue = model.values.some((v) => v !== null);
  const state = error ? "error" : loading ? "loading" : !anyValue ? "empty" : null;
  if (state)
    return (
      <ChartScope theme={theme} className={wrapperClass}>
        <div ref={outerRef}>
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
              <ChartState kind="loading" skeleton={{ grid: [7, Math.max(1, model.weeks)] }} />
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

  const etchedDay = (i: number, v: number | null) =>
    etched ? etched(dayKey(model.date(i)), v) : todayIndex !== null && i >= todayIndex;
  const flatLayout = (): FlatCell[] =>
    model.values.map((v, i) => {
      const r = layout.rect(i);
      return {
        etched: liquid && v !== null && etchedDay(i, v),
        key: `d${i}`,
        x: r.left,
        y: r.top,
        w: layout.cell,
        h: layout.cell,
        colour: v === null ? null : colours.colour(v),
        lit: 1,
        delay: delay(i),
        focus: active === i,
        grow: layout.cell >= 12 ? layout.gap : Math.min(1, layout.gap),
        mark: i === todayIndex,
      };
    });
  const glassLayout = (): BarMark[] =>
    model.values.map((v, i) => {
      const r = layout.rect(i);
      return {
        key: `d${i}`,
        cross: (r.left + r.right) / 2,
        base: r.bottom,
        thickness: layout.cell,
        length: layout.cell,
        direction: "up",
        tint: colours.tint(v ?? colours.domain[1]),
        lit: active === null || active === i ? 1 : 0.55,
        level: v === null ? 0 : Math.abs(colours.intensity(v)),
        pattern: v === null ? true : undefined,
        delay: delay(i),
      };
    });
  const overlay = selected
    ? () => rangeOutline(model, layout, selected.from, selected.to)
    : undefined;

  const select = (a: number, b: number) =>
    onRangeChange?.({
      from: dayKey(model.date(Math.min(a, b))),
      to: dayKey(model.date(Math.max(a, b))),
    });
  const onKeyDownCapture = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Escape") return;
    const next = calendarStep(active, event.key, model.days);
    if (next === undefined) return;
    event.preventDefault();
    event.stopPropagation();
    if (event.shiftKey && onRangeChange && next !== null) {
      anchor.current ??= active ?? next;
      select(anchor.current, next);
    } else anchor.current = null;
    if (next !== active) setActive(next);
  };
  const onClickCapture = (event: MouseEvent<HTMLDivElement>) => {
    if (!onRangeChange) return;
    const plot = (event.currentTarget as HTMLElement).querySelector('[role="group"]');
    if (!plot) return;
    const r = plot.getBoundingClientRect();
    const day = layout.at(event.clientX - r.left, event.clientY - r.top);
    if (day === null) return;
    if (event.shiftKey) {
      select(anchor.current ?? active ?? day, day);
      event.stopPropagation();
      setActive(day);
    } else anchor.current = day;
  };

  const formatted = (v: number | null) => (v === null ? "no data" : formatValue(v, format));
  const rangeName = plain(range?.label, "selected range");
  const describeDay = (i: number) => {
    const notes = [
      i === todayIndex ? "today" : "",
      selected && i >= selected.from && i <= selected.to
        ? `day ${i - selected.from + 1} of ${selected.length} in the ${rangeName.toLowerCase()}`
        : "",
    ].filter(Boolean);
    return `${longDate(model.date(i))}: ${measure} ${formatted(model.values[i])}${notes.length ? `, ${notes.join(", ")}` : ""}.`;
  };
  const activeValue =
    active !== null && active >= 0 && active < model.days ? model.values[active] : null;

  return (
    <ChartScope theme={theme} className={wrapperClass}>
      <div
        ref={outerRef}
        className={styles.scroller}
        data-scroll={layout.scroll}
        onKeyDownCapture={onKeyDownCapture}
        onClickCapture={onClickCapture}
      >
        <div style={layout.scroll ? { width: layout.width } : undefined}>
          <ChartFrame
            credit={<LiquidCredit />}
            theme={theme}
            height={frameHeight}
            insets={insets}
            count={model.days}
            active={active}
            onActiveChange={setActive}
            pick={(x, y) => layout.at(x, y)}
            label={label}
            describe={describe ?? describeDay}
            table={{
              caption: label,
              columns: ["Date", measure],
              rows: model.values.map((v, i) => [longDate(model.date(i)), formatted(v)]),
            }}
          >
            {glass ? (
              <UnitLayer theme={theme} replay={replay} material="glass" layout={glassLayout} />
            ) : liquid ? (
              <CellLayer theme={theme} replay={replay} layout={flatLayout} overlay={overlay} />
            ) : (
              <FlatCellLayer theme={theme} replay={replay} layout={flatLayout} overlay={overlay} />
            )}
            <CalendarAxes model={model} layout={layout} active={active} />
            {tooltip && active !== null && active >= 0 && active < model.days && (
              <DayTooltip
                layout={layout}
                index={active}
                title={longDate(model.date(active))}
                measure={measure}
                value={model.values[active]}
                colours={colours}
                format={format}
                better={better}
                midpointLabel={midpointLabel}
                foot={tooltipFoot?.(dayKey(model.date(active)), model.values[active])}
                today={active === todayIndex}
                range={
                  selected && active >= selected.from && active <= selected.to
                    ? {
                        day: active - selected.from + 1,
                        length: selected.length,
                        name: range?.label,
                      }
                    : null
                }
              />
            )}
          </ChartFrame>
        </div>
      </div>
      {legend && (
        <div className={styles.legendRow} style={{ paddingLeft: Math.max(0, left - 2) }}>
          <GradientLegend
            scale={colours}
            format={format}
            name={measure}
            value={activeValue}
            midLabel={midpointLabel}
            missing={model.values.some((v) => v === null)}
            background={glass ? glassStrength(colours) : undefined}
            extra={
              <>
                {todayIndex !== null && (
                  <CellKey
                    mark="today"
                    etched={liquid && etchedDay(todayIndex, model.values[todayIndex])}
                    colour={
                      model.values[todayIndex] === null
                        ? undefined
                        : colours.colour(model.values[todayIndex]!)
                    }
                  >
                    Today
                  </CellKey>
                )}
                {selected && <CellKey mark="range">{range?.label ?? "Selected"}</CellKey>}
              </>
            }
          />
        </div>
      )}
    </ChartScope>
  );
}

const glassStrength = (colours: ColourScale) => strengthCss(colours.tint(colours.domain[1]), 0.08);

/* ——— Month and weekday names ——— */

function CalendarAxes({
  model,
  layout,
  active,
}: {
  model: CalendarModel;
  layout: CalendarLayout;
  active: number | null;
}) {
  const ticks = monthTicks(model);
  const activeMonth =
    active !== null && active >= 0 && active < model.days
      ? new Date(model.date(active)).getUTCMonth()
      : null;
  const lineHeight = 13;
  return (
    <div className={styles.axes} aria-hidden="true">
      {layout.bands.map((band, b) => {
        const own = ticks.filter((t) => t.week >= band.from && t.week < band.to);
        const texts = own.map((t) => (t.month === 0 ? `${t.label} ${t.year}` : t.label));
        const starts = own.map((t) => layout.left + (t.week - band.from) * layout.pitch);
        const widths = texts.map((text) => textWidth(text, TYPE.micro) + 2);
        const thin = thinLabels({
          centres: starts.map((s, i) => s + widths[i] / 2),
          extents: widths,
          lo: layout.left,
          hi: Math.max(layout.width, layout.left + 1),
          gap: 8,
        });
        const weekdays = thinLabels({
          centres: WEEKDAYS.map((_, d) => band.top + d * layout.pitch + layout.cell / 2),
          extents: WEEKDAYS.map(() => lineHeight),
          lo: band.top - 2,
          hi: band.top + 7 * layout.pitch,
          gap: 1,
        });
        return (
          <div key={b}>
            {own.map((t, i) => (
              <span
                key={t.value}
                className={`${styles.monthLabel} ${roles.micro}`}
                data-shown={thin.visible[i]}
                data-active={activeMonth === t.month}
                style={{ left: thin.starts[i], top: band.labelTop }}
              >
                {t.label}
                {t.month === 0 && <span className={styles.year}>{t.year}</span>}
              </span>
            ))}
            {WEEKDAYS.map((name, d) => (
              <span
                key={name}
                className={`${styles.weekdayLabel} ${roles.micro}`}
                data-shown={weekdays.visible[d]}
                style={{
                  right: `calc(100% - ${layout.left - 8}px)`,
                  top: band.top + d * layout.pitch + layout.cell / 2,
                }}
              >
                {name}
              </span>
            ))}
          </div>
        );
      })}
    </div>
  );
}

/* ——— Tooltip ——— */

function DayTooltip({
  layout,
  index,
  title,
  measure,
  value,
  colours,
  format,
  better,
  midpointLabel,
  foot,
  today,
  range,
}: {
  layout: CalendarLayout;
  index: number;
  title: string;
  measure: string;
  value: number | null;
  colours: ColourScale;
  format: ValueFormat;
  better: "up" | "down";
  midpointLabel?: string;
  foot?: ReactNode;
  today: boolean;
  range: { day: number; length: number; name?: ReactNode } | null;
}) {
  const tip = useRef<HTMLDivElement>(null);
  const { size } = useChartFrame();
  useIsomorphicLayoutEffect(() => {
    const element = tip.current;
    if (!element || !size.width) return;
    const r = layout.rect(index);
    const grow = layout.gap + 1;
    placeAt(
      element,
      quadrantTooltipPosition(
        { left: r.left - grow, top: r.top - grow, right: r.right + grow, bottom: r.bottom + grow },
        { width: element.offsetWidth, height: element.offsetHeight },
        { left: 0, top: 0, right: size.width, bottom: size.height },
      ),
    );
  });
  const notes: ReactNode[] = [];
  if (foot !== undefined) notes.push(<span key="foot">{foot}</span>);
  if (value !== null && colours.kind === "diverging" && colours.midpoint !== null) {
    const diff = value - colours.midpoint;
    const good = diff === 0 || diff > 0 === (better === "up");
    notes.push(
      <span key="mid" className={`${roles.caption} ${styles.tipNote}`} data-good={good}>
        <Fig value={Math.abs(diff)} format={format} signed={false} className="" />{" "}
        {diff >= 0 ? "above" : "below"} {midpointLabel ?? "the midpoint"}
      </span>,
    );
  }
  if (range)
    notes.push(
      <TooltipFoot key="range">
        Day {range.day} of {range.length} · {range.name ?? "selected"}
      </TooltipFoot>,
    );
  if (today) notes.push(<TooltipFoot key="today">Today</TooltipFoot>);
  return (
    <ChartTooltip
      ref={tip}
      title={title}
      rows={[
        {
          label: measure,
          color: value === null ? "var(--chart-ink-3)" : cssRgb(colours.colour(value)),
          muted: value === null,
          value:
            value === null ? (
              <span className={`${roles.caption} ${mutedText}`}>No data</span>
            ) : (
              <Fig value={value} format={format} />
            ),
        },
      ]}
      footer={notes.length ? <>{notes}</> : undefined}
    />
  );
}
