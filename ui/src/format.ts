// Mirrors internal/client/format.go (docs/08-clients.md#shared-behaviour).

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

/** 24-hour "HH:MM" in the local zone. */
export function formatTime(ms: number): string {
  const d = new Date(ms);
  return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** "YYYY-MM-DD" of a local date. */
export function formatDay(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
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
