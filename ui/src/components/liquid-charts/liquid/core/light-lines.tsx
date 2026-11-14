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

import { pixelRatio } from "@/components/liquid-charts/liquid/core/render-scale";
import { chartPresence, frameDelta, type ChartPresence } from "./presence";
import { cssToRgb, type Rgb, type Theme } from "./colour";
import { useEffect, useRef } from "react";
import { cssModule } from "./css-module";
import Styles from "./core.module.css";

const styles = cssModule(Styles, "lc-core-");

/**
 * Lines as light: each series is light travelling through a fibre. On dark cards a white-hot core
 * sits in the series colour with a soft bloom, and crossing lines add up; on light cards the line
 * is rich ink with a soft tinted shadow, so nothing washes out. Light is emissive, so colours stay
 * exact. Optional touches carry meaning: the stretch up to the pointer burns brighter (scrub), a
 * forecast is dashed light that dims, a single series casts a short wake of light beneath it (it
 * fades long before the baseline, so it never reads as an amount), and focus is shown by the
 * line itself: its light flares in a short stretch round the focused point, with a faint beam of
 * light rising through it. No dots or markers sit on the line.
 */

/** One intersecting series-paint clip, expressed in the line's frame coordinates. */
export type LightLineClip = {
  path: string;
  matrix: readonly [number, number, number, number, number, number];
  fillRule: CanvasFillRule;
};

export type LightLine = {
  key: string;
  /** SVG path data in frame px (any curve; gaps are separate subpaths). */
  path: string;
  /** CSS colour or Rgb. */
  colour: string | Rgb;
  /** Core width, px. Default 2. */
  width?: number;
  /** `dim` for out-of-focus series, `focus` for the one in focus. */
  state?: "normal" | "focus" | "dim";
  /** A continuous presence instead of `state` (0.32 dim, 1 normal, 1.15 focus), so a change of
   * focus can fade. */
  presence?: number;
  /** Frame x from which the line is a forecast: dashed light that dims. */
  forecastFrom?: number;
  /** A short wake of light beneath the line (single series). */
  wake?: boolean;
  /** Frame x of the latest point: the line's last stretch burns a little brighter. */
  tip?: number;
  /** Clips stroke, bloom and wake only; the native focus beam keeps its frame extent. */
  clips?: readonly LightLineClip[];
};

/** The focused point: the line flares there and a faint beam rises through it. */
export type LightFocus = { x: number; y: number; colour: string | Rgb };

const rgbOf = (colour: string | Rgb): Rgb =>
  typeof colour === "string" ? (cssToRgb(colour) ?? [0.61, 0.61, 0.61]) : colour;
const rgba = ([r, g, b]: Rgb, a: number) =>
  `rgba(${Math.round(r * 255)}, ${Math.round(g * 255)}, ${Math.round(b * 255)}, ${a})`;
const mix = (a: Rgb, b: Rgb, t: number): Rgb => [
  a[0] + (b[0] - a[0]) * t,
  a[1] + (b[1] - a[1]) * t,
  a[2] + (b[2] - a[2]) * t,
];
const WHITE: Rgb = [1, 1, 1];
const INK: Rgb = [0.08, 0.08, 0.08];

/** The stroke gradients of a context's last frame, in the order the frame asked for them. A frame
 * whose stops come out exactly as last frame's reuses that gradient instead of building it again
 * (a wake asks for 24 a frame, each ~150 stops). */
type Kept = { width: number; rgb: Rgb; alphas: Float64Array; gradient: CanvasGradient };
const kept = new WeakMap<CanvasRenderingContext2D, Kept[]>();
let scratch = new Float64Array(0);

/** Draws one frame of light lines onto a 2D context sized in CSS px (already scaled for DPR). */
export function drawLightLines(
  ctx: CanvasRenderingContext2D,
  {
    theme,
    lines,
    width,
    height,
    scrub = null,
    focus = null,
    reveal = 1,
    left = 0,
    origin = null,
  }: {
    theme: Theme;
    lines: readonly LightLine[];
    width: number;
    height: number;
    /** Frame x: lines are lit up to here, quieter beyond. */
    scrub?: number | null;
    focus?: LightFocus | null;
    /** Entrance, 0–1: how much of the plot (from `left`) is drawn. */
    reveal?: number;
    left?: number;
    /** Where frame (0, 0) sits on the canvas, for a canvas that bleeds past the frame. */
    origin?: { x: number; y: number } | null;
  },
) {
  const dark = theme === "dark";
  const gradients = kept.get(ctx) ?? [];
  kept.set(ctx, gradients);
  let slot = 0;
  ctx.clearRect(0, 0, width, height);
  ctx.save();
  if (origin) ctx.translate(origin.x, origin.y);
  // The entrance draws the lines in from the left.
  ctx.beginPath();
  ctx.rect(0, 0, left + (width - left) * Math.max(0, Math.min(1, reveal)), height);
  ctx.clip();
  ctx.lineCap = "round";
  ctx.lineJoin = "round";

  // A faint beam of light rising and falling through the focused point.
  if (focus) {
    const c = rgbOf(focus.colour);
    const peak = Math.max(0.02, Math.min(0.98, focus.y / Math.max(height, 1)));
    const beam = ctx.createLinearGradient(0, 0, 0, height);
    const tone = dark ? c : mix(c, INK, 0.35);
    // The beam fades within a third of the plot either side, so it reads as light at the point,
    // not as a crosshair.
    beam.addColorStop(Math.max(0, peak - 0.34), rgba(tone, 0));
    beam.addColorStop(peak, rgba(tone, dark ? 0.5 : 0.24));
    beam.addColorStop(Math.min(1, peak + 0.34), rgba(tone, 0));
    ctx.save();
    ctx.globalCompositeOperation = dark ? "lighter" : "source-over";
    ctx.fillStyle = beam;
    ctx.fillRect(Math.round(focus.x) - 0.5, 0, 1, height);
    if (dark) {
      ctx.globalAlpha = 0.18;
      ctx.fillRect(Math.round(focus.x) - 3, 0, 6, height);
    }
    ctx.restore();
  }

  const smoothstep = (a: number, b: number, x: number) => {
    const t = Math.max(0, Math.min(1, (x - a) / (b - a)));
    return t * t * (3 - 2 * t);
  };

  for (const line of lines) {
    if (line.clips?.length) {
      ctx.save();
      for (const clip of line.clips) {
        const [a, b, c, d, e, f] = clip.matrix;
        const path = new Path2D();
        path.addPath(new Path2D(clip.path), { a, b, c, d, e, f });
        ctx.clip(path, clip.fillRule);
      }
    }
    const colour = rgbOf(line.colour);
    const path = new Path2D(line.path);
    const w = line.width ?? 2;
    const presence =
      line.presence ?? (line.state === "dim" ? 0.32 : line.state === "focus" ? 1.15 : 1);
    const ink = dark ? colour : mix(colour, INK, 0.18);
    const end = line.forecastFrom ?? width;

    // The line's strength along x, as one smooth profile: lit up to the scrub and quieter beyond
    // (a soft ramp, not a cut), with a flare round the focused point and a glow at the latest one.
    const tipX =
      line.tip !== undefined && !(focus && Math.abs(focus.x - line.tip) < 40) ? line.tip : null;
    const strengthAt = (x: number) => {
      let value = scrub === null ? 1 : 1.15 - 0.65 * smoothstep(scrub - 14, scrub + 14, x);
      if (focus && line.state !== "dim")
        value += (line.state === "focus" ? 1.05 : 0.9) * Math.exp(-(((x - focus.x) / 17) ** 2));
      if (tipX !== null) value += 0.55 * Math.exp(-(((x - (tipX - 6)) / 13) ** 2));
      return value * presence;
    };
    const samples: [number, number][] = [];
    for (let x = 0; x <= width; x += 4) samples.push([x, strengthAt(x)]);
    /** A stroke style whose alpha follows the profile (times `scale`, offset by `over`), optionally
     * faded in from the plot's left and out before the forecast (`soft`, for the wake). */
    const along = (rgb: Rgb, scale: number, over = 0, soft = false) => {
      const last = gradients[slot];
      let same =
        last !== undefined &&
        last.width === width &&
        last.alphas.length === samples.length &&
        last.rgb[0] === rgb[0] &&
        last.rgb[1] === rgb[1] &&
        last.rgb[2] === rgb[2];
      if (scratch.length < samples.length) scratch = new Float64Array(samples.length * 2);
      for (let i = 0; i < samples.length; i++) {
        const [x, value] = samples[i];
        const edge = soft ? smoothstep(left, left + 28, x) * smoothstep(end, end - 28, x) : 1;
        const alpha = Math.max(0, Math.min(1, (value - over) * scale * edge));
        scratch[i] = alpha;
        if (same && last?.alphas[i] !== alpha) same = false;
      }
      if (same && last) {
        slot++;
        return last.gradient;
      }
      const gradient = ctx.createLinearGradient(0, 0, width, 0);
      for (let i = 0; i < samples.length; i++)
        gradient.addColorStop(samples[i][0] / width, rgba(rgb, scratch[i]));
      gradients[slot++] = { width, rgb, alphas: scratch.slice(0, samples.length), gradient };
      return gradient;
    };

    const paint = (from: number, to: number, dashed: boolean, quiet = 1) => {
      ctx.save();
      ctx.beginPath();
      const x0 = from <= 0 ? -60 : from,
        x1 = to >= width ? width + 60 : to;
      ctx.rect(x0, -60, x1 - x0, height + 120);
      ctx.clip();
      if (dashed) ctx.setLineDash([5, 5]);
      ctx.globalCompositeOperation = dark ? "lighter" : "source-over";
      // Wake: light falling onto the card beneath the line, gone within a few px.
      if (line.wake && !dashed) {
        for (let k = 1; k <= 24; k++) {
          ctx.save();
          ctx.translate(0, k * 0.85);
          ctx.strokeStyle = along(colour, (dark ? 0.068 : 0.026) * (1 - k / 25) * quiet, 0, true);
          ctx.lineWidth = w + 2;
          ctx.stroke(path);
          ctx.restore();
        }
      }
      // Bloom on dark; a soft tinted shadow on light. The shadow follows the stroke's alpha.
      ctx.shadowColor = rgba(colour, dark ? 1 : 0.36);
      ctx.shadowBlur = dark ? 14 : 8;
      ctx.shadowOffsetY = dark ? 0 : 2;
      ctx.strokeStyle = along(ink, (dark ? 0.85 : 0.95) * quiet);
      ctx.lineWidth = w + (dark ? 0.6 : 0.3);
      ctx.stroke(path);
      ctx.shadowBlur = 0;
      ctx.shadowOffsetY = 0;
      if (dark) {
        // The white-hot core, and where the profile passes full strength, a second pass so the
        // line burns brighter instead of just saturating.
        ctx.strokeStyle = along(mix(colour, WHITE, 0.62), 0.9 * quiet);
        ctx.lineWidth = Math.max(0.8, w * 0.45);
        ctx.stroke(path);
        ctx.shadowColor = rgba(colour, 1);
        ctx.shadowBlur = 18;
        ctx.strokeStyle = along(mix(colour, WHITE, 0.72), 0.75 * quiet, 1);
        ctx.lineWidth = Math.max(1, w * 0.6);
        ctx.stroke(path);
        ctx.shadowBlur = 0;
      } else {
        // A fine highlight along the top, and a darker, slightly thicker ink where it burns.
        ctx.strokeStyle = along(mix(colour, INK, 0.3), 0.55 * quiet, 1);
        ctx.lineWidth = w + 1;
        ctx.stroke(path);
        // The highlight rides the top edge and stays soft; in the middle of a thin line it read
        // as a hollow double stroke.
        ctx.translate(0, -(w * 0.5 - 0.3));
        ctx.strokeStyle = along(WHITE, 0.22 * quiet);
        ctx.lineWidth = 0.5;
        ctx.stroke(path);
      }
      ctx.restore();
    };

    paint(0, end, false);
    if (line.forecastFrom !== undefined) paint(end, width, true, 0.6);
    if (line.clips?.length) ctx.restore();
  }

  gradients.length = slot;
  ctx.restore();
}

/** Light lines on a canvas you drive yourself (for charts that redraw every animation frame):
 * `resize` sizes it to whole CSS pixels for the device's pixel ratio and repaints the last frame
 * at once (so the canvas is never shown blank), `draw` paints a frame. */
export function createLightLines(canvas: HTMLCanvasElement) {
  const ctx = canvas.getContext("2d");
  type Scene = Omit<Parameters<typeof drawLightLines>[1], "width" | "height">;
  let width = 0,
    height = 0,
    ratio = 1,
    last: Scene | null = null;
  const draw = (scene: Scene) => {
    last = scene;
    if (!ctx || !width || !height) return false;
    ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
    drawLightLines(ctx, { ...scene, width, height });
    return true;
  };
  return {
    resize(nextWidth: number, nextHeight: number) {
      ratio = pixelRatio(2);
      width = Math.round(nextWidth);
      height = Math.round(nextHeight);
      canvas.width = Math.max(1, width * ratio);
      canvas.height = Math.max(1, height * ratio);
      canvas.style.width = `${width}px`;
      canvas.style.height = `${height}px`;
      if (last) draw(last);
    },
    draw,
  };
}

/**
 * A canvas of light lines for a chart frame: whole-pixel sized, drawn in from the left on entrance
 * (and again on `replay`), the focus easing to the focused point, the scrub easing to the pointer.
 * Reduced motion draws everything at once.
 */
export function LightLineLayer({
  theme,
  lines,
  width,
  height,
  scrub = null,
  focus = null,
  left = 0,
  replay = 0,
}: {
  theme: Theme;
  lines: readonly LightLine[];
  /** Frame size, CSS px. */
  width: number;
  height: number;
  scrub?: number | null;
  focus?: LightFocus | null;
  /** Plot's left edge, where the entrance starts. */
  left?: number;
  replay?: number;
}) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const live = useRef({ theme, lines, scrub, focus, left, wake: () => {}, redraw: () => {} });
  // The entrance: when it started on the presence clock, and for which replay. It outlives a
  // resize, which only redraws.
  const entrance = useRef<{
    presence: ChartPresence | null;
    replay: number;
    started: number | null;
  }>({ presence: null, replay, started: null });

  useEffect(() => {
    Object.assign(live.current, { theme, lines, scrub, focus, left });
    live.current.wake();
  });

  // The lines draw in when the chart is about a third visible, and rest while it is offscreen.
  useEffect(() => {
    const surface = canvas.current;
    if (!surface) return;
    const run = entrance.current;
    const presence = chartPresence(surface, () => live.current.wake(), {
      redraw: () => live.current.redraw(),
    });
    run.presence = presence;
    return () => {
      presence.stop();
      run.presence = null;
    };
  }, []);

  useEffect(() => {
    const surface = canvas.current;
    const ctx = surface?.getContext("2d");
    if (!surface || !ctx || !width || !height) return;
    const shared = live.current;
    const run = entrance.current;
    const presence = run.presence;
    if (run.replay !== replay) {
      run.replay = replay;
      run.started = null;
    }
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    const ratio = pixelRatio(2);
    const w = Math.round(width),
      h = Math.round(height);
    surface.width = w * ratio;
    surface.height = h * ratio;
    surface.style.width = `${w}px`;
    surface.style.height = `${h}px`;
    let raf = 0,
      eased = { scrub: shared.scrub, x: shared.focus?.x ?? 0, y: shared.focus?.y ?? 0 },
      last = 0;
    function step(frameTime: number) {
      raf = 0;
      if (presence && !presence.active) {
        last = 0;
        return;
      }
      const now = presence ? presence.clock(frameTime) : frameTime;
      // A settled frame (print, export) shows the lines drawn in, with nothing left to ease.
      const settling = !!presence?.settling;
      if (run.started === null && (!presence || presence.entered))
        run.started = settling ? -Infinity : now;
      const since = run.started === null ? 0 : now - run.started;
      // The real time since the last frame; past a quarter second the easing lands at once.
      const { dt, snap } = frameDelta(now, last);
      last = now;
      const reveal =
        reduced || settling ? 1 : Math.min(1, 1 - (1 - Math.min(1, (since - 120) / 900)) ** 3);
      const k = reduced || settling || snap ? 1 : 1 - Math.exp(-18 * dt);
      let moving = reveal < 1;
      if (shared.scrub === null || eased.scrub === null) eased.scrub = shared.scrub;
      else {
        eased.scrub += (shared.scrub - eased.scrub) * k;
        if (Math.abs(shared.scrub - eased.scrub) > 0.3) moving = true;
      }
      if (shared.focus) {
        if (!eased.x && !eased.y) eased = { ...eased, x: shared.focus.x, y: shared.focus.y };
        eased.x += (shared.focus.x - eased.x) * k;
        eased.y += (shared.focus.y - eased.y) * k;
        if (Math.hypot(shared.focus.x - eased.x, shared.focus.y - eased.y) > 0.3) moving = true;
      }
      ctx!.setTransform(ratio, 0, 0, ratio, 0, 0);
      drawLightLines(ctx!, {
        theme: shared.theme,
        lines: shared.lines,
        width: w,
        height: h,
        scrub: eased.scrub,
        focus: shared.focus ? { ...shared.focus, x: eased.x, y: eased.y } : null,
        reveal,
        left: shared.left,
      });
      if (moving && (!presence || presence.running)) raf = requestAnimationFrame(step);
      else last = 0;
    }
    function wake() {
      if (!raf) raf = requestAnimationFrame(step);
    }
    shared.wake = wake;
    shared.redraw = () => {
      cancelAnimationFrame(raf);
      raf = 0;
      step(performance.now());
    };
    // Draw now: sizing the canvas cleared it, and waiting a frame would show it blank.
    step(performance.now());
    return () => {
      cancelAnimationFrame(raf);
      shared.wake = () => {};
      shared.redraw = () => {};
    };
  }, [width, height, replay]);

  return <canvas ref={canvas} className={styles.lightLines} aria-hidden="true" />;
}
