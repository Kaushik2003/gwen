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

import { ChartFrame } from "@/components/liquid-charts/liquid/core/frame";
import type { ComponentProps } from "react";

/** DonutChart's original visible-item order. Values are already normalized by its caller. */
export function donutVisibleItems(values: readonly number[]) {
  return values.map((_, index) => index).filter((index) => values[index] > 0);
}

/** Original DonutChart mark inputs; the ring renderer still owns all easing and liquid motion. */
export function donutMarkFocus(
  active: number | null,
  index: number,
  { tilted = false, tile = false }: { tilted?: boolean; tile?: boolean } = {},
) {
  return {
    lit: active === null || active === index ? 1 : tilted ? 0.32 : 0,
    pop: !tilted && active === index ? (tile ? 7 : 16) : 0,
  };
}

/** Original DonutChart mapping between data indices and ChartFrame's visible-item positions. */
export function donutFrameSelection(
  visible: readonly number[],
  active: number | null,
  disabled: boolean,
  pickAt: (x: number, y: number) => number | null,
) {
  const frameActive = active === null ? null : visible.indexOf(active);
  return {
    count: disabled ? 0 : visible.length,
    active: frameActive !== null && frameActive >= 0 ? frameActive : null,
    index: (position: number | null) => (position === null ? null : (visible[position] ?? null)),
    pick: (x: number, y: number) => {
      if (disabled) return null;
      const index = pickAt(x, y);
      if (index === null) return null;
      const position = visible.indexOf(index);
      return position >= 0 ? position : null;
    },
  };
}

type FrameProps = ComponentProps<typeof ChartFrame>;
export type DonutInteractionFrameProps = Omit<
  FrameProps,
  "count" | "active" | "onActiveChange" | "pick"
> & {
  visible: readonly number[];
  active: number | null;
  disabled?: boolean;
  onActiveChange: (index: number | null) => void;
  pick: (x: number, y: number) => number | null;
};

/** Keep the genuine native pointer arrival/move/leave, click pinning and keyboard handlers.
 * No event policy is reimplemented here; ChartFrame remains their sole owner. */
export function DonutInteractionFrame({
  visible,
  active,
  disabled = false,
  onActiveChange,
  pick,
  ...props
}: DonutInteractionFrameProps) {
  const selection = donutFrameSelection(visible, active, disabled, pick);
  return (
    <ChartFrame
      {...props}
      count={selection.count}
      active={selection.active}
      onActiveChange={(position) => onActiveChange(selection.index(position))}
      pick={selection.pick}
    />
  );
}
