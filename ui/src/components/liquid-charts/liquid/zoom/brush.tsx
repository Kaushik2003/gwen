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

/* Icons: Solar (restart-linear) by 480 Design, CC BY 4.0,
 * https://www.figma.com/community/file/1166831539721848736 */

import {
  brushHit,
  brushLabels,
  dragSpan,
  isZoomed,
  keySpan,
  ordered,
  panSpan,
  pxOf,
  springValues,
  valueAt,
  wheelScale,
  zoomSpan,
  type BrushDrag,
  type BrushGrip,
  type Snap,
  type Span,
  type SpringState,
} from "./zoom";
import {
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  useSyncExternalStore,
  version,
  type CSSProperties,
  type KeyboardEvent,
  type PointerEvent,
  type ReactNode,
} from "react";
import { LightLineLayer, type LightLine } from "@/components/liquid-charts/liquid/core/light-lines";
import { GRAPHITE, type Theme } from "@/components/liquid-charts/liquid/core/colour";
import type { Insets, Rect } from "@/components/liquid-charts/liquid/core/geometry";
import { cssModule } from "@/components/liquid-charts/liquid/core/css-module";
import { toggleStyles } from "@/components/liquid-charts/liquid/core/toggles";
import { useChartFrame } from "@/components/liquid-charts/liquid/core/frame";
import Roles from "@/components/liquid-charts/liquid/core/type.module.css";
import Core from "@/components/liquid-charts/liquid/core/core.module.css";
import Styles from "./brush.module.css";

const roles = cssModule(Roles, "lc-type-");
const core = cssModule(Core, "lc-core-");
const styles = cssModule(Styles, "lc-brush-");

/**
 * Choosing a range and looking closer: the brush, overview strips, springing domains and zoom.
 *
 * - `Brush`: an x-range brush over any ChartFrame plot. Drag across the plot to choose a range,
 *   drag a handle to resize it, drag the window to move it; a click outside, Escape or a double
 *   click clears it. It is one focusable slider for the keyboard (Left and Right move the window,
 *   Shift with them resizes it, Home and End take it to the ends), snaps to whole bands, points or
 *   the nearest data x, and reads its range aloud. Outside the window the chart goes graphite (out
 *   of focus) under a quiet veil; the window stays clear; the handles are small lit capsules with
 *   a grip, and the range's bounds read beside them.
 * - `BrushStrip`: a compact overview of the whole series (light lines, or any chart you pass) with
 *   a brush on it, to drive a main chart's x domain.
 * - `useSpringValues`: numbers (a domain's ends) riding a critically damped spring, so axes and
 *   marks glide to a new range; reduced motion settles at once.
 * - `useZoom`, `ZoomSurface` and `ResetZoom`: pinch or ⌘/Ctrl-scroll to zoom about the pointer,
 *   drag to pan, double-click or `0` to reset, with a "Reset zoom" key while zoomed (zoom-in,
 *   zoom-out and whole-view keys are `ZoomControls`, in zoom-controls.tsx).
 *
 * The maths (clamping, zooming about a point, snapping, the drag and keyboard rules) is pure and
 * lives in `zoom.ts`.
 */

/* ——— Motion ——— */

const REDUCED = "(prefers-reduced-motion: reduce)";
const subscribeReduced = (callback: () => void) => {
  const media = window.matchMedia(REDUCED);
  media.addEventListener("change", callback);
  return () => media.removeEventListener("change", callback);
};

/** True when the reader prefers reduced motion (false on the server). */
export function useReducedMotion() {
  return useSyncExternalStore(
    subscribeReduced,
    () => window.matchMedia(REDUCED).matches,
    () => false,
  );
}

const COARSE = "(pointer: coarse)";
const subscribeCoarse = (callback: () => void) => {
  const media = window.matchMedia(COARSE);
  media.addEventListener("change", callback);
  return () => media.removeEventListener("change", callback);
};
/** True on touch screens, where handles answer to a wider reach. */
function useCoarsePointer() {
  return useSyncExternalStore(
    subscribeCoarse,
    () => window.matchMedia(COARSE).matches,
    () => false,
  );
}

/**
 * Numbers that spring to `target` (a domain's ends, a view): returns their current values and
 * whether they are still moving. `immediate` follows the target exactly (direct manipulation, such
 * as a pan under the finger); reduced motion always does. Each number rests within `rest[i]`.
 */
export function useSpringValues(
  target: readonly number[],
  { immediate = false, rest }: { immediate?: boolean; rest?: readonly number[] } = {},
): { values: readonly number[]; moving: boolean } {
  const reduced = useReducedMotion();
  const key = target.join(",");
  const restKey = rest?.join(",") ?? "";
  const [values, setValues] = useState<readonly number[]>(() => [...target]);
  const state = useRef<SpringState>({ value: [...target], velocity: target.map(() => 0) });
  const snap = immediate || reduced;

  useEffect(() => {
    const goal = key ? key.split(",").map(Number) : [];
    const tolerance = restKey ? restKey.split(",").map(Number) : undefined;
    if (snap || state.current.value.length !== goal.length) {
      state.current = { value: [...goal], velocity: goal.map(() => 0) };
      const raf = requestAnimationFrame(() => setValues(goal));
      return () => cancelAnimationFrame(raf);
    }
    let raf = 0,
      last = 0;
    const step = (now: number) => {
      const dt = last ? Math.min(0.05, (now - last) / 1000) : 1 / 60;
      last = now;
      const moving = springValues(state.current, goal, dt, { rest: tolerance });
      setValues([...state.current.value]);
      if (moving) raf = requestAnimationFrame(step);
    };
    raf = requestAnimationFrame(step);
    return () => cancelAnimationFrame(raf);
  }, [key, restKey, snap]);

  const shown = snap ? target : values;
  return { values: shown, moving: !snap && values.join(",") !== key };
}

/* ——— The brush ——— */

/** How a bound reads beside its handle: a Figure, a date, any node. */
export type BrushFormat = (value: number, edge: "from" | "to") => ReactNode;

export type BrushProps = {
  /** The whole x extent the brush runs over, in domain units (indices, band edges, ms…). */
  extent: Span;
  /** The chosen range, controlled; null for none. */
  range?: Span | null;
  /** The starting range when uncontrolled. Default none. */
  defaultRange?: Span | null;
  /** Every change, live while dragging; null when cleared. */
  onRangeChange?: (range: [number, number] | null) => void;
  /** `integer` snaps to whole bands or point indices; a sorted list snaps to the nearest data x. */
  snap?: Snap;
  /** The narrowest range, in domain units. Default one snap unit (or a fiftieth of the extent). */
  minSpan?: number;
  /** One keyboard step, in domain units. Default one snap unit (or a fiftieth of the extent). */
  step?: number;
  /** The handles' light: a series colour from the config. Default the chart ink. */
  colour?: string;
  /** A bound as it reads beside its handle. Default the value as a number. */
  format?: BrushFormat;
  /** The range in words, for the slider's value and the live readout ("1 Jul to 30 Sep"). */
  describe?: (range: Span | null) => string;
  /** With no range: `handles` rests a handle at each end (overview strips), `none` shows nothing. */
  idle?: "handles" | "none";
  /** The bound readouts beside the handles. Default true. */
  labels?: boolean;
  /** The brush's accessible name. */
  label: string;
  className?: string;
};

/** The range held here unless `range` is passed. */
function useRangeState(
  range: Span | null | undefined,
  defaultRange: Span | null | undefined,
  onChange?: (range: [number, number] | null) => void,
) {
  const [inner, setInner] = useState<Span | null>(defaultRange ?? null);
  const current = range === undefined ? inner : range;
  const set = useCallback(
    (next: [number, number] | null) => {
      if (range === undefined) setInner(next);
      onChange?.(next);
    },
    [range, onChange],
  );
  return [current, set] as const;
}

const sameSpan = (a: Span | null, b: Span | null) =>
  a === b || (!!a && !!b && a[0] === b[0] && a[1] === b[1]);

/** A typical gap between snap points, for keyboard steps and the minimum span. */
function snapUnit(snap: Snap, extent: Span) {
  const full = Math.abs(extent[1] - extent[0]) || 1;
  if (snap === "integer") return 1;
  if (Array.isArray(snap) && snap.length > 1) {
    const gaps = snap
      .slice(1)
      .map((v, i) => v - snap[i])
      .filter((g) => g > 0)
      .sort((a, b) => a - b);
    return gaps[Math.floor(gaps.length / 2)] ?? full / 50;
  }
  return full / 50;
}

/**
 * An x-range brush over the plot of the ChartFrame it sits in. Drag across the plot to choose a
 * range, a handle to resize it, the window to move it; a click outside, Escape or a double click
 * clears it. To a keyboard it is one slider; it snaps to bands, points or the nearest data x, and
 * reads its range aloud. `BrushStrip` puts one on an overview of the whole series.
 */
export function Brush(props: BrushProps) {
  const { plot } = useChartFrame();
  if (plot.right - plot.left < 2) return null;
  return <BrushView {...props} plot={plot} variant="plot" />;
}

function BrushView({
  plot,
  variant,
  extent,
  range: rangeProp,
  defaultRange,
  onRangeChange,
  snap,
  minSpan: minProp,
  step: stepProp,
  colour,
  format,
  describe,
  idle = "none",
  labels = true,
  label,
  className,
  backdrop = true,
}: BrushProps & {
  plot: Rect;
  variant: "plot" | "strip";
  /** Grey out what lies under the veil (a backdrop filter); off when the marks outside are drawn
   * graphite already. */
  backdrop?: boolean;
}) {
  const [range, setRange] = useRangeState(rangeProp, defaultRange, onRangeChange);
  const coarse = useCoarsePointer();
  const unit = snapUnit(snap, extent);
  const minSpan = minProp ?? unit;
  const step = stepProp ?? unit;
  const width = plot.right - plot.left;
  const height = plot.bottom - plot.top;
  const span: Span = ordered(extent);
  const px = (value: number) => pxOf(value, [0, width], span);
  const value = (x: number) => valueAt(x, [0, width], span);
  const current = range ? ordered(range) : null;
  const edges: Span | null = current ? [px(current[0]), px(current[1])] : null;
  const resting: Span | null = edges ?? (idle === "handles" ? [0, width] : null);
  const narrow = width < 520;
  const reach = coarse || narrow ? 22 : 12;

  const root = useRef<HTMLDivElement>(null);
  const fromLabel = useRef<HTMLSpanElement>(null);
  const toLabel = useRef<HTMLSpanElement>(null);
  const bothLabel = useRef<HTMLSpanElement>(null);
  const drag = useRef<(BrushDrag & { x: number; moved: boolean; id: number }) | null>(null);
  const swallowClick = useRef(false);
  const [grip, setGrip] = useState<"from" | "to" | "inside" | "outside" | null>(null);
  // The part being dragged (null at rest), as state so the look can follow it.
  const [dragGrip, setDragGrip] = useState<BrushGrip | null>(null);
  const dragging = dragGrip !== null;

  const emit = (next: [number, number] | null) => {
    if (!sameSpan(next, current)) setRange(next);
  };
  const local = (event: { clientX: number }) => {
    const r = root.current?.getBoundingClientRect();
    return r ? event.clientX - r.left : 0;
  };
  // With no range, only the resting handles (if any) can be taken; everywhere else starts anew.
  const hit = (x: number) => {
    if (edges) return brushHit(x, edges, reach);
    if (!resting) return "outside" as const;
    const near = brushHit(x, resting, reach);
    return near === "inside" ? ("outside" as const) : near;
  };

  const onPointerDown = (event: PointerEvent<HTMLDivElement>) => {
    if (event.button !== 0 || !event.isPrimary) return;
    const x = local(event);
    const g = hit(x);
    drag.current = {
      grip: g,
      origin: value(x),
      start: current ?? (g === "from" || g === "to" ? span : null),
      x,
      moved: false,
      id: event.pointerId,
    };
    event.currentTarget.setPointerCapture?.(event.pointerId);
    // The press focuses the brush in the ordinary way (so a focus ring shows for keys only).
    event.stopPropagation();
  };
  const onPointerMove = (event: PointerEvent<HTMLDivElement>) => {
    const d = drag.current;
    const x = local(event);
    if (!d || d.id !== event.pointerId) {
      const g = hit(x);
      if (g !== grip) setGrip(g);
      return;
    }
    event.stopPropagation();
    if (!d.moved && Math.abs(x - d.x) < 3) return;
    if (!d.moved) {
      d.moved = true;
      setDragGrip(d.grip);
    }
    emit(dragSpan(d, value(x), span, { minSpan, snap }));
  };
  const finish = (event: PointerEvent<HTMLDivElement>) => {
    const d = drag.current;
    if (!d || d.id !== event.pointerId) return;
    drag.current = null;
    setDragGrip(null);
    event.currentTarget.releasePointerCapture?.(event.pointerId);
    event.stopPropagation();
    swallowClick.current = d.moved || (d.grip === "outside" && !!current);
    // A click outside the window (no drag) clears it.
    if (!d.moved && d.grip === "outside" && current) emit(null);
  };
  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    const next = keySpan(current, event.key, {
      shift: event.shiftKey,
      extent: span,
      step,
      minSpan,
      snap,
    });
    if (next === undefined) return;
    event.preventDefault();
    event.stopPropagation();
    emit(next);
  };

  // The bound readouts sit beside their handles, or merge into one over a narrow window.
  useLayoutEffect(() => {
    const a = fromLabel.current,
      b = toLabel.current,
      both = bothLabel.current;
    if (!a || !b || !both) return;
    if (!edges) {
      for (const node of [a, b, both]) node.dataset.shown = "false";
      return;
    }
    const layout = brushLabels({
      from: edges[0],
      to: edges[1],
      widths: [a.offsetWidth, b.offsetWidth],
      merged: both.offsetWidth,
      bounds: [-plot.left + 2, width + 6],
      gap: 10,
    });
    if (layout.merged) {
      a.dataset.shown = b.dataset.shown = "false";
      both.dataset.shown = "true";
      both.style.transform = `translate(${Math.round(layout.left)}px, -50%)`;
    } else {
      both.dataset.shown = "false";
      a.dataset.shown = b.dataset.shown = "true";
      a.style.transform = `translate(${Math.round(layout.from)}px, -50%)`;
      b.style.transform = `translate(${Math.round(layout.to)}px, -50%)`;
    }
  });

  // The range in words for screen readers, once it settles.
  const words = describe?.(current) ?? defaultWords(current);
  const [spoken, setSpoken] = useState("");
  useEffect(() => {
    const timer = window.setTimeout(() => setSpoken(words), 350);
    return () => window.clearTimeout(timer);
  }, [words]);

  const fmt = format ?? ((v: number) => <span>{Math.round(v * 100) / 100}</span>);
  const handle = (edge: "from" | "to", x: number) => (
    <span
      className={styles.handle}
      data-edge={edge}
      data-active={dragGrip === edge}
      style={{ left: x }}
      aria-hidden="true"
    >
      <span className={styles.grip} />
    </span>
  );

  return (
    <div
      ref={root}
      className={`${styles.brush} ${className ?? ""}`}
      data-variant={variant}
      data-grip={dragGrip ?? grip}
      data-dragging={dragging}
      data-size={narrow || coarse ? "touch" : "fine"}
      role="slider"
      tabIndex={0}
      aria-label={label}
      aria-valuemin={span[0]}
      aria-valuemax={span[1]}
      aria-valuenow={current ? current[0] : span[0]}
      aria-valuetext={words}
      aria-orientation="horizontal"
      style={
        {
          left: plot.left,
          top: plot.top,
          width,
          height,
          "--brush-light": colour ?? "var(--chart-ink)",
        } as CSSProperties
      }
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={finish}
      onPointerCancel={finish}
      onPointerLeave={() => {
        if (!drag.current) setGrip(null);
      }}
      onClickCapture={(event) => {
        if (!swallowClick.current) return;
        swallowClick.current = false;
        event.stopPropagation();
      }}
      onDoubleClick={(event) => {
        event.stopPropagation();
        emit(null);
      }}
      onKeyDown={onKeyDown}
    >
      {edges && (
        <>
          <span
            className={styles.veil}
            data-side="left"
            data-backdrop={backdrop}
            style={{ left: 0, width: edges[0] }}
          />
          <span
            className={styles.veil}
            data-backdrop={backdrop}
            data-side="right"
            style={{ left: edges[1], width: Math.max(0, width - edges[1]) }}
          />
          <span
            className={styles.window}
            style={{ left: edges[0], width: Math.max(0, edges[1] - edges[0]) }}
          />
        </>
      )}
      {resting && handle("from", resting[0])}
      {resting && handle("to", resting[1])}
      {labels && (
        <>
          <span ref={fromLabel} className={`${styles.readout} ${roles.micro}`} aria-hidden="true">
            {current && fmt(current[0], "from")}
          </span>
          <span ref={toLabel} className={`${styles.readout} ${roles.micro}`} aria-hidden="true">
            {current && fmt(current[1], "to")}
          </span>
          <span ref={bothLabel} className={`${styles.readout} ${roles.micro}`} aria-hidden="true">
            {current && (
              <>
                {fmt(current[0], "from")}
                <span className={styles.dash}>–</span>
                {fmt(current[1], "to")}
              </>
            )}
          </span>
        </>
      )}
      <span className={core.srOnly} aria-live="polite">
        {spoken}
      </span>
    </div>
  );
}

const defaultWords = (range: Span | null) =>
  range
    ? `${Math.round(range[0] * 100) / 100} to ${Math.round(range[1] * 100) / 100}`
    : "Everything";

/* ——— The overview strip ——— */

/** One series on an overview strip: its values in x order, drawn as a thin light line. */
export type StripLine = {
  key: string;
  /** Any CSS colour (from the config). */
  colour: string;
  values: readonly (number | null)[];
  /** Each value's x in the extent's units. Default its index. */
  xs?: readonly number[];
};

export type BrushStripProps = BrushProps & {
  theme: Theme;
  /** The strip's height, px. Default 56 (64 on touch screens and narrow cards). */
  height?: number;
  /** Room round the overview's plot; match the main chart's left and right so the two line up. */
  insets?: Partial<Insets>;
  /** The whole series as thin light lines. */
  lines?: readonly StripLine[];
  /** Or an overview of your own (a tile chart), drawn under the brush and kept out of the tab
   * order; give it the same insets. A function is drawn twice: `quiet` (draw it graphite) outside
   * the window and `lit` inside it. A plain node is greyed outside the window by the veil (a
   * backdrop filter, so in browsers that support one). */
  children?: ReactNode | ((tone: "lit" | "quiet") => ReactNode);
};

/* The overview under the brush is inert: out of the tab order and the pointer's way. React 19 takes
 * `inert` as a boolean; React 18 has no such prop (nor its types) and writes it only from a
 * string, so each gets the value it reads. */
const INERT = { inert: version.startsWith("18.") ? "" : true } as object;

/**
 * A compact overview of the whole series with a brush on it: light lines (or any chart you pass)
 * that go graphite outside the window, handles resting at both ends until a range is chosen. Put it
 * under a main chart and feed the range to that chart's x domain.
 */
export function BrushStrip({
  theme,
  height: heightProp,
  insets: insetProp,
  lines = [],
  children,
  className,
  ...brush
}: BrushStripProps) {
  const box = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  useLayoutEffect(() => {
    const element = box.current;
    if (!element) return;
    const observer = new ResizeObserver(([entry]) => setWidth(entry.contentRect.width));
    observer.observe(element);
    return () => observer.disconnect();
  }, []);
  const [range, setRange] = useRangeState(brush.range, brush.defaultRange, brush.onRangeChange);
  const coarse = useCoarsePointer();
  const height = heightProp ?? (coarse || (width > 0 && width < 520) ? 64 : 56);
  const insets: Insets = { left: 0, right: 0, top: 6, bottom: 6, ...insetProp };
  const plot: Rect = {
    left: insets.left,
    top: insets.top,
    right: Math.max(insets.left + 1, width - insets.right),
    bottom: Math.max(insets.top + 1, height - insets.bottom),
  };
  const span = ordered(brush.extent);
  const lit: LightLine[] = stripLines(lines, span, plot);
  // Outside the window the same lines run in graphite (out of focus); inside, in their colours.
  const quiet: LightLine[] = lit.map((line) => ({
    ...line,
    colour: stripGraphite(theme),
    presence: 0.62,
  }));
  const at = range ? ordered(range).map((v) => pxOf(v, [plot.left, plot.right], span)) : null;
  const H = height + 40;
  const inside = at
    ? `inset(-20px ${Math.max(0, width - at[1])}px -20px ${Math.max(0, at[0])}px)`
    : undefined;
  const outside = at
    ? `polygon(evenodd, -40px -20px, ${width + 40}px -20px, ${width + 40}px ${H}px, -40px ${H}px, -40px -20px, ${at[0]}px -20px, ${at[0]}px ${H}px, ${at[1]}px ${H}px, ${at[1]}px -20px, ${at[0]}px -20px)`
    : undefined;

  return (
    <div
      ref={box}
      className={`${core.frame} ${roles.root} ${styles.strip} ${className ?? ""}`}
      data-scheme={theme}
      style={{ height }}
    >
      <div className={styles.track} style={{ left: plot.left, right: insets.right }} />
      {typeof children === "function" ? (
        <>
          {at && (
            <div
              className={styles.overview}
              style={{ clipPath: outside }}
              {...INERT}
              aria-hidden="true"
            >
              {children("quiet")}
            </div>
          )}
          <div
            className={styles.overview}
            style={{ clipPath: inside }}
            {...INERT}
            aria-hidden="true"
          >
            {children("lit")}
          </div>
        </>
      ) : (
        children && (
          <div className={styles.overview} {...INERT} aria-hidden="true">
            {children}
          </div>
        )
      )}
      {width > 0 && lit.length > 0 && (
        <>
          {at && (
            <div className={styles.overview} style={{ clipPath: outside }} aria-hidden="true">
              <LightLineLayer
                theme={theme}
                lines={quiet}
                width={width}
                height={height}
                left={plot.left}
              />
            </div>
          )}
          <div className={styles.overview} style={{ clipPath: inside }} aria-hidden="true">
            <LightLineLayer
              theme={theme}
              lines={lit}
              width={width}
              height={height}
              left={plot.left}
            />
          </div>
        </>
      )}
      {width > 0 && (
        <BrushView
          {...brush}
          range={range}
          defaultRange={undefined}
          onRangeChange={setRange}
          plot={plot}
          variant="strip"
          idle={brush.idle ?? "handles"}
          backdrop={lit.length === 0 && typeof children !== "function"}
        />
      )}
    </div>
  );
}

/** Graphite for overview lines outside the window: the core's out-of-focus tone, as CSS. */
const stripGraphite = (theme: Theme) => {
  const [r, g, b] = GRAPHITE[theme];
  return `rgb(${Math.round(r * 255)} ${Math.round(g * 255)} ${Math.round(b * 255)})`;
};

/** Overview lines as light-line paths in strip px: one shared y scale with a little air. */
function stripLines(lines: readonly StripLine[], span: Span, plot: Rect): LightLine[] {
  let lo = Infinity,
    hi = -Infinity;
  for (const line of lines)
    for (const v of line.values)
      if (v !== null && Number.isFinite(v)) {
        lo = Math.min(lo, v);
        hi = Math.max(hi, v);
      }
  if (lo > hi) return [];
  const pad = (hi - lo) * 0.12 || Math.abs(hi) * 0.1 || 1;
  const y0 = lo - pad,
    y1 = hi + pad;
  const top = plot.top + 2,
    bottom = plot.bottom - 2;
  const y = (v: number) => bottom - ((v - y0) / (y1 - y0)) * (bottom - top);
  const x = (v: number) => pxOf(v, [plot.left, plot.right], span);
  return lines.map((line) => {
    let path = "",
      pen = false;
    line.values.forEach((v, i) => {
      if (v === null || !Number.isFinite(v)) {
        pen = false;
        return;
      }
      const px = x(line.xs?.[i] ?? i);
      path += `${pen ? "L" : "M"}${px.toFixed(1)} ${y(v).toFixed(1)}`;
      pen = true;
    });
    return { key: line.key, path, colour: line.colour, width: 1.5, presence: 0.95 };
  });
}

/* ——— Zoom and pan ——— */

export type ZoomView = { x: Span; y: Span };
export type ZoomAxes = "x" | "y" | "xy";

export type Zoom = {
  /** Where the view is going (the whole extent when not zoomed). */
  target: ZoomView;
  /** The view now, springing towards the target. */
  view: ZoomView;
  /** The target shows less than everything. */
  zoomed: boolean;
  /** The view now is not everything (still zoomed, or springing back). */
  showing: boolean;
  /** The view is springing. */
  moving: boolean;
  axes: ZoomAxes;
  extent: ZoomView;
  /** Zooms the target by `scale` (below 1 in) about `anchor` (default the view's centre). */
  zoomBy: (scale: number, anchor?: { x?: number; y?: number }) => void;
  /** Moves the target by a distance in each axis's units; `immediate` skips the spring. */
  panBy: (dx: number, dy: number, immediate?: boolean) => void;
  /** Sets the target view (null for everything). */
  set: (view: ZoomView | null, immediate?: boolean) => void;
  reset: () => void;
};

/**
 * A zoomable view over an extent in each axis (in the axes' own units; pass log10 values for a log
 * axis). Zooming keeps within the extent and at least `minSpan` wide; the view springs to each new
 * target unless a change asks to be immediate (a pan under the finger). The extent may change with
 * the data: a view that no longer fits is clamped, and an unzoomed view follows it.
 */
export function useZoom({
  extent,
  axes = "xy",
  minSpan,
}: {
  extent: ZoomView;
  axes?: ZoomAxes;
  /** The narrowest view per axis. Default a fortieth of each extent. */
  minSpan?: { x?: number; y?: number };
}): Zoom {
  const [state, setState] = useState<{ view: ZoomView | null; immediate: boolean }>({
    view: null,
    immediate: false,
  });
  const ex = ordered(extent.x),
    ey = ordered(extent.y);
  const minX = minSpan?.x ?? (ex[1] - ex[0]) / 40;
  const minY = minSpan?.y ?? (ey[1] - ey[0]) / 40;
  const target: ZoomView = state.view
    ? { x: clampTo(state.view.x, ex, minX), y: clampTo(state.view.y, ey, minY) }
    : { x: ex, y: ey };
  const zoomed = isZoomed(target.x, ex) || isZoomed(target.y, ey);
  const rest = [ex, ex, ey, ey].map(([a, b]) => Math.max(1e-9, Math.abs(b - a) * 2e-4));
  const { values, moving } = useSpringValues([target.x[0], target.x[1], target.y[0], target.y[1]], {
    immediate: state.immediate,
    rest,
  });
  const view: ZoomView = { x: [values[0], values[1]], y: [values[2], values[3]] };
  const showing = isZoomed(view.x, ex, 1e-4) || isZoomed(view.y, ey, 1e-4);

  const set = (next: ZoomView | null, immediate = false) => {
    setState({
      view:
        next && (isZoomed(next.x, ex) || isZoomed(next.y, ey))
          ? { x: clampTo(next.x, ex, minX), y: clampTo(next.y, ey, minY) }
          : null,
      immediate,
    });
  };
  return {
    target,
    view,
    zoomed,
    showing,
    moving,
    axes,
    extent: { x: ex, y: ey },
    zoomBy: (scale, anchor) => {
      const ax = anchor?.x ?? (target.x[0] + target.x[1]) / 2;
      const ay = anchor?.y ?? (target.y[0] + target.y[1]) / 2;
      set({
        x: axes === "y" ? target.x : zoomSpan(target.x, scale, ax, ex, minX),
        y: axes === "x" ? target.y : zoomSpan(target.y, scale, ay, ey, minY),
      });
    },
    panBy: (dx, dy, immediate = false) =>
      set(
        {
          x: axes === "y" ? target.x : panSpan(target.x, dx, ex),
          y: axes === "x" ? target.y : panSpan(target.y, dy, ey),
        },
        immediate,
      ),
    set,
    reset: () => set(null),
  };
}

const clampTo = (span: Span, extent: Span, min: number): Span => {
  const [a, b] = ordered(span);
  const full = extent[1] - extent[0];
  const width = Math.min(full, Math.max(b - a, Math.min(min, full)));
  const centre = (a + b) / 2;
  let lo = Math.max(extent[0], Math.min(centre - width / 2, extent[1] - width));
  if (!Number.isFinite(lo)) lo = extent[0];
  return [lo, lo + width];
};

/** The zoom keys for a focused plot: `+`/`=` in, `-` out, `0` back to everything. Returns true
 * when the key was one of them. `anchor` is where to zoom about (a focused point), if anywhere. */
export function zoomKey(zoom: Zoom, key: string, anchor?: { x?: number; y?: number }) {
  if (key === "+" || key === "=") zoom.zoomBy(0.7, anchor);
  else if (key === "-" || key === "_") zoom.zoomBy(1 / 0.7, anchor);
  else if (key === "0") zoom.reset();
  else return false;
  return true;
}

/**
 * The surface that turns gestures into zoom over a ChartFrame's plot: pinch (or ⌘/Ctrl and the
 * wheel) zooms about the pointer, a sideways swipe or a drag pans while zoomed, a double click
 * resets, and two fingers pinch on touch screens. Pointer moves still reach the chart, so hover and
 * click work as before; a plain vertical wheel scrolls the page. How to zoom is part of the plot's
 * accessible description, never a line on the chart. Put it in the frame above the marks. `pad`
 * is the px the chart keeps between its plot edge and its scale's ends, if any.
 */
export function ZoomSurface({ zoom, pad = 0 }: { zoom: Zoom; pad?: number }) {
  const { plot } = useChartFrame();
  const element = useRef<HTMLDivElement>(null);
  const live = useRef({ zoom, plot, pad });
  const help = useId();
  const [panning, setPanning] = useState(false);
  const pointers = useRef(new Map<number, { x: number; y: number }>());
  const gesture = useRef<{
    x: number;
    y: number;
    view: ZoomView;
    moved: boolean;
    pinch: number | null;
  } | null>(null);
  const swallow = useRef(false);

  useEffect(() => {
    live.current = { zoom, plot, pad };
  });

  // The wheel listener must not be passive, so it can keep a pinch from zooming the page.
  useEffect(() => {
    const node = element.current;
    if (!node) return;
    const onWheel = (event: WheelEvent) => {
      const { zoom, plot, pad } = live.current;
      const r = node.getBoundingClientRect();
      const x = event.clientX - r.left + plot.left,
        y = event.clientY - r.top + plot.top;
      const t = zoom.target;
      if (event.ctrlKey || event.metaKey) {
        event.preventDefault();
        zoom.zoomBy(wheelScale(event), {
          x: valueAt(x, [plot.left + pad, plot.right - pad], t.x),
          y: valueAt(y, [plot.bottom - pad, plot.top + pad], t.y),
        });
        return;
      }
      if (zoom.zoomed && Math.abs(event.deltaX) > Math.abs(event.deltaY)) {
        event.preventDefault();
        const w = plot.right - plot.left - pad * 2 || 1;
        zoom.panBy(((t.x[1] - t.x[0]) * event.deltaX) / w, 0, true);
      }
      // A plain scroll scrolls the page.
    };
    node.addEventListener("wheel", onWheel, { passive: false });
    // The plot names how to zoom in its accessible description.
    const group = node.closest('[role="group"]');
    const before = group?.getAttribute("aria-describedby") ?? null;
    group?.setAttribute("aria-describedby", before ? `${before} ${help}` : help);
    return () => {
      node.removeEventListener("wheel", onWheel);
      if (!group) return;
      if (before) group.setAttribute("aria-describedby", before);
      else group.removeAttribute("aria-describedby");
    };
  }, [help]);

  const local = (event: { clientX: number; clientY: number }) => {
    const r = element.current?.getBoundingClientRect();
    return { x: event.clientX - (r?.left ?? 0), y: event.clientY - (r?.top ?? 0) };
  };
  const spread = () => {
    const [a, b] = [...pointers.current.values()];
    return a && b ? Math.hypot(a.x - b.x, a.y - b.y) : 0;
  };
  const onPointerDown = (event: PointerEvent<HTMLDivElement>) => {
    if (event.pointerType === "mouse" && event.button !== 0) return;
    pointers.current.set(event.pointerId, local(event));
    const at = local(event);
    if (pointers.current.size === 2) {
      gesture.current = { ...at, view: zoom.target, moved: true, pinch: spread() };
      event.currentTarget.setPointerCapture?.(event.pointerId);
    } else if (zoom.zoomed || event.pointerType === "touch") {
      gesture.current = { ...at, view: zoom.target, moved: false, pinch: null };
      event.currentTarget.setPointerCapture?.(event.pointerId);
    }
  };
  const onPointerMove = (event: PointerEvent<HTMLDivElement>) => {
    const g = gesture.current;
    if (!pointers.current.has(event.pointerId) || !g) return;
    const at = local(event);
    pointers.current.set(event.pointerId, at);
    const w = plot.right - plot.left - pad * 2 || 1,
      h = plot.bottom - plot.top - pad * 2 || 1;
    if (g.pinch && pointers.current.size === 2) {
      event.stopPropagation();
      const [a, b] = [...pointers.current.values()];
      const scale = g.pinch / Math.max(8, spread());
      const mid = { x: (a.x + b.x) / 2 + plot.left, y: (a.y + b.y) / 2 + plot.top };
      const ax = valueAt(mid.x, [plot.left + pad, plot.right - pad], g.view.x);
      const ay = valueAt(mid.y, [plot.bottom - pad, plot.top + pad], g.view.y);
      const zx = zoom.axes === "y" ? g.view.x : zoomSpan(g.view.x, scale, ax, zoom.extent.x);
      const zy = zoom.axes === "x" ? g.view.y : zoomSpan(g.view.y, scale, ay, zoom.extent.y);
      zoom.set({ x: zx, y: zy }, true);
      return;
    }
    if (!zoom.zoomed && !g.moved) return;
    const dx = at.x - g.x,
      dy = at.y - g.y;
    if (!g.moved && Math.hypot(dx, dy) < 4) return;
    if (!g.moved) {
      g.moved = true;
      setPanning(true);
    }
    event.stopPropagation();
    const vx = g.view.x[1] - g.view.x[0],
      vy = g.view.y[1] - g.view.y[0];
    zoom.set(
      {
        x: zoom.axes === "y" ? g.view.x : panSpan(g.view.x, (-dx / w) * vx, zoom.extent.x),
        y: zoom.axes === "x" ? g.view.y : panSpan(g.view.y, (dy / h) * vy, zoom.extent.y),
      },
      true,
    );
  };
  const onPointerUp = (event: PointerEvent<HTMLDivElement>) => {
    pointers.current.delete(event.pointerId);
    const g = gesture.current;
    if (g && pointers.current.size < 2) {
      if (g.moved) swallow.current = true;
      gesture.current = null;
      setPanning(false);
    }
    event.currentTarget.releasePointerCapture?.(event.pointerId);
  };

  return (
    <div
      ref={element}
      className={styles.zoomSurface}
      data-zoomed={zoom.zoomed}
      data-panning={panning}
      style={{
        left: plot.left,
        top: plot.top,
        width: Math.max(0, plot.right - plot.left),
        height: Math.max(0, plot.bottom - plot.top),
      }}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onPointerCancel={onPointerUp}
      onClickCapture={(event) => {
        if (!swallow.current) return;
        swallow.current = false;
        event.stopPropagation();
      }}
      onDoubleClick={(event) => {
        if (!zoom.zoomed) return;
        event.stopPropagation();
        zoom.reset();
      }}
    >
      <span id={help} className={core.srOnly}>
        Pinch, or scroll with Command or Control held, to zoom. Drag to pan; double-click or press 0
        to reset.
      </span>
    </div>
  );
}

const RESTART = "m18.364 8.05l-.707-.707a8 8 0 1 0 2.28 4.658m-1.573-3.95h-4.243m4.243 0V3.807";

/** A "Reset zoom" key at the plot's top right, shown while zoomed; announces zoom changes. */
export function ResetZoom({
  zoom,
  label = "Reset zoom",
  describe,
}: {
  zoom: Zoom;
  label?: string;
  /** The view in words, read aloud when it settles ("Zoomed to 10–20 by 4–8"). */
  describe?: (view: ZoomView) => string;
}) {
  const { plot } = useChartFrame();
  const words = zoom.zoomed ? (describe?.(zoom.target) ?? "Zoomed in") : "";
  const [spoken, setSpoken] = useState("");
  const was = useRef(false);
  useEffect(() => {
    if (!words && !was.current) return;
    was.current = !!words;
    const timer = window.setTimeout(() => setSpoken(words || "Zoom reset"), 400);
    return () => window.clearTimeout(timer);
  }, [words]);
  return (
    <>
      <span
        className={styles.resetSpot}
        data-export="skip"
        data-shown={zoom.zoomed}
        style={{ top: plot.top + 6, right: `calc(100% - ${plot.right - 6}px)` }}
      >
        <button
          type="button"
          className={toggleStyles.toggleKey}
          tabIndex={zoom.zoomed ? 0 : -1}
          aria-hidden={!zoom.zoomed}
          onPointerDown={(event) => event.stopPropagation()}
          onPointerMove={(event) => event.stopPropagation()}
          onClick={(event) => {
            event.stopPropagation();
            zoom.reset();
          }}
          onKeyDown={(event) => event.stopPropagation()}
        >
          <svg viewBox="0 0 24 24" width="15" height="15" aria-hidden="true">
            <path
              d={RESTART}
              fill="none"
              stroke="currentColor"
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth="1.5"
            />
          </svg>
          {label}
        </button>
      </span>
      <span className={core.srOnly} aria-live="polite">
        {spoken}
      </span>
    </>
  );
}
