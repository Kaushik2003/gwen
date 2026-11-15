import { LoaderCircle, Maximize2, Minus, Plus } from "lucide-react";
import * as pdfjs from "pdfjs-dist/legacy/build/pdf.mjs";
import workerUrl from "pdfjs-dist/legacy/build/pdf.worker.min.mjs?url";
import { EventBus, PDFLinkService, PDFViewer } from "pdfjs-dist/legacy/web/pdf_viewer.mjs";
import "pdfjs-dist/legacy/web/pdf_viewer.css";
import { useEffect, useRef, useState } from "react";
import { IconButton } from "../components/ui";
import { BrowserOpenURL } from "../wailsjs/runtime/runtime";

// A PDF in its file tab (MediaView): Mozilla's PDF.js viewer, which draws pages as they scroll into view and
// lays text over them so it can be selected and copied. Its own chunk, loaded the first time a PDF opens.
// A new url (the file changed on disk) loads the new file at the same page.

pdfjs.GlobalWorkerOptions.workerSrc = workerUrl;

const MIN_SCALE = 0.25;
const MAX_SCALE = 5;

export default function PdfView({ url, onInfo }: { url: string; onInfo?: (pages: number) => void }) {
  const box = useRef<HTMLDivElement>(null);
  const viewer = useRef<PDFViewer | null>(null);
  const links = useRef<PDFLinkService | null>(null);
  const [page, setPage] = useState(1);
  const [pages, setPages] = useState(0);
  const [scale, setScale] = useState(1);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const info = useRef(onInfo);
  info.current = onInfo;

  // The viewer, once.
  useEffect(() => {
    const container = box.current;
    if (!container) return;
    const eventBus = new EventBus();
    const linkService = new PDFLinkService({ eventBus });
    const v = new PDFViewer({ container, eventBus, linkService });
    linkService.setViewer(v);
    viewer.current = v;
    links.current = linkService;
    eventBus.on("pagechanging", (e: { pageNumber: number }) => setPage(e.pageNumber));
    eventBus.on("scalechanging", (e: { scale: number }) => setScale(e.scale));
    // Ctrl + wheel zooms, as in a browser.
    const onWheel = (e: WheelEvent) => {
      if (!e.ctrlKey) return;
      e.preventDefault();
      v.currentScale = Math.min(MAX_SCALE, Math.max(MIN_SCALE, v.currentScale * (e.deltaY < 0 ? 1.1 : 1 / 1.1)));
    };
    // A web link in the PDF opens in the browser, never in the Gwen window.
    const onClick = (e: MouseEvent) => {
      const a = (e.target as HTMLElement).closest("a[href]");
      const href = a?.getAttribute("href") ?? "";
      if (/^(https?|mailto):/i.test(href)) {
        e.preventDefault();
        e.stopPropagation();
        BrowserOpenURL(href);
      }
    };
    container.addEventListener("wheel", onWheel, { passive: false });
    container.addEventListener("click", onClick, true);
    return () => {
      container.removeEventListener("wheel", onWheel);
      container.removeEventListener("click", onClick, true);
      v.cleanup();
      viewer.current = null;
    };
  }, []);

  // The document at url; a later url replaces it and keeps the page.
  useEffect(() => {
    const v = viewer.current;
    if (!v) return;
    let live = true;
    const keepPage = v.pagesCount ? v.currentPageNumber : 1;
    const keepScale = v.pagesCount ? v.currentScaleValue : "page-width";
    // Destroying the loading task frees the document it loaded.
    const task = pdfjs.getDocument({ url });
    setError("");
    task.promise.then(
      (d) => {
        if (!live) return;
        v.eventBus.on(
          "pagesinit",
          () => {
            v.currentScaleValue = keepScale || "page-width";
            if (keepPage > 1 && keepPage <= d.numPages) v.currentPageNumber = keepPage;
          },
          { once: true },
        );
        v.setDocument(d);
        links.current?.setDocument(d, null);
        setPages(d.numPages);
        info.current?.(d.numPages);
        setLoading(false);
      },
      (e: unknown) => {
        if (!live) return;
        setError(e instanceof Error ? e.message : String(e));
        setLoading(false);
      },
    );
    return () => {
      live = false;
      void task.destroy();
    };
  }, [url]);

  const zoom = (f: number) => {
    const v = viewer.current;
    if (v) v.currentScale = Math.min(MAX_SCALE, Math.max(MIN_SCALE, v.currentScale * f));
  };

  return (
    <div className="relative min-h-0 flex-1 bg-canvas">
      <div ref={box} className="pdf-box absolute inset-0 overflow-auto">
        <div className="pdfViewer" />
      </div>
      {loading && (
        <p className="absolute top-4 left-5 flex items-center gap-2 text-[13px] text-ink-subtle">
          <LoaderCircle size={14} className="animate-spin" aria-hidden />
          Opening the PDF…
        </p>
      )}
      {error && <p className="absolute top-4 left-5 text-[13px] text-danger">Couldn't read this PDF: {error}</p>}
      {pages > 0 && (
        <div className="absolute bottom-4 left-1/2 flex -translate-x-1/2 items-center gap-1 rounded-full border border-line bg-surface-2/95 px-2 py-1 text-xs text-ink-muted shadow-lg backdrop-blur">
          <span className="px-2 tabular-nums">
            Page {page} of {pages}
          </span>
          <span className="h-4 w-px bg-line" aria-hidden />
          <IconButton icon={Minus} label="Zoom out" size="sm" onClick={() => zoom(1 / 1.2)} />
          <span className="w-11 text-center tabular-nums">{Math.round(scale * 100)}%</span>
          <IconButton icon={Plus} label="Zoom in" size="sm" onClick={() => zoom(1.2)} />
          <IconButton
            icon={Maximize2}
            label="Fit the width"
            size="sm"
            onClick={() => {
              if (viewer.current) viewer.current.currentScaleValue = "page-width";
            }}
          />
        </div>
      )}
    </div>
  );
}
