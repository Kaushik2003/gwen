import { Drama, RotateCcw } from "lucide-react";
import { useEffect, useState } from "react";
import { App, isHub, wire } from "../api";
import Face from "../components/Face";
import { useConfirm, useToast } from "../components/feedback";
import { Button, Field, Input, Panel, TextArea } from "../components/ui";
import { useDaemon } from "../daemon";
import { formatAgo } from "../format";
import { attitudeOf } from "../llm";

/** Settings → AI → Personality: who she is, how she has chosen to treat you, and what you tell her about yourself. */
export default function PersonalitySettings() {
  const d = useDaemon();
  const notify = useToast();
  const confirm = useConfirm();
  const llm = d.config!.llm;
  const [name, setName] = useState(llm.assistant_name);
  const [instructions, setInstructions] = useState(llm.instructions);
  const [saving, setSaving] = useState(false);
  const [self, setSelf] = useState<wire.AssistantSelf | null>(null);
  useEffect(() => {
    setName(llm.assistant_name);
    setInstructions(llm.instructions);
  }, [llm.assistant_name, llm.instructions]);
  useEffect(() => {
    if (!isHub) App.AssistantSelf().then(setSelf, () => {});
  }, []);

  const shown = llm.assistant_name || "Gwen";
  const changed = name.trim() !== llm.assistant_name || instructions.trim() !== llm.instructions;
  async function save() {
    setSaving(true);
    const ok = await d.act(() => App.PatchConfig({ llm: { assistant_name: name.trim(), instructions: instructions.trim() } }));
    setSaving(false);
    if (ok) notify(`${name.trim()} is ready`, "success");
  }

  async function reset() {
    const ok = await confirm({
      title: `Let ${shown} start fresh?`,
      body: "She forgets the attitude she chose and her note about you, and makes up her mind again as you talk.",
      confirm: "Start fresh",
    });
    if (!ok) return;
    if (await d.act(() => App.ResetAssistantSelf())) {
      setSelf(wire.AssistantSelf.createFrom({ attitude: "", note: "", since: null }));
      notify(`${shown} is starting fresh`, "success");
    }
  }

  const attitude = attitudeOf(self?.attitude);
  const decided = !!attitude || !!self?.note;
  return (
    <Panel title="Personality" icon={Drama}>
      <p className="-mt-1 mb-4 text-[13px] leading-relaxed text-ink-subtle">
        {shown} has one personality: a Gwen Stacy who's witty, affectionate, and honest with you. She decides for herself how to treat you, from how your days are going and what you tell her, and
        keeps a note on why. It shapes the chat, the day plan, and the weekly retro.
      </p>
      <div className="flex flex-col gap-5">
        {self && (
          <div className="flex items-start gap-4 rounded-lg border border-line bg-surface-2/40 p-4">
            <Face mood={attitude?.face ?? "smiling"} react className="size-24" />
            <div className="min-w-0 flex-1">
              {decided ? (
                <>
                  <div className="text-sm font-medium text-ink">
                    {attitude ? `${attitude.label} with you` : "Making up her mind"}
                    {self.since != null && <span className="ml-2 text-xs font-normal text-ink-faint">since {formatAgo(self.since, Date.now())}</span>}
                  </div>
                  {attitude && <div className="mt-0.5 text-xs text-ink-subtle">{attitude.means}</div>}
                  {self.note && (
                    <blockquote className="mt-3 border-l-2 border-accent/50 pl-3 text-[13px] leading-relaxed text-ink-muted italic">
                      “{self.note}”<span className="mt-1 block text-[11px] text-ink-faint not-italic">Her note to herself</span>
                    </blockquote>
                  )}
                </>
              ) : (
                <>
                  <div className="text-sm font-medium text-ink">Just herself, for now</div>
                  <div className="mt-0.5 text-xs leading-relaxed text-ink-subtle">Talk to {shown} in the assistant. If you slack off or have a rough day, she'll change how she treats you, and say so.</div>
                </>
              )}
            </div>
            {decided && (
              <Button size="sm" tone="ghost" icon={RotateCcw} onClick={reset}>
                Start fresh
              </Button>
            )}
          </div>
        )}
        <Field label="Name">
          <Input value={name} onChange={(e) => setName(e.target.value)} maxLength={40} className="w-60" placeholder="Gwen" />
        </Field>
        <Field label="About you" hint={`Anything about you or how you like to be helped. ${2000 - instructions.length} characters left.`}>
          <TextArea
            rows={4}
            value={instructions}
            onChange={(e) => setInstructions(e.target.value)}
            maxLength={2000}
            placeholder="I'm a student; my exams are in December. Keep replies short. Call me Kaushik. Push me on deep work, go easy on weekends."
          />
        </Field>
        <div className="flex justify-end gap-2 border-t border-line pt-4">
          <Button tone="primary" disabled={!changed || !name.trim()} busy={saving} onClick={save}>
            Save
          </Button>
        </div>
      </div>
    </Panel>
  );
}
