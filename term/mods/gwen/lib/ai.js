// What every screen that asks the AI shares: the provider's name, the
// progress and failure notices, and the week helper retros use.

import { div } from "gpui-kit";
import { v_flex, h_flex } from "gpui-base";
import { gwen } from "./gwen.js";
import { addDays, formatDuration, parseDuration } from "./format.js";
import { alpha } from "./kit.js";

/** The provider as a person would name it. */
export function providerName(llm) {
  switch (llm?.provider) {
    case "claude_code":
      return "Claude Code";
    case "anthropic":
      return "the Anthropic API";
    case "openai_compatible":
      return llm.model || "your model";
    default:
      return "no provider";
  }
}

/** A Claude subscription as its name is written: "max" is the Max plan. */
export const planName = (plan) => (plan ? plan[0].toUpperCase() + plan.slice(1) : "");

/** The daemon's messages end with a command-line hint; the screens have their own way there. */
export function withoutCliHint(message) {
  return String(message)
    .replace(/\s*[,;]\s*(?:or give its path with|run)\s+gwen setup.*$/i, "")
    .replace(/\s*\(run gwen setup.*\)$/i, "");
}

/**
 * What a request is doing while the provider thinks, with how long it has
 * taken. since: Date.now() when it started. The screen showing one sets its
 * `tick` true meanwhile, so the seconds move.
 */
export function aiProgress(ui, since, doing) {
  const t = ui.t;
  const secs = Math.max(0, Math.floor((Date.now() - since) / 1000));
  const limit = parseDuration(gwen.config?.llm?.timeout ?? "2m");
  return v_flex()
    .w_full()
    .gap(8)
    .p(14)
    .rounded(8)
    .border_1()
    .border_color(alpha(t.accent, 0.3))
    .bg(alpha(t.accent, 0.06))
    .child(
      h_flex()
        .items_center()
        .gap(10)
        .child(ui.face("thinking", 34))
        .child(div().flex_1().min_w(0).text_size(13).text_color(t.muted).child(`${doing} with ${providerName(gwen.config?.llm)}`))
        .child(ui.spinner(13, t.accent))
        .child(div().flex_none().text_size(12.5).text_color(t.dim).child(`${secs}s`)),
    )
    .when(limit > 0, (e) => e.child(ui.dim(`It gives up after ${formatDuration(limit)}.`)));
}

/** The provider is not set up: say so and go there. */
export function aiUnavailable(ui, app, message, id = "ai-setup") {
  return ui.callout({
    tone: "warn",
    icon: "sparkles",
    text: `The assistant can't answer yet: ${withoutCliHint(message)}.`,
    action: ui.button(id, "Set up AI", () => app.go("settings", "ai"), { small: true }),
  });
}

/** A failed run: the provider's reason, and the setting that usually fixes it. */
export function aiFailed(ui, app, error, id = "ai-failed") {
  const timedOut = /timed out/i.test(error);
  return ui.callout({
    tone: "danger",
    icon: "circle-alert",
    text: withoutCliHint(error),
    action: ui.button(id, timedOut ? "Allow more time" : "Check AI settings", () => app.go("settings", "ai"), { small: true }),
  });
}

/** The Monday on or before day. */
export function mondayOf(day) {
  const [y, m, d] = day.split("-").map(Number);
  return addDays(day, -((new Date(y, m - 1, d).getDay() + 6) % 7));
}
