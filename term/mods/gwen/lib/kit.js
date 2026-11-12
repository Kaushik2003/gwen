// Gwen's shared pieces, built from gpui-kit so they look like the rest of
// the window (plan.md 3.15): buttons, fields, pickers, panels, dialogs.
//
// Each Gwen tab has one Ui. Screens build their elements through it:
//
//   ui.button("save", "Save", (e, cx) => this.save(cx), { tone: "primary" })
//
// Rules that come from gpui-kit, which every screen follows:
//  - an element that is clicked, hovered or scrolled needs .id("unique-in-this-tab");
//  - text fields are created outside render (in a constructor or a handler):
//    `this.title = new TextField({ placeholder: "Title" })`, then `this.title.el(ui)` in render;
//  - every handler gets its own cx: (event, cx) => { ...; cx.notify(); }.

import { div, svg, image } from "gpui-kit";
import { v_flex, h_flex, InputState, Input, TextareaState, Textarea, Popup, TextView } from "gpui-base";
import { theme, settings } from "herdr";
import { addDays, dateOf, formatClock, formatDate, formatDay, goDuration, minutesOf, parseDuration } from "./format.js";

/** The colour a tracking state is shown in. Fixed: they match the tray icons. */
export const stateColor = { off: "#62666d", working: "#27a644", idle_pending: "#f2c94c", break_auto: "#4ea7fc", break_manual: "#4ea7fc" };
export const stateLabel = { off: "Not clocked in", working: "Working", idle_pending: "Idle (still counting)", break_auto: "On break (automatic)", break_manual: "On break" };
/** The colour of an unassigned project's time. */
export const unassignedColor = "#8a8f98";
export const priorities = [
  { value: 1, label: "Low", color: "#62666d" },
  { value: 2, label: "Normal", color: "#8a8f98" },
  { value: 3, label: "High", color: "#f2c94c" },
  { value: 4, label: "Urgent", color: "#eb5757" },
];
export const priorityOf = (p) => priorities.find((x) => x.value === p) ?? priorities[1];
export const clamp01 = (n) => Math.max(0, Math.min(1, n));

const CLEAR = "#00000000";
const DANGER = "#e5484d";
const WARN = "#f2c94c";
const OK = "#27a644";

/** A colour with an alpha, from "#rrggbb" and 0..1. */
export function alpha(color, a) {
  const hex = String(color).replace("#", "");
  // A theme colour may carry its own alpha: the two multiply.
  const own = hex.length >= 8 ? parseInt(hex.slice(6, 8), 16) / 255 : 1;
  return `#${hex.slice(0, 6)}${Math.round(clamp01(a * own) * 255).toString(16).padStart(2, "0")}`;
}

/** "HH:MM" from what was typed, such as 9, 930, 9:30, 9.30 pm, 930p or 21:05; null when it is not a time. */
export function parseClock(s) {
  const ampm = /^(.*?)\s*([ap])\.?m?\.?$/i.exec(s.trim());
  const t = (ampm ? ampm[1] : s).trim().replace(/[.h]/, ":");
  let h, m;
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
  const pad = (n) => String(n).padStart(2, "0");
  return h > 23 || m > 59 ? null : `${pad(h)}:${pad(m)}`;
}

/**
 * A text field: its retained state and how it is drawn. Create it outside
 * render. Options: value, placeholder, multiline, rows (multiline), masked,
 * onChange(value, cx), onSubmit(value, cx, event), onBlur(value, cx).
 * Enter submits; in a multiline field Shift+Enter starts a new line.
 */
export class TextField {
  constructor(o = {}) {
    this.o = o;
    this.multiline = Boolean(o.multiline);
    this.focused = false;
    this.last = o.value ?? "";
    this.state = this.multiline
      ? TextareaState.new({ value: o.value ?? "", placeholder: o.placeholder ?? "", rows: o.rows ?? 3 })
      : InputState.new({ value: o.value ?? "", placeholder: o.placeholder ?? "" });
    if (o.masked) this.state.set_masked(true);
    if (this.multiline && o.grow) this.state.set_auto_grow(o.grow[0], o.grow[1]);
    this.state.on("change", (_e, cx) => {
      const v = this.state.value();
      if (v === this.last) return;
      this.last = v;
      this.o.onChange?.(v, cx);
      cx.notify();
    });
    this.state.on("submit", (e, cx) => {
      if (this.multiline && e?.shift) return;
      this.o.onSubmit?.(this.state.value(), cx, e);
    });
    this.state.on("focus", (_e, cx) => {
      this.focused = true;
      cx.notify();
    });
    this.state.on("blur", (_e, cx) => {
      this.focused = false;
      this.o.onBlur?.(this.state.value(), cx);
      cx.notify();
    });
  }

  get value() {
    return this.state.value();
  }

  /** Replaces the text without reporting a change. */
  set(v) {
    this.last = v ?? "";
    this.state.set_value(this.last);
  }

  focus() {
    this.state.focus();
  }

  /** The field as an element. o: {w, h, flex, mono, invalid, size, bare}. */
  el(ui, o = {}) {
    const t = ui.t;
    const el = (this.multiline ? Textarea.new(this.state) : Input.new(this.state))
      .px(10)
      .rounded(7)
      .text_size(o.size ?? 13)
      .text_color(t.foreground);
    if (!this.multiline) el.h(o.h ?? 32).items_center();
    else {
      el.py(7);
      if (o.h) el.h(o.h);
    }
    if (!o.bare) el.border_1().border_color(o.invalid ? DANGER : this.focused ? t.accent : t.input).bg(t.background);
    if (o.mono) el.font_family(settings().font_family ?? "JetBrains Mono");
    if (o.w) el.w(o.w);
    else if (o.flex !== false) el.flex_1().min_w(0);
    return el;
  }
}

/** One Gwen tab's building kit and the little state its pieces need. */
export class Ui {
  constructor() {
    this.t = theme();
    this.popup = null; // the id of the open menu or picker
    this.popupState = {}; // per popup: the month a date picker shows, and so on
    this.dialogs = []; // {id, render(ui), onClose, width}
    // A text field cannot be made while drawing, so the fields that pickers
    // and textInput need are made ahead of time (refill) and handed out by id.
    this.fields = {};
    this.spare = [];
    this.starved = false;
    this.frame = 0;
    this.seq = 0;
    this.refill();
  }

  /** Call first in every render: the theme may have changed. */
  begin() {
    this.t = theme();
    this.frame++;
    if (this.frame % 40 === 0) {
      // Fields nothing has drawn for a while go back to the spares.
      for (const [id, f] of Object.entries(this.fields)) {
        if (f.usedAt >= this.frame - 3 || f.focused) continue;
        delete this.fields[id];
        f.o = {};
        f.current = null;
        f.state.set_masked(false);
        f.set("");
        this.spare.push(f);
      }
    }
    return this.t;
  }

  /**
   * Makes spare text fields. Call from init, a handler or a timer, never
   * from render. Returns true when a picker was waiting for one, so the
   * caller redraws.
   */
  refill() {
    while (this.spare.length < 24) this.spare.push(new TextField({}));
    const waiting = this.starved;
    this.starved = false;
    return waiting;
  }

  // Colours ---------------------------------------------------------------

  /** A named tone as a colour: ok, warn, danger, accent, working, break, idle, muted, dim, blocked. */
  tone(name) {
    const t = this.t;
    return (
      { ok: OK, working: OK, warn: WARN, idle_pending: WARN, danger: DANGER, blocked: DANGER, accent: t.accent, break: "#4ea7fc", idle: t.dim, off: "#62666d", muted: t.muted, dim: t.dim, neutral: t.muted }[name] ?? name
    );
  }

  // Text and icons ----------------------------------------------------------

  /** A lucide icon from icons/, by its kebab-case name, tinted. */
  icon(name, size = 14, color) {
    return svg(`mods/gwen/icons/${name}.svg`).size(size).flex_none().text_color(color ?? this.t.muted);
  }

  /** A turning ring for work in progress. */
  spinner(size = 13, color) {
    return svg("mods/gwen/icons/loader-circle.svg").size(size).flex_none().text_color(color ?? this.t.muted).spin(900);
  }

  /** Gwen's face for a mood (smiling, happy, love, winking, cool, smug, thinking, shocked, annoyed, grumpy, angry, crying). */
  face(mood = "smiling", size = 48) {
    return image(`mods/gwen/faces/${mood}.png`).w(size).h(size).flex_none();
  }

  text(s, o = {}) {
    const el = div().text_size(o.size ?? 13).text_color(o.color ?? this.t.foreground).child(String(s ?? ""));
    if (o.bold) el.font_semibold();
    if (o.medium) el.font_medium();
    if (o.truncate) el.truncate().min_w(0);
    if (o.mono) el.font_family(settings().font_family ?? "JetBrains Mono");
    if (o.lh) el.line_height(`${o.lh}px`);
    return el;
  }

  /** Secondary text. */
  muted(s, o = {}) {
    return this.text(s, { size: 12.5, color: this.t.muted, lh: 18, ...o });
  }

  /** Tertiary text: hints, captions. */
  dim(s, o = {}) {
    return this.text(s, { size: 12, color: this.t.dim, lh: 17, ...o });
  }

  /** A small upper-case section label. */
  label(s) {
    return div().text_size(11).font_semibold().text_color(this.t.dim).child(String(s).toUpperCase());
  }

  /** Markdown text, selectable. */
  markdown(id, text) {
    return TextView.markdown(id, text ?? "").selectable(true).text_size(13).text_color(this.t.foreground);
  }

  // Buttons -----------------------------------------------------------------

  /**
   * A button. o: {tone: "secondary" | "primary" | "ghost" | "danger" | "accent", icon, small, disabled, busy, tip, iconColor, grow}.
   */
  button(id, label, onClick, o = {}) {
    const t = this.t;
    const tone = o.tone ?? "secondary";
    const off = o.disabled || o.busy;
    const fg = tone === "primary" ? t.solid_foreground : tone === "accent" ? t.accent_foreground : tone === "danger" ? DANGER : t.foreground;
    const el = h_flex()
      .id(id)
      .flex_none()
      .items_center()
      .justify_center()
      .gap(6)
      .px(o.small ? 8 : 12)
      .h(o.small ? 24 : 30)
      .rounded(7)
      .text_size(o.small ? 12 : 13)
      .text_color(fg);
    if (tone === "primary") el.bg(t.solid).font_medium();
    else if (tone === "accent") el.bg(t.accent).font_medium();
    else if (tone === "ghost") el.bg(CLEAR);
    else el.bg(t.raised).border_1().border_color(tone === "danger" ? alpha(DANGER, 0.45) : t.border);
    if (o.grow) el.flex_1();
    if (off) el.opacity(0.5);
    else {
      el.cursor_pointer().on_click((e, cx) => {
        this.flush(cx);
        onClick(e, cx);
      });
      if (tone === "primary" || tone === "accent") el.hover((s) => s.opacity(0.9));
      else el.hover((s) => s.bg(t.hover));
    }
    if (o.busy) el.child(this.spinner(12, fg));
    else if (o.icon) el.child(this.icon(o.icon, o.small ? 12 : 14, o.iconColor ?? (tone === "secondary" || tone === "ghost" ? t.muted : fg)));
    if (label) el.child(String(label));
    if (o.tip) el.tooltip(o.tip);
    return el;
  }

  /** A square button with only an icon. o: {tip, active, color, size, disabled, tone: "danger"}. */
  iconButton(id, icon, onClick, o = {}) {
    const t = this.t;
    const size = o.size ?? 26;
    const el = div()
      .id(id)
      .flex()
      .flex_none()
      .items_center()
      .justify_center()
      .w(size)
      .h(size)
      .rounded(6)
      .bg(o.active ? t.selected : CLEAR)
      .child(this.icon(icon, Math.round(size * 0.56), o.color ?? (o.tone === "danger" ? DANGER : o.active ? t.foreground : t.muted)));
    if (o.disabled) el.opacity(0.4);
    else
      el.cursor_pointer()
        .hover((s) => s.bg(t.hover))
        .on_click((e, cx) => {
          this.flush(cx);
          onClick(e, cx);
        });
    if (o.tip) el.tooltip(o.tip);
    return el;
  }

  /** Underlined text that acts. */
  link(id, label, onClick, o = {}) {
    return div().id(id).cursor_pointer().text_size(o.size ?? 12.5).text_color(o.color ?? this.t.accent).hover((s) => s.opacity(0.8)).on_click(onClick).child(label);
  }

  // Switches and choices ------------------------------------------------------

  /** A switch. onChange(next, cx). */
  toggle(id, on, onChange, o = {}) {
    const t = this.t;
    const el = div()
      .id(id)
      .w(34)
      .h(20)
      .flex_none()
      .rounded(10)
      .p(2)
      .bg(on ? t.accent : t.hover)
      .flex()
      .when(on, (e) => e.justify_end())
      .child(div().w(16).h(16).rounded(8).bg(on ? t.accent_foreground : t.muted));
    if (o.disabled) el.opacity(0.5);
    else el.cursor_pointer().on_click((e, cx) => onChange(!on, cx));
    return el;
  }

  /** A setting with a switch: title, what it does, and the switch on the right. */
  toggleRow(id, title, about, on, onChange, o = {}) {
    return this.row(title, about, this.toggle(id, on, onChange, o), o);
  }

  /** One setting: what it is on the left, its control on the right. */
  row(title, about, control, o = {}) {
    const t = this.t;
    const el = h_flex()
      .w_full()
      .gap(16)
      .py(11)
      .items_center()
      .child(
        v_flex()
          .flex_1()
          .min_w(0)
          .gap(3)
          .child(div().text_size(13.5).text_color(t.foreground).child(title))
          .when(Boolean(about), (e) => e.child(typeof about === "string" ? this.muted(about) : about)),
      );
    if (control) el.child(div().flex_none().child(control));
    if (o.divider !== false) el.border_b(1).border_color(t.border);
    return el;
  }

  /** A check box; round: a circle, as tasks use. onChange(next, cx). */
  checkbox(id, checked, onChange, o = {}) {
    const t = this.t;
    const size = o.size ?? 16;
    const color = o.color ?? t.accent;
    const el = div()
      .id(id)
      .flex()
      .flex_none()
      .items_center()
      .justify_center()
      .w(size)
      .h(size)
      .rounded(o.round ? size / 2 : 4)
      .border_1()
      .border_color(checked ? color : t.input)
      .bg(checked ? color : CLEAR);
    if (checked) el.child(this.icon("check", size - 5, o.round || o.color ? "#ffffff" : t.accent_foreground));
    if (o.disabled) el.opacity(0.5);
    else el.cursor_pointer().hover((s) => s.border_color(color)).on_click((e, cx) => onChange(!checked, cx));
    if (o.tip) el.tooltip(o.tip);
    return el;
  }

  /** A row of choices where one is picked. options: [{value, label, icon?}]. onChange(value, cx). */
  segmented(id, options, value, onChange, o = {}) {
    const t = this.t;
    return h_flex()
      .flex_none()
      .p(2)
      .gap(2)
      .rounded(8)
      .border_1()
      .border_color(t.border)
      .bg(t.background)
      .children(
        options.map((opt) => {
          const on = opt.value === value;
          const el = h_flex()
            .id(`${id}-${opt.value}`)
            .items_center()
            .gap(5)
            .px(o.small ? 8 : 11)
            .h(o.small ? 22 : 26)
            .rounded(6)
            .text_size(o.small ? 12 : 12.5)
            .cursor_pointer()
            .on_click((e, cx) => {
              if (!on) onChange(opt.value, cx);
            });
          if (on) el.bg(t.raised).text_color(t.foreground).border_1().border_color(t.border);
          else el.text_color(t.muted).hover((s) => s.text_color(t.foreground));
          if (opt.icon) el.child(this.icon(opt.icon, 13, on ? t.foreground : t.muted));
          if (opt.label) el.child(opt.label);
          if (opt.tip) el.tooltip(opt.tip);
          return el;
        }),
      );
  }

  /** The sub-screen tabs under a Gwen tab's status line. tabs: [{id, label, icon?, count?}]. */
  tabs(id, tabs, value, onChange) {
    const t = this.t;
    return h_flex()
      .flex_none()
      .gap(2)
      .children(
        tabs.map((tab) => {
          const on = tab.id === value;
          const el = h_flex()
            .id(`${id}-${tab.id}`)
            .items_center()
            .gap(6)
            .px(10)
            .h(28)
            .rounded(7)
            .text_size(13)
            .cursor_pointer()
            .on_click((e, cx) => {
              if (!on) onChange(tab.id, cx);
            });
          if (on) el.bg(t.selected).text_color(t.foreground);
          else el.text_color(t.muted).hover((s) => s.bg(t.hover).text_color(t.foreground));
          if (tab.icon) el.child(this.icon(tab.icon, 14, on ? t.foreground : t.muted));
          el.child(tab.label);
          if (tab.count) el.child(div().px(5).rounded(8).bg(t.raised).text_size(11).text_color(t.muted).child(String(tab.count)));
          return el;
        }),
      );
  }

  // Menus and pickers ---------------------------------------------------------

  isOpen(id) {
    return this.popup === id;
  }

  /** Opens a menu or picker by id, closing any other; again closes it. */
  togglePopup(id, cx) {
    this.popup = this.popup === id ? null : id;
    cx.notify();
  }

  closePopup(cx) {
    if (this.popup === null) return;
    this.popup = null;
    cx?.notify();
  }

  /** The floating surface of a menu or picker, closed by a press outside it. */
  surface(id, width) {
    const t = this.t;
    const el = v_flex()
      .id(`${id}-surface`)
      .mt(4)
      .p(4)
      .rounded(9)
      .border_1()
      .border_color(t.border)
      .bg(t.popover)
      .shadow_lg()
      .on_mouse_down_out((e, cx) => this.closePopup(cx))
      .on_scroll_wheel((e, cx) => cx.stop_propagation());
    if (width) el.w(width);
    return el;
  }

  /**
   * A trigger with a floating surface under it. content is built only while
   * open. o: {width, anchor}.
   */
  popover(id, trigger, content, o = {}) {
    const open = this.isOpen(id);
    return Popup.new(`${id}-popup`, trigger).when(open, (el) => el.content(this.surface(id, o.width).child(content())));
  }

  /** One row of a menu. o: {icon, color, checked, hint, disabled}. */
  menuItem(id, label, onClick, o = {}) {
    const t = this.t;
    const el = h_flex()
      .id(id)
      .w_full()
      .items_center()
      .gap(8)
      .px(8)
      .h(28)
      .rounded(6)
      .text_size(13)
      .text_color(o.color ?? t.foreground);
    if (o.dot) el.child(div().w(8).h(8).flex_none().rounded(4).bg(o.dot));
    if (o.icon) el.child(this.icon(o.icon, 14, o.color ?? t.muted));
    el.child(div().flex_1().min_w(0).truncate().child(label));
    if (o.hint) el.child(div().flex_none().text_size(11.5).text_color(t.dim).child(o.hint));
    if (o.checked) el.child(this.icon("check", 13, t.accent));
    if (o.disabled) el.opacity(0.5);
    else
      el.cursor_pointer()
        .hover((s) => s.bg(t.hover))
        .on_click((e, cx) => {
          this.popup = null;
          onClick(e, cx);
          cx.notify();
        });
    return el;
  }

  /**
   * A drop-down: shows the picked option, opens a list. options: [{value,
   * label, dot?, icon?, hint?}]. onChange(value, cx). o: {width, placeholder,
   * small, menuWidth, disabled, bare}.
   */
  select(id, options, value, onChange, o = {}) {
    const t = this.t;
    const current = options.find((x) => x.value === value);
    const open = this.isOpen(id);
    const trigger = h_flex()
      .id(id)
      .items_center()
      .gap(7)
      .px(o.small ? 8 : 10)
      .h(o.small ? 24 : 32)
      .rounded(7)
      .text_size(o.small ? 12 : 13);
    if (!o.bare) trigger.border_1().border_color(open ? t.accent : t.input).bg(t.background);
    if (o.width) trigger.w(o.width);
    if (current?.dot) trigger.child(div().w(8).h(8).flex_none().rounded(4).bg(current.dot));
    if (current?.icon) trigger.child(this.icon(current.icon, 13));
    trigger.child(div().flex_1().min_w(0).truncate().text_color(current ? t.foreground : t.dim).child(current?.label ?? o.placeholder ?? "Choose"));
    trigger.child(this.icon("chevron-down", 13, t.dim));
    if (o.disabled) return trigger.opacity(0.5);
    trigger.cursor_pointer().hover((s) => s.bg(t.hover)).on_click((e, cx) => this.togglePopup(id, cx));
    return this.popover(id, trigger, () =>
      v_flex()
        .id(`${id}-list`)
        .w(o.menuWidth ?? Math.max(o.width ?? 0, 200))
        .max_h(280)
        .overflow_y_scrollbar()
        .children(options.map((opt, i) => this.menuItem(`${id}-o${i}`, opt.label, (e, cx) => onChange(opt.value, cx), { dot: opt.dot, icon: opt.icon, hint: opt.hint, checked: opt.value === value }))),
    );
  }

  /** A "…" button that opens a menu. items: [{label, icon, onClick, danger, hint}] (null entries are skipped). */
  menuButton(id, items, o = {}) {
    const trigger = o.trigger ?? this.iconButton(id, o.icon ?? "ellipsis", (e, cx) => this.togglePopup(id, cx), { tip: o.tip, active: this.isOpen(id), size: o.size });
    return this.popover(id, trigger, () =>
      v_flex()
        .w(o.width ?? 190)
        .children(items.filter(Boolean).map((it, i) => this.menuItem(`${id}-m${i}`, it.label, it.onClick, { icon: it.icon, hint: it.hint, color: it.danger ? DANGER : undefined, disabled: it.disabled }))),
    );
  }

  /**
   * A date field with a calendar. value: "YYYY-MM-DD" or "". onChange(day, cx).
   * o: {clear: true to allow no date, placeholder, width, min, small}.
   */
  datePicker(id, value, onChange, o = {}) {
    const t = this.t;
    const open = this.isOpen(id);
    const trigger = h_flex()
      .id(id)
      .items_center()
      .gap(7)
      .px(o.small ? 8 : 10)
      .h(o.small ? 24 : 32)
      .rounded(7)
      .text_size(o.small ? 12 : 13)
      .border_1()
      .border_color(open ? t.accent : t.input)
      .bg(t.background)
      .cursor_pointer()
      .hover((s) => s.bg(t.hover))
      .on_click((e, cx) => {
        this.popupState[id] = { month: (value || formatDay(new Date())).slice(0, 7) };
        this.togglePopup(id, cx);
      })
      .child(this.icon("calendar", 13))
      .child(div().flex_1().min_w(0).truncate().text_color(value ? t.foreground : t.dim).child(value ? formatDate(value) : (o.placeholder ?? "No date")));
    if (o.width) trigger.w(o.width);
    return this.popover(id, trigger, () => this.calendar(id, value, onChange, o));
  }

  calendar(id, value, onChange, o) {
    const t = this.t;
    const st = (this.popupState[id] ??= { month: (value || formatDay(new Date())).slice(0, 7) });
    const [y, m] = st.month.split("-").map(Number);
    const first = new Date(y, m - 1, 1);
    const today = formatDay(new Date());
    const start = addDays(formatDay(first), -((first.getDay() + 6) % 7)); // Monday first
    const shift = (n, cx) => {
      const d = new Date(y, m - 1 + n, 1);
      st.month = formatDay(d).slice(0, 7);
      cx.notify();
    };
    const pick = (day, cx) => {
      this.popup = null;
      onChange(day, cx);
      cx.notify();
    };
    const monthName = `${["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"][m - 1]} ${y}`;
    const weeks = [];
    for (let w = 0; w < 6; w++) {
      const days = [];
      for (let d = 0; d < 7; d++) days.push(addDays(start, w * 7 + d));
      if (w > 3 && days[0].slice(0, 7) !== st.month) break;
      weeks.push(days);
    }
    const cell = (day) => {
      const inMonth = day.slice(0, 7) === st.month;
      const on = day === value;
      const blocked = o.min && day < o.min;
      const el = div()
        .id(`${id}-d-${day}`)
        .flex()
        .items_center()
        .justify_center()
        .w(30)
        .h(28)
        .rounded(6)
        .text_size(12.5)
        .text_color(on ? t.accent_foreground : !inMonth || blocked ? t.dim : t.foreground)
        .child(String(Number(day.slice(8))));
      if (on) el.bg(t.accent);
      else if (day === today) el.border_1().border_color(t.accent);
      if (!blocked) el.cursor_pointer().on_click((e, cx) => pick(day, cx));
      if (!on && !blocked) el.hover((s) => s.bg(t.hover));
      return el;
    };
    return v_flex()
      .w(226)
      .p(4)
      .gap(4)
      .child(
        h_flex()
          .items_center()
          .child(this.iconButton(`${id}-prev`, "chevron-left", (e, cx) => shift(-1, cx), { size: 24 }))
          .child(div().flex_1().flex().justify_center().text_size(13).font_medium().text_color(t.foreground).child(monthName))
          .child(this.iconButton(`${id}-next`, "chevron-right", (e, cx) => shift(1, cx), { size: 24 })),
      )
      .child(h_flex().children(["Mo", "Tu", "We", "Th", "Fr", "Sa", "Su"].map((d) => div().w(30).flex().justify_center().text_size(11).text_color(t.dim).child(d))))
      .children(weeks.map((days) => h_flex().children(days.map(cell))))
      .child(
        h_flex()
          .pt(4)
          .gap(6)
          .child(this.button(`${id}-today`, "Today", (e, cx) => pick(today, cx), { small: true, tone: "ghost" }))
          .child(this.button(`${id}-tomorrow`, "Tomorrow", (e, cx) => pick(addDays(today, 1), cx), { small: true, tone: "ghost" }))
          .child(div().flex_1())
          .when(Boolean(o.clear && value), (e) => e.child(this.button(`${id}-clear`, "No date", (ev, cx) => pick("", cx), { small: true, tone: "ghost" }))),
      );
  }

  /** The text field kept for id, taking a spare the first time; null when none is spare yet (the shell refills within a moment). */
  ownField(id, setup) {
    let f = this.fields[id];
    if (!f) {
      f = this.spare.pop();
      if (!f) {
        this.starved = true;
        return null;
      }
      setup(f);
      this.fields[id] = f;
    }
    f.usedAt = this.frame;
    return f;
  }

  /**
   * Commits what is typed in a kit field that still has the keyboard. A click
   * on a button does not take the keyboard from a field, so every kit button
   * calls this first: "30" typed in a minutes box counts when Save is pressed.
   */
  flush(cx) {
    for (const f of Object.values(this.fields)) if (f.focused && f.commit && f.current) f.commit(f.value, cx);
  }

  /** What a picker shows for the instant no text field is spare. */
  fieldStandIn(text, width) {
    const t = this.t;
    return div().h(32).w(width).px(10).flex().items_center().rounded(7).border_1().border_color(t.input).bg(t.background).text_size(13).text_color(t.foreground).child(text);
  }

  /**
   * A one-line text field for a value kept elsewhere, such as a setting:
   * shows value, and reports the new text when Enter is pressed or the field
   * is left. o: {width, mono, masked, live: true to report every keystroke}.
   */
  textInput(id, value, onCommit, o = {}) {
    const shown = value ?? "";
    const f = this.ownField(id, (field) => {
      field.set(shown);
      const commit = (text, cx) => {
        const cur = field.current;
        if (cur && text !== cur.value) {
          cur.value = text;
          cur.onCommit(text, cx);
        }
      };
      field.commit = commit;
      field.o = {
        onSubmit: commit,
        onBlur: commit,
        onChange: (text, cx) => {
          if (field.current?.o.live) commit(text, cx);
        },
      };
    });
    if (!f) return this.fieldStandIn(o.masked ? "" : shown, o.width ?? 220);
    f.current = { value: shown, onCommit, o };
    f.state.set_masked(Boolean(o.masked));
    if (!f.focused && f.value !== shown) f.set(shown);
    return f.el(this, { w: o.width, flex: o.width ? false : undefined, mono: o.mono });
  }

  /**
   * A time field: shows "9:30 am", takes 9, 930, 9:30 pm, 21:05, and reports
   * "HH:MM" when Enter is pressed or the field is left. value: "HH:MM" or "".
   * o: {empty: true to allow no time, width}.
   */
  timeInput(id, value, onCommit, o = {}) {
    const shown = value ? formatClock(minutesOf(value)) : "";
    const f = this.ownField(id, (field) => {
      field.set(shown);
      const commit = (text, cx) => {
        const cur = field.current;
        if (!cur) return;
        if (cur.o.empty && text.trim() === "") {
          if (cur.value !== "") {
            cur.value = "";
            cur.onCommit("", cx);
          }
          return;
        }
        const v = parseClock(text);
        if (v === null) field.set(cur.value ? formatClock(minutesOf(cur.value)) : "");
        else {
          field.set(formatClock(minutesOf(v)));
          if (v !== cur.value) {
            cur.value = v; // a later blur does not send it twice
            cur.onCommit(v, cx);
          }
        }
        cx.notify();
      };
      field.o = { onSubmit: commit, onBlur: commit };
      field.commit = commit;
    });
    if (!f) return this.fieldStandIn(shown, o.width ?? 96);
    f.current = { value, onCommit, o };
    if (!f.focused && f.value !== shown) f.set(shown);
    return f.el(this, { w: o.width ?? 96, flex: false });
  }

  /**
   * A whole number field. value: number or null. onCommit(number | null, cx)
   * on Enter or on leaving it. o: {min, max, width, unit, placeholder, empty}.
   */
  numberInput(id, value, onCommit, o = {}) {
    const shown = value === null || value === undefined ? "" : String(value);
    const f = this.ownField(id, (field) => {
      field.set(shown);
      const commit = (text, cx) => {
        const cur = field.current;
        if (!cur) return;
        const clean = text.trim();
        if (clean === "") {
          if (cur.o.empty) {
            if (cur.value !== null && cur.value !== undefined) {
              cur.value = null;
              cur.onCommit(null, cx);
            }
          } else field.set(cur.value === null || cur.value === undefined ? "" : String(cur.value));
          cx.notify();
          return;
        }
        let n = Number(clean);
        if (!Number.isFinite(n)) {
          field.set(cur.value === null || cur.value === undefined ? "" : String(cur.value));
          cx.notify();
          return;
        }
        if (!cur.o.decimal) n = Math.round(n);
        if (cur.o.min !== undefined) n = Math.max(cur.o.min, n);
        if (cur.o.max !== undefined) n = Math.min(cur.o.max, n);
        field.set(String(n));
        if (n !== cur.value) {
          cur.value = n;
          cur.onCommit(n, cx);
        }
        cx.notify();
      };
      field.o = { onSubmit: commit, onBlur: commit };
      field.commit = commit;
    });
    const el = f ? f.el(this, { w: o.width ?? 72, flex: false }) : this.fieldStandIn(shown, o.width ?? 72);
    if (f) {
      f.current = { value, onCommit, o };
      if (!f.focused && f.value !== shown) f.set(shown);
    }
    if (!o.unit) return el;
    return h_flex().items_center().gap(6).child(el).child(this.muted(o.unit));
  }

  /**
   * A duration as numbers with units, such as 7 h 30 m. value: a Go duration
   * string ("7h30m"). onCommit(goDuration, cx). o: {units: ["h", "m"]}.
   */
  durationInput(id, value, onCommit, o = {}) {
    const units = o.units ?? ["h", "m"];
    const unitMs = { h: 3_600_000, m: 60_000, s: 1000 };
    let rest = parseDuration(value || "0s");
    const parts = units.map((u, i) => {
      const n = i === units.length - 1 ? Math.round(rest / unitMs[u]) : Math.floor(rest / unitMs[u]);
      rest -= n * unitMs[u];
      return n;
    });
    return h_flex()
      .items_center()
      .gap(6)
      .children(
        units.map((u, i) =>
          this.numberInput(
            `${id}-${u}`,
            parts[i],
            (n, cx) => {
              const next = parts.map((p, j) => (j === i ? (n ?? 0) : p));
              onCommit(goDuration(next.reduce((sum, p, j) => sum + p * unitMs[units[j]], 0)), cx);
            },
            { min: 0, width: 56, unit: u },
          ),
        ),
      );
  }

  /** A length of work in minutes, shown as hours and minutes. onCommit(minutes | null, cx). */
  minutesInput(id, minutes, onCommit, o = {}) {
    const total = minutes ?? 0;
    return h_flex()
      .items_center()
      .gap(6)
      .child(this.numberInput(`${id}-h`, Math.floor(total / 60), (n, cx) => onCommit((n ?? 0) * 60 + (total % 60) || (o.empty ? null : 0), cx), { min: 0, width: 52, unit: "h" }))
      .child(this.numberInput(`${id}-m`, total % 60, (n, cx) => onCommit(Math.floor(total / 60) * 60 + (n ?? 0) || (o.empty ? null : 0), cx), { min: 0, max: 59, width: 52, unit: "m" }));
  }

  // Containers ----------------------------------------------------------------

  /** A card. o: {title, subtitle, actions (element), pad, gap, icon}. Add content with .child(). */
  panel(o = {}) {
    const t = this.t;
    const el = v_flex().w_full().rounded(10).border_1().border_color(t.border).bg(t.surface).p(o.pad ?? 16).gap(o.gap ?? 12);
    if (o.title || o.actions) {
      el.child(
        h_flex()
          .w_full()
          .items_center()
          .gap(8)
          .when(Boolean(o.icon), (e) => e.child(this.icon(o.icon, 15)))
          .child(
            v_flex()
              .flex_1()
              .min_w(0)
              .gap(2)
              .child(div().text_size(13.5).font_semibold().text_color(t.foreground).child(o.title ?? ""))
              .when(Boolean(o.subtitle), (e) => e.child(this.muted(o.subtitle))),
          )
          .when(Boolean(o.actions), (e) => e.child(o.actions)),
      );
    }
    return el;
  }

  /** The heading of a screen: title, a line under it, and actions on the right. */
  pageHeader(title, subtitle, actions) {
    const t = this.t;
    return h_flex()
      .w_full()
      .items_center()
      .gap(12)
      .child(
        v_flex()
          .flex_1()
          .min_w(0)
          .gap(3)
          .child(div().text_size(19).font_semibold().text_color(t.foreground).child(title))
          .when(Boolean(subtitle), (e) => e.child(typeof subtitle === "string" ? this.muted(subtitle) : subtitle)),
      )
      .when(Boolean(actions), (e) => e.child(actions));
  }

  /** A label over a control, with an optional hint under it. */
  field(label, control, hint) {
    return v_flex()
      .gap(5)
      .min_w(0)
      .child(div().text_size(12).font_medium().text_color(this.t.muted).child(label))
      .child(control)
      .when(Boolean(hint), (e) => e.child(this.dim(hint)));
  }

  /** A small pill. o: {tone, icon, tip, color}. */
  badge(label, o = {}) {
    const t = this.t;
    const color = o.color ?? (o.tone && o.tone !== "neutral" ? this.tone(o.tone) : t.muted);
    const el = h_flex()
      .flex_none()
      .items_center()
      .gap(4)
      .px(7)
      .h(20)
      .rounded(10)
      .bg(o.tone && o.tone !== "neutral" || o.color ? alpha(color, 0.16) : t.raised)
      .text_size(11.5)
      .text_color(color);
    if (o.icon) el.child(this.icon(o.icon, 11, color));
    el.child(String(label));
    if (o.tip) el.id(`badge-${++this.seq}`).tooltip(o.tip);
    return el;
  }

  /** A coloured dot. */
  dot(color, size = 8) {
    return div().w(size).h(size).flex_none().rounded(size / 2).bg(color);
  }

  /** A progress bar. fraction 0..1. o: {color, height, mark: 0..1 for a tick, track}. */
  meter(fraction, o = {}) {
    const t = this.t;
    const h = o.height ?? 6;
    const el = div()
      .relative()
      .w_full()
      .h(h)
      .rounded(h / 2)
      .bg(o.track ?? t.raised)
      .overflow_hidden()
      .child(div().h(h).rounded(h / 2).bg(o.color ?? t.accent).w(`${Math.round(clamp01(fraction) * 1000) / 10}%`));
    if (o.mark !== undefined && o.mark !== null) el.child(div().absolute().top(0).left(`${Math.round(clamp01(o.mark) * 1000) / 10}%`).w(2).h(h).bg(t.foreground));
    return el;
  }

  /** What a list shows when it has nothing. o: {icon, title, body, action (element)}. */
  empty(o) {
    const t = this.t;
    return v_flex()
      .w_full()
      .items_center()
      .gap(8)
      .py(28)
      .px(16)
      .when(Boolean(o.icon), (e) => e.child(this.icon(o.icon, 22, t.dim)))
      .child(div().text_size(13.5).font_medium().text_color(t.foreground).child(o.title))
      .when(Boolean(o.body), (e) => e.child(div().max_w(420).text_size(12.5).line_height("18px").text_color(t.muted).child(o.body)))
      .when(Boolean(o.action), (e) => e.child(div().pt(4).child(o.action)));
  }

  /** A notice. o: {tone: neutral | warn | danger | ok | accent, icon, text (string or element), action}. */
  callout(o) {
    const t = this.t;
    const color = o.tone && o.tone !== "neutral" ? this.tone(o.tone) : t.muted;
    return h_flex()
      .w_full()
      .items_center()
      .gap(10)
      .px(12)
      .py(9)
      .rounded(8)
      .border_1()
      .border_color(o.tone && o.tone !== "neutral" ? alpha(color, 0.4) : t.border)
      .bg(o.tone && o.tone !== "neutral" ? alpha(color, 0.08) : t.raised)
      .when(Boolean(o.icon), (e) => e.child(this.icon(o.icon, 15, color)))
      .child(div().flex_1().min_w(0).child(typeof o.text === "string" ? div().text_size(12.5).line_height("18px").text_color(t.foreground).child(o.text) : o.text))
      .when(Boolean(o.action), (e) => e.child(o.action));
  }

  /** A full-height area that scrolls its content. Give every one its own id. */
  scroll(id, content, o = {}) {
    return div()
      .id(id)
      .flex_1()
      .min_h(0)
      .w_full()
      .overflow_y_scrollbar()
      .child(v_flex().w_full().p(o.pad ?? 20).gap(o.gap ?? 16).when(Boolean(o.maxW), (e) => e.max_w(o.maxW)).children(Array.isArray(content) ? content.filter(Boolean) : [content]));
  }

  /** Centered text for a screen that is loading or has nothing to show. */
  splash(text, o = {}) {
    return v_flex()
      .size_full()
      .items_center()
      .justify_center()
      .gap(10)
      .when(Boolean(o.busy), (e) => e.child(this.spinner(16)))
      .child(div().text_size(13).text_color(this.t.dim).child(text));
  }

  // Dialogs ---------------------------------------------------------------------

  /**
   * Opens a dialog over this tab. d: {title, width, render(ui) -> element,
   * onClose(cx), actions(ui) -> [buttons]}. Returns its id.
   */
  openDialog(d, cx) {
    const id = `dlg-${++this.seq}`;
    this.dialogs.push({ id, ...d });
    this.popup = null;
    cx?.notify();
    return id;
  }

  /** Closes the top dialog, or the one with this id. */
  closeDialog(cx, id) {
    const i = id ? this.dialogs.findIndex((d) => d.id === id) : this.dialogs.length - 1;
    if (i < 0) return;
    const [d] = this.dialogs.splice(i, 1);
    this.popup = null;
    d.onClose?.(cx);
    cx?.notify();
  }

  /**
   * Asks before something that cannot be undone. Resolves true when confirmed.
   * o: {title, body, confirm: "Delete", danger: true, cancel: "Cancel"}.
   */
  confirm(o, cx) {
    return new Promise((resolve) => {
      let answered = false;
      const answer = (yes, cx) => {
        if (answered) return;
        answered = true;
        resolve(yes);
        this.closeDialog(cx, id);
      };
      const id = this.openDialog(
        {
          title: o.title,
          width: 420,
          render: (ui) => (o.body ? ui.muted(o.body, { size: 13, lh: 19 }) : div()),
          actions: (ui) => [
            ui.button("confirm-no", o.cancel ?? "Cancel", (e, cx) => answer(false, cx), { tone: "ghost" }),
            ui.button("confirm-yes", o.confirm ?? "OK", (e, cx) => answer(true, cx), { tone: o.danger ? "danger" : "primary" }),
          ],
          onClose: () => {
            if (!answered) {
              answered = true;
              resolve(false);
            }
          },
        },
        cx,
      );
    });
  }

  /** The open dialogs, drawn over the tab. The shell adds this last. */
  dialogLayer() {
    const t = this.t;
    if (this.dialogs.length === 0) return null;
    const d = this.dialogs[this.dialogs.length - 1];
    const card = v_flex()
      .id(`${d.id}-card`)
      .w(d.width ?? 520)
      .max_w("92%")
      .rounded(12)
      .border_1()
      .border_color(t.border)
      .bg(t.popover)
      .shadow_lg()
      .on_mouse_down("left", (e, cx) => cx.stop_propagation())
      .on_click((e, cx) => cx.stop_propagation())
      .child(
        h_flex()
          .flex_none()
          .items_center()
          .gap(8)
          .pl(18)
          .pr(10)
          .h(46)
          .border_b(1)
          .border_color(t.border)
          .child(div().flex_1().min_w(0).truncate().text_size(14).font_semibold().text_color(t.foreground).child(d.title ?? ""))
          .child(this.iconButton(`${d.id}-x`, "x", (e, cx) => this.closeDialog(cx, d.id), { tip: "Close" })),
      )
      .child(div().id(`${d.id}-body`).w_full().max_h(d.height ?? 540).overflow_y_scrollbar().child(v_flex().w_full().p(18).gap(14).child(d.render(this))));
    const actions = d.actions?.(this)?.filter(Boolean) ?? [];
    if (actions.length) card.child(h_flex().flex_none().justify_end().gap(8).px(18).py(12).border_t(1).border_color(t.border).children(actions));
    return div()
      .id(`${d.id}-backdrop`)
      .absolute()
      .inset_0()
      .flex()
      .items_center()
      .justify_center()
      .bg(alpha("#000000", 0.5))
      .on_mouse_down("left", (e, cx) => cx.stop_propagation())
      .on_mouse_down("right", (e, cx) => cx.stop_propagation())
      .on_scroll_wheel((e, cx) => cx.stop_propagation())
      .on_click((e, cx) => {
        if (!d.modal) this.closeDialog(cx, d.id);
      })
      .child(card);
  }
}

export { CLEAR, DANGER, WARN, OK, dateOf };
