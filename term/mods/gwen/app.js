// The shell every Gwen tab shares (plan.md 3.15): the status line at the
// top, the sub-screen tabs, the screen itself, and what shows while gwend is
// not running. A tab is one copy of this mod with props {screen}.
//
// A screen is a plain class:
//
//   constructor(app, cx)   make text fields and first state here; cx may start
//                          timers and tasks (cx.timer.every, cx.spawn), never keep it
//   subs                   optional [{id, label, icon}]: the tabs inside this screen
//   sync(cx)               shared data changed: compare gwen.versions with what
//                          was loaded and reload; also called once at the start
//   show(sub, param, cx)   optional: this tab was asked for from elsewhere
//   render(ui, cx)         the screen's element
//   tick                   optional true: re-render every second while visible
//
// and reaches the rest through `app`: app.ui, app.sub, app.go(screen, sub, param),
// app.spawn(cx, async (cx) => ...).

import { View, div } from "gpui-kit";
import { v_flex, h_flex } from "gpui-base";
import { props, nextCommand, spaces } from "herdr";
import { gwen, api, tracking } from "./lib/gwen.js";
import { host } from "./lib/host.js";
import { Ui, stateColor } from "./lib/kit.js";
import { clockFace, formatDuration } from "./lib/format.js";
import Today from "./screens/today.js";
import Chat from "./screens/gwen.js";
import Tasks from "./screens/tasks.js";
import Plan from "./screens/plan.js";
import Review from "./screens/review.js";
import Connections from "./screens/connections.js";
import Memory from "./screens/memory.js";
import Settings from "./screens/settings.js";

/** Gwen's tabs: the screen each shows, and the tab's name in the tab bar. */
export const SCREENS = {
  today: { title: "Today", make: (app, cx) => new Today(app, cx) },
  gwen: { title: "Gwen", make: (app, cx) => new Chat(app, cx) },
  tasks: { title: "Tasks", make: (app, cx) => new Tasks(app, cx) },
  plan: { title: "Plan", make: (app, cx) => new Plan(app, cx) },
  review: { title: "Review", make: (app, cx) => new Review(app, cx) },
  connections: { title: "Connections", make: (app, cx) => new Connections(app, cx) },
  memory: { title: "Memory", make: (app, cx) => new Memory(app, cx) },
  settings: { title: "Settings", make: (app, cx) => new Settings(app, cx) },
};

/** Old screen names, as links inside screens and the tray use them: the tab and its sub-screen. */
const ALIASES = { assistant: ["gwen", ""], inbox: ["tasks", "inbox"], board: ["tasks", "board"], projects: ["tasks", "projects"], goals: ["plan", "goals"], weekly: ["review", "weekly"], stats: ["review", "stats"], history: ["review", "history"] };

/** What a tab shows when its screen threw: the error, so it can be reported and fixed. */
class Broken {
  constructor(error) {
    this.error = error;
  }
  render(ui) {
    return v_flex()
      .size_full()
      .items_center()
      .justify_center()
      .p(24)
      .child(div().w(560).max_w("100%").child(ui.callout({ tone: "danger", icon: "circle-alert", text: `This screen hit an error and could not be drawn: ${String(this.error?.message ?? this.error)}` })));
  }
}

const dockLabel = { off: "Not clocked in", working: "Working", idle_pending: "Idle", break_auto: "On break", break_manual: "On break" };

export default class GwenApp extends View {
  init(_props, cx) {
    this.screenId = SCREENS[props()?.screen] ? props().screen : "today";
    this.ui = new Ui();
    this.handle = cx.focus_handle();
    this.sub = null;
    // A tab that was just made because another tab asked for it takes that request too.
    const nav = gwen.navigate;
    this.navSeen = nav && nav.screen === this.screenId && Date.now() - nav.at < 10000 ? nav.n - 1 : (nav?.n ?? 0);
    this.starting = false;
    this.startError = "";
    this.tray = null; // whether the tray starts at login, once asked
    try {
      this.sub = localStorage.getItem("sub");
    } catch {}
    // A screen that fails to start says so in its tab instead of leaving it blank.
    try {
      this.screen = SCREENS[this.screenId].make(this, cx);
    } catch (e) {
      this.screen = new Broken(e);
    }
    if (this.screen.subs && !this.screen.subs.some((s) => s.id === this.sub)) this.sub = this.screen.subs[0].id;

    gwen.attach(cx);
    // Shared data changed: let the screen reload what it shows, then draw.
    cx.spawn(async (cx) => {
      for (;;) {
        await gwen.changed();
        this.follow(cx);
        if (gwen.up) this.syncScreen(cx);
        cx.notify();
      }
    });
    // Keys bound to this mod, and "show:<sub>" from gwend when the tray asks for a sub-screen.
    cx.spawn(async (cx) => {
      for (;;) {
        const command = await nextCommand();
        if (command === "focus") this.handle.focus();
        else if (command.startsWith("show:")) this.showSub(command.slice(5), null, cx);
        cx.notify();
      }
    });
    // Timers on screen tick once a second, only while this tab is the one shown.
    cx.timer.every(1000, (cx) => {
      if (!gwen.up || !this.visible()) return;
      const state = gwen.status?.state ?? "off";
      if (state !== "off" || this.screen.tick || gwen.toasts.length) cx.notify();
    });
    // The kit's pickers take their text fields from spares, which only a timer or a handler may make.
    cx.timer.every(200, (cx) => {
      if (this.ui.refill()) cx.notify();
    });
    if (gwen.up) this.syncScreen(cx);
    this.follow(cx);
  }

  syncScreen(cx) {
    try {
      this.screen.sync?.(cx);
    } catch (e) {
      console.error(`gwen ${this.screenId} sync: ${e?.stack ?? e}`);
    }
  }

  /** Whether this tab is the one on screen: the shown space's shown tab has this screen's name. */
  visible() {
    try {
      const tab = spaces().find((s) => s.active)?.tabs.find((t) => t.active);
      return !tab || !tab.named || tab.name === SCREENS[this.screenId].title;
    } catch {
      return true;
    }
  }

  /** Runs async work for a handler: app.spawn(cx, async (cx) => { ...; cx.notify(); }). Failures show as a toast. */
  spawn(cx, body) {
    cx.spawn(async (cx) => {
      try {
        await body(cx);
      } catch (e) {
        gwen.fail(e);
      }
      cx.notify();
    });
  }

  /** Opens another of Gwen's tabs, by its name or an old screen name ("board"), with an optional sub-screen and param. */
  go(screen, sub = null, param = null, cx = null) {
    const [tab, aliasSub] = ALIASES[screen] ?? [screen, ""];
    const target = sub ?? (aliasSub || null);
    if (tab === this.screenId) {
      if (cx) this.showSub(target, param, cx);
      return;
    }
    gwen.navigate = { screen: tab, sub: target, param, n: (gwen.navigate?.n ?? 0) + 1, at: Date.now() };
    gwen.bump();
    api.OpenApp(tab).catch((e) => gwen.fail(e));
  }

  /** Another tab asked for this one: take the sub-screen and param it named. */
  follow(cx) {
    const nav = gwen.navigate;
    if (!nav || nav.n <= this.navSeen) return;
    this.navSeen = nav.n;
    if (nav.screen === this.screenId) this.showSub(nav.sub, nav.param, cx);
  }

  showSub(sub, param, cx) {
    if (sub && this.screen.subs?.some((s) => s.id === sub)) this.setSub(sub);
    // A name that is not one of the screen's tabs is for the screen itself: a Settings section, a mode.
    else if (sub && param == null) param = sub;
    this.screen.show?.(this.sub, param, cx);
    cx.notify();
  }

  setSub(sub) {
    this.sub = sub;
    try {
      localStorage.setItem("sub", sub);
    } catch {}
  }

  // The status line -----------------------------------------------------------

  statusLine(ui) {
    const t = ui.t;
    const st = gwen.status;
    const tr = tracking();
    const bar = h_flex().flex_none().w_full().h(40).px(14).gap(10).items_center().border_b(1).border_color(t.border).bg(t.chrome);
    if (!st) return bar;
    const color = stateColor[st.state];
    const now = gwen.now();
    // The day's worked time goes on from its total, so a break or a switch never restarts it.
    const worked = (st.today?.worked_ms ?? 0) + (tr.working && st.today ? now - st.server_now_at : 0);
    const breakFor = tr.onBreak && st.open_segment ? now - st.open_segment.started_at : 0;
    const project = gwen.projects.find((p) => p.id === st.project_id);
    const task = gwen.tasks.find((x) => x.id === st.task_id);
    bar.child(ui.dot(color, 8)).child(
      div()
        .text_size(13)
        .font_medium()
        .text_color(color)
        .child(`${dockLabel[st.state]}${tr.onBreak ? ` · ${formatDuration(breakFor)}` : ""}`),
    );
    if (st.state !== "off") {
      bar
        .child(div().id("st-worked").text_size(13).font_semibold().text_color(tr.onBreak ? t.muted : t.foreground).tooltip("Worked today").child(clockFace(worked)))
        .child(div().flex_1().min_w(0).truncate().text_size(12.5).text_color(t.muted).child(task?.title ?? project?.name ?? "Unassigned"));
    } else bar.child(div().flex_1());
    if (st.warnings?.length) bar.child(div().id("st-warn").tooltip(st.warnings.join("\n")).child(ui.icon("triangle-alert", 14, ui.tone("warn"))));
    if (st.state === "off") bar.child(ui.button("st-in", "Clock in", () => tr.clockIn(gwen.lastAttribution.project_id, gwen.lastAttribution.task_id), { tone: "primary", small: true, icon: "play" }));
    if (tr.onBreak) bar.child(ui.button("st-back", "End break", () => tr.breakEnd(), { tone: "primary", small: true, icon: "play" }));
    if (tr.working) bar.child(ui.button("st-break", "Break", () => tr.breakStart(), { small: true, icon: "coffee", iconColor: stateColor.break_manual }));
    if (st.state !== "off") bar.child(ui.iconButton("st-out", "square", () => tr.clockOut(), { tip: "Clock out", size: 24 }));
    return bar;
  }

  // Not running -----------------------------------------------------------------

  notRunning(ui, cx) {
    const t = ui.t;
    if (this.tray === null) {
      this.tray = false;
      cx.spawn(async (cx) => {
        this.tray = await host.autostart();
        cx.notify();
      });
    }
    const start = (e, cx) => {
      this.starting = true;
      this.startError = "";
      cx.notify();
      cx.spawn(async (cx) => {
        try {
          await host.startService();
          for (let i = 0; i < 20 && gwen.up !== true; i++) {
            await gwen.refresh();
            if (gwen.up !== true) await cx.sleep(250);
          }
          if (gwen.up !== true) this.startError = "gwend did not answer within 5 s. See: journalctl --user -u gwend";
        } catch (e) {
          this.startError = String(e?.message ?? e);
        }
        this.starting = false;
        cx.notify();
      });
    };
    return v_flex()
      .size_full()
      .items_center()
      .justify_center()
      .p(24)
      .child(
        v_flex()
          .w(480)
          .max_w("100%")
          .gap(12)
          .child(ui.face("smiling", 56))
          .child(div().pt(8).text_size(22).font_semibold().text_color(t.foreground).child("Gwen isn't running"))
          .child(ui.text("Gwen clocks your working day, notices when you step away, nudges you back, and turns your goals into daily plans. Everything stays on this computer.", { size: 14, color: t.muted, lh: 21 }))
          .child(ui.text("Start her background service so she can track your time. It starts again every time you log in, and closing this window never stops it.", { size: 14, color: t.dim, lh: 21 }))
          .child(
            h_flex()
              .pt(10)
              .gap(8)
              .child(ui.button("fr-start", this.starting ? "Starting…" : "Start Gwen", start, { tone: "primary", icon: "power", busy: this.starting }))
              .child(ui.button("fr-retry", "Try again", () => gwen.refresh(), { icon: "rotate-ccw" })),
          )
          .child(
            div()
              .pt(10)
              .child(
                ui.toggleRow(
                  "fr-tray",
                  "Show the tray icon at login",
                  "Clock in, take breaks, and clock out from the panel.",
                  this.tray,
                  (on, cx) =>
                    this.spawn(cx, async () => {
                      await host.setAutostart(on);
                      this.tray = on;
                    }),
                  { divider: false },
                ),
              ),
          )
          .when(Boolean(this.startError), (e) => e.child(ui.callout({ tone: "danger", icon: "circle-alert", text: this.startError }))),
      );
  }

  // Toasts ------------------------------------------------------------------------

  toasts(ui) {
    const t = ui.t;
    const live = gwen.toasts.filter((x) => Date.now() - x.at < 6000);
    if (live.length !== gwen.toasts.length) gwen.toasts = live;
    if (live.length === 0) return null;
    return v_flex()
      .absolute()
      .bottom(16)
      .right(16)
      .gap(8)
      .children(
        live.slice(-3).map((x) =>
          h_flex()
            .id(`toast-${x.id}`)
            .max_w(440)
            .items_center()
            .gap(9)
            .px(12)
            .py(9)
            .rounded(9)
            .border_1()
            .border_color(x.level === "error" ? ui.tone("danger") : t.border)
            .bg(t.popover)
            .shadow_lg()
            .cursor_pointer()
            .on_click((e, cx) => {
              gwen.toasts = gwen.toasts.filter((y) => y.id !== x.id);
              cx.notify();
            })
            .child(ui.icon(x.level === "error" ? "circle-alert" : x.level === "success" ? "circle-check" : "info", 14, x.level === "error" ? ui.tone("danger") : x.level === "success" ? ui.tone("ok") : t.muted))
            .child(div().flex_1().min_w(0).text_size(12.5).line_height("18px").text_color(t.foreground).child(x.text))
            .when(Boolean(x.action), (e) =>
              e.child(
                ui.button(`toast-${x.id}-act`, x.action.label, (ev, cx) => {
                  cx.stop_propagation();
                  gwen.toasts = gwen.toasts.filter((y) => y.id !== x.id);
                  x.action.run();
                  cx.notify();
                }, { small: true }),
              ),
            ),
        ),
      );
  }

  render(cx) {
    const ui = this.ui;
    const t = ui.begin();
    const root = v_flex()
      .id("gwen-root")
      .track_focus(this.handle)
      .relative()
      .size_full()
      .bg(t.background)
      .text_color(t.foreground)
      .text_size(13)
      .on_key_down((e, cx) => {
        if (e.key !== "escape") return;
        if (ui.popup !== null) ui.closePopup(cx);
        else if (ui.dialogs.length) ui.closeDialog(cx);
        else return;
        cx.stop_propagation();
      });
    if (gwen.up === null) return root.child(ui.splash("Connecting to Gwen…", { busy: true }));
    if (!gwen.up) return root.child(this.notRunning(ui, cx));
    root.child(this.statusLine(ui));
    const subs = this.screen.subs;
    if (subs?.length > 1) {
      root.child(
        h_flex()
          .flex_none()
          .w_full()
          .px(14)
          .h(40)
          .items_center()
          .border_b(1)
          .border_color(t.border)
          .child(
            ui.tabs("sub", subs.map((s) => ({ ...s, count: s.count?.() })), this.sub, (id, cx) => {
              this.setSub(id);
              this.screen.show?.(id, null, cx);
              cx.notify();
            }),
          ),
      );
    }
    let body;
    try {
      body = this.screen.render(ui, cx);
    } catch (e) {
      console.error(`gwen ${this.screenId}: ${e?.stack ?? e}`);
      body = new Broken(e).render(ui);
    }
    root.child(div().flex_1().min_h(0).w_full().flex().flex_col().child(body));
    const toasts = this.toasts(ui);
    if (toasts) root.child(toasts);
    const dialog = ui.dialogLayer();
    if (dialog) root.child(dialog);
    return root;
  }
}
