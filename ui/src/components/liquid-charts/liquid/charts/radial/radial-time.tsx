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

import { pixelRatio } from "@/components/liquid-charts/liquid/core/render-scale";
import {
  ChartFrame,
  ChartScope,
  ChartState,
  ChartTooltip,
  cssToRgb,
  Delta,
  easeOut,
  ENTER,
  Figure,
  finite,
  FocusMarker,
  labelPriorities,
  Legend,
  mutedText,
  placeTooltip,
  resolveStyle,
  seriesColour,
  stateOf,
  textWidth,
  tickFormatter,
  TooltipFoot,
  TYPE,
  useActive,
  useChartFrame,
  valueTicks,
  type ChartConfig,
  type Density,
  type LabelMode,
  type LightLine,
  type Rect,
  type Rgb,
  type Theme,
  type TooltipRow,
} from "@/components/liquid-charts/liquid/core";
import {
  clockGeometry,
  momentLabel,
  nearestFraction,
  outwardSides,
  placeRound,
  periodFraction,
  periodTicks,
  polar,
  radialDomain,
  radialSamples,
  radialScale,
  samplesPath,
  TAU,
  type Period,
  type RadialSample,
} from "./layout";
import {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent,
  type ReactNode,
} from "react";
import {
  labelNode,
  labelText,
  plain,
  put,
  stateMessage,
  useMeasured,
  type ValueFormat,
} from "./radial-parts";
import { createLiquidRenderer, type LiquidRenderer } from "@/components/liquid-charts/liquid/pour";
import { useRadialLayer, type LayerClock } from "./use-radial-layer";
import { usePageTheme } from "@/components/liquid-charts/liquid/core/page-theme";
import { cssModule } from "@/components/liquid-charts/liquid/core/css-module";
import Roles from "@/components/liquid-charts/liquid/core/type.module.css";
import { LiquidCredit } from "@/components/liquid-charts/liquid/credit";
import Styles from "./radial.module.css";

const roles = cssModule(Roles, "lc-type-");
const styles = cssModule(Styles, "lc-radial-");

export type RadialRow = Record<string, unknown>;

export type RadialTimeProps = {
  /** One row per moment: its time under `timeKey` (epoch ms, a Date or an ISO string) and each
   * series' value (null for a gap). */
  data: readonly RadialRow[];
  timeKey?: string;
  /** Series keys in `config`, which gives each its name and colour. The first leads. */
  series: readonly string[];
  /** A previous period's key: quiet dashed light under the lead. */
  compare?: string;
  config: ChartConfig;
  /** What one turn is: a year (months round the rim), a day (hours) or a week (days). UTC. */
  period: Period;
  /** Light or dark. Without one, the page's: `data-theme` or a `dark` class on `<html>`, else the system setting. @default the page's */
  theme?: Theme;
  height?: number;
  density?: Density;
  domain?: readonly [number, number];
  /** Read the radius from zero (default for areas) or from the data's low end (lines). */
  includeZero?: boolean;
  /** Quiet dashed rings at a value: a target, an average. */
  references?: readonly { value: number; label?: string }[];
  /** The moment in focus (a row index). */
  active?: number | null;
  onActiveChange?: (index: number | null) => void;
  /** Value labels on the lead: `auto` names its peak and low. */
  labels?: LabelMode;
  tooltip?: boolean;
  format?: ValueFormat;
  /** Delta colour in the tooltip: `down` when a fall is good (cost, latency, energy). */
  better?: "up" | "down";
  /** A moment's name, for the tooltip and the live region. */
  title?: (row: RadialRow, index: number) => string;
  /** What the hole reads at rest: the lead's total, its average, or nothing. `focus` leaves it
   * empty at rest and reads only the moment in focus (use it when the card's headline already
   * gives the total); `none` never reads anything there. */
  centre?: "total" | "average" | "focus" | "none";
  centreLabel?: ReactNode;
  legend?: boolean;
  loading?: boolean;
  empty?: boolean | ReactNode;
  error?: boolean | ReactNode;
  replay?: number;
  label: string;
  describe?: (index: number) => string;
  className?: string;
};

const FALLBACK_TINT: Rgb = [0.62, 0.55, 0.92];
/** Room past the plot for the liquid's bloom, px. */
const BLEED = 24;
/** Samples round the turn for the liquid's surface: one a degree. */
const LIQUID_SAMPLES = 361;

/** A sampled radial curve's value at `fraction` of the turn (interpolated between its samples,
 * round the seam too), or NaN in a gap. */
function runValueAt(runs: readonly RadialSample[][], fraction: number): number {
  for (const run of runs) {
    if (run.length < 2) continue;
    const a = run[0].fraction,
      b = run[run.length - 1].fraction;
    for (const f of [fraction, fraction + 1, fraction - 1]) {
      if (f < a - 1e-9 || f > b + 1e-9) continue;
      const t = Math.max(0, Math.min(run.length - 1, ((f - a) / (b - a || 1)) * (run.length - 1)));
      const i = Math.min(run.length - 2, Math.floor(t));
      return run[i].value + (run[i + 1].value - run[i].value) * (t - i);
    }
  }
  return Number.NaN;
}
const timeOf = (value: unknown) =>
  value instanceof Date
    ? value.getTime()
    : typeof value === "string"
      ? Date.parse(value)
      : typeof value === "number"
        ? value
        : Number.NaN;

/** The first time in the data that reads as one: it places the period the face shows. Read from
 * the rows themselves, so the ticks built from it can stay memoized (the rows are props). */
function referenceTime(data: readonly Record<string, unknown>[], timeKey: string) {
  for (const row of data) {
    const time = timeOf(row[timeKey]);
    if (Number.isFinite(time)) return time;
  }
  return Date.UTC(2026, 0, 1);
}

/**
 * A year or a day wrapped round a clock face: seasonality and daily rhythm as a radial line (or, as
 * `RadialArea`, a body of liquid under the line). The line carries light; a previous period is quiet
 * dashed light beneath it; months or hours are ticked round the rim, values on quiet rings. Scrub
 * round the circle by angle (or with the arrow keys): the light burns brighter up to the pointer,
 * the black hole marks the moment, and the tooltip reads it against the previous period. Lines draw
 * round from their first moment on entrance and spring between data.
 *
 * The curve is a monotone cubic in angle sampled every degree, so it follows the circle rather than
 * cutting chords, and never overshoots a value. A full period joins itself at 12 o'clock.
 */
function RadialTime({
  data,
  timeKey = "time",
  series,
  compare,
  config,
  period,
  theme: themeProp,
  height,
  density = "standard",
  domain: domainProp,
  includeZero,
  references = [],
  active: activeProp,
  onActiveChange,
  labels = "auto",
  tooltip = true,
  format = { kind: "number" },
  better = "up",
  title,
  centre,
  centreLabel,
  legend,
  loading = false,
  empty = false,
  error = false,
  replay = 0,
  label,
  describe,
  className,
  area,
}: RadialTimeProps & { area: boolean }) {
  const theme = usePageTheme(themeProp);
  const tile = density === "tile";
  const plotHeight = height ?? (tile ? 180 : density === "hero" ? 400 : 340);
  const [active, setActive] = useActive(activeProp, onActiveChange);
  const [hidden, setHidden] = useState<ReadonlySet<string>>(() => new Set());
  const [focusKey, setFocusKey] = useState<string | null>(null);
  const uid = useId().replace(/[^a-zA-Z0-9_-]/g, "");
  const visible = series.filter((key) => !hidden.has(key));
  const lead = visible[0] ?? series[0];
  const times = data.map((row) => timeOf(row[timeKey]));
  const fractions = times.map((t) => periodFraction(t, period));
  const valueOf = (key: string, i: number) => {
    const v = data[i]?.[key];
    return finite(v) ? v : null;
  };
  const allValues = [...series, ...(compare ? [compare] : [])].flatMap((key) =>
    data.map((_, i) => valueOf(key, i)),
  );
  const nothing = !loading && !error && allValues.every((v) => v === null);
  const status = stateOf({ loading, error, empty: empty || nothing });
  const domain =
    domainProp ??
    radialDomain([...allValues, ...references.map((r) => r.value)], {
      includeZero: includeZero ?? area,
      count: 4,
    });
  // Rings at round values; a reference ring names itself, so no figure repeats it.
  const rings = valueTicks([domain[0], domain[1]], tile ? 2 : 3).filter(
    (t) => t > domain[0] + 1e-9 && t <= domain[1],
  );
  const ringFigures = rings.filter((t) => !references.some((r) => Math.abs(r.value - t) < 1e-9));
  // Where the curves cross 12 o'clock (between the last moment and the first): a ring figure there
  // would sit under a line, so it steps aside.
  const crossing = (() => {
    const ends = [...visible, ...(compare ? [compare] : [])]
      .flatMap((key) => [valueOf(key, 0), valueOf(key, data.length - 1)])
      .filter((v): v is number => v !== null);
    return ends.length ? ([Math.min(...ends), Math.max(...ends)] as const) : null;
  })();
  // Only the outermost figure carries the unit; the rest are plain numbers.
  const plainTick = tickFormatter({ ...format, unit: undefined }, [domain[0], domain[1]]);
  const unitTick = tickFormatter(format, [domain[0], domain[1]]);
  const top = Math.max(...rings, -Infinity);
  const formatRing = (value: number) => (value === top ? unitTick(value) : plainTick(value));
  const reference = referenceTime(data, timeKey);
  const ticks = useMemo(() => periodTicks(reference, period), [reference, period]);
  const colours = Object.fromEntries(
    [...series, ...(compare ? [compare] : [])].map((key) => [
      key,
      seriesColour(config, key, theme),
    ]),
  );
  const tints = Object.fromEntries(
    Object.entries(colours).map(([key, colour]) => [key, cssToRgb(colour) ?? FALLBACK_TINT]),
  );
  const compareColour = compare && config[compare]?.colors ? colours[compare] : colours[lead];
  const compareTint = cssToRgb(compareColour) ?? FALLBACK_TINT;
  const style = resolveStyle({
    geometry: "circular",
    marks: area ? ["area", "stroke", "reference"] : ["stroke", "reference"],
    axis: "time",
    measure: area ? "amount" : "rate",
    zeroBaseline: domain[0] === 0,
    series: area ? 1 : visible.length,
    density,
  });
  useEffect(() => {
    if (process.env.NODE_ENV === "production") return;
    for (const warning of style.warnings)
      console.warn(`[${area ? "RadialArea" : "RadialLine"}] ${warning.message}`);
    // Warn once per distinct set of warnings.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [style.warnings.map((w) => w.rule).join()]);
  // Filling to the floor says "this much stuff": only an amount from zero is drawn as a body.
  const filled = area && style.materials.area === "liquid";

  const geometryFor = (width: number, h: number) =>
    clockGeometry(width, h, {
      rim: tile ? 18 : 32,
      hole: 0.3,
      max: density === "hero" ? 200 : 180,
    });
  const defined = (key: string) => data.map((_, i) => valueOf(key, i) !== null);
  const closed = (() => {
    const known = fractions.filter(Number.isFinite).sort((a, b) => a - b);
    if (known.length < 3) return false;
    const step = (known[known.length - 1] - known[0]) / (known.length - 1);
    return 1 - known[known.length - 1] + known[0] <= step * 1.5 + 1e-9;
  })();

  const tip = useRef<HTMLDivElement>(null);
  const marker = useRef<HTMLSpanElement>(null);
  const centreRef = useRef<HTMLDivElement>(null);
  const bodyRef = useRef<SVGPathElement>(null);
  const liquidCanvas = useRef<HTMLCanvasElement>(null);
  const liquid = useRef<{
    renderer: LiquidRenderer;
    width: number;
    height: number;
    offset: { x: number; y: number };
  } | null>(null);
  // Without WebGL the body falls back to a flat translucent fill.
  const [poured, setPoured] = useState(true);
  const measured = useMeasured();
  const focusMoment = active !== null && active < data.length ? active : null;
  const focusSeries = focusKey && visible.includes(focusKey) ? focusKey : lead;

  const { box, canvas, lights } = useRadialLayer({
    theme,
    replay,
    material: "flat",
    frame: (clock) => drawFrame(clock),
  });

  // The liquid body's canvas, fed from the page's shared WebGL context.
  const pouring = filled && !status;
  useEffect(() => {
    const target = liquidCanvas.current;
    if (!pouring || !target) return;
    const renderer = createLiquidRenderer(target);
    if (!renderer) {
      setPoured(false);
      return;
    }
    liquid.current = { renderer, width: 0, height: 0, offset: { x: BLEED, y: BLEED } };
    return () => {
      renderer.destroy();
      liquid.current = null;
    };
  }, [pouring]);

  /** Pours the lead's body round the circle: from the hole (the floor, zero) out to its line,
   * entering round from the first moment as the line draws. */
  function pourBody(
    clock: LayerClock,
    g: { cx: number; cy: number; outer: number; inner: number },
    radius: (value: number) => number,
    runs: readonly RadialSample[][],
    start: number,
    reveal: number,
  ) {
    const body = liquid.current,
      target = liquidCanvas.current;
    if (!body || !target) return;
    const { width, height: h, motion } = clock;
    if (body.width !== width || body.height !== h) {
      // The canvas reaches past the plot for the bloom and sits on a whole device pixel.
      const r = box.current?.getBoundingClientRect();
      const ratio = pixelRatio();
      const fraction = (value: number) => ((((value * ratio) % 1) + 1) % 1) / ratio;
      body.offset = { x: BLEED + (r ? fraction(r.left) : 0), y: BLEED + (r ? fraction(r.top) : 0) };
      body.width = width;
      body.height = h;
      target.style.left = `${-body.offset.x}px`;
      target.style.top = `${-body.offset.y}px`;
      body.renderer.resize(
        Math.ceil(width + body.offset.x + BLEED),
        Math.ceil(h + body.offset.y + BLEED),
      );
    }
    // Unrolled: the run is the turn from the first moment (about px along the middle radius),
    // the height the radius from the hole.
    const depth = g.outer - g.inner;
    const turn = TAU * ((g.inner + g.outer) / 2);
    const step = turn / (LIQUID_SAMPLES - 1);
    const surface = new Float32Array(LIQUID_SAMPLES);
    for (let j = 0; j < LIQUID_SAMPLES; j++) {
      const v = runValueAt(runs, start + j / (LIQUID_SAMPLES - 1));
      surface[j] = Number.isFinite(v) ? depth - (radius(v) - g.inner) : Number.NaN;
    }
    const along = (f: number) => ((((f - start) % 1) + 1) % 1) * turn;
    const lit = focusMoment !== null && focusSeries === lead && valueOf(lead, focusMoment) !== null;
    const focus = motion.ease("liquid:focus", lit ? 1 : 0);
    target.style.opacity = String(
      motion.ease("liquid:tone", focusKey === null || focusKey === lead ? 1 : 0.4),
    );
    const revealed = reveal >= 1;
    body.renderer.draw({
      theme,
      plot: { left: 0, top: 0, right: turn, bottom: depth },
      surfaces: [surface],
      x0: 0,
      step,
      colours: [[tints[lead] ?? FALLBACK_TINT]],
      tip: revealed ? turn + 1 : reveal * (closed ? 1 : 1 - start) * turn,
      nose: revealed ? 0 : Math.min(64, turn * 0.08),
      focusX: lit ? along(fractions[focusMoment!]) : 0,
      focus,
      project: null,
      polar: {
        cx: g.cx + body.offset.x,
        cy: g.cy + body.offset.y,
        inner: g.inner,
        outer: g.outer,
        from: start * TAU,
        span: TAU,
      },
    });
  }

  function drawFrame(clock: LayerClock) {
    if (status) return { shapes: [], lines: [] };
    const { width, height: h, motion, time } = clock;
    const g = geometryFor(width, h);
    const radius = radialScale(domain, g.inner, g.outer);
    // One sweep round the dial, on the shared beat: a stagger's run plus a mark.
    const reveal = easeOut((time - ENTER.delay) / (ENTER.run + ENTER.mark));
    const point = (s: RadialSample) => polar(g.cx, g.cy, radius(s.value), s.fraction * TAU);
    const runsFor = (key: string) => {
      const values = data.map((_, i) => {
        const v = valueOf(key, i);
        return v === null || hidden.has(key)
          ? null
          : motion.spring(`v:${key}:${i}`, v, {
              rest: Math.abs(domain[1] - domain[0]) * 1e-4 || 1e-4,
            });
      });
      return radialSamples(fractions, values, { closed });
    };
    const start = Math.min(...fractions.filter(Number.isFinite), 1);
    const until = start + reveal * (closed ? 1 : 1 - start + 1e-9);
    const lines: LightLine[] = [];
    if (compare) {
      const runs = runsFor(compare);
      const path = samplesPath(runs, point, { until, close: closed });
      if (path)
        lines.push({
          key: `~${compare}`,
          path,
          colour: compareTint,
          width: 1.6,
          state: "dim",
          forecastFrom: -1,
        });
    }
    // The scrub: the lead burns brighter up to the moment in focus.
    const scrub = focusMoment !== null ? fractions[focusMoment] : null;
    const leadRuns = new Map<string, RadialSample[][]>();
    for (const key of visible) {
      const runs = runsFor(key);
      leadRuns.set(key, runs);
      const tone = motion.ease(`p:${key}`, focusKey === null || focusKey === key ? 1 : 0.32);
      const width = key === lead ? 2.4 : 2;
      if (scrub !== null && key === focusSeries && Number.isFinite(scrub)) {
        const lit = samplesPath(runs, point, {
          until: Math.min(until, scrub < start ? scrub + 1 : scrub),
        });
        const rest = samplesPath(
          runs.map((run) =>
            run.filter((s) => s.fraction >= (scrub < start ? scrub + 1 : scrub) - 1e-9),
          ),
          point,
          { until },
        );
        if (lit)
          lines.push({
            key: `${key}:lit`,
            path: lit,
            colour: tints[key],
            width,
            presence: 1.12 * tone,
            tip: undefined,
          });
        if (rest) lines.push({ key, path: rest, colour: tints[key], width, presence: 0.62 * tone });
      } else {
        const path = samplesPath(runs, point, { until, close: closed });
        if (path) lines.push({ key, path, colour: tints[key], width, presence: tone });
      }
    }

    // The body under the lead: liquid poured round the circle from the hole (or, without WebGL, a
    // flat translucent fill lit under its edge).
    if (filled && liquid.current)
      pourBody(clock, g, radius, leadRuns.get(lead) ?? [], start, reveal);
    const body = bodyRef.current;
    if (body) {
      const runs = leadRuns.get(lead) ?? [];
      let d = "";
      if (filled)
        for (const run of runs) {
          const kept = run.filter((s) => s.fraction <= until + 1e-9);
          if (kept.length < 2) continue;
          const outer = kept.map((s) => point(s));
          const inner = kept
            .map((s) => polar(g.cx, g.cy, radius(domain[0] < 0 ? 0 : domain[0]), s.fraction * TAU))
            .reverse();
          d += `M${outer.map((p) => `${p.x} ${p.y}`).join("L")}L${inner.map((p) => `${p.x} ${p.y}`).join("L")}Z`;
        }
      if (body.getAttribute("d") !== d) body.setAttribute("d", d);
      const tone = focusKey === null || focusKey === lead ? 1 : 0.4;
      body.style.opacity = String(tone);
    }
    place(g, width, h, radius, reveal, until);
    return { shapes: [], lines, moving: reveal < 1 };
  }

  function place(
    g: { cx: number; cy: number; outer: number; inner: number },
    width: number,
    h: number,
    radius: (value: number) => number,
    reveal: number,
    until: number,
  ) {
    const drawnTo = (i: number) => {
      const f = fractions[i];
      return Number.isFinite(f) && (f <= until + 1e-9 || f + 1 <= until + 1e-9 || reveal >= 1);
    };
    // The centre reading follows the focus.
    if (centreRef.current) {
      centreRef.current.style.left = `${g.cx}px`;
      centreRef.current.style.top = `${g.cy}px`;
      centreRef.current.dataset.placed = "";
    }
    // The black hole on the focused moment.
    const element = marker.current;
    let anchor: Rect | null = null;
    if (element) {
      const v = focusMoment !== null ? valueOf(focusSeries, focusMoment) : null;
      if (focusMoment !== null && v !== null && drawnTo(focusMoment)) {
        const at = polar(g.cx, g.cy, radius(v), fractions[focusMoment] * TAU);
        element.style.transform = `translate(${at.x}px, ${at.y}px)`;
        element.style.setProperty("--marker", colours[focusSeries]);
        element.dataset.visible = "true";
        anchor = { left: at.x - 8, top: at.y - 8, right: at.x + 8, bottom: at.y + 8 };
      } else element.dataset.visible = "false";
    }
    let tipRect: Rect | null = null;
    const card = tip.current;
    if (card && focusMoment !== null && tooltip) {
      placeTooltip(
        card,
        anchor ?? { left: g.cx, top: g.cy, right: g.cx, bottom: g.cy },
        { left: 4, top: 4, right: width - 4, bottom: h - 4 },
        {
          prefer: "outward",
          ring: { cx: g.cx, cy: g.cy, outer: g.outer + 6, angle: fractions[focusMoment] * TAU },
        },
      );
      const placed = /translate\(([-\d.]+)px, ([-\d.]+)px\)/.exec(card.style.transform);
      if (placed) {
        const left = Number(placed[1]),
          top = Number(placed[2]);
        tipRect = { left, top, right: left + card.offsetWidth, bottom: top + card.offsetHeight };
      }
    }
    // The rings' figures up from 12 o'clock are obstacles for the value labels.
    const figureBoxes: Rect[] = tile
      ? []
      : ringFigures.map((value) => {
          const y = g.cy - radius(value);
          const w = textWidth(formatRing(value), TYPE.micro);
          return { left: g.cx + 3, top: y - 8, right: g.cx + 7 + w, bottom: y + 8 };
        });
    // Value labels on the lead: its peak and low, outward from the curve. The moment in focus is
    // read by the tooltip and the centre, so it is not labelled again.
    const values = data.map((_, i) => valueOf(lead, i));
    const priorities = labelPriorities(
      values.map((v) => (v === null ? Number.NaN : v)),
      tile ? "none" : labels === "auto" ? "auto" : labels,
      tooltip ? null : focusSeries === lead ? focusMoment : null,
    );
    // `auto` names the peak and the low only; the latest of a cycle is not special.
    if (labels === "auto") {
      const last = values.length - 1;
      if (priorities[last] === 1) priorities[last] = 0;
    }
    const requests = data.map((_, i) => {
      const v = values[i];
      const angle = (fractions[i] ?? 0) * TAU;
      const at = v === null ? { x: g.cx, y: g.cy } : polar(g.cx, g.cy, radius(v), angle);
      // A label arrives once the sweep has passed its moment, not after the whole dial is drawn
      // (and the moment at the sweep's start waits for the line to leave it).
      const f = fractions[i];
      const passed =
        reveal >= 1 || (Number.isFinite(f) && (f < until - 0.02 || f + 1 < until - 0.02));
      const show = v !== null && passed;
      const size = measured.size(`v${i}`);
      return {
        id: `v${i}`,
        x: at.x,
        y: at.y,
        width: size.width,
        height: size.height,
        side: outwardSides(angle)[0],
        sides: outwardSides(angle),
        offset: 9,
        priority: show ? priorities[i] : 0,
      };
    });
    placeRound(requests, {
      bounds: { left: 2, top: 2, right: width - 2, bottom: h - 2 },
      obstacles: tipRect ? [...figureBoxes, tipRect] : figureBoxes,
    }).forEach((spot, i) => put(measured.element(`v${i}`), spot.left, spot.top, spot.visible));
  }

  const pickAt = (x: number, y: number, size: { width: number; height: number }) => {
    if (status) return null;
    const g = geometryFor(size.width, size.height);
    const r = Math.hypot(x - g.cx, y - g.cy);
    if (r > g.outer + 40 || r < g.inner * 0.4) return null;
    const angle = ((Math.atan2(x - g.cx, g.cy - y) % TAU) + TAU) % TAU;
    const index = nearestFraction(
      fractions,
      angle / TAU,
      defined(lead).map((d, i) => d || visible.some((k) => valueOf(k, i) !== null)),
    );
    if (index !== null && visible.length > 1) {
      const radius = radialScale(domain, g.inner, g.outer);
      let best: string | null = null,
        distance = 24;
      for (const key of visible) {
        const v = valueOf(key, index);
        if (v === null) continue;
        const d = Math.abs(radius(v) - r);
        if (d < distance) {
          distance = d;
          best = key;
        }
      }
      if (best !== focusKey) setFocusKey(best);
    }
    return index;
  };

  const onKeyDownCapture = (event: KeyboardEvent<HTMLDivElement>) => {
    if (visible.length < 2 || focusMoment === null) return;
    if (event.key !== "ArrowUp" && event.key !== "ArrowDown") return;
    event.preventDefault();
    event.stopPropagation();
    const order = [...visible].sort(
      (a, b) => (valueOf(b, focusMoment) ?? -Infinity) - (valueOf(a, focusMoment) ?? -Infinity),
    );
    const from = focusKey ? order.indexOf(focusKey) : -1;
    const next =
      event.key === "ArrowUp"
        ? order[from <= 0 ? (from < 0 ? 0 : order.length - 1) : from - 1]
        : order[from < 0 || from >= order.length - 1 ? 0 : from + 1];
    setFocusKey(next);
  };

  const momentName = (i: number) =>
    title
      ? title(data[i], i)
      : Number.isFinite(times[i])
        ? momentLabel(times[i], period)
        : `Moment ${i + 1}`;
  const name = (key: string) => labelText(config, key);
  const compareName = compare ? labelText(config, compare, "Previous") : "";
  const speak =
    describe ??
    ((i: number) => {
      const parts = visible.map((key) => `${name(key)} ${plain(valueOf(key, i), format)}`);
      if (compare) parts.push(`${compareName} ${plain(valueOf(compare, i), format)}`);
      return `${momentName(i)}: ${parts.join(", ")}.`;
    });
  const leadValues = data.map((_, i) => valueOf(lead, i)).filter((v): v is number => v !== null);
  const total = leadValues.reduce((a, b) => a + b, 0);
  const centreMode = centre ?? (area ? "total" : "average");
  const focusValue = focusMoment !== null ? valueOf(focusSeries, focusMoment) : null;
  const previous = focusMoment !== null && compare ? valueOf(compare, focusMoment) : null;
  const showLegend = (legend ?? (visible.length > 1 || Boolean(compare))) && !status && !tile;

  const tooltipRows: TooltipRow[] = [];
  if (focusMoment !== null) {
    const order = [...visible].sort((a, b) =>
      a === focusSeries
        ? -1
        : b === focusSeries
          ? 1
          : (valueOf(b, focusMoment) ?? -Infinity) - (valueOf(a, focusMoment) ?? -Infinity),
    );
    for (const key of order) {
      const v = valueOf(key, focusMoment);
      tooltipRows.push({
        label: labelNode(config, key),
        color: colours[key],
        value:
          v === null ? (
            <span className={`${roles.caption} ${mutedText}`}>no data</span>
          ) : (
            <Figure value={v} {...format} className={roles.figureS} />
          ),
      });
    }
    if (compare)
      tooltipRows.push({
        label: labelNode(config, compare, "Previous"),
        color: compareColour,
        muted: true,
        value:
          previous === null ? (
            <span className={`${roles.caption} ${mutedText}`}>no data</span>
          ) : (
            <Figure value={previous} {...format} className={`${roles.figureS} ${mutedText}`} />
          ),
      });
  }
  const change =
    focusValue !== null && previous !== null && previous !== 0 ? focusValue / previous - 1 : null;
  const gradient = `${uid}-body`;

  return (
    <ChartScope theme={theme} className={`${styles.chart} ${className ?? ""}`}>
      <div onKeyDownCapture={onKeyDownCapture}>
        <ChartFrame
          credit={<LiquidCredit />}
          theme={theme}
          height={plotHeight}
          insets={{ left: 0, right: 0, top: 0, bottom: 0 }}
          count={status ? 0 : data.length}
          active={status ? null : focusMoment}
          onActiveChange={(index) => {
            setActive(index);
            if (index === null) setFocusKey(null);
          }}
          plotRef={box}
          pick={pickAt}
          label={`${label}. Use the arrow keys to move round the ${period === "year" ? "year" : period === "day" ? "day" : "week"}${visible.length > 1 ? ", and Up and Down to move between series" : ""}.`}
          describe={speak}
          table={{
            caption: label,
            columns: ["Moment", ...series.map(name), ...(compare ? [compareName] : [])],
            rows: data.map((_, i) => [
              momentName(i),
              ...series.map((key) => plain(valueOf(key, i), format)),
              ...(compare ? [plain(valueOf(compare, i), format)] : []),
            ]),
          }}
        >
          {!status && (
            <Face
              geometryFor={geometryFor}
              ticks={ticks}
              rings={rings}
              references={references}
              domain={domain}
              active={focusMoment === null ? null : fractions[focusMoment]}
              tile={tile}
            />
          )}
          {!status && filled && poured && (
            <canvas ref={liquidCanvas} className={styles.canvas} aria-hidden="true" />
          )}
          {!status && filled && !poured && (
            <svg className={styles.layer} aria-hidden="true">
              <defs>
                <BodyGradient
                  id={gradient}
                  geometryFor={geometryFor}
                  colour={colours[lead]}
                  theme={theme}
                />
              </defs>
              <path
                ref={bodyRef}
                className={styles.body}
                fill={`url(#${gradient})`}
                fillRule="evenodd"
              />
            </svg>
          )}
          <canvas ref={canvas} className={styles.canvas} aria-hidden="true" />
          <canvas ref={lights} className={styles.lights} aria-hidden="true" />
          {!status && !tile && (
            <RingFigures
              geometryFor={geometryFor}
              domain={domain}
              rings={ringFigures}
              crossing={crossing}
              format={formatRing}
            />
          )}
          {status?.kind === "loading" && (
            <ChartState kind="loading" shape="ring" ring={{ inner: 0.3 }} />
          )}
          {status && status.kind !== "loading" && (
            <ChartState
              kind={status.kind}
              message={status.kind === "error" ? stateMessage(error) : stateMessage(empty)}
            />
          )}
          {!status && centreMode !== "none" && !tile && (
            <div ref={centreRef} className={styles.centre} aria-hidden="true">
              {(focusMoment !== null || centreMode !== "focus") && (
                <>
                  <span className={`${styles.centreCaption} ${roles.caption}`}>
                    {focusMoment !== null
                      ? momentName(focusMoment)
                      : (centreLabel ?? (centreMode === "total" ? "Total" : "Average"))}
                  </span>
                  <Figure
                    value={
                      focusMoment !== null
                        ? (focusValue ?? 0)
                        : centreMode === "total"
                          ? total
                          : total / Math.max(1, leadValues.length)
                    }
                    {...format}
                    className={`${roles.figureM} ${styles.centreValue}`}
                  />
                </>
              )}
            </div>
          )}
          {!status &&
            data.map((_, i) => {
              const v = valueOf(lead, i);
              if (v === null) return null;
              return (
                <div
                  key={`v${i}`}
                  ref={measured.ref(`v${i}`)}
                  className={`${styles.placed} ${styles.value}`}
                  data-shown="false"
                  aria-hidden="true"
                >
                  <Figure value={v} {...format} className={roles.figureS} />
                </div>
              );
            })}
          {!status && (
            <FocusMarker ref={marker} colour={colours[focusSeries] ?? "#fff"} visible={false} />
          )}
          {!status && tooltip && focusMoment !== null && (
            <ChartTooltip
              ref={tip}
              title={momentName(focusMoment)}
              rows={tooltipRows}
              footer={
                change !== null ? (
                  <TooltipFoot>
                    <Delta value={change} better={better}>
                      {`${change < 0 ? "−" : "+"}${Math.abs(change * 100).toFixed(1)}% vs ${compareName.toLowerCase()}`}
                    </Delta>
                  </TooltipFoot>
                ) : undefined
              }
            />
          )}
        </ChartFrame>
      </div>
      {showLegend && (
        <Legend
          items={[
            ...series.map((key) => ({ key, label: labelNode(config, key), color: colours[key] })),
            ...(compare
              ? [
                  {
                    key: `~${compare}`,
                    label: labelNode(config, compare, "Previous"),
                    color: compareColour,
                    muted: true,
                  },
                ]
              : []),
          ]}
          hidden={hidden}
          onToggle={
            series.length > 1
              ? (key) => {
                  if (key.startsWith("~")) return;
                  setHidden((was) => {
                    const next = new Set(was);
                    if (next.has(key)) next.delete(key);
                    else if (series.length - next.size > 1) next.add(key);
                    return next;
                  });
                }
              : undefined
          }
        />
      )}
    </ChartScope>
  );
}

/** The flat body's light (without WebGL): brightest just under the edge, fading towards the
 * floor, in the series' colour. */
function BodyGradient({
  id,
  geometryFor,
  colour,
  theme,
}: {
  id: string;
  geometryFor: (
    width: number,
    height: number,
  ) => { cx: number; cy: number; outer: number; inner: number };
  colour: string;
  theme: Theme;
}) {
  const { size } = useChartFrame();
  const g = geometryFor(size.width || 1, size.height || 1);
  const from = g.inner / Math.max(1, g.outer);
  const light = theme === "light";
  return (
    <radialGradient id={id} gradientUnits="userSpaceOnUse" cx={g.cx} cy={g.cy} r={g.outer}>
      <stop offset={from} stopColor={colour} stopOpacity={light ? 0.05 : 0.04} />
      <stop
        offset={from + (1 - from) * 0.55}
        stopColor={colour}
        stopOpacity={light ? 0.14 : 0.14}
      />
      <stop offset={1} stopColor={colour} stopOpacity={light ? 0.3 : 0.34} />
    </radialGradient>
  );
}

/** The clock face: quiet value rings, ticks round the rim with their names, reference rings. */
function Face({
  geometryFor,
  ticks,
  rings,
  references,
  domain,
  active,
  tile,
}: {
  geometryFor: (
    width: number,
    height: number,
  ) => { cx: number; cy: number; outer: number; inner: number };
  ticks: readonly { value: number; label: string; angle: number; labelAngle: number }[];
  rings: readonly number[];
  references: readonly { value: number; label?: string }[];
  domain: readonly [number, number];
  active: number | null;
  tile: boolean;
}) {
  const { size } = useChartFrame();
  if (!size.width) return null;
  const g = geometryFor(size.width, size.height);
  const radius = radialScale(domain, g.inner, g.outer);
  // The tick whose span holds the focused moment brightens.
  const current =
    active === null
      ? -1
      : ticks.reduce((best, tick, i) => (tick.angle / TAU <= active + 1e-9 ? i : best), 0);
  return (
    <>
      <svg className={styles.layer} aria-hidden="true">
        <circle className={styles.axis} cx={g.cx} cy={g.cy} r={g.inner} />
        {rings.map((value) => (
          <circle
            key={value}
            className={styles.axis}
            data-dashed={Math.abs(value - domain[1]) > 1e-9}
            data-strong={Math.abs(value - domain[1]) < 1e-9}
            cx={g.cx}
            cy={g.cy}
            r={radius(value)}
          />
        ))}
        {references.map((ref) => (
          <circle
            key={ref.value}
            className={styles.reference}
            cx={g.cx}
            cy={g.cy}
            r={radius(ref.value)}
          />
        ))}
        {ticks.map((tick) => {
          const a = polar(g.cx, g.cy, g.inner, tick.angle),
            b = polar(g.cx, g.cy, g.outer + 5, tick.angle);
          return (
            <line
              key={tick.value}
              className={styles.axis}
              data-dashed="true"
              x1={a.x}
              y1={a.y}
              x2={b.x}
              y2={b.y}
            />
          );
        })}
      </svg>
      {ticks.map((tick, i) => {
        const at = polar(g.cx, g.cy, g.outer + (tile ? 10 : 17), tick.labelAngle);
        return (
          <span
            key={tick.value}
            className={`${styles.tick} ${roles.micro}`}
            data-active={current === i}
            style={{
              position: "absolute",
              left: at.x,
              top: at.y,
              transform: "translate(-50%, -50%)",
              pointerEvents: "none",
              whiteSpace: "nowrap",
            }}
            aria-hidden="true"
          >
            {tick.label}
          </span>
        );
      })}
      {references.map((ref) =>
        ref.label ? (
          <span
            key={`r${ref.value}`}
            className={`${styles.ringLabel} ${roles.micro}`}
            style={
              {
                position: "absolute",
                left: g.cx - 5,
                top: g.cy - radius(ref.value),
                transform: "translate(-100%, -50%)",
                pointerEvents: "none",
                whiteSpace: "nowrap",
              } as CSSProperties
            }
            aria-hidden="true"
          >
            {ref.label}
          </span>
        ) : null,
      )}
    </>
  );
}

/** The rings' values up from 12 o'clock, over the data, with a halo in the card's colour. A figure
 * within reach of where the curves cross 12 o'clock (`crossing`, a value span) is left out, so no
 * scale number sits under a line. */
function RingFigures({
  geometryFor,
  domain,
  rings,
  crossing,
  format,
}: {
  geometryFor: (
    width: number,
    height: number,
  ) => { cx: number; cy: number; outer: number; inner: number };
  domain: readonly [number, number];
  rings: readonly number[];
  crossing: readonly [number, number] | null;
  format: (value: number) => string;
}) {
  const { size } = useChartFrame();
  if (!size.width) return null;
  const g = geometryFor(size.width, size.height);
  const radius = radialScale(domain, g.inner, g.outer);
  const clear = (value: number) =>
    !crossing || radius(value) < radius(crossing[0]) - 9 || radius(value) > radius(crossing[1]) + 9;
  return (
    <>
      {rings.filter(clear).map((value) => (
        <span
          key={value}
          className={`${styles.ringLabel} ${roles.micro}`}
          style={{
            position: "absolute",
            left: g.cx + 5,
            top: g.cy - radius(value),
            transform: "translate(0, -50%)",
            pointerEvents: "none",
            whiteSpace: "nowrap",
          }}
          aria-hidden="true"
        >
          {format(value)}
        </span>
      ))}
    </>
  );
}

/** Seasonality and rhythm as a line of light round a clock face: a year (months round the rim), a
 * day (hours) or a week. For rates, prices, temperatures and averages, read from the data's low end. */
export function RadialLine(props: RadialTimeProps) {
  return <RadialTime {...props} area={false} />;
}

/**
 * An amount round a clock face: liquid poured from zero (the hole) out to a line of light, bent
 * round the circle by the core's polar liquid, pouring round from the first moment on entrance.
 * Amounts only, from zero; a rate or a non-zero floor draws a line. Without WebGL the body is a
 * flat translucent fill.
 */
export function RadialArea(props: RadialTimeProps) {
  return <RadialTime {...props} area />;
}
