// Connections: what Gwen can reach, and how. Google Calendar, nudges on the
// phone, and the sync hub reuse their Settings sections; Gmail and running
// coding tasks are still to come.

import { div } from "gpui-kit";
import { h_flex } from "gpui-base";
import Calendar from "./settings/calendar.js";
import Phone from "./settings/phone.js";
import Sync from "./settings/sync.js";

export default class Connections {
  constructor(app, cx) {
    this.app = app;
    this.calendar = new Calendar(app, cx);
    this.phone = new Phone(app, cx);
    this.sync_ = new Sync(app, cx);
  }

  sync(cx) {
    this.calendar.sync?.(cx);
    this.phone.sync?.(cx);
    this.sync_.sync?.(cx);
  }

  get tick() {
    return Boolean(this.phone.tick || this.sync_.tick);
  }

  notBuilt(ui, icon, title, body, action) {
    return ui
      .panel({ title, icon, actions: ui.badge("Not built yet") })
      .child(ui.muted(body, { size: 13 }))
      .when(Boolean(action), (e) => e.child(h_flex().child(action)));
  }

  render(ui, cx) {
    const flat = (x) => (Array.isArray(x) ? x : [x]);
    return ui.scroll("cn-scroll", [
      ui.pageHeader("Connections", "What Gwen can reach, and how."),
      ...flat(this.calendar.render(ui, cx)),
      ...flat(this.phone.ntfyPanels(ui)),
      ui
        .panel({ title: "Gwen on your phone and Claude sessions", icon: "smartphone" })
        .child(ui.muted("Reach your Claude sessions from your phone over your tailnet, and let Gwen see what they ask and change.", { size: 13 }))
        .child(h_flex().child(ui.button("cn-phone", "Open phone settings", () => this.app.go("settings", null, "phone"), { icon: "settings" }))),
      ...flat(this.sync_.render(ui, cx)),
      this.notBuilt(ui, "mail", "Gmail", "Reading your latest emails and suggesting replies, which you send or Gwen sends with your OK. Coming in a later version."),
      this.notBuilt(ui, "code-2", "Coding tasks", "Handing a coding task to Claude Code in a project folder, with a plan you discuss first and changes you approve. Coming in a later version."),
    ]);
  }
}
