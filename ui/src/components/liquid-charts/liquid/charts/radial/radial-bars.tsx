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
  HeightNote,
  LIFT,
  mutedText,
  overlaps,
  placeTooltip,
  seriesColour,
  stateOf,
  textWidth,
  tickFormatter,
  TooltipFoot,
  TYPE,
  useActive,
  useChartFrame,
  valueTicks,
  ViewSwitch,
  type ChartConfig,
  type Density,
  type LabelMode,
  type Rect,
  type Rgb,
  type Theme,
  type TooltipRow,
  type View,
} from "@/components/liquid-charts/liquid/core";
import {
  barAngle,
  placeArcLabels,
  polar,
  radialBarsGeometry,
  radialBarsTilt,
  radialDomain,
  startLabelBoxes,
  tiltHeights,
  trackAt,
  trackOrder,
  type ArcLabelRequest,
  type RadialArc,
  type RadialBarsGeometry,
  type TrackGlass,
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
import {
  layoutPinLabels,
  pinPoints,
  slabExtent,
  slabPin,
  type PinRequest,
} from "@/components/liquid-charts/liquid/rings";
import { useRingMarks, type RingFrame, type RingMark } from "@/components/liquid-charts/liquid/rings/ring-marks";
import { useEffect, useRef, useState, type CSSProperties, type ReactNode } from "react";
import { radialBarFrame, radialBarStyle, RadialBarTrack } from "./radial-bar-frame";
import { useRadialLayer, type LayerClock } from "./use-radial-layer";
import Pins from "@/components/liquid-charts/liquid/rings/pin-labels.module.css";
import { usePageTheme } from "@/components/liquid-charts/liquid/core/page-theme";
import { cssModule } from "@/components/liquid-charts/liquid/core/css-module";
import Roles from "@/components/liquid-charts/liquid/core/type.module.css";
import { createRingGlass } from "@/components/liquid-charts/liquid/glass";
import { LiquidCredit } from "@/components/liquid-charts/liquid/credit";
import Styles from "./radial.module.css";

const pins = cssModule(Pins, "lc-pin-labels-");
const roles = cssModule(Roles, "lc-type-");
const styles = cssModule(Styles, "lc-radial-");

export type RadialBarDatum = {
  /** The track's key in `config`, which gives its name and colour. */
  key: string;
  value: number;
  /** A point on the scale to reach (a goal, a quota): engraved in the glass once the bar passes
   * it, a notch in the etched track before that. */
  target?: number;
  /** A second measure, for the tilted view's height (`heightMeaning="second-measure"`). */
  measure?: number;
};

export type RadialBarsProps = {
  data: readonly RadialBarDatum[];
  config: ChartConfig;
  /** Light or dark. Without one, the page's: `data-theme` or a `dark` class on `<html>`, else the system setting. @default the page's */
  theme?: Theme;
  height?: number;
  density?: Density;
  /** Three quarters of a turn from 12 o'clock (names in the open quadrant), or a half over the top
   * from 9 o'clock (names in a row under the baseline). */
  arc?: RadialArc;
  /** `descending` puts the longest bar outside; `data` keeps the data's order, first outside. */
  order?: "descending" | "data";
  /** The value at the end of the span. Defaults to a round number above every value and target. */
  max?: number;
  /** Track thickness and the gap between tracks, px. */
  thickness?: number;
  gap?: number;
  active?: number | null;
  onActiveChange?: (index: number | null) => void;
  /** Values at the bars' ends: `auto` and `all` show each that fits, `ends` the outermost and
   * innermost, `none` only the one in focus. */
  labels?: LabelMode;
  tooltip?: boolean;
  format?: ValueFormat;
  /** Spokes at round values with their figures round the rim (default on, except tiles). */
  scale?: boolean;
  /** What the tooltip calls a datum's target. */
  targetLabel?: string;
  material?: "glass" | "flat";
  /** What a tilted slab's height shows: the bar's value, or each datum's `measure`. Tilt is
   * offered only when height means something (glass, up to 12 tracks). */
  heightMeaning?: "value" | "second-measure" | "none";
  /** Flat, or the tracks seen from the side as glass slabs standing by their height. */
  view?: View;
  onViewChange?: (view: View) => void;
  /** The Flat / Tilt switch in the corner (default on when tilt is offered). */
  viewSwitch?: boolean;
  /** What the height reads as ("Of plan"), and how the measure is written. */
  measureLabel?: string;
  measureFormat?: ValueFormat;
  loading?: boolean;
  empty?: boolean | ReactNode;
  error?: boolean | ReactNode;
  replay?: number;
  label: string;
  describe?: (index: number) => string;
  className?: string;
};

const FALLBACK_TINT: Rgb = [0.62, 0.55, 0.92];

/**
 * Radial bars in edge-lit glass: one concentric track per category, each a glass arc sweeping
 * clockwise in an etched groove, the longest outside (or in the data's order). Names sit at the
 * start of each track; values ride at the arcs' ends and slide along their tracks to clear each
 * other. A target is engraved in the glass, glowing once the bar has passed it, and is a cool notch
 * in the track before that. Hover or the arrow keys light one track and turn the rest graphite; a
 * click pins it. `arc="semi"` runs the tracks over the top from 9 o'clock to 3 o'clock.
 *
 * Angles are read against `max` (by default a round number above the data), so every bar shares
 * one scale; values below zero stay at the start. Flat, the tracks are drawn by the optical glass
 * renderer; without WebGL, or with `material="flat"`, they draw in SVG. Where height means
 * something (`heightMeaning`), a Flat / Tilt switch stands the bars up: the arcs square their ends
 * and hand over to concentric raymarched glass tracks seen from the side, the open quadrant turned
 * to the front, each slab as tall as its value or second measure. Tilted, every slab names itself
 * on a pin from its value end, in a row above or below the ring (core `layoutPinLabels`).
 */
export function RadialBars({
  data,
  config,
  theme: themeProp,
  height,
  density = "standard",
  arc = "full",
  order = "descending",
  max,
  thickness,
  gap,
  active: activeProp,
  onActiveChange,
  labels = "auto",
  tooltip = true,
  format = { kind: "number" },
  scale,
  targetLabel = "Target",
  material = "glass",
  heightMeaning = "none",
  view: viewProp,
  onViewChange,
  viewSwitch = true,
  measureLabel,
  measureFormat = { kind: "number" },
  loading = false,
  empty = false,
  error = false,
  replay = 0,
  label,
  describe,
  className,
}: RadialBarsProps) {
  const theme = usePageTheme(themeProp);
  const tile = density === "tile";
  const semi = arc === "semi";
  const plotHeight = height ?? (tile ? 150 : density === "hero" ? 360 : semi ? 240 : 300);
  const showScale = (scale ?? !tile) && data.length > 0;
  const [active, setActive] = useActive(activeProp, onActiveChange);
  const values = data.map((d) => (finite(d.value) ? Math.max(0, d.value) : 0));
  const targets = data.map((d) => (finite(d.target) ? d.target : undefined));
  const nothing = !loading && !error && (data.length === 0 || values.every((v) => v <= 0));
  const status = stateOf({ loading, error, empty: empty || nothing });
  const top = max ?? radialDomain([...values, ...targets.filter(finite)], { count: 4 })[1];
  const ticks = valueTicks([0, top], tile ? 2 : 4).filter((t) => t >= 0 && t <= top);
  const formatTick = tickFormatter(format, [0, top]);
  const positions = trackOrder(values, order);
  const byPosition: number[] = [];
  positions.forEach((p, i) => (byPosition[p] = i));
  const colours = data.map((d) => seriesColour(config, d.key, theme));
  const tints = colours.map((c) => cssToRgb(c) ?? FALLBACK_TINT);
  const names = data.map((d) => labelText(config, d.key));

  const style = radialBarStyle({
    count: data.length,
    density,
    material,
    heightMeaning,
    view: viewProp,
  });
  useEffect(() => {
    if (process.env.NODE_ENV === "production") return;
    for (const warning of style.warnings) console.warn(`[RadialBars] ${warning.message}`);
    // Warn once per distinct set of warnings.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [style.warnings.map((w) => w.rule).join()]);

  // Room for the widest name (lifted) left of 12 o'clock; estimated, so the server agrees.
  const reach =
    data.reduce(
      (widest, d) => Math.max(widest, textWidth(labelText(config, d.key), TYPE.label)),
      0,
    ) *
      LIFT +
    14;
  const geometryFor = (width: number, h: number): RadialBarsGeometry =>
    radialBarsGeometry(width, h, data.length, {
      arc,
      thickness,
      gap,
      labelReach: semi ? 0 : reach,
      pad: showScale ? 26 : 8,
      bottom: semi ? (tile ? 8 : 32) : 0,
      max: density === "hero" ? 200 : 180,
    });

  // Values ride inside their tracks; on thin tracks they take the smaller figure.
  const small = tile || geometryFor(640, plotHeight).thickness < 18;
  const measured = useMeasured();
  const tip = useRef<HTMLDivElement>(null);
  const notches = useRef<(SVGLineElement | null)[]>([]);

  /* ——— The tilted view ——— */
  const measures = data.map((d) => (finite(d.measure) ? d.measure : undefined));
  const [localView, setLocalView] = useState<View | null>(null);
  /** True once the flat arcs have squared their ends and handed over to the raymarched tracks. */
  const [handed, setHanded] = useState(false);
  const tiltLayer = useRef<HTMLDivElement>(null);
  const tiltLeaders = useRef<(SVGPolylineElement | null)[]>([]);
  const noteRef = useRef<HTMLDivElement>(null);
  const canTiltStyle =
    heightMeaning !== "none" &&
    style.tilt !== "never" &&
    style.materials.solid === "glass" &&
    data.length <= 12 &&
    !tile &&
    !status;
  const heightsFor = (low: number, high: number) =>
    heightMeaning === "second-measure"
      ? tiltHeights(measures, low, high)
      : tiltHeights(values, low, high, top);

  const layer = useRadialLayer({
    theme,
    replay,
    material: style.materials.solid === "flat" ? "flat" : "glass",
    frame: (clock) => drawFrame(clock),
  });
  const { box, canvas, lights, svg } = layer;
  const {
    box: ringBox,
    canvas: ringCanvas,
    frame: ringFrame,
    pick: ringPick,
    available: ringAvailable,
  } = useRingMarks({
    glass: createRingGlass,
    theme,
    replay,
    material: "glass",
    layout: (width, h) => tiltLayout(width, h),
    onFrame: (frame) => placeTilted(frame),
  });
  const canTilt = canTiltStyle && layer.available && ringAvailable;
  const view: View = canTilt ? (viewProp ?? localView ?? style.view) : "flat";
  const tilted = view === "tilt";

  function drawFrame(clock: LayerClock) {
    if (status) return { shapes: [] };
    const { width, height: h, engine } = clock;
    const g = geometryFor(width, h);
    const frame = radialBarFrame(
      clock,
      data.map((d, i) => ({
        key: d.key,
        position: positions[i],
        cx: g.cx,
        cy: g.cy,
        radius: g.radii[positions[i]],
        width: g.thickness,
        start: g.from,
        end: barAngle(values[i], top, g.from, g.span),
        tint: tints[i],
        lit: active === null || active === i,
        targetAngle: targets[i] === undefined ? null : barAngle(targets[i]!, top, g.from, g.span),
      })),
      { tilted, handed },
    );
    if (frame.handoff) queueMicrotask(() => setHanded(true));
    place(g, width, h, frame.glass, frame.ends, frame.fills, engine, frame.flatShown);
    return { shapes: frame.shapes, moving: frame.moving };
  }

  /** The tilted tracks: every bar a slab on its own track, drawn by the raymarched ring once the
   * flat arcs have handed over. */
  function tiltLayout(width: number, h: number) {
    const g = geometryFor(width, h);
    // Most tracks end at the back, so a crowd of them staggers into a second row above.
    const shares = heightsFor(0, 1);
    const t = radialBarsTilt(width, h, g, {
      rows: { above: data.length > 4 ? 2 : 1, below: 1 },
      slabs: data.map((_, i) => ({
        start: g.from,
        end: barAngle(values[i], top, g.from, g.span),
        share: shares[i],
        radius: (g.radii[positions[i]] + g.thickness / 2) / Math.max(1, g.outer),
      })),
    });
    const heights = heightsFor(t.low, t.high);
    const marks: RingMark[] =
      handed && !status
        ? data.map((d, i) => {
            const p = positions[i];
            return {
              key: d.key,
              start: g.from,
              end: barAngle(values[i], top, g.from, g.span),
              tint: tints[i],
              lit: active === null || active === i ? 1 : 0.32,
              height: heights[i],
              radius: g.radii[p],
              width: g.thickness,
              tilted: { radius: t.radii[p], width: t.thickness },
            };
          })
        : [];
    return {
      ring: {
        cx: g.cx,
        cy: g.cy,
        radius: g.radii[0] ?? g.outer,
        width: g.thickness,
        depth: Math.max(g.thickness * 0.7, 10),
        gap: 0,
        bevel: Math.min(3, g.thickness * 0.2),
      },
      tilt: t.tilt,
      view: handed ? view : ("flat" as View),
      turn: t.turn,
      sweep: { from: g.from, span: g.span },
      marks,
    };
  }

  /** Tilted, each slab names itself on a pin from the top of its value end: back ends in a row
   * above the ring, front ends in a row below it, each on a vertical stem. The slab in focus
   * brightens its pin, and the tooltip sits beside its end, as it does flat. */
  function placeTilted(frame: RingFrame) {
    if (!tilted && handed && frame.phase <= 0) queueMicrotask(() => setHanded(false));
    const shown = Math.max(0, Math.min(1, (frame.phase - 0.6) / 0.3));
    const opacity = String(Math.round(shown * 100) / 100);
    if (tiltLayer.current) tiltLayer.current.style.opacity = opacity;
    if (noteRef.current) noteRef.current.style.opacity = opacity;
    const { segments, width, height: h } = frame;
    if (!segments.length) return;
    /** The stretch of a slab next to its value end: the tooltip sits beside it. */
    const endOf = (i: number) => {
      const s = segments[i];
      const span = s.end - s.start;
      return { ...s, start: s.end - Math.min(span * 0.35, 36 / Math.max(1, s.radius)) };
    };
    /** Where a slab's pin may stand: by its value end, or back along the slab on the same side of
     * the ring (up to a fifth of a turn), so a crowded label can slide and keep a straight stem. */
    const pinOf = (i: number) => {
      const s = segments[i];
      if (s.end - s.start <= 1e-3) return null;
      const side = Math.cos(s.end - Math.min(0.08, (s.end - s.start) / 2)) > 0 ? "up" : "down";
      let start = s.end;
      while (
        start - 0.02 > Math.max(s.start, s.end - 1.25) &&
        Math.cos(start - 0.02) > 0 === (side === "up")
      )
        start -= 0.02;
      return slabPin(frame.at, { ...s, start }, { side, along: 1 });
    };

    // The tooltip beside the focused slab's end.
    let tipRect: Rect | null = null;
    const card = tip.current;
    if (card && active !== null && tooltip && segments[active] && frame.camera > 0.02) {
      const s = endOf(active);
      const corners = [s.start, (s.start + s.end) / 2, s.end].flatMap((a) => [
        frame.at(a, s.radius - s.width / 2, 0),
        frame.at(a, s.radius + s.width / 2, 0),
        frame.at(a, s.radius + s.width / 2, s.height),
        frame.at(a, s.radius - s.width / 2, s.height),
      ]);
      placeTooltip(
        card,
        {
          left: Math.min(...corners.map((q) => q.x)),
          right: Math.max(...corners.map((q) => q.x)),
          top: Math.min(...corners.map((q) => q.y)),
          bottom: Math.max(...corners.map((q) => q.y)),
        },
        { left: 4, top: 30, right: width - 4, bottom: h - 4 },
      );
      const placed = /translate\(([-\d.]+)px, ([-\d.]+)px\)/.exec(card.style.transform);
      if (placed) {
        const left = Number(placed[1]),
          top = Number(placed[2]);
        tipRect = { left, top, right: left + card.offsetWidth, bottom: top + card.offsetHeight };
      }
    }

    const extent = slabExtent(frame.at, segments);
    const requests: PinRequest[] = segments.map((_s, i) => {
      const anchor = pinOf(i);
      const size = measured.size(`tl-${data[i].key}`);
      return {
        id: data[i].key,
        side: anchor?.side ?? "up",
        x: anchor?.x ?? 0,
        y: anchor?.y ?? 0,
        ridge: anchor?.ridge,
        width: size.width,
        height: size.height,
        priority: anchor ? (active === i ? Infinity : 1 + values[i] / (top || 1)) : 0,
      };
    });
    const placed = layoutPinLabels(requests, {
      // Clear of the height note and the view switch along the top.
      bounds: { left: 4, top: 30, right: width - 4, bottom: h - 2 },
      ceiling: extent.ceiling,
      floor: extent.floor,
      // A narrow card keeps the slab in focus and the largest few.
      limit: width < 520 ? 3 : Infinity,
    });
    placed.forEach((spot, i) => {
      const size = requests[i];
      const box = {
        left: spot.left,
        top: spot.top,
        right: spot.left + size.width,
        bottom: spot.top + size.height,
      };
      const visible = spot.visible && shown > 0 && !(tipRect && overlaps(box, tipRect, 4));
      put(measured.element(`tl-${data[i].key}`), spot.left, spot.top, visible);
      const stem = tiltLeaders.current[i];
      if (stem) {
        stem.setAttribute("points", pinPoints(spot.points));
        stem.dataset.shown = String(visible);
      }
    });
  }

  /** Names, values, target notches and the tooltip, from the geometry just drawn. */
  function place(
    g: RadialBarsGeometry,
    width: number,
    h: number,
    glass: TrackGlass[],
    ends: number[],
    fills: number[],
    engine: "optical" | "svg",
    flatShown: boolean,
  ) {
    const bounds: Rect = { left: 2, top: 2, right: width - 2, bottom: h - 2 };
    const obstacles: Rect[] = [];

    // Names at the start of each track.
    if (!semi) {
      const sizes = byPosition.map((i) => {
        const s = measured.size(`n-${data[i].key}`);
        return active === i ? { width: s.width * LIFT, height: s.height * LIFT } : s;
      });
      startLabelBoxes(g, sizes, 10).forEach((rect, p) => {
        const i = byPosition[p];
        const element = measured.element(`n-${data[i].key}`);
        const natural = measured.size(`n-${data[i].key}`);
        // The lifted box grows from its right end, nearest the track.
        put(
          element,
          rect.right - natural.width,
          rect.top + (rect.bottom - rect.top - natural.height) / 2,
          flatShown,
        );
        obstacles.push(rect);
      });
    }

    // Scale figures round the rim are obstacles too, with a few px of air so a value never butts
    // up against one.
    if (showScale)
      ticks.forEach((value) => {
        const at = polar(g.cx, g.cy, g.outer + 13, barAngle(value, top, g.from, g.span));
        const w = textWidth(formatTick(value), TYPE.micro);
        obstacles.push({
          left: at.x - w / 2 - 5,
          top: at.y - 10,
          right: at.x + w / 2 + 5,
          bottom: at.y + 10,
        });
      });

    // Target notches: in the etched track until the bar reaches them, then in the glass.
    data.forEach((_d, i) => {
      const target = targets[i];
      if (target === undefined) return;
      const at = polar(g.cx, g.cy, g.radii[positions[i]], barAngle(target, top, g.from, g.span));
      const half = g.thickness / 2;
      obstacles.push({
        left: at.x - half,
        top: at.y - half,
        right: at.x + half,
        bottom: at.y + half,
      });
    });
    data.forEach((_d, i) => {
      const line = notches.current[i];
      const target = targets[i];
      if (!line || target === undefined) return;
      const p = positions[i];
      const angle = barAngle(target, top, g.from, g.span);
      const r = g.radii[p],
        w = g.thickness;
      const a = polar(g.cx, g.cy, r - w / 2 + 2.5, angle),
        b = polar(g.cx, g.cy, r + w / 2 - 2.5, angle);
      line.setAttribute("x1", String(a.x));
      line.setAttribute("y1", String(a.y));
      line.setAttribute("x2", String(b.x));
      line.setAttribute("y2", String(b.y));
      const reached = ends[i] >= angle - 1e-4;
      const inGlass = engine === "optical" && reached;
      line.dataset.shown = String(!inGlass && fills[i] > 0.6 && flatShown);
      line.dataset.reached = String(reached && fills[i] > 0.98);
    });

    // The tooltip, out past the rim beside the focused bar's end.
    let tipRect: Rect | null = null;
    const element = tip.current;
    if (element && active !== null && tooltip && data[active] && flatShown) {
      const angle = Math.min(Math.max(ends[active], g.from + 0.15), g.from + g.span - 0.05);
      placeTooltip(
        element,
        { left: 0, top: 0, right: 0, bottom: 0 },
        { left: 4, top: 4, right: width - 4, bottom: h - 4 },
        { prefer: "outward", ring: { cx: g.cx, cy: g.cy, outer: g.outer + 2, angle } },
      );
      const placed = /translate\(([-\d.]+)px, ([-\d.]+)px\)/.exec(element.style.transform);
      if (placed) {
        const left = Number(placed[1]),
          y = Number(placed[2]);
        tipRect = {
          left,
          top: y,
          right: left + element.offsetWidth,
          bottom: y + element.offsetHeight,
        };
      }
    }

    // Values at the bars' ends, sliding along their tracks to clear each other.
    const requests: ArcLabelRequest[] = data.map((d, i) => {
      const p = positions[i];
      const size = measured.size(`v-${d.key}`);
      const lifted = active === i;
      let priority = 0;
      if (lifted) priority = Infinity;
      else if (labels === "all" || labels === "auto") priority = 1 + values[i] / (top || 1);
      else if (labels === "ends")
        priority = p === 0 || p === data.length - 1 ? 1 + values[i] / (top || 1) : 0;
      if (fills[i] < 0.92 || values[i] <= 0) priority = 0;
      return {
        id: d.key,
        track: p,
        radius: g.radii[p],
        angle: glass[p].end,
        limit: g.from + g.span + (semi ? 0.6 : 0.45),
        width: size.width * (lifted ? LIFT : 1),
        height: size.height * (lifted ? LIFT : 1),
        priority,
      };
    });
    const spots = placeArcLabels(requests, { cx: g.cx, cy: g.cy, glass, bounds, obstacles });
    spots.forEach((spot, i) => {
      const key = data[i].key;
      const natural = measured.size(`v-${key}`);
      const lifted = active === i;
      const dx = lifted ? (natural.width * (LIFT - 1)) / 2 : 0,
        dy = lifted ? (natural.height * (LIFT - 1)) / 2 : 0;
      const rect = {
        left: spot.left,
        top: spot.top,
        right: spot.left + requests[i].width,
        bottom: spot.top + requests[i].height,
      };
      const hidden = Boolean(tipRect && overlaps(rect, tipRect, 4));
      put(
        measured.element(`v-${key}`),
        spot.left + dx,
        spot.top + dy,
        spot.visible && !hidden && flatShown,
      );
    });
  }

  const pickAt = (x: number, y: number, size: { width: number; height: number }) => {
    if (status) return null;
    if (handed && ringFrame.current && ringFrame.current.camera > 0.02) {
      const hit = ringPick(x, y);
      return hit === null ? null : (positions[hit] ?? null);
    }
    const g = geometryFor(size.width, size.height);
    if (semi && y > g.cy + 8) return null;
    return trackAt(g, x, y);
  };
  const ratio = (i: number) => {
    const target = targets[i];
    return target !== undefined && target > 0 ? values[i] / target : null;
  };
  const secondMeasure = heightMeaning === "second-measure" && measures.some(finite);
  const speak =
    describe ??
    ((i: number) => {
      const target = targets[i];
      const share = ratio(i);
      return (
        `${names[i]}: ${plain(values[i], format)}` +
        (target !== undefined
          ? ` against a ${targetLabel.toLowerCase()} of ${plain(target, format)}, ${plain(share ?? 0, { kind: "percent" })} of ${targetLabel.toLowerCase()}.`
          : ".") +
        (secondMeasure && measures[i] !== undefined
          ? ` ${measureLabel ?? "Height"} ${plain(measures[i], measureFormat)}.`
          : "")
      );
    });
  const frameActive = active === null ? null : (positions[active] ?? null);
  const focus = active !== null && data[active] ? active : null;

  const tooltipRows: TooltipRow[] =
    focus === null
      ? []
      : [
          {
            label: "Value",
            color: colours[focus],
            value: <Figure value={values[focus]} {...format} className={roles.figureS} />,
          },
          ...(targets[focus] !== undefined
            ? [
                {
                  label: targetLabel,
                  color: colours[focus],
                  muted: true,
                  value: (
                    <Figure
                      value={targets[focus]!}
                      {...format}
                      className={`${roles.figureS} ${mutedText}`}
                    />
                  ),
                },
              ]
            : []),
        ];

  return (
    <ChartScope theme={theme} className={`${styles.chart} ${className ?? ""}`}>
      <ChartFrame
        credit={<LiquidCredit />}
        theme={theme}
        height={plotHeight}
        insets={{ left: 0, right: 0, top: 0, bottom: 0 }}
        count={status ? 0 : data.length}
        active={status ? null : frameActive}
        onActiveChange={(position) =>
          setActive(position === null ? null : (byPosition[position] ?? null))
        }
        plotRef={box}
        pick={pickAt}
        label={`${label}. Use the arrow keys to read each track.`}
        describe={(position) => speak(byPosition[position])}
        table={{
          caption: label,
          columns: [
            "Track",
            "Value",
            ...(targets.some(finite) ? [targetLabel, `Of ${targetLabel.toLowerCase()}`] : []),
            ...(secondMeasure
              ? [
                  measureLabel && measureLabel !== `Of ${targetLabel.toLowerCase()}`
                    ? measureLabel
                    : `${measureLabel ?? "Measure"} (height)`,
                ]
              : []),
          ],
          rows: data.map((_d, i) => [
            names[i],
            plain(values[i], format),
            ...(targets.some(finite)
              ? [
                  targets[i] === undefined ? "none" : plain(targets[i], format),
                  ratio(i) === null ? "none" : plain(ratio(i), { kind: "percent" }),
                ]
              : []),
            ...(secondMeasure ? [plain(measures[i], measureFormat)] : []),
          ]),
        }}
      >
        {!status && (
          <Tracks
            geometryFor={geometryFor}
            colours={colours}
            positions={positions}
            active={active}
            ticks={showScale ? ticks : []}
            top={top}
            formatTick={formatTick}
            hidden={tilted || handed}
          />
        )}
        <div ref={ringBox} className={styles.layer}>
          <canvas ref={ringCanvas} className={styles.canvas} aria-hidden="true" />
        </div>
        <canvas ref={canvas} className={styles.canvas} aria-hidden="true" />
        <svg ref={svg} className={styles.layer} aria-hidden="true" />
        <canvas ref={lights} className={styles.lights} aria-hidden="true" />
        {!status && (
          <svg className={styles.layer} aria-hidden="true">
            {data.map((d, i) =>
              targets[i] === undefined ? null : (
                <line
                  key={d.key}
                  ref={(element) => {
                    notches.current[i] = element;
                  }}
                  className={styles.notch}
                  style={{ "--track": colours[i] } as CSSProperties}
                  data-shown="false"
                />
              ),
            )}
          </svg>
        )}
        {status?.kind === "loading" && (
          <ChartState
            kind="loading"
            shape="ring"
            ring={semi ? { inner: 0.4, from: -Math.PI / 2, span: Math.PI } : { inner: 0.3 }}
          />
        )}
        {status && status.kind !== "loading" && (
          <ChartState
            kind={status.kind}
            message={status.kind === "error" ? stateMessage(error) : stateMessage(empty)}
          />
        )}
        {!status &&
          !semi &&
          data.map((d, i) => (
            <div
              key={`n-${d.key}`}
              ref={measured.ref(`n-${d.key}`)}
              className={`${styles.placed} ${styles.name}`}
              data-active={active === i}
              data-dim={active !== null && active !== i}
              data-figure-active={active === i}
              aria-hidden="true"
            >
              <span
                className={`${styles.nameRow} ${roles.lift}`}
                style={{ transformOrigin: "100% 50%" }}
              >
                <span className={tile ? roles.micro : roles.label}>{labelNode(config, d.key)}</span>
              </span>
            </div>
          ))}
        {!status &&
          data.map((d, i) => (
            <div
              key={`v-${d.key}`}
              ref={measured.ref(`v-${d.key}`)}
              className={`${styles.placed} ${styles.value}`}
              data-figure-active={active === i}
              data-dim={active !== null && active !== i}
              data-shown="false"
              aria-hidden="true"
            >
              <span
                className={roles.lift}
                style={{ display: "inline-flex", transformOrigin: "50% 50%" }}
              >
                <Figure
                  value={values[i]}
                  {...format}
                  className={small ? roles.micro : roles.figureS}
                />
              </span>
            </div>
          ))}
        {!status && canTilt && (
          <div ref={tiltLayer} className={styles.layer} style={{ opacity: 0 }} aria-hidden="true">
            <svg className={pins.stems} aria-hidden="true">
              {data.map((d, i) => (
                <polyline
                  key={d.key}
                  ref={(element) => {
                    tiltLeaders.current[i] = element;
                  }}
                  data-active={active === i}
                  data-dim={active !== null && active !== i}
                  data-shown="false"
                />
              ))}
            </svg>
            {data.map((d, i) => (
              <div
                key={`tl-${d.key}`}
                ref={measured.ref(`tl-${d.key}`)}
                className={pins.label}
                data-active={active === i}
                data-dim={active !== null && active !== i}
                data-shown="false"
              >
                <Figure value={values[i]} {...format} className={roles.figureS} />
                <span className={`${pins.name} ${roles.caption}`}>{labelNode(config, d.key)}</span>
                {secondMeasure && measures[i] !== undefined && (
                  <Figure
                    value={measures[i]!}
                    {...measureFormat}
                    className={`${pins.measure} ${roles.micro}`}
                  />
                )}
              </div>
            ))}
          </div>
        )}
        {!status && canTilt && (
          <HeightNote
            ref={noteRef}
            meaning={heightMeaning === "second-measure" ? "second-measure" : "value"}
            measureLabel={measureLabel}
            shown={tilted}
          />
        )}
        {!status && tooltip && focus !== null && tilted === handed && (
          <ChartTooltip
            ref={tip}
            title={labelNode(config, data[focus].key)}
            rows={tooltipRows}
            footer={
              ratio(focus) !== null ? (
                <TooltipFoot>
                  <Figure value={ratio(focus)!} kind="percent" /> of {targetLabel.toLowerCase()}
                </TooltipFoot>
              ) : undefined
            }
          />
        )}
      </ChartFrame>
      {canTilt && viewSwitch && !status && (
        <ViewSwitch
          available
          view={view}
          onChange={(next) => {
            setLocalView(next);
            onViewChange?.(next);
          }}
          label={`${label}: view`}
          className={styles.switch}
        />
      )}
      {semi && !status && (
        <ul className={styles.names} aria-hidden="true">
          {byPosition.map((i) => (
            <li
              key={data[i].key}
              className={styles.nameItem}
              data-active={active === i}
              data-dim={active !== null && active !== i}
              onPointerEnter={() => setActive(i)}
              onPointerLeave={() => setActive(null)}
            >
              <span className={styles.swatch} style={{ background: colours[i] }} />
              <span className={tile ? roles.micro : roles.caption}>
                {labelNode(config, data[i].key)}
              </span>
            </li>
          ))}
        </ul>
      )}
    </ChartScope>
  );
}

/** The etched tracks under the glass, and the scale: spokes at round values and their figures
 * round the rim. Flat and quiet; redrawn only when the plot resizes or focus moves. */
function Tracks({
  geometryFor,
  colours,
  positions,
  active,
  ticks,
  top,
  formatTick,
  hidden = false,
}: {
  geometryFor: (width: number, height: number) => RadialBarsGeometry;
  colours: readonly string[];
  positions: readonly number[];
  active: number | null;
  ticks: readonly number[];
  top: number;
  formatTick: (value: number) => string;
  /** Faded out while the tracks stand up (they stay flat on the card). */
  hidden?: boolean;
}) {
  const { size } = useChartFrame();
  if (!size.width) return null;
  const g = geometryFor(size.width, size.height);
  const w = g.thickness;
  return (
    <div className={styles.layer} style={{ opacity: hidden ? 0 : 1, transition: "opacity 200ms" }}>
      <svg className={styles.layer} aria-hidden="true">
        {ticks.map((value) => {
          const angle = barAngle(value, top, g.from, g.span);
          const a = polar(g.cx, g.cy, Math.max(0, g.inner - 4), angle),
            b = polar(g.cx, g.cy, g.outer + 4, angle);
          return (
            <line
              key={value}
              className={styles.axis}
              data-dashed={value !== 0}
              x1={a.x}
              y1={a.y}
              x2={b.x}
              y2={b.y}
            />
          );
        })}
        {positions.map((p, i) => (
          <RadialBarTrack
            key={i}
            active={active === i}
            colour={colours[i]}
            sector={{
              cx: g.cx,
              cy: g.cy,
              inner: g.radii[p] - w / 2,
              outer: g.radii[p] + w / 2,
              start: g.from,
              end: g.from + g.span,
              corner: w / 2,
            }}
          />
        ))}
      </svg>
      {ticks.map((value) => {
        const at = polar(g.cx, g.cy, g.outer + 13, barAngle(value, top, g.from, g.span));
        return (
          <span
            key={value}
            className={`${styles.tick} ${roles.micro}`}
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
            {formatTick(value)}
          </span>
        );
      })}
    </div>
  );
}
