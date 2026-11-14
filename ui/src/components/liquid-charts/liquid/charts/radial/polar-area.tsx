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
  overlaps,
  placeTooltip,
  resolveStyle,
  sectorPath,
  seriesColour,
  stateOf,
  textWidth,
  tickFormatter,
  TooltipFoot,
  trimmed,
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
  polar,
  polarRadius,
  roseGeometry,
  rimStep,
  roseTilt,
  sectorBounds,
  sliceAngles,
  sliceAt,
  type PolarScale,
} from "./layout";
import {
  layoutOutsideLabels,
  layoutPinLabels,
  pinPoints,
  slabExtent,
  slabPin,
  type PinRequest,
} from "@/components/liquid-charts/liquid/rings";
import { useRingMarks, type RingFrame, type RingMark } from "@/components/liquid-charts/liquid/rings/ring-marks";
import { plain, put, stateMessage, useMeasured, type ValueFormat } from "./radial-parts";
import { useRadialLayer, type GlassShape, type LayerClock } from "./use-radial-layer";
import { useEffect, useRef, useState, type ReactNode } from "react";
import Pins from "@/components/liquid-charts/liquid/rings/pin-labels.module.css";
import { usePageTheme } from "@/components/liquid-charts/liquid/core/page-theme";
import { cssModule } from "@/components/liquid-charts/liquid/core/css-module";
import Roles from "@/components/liquid-charts/liquid/core/type.module.css";
import { createRingGlass } from "@/components/liquid-charts/liquid/glass";
import { LiquidCredit } from "@/components/liquid-charts/liquid/credit";
import Styles from "./radial.module.css";
import { easeInOut } from "./motion";

const pins = cssModule(Pins, "lc-pin-labels-");
const roles = cssModule(Roles, "lc-type-");
const styles = cssModule(Styles, "lc-radial-");

export type PolarDatum = {
  /** The slice's name (an hour, a month). */
  label: string;
  value: number;
  /** A key in `config` for this slice's own colour; slices without one take `series`. */
  key?: string;
};

export type PolarAreaProps = {
  data: readonly PolarDatum[];
  config: ChartConfig;
  /** The config key that colours every slice without its own `key`. Defaults to the first. */
  series?: string;
  /** Light or dark. Without one, the page's: `data-theme` or a `dark` class on `<html>`, else the system setting. @default the page's */
  theme?: Theme;
  height?: number;
  density?: Density;
  /** `area` (the default, for amounts): a slice's area grows with its value, so its radius grows
   * with the square root. `radius`: the radius itself is linear (rates and scores, when asked). */
  scale?: PolarScale;
  /** The value at the rim. Defaults to a round number above the data. */
  max?: number;
  /** Where the first slice starts, radians clockwise from 12 o'clock. */
  from?: number;
  /** Px between slices. */
  gap?: number;
  active?: number | null;
  onActiveChange?: (index: number | null) => void;
  /** Leader labels outside: `auto` names the highest and lowest slices, `all` every slice that
   * fits, `ends` the first and last, `none` only the slice in focus. */
  labels?: LabelMode;
  /** Slice names round the rim: `auto` every one that fits (every third, sixth…), or none. */
  rim?: "auto" | "none";
  tooltip?: boolean;
  format?: ValueFormat;
  /** What the tooltip calls a slice's share of the total. */
  shareLabel?: string;
  material?: "glass" | "flat";
  /** Tilt shows equal slices whose height is the value: seasonality as a skyline. Up to 12
   * slices, glass only. */
  view?: View;
  onViewChange?: (view: View) => void;
  viewSwitch?: boolean;
  loading?: boolean;
  empty?: boolean | ReactNode;
  error?: boolean | ReactNode;
  replay?: number;
  label: string;
  describe?: (index: number) => string;
  className?: string;
};

const FALLBACK_TINT: Rgb = [0.62, 0.55, 0.92];
const mix = (a: number, b: number, t: number) => a + (b - a) * t;

/**
 * A polar area (rose) chart in edge-lit glass: equal slices round the circle (hours, months), each
 * reaching out by its value. Amounts are read by area, so a slice twice the value has twice the
 * glass, not four times: the radius grows with the square root (`scale="radius"` makes it linear,
 * for rates and scores). On entrance each slice fills from the hub out to its value, one after
 * another round the clock, lit at its outer arc like a bar at its tip; slices spring between data;
 * hover or the arrow keys pop a slice out and turn the rest graphite; leader labels name the
 * highest and lowest outside the circle, slice names ride the rim and value rings stay quiet.
 * Twelve slices or fewer can tilt (`view`): the slices close into an even ring whose height is the
 * value, a skyline of the seasons, and every slab names its value. Without WebGL, or with
 * `material="flat"`, slices draw in SVG.
 */
export function PolarArea({
  data,
  config,
  series,
  theme: themeProp,
  height,
  density = "standard",
  scale = "area",
  max,
  from = 0,
  gap: gapProp,
  active: activeProp,
  onActiveChange,
  labels = "auto",
  rim = "auto",
  tooltip = true,
  format = { kind: "number" },
  shareLabel = "of the total",
  material = "glass",
  view: viewProp,
  onViewChange,
  viewSwitch = true,
  loading = false,
  empty = false,
  error = false,
  replay = 0,
  label,
  describe,
  className,
}: PolarAreaProps) {
  const theme = usePageTheme(themeProp);
  const tile = density === "tile";
  const plotHeight = height ?? (tile ? 160 : density === "hero" ? 380 : 320);
  const [active, setActive] = useActive(activeProp, onActiveChange);
  const [localView, setLocalView] = useState<View | null>(null);
  const [handed, setHanded] = useState(false);
  const values = data.map((d) => (finite(d.value) ? Math.max(0, d.value) : 0));
  const total = values.reduce((a, b) => a + b, 0);
  const nothing = !loading && !error && (data.length === 0 || total <= 0);
  const status = stateOf({ loading, error, empty: empty || nothing });
  const top = max ?? valueTicks([0, Math.max(...values, 1)], 4).at(-1) ?? 1;
  const rimMax = Math.max(top, ...values);
  // Rings well inside the rim, so their figures never meet the slice names.
  const rings = valueTicks([0, top], 3).filter((t) => t > 0 && t < rimMax * 0.9);
  const formatRing = tickFormatter({ ...format, unit: undefined }, [0, top]);
  const fallbackKey = series ?? Object.keys(config)[0] ?? "";
  const colourKey = (d: PolarDatum) => (d.key && config[d.key] ? d.key : fallbackKey);
  const colours = data.map((d) => seriesColour(config, colourKey(d), theme));
  const tints = colours.map((c) => cssToRgb(c) ?? FALLBACK_TINT);
  const gap = gapProp ?? (data.length > 16 ? 2 : 3);
  const slices = sliceAngles(data.length, from);

  const style = resolveStyle({
    geometry: "circular",
    marks: ["solid", "reference"],
    measure: "amount",
    segments: data.length,
    density,
    heightMeaning: "value",
    material,
    view: viewProp,
  });
  useEffect(() => {
    if (process.env.NODE_ENV === "production") return;
    // A rose names its slices by place round the clock, so "too many segments" (a donut's warning)
    // does not apply; the rest do.
    for (const warning of style.warnings)
      if (warning.rule !== "too-many-segments") console.warn(`[PolarArea] ${warning.message}`);
    // Warn once per distinct set of warnings.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [style.warnings.map((w) => w.rule).join()]);

  // Leader labels need columns either side; on a narrow plot the rim and the tooltip carry them.
  const widest = data.reduce(
    (w, d, i) =>
      Math.max(w, textWidth(`${plain(values[i], format)} ${d.label}`, TYPE.label) * LIFT),
    0,
  );
  const sideFor = (width: number) => {
    if (labels === "none" || tile) return 0;
    const side = Math.min(width * 0.3, widest + 30);
    return width / 2 - side < 90 ? 0 : side;
  };
  const geometryFor = (width: number, h: number) =>
    roseGeometry(width, h, {
      side: sideFor(width),
      rim: tile ? 8 : 26,
      hole: 0.12,
      max: density === "hero" ? 190 : 170,
    });
  const radiusOf = (value: number, g: { inner: number; outer: number }) =>
    polarRadius(value, rimMax, g.inner, g.outer, scale);

  const measured = useMeasured();
  const tip = useRef<HTMLDivElement>(null);
  const leaders = useRef<(SVGPolylineElement | null)[]>([]);
  const tiltLayer = useRef<HTMLDivElement>(null);
  const tiltLeaders = useRef<(SVGPolylineElement | null)[]>([]);
  const noteRef = useRef<HTMLDivElement>(null);
  const ringPhase = useRef(0);
  const canTiltStyle =
    style.tilt !== "never" && style.materials.solid === "glass" && data.length <= 12 && !status;
  /** The tilted slabs as they will stand, so the camera frames them (and their pins) tightly. */
  const standing = slices.map((slice, i) => ({
    start: slice.start,
    end: slice.end,
    share: rimMax > 0 ? values[i] / rimMax : 0,
  }));

  const {
    box: layerBox,
    canvas: layerCanvas,
    svg: layerSvg,
    available: layerAvailable,
  } = useRadialLayer({
    theme,
    replay,
    material: style.materials.solid === "flat" ? "flat" : "glass",
    frame: (clock) => drawFrame(clock),
  });
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
  const canTilt = canTiltStyle && layerAvailable && ringAvailable;
  const view: View = canTilt ? (viewProp ?? localView ?? style.view) : "flat";
  const tilted = view === "tilt";

  /** Flat: the rose in optical glass; while turning to tilt, its slices close into an even ring. */
  function drawFrame(clock: LayerClock) {
    if (status) return { shapes: [] };
    const { width, height: h, motion, time } = clock;
    const g = geometryFor(width, h);
    const t = roseTilt(width, h, g, { rows: 1, slabs: standing });
    const morph = motion.spring("morph", tilted || handed ? 1 : 0, { rest: 0.002 });
    if (tilted && !handed && morph >= 0.998) queueMicrotask(() => setHanded(true));
    const stagger = Math.min(0.06, 0.9 / Math.max(1, data.length));
    let entering = false;
    const shapes: GlassShape[] = [];
    const reach: number[] = [];
    data.forEach((_d, i) => {
      const r = motion.spring(`r:${i}`, radiusOf(values[i], g), { rest: 0.05 });
      const outer = mix(r, t.outer, morph),
        inner = mix(g.inner, t.inner, morph);
      const pop = motion.ease(`pop:${i}`, active === i && !tilted ? 8 : 0, { rest: 0.05 });
      const tone = motion.ease(`tone:${i}`, active === null || active === i ? 1 : 0);
      const fill = easeInOut((time - 0.2 - i * stagger) / 0.55);
      if (fill < 1) entering = true;
      reach[i] = outer + pop;
      const { start, end, mid } = slices[i];
      const cx = g.cx + Math.sin(mid) * pop,
        cy = g.cy - Math.cos(mid) * pop;
      const [a0, a1] = trimmed(start, end, gap, (inner + outer) / 2);
      if (handed || outer - inner < 0.5 || a1 - a0 < 1e-4) return;
      const corner = Math.min(4, (outer - inner) * 0.2);
      shapes.push({
        id: `s${i}`,
        path: sectorPath(cx, cy, inner, outer, a0, a1, corner),
        bounds: sectorBounds(cx, cy, inner, outer, a0, a1),
        tint: tints[i],
        tone,
        fill,
        // The liquid rises from the hub out to the value, and the outer arc takes the tip light.
        radial: { cx, cy, inner, outer },
        // The path's own rounds, so the glass is evaluated from the sector, with no mask.
        sector: { cx, cy, inner, outer, start: a0, end: a1, corner },
        depth: Math.max(10, Math.min(18, (outer - inner) * 0.5)),
        bevel: Math.min(4, (outer - inner) * 0.18),
        flat:
          fill <= 0.001
            ? ""
            : sectorPath(
                cx,
                cy,
                inner,
                inner + (outer - inner) * fill,
                a0,
                a1,
                Math.min(corner, (outer - inner) * fill * 0.2),
              ),
      });
    });
    placeFlat(g, width, h, reach, morph, entering ? 0 : 1);
    return { shapes, moving: entering || (view === "flat" && handed) };
  }

  /** The tilted ring: equal slabs whose height is the value, drawn by the raymarched ring once
   * the rose has closed into it. */
  function tiltLayout(width: number, h: number) {
    const g = geometryFor(width, h);
    const t = roseTilt(width, h, g, { rows: 1, slabs: standing });
    const marks: RingMark[] =
      handed && !status
        ? data.map((_d, i) => ({
            key: `s${i}`,
            start: slices[i].start,
            end: slices[i].end,
            tint: tints[i],
            lit: active === null || active === i ? 1 : 0.32,
            height: t.low + (rimMax > 0 ? values[i] / rimMax : 0) * (t.high - t.low),
          }))
        : [];
    return {
      ring: {
        cx: g.cx,
        cy: g.cy,
        radius: (t.inner + t.outer) / 2,
        width: t.outer - t.inner,
        gap,
        bevel: 3,
      },
      tilt: t.tilt,
      view: handed ? view : ("flat" as View),
      turn: 0,
      sweep: { from, span: Math.PI * 2 },
      marks,
    };
  }

  /** Leader labels, rim names and the tooltip in the flat view. */
  function placeFlat(
    g: { cx: number; cy: number; outer: number; inner: number },
    width: number,
    h: number,
    reach: number[],
    morph: number,
    settled: number,
  ) {
    const flatShown = morph < 0.02 && !handed;
    const bounds: Rect = { left: 2, top: 2, right: width - 2, bottom: h - 2 };

    // The tooltip, out past the focused slice along its middle.
    let tipRect: Rect | null = null;
    const card = tip.current;
    if (card && active !== null && tooltip && flatShown && data[active]) {
      placeTooltip(
        card,
        { left: 0, top: 0, right: 0, bottom: 0 },
        { left: 4, top: 4, right: width - 4, bottom: h - 4 },
        {
          prefer: "outward",
          ring: {
            cx: g.cx,
            cy: g.cy,
            outer: Math.max(reach[active], g.outer * 0.5) + 4,
            angle: slices[active].mid,
          },
        },
      );
      const placed = /translate\(([-\d.]+)px, ([-\d.]+)px\)/.exec(card.style.transform);
      if (placed) {
        const left = Number(placed[1]),
          top = Number(placed[2]);
        tipRect = { left, top, right: left + card.offsetWidth, bottom: top + card.offsetHeight };
      }
    }

    // Leader labels outside the circle, each leader starting at its own slice's rim.
    const side = sideFor(width);
    const lines: [number, number, number, number][] = [];
    const labelBoxes: Rect[] = [];
    if (side > 0) {
      // `auto` names the highest slice, and on a clock of twelve or fewer the lowest too (first
      // of any tie): with more, the low slices sit by the hub and their leaders would cross it.
      const highest = values.indexOf(Math.max(...values)),
        lowest = values.indexOf(Math.min(...values));
      const requests = data.map((_d, i) => {
        const size = measured.size(`o${i}`);
        const focus = active === i;
        let priority = 0;
        if (values[i] > 0 || focus) {
          if (focus) priority = Infinity;
          else if (labels === "all") priority = 1 + values[i] / (rimMax || 1);
          else if (labels === "auto")
            priority =
              i === highest ? 3 : i === lowest && lowest !== highest && data.length <= 12 ? 2 : 0;
          else if (labels === "ends") priority = i === 0 || i === data.length - 1 ? 1 : 0;
        }
        return {
          id: `o${i}`,
          angle: slices[i].mid,
          width: size.width * (focus ? LIFT : 1),
          height: size.height * (focus ? LIFT : 1),
          priority: settled >= 1 ? priority : 0,
        };
      });
      const spots = layoutOutsideLabels(requests, {
        cx: g.cx,
        cy: g.cy,
        radius: g.outer + (rim === "none" ? 6 : 22),
        bounds,
        offset: 10,
        reach: 10,
      });
      spots.forEach((spot, i) => {
        const size = requests[i];
        const box = {
          left: spot.left,
          top: spot.top,
          right: spot.left + size.width,
          bottom: spot.top + size.height,
        };
        const shown = spot.visible && flatShown && !(tipRect && overlaps(box, tipRect, 4));
        const natural = measured.size(`o${i}`);
        const dx = spot.side === "left" ? size.width - natural.width : 0;
        const dy = (size.height - natural.height) / 2;
        const element = measured.element(`o${i}`);
        if (element) element.dataset.side = spot.side;
        put(element, spot.left + dx, spot.top + dy, shown);
        const line = leaders.current[i];
        if (line) {
          const edge = polar(g.cx, g.cy, reach[i] + 2, slices[i].mid);
          const points = [edge, ...spot.points];
          line.setAttribute("points", points.map((p) => `${p.x},${p.y}`).join(" "));
          line.dataset.shown = String(shown);
          if (shown)
            for (let k = 0; k < points.length - 1; k++)
              lines.push([points[k].x, points[k].y, points[k + 1].x, points[k + 1].y]);
        }
        if (shown) labelBoxes.push(box);
      });
    }

    // Slice names round the rim: every one that fits, and never under a label, leader or tooltip.
    if (rim !== "none") {
      const step = rimStep(
        data.length,
        g.outer + 12,
        Math.max(...data.map((d) => textWidth(d.label, TYPE.micro))) + 6,
      );
      data.forEach((_d, i) => {
        const element = measured.element(`t${i}`);
        if (!element) return;
        const size = measured.size(`t${i}`);
        const at = polar(g.cx, g.cy, g.outer + 12, slices[i].mid);
        const box = {
          left: at.x - size.width / 2,
          top: at.y - size.height / 2,
          right: at.x + size.width / 2,
          bottom: at.y + size.height / 2,
        };
        const crossed = lines.some(([x1, y1, x2, y2]) => segmentHits(box, x1, y1, x2, y2));
        const shown =
          flatShown &&
          (i % step === 0 || active === i) &&
          !crossed &&
          !labelBoxes.some((other) => overlaps(box, other, 2)) &&
          !(tipRect && overlaps(box, tipRect, 2)) &&
          reach[i] < g.outer + 4;
        put(element, box.left, box.top, shown);
      });
    }
    if (tiltLayer.current && !handed) tiltLayer.current.style.opacity = "0";
  }

  /** Tilted, every slab names its value on a pin: back slabs in a row above the ring, front slabs
   * in a row below it, each on a vertical stem, so no label sits on glass; the tooltip goes beside
   * the slab. */
  function placeTilted(frame: RingFrame) {
    ringPhase.current = frame.phase;
    if (!tilted && handed && frame.phase <= 0) queueMicrotask(() => setHanded(false));
    const layerEl = tiltLayer.current;
    if (!layerEl) return;
    const shown = Math.max(0, Math.min(1, (frame.phase - 0.6) / 0.3));
    layerEl.style.opacity = String(Math.round(shown * 100) / 100);
    if (noteRef.current) noteRef.current.style.opacity = String(Math.round(shown * 100) / 100);
    const { segments, width, height: h } = frame;
    if (!segments.length) return;

    // The tooltip beside the focused slab.
    let tipRect: Rect | null = null;
    const card = tip.current;
    if (card && active !== null && tooltip && segments[active] && frame.camera > 0.02) {
      const segment = segments[active];
      const corners = [segment.start, (segment.start + segment.end) / 2, segment.end].flatMap(
        (a) => [
          frame.at(a, segment.radius - segment.width / 2, 0),
          frame.at(a, segment.radius + segment.width / 2, 0),
          frame.at(a, segment.radius + segment.width / 2, segment.height),
          frame.at(a, segment.radius - segment.width / 2, segment.height),
        ],
      );
      placeTooltip(
        card,
        {
          left: Math.min(...corners.map((p) => p.x)),
          right: Math.max(...corners.map((p) => p.x)),
          top: Math.min(...corners.map((p) => p.y)),
          bottom: Math.max(...corners.map((p) => p.y)),
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

    // Every slab on a pin: back slabs in a row above the ring, front slabs in a row below it.
    const extent = slabExtent(frame.at, segments);
    const requests: PinRequest[] = segments.map((segment, i) => {
      const anchor = slabPin(frame.at, segment);
      const size = measured.size(`v${i}`);
      return {
        id: `v${i}`,
        side: anchor?.side ?? "up",
        x: anchor?.x ?? 0,
        y: anchor?.y ?? 0,
        ridge: anchor?.ridge,
        width: size.width,
        height: size.height,
        priority: anchor ? (active === i ? Infinity : 1 + values[i] / (rimMax || 1)) : 0,
      };
    });
    const placed = layoutPinLabels(requests, {
      // Clear of the height note and the view switch along the top.
      bounds: { left: 4, top: 34, right: width - 4, bottom: h - 2 },
      ceiling: extent.ceiling,
      floor: extent.floor,
      // A narrow card keeps the slab in focus and the largest few.
      limit: width < 520 ? 4 : Infinity,
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
      put(measured.element(`v${i}`), spot.left, spot.top, visible);
      const line = tiltLeaders.current[i];
      if (line) {
        line.setAttribute("points", pinPoints(spot.points));
        line.dataset.shown = String(visible);
      }
    });
  }

  const pickAt = (x: number, y: number, size: { width: number; height: number }) => {
    if (status) return null;
    if (handed && ringFrame.current && ringFrame.current.camera > 0.02) return ringPick(x, y);
    const g = geometryFor(size.width, size.height);
    return sliceAt(slices, g.cx, g.cy, x, y, g.outer + 30);
  };
  const share = (i: number) => (total > 0 ? values[i] / total : 0);
  const speak =
    describe ??
    ((i: number) =>
      `${data[i].label}: ${plain(values[i], format)}, ${plain(share(i), { kind: "percent", decimals: 1 })} ${shareLabel}.`);
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
        ];

  function switchView(next: View) {
    setLocalView(next);
    onViewChange?.(next);
  }

  return (
    <ChartScope theme={theme} className={`${styles.chart} ${className ?? ""}`}>
      <ChartFrame
        credit={<LiquidCredit />}
        theme={theme}
        height={plotHeight}
        insets={{ left: 0, right: 0, top: 0, bottom: 0 }}
        count={status ? 0 : data.length}
        active={status ? null : focus}
        onActiveChange={setActive}
        plotRef={layerBox}
        pick={pickAt}
        label={`${label}. Use the arrow keys to read each slice.`}
        describe={speak}
        table={{
          caption: label,
          columns: ["Slice", "Value", "Share"],
          rows: data.map((d, i) => [
            d.label,
            plain(values[i], format),
            plain(share(i), { kind: "percent", decimals: 1 }),
          ]),
        }}
      >
        {!status && (
          <Rings
            geometryFor={geometryFor}
            rings={rings}
            radiusOf={radiusOf}
            hidden={tilted || handed}
          />
        )}
        <div ref={ringBox} className={styles.layer}>
          <canvas ref={ringCanvas} className={styles.canvas} aria-hidden="true" />
        </div>
        <canvas ref={layerCanvas} className={styles.canvas} aria-hidden="true" />
        <svg ref={layerSvg} className={styles.layer} aria-hidden="true" />
        {!status && !tile && !(tilted || handed) && (
          <RingFigures
            geometryFor={geometryFor}
            rings={rings}
            radiusOf={radiusOf}
            format={formatRing}
          />
        )}
        {status?.kind === "loading" && (
          <ChartState kind="loading" shape="ring" ring={{ inner: 0.12 }} />
        )}
        {status && status.kind !== "loading" && (
          <ChartState
            kind={status.kind}
            message={status.kind === "error" ? stateMessage(error) : stateMessage(empty)}
          />
        )}
        {!status && (
          <svg className={`${styles.layer} ${styles.leaders}`} aria-hidden="true">
            {data.map((_d, i) => (
              <polyline
                key={i}
                ref={(element) => {
                  leaders.current[i] = element;
                }}
                data-active={active === i}
                data-shown="false"
              />
            ))}
          </svg>
        )}
        {!status &&
          data.map((d, i) => (
            <div
              key={`t${i}`}
              ref={measured.ref(`t${i}`)}
              className={`${styles.placed} ${styles.tick} ${roles.micro}`}
              data-active={active === i}
              data-shown="false"
              aria-hidden="true"
            >
              {d.label}
            </div>
          ))}
        {!status &&
          data.map((d, i) => (
            <div
              key={`o${i}`}
              ref={measured.ref(`o${i}`)}
              className={`${styles.placed} ${styles.value} ${styles.outside}`}
              data-figure-active={active === i}
              data-shown="false"
              aria-hidden="true"
            >
              <span className={`${styles.reading} ${roles.lift}`}>
                <Figure value={values[i]} {...format} className={roles.figureS} />
                <span className={`${styles.caption} ${roles.caption}`}>{d.label}</span>
              </span>
            </div>
          ))}
        {!status && canTilt && (
          <div ref={tiltLayer} className={styles.layer} style={{ opacity: 0 }} aria-hidden="true">
            <svg className={pins.stems} aria-hidden="true">
              {data.map((_d, i) => (
                <polyline
                  key={i}
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
                key={`v${i}`}
                ref={measured.ref(`v${i}`)}
                className={pins.label}
                data-active={active === i}
                data-dim={active !== null && active !== i}
                data-shown="false"
              >
                <Figure value={values[i]} {...format} className={roles.figureS} />
                <span className={`${pins.name} ${roles.caption}`}>{d.label}</span>
              </div>
            ))}
          </div>
        )}
        {!status && canTilt && <HeightNote ref={noteRef} meaning="value" shown={tilted} />}
        {!status && tooltip && focus !== null && (
          <ChartTooltip
            ref={tip}
            title={data[focus].label}
            rows={tooltipRows}
            footer={
              <TooltipFoot>
                <Figure value={share(focus)} kind="percent" decimals={1} /> {shareLabel}
              </TooltipFoot>
            }
          />
        )}
      </ChartFrame>
      {canTilt && viewSwitch && !status && (
        <ViewSwitch
          available
          view={view}
          onChange={switchView}
          label={`${label}: view`}
          className={styles.switch}
        />
      )}
      {focus !== null && (tilted || handed) && <p className={`${styles.srOnly}`}>{speak(focus)}</p>}
    </ChartScope>
  );
}

/** True when the segment from (x1, y1) to (x2, y2) passes through the box. */
function segmentHits(box: Rect, x1: number, y1: number, x2: number, y2: number) {
  for (let k = 0; k <= 8; k++) {
    const x = x1 + ((x2 - x1) * k) / 8,
      y = y1 + ((y2 - y1) * k) / 8;
    if (x >= box.left - 1 && x <= box.right + 1 && y >= box.top - 1 && y <= box.bottom + 1)
      return true;
  }
  return false;
}

/** Value rings: quiet dashed circles at round values (spaced by area when the slices are). */
function Rings({
  geometryFor,
  rings,
  radiusOf,
  hidden,
}: {
  geometryFor: (
    width: number,
    height: number,
  ) => { cx: number; cy: number; outer: number; inner: number };
  rings: readonly number[];
  radiusOf: (value: number, g: { inner: number; outer: number }) => number;
  hidden: boolean;
}) {
  const { size } = useChartFrame();
  if (!size.width) return null;
  const g = geometryFor(size.width, size.height);
  return (
    <svg
      className={styles.layer}
      style={{ opacity: hidden ? 0 : 1, transition: "opacity 200ms" }}
      aria-hidden="true"
    >
      <circle className={styles.axis} data-strong="true" cx={g.cx} cy={g.cy} r={g.outer} />
      {rings.map((value) => (
        <circle
          key={value}
          className={styles.axis}
          data-dashed="true"
          cx={g.cx}
          cy={g.cy}
          r={radiusOf(value, g)}
        />
      ))}
    </svg>
  );
}

/** The rings' values up from 12 o'clock, over the glass with a halo. */
function RingFigures({
  geometryFor,
  rings,
  radiusOf,
  format,
}: {
  geometryFor: (
    width: number,
    height: number,
  ) => { cx: number; cy: number; outer: number; inner: number };
  rings: readonly number[];
  radiusOf: (value: number, g: { inner: number; outer: number }) => number;
  format: (value: number) => string;
}) {
  const { size } = useChartFrame();
  if (!size.width) return null;
  const g = geometryFor(size.width, size.height);
  return (
    <>
      {rings.map((value) => (
        <span
          key={value}
          className={`${styles.ringLabel} ${roles.micro}`}
          style={{
            position: "absolute",
            left: g.cx + 4,
            top: g.cy - radiusOf(value, g),
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
