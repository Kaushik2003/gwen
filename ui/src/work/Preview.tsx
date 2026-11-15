import DOMPurify from "dompurify";
import { Marked } from "marked";
import { useEffect, useMemo, useRef, type MouseEvent } from "react";
import { BrowserOpenURL } from "../wailsjs/runtime/runtime";
import { linkedPath, rawUrl } from "./raw";

// The rendered side of a text file's tab (FileView): Markdown as a page, SVG as a picture. It renders the
// editor's text, unsaved edits included. Markdown goes through DOMPurify, so a file's HTML can't run script
// in the window; its images load from the project (rawUrl), its links to other files open them in Gwen, and
// web links open in the browser.

const md = new Marked({ gfm: true, breaks: false, async: false });

/** GitHub's heading anchors: lower case, punctuation dropped, spaces to "-", repeats numbered. */
function slugger() {
  const seen = new Map<string, number>();
  return (text: string) => {
    const base =
      text
        .trim()
        .toLowerCase()
        .replace(/[^\p{L}\p{N}\s_-]/gu, "")
        .replace(/\s/g, "-") || "section";
    const n = seen.get(base) ?? 0;
    seen.set(base, n + 1);
    return n ? `${base}-${n}` : base;
  };
}

function renderMarkdown(text: string, folder: string, path: string): DocumentFragment {
  const html = DOMPurify.sanitize(md.parse(text) as string, { ADD_ATTR: ["target"] });
  // A template's content is inert: images don't load until their src points into the project.
  const tpl = document.createElement("template");
  tpl.innerHTML = html;
  for (const img of tpl.content.querySelectorAll<HTMLImageElement>("img[src]")) {
    const p = linkedPath(path, img.getAttribute("src") ?? "");
    if (p) img.setAttribute("src", rawUrl(folder, p));
    img.loading = "lazy";
  }
  const slug = slugger();
  for (const h of tpl.content.querySelectorAll<HTMLElement>("h1, h2, h3, h4, h5, h6")) if (!h.id) h.id = slug(h.textContent ?? "");
  return tpl.content;
}

export default function Preview({ kind, text, folder, path, onOpenFile }: { kind: "markdown" | "svg"; text: string; folder: string; path: string; onOpenFile?: (path: string) => void }) {
  const page = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (kind !== "markdown" || !page.current) return;
    page.current.replaceChildren(renderMarkdown(text, folder, path));
  }, [kind, text, folder, path]);

  const svg = useMemo(() => (kind === "svg" ? `data:image/svg+xml;charset=utf-8,${encodeURIComponent(text)}` : ""), [kind, text]);

  const onClick = (e: MouseEvent) => {
    const a = (e.target as HTMLElement).closest("a[href]");
    if (!a) return;
    e.preventDefault();
    const href = a.getAttribute("href") ?? "";
    if (href.startsWith("#")) {
      const id = decodeURIComponent(href.slice(1));
      page.current?.querySelector(`[id="${CSS.escape(id)}"]`)?.scrollIntoView({ block: "start" });
      return;
    }
    if (/^(https?|mailto):/i.test(href)) {
      BrowserOpenURL(href);
      return;
    }
    const p = linkedPath(path, href);
    if (p) onOpenFile?.(p);
  };

  if (kind === "svg")
    return (
      <div className="preview-checker flex h-full items-center justify-center overflow-auto p-6">
        <img src={svg} alt={path} className="max-h-full max-w-full" />
      </div>
    );
  return (
    <div className="h-full overflow-auto">
      <article ref={page} onClick={onClick} className="md-page mx-auto max-w-[820px] px-10 pt-8 pb-24" />
    </div>
  );
}
