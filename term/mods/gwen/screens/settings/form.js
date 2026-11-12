// What the settings sections share: the generic form over one part of the
// configuration, saving a patch, the pick-one cards, and a download's progress.
//
//   export default class Tracking extends ConfigForm {
//     constructor(app) {
//       super(app, { title: "Tracking", description: "…", name: "tracking", fields: [{ key, label, hint, kind }] });
//     }
//   }
//
// A field's kind is {type: "duration", units?}, {type: "time"}, {type: "bool"},
// {type: "text", placeholder?} or {type: "select", options: [{value, label}]}.

import { div } from "gpui-kit";
import { v_flex, h_flex } from "gpui-base";
import { api, apiError, gwen } from "../../lib/gwen.js";
import { alpha } from "../../lib/kit.js";

/**
 * Saves a patch of the configuration ({section: {key: value}}) and shows the
 * daemon's answer at once, without waiting for its change event. Throws what
 * the daemon refused.
 */
export async function patchConfig(patch) {
  const config = await api.PatchConfig(patch);
  if (config?.tracking) {
    gwen.config = config;
    gwen.versions.config++;
    gwen.bump();
  }
  return config;
}

/** As patchConfig, reporting a failure as a toast: the new configuration, or undefined when it failed. */
export async function saveConfig(patch) {
  try {
    return await patchConfig(patch);
  } catch (e) {
    gwen.fail(e);
    return undefined;
  }
}

/** The text of a failure, for a notice inside a panel. */
export const errorText = (e) => String(apiError(e).message || e);

/**
 * A card that is one of a few choices: an icon or a swatch, a title with
 * badges, and a line about it. o: {icon, lead (element instead of the icon),
 * title, badges: [elements], body, on, onClick(e, cx), width}.
 */
export function choiceCard(ui, id, o) {
  const t = ui.t;
  const el = h_flex()
    .id(id)
    .items_start()
    .gap(12)
    .p(13)
    .rounded(8)
    .border_1()
    .border_color(o.on ? t.accent : t.input)
    .bg(o.on ? alpha(t.accent, 0.1) : t.raised)
    .cursor_pointer()
    .on_click(o.onClick);
  if (!o.on) el.hover((s) => s.bg(t.hover));
  if (o.width) el.w(o.width);
  else el.flex_1().min_w(0);
  if (o.lead) el.child(o.lead);
  else if (o.icon) el.child(div().pt(1).child(ui.icon(o.icon, 18, o.on ? t.accent : t.muted)));
  return el.child(
    v_flex()
      .flex_1()
      .min_w(0)
      .gap(3)
      .child(
        h_flex()
          .items_center()
          .gap(8)
          .child(div().text_size(13.5).font_medium().text_color(t.foreground).child(o.title))
          .children((o.badges ?? []).filter(Boolean)),
      )
      .when(Boolean(o.body), (e) => e.child(ui.dim(o.body))),
  );
}

/** Cards laid out two to a row. */
export function cardGrid(cards) {
  const rows = [];
  for (let i = 0; i < cards.length; i += 2) {
    const row = h_flex().w_full().items_stretch().gap(10).child(cards[i]);
    row.child(cards[i + 1] ?? div().flex_1().min_w(0));
    rows.push(row);
  }
  return v_flex().w_full().gap(10).children(rows);
}

/** A model download as a person reads it: "42 of 99 MB". progress: {done, total} bytes, or null. */
export function downloadProgress(ui, progress, size, what = "the voice model") {
  const t = ui.t;
  const mb = (n) => Math.round(n / 2 ** 20);
  const known = progress && progress.total > 0;
  return v_flex()
    .w_full()
    .gap(8)
    .child(
      h_flex()
        .items_center()
        .gap(8)
        .child(ui.spinner(14, t.accent))
        .child(div().flex_1().min_w(0).text_size(13).text_color(t.muted).child(`Downloading ${what}`))
        .child(div().flex_none().text_size(12).text_color(t.dim).child(known ? `${mb(progress.done)} of ${mb(progress.total)} MB` : `${size} MB`)),
    )
    .child(ui.meter(known ? progress.done / progress.total : 0, { height: 4 }));
}

/** One part of the configuration: a draft of its values, saved as a patch of the changed keys. */
export class ConfigForm {
  /** o: {title, description, icon, name (the config section), fields}. */
  constructor(app, o) {
    this.app = app;
    this.o = o;
    this.draft = {}; // the keys changed here and not saved yet
    this.errors = {}; // per key: why the daemon refused it
    this.saving = false;
    this.stamp = "";
  }

  get current() {
    return gwen.config?.[this.o.name] ?? {};
  }

  value(key) {
    return key in this.draft ? this.draft[key] : this.current[key];
  }

  changed() {
    return this.o.fields.filter((f) => f.key in this.draft && this.draft[f.key] !== this.current[f.key]);
  }

  set(key, value, cx) {
    this.draft[key] = value;
    cx.notify();
  }

  revert(cx) {
    this.draft = {};
    this.errors = {};
    cx.notify();
  }

  save(cx) {
    const { name, title } = this.o;
    const changed = this.changed();
    if (changed.length === 0 || this.saving) return;
    const patch = {};
    for (const f of changed) patch[f.key] = this.draft[f.key];
    this.saving = true;
    cx.notify();
    cx.spawn(async (cx) => {
      try {
        await patchConfig({ [name]: patch });
        this.draft = {};
        this.errors = {};
        gwen.toast(`Saved ${title.toLowerCase()} settings`, "success");
      } catch (e) {
        const err = apiError(e);
        const key = typeof err.details.key === "string" ? err.details.key : "";
        if (key.startsWith(`${name}.`)) this.errors = { [key.slice(name.length + 1)]: err.message.replace(`${key}: `, "") };
        else gwen.fail(e);
      }
      this.saving = false;
      cx.notify();
    });
  }

  control(ui, f) {
    const id = `cf-${this.o.name}-${f.key}`;
    const value = this.value(f.key);
    const change = (v, cx) => this.set(f.key, v, cx);
    switch (f.kind.type) {
      case "duration":
        return ui.durationInput(id, String(value ?? "0s"), change, { units: f.kind.units });
      case "time":
        return ui.timeInput(id, String(value ?? ""), change, { width: 96 });
      case "select":
        return ui.select(id, f.kind.options, String(value), change, { width: 208 });
      default:
        return ui.textInput(id, String(value ?? ""), change, { width: 288, live: true });
    }
  }

  fieldRow(ui, f) {
    const error = this.errors[f.key];
    if (f.kind.type === "bool" && !error) return ui.toggleRow(`cf-${this.o.name}-${f.key}`, f.label, f.hint, Boolean(this.value(f.key)), (on, cx) => this.set(f.key, on, cx));
    const about =
      f.hint || error
        ? v_flex()
            .gap(4)
            .when(Boolean(f.hint), (e) => e.child(ui.muted(f.hint)))
            .when(Boolean(error), (e) => e.child(ui.text(error, { size: 12, color: ui.tone("danger"), lh: 17 })))
        : null;
    const control = f.kind.type === "bool" ? ui.toggle(`cf-${this.o.name}-${f.key}`, Boolean(this.value(f.key)), (on, cx) => this.set(f.key, on, cx)) : this.control(ui, f);
    return ui.row(f.label, about, control);
  }

  render(ui) {
    const { title, description, icon, fields } = this.o;
    // The section changed under the draft (saved here, or from the command line): show what is stored.
    const stamp = JSON.stringify(this.current);
    if (stamp !== this.stamp) {
      this.stamp = stamp;
      this.draft = {};
    }
    const dirty = this.changed().length > 0;
    return ui
      .panel({ title, subtitle: description, icon, gap: 4 })
      .child(v_flex().w_full().children(fields.map((f) => this.fieldRow(ui, f))))
      .child(
        h_flex()
          .w_full()
          .pt(12)
          .justify_end()
          .gap(8)
          .when(dirty, (e) => e.child(ui.button(`cf-${this.o.name}-revert`, "Revert", (ev, cx) => this.revert(cx), { tone: "ghost" })))
          .child(ui.button(`cf-${this.o.name}-save`, "Save changes", (ev, cx) => this.save(cx), { tone: "primary", disabled: !dirty, busy: this.saving })),
      );
  }
}
