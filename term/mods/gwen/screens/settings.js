// Settings: a list of sections on the left, the chosen one on the right.
// Each section is its own file in screens/settings/ and a plain class:
//
//   constructor(app, cx) make text fields and first state here; cx may start timers and tasks
//   sync(cx)             optional: shared data changed, reload what it shows
//   render(ui, cx)       an element, or a list of them (usually ui.panel(...)s)
//   tick                 optional true while it shows something that counts seconds
//
// app.go("settings", "ai") from any screen opens a section by its id.

import { div } from "gpui-kit";
import { v_flex, h_flex } from "gpui-base";
import { gwen } from "../lib/gwen.js";
import Tracking from "./settings/tracking.js";
import Nudges from "./settings/nudges.js";
import Phone from "./settings/phone.js";
import Planner from "./settings/planner.js";
import Methods from "./settings/methods.js";
import Calendar from "./settings/calendar.js";
import AI from "./settings/ai.js";
import Voice from "./settings/voice.js";
import Personality from "./settings/personality.js";
import MemorySettings from "./settings/memory.js";
import Appearance from "./settings/appearance.js";
import Sync from "./settings/sync.js";
import Advanced from "./settings/advanced.js";
import About from "./settings/about.js";

const SECTIONS = [
  { id: "tracking", label: "Tracking", icon: "timer", parts: [Tracking] },
  { id: "nudges", label: "Nudges", icon: "bell-ring", parts: [Nudges] },
  { id: "phone", label: "Phone", icon: "smartphone", parts: [Phone] },
  { id: "planner", label: "Planner", icon: "calendar-clock", parts: [Planner] },
  { id: "methods", label: "Methods", icon: "brain", parts: [Methods] },
  { id: "calendar", label: "Google Calendar", icon: "calendar-days", parts: [Calendar] },
  { id: "ai", label: "AI", icon: "sparkles", parts: [AI, Voice, Personality] },
  { id: "memory", label: "Memory", icon: "book-marked", parts: [MemorySettings] },
  { id: "appearance", label: "Appearance", icon: "palette", parts: [Appearance] },
  { id: "sync", label: "Sync", icon: "cloud", parts: [Sync] },
  { id: "advanced", label: "Advanced", icon: "wrench", parts: [Advanced] },
  { id: "about", label: "About", icon: "info", parts: [About] },
];

export default class Settings {
  constructor(app, cx) {
    this.app = app;
    this.section = "tracking";
    try {
      const saved = localStorage.getItem("settings.section");
      if (SECTIONS.some((s) => s.id === saved)) this.section = saved;
    } catch {}
    // Every section is made up front: text fields cannot be created while drawing.
    this.parts = {};
    for (const s of SECTIONS)
      this.parts[s.id] = s.parts.map((Part) => {
        try {
          return new Part(app, cx);
        } catch (e) {
          console.error(`gwen settings ${s.id}: ${e?.stack ?? e}`);
          return { render: (ui) => ui.callout({ tone: "danger", icon: "circle-alert", text: `This section hit an error and could not start: ${String(e?.message ?? e)}` }) };
        }
      });
  }

  get tick() {
    return this.parts[this.section].some((p) => p.tick);
  }

  open(id, cx) {
    if (!SECTIONS.some((s) => s.id === id)) return;
    this.section = id;
    try {
      localStorage.setItem("settings.section", id);
    } catch {}
    for (const p of this.parts[id]) p.sync?.(cx);
    cx.notify();
  }

  /** Another screen asked for a section: app.go("settings", "ai"). */
  show(_sub, param, cx) {
    if (param) this.open(param, cx);
  }

  sync(cx) {
    for (const p of this.parts[this.section]) p.sync?.(cx);
  }

  render(ui, cx) {
    const t = ui.t;
    if (!gwen.config) return ui.splash("Loading…", { busy: true });
    const nav = v_flex()
      .flex_none()
      .w(200)
      .h_full()
      .p(10)
      .gap(2)
      .border_r(1)
      .border_color(t.border)
      .children(
        SECTIONS.map((s) => {
          const on = s.id === this.section;
          return h_flex()
            .id(`set-nav-${s.id}`)
            .items_center()
            .gap(10)
            .px(10)
            .h(30)
            .rounded(7)
            .cursor_pointer()
            .text_size(13)
            .when(on, (e) => e.bg(t.selected).text_color(t.foreground))
            .when(!on, (e) => e.text_color(t.muted).hover((st) => st.bg(t.hover).text_color(t.foreground)))
            .on_click((e, cx) => this.open(s.id, cx))
            .child(ui.icon(s.icon, 15, on ? t.foreground : t.dim))
            .child(s.label);
        }),
      );
    const content = this.parts[this.section].flatMap((p) => {
      try {
        const out = p.render(ui, cx);
        return Array.isArray(out) ? out : [out];
      } catch (e) {
        console.error(`gwen settings ${this.section}: ${e?.stack ?? e}`);
        return [ui.callout({ tone: "danger", icon: "circle-alert", text: `This section hit an error and could not be drawn: ${String(e?.message ?? e)}` })];
      }
    });
    return h_flex().size_full().min_h(0).items_start().child(nav).child(div().flex_1().min_w(0).h_full().flex().flex_col().child(ui.scroll("set-scroll", content, { maxW: 780 })));
  }
}
