"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { FileArchive, Package } from "lucide-react";
import { Button, Checkbox, SearchInput, Select, Switch, Tabs } from "@/components/ui/primitives";
import { Dialog, useToast } from "@/components/ui/feedback";
import { MemoryInput } from "@/components/memory-input";
import { api, upload, useApi } from "@/lib/api";
import { compact, megabytes } from "@/lib/format";
import type { ModpackVersion, ModrinthHit, NodeInfo, Project, Server, SoftwareInfo } from "@/lib/types";

/** Vorlagen setzen Name, Software und Speicher, damit man für die üblichen Fälle nichts nachdenken muss. */
const templates = [
  { id: "survival", label: "Survival", software: "paper", memory: 4096, name: "Survival" },
  { id: "lobby", label: "Lobby", software: "paper", memory: 1024, name: "Lobby" },
  { id: "creative", label: "Creative / Bauen", software: "purpur", memory: 3072, name: "Creative" },
  { id: "mods", label: "Mods", software: "fabric", memory: 4096, name: "Mods" },
];

export function AddServerDialog({ project, open, onClose, onCreated }: { project: Project; open: boolean; onClose: () => void; onCreated: (server: Server) => void }) {
  const toast = useToast();
  const { data: software } = useApi<SoftwareInfo[]>(open ? "/software" : null);
  const { data: nodes } = useApi<NodeInfo[]>(open ? "/nodes" : null);

  const hasProxy = project.servers.some((server) => server.role === "proxy");
  const fixedNode = project.kind === "network" ? project.servers[0]?.nodeId : undefined;

  const options = useMemo(() => {
    const all = software ?? [];
    if (project.kind === "server") return all;
    return all.filter((item) => (item.id === "velocity" ? !hasProxy : (item.canBeBackend ?? ["paper", "purpur", "folia", "leaf", "fabric", "neoforge"].includes(item.id))));
  }, [software, project.kind, hasProxy]);

  const [name, setName] = useState("");
  const [softwareId, setSoftwareId] = useState("");
  const [version, setVersion] = useState("");
  const [memory, setMemory] = useState(2048);
  const [nodeId, setNodeId] = useState("");
  const [eula, setEula] = useState(false);
  const [voicechat, setVoicechat] = useState(false);
  const [busy, setBusy] = useState(false);
  const [mode, setMode] = useState<"software" | "modpack" | "import">("software");
  const [zip, setZip] = useState<File | null>(null);
  const templateMemory = useRef<number | null>(null);
  const [packQuery, setPackQuery] = useState("");
  const [packSearch, setPackSearch] = useState("");
  const [pack, setPack] = useState<ModrinthHit | null>(null);
  const [packVersion, setPackVersion] = useState("");

  useEffect(() => {
    const timer = setTimeout(() => setPackSearch(packQuery.trim()), 350);
    return () => clearTimeout(timer);
  }, [packQuery]);
  const { data: packs, error: packError, isLoading: packsLoading } = useApi<{ hits: ModrinthHit[] }>(open && mode === "modpack" ? `/modpacks/search?q=${encodeURIComponent(packSearch)}` : null);
  const { data: packInfo } = useApi<{ versions: ModpackVersion[] }>(pack ? `/modpacks/${encodeURIComponent(pack.project_id)}` : null);
  const chosenPackVersion = packInfo?.versions.find((entry) => entry.id === packVersion) ?? packInfo?.versions[0];
  const canModpack = options.some((item) => item.id === "fabric");

  // Die gewählte Software muss immer eine der angebotenen sein. Nach dem Anlegen des Proxys fällt Velocity weg,
  // dann würde sonst die alte Wahl unsichtbar weiter gelten.
  useEffect(() => {
    if (!open || options.length === 0 || options.some((item) => item.id === softwareId)) return;
    const first = project.kind === "network" && !hasProxy ? options.find((item) => item.id === "velocity") : options.find((item) => item.id === "paper");
    setSoftwareId((first ?? options[0]).id);
  }, [open, options, softwareId, hasProxy, project.kind]);

  useEffect(() => {
    if (fixedNode) setNodeId(fixedNode);
    else if (!nodeId && nodes && nodes.length > 0) setNodeId((nodes.find((node) => node.online) ?? nodes[0]).id);
  }, [nodes, fixedNode, nodeId]);

  const { data: versions } = useApi<string[]>(open && softwareId ? `/software/${softwareId}/versions` : null);
  // Die Version gilt nur, wenn sie zur gewählten Software gehört (die Liste ist immer die der aktuellen Software)
  const effectiveVersion = versions && versions.includes(version) ? version : (versions?.[0] ?? "");
  useEffect(() => {
    if (softwareId) setMemory(templateMemory.current ?? (softwareId === "velocity" ? 512 : 2048));
    templateMemory.current = null;
    setVoicechat(false);
  }, [softwareId]);

  const info = options.find((item) => item.id === softwareId);
  const needsEula = info?.kind === "server";
  const canVoice = project.kind === "network" && (info?.canVoice ?? ["paper", "purpur", "folia", "leaf", "fabric", "neoforge"].includes(softwareId));

  function applyTemplate(template: (typeof templates)[number]) {
    if (!options.some((item) => item.id === template.software)) return;
    templateMemory.current = template.memory;
    setName((current) => current.trim() || template.name);
    if (softwareId === template.software) {
      setMemory(template.memory);
      templateMemory.current = null;
    } else {
      setSoftwareId(template.software);
    }
  }

  async function create() {
    setBusy(true);
    try {
      const body =
        mode === "modpack" && pack && chosenPackVersion
          ? { name, nodeId, memoryMB: Math.max(memory, 4096), eula, modpack: { project: pack.project_id, versionId: chosenPackVersion.id } }
          : { name, nodeId, software: softwareId, version: effectiveVersion, memoryMB: memory, eula: needsEula ? eula : true, voicechat };
      const server = await api.post<Server>(`/projects/${project.id}/servers`, body);
      if (mode === "import" && zip) {
        try {
          // Das Archiv wird in den Serverordner gelegt und dort entpackt; vorhandene Dateien (etwa die Server-Datei) bleiben
          await upload(`/servers/${server.id}/files/upload?path=&overwrite=1`, [zip]);
          await api.post(`/servers/${server.id}/files/extract`, { path: zip.name, dest: "" });
          await api.delete(`/servers/${server.id}/files?path=${encodeURIComponent(zip.name)}`).catch(() => null);
        } catch (error) {
          toast.error(`Der Server ist angelegt, der Import ist aber fehlgeschlagen: ${error instanceof Error ? error.message : "Fehler"}`);
        }
      }
      toast.success(mode === "modpack" ? "Server wird angelegt, das Modpack wird danach installiert" : mode === "import" ? "Server wird angelegt, das Archiv ist entpackt" : "Server wird installiert");
      setZip(null);
      setName("");
      setEula(false);
      onClose();
      onCreated(server);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Fehler");
    } finally {
      setBusy(false);
    }
  }

  const ready = name.trim() && nodeId && (mode === "modpack" ? pack && chosenPackVersion && eula : info && effectiveVersion && (!needsEula || eula) && (mode !== "import" || zip));

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={project.kind === "network" && !hasProxy ? "Proxy hinzufügen" : "Server hinzufügen"}
      description={project.kind === "network" ? "Server in einem Netzwerk werden automatisch mit dem Proxy verbunden." : undefined}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Abbrechen
          </Button>
          <Button variant="primary" loading={busy} disabled={!ready} onClick={create}>
            Anlegen
          </Button>
        </>
      }
    >
      {project.kind === "server" && (
        <Tabs
          value={mode}
          onChange={(next) => {
            setMode(next);
            if (next === "modpack") setMemory((current) => Math.max(current, 4096));
          }}
          items={[
            { id: "software", label: "Software" },
            ...(canModpack ? [{ id: "modpack" as const, label: "Modrinth-Modpack" }] : []),
            { id: "import" as const, label: "ZIP importieren" },
          ]}
        />
      )}
      {mode === "software" && (
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="text-xs text-muted">Vorlage:</span>
          {templates
            .filter((template) => options.some((item) => item.id === template.software))
            .map((template) => (
              <button key={template.id} type="button" onClick={() => applyTemplate(template)} className="rounded-full border border-white/[0.1] px-2.5 py-1 text-xs text-muted transition hover:bg-white/[0.06] hover:text-ink">
                {template.label}
              </button>
            ))}
        </div>
      )}
      <div>
        <label className="label">Name</label>
        <input className="field" value={name} onChange={(event) => setName(event.target.value)} placeholder={softwareId === "velocity" ? "Proxy" : "Lobby"} maxLength={48} autoFocus />
      </div>
      {mode === "import" && (
        <div className="space-y-2">
          <label className="flex cursor-pointer items-center gap-3 rounded-lg border border-dashed border-white/[0.15] px-3 py-3 text-sm hover:bg-white/[0.03]">
            <FileArchive className="h-5 w-5 shrink-0 text-muted" />
            <span className="min-w-0 flex-1 truncate">{zip ? `${zip.name} (${megabytes(Math.max(1, Math.round(zip.size / 1024 ** 2)))})` : "ZIP-Datei auswählen (.zip, .tar.gz)"}</span>
            <input type="file" accept=".zip,.tar,.tar.gz,.tgz" className="hidden" onChange={(event) => setZip(event.target.files?.[0] ?? null)} />
          </label>
          <p className="text-xs text-muted">Der Inhalt landet im Serverordner: Welt, Plugins, Konfiguration. Vorhandene Dateien werden nicht überschrieben, lege deshalb keine eigene Server-Datei bei. Die Software und Version wählst du unten.</p>
        </div>
      )}
      {mode === "modpack" && (
        <div className="space-y-2">
          <SearchInput value={packQuery} onChange={(event) => setPackQuery(event.target.value)} placeholder="Fabric-Modpack auf Modrinth suchen" />
          {pack ? (
            <div className="flex items-center gap-3 rounded-lg border border-accent/40 bg-accent/5 p-2.5">
              {/* eslint-disable-next-line @next/next/no-img-element */}
              {pack.icon_url ? <img src={pack.icon_url} alt="" className="h-10 w-10 rounded-lg object-cover" /> : <Package className="h-6 w-6 text-muted" />}
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">{pack.title}</p>
                <Select
                  size="sm"
                  className="mt-1"
                  value={chosenPackVersion?.id ?? ""}
                  onChange={setPackVersion}
                  disabled={!packInfo}
                  aria-label="Modpack-Version"
                  options={(packInfo?.versions ?? []).slice(0, 50).map((entry) => ({ value: entry.id, label: `${entry.versionNumber} · MC ${entry.gameVersions[0]}` }))}
                />
              </div>
              <Button size="sm" variant="ghost" onClick={() => { setPack(null); setPackVersion(""); }}>
                Ändern
              </Button>
            </div>
          ) : packError ? (
            <p className="text-xs text-bad">{packError.message}</p>
          ) : (
            <div className="max-h-52 divide-y divide-white/[0.05] overflow-y-auto rounded-lg border border-white/[0.07]">
              {packsLoading && !packs && <p className="p-3 text-xs text-muted">Suche läuft …</p>}
              {packs?.hits.map((hit) => (
                <button key={hit.project_id} className="flex w-full items-center gap-3 px-3 py-2 text-left hover:bg-white/[0.04]" onClick={() => setPack(hit)}>
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  {hit.icon_url ? <img src={hit.icon_url} alt="" className="h-8 w-8 rounded-md object-cover" /> : <Package className="h-5 w-5 text-muted" />}
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm">{hit.title}</span>
                    <span className="block truncate text-xs text-muted">
                      {compact(hit.downloads)} Downloads · {hit.description}
                    </span>
                  </span>
                </button>
              ))}
              {packs && packs.hits.length === 0 && <p className="p-3 text-xs text-muted">Nichts gefunden.</p>}
            </div>
          )}
          <p className="text-xs text-muted">Minecraft-Version und Fabric ergeben sich aus dem Modpack. Mods werden nach dem Anlegen automatisch heruntergeladen (Fortschritt im Server). Mindestens 4 GB RAM.</p>
        </div>
      )}
      <div className="grid gap-3 sm:grid-cols-2">
        {mode !== "modpack" && (
          <>
            <div>
              <label className="label">Software</label>
              <Select value={softwareId} onChange={setSoftwareId} options={options.map((item) => ({ value: item.id, label: item.name }))} />
            </div>
            <div>
              <label className="label">Version</label>
              <Select
                value={effectiveVersion}
                onChange={setVersion}
                disabled={!versions}
                options={(versions ?? []).slice(0, 80).map((entry) => ({ value: entry, label: entry }))}
              />
            </div>
          </>
        )}
        <div>
          <label className="label">Arbeitsspeicher</label>
          <MemoryInput value={memory} onChange={setMemory} min={mode === "modpack" ? 4096 : 256} />
        </div>
        <div>
          <label className="label">Node</label>
          <Select
            value={nodeId}
            onChange={setNodeId}
            disabled={!!fixedNode}
            options={(nodes ?? []).map((node) => ({ value: node.id, label: node.online ? node.name : `${node.name} (offline)` }))}
          />
        </div>
      </div>
      {project.kind === "network" && <p className="text-xs text-muted">Fabric und NeoForge bekommen die Proxy-Anbindung über einen Mod, den Lodestone mitinstalliert. Forge gibt es nur für einzelne Server.</p>}
      {fixedNode && <p className="text-xs text-muted">Ein Netzwerk liegt komplett auf einem Node, damit sich die Server im Docker-Netz finden.</p>}
      {canVoice && mode !== "modpack" && (
        <div className="flex items-center justify-between rounded-lg border border-white/[0.07] bg-white/[0.02] px-3 py-2.5">
          <div>
            <p className="text-sm font-medium">Simple Voice Chat</p>
            <p className="text-xs text-muted">Installiert das Plugin bzw. den Mod und richtet den UDP-Port ein.</p>
          </div>
          <Switch checked={voicechat} onChange={setVoicechat} />
        </div>
      )}
      {(needsEula || mode === "modpack") && (
        <label className="flex cursor-pointer items-start gap-2.5 text-xs text-muted">
          <Checkbox checked={eula} onChange={setEula} label="EULA" />
          <span>
            Ich akzeptiere die{" "}
            <a className="text-accent hover:underline" href="https://aka.ms/MinecraftEULA" target="_blank" rel="noreferrer">
              Minecraft-EULA
            </a>
            .
          </span>
        </label>
      )}
    </Dialog>
  );
}
