// Board: every task by stage, as a Kanban board, or as the Eisenhower matrix
// of urgent against important. There is no dragging in the Gwen window: a
// card moves with its arrows, or with "Move to" for any column.

import { div } from "gpui-kit";
import { v_flex, h_flex } from "gpui-base";
import { api, gwen, tracking } from "../../lib/gwen.js";
import { addDays, formatDuration } from "../../lib/format.js";
import { TextField, alpha } from "../../lib/kit.js";
import { Completer, TaskForm, dueTag, effortTag, priorityFlag, projectTag, stages } from "../../lib/parts.js";

/** The board's columns: the stages, then Done. */
const columns = [...stages.map((s) => ({ id: s.value, label: s.label, hint: s.hint })), { id: "done", label: "Done", hint: "Finished this week" }];

/** Where a task sits on the board. */
const columnOf = (t) => (t.status === "done" ? "done" : t.stage || "todo");

/** Eisenhower: urgent is due within two days; important is priority 3 or 4, or tied to a goal. */
export function quadrantOf(t, today) {
  const urgent = Boolean(t.due_day) && t.due_day <= addDays(today, 2);
  const important = t.priority >= 3 || Boolean(t.goal_id);
  return urgent ? (important ? "do" : "delegate") : important ? "schedule" : "drop";
}

const quadrants = [
  { id: "do", title: "Do first", sub: "Urgent and important", icon: "flame", color: "#eb5757" },
  { id: "schedule", title: "Schedule", sub: "Important, not urgent", icon: "calendar-clock", color: "#5e6ad2" },
  { id: "delegate", title: "Delegate", sub: "Urgent, not important", icon: "hand", color: "#f2994a" },
  { id: "drop", title: "Drop", sub: "Neither: someday, or delete", icon: "trash", color: "#8a8f98" },
];

export default class Board {
  constructor(app, parent) {
    this.app = app;
    this.parent = parent;
    this.view = "kanban";
    try {
      if (localStorage.getItem("board.view") === "matrix") this.view = "matrix";
    } catch {}
    this.project = "";
    this.done = []; // finished in the last week
    this.loaded = null;
    this.completer = new Completer(app);
  }

  sync(cx) {
    const stamp = `${gwen.versions.tasks}`;
    if (this.loaded === stamp) return;
    this.loaded = stamp;
    cx.spawn(async (cx) => {
      const since = Date.now() - 7 * 86_400_000;
      try {
        const l = await api.ListTasks({ status: "done" });
        this.done = (l.tasks ?? []).filter((t) => (t.done_at ?? 0) >= since && !t.parent_id);
      } catch (e) {
        gwen.fail(e);
      }
      cx.notify();
    });
  }

  setView(v, cx) {
    this.view = v;
    try {
      localStorage.setItem("board.view", v);
    } catch {}
    cx.notify();
  }

  edit(task, cx, stage) {
    new TaskForm(this.app, { task, goals: this.parent.activeGoals, defaultProject: this.project === "none" ? "" : this.project, defaultStage: stage }).open(cx);
  }

  /** Moves a task to a column: a stage, or done; out of done reopens it. */
  move(t, to, cx) {
    if (columnOf(t) === to) return;
    if (to === "done") {
      this.completer.complete(t, false, cx);
      return;
    }
    this.app.spawn(cx, async (cx) => {
      if (t.status === "done" && !(await gwen.act(() => api.ReopenTask(t.id)))) return;
      if (to === "waiting" && !t.delegated_to) {
        this.askWho(t, cx);
        return;
      }
      await gwen.act(() => api.PatchTask(t.id, { stage: to }));
    });
  }

  /** Who a task moved to Waiting waits on. */
  askWho(t, cx) {
    const ui = this.app.ui;
    const who = new TextField({ placeholder: "Alex", onSubmit: (v, cx) => go(cx) });
    const go = (cx) => {
      ui.closeDialog(cx, id);
      gwen.act(() => api.PatchTask(t.id, { stage: "waiting", delegated_to: who.value.trim().slice(0, 200) }));
    };
    const id = ui.openDialog(
      {
        title: `Who is "${t.title}" waiting on?`,
        width: 420,
        render: (ui) => ui.field("Person or thing", who.el(ui), "Leave empty when it is simply blocked."),
        actions: (ui) => [ui.button("bd-who-cancel", "Cancel", (e, cx) => ui.closeDialog(cx, id), { tone: "ghost" }), ui.button("bd-who-ok", "Move to Waiting", (e, cx) => go(cx), { tone: "primary" })],
      },
      cx,
    );
    who.focus();
  }

  /** The tasks shown: open ones and those done this week, without steps or later copies of repeating ones. */
  tasks() {
    const today = gwen.today();
    const p = this.project;
    return [...gwen.tasks.filter((t) => !t.parent_id && !(t.occurrence_day && t.occurrence_day > today)), ...this.done].filter((t) => !p || (p === "none" ? !t.project_id : t.project_id === p));
  }

  tags(ui, t, done) {
    const today = gwen.today();
    const project = gwen.projects.find((p) => p.id === t.project_id);
    const row = h_flex().flex_wrap().items_center().gap(8);
    if (project) row.child(projectTag(ui, project));
    if (t.due_day) row.child(dueTag(ui, t.due_day, today, done));
    for (const x of [priorityFlag(ui, t.priority), effortTag(ui, t.effort)]) if (x) row.child(x);
    if (t.estimate_minutes != null) row.child(h_flex().items_center().gap(4).text_size(12).text_color(ui.t.muted).child(ui.icon("clock", 12)).child(formatDuration(t.estimate_minutes * 60_000)));
    if (t.stage === "waiting" && t.delegated_to) row.child(h_flex().items_center().gap(4).text_size(12).text_color(ui.t.muted).child(ui.icon("user-round", 12)).child(t.delegated_to));
    return row;
  }

  card(ui, t, ci) {
    const th = ui.t;
    const tr = tracking();
    const done = t.status === "done";
    const live = tr.isTracking(t.id);
    const project = gwen.projects.find((p) => p.id === t.project_id);
    const id = `bd-card-${t.id}`;
    const controls = h_flex().items_center().gap(2);
    if (ci > 0) controls.child(ui.iconButton(`${id}-l`, "arrow-left", (e, cx) => this.move(t, columns[ci - 1].id, cx), { tip: `Move to ${columns[ci - 1].label}`, size: 22 }));
    if (ci < columns.length - 1) controls.child(ui.iconButton(`${id}-r`, "arrow-right", (e, cx) => this.move(t, columns[ci + 1].id, cx), { tip: `Move to ${columns[ci + 1].label}`, size: 22 }));
    if (!done && !live)
      controls.child(
        ui.iconButton(
          `${id}-play`,
          "play",
          (e, cx) =>
            this.app.spawn(cx, async () => {
              if (t.stage !== "doing") await gwen.act(() => api.PatchTask(t.id, { stage: "doing" }));
              tr.startTask(t);
            }),
          { tip: "Start tracking it", size: 22 },
        ),
      );
    controls.child(div().flex_1());
    controls.child(
      ui.menuButton(
        `${id}-menu`,
        [
          !done ? { label: "Edit", icon: "pencil", onClick: (e, cx) => this.edit(t, cx) } : null,
          ...columns.filter((c) => c.id !== columnOf(t)).map((c) => ({ label: `Move to ${c.label}`, icon: c.id === "done" ? "circle-check" : "arrow-right", onClick: (e, cx) => this.move(t, c.id, cx) })),
        ],
        { tip: "More", size: 22 },
      ),
    );
    return v_flex()
      .id(id)
      .flex_none()
      .w_full()
      .gap(7)
      .p(9)
      .pl(11)
      .rounded(8)
      .border_1()
      .relative()
      .overflow_hidden()
      .border_color(live ? alpha(ui.tone("ok"), 0.6) : th.input)
      .bg(th.raised)
      .when(Boolean(project), (e) => e.child(div().absolute().left(0).top(0).bottom(0).w(3).bg(project.color)))
      .on_click((e, cx) => {
        if (e.click_count === 2 && !done) this.edit(t, cx);
      })
      .child(
        h_flex()
          .items_start()
          .gap(6)
          .when(t.effort === 3 && !done, (e) => e.child(div().id(`${id}-frog`).pt(2).tooltip("A frog: do it first").child(ui.icon("flame", 13, ui.tone("danger")))))
          .child(div().flex_1().min_w(0).text_size(13).line_height("18px").text_color(done ? th.muted : th.foreground).when(done, (e) => e.line_through()).child(t.title)),
      )
      .child(this.tags(ui, t, done))
      .child(controls);
  }

  kanban(ui, tasks) {
    const th = ui.t;
    return div()
      .id("bd-kanban")
      .flex_1()
      .min_h(0)
      .w_full()
      .overflow_x_scrollbar()
      .child(
        h_flex()
          .h_full()
          .items_start()
          .gap(10)
          .px(20)
          .pb(12)
          .children(
            columns.map((c, ci) => {
              const cards = tasks
                .filter((t) => columnOf(t) === c.id)
                .sort((a, b) => (c.id === "done" ? (b.done_at ?? 0) - (a.done_at ?? 0) : b.priority - a.priority || (a.due_day ?? "9").localeCompare(b.due_day ?? "9")));
              return v_flex()
                .w(250)
                .flex_none()
                .h_full()
                .rounded(10)
                .border_1()
                .border_color(th.border)
                .bg(alpha(th.surface, 0.6))
                .child(
                  v_flex()
                    .flex_none()
                    .px(12)
                    .pt(10)
                    .pb(6)
                    .gap(2)
                    .child(
                      h_flex()
                        .h(26)
                        .items_center()
                        .gap(8)
                        .child(div().truncate().text_size(13).font_semibold().child(c.label))
                        .child(div().px(6).rounded(8).bg(th.hover).text_size(11.5).text_color(th.muted).child(String(cards.length)))
                        .child(div().flex_1())
                        .when(c.id !== "done", (e) => e.child(ui.iconButton(`bd-add-${c.id}`, "plus", (ev, cx) => this.edit(null, cx, c.id), { tip: `New task in ${c.label}`, size: 22 }))),
                    )
                    .child(ui.dim(c.hint, { size: 11, truncate: true })),
                )
                .child(
                  div()
                    .id(`bd-col-${c.id}`)
                    .flex_1()
                    .min_h(0)
                    .overflow_y_scrollbar()
                    .child(
                      v_flex()
                        .gap(8)
                        .px(8)
                        .pb(8)
                        .children(cards.map((t) => this.card(ui, t, ci)))
                        .when(cards.length === 0, (e) => e.child(div().py(28).flex().justify_center().rounded(8).border_1().border_color(th.border).text_size(12).text_color(th.dim).child("Nothing here"))),
                    ),
                );
            }),
          ),
      );
  }

  matrix(ui, tasks) {
    const th = ui.t;
    const tr = tracking();
    const today = gwen.today();
    const box = (q) => {
      const list = tasks.filter((t) => quadrantOf(t, today) === q.id).sort((a, b) => (a.due_day ?? "9").localeCompare(b.due_day ?? "9") || b.priority - a.priority);
      return ui
        .panel({ pad: 0, gap: 0 })
        .flex_1()
        .min_w(0)
        .relative()
        .overflow_hidden()
        .child(div().absolute().top(0).left(0).right(0).h(3).bg(q.color))
        .child(
          h_flex()
            .items_center()
            .gap(10)
            .px(14)
            .pt(12)
            .pb(8)
            .child(ui.icon(q.icon, 16, q.color))
            .child(v_flex().flex_1().min_w(0).child(div().text_size(13.5).font_semibold().child(q.title)).child(ui.muted(q.sub, { size: 12 })))
            .child(div().px(7).rounded(8).bg(th.hover).text_size(12).text_color(th.muted).child(String(list.length))),
        )
        .child(
          v_flex()
            .px(4)
            .pb(8)
            .children(
              list.map((t) => {
                const id = `bd-q-${t.id}`;
                const project = gwen.projects.find((p) => p.id === t.project_id);
                const meta = h_flex().flex_wrap().items_center().gap(8).child(projectTag(ui, project));
                if (t.due_day) meta.child(dueTag(ui, t.due_day, today));
                for (const x of [priorityFlag(ui, t.priority), effortTag(ui, t.effort)]) if (x) meta.child(x);
                const actions = h_flex().flex_none().items_center().gap(4);
                if (q.id === "do" && !tr.isTracking(t.id)) actions.child(ui.button(`${id}-start`, "Start", () => tr.startTask(t), { small: true, icon: "play" }));
                if (q.id === "schedule") actions.child(ui.button(`${id}-sched`, "Schedule", (e, cx) => this.edit(t, cx), { small: true, icon: "calendar-clock" }));
                if (q.id === "delegate" && t.stage !== "waiting") actions.child(ui.button(`${id}-deleg`, "Delegate", (e, cx) => this.move(t, "waiting", cx), { small: true, icon: "hand" }));
                if (q.id === "drop") actions.child(ui.button(`${id}-some`, "Someday", (e, cx) => this.move(t, "someday", cx), { small: true }));
                actions.child(ui.iconButton(`${id}-done`, "circle-check", (e, cx) => this.move(t, "done", cx), { tip: "Done", size: 24 }));
                return h_flex()
                  .items_center()
                  .gap(8)
                  .px(10)
                  .py(8)
                  .rounded(7)
                  .hover((s) => s.bg(th.hover))
                  .child(
                    v_flex()
                      .id(`${id}-open`)
                      .flex_1()
                      .min_w(0)
                      .gap(3)
                      .cursor_pointer()
                      .on_click((e, cx) => this.edit(t, cx))
                      .child(div().truncate().text_size(13).child(t.title))
                      .child(meta),
                  )
                  .child(actions);
              }),
            )
            .when(list.length === 0, (e) => e.child(div().py(24).flex().justify_center().text_size(12).text_color(th.dim).child("Nothing here."))),
        );
    };
    return ui.scroll(
      "bd-matrix",
      [h_flex().w_full().items_start().gap(12).child(box(quadrants[0])).child(box(quadrants[1])), h_flex().w_full().items_start().gap(12).child(box(quadrants[2])).child(box(quadrants[3]))],
      { pad: 20, gap: 12 },
    );
  }

  render(ui) {
    const tasks = this.tasks();
    const header = ui.pageHeader(
      "Board",
      this.view === "kanban" ? "Move cards between columns as work moves along. Done this week sits at the end." : "Urgent against important: do, schedule, delegate, or drop.",
      h_flex()
        .gap(8)
        .items_center()
        .child(
          ui.select(
            "bd-project",
            [{ value: "", label: "All projects" }, { value: "none", label: "No project" }, ...gwen.projects.map((p) => ({ value: p.id, label: p.name, dot: p.color }))],
            this.project,
            (v, cx) => {
              this.project = v;
              cx.notify();
            },
            { width: 180 },
          ),
        )
        .child(
          ui.segmented(
            "bd-view",
            [
              { value: "kanban", label: "Kanban", icon: "columns-3" },
              { value: "matrix", label: "Eisenhower", icon: "grid-2x2" },
            ],
            this.view,
            (v, cx) => this.setView(v, cx),
          ),
        ),
    );
    const body =
      this.view === "kanban"
        ? this.kanban(ui, tasks)
        : this.matrix(
            ui,
            tasks.filter((t) => t.status !== "done" && t.stage !== "someday"),
          );
    return v_flex().size_full().min_h(0).child(div().flex_none().px(20).pt(20).pb(14).child(header)).child(body);
  }
}
