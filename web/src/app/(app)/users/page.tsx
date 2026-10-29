"use client";

import { KeyRound, Plus, ShieldCheck, Trash2, UserX } from "lucide-react";
import { useState } from "react";
import { Badge, Button, Card, PageHeader, RowMenu, Switch } from "@/components/ui/primitives";
import { ConfirmDialog, Dialog, useToast } from "@/components/ui/feedback";
import { ErrorState, Loading } from "@/components/ui/async";
import { useUser } from "@/components/layout/shell";
import { PixelAvatar } from "@/components/ui/mc";
import { api, useApi } from "@/lib/api";
import { dateTime } from "@/lib/format";
import type { User } from "@/lib/types";

export default function UsersPage() {
  const me = useUser();
  const toast = useToast();
  const { data, error, mutate } = useApi<User[]>(me.isAdmin ? "/users" : null);
  const [creating, setCreating] = useState(false);
  const [resetting, setResetting] = useState<User | null>(null);
  const [removing, setRemoving] = useState<User | null>(null);

  if (!me.isAdmin) return <ErrorState error={new Error("Dafür braucht es einen Administrator.")} />;
  if (error) return <ErrorState error={error} retry={() => mutate()} />;
  if (!data) return <Loading />;

  async function patch(user: User, body: Partial<Pick<User, "isAdmin" | "disabled">>, message: string) {
    await toast.run(() => api.patch(`/users/${user.id}`, body), message);
    mutate();
  }

  return (
    <>
      <PageHeader
        title="Benutzer"
        description="Wer sich am Panel anmelden darf. Zugriff auf Projekte vergibst du im jeweiligen Projekt unter „Mitglieder“."
        action={
          <Button variant="primary" onClick={() => setCreating(true)}>
            <Plus className="h-4 w-4" />
            Benutzer anlegen
          </Button>
        }
      />
      <Card className="divide-y divide-white/[0.05]">
        {data.map((user) => (
          <div key={user.id} className="flex flex-wrap items-center gap-4 px-5 py-3.5">
            <PixelAvatar name={user.username} size={32} />
            <div className="min-w-[160px] flex-1">
              <p className="text-sm font-medium">
                {user.username}
                {user.id === me.id && <span className="ml-2 text-xs text-muted">(du)</span>}
              </p>
              <p className="text-xs text-muted">Seit {dateTime(user.createdAt)}</p>
            </div>
            {user.isAdmin && (
              <Badge tone="accent">
                <ShieldCheck className="h-3 w-3" />
                Administrator
              </Badge>
            )}
            {user.disabled && <Badge tone="bad">Gesperrt</Badge>}
            <RowMenu
              items={[
                { label: "Passwort zurücksetzen", icon: <KeyRound className="h-3.5 w-3.5" />, onClick: () => setResetting(user) },
                { label: user.isAdmin ? "Administratorrechte entziehen" : "Zum Administrator machen", icon: <ShieldCheck className="h-3.5 w-3.5" />, disabled: user.id === me.id, onClick: () => patch(user, { isAdmin: !user.isAdmin }, "Gespeichert") },
                { label: user.disabled ? "Entsperren" : "Sperren", icon: <UserX className="h-3.5 w-3.5" />, disabled: user.id === me.id, onClick: () => patch(user, { disabled: !user.disabled }, user.disabled ? "Entsperrt" : "Gesperrt") },
                "separator",
                { label: "Löschen", icon: <Trash2 className="h-3.5 w-3.5" />, danger: true, disabled: user.id === me.id, onClick: () => setRemoving(user) },
              ]}
            />
          </div>
        ))}
      </Card>

      <CreateDialog open={creating} onClose={() => setCreating(false)} onDone={() => mutate()} />
      <ResetDialog user={resetting} onClose={() => setResetting(null)} />
      <ConfirmDialog
        open={!!removing}
        onClose={() => setRemoving(null)}
        title={`„${removing?.username}“ löschen?`}
        text="Der Benutzer und seine Mitgliedschaften werden gelöscht. Projekte bleiben bestehen."
        confirm="Löschen"
        danger
        onConfirm={async () => {
          if (!removing) return;
          await toast.run(() => api.delete(`/users/${removing.id}`), "Benutzer gelöscht");
          mutate();
        }}
      />
    </>
  );
}

function CreateDialog({ open, onClose, onDone }: { open: boolean; onClose: () => void; onDone: () => void }) {
  const toast = useToast();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [isAdmin, setIsAdmin] = useState(false);
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title="Benutzer anlegen"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Abbrechen
          </Button>
          <Button
            variant="primary"
            disabled={username.trim().length < 3 || password.length < 8}
            onClick={async () => {
              if (await toast.run(() => api.post("/users", { username: username.trim(), password, isAdmin }), "Benutzer angelegt")) {
                setUsername("");
                setPassword("");
                setIsAdmin(false);
                onClose();
                onDone();
              }
            }}
          >
            Anlegen
          </Button>
        </>
      }
    >
      <div>
        <label className="label">Benutzername</label>
        <input className="field" value={username} onChange={(event) => setUsername(event.target.value)} autoComplete="off" autoFocus />
      </div>
      <div>
        <label className="label">Startpasswort (mindestens 8 Zeichen)</label>
        <input className="field" type="text" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="off" />
      </div>
      <label className="flex items-center justify-between gap-3 text-sm">
        <span>Administrator (voller Zugriff auf alles)</span>
        <Switch checked={isAdmin} onChange={setIsAdmin} />
      </label>
    </Dialog>
  );
}

function ResetDialog({ user, onClose }: { user: User | null; onClose: () => void }) {
  const toast = useToast();
  const [password, setPassword] = useState("");
  return (
    <Dialog
      open={!!user}
      onClose={onClose}
      title={`Passwort von ${user?.username} zurücksetzen`}
      description="Alle Sitzungen des Benutzers enden."
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Abbrechen
          </Button>
          <Button
            variant="primary"
            disabled={password.length < 8}
            onClick={async () => {
              if (user && (await toast.run(() => api.post(`/users/${user.id}/password`, { password }), "Passwort geändert"))) {
                setPassword("");
                onClose();
              }
            }}
          >
            Setzen
          </Button>
        </>
      }
    >
      <div>
        <label className="label">Neues Passwort (mindestens 8 Zeichen)</label>
        <input className="field" type="text" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="off" autoFocus />
      </div>
    </Dialog>
  );
}
