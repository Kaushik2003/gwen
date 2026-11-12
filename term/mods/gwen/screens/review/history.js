// History: days by project over a range, and one day's detail: its timeline,
// its pieces of time (change, split, delete, add), and its target and note.

import { div } from "gpui-kit";
import { v_flex, h_flex } from "gpui-base";
import { api, apiError, gwen } from "../../lib/gwen.js";
import { addDays, clockValue, dateOf, formatDate, formatDuration, formatLongDate, formatTime, goDuration, instantOn, parseDuration } from "../../lib/format.js";
import { TextField, alpha, unassignedColor } from "../../lib/kit.js";
import { timeline } from "../../lib/parts.js";

const kinds = [
  { value: "work", label: "Work" },
  { value: "break_manual", label: "Break" },
  { value: "break_auto", label: "Auto break" },
];
const BREAK = "#4ea7fc";

/** "Mon 5" for a week, "5" for longer ranges with the month on the 1st. */
function label(day, n) {
  const d = dateOf(day);
  if (n <= 14) return `${["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"][d.getDay()]} ${d.getDate()}`;
  return d.getDate() === 1 ? formatDate(day, false) : String(d.getDate());
}

/** One day's segments, editable inline, with its target and note. */
class DayDetail {
  constructor(app, history, day) {
    this.app = app;
    this.history = history;
    this.day = day;
    this.detail = null;
    this.missing = false;
    this.target = "8h";
    this.note = new TextField({ multiline: true, rows: 2, placeholder: "Anything worth remembering about this day" });
    this.focus = null;
    this.allTasks = [];
    this.loaded = {};
  }

  get rollover() {
    return gwen.config?.tracking.day_rollover ?? "04:00";
  }

  sync(cx) {
    const v = gwen.versions;
    if (this.loaded.day !== v.days) {
      this.loaded.day = v.days;
      this.load(cx);
    }
    if (this.loaded.tasks !== v.tasks) {
      this.loaded.tasks = v.tasks;
      // Every task, done ones too, so past time shows what it was spent on.
      cx.spawn(async (cx) => {
        this.allTasks = (await api.ListTasks({ status: "all" }).catch(() => null))?.tasks ?? [];
        cx.notify();
      });
    }
  }

  load(cx) {
    cx.spawn(async (cx) => {
      try {
        const x = await api.GetDay(this.day);
        const first = !this.detail;
        this.detail = x;
        this.missing = false;
        if (first) {
          this.target = goDuration(x.work_day.target_seconds * 1000);
          this.note.set(x.work_day.note ?? "");
        }
      } catch (e) {
        if (apiError(e).code === "not_found") this.missing = true;
        else gwen.fail(e);
      }
      cx.notify();
    });
  }

  patch(s, fields, cx) {
    this.app.spawn(cx, async (cx) => {
      await gwen.act(() => api.PatchSegment(s.id, { ...fields, rev: s.rev }));
      this.load(cx);
    });
  }

  remove(s, cx) {
    this.app.spawn(cx, async (cx) => {
      const ok = await this.app.ui.confirm(
        { title: "Delete this time?", body: `${kinds.find((k) => k.value === s.kind)?.label ?? s.kind} from ${formatTime(s.started_at)} to ${formatTime(s.ended_at)} is removed from the day.`, confirm: "Delete", danger: true },
        cx,
      );
      if (ok && (await gwen.run(() => api.DeleteSegment(s.id)))) this.load(cx);
    });
  }

  saveDay(cx) {
    const d = this.detail;
    this.app.spawn(cx, async (cx) => {
      const ok = await gwen.act(() => api.PatchDay(this.day, { target_seconds: Math.round(parseDuration(this.target) / 1000), note: this.note.value, rev: d.work_day.rev }));
      if (ok) gwen.toast("Saved the day's target and note", "success");
      this.detail = null;
      this.load(cx);
    });
  }

  split(s, cx) {
    const ui = this.app.ui;
    let at = clockValue((s.started_at + (s.ended_at ?? s.started_at)) / 2);
    const id = ui.openDialog(
      {
        title: "Split in two",
        width: 400,
        render: (ui) =>
          v_flex()
            .gap(14)
            .child(ui.muted(`${formatTime(s.started_at)} to ${formatTime(s.ended_at)} becomes two pieces you can label separately.`, { size: 13 }))
            .child(ui.field("Split at", ui.timeInput("hd-split-at", at, (v) => (at = v)))),
        actions: (ui) => [
          ui.button("hd-split-cancel", "Cancel", (e, cx) => ui.closeDialog(cx, id), { tone: "ghost" }),
          ui.button(
            "hd-split-ok",
            "Split",
            (e, cx) =>
              this.app.spawn(cx, async (cx) => {
                if (await gwen.act(() => api.SplitSegment(s.id, { at: instantOn(this.day, at, this.rollover) }))) {
                  ui.closeDialog(cx, id);
                  this.load(cx);
                }
              }),
            { tone: "primary", icon: "scissors" },
          ),
        ],
      },
      cx,
    );
  }

  add(cx) {
    const ui = this.app.ui;
    const day = this.day;
    const f = { kind: "work", start: "09:00", end: "10:00", project: "", saving: false };
    const length = () => instantOn(day, f.end, this.rollover) - instantOn(day, f.start, this.rollover);
    const set = (k, v, cx) => {
      f[k] = v;
      cx.notify();
    };
    const two = (a, b) => h_flex().w_full().items_start().gap(14).child(div().flex_1().min_w(0).child(a)).child(div().flex_1().min_w(0).child(b));
    const id = ui.openDialog(
      {
        title: "Add time",
        width: 520,
        render: (ui) =>
          v_flex()
            .gap(14)
            .child(ui.muted(`Record time on ${formatLongDate(day)} that Gwen did not see, such as work away from the computer.`, { size: 13 }))
            .child(
              two(
                ui.field("Kind", ui.select("hd-add-kind", kinds, f.kind, (v, cx) => set("kind", v, cx))),
                ui.field("Project", ui.select("hd-add-project", [{ value: "", label: "Unassigned" }, ...gwen.projects.map((p) => ({ value: p.id, label: p.name, dot: p.color }))], f.project, (v, cx) => set("project", v, cx), { disabled: f.kind !== "work" })),
              ),
            )
            .child(two(ui.field("From", ui.timeInput("hd-add-from", f.start, (v, cx) => set("start", v, cx))), ui.field("To", ui.timeInput("hd-add-to", f.end, (v, cx) => set("end", v, cx)), length() <= 0 ? "The end is before the start." : undefined))),
        actions: (ui) => [
          ui.button("hd-add-cancel", "Cancel", (e, cx) => ui.closeDialog(cx, id), { tone: "ghost" }),
          ui.button(
            "hd-add-ok",
            `Add ${length() > 0 ? formatDuration(length()) : "time"}`,
            (e, cx) => {
              f.saving = true;
              this.app.spawn(cx, async (cx) => {
                const ok = await gwen.act(() =>
                  api.CreateSegment({ day, kind: f.kind, project_id: f.kind === "work" && f.project ? f.project : null, task_id: null, started_at: instantOn(day, f.start, this.rollover), ended_at: instantOn(day, f.end, this.rollover) }),
                );
                f.saving = false;
                if (ok) {
                  ui.closeDialog(cx, id);
                  this.load(cx);
                }
              });
            },
            { tone: "primary", busy: f.saving, disabled: length() <= 0 },
          ),
        ],
      },
      cx,
    );
  }

  render(ui) {
    const t = ui.t;
    const day = this.day;
    const header = h_flex()
      .w_full()
      .items_center()
      .gap(14)
      .child(ui.button("hd-back", "History", (e, cx) => this.history.close(cx), { tone: "ghost", icon: "arrow-left" }))
      .child(div().text_size(19).font_semibold().child(formatLongDate(day)))
      .child(div().flex_1())
      .child(ui.button("hd-add", "Add time", (e, cx) => this.add(cx), { icon: "plus" }));
    if (this.missing && !this.detail) return ui.scroll("hd-scroll", [header, ui.muted("Nothing was tracked on this day. Add time to record work you did away from the computer.", { size: 13 })]);
    if (!this.detail) return ui.scroll("hd-scroll", [header, ui.splash("Loading…", { busy: true })]);

    const detail = this.detail;
    const cfg = gwen.config;
    const colors = new Map([[null, unassignedColor]]);
    const names = new Map([[null, "Unassigned"]]);
    for (const p of gwen.projects) {
      colors.set(p.id, p.color);
      names.set(p.id, p.name);
    }
    for (const p of detail.summary.by_project) {
      colors.set(p.project_id ?? null, p.color);
      names.set(p.project_id ?? null, p.name);
    }
    const live = detail.segments.some((s) => s.ended_at == null);
    const s = detail.summary;
    const dayTarget = detail.work_day.target_seconds * 1000;
    const figure = (label, value, color) => v_flex().flex_1().gap(2).child(ui.dim(label)).child(div().text_size(20).font_semibold().text_color(color ?? t.foreground).child(value));

    const card = ui
      .panel({ pad: 20, gap: 16 })
      .child(
        h_flex()
          .gap(16)
          .child(figure("Worked", formatDuration(s.worked_ms)))
          .child(figure("Break", formatDuration(s.break_ms)))
          .child(figure("Target", formatDuration(dayTarget)))
          .child(figure(s.target_met ? "Target met" : "Short of target", `${dayTarget > 0 ? Math.round((s.worked_ms / dayTarget) * 100) : 0}%`, s.target_met ? ui.tone("ok") : undefined)),
      )
      .child(
        div()
          .pt(6)
          .border_t(1)
          .border_color(t.border)
          .child(
            timeline(ui, {
              id: "hd-tl",
              segments: detail.segments,
              colors,
              names,
              now: gwen.now(),
              live,
              from: instantOn(day, cfg?.planner.day_start ?? "09:00", this.rollover),
              to: instantOn(day, cfg?.planner.day_end ?? "23:00", this.rollover),
              onSegment: (seg, cx) => {
                this.focus = seg.id;
                cx.notify();
              },
            }),
          ),
      );

    const time = ui.panel({ title: "Time", pad: 12 });
    const rows = v_flex().gap(4);
    for (const seg of detail.segments) {
      const open = seg.ended_at == null;
      const locked = open;
      const work = seg.kind === "work";
      const color = work ? (colors.get(seg.project_id ?? null) ?? unassignedColor) : BREAK;
      const id = `hd-seg-${seg.id}`;
      const row = h_flex()
        .flex_wrap()
        .items_center()
        .gap(8)
        .pl(10)
        .pr(6)
        .py(7)
        .rounded(8)
        .when(this.focus === seg.id, (e) => e.bg(alpha(t.accent, 0.1)))
        .child(div().w(4).h(30).flex_none().rounded(2).bg(work ? color : alpha(BREAK, 0.5)))
        .child(ui.select(`${id}-kind`, kinds, seg.kind, (v, cx) => this.patch(seg, { kind: v }, cx), { width: 130, disabled: locked }));
      if (work) {
        const projects = [{ value: "", label: "Unassigned", dot: unassignedColor }, ...gwen.projects.map((p) => ({ value: p.id, label: p.name, dot: p.color }))];
        if (seg.project_id && !gwen.projects.some((p) => p.id === seg.project_id)) projects.push({ value: seg.project_id, label: names.get(seg.project_id) ?? "A deleted project", dot: color });
        const tasks = [{ value: "", label: "No task" }, ...gwen.tasks.map((x) => ({ value: x.id, label: x.title }))];
        if (seg.task_id && !gwen.tasks.some((x) => x.id === seg.task_id)) tasks.push({ value: seg.task_id, label: this.allTasks.find((x) => x.id === seg.task_id)?.title ?? "A deleted task" });
        row
          .child(ui.select(`${id}-project`, projects, seg.project_id ?? "", (v, cx) => this.patch(seg, { project_id: v || null }, cx), { width: 160, disabled: locked }))
          .child(div().flex_1().min_w(150).child(ui.select(`${id}-task`, tasks, seg.task_id ?? "", (v, cx) => this.patch(seg, { task_id: v || null }, cx), { disabled: locked, menuWidth: 340 })));
      } else row.child(div().flex_1());
      row.child(
        h_flex()
          .items_center()
          .gap(6)
          .child(locked ? ui.text(formatTime(seg.started_at), { size: 13 }) : ui.timeInput(`${id}-start`, clockValue(seg.started_at), (v, cx) => v && this.patch(seg, { started_at: instantOn(day, v, this.rollover) }, cx)))
          .child(ui.dim("–"))
          .child(
            open
              ? div().w(70).flex().justify_center().text_size(13).font_medium().text_color(ui.tone("ok")).child("now")
              : ui.timeInput(`${id}-end`, clockValue(seg.ended_at), (v, cx) => v && this.patch(seg, { ended_at: instantOn(day, v, this.rollover) }, cx)),
          ),
      );
      row
        .child(div().w(60).flex_none().flex().justify_end().child(ui.muted(formatDuration((seg.ended_at ?? gwen.now()) - seg.started_at), { size: 13 })))
        .child(ui.iconButton(`${id}-split`, "scissors", (e, cx) => this.split(seg, cx), { tip: "Split in two", disabled: locked }))
        .child(ui.iconButton(`${id}-del`, "trash-2", (e, cx) => this.remove(seg, cx), { tip: "Delete", disabled: locked }));
      rows.child(row);
    }
    time.child(rows);
    if (live) time.child(div().px(10).child(ui.dim("The running segment can be changed once it ends.")));

    const dayPanel = ui
      .panel({ title: "Day" })
      .child(
        h_flex()
          .items_start()
          .gap(18)
          .child(
            ui.field(
              "Target",
              ui.durationInput("hd-target", this.target, (v, cx) => {
                this.target = v;
                cx.notify();
              }),
            ),
          )
          .child(div().flex_1().min_w(0).child(ui.field("Note", this.note.el(ui, { h: 60 })))),
      )
      .child(h_flex().justify_end().child(ui.button("hd-save", "Save day", (e, cx) => this.saveDay(cx), { tone: "primary" })));

    return ui.scroll("hd-scroll", [header, card, time, dayPanel]);
  }
}

export default class History {
  constructor(app) {
    this.app = app;
    this.range = "7";
    this.from = null;
    this.to = null;
    this.days = null;
    this.loaded = null;
    this.open = null; // the day shown in detail
  }

  get tick() {
    return Boolean(this.open?.detail?.segments?.some((s) => s.ended_at == null));
  }

  sync(cx) {
    const today = gwen.today();
    if (!this.from) {
      this.from = addDays(today, -6);
      this.to = today;
    }
    if (this.open) {
      this.open.sync(cx);
      return;
    }
    const stamp = `${this.from}/${this.to}/${gwen.versions.days}`;
    if (this.loaded === stamp) return;
    this.loaded = stamp;
    const from = this.from;
    const to = this.to;
    cx.spawn(async (cx) => {
      try {
        const l = await api.ListDays(from, to);
        if (this.from === from && this.to === to) this.days = l.days ?? [];
      } catch (e) {
        gwen.fail(e);
      }
      cx.notify();
    });
  }

  openDay(day, cx) {
    this.open = new DayDetail(this.app, this, day);
    this.open.sync(cx);
    cx.notify();
  }

  close(cx) {
    this.open = null;
    this.loaded = null;
    this.sync(cx);
    cx.notify();
  }

  pick(r, cx) {
    this.range = r;
    if (r !== "custom") {
      const today = gwen.today();
      this.to = today;
      this.from = addDays(today, -(Number(r) - 1));
    }
    this.sync(cx);
    cx.notify();
  }

  render(ui) {
    if (this.open) return this.open.render(ui);
    const t = ui.t;
    const today = gwen.today();
    const target = parseDuration(gwen.config?.tracking.daily_target ?? "8h");
    const byDay = new Map((this.days ?? []).map((x) => [x.day, x]));
    // Every day of the range, so untracked days show as gaps rather than vanish.
    const all = [];
    for (let day = this.from; day && day <= this.to && all.length < 400; day = addDays(day, 1)) all.push(byDay.get(day) ?? { day, worked_ms: 0, break_ms: 0, target_met: false, by_project: [], target_seconds: 0 });
    const tracked = all.filter((x) => x.worked_ms > 0);
    const total = tracked.reduce((sum, x) => sum + x.worked_ms, 0);
    const scale = Math.max(target, ...all.map((x) => x.worked_ms), 1);

    const actions = h_flex()
      .items_center()
      .gap(10)
      .child(
        ui.segmented(
          "hi-range",
          [
            { value: "7", label: "7 days" },
            { value: "14", label: "14 days" },
            { value: "30", label: "30 days" },
            { value: "custom", label: "Custom" },
          ],
          this.range,
          (r, cx) => this.pick(r, cx),
        ),
      );
    if (this.range === "custom")
      actions
        .child(
          ui.datePicker("hi-from", this.from, (d, cx) => {
            if (!d) return;
            this.from = d > this.to ? this.to : d;
            this.sync(cx);
          }, { width: 150 }),
        )
        .child(ui.muted("to", { size: 13 }))
        .child(
          ui.datePicker("hi-to", this.to, (d, cx) => {
            if (!d) return;
            this.to = d < this.from ? this.from : d;
            this.sync(cx);
          }, { width: 150, min: this.from }),
        );
    const header = ui.pageHeader(
      "History",
      tracked.length ? `${formatDuration(total)} over ${tracked.length} ${tracked.length === 1 ? "day" : "days"}, ${all.filter((x) => x.target_met).length} on target` : "Every day you tracked, by project.",
      actions,
    );

    // The chart: one stacked bar a day, with the target as a line. A click opens the day.
    const chart = ui.panel({});
    if (this.days && tracked.length === 0) chart.child(ui.empty({ icon: "history", title: "Nothing tracked in this range", body: "Clock in on Today, or add time by hand from a day below." }));
    else {
      const height = 220;
      const peak = Math.max(scale, 1);
      const targetLine = target > 0 ? Math.round((target / peak) * height) : null;
      const bars = h_flex()
        .relative()
        .w_full()
        .h(height)
        .items_end()
        .gap(all.length > 20 ? 2 : 6)
        .children(
          all.map((x) =>
            v_flex()
              .id(`hi-bar-${x.day}`)
              .flex_1()
              .min_w(0)
              .h_full()
              .justify_end()
              .cursor_pointer()
              .rounded(3)
              .hover((s) => s.bg(t.hover))
              .tooltip(`${formatDate(x.day)}: ${x.worked_ms > 0 ? formatDuration(x.worked_ms) : "nothing tracked"}`)
              .on_click((e, cx) => this.openDay(x.day, cx))
              .children([...x.by_project].reverse().map((p) => div().w_full().h(Math.max(1, Math.round((p.worked_ms / peak) * height))).bg(p.color))),
          ),
        );
      if (targetLine !== null) bars.child(div().absolute().left(0).right(0).bottom(targetLine).h(1).bg(alpha(t.foreground, 0.35)));
      chart
        .child(
          h_flex()
            .items_center()
            .gap(10)
            .child(ui.dim(`Up to ${formatDuration(peak)} a day`))
            .child(div().flex_1())
            .when(target > 0, (e) => e.child(h_flex().items_center().gap(6).child(div().w(16).h(1).bg(alpha(t.foreground, 0.35))).child(ui.dim(`Target ${formatDuration(target)}`)))),
        )
        .child(bars)
        .child(
          h_flex()
            .w_full()
            .gap(all.length > 20 ? 2 : 6)
            .children(all.map((x, i) => div().flex_1().min_w(0).flex().justify_center().text_size(10.5).text_color(t.dim).truncate().child(all.length <= 14 || i % Math.ceil(all.length / 10) === 0 || dateOf(x.day).getDate() === 1 ? label(x.day, all.length) : ""))),
        );
    }

    const daysPanel = ui.panel({ title: "Days", pad: 12 }).child(
      v_flex().children(
        [...all].reverse().map((x) =>
          h_flex()
            .id(`hi-day-${x.day}`)
            .items_center()
            .gap(16)
            .px(10)
            .py(9)
            .rounded(8)
            .cursor_pointer()
            .hover((s) => s.bg(t.hover))
            .on_click((e, cx) => this.openDay(x.day, cx))
            .child(
              v_flex()
                .w(110)
                .flex_none()
                .child(div().text_size(13).child(formatDate(x.day)))
                .when(x.day === today, (e) => e.child(ui.text("Today", { size: 12, color: t.accent }))),
            )
            .child(
              div()
                .relative()
                .flex_1()
                .h(8)
                .rounded(4)
                .bg(t.hover)
                .overflow_hidden()
                .child(
                  h_flex()
                    .absolute()
                    .top(0)
                    .bottom(0)
                    .left(0)
                    .w(`${Math.round((x.worked_ms / scale) * 1000) / 10}%`)
                    .children(x.by_project.map((p) => div().h_full().w(`${Math.round((p.worked_ms / Math.max(x.worked_ms, 1)) * 1000) / 10}%`).bg(p.color))),
                )
                .when(target > 0, (e) => e.child(div().absolute().top(0).bottom(0).w(1).left(`${Math.round((target / scale) * 1000) / 10}%`).bg(t.dim))),
            )
            .child(div().w(70).flex_none().flex().justify_end().text_size(13).font_medium().child(x.worked_ms > 0 ? formatDuration(x.worked_ms) : "—"))
            .child(div().w(90).flex_none().flex().justify_end().child(ui.dim(x.break_ms > 0 ? `${formatDuration(x.break_ms)} break` : "")))
            .child(div().w(18).flex_none().when(x.target_met, (e) => e.child(ui.icon("circle-check", 16, ui.tone("ok")))))
            .child(ui.icon("chevron-right", 16, t.dim)),
        ),
      ),
    );
    return ui.scroll("hi-scroll", [header, chart, daysPanel]);
  }
}
