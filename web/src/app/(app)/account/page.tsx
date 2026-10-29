"use client";

import QRCode from "qrcode";
import { Copy, KeyRound, LogOut, Plus, ShieldCheck, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import { Badge, Button, Card, CardHeader, PageHeader } from "@/components/ui/primitives";
import { ConfirmDialog, Dialog, useToast } from "@/components/ui/feedback";
import { Loading } from "@/components/ui/async";
import { useUser } from "@/components/layout/shell";
import { PixelAvatar } from "@/components/ui/mc";
import { api, useApi } from "@/lib/api";
import { ago, dateTime } from "@/lib/format";
import type { ApiKeyInfo, Me, SessionInfo } from "@/lib/types";

export default function AccountPage() {
  const user = useUser();
  return (
    <>
      <PageHeader title="Konto" description="Anmeldung, Sicherheit und Zugriff für Skripte." />
      <div className="max-w-2xl space-y-5">
        <Card className="flex items-center gap-4 p-5">
          <PixelAvatar name={user.username} size={48} />
          <div>
            <p className="text-sm font-semibold">{user.username}</p>
            {user.isAdmin ? <Badge tone="accent">Administrator</Badge> : <p className="text-xs text-muted">Benutzer</p>}
          </div>
        </Card>
        <PasswordCard />
        <TwoFactorCard />
        <KeysCard />
        <SessionsCard />
      </div>
    </>
  );
}

function PasswordCard() {
  const toast = useToast();
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [repeat, setRepeat] = useState("");
  const [busy, setBusy] = useState(false);
  const mismatch = repeat !== "" && next !== repeat;
  return (
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
  );
}

// ---------------------------------------------------------------- Zwei-Faktor

function TwoFactorCard() {
  const toast = useToast();
  const { data: me, mutate } = useApi<Me>("/me");
  const [setup, setSetup] = useState<{ secret: string; uri: string } | null>(null);
  const [qr, setQr] = useState("");
  const [code, setCode] = useState("");
  const [codes, setCodes] = useState<string[] | null>(null);
  const [disabling, setDisabling] = useState(false);
  const [password, setPassword] = useState("");
  const [otp, setOtp] = useState("");

  useEffect(() => {
    if (setup) QRCode.toDataURL(setup.uri, { margin: 1, width: 192 }).then(setQr).catch(() => setQr(""));
    else setQr("");
  }, [setup]);

  if (!me) return null;
  return (
    <Card>
      <CardHeader
        title="Zwei-Faktor-Anmeldung"
        description="Neben dem Passwort wird bei der Anmeldung ein Code aus einer App (Google Authenticator, Aegis, 1Password …) verlangt."
        action={me.totpEnabled ? <Badge tone="good">Eingeschaltet</Badge> : <Badge>Aus</Badge>}
      />
      <div className="space-y-4 px-5 py-4 text-sm">
        {me.totpEnabled ? (
          <>
            <p className="text-xs text-muted">Noch {me.recoveryCodes} Wiederherstellungscodes übrig. Jeder gilt einmal, falls du die App nicht hast.</p>
            <Button variant="danger" onClick={() => setDisabling(true)}>
              Ausschalten
            </Button>
          </>
        ) : setup ? (
          <div className="flex flex-wrap items-start gap-5">
            {/* eslint-disable-next-line @next/next/no-img-element */}
            {qr && <img src={qr} alt="QR-Code" width={160} height={160} className="rounded-lg bg-white p-1" />}
            <div className="min-w-[220px] flex-1 space-y-3">
              <p className="text-xs text-muted">Scanne den Code mit deiner App oder gib diesen Schlüssel ein:</p>
              <code className="block break-all rounded-md bg-black/30 px-3 py-2 text-xs">{setup.secret}</code>
              <div>
                <label className="label">Code aus der App zur Bestätigung</label>
                <input className="field w-40 font-mono tracking-widest" inputMode="numeric" value={code} onChange={(event) => setCode(event.target.value)} placeholder="000000" />
              </div>
              <div className="flex gap-2">
                <Button
                  variant="primary"
                  disabled={code.trim().length < 6}
                  onClick={async () => {
                    try {
                      const answer = await api.post<{ recoveryCodes: string[] }>("/me/totp/enable", { code });
                      setCodes(answer.recoveryCodes);
                      setSetup(null);
                      setCode("");
                      mutate();
                    } catch (error) {
                      toast.error(error instanceof Error ? error.message : "Fehler");
                    }
                  }}
                >
                  Einschalten
                </Button>
                <Button variant="ghost" onClick={() => setSetup(null)}>
                  Abbrechen
                </Button>
              </div>
            </div>
          </div>
        ) : (
          <Button
            onClick={async () => {
              try {
                setSetup(await api.post("/me/totp/setup"));
              } catch (error) {
                toast.error(error instanceof Error ? error.message : "Fehler");
              }
            }}
          >
            <ShieldCheck className="h-4 w-4" />
            Einrichten
          </Button>
        )}
      </div>

      <Dialog
        open={!!codes}
        onClose={() => setCodes(null)}
        title="Deine Wiederherstellungscodes"
        description="Speichere sie jetzt an einem sicheren Ort. Sie werden nicht noch einmal angezeigt."
        footer={
          <>
            <Button onClick={() => navigator.clipboard.writeText((codes ?? []).join("\n")).then(() => toast.success("Kopiert"))}>
              <Copy className="h-3.5 w-3.5" />
              Kopieren
            </Button>
            <Button variant="primary" onClick={() => setCodes(null)}>
              Gespeichert
            </Button>
          </>
        }
      >
        <div className="grid grid-cols-2 gap-2 font-mono text-sm">
          {(codes ?? []).map((entry) => (
            <code key={entry} className="rounded bg-black/30 px-3 py-1.5 text-center">
              {entry}
            </code>
          ))}
        </div>
      </Dialog>

      <Dialog
        open={disabling}
        onClose={() => setDisabling(false)}
        title="Zwei-Faktor-Anmeldung ausschalten"
        footer={
          <>
            <Button variant="ghost" onClick={() => setDisabling(false)}>
              Abbrechen
            </Button>
            <Button
              variant="danger"
              disabled={!password || !otp}
              onClick={async () => {
                if (await toast.run(() => api.post("/me/totp/disable", { password, code: otp }), "Ausgeschaltet")) {
                  setDisabling(false);
                  setPassword("");
                  setOtp("");
                  mutate();
                }
              }}
            >
              Ausschalten
            </Button>
          </>
        }
      >
        <div>
          <label className="label">Passwort</label>
          <input className="field" type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="current-password" />
        </div>
        <div>
          <label className="label">Code aus der App oder Wiederherstellungscode</label>
          <input className="field font-mono" value={otp} onChange={(event) => setOtp(event.target.value)} autoComplete="off" />
        </div>
      </Dialog>
    </Card>
  );
}

// ---------------------------------------------------------------- API-Schlüssel

function KeysCard() {
  const toast = useToast();
  const { data, mutate } = useApi<ApiKeyInfo[]>("/me/keys");
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const [days, setDays] = useState("0");
  const [created, setCreated] = useState<string | null>(null);
  const [removing, setRemoving] = useState<ApiKeyInfo | null>(null);

  return (
    <Card>
      <CardHeader
        title="API-Schlüssel"
        description="Für Skripte und Automatisierung. Ein Schlüssel hat dieselben Rechte wie du (außer an deinen Zugangsdaten). Nutzung: Authorization: Bearer lds_…"
        action={
          <Button size="sm" onClick={() => setCreating(true)}>
            <Plus className="h-3.5 w-3.5" />
            Neuer Schlüssel
          </Button>
        }
      />
      {!data ? (
        <Loading />
      ) : data.length === 0 ? (
        <p className="px-5 py-6 text-center text-xs text-muted">Noch keine Schlüssel.</p>
      ) : (
        <div className="divide-y divide-white/[0.05]">
          {data.map((key) => (
            <div key={key.id} className="flex flex-wrap items-center gap-x-4 gap-y-1 px-5 py-3">
              <KeyRound className="h-4 w-4 text-muted" />
              <div className="min-w-[160px] flex-1">
                <p className="text-sm font-medium">{key.name}</p>
                <p className="text-xs text-muted">
                  <span className="font-mono">{key.prefix}…</span> · {key.lastUsed ? `zuletzt ${ago(key.lastUsed)}` : "noch nie benutzt"}
                  {key.expiresAt > 0 && ` · läuft ${dateTime(key.expiresAt)} ab`}
                </p>
              </div>
              <Button variant="ghost" size="icon-sm" onClick={() => setRemoving(key)} aria-label="Widerrufen">
                <Trash2 className="h-3.5 w-3.5" />
              </Button>
            </div>
          ))}
        </div>
      )}

      <Dialog
        open={creating}
        onClose={() => setCreating(false)}
        title="Neuer API-Schlüssel"
        footer={
          <>
            <Button variant="ghost" onClick={() => setCreating(false)}>
              Abbrechen
            </Button>
            <Button
              variant="primary"
              disabled={!name.trim()}
              onClick={async () => {
                try {
                  const answer = await api.post<{ key: string }>("/me/keys", { name, expiresDays: Number(days) || 0 });
                  setCreated(answer.key);
                  setCreating(false);
                  setName("");
                  mutate();
                } catch (error) {
                  toast.error(error instanceof Error ? error.message : "Fehler");
                }
              }}
            >
              Anlegen
            </Button>
          </>
        }
      >
        <div>
          <label className="label">Name</label>
          <input className="field" value={name} onChange={(event) => setName(event.target.value)} placeholder="Backup-Skript" maxLength={48} autoFocus />
        </div>
        <div>
          <label className="label">Gültig für (Tage, 0 = unbegrenzt)</label>
          <input className="field w-32" inputMode="numeric" value={days} onChange={(event) => setDays(event.target.value)} />
        </div>
      </Dialog>

      <Dialog
        open={!!created}
        onClose={() => setCreated(null)}
        title="Dein neuer Schlüssel"
        description="Kopiere ihn jetzt. Aus Sicherheitsgründen siehst du ihn danach nicht mehr."
        footer={
          <>
            <Button onClick={() => navigator.clipboard.writeText(created ?? "").then(() => toast.success("Kopiert"))}>
              <Copy className="h-3.5 w-3.5" />
              Kopieren
            </Button>
            <Button variant="primary" onClick={() => setCreated(null)}>
              Fertig
            </Button>
          </>
        }
      >
        <code className="block break-all rounded-md bg-black/30 px-3 py-2 text-xs">{created}</code>
      </Dialog>

      <ConfirmDialog
        open={!!removing}
        onClose={() => setRemoving(null)}
        title={`„${removing?.name}“ widerrufen?`}
        text="Programme, die diesen Schlüssel nutzen, verlieren sofort den Zugriff."
        confirm="Widerrufen"
        danger
        onConfirm={async () => {
          if (!removing) return;
          await toast.run(() => api.delete(`/me/keys/${removing.id}`), "Widerrufen");
          mutate();
        }}
      />
    </Card>
  );
}

// ---------------------------------------------------------------- Sitzungen

function browserName(agent: string): string {
  const match = /(Edg|Firefox|Chrome|Safari|OPR)\/[\d.]+/.exec(agent);
  const name = match ? ({ Edg: "Edge", OPR: "Opera" } as Record<string, string>)[match[1]] ?? match[1] : "Browser";
  const system = /Windows/.test(agent) ? "Windows" : /Android/.test(agent) ? "Android" : /iPhone|iPad/.test(agent) ? "iOS" : /Mac OS/.test(agent) ? "macOS" : /Linux/.test(agent) ? "Linux" : "";
  return system ? `${name} auf ${system}` : name;
}

function SessionsCard() {
  const toast = useToast();
  const { data, mutate } = useApi<SessionInfo[]>("/me/sessions");
  return (
    <Card>
      <CardHeader title="Angemeldete Geräte" description="Beende Sitzungen, die du nicht kennst." />
      {!data ? (
        <Loading />
      ) : (
        <div className="divide-y divide-white/[0.05]">
          {data.map((session) => (
            <div key={session.id} className="flex flex-wrap items-center gap-x-4 gap-y-1 px-5 py-3">
              <div className="min-w-[180px] flex-1">
                <p className="text-sm font-medium">
                  {browserName(session.userAgent)} {session.current && <Badge tone="good">Dieses Gerät</Badge>}
                </p>
                <p className="text-xs text-muted">
                  {session.ip || "unbekannte Adresse"} · angemeldet {ago(session.createdAt)}
                </p>
              </div>
              {!session.current && (
                <Button
                  size="sm"
                  onClick={async () => {
                    await toast.run(() => api.delete(`/me/sessions/${session.id}`), "Sitzung beendet");
                    mutate();
                  }}
                >
                  <LogOut className="h-3.5 w-3.5" />
                  Beenden
                </Button>
              )}
            </div>
          ))}
        </div>
      )}
    </Card>
  );
}
