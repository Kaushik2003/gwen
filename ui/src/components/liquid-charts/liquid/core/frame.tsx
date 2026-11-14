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

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useId,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent,
  type ReactNode,
} from "react";
import { plotRect, type Insets, type Rect, type Size } from "./geometry";
import { FrameSensesContext, type FrameSenses } from "./frame-senses";
import { pointerFocus } from "./liquid-pointer";
import { useMaterials } from "./materials";
import { cssModule } from "./css-module";
import { createPortal } from "react-dom";
import Styles from "./core.module.css";
import Roles from "./type.module.css";
import type { Theme } from "./colour";

const styles = cssModule(Styles, "lc-core-");
const roles = cssModule(Roles, "lc-type-");

type FrameContext = {
  theme: Theme;
  size: Size;
  insets: Insets;
  /** The plot inside the insets, in frame px. */
  plot: Rect;
  active: number | null;
};
const Context = createContext<FrameContext | null>(null);

let scorecardChecked = false;
/** A tally for the material scorecard (src/scripts/scorecard.ts), only on a page whose root carries
 * `data-scorecard`: `placeLabels` and `resolveStyle` write into it. */
function scorecardTally() {
  if (scorecardChecked || typeof document === "undefined") return;
  scorecardChecked = true;
  if (document.documentElement.hasAttribute("data-scorecard"))
    Object.assign(window, { __liquidScorecard: { labels: new Map(), resolved: new Set() } });
}

/** The frame a chart part sits in: its size, insets, plot rect, theme and the item in focus. */
export function useChartFrame() {
  const frame = useContext(Context);
  if (!frame) throw new Error("Chart parts must be rendered inside <ChartFrame>.");
  return frame;
}

/** The item in focus: controlled when `value` is passed, otherwise held here. */
export function useActive(value?: number | null, onChange?: (index: number | null) => void) {
  const [inner, setInner] = useState<number | null>(null);
  const active = value === undefined ? inner : value;
  const set = useCallback(
    (index: number | null) => {
      if (value === undefined) setInner(index);
      onChange?.(index);
    },
    [value, onChange],
  );
  return [active, set] as const;
}

/** Chart colours, type and theme for anything around a chart (a card header, a legend). The
 * frame sets the same scope on itself. */
export function ChartScope({
  theme,
  className,
  children,
}: {
  theme: Theme;
  className?: string;
  children: ReactNode;
}) {
  // The chart's last line: its credit, centred under everything in the scope (the legend
  // included), when a frame inside sends one here.
  const [creditLine, setCreditLine] = useState<HTMLElement | null>(null);
  return (
    <div className={`${styles.frame} ${roles.root} ${className ?? ""}`} data-scheme={theme}>
      <CreditLineContext.Provider value={creditLine}>{children}</CreditLineContext.Provider>
      <div ref={setCreditLine} className={styles.creditLine} />
    </div>
  );
}

/** The scope's credit line in reach, for the frame to send its credit to. */
const CreditLineContext = createContext<HTMLElement | null>(null);

export type DataTable = {
  caption: string;
  columns: readonly string[];
  rows: readonly (readonly (string | number)[])[];
};

/**
 * The plot every chart draws in. It measures itself, reserves `insets` for axes, and owns focus:
 * the pointer and the arrow keys move it, a click pins it (a second click lets go), Escape clears
 * it, and an `aria-live` line reads the focused item aloud. A data table gives screen readers the
 * whole set. `help` says how to work the chart (its keys and clicks) as the plot's accessible
 * description; it is never drawn, so no hint line sits on the card. Inside a `ChartCursor` the plot
 * also takes the glass cursor and, when the visitor has turned them on, sounds and haptics as the
 * focus moves (pointer or keyboard, never on scroll) and as an item is pinned and let go.
 */
export function ChartFrame({
  theme,
  height,
  insets,
  count,
  active,
  onActiveChange,
  pick,
  label,
  help,
  describe,
  table,
  plotRef,
  onPointer,
  keys,
  credit,
  className,
  children,
}: {
  theme: Theme;
  height: number;
  insets: Insets;
  /** Items the keyboard steps through. */
  count: number;
  active: number | null;
  onActiveChange: (index: number | null) => void;
  /** The item under a point in frame px, or null. */
  pick: (x: number, y: number, size: Size) => number | null;
  label: string;
  /** How to work the chart ("Arrow keys move between parts; Enter opens one"), read as the plot's
   * accessible description. Never shown. */
  help?: string;
  /** One sentence for the focused item, read aloud. */
  describe?: (index: number) => string;
  table?: DataTable;
  /** The mark layer's box ref, so the canvas and the frame measure the same element. Typed by its
   * shape, which `useRef(null)` returns under React 18's types and 19's alike. */
  plotRef?: { readonly current: HTMLDivElement | null };
  /** Raw pointer positions in frame px (null on leave) and the frame's size, for effects such as
   * a liquid ripple or a reading between items. */
  onPointer?: (point: { x: number; y: number } | null, size: Size) => void;
  /** Keys checked before the defaults: return the index to move to (null clears) to handle a key,
   * or undefined to fall through. A second arrow axis (Up and Down between series) goes here. */
  keys?: (
    key: string,
    active: number | null,
    event: KeyboardEvent<HTMLDivElement>,
  ) => number | null | undefined;
  /** The chart's last line, centred, never on the plot: the Liquid credit a source-available chart
   * carries (`<LiquidCredit />`). Without one, the frame shows its material pack's, if that has
   * one. Inside a `ChartScope` it goes to the scope's end, under the legend. */
  credit?: ReactNode;
  className?: string;
  children: ReactNode;
}) {
  scorecardTally();
  const PackCredit = useMaterials().Credit;
  // The chart's last line, centred: under everything in its ChartScope (a legend included), or
  // under the plot when the frame stands alone.
  const shownCredit = credit ?? (PackCredit ? <PackCredit /> : null);
  const creditLine = useContext(CreditLineContext);
  const own = useRef<HTMLDivElement>(null);
  const ref = plotRef ?? own;
  const [size, setSize] = useState<Size>({ width: 0, height });
  const pinned = useRef<number | null>(null);
  const helpId = useId();

  useEffect(() => {
    const element = ref.current;
    if (!element) return;
    const observer = new ResizeObserver(([entry]) => {
      const { width, height } = entry.contentRect;
      setSize((size) =>
        size.width === width && size.height === height ? size : { width, height },
      );
    });
    observer.observe(element);
    return () => observer.disconnect();
  }, [ref]);

  const senses = useContext(FrameSensesContext);
  const cursor = senses?.cursor;
  useEffect(() => {
    const element = ref.current;
    if (!element || !cursor) return;
    return cursor.attach(element, theme);
  }, [ref, cursor, theme]);

  const makeFeedback = senses?.feedback;
  const feedback = useRef<ReturnType<NonNullable<FrameSenses["feedback"]>> | null>(null);
  useEffect(() => {
    const element = ref.current;
    if (!element || !makeFeedback) return;
    const own = makeFeedback(element);
    feedback.current = own;
    return () => {
      own.stop();
      feedback.current = null;
    };
  }, [ref, makeFeedback]);
  const prime = () => feedback.current?.prime();
  // What the pointer gives the focus: the item under it on a move, and on a mouse's arrival
  // without one (the browser puts a still pointer back over the plot with pointerover alone when
  // something above the chart goes away); quiet when the page moved under a still pointer.
  const [takes] = useState(pointerFocus);
  // The arrival listener is native: React's own enter fires from a pointerout it can see, and
  // the one an overlay above the chart fires as it is removed never reaches the root. The ref
  // holds the latest handler, so the listener attaches once and never reads a stale focus.
  const arrive = useRef<(event: PointerEvent) => void>(() => {});
  useEffect(() => {
    const element = ref.current;
    if (!element) return;
    const over = (event: PointerEvent) => arrive.current(event);
    element.addEventListener("pointerover", over);
    return () => element.removeEventListener("pointerover", over);
  }, [ref]);

  const move = (index: number | null, quiet = false) => {
    if (index === active) return;
    onActiveChange(index);
    if (index !== null) feedback.current?.focus(quiet);
  };
  const at = (event: { clientX: number; clientY: number; currentTarget: HTMLElement }) => {
    const r = event.currentTarget.getBoundingClientRect();
    return pick(event.clientX - r.left, event.clientY - r.top, {
      width: r.width,
      height: r.height,
    });
  };
  useEffect(() => {
    arrive.current = (event) => {
      // From inside the plot (a child, which never takes events anyway) nothing arrived.
      const from = event.relatedTarget;
      const plot = event.currentTarget as HTMLElement;
      if (from instanceof Node && plot.contains(from)) return;
      const take = takes("enter", event);
      if (!take) return;
      move(
        at({ clientX: event.clientX, clientY: event.clientY, currentTarget: plot }) ??
          pinned.current,
        take.quiet,
      );
    };
  });
  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    prime();
    const custom = keys?.(event.key, active, event);
    if (custom !== undefined) {
      event.preventDefault();
      move(custom);
      return;
    }
    const from = active ?? -1;
    let next: number | null;
    if (event.key === "ArrowRight" || event.key === "ArrowDown") next = (from + 1) % count;
    else if (event.key === "ArrowLeft" || event.key === "ArrowUp")
      next = (from - 1 + count) % count;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = count - 1;
    else if (event.key === "Escape") {
      if (pinned.current !== null) feedback.current?.pin(false);
      pinned.current = null;
      next = null;
    } else return;
    event.preventDefault();
    move(next);
  };

  const plot = plotRect(size, insets);
  return (
    <div className={`${styles.frame} ${roles.root} ${className ?? ""}`} data-scheme={theme}>
      <div
        ref={ref}
        className={styles.plot}
        role="group"
        tabIndex={0}
        aria-label={label}
        aria-describedby={help ? helpId : undefined}
        style={
          {
            height,
            "--plot-left": `${insets.left}px`,
            "--plot-right": `${insets.right}px`,
            "--plot-top": `${insets.top}px`,
            "--plot-bottom": `${insets.bottom}px`,
          } as CSSProperties
        }
        onPointerMove={(event) => {
          const take = takes("move", event);
          if (take) move(at(event) ?? pinned.current, take.quiet);
          if (onPointer) {
            const r = event.currentTarget.getBoundingClientRect();
            onPointer(
              { x: event.clientX - r.left, y: event.clientY - r.top },
              { width: r.width, height: r.height },
            );
          }
        }}
        onPointerDown={makeFeedback ? prime : undefined}
        onPointerLeave={() => {
          move(pinned.current);
          onPointer?.(null, size);
        }}
        onClick={(event) => {
          const index = at(event);
          pinned.current = index === pinned.current ? null : index;
          if (index !== null) feedback.current?.pin(pinned.current !== null);
          move(index);
        }}
        onKeyDown={onKeyDown}
      >
        {/* The plot's preferred width (core.module.css, `strut`), for a parent that shrink-wraps. */}
        <svg className={styles.strut} aria-hidden="true" focusable="false" />
        <Context.Provider value={{ theme, size, insets, plot, active }}>
          {children}
        </Context.Provider>
        <p className={styles.srOnly} aria-live="polite">
          {active === null || !describe ? "" : describe(active)}
        </p>
      </div>
      {shownCredit &&
        (creditLine ? (
          createPortal(shownCredit, creditLine)
        ) : (
          <div className={styles.creditLine}>{shownCredit}</div>
        ))}
      {help && (
        <p id={helpId} className={styles.srOnly}>
          {help}
        </p>
      )}
      {table && (
        // A table ignores a 1px width, so the hiding box wraps it: hidden alone, it would still widen
        // and scroll whatever contains the chart.
        <div className={styles.srOnly}>
          <table>
            <caption>{table.caption}</caption>
            <thead>
              <tr>
                {table.columns.map((column) => (
                  <th key={column} scope="col">
                    {column}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {table.rows.map((row, i) => (
                <tr key={i}>
                  {row.map((cell, j) =>
                    j === 0 ? (
                      <th key={j} scope="row">
                        {cell}
                      </th>
                    ) : (
                      <td key={j}>{cell}</td>
                    ),
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
