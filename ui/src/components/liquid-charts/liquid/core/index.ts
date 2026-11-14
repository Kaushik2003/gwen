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
/** The Liquid Charts core: what every chart is built from, in the flat and light materials.
 * Glass, liquid and tilt come from the materials (`liquid-core-pro`), handed in as a material pack
 * (`materials.tsx`). The
 * other slices (`rings`, `beads`, `drag`, `zoom`, `export`, `senses`) build on this one. */
export * from "./style";
export * from "./geometry";
export * from "./scale";
export * from "./format";
export * from "./labels";
export * from "./colour";
export * from "./colour-scale";
export { readPageTheme, usePageTheme } from "./page-theme";
export type { BarFrame, BarMark, Direction, DotFrame, DotMark } from "./mark-types";
export * from "./materials";
export * from "./arc";
export * from "./camera";
export { useFlatMarks } from "./flat-marks";
export { FlatLayer, MarkLayer, MarkSurface, type FlatLayerProps } from "./mark-layers";
export * from "./tooltip-position";
export { ChartFrame, ChartScope, useActive, useChartFrame, type DataTable } from "./frame";
export { FrameSensesProvider, type FrameSenses } from "./frame-senses";
export {
  axisTicks,
  NameText,
  bareTick,
  CategoryAxis,
  type CategoryGroup,
  Gridlines,
  quietLines,
  TickText,
  type ValueAxis,
} from "./axis";
export { MAX_TYPE_SCALE, setTypeScale, typeScale } from "./type-scale";
export {
  KeyValue,
  LIFT,
  mutedText,
  useValueLabels,
  ValueLabel,
  type ValueLabelRequest,
} from "./value-labels";
export {
  ChartTooltip,
  Delta,
  DeltaArrow,
  moveTooltip,
  placeTooltip,
  TooltipExtra,
  TooltipFoot,
  type TooltipRow,
} from "./tooltip";
export { HeightNote, heightNoteText, Legend, type LegendEntry } from "./legend";
export { GradientLegend, strengthCss } from "./gradient-legend";
export { Segmented, Switch, ViewSwitch, toggleStyles, type SegmentOption } from "./toggles";
export {
  EventMarker,
  EventMarkers,
  eventLanes,
  LitGap,
  ReferenceBand,
  ReferenceLine,
} from "./references";
export { FocusMarker } from "./focus-marker";
export { focusAnchor, noteFocusAnchor, watchFocusAnchor } from "./focus-anchor";
export { emitLiquid, liquidFeel, onLiquid, type LiquidMoment } from "./liquid-feel";
export {
  createLightLines,
  drawLightLines,
  LightLineLayer,
  type LightFocus,
  type LightLine,
} from "./light-lines";
export {
  ChartState,
  SkeletonBlock,
  stateOf,
  type ChartStateKind,
  type SkeletonShape,
} from "./states";
export { digitWidths, Figure, formatFigure, type FigureKind } from "./figure";
