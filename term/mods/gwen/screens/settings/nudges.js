import { v_flex, h_flex } from "gpui-base";
import { api, gwen } from "../../lib/gwen.js";
import { ConfigForm } from "./form.js";

const fields = [
  { key: "desktop", label: "Desktop notifications", kind: { type: "bool" } },
  { key: "phone", label: "Phone notifications", hint: "Through the ntfy app. Set it up under Phone.", kind: { type: "bool" } },
  { key: "break_reminder", label: "Remind me to take a break after", hint: "Of work without one. Zero turns it off.", kind: { type: "duration" } },
  { key: "repeat", label: "Repeat an unanswered nudge every", kind: { type: "duration", units: ["m", "s"] } },
  { key: "snooze", label: "Snooze nudges for", kind: { type: "duration" } },
];

export default class Nudges extends ConfigForm {
  constructor(app) {
    super(app, { title: "Nudges", description: "When Gwen taps you on the shoulder.", name: "nudge", fields });
    this.shown = null;
    this.previewing = false;
  }

  preview(cx) {
    if (this.previewing || !gwen.config?.nudge?.desktop) return;
    this.previewing = true;
    this.app.spawn(cx, async () => {
      try {
        const result = await api.NotifyPreview();
        this.shown = result.nudges;
        gwen.toast(`Watch the top of your screen: ${result.nudges.length} nudges, one every 7 seconds.`, "success");
      } finally {
        this.previewing = false;
      }
    });
  }

  render(ui) {
    const desktop = Boolean(gwen.config?.nudge?.desktop);
    const preview = ui
      .panel({ title: "See every nudge", icon: "bell-ring" })
      .child(ui.muted("Gwen only nudges when the moment fits: after you're idle a while, on a long break, or when a day starts. Preview them all here, one every 7 seconds, with her face, words, and sound. Nothing goes to your phone, and the buttons only close them.", { size: 13, lh: 19 }))
      .child(
        h_flex()
          .flex_wrap()
          .items_center()
          .gap(12)
          .child(ui.button("nudges-preview", "Preview all nudges", (event, cx) => {
            this.preview(cx);
            cx.notify();
          }, { icon: "sparkles", busy: this.previewing, disabled: !desktop }))
          .when(!desktop, (el) => el.child(ui.muted("Turn on desktop notifications first."))),
      );
    if (this.shown) {
      preview.child(
        v_flex().gap(6).children(this.shown.map((nudge, i) =>
          h_flex()
            .items_baseline()
            .gap(12)
            .child(ui.dim(`${i * 7}s`).w(32).flex_none().text_right())
            .child(ui.text(nudge.title).flex_1().min_w(0)),
        )),
      );
    }
    return [super.render(ui), preview];
  }
}
