import { useEffect, useState } from "react";
import { App } from "../api";
import { Button, Card, Input, Label, Select } from "../components/ui";
import { useDaemon } from "../daemon";

/** Settings → AI (docs/08-clients.md#v3-additions). */
export default function AISettings() {
  const d = useDaemon();
  const llm = d.config?.llm;
  const [provider, setProvider] = useState(llm?.provider ?? "none");
  const [model, setModel] = useState(llm?.model ?? "");
  const [endpoint, setEndpoint] = useState(llm?.endpoint ?? "");
  const [key, setKey] = useState("");
  const [saved, setSaved] = useState(false);
  useEffect(() => {
    setProvider(llm?.provider ?? "none");
    setModel(llm?.model ?? "");
    setEndpoint(llm?.endpoint ?? "");
  }, [llm?.provider, llm?.model, llm?.endpoint]);

  async function save() {
    setSaved(false);
    const patch: Record<string, unknown> = { provider, model };
    if (provider === "openai_compatible") patch.endpoint = endpoint;
    if (!(await d.act(() => App.PatchConfig({ llm: patch })))) return;
    if (key) {
      try {
        await App.SetCredential("llm_api_key", key);
        setKey("");
      } catch (e) {
        d.fail(e);
        return;
      }
    }
    setSaved(true);
  }

  return (
    <Card title="AI">
      <p className="mb-3 text-xs text-zinc-500">
        Optional. An LLM can propose tasks for a goal and write a weekly retrospective. It never changes your plan
        without you.
      </p>
      <div className="grid grid-cols-2 gap-3">
        <Label text="Provider">
          <Select value={provider} onChange={(e) => setProvider(e.target.value)}>
            <option value="none">None</option>
            <option value="anthropic">Anthropic</option>
            <option value="openai_compatible">OpenAI-compatible (such as Ollama)</option>
          </Select>
        </Label>
        {provider !== "none" && (
          <Label text="Model">
            <Input value={model} onChange={(e) => setModel(e.target.value)} />
          </Label>
        )}
        {provider === "openai_compatible" && (
          <Label text="Endpoint">
            <Input value={endpoint} placeholder="http://localhost:11434/v1" onChange={(e) => setEndpoint(e.target.value)} />
          </Label>
        )}
        {provider !== "none" && (
          <Label text="API key (leave empty to keep the current one)">
            <Input type="password" value={key} onChange={(e) => setKey(e.target.value)} autoComplete="off" />
          </Label>
        )}
      </div>
      <div className="mt-3 flex items-center gap-3">
        <Button tone="primary" onClick={save}>
          Save
        </Button>
        {saved && <span className="text-xs text-emerald-600">Saved</span>}
      </div>
    </Card>
  );
}
