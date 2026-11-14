import { BookMarked, CircleCheck, Download, FolderOpen, FolderSearch, RefreshCw, ShieldAlert, Trash2 } from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";
import { App, apiError, wire } from "../api";
import { useToast } from "../components/feedback";
import { useAssistantName } from "../components/MemoryDetail";
import { Button, Dot, Field, Input, Modal, Panel, Segmented, ToggleRow } from "../components/ui";
import { useDaemon } from "../daemon";
import { countWords, laneState, receiptText, useMemoryVersion } from "../memory";
import { useHasCredential } from "./Credential";

/** Where semantic search finds its embedding model when it is turned on: Ollama on this computer. */
const ollama = "http://localhost:11434";
/** The readable memory folder when it is turned on. */
const defaultFolder = "~/Gwen/memory";
/** What Forget everything asks you to type. */
const forgetPhrase = "forget everything";

type Draft = wire.MemoryConfig;

/** Settings → Memory: whether and how Gwen learns, where memory is kept, optional services, privacy, export, and forgetting everything. */
export default function MemorySettings() {
  const d = useDaemon();
  const notify = useToast();
  const name = useAssistantName();
  const version = useMemoryVersion();
  const saved = d.config!.memory;
  const [draft, setDraft] = useState<Draft>(saved);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [keys, setKeys] = useState({ mem0: "", supermemory: "" });
  const [hasMem0, setHasMem0] = useHasCredential("mem0_api_key");
  const [hasSuper, setHasSuper] = useHasCredential("supermemory_api_key");
  const [status, setStatus] = useState<wire.MemoryStatus | null>(null);
  const [saving, setSaving] = useState(false);
  const [rebuilding, setRebuilding] = useState(false);
  useEffect(() => setDraft(saved), [saved]);
  useEffect(() => {
    App.MemoryStatus().then(setStatus, () => setStatus(null));
  }, [saved, version]);

  const set = (patch: Partial<Draft>) => setDraft((x) => wire.MemoryConfig.createFrom({ ...x, ...patch }));
  const fields = Object.keys(saved) as (keyof Draft)[];
  const changed = fields.filter((k) => draft[k] !== saved[k]);
  const dirty = changed.length > 0 || keys.mem0.trim() !== "" || keys.supermemory.trim() !== "";
  const lane = (id: string) => status?.lanes?.find((l) => l.lane === id);

  async function save() {
    setSaving(true);
    try {
      if (changed.length) {
        const patch: Record<string, unknown> = {};
        for (const k of changed) patch[k] = typeof draft[k] === "string" ? (draft[k] as string).trim() : draft[k];
        await App.PatchConfig({ memory: patch });
      }
      for (const [cred, value, done] of [
        ["mem0_api_key", keys.mem0, setHasMem0],
        ["supermemory_api_key", keys.supermemory, setHasSuper],
      ] as const) {
        if (!value.trim()) continue;
        await App.SetCredential(cred, value.trim());
        done(true);
      }
      setKeys({ mem0: "", supermemory: "" });
      setErrors({});
      notify("Saved memory settings", "success");
    } catch (e) {
      const err = apiError(e);
      const key = typeof err.details.key === "string" ? err.details.key : "";
      if (key.startsWith("memory.")) setErrors({ [key.slice("memory.".length)]: err.message.replace(`${key}: `, "") });
      else d.fail(e);
    } finally {
      setSaving(false);
    }
  }

  async function rebuild() {
    setRebuilding(true);
    if (await d.run(() => App.RebuildMemory(wire.RebuildRequest.createFrom({ lane: "vec" })))) notify(`Rebuilding the search index. ${name} works as usual meanwhile.`, "success");
    setRebuilding(false);
  }

  async function pickFolder() {
    try {
      const dir = await App.PickFolder();
      if (dir) set({ files_dir: dir });
    } catch (e) {
      d.fail(e);
    }
  }

  const semantic = draft.embed_endpoint !== "";
  const folder = draft.files_dir !== "";
  return (
    <>
      <Panel title="Memory" icon={BookMarked}>
        <p className="-mt-1 mb-2 text-[13px] leading-relaxed text-ink-subtle">
          {name} learns about you and the people in your life from your chats and from what you ask her to remember, and brings it up when it helps. You can see and fix all of it on the
          Memory screen.
        </p>
        <div className="flex flex-col divide-y divide-line">
          <div className="py-3.5">
            <ToggleRow
              title="Memory"
              description={`Off, ${name} learns nothing new and recalls nothing. What she knows is kept until you forget it.`}
              checked={draft.enabled}
              onChange={(on) => set({ enabled: on })}
            />
          </div>

          <Row title="Learn from chats" hint={learnHint(draft.learn, name)} error={errors.learn}>
            <Segmented
              label="Learn from chats"
              value={draft.learn}
              onChange={(learn) => set({ learn })}
              options={[
                { value: "assistant", label: "Using the assistant's model" },
                { value: "local", label: "Using a local model" },
                { value: "off", label: "Off" },
              ]}
            />
            {draft.learn === "local" && (
              <div className="mt-3 grid gap-3 @lg:grid-cols-2">
                <Field label="Server" hint="Ollama on this computer is http://localhost:11434/v1." error={errors.local_endpoint}>
                  <Input value={draft.local_endpoint} onChange={(e) => set({ local_endpoint: e.target.value })} placeholder="http://localhost:11434/v1" spellCheck={false} />
                </Field>
                <Field label="Model" hint="As the server names it, such as llama3.1." error={errors.local_model}>
                  <Input value={draft.local_model} onChange={(e) => set({ local_model: e.target.value })} placeholder="llama3.1" spellCheck={false} />
                </Field>
              </div>
            )}
          </Row>

          <div className="py-3.5">
            <ToggleRow
              title="Double-check each fact"
              description="Before keeping a fact, a second look checks it against your own words. A little slower, with fewer mistakes."
              checked={draft.verify}
              onChange={(verify) => set({ verify })}
            />
          </div>

          <div className="flex flex-col gap-3 py-3.5">
            <ToggleRow
              title="Semantic search"
              description={`${name} finds memories by meaning as well as by words, with an embedding model running in Ollama on this computer.`}
              checked={semantic}
              onChange={(on) => set({ embed_endpoint: on ? (saved.embed_endpoint || ollama) : "" })}
            />
            {semantic && (
              <Field label="Embedding model" hint="Install it once with Ollama, such as nomic-embed-text." error={errors.embed_model || errors.embed_endpoint} className="w-72 max-w-full">
                <Input value={draft.embed_model} onChange={(e) => set({ embed_model: e.target.value })} placeholder="nomic-embed-text" spellCheck={false} />
              </Field>
            )}
            {semantic && saved.embed_endpoint && (
              <div className="flex flex-wrap items-center gap-3">
                <LaneLine lane={lane("vec")} />
                <Button size="sm" icon={RefreshCw} busy={rebuilding} onClick={rebuild}>
                  Rebuild index
                </Button>
              </div>
            )}
          </div>

          <div className="flex flex-col gap-3 py-3.5">
            <ToggleRow
              title="Readable memory folder"
              description={`${name} also writes what she knows as plain text files you can read. Edit them, and she learns from your changes.`}
              checked={folder}
              onChange={(on) => set({ files_dir: on ? (saved.files_dir || defaultFolder) : "" })}
            />
            {folder && (
              <div className="flex flex-wrap items-center gap-2">
                <span className="min-w-0 truncate rounded-md border border-line bg-surface-2 px-2.5 py-1.5 text-[13px] text-ink-muted" title={draft.files_dir}>
                  {draft.files_dir}
                </span>
                <Button size="sm" icon={FolderSearch} onClick={pickFolder}>
                  Change
                </Button>
                {saved.files_dir && saved.files_dir === draft.files_dir && (
                  <Button size="sm" icon={FolderOpen} onClick={() => d.run(() => App.OpenMemoryFolder())}>
                    Open folder
                  </Button>
                )}
              </div>
            )}
            {errors.files_dir && <p className="text-xs text-danger">{errors.files_dir}</p>}
            {folder && saved.files_dir && <LaneLine lane={lane("files")} />}
          </div>

          <div className="flex flex-col gap-3 py-3.5">
            <div>
              <div className="text-sm font-medium text-ink">Optional memory services</div>
              <div className="mt-0.5 text-[13px] text-ink-subtle">Mem0 and Supermemory can hold a copy for recall. Leave the address empty to not use one.</div>
            </div>
            <Service
              title="Mem0"
              url={draft.mem0_url}
              onURL={(mem0_url) => set({ mem0_url })}
              placeholder="http://localhost:8888"
              keyValue={keys.mem0}
              onKey={(mem0) => setKeys((k) => ({ ...k, mem0 }))}
              hasKey={hasMem0}
              error={errors.mem0_url}
              lane={saved.mem0_url ? lane("mem0") : undefined}
            />
            <Service
              title="Supermemory"
              url={draft.supermemory_url}
              onURL={(supermemory_url) => set({ supermemory_url })}
              placeholder="http://127.0.0.1:6767"
              keyValue={keys.supermemory}
              onKey={(supermemory) => setKeys((k) => ({ ...k, supermemory }))}
              hasKey={hasSuper}
              error={errors.supermemory_url}
              lane={saved.supermemory_url ? lane("supermemory") : undefined}
            />
          </div>

          <Row
            title="Privacy"
            hint={
              draft.privacy === "local"
                ? "Mem0 and Supermemory must run on this computer. Nothing leaves it."
                : "Mem0 and Supermemory may be hosted services. Facts about your health, contacts, and other sensitive things still never leave this computer."
            }
            error={errors.privacy}
          >
            <Segmented
              label="Privacy"
              value={draft.privacy}
              onChange={(privacy) => set({ privacy })}
              options={[
                { value: "local", label: "Keep everything on my devices" },
                { value: "cloud_ok", label: "Allow hosted connectors" },
              ]}
            />
          </Row>
        </div>
        <div className="mt-2 flex items-center justify-end gap-2 border-t border-line pt-4">
          {dirty && (
            <Button
              tone="ghost"
              onClick={() => {
                setDraft(saved);
                setKeys({ mem0: "", supermemory: "" });
                setErrors({});
              }}
            >
              Revert
            </Button>
          )}
          <Button tone="primary" disabled={!dirty} busy={saving} onClick={save}>
            Save changes
          </Button>
        </div>
      </Panel>
      <MemoryData name={name} />
    </>
  );
}

function learnHint(learn: string, name: string): string {
  if (learn === "local") return `A model on this computer or your Pi reads your chats for facts. Your chats don't leave your devices to be learned from.`;
  if (learn === "off") return `${name} only keeps what you tell her to remember, on the Memory screen.`;
  return `The model that answers in the assistant also reads your chats for facts, a little after each one.`;
}

/** A setting with its control under the words. */
function Row({ title, hint, error, children }: { title: string; hint?: ReactNode; error?: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-2.5 py-3.5">
      <div>
        <div className="text-sm font-medium text-ink">{title}</div>
        {hint && <div className="mt-0.5 text-[13px] leading-relaxed text-ink-subtle">{hint}</div>}
        {error && <div className="mt-1 text-xs text-danger">{error}</div>}
      </div>
      <div>{children}</div>
    </div>
  );
}

/** A lane's health in a line: a dot and words. */
function LaneLine({ lane }: { lane?: wire.MemoryLane }) {
  if (!lane) return null;
  const s = laneState(lane);
  return (
    <span className="flex items-center gap-2 text-xs text-ink-subtle">
      <Dot color={s.color} />
      {s.words}
    </span>
  );
}

/** One optional memory service on a line: its address, its API key, and how it is doing. */
function Service({
  title,
  url,
  onURL,
  placeholder,
  keyValue,
  onKey,
  hasKey,
  error,
  lane,
}: {
  title: string;
  url: string;
  onURL: (v: string) => void;
  placeholder: string;
  keyValue: string;
  onKey: (v: string) => void;
  hasKey: boolean | null;
  error?: string;
  lane?: wire.MemoryLane;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex flex-wrap items-center gap-2">
        <span className="w-24 shrink-0 text-[13px] font-medium text-ink-muted">{title}</span>
        <Input value={url} onChange={(e) => onURL(e.target.value)} placeholder={placeholder} aria-label={`${title} address`} className="min-w-0 flex-1 basis-48" spellCheck={false} />
        <Input
          type="password"
          value={keyValue}
          onChange={(e) => onKey(e.target.value)}
          placeholder={hasKey ? "Key saved. Type to replace" : "API key, if it needs one"}
          aria-label={`${title} API key`}
          autoComplete="off"
          className="min-w-0 flex-1 basis-48"
        />
        <span className="grid w-4 shrink-0 place-items-center">{hasKey && <CircleCheck size={15} className="text-working" aria-label="API key saved" />}</span>
      </div>
      {error && <p className="pl-26 text-xs text-danger">{error}</p>}
      {lane && (
        <div className="pl-26">
          <LaneLine lane={lane} />
        </div>
      )}
    </div>
  );
}

/** Export everything memory holds, or erase it all. */
function MemoryData({ name }: { name: string }) {
  const d = useDaemon();
  const notify = useToast();
  const [exporting, setExporting] = useState(false);
  const [forgetting, setForgetting] = useState(false);

  async function exportMemory() {
    setExporting(true);
    try {
      const path = await App.SaveMemoryExport();
      if (path) notify(`Saved ${name}'s memory to ${path}. Only you can read the file.`, "success");
    } catch (e) {
      d.fail(e);
    } finally {
      setExporting(false);
    }
  }

  return (
    <Panel title="Your memory data" icon={ShieldAlert}>
      <div className="flex flex-col divide-y divide-line">
        <div className="flex flex-wrap items-center justify-between gap-x-6 gap-y-2 py-3.5">
          <div className="min-w-0 flex-1 basis-56">
            <div className="text-sm font-medium text-ink">Export memory</div>
            <div className="mt-0.5 text-[13px] text-ink-subtle">Save everything {name} remembers to a file on this computer, readable only by you.</div>
          </div>
          <Button icon={Download} busy={exporting} onClick={exportMemory}>
            Export memory
          </Button>
        </div>
        <div className="flex flex-wrap items-center justify-between gap-x-6 gap-y-2 py-3.5">
          <div className="min-w-0 flex-1 basis-56">
            <div className="text-sm font-medium text-ink">Forget everything</div>
            <div className="mt-0.5 text-[13px] text-ink-subtle">Erase everything {name} has learned, everywhere she keeps it. Your chats and tasks stay.</div>
          </div>
          <Button tone="danger" icon={Trash2} onClick={() => setForgetting(true)}>
            Forget everything
          </Button>
        </div>
      </div>
      {forgetting && <ForgetEverything name={name} onClose={() => setForgetting(false)} />}
    </Panel>
  );
}

/** Erasing all of memory asks you to type the words first. */
function ForgetEverything({ name, onClose }: { name: string; onClose: () => void }) {
  const d = useDaemon();
  const notify = useToast();
  const [typed, setTyped] = useState("");
  const [busy, setBusy] = useState(false);
  const ready = typed.trim().toLowerCase() === forgetPhrase;
  async function forget() {
    if (!ready) return;
    setBusy(true);
    const r = await d.act(() => App.ForgetAllMemory(forgetPhrase));
    setBusy(false);
    if (!r) return;
    const counted = countWords(r);
    notify(receiptText(r, counted ? `Forgot everything: ${counted}.` : "Forgot everything.", name), "success");
    onClose();
  }
  return (
    <Modal
      title="Forget everything?"
      size="sm"
      description={`${name} erases every fact, note, and document she holds, in every place she keeps them. This can't be undone. Your chats and tasks stay.`}
      onClose={onClose}
      footer={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button tone="destroy" busy={busy} disabled={!ready} onClick={forget}>
            Forget everything
          </Button>
        </>
      }
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          forget();
        }}
      >
        <Field label={`To confirm, type “${forgetPhrase}”`}>
          <Input value={typed} onChange={(e) => setTyped(e.target.value)} autoFocus autoComplete="off" spellCheck={false} placeholder={forgetPhrase} />
        </Field>
      </form>
    </Modal>
  );
}
