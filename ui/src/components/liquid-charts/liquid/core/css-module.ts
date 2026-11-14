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
/* A CSS module's classes, however the app's bundler loads the stylesheet.
 *
 * Normally a .module.css import maps each class to a scoped name. An app made with
 * create-next-app 16.4 or later with Tailwind sends every .css file through Tailwind's Turbopack
 * loader as plain CSS (`"*.css": { loaders: ["@tailwindcss/turbopack"], as: "*.css" }`), so a
 * .module.css loads as global CSS with its classes as written, and its import is empty. Every class
 * in the charts' stylesheets is written with its file's prefix (`lc-<file>-`), so a lookup falls
 * back to that written name and the rules still match, without meeting the app's own classes.
 *
 *   import barChartSheet from "./bar-chart.module.css";
 *   const styles = cssModule(barChartSheet, "lc-bar-chart-");
 *   <div className={styles.zero} />  // the scoped name, or "lc-bar-chart-zero" */
export function cssModule<T extends Readonly<Record<string, string>>>(sheet: T, prefix: string): T {
  const scoped = sheet && typeof sheet === "object" ? Object.entries(sheet) : [];
  if (scoped.length)
    return Object.fromEntries(
      scoped.map(([name, value]) => [
        name.startsWith(prefix) ? name.slice(prefix.length) : name,
        value,
      ]),
    ) as T;
  return new Proxy({} as T, {
    get: (_, name) => (typeof name === "string" ? prefix + name : undefined),
  });
}
