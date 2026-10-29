"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { Cpu, FolderPlus, Network, Play, Plus, Server as ServerIcon, Users } from "lucide-react";
import { useState } from "react";
import { Badge, Button, Card, EmptyState, PageHeader, StatCard } from "@/components/ui/primitives";
import { Dialog, useToast } from "@/components/ui/feedback";
import { ErrorState, Loading } from "@/components/ui/async";
import { StateBadge } from "@/components/status";
import { api, useApi } from "@/lib/api";
import { roleLabel, type NodeInfo, type Project } from "@/lib/types";

export default function OverviewPage() {
  const router = useRouter();
  const { data: projects, error, mutate } = useApi<Project[]>("/projects", { refreshInterval: 5000 });
  const { data: nodes } = useApi<NodeInfo[]>("/nodes", { refreshInterval: 15000 });
  const [creating, setCreating] = useState(false);

  if (error) return <ErrorState error={error} retry={() => mutate()} />;
  if (!projects) return <Loading />;

  const servers = projects.flatMap((project) => project.servers);
  const running = servers.filter((server) => server.view?.state === "running").length;
  const online = nodes?.filter((node) => node.online).length ?? 0;

  return (
    <>
      <PageHeader
        title="Übersicht"
        description="Deine Projekte – einzelne Server und Netzwerke."
        action={
          <Button variant="primary" onClick={() => setCreating(true)}>
            <Plus className="h-4 w-4" />
            Neues Projekt
          </Button>
        }
      />

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard label="Projekte" value={projects.length} icon={<FolderPlus className="h-4 w-4" />} tone="violet" hint={`${projects.filter((p) => p.kind === "network").length} Netzwerke`} />
        <StatCard label="Server" value={servers.length} icon={<ServerIcon className="h-4 w-4" />} tone="blue" hint="in allen Projekten" />
        <StatCard label="Laufen gerade" value={running} icon={<Play className="h-4 w-4" />} tone={running > 0 ? "green" : "neutral"} hint={`von ${servers.length}`} />
        <StatCard
          label="Nodes online"
          value={nodes ? `${online}/${nodes.length}` : "–"}
          icon={<Cpu className="h-4 w-4" />}
          tone={nodes && online < nodes.length ? "red" : "amber"}
          hint={nodes?.length === 0 ? "Noch kein Node eingerichtet" : "Maschinen mit Docker"}
        />
      </div>

      <div className="mt-8">
        {projects.length === 0 ? (
          <Card>
            <EmptyState
              icon={<FolderPlus className="h-6 w-6" />}
              title="Noch kein Projekt"
              text={nodes && nodes.length === 0 ? "Richte zuerst unter „Nodes“ eine Maschine ein, dann lege hier ein Projekt an." : "Lege dein erstes Projekt an: ein einzelner Server oder ein Netzwerk mit Velocity."}
            />
            <div className="flex justify-center pb-10">
              <Button variant="primary" onClick={() => setCreating(true)}>
                <Plus className="h-4 w-4" />
                Projekt anlegen
              </Button>
            </div>
          </Card>
        ) : (
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            {projects.map((project) => {
              const live = project.servers.filter((server) => server.view?.state === "running").length;
              const first = project.servers[0];
              return (
                <Link key={project.id} href={`/project/?id=${project.id}`} className="group">
                  <Card className="h-full p-5 transition duration-100 group-hover:border-white/[0.16] group-active:scale-[0.99]">
                    <div className="flex items-start justify-between gap-3">
                      <div className="flex min-w-0 items-center gap-3">
                        <span className="grid h-9 w-9 shrink-0 place-items-center rounded-lg border border-white/[0.08] bg-white/[0.04] text-muted">
                          {project.kind === "network" ? <Network className="h-4 w-4" /> : <ServerIcon className="h-4 w-4" />}
                        </span>
                        <div className="min-w-0">
                          <p className="truncate text-sm font-semibold">{project.name}</p>
                          <p className="text-xs text-muted">{project.kind === "network" ? "Netzwerk" : "Einzelner Server"}</p>
                        </div>
                      </div>
                      {project.kind === "server" && first ? <StateBadge server={first} /> : <Badge tone={live > 0 ? "good" : "neutral"}>{live}/{project.servers.length} laufen</Badge>}
                    </div>
                    {project.description && <p className="mt-3 line-clamp-2 text-xs text-muted">{project.description}</p>}
                    <div className="mt-4 flex items-center gap-3 text-xs text-muted">
                      <span className="inline-flex items-center gap-1">
                        <Users className="h-3.5 w-3.5" />
                        {roleLabel[project.myRole]}
                      </span>
                      {first?.address && <span className="truncate font-mono">{first.address}</span>}
                    </div>
                  </Card>
                </Link>
              );
            })}
          </div>
        )}
      </div>

      <NewProjectDialog
        open={creating}
        onClose={() => setCreating(false)}
        onCreated={(project) => {
          mutate();
          router.push(`/project/?id=${project.id}`);
        }}
      />
    </>
  );
}

function NewProjectDialog({ open, onClose, onCreated }: { open: boolean; onClose: () => void; onCreated: (project: Project) => void }) {
  const toast = useToast();
  const [name, setName] = useState("");
  const [kind, setKind] = useState<"server" | "network">("server");
  const [description, setDescription] = useState("");
  const [busy, setBusy] = useState(false);

  async function create() {
    setBusy(true);
    try {
      const project = await api.post<Project>("/projects", { name, kind, description });
      setName("");
      setDescription("");
      onClose();
      onCreated(project);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Fehler");
    } finally {
      setBusy(false);
    }
  }

  const options = [
    { id: "server" as const, icon: ServerIcon, title: "Einzelner Server", text: "Ein Server: Paper, Purpur, Folia, Fabric, Vanilla oder Velocity." },
    { id: "network" as const, icon: Network, title: "Netzwerk", text: "Velocity-Proxy mit mehreren Servern dahinter. Wird automatisch verbunden." },
  ];

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title="Neues Projekt"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Abbrechen
          </Button>
          <Button variant="primary" loading={busy} disabled={!name.trim()} onClick={create}>
            Anlegen
          </Button>
        </>
      }
    >
      <div className="grid gap-3 sm:grid-cols-2">
        {options.map((option) => (
          <button
            key={option.id}
            type="button"
            onClick={() => setKind(option.id)}
            className={`rounded-lg border p-3 text-left transition active:scale-[0.98] ${kind === option.id ? "border-accent/60 bg-accent/10" : "border-white/[0.08] bg-white/[0.02] hover:border-white/[0.16]"}`}
          >
            <option.icon className="mb-2 h-4 w-4 text-muted" />
            <p className="text-sm font-medium">{option.title}</p>
            <p className="mt-1 text-xs text-muted">{option.text}</p>
          </button>
        ))}
      </div>
      <div>
        <label className="label">Name</label>
        <input className="field" value={name} onChange={(event) => setName(event.target.value)} placeholder={kind === "network" ? "Mein Netzwerk" : "Survival"} maxLength={48} autoFocus />
      </div>
      <div>
        <label className="label">Beschreibung (optional)</label>
        <input className="field" value={description} onChange={(event) => setDescription(event.target.value)} maxLength={500} />
      </div>
    </Dialog>
  );
}
