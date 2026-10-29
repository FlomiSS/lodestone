"use client";

import { ArrowUpCircle, CheckCircle2, Download, ExternalLink, Package, Plug, Power, RefreshCw, Repeat, Search, Trash2, Upload } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Badge, Button, Card, CardHeader, EmptyState, SearchInput, Tabs } from "@/components/ui/primitives";
import { ConfirmDialog, Dialog, useToast } from "@/components/ui/feedback";
import { ErrorState, Loading } from "@/components/ui/async";
import { api, upload, useApi } from "@/lib/api";
import { bytes, compact } from "@/lib/format";
import type { Addon, AddonDetail, ModrinthHit, UpdateEntry } from "@/lib/types";

type View = "installed" | "browse";

export function AddonsTab({ serverId, canWrite, noun, running }: { serverId: string; canWrite: boolean; noun: "Plugins" | "Mods"; running: boolean }) {
  const [view, setView] = useState<View>("installed");
  return (
    <div className="space-y-4">
      <Tabs
        value={view}
        onChange={setView}
        items={[
          { id: "installed", label: `Installierte ${noun}` },
          ...(canWrite ? [{ id: "browse" as const, label: "Modrinth durchsuchen" }] : []),
        ]}
      />
      {view === "installed" ? <Installed serverId={serverId} canWrite={canWrite} noun={noun} running={running} /> : <Browse serverId={serverId} noun={noun} onInstalled={() => setView("installed")} />}
    </div>
  );
}

function Installed({ serverId, canWrite, noun, running }: { serverId: string; canWrite: boolean; noun: string; running: boolean }) {
  const toast = useToast();
  const { data, error, mutate } = useApi<Addon[]>(`/servers/${serverId}/addons`);
  const [updates, setUpdates] = useState<Record<string, UpdateEntry> | null>(null);
  const [checking, setChecking] = useState(false);
  const [remove, setRemove] = useState<Addon | null>(null);
  const [switching, setSwitching] = useState<AddonDetail | null>(null);
  const [progress, setProgress] = useState<number | null>(null);
  const picker = useRef<HTMLInputElement>(null);
  const { data: detailList, mutate: reloadDetails } = useApi<AddonDetail[]>(`/servers/${serverId}/addons/details`);
  const details = new Map((detailList ?? []).map((entry) => [`${entry.dir}/${entry.file}`, entry]));

  if (error) return <ErrorState error={error} retry={() => mutate()} />;
  if (!data) return <Loading />;

  const path = (addon: Addon) => `${addon.dir}/${addon.file}`;
  const pending = updates ? Object.values(updates).filter((entry) => entry.updateAvailable) : [];

  async function check() {
    setChecking(true);
    try {
      const list = await api.post<UpdateEntry[]>(`/servers/${serverId}/addons/check`);
      setUpdates(Object.fromEntries(list.map((entry) => [entry.file, entry])));
      const count = list.filter((entry) => entry.updateAvailable).length;
      toast.success(count ? `${count} Update${count === 1 ? "" : "s"} verfügbar` : "Alles ist aktuell");
    } catch (failure) {
      toast.error(failure instanceof Error ? failure.message : "Prüfung fehlgeschlagen");
    } finally {
      setChecking(false);
    }
  }

  async function updateAll(files: string[]) {
    const results = await api.post<{ file: string; updated: boolean; to?: string; error?: string }[]>(`/servers/${serverId}/addons/update`, { files });
    const ok = results.filter((entry) => entry.updated).length;
    const bad = results.filter((entry) => !entry.updated && entry.error && entry.error !== "schon aktuell");
    if (ok) toast.success(`${ok} aktualisiert${running ? " – Neustart nötig" : ""}`);
    if (bad.length) toast.error(bad.map((entry) => `${entry.file}: ${entry.error}`).join("\n"));
    setUpdates(null);
    mutate();
    reloadDetails();
  }

  async function toggle(addon: Addon) {
    const target = addon.enabled ? `${path(addon)}.disabled` : path(addon).replace(/\.disabled$/, "");
    await toast.run(() => api.post(`/servers/${serverId}/files/rename`, { from: path(addon), to: target }), addon.enabled ? "Deaktiviert" : "Aktiviert");
    mutate();
  }

  return (
    <Card>
      <CardHeader
        title={`Installierte ${noun}`}
        description={running ? "Änderungen werden nach einem Neustart des Servers wirksam." : undefined}
        action={
          canWrite && (
            <div className="flex gap-2">
              <Button size="sm" onClick={check} loading={checking}>
                <RefreshCw className="h-3.5 w-3.5" />
                Updates prüfen
              </Button>
              {pending.length > 0 && (
                <Button size="sm" variant="primary" onClick={() => toast.run(() => updateAll(pending.map((entry) => entry.file)))}>
                  <ArrowUpCircle className="h-3.5 w-3.5" />
                  Alle aktualisieren ({pending.length})
                </Button>
              )}
              <Button size="sm" onClick={() => picker.current?.click()}>
                <Upload className="h-3.5 w-3.5" />
                Hochladen
              </Button>
              <input
                ref={picker}
                type="file"
                accept=".jar"
                multiple
                className="hidden"
                onChange={async (event) => {
                  const files = Array.from(event.target.files ?? []);
                  event.target.value = "";
                  if (files.length === 0) return;
                  const folder = data[0]?.dir ?? (noun === "Mods" ? "mods" : "plugins");
                  setProgress(0);
                  await toast.run(() => upload(`/servers/${serverId}/files/upload?path=${folder}&overwrite=1`, files, setProgress), "Hochgeladen");
                  setProgress(null);
                  mutate();
                }}
              />
            </div>
          )
        }
      />
      {progress !== null && (
        <div className="h-1 bg-white/[0.06]">
          <div className="h-full bg-accent" style={{ width: `${Math.round(progress * 100)}%` }} />
        </div>
      )}
      {data.length === 0 ? (
        <EmptyState icon={<Package className="h-6 w-6" />} title={`Keine ${noun} installiert`} text={canWrite ? "Durchsuche Modrinth oder lade eine .jar-Datei hoch." : undefined} />
      ) : (
        <div className="divide-y divide-white/[0.05]">
          {data.map((addon) => {
            const update = updates?.[addon.file];
            const detail = details.get(path(addon));
            return (
              <div key={`${addon.dir}/${addon.file}`} className="flex flex-wrap items-center gap-x-4 gap-y-2 px-5 py-3">
                {detail?.iconUrl ? (
                  // eslint-disable-next-line @next/next/no-img-element
                  <img src={detail.iconUrl} alt="" className="h-9 w-9 shrink-0 rounded-lg bg-raised object-cover" />
                ) : (
                  <Plug className={`h-4 w-4 shrink-0 ${addon.enabled ? "text-muted" : "text-muted/40"}`} />
                )}
                <div className="min-w-[180px] flex-1">
                  <p className={`truncate text-sm font-medium ${addon.enabled ? "" : "text-muted line-through"}`}>{detail?.title || addon.name || addon.file}</p>
                  <p className="truncate text-xs text-muted">
                    {detail?.versionLink ? (
                      <a className="text-accent hover:underline" href={detail.versionLink} target="_blank" rel="noreferrer" title={detail.versionName}>
                        {detail.versionNumber}
                      </a>
                    ) : (
                      addon.version
                    )}
                    {(detail?.versionNumber || addon.version) && " · "}
                    {addon.file} · {bytes(addon.size)}
                  </p>
                  {detail?.known && (
                    <p className="mt-0.5 flex flex-wrap items-center gap-x-3 text-[11px] text-muted">
                      {detail.link && (
                        <a className="inline-flex items-center gap-1 hover:text-ink hover:underline" href={detail.link} target="_blank" rel="noreferrer">
                          <ExternalLink className="h-3 w-3" />
                          Modrinth-Seite
                        </a>
                      )}
                      {detail.sourceUrl && (
                        <a className="hover:text-ink hover:underline" href={detail.sourceUrl} target="_blank" rel="noreferrer">
                          Quellcode
                        </a>
                      )}
                      {detail.issuesUrl && (
                        <a className="hover:text-ink hover:underline" href={detail.issuesUrl} target="_blank" rel="noreferrer">
                          Fehler melden
                        </a>
                      )}
                      {detail.license && <span>{detail.license}</span>}
                      {detail.versionType && detail.versionType !== "release" && <Badge tone="warn">{detail.versionType}</Badge>}
                    </p>
                  )}
                </div>
                {!addon.enabled && <Badge>Deaktiviert</Badge>}
                {update?.updateAvailable && (
                  <Badge tone="accent">
                    <ArrowUpCircle className="h-3 w-3" />
                    {update.latest}
                  </Badge>
                )}
                {update && !update.updateAvailable && update.known && (
                  <Badge tone="good">
                    <CheckCircle2 className="h-3 w-3" />
                    Aktuell
                  </Badge>
                )}
                {update && !update.known && <span title="Nicht bei Modrinth gefunden"><Badge>Unbekannt</Badge></span>}
                {canWrite && (
                  <div className="flex items-center gap-1">
                    {update?.updateAvailable && (
                      <Button size="sm" onClick={() => toast.run(() => updateAll([addon.file]))}>
                        Aktualisieren
                      </Button>
                    )}
                    {detail?.known && detail.projectId && (
                      <Button variant="ghost" size="icon-sm" onClick={() => setSwitching(detail)} title="Version wechseln" aria-label="Version wechseln">
                        <Repeat className="h-3.5 w-3.5" />
                      </Button>
                    )}
                    <Button variant="ghost" size="icon-sm" onClick={() => toggle(addon)} title={addon.enabled ? "Deaktivieren" : "Aktivieren"} aria-label="Umschalten">
                      <Power className="h-3.5 w-3.5" />
                    </Button>
                    <Button variant="ghost" size="icon-sm" onClick={() => setRemove(addon)} title="Löschen" aria-label="Löschen">
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
      <SwitchDialog
        serverId={serverId}
        detail={switching}
        onClose={() => setSwitching(null)}
        onDone={() => {
          setSwitching(null);
          setUpdates(null);
          mutate();
          reloadDetails();
        }}
        running={running}
      />
      <ConfirmDialog
        open={!!remove}
        onClose={() => setRemove(null)}
        title={`„${remove?.name || remove?.file}“ löschen?`}
        text="Die Datei wird vom Server gelöscht. Die Konfiguration des Plugins bleibt erhalten."
        confirm="Löschen"
        danger
        onConfirm={async () => {
          if (!remove) return;
          await toast.run(() => api.delete(`/servers/${serverId}/files?path=${encodeURIComponent(path(remove))}`), "Gelöscht");
          mutate();
        }}
      />
    </Card>
  );
}

// Tauscht eine installierte Datei gegen eine andere Version desselben Projekts (auch ältere).
function SwitchDialog({ serverId, detail, onClose, onDone, running }: { serverId: string; detail: AddonDetail | null; onClose: () => void; onDone: () => void; running: boolean }) {
  const toast = useToast();
  const [busy, setBusy] = useState<string | null>(null);
  const { data, error } = useApi<ProjectDetail>(detail?.projectId ? `/servers/${serverId}/modrinth/project/${encodeURIComponent(detail.projectId)}` : null);
  return (
    <Dialog open={!!detail} onClose={onClose} title={`${detail?.title ?? ""}: Version wechseln`} description={`Installiert: ${detail?.versionNumber ?? "?"}${running ? " · Neustart nötig" : ""}`} wide>
      {error ? (
        <p className="text-sm text-bad">{error.message}</p>
      ) : !data ? (
        <Loading />
      ) : (
        <div className="max-h-[50vh] divide-y divide-white/[0.05] overflow-y-auto rounded-lg border border-white/[0.07]">
          {data.versions.slice(0, 60).map((version) => (
            <div key={version.id} className="flex items-center gap-3 px-3 py-2.5">
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm">{version.versionNumber}</p>
                <p className="truncate text-xs text-muted">
                  {version.name} · {new Date(version.published).toLocaleDateString("de-DE")}
                </p>
              </div>
              <Badge tone={version.type === "release" ? "good" : "warn"}>{version.type}</Badge>
              {version.id === detail?.versionId ? (
                <Badge tone="accent">Installiert</Badge>
              ) : (
                <Button
                  size="sm"
                  loading={busy === version.id}
                  disabled={!!busy}
                  onClick={async () => {
                    setBusy(version.id);
                    const ok = await toast.run(() => api.post(`/servers/${serverId}/addons/switch`, { file: detail?.file, versionId: version.id }), `Auf ${version.versionNumber} gewechselt`);
                    setBusy(null);
                    if (ok) onDone();
                  }}
                >
                  Wechseln
                </Button>
              )}
            </div>
          ))}
        </div>
      )}
    </Dialog>
  );
}

// ---------------------------------------------------------------- Modrinth

interface ProjectDetail {
  project: { id: string; title: string; description: string; icon_url: string; downloads: number; slug: string };
  versions: { id: string; name: string; versionNumber: string; type: string; published: string; gameVersions: string[] }[];
}

function Browse({ serverId, noun, onInstalled }: { serverId: string; noun: string; onInstalled: () => void }) {
  const toast = useToast();
  const [query, setQuery] = useState("");
  const [debounced, setDebounced] = useState("");
  const [detail, setDetail] = useState<string | null>(null);
  const [installing, setInstalling] = useState<string | null>(null);

  useEffect(() => {
    const timer = setTimeout(() => setDebounced(query.trim()), 350);
    return () => clearTimeout(timer);
  }, [query]);

  const { data, error, isLoading } = useApi<{ hits: ModrinthHit[]; total_hits: number }>(`/servers/${serverId}/modrinth/search?q=${encodeURIComponent(debounced)}`);

  async function install(project: string, versionId?: string) {
    setInstalling(project);
    try {
      const result = await api.post<{ installed: string[] }>(`/servers/${serverId}/modrinth/install`, { project, versionId: versionId ?? "" });
      toast.success(`${result.installed.join(", ")} installiert`);
      setDetail(null);
      onInstalled();
    } catch (failure) {
      toast.error(failure instanceof Error ? failure.message : "Installation fehlgeschlagen");
    } finally {
      setInstalling(null);
    }
  }

  return (
    <div className="space-y-4">
      <SearchInput value={query} onChange={(event) => setQuery(event.target.value)} placeholder={`${noun} auf Modrinth suchen – nur passende zu Software und Version`} />
      {error ? (
        <ErrorState error={error} />
      ) : isLoading && !data ? (
        <Loading label="Suche läuft" />
      ) : data && data.hits.length === 0 ? (
        <Card>
          <EmptyState icon={<Search className="h-6 w-6" />} title="Nichts gefunden" text="Probiere einen anderen Suchbegriff." />
        </Card>
      ) : (
        <div className="grid gap-3 lg:grid-cols-2">
          {data?.hits.map((hit) => (
            <Card key={hit.project_id} className="flex items-start gap-3 p-4">
              {/* eslint-disable-next-line @next/next/no-img-element */}
              {hit.icon_url ? <img src={hit.icon_url} alt="" className="h-12 w-12 shrink-0 rounded-lg bg-raised object-cover" /> : <span className="grid h-12 w-12 shrink-0 place-items-center rounded-lg bg-raised text-muted"><Package className="h-5 w-5" /></span>}
              <div className="min-w-0 flex-1">
                <button className="truncate text-left text-sm font-semibold hover:underline" onClick={() => setDetail(hit.project_id)}>
                  {hit.title}
                </button>
                <p className="text-xs text-muted">
                  von {hit.author} · {compact(hit.downloads)} Downloads
                </p>
                <p className="mt-1.5 line-clamp-2 text-xs text-ink/70">{hit.description}</p>
              </div>
              <Button size="sm" variant="primary" loading={installing === hit.project_id} onClick={() => install(hit.project_id)}>
                <Download className="h-3.5 w-3.5" />
                Installieren
              </Button>
            </Card>
          ))}
        </div>
      )}
      <DetailDialog serverId={serverId} project={detail} installing={!!installing} onClose={() => setDetail(null)} onInstall={install} />
    </div>
  );
}

function DetailDialog({ serverId, project, installing, onClose, onInstall }: { serverId: string; project: string | null; installing: boolean; onClose: () => void; onInstall: (project: string, versionId?: string) => void }) {
  const { data, error } = useApi<ProjectDetail>(project ? `/servers/${serverId}/modrinth/project/${encodeURIComponent(project)}` : null);
  return (
    <Dialog open={!!project} onClose={onClose} title={data?.project.title ?? "Wird geladen"} description={data?.project.description} wide>
      {error ? (
        <p className="text-sm text-bad">{error.message}</p>
      ) : !data ? (
        <Loading />
      ) : data.versions.length === 0 ? (
        <p className="text-sm text-muted">Es gibt keine Version, die zu dieser Software und Spielversion passt.</p>
      ) : (
        <div className="max-h-[50vh] divide-y divide-white/[0.05] overflow-y-auto rounded-lg border border-white/[0.07]">
          {data.versions.slice(0, 40).map((version) => (
            <div key={version.id} className="flex items-center gap-3 px-3 py-2.5">
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm">{version.versionNumber}</p>
                <p className="truncate text-xs text-muted">
                  {version.name} · {new Date(version.published).toLocaleDateString("de-DE")}
                </p>
              </div>
              <Badge tone={version.type === "release" ? "good" : "warn"}>{version.type}</Badge>
              <Button size="sm" disabled={installing} onClick={() => onInstall(data.project.id, version.id)}>
                Installieren
              </Button>
            </div>
          ))}
        </div>
      )}
    </Dialog>
  );
}
