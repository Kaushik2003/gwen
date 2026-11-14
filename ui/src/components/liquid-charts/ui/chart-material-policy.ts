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
/** Appearance only. Providers remain authoritative for scales, geometry and interaction. */
export type ChartMaterialKind = "glass" | "area" | "restrained";
export type ChartMaterialTheme = "dark" | "light";
export type ChartMaterialOrientation = "vertical" | "horizontal";
export type ChartMaterialStop = { offset: number; color: string };
export type ChartMaterialPolicy = {
  family?: string;
  mark?: string;
  quantitativeColor?: boolean;
  dense?: boolean;
  minDimension?: number;
  overlap?: boolean;
  stacked?: boolean;
};

/** Hybrid charts call this separately for each mark, never for a whole composed chart. */
export function resolveChartMaterial(input: ChartMaterialPolicy): ChartMaterialKind {
  const mark = (input.mark ?? input.family ?? "").toLowerCase();
  if (
    input.quantitativeColor ||
    input.dense ||
    input.overlap ||
    (input.minDimension !== undefined && input.minDimension < 8) ||
    /heatmap|choropleth|threshold-line|whisker|axis|grid|text|legend|line|link|point|scatter|boxplot/.test(
      mark,
    )
  )
    return "restrained";
  if (/area|stream|band|ribbon|radar|violin/.test(mark)) return "area";
  if (
    /bar|rect|cell|pie|donut|radial|sector|sunburst|treemap|icicle|pack|bubble|circle|node|histogram|capacity/.test(
      mark,
    )
  )
    return "glass";
  return "restrained";
}

type Rgba = readonly [number, number, number, number];
const clamp = (n: number, max = 1) => Math.max(0, Math.min(max, n));

/** Canvas callers should resolve CSS variables first. Unknown CSS colors remain untouched. */
export function parseMaterialColor(color: string): Rgba | null {
  const hex = color.trim().match(/^#([\da-f]{3,8})$/i)?.[1];
  if (hex && [3, 4, 6, 8].includes(hex.length)) {
    const full = hex.length < 5 ? [...hex].map((c) => c + c).join("") : hex;
    return [
      parseInt(full.slice(0, 2), 16),
      parseInt(full.slice(2, 4), 16),
      parseInt(full.slice(4, 6), 16),
      full.length === 8 ? parseInt(full.slice(6, 8), 16) / 255 : 1,
    ];
  }
  const rgb = color.trim().match(/^rgba?\((.+)\)$/i)?.[1];
  if (!rgb) return null;
  const channels = rgb
    .replace(/\s*\/\s*/, ",")
    .split(/[\s,]+/)
    .filter(Boolean);
  if (channels.length < 3 || channels.length > 4) return null;
  const values = channels.map((value, index) => {
    const n = parseFloat(value);
    return value.endsWith("%") ? n * (index < 3 ? 2.55 : 0.01) : n;
  });
  if (!values.every(Number.isFinite)) return null;
  return [
    clamp(values[0], 255),
    clamp(values[1], 255),
    clamp(values[2], 255),
    clamp(values[3] ?? 1),
  ];
}

function rgba([r, g, b, a]: Rgba) {
  return `rgba(${Math.round(r)}, ${Math.round(g)}, ${Math.round(b)}, ${Number(a.toFixed(4))})`;
}

type StopRecipe = readonly [offset: number, white: number, black: number, alpha: number];
/** Separate authored responses: dark saturated core; light vivid core and pale edge. */
export const MATERIAL_STOP_RECIPES: Record<
  ChartMaterialKind,
  Record<ChartMaterialTheme, readonly StopRecipe[]>
> = {
  glass: {
    dark: [
      [0, 0.66, 0, 1],
      [0.09, 0.23, 0, 1],
      [0.28, 0, 0.4, 1],
      [0.58, 0, 0.65, 1],
      [0.82, 0, 0.32, 1],
      [0.96, 0.28, 0, 1],
      [1, 0.56, 0, 1],
    ],
    light: [
      [0, 0.76, 0, 1],
      [0.12, 0.4, 0, 1],
      [0.35, 0, 0, 1],
      [0.65, 0, 0.08, 1],
      [0.9, 0.24, 0, 1],
      [1, 0.65, 0, 1],
    ],
  },
  area: {
    dark: [
      [0, 0.28, 0, 0.92],
      [0.08, 0, 0, 0.84],
      [0.4, 0, 0.32, 0.68],
      [0.76, 0, 0.62, 0.38],
      [1, 0, 0.8, 0.12],
    ],
    light: [
      [0, 0, 0.12, 0.75],
      [0.12, 0, 0, 0.54],
      [0.5, 0, 0, 0.27],
      [0.82, 0.12, 0, 0.12],
      [1, 0.2, 0, 0.04],
    ],
  },
  restrained: {
    dark: [
      [0, 0, 0, 1],
      [1, 0, 0, 1],
    ],
    light: [
      [0, 0, 0, 1],
      [1, 0, 0, 1],
    ],
  },
};

function recipeColor(color: string, recipe: StopRecipe, css: boolean): string {
  const [, white, black, alpha] = recipe;
  const parsed = parseMaterialColor(color);
  if (parsed) {
    const channels = parsed.slice(0, 3).map((c) => (c * (1 - white) + 255 * white) * (1 - black));
    return rgba([channels[0], channels[1], channels[2], parsed[3] * alpha]);
  }
  if (!css) return color;
  let result = color;
  if (white) result = `color-mix(in srgb, ${result} ${(1 - white) * 100}%, white)`;
  if (black) result = `color-mix(in srgb, ${result} ${(1 - black) * 100}%, black)`;
  if (alpha !== 1) result = `color-mix(in srgb, ${result} ${alpha * 100}%, transparent)`;
  return result;
}

/** Concrete rgba stops for native Canvas/ECharts. Unresolved colors safely remain semantic. */
export function chartMaterialStops(
  color: string,
  kind: ChartMaterialKind,
  theme: ChartMaterialTheme,
): ChartMaterialStop[] {
  return MATERIAL_STOP_RECIPES[kind][theme].map((recipe) => ({
    offset: recipe[0],
    color: recipeColor(color, recipe, false),
  }));
}

/** SVG supports CSS tokens and color-mix; this variant deliberately retains token reactivity. */
export function chartMaterialCssStops(
  color: string,
  kind: ChartMaterialKind,
  theme: ChartMaterialTheme,
): ChartMaterialStop[] {
  return MATERIAL_STOP_RECIPES[kind][theme].map((recipe) => ({
    offset: recipe[0],
    color: recipeColor(color, recipe, true),
  }));
}

export function chartMaterialRimColor(color: string, theme: ChartMaterialTheme): string {
  return recipeColor(color, theme === "dark" ? [0, 0.62, 0, 0.78] : [0, 0, 0.24, 0.7], false);
}

/** Color at a fraction of LOCAL band thickness; caller supplies both true boundaries. */
export function areaColourAtDepth(color: string, depth: number, theme: ChartMaterialTheme): string {
  const bounded = clamp(Number.isFinite(depth) ? depth : 0);
  const recipes = MATERIAL_STOP_RECIPES.area[theme];
  const upper = recipes.findIndex((recipe) => recipe[0] >= bounded);
  if (upper <= 0) return recipeColor(color, recipes[Math.max(0, upper)], true);
  const a = recipes[upper - 1],
    b = recipes[upper];
  const ratio = (bounded - a[0]) / (b[0] - a[0]);
  const recipe = a.map(
    (value, index) => value + (b[index] - value) * ratio,
  ) as unknown as StopRecipe;
  return recipeColor(color, recipe, true);
}
