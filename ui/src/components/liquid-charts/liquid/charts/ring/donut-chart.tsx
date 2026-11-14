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
  ChartScope,
  ChartState,
  ChartTooltip,
  cssToRgb,
  Delta,
  Figure,
  HeightNote,
  Legend,
  LIFT,
  moveTooltip,
  mutedText,
  placeTooltip,
  resolveStyle,
  seriesColour,
  useActive,
  useMaterialNote,
  useMaterials,
  useValueLabels,
  ValueLabel,
  ViewSwitch,
  type ChartConfig,
  type Density,
  type HeightMeaning,
  type LabelMode,
  type Rect,
  type Rgb,
  type Theme,
  type TooltipRow,
  type View,
} from "@/components/liquid-charts/liquid/core";
import {
  arcSpan,
  fitRing,
  insideLabelFits,
  layoutOutsideLabels,
  polar,
  ringAngles,
  clearRingTooltipPosition,
  ringTooltipPosition,
  TAU,
  tiltHeights,
  wrapAngle,
  type Arc,
  donutGeometry,
  type RingFit,
} from "./ring-geometry";
import {
  layoutPinLabels,
  pinPoints,
  slabExtent,
  slabPin,
  type PinRequest,
} from "@/components/liquid-charts/liquid/rings";
import { DonutInteractionFrame, donutMarkFocus, donutVisibleItems } from "./donut-frame-policy";
import { formatText, labelNode, labelText, useMeasured, type RingFormat } from "./ring-parts";
import { useRingMarks, type RingFrame } from "@/components/liquid-charts/liquid/rings/ring-marks";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { clamp01, overlaps } from "@/components/liquid-charts/liquid/core/geometry";
import Pins from "@/components/liquid-charts/liquid/rings/pin-labels.module.css";
import { usePageTheme } from "@/components/liquid-charts/liquid/core/page-theme";
import { cssModule } from "@/components/liquid-charts/liquid/core/css-module";
import Roles from "@/components/liquid-charts/liquid/core/type.module.css";
import Styles from "./ring.module.css";

const pins = cssModule(Pins, "lc-pin-labels-");
const roles = cssModule(Roles, "lc-type-");
const styles = cssModule(Styles, "lc-ring-");

export type DonutDatum = {
  /** The segment's key in `config`, which gives its label and colours. */
  key: string;
  value: number;
  /** A second measure (growth against last period, say) the tilt view can show as height. */
  measure?: number;
  /** How full the segment's glass is, 0–1 (default 1). 0 is an empty shell: what is left of a
   * budget, free space, an open seat. */
  level?: number;
};

export type DonutChartProps = {
  data: readonly DonutDatum[];
  config: ChartConfig;
  /** Light or dark. Without one, the page's: `data-theme` or a `dark` class on `<html>`, else the system setting. @default the page's */
  theme?: Theme;
  height?: number;
  /** `tile` is the mini donut; `hero` starts tilted where the style allows it. */
  density?: Density;
  /** A whole ring, or the upper half (a semi donut). */
  arc?: Arc;
  /** The hole as a share of the outer radius: 0 is a pie. */
  innerRadius?: number;
  /** Px between segments. */
  gap?: number;
  /** Corner rounding, px. */
  cornerRadius?: number;
  active?: number | null;
  onActiveChange?: (index: number | null) => void;
  /** Reports the resolved share-label decimals after layout/view changes: inside 0, outside or
   * actual tilt 1. Observation only; never changes the chart's responsive label policy. */
  onShareLabelDigitsChange?: (digits: 0 | 1) => void;
  labels?: LabelMode;
  /** Outside, in columns with leader lines; or inside, on the segments. */
  labelPlacement?: "outside" | "inside";
  tooltip?: boolean;
  format?: RingFormat;
  /** What the hole shows at rest: the total, a reading of your own, or nothing. The segment in
   * focus replaces it either way. */
  centre?: "total" | "none" | { value: number; format?: RingFormat };
  centreLabel?: ReactNode;
  /** A legend under the chart whose entries switch segments off. Default: shown (not on tiles),
   * and always once the card is too narrow for label columns, where the labels move onto the
   * segments. `false` never draws it (the page names the segments in its own legend row);
   * `"narrow"` draws it only once the labels move onto the segments. */
  legend?: boolean | "narrow";
  material?: "glass" | "flat";
  view?: View;
  onViewChange?: (view: View) => void;
  /** Show the Flat / Tilt switch when the view can tilt (default true). */
  viewSwitch?: boolean;
  /** What a tilted slab's height shows: the value again, or each datum's `measure`. */
  heightMeaning?: HeightMeaning;
  /** Names the measure, e.g. "Growth vs last quarter". */
  measureLabel?: string;
  measureFormat?: RingFormat;
  /** A dial of ticks inside the ring (default in hero density). */
  dial?: boolean;
  loading?: boolean;
  empty?: boolean | ReactNode;
  error?: boolean | ReactNode;
  replay?: number;
  label: string;
  describe?: (index: number) => string;
  className?: string;
};

const DIAL_TICKS = 72;
const ease = (t: number) => {
  const u = clamp01(t);
  return u * u * (3 - 2 * u);
};
const fade = (from: number, to: number, t: number) => ease((t - from) / (to - from));
const round = (value: number) => Math.round(value * 100) / 100;
const FALLBACK_TINT: Rgb = [0.62, 0.55, 0.92];

/**
 * Donut, pie and semi donut in edge-lit glass: segments padded and rounded, a centre reading of
 * the total (or the segment in focus), a segment that pops out on focus and pins on click, legend
 * toggles, outside labels with leaders that never cross (or labels on the segments), and a tooltip
 * with value and share. Where the style allows it, a Flat / Tilt switch shows the ring from the
 * side, its slabs extruded by the value or a second measure; every slab is labelled while tilted.
 * `density="tile"` is the mini donut. Without WebGL, or with `material="flat"`, it draws in SVG.
 */
export function DonutChart({
  data,
  config,
  theme: themeProp,
  height,
  density = "standard",
  arc = "full",
  innerRadius,
  gap,
  cornerRadius,
  active: activeProp,
  onActiveChange,
  onShareLabelDigitsChange,
  labels = "auto",
  labelPlacement = "outside",
  tooltip,
  format = { kind: "number" },
  centre,
  centreLabel = "Total",
  legend,
  material: materialProp,
  view: viewProp,
  onViewChange,
  viewSwitch = true,
  heightMeaning = "none",
  measureLabel,
  measureFormat = { kind: "percent", decimals: 1, signed: true },
  dial,
  loading = false,
  empty = false,
  error = false,
  replay = 0,
  label,
  describe,
  className,
}: DonutChartProps) {
  const theme = usePageTheme(themeProp);
  const tile = density === "tile";
  const hero = density === "hero";
  const plotHeight = height ?? (tile ? 112 : hero ? 380 : 300);
  const inner = innerRadius ?? (tile ? 0.68 : arc === "semi" ? 0.6 : 0.62);
  const pie = inner < 0.35;
  const padding = gap ?? (tile ? 3 : pie ? 4 : 6);
  const centreMode = centre ?? (pie ? "none" : "total");
  const showCentre = centreMode !== "none";
  const reading = typeof centreMode === "object" ? centreMode : null;
  // Too narrow for label columns: labels move onto the segments and a legend names them.
  const [narrow, setNarrow] = useState(false);
  const showLegend =
    legend === false
      ? false
      : legend === "narrow"
        ? narrow && !tile
        : (legend ?? !tile) || (narrow && !tile);
  const showTooltip = tooltip ?? !tile;
  const showDial = (dial ?? hero) && !pie && arc === "full";
  const placement = tile || narrow ? "inside" : labelPlacement;
  const { from, span } = arcSpan(arc);

  const [hidden, setHidden] = useState<ReadonlySet<string>>(() => new Set());
  const [active, setActive] = useActive(activeProp, onActiveChange);
  const [localView, setLocalView] = useState<View | null>(null);
  const [turnTo, setTurnTo] = useState<number | null>(null);
  const values = data.map((d) => (hidden.has(d.key) ? 0 : Math.max(0, d.value || 0)));
  const total = values.reduce((sum, value) => sum + value, 0);
  const visible = donutVisibleItems(values);
  const nothing = empty || (!loading && !error && total <= 0 && hidden.size === 0);
  const status = loading ? "loading" : error ? "error" : nothing ? "empty" : null;

  const material = materialProp ?? "glass";
  const tiltAsked = viewProp === "tilt";
  useMaterialNote(
    tiltAsked ? "tilt" : "glass",
    tiltAsked || materialProp === "glass",
    Boolean(useMaterials().ring),
  );
  const style = resolveStyle({
    geometry: "circular",
    marks: ["solid"],
    measure: "share",
    segments: visible.length,
    density,
    heightMeaning: arc === "full" ? heightMeaning : "none",
    // A semi donut reads its arc like a scale, so it never tilts.
    precise: arc === "semi",
    material,
    view: viewProp,
  });
  const {
    box,
    canvas,
    svg,
    available,
    pick: pickAt,
  } = useRingMarks({
    theme,
    replay,
    material: style.materials.solid === "glass" ? "glass" : "flat",
    layout: (width, h) => layout(width, h),
    onFrame: (frame) => place(frame),
  });
  const canTilt = style.tilt !== "never" && available && style.materials.solid === "glass";
  const view: View = canTilt ? (viewProp ?? localView ?? style.view) : "flat";
  const tilted = view === "tilt";
  // Angles can't be read in perspective: tilted, every segment is labelled.
  const mode: LabelMode = tilted ? "all" : labels;
  const shareDigits = tilted || placement === "outside" ? 1 : 0;
  useEffect(() => {
    onShareLabelDigitsChange?.(shareDigits);
  }, [onShareLabelDigitsChange, shareDigits]);

  useEffect(() => {
    if (process.env.NODE_ENV === "production") return;
    for (const warning of style.warnings) console.warn(`[DonutChart] ${warning.message}`);
    // Warn once per distinct set of warnings.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [style.warnings.map((w) => w.rule).join()]);

  const measured = useMeasured();
  const insideLabels = useValueLabels();
  const tiltLeaders = useRef<(SVGPolylineElement | null)[]>([]);
  /** Where the tilted ring is turning to, so its pins can wait out a turn. */
  const turnGoal = useRef(0);
  const tip = useRef<HTMLDivElement>(null);
  const centreRef = useRef<HTMLDivElement>(null);
  /** Whether the flat tooltip sits in the ring's hole (no room beside the ring). */
  const tipInHole = useRef(false);
  const noteRef = useRef<HTMLDivElement>(null);
  const leaders = useRef<(SVGPolylineElement | null)[]>([]);
  const ticks = useRef<(SVGLineElement | null)[]>([]);
  const outsideLayer = useRef<HTMLDivElement>(null);
  const tiltLayer = useRef<HTMLDivElement>(null);

  const colours = data.map((d) => seriesColour(config, d.key, theme));
  const tints = data.map((_d, i) => cssToRgb(colours[i]) ?? FALLBACK_TINT);
  const heights = useMemo(() => {
    if (heightMeaning === "second-measure") return data.map((d) => d.measure ?? 0);
    if (heightMeaning === "value") return values;
    return null;
    // values derive from data and hidden.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data, hidden, heightMeaning]);

  /** Px each side reserved for outside labels: the widest label plus its leader. */
  function sideRoom(width: number) {
    if (placement !== "outside" || mode === "none" || tile) return 0;
    return outsideRoom(width);
  }
  function outsideRoom(width: number) {
    let widest = 0;
    data.forEach((d) => (widest = Math.max(widest, measured.size(`o-${d.key}`).width)));
    if (!widest) widest = 84;
    // Room for the widest label lifted, so a label in focus still fits its column.
    return Math.min(width * 0.38, widest * LIFT + 16 + 12 + 4);
  }
  const geometryFor = (width: number, h: number) =>
    donutGeometry(width, h, { arc, inner, side: sideRoom(width), density, gap: padding });

  function angles(fit: RingFit) {
    return ringAngles(values, {
      from,
      span,
      minAngle: (padding + 4) / Math.max(1, fit.radius),
    });
  }

  function layout(width: number, h: number) {
    if (labelPlacement === "outside" && !tile && labels !== "none") {
      // Columns would shrink the ring to less than about half its size: go inside instead.
      const room = outsideRoom(width);
      const open = fitRing(width, h, { arc }).outer;
      const squeezed = fitRing(width, h, { arc, side: room }).outer;
      const next = squeezed < Math.max(70, open * 0.55);
      if (next !== narrow) queueMicrotask(() => setNarrow(next));
    }
    const g = geometryFor(width, h);
    const a = angles(g.fit);
    const tallHeights = heights ? tiltHeights(heights, g.tilt.low, g.tilt.high) : null;
    const uniform = round((g.tilt.low + g.tilt.high) / 2.4);
    // Tilted, the ring turns its tallest slab (or the one tapped) to the back.
    let back = turnTo;
    if (back === null || values[back] <= 0) {
      back = visible[0] ?? 0;
      if (tallHeights) for (const i of visible) if (tallHeights[i] > tallHeights[back]) back = i;
    }
    turnGoal.current = -(a[back]?.mid ?? 0);
    return {
      ring: {
        cx: g.fit.cx,
        cy: g.fit.cy,
        radius: g.fit.radius,
        width: g.fit.ringWidth,
        gap: padding,
        bevel: cornerRadius ?? Math.min(6, g.fit.ringWidth * 0.12),
      },
      tilt: {
        ...g.tilt,
        bevel: cornerRadius !== undefined ? Math.min(cornerRadius, 5) : undefined,
      },
      view,
      turn: -(a[back]?.mid ?? 0),
      sweep: { from, span },
      marks: (status ? [] : data).map((d, i) => ({
        key: d.key,
        start: a[i].start,
        end: a[i].end,
        tint: tints[i],
        ...donutMarkFocus(active, i, { tilted, tile }),
        height: tallHeights ? tallHeights[i] : uniform,
        level: d.level,
      })),
    };
  }

  /** Everything that rides the ring, placed from the frame the glass just drew. */
  function place(frame: RingFrame) {
    if (status || frame.segments.length !== data.length) return;
    const { width, height: h, ring, segments, camera, phase } = frame;
    const g = geometryFor(width, h);
    const outer = ring.radius + ring.width / 2;
    const bounds: Rect = { left: 0, top: 0, right: width, bottom: h };
    const flatShown = 1 - fade(0, 0.35, camera);
    const tiltShown = fade(0.6, 0.92, phase);

    // The centre reading follows the floor's centre, and fades as the camera rises.
    const centreEl = centreRef.current;
    if (centreEl) {
      const at = frame.project(ring.cx, arc === "semi" ? ring.cy - 2 : ring.cy);
      centreEl.style.left = `${at.x}px`;
      centreEl.style.top = `${at.y}px`;
      centreEl.dataset.placed = "";
    }
    if (noteRef.current) noteRef.current.style.opacity = String(round(tiltShown));

    // The dial lies on the floor inside the ring, carried by the camera.
    if (showDial) {
      const dialRadius = Math.max(8, ring.radius - ring.width / 2 - (tile ? 10 : 18));
      ticks.current.forEach((line, i) => {
        if (!line) return;
        const angle = (i / DIAL_TICKS) * TAU;
        const a = frame.at(angle, dialRadius - 3),
          b = frame.at(angle, dialRadius + 3);
        line.setAttribute("x1", String(a.x));
        line.setAttribute("y1", String(a.y));
        line.setAttribute("x2", String(b.x));
        line.setAttribute("y2", String(b.y));
      });
    }

    // Outside labels with leaders, flat view only.
    const tipRect = placeTip(frame, outer, bounds);
    // The centre reading fades as the camera rises, and gives the hole to the tooltip while the
    // tooltip sits in it (crossfading, by its CSS transition, as the card fades in and out).
    if (centreEl) {
      const yielded = tipRect !== null && tipInHole.current;
      centreEl.style.opacity = String(yielded ? 0 : round(1 - fade(0.05, 0.4, camera)));
      centreEl.dataset.yielded = String(yielded);
    }
    if (narrow && outsideLayer.current) outsideLayer.current.style.opacity = "0";
    if (placement === "outside" && !tile) {
      const requests = data.map((d, i) => {
        const size = measured.size(`o-${d.key}`);
        const segment = segments[i];
        const share = total > 0 ? values[i] / total : 0;
        const focus = active === i;
        let priority = 0;
        if (values[i] > 0 && mode !== "none") {
          if (focus) priority = Infinity;
          else if (mode === "all") priority = 1 + share;
          else if (mode === "auto") priority = share >= 0.025 ? 1 + share : 0;
          else if (mode === "ends")
            priority = i === visible[0] || i === visible[visible.length - 1] ? 1 : 0;
        }
        return {
          id: d.key,
          angle: (segment.start + segment.end) / 2,
          width: size.width * (focus ? LIFT : 1),
          height: size.height * (focus ? LIFT : 1),
          priority,
        };
      });
      const placed = layoutOutsideLabels(requests, {
        cx: ring.cx,
        cy: ring.cy,
        radius: outer + 3,
        bounds: {
          left: 0,
          top: 0,
          right: width,
          bottom: arc === "semi" ? ring.cy + 4 : h,
        },
        offset: g.label.offset,
        reach: g.label.reach,
      });
      placed.forEach((spot, i) => {
        const element = measured.element(`o-${data[i].key}`);
        const line = leaders.current[i];
        const size = requests[i];
        const box = {
          left: spot.left,
          top: spot.top,
          right: spot.left + size.width,
          bottom: spot.top + size.height,
        };
        const shown = spot.visible && !(tipRect && overlaps(box, tipRect, 4));
        // The solver placed the lifted box; the element keeps its natural box and grows from the
        // end nearest its leader, so it sits flush with the column on either side.
        const natural = measured.size(`o-${data[i].key}`);
        const dx = spot.side === "left" ? size.width - natural.width : 0;
        const dy = (size.height - natural.height) * 0.7;
        if (element) {
          element.style.transform = `translate(${round(spot.left + dx)}px, ${round(spot.top + dy)}px)`;
          element.dataset.side = spot.side;
          element.dataset.shown = String(shown);
          element.style.visibility = "visible";
        }
        if (line) {
          line.setAttribute("points", spot.points.map((p) => `${p.x},${p.y}`).join(" "));
          line.dataset.shown = String(shown);
        }
      });
      if (outsideLayer.current) outsideLayer.current.style.opacity = String(round(flatShown));
    }

    // Labels on the segments.
    if (placement === "inside") {
      const labelRadius = pie ? (ring.radius + ring.width / 2) * 0.64 : ring.radius;
      insideLabels.place(
        data.map((d, i) => {
          const segment = segments[i];
          const mid = (segment.start + segment.end) / 2;
          const shift = segment.pop * 0.3;
          const at = polar(ring.cx, ring.cy, labelRadius + shift, mid);
          const size = measured.size(`i-${d.key}`);
          const fits = insideLabelFits(
            segment.end - segment.start,
            labelRadius,
            pie ? ring.width * 0.6 : ring.width,
            size.width,
            size.height,
            padding,
          );
          const show =
            values[i] > 0 && mode !== "none" && fits && segment.fill > 0.98 && camera < 0.02;
          return {
            id: `i-${d.key}`,
            x: at.x,
            y: at.y,
            side: "center" as const,
            priority: show ? (active === i ? Infinity : 1 + values[i] / (total || 1)) : 0,
          };
        }),
        { bounds },
      );
    }

    // Tilted, every slab names its value on a pin: back slabs in a row above the ring, front slabs
    // in a row below it, each on a vertical stem.
    if (canTilt) placeTilted(frame, tiltShown, tipRect);
  }

  function placeTilted(frame: RingFrame, shown: number, tipRect: Rect | null) {
    const { width, height: h, segments } = frame;
    // Each slab on a pin: back slabs in a row above the ring, front slabs in a row below it, every
    // stem vertical. A turn changes which slabs are at the back, so the pins wait it out.
    const settled = Math.abs(wrapAngle(frame.turn - turnGoal.current)) < 0.03;
    const live = segments.map((segment, i) =>
      values[i] > 0 ? segment : { ...segment, end: segment.start },
    );
    const extent = slabExtent(frame.at, live);
    const requests: PinRequest[] = data.map((d, i) => {
      const anchor = slabPin(frame.at, live[i]);
      const size = measured.size(`t-${d.key}`);
      return {
        id: d.key,
        side: anchor?.side ?? "up",
        x: anchor?.x ?? 0,
        y: anchor?.y ?? 0,
        ridge: anchor?.ridge,
        width: size.width,
        height: size.height,
        priority: anchor ? (active === i ? Infinity : 1 + values[i] / (total || 1)) : 0,
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
      const element = measured.element(`t-${data[i].key}`);
      const line = tiltLeaders.current[i];
      const size = requests[i];
      const box = {
        left: spot.left,
        top: spot.top,
        right: spot.left + size.width,
        bottom: spot.top + size.height,
      };
      const visible =
        spot.visible && settled && shown > 0 && !(tipRect && overlaps(box, tipRect, 4));
      if (element) {
        element.style.transform = `translate(${round(spot.left)}px, ${round(spot.top)}px)`;
        element.dataset.shown = String(visible);
        element.style.visibility = "visible";
      }
      if (line) {
        line.setAttribute("points", pinPoints(spot.points));
        line.dataset.shown = String(visible);
      }
    });
    if (tiltLayer.current) tiltLayer.current.style.opacity = String(round(shown));
  }

  /** The tooltip beside the segment in focus: out past the ring flat, beside the slab tilted. */
  function placeTip(frame: RingFrame, outer: number, bounds: Rect): Rect | null {
    tipInHole.current = false;
    const element = tip.current;
    if (!element || active === null || !showTooltip) return null;
    const segment = frame.segments[active];
    if (!segment) return null;
    const mid = (segment.start + segment.end) / 2;
    const inside = { left: 4, top: 4, right: bounds.right - 4, bottom: bounds.bottom - 4 };
    if (frame.camera > 0.02) {
      const corners = [segment.start, mid, segment.end].flatMap((a) => [
        frame.at(a, segment.radius - segment.width / 2, 0),
        frame.at(a, segment.radius + segment.width / 2, 0),
        frame.at(a, segment.radius + segment.width / 2, segment.height),
        frame.at(a, segment.radius - segment.width / 2, segment.height),
      ]);
      const anchor = {
        left: Math.min(...corners.map((p) => p.x)),
        right: Math.max(...corners.map((p) => p.x)),
        top: Math.min(...corners.map((p) => p.y)),
        bottom: Math.max(...corners.map((p) => p.y)),
      };
      placeTooltip(element, anchor, inside);
      const placed = /translate\(([-\d.]+)px, ([-\d.]+)px\)/.exec(element.style.transform);
      if (!placed) return null;
      const left = Number(placed[1]),
        top = Number(placed[2]);
      return { left, top, right: left + element.offsetWidth, bottom: top + element.offsetHeight };
    }
    const shift = segment.pop * 0.3;
    const size = { width: element.offsetWidth, height: element.offsetHeight };
    // Never over the ring: beside the segment's outer edge, else the nearest free spot close to
    // it. A ring that fills its card has none: then the whole card (every row) sits in the hole,
    // standing in for the centre reading while a segment is in focus. Only a hole too small for
    // the card (a small ring, or a pie) falls back to out past the segment, covering as little of
    // the ring as it can.
    const clear = clearRingTooltipPosition(
      size,
      { cx: frame.ring.cx, cy: frame.ring.cy, outer: outer + shift + 2 },
      mid,
      inside,
      { gap: 14, reach: outer * 0.75 },
    );
    const hole = frame.ring.radius - frame.ring.width / 2;
    const fitsHole = Math.hypot(size.width / 2, size.height / 2) <= hole - 6;
    tipInHole.current = !clear && fitsHole;
    const at =
      clear ??
      (fitsHole
        ? { left: frame.ring.cx - size.width / 2, top: frame.ring.cy - size.height / 2 }
        : ringTooltipPosition(
            size,
            {
              cx: frame.ring.cx + Math.sin(mid) * shift,
              cy: frame.ring.cy - Math.cos(mid) * shift,
              outer: outer + 2,
            },
            mid,
            inside,
            14,
          ));
    moveTooltip(element, at.left, at.top);
    return {
      left: at.left,
      top: at.top,
      right: at.left + size.width,
      bottom: at.top + size.height,
    };
  }

  const item = active === null ? null : data[active];
  const share = (i: number) => (total > 0 ? values[i] / total : 0);
  const name = (i: number) => labelText(config, data[i].key);
  const speak =
    describe ??
    ((i: number) =>
      `${name(i)}: ${formatText(values[i], format)}, ${formatText(share(i), { kind: "percent", decimals: 1 })} of the total.` +
      (data[i].measure !== undefined && heightMeaning === "second-measure"
        ? ` ${measureLabel ?? "Change"} ${formatText(data[i].measure ?? 0, measureFormat)}.`
        : ""));
  const figureClass = tile
    ? roles.figureM
    : hero || arc === "semi"
      ? roles.figureXL
      : roles.figureL;

  function toggle(key: string) {
    setHidden((current) => {
      const next = new Set(current);
      if (next.has(key)) next.delete(key);
      else if (data.length - next.size > 1) next.add(key);
      return next;
    });
    setActive(null);
  }
  function switchView(next: View) {
    setLocalView(next);
    onViewChange?.(next);
    setTurnTo(null);
  }

  const tooltipRows: TooltipRow[] =
    item && active !== null
      ? [
          {
            label: "Value",
            color: colours[active],
            value: <Figure value={values[active]} {...format} className={roles.figureS} />,
          },
          {
            label: "Share",
            color: colours[active],
            muted: true,
            value: (
              <Figure
                value={share(active)}
                kind="percent"
                decimals={1}
                className={`${roles.figureS} ${mutedText}`}
              />
            ),
          },
        ]
      : [];

  return (
    <ChartScope theme={theme} className={className}>
      <div
        className={styles.chart}
        onClick={(event) => {
          // Tilted, a tap turns that slab to the back (and a second tap lets it go).
          if (!tilted || !box.current?.contains(event.target as Node)) return;
          const r = box.current.getBoundingClientRect();
          const index = pickAt(event.clientX - r.left, event.clientY - r.top);
          setTurnTo(index === null || index === turnTo ? null : index);
        }}
      >
        <DonutInteractionFrame
          theme={theme}
          height={plotHeight}
          insets={{ left: 0, right: 0, top: 0, bottom: 0 }}
          visible={visible}
          disabled={!!status}
          active={active}
          onActiveChange={setActive}
          plotRef={box}
          pick={pickAt}
          label={`${label}. Use the arrow keys to read each segment.`}
          describe={(index) => speak(visible[index])}
          table={{
            caption: label,
            columns: ["Segment", "Value", "Share"],
            rows: data.map((_d, i) => [
              name(i),
              formatText(values[i], format),
              formatText(share(i), { kind: "percent", decimals: 1 }),
            ]),
          }}
        >
          {showDial && !status && (
            <svg className={`${styles.layer} ${styles.dial}`} aria-hidden="true">
              {Array.from({ length: DIAL_TICKS }, (_, i) => (
                <line
                  key={i}
                  ref={(element) => {
                    ticks.current[i] = element;
                  }}
                />
              ))}
            </svg>
          )}
          <svg ref={svg} className={styles.layer} aria-hidden="true" />
          <canvas ref={canvas} className={styles.canvas} aria-hidden="true" />
          {status === "loading" && (
            <ChartState kind="loading" shape="ring" ring={{ inner: pie ? 0 : inner, from, span }} />
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
          {!status && labelPlacement === "outside" && !tile && (
            <div ref={outsideLayer}>
              <svg className={`${styles.layer} ${styles.leaders}`} aria-hidden="true">
                <g>
                  {data.map((d, i) => (
                    <polyline
                      key={d.key}
                      ref={(element) => {
                        leaders.current[i] = element;
                      }}
                      data-active={active === i}
                      data-shown="false"
                    />
                  ))}
                </g>
              </svg>
              {data.map((d, i) => (
                <div
                  key={d.key}
                  ref={measured.ref(`o-${d.key}`)}
                  className={styles.outside}
                  data-figure-active={active === i}
                  data-dim={active !== null && active !== i}
                  aria-hidden="true"
                >
                  <span className={`${styles.reading} ${roles.lift}`}>
                    <Figure
                      value={share(i)}
                      kind="percent"
                      decimals={1}
                      className={roles.figureS}
                    />
                    <span className={`${styles.name} ${roles.caption}`}>
                      {labelNode(config, d.key)}
                    </span>
                  </span>
                </div>
              ))}
            </div>
          )}
          {!status &&
            placement === "inside" &&
            data.map((d, i) => (
              <ValueLabel
                key={d.key}
                ref={(element) => {
                  insideLabels.ref(`i-${d.key}`)(element);
                  measured.ref(`i-${d.key}`)(element);
                }}
                active={active === i}
              >
                <span className={`${styles.inside} ${roles.lift}`}>
                  <Figure
                    value={share(i)}
                    kind="percent"
                    decimals={0}
                    className={tile ? roles.micro : roles.figureS}
                  />
                </span>
              </ValueLabel>
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
                  key={d.key}
                  ref={measured.ref(`t-${d.key}`)}
                  className={pins.label}
                  data-active={active === i}
                  data-dim={active !== null && active !== i}
                  data-shown="false"
                >
                  <Figure value={share(i)} kind="percent" decimals={1} className={roles.figureS} />
                  <span className={`${pins.name} ${roles.caption}`}>
                    {labelNode(config, d.key)}
                  </span>
                  {heightMeaning === "second-measure" && d.measure !== undefined && (
                    <Figure
                      value={d.measure}
                      {...measureFormat}
                      className={`${pins.measure} ${roles.micro}`}
                    />
                  )}
                </div>
              ))}
            </div>
          )}
          {!status && canTilt && heightMeaning !== "none" && (
            <HeightNote
              ref={noteRef}
              meaning={heightMeaning}
              measureLabel={measureLabel}
              shown={tilted}
            />
          )}
          {!status && showCentre && (
            <div
              ref={centreRef}
              className={styles.centre}
              data-anchor={arc === "semi" ? "bottom" : undefined}
            >
              <span className={`${styles.centreCaption} ${tile ? roles.micro : roles.caption}`}>
                {item ? labelNode(config, item.key) : centreLabel}
                {item && !tile && (
                  <Figure
                    value={share(active!)}
                    kind="percent"
                    decimals={1}
                    className={styles.centreShare}
                  />
                )}
              </span>
              {/* A mini donut reads in shares, so the segment in focus does too. */}
              <Figure
                value={item ? (tile ? share(active!) : values[active!]) : (reading?.value ?? total)}
                {...(item
                  ? tile
                    ? { kind: "percent" as const }
                    : format
                  : (reading?.format ?? format))}
                className={`${figureClass} ${styles.centreValue}`}
              />
            </div>
          )}
          {!status && showTooltip && item && active !== null && (
            <ChartTooltip
              ref={tip}
              title={labelNode(config, item.key)}
              rows={tooltipRows}
              footer={
                heightMeaning === "second-measure" && item.measure !== undefined ? (
                  <Delta value={item.measure}>
                    {formatText(item.measure, measureFormat)}
                    {measureLabel ? ` · ${measureLabel}` : ""}
                  </Delta>
                ) : undefined
              }
            />
          )}
        </DonutInteractionFrame>
        {canTilt && viewSwitch && !status && (
          <ViewSwitch
            available
            view={view}
            onChange={switchView}
            label={`${label}: view`}
            className={styles.switch}
          />
        )}
        {showLegend && !status && (
          <Legend
            items={data.map((d, i) => ({
              key: d.key,
              label: labelNode(config, d.key),
              color: colours[i],
            }))}
            hidden={hidden}
            onToggle={toggle}
          />
        )}
      </div>
    </ChartScope>
  );
}

/** The angle a donut's segment `index` sits at, for pages that point at one (for example, a
 * caption beside a mini donut). */
export function donutSegmentMid(values: readonly number[], index: number, arc: Arc = "full") {
  const { from, span } = arcSpan(arc);
  return wrapAngle(ringAngles(values, { from, span })[index]?.mid ?? from);
}
