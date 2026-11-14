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

/* Icons: Solar (magnifier-zoom-in-bold, magnifier-zoom-out-bold, restart-bold) by 480 Design,
 * CC BY 4.0, https://www.figma.com/community/file/1166831539721848736 — credit required. */

import { cssModule } from "@/components/liquid-charts/liquid/core/css-module";
import type { Rect } from "@/components/liquid-charts/liquid/core/geometry";
import type { Theme } from "@/components/liquid-charts/liquid/core/colour";
import Core from "@/components/liquid-charts/liquid/core/core.module.css";
import { useEffect, useRef, useState } from "react";
import Styles from "./zoom-controls.module.css";
import type { Zoom } from "./brush";

const core = cssModule(Core, "lc-core-");
const styles = cssModule(Styles, "lc-zoom-controls-");

const ZOOM_IN =
  "M21.7883 21.7883C22.0706 21.506 22.0706 21.0483 21.7883 20.7659L18.1224 17.1001C19.4885 15.5006 20.3133 13.4249 20.3133 11.1566C20.3133 6.09956 16.2137 2 11.1566 2C6.09956 2 2 6.09956 2 11.1566C2 16.2137 6.09956 20.3133 11.1566 20.3133C13.4249 20.3133 15.5006 19.4885 17.1001 18.1224L20.7659 21.7883C21.0483 22.0706 21.506 22.0706 21.7883 21.7883ZM11.1566 8.0241C11.5559 8.0241 11.8795 8.34775 11.8795 8.74699V10.4337H13.5663C13.9655 10.4337 14.2892 10.7574 14.2892 11.1566C14.2892 11.5559 13.9655 11.8795 13.5663 11.8795H11.8795V13.5663C11.8795 13.9655 11.5559 14.2892 11.1566 14.2892C10.7574 14.2892 10.4337 13.9655 10.4337 13.5663V11.8795H8.74699C8.34775 11.8795 8.0241 11.5559 8.0241 11.1566C8.0241 10.7574 8.34775 10.4337 8.74699 10.4337H10.4337V8.74699C10.4337 8.34775 10.7574 8.0241 11.1566 8.0241Z";
const ZOOM_OUT =
  "M21.7883 21.7883C22.0706 21.506 22.0706 21.0483 21.7883 20.7659L18.1224 17.1002C19.4884 15.5007 20.3133 13.425 20.3133 11.1566C20.3133 6.09956 16.2137 2 11.1566 2C6.09956 2 2 6.09956 2 11.1566C2 16.2137 6.09956 20.3133 11.1566 20.3133C13.4249 20.3133 15.5006 19.4885 17.1 18.1225L20.7659 21.7883C21.0483 22.0706 21.506 22.0706 21.7883 21.7883ZM8.0241 11.1566C8.0241 10.7574 8.34775 10.4337 8.74699 10.4337H13.5663C13.9655 10.4337 14.2892 10.7574 14.2892 11.1566C14.2892 11.5559 13.9655 11.8795 13.5663 11.8795H8.74699C8.34775 11.8795 8.0241 11.5559 8.0241 11.1566Z";
const RESTART =
  "M18.2577 3.50828C18.538 3.62437 18.7207 3.89785 18.7207 4.20119V8.44383C18.7207 8.85805 18.3849 9.19383 17.9707 9.19383H13.728C13.4247 9.19383 13.1512 9.0111 13.0351 8.73085C12.9191 8.45059 12.9832 8.128 13.1977 7.9135L14.8007 6.3105C12.1674 5.20912 9.01606 5.7309 6.87348 7.87348C4.04217 10.7048 4.04217 15.2952 6.87348 18.1265C9.70478 20.9578 14.2952 20.9578 17.1265 18.1265C18.7727 16.4803 19.4622 14.2401 19.1935 12.0937C19.142 11.6827 19.4335 11.3078 19.8445 11.2563C20.2555 11.2049 20.6304 11.4963 20.6819 11.9073C21.0057 14.4934 20.1746 17.1997 18.1872 19.1872C14.7701 22.6043 9.2299 22.6043 5.81282 19.1872C2.39573 15.7701 2.39573 10.2299 5.81282 6.81282C8.55119 4.07444 12.6515 3.5312 15.9309 5.18028L17.4404 3.67086C17.6549 3.45637 17.9774 3.3922 18.2577 3.50828Z";

function Icon({ d, evenodd = true }: { d: string; evenodd?: boolean }) {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <path
        d={d}
        fillRule={evenodd ? "evenodd" : undefined}
        clipRule={evenodd ? "evenodd" : undefined}
      />
    </svg>
  );
}

/**
 * Zoom in, zoom out and back to the whole view: three raised keys in the plot's top right corner,
 * above the marks. They act on the core zoom (the same one pinch, ⌘/Ctrl-scroll, drag, `+`, `-`
 * and `0` drive). `wholeLabel` names the third key ("Show the whole map").
 */
export function ZoomControls({
  zoom,
  plot,
  theme,
  wholeLabel = "Show the whole view",
}: {
  zoom: Zoom;
  plot: Rect;
  theme: Theme;
  wholeLabel?: string;
}) {
  const keep = (event: { stopPropagation: () => void }) => event.stopPropagation();
  const atMost = zoom.target.x[1] - zoom.target.x[0] <= (zoom.extent.x[1] - zoom.extent.x[0]) / 7.9;
  return (
    <div
      className={styles.zoomKeys}
      data-scheme={theme}
      data-export="skip"
      style={{ top: plot.top + 6, right: `calc(100% - ${plot.right - 6}px)` }}
      onPointerDown={keep}
      onPointerMove={keep}
      onClick={keep}
      onDoubleClick={keep}
      onKeyDown={keep}
    >
      <button
        type="button"
        className={styles.zoomKey}
        aria-label="Zoom in"
        disabled={atMost}
        onClick={() => zoom.zoomBy(0.6)}
      >
        <Icon d={ZOOM_IN} />
      </button>
      <button
        type="button"
        className={styles.zoomKey}
        aria-label="Zoom out"
        disabled={!zoom.zoomed}
        onClick={() => zoom.zoomBy(1 / 0.6)}
      >
        <Icon d={ZOOM_OUT} />
      </button>
      <button
        type="button"
        className={styles.zoomKey}
        aria-label={wholeLabel}
        disabled={!zoom.zoomed}
        onClick={() => zoom.reset()}
      >
        <Icon d={RESTART} evenodd={false} />
      </button>
    </div>
  );
}

/** Reads the zoom aloud once it settles ("Zoomed in 2.8 times", "Showing the whole map"). Put it
 * outside the plot, so the plot's own live line stays its first. (`ResetZoom` announces for itself;
 * use this with `ZoomControls`.) */
export function ZoomAnnouncer({
  zoom,
  describe,
}: {
  zoom: Zoom;
  describe: (zoomed: boolean, scale: number) => string;
}) {
  const scale = 1 / Math.max(1e-9, zoom.target.x[1] - zoom.target.x[0]);
  const words = describe(zoom.zoomed, scale);
  const [spoken, setSpoken] = useState("");
  const was = useRef(false);
  useEffect(() => {
    if (!zoom.zoomed && !was.current) return;
    was.current = zoom.zoomed;
    const timer = window.setTimeout(() => setSpoken(words), 400);
    return () => window.clearTimeout(timer);
  }, [words, zoom.zoomed]);
  return (
    <span className={core.srOnly} aria-live="polite">
      {spoken}
    </span>
  );
}
