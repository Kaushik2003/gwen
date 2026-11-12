// Weekly review: once a week, somewhere quiet, look back at your
// productivity and energy, what worked and what didn't, and turn it into
// changes for the next week. The assistant reads it.

import { div } from "gpui-kit";
import { v_flex, h_flex } from "gpui-base";
import { api, gwen } from "../../lib/gwen.js";
import { addDays, formatDate, formatDuration } from "../../lib/format.js";
import { mondayOf } from "../../lib/ai.js";
import { TextField } from "../../lib/kit.js";
import { Retro } from "./retro.js";

/** The review's steps, a bit each in WeeklyReview.checklist. screen and param open where the step happens. */
const steps = [
  { title: "Empty your inbox", detail: "Process everything you captured this week.", screen: "inbox" },
  { title: "Look back at the week", detail: "What you finished, and how your time went.", screen: "stats" },
  { title: "Review every project", detail: "Each one has a clear next action.", screen: "board" },
  { title: "Check what you're waiting for", detail: "Chase what is overdue from others.", screen: "inbox" },
  { title: "Look through someday / maybe", detail: "Activate anything whose time has come.", screen: "inbox" },
  { title: "Check your goals' pace", detail: "Are they still SMART, and on track?", screen: "goals" },
  { title: "Plan the week ahead", detail: "Put the important, not urgent work on the calendar.", screen: "board", param: "matrix" },
];

const prompts = [
  { key: "went_well", title: "What worked for you?", placeholder: "Waking up at 06:00 made a real productivity change for me." },
  { key: "went_badly", title: "What didn't?", placeholder: "Eating a large meal at lunch threw me off my groove." },
  { key: "energy", title: "How was your energy?", placeholder: "Sharp in the mornings, flat after 15:00." },
  { key: "decisions", title: "Which decisions would you change?", placeholder: "I said yes to two meetings I didn't need." },
  { key: "improvements", title: "What will you do differently next week?", placeholder: "I don't estimate how long tasks take well: add 30% to every estimate." },
];

export default class Weekly {
  constructor(app) {
    this.app = app;
    // The week just ended is the one to review, until it is a few days old.
    const wd = new Date().getDay();
    this.week = null;
    this.startBack = wd >= 1 && wd <= 4;
    this.review = null;
    this.summary = null;
    this.finished = null;
    this.saved = false;
    this.loaded = null;
    this.retro = new Retro(app, "wk-retro", { fixed: true });
    this.fields = {};
    // An answer saves a moment after typing stops, and when its box is left.
    this.typing = {};
    for (const p of prompts)
      this.fields[p.key] = new TextField({
        multiline: true,
        rows: 2,
        grow: [2, 6],
        placeholder: p.placeholder,
        onBlur: (v, cx) => this.saveText(p.key, v, cx),
        onChange: (v, cx) => {
          const n = (this.typing[p.key] = (this.typing[p.key] ?? 0) + 1);
          cx.spawn(async (cx) => {
            await cx.sleep(900);
            if (this.typing[p.key] === n) this.saveText(p.key, this.fields[p.key].value, cx);
          });
        },
      });
  }

  get tick() {
    return this.retro.tick;
  }

  sync(cx) {
    if (!this.week) {
      const thisWeek = mondayOf(gwen.today());
      this.week = this.startBack ? addDays(thisWeek, -7) : thisWeek;
    }
    const week = this.week;
    this.retro.setWeek(week);
    const stamp = `${week}/${gwen.versions.days}/${gwen.versions.tasks}`;
    if (this.loaded === stamp) return;
    const sameWeek = this.loaded?.startsWith(`${week}/`);
    this.loaded = stamp;
    cx.spawn(async (cx) => {
      try {
        if (!sameWeek) {
          const r = await api.GetReview(week);
          if (this.week !== week) return;
          this.adopt(r);
        }
        this.summary = await api.StatsSummary(week, addDays(week, 6)).catch(() => null);
        const l = await api.ListTasks({ status: "done" });
        const from = new Date(`${week}T00:00`).getTime();
        const to = from + 7 * 86_400_000;
        if (this.week === week) this.finished = (l.tasks ?? []).filter((x) => (x.done_at ?? 0) >= from && (x.done_at ?? 0) < to).length;
      } catch (e) {
        gwen.fail(e);
      }
      cx.notify();
    });
  }

  adopt(r) {
    this.review = r;
    for (const p of prompts) if (!this.fields[p.key].focused) this.fields[p.key].set(r[p.key] ?? "");
  }

  setWeek(week, cx) {
    this.week = week;
    this.review = this.summary = this.finished = null;
    this.saved = false;
    this.sync(cx);
    cx.notify();
  }

  save(patch, cx) {
    const week = this.week;
    this.app.spawn(cx, async () => {
      const r = await gwen.act(() => api.SaveReview(week, patch));
      if (r && this.week === week) {
        this.review = r;
        this.saved = true;
      }
    });
  }

  saveText(key, value, cx) {
    const v = value.trim();
    if (this.review && v !== (this.review[key] ?? "")) {
      this.review = { ...this.review, [key]: v }; // a second save of the same words is not sent
      this.save({ [key]: v }, cx);
    }
  }

  render(ui) {
    const t = ui.t;
    const thisWeek = mondayOf(gwen.today());
    const week = this.week ?? thisWeek;
    const checked = this.review?.checklist ?? 0;
    const doneSteps = steps.filter((_, i) => checked & (1 << i)).length;
    const inbox = gwen.tasks.filter((x) => x.stage === "inbox").length;
    const s = this.summary;

    const header = ui.pageHeader(
      "Weekly review",
      "Once a week, somewhere quiet: look back at your productivity and energy, what worked and what didn't, and decide what changes.",
      h_flex()
        .items_center()
        .gap(6)
        .child(ui.iconButton("wk-prev", "chevron-left", (e, cx) => this.setWeek(addDays(week, -7), cx), { tip: "Earlier week", size: 28 }))
        .child(
          v_flex()
            .w(176)
            .items_center()
            .child(div().text_size(13).font_medium().child(week === thisWeek ? "This week" : week === addDays(thisWeek, -7) ? "Last week" : `Week of ${formatDate(week, false)}`))
            .child(ui.dim(`${formatDate(week, false)} – ${formatDate(addDays(week, 6), false)}`)),
        )
        .child(ui.iconButton("wk-next", "chevron-right", (e, cx) => this.setWeek(addDays(week, 7), cx), { tip: "Later week", size: 28, disabled: week >= thisWeek })),
    );

    const stat = (label, value) => v_flex().flex_1().min_w(0).gap(4).px(16).py(12).rounded(10).border_1().border_color(t.border).bg(t.surface).child(ui.dim(label)).child(div().text_size(17).font_semibold().child(String(value)));
    const stats = h_flex()
      .w_full()
      .gap(12)
      .child(stat("Worked", s ? formatDuration(s.worked_ms) : "–"))
      .child(stat("Days on target", s ? `${s.days_target_met} of ${s.days_tracked}` : "–"))
      .child(stat("Tasks finished", this.finished ?? "–"))
      .child(stat("Review steps", `${doneSteps} of ${steps.length}`));

    const checklist = ui
      .panel({ title: "Checklist", icon: "clipboard-check" })
      .flex_1()
      .min_w(0)
      .child(
        v_flex().children(
          steps.map((st, i) => {
            const on = Boolean(checked & (1 << i));
            return h_flex()
              .items_start()
              .gap(11)
              .px(4)
              .py(8)
              .child(div().pt(1).child(ui.checkbox(`wk-step-${i}`, on, (v, cx) => this.save({ checklist: checked ^ (1 << i) }, cx), { disabled: !this.review })))
              .child(
                v_flex()
                  .flex_1()
                  .min_w(0)
                  .gap(2)
                  .child(
                    h_flex()
                      .items_center()
                      .gap(8)
                      .child(div().text_size(13).font_medium().text_color(on ? t.muted : t.foreground).when(on, (e) => e.line_through()).child(st.title))
                      .when(i === 0 && inbox > 0, (e) => e.child(ui.badge(String(inbox), { tone: "accent" }))),
                  )
                  .child(ui.dim(st.detail)),
              )
              .child(ui.button(`wk-open-${i}`, "Open", () => this.app.go(st.screen, null, st.param ?? null), { small: true, tone: "ghost" }));
          }),
        ),
      );
    if (doneSteps === steps.length) checklist.child(h_flex().items_center().gap(6).child(ui.icon("circle-check", 14, ui.tone("ok"))).child(ui.text("Review done. Enjoy your week.", { size: 13, color: ui.tone("ok") })));

    const reflect = ui.panel({ title: "Reflect", icon: "notebook-pen", actions: this.saved ? ui.dim("Saved") : null }).flex_1().min_w(0);
    if (!this.review) reflect.child(ui.muted("Loading…", { size: 13 }));
    else {
      for (const p of prompts) reflect.child(ui.field(p.title, this.fields[p.key].el(ui)));
      reflect.child(ui.dim("Answers save as you type. The assistant reads your latest review, so it can keep what worked and help with what didn't."));
    }

    const talk = ui
      .panel({ title: "Talk it through", icon: "message-square-text" })
      .flex_1()
      .min_w(0)
      .child(ui.muted("Go through the week with the assistant: it sees your tasks, goals, and this review, and can reschedule and plan next week with you as you talk.", { size: 13 }))
      .child(
        h_flex()
          .flex_wrap()
          .gap(8)
          .child(ui.button("wk-talk", "Review with the assistant", () => this.app.go("gwen", null, "Let's do my weekly review: what should I change for next week?"), { tone: "primary", icon: "sparkles" }))
          .child(ui.button("wk-plan", "Plan next week", () => this.app.go("gwen", null, "Plan my week ahead, important work first"), { icon: "calendar-check" }))
          .child(ui.button("wk-matrix", "Eisenhower matrix", () => this.app.go("board", null, "matrix"), { icon: "lightbulb" })),
      );

    return ui.scroll("wk-scroll", [
      header,
      stats,
      h_flex().w_full().items_start().gap(16).child(checklist).child(reflect),
      h_flex()
        .w_full()
        .items_start()
        .gap(16)
        .child(ui.panel({ title: "Retro from your numbers", icon: "scroll-text" }).flex_1().min_w(0).child(this.retro.render(ui)))
        .child(talk),
    ]);
  }
}
