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

import { useRef, type KeyboardEvent, type ReactNode } from "react";
import { useMaterials } from "./materials";
import Styles from "./toggles.module.css";
import { cssModule } from "./css-module";
import Roles from "./type.module.css";
import type { Theme } from "./colour";

const styles = cssModule(Styles, "lc-toggles-");
const roles = cssModule(Roles, "lc-type-");

export type SegmentOption<T extends string> = { value: T; label: ReactNode };

/**
 * A segmented control: one choice of a few (a range, a segment, a view), as square hardware caps on
 * a recessed deck; the chosen cap is pressed down and its LED is lit. A radio group: arrow keys move the choice, and only the chosen key is in the tab
 * order. `value` may be null when none applies (a range brushed by hand).
 */
export function Segmented<T extends string>({
  options,
  value,
  onChange,
  label,
  theme,
  size = "standard",
  className,
}: {
  options: readonly SegmentOption<T>[];
  value: T | null;
  onChange: (value: T) => void;
  /** The group's accessible name. */
  label: string;
  theme?: Theme;
  size?: "small" | "standard";
  className?: string;
}) {
  const keys = useRef<(HTMLButtonElement | null)[]>([]);
  const at = Math.max(
    0,
    options.findIndex((option) => option.value === value),
  );
  function move(event: KeyboardEvent<HTMLDivElement>) {
    const step =
      event.key === "ArrowRight" || event.key === "ArrowDown"
        ? 1
        : event.key === "ArrowLeft" || event.key === "ArrowUp"
          ? -1
          : 0;
    if (!step) return;
    event.preventDefault();
    const next = (at + step + options.length) % options.length;
    onChange(options[next].value);
    keys.current[next]?.focus();
  }
  return (
    <div
      className={`${styles.well} ${className ?? ""}`}
      data-scheme={theme}
      data-size={size}
      data-export="skip"
      role="radiogroup"
      aria-label={label}
      onKeyDown={move}
    >
      {options.map((option, i) => (
        <button
          key={option.value}
          ref={(element) => {
            keys.current[i] = element;
          }}
          type="button"
          role="radio"
          aria-checked={option.value === value}
          data-on={option.value === value}
          tabIndex={i === at ? 0 : -1}
          className={`${styles.key} ${roles.casing}`}
          onClick={() => onChange(option.value)}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}

/**
 * The one view toggle: Flat or Tilt, for circular charts whose height means something. Tilt comes
 * with the materials: without a material pack that can tilt (`<LiquidMaterials>`) the switch
 * renders nothing, so a project without them never shows a key that does nothing. `available`
 * overrides that, for a chart that brings its own glass outside the provider.
 */
export function ViewSwitch({
  view,
  onChange,
  theme,
  label = "View",
  labels,
  available,
  className,
}: {
  view: "flat" | "tilt";
  onChange: (view: "flat" | "tilt") => void;
  theme?: Theme;
  label?: string;
  /** The keys' labels, if not "Flat" and "Tilt". */
  labels?: { flat?: ReactNode; tilt?: ReactNode };
  /** Whether the chart can tilt here; defaults to whether the materials in reach can. */
  available?: boolean;
  className?: string;
}) {
  const { ring } = useMaterials();
  if (!(available ?? Boolean(ring))) return null;
  const options: SegmentOption<"flat" | "tilt">[] = [
    { value: "flat", label: labels?.flat ?? "Flat" },
    { value: "tilt", label: labels?.tilt ?? "Tilt" },
  ];
  return (
    <Segmented
      options={options}
      value={view}
      onChange={onChange}
      label={label}
      theme={theme}
      size="small"
      className={className}
    />
  );
}

/** A one-press toggle: a single hardware cap that stays down with its LED lit while it is on (show a
 * series, mute a layer, follow live data). Give it an icon or a short label as children; `label` names it for
 * assistive tech. */
export function Switch({
  on,
  onChange,
  label,
  theme,
  className,
  children,
}: {
  on: boolean;
  onChange: (on: boolean) => void;
  /** The key's accessible name. */
  label: string;
  theme?: Theme;
  className?: string;
  children?: ReactNode;
}) {
  return (
    <button
      type="button"
      aria-pressed={on}
      aria-label={label}
      data-scheme={theme}
      data-export="skip"
      className={`${styles.toggleKey} ${roles.casing} ${className ?? ""}`}
      onClick={() => onChange(!on)}
    >
      {children}
    </button>
  );
}

/** The deck, cap and single-cap classes, for controls that look like toggles but aren't one (a menu
 * trigger, a button that opens something). */
/** Class names for keys a chart lays out itself; `key` and `toggleKey` carry the casing type role. */
export const toggleStyles = {
  well: styles.well,
  key: `${styles.key} ${roles.casing}`,
  toggleKey: `${styles.toggleKey} ${roles.casing}`,
};
