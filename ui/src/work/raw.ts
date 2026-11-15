// What the file tab shows a file as, and where the window reads a file's bytes (cmd/gwen-ui/rawfile.go):
// PDFs and images are shown, not edited; Markdown is text with a rendered preview beside the editor.

export type ViewerKind = "pdf" | "image" | "text";

const imageExts = new Set(["png", "jpg", "jpeg", "gif", "webp", "avif", "bmp", "ico"]);

const extOf = (path: string) => {
  const base = path.slice(path.lastIndexOf("/") + 1).toLowerCase();
  const dot = base.lastIndexOf(".");
  return dot > 0 ? base.slice(dot + 1) : "";
};

/** How the file tab shows a file. SVG stays text (it is XML) with a preview, like Markdown. */
export function viewerKind(path: string): ViewerKind {
  const ext = extOf(path);
  if (ext === "pdf") return "pdf";
  if (imageExts.has(ext)) return "image";
  return "text";
}

/** Files whose text has a rendered preview beside the editor. */
export const previewKind = (path: string): "markdown" | "svg" | null => {
  const ext = extOf(path);
  if (ext === "md" || ext === "markdown" || ext === "mdx") return "markdown";
  if (ext === "svg") return "svg";
  return null;
};

/** The URL of a project file's bytes; v (the file's stamp) changes it when the file does. */
export function rawUrl(folder: string, path: string, v = ""): string {
  const q = new URLSearchParams({ dir: folder, path });
  if (v) q.set("v", v);
  return `/_gwen/file?${q}`;
}

/**
 * Resolves a link in a file to a path in the project, as GitHub does: relative to the file's folder, or to the
 * project's folder when it starts with "/". null for a URL with a scheme, or one that leaves the project.
 */
export function linkedPath(fromFile: string, href: string): string | null {
  if (/^[a-z][a-z0-9+.-]*:/i.test(href) || href.startsWith("//")) return null;
  const clean = href.replace(/[?#].*$/, "");
  if (!clean) return null;
  const out = clean.startsWith("/") ? [] : fromFile.split("/").slice(0, -1);
  for (const raw of clean.split("/")) {
    let part = raw;
    try {
      part = decodeURIComponent(raw);
    } catch {
      // keep it as written
    }
    if (!part || part === ".") continue;
    if (part === "..") {
      if (!out.length) return null;
      out.pop();
    } else out.push(part);
  }
  return out.length ? out.join("/") : null;
}
