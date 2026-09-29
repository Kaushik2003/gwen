import { useState } from "react";
import { App, apiError, wire } from "../api";
import { useDaemon } from "../daemon";
import { addDays } from "../format";
import type { RetroOutput } from "../llm";
import Markdown from "./Markdown";
import { Button, Card, Input } from "./ui";

/** The Monday of the week before the one day belongs to. */
function lastMonday(day: string): string {
  const [y, m, d] = day.split("-").map(Number);
  const back = (new Date(y, m - 1, d).getDay() + 6) % 7;
  return addDays(day, -back - 7);
}

/** Stats → Week → Retro (docs/08-clients.md#v3-additions). */
export default function Retro() {
  const d = useDaemon();
  const [week, setWeek] = useState(lastMonday(d.today()));
  const [out, setOut] = useState<RetroOutput | null>(null);
  const [note, setNote] = useState<string | null>(null);
  const [working, setWorking] = useState(false);

  async function generate() {
    setWorking(true);
    setNote(null);
    try {
      const run = await App.Retro(wire.RetroRequest.createFrom({ week_start: week }));
      setOut(run.output as RetroOutput);
    } catch (e) {
      const err = apiError(e);
      if (err.code === "unavailable") setNote(`${err.message}. Choose a provider under Settings → AI.`);
      else d.fail(e);
    }
    setWorking(false);
  }

  return (
    <Card
      title="Retro"
      actions={
        <>
          <Input type="date" value={week} onChange={(e) => e.target.value && setWeek(e.target.value)} />
          <Button tone="primary" onClick={generate} disabled={working}>
            {working ? "Writing…" : "Generate"}
          </Button>
        </>
      }
    >
      {note && <p className="text-sm text-zinc-500">{note}</p>}
      {!out && !note && <p className="text-sm text-zinc-500">A look back at the 7 days from the chosen day.</p>}
      {out && (
        <>
          <Markdown text={out.markdown} />
          {out.generated_by === "rules" && (
            <p className="mt-3 text-xs text-zinc-500">Written from your numbers; the LLM did not answer.</p>
          )}
        </>
      )}
    </Card>
  );
}
