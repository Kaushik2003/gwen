// Mirrors the matching in internal/planner/rrule.go (docs/06-planner.md#recurrence),
// to show which commitments fall on a day, and builds and describes the rules
// the recurrence picker edits. The daemon validates every rule, so anything
// unparsable here simply never matches and is edited as text.

import type { wire } from "./api";

/** Weekday codes, Monday first, as the picker shows them. */
export const weekdayCodes = ["MO", "TU", "WE", "TH", "FR", "SA", "SU"] as const;
export type WeekdayCode = (typeof weekdayCodes)[number];
const weekdayIndex: Record<string, number> = { SU: 0, MO: 1, TU: 2, WE: 3, TH: 4, FR: 5, SA: 6 };
const weekdayNames: Record<WeekdayCode, string> = { MO: "Mon", TU: "Tue", WE: "Wed", TH: "Thu", FR: "Fri", SA: "Sat", SU: "Sun" };

export interface Rule {
  freq: "DAILY" | "WEEKLY" | "MONTHLY";
  interval: number;
  byDay: WeekdayCode[]; // WEEKLY only; empty means the anchor's weekday
  byMonthDay: number; // MONTHLY only, 1–28
  until: string | null; // YYYY-MM-DD
}

/** The rule's parts, or null when it is outside the subset the planner accepts. */
export function parseRule(rule: string): Rule | null {
  const r: Rule = { freq: "DAILY", interval: 1, byDay: [], byMonthDay: 0, until: null };
  let freq = "";
  for (const part of rule.trim().split(";")) {
    const [key, value] = part.split("=");
    if (!value) return null;
    if (key === "FREQ") freq = value;
    else if (key === "INTERVAL") r.interval = Number(value);
    else if (key === "BYDAY") {
      const days = value.split(",");
      if (!days.every((d) => d in weekdayIndex)) return null;
      r.byDay = days as WeekdayCode[];
    } else if (key === "BYMONTHDAY") r.byMonthDay = Number(value);
    else if (key === "UNTIL" && /^\d{8}$/.test(value)) r.until = `${value.slice(0, 4)}-${value.slice(4, 6)}-${value.slice(6, 8)}`;
    else return null;
  }
  if (freq !== "DAILY" && freq !== "WEEKLY" && freq !== "MONTHLY") return null;
  if (!Number.isInteger(r.interval) || r.interval < 1) return null;
  r.freq = freq;
  return r;
}

/** The RRULE text of r, in a fixed part order. */
export function buildRule(r: Rule): string {
  const parts = [`FREQ=${r.freq}`];
  if (r.interval > 1) parts.push(`INTERVAL=${r.interval}`);
  if (r.freq === "WEEKLY" && r.byDay.length) parts.push(`BYDAY=${weekdayCodes.filter((d) => r.byDay.includes(d)).join(",")}`);
  if (r.freq === "MONTHLY") parts.push(`BYMONTHDAY=${r.byMonthDay || 1}`);
  if (r.until) parts.push(`UNTIL=${r.until.replaceAll("-", "")}`);
  return parts.join(";");
}

const weekdaysOnly: WeekdayCode[] = ["MO", "TU", "WE", "TH", "FR"];

function ordinal(n: number): string {
  const s = n % 100 >= 11 && n % 100 <= 13 ? "th" : ({ 1: "st", 2: "nd", 3: "rd" } as Record<number, string>)[n % 10] ?? "th";
  return `${n}${s}`;
}

/** "Every weekday", "Mon, Wed and Fri", "Every 2 days", "Monthly on the 5th"; the rule itself when unknown. */
export function describeRule(rule: string): string {
  const r = parseRule(rule);
  if (!r) return rule;
  const every = (unit: string) => (r.interval > 1 ? `Every ${r.interval} ${unit}s` : "");
  let text: string;
  if (r.freq === "DAILY") text = every("day") || "Every day";
  else if (r.freq === "MONTHLY") text = `${every("month") || "Monthly"} on the ${ordinal(r.byMonthDay || 1)}`;
  else {
    const days = weekdayCodes.filter((d) => r.byDay.includes(d));
    let on: string;
    if (days.length === 7) on = "every day";
    else if (days.length === 5 && weekdaysOnly.every((d) => days.includes(d))) on = "weekdays";
    else if (days.length === 2 && days.includes("SA") && days.includes("SU")) on = "weekends";
    else if (days.length === 0) on = "the same weekday";
    else on = listOf(days.map((d) => weekdayNames[d]));
    text = r.interval > 1 ? `Every ${r.interval} weeks on ${on}` : on === "weekdays" ? "Every weekday" : on === "every day" ? "Every day" : `Every ${on}`;
  }
  return r.until ? `${text}, until ${r.until}` : text;
}

function listOf(items: string[]): string {
  return items.length <= 1 ? items.join("") : `${items.slice(0, -1).join(", ")} and ${items[items.length - 1]}`;
}

/** The weekdays a weekly rule lights up, for the dot strip; null when it is not weekly or daily. */
export function litWeekdays(rule: string): Set<WeekdayCode> | null {
  const r = parseRule(rule);
  if (!r || r.freq === "MONTHLY") return null;
  if (r.freq === "DAILY") return r.interval === 1 ? new Set(weekdayCodes) : null;
  return new Set(r.byDay);
}

function utc(day: string): Date {
  const [y, m, d] = day.split("-").map(Number);
  return new Date(Date.UTC(y, m - 1, d));
}

function daysApart(a: Date, b: Date): number {
  return Math.round((b.getTime() - a.getTime()) / 86_400_000);
}

function monday(d: Date): Date {
  return new Date(d.getTime() - ((d.getUTCDay() + 6) % 7) * 86_400_000);
}

/** Whether rule, anchored at anchor, recurs on day. */
export function matches(rule: string, anchor: string, day: string): boolean {
  const r = parseRule(rule);
  if (!r || day < anchor || (r.until && day > r.until)) return false;
  const a = utc(anchor);
  const d = utc(day);
  switch (r.freq) {
    case "DAILY":
      return daysApart(a, d) % r.interval === 0;
    case "WEEKLY": {
      const on = r.byDay.length ? r.byDay.map((x) => weekdayIndex[x]) : [a.getUTCDay()];
      return on.includes(d.getUTCDay()) && (daysApart(monday(a), monday(d)) / 7) % r.interval === 0;
    }
    case "MONTHLY": {
      const months = (d.getUTCFullYear() - a.getUTCFullYear()) * 12 + d.getUTCMonth() - a.getUTCMonth();
      return d.getUTCDate() === r.byMonthDay && months % r.interval === 0;
    }
  }
  return false;
}

/** Whether a commitment occurs on day: its rule matches within its active range. */
export function occursOn(c: wire.Commitment, day: string): boolean {
  if (c.active_until && day > c.active_until) return false;
  return matches(c.rrule, c.active_from, day);
}

/** "HH:MM" of minutes after midnight. */
export function clockOf(minutes: number): string {
  const m = ((minutes % 1440) + 1440) % 1440;
  return `${String(Math.floor(m / 60)).padStart(2, "0")}:${String(m % 60).padStart(2, "0")}`;
}
