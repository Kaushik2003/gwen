// Goals: targets with a date that Gwen paces into the daily plan, with their
// SMART check and the AI breakdown; and commitments, the recurring time that
// is not planned work.

import { div } from "gpui-kit";
import { v_flex, h_flex } from "gpui-base";
import { api, apiError, gwen } from "../../lib/gwen.js";
import { daysBetween, formatClock, formatDate, formatDuration, goDuration, minutesOf, parseDuration } from "../../lib/format.js";
import { clockOf, describeRule, litWeekdays, weekdayCodes } from "../../lib/rrule.js";
import { aiFailed, aiProgress, aiUnavailable } from "../../lib/ai.js";
import { TextField, alpha, priorityOf } from "../../lib/kit.js";
import { Recurrence, goalFraction, goalMeter, paceChip, projectTag } from "../../lib/parts.js";

const singular = (unit) => (!unit ? "unit" : unit.endsWith("s") ? unit.slice(0, -1) : unit);

/** "2h a day · 6 a session" for a quantity goal with a daily time. */
function goalShape(g) {
  if (g.kind === "tasks") return "A set of tasks";
  const each = `${g.minutes_per_unit ?? 0} min each`;
  if (g.daily_minutes == null) return each;
  return `${formatDuration(g.daily_minutes * 60_000)} a day · ${g.progress.per_session ?? 0} a session · ${each}`;
}

const smartLetters = [
  { key: "specific", letter: "S", name: "Specific", ask: "What exactly will you achieve?", placeholder: "Solve the 150 NeetCode problems, every topic, in Python." },
  { key: "measurable", letter: "M", name: "Measurable", ask: "How will you measure progress?", placeholder: "Problems solved, and a timed mock interview each Sunday." },
  { key: "assignable", letter: "A", name: "Assignable", ask: "Who does it?", placeholder: "Me, with Sam reviewing my solutions on Fridays." },
  { key: "realistic", letter: "R", name: "Realistic", ask: "Why can you do it with the time you have?", placeholder: "Two problems a day fits in my 1h evening slot." },
  { key: "time", letter: "T", name: "Time-related", ask: "By when?", placeholder: "" },
];

function smartBadges(ui, id, lit) {
  const t = ui.t;
  return h_flex()
    .gap(4)
    .children(
      smartLetters.map((l) =>
        div()
          .id(`${id}-${l.letter}`)
          .w(20)
          .h(20)
          .flex()
          .items_center()
          .justify_center()
          .rounded(4)
          .text_size(11)
          .font_semibold()
          .tooltip(`${l.name}${lit(l.key) ? "" : ": not set"}`)
          .when(lit(l.key), (e) => e.bg(alpha(t.accent, 0.2)).text_color(t.accent))
          .when(!lit(l.key), (e) => e.bg(t.hover).text_color(t.dim))
          .child(l.letter),
      ),
    );
}

/** The goal dialog: makes or changes one goal. */
class GoalForm {
  constructor(app, goal) {
    this.app = app;
    this.goal = goal;
    this.title = new TextField({ value: goal?.title ?? "", placeholder: "Solve 300 DSA problems" });
    this.unit = new TextField({ value: goal?.unit ?? "", placeholder: "problems", onChange: (v, cx) => cx.notify() });
    this.kind = goal?.kind ?? "quantity";
    this.target = goal?.target_quantity ?? null;
    this.perUnit = goal?.minutes_per_unit ?? null;
    this.daily = goDuration((goal?.daily_minutes ?? 0) * 60_000);
    this.project = goal?.project_id ?? "";
    this.start = goal?.start_day ?? gwen.today();
    this.due = goal?.due_day ?? "";
    this.status = goal?.status ?? "active";
    this.template = null; // a quantity goal's repeating session task
    this.rule = "";
    this.repeat = new Recurrence("goal-repeat");
    this.smart = {};
    for (const l of smartLetters.slice(0, 4)) this.smart[l.key] = new TextField({ value: goal?.[l.key] ?? "", multiline: true, rows: 1, grow: [1, 4], placeholder: l.placeholder, onChange: (v, cx) => cx.notify() });
    this.preview = null;
    this.previewSeq = 0;
    this.saving = false;
  }

  get quantity() {
    return this.kind === "quantity";
  }

  get dailyMinutes() {
    return Math.round(parseDuration(this.daily) / 60_000);
  }

  open(cx) {
    const ui = this.app.ui;
    if (this.goal?.kind === "quantity")
      cx.spawn(async (cx) => {
        try {
          const l = await api.ListTasks({ goal_id: this.goal.id, templates: true, status: "all" });
          this.template = (l.tasks ?? []).find((x) => x.rrule) ?? null;
          this.rule = this.template?.rrule ?? "";
        } catch (e) {
          gwen.fail(e);
        }
        cx.notify();
      });
    this.id = ui.openDialog(
      {
        title: this.goal ? "Edit goal" : "New goal",
        width: 680,
        height: 600,
        render: (ui) => this.render(ui),
        actions: (ui) => [
          ui.button("goal-cancel", "Cancel", (e, cx) => ui.closeDialog(cx, this.id), { tone: "ghost" }),
          ui.button("goal-save", this.goal ? "Save goal" : "Create goal", (e, cx) => this.save(cx), { tone: "primary", busy: this.saving, disabled: !this.valid() }),
        ],
      },
      cx,
    );
    this.title.focus();
    this.refreshPreview(cx);
  }

  valid() {
    const d = this.dailyMinutes;
    return Boolean(this.title.value.trim() && this.due && this.start && this.due >= this.start && (!this.quantity || (this.target > 0 && this.perUnit > 0 && (d === 0 || d >= 5))));
  }

  /** What the goal would look like, checked by the daemon as it is typed. */
  refreshPreview(cx) {
    const seq = ++this.previewSeq;
    this.preview = null;
    if (!this.quantity || !(this.target > 0) || !(this.perUnit > 0) || !this.start || !this.due || this.due < this.start) return;
    const req = {
      goal_id: this.goal?.id ?? null,
      kind: this.kind,
      target_quantity: this.target,
      minutes_per_unit: this.perUnit,
      daily_minutes: this.dailyMinutes >= 5 ? this.dailyMinutes : null,
      start_day: this.start,
      due_day: this.due,
    };
    cx.spawn(async (cx) => {
      await cx.sleep(250);
      if (seq !== this.previewSeq) return;
      const p = await api.PreviewGoal(req).catch(() => null);
      if (seq === this.previewSeq) this.preview = p;
      cx.notify();
    });
  }

  set(field, value, cx) {
    this[field] = value;
    this.refreshPreview(cx);
    cx.notify();
  }

  save(cx) {
    if (!this.valid() || this.saving) return;
    this.saving = true;
    const goal = this.goal;
    const title = this.title.value.trim();
    const fields = {
      title,
      unit: this.unit.value.trim(),
      target_quantity: this.quantity ? this.target : null,
      minutes_per_unit: this.quantity ? this.perUnit : null,
      daily_minutes: this.quantity && this.dailyMinutes >= 5 ? this.dailyMinutes : null,
      project_id: this.project || null,
      start_day: this.start,
      due_day: this.due,
    };
    for (const k of Object.keys(this.smart)) fields[k] = this.smart[k].value;
    this.app.spawn(cx, async (cx) => {
      const ok = goal ? await gwen.act(() => api.PatchGoal(goal.id, { ...fields, status: this.status, rev: goal.rev })) : await gwen.act(() => api.CreateGoal({ ...fields, kind: this.kind }));
      if (ok && this.template && this.rule !== this.template.rrule) {
        if (!(await gwen.act(() => api.PatchTask(this.template.id, { rrule: this.rule, rev: this.template.rev })))) {
          this.saving = false;
          return;
        }
      }
      this.saving = false;
      if (!ok) return;
      gwen.toast(goal ? `Saved ${title}` : `Created ${title}`, "success");
      this.app.ui.closeDialog(cx, this.id);
    });
  }

  /** Whether the goal fits its time a day, and when it does not, the three ways to make it fit. */
  fit(ui) {
    const p = this.preview;
    const t = ui.t;
    const target = this.target;
    const unit = this.unit.value.trim() || "units";
    const perDay = p.required_per_day;
    if (p.per_session == null) return h_flex().items_center().gap(8).child(ui.icon("calendar-range", 14)).child(ui.muted(`About ${perDay < 10 ? perDay.toFixed(1) : Math.round(perDay)} ${unit} a day. Set a time a day to check it fits.`, { size: 13 }));
    if (p.reachable_quantity >= target) return ui.callout({ tone: "ok", icon: "circle-check", text: `It fits: each session holds up to ${p.per_session} ${unit}, and ${Math.ceil(perDay)} a day reaches all ${target} by ${formatDate(this.due)}.` });
    const fixes = h_flex().flex_wrap().gap(8);
    if (p.needed_daily_minutes != null) fixes.child(ui.button("goal-fix-daily", `Give it ${formatDuration(p.needed_daily_minutes * 60_000)} a day`, (e, cx) => this.set("daily", goDuration(p.needed_daily_minutes * 60_000), cx), { small: true, icon: "clock" }));
    if (p.needed_due_day) fixes.child(ui.button("goal-fix-due", `Finish by ${formatDate(p.needed_due_day)}`, (e, cx) => this.set("due", p.needed_due_day, cx), { small: true, icon: "calendar-range" }));
    fixes.child(ui.button("goal-fix-target", `Aim for ${p.reachable_quantity}`, (e, cx) => this.set("target", p.reachable_quantity, cx), { small: true, icon: "target" }));
    return v_flex()
      .gap(10)
      .p(14)
      .rounded(8)
      .border_1()
      .border_color(alpha(ui.tone("warn"), 0.3))
      .bg(alpha(ui.tone("warn"), 0.06))
      .child(
        h_flex()
          .items_start()
          .gap(8)
          .child(div().pt(2).child(ui.icon("triangle-alert", 15, ui.tone("warn"))))
          .child(
            div()
              .flex_1()
              .min_w(0)
              .text_size(13)
              .line_height("19px")
              .text_color(t.foreground)
              .child(`That time fits ${p.per_session} ${unit} a session, so you reach ${p.reachable_quantity} of ${target} by ${formatDate(this.due)}. You need about ${Math.ceil(perDay)} a day. Pick one:`),
          ),
      )
      .child(fixes);
  }

  kindTile(ui, id, icon, kind, title, body) {
    const t = ui.t;
    const on = this.kind === kind;
    return v_flex()
      .id(id)
      .flex_1()
      .gap(6)
      .p(13)
      .rounded(8)
      .border_1()
      .cursor_pointer()
      .border_color(on ? t.accent : t.input)
      .bg(on ? alpha(t.accent, 0.1) : t.raised)
      .on_click((e, cx) => this.set("kind", kind, cx))
      .child(ui.icon(icon, 18, on ? t.accent : t.muted))
      .child(div().text_size(13.5).font_medium().child(title))
      .child(ui.muted(body, { size: 12 }));
  }

  render(ui) {
    const t = ui.t;
    const two = (a, b) => h_flex().w_full().items_start().gap(12).child(div().flex_1().min_w(0).child(a)).child(div().flex_1().min_w(0).child(b ?? div()));
    const el = v_flex().gap(16).child(ui.field("Title", this.title.el(ui)));
    if (!this.goal)
      el.child(
        ui.field(
          "What kind of goal?",
          h_flex()
            .gap(12)
            .child(this.kindTile(ui, "goal-kind-q", "hash", "quantity", "A number to reach", "Such as 300 problems. Gwen plans a session for it each day."))
            .child(this.kindTile(ui, "goal-kind-t", "list-checks", "tasks", "A set of tasks", "Finish tasks you add, or ones the assistant proposes.")),
        ),
      );
    if (this.quantity) {
      el.child(two(ui.field("How many", ui.numberInput("goal-target", this.target, (n, cx) => this.set("target", n, cx), { min: 1, width: 120, empty: true, placeholder: "300" })), ui.field("Of what", this.unit.el(ui))));
      el.child(
        two(
          ui.field("Time you can give it a day", ui.durationInput("goal-daily", this.daily, (v, cx) => this.set("daily", v, cx)), "Each day's session is one block of this time."),
          ui.field(`Minutes for one ${singular(this.unit.value.trim())}`, ui.numberInput("goal-per", this.perUnit, (n, cx) => this.set("perUnit", n, cx), { min: 1, width: 120, empty: true, placeholder: "20" }), "Your average. It sizes each session."),
        ),
      );
    }
    el.child(
      two(
        ui.field("Starts", ui.datePicker("goal-start", this.start, (d, cx) => d && this.set("start", d, cx), { width: 180 })),
        ui.field("Due", ui.datePicker("goal-due", this.due, (d, cx) => this.set("due", d, cx), { width: 180, min: this.start }), this.due && this.start && this.due < this.start ? "The due day is before the start." : undefined),
      ),
    );
    if (this.quantity && this.preview) el.child(this.fit(ui));
    el.child(
      two(
        ui.field(
          "Project",
          ui.select("goal-project", [{ value: "", label: "None" }, ...gwen.projects.map((p) => ({ value: p.id, label: p.name, dot: p.color }))], this.project, (v, cx) => this.set("project", v, cx)),
        ),
        this.goal
          ? ui.field(
              "Status",
              h_flex().child(
                ui.segmented(
                  "goal-status",
                  [
                    { value: "active", label: "Active" },
                    { value: "done", label: "Done" },
                    { value: "abandoned", label: "Abandoned" },
                  ],
                  this.status,
                  (v, cx) => this.set("status", v, cx),
                ),
              ),
            )
          : null,
      ),
    );
    if (this.template) el.child(ui.field("Daily session repeats", this.repeat.el(ui, this.rule, this.start, (rule, cx) => this.set("rule", rule, cx)), "Which days the goal gets a session on your plan."));

    // SMART -----------------------------------------------------------------
    const measured = this.quantity && this.target > 0 ? `${this.target} ${this.unit.value.trim() || "units"}` : null;
    const timed = this.start && this.due ? `${formatDate(this.start, false)} to ${formatDate(this.due, false)}` : null;
    const reachable = this.quantity && this.preview ? this.preview.reachable_quantity >= this.target : null;
    const lit = (k) => (k === "time" ? Boolean(timed) : k === "measurable" ? Boolean(this.smart.measurable.value.trim() || measured) : Boolean(this.smart[k].value.trim()));
    const filled = smartLetters.filter((l) => lit(l.key)).length;
    const smart = v_flex()
      .gap(10)
      .p(14)
      .rounded(8)
      .border_1()
      .border_color(t.border)
      .bg(alpha(t.raised, 0.5))
      .child(h_flex().items_center().gap(10).child(div().text_size(13).font_semibold().child("Make it SMART")).child(smartBadges(ui, "goal-smart", lit)).child(div().flex_1()).child(ui.dim(`${filled} of 5`)))
      .child(ui.dim("Concrete goals are easier to prioritise. Fill in what you can; the assistant reads these when it plans with you."));
    for (const l of smartLetters.slice(0, 4)) {
      const extra =
        l.key === "measurable" && measured
          ? `Counted: ${measured}.`
          : l.key === "realistic" && reachable != null
            ? reachable
              ? "The time you set a day reaches the target."
              : "The time you set a day falls short of the target; see above."
            : undefined;
      smart.child(ui.field(`${l.letter} · ${l.ask}`, this.smart[l.key].el(ui), extra));
    }
    smart.child(ui.dim(`T · Time-related: ${timed ?? "set the start and due days above."}`));
    el.child(smart);
    return el;
  }
}

/** Goal → Break down: ask for tasks, pick the good ones, add them. */
class Breakdown {
  constructor(app, goal) {
    this.app = app;
    this.goal = goal;
    this.quantity = goal.kind === "quantity";
    this.unit = goal.unit || "units";
    this.instructions = new TextField({ multiline: true, rows: 2, placeholder: this.quantity ? "Arrays and strings first, LeetCode mediums" : "The chapters in order, a review before each test" });
    this.sessions = 7;
    this.run = null;
    this.picked = new Set();
    this.since = null;
    this.unavailable = this.refused = null;
    this.saving = false;
  }

  open(cx) {
    const ui = this.app.ui;
    this.id = ui.openDialog(
      {
        title: `Break down “${this.goal.title}”`,
        width: 720,
        height: 620,
        render: (ui) => this.render(ui),
        onClose: () => {
          if (this.run?.status === "ok") api.RejectLLMRun(this.run.id).catch(() => {});
        },
      },
      cx,
    );
    // The seconds of a request that is thinking move while it waits.
    cx.timer.every(1000, (cx) => {
      if (this.since !== null) cx.notify();
    });
  }

  get tasks() {
    return this.run?.status === "ok" ? (this.run.output?.tasks ?? []) : [];
  }

  ask(cx) {
    if (this.since !== null) return;
    this.since = Date.now();
    this.unavailable = this.refused = null;
    cx.notify();
    cx.spawn(async (cx) => {
      try {
        const r = await api.GoalBreakdown(this.goal.id, { instructions: this.instructions.value, sessions: this.quantity ? this.sessions : 0 });
        this.run = r;
        this.picked = r.status === "ok" ? new Set((r.output?.tasks ?? []).map((_, i) => i)) : new Set();
      } catch (e) {
        const err = apiError(e);
        if (err.code === "unavailable") this.unavailable = err.message;
        else if (err.code === "invalid_request") this.refused = err.message;
        else gwen.fail(e);
      }
      this.since = null;
      cx.notify();
    });
  }

  add(cx) {
    if (!this.run || this.saving) return;
    this.saving = true;
    const run = this.run;
    this.app.spawn(cx, async (cx) => {
      const added = await gwen.act(() => api.AcceptLLMRun(run.id, { indexes: [...this.picked].sort((a, b) => a - b) }));
      this.saving = false;
      if (!added) return;
      const n = added.tasks.length;
      gwen.toast(this.quantity ? `Lined up ${n} ${this.unit} for ${this.goal.title}` : `Added ${n} ${n === 1 ? "task" : "tasks"} to ${this.goal.title}`, "success");
      this.run = null;
      this.app.ui.closeDialog(cx, this.id);
    });
  }

  discard(cx) {
    const run = this.run;
    this.run = null;
    this.picked = new Set();
    this.app.spawn(cx, async (cx) => {
      if (run?.status === "ok") await gwen.run(() => api.RejectLLMRun(run.id));
      this.app.ui.closeDialog(cx, this.id);
    });
  }

  render(ui) {
    const t = ui.t;
    const tasks = this.tasks;
    const failure = this.run?.status === "failed" ? (this.run.output?.error ?? "The assistant did not answer.") : null;
    const busy = this.since !== null;
    const el = v_flex()
      .gap(14)
      .child(ui.muted(this.quantity ? `The assistant lines up the next sessions' ${this.unit}, each saying what to do. Today's session takes the first ones.` : "The assistant proposes tasks that reach this goal by its due day.", { size: 13 }));
    if (this.quantity && tasks.length === 0)
      el.child(
        ui.field(
          "How far ahead",
          h_flex().child(
            ui.segmented(
              "bd-sessions",
              [
                { value: 3, label: "Next 3 sessions" },
                { value: 7, label: "A week" },
                { value: 14, label: "Two weeks" },
              ],
              this.sessions,
              (v, cx) => {
                this.sessions = v;
                cx.notify();
              },
            ),
          ),
          this.goal.progress.lined_up > 0 ? `${this.goal.progress.lined_up} ${this.unit} are already lined up; these come after them.` : `Each session gets its ${this.unit}, one task each, with what to do.`,
        ),
      );
    el.child(ui.field("What should the tasks focus on?", this.instructions.el(ui, { h: 58 }), "Optional."));
    if (tasks.length === 0)
      el.child(
        h_flex()
          .items_center()
          .gap(12)
          .child(ui.button("bd-ask", this.run ? "Ask again" : this.quantity ? `Line up ${this.unit}` : "Propose tasks", (e, cx) => this.ask(cx), { tone: "primary", icon: "sparkles", busy }))
          .child(ui.dim("Nothing is added until you choose.")),
      );
    if (busy) el.child(aiProgress(ui, this.since, this.quantity ? `Lining up ${this.unit}` : "Breaking the goal down"));
    if (this.unavailable) el.child(aiUnavailable(ui, this.app, this.unavailable, "bd-unavailable"));
    if (this.refused) el.child(ui.callout({ tone: "neutral", icon: "circle-alert", text: `${this.refused[0].toUpperCase()}${this.refused.slice(1)}.` }));
    if (failure) el.child(aiFailed(ui, this.app, failure, "bd-failed"));
    if (tasks.length > 0) {
      const pickedMinutes = tasks.reduce((sum, x, i) => sum + (this.picked.has(i) ? x.estimate_minutes : 0), 0);
      // A quantity goal's sessions are shown by day.
      const groups = [];
      tasks.forEach((x, i) => {
        const last = groups[groups.length - 1];
        if (this.quantity && last && last.day === x.due_day) last.indexes.push(i);
        else if (this.quantity || !last) groups.push({ day: this.quantity ? x.due_day : "", indexes: [i] });
        else last.indexes.push(i);
      });
      const allPicked = this.picked.size === tasks.length;
      const box = v_flex()
        .rounded(8)
        .border_1()
        .border_color(t.border)
        .overflow_hidden()
        .child(
          h_flex()
            .items_center()
            .gap(10)
            .px(14)
            .py(8)
            .border_b(1)
            .border_color(t.border)
            .bg(t.raised)
            .child(
              ui.checkbox("bd-all", allPicked, (on, cx) => {
                this.picked = on ? new Set(tasks.map((_, i) => i)) : new Set();
                cx.notify();
              }),
            )
            .child(ui.muted(`${this.picked.size} of ${tasks.length} picked`, { size: 13 }))
            .child(div().flex_1())
            .child(ui.dim(`${formatDuration(pickedMinutes * 60_000)} of work`))
            .child(ui.button("bd-again", "Ask again", (e, cx) => this.ask(cx), { small: true, tone: "ghost", icon: "sparkles", busy })),
        );
      const list = v_flex();
      for (const g of groups) {
        if (this.quantity)
          list.child(
            h_flex()
              .items_center()
              .gap(8)
              .px(14)
              .py(7)
              .border_b(1)
              .border_color(t.border)
              .child(ui.icon("calendar-days", 13))
              .child(div().text_size(12).font_medium().text_color(t.muted).child(formatDate(g.day)))
              .child(ui.dim(`· ${g.indexes.length} ${this.unit} · about ${formatDuration(g.indexes.reduce((sum, i) => sum + tasks[i].estimate_minutes, 0) * 60_000)}`)),
          );
        for (const i of g.indexes) {
          const x = tasks[i];
          const p = priorityOf(x.priority);
          const toggle = (cx) => {
            if (this.picked.has(i)) this.picked.delete(i);
            else this.picked.add(i);
            cx.notify();
          };
          const meta = h_flex().flex_wrap().items_center().gap(12).child(h_flex().items_center().gap(4).child(ui.icon("clock", 12)).child(ui.muted(formatDuration(x.estimate_minutes * 60_000), { size: 12 })));
          if (!this.quantity) meta.child(ui.muted(`Due ${formatDate(x.due_day)}`, { size: 12 })).child(h_flex().items_center().gap(4).child(ui.icon("flag", 12, p.color)).child(ui.text(p.label, { size: 12, color: p.color })));
          if (x.quantity != null && x.quantity !== 1) meta.child(ui.muted(`${x.quantity} ${this.unit}`, { size: 12 }));
          list.child(
            h_flex()
              .items_start()
              .gap(10)
              .px(14)
              .py(9)
              .border_b(1)
              .border_color(t.border)
              .child(div().pt(1).child(ui.checkbox(`bd-pick-${i}`, this.picked.has(i), (on, cx) => toggle(cx))))
              .child(
                v_flex()
                  .id(`bd-row-${i}`)
                  .flex_1()
                  .min_w(0)
                  .gap(4)
                  .cursor_pointer()
                  .on_click((e, cx) => toggle(cx))
                  .child(div().text_size(13).text_color(this.picked.has(i) ? t.foreground : t.muted).child(x.title))
                  .when(Boolean(x.notes), (e) => e.child(div().text_size(12).line_height("17px").text_color(t.muted).child(x.notes.length > 320 ? `${x.notes.slice(0, 320)}…` : x.notes)))
                  .child(meta),
              ),
          );
        }
      }
      box.child(div().id("bd-list").max_h(320).overflow_y_scrollbar().child(list));
      box.child(
        h_flex()
          .justify_end()
          .gap(8)
          .px(14)
          .py(9)
          .bg(t.raised)
          .child(ui.button("bd-discard", "Discard", (e, cx) => this.discard(cx), { disabled: this.saving }))
          .child(
            ui.button("bd-add", this.quantity ? `Line up ${this.picked.size}` : `Add ${this.picked.size} ${this.picked.size === 1 ? "task" : "tasks"}`, (e, cx) => this.add(cx), {
              tone: "primary",
              busy: this.saving,
              disabled: this.picked.size === 0,
            }),
          ),
      );
      el.child(box);
    }
    return el;
  }
}

export default class Goals {
  constructor(app) {
    this.app = app;
    this.status = "active";
    this.goals = [];
    this.loadedGoals = false;
    this.commitments = [];
    this.loaded = {};
    this.smartOpen = new Set();
  }

  sync(cx) {
    const v = gwen.versions;
    const stamp = `${this.status}/${v.goals}/${v.tasks}`;
    if (this.loaded.goals !== stamp) {
      this.loaded.goals = stamp;
      const status = this.status;
      cx.spawn(async (cx) => {
        try {
          const l = await api.ListGoals(status);
          if (this.status === status) {
            this.goals = l.goals ?? [];
            this.loadedGoals = true;
          }
        } catch (e) {
          gwen.fail(e);
        }
        cx.notify();
      });
    }
    if (this.loaded.commitments !== v.goals) {
      this.loaded.commitments = v.goals;
      cx.spawn(async (cx) => {
        this.commitments = (await api.ListCommitments().catch((e) => gwen.fail(e) && null))?.commitments ?? this.commitments;
        cx.notify();
      });
    }
  }

  /** Delete a goal, and by default its open tasks: the problems it lined up and anything else linked to it. */
  deleteGoal(g, cx) {
    const ui = this.app.ui;
    let open = null;
    let withTasks = true;
    let busy = false;
    cx.spawn(async (cx) => {
      try {
        const l = await api.ListTasks({ goal_id: g.id, status: "open" });
        open = (l.tasks ?? []).filter((x) => !x.template_id).length;
      } catch {
        open = 0;
      }
      cx.notify();
    });
    const id = ui.openDialog(
      {
        title: `Delete ${g.title}?`,
        width: 440,
        render: (ui) => {
          const el = v_flex().gap(12).child(ui.muted(`${g.kind === "quantity" ? "Its daily session stops. " : ""}Time already tracked stays in your history.`, { size: 13 }));
          if (open > 0)
            el.child(
              h_flex()
                .items_start()
                .gap(10)
                .p(12)
                .rounded(8)
                .border_1()
                .border_color(ui.t.border)
                .bg(ui.t.raised)
                .child(
                  ui.checkbox("gd-tasks", withTasks, (on, cx) => {
                    withTasks = on;
                    cx.notify();
                  }),
                )
                .child(
                  v_flex()
                    .gap(2)
                    .child(ui.text(`Also delete its ${open} open ${open === 1 ? "task" : "tasks"}`, { size: 13 }))
                    .child(ui.dim(withTasks ? "Done ones stay, so your history adds up." : "They stay, as ordinary tasks on your plan.")),
                ),
            );
          return el;
        },
        actions: (ui) => [
          ui.button("gd-cancel", "Cancel", (e, cx) => ui.closeDialog(cx, id), { tone: "ghost" }),
          ui.button(
            "gd-delete",
            "Delete goal",
            (e, cx) => {
              busy = true;
              this.app.spawn(cx, async (cx) => {
                const ok = await gwen.run(() => api.DeleteGoal(g.id, withTasks && (open ?? 0) > 0));
                busy = false;
                if (!ok) return;
                gwen.toast(`Deleted ${g.title}`, "success");
                ui.closeDialog(cx, id);
              });
            },
            { tone: "danger", busy, disabled: open === null },
          ),
        ],
      },
      cx,
    );
  }

  goalCard(ui, g) {
    const t = ui.t;
    const today = gwen.today();
    const p = g.progress;
    const total = p.done_quantity + p.remaining_quantity;
    const unit = g.kind === "tasks" ? "tasks" : g.unit || "units";
    const left = daysBetween(today, g.due_day);
    const keepingUp = p.actual_per_day >= p.required_per_day;
    const project = gwen.projects.find((x) => x.id === g.project_id);
    const id = `gl-${g.id}`;

    const text = { specific: g.specific, measurable: g.measurable || (g.kind === "quantity" ? `${g.target_quantity} ${g.unit || "units"}` : ""), assignable: g.assignable, realistic: g.realistic };
    const lit = (k) => (k === "time" ? true : Boolean(text[k]));
    const any = Object.values(text).some(Boolean);
    const open = this.smartOpen.has(g.id);
    const smart = v_flex()
      .gap(8)
      .child(
        h_flex()
          .id(`${id}-smart`)
          .items_center()
          .gap(8)
          .when(any, (e) =>
            e.cursor_pointer().on_click((ev, cx) => {
              if (open) this.smartOpen.delete(g.id);
              else this.smartOpen.add(g.id);
              cx.notify();
            }),
          )
          .child(smartBadges(ui, `${id}-sl`, lit))
          .child(ui.dim(any ? (open ? "Hide" : "Why and how") : "Not SMART yet: edit the goal to add it")),
      );
    if (open)
      smart.child(
        v_flex()
          .gap(6)
          .p(10)
          .rounded(8)
          .bg(t.raised)
          .children(
            smartLetters
              .filter((l) => l.key !== "time" && text[l.key])
              .map((l) => h_flex().items_start().gap(8).child(div().w(96).flex_none().text_size(12).font_medium().text_color(t.muted).child(l.name)).child(div().flex_1().text_size(12).line_height("17px").child(text[l.key]))),
          ),
      );

    const stat = (label, value, color) => v_flex().flex_1().min_w(0).gap(2).child(ui.dim(label, { truncate: true })).child(div().text_size(13).font_medium().text_color(color ?? t.foreground).child(value));
    const card = ui
      .panel({ pad: 18, gap: 14 })
      .flex_1()
      .min_w(0)
      .child(
        h_flex()
          .items_start()
          .gap(10)
          .child(
            v_flex()
              .flex_1()
              .min_w(0)
              .gap(4)
              .child(div().truncate().text_size(15).font_semibold().child(g.title))
              .child(
                h_flex()
                  .items_center()
                  .gap(10)
                  .when(Boolean(project), (e) => e.child(projectTag(ui, project)))
                  .child(ui.dim(goalShape(g))),
              ),
          )
          .child(g.status === "active" ? paceChip(ui, p.pace) : ui.badge(g.status === "done" ? "Done" : "Abandoned")),
      )
      .child(smart)
      .child(
        v_flex()
          .gap(8)
          .child(
            h_flex()
              .items_baseline()
              .gap(8)
              .child(div().text_size(22).font_semibold().child(String(p.done_quantity)))
              .child(ui.muted(`of ${total} ${unit}`, { size: 13 }))
              .child(div().flex_1())
              .child(div().text_size(13).font_medium().text_color(t.muted).child(`${Math.round(goalFraction(g) * 100)}%`)),
          )
          .child(goalMeter(ui, g, today, 8)),
      )
      .child(
        h_flex()
          .gap(12)
          .child(stat("Needed a day", p.required_per_day.toFixed(1)))
          .child(stat("Doing a day", p.actual_per_day.toFixed(1), g.status !== "active" ? undefined : keepingUp ? ui.tone("ok") : ui.tone("warn")))
          .child(stat(left >= 0 ? `Due in ${left} ${left === 1 ? "day" : "days"}` : "Overdue", formatDate(g.due_day))),
      )
      .child(ui.dim(`${p.projected_finish_day ? `At this pace you finish on ${formatDate(p.projected_finish_day)}.` : "Not enough done yet to say when you will finish."}${g.kind === "quantity" ? ` ${p.lined_up} lined up.` : ""}`));
    if (g.status === "active" && p.reachable_quantity < total)
      card.child(
        ui.callout({
          tone: "warn",
          icon: "triangle-alert",
          text: `At this time a day you reach ${p.reachable_quantity} of ${total} by ${formatDate(g.due_day)}.`,
          action: ui.button(`${id}-fix`, "Fix", (e, cx) => new GoalForm(this.app, g).open(cx), { small: true }),
        }),
      );
    card.child(
      h_flex()
        .items_center()
        .gap(8)
        .pt(10)
        .border_t(1)
        .border_color(t.border)
        .child(ui.button(`${id}-bd`, "Break down with AI", (e, cx) => new Breakdown(this.app, g).open(cx), { small: true, icon: "sparkles", iconColor: t.accent }))
        .child(div().flex_1())
        .child(ui.iconButton(`${id}-edit`, "pencil", (e, cx) => new GoalForm(this.app, g).open(cx), { tip: `Edit ${g.title}` }))
        .child(ui.iconButton(`${id}-del`, "trash-2", (e, cx) => this.deleteGoal(g, cx), { tip: `Delete ${g.title}` })),
    );
    return card;
  }

  // Commitments -------------------------------------------------------------------

  weekdayDots(ui, id, rule) {
    const t = ui.t;
    const lit = litWeekdays(rule);
    if (!lit) return div().w(140).flex_none().truncate().text_size(12).text_color(t.muted).child(describeRule(rule));
    const letters = ["M", "T", "W", "T", "F", "S", "S"];
    return h_flex()
      .id(id)
      .w(140)
      .flex_none()
      .gap(4)
      .tooltip(describeRule(rule))
      .children(
        weekdayCodes.map((c, i) =>
          div()
            .w(16)
            .h(16)
            .flex()
            .items_center()
            .justify_center()
            .rounded(8)
            .text_size(9)
            .font_semibold()
            .when(lit.has(c), (e) => e.bg(alpha(t.accent, 0.8)).text_color(t.accent_foreground))
            .when(!lit.has(c), (e) => e.bg(t.hover).text_color(t.dim))
            .child(letters[i]),
        ),
      );
  }

  editCommitment(c, cx) {
    const ui = this.app.ui;
    const title = new TextField({ value: c?.title ?? "", placeholder: "Internship" });
    const s = {
      rule: c?.rrule ?? "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR",
      anyTime: c ? c.start_minute == null : false,
      at: c?.start_minute != null ? clockOf(c.start_minute) : "10:00",
      duration: goDuration((c?.duration_minutes ?? 60) * 60_000),
      project: c?.project_id ?? "",
      counts: c?.counts_toward_target ?? true,
      from: c?.active_from ?? gwen.today(),
      until: c?.active_until ?? "",
      saving: false,
    };
    const repeat = new Recurrence("cm-repeat");
    const minutes = () => Math.round(parseDuration(s.duration) / 60_000);
    const set = (k, v, cx) => {
      s[k] = v;
      cx.notify();
    };
    const save = (cx) => {
      if (s.saving) return;
      s.saving = true;
      const fields = {
        title: title.value.trim(),
        rrule: s.rule,
        start_minute: s.anyTime || !s.at ? null : minutesOf(s.at),
        duration_minutes: minutes(),
        project_id: s.project || null,
        counts_toward_target: s.counts,
        active_from: s.from,
        active_until: s.until || null,
      };
      this.app.spawn(cx, async (cx) => {
        const ok = c ? await gwen.act(() => api.PatchCommitment(c.id, { ...fields, rev: c.rev })) : await gwen.act(() => api.CreateCommitment(fields));
        s.saving = false;
        if (ok) ui.closeDialog(cx, id);
      });
    };
    const two = (a, b) => h_flex().w_full().items_start().gap(14).child(div().flex_1().min_w(0).child(a)).child(div().flex_1().min_w(0).child(b ?? div()));
    const id = ui.openDialog(
      {
        title: c ? "Edit commitment" : "New commitment",
        width: 640,
        height: 600,
        render: (ui) =>
          v_flex()
            .gap(16)
            .child(ui.field("Title", title.el(ui)))
            .child(ui.field("Repeats", repeat.el(ui, s.rule, s.from, (r, cx) => set("rule", r, cx))))
            .child(
              two(
                ui.field(
                  "Starts at",
                  h_flex()
                    .items_center()
                    .gap(12)
                    .child(s.anyTime ? div().w(96).h(32).rounded(7).border_1().border_color(ui.t.border).opacity(0.5) : ui.timeInput("cm-at", s.at, (v, cx) => set("at", v, cx)))
                    .child(ui.checkbox("cm-any", s.anyTime, (on, cx) => set("anyTime", on, cx)))
                    .child(ui.muted("Any time", { size: 13 })),
                  s.anyTime ? "Plans leave room for it, wherever it fits." : undefined,
                ),
                ui.field("Lasts", ui.durationInput("cm-len", s.duration, (v, cx) => set("duration", v, cx))),
              ),
            )
            .child(two(ui.field("From", ui.datePicker("cm-from", s.from, (d, cx) => d && set("from", d, cx), { width: 180 })), ui.field("Until", ui.datePicker("cm-until", s.until, (d, cx) => set("until", d, cx), { width: 180, min: s.from, clear: true, placeholder: "No end" }), "Empty for no end.")))
            .child(two(ui.field("Project", ui.select("cm-project", [{ value: "", label: "None" }, ...gwen.projects.map((p) => ({ value: p.id, label: p.name, dot: p.color }))], s.project, (v, cx) => set("project", v, cx)))))
            .child(ui.toggleRow("cm-counts", "Counts toward the daily target", "Turn off for time that is not work, such as a class you only attend.", s.counts, (on, cx) => set("counts", on, cx), { divider: false })),
        actions: (ui) => [
          ui.button("cm-cancel", "Cancel", (e, cx) => ui.closeDialog(cx, id), { tone: "ghost" }),
          ui.button(c ? "cm-save" : "cm-create", c ? "Save commitment" : "Create commitment", (e, cx) => save(cx), {
            tone: "primary",
            busy: s.saving,
            disabled: !title.value.trim() || !s.rule.trim() || !(minutes() > 0 && minutes() <= 1440) || !s.from,
          }),
        ],
      },
      cx,
    );
    title.focus();
  }

  removeCommitment(c, cx) {
    this.app.spawn(cx, async (cx) => {
      if (await this.app.ui.confirm({ title: `Delete ${c.title}?`, body: "It stops reserving time on your plans.", confirm: "Delete commitment", danger: true }, cx)) await gwen.run(() => api.DeleteCommitment(c.id));
    });
  }

  commitmentsPanel(ui) {
    const t = ui.t;
    const panel = ui
      .panel({ title: "Commitments", icon: "calendar-range", actions: ui.button("cm-new", "New commitment", (e, cx) => this.editCommitment(null, cx), { small: true, icon: "plus" }) })
      .child(ui.muted("Recurring time that is not planned work, such as an internship or a class. Plans leave room for it.", { size: 13 }));
    if (this.commitments.length === 0) return panel.child(ui.dim("No commitments.", { size: 13 }));
    return panel.child(
      v_flex().children(
        this.commitments.map((c) => {
          const meta = h_flex().items_center().gap(10);
          if (c.project_id) meta.child(projectTag(ui, gwen.projects.find((p) => p.id === c.project_id)));
          if (!c.counts_toward_target) meta.child(ui.dim("Not counted toward the target"));
          if (c.active_until) meta.child(ui.muted(`Until ${formatDate(c.active_until)}`, { size: 12 }));
          return h_flex()
            .items_center()
            .gap(14)
            .px(8)
            .py(9)
            .rounded(8)
            .hover((s) => s.bg(t.hover))
            .child(this.weekdayDots(ui, `cm-dots-${c.id}`, c.rrule))
            .child(v_flex().flex_1().min_w(0).gap(3).child(div().truncate().text_size(13.5).child(c.title)).child(meta))
            .child(div().w(130).flex_none().flex().justify_end().child(ui.muted(c.start_minute == null ? "Any time" : `${formatClock(c.start_minute)}–${formatClock(c.start_minute + c.duration_minutes)}`, { size: 13 })))
            .child(div().w(60).flex_none().flex().justify_end().child(ui.dim(formatDuration(c.duration_minutes * 60_000), { size: 13 })))
            .child(ui.iconButton(`cm-edit-${c.id}`, "pencil", (e, cx) => this.editCommitment(c, cx), { tip: `Edit ${c.title}` }))
            .child(ui.iconButton(`cm-del-${c.id}`, "trash-2", (e, cx) => this.removeCommitment(c, cx), { tip: `Delete ${c.title}` }));
        }),
      ),
    );
  }

  render(ui) {
    const header = ui.pageHeader(
      "Goals",
      "Targets with a date. Gwen paces them into your daily plan.",
      h_flex()
        .items_center()
        .gap(8)
        .child(
          ui.segmented(
            "gl-status",
            [
              { value: "active", label: "Active" },
              { value: "done", label: "Done" },
              { value: "abandoned", label: "Abandoned" },
              { value: "all", label: "All" },
            ],
            this.status,
            (v, cx) => {
              this.status = v;
              this.sync(cx);
              cx.notify();
            },
          ),
        )
        .child(ui.button("gl-new", "New goal", (e, cx) => new GoalForm(this.app, null).open(cx), { tone: "primary", icon: "plus" })),
    );
    const parts = [header];
    if (this.loadedGoals && this.goals.length === 0)
      parts.push(
        ui.empty({
          icon: "target",
          title: this.status === "active" ? "No active goals" : "Nothing here",
          body: this.status === "active" ? "Try 300 problems by December, or a set of tasks such as finishing a course." : undefined,
          action: this.status === "active" ? ui.button("gl-empty-new", "New goal", (e, cx) => new GoalForm(this.app, null).open(cx), { tone: "primary", icon: "plus" }) : null,
        }),
      );
    else
      for (let i = 0; i < this.goals.length; i += 2)
        parts.push(
          h_flex()
            .w_full()
            .items_start()
            .gap(16)
            .child(this.goalCard(ui, this.goals[i]))
            .child(this.goals[i + 1] ? this.goalCard(ui, this.goals[i + 1]) : div().flex_1()),
        );
    parts.push(this.commitmentsPanel(ui));
    return ui.scroll("gl-scroll", parts);
  }
}
