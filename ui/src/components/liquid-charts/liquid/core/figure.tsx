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

import { memo, useLayoutEffect, useRef, useState } from "react";
import { cssModule } from "./css-module";
import Styles from "./type.module.css";

const styles = cssModule(Styles, "lc-type-");

export type FigureKind = "currency" | "currencyCompact" | "percent" | "compact" | "number";

const formatters = {
  currency: (decimals: number, currency: string) =>
    new Intl.NumberFormat("en-US", {
      style: "currency",
      currency,
      minimumFractionDigits: decimals,
      maximumFractionDigits: decimals,
    }),
  currencyCompact: (decimals: number, currency: string) =>
    new Intl.NumberFormat("en-US", {
      style: "currency",
      currency,
      notation: "compact",
      // Explicit, or WebKit pads to the maximum ($2.20M) while Node and Chromium don't, and
      // server and client render different text.
      minimumFractionDigits: 0,
      maximumFractionDigits: decimals,
    }),
  percent: (decimals: number) =>
    new Intl.NumberFormat("en-US", {
      style: "percent",
      minimumFractionDigits: decimals,
      maximumFractionDigits: decimals,
    }),
  compact: (decimals: number) =>
    new Intl.NumberFormat("en-US", {
      notation: "compact",
      minimumFractionDigits: 0,
      maximumFractionDigits: decimals,
    }),
  number: (decimals: number) =>
    new Intl.NumberFormat("en-US", {
      minimumFractionDigits: decimals,
      maximumFractionDigits: decimals,
    }),
};

/** One formatter per kind, decimals and currency for the page: building an Intl.NumberFormat costs
 * far more than using one. */
const formats = new Map<string, Intl.NumberFormat>();
function formatterFor(kind: FigureKind, decimals: number, currency = "USD") {
  const key = `${kind}:${decimals}:${currency}`;
  let format = formats.get(key);
  if (!format) formats.set(key, (format = formatters[kind](decimals, currency)));
  return format;
}

const DIGITS = "0123456789";
// Until the face is measured, every digit takes a typical figure width.
const FALLBACK = Array.from({ length: 10 }, () => 0.55);
const measured = new Map<string, number[]>();
// One measuring context for the page, made on first use.
let measuring: CanvasRenderingContext2D | null | undefined;
/** Mounted rolling digits that re-measure when webfonts arrive. The fonts are watched once for
 * the page, not once per figure. */
const remeasure = new Set<() => void>();
let fontsWatched = false;
// Module level, like the font listener itself: a function made inside `watchFonts` (inlined into
// the first figure's effect when minified) would keep that figure's detached page alive.
function refreshDigits() {
  measured.clear();
  remeasure.forEach((measure) => measure());
}
function watchFonts() {
  if (fontsWatched || typeof document === "undefined" || !document.fonts) return;
  fontsWatched = true;
  document.fonts.ready.then(refreshDigits);
  document.fonts.addEventListener?.("loadingdone", refreshDigits);
}

/** Each digit's advance in em for the element's face, measured once per font. */
export function digitWidths(element: HTMLElement) {
  const style = getComputedStyle(element);
  const key = `${style.fontWeight} ${style.fontFamily}`;
  const known = measured.get(key);
  if (known) return known;
  if (measuring === undefined) measuring = document.createElement("canvas").getContext("2d");
  const context = measuring;
  if (!context) return FALLBACK;
  context.font = `${style.fontWeight} 100px ${style.fontFamily}`;
  const widths = [...DIGITS].map((digit) => context.measureText(digit).width / 100);
  measured.set(key, widths);
  return widths;
}

/**
 * The whole part of a figure. It is plain text until its value first changes: most figures never
 * roll, and plain text is one node where rolling digits are eleven per digit. On the first change
 * the rolling digits mount at the old value and roll to the new one.
 */
const Whole = memo(function Whole({ text }: { text: string }) {
  const [initial] = useState(text);
  const [rolling, setRolling] = useState(false);
  // An update during render, from the previous value (React's derived-state pattern).
  if (!rolling && text !== initial) setRolling(true);
  if (!rolling && text === initial)
    return (
      <span className={styles.digits} aria-hidden="true">
        <span className={styles.glyph}>{text}</span>
      </span>
    );
  return <Digits text={text} from={initial} />;
});

/** Rolling digits that keep the face's own spacing: each digit is as wide as itself, and the
 * width eases with the roll, so a 1 never sits in a 0's box. Until they are measured and ready
 * they show `from` (the value they roll from). */
function Digits({ text, from = text }: { text: string; from?: string }) {
  const root = useRef<HTMLSpanElement>(null);
  const [widths, setWidths] = useState(FALLBACK);
  const [ready, setReady] = useState(false);
  useLayoutEffect(() => {
    const element = root.current;
    if (!element) return;
    let live = true;
    const measure = () => {
      if (live) setWidths(digitWidths(element));
    };
    measure();
    requestAnimationFrame(() => live && setReady(true));
    // Webfonts can arrive after the first paint; measure again once they have.
    watchFonts();
    remeasure.add(measure);
    return () => {
      live = false;
      remeasure.delete(measure);
    };
  }, []);
  const shown = ready ? text : from;
  return (
    <span ref={root} className={styles.digits} data-ready={ready} aria-hidden="true">
      {[...shown].map((char, i) => {
        const digit = DIGITS.indexOf(char);
        if (digit < 0) {
          return (
            <span key={`${i}-${char}`} className={styles.glyph}>
              {char}
            </span>
          );
        }
        return (
          <span key={`${i}-d`} className={styles.digit} style={{ width: `${widths[digit]}em` }}>
            <span className={styles.strip} style={{ transform: `translateY(${-digit * 10}%)` }}>
              {[...DIGITS].map((value) => (
                <span key={value}>{value}</span>
              ))}
            </span>
          </span>
        );
      })}
    </span>
  );
}

/** A figure as plain text, for aria labels, data tables and titles: "−$6.2K", "412 ms". */
export function formatFigure(
  value: number,
  {
    kind = "currency",
    decimals = 0,
    signed = false,
    unit,
    currency,
  }: {
    kind?: FigureKind;
    decimals?: number;
    signed?: boolean;
    unit?: string;
    currency?: string;
  } = {},
) {
  const sign = value < 0 ? "−" : signed ? "+" : "";
  return `${sign}${formatterFor(kind, decimals, currency).format(Math.abs(value))}${unit ? ` ${unit}` : ""}`;
}

/** One number, set as a figure: the whole part leads; the currency sign, unit and decimals are
 * smaller and quieter; a sign uses a true minus. The whole part rolls when it changes. Percent
 * values are given as fractions (0.295 for 29.5%). Memoized: a figure re-renders only when its
 * own props change. */
export const Figure = memo(function Figure({
  value,
  kind = "currency",
  decimals = 0,
  signed = false,
  unit,
  currency,
  className,
}: {
  value: number;
  kind?: FigureKind;
  decimals?: number;
  signed?: boolean;
  /** A unit after the number ("ms", "TB", "kcal"), set small and quiet like a currency sign. */
  unit?: string;
  /** The ISO 4217 code a currency kind is in ("GBP", "EUR"). Default USD. */
  currency?: string;
  className?: string;
}) {
  const format = formatterFor(kind, decimals, currency);
  const parts = format.formatToParts(Math.abs(value));
  const whole = parts
    .filter((part) => part.type === "integer" || part.type === "group")
    .map((part) => part.value)
    .join("");
  const fraction = parts.find((part) => part.type === "fraction")?.value;
  const before = parts.find((part) => part.type === "currency")?.value;
  const after = parts
    .filter((part) => part.type === "percentSign" || part.type === "compact")
    .map((part) => part.value)
    .join("");
  const sign = value < 0 ? "−" : signed ? "+" : "";
  return (
    <span className={`${styles.figure} ${className ?? ""}`} data-kind={kind}>
      <span className={styles.srOnly}>
        {value < 0 ? "-" : sign}
        {format.format(Math.abs(value))}
        {unit ? ` ${unit}` : ""}
      </span>
      <span className={styles.figureFace} aria-hidden="true">
        {sign && <span className={styles.sign}>{sign}</span>}
        {before && <span className={styles.unit}>{before}</span>}
        <Whole text={whole} />
        {fraction && <span className={styles.minor}>.{fraction}</span>}
        {after && <span className={styles.unit}>{after}</span>}
        {unit && <span className={`${styles.unit} ${styles.suffix}`}>{unit}</span>}
      </span>
    </span>
  );
});
