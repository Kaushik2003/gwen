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
  Figure,
  finite,
  FocusMarker,
  Legend,
  LIFT,
  mutedText,
  overlaps,
  placeTooltip,
  seriesColour,
  stateOf,
  textWidth,
  tickFormatter,
  TYPE,
  useActive,
  useChartFrame,
  valueTicks,
  type ChartConfig,
  type Density,
  type Rect,
  type Rgb,
  type Theme,
  type TooltipRow,
} from "@/components/liquid-charts/liquid/core";
import {
  axisAnchor,
  placeAxisLabels,
  polar,
  radarGeometry,
  radarPath,
  radarRadii,
  radialDomain,
  readingOrder,
  seriesAt,
  spokeAngles,
  spokeAt,
  type Size,
} from "./layout";
import {
  labelNode,
  labelText,
  plain,
  put,
  stateMessage,
  useMeasured,
  type ValueFormat,
} from "./radial-parts";
import { useEffect, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { useRadialLayer, type LayerClock } from "./use-radial-layer";
import { drawRadarFrame, resolveRadarStyle } from "./radar-frame";
import { usePageTheme } from "@/components/liquid-charts/liquid/core/page-theme";
import { cssModule } from "@/components/liquid-charts/liquid/core/css-module";
import Roles from "@/components/liquid-charts/liquid/core/type.module.css";
import { LiquidCredit } from "@/components/liquid-charts/liquid/credit";
import Styles from "./radial.module.css";

const roles = cssModule(Roles, "lc-type-");
const styles = cssModule(Styles, "lc-radial-");

export type RadarRow = Record<string, string | number | null | undefined>;

export type RadarProps = {
  /** One row per axis (a criterion): its name under `axisKey`, and each series' value. */
  data: readonly RadarRow[];
  axisKey: string;
  /** Series keys in `config`, which gives each its name and colour. */
  series: readonly string[];
  config: ChartConfig;
  /** Light or dark. Without one, the page's: `data-theme` or a `dark` class on `<html>`, else the system setting. @default the page's */
  theme?: Theme;
  height?: number;
  density?: Density;
  /** The scale from the centre out. Defaults to zero to a round number above the data. */
  domain?: readonly [number, number];
  /** Rings as polygons through the spokes (a web) or as circles. */
  grid?: "polygon" | "circle";
  /** About this many rings, at round values. */
  rings?: number;
  /** The spoke in focus (an axis index). */
  active?: number | null;
  onActiveChange?: (index: number | null) => void;
  tooltip?: boolean;
  format?: ValueFormat;
  /** One or two series are filled, more are light outlines; `false` draws outlines always. */
  fill?: boolean;
  /** A legend under the chart whose entries switch series off (default with two or more). */
  legend?: boolean;
  /** Fills are flat and translucent under a light edge by default: a radar's polygons are large,
   * and glass that size goes dark in the middle. `"glass"` opts in to glass fills. */
  material?: "glass" | "flat";
  loading?: boolean;
  empty?: boolean | ReactNode;
  error?: boolean | ReactNode;
  replay?: number;
  label: string;
  describe?: (index: number) => string;
  className?: string;
};

const FALLBACK_TINT: Rgb = [0.62, 0.55, 0.92];
const LABEL_HEIGHT = 18;

/**
 * A radar (spider) chart: one spoke per criterion, each series a closed outline through its values.
 * One or two series are glass fills, the larger underneath, so a profile reads as one solid shape;
 * three or more are light outlines, so they stay legible where they cross. Spokes and rings are
 * quiet flat axis lines, the rings carry their values, and the criteria are named round the
 * outside without colliding. Hover a spoke (or step round with the arrow keys) to read every series
 * at that criterion, focused first; Up and Down move between series, and the black hole marks the
 * focused point. Outlines bloom from the centre on entrance and spring between data; glass fills
 * (`material="glass"`) keep their outline and fill with liquid from the centre out instead.
 *
 * Missing values are left out of an outline (it runs straight between its neighbours) and read
 * "no data". Without WebGL, or with `material="flat"`, fills are flat and translucent under a light
 * edge.
 */
export function Radar({
  data,
  axisKey,
  series,
  config,
  theme: themeProp,
  height,
  density = "standard",
  domain: domainProp,
  grid = "polygon",
  rings = 4,
  active: activeProp,
  onActiveChange,
  tooltip = true,
  format = { kind: "number" },
  fill = true,
  legend,
  material = "flat",
  loading = false,
  empty = false,
  error = false,
  replay = 0,
  label,
  describe,
  className,
}: RadarProps) {
  const theme = usePageTheme(themeProp);
  const tile = density === "tile";
  const plotHeight = height ?? (tile ? 180 : density === "hero" ? 380 : 320);
  const [hidden, setHidden] = useState<ReadonlySet<string>>(() => new Set());
  const [active, setActive] = useActive(activeProp, onActiveChange);
  const [focusSeries, setFocusSeries] = useState<string | null>(null);
  const visible = series.filter((key) => !hidden.has(key));
  const axes = data.map((row, i) => String(row[axisKey] ?? `Axis ${i + 1}`));
  const valueOf = (key: string, i: number) => {
    const v = data[i]?.[key];
    return finite(v) ? v : null;
  };
  const all = series.flatMap((key) => data.map((_, i) => valueOf(key, i)));
  const nothing = !loading && !error && (data.length < 3 || all.every((v) => v === null));
  const status = stateOf({ loading, error, empty: empty || nothing });
  const domain = domainProp ?? radialDomain(all, { count: rings });
  const ringValues = valueTicks([domain[0], domain[1]], rings).filter(
    (t) => t > domain[0] && t <= domain[1],
  );
  const formatRing = tickFormatter(format, [domain[0], domain[1]]);
  const colours = Object.fromEntries(series.map((key) => [key, seriesColour(config, key, theme)]));
  const tints = Object.fromEntries(
    series.map((key) => [key, cssToRgb(colours[key]) ?? FALLBACK_TINT]),
  );
  const style = resolveRadarStyle({
    fill,
    series: visible.length,
    density,
    material,
  });
  useEffect(() => {
    if (process.env.NODE_ENV === "production") return;
    for (const warning of style.warnings) console.warn(`[Radar] ${warning.message}`);
    // Warn once per distinct set of warnings.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [style.warnings.map((w) => w.rule).join()]);

  // The radar's size, from estimated label sizes, so the server and the client agree.
  const estimates: Size[] = axes.map((name) => ({
    width: textWidth(name, tile ? TYPE.micro : TYPE.label) * 1.1 + 2,
    height: LABEL_HEIGHT,
  }));
  const geometryFor = (width: number, h: number) =>
    radarGeometry(width, h, estimates, { gap: tile ? 8 : 12, max: density === "hero" ? 210 : 180 });
  const angles = spokeAngles(data.length);

  const measured = useMeasured();
  const tip = useRef<HTMLDivElement>(null);
  const marker = useRef<HTMLSpanElement>(null);
  /** Each series' radii as drawn this frame, for picking the nearest series. */
  const drawn = useRef(new Map<string, number[]>());
  const lead = focusSeries && visible.includes(focusSeries) ? focusSeries : null;

  const { box, canvas, lights, svg } = useRadialLayer({
    theme,
    replay,
    material: style.materials.solid === "flat" ? "flat" : "glass",
    frame: (clock) => drawFrame(clock),
  });

  function drawFrame(clock: LayerClock) {
    if (status) return { shapes: [], lines: [] };
    const { width, height: h } = clock;
    const g = geometryFor(width, h);
    const result = drawRadarFrame(clock, {
      series: series.map((key) => ({
        key,
        centre: { x: g.cx, y: g.cy },
        angles,
        radii: radarRadii(
          data.map((_, i) => (!hidden.has(key) ? valueOf(key, i) : 0)),
          domain,
          g.radius,
        ),
        tint: tints[key],
        hidden: hidden.has(key),
        fill,
      })),
      lead,
    });
    drawn.current = result.drawn;
    place(g, width, h);
    return result.scene;
  }

  /** Axis names, the focus marker and the tooltip. */
  function place(g: { cx: number; cy: number; radius: number }, width: number, h: number) {
    const bounds: Rect = { left: 2, top: 2, right: width - 2, bottom: h - 2 };

    // The tooltip first, out past the circle along the focused spoke, so the names can make way.
    let tipRect: Rect | null = null;
    const card = tip.current;
    if (card && active !== null && tooltip) {
      placeTooltip(
        card,
        { left: 0, top: 0, right: 0, bottom: 0 },
        { left: 4, top: 4, right: width - 4, bottom: h - 4 },
        {
          prefer: "outward",
          ring: { cx: g.cx, cy: g.cy, outer: g.radius + 6, angle: angles[active] },
        },
      );
      const placed = /translate\(([-\d.]+)px, ([-\d.]+)px\)/.exec(card.style.transform);
      if (placed) {
        const left = Number(placed[1]),
          top = Number(placed[2]);
        tipRect = { left, top, right: left + card.offsetWidth, bottom: top + card.offsetHeight };
      }
    }

    const sizes = axes.map((_, i) => {
      const s = measured.size(`a-${i}`);
      const k = active === i ? LIFT : 1;
      return { width: s.width * k, height: s.height * k };
    });
    // The focused name places first, so it is never the one that gives way.
    const order = axes
      .map((_, i) => i)
      .sort((a, b) => (a === active ? -1 : b === active ? 1 : a - b));
    const spots = placeAxisLabels(
      order.map((i) => angles[i]),
      order.map((i) => sizes[i]),
      { cx: g.cx, cy: g.cy, radius: g.radius, gap: tile ? 8 : 12, bounds },
    );
    spots.forEach((spot, k) => {
      const i = order[k];
      const natural = measured.size(`a-${i}`);
      const { horizontal, vertical } = axisAnchor(angles[i]);
      // The lifted box grows away from the circle; the element keeps its natural box.
      const dx =
        active === i
          ? horizontal === "end"
            ? sizes[i].width - natural.width
            : horizontal === "middle"
              ? (sizes[i].width - natural.width) / 2
              : 0
          : 0;
      const dy =
        active === i
          ? vertical === "bottom"
            ? sizes[i].height - natural.height
            : vertical === "middle"
              ? (sizes[i].height - natural.height) / 2
              : 0
          : 0;
      const box = {
        left: spot.left,
        top: spot.top,
        right: spot.left + sizes[i].width,
        bottom: spot.top + sizes[i].height,
      };
      // A name under the tooltip makes way; the tooltip's title says it.
      const covered = Boolean(tipRect && overlaps(box, tipRect, 2));
      put(measured.element(`a-${i}`), spot.left + dx, spot.top + dy, spot.visible && !covered);
    });

    // The black hole on the focused point: the focused series, else the highest at that spoke.
    const element = marker.current;
    if (element) {
      let key: string | null = null;
      if (active !== null && active < data.length) {
        const values = visible.map((k) => valueOf(k, active));
        const first = readingOrder(values, lead ? visible.indexOf(lead) : null)[0];
        key = first === undefined || values[first] === null ? null : visible[first];
      }
      const r = key ? drawn.current.get(key)?.[active!] : undefined;
      if (key && r !== undefined && Number.isFinite(r)) {
        const at = polar(g.cx, g.cy, r, angles[active!]);
        element.style.transform = `translate(${at.x}px, ${at.y}px)`;
        element.style.setProperty("--marker", colours[key]);
        element.dataset.visible = "true";
      } else element.dataset.visible = "false";
    }
  }

  const pickAt = (x: number, y: number, size: { width: number; height: number }) => {
    if (status) return null;
    const g = geometryFor(size.width, size.height);
    const r = Math.hypot(x - g.cx, y - g.cy);
    if (r > g.radius + 48) return null;
    const index = spokeAt(angles, g.cx, g.cy, x, y);
    if (index !== null && visible.length > 1) {
      const near = seriesAt(
        visible.map((key) => drawn.current.get(key)?.[index] ?? Number.NaN),
        r,
      );
      const key = near === null ? null : visible[near];
      if (key !== focusSeries) setFocusSeries(key);
    }
    return index;
  };

  const onKeyDownCapture = (event: KeyboardEvent<HTMLDivElement>) => {
    if (visible.length < 2 || active === null) return;
    if (event.key !== "ArrowUp" && event.key !== "ArrowDown") return;
    const values = visible.map((key) => valueOf(key, active));
    const order = readingOrder(values, null)
      .filter((s) => values[s] !== null)
      .map((s) => visible[s]);
    if (!order.length) return;
    event.preventDefault();
    event.stopPropagation();
    const from = lead ? order.indexOf(lead) : -1;
    const next =
      event.key === "ArrowUp"
        ? order[from <= 0 ? (from < 0 ? 0 : order.length - 1) : from - 1]
        : order[from < 0 || from >= order.length - 1 ? 0 : from + 1];
    setFocusSeries(next);
  };

  const name = (key: string) => labelText(config, key);
  const speak =
    describe ??
    ((i: number) => {
      const values = visible.map((key) => valueOf(key, i));
      const order = readingOrder(values, lead ? visible.indexOf(lead) : null);
      return `${axes[i]}: ${order.map((s) => `${name(visible[s])} ${plain(values[s], format)}`).join(", ")}.`;
    });
  const showLegend = (legend ?? series.length > 1) && !status;
  const focusIndex = active !== null && active < data.length ? active : null;
  const tooltipRows: TooltipRow[] = [];
  if (focusIndex !== null) {
    const values = visible.map((key) => valueOf(key, focusIndex));
    for (const s of readingOrder(values, lead ? visible.indexOf(lead) : null)) {
      const key = visible[s];
      const quiet = lead !== null && key !== lead;
      tooltipRows.push({
        label: labelNode(config, key),
        color: colours[key],
        muted: quiet,
        value:
          values[s] === null ? (
            <span className={`${roles.caption} ${mutedText}`}>no data</span>
          ) : (
            <Figure
              value={values[s]!}
              {...format}
              className={`${roles.figureS} ${quiet ? mutedText : ""}`}
            />
          ),
      });
    }
  }

  return (
    <ChartScope theme={theme} className={`${styles.chart} ${className ?? ""}`}>
      <div onKeyDownCapture={onKeyDownCapture}>
        <ChartFrame
          credit={<LiquidCredit />}
          theme={theme}
          height={plotHeight}
          insets={{ left: 0, right: 0, top: 0, bottom: 0 }}
          count={status ? 0 : data.length}
          active={status ? null : focusIndex}
          onActiveChange={(index) => {
            setActive(index);
            if (index === null) setFocusSeries(null);
          }}
          plotRef={box}
          pick={pickAt}
          label={`${label}. Use the arrow keys to move round the criteria${visible.length > 1 ? ", and Up and Down to move between series" : ""}.`}
          describe={speak}
          table={{
            caption: label,
            columns: ["Criterion", ...series.map(name)],
            rows: data.map((_, i) => [
              axes[i],
              ...series.map((key) => plain(valueOf(key, i), format)),
            ]),
          }}
        >
          {!status && (
            <Web
              geometryFor={geometryFor}
              angles={angles}
              domain={domain}
              rings={ringValues}
              grid={grid}
              active={focusIndex}
            />
          )}
          <canvas ref={canvas} className={styles.canvas} aria-hidden="true" />
          <svg ref={svg} className={styles.layer} aria-hidden="true" />
          <canvas ref={lights} className={styles.lights} aria-hidden="true" />
          {!status && !tile && (
            <RingLabels
              geometryFor={geometryFor}
              domain={domain}
              rings={ringValues}
              angles={angles}
              grid={grid}
              values={visible.map((key) => data.map((_, i) => valueOf(key, i)))}
              format={formatRing}
            />
          )}
          {status?.kind === "loading" && (
            <ChartState kind="loading" shape="ring" ring={{ inner: 0 }} />
          )}
          {status && status.kind !== "loading" && (
            <ChartState
              kind={status.kind}
              message={status.kind === "error" ? stateMessage(error) : stateMessage(empty)}
            />
          )}
          {!status &&
            axes.map((axis, i) => {
              const { horizontal, vertical } = axisAnchor(angles[i]);
              return (
                <div
                  key={`a-${i}`}
                  ref={measured.ref(`a-${i}`)}
                  className={`${styles.placed} ${styles.name}`}
                  data-active={focusIndex === i}
                  data-figure-active={focusIndex === i}
                  aria-hidden="true"
                >
                  <span
                    className={`${tile ? roles.micro : roles.label} ${roles.lift}`}
                    style={{
                      display: "inline-block",
                      transformOrigin: `${horizontal === "start" ? "0" : horizontal === "end" ? "100%" : "50%"} ${vertical === "bottom" ? "100%" : vertical === "top" ? "0" : "50%"}`,
                    }}
                  >
                    {axis}
                  </span>
                </div>
              );
            })}
          {!status && (
            <FocusMarker
              ref={marker}
              colour={colours[lead ?? visible[0] ?? series[0]] ?? "#fff"}
              visible={false}
            />
          )}
          {!status && tooltip && focusIndex !== null && (
            <ChartTooltip ref={tip} title={axes[focusIndex]} rows={tooltipRows} />
          )}
        </ChartFrame>
      </div>
      {showLegend && (
        <Legend
          items={series.map((key) => ({ key, label: labelNode(config, key), color: colours[key] }))}
          hidden={hidden}
          onToggle={(key) =>
            setHidden((was) => {
              const next = new Set(was);
              if (next.has(key)) next.delete(key);
              else if (series.length - next.size > 1) next.add(key);
              return next;
            })
          }
        />
      )}
    </ChartScope>
  );
}

/** The web: rings at round values (polygons through the spokes, or circles) and the spokes
 * themselves. Flat and quiet, under the data; `RingLabels` names the rings. */
function Web({
  geometryFor,
  angles,
  domain,
  rings,
  grid,
  active,
}: {
  geometryFor: (width: number, height: number) => { cx: number; cy: number; radius: number };
  angles: readonly number[];
  domain: readonly [number, number];
  rings: readonly number[];
  grid: "polygon" | "circle";
  active: number | null;
}) {
  const { size } = useChartFrame();
  if (!size.width) return null;
  const g = geometryFor(size.width, size.height);
  const radiusOf = (value: number) =>
    ((value - domain[0]) / (domain[1] - domain[0] || 1)) * g.radius;
  return (
    <>
      <svg className={styles.layer} aria-hidden="true">
        {rings.map((value) => {
          const r = radiusOf(value);
          const outer = Math.abs(value - domain[1]) < 1e-9;
          return grid === "circle" ? (
            <circle
              key={value}
              className={styles.axis}
              data-strong={outer}
              cx={g.cx}
              cy={g.cy}
              r={r}
            />
          ) : (
            <path
              key={value}
              className={styles.axis}
              data-strong={outer}
              d={radarPath(
                g.cx,
                g.cy,
                angles,
                angles.map(() => r),
              )}
            />
          );
        })}
        {angles.map((angle, i) => {
          const end = polar(g.cx, g.cy, g.radius, angle);
          return (
            <line
              key={i}
              className={`${styles.axis} ${styles.spoke}`}
              data-active={active === i}
              x1={g.cx}
              y1={g.cy}
              x2={end.x}
              y2={end.y}
            />
          );
        })}
      </svg>
    </>
  );
}

/** The rings' values up the empty way between two spokes, where no polygon's corner lands: the
 * gap whose polygon edges pass furthest from the figures, preferring the first. Each figure sits on
 * its ring with a halo in the card's colour; one an edge would still run through is left out. */
function RingLabels({
  geometryFor,
  domain,
  rings,
  angles,
  grid,
  values,
  format,
}: {
  geometryFor: (width: number, height: number) => { cx: number; cy: number; radius: number };
  domain: readonly [number, number];
  rings: readonly number[];
  angles: readonly number[];
  grid: "polygon" | "circle";
  /** Each visible series' values, spoke by spoke. */
  values: readonly (readonly (number | null)[])[];
  format: (value: number) => string;
}) {
  const { size } = useChartFrame();
  if (!size.width) return null;
  const g = geometryFor(size.width, size.height);
  const radiusOf = (value: number) =>
    Math.max(0, ((value - domain[0]) / (domain[1] - domain[0] || 1)) * g.radius);
  const n = angles.length;
  const step = n > 1 ? angles[1] - angles[0] : 0;
  // A web's ring crosses the bisector nearer the centre than its corners.
  const reach = grid === "polygon" && step ? Math.cos(step / 2) : 1;
  /** Where each series' edge between spokes k and k + 1 crosses their bisector, px from the centre. */
  const crossings = (k: number) =>
    values.flatMap((row) => {
      const a = row[k],
        b = row[(k + 1) % n];
      if (a === null || b === null || a === undefined || b === undefined) return [];
      const r1 = radiusOf(a),
        r2 = radiusOf(b);
      return r1 + r2 > 0 ? [(2 * r1 * r2 * Math.cos(step / 2)) / (r1 + r2)] : [0];
    });
  const clearance = (r: number, edges: readonly number[]) =>
    edges.reduce((least, e) => Math.min(least, Math.abs(r - e)), Infinity);
  let best = 0,
    bestScore = -Infinity;
  for (let k = 0; k < Math.max(1, n); k++) {
    const edges = crossings(k);
    const score = Math.min(...rings.map((v) => clearance(radiusOf(v) * reach, edges)), 1e6);
    // A later gap must clear by a few px more to win over the first.
    if (score > bestScore + (k === 0 ? 0 : 3)) {
      best = k;
      bestScore = score;
    }
  }
  const along = (angles[best] ?? 0) + step / 2;
  const edges = crossings(best);
  return (
    <>
      {rings.map((value) => {
        if (clearance(radiusOf(value) * reach, edges) < 6) return null;
        const at = polar(g.cx, g.cy, radiusOf(value) * reach, along);
        return (
          <span
            key={value}
            className={`${styles.ringLabel} ${roles.micro}`}
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
            {format(value)}
          </span>
        );
      })}
    </>
  );
}
