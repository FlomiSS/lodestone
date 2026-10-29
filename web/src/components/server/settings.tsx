"use client";

import { RefreshCw, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import { Badge, Button, Card, CardHeader } from "@/components/ui/primitives";
import { ConfirmDialog, useToast } from "@/components/ui/feedback";
import { api, useApi } from "@/lib/api";
import { megabytes } from "@/lib/format";
import type { Server } from "@/lib/types";

const memoryOptions = [256, 512, 1024, 2048, 3072, 4096, 6144, 8192, 12288, 16384];

function envToText(env?: Record<string, string>): string {
  return Object.entries(env ?? {})
    .map(([key, value]) => `${key}=${value}`)
    .join("\n");
}

function textToEnv(text: string): Record<string, string> {
  const env: Record<string, string> = {};
  for (const line of text.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) continue;
    const cut = trimmed.indexOf("=");
    if (cut > 0) env[trimmed.slice(0, cut).trim()] = trimmed.slice(cut + 1);
  }
  return env;
}

export function SettingsTab({ server, onChanged, onDeleted }: { server: Server; onChanged: () => void; onDeleted: () => void }) {
  const toast = useToast();
  const view = server.view;
  const offline = view?.state === "offline";

  const [name, setName] = useState(server.name);
  const [memory, setMemory] = useState(view?.memoryMB ?? 2048);
  const [cpu, setCpu] = useState(String(view?.cpu ?? 0));
  const [keep, setKeep] = useState(String(view?.backupKeep ?? 5));
  const [startup, setStartup] = useState(view?.startup ?? "");
  const [env, setEnv] = useState(envToText(view?.env));
  const [busy, setBusy] = useState(false);
  const [deleting, setDeleting] = useState(false);

  useEffect(() => {
    if (!view) return;
    setName(server.name);
    setMemory(view.memoryMB);
    setCpu(String(view.cpu));
    setKeep(String(view.backupKeep));
    setStartup(view.startup ?? "");
    setEnv(envToText(view.env));
    // nur beim Wechsel des Servers oder nach Speichern neu laden
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [server.id, view?.memoryMB, view?.cpu, view?.backupKeep, view?.startup]);

  async function save() {
    setBusy(true);
    const ok = await toast.run(
      () =>
        api.patch(`/servers/${server.id}`, {
          name,
          memoryMB: memory,
          cpu: Number(cpu) || 0,
          backupKeep: Number(keep) || 5,
          startup,
          env: textToEnv(env),
        }),
      "Einstellungen gespeichert",
    );
    setBusy(false);
    if (ok) onChanged();
  }

  return (
    <div className="space-y-5">
      <Card>
        <CardHeader
          title="Einstellungen"
          description={offline ? "Änderungen gelten ab dem nächsten Start." : "Zum Ändern muss der Server ausgeschaltet sein."}
          action={
            <Button variant="primary" size="sm" loading={busy} disabled={!offline} onClick={save}>
              Speichern
            </Button>
          }
        />
        <div className="grid gap-4 px-5 py-4 md:grid-cols-2">
          <div>
            <label className="label">Name</label>
            <input className="field" value={name} onChange={(event) => setName(event.target.value)} maxLength={48} disabled={!offline} />
          </div>
          <div>
            <label className="label">Arbeitsspeicher</label>
            <select className="field" value={memory} onChange={(event) => setMemory(Number(event.target.value))} disabled={!offline}>
              {[...new Set([...memoryOptions, memory])].sort((a, b) => a - b).map((value) => (
                <option key={value} value={value}>
                  {megabytes(value)}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="label">CPU-Kerne (0 = unbegrenzt)</label>
            <input className="field" inputMode="decimal" value={cpu} onChange={(event) => setCpu(event.target.value)} disabled={!offline} />
          </div>
          <div>
            <label className="label">Backups aufbewahren</label>
            <input className="field" inputMode="numeric" value={keep} onChange={(event) => setKeep(event.target.value)} disabled={!offline} />
          </div>
          <div className="md:col-span-2">
            <label className="label">Startzeile (leer = Standard der Software)</label>
            <textarea className="field font-mono text-xs" rows={3} value={startup} onChange={(event) => setStartup(event.target.value)} disabled={!offline} spellCheck={false} placeholder="exec java -Xms{{MEMORY}}M -Xmx{{MEMORY}}M -jar {{JAR}} nogui" />
          </div>
          <div className="md:col-span-2">
            <label className="label">Umgebungsvariablen (eine pro Zeile: NAME=Wert)</label>
            <textarea className="field font-mono text-xs" rows={3} value={env} onChange={(event) => setEnv(event.target.value)} disabled={!offline} spellCheck={false} />
          </div>
        </div>
      </Card>

      <Card>
        <CardHeader title="Anschlüsse" description="Ports der Maschine, über die der Server von außen erreichbar ist." />
        <div className="flex flex-wrap gap-2 px-5 py-4">
          {(view?.ports ?? []).length === 0 ? (
            <p className="text-sm text-muted">Der Server ist nicht nach außen veröffentlicht – er ist nur im Docker-Netzwerk erreichbar (z. B. hinter dem Proxy).</p>
          ) : (
            view?.ports?.map((port) => (
              <Badge key={`${port.container}/${port.protocol}`} tone="info">
                {port.host} → {port.container}/{port.protocol}
              </Badge>
            ))
          )}
        </div>
      </Card>

      <PublicAddress server={server} onDone={onChanged} />

      <Reinstall server={server} offline={!!offline} onDone={onChanged} />

      <Card className="border-bad/25">
        <CardHeader title="Server löschen" description="Löscht den Server samt Welten, Plugins und Backups unwiderruflich." />
        <div className="px-5 py-4">
          <Button variant="danger" onClick={() => setDeleting(true)}>
            <Trash2 className="h-4 w-4" />
            Server löschen
          </Button>
        </div>
      </Card>
      <ConfirmDialog
        open={deleting}
        onClose={() => setDeleting(false)}
        title={`„${server.name}“ löschen?`}
        text="Alle Dateien dieses Servers, auch Welten und Backups, werden unwiderruflich gelöscht."
        confirm="Endgültig löschen"
        danger
        onConfirm={async () => {
          if (await toast.run(() => api.delete(`/servers/${server.id}`), "Server gelöscht")) onDeleted();
        }}
      />
    </div>
  );
}

function PublicAddress({ server, onDone }: { server: Server; onDone: () => void }) {
  const toast = useToast();
  const [address, setAddress] = useState(server.publicAddress ?? "");
  const [busy, setBusy] = useState(false);
  useEffect(() => setAddress(server.publicAddress ?? ""), [server.id, server.publicAddress]);
  return (
    <Card>
      <CardHeader
        title="Öffentliche Adresse"
        description="Adresse, unter der Spieler den Server erreichen, z. B. ein playit-Tunnel (abc.joinmc.link:12345). Sie wird nur angezeigt; leer = Adresse der Maschine."
        action={
          <Button
            size="sm"
            loading={busy}
            disabled={address.trim() === (server.publicAddress ?? "")}
            onClick={async () => {
              setBusy(true);
              const ok = await toast.run(() => api.put(`/servers/${server.id}/public-address`, { address: address.trim() }), "Adresse gespeichert");
              setBusy(false);
              if (ok) onDone();
            }}
          >
            Speichern
          </Button>
        }
      />
      <div className="px-5 py-4">
        <input className="field font-mono text-xs" value={address} onChange={(event) => setAddress(event.target.value)} placeholder="mc.example.org oder abc.joinmc.link:12345" spellCheck={false} />
      </div>
    </Card>
  );
}

function Reinstall({ server, offline, onDone }: { server: Server; offline: boolean; onDone: () => void }) {
  const toast = useToast();
  const { data: versions } = useApi<string[]>(`/software/${server.software}/versions`);
  const [version, setVersion] = useState("");
  const [confirm, setConfirm] = useState(false);
  const current = server.view?.version;

  useEffect(() => {
    if (versions && !version) setVersion(current && versions.includes(current) ? current : versions[0]);
  }, [versions, current, version]);

  return (
    <Card>
      <CardHeader title="Version wechseln" description="Lädt die Server-Datei der gewählten Version neu. Welten, Plugins und Configs bleiben erhalten – sichere vorher ein Backup, besonders bei Downgrades." />
      <div className="flex flex-wrap items-end gap-3 px-5 py-4">
        <div className="w-56">
          <label className="label">Version (aktuell {current})</label>
          <select className="field" value={version} onChange={(event) => setVersion(event.target.value)} disabled={!offline || !versions}>
            {(versions ?? []).slice(0, 80).map((entry) => (
              <option key={entry} value={entry}>
                {entry}
              </option>
            ))}
          </select>
        </div>
        <Button disabled={!offline || !version || version === current} onClick={() => setConfirm(true)}>
          <RefreshCw className="h-4 w-4" />
          Version ändern
        </Button>
      </div>
      <ConfirmDialog
        open={confirm}
        onClose={() => setConfirm(false)}
        title={`Auf ${version} wechseln?`}
        text="Die Server-Datei wird ersetzt. Welten, die mit einer neueren Version gespeichert wurden, lassen sich mit einer älteren oft nicht mehr laden."
        confirm="Wechseln"
        onConfirm={async () => {
          if (await toast.run(() => api.post(`/servers/${server.id}/reinstall`, { software: server.software, version }), "Neue Version wird installiert")) onDone();
        }}
      />
    </Card>
  );
}
