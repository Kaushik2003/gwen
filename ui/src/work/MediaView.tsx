import { ExternalLink, FileWarning, History as HistoryIcon, LoaderCircle } from "lucide-react";
import { lazy, Suspense, useEffect, useState } from "react";
import { apiError } from "../api";
import { Button, Callout, Empty, IconButton, cx } from "../components/ui";
import PathCrumbs from "./PathCrumbs";
import { rawUrl } from "./raw";
import { baseName, git } from "./repo";

// A file the tab shows rather than edits (plan.md 3.15, board 5): a PDF in PDF.js, an image at fit or full
// size. It reads the bytes from the window's file route (cmd/gwen-ui/rawfile.go) and, like the editor,
// follows the file on disk while its tab is in front.

const PdfView = lazy(() => import("./PdfView"));

const POLL_MS = 1500;

export default function MediaView({ folder, path, kind, active = true, onOpenHistory }: { folder: string; path: string; kind: "pdf" | "image"; active?: boolean; onOpenHistory?: () => void }) {
  const [stamp, setStamp] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [openError, setOpenError] = useState("");
  const [info, setInfo] = useState("");

  // The stamp names the file's version in its URL: a change on disk loads it again.
  useEffect(() => {
    let live = true;
    const look = () =>
      git.stamp(folder, path).then(
        (s) => {
          if (!live) return;
          setError("");
          setStamp((old) => (old === s ? old : s));
        },
        (e) => live && setError(apiError(e).message),
      );
    void look();
    if (!active) return () => void (live = false);
    const t = setInterval(look, POLL_MS);
    return () => {
      live = false;
      clearInterval(t);
    };
  }, [folder, path, active]);

  const openInApp = () => {
    setOpenError("");
    git.open(folder, path).catch((e) => setOpenError(apiError(e).message));
  };

  const url = stamp === null ? "" : rawUrl(folder, path, stamp);

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex min-h-[42px] flex-wrap items-center gap-x-2.5 gap-y-1.5 border-b border-chrome-line bg-chrome py-1.5 pr-3.5 pl-[18px]">
        <PathCrumbs folder={folder} path={path} />
        {info && <span className="text-xs whitespace-nowrap text-ink-subtle">{info}</span>}
        <span className="flex-1" />
        {onOpenHistory && (
          <Button size="sm" icon={HistoryIcon} onClick={onOpenHistory}>
            History
          </Button>
        )}
        <IconButton icon={ExternalLink} label="Open in its app" size="sm" onClick={openInApp} />
      </div>
      {openError && (
        <Callout tone="danger" className="m-3 mb-0">
          Couldn't open the file: {openError}
        </Callout>
      )}
      {error ? (
        <Empty icon={FileWarning} title="Can't show this file" className="m-5" action={onOpenHistory && <Button size="sm" icon={HistoryIcon} onClick={onOpenHistory}>Open history</Button>}>
          {error}
        </Empty>
      ) : !url ? (
        <Loading text={`Reading ${baseName(path)}…`} />
      ) : kind === "pdf" ? (
        <Suspense fallback={<Loading text="Opening the PDF viewer…" />}>
          <PdfView url={url} onInfo={(n) => setInfo(`PDF · ${n.toLocaleString("en-US")} ${n === 1 ? "page" : "pages"}`)} />
        </Suspense>
      ) : (
        <ImageView url={url} path={path} onInfo={setInfo} />
      )}
    </div>
  );
}

function Loading({ text }: { text: string }) {
  return (
    <p className="flex items-center gap-2 p-5 text-[13px] text-ink-subtle">
      <LoaderCircle size={14} className="animate-spin" aria-hidden />
      {text}
    </p>
  );
}

/** An image fitted to the tab; a click shows it at full size, scrolling, and back. */
function ImageView({ url, path, onInfo }: { url: string; path: string; onInfo: (s: string) => void }) {
  const [full, setFull] = useState(false);
  const [failed, setFailed] = useState(false);
  useEffect(() => setFailed(false), [url]);
  const ext = path.slice(path.lastIndexOf(".") + 1).toUpperCase();
  if (failed)
    return (
      <Empty icon={FileWarning} title="Can't show this image" className="m-5">
        The window couldn't decode {baseName(path)}. Open it in its app instead.
      </Empty>
    );
  return (
    <div className={cx("preview-checker min-h-0 flex-1 overflow-auto", !full && "flex items-center justify-center p-6")}>
      <img
        src={url}
        alt={baseName(path)}
        onLoad={(e) => onInfo(`${ext} · ${e.currentTarget.naturalWidth} × ${e.currentTarget.naturalHeight}`)}
        onError={() => setFailed(true)}
        onClick={() => setFull((f) => !f)}
        title={full ? "Click to fit" : "Click for full size"}
        className={cx(full ? "max-w-none cursor-zoom-out" : "max-h-full max-w-full cursor-zoom-in object-contain")}
        style={{ imageRendering: full ? "pixelated" : undefined }}
      />
    </div>
  );
}
