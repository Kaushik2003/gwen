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

import { afterFrameDelay, onSettle } from "@/components/liquid-charts/liquid/core/presence";

import {
  ChartFrame,
  ChartState,
  ChartTooltip,
  cssToRgb,
  Figure,
  mixRgb,
  moveTooltip,
  mutedText,
  resolveStyle,
  seriesColour,
  TooltipFoot,
  useActive,
  useMaterialNote,
  useMaterials,
  type ChartConfig,
  type Density,
  type LabelMode,
  type Rgb,
  type Theme,
} from "@/components/liquid-charts/liquid/core";
import {
  bandOf,
  bandsOffDomain,
  gaugeAngle,
  gaugeBands,
  gaugeDecimals,
  gaugeGeometry,
  polar,
  ringTooltipPosition,
  stopDecimals,
  type RingFit,
} from "./ring-geometry";
import { useRingMarks, type RingFrame } from "@/components/liquid-charts/liquid/rings/ring-marks";
import { formatText, labelNode, labelText, type RingFormat } from "./ring-parts";
import { useEffect, useRef, useState, type ReactNode, type Ref } from "react";
import { usePageTheme } from "@/components/liquid-charts/liquid/core/page-theme";
import { cssModule } from "@/components/liquid-charts/liquid/core/css-module";
import Roles from "@/components/liquid-charts/liquid/core/type.module.css";
import { settle } from "@/components/liquid-charts/liquid/core/geometry";
import Styles from "./ring.module.css";

const roles = cssModule(Roles, "lc-type-");
const styles = cssModule(Styles, "lc-ring-");

export type GaugeBandDatum = {
  /** The band's key in `config`, which gives its label and colours. */
  key: string;
  /** The band's upper bound; it starts where the previous band ends (or at the domain's minimum). */
  to: number;
};

export type GaugeProps = {
  value: number;
  bands: readonly GaugeBandDatum[];
  config: ChartConfig;
  domain?: readonly [number, number];
  /** The arc's sweep in degrees, centred on 12 o'clock. */
  sweep?: number;
  /** A goal, marked across the ring. */
  target?: number;
  /** Light or dark. Without one, the page's: `data-theme` or a `dark` class on `<html>`, else the system setting. @default the page's */
  theme?: Theme;
  height?: number;
  density?: Density;
  active?: number | null;
  onActiveChange?: (index: number | null) => void;
  /** "none" hides band names and the scale. */
  labels?: LabelMode;
  tooltip?: boolean;
  /** How the value and scale are set. A plain number without `decimals` takes as many as the
   * domain needs (none on 0–100, two on 0–1). */
  format?: RingFormat;
  /** A caption under the value; defaults to the band the value falls in. `false` shows the value
   * alone (the band names round the dial already say which band it is in). */
  caption?: ReactNode | false;
  /** How far the needle reaches, as a share of the way to the arc's inner edge. Default 0.84: it
   * stops well inside the glass. */
  needle?: number;
  material?: "glass" | "flat";
  loading?: boolean;
  empty?: boolean | ReactNode;
  error?: boolean | ReactNode;
  replay?: number;
  label: string;
  describe?: (index: number) => string;
  className?: string;
};

const FALLBACK_TINT: Rgb = [0.62, 0.62, 0.62];
const DEG = 180 / Math.PI;

/**
 * A banded gauge: glass bands (poor, fair, good…) round an arc, a needle that swings to the value,
 * and the value set large under its pivot, with the band it falls in. A gauge is a precise reading,
 * so it never tilts. Hover or step through the bands to read each one's range. Without WebGL, or
 * with `material="flat"`, the bands draw in SVG.
 */
export function Gauge({
  value,
  bands,
  config,
  domain = [0, 100],
  sweep: sweepDegrees = 240,
  target,
  theme: themeProp,
  height,
  density = "standard",
  active: activeProp,
  onActiveChange,
  labels = "auto",
  tooltip = true,
  format: formatProp,
  caption,
  needle: needleReach = 0.84,
  material: materialProp,
  loading = false,
  empty = false,
  error = false,
  replay = 0,
  label,
  describe,
  className,
}: GaugeProps) {
  const theme = usePageTheme(themeProp);
  const tile = density === "tile";
  const plotHeight = height ?? (tile ? 120 : 250);
  const sweep = (Math.max(60, Math.min(340, sweepDegrees)) * Math.PI) / 180;
  const labelled = labels !== "none" && !tile;
  const [active, setActive] = useActive(activeProp, onActiveChange);
  const tip = useRef<HTMLDivElement>(null);
  const bandEls = useRef<(HTMLSpanElement | null)[]>([]);
  const tickEls = useRef<(HTMLSpanElement | null)[]>([]);
  const readingRef = useRef<HTMLDivElement>(null);
  const needleRef = useRef<SVGGElement>(null);
  const targetRef = useRef<SVGLineElement>(null);
  const colours = bands.map((band) => seriesColour(config, band.key, theme));
  const tints = bands.map((_, i) => cssToRgb(colours[i]) ?? FALLBACK_TINT);
  const ranges = gaugeBands(bands, domain, sweep);
  const holds = bandOf(value, ranges);
  const plainNumber = (formatProp?.kind ?? "number") === "number";
  const format: RingFormat =
    plainNumber && formatProp?.decimals === undefined
      ? { ...formatProp, kind: "number", decimals: gaugeDecimals(domain) }
      : (formatProp as RingFormat);
  // Thresholds in other units than the domain (0–100 on a 0–1 gauge) all land on the dial's ends.
  const stray = bandsOffDomain(bands, domain);
  const offDomain = stray.map((band) => `"${band.key ?? band.index}" (to: ${band.to})`).join(", ");
  useEffect(() => {
    if (process.env.NODE_ENV === "production" || !offDomain) return;
    console.warn(
      `[liquid-charts] Gauge "${label}": ${stray.length > 1 ? `bands ${offDomain} fall` : `band ${offDomain} falls`} outside the domain [${domain[0]}, ${domain[1]}] (clamped to the dial's end). Give each band's \`to\` in the domain's units.`,
    );
    // Said once per set of stray bands, not on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [offDomain, domain[0], domain[1]]);
  const nothing = empty || (!loading && !error && (!bands.length || !Number.isFinite(value)));
  const status = loading ? "loading" : error ? "error" : nothing ? "empty" : null;
  // Gauges are precise readings: the style never offers them a tilt.
  const material = materialProp ?? "glass";
  useMaterialNote("glass", materialProp === "glass", Boolean(useMaterials().ring));
  const style = resolveStyle({
    geometry: "circular",
    marks: ["solid"],
    precise: true,
    segments: bands.length,
    density,
    material,
  });

  // The needle swings from the scale's start to the value once the bands begin to fill, and
  // springs to every new value after that.
  const [swing, setSwing] = useState({ angle: -sweep / 2, instant: true });
  const goal = gaugeAngle(value, domain, sweep);
  const latestGoal = useRef(goal);
  useEffect(() => {
    latestGoal.current = goal;
  }, [goal]);
  // A replay puts the needle back at the start, in the render that brings it.
  const [entered, setEntered] = useState(replay);
  if (entered !== replay) {
    setEntered(replay);
    setSwing({ angle: -sweep / 2, instant: true });
  }
  useEffect(() => {
    return afterFrameDelay(320, () => setSwing({ angle: latestGoal.current, instant: false }));
    // Only the entrance resets the needle; value changes are handled below.
  }, [replay]);
  useEffect(() => {
    // Mount/replay must keep the scale-start pose until the entrance delay has elapsed.
    // Otherwise this value-update effect races the needle's first ResizeObserver layout and
    // sometimes mounts it directly at the goal, without a transition at all.
    if (swing.instant) return;
    const frame = requestAnimationFrame(() =>
      setSwing((current) => (current.angle === goal ? current : { angle: goal, instant: false })),
    );
    return () => cancelAnimationFrame(frame);
  }, [goal, swing.instant]);

  const {
    box,
    canvas,
    svg,
    pick: pickAt,
  } = useRingMarks({
    theme,
    replay,
    material: style.materials.solid === "glass" ? "glass" : "flat",
    layout: (width, h) => {
      const g = gaugeGeometry(width, h, sweep, { labelled, tile });
      return {
        ring: {
          cx: g.cx,
          cy: g.cy,
          radius: g.radius,
          width: g.ringWidth,
          gap: tile ? 3 : 5,
          bevel: Math.min(5, g.ringWidth * 0.2),
        },
        sweep: { from: -sweep / 2, span: sweep },
        // In a state (loading, empty, error) the plot shows the state, not the bands.
        marks: (status ? [] : ranges).map((range, i) => ({
          key: bands[i].key,
          start: range.start,
          end: range.end,
          tint: tints[i],
          lit: active === null || active === i ? 1 : 0,
        })),
      };
    },
    onFrame: (frame) => place(frame),
  });

  // A settled draw (print, export) shows the needle at the value without waiting for its swing:
  // turned at once on the element, its transition finished, and in state so later renders agree.
  useEffect(() => {
    const root = box.current;
    if (!root) return;
    return onSettle(root, () => {
      const angle = latestGoal.current;
      const needle = needleRef.current;
      if (needle) {
        needle.style.rotate = `${settle(angle * DEG)}deg`;
        for (const animation of needle.getAnimations()) animation.finish();
      }
      setSwing((current) => (current.angle === angle && !current.instant ? current : { angle, instant: false }));
    });
  }, [box]);

  function place(frame: RingFrame) {
    const g = gaugeGeometry(frame.width, frame.height, sweep, { labelled, tile });
    // The needle turns about its own box's centre (ring.module.css), which its reach keeps on the
    // pivot, so nothing here sets its origin.
    const needle = needleRef.current;
    if (needle) needle.dataset.placed = "";
    if (readingRef.current) {
      readingRef.current.style.left = `${g.cx}px`;
      readingRef.current.style.top = `${g.cy + (tile ? 4 : 16)}px`;
      readingRef.current.dataset.placed = "";
    }
    ranges.forEach((range, i) => {
      const element = bandEls.current[i];
      if (!element) return;
      // Each name sits just past the ring, its near edge towards it.
      const at = polar(g.cx, g.cy, g.outer + 9, range.mid);
      const s = Math.sin(range.mid),
        c = Math.cos(range.mid);
      element.style.transform = `translate(${settle(at.x)}px, ${settle(at.y)}px) translate(${settle(-50 + 50 * s)}%, ${settle(-50 - 50 * c)}%)`;
      element.style.visibility = range.end - range.start > 0.12 ? "visible" : "hidden";
    });
    // The scale inside the ring: its ends and every threshold between bands.
    const stops = [domain[0], ...ranges.map((range) => range.to)];
    stops.forEach((stop, i) => {
      const element = tickEls.current[i];
      if (!element) return;
      const at = polar(g.cx, g.cy, g.inner - 13, gaugeAngle(stop, domain, sweep));
      element.style.transform = `translate(${settle(at.x)}px, ${settle(at.y)}px) translate(-50%, -50%)`;
      element.style.visibility = "visible";
    });
    if (targetRef.current && target !== undefined) {
      const angle = gaugeAngle(target, domain, sweep);
      // A notch on the inside of the ring, pointing at the goal.
      const a = polar(g.cx, g.cy, g.inner - 3, angle),
        b = polar(g.cx, g.cy, g.inner - 10, angle);
      targetRef.current.setAttribute("x1", String(settle(a.x)));
      targetRef.current.setAttribute("y1", String(settle(a.y)));
      targetRef.current.setAttribute("x2", String(settle(b.x)));
      targetRef.current.setAttribute("y2", String(settle(b.y)));
    }
    const element = tip.current;
    if (element && active !== null && ranges[active]) {
      const size = { width: element.offsetWidth, height: element.offsetHeight };
      const at = ringTooltipPosition(
        size,
        { cx: g.cx, cy: g.cy, outer: g.outer + 18 },
        ranges[active].mid,
        {
          left: 4,
          top: 4,
          right: frame.width - 4,
          bottom: frame.height - 4,
        },
      );
      moveTooltip(element, at.left, at.top);
    }
  }

  const name = (i: number) => labelText(config, bands[i].key);
  const range = (i: number) =>
    `${formatText(ranges[i].from, format)}–${formatText(ranges[i].to, format)}`;
  const speak =
    describe ??
    ((i: number) =>
      `${name(i)}: ${range(i)}.${holds === i ? ` The value, ${formatText(value, format)}, is in this band.` : ""}`);

  // The needle and its hub are drawn in plot px from the same geometry the glass uses; until the
  // frame has measured the plot they wait hidden. The needle is a watch's hand: a slender pill with
  // a round tip and a round heel under the hub, a lit face and a shaded one either side of its
  // ridge, a bright rim down the lit edge, and along its middle a luminous strip set in a groove.
  const needle = (width: number) => {
    const g = gaugeGeometry(width, plotHeight, sweep, { labelled, tile });
    const { cx, cy } = g;
    const length = (g.inner - (tile ? 4 : 8)) * Math.min(1, Math.max(0.2, needleReach));
    // As broad as stays clear of the reading under the hub when the needle rests at either end of
    // the dial; a tile's hand is narrower than its hub, so turned flat it stays clear too.
    const half = tile ? 5 : 10;
    const end = tile ? 2 : 3.5;
    const tip = cy - length;
    return {
      g,
      hand: `M${cx - half} ${cy}L${cx - end} ${tip + end}A${end} ${end} 0 0 1 ${cx + end} ${tip + end}L${cx + half} ${cy}A${half} ${half} 0 0 1 ${cx - half} ${cy}Z`,
      lit: `M${cx - half} ${cy}L${cx - end} ${tip + end}L${cx} ${tip}L${cx} ${cy}Z`,
      shaded: `M${cx} ${cy}L${cx} ${tip}L${cx + end} ${tip + end}L${cx + half} ${cy}Z`,
      rim: `M${cx - half + 1.2} ${cy}L${cx - end + 0.6} ${tip + end}`,
      lume: `M${cx} ${cy - length * 0.38}L${cx} ${cy - length * 0.86}`,
      reach: length + 1,
      hub: tile ? 5.5 : 9.5,
    };
  };

  return (
    <div className={`${styles.chart} ${className ?? ""}`}>
      <ChartFrame
        theme={theme}
        height={plotHeight}
        insets={{ left: 0, right: 0, top: 0, bottom: 0 }}
        count={status ? 0 : bands.length}
        active={active}
        onActiveChange={setActive}
        plotRef={box}
        pick={(x, y) => (status ? null : pickAt(x, y, 6))}
        label={`${label}: ${formatText(value, format)}${holds !== null ? `, ${name(holds)}` : ""}. Use the arrow keys to read each band.`}
        describe={speak}
        table={{
          caption: label,
          columns: ["Band", "From", "To"],
          rows: bands.map((_band, i) => [
            name(i),
            formatText(ranges[i].from, format),
            formatText(ranges[i].to, format),
          ]),
        }}
      >
        <svg ref={svg} className={styles.layer} aria-hidden="true" />
        <canvas ref={canvas} className={styles.canvas} aria-hidden="true" />
        {status === "loading" && (
          <ChartState
            kind="loading"
            shape="ring"
            ring={{ inner: 0.78, from: -sweep / 2, span: sweep }}
          />
        )}
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
        {!status && (
          <GaugeNeedle
            build={needle}
            swing={swing}
            needleRef={needleRef}
            targetRef={targetRef}
            hasTarget={target !== undefined}
            tint={tints[holds ?? 0] ?? FALLBACK_TINT}
            theme={theme}
          />
        )}
        {!status &&
          labelled &&
          bands.map((band, i) => (
            <span
              key={band.key}
              ref={(element) => {
                bandEls.current[i] = element;
              }}
              className={`${styles.bandName} ${roles.caption}`}
              data-active={active === i || (active === null && holds === i)}
              data-dim={active !== null && active !== i}
              style={{ visibility: "hidden", transform: "none" }}
              aria-hidden="true"
            >
              {labelNode(config, band.key)}
            </span>
          ))}
        {!status &&
          labelled &&
          [domain[0], ...ranges.map((r) => r.to)].map((stop, i) => (
            <span
              key={i}
              ref={(element) => {
                tickEls.current[i] = element;
              }}
              className={`${styles.tickLabel} ${roles.micro}`}
              style={{ visibility: "hidden", transform: "none" }}
              aria-hidden="true"
            >
              <Figure
                value={stop}
                {...format}
                decimals={plainNumber ? stopDecimals(stop, format.decimals ?? 0) : format.decimals}
              />
            </span>
          ))}
        {!status && (
          <div
            ref={readingRef}
            className={styles.centre}
            data-anchor="top"
            data-figure-active={active === null}
          >
            <Figure
              value={value}
              {...format}
              className={`${tile ? roles.figureL : roles.figureXL} ${styles.centreValue}`}
            />
            {!tile && caption !== false && (
              <span className={`${styles.centreCaption} ${roles.caption}`}>
                {holds !== null && (
                  <span
                    className={styles.rowSwatch}
                    style={{ background: colours[holds] }}
                    aria-hidden="true"
                  />
                )}
                {caption ?? (holds !== null ? labelNode(config, bands[holds].key) : null)}
              </span>
            )}
          </div>
        )}
        {!status && tooltip && active !== null && bands[active] && (
          <ChartTooltip
            ref={tip}
            title={labelNode(config, bands[active].key)}
            rows={[
              {
                label: "Range",
                color: colours[active],
                value: (
                  <span className={roles.figureS}>
                    <Figure value={ranges[active].from} {...format} />
                    <span className={mutedText}>–</span>
                    <Figure value={ranges[active].to} {...format} />
                  </span>
                ),
              },
            ]}
            footer={
              holds === active ? (
                <TooltipFoot>
                  Now <Figure value={value} {...format} />
                </TooltipFoot>
              ) : undefined
            }
          />
        )}
      </ChartFrame>
    </div>
  );
}

/** The hand's shadow, three soft strokes from wide and faint to close and firm, and the hub's. */
const HAND_SHADOW = [
  [9, 0.48],
  [5, 0.72],
  [2.5, 1],
] as const;
const HUB_SHADOW = [
  [5, 0.55],
  [2.5, 0.85],
] as const;
const WHITE: Rgb = [1, 1, 1];
const INK: Rgb = [0.08, 0.08, 0.08];
const css = ([r, g, b]: Rgb) => `rgb(${Math.round(r * 255)} ${Math.round(g * 255)} ${Math.round(b * 255)})`;

/** The needle and its hub, sized from the plot's width once the frame has measured it. */
function GaugeNeedle({
  build,
  swing,
  needleRef,
  targetRef,
  hasTarget,
  tint,
  theme,
}: {
  build: (width: number) => {
    g: RingFit;
    hand: string;
    lit: string;
    shaded: string;
    rim: string;
    lume: string;
    reach: number;
    hub: number;
  };
  swing: { angle: number; instant: boolean };
  needleRef: Ref<SVGGElement>;
  targetRef: Ref<SVGLineElement>;
  hasTarget: boolean;
  /** The colour of the band the value is in. */
  tint: Rgb;
  theme: Theme;
}) {
  const [width, setWidth] = useState(0);
  const svg = useRef<SVGSVGElement>(null);
  useEffect(() => {
    const element = svg.current;
    if (!element) return;
    const observer = new ResizeObserver(([entry]) => setWidth(entry.contentRect.width));
    observer.observe(element);
    return () => observer.disconnect();
  }, []);
  const { g, hand, lit, shaded, rim, lume, reach, hub } = build(width || 1);
  // The lume is the band's colour: a shade deeper on the white hand of a light card, a shade
  // lighter and glowing on the dark hand of a dark one, with a finer, lighter line along it.
  const dark = theme === "dark";
  const strip = css(dark ? mixRgb(tint, WHITE, 0.15) : mixRgb(tint, INK, 0.1));
  const shine = css(mixRgb(tint, WHITE, dark ? 0.65 : 0.55));
  return (
    <svg ref={svg} className={styles.layer} aria-hidden="true">
      {hasTarget && <line ref={targetRef} className={styles.goalTick} />}
      {width > 0 && (
        <g
          ref={needleRef}
          className={styles.needle}
          data-instant={swing.instant}
          style={{ rotate: `${settle(swing.angle * DEG)}deg` }}
        >
          {/* Unpainted, as long as the needle's reach: it centres the group's box on the pivot,
              which the needle turns about (a pixel origin drifts under Safari's page zoom). */}
          <circle cx={g.cx} cy={g.cy} r={reach} style={{ fill: "none", stroke: "none" }} />
          {HAND_SHADOW.map(([stroke, opacity]) => (
            <path key={stroke} className={styles.needleShadow} d={hand} style={{ strokeWidth: stroke, opacity }} />
          ))}
          <path className={styles.needleHand} d={hand} />
          <path className={styles.needleShaded} d={shaded} />
          <path className={styles.needleLit} d={lit} />
          <path className={styles.needleRim} d={rim} />
          {dark && <path className={styles.needleLume} d={lume} style={{ stroke: strip, strokeOpacity: 0.3, strokeWidth: 9 }} />}
          <path className={styles.needleGroove} d={lume} />
          <path className={styles.needleLume} d={lume} style={{ stroke: strip, strokeWidth: 3.6 }} />
          <path className={styles.needleLume} d={lume} style={{ stroke: shine, strokeOpacity: 0.85, strokeWidth: 1.2 }} />
          {HUB_SHADOW.map(([grow, opacity]) => (
            <circle key={grow} className={styles.needleHubShadow} cx={g.cx} cy={g.cy} r={hub + grow / 2} style={{ opacity }} />
          ))}
          <circle className={styles.needleHub} cx={g.cx} cy={g.cy} r={hub} />
        </g>
      )}
    </svg>
  );
}
