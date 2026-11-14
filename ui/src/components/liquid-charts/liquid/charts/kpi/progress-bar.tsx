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
  DENSITY,
  levelOf,
  levelScale,
  levelText,
  markShare,
  positionOf,
  thresholdFor,
  type KpiDensity,
  type KpiFormat,
  type Level,
  type Threshold,
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
import { KPI_LAYER, levelMaterial, useSize } from "./kpi-marks";
import { cssModule } from "@/components/liquid-charts/liquid/core/css-module";
import Roles from "@/components/liquid-charts/liquid/core/type.module.css";
import { GlassLayer } from "@/components/liquid-charts/liquid/glass";
import { KpiState, kpiState } from "./kpi-state";
import Styles from "./kpi.module.css";

const roles = cssModule(Roles, "lc-type-");
const styles = cssModule(Styles, "lc-kpi-");

export type ProgressBarProps = {
  /** The level: amount used, booked, spent or done. */
  value: number;
  /** The capacity, quota or goal: the container's size. */
  max: number;
  min?: number;
  /** A point on the scale, engraved into the glass: a pace, plan or goal. Omit for none. */
  target?: number | null;
  /** Names the target in its label and in the description ("Pace", "Plan", "Goal"). */
  targetLabel?: string;
  /** Shares of `max` where the reading changes: the liquid takes `config[key]`, and `label`
   * (above 0.9, "Near capacity"; at 1, "Over capacity") is read out with the value. Below 1 each
   * one is also engraved into the capsule. */
  thresholds?: readonly Threshold[];
  /** The status when over `max` and no threshold names it. Default "Over capacity". */
  overLabel?: string;
  /** Labels the limit (the pattern's seam) when the value is over `max`. Default "Max". */
  capacityLabel?: string;
  config: ChartConfig;
  /** The config key for the liquid. Defaults to `value`, else the first key. */
  colorKey?: string;
  theme: Theme;
  /** A name above the readout ("Primary volume"). */
  name?: ReactNode;
  orientation?: "horizontal" | "vertical";
  /** Horizontal: the capsule's thickness. Vertical: its height. */
  height?: number;
  density?: KpiDensity;
  /** What the readout shows: amounts and percent (true), one of them, or nothing. */
  readout?: boolean | "amount" | "percent";
  /** The target and capacity labels and the threshold engravings. */
  labels?: boolean;
  format?: KpiFormat;
  material?: "glass" | "flat";
  loading?: boolean;
  empty?: boolean | ReactNode;
  error?: boolean | ReactNode;
  /** Change to replay the entrance. */
  replay?: number;
  /** The accessible name. */
  label: string;
  /** A longer description, read after the value. */
  describe?: string;
  className?: string;
};

const FALLBACK: Rgb = [0.56, 0.56, 0.56];
const useIsomorphicLayoutEffect = typeof window === "undefined" ? useEffect : useLayoutEffect;

/** The liquid's colour and status for a level: a threshold it has reached, or the series. */
export function levelReading(
  reading: Level,
  {
    config,
    colorKey,
    thresholds,
    theme,
    overLabel = "Over capacity",
  }: {
    config: ChartConfig;
    colorKey?: string;
    thresholds?: readonly Threshold[];
    theme: Theme;
    overLabel?: string;
  },
) {
  const key = colorKey ?? (config.value ? "value" : (Object.keys(config)[0] ?? "value"));
  const threshold = thresholdFor(reading.ratio, thresholds);
  const colour = seriesColour(config, threshold?.key ?? key, theme);
  const status = threshold?.label ?? (reading.over ? overLabel : null);
  return { key, threshold, colour, status, tone: threshold ? "alert" : "normal" } as const;
}

/**
 * A level against a capacity, as liquid in a glass capsule: storage used, quota booked, budget
 * spent. The readout gives the value, the capacity and the share; a target is engraved into the
 * glass, lit in the liquid's colour once the liquid passes it; thresholds recolour the liquid (and
 * say why to a screen reader). Over capacity the capsule stays one piece: its scale runs on to the value, the liquid fills
 * to it, and the stretch past the limit is etched, its seam marking the limit.
 * Focus or hover lifts the reading, and a change of value is announced.
 */
export function ProgressBar({
  value,
  max,
  min = 0,
  target,
  targetLabel = "Target",
  thresholds,
  overLabel,
  capacityLabel = "Max",
  config,
  colorKey,
  theme,
  name,
  orientation = "horizontal",
  height,
  density = "standard",
  readout = true,
  labels = true,
  format = { kind: "number" },
  material,
  loading = false,
  empty = false,
  error = false,
  replay = 0,
  label,
  describe,
  className,
}: ProgressBarProps) {
  const metrics = DENSITY[density];
  const vertical = orientation === "vertical";
  const thickness = vertical ? metrics.thickness * 1.6 : (height ?? metrics.thickness);
  const length = vertical ? (height ?? 200) : 0;
  const reading = levelOf(value, max, min);
  const scale = levelScale(value, max, min);
  const { colour, status } = levelReading(reading, {
    config,
    colorKey,
    thresholds,
    theme,
    overLabel,
  });
  const flat = levelMaterial({ material, density, thickness }) === "flat";

  const gauge = useRef<HTMLDivElement>(null);
  const size = useSize(gauge);
  const along = vertical ? length : size.width;
  const targetAt = positionOf(target, min, scale.top, along);
  const capacityAt = scale.over ? scale.capacity * along : null;
  // Reference marks live in the glass (style rule 9): the target and any threshold below the
  // capacity are engraved into the capsule. Over capacity the limit is the pattern's seam instead,
  // and thresholds step back.
  const marks = [
    markShare(target, min, scale.top),
    ...(labels && density !== "tile" && max > min && !scale.over
      ? (thresholds ?? [])
          .filter((t) => t.at > 0 && t.at < 1)
          .map((t) => markShare(min + t.at * (max - min), min, scale.top))
      : []),
  ]
    .filter((share): share is number => share !== null)
    .slice(0, 4);
  const stateInfo = kpiState({ loading, empty, error });
  const state = stateInfo?.kind ?? null;
  const showTargetLabel = labels && targetAt !== null && !state;
  const showCapacityLabel = labels && capacityAt !== null && !state && !vertical;
  const [active, setActive] = useState(false);
  const descriptionId = useId();
  const text = levelText(value, max, format, {
    min,
    target,
    targetLabel: targetLabel.toLowerCase(),
    status,
  });

  // Announce a changed value, never the first one.
  const [spoken, setSpoken] = useState({ text, announce: "" });
  if (spoken.text !== text) setSpoken({ text, announce: text });

  // Over capacity the stretch past the limit is etched into the same glass (style rule 10).
  const pattern = scale.over ? scale.capacity : undefined;
  const layout = (width: number, h: number): BarMark[] => {
    const tint = cssToRgb(colour) ?? FALLBACK;
    return [
      vertical
        ? {
            key: "level",
            cross: width / 2,
            base: h,
            thickness: width,
            length: h,
            direction: "up",
            tint,
            lit: 1,
            level: scale.fill,
            pattern,
            marks,
          }
        : {
            key: "level",
            cross: h / 2,
            base: 0,
            thickness: h,
            length: width,
            direction: "right",
            tint,
            lit: 1,
            level: scale.fill,
            pattern,
            marks,
          },
    ];
  };

  // The ruler's labels place themselves: the target wins, the capacity gives way.
  const ruler = useValueLabels();
  const placeRuler = useRef<() => void>(() => {});
  useIsomorphicLayoutEffect(() => {
    placeRuler.current = () => {
      const width = gauge.current?.offsetWidth ?? 0;
      if (!width || vertical) return;
      // Positions from the same measurement as the bounds, never a stale size.
      const at = positionOf(target, min, scale.top, width);
      const limit = scale.over ? scale.capacity * width : null;
      const requests = [];
      if (showTargetLabel && at !== null)
        requests.push({
          id: "target",
          x: at,
          y: 0,
          side: "below" as const,
          offset: 0,
          priority: 2,
        });
      if (showCapacityLabel && limit !== null)
        requests.push({
          id: "capacity",
          x: limit,
          y: 0,
          side: "below" as const,
          offset: 0,
          priority: 1,
        });
      if (requests.length)
        ruler.place(requests, { bounds: { left: 0, top: 0, right: width, bottom: 24 } });
    };
    placeRuler.current();
  });
  useEffect(() => {
    let live = true;
    document.fonts?.ready.then(() => live && placeRuler.current());
    return () => {
      live = false;
    };
  }, []);

  const showPercent = readout === true || readout === "percent";
  const showAmount = readout === true || readout === "amount";

  return (
    <ChartScope theme={theme} className={className}>
      <div
        className={styles.progress}
        data-orientation={orientation}
        data-density={density}
        data-figure-active={active}
        style={
          {
            "--kpi-thickness": `${thickness}px`,
            "--kpi-length": `${length}px`,
            "--kpi-liquid": colour,
          } as CSSProperties
        }
        onPointerEnter={() => setActive(true)}
        onPointerLeave={() => setActive(false)}
      >
        {name && <span className={`${styles.progressName} ${roles.label}`}>{name}</span>}
        {readout !== false && !vertical && (
          <div className={styles.readout} data-density={density}>
            {stateInfo ? (
              <KpiState {...stateInfo} />
            ) : (
              <>
                <span className={`${styles.reading} ${roles.lift}`}>
                  {showAmount ? (
                    <>
                      <Figure value={value} {...format} className={roles[metrics.figure]} />
                      <span className={`${styles.of} ${roles.caption}`}>of</span>
                      <Figure
                        value={max}
                        {...format}
                        className={`${roles.figureS} ${styles.quiet}`}
                      />
                    </>
                  ) : (
                    <Figure
                      value={reading.ratio}
                      kind="percent"
                      className={roles[metrics.figure]}
                    />
                  )}
                </span>
                {showPercent && showAmount && (
                  <span className={styles.aside}>
                    <Figure value={reading.ratio} kind="percent" className={roles.figureS} />
                  </span>
                )}
              </>
            )}
          </div>
        )}
        <div className={styles.gaugeRow}>
          <div
            ref={gauge}
            className={styles.gauge}
            role="progressbar"
            tabIndex={state ? -1 : 0}
            aria-label={label}
            aria-valuemin={min}
            aria-valuemax={max}
            aria-valuenow={state ? undefined : value}
            aria-valuetext={state ? undefined : text}
            aria-describedby={describe ? descriptionId : undefined}
            aria-busy={loading || undefined}
            data-labelled={!vertical && (showTargetLabel || showCapacityLabel)}
            onFocus={() => setActive(true)}
            onBlur={() => setActive(false)}
          >
            <div className={styles.track} data-state={state ?? undefined}>
              {state === "loading" ? (
                <SkeletonBlock />
              ) : state ? (
                <span className={styles.emptyTrack} />
              ) : (
                <>
                  <span className={styles.well} aria-hidden="true" />
                  <MarkLayer
                    Glass={GlassLayer}
                    {...KPI_LAYER}
                    flat={flat}
                    theme={theme}
                    replay={replay}
                    layout={layout}
                  />
                </>
              )}
            </div>
            {!vertical && (showTargetLabel || showCapacityLabel) && (
              <div className={styles.scale} aria-hidden="true">
                {showTargetLabel && (
                  <ValueLabel ref={ruler.ref("target")}>
                    <span className={`${styles.scaleTarget} ${roles.micro}`}>
                      {targetLabel} <Figure value={target!} {...format} signed={false} />
                    </span>
                  </ValueLabel>
                )}
                {showCapacityLabel && (
                  <ValueLabel ref={ruler.ref("capacity")}>
                    <span className={`${styles.scaleCapacity} ${roles.micro}`}>
                      {capacityLabel}
                    </span>
                  </ValueLabel>
                )}
              </div>
            )}
            {vertical && showTargetLabel && (
              <span
                className={`${styles.targetLabel} ${roles.micro}`}
                style={{ bottom: targetAt! }}
                aria-hidden="true"
              >
                {targetLabel} <Figure value={target!} {...format} signed={false} />
              </span>
            )}
          </div>
          {vertical && readout !== false && (
            <div className={styles.sideReadout}>
              {stateInfo ? (
                <KpiState {...stateInfo} />
              ) : (
                <>
                  <span className={`${styles.stack} ${roles.lift}`}>
                    <Figure
                      value={showAmount ? value : reading.ratio}
                      {...(showAmount ? format : { kind: "percent" as const })}
                      className={roles[metrics.figure]}
                    />
                    {showAmount && (
                      <span className={`${roles.caption} ${styles.quiet}`}>
                        of <Figure value={max} {...format} />
                      </span>
                    )}
                  </span>
                  {showPercent && showAmount && (
                    <Figure value={reading.ratio} kind="percent" className={roles.figureS} />
                  )}
                </>
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
