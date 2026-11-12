// A secret kept in Gwen's credentials folder, readable only by this user.
// The daemon writes it and only ever says whether one is stored.
//
//   this.key = new Credential(app, "llm_api_key", "API key", { hint, onSaved(cx) });   // in a constructor
//   this.key.load(cx)                                                                  // in sync(cx)
//   this.key.el(ui)                                                                    // in render
//   this.key.has                                                                       // true, false, or null until known

import { v_flex, h_flex } from "gpui-base";
import { api, gwen } from "../../lib/gwen.js";
import { TextField } from "../../lib/kit.js";

/** Which credential files are stored: {name: bool}. Shared by every field; reload with loadCredentials(). */
let stored = null;
let loading = null;

export function loadCredentials() {
  loading ??= api
    .Credentials()
    .then((r) => {
      stored = r.stored ?? {};
    })
    .catch(() => {})
    .finally(() => {
      loading = null;
    });
  return loading;
}

/** Whether a credential is stored; null until loadCredentials() has answered. */
export const hasCredential = (name) => (stored ? Boolean(stored[name]) : null);

export class Credential {
  constructor(app, name, label, o = {}) {
    this.app = app;
    this.name = name;
    this.label = label;
    this.o = o;
    this.saving = false;
    this.field = new TextField({ masked: true, placeholder: "Paste it here", onSubmit: (v, cx) => this.save(cx) });
  }

  get has() {
    return hasCredential(this.name);
  }

  /** Asks the daemon which credentials are stored, and redraws when it answers. */
  load(cx) {
    cx.spawn(async (cx) => {
      await loadCredentials();
      cx.notify();
    });
  }

  save(cx) {
    const value = this.field.value.trim();
    if (!value || this.saving) return;
    this.saving = true;
    this.app.spawn(cx, async (cx) => {
      try {
        await api.SetCredential(this.name, value);
        this.field.set("");
        if (stored) stored[this.name] = true;
        gwen.toast(`Saved the ${this.label}`, "success");
        this.o.onSaved?.(cx);
      } finally {
        this.saving = false;
      }
    });
  }

  /** The label, a Saved badge once one is stored, the hint, and a masked field with Save. */
  el(ui) {
    const t = ui.t;
    return v_flex()
      .gap(8)
      .child(
        h_flex()
          .items_center()
          .gap(8)
          .child(ui.text(this.label, { size: 13.5, medium: true }))
          .when(this.has === true, (e) => e.child(ui.badge("Saved", { tone: "ok", icon: "circle-check" }))),
      )
      .when(Boolean(this.o.hint), (e) => e.child(typeof this.o.hint === "string" ? ui.muted(this.o.hint) : this.o.hint))
      .child(
        h_flex()
          .gap(8)
          .child(this.field.el(ui))
          .child(ui.button(`cred-save-${this.name}`, "Save", (e, cx) => this.save(cx), { disabled: !this.field.value.trim(), busy: this.saving })),
      )
      .when(this.has === true, (e) => e.child(ui.dim("Type a new one to replace it.")));
  }
}
