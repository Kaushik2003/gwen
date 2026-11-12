// Small pieces the Phone, Google Calendar, Sync and Memory sections share:
// a status card, the two kinds of setup step, a path chip, and a QR code.

import { div } from "gpui-kit";
import { v_flex, h_flex } from "gpui-base";
import { settings } from "herdr";
import { qrMatrix } from "./connect-qr.js";

/** A daemon message without its command-line hint, which the screen has a button for. */
export const withoutCLI = (msg) => String(msg ?? "").replace(/;\s*run gwen.*$/i, "");

/** The lines of a text box as a list, without blanks. */
export const fromLines = (text) => text.split("\n").map((l) => l.trim()).filter(Boolean);
export const sameList = (a, b) => a.length === b.length && a.every((v, i) => v === b[i]);

/** The raised box a connection's state sits in: an icon, words, and buttons on the right. */
export function statusCard(ui, o) {
  const t = ui.t;
  return h_flex()
    .w_full()
    .flex_wrap()
    .items_center()
    .gap(12)
    .px(14)
    .py(11)
    .rounded(8)
    .border_1()
    .border_color(t.border)
    .bg(t.raised)
    .child(ui.icon(o.icon ?? "circle-check", 18, o.color ?? ui.tone("ok")))
    .child(
      v_flex()
        .flex_1()
        .min_w(160)
        .gap(2)
        .child(div().text_size(13.5).font_medium().text_color(t.foreground).truncate().child(o.title))
        .when(Boolean(o.detail), (e) => e.child(ui.muted(o.detail, { size: 12 }))),
    )
    .children((o.actions ?? []).filter(Boolean));
}

/** One numbered step of a setup list: the number in a circle, the words, and whatever goes under them. */
export function numberedStep(ui, n, text, under) {
  const t = ui.t;
  return h_flex()
    .w_full()
    .items_start()
    .gap(12)
    .child(div().flex().flex_none().items_center().justify_center().w(24).h(24).rounded(12).bg(t.hover).text_size(12).font_semibold().text_color(t.muted).child(String(n)))
    .child(
      v_flex()
        .flex_1()
        .min_w(0)
        .pt(2)
        .gap(8)
        .child(div().text_size(13.5).line_height("20px").text_color(t.muted).child(text))
        .when(Boolean(under), (e) => e.child(under)),
    );
}

/** One row of a checklist: whether it is in order, what it is, what to do about it. */
export function checkStep(ui, ok, title, body, action) {
  const t = ui.t;
  return h_flex()
    .w_full()
    .items_start()
    .gap(12)
    .py(10)
    .child(div().pt(1).child(ui.icon(ok ? "circle-check" : "circle-alert", 17, ok ? ui.tone("ok") : ui.tone("warn"))))
    .child(
      v_flex()
        .flex_1()
        .min_w(0)
        .gap(3)
        .child(div().text_size(13.5).font_medium().text_color(t.foreground).child(title))
        .when(Boolean(body), (e) => e.child(typeof body === "string" ? ui.muted(body, { size: 13, lh: 19 }) : body)),
    )
    .when(Boolean(action), (e) => e.child(div().flex_none().child(action)));
}

/** A file path, address or command shown as it is typed. */
export function chip(ui, text, o = {}) {
  const t = ui.t;
  const el = div()
    .min_w(0)
    .px(9)
    .py(5)
    .rounded(6)
    .border_1()
    .border_color(t.border)
    .bg(t.raised)
    .text_size(o.size ?? 12.5)
    .text_color(t.muted)
    .font_family(settings().font_family ?? "JetBrains Mono")
    .child(text);
  if (o.wrap) el.line_height("17px");
  else el.truncate();
  return el;
}

/** A title with words under it and a control below both, for settings whose control is wide. */
export function stacked(ui, title, hint, control, o = {}) {
  const t = ui.t;
  const el = v_flex()
    .w_full()
    .gap(9)
    .py(12)
    .child(
      v_flex()
        .gap(3)
        .child(div().text_size(13.5).text_color(t.foreground).child(title))
        .when(Boolean(hint), (e) => e.child(typeof hint === "string" ? ui.muted(hint) : hint))
        .when(Boolean(o.error), (e) => e.child(ui.text(o.error, { size: 12, color: ui.tone("danger") }))),
    );
  if (control) el.child(control);
  if (o.divider !== false) el.border_b(1).border_color(t.border);
  return el;
}

/** The buttons under a form: Revert when something changed, then Save changes. */
export function saveBar(ui, id, o) {
  return h_flex()
    .w_full()
    .justify_end()
    .gap(8)
    .pt(4)
    .when(Boolean(o.onRevert && o.changed), (e) => e.child(ui.button(`${id}-revert`, "Revert", o.onRevert, { tone: "ghost" })))
    .child(ui.button(`${id}-save`, o.label ?? "Save changes", o.onSave, { tone: "primary", disabled: !o.changed || o.disabled, busy: o.busy }));
}

const qrCache = new Map();

/**
 * A QR code of text, drawn as squares on white: `size` is the side of the
 * whole picture. Each row is drawn as runs of dark and light, so a code is a
 * few hundred elements rather than a few thousand.
 */
export function qrCode(ui, text, size = 176) {
  let m = qrCache.get(text);
  if (m === undefined) {
    try {
      m = qrMatrix(text);
    } catch {
      m = null; // too long for a code
    }
    if (qrCache.size > 20) qrCache.clear();
    qrCache.set(text, m);
  }
  if (!m) return null;
  const n = m.length;
  const quiet = 2; // modules of white around the code
  const cell = Math.max(2, Math.floor(size / (n + quiet * 2)));
  const pad = Math.floor((size - cell * n) / 2);
  const rows = m.map((row) => {
    const runs = [];
    for (let x = 0; x < n; ) {
      let end = x;
      while (end < n && row[end] === row[x]) end++;
      runs.push(div().flex_none().h(cell).w((end - x) * cell).bg(row[x] ? "#010102" : "#ffffff"));
      x = end;
    }
    return h_flex().flex_none().h(cell).children(runs);
  });
  return div()
    .flex_none()
    .w(cell * n + pad * 2)
    .h(cell * n + pad * 2)
    .p(pad)
    .rounded(8)
    .bg("#ffffff")
    .child(v_flex().children(rows));
}
