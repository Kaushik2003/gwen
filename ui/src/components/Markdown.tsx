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
      blocks.push(<p key={blocks.length}>{para.join(" ")}</p>);
      para = [];
    }
    if (list.length) {
      blocks.push(
        <ul key={blocks.length} className="list-disc pl-5">
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
        <h3 key={blocks.length} className="font-semibold">
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
  return <div className="flex flex-col gap-2 text-sm">{blocks}</div>;
}

/** Drops emphasis markers, which plain text shows as noise. */
function strip(s: string): string {
  return s.replace(/\*\*(.+?)\*\*/g, "$1").replace(/__(.+?)__/g, "$1").replace(/`([^`]+)`/g, "$1");
}
