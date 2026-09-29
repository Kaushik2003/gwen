// Mirrors the matching in internal/planner/rrule.go (docs/06-planner.md#recurrence),
// only to show which commitments fall on a day. The daemon validates every rule,
// so anything unparsable here simply never matches.

import type { wire } from "./api";

const weekdays: Record<string, number> = { SU: 0, MO: 1, TU: 2, WE: 3, TH: 4, FR: 5, SA: 6 };

interface Rule {
  freq: string;
  interval: number;
  byDay: number[];
  byMonthDay: number;
  until: string | null; // YYYY-MM-DD
}

function parse(rule: string): Rule | null {
  const r: Rule = { freq: "", interval: 1, byDay: [], byMonthDay: 0, until: null };
  for (const part of rule.split(";")) {
    const [key, value] = part.split("=");
    if (!value) return null;
    if (key === "FREQ") r.freq = value;
    else if (key === "INTERVAL") r.interval = Number(value);
    else if (key === "BYDAY") r.byDay = value.split(",").map((d) => weekdays[d]);
    else if (key === "BYMONTHDAY") r.byMonthDay = Number(value);
    else if (key === "UNTIL") r.until = `${value.slice(0, 4)}-${value.slice(4, 6)}-${value.slice(6, 8)}`;
    else return null;
  }
  return r.freq ? r : null;
}

function utc(day: string): Date {
  const [y, m, d] = day.split("-").map(Number);
  return new Date(Date.UTC(y, m - 1, d));
}

function daysBetween(a: Date, b: Date): number {
  return Math.round((b.getTime() - a.getTime()) / 86_400_000);
}

function monday(d: Date): Date {
  return new Date(d.getTime() - ((d.getUTCDay() + 6) % 7) * 86_400_000);
}

/** Whether rule, anchored at anchor, recurs on day. */
export function matches(rule: string, anchor: string, day: string): boolean {
  const r = parse(rule);
  if (!r || day < anchor || (r.until && day > r.until)) return false;
  const a = utc(anchor);
  const d = utc(day);
  switch (r.freq) {
    case "DAILY":
      return daysBetween(a, d) % r.interval === 0;
    case "WEEKLY": {
      const on = r.byDay.length ? r.byDay : [a.getUTCDay()];
      return on.includes(d.getUTCDay()) && (daysBetween(monday(a), monday(d)) / 7) % r.interval === 0;
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
