// Settings › AI › Personality: who she is, how she has chosen to treat you,
// and what you tell her about yourself.

import { div } from "gpui-kit";
import { v_flex, h_flex } from "gpui-base";
import { api, gwen } from "../../lib/gwen.js";
import { formatAgo } from "../../lib/format.js";
import { attitudeOf } from "../../lib/llm.js";
import { TextField, alpha } from "../../lib/kit.js";

export default class Personality {
  constructor(app) {
    this.app = app;
    this.loadedFrom = null;
    this.self = null;
    this.saving = false;
    this.name = new TextField({ placeholder: "Gwen", onChange: (v, cx) => cx.notify() });
    this.instructions = new TextField({
      multiline: true,
      rows: 4,
      placeholder: "I'm a student; my exams are in December. Keep replies short. Push me on deep work, go easy on weekends.",
      onChange: (v, cx) => cx.notify(),
    });
  }

  sync(cx) {
    const llm = gwen.config?.llm;
    if (!llm) return;
    const stamp = JSON.stringify([llm.assistant_name, llm.instructions]);
    if (this.loadedFrom === stamp) return;
    const first = this.loadedFrom === null;
    this.loadedFrom = stamp;
    this.name.set(llm.assistant_name ?? "");
    this.instructions.set(llm.instructions ?? "");
    if (first)
      cx.spawn(async (cx) => {
        this.self = await api.AssistantSelf().catch(() => null);
        cx.notify();
      });
  }

  save(cx) {
    const name = this.name.value.trim();
    this.saving = true;
    this.app.spawn(cx, async () => {
      const ok = await gwen.act(() => api.PatchConfig({ llm: { assistant_name: name, instructions: this.instructions.value.trim().slice(0, 2000) } }));
      this.saving = false;
      if (ok) gwen.toast(`${name} is ready`, "success");
    });
  }

  reset(cx) {
    const shown = gwen.config?.llm?.assistant_name || "Gwen";
    this.app.spawn(cx, async (cx) => {
      const ok = await this.app.ui.confirm({ title: `Let ${shown} start fresh?`, body: "She forgets the attitude she chose and her note about you, and makes up her mind again as you talk.", confirm: "Start fresh" }, cx);
      if (!ok) return;
      if (await gwen.run(() => api.ResetAssistantSelf())) {
        this.self = { attitude: "", note: "", since: null };
        gwen.toast(`${shown} is starting fresh`, "success");
      }
    });
  }

  render(ui) {
    const t = ui.t;
    const llm = gwen.config.llm;
    const shown = llm.assistant_name || "Gwen";
    const changed = this.name.value.trim() !== (llm.assistant_name ?? "") || this.instructions.value.trim() !== (llm.instructions ?? "");
    const panel = ui
      .panel({ title: "Personality", icon: "drama" })
      .child(
        ui.muted(
          `${shown} has one personality: a Gwen Stacy who's witty, affectionate, and honest with you. She decides for herself how to treat you, from how your days are going and what you tell her, and keeps a note on why. It shapes the chat, the day plan, and the weekly retro.`,
          { size: 13 },
        ),
      );
    const self = this.self;
    if (self) {
      const attitude = attitudeOf(self.attitude);
      const decided = Boolean(attitude || self.note);
      const words = v_flex().flex_1().min_w(0).gap(4);
      if (decided) {
        words.child(
          h_flex()
            .items_center()
            .gap(8)
            .child(div().text_size(13.5).font_medium().child(attitude ? `${attitude.label} with you` : "Making up her mind"))
            .when(self.since != null, (e) => e.child(ui.dim(`since ${formatAgo(self.since, Date.now())}`))),
        );
        if (attitude) words.child(ui.dim(attitude.means));
        if (self.note)
          words.child(
            v_flex()
              .mt(6)
              .pl(10)
              .border_l(2)
              .border_color(alpha(t.accent, 0.5))
              .gap(3)
              .child(div().text_size(13).line_height("19px").italic().text_color(t.muted).child(`“${self.note}”`))
              .child(ui.dim("Her note to herself", { size: 11 })),
          );
      } else
        words
          .child(div().text_size(13.5).font_medium().child("Just herself, for now"))
          .child(ui.dim(`Talk to ${shown} in the Gwen tab. If you slack off or have a rough day, she'll change how she treats you, and say so.`));
      panel.child(
        h_flex()
          .items_start()
          .gap(16)
          .p(14)
          .rounded(8)
          .border_1()
          .border_color(t.border)
          .bg(alpha(t.raised, 0.4))
          .child(ui.face(attitude?.face ?? "smiling", 88))
          .child(words)
          .when(decided, (e) => e.child(ui.button("ps-reset", "Start fresh", (ev, cx) => this.reset(cx), { small: true, tone: "ghost", icon: "rotate-ccw" }))),
      );
    }
    const left = 2000 - this.instructions.value.length;
    panel
      .child(ui.field("Name", this.name.el(ui, { w: 240 })))
      .child(ui.field("About you", this.instructions.el(ui, { h: 100 }), left >= 0 ? `Anything about you or how you like to be helped. ${left} characters left.` : `${-left} characters too many: the end is cut off when you save.`))
      .child(h_flex().justify_end().pt(12).border_t(1).border_color(t.border).child(ui.button("ps-save", "Save", (e, cx) => this.save(cx), { tone: "primary", busy: this.saving, disabled: !changed || !this.name.value.trim() })));
    return panel;
  }
}
