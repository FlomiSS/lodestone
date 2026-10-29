"use client";

import { useState } from "react";
import { Badge, Button, Card, CardHeader, PageHeader } from "@/components/ui/primitives";
import { useToast } from "@/components/ui/feedback";
import { useUser } from "@/components/layout/shell";
import { PixelAvatar } from "@/components/ui/mc";
import { api } from "@/lib/api";

export default function AccountPage() {
  const user = useUser();
  const toast = useToast();
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [repeat, setRepeat] = useState("");
  const [busy, setBusy] = useState(false);

  const mismatch = repeat !== "" && next !== repeat;

  return (
    <>
      <PageHeader title="Konto" description="Deine Anmeldedaten." />
      <div className="max-w-xl space-y-5">
        <Card className="flex items-center gap-4 p-5">
          <PixelAvatar name={user.username} size={48} />
          <div>
            <p className="text-sm font-semibold">{user.username}</p>
            {user.isAdmin ? <Badge tone="accent">Administrator</Badge> : <p className="text-xs text-muted">Benutzer</p>}
          </div>
        </Card>
        <Card>
          <CardHeader title="Passwort ändern" description="Danach musst du dich auf anderen Geräten neu anmelden." />
          <form
            className="space-y-4 px-5 py-4"
            onSubmit={async (event) => {
              event.preventDefault();
              setBusy(true);
              const ok = await toast.run(() => api.post("/me/password", { current, next }), "Passwort geändert");
              setBusy(false);
              if (ok) {
                setCurrent("");
                setNext("");
                setRepeat("");
              }
            }}
          >
            <div>
              <label className="label">Aktuelles Passwort</label>
              <input className="field" type="password" autoComplete="current-password" value={current} onChange={(event) => setCurrent(event.target.value)} required />
            </div>
            <div>
              <label className="label">Neues Passwort (mindestens 8 Zeichen)</label>
              <input className="field" type="password" autoComplete="new-password" value={next} onChange={(event) => setNext(event.target.value)} required minLength={8} />
            </div>
            <div>
              <label className="label">Neues Passwort wiederholen</label>
              <input className="field" type="password" autoComplete="new-password" value={repeat} onChange={(event) => setRepeat(event.target.value)} required />
              {mismatch && <p className="mt-1 text-xs text-bad">Die Passwörter stimmen nicht überein.</p>}
            </div>
            <Button type="submit" variant="primary" loading={busy} disabled={!current || next.length < 8 || next !== repeat}>
              Passwort ändern
            </Button>
          </form>
        </Card>
      </div>
    </>
  );
}
