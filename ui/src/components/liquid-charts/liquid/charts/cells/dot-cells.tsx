"use client";

/**
 * The heatmap's circle variant: one round glass bead per cell, sized by the value (its area
 * follows the value's strength) in the scale's tint, on the core's `useDotMarks`. Without WebGL the
 * same beads draw flat on the cell canvas. Kept behind this one component so the heatmap never
 * draws circles itself.
 */

import { type DotMark, type Theme } from "@/components/liquid-charts/liquid/core";
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
import { useDotMarks } from "@/components/liquid-charts/liquid/dots";
import type { FlatCell } from "./cell-canvas";
import { useEffect, useState } from "react";
export { beadRadius } from "./layout";
import { cssModule } from "@/components/liquid-charts/liquid/core/css-module";
import { FlatCellLayer } from "./cell-parts";
import Styles from "./cells.module.css";

const styles = cssModule(Styles, "lc-cells-");

export function DotCells({
  theme,
  replay,
  layout,
  flat,
}: {
  theme: Theme;
  replay: number;
  layout: (width: number, height: number) => DotMark[];
  /** The same beads as flat cells, for browsers without WebGL. */
  flat: (width: number, height: number) => FlatCell[];
}) {
  const [webgl, setWebgl] = useState(true);
  if (!webgl) return <FlatCellLayer theme={theme} replay={replay} layout={flat} />;
  return (
    <Beads theme={theme} replay={replay} layout={layout} onUnavailable={() => setWebgl(false)} />
  );
}

function Beads({
  onUnavailable,
  ...props
}: {
  theme: Theme;
  replay: number;
  layout: (width: number, height: number) => DotMark[];
  onUnavailable: () => void;
}) {
  const { box, canvas, available } = useDotMarks(props);
  useEffect(() => {
    if (!available) onUnavailable();
  }, [available, onUnavailable]);
  return (
    <div ref={box} className={styles.layer}>
      <canvas ref={canvas} className={styles.canvas} aria-hidden="true" />
    </div>
  );
}
