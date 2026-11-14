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
 * When a chart's motion runs, for every mark hook's animation loop.
 *
 * - `chartPresence`: a chart animates only while it is on screen in a visible tab, and its
 *   entrance waits until it is about a third visible, so a chart below the fold pours when you
 *   reach it instead of unseen at load. Just outside the viewport (a margin) it may draw a frame,
 *   so it has drawn before it scrolls in, but it never loops there. Further off the hook skips its
 *   step and keeps all its state; `clock` holds meanwhile, so delays and pours resume where they
 *   left off.
 * - `frameDelta`: the real time a frame integrates. Past `SNAP_SECONDS` the motion snaps to its
 *   end, so a slow frame stutters and never plays in slow motion; `substeps` keeps the springs
 *   stable over a long frame.
 * - `sizeCanvas` and `lostContext`: while the shared GPU context is lost a chart keeps its last
 *   frame (through a resize too) and its engine, and repaints on the restore.
 * - `settleCharts`: print (`beforeprint`) and export draw every chart at once in its settled state,
 *   wherever it is on the page. A chart that was never on screen has never drawn, so a print or an
 *   export would otherwise show its card empty.
 */

/**
 * Sizes a renderer's 2D output canvas, which clears it. With `keep` (its shared GPU context is
 * lost, so nothing can repaint it until the restore) the picture is carried over, anchored at the
 * top left, so the chart keeps its last frame instead of going blank.
 */
export function sizeCanvas(canvas: HTMLCanvasElement, width: number, height: number, keep = false) {
  if (canvas.width === width && canvas.height === height) return;
  let copy: HTMLCanvasElement | null = null;
  if (keep && canvas.width > 1 && canvas.height > 1) {
    copy = canvas.ownerDocument.createElement("canvas");
    copy.width = canvas.width;
    copy.height = canvas.height;
    copy.getContext("2d")?.drawImage(canvas, 0, 0);
  }
  canvas.width = width;
  canvas.height = height;
  if (copy) canvas.getContext("2d")?.drawImage(copy, 0, 0);
}

/**
 * An optical draw that failed only because the shared GPU context is lost. That is temporary: the
 * chart keeps its engine and last frame, and the optical renderer repaints it on the restore
 * (announced by a `chart-optical-status` event on the canvas). Only other failures mean "draw flat".
 */
export function lostContext(result: {
  available: boolean;
  fallback: readonly { reason: string }[];
}) {
  return !result.available && result.fallback.every((entry) => entry.reason === "context-lost");
}

/** A frame longer than this (s) snaps the motion to its end instead of integrating it. */
export const SNAP_SECONDS = 0.25;
/** The longest step the core's springs (k 110, c 21) take at once: the old per-frame clamp. */
const MAX_STEP = 0.05;

/** A frame's time step: `dt` (s) since `last` (1/60 on the first frame of a run, when `last` is
 * 0), and whether it is so long the motion should snap to its end. */
export function frameDelta(now: number, last: number) {
  const dt = last ? Math.max(0, (now - last) / 1000) : 1 / 60;
  return { dt, snap: dt > SNAP_SECONDS };
}

/** A visual delay on the same clock as chart motion. Unlike a wall-clock timeout this also
 * follows a host's stepped capture clock. Cancel on unmount/replay, just like a normal rAF loop. */
export function afterFrameDelay(milliseconds: number, callback: () => void): () => void {
  const start = performance.now();
  let stopped = false;
  let frame = 0;
  const step = (now: number) => {
    if (stopped) return;
    if (now - start >= milliseconds) {
      stopped = true;
      callback();
    } else frame = window.requestAnimationFrame(step);
  };
  frame = window.requestAnimationFrame(step);
  return () => {
    stopped = true;
    window.cancelAnimationFrame(frame);
  };
}

/** How many equal substeps integrate `dt` stably (each at most 50 ms). */
export function substeps(dt: number) {
  return Math.max(1, Math.ceil(dt / MAX_STEP));
}

/** How far (ms) a settled chart's clock jumps ahead: past every entrance and delay. */
const SETTLE_JUMP = 1e6;

type Settler = { root: Element; settle: () => void };
const settlers = new Set<Settler>();
const printing = new WeakSet<Window>();
const staticScopes = new Map<
  Element,
  { count: number; previous: string | null; style: HTMLStyleElement }
>();

/** Whether a chart is inside an explicitly authored static-glass export scope. */
export function isStaticGlass(node: Element): boolean {
  return !!node.closest('[data-lc-render-mode="static-glass"]');
}

/** Hold a motion-free glass frame until the returned, idempotent release is called.
 * Nested exports share the scope. Geometry/data settle; material quality and DPR stay intact.
 * Release restores the interactive material at rest, as settleCharts already does for exports.
 */
export function beginStaticGlass(scope: Element | Document = document): () => void {
  const root = scope.nodeType === 9 ? (scope as Document).documentElement : (scope as Element);
  let entry = staticScopes.get(root);
  if (!entry) {
    const style = root.ownerDocument.createElement("style");
    style.textContent =
      '[data-lc-render-mode="static-glass"], [data-lc-render-mode="static-glass"] * { animation: none !important; transition: none !important; pointer-events: none !important; }';
    entry = { count: 0, previous: root.getAttribute("data-lc-render-mode"), style };
    staticScopes.set(root, entry);
    root.setAttribute("data-lc-render-mode", "static-glass");
    root.ownerDocument.head.append(style);
  }
  entry.count++;
  let released = false;
  const release = () => {
    if (released) return;
    released = true;
    if (--entry.count) return;
    staticScopes.delete(root);
    if (entry.previous === null) root.removeAttribute("data-lc-render-mode");
    else root.setAttribute("data-lc-render-mode", entry.previous);
    entry.style.remove();
    settleCharts(root);
  };
  try {
    settleCharts(root);
  } catch (error) {
    release();
    throw error;
  }
  return release;
}

/**
 * Draws every chart inside `scope` (default: the whole page) now, settled: its entrance finished
 * and every motion at its end, on screen or not. For print and export. Each chart's loop draws
 * synchronously with `presence.settling` true (its last frame with the clock jumped past every
 * entrance), then carries on as before; a chart settled this way counts as entered, so it doesn't
 * replay its entrance when it later scrolls into view.
 */
export function settleCharts(scope?: Node | null) {
  for (const settler of [...settlers]) if (!scope || scope.contains(settler.root)) settler.settle();
}

/**
 * Calls `settle` whenever `settleCharts` settles a scope holding `root`: for motion a chart runs
 * outside its mark loop, such as a needle swung by a delayed state update, which a settled draw
 * must show at its end too. It runs synchronously, inside the settle. Returns the unregister.
 */
export function onSettle(root: Element, settle: () => void): () => void {
  const settler: Settler = { root, settle };
  settlers.add(settler);
  return () => {
    settlers.delete(settler);
  };
}

/**
 * A 2D canvas drawn while the page prints is printed from its recorded drawing commands, and the
 * PDF backend can't express what the materials use (additive light, blend modes, clears): light
 * lines print as solid blocks and cleared canvases as white boxes. So for the print, each chart
 * canvas is hidden behind a fresh canvas holding one bitmap of it, drawn once over nothing, which
 * prints as the picture it is. Returns the undo, run after the print.
 */
function printCopies(doc: Document): () => void {
  if (typeof OffscreenCanvas === "undefined") return () => {};
  const seen = new Set<HTMLCanvasElement>();
  const undo: (() => void)[] = [];
  for (const { root } of settlers) {
    if (root.ownerDocument !== doc) continue;
    // The chart's own layers sit in its plot, beside the element it is watched through.
    for (const canvas of Array.from((root.parentElement ?? root).querySelectorAll("canvas"))) {
      if (seen.has(canvas) || !canvas.width || !canvas.height || !canvas.isConnected) continue;
      seen.add(canvas);
      if (doc.defaultView?.getComputedStyle(canvas).display === "none") continue;
      const bitmap = new OffscreenCanvas(canvas.width, canvas.height);
      bitmap.getContext("2d")?.drawImage(canvas, 0, 0);
      const copy = doc.createElement("canvas");
      copy.width = canvas.width;
      copy.height = canvas.height;
      copy.className = canvas.className;
      copy.style.cssText = canvas.style.cssText;
      copy.setAttribute("aria-hidden", "true");
      copy.getContext("2d")?.drawImage(bitmap, 0, 0);
      canvas.after(copy);
      const visibility = canvas.style.visibility;
      canvas.style.visibility = "hidden";
      undo.push(() => {
        copy.remove();
        canvas.style.visibility = visibility;
      });
    }
  }
  return () => undo.forEach((step) => step());
}

export type ChartPresence = {
  /** Within the margin of the viewport in a visible tab: worth drawing a frame. */
  readonly active: boolean;
  /** The chart has been about a third visible: its entrance runs. */
  readonly entered: boolean;
  /** On screen, entered, in a visible tab: worth animating (looping). */
  readonly running: boolean;
  /** True during a settled frame (print, export): draw it whatever `active` says, with every
   * spring, ease and pour at its end. */
  readonly settling: boolean;
  /** Explicit static material, independent of the user’s reduced-motion preference. */
  readonly staticGlass: boolean;
  /** `now` less every moment the chart was not running (offscreen, hidden, or waiting to enter):
   * the clock entrances and frame steps run on. */
  clock(now: number): number;
  stop(): void;
};

/**
 * Watches `root` (a chart's plot box) and calls `wake` when the chart comes near or on screen,
 * enters, or its tab is shown again. `enter` is the share of the chart (or of the viewport, for a
 * chart taller than it) that must show before the entrance starts; `margin` (px) is how far outside
 * the viewport it may draw. `redraw` draws a frame synchronously (cancelling a pending one), for
 * print and export; without it a settle only wakes the loop.
 */
export function chartPresence(
  root: Element,
  wake: () => void,
  {
    enter = 0.3,
    margin = 160,
    redraw,
  }: { enter?: number; margin?: number; redraw?: () => void } = {},
): ChartPresence {
  const doc = root.ownerDocument;
  let near = false,
    visible = false,
    entered = false,
    hidden = doc.visibilityState === "hidden",
    heldSince: number | null = performance.now(),
    held = 0,
    jumped = 0,
    settling = false,
    // The window resized since the observers last reported: their answer may be stale (they report
    // a task later, after the resize observers have already redrawn the canvases they cleared).
    resized = false;
  const update = () => {
    const live = visible && entered && !hidden;
    const now = performance.now();
    if (live && heldSince !== null) {
      held += now - heldSince;
      heldSince = null;
    } else if (!live && heldSince === null) heldSince = now;
  };
  const onVisibility = () => {
    hidden = doc.visibilityState === "hidden";
    update();
    if (!hidden) wake();
  };
  doc.addEventListener("visibilitychange", onVisibility);
  let nearby: IntersectionObserver | null = null,
    onScreen: IntersectionObserver | null = null;
  if (typeof IntersectionObserver === "undefined") {
    near = visible = entered = true;
    update();
  } else {
    nearby = new IntersectionObserver(
      (entries) => {
        const next = entries[entries.length - 1].isIntersecting;
        if (next === near) return;
        near = next;
        update();
        if (near) wake();
      },
      { rootMargin: `${margin}px 0px` },
    );
    onScreen = new IntersectionObserver(
      (entries) => {
        const entry = entries[entries.length - 1];
        const view = entry.rootBounds?.height ?? doc.defaultView?.innerHeight ?? 0;
        const shown =
          entry.isIntersecting &&
          (entry.intersectionRatio >= enter - 1e-3 ||
            entry.intersectionRect.height >= view * enter);
        const next = entry.isIntersecting;
        if (next === visible && (entered || !shown)) return;
        visible = next;
        entered ||= shown;
        update();
        if (visible) wake();
      },
      { threshold: [0, 0.05, 0.1, 0.15, 0.2, 0.25, enter] },
    );
    nearby.observe(root);
    onScreen.observe(root);
  }
  const settler: Settler = {
    root,
    settle() {
      if (!entered) {
        entered = true;
        update();
      }
      settling = true;
      try {
        // Twice: first at the chart's own time, so an entrance that a chart starts on its first
        // frame (a never-drawn chart's) has started; then with the clock past every entrance.
        (redraw ?? wake)();
        jumped += SETTLE_JUMP;
        (redraw ?? wake)();
      } finally {
        settling = false;
      }
    },
  };
  settlers.add(settler);
  const view = doc.defaultView;
  const onResize = () => {
    resized = true;
  };
  view?.addEventListener("resize", onResize);
  if (view && !printing.has(view)) {
    printing.add(view);
    let restore = () => {};
    view.addEventListener("beforeprint", () => {
      restore();
      const release = beginStaticGlass(view.document);
      try {
        const copies = printCopies(view.document);
        restore = () => {
          copies();
          release();
        };
      } catch (error) {
        release();
        throw error;
      }
    });
    view.addEventListener("afterprint", () => {
      restore();
      restore = () => {};
    });
  }
  return {
    get active() {
      if (resized && view && nearby) {
        // Asked during a resize (a canvas was just cleared and wants redrawing at once): measure
        // instead of waiting a frame for the observer, which would show the canvas blank.
        resized = false;
        const r = root.getBoundingClientRect();
        near = r.bottom > -margin && r.top < view.innerHeight + margin && r.width > 0;
      }
      return (near && !hidden) || settling || isStaticGlass(root);
    },
    get entered() {
      return entered;
    },
    get running() {
      return visible && entered && !hidden && !isStaticGlass(root);
    },
    get settling() {
      return settling || isStaticGlass(root);
    },
    get staticGlass() {
      return isStaticGlass(root);
    },
    clock(now: number) {
      return now - held - (heldSince === null ? 0 : Math.max(0, now - heldSince)) + jumped;
    },
    stop() {
      nearby?.disconnect();
      onScreen?.disconnect();
      doc.removeEventListener("visibilitychange", onVisibility);
      view?.removeEventListener("resize", onResize);
      settlers.delete(settler);
    },
  };
}
