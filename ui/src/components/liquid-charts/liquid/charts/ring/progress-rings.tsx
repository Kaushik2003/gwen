"use client";
/*
Liquid Charts by Ojieame · https://liquid.ojieame.design · nathan@ojieame.design

Liquid Charts adaptation. Original foundation: Evil Charts by Gurbinder.
Upstream: ecbd6a5db7b25f9a7e070c02b2ef728c378eeb5c
MIT License

Copyright (c) 2026 Gurbinder
Copyright (c) 2026 NATEPAY, LLC (Liquid Charts additions)

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
  moveTooltip,
  mutedText,
  seriesColour,
  TooltipFoot,
  useActive,
  useChartFrame,
  useMaterialNote,
  useMaterials,
  type ChartConfig,
  type Density,
  type LabelMode,
  type Rgb,
  type Theme,
} from "@/components/liquid-charts/liquid/core";
import {
  progressArc,
  progressGeometry,
  ringTooltipPosition,
  TAU,
  type Point,
} from "./ring-geometry";
import { useRingMarks, type RingFrame, type RingMark } from "@/components/liquid-charts/liquid/rings/ring-marks";
import { formatText, labelNode, labelText, type RingFormat } from "./ring-parts";
import { usePageTheme } from "@/components/liquid-charts/liquid/core/page-theme";
import { cssModule } from "@/components/liquid-charts/liquid/core/css-module";
import Roles from "@/components/liquid-charts/liquid/core/type.module.css";
import { useRef, type ReactNode } from "react";
import Styles from "./ring.module.css";

const roles = cssModule(Roles, "lc-type-");
const styles = cssModule(Styles, "lc-ring-");

export type ProgressDatum = {
  /** The ring's key in `config`, which gives its label and colours. */
  key: string;
  value: number;
  goal: number;
};

export type ProgressRingsProps = {
  data: readonly ProgressDatum[];
  config: ChartConfig;
  /** Light or dark. Without one, the page's: `data-theme` or a `dark` class on `<html>`, else the system setting. @default the page's */
  theme?: Theme;
  height?: number;
  density?: Density;
  active?: number | null;
  onActiveChange?: (index: number | null) => void;
  /** "none" hides the row beside the rings that names each one. */
  labels?: LabelMode;
  /** Defaults on only when the rows are hidden, since they already say the same. */
  tooltip?: boolean;
  format?: RingFormat;
  /** A unit after each value and goal in the rows, e.g. "kcal". */
  units?: readonly (string | undefined)[];
  /** Ring thickness and the gap between rings, px. */
  thickness?: number;
  gap?: number;
  material?: "glass" | "flat";
  loading?: boolean;
  empty?: boolean | ReactNode;
  error?: boolean | ReactNode;
  replay?: number;
  label: string;
  describe?: (index: number) => string;
  className?: string;
};

const FALLBACK_TINT: Rgb = [0.6, 0.8, 0.18];

/** Tracks: each ring's full lap, quiet, under the glass. */
function Tracks({
  count,
  colours,
  thickness,
  gap,
  theme,
}: {
  count: number;
  colours: readonly string[];
  thickness?: number;
  gap?: number;
  theme: Theme;
}) {
  const { size } = useChartFrame();
  if (!size.width) return null;
  const g = progressGeometry(size.width, size.height, count, { thickness, gap });
  return (
    <svg className={`${styles.layer} ${styles.tracks}`} aria-hidden="true">
      {g.radii.map((radius, i) => (
        <circle
          key={i}
          cx={g.cx}
          cy={g.cy}
          r={radius}
          strokeWidth={g.thickness}
          style={{
            stroke: `color-mix(in srgb, ${colours[i]} ${theme === "light" ? 16 : 13}%, transparent)`,
          }}
        />
      ))}
    </svg>
  );
}

/**
 * One or several concentric progress rings, each against its goal: glass arcs with round ends
 * that sweep in from 12 o'clock over a quiet track. Past the goal a ring closes and a second,
 * brighter lap rides on top of the first, so 124% reads as more than full without breaking the
 * ring. A row per ring names it with its value, goal and share of goal. Concentric rings of
 * different radii are drawn by the optical glass renderer, flat view only; without WebGL, or with
 * `material="flat"`, in SVG.
 */
export function ProgressRings({
  data,
  config,
  theme: themeProp,
  height,
  density = "standard",
  active: activeProp,
  onActiveChange,
  labels = "auto",
  tooltip,
  format = { kind: "number" },
  units,
  thickness,
  gap = 5,
  material: materialProp,
  loading = false,
  empty = false,
  error = false,
  replay = 0,
  label,
  describe,
  className,
}: ProgressRingsProps) {
  const theme = usePageTheme(themeProp);
  const tile = density === "tile";
  const plotHeight = height ?? (tile ? 112 : density === "hero" ? 300 : 220);
  const showRows = labels !== "none" && !tile;
  const showTooltip = tooltip ?? !showRows;
  const [active, setActive] = useActive(activeProp, onActiveChange);
  const tip = useRef<HTMLDivElement>(null);
  const centreRef = useRef<HTMLDivElement>(null);
  const colours = data.map((d) => seriesColour(config, d.key, theme));
  const tints = data.map((_, i) => cssToRgb(colours[i]) ?? FALLBACK_TINT);
  const arcs = data.map((d) => progressArc(d.value, d.goal));
  const nothing = empty || (!loading && !error && data.length === 0);
  const status = loading ? "loading" : error ? "error" : nothing ? "empty" : null;

  const material = materialProp ?? "glass";
  useMaterialNote("glass", materialProp === "glass", Boolean(useMaterials().ring));
  const { box, canvas, svg, shade } = useRingMarks({
    theme,
    replay,
    material,
    layout: (width, h) => {
      const g = progressGeometry(width, h, data.length, { thickness, gap });
      const list: RingMark[] = [];
      // In a state (loading, empty, error) the plot shows the state, not the rings.
      (status ? [] : data).forEach((d, i) => {
        const lit = active === null || active === i ? 1 : 0;
        list.push({
          key: d.key,
          start: 0,
          end: arcs[i].end,
          tint: tints[i],
          lit,
          radius: g.radii[i],
          width: g.thickness,
          round: true,
          delay: i * 0.12,
        });
        if (arcs[i].overflow !== null) {
          // Past the goal: a second lap, as wide as the first, from 12 o'clock on top of it,
          // etched with the pattern that means "beyond a limit", its end casting a soft shadow
          // onto the lap below. The goal stays at 12 o'clock for every ring.
          list.push({
            key: `${d.key}~lap`,
            start: 0,
            end: arcs[i].overflow,
            tint: tints[i],
            lit,
            radius: g.radii[i],
            width: g.thickness,
            round: true,
            pattern: true,
            shadow: true,
            delay: i * 0.12 + 1.1,
          });
        }
      });
      return {
        ring: { cx: g.cx, cy: g.cy, radius: g.radii[0] ?? g.outer, width: g.thickness, gap: 0 },
        marks: list,
        sweep: { from: 0, span: TAU },
      };
    },
    onFrame: (frame) => place(frame),
  });

  function place(frame: RingFrame) {
    const g = progressGeometry(frame.width, frame.height, data.length, { thickness, gap });
    const centreEl = centreRef.current;
    if (centreEl) {
      centreEl.style.left = `${g.cx}px`;
      centreEl.style.top = `${g.cy}px`;
      centreEl.dataset.placed = "";
    }
    const element = tip.current;
    if (!element || active === null) return;
    // Beside the ring's leading end, out past the outermost ring.
    const angle = Math.min(Math.max(arcs[active].end, 0.2), TAU - 0.2);
    const size = { width: element.offsetWidth, height: element.offsetHeight };
    const at = ringTooltipPosition(size, { cx: g.cx, cy: g.cy, outer: g.outer + 2 }, angle, {
      left: 4,
      top: 4,
      right: frame.width - 4,
      bottom: frame.height - 4,
    });
    moveTooltip(element, at.left, at.top);
  }

  /** The ring under a point: by radius, so its track counts as well as its arc. */
  function pick(x: number, y: number, size: { width: number; height: number }) {
    const g = progressGeometry(size.width, size.height, data.length, { thickness, gap });
    const r = Math.hypot(x - g.cx, y - g.cy);
    let best: number | null = null,
      distance = Infinity;
    g.radii.forEach((radius, i) => {
      const d = Math.abs(r - radius);
      if (d <= g.thickness / 2 + gap / 2 + 2 && d < distance) {
        distance = d;
        best = i;
      }
    });
    return best;
  }

  const unit = (i: number) => (units?.[i] ? ` ${units[i]}` : "");
  const speak =
    describe ??
    ((i: number) =>
      `${labelText(config, data[i].key)}: ${formatText(data[i].value, format)}${unit(i)} of ${formatText(data[i].goal, format)}${unit(i)}, ${formatText(arcs[i].progress, { kind: "percent" })} of goal.`);
  const single = data.length === 1;
  const focus = single ? 0 : active;

  const plot = (
    <ChartFrame
      theme={theme}
      height={plotHeight}
      insets={{ left: 0, right: 0, top: 0, bottom: 0 }}
      count={status ? 0 : data.length}
      active={active}
      onActiveChange={setActive}
      plotRef={box}
      pick={(x, y, size) => (status ? null : pick(x, y, size))}
      label={`${label}. Use the arrow keys to read each ring.`}
      describe={speak}
      table={{
        caption: label,
        columns: ["Ring", "Value", "Goal", "Of goal"],
        rows: data.map((d, i) => [
          labelText(config, d.key),
          formatText(d.value, format),
          formatText(d.goal, format),
          formatText(arcs[i].progress, { kind: "percent" }),
        ]),
      }}
    >
      {!status && (
        <Tracks
          count={data.length}
          colours={colours}
          thickness={thickness}
          gap={gap}
          theme={theme}
        />
      )}
      <svg ref={svg} className={styles.layer} aria-hidden="true" />
      <canvas ref={canvas} className={styles.canvas} aria-hidden="true" />
      <svg ref={shade} className={styles.layer} aria-hidden="true" />
      {status === "loading" && <ChartState kind="loading" shape="ring" ring={{ inner: 0.55 }} />}
      {status && status !== "loading" && (
        <ChartState
          kind={status}
          message={
            status === "error"
              ? typeof error === "boolean"
                ? undefined
                : error
              : typeof empty === "boolean"
                ? undefined
                : empty
          }
        />
      )}
      {!status && focus !== null && (
        <div ref={centreRef} className={styles.centre}>
          <Figure
            value={arcs[focus].progress}
            kind="percent"
            className={`${single && !tile ? roles.figureXL : tile ? roles.figureM : roles.figureL} ${styles.centreValue}`}
          />
          {!tile && (
            <span className={`${styles.centreCaption} ${roles.caption}`}>
              {labelNode(config, data[focus].key)}
            </span>
          )}
        </div>
      )}
      {!status && showTooltip && active !== null && (
        <ChartTooltip
          ref={tip}
          title={labelNode(config, data[active].key)}
          rows={[
            {
              label: "Value",
              color: colours[active],
              value: <Figure value={data[active].value} {...format} className={roles.figureS} />,
            },
            {
              label: "Goal",
              color: colours[active],
              muted: true,
              value: (
                <Figure
                  value={data[active].goal}
                  {...format}
                  className={`${roles.figureS} ${mutedText}`}
                />
              ),
            },
          ]}
          footer={
            <TooltipFoot>
              <Figure value={arcs[active].progress} kind="percent" /> of goal
            </TooltipFoot>
          }
        />
      )}
    </ChartFrame>
  );

  if (!showRows || status)
    return <div className={`${styles.chart} ${className ?? ""}`}>{plot}</div>;
  return (
    <div className={`${styles.chart} ${styles.progressWrap} ${className ?? ""}`}>
      <ChartScope theme={theme} className={styles.progress}>
        {plot}
        <ul className={styles.rows}>
          {data.map((d, i) => {
            const over = arcs[i].progress >= 1;
            return (
              <li
                key={d.key}
                className={styles.row}
                data-active={active === i}
                data-dim={active !== null && active !== i}
                data-figure-active={active === i}
                onPointerEnter={() => setActive(i)}
                onPointerLeave={() => setActive(null)}
              >
                <span
                  className={styles.rowSwatch}
                  style={{ background: colours[i] }}
                  aria-hidden="true"
                />
                <span className={styles.rowName}>
                  <span className={roles.label}>{labelNode(config, d.key)}</span>
                  <span className={`${styles.rowGoal} ${roles.caption}`}>
                    <Figure value={d.value} {...format} /> of <Figure value={d.goal} {...format} />
                    {unit(i)}
                  </span>
                </span>
                <span className={`${styles.rowPercent} ${over ? styles.over : ""}`}>
                  <Figure
                    value={arcs[i].progress}
                    kind="percent"
                    className={`${roles.figureM} ${roles.lift}`}
                  />
                </span>
              </li>
            );
          })}
        </ul>
      </ChartScope>
    </div>
  );
}

/** The point where a progress ring's arc ends, for pages that annotate it. */
export function progressEnd(
  geometry: { cx: number; cy: number; radii: readonly number[] },
  index: number,
  value: number,
  goal: number,
): Point {
  const { end } = progressArc(value, goal);
  const r = geometry.radii[index] ?? 0;
  return { x: geometry.cx + Math.sin(end) * r, y: geometry.cy - Math.cos(end) * r };
}
