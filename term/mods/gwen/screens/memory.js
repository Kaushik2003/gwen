// Memory: what Gwen knows about you and the people in your life, what she is
// unsure of, and why she holds each thing. Fix what's wrong, hide what she
// shouldn't bring up, or forget it for good.

import { div } from "gpui-kit";
import { v_flex, h_flex } from "gpui-base";
import { api, apiError, gwen } from "../lib/gwen.js";
import { host } from "../lib/host.js";
import { formatAgo } from "../lib/format.js";
import { cardSections, kindLabel, laneLabel, laneState, receiptText, rememberedText, sectionOf, shortDate, sizeText, whenText } from "../lib/memory.js";
import { MemoryDetail, assistantName } from "../lib/memory-detail.js";
import { TextField, alpha } from "../lib/kit.js";

const query = (status, limit) => ({ status, hidden: true, limit });

/** The daemon's message as a sentence. */
function sentence(s) {
  const t = s.charAt(0).toUpperCase() + s.slice(1);
  return /[.!?]$/.test(t) ? t : `${t}.`;
}

export default class Memory {
  constructor(app) {
    this.app = app;
    this.detail = new MemoryDetail(app, { onChanged: (cx) => this.reload(cx) });
    this.past = false;
    this.data = null;
    this.error = null;
    this.loaded = null;
    this.seq = 0;
    this.recall = null; // {query, result}
    this.resolving = {};
    this.adding = false;
    this.filter = new TextField({ placeholder: `Search what ${assistantName()} knows`, onChange: (v, cx) => cx.notify(), onSubmit: (v, cx) => this.runRecall(v, cx) });
  }

  get name() {
    return assistantName();
  }

  sync(cx) {
    this.detail.sync(cx);
    const enabled = gwen.config?.memory?.enabled ?? true;
    const stamp = `${enabled}/${this.past}/${gwen.versions.memory}`;
    if (this.loaded === stamp) return;
    this.loaded = stamp;
    if (enabled) this.load(cx);
  }

  reload(cx) {
    this.loaded = null;
    this.sync(cx);
  }

  load(cx) {
    const seq = ++this.seq;
    cx.spawn(async (cx) => {
      try {
        const [claims, candidates, quarantined, conflicts, status, resources, card] = await Promise.all([
          api.ListClaims(query(this.past ? "all" : "active", 500)),
          api.ListClaims(query("candidate", 100)),
          api.ListClaims(query("quarantined", 100)),
          api.ListConflicts(""),
          api.MemoryStatus(),
          api.ListResources(),
          api.MemoryProfile().catch(() => null),
        ]);
        if (seq !== this.seq) return;
        this.data = {
          claims: (claims.claims ?? []).filter((c) => c.status === "active" || c.status === "superseded"),
          candidates: candidates.claims ?? [],
          quarantined: quarantined.claims ?? [],
          conflicts: conflicts.conflicts ?? [],
          status,
          resources: resources.resources ?? [],
          card,
        };
        this.error = null;
      } catch (e) {
        if (seq !== this.seq) return;
        const err = apiError(e);
        if (err.code === "daemon_not_running" || err.code === "unauthorized") gwen.fail(e);
        this.error = err;
      }
      cx.notify();
    });
  }

  runRecall(q, cx) {
    const text = q.trim();
    if (!text) {
      this.recall = null;
      cx.notify();
      return;
    }
    this.recall = { query: text, result: null };
    cx.notify();
    cx.spawn(async (cx) => {
      try {
        const result = await api.RecallMemory({ query: text, as_of: "", lanes: [] });
        if (this.recall?.query === text) this.recall = { query: text, result };
      } catch (e) {
        this.recall = null;
        gwen.fail(e);
      }
      cx.notify();
    });
  }

  remember(cx) {
    const ui = this.app.ui;
    const name = this.name;
    let saving = false;
    const text = new TextField({
      multiline: true,
      rows: 4,
      placeholder: "My sister Mira's birthday is 3 March. I work at Acme as a designer. I hate meetings before 10.",
      onChange: (v, cx) => cx.notify(),
      onSubmit: (v, cx, e) => save(cx),
    });
    const save = (cx) => {
      const words = text.value.trim();
      if (!words || saving) return;
      saving = true;
      cx.notify();
      this.app.spawn(cx, async (cx) => {
        const r = await gwen.act(() => api.Remember({ text: words.slice(0, 4000) }, ""));
        saving = false;
        if (!r) return;
        const claims = r.claims ?? [];
        gwen.toast(rememberedText(claims, name), claims.length ? "success" : "info");
        this.reload(cx);
        if (claims.length) ui.closeDialog(cx, id);
      });
    };
    const id = ui.openDialog(
      {
        title: "Remember something",
        width: 520,
        render: (ui) =>
          v_flex()
            .gap(12)
            .child(ui.muted(`Write it the way you'd tell ${name}. She keeps each fact in it as something you set yourself.`, { size: 13 }))
            .child(ui.field("What to remember", text.el(ui, { h: 100 }), saving ? `${name} is reading it…` : `${4000 - text.value.length} characters left. Enter saves, Shift+Enter starts a new line.`)),
        actions: (ui) => [
          ui.button("mm-rem-cancel", "Cancel", (e, cx) => ui.closeDialog(cx, id), { tone: "ghost" }),
          ui.button("mm-rem-save", "Remember", (e, cx) => save(cx), { tone: "primary", icon: "book-marked", busy: saving, disabled: !text.value.trim() }),
        ],
      },
      cx,
    );
    text.focus();
  }

  resolve(c, resolution, said, cx) {
    this.resolving[c.id] = resolution;
    cx.notify();
    this.app.spawn(cx, async (cx) => {
      const ok = await gwen.act(() => api.ResolveConflict(c.id, { resolution }));
      delete this.resolving[c.id];
      if (ok) {
        gwen.toast(said, "success");
        this.reload(cx);
      }
    });
  }

  addDocument(cx) {
    const name = this.name;
    this.app.spawn(cx, async (cx) => {
      const path = await host.pickFile("Give Gwen a document", "*.md *.txt *.pdf *.html *.org *.rst");
      if (!path) return;
      this.adding = true;
      cx.notify();
      try {
        const r = await gwen.act(() => api.AddResource({ path, title: "" }));
        if (r) {
          gwen.toast(`${name} read “${r.title}”. What she learns from it waits for you under To review.`, "success");
          this.reload(cx);
        }
      } finally {
        this.adding = false;
      }
    });
  }

  removeDocument(doc, cx) {
    const name = this.name;
    this.app.spawn(cx, async (cx) => {
      const ok = await this.app.ui.confirm({ title: `Remove “${doc.title}”?`, body: `${name} forgets the document's words and what she learned only from it. The file itself stays where it is.`, confirm: "Remove", danger: true }, cx);
      if (!ok) return;
      const r = await gwen.act(() => api.RemoveResource(doc.id));
      if (r) {
        gwen.toast(receiptText(r, `Removed “${doc.title}”.`, name), "success");
        this.reload(cx);
      }
    });
  }

  // Drawing ----------------------------------------------------------------------

  recallPanel(ui) {
    const r = this.recall.result;
    const hits = r?.hits ?? [];
    const panel = ui
      .panel({
        title: `${this.name} would recall`,
        icon: "sparkles",
        actions: ui.iconButton(
          "mm-recall-close",
          "x",
          (e, cx) => {
            this.recall = null;
            cx.notify();
          },
          { tip: "Close" },
        ),
      })
      .child(ui.muted(`If you asked her about “${this.recall.query}”.`, { size: 13 }));
    if (!r) panel.child(h_flex().items_center().gap(8).child(ui.spinner(14)).child(ui.muted("Thinking back…", { size: 13 })));
    else if (hits.length === 0 || r.abstain) panel.child(ui.muted("Nothing comes to mind for that.", { size: 13 }));
    else
      panel.child(
        v_flex().children(
          hits.map((h, i) =>
            v_flex()
              .id(`mm-hit-${i}`)
              .gap(5)
              .px(8)
              .py(8)
              .rounded(8)
              .cursor_pointer()
              .hover((s) => s.bg(ui.t.hover))
              .on_click((e, cx) => this.detail.openRef(h.ref, cx))
              .child(div().text_size(13.5).child(h.text))
              .child(
                h_flex()
                  .flex_wrap()
                  .items_center()
                  .gap(6)
                  .child(h.kind === "claim" ? (this.detail.trustBadge(ui, `mm-hit-${i}-t`, h.trust) ?? div()) : ui.badge(kindLabel(h.kind)))
                  .when(Boolean(h.disputed), (e) => e.child(ui.badge("Not sure", { tone: "warn" })))
                  .child(this.detail.sourceChip(ui, `mm-hit-${i}-s`, h.source) ?? div()),
              ),
          ),
        ),
      );
    if (r && (r.degraded ?? []).length > 0) panel.child(ui.dim(`Didn't answer in time: ${r.degraded.map(laneLabel).join(", ")}.`));
    return panel;
  }

  conflictCard(ui, c) {
    const t = ui.t;
    const name = this.name;
    const busy = this.resolving[c.id];
    const side = (label, x, key) =>
      v_flex()
        .flex_1()
        .min_w(0)
        .gap(6)
        .p(12)
        .rounded(8)
        .border_1()
        .border_color(t.border)
        .bg(t.raised)
        .child(ui.dim(label, { size: 11 }))
        .child(div().id(`mm-cf-${c.id}-${key}`).cursor_pointer().text_size(13.5).hover((s) => s.text_color(t.accent)).on_click((e, cx) => this.detail.openClaim(x.id, cx)).child(x.text))
        .child(
          h_flex()
            .flex_wrap()
            .items_center()
            .gap(6)
            .child(this.detail.sourceChip(ui, `mm-cf-${c.id}-${key}-s`, x.source, { thread: x.thread_id || undefined }) ?? div())
            .child(ui.dim(shortDate(x.mentioned_at || x.recorded_at), { size: 12 })),
        );
    const btn = (res, label, said) => ui.button(`mm-cf-${c.id}-${res}`, label, (e, cx) => this.resolve(c, res, said, cx), { small: true, busy: busy === res, disabled: Boolean(busy) && busy !== res });
    return v_flex()
      .gap(10)
      .p(13)
      .rounded(8)
      .border_1()
      .border_color(alpha(ui.tone("warn"), 0.25))
      .bg(alpha(ui.tone("warn"), 0.04))
      .child(h_flex().items_center().gap(8).child(ui.icon("circle-help", 15, ui.tone("warn"))).child(div().text_size(13.5).font_medium().child(`${name} isn't sure which is right`)))
      .child(h_flex().gap(8).items_start().child(side("The first", c.a, "a")).child(side("The second", c.b, "b")))
      .child(
        h_flex()
          .flex_wrap()
          .gap(8)
          .child(btn("keep_a", "The first", `${name} will go with: ${c.a.text}`))
          .child(btn("keep_b", "The second", `${name} will go with: ${c.b.text}`))
          .child(btn("both", "Both are true", `${name} will keep both.`))
          .child(btn("neither", "Neither", `${name} will drop both.`)),
      );
  }

  candidates(ui, title, claims) {
    const t = ui.t;
    return v_flex()
      .gap(6)
      .child(ui.dim(title))
      .child(
        v_flex()
          .rounded(8)
          .border_1()
          .border_color(t.border)
          .children(
            claims.map((c, i) =>
              h_flex()
                .flex_wrap()
                .items_center()
                .gap(10)
                .px(12)
                .py(9)
                .when(i < claims.length - 1, (e) => e.border_b(1).border_color(t.border))
                .child(
                  v_flex()
                    .id(`mm-cand-${c.id}`)
                    .flex_1()
                    .min_w(240)
                    .gap(4)
                    .cursor_pointer()
                    .on_click((e, cx) => this.detail.openClaim(c.id, cx))
                    .child(div().text_size(13.5).child(c.text))
                    .child(
                      h_flex()
                        .flex_wrap()
                        .items_center()
                        .gap(6)
                        .child(this.detail.sourceChip(ui, `mm-cand-${c.id}-s`, c.source, { thread: c.thread_id || undefined }) ?? div())
                        .child(ui.dim(whenText(c), { size: 12 })),
                    ),
                )
                .child(
                  h_flex()
                    .flex_none()
                    .gap(6)
                    .child(ui.button(`mm-cand-${c.id}-keep`, "Keep", (e, cx) => this.detail.keep(c, cx), { small: true, tone: "primary", icon: "check" }))
                    .child(ui.button(`mm-cand-${c.id}-fix`, "Fix", (e, cx) => this.detail.edit(c, cx), { small: true, icon: "pencil" }))
                    .child(ui.button(`mm-cand-${c.id}-no`, "Not true", (e, cx) => this.detail.reject(c, cx), { small: true, icon: "thumbs-down" })),
                ),
            ),
          ),
      );
  }

  claimList(ui, title, icon, claims, filtered, grouped) {
    if (claims.length === 0) return null;
    const t = ui.t;
    const groups = [];
    if (grouped) {
      const by = new Map();
      for (const c of claims) by.set(c.subject || "Someone", [...(by.get(c.subject || "Someone") ?? []), c]);
      groups.push(...[...by.entries()].sort(([a], [b]) => a.localeCompare(b)));
    } else groups.push(["", claims]);
    const panel = ui.panel({ title, icon, pad: 12, actions: filtered ? ui.dim(`${claims.length} matching`) : null });
    for (const [who, list] of groups) {
      const block = v_flex();
      if (who) block.child(div().px(10).pt(6).pb(2).text_size(13).font_medium().text_color(t.muted).child(who));
      for (const c of list) {
        const pastValue = c.status === "superseded";
        block.child(
          h_flex()
            .id(`mm-c-${c.id}`)
            .items_start()
            .gap(8)
            .px(10)
            .py(8)
            .rounded(8)
            .cursor_pointer()
            .hover((s) => s.bg(t.hover))
            .on_click((e, cx) => this.detail.openClaim(c.id, cx))
            .child(
              v_flex()
                .flex_1()
                .min_w(0)
                .gap(5)
                .child(div().text_size(13.5).text_color(pastValue || c.hidden ? t.muted : t.foreground).child(c.text))
                .child(
                  h_flex()
                    .flex_wrap()
                    .items_center()
                    .gap(6)
                    .child(ui.dim(whenText(c), { size: 12 }))
                    .child(this.detail.sourceChip(ui, `mm-c-${c.id}-s`, c.source, { thread: c.thread_id || undefined }) ?? div())
                    .child(this.detail.trustBadge(ui, `mm-c-${c.id}-t`, c.trust) ?? div())
                    .children(this.detail.stateBadges(ui, `mm-c-${c.id}-b`, c) ?? []),
                ),
            )
            .when(!pastValue, (e) => e.child(this.detail.claimMenu(ui, `mm-c-${c.id}-menu`, c))),
        );
      }
      panel.child(block);
    }
    return panel;
  }

  side(ui) {
    const t = ui.t;
    const name = this.name;
    const data = this.data;
    const st = data.status;
    const learn = gwen.config?.memory?.learn ?? "assistant";
    const card = cardSections(data.card?.text ?? st.profile?.text ?? "");
    const waiting = (st.jobs?.pending ?? 0) + (st.jobs?.running ?? 0);
    const now = Date.now();
    const lanes = (st.lanes ?? []).filter((l) => l.enabled || ["vec", "files", "mem0", "supermemory"].includes(l.lane));
    const section = (title, ...children) => v_flex().gap(8).px(18).py(14).border_b(1).border_color(t.border).child(div().text_size(13).font_semibold().child(title)).children(children);

    const cardEl =
      card.length === 0
        ? ui.muted("Nothing yet. Once she knows you a little, a short summary appears here. She reads it before every chat.", { size: 13 })
        : v_flex()
            .gap(10)
            .children(
              card.map((s) =>
                v_flex()
                  .gap(3)
                  .when(Boolean(s.heading), (e) => e.child(ui.dim(s.heading)))
                  .children(s.lines.map((l) => div().text_size(13).line_height("19px").text_color(t.muted).child(l))),
              ),
            );
    const learning = [
      learn === "off"
        ? `Learning from chats is off. ${name} only keeps what you tell her to remember.`
        : !st.learning
          ? "Learning is paused: the model she learns with isn't working. Check Settings, Memory."
          : `${name} learns from your chats ${learn === "local" ? "with a local model" : "with the assistant's model"}.`,
      st.last_learned_at != null ? ` Last learned ${formatAgo(st.last_learned_at, now)}.` : "",
      waiting > 0 ? ` ${waiting} ${waiting === 1 ? "thing" : "things"} waiting.` : "",
      st.last_slept_at != null ? ` Tidied up ${formatAgo(st.last_slept_at, now)}.` : "",
    ].join("");
    const laneList = v_flex()
      .gap(6)
      .children(
        lanes.map((l) => {
          const s = laneState(l);
          return h_flex()
            .id(`mm-lane-${l.lane}`)
            .items_center()
            .gap(8)
            .tooltip(s.words)
            .child(ui.dot(ui.tone(s.color), 8))
            .child(div().text_size(13).text_color(t.muted).whitespace_nowrap().child(laneLabel(l.lane)))
            .child(div().flex_1())
            .child(div().min_w(0).truncate().text_size(12).text_color(t.dim).child(s.short));
        }),
      );
    const docs =
      data.resources.length === 0
        ? ui.muted(`Give ${name} a document, such as notes about your course, and she remembers what's in it.`, { size: 13 })
        : v_flex()
            .gap(4)
            .children(
              data.resources.map((r) =>
                h_flex()
                  .items_center()
                  .gap(8)
                  .child(ui.icon("file-text", 14, t.dim))
                  .child(v_flex().flex_1().min_w(0).child(div().id(`mm-doc-${r.id}`).truncate().text_size(13).tooltip(r.path).child(r.title)).child(ui.dim(`${sizeText(r.size)} · added ${shortDate(r.added_at)}`, { size: 11 })))
                  .child(ui.iconButton(`mm-doc-${r.id}-rm`, "trash-2", (e, cx) => this.removeDocument(r, cx), { tip: `Remove ${r.title}`, size: 24 })),
              ),
            );
    return v_flex()
      .w(320)
      .flex_none()
      .rounded(10)
      .border_1()
      .border_color(t.border)
      .bg(t.surface)
      .child(
        div()
          .px(18)
          .py(8)
          .border_b(1)
          .border_color(t.border)
          .child(
            ui.toggleRow(
              "mm-past",
              "Show past values",
              "Things that used to be true, such as a job you left.",
              this.past,
              (on, cx) => {
                this.past = on;
                this.sync(cx);
              },
              { divider: false },
            ),
          ),
      )
      .child(section(`What ${name} keeps in mind today`, cardEl))
      .child(section("Learning", ui.muted(learning, { size: 13 }), laneList, h_flex().child(ui.button("mm-settings", "Memory settings", () => this.app.go("settings", null, "memory"), { small: true, tone: "ghost", icon: "settings" }))))
      .child(section("Documents", docs, h_flex().child(ui.button("mm-doc-add", "Add a document", (e, cx) => this.addDocument(cx), { small: true, icon: "file-plus-2", busy: this.adding }))).border_b(0));
  }

  render(ui) {
    const t = ui.t;
    const name = this.name;
    const enabled = gwen.config?.memory?.enabled ?? true;
    const header = ui.pageHeader(
      `What ${name} knows`,
      `What ${name} has learned about you and the people in your life, and why. Fix what's wrong, hide what she shouldn't bring up, or forget it for good.`,
      enabled && !this.error ? ui.button("mm-remember", "Remember something", (e, cx) => this.remember(cx), { tone: "primary", icon: "plus" }) : null,
    );
    const err = this.error;
    if (!enabled || err?.code === "unavailable")
      return ui.scroll("mm-scroll", [
        header,
        ui.callout({
          tone: "warn",
          icon: "circle-alert",
          text: !enabled ? `Memory is off, so ${name} doesn't learn or recall anything about you. Turn it on in Settings, Memory.` : sentence(err.message),
          action: ui.button("mm-off-settings", "Memory settings", () => this.app.go("settings", null, "memory"), { small: true, tone: "primary", icon: "settings" }),
        }),
      ]);
    if (err)
      return ui.scroll("mm-scroll", [header, ui.callout({ tone: "danger", icon: "circle-alert", text: `${name}'s memory couldn't be read: ${err.message}`, action: ui.button("mm-retry", "Try again", (e, cx) => this.reload(cx), { small: true }) })]);

    const words = this.filter.value.trim().toLowerCase().split(/\s+/).filter(Boolean);
    const matches = (c) => words.every((w) => `${c.text} ${c.label} ${c.subject} ${c.source}`.toLowerCase().includes(w));
    const data = this.data;
    const main = v_flex()
      .flex_1()
      .min_w(0)
      .gap(16)
      .child(
        v_flex()
          .gap(6)
          .child(
            h_flex()
              .items_center()
              .gap(8)
              .child(ui.icon("search", 15, t.dim))
              .child(this.filter.el(ui))
              .when(Boolean(this.filter.value), (e) =>
                e.child(
                  ui.iconButton(
                    "mm-clear",
                    "x",
                    (ev, cx) => {
                      this.filter.set("");
                      this.recall = null;
                      cx.notify();
                    },
                    { tip: "Clear" },
                  ),
                ),
              ),
          )
          .child(ui.dim(`Typing filters the lists below. Press Enter to see what ${name} would recall if you asked her about it.`)),
      );
    if (this.recall) main.child(this.recallPanel(ui));
    if (!data) main.child(h_flex().items_center().gap(8).child(ui.spinner(14)).child(ui.muted(`Reading what ${name} knows…`, { size: 13 })));
    else {
      const review = data.conflicts.length + data.candidates.length + data.quarantined.length;
      if (data.claims.length === 0 && review === 0)
        main.child(
          ui.empty({
            icon: "book-marked",
            title: `${name} hasn't learned anything about you yet.`,
            body: "Tell her about yourself in the Gwen tab, or use Remember something.",
            action: h_flex().gap(8).child(ui.button("mm-empty-chat", "Open Gwen", () => this.app.go("gwen"), { icon: "sparkles" })).child(ui.button("mm-empty-rem", "Remember something", (e, cx) => this.remember(cx), { tone: "primary", icon: "plus" })),
          }),
        );
      else {
        if (review > 0) {
          const panel = ui.panel({ title: "To review", icon: "circle-help", actions: ui.dim("She doesn't rely on these until you say") });
          for (const c of data.conflicts) panel.child(this.conflictCard(ui, c));
          if (data.candidates.length) panel.child(this.candidates(ui, `${name} thinks`, data.candidates));
          if (data.quarantined.length) panel.child(this.candidates(ui, "From a document", data.quarantined));
          main.child(panel);
        }
        const by = { you: [], people: [], other: [] };
        for (const c of data.claims) if (matches(c)) by[sectionOf(c)].push(c);
        // Past values after the ones that hold now.
        for (const s of Object.values(by)) s.sort((a, b) => Number(a.status === "superseded") - Number(b.status === "superseded"));
        const filtered = words.length > 0;
        for (const el of [this.claimList(ui, "About you", null, by.you, filtered, false), this.claimList(ui, "People", "users", by.people, filtered, true), this.claimList(ui, "Other", null, by.other, filtered, true)]) if (el) main.child(el);
      }
    }
    const body = h_flex().w_full().items_start().gap(16).child(main);
    if (data) body.child(this.side(ui));
    return ui.scroll("mm-scroll", [header, body]);
  }
}
