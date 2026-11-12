// Today: the timer and its controls, the day against the clock, the agenda,
// quick to-dos, goals, and where today's time went.

import { div } from "gpui-kit";
import { v_flex, h_flex } from "gpui-base";
import { api, gwen, tracking } from "../lib/gwen.js";
import { addDays, clockFace, formatClock, formatDate, formatDuration, formatLongDate, formatTime, instantOn, parseDuration } from "../lib/format.js";
import { blockTimes, eventBlocks, groupPlan } from "../lib/plan.js";
import { clockOf, occursOn } from "../lib/rrule.js";
import { TextField, stateColor, stateLabel, unassignedColor } from "../lib/kit.js";
import { Completer, Steps, TaskForm, goalAmount, goalMeter, paceChip, priorityFlag, projectTag, stepsDone, timeline } from "../lib/parts.js";

/** Goals behind pace come first. */
const rank = (pace) => (pace === "behind" ? 0 : pace === "on_track" ? 1 : 2);

const greeting = () => {
  const h = new Date().getHours();
  return h < 12 ? "Good morning" : h < 18 ? "Good afternoon" : "Good evening";
};

export default class Today {
  constructor(app) {
    this.app = app;
    this.tick = true; // the clock in the header and the live timeline move
    this.day = null;
    this.plan = null;
    this.goals = [];
    this.commitments = [];
    this.streak = 0;
    this.loaded = {}; // the versions each piece was loaded at
    this.pick = null; // what Clock in starts with, once another project or task is picked
    this.completer = new Completer(app);
    this.steps = new Steps(app, this.completer);
    this.addingTodo = false;
    this.todo = new TextField({ placeholder: "Add a quick to-do, then Enter", onSubmit: (v, cx) => this.addTodo(cx) });
    this.briefing = null;
    this.briefingAsked = false;
  }

  /** Loads what changed since it was last loaded. */
  sync(cx) {
    const v = gwen.versions;
    const today = gwen.today();
    const state = gwen.status?.state;
    const load = (key, stamp, fn) => {
      if (this.loaded[key] === stamp) return;
      this.loaded[key] = stamp;
      cx.spawn(async (cx) => {
        await fn();
        cx.notify();
      });
    };
    load("day", `${today}/${v.days}/${state}`, async () => {
      this.day = await api.GetDay(today).catch(() => null); // no work day yet
    });
    load("plan", `${today}/${v.plan}/${v.tasks}`, async () => {
      this.plan = await api.GetPlan(today).catch(() => null);
    });
    load("goals", `${v.goals}/${v.tasks}`, async () => {
      this.goals = (await api.ListGoals("active").catch(() => null))?.goals ?? [];
    });
    load("commitments", `${v.goals}`, async () => {
      this.commitments = (await api.ListCommitments().catch(() => null))?.commitments ?? [];
    });
    // The streak is a running count ending today, so it needs a week behind it.
    load("streak", `${today}/${v.days}`, async () => {
      this.streak = (await api.StatsSummary(addDays(today, -6), today).catch(() => null))?.current_streak ?? 0;
    });
    // The briefing opens by itself on the first look of each day.
    if (!this.briefingAsked) {
      this.briefingAsked = true;
      let dismissed = null;
      try {
        dismissed = localStorage.getItem("briefing.dismissed");
      } catch {}
      if (dismissed !== today) this.openBriefing(cx);
    }
  }

  addTodo(cx) {
    const title = this.todo.value.trim();
    if (!title || this.addingTodo) return;
    this.addingTodo = true; // one to-do per Enter, however fast it is pressed
    this.app.spawn(cx, async () => {
      if (await gwen.act(() => api.CreateTask({ title, start_day: gwen.today() }))) this.todo.set("");
      this.addingTodo = false;
    });
  }

  // The briefing ------------------------------------------------------------

  openBriefing(cx) {
    const ui = this.app.ui;
    this.briefing = null;
    const today = gwen.today();
    cx.spawn(async (cx) => {
      this.briefing = await api.Briefing().catch((e) => {
        gwen.fail(e);
        return null;
      });
      cx.notify();
    });
    const close = () => {
      try {
        localStorage.setItem("briefing.dismissed", today);
      } catch {}
    };
    const id = ui.openDialog(
      {
        title: greeting(),
        width: 560,
        onClose: close,
        render: (ui) => this.renderBriefing(ui),
        actions: (ui) => [
          ui.button(
            "brief-plan",
            "Open plan",
            (e, cx) => {
              ui.closeDialog(cx, id);
              this.app.go("plan");
            },
            { icon: "calendar-clock" },
          ),
          ui.button("brief-close", "Close", (e, cx) => ui.closeDialog(cx, id), { tone: "primary" }),
        ],
      },
      cx,
    );
  }

  renderBriefing(ui) {
    const t = ui.t;
    const b = this.briefing;
    if (!b) return ui.muted("Loading your day…");
    const behind = b.goals.filter((g) => g.progress.pace === "behind");
    const planned = b.today.filter((it) => it.status !== "skipped").reduce((sum, it) => sum + it.planned_minutes, 0);
    const section = (title, icon, rows) =>
      v_flex()
        .gap(2)
        .child(h_flex().items_center().gap(7).pb(2).child(ui.icon(icon, 14)).child(div().text_size(13).font_semibold().text_color(t.muted).child(title)))
        .children(rows);
    const row = (left, right, o = {}) =>
      h_flex()
        .items_center()
        .gap(12)
        .py(6)
        .border_b(1)
        .border_color(t.border)
        .child(div().flex_1().min_w(0).truncate().text_size(13).text_color(o.muted ? t.dim : t.foreground).when(Boolean(o.muted), (e) => e.line_through()).child(left))
        .child(div().flex_none().text_size(12).text_color(o.warn ? ui.tone("danger") : t.muted).child(right));
    const el = v_flex().gap(16).child(ui.muted(`${formatLongDate(b.day)}${planned ? `. ${formatDuration(planned * 60_000)} planned.` : "."}`));
    if (b.pending.length + b.today.length + b.reminders.length + behind.length === 0) return el.child(ui.muted("Nothing is planned, carried over, or due. A clear day."));
    if (b.pending.length) el.child(section("Carried over", "corner-down-right", b.pending.map((it) => row(it.task.title, `from ${formatDate(it.day)}`))));
    if (b.today.length)
      el.child(
        section(
          "Today's plan",
          "calendar-clock",
          b.today.map((it) => row(it.task.title, `${it.start_at != null ? formatTime(it.start_at) + "  " : ""}${formatDuration(it.planned_minutes * 60_000)}`, { muted: it.status !== "planned" })),
        ),
      );
    if (b.reminders.length) el.child(section("Reminders", "bell-ring", b.reminders.map((r) => row(r.title, r.message, { warn: r.days_left < 0 }))));
    if (behind.length)
      el.child(
        section(
          "Goals behind pace",
          "target",
          behind.map((g) =>
            v_flex()
              .gap(7)
              .py(8)
              .child(h_flex().items_center().gap(10).child(div().flex_1().min_w(0).truncate().text_size(13).child(g.title)).child(ui.muted(goalAmount(g), { size: 12 })).child(paceChip(ui, g.progress.pace)))
              .child(goalMeter(ui, g, b.day)),
          ),
        ),
      );
    return el;
  }

  // The screen ----------------------------------------------------------------

  render(ui) {
    const t = ui.t;
    const st = gwen.status;
    if (!st) return ui.splash("Loading…", { busy: true });
    const tr = tracking();
    const today = gwen.today();
    const now = gwen.now();
    const state = st.state;
    const color = stateColor[state];
    const cfg = gwen.config;
    const day = this.day;
    const plan = this.plan;

    const colors = new Map([[null, unassignedColor]]);
    const names = new Map([[null, "Unassigned"]]);
    for (const p of gwen.projects) {
      colors.set(p.id, p.color);
      names.set(p.id, p.name);
    }
    for (const p of day?.summary?.by_project ?? []) {
      colors.set(p.project_id ?? null, p.color);
      names.set(p.project_id ?? null, p.name);
    }

    const elapsed = st.open_segment ? now - st.open_segment.started_at : 0;
    const summary = st.today ?? day?.summary;
    const worked = (summary?.worked_ms ?? 0) + (tr.working && st.today ? now - st.server_now_at : 0);
    const breakMs = (summary?.break_ms ?? 0) + (tr.onBreak && st.today ? now - st.server_now_at : 0);
    const target = (summary?.target_seconds ?? 0) * 1000 || parseDuration(cfg?.tracking.daily_target ?? "8h");
    const met = target > 0 && worked >= target;

    const pick = this.pick ?? gwen.lastAttribution;
    const projectId = state === "off" ? pick.project_id : (st.project_id ?? null);
    const taskId = state === "off" ? pick.task_id : (st.task_id ?? null);
    const projectTasks = gwen.tasks.filter((x) => (!projectId || x.project_id === projectId || !x.project_id) && !(x.occurrence_day && x.occurrence_day > today));
    const chooseProject = (id) => {
      if (state === "off") this.pick = { project_id: id, task_id: null };
      else tr.switchTo(id, null);
    };
    const chooseTask = (id) => {
      const task = gwen.tasks.find((x) => x.id === id);
      const project = task?.project_id ?? projectId;
      if (state === "off") this.pick = { project_id: project, task_id: id };
      else tr.switchTo(project, id);
    };

    const rollover = cfg?.tracking.day_rollover ?? "04:00";
    const from = instantOn(today, cfg?.planner.day_start ?? "09:00", rollover);
    const to = instantOn(today, cfg?.planner.day_end ?? "23:00", rollover);
    const items = plan?.items ?? [];
    const groups = groupPlan(items);
    const planned = items
      .filter((it) => it.start_at != null && it.status !== "skipped")
      .map((it) => ({ id: it.id, title: it.task.title, start: it.start_at, minutes: it.planned_minutes, color: colors.get(it.task.project_id ?? null) ?? unassignedColor, done: it.status === "done" }));
    const fixed = this.commitments
      .filter((c) => c.start_minute != null && occursOn(c, today))
      .map((c) => ({ id: c.id, title: c.title, start: instantOn(today, clockOf(c.start_minute), rollover), minutes: c.duration_minutes }))
      .concat(eventBlocks(plan, from, to));
    // What is coming up next, across calendar events and Gwen's own plan: whichever starts soonest.
    const next = [...planned.filter((p) => !p.done && p.start > now).map((p) => ({ title: p.title, start: p.start })), ...fixed.filter((f) => f.start > now)].sort((a, b) => a.start - b.start)[0];
    const agenda = [...groups.map((group) => ({ kind: "task", group, at: group.start ?? Infinity })), ...fixed.map((block) => ({ kind: "event", block, at: block.start }))].sort((a, b) => a.at - b.at);

    const byProject = day?.summary?.by_project ?? [];
    const byTotal = byProject.reduce((sum, p) => sum + p.worked_ms, 0);
    const shownGoals = [...this.goals].sort((a, b) => rank(a.progress.pace) - rank(b.progress.pace)).slice(0, 4);
    const plannedLeft = items.filter((it) => it.status === "planned").reduce((sum, it) => sum + it.planned_minutes, 0);
    // Quick to-dos: open tasks with no time to plan, for today or earlier, timed ones first.
    const quantityGoals = new Set(this.goals.filter((g) => g.kind === "quantity").map((g) => g.id));
    const todos = gwen.tasks
      .filter((x) => x.estimate_minutes == null && !x.parent_id && !(x.goal_id && quantityGoals.has(x.goal_id)))
      .filter((x) => (!x.start_day || x.start_day <= today) && (!x.occurrence_day || x.occurrence_day <= today))
      .sort((a, b) => (a.start_minute ?? 1440) - (b.start_minute ?? 1440));
    const tasksLeft = agenda.filter((r) => r.kind === "task" && !r.group.done && !r.group.skipped).length + todos.length;
    const topGoal = shownGoals[0];

    // Header ----------------------------------------------------------------
    const header = ui.pageHeader(
      formatLongDate(today),
      null,
      h_flex()
        .items_center()
        .gap(10)
        .child(ui.muted(formatTime(now), { size: 13 }))
        .when(Boolean(next), (e) => e.child(ui.badge(`${next.title.slice(0, 28)}${next.title.length > 28 ? "…" : ""}  ${formatTime(next.start)}`, { tone: "accent", icon: "calendar-clock", tip: `Next: ${next.title}` })))
        .child(ui.button("td-brief", "Briefing", (e, cx) => this.openBriefing(cx), { small: true, icon: "scroll-text" })),
    );

    // The status card -------------------------------------------------------
    const pctDone = Math.round(target > 0 ? (worked / target) * 100 : 0);
    const face = clockFace(worked);
    const timer = v_flex()
      .flex_none()
      .gap(4)
      .child(
        h_flex()
          .items_center()
          .gap(7)
          .child(ui.dot(color, 8))
          .child(div().text_size(13).font_medium().text_color(color).child(stateLabel[state]))
          .when(state !== "off", (e) => e.child(ui.muted(tr.onBreak ? `for ${formatDuration(elapsed)}` : `since ${formatTime(st.state_since_at)}`, { size: 13 }))),
      )
      .child(
        state === "off"
          ? div().text_size(28).font_semibold().text_color(t.muted).child("Off the clock")
          : h_flex()
              .items_baseline()
              .child(div().text_size(38).font_semibold().text_color(tr.onBreak ? t.muted : t.foreground).child(face.slice(0, -3)))
              .child(div().text_size(38).font_semibold().text_color(t.dim).child(face.slice(-3))),
      )
      .child(
        h_flex()
          .items_center()
          .gap(8)
          .child(ui.muted(`Break ${formatDuration(breakMs)}`, { size: 12 }))
          .child(ui.dim("·"))
          .child(met ? h_flex().items_center().gap(4).child(ui.icon("circle-check", 12, ui.tone("ok"))).child(ui.text("Target met", { size: 12, color: ui.tone("ok") })) : ui.muted(`${formatDuration(target - worked)} to go`, { size: 12 }))
          .when(st.snoozed_until_at != null && st.snoozed_until_at > now, (e) =>
            e.child(h_flex().id("td-snoozed").items_center().gap(4).tooltip("Nudges are snoozed").child(ui.icon("bell-off", 12)).child(ui.muted(formatDuration(st.snoozed_until_at - now), { size: 12 }))),
          ),
      )
      .child(
        v_flex()
          .pt(4)
          .w(250)
          .gap(4)
          .child(ui.meter(target > 0 ? worked / target : 0, { color: met ? stateColor.working : color, height: 6 }))
          .child(ui.dim(`${pctDone}% of ${formatDuration(target)}`)),
      );

    const controls = v_flex()
      .flex_1()
      .min_w(260)
      .gap(10)
      .child(
        h_flex()
          .gap(8)
          .child(
            ui.select("td-project", [{ value: "", label: "Unassigned", dot: unassignedColor }, ...gwen.projects.map((p) => ({ value: p.id, label: p.name, dot: p.color }))], projectId ?? "", (v, cx) => {
              chooseProject(v || null);
              cx.notify();
            }, { width: 170 }),
          )
          .child(
            div().flex_1().min_w(0).child(
              ui.select("td-task", [{ value: "", label: "No task" }, ...projectTasks.map((x) => ({ value: x.id, label: x.title }))], taskId ?? "", (v, cx) => {
                chooseTask(v || null);
                cx.notify();
              }, { menuWidth: 340 }),
            ),
          ),
      )
      .child(
        h_flex()
          .flex_wrap()
          .items_center()
          .gap(8)
          .when(state === "off", (e) => e.child(ui.button("td-in", "Clock in", () => tr.clockIn(pick.project_id, pick.task_id), { tone: "primary", icon: "play" })))
          .when(tr.onBreak, (e) => e.child(ui.button("td-back", "End break", () => tr.breakEnd(), { tone: "primary", icon: "play" })))
          .when(state === "working" || state === "break_auto", (e) => e.child(ui.button("td-break", state === "break_auto" ? "Stay on break" : "Start break", () => tr.breakStart(), { icon: "coffee", iconColor: stateColor.break_manual })))
          .when(state !== "off", (e) => e.child(ui.button("td-out", "Clock out", () => tr.clockOut(), { icon: "square", iconColor: ui.tone("danger") })))
          .when(state !== "off", (e) => e.child(ui.iconButton("td-snooze", "bell-off", () => tr.snooze(), { tip: "Snooze nudges", size: 30 }))),
      );

    const tile = (icon, label, value, iconColor) =>
      v_flex()
        .w(118)
        .gap(4)
        .px(10)
        .py(8)
        .rounded(8)
        .bg(t.raised)
        .child(h_flex().items_center().gap(6).child(ui.icon(icon, 12, iconColor ?? t.muted)).child(div().text_size(10.5).text_color(t.muted).child(label)))
        .child(div().truncate().text_size(14).font_semibold().text_color(t.foreground).child(value));
    const tiles = v_flex()
      .flex_none()
      .gap(8)
      .child(h_flex().gap(8).child(tile("timer", "Focus", formatDuration(worked), t.accent)).child(tile("list-todo", "Tasks", `${tasksLeft} left`)))
      .child(h_flex().gap(8).child(tile("target", "Goals", topGoal ? goalAmount(topGoal) : "No goals")).child(tile("flame", "Streak", `${this.streak} ${this.streak === 1 ? "day" : "days"}`, this.streak > 0 ? ui.tone("warn") : undefined)));

    const card = ui.panel({ pad: 20, gap: 14 }).child(h_flex().flex_wrap().items_start().gap(28).child(timer).child(controls).child(tiles));
    if (state === "idle_pending" && st.idle_since_at)
      card.child(
        ui.callout({
          tone: "warn",
          icon: "hourglass",
          text: `No keyboard or mouse for ${formatDuration(now - st.idle_since_at)}. It still counts as work until ${cfg?.tracking.hard_idle ? formatDuration(parseDuration(cfg.tracking.hard_idle)) : "the limit"}, then becomes a break.`,
          action: ui.button("td-idle-break", "Count it as a break", () => tr.breakStart(), { small: true, icon: "coffee" }),
        }),
      );

    // Schedule ---------------------------------------------------------------
    const schedule = ui
      .panel({ title: "Schedule", icon: "calendar-clock", actions: state !== "off" && !tr.onBreak ? ui.badge("Live", { tone: "ok", icon: "radio" }) : null })
      .child(timeline(ui, { id: "td-tl", segments: day?.segments ?? [], colors, names, now, live: true, from, to, planned, fixed, density: true }));

    // Agenda -----------------------------------------------------------------
    const agendaPanel = ui.panel({
      title: "Agenda",
      icon: "calendar-clock",
      actions: h_flex()
        .items_center()
        .gap(6)
        .when(items.length > 0, (e) => e.child(ui.muted(`${formatDuration(plannedLeft * 60_000)} left`, { size: 12 })))
        .child(ui.button("td-ai", "Plan with AI", () => this.app.go("plan", "plan", "chat"), { small: true, tone: "ghost", icon: "sparkles", iconColor: t.accent }))
        .when(items.length > 0, (e) => e.child(ui.button("td-open-plan", "Open", () => this.app.go("plan", "plan"), { small: true, tone: "ghost" }))),
    });
    if (plan && agenda.length === 0)
      agendaPanel.child(ui.empty({ icon: "calendar-clock", title: "Nothing on today yet", body: "Tasks with a time estimate and your goals' sessions show up here by themselves, timed around your calendar." }));
    else
      agendaPanel.child(
        v_flex().children(
          agenda.map((row) => {
            if (row.kind === "event") {
              const e = row.block;
              return h_flex()
                .items_center()
                .gap(12)
                .px(10)
                .py(9)
                .child(div().w(18).flex_none())
                .child(
                  v_flex()
                    .flex_1()
                    .min_w(0)
                    .gap(5)
                    .child(div().truncate().text_size(14).text_color(t.muted).child(e.title))
                    .child(h_flex().items_center().gap(10).child(ui.badge("Calendar", { icon: "calendar-clock" })).child(ui.muted(formatTime(e.start), { size: 12 }))),
                );
            }
            const g = row.group;
            const live = tr.isTracking(g.task.id);
            const project = gwen.projects.find((p) => p.id === g.task.project_id);
            const meta = h_flex().flex_wrap().items_center().gap(10).child(projectTag(ui, project));
            const flag = priorityFlag(ui, g.task.priority);
            if (flag) meta.child(flag);
            if (g.start != null) meta.child(ui.muted(blockTimes(g), { size: 12 }));
            if (g.steps.length > 0) meta.child(h_flex().items_center().gap(6).child(div().w(40).child(ui.meter(g.steps.filter((s) => s.status === "done").length / g.steps.length, { height: 5 }))).child(ui.muted(stepsDone(g.steps), { size: 12 })));
            if (g.rollover > 0) meta.child(ui.badge(`Carried over ${g.rollover}×`, { tone: "warn" }));
            if (g.skipped) meta.child(ui.badge("Skipped"));
            if (!g.done && !g.skipped && g.start == null) meta.child(ui.text("No room left today", { size: 12, color: ui.tone("warn") }));
            const line = h_flex()
              .id(`td-ag-${g.task.id}`)
              .items_center()
              .gap(12)
              .px(10)
              .py(9)
              .rounded(9)
              .hover((s) => s.bg(t.hover))
              .when(Boolean(g.task.notes), (e) => e.tooltip(g.task.notes.slice(0, 400)))
              .child(ui.checkbox(`td-ag-check-${g.task.id}`, g.done, (on, cx) => (on ? this.completer.complete(g.task, g.steps.length > 0, cx) : this.completer.reopen(g.task)), { round: true, size: 18, disabled: g.skipped }))
              .child(
                v_flex()
                  .flex_1()
                  .min_w(0)
                  .gap(5)
                  .child(div().truncate().text_size(14).text_color(g.done || g.skipped ? t.dim : t.foreground).when(g.done, (e) => e.line_through()).child(g.task.title))
                  .child(meta),
              )
              .child(ui.muted(formatDuration(g.minutes * 60_000), { size: 13 }));
            if (live) line.child(ui.badge("Tracking", { tone: "ok", icon: "radio" }));
            else if (!g.done && !g.skipped) line.child(ui.iconButton(`td-ag-start-${g.task.id}`, "play", () => tr.startTask(g.task), { tip: `Start ${g.task.title}` }));
            const item = v_flex().rounded(9).when(live, (e) => e.bg(t.raised)).child(line);
            if (g.steps.length > 0) item.child(div().pb(6).child(this.steps.list(ui, g.task, g.steps, { adding: live })));
            return item;
          }),
        ),
      );

    // To-do -------------------------------------------------------------------
    const todoPanel = ui
      .panel({
        title: "To-do",
        icon: "list-todo",
        actions: ui.button("td-new-task", "New task", (e, cx) => new TaskForm(this.app, { goals: this.goals }).open(cx), { small: true, tone: "ghost", icon: "plus" }),
      })
      .child(h_flex().gap(8).child(this.todo.el(ui)).child(ui.iconButton("td-todo-add", "plus", (e, cx) => this.addTodo(cx), { tip: "Add to-do", size: 32, disabled: !this.todo.value.trim() })));
    if (todos.length === 0) todoPanel.child(ui.muted("Nothing left to tick off.", { size: 13 }));
    else
      todoPanel.child(
        v_flex().children(
          todos.map((x) => {
            const steps = gwen.tasks.filter((s) => s.parent_id === x.id);
            const line = h_flex()
              .id(`td-todo-${x.id}`)
              .items_center()
              .gap(10)
              .px(8)
              .py(7)
              .rounded(7)
              .hover((s) => s.bg(t.hover))
              .when(Boolean(x.notes), (e) => e.tooltip(x.notes.slice(0, 400)))
              .child(ui.checkbox(`td-todo-check-${x.id}`, false, (on, cx) => this.completer.complete(x, steps.length > 0, cx), { round: true }))
              .child(div().flex_1().min_w(0).truncate().text_size(13.5).child(x.title));
            const flag = priorityFlag(ui, x.priority);
            if (flag) line.child(flag);
            if (x.start_minute != null && x.start_day === today) line.child(ui.muted(formatClock(x.start_minute), { size: 12 }));
            if (x.due_day && x.due_day <= today) line.child(ui.text(x.due_day < today ? "Late" : "Due today", { size: 12, color: ui.tone(x.due_day < today ? "danger" : "warn") }));
            const item = v_flex().child(line);
            if (steps.length > 0) item.child(div().pb(4).child(this.steps.list(ui, x, steps, { adding: false })));
            return item;
          }),
        ),
      );

    // Goals --------------------------------------------------------------------
    const goalsPanel = ui.panel({ title: "Goals", icon: "target", actions: this.goals.length > 0 ? ui.button("td-goals-all", "All", () => this.app.go("goals"), { small: true, tone: "ghost" }) : null });
    if (this.goals.length === 0)
      goalsPanel.child(
        ui.empty({ title: "No goals yet", body: "A goal such as 300 problems by December turns into daily sessions on your plan.", action: ui.button("td-goal-set", "Set a goal", () => this.app.go("goals"), { icon: "target" }) }),
      );
    else
      goalsPanel.child(
        v_flex()
          .gap(14)
          .children(
            shownGoals.map((g) =>
              v_flex()
                .gap(6)
                .child(h_flex().items_center().gap(8).child(div().flex_1().min_w(0).truncate().text_size(13.5).child(g.title)).child(paceChip(ui, g.progress.pace)))
                .child(goalMeter(ui, g, today))
                .child(ui.muted(goalAmount(g), { size: 12 })),
            ),
          ),
      );

    // Where today went -----------------------------------------------------------
    const breakdown = ui.panel({ title: "Where today went", icon: "chart-pie" });
    if (byTotal === 0) breakdown.child(ui.muted("No work tracked yet today.", { size: 13 }));
    else
      breakdown
        .child(v_flex().gap(2).child(div().text_size(24).font_semibold().child(formatDuration(byTotal))).child(ui.muted(`across ${byProject.length} ${byProject.length === 1 ? "project" : "projects"}`, { size: 12 })))
        .child(
          v_flex()
            .gap(14)
            .children(
              byProject.map((p) =>
                h_flex()
                  .items_center()
                  .gap(12)
                  .child(div().w(26).h(26).flex_none().rounded(6).bg(p.color))
                  .child(
                    v_flex()
                      .flex_1()
                      .min_w(0)
                      .gap(6)
                      .child(h_flex().gap(8).child(div().flex_1().min_w(0).truncate().text_size(13).font_medium().child(p.name)).child(div().flex_none().text_size(13).font_medium().child(formatDuration(p.worked_ms))))
                      .child(h_flex().items_center().gap(8).child(ui.meter(p.worked_ms / byTotal, { color: p.color, height: 5 })).child(div().w(34).flex_none().text_size(12).text_color(t.muted).child(`${Math.round((p.worked_ms / byTotal) * 100)}%`))),
                  ),
              ),
            ),
        );

    const columns = h_flex()
      .w_full()
      .items_start()
      .gap(16)
      .child(div().flex_1().min_w(0).child(agendaPanel))
      .child(v_flex().w(380).flex_none().gap(16).child(todoPanel).child(goalsPanel).child(breakdown));

    return ui.scroll("td-scroll", [header, card, schedule, columns]);
  }
}
