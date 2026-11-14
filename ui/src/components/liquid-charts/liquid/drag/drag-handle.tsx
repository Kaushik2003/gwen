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

/* Icons: Solar (sort-vertical-linear) by 480 Design, CC BY 4.0,
 * https://www.figma.com/community/file/1166831539721848736 */

import {
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type MouseEvent as ReactMouseEvent,
  type PointerEvent as ReactPointerEvent,
  type ReactNode,
} from "react";
import { cssModule } from "@/components/liquid-charts/liquid/core/css-module";
import type { Rect } from "@/components/liquid-charts/liquid/core/geometry";
import Styles from "./interact.module.css";

const styles = cssModule(Styles, "lc-interact-");

/**
 * Drag to edit: setting a value by direct manipulation (a bar's tip, a target, a slider's thumb).
 * The maths is pure and tested; `useDrag` holds one gesture at a time over any number of items, so
 * one hook serves a whole chart. Values snap to a step inside bounds, the keyboard has the same
 * reach (arrows by a step, Page keys by ten, Home and End to the bounds), a gesture can be
 * cancelled with Escape, and `onCommit` fires once when it ends.
 */

/** The smallest hit area a touch target gets, CSS px. */
export const TOUCH_TARGET = 44;
/** Steps the Page keys move. */
export const PAGE_STEPS = 10;

export type DragBounds = {
  /** The grid values snap to; 0 or missing for none. */
  step?: number;
  min?: number;
  max?: number;
  /** Where the grid starts (default 0), so a step of 5 from 2 gives 2, 7, 12… */
  origin?: number;
};

const decimalsOf = (value: number) => {
  if (!Number.isFinite(value)) return 0;
  const text = String(value);
  const exp = text.match(/e-(\d+)$/);
  if (exp) return Number(exp[1]);
  const dot = text.indexOf(".");
  return dot < 0 ? 0 : text.length - dot - 1;
};
/** Rounds away float noise from step arithmetic (0.1 + 0.2), to the step's own precision. */
const tidy = (value: number, bounds: DragBounds) => {
  const places = Math.min(12, decimalsOf(bounds.step ?? 0) + decimalsOf(bounds.origin ?? 0));
  return Number(value.toFixed(places));
};

/** `value` inside [min, max] (either may be missing). */
export function clampValue(value: number, min = -Infinity, max = Infinity) {
  if (min > max) [min, max] = [max, min];
  return Math.max(min, Math.min(max, value));
}

/** The nearest grid value to `value`, kept inside the bounds. Non-finite input gives `min` (or 0). */
export function snapToStep(value: number, bounds: DragBounds = {}): number {
  const { step = 0, min = -Infinity, max = Infinity, origin = 0 } = bounds;
  if (!Number.isFinite(value)) return Number.isFinite(min) ? min : 0;
  let snapped = value;
  if (step > 0) snapped = tidy(origin + Math.round((value - origin) / step) * step, bounds);
  return clampValue(snapped, min, max);
}

/**
 * The value a key moves `value` to, or null when the key isn't an editing key. `axis` says which
 * arrows edit: `y` (columns) uses Up and Down, `x` (rows) Right and Left. An arrow moves to the next
 * grid value in its direction (412 with a step of 10 goes up to 420 and down to 410), the Page keys
 * ten steps, Home and End to the bounds (when they are finite).
 */
export function stepValue(
  value: number,
  key: string,
  bounds: DragBounds & { axis?: "x" | "y"; page?: number } = {},
): number | null {
  const { step = 1, min = -Infinity, max = Infinity, origin = 0, axis = "y" } = bounds;
  const page = bounds.page ?? PAGE_STEPS;
  const up = axis === "y" ? "ArrowUp" : "ArrowRight";
  const down = axis === "y" ? "ArrowDown" : "ArrowLeft";
  if (key === "Home") return Number.isFinite(min) ? min : null;
  if (key === "End") return Number.isFinite(max) ? max : null;
  const steps =
    key === up ? 1 : key === down ? -1 : key === "PageUp" ? page : key === "PageDown" ? -page : 0;
  if (!steps) return null;
  const size = step > 0 ? step : 1;
  const start = Number.isFinite(value) ? value : Number.isFinite(min) ? min : 0;
  // Off the grid, the first step lands on the next grid value in that direction.
  const k = (start - origin) / size;
  const near = Math.round(k);
  const next =
    Math.abs(k - near) < 1e-9
      ? near + steps
      : steps > 0
        ? Math.ceil(k) + steps - 1
        : Math.floor(k) + steps + 1;
  return clampValue(tidy(origin + next * size, { ...bounds, step: size }), min, max);
}

/** `rect` grown about its centre to at least `size` px each way, so a thin mark is easy to grab. */
export function touchTarget(rect: Rect, size = TOUCH_TARGET): Rect {
  const width = rect.right - rect.left,
    height = rect.bottom - rect.top;
  const cx = (rect.left + rect.right) / 2,
    cy = (rect.top + rect.bottom) / 2;
  const w = Math.max(width, size) / 2,
    h = Math.max(height, size) / 2;
  return { left: cx - w, top: cy - h, right: cx + w, bottom: cy + h };
}

/* ——— The gesture ——— */

export type DragPoint = { x: number; y: number };

/** The gesture in progress: which item, its value now and when it began, and the pointer. */
export type DragState = {
  index: number;
  value: number;
  from: number;
  /** The pointer in the surface's px (the chart frame, by default). */
  point: DragPoint;
  /** How the item was moved: by a pointer, or a key. */
  via: "pointer" | "key";
};

export type DragOptions = {
  /** Each item's value now (controlled). */
  value: (index: number) => number;
  /** Every change while the value moves (live during a drag). */
  onChange: (index: number, value: number) => void;
  /** Once, when a drag ends or a key changes the value. Not called when Escape cancels. */
  onCommit?: (index: number, value: number, from: number) => void;
  /** The raw value under a pointer at `point` (surface px). */
  toValue: (index: number, point: DragPoint) => number;
  /** Step and bounds, for every item or per item. */
  bounds?: DragBounds | ((index: number) => DragBounds);
  /** Which arrows edit: `y` (Up and Down) or `x` (Right and Left). Default `y`. */
  axis?: "x" | "y";
  /** The element pointer positions are measured from. Default: the handle's offset parent. */
  surface?: () => HTMLElement | null;
  /** Says a value aloud ("Design budget $420K"), for the live readout. */
  speak?: (index: number, value: number) => string;
  /** Called when a gesture starts, with the item. */
  onStart?: (index: number) => void;
  disabled?: boolean;
};

/**
 * One drag gesture at a time over any number of items. Spread `handle(index)` on an item's grab
 * area (pointer capture, touch-action none, a 44px target is up to the host); call `key(index,
 * key)` from the host's key handler for the keyboard path (it returns true when it used the key).
 * `drag` is the gesture in progress (for a readout at the pointer), `live` a sentence for an
 * `aria-live` region (keyboard steps and committed drags only, never every pointer move).
 */
export function useDrag(options: DragOptions) {
  const [drag, setDrag] = useState<DragState | null>(null);
  const [live, setLive] = useState("");
  const latest = useRef(options);
  const gesture = useRef<(DragState & { offset: number; pointer: number }) | null>(null);
  useEffect(() => {
    latest.current = options;
  });

  const boundsOf = (index: number): DragBounds => {
    const b = latest.current.bounds;
    return (typeof b === "function" ? b(index) : b) ?? {};
  };
  const say = (index: number, value: number) =>
    latest.current.speak ? latest.current.speak(index, value) : String(value);

  const end = (commit: boolean) => {
    const g = gesture.current;
    if (!g) return;
    gesture.current = null;
    setDrag(null);
    const o = latest.current;
    if (!commit) {
      if (g.value !== g.from) o.onChange(g.index, g.from);
      return;
    }
    o.onCommit?.(g.index, g.value, g.from);
    if (g.value !== g.from) setLive(say(g.index, g.value));
  };

  // Escape cancels a drag wherever focus is.
  useEffect(() => {
    if (!drag || drag.via !== "pointer") return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      event.stopPropagation();
      end(false);
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
    // `end` reads refs only.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [drag?.via, drag !== null]);

  const pointOf = (event: ReactPointerEvent<HTMLElement>): DragPoint => {
    const surface =
      latest.current.surface?.() ?? (event.currentTarget.offsetParent as HTMLElement | null);
    const r = surface?.getBoundingClientRect() ?? { left: 0, top: 0 };
    return { x: event.clientX - r.left, y: event.clientY - r.top };
  };

  /** Props for one item's grab area. */
  const handle = (index: number) => ({
    onPointerDown(event: ReactPointerEvent<HTMLElement>) {
      const o = latest.current;
      if (o.disabled || gesture.current) return;
      if (event.pointerType === "mouse" && event.button !== 0) return;
      event.preventDefault();
      event.stopPropagation();
      event.currentTarget.setPointerCapture?.(event.pointerId);
      const point = pointOf(event);
      const from = o.value(index);
      // Grabbing a little off the tip keeps that offset, so the value doesn't jump on press.
      const raw = o.toValue(index, point);
      const offset = Number.isFinite(raw) && Number.isFinite(from) ? raw - from : 0;
      gesture.current = {
        index,
        value: from,
        from,
        point,
        via: "pointer",
        offset,
        pointer: event.pointerId,
      };
      o.onStart?.(index);
      setDrag({ index, value: from, from, point, via: "pointer" });
    },
    onPointerMove(event: ReactPointerEvent<HTMLElement>) {
      const g = gesture.current;
      if (!g || g.index !== index || g.pointer !== event.pointerId) return;
      event.stopPropagation();
      const o = latest.current;
      const point = pointOf(event);
      const value = snapToStep(o.toValue(index, point) - g.offset, boundsOf(index));
      const changed = value !== g.value;
      g.value = value;
      g.point = point;
      if (changed) o.onChange(index, value);
      setDrag({ index, value, from: g.from, point, via: "pointer" });
    },
    onPointerUp(event: ReactPointerEvent<HTMLElement>) {
      const g = gesture.current;
      if (!g || g.pointer !== event.pointerId) return;
      event.stopPropagation();
      end(true);
    },
    onPointerCancel() {
      end(false);
    },
    onLostPointerCapture() {
      if (gesture.current) end(true);
    },
    // The frame pins an item on click; a drag is not a click.
    onClick(event: ReactMouseEvent<HTMLElement>) {
      event.stopPropagation();
    },
  });

  /** The keyboard path: moves the item's value for an editing key and commits it. */
  const key = (index: number, keyName: string) => {
    const o = latest.current;
    if (o.disabled) return false;
    const from = o.value(index);
    const next = stepValue(from, keyName, { ...boundsOf(index), axis: o.axis ?? "y" });
    if (next === null) return false;
    if (next !== from) {
      o.onChange(index, next);
      o.onCommit?.(index, next, from);
      setLive(say(index, next));
    }
    return true;
  };

  return { drag, live, handle, key, cancel: () => end(false) };
}

/* ——— Pieces a host places ——— */

/**
 * The value at the pointer while it drags: a plain figure beside the pointer (up and to the right,
 * flipping left near the edge), with an optional quiet note under it (the change). Place it inside
 * the chart frame; `x`/`y` are frame px.
 */
export function DragReadout({
  x,
  y,
  bounds,
  children,
  note,
}: {
  x: number;
  y: number;
  /** Where it must stay, frame px. */
  bounds: Rect;
  children: ReactNode;
  note?: ReactNode;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [size, setSize] = useState({ width: 60, height: 32 });
  useEffect(() => {
    const element = ref.current;
    if (!element) return;
    const observer = new ResizeObserver(() =>
      setSize({ width: element.offsetWidth, height: element.offsetHeight }),
    );
    observer.observe(element);
    return () => observer.disconnect();
  }, []);
  const at = readoutPosition({ x, y }, size, bounds);
  return (
    <div
      ref={ref}
      className={styles.readout}
      style={{ transform: `translate(${at.x}px, ${at.y}px)` } as CSSProperties}
      aria-hidden="true"
    >
      <span className={styles.readoutValue}>{children}</span>
      {note && <span className={styles.readoutNote}>{note}</span>}
    </div>
  );
}

/** Where a readout of `size` sits for a pointer at `point`: up and right of it, flipped left or
 * below where it would leave `bounds`, and always inside them. */
export function readoutPosition(
  point: DragPoint,
  size: { width: number; height: number },
  bounds: Rect,
  gap = 14,
): DragPoint {
  let x = point.x + gap;
  let y = point.y - gap - size.height;
  if (x + size.width > bounds.right) x = point.x - gap - size.width;
  if (y < bounds.top) y = point.y + gap;
  return {
    x: Math.round(clampValue(x, bounds.left, bounds.right - size.width)),
    y: Math.round(clampValue(y, bounds.top, bounds.bottom - size.height)),
  };
}

/** The sentence `useDrag` returns as `live`, in a polite live region screen readers announce. */
export function DragLive({ text }: { text: string }) {
  return (
    <p className={styles.srOnly} aria-live="polite">
      {text}
    </p>
  );
}

const SORT_VERTICAL = "M16 18V6m0 0l4 4.125M16 6l-4 4.125M8 6v12m0 0l4-4.125M8 18l-4-4.125";

/**
 * A grab handle: a 44px hit area (touch-action none) centred on `x`/`y` (frame px). A small
 * up-and-down icon shows while its item is in focus (hovered or reached by the keyboard), offset by
 * `icon` from the centre; `axis` turns it for rows. Spread `useDrag`'s `handle(index)` on it.
 */
export function DragHandle({
  x,
  y,
  axis = "y",
  width = TOUCH_TARGET,
  height = TOUCH_TARGET,
  active = false,
  dragging = false,
  icon = { x: 0, y: 0 },
  ...events
}: {
  x: number;
  y: number;
  axis?: "x" | "y";
  /** The hit area, px (at least 44 each way is best; narrower between crowded bars). */
  width?: number;
  height?: number;
  /** The item is in focus: the icon brightens. */
  active?: boolean;
  dragging?: boolean;
  /** Where the icon sits from the hit area's centre, px (beside a column's tip, say). */
  icon?: DragPoint;
} & ReturnType<ReturnType<typeof useDrag>["handle"]>) {
  return (
    <span
      className={styles.handle}
      data-axis={axis}
      data-active={active}
      data-dragging={dragging}
      style={{ left: x - width / 2, top: y - height / 2, width, height }}
      aria-hidden="true"
      {...events}
    >
      <svg
        className={styles.handleIcon}
        viewBox="0 0 24 24"
        width="14"
        height="14"
        style={{ translate: `${icon.x}px ${icon.y}px`, rotate: axis === "x" ? "90deg" : undefined }}
      >
        <path
          d={SORT_VERTICAL}
          fill="none"
          stroke="currentColor"
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth="1.8"
        />
      </svg>
    </span>
  );
}
