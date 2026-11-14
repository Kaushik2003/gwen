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

import { cssModule } from "./css-module";
import { sectorPath, TAU } from "./arc";
import Styles from "./core.module.css";
import type { ReactNode } from "react";
import Roles from "./type.module.css";

const styles = cssModule(Styles, "lc-core-");
const roles = cssModule(Roles, "lc-type-");

export type ChartStateKind = "loading" | "empty" | "error";

/** Which state a chart is in, and its message: loading wins, then error, then empty. Null when the
 * chart should draw its data. */
export function stateOf({
  loading,
  empty,
  error,
}: {
  loading?: boolean;
  empty?: boolean | ReactNode;
  error?: boolean | ReactNode;
}): { kind: ChartStateKind; message?: ReactNode } | null {
  if (loading) return { kind: "loading" };
  if (error) return { kind: "error", message: error === true ? undefined : error };
  if (empty) return { kind: "empty", message: empty === true ? undefined : empty };
  return null;
}

/**
 * A loading skeleton in a chart's own shape, given as geometry only: the core draws it, in one
 * tone, with 2px corners, pulsing for three breaths and then resting. Coordinates are in `box`
 * units (default `[0, 0, 100, 100]`), stretched over the plot; `contain` keeps the shapes' aspect
 * (a ring, a chord), centred. Every part is optional.
 */
export type SkeletonShape = {
  box?: readonly [x: number, y: number, width: number, height: number];
  contain?: boolean;
  /** Blocks (tiles, pills, nodes): `[x, y, width, height]`. */
  blocks?: readonly (readonly [x: number, y: number, width: number, height: number])[];
  /** Round beads and blobs centred on `[x, y]`, `diameter` in px. */
  dots?: readonly (readonly [x: number, y: number, diameter: number])[];
  /** A grid of cells, `[rows, columns]`, 3px apart, filling the plot. */
  grid?: readonly [rows: number, columns: number];
  /** Filled shapes (arcs, regions, discs), SVG path data. */
  shapes?: readonly string[];
  /** Fainter fills behind them (ribbons and flows). */
  flows?: readonly string[];
  /** Stroked lines (links and connectors). */
  lines?: readonly string[];
};

/** A loading block that fills its parent (a gauge's track, a readout's row), drawn like a
 * skeleton's blocks. */
export function SkeletonBlock() {
  return <span className={styles.skeletonFill} aria-hidden="true" />;
}

const DEFAULTS: Record<Exclude<ChartStateKind, "loading">, string> = {
  empty: "No data for this period",
  error: "This chart couldn’t load",
};

/**
 * What a chart shows instead of marks: a shimmering skeleton in the chart's own shape while
 * loading, or a short note when there is nothing to show or the data failed. Charts render it
 * inside their frame, where it fills the plot; `compact` is a single line for readouts and tiles.
 * - `bars`: columns of varied height. `rows`: horizontal bars. `ring`: a ring or an arc (`ring`
 *   sets its inner radius as a share of the outer, and where it starts and how far it spans).
 *   `line`: a soft wave. `points`: a soft scatter of blobs. `swarm`: blobs in rows, for strips and
 *   swarms.
 * - `skeleton`: any other shape, as geometry (`SkeletonShape`); it wins over `shape`.
 */
export function ChartState({
  kind,
  message,
  shape = "bars",
  skeleton,
  bars = 7,
  ring = {},
  compact = false,
}: {
  kind: ChartStateKind;
  message?: ReactNode;
  shape?: "bars" | "rows" | "ring" | "line" | "points" | "swarm";
  /** The chart's own skeleton, as geometry. */
  skeleton?: SkeletonShape;
  /** Skeleton columns or rows while loading. */
  bars?: number;
  ring?: { inner?: number; from?: number; span?: number };
  compact?: boolean;
}) {
  if (compact)
    return kind === "loading" ? (
      <span className={styles.compactLoading} role="status" aria-label="Loading">
        <span className={styles.shimmerLine} />
      </span>
    ) : (
      <span
        className={`${styles.compactText} ${roles.caption}`}
        data-kind={kind}
        role={kind === "error" ? "alert" : "status"}
      >
        {message ?? DEFAULTS[kind]}
      </span>
    );
  if (kind !== "loading")
    return (
      <div
        className={`${styles.state} ${roles.label}`}
        role={kind === "error" ? "alert" : "status"}
      >
        {message ?? DEFAULTS[kind]}
      </div>
    );
  return (
    <div className={styles.state} role="status" aria-label="Loading chart">
      {skeleton ? (
        <Skeleton {...skeleton} />
      ) : shape === "swarm" ? (
        <div className={styles.skeleton} data-shape="points" aria-hidden="true">
          {Array.from({ length: 27 }, (_, i) => (
            <span
              key={i}
              style={{
                left: `${6 + ((i * 37) % 88)}%`,
                top: `${Math.min(92, 18 + Math.floor(i / 9) * 32 + ((i * 13) % 12))}%`,
                width: 10,
                height: 10,
                animationDelay: `${(i % 6) * 0.12}s`,
              }}
            />
          ))}
        </div>
      ) : shape === "ring" ? (
        <RingSkeleton {...ring} />
      ) : shape === "points" ? (
        <div className={styles.skeleton} data-shape="points" aria-hidden="true">
          {Array.from({ length: Math.max(bars, 7) * 2 + 4 }, (_, i) => {
            const t = (i * 0.618034) % 1;
            const size = 10 + ((i * 7) % 4) * 3;
            return (
              <span
                key={i}
                style={{
                  left: `${6 + t * 86}%`,
                  top: `${Math.max(4, Math.min(90, 82 - t * 62 + ((i * 29) % 22) - 11))}%`,
                  width: size,
                  height: size,
                  animationDelay: `${(i % 6) * 0.12}s`,
                }}
              />
            );
          })}
        </div>
      ) : shape === "line" ? (
        <svg
          className={styles.lineSkeleton}
          viewBox="0 0 100 40"
          preserveAspectRatio="none"
          aria-hidden="true"
        >
          <path d="M0 28 C 12 18, 22 30, 34 22 S 56 8, 68 18 S 88 30, 100 14" />
        </svg>
      ) : (
        <div className={styles.skeleton} data-shape={shape} aria-hidden="true">
          {Array.from({ length: bars }, (_, i) => (
            <span
              key={i}
              style={
                shape === "rows"
                  ? { width: `${92 - ((i * 23) % 58)}%` }
                  : { height: `${36 + ((i * 37) % 52)}%` }
              }
            />
          ))}
        </div>
      )}
    </div>
  );
}

const pct = (value: number, from: number, span: number) => `${((value - from) / span) * 100}%`;

/** A family's skeleton geometry, drawn in the core's tone, corners and pulse. */
function Skeleton({
  box = [0, 0, 100, 100],
  contain = false,
  blocks = [],
  dots = [],
  grid,
  shapes = [],
  flows = [],
  lines = [],
}: SkeletonShape) {
  const [bx, by, bw, bh] = box;
  const w = Math.max(1e-6, bw),
    h = Math.max(1e-6, bh);
  const delay = (i: number) => `${(i % 6) * 0.12}s`;
  const rows = grid ? Math.max(1, Math.round(grid[0])) : 0,
    columns = grid ? Math.max(1, Math.round(grid[1])) : 0;
  return (
    <div className={styles.skeletonShape} aria-hidden="true">
      {(shapes.length > 0 || flows.length > 0 || lines.length > 0) && (
        <svg
          viewBox={`${bx} ${by} ${w} ${h}`}
          preserveAspectRatio={contain ? "xMidYMid meet" : "none"}
        >
          {flows.map((d, i) => (
            <path key={`f${i}`} className={styles.skeletonFlow} d={d} />
          ))}
          {shapes.map((d, i) => (
            <path key={`s${i}`} d={d} style={{ animationDelay: delay(i) }} />
          ))}
          {lines.map((d, i) => (
            <path key={`l${i}`} className={styles.skeletonLine} d={d} />
          ))}
        </svg>
      )}
      {rows > 0 && (
        <div
          className={styles.skeletonGrid}
          style={{
            gridTemplateRows: `repeat(${rows}, 1fr)`,
            gridTemplateColumns: `repeat(${columns}, 1fr)`,
          }}
        >
          {Array.from({ length: rows * columns }, (_, i) => (
            // A wave from the top left corner.
            <span
              key={i}
              style={{ animationDelay: `${((i % columns) + Math.floor(i / columns)) * 40}ms` }}
            />
          ))}
        </div>
      )}
      {blocks.map(([x, y, bwidth, bheight], i) => (
        <span
          key={`b${i}`}
          className={styles.skeletonBlock}
          style={{
            left: pct(x, bx, w),
            top: pct(y, by, h),
            width: pct(bwidth, 0, w),
            height: pct(bheight, 0, h),
            animationDelay: delay(i),
          }}
        />
      ))}
      {dots.map(([x, y, diameter], i) => (
        <span
          key={`d${i}`}
          className={styles.skeletonDot}
          style={{
            left: pct(x, bx, w),
            top: pct(y, by, h),
            width: diameter,
            height: diameter,
            animationDelay: delay(i),
          }}
        />
      ))}
    </div>
  );
}

function RingSkeleton({
  inner = 0.62,
  from = 0,
  span = TAU,
}: {
  inner?: number;
  from?: number;
  span?: number;
}) {
  // The view box hugs the arc, so a gauge or a 270° dial fills the space without being cropped.
  let minX = 50,
    maxX = 50,
    minY = 50,
    maxY = 50;
  for (let i = 0; i <= 48; i++) {
    const a = from + (span * i) / 48;
    for (const r of [50 * inner, 50]) {
      const x = 50 + r * Math.sin(a),
        y = 50 - r * Math.cos(a);
      minX = Math.min(minX, x);
      maxX = Math.max(maxX, x);
      minY = Math.min(minY, y);
      maxY = Math.max(maxY, y);
    }
  }
  const pad = 2;
  return (
    <svg
      className={styles.ringSkeleton}
      viewBox={`${minX - pad} ${minY - pad} ${maxX - minX + pad * 2} ${maxY - minY + pad * 2}`}
      aria-hidden="true"
    >
      <path d={sectorPath(50, 50, 50 * inner, 50, from, from + span, 4)} fillRule="evenodd" />
    </svg>
  );
}
