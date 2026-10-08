import { CircleAlert, CircleCheck, Download, Mic, Trash2 } from "lucide-react";
import { useState } from "react";
import { App } from "../api";
import { useConfirm, useToast } from "../components/feedback";
import { Button, Callout, Panel } from "../components/ui";
import { DownloadProgress, useDictation, useVoice } from "../components/Voice";
import { useDaemon } from "../daemon";

/** Settings → AI → Voice input: the one-time model download, a mic test, and removal. */
export default function VoiceSettings() {
  const d = useDaemon();
  const notify = useToast();
  const confirm = useConfirm();
  const { status, progress } = useVoice();
  const [heard, setHeard] = useState<string | null>(null);
  const test = useDictation({ onText: (said) => setHeard(said || null) });

  async function install() {
    try {
      await App.InstallVoice();
      notify("Voice input is ready", "success");
    } catch (e) {
      d.fail(e);
    }
  }

  async function remove() {
    const ok = await confirm({
      title: "Remove the voice model?",
      body: `Frees ${status?.download_mb ?? 100} MB. You can download it again any time.`,
      confirm: "Remove",
      danger: true,
    });
    if (!ok) return;
    setHeard(null);
    try {
      await App.RemoveVoice();
      notify("Removed the voice model", "success");
    } catch (e) {
      d.fail(e);
    }
  }

  if (!status) return null;
  return (
    <Panel title="Voice input" icon={Mic}>
      <p className="-mt-1 mb-4 text-[13px] leading-relaxed text-ink-subtle">
        Talk to the assistant instead of typing. NVIDIA's Parakeet model turns your speech into text right on this computer, in English. No audio leaves it, and it only takes memory while you talk.
      </p>

      {!status.recorder ? (
        <Callout tone="idle" icon={CircleAlert}>
          Voice input records through PipeWire's recorder, which isn't installed. Install the <span className="text-ink">pipewire-utils</span> package, then reopen Settings.
        </Callout>
      ) : status.installing ? (
        <DownloadProgress progress={progress} size={status.download_mb} />
      ) : !status.installed ? (
        <div className="flex flex-wrap items-center gap-3">
          <Button tone="primary" icon={Download} onClick={install}>
            Download voice model
          </Button>
          <span className="text-xs text-ink-faint">{status.download_mb} MB, once</span>
        </div>
      ) : (
        <div className="flex flex-col gap-4">
          <Callout
            tone="working"
            icon={CircleCheck}
            action={
              <Button size="sm" tone="ghost" icon={Trash2} onClick={remove} disabled={status.listening}>
                Remove
              </Button>
            }
          >
            Ready. Press the mic beside the message box in the assistant, speak, then press it again.
          </Callout>
          <div className="flex flex-col gap-3 rounded-lg border border-line bg-surface-2/40 p-4">
            <div className="flex items-center gap-3">
              {test.button}
              <div className="min-w-0 text-[13px]">
                <div className="font-medium text-ink">Test your microphone</div>
                <div className="text-xs text-ink-subtle">Press the mic, say a sentence, and press it again.</div>
              </div>
            </div>
            {test.panel}
            {heard != null && (
              <p className="rounded-md border border-line bg-surface-1 px-3 py-2 text-[13px] text-ink">
                <span className="text-ink-faint">Heard: </span>
                {heard}
              </p>
            )}
          </div>
        </div>
      )}
    </Panel>
  );
}
