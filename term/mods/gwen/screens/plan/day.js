// The day plan: what is planned against the room in the day, the day's own
// hours, its timeline, the AI planning chat, and the tasks in the order the
// day runs. There is no dragging in the Gwen window: the arrows on each row
// move a task up or down.

import { div } from "gpui-kit";
import { v_flex, h_flex, ScrollHandle } from "gpui-base";
import { api, apiError, gwen, tracking } from "../../lib/gwen.js";
import { addDays, clockValue, formatClock, formatDuration, formatLongDate, formatTime, goDuration, instantOn, minutesOf, parseDuration, relativeDay } from "../../lib/format.js";
import { blockTimes, eventBlocks, groupPlan, leadItem } from "../../lib/plan.js";
import { clockOf, occursOn } from "../../lib/rrule.js";
import { aiFailed, aiProgress, aiUnavailable, providerName } from "../../lib/ai.js";
import { TextField, alpha, unassignedColor } from "../../lib/kit.js";
import { Completer, Steps, projectTag, stepsDone, timeline } from "../../lib/parts.js";

const minutes = (m) => formatDuration(m * 60_000);

/** Messages to start the planning chat from, sent with one click. */
const starters = ["Plan my day", "I'm tired today, keep it light", "Start at 14:00 and work 3 hours", "Most urgent work first, with breaks"];

/** "from 14:00 · 3h for tasks" for a change of hours. */
function hoursText(h) {
  if (!h) return "";
  const parts = [];
  if (h.start_minute != null) parts.push(`from ${formatClock(h.start_minute)}`);
  if (h.work_minutes != null) parts.push(h.work_minutes === 0 ? "no time for tasks" : `${minutes(h.work_minutes)} for tasks`);
  return parts.join(" · ");
}

/** The conversation about one day that ends in a plan; nothing changes until Apply. */
class PlanChat {
  constructor(app, owner) {
    this.app = app;
    this.owner = owner;
    this.day = null;
    this.run = null; // the conversation's latest answer
    this.picked = new Set();
    this.asking = null;
    this.failure = this.unavailable = this.refused = null;
    this.applying = false;
    this.scroll = ScrollHandle.new();
    this.input = new TextField({ multiline: true, rows: 2, placeholder: "How should today go?", onSubmit: (v, cx) => this.send(v, cx) });
  }

  key(day) {
    return `planChat.${day}`;
  }

  /** Shows the chat of a day, bringing back its last conversation. */
  open(day, cx) {
    if (this.day === day) return;
    this.day = day;
    this.run = null;
    this.picked = new Set();
    this.failure = this.unavailable = this.refused = null;
    let id = null;
    try {
      id = localStorage.getItem(this.key(day));
    } catch {}
    if (!id) return;
    cx.spawn(async (cx) => {
      try {
        const r = await api.GetLLMRun(id);
        if (this.day === day && r.kind === "day_plan" && r.subject_id === day && (r.status === "ok" || r.status === "accepted")) this.adopt(r);
      } catch {
        this.remember(null);
      }
      cx.notify();
    });
  }

  remember(id) {
    try {
      if (id) localStorage.setItem(this.key(this.day), id);
      else localStorage.removeItem(this.key(this.day));
    } catch {}
  }

  adopt(r) {
    this.run = r;
    this.picked = new Set((r.output?.items ?? []).map((_, i) => i));
    this.remember(r.id);
    const n = (r.output?.messages ?? []).length;
    if (n) this.scroll.scroll_to_item(n - 1);
  }

  get out() {
    return this.run?.output ?? null;
  }

  /** Proposed, not yet applied. */
  get pending() {
    return this.run?.status === "ok";
  }

  /** What the timeline previews in place of the plan: the picked blocks of an open proposal. */
  get preview() {
    return this.pending ? (this.out?.items ?? []).filter((_, i) => this.picked.has(i)) : null;
  }

  send(message, cx) {
    const m = String(message ?? "").trim();
    if (!m || this.asking) return;
    const day = this.day;
    this.asking = { message: m, since: Date.now() };
    this.input.set("");
    this.failure = this.unavailable = this.refused = null;
    cx.notify();
    cx.spawn(async (cx) => {
      try {
        const r = await api.PlanChat({ day, message: m, run_id: this.run?.id ?? null });
        if (this.day !== day) return;
        if (r.status === "failed") {
          this.failure = r.output?.error ?? "The assistant did not answer.";
          this.input.set(m);
        } else this.adopt(r);
      } catch (e) {
        const err = apiError(e);
        this.input.set(m);
        if (err.code === "unavailable") this.unavailable = err.message;
        else if (err.code === "invalid_request") this.refused = err.message;
        else gwen.fail(e);
      }
      this.asking = null;
      cx.notify();
    });
  }

  apply(cx) {
    if (!this.run || this.applying) return;
    const run = this.run;
    const n = this.picked.size;
    this.applying = true;
    this.app.spawn(cx, async () => {
      const done = await gwen.act(() => api.AcceptLLMRun(run.id, { indexes: [...this.picked].sort((a, b) => a - b) }));
      this.applying = false;
      if (!done) return;
      this.run = { ...run, status: "accepted" };
      gwen.toast(n ? `Planned ${n} ${n === 1 ? "block" : "blocks"} from the chat` : "Cleared the day's plan", "success");
    });
  }

  startOver(cx) {
    const run = this.run;
    this.app.spawn(cx, async () => {
      if (run?.status === "ok") await gwen.run(() => api.RejectLLMRun(run.id));
      this.remember(null);
      this.run = null;
      this.picked = new Set();
      this.failure = this.refused = null;
      this.input.set("");
    });
  }

  bubble(ui, role, text, key, pending) {
    const t = ui.t;
    const user = role === "user";
    const b = div()
      .max_w("85%")
      .px(12)
      .py(8)
      .rounded(11)
      .text_size(13)
      .line_height("19px")
      .when(user, (e) => e.bg(alpha(t.accent, 0.15)).text_color(t.foreground))
      .when(!user, (e) => e.border_1().border_color(t.border).bg(t.raised).text_color(t.foreground))
      .when(Boolean(pending), (e) => e.opacity(0.7))
      .child(text);
    const row = h_flex().w_full().items_end().gap(8).py(4);
    if (user) return row.justify_end().child(b);
    return row.child(ui.face("smiling", 34)).child(b);
  }

  render(ui, plan) {
    const t = ui.t;
    const out = this.out;
    const messages = out?.messages ?? [];
    const blocks = out?.items ?? [];
    const open = this.pending;
    const busy = this.asking !== null;

    const talk = v_flex()
      .flex_1()
      .min_w(0)
      .gap(10)
      .child(
        div()
          .id("pc-scroll")
          .w_full()
          .h(260)
          .rounded(8)
          .border_1()
          .border_color(t.border)
          .bg(alpha(t.background, 0.4))
          .overflow_y_scrollbar()
          .track_scroll(this.scroll)
          .child(
            v_flex()
              .p(12)
              .when(messages.length === 0 && !busy, (e) =>
                e.child(
                  v_flex()
                    .items_center()
                    .gap(12)
                    .py(18)
                    .child(
                      div()
                        .max_w(380)
                        .text_size(13)
                        .line_height("19px")
                        .text_color(t.muted)
                        .child("Say how you want the day to go: when you start, how long you have, what matters most, how you feel. The plan comes from your open tasks, and nothing changes until you apply it."),
                    )
                    .child(
                      h_flex()
                        .flex_wrap()
                        .justify_center()
                        .gap(8)
                        .children(
                          starters.map((s, i) =>
                            div()
                              .id(`pc-start-${i}`)
                              .px(12)
                              .h(28)
                              .flex()
                              .items_center()
                              .rounded(14)
                              .border_1()
                              .border_color(t.input)
                              .bg(t.raised)
                              .text_size(12)
                              .text_color(t.muted)
                              .cursor_pointer()
                              .hover((st) => st.text_color(t.foreground))
                              .on_click((e, cx) => this.send(s, cx))
                              .child(s),
                          ),
                        ),
                    ),
                ),
              )
              .children(messages.map((m, i) => this.bubble(ui, m.role, m.text, `pc-m-${i}`)))
              .when(busy, (e) => e.child(this.bubble(ui, "user", this.asking.message, "pc-pending", true))),
          ),
      );
    if (busy) talk.child(aiProgress(ui, this.asking.since, "Planning your day"));
    if (this.unavailable) talk.child(aiUnavailable(ui, this.app, this.unavailable, "pc-unavailable"));
    if (this.refused)
      talk.child(ui.callout({ tone: "neutral", icon: "circle-alert", text: `${this.refused[0].toUpperCase()}${this.refused.slice(1)}.`, action: ui.button("pc-refused-new", "New chat", (e, cx) => this.startOver(cx), { small: true }) }));
    if (this.failure) talk.child(aiFailed(ui, this.app, this.failure, "pc-failed"));
    talk
      .child(
        h_flex()
          .items_end()
          .gap(8)
          .child(this.input.el(ui, { h: 58 }))
          .child(ui.button("pc-send", this.failure ? "Send again" : "Send", (e, cx) => this.send(this.input.value, cx), { tone: "primary", icon: "send-horizontal", busy, disabled: !this.input.value.trim() })),
      )
      .child(ui.dim("Enter sends, Shift+Enter starts a new line."));

    const total = blocks.reduce((sum, b, i) => sum + (this.picked.has(i) ? b.planned_minutes : 0), 0);
    const room = out?.hours?.work_minutes ?? plan.capacity_minutes;
    const change = hoursText(out?.hours ?? null);
    const proposal = v_flex()
      .flex_1()
      .min_w(0)
      .rounded(8)
      .border_1()
      .border_color(t.border)
      .overflow_hidden()
      .child(
        h_flex()
          .items_center()
          .gap(8)
          .px(14)
          .py(9)
          .border_b(1)
          .border_color(t.border)
          .bg(t.raised)
          .child(ui.icon("calendar-check-2", 15))
          .child(div().flex_1().text_size(13).font_medium().child("Proposed plan"))
          .when(Boolean(this.run) && !open, (e) => e.child(ui.badge("Applied", { tone: "ok" })))
          .when(open, (e) => e.child(ui.text(`${minutes(total)} of ${minutes(room)}`, { size: 12, color: total > room ? ui.tone("warn") : t.muted }))),
      );
    if (!this.run) proposal.child(div().px(16).py(40).flex().justify_center().child(ui.muted("The blocks the assistant proposes show here, and on the timeline above.", { size: 13 })));
    else {
      proposal.child(
        div()
          .id("pc-blocks")
          .max_h(300)
          .overflow_y_scrollbar()
          .child(
            v_flex()
              .children(
                blocks.map((b, i) =>
                  h_flex()
                    .items_start()
                    .gap(10)
                    .px(14)
                    .py(8)
                    .border_b(1)
                    .border_color(t.border)
                    .child(
                      div()
                        .pt(1)
                        .child(
                          ui.checkbox(
                            `pc-pick-${i}`,
                            this.picked.has(i),
                            (on, cx) => {
                              if (on) this.picked.add(i);
                              else this.picked.delete(i);
                              cx.notify();
                            },
                            { disabled: !open },
                          ),
                        ),
                    )
                    .child(
                      v_flex()
                        .flex_1()
                        .min_w(0)
                        .gap(3)
                        .child(div().truncate().text_size(13).text_color(this.picked.has(i) || !open ? t.foreground : t.muted).child(b.title))
                        .child(
                          h_flex()
                            .flex_wrap()
                            .items_center()
                            .gap(10)
                            .child(ui.muted(`${formatTime(b.start_at)}–${formatTime(b.start_at + b.planned_minutes * 60_000)}`, { size: 12 }))
                            .child(projectTag(ui, gwen.projects.find((p) => p.id === b.project_id)))
                            .child(h_flex().items_center().gap(4).child(ui.icon("clock", 12)).child(ui.muted(minutes(b.planned_minutes), { size: 12 }))),
                        ),
                    ),
                ),
              )
              .when(blocks.length === 0, (e) => e.child(div().py(20).flex().justify_center().child(ui.muted("Nothing planned: a day off.", { size: 13 })))),
          ),
      );
      if (change) proposal.child(div().px(14).py(8).border_t(1).border_color(t.border).child(ui.muted(`Also sets this day's hours: ${change}`, { size: 12 })));
      const allPicked = this.picked.size === blocks.length;
      proposal.child(
        h_flex()
          .justify_end()
          .gap(8)
          .px(14)
          .py(9)
          .border_t(1)
          .border_color(t.border)
          .bg(t.raised)
          .child(
            open
              ? ui.button("pc-apply", blocks.length === 0 ? "Clear the plan" : allPicked ? "Apply plan" : `Apply ${this.picked.size} of ${blocks.length}`, (e, cx) => this.apply(cx), {
                  tone: "primary",
                  icon: "calendar-check-2",
                  busy: this.applying,
                  disabled: blocks.length > 0 && this.picked.size === 0,
                })
              : ui.muted("On your plan. Keep talking to change it again.", { size: 12 }),
          ),
      );
    }

    return ui
      .panel({
        title: "Plan with AI",
        icon: "sparkles",
        actions: h_flex()
          .items_center()
          .gap(6)
          .child(ui.muted(providerName(gwen.config?.llm), { size: 12 }))
          .when(Boolean(this.run || this.failure), (e) => e.child(ui.button("pc-new", "New chat", (ev, cx) => this.startOver(cx), { small: true, tone: "ghost", icon: "message-square-plus", disabled: busy })))
          .child(ui.iconButton("pc-close", "x", (e, cx) => this.owner.closeChat(cx), { tip: "Close the chat" })),
      })
      .child(h_flex().w_full().items_start().gap(16).child(talk).child(proposal));
  }
}

export default class DayPlan {
  constructor(app, cx) {
    this.app = app;
    this.day = null; // set on the first sync, to today
    this.plan = null;
    this.commitments = [];
    this.tracked = [];
    this.loaded = {};
    this.chatOpen = false;
    this.chat = new PlanChat(app, this);
    this.generating = false;
    this.hoursStart = null; // edited start, "HH:MM", else null
    this.hoursWork = null; // edited time for tasks, a Go duration, else null
    this.hoursBusy = false;
    this.completer = new Completer(app);
    this.steps = new Steps(app, this.completer);
  }

  get tick() {
    return this.chat.asking !== null;
  }

  sync(cx) {
    const v = gwen.versions;
    const today = gwen.today();
    this.day ??= today;
    const day = this.day;
    const load = (key, stamp, fn) => {
      if (this.loaded[key] === stamp) return;
      this.loaded[key] = stamp;
      cx.spawn(async (cx) => {
        try {
          await fn();
        } catch (e) {
          gwen.fail(e);
        }
        cx.notify();
      });
    };
    load("plan", `${day}/${v.plan}/${v.tasks}`, async () => {
      const p = await api.GetPlan(day);
      if (this.day === day) this.setPlan(p);
    });
    load("commitments", `${v.goals}`, async () => {
      this.commitments = (await api.ListCommitments()).commitments ?? [];
    });
    load("tracked", `${day}/${today}/${v.days}`, async () => {
      if (day > today) this.tracked = [];
      else {
        const d = await api.GetDay(day).catch(() => null);
        if (this.day === day) this.tracked = d?.segments ?? [];
      }
    });
    if (this.chatOpen) this.chat.open(day, cx);
  }

  setPlan(p) {
    const changed = !this.plan || this.plan.day !== p.day || JSON.stringify(this.plan.hours ?? null) !== JSON.stringify(p.hours ?? null) || this.plan.window?.start_minute !== p.window?.start_minute;
    this.plan = p;
    if (changed) this.hoursStart = this.hoursWork = null;
  }

  setDay(day, cx) {
    if (!day) return;
    this.day = day;
    this.plan = null;
    this.hoursStart = this.hoursWork = null;
    this.chat.open(day, cx);
    this.sync(cx);
    cx.notify();
  }

  openChat(cx) {
    this.chatOpen = true;
    this.chat.open(this.day, cx);
    cx.notify();
  }

  closeChat(cx) {
    this.chatOpen = false;
    cx.notify();
  }

  /** Plan with AI from Today or the chat opens the chat for today. */
  show(param, cx) {
    if (param === "chat") {
      this.setDay(gwen.today(), cx);
      this.openChat(cx);
    }
  }

  generate(cx) {
    this.generating = true;
    this.app.spawn(cx, async () => {
      const p = await gwen.act(() => api.GeneratePlan({ day: this.day }));
      if (p) this.setPlan(p);
      this.generating = false;
    });
  }

  /** Skipping and pinning go through one block and apply to all the task's blocks on the day. */
  patch(it, fields, cx) {
    this.app.spawn(cx, () => gwen.act(() => api.PatchPlanItem(it.id, { ...fields, rev: it.rev })));
  }

  /**
   * Puts a task before the row at index (groups.length: last); the planner times the day in the new order. Rows with
   * a fixed time, done, or skipped keep their place in time, so the task goes before the next row Gwen still times.
   */
  move(groups, taskId, index, cx) {
    const before = groups.slice(index).find((g) => g.task.id !== taskId && !g.pinned && !g.done && !g.skipped);
    this.app.spawn(cx, async () => {
      const p = await gwen.act(() => api.MovePlanTask({ day: this.day, task_id: taskId, before_task_id: before?.task.id }));
      if (p) this.setPlan(p);
    });
  }

  /** A time set by hand: the task's whole time on the day, as one block that stays there. */
  setTime(g, hhmm, cx) {
    if (!hhmm) {
      if (g.pinned) this.patch(leadItem(g), { pinned: false }, cx);
      return;
    }
    const rollover = gwen.config?.tracking.day_rollover ?? "04:00";
    const planned = Math.min(720, Math.max(5, g.minutes || g.task.estimate_minutes || 30));
    this.app.spawn(cx, () => gwen.act(() => api.ScheduleTask({ task_id: g.task.id, day: this.day, start_at: instantOn(this.day, hhmm, rollover), planned_minutes: planned })));
  }

  // The day's own hours ----------------------------------------------------------

  saveHours(next, cx) {
    this.hoursBusy = true;
    this.app.spawn(cx, async () => {
      const p = await gwen.act(() => api.SetDayHours({ day: this.plan.day, ...next }));
      this.hoursBusy = false;
      if (p) {
        this.hoursStart = this.hoursWork = null;
        this.setPlan(p);
      }
    });
  }

  dayHours(ui) {
    const plan = this.plan;
    const hours = plan.hours ?? null;
    const savedStart = clockOf(plan.window.start_minute);
    const savedWork = goDuration((hours?.work_minutes ?? plan.capacity_minutes) * 60_000);
    const changed = (this.hoursStart !== null && this.hoursStart !== savedStart) || (this.hoursWork !== null && parseDuration(this.hoursWork) !== parseDuration(savedWork));
    const usualStart = gwen.config?.planner.day_start ?? "09:00";
    const target = parseDuration(gwen.config?.tracking.daily_target ?? "8h");
    const apply = (e, cx) =>
      this.saveHours(
        {
          start_minute: this.hoursStart !== null ? minutesOf(this.hoursStart) : (hours?.start_minute ?? null),
          work_minutes: this.hoursWork !== null ? Math.round(parseDuration(this.hoursWork) / 60_000) : (hours?.work_minutes ?? null),
        },
        cx,
      );
    const el = h_flex()
      .flex_wrap()
      .items_end()
      .gap(20)
      .child(
        ui.field(
          "Start at",
          ui.timeInput("dh-start", this.hoursStart ?? savedStart, (v, cx) => {
            this.hoursStart = v;
            cx.notify();
          }),
        ),
      )
      .child(
        ui.field(
          "Work for",
          ui.durationInput("dh-work", this.hoursWork ?? savedWork, (v, cx) => {
            this.hoursWork = v;
            cx.notify();
          }),
        ),
      );
    if (changed)
      el.child(
        h_flex()
          .gap(8)
          .child(ui.button("dh-apply", "Replan with these hours", apply, { tone: "primary", busy: this.hoursBusy }))
          .child(
            ui.button(
              "dh-cancel",
              "Cancel",
              (e, cx) => {
                this.hoursStart = this.hoursWork = null;
                cx.notify();
              },
              { tone: "ghost", disabled: this.hoursBusy },
            ),
          ),
      );
    else if (hours) el.child(ui.button("dh-usual", "Usual hours", (e, cx) => this.saveHours({ start_minute: null, work_minutes: null }, cx), { tone: "ghost", icon: "rotate-ccw", busy: this.hoursBusy }));
    el.child(
      div()
        .flex_1()
        .min_w(220)
        .child(
          ui.dim(
            hours
              ? `Set for this day. Usually from ${formatClock(minutesOf(usualStart))}, with the time worked out from your ${formatDuration(target)} target.`
              : `Usual hours: from ${formatClock(minutesOf(usualStart))}, with the time for tasks worked out from your ${formatDuration(target)} target and commitments. Change either for this day alone.`,
          ),
        ),
    );
    return el;
  }

  // The screen ------------------------------------------------------------------

  render(ui) {
    const t = ui.t;
    const tr = tracking();
    const today = gwen.today();
    const day = this.day ?? today;
    const plan = this.plan;
    const cfg = gwen.config;
    const past = day < today; // nothing past can change
    const rollover = cfg?.tracking.day_rollover ?? "04:00";
    const onDay = this.commitments.filter((c) => occursOn(c, day));
    const items = plan?.items ?? [];
    const groups = groupPlan(items);
    const colors = new Map([[null, unassignedColor]]);
    const names = new Map([[null, "Unassigned"]]);
    for (const p of gwen.projects) {
      colors.set(p.id, p.color);
      names.set(p.id, p.name);
    }

    const capacity = plan?.capacity_minutes ?? 0;
    const planned = plan?.planned_minutes ?? 0;
    const over = planned > capacity;
    const itemBlock = (it) => ({ id: it.id, title: it.task.title, start: it.start_at, minutes: it.planned_minutes, color: colors.get(it.task.project_id ?? null) ?? unassignedColor, done: it.status === "done" });
    // While the chat proposes a plan, the plan lane previews it in place of the planned items.
    const preview = this.chatOpen ? this.chat.preview : null;
    const blocks = preview
      ? [
          ...items.filter((it) => it.start_at != null && it.status === "done").map(itemBlock),
          ...preview.map((b, i) => ({ id: `preview-${i}`, title: b.title, start: b.start_at, minutes: b.planned_minutes, color: colors.get(b.project_id) ?? unassignedColor, done: false })),
        ]
      : items.filter((it) => it.start_at != null && it.status !== "skipped").map(itemBlock);
    const usualStart = cfg?.planner.day_start ?? "09:00";
    const shownStart = plan ? clockOf(Math.min(minutesOf(usualStart), plan.window.start_minute)) : usualStart;
    const shownFrom = instantOn(day, shownStart, rollover);
    const shownTo = instantOn(day, cfg?.planner.day_end ?? "23:00", rollover);
    const events = eventBlocks(plan, shownFrom, shownTo);
    const fixed = onDay
      .filter((c) => c.start_minute != null)
      .map((c) => ({ id: c.id, title: c.title, start: instantOn(day, clockOf(c.start_minute), rollover), minutes: c.duration_minutes }))
      .concat(events);
    const rel = relativeDay(day, today);

    const header = ui.pageHeader(
      "Plan",
      ["today", "tomorrow", "yesterday"].includes(rel) ? `${formatLongDate(day)} (${rel})` : formatLongDate(day),
      h_flex()
        .items_center()
        .gap(8)
        .child(ui.iconButton("pl-prev", "chevron-left", (e, cx) => this.setDay(addDays(day, -1), cx), { tip: "Previous day", size: 30 }))
        .child(ui.datePicker("pl-day", day, (d, cx) => this.setDay(d, cx), { width: 160 }))
        .child(ui.iconButton("pl-next", "chevron-right", (e, cx) => this.setDay(addDays(day, 1), cx), { tip: "Next day", size: 30 }))
        .when(day !== today, (e) => e.child(ui.button("pl-today", "Today", (ev, cx) => this.setDay(today, cx))))
        .when(!past && !this.chatOpen, (e) => e.child(ui.button("pl-ai", "Plan with AI", (ev, cx) => this.openChat(cx), { icon: "sparkles", iconColor: t.accent })))
        .when(!past && day > today && items.length === 0, (e) => e.child(ui.button("pl-generate", "Plan this day", (ev, cx) => this.generate(cx), { tone: "primary", icon: "refresh-cw", busy: this.generating }))),
    );

    const figure = (value, label, muted) => v_flex().child(div().text_size(20).font_semibold().text_color(muted ? t.muted : t.foreground).child(value)).child(ui.dim(label));
    const card = ui
      .panel({ pad: 20, gap: 16 })
      .child(
        h_flex()
          .flex_wrap()
          .items_end()
          .gap(36)
          .child(figure(minutes(planned), "planned"))
          .child(figure(minutes(capacity), "room in the day"))
          .child(figure(minutes(Math.max(0, capacity - planned)), "free", capacity - planned <= 0))
          .when(over, (e) => e.child(div().flex_1().child(ui.text(`${minutes(planned - capacity)} more than the day has room for. Skip or shorten something.`, { size: 13, color: ui.tone("warn") })))),
      )
      .child(ui.meter(capacity > 0 ? planned / capacity : planned > 0 ? 1 : 0, { color: over ? ui.tone("warn") : t.accent, height: 8 }));
    if (!past && plan) card.child(this.dayHours(ui));
    if (onDay.length + events.length > 0) {
      const chip = (key, icon, title, when, len, tip) =>
        h_flex()
          .id(key)
          .items_center()
          .gap(8)
          .px(9)
          .h(24)
          .rounded(6)
          .border_1()
          .border_color(t.border)
          .bg(alpha(t.muted, 0.12))
          .text_size(12)
          .when(Boolean(tip), (e) => e.tooltip(tip))
          .when(Boolean(icon), (e) => e.child(ui.icon(icon, 12)))
          .child(div().font_medium().child(title))
          .child(ui.muted(when, { size: 12 }))
          .child(ui.dim(len, { size: 12 }));
      card.child(
        h_flex()
          .flex_wrap()
          .gap(8)
          .children(onDay.map((c) => chip(`pl-c-${c.id}`, null, c.title, c.start_minute == null ? "any time" : `${formatClock(c.start_minute)}–${formatClock(c.start_minute + c.duration_minutes)}`, minutes(c.duration_minutes))))
          .children(events.map((e) => chip(`pl-e-${e.id}`, "calendar-days", e.title, `${formatTime(e.start)}–${formatTime(e.start + e.minutes * 60_000)}`, minutes(e.minutes), "From Google Calendar"))),
      );
    }
    card.child(div().pt(4).border_t(1).border_color(t.border).child(timeline(ui, { id: "pl-tl", segments: this.tracked, colors, names, now: gwen.now(), live: day === today, from: shownFrom, to: shownTo, planned: blocks, fixed })));
    if (preview) card.child(h_flex().items_center().gap(6).child(ui.icon("sparkles", 12, t.accent)).child(ui.dim("The plan lane shows the AI's proposal. Apply it below to keep it.")));

    const parts = [header, card];
    if (this.chatOpen && !past && plan) parts.push(this.chat.render(ui, plan));

    // Tasks ------------------------------------------------------------------------
    const tasks = ui.panel({ title: "Tasks", icon: "calendar-clock", pad: 12 });
    if (plan && items.length === 0)
      tasks.child(
        ui.empty({
          icon: "calendar-clock",
          title: past ? "Nothing was planned" : day > today ? "Not planned yet" : "Nothing to plan yet",
          body: past
            ? undefined
            : day > today
              ? "Gwen fills the day from your goals and open tasks, around your commitments, and keeps it up to date from then on."
              : "Tasks with a time estimate and your goals' sessions show up here by themselves, timed around your commitments.",
          action: !past && day > today ? ui.button("pl-generate-empty", "Plan this day", (e, cx) => this.generate(cx), { tone: "primary", icon: "refresh-cw", busy: this.generating }) : null,
        }),
      );
    else if (!plan) tasks.child(ui.splash("Loading…", { busy: true }));
    else {
      const list = v_flex();
      groups.forEach((g, i) => {
        const id = g.task.id;
        const key = `pl-g-${id}`;
        const isDone = g.done;
        const live = tr.isTracking(id);
        const movable = !past && !isDone && !g.skipped;
        const lead = leadItem(g);
        const meta = h_flex().flex_wrap().items_center().gap(10).child(projectTag(ui, gwen.projects.find((p) => p.id === g.task.project_id)));
        if (g.items.length > 1 && g.start != null) meta.child(ui.muted(blockTimes(g), { size: 12 }));
        if (g.pinned) meta.child(h_flex().id(`${key}-pin`).items_center().gap(4).tooltip("A time you set: the rest of the day flows around it").child(ui.icon("pin", 11, t.accent)).child(ui.text("Fixed time", { size: 12, color: t.accent })));
        if (g.steps.length > 0) meta.child(ui.muted(stepsDone(g.steps), { size: 12 }));
        if (g.rollover > 0) meta.child(ui.badge(`Carried over ${g.rollover}×`, { tone: "warn" }));
        if (g.skipped) meta.child(ui.badge("Skipped"));
        if (!past && !isDone && !g.skipped && g.start == null) meta.child(ui.text("No room left today", { size: 12, color: ui.tone("warn") }));
        const row = h_flex().items_center().gap(10).px(8).py(8);
        if (!past)
          row.child(
            v_flex()
              .flex_none()
              .child(ui.iconButton(`${key}-up`, "chevron-up", (e, cx) => this.move(groups, id, i - 1, cx), { tip: "Earlier: Gwen times the day in this order", size: 18, disabled: !movable || i === 0 }))
              .child(ui.iconButton(`${key}-down`, "chevron-down", (e, cx) => this.move(groups, id, i + 2, cx), { tip: "Later", size: 18, disabled: !movable || i === groups.length - 1 })),
          );
        row.child(ui.checkbox(`${key}-check`, isDone, (on, cx) => (on ? this.completer.complete(g.task, g.steps.length > 0, cx) : this.completer.reopen(g.task)), { round: true, size: 18, disabled: past || g.skipped }));
        row.child(
          v_flex()
            .flex_1()
            .min_w(0)
            .gap(4)
            .child(div().truncate().text_size(13.5).text_color(isDone || g.skipped ? t.dim : t.foreground).when(isDone, (e) => e.line_through()).child(g.task.title))
            .child(meta),
        );
        if (past || isDone) row.child(div().w(140).flex_none().flex().justify_end().child(ui.muted(`${g.start != null ? `${formatTime(g.start)}  ` : ""}${minutes(g.minutes)}`, { size: 12 })));
        else {
          row
            .child(ui.timeInput(`${key}-time`, g.start != null ? clockValue(g.start) : "", (v, cx) => this.setTime(g, v, cx), { empty: true, width: 92, placeholder: "Start" }))
            .child(
              ui.numberInput(
                `${key}-min`,
                g.minutes,
                (m, cx) => {
                  if (m > 0 && m !== g.minutes) this.patch(lead, { planned_minutes: m }, cx);
                },
                { min: 5, width: 62, unit: "min" },
              ),
            )
            .child(ui.iconButton(`${key}-pinbtn`, "pin", (e, cx) => this.patch(lead, { pinned: !g.pinned }, cx), { tip: g.pinned ? "Let Gwen time it" : "Fix it at this time", active: g.pinned, disabled: g.skipped }))
            .child(ui.iconButton(`${key}-skip`, g.skipped ? "undo-2" : "skip-forward", (e, cx) => this.patch(lead, { status: g.skipped ? "planned" : "skipped" }, cx), { tip: g.skipped ? "Put back on the plan" : "Skip for this day" }));
          if (day === today) {
            if (live) row.child(div().id(`${key}-live`).tooltip("Tracking now").px(4).child(ui.icon("radio", 15, ui.tone("ok"))));
            else row.child(ui.iconButton(`${key}-play`, "play", () => tr.startTask(g.task), { tip: `Start ${g.task.title}`, disabled: g.skipped }));
          }
        }
        const item = v_flex().rounded(8).when(live, (e) => e.bg(t.raised)).child(row);
        if (g.steps.length > 0) item.child(div().pb(6).child(this.steps.list(ui, g.task, g.steps, { adding: !past })));
        list.child(item.when(i < groups.length - 1, (e) => e.border_b(1).border_color(t.border)));
      });
      tasks.child(list);
      if (groups.length > 1 && !past)
        tasks.child(div().px(8).child(ui.dim("Put tasks in the order you want to do them with the arrows. Gwen times the day in that order and keeps it current as you go; a time you set by hand stays fixed.")));
    }
    parts.push(tasks);
    return ui.scroll("pl-scroll", parts);
  }
}
