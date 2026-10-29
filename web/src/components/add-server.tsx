"use client";

import { useEffect, useMemo, useState } from "react";
import { Package } from "lucide-react";
import { Button, Checkbox, SearchInput, Switch, Tabs } from "@/components/ui/primitives";
import { Dialog, useToast } from "@/components/ui/feedback";
import { api, useApi } from "@/lib/api";
import { compact, megabytes } from "@/lib/format";
import type { ModpackVersion, ModrinthHit, NodeInfo, Project, Server, SoftwareInfo } from "@/lib/types";

const memoryOptions = [256, 512, 1024, 2048, 3072, 4096, 6144, 8192, 12288, 16384];

export function AddServerDialog({ project, open, onClose, onCreated }: { project: Project; open: boolean; onClose: () => void; onCreated: (server: Server) => void }) {
  const toast = useToast();
  const { data: software } = useApi<SoftwareInfo[]>(open ? "/software" : null);
  const { data: nodes } = useApi<NodeInfo[]>(open ? "/nodes" : null);

  const hasProxy = project.servers.some((server) => server.role === "proxy");
  const fixedNode = project.kind === "network" ? project.servers[0]?.nodeId : undefined;

  const options = useMemo(() => {
    const all = software ?? [];
    if (project.kind === "server") return all;
    return all.filter((item) => (item.id === "velocity" ? !hasProxy : ["paper", "purpur", "folia", "fabric"].includes(item.id)));
  }, [software, project.kind, hasProxy]);

  const [name, setName] = useState("");
  const [softwareId, setSoftwareId] = useState("");
  const [version, setVersion] = useState("");
  const [memory, setMemory] = useState(2048);
  const [nodeId, setNodeId] = useState("");
  const [eula, setEula] = useState(false);
  const [voicechat, setVoicechat] = useState(false);
  const [busy, setBusy] = useState(false);
  const [mode, setMode] = useState<"software" | "modpack">("software");
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
    if (softwareId) setMemory(softwareId === "velocity" ? 512 : 2048);
    setVoicechat(false);
  }, [softwareId]);

  const info = options.find((item) => item.id === softwareId);
  const needsEula = info?.kind === "server";
  const canVoice = project.kind === "network" && ["paper", "purpur", "folia", "fabric"].includes(softwareId);

  async function create() {
    setBusy(true);
    try {
      const body =
        mode === "modpack" && pack && chosenPackVersion
          ? { name, nodeId, memoryMB: Math.max(memory, 4096), eula, modpack: { project: pack.project_id, versionId: chosenPackVersion.id } }
          : { name, nodeId, software: softwareId, version: effectiveVersion, memoryMB: memory, eula: needsEula ? eula : true, voicechat };
      const server = await api.post<Server>(`/projects/${project.id}/servers`, body);
      toast.success(mode === "modpack" ? "Server wird angelegt, das Modpack wird danach installiert" : "Server wird installiert");
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

  const ready = name.trim() && nodeId && (mode === "modpack" ? pack && chosenPackVersion && eula : info && effectiveVersion && (!needsEula || eula));

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
      {canModpack && project.kind === "server" && (
        <Tabs
          value={mode}
          onChange={(next) => {
            setMode(next);
            if (next === "modpack") setMemory((current) => Math.max(current, 4096));
          }}
          items={[
            { id: "software", label: "Software" },
            { id: "modpack", label: "Modrinth-Modpack" },
          ]}
        />
      )}
      <div>
        <label className="label">Name</label>
        <input className="field" value={name} onChange={(event) => setName(event.target.value)} placeholder={softwareId === "velocity" ? "Proxy" : "Lobby"} maxLength={48} autoFocus />
      </div>
      {mode === "modpack" && (
        <div className="space-y-2">
          <SearchInput value={packQuery} onChange={(event) => setPackQuery(event.target.value)} placeholder="Fabric-Modpack auf Modrinth suchen" />
          {pack ? (
            <div className="flex items-center gap-3 rounded-lg border border-accent/40 bg-accent/5 p-2.5">
              {/* eslint-disable-next-line @next/next/no-img-element */}
              {pack.icon_url ? <img src={pack.icon_url} alt="" className="h-10 w-10 rounded-lg object-cover" /> : <Package className="h-6 w-6 text-muted" />}
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">{pack.title}</p>
                <select className="field mt-1 py-1 text-xs" value={chosenPackVersion?.id ?? ""} onChange={(event) => setPackVersion(event.target.value)} disabled={!packInfo}>
                  {(packInfo?.versions ?? []).slice(0, 50).map((entry) => (
                    <option key={entry.id} value={entry.id}>
                      {entry.versionNumber} · MC {entry.gameVersions[0]}
                    </option>
                  ))}
                </select>
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
        {mode === "software" && (
          <>
            <div>
              <label className="label">Software</label>
              <select className="field" value={softwareId} onChange={(event) => setSoftwareId(event.target.value)}>
                {options.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.name}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label className="label">Version</label>
              <select className="field" value={effectiveVersion} onChange={(event) => setVersion(event.target.value)} disabled={!versions}>
                {(versions ?? []).slice(0, 80).map((entry) => (
                  <option key={entry} value={entry}>
                    {entry}
                  </option>
                ))}
              </select>
            </div>
          </>
        )}
        <div>
          <label className="label">Arbeitsspeicher</label>
          <select className="field" value={memory} onChange={(event) => setMemory(Number(event.target.value))}>
            {memoryOptions.map((value) => (
              <option key={value} value={value}>
                {megabytes(value)}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className="label">Node</label>
          <select className="field" value={nodeId} onChange={(event) => setNodeId(event.target.value)} disabled={!!fixedNode}>
            {(nodes ?? []).map((node) => (
              <option key={node.id} value={node.id}>
                {node.name}
                {node.online ? "" : " (offline)"}
              </option>
            ))}
          </select>
        </div>
      </div>
      {fixedNode && <p className="text-xs text-muted">Ein Netzwerk liegt komplett auf einem Node, damit sich die Server im Docker-Netz finden.</p>}
      {canVoice && mode === "software" && (
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
