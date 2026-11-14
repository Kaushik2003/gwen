"use client";
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

import { SIGNATURE_PATH, SIGNATURE_VIEWBOX } from "@/components/liquid-charts/liquid/credit/signature";
import { createContext, useContext, type ReactNode } from "react";
import { LIQUID_CREDIT } from "@/components/liquid-charts/liquid/credit/switch";
import { cssModule } from "@/components/liquid-charts/liquid/core/css-module";
import Styles from "./credit.module.css";

const styles = cssModule(Styles, "lc-credit-");

/** Where the credit goes: the site, with a plain `ref` so visits from charts can be counted, and
 * nothing about the page or the visitor. */
export const CREDIT_URL = "https://liquid.ojieame.design/?ref=credit";

/** The mark itself: the brush "Liquid" signature in the chart's quietest ink, linking to the site.
 * It is the chart's last line, centred under everything, the legend included (`ChartFrame`). */
export function CreditMark() {
  return (
    <a
      className={styles.credit}
      href={CREDIT_URL}
      target="_blank"
      rel="noopener"
      aria-label="Chart made with Liquid Charts"
    >
      <svg viewBox={SIGNATURE_VIEWBOX} aria-hidden="true" focusable="false">
        <path d={SIGNATURE_PATH} />
      </svg>
    </a>
  );
}

const Hosted = createContext(false);

/** A host that shows one credit for everything in it (a dashboard of many charts): the charts
 * inside it show none, and the host shows its own with `<LiquidCredit host />`, once. */
export function CreditHost({ children }: { children: ReactNode }) {
  return <Hosted.Provider value={true}>{children}</Hosted.Provider>;
}

/** The credit a chart shows: the mark in the free registry, nothing in the Pro build. A chart
 * hands it to its frame (`<ChartFrame credit={<LiquidCredit />}>`). Inside a `CreditHost` only the
 * host's own credit (`host`) shows. */
export function LiquidCredit({ host = false }: { host?: boolean } = {}) {
  const hosted = useContext(Hosted);
  if (!LIQUID_CREDIT || (hosted && !host)) return null;
  return <CreditMark />;
}
