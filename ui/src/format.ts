// Mirrors internal/client/format.go (docs/08-clients.md#shared-behaviour),
// plus the friendlier dates the GUI shows.

/** "7h 32m", "8h", "45m", or "30s" below one minute. */
export function formatDuration(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000));
  if (s < 60) return `${s}s`;
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (h === 0) return `${m}m`;
  if (m === 0) return `${h}h`;
  return `${h}h ${m}m`;
}

/** "3:12:05", or "3:12" without seconds, for ticking timers. */
export function clockFace(ms: number, seconds = true): string {
  const s = Math.max(0, Math.floor(ms / 1000));
  const h = Math.floor(s / 3600);
  const m = String(Math.floor((s % 3600) / 60)).padStart(2, "0");
  return seconds ? `${h}:${m}:${String(s % 60).padStart(2, "0")}` : `${h}:${m}`;
}

/** 24-hour "HH:MM" in the local zone. */
export function formatTime(ms: number): string {
  const d = new Date(ms);
  return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** "YYYY-MM-DD" of a local date. */
export function formatDay(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

const weekdays = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];
const months = ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"];

/** The local Date of "YYYY-MM-DD". */
export function dateOf(day: string): Date {
  const [y, m, d] = day.split("-").map(Number);
  return new Date(y, m - 1, d);
}

/** "Mon 5 Oct", with the year when it is not this year; weekday false drops "Mon". */
export function formatDate(day: string, weekday = true): string {
  const d = dateOf(day);
  const year = d.getFullYear() !== new Date().getFullYear() ? ` ${d.getFullYear()}` : "";
  return `${weekday ? weekdays[d.getDay()].slice(0, 3) + " " : ""}${d.getDate()} ${months[d.getMonth()].slice(0, 3)}${year}`;
}

/** "Monday, 5 October". */
export function formatLongDate(day: string): string {
  const d = dateOf(day);
  const year = d.getFullYear() !== new Date().getFullYear() ? ` ${d.getFullYear()}` : "";
  return `${weekdays[d.getDay()]}, ${d.getDate()} ${months[d.getMonth()]}${year}`;
}

/** Whole days from a to b, both "YYYY-MM-DD". */
export function daysBetween(a: string, b: string): number {
  return Math.round((dateOf(b).getTime() - dateOf(a).getTime()) / 86_400_000);
}

/** "today", "tomorrow", "in 5 days", "yesterday", "3 days ago" for day seen from today. */
export function relativeDay(day: string, today: string): string {
  const n = daysBetween(today, day);
  if (n === 0) return "today";
  if (n === 1) return "tomorrow";
  if (n === -1) return "yesterday";
  return n > 0 ? `in ${n} days` : `${-n} days ago`;
}

/** "just now", "12 min ago", "3 h ago", or the date and time. */
export function formatAgo(ms: number, now: number): string {
  const s = Math.max(0, Math.floor((now - ms) / 1000));
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.floor(s / 60)} min ago`;
  if (s < 86_400) return `${Math.floor(s / 3600)} h ago`;
  return `${formatDate(formatDay(new Date(ms)))} at ${formatTime(ms)}`;
}

/** The last 8 characters of an id. */
export function shortId(id: string): string {
  return id.length <= 8 ? id : id.slice(-8);
}

function pad(n: number): string {
  return String(n).padStart(2, "0");
}

/** Minutes after midnight of "HH:MM". */
export function minutesOf(hhmm: string): number {
  const [h, m] = hhmm.split(":").map(Number);
  return h * 60 + m;
}

/** The day an instant belongs to under the day rollover (docs/05-time-engine.md#day-boundaries). */
export function dayOf(ms: number, rollover: string): string {
  const d = new Date(ms);
  if (d.getHours() * 60 + d.getMinutes() < minutesOf(rollover)) d.setDate(d.getDate() - 1);
  return formatDay(d);
}

/** Offsets a "YYYY-MM-DD" date by n days. */
export function addDays(day: string, n: number): string {
  const [y, m, d] = day.split("-").map(Number);
  return formatDay(new Date(y, m - 1, d + n));
}

/** The instant "HH:MM" names on a work day; times before the rollover are on the next date. */
export function instantOn(day: string, hhmm: string, rollover: string): number {
  const [y, m, d] = day.split("-").map(Number);
  const mins = minutesOf(hhmm);
  const date = new Date(y, m - 1, d + (mins < minutesOf(rollover) ? 1 : 0), Math.floor(mins / 60), mins % 60);
  return date.getTime();
}

/** Milliseconds of a Go duration string such as "8h" or "7h30m". */
export function parseDuration(s: string): number {
  let ms = 0;
  for (const [, n, unit] of s.matchAll(/(\d+(?:\.\d+)?)(ms|h|m|s)/g)) {
    ms += Number(n) * ({ h: 3_600_000, m: 60_000, s: 1000, ms: 1 } as Record<string, number>)[unit];
  }
  return ms;
}

/** A Go duration string for ms: "7h30m", "45m", "30s", or "0s". */
export function goDuration(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  const out = `${h ? h + "h" : ""}${m ? m + "m" : ""}${sec ? sec + "s" : ""}`;
  return out || "0s";
}
