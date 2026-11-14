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
  cssRgb as css,
  easeOut,
  MarkLayer,
  resolveStyle,
  type BarFrame,
  type BarMark,
  type Rgb,
  type StyleSpec,
  type Theme,
} from "@/components/liquid-charts/liquid/core";
import { createChartOpticalRenderer, type ChartOpticalMark } from "@/components/liquid-charts/ui/chart-optical";
import { chartPresence, frameDelta, lostContext } from "@/components/liquid-charts/liquid/core/presence";
import { cssModule } from "@/components/liquid-charts/liquid/core/css-module";
import { ENTER } from "@/components/liquid-charts/liquid/core/format";
import { GlassLayer } from "@/components/liquid-charts/liquid/glass";
import { useEffect, useRef, useState } from "react";
import Styles from "./time.module.css";
import { diamondPath } from "./layout";

const styles = cssModule(Styles, "lc-time-");

export type BarLayerProps = {
  theme: Theme;
  replay: number;
  layout: (width: number, height: number) => BarMark[];
  onFrame: (bars: BarFrame[], width: number, height: number) => void;
};

/**
 * Glass bars from the core's mark layer, or the same bars drawn flat when the style rules say so
 * (too thin or too many: candles over a long range) or the browser has no WebGL. Glass stays
 * mounted while flat draws, so a resize back past the rule never replays the entrance.
 */
export function BarLayer({
  spec,
  material,
  smallest,
  ...layer
}: BarLayerProps & {
  spec: StyleSpec;
  material?: "glass" | "flat";
  /** The thinnest bar, px, for the 5px rule. */
  smallest?: number;
  className?: string;
}) {
  const style = resolveStyle({ ...spec, smallestMark: smallest, material });
  return (
    <MarkLayer
      Glass={GlassLayer}
      {...layer}
      flat={material === "flat"}
      fallback={style.materials.solid === "flat"}
    />
  );
}

/* ——— Glass diamonds: milestones ——— */

/** Where one milestone should be, in frame px. */
export type DiamondMark = {
  key: string;
  cx: number;
  cy: number;
  /** Corner to corner, px. */
  size: number;
  tint: Rgb;
  /** 1 lit in its tint, 0 smoked graphite. */
  lit: number;
  /** How full the glass is: 1 reached, 0 still ahead (empty glass). Changes ease. */
  level: number;
  /** Etch the glass from this share of its height (0: all of it): a milestone still ahead. */
  pattern?: number;
  /** Seconds after the entrance starts that it fills. */
  delay?: number;
};

/**
 * Milestones as small glass diamonds, through the optical renderer (it takes any closed path) on
 * the page's shared WebGL context. Each fills from below on entrance, to full once reached and not
 * at all while still ahead, so an upcoming milestone is clear empty glass; tone and level ease.
 * It draws only while something moves. `available` turns false without WebGL, so the chart can
 * draw the diamonds flat.
 */
export function useDiamondMarks({
  theme,
  layout,
  replay = 0,
  bleed = 24,
}: {
  theme: Theme;
  layout: (width: number, height: number) => DiamondMark[];
  replay?: number;
  bleed?: number;
}) {
  const box = useRef<HTMLDivElement>(null);
  const canvas = useRef<HTMLCanvasElement>(null);
  const [available, setAvailable] = useState(true);
  const live = useRef({ theme, layout, replay, wake: () => {} });

  useEffect(() => {
    Object.assign(live.current, { theme, layout, replay });
    live.current.wake();
  });

  useEffect(() => {
    const surface = canvas.current,
      root = box.current;
    if (!surface || !root) return;
    const optical = createChartOpticalRenderer(surface);
    const shared = live.current;
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    const edge = bleed;
    let width = 0,
      height = 0,
      canvasWidth = 0,
      canvasHeight = 0;
    const offset = { x: edge, y: edge };
    // Whole CSS px on a whole device pixel, as the core's layers do, so edges stay crisp.
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
      Object.assign(surface.style, {
        left: `${-offset.x}px`,
        top: `${-offset.y}px`,
        width: `${canvasWidth}px`,
        height: `${canvasHeight}px`,
      });
      wake();
    });
    observer.observe(root);
    const presence = chartPresence(root, () => wake(), { redraw });
    // The optical renderer announces a restored context on the canvas: pick the motion back up.
    const onOptical = (event: Event) => {
      if ((event as CustomEvent<{ available?: boolean }>).detail?.available) wake();
    };
    surface.addEventListener("chart-optical-status", onOptical);
    const state = new Map<string, { tone: number; level: number }>();
    let replayed = shared.replay,
      start = presence.clock(performance.now()),
      raf = 0,
      last = 0,
      failed = false;

    function step(frameTime: number) {
      raf = 0;
      if (!width || !height || failed) return;
      // Offscreen or in a hidden tab: hold everything and draw nothing until it is back.
      if (!presence.active) {
        last = 0;
        return;
      }
      // The presence clock at its real rate: a long frame stutters (past a quarter second the
      // easing snaps to its end) instead of playing in slow motion.
      const now = presence.clock(frameTime);
      const { dt, snap: long } = frameDelta(now, last);
      // A settled frame (print, export) lands every motion at its end.
      const snap = long || presence.settling;
      last = now;
      if (shared.replay !== replayed) {
        replayed = shared.replay;
        start = now;
      }
      const elapsed = (now - start) / 1000;
      let moving = false;
      const marks: ChartOpticalMark[] = shared.layout(width, height).map((mark) => {
        let s = state.get(mark.key);
        if (!s) {
          s = { tone: mark.lit, level: mark.level };
          state.set(mark.key, s);
        }
        if (reduced || snap) {
          s.tone = mark.lit;
          s.level = mark.level;
        } else {
          s.tone += (mark.lit - s.tone) * (1 - Math.exp(-11 * dt));
          s.level += (mark.level - s.level) * (1 - Math.exp(-7 * dt));
          if (Math.abs(mark.lit - s.tone) > 0.004 || Math.abs(mark.level - s.level) > 0.002)
            moving = true;
          else {
            s.tone = mark.lit;
            s.level = mark.level;
          }
        }
        const entrance = reduced
          ? 1
          : easeOut((elapsed - ENTER.delay - (mark.delay ?? 0)) / ENTER.small);
        if (entrance < 1) moving = true;
        const cx = mark.cx + offset.x,
          cy = mark.cy + offset.y,
          h = mark.size / 2;
        // A diamond enters by growing from its centre as it fills.
        const grow = 0.55 + 0.45 * entrance;
        // Its outline in local coordinates, placed and scaled by the transform, so a moving
        // diamond keeps its mask and a growing one reuses a mask per 2px of size. While it grows,
        // the outline is cut for the size step it is in (the renderer's step), so its 2.2px corners
        // stay 2.2px; at full size it is the exact outline.
        const step = grow >= 1 ? 1 : Math.max(1, 2 * Math.round(h * grow)) / mark.size;
        return {
          id: mark.key,
          kind: "glass",
          path: diamondPath(0, 0, mark.size, 2.2 / step),
          transform: [grow, 0, 0, grow, cx, cy],
          bounds: { x: cx - h * grow, y: cy - h * grow, width: h * 2 * grow, height: h * 2 * grow },
          color: css(mark.tint),
          tone: s.tone,
          fill: entrance * s.level,
          fillOrigin: "bottom",
          pattern: mark.pattern,
          depth: Math.max(4, mark.size * 0.55),
          bevel: Math.min(3, mark.size * 0.16),
          glow: 0.8,
        };
      });
      const result = optical.draw({
        width: canvasWidth,
        height: canvasHeight,
        dpr: pixelRatio(),
        theme: shared.theme,
        marks,
      });
      // A lost context is temporary: hold the last frame until the restore repaints it.
      if (lostContext(result)) {
        last = 0;
        return;
      }
      if (!result.available && marks.length) {
        failed = true;
        setAvailable(false);
        return;
      }
      // It loops only on screen once entered (about a third visible); else it holds this frame.
      if (moving && presence.running) raf = requestAnimationFrame(step);
      else last = 0;
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
      cancelAnimationFrame(raf);
      observer.disconnect();
      presence.stop();
      surface.removeEventListener("chart-optical-status", onOptical);
      optical.destroy();
      shared.wake = () => {};
    };
    // The bleed is fixed for the canvas's life.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return { box, canvas, available };
}

/** Glass diamonds, with flat ones in their place when the browser has no WebGL. */
export function DiamondLayer({
  theme,
  layout,
  replay,
  flat,
}: {
  theme: Theme;
  layout: (width: number, height: number) => DiamondMark[];
  replay: number;
  /** Drawn flat (forced, or crowded). */
  flat?: boolean;
}) {
  const [webgl, setWebgl] = useState(true);
  if (!webgl || flat) return <FlatDiamonds layout={layout} replay={replay} />;
  return (
    <GlassDiamonds
      theme={theme}
      layout={layout}
      replay={replay}
      onUnavailable={() => setWebgl(false)}
    />
  );
}

function GlassDiamonds({
  onUnavailable,
  ...props
}: {
  theme: Theme;
  layout: (width: number, height: number) => DiamondMark[];
  replay: number;
  onUnavailable: () => void;
}) {
  const { box, canvas, available } = useDiamondMarks(props);
  useEffect(() => {
    if (!available) onUnavailable();
  }, [available, onUnavailable]);
  return (
    <div ref={box} className={styles.layer}>
      <canvas ref={canvas} className={styles.canvas} aria-hidden="true" />
    </div>
  );
}

/** Flat diamonds: the tint, filled when reached and an outline while ahead. */
function FlatDiamonds({
  layout,
  replay,
}: {
  layout: (width: number, height: number) => DiamondMark[];
  replay: number;
}) {
  const box = useRef<HTMLDivElement>(null);
  const [size, setSize] = useState({ width: 0, height: 0 });
  useEffect(() => {
    const element = box.current;
    if (!element) return;
    const observer = new ResizeObserver(([entry]) =>
      setSize({ width: entry.contentRect.width, height: entry.contentRect.height }),
    );
    observer.observe(element);
    return () => observer.disconnect();
  }, []);
  const marks = size.width ? layout(size.width, size.height) : [];
  return (
    <div ref={box} className={styles.layer}>
      <svg key={replay} className={styles.flat} aria-hidden="true">
        {marks.map((mark) => {
          const colour = css(mark.tint);
          return (
            <path
              key={mark.key}
              className={styles.flatDiamond}
              d={diamondPath(mark.cx, mark.cy, mark.size, 2)}
              data-dim={mark.lit < 0.5}
              data-pattern={mark.pattern !== undefined}
              style={{
                fill: mark.level > 0.5 ? colour : "transparent",
                stroke: colour,
                animationDelay: `${0.25 + (mark.delay ?? 0)}s`,
              }}
            />
          );
        })}
      </svg>
    </div>
  );
}
