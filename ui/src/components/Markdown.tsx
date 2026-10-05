import type { ReactNode } from "react";

/**
 * Renders a retro's Markdown as plain paragraphs, headings, and lists. Text
 * is never parsed as HTML, so a reply cannot inject markup.
 */
export default function Markdown({ text }: { text: string }) {
  const blocks: ReactNode[] = [];
  let list: string[] = [];
  let para: string[] = [];
  const flush = () => {
    if (para.length) {
      blocks.push(
        <p key={blocks.length} className="text-ink-muted">
          {para.join(" ")}
        </p>,
      );
      para = [];
    }
    if (list.length) {
      blocks.push(
        <ul key={blocks.length} className="flex list-disc flex-col gap-1 pl-5 text-ink-muted marker:text-accent">
          {list.map((item, i) => (
            <li key={i}>{item}</li>
          ))}
        </ul>,
      );
      list = [];
    }
  };
  for (const raw of text.split("\n")) {
    const line = raw.trim();
    const heading = /^(#{1,6})\s+(.*)$/.exec(line);
    const item = /^[-*]\s+(.*)$/.exec(line) ?? /^\d+[.)]\s+(.*)$/.exec(line);
    if (!line) {
      flush();
    } else if (heading) {
      flush();
      blocks.push(
        <h3 key={blocks.length} className="mt-2 text-[15px] font-semibold tracking-[-0.01em] text-ink first:mt-0">
          {strip(heading[2])}
        </h3>,
      );
    } else if (item) {
      if (para.length) flush();
      list.push(strip(item[1]));
    } else {
      if (list.length) flush();
      para.push(strip(line));
    }
  }
  flush();
  return <div className="flex max-w-[68ch] flex-col gap-3 text-sm leading-relaxed">{blocks}</div>;
}

/** Drops emphasis markers, which plain text shows as noise. */
function strip(s: string): string {
  return s
    .replace(/\*\*(.+?)\*\*/g, "$1")
    .replace(/__(.+?)__/g, "$1")
    .replace(/\*([^*\s][^*]*?)\*/g, "$1")
    .replace(/(^|\W)_([^_\s][^_]*?)_(?=\W|$)/g, "$1$2")
    .replace(/`([^`]+)`/g, "$1");
}
