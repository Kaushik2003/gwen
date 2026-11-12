import { div } from "gpui-kit";
import { v_flex, h_flex } from "gpui-base";
import { themes, settings, run } from "herdr";
import { alpha } from "../../lib/kit.js";

const palettes = [
  { name: "ghost", label: "Ghost", blurb: "Gwen's own colours: obsidian black, a web-slinger's crimson." },
  { name: "tide", label: "Tide", blurb: "The same shapes, cooler and quieter." },
  { name: "ember", label: "Ember", blurb: "Warmer graphite, an apricot accent." },
];

export default class Appearance {
  render(ui) {
    const t = ui.t;
    const look = settings();
    const available = themes();
    const cards = palettes.map(({ name, label, blurb }) => {
      const palette = available.find((theme) => theme.name === name);
      const colors = palette?.colors ?? t;
      const on = name === (palette?.light ? look.theme_light : look.theme_dark);
      return v_flex()
        .id(`appearance-${name}`)
        .role("button")
        .accessibility_label(`${label}${on ? ", selected" : ""}. ${blurb}`)
        .flex_1()
        .min_w(0)
        .gap(12)
        .p(14)
        .rounded(8)
        .border_1()
        .border_color(on ? t.accent : t.input)
        .bg(on ? alpha(t.accent, 0.1) : t.raised)
        .cursor_pointer()
        .hover((style) => style.border_color(t.accent))
        .on_click((event, cx) => {
          run("app.theme", { name });
          cx.notify();
        })
        .child(v_flex().w_full().h(52).p(10).gap(8).rounded(6).bg(colors.background)
          .child(div().h(4).w(48).rounded(2).bg(colors.foreground))
          .child(h_flex().gap(5).children([colors.accent, colors.ansi1, colors.ansi2, colors.ansi3, colors.ansi5].filter(Boolean).map((color) => ui.dot(color, 9)))),
        )
        .child(h_flex().items_center().gap(8)
          .child(ui.text(label, { size: 13.5, medium: true }).flex_1().min_w(0))
          .when(on, (el) => el.child(ui.icon("check", 14, t.accent))),
        )
        .child(ui.dim(blurb));
    });
    return ui
      .panel({ title: "Appearance", icon: "palette" })
      .child(h_flex().w_full().items_stretch().gap(10).children(cards))
      .child(v_flex().pt(4).gap(10)
        .child(ui.muted("The window's own themes and fonts are in its settings."))
        .child(h_flex().child(ui.button("appearance-settings", "More themes and fonts", (event, cx) => {
          run("app.settings", { section: "look" });
          cx.notify();
        }, { icon: "settings" }))),
      );
  }
}
