"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { AlertTriangle, ArrowDown, ArrowUp, Check, Copy, Network, Plus, Server as ServerIcon, Trash2, UserPlus } from "lucide-react";
import { Suspense, useEffect, useState } from "react";
import { Badge, Button, Card, CardHeader, EmptyState, PageHeader, Switch, Tabs } from "@/components/ui/primitives";
import { ConfirmDialog, Dialog, useToast } from "@/components/ui/feedback";
import { ErrorState, Loading } from "@/components/ui/async";
import { AddServerDialog } from "@/components/add-server";
import { GroupsTab } from "@/components/groups";
import { ChartCard, RangeTabs } from "@/components/server/stats";
import { SeriesChart } from "@/components/ui/chart";
import { PowerButtons, StateBadge } from "@/components/status";
import { useUser } from "@/components/layout/shell";
import { PixelAvatar } from "@/components/ui/mc";
import { api, useApi } from "@/lib/api";
import { megabytes } from "@/lib/format";
import { roleLabel, roleRank, type Member, type Metrics, type Project, type Range, type Role, type Server, type SyncResult } from "@/lib/types";

export default function ProjectPage() {
  return (
    <Suspense fallback={<Loading />}>
      <ProjectView />
    </Suspense>
  );
}

type Tab = "servers" | "network" | "groups" | "members" | "settings";

function ProjectView() {
  const id = useSearchParams().get("id");
  const { data: project, error, mutate } = useApi<Project>(id ? `/projects/${id}` : null, { refreshInterval: 4000 });
  const [tab, setTab] = useState<Tab>("servers");
  const [adding, setAdding] = useState(false);

  if (!id) return <ErrorState error={new Error("Kein Projekt angegeben")} />;
  if (error) return <ErrorState error={error} retry={() => mutate()} />;
  if (!project) return <Loading />;

  const rank = roleRank[project.myRole];
  const canManage = rank >= roleRank.admin;
  const network = project.kind === "network";
  const canAdd = canManage && (network || project.servers.length === 0);

  const tabs: { id: Tab; label: string; count?: number }[] = [
    { id: "servers", label: "Server", count: project.servers.length },
    ...(network ? [{ id: "network" as const, label: "Netzwerk" }, { id: "groups" as const, label: "Gruppen", count: project.groups.length }] : []),
    { id: "members", label: "Mitglieder", count: project.members?.length },
    ...(canManage ? [{ id: "settings" as const, label: "Einstellungen" }] : []),
  ];

  return (
    <>
      <PageHeader
        title={project.name}
        description={project.description || (network ? "Netzwerk mit Velocity-Proxy" : "Einzelner Server")}
        action={
          canAdd && (
            <Button variant="primary" onClick={() => setAdding(true)}>
              <Plus className="h-4 w-4" />
              {network && !project.servers.some((server) => server.role === "proxy") ? "Proxy hinzufügen" : "Server hinzufügen"}
            </Button>
          )
        }
      />

      {project.problems && project.problems.length > 0 && project.servers.length > 0 && (
        <div className="mb-5 flex gap-3 rounded-lg border border-warn/25 bg-warn/10 px-4 py-3 text-xs text-warn">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
          <ul className="space-y-0.5">
            {project.problems.map((problem) => (
              <li key={problem}>{problem}</li>
            ))}
          </ul>
        </div>
      )}

      <div className="mb-5 overflow-x-auto">
        <Tabs value={tab} onChange={setTab} items={tabs} />
      </div>

      {tab === "servers" && <ServersTab project={project} refresh={() => mutate()} onAdd={canAdd ? () => setAdding(true) : undefined} />}
      {tab === "network" && network && <NetworkTab project={project} canManage={canManage} refresh={() => mutate()} />}
      {tab === "groups" && network && <GroupsTab project={project} refresh={() => mutate()} />}
      {tab === "members" && <MembersTab project={project} refresh={() => mutate()} />}
      {tab === "settings" && canManage && <SettingsTab project={project} refresh={() => mutate()} />}

      <AddServerDialog project={project} open={adding} onClose={() => setAdding(false)} onCreated={() => mutate()} />
    </>
  );
}

// ---------------------------------------------------------------- Server

function ServersTab({ project, refresh, onAdd }: { project: Project; refresh: () => void; onAdd?: () => void }) {
  if (project.servers.length === 0) {
    return (
      <Card>
        <EmptyState
          icon={project.kind === "network" ? <Network className="h-6 w-6" /> : <ServerIcon className="h-6 w-6" />}
          title="Noch kein Server"
          text={project.kind === "network" ? "Beginne mit dem Velocity-Proxy, danach kommen die Server dahinter." : "Lege den Server dieses Projekts an."}
        />
        {onAdd && (
          <div className="flex justify-center pb-10">
            <Button variant="primary" onClick={onAdd}>
              <Plus className="h-4 w-4" />
              {project.kind === "network" ? "Proxy hinzufügen" : "Server anlegen"}
            </Button>
          </div>
        )}
      </Card>
    );
  }
  const canOperate = roleRank[project.myRole] >= roleRank.operator;
  const groupName = (id?: string) => project.groups.find((group) => group.id === id)?.name;
  return (
    <div className="space-y-3">
      <ProjectStats project={project} />
      {project.servers.map((server) => (
        <Card key={server.id} className="p-4">
          <div className="flex flex-wrap items-center gap-x-6 gap-y-3">
            <Link href={`/server/?id=${server.id}`} className="flex min-w-[200px] flex-1 items-center gap-3">
              <span className="grid h-9 w-9 shrink-0 place-items-center rounded-lg border border-white/[0.08] bg-white/[0.04] text-muted">
                <ServerIcon className="h-4 w-4" />
              </span>
              <div className="min-w-0">
                <p className="truncate text-sm font-semibold hover:underline">{server.name}</p>
                <p className="truncate text-xs text-muted">
                  {server.software} {server.view?.version} · {server.view ? megabytes(server.view.memoryMB) : "–"} · {server.nodeName}
                </p>
              </div>
            </Link>
            <div className="flex items-center gap-2">
              {server.role === "proxy" && <Badge tone="accent">Proxy</Badge>}
              {server.role === "backend" && <Badge>{server.inTry ? `Beitritt #${server.tryPosition + 1}` : "Backend"}</Badge>}
              {server.groupId && <Badge tone="info">Gruppe {groupName(server.groupId)}</Badge>}
              {server.voicechat && <Badge tone="info">Voicechat</Badge>}
              <StateBadge server={server} />
            </div>
            <div className="min-w-[150px] text-xs">
              {server.address ? <span className="font-mono text-ink/80">{server.address}</span> : <span className="text-muted">{server.role === "backend" ? "nur im Netzwerk" : "–"}</span>}
              {server.view?.stats && server.view.state === "running" && (
                <p className="mt-0.5 text-muted">
                  {server.view.stats.cpuPercent.toFixed(0)} % CPU · {Math.round(server.view.stats.memoryBytes / 1024 ** 2)} MB
                </p>
              )}
            </div>
            <PowerButtons server={server} onDone={refresh} enabled={canOperate} />
          </div>
          {server.view?.lastError && server.view.state === "offline" && <p className="mt-3 rounded-md border border-bad/25 bg-bad/10 px-3 py-2 text-xs text-bad">{server.view.lastError}</p>}
        </Card>
      ))}
    </div>
  );
}

// ---------------------------------------------------------------- Netzwerk

interface Entry {
  id: string;
  name: string;
  software: string;
  inTry: boolean;
  forcedHost: string;
  voicechat: boolean;
  state?: string;
}

function NetworkTab({ project, canManage, refresh }: { project: Project; canManage: boolean; refresh: () => void }) {
  const toast = useToast();
  const proxy = project.servers.find((server) => server.role === "proxy");
  const initial = (): Entry[] =>
    project.servers
      .filter((server) => server.role === "backend")
      .sort((a, b) => a.tryPosition - b.tryPosition)
      .map((server) => ({ id: server.id, name: server.name, software: server.software, inTry: server.inTry, forcedHost: server.forcedHost, voicechat: server.voicechat, state: server.view?.state }));

  const [entries, setEntries] = useState<Entry[]>(initial);
  const [dirty, setDirty] = useState(false);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<SyncResult | null>(null);
  const [copied, setCopied] = useState(false);
  const signature = project.servers.map((server) => `${server.id}:${server.role}`).join(",");

  // Neu laden, wenn Server dazukommen oder wegfallen und nichts bearbeitet wird
  useEffect(() => {
    if (!dirty) setEntries(initial());
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [signature, dirty]);

  function change(index: number, patch: Partial<Entry>) {
    setEntries((list) => list.map((entry, position) => (position === index ? { ...entry, ...patch } : entry)));
    setDirty(true);
  }

  function move(index: number, delta: number) {
    setEntries((list) => {
      const next = [...list];
      const target = index + delta;
      if (target < 0 || target >= next.length) return list;
      [next[index], next[target]] = [next[target], next[index]];
      return next;
    });
    setDirty(true);
  }

  async function save() {
    setBusy(true);
    try {
      const answer = await api.put<SyncResult>(`/projects/${project.id}/network`, {
        servers: entries.map(({ id, inTry, forcedHost, voicechat }) => ({ id, inTry, forcedHost, voicechat })),
      });
      setResult(answer);
      setDirty(false);
      refresh();
      toast.success("Netzwerk-Konfiguration gespeichert");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Fehler");
    } finally {
      setBusy(false);
    }
  }

  async function resync() {
    setBusy(true);
    await toast.run(async () => setResult(await api.post<SyncResult>(`/projects/${project.id}/network/sync`)), "Konfiguration neu geschrieben");
    setBusy(false);
  }

  const joinAddress = proxy?.address;

  return (
    <div className="space-y-5">
      <Card>
        <CardHeader title="Beitritts-Adresse" description="Spieler verbinden sich mit dem Proxy. Alles dahinter ist von außen nicht erreichbar." />
        <div className="flex items-center justify-between gap-4 px-5 py-4">
          {joinAddress ? (
            <>
              <code className="rounded-md bg-black/30 px-3 py-1.5 text-sm">{joinAddress}</code>
              <Button
                size="sm"
                onClick={() => {
                  navigator.clipboard.writeText(joinAddress).then(() => {
                    setCopied(true);
                    setTimeout(() => setCopied(false), 1500);
                  });
                }}
              >
                {copied ? <Check className="h-3.5 w-3.5 text-good" /> : <Copy className="h-3.5 w-3.5" />}
                Kopieren
              </Button>
            </>
          ) : (
            <p className="text-sm text-muted">{proxy ? "Der Node hat keine öffentliche Adresse eingetragen – die Adresse ist die des Nodes mit dem Port des Proxys." : "Es gibt noch keinen Proxy."}</p>
          )}
        </div>
      </Card>

      <VoicechatCard project={project} proxy={proxy} canManage={canManage} refresh={refresh} />

      <Card>
        <CardHeader
          title="Server hinter dem Proxy"
          description="Die Reihenfolge bestimmt, wohin Spieler beim Beitritt kommen: der erste erreichbare Server mit „Beitritt“ gewinnt."
          action={
            canManage && (
              <div className="flex gap-2">
                <Button size="sm" onClick={resync} disabled={busy || dirty || !proxy} title="Schreibt die Konfiguration erneut in alle Server">
                  Neu abgleichen
                </Button>
                <Button size="sm" variant="primary" onClick={save} loading={busy} disabled={!dirty || !proxy}>
                  Speichern & anwenden
                </Button>
              </div>
            )
          }
        />
        {entries.length === 0 ? (
          <EmptyState icon={<ServerIcon className="h-6 w-6" />} title="Noch keine Server" text="Füge Server hinzu (Paper, Purpur, Folia oder Fabric) – sie werden dem Proxy automatisch bekannt gemacht." />
        ) : (
          <div className="divide-y divide-white/[0.05]">
            {entries.map((entry, index) => (
              <div key={entry.id} className="flex flex-wrap items-center gap-x-5 gap-y-3 px-5 py-3.5">
                <div className="flex items-center gap-1">
                  <Button variant="ghost" size="icon-sm" disabled={!canManage || index === 0} onClick={() => move(index, -1)} aria-label="Nach oben">
                    <ArrowUp className="h-3.5 w-3.5" />
                  </Button>
                  <Button variant="ghost" size="icon-sm" disabled={!canManage || index === entries.length - 1} onClick={() => move(index, 1)} aria-label="Nach unten">
                    <ArrowDown className="h-3.5 w-3.5" />
                  </Button>
                </div>
                <div className="min-w-[140px] flex-1">
                  <p className="text-sm font-medium">{entry.name}</p>
                  <p className="text-xs text-muted">{entry.software}</p>
                </div>
                <label className="flex items-center gap-2 text-xs text-muted">
                  <Switch checked={entry.inTry} disabled={!canManage} onChange={(value) => change(index, { inTry: value })} />
                  Beitritt
                </label>
                <div className="w-full sm:w-64">
                  <input
                    className="field"
                    placeholder="Forced Host, z. B. survival.example.org"
                    value={entry.forcedHost}
                    disabled={!canManage}
                    onChange={(event) => change(index, { forcedHost: event.target.value })}
                  />
                </div>
                {project.voicechat ? (
                  <span className="text-xs text-muted">Voicechat über den Proxy</span>
                ) : (
                  <label className="flex items-center gap-2 text-xs text-muted" title="Simple Voice Chat mit eigenem UDP-Port installieren (Server muss aus sein)">
                    <Switch checked={entry.voicechat} disabled={!canManage} onChange={(value) => change(index, { voicechat: value })} />
                    Eigener Voicechat
                  </label>
                )}
              </div>
            ))}
          </div>
        )}
      </Card>

      {result && (
        <Card className="p-5 text-sm">
          <p className="font-medium">Ergebnis des Abgleichs</p>
          <ul className="mt-2 space-y-1 text-xs">
            {result.applied.length > 0 && <li className="text-good">Angewendet: {result.applied.join(", ")}</li>}
            {result.pending.length > 0 && (
              <li className="text-warn">
                Gespeichert, wird beim nächsten Start angewendet (die Server legen ihre Dateien erst beim ersten Start an): {result.pending.join(", ")}
              </li>
            )}
            {result.failed.map((failure) => (
              <li key={failure} className="text-bad">
                Fehlgeschlagen: {failure}
              </li>
            ))}
            {result.warnings?.map((warning) => (
              <li key={warning} className="text-muted">
                {warning}
              </li>
            ))}
          </ul>
          <p className="mt-3 text-xs text-muted">Server, deren Einstellungen sich geändert haben, brauchen einen Neustart, damit sie die Proxy-Anbindung lesen. Der Proxy lädt seine Liste selbst neu.</p>
        </Card>
      )}
    </div>
  );
}

// ---------------------------------------------------------------- Mitglieder

function MembersTab({ project, refresh }: { project: Project; refresh: () => void }) {
  const toast = useToast();
  const user = useUser();
  const canManage = roleRank[project.myRole] >= roleRank.admin;
  const [adding, setAdding] = useState(false);
  const [removing, setRemoving] = useState<Member | null>(null);
  const members = project.members ?? [];

  async function setRole(member: Member, role: Role) {
    await toast.run(() => api.put(`/projects/${project.id}/members/${member.userId}`, { role }), "Rolle geändert");
    refresh();
  }

  return (
    <Card>
      <CardHeader
        title="Mitglieder"
        description="Betrachter sehen Konsole und Spieler, Operatoren steuern Server und Dateien, Admins ändern Einstellungen, Eigentümer verwalten das Projekt."
        action={
          canManage && (
            <Button size="sm" onClick={() => setAdding(true)}>
              <UserPlus className="h-3.5 w-3.5" />
              Hinzufügen
            </Button>
          )
        }
      />
      <div className="divide-y divide-white/[0.05]">
        {members.map((member) => (
          <div key={member.userId} className="flex items-center gap-4 px-5 py-3">
            <PixelAvatar name={member.username} size={32} />
            <p className="min-w-0 flex-1 truncate text-sm">
              {member.username}
              {member.userId === user.id && <span className="ml-2 text-xs text-muted">(du)</span>}
            </p>
            <select
              className="field w-40"
              value={member.role}
              disabled={!canManage || roleRank[member.role] > roleRank[project.myRole]}
              onChange={(event) => setRole(member, event.target.value as Role)}
            >
              {(Object.keys(roleLabel) as Role[]).map((role) => (
                <option key={role} value={role} disabled={roleRank[role] > roleRank[project.myRole]}>
                  {roleLabel[role]}
                </option>
              ))}
            </select>
            {(canManage || member.userId === user.id) && (
              <Button variant="ghost" size="icon-sm" onClick={() => setRemoving(member)} aria-label="Entfernen" title={member.userId === user.id ? "Projekt verlassen" : "Entfernen"}>
                <Trash2 className="h-3.5 w-3.5" />
              </Button>
            )}
          </div>
        ))}
      </div>
      <AddMemberDialog project={project} open={adding} onClose={() => setAdding(false)} onDone={refresh} />
      <ConfirmDialog
        open={!!removing}
        onClose={() => setRemoving(null)}
        title={removing?.userId === user.id ? "Projekt verlassen?" : "Mitglied entfernen?"}
        text={removing ? `${removing.username} verliert den Zugriff auf dieses Projekt.` : ""}
        confirm="Entfernen"
        danger
        onConfirm={async () => {
          if (!removing) return;
          const leaving = removing.userId === user.id;
          const ok = await toast.run(() => api.delete(`/projects/${project.id}/members/${removing.userId}`), "Entfernt");
          if (ok && leaving) window.location.replace("/");
          refresh();
        }}
      />
    </Card>
  );
}

function AddMemberDialog({ project, open, onClose, onDone }: { project: Project; open: boolean; onClose: () => void; onDone: () => void }) {
  const toast = useToast();
  const { data: users } = useApi<{ id: string; username: string }[]>(open ? "/users" : null);
  const [userId, setUserId] = useState("");
  const [role, setRole] = useState<Role>("viewer");
  const present = new Set((project.members ?? []).map((member) => member.userId));
  const choices = (users ?? []).filter((candidate) => !present.has(candidate.id));

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title="Mitglied hinzufügen"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Abbrechen
          </Button>
          <Button
            variant="primary"
            disabled={!userId}
            onClick={async () => {
              if (await toast.run(() => api.put(`/projects/${project.id}/members/${userId}`, { role }), "Hinzugefügt")) {
                setUserId("");
                onClose();
                onDone();
              }
            }}
          >
            Hinzufügen
          </Button>
        </>
      }
    >
      <div>
        <label className="label">Benutzer</label>
        <select className="field" value={userId} onChange={(event) => setUserId(event.target.value)}>
          <option value="">Auswählen …</option>
          {choices.map((candidate) => (
            <option key={candidate.id} value={candidate.id}>
              {candidate.username}
            </option>
          ))}
        </select>
        {users && choices.length === 0 && <p className="mt-1.5 text-xs text-muted">Alle Benutzer sind schon Mitglied. Neue Benutzer legt ein Administrator unter „Benutzer“ an.</p>}
      </div>
      <div>
        <label className="label">Rolle</label>
        <select className="field" value={role} onChange={(event) => setRole(event.target.value as Role)}>
          {(Object.keys(roleLabel) as Role[]).map((value) => (
            <option key={value} value={value} disabled={roleRank[value] > roleRank[project.myRole]}>
              {roleLabel[value]}
            </option>
          ))}
        </select>
      </div>
    </Dialog>
  );
}

// ---------------------------------------------------------------- Einstellungen

function SettingsTab({ project, refresh }: { project: Project; refresh: () => void }) {
  const toast = useToast();
  const router = useRouter();
  const [name, setName] = useState(project.name);
  const [description, setDescription] = useState(project.description);
  const [deleting, setDeleting] = useState(false);
  const isOwner = project.myRole === "owner";

  return (
    <div className="space-y-5">
      <Card>
        <CardHeader title="Allgemein" />
        <div className="space-y-4 px-5 py-4">
          <div>
            <label className="label">Name</label>
            <input className="field max-w-md" value={name} onChange={(event) => setName(event.target.value)} maxLength={48} />
          </div>
          <div>
            <label className="label">Beschreibung</label>
            <input className="field max-w-xl" value={description} onChange={(event) => setDescription(event.target.value)} maxLength={500} />
          </div>
          <Button
            variant="primary"
            disabled={!name.trim() || (name === project.name && description === project.description)}
            onClick={async () => {
              if (await toast.run(() => api.patch(`/projects/${project.id}`, { name, description }), "Gespeichert")) refresh();
            }}
          >
            Speichern
          </Button>
        </div>
      </Card>

      {isOwner && (
        <Card className="border-bad/25">
          <CardHeader title="Projekt löschen" description="Löscht alle Server dieses Projekts samt Dateien und Backups. Das lässt sich nicht rückgängig machen." />
          <div className="px-5 py-4">
            <Button variant="danger" onClick={() => setDeleting(true)}>
              <Trash2 className="h-4 w-4" />
              Projekt löschen
            </Button>
          </div>
        </Card>
      )}
      <ConfirmDialog
        open={deleting}
        onClose={() => setDeleting(false)}
        title={`„${project.name}“ löschen?`}
        text="Alle Server, Welten, Plugins und Backups dieses Projekts werden unwiderruflich gelöscht."
        confirm="Endgültig löschen"
        danger
        onConfirm={async () => {
          if (await toast.run(() => api.delete(`/projects/${project.id}`), "Projekt gelöscht")) router.replace("/");
        }}
      />
    </div>
  );
}


// ---------------------------------------------------------------- Spieler im Netzwerk

function ProjectStats({ project }: { project: Project }) {
  const [range, setRange] = useState<Range>("24h");
  const { data } = useApi<Metrics>(`/projects/${project.id}/metrics?range=${range}`, { refreshInterval: 30000 });
  if (!data) return null;
  const series = data.series;
  return (
    <ChartCard
      title={project.kind === "network" ? "Spieler im Netzwerk" : "Spieler"}
      description={series.samples ? `Jetzt ${data.players} · Spitze ${series.peakPlayers} · Ø ${series.avgPlayers.toFixed(1).replace(".", ",")}` : undefined}
      empty={series.samples === 0}
      action={<RangeTabs value={range} onChange={setRange} />}
    >
      <SeriesChart times={series.times} series={[{ name: "Spieler", color: "#f59e0b", values: series.players }]} height={170} long={range === "24h" || range === "7d"} format={(value) => String(Math.round(value))} />
    </ChartCard>
  );
}

// ---------------------------------------------------------------- Voicechat über den Proxy

function VoicechatCard({ project, proxy, canManage, refresh }: { project: Project; proxy?: Server; canManage: boolean; refresh: () => void }) {
  const toast = useToast();
  const [busy, setBusy] = useState(false);
  const canEnable = !!proxy;
  const stopped = proxy?.view?.state === "offline";

  async function toggle(enabled: boolean) {
    setBusy(true);
    try {
      const result = await api.put<SyncResult>(`/projects/${project.id}/voicechat`, { enabled });
      toast.success(enabled ? "Voicechat über den Proxy ist eingerichtet – Plugins werden installiert" : "Voicechat über den Proxy ist aus");
      if (result.failed.length) toast.error(`Fehlgeschlagen: ${result.failed.join(", ")}`);
      refresh();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Fehler");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card>
      <CardHeader
        title="Simple Voice Chat"
        description="Der Proxy nimmt den Voicechat aller Server über einen einzigen UDP-Port an und reicht ihn weiter. Spieler brauchen nur diesen Port."
        action={<Switch checked={project.voicechat} disabled={!canManage || busy || !canEnable} onChange={toggle} />}
      />
      <div className="space-y-2 px-5 py-4 text-xs text-muted">
        {project.voicechat ? (
          <>
            <p>
              Aktiv: Das Proxy-Plugin liegt auf dem Proxy, das normale Plugin auf allen Servern – neue Server bekommen es automatisch. Öffne in der Firewall den{" "}
              <b className="text-ink">UDP-Port {proxy?.voiceAddress?.split(":")[1] ?? "des Proxys"}</b>.
            </p>
            {proxy?.voiceAddress && (
              <p>
                Voicechat-Adresse: <code className="rounded bg-black/30 px-2 py-0.5 text-ink">{proxy.voiceAddress}</code>
              </p>
            )}
            <p>Ändere Server oder Proxy-Einstellungen, braucht der betroffene Server einen Neustart, damit er das Plugin lädt.</p>
          </>
        ) : !canEnable ? (
          <p>Lege zuerst den Proxy an.</p>
        ) : (
          <p>
            Beim Einschalten bekommt der Proxy zusätzlich einen UDP-Port{!stopped && " – dafür muss er kurz ausgeschaltet sein"}. Alternativ kannst du Voicechat pro Server mit
            eigenem UDP-Port einschalten (siehe unten).
          </p>
        )}
      </div>
    </Card>
  );
}
