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
  CategoryAxis,
  ChartFrame,
  ChartState,
  cssToRgb,
  Delta,
  eventLanes as eventLanesFor,
  EventMarkers,
  Figure,
  FocusMarker,
  formatValue,
  Gridlines,
  LightLineLayer,
  mutedText,
  noop,
  placeTooltip,
  seriesColour,
  stagger,
  textWidth,
  tickFormatter,
  TYPE,
  useActive,
  useChartFrame,
  useValueLabels,
  ValueLabel,
  valueTicks,
  type BarFrame,
  type BarMark,
  type ChartConfig,
  type Insets,
  type LabelMode,
  type Rgb,
  type StyleSpec,
  type Theme,
} from "@/components/liquid-charts/liquid/core";
import {
  CANDLE_DENSITY,
  candleExtent,
  candleGeometry,
  candleInsets,
  candleLabelPlan,
  candleModel,
  complete,
  polylinePath,
  readCandles,
  sessionIndex,
  sessionLabelVisibility,
  sessionTicks,
  timeValue,
  wicks,
  type CandleDensity,
  type CandleGeometry,
  type CandleModel,
  type TimeRow,
} from "./layout";
import {
  forwardRef,
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type ForwardedRef,
  type ReactNode,
} from "react";
import { BrushStrip, useSpringValues } from "@/components/liquid-charts/liquid/zoom";
import { usePageTheme } from "@/components/liquid-charts/liquid/core/page-theme";
import type { ValueFormat } from "@/components/liquid-charts/liquid/core/format";
import { cssModule } from "@/components/liquid-charts/liquid/core/css-module";
import Roles from "@/components/liquid-charts/liquid/core/type.module.css";
import Core from "@/components/liquid-charts/liquid/core/core.module.css";
import { LiquidCredit } from "@/components/liquid-charts/liquid/credit";
import Styles from "./time.module.css";
import { BarLayer } from "./marks";

const roles = cssModule(Roles, "lc-type-");
const core = cssModule(Core, "lc-core-");
const styles = cssModule(Styles, "lc-time-");

export type CandleTone = "neutral" | "good" | "bad" | "accent";
/** A moment on the axis (earnings, a split): by time (it lands on the first session on or after
 * it) or by session index in `data`. */
export type CandleEvent = {
  at?: Date | number | string;
  index?: number;
  label: string;
  tone?: CandleTone;
};

/** A brush on an overview of the whole history, choosing the sessions the chart shows. */
export type CandleBrush = {
  /** The sessions shown, controlled: the first and last index into `data`; null for all. */
  range?: readonly [number, number] | null;
  /** The starting sessions when uncontrolled. Default: the latest `range` sessions (or all). */
  defaultRange?: readonly [number, number] | null;
  /** Every change, live while dragging; null when cleared. */
  onRangeChange?: (range: [number, number] | null) => void;
  /** The fewest sessions the window holds. Default 5. */
  minSessions?: number;
  /** The overview strip's height, px. */
  height?: number;
  /** The overview's accessible name. Default "Choose the sessions shown". */
  label?: string;
};

export type CandlestickProps<Row extends TimeRow> = {
  /** One row per trading session, oldest first. Rows before `range` feed the moving average. */
  data: readonly Row[];
  /** Colours: `up` and `down` for the candles, `average` for the moving average. */
  config: ChartConfig;
  dateKey?: keyof Row & string;
  openKey?: keyof Row & string;
  highKey?: keyof Row & string;
  lowKey?: keyof Row & string;
  closeKey?: keyof Row & string;
  /** The volume field, or null for no volume pane. */
  volumeKey?: (keyof Row & string) | null;
  /** Show the latest this many sessions (1M is about 21). Default: all. With a brush, the window
   * it starts with. */
  range?: number | null;
  /** An overview strip of the whole history (its closes as a light line) with a brush that
   * chooses the sessions shown: drag across it, drag its handles or window, or use the keyboard.
   * The candles, axes and price scale spring to each window. Presets (1M, 3M) set it through
   * `range` in the options. */
  brush?: boolean | CandleBrush;
  /** Sessions in the moving average; 0 for none. Default 20. */
  average?: number;
  /** Its name in the tooltip and table. Default "20-day average". */
  averageLabel?: string;
  events?: readonly CandleEvent[];
  /** Label lanes for events before a crowded one shows on hover only. Default 2. */
  eventLanes?: number;
  /** Light or dark. Without one, the page's: `data-theme` or a `dark` class on `<html>`, else the system setting. @default the page's */
  theme?: Theme;
  height?: number;
  density?: CandleDensity;
  active?: number | null;
  onActiveChange?: (index: number | null) => void;
  /** `auto` marks the period's high and low; `ends` the first and latest close. */
  labels?: LabelMode;
  tooltip?: boolean;
  /** Prices. Default USD with cents. */
  format?: ValueFormat;
  volumeFormat?: ValueFormat;
  /** Forces glass or flat bodies; otherwise the style rules decide (thin candles draw flat). */
  material?: "glass" | "flat";
  /** The time zone dates are read in. Default UTC. */
  timeZone?: string;
  loading?: boolean;
  empty?: ReactNode;
  error?: ReactNode;
  /** Change to replay the entrance. */
  replay?: number;
  label: string;
  /** One sentence for the focused session (an index into the sessions shown), read aloud. */
  describe?: (index: number) => string;
  className?: string;
};

const FALLBACK: Rgb = [0.56, 0.56, 0.56];

/**
 * Daily prices as candles: glass bodies that fill from the open towards the close (up in the
 * rising colour, down in the falling one), wicks as fine flat strokes, volume as quieter glass in
 * a pane beneath on the same sessions, a moving average as a light line, and the latest close as a
 * breathing black hole with its price at the edge. The axis runs over trading sessions, so
 * weekends and holidays leave no false gaps, and is labelled where months or weeks begin.
 * Earnings and splits stand in lanes above. Hover or step with the keyboard for a crosshair and
 * the session's open, high, low, close, volume and change.
 */
export function Candlestick<Row extends TimeRow>({
  data,
  config,
  dateKey = "date" as keyof Row & string,
  openKey = "open" as keyof Row & string,
  highKey = "high" as keyof Row & string,
  lowKey = "low" as keyof Row & string,
  closeKey = "close" as keyof Row & string,
  volumeKey = "volume" as keyof Row & string,
  range = null,
  brush: brushProp,
  average = 20,
  averageLabel,
  events = [],
  eventLanes = 2,
  theme: themeProp,
  height,
  density = "standard",
  active: activeProp,
  onActiveChange,
  labels = "auto",
  tooltip = true,
  format = { kind: "currency", decimals: 2 },
  volumeFormat = { kind: "compact", decimals: 1 },
  material,
  timeZone = "UTC",
  loading = false,
  empty,
  error,
  replay = 0,
  label,
  describe,
  className,
}: CandlestickProps<Row>) {
  const theme = usePageTheme(themeProp);
  const [active, setActive] = useActive(activeProp, onActiveChange);
  const valueLabels = useValueLabels();
  const tip = useRef<HTMLDivElement>(null);

  const all = readCandles(data, {
    date: dateKey,
    open: openKey,
    high: highKey,
    low: lowKey,
    close: closeKey,
    volume: volumeKey ?? undefined,
  });
  /* ——— The brushed window: the sessions shown, and a view springing to them ——— */
  const brush: CandleBrush | null = brushProp ? (brushProp === true ? {} : brushProp) : null;
  const total = all.length;
  const preset: [number, number] | null =
    range !== null && Number.isFinite(range) && range < total
      ? [Math.max(0, total - Math.floor(range)), total - 1]
      : null;
  const [innerWindow, setInnerWindow] = useState<readonly [number, number] | null>(
    brush?.defaultRange !== undefined ? brush.defaultRange : preset,
  );
  const rawWindow = brush ? (brush.range === undefined ? innerWindow : brush.range) : null;
  const pin = (v: number) => Math.max(0, Math.min(total - 1, Math.round(v)));
  const chosen: [number, number] | null =
    rawWindow && total > 0
      ? [pin(Math.min(rawWindow[0], rawWindow[1])), pin(Math.max(rawWindow[0], rawWindow[1]))]
      : null;
  const setWindow = (next: [number, number] | null) => {
    if (brush?.range === undefined) setInnerWindow(next);
    brush?.onRangeChange?.(next);
  };
  // Session i spans [i, i + 1): the view runs over those edges.
  const targetView: [number, number] = chosen ? [chosen[0], chosen[1] + 1] : [0, total];
  const targetModel = brush ? candleModel(all, { window: targetView, average }) : null;
  const priceTarget = targetModel?.priceDomain ?? [0, 1];
  const priceRest = Math.abs(priceTarget[1] - priceTarget[0]) * 1e-4 || 1e-6;
  const spring = useSpringValues(
    brush ? [targetView[0], targetView[1], priceTarget[0], priceTarget[1]] : [0, 0, 0, 0],
    { rest: [1e-3, 1e-3, priceRest, priceRest] },
  );
  const view: [number, number] = [spring.values[0], spring.values[1]];
  // While the view springs, every candle is laid out afresh each frame (so wicks, bodies and the
  // average move as one); at rest they spring between data as usual.
  const following = !!brush && spring.moving;
  const model = brush
    ? candleModel(all, {
        window: view,
        average,
        priceDomain: [spring.values[2], spring.values[3]],
      })
    : candleModel(all, { range, average });
  const count = model.candles.length;
  const times = model.candles.map((c) => c.time);
  const averageName = averageLabel ?? `${average}-day average`;
  const motionKey = following ? `@${view[0].toFixed(3)}:${view[1].toFixed(3)}` : "";
  // The newest session's tag only reads as "latest" when the window reaches it.
  const reachesEnd = !brush || (targetModel?.latest !== null && targetView[1] >= total);

  // A new range is a new set of candles: they pour in again rather than slide. A brushed window
  // slides instead.
  const rangeKey = brush ? "brush" : `${model.offset}:${count}`;
  const [cycle, setCycle] = useState({ key: rangeKey, n: 0 });
  if (cycle.key !== rangeKey) setCycle({ key: rangeKey, n: cycle.n + 1 });
  const entrance = replay * 1000 + cycle.n;

  const frameHeight = height ?? CANDLE_DENSITY[density].height;
  const frameHeightFor = () => frameHeight;
  const ticks = valueTicks(model.priceDomain, CANDLE_DENSITY[density].ticks);
  const formatTick = tickFormatter(
    { ...format, decimals: 0 },
    targetModel?.priceDomain ?? model.priceDomain,
  );
  const latest = model.latest === null ? null : model.candles[model.latest];
  const placed = events
    .map((event) => ({
      event,
      index:
        event.index !== undefined
          ? event.index - model.offset
          : sessionIndex(times, timeValue(event.at)),
    }))
    .filter(
      (e): e is { event: CandleEvent; index: number } =>
        e.index !== null && e.index >= 0 && e.index < count,
    );
  // Event tags take as many lanes as they need at this width (measured after the first frame).
  const [measured, setMeasured] = useState(0);
  const insetTicks = targetModel
    ? valueTicks(targetModel.priceDomain, CANDLE_DENSITY[density].ticks)
    : ticks;
  const lastClose = brush
    ? [...all].reverse().find(complete)?.close
    : latest
      ? latest.close
      : undefined;
  const baseInsets = (lanes: number): Insets =>
    candleInsets({
      ticks: insetTicks.map(formatTick),
      latest: lastClose !== undefined ? formatValue(lastClose, format) : undefined,
      lanes,
      density,
    });
  const cap = CANDLE_DENSITY[density].thickness;
  const lanesFor = (width: number) => {
    if (!placed.length) return 0;
    if (!width) return Math.min(placed.length, Math.max(1, eventLanes));
    const g = candleGeometry(model, {
      width,
      height: frameHeightFor(),
      insets: baseInsets(1),
      maxThickness: cap,
    });
    const order = placed
      .map((e) => ({ x: g.slots.centre(e.index), label: e.event.label }))
      .sort((a, b) => a.x - b.x);
    const used = eventLanesFor(order, { lanes: Math.max(1, eventLanes) });
    return Math.max(1, ...used.map((lane) => (lane ?? 0) + 1));
  };
  const lanes = lanesFor(measured);
  const insets: Insets = baseInsets(lanes);
  const geometryFor = (width: number, h: number) =>
    candleGeometry(model, { width, height: h, insets, maxThickness: cap });

  const colourOf = (key: string) => seriesColour(config, key, theme);
  const upColour = colourOf("up"),
    downColour = colourOf("down");
  const upTint = cssToRgb(upColour) ?? FALLBACK,
    downTint = cssToRgb(downColour) ?? FALLBACK;
  const averageColour = config.average ? colourOf("average") : null;
  const pace = stagger(count);

  const title = (i: number) =>
    new Intl.DateTimeFormat("en-US", {
      weekday: "short",
      month: "short",
      day: "numeric",
      year: "numeric",
      timeZone,
    }).format(times[i]);
  const shortDate = (i: number) =>
    new Intl.DateTimeFormat("en-US", { month: "short", day: "numeric", timeZone }).format(times[i]);

  const bodies = (width: number, h: number): BarMark[] => {
    const g = geometryFor(width, h);
    return g.shapes.flatMap((s) =>
      s
        ? [
            {
              key: `c${times[s.index]}${motionKey}`,
              cross: s.cross,
              base: s.base,
              length: s.length,
              thickness: g.thickness,
              direction: s.direction,
              tint: s.up ? upTint : downTint,
              lit: active === null || active === s.index ? 1 : 0.5,
              delay: s.index * pace,
            },
          ]
        : [],
    );
  };
  const volumes = (width: number, h: number): BarMark[] => {
    const g = geometryFor(width, h);
    return g.shapes.flatMap((s) =>
      s?.volume
        ? [
            {
              key: `v${times[s.index]}${motionKey}`,
              cross: s.cross,
              base: s.volume.base,
              length: s.volume.length,
              thickness: g.thickness,
              direction: "up" as const,
              tint: s.up ? upTint : downTint,
              // Volume is the quieter pane: half lit at rest, the focused session's in full.
              lit: active === null ? 0.5 : active === s.index ? 1 : 0.12,
              delay: 0.15 + s.index * pace,
            },
          ]
        : [],
    );
  };

  const plan = candleLabelPlan(model, labels);
  const onFrame = (_bars: BarFrame[], width: number, h: number) => {
    const g = geometryFor(width, h);
    // Event tags are obstacles for the high and low labels.
    const tags = placed
      .map((e) => ({ x: g.slots.centre(e.index), label: e.event.label }))
      .sort((a, b) => a.x - b.x);
    const tagLanes = eventLanesFor(tags, { lanes: Math.max(1, eventLanes) });
    const obstacles = tags.flatMap((tag, k) => {
      const lane = tagLanes[k];
      if (lane === null) return [];
      const half = textWidth(tag.label, TYPE.micro) / 2 + 8;
      const top = g.plot.top - 18 - lane * 20;
      return [{ left: tag.x - half, right: tag.x + half, top, bottom: top + 18 }];
    });
    valueLabels.place(
      plan.map((item) => {
        const s = g.shapes[item.index];
        if (!s || active !== null) return { id: item.id, x: 0, y: 0, side: item.side, priority: 0 };
        const y =
          item.kind === "high" ? s.high : item.kind === "low" ? s.low : Math.min(s.high, s.close);
        return { id: item.id, x: s.cross, y, side: item.side, offset: 5, flip: false, priority: 2 };
      }),
      {
        bounds: {
          left: g.plot.left - 4,
          top: 0,
          right: g.plot.right + 4,
          bottom: g.price.bottom + 6,
        },
        obstacles,
      },
    );
    const card = tip.current;
    const s = active === null ? null : g.shapes[active];
    if (card && s) {
      placeTooltip(card, candleExtent(s, g.thickness), {
        left: 4,
        top: g.plot.top,
        right: width - 4,
        bottom: g.plot.bottom,
      });
    }
  };

  /* ——— States ——— */
  const hasData = model.candles.some(complete);
  if (error || loading || !hasData) {
    const kind = error ? "error" : loading ? "loading" : "empty";
    return (
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
        className={className}
      >
        <ChartState
          kind={kind}
          message={kind === "error" ? (error === true ? undefined : error) : empty}
          bars={Math.max(6, Math.min(14, count || 12))}
        />
      </ChartFrame>
    );
  }

  /* ——— Reading aloud and the data table ——— */
  const price = (value: number) => formatValue(value, format);
  const describeSession = (i: number) => {
    const c = model.candles[i];
    if (!complete(c)) return `${title(i)}: no trading data.`;
    const change = model.change[i];
    const moves = !Number.isFinite(change)
      ? ""
      : change === 0
        ? ", unchanged on the previous close"
        : `, ${change > 0 ? "up" : "down"} ${formatValue(Math.abs(change), { kind: "percent", decimals: 2 })} on the previous close`;
    const volume = Number.isFinite(c.volume)
      ? `, volume ${formatValue(c.volume, volumeFormat)}`
      : "";
    const notes = placed.filter((e) => e.index === i).map((e) => `, ${e.event.label}`);
    return `${title(i)}: open ${price(c.open)}, high ${price(c.high)}, low ${price(c.low)}, close ${price(c.close)}${moves}${volume}${notes.join("")}.`;
  };
  const columns = [
    "Date",
    "Open",
    "High",
    "Low",
    "Close",
    "Change",
    ...(model.hasVolume ? ["Volume"] : []),
    ...(average > 0 ? [averageName] : []),
  ];
  const tableRows = model.candles.map((c, i) => [
    title(i),
    price(c.open),
    price(c.high),
    price(c.low),
    price(c.close),
    formatValue(model.change[i], { kind: "percent", decimals: 2, signed: true }),
    ...(model.hasVolume ? [formatValue(c.volume, volumeFormat)] : []),
    ...(average > 0 ? [price(model.average[i])] : []),
  ]);

  const spec: StyleSpec = {
    geometry: "cartesian",
    marks: ["solid", "stroke"],
    axis: "discrete",
    measure: "amount",
    series: 1,
    markCount: count,
    density,
  };

  const chart = (
    <ChartFrame
      credit={<LiquidCredit />}
      theme={theme}
      height={frameHeight}
      insets={insets}
      count={count}
      active={active}
      onActiveChange={setActive}
      pick={(x, y, size) => {
        const g = geometryFor(size.width, size.height);
        if (y < g.plot.top - 12 || y > g.plot.bottom + 8) return null;
        if (x < g.plot.left - 12 || x > size.width) return null;
        // The latest price's tag sits right of the plot; pointing there reads the latest session.
        return g.slots.indexAt(Math.max(g.plot.left, Math.min(g.plot.right, x)));
      }}
      label={label}
      describe={describe ?? describeSession}
      table={{ caption: label, columns, rows: tableRows }}
      className={`${styles.frame} ${brush ? "" : (className ?? "")}`}
    >
      <ReportWidth onWidth={setMeasured} />
      <Axes
        geometryFor={geometryFor}
        model={model}
        ticks={ticks}
        formatTick={formatTick}
        volumeFormat={volumeFormat}
        timeZone={timeZone}
        shortDate={shortDate}
      />
      <EventLayer
        geometryFor={geometryFor}
        model={model}
        events={placed}
        lanes={Math.max(1, eventLanes)}
      />
      <Crosshair geometryFor={geometryFor} />
      <Wicks
        key={`w${entrance}`}
        geometryFor={geometryFor}
        up={upColour}
        down={downColour}
        pace={pace}
      />
      {model.hasVolume && (
        <Bodies
          theme={theme}
          replay={entrance}
          spec={spec}
          material={material}
          geometryFor={geometryFor}
          layout={volumes}
          onFrame={noop}
          className={styles.volumeLayer}
        />
      )}
      <Bodies
        theme={theme}
        replay={entrance}
        spec={spec}
        material={material}
        geometryFor={geometryFor}
        layout={bodies}
        onFrame={onFrame}
      />
      {averageColour && average > 0 && (
        <AverageLine
          key={`a${entrance}`}
          theme={theme}
          geometryFor={geometryFor}
          model={model}
          colour={averageColour}
          replay={entrance}
        />
      )}
      {reachesEnd && (
        <Latest
          geometryFor={geometryFor}
          model={model}
          format={format}
          colour={latest && latest.close >= latest.open ? upColour : downColour}
          visible={active === null}
        />
      )}
      {plan.map((item) => (
        <ValueLabel key={item.id} ref={valueLabels.ref(item.id)}>
          <span className={styles.extreme}>
            <span className={`${roles.micro} ${mutedText}`}>
              {item.kind === "high" ? "High" : item.kind === "low" ? "Low" : shortDate(item.index)}
            </span>
            <Figure
              value={item.value}
              kind={format.kind}
              decimals={format.decimals}
              className={roles.figureS}
            />
          </span>
        </ValueLabel>
      ))}
      {tooltip && active !== null && active < count && complete(model.candles[active]) && (
        <CandleTooltip
          ref={tip}
          index={active}
          model={model}
          title={title(active)}
          format={format}
          volumeFormat={volumeFormat}
          averageName={average > 0 && averageColour ? averageName : null}
          averageColour={averageColour}
          notes={placed.filter((e) => e.index === active).map((e) => e.event.label)}
          colour={model.candles[active].close >= model.candles[active].open ? upColour : downColour}
        />
      )}
    </ChartFrame>
  );
  if (!brush) return chart;

  // The overview: every close as a light line, on the same band centres as the candles.
  const closes = all.map((c) => (complete(c) ? c.close : null));
  const day = new Intl.DateTimeFormat("en-US", { month: "short", day: "numeric", timeZone });
  const long = new Intl.DateTimeFormat("en-US", {
    month: "long",
    day: "numeric",
    year: "numeric",
    timeZone,
  });
  // A window edge sits between sessions: the left edge names the session after it, the right
  // edge the session before it.
  const sessionAt = (edge: number, side: "from" | "to") =>
    Math.max(0, Math.min(total - 1, Math.round(side === "from" ? edge : edge - 1)));
  return (
    <WithOverview className={className}>
      {chart}
      <BrushStrip
        theme={theme}
        height={brush.height}
        insets={{ left: insets.left, right: insets.right }}
        extent={[0, total]}
        range={chosen ? [chosen[0], chosen[1] + 1] : null}
        onRangeChange={(next) =>
          setWindow(next ? [sessionAt(next[0], "from"), sessionAt(next[1], "to")] : null)
        }
        snap="integer"
        minSpan={Math.max(2, Math.min(total, brush.minSessions ?? 5))}
        colour={config.close ? colourOf("close") : upColour}
        lines={[
          {
            key: "close",
            colour: config.close ? colourOf("close") : upColour,
            values: closes,
            xs: closes.map((_, i) => i + 0.5),
          },
        ]}
        format={(v, edge) => <span>{day.format(all[sessionAt(v, edge)]?.time ?? 0)}</span>}
        describe={(r) => {
          if (!r)
            return `All ${total} sessions, ${long.format(all[0].time)} to ${long.format(all[total - 1].time)}`;
          const a = sessionAt(r[0], "from"),
            b = sessionAt(r[1], "to");
          return `${long.format(all[a].time)} to ${long.format(all[b].time)}, ${b - a + 1} of ${total} sessions`;
        }}
        label={brush.label ?? "Choose the sessions shown"}
      />
    </WithOverview>
  );
}

/** A chart with its overview strip under it. */
function WithOverview({ className, children }: { className?: string; children: ReactNode }) {
  return (
    <div className={className} style={{ display: "grid", gap: 10, minWidth: 0 }}>
      {children}
    </div>
  );
}

/** Reports the frame's width once it is measured, and again whenever it changes. */
function ReportWidth({ onWidth }: { onWidth: (width: number) => void }) {
  const { size } = useChartFrame();
  useEffect(() => {
    if (size.width) onWidth(Math.round(size.width));
  }, [size.width, onWidth]);
  return null;
}

/* ——— Bodies: glass, or flat when the candles are too thin ——— */

function Bodies({
  geometryFor,
  ...props
}: {
  theme: Theme;
  replay: number;
  spec: StyleSpec;
  material?: "glass" | "flat";
  geometryFor: (width: number, height: number) => CandleGeometry;
  layout: (width: number, height: number) => BarMark[];
  onFrame: (bars: BarFrame[], width: number, height: number) => void;
  className?: string;
}) {
  const { size } = useChartFrame();
  const thickness = size.width ? geometryFor(size.width, size.height).thickness : undefined;
  return <BarLayer {...props} smallest={thickness} />;
}

/* ——— Axes ——— */

function Axes({
  geometryFor,
  model,
  ticks,
  formatTick,
  volumeFormat,
  timeZone,
  shortDate,
}: {
  geometryFor: (width: number, height: number) => CandleGeometry;
  model: CandleModel;
  ticks: readonly number[];
  formatTick: (value: number) => string;
  volumeFormat: ValueFormat;
  timeZone: string;
  shortDate: (index: number) => string;
}) {
  const { size, active } = useChartFrame();
  if (!size.width) return null;
  const g = geometryFor(size.width, size.height);
  const times = model.candles.map((c) => c.time);
  const marks = sessionTicks(times, g.plot.right - g.plot.left, { timeZone });
  const shown = sessionLabelVisibility(marks, g.slots.centre);
  // The focused session's date sits in a pill; calendar labels near it step aside.
  const room = active !== null ? 44 : 0;
  const labels = model.candles.map(() => "" as ReactNode);
  const inside = (i: number) =>
    g.slots.centre(i) >= g.plot.left - 1 && g.slots.centre(i) <= g.plot.right + 1;
  marks.forEach((mark, k) => {
    if (!shown[k] || !inside(mark.index)) return;
    if (active !== null && Math.abs(g.slots.centre(mark.index) - g.slots.centre(active)) < room)
      return;
    labels[mark.index] = mark.label;
  });
  if (active !== null) labels[active] = shortDate(active);
  const volumeTop = model.volumeDomain[1];
  const volumeTick = tickFormatter(volumeFormat, model.volumeDomain);
  return (
    <>
      <Gridlines
        values={ticks.filter((t) => g.y(t) >= g.price.top - 1 && g.y(t) <= g.price.bottom + 1)}
        position={g.y}
        format={formatTick}
      />
      {g.volume && (
        <>
          <span className={styles.paneRule} style={{ top: g.volume.bottom }} aria-hidden="true" />
          <span
            className={`${styles.paneTick} ${roles.micro}`}
            style={{ top: g.v(volumeTop), right: `calc(100% - ${g.plot.left - 10}px)` }}
            aria-hidden="true"
          >
            {volumeTick(volumeTop)}
          </span>
        </>
      )}
      <CategoryAxis labels={labels} position={g.slots.centre} active={active} pill />
    </>
  );
}

function EventLayer({
  geometryFor,
  model,
  events,
  lanes,
}: {
  geometryFor: (width: number, height: number) => CandleGeometry;
  model: CandleModel;
  events: readonly { event: CandleEvent; index: number }[];
  lanes: number;
}) {
  const { size } = useChartFrame();
  if (!size.width || !events.length) return null;
  const g = geometryFor(size.width, size.height);
  /* An event's line stops 6px short of every candle (and volume bar) within 6px of it, its own
   * session's included, so it never runs through the glass or past the latest close. */
  const clearOf = (x: number) =>
    model.candles.flatMap((candle, i) => {
      if (Math.abs(g.slots.centre(i) - x) > g.thickness / 2 + 6) return [];
      const spans: (readonly [number, number])[] = [];
      if (Number.isFinite(candle.high) && Number.isFinite(candle.low))
        spans.push([g.y(candle.high) - 6, g.y(candle.low) + 6]);
      if (g.volume && Number.isFinite(candle.volume) && candle.volume > 0)
        spans.push([g.v(candle.volume) - 6, g.volume.bottom]);
      return spans;
    });
  return (
    <EventMarkers
      lanes={lanes}
      events={events
        .filter(
          ({ index }) =>
            g.slots.centre(index) >= g.plot.left - 1 && g.slots.centre(index) <= g.plot.right + 1,
        )
        .map(({ event, index }) => {
          const x = Math.round(g.slots.centre(index)) + 0.5;
          return { x, label: event.label, tone: event.tone, clear: clearOf(x) };
        })}
    />
  );
}

/** A crosshair through the focused session: a hairline through both panes and a quiet rule at its
 * close. Flat, with the axes. */
function Crosshair({
  geometryFor,
}: {
  geometryFor: (width: number, height: number) => CandleGeometry;
}) {
  const { size, active } = useChartFrame();
  if (!size.width || active === null) return null;
  const g = geometryFor(size.width, size.height);
  const s = g.shapes[active];
  if (!s) return null;
  const x = Math.round(s.cross) + 0.5;
  return (
    <>
      <span
        className={styles.crossX}
        style={{ left: x, top: g.plot.top, height: g.plot.bottom - g.plot.top }}
        aria-hidden="true"
      />
      <span
        className={styles.crossY}
        style={{ top: Math.round(s.close) + 0.5, left: g.plot.left, width: s.cross - g.plot.left }}
        aria-hidden="true"
      />
    </>
  );
}

/** Wicks: fine flat strokes from the high to the body and from the body to the low, fading in as
 * each body fills. */
function Wicks({
  geometryFor,
  up,
  down,
  pace,
}: {
  geometryFor: (width: number, height: number) => CandleGeometry;
  up: string;
  down: string;
  pace: number;
}) {
  const { size, active } = useChartFrame();
  if (!size.width) return null;
  const g = geometryFor(size.width, size.height);
  const crisp = (x: number) => Math.round(x) + 0.5;
  return (
    <svg className={styles.wicks} aria-hidden="true">
      {g.shapes.map((s) => {
        if (!s) return null;
        const { upper, lower } = wicks(s);
        const x = crisp(s.cross);
        const d =
          (upper ? `M${x} ${upper[0]}V${upper[1]}` : "") +
          (lower ? `M${x} ${lower[0]}V${lower[1]}` : "");
        if (!d) return null;
        return (
          <path
            key={s.index}
            d={d}
            className={styles.wick}
            data-dim={active !== null && active !== s.index}
            style={
              {
                "--wick": s.up ? up : down,
                animationDelay: `${0.45 + s.index * pace}s`,
              } as CSSProperties
            }
          />
        );
      })}
    </svg>
  );
}

function AverageLine({
  theme,
  geometryFor,
  model,
  colour,
  replay,
}: {
  theme: Theme;
  geometryFor: (width: number, height: number) => CandleGeometry;
  model: CandleModel;
  colour: string;
  replay: number;
}) {
  const { size, active } = useChartFrame();
  if (!size.width) return null;
  const g = geometryFor(size.width, size.height);
  const xs = model.candles.map((_, i) => g.slots.centre(i));
  // Sessions half out of a springing window leave the line at the plot's edge.
  const path = polylinePath(
    xs,
    model.average.map((v, i) =>
      Number.isFinite(v) && xs[i] >= g.plot.left - 1 && xs[i] <= g.plot.right + 1 ? g.y(v) : NaN,
    ),
  );
  return (
    <LightLineLayer
      theme={theme}
      width={size.width}
      height={size.height}
      left={g.plot.left}
      replay={replay}
      lines={[
        {
          key: "average",
          path,
          colour,
          width: 1.5,
          presence: active === null ? 0.9 : 0.55,
        },
      ]}
    />
  );
}

/** The latest close: a quiet rule from the last candle to the edge, the breathing black hole, and
 * the price in a tag beside the plot. */
function Latest({
  geometryFor,
  model,
  format,
  colour,
  visible,
}: {
  geometryFor: (width: number, height: number) => CandleGeometry;
  model: CandleModel;
  format: ValueFormat;
  colour: string;
  visible: boolean;
}) {
  const { size } = useChartFrame();
  if (!size.width || model.latest === null) return null;
  const g = geometryFor(size.width, size.height);
  const s = g.shapes[model.latest];
  if (!s) return null;
  const y = Math.round(s.close) + 0.5;
  const from = s.cross + g.thickness / 2 + 3;
  const markerX = g.plot.right + 1;
  return (
    <div
      className={styles.latest}
      data-visible={visible}
      style={{ "--latest": colour } as CSSProperties}
      aria-hidden="true"
    >
      {markerX - from > 6 && (
        <span
          className={styles.latestRule}
          style={{ left: from, top: y, width: markerX - from - 6 }}
        />
      )}
      <FocusMarker x={markerX} y={y} colour={colour} variant="latest" />
      <span className={styles.priceTag} style={{ left: markerX + 8, top: y }}>
        <Figure
          value={model.candles[model.latest].close}
          kind={format.kind}
          decimals={format.decimals}
          className={roles.micro}
        />
      </span>
    </div>
  );
}

/* ——— The tooltip: open, high, low, close, volume and the change ——— */

const CandleTooltip = forwardRef(function CandleTooltip(
  {
    index,
    model,
    title,
    format,
    volumeFormat,
    averageName,
    averageColour,
    notes,
    colour,
  }: {
    index: number;
    model: CandleModel;
    title: string;
    format: ValueFormat;
    volumeFormat: ValueFormat;
    averageName: string | null;
    averageColour: string | null;
    notes: readonly string[];
    colour: string;
  },
  ref: ForwardedRef<HTMLDivElement>,
) {
  const c = model.candles[index];
  const change = model.change[index];
  const before = index > 0 ? model.candles[index - 1].close : NaN;
  const prior = Number.isFinite(before) ? before : c.close / (1 + change);
  const fig = (value: number, className: string = roles.figureS, f: ValueFormat = format) => (
    <Figure
      value={value}
      kind={f.kind}
      decimals={f.decimals}
      signed={f.signed}
      unit={f.unit}
      className={className}
    />
  );
  const cells: [string, number][] = [
    ["Open", c.open],
    ["High", c.high],
    ["Low", c.low],
    ["Close", c.close],
  ];
  return (
    <div
      ref={ref}
      className={`${core.tooltip} ${styles.candleTip}`}
      style={{ "--tint": colour } as CSSProperties}
      aria-hidden="true"
    >
      <span className={`${core.tooltipTitle} ${roles.casing}`}>{title}</span>
      <span className={styles.ohlc}>
        {cells.map(([name, value]) => (
          <span key={name} className={styles.ohlcCell} data-close={name === "Close"}>
            <span className={`${roles.micro} ${mutedText}`}>{name}</span>
            {fig(value, name === "Close" ? roles.figureM : roles.figureS)}
          </span>
        ))}
      </span>
      {Number.isFinite(c.volume) && (
        <span className={styles.tipRow}>
          <span className={`${roles.label} ${core.tooltipLabel}`}>Volume</span>
          {fig(c.volume, roles.figureS, volumeFormat)}
        </span>
      )}
      {averageName && Number.isFinite(model.average[index]) && (
        <span className={`${styles.tipRow} ${styles.averageRow}`}>
          <span className={`${roles.label} ${core.tooltipLabel} ${styles.averageName}`}>
            <span
              className={styles.averageSwatch}
              style={{ background: averageColour ?? undefined }}
              aria-hidden="true"
            />
            {averageName}
          </span>
          {fig(model.average[index])}
        </span>
      )}
      {Number.isFinite(change) && (
        <span className={styles.tipFoot}>
          <Delta value={change} better="up">
            {`${change < 0 ? "−" : "+"}${Math.abs(change * 100).toFixed(2)}%`}
          </Delta>
          <span className={`${roles.caption} ${mutedText} ${styles.onTheDay}`}>
            {fig(c.close - prior, "", { ...format, signed: true })} on the day
          </span>
        </span>
      )}
      {notes.map((note) => (
        <span key={note} className={`${styles.tipNote} ${roles.caption}`}>
          {note}
        </span>
      ))}
    </div>
  );
});
