import { CircleCheck } from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";
import { App } from "../api";
import { useToast } from "../components/feedback";
import { Badge, Button, Input } from "../components/ui";
import { useDaemon } from "../daemon";

/** Whether a credential file is stored, without reading it; null until known. */
export function useHasCredential(name: string, version = 0): [boolean | null, (on: boolean) => void] {
  const [has, setHas] = useState<boolean | null>(null);
  useEffect(() => {
    App.HasCredential(name).then(setHas, () => setHas(null));
  }, [name, version]);
  return [has, setHas];
}

/** A secret written to the credentials directory, readable only by you (docs/07-integrations.md#credentials). */
export function CredentialField({ name, label, hint, onSaved }: { name: string; label: string; hint?: ReactNode; onSaved?: () => void }) {
  const d = useDaemon();
  const notify = useToast();
  const [has, setHas] = useHasCredential(name);
  const [value, setValue] = useState("");
  async function save() {
    try {
      await App.SetCredential(name, value.trim());
      setValue("");
      setHas(true);
      onSaved?.();
      notify(`Saved the ${label}`, "success");
    } catch (e) {
      d.fail(e);
    }
  }
  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-2">
        <span className="text-sm font-medium text-ink">{label}</span>
        {has && (
          <Badge tone="working" icon={CircleCheck}>
            Saved
          </Badge>
        )}
      </div>
      {hint && <p className="-mt-2 text-[13px] text-ink-subtle">{hint}</p>}
      <form
        className="flex gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          if (value.trim()) save();
        }}
      >
        <Input type="password" value={value} onChange={(e) => setValue(e.target.value)} placeholder={has ? "Type a new one to replace it" : "Paste it here"} autoComplete="off" className="min-w-0 flex-1" aria-label={label} />
        <Button type="submit" disabled={!value.trim()}>
          Save
        </Button>
      </form>
    </div>
  );
}
