import { useState } from "react";
import BrandMark from "../components/BrandMark";
import { Button, Field, Input } from "../components/ui";
import { useDaemon } from "../daemon";
import { login } from "../hub";

/** The hub's sign-in: the sync token, once per browser for 30 days. */
export default function Login() {
  const d = useDaemon();
  const [token, setToken] = useState("");
  const [wrong, setWrong] = useState(false);
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    const ok = await login(token.trim()).catch(() => false);
    setBusy(false);
    setWrong(!ok);
    if (ok) await d.refresh();
  }

  return (
    <div className="flex h-full items-center justify-center p-4">
      <form onSubmit={submit} className="lift flex w-full max-w-sm flex-col gap-5 rounded-2xl border border-line bg-surface-1 p-7">
        <div className="flex items-center gap-2.5">
          <BrandMark className="size-7" />
          <span className="text-lg font-semibold tracking-[-0.01em]">Gwen</span>
        </div>
        <p className="text-sm leading-relaxed text-ink-subtle">Enter the hub's sync token to see your dashboard.</p>
        <Field label="Sync token" error={wrong ? "That is not the hub's token." : undefined}>
          <Input type="password" value={token} onChange={(e) => setToken(e.target.value)} autoFocus autoComplete="current-password" />
        </Field>
        <Button tone="primary" size="lg" type="submit" disabled={!token.trim()} busy={busy}>
          Sign in
        </Button>
      </form>
    </div>
  );
}
