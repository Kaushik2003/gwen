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
import { useSyncExternalStore } from "react";
import type { Theme } from "./colour";

/* A chart's theme when none is passed: the page's. Theme switchers mark it on <html> (or <body>)
 * as `data-theme="dark"` or a `dark` class, `light` the same way; without a mark, the system
 * setting. Read through useSyncExternalStore, so the client's first paint already has the page's
 * scheme and follows it as it changes. */

const DARK = "(prefers-color-scheme: dark)";
const WATCH: MutationObserverInit = { attributes: true, attributeFilter: ["class", "data-theme"] };

/** The page's scheme, as a chart would take it. */
export function readPageTheme(): Theme {
  for (const element of [document.documentElement, document.body]) {
    if (!element) continue;
    const marked = element.getAttribute("data-theme");
    if (marked === "dark" || marked === "light") return marked;
    if (element.classList.contains("dark")) return "dark";
    if (element.classList.contains("light")) return "light";
  }
  return window.matchMedia?.(DARK).matches ? "dark" : "light";
}

function subscribe(change: () => void) {
  const observer = new MutationObserver(change);
  observer.observe(document.documentElement, WATCH);
  if (document.body) observer.observe(document.body, WATCH);
  const media = window.matchMedia?.(DARK);
  media?.addEventListener("change", change);
  return () => {
    observer.disconnect();
    media?.removeEventListener("change", change);
  };
}

/** On the server there is no page yet: dark, the charts' default before they followed the page. A
 * chart draws nothing there until it's measured, so the server's guess never shows. */
const onServer = (): Theme => "dark";

/** The theme a chart draws in: the one it was given, else the page's (`readPageTheme`). */
export function usePageTheme(given?: Theme): Theme {
  const page = useSyncExternalStore(subscribe, readPageTheme, onServer);
  return given ?? page;
}
