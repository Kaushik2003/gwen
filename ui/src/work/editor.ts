import { autocompletion, closeBrackets, closeBracketsKeymap, completionKeymap } from "@codemirror/autocomplete";
import { defaultKeymap, history, historyKeymap, indentWithTab } from "@codemirror/commands";
import { cpp } from "@codemirror/lang-cpp";
import { css } from "@codemirror/lang-css";
import { go } from "@codemirror/lang-go";
import { html } from "@codemirror/lang-html";
import { java } from "@codemirror/lang-java";
import { javascript } from "@codemirror/lang-javascript";
import { json } from "@codemirror/lang-json";
import { markdown } from "@codemirror/lang-markdown";
import { python } from "@codemirror/lang-python";
import { rust } from "@codemirror/lang-rust";
import { sql } from "@codemirror/lang-sql";
import { xml } from "@codemirror/lang-xml";
import { yaml } from "@codemirror/lang-yaml";
import { bracketMatching, foldGutter, foldKeymap, HighlightStyle, indentOnInput, indentUnit, StreamLanguage, syntaxHighlighting } from "@codemirror/language";
import { kotlin } from "@codemirror/legacy-modes/mode/clike";
import { diff } from "@codemirror/legacy-modes/mode/diff";
import { dockerFile } from "@codemirror/legacy-modes/mode/dockerfile";
import { lua } from "@codemirror/legacy-modes/mode/lua";
import { properties } from "@codemirror/legacy-modes/mode/properties";
import { protobuf } from "@codemirror/legacy-modes/mode/protobuf";
import { ruby } from "@codemirror/legacy-modes/mode/ruby";
import { shell } from "@codemirror/legacy-modes/mode/shell";
import { swift } from "@codemirror/legacy-modes/mode/swift";
import { toml } from "@codemirror/legacy-modes/mode/toml";
import { highlightSelectionMatches, searchKeymap } from "@codemirror/search";
import { Compartment, EditorState, RangeSet, StateEffect, StateField, type Extension } from "@codemirror/state";
import {
  crosshairCursor,
  Decoration,
  drawSelection,
  dropCursor,
  EditorView,
  gutter,
  GutterMarker,
  highlightActiveLine,
  highlightActiveLineGutter,
  highlightSpecialChars,
  keymap,
  lineNumbers,
  rectangularSelection,
  WidgetType,
  type DecorationSet,
  type ViewUpdate,
} from "@codemirror/view";
import { tags as t } from "@lezer/highlight";

// The file tab's editor (plan.md 3.15, board 5): CodeMirror 6 set up the way VS Code feels (line numbers,
// folding, multiple cursors, bracket matching and closing, search and replace, completion, Tab indents),
// coloured from Gwen's theme tokens, with the gutter marks and blame the read-only viewer had.

/** The editor for each internal/repo Lang name. */
function language(lang: string): Extension {
  const legacy = (p: Parameters<typeof StreamLanguage.define>[0]) => StreamLanguage.define(p);
  switch (lang) {
    case "go":
      return go();
    case "ts":
      return javascript({ typescript: true });
    case "tsx":
      return javascript({ typescript: true, jsx: true });
    case "js":
      return javascript();
    case "jsx":
      return javascript({ jsx: true });
    case "json":
      return json();
    case "md":
      return markdown();
    case "css":
      return css();
    case "html":
      return html();
    case "xml":
      return xml();
    case "py":
      return python();
    case "rs":
      return rust();
    case "yaml":
      return yaml();
    case "sql":
      return sql();
    case "c":
    case "cpp":
      return cpp();
    case "java":
      return java();
    case "sh":
    case "make":
    case "ignore":
      return legacy(shell);
    case "toml":
      return legacy(toml);
    case "kotlin":
      return legacy(kotlin);
    case "swift":
      return legacy(swift);
    case "ruby":
      return legacy(ruby);
    case "lua":
      return legacy(lua);
    case "proto":
      return legacy(protobuf);
    case "ini":
      return legacy(properties);
    case "docker":
      return legacy(dockerFile);
    case "diff":
      return legacy(diff);
  }
  return [];
}

/** The text's indent: a tab when most indented lines start with one, else the commonest step of 2, 4 or 8 spaces. */
export function detectIndent(text: string, lang: string): string {
  if (lang === "go" || lang === "make") return "\t";
  let tabs = 0;
  let spaces = 0;
  const steps = new Map<number, number>();
  let prev = 0;
  let seen = 0;
  for (const line of text.split("\n", 5000)) {
    if (!line.trim()) continue;
    if (line[0] === "\t") tabs++;
    const n = line.length - line.trimStart().length;
    if (line[0] === " ") spaces++;
    if (line[0] !== "\t") {
      const d = Math.abs(n - prev);
      if (d === 2 || d === 4 || d === 8) steps.set(d, (steps.get(d) ?? 0) + 1);
      prev = n;
    }
    if (++seen > 2000) break;
  }
  if (tabs > spaces) return "\t";
  let best = 4;
  let most = 0;
  for (const [d, c] of steps) if (c > most) [best, most] = [d, c];
  return " ".repeat(best);
}

// ---------------------------------------------------------------------------------------------
// Gutter marks: the uncommitted lines (or the last commit's), moved along as you type.

export type MarkKind = "add" | "mod" | "del";

class Mark extends GutterMarker {
  constructor(
    readonly kind: MarkKind,
    readonly top = false,
  ) {
    super();
  }
  eq(o: GutterMarker) {
    return o instanceof Mark && o.kind === this.kind && o.top === this.top;
  }
  toDOM() {
    const el = document.createElement("div");
    el.className = `cm-mark cm-mark-${this.kind}${this.top ? " cm-mark-top" : ""}`;
    el.title = this.kind === "add" ? "Added" : this.kind === "mod" ? "Changed" : "Lines deleted here";
    return el;
  }
}

/** Sets the marks: line numbers of the text as last read; a del on line n is below it, on line 0 above line 1. */
export const setMarks = StateEffect.define<{ line: number; kind: string }[]>();

const marksField = StateField.define<RangeSet<GutterMarker>>({
  create: () => RangeSet.empty,
  update(set, tr) {
    set = set.map(tr.changes);
    for (const e of tr.effects) {
      if (!e.is(setMarks)) continue;
      const doc = tr.state.doc;
      const ranges = [];
      for (const m of e.value) {
        if (m.kind !== "add" && m.kind !== "mod" && m.kind !== "del") continue;
        const n = Math.min(doc.lines, Math.max(1, m.line));
        ranges.push(new Mark(m.kind, m.kind === "del" && m.line === 0).range(doc.line(n).from));
      }
      set = RangeSet.of(ranges, true);
    }
    return set;
  },
});

/** The marked lines where they are now, for the strip at the right edge. */
export function markedLines(state: EditorState): { line: number; kind: MarkKind }[] {
  const out: { line: number; kind: MarkKind }[] = [];
  state.field(marksField).between(0, state.doc.length, (from, _to, m) => {
    out.push({ line: state.doc.lineAt(from).number, kind: (m as Mark).kind });
  });
  return out;
}

// ---------------------------------------------------------------------------------------------
// Blame: who last changed the cursor's line, after its text.

class BlameNote extends WidgetType {
  constructor(readonly text: string) {
    super();
  }
  eq(o: BlameNote) {
    return o.text === this.text;
  }
  toDOM() {
    const el = document.createElement("span");
    el.className = "cm-blame";
    el.setAttribute("aria-live", "polite");
    el.textContent = this.text;
    return el;
  }
  ignoreEvent() {
    return false;
  }
}

export const setBlame = StateEffect.define<{ line: number; text: string } | null>();

const blameField = StateField.define<DecorationSet>({
  create: () => Decoration.none,
  update(deco, tr) {
    if (tr.docChanged) deco = Decoration.none;
    for (const e of tr.effects) {
      if (!e.is(setBlame)) continue;
      const v = e.value;
      deco = v && v.line <= tr.state.doc.lines ? Decoration.set([Decoration.widget({ widget: new BlameNote(v.text), side: 1 }).range(tr.state.doc.line(v.line).to)]) : Decoration.none;
    }
    return deco;
  },
  provide: (f) => EditorView.decorations.from(f),
});

// ---------------------------------------------------------------------------------------------
// Looks: Gwen's tokens, so every theme (and a theme change) applies without rebuilding the editor.

const mix = (color: string, pct: number) => `color-mix(in srgb, var(${color}) ${pct}%, transparent)`;

const look = EditorView.theme(
  {
    "&": { height: "100%", backgroundColor: "var(--color-canvas)", color: mix("--color-ink", 85), fontSize: "12.5px" },
    "&.cm-focused": { outline: "none" },
    ".cm-scroller": { fontFamily: "var(--font-mono)", lineHeight: "20px", tabSize: "4" },
    ".cm-content": { padding: "10px 0", caretColor: "var(--color-accent-focus)" },
    ".cm-cursor, .cm-dropCursor": { borderLeft: "2px solid var(--color-accent-focus)" },
    ".cm-selectionBackground": { backgroundColor: `${mix("--color-info", 22)} !important` },
    "&.cm-focused .cm-selectionBackground": { backgroundColor: `${mix("--color-info", 32)} !important` },
    ".cm-selectionMatch": { backgroundColor: mix("--color-ink", 10) },
    ".cm-activeLine": { backgroundColor: "var(--color-surface-1)" },
    ".cm-gutters": { backgroundColor: "var(--color-canvas)", color: "var(--color-ink-faint)", border: "none" },
    ".cm-activeLineGutter": { backgroundColor: "var(--color-surface-1)", color: "var(--color-ink)" },
    ".cm-lineNumbers .cm-gutterElement": { padding: "0 10px 0 8px", minWidth: "36px" },
    ".cm-foldGutter .cm-gutterElement": { color: "var(--color-ink-faint)", padding: "0 4px", cursor: "pointer" },
    ".cm-foldGutter .cm-gutterElement:hover": { color: "var(--color-ink)" },
    ".cm-foldPlaceholder": { backgroundColor: "var(--color-surface-2)", border: "1px solid var(--color-line)", color: "var(--color-ink-subtle)", padding: "0 6px", borderRadius: "4px" },
    "&.cm-focused .cm-matchingBracket": { backgroundColor: mix("--color-ink", 14), outline: `1px solid ${mix("--color-ink", 30)}` },
    "&.cm-focused .cm-nonmatchingBracket": { backgroundColor: mix("--color-danger", 25) },
    ".cm-searchMatch": { backgroundColor: mix("--color-warning", 22), outline: `1px solid ${mix("--color-warning", 45)}` },
    ".cm-searchMatch.cm-searchMatch-selected": { backgroundColor: mix("--color-warning", 45) },
    ".cm-tooltip": { backgroundColor: "var(--color-surface-2)", border: "1px solid var(--color-line-strong)", borderRadius: "8px", color: "var(--color-ink)", overflow: "hidden" },
    ".cm-tooltip-autocomplete > ul": { fontFamily: "var(--font-mono)", maxHeight: "16em" },
    ".cm-tooltip-autocomplete > ul > li": { padding: "2px 10px !important" },
    ".cm-tooltip-autocomplete > ul > li[aria-selected]": { backgroundColor: "var(--color-accent-soft)", color: "var(--color-ink)" },
    ".cm-completionMatchedText": { textDecoration: "none", color: "var(--color-accent-focus)", fontWeight: "600" },
    ".cm-panels": { backgroundColor: "var(--color-chrome)", color: "var(--color-ink)" },
    ".cm-panels-top": { borderBottom: "1px solid var(--color-chrome-line)" },
    ".cm-panels-bottom": { borderTop: "1px solid var(--color-chrome-line)" },
    ".cm-panel.cm-search": { padding: "6px 10px", fontFamily: "var(--font-sans)", fontSize: "12.5px" },
    ".cm-panel.cm-search input, .cm-panel.cm-search button": { fontFamily: "inherit", fontSize: "12.5px", borderRadius: "6px" },
    ".cm-textfield": { backgroundColor: "var(--color-surface-1)", border: "1px solid var(--color-line-strong)", color: "var(--color-ink)", padding: "3px 8px" },
    ".cm-textfield:focus": { outline: "none", borderColor: "var(--color-accent)" },
    ".cm-button": { backgroundImage: "none", backgroundColor: "var(--color-surface-2)", border: "1px solid var(--color-line-strong)", color: "var(--color-ink)", padding: "3px 10px" },
    ".cm-button:hover": { backgroundColor: "var(--color-surface-3)" },
    ".cm-panel.cm-search label": { color: "var(--color-ink-muted)" },
    ".cm-panel.cm-search [name=close]": { color: "var(--color-ink-subtle)", fontSize: "16px" },
    ".cm-specialChar": { color: "var(--color-danger)" },
    // The marks gutter: a bar for added and changed lines, a wedge where lines were deleted.
    ".cm-marks .cm-gutterElement": { position: "relative", width: "6px" },
    ".cm-mark": { position: "absolute", left: "2px", top: "0", bottom: "0", width: "3px" },
    ".cm-mark-add": { backgroundColor: "var(--color-success)" },
    ".cm-mark-mod": { backgroundColor: "var(--color-warning)" },
    ".cm-mark-del": {
      top: "auto",
      bottom: "-4px",
      left: "0",
      width: "0",
      height: "0",
      borderTop: "4px solid transparent",
      borderBottom: "4px solid transparent",
      borderLeft: "6px solid var(--color-danger)",
      zIndex: "2",
    },
    ".cm-mark-del.cm-mark-top": { top: "-4px", bottom: "auto" },
    ".cm-blame": { paddingLeft: "28px", fontFamily: "var(--font-sans)", fontSize: "12px", fontStyle: "italic", color: "var(--color-ink-faint)" },
  },
  { dark: true },
);

/** Token colours, from the approved mockup. */
const colours = HighlightStyle.define([
  { tag: [t.keyword, t.controlKeyword, t.moduleKeyword, t.operatorKeyword, t.definitionKeyword, t.modifier, t.tagName], color: "#ee8496" },
  { tag: [t.typeName, t.className, t.namespace, t.attributeName, t.labelName], color: "#7cc4f2" },
  { tag: [t.function(t.variableName), t.function(t.propertyName), t.macroName], color: "#86e2d6" },
  { tag: [t.string, t.special(t.string), t.regexp, t.character, t.attributeValue], color: "#f2c58a" },
  { tag: [t.number, t.bool, t.null, t.atom, t.constant(t.variableName), t.escape], color: "#f6c29a" },
  { tag: [t.comment, t.lineComment, t.blockComment, t.docComment], color: "#958a88", fontStyle: "italic" },
  { tag: [t.meta, t.processingInstruction, t.annotation], color: "var(--color-ink-subtle)" },
  { tag: t.heading, color: "var(--color-ink)", fontWeight: "700" },
  { tag: [t.link, t.url], color: "#7cc4f2", textDecoration: "underline" },
  { tag: t.emphasis, fontStyle: "italic" },
  { tag: t.strong, fontWeight: "700" },
  { tag: t.strikethrough, textDecoration: "line-through" },
  { tag: t.inserted, color: "var(--color-success)" },
  { tag: t.deleted, color: "var(--color-danger)" },
  { tag: t.invalid, color: "var(--color-danger)" },
]);

// ---------------------------------------------------------------------------------------------

/** Turned on and off as the file allows: editing, and the line separator the file uses. */
export const editable = new Compartment();

export interface EditorHooks {
  save: () => void;
  update: (u: ViewUpdate) => void;
}

export interface EditorFile {
  text: string;
  lang: string;
  /** Editable: the whole file, valid UTF-8. */
  canEdit: boolean;
}

/** The file's line separator: CRLF when its first line ends with one, so a save keeps it. */
export const separatorOf = (text: string) => {
  const nl = text.indexOf("\n");
  return nl > 0 && text[nl - 1] === "\r" ? "\r\n" : "\n";
};

export const editableExt = (on: boolean): Extension => [EditorState.readOnly.of(!on), EditorView.editable.of(true)];

export function editorState(f: EditorFile, hooks: { current: EditorHooks }): EditorState {
  const sep = separatorOf(f.text);
  return EditorState.create({
    doc: f.text,
    extensions: [
      EditorState.lineSeparator.of(sep),
      editable.of(editableExt(f.canEdit)),
      indentUnit.of(detectIndent(f.text, f.lang)),
      language(f.lang),
      gutter({ class: "cm-marks", markers: (v) => v.state.field(marksField) }),
      marksField,
      lineNumbers(),
      foldGutter({ openText: "⌄", closedText: "›" }),
      highlightActiveLineGutter(),
      highlightSpecialChars(),
      history(),
      drawSelection(),
      dropCursor(),
      EditorState.allowMultipleSelections.of(true),
      indentOnInput(),
      syntaxHighlighting(colours),
      bracketMatching(),
      closeBrackets(),
      autocompletion(),
      rectangularSelection(),
      crosshairCursor(),
      highlightActiveLine(),
      highlightSelectionMatches(),
      blameField,
      look,
      keymap.of([
        { key: "Mod-s", preventDefault: true, run: () => (hooks.current.save(), true) },
        ...closeBracketsKeymap,
        ...defaultKeymap,
        ...searchKeymap,
        ...historyKeymap,
        ...foldKeymap,
        ...completionKeymap,
        indentWithTab,
      ]),
      EditorView.updateListener.of((u) => hooks.current.update(u)),
    ],
  });
}

/** The smallest change that turns before into after: what differs between their common start and end. */
export function minimalChange(before: string, after: string): { from: number; to: number; insert: string } {
  const max = Math.min(before.length, after.length);
  let a = 0;
  while (a < max && before.charCodeAt(a) === after.charCodeAt(a)) a++;
  let b = 0;
  while (b < max - a && before.charCodeAt(before.length - 1 - b) === after.charCodeAt(after.length - 1 - b)) b++;
  return { from: a, to: before.length - b, insert: after.slice(a, after.length - b) };
}
