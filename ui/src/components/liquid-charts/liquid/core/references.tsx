"use client";

import type { CSSProperties, ReactNode } from "react";
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
import { textWidth, TYPE } from "./format";
import { cssModule } from "./css-module";
import { useChartFrame } from "./frame";
import Styles from "./core.module.css";
import Roles from "./type.module.css";
import { TickText } from "./axis";

const styles = cssModule(Styles, "lc-core-");
const roles = cssModule(Roles, "lc-type-");

/*
 * References for flat and liquid charts (lines, areas, spectra): a target or average as a quiet
 * line across the plot, a range as a translucent band, and moments (a deploy, a launch) as a
 * hairline with a small label. They sit with the axes, quieter than the data. Glass charts don't
 * use these: their references live in the glass (engravings, containers, lit gaps).
 */

type Tone = "neutral" | "good" | "bad" | "accent";

/** A line at a value: horizontal across the plot (`axis="y"`, the default), or vertical up it
 * (`axis="x"`: a target on an x scale, a limit, a threshold). `position` maps the value to frame
 * px. A horizontal line's label sits at one end, above or below it; a vertical line's sits above
 * the plot on the side with room (or `labelSide`). */
export function ReferenceLine({
  value,
  position,
  label,
  tone = "neutral",
  dashed = true,
  axis = "y",
  labelSide,
  labelPlacement = "above",
}: {
  value: number;
  position: (value: number) => number;
  label?: ReactNode;
  tone?: Tone;
  dashed?: boolean;
  axis?: "x" | "y";
  /** Which end of the plot the label sits at (horizontal), or which side of the line (vertical). */
  labelSide?: "right" | "left";
  /** Above or below the line at its end; `axis`: in the value axis' tick column, set like a tick;
   * `end`: just past the plot's right end, level with the line. `axis` and `end` never sit over the
   * marks (horizontal lines). */
  labelPlacement?: "above" | "below" | "axis" | "end";
}) {
  const { size } = useChartFrame();
  if (!size.width) return null;
  const at = position(value);
  const side = labelSide ?? (axis === "x" && size.width - at < 90 ? "left" : "right");
  return (
    <span
      className={styles.referenceLine}
      data-axis={axis}
      data-tone={tone}
      data-dashed={dashed}
      style={axis === "x" ? { left: Math.round(at) - 0.5 } : { top: at }}
      aria-hidden="true"
    >
      {label && (
        <span
          className={`${styles.referenceLabel} ${roles.micro}`}
          data-side={side}
          data-placement={labelPlacement}
        >
          {labelPlacement === "axis" ? <TickText text={label} /> : label}
        </span>
      )}
    </span>
  );
}

/** A translucent band between two values (a normal range, a confidence interval), across the plot
 * (`axis="y"`) or up it (`axis="x"`: a period, a selection). */
export function ReferenceBand({
  from,
  to,
  position,
  label,
  tone = "neutral",
  axis = "y",
}: {
  from: number;
  to: number;
  position: (value: number) => number;
  label?: ReactNode;
  tone?: Tone;
  axis?: "x" | "y";
}) {
  const { size } = useChartFrame();
  if (!size.width) return null;
  const a = position(from),
    b = position(to);
  return (
    <span
      className={styles.referenceBand}
      data-axis={axis}
      data-tone={tone}
      style={
        axis === "x"
          ? { left: Math.min(a, b), width: Math.abs(b - a) }
          : { top: Math.min(a, b), height: Math.abs(b - a) }
      }
      aria-hidden="true"
    >
      {label && <span className={`${styles.referenceLabel} ${roles.micro}`}>{label}</span>}
    </span>
  );
}

/** A moment on the x axis: a hairline from the plot's top to its bottom, with a small label above.
 * `clear` spans (frame px, top to bottom) are left open, so the line steps round labels or marks
 * it would otherwise cross. */
export function EventMarker({
  x,
  label,
  tone = "neutral",
  clear,
}: {
  /** Frame px. */
  x: number;
  label?: ReactNode;
  tone?: Tone;
  clear?: readonly (readonly [number, number])[];
}) {
  const { size, plot } = useChartFrame();
  if (!size.width) return null;
  return (
    <span
      className={styles.event}
      data-tone={tone}
      data-broken={clear?.length ? true : undefined}
      style={{ left: x }}
      aria-hidden="true"
    >
      {clear?.length ? <LinePieces top={plot.top} bottom={plot.bottom} clear={clear} /> : null}
      {label && <span className={`${styles.eventLabel} ${roles.micro}`}>{label}</span>}
    </span>
  );
}

/** Event labels in lanes: when two labels would overlap, the later one moves up a lane instead of
 * disappearing (up to `lanes`, then it shows on hover only, through `title`). */
export function eventLanes(
  events: readonly { x: number; label?: string }[],
  { lanes = 2, gap = 8, size = TYPE.micro }: { lanes?: number; gap?: number; size?: number } = {},
): (number | null)[] {
  const ends: number[] = Array.from({ length: lanes }, () => -Infinity);
  return events.map((event) => {
    if (!event.label) return 0;
    const half = textWidth(event.label, size) / 2 + 7;
    for (let lane = 0; lane < lanes; lane++)
      if (event.x - half >= ends[lane] + gap) {
        ends[lane] = event.x + half;
        return lane;
      }
    return null;
  });
}

/** An event line from `top` to `bottom` (frame px) drawn in the pieces left once the `clear` spans
 * are cut out of it. */
function LinePieces({
  top,
  bottom,
  clear,
}: {
  top: number;
  bottom: number;
  clear: readonly (readonly [number, number])[];
}) {
  return linePieces(top, bottom, clear).map(([a, b]) => (
    <span key={a} className={styles.eventPiece} style={{ top: a - top, height: b - a }} />
  ));
}

function linePieces(top: number, bottom: number, clear: readonly (readonly [number, number])[]) {
  const cuts = clear
    .map(([a, b]) => [Math.max(top, Math.min(a, b)), Math.min(bottom, Math.max(a, b))] as const)
    .filter(([a, b]) => b > a)
    .sort((p, q) => p[0] - q[0]);
  const pieces: [number, number][] = [];
  let from = top;
  for (const [a, b] of cuts) {
    if (a > from) pieces.push([from, a]);
    from = Math.max(from, b);
  }
  if (bottom > from) pieces.push([from, bottom]);
  return pieces;
}

/** Several moments at once, their labels stacked in lanes so none collide. An event's `clear`
 * spans (frame px, top to bottom) are left open, so its line steps round the marks it would
 * otherwise cross. */
export function EventMarkers({
  events,
  lanes = 2,
}: {
  events: readonly {
    x: number;
    label?: string;
    tone?: Tone;
    clear?: readonly (readonly [number, number])[];
  }[];
  lanes?: number;
}) {
  const { size, plot } = useChartFrame();
  if (!size.width) return null;
  const order = events.map((event, i) => ({ event, i })).sort((a, b) => a.event.x - b.event.x);
  const placed = eventLanes(
    order.map(({ event }) => event),
    { lanes },
  );
  return (
    <>
      {order.map(({ event, i }, k) => (
        <span
          key={i}
          className={styles.event}
          data-tone={event.tone ?? "neutral"}
          style={{ left: event.x }}
          title={placed[k] === null ? event.label : undefined}
          data-broken={event.clear?.length ? true : undefined}
          aria-hidden="true"
        >
          {event.clear?.length ? (
            <LinePieces top={plot.top} bottom={plot.bottom} clear={event.clear} />
          ) : null}
          {event.label && placed[k] !== null && (
            <span
              className={`${styles.eventLabel} ${roles.micro}`}
              style={{ top: -18 - (placed[k] ?? 0) * 20 }}
            >
              {event.label}
            </span>
          )}
        </span>
      ))}
    </>
  );
}

/** A reference on a segmented meter: the gap between two segments glows (rule 9). `x` is the
 * gap's centre and `top`/`height` the segments' extent, frame px. */
export function LitGap({
  x,
  top,
  height,
  colour,
}: {
  x: number;
  top: number;
  height: number;
  colour: string;
}) {
  return (
    <span
      className={styles.litGap}
      style={
        {
          left: x,
          top: top - 4,
          height: height + 8,
          "--gap": colour,
        } as CSSProperties
      }
      aria-hidden="true"
    />
  );
}
