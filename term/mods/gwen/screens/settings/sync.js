import { v_flex, h_flex } from "gpui-base";
import { api, apiError, gwen } from "../../lib/gwen.js";
import { formatAgo } from "../../lib/format.js";
import { Credential } from "./credential.js";
import { patchConfig } from "./form.js";

const hubAddress = (value) => value.trim().replace(/\/+$/, "");

export default class Sync {
  constructor(app) {
    this.app = app;
    this.hubURL = gwen.config?.sync?.hub_url ?? "";
    this.every = gwen.config?.sync?.interval ?? "5m";
    this.status = null;
    this.syncing = false;
    this.saving = false;
    this.loaded = {};
    this.statusRequest = 0;
    this.token = new Credential(app, "sync_token", "Sync token", {
      hint: "The token the hub printed when it was set up.",
      onSaved: (cx) => {
        this.loadStatus(cx);
        cx.notify();
      },
    });
  }

  sync(cx) {
    const config = gwen.config?.sync;
    if (!config) return;
    if (this.loaded.hubURL !== config.hub_url) {
      this.loaded.hubURL = config.hub_url;
      this.hubURL = config.hub_url;
    }
    if (this.loaded.every !== config.interval) {
      this.loaded.every = config.interval;
      this.every = config.interval;
    }
    const stamp = `${gwen.versions.integration}/${config.hub_url}`;
    if (this.loaded.status !== stamp) {
      this.loaded.status = stamp;
      this.loadStatus(cx);
      this.token.load(cx);
    }
  }

  loadStatus(cx) {
    const request = ++this.statusRequest;
    this.app.spawn(cx, async () => {
      try {
        const status = await api.SyncStatus();
        if (request === this.statusRequest) this.status = status;
      } catch (error) {
        if (request !== this.statusRequest) return;
        if (apiError(error).code === "unavailable") this.status = null;
        else throw error;
      }
    });
  }

  save(cx) {
    if (this.saving) return;
    const hub_url = hubAddress(this.hubURL);
    const interval = this.every;
    this.saving = true;
    this.app.spawn(cx, async (cx) => {
      try {
        await patchConfig({ sync: { hub_url, interval } });
        this.hubURL = hub_url;
        gwen.toast("Saved sync settings", "success");
        this.loadStatus(cx);
      } finally {
        this.saving = false;
      }
    });
  }

  syncNow(cx) {
    if (this.syncing) return;
    this.syncing = true;
    const request = ++this.statusRequest;
    this.app.spawn(cx, async () => {
      try {
        const status = await api.SyncNow();
        if (request === this.statusRequest) this.status = status;
        if (!status.last_error) gwen.toast("Synced with the hub", "success");
      } finally {
        this.syncing = false;
      }
    });
  }

  get tick() {
    return Boolean(this.status?.configured);
  }

  render(ui) {
    const config = gwen.config?.sync;
    const status = this.status;
    const now = gwen.now();
    const changed = hubAddress(this.hubURL) !== (config?.hub_url ?? "") || this.every !== (config?.interval ?? "5m");
    const panel = ui
      .panel({ title: "Sync with your hub", icon: "cloud" })
      .child(ui.muted("Copies your data to the hub on your Raspberry Pi, where your phone can open the dashboard and a new install can restore everything.", { size: 13, lh: 19 }));
    if (status?.configured) {
      panel.child(h_flex().flex_wrap().items_center().gap(12).p(12).rounded(8).border_1().border_color(ui.t.border).bg(ui.t.raised)
        .child(ui.icon("circle-check", 18, status.last_error ? ui.t.dim : ui.tone("ok")))
        .child(v_flex().flex_1().min_w(180).gap(3)
          .child(ui.text(config?.hub_url ?? "", { medium: true }).id("sync-hub-address").truncate().tooltip(config?.hub_url ?? ""))
          .child(ui.dim(`Sent ${status.last_push_at != null ? formatAgo(status.last_push_at, now) : "never"}, received ${status.last_pull_at != null ? formatAgo(status.last_pull_at, now) : "never"}`)),
        )
        .child(ui.button("sync-open-hub", "Open hub dashboard", (event, cx) => {
          cx.open_url(`${hubAddress(gwen.config.sync.hub_url)}/`);
          cx.notify();
        }, { small: true, icon: "external-link" }))
        .child(ui.button("sync-now", "Sync now", (event, cx) => {
          this.syncNow(cx);
          cx.notify();
        }, { small: true, tone: "primary", icon: "refresh-cw", busy: this.syncing })),
      );
    } else {
      panel.child(ui.callout({ text: "Not set up yet. Run the hub on your Pi, then give its address and token here." }));
    }
    if (status?.last_error) panel.child(ui.callout({ tone: "danger", text: status.last_error }));
    return panel
      .child(ui.row("Hub address", "Such as http://raspberrypi:7777.", ui.textInput("sync-hub-url", this.hubURL, (value, cx) => {
        this.hubURL = value;
        cx.notify();
      }, { width: 288, live: true }).accessibility_label("Hub address")))
      .child(this.token.el(ui))
      .child(ui.row("Sync every", null, ui.durationInput("sync-interval", this.every, (value, cx) => {
        this.every = value;
        cx.notify();
      }, { units: ["h", "m"] })))
      .child(h_flex().w_full().justify_end().pt(4).child(ui.button("sync-save", "Save changes", (event, cx) => {
        this.save(cx);
        cx.notify();
      }, { tone: "primary", busy: this.saving, disabled: !changed })));
  }
}
