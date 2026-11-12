import { v_flex, h_flex } from "gpui-base";
import { api, gwen } from "../../lib/gwen.js";
import { host } from "../../lib/host.js";

const activityNames = {
  wayland: "Wayland",
  swayidle: "Sway idle detection",
  dbus_screensaver: "Desktop screensaver",
  xprintidle: "X11 idle detection",
  none: "Not available",
};

export default class About {
  constructor(app) {
    this.app = app;
    this.health = null;
    this.autostart = null;
    this.saving = false;
    this.loaded = false;
  }

  sync(cx) {
    if (this.loaded) return;
    this.loaded = true;
    this.app.spawn(cx, async () => {
      this.health = await api.Health();
    });
    this.app.spawn(cx, async () => {
      this.autostart = await host.autostart();
    });
  }

  setAutostart(on, cx) {
    if (this.saving) return;
    this.saving = true;
    this.app.spawn(cx, async () => {
      try {
        await host.setAutostart(on);
        this.autostart = on;
      } finally {
        this.saving = false;
      }
    });
  }

  render(ui) {
    const health = this.health;
    const backend = gwen.status?.activity_backend;
    const row = (label, value) => h_flex()
      .items_center()
      .gap(12)
      .child(ui.muted(label).w(160).flex_none())
      .child(value.flex_1().min_w(0));
    return ui
      .panel({ title: "About", icon: "info" })
      .child(ui.toggleRow(
        "about-autostart",
        "Show the tray icon at login",
        "The tray holds the same controls as the status box in the sidebar. On KDE, the Gwen panel widget takes its place when it is on a panel.",
        this.autostart === true,
        (on, cx) => {
          this.setAutostart(on, cx);
          cx.notify();
        },
        { disabled: this.autostart === null || this.saving },
      ))
      .child(v_flex().gap(10).py(6)
        .child(row("Background service", h_flex().items_center().gap(8)
          .child(ui.text(health?.version ?? "…"))
          .when(Boolean(health?.ok), (el) => el.child(ui.badge("Running", { tone: "ok", icon: "circle-check" }))),
        ))
        .child(row("Database schema", ui.text(health?.schema_version ?? "…")))
        .child(row("Activity detection", ui.text(backend ? activityNames[backend] ?? "Unknown" : "…"))),
      );
  }
}
