// Inbox (Getting Things Done): capture everything here without sorting it,
// then process it into actions, the calendar, someone else's hands, someday,
// or the trash.

import { div } from "gpui-kit";
import { v_flex, h_flex } from "gpui-base";
import { api, gwen } from "../../lib/gwen.js";
import { formatAgo, parseDuration } from "../../lib/format.js";
import { TextField, priorities } from "../../lib/kit.js";
import { atOn, efforts, projectTag, scheduleInput } from "../../lib/parts.js";

export default class Inbox {
  constructor(app) {
    this.app = app;
    this.capturing = false; // Enter and the button both capture: once per press, however quick
    this.text = new TextField({
      multiline: true,
      rows: 1,
      grow: [1, 6],
      placeholder: "What's on your mind? Buy printer ink, email Sam about the trip…",
      onSubmit: (v, cx) => this.capture(cx),
    });
  }

  capture(cx) {
    // One task per line, so a pasted list lands as several.
    const lines = this.text.value
      .split("\n")
      .map((l) => l.replace(/^[\s\-*•\d.)]+/, "").trim())
      .filter(Boolean);
    if (!lines.length || this.capturing) return;
    this.capturing = true;
    cx.notify();
    this.app.spawn(cx, async () => {
      let n = 0;
      for (const title of lines) if (await gwen.act(() => api.CreateTask({ title: title.slice(0, 200), stage: "inbox" }))) n++;
      this.capturing = false;
      if (n) {
        this.text.set("");
        gwen.toast(n === 1 ? "Captured to your inbox" : `Captured ${n} things to your inbox`, "success");
      }
    });
  }

  remove(t, cx) {
    this.app.spawn(cx, async (cx) => {
      if (await this.app.ui.confirm({ title: `Delete "${t.title}"?`, confirm: "Delete", danger: true }, cx)) await gwen.act(() => api.DeleteTask(t.id));
    });
  }

  /** Rows with a line between them. */
  list(ui, rows) {
    const t = ui.t;
    return v_flex().w_full().children(rows.map((row, i) => row.when(i < rows.length - 1, (e) => e.border_b(1).border_color(t.border))));
  }

  render(ui) {
    const t = ui.t;
    const inbox = gwen.tasks.filter((x) => x.stage === "inbox" && !x.parent_id);
    const someday = gwen.tasks.filter((x) => x.stage === "someday" && !x.parent_id);
    const waiting = gwen.tasks.filter((x) => x.stage === "waiting" && !x.parent_id);
    const now = gwen.now();

    const header = ui.pageHeader(
      "Inbox",
      "Get it out of your head: capture everything here, sort it later.",
      inbox.length > 0
        ? h_flex()
            .gap(8)
            .child(ui.button("in-assistant", "Let the assistant sort it", () => this.app.go("assistant", null, "Process my inbox"), { icon: "sparkles" }))
            .child(ui.button("in-process", "Process inbox", (e, cx) => new Process(this.app, inbox).open(cx), { tone: "primary", icon: "wand-2" }))
        : null,
    );

    const capture = ui
      .panel({ gap: 8 })
      .child(
        h_flex()
          .items_start()
          .gap(8)
          .child(this.text.el(ui))
          .child(ui.button("in-capture", "Capture", (e, cx) => this.capture(cx), { tone: "primary", icon: "plus", busy: this.capturing, disabled: !this.text.value.trim() })),
      )
      .child(ui.dim("Enter captures; Shift+Enter for a new line, and each line becomes its own item."));

    const toProcess = ui.panel({ title: `To process · ${inbox.length}`, icon: "inbox", gap: 6 });
    if (inbox.length === 0) toProcess.child(ui.empty({ icon: "circle-check", title: "Inbox zero", body: "Everything is sorted. Capture new things above as they come up." }));
    else
      toProcess.child(
        this.list(
          ui,
          inbox.map((x) =>
            h_flex()
              .items_center()
              .gap(12)
              .py(7)
              .child(div().flex_1().min_w(0).truncate().text_size(14).text_color(t.foreground).child(x.title))
              .child(div().flex_none().text_size(12).text_color(t.dim).child(formatAgo(x.created_at, now)))
              .child(ui.iconButton(`in-del-${x.id}`, "trash-2", (e, cx) => this.remove(x, cx), { tip: "Delete", size: 24 })),
          ),
        ),
      );

    const back = (x) => gwen.act(() => api.PatchTask(x.id, { stage: "todo" }));
    const waitingPanel = ui.panel({ title: `Waiting for · ${waiting.length}`, icon: "hand", gap: 6 });
    if (waiting.length === 0) waitingPanel.child(ui.muted("Nothing handed off. Delegated tasks show here until they come back.", { size: 13 }));
    else
      waitingPanel.child(
        this.list(
          ui,
          waiting.map((x) =>
            h_flex()
              .items_center()
              .gap(12)
              .py(7)
              .child(div().flex_1().min_w(0).truncate().text_size(13).text_color(t.foreground).child(x.title))
              .when(Boolean(x.delegated_to), (e) => e.child(div().flex_none().max_w(140).truncate().text_size(12).text_color(t.muted).child(x.delegated_to)))
              .child(ui.button(`in-back-${x.id}`, "It's back", () => back(x), { small: true })),
          ),
        ),
      );

    const somedayPanel = ui.panel({ title: `Someday / maybe · ${someday.length}`, icon: "archive", gap: 6 });
    if (someday.length === 0) somedayPanel.child(ui.muted("Ideas you parked. Look through them at your weekly review.", { size: 13 }));
    else
      somedayPanel.child(
        this.list(
          ui,
          someday.map((x) =>
            h_flex()
              .items_center()
              .gap(12)
              .py(7)
              .child(div().flex_1().min_w(0).truncate().text_size(13).text_color(t.foreground).child(x.title))
              .child(div().flex_none().max_w(160).child(projectTag(ui, gwen.projects.find((p) => p.id === x.project_id))))
              .child(ui.button(`in-activate-${x.id}`, "Activate", () => back(x), { small: true })),
          ),
        ),
      );

    return ui.scroll("in-scroll", [header, capture, toProcess, h_flex().w_full().items_start().gap(16).child(div().flex_1().min_w(0).child(waitingPanel)).child(div().flex_1().min_w(0).child(somedayPanel))]);
  }
}

/** One inbox item at a time, through the GTD questions. */
class Process {
  constructor(app, tasks) {
    this.app = app;
    this.today = gwen.today();
    // The list as it was when processing began; items leave the inbox as they are sorted.
    this.queue = tasks;
    this.i = 0;
    this.busy = false;
    this.who = new TextField({ placeholder: "Alex", onSubmit: (v, cx) => this.delegate(cx) });
    this.reset();
  }

  get task() {
    return this.queue[this.i];
  }

  /** Every item starts from the first question. */
  reset() {
    const t = this.task;
    this.step = "actionable";
    this.who.set("");
    this.block = this.defaultBlock();
    this.project = t?.project_id ?? "";
    this.estimate = 30; // minutes
    this.effort = 0;
    this.priority = t?.priority ?? 2;
  }

  defaultBlock() {
    return { day: this.today, minute: 17 * 60, length: "30m" };
  }

  title() {
    return this.task ? `Process inbox · ${this.i + 1} of ${this.queue.length}` : "Inbox processed";
  }

  open(cx) {
    const ui = this.app.ui;
    this.id = ui.openDialog({ title: this.title(), width: 560, render: (ui) => this.render(ui), actions: (ui) => this.actions(ui) }, cx);
  }

  close(cx) {
    this.app.ui.closeDialog(cx, this.id);
  }

  next(cx) {
    this.i++;
    this.reset();
    const dialog = this.app.ui.dialogs.find((d) => d.id === this.id);
    if (dialog) dialog.title = this.title();
    cx.notify();
  }

  go(step, cx) {
    this.step = step;
    if (step === "delegate") this.who.focus();
    cx.notify();
  }

  settle(cx, call, said) {
    if (this.busy) return;
    this.busy = true;
    cx.notify();
    this.app.spawn(cx, async (cx) => {
      const ok = await gwen.run(call);
      this.busy = false;
      if (ok) {
        gwen.toast(said, "success");
        this.next(cx);
      }
    });
  }

  delegate(cx) {
    const t = this.task;
    const who = this.who.value;
    if (!t || !who.trim()) return;
    this.settle(cx, () => api.PatchTask(t.id, { stage: "waiting", delegated_to: who }), `Waiting on ${who} for ${t.title}`);
  }

  actions(ui) {
    if (!this.task) return [ui.button("proc-done", "Done", (e, cx) => this.close(cx), { tone: "primary" })];
    const back = { delegate: "how", calendar: "how", next: "how", how: "two" }[this.step] ?? "actionable";
    return [
      this.step !== "actionable" ? ui.button("proc-back", "Back", (e, cx) => this.go(back, cx), { tone: "ghost" }) : null,
      div().flex_1(),
      ui.button("proc-skip", "Skip for now", (e, cx) => this.next(cx)),
    ];
  }

  choice(ui, id, icon, title, sub, onClick) {
    const t = ui.t;
    const el = h_flex()
      .id(id)
      .flex_1()
      .min_w(0)
      .items_start()
      .gap(12)
      .p(12)
      .rounded(8)
      .border_1()
      .border_color(t.input)
      .bg(t.raised)
      .child(div().pt(1).child(ui.icon(icon, 18, t.accent)))
      .child(v_flex().flex_1().min_w(0).gap(2).child(div().text_size(14).font_medium().text_color(t.foreground).child(title)).child(div().text_size(12).line_height("17px").text_color(t.muted).child(sub)));
    if (this.busy) return el.opacity(0.5);
    return el
      .cursor_pointer()
      .hover((s) => s.border_color(t.accent))
      .on_click(onClick);
  }

  render(ui) {
    const t = ui.t;
    const x = this.task;
    if (!x) return ui.muted("Every item has a place now. Your mind can let go of them.", { size: 14, lh: 20 });
    const step = this.step;
    const patch = (req) => () => api.PatchTask(x.id, req);
    const question = (text) => ui.muted(text, { size: 14, lh: 20 });
    const pair = (a, b) => h_flex().w_full().items_stretch().gap(8).child(a).child(b);
    const redraw = (cx) => cx.notify();
    const el = v_flex()
      .w_full()
      .gap(16)
      .child(div().w_full().px(16).py(12).rounded(8).border_1().border_color(t.border).bg(t.background).text_size(15).font_medium().text_color(t.foreground).child(x.title));

    if (step === "actionable")
      el.child(question("Is it actionable: is there something you can do about it?")).child(
        pair(
          this.choice(ui, "proc-yes", "check", "Yes", "There is a next step", (e, cx) => this.go("two", cx)),
          this.choice(ui, "proc-no", "x", "No", "Trash it, or keep it for someday", (e, cx) => this.go("not", cx)),
        ),
      );

    if (step === "not")
      el.child(
        pair(
          this.choice(ui, "proc-trash", "trash-2", "Trash", "It doesn't matter any more", (e, cx) => this.settle(cx, () => api.DeleteTask(x.id), `Deleted ${x.title}`)),
          this.choice(ui, "proc-someday", "archive", "Someday / maybe", "Park it; look again at the weekly review", (e, cx) => this.settle(cx, patch({ stage: "someday" }), `Parked ${x.title} for someday`)),
        ),
      );

    if (step === "two")
      el.child(question("Does it take less than two minutes?")).child(
        pair(
          this.choice(ui, "proc-now", "timer", "Yes: do it now", "Then tick it off here", (e, cx) => this.settle(cx, () => api.CompleteTask(x.id, {}), `Done: ${x.title}`)),
          this.choice(ui, "proc-longer", "list-todo", "No, it takes longer", "Decide who and when", (e, cx) => this.go("how", cx)),
        ),
      );

    if (step === "how")
      el.child(
        v_flex()
          .w_full()
          .gap(8)
          .child(h_flex().w_full().child(this.choice(ui, "proc-delegate", "hand", "Delegate it", "Someone else should do it; you wait for it", (e, cx) => this.go("delegate", cx))))
          .child(h_flex().w_full().child(this.choice(ui, "proc-calendar", "calendar-clock", "It has a set date or time", "Put it in your calendar", (e, cx) => this.go("calendar", cx))))
          .child(h_flex().w_full().child(this.choice(ui, "proc-next", "list-todo", "It's my next action", "Give it a project; Gwen plans it by urgency", (e, cx) => this.go("next", cx)))),
      );

    if (step === "delegate")
      el.child(ui.field("Who are you handing it to?", this.who.el(ui))).child(
        h_flex()
          .w_full()
          .justify_end()
          .child(ui.button("proc-do-delegate", "Delegate", (e, cx) => this.delegate(cx), { tone: "primary", busy: this.busy, disabled: !this.who.value.trim() })),
      );

    if (step === "calendar") {
      const block = this.block;
      el.child(
        scheduleInput(ui, "proc-sched", block, this.today, (b, cx) => {
          this.block = b ?? this.defaultBlock();
          redraw(cx);
        }),
      ).child(
        h_flex()
          .w_full()
          .justify_end()
          .child(
            ui.button(
              "proc-do-calendar",
              "Put it on the calendar",
              (e, cx) =>
                this.settle(
                  cx,
                  async () => {
                    const minutes = Math.round(parseDuration(block.length) / 60_000);
                    await api.PatchTask(x.id, { estimate_minutes: minutes, stage: "todo" });
                    return api.ScheduleTask({ task_id: x.id, day: block.day, start_at: atOn(block.day, block.minute), planned_minutes: minutes });
                  },
                  `Scheduled ${x.title}`,
                ),
              { tone: "primary", busy: this.busy, disabled: parseDuration(block.length) <= 0 },
            ),
          ),
      );
    }

    if (step === "next")
      el.child(
        h_flex()
          .w_full()
          .items_start()
          .gap(16)
          .child(
            div().flex_1().min_w(0).child(
              ui.field(
                "Project",
                ui.select("proc-project", [{ value: "", label: "None" }, ...gwen.projects.map((p) => ({ value: p.id, label: p.name, dot: p.color }))], this.project, (v, cx) => {
                  this.project = v;
                  redraw(cx);
                }),
              ),
            ),
          )
          .child(
            div().flex_1().min_w(0).child(
              ui.field(
                "How long",
                ui.minutesInput("proc-estimate", this.estimate, (m, cx) => {
                  this.estimate = m ?? 0;
                  redraw(cx);
                }),
                "0 makes it a quick to-do you tick off.",
              ),
            ),
          ),
      )
        .child(
          ui.field(
            "Priority",
            h_flex().child(
              ui.segmented("proc-priority", priorities.map((p) => ({ value: p.value, label: p.label })), this.priority, (v, cx) => {
                this.priority = v;
                redraw(cx);
              }),
            ),
          ),
        )
        .child(
          ui.field(
            "How hard",
            h_flex().child(
              ui.segmented("proc-effort", efforts.map((e) => ({ value: e.value, label: e.label })), this.effort, (v, cx) => {
                this.effort = v;
                redraw(cx);
              }),
            ),
          ),
        )
        .child(
          h_flex()
            .w_full()
            .justify_end()
            .child(
              ui.button(
                "proc-do-next",
                "Make it a next action",
                (e, cx) => {
                  const minutes = this.estimate;
                  this.settle(cx, patch({ stage: "todo", project_id: this.project || null, estimate_minutes: minutes > 0 ? minutes : null, priority: this.priority, effort: this.effort || null }), `${x.title} is a next action`);
                },
                { tone: "primary", busy: this.busy },
              ),
            ),
        );
    return el;
  }
}
