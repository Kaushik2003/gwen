// Projects & tasks: projects colour your time; tasks are what you plan and
// track. The list of projects on the left, the chosen one's tasks on the
// right, with their steps, repeating tasks' copies, and goal queues.

import { div } from "gpui-kit";
import { v_flex, h_flex } from "gpui-base";
import { api, gwen, tracking } from "../../lib/gwen.js";
import { formatDate, formatDuration, relativeDay } from "../../lib/format.js";
import { describeRule } from "../../lib/rrule.js";
import { TextField } from "../../lib/kit.js";
import { Completer, Steps, TaskForm, dueTag, effortTag, priorityFlag, projectTag, stageTag, startTag } from "../../lib/parts.js";

/** The colours a project can take: the first ten go to new projects in turn. */
const projectPalette = ["#5e6ad2", "#27a644", "#4ea7fc", "#f2c94c", "#eb5757", "#f2994a", "#bb87fc", "#4cb782", "#e46aa8", "#8a8f98"];
const moreColours = ["#3ec1af", "#d72638", "#e9935a", "#26b5ce", "#a3b14b", "#c879d8", "#ff7eb6", "#6f7bf7", "#b08968", "#62666d"];

export default class Projects {
  constructor(app, parent) {
    this.app = app;
    this.parent = parent;
    this.projects = [];
    this.tasks = null; // every task of the chosen project, steps and templates included
    this.showArchived = false;
    this.selected = "";
    this.status = "open";
    this.picking = false; // select mode: the rows' boxes pick tasks for one bulk delete
    this.picked = new Set();
    this.openQueues = new Set();
    this.stepping = new Set(); // tasks whose "add step" row is shown
    this.loaded = {};
    this.completer = new Completer(app);
    this.steps = new Steps(app, this.completer);
    this.newName = new TextField({ placeholder: "New project", onSubmit: (v, cx) => this.addProject(cx) });
  }

  sync(cx) {
    const v = gwen.versions;
    this.parent.load(cx, "projects", `${v.projects}/${this.showArchived}`, () => api.ListProjects(this.showArchived ? "all" : "false"), (l) => (this.projects = l.projects ?? []));
    // Every status is fetched, so a parent's done steps still show under it.
    this.parent.load(cx, "ptasks", `${v.tasks}/${this.selected}`, () => api.ListTasks({ project_id: this.selected, status: "all", templates: true }), (l) => (this.tasks = l.tasks ?? []));
  }

  select(id, cx) {
    this.selected = id;
    this.picked = new Set();
    this.sync(cx);
    cx.notify();
  }

  addProject(cx) {
    const name = this.newName.value.trim();
    if (!name) return;
    const color = projectPalette[this.projects.length % projectPalette.length];
    this.app.spawn(cx, async (cx) => {
      const p = await gwen.act(() => api.CreateProject({ name, color }));
      if (p) {
        this.newName.set("");
        this.select(p.id, cx);
      }
    });
  }

  patchProject(p, fields, cx) {
    this.app.spawn(cx, () => gwen.act(() => api.PatchProject(p.id, { ...fields, rev: p.rev })));
  }

  goalOf(x) {
    return this.parent.goals.find((g) => g.id === x.goal_id);
  }

  /** A quantity goal's open items wait in its queue, folded into one row. */
  isItem(x) {
    return this.goalOf(x)?.kind === "quantity" && !x.rrule && !x.template_id && !x.parent_id && x.status === "open";
  }

  /** Open tasks of a project, or of all: steps and lined-up items are counted under their parent. */
  openCount(id) {
    return gwen.tasks.filter((x) => (id === undefined || x.project_id === id) && !x.parent_id && !this.isItem(x)).length;
  }

  removeProject(p, cx) {
    const n = this.openCount(p.id);
    const g = this.parent.goals.filter((x) => x.project_id === p.id).length;
    const parts = [n > 0 && `${n} open ${n === 1 ? "task" : "tasks"}`, g > 0 && `${g} ${g === 1 ? "goal" : "goals"}`].filter(Boolean).join(" and ");
    this.app.spawn(cx, async (cx) => {
      const ok = await this.app.ui.confirm(
        { title: `Delete ${p.name}?`, body: `${parts ? `Its ${parts} go too, with everything planned for them. ` : ""}Time already tracked on it stays in your history.`, confirm: "Delete project", danger: true },
        cx,
      );
      if (ok && (await gwen.run(() => api.DeleteProject(p.id)))) {
        if (this.selected === p.id) this.select("", cx);
        gwen.toast(`Deleted ${p.name}`, "success");
      }
    });
  }

  removeTask(x, cx) {
    const steps = (this.tasks ?? []).filter((s) => s.parent_id === x.id).length;
    const g = this.goalOf(x);
    let what = "Time already tracked on it stays in your history.";
    if (x.rrule) what = "It stops repeating, and its open copies leave your plan. " + what;
    else if (x.template_id && g?.kind === "quantity" && steps > 0) what = `Its ${steps} ${g.unit || "units"} go back to the goal's queue for the next session.`;
    else if (steps > 0) what = `Its ${steps} ${steps === 1 ? "step goes" : "steps go"} too. ` + what;
    this.app.spawn(cx, async (cx) => {
      if (await this.app.ui.confirm({ title: `Delete ${x.title}?`, body: what, confirm: "Delete task", danger: true }, cx)) await gwen.run(() => api.DeleteTask(x.id));
    });
  }

  removePicked(cx) {
    const ids = [...this.picked];
    this.app.spawn(cx, async (cx) => {
      const ok = await this.app.ui.confirm(
        { title: `Delete ${ids.length} ${ids.length === 1 ? "task" : "tasks"}?`, body: "Their steps and repeats go too. Time already tracked stays in your history.", confirm: `Delete ${ids.length}`, danger: true },
        cx,
      );
      if (!ok) return;
      const out = await gwen.act(() => api.DeleteTasks({ ids }));
      if (!out) return;
      gwen.toast(`Deleted ${out.task_ids.length} ${out.task_ids.length === 1 ? "task" : "tasks"}`, "success");
      this.picking = false;
      this.picked = new Set();
    });
  }

  editTask(task, cx) {
    const goals = this.parent.goals.filter((g) => g.status === "active" || g.id === (task?.goal_id ?? ""));
    new TaskForm(this.app, { task, goals, defaultProject: this.selected }).open(cx);
  }

  editProject(p, cx) {
    const ui = this.app.ui;
    const name = new TextField({ value: p.name, onSubmit: (v, cx) => save(cx) });
    let color = p.color;
    const save = (cx) => {
      if (!name.value.trim()) return;
      this.app.spawn(cx, async (cx) => {
        if (await gwen.act(() => api.PatchProject(p.id, { name: name.value.trim(), color, rev: p.rev }))) ui.closeDialog(cx, id);
      });
    };
    const swatches = [...projectPalette, ...moreColours];
    if (!swatches.includes(p.color.toLowerCase())) swatches.push(p.color);
    const id = ui.openDialog(
      {
        title: "Edit project",
        width: 440,
        render: (ui) =>
          v_flex()
            .gap(16)
            .child(ui.field("Name", name.el(ui)))
            .child(
              ui.field(
                "Colour",
                h_flex()
                  .flex_wrap()
                  .gap(8)
                  .children(
                    swatches.map((c) => {
                      const on = color.toLowerCase() === c.toLowerCase();
                      return div()
                        .id(`pj-colour-${c}`)
                        .w(26)
                        .h(26)
                        .rounded(13)
                        .bg(c)
                        .cursor_pointer()
                        .when(on, (e) => e.border_2().border_color(ui.t.foreground))
                        .on_click((e, cx) => {
                          color = c;
                          cx.notify();
                        });
                    }),
                  ),
              ),
            ),
        actions: (ui) => [ui.button("pj-cancel", "Cancel", (e, cx) => ui.closeDialog(cx, id), { tone: "ghost" }), ui.button("pj-save", "Save project", (e, cx) => save(cx), { tone: "primary", disabled: !name.value.trim() })],
      },
      cx,
    );
    name.focus();
  }

  flip(set, id, cx) {
    if (set.has(id)) set.delete(id);
    else set.add(id);
    cx.notify();
  }

  // Drawing ------------------------------------------------------------------

  projectItem(ui, o) {
    const t = ui.t;
    const row = h_flex()
      .id(`pj-item-${o.id || "all"}`)
      .items_center()
      .gap(9)
      .px(10)
      .h(32)
      .rounded(7)
      .cursor_pointer()
      .when(o.selected, (e) => e.bg(t.selected))
      .when(!o.selected, (e) => e.hover((s) => s.bg(t.hover)))
      .on_click((e, cx) => this.select(o.id, cx))
      .child(ui.dot(o.color, 9))
      .child(div().flex_1().min_w(0).truncate().text_size(13).text_color(o.archived ? t.dim : o.selected ? t.foreground : t.muted).child(o.name));
    if (o.archived) row.child(ui.dim("archived", { size: 11.5 }));
    else if (o.count > 0) row.child(ui.dim(String(o.count), { size: 12 }));
    if (o.onEdit) row.child(ui.iconButton(`pj-item-edit-${o.id}`, "pencil", (e, cx) => o.onEdit(cx), { tip: `Edit ${o.name}`, size: 22 }));
    return row;
  }

  taskRow(ui, x, step = false, label) {
    const t = ui.t;
    const tr = tracking();
    const today = gwen.today();
    const isDone = x.status === "done";
    const template = Boolean(x.rrule);
    const live = tr.isTracking(x.id);
    const goal = this.goalOf(x);
    const project = !this.selected ? (this.projects.find((p) => p.id === x.project_id) ?? gwen.projects.find((p) => p.id === x.project_id)) : undefined;
    const steps = (this.tasks ?? []).filter((s) => s.parent_id === x.id);
    const id = `pj-t-${x.id}`;
    const mark = this.picking
      ? ui.checkbox(`${id}-pick`, this.picked.has(x.id), (on, cx) => this.flip(this.picked, x.id, cx))
      : template
        ? div().id(`${id}-rep`).tooltip("Repeating: each day's copy is what you complete").child(ui.icon("repeat", 14))
        : ui.checkbox(`${id}-check`, isDone, (on, cx) => (on ? this.completer.complete(x, steps.length > 0, cx) : this.completer.reopen(x)), { round: true });
    const title = div()
      .text_size(step ? 13 : 13.5)
      .text_color(isDone ? t.dim : t.foreground)
      .when(isDone, (e) => e.line_through())
      .child(label ? `${label} · ${x.title}` : x.title);
    const body = v_flex()
      .id(`${id}-open`)
      .flex_1()
      .min_w(0)
      .gap(3)
      .cursor_pointer()
      .on_click((e, cx) => (this.picking ? this.flip(this.picked, x.id, cx) : this.editTask(x, cx)))
      .child(title);
    if (x.notes) body.child(div().text_size(12).text_color(t.muted).truncate().child(x.notes.split("\n")[0]));
    if (!step) {
      const meta = h_flex().flex_wrap().items_center().gap(10);
      if (project) meta.child(projectTag(ui, project));
      if (goal) meta.child(h_flex().items_center().gap(4).text_size(12).text_color(t.muted).child(ui.icon("target", 12)).child(`${goal.title}${x.quantity != null ? ` (${x.quantity} ${goal.unit || "units"})` : ""}`));
      if (template) meta.child(ui.badge(describeRule(x.rrule), { icon: "repeat" }));
      for (const tag of [!isDone && startTag(ui, x, today), x.due_day && dueTag(ui, x.due_day, today, isDone), priorityFlag(ui, x.priority), effortTag(ui, x.effort), stageTag(ui, x)]) if (tag) meta.child(tag);
      if (x.estimate_minutes != null) meta.child(ui.muted(`${formatDuration(x.estimate_minutes * 60_000)} estimated`, { size: 12 }));
      body.child(meta);
    }
    const row = h_flex()
      .items_start()
      .gap(11)
      .px(10)
      .py(step ? 6 : 9)
      .rounded(8)
      .when(live, (e) => e.bg(t.raised))
      .when(!live, (e) => e.hover((s) => s.bg(t.hover)))
      .child(div().pt(1).child(mark))
      .child(body);
    if (x.tracked_ms > 0 || (step && x.estimate_minutes != null))
      row.child(div().flex_none().pt(1).text_size(12).text_color(t.muted).child(formatDuration(x.tracked_ms > 0 ? x.tracked_ms : x.estimate_minutes * 60_000)));
    if (!this.picking) {
      const controls = h_flex().flex_none().items_center().gap(2);
      if (!template && !isDone && !step) {
        if (live) controls.child(div().id(`${id}-live`).tooltip("Tracking now").px(4).child(ui.icon("radio", 15, ui.tone("ok"))));
        else controls.child(ui.iconButton(`${id}-play`, "play", () => tr.startTask(x), { tip: `Start ${x.title}`, size: 24 }));
      }
      const canStep = !(x.parent_id || x.rrule || (step && !x.template_id));
      if (canStep && !isDone) controls.child(ui.iconButton(`${id}-step`, "list-plus", (e, cx) => this.flip(this.stepping, x.id, cx), { tip: `Add a step to ${x.title}`, size: 24 }));
      controls.child(ui.iconButton(`${id}-edit`, "pencil", (e, cx) => this.editTask(x, cx), { tip: `Edit ${x.title}`, size: 24 }));
      controls.child(ui.iconButton(`${id}-del`, "trash-2", (e, cx) => this.removeTask(x, cx), { tip: `Delete ${x.title}`, size: 24 }));
      row.child(controls);
    }
    return row;
  }

  render(ui) {
    const t = ui.t;
    const today = gwen.today();
    const all = this.tasks ?? [];
    const listed = new Set(all.map((x) => x.id));
    const stepsOf = (id) => all.filter((x) => x.parent_id === id);
    const shown = (x) => this.status === "all" || x.status === this.status;
    // A repeating task's copies sit under it, so the task and today's copy never read as two of the same task.
    const shownIds = new Set(all.filter(shown).map((x) => x.id));
    const copiesOf = (id) => all.filter((x) => x.template_id === id && shown(x)).sort((a, b) => (a.occurrence_day ?? "").localeCompare(b.occurrence_day ?? ""));
    const top = all.filter((x) => !(x.parent_id && listed.has(x.parent_id)) && !(x.template_id && shownIds.has(x.template_id)) && !this.isItem(x) && shown(x));
    const queues = this.parent.goals.map((g) => ({ goal: g, items: all.filter((x) => this.isItem(x) && x.goal_id === g.id) })).filter((q) => q.items.length > 0 && this.status !== "done");
    const visible = [...top.flatMap((x) => [x, ...stepsOf(x.id), ...copiesOf(x.id).flatMap((c) => [c, ...stepsOf(c.id)])]), ...queues.flatMap((q) => (this.openQueues.has(q.goal.id) ? q.items : []))];
    const copyLabel = (c) => {
      if (!c.occurrence_day) return undefined;
      const rel = relativeDay(c.occurrence_day, today);
      return ["today", "tomorrow", "yesterday"].includes(rel) ? rel[0].toUpperCase() + rel.slice(1) : formatDate(c.occurrence_day);
    };
    const current = this.projects.find((p) => p.id === this.selected);

    // Projects --------------------------------------------------------------
    const projectsPanel = ui
      .panel({
        title: "Projects",
        icon: "folder-kanban",
        pad: 12,
        gap: 8,
        actions: h_flex()
          .items_center()
          .gap(8)
          .child(ui.dim("Archived", { size: 12 }))
          .child(
            ui.toggle("pj-archived", this.showArchived, (on, cx) => {
              this.showArchived = on;
              this.sync(cx);
            }),
          ),
      })
      .w(260)
      .flex_none()
      .child(
        v_flex()
          .gap(1)
          .child(this.projectItem(ui, { id: "", name: "All projects", color: t.dim, selected: this.selected === "", count: this.openCount() }))
          .children(
            this.projects.map((p) =>
              this.projectItem(ui, { id: p.id, name: p.name, color: p.color, archived: Boolean(p.archived_at), selected: this.selected === p.id, count: this.openCount(p.id), onEdit: (cx) => this.editProject(p, cx) }),
            ),
          ),
      )
      .child(h_flex().gap(8).pt(4).child(this.newName.el(ui)).child(ui.iconButton("pj-add", "plus", (e, cx) => this.addProject(cx), { tip: "Add project", size: 32, disabled: !this.newName.value.trim() })));

    // Tasks -------------------------------------------------------------------
    const actions = h_flex().items_center().gap(6);
    if (this.picking) {
      const allPicked = this.picked.size === visible.length && visible.length > 0;
      actions
        .child(ui.muted(`${this.picked.size} selected`, { size: 12 }))
        .child(
          ui.button(
            "pj-pick-all",
            allPicked ? "Select none" : "Select all",
            (e, cx) => {
              this.picked = allPicked ? new Set() : new Set(visible.map((x) => x.id));
              cx.notify();
            },
            { small: true, tone: "ghost" },
          ),
        )
        .child(ui.button("pj-pick-del", `Delete ${this.picked.size || ""}`, (e, cx) => this.removePicked(cx), { small: true, tone: "danger", icon: "trash-2", disabled: this.picked.size === 0 }))
        .child(
          ui.button(
            "pj-pick-done",
            "Done",
            (e, cx) => {
              this.picking = false;
              this.picked = new Set();
              cx.notify();
            },
            { small: true },
          ),
        );
    } else {
      if (current)
        actions
          .child(ui.iconButton("pj-edit", "pencil", (e, cx) => this.editProject(current, cx), { tip: `Edit ${current.name}` }))
          .child(
            ui.iconButton(current.archived_at ? "pj-unarchive" : "pj-archive", current.archived_at ? "archive-restore" : "archive", (e, cx) => this.patchProject(current, { archived: !current.archived_at }, cx), {
              tip: current.archived_at ? "Unarchive" : "Archive: hide it from pickers",
            }),
          )
          .child(ui.iconButton("pj-delete", "trash-2", (e, cx) => this.removeProject(current, cx), { tip: `Delete ${current.name}` }));
      actions.child(
        ui.segmented(
          "pj-status",
          [
            { value: "open", label: "Open" },
            { value: "done", label: "Done" },
            { value: "all", label: "All" },
          ],
          this.status,
          (v, cx) => {
            this.status = v;
            this.picked = new Set();
            cx.notify();
          },
          { small: true },
        ),
      );
      if (all.length > 0)
        actions.child(
          ui.button(
            "pj-select",
            "Select",
            (e, cx) => {
              this.picking = true;
              this.picked = new Set();
              cx.notify();
            },
            { small: true, icon: "check-square" },
          ),
        );
      actions.child(ui.button("pj-new-task", "New task", (e, cx) => this.editTask(null, cx), { small: true, tone: "primary", icon: "plus" }));
    }
    const tasksPanel = ui.panel({ title: current ? current.name : "All tasks", icon: "list-checks", actions, pad: 12 }).flex_1().min_w(0);
    if (this.tasks && top.length === 0 && queues.length === 0)
      tasksPanel.child(
        ui.empty({
          icon: "list-checks",
          title: this.status === "done" ? "Nothing done here yet" : "No tasks",
          body: this.status !== "done" ? "Add a task, or let the assistant break a goal into tasks." : undefined,
          action: ui.button("pj-empty-new", "New task", (e, cx) => this.editTask(null, cx), { icon: "plus" }),
        }),
      );
    else if (!this.tasks) tasksPanel.child(ui.splash("Loading…", { busy: true }));
    else {
      const nested = (children) => v_flex().ml(26).pl(8).border_l(1).border_color(t.border).children(children);
      const stepBlock = (x) => {
        const steps = stepsOf(x.id);
        if (this.picking) return steps.length > 0 ? nested(steps.map((s) => this.taskRow(ui, s, true))) : null;
        if (steps.length > 0 || this.stepping.has(x.id)) return div().pb(4).child(this.steps.list(ui, x, steps));
        return null;
      };
      const list = v_flex();
      for (const x of top) {
        list.child(this.taskRow(ui, x));
        const copies = copiesOf(x.id);
        if (copies.length) {
          const items = [];
          for (const c of copies) {
            items.push(this.taskRow(ui, c, true, copyLabel(c)));
            const sb = stepBlock(c);
            if (sb) items.push(sb);
          }
          list.child(nested(items));
        }
        const sb = stepBlock(x);
        if (sb) list.child(sb);
      }
      for (const { goal: g, items } of queues) {
        const open = this.openQueues.has(g.id);
        const unit = g.unit || "units";
        const units = items.reduce((n, x) => n + (x.quantity ?? 1), 0);
        list.child(
          h_flex()
            .id(`pj-queue-${g.id}`)
            .mt(4)
            .items_center()
            .gap(10)
            .px(10)
            .py(9)
            .rounded(8)
            .cursor_pointer()
            .hover((s) => s.bg(t.hover))
            .on_click((e, cx) => this.flip(this.openQueues, g.id, cx))
            .child(ui.icon(open ? "chevron-down" : "chevron-right", 15))
            .child(ui.icon("layers", 15, t.accent))
            .child(div().flex_1().min_w(0).truncate().text_size(13.5).child(`Lined up for ${g.title}`))
            .child(ui.muted(`${units} ${unit}${g.progress.per_session ? `, about ${Math.ceil(units / g.progress.per_session)} sessions` : ""}`, { size: 12 })),
        );
        if (open) list.child(nested([div().px(10).pb(4).child(ui.dim("Each day's session takes the next ones from the top. Edit or delete any of them.")), ...items.map((x) => this.taskRow(ui, x, true))]));
      }
      tasksPanel.child(list);
    }

    return ui.scroll("pj-scroll", [ui.pageHeader("Projects & tasks", "Projects colour your time. Tasks are what you plan and track."), h_flex().w_full().items_start().gap(16).child(projectsPanel).child(tasksPanel)]);
  }
}
