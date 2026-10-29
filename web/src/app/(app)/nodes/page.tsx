"use client";

import { Cable, Cpu, Pencil, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { Badge, Button, Card, Dot, EmptyState, Meter, PageHeader } from "@/components/ui/primitives";
import { ConfirmDialog, Dialog, useToast } from "@/components/ui/feedback";
import { ErrorState, Loading } from "@/components/ui/async";
import { useUser } from "@/components/layout/shell";
import { api, useApi } from "@/lib/api";
import { bytes } from "@/lib/format";
import type { NodeInfo, PlayitStatus } from "@/lib/types";

export default function NodesPage() {
  const user = useUser();
  const { data, error, mutate } = useApi<NodeInfo[]>("/nodes", { refreshInterval: 10000 });
  const [editing, setEditing] = useState<NodeInfo | "new" | null>(null);
  const [removing, setRemoving] = useState<NodeInfo | null>(null);
  const toast = useToast();

  if (error) return <ErrorState error={error} retry={() => mutate()} />;
  if (!data) return <Loading />;

  return (
    <>
      <PageHeader
        title="Nodes"
        description="Maschinen mit Docker, auf denen die Server laufen. Auf jeder läuft der Lodestone-Node-Daemon."
        action={
          user.isAdmin && (
            <Button variant="primary" onClick={() => setEditing("new")}>
              <Plus className="h-4 w-4" />
              Node hinzufügen
            </Button>
          )
        }
      />
      {data.length === 0 ? (
        <Card>
          <EmptyState icon={<Cpu className="h-6 w-6" />} title="Noch kein Node" text={user.isAdmin ? "Starte den Node-Daemon auf einer Maschine und trage hier Adresse und Token ein." : "Ein Administrator muss einen Node einrichten."} />
        </Card>
      ) : (
        <div className="grid gap-4 lg:grid-cols-2">
          {data.map((node) => (
            <Card key={node.id} className="p-5">
              <div className="flex items-start justify-between gap-3">
                <div className="flex items-center gap-3">
                  <span className="grid h-9 w-9 place-items-center rounded-lg border border-white/[0.08] bg-white/[0.04] text-muted">
                    <Cpu className="h-4 w-4" />
                  </span>
                  <div>
                    <p className="text-sm font-semibold">{node.name}</p>
                    <p className="text-xs text-muted">{node.publicHost || "keine öffentliche Adresse"}</p>
                  </div>
                </div>
                <Badge tone={node.online ? "good" : "bad"}>
                  <Dot tone={node.online ? "good" : "bad"} />
                  {node.online ? "Online" : "Offline"}
                </Badge>
              </div>
              {node.online && node.system ? (
                <dl className="mt-4 grid grid-cols-2 gap-x-4 gap-y-2 text-xs">
                  <dt className="text-muted">Docker</dt>
                  <dd>{node.system.dockerVersion}</dd>
                  <dt className="text-muted">System</dt>
                  <dd>
                    {node.system.os} · {node.system.architecture}
                  </dd>
                  <dt className="text-muted">Kerne / RAM</dt>
                  <dd>
                    {node.system.cpus} · {bytes(node.system.memoryBytes)}
                  </dd>
                  <dt className="text-muted">Server</dt>
                  <dd>{node.serverCount}</dd>
                </dl>
              ) : (
                node.error && <p className="mt-4 rounded-md border border-bad/25 bg-bad/10 px-3 py-2 text-xs text-bad">{node.error}</p>
              )}
              {node.online && node.system && <MemoryPlanner node={node} />}
              {user.isAdmin && node.online && <PlayitCard node={node} />}
              {user.isAdmin && (
                <div className="mt-4 flex items-center justify-between gap-2 border-t border-white/[0.06] pt-3">
                  <span className="truncate font-mono text-[11px] text-muted">{node.url}</span>
                  <div className="flex gap-1">
                    <Button variant="ghost" size="icon-sm" onClick={() => setEditing(node)} aria-label="Bearbeiten">
                      <Pencil className="h-3.5 w-3.5" />
                    </Button>
                    <Button variant="ghost" size="icon-sm" onClick={() => setRemoving(node)} aria-label="Entfernen">
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                </div>
              )}
            </Card>
          ))}
        </div>
      )}
      <NodeDialog node={editing} onClose={() => setEditing(null)} onDone={() => mutate()} />
      <ConfirmDialog
        open={!!removing}
        onClose={() => setRemoving(null)}
        title={`„${removing?.name}“ entfernen?`}
        text="Der Node wird aus dem Panel entfernt. Das geht nur, wenn kein Server mehr auf ihm liegt. Der Daemon selbst wird nicht angehalten."
        confirm="Entfernen"
        danger
        onConfirm={async () => {
          if (!removing) return;
          await toast.run(() => api.delete(`/nodes/${removing.id}`), "Node entfernt");
          mutate();
        }}
      />
    </>
  );
}

/** Zeigt, wie viel Arbeitsspeicher die Server zusagen (Limits) und wie viel der Node tatsächlich noch frei hat. */
function MemoryPlanner({ node }: { node: NodeInfo }) {
  const total = node.system?.memoryBytes ?? 0;
  const committed = node.memoryCommitted ?? 0;
  const available = node.memoryAvailable ?? 0;
  if (!total) return null;
  const used = Math.max(0, total - available);
  const committedPercent = (committed / total) * 100;
  const usedPercent = (used / total) * 100;
  return (
    <div className="mt-4 space-y-3 border-t border-white/[0.06] pt-3">
      <div>
        <div className="mb-1.5 flex justify-between text-[11px] text-muted">
          <span>Zugesagt (Limits laufender Server)</span>
          <span className="tabular-nums text-ink/80">
            {bytes(committed)} von {bytes(total)}
          </span>
        </div>
        <Meter value={committedPercent} max={100} tone={committedPercent > 100 ? "bad" : committedPercent > 85 ? "warn" : "good"} />
      </div>
      <div>
        <div className="mb-1.5 flex justify-between text-[11px] text-muted">
          <span>Tatsächlich belegt</span>
          <span className="tabular-nums text-ink/80">
            {bytes(used)} · {bytes(available)} frei
          </span>
        </div>
        <Meter value={usedPercent} max={100} tone={usedPercent > 90 ? "bad" : usedPercent > 75 ? "warn" : "good"} />
      </div>
      {committedPercent > 100 && <p className="text-[11px] text-warn">Die Limits überschreiten den Speicher des Nodes. Das geht gut, solange nicht alle Server voll ausgelastet sind.</p>}
    </div>
  );
}

/** playit.gg: macht Server ohne Portfreigabe im Router erreichbar. Der Node startet den Agenten mit dem Schlüssel. */
function PlayitCard({ node }: { node: NodeInfo }) {
  const toast = useToast();
  const { data, mutate } = useApi<PlayitStatus>(`/nodes/${node.id}/playit`, { refreshInterval: 8000 });
  const [open, setOpen] = useState(false);
  const [secret, setSecret] = useState("");
  const [busy, setBusy] = useState(false);
  const [removing, setRemoving] = useState(false);

  return (
    <div className="mt-4 border-t border-white/[0.06] pt-3">
      <div className="flex items-center justify-between gap-2">
        <span className="flex items-center gap-2 text-xs">
          <Cable className="h-3.5 w-3.5 text-muted" />
          playit.gg
          {data?.configured ? <Badge tone={data.running ? "good" : "warn"}>{data.running ? "Verbunden" : "Gestoppt"}</Badge> : <Badge>Nicht eingerichtet</Badge>}
        </span>
        <div className="flex gap-1">
          <Button size="sm" variant="ghost" onClick={() => setOpen(true)}>
            {data?.configured ? "Schlüssel ändern" : "Einrichten"}
          </Button>
          {data?.configured && (
            <Button size="sm" variant="ghost" onClick={() => setRemoving(true)}>
              Entfernen
            </Button>
          )}
        </div>
      </div>
      {data?.error && <p className="mt-2 text-xs text-bad">{data.error}</p>}
      {data?.configured && data.logs.length > 0 && <pre className="mt-2 max-h-28 overflow-auto rounded-md bg-black/30 p-2 font-mono text-[10px] leading-relaxed text-muted">{data.logs.slice(-12).join("\n")}</pre>}
      <Dialog
        open={open}
        onClose={() => setOpen(false)}
        title="playit.gg einrichten"
        description="Spieler verbinden sich über eine playit-Adresse, du brauchst keine Portfreigabe im Router."
        footer={
          <>
            <Button variant="ghost" onClick={() => setOpen(false)}>
              Abbrechen
            </Button>
            <Button
              variant="primary"
              loading={busy}
              disabled={secret.trim().length < 16}
              onClick={async () => {
                setBusy(true);
                const ok = await toast.run(() => api.put(`/nodes/${node.id}/playit`, { secret: secret.trim() }), "playit-Agent gestartet");
                setBusy(false);
                if (ok) {
                  setOpen(false);
                  setSecret("");
                  mutate();
                }
              }}
            >
              Speichern und starten
            </Button>
          </>
        }
      >
        <ol className="list-decimal space-y-1 pl-5 text-xs text-muted">
          <li>
            Melde dich auf{" "}
            <a className="text-accent hover:underline" href="https://playit.gg/account/agents" target="_blank" rel="noreferrer">
              playit.gg
            </a>{" "}
            an und lege einen neuen Agenten an (Typ „Docker“).
          </li>
          <li>Kopiere den Geheimschlüssel (SECRET_KEY) hierher.</li>
          <li>Lege im playit-Dashboard einen Minecraft-Java-Tunnel an, der auf den Port des Servers auf dieser Maschine zeigt.</li>
          <li>Trage die Tunnel-Adresse beim Server unter Einstellungen → Öffentliche Adresse ein.</li>
        </ol>
        <div>
          <label className="label">Schlüssel</label>
          <input className="field font-mono text-xs" type="password" value={secret} onChange={(event) => setSecret(event.target.value)} autoComplete="off" />
        </div>
      </Dialog>
      <ConfirmDialog
        open={removing}
        onClose={() => setRemoving(false)}
        title="playit-Agent entfernen?"
        text="Der Agent wird gestoppt und der Schlüssel vergessen. Die Tunnel-Adressen funktionieren dann nicht mehr."
        confirm="Entfernen"
        danger
        onConfirm={async () => {
          await toast.run(() => api.delete(`/nodes/${node.id}/playit`), "Agent entfernt");
          mutate();
        }}
      />
    </div>
  );
}

function NodeDialog({ node, onClose, onDone }: { node: NodeInfo | "new" | null; onClose: () => void; onDone: () => void }) {
  const toast = useToast();
  const existing = node && node !== "new" ? node : null;
  const [form, setForm] = useState({ name: "", url: "", token: "", publicHost: "" });
  const [busy, setBusy] = useState(false);
  const [seen, setSeen] = useState<NodeInfo | "new" | null>(null);

  if (node !== seen) {
    setSeen(node);
    setForm({ name: existing?.name ?? "", url: existing?.url ?? "", token: "", publicHost: existing?.publicHost ?? "" });
  }

  const set = (key: keyof typeof form) => (event: React.ChangeEvent<HTMLInputElement>) => setForm((current) => ({ ...current, [key]: event.target.value }));

  return (
    <Dialog
      open={!!node}
      onClose={onClose}
      title={existing ? "Node bearbeiten" : "Node hinzufügen"}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Abbrechen
          </Button>
          <Button
            variant="primary"
            loading={busy}
            disabled={!form.name.trim() || !form.url.trim() || (!existing && !form.token.trim())}
            onClick={async () => {
              setBusy(true);
              const ok = await toast.run(() => (existing ? api.patch(`/nodes/${existing.id}`, form) : api.post("/nodes", form)), existing ? "Gespeichert" : "Node hinzugefügt");
              setBusy(false);
              if (ok) {
                onClose();
                onDone();
              }
            }}
          >
            {existing ? "Speichern" : "Hinzufügen"}
          </Button>
        </>
      }
    >
      <div>
        <label className="label">Name</label>
        <input className="field" value={form.name} onChange={set("name")} placeholder="home" maxLength={48} autoFocus />
      </div>
      <div>
        <label className="label">Adresse des Daemons</label>
        <input className="field font-mono text-xs" value={form.url} onChange={set("url")} placeholder="http://node.example.org:8480" />
      </div>
      <div>
        <label className="label">Token {existing && "(leer lassen, um es zu behalten)"}</label>
        <input className="field font-mono text-xs" type="password" value={form.token} onChange={set("token")} autoComplete="off" />
      </div>
      <div>
        <label className="label">Öffentliche Adresse (für Spieler)</label>
        <input className="field" value={form.publicHost} onChange={set("publicHost")} placeholder="mc.example.org" />
        <p className="mt-1 text-xs text-muted">Wird für die Beitritts-Adresse und die Voicechat-Konfiguration verwendet.</p>
      </div>
    </Dialog>
  );
}
