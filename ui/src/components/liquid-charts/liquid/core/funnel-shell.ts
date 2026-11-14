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
import type { BarShell } from "./mark-types";

/** Validate convex boundary order, allowing a single collapsed edge for a triangle. */
export function validFunnelShell(points: readonly { x: number; y: number }[]): points is BarShell {
  if (points.length !== 4 || !points.every((p) => Number.isFinite(p.x) && Number.isFinite(p.y)))
    return false;
  const vertices = points.filter((p, i) => {
    const before = points[(i + 3) % 4];
    return p.x !== before.x || p.y !== before.y;
  });
  if (vertices.length < 3) return false;
  let sign = 0;
  for (let i = 0; i < vertices.length; i++) {
    const a = vertices[i],
      b = vertices[(i + 1) % vertices.length],
      c = vertices[(i + 2) % vertices.length];
    const cross = (b.x - a.x) * (c.y - b.y) - (b.y - a.y) * (c.x - b.x);
    if (!Number.isFinite(cross) || cross === 0 || (sign && Math.sign(cross) !== sign)) return false;
    sign = Math.sign(cross);
  }
  return true;
}

export function funnelShellBounds(shell: BarShell) {
  const x = Math.min(...shell.map((p) => p.x)),
    y = Math.min(...shell.map((p) => p.y));
  return {
    x,
    y,
    width: Math.max(...shell.map((p) => p.x)) - x,
    height: Math.max(...shell.map((p) => p.y)) - y,
  };
}
export function funnelShellPath(shell: BarShell) {
  return shell.map((p, i) => `${i ? "L" : "M"}${p.x} ${p.y}`).join("") + "Z";
}

/** Signed Euclidean distance to the actual convex boundary, also used by geometry tests. */
export function funnelShellDistance(shell: BarShell, point: { x: number; y: number }) {
  let squared = Infinity,
    positive = false,
    negative = false;
  for (let i = 0; i < 4; i++) {
    const a = shell[i],
      b = shell[(i + 1) % 4];
    const ex = b.x - a.x,
      ey = b.y - a.y,
      vx = point.x - a.x,
      vy = point.y - a.y;
    const length = ex * ex + ey * ey;
    if (!length) continue;
    const t = Math.max(0, Math.min(1, (vx * ex + vy * ey) / length));
    squared = Math.min(squared, (vx - ex * t) ** 2 + (vy - ey * t) ** 2);
    const cross = ex * vy - ey * vx;
    positive ||= cross > 0;
    negative ||= cross < 0;
  }
  return Math.sqrt(squared) * (positive && negative ? 1 : -1);
}
