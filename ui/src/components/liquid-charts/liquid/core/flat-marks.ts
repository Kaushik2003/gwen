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
"use client";

/**
 * The flat path for the mark layer: bars in SVG. It draws wherever glass doesn't (a project without
 * the glass materials, a browser without WebGL2, bars too thin or too many for glass, or
 * flat asked for), with the same `BarMark` layout (`level` as a container filled to it, `marks` as
 * engraved lines, `pattern` as etched lanes from where it begins), the same springs, tone and level
 * easing and entrance, and the same `onFrame`, so labels and tooltips follow the bars exactly as
 * they do on glass.
 */

import { chartPresence, frameDelta, substeps } from "./presence";
import { GRAPHITE, mixRgb, type Theme } from "./colour";
import type { BarFrame, BarMark } from "./mark-types";
import { cssRgb as css } from "./colour-scale";
import { barRect, easeOut } from "./geometry";
import { useEffect, useRef } from "react";
import { ENTER } from "./format";

const SVG = "http://www.w3.org/2000/svg";
let patterns = 0;

/** The part of a bar's rect from its base to `share` of its length (a container's liquid). */
function baseRect(
  r: { left: number; top: number; right: number; bottom: number },
  mark: BarMark,
  share: number,
) {
  const w = r.right - r.left,
    h = r.bottom - r.top;
  if (mark.direction === "up")
    return { x: r.left, y: r.bottom - h * share, width: w, height: h * share };
  if (mark.direction === "down") return { x: r.left, y: r.top, width: w, height: h * share };
  if (mark.direction === "right") return { x: r.left, y: r.top, width: w * share, height: h };
  return { x: r.right - w * share, y: r.top, width: w * share, height: h };
}

/** The part of a bar's rect its pattern covers: from `start` (a share of the length, from the
 * base) to the tip. */
function etchRect(
  r: { left: number; top: number; right: number; bottom: number },
  mark: BarMark,
  start: number,
) {
  const w = r.right - r.left,
    h = r.bottom - r.top;
  if (mark.direction === "up") return { x: r.left, y: r.top, width: w, height: h * (1 - start) };
  if (mark.direction === "down")
    return { x: r.left, y: r.top + h * start, width: w, height: h * (1 - start) };
  if (mark.direction === "right")
    return { x: r.left + w * start, y: r.top, width: w * (1 - start), height: h };
  return { x: r.left, y: r.top, width: w * (1 - start), height: h };
}

/** Flat bars in an SVG: the glass layer's twin. Bars grow from their base on entrance (the frame's
 * `current` is the drawn length, so labels ride the growing tip) and spring on change. */
export function useFlatMarks({
  theme,
  layout,
  onFrame,
  replay = 0,
  radius = 3,
  entrance,
}: {
  theme: Theme;
  layout: (width: number, height: number) => BarMark[];
  onFrame?: (bars: BarFrame[], width: number, height: number) => void;
  replay?: number;
  /** Corner radius, px. */
  radius?: number;
  /** Read once on mount: when it is true, this layer takes over bars already on screen (from the
   * glass layer) and skips the entrance. Replays still play it. */
  entrance?: { readonly current: boolean };
}) {
  const box = useRef<HTMLDivElement>(null);
  const svg = useRef<SVGSVGElement>(null);
  const live = useRef({ theme, layout, onFrame, replay, radius, entrance, wake: () => {} });

  useEffect(() => {
    Object.assign(live.current, { theme, layout, onFrame, replay, radius });
    live.current.wake();
  });

  useEffect(() => {
    const root = box.current,
      surface = svg.current;
    if (!root || !surface) return;
    const shared = live.current;
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    let width = 0,
      height = 0;
    const observer = new ResizeObserver(() => {
      const r = root.getBoundingClientRect();
      width = r.width;
      height = r.height;
      surface.setAttribute("viewBox", `0 0 ${width} ${height}`);
      wake();
    });
    observer.observe(root);
    const state = new Map<
      string,
      { length: number; velocity: number; tone: number; level: number }
    >();
    /** Each bar is a group, bottom to top: the glass (or container), its liquid, the etched lanes,
     * then the engraved lines. */
    type Parts = {
      group: SVGGElement;
      node: SVGRectElement;
      liquid: SVGRectElement;
      etch: SVGRectElement;
      clip: SVGClipPathElement | null;
      shape: SVGRectElement | null;
      lines: SVGLineElement[];
    };
    const nodes = new Map<string, Parts>();
    const set = (element: Element, attributes: Record<string, string | number>) => {
      for (const name in attributes) element.setAttribute(name, String(attributes[name]));
    };
    // Etched lanes for patterned bars: light diagonals over the bar's own colour.
    const patternId = `flat-etch-${++patterns}`;
    let clips = 0;
    const defs = document.createElementNS(SVG, "defs");
    defs.innerHTML = `<pattern id="${patternId}" width="5" height="5" patternUnits="userSpaceOnUse" patternTransform="rotate(45)"><rect width="1.6" height="5" fill="#fff"/></pattern>`;
    surface.appendChild(defs);
    const presence = chartPresence(root, () => wake(), { redraw });
    let replayed = shared.replay,
      start = shared.entrance?.current ? -Infinity : presence.clock(performance.now()),
      raf = 0,
      last = 0;

    function step(frameTime: number) {
      raf = 0;
      if (!width || !height) return;
      // Offscreen or in a hidden tab: keep everything as it is and draw nothing until it is back.
      // Time runs on the presence clock, held meanwhile.
      if (!presence.active) {
        last = 0;
        return;
      }
      const now = presence.clock(frameTime);
      // The real time since the last frame; past a quarter second (or in a settled frame, for
      // print and export) the motion lands at its end instead of playing in slow motion.
      const { dt, snap } = frameDelta(now, last);
      const jump = snap || presence.settling;
      last = now;
      if (shared.replay !== replayed) {
        replayed = shared.replay;
        start = now;
      }
      const elapsed = (now - start) / 1000;
      const graphite = GRAPHITE[shared.theme];
      let moving = false;
      const seen = new Set<string>();
      const frames: BarFrame[] = shared.layout(width, height).map((target) => {
        const level = Math.max(0, Math.min(1, target.level ?? 1));
        let bar = state.get(target.key);
        if (!bar) {
          bar = { length: target.length, velocity: 0, tone: target.lit, level };
          state.set(target.key, bar);
        }
        if (reduced || jump) {
          bar.length = target.length;
          bar.velocity = 0;
          bar.tone = target.lit;
          bar.level = level;
        } else {
          for (let k = 0, n = substeps(dt); k < n; k++) {
            bar.velocity += (110 * (target.length - bar.length) - 21 * bar.velocity) * (dt / n);
            bar.length += bar.velocity * (dt / n);
          }
          if (Math.abs(target.length - bar.length) < 0.3 && Math.abs(bar.velocity) < 1) {
            bar.length = target.length;
            bar.velocity = 0;
          } else moving = true;
          bar.tone += (target.lit - bar.tone) * (1 - Math.exp(-11 * dt));
          if (Math.abs(target.lit - bar.tone) > 0.004) moving = true;
          else bar.tone = target.lit;
          bar.level += (level - bar.level) * (1 - Math.exp(-7 * dt));
          if (Math.abs(level - bar.level) > 0.002) moving = true;
          else bar.level = level;
        }
        const entrance = reduced
          ? 1
          : easeOut((elapsed - ENTER.delay - (target.delay ?? 0)) / ENTER.mark);
        if (entrance < 1) moving = true;
        // A bar grows from its base; a container stands at its full length while its liquid pours
        // in, as on glass.
        const container = bar.level < 0.999;
        const drawn = Math.max(0, bar.length) * (container ? 1 : entrance);

        seen.add(target.key);
        let parts = nodes.get(target.key);
        if (!parts) {
          const group = document.createElementNS(SVG, "g");
          const node = document.createElementNS(SVG, "rect");
          const liquid = document.createElementNS(SVG, "rect");
          const etch = document.createElementNS(SVG, "rect");
          etch.setAttribute("fill", `url(#${patternId})`);
          group.append(node, liquid, etch);
          surface!.appendChild(group);
          parts = { group, node, liquid, etch, clip: null, shape: null, lines: [] };
          nodes.set(target.key, parts);
        }
        const r = barRect(target, drawn);
        const w = r.right - r.left,
          h = r.bottom - r.top;
        const k = Math.max(0, Math.min(shared.radius, w / 2, h / 2));
        const colour = mixRgb(graphite, target.tint, bar.tone);
        const shown = drawn > 0.25;
        parts.group.setAttribute("opacity", shown ? "1" : "0");
        // A container is quiet glass up to its limit; the liquid inside is the value.
        set(parts.node, {
          x: r.left,
          y: r.top,
          width: w,
          height: h,
          rx: k,
          fill: css(colour),
          "fill-opacity": container ? 0.14 : 1,
          stroke: container ? css(colour) : "none",
          "stroke-opacity": 0.5,
          "stroke-width": 1,
        });
        const liquid = baseRect(r, target, container ? bar.level * entrance : 0);
        set(parts.liquid, {
          ...liquid,
          rx: Math.min(k, liquid.width / 2, liquid.height / 2),
          fill: css(colour),
          opacity: container ? 1 : 0,
        });

        const start =
          target.pattern === true ? 0 : typeof target.pattern === "number" ? target.pattern : null;
        if (start !== null && start < 1 && shown) {
          if (!parts.clip || !parts.shape) {
            // The lanes are clipped to the bar's own rounded shape.
            parts.clip = document.createElementNS(SVG, "clipPath");
            parts.clip.id = `${patternId}-${++clips}`;
            parts.shape = document.createElementNS(SVG, "rect");
            parts.clip.appendChild(parts.shape);
            defs.appendChild(parts.clip);
            parts.etch.setAttribute("clip-path", `url(#${parts.clip.id})`);
          }
          set(parts.shape, { x: r.left, y: r.top, width: w, height: h, rx: k });
          const e = etchRect(r, target, Math.max(0, start));
          set(parts.etch, {
            x: e.x,
            y: e.y,
            width: Math.max(0, e.width),
            height: Math.max(0, e.height),
            opacity: (shared.theme === "dark" ? 0.3 : 0.42) * entrance,
          });
        } else parts.etch.setAttribute("opacity", "0");

        // Engraved references: bright where the liquid has passed them, frost before.
        const marks = (target.marks ?? []).slice(0, 4);
        while (parts.lines.length < marks.length) {
          const line = document.createElementNS(SVG, "line");
          parts.group.appendChild(line);
          parts.lines.push(line);
        }
        parts.lines.forEach((line, i) => {
          // A graduation ring keeps its +4; drawn flat, it is a faint line, as quiet as in the glass.
          const ring = marks[i] !== undefined && marks[i] > 3.5;
          const m = ring ? marks[i] - 4 : marks[i];
          if (m === undefined || !shown) return line.setAttribute("opacity", "0");
          const passed = m <= (container ? bar.level : 1) + 1e-6;
          const vertical = target.direction === "up" || target.direction === "down";
          const at =
            target.direction === "up"
              ? r.bottom - h * m
              : target.direction === "down"
                ? r.top + h * m
                : target.direction === "right"
                  ? r.left + w * m
                  : r.right - w * m;
          set(line, {
            x1: vertical ? r.left + 1 : at,
            x2: vertical ? r.right - 1 : at,
            y1: vertical ? at : r.top + 1,
            y2: vertical ? at : r.bottom - 1,
            stroke: passed ? css(mixRgb(colour, [1, 1, 1], 0.7)) : "#fff",
            "stroke-width": 1.5,
            "stroke-linecap": "round",
            opacity: (passed ? 0.95 : 0.45) * (ring ? 0.25 : 1) * entrance,
          });
        });
        return { ...target, current: drawn, tone: bar.tone, fill: entrance * bar.level };
      });
      for (const [key, parts] of nodes)
        if (!seen.has(key)) {
          parts.group.remove();
          parts.clip?.remove();
          nodes.delete(key);
          state.delete(key);
        }
      shared.onFrame?.(frames, width, height);
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
      for (const parts of nodes.values()) parts.group.remove();
      defs.remove();
      shared.wake = () => {};
    };
  }, []);

  return { box, svg };
}
