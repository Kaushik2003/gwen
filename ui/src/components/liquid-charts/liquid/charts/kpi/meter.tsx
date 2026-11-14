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
  ChartScope,
  cssToRgb,
  Figure,
  MarkLayer,
  seriesColour,
  SkeletonBlock,
  useValueLabels,
  ValueLabel,
  type BarMark,
  type ChartConfig,
  type Rgb,
  type Theme,
} from "@/components/liquid-charts/liquid/core";
import {
  bandFor,
  DENSITY,
  formatValue,
  meterSegments,
  referenceGap,
  segmentSlots,
  type Band,
  type KpiDensity,
  type KpiFormat,
} from "./layout";
import {
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";
import { KPI_LAYER, meterMaterial, useSize } from "./kpi-marks";
import { cssModule } from "@/components/liquid-charts/liquid/core/css-module";
import Roles from "@/components/liquid-charts/liquid/core/type.module.css";
import { GlassLayer } from "@/components/liquid-charts/liquid/glass";
import { KpiState, kpiState } from "./kpi-state";
import Styles from "./kpi.module.css";

const roles = cssModule(Roles, "lc-type-");
const styles = cssModule(Styles, "lc-kpi-");

export type MeterProps = {
  value: number;
  min?: number;
  max: number;
  /** A reference (an SLA, a plan to date, a pass mark): it lights the gap between the segments
   * nearest it. */
  target?: number | null;
  targetLabel?: string;
  /** Named ranges with their own colours: the reading takes the band its value falls in. */
  bands?: readonly Band[];
  /** How many glass segments. Default 20. */
  segments?: number;
  /** Filled segments take the colour of the value's band ("value", the default), or each
   * segment takes the colour of its own band ("segment"), so an overspend shows as its own run. */
  colorBy?: "value" | "segment";
  config: ChartConfig;
  /** The config key for filled segments outside any band. Defaults to `value`, else the first. */
  colorKey?: string;
  theme: Theme;
  /** Segment height. */
  height?: number;
  density?: KpiDensity;
  /** The value above the meter. Its band shows as the segments' colour; the band's name is read
   * out with the value. */
  readout?: boolean;
  /** A label after the value in the readout ("uptime", "spent"). */
  valueLabel?: ReactNode;
  /** The scale's ends and the target label. */
  labels?: boolean;
  format?: KpiFormat;
  /** The scale labels' format, when it should be shorter than the readout's. */
  scaleFormat?: KpiFormat;
  material?: "glass" | "flat";
  loading?: boolean;
  empty?: boolean | ReactNode;
  error?: boolean | ReactNode;
  replay?: number;
  label: string;
  describe?: string;
  className?: string;
};

const FALLBACK: Rgb = [0.56, 0.56, 0.56];
const useIsomorphicLayoutEffect = typeof window === "undefined" ? useEffect : useLayoutEffect;

/**
 * A segmented linear meter for scores, SLAs and budgets: whole glass segments lit to the value
 * (the nearest segment; the readout and the marker carry the precise figure), unfilled segments in
 * graphite with their rims, a reference lit in the gap nearest it, and bands that colour the
 * reading (and name it for screen readers).
 */
export function Meter({
  value,
  min = 0,
  max,
  target,
  targetLabel = "Target",
  bands,
  segments = 20,
  colorBy = "value",
  config,
  colorKey,
  theme,
  height,
  density = "standard",
  readout = true,
  valueLabel,
  labels = true,
  format = { kind: "number" },
  scaleFormat = format,
  material,
  loading = false,
  empty = false,
  error = false,
  replay = 0,
  label,
  describe,
  className,
}: MeterProps) {
  const metrics = DENSITY[density];
  const thickness = height ?? (density === "tile" ? 12 : density === "hero" ? 40 : 28);
  const gap = density === "tile" ? 2 : 3;
  const gauge = useRef<HTMLDivElement>(null);
  const size = useSize(gauge);
  const key = colorKey ?? (config.value ? "value" : (Object.keys(config)[0] ?? "value"));
  const band = bandFor(Math.min(Math.max(value, min), max), bands);
  const colour = seriesColour(config, band?.key ?? key, theme);
  const slots = segmentSlots(size.width, segments, gap);
  // The count or a forced material decides flat for good; a narrow width only stands flat in
  // while it lasts, with the glass kept underneath.
  const flat = meterMaterial({ material, density, segments, smallest: thickness }) === "flat";
  const narrow =
    !flat &&
    size.width > 0 &&
    meterMaterial({ density, segments, smallest: Math.min(thickness, slots[0]?.size ?? 0) }) ===
      "flat";
  // The reference sits in the gap nearest it, and the gap glows (style rule 9): never a line
  // drawn through the segments.
  const gapIndex = referenceGap(target, { min, max, count: segments });
  const gapAt = (width: number) => {
    if (gapIndex === null) return null;
    const places = segmentSlots(width, segments, gap);
    const before = places[gapIndex - 1],
      after = places[gapIndex];
    return before && after ? (before.start + before.size + after.start) / 2 : null;
  };
  const targetAt = size.width ? gapAt(size.width) : null;
  const refColour = seriesColour(config, key, theme);
  const state = kpiState({ loading, empty, error });
  const [active, setActive] = useState(false);
  const descriptionId = useId();
  const text = [
    formatValue(value, format),
    band?.label?.toLowerCase(),
    target !== null && target !== undefined && Number.isFinite(target)
      ? `${targetLabel.toLowerCase()} ${formatValue(target, format)}`
      : null,
    `on a scale of ${formatValue(min, format)} to ${formatValue(max, format)}`,
  ]
    .filter(Boolean)
    .join(", ");
  const [spoken, setSpoken] = useState({ text, announce: "" });
  if (spoken.text !== text) setSpoken({ text, announce: text });

  const layout = (width: number, h: number): BarMark[] => {
    const cells = meterSegments(value, { min, max, count: segments, bands });
    const places = segmentSlots(width, segments, gap);
    const valueTint = cssToRgb(colour) ?? FALLBACK;
    return cells.map((cell, i) => {
      const own =
        colorBy === "segment"
          ? (cssToRgb(seriesColour(config, cell.band?.key ?? key, theme)) ?? FALLBACK)
          : valueTint;
      return {
        key: `segment-${i}`,
        cross: h / 2,
        base: places[i].start,
        thickness: h,
        length: places[i].size,
        direction: "right",
        tint: own,
        lit: cell.on ? 1 : 0,
        delay: i * 0.025,
      };
    });
  };

  // The scale's labels place themselves: the target wins, the ends give way.
  const scale = useValueLabels();
  const placeScale = useRef<() => void>(() => {});
  useIsomorphicLayoutEffect(() => {
    placeScale.current = () => {
      const width = gauge.current?.offsetWidth ?? 0;
      if (!width) return;
      const requests = [
        { id: "min", x: 0, y: 0, side: "below" as const, offset: 0, priority: 1 },
        { id: "max", x: width, y: 0, side: "below" as const, offset: 0, priority: 1 },
      ];
      // Positions from the same measurement as the bounds, never a stale size.
      const at = gapAt(width);
      if (at !== null)
        requests.push({ id: "target", x: at, y: 0, side: "below", offset: 0, priority: 2 });
      scale.place(requests, { bounds: { left: 0, top: 0, right: width, bottom: 24 } });
    };
    placeScale.current();
  });
  useEffect(() => {
    let live = true;
    document.fonts?.ready.then(() => live && placeScale.current());
    return () => {
      live = false;
    };
  }, []);

  return (
    <ChartScope theme={theme} className={className}>
      <div
        className={styles.meter}
        data-density={density}
        data-figure-active={active}
        style={{ "--kpi-thickness": `${thickness}px`, "--kpi-liquid": colour } as CSSProperties}
        onPointerEnter={() => setActive(true)}
        onPointerLeave={() => setActive(false)}
      >
        {readout && (
          <div className={styles.readout} data-density={density}>
            {state ? (
              <KpiState {...state} />
            ) : (
              <>
                <span className={`${styles.reading} ${roles.lift}`}>
                  <Figure value={value} {...format} className={roles[metrics.figure]} />
                  {valueLabel && (
                    <span className={`${styles.of} ${roles.caption}`}>{valueLabel}</span>
                  )}
                </span>
              </>
            )}
          </div>
        )}
        <div
          ref={gauge}
          className={styles.gauge}
          role="meter"
          tabIndex={state ? -1 : 0}
          aria-label={label}
          aria-valuemin={min}
          aria-valuemax={max}
          aria-valuenow={state ? undefined : value}
          aria-valuetext={state ? undefined : text}
          aria-describedby={describe ? descriptionId : undefined}
          aria-busy={loading || undefined}
          onFocus={() => setActive(true)}
          onBlur={() => setActive(false)}
        >
          <div className={styles.track} data-state={state?.kind}>
            {state?.kind === "loading" ? (
              <SkeletonBlock />
            ) : state ? (
              <span className={styles.emptyTrack} />
            ) : (
              <MarkLayer
                Glass={GlassLayer}
                {...KPI_LAYER}
                flat={flat}
                fallback={narrow}
                theme={theme}
                replay={replay}
                layout={layout}
              />
            )}
            {!state && targetAt !== null && (
              <span
                className={styles.gapGlow}
                style={{ left: targetAt, "--kpi-ref": refColour } as CSSProperties}
                aria-hidden="true"
              />
            )}
          </div>
          {labels && !state && (
            <div className={styles.scale} aria-hidden="true">
              <ValueLabel ref={scale.ref("min")}>
                <Figure value={min} {...scaleFormat} signed={false} className={roles.micro} />
              </ValueLabel>
              <ValueLabel ref={scale.ref("max")}>
                <Figure value={max} {...scaleFormat} signed={false} className={roles.micro} />
              </ValueLabel>
              {targetAt !== null && (
                <ValueLabel ref={scale.ref("target")} active>
                  <span className={`${styles.scaleTarget} ${roles.micro}`}>
                    {targetLabel} <Figure value={target!} {...scaleFormat} signed={false} />
                  </span>
                </ValueLabel>
              )}
            </div>
          )}
        </div>
        {describe && (
          <p id={descriptionId} className={styles.srOnly}>
            {describe}
          </p>
        )}
        <p className={styles.srOnly} aria-live="polite">
          {spoken.announce}
        </p>
      </div>
    </ChartScope>
  );
}
