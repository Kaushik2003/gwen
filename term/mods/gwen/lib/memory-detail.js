// Why Gwen holds something, and what can be done about it: the detail dialog
// of a memory, the claim actions with their toasts and confirmations, and the
// small pieces that show a claim (ui/src/components/MemoryDetail.tsx). The
// Memory screen and the chat both keep one.
//
//   import { MemoryDetail } from "../lib/memory-detail.js";
//
//   this.detail = new MemoryDetail(app, { onChanged: (cx) => this.reload(cx) });   // in a screen's constructor
//   this.detail.sync(cx);                                                           // in the screen's sync(cx)
//
// Exports
//
//   assistantName()                     the assistant's name, as the chat shows it ("Gwen")
//   claimOfRef(ref)                     a chat chip's MemoryRef {ref, text, status} as the claim the actions take
//   class MemoryDetail
//     constructor(app, o)               o.onChanged(cx): runs after any action changed memory (reload your lists)
//     name                              the assistant's name
//     sync(cx)                          call from the screen's sync: an open detail reads again when memory changed
//
//     Opening (from a handler; cx is the handler's)
//     openClaim(id, cx, o)              the detail dialog of a claim id
//     openRef(ref, cx, o)               the same for a ref: "claim:<id>", "msg:<id>", "ep:<id>", "doc:<id>"; a bare claim id works too
//                                       o.thread: the chat it came from, when the caller knows it (its source chip opens that chat)
//     close(cx)                         closes the open detail, if any
//
//     Actions (from a handler). Each reports its own toast and failure, then
//     runs o.onChanged and after(cx) when it changed something.
//     keep(claim, cx, after)            confirm a claim she was unsure of
//     reject(claim, cx, after)          not true: she drops it and won't learn it again
//     hide(claim, hidden, cx, after)    don't bring this up / bring it up again
//     forget(claim, cx, after)          asks first, then erases it everywhere
//     edit(claim, cx, after)            opens the editor dialog (the value of a one-valued fact, else the statement)
//                                       claim needs {id, text}; edit also reads key, value, label, subject
//
//     Pieces to draw with (ui is the tab's Ui; ids must be unique in the tab)
//     trustBadge(ui, id, trust)         "You set it", "Gwen noticed", ...; null for an unknown trust
//     stateBadges(ui, id, claim)        [badges]: To confirm, Past value, Not true, Not sure, Hidden
//     sourceChip(ui, id, source, o)     where something came from; o.thread makes it open that chat; o.onOpen(cx) runs first; null without a source
//     claimMenu(ui, id, claim)          a row's "…" menu: Edit, Don't bring this up / Bring it up again, Not true, Forget
//     chips(ui, idPrefix, message)      the memory chips under a chat message {role, cites, forget, learned} (each a
//                                       list of MemoryRef): "Remembered: …" opens why; "Won't bring up: …" offers Forget
//                                       for good; "Gwen noted: …" opens why and offers Keep / Not true while it waits.
//                                       null when the message has none.

import { div } from "gpui-kit";
import { v_flex, h_flex } from "gpui-base";
import { api, gwen, apiError } from "./gwen.js";
import { TextField, alpha } from "./kit.js";
import { formatTime } from "./format.js";
import { claimIdOf, historyLine, kindLabel, plainValue, receiptText, shortDate, threadParam, toConfirm, trustLabel, trustMeans, whenText } from "./memory.js";

/** The assistant's name, as the chat shows it. */
export function assistantName() {
  return gwen.config?.llm?.assistant_name || "Gwen";
}

/** The claim a chat chip names, enough for the claim actions. */
export function claimOfRef(ref) {
  return { id: claimIdOf(ref.ref) ?? "", text: ref.text, status: ref.status ?? "" };
}

/** The message id of a ref such as "msg:<id>", or null for any other ref. */
function messageIdOf(ref) {
  return ref?.startsWith("msg:") ? ref.slice("msg:".length) : null;
}

function cut(s, n) {
  const text = String(s ?? "");
  return text.length > n ? `${text.slice(0, n).trimEnd()}…` : text;
}

export class MemoryDetail {
  constructor(app, o = {}) {
    this.app = app;
    this.o = o;
    this.cur = null; // the open detail: {ref, thread, x, error, id, dialog, stamp}
  }

  get name() {
    return assistantName();
  }

  // Actions ---------------------------------------------------------------------

  /** Runs one action from a handler; when it changed something, the lists and the open detail read again. */
  act(cx, work, after) {
    this.app.spawn(cx, async (cx) => {
      if (!(await work(cx))) return;
      this.changed(cx);
      after?.(cx);
    });
  }

  changed(cx) {
    this.o.onChanged?.(cx);
    if (this.cur) this.load(this.cur, cx);
  }

  keep(c, cx, after) {
    this.act(
      cx,
      async () => {
        const ok = await gwen.act(() => api.ConfirmClaim(c.id));
        if (ok) gwen.toast(`${this.name} will keep this: ${c.text}`, "success");
        return Boolean(ok);
      },
      after,
    );
  }

  reject(c, cx, after) {
    this.act(
      cx,
      async () => {
        const ok = await gwen.act(() => api.RejectClaim(c.id));
        if (ok) gwen.toast(`Got it. ${this.name} won't hold on to that, and won't learn it again.`, "success");
        return Boolean(ok);
      },
      after,
    );
  }

  hide(c, hidden, cx, after) {
    this.act(
      cx,
      async () => {
        const ok = await gwen.act(() => api.HideClaim(c.id, { hidden }));
        if (ok) gwen.toast(hidden ? `${this.name} won't bring this up. She still knows it.` : `${this.name} may bring this up again.`, "success");
        return Boolean(ok);
      },
      after,
    );
  }

  forget(c, cx, after) {
    this.act(
      cx,
      async (cx) => {
        if (!(await this.confirmForget(c, cx))) return false;
        const r = await gwen.act(() => api.ForgetClaim(c.id));
        if (r) gwen.toast(receiptText(r, "Forgotten.", this.name), "success");
        return Boolean(r);
      },
      after,
    );
  }

  /** Asks before a forget, showing the words that go. Resolves true when confirmed. */
  confirmForget(c, cx) {
    const ui = this.app.ui;
    const name = this.name;
    return new Promise((resolve) => {
      let answered = false;
      const answer = (yes, cx) => {
        if (answered) return;
        answered = true;
        resolve(yes);
        ui.closeDialog(cx, id);
      };
      const id = ui.openDialog(
        {
          title: "Forget this for good?",
          width: 440,
          render: (ui) =>
            v_flex()
              .gap(8)
              .child(ui.text(c.text, { size: 13, lh: 19 }))
              .child(ui.muted(`${name} erases it everywhere she keeps it, and won't learn it again from the same words. This can't be undone. To keep it but stop her mentioning it, use Don't bring this up instead.`, { size: 13, lh: 19 })),
          actions: (ui) => [ui.button("md-forget-no", "Cancel", (e, cx) => answer(false, cx), { tone: "ghost" }), ui.button("md-forget-yes", "Forget", (e, cx) => answer(true, cx), { tone: "danger" })],
          onClose: () => {
            if (!answered) {
              answered = true;
              resolve(false);
            }
          },
        },
        cx,
      );
    });
  }

  /** Fixes a claim: the value of a one-valued plain fact, else the whole statement. */
  edit(c, cx, after) {
    const ui = this.app.ui;
    const name = this.name;
    const value = plainValue(c);
    const before = value ?? c.text ?? "";
    const limit = value != null ? 200 : 500;
    let saving = false;
    const changed = () => field.value.trim() !== "" && field.value.trim() !== before && field.value.trim().length <= limit;
    const save = (e, cx) => {
      if (!changed() || saving) return;
      saving = true;
      cx.notify();
      this.app.spawn(cx, async (cx) => {
        const text = field.value.trim();
        const r = await gwen.act(() => api.EditClaim(c.id, value != null ? { text: "", value: text } : { text }));
        saving = false;
        if (!r) return;
        const now = (r.claims ?? []).map((x) => x.text);
        gwen.toast(now.length ? `Saved. ${name} now remembers: ${now.join("; ")}.` : "Saved.", "success");
        ui.closeDialog(cx, id);
        this.changed(cx);
        after?.(cx);
      });
    };
    const field = new TextField({ value: before, multiline: value == null, rows: 3, onSubmit: (v, cx) => save(null, cx) });
    const id = ui.openDialog(
      {
        title: `Fix what ${name} knows`,
        width: 500,
        render: (ui) => {
          const over = field.value.trim().length - limit;
          return v_flex()
            .gap(14)
            .child(ui.muted(value != null ? `The old value is kept as history, so ${name} knows it changed.` : `${name} remembers it the way you write it here.`, { size: 13, lh: 19 }))
            .child(
              value != null
                ? ui.field(c.subject ? `${c.subject}: ${c.label}` : c.label || "Value", field.el(ui), over > 0 ? `${over} characters too long.` : null)
                : ui.field(`What ${name} should remember`, field.el(ui, { h: 84 }), over > 0 ? `${over} characters too long.` : "Enter saves, Shift+Enter starts a new line."),
            );
        },
        actions: (ui) => [ui.button("md-edit-cancel", "Cancel", (e, cx) => ui.closeDialog(cx, id), { tone: "ghost" }), ui.button("md-edit-save", "Save", save, { tone: "primary", busy: saving, disabled: !changed() })],
      },
      cx,
    );
    field.focus();
  }

  // Pieces ------------------------------------------------------------------------

  /** A pill; with a tip it needs the id. */
  badge(ui, id, label, o = {}) {
    const el = ui.badge(label, { tone: o.tone, icon: o.icon });
    if (o.tip) el.id(id).tooltip(o.tip);
    return el;
  }

  /** Who a claim rests on, in words: You set it, You said it, Gwen noticed, Gwen inferred, From a document. */
  trustBadge(ui, id, trust) {
    const label = trustLabel(trust, this.name);
    if (!label) return null;
    return this.badge(ui, id, label, { tone: trust === "owner_edit" || trust === "owner_said" ? "neutral" : "accent", tip: trustMeans(trust, this.name) });
  }

  /** The badges of a claim's state: waiting for you, a past value, not true, not sure while disputed, hidden. */
  stateBadges(ui, id, c) {
    const out = [];
    if (c.status === "candidate" || c.status === "quarantined") out.push(ui.badge("To confirm", { tone: "warn" }));
    if (c.status === "superseded") out.push(ui.badge("Past value"));
    if (c.status === "rejected") out.push(ui.badge("Not true", { tone: "danger" }));
    if (c.disputed) out.push(this.badge(ui, `${id}-disputed`, "Not sure", { tone: "warn", icon: "circle-alert", tip: `${this.name} holds something else that disagrees with this.` }));
    if (c.hidden) out.push(this.badge(ui, `${id}-hidden`, "Hidden", { icon: "eye-off", tip: `${this.name} knows it but won't bring it up.` }));
    return out;
  }

  /** Where something came from; with a chat, it opens that chat on the Gwen tab. o: {thread, onOpen(cx)}. */
  sourceChip(ui, id, source, o = {}) {
    if (!source) return null;
    const t = ui.t;
    const icon = source.startsWith("document") ? "file-text" : source.startsWith("chat") ? "message-square" : "notebook-pen";
    const el = h_flex()
      .id(id)
      .max_w("100%")
      .min_w(0)
      .items_center()
      .gap(4)
      .px(6)
      .h(20)
      .rounded(6)
      .border_1()
      .border_color(t.border)
      .bg(t.raised)
      .text_size(11)
      .text_color(t.muted)
      .child(ui.icon(icon, 11, t.muted))
      .child(div().min_w(0).truncate().child(source));
    if (!o.thread) return el;
    return el
      .cursor_pointer()
      .tooltip("Open this chat")
      .hover((s) => s.border_color(t.input).text_color(t.foreground))
      .on_click((e, cx) => {
        cx.stop_propagation();
        o.onOpen?.(cx);
        this.app.go("gwen", null, threadParam(o.thread), cx);
      });
  }

  /** A claim row's menu: Edit, Don't bring this up or Bring it up again, Not true, Forget. */
  claimMenu(ui, id, c) {
    return ui.menuButton(
      id,
      [
        { label: "Edit", icon: "pencil", onClick: (e, cx) => this.edit(c, cx) },
        c.hidden ? { label: "Bring it up again", icon: "eye", onClick: (e, cx) => this.hide(c, false, cx) } : { label: "Don't bring this up", icon: "eye-off", onClick: (e, cx) => this.hide(c, true, cx) },
        { label: "Not true", icon: "thumbs-down", onClick: (e, cx) => this.reject(c, cx) },
        { label: "Forget", icon: "trash-2", danger: true, onClick: (e, cx) => this.forget(c, cx) },
      ],
      { tip: "More", width: 210 },
    );
  }

  /**
   * What a message did to memory, as chips: under a reply, what she
   * remembered to answer and what she won't bring up again; under your
   * message, what she noted from it, with Keep and Not true while it waits
   * for you. m: {role, cites, forget, learned}.
   */
  chips(ui, idPrefix, m) {
    const t = ui.t;
    const cites = m.cites ?? [];
    const forget = m.forget ?? [];
    const learned = m.learned ?? [];
    if (cites.length + forget.length + learned.length === 0) return null;
    const warn = ui.tone("warn");
    const chip = (id) => h_flex().id(id).max_w("100%").min_w(0).items_center().gap(6).px(8).h(24).rounded(6).border_1().text_size(12);
    const small = (id, label, color, onClick, icon) =>
      h_flex()
        .id(id)
        .flex_none()
        .items_center()
        .gap(4)
        .px(5)
        .h(18)
        .rounded(4)
        .text_size(11)
        .font_medium()
        .text_color(color)
        .cursor_pointer()
        .hover((s) => s.bg(t.hover))
        .on_click((e, cx) => {
          cx.stop_propagation();
          onClick(e, cx);
        })
        .when(Boolean(icon), (e) => e.child(ui.icon(icon, 11, color)))
        .child(label);
    const open = (r) => (e, cx) => this.openRef(r.ref, cx, { thread: r.thread_id || undefined });
    const row = h_flex().max_w("100%").min_w(0).flex_wrap().gap(6);
    if (m.role === "user") row.justify_end();
    cites.forEach((r, i) => {
      if (r.gone) row.child(chip(`${idPrefix}-c${i}`).border_color(t.border).bg(t.raised).text_color(t.dim).child(ui.icon("book-marked", 12, t.dim)).child("Remembered something since forgotten"));
      else
        row.child(
          chip(`${idPrefix}-c${i}`)
            .border_color(alpha(t.accent, 0.25))
            .bg(alpha(t.accent, 0.06))
            .text_color(t.muted)
            .cursor_pointer()
            .tooltip("Why she remembers this")
            .hover((s) => s.border_color(alpha(t.accent, 0.5)).text_color(t.foreground))
            .on_click(open(r))
            .child(ui.icon("book-marked", 12, t.accent))
            .child(div().min_w(0).truncate().child(`Remembered: ${r.text}`)),
        );
    });
    forget.forEach((r, i) => {
      const c = chip(`${idPrefix}-f${i}`)
        .border_color(t.input)
        .bg(t.raised)
        .text_color(t.muted)
        .child(ui.icon("eye-off", 12, t.muted))
        .child(div().min_w(0).truncate().child(r.gone ? "Won't bring up something since forgotten" : `Won't bring up: ${r.text}`));
      if (!r.gone && claimIdOf(r.ref)) c.child(small(`${idPrefix}-f${i}-x`, "Forget for good", ui.tone("danger"), (e, cx) => this.forget(claimOfRef(r), cx)));
      row.child(c);
    });
    learned.forEach((r, i) => {
      const waiting = toConfirm(r.status);
      const c = chip(`${idPrefix}-l${i}`)
        .border_color(waiting ? alpha(warn, 0.3) : t.input)
        .bg(waiting ? alpha(warn, 0.06) : t.raised)
        .text_color(t.muted)
        .child(ui.icon("notebook-pen", 12, waiting ? warn : t.muted))
        .child(
          div()
            .id(`${idPrefix}-l${i}-open`)
            .min_w(0)
            .truncate()
            .cursor_pointer()
            .tooltip("Why she noted this")
            .hover((s) => s.text_color(t.foreground))
            .on_click(open(r))
            .child(`${this.name} noted: ${r.text}`),
        );
      if (waiting)
        c.child(div().flex_none().text_size(11).text_color(warn).child("to confirm"))
          .child(small(`${idPrefix}-l${i}-keep`, "Keep", ui.tone("ok"), (e, cx) => this.keep(claimOfRef(r), cx), "check"))
          .child(small(`${idPrefix}-l${i}-no`, "Not true", t.muted, (e, cx) => this.reject(claimOfRef(r), cx), "thumbs-down"));
      row.child(c);
    });
    return row;
  }

  // The detail dialog -----------------------------------------------------------------

  openClaim(id, cx, o = {}) {
    this.openRef(`claim:${id}`, cx, o);
  }

  openRef(ref, cx, o = {}) {
    const ui = this.app.ui;
    if (this.cur) ui.closeDialog(cx, this.cur.id);
    const cur = { ref: ref.includes(":") ? ref : `claim:${ref}`, thread: o.thread || "", x: null, error: null, id: null, dialog: null, stamp: null };
    this.cur = cur;
    cur.id = ui.openDialog(
      {
        title: "Loading",
        width: 640,
        render: (ui) => this.body(ui, cur),
        actions: (ui) => this.footer(ui, cur),
        onClose: () => {
          if (this.cur === cur) this.cur = null;
        },
      },
      cx,
    );
    cur.dialog = ui.dialogs.find((d) => d.id === cur.id) ?? null;
    this.load(cur, cx);
  }

  close(cx) {
    if (this.cur) this.app.ui.closeDialog(cx, this.cur.id);
  }

  /** An open detail reads again when memory changed. */
  sync(cx) {
    if (this.cur && this.cur.stamp !== gwen.versions.memory) this.load(this.cur, cx);
  }

  load(cur, cx) {
    cur.stamp = gwen.versions.memory;
    cx.spawn(async (cx) => {
      try {
        cur.x = await api.ExplainMemory(cur.ref);
        cur.error = null;
      } catch (e) {
        const err = apiError(e);
        cur.x = null;
        cur.error = { gone: err.code === "not_found", message: err.message };
      }
      if (cur.dialog) cur.dialog.title = this.title(cur);
      cx.notify();
    });
  }

  title(cur) {
    const claim = cur.x?.claim;
    return claim ? claim.text : cur.x ? kindLabel(cur.ref.split(":")[0]) : cur.error ? "Not in memory" : "Loading";
  }

  footer(ui, cur) {
    const c = cur.x?.claim;
    if (!c || c.status === "rejected") return [];
    const close = (cx) => ui.closeDialog(cx, cur.id);
    const waiting = toConfirm(c.status);
    return [
      waiting ? ui.button("md-keep", "Keep", (e, cx) => this.keep(c, cx), { tone: "primary", icon: "check" }) : null,
      ui.button("md-edit", waiting ? "Fix" : "Edit", (e, cx) => this.edit(c, cx), { icon: "pencil" }),
      c.status === "active" ? ui.button("md-hide", c.hidden ? "Bring it up again" : "Don't bring this up", (e, cx) => this.hide(c, !c.hidden, cx), { icon: c.hidden ? "eye" : "eye-off" }) : null,
      ui.button("md-reject", "Not true", (e, cx) => this.reject(c, cx, close), { icon: "thumbs-down" }),
      div().flex_1(),
      ui.button("md-forget", "Forget", (e, cx) => this.forget(c, cx, close), { tone: "danger", icon: "trash-2" }),
    ];
  }

  body(ui, cur) {
    const t = ui.t;
    const name = this.name;
    if (cur.error) return cur.error.gone ? ui.muted(`${name} has forgotten this, or it was never kept.`, { size: 14, lh: 20 }) : ui.callout({ tone: "danger", icon: "circle-alert", text: cur.error.message });
    const x = cur.x;
    if (!x) return h_flex().items_center().gap(8).child(ui.spinner(14)).child(ui.muted("Looking it up…", { size: 13 }));

    const c = x.claim ?? null;
    const quote = x.quote || c?.quote || "";
    const sourceMessage = messageIdOf(c ? c.source_ref : cur.ref);
    const history = (x.history ?? []).filter((h) => h.id !== c?.id);
    const chat = c?.thread_id || cur.thread || "";
    const section = (title, content) => v_flex().w_full().gap(8).child(div().text_size(12).font_medium().text_color(t.muted).child(title)).child(content);
    const el = v_flex().w_full().gap(18);

    if (c) {
      // The whole statement (the title bar cuts a long one short), then how sure and since when.
      const head = v_flex().w_full().gap(8);
      if (c.text.length > 60) head.child(ui.text(c.text, { size: 14, lh: 21 }));
      const when = whenText(c);
      head.child(
        h_flex()
          .flex_wrap()
          .items_center()
          .gap(6)
          .children([this.trustBadge(ui, "md-trust", c.trust), ...this.stateBadges(ui, "md-state", c), when ? ui.dim(when) : null].filter(Boolean)),
      );
      if (c.subject) head.child(h_flex().gap(4).child(ui.muted("About", { size: 13 })).child(ui.text(c.subject, { size: 13 })));
      el.child(head);
    }
    if (quote)
      el.child(
        section(
          "In the words it came from",
          div()
            .w_full()
            .border_l(2)
            .border_color(alpha(t.accent, 0.5))
            .pl(12)
            .text_size(13)
            .line_height("20px")
            .italic()
            .text_color(t.muted)
            .child(`“${quote}”`),
        ),
      );
    if (c?.source || chat)
      el.child(
        section(
          "Where it came from",
          h_flex()
            .w_full()
            .flex_wrap()
            .items_center()
            .gap(8)
            .child(this.sourceChip(ui, "md-source", c?.source || "a chat", { thread: chat || undefined, onOpen: (cx) => ui.closeDialog(cx, cur.id) }))
            .when(Boolean(c), (e) => e.child(ui.dim(`learned ${shortDate(c.recorded_at)}`))),
        ),
      );
    if (x.context?.length)
      el.child(
        section(
          "Around it",
          v_flex()
            .w_full()
            .gap(6)
            .p(10)
            .rounded(8)
            .border_1()
            .border_color(t.border)
            .bg(alpha(t.raised, 0.4))
            .children(
              x.context.map((m) => {
                const user = m.role === "user";
                const on = Boolean(m.id) && m.id === sourceMessage;
                const line = h_flex()
                  .w_full()
                  .items_start()
                  .gap(8)
                  .px(9)
                  .py(6)
                  .rounded(6)
                  .child(div().flex_none().w(42).pt(2).text_size(11).font_medium().text_color(user ? t.accent : t.muted).child(user ? "You" : name))
                  .child(div().flex_1().min_w(0).text_size(13).line_height("20px").text_color(on ? t.foreground : t.muted).child(cut(m.text, 600)));
                if (m.at > 0) line.child(div().flex_none().pt(2).text_size(11).text_color(t.dim).child(formatTime(m.at)));
                if (on) line.bg(alpha(t.accent, 0.12)).border_1().border_color(t.accent);
                return line;
              }),
            ),
        ),
      );
    if (x.episode) {
      const ep = x.episode;
      el.child(
        section(
          ep.kind === "resource" ? "The document" : "The note",
          v_flex()
            .w_full()
            .gap(6)
            .p(12)
            .rounded(8)
            .border_1()
            .border_color(t.border)
            .bg(alpha(t.raised, 0.4))
            .when(Boolean(ep.title), (e) => e.child(div().text_size(13).font_medium().text_color(t.foreground).child(ep.title)))
            .child(div().id("md-episode").w_full().max_h(192).overflow_y_scrollbar().child(div().w_full().text_size(13).line_height("20px").text_color(t.muted).child(cut(ep.body, 1200))))
            .child(div().text_size(11).text_color(t.dim).child(shortDate(ep.at))),
        ),
      );
    }
    if (history.length)
      el.child(
        section(
          "History",
          v_flex()
            .w_full()
            .gap(6)
            .children(
              history.map((h) =>
                h_flex()
                  .w_full()
                  .items_start()
                  .gap(6)
                  .text_size(13)
                  .line_height("20px")
                  .child(div().flex_none().text_color(t.dim).child(h.status === "rejected" ? "Not true:" : h.status === "superseded" ? "Before:" : "Also:"))
                  .child(div().flex_1().min_w(0).text_color(t.muted).child(historyLine(h))),
              ),
            ),
        ),
      );
    if (c?.status === "rejected") el.child(ui.callout({ text: `You said this isn't true, so ${name} doesn't use it.` }));
    return el;
  }
}
