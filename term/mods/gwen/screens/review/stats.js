// Stats: totals and streaks over a range, time by project, the weekly retro,
// and a year of days as a heatmap against the daily target.

import { div } from "gpui-kit";
import { v_flex, h_flex } from "gpui-base";
import { api, gwen } from "../../lib/gwen.js";
import { addDays, formatDate, formatDay, formatDuration, parseDuration } from "../../lib/format.js";
import { Retro } from "./retro.js";

const plural = (n, unit) => `${n} ${n === 1 ? unit : unit + "s"}`;
const monthNames = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
const WORKING = "#27a644";

export default class Stats {
  constructor(app) {
    this.app = app;
    this.range = "week";
    this.from = null;
    this.to = null;
    this.summary = null;
    this.year = null;
    this.heat = null;
    this.loaded = {};
    this.retro = new Retro(app, "st-retro");
  }

  get tick() {
    return this.retro.tick;
  }

  sync(cx) {
    const today = gwen.today();
    if (!this.from) {
      this.from = addDays(today, -6);
      this.to = today;
      this.year = Number(today.slice(0, 4));
    }
    const v = gwen.versions.days;
    const from = this.from;
    const to = this.to;
    const year = this.year;
    if (this.loaded.summary !== `${from}/${to}/${v}`) {
      this.loaded.summary = `${from}/${to}/${v}`;
      cx.spawn(async (cx) => {
        try {
          const s = await api.StatsSummary(from, to);
          if (this.from === from && this.to === to) this.summary = s;
        } catch (e) {
          gwen.fail(e);
        }
        cx.notify();
      });
    }
    if (this.loaded.heat !== `${year}/${v}`) {
      this.loaded.heat = `${year}/${v}`;
      cx.spawn(async (cx) => {
        try {
          const h = await api.StatsHeatmap(year);
          if (this.year === year) this.heat = h;
        } catch (e) {
          gwen.fail(e);
        }
        cx.notify();
      });
    }
  }

  pick(r, cx) {
    this.range = r;
    if (r !== "custom") {
      const today = gwen.today();
      this.to = today;
      this.from = addDays(today, r === "week" ? -6 : r === "month" ? -29 : -364);
    }
    this.sync(cx);
    cx.notify();
  }

  /** A year of days as a Monday-first grid, shaded by worked time against the daily target. */
  heatmap(ui, heatmap, targetMs, today) {
    const t = ui.t;
    const worked = new Map(heatmap.days.map((d) => [d.day, d.worked_ms]));
    const jan1 = new Date(heatmap.year, 0, 1);
    const start = new Date(heatmap.year, 0, 1 - ((jan1.getDay() + 6) % 7));
    const weeks = [];
    for (let i = 0; ; i++) {
      const d = new Date(start.getFullYear(), start.getMonth(), start.getDate() + i);
      if (d.getFullYear() > heatmap.year && d.getDay() === 1) break;
      const w = Math.floor(i / 7);
      weeks[w] ??= [];
      weeks[w].push(d.getFullYear() === heatmap.year ? { day: formatDay(d), ms: worked.get(formatDay(d)) ?? 0, month: d.getMonth(), date: d.getDate() } : null);
    }
    const level = (ms) => (ms <= 0 ? 0 : targetMs <= 0 ? 1 : Math.min(4, Math.max(1, Math.ceil((ms / targetMs) * 4))));
    const shade = (lv) => (lv === 0 ? t.hover : `${WORKING}${Math.round((0.22 + 0.195 * lv) * 255).toString(16).padStart(2, "0")}`);
    const size = 11;
    const gap = 3;
    const tracked = heatmap.days.filter((d) => d.worked_ms > 0).length;
    const labels = v_flex()
      .flex_none()
      .w(28)
      .gap(gap)
      .pt(16)
      .children(["Mon", "", "Wed", "", "Fri", "", ""].map((l) => div().h(size).text_size(9).line_height(`${size}px`).text_color(t.dim).child(l)));
    const columns = h_flex()
      .gap(gap)
      .children(
        weeks.map((cells) => {
          const monthStart = cells.find((c) => c && c.date === 1);
          return v_flex()
            .gap(gap)
            .child(div().h(13).w(size).text_size(10).text_color(t.dim).whitespace_nowrap().child(monthStart ? monthNames[monthStart.month] : ""))
            .children(
              cells.map((c) =>
                c
                  ? div()
                      .id(`hm-${c.day}`)
                      .w(size)
                      .h(size)
                      .rounded(2)
                      .bg(shade(level(c.ms)))
                      .when(c.day === today, (e) => e.border_1().border_color(t.foreground))
                      .tooltip(`${formatDate(c.day)}: ${c.ms > 0 ? formatDuration(c.ms) : "nothing tracked"}`)
                  : div().w(size).h(size),
              ),
            );
        }),
      );
    return v_flex()
      .gap(12)
      .child(div().id("hm-scroll").w_full().overflow_x_scrollbar().child(h_flex().child(labels).child(columns)))
      .child(
        h_flex()
          .justify_end()
          .items_center()
          .gap(5)
          .child(ui.dim(`${tracked} days tracked · Less`, { size: 11 }))
          .children([0, 1, 2, 3, 4].map((lv) => div().w(11).h(11).rounded(2).bg(shade(lv))))
          .child(ui.dim("More, against the daily target", { size: 11 })),
      );
  }

  render(ui) {
    const t = ui.t;
    const today = gwen.today();
    const thisYear = Number(today.slice(0, 4));
    const target = parseDuration(gwen.config?.tracking.daily_target ?? "8h");
    const s = this.summary;

    const actions = h_flex()
      .items_center()
      .gap(10)
      .child(
        ui.segmented(
          "st-range",
          [
            { value: "week", label: "Week" },
            { value: "month", label: "Month" },
            { value: "year", label: "Year" },
            { value: "custom", label: "Custom" },
          ],
          this.range,
          (r, cx) => this.pick(r, cx),
        ),
      );
    if (this.range === "custom")
      actions
        .child(
          ui.datePicker("st-from", this.from, (d, cx) => {
            if (!d) return;
            this.from = d > this.to ? this.to : d;
            this.sync(cx);
          }, { width: 150 }),
        )
        .child(ui.muted("to", { size: 13 }))
        .child(
          ui.datePicker("st-to", this.to, (d, cx) => {
            if (!d) return;
            this.to = d < this.from ? this.from : d;
            this.sync(cx);
          }, { width: 150, min: this.from }),
        );
    const parts = [ui.pageHeader("Stats", null, actions)];

    if (s) {
      const kpi = (icon, label, value, iconColor, extra) =>
        v_flex()
          .flex_1()
          .min_w(0)
          .gap(6)
          .px(18)
          .py(14)
          .rounded(10)
          .border_1()
          .border_color(t.border)
          .bg(t.surface)
          .child(h_flex().items_center().gap(6).child(ui.icon(icon, 13, iconColor)).child(ui.dim(label)))
          .child(div().text_size(20).font_semibold().child(value))
          .when(Boolean(extra), (e) => e.child(extra));
      parts.push(
        h_flex()
          .w_full()
          .items_start()
          .gap(12)
          .child(kpi("clock", "Worked", formatDuration(s.worked_ms)))
          .child(kpi("calendar-days", "Daily average", formatDuration(s.avg_worked_ms)))
          .child(kpi("target", "Days on target", `${s.days_target_met} of ${s.days_tracked}`, undefined, ui.meter(s.days_tracked ? s.days_target_met / s.days_tracked : 0, { color: WORKING, height: 4 })))
          .child(kpi("flame", "Current streak", plural(s.current_streak, "day"), s.current_streak > 0 ? ui.tone("warn") : undefined))
          .child(kpi("trophy", "Longest streak", plural(s.longest_streak, "day"))),
      );

      const byProject = ui.panel({ title: "By project", icon: "chart-pie" });
      if (s.by_project.length === 0) byProject.child(ui.muted("No work in this range.", { size: 13 }));
      else {
        // One bar split by project, in place of the old ring.
        byProject.child(
          v_flex()
            .gap(6)
            .child(h_flex().items_baseline().gap(8).child(div().text_size(20).font_semibold().child(formatDuration(s.worked_ms))).child(ui.dim("worked")))
            .child(
              h_flex()
                .w_full()
                .h(14)
                .rounded(7)
                .overflow_hidden()
                .gap(2)
                .children(s.by_project.map((p) => div().id(`st-bar-${p.project_id ?? "none"}`).h_full().w(`${Math.max(0.5, (p.worked_ms / Math.max(s.worked_ms, 1)) * 100)}%`).bg(p.color).tooltip(`${p.name}: ${formatDuration(p.worked_ms)}`))),
            ),
        );
        byProject.child(
          v_flex()
            .gap(12)
            .children(
              s.by_project.map((p) => {
                const share = s.worked_ms > 0 ? p.worked_ms / s.worked_ms : 0;
                return v_flex()
                  .gap(6)
                  .child(
                    h_flex()
                      .items_center()
                      .gap(10)
                      .child(ui.dot(p.color, 8))
                      .child(div().flex_1().min_w(0).truncate().text_size(13).text_color(t.muted).child(p.name))
                      .child(div().text_size(13).font_medium().child(formatDuration(p.worked_ms)))
                      .child(div().w(40).flex().justify_end().child(ui.dim(`${Math.round(share * 100)}%`))),
                  )
                  .child(ui.meter(share, { color: p.color, height: 4 }));
              }),
            ),
        );
      }
      parts.push(byProject);
    }

    if (this.range === "week") parts.push(ui.panel({ title: "Weekly retro", icon: "scroll-text" }).child(this.retro.render(ui)));

    const heat = ui.panel({
      title: String(this.year ?? thisYear),
      icon: "calendar-days",
      actions: ui.segmented(
        "st-year",
        [3, 2, 1, 0].map((i) => ({ value: thisYear - i, label: String(thisYear - i) })),
        this.year ?? thisYear,
        (y, cx) => {
          this.year = y;
          this.heat = null;
          this.sync(cx);
          cx.notify();
        },
        { small: true },
      ),
    });
    if (this.heat) heat.child(this.heatmap(ui, this.heat, target, today));
    else heat.child(ui.muted("Loading…", { size: 13 }));
    parts.push(heat);
    return ui.scroll("st-scroll", parts);
  }
}
