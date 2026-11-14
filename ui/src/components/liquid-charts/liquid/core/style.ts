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
 * Which style each chart uses, decided from what its data is rather than from taste.
 *
 * Liquid says "this much stuff, filling or flowing"; glass says "things you can count"; tilt is a
 * side view on circular glass that adds height; flat is the quiet, precise baseline. The rules
 * below run in order and the first that matches decides. Charts call `resolveStyle` for their
 * defaults; `material` and `view` override it, with a warning when an override breaks a rule that
 * protects reading. See docs/LIBRARY_COVERAGE.md, "Which style where".
 */

export type Material = "glass" | "liquid" | "flat";
/** What a chart draws. One chart may draw several kinds (bars with a target line, say). */
export type MarkKind =
  | "solid" // bars, sectors, rings, tiles, candles, funnel stages, waffle units
  | "area" // filled areas, bands, streams, violins and densities
  | "stroke" // lines, links, whiskers, wicks, connectors
  | "point" // scatter beads, bubbles, line markers
  | "cell" // heatmap and calendar cells
  | "flow" // sankey and chord ribbons, flow funnels
  | "node" // sankey, network and tree nodes
  | "reference" // targets, ranges, confidence bands, reference lines
  | "text";
/** What the numbers are. Decides whether filling to a baseline tells the truth. */
export type Measure = "amount" | "rate" | "level" | "share" | "flow" | "distribution";
export type Geometry = "cartesian" | "circular" | "polygon" | "layout" | "geo";
export type ColourEncoding = "categorical" | "sequential" | "diverging";
export type Density = "tile" | "standard" | "hero";
export type TiltMode = "never" | "available" | "default";
export type View = "flat" | "tilt";
export type LabelMode = "auto" | "all" | "ends" | "none";
/** What the height of a tilted segment would show. */
export type HeightMeaning = "value" | "second-measure" | "depth" | "none";

export type StyleSpec = {
  geometry: Geometry;
  marks: readonly MarkKind[];
  axis?: "time" | "continuous" | "discrete";
  measure?: Measure;
  /** False when the value axis does not start at zero. */
  zeroBaseline?: boolean;
  series?: number;
  markCount?: number;
  /** The smallest solid mark's shorter side, CSS px. */
  smallestMark?: number;
  overlap?: boolean;
  stacked?: boolean;
  /** A streamgraph: many layers by design, so the stacked-layer limit does not apply. */
  stream?: boolean;
  colour?: ColourEncoding;
  density?: Density;
  /** Gauges and meters are precise readings and never tilt. */
  precise?: boolean;
  segments?: number;
  heightMeaning?: HeightMeaning;
  /** Overrides: one material for every mark, or one per kind. */
  material?: Material | Partial<Record<MarkKind, Material>>;
  view?: View;
};

export type StyleRule =
  | "colour-is-value"
  | "too-small-or-dense"
  | "thin-marks"
  | "liquid-needs-an-amount"
  | "too-many-layers"
  | "amount"
  | "level"
  | "flow"
  | "distribution"
  | "discrete"
  | "quiet-references"
  | "tilt-circular-only"
  | "tilt-needs-meaning"
  | "tilt-segments"
  | "bars-on-a-long-time-axis"
  | "too-many-segments";

export type StyleWarning = { rule: StyleRule; message: string };

export type ResolvedStyle = {
  materials: Partial<Record<MarkKind, Material>>;
  /** The rule that decided each kind's material. */
  reasons: Partial<Record<MarkKind, StyleRule>>;
  tilt: TiltMode;
  view: View;
  /** Set when the style forces a label mode (every segment is labelled while tilted). */
  labels?: LabelMode;
  warnings: StyleWarning[];
};

/** Rules an override may break only with a warning, because they protect the reading. */
const PROTECTIVE: ReadonlySet<StyleRule> = new Set([
  "colour-is-value",
  "too-small-or-dense",
  "liquid-needs-an-amount",
]);
const FILLED: ReadonlySet<MarkKind> = new Set(["solid", "area", "point", "cell", "node", "flow"]);
const MIN_MARK = 5;
const MAX_MARKS = 160;
const MAX_LAYERS = 3;
const MAX_TILT_SEGMENTS = 12;

function decide(kind: MarkKind, spec: StyleSpec): [Material, StyleRule] {
  const quantitativeColour = spec.colour === "sequential" || spec.colour === "diverging";
  // Cells and places whose colour is the value are liquid pools in their exact colour, lit only at
  // the surface and walls (the core's liquid cells), so the colour still reads true. Cells too
  // small to hold a meniscus, and every other mark coloured by value, stay flat.
  if (quantitativeColour && kind === "cell")
    return spec.smallestMark !== undefined && spec.smallestMark < MIN_MARK
      ? ["flat", "colour-is-value"]
      : ["liquid", "colour-is-value"];
  if (quantitativeColour && FILLED.has(kind)) return ["flat", "colour-is-value"];
  const solidLike = kind === "solid" || kind === "point" || kind === "cell" || kind === "node";
  if (
    solidLike &&
    ((spec.smallestMark !== undefined && spec.smallestMark < MIN_MARK) ||
      (spec.markCount !== undefined && spec.markCount > MAX_MARKS) ||
      spec.overlap)
  )
    return ["flat", "too-small-or-dense"];
  if (kind === "stroke" || kind === "text") return ["flat", "thin-marks"];
  if (kind === "reference") return ["flat", "quiet-references"];
  if (kind === "area") {
    if (spec.measure === "distribution") return ["liquid", "distribution"];
    if (spec.measure === "flow") return ["liquid", "flow"];
    if (spec.measure === "rate" || spec.zeroBaseline === false)
      return ["flat", "liquid-needs-an-amount"];
    if ((spec.series ?? 1) > MAX_LAYERS && !spec.stream) return ["flat", "too-many-layers"];
    if (spec.measure === "level") return ["liquid", "level"];
    return ["liquid", "amount"];
  }
  if (kind === "flow") return ["liquid", "flow"];
  if (kind === "solid" && spec.measure === "level") return ["liquid", "level"];
  return ["glass", "discrete"];
}

function tiltFor(spec: StyleSpec, solid: Material | undefined): [TiltMode, StyleRule | null] {
  if (spec.geometry !== "circular" || spec.precise || solid !== "glass")
    return ["never", "tilt-circular-only"];
  if ((spec.segments ?? 0) > MAX_TILT_SEGMENTS) return ["never", "tilt-segments"];
  const hero = spec.density === "hero";
  const meaningful = spec.heightMeaning !== undefined && spec.heightMeaning !== "none";
  if (!meaningful) return hero ? ["default", null] : ["never", "tilt-needs-meaning"];
  return [hero ? "default" : "available", null];
}

const MESSAGES: Record<StyleRule, string> = {
  "colour-is-value":
    "Colour encodes the value here: cells are liquid in their exact colour, lit only at the surface and walls; glass shading would change how light or dark a colour reads.",
  "too-small-or-dense": `Marks under ${MIN_MARK}px, over ${MAX_MARKS} marks or overlapping read better flat.`,
  "thin-marks": "Lines, links and connectors are drawn flat.",
  "liquid-needs-an-amount":
    "Filling to a baseline implies an amount; a rate or a non-zero baseline reads truthfully as a line.",
  "too-many-layers": `More than ${MAX_LAYERS} stacked layers: consider lines, or a 100% stacked bar.`,
  amount: "An amount over a continuous axis, from zero, fills like liquid.",
  level: "A level against a capacity fills like liquid.",
  flow: "Flows are liquid; the nodes they join are glass.",
  distribution: "The shape of a distribution is poured.",
  discrete: "Countable marks are glass.",
  "quiet-references": "Targets, ranges and references stay quieter than the data.",
  "tilt-circular-only": "Tilt is only for circular glass charts, and never for gauges.",
  "tilt-needs-meaning":
    "Give the height a meaning (value, a second measure or hierarchy depth), or tilt only in hero density.",
  "tilt-segments": `Tilt reads only up to ${MAX_TILT_SEGMENTS} segments.`,
  "bars-on-a-long-time-axis":
    "More than 60 bars on a time axis: the shape matters more than any one bar; consider a liquid area.",
  "too-many-segments": "More than 8 segments: a ranked bar compares them more clearly.",
};

const warn = (rule: StyleRule, prefix = ""): StyleWarning => ({
  rule,
  message: prefix + MESSAGES[rule],
});

export function resolveStyle(spec: StyleSpec): ResolvedStyle {
  const materials: ResolvedStyle["materials"] = {};
  const reasons: ResolvedStyle["reasons"] = {};
  const warnings: StyleWarning[] = [];
  for (const kind of spec.marks) {
    const [material, rule] = decide(kind, spec);
    materials[kind] = material;
    reasons[kind] = rule;
    if (rule === "too-many-layers" || rule === "liquid-needs-an-amount") warnings.push(warn(rule));
  }

  // Overrides win, but say so when they break a rule that protects the reading.
  const override = spec.material;
  if (override) {
    for (const kind of spec.marks) {
      const forced = typeof override === "string" ? override : override[kind];
      if (!forced || forced === materials[kind]) continue;
      const rule = reasons[kind];
      // Flat never misreads a colour, so forcing it on a colour-coded cell is quiet.
      const harmless = rule === "colour-is-value" && forced === "flat";
      if (rule && PROTECTIVE.has(rule) && !harmless)
        warnings.push(warn(rule, `Forced ${forced} on ${kind}. `));
      materials[kind] = forced;
    }
  }

  const solid = materials.solid ?? materials.node;
  const [tilt, tiltRule] = tiltFor(spec, solid);
  let view: View = tilt === "default" ? "tilt" : "flat";
  if (spec.view) {
    if (spec.view === "tilt" && tilt === "never" && tiltRule)
      warnings.push(warn(tiltRule, "Forced tilt. "));
    view = spec.view;
  }

  if (spec.geometry === "cartesian" && spec.axis === "time" && (spec.markCount ?? 0) > 60)
    if (materials.solid === "glass") warnings.push(warn("bars-on-a-long-time-axis"));
  if (spec.geometry === "circular" && (spec.segments ?? 0) > 8)
    warnings.push(warn("too-many-segments"));
  // The material scorecard's tally (see ChartFrame): the materials charts resolve.
  const resolved = (globalThis as { __liquidScorecard?: { resolved: Set<string> } })
    .__liquidScorecard?.resolved;
  if (resolved)
    for (const material of Object.values(materials)) if (material) resolved.add(material);

  return {
    materials,
    reasons,
    tilt,
    view,
    labels: view === "tilt" ? "all" : undefined,
    warnings,
  };
}

/** What a value's format says about it, when the chart is not told. */
export function inferMeasure(
  format: "currency" | "currencyCompact" | "number" | "compact" | "percent",
  partsMakeAWhole = false,
): Measure {
  if (format === "percent") return partsMakeAWhole ? "share" : "rate";
  return "amount";
}
