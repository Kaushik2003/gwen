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
  arrived,
  axisTicks,
  BAR_SLANTS,
  CategoryAxis,
  type CategoryGroup,
  ChartFrame,
  ChartState,
  ChartTooltip,
  cssToRgb,
  Delta,
  Figure,
  greyBody,
  Gridlines,
  MarkLayer,
  moveTooltip,
  mutedText,
  niceDomain,
  niceStep,
  noop,
  noteFocusAnchor,
  placeTooltip,
  ReferenceLine,
  resolveStyle,
  seriesColour,
  slantedSolid,
  TooltipFoot,
  useActive,
  useChartFrame,
  useValueLabels,
  ValueLabel,
  type BarFrame,
  type BarMark,
  type BarView,
  type ChartConfig,
  type Insets,
  type LabelMode,
  type Rect,
  type Rgb,
  type StyleSpec,
  type Theme,
  type TooltipRow,
  type ValueAxis,
} from "@/components/liquid-charts/liquid/core";
import {
  barGeometry,
  barInsets,
  barModel,
  barRect,
  categoryLabelVisibility,
  cell,
  DENSITY,
  figureWidth,
  fitsInside,
  formatValue,
  labelPlan,
  labelTargets,
  projectionStart,
  resolveHighlight,
  rowReach,
  rowTooltipPosition,
  segmentPattern,
  seriesValues,
  stagger,
  textWidth,
  tickFormatter,
  tipAnchor,
  TYPE,
  unionRect,
  valueTicks,
  type BarDensity,
  type BarFormat,
  type BarGeometry,
  type BarHighlight,
  type BarLayout,
  type BarModel,
  type BarRow,
  type BarSegment,
  type BarSort,
  type BarStack,
} from "./layout";
import {
  DragHandle,
  DragReadout,
  TOUCH_TARGET,
  useDrag,
  type DragState,
} from "@/components/liquid-charts/liquid/drag";
import { forwardRef, useRef, useState, type ForwardedRef, type ReactNode, type Ref } from "react";
import { usePageTheme } from "@/components/liquid-charts/liquid/core/page-theme";
import { cssModule } from "@/components/liquid-charts/liquid/core/css-module";
import Roles from "@/components/liquid-charts/liquid/core/type.module.css";
import Styles from "./bar-chart.module.css";

const roles = cssModule(Roles, "lc-type-");
const styles = cssModule(Styles, "lc-bar-chart-");

export type BarTooltip = "single" | "compare" | "stack";

/** What a custom value label is given. */
export type BarLabelInfo<Row extends BarRow> = {
  row: Row;
  /** The row's index in `data`. */
  index: number;
  /** The labelled value: the bar's value, a stack's total, or the lead series' share. */
  value: number;
  /** The value's share of all rows' totals. */
  share: number;
  active: boolean;
  projected: boolean;
  highlighted: boolean;
};

/** Drag to edit: what a drag (or the keyboard) sets, and where it may go. */
export type BarEditable<Row extends BarRow> = {
  /** What moves: a series' values (the bars' lengths, the default) or the targets (each bar's
   * container, so the glass level follows: value ÷ target). */
  edit?: "value" | "target";
  /** The series a value drag sets. Default the first. */
  series?: keyof Row & string;
  /** Rows that may be edited. Default every row. */
  rows?: (row: Row, index: number) => boolean;
  /** The grid values snap to. Default a round hundredth of the value axis. */
  step?: number;
  /** Default 0 (or the axis' low end below zero). */
  min?: number;
  /** Default the top of the axis the chart first drew, which then stays put while editing. */
  max?: number;
  /** Every change, live while dragging. Pass it to control the values through `data` (or
   * `target`); without it the chart keeps the edits itself. */
  onChange?: (index: number, value: number) => void;
  /** Once, when a drag ends or a key changes a value. */
  onCommit?: (index: number, value: number, from: number) => void;
};

/** A quiet line across the columns at a value, with a label at its left end, or (`axis`) in the
 * tick column, set like a tick: a label there never crosses a bar, however many there are. */
/** The line a tier of category groups takes under the names. */
const GROUP_ROOM = 20;

/** A reference line and its label: over the plot at its left (`plot`), in the tick column (`axis`),
 * or just past the plot's right end (`end`, which needs its room on the right: `insets.right`). */
export type BarReference = {
  value: number;
  label?: ReactNode;
  placement?: "plot" | "axis" | "end";
};

export type BarChartProps<Row extends BarRow> = {
  data: readonly Row[];
  /** Series colours and names. Colours come only from here. */
  config: ChartConfig;
  /** The field that names each row (the category axis). */
  categoryKey: keyof Row & string;
  /** Fields to draw, in stack or group order. Default: every config key except `compare` and
   * `lossKey`. */
  series?: readonly (keyof Row & string)[];
  /** A longer name for the tooltip and screen readers ("Monday" for "Mon"). */
  categoryTitle?: (row: Row, index: number) => string;
  /** A field drawn as a graphite ghost beside the live bars: last period, say. */
  compare?: keyof Row & string;
  /** A config key whose colour negative values take (losses). */
  lossKey?: string;
  /** Columns (`vertical`, the default) or rows (`horizontal`). */
  layout?: BarLayout;
  stack?: BarStack;
  /** Ranks rows by their total. `active` and callbacks still use data indices. */
  sort?: BarSort;
  /** The row that carries the label at rest: the largest, the smallest, or a data index. */
  highlight?: BarHighlight;
  /** A short note beside the highlighted figure ("Peak"). */
  highlightLabel?: ReactNode;
  /** With a `highlight`, how lit the other bars are: 1 (the default) as lit as the highlight, 0
   * smoked graphite. About 0.5 keeps their colour quieter, so the highlight leads. `"empty"` leaves
   * them as glass with no liquid, their edges etched at a slant; a bar in focus fills. */
  quiet?: number | "empty";
  /** One target for every row (a line), or one per row (a tick on each). */
  target?: number | readonly (number | null | undefined)[];
  targetLabel?: string;
  /** Rows that are projected or incomplete, etched with the glass pattern: `true` for the whole
   * bar, or the share of the bar (0–1, from the base) that is actual, the rest projected (month to
   * date plus forecast). The tooltip says "Projected". */
  projected?: (row: Row, index: number) => boolean | number;
  /** Light or dark. Without one, the page's: `data-theme` or a `dark` class on `<html>`, else the system setting. @default the page's */
  theme?: Theme;
  height?: number;
  density?: BarDensity;
  active?: number | null;
  onActiveChange?: (index: number | null) => void;
  labels?: LabelMode;
  /** Custom value label content (use `roles.lift` so it lifts on focus). */
  renderLabel?: (info: BarLabelInfo<Row>) => ReactNode;
  /** `true` picks a variant: stack for stacks, compare with a comparison series or target. */
  tooltip?: boolean | BarTooltip;
  /** Each row's change on a period the chart doesn't draw, as a fraction (0.041 is +4.1%), named
   * in the tooltip's foot ("+4.1% vs September"). Missing or non-finite values show nothing. */
  change?: (row: Row, index: number) => number | null | undefined;
  /** What `change` is against, for the tooltip ("September"). */
  changeLabel?: string;
  format?: BarFormat;
  /** Value tick labels; derived from `format` by default. */
  tickFormat?: (value: number) => string;
  /** The value axis: every tick (`full`, the default), quiet hairlines with the unit on the top
   * tick only, `etched` (the quiet ticks' numbers, the graduations engraved in each bar's glass), or
   * `none` when the bars carry their own values. Columns only. */
  valueAxis?: ValueAxis;
  /** How an etched axis cuts its graduations: short `tick`s from the bar's left edge, like a
   * measuring cylinder (the default), or `ring`s across the whole bar. */
  graduation?: "tick" | "ring";
  /** Quiet lines across the columns at a value, labelled at the left: a comparison ("Jan $182K"),
   * an average. Unlike `target` they leave the glass as it is. Columns only. */
  references?: readonly BarReference[];
  /** Runs of columns named once in a tier under their names: the years under a run of months, the
   * quarters under a year's. Indices are rows in the data's order; columns kept in that order only
   * (no `sort`). Absent, the names stand alone, as they always have. */
  categoryGroups?: readonly CategoryGroup[];
  domain?: readonly [number, number];
  /** Overrides for the fitted insets. */
  insets?: Partial<Insets>;
  /** The thickest a bar may be, px. */
  maxThickness?: number;
  /** The space between bars, as a share of each one's slot (0 to 0.9): 0 sets them side by side,
   * touching. Given a gap, bars fill their slots unless `maxThickness` caps them. Seen at a slant,
   * touching columns hide each other's sides, so a side shows only where a bar stands taller than
   * its neighbour. @default 0.44, or 0.4 between groups */
  gap?: number;
  /** Corner rounding, px: the glass bars' bevel and the flat bars' radius. The glass keeps it under
   * a fifth of the thinnest bar. */
  cornerRadius?: number;
  /** Forces glass or flat; otherwise the style rules decide. Glass needs the materials
   * (`<LiquidMaterials>`); without it the bars draw flat. */
  material?: "glass" | "flat";
  /** How the columns are seen: straight on (`flat`, the default), from the side (`side`: each
   * bar's side shows, and the bars fade toward the floor they stand on), or from a corner and above
   * (`isometric`). The view has no perspective and every bar's front stays true to the value axis,
   * so labels, hover and the axis stay put. Glass columns only, up to 48 bars; rows, flat bars and
   * more bars stay straight on.
   * @default "flat" */
  view?: BarView;
  /** Drag to edit: drag a bar's tip (or its container's, for targets) to set it, or use Up and
   * Down (Right and Left for rows), the Page keys and Home and End. Side-by-side bars only. */
  editable?: boolean | BarEditable<Row>;
  loading?: boolean;
  empty?: ReactNode;
  error?: ReactNode;
  /** Change to replay the entrance. */
  replay?: number;
  /** The chart's accessible name. */
  label: string;
  /** One sentence for the focused row (a data index), read aloud. */
  describe?: (index: number) => string;
  className?: string;
};

const FALLBACK: Rgb = [0.57, 0.57, 0.57];

const plain = (node: ReactNode, fallback: string) =>
  typeof node === "string" || typeof node === "number" ? String(node) : fallback;

/**
 * Bars of every Wave 1 shape from one component: columns or rows, ranked, grouped, against a
 * previous period, stacked and 100% stacked, diverging, highlighted, against a target, and with
 * projected periods. Countable bars are glass; thin, crowded or forced bars and browsers without
 * WebGL draw flat with the same motion, labels, tooltip and keyboard.
 */
export function BarChart<Row extends BarRow>({
  data,
  config,
  categoryKey,
  series,
  categoryTitle,
  compare,
  lossKey,
  layout = "vertical",
  stack = "none",
  sort = "none",
  highlight,
  highlightLabel,
  quiet = 1,
  target,
  targetLabel = "Target",
  projected,
  theme: themeProp,
  height,
  density = "standard",
  active: activeProp,
  onActiveChange,
  labels = "auto",
  renderLabel,
  tooltip = true,
  change,
  changeLabel,
  format = { kind: "number" },
  tickFormat,
  valueAxis = "full",
  graduation = "tick",
  references,
  categoryGroups,
  domain,
  insets: insetOverrides,
  maxThickness,
  gap,
  cornerRadius,
  material,
  view = "flat",
  editable,
  loading = false,
  empty,
  error,
  replay = 0,
  label,
  describe,
  className,
}: BarChartProps<Row>) {
  const theme = usePageTheme(themeProp);
  const [active, setActive] = useActive(activeProp, onActiveChange);
  const valueLabels = useValueLabels();
  const tip = useRef<HTMLDivElement>(null);
  const targetNote = useRef<HTMLDivElement>(null);

  const vertical = layout === "vertical";
  const frameHeight = height ?? DENSITY[density].height;
  const keys: readonly string[] =
    series ??
    Object.keys(config).filter((key) => key !== compare && key !== lossKey && key !== categoryKey);
  const baseValues = seriesValues(data, keys);
  const compareValues = compare ? data.map((row) => cell(row[compare])) : null;
  const single = typeof target === "number" ? target : null;
  const baseTargets =
    typeof target === "number"
      ? data.map(() => target)
      : target
        ? data.map((_, i) => cell(target[i]))
        : [];

  /* ——— Drag to edit: the edits in progress (or kept, when uncontrolled) over the data ——— */
  const edit = editable && stack === "none" ? (editable === true ? {} : editable) : null;
  const editsTarget = edit?.edit === "target" && baseTargets.length > 0;
  const editSeries = Math.max(0, edit?.series ? keys.indexOf(edit.series) : 0);
  const [draft, setDraft] = useState<ReadonlyMap<number, number>>(() => new Map());
  const values =
    edit && !editsTarget && draft.size
      ? baseValues.map((row, s) => (s === editSeries ? row.map((v, i) => draft.get(i) ?? v) : row))
      : baseValues;
  const targets =
    editsTarget && draft.size ? baseTargets.map((v, i) => draft.get(i) ?? v) : baseTargets;
  // The axis the chart first drew stays put while editing, so a drag never rescales the bars.
  const [editTop] = useState(() =>
    edit
      ? barModel({
          values: baseValues,
          compare: compareValues,
          stack,
          sort,
          targets: baseTargets,
          domain,
        }).domain[1]
      : 0,
  );
  const free = barModel({ values, compare: compareValues, stack, sort, targets, domain });
  const editMax = edit?.max ?? editTop;
  const editMin = edit?.min ?? Math.min(0, free.domain[0]);
  const model =
    edit && !domain
      ? barModel({
          values,
          compare: compareValues,
          stack,
          sort,
          targets,
          domain: niceDomain([...free.domain, editMax, editMin], { count: 10 }),
        })
      : free;
  const count = model.order.length;
  const projectedFrom = data.map((row, i) => projectionStart(projected?.(row, i)));
  const isProjected = projectedFrom.map((from) => from !== null);
  const reach = rowReach(model);
  const highlighted = resolveHighlight(model.totals, highlight, isProjected);
  const percent = stack === "percent";
  const labelFormat: BarFormat = percent ? { kind: "percent", decimals: 0 } : format;
  const plan = labelTargets(model, stack);
  const variant: BarTooltip | null =
    tooltip === false
      ? null
      : tooltip === true
        ? stack !== "none"
          ? "stack"
          : compare || target !== undefined
            ? "compare"
            : "single"
        : tooltip;

  const names = data.map((row) => String(row[categoryKey] ?? ""));
  const title = (index: number) => categoryTitle?.(data[index], index) ?? names[index];
  const seriesName = (key: string) => config[key]?.label ?? key;
  const colourOf = (key: string) => seriesColour(config, key, theme);
  const tints = keys.map((key) => greyBody(cssToRgb(colourOf(key)) ?? FALLBACK, theme));
  const lossTint = lossKey ? greyBody(cssToRgb(colourOf(lossKey)) ?? FALLBACK, theme) : null;
  /* A comparison draws as a graphite ghost, which on a white card is barely there. A grey one
   * (last period) draws in its own grey on light instead, a step darker, smoking with the rest when
   * another row is in focus. */
  const compareTint = compare ? cssToRgb(colourOf(compare)) : null;
  const greyGhost =
    theme === "light" && compareTint && Math.max(...compareTint) - Math.min(...compareTint) < 0.05
      ? greyBody(compareTint, theme)
      : null;
  const segmentColour = (segment: BarSegment) =>
    segment.value < 0 && lossKey ? colourOf(lossKey) : colourOf(keys[Math.max(0, segment.series)]);
  /** A segment's name: its series, or the loss key's label for a negative value. */
  const segmentName = (segment: BarSegment) =>
    segment.value < 0 && lossKey && config[lossKey]?.label
      ? config[lossKey].label
      : seriesName(keys[Math.max(0, segment.series)]);

  const ticks = valueTicks(model.domain, DENSITY[density].ticks);
  const formatTick = axisTicks(
    ticks,
    tickFormat ?? tickFormatter(percent ? { kind: "percent" } : format, model.domain, ticks),
    vertical ? valueAxis : "full",
  );
  /* Rows with a target each show it, muted, after the value ("$412K of $380K"). */
  const quotaAfter = !vertical && single === null && targets.length > 0;
  const quotaText = (index: number) =>
    quotaAfter && Number.isFinite(targets[index])
      ? ` of ${formatValue(targets[index], { ...format, signed: false })}`
      : "";
  const widestLabel = plan.reduce(
    (widest, t) => {
      const text = formatValue(t.value, labelFormat) + quotaText(t.category);
      return text.length > widest.length ? text : widest;
    },
    formatValue(0, labelFormat),
  );
  // Bars set side by side (a `gap`) fill their slots, unless a `maxThickness` says otherwise.
  const cap = maxThickness ?? (gap === undefined ? DENSITY[density][vertical ? "column" : "row"] : Infinity);
  const paddingInner = gap === undefined ? undefined : Math.min(0.9, Math.max(0, gap));
  const slant = vertical && view !== "flat" ? BAR_SLANTS[view] : null;
  const solidRect = (r: Rect, thickness: number): Rect => {
    if (!slant) return r;
    const { right, above } = slantedSolid(slant, thickness);
    return { ...r, right: r.right + right, top: r.top - above };
  };
  const framed: Insets = {
    ...barInsets({
      layout,
      // A reference set in the tick column needs its room there too.
      ticks: [
        ...ticks.map(formatTick),
        ...(references ?? [])
          .filter((r) => r.placement === "axis" && typeof r.label === "string")
          .map((r) => r.label as string),
      ],
      categories: names,
      labels,
      valueLabel: widestLabel,
      negative: model.negative,
      density,
      inside: percent,
      targetLabel:
        single === null
          ? null
          : [targetLabel, formatValue(single, format)].reduce((a, b) =>
              textWidth(a, TYPE.figure) > textWidth(b, TYPE.figure) ? a : b,
            ),
    }),
    ...insetOverrides,
  };
  // A tier of groups under the names takes a line of its own below them.
  const insets: Insets =
    categoryGroups?.length && layout === "vertical" && insetOverrides?.bottom === undefined
      ? { ...framed, bottom: framed.bottom + GROUP_ROOM }
      : framed;
  /* At a slant a column's side reaches right and a corner's top rises above its value: the bars'
   * plot keeps that room, for the bars as thick as they'd be without it (a little thicker than they
   * end up), inside the frame's plot so the axes follow. */
  const geometryFor = (width: number, h: number) => {
    const plain = barGeometry(model, { width, height: h, insets, layout, maxThickness: cap, paddingInner });
    if (!slant) return plain;
    const { right, above } = slantedSolid(slant, plain.thickness);
    return barGeometry(model, {
      width,
      height: h,
      insets: { ...insets, top: insets.top + above, right: insets.right + right },
      layout,
      maxThickness: cap,
      paddingInner,
    });
  };

  /* ——— Drag to edit ——— */
  const plot = useRef<HTMLDivElement>(null);
  const valueOf = (index: number) =>
    (editsTarget ? targets[index] : values[editSeries]?.[index]) ?? NaN;
  const canEdit = (index: number) =>
    !!edit &&
    index >= 0 &&
    index < data.length &&
    Number.isFinite(valueOf(index)) &&
    (!edit.rows || edit.rows(data[index], index));
  const editStep = edit?.step ?? (niceStep((model.domain[1] - model.domain[0]) / 100) || 1);
  const drag = useDrag({
    value: valueOf,
    onChange: (index, value) => {
      setDraft((was) => new Map(was).set(index, value));
      edit?.onChange?.(index, value);
    },
    onCommit: (index, value, from) => {
      // Controlled: the data takes over from the draft once the gesture ends.
      if (edit?.onChange)
        setDraft((was) => {
          if (!was.has(index)) return was;
          const next = new Map(was);
          next.delete(index);
          return next;
        });
      edit?.onCommit?.(index, value, from);
    },
    toValue: (_, point) => {
      const element = plot.current;
      if (!element) return NaN;
      const g = geometryFor(element.clientWidth, element.clientHeight);
      return g.value.invert(vertical ? point.y : point.x);
    },
    bounds: { step: editStep, min: editMin, max: editMax },
    axis: vertical ? "y" : "x",
    surface: () => plot.current,
    onStart: (index) => setActive(index),
    disabled: !edit,
  });
  /** The row a pointer is dragging, if any. */
  const dragging = drag.drag?.via === "pointer" ? drag.drag.index : null;

  const activeSlot = active === null ? -1 : model.order.indexOf(active);
  const focus = activeSlot < 0 ? null : activeSlot;
  const segmentOf = new Map(model.segments.map((segment) => [segment.key, segment]));
  const pace = stagger(count);

  /* An etched axis: the ticks the segment passes, engraved as shares of its length (a tick's share
   * carries +2, which the glass reads as a short tick, a ring's +4: either is cut quieter than a
   * reference mark). */
  const etched = valueAxis === "etched" && vertical ? ticks.filter((t) => t !== 0) : [];
  const graduations = (s: BarSegment): number[] | undefined => {
    const low = Math.min(s.start, s.end),
      high = Math.max(s.start, s.end);
    if (!etched.length || high <= low) return undefined;
    const shares = etched
      .filter((t) => t > low && t < high)
      .map((t) => (s.end >= s.start ? t - low : high - t) / (high - low))
      .filter((share) => share > 0.04 && share < 0.97)
      .slice(0, 4);
    return shares.length
      ? shares.map((share) => share + (graduation === "tick" ? 2 : 4))
      : undefined;
  };

  const layoutMarks = (width: number, h: number): BarMark[] =>
    geometryFor(width, h).bars.map((bar) => {
      const s = bar.segment;
      const inFocus = focus === null || focus === s.slot ? 1 : 0;
      // Out of focus is smoked; at rest, the bars beside a highlight can be quieter than it, or
      // empty glass (until one comes into focus and fills).
      const beside = highlighted !== null && s.category !== highlighted;
      const empty = quiet === "empty" && beside && focus !== s.slot;
      const tone =
        focus === null && beside
          ? quiet === "empty"
            ? 0
            : Math.min(1, Math.max(0, quiet))
          : inFocus;
      const lit = s.series < 0 ? (greyGhost ? inFocus : 0) : tone;
      // A projected row keeps its tone; the etched pattern carries the meaning.
      const side = s.end >= s.start ? reach[s.category].up : reach[s.category].down;
      const pattern =
        s.series < 0
          ? undefined
          : segmentPattern(s, projectedFrom[s.category], stack === "none" ? s.end : side);
      return {
        key: bar.key,
        cross: bar.cross,
        base: bar.base,
        thickness: bar.thickness,
        length: bar.length,
        direction: bar.direction,
        tint:
          s.series < 0
            ? (greyGhost ?? tints[0])
            : s.value < 0 && lossTint
              ? lossTint
              : tints[s.series],
        lit,
        pattern,
        // A target lives in the glass: a container filled to the value, or an engraving.
        level: s.series < 0 ? undefined : empty ? 0 : s.level,
        marks: s.series < 0 ? undefined : (s.marks ?? graduations(s)),
        delay: s.slot * pace + (stack !== "none" ? s.series * 0.14 : s.group * 0.05),
      };
    });

  const onFrame = (bars: BarFrame[], width: number, h: number) => {
    const byKey = new Map(bars.map((bar) => [bar.key, bar]));
    const priorities = labelPlan(plan, {
      mode: labels,
      active: focus === null ? null : active,
      highlight: highlighted,
      exclude: isProjected,
      hideOnFocus: vertical,
    });
    const note = targetNote.current;
    const obstacles: Rect[] = [];
    if (note?.parentElement) {
      const r = note.getBoundingClientRect(),
        p = note.parentElement.getBoundingClientRect();
      obstacles.push({
        left: r.left - p.left,
        top: r.top - p.top,
        right: r.right - p.left,
        bottom: r.bottom - p.top,
      });
    }
    // Labels at the tips keep off the other bars: a last bar's label slid in from the edge would
    // sit on its neighbour. (Labels inside their bars can't, as their own bar would block them.)
    if (!plan.some((t) => t.centre))
      for (const bar of bars) obstacles.push(solidRect(barRect(bar, bar.current), bar.thickness));
    valueLabels.place(
      plan.map((t, i) => {
        const bar = byKey.get(t.markKey);
        if (!bar) return { id: t.id, x: 0, y: 0, side: "above" as const, priority: 0 };
        const r = barRect(bar, bar.current);
        const lifted = active === t.category;
        // A label arrives with its bar's liquid, not before it (the focused one at once).
        const ready = lifted || arrived(bar);
        if (t.centre) {
          const text = formatValue(t.value, labelFormat);
          const fits = fitsInside(r, figureWidth(text, TYPE.figure) * (lifted ? 1.22 : 1), 16, 3);
          return {
            id: t.id,
            x: (r.left + r.right) / 2,
            y: (r.top + r.bottom) / 2,
            side: "center" as const,
            priority: ready && fits && dragging !== t.category ? priorities[i] : 0,
            lifted,
          };
        }
        // The bar's full length: past a container's top, not at its liquid, and over the top a
        // corner shows from above.
        const anchor = tipAnchor(r, bar.direction, bar.cross);
        const lift = slant && bar.direction === "up" ? slantedSolid(slant, bar.thickness).above : 0;
        return {
          id: t.id,
          ...anchor,
          offset: (vertical ? 6 : 10) + lift,
          flip: false,
          priority: dragging === t.category || !ready ? 0 : priorities[i],
          lifted,
        };
      }),
      {
        // Clear of the tick (or category) column on the left.
        bounds: {
          left: insets.left - 4,
          top: 0,
          right: width,
          bottom: vertical
            ? model.negative
              ? h - (density === "tile" ? 18 : 22)
              : Math.max(...bars.map((bar) => bar.base), 0)
            : h,
        },
        obstacles,
      },
    );

    if (tip.current && active !== null && focus !== null) {
      const own = bars.filter((bar) => segmentOf.get(bar.key)?.category === active);
      const anchor = unionRect(own.map((bar) => barRect(bar, bar.current)));
      if (anchor) {
        if (vertical)
          placeTooltip(
            tip.current,
            anchor,
            {
              left: Math.max(4, insets.left - 8),
              top: 4,
              right: width - 4,
              bottom: h - insets.bottom,
            },
            { foot: model.totals[active] < 0 },
          );
        else
          placeRowTooltip(tip.current, anchor, {
            left: 4,
            top: 4,
            right: width - 4,
            bottom: h - 4,
          });
      }
    }
  };

  /* ——— States ——— */
  if (error || loading || !model.segments.length) {
    const kind = error ? "error" : loading ? "loading" : "empty";
    return (
      <ChartFrame
        theme={theme}
        height={frameHeight}
        insets={insets}
        count={1}
        active={null}
        onActiveChange={noop}
        pick={() => null}
        label={label}
        className={className}
      >
        <ChartState
          kind={kind}
          message={kind === "error" ? (error === true ? undefined : error) : empty}
          bars={Math.max(4, Math.min(12, data.length || 7))}
        />
      </ChartFrame>
    );
  }

  /* ——— Reading aloud and the data table ——— */
  const formatted = (value: number) => formatValue(value, format);
  const projectedNote = (index: number) =>
    projectedFrom[index] === null
      ? ""
      : projectedFrom[index] === 0
        ? " (projected)"
        : " (partly projected)";
  const describeRow = (index: number) => {
    // While a pointer drags this row it reads as it was, so a screen reader hears where the drag
    // ends rather than every step of it.
    const held = drag.drag?.via === "pointer" && drag.drag.index === index ? drag.drag.from : null;
    const valueAt = (s: number) =>
      held !== null && !editsTarget && s === editSeries ? held : values[s][index];
    const targetAt = held !== null && editsTarget ? held : targets[index];
    const parts = keys.map((key, s) => `${plain(seriesName(key), key)} ${formatted(valueAt(s))}`);
    if (stack !== "none" && keys.length > 1)
      parts.unshift(`total ${formatted(model.totals[index])}`);
    if (compare && compareValues)
      parts.push(`${plain(seriesName(compare), compare)} ${formatted(compareValues[index])}`);
    if (targets.length && Number.isFinite(targetAt))
      parts.push(`${targetLabel.toLowerCase()} ${formatted(targetAt)}`);
    const hint = canEdit(index) ? ` ${vertical ? "Up and Down" : "Right and Left"} change it.` : "";
    return `${title(index)}${projectedNote(index)}: ${parts.join(", ")}.${hint}`;
  };
  const columns = [
    plain(config[categoryKey]?.label, categoryKey.charAt(0).toUpperCase() + categoryKey.slice(1)),
    ...keys.map((key) => plain(seriesName(key), key)),
    ...(compare ? [plain(seriesName(compare), compare)] : []),
    ...(targets.length ? [targetLabel] : []),
  ];
  const tableRows = data.map((_, i) => [
    title(i) + projectedNote(i),
    ...keys.map((_, s) => formatted(values[s][i])),
    ...(compareValues ? [formatted(compareValues[i])] : []),
    ...(targets.length ? [formatted(targets[i])] : []),
  ]);

  const grandTotal = model.totals.reduce((sum, v) => sum + (Number.isFinite(v) ? v : 0), 0);
  const spec: StyleSpec = {
    geometry: "cartesian",
    marks: target !== undefined ? ["solid", "reference"] : ["solid"],
    axis: "discrete",
    measure: percent ? "share" : "amount",
    series: keys.length + (compare ? 1 : 0),
    markCount: model.segments.length,
    stacked: stack !== "none",
    density,
  };

  return (
    <ChartFrame
      theme={theme}
      height={frameHeight}
      insets={insets}
      count={count}
      active={focus}
      onActiveChange={(slot) => setActive(slot === null ? null : model.order[slot])}
      pick={(x, y, size) => geometryFor(size.width, size.height).slots.indexAt(vertical ? x : y)}
      label={label}
      describe={(slot) => (describe ?? describeRow)(model.order[slot])}
      table={{ caption: label, columns, rows: tableRows }}
      plotRef={plot}
      keys={
        edit
          ? (key, slot) => {
              if (slot === null) return undefined;
              const index = model.order[slot];
              return canEdit(index) && drag.key(index, key) ? slot : undefined;
            }
          : undefined
      }
      className={className}
    >
      <Axes
        geometryFor={geometryFor}
        layout={layout}
        names={model.order.map((i) => names[i])}
        ticks={ticks}
        formatTick={formatTick}
        negative={model.negative}
        valueAxis={valueAxis}
        references={references}
        strong={highlighted === null ? null : model.order.indexOf(highlighted)}
        groups={layout === "vertical" ? categoryGroups : undefined}
      />
      <Marks
        theme={theme}
        replay={replay}
        spec={spec}
        material={material}
        cornerRadius={cornerRadius}
        view={slant ? view : "flat"}
        geometryFor={geometryFor}
        layout={layoutMarks}
        onFrame={onFrame}
      />
      {single !== null && (
        <TargetNote
          key={replay}
          geometryFor={geometryFor}
          layout={layout}
          value={single}
          label={targetLabel}
          format={format}
          noteRef={targetNote}
        />
      )}
      {plan.map((t) => {
        const row = data[t.category];
        const isActive = active === t.category;
        const isHighlighted = highlighted === t.category;
        const info: BarLabelInfo<Row> = {
          row,
          index: t.category,
          value: t.value,
          share: grandTotal ? t.value / grandTotal : 0,
          active: isActive,
          projected: isProjected[t.category],
          highlighted: isHighlighted,
        };
        const figure = (className: string) => (
          <Figure
            value={t.value}
            kind={labelFormat.kind}
            currency={labelFormat.currency}
            decimals={labelFormat.decimals}
            signed={labelFormat.signed}
            className={className}
          />
        );
        return (
          <ValueLabel key={t.id} ref={valueLabels.ref(t.id)} active={isActive}>
            {renderLabel ? (
              renderLabel(info)
            ) : isHighlighted && !t.centre ? (
              <span className={`${styles.callout} ${roles.lift}`}>
                {figure(roles.figureS)}
                {highlightLabel && (
                  <span className={`${styles.calloutNote} ${roles.micro}`}>{highlightLabel}</span>
                )}
              </span>
            ) : quotaText(t.category) ? (
              <span className={`${styles.reading} ${roles.lift}`}>
                {figure(roles.figureS)}
                <span className={`${roles.micro} ${mutedText}`}>{quotaText(t.category)}</span>
              </span>
            ) : (
              figure(`${roles.figureS} ${roles.lift} ${t.centre ? styles.inside : ""}`)
            )}
          </ValueLabel>
        );
      })}
      {edit && (
        <EditLayer
          geometryFor={geometryFor}
          layout={layout}
          rows={data.map((_, i) => i).filter(canEdit)}
          segmentKey={(index) => `${editSeries}-${index}`}
          valueOf={valueOf}
          handle={drag.handle}
          active={active}
          drag={drag.drag?.via === "pointer" ? drag.drag : null}
          format={format}
        />
      )}
      {variant && active !== null && focus !== null && dragging === null && (
        <BarTooltipCard
          ref={tip}
          variant={variant}
          index={active}
          slot={focus}
          model={model}
          compare={compare}
          compareValues={compareValues}
          targets={targets}
          targetLabel={targetLabel}
          projectedFrom={projectedFrom[active]}
          change={change?.(data[active], active) ?? null}
          changeLabel={changeLabel}
          title={title(active)}
          shortName={(i) => names[i]}
          seriesName={seriesName}
          segmentName={segmentName}
          colourOf={colourOf}
          segmentColour={segmentColour}
          format={format}
          percent={percent}
          stacked={stack !== "none"}
        />
      )}
    </ChartFrame>
  );
}

/** Sets a row's tooltip where `rowTooltipPosition` says; like the core's, it appears in place and
 * then glides between rows. */
function placeRowTooltip(tip: HTMLElement, anchor: Rect, bounds: Rect) {
  noteFocusAnchor(tip, anchor);
  const { x, y } = rowTooltipPosition(
    anchor,
    { width: tip.offsetWidth, height: tip.offsetHeight },
    bounds,
  );
  moveTooltip(tip, x, y);
}

/* ——— Axes ——— */

function Axes({
  geometryFor,
  layout,
  names,
  ticks,
  formatTick,
  negative,
  valueAxis,
  references,
  strong,
  groups,
}: {
  geometryFor: (width: number, height: number) => BarGeometry;
  layout: BarLayout;
  /** Runs of columns named once under their names. */
  groups?: readonly CategoryGroup[];
  /** In display order. */
  names: readonly string[];
  ticks: readonly number[];
  formatTick: (value: number) => string;
  negative: boolean;
  valueAxis: ValueAxis;
  references?: readonly BarReference[];
  /** The display slot of the highlighted row, whose name is set in the first ink. */
  strong: number | null;
}) {
  const { size, active } = useChartFrame();
  if (!size.width) return null;
  const g = geometryFor(size.width, size.height);
  if (layout === "horizontal")
    return (
      <>
        {negative && (
          <span
            className={styles.zero}
            style={{ left: g.zero, top: g.plot.top - 4, height: g.plot.bottom - g.plot.top + 8 }}
            aria-hidden="true"
          />
        )}
        <CategoryAxis
          orientation="left"
          labels={names}
          position={g.slots.centre}
          active={active}
          strong={strong}
          dim
        />
      </>
    );
  const widths = names.map((name) => textWidth(name, TYPE.label));
  const visible = categoryLabelVisibility(names.length, g.slots.step, widths, active);
  return (
    <>
      <Gridlines
        values={ticks}
        position={g.value}
        // A tick beside a reference's label reads as one phrase with it ("200 Jan $182K"): the
        // reference already names that height, so the tick gives way. A label past the plot's end
        // is nowhere near the ticks, and every tick stays.
        format={(value) =>
          references?.some(
            (r) => r.placement !== "end" && Math.abs(g.value(value) - g.value(r.value)) < 20,
          )
            ? ""
            : formatTick(value)
        }
        mode={valueAxis}
      />
      {references?.map((reference, i) => (
        <ReferenceLine
          key={i}
          value={reference.value}
          position={g.value}
          label={reference.label}
          labelSide="left"
          labelPlacement={
            reference.placement === "axis" || reference.placement === "end"
              ? reference.placement
              : "above"
          }
        />
      ))}
      <CategoryAxis
        labels={names.map((name, i) => (visible[i] ? name : ""))}
        position={g.slots.centre}
        active={active}
        strong={strong}
        groups={groups}
        step={g.slots.step}
        contain
      />
    </>
  );
}

/* ——— Marks: glass where the rules allow it and a material pack brings it, flat otherwise ——— */

/** The core mark layer, flat while the bars are too thin for glass at this size (the glass stays
 * mounted underneath, so widening again is seamless) or when flat is asked for. */
function Marks({
  spec,
  material,
  cornerRadius,
  view,
  geometryFor,
  ...layer
}: {
  theme: Theme;
  replay: number;
  layout: (width: number, height: number) => BarMark[];
  onFrame: (bars: BarFrame[], width: number, height: number) => void;
  spec: StyleSpec;
  material?: "glass" | "flat";
  cornerRadius?: number;
  view: BarView;
  geometryFor: (width: number, height: number) => BarGeometry;
}) {
  const { size } = useChartFrame();
  const thickness = size.width ? geometryFor(size.width, size.height).thickness : undefined;
  const style = resolveStyle({ ...spec, smallestMark: thickness, material });
  return (
    <MarkLayer
      {...layer}
      flat={material === "flat"}
      asked={material === "glass"}
      fallback={style.materials.solid === "flat"}
      bevel={cornerRadius}
      flatRadius={cornerRadius}
      view={view}
    />
  );
}

/* ——— Drag to edit: a grab handle on each editable bar, and the value at the pointer ——— */

function EditLayer({
  geometryFor,
  layout,
  rows,
  segmentKey,
  valueOf,
  handle,
  active,
  drag,
  format,
}: {
  geometryFor: (width: number, height: number) => BarGeometry;
  layout: BarLayout;
  /** Editable rows, by data index. */
  rows: readonly number[];
  segmentKey: (index: number) => string;
  valueOf: (index: number) => number;
  handle: ReturnType<typeof useDrag>["handle"];
  active: number | null;
  drag: DragState | null;
  format: BarFormat;
}) {
  const { size } = useChartFrame();
  if (!size.width) return null;
  const g = geometryFor(size.width, size.height);
  const vertical = layout === "vertical";
  const bars = new Map(g.bars.map((bar) => [bar.key, bar]));
  const [lo, hi] = [Math.min(...g.value.domain), Math.max(...g.value.domain)];
  return (
    <>
      {rows.map((index) => {
        const bar = bars.get(segmentKey(index));
        const value = valueOf(index);
        if (!bar || !Number.isFinite(value)) return null;
        const at = g.value(Math.max(lo, Math.min(hi, value)));
        // Wide enough to grab, never wider than its own slot.
        const across = Math.min(Math.max(bar.thickness + 12, TOUCH_TARGET), g.slots.step);
        // The icon sits left of a column's tip (the tooltip takes the right), or just inside a
        // row's (its label follows the tip).
        const inward = bar.direction === "left" ? 1 : -1;
        const room = Math.abs(at - g.zero);
        return (
          <DragHandle
            key={index}
            x={vertical ? bar.cross : at}
            y={vertical ? at : bar.cross}
            axis={vertical ? "y" : "x"}
            width={vertical ? across : TOUCH_TARGET}
            height={vertical ? TOUCH_TARGET : across}
            active={active === index}
            dragging={drag?.index === index}
            icon={
              vertical
                ? { x: -(bar.thickness / 2 + 12), y: 0 }
                : { x: room > 30 ? inward * 13 : -inward * 15, y: 0 }
            }
            {...handle(index)}
          />
        );
      })}
      {drag && (
        <DragReadout
          x={drag.point.x}
          y={drag.point.y}
          bounds={{ left: 0, top: 0, right: size.width, bottom: size.height }}
          note={
            drag.value !== drag.from ? (
              <Figure
                value={drag.value - drag.from}
                kind={format.kind}
                currency={format.currency}
                decimals={format.decimals}
                signed
                className={roles.micro}
              />
            ) : undefined
          }
        >
          <Figure
            value={drag.value}
            kind={format.kind}
            currency={format.currency}
            decimals={format.decimals}
            signed={format.signed}
            className={roles.figureM}
          />
        </DragReadout>
      )}
    </>
  );
}

/* ——— A single target's label: the containers and engravings line up into the target, so only a
 * quiet note at the plot's edge names it, with no line (style rule 9). ——— */

function TargetNote({
  geometryFor,
  layout,
  value,
  label,
  format,
  noteRef,
}: {
  geometryFor: (width: number, height: number) => BarGeometry;
  layout: BarLayout;
  value: number;
  label: string;
  format: BarFormat;
  noteRef: Ref<HTMLDivElement>;
}) {
  const { size } = useChartFrame();
  if (!size.width) return null;
  const g = geometryFor(size.width, size.height);
  const at = g.value(value);
  return (
    <div
      ref={noteRef}
      className={styles.targetLabel}
      data-layout={layout}
      style={
        layout === "vertical"
          ? { left: g.plot.right + 10, top: at }
          : { left: at, top: g.plot.bottom + 8 }
      }
      aria-hidden="true"
    >
      <span className={`${styles.targetName} ${roles.micro}`}>{label}</span>
      <Figure
        value={value}
        kind={format.kind}
        currency={format.currency}
        decimals={format.decimals}
        className={roles.figureS}
      />
    </div>
  );
}

/* ——— Tooltip: single, compare and stack ——— */

const BarTooltipCard = forwardRef(function BarTooltipCard(
  {
    variant,
    index,
    slot,
    model,
    compare,
    compareValues,
    targets,
    targetLabel,
    projectedFrom,
    change,
    changeLabel,
    title,
    shortName,
    seriesName,
    segmentName,
    colourOf,
    segmentColour,
    format,
    percent,
    stacked,
  }: {
    variant: BarTooltip;
    index: number;
    slot: number;
    model: BarModel;
    compare?: string;
    compareValues: readonly number[] | null;
    targets: readonly number[];
    targetLabel: string;
    /** Where this row's projection begins along its bar, or null. */
    projectedFrom: number | null;
    /** This row's change on an undrawn period, and what it is against. */
    change: number | null;
    changeLabel?: string;
    title: string;
    shortName: (index: number) => string;
    seriesName: (key: string) => ReactNode;
    segmentName: (segment: BarSegment) => ReactNode;
    colourOf: (key: string) => string;
    segmentColour: (segment: BarSegment) => string;
    format: BarFormat;
    percent: boolean;
    stacked: boolean;
  },
  ref: ForwardedRef<HTMLDivElement>,
) {
  const figure = (value: number, className: string = roles.figureS, f: BarFormat = format) => (
    <Figure
      value={value}
      kind={f.kind}
      currency={f.currency}
      decimals={f.decimals}
      signed={f.signed}
      className={className}
    />
  );
  const total = model.totals[index];
  const whole = model.wholes[index];
  const target = targets[index];
  const hasTarget = Number.isFinite(target);
  const previous = slot > 0 ? model.order[slot - 1] : null;
  const segments = model.segments.filter((s) => s.category === index && s.series >= 0);
  const rows: TooltipRow[] = [];
  const foot: ReactNode[] = [];

  if (variant === "stack") {
    // Top of the stack first, so the card reads in the order the bar does.
    for (const segment of [...segments].reverse()) {
      const share = whole ? segment.raw / whole : 0;
      rows.push({
        label: segmentName(segment),
        color: segmentColour(segment),
        value: (
          <span className={styles.pair}>
            {percent ? (
              <>
                <span className={`${roles.micro} ${mutedText}`}>
                  {formatValue(segment.raw, { ...format, signed: false })}
                </span>
                {figure(share, roles.figureS, { kind: "percent", decimals: 0 })}
              </>
            ) : (
              <>
                <span className={`${roles.micro} ${mutedText}`}>{Math.round(share * 100)}%</span>
                {figure(segment.raw)}
              </>
            )}
          </span>
        ),
      });
    }
    foot.push(
      <span key="total" className={`${styles.total} ${roles.label}`}>
        <span>Total</span>
        {figure(total)}
      </span>,
    );
  } else {
    for (const segment of segments)
      rows.push({
        label: segmentName(segment),
        color: segmentColour(segment),
        value: figure(segment.raw),
      });
  }

  const against = compareValues ? compareValues[index] : NaN;
  if (variant !== "single" && compare && Number.isFinite(against))
    rows.push({
      label: seriesName(compare),
      color: colourOf(compare),
      muted: true,
      value: figure(against, `${roles.figureS} ${mutedText}`),
    });
  if (hasTarget)
    rows.push({
      label: targetLabel,
      color: "var(--chart-ink-3)",
      muted: true,
      value: figure(target, `${roles.figureS} ${mutedText}`),
    });

  const changed = change !== null && Number.isFinite(change);
  if (variant === "compare" && !changed) {
    if (compare && Number.isFinite(against) && against !== 0)
      foot.push(<Delta key="delta" value={(total - against) / Math.abs(against)} />);
    else if (hasTarget && target !== 0) {
      const change = (total - target) / Math.abs(target);
      foot.push(
        <Delta key="delta" value={change}>
          {`${change < 0 ? "−" : "+"}${Math.abs(change * 100).toFixed(1)}% vs ${targetLabel.toLowerCase()}`}
        </Delta>,
      );
    } else if (previous !== null && Number.isFinite(model.totals[previous]))
      foot.push(
        <TooltipFoot key="previous">
          {figure(total - model.totals[previous], "", { ...format, signed: true })} vs{" "}
          {shortName(previous)}
        </TooltipFoot>,
      );
  } else if (
    variant === "stack" &&
    !changed &&
    compare &&
    Number.isFinite(against) &&
    against !== 0
  )
    foot.push(<Delta key="delta" value={(total - against) / Math.abs(against)} />);
  if (changed)
    foot.push(
      <Delta key="change" value={change}>
        {`${change < 0 ? "−" : "+"}${Math.abs(change * 100).toFixed(1)}%${changeLabel ? ` vs ${changeLabel}` : ""}`}
      </Delta>,
    );

  if (projectedFrom !== null)
    foot.push(
      <span key="projected" className={`${styles.note} ${roles.caption}`}>
        <span
          className={styles.noteSwatch}
          style={{ backgroundColor: segments[0] ? segmentColour(segments[0]) : undefined }}
          aria-hidden="true"
        />
        {projectedFrom === 0 ? (
          <>Projected{stacked ? "" : ", not final"}</>
        ) : (
          <>
            {figure(projectedFrom * total, "", { ...format, signed: false })} actual, rest projected
          </>
        )}
      </span>,
    );

  return (
    <ChartTooltip
      ref={ref}
      title={title}
      rows={rows}
      footer={foot.length ? <>{foot}</> : undefined}
    />
  );
});
