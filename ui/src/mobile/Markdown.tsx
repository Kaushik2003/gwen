import { Fragment, type ReactNode } from "react";

// A small Markdown renderer for what Claude writes: paragraphs, headings, lists, quotes, tables, fenced code, and inline code, bold,
// italics and links. It builds React elements only, so nothing Claude writes is ever treated as HTML.

function inline(text: string, key = 0): ReactNode[] {
  const out: ReactNode[] = [];
  const re = /(`[^`\n]+`)|(\*\*[^*\n]+\*\*)|(\[[^\]\n]+\]\(https?:\/\/[^\s)]+\))|(\*[^*\s][^*\n]*\*)/g;
  let last = 0;
  for (let m = re.exec(text); m; m = re.exec(text)) {
    if (m.index > last) out.push(text.slice(last, m.index));
    const s = m[0];
    const k = `${key}-${m.index}`;
    if (m[1]) out.push(<code key={k}>{s.slice(1, -1)}</code>);
    else if (m[2]) out.push(<strong key={k}>{inline(s.slice(2, -2), m.index)}</strong>);
    else if (m[3]) {
      const split = s.indexOf("](");
      out.push(<a key={k} href={s.slice(split + 2, -1)} target="_blank" rel="noreferrer noopener">{s.slice(1, split)}</a>);
    } else out.push(<em key={k}>{s.slice(1, -1)}</em>);
    last = m.index + s.length;
  }
  if (last < text.length) out.push(text.slice(last));
  return out;
}

const isRow = (l: string) => /^\s*\|.*\|\s*$/.test(l);
const cells = (l: string) => l.trim().replace(/^\||\|$/g, "").split("|").map((c) => c.trim());

export function Markdown({ text }: { text: string }) {
  const lines = text.replace(/\r\n/g, "\n").split("\n");
  const blocks: ReactNode[] = [];
  let i = 0;
  const key = () => blocks.length;
  while (i < lines.length) {
    const line = lines[i];
    if (!line.trim()) {
      i++;
      continue;
    }
    const fence = line.match(/^\s*(```|~~~)/);
    if (fence) {
      const body: string[] = [];
      for (i++; i < lines.length && !lines[i].trimStart().startsWith(fence[1]); i++) body.push(lines[i]);
      i++;
      blocks.push(<pre key={key()}><code>{body.join("\n")}</code></pre>);
      continue;
    }
    const heading = line.match(/^(#{1,6})\s+(.*)$/);
    if (heading) {
      blocks.push(<h3 key={key()}>{inline(heading[2])}</h3>);
      i++;
      continue;
    }
    if (isRow(line) && i + 1 < lines.length && /^\s*\|?[\s:|-]+\|?\s*$/.test(lines[i + 1]) && lines[i + 1].includes("-")) {
      const head = cells(line);
      const rows: string[][] = [];
      for (i += 2; i < lines.length && isRow(lines[i]); i++) rows.push(cells(lines[i]));
      blocks.push(
        <table key={key()}>
          <thead><tr>{head.map((c, n) => <th key={n}>{inline(c)}</th>)}</tr></thead>
          <tbody>{rows.map((r, n) => <tr key={n}>{r.map((c, m) => <td key={m}>{inline(c)}</td>)}</tr>)}</tbody>
        </table>,
      );
      continue;
    }
    if (/^\s*>/.test(line)) {
      const body: string[] = [];
      for (; i < lines.length && /^\s*>/.test(lines[i]); i++) body.push(lines[i].replace(/^\s*>\s?/, ""));
      blocks.push(<blockquote key={key()}>{inline(body.join(" "))}</blockquote>);
      continue;
    }
    const bullet = /^\s*([-*+]|\d+[.)])\s+/;
    if (bullet.test(line)) {
      const ordered = /^\s*\d/.test(line);
      const items: string[] = [];
      for (; i < lines.length && (bullet.test(lines[i]) || (/^\s{2,}\S/.test(lines[i]) && items.length > 0)); i++) {
        if (bullet.test(lines[i])) items.push(lines[i].replace(bullet, ""));
        else items[items.length - 1] += " " + lines[i].trim();
      }
      const list = items.map((it, n) => <li key={n}>{inline(it)}</li>);
      blocks.push(ordered ? <ol key={key()}>{list}</ol> : <ul key={key()}>{list}</ul>);
      continue;
    }
    const para: string[] = [];
    for (; i < lines.length && lines[i].trim() && !/^\s*(```|~~~|#{1,6}\s|>)/.test(lines[i]) && !bullet.test(lines[i]); i++) para.push(lines[i]);
    blocks.push(<p key={key()}>{para.map((l, n) => <Fragment key={n}>{n > 0 && <br />}{inline(l, n)}</Fragment>)}</p>);
  }
  return <div className="md">{blocks}</div>;
}
