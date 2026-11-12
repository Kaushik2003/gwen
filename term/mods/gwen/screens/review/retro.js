// A look back at one week, written from its numbers by the AI: what went
// well, what slipped, and one or two things to try next week.

import { div } from "gpui-kit";
import { v_flex, h_flex } from "gpui-base";
import { api, apiError, gwen } from "../../lib/gwen.js";
import { addDays, formatDate } from "../../lib/format.js";
import { aiProgress, aiUnavailable, mondayOf } from "../../lib/ai.js";

export class Retro {
  /** fixed: true when the week is set by the screen (weekly review), else a picker of the last four weeks shows. */
  constructor(app, id, o = {}) {
    this.app = app;
    this.id = id;
    this.fixed = Boolean(o.fixed);
    this.week = null;
    this.out = null;
    this.since = null;
    this.unavailable = null;
  }

  get tick() {
    return this.since !== null;
  }

  /** Shows another week; what was written for the last one goes. */
  setWeek(week) {
    if (this.week === week) return;
    this.week = week;
    this.out = null;
    this.unavailable = null;
  }

  generate(cx) {
    if (this.since !== null) return;
    const week = this.week;
    this.since = Date.now();
    this.unavailable = null;
    cx.notify();
    cx.spawn(async (cx) => {
      try {
        const run = await api.Retro({ week_start: week });
        if (this.week === week) this.out = run.output;
      } catch (e) {
        const err = apiError(e);
        if (err.code === "unavailable") this.unavailable = err.message;
        else gwen.fail(e);
      }
      this.since = null;
      cx.notify();
    });
  }

  render(ui) {
    const t = ui.t;
    const thisWeek = mondayOf(gwen.today());
    if (!this.week) this.week = addDays(thisWeek, -7);
    const week = this.week;
    const row = h_flex().flex_wrap().items_center().gap(12);
    if (!this.fixed) {
      const weeks = [0, 1, 2, 3].map((i) => addDays(thisWeek, -7 * i));
      const label = (w, i) => (i === 0 ? "This week" : i === 1 ? "Last week" : formatDate(w, false));
      row.child(
        ui.segmented(
          `${this.id}-week`,
          weeks.map((w, i) => ({ value: w, label: label(w, i) })),
          week,
          (w, cx) => {
            this.setWeek(w);
            cx.notify();
          },
          { small: true },
        ),
      );
    }
    row
      .child(ui.muted(`${formatDate(week, false)} to ${formatDate(addDays(week, 6), false)}`, { size: 13 }))
      .child(div().flex_1())
      .child(ui.button(`${this.id}-go`, this.out ? "Write again" : "Write retro", (e, cx) => this.generate(cx), { tone: "primary", icon: "scroll-text", busy: this.since !== null }));
    const el = v_flex().gap(14).child(row);
    if (this.since !== null) el.child(aiProgress(ui, this.since, "Writing your retro"));
    if (this.unavailable) el.child(aiUnavailable(ui, this.app, this.unavailable, `${this.id}-setup`));
    if (this.out)
      el.child(
        v_flex()
          .gap(10)
          .p(16)
          .rounded(8)
          .border_1()
          .border_color(t.border)
          .bg(t.raised)
          .child(ui.markdown(`${this.id}-md`, this.out.markdown ?? ""))
          .when(this.out.generated_by === "rules", (e) => e.child(div().pt(8).border_t(1).border_color(t.border).child(ui.dim("Written from your numbers alone, because the AI did not answer.")))),
      );
    if (!this.out && this.since === null && !this.unavailable) el.child(ui.muted("What went well, what slipped, and one or two things to try next week, written from that week's numbers.", { size: 13 }));
    return el;
  }
}
