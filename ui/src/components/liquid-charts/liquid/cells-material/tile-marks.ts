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
"use client";

/**
 * Glass tiles whose every edge springs: treemap tiles and sankey nodes. A treemap that zooms
 * between levels, and sankey nodes that move when the data changes, need the whole rect to move:
 * x, y, width and height each on their own spring, as `useDotMarks` does for beads. Same renderers
 * as `useBarMarks` (raymarched up to 24 tiles, optical beyond), with an eased fill from the base
 * instead of the poured liquid, a tile's `from` rect (where a tile new to the layout starts, so it
 * can grow out of its parent) and a flat SVG twin for `material="flat"` or no WebGL.
 */

import { pixelRatio } from "@/components/liquid-charts/liquid/core/render-scale";
import { chartPresence, frameDelta, lostContext, substeps } from "@/components/liquid-charts/liquid/core/presence";
import { createChartOpticalRenderer, type ChartOpticalMark } from "@/components/liquid-charts/ui/chart-optical";
import { createLiquidGlass, type Rgb, type Theme } from "@/components/liquid-charts/liquid/glass/liquid-glass";
import { clamp01, easeOut, settle } from "@/components/liquid-charts/liquid/core/geometry";
import { cssRgb as css } from "@/components/liquid-charts/liquid/core/colour-scale";
import { useEffect, useId, useRef, useState } from "react";
import { ENTER } from "@/components/liquid-charts/liquid/core/format";

export type TileBox = { x: number; y: number; width: number; height: number };

/** Where one tile should be, in plot px. */
export type TileMark = TileBox & {
  key: string;
  /** Optional caller revision echoed by actual paint outcomes. */
  revision?: number;
  tint: Rgb;
  /** 1 lit in its tint, 0 smoked graphite (out of focus). */
  lit: number;
  /** Seconds after the entrance starts that this tile begins to fill. */
  delay?: number;
  /** How full the glass is, 0–1 (default 1): below 1 it is a container holding liquid. */
  level?: number;
  /** Etch the "other state" pattern (projected, beyond a limit): the whole tile, or from a share of
   * its height up. */
  pattern?: boolean | number;
  /** Where a tile new to the layout starts (its parent's rect, or beyond the plot for a zoom),
   * already filled. Without it a new tile fills from its base where it stands. */
  from?: TileBox;
};

/** A tile as drawn this frame: its sprung rect, tone and fill. */
export type TileFrame = TileMark & { box: TileBox; tone: number; fill: number };

export type TileEngine = "raymarch" | "optical" | "svg";
export type TilePaintOutcome = {
  key: string;
  revision: number;
  status: "native" | "host";
  reason?: string;
};

/** Bind actual renderer outcomes to the current caller identities. Flat SVG is
 * reported separately from native material; filtered and removed tiles restore host paint. */
export function createTilePaintReporter() {
  type Identity = Pick<TileMark, "key" | "revision">;
  type Emit = (outcomes: readonly TilePaintOutcome[]) => void;
  type Result = {
    engine: TileEngine;
    painted: boolean;
    rendered?: readonly string[];
    reason?: string;
  };
  type Binding = {
    emit: Emit;
    frames: Identity[];
    last: Map<string, string>;
    retired: Set<string>;
    deliver(outcomes: TilePaintOutcome[]): void;
    invalidate(reason: string): void;
  };
  let current: Binding | null = null;
  return {
    bind(emit: Emit, frames: readonly Identity[], drawn: readonly string[]) {
      const captured = frames.map(({ key, revision }) => ({ key, revision }));
      const keys = new Map(captured.map((frame) => [frame.key, frame.revision ?? 0]));
      const drawable = new Set(drawn);
      const last = new Map<string, string>();
      if (current) {
        const previous = current;
        const retired = previous.frames.filter(
          (frame) => previous.emit !== emit || keys.get(frame.key) !== (frame.revision ?? 0),
        );
        previous.deliver(
          retired.map((frame) => ({
            key: frame.key,
            revision: frame.revision ?? 0,
            status: "host",
            reason:
              previous.emit !== emit
                ? "consumer-replaced"
                : keys.has(frame.key)
                  ? "revision-replaced"
                  : "removed",
          })),
        );
        retired.forEach((frame) => previous.retired.add(frame.key));
        if (previous.emit === emit)
          captured.forEach((frame) => {
            const signature = previous.last.get(frame.key);
            if (signature && !previous.retired.has(frame.key)) last.set(frame.key, signature);
          });
      }
      // Capture both callback and identities when the renderer accepts this scene.
      // A retained canvas redraw must never consult the next render's callback.
      const binding: Binding = {
        emit,
        frames: captured,
        last,
        retired: new Set(),
        deliver(outcomes) {
          // Even an unchanged draw supersedes its predecessor. Retaining that
          // earlier binding must not revive receipts after a later owner replaces it.
          if (current !== binding) return;
          const changed = outcomes.filter((outcome) => {
            const signature = JSON.stringify([outcome.revision, outcome.status, outcome.reason]);
            if (last.get(outcome.key) === signature) return false;
            last.set(outcome.key, signature);
            return true;
          });
          if (changed.length) emit(changed);
        },
        invalidate(reason) {
          binding.deliver(
            captured
              .filter((frame) => !binding.retired.has(frame.key))
              .map((frame) => ({
                key: frame.key,
                revision: frame.revision ?? 0,
                status: "host",
                reason,
              })),
          );
        },
      };
      current = binding;
      return {
        report(result: Result) {
          const rendered = result.rendered ? new Set(result.rendered) : null;
          binding.deliver(
            captured
              .filter((frame) => !binding.retired.has(frame.key))
              .map((frame): TilePaintOutcome => {
                const painted =
                  result.engine !== "svg" &&
                  result.painted &&
                  drawable.has(frame.key) &&
                  (!rendered || rendered.has(frame.key));
                return {
                  key: frame.key,
                  revision: frame.revision ?? 0,
                  status: painted ? "native" : "host",
                  ...(!painted
                    ? {
                        reason: !drawable.has(frame.key)
                          ? "not-rendered"
                          : (result.reason ??
                            (result.engine === "svg" ? "flat-svg" : "not-rendered")),
                      }
                    : {}),
                };
              }),
          );
        },
        invalidate: binding.invalidate,
      };
    },
    invalidate(reason: string) {
      current?.invalidate(reason);
    },
    dispose() {
      current?.invalidate("unmounted");
      current = null;
    },
  };
}

/** The raymarched renderer draws up to this many tiles; beyond it the optical one takes over. */
export const TILE_RAYMARCH_LIMIT = 24;

const SVG = "http://www.w3.org/2000/svg";
/** Flat graphite, per theme: the SVG twin of the glass's smoked tone. */
const FLAT_GREY: Record<Theme, Rgb> = { dark: [0.26, 0.26, 0.26], light: [0.81, 0.81, 0.81] };

/** A tile's flat colour at a tone: its tint, eased towards graphite as it leaves focus. */
export function flatTileColour(tint: Rgb, tone: number, theme: Theme) {
  const grey = FLAT_GREY[theme];
  const t = clamp01(tone);
  return css([0, 1, 2].map((k) => grey[k] + (tint[k] - grey[k]) * t) as unknown as Rgb);
}

/** A rounded rect as path data, corners clamped to fit. */
export function roundedRect(r: TileBox, radius: number) {
  const k = Math.max(0, Math.min(radius, r.width / 2, r.height / 2));
  const right = r.x + r.width,
    bottom = r.y + r.height;
  const f = settle;
  return `M${f(r.x + k)} ${f(r.y)}H${f(right - k)}A${f(k)} ${f(k)} 0 0 1 ${f(right)} ${f(r.y + k)}V${f(bottom - k)}A${f(k)} ${f(k)} 0 0 1 ${f(right - k)} ${f(bottom)}H${f(r.x + k)}A${f(k)} ${f(k)} 0 0 1 ${f(r.x)} ${f(bottom - k)}V${f(r.y + k)}A${f(k)} ${f(k)} 0 0 1 ${f(r.x + k)} ${f(r.y)}Z`;
}

/** Glass depth for a tile whose shorter side is `side` px: shallow for small tiles, capped for
 * large ones so their cores don't go dark. */
export const tileDepth = (side: number, factor = 0.6) =>
  settle(Math.max(4, Math.min(20, side * factor)));

type Spring = { value: number; velocity: number };
const spring = (s: Spring, target: number, dt: number) => {
  for (let k = 0, n = substeps(dt); k < n; k++) {
    s.velocity += (110 * (target - s.value) - 21 * s.velocity) * (dt / n);
    s.value += s.velocity * (dt / n);
  }
  if (Math.abs(target - s.value) < 0.25 && Math.abs(s.velocity) < 1) {
    s.value = target;
    s.velocity = 0;
    return false;
  }
  return true;
};

/**
 * One glass canvas for rectangular tiles: treemap tiles, sankey nodes. Tiles spring to new rects
 * (all four edges), fill from their base on entrance, and fade between tint and graphite;
 * `onFrame` receives every tile's current rect so labels and tooltips follow. It draws only while
 * something moves. Up to 24 tiles are raymarched, more use the optical renderer, both on the page's
 * one GPU context; `material="flat"` or no WebGL draws the same frames into the `svg` layer and
 * `available` turns false.
 */
export function useTileMarks({
  theme,
  layout,
  onFrame,
  onPaint,
  depth = 0.6,
  bevel = 5,
  replay = 0,
  bleed = 24,
  material = "glass",
}: {
  theme: Theme;
  layout: (width: number, height: number) => TileMark[];
  onFrame?: (tiles: TileFrame[], width: number, height: number) => void;
  /** Actual renderer/canvas outcome for the caller revision. Flat SVG is host
   * fallback, not native material; onFrame alone never establishes paint. */
  onPaint?: (outcomes: readonly TilePaintOutcome[]) => void;
  /** Glass depth as a multiple of each tile's shorter side (capped). */
  depth?: number;
  bevel?: number;
  replay?: number;
  bleed?: number;
  material?: "glass" | "flat";
}) {
  const box = useRef<HTMLDivElement>(null);
  const canvas = useRef<HTMLCanvasElement>(null);
  const svg = useRef<SVGSVGElement>(null);
  const uid = useId().replace(/[^a-zA-Z0-9_-]/g, "");
  const [available, setAvailable] = useState(true);
  const live = useRef({
    theme,
    depth,
    bevel,
    replay,
    layout,
    onFrame,
    onPaint,
    material,
    wake: () => {},
  });

  useEffect(() => {
    Object.assign(live.current, {
      theme,
      depth,
      bevel,
      replay,
      layout,
      onFrame,
      onPaint,
      material,
    });
    live.current.wake();
  });

  useEffect(() => {
    const surface = canvas.current,
      root = box.current;
    if (!root) return;
    const shared = live.current;
    type PaintBinding = ReturnType<ReturnType<typeof createTilePaintReporter>["bind"]>;
    let glassPaint: PaintBinding | null = null,
      opticalPaint: PaintBinding | null = null,
      engine: TileEngine | null = null;
    const receipts = createTilePaintReporter();
    const report = (
      binding: PaintBinding | null,
      painted: boolean,
      reason?: string,
      rendered?: readonly string[],
    ) => {
      if (!binding || !engine) return;
      binding.report({ engine, painted, reason, rendered });
      if (surface) surface.style.visibility = painted && engine !== "svg" ? "" : "hidden";
    };
    const glass = surface
      ? createLiquidGlass(surface, {
          shared: true,
          onPaint: onPaint
            ? (result) => {
                if (engine === "raymarch") report(glassPaint, result.painted, result.reason);
              }
            : undefined,
        })
      : null;
    if (!glass) setAvailable(false);
    let optical: ReturnType<typeof createChartOpticalRenderer> | null = null;
    let opticalFailed = false;
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    const edge = bleed;
    if (surface)
      Object.assign(surface.style, {
        left: `${-edge}px`,
        top: `${-edge}px`,
        width: `calc(100% + ${edge * 2}px)`,
        height: `calc(100% + ${edge * 2}px)`,
      });
    let width = 0,
      height = 0,
      canvasWidth = 0,
      canvasHeight = 0;
    // As in useBarMarks: a whole number of CSS px on a whole device pixel, so the canvas maps 1:1
    // to the screen and no edge softens.
    const offset = { x: edge, y: edge };
    const observer = new ResizeObserver(() => {
      const r = root.getBoundingClientRect();
      const ratio = pixelRatio();
      const fraction = (value: number) => ((((value * ratio) % 1) + 1) % 1) / ratio;
      width = r.width;
      height = r.height;
      offset.x = edge + fraction(r.left);
      offset.y = edge + fraction(r.top);
      canvasWidth = Math.ceil(width + offset.x + edge);
      canvasHeight = Math.ceil(height + offset.y + edge);
      if (surface) {
        surface.style.left = `${-offset.x}px`;
        surface.style.top = `${-offset.y}px`;
      }
      glass?.resize(canvasWidth, canvasHeight);
      wake();
    });
    observer.observe(root);
    type State = {
      x: Spring;
      y: Spring;
      w: Spring;
      h: Spring;
      tone: number;
      level: number;
      /** When its entrance starts (s), or -Infinity once it should be full. */
      born: number;
    };
    const state = new Map<string, State>();
    const presence = chartPresence(root, () => wake(), { redraw });
    // The optical renderer announces a restored context on the canvas: pick the motion back up.
    const onOptical = (event: Event) => {
      const available = (event as CustomEvent<{ available?: boolean }>).detail?.available;
      if (available) wake();
      else if (engine === "optical" && available === false)
        report(opticalPaint, false, "context-lost");
    };
    surface?.addEventListener("chart-optical-status", onOptical);
    let replayed = shared.replay,
      start = presence.clock(performance.now()) / 1000,
      raf = 0,
      last = 0,
      first = true;
    const rects = new Map<string, SVGRectElement[]>();

    function step(nowMs: number) {
      raf = 0;
      if (!width || !height) return;
      // Offscreen or in a hidden tab: keep everything as it is and draw nothing until it is back.
      // Time runs on the presence clock, held meanwhile.
      if (!presence.active) {
        last = 0;
        return;
      }
      const now = presence.clock(nowMs) / 1000;
      // The real time since the last frame; past a quarter second (or in a settled frame, for
      // print and export) the motion lands at its end instead of playing in slow motion.
      const { dt, snap } = frameDelta(now * 1000, last * 1000);
      const jump = snap || presence.settling;
      last = now;
      if (shared.replay !== replayed) {
        replayed = shared.replay;
        start = now;
        for (const s of state.values()) s.born = start + 0.25;
      }
      let moving = false;
      const seen = new Set<string>();
      const targets = shared.layout(width, height);
      const frames: TileFrame[] = targets.map((target) => {
        seen.add(target.key);
        const level = clamp01(target.level ?? 1);
        let s = state.get(target.key);
        if (!s) {
          const from = !first && target.from ? target.from : target;
          s = {
            x: { value: from.x, velocity: 0 },
            y: { value: from.y, velocity: 0 },
            w: { value: from.width, velocity: 0 },
            h: { value: from.height, velocity: 0 },
            tone: target.lit,
            level,
            // The first layout enters with the chart; later arrivals with a `from` are already
            // full (they are the parts of something already on screen); others fill quickly.
            born: first ? start + 0.25 : target.from ? -Infinity : now - 0.15,
          };
          state.set(target.key, s);
        }
        // A settled frame (print, export) shows every tile full, however late it arrived.
        if (presence.settling) s.born = -Infinity;
        if (reduced || jump) {
          s.x = { value: target.x, velocity: 0 };
          s.y = { value: target.y, velocity: 0 };
          s.w = { value: target.width, velocity: 0 };
          s.h = { value: target.height, velocity: 0 };
          s.tone = target.lit;
          s.level = level;
        } else {
          if (spring(s.x, target.x, dt)) moving = true;
          if (spring(s.y, target.y, dt)) moving = true;
          if (spring(s.w, target.width, dt)) moving = true;
          if (spring(s.h, target.height, dt)) moving = true;
          s.level += (level - s.level) * (1 - Math.exp(-7 * dt));
          if (Math.abs(level - s.level) > 0.002) moving = true;
          else s.level = level;
          s.tone += (target.lit - s.tone) * (1 - Math.exp(-11 * dt));
          if (Math.abs(target.lit - s.tone) > 0.004) moving = true;
          else s.tone = target.lit;
        }
        const entrance =
          reduced || s.born === -Infinity
            ? 1
            : easeOut((now - s.born - (target.delay ?? 0)) / ENTER.mark);
        if (entrance < 1) moving = true;
        return {
          ...target,
          box: {
            x: s.x.value,
            y: s.y.value,
            width: Math.max(0, s.w.value),
            height: Math.max(0, s.h.value),
          },
          tone: s.tone,
          fill: entrance * s.level,
        };
      });
      first = false;
      for (const key of state.keys()) if (!seen.has(key)) state.delete(key);

      const glassOn = shared.material === "glass" && Boolean(glass) && !opticalFailed;
      const next: TileEngine = !glassOn
        ? "svg"
        : frames.length > TILE_RAYMARCH_LIMIT
          ? "optical"
          : "raymarch";
      if (next !== engine) {
        // Whichever engine paints next, the glass's last picture is no longer on the canvas.
        glass?.invalidate();
        if (engine === "optical") {
          optical?.destroy();
          optical = null;
        }
        if (engine === "svg") clearSvg();
        if (next !== "svg" && surface) surface.style.visibility = "";
        if (next === "svg" && surface)
          surface.getContext("2d")?.clearRect(0, 0, surface.width, surface.height);
        engine = next;
      }
      const drawn = frames.filter((t) => t.box.width >= 0.5 && t.box.height >= 0.5);
      const submitted = shared.onPaint
        ? receipts.bind(
            shared.onPaint,
            frames,
            drawn.map((tile) => tile.key),
          )
        : null;
      if (!submitted && surface && engine !== "svg") surface.style.visibility = "";
      const thinnest = Math.min(200, ...drawn.map((t) => Math.min(t.box.width, t.box.height)));
      const solidBevel = settle(Math.min(shared.bevel, thinnest * 0.2));

      if (engine === "raymarch" && glass) {
        glassPaint = submitted;
        glass.draw({
          kind: "bars",
          theme: shared.theme,
          depth: tileDepth(thinnest, shared.depth),
          bevel: solidBevel,
          bars: drawn.map((t) => ({
            x: settle(t.box.x + t.box.width / 2 + offset.x),
            base: settle(t.box.y + t.box.height + offset.y),
            width: settle(t.box.width),
            height: settle(Math.max(1, t.box.height)),
            direction: "up" as const,
            fill: settle(t.fill * 1e3) / 1e3,
            tint: t.tint,
            tone: settle(t.tone),
            pattern:
              t.pattern === undefined || t.pattern === false
                ? undefined
                : t.pattern === true
                  ? 0
                  : t.pattern,
          })),
        });
      } else if (engine === "optical" && surface) {
        opticalPaint = submitted;
        optical ??= createChartOpticalRenderer(surface);
        const result = optical.draw({
          width: canvasWidth,
          height: canvasHeight,
          dpr: pixelRatio(),
          theme: shared.theme,
          reducedMotion: reduced,
          marks: drawn.map((t): ChartOpticalMark => {
            const r = {
              x: t.box.x + offset.x,
              y: t.box.y + offset.y,
              width: t.box.width,
              height: t.box.height,
            };
            const side = Math.min(r.width, r.height);
            const corner = settle(Math.min(shared.bevel, side * 0.2));
            return {
              id: t.key,
              kind: "glass",
              path: roundedRect(r, corner),
              bounds: r,
              color: css(t.tint),
              tone: t.tone,
              fill: t.fill,
              fillOrigin: "bottom",
              pattern:
                t.pattern === undefined || t.pattern === false
                  ? undefined
                  : t.pattern === true
                    ? 0
                    : t.pattern,
              // The path is this rect with its corners rounded by `corner`: no mask needed.
              shape: { kind: "rect", ...r, radius: corner, exact: true },
              depth: tileDepth(side, shared.depth),
              bevel: corner,
            };
          }),
        });
        report(
          opticalPaint,
          result.available && result.visible && !lostContext(result),
          lostContext(result)
            ? "context-lost"
            : !result.available
              ? "optical-unavailable"
              : undefined,
          result.rendered,
        );
        if (lostContext(result)) {
          // The context is lost for now: keep the last frame until the restore repaints it.
          moving = false;
        } else if (!result.available) {
          opticalFailed = true;
          setAvailable(false);
          moving = true;
        }
      } else if (engine === "svg" && svg.current) {
        drawSvg(svg.current, drawn, shared.theme);
        report(
          submitted,
          false,
          shared.material === "flat" ? "flat-svg" : "native-unavailable-flat-svg",
        );
      } else {
        report(submitted, false, "renderer-unavailable");
      }
      shared.onFrame?.(frames, width, height);
      // It loops only on screen once entered (about a third visible); else it holds this frame.
      if (moving && presence.running) raf = requestAnimationFrame(step);
      else last = 0;
    }

    function clearSvg() {
      rects.forEach((group) => group.forEach((rect) => rect.remove()));
      rects.clear();
    }

    function drawSvg(layer: SVGSVGElement, drawn: TileFrame[], theme: Theme) {
      const keys = new Set(drawn.map((t) => t.key));
      for (const [key, group] of rects)
        if (!keys.has(key)) {
          group.forEach((rect) => rect.remove());
          rects.delete(key);
        }
      const hatch = `${uid}-hatch`;
      if (!layer.querySelector("pattern")) {
        const defs = document.createElementNS(SVG, "defs");
        defs.innerHTML = `<pattern id="${hatch}" width="6" height="6" patternUnits="userSpaceOnUse" patternTransform="rotate(45)"><rect width="2" height="6" fill="white" opacity="0.4"/></pattern>`;
        layer.prepend(defs);
      }
      for (const t of drawn) {
        let group = rects.get(t.key);
        if (!group) {
          group = [0, 1, 2].map(() => document.createElementNS(SVG, "rect"));
          group[2].setAttribute("fill", `url(#${hatch})`);
          layer.append(...group);
          rects.set(t.key, group);
        }
        const [shell, liquid, etch] = group;
        const colour = flatTileColour(t.tint, t.tone, theme);
        const radius = String(settle(Math.min(3, t.box.width / 2, t.box.height / 2)));
        const set = (rect: SVGRectElement, b: TileBox) => {
          rect.setAttribute("x", String(settle(b.x)));
          rect.setAttribute("y", String(settle(b.y)));
          rect.setAttribute("width", String(settle(Math.max(0, b.width))));
          rect.setAttribute("height", String(settle(Math.max(0, b.height))));
          rect.setAttribute("rx", radius);
        };
        set(shell, t.box);
        shell.setAttribute("fill", colour);
        shell.setAttribute("opacity", t.fill >= 0.999 ? "0" : "0.16");
        const filled = {
          ...t.box,
          y: t.box.y + t.box.height * (1 - t.fill),
          height: t.box.height * t.fill,
        };
        set(liquid, filled);
        liquid.setAttribute("fill", colour);
        const from =
          t.pattern === undefined || t.pattern === false
            ? null
            : t.pattern === true
              ? 0
              : t.pattern;
        if (from === null) etch.setAttribute("height", "0");
        else {
          const top = t.box.y + t.box.height * (1 - Math.max(t.fill, 0));
          const bottom = t.box.y + t.box.height * (1 - from);
          set(etch, { ...t.box, y: top, height: Math.max(0, bottom - top) });
        }
      }
    }

    function wake() {
      if (!raf) raf = requestAnimationFrame(step);
    }
    /** Draws a frame now (print, export). */
    function redraw() {
      cancelAnimationFrame(raf);
      raf = 0;
      step(performance.now());
    }
    shared.wake = wake;
    wake();
    return () => {
      receipts.dispose();
      cancelAnimationFrame(raf);
      observer.disconnect();
      presence.stop();
      surface?.removeEventListener("chart-optical-status", onOptical);
      glass?.destroy();
      optical?.destroy();
      clearSvg();
      shared.wake = () => {};
    };
    // The bleed is fixed for the canvas's life.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return { box, canvas, svg, available };
}
