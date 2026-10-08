import { useEffect, useState, type InputHTMLAttributes } from "react";
import { formatClock, minutesOf } from "../format";
import { cx } from "./ui";

/**
 * "HH:MM" from what was typed, such as 9, 930, 9:30, 9.30 pm, 930p or 21:05;
 * null when it is not a time. Without am or pm the hour is on the 24-hour clock.
 */
export function parseClock(s: string): string | null {
  const ampm = /^(.*?)\s*([ap])\.?m?\.?$/i.exec(s.trim());
  const t = (ampm ? ampm[1] : s).trim().replace(/[.h]/, ":");
  let h: number;
  let m: number;
  const colon = /^(\d{1,2}):(\d{1,2})$/.exec(t);
  if (colon) {
    h = Number(colon[1]);
    m = Number(colon[2]);
  } else if (/^\d{1,4}$/.test(t)) {
    h = Number(t.length <= 2 ? t : t.slice(0, -2));
    m = t.length <= 2 ? 0 : Number(t.slice(-2));
  } else return null;
  if (ampm) {
    if (h < 1 || h > 12) return null;
    h = (h % 12) + (ampm[2].toLowerCase() === "p" ? 12 : 0);
  }
  return h > 23 || m > 59 ? null : `${pad(h)}:${pad(m)}`;
}

function pad(n: number): string {
  return String(n).padStart(2, "0");
}

/**
 * A time field that shows "9:30 am" and reports "HH:MM". The webview's own
 * time input follows the system locale, so the dashboard keeps its own: 12-hour
 * with am and pm everywhere. The time is reported when it is committed, on
 * Enter or on leaving the field; the up and down arrows step it by 15 minutes,
 * or by an hour with Shift.
 */
export default function TimeInput({
  value,
  onCommit,
  allowEmpty,
  className,
  ...rest
}: { value: string; onCommit: (hhmm: string) => void; allowEmpty?: boolean } & Omit<InputHTMLAttributes<HTMLInputElement>, "value" | "onChange" | "type">) {
  const [text, setText] = useState(shown(value));
  useEffect(() => {
    setText(shown(value));
  }, [value]);

  function commit() {
    if (allowEmpty && text.trim() === "") {
      if (value !== "") onCommit("");
      return;
    }
    const v = parseClock(text);
    if (v === null) {
      setText(shown(value));
      return;
    }
    setText(shown(v));
    if (v !== value) onCommit(v);
  }
  function step(minutes: number) {
    const [h, m] = (parseClock(text) ?? (value || "09:00")).split(":").map(Number);
    const t = (((h * 60 + m + minutes) % 1440) + 1440) % 1440;
    setText(formatClock(t));
  }

  return (
    <input
      {...rest}
      type="text"
      autoComplete="off"
      maxLength={8}
      placeholder={allowEmpty ? "--:--" : "9:00 am"}
      className={cx("field text-center tabular-nums", className)}
      value={text}
      onChange={(e) => setText(e.target.value)}
      onBlur={commit}
      onKeyDown={(e) => {
        if (e.key === "Enter") {
          e.preventDefault();
          commit();
        } else if (e.key === "ArrowUp" || e.key === "ArrowDown") {
          e.preventDefault();
          step((e.key === "ArrowUp" ? 1 : -1) * (e.shiftKey ? 60 : 15));
        }
      }}
    />
  );
}

/** "9:30 am" of "HH:MM", or "" for none. */
function shown(hhmm: string): string {
  return hhmm ? formatClock(minutesOf(hhmm)) : "";
}
