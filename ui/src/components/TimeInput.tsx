import { useEffect, useState, type InputHTMLAttributes } from "react";
import { cx } from "./ui";

/** "HH:MM" from what was typed, such as 9, 930, 0930, 9:30 or 21.05; null when it is not a time. */
export function parseClock(s: string): string | null {
  const t = s.trim().replace(/[.h]/, ":");
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
  return h > 23 || m > 59 ? null : `${pad(h)}:${pad(m)}`;
}

function pad(n: number): string {
  return String(n).padStart(2, "0");
}

/**
 * A 24-hour HH:MM field. The webview's own time input follows the system
 * locale, which shows AM and PM on many systems, while the dashboard shows
 * 24-hour times everywhere (docs/08-clients.md#shared-behaviour). The time is
 * reported when it is committed, on Enter or on leaving the field; the up and
 * down arrows step it by 15 minutes, or by an hour with Shift.
 */
export default function TimeInput({
  value,
  onCommit,
  allowEmpty,
  className,
  ...rest
}: { value: string; onCommit: (hhmm: string) => void; allowEmpty?: boolean } & Omit<InputHTMLAttributes<HTMLInputElement>, "value" | "onChange" | "type">) {
  const [text, setText] = useState(value);
  useEffect(() => {
    setText(value);
  }, [value]);

  function commit() {
    if (allowEmpty && text.trim() === "") {
      if (value !== "") onCommit("");
      return;
    }
    const v = parseClock(text);
    if (v === null) {
      setText(value);
      return;
    }
    setText(v);
    if (v !== value) onCommit(v);
  }
  function step(minutes: number) {
    const [h, m] = (parseClock(text) ?? (value || "09:00")).split(":").map(Number);
    const t = (((h * 60 + m + minutes) % 1440) + 1440) % 1440;
    setText(`${pad(Math.floor(t / 60))}:${pad(t % 60)}`);
  }

  return (
    <input
      {...rest}
      type="text"
      inputMode="numeric"
      autoComplete="off"
      maxLength={5}
      placeholder={allowEmpty ? "--:--" : "HH:MM"}
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
