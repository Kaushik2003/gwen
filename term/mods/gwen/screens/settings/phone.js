// Settings › Phone: Gwen on the phone over your tailnet (plan.md 3.14), and
// nudges on the phone through ntfy.

import { div } from "gpui-kit";
import { v_flex, h_flex } from "gpui-base";
import { api, apiError, gwen } from "../../lib/gwen.js";
import { host } from "../../lib/host.js";
import { formatAgo } from "../../lib/format.js";
import { TextField } from "../../lib/kit.js";
import { ConfigForm } from "./form.js";
import { Credential } from "./credential.js";
import { checkStep, chip, fromLines, numberedStep, qrCode, sameList } from "./connect-parts.js";

const ntfyFields = [
  { key: "server", label: "Server", hint: "ntfy.sh, or your own on the Pi.", kind: { type: "text", placeholder: "https://ntfy.sh" } },
  { key: "fallback_server", label: "Fallback server", hint: "Tried when the first one fails. Empty for none.", kind: { type: "text", placeholder: "http://raspberrypi:8080" } },
  { key: "topic", label: "Topic", hint: "Works like a password: anyone who knows it can read your nudges.", kind: { type: "text" } },
];

const tsWords = { missing: "Tailscale isn't installed on this laptop.", stopped: "Tailscale isn't running.", logged_out: "Tailscale is signed out." };

export default class Phone {
  constructor(app, cx) {
    this.app = app;
    this.st = null;
    this.unavailable = false;
    this.devices = [];
    this.pair = null; // {code: {url, expires_at}}
    this.busy = "";
    this.loaded = null;
    this.result = null; // the last test notification's answer
    this.testing = false;
    this.roots = new TextField({ multiline: true, rows: 2, grow: [2, 8], onChange: (v, cx) => cx.notify() });
    this.extra = new TextField({ multiline: true, rows: 2, grow: [2, 8], placeholder: "~/data", onChange: (v, cx) => cx.notify() });
    this.rootsFrom = this.extraFrom = null;
    this.sandboxKey = new Credential(app, "claude_sandbox_token", "Sandbox sign-in", { onSaved: (cx) => this.load(cx) });
    this.ntfyToken = new Credential(app, "ntfy_token", "ntfy access token", { hint: "Only for a server that needs one, such as your own with access control." });
    this.ntfy = new ConfigForm(app, { title: "ntfy server", name: "ntfy", fields: ntfyFields });
    // A pairing code lasts ten minutes and works once: count it down.
    cx.timer.every(1000, (cx) => {
      if (!this.pair) return;
      if (this.pair.code.expires_at - Date.now() <= 0) this.pair = null;
      cx.notify();
    });
  }

  sync(cx) {
    const remote = gwen.config?.remote;
    if (remote) {
      const roots = (remote.roots ?? []).join("\n");
      if (this.rootsFrom !== roots && !this.roots.focused) {
        this.rootsFrom = roots;
        this.roots.set(roots);
      }
      const extra = (remote.sandbox_extra ?? []).join("\n");
      if (this.extraFrom !== extra && !this.extra.focused) {
        this.extraFrom = extra;
        this.extra.set(extra);
      }
    }
    this.ntfy.sync?.(cx);
    // The page shows the real state of Tailscale and of Claude Code's settings file.
    const stamp = `${gwen.versions.remote}/${remote?.enabled}`;
    if (this.loaded !== stamp) {
      this.loaded = stamp;
      this.load(cx);
      this.sandboxKey.load(cx);
    }
  }

  load(cx) {
    cx.spawn(async (cx) => {
      try {
        this.st = await api.RemoteStatus();
        this.unavailable = false;
      } catch (e) {
        if (apiError(e).code === "unavailable") this.unavailable = true;
        else gwen.fail(e);
      }
      const before = this.devices.length;
      this.devices = (await api.RemoteDevices().catch(() => null))?.devices ?? this.devices;
      if (this.devices.length !== before) this.pair = null; // a phone used the code
      cx.notify();
    });
  }

  /** Runs one step with its button busy; a failure shows as a toast. */
  step(name, cx, body) {
    this.busy = name;
    cx.notify();
    this.app.spawn(cx, async (cx) => {
      try {
        await body(cx);
      } finally {
        this.busy = "";
      }
    });
  }

  /**
   * Turns Gwen on the phone on or off: sets [remote] enabled and has
   * Tailscale Serve publish, or stop publishing, the remote socket. Tailscale
   * needs root to serve a Unix socket, so the desktop asks for the password
   * once (plan.md 3.14.4).
   */
  publish(on, cx) {
    this.step("publish", cx, async () => {
      if (on) await api.PatchConfig({ remote: { enabled: true } });
      let st = await api.RemoteStatus();
      const port = `--https=${st.port}`;
      if (on && st.tailscale !== "ok") throw new Error("Tailscale isn't running on this laptop. Start it, then turn this on again.");
      if (on && !st.published) await host.tailscaleServe(["--bg", port, `unix:${st.socket}`]);
      if (!on && st.published) await host.tailscaleServe([port, "off"]);
      if (!on) await api.PatchConfig({ remote: { enabled: false } });
      st = await api.RemoteStatus();
      this.st = st;
      gwen.toast(on ? (st.published ? "Your phone can reach this laptop" : "Turned on, but Tailscale isn't publishing it yet") : "Your phone can no longer reach this laptop", on && !st.published ? "info" : "success");
    });
  }

  showCode(cx) {
    this.step("pair", cx, async () => {
      this.pair = { code: await api.RemotePairCode() };
    });
  }

  revoke(dev, cx) {
    this.app.spawn(cx, async (cx) => {
      if (!(await this.app.ui.confirm({ title: `Unpair ${dev.name}?`, body: "It is signed out at once. Pair it again with a new QR code.", confirm: "Unpair", danger: true }, cx))) return;
      await api.RevokeRemoteDevice(dev.id);
      this.load(cx);
    });
  }

  patch(values, said, cx) {
    this.app.spawn(cx, async () => {
      if (await gwen.act(() => api.PatchConfig({ remote: values }))) gwen.toast(said, "success");
    });
  }

  newTopic(cx) {
    this.app.spawn(cx, async (cx) => {
      const ok = await this.app.ui.confirm(
        { title: "Make a new topic?", body: "Your phone stops getting nudges until you subscribe to the new topic in the ntfy app. Do this if the old one may have leaked.", confirm: "Make new topic" },
        cx,
      );
      if (!ok) return;
      if (await gwen.act(() => api.PatchConfig({ ntfy: { topic: host.newTopic() } }))) gwen.toast("New topic made. Subscribe to it on your phone.", "success");
    });
  }

  // Gwen on the phone ---------------------------------------------------------------

  remotePanels(ui) {
    const t = ui.t;
    const remote = gwen.config.remote;
    const st = this.st;
    const tsOK = st?.tailscale === "ok";
    const on = remote.enabled && Boolean(st?.published);

    const reach = ui
      .panel({ title: "Gwen on your phone", icon: "smartphone" })
      .child(
        ui.muted(
          "See your Claude sessions and terminals from your phone, type into them, answer Claude's questions, and review what changed. It travels over your own tailnet only: nothing goes through Claude's app or anyone else's servers.",
          { size: 13 },
        ),
      )
      .child(
        ui.toggleRow(
          "ph-use",
          "Use Gwen from my phone",
          on ? h_flex().gap(6).child(ui.muted("On at")).child(chip(ui, st?.url ?? "")) : "Publishes Gwen to your tailnet. Asks for your password once.",
          on,
          (v, cx) => this.publish(v, cx),
          { disabled: this.busy === "publish" || !st },
        ),
      );
    if (remote.enabled && st && !st.published && tsOK)
      reach.child(
        ui.callout({
          tone: "warn",
          text: "Gwen is ready, but Tailscale isn't publishing it. This happens after Tailscale's settings are reset.",
          action: ui.button("ph-publish", "Publish", (e, cx) => this.publish(true, cx), { small: true, tone: "primary", busy: this.busy === "publish" }),
        }),
      );
    reach.child(
      v_flex()
        .child(checkStep(ui, tsOK, tsOK ? `Tailscale is running as ${st?.host}` : "Tailscale", !tsOK && st ? (tsWords[st.tailscale] ?? "Tailscale isn't ready.") : null))
        .child(
          checkStep(
            ui,
            Boolean(st?.certificates),
            "HTTPS certificates",
            st?.certificates
              ? "On. The laptop's tailnet name appears in the public certificate logs, as with every HTTPS certificate."
              : "Your tailnet needs HTTPS certificates turned on, or the phone's browser won't open the page or allow the microphone. Turning them on lists this laptop's tailnet name in the public certificate logs.",
            !st?.certificates ? ui.button("ph-certs", "Turn on", (e, cx) => cx.open_url("https://login.tailscale.com/admin/dns"), { small: true, icon: "external-link" }) : null,
          ),
        )
        .when(Boolean(st?.owner), (e) => e.child(checkStep(ui, true, `Only ${st.owner} gets in`, "Tailscale tells Gwen who is calling. Anyone else on your tailnet is refused, and so is every phone you haven't paired."))),
    );

    const phones = ui.panel({
      title: "Paired phones",
      icon: "qr-code",
      actions: ui.button("ph-pair", "Pair a phone", (e, cx) => this.showCode(cx), { small: true, tone: "primary", icon: "qr-code", busy: this.busy === "pair", disabled: !on }),
    });
    if (!on) phones.child(ui.muted("Turn on “Use Gwen from my phone” first.", { size: 13 }));
    if (this.pair) {
      const ms = this.pair.code.expires_at - Date.now();
      const left = `${Math.floor(ms / 60000)}:${String(Math.floor((ms % 60000) / 1000)).padStart(2, "0")}`;
      phones.child(
        h_flex()
          .items_center()
          .gap(18)
          .p(14)
          .rounded(8)
          .border_1()
          .border_color(t.border)
          .bg(t.raised)
          .child(qrCode(ui, this.pair.code.url, 176) ?? div())
          .child(
            v_flex()
              .flex_1()
              .min_w(0)
              .gap(6)
              .child(div().text_size(13.5).font_medium().child("Scan this with your phone's camera"))
              .child(ui.muted(`The phone must be on your tailnet (Tailscale on). The code works once and for ${left} more.`, { size: 13 }))
              .child(chip(ui, this.pair.code.url, { wrap: true, size: 11.5 }))
              .child(
                h_flex().child(
                  ui.button(
                    "ph-hide",
                    "Hide",
                    (e, cx) => {
                      this.pair = null;
                      cx.notify();
                    },
                    { small: true },
                  ),
                ),
              ),
          ),
      );
    }
    if (this.devices.length === 0) {
      if (on && !this.pair) phones.child(ui.muted("No phone is paired yet.", { size: 13 }));
    } else
      phones.child(
        v_flex().children(
          this.devices.map((dev) =>
            h_flex()
              .items_center()
              .gap(12)
              .py(9)
              .border_b(1)
              .border_color(t.border)
              .child(ui.icon("smartphone", 16, t.dim))
              .child(
                v_flex()
                  .flex_1()
                  .min_w(0)
                  .child(div().truncate().text_size(13.5).font_medium().child(dev.name))
                  .child(ui.dim(dev.last_seen_at != null ? `Last used ${formatAgo(dev.last_seen_at, Date.now())}` : "Not used yet")),
              )
              .child(ui.button(`ph-unpair-${dev.id}`, "Unpair", (e, cx) => this.revoke(dev, cx), { small: true, tone: "ghost", icon: "trash-2" })),
          ),
        ),
      );
    phones.child(ui.muted("A paired phone can type into any terminal on this laptop, so treat it like the laptop itself, unlocked. Unpair a phone you lose.", { size: 13 }));

    const term = !st?.terminal
      ? "Not installed. In Gwen's folder, run make term."
      : st.core === "outdated"
        ? "Gwen's terminal core is out of date. In Gwen's folder, run make term."
        : st.core === "ok"
          ? `Installed. Its core (${st.core_version}) is running, and every Claude you start in it shows up on your phone.`
          : "Installed. It starts when you open it, or when you start a session from your phone.";
    const sessions = ui
      .panel({ title: "Claude sessions", icon: "square-terminal" })
      .child(checkStep(ui, Boolean(st?.terminal) && st.core !== "outdated", "Gwen's terminals", term))
      .child(
        ui.toggleRow(
          "ph-hooks",
          "Let Gwen see my Claude sessions",
          st?.hooks_note ||
            "Adds Gwen's hooks to Claude Code's settings (~/.claude/settings.json), keeping everything else in the file. Without them Gwen still sees whether Claude is working, but not what it asks, says, or edits.",
          Boolean(st?.hooks),
          (v, cx) =>
            this.step("hooks", cx, async () => {
              this.st = await api.SetRemoteHooks(v);
            }),
          { disabled: this.busy === "hooks" || !st, divider: false },
        ),
      );

    const roots = fromLines(this.roots.value);
    const extra = fromLines(this.extra.value);
    const sandboxBody = `A sandboxed Claude sees only its project folder and your toolchains. It gets a private Claude folder of its own, so it can't see your other conversations or change your Claude's settings, and it signs in separately so it never touches your Claude's sign-in.${st && !st.sandbox ? " The sandbox needs bubblewrap, which isn't installed." : ""}`;
    this.sandboxKey.o.hint = `${st?.sandbox_signed_in ? "Signed in. " : "Not signed in yet: each sandboxed project would ask you to log in. "}In a terminal, run claude setup-token and paste the token it shows here. That token can only talk to the model. Or run gwen sandbox-login, which does both.`;
    const may = ui
      .panel({ title: "What the phone may do", icon: "shield-check" })
      .child(
        ui.field(
          "Folders the phone may start a session in",
          v_flex()
            .gap(8)
            .child(this.roots.el(ui, { mono: true }))
            .when(!sameList(roots, remote.roots ?? []), (e) =>
              e.child(h_flex().child(ui.button("ph-roots", "Save folders", (ev, cx) => this.patch({ roots }, "Saved the folders", cx), { small: true, tone: "primary", disabled: roots.length === 0 }))),
            ),
          "One per line. Hidden folders and Gwen's own files are never offered.",
        ),
      )
      .child(
        ui.toggleRow(
          "ph-push",
          "Push to my phone when I'm away",
          "When a session needs you, finishes, or stops with an error while the laptop is locked or idle. The push names the folder only: never a command, a path, or code.",
          remote.push,
          (v, cx) => this.patch({ push: v }, v ? "Pushes are on" : "Pushes are off", cx),
        ),
      )
      .child(
        ui.row(
          "Keep the laptop awake",
          "So a session isn't cut off by sleep. Gwen lets go below 15% battery.",
          ui.select(
            "ph-awake",
            [
              { value: "working", label: "While a session works" },
              { value: "working_lid", label: "Also with the lid closed" },
              { value: "never", label: "Never" },
            ],
            remote.keep_awake,
            (v, cx) => this.patch({ keep_awake: v }, "Saved", cx),
            { width: 220 },
          ),
        ),
      )
      .child(v_flex().gap(8).child(div().text_size(13.5).font_medium().child("Sandboxed sessions")).child(ui.muted(sandboxBody, { size: 13 })).child(this.sandboxKey.el(ui)))
      .child(
        ui.field(
          "More folders a sandboxed session may see",
          v_flex()
            .gap(8)
            .child(this.extra.el(ui, { mono: true }))
            .when(!sameList(extra, remote.sandbox_extra ?? []), (e) => e.child(h_flex().child(ui.button("ph-extra", "Save sandbox folders", (ev, cx) => this.patch({ sandbox_extra: extra }, "Saved the sandbox folders", cx), { small: true, tone: "primary" })))),
          "One per line; end a line with :rw to let Claude write there.",
        ),
      );
    return [reach, phones, sessions, may];
  }

  // Nudges on the phone -------------------------------------------------------------

  ntfyPanels(ui) {
    const c = gwen.config;
    const url = `${c.ntfy.server.replace(/\/+$/, "")}/${c.ntfy.topic}`;
    const panel = ui.panel({ title: "Nudges on your phone", icon: "smartphone" });
    if (!c.nudge.phone)
      panel.child(
        ui.callout({
          tone: "warn",
          text: "Phone notifications are off.",
          action: ui.button("ph-ntfy-on", "Turn on", (e, cx) => this.app.spawn(cx, () => gwen.act(() => api.PatchConfig({ nudge: { phone: true } }))), { small: true }),
        }),
      );
    const steps = v_flex()
      .flex_1()
      .min_w(0)
      .gap(16)
      .child(numberedStep(ui, 1, "Install the free ntfy app. Android: Play Store or F-Droid. iOS: App Store."))
      .child(
        numberedStep(
          ui,
          2,
          "Tap + and subscribe to this topic, or scan the code.",
          h_flex()
            .items_center()
            .gap(8)
            .child(chip(ui, url))
            .child(
              ui.button(
                "ph-ntfy-copy",
                "Copy",
                (e, cx) => {
                  cx.write_to_clipboard(url);
                  gwen.toast("Copied the topic link", "success");
                },
                { small: true, icon: "copy" },
              ),
            ),
        ),
      )
      .child(
        numberedStep(
          ui,
          3,
          "Send a test to check that it arrives.",
          h_flex()
            .flex_wrap()
            .items_center()
            .gap(8)
            .child(
              ui.button(
                "ph-ntfy-test",
                "Send test",
                (e, cx) => {
                  this.testing = true;
                  this.app.spawn(cx, async () => {
                    this.result = (await gwen.act(() => api.NotifyTest())) ?? null;
                    this.testing = false;
                  });
                },
                { small: true, icon: "send", busy: this.testing },
              ),
            )
            .when(Boolean(this.result), (e) =>
              e.child(ui.badge(`Desktop: ${this.result.desktop}`, { tone: this.result.desktop === "sent" ? "ok" : "neutral" })).child(ui.badge(`Phone: ${this.result.phone}`, { tone: this.result.phone === "sent" ? "ok" : "neutral" })),
            ),
        ),
      );
    panel
      .child(h_flex().items_start().gap(24).child(qrCode(ui, url, 176) ?? div()).child(steps))
      .child(h_flex().justify_end().pt(12).border_t(1).border_color(ui.t.border).child(ui.button("ph-ntfy-new", "Make a new topic", (e, cx) => this.newTopic(cx), { icon: "refresh-cw" })));
    const ntfy = this.ntfy.render(ui);
    return [panel, ...(Array.isArray(ntfy) ? ntfy : [ntfy]), ui.panel({ title: "Access token" }).child(this.ntfyToken.el(ui))];
  }

  render(ui) {
    if (!gwen.config) return ui.splash("Loading…", { busy: true });
    const parts = [];
    if (!this.unavailable && gwen.config.remote) parts.push(...this.remotePanels(ui));
    parts.push(...this.ntfyPanels(ui));
    return parts;
  }
}
