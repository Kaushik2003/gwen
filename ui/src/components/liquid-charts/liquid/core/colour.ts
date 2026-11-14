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
import type { ComponentType, ReactNode } from "react";

export type Theme = "dark" | "light";
/** A colour as red, green and blue, 0–1. */
export type Rgb = readonly [number, number, number];

/** The chart config: per series a label, an optional icon, and colours for one or both themes
 * (several make a gradient). */
export type ChartConfig = Record<
  string,
  {
    label?: ReactNode;
    icon?: ComponentType;
    colors?: { light?: readonly string[]; dark?: readonly string[] };
  }
>;

/** The series' colours for a theme, falling back to the other theme's when one is missing. */
export function seriesColours(config: ChartConfig, key: string, theme: Theme): readonly string[] {
  const colours = config[key]?.colors;
  const other: Theme = theme === "dark" ? "light" : "dark";
  return colours?.[theme] ?? colours?.[other] ?? [];
}

/** The series' main colour (its first) for a theme. */
export const seriesColour = (
  config: ChartConfig,
  key: string,
  theme: Theme,
  fallback = "#909090",
) => seriesColours(config, key, theme)[0] ?? fallback;

const cache = new Map<string, Rgb>();
let probe: CanvasRenderingContext2D | null | undefined;

/** Any CSS colour as an Rgb: hex, rgb(), hsl(), oklch(), named colours and `var(--x)` (resolved
 * against `scope`, so a theme's variables apply). Painting one pixel lets the browser do the
 * conversion, so every colour syntax it understands works. Hex and opaque rgb() with whole
 * channels are read directly, so a server render gets the browser's colour and hydrates without a
 * mismatch. Null for an unknown colour, and outside a browser for any other syntax. */
export function cssToRgb(colour: string, scope?: Element | null): Rgb | null {
  const resolved = resolveVariables(colour.trim(), scope);
  const hit = cache.get(resolved);
  if (hit) return hit;
  const parsed = parseHex(resolved) ?? parseRgb(resolved) ?? paint(resolved);
  if (parsed) cache.set(resolved, parsed);
  return parsed;
}

function resolveVariables(colour: string, scope?: Element | null, depth = 0): string {
  if (!colour.includes("var(") || depth > 8 || typeof window === "undefined") return colour;
  const style = getComputedStyle(scope ?? document.documentElement);
  const next = colour.replace(
    /var\(\s*(--[\w-]+)\s*(?:,\s*([^()]*(?:\([^()]*\))?[^()]*))?\)/g,
    (_, name: string, fallback?: string) =>
      style.getPropertyValue(name).trim() || fallback?.trim() || "",
  );
  return resolveVariables(next, scope, depth + 1);
}

function parseHex(colour: string): Rgb | null {
  const match = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(colour);
  if (!match) return null;
  const hex = match[1].length === 3 ? [...match[1]].map((c) => c + c).join("") : match[1];
  return [0, 2, 4].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255) as unknown as Rgb;
}

/* rgb() and rgba() with whole channels and no transparency, comma or space separated: exactly
 * what painting them gives. Percentages, fractions and translucency are left to the browser. */
const RGB_COMMAS =
  /^rgba?\(\s*(\d{1,3})\s*,\s*(\d{1,3})\s*,\s*(\d{1,3})\s*(?:,\s*(?:1(?:\.0+)?|100%)\s*)?\)$/i;
const RGB_SPACES =
  /^rgba?\(\s*(\d{1,3})\s+(\d{1,3})\s+(\d{1,3})\s*(?:\/\s*(?:1(?:\.0+)?|100%)\s*)?\)$/i;

function parseRgb(colour: string): Rgb | null {
  const match = RGB_COMMAS.exec(colour) ?? RGB_SPACES.exec(colour);
  if (!match) return null;
  return [1, 2, 3].map((i) => Math.min(255, Number(match[i])) / 255) as unknown as Rgb;
}

function paint(colour: string): Rgb | null {
  if (typeof document === "undefined") return null;
  if (probe === undefined) {
    const canvas = document.createElement("canvas");
    canvas.width = canvas.height = 1;
    probe = canvas.getContext("2d", { willReadFrequently: true });
  }
  if (!probe) return null;
  // An unknown colour leaves fillStyle unchanged; a sentinel tells the two apart.
  probe.fillStyle = "#010203";
  probe.fillStyle = colour;
  if (probe.fillStyle === "#010203" && colour.toLowerCase() !== "#010203") return null;
  probe.clearRect(0, 0, 1, 1);
  probe.fillRect(0, 0, 1, 1);
  const [r, g, b] = probe.getImageData(0, 0, 1, 1).data;
  return [r / 255, g / 255, b / 255];
}

/** Smoked graphite for marks out of focus or in comparison, per theme: the flat tone every family
 * eases a tint towards (bars, dots, lines, heads, overview strips). Neutral greys, no tint. */
export const GRAPHITE: Record<Theme, Rgb> = {
  dark: [0.34, 0.34, 0.34],
  light: [0.75, 0.75, 0.75],
};

/** Mixes two Rgb colours. */
export const mixRgb = (a: Rgb, b: Rgb, t: number): Rgb => [
  a[0] + (b[0] - a[0]) * t,
  a[1] + (b[1] - a[1]) * t,
  a[2] + (b[2] - a[2]) * t,
];

/** Light glass lifts a colour's body towards white, and a grey has no hue left to hold it, so a
 * grey series (a comparison, last period) all but vanishes on a white card. Greys draw a step
 * darker in light. */
export function greyBody(tint: Rgb, theme: Theme): Rgb {
  if (theme !== "light" || Math.max(...tint) - Math.min(...tint) >= 0.05) return tint;
  return [tint[0] * 0.78, tint[1] * 0.78, tint[2] * 0.78];
}
