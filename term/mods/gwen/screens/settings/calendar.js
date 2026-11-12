// Settings › Google Calendar, set up end to end without the command line:
// Gwen writes your plan to a calendar of its own and reads the events of the
// calendars you choose, so plans fit around them.

import { div } from "gpui-kit";
import { v_flex, h_flex } from "gpui-base";
import { api, apiError, gwen } from "../../lib/gwen.js";
import { host } from "../../lib/host.js";
import { formatAgo } from "../../lib/format.js";
import { TextField } from "../../lib/kit.js";
import { hasCredential, loadCredentials } from "./credential.js";
import { numberedStep, statusCard, withoutCLI } from "./connect-parts.js";

export default class Calendar {
  constructor(app) {
    this.app = app;
    this.status = null;
    this.unavailable = null;
    this.busy = null;
    this.loaded = null;
    this.calendars = null; // the account's calendars, once connected
    this.listError = null;
    this.listedAt = null;
    this.draftFrom = null;
    this.busyList = [];
    this.name = new TextField({ onChange: (v, cx) => cx.notify() });
    this.adding = new TextField({ placeholder: "Shared calendar's address", onSubmit: (v, cx) => this.add(cx), onChange: (v, cx) => cx.notify() });
  }

  get connected() {
    return Boolean(gwen.config?.calendar?.enabled && this.status?.connected);
  }

  sync(cx) {
    const cal = gwen.config?.calendar;
    if (!cal) return;
    const draft = JSON.stringify([cal.name, cal.busy_calendars ?? []]);
    if (this.draftFrom !== draft) {
      this.draftFrom = draft;
      if (!this.name.focused) this.name.set(cal.name ?? "");
      this.busyList = [...(cal.busy_calendars ?? [])];
    }
    const stamp = `${gwen.versions.integration}/${cal.enabled}`;
    if (this.loaded !== stamp) {
      this.loaded = stamp;
      this.load(cx);
    }
  }

  load(cx) {
    cx.spawn(async (cx) => {
      await loadCredentials();
      try {
        this.status = await api.CalendarStatus();
        this.unavailable = null;
      } catch (e) {
        const err = apiError(e);
        this.status = null;
        if (err.code === "unavailable") this.unavailable = err.message;
        else gwen.fail(e);
      }
      if (this.connected && this.listedAt !== this.loaded) {
        this.listedAt = this.loaded;
        try {
          this.calendars = (await api.CalendarCalendars()).calendars ?? [];
          this.listError = null;
        } catch (e) {
          this.calendars = null;
          this.listError = withoutCLI(apiError(e).message);
        }
      }
      cx.notify();
    });
  }

  step(name, cx, body) {
    this.busy = name;
    cx.notify();
    this.app.spawn(cx, async (cx) => {
      try {
        await body(cx);
      } finally {
        this.busy = null;
      }
    });
  }

  chooseClient(cx) {
    this.step("client", cx, async (cx) => {
      if (await host.chooseGoogleClient()) {
        await loadCredentials();
        this.load(cx);
        gwen.toast("Installed the OAuth client. Now connect your Google account.", "success");
      }
    });
  }

  connect(cx) {
    this.step("connect", cx, async (cx) => {
      const auth = await api.CalendarAuthStart();
      if (auth?.auth_url) cx.open_url(auth.auth_url);
    });
  }

  syncNow(cx) {
    this.step("sync", cx, async () => {
      const s = await api.CalendarSync();
      this.status = s;
      if (!s.last_error) gwen.toast("Synced with Google Calendar", "success");
    });
  }

  add(cx) {
    const v = this.adding.value.trim();
    if (v && !this.busyList.includes(v)) this.busyList.push(v);
    this.adding.set("");
    cx.notify();
  }

  save(cx) {
    this.app.spawn(cx, async () => {
      if (await gwen.act(() => api.PatchConfig({ calendar: { name: this.name.value.trim(), busy_calendars: this.busyList } }))) gwen.toast("Saved calendar settings", "success");
    });
  }

  main(ui) {
    const enabled = gwen.config.calendar.enabled;
    const hasClient = hasCredential("google_client.json");
    const st = this.status;
    const panel = ui.panel({ title: "Google Calendar", icon: "calendar-days" }).child(ui.muted("Gwen writes your plan to a calendar of its own and reads the events of the calendars you choose, so plans fit around them.", { size: 13 }));
    if (this.connected && st)
      panel.child(
        statusCard(ui, {
          icon: "circle-check",
          title: "Connected",
          detail: st.last_sync_at != null ? `Last synced ${formatAgo(st.last_sync_at, Date.now())}` : "Not synced yet",
          actions: [
            ui.button("cal-reconnect", "Reconnect", (e, cx) => this.connect(cx), { small: true, busy: this.busy === "connect" }),
            ui.button("cal-sync", "Sync now", (e, cx) => this.syncNow(cx), { small: true, tone: "primary", icon: "refresh-cw", busy: this.busy === "sync" }),
          ],
        }),
      );
    else if (hasClient === false)
      panel.child(
        v_flex()
          .gap(14)
          .child(
            numberedStep(
              ui,
              1,
              "Google needs every app to have its own sign-in client, so you make one once. In Google Cloud Console, create a project and turn on the Google Calendar API.",
              h_flex().child(ui.button("cal-console", "Open Google Cloud Console", (e, cx) => cx.open_url("https://console.cloud.google.com/apis/library/calendar-json.googleapis.com"), { small: true, icon: "external-link" })),
            ),
          )
          .child(numberedStep(ui, 2, "Set up the OAuth consent screen as External, add yourself as a test user, then publish the app. Unverified is fine for personal use; unpublished, Google signs you out every 7 days."))
          .child(numberedStep(ui, 3, "Create an OAuth client of type Desktop app and download its JSON file."))
          .child(numberedStep(ui, 4, "Choose that file here.", h_flex().child(ui.button("cal-client", "Choose client file", (e, cx) => this.chooseClient(cx), { small: true, tone: "primary", icon: "file-json", busy: this.busy === "client" }))))
          .child(numberedStep(ui, 5, "Connect, and sign in with Google in the browser window that opens.")),
      );
    else if (hasClient && !enabled)
      panel.child(
        ui.callout({
          text: "The calendar is off. Your OAuth client is installed, so turning it on is all it takes.",
          action: ui.button("cal-on", "Turn on", (e, cx) => this.app.spawn(cx, () => gwen.act(() => api.PatchConfig({ calendar: { enabled: true } }))), { small: true, tone: "primary" }),
        }),
      );
    else if (hasClient) {
      panel.child(
        ui.callout({
          tone: "accent",
          text: "Your OAuth client is installed. Connect, and sign in with Google in the browser window that opens.",
          action: ui.button("cal-connect", "Connect Google Calendar", (e, cx) => this.connect(cx), { small: true, tone: "primary", busy: this.busy === "connect" }),
        }),
      );
      if (this.unavailable) panel.child(ui.dim(withoutCLI(this.unavailable)));
    } else panel.child(ui.muted("Checking…", { size: 13 }));
    if (st?.last_error) panel.child(ui.callout({ tone: "danger", icon: "circle-alert", text: withoutCLI(st.last_error) }));
    if (hasClient)
      panel.child(
        h_flex()
          .flex_wrap()
          .items_center()
          .gap(12)
          .pt(12)
          .border_t(1)
          .border_color(ui.t.border)
          .child(div().flex_1().child(ui.dim("Using a different Google Cloud project? Install its client file instead.")))
          .child(ui.button("cal-replace", "Replace client file", (e, cx) => this.chooseClient(cx), { small: true, icon: "file-json", busy: this.busy === "client" })),
      );
    return panel;
  }

  options(ui) {
    const t = ui.t;
    const cal = gwen.config.calendar;
    const listed = new Set((this.calendars ?? []).map((c) => c.id));
    // Busy calendars the list does not hold: added by address, or every one while the list is unknown.
    const others = this.busyList.filter((b) => !listed.has(b));
    const changed = this.name.value !== (cal.name ?? "") || this.busyList.join("\n") !== (cal.busy_calendars ?? []).join("\n");
    const busy = v_flex()
      .gap(8)
      .py(12)
      .child(div().text_size(13.5).child("Busy calendars"))
      .child(ui.muted("Their events show on your plan and take their time out of it. All-day events count only when shown as busy."));
    if (this.listError)
      busy.child(ui.callout({ text: this.listError, action: ui.button("cal-list-reconnect", "Reconnect", (e, cx) => this.connect(cx), { small: true, tone: "primary", busy: this.busy === "connect" }) }));
    if (this.calendars)
      busy.child(
        v_flex()
          .gap(2)
          .children(
            this.calendars.map((c) =>
              h_flex()
                .items_center()
                .gap(10)
                .px(4)
                .py(4)
                .child(
                  ui.checkbox(
                    `cal-busy-${c.id}`,
                    this.busyList.includes(c.id),
                    (on, cx) => {
                      this.busyList = on ? [...this.busyList, c.id] : this.busyList.filter((x) => x !== c.id);
                      cx.notify();
                    },
                    { color: c.color || undefined },
                  ),
                )
                .child(div().flex_1().min_w(0).truncate().text_size(13).child(c.name))
                .when(Boolean(c.primary), (e) => e.child(ui.badge("Main calendar"))),
            ),
          ),
      );
    busy.child(
      h_flex()
        .flex_wrap()
        .items_center()
        .gap(8)
        .children(
          others.map((b) =>
            h_flex()
              .items_center()
              .gap(4)
              .pl(10)
              .pr(3)
              .h(26)
              .rounded(13)
              .bg(t.raised)
              .text_size(12.5)
              .child(b === "primary" ? "Main calendar" : b)
              .child(
                ui.iconButton(
                  `cal-rm-${b}`,
                  "x",
                  (e, cx) => {
                    this.busyList = this.busyList.filter((x) => x !== b);
                    cx.notify();
                  },
                  { size: 20, tip: `Remove ${b}` },
                ),
              ),
          ),
        )
        .child(this.adding.el(ui, { w: 240, h: 28 }))
        .child(ui.button("cal-add", "Add", (e, cx) => this.add(cx), { small: true, icon: "plus", disabled: !this.adding.value.trim() })),
    );
    return ui
      .panel({ title: "Calendar options" })
      .child(ui.toggleRow("cal-enabled", "Use Google Calendar", "Off stops syncing but keeps your sign-in.", cal.enabled, (on, cx) => this.app.spawn(cx, () => gwen.act(() => api.PatchConfig({ calendar: { enabled: on } })))))
      .child(ui.row("Gwen's calendar", "The calendar your plan is written to. Gwen creates it.", this.name.el(ui, { w: 240 })))
      .child(busy)
      .child(h_flex().justify_end().pt(12).border_t(1).border_color(t.border).child(ui.button("cal-save", "Save changes", (e, cx) => this.save(cx), { tone: "primary", disabled: !changed || !this.name.value.trim() })));
  }

  render(ui) {
    if (!gwen.config) return ui.splash("Loading…", { busy: true });
    const parts = [this.main(ui)];
    if (hasCredential("google_client.json")) parts.push(this.options(ui));
    return parts;
  }
}
