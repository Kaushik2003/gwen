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

/* Icons: Solar (eye-linear, restart-linear, pen-linear) by 480 Design, CC BY 4.0,
 * https://www.figma.com/community/file/1166831539721848736 */

import {
  useEffect,
  useRef,
  useState,
  type MouseEvent as ReactMouseEvent,
  type PointerEvent as ReactPointerEvent,
  type ReactNode,
} from "react";
import { clampValue, stepValue, type DragPoint } from "./drag-handle";
import { cssModule } from "@/components/liquid-charts/liquid/core/css-module";
import { toggleStyles } from "@/components/liquid-charts/liquid/core/toggles";
import type { Rect } from "@/components/liquid-charts/liquid/core/geometry";
import Roles from "@/components/liquid-charts/liquid/core/type.module.css";
import { Figure } from "@/components/liquid-charts/liquid/core/figure";
import Styles from "./interact.module.css";

const roles = cssModule(Roles, "lc-type-");
const styles = cssModule(Styles, "lc-interact-");

/**
 * Draw your guess: the reader draws the rest of a line over a hidden stretch, then "Show me"
 * reveals the real one. The maths is pure and tested: a stroke is written onto the x grid (every
 * grid point it passes gets the stroke's value there, and any undrawn gap behind it is filled from
 * the last drawn point, so the guess stays one continuous line), the keyboard steps along x and
 * sets y, and the result says how close the guess came. `useGuess` holds the state and the
 * reveal; the host draws the lines in its own material (LineChart: the guess as dashed light, the
 * truth as light, the gap between them a quiet band).
 */

export type Guess = readonly (number | null)[];
/** The hidden stretch: its first and last data index. */
export type GuessSpan = readonly [number, number];

/** The stretch to hide for `count` points from `from` to `to` (default the last), or null when
 * there is nothing to draw. `from` is at least 1, so a known point anchors the guess. */
export function guessSpan(count: number, from: number, to?: number): GuessSpan | null {
  if (!(count > 1) || !Number.isFinite(from)) return null;
  const a = Math.max(1, Math.round(from));
  const b = Math.min(count - 1, Math.round(to ?? count - 1));
  return a <= b ? [a, b] : null;
}

/** The last drawn index before `index` inside the span, or the anchor (from − 1). */
function lastDrawn(guess: Guess, span: GuessSpan, index: number) {
  for (let k = Math.min(index - 1, span[1]); k >= span[0]; k--) if (guess[k] !== null) return k;
  return span[0] - 1;
}

/**
 * Writes a pointer stroke from `a` to `b` (each a fractional data index and a value) onto the grid
 * indices it passes, inside the span. A stroke too short to pass a grid point sets the nearest one.
 * Any undrawn indices between the last drawn point (or the anchor) and the stroke are filled on a
 * straight line, so the guess never has a hole.
 */
export function drawStroke(
  guess: Guess,
  a: { index: number; value: number },
  b: { index: number; value: number },
  span: GuessSpan,
  anchor: number,
): (number | null)[] {
  const next = guess.slice();
  const [from, to] = span;
  const lo = Math.min(a.index, b.index),
    hi = Math.max(a.index, b.index);
  const at = (k: number) =>
    b.index === a.index
      ? b.value
      : a.value + ((k - a.index) / (b.index - a.index)) * (b.value - a.value);
  const written: number[] = [];
  for (
    let k = Math.max(from, Math.ceil(lo - 1e-9));
    k <= Math.min(to, Math.floor(hi + 1e-9));
    k++
  ) {
    next[k] = at(k);
    written.push(k);
  }
  if (!written.length) {
    const k = clampValue(Math.round(b.index), from, to);
    if (Number.isFinite(k)) {
      next[k] = b.value;
      written.push(k);
    }
  }
  if (!written.length) return next;
  const first = Math.min(...written);
  const j = lastDrawn(next, span, first);
  const start = j < from ? anchor : (next[j] as number);
  for (let k = j + 1; k < first; k++)
    if (next[k] === null || next[k] === undefined)
      next[k] = start + ((k - j) / (first - j)) * ((next[first] as number) - start);
  return next;
}

/** True when every hidden index has a guess. */
export function guessComplete(guess: Guess, span: GuessSpan | null) {
  if (!span) return false;
  for (let k = span[0]; k <= span[1]; k++)
    if (guess[k] === null || guess[k] === undefined) return false;
  return true;
}

/** The keyboard reaching a hidden index: any gap up to it is filled flat from the last drawn point
 * (or the anchor), so the reader can step right and then set each value. */
export function enterGuess(guess: Guess, index: number, span: GuessSpan, anchor: number) {
  if (index < span[0] || index > span[1] || guess[index] !== null) return guess;
  const next = guess.slice();
  const j = lastDrawn(next, span, index);
  const carry = j < span[0] ? anchor : (next[j] as number);
  for (let k = j + 1; k <= index; k++) if (next[k] === null) next[k] = carry;
  return next;
}

/** Up, Down and the Page keys on a hidden index: moves its guess by a step (filling the way to it
 * first). Null when the key or index isn't one the guess uses. */
export function stepGuess(
  guess: Guess,
  index: number,
  key: string,
  span: GuessSpan,
  anchor: number,
  bounds: { step: number; min?: number; max?: number },
): (number | null)[] | null {
  if (index < span[0] || index > span[1]) return null;
  if (!["ArrowUp", "ArrowDown", "PageUp", "PageDown"].includes(key)) return null;
  const filled = enterGuess(guess, index, span, anchor).slice();
  const next = stepValue(filled[index] as number, key, { ...bounds, axis: "y" });
  if (next === null) return null;
  filled[index] = next;
  return filled;
}

export type GuessResult = {
  /** Points compared. */
  points: number;
  /** Mean absolute error as a share of the real values (0.08 is 8% off on average). */
  error: number;
  /** Mean signed error as a share: above 0 the guess ran high. */
  bias: number;
  /** The last point's error as a share: above 0 the guess ended high. */
  end: number;
  /** Share of points within 10% of the real value. */
  within: number;
  closeness: "close" | "near" | "far";
};

/** How close the guess came to the truth over the span. Null with nothing to compare. */
export function guessResult(
  guess: Guess,
  truth: readonly (number | null | undefined)[],
  span: GuessSpan | null,
): GuessResult | null {
  if (!span) return null;
  let n = 0,
    abs = 0,
    signed = 0,
    scale = 0,
    within = 0,
    end: number | null = null;
  for (let k = span[0]; k <= span[1]; k++) {
    const g = guess[k],
      t = truth[k];
    if (g === null || g === undefined || t === null || t === undefined) continue;
    if (!Number.isFinite(g) || !Number.isFinite(t)) continue;
    n++;
    abs += Math.abs(g - t);
    signed += g - t;
    scale += Math.abs(t);
    const share = Math.abs(t) > 1e-12 ? Math.abs(g - t) / Math.abs(t) : g === t ? 0 : Infinity;
    if (share <= 0.1 + 1e-9) within++;
    end = Math.abs(t) > 1e-12 ? (g - t) / Math.abs(t) : 0;
  }
  if (!n) return null;
  const error = scale > 1e-12 ? abs / scale : abs ? Infinity : 0;
  return {
    points: n,
    error,
    bias: scale > 1e-12 ? signed / scale : 0,
    end: end ?? 0,
    within: within / n,
    closeness: error <= 0.1 ? "close" : error <= 0.25 ? "near" : "far",
  };
}

const pct = (share: number) => `${Math.round(Math.abs(share) * 100)}%`;

/** The result in one sentence ("Within 8% of the real line; you ended 12% low."). */
export function guessVerdict(result: GuessResult): string {
  const lead =
    result.closeness === "close"
      ? `Within ${pct(result.error)} of the real line`
      : `Off by ${pct(result.error)} on average`;
  const tail =
    Math.abs(result.end) < 0.03
      ? ", right to the end."
      : `; you ended ${pct(result.end)} ${result.end > 0 ? "high" : "low"}.`;
  return lead + tail;
}

/** Where the gap between the guess and the truth runs, by data index: from the anchor to `upTo`,
 * the lower and upper of the two (null outside, or where either is missing). */
export function guessBand(
  guess: Guess,
  truth: readonly (number | null | undefined)[],
  span: GuessSpan,
  upTo: number,
): { lower: (number | null)[]; upper: (number | null)[] } {
  const lower: (number | null)[] = truth.map(() => null);
  const upper: (number | null)[] = truth.map(() => null);
  for (let k = span[0] - 1; k <= Math.min(upTo, span[1]); k++) {
    const t = truth[k];
    const g = k === span[0] - 1 ? t : guess[k];
    if (t === null || t === undefined || g === null || g === undefined) continue;
    lower[k] = Math.min(g, t);
    upper[k] = Math.max(g, t);
  }
  return { lower, upper };
}

/** How many hidden points the reveal shows at `progress` (0–1). */
export const revealCount = (progress: number, span: GuessSpan) =>
  Math.max(
    0,
    Math.min(span[1] - span[0] + 1, Math.ceil(progress * (span[1] - span[0] + 1) - 1e-9)),
  );

/* ——— The state ——— */

export type GuessGeometry = {
  /** A data index's px (frame). */
  x: (index: number) => number;
  /** The fractional data index at a px. */
  index: (px: number) => number;
  /** The value at a px. */
  value: (py: number) => number;
};

export type GuessOptions = {
  count: number;
  span: GuessSpan | null;
  /** The known value the guess starts from (at span[0] − 1). */
  anchor: number;
  /** The real values, by data index (for the result). */
  truth: readonly (number | null | undefined)[];
  /** The value range a guess may take (the chart's domain). */
  min: number;
  max: number;
  /** The keyboard's step. */
  step: number;
  /** Maps frame px to data, for the current size. */
  geometry: () => GuessGeometry | null;
  /** Revealed, controlled. */
  revealed?: boolean;
  onRevealedChange?: (revealed: boolean) => void;
  onChange?: (guess: Guess) => void;
  onReveal?: (result: GuessResult | null) => void;
};

/**
 * The guess, its drawing gesture and the reveal. Spread `zone` on the drawing surface; call
 * `key(active, key)` from the frame's key handler (Up, Down and the Page keys set the guess at a
 * hidden index) and `enter(index)` when the keyboard steps onto one. `shown` is how many hidden
 * points the reveal shows so far (it sweeps left to right; reduced motion shows them at once).
 */
export function useGuess(options: GuessOptions) {
  const { count, span, anchor, truth } = options;
  const [guess, setGuessState] = useState<Guess>(() => Array.from({ length: count }, () => null));
  const [innerRevealed, setInnerRevealed] = useState(false);
  const revealed = options.revealed ?? innerRevealed;
  const [progress, setProgress] = useState(revealed ? 1 : 0);
  const [drawing, setDrawing] = useState<{ index: number; value: number; point: DragPoint } | null>(
    null,
  );
  const latest = useRef(options);
  const last = useRef<{ index: number; value: number; pointer: number } | null>(null);
  useEffect(() => {
    latest.current = options;
  });

  // A different length of data starts a fresh guess.
  const current = guess.length === count ? guess : Array.from({ length: count }, () => null);
  // Pointer events can outrun renders, so handlers build on the newest guess, not the rendered one.
  const newest = useRef<Guess>(current);
  const base = () => (newest.current.length === count ? newest.current : current);

  const setGuess = (next: Guess) => {
    newest.current = next;
    setGuessState(next);
    latest.current.onChange?.(next);
  };
  const setRevealed = (value: boolean) => {
    if (latest.current.revealed === undefined) setInnerRevealed(value);
    latest.current.onRevealedChange?.(value);
  };

  // The reveal sweeps the truth in from the anchor; going back hides it again. Every change of
  // progress lands in a frame, never in the effect itself: hidden again it returns to 0 (what's
  // shown reads 0 at once, below), and without motion it jumps to 1.
  useEffect(() => {
    let raf = 0;
    if (!revealed || window.matchMedia("(prefers-reduced-motion: reduce)").matches || !span) {
      const to = revealed ? 1 : 0;
      raf = requestAnimationFrame(() => setProgress(to));
      return () => cancelAnimationFrame(raf);
    }
    const n = span[1] - span[0] + 1;
    const duration = Math.max(600, Math.min(1400, 280 + n * 70));
    const start = performance.now();
    const tick = (now: number) => {
      const t = Math.min(1, (now - start) / duration);
      setProgress(1 - (1 - t) ** 2);
      if (t < 1) raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
    // Only a change of `revealed` replays it.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [revealed]);

  const complete = guessComplete(current, span);
  const result = guessResult(current, truth, span);
  const shown = span ? (revealed ? revealCount(progress, span) : 0) : 0;

  const toData = (event: ReactPointerEvent<HTMLElement>) => {
    const g = latest.current.geometry();
    const surface = event.currentTarget.offsetParent as HTMLElement | null;
    const r = surface?.getBoundingClientRect() ?? { left: 0, top: 0 };
    const point = { x: event.clientX - r.left, y: event.clientY - r.top };
    if (!g) return null;
    const o = latest.current;
    return {
      point,
      index: g.index(point.x),
      value: clampValue(g.value(point.y), o.min, o.max),
    };
  };

  const zone = {
    onPointerDown(event: ReactPointerEvent<HTMLElement>) {
      if (revealed || !span) return;
      if (event.pointerType === "mouse" && event.button !== 0) return;
      const at = toData(event);
      if (!at) return;
      event.preventDefault();
      event.currentTarget.setPointerCapture?.(event.pointerId);
      last.current = { index: at.index, value: at.value, pointer: event.pointerId };
      const next = drawStroke(base(), at, at, span, anchor);
      setGuess(next);
      const k = clampValue(Math.round(at.index), span[0], span[1]);
      setDrawing({ index: k, value: next[k] ?? at.value, point: at.point });
    },
    onPointerMove(event: ReactPointerEvent<HTMLElement>) {
      const from = last.current;
      if (!from || from.pointer !== event.pointerId || !span) return;
      const at = toData(event);
      if (!at) return;
      last.current = { index: at.index, value: at.value, pointer: event.pointerId };
      const next = drawStroke(base(), from, at, span, anchor);
      setGuess(next);
      const k = clampValue(Math.round(at.index), span[0], span[1]);
      setDrawing({ index: k, value: next[k] ?? at.value, point: at.point });
    },
    onPointerUp() {
      last.current = null;
      setDrawing(null);
    },
    onPointerCancel() {
      last.current = null;
      setDrawing(null);
    },
    // The frame pins a point on click; drawing is not a click.
    onClick(event: ReactMouseEvent<HTMLElement>) {
      event.stopPropagation();
    },
  };

  /** The keyboard on a hidden index. Returns true when it used the key. */
  const key = (index: number | null, keyName: string) => {
    if (revealed || !span || index === null) return false;
    const o = latest.current;
    const next = stepGuess(base(), index, keyName, span, anchor, {
      step: o.step,
      min: o.min,
      max: o.max,
    });
    if (!next) return false;
    setGuess(next);
    return true;
  };
  /** The keyboard stepping onto `index`: fills the way to it, so it has a value to set. */
  const enter = (index: number | null) => {
    if (revealed || !span || index === null) return;
    const was = base();
    const next = enterGuess(was, index, span, anchor);
    if (next !== was) setGuess(next);
  };

  const reveal = () => {
    if (!complete || revealed) return;
    setRevealed(true);
    latest.current.onReveal?.(result);
  };
  const reset = () => {
    setGuess(Array.from({ length: count }, () => null));
    setRevealed(false);
  };

  return {
    guess: current,
    complete,
    revealed,
    progress: revealed ? progress : 0,
    shown,
    drawing,
    result,
    zone,
    key,
    enter,
    reveal,
    reset,
  };
}

/* ——— Pieces a host places ——— */

const PEN =
  "m14.36 4.079l.927-.927a3.932 3.932 0 0 1 5.561 5.561l-.927.927m-5.56-5.561s.115 1.97 1.853 3.707C17.952 9.524 19.92 9.64 19.92 9.64m-5.56-5.561l-8.522 8.52c-.577.578-.866.867-1.114 1.185a6.6 6.6 0 0 0-.749 1.211c-.173.364-.302.752-.56 1.526l-1.094 3.281m17.6-10.162L11.4 18.16c-.577.577-.866.866-1.184 1.114a6.6 6.6 0 0 1-1.211.749c-.364.173-.751.302-1.526.56l-3.281 1.094m0 0l-.802.268a1.06 1.06 0 0 1-1.342-1.342l.268-.802m1.876 1.876l-1.876-1.876";
const EYE_OUTLINE =
  "M3.275 15.296C2.425 14.192 2 13.639 2 12c0-1.64.425-2.191 1.275-3.296C4.972 6.5 7.818 4 12 4s7.028 2.5 8.725 4.704C21.575 9.81 22 10.361 22 12c0 1.64-.425 2.191-1.275 3.296C19.028 17.5 16.182 20 12 20s-7.028-2.5-8.725-4.704Z";
const EYE_PUPIL = "M15 12a3 3 0 1 1-6 0a3 3 0 0 1 6 0Z";
const RESTART = "m18.364 8.05l-.707-.707a8 8 0 1 0 2.28 4.658m-1.573-3.95h-4.243m4.243 0V3.807";

/**
 * The drawing surface over the hidden stretch: a quiet tint with a dashed edge where the known line
 * ends, and a small pen at the anchor until the first stroke. Frame px. Spread `useGuess`'s `zone`.
 */
export function GuessZone({
  rect,
  anchor,
  drawn,
  revealed,
  ...events
}: {
  rect: Rect;
  /** Where the pen sits (the known line's last point), frame px. */
  anchor: DragPoint | null;
  drawn: boolean;
  revealed: boolean;
} & ReturnType<typeof useGuess>["zone"]) {
  return (
    <div
      className={styles.zone}
      data-drawn={drawn}
      data-revealed={revealed}
      style={{
        left: rect.left,
        top: rect.top,
        width: Math.max(0, rect.right - rect.left),
        height: Math.max(0, rect.bottom - rect.top),
      }}
      aria-hidden="true"
      {...events}
    >
      {anchor && !revealed && (
        <svg
          className={styles.zoneIcon}
          viewBox="0 0 24 24"
          width="15"
          height="15"
          style={{ left: Math.max(4, anchor.x - rect.left + 13), top: anchor.y - rect.top - 7.5 }}
        >
          <path d={PEN} fill="none" stroke="currentColor" strokeWidth="1.5" />
        </svg>
      )}
    </div>
  );
}

/** The result as a sentence with figures in it, for the controls. */
export function GuessVerdict({ result }: { result: GuessResult }) {
  const figure = (share: number) => (
    <Figure value={Math.abs(share)} kind="percent" decimals={0} className={roles.label} />
  );
  return (
    <>
      {result.closeness === "close" ? (
        <>
          <strong>Within {figure(result.error)}</strong> of the real line
        </>
      ) : (
        <>
          <strong>Off by {figure(result.error)}</strong> on average
        </>
      )}
      {Math.abs(result.end) < 0.03 ? (
        ", right to the end."
      ) : (
        <>
          ; you ended {figure(result.end)} {result.end > 0 ? "high" : "low"}.
        </>
      )}
    </>
  );
}

/**
 * Under a guess chart: "Show me" until the reveal (inactive until the guess reaches the end), then
 * the verdict, read aloud, and a key to draw again.
 */
export function GuessControls({
  complete,
  revealed,
  result,
  onReveal,
  onReset,
  verdict,
  showLabel = "Show me",
  resetLabel = "Draw again",
}: {
  complete: boolean;
  revealed: boolean;
  result: GuessResult | null;
  onReveal: () => void;
  onReset: () => void;
  /** The sentence after the reveal; default `GuessVerdict`. */
  verdict?: ReactNode;
  showLabel?: string;
  resetLabel?: string;
}) {
  return (
    <div className={styles.guessBar} data-export="skip">
      {!revealed ? (
        <button
          type="button"
          className={`${toggleStyles.toggleKey} ${styles.action}`}
          aria-disabled={!complete}
          onClick={() => complete && onReveal()}
        >
          <svg viewBox="0 0 24 24" width="15" height="15" aria-hidden="true">
            <g fill="none" stroke="currentColor" strokeWidth="1.5">
              <path d={EYE_OUTLINE} />
              <path d={EYE_PUPIL} />
            </g>
          </svg>
          {showLabel}
        </button>
      ) : (
        <button
          type="button"
          className={toggleStyles.toggleKey}
          aria-label={resetLabel}
          title={resetLabel}
          onClick={onReset}
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
        </button>
      )}
      <p className={`${styles.guessVerdict} ${roles.caption}`} aria-live="polite">
        {revealed && result ? (verdict ?? <GuessVerdict result={result} />) : null}
      </p>
    </div>
  );
}
