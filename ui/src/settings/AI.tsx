import { ArrowLeftRight, CircleAlert, CircleCheck, CircleOff, Copy, KeyRound, LoaderCircle, Sparkles, type LucideIcon } from "lucide-react";
import { useEffect, useState } from "react";
import { App } from "../api";
import { planName } from "../components/ai";
import DurationInput from "../components/DurationInput";
import { useToast } from "../components/feedback";
import { Badge, Button, Callout, Field, Input, Panel, cx } from "../components/ui";
import { useDaemon } from "../daemon";
import { parseDuration } from "../format";
import { useNav } from "../nav";
import type { main } from "../wailsjs/go/models";
import { ClipboardSetText } from "../wailsjs/runtime/runtime";
import { useHasCredential } from "./Credential";

const providers: { id: string; title: string; body: string; icon: LucideIcon }[] = [
  { id: "claude_code", title: "Claude Code", body: "Your Claude Pro or Max plan, through the Claude Code app. No API key.", icon: Sparkles },
  { id: "anthropic", title: "Anthropic API", body: "Pay per request, with an API key from the Anthropic Console.", icon: KeyRound },
  { id: "openai_compatible", title: "OpenAI-compatible", body: "Ollama, LM Studio, or any server with a /v1 API, on this computer or the Pi.", icon: ArrowLeftRight },
  { id: "none", title: "Off", body: "No AI features.", icon: CircleOff },
];

/** Settings → AI (docs/08-clients.md#v3-additions): provider, model, and a live sign-in check for Claude Code. */
export default function AISettings() {
  const d = useDaemon();
  const notify = useToast();
  const { go } = useNav();
  const llm = d.config?.llm;
  const [provider, setProvider] = useState(llm?.provider ?? "none");
  const [model, setModel] = useState(llm?.model ?? "");
  const [endpoint, setEndpoint] = useState(llm?.endpoint ?? "");
  const [command, setCommand] = useState(llm?.command ?? "claude");
  const [timeout, setTimeoutValue] = useState(llm?.timeout ?? "2m");
  const [key, setKey] = useState("");
  const [hasKey, setHasKey] = useHasCredential("llm_api_key");
  const [claude, setClaude] = useState<main.ClaudeCodeStatus | null>(null);
  const [checking, setChecking] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setProvider(llm?.provider ?? "none");
    setModel(llm?.model ?? "");
    setEndpoint(llm?.endpoint ?? "");
    setCommand(llm?.command ?? "claude");
    setTimeoutValue(llm?.timeout ?? "2m");
  }, [llm?.provider, llm?.model, llm?.endpoint, llm?.command, llm?.timeout]);

  async function check(cmd = command) {
    setChecking(true);
    try {
      setClaude(await App.ClaudeCode(cmd.trim() || "claude"));
    } catch (e) {
      d.fail(e);
    }
    setChecking(false);
  }
  // Checked when Claude Code is picked, and again on request.
  useEffect(() => {
    if (provider === "claude_code") check(llm?.command ?? "claude");
  }, [provider]);

  const ready = claude?.is_claude && claude.logged_in;
  async function save() {
    setSaving(true);
    const patch: Record<string, unknown> = { provider, model: model.trim(), timeout };
    if (provider === "openai_compatible") patch.endpoint = endpoint.trim();
    // As gwen setup llm does: the daemon's PATH may not be this window's, so store the full path.
    if (provider === "claude_code") patch.command = claude?.found && claude.is_claude ? claude.command : command.trim();
    const ok = await d.act(() => App.PatchConfig({ llm: patch }));
    if (ok && key.trim() && provider !== "claude_code" && provider !== "none") {
      try {
        await App.SetCredential("llm_api_key", key.trim());
        setKey("");
        setHasKey(true);
      } catch (e) {
        d.fail(e);
        setSaving(false);
        return;
      }
    }
    setSaving(false);
    if (ok) notify(provider === "none" ? "Turned AI off" : "Saved AI settings", "success");
  }

  const dirty =
    provider !== llm?.provider ||
    model.trim() !== (llm?.model ?? "") ||
    timeout !== llm?.timeout ||
    (provider === "openai_compatible" && endpoint.trim() !== (llm?.endpoint ?? "")) ||
    (provider === "claude_code" && (claude?.is_claude && claude.found ? claude.command : command.trim()) !== llm?.command) ||
    key.trim() !== "";

  return (
    <Panel title="AI" icon={Sparkles}>
      <p className="-mt-1 mb-4 text-[13px] leading-relaxed text-ink-subtle">
        Optional. The assistant chats with you and adds, moves, and reschedules your tasks as you ask, plans your day, breaks goals into tasks, and writes a weekly retro from your numbers.
      </p>

      <div role="radiogroup" aria-label="Provider" className="grid gap-2.5 @lg:grid-cols-2">
        {providers.map((p) => {
          const on = p.id === provider;
          const Icon = p.icon;
          return (
            <button
              key={p.id}
              type="button"
              role="radio"
              aria-checked={on}
              onClick={() => setProvider(p.id)}
              className={cx("flex gap-3 rounded-lg border p-3.5 text-left transition-colors", on ? "border-accent bg-accent/10" : "border-line-strong bg-surface-2 hover:border-line-3")}
            >
              <Icon size={18} className={cx("mt-0.5 shrink-0", on ? "text-accent-hover" : "text-ink-subtle")} aria-hidden />
              <span className="min-w-0">
                <span className="flex items-center gap-2 text-sm font-medium text-ink">
                  {p.title}
                  {p.id === llm?.provider && <Badge>In use</Badge>}
                </span>
                <span className="mt-0.5 block text-xs leading-relaxed text-ink-subtle">{p.body}</span>
              </span>
            </button>
          );
        })}
      </div>

      {provider === "claude_code" && (
        <div className="mt-5 flex flex-col gap-4">
          <Field label="Claude Code program" hint="The claude command, or its full path.">
            <div className="flex gap-2">
              <Input value={command} onChange={(e) => setCommand(e.target.value)} placeholder="claude" className="min-w-0 flex-1" spellCheck={false} />
              <Button onClick={() => check()} busy={checking}>
                Check
              </Button>
            </div>
          </Field>
          <ClaudeState
            status={claude}
            checking={checking}
            onUse={(path) => {
              setCommand(path);
              check(path);
            }}
            onCopy={() => {
              ClipboardSetText("claude auth login");
              notify("Copied. Paste it into a terminal, sign in, then check again.", "success");
            }}
          />
        </div>
      )}

      {provider === "openai_compatible" && (
        <div className="mt-5">
          <Field label="Endpoint" hint="Ends in /v1. Ollama on this computer is http://localhost:11434/v1.">
            <Input value={endpoint} placeholder="http://localhost:11434/v1" onChange={(e) => setEndpoint(e.target.value)} spellCheck={false} />
          </Field>
        </div>
      )}

      {provider !== "none" && (
        <div className="mt-5 grid gap-4 @lg:grid-cols-2">
          {provider === "claude_code" ? (
            <Field label="Model" compound className="@lg:col-span-2" hint="An alias such as sonnet or opus, or a full model name.">
              <div className="flex flex-wrap items-center gap-2">
                <Input value={model} onChange={(e) => setModel(e.target.value)} spellCheck={false} aria-label="Model" className="w-64 max-w-full" />
                {["sonnet", "opus", "haiku"].map((m) => (
                  <Button key={m} size="sm" tone={model === m ? "primary" : "secondary"} onClick={() => setModel(m)}>
                    {m}
                  </Button>
                ))}
              </div>
            </Field>
          ) : (
            <Field label="Model" hint={provider === "anthropic" ? "A model name from the Anthropic docs." : "As the server names it, such as llama3.1."}>
              <Input value={model} onChange={(e) => setModel(e.target.value)} spellCheck={false} />
            </Field>
          )}
          {(provider === "anthropic" || provider === "openai_compatible") && (
            <Field
              label="API key"
              hint={
                hasKey ? (
                  <span className="inline-flex items-center gap-1 text-working">
                    <CircleCheck size={12} aria-hidden />
                    A key is saved. Type a new one to replace it.
                  </span>
                ) : provider === "openai_compatible" ? (
                  "Leave empty if the server needs none, as Ollama does."
                ) : (
                  "Stored on this computer, readable only by you."
                )
              }
            >
              <Input type="password" value={key} onChange={(e) => setKey(e.target.value)} autoComplete="off" placeholder={hasKey ? "••••••••" : ""} />
            </Field>
          )}
          <Field
            label="Wait for an answer up to"
            compound
            hint={
              provider === "claude_code" && parseDuration(timeout) < 60_000 ? (
                <span className="text-idle">
                  Short for Claude Code, which often needs a minute or two.{" "}
                  <button type="button" className="underline underline-offset-2 hover:text-ink" onClick={() => setTimeoutValue("2m")}>
                    Use 2 minutes
                  </button>
                </span>
              ) : (
                "Claude Code often needs a minute or two for a long breakdown."
              )
            }
          >
            <DurationInput value={timeout} onChange={setTimeoutValue} units={["m", "s"]} label="Wait for an answer up to" />
          </Field>
        </div>
      )}

      <div className="mt-5 flex flex-wrap items-center justify-end gap-2 border-t border-line pt-4">
        {provider === "claude_code" && claude?.is_claude && !claude.logged_in && !checking && <span className="mr-auto text-xs text-idle">You can save now and sign in later.</span>}
        {llm?.provider !== "none" && !dirty && (provider !== "claude_code" || ready) && (
          <Button icon={Sparkles} onClick={() => go("assistant")}>
            Open the assistant
          </Button>
        )}
        <Button tone="primary" onClick={save} busy={saving} disabled={!dirty}>
          Save changes
        </Button>
      </div>
    </Panel>
  );
}

function ClaudeState({ status, checking, onUse, onCopy }: { status: main.ClaudeCodeStatus | null; checking: boolean; onUse: (path: string) => void; onCopy: () => void }) {
  if (checking || !status)
    return (
      <p className="flex items-center gap-2 text-[13px] text-ink-subtle">
        <LoaderCircle size={14} className="animate-spin" aria-hidden />
        Asking Claude Code whether it is signed in…
      </p>
    );
  if (status.is_claude && status.logged_in)
    return (
      <Callout tone="working" icon={CircleCheck}>
        {status.plan ? `Signed in with your Claude ${planName(status.plan)} plan.` : `Signed in (${status.auth_method || "Claude Code"}).`}{" "}
        <span className="text-ink-subtle">{status.command}</span>
      </Callout>
    );
  if (status.is_claude)
    return (
      <Callout
        tone="idle"
        icon={CircleAlert}
        action={
          <Button size="sm" icon={Copy} onClick={onCopy}>
            Copy command
          </Button>
        }
      >
        Claude Code is installed but not signed in. Run <code className="rounded bg-surface-3 px-1.5 py-0.5 text-ink">claude auth login</code> in a terminal once, then check again.
      </Callout>
    );
  return (
    <Callout
      tone="danger"
      icon={CircleAlert}
      action={
        status.suggested && (
          <Button size="sm" tone="primary" onClick={() => onUse(status.suggested)}>
            Use it
          </Button>
        )
      }
    >
      {status.found ? `${status.command} is not Claude Code.` : `Claude Code was not found as ${status.command}.`}{" "}
      {status.suggested ? (
        <>
          Found Claude Code at <span className="text-ink">{status.suggested}</span>.
        </>
      ) : (
        "Install Claude Code, or give the full path to claude, then check again."
      )}
    </Callout>
  );
}
