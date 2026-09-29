import { useState } from "react";
import { App, wire } from "../api";
import { formatDuration } from "../format";
import type { BreakdownOutput, ProposedTask, RunError } from "../llm";
import { Button, Input, Label, Modal } from "./ui";
import { useDaemon } from "../daemon";

/** Goal detail → Break down (docs/08-clients.md#v3-additions). */
export default function Breakdown({ goal, onClose }: { goal: wire.Goal; onClose: () => void }) {
  const d = useDaemon();
  const [instructions, setInstructions] = useState("");
  const [run, setRun] = useState<wire.LlmRun | null>(null);
  const [picked, setPicked] = useState<Set<number>>(new Set());
  const [working, setWorking] = useState(false);

  const tasks: ProposedTask[] = run?.status === "ok" ? (run.output as BreakdownOutput).tasks : [];
  const failure = run?.status === "failed" ? (run.output as RunError).error : null;

  async function ask() {
    setWorking(true);
    const r = await d.act(() => App.GoalBreakdown(goal.id, wire.BreakdownRequest.createFrom({ instructions })));
    setWorking(false);
    if (!r) return;
    setRun(r);
    if (r.status === "ok") setPicked(new Set((r.output as BreakdownOutput).tasks.map((_, i) => i)));
  }
  async function add() {
    if (!run) return;
    const ok = await d.act(() =>
      App.AcceptLLMRun(run.id, wire.AcceptRunRequest.createFrom({ indexes: [...picked].sort((a, b) => a - b) })),
    );
    if (ok) onClose();
  }
  async function discard() {
    if (run?.status === "ok") await d.act(() => App.RejectLLMRun(run.id));
    onClose();
  }
  const toggle = (i: number) =>
    setPicked((p) => {
      const n = new Set(p);
      if (n.has(i)) n.delete(i);
      else n.add(i);
      return n;
    });

  return (
    <Modal title={`Break down: ${goal.title}`} onClose={onClose}>
      <div className="flex flex-col gap-3">
        <Label text="What should the tasks focus on?">
          <Input value={instructions} onChange={(e) => setInstructions(e.target.value)} placeholder="optional" autoFocus />
        </Label>
        <div>
          <Button onClick={ask} disabled={working}>
            {working ? "Asking…" : run ? "Ask again" : "Propose tasks"}
          </Button>
        </div>
        {failure && <p className="text-sm text-red-600 dark:text-red-400">{failure}</p>}
        {tasks.length > 0 && (
          <ul className="max-h-72 overflow-y-auto divide-y divide-zinc-200 text-sm dark:divide-zinc-800">
            {tasks.map((t, i) => (
              <li key={i} className="flex items-start gap-2 py-1.5">
                <input type="checkbox" className="mt-1" checked={picked.has(i)} onChange={() => toggle(i)} />
                <div className="flex-1">
                  <div>{t.title}</div>
                  <div className="text-xs text-zinc-500">
                    {formatDuration(t.estimate_minutes * 60_000)} · due {t.due_day} · P{t.priority}
                    {t.quantity != null && ` · ${t.quantity} ${goal.unit || "units"}`}
                  </div>
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
      <div className="mt-5 flex justify-end gap-2">
        <Button onClick={discard}>Discard</Button>
        <Button tone="primary" onClick={add} disabled={picked.size === 0 || tasks.length === 0}>
          Add selected
        </Button>
      </div>
    </Modal>
  );
}
