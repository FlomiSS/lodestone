"use client";

import Link from "next/link";
import { AlertTriangle, Bell, Info, Pencil, Plus, Send, Trash2, XCircle } from "lucide-react";
import { useState } from "react";
import { Badge, Button, Card, CardHeader, Checkbox, EmptyState, Switch } from "@/components/ui/primitives";
import { ConfirmDialog, Dialog, useToast } from "@/components/ui/feedback";
import { api, useApi } from "@/lib/api";
import { ago, dateTime } from "@/lib/format";
import { roleRank, type Project, type ProjectEvent, type Webhook } from "@/lib/types";

export const eventLabels: Record<string, string> = {
  crash: "Absturz",
  restart: "Neustart nach Absturz",
  crashloop: "Absturzschleife",
  backup: "Backup fertig",
  backup_failed: "Backup fehlgeschlagen",
  schedule_failed: "Zeitplan fehlgeschlagen",
  node_offline: "Node nicht erreichbar",
  node_online: "Node wieder erreichbar",
  lowmemory: "Wenig Arbeitsspeicher",
  scale: "Skalierung",
};

const defaults = ["crash", "crashloop", "backup_failed", "schedule_failed", "node_offline", "lowmemory"];

export function EventsTab({ project }: { project: Project }) {
  const canManage = roleRank[project.myRole] >= roleRank.admin;
  const { data } = useApi<ProjectEvent[]>(`/projects/${project.id}/events`, { refreshInterval: 10000 });
  const names = new Map(project.servers.map((server) => [server.id, server.name]));

  return (
    <div className="space-y-5">
      {canManage && <WebhooksCard project={project} />}
      <Card>
        <CardHeader title="Verlauf" description="Abstürze, Neustarts, Backups und Meldungen der letzten 30 Tage." />
        {!data ? null : data.length === 0 ? (
          <EmptyState icon={<Bell className="h-6 w-6" />} title="Noch nichts passiert" text="Hier erscheinen Abstürze, automatische Neustarts, Backups und Ausfälle." />
        ) : (
          <div className="divide-y divide-white/[0.05]">
            {data.map((event) => (
              <div key={event.id} className="flex items-start gap-3 px-5 py-3">
                {event.level === "error" ? <XCircle className="mt-0.5 h-4 w-4 shrink-0 text-bad" /> : event.level === "warn" ? <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warn" /> : <Info className="mt-0.5 h-4 w-4 shrink-0 text-info" />}
                <div className="min-w-0 flex-1">
                  <p className="text-sm">{event.message}</p>
                  <p className="mt-0.5 text-xs text-muted">
                    <span title={dateTime(event.time)}>{ago(event.time)}</span> · {eventLabels[event.kind] ?? event.kind}
                    {event.serverId && names.has(event.serverId) && (
                      <>
                        {" · "}
                        <Link href={`/server/?id=${event.serverId}`} className="hover:text-ink hover:underline">
                          {names.get(event.serverId)}
                        </Link>
                      </>
                    )}
                  </p>
                </div>
              </div>
            ))}
          </div>
        )}
      </Card>
    </div>
  );
}

// ---------------------------------------------------------------- Webhooks

function WebhooksCard({ project }: { project: Project }) {
  const toast = useToast();
  const { data, mutate } = useApi<Webhook[]>(`/projects/${project.id}/webhooks`);
  const [editing, setEditing] = useState<Webhook | "new" | null>(null);
  const [removing, setRemoving] = useState<Webhook | null>(null);

  return (
    <Card>
      <CardHeader
        title="Benachrichtigungen"
        description="Schickt Meldungen an Discord oder eine eigene Adresse (z. B. n8n, ntfy, Slack-Brücken). Webhooks an das interne Netz sind aus Sicherheitsgründen gesperrt."
        action={
          <Button size="sm" onClick={() => setEditing("new")}>
            <Plus className="h-3.5 w-3.5" />
            Webhook
          </Button>
        }
      />
      {!data ? null : data.length === 0 ? (
        <p className="px-5 py-6 text-center text-xs text-muted">Noch kein Webhook. Lege einen an, damit du bei einem Absturz sofort Bescheid weißt.</p>
      ) : (
        <div className="divide-y divide-white/[0.05]">
          {data.map((hook) => (
            <div key={hook.id} className="flex flex-wrap items-center gap-x-4 gap-y-1 px-5 py-3">
              <div className="min-w-[180px] flex-1">
                <p className="text-sm font-medium">
                  {hook.name} <Badge tone="info">{hook.kind === "discord" ? "Discord" : "Allgemein"}</Badge>
                </p>
                <p className="text-xs text-muted">
                  {hook.events.length} Ereignisse
                  {hook.lastStatus && (
                    <>
                      {" · "}
                      <span className={hook.lastStatus.startsWith("OK") ? "text-good" : "text-bad"}>{hook.lastStatus}</span>
                    </>
                  )}
                </p>
              </div>
              <Switch
                checked={hook.enabled}
                onChange={async (value) => {
                  await toast.run(() => api.patch(`/webhooks/${hook.id}`, { enabled: value }));
                  mutate();
                }}
              />
              <Button
                size="sm"
                onClick={async () => {
                  try {
                    const result = await api.post<{ ok: boolean; status: string }>(`/webhooks/${hook.id}/test`);
                    if (result.ok) toast.success("Testnachricht gesendet");
                    else toast.error(result.status);
                  } catch (error) {
                    toast.error(error instanceof Error ? error.message : "Fehler");
                  }
                  mutate();
                }}
              >
                <Send className="h-3.5 w-3.5" />
                Test
              </Button>
              <Button variant="ghost" size="icon-sm" onClick={() => setEditing(hook)} aria-label="Bearbeiten">
                <Pencil className="h-3.5 w-3.5" />
              </Button>
              <Button variant="ghost" size="icon-sm" onClick={() => setRemoving(hook)} aria-label="Löschen">
                <Trash2 className="h-3.5 w-3.5" />
              </Button>
            </div>
          ))}
        </div>
      )}
      <WebhookDialog project={project} hook={editing} onClose={() => setEditing(null)} onDone={() => mutate()} />
      <ConfirmDialog
        open={!!removing}
        onClose={() => setRemoving(null)}
        title={`„${removing?.name}“ löschen?`}
        text="Es werden keine Meldungen mehr an diese Adresse geschickt."
        confirm="Löschen"
        danger
        onConfirm={async () => {
          if (!removing) return;
          await toast.run(() => api.delete(`/webhooks/${removing.id}`), "Gelöscht");
          mutate();
        }}
      />
    </Card>
  );
}

function WebhookDialog({ project, hook, onClose, onDone }: { project: Project; hook: Webhook | "new" | null; onClose: () => void; onDone: () => void }) {
  const toast = useToast();
  const existing = hook && hook !== "new" ? hook : null;
  const [name, setName] = useState("");
  const [kind, setKind] = useState<"discord" | "generic">("discord");
  const [url, setUrl] = useState("");
  const [events, setEvents] = useState<string[]>(defaults);
  const [seen, setSeen] = useState<Webhook | "new" | null>(null);
  const [busy, setBusy] = useState(false);

  if (hook !== seen) {
    setSeen(hook);
    setName(existing?.name ?? "");
    setKind(existing?.kind ?? "discord");
    setUrl(existing?.url ?? "");
    setEvents(existing?.events ?? defaults);
  }

  return (
    <Dialog
      open={hook !== null}
      onClose={onClose}
      title={existing ? "Webhook bearbeiten" : "Webhook anlegen"}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Abbrechen
          </Button>
          <Button
            variant="primary"
            loading={busy}
            disabled={!name.trim() || !url.trim() || events.length === 0}
            onClick={async () => {
              setBusy(true);
              const body = { name, kind, url, events };
              const ok = await toast.run(() => (existing ? api.patch(`/webhooks/${existing.id}`, body) : api.post(`/projects/${project.id}/webhooks`, body)), existing ? "Gespeichert" : "Webhook angelegt");
              setBusy(false);
              if (ok) {
                onClose();
                onDone();
              }
            }}
          >
            {existing ? "Speichern" : "Anlegen"}
          </Button>
        </>
      }
    >
      <div className="grid gap-3 sm:grid-cols-2">
        <div>
          <label className="label">Name</label>
          <input className="field" value={name} onChange={(event) => setName(event.target.value)} maxLength={48} placeholder="Discord-Kanal #server" autoFocus />
        </div>
        <div>
          <label className="label">Art</label>
          <select className="field" value={kind} onChange={(event) => setKind(event.target.value as "discord" | "generic")}>
            <option value="discord">Discord</option>
            <option value="generic">Allgemein (JSON)</option>
          </select>
        </div>
      </div>
      <div>
        <label className="label">Adresse (https://…)</label>
        <input className="field font-mono text-xs" value={url} onChange={(event) => setUrl(event.target.value)} placeholder={kind === "discord" ? "https://discord.com/api/webhooks/…" : "https://example.org/hook"} />
        {kind === "discord" && <p className="mt-1 text-xs text-muted">In Discord: Kanal → Einstellungen → Integrationen → Webhooks → Neuer Webhook → URL kopieren.</p>}
      </div>
      <div>
        <label className="label">Wobei melden?</label>
        <div className="grid gap-2 sm:grid-cols-2">
          {Object.entries(eventLabels).map(([id, label]) => (
            <label key={id} className="flex cursor-pointer items-center gap-2.5 text-sm">
              <Checkbox checked={events.includes(id)} onChange={(on) => setEvents((current) => (on ? [...current, id] : current.filter((entry) => entry !== id)))} label={label} />
              {label}
            </label>
          ))}
        </div>
      </div>
    </Dialog>
  );
}

