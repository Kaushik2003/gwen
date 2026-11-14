import QRCode from "qrcode";
import { CircleAlert, CircleCheck, ExternalLink, QrCode, ShieldCheck, Smartphone, SquareTerminal, Trash2 } from "lucide-react";
import { useCallback, useEffect, useState, type ReactNode } from "react";
import { App, apiError, onEvent, wire } from "../api";
import { useConfirm, useToast } from "../components/feedback";
import { Button, Callout, Panel, Select, TextArea, ToggleRow } from "../components/ui";
import { useDaemon } from "../daemon";
import { formatAgo } from "../format";
import { CredentialField } from "./Credential";

const lines = (list: string[]) => list.join("\n");
const fromLines = (text: string) => text.split("\n").map((l) => l.trim()).filter(Boolean);
const same = (a: string[], b: string[]) => a.length === b.length && a.every((v, i) => v === b[i]);

/** One row of the setup list: a state, what it means, and what to do about it. */
function Step({ ok, title, children, action }: { ok: boolean; title: string; children?: ReactNode; action?: ReactNode }) {
  return (
    <div className="flex items-start gap-3 py-2.5">
      {ok ? <CircleCheck size={17} className="mt-0.5 shrink-0 text-working" aria-hidden /> : <CircleAlert size={17} className="mt-0.5 shrink-0 text-idle" aria-hidden />}
      <div className="min-w-0 flex-1">
        <div className="text-sm font-medium text-ink">{title}</div>
        {children && <div className="mt-0.5 text-[13px] leading-relaxed text-ink-subtle">{children}</div>}
      </div>
      {action}
    </div>
  );
}

/** Settings › Phone: use Gwen from your phone over your tailnet (plan.md 3.14). */
export default function RemoteSettings() {
  const d = useDaemon();
  const notify = useToast();
  const confirm = useConfirm();
  const remote = d.config?.remote;
  const [st, setSt] = useState<wire.RemoteStatus | null>(null);
  const [unavailable, setUnavailable] = useState(false);
  const [devices, setDevices] = useState<wire.RemoteDevice[]>([]);
  const [pair, setPair] = useState<{ code: wire.RemotePairCode; image: string } | null>(null);
  const [left, setLeft] = useState("");
  const [busy, setBusy] = useState("");
  const [roots, setRoots] = useState(lines(remote?.roots ?? []));
  const [extra, setExtra] = useState(lines(remote?.sandbox_extra ?? []));

  const load = useCallback(() => {
    App.RemoteStatus().then(
      (s) => (setSt(s), setUnavailable(false)),
      (e) => (apiError(e).code === "unavailable" ? setUnavailable(true) : d.fail(e)),
    );
    App.RemoteDevices().then((l) => setDevices(l.devices), () => {});
  }, [d.fail]);
  // The page shows the real state of Tailscale and of Claude Code's settings file: read it whenever the page opens or something changes.
  useEffect(load, [load, remote?.enabled]);
  useEffect(() => onEvent("remote_changed", load), [load]);
  useEffect(() => setRoots(lines(remote?.roots ?? [])), [remote?.roots]);
  useEffect(() => setExtra(lines(remote?.sandbox_extra ?? [])), [remote?.sandbox_extra]);

  // A pairing code lasts ten minutes and works once: count it down, and take it away when a phone has used it.
  useEffect(() => {
    if (!pair) return;
    const tick = () => {
      const ms = pair.code.expires_at - Date.now();
      if (ms <= 0) return setPair(null);
      setLeft(`${Math.floor(ms / 60000)}:${String(Math.floor((ms % 60000) / 1000)).padStart(2, "0")}`);
    };
    tick();
    const t = window.setInterval(tick, 1000);
    return () => window.clearInterval(t);
  }, [pair]);
  const paired = devices.length;
  useEffect(() => setPair(null), [paired]);

  if (unavailable || !remote) return null;

  async function run<T>(name: string, fn: () => Promise<T>): Promise<T | undefined> {
    setBusy(name);
    try {
      return await fn();
    } catch (e) {
      d.fail(e);
    } finally {
      setBusy("");
    }
  }
  const publish = async (on: boolean) => {
    const s = await run("publish", () => App.PublishRemote(on));
    if (s) {
      setSt(s);
      notify(on ? (s.published ? "Your phone can reach this laptop" : "Turned on, but Tailscale isn't publishing it yet") : "Your phone can no longer reach this laptop", on && !s.published ? "info" : "success");
    } else load();
  };
  const showCode = async () => {
    const code = await run("pair", () => App.RemotePairCode());
    if (code) setPair({ code, image: await QRCode.toDataURL(code.url, { margin: 1, width: 440, errorCorrectionLevel: "M" }) });
  };
  const revoke = async (dev: wire.RemoteDevice) => {
    if (!(await confirm({ title: `Unpair ${dev.name}?`, body: "It is signed out at once. Pair it again with a new QR code.", confirm: "Unpair", danger: true }))) return;
    await run("revoke", () => App.RevokeRemoteDevice(dev.id));
    load();
  };
  const patch = (values: Record<string, unknown>, said: string) =>
    d.act(() => App.PatchConfig({ remote: values })).then((ok) => {
      if (ok) notify(said, "success");
    });

  const tsOK = st?.tailscale === "ok";
  const tsWords: Record<string, string> = { missing: "Tailscale isn't installed on this laptop.", stopped: "Tailscale isn't running.", logged_out: "Tailscale is signed out." };
  const on = remote.enabled && !!st?.published;
  const now = Date.now();

  return (
    <>
      <Panel title="Gwen on your phone" icon={Smartphone}>
        <p className="-mt-1 mb-3 text-[13px] leading-relaxed text-ink-subtle">
          See your Claude sessions and terminals from your phone, type into them, answer Claude's questions, and review what changed. It travels over your own tailnet only: nothing
          goes through Claude's app or anyone else's servers.
        </p>
        <ToggleRow
          title="Use Gwen from my phone"
          description={on ? <>On at <span className="font-mono text-[12.5px] text-ink">{st?.url}</span></> : "Publishes Gwen to your tailnet. Asks for your password once."}
          checked={on}
          disabled={busy === "publish" || !st}
          onChange={publish}
        />
        {remote.enabled && st && !st.published && tsOK && (
          <Callout tone="idle" className="mt-3" action={<Button size="sm" tone="primary" busy={busy === "publish"} onClick={() => publish(true)}>Publish</Button>}>
            Gwen is ready, but Tailscale isn't publishing it. This happens after Tailscale's settings are reset.
          </Callout>
        )}
        <div className="mt-3 divide-y divide-line border-t border-line">
          <Step ok={tsOK} title={tsOK ? `Tailscale is running as ${st?.host}` : "Tailscale"}>
            {!tsOK && st && (tsWords[st.tailscale] ?? "Tailscale isn't ready.")}
          </Step>
          <Step
            ok={!!st?.certificates}
            title="HTTPS certificates"
            action={!st?.certificates && <Button size="sm" icon={ExternalLink} onClick={() => d.act(() => App.OpenURL("https://login.tailscale.com/admin/dns"))}>Turn on</Button>}
          >
            {st?.certificates
              ? "On. The laptop's tailnet name appears in the public certificate logs, as with every HTTPS certificate."
              : "Your tailnet needs HTTPS certificates turned on, or the phone's browser won't open the page or allow the microphone. Turning them on lists this laptop's tailnet name in the public certificate logs."}
          </Step>
          {st?.owner && (
            <Step ok title={`Only ${st.owner} gets in`}>
              Tailscale tells Gwen who is calling. Anyone else on your tailnet is refused, and so is every phone you haven't paired.
            </Step>
          )}
        </div>
      </Panel>

      <Panel
        title="Paired phones"
        icon={QrCode}
        actions={<Button size="sm" tone="primary" icon={QrCode} busy={busy === "pair"} disabled={!on} onClick={showCode}>Pair a phone</Button>}
      >
        {!on && <p className="text-[13px] text-ink-subtle">Turn on “Use Gwen from my phone” first.</p>}
        {pair && (
          <div className="mb-4 flex flex-wrap items-center gap-5 rounded-lg border border-line bg-surface-2 p-4">
            <img src={pair.image} alt="QR code that pairs a phone" width={176} height={176} className="rounded-md bg-white p-1" />
            <div className="min-w-0 flex-1 text-[13px] leading-relaxed text-ink-subtle">
              <div className="text-sm font-medium text-ink">Scan this with your phone's camera</div>
              The phone must be on your tailnet (Tailscale on). The code works once and for {left} more.
              <div className="mt-2 font-mono text-[11.5px] break-all text-ink-faint">{pair.code.url}</div>
              <Button size="sm" className="mt-3" onClick={() => setPair(null)}>Hide</Button>
            </div>
          </div>
        )}
        {devices.length === 0
          ? on && !pair && <p className="text-[13px] text-ink-subtle">No phone is paired yet.</p>
          : (
            <ul className="divide-y divide-line">
              {devices.map((dev) => (
                <li key={dev.id} className="flex items-center gap-3 py-2.5">
                  <Smartphone size={16} className="shrink-0 text-ink-faint" aria-hidden />
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm font-medium text-ink">{dev.name}</div>
                    <div className="text-xs text-ink-subtle">{dev.last_seen_at != null ? `Last used ${formatAgo(dev.last_seen_at, now)}` : "Not used yet"}</div>
                  </div>
                  <Button size="sm" tone="ghost" icon={Trash2} onClick={() => revoke(dev)}>Unpair</Button>
                </li>
              ))}
            </ul>
          )}
        <p className="mt-3 text-[13px] leading-relaxed text-ink-subtle">
          A paired phone can type into any terminal on this laptop, so treat it like the laptop itself, unlocked. Unpair a phone you lose.
        </p>
      </Panel>

      <Panel title="Claude sessions" icon={SquareTerminal}>
        <div className="divide-y divide-line">
          <Step ok={!!st?.terminal && st.core !== "outdated"} title="Gwen Terminal">
            {!st?.terminal
              ? <>Not installed. In Gwen's folder, run <span className="font-mono text-[12.5px] text-ink">make term</span>.</>
              : st.core === "outdated"
                ? <>Gwen Terminal's core is out of date. Run <span className="font-mono text-[12.5px] text-ink">make term</span>.</>
                : st.core === "ok"
                  ? `Installed. Its core (${st.core_version}) is running, and every Claude you start in it shows up on your phone.`
                  : "Installed. It starts when you open it, or when you start a session from your phone."}
          </Step>
        </div>
        <div className="mt-2 border-t border-line pt-3">
          <ToggleRow
            title="Let Gwen see my Claude sessions"
            description={
              st?.hooks_note || "Adds Gwen's hooks to Claude Code's settings (~/.claude/settings.json), keeping everything else in the file. Without them Gwen still sees whether Claude is working, but not what it asks, says, or edits."
            }
            checked={!!st?.hooks}
            disabled={busy === "hooks" || !st}
            onChange={async (v) => {
              const s = await run("hooks", () => App.SetRemoteHooks(v));
              if (s) setSt(s);
            }}
          />
        </div>
      </Panel>

      <Panel title="What the phone may do" icon={ShieldCheck}>
        <div className="flex flex-col gap-5">
          <div>
            <label htmlFor="remote-roots" className="text-sm font-medium text-ink">Folders the phone may start a session in</label>
            <p className="mt-0.5 mb-2 text-[13px] text-ink-subtle">One per line. Hidden folders and Gwen's own files are never offered.</p>
            <TextArea id="remote-roots" rows={Math.max(2, fromLines(roots).length + 1)} value={roots} onChange={(e) => setRoots(e.target.value)} className="font-mono text-[13px]" spellCheck={false} />
            {!same(fromLines(roots), remote.roots) && (
              <Button size="sm" tone="primary" className="mt-2" disabled={fromLines(roots).length === 0} onClick={() => patch({ roots: fromLines(roots) }, "Saved the folders")}>Save folders</Button>
            )}
          </div>
          <ToggleRow
            title="Push to my phone when I'm away"
            description="When a session needs you, finishes, or stops with an error while the laptop is locked or idle. The push names the folder only: never a command, a path, or code."
            checked={remote.push}
            onChange={(v) => patch({ push: v }, v ? "Pushes are on" : "Pushes are off")}
          />
          <div className="flex items-start justify-between gap-6">
            <div className="min-w-0">
              <label htmlFor="remote-awake" className="text-sm font-medium text-ink">Keep the laptop awake</label>
              <p className="mt-0.5 text-[13px] text-ink-subtle">So a session isn't cut off by sleep. Gwen lets go below 15% battery.</p>
            </div>
            <Select id="remote-awake" className="w-56 shrink-0" value={remote.keep_awake} onChange={(e) => patch({ keep_awake: e.target.value }, "Saved")}>
              <option value="working">While a session works</option>
              <option value="working_lid">Also with the lid closed</option>
              <option value="never">Never</option>
            </Select>
          </div>
          <div>
            <div className="text-sm font-medium text-ink">Sandboxed sessions</div>
            <p className="mt-0.5 mb-3 text-[13px] leading-relaxed text-ink-subtle">
              A sandboxed Claude sees only its project folder and your toolchains. It gets a private Claude folder of its own, so it can't see your other conversations or change your Claude's
              settings, and it signs in separately so it never touches your Claude's sign-in.
              {st && !st.sandbox && " The sandbox needs bubblewrap, which isn't installed."}
            </p>
            <CredentialField
              name="claude_sandbox_token"
              label="Sandbox sign-in"
              onSaved={load}
              hint={
                <>
                  {st?.sandbox_signed_in ? "Signed in. " : "Not signed in yet: each sandboxed project would ask you to log in. "}
                  In a terminal, run <span className="font-mono text-[12.5px] text-ink">claude setup-token</span> and paste the token it shows here. That token can only talk to the model. Or run{" "}
                  <span className="font-mono text-[12.5px] text-ink">gwen sandbox-login</span>, which does both.
                </>
              }
            />
          </div>
          <div>
            <label htmlFor="remote-extra" className="text-sm font-medium text-ink">More folders a sandboxed session may see</label>
            <p className="mt-0.5 mb-2 text-[13px] leading-relaxed text-ink-subtle">
              One per line; end a line with <span className="font-mono text-[12.5px] text-ink">:rw</span> to let Claude write there.
            </p>
            <TextArea id="remote-extra" rows={Math.max(2, fromLines(extra).length + 1)} value={extra} onChange={(e) => setExtra(e.target.value)} placeholder="~/data" className="font-mono text-[13px]" spellCheck={false} />
            {!same(fromLines(extra), remote.sandbox_extra) && (
              <Button size="sm" tone="primary" className="mt-2" onClick={() => patch({ sandbox_extra: fromLines(extra) }, "Saved the sandbox folders")}>Save sandbox folders</Button>
            )}
          </div>
        </div>
      </Panel>
    </>
  );
}
