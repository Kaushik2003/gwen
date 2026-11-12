// Pieces more than one Gwen screen shows: a task's tags, its steps, ticking
// it off, the task form, a goal's meter, the pickers for when and how often,
// and the day's timeline.

import { div } from "gpui-kit";
import { v_flex, h_flex } from "gpui-base";
import { api, gwen } from "./gwen.js";
import { addDays, dateOf, daysBetween, formatClock, formatDate, formatDuration, formatTime, goDuration, minutesOf, parseDuration } from "./format.js";
import { buildRule, clockOf, describeRule, parseRule, weekdayCodes } from "./rrule.js";
import { TextField, alpha, clamp01, priorities, priorityOf, unassignedColor } from "./kit.js";

// Tags -------------------------------------------------------------------

export const efforts = [
  { value: 0, label: "Not rated", hint: "" },
  { value: 1, label: "Easy", hint: "light work" },
  { value: 2, label: "Medium", hint: "" },
  { value: 3, label: "Hard", hint: "a frog: Gwen puts it first" },
];

export const stages = [
  { value: "inbox", label: "Inbox", hint: "Captured, not sorted yet" },
  { value: "todo", label: "To do", hint: "Ready to be planned" },
  { value: "doing", label: "In progress", hint: "Being worked on" },
  { value: "waiting", label: "Waiting", hint: "Handed to someone, or blocked" },
  { value: "someday", label: "Someday", hint: "Parked; never planned" },
];

export const stageLabel = (stage) => stages.find((s) => s.value === stage)?.label ?? stage;

const effortStyle = { 1: { label: "Easy", color: "#4cb782" }, 2: { label: "Medium", color: "#8a8f98" }, 3: { label: "Hard", color: "#eb5757" } };

const tag = (ui, icon, text, color) => h_flex().flex_none().items_center().gap(4).text_size(12).text_color(color).child(ui.icon(icon, 12, color)).child(text);

/** A project's dot and name; "Unassigned" without one. */
export function projectTag(ui, project) {
  return h_flex()
    .min_w(0)
    .items_center()
    .gap(6)
    .text_size(12)
    .text_color(ui.t.muted)
    .child(ui.dot(project?.color ?? unassignedColor, 7))
    .child(div().truncate().child(project?.name ?? "Unassigned"));
}

/** When a task is due, in red once it is late. */
export function dueTag(ui, day, today, done) {
  const n = daysBetween(today, day);
  const text = n === 0 ? "Due today" : n === 1 ? "Due tomorrow" : n < 0 ? `${-n} ${n === -1 ? "day" : "days"} late` : `Due ${formatDate(day)}`;
  const color = done ? ui.t.dim : n < 0 ? ui.tone("danger") : n === 0 ? ui.tone("warn") : ui.t.muted;
  return tag(ui, "calendar-days", text, color);
}

/** A flag in the priority's colour; normal priority shows nothing unless always. */
export function priorityFlag(ui, priority, always) {
  if (priority === 2 && !always) return null;
  const p = priorityOf(priority);
  return tag(ui, "flag", p.label, p.color);
}

/** When a task starts: shown for a start time, or a start day still ahead. */
export function startTag(ui, task, today) {
  if (!task.start_day || (task.start_day <= today && task.start_minute == null)) return null;
  const n = daysBetween(today, task.start_day);
  const day = n === 0 ? "Today" : n === 1 ? "Tomorrow" : n < 0 ? formatDate(task.start_day) : `From ${formatDate(task.start_day)}`;
  return tag(ui, "clock", `${day}${task.start_minute != null ? ` ${formatClock(task.start_minute)}` : ""}`, ui.t.muted);
}

/** How hard a task is; a hard one is a frog, done first. Unrated shows nothing. */
export function effortTag(ui, effort) {
  if (!effort) return null;
  const e = effortStyle[effort];
  return tag(ui, "gauge", e.label, e.color);
}

/** A stage other than To do, and who a waiting task waits on. */
export function stageTag(ui, task) {
  if (task.status === "done" || task.stage === "todo" || !task.stage) return null;
  return ui.badge(task.stage === "waiting" && task.delegated_to ? `Waiting on ${task.delegated_to}` : stageLabel(task.stage));
}

// Goals --------------------------------------------------------------------

const paces = {
  ahead: { label: "Ahead", tone: "ok", icon: "trending-up" },
  on_track: { label: "On track", tone: "accent", icon: "minus" },
  behind: { label: "Behind", tone: "warn", icon: "trending-down" },
};
const paceOf = (p) => paces[p] ?? paces.on_track;

export function paceChip(ui, pace) {
  const x = paceOf(pace);
  return ui.badge(x.label, { tone: x.tone, icon: x.icon });
}

/** "12 / 300 problems" for a quantity goal, "3 / 8 tasks" for a tasks goal. */
export function goalAmount(g) {
  const p = g.progress;
  const unit = g.kind === "tasks" ? "tasks" : g.unit;
  return `${p.done_quantity} / ${p.done_quantity + p.remaining_quantity}${unit ? " " + unit : ""}`;
}

export function goalFraction(g) {
  const total = g.progress.done_quantity + g.progress.remaining_quantity;
  return total > 0 ? g.progress.done_quantity / total : 0;
}

/** How far through its days the goal is today: where an even pace would have it. */
export function evenPaceMark(g, today) {
  if (today < g.start_day || today > g.due_day) return null;
  const days = daysBetween(g.start_day, g.due_day) + 1;
  return clamp01((daysBetween(g.start_day, today) + 1) / days);
}

/** Done against total, in the pace colour, with a mark where an even pace would be today. */
export function goalMeter(ui, g, today, height = 6) {
  return ui.meter(goalFraction(g), { color: ui.tone(paceOf(g.progress.pace).tone), mark: g.status === "active" ? evenPaceMark(g, today) : null, height });
}

// Completing ---------------------------------------------------------------

/**
 * Completing and reopening a task. A task that covers several units of a
 * quantity goal first asks how many were done; a step, or a task whose steps
 * count instead, never asks. Every tick can be undone from its toast.
 */
export class Completer {
  constructor(app) {
    this.app = app;
  }

  async done(t, quantity) {
    const req = quantity != null ? { quantity_done: quantity } : {};
    if (await gwen.act(() => api.CompleteTask(t.id, req))) gwen.toast(`Completed ${t.title}`, "success", { label: "Undo", run: () => gwen.act(() => api.ReopenTask(t.id)) });
  }

  complete(t, hasSteps, cx) {
    if (!(t.quantity != null && t.quantity > 1 && !t.parent_id && !hasSteps)) {
      this.done(t);
      return;
    }
    const ui = this.app.ui;
    let qty = t.quantity;
    const id = ui.openDialog(
      {
        title: `Complete ${t.title}`,
        width: 400,
        render: (ui) =>
          ui.field("How many did you do?", ui.numberInput("complete-qty", qty, (n) => (qty = n ?? 0), { min: 0, width: 110 }), `This task planned ${t.quantity}. Any shortfall moves to the goal's next session.`),
        actions: (ui) => [
          ui.button("complete-cancel", "Cancel", (e, cx) => ui.closeDialog(cx, id), { tone: "ghost" }),
          ui.button(
            "complete-ok",
            "Complete",
            (e, cx) => {
              ui.closeDialog(cx, id);
              this.done(t, qty);
            },
            { tone: "primary" },
          ),
        ],
      },
      cx,
    );
  }

  reopen(t) {
    gwen.act(() => api.ReopenTask(t.id));
  }
}

// Steps ----------------------------------------------------------------------

/** "3 of 6 done", for a task's steps. */
export const stepsDone = (steps) => `${steps.filter((s) => s.status === "done").length} of ${steps.length} done`;

/**
 * A task's steps, indented under it: tick each one, read what it says to do,
 * edit or delete it, or add another. One per screen: `this.steps = new
 * Steps(app, completer)`, then `this.steps.list(ui, parent, steps)`.
 */
export class Steps {
  constructor(app, completer) {
    this.app = app;
    this.completer = completer ?? new Completer(app);
    this.addingTo = null; // the task whose "add step" field is open
    this.addField = null;
    this.open = new Set(); // steps whose long notes are shown in full
    this.busy = false;
  }

  startAdding(parent, cx) {
    this.addingTo = parent.id;
    this.addField = new TextField({
      placeholder: "Add step, then Enter",
      onSubmit: (value, cx) => this.add(parent, value, cx),
      onBlur: (value, cx) => {
        if (!value.trim()) {
          this.addingTo = null;
          cx.notify();
        }
      },
    });
    this.addField.focus();
    cx.notify();
  }

  add(parent, value, cx) {
    const title = value.trim();
    if (!title || this.busy) return;
    this.busy = true;
    this.app.spawn(cx, async () => {
      const created = await gwen.act(() => api.CreateTask({ title, parent_id: parent.id, project_id: parent.project_id ?? null }));
      this.busy = false;
      if (created) this.addField?.set("");
    });
  }

  edit(step, cx) {
    const ui = this.app.ui;
    const title = new TextField({ value: step.title });
    const notes = new TextField({ value: step.notes ?? "", multiline: true, rows: 4 });
    let estimate = step.estimate_minutes ?? null;
    let saving = false;
    const save = (e, cx) => {
      if (!title.value.trim() || saving) return;
      saving = true;
      this.app.spawn(cx, async (cx) => {
        const ok = await gwen.act(() => api.PatchTask(step.id, { title: title.value, notes: notes.value, estimate_minutes: estimate && estimate > 0 ? estimate : null, rev: step.rev }));
        saving = false;
        if (ok) ui.closeDialog(cx, id);
      });
    };
    title.o.onSubmit = (v, cx) => save(null, cx);
    const id = ui.openDialog(
      {
        title: "Edit step",
        width: 500,
        render: (ui) =>
          v_flex()
            .gap(14)
            .child(ui.field("Title", title.el(ui)))
            .child(ui.field("What to do", notes.el(ui, { h: 96 }), "Where to find it, the idea to try, how you know it's done."))
            .child(ui.field("Estimate", ui.minutesInput("step-estimate", estimate, (m) => (estimate = m), { empty: true }))),
        actions: (ui) => [ui.button("step-cancel", "Cancel", (e, cx) => ui.closeDialog(cx, id), { tone: "ghost" }), ui.button("step-save", "Save step", save, { tone: "primary", busy: saving })],
      },
      cx,
    );
    title.focus();
  }

  remove(step, cx) {
    this.app.spawn(cx, async (cx) => {
      if (await this.app.ui.confirm({ title: `Delete ${step.title}?`, body: "Only this step goes.", confirm: "Delete step", danger: true }, cx)) await gwen.run(() => api.DeleteTask(step.id));
    });
  }

  row(ui, s) {
    const t = ui.t;
    const isDone = s.status === "done";
    const long = (s.notes ?? "").length > 140 || (s.notes ?? "").includes("\n");
    const open = this.open.has(s.id);
    const notes = s.notes ? (open || !long ? s.notes : `${s.notes.split("\n")[0].slice(0, 140)}…`) : "";
    const body = v_flex().flex_1().min_w(0).gap(2).child(div().text_size(13).text_color(isDone ? t.dim : t.foreground).when(isDone, (e) => e.line_through()).child(s.title));
    if (notes) {
      const n = div().id(`step-notes-${s.id}`).text_size(12).line_height("17px").text_color(t.muted).child(notes);
      if (long)
        n.cursor_pointer().on_click((e, cx) => {
          if (open) this.open.delete(s.id);
          else this.open.add(s.id);
          cx.notify();
        });
      body.child(n);
    }
    return h_flex()
      .id(`step-${s.id}`)
      .w_full()
      .items_start()
      .gap(9)
      .px(8)
      .py(5)
      .rounded(6)
      .hover((st) => st.bg(t.hover))
      .child(div().pt(1).child(ui.checkbox(`step-check-${s.id}`, isDone, (on, cx) => (on ? this.completer.complete(s, false, cx) : this.completer.reopen(s)), { round: true, size: 15 })))
      .child(body)
      .when(s.estimate_minutes != null, (e) => e.child(div().flex_none().pt(1).text_size(12).text_color(t.dim).child(formatDuration(s.estimate_minutes * 60_000))))
      .child(ui.iconButton(`step-edit-${s.id}`, "pencil", (e, cx) => this.edit(s, cx), { tip: "Edit step", size: 22 }))
      .child(ui.iconButton(`step-del-${s.id}`, "trash-2", (e, cx) => this.remove(s, cx), { tip: "Delete step", size: 22 }));
  }

  /** The steps of parent, with "Add step" under them. o: {adding: false to leave that out}. */
  list(ui, parent, steps, o = {}) {
    const t = ui.t;
    const el = v_flex().ml(26).pl(8).border_l(1).border_color(t.border).children(steps.map((s) => this.row(ui, s)));
    if (o.adding !== false && parent.status === "open") {
      if (this.addingTo === parent.id && this.addField) el.child(h_flex().items_center().gap(8).px(8).py(3).child(ui.icon("plus", 13, t.dim)).child(this.addField.el(ui, { h: 26, bare: true })));
      else
        el.child(
          h_flex()
            .id(`step-add-${parent.id}`)
            .items_center()
            .gap(8)
            .px(8)
            .py(5)
            .rounded(6)
            .cursor_pointer()
            .text_size(12.5)
            .text_color(t.dim)
            .hover((st) => st.text_color(t.foreground))
            .on_click((e, cx) => this.startAdding(parent, cx))
            .child(ui.icon("plus", 13, t.dim))
            .child("Add step"),
        );
    }
    return el;
  }
}

// When, and how often ----------------------------------------------------------

/** The instant of minute on the calendar date day, as a block's start. */
export function atOn(day, minute) {
  const [y, m, d] = day.split("-").map(Number);
  return new Date(y, m - 1, d, Math.floor(minute / 60), minute % 60).getTime();
}

/** The next quarter hour from now, in minutes after midnight. */
function nextQuarter() {
  const d = new Date();
  return Math.min(23 * 60 + 45, Math.ceil((d.getHours() * 60 + d.getMinutes() + 1) / 15) * 15);
}

/** Today or Tomorrow in one click, or any date. onChange(day, cx). */
function dayChoice(ui, id, day, today, onChange, o = {}) {
  const tomorrow = addDays(today, 1);
  return h_flex()
    .items_center()
    .gap(8)
    .child(
      ui.segmented(
        `${id}-quick`,
        [
          { value: "today", label: "Today" },
          { value: "tomorrow", label: "Tomorrow" },
        ],
        day === today ? "today" : day === tomorrow ? "tomorrow" : "",
        (v, cx) => onChange(v === "today" ? today : tomorrow, cx),
        { small: true },
      ),
    )
    .child(ui.datePicker(`${id}-date`, day, onChange, { min: today, width: 150, clear: o.clear }));
}

/**
 * When a task starts: a day, Today or Tomorrow in one click or any date, and
 * an optional time on it. onChange(day, minute | null, cx).
 */
export function startInput(ui, id, day, minute, today, onChange) {
  return h_flex()
    .flex_wrap()
    .items_center()
    .gap(8)
    .child(dayChoice(ui, id, day, today, (d, cx) => onChange(d, d ? minute : null, cx), { clear: true }))
    .child(ui.muted("at"))
    .child(ui.timeInput(`${id}-time`, minute != null ? clockOf(minute) : "", (hhmm, cx) => onChange(day || today, hhmm ? minutesOf(hhmm) : null, cx), { empty: true }));
}

/**
 * When a task happens: left to the planner, or at a time the person sets.
 * value: {day, minute, length (Go duration)} or null. onChange(block | null, cx).
 */
export function scheduleInput(ui, id, value, today, onChange) {
  const el = v_flex()
    .gap(10)
    .child(
      h_flex().child(
        ui.segmented(
          `${id}-mode`,
          [
            { value: "auto", label: "Gwen fits it in", icon: "sparkles" },
            { value: "set", label: "At a set time", icon: "calendar-clock" },
          ],
          value ? "set" : "auto",
          (v, cx) => {
            onChange(v === "set" ? (value ?? { day: today, minute: nextQuarter(), length: "30m" }) : null, cx);
            cx.notify();
          },
        ),
      ),
    );
  if (value)
    el.child(
      h_flex()
        .flex_wrap()
        .items_center()
        .gap(8)
        .p(10)
        .rounded(8)
        .border_1()
        .border_color(ui.t.border)
        .child(dayChoice(ui, id, value.day, today, (d, cx) => d && onChange({ ...value, day: d }, cx)))
        .child(ui.muted("at"))
        .child(ui.timeInput(`${id}-time`, clockOf(value.minute), (hhmm, cx) => hhmm && onChange({ ...value, minute: minutesOf(hhmm) }, cx)))
        .child(ui.muted("for"))
        .child(ui.durationInput(`${id}-len`, value.length, (length, cx) => onChange({ ...value, length }, cx))),
    );
  return el;
}

const workweek = ["MO", "TU", "WE", "TH", "FR"];
const dayLetters = { MO: "M", TU: "T", WE: "W", TH: "T", FR: "F", SA: "S", SU: "S" };
const dayNames = { MO: "Monday", TU: "Tuesday", WE: "Wednesday", TH: "Thursday", FR: "Friday", SA: "Saturday", SU: "Sunday" };

function ruleMode(value, allowNone) {
  if (!value.trim()) return allowNone ? "none" : "daily";
  const r = parseRule(value);
  if (!r) return "custom";
  if (r.freq === "DAILY") return "daily";
  if (r.freq === "MONTHLY") return "monthly";
  return r.interval === 1 && r.byDay.length === 5 && workweek.every((d) => r.byDay.includes(d)) ? "weekdays" : "weekly";
}

/**
 * Picks a repeat rule without typing one: every day, weekdays, chosen days,
 * or a day of the month, with an optional interval and end date. Custom keeps
 * the rule editable as text for anything else the planner accepts.
 *
 * Keep one per form: `this.repeat = new Recurrence(id, {allowNone})`, then
 * `this.repeat.el(ui, value, anchorDay, (rule, cx) => ...)`.
 */
export class Recurrence {
  constructor(id, o = {}) {
    this.id = id;
    this.allowNone = Boolean(o.allowNone);
    this.mode = null;
    this.custom = new TextField({ placeholder: "FREQ=WEEKLY;INTERVAL=2;BYDAY=MO", onChange: (v, cx) => this.onChange?.(v.toUpperCase(), cx) });
  }

  el(ui, value, anchorDay, onChange) {
    const t = ui.t;
    const id = this.id;
    this.onChange = onChange;
    this.mode ??= ruleMode(value, this.allowNone);
    const mode = this.mode;
    const anchor = dateOf(anchorDay);
    const anchorCode = weekdayCodes[(anchor.getDay() + 6) % 7];
    const rule = parseRule(value) ?? { freq: "DAILY", interval: 1, byDay: [], byMonthDay: 0, until: null };
    const pick = (m, cx) => {
      this.mode = m;
      const keep = { interval: 1, until: rule.until };
      if (m === "none") onChange("", cx);
      else if (m === "daily") onChange(buildRule({ ...rule, ...keep, freq: "DAILY", byDay: [], byMonthDay: 0 }), cx);
      else if (m === "weekdays") onChange(buildRule({ ...rule, ...keep, freq: "WEEKLY", byDay: workweek, byMonthDay: 0 }), cx);
      else if (m === "weekly") onChange(buildRule({ ...rule, ...keep, freq: "WEEKLY", byDay: rule.freq === "WEEKLY" && rule.byDay.length ? rule.byDay : [anchorCode], byMonthDay: 0 }), cx);
      else if (m === "monthly") onChange(buildRule({ ...rule, ...keep, freq: "MONTHLY", byDay: [], byMonthDay: Math.min(anchor.getDate(), 28) }), cx);
      else this.custom.set(value);
      cx.notify();
    };
    const set = (patch, cx) => {
      onChange(buildRule({ ...rule, ...patch }), cx);
      cx.notify();
    };
    const unit = mode === "daily" ? "day" : mode === "monthly" ? "month" : "week";
    const options = [
      ...(this.allowNone ? [{ value: "none", label: "Doesn't repeat" }] : []),
      { value: "daily", label: "Daily" },
      { value: "weekdays", label: "Weekdays" },
      { value: "weekly", label: "Some days" },
      { value: "monthly", label: "Monthly" },
      { value: "custom", label: "Custom" },
    ];
    const el = v_flex().gap(10).child(h_flex().child(ui.segmented(`${id}-mode`, options, mode, pick, { small: true })));
    if (mode === "weekly")
      el.child(
        h_flex()
          .gap(6)
          .children(
            weekdayCodes.map((d) => {
              const on = rule.byDay.includes(d);
              return div()
                .id(`${id}-day-${d}`)
                .flex()
                .items_center()
                .justify_center()
                .w(30)
                .h(30)
                .rounded(15)
                .text_size(12)
                .font_semibold()
                .cursor_pointer()
                .tooltip(dayNames[d])
                .when(on, (e) => e.bg(t.accent).text_color(t.accent_foreground))
                .when(!on, (e) => e.border_1().border_color(t.input).text_color(t.muted))
                .on_click((e, cx) => {
                  const next = on ? rule.byDay.filter((x) => x !== d) : [...rule.byDay, d];
                  if (next.length) set({ byDay: next }, cx);
                })
                .child(dayLetters[d]);
            }),
          ),
      );
    if (mode === "monthly")
      el.child(
        h_flex()
          .items_center()
          .gap(8)
          .child(ui.muted("On day"))
          .child(ui.numberInput(`${id}-monthday`, rule.byMonthDay || 1, (n, cx) => set({ byMonthDay: n ?? 1 }, cx), { min: 1, max: 28, width: 56 }))
          .child(ui.muted("of the month")),
      );
    if (mode !== "none" && mode !== "custom" && mode !== "weekdays")
      el.child(
        h_flex()
          .items_center()
          .gap(8)
          .child(ui.muted("Every"))
          .child(ui.numberInput(`${id}-interval`, rule.interval, (n, cx) => set({ interval: n ?? 1 }, cx), { min: 1, max: 99, width: 56 }))
          .child(ui.muted(rule.interval === 1 ? unit : unit + "s")),
      );
    if (mode === "custom") {
      if (!this.custom.focused && this.custom.value !== value) this.custom.set(value);
      el.child(this.custom.el(ui, { mono: true }));
    }
    if (mode !== "none" && mode !== "custom")
      el.child(
        h_flex()
          .items_center()
          .gap(10)
          .child(ui.toggle(`${id}-until-on`, rule.until !== null, (on, cx) => set({ until: on ? anchorDay : null }, cx)))
          .child(ui.muted("Ends on a date"))
          .when(rule.until !== null, (e) => e.child(ui.datePicker(`${id}-until`, rule.until, (d, cx) => d && set({ until: d }, cx), { min: anchorDay, width: 150 }))),
      );
    if (mode !== "none" && value) el.child(ui.dim(describeRule(value)));
    return el;
  }
}

// The task form ------------------------------------------------------------------

/**
 * The dialog that makes or changes a task. `new TaskForm(app, {task, goals,
 * defaultProject, defaultStage}).open(cx)`; task null makes a new one. Call it
 * from a handler.
 */
export class TaskForm {
  constructor(app, o = {}) {
    const task = o.task ?? null;
    this.app = app;
    this.task = task;
    this.goals = o.goals ?? [];
    this.today = gwen.today();
    this.title = new TextField({ value: task?.title ?? "", placeholder: "Call the bank", onSubmit: (v, cx) => this.save(cx) });
    this.notes = new TextField({ value: task?.notes ?? "", multiline: true, rows: 2 });
    this.delegatedTo = new TextField({ value: task?.delegated_to ?? "", placeholder: "Alex" });
    // A new task starts today; how long it takes and the rest wait under More.
    this.startDay = task ? (task.start_day ?? "") : this.today;
    this.startMinute = task?.start_minute ?? null;
    this.more = Boolean(task);
    this.project = task ? (task.project_id ?? "") : (o.defaultProject ?? "");
    this.goal = task?.goal_id ?? "";
    this.priority = task?.priority ?? 2;
    this.due = task?.due_day ?? "";
    this.estimate = task?.estimate_minutes ?? 0;
    this.quantity = task?.quantity ?? null;
    this.rule = task?.rrule ?? "";
    this.effort = task?.effort ?? 0;
    this.stage = task?.stage ?? o.defaultStage ?? "todo";
    this.block = null; // a time set by hand: a pinned block on the plan
    this.saving = false;
    this.repeat = new Recurrence("task-repeat", { allowNone: true });
    // Occurrences of a repeating task cannot repeat themselves.
    this.canRepeat = !task?.template_id;
  }

  open(cx) {
    const ui = this.app.ui;
    this.id = ui.openDialog(
      {
        title: this.task ? "Edit task" : "New task",
        width: 620,
        render: (ui) => this.render(ui),
        actions: (ui) => [
          ui.button("task-cancel", "Cancel", (e, cx) => ui.closeDialog(cx, this.id), { tone: "ghost" }),
          ui.button("task-save", this.task ? "Save task" : "Create task", (e, cx) => this.save(cx), { tone: "primary", busy: this.saving, disabled: !this.title.value.trim() }),
        ],
      },
      cx,
    );
    this.title.focus();
  }

  save(cx) {
    const task = this.task;
    if (!this.title.value.trim() || this.saving) return;
    this.saving = true;
    cx.notify();
    this.app.spawn(cx, async (cx) => {
      const minutes = this.estimate;
      const at = this.startDay && this.startMinute != null ? this.startMinute : null;
      const block = this.block;
      const blockMinutes = block ? Math.round(parseDuration(block.length) / 60_000) : 0;
      const schedule = block && blockMinutes > 0 ? { day: block.day, start_at: atOn(block.day, block.minute), planned_minutes: blockMinutes } : null;
      const goalKind = this.goals.find((g) => g.id === this.goal)?.kind;
      const fields = {
        title: this.title.value,
        start_day: this.startDay || null,
        start_minute: at,
        project_id: this.project || null,
        priority: this.priority,
        due_day: this.due || null,
        estimate_minutes: minutes > 0 ? minutes : null,
        notes: this.notes.value,
        goal_id: this.goal || null,
        quantity: goalKind === "quantity" && this.quantity > 0 ? this.quantity : null,
        effort: this.effort || null,
        stage: this.stage,
        delegated_to: this.stage === "waiting" ? this.delegatedTo.value : "",
      };
      const repeat = this.canRepeat && this.rule !== (task?.rrule ?? "") ? { rrule: this.rule || null } : {};
      let ok;
      if (task) ok = await gwen.act(() => api.PatchTask(task.id, { ...fields, ...repeat, rev: task.rev }));
      else {
        const req = { ...fields };
        if (!req.effort) delete req.effort;
        if (!req.start_day) delete req.start_day;
        if (req.start_minute == null) delete req.start_minute;
        if (this.rule) req.rrule = this.rule;
        else if (schedule) req.schedule = schedule;
        ok = await gwen.act(() => api.CreateTask(req));
      }
      // Gwen adds new and changed tasks to today's plan by herself; a time set by hand on an existing task moves it there.
      if (ok && task && schedule && !this.rule) await gwen.act(() => api.ScheduleTask({ task_id: task.id, ...schedule }));
      this.saving = false;
      if (ok) this.app.ui.closeDialog(cx, this.id);
    });
  }

  render(ui) {
    const task = this.task;
    const goalKind = this.goals.find((g) => g.id === this.goal)?.kind;
    const redraw = (cx) => cx.notify();
    const el = v_flex().gap(16).child(ui.field("Title", this.title.el(ui)));
    if (!this.block)
      el.child(
        ui.field(
          "From",
          startInput(ui, "task-start", this.startDay, this.startMinute, this.today, (day, minute, cx) => {
            this.startDay = day;
            this.startMinute = minute;
            redraw(cx);
          }),
          this.startMinute != null ? undefined : "Add a time if it should start at one.",
        ),
      );
    el.child(
      ui.field(
        "How long",
        ui.minutesInput("task-estimate", this.estimate, (m, cx) => {
          this.estimate = m ?? 0;
          if (this.block && this.estimate > 0) this.block = { ...this.block, length: goDuration(this.estimate * 60_000) };
          redraw(cx);
        }),
        this.estimate > 0 ? "Gwen plans time for it." : "Leave at 0 for a quick to-do you just tick off. Set a time to have Gwen plan it.",
      ),
    );
    if (!this.rule)
      el.child(
        ui.field(
          task ? "Put it on the plan" : "When",
          scheduleInput(ui, "task-sched", this.block, this.today, (b, cx) => {
            this.block = b && !this.block && this.estimate > 0 ? { ...b, length: goDuration(this.estimate * 60_000) } : b;
            redraw(cx);
          }),
          this.block
            ? "Blocked on your plan at exactly this time; everything else is planned around it."
            : task
              ? "Set a time to move it there; it leaves any other day it was on."
              : "Gwen puts it on today's plan by urgency, or set the time yourself.",
        ),
      );
    el.child(
      ui.field(
        "How hard",
        h_flex().child(
          ui.segmented(
            "task-effort",
            efforts.map((e) => ({ value: e.value, label: e.label })),
            this.effort,
            (v, cx) => {
              this.effort = v;
              redraw(cx);
            },
          ),
        ),
        this.effort === 3 ? "Eat the frog: hard work goes first in your day, in your prime time when you set one." : undefined,
      ),
    );
    if (!this.more)
      return el.child(
        h_flex().child(
          ui.button(
            "task-more",
            "More: notes, project, goal, priority, due date, stage, repeat",
            (e, cx) => {
              this.more = true;
              redraw(cx);
            },
            { tone: "ghost", small: true, icon: "sliders-horizontal" },
          ),
        ),
      );
    const projects = gwen.projects.filter((p) => !p.archived_at || p.id === this.project);
    el.child(ui.field("Notes", this.notes.el(ui, { h: 60 })));
    el.child(
      h_flex()
        .gap(14)
        .items_start()
        .child(
          div().flex_1().min_w(0).child(
            ui.field(
              "Project",
              ui.select("task-project", [{ value: "", label: "None" }, ...projects.map((p) => ({ value: p.id, label: p.name, dot: p.color }))], this.project, (v, cx) => {
                this.project = v;
                redraw(cx);
              }),
            ),
          ),
        )
        .child(
          div().flex_1().min_w(0).child(
            ui.field(
              "Goal",
              ui.select("task-goal", [{ value: "", label: "None" }, ...this.goals.map((g) => ({ value: g.id, label: g.title }))], this.goal, (v, cx) => {
                this.goal = v;
                redraw(cx);
              }),
              this.goals.length === 0 ? "No active goals." : undefined,
            ),
          ),
        ),
    );
    el.child(
      ui.field(
        "Priority",
        h_flex().child(
          ui.segmented(
            "task-priority",
            priorities.map((p) => ({ value: p.value, label: p.label })),
            this.priority,
            (v, cx) => {
              this.priority = v;
              redraw(cx);
            },
          ),
        ),
      ),
    );
    el.child(
      h_flex()
        .gap(14)
        .items_start()
        .child(
          div().flex_1().min_w(0).child(
            ui.field(
              "Due",
              ui.datePicker("task-due", this.due, (d, cx) => {
                this.due = d;
                redraw(cx);
              }, { clear: true }),
            ),
          ),
        )
        .child(
          div().flex_1().min_w(0).child(
            ui.field(
              "Stage",
              ui.select("task-stage", stages.map((s) => ({ value: s.value, label: s.label })), this.stage, (v, cx) => {
                this.stage = v;
                redraw(cx);
              }),
              stages.find((s) => s.value === this.stage)?.hint,
            ),
          ),
        ),
    );
    if (this.stage === "waiting") el.child(ui.field("Waiting on", this.delegatedTo.el(ui), "Who you handed it to, or what it waits for."));
    if (goalKind === "quantity")
      el.child(ui.field("Covers how many of the goal", ui.numberInput("task-qty", this.quantity, (n) => (this.quantity = n), { min: 1, width: 110, empty: true }), "Completing it asks how many you actually did."));
    if (this.canRepeat)
      el.child(
        ui.field(
          "Repeats",
          this.repeat.el(ui, this.rule, this.startDay || this.today, (rule, cx) => {
            this.rule = rule;
            redraw(cx);
          }),
          this.rule ? "A repeating task puts a fresh copy on each matching day's plan." : undefined,
        ),
      );
    return el;
  }
}

// The timeline ---------------------------------------------------------------------

const kindLabel = { work: "Work", break_manual: "Break", break_auto: "Automatic break" };
const hour = 3_600_000;
const floorHour = (ms) => {
  const d = new Date(ms);
  d.setMinutes(0, 0, 0);
  return d.getTime();
};
const ceilHour = (ms) => (floorHour(ms) === ms ? ms : floorHour(ms) + hour);
/** Whether text on this colour should be dark. */
const lightColor = (c) => {
  const h = String(c).replace("#", "");
  const [r, g, b] = [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16));
  return (r * 299 + g * 587 + b * 114) / 1000 > 150;
};

/**
 * One day against the clock: what was planned on the top lane, what was
 * tracked below it, coloured by project with breaks greyed, a legend, and a
 * line at now while the day is live. Hovering a block shows its time.
 *
 * o: {id, segments, colors (Map project id | null -> colour), names (Map),
 * now, live, from, to, planned: [{id, title, start, minutes, color, done}],
 * fixed: [{id, title, start, minutes}], onSegment(segment, cx), density}.
 */
export function timeline(ui, o) {
  const t = ui.t;
  const id = o.id ?? "tl";
  const segments = o.segments ?? [];
  const planned = o.planned ?? [];
  const fixed = o.fixed ?? [];
  const { now, live } = o;
  const ends = (s) => s.ended_at ?? (live ? now : s.started_at);
  const start = floorHour(Math.min(o.from, ...segments.map((s) => s.started_at), ...planned.map((p) => p.start), ...fixed.map((f) => f.start)));
  const end = Math.max(
    start + hour,
    ceilHour(Math.max(o.to, ...segments.map(ends), ...planned.map((p) => p.start + p.minutes * 60_000), ...fixed.map((f) => f.start + f.minutes * 60_000), ...(live ? [now] : []))),
  );
  const span = end - start;
  const pct = (at) => `${Math.round(((at - start) / span) * 10000) / 100}%`;
  const width = (a, b) => `${Math.round(Math.max(((b - a) / span) * 100, 0.35) * 100) / 100}%`;
  const frac = (a, b) => (b - a) / span;
  const hours = Math.round(span / hour);
  const step = hours <= 12 ? 1 : hours <= 20 ? 2 : 3;
  const ticks = [];
  for (let at = start; at <= end; at += hour) ticks.push(at);
  const nowIn = live && now >= start && now <= end;
  const gutter = 64;
  const grid = () => ticks.slice(1, -1).map((at) => div().absolute().top(0).bottom(0).w(1).left(pct(at)).bg(alpha(t.border, 0.7)));
  const lane = (label, icon, height, children) =>
    h_flex()
      .mt(8)
      .items_center()
      .child(h_flex().w(gutter).flex_none().items_center().gap(6).text_size(11).font_medium().text_color(t.dim).child(ui.icon(icon, 12, t.dim)).child(label))
      .child(
        div()
          .relative()
          .flex_1()
          .h(height)
          .rounded(8)
          .bg(t.raised)
          .overflow_hidden()
          .when(nowIn, (e) => e.child(div().absolute().top(0).bottom(0).left(0).w(pct(now)).bg(alpha(t.foreground, 0.04))))
          .children(grid())
          .children(children)
          .when(nowIn, (e) => e.child(div().absolute().top(0).bottom(0).w(2).left(pct(now)).bg(t.accent))),
      );
  const block = (bid, left, w, tip) => div().id(bid).absolute().top(4).bottom(4).left(left).w(w).rounded(4).overflow_hidden().tooltip(tip);

  const axis = div()
    .relative()
    .ml(gutter)
    .h(18)
    .children(
      ticks
        .map((at, i) => {
          const edge = i === 0 || i === ticks.length - 1;
          const nearNow = nowIn && Math.abs(at - now) / span < (edge ? 0.12 : 0.05);
          if (i % step !== 0 || nearNow || i === ticks.length - 1) return null;
          return div().absolute().top(0).left(pct(at)).text_size(11).text_color(t.dim).whitespace_nowrap().child(formatTime(at).replace(":00 ", " "));
        })
        .filter(Boolean),
    )
    .when(nowIn, (e) => e.child(div().absolute().top(0).left(pct(now)).px(5).rounded(8).bg(alpha(t.accent, 0.18)).text_size(11).font_semibold().text_color(t.accent).whitespace_nowrap().child(formatTime(now))));

  const el = v_flex().w_full().child(axis);
  if (planned.length + fixed.length > 0) {
    const blocks = [];
    for (const f of fixed) {
      const stop = f.start + f.minutes * 60_000;
      const b = block(`${id}-f-${f.id}`, pct(f.start), width(f.start, stop), `${f.title}\n${formatTime(f.start)} – ${formatTime(stop)} · ${formatDuration(f.minutes * 60_000)}`)
        .bg(alpha(t.muted, 0.22))
        .border_1()
        .border_color(alpha(t.muted, 0.5));
      if (frac(f.start, stop) > 0.035) b.child(div().px(6).text_size(11).line_height("18px").text_color(t.muted).truncate().child(f.title));
      blocks.push(b);
    }
    for (const p of planned) {
      const stop = p.start + p.minutes * 60_000;
      const b = block(`${id}-p-${p.id}`, pct(p.start), width(p.start, stop), `${p.title}\n${formatTime(p.start)} · ${formatDuration(p.minutes * 60_000)}${p.done ? " · done" : ""}`)
        .bg(alpha(p.color, p.done ? 0.4 : 0.2))
        .border_1()
        .border_color(alpha(p.color, 0.7));
      if (frac(p.start, stop) > 0.035) b.child(div().px(6).text_size(11).line_height("18px").font_medium().text_color(t.foreground).truncate().child(p.title));
      blocks.push(b);
    }
    el.child(lane("Plan", "calendar-clock", 28, blocks));
  }
  const tracked = segments.map((s) => {
    const e = ends(s);
    const work = s.kind === "work";
    const open = live && s.ended_at == null;
    const color = work ? (o.colors.get(s.project_id ?? null) ?? unassignedColor) : alpha(t.muted, 0.3);
    const who = work ? (o.names.get(s.project_id ?? null) ?? "Unassigned") : (kindLabel[s.kind] ?? s.kind);
    const b = block(`${id}-s-${s.id}`, pct(s.started_at), width(s.started_at, e), `${who}\n${formatTime(s.started_at)} – ${open ? "now" : formatTime(e)} · ${formatDuration(e - s.started_at)}${open ? " · in progress" : ""}`).bg(color);
    if (frac(s.started_at, e) > 0.035) b.child(div().px(6).text_size(11).line_height("28px").font_medium().text_color(work ? (lightColor(color) ? "#111111" : "#ffffff") : t.muted).truncate().child(who));
    if (o.onSegment) b.cursor_pointer().hover((st) => st.opacity(0.85)).on_click((ev, cx) => o.onSegment(s, cx));
    return b;
  });
  if (segments.length === 0) tracked.push(div().absolute().inset_0().flex().items_center().px(12).text_size(12).text_color(t.dim).child("Nothing tracked yet"));
  el.child(lane("Tracked", "radio", 36, tracked));

  if (o.density) {
    // Real tracked minutes per quarter hour: each bar is work time overlapping its slice.
    const bucket = 15 * 60_000;
    const n = Math.max(1, Math.round(span / bucket));
    const buckets = new Array(n).fill(0);
    for (const s of segments) {
      if (s.kind !== "work") continue;
      const a = s.started_at;
      const b = ends(s);
      for (let i = Math.max(0, Math.floor((a - start) / bucket)); i <= Math.min(n - 1, Math.ceil((b - start) / bucket)); i++) {
        const bs = start + i * bucket;
        const overlap = Math.min(b, bs + bucket) - Math.max(a, bs);
        if (overlap > 0) buckets[i] += overlap;
      }
    }
    el.child(
      h_flex()
        .mt(8)
        .ml(gutter)
        .h(22)
        .items_end()
        .gap(1)
        .children(buckets.map((ms) => div().flex_1().min_w(0).h(Math.max(Math.round(Math.min(1, ms / bucket) * 22), 2)).bg(ms > 0 ? t.accent : t.border))),
    );
  }

  const legend = new Map();
  let hasBreak = false;
  for (const s of segments) {
    if (s.kind === "work") legend.set(s.project_id ?? null, o.colors.get(s.project_id ?? null) ?? unassignedColor);
    else hasBreak = true;
  }
  if (segments.length > 0 && (legend.size > 0 || hasBreak))
    el.child(
      h_flex()
        .mt(10)
        .ml(gutter)
        .flex_wrap()
        .items_center()
        .gap(14)
        .children([...legend.entries()].map(([pid, color]) => h_flex().items_center().gap(6).text_size(11).text_color(t.muted).child(ui.dot(color, 7)).child(o.names.get(pid) ?? "Unassigned")))
        .when(hasBreak, (e) => e.child(h_flex().items_center().gap(6).text_size(11).text_color(t.muted).child(div().w(9).h(9).rounded(2).bg(alpha(t.muted, 0.3))).child("Break"))),
    );
  return el;
}
