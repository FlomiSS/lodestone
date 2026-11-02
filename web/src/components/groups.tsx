"use client";

import Link from "next/link";
import { Layers, Pencil, Plus, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import { Badge, Button, Card, Checkbox, EmptyState, Meter, Select, Switch } from "@/components/ui/primitives";
import { ConfirmDialog, Dialog, useToast } from "@/components/ui/feedback";
import { MemoryInput } from "@/components/memory-input";
import { StateBadge } from "@/components/status";
import { api, useApi } from "@/lib/api";
import { megabytes } from "@/lib/format";
import { roleRank, type Group, type Project, type SoftwareInfo } from "@/lib/types";

const backendSoftware = ["paper", "purpur", "folia", "leaf", "fabric", "neoforge"];

export function GroupsTab({ project, refresh }: { project: Project; refresh: () => void }) {
  const toast = useToast();
  const canManage = roleRank[project.myRole] >= roleRank.admin;
  const canOperate = roleRank[project.myRole] >= roleRank.operator;
  const [editing, setEditing] = useState<Group | "new" | null>(null);
  const [removing, setRemoving] = useState<Group | null>(null);
  const hasProxy = project.servers.some((server) => server.role === "proxy");

  return (
    <div className="space-y-5">
      <Card className="flex flex-wrap items-center justify-between gap-4 p-5">
        <div className="max-w-2xl">
          <p className="text-sm font-semibold">Gruppen skalieren sich selbst</p>
          <p className="mt-1 text-xs leading-relaxed text-muted">
            Eine Gruppe ist eine Vorlage für gleiche Server (z. B. Lobby). Das Panel hält zwischen Minimum und Maximum Instanzen am Laufen: Sind die Server zu
            80 % voll, kommt eine dazu; sind sie lange leer, wird eine überzählige gestoppt. Neue Instanzen tragen sich selbst im Proxy ein (Beitrittsreihenfolge
            und Forced Hosts wie bei allen Servern).
          </p>
        </div>
        {canManage && (
          <Button variant="primary" onClick={() => setEditing("new")} disabled={!hasProxy} title={hasProxy ? undefined : "Lege zuerst den Proxy an"}>
            <Plus className="h-4 w-4" />
            Gruppe anlegen
          </Button>
        )}
      </Card>

      {project.groups.length === 0 ? (
        <Card>
          <EmptyState icon={<Layers className="h-6 w-6" />} title="Noch keine Gruppe" text={hasProxy ? "Lege eine Gruppe an, z. B. „Lobby“ mit 1 bis 4 Instanzen." : "Lege zuerst den Proxy an, dann kannst du Gruppen anlegen."} />
        </Card>
      ) : (
        project.groups.map((group) => {
          const instances = project.servers.filter((server) => server.groupId === group.id);
          const capacity = Math.max(1, group.active * group.playersPerInstance);
          return (
            <Card key={group.id}>
              <div className="flex flex-wrap items-start justify-between gap-4 border-b border-white/[0.06] px-5 py-4">
                <div className="flex min-w-0 items-center gap-3">
                  <span className="grid h-9 w-9 shrink-0 place-items-center rounded-lg border border-white/[0.08] bg-white/[0.04] text-muted">
                    <Layers className="h-4 w-4" />
                  </span>
                  <div className="min-w-0">
                    <p className="truncate text-sm font-semibold">{group.name}</p>
                    <p className="truncate text-xs text-muted">
                      {group.software} {group.version} · {megabytes(group.memoryMB)} je Instanz · {group.dynamic ? "dynamisch" : "statisch"}
                    </p>
                  </div>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <Badge tone={group.active > 0 ? "good" : "neutral"}>
                    {group.active} aktiv · {group.minInstances}–{group.maxInstances}
                  </Badge>
                  <Badge tone="info">{group.players} Spieler</Badge>
                  {canOperate && (
                    <Button
                      size="sm"
                      disabled={instances.length >= group.maxInstances}
                      onClick={async () => {
                        await toast.run(() => api.post(`/groups/${group.id}/instances`), "Instanz wird angelegt");
                        refresh();
                      }}
                    >
                      <Plus className="h-3.5 w-3.5" />
                      Instanz
                    </Button>
                  )}
                  {canManage && (
                    <>
                      <Button variant="ghost" size="icon-sm" onClick={() => setEditing(group)} aria-label="Bearbeiten">
                        <Pencil className="h-3.5 w-3.5" />
                      </Button>
                      <Button variant="ghost" size="icon-sm" onClick={() => setRemoving(group)} aria-label="Löschen">
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </>
                  )}
                </div>
              </div>
              {group.note && <p className="border-b border-warn/25 bg-warn/10 px-5 py-2.5 text-xs text-warn">{group.note}</p>}
              <div className="px-5 py-3">
                <div className="mb-1.5 flex justify-between text-[11px] text-muted">
                  <span>Auslastung</span>
                  <span className="tabular-nums">
                    {group.players} / {group.active * group.playersPerInstance} Plätze
                  </span>
                </div>
                <Meter value={group.players} max={capacity} tone={group.players / capacity >= 0.8 ? "warn" : "good"} />
              </div>
              {instances.length === 0 ? (
                <p className="px-5 pb-5 text-xs text-muted">Noch keine Instanz – das Panel legt sie gleich an.</p>
              ) : (
                <div className="divide-y divide-white/[0.05] border-t border-white/[0.05]">
                  {instances.map((server) => (
                    <Link key={server.id} href={`/server/${server.id}/`} className="row-hover flex flex-wrap items-center gap-x-5 gap-y-1 px-5 py-2.5">
                      <span className="min-w-[120px] flex-1 truncate text-sm">{server.name}</span>
                      <StateBadge server={server} />
                      <span className="w-24 text-right text-xs tabular-nums text-muted">
                        {server.view?.stats && server.view.state === "running" ? `${server.view.stats.cpuPercent.toFixed(0)} % CPU` : "–"}
                      </span>
                    </Link>
                  ))}
                </div>
              )}
            </Card>
          );
        })
      )}

      <GroupDialog project={project} group={editing} onClose={() => setEditing(null)} onDone={refresh} />
      <ConfirmDialog
        open={!!removing}
        onClose={() => setRemoving(null)}
        title={`Gruppe „${removing?.name}“ löschen?`}
        text="Alle Instanzen der Gruppe werden gestoppt und samt Dateien gelöscht."
        confirm="Löschen"
        danger
        onConfirm={async () => {
          if (!removing) return;
          await toast.run(() => api.delete(`/groups/${removing.id}`), "Gruppe gelöscht");
          refresh();
        }}
      />
    </div>
  );
}

function GroupDialog({ project, group, onClose, onDone }: { project: Project; group: Group | "new" | null; onClose: () => void; onDone: () => void }) {
  const toast = useToast();
  const existing = group && group !== "new" ? group : null;
  const open = group !== null;
  const { data: software } = useApi<SoftwareInfo[]>(open ? "/software" : null);

  const [name, setName] = useState("");
  const [softwareId, setSoftwareId] = useState("paper");
  const [version, setVersion] = useState("");
  const [memory, setMemory] = useState(1536);
  const [min, setMin] = useState("1");
  const [max, setMax] = useState("4");
  const [perInstance, setPerInstance] = useState("20");
  const [idle, setIdle] = useState("5");
  const [inTry, setInTry] = useState(true);
  const [dynamic, setDynamic] = useState(true);
  const [plugins, setPlugins] = useState("");
  const [eula, setEula] = useState(false);
  const [busy, setBusy] = useState(false);
  const [seen, setSeen] = useState<Group | "new" | null>(null);

  // Beim Öffnen die Felder mit der Gruppe füllen
  if (group !== seen) {
    setSeen(group);
    setName(existing?.name ?? "");
    setSoftwareId(existing?.software ?? "paper");
    setVersion(existing?.version ?? "");
    setMemory(existing?.memoryMB ?? 1536);
    setMin(String(existing?.minInstances ?? 1));
    setMax(String(existing?.maxInstances ?? 4));
    setPerInstance(String(existing?.playersPerInstance ?? 20));
    setIdle(String(existing?.idleMinutes ?? 5));
    setInTry(existing?.inTry ?? true);
    setDynamic(existing?.dynamic ?? true);
    setPlugins(existing?.plugins.join(", ") ?? "");
    setEula(false);
  }

  const { data: versions } = useApi<string[]>(open && !existing ? `/software/${softwareId}/versions` : null);
  const effectiveVersion = existing ? version : versions && versions.includes(version) ? version : (versions?.[0] ?? "");
  const options = (software ?? []).filter((item) => (item.canBeBackend ?? backendSoftware.includes(item.id)));

  useEffect(() => {
    if (!existing && software && !options.some((item) => item.id === softwareId)) setSoftwareId("paper");
  }, [existing, software, options, softwareId]);

  async function save() {
    setBusy(true);
    const body = {
      name,
      memoryMB: memory,
      minInstances: Number(min),
      maxInstances: Number(max),
      playersPerInstance: Number(perInstance),
      idleMinutes: Number(idle),
      inTry,
      dynamic,
      plugins: plugins.split(",").map((entry) => entry.trim()).filter(Boolean),
    };
    const ok = await toast.run(
      () => (existing ? api.patch(`/groups/${existing.id}`, body) : api.post(`/projects/${project.id}/groups`, { ...body, software: softwareId, version: effectiveVersion, eula })),
      existing ? "Gruppe gespeichert" : "Gruppe angelegt",
    );
    setBusy(false);
    if (ok) {
      onClose();
      onDone();
    }
  }

  const valid = name.trim() && Number(max) >= 1 && Number(max) >= Number(min) && (existing || (effectiveVersion && eula));

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={existing ? `Gruppe „${existing.name}“ bearbeiten` : "Gruppe anlegen"}
      description={existing ? "Änderungen gelten für neue Instanzen." : undefined}
      wide
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Abbrechen
          </Button>
          <Button variant="primary" loading={busy} disabled={!valid} onClick={save}>
            {existing ? "Speichern" : "Anlegen"}
          </Button>
        </>
      }
    >
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="sm:col-span-2">
          <label className="label">Name</label>
          <input className="field" value={name} onChange={(event) => setName(event.target.value)} placeholder="Lobby" maxLength={32} autoFocus />
          <p className="mt-1 text-xs text-muted">Die Instanzen heißen {name.trim() ? name.trim() : "Lobby"}-1, -2 …</p>
        </div>
        <div>
          <label className="label">Software</label>
          <Select
            value={softwareId}
            onChange={setSoftwareId}
            disabled={!!existing}
            options={options.map((item) => ({ value: item.id, label: item.name }))}
          />
        </div>
        <div>
          <label className="label">Version</label>
          {existing ? (
            <input className="field" value={version} onChange={(event) => setVersion(event.target.value)} />
          ) : (
            <Select
              value={effectiveVersion}
              onChange={setVersion}
              disabled={!versions}
              options={(versions ?? []).slice(0, 60).map((entry) => ({ value: entry, label: entry }))}
            />
          )}
        </div>
        <div>
          <label className="label">Arbeitsspeicher je Instanz</label>
          <MemoryInput value={memory} onChange={setMemory} max={65536} />
        </div>
        <div>
          <label className="label">Spieler pro Instanz</label>
          <input className="field" inputMode="numeric" value={perInstance} onChange={(event) => setPerInstance(event.target.value)} />
        </div>
        <div>
          <label className="label">Mindestens (Instanzen)</label>
          <input className="field" inputMode="numeric" value={min} onChange={(event) => setMin(event.target.value)} />
        </div>
        <div>
          <label className="label">Höchstens (Instanzen)</label>
          <input className="field" inputMode="numeric" value={max} onChange={(event) => setMax(event.target.value)} />
        </div>
        <div>
          <label className="label">Leerlauf bis zum Stoppen (Minuten)</label>
          <input className="field" inputMode="numeric" value={idle} onChange={(event) => setIdle(event.target.value)} />
        </div>
        <div>
          <label className="label">Plugins von Modrinth (kommagetrennt)</label>
          <input className="field" value={plugins} onChange={(event) => setPlugins(event.target.value)} placeholder="luckperms, essentialsx" />
        </div>
      </div>
      <div className="space-y-2.5 rounded-lg border border-white/[0.07] bg-white/[0.02] p-3">
        <label className="flex items-center justify-between gap-3 text-sm">
          <span>
            In die Beitrittsreihenfolge aufnehmen
            <span className="block text-xs text-muted">Neue Spieler landen auf diesen Servern (z. B. bei einer Lobby).</span>
          </span>
          <Switch checked={inTry} onChange={setInTry} />
        </label>
        <label className="flex items-center justify-between gap-3 text-sm">
          <span>
            Dynamisch
            <span className="block text-xs text-muted">Überzählige Instanzen werden samt Dateien gelöscht. Sonst werden sie nur gestoppt und bleiben erhalten.</span>
          </span>
          <Switch checked={dynamic} onChange={setDynamic} />
        </label>
      </div>
      {!existing && (
        <label className="flex cursor-pointer items-start gap-2.5 text-xs text-muted">
          <Checkbox checked={eula} onChange={setEula} label="EULA" />
          <span>
            Ich akzeptiere die{" "}
            <a className="text-accent hover:underline" href="https://aka.ms/MinecraftEULA" target="_blank" rel="noreferrer">
              Minecraft-EULA
            </a>{" "}
            für alle Instanzen.
          </span>
        </label>
      )}
    </Dialog>
  );
}
