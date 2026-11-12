// What the Memory screen, Settings › Memory, and the chat's memory chips share:
// claims and refs in words, and receipts.

/** The nav param that opens a chat on the Assistant screen; any other param is a message to send. */
export function threadParam(id) {
  return `thread:${id}`;
}

/** The chat a nav param opens, or null when it is a message instead. */
export function threadOf(param) {
  return param?.startsWith("thread:") ? param.slice("thread:".length) : null;
}

/** The claim id of a ref such as "claim:<id>", or null for any other ref. */
export function claimIdOf(ref) {
  return ref.startsWith("claim:") ? ref.slice("claim:".length) : null;
}

/** Where a claim came from, in words: who said or set it. */
export function trustLabel(trust, name) {
  switch (trust) {
    case "owner_edit":
      return "You set it";
    case "owner_said":
      return "You said it";
    case "observed":
      return `${name} noticed`;
    case "inferred":
      return `${name} inferred`;
    case "external":
      return "From a document";
  }
  return "";
}

/** What a trust level means, for the badge's tooltip. */
export function trustMeans(trust, name) {
  switch (trust) {
    case "owner_edit":
      return "You typed it in yourself.";
    case "owner_said":
      return "You said it in a chat.";
    case "observed":
      return `${name} saw it in what you track.`;
    case "inferred":
      return `${name} worked it out; you never said it.`;
    case "external":
      return "It came from a document you gave her.";
  }
  return "";
}

const months = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

/** "Oct 2026". */
export function monthYear(ms) {
  const d = new Date(ms);
  return `${months[d.getMonth()]} ${d.getFullYear()}`;
}

/** "12 Oct 2026". */
export function shortDate(ms) {
  const d = new Date(ms);
  return `${d.getDate()} ${months[d.getMonth()]} ${d.getFullYear()}`;
}

/** "since Oct 2026", or "Oct 2025 to Oct 2026" once it stopped being true. */
export function whenText(c) {
  const from = c.valid_from ?? c.mentioned_at ?? c.recorded_at;
  if (c.valid_to != null) return c.valid_from != null ? `${monthYear(c.valid_from)} to ${monthYear(c.valid_to)}` : `until ${monthYear(c.valid_to)}`;
  return from ? `since ${monthYear(from)}` : "";
}

/** "Works at Acme, Oct 2025 to Oct 2026": a past value for a claim's history. */
export function historyLine(c) {
  const when = whenText(c);
  return when ? `${c.text}, ${when}` : c.text;
}

/** The keys that hold one value at a time (memory.Keys with CardinalityOne). */
const oneValued = new Set([
  "user.name",
  "user.preferred_name",
  "user.pronouns",
  "user.birthday",
  "user.home",
  "user.timezone",
  "user.employer",
  "user.role",
  "user.school",
  "user.study",
  "user.work_hours",
  "user.wake_time",
  "user.sleep_time",
  "person.relation",
  "person.birthday",
  "person.home",
  "person.employer",
  "person.role",
]);

/** The value of a one-valued claim whose value is plain words, which Edit changes on its own; null for any other claim. */
export function plainValue(c) {
  return oneValued.has(c.key) && typeof c.value === "string" ? c.value : null;
}

/** Which list a claim is shown in: about the user, about a person, or about anything else. */

export function sectionOf(c) {
  const ns = c.key.split(".")[0];
  if (ns === "person") return "people";
  if (ns === "org" || ns === "place" || ns === "topic") return "other";
  return c.subject ? "other" : "you";
}

/** A claim that is reviewed before Gwen relies on it. */
export function toConfirm(status) {
  return status === "candidate" || status === "quarantined";
}

/** The lanes in words. */
const laneNames = {
  gwen: "Gwen's memory",
  claims: "Facts",
  chats: "Past chats",
  episodes: "Notes and documents",
  entity: "People",
  time: "By date",
  data: "Your tracked days",
  vec: "Semantic search",
  mem0: "Mem0",
  supermemory: "Supermemory",
  files: "Memory folder",
};

export function laneLabel(lane) {
  return laneNames[lane] ?? lane.charAt(0).toUpperCase() + lane.slice(1);
}

/** Where a forget erased things, as the receipt names it. */
const erasedFrom = {
  vec: "the search index",
  files: "the memory folder",
  mem0: "Mem0",
  supermemory: "Supermemory",
};

/** A lane's state in words, in short for a list, and a dot tone: "idle", "blocked" or "ok". */
export function laneState(l) {
  if (!l.enabled) return { words: "Off", short: "Off", color: "idle" };
  if (!l.ok) return { words: l.detail ? `Not working: ${l.detail}` : "Not working", short: "Not working", color: "blocked" };
  const items = l.items >= 0 ? `${l.items.toLocaleString()} ${l.items === 1 ? "item" : "items"}` : "";
  return { words: ["Working", items].filter(Boolean).join(", "), short: items || "Working", color: "ok" };
}

/** "a, b, and c". */
export function joinWords(parts) {
  if (parts.length <= 1) return parts[0] ?? "";
  if (parts.length === 2) return `${parts[0]} and ${parts[1]}`;
  return `${parts.slice(0, -1).join(", ")}, and ${parts.at(-1)}`;
}

/** A forget receipt in words: what was forgotten, where it was erased, and what could not be. */
export function receiptText(r, what, name) {
  const lanes = r.lanes ?? [];
  const erased = lanes.filter((l) => l.ok).map((l) => (l.lane === "gwen" ? `${name}'s memory` : (erasedFrom[l.lane] ?? laneLabel(l.lane))));
  const failed = lanes.filter((l) => !l.ok).map((l) => `${laneLabel(l.lane)} couldn't erase it${l.note ? `: ${l.note}` : ""}.`);
  const notes = lanes.filter((l) => l.ok && l.note).map((l) => l.note);
  const parts = [erased.length ? `${what} Erased from ${joinWords(erased)}.` : what, ...failed, ...notes];
  return parts.map((p) => (/[.!?]$/.test(p) ? p : `${p}.`)).join(" ");
}

/** "3 facts and 2 notes". */
export function countWords(r) {
  const parts = [];
  if (r.claims) parts.push(`${r.claims} ${r.claims === 1 ? "fact" : "facts"}`);
  if (r.episodes) parts.push(`${r.episodes} ${r.episodes === 1 ? "note" : "notes"}`);
  return joinWords(parts);
}

/** What remembering made, in words, for a toast. */
export function rememberedText(claims, name) {
  if (claims.length === 0) return `${name} didn't find anything to remember in that. Try saying it as a plain fact.`;
  const shown = claims.slice(0, 4).map((c) => c.text);
  const more = claims.length > 4 ? `, and ${claims.length - 4} more` : "";
  return `${name} will remember: ${shown.join("; ")}${more}.`;
}

/** One section of the daily profile card, without its refs. */

/** The profile card's text as sections of lines, without the header for the model or the [pN] refs. */
export function cardSections(text) {
  const out = [];
  for (const raw of text.split("\n")) {
    const line = raw.trim();
    if (!line || line.startsWith("What you know about them")) continue;
    if (line.startsWith("- ")) {
      if (out.length === 0) out.push({ heading: "", lines: [] });
      out[out.length - 1].lines.push(line.slice(2).replace(/^\[p\d+\]\s*/, ""));
    } else {
      const heading = line.replace(/:$/, "");
      out.push({ heading: heading === "About them" ? "About you" : heading, lines: [] });
    }
  }
  return out.filter((s) => s.lines.length > 0);
}

/** What a recall hit or a cited ref is, in words, when it is not a fact. */
export function kindLabel(kind) {
  switch (kind) {
    case "claim":
      return "Fact";
    case "msg":
      return "From a chat";
    case "ep":
      return "A note";
    case "doc":
      return "A document";
    case "data":
      return "Your tracked days";
  }
  return "Memory";
}

/** The size of a file in words. */
export function sizeText(bytes) {
  if (bytes < 1024) return `${bytes} bytes`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}
