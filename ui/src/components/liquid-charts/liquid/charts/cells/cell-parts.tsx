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
  Figure,
  INK,
  MarkLayer,
  moveTooltip,
  type BarFrame,
  type BarMark,
  type Rgb,
  type StyleWarning,
  type Theme,
  type ValueFormat,
} from "@/components/liquid-charts/liquid/core";
import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";
import { useLiquidCells, type LiquidCell } from "@/components/liquid-charts/liquid/cells-material";
import { cssModule } from "@/components/liquid-charts/liquid/core/css-module";
import { cssRgb } from "@/components/liquid-charts/liquid/core/colour-scale";
import { useCellCanvas, type FlatCell } from "./cell-canvas";
import Roles from "@/components/liquid-charts/liquid/core/type.module.css";
import { GlassLayer } from "@/components/liquid-charts/liquid/glass";
import type { Segment } from "./calendar";
import Styles from "./cells.module.css";

const roles = cssModule(Roles, "lc-type-");
const styles = cssModule(Styles, "lc-cells-");

export const useIsomorphicLayoutEffect =
  typeof window === "undefined" ? useEffect : useLayoutEffect;

/** An element's content width, kept current; zero until measured. */
export function useWidth() {
  const ref = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  useIsomorphicLayoutEffect(() => {
    const element = ref.current;
    if (!element) return;
    const observer = new ResizeObserver(([entry]) => {
      const next = Math.floor(entry.contentRect.width);
      setWidth((w) => (w === next ? w : next));
    });
    observer.observe(element);
    return () => observer.disconnect();
  }, []);
  return [ref, width] as const;
}

/** Says once, in development, when an override breaks a style rule that protects the reading. */
export function useStyleWarnings(warnings: readonly StyleWarning[], name: string) {
  const said = useRef(new Set<string>());
  useEffect(() => {
    if (process.env.NODE_ENV === "production") return;
    for (const warning of warnings) {
      if (said.current.has(warning.rule)) continue;
      said.current.add(warning.rule);
      console.warn(`[liquid-charts] ${name}: ${warning.message} (rule: ${warning.rule})`);
    }
  }, [warnings, name]);
}

/** A figure in a value format. */
export function Fig({
  value,
  format,
  signed,
  className = roles.figureS,
}: {
  value: number;
  format: ValueFormat;
  signed?: boolean;
  className?: string;
}) {
  return (
    <Figure
      value={value}
      kind={format.kind}
      currency={format.currency}
      decimals={format.decimals}
      signed={signed ?? format.signed}
      unit={format.unit}
      className={className}
    />
  );
}

export const plain = (node: ReactNode, fallback: string) =>
  typeof node === "string" || typeof node === "number" ? String(node) : fallback;

/** Moves a tooltip to a point: it appears in place, then glides between items (as the core's
 * `placeTooltip` does). */
export function placeAt(tip: HTMLElement, { x, y }: { x: number; y: number }) {
  moveTooltip(tip, x, y);
}

/* ——— Mark layers ——— */

type CellLayerProps = {
  theme: Theme;
  replay: number;
  layout: (width: number, height: number) => FlatCell[];
  overlay?: (width: number, height: number) => readonly Segment[];
  radius?: number;
};

/**
 * Colour-coded cells as liquid: each cell a small pool in its exact scale colour, lit only at its
 * surface and walls (the core's `useLiquidCells`), or flat cells without WebGL.
 */
export function CellLayer(props: CellLayerProps) {
  const [webgl, setWebgl] = useState(true);
  if (!webgl) return <FlatCellLayer {...props} />;
  return <LiquidCellLayer {...props} onUnavailable={() => setWebgl(false)} />;
}

function LiquidCellLayer({
  theme,
  replay,
  layout,
  overlay,
  radius = 2,
  onUnavailable,
}: CellLayerProps & { onUnavailable: () => void }) {
  const { box, canvas, available } = useLiquidCells({
    theme,
    replay,
    layout: (w, h) =>
      layout(w, h).map(
        (cell): LiquidCell => ({
          key: cell.key,
          shape: {
            kind: "rect",
            rect: { left: cell.x, top: cell.y, right: cell.x + cell.w, bottom: cell.y + cell.h },
            radius: Math.min(radius, Math.min(cell.w, cell.h) * 0.24),
          },
          colour: cell.colour,
          lit: cell.lit,
          level: cell.level,
          etched: cell.etched,
          delay: cell.delay,
          focus: cell.focus,
          grow: cell.grow,
          mark: cell.mark,
        }),
      ),
    overlay: overlay
      ? (ctx, w, h, settled) => {
          const segments = overlay(w, h);
          if (!segments.length) return;
          const [r, g, b] = INK[theme];
          ctx.globalAlpha = settled;
          ctx.beginPath();
          for (const seg of segments) {
            ctx.moveTo(seg.x1, seg.y1);
            ctx.lineTo(seg.x2, seg.y2);
          }
          ctx.strokeStyle = `rgba(${Math.round(r * 255)},${Math.round(g * 255)},${Math.round(b * 255)},${theme === "dark" ? 0.85 : 0.8})`;
          ctx.lineWidth = 1.5;
          ctx.lineCap = "square";
          ctx.stroke();
          ctx.globalAlpha = 1;
        }
      : undefined,
  });
  useEffect(() => {
    if (!available) onUnavailable();
  }, [available, onUnavailable]);
  return (
    <div ref={box} className={styles.layer}>
      <canvas ref={canvas} className={styles.canvas} aria-hidden="true" />
    </div>
  );
}

/** Flat cells on a whole-pixel canvas. */
export function FlatCellLayer(props: {
  theme: Theme;
  replay: number;
  layout: (width: number, height: number) => FlatCell[];
  overlay?: (width: number, height: number) => readonly Segment[];
  radius?: number;
}) {
  const { box, canvas } = useCellCanvas(props);
  return (
    <div ref={box} className={styles.layer}>
      <canvas ref={canvas} className={styles.canvas} aria-hidden="true" />
    </div>
  );
}

type UnitLayerProps = {
  theme: Theme;
  replay: number;
  layout: (width: number, height: number) => BarMark[];
  onFrame?: (bars: BarFrame[], width: number, height: number) => void;
};

/** Glass units (the core's mark layer: `level` for a partial unit or a cell's value, `pattern`
 * for projected units), or flat units where the style says flat or WebGL is missing. */
export function UnitLayer({
  material,
  ...layer
}: UnitLayerProps & { material: "glass" | "flat"; onUnavailable?: () => void }) {
  return (
    <MarkLayer Glass={GlassLayer} {...layer} bevel={4} flatRadius={2} flat={material === "flat"} />
  );
}

/* ——— Legend keys ——— */

/** A legend key drawn as the cells draw it: today's quiet ring, or a range's outline. */
export function CellKey({
  mark,
  colour,
  etched,
  children,
}: {
  mark: "today" | "range" | "etched" | "glass" | "lanes";
  colour?: Rgb | string;
  /** The key's cell is etched too (today, still counting). */
  etched?: boolean;
  children: ReactNode;
}) {
  const swatch =
    colour === undefined ? undefined : typeof colour === "string" ? colour : cssRgb(colour);
  return (
    <span className={`${roles.caption} ${styles.key}`}>
      <span
        className={styles[`${mark}Swatch` as const]}
        data-etched={etched || undefined}
        style={swatch ? ({ "--swatch": swatch } as CSSProperties) : undefined}
      />
      {children}
    </span>
  );
}
