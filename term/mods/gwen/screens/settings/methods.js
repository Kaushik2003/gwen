import { div } from "gpui-kit";
import { v_flex, h_flex } from "gpui-base";
import { api, gwen } from "../../lib/gwen.js";
import { formatClock, formatHHMM, formatTime } from "../../lib/format.js";
import { alpha } from "../../lib/kit.js";
import { patchConfig } from "./form.js";

const energyLevels = [
  { level: 1, label: "Drained" },
  { level: 2, label: "Low" },
  { level: 3, label: "Okay" },
  { level: 4, label: "Good" },
  { level: 5, label: "Peak" },
];

export default class Methods {
  constructor(app) {
    this.app = app;
    this.start = gwen.config?.planner?.prime_start || "09:00";
    this.end = gwen.config?.planner?.prime_end || "12:00";
    this.report = null;
    this.reportRequest = 0;
    this.loaded = {};
    this.saving = false;
    this.logging = null;
  }

  sync(cx) {
    const planner = gwen.config?.planner;
    if (!planner) return;
    if (this.loaded.start !== planner.prime_start) {
      this.loaded.start = planner.prime_start;
      if (planner.prime_start) this.start = planner.prime_start;
    }
    if (this.loaded.end !== planner.prime_end) {
      this.loaded.end = planner.prime_end;
      if (planner.prime_end) this.end = planner.prime_end;
    }
    const stamp = `${gwen.today()}/${gwen.versions.days}`;
    if (this.loaded.report !== stamp) {
      this.loaded.report = stamp;
      this.app.spawn(cx, async () => this.loadReport());
    }
  }

  async loadReport() {
    const request = ++this.reportRequest;
    const report = await api.EnergyReport(0);
    if (request === this.reportRequest) this.report = report;
  }

  save(patch, said, cx) {
    if (this.saving) return;
    this.saving = true;
    this.app.spawn(cx, async () => {
      try {
        await patchConfig({ planner: patch });
        gwen.toast(said, "success");
      } finally {
        this.saving = false;
      }
    });
  }

  log(level, cx) {
    if (this.logging !== null) return;
    this.logging = level;
    this.app.spawn(cx, async () => {
      try {
        await api.LogEnergy({ level, at: null });
        gwen.toast(`Logged: ${energyLevels[level - 1].label}`, "success");
        await this.loadReport();
      } finally {
        this.logging = null;
      }
    });
  }

  render(ui) {
    const planner = gwen.config?.planner ?? {};
    const prime = Boolean(planner.prime_start);
    const report = this.report;
    const frog = ui.panel({ title: "Eat the frog", icon: "flame" }).child(ui.toggleRow(
      "methods-frog",
      "Hardest work first",
      "Mark tasks Hard and Gwen plans them first thing, before the easier work, so the biggest task of the day is behind you early. The assistant does the same.",
      Boolean(planner.eat_the_frog),
      (on, cx) => {
        this.save({ eat_the_frog: on }, on ? "Frogs go first now" : "Plans go by urgency alone", cx);
        cx.notify();
      },
      { disabled: this.saving },
    ));
    const primePanel = ui
      .panel({ title: "Biological prime time", icon: "sunrise" })
      .child(ui.muted("Your energy rises and falls through the day. Set the hours you're sharpest, and Gwen puts hard work there and lighter work around it. Not sure when that is? Log your energy for a couple of weeks, here or from the tray, and Gwen will suggest it.", { size: 13, lh: 19 }))
      .child(ui.toggleRow(
        "methods-prime",
        "Plan around my prime time",
        prime ? `Hard work goes between ${formatHHMM(planner.prime_start)} and ${formatHHMM(planner.prime_end)}.` : "Off: hard work goes wherever it fits.",
        prime,
        (on, cx) => {
          this.save(on ? { prime_start: this.start, prime_end: this.end } : { prime_start: "", prime_end: "" }, on ? "Prime time on" : "Prime time off", cx);
          cx.notify();
        },
        { disabled: this.saving },
      ))
      .child(h_flex().flex_wrap().items_center().gap(8)
        .child(ui.muted("From"))
        .child(ui.timeInput("methods-prime-start", this.start, (value, cx) => {
          this.start = value;
          cx.notify();
        }).accessibility_label("Prime time starts"))
        .child(ui.muted("to"))
        .child(ui.timeInput("methods-prime-end", this.end, (value, cx) => {
          this.end = value;
          cx.notify();
        }).accessibility_label("Prime time ends"))
        .child(ui.button("methods-save-hours", prime ? "Save hours" : "Use these hours", (event, cx) => {
          this.save({ prime_start: this.start, prime_end: this.end }, `Prime time is ${formatHHMM(this.start)}–${formatHHMM(this.end)}`, cx);
          cx.notify();
        }, { small: true, tone: "primary", busy: this.saving, disabled: prime && this.start === planner.prime_start && this.end === planner.prime_end })),
      );
    const energy = v_flex().mt(10).pt(16).gap(12).border_t(1).border_color(ui.t.border)
      .child(h_flex().items_center().gap(8)
        .child(ui.icon("battery-charging", 16))
        .child(ui.text("How's your energy right now?", { medium: true })),
      )
      .child(h_flex().flex_wrap().gap(8).children(energyLevels.map(({ level, label }) =>
        ui.button(`methods-energy-${level}`, label, (event, cx) => {
          this.log(level, cx);
          cx.notify();
        }, { small: true, disabled: this.logging !== null, busy: this.logging === level }),
      )));
    if (report) energy.child(energyChart(ui, report));
    if (report?.suggested_start && report.suggested_end) {
      const applied = planner.prime_start === report.suggested_start && planner.prime_end === report.suggested_end;
      energy.child(ui.callout({
        tone: "accent",
        text: `Your energy peaks around ${formatHHMM(report.suggested_start)}–${formatHHMM(report.suggested_end)}.`,
        action: applied ? null : ui.button("methods-use-prime", "Use as prime time", (event, cx) => {
          this.save({ prime_start: report.suggested_start, prime_end: report.suggested_end }, "Prime time set from your check-ins", cx);
          cx.notify();
        }, { small: true, tone: "primary", disabled: this.saving }),
      }));
    }
    primePanel.child(energy);
    return [frog, primePanel];
  }
}

function energyChart(ui, report) {
  if (!report.hours.length) return ui.muted(`No check-ins in the last ${report.days} days yet. A few a day, at different times, are enough.`);
  const byHour = new Map(report.hours.map((hour) => [hour.hour, hour]));
  const first = Math.min(...report.hours.map((hour) => hour.hour));
  const lastHour = Math.max(...report.hours.map((hour) => hour.hour));
  const hours = Array.from({ length: lastHour - first + 1 }, (_, i) => first + i);
  const last = report.logs[report.logs.length - 1];
  const description = report.hours.map(({ hour, average, count }) => `${formatClock(hour * 60, true)}: ${average.toFixed(1)} of 5, ${count} check-ins`).join("; ");
  return v_flex().id("methods-energy-chart").role("image").accessibility_label(`Average energy by hour, last ${report.days} days. ${description}`).gap(6)
    .child(h_flex().flex_wrap().justify_between().items_baseline().gap(8)
      .child(ui.dim(`Average energy by hour, last ${report.days} days`))
      .child(ui.dim(`${report.logs.length} check-ins${last ? `, latest ${formatTime(last.at)}` : ""}`)),
    )
    .child(h_flex().h(128).w_full().items_end().gap(2).border_b(1).border_color(ui.t.border).children(hours.map((hour) => {
      const entry = byHour.get(hour);
      const inPrime = report.suggested_start && report.suggested_end && hour >= Number(report.suggested_start.slice(0, 2)) && hour < Number(report.suggested_end.slice(0, 2));
      return div()
        .id(`methods-energy-hour-${hour}`)
        .flex()
        .flex_1()
        .min_w(0)
        .h_full()
        .items_end()
        .tooltip(`${formatClock(hour * 60, true)} · ${entry ? `${entry.average.toFixed(1)} of 5 (${entry.count})` : "no check-ins"}`)
        .hover((style) => style.opacity(0.8))
        .when(Boolean(entry), (el) => el.child(div().w_full().h(`${entry.average / 5 * 100}%`).rounded(4).bg(inPrime ? ui.t.accent : alpha(ui.t.accent, 0.45))));
    })))
    .child(h_flex().gap(2).children(hours.map((hour) =>
      ui.dim(hour % 3 === 0 ? `${hour % 12 || 12}${hour < 12 ? "a" : "p"}` : "", { size: 10 }).flex_1().min_w(0).text_center(),
    )));
}
