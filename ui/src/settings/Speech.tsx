import { CircleAlert, CircleCheck, Cloud, Download, ExternalLink, HardDrive, LoaderCircle, Square, Trash2, Volume2 } from "lucide-react";
import { App } from "../api";
import Face from "../components/Face";
import { useConfirm, useToast } from "../components/feedback";
import { useAloud, useSpeech } from "../components/Speech";
import { Badge, Button, Callout, Field, Panel, Select, ToggleRow, cx } from "../components/ui";
import { DownloadProgress } from "../components/Voice";
import { useDaemon } from "../daemon";
import { CredentialField } from "./Credential";

/** Settings → AI → Voice output: where her voice comes from, its download or API key, which voice, a sample, and reading replies aloud. */
export default function SpeechSettings() {
  const d = useDaemon();
  const notify = useToast();
  const confirm = useConfirm();
  const speech = useSpeech();
  const [aloud, setAloud] = useAloud();
  const name = d.config?.llm.assistant_name || "Gwen";
  const status = speech.status;
  const provider = speech.provider;
  const busy = speech.preparing !== 0 || speech.sounding !== 0;

  async function install() {
    try {
      await speech.install();
      setAloud(true);
      notify(`${name} can talk now`, "success");
    } catch (e) {
      d.fail(e);
    }
  }

  async function remove() {
    const ok = await confirm({
      title: `Remove ${name}'s offline voice?`,
      body: `Frees ${provider?.download_mb ?? 300} MB. You can download it again any time.`,
      confirm: "Remove",
      danger: true,
    });
    if (!ok) return;
    try {
      await speech.remove();
      setAloud(false);
      notify(`Removed ${name}'s offline voice`, "success");
    } catch (e) {
      d.fail(e);
    }
  }

  if (!status || !provider) return null;
  return (
    <Panel title="Voice output" icon={Volume2}>
      <p className="-mt-1 mb-4 text-[13px] leading-relaxed text-ink-subtle">
        Let {name} talk back, in a voice from the internet or one that runs on this computer. With voice input as well, Talk in the assistant makes it a spoken conversation.
      </p>

      {!status.player && (
        <Callout tone="idle" icon={CircleAlert} className="mb-4">
          {name}'s voice plays through PipeWire's player, which isn't installed. Install the <span className="text-ink">pipewire-utils</span> package, then reopen Settings.
        </Callout>
      )}

      <div role="radiogroup" aria-label="Where her voice comes from" className="grid gap-2.5 @lg:grid-cols-2">
        {status.providers.map((p) => {
          const on = p.id === provider.id;
          const Icon = p.online ? Cloud : HardDrive;
          return (
            <button
              key={p.id}
              type="button"
              role="radio"
              aria-checked={on}
              onClick={() => speech.setProvider(p.id)}
              className={cx("flex gap-3 rounded-lg border p-3.5 text-left transition-colors", on ? "border-accent bg-accent/10" : "border-line-strong bg-surface-2 hover:border-line-3")}
            >
              <Icon size={18} className={cx("mt-0.5 shrink-0", on ? "text-accent-hover" : "text-ink-subtle")} aria-hidden />
              <span className="min-w-0">
                <span className="flex items-center gap-2 text-sm font-medium text-ink">
                  {p.name}
                  {p.ready && (
                    <Badge tone="working" icon={CircleCheck}>
                      Ready
                    </Badge>
                  )}
                </span>
                <span className="mt-0.5 block text-xs leading-relaxed text-ink-subtle">{p.about}</span>
              </span>
            </button>
          );
        })}
      </div>

      <div className="mt-5 flex flex-col gap-4">
        {provider.key && (
          <CredentialField
            key={provider.key}
            name={provider.key}
            label={`${provider.name} API key`}
            onSaved={speech.refresh}
            hint={
              <>
                From your {provider.name} account, kept in a file only you can read.{" "}
                {provider.key_url && (
                  <Button size="sm" tone="ghost" icon={ExternalLink} className="align-middle" onClick={() => d.act(() => App.OpenURL(provider.key_url))}>
                    Get a key
                  </Button>
                )}
              </>
            }
          />
        )}

        {provider.download_mb > 0 &&
          (provider.installing ? (
            <DownloadProgress progress={speech.progress} size={provider.download_mb} what={`${name}'s voice`} />
          ) : !provider.installed ? (
            <div className="flex flex-wrap items-center gap-3">
              <Button tone="primary" icon={Download} onClick={install} disabled={!status.player}>
                Download {name}'s voice
              </Button>
              <span className="text-xs text-ink-faint">{provider.download_mb} MB, once</span>
            </div>
          ) : (
            <Callout
              tone="working"
              icon={CircleCheck}
              action={
                <Button size="sm" tone="ghost" icon={Trash2} onClick={remove}>
                  Remove
                </Button>
              }
            >
              Downloaded. Turn on the speaker in the assistant, or press Talk to have a conversation.
            </Callout>
          ))}

        {speech.ready && (
          <>
            <ToggleRow title="Read replies aloud" description={`${name} says each reply in the assistant as it arrives.`} checked={aloud} onChange={setAloud} />
            <div className="flex flex-wrap items-end gap-3 rounded-lg border border-line bg-surface-2/40 p-4">
              <Face mood={busy ? "happy" : "smiling"} talking={speech.sounding !== 0} className="size-20" />
              <Field label="Her voice" className="min-w-56 flex-1">
                <Select value={speech.voice} onChange={(e) => speech.setVoice(e.target.value)}>
                  {provider.voices.map((v) => (
                    <option key={v.id} value={v.id}>
                      {v.name}
                    </option>
                  ))}
                </Select>
              </Field>
              <Button
                icon={busy ? (speech.sounding ? Square : LoaderCircle) : Volume2}
                className={cx(busy && !speech.sounding && "[&>svg]:animate-spin")}
                onClick={() => (busy ? speech.stop() : speech.say(`Hi, I'm ${name}. Tell me what's on your mind, and I'll sort out your day with you.`))}
              >
                {busy ? "Stop" : "Hear her"}
              </Button>
            </div>
          </>
        )}

        {speech.error && (
          <Callout tone="danger" icon={CircleAlert}>
            {speech.error}
          </Callout>
        )}
      </div>
    </Panel>
  );
}
