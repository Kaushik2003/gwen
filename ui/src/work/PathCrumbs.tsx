import { Fragment } from "react";
import { cx } from "../components/ui";
import { baseName } from "./repo";

/** A file tab's path: the project's folder, then each part, the file's name in full. */
export default function PathCrumbs({ folder, path, dirty }: { folder: string; path: string; dirty?: boolean }) {
  const parts = path.split("/");
  return (
    <nav aria-label="Path" className="flex min-w-0 items-center gap-1.5 text-[12.5px] text-ink-muted">
      <span className="shrink-0">{baseName(folder)}</span>
      {parts.map((p, i) => (
        <Fragment key={i}>
          <span aria-hidden className="text-ink-faint">
            /
          </span>
          <span className={cx(i === parts.length - 1 ? "font-medium text-ink" : "truncate")} aria-current={i === parts.length - 1 ? "page" : undefined}>
            {p}
          </span>
        </Fragment>
      ))}
      {dirty && (
        <span className="text-ink-muted" title="Unsaved changes" aria-label="Unsaved changes">
          ●
        </span>
      )}
    </nav>
  );
}
