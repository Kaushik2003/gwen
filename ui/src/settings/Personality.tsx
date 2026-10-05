import { Drama } from "lucide-react";
import { useEffect, useState } from "react";
import { App } from "../api";
import { useToast } from "../components/feedback";
import { Button, Field, Input, Panel, TextArea, cx } from "../components/ui";
import { useDaemon } from "../daemon";

/** The personalities of llm.personality, with how each sounds. */
const personas = [
  { id: "coach", name: "Coach", line: "Nice work on the report! Next up: the hard one, while you're fresh." },
  { id: "friend", name: "Friend", line: "Okay, three things today and then you're free. Coffee first?" },
  { id: "mentor", name: "Mentor", line: "Start with the essay: it moves your exam goal forward the most." },
  { id: "sergeant", name: "Drill sergeant", line: "Essay. Now. You can check your email after." },
  { id: "zen", name: "Zen", line: "One task at a time. The essay, gently, until lunch." },
];

/** Settings → AI → Personality: who the assistant is when it talks to you. */
export default function PersonalitySettings() {
  const d = useDaemon();
  const notify = useToast();
  const llm = d.config!.llm;
  const [name, setName] = useState(llm.assistant_name);
  const [personality, setPersonality] = useState(llm.personality);
  const [instructions, setInstructions] = useState(llm.instructions);
  const [saving, setSaving] = useState(false);
  useEffect(() => {
    setName(llm.assistant_name);
    setPersonality(llm.personality);
    setInstructions(llm.instructions);
  }, [llm.assistant_name, llm.personality, llm.instructions]);

  const changed = name.trim() !== llm.assistant_name || personality !== llm.personality || instructions.trim() !== llm.instructions;
  async function save() {
    setSaving(true);
    const ok = await d.act(() => App.PatchConfig({ llm: { assistant_name: name.trim(), personality, instructions: instructions.trim() } }));
    setSaving(false);
    if (ok) notify(`${name.trim()} is ready`, "success");
  }

  return (
    <Panel title="Personality" icon={Drama}>
      <p className="-mt-1 mb-4 text-[13px] leading-relaxed text-ink-subtle">How your assistant talks to you, in the chat, the day plan, and the weekly retro.</p>
      <div className="flex flex-col gap-5">
        <Field label="Name">
          <Input value={name} onChange={(e) => setName(e.target.value)} maxLength={40} className="w-60" placeholder="Gwen" />
        </Field>
        <div role="radiogroup" aria-label="Personality" className="grid gap-2 @lg:grid-cols-2 @2xl:grid-cols-3">
          {personas.map((p) => {
            const on = p.id === personality;
            return (
              <button
                key={p.id}
                type="button"
                role="radio"
                aria-checked={on}
                onClick={() => setPersonality(p.id)}
                className={cx("flex flex-col gap-1.5 rounded-lg border p-3 text-left transition-colors", on ? "border-accent bg-accent/10" : "border-line-strong bg-surface-2 hover:border-line-3")}
              >
                <span className="text-sm font-medium text-ink">{p.name}</span>
                <span className="text-xs leading-relaxed text-ink-subtle italic">“{p.line}”</span>
              </button>
            );
          })}
        </div>
        <Field label="Your own instructions" hint={`Anything about you or how you like to be helped. ${2000 - instructions.length} characters left.`}>
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
            Save personality
          </Button>
        </div>
      </div>
    </Panel>
  );
}
