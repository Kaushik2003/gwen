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
/**
 * Colour scales for marks where colour is the value (style rule 1, so the marks are flat): heatmap
 * and calendar cells, choropleth regions, tile-map tiles.
 *
 * Stops come only from `ChartConfig`: a sequential scale reads one key's colours (low to high; a
 * single colour ramps up from a faint tint of itself on the card), a diverging scale reads two
 * keys, one for each side of a meaningful midpoint (an SLO, a target, zero), each ramping out from
 * a neutral near the card's own colour. Ramps interpolate in OKLab, so equal steps in value look
 * like equal steps in colour, and a diverging midpoint sits at its true place on the domain.
 *
 * Ramps start from the card (pure neutral greys, the frame's tokens; colour belongs to the data
 * alone). A `ScaleTone` sets how far a ramp's faint end and diverging neutral lift off the card:
 * `CARD_TONE` (the default) for cells on a grid, `LIFTED_TONE` for places on a map, which have no
 * grid around them to say where they are.
 */

import { cssToRgb, seriesColours, type ChartConfig, type Rgb, type Theme } from "./colour";
import { clamp01, finite } from "./geometry";

export type SequentialScale = {
  kind: "sequential";
  /** The config key whose colours are the stops, low to high. Default: the first config key. */
  key?: string;
  /** Default: zero (when every value is zero or more) or the lowest value, to the highest. */
  domain?: readonly [number, number];
};
export type DivergingScale = {
  kind: "diverging";
  /** Config keys for the side below the midpoint and the side above it. Each key's colours run
   * from the midpoint outward. */
  keys: readonly [string, string];
  /** The value that means "neither": an SLO, a target, zero (the default). */
  midpoint?: number;
  domain?: readonly [number, number];
  /** Give both sides the same reach from the midpoint, so equal colours mean equal distances.
   * By default each side spans its own extent (the best and worst read at full strength). The
   * domain (and the legend) stay the values' extent either way. */
  symmetric?: boolean;
};
export type HeatScale = SequentialScale | DivergingScale;

export type ColourScale = {
  kind: HeatScale["kind"];
  domain: [number, number];
  /** Diverging only. */
  midpoint: number | null;
  /** 0–1 for a sequential scale; −1–1 for a diverging one (negative below the midpoint). */
  intensity: (value: number) => number;
  colour: (value: number) => Rgb;
  /** The strongest colour on the value's side: the tint a glass cell or bead takes. */
  tint: (value: number) => Rgb;
  /** Where a value sits along the legend, 0–1 (linear in value). */
  offset: (value: number) => number;
  /** Stops for the legend's gradient, linear in value, with one exactly at the midpoint. */
  gradient: (samples?: number) => { offset: number; colour: Rgb }[];
};

/** The card each theme's charts sit on, and its ink: the ends a ramp starts from. Pure neutral
 * greys, matching the frame's tokens. */
export const SURFACE: Record<Theme, Rgb> = {
  dark: [12 / 255, 12 / 255, 12 / 255],
  light: [1, 1, 1],
};
export const INK: Record<Theme, Rgb> = {
  dark: [243 / 255, 243 / 255, 243 / 255],
  light: [20 / 255, 20 / 255, 20 / 255],
};

/** How far a ramp's quiet ends lift off the card. */
export type ScaleTone = {
  /** How strongly a single colour's faint end is tinted over the card. Enough that a zero still
   * reads as a mark, not as a hole. */
  faint: Record<Theme, number>;
  /** The diverging neutral: the card, lifted this far towards the ink. */
  neutral: Record<Theme, number>;
};

/** Cells on a grid: the grid already says where every cell is, so the quiet ends sit close to the
 * card. */
export const CARD_TONE: ScaleTone = {
  faint: { dark: 0.17, light: 0.11 },
  neutral: { dark: 0.11, light: 0.075 },
};

/** Places on a map: a region has no grid around it, so the faint end and the diverging neutral
 * are lifted clear of the card (a zero reads as a place, not a hole in the map). */
export const LIFTED_TONE: ScaleTone = {
  faint: { dark: 0.2, light: 0.13 },
  neutral: { dark: 0.17, light: 0.1 },
};

/* ——— OKLab ——— */

const toLinear = (c: number) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4);
const toGamma = (c: number) => (c <= 0.0031308 ? 12.92 * c : 1.055 * c ** (1 / 2.4) - 0.055);

/** sRGB (0–1) to OKLab. */
export function toOklab([r, g, b]: Rgb): [number, number, number] {
  const lr = toLinear(r),
    lg = toLinear(g),
    lb = toLinear(b);
  const l = Math.cbrt(0.4122214708 * lr + 0.5363325363 * lg + 0.0514459929 * lb);
  const m = Math.cbrt(0.2119034982 * lr + 0.6806995451 * lg + 0.1073969566 * lb);
  const s = Math.cbrt(0.0883024619 * lr + 0.2817188376 * lg + 0.6299787005 * lb);
  return [
    0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s,
    1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s,
    0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s,
  ];
}

/** OKLab to sRGB (0–1), clipped into gamut. */
export function fromOklab([L, a, b]: readonly [number, number, number]): Rgb {
  const l = (L + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const m = (L - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const s = (L - 0.0894841775 * a - 1.291485548 * b) ** 3;
  return [
    clamp01(toGamma(clamp01(4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s))),
    clamp01(toGamma(clamp01(-1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s))),
    clamp01(toGamma(clamp01(-0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s))),
  ];
}

/** Mixes two colours in OKLab: `t` 0 is `a`, 1 is `b`. */
export function mixLab(a: Rgb, b: Rgb, t: number): Rgb {
  if (t <= 0) return a;
  if (t >= 1) return b;
  const p = toOklab(a),
    q = toOklab(b);
  return fromOklab([p[0] + (q[0] - p[0]) * t, p[1] + (q[1] - p[1]) * t, p[2] + (q[2] - p[2]) * t]);
}

/** A colour along evenly spaced stops, `t` 0–1 (clamped). One stop is a flat colour. */
export function ramp(stops: readonly Rgb[], t: number): Rgb {
  if (!stops.length) return [0.5, 0.5, 0.5];
  if (stops.length === 1) return stops[0];
  const at = clamp01(t) * (stops.length - 1);
  const i = Math.min(stops.length - 2, Math.floor(at));
  return mixLab(stops[i], stops[i + 1], at - i);
}

/** WCAG relative luminance of an sRGB colour. */
export function luminance([r, g, b]: Rgb): number {
  return 0.2126 * toLinear(r) + 0.7152 * toLinear(g) + 0.0722 * toLinear(b);
}

/** Whether text on a cell of this colour should be dark (light cells) or light. */
export const inkOn = (colour: Rgb): "dark" | "light" =>
  luminance(colour) > 0.36 ? "dark" : "light";

/** Out of focus: the mark's own lightness as a neutral graphite, eased back towards the card, so
 * the pattern still reads but the marks in focus carry the colour. */
export function dimmed(colour: Rgb, theme: Theme, amount: number): Rgb {
  if (amount <= 0) return colour;
  const grey = 0.3 * colour[0] + 0.59 * colour[1] + 0.11 * colour[2];
  const surface = SURFACE[theme];
  const back = theme === "dark" ? 0.42 : 0.5;
  const target: Rgb = [
    grey + (surface[0] - grey) * back,
    grey + (surface[1] - grey) * back,
    grey + (surface[2] - grey) * back,
  ];
  return [
    colour[0] + (target[0] - colour[0]) * amount,
    colour[1] + (target[1] - colour[1]) * amount,
    colour[2] + (target[2] - colour[2]) * amount,
  ];
}

export const cssRgb = ([r, g, b]: Rgb, alpha = 1) =>
  alpha >= 1
    ? `rgb(${Math.round(r * 255)} ${Math.round(g * 255)} ${Math.round(b * 255)})`
    : `rgb(${Math.round(r * 255)} ${Math.round(g * 255)} ${Math.round(b * 255)} / ${Math.round(alpha * 1000) / 1000})`;

/* ——— Domains and scales ——— */

/**
 * The value domain a scale spans. Sequential: from zero when every value is zero or more
 * (counts and amounts read honestly from nothing), else from the lowest value, to the highest.
 * Diverging: the values' extent with the midpoint inside it. A given domain wins. Empty or flat input still gives a usable, non-zero span.
 */
export function scaleDomain(values: readonly unknown[], scale: HeatScale): [number, number] {
  if (scale.domain) return [scale.domain[0], scale.domain[1]];
  let lo = Infinity,
    hi = -Infinity;
  for (const v of values)
    if (finite(v)) {
      if (v < lo) lo = v;
      if (v > hi) hi = v;
    }
  if (scale.kind === "sequential") {
    if (lo > hi) return [0, 1];
    if (lo >= 0) lo = 0;
    if (lo === hi) return lo === 0 ? [0, 1] : [lo, lo + Math.abs(lo)];
    return [lo, hi];
  }
  const mid = scale.midpoint ?? 0;
  if (lo > hi) lo = hi = mid;
  lo = Math.min(lo, mid);
  hi = Math.max(hi, mid);
  if (lo === hi) {
    const pad = Math.abs(mid) / 2 || 1;
    return [mid - pad, mid + pad];
  }
  return [lo, hi];
}

/** How far each side of a diverging scale reaches from its midpoint: its own extent, or (when
 * `symmetric`) the farther side's for both, so equal colours mean equal distances. The domain
 * itself stays the values' extent, so a legend never shows values that cannot happen. */
export function divergingReach(
  domain: readonly [number, number],
  midpoint: number,
  symmetric = false,
): { below: number; above: number } {
  const below = Math.max(0, midpoint - domain[0]),
    above = Math.max(0, domain[1] - midpoint);
  if (!symmetric) return { below, above };
  const reach = Math.max(below, above);
  return { below: reach, above: reach };
}

/** A ramp's stops from a key's colours: several are used as given; one ramps up from a faint
 * tint of itself on the card. */
export function sequentialStops(colours: readonly Rgb[], theme: Theme, tone = CARD_TONE): Rgb[] {
  if (colours.length > 1) return [...colours];
  const top = colours[0] ?? INK[theme];
  return [mixLab(SURFACE[theme], top, tone.faint[theme]), top];
}

/** One side of a diverging ramp: from the neutral out through the key's colours. */
export function divergingStops(colours: readonly Rgb[], theme: Theme, tone = CARD_TONE): Rgb[] {
  const neutral = mixLab(SURFACE[theme], INK[theme], tone.neutral[theme]);
  return [neutral, ...(colours.length ? colours : [INK[theme]])];
}

/** A key's colours for a theme as Rgb (hex and opaque rgb() resolve anywhere; other CSS colours
 * need a browser). Unknown colours are skipped. */
export function configColours(
  config: ChartConfig,
  key: string,
  theme: Theme,
  scope?: Element | null,
): Rgb[] {
  return seriesColours(config, key, theme)
    .map((colour) => cssToRgb(colour, scope))
    .filter((rgb): rgb is Rgb => rgb !== null);
}

/** The config keys a scale reads. */
export const scaleKeys = (scale: HeatScale, config: ChartConfig): string[] =>
  scale.kind === "diverging" ? [...scale.keys] : [scale.key ?? Object.keys(config)[0] ?? ""];

/**
 * A colour scale over the values. `low` and `high` are the configured colours (for a sequential
 * scale only `high` is read); pass them from `configColours` so they follow the theme. `tone`
 * sets how far its quiet ends lift off the card (default `CARD_TONE`; `LIFTED_TONE` for map
 * places).
 */
export function colourScale({
  scale,
  values,
  low = [],
  high,
  theme,
  tone = CARD_TONE,
}: {
  scale: HeatScale;
  values: readonly unknown[];
  low?: readonly Rgb[];
  high: readonly Rgb[];
  theme: Theme;
  tone?: ScaleTone;
}): ColourScale {
  const domain = scaleDomain(values, scale);
  const [lo, hi] = domain;
  const offset = (value: number) => (hi === lo ? 0.5 : clamp01((value - lo) / (hi - lo)));
  if (scale.kind === "sequential") {
    const stops = sequentialStops(high, theme, tone);
    const intensity = offset;
    return {
      kind: "sequential",
      domain,
      midpoint: null,
      intensity,
      colour: (value) => ramp(stops, intensity(value)),
      // One colour is the tint itself; a longer ramp's top can be a pale highlight, so a little
      // below it.
      tint: () => (stops.length > 2 ? ramp(stops, 0.8) : stops[stops.length - 1]),
      offset,
      gradient: (samples = 12) =>
        Array.from({ length: samples }, (_, i) => {
          const t = i / Math.max(1, samples - 1);
          return { offset: t, colour: ramp(stops, t) };
        }),
    };
  }
  const mid = scale.midpoint ?? 0;
  const below = divergingStops(low, theme, tone),
    above = divergingStops(high, theme, tone);
  const reach = divergingReach(domain, mid, scale.symmetric);
  const intensity = (value: number) => {
    if (!Number.isFinite(value)) return 0;
    if (value >= mid) return reach.above > 0 ? clamp01((value - mid) / reach.above) : 0;
    return reach.below > 0 ? -clamp01((mid - value) / reach.below) : 0;
  };
  const colour = (value: number) => {
    const s = intensity(value);
    return s < 0 ? ramp(below, -s) : ramp(above, s);
  };
  return {
    kind: "diverging",
    domain,
    midpoint: mid,
    intensity,
    colour,
    tint: (value) => (value < mid ? below[below.length - 1] : above[above.length - 1]),
    offset,
    gradient: (samples = 16) => {
      const stops = Array.from({ length: samples }, (_, i) => {
        const t = i / Math.max(1, samples - 1);
        return { offset: t, colour: colour(lo + t * (hi - lo)) };
      });
      const at = offset(mid);
      if (!stops.some((stop) => Math.abs(stop.offset - at) < 1e-9))
        stops.push({ offset: at, colour: colour(mid) });
      return stops.sort((a, b) => a.offset - b.offset);
    },
  };
}

/** A CSS linear-gradient for a legend bar from a scale's stops. */
export function gradientCss(stops: readonly { offset: number; colour: Rgb }[], angle = "90deg") {
  return `linear-gradient(${angle}, ${stops
    .map((stop) => `${cssRgb(stop.colour)} ${Math.round(stop.offset * 10000) / 100}%`)
    .join(", ")})`;
}
