import { useState } from "react";
import { Button, Card, Input, Label } from "../components/ui";
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
      <Card className="w-full max-w-sm">
        <form onSubmit={submit} className="flex flex-col gap-3">
          <div className="flex items-center gap-2">
            <span className="inline-block h-6 w-6 rounded-md bg-emerald-500" />
            <span className="text-lg font-semibold">Gwen</span>
          </div>
          <p className="text-sm text-zinc-500">Enter the hub's sync token to see your dashboard.</p>
          <Label text="Sync token" error={wrong ? "That is not the hub's token." : undefined}>
            <Input type="password" value={token} onChange={(e) => setToken(e.target.value)} autoFocus autoComplete="current-password" />
          </Label>
          <Button tone="primary" type="submit" disabled={!token.trim() || busy}>
            {busy ? "Signing in…" : "Sign in"}
          </Button>
        </form>
      </Card>
    </div>
  );
}
