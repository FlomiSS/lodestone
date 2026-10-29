"use client";

import { Archive, ArrowLeft, ChevronRight, Download, File as FileIcon, FilePlus, Folder, FolderInput, FolderPlus, PackageOpen, Pencil, RefreshCw, Save, Trash2, Upload, X } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { Badge, Button, Card, Checkbox, EmptyState, RowMenu } from "@/components/ui/primitives";
import { ConfirmDialog, Dialog, useToast } from "@/components/ui/feedback";
import { ErrorState, Loading } from "@/components/ui/async";
import { CodeEditor, isEditable, languageFor } from "./code-editor";
import { api, downloadUrl, fetchText, putText, upload, useApi } from "@/lib/api";
import { bytes, dateTime, joinPath, parentPath } from "@/lib/format";
import type { FileEntry } from "@/lib/types";

export function FilesTab({ serverId, canWrite }: { serverId: string; canWrite: boolean }) {
  const toast = useToast();
  const [path, setPath] = useState("");
  const [editing, setEditing] = useState<string | null>(null);
  const [rename, setRename] = useState<FileEntry | null>(null);
  const [remove, setRemove] = useState<FileEntry | null>(null);
  const [creating, setCreating] = useState<"file" | "folder" | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [moving, setMoving] = useState<string[] | null>(null);
  const [archiving, setArchiving] = useState<string[] | null>(null);
  const [extracting, setExtracting] = useState<FileEntry | null>(null);
  const [bulkRemove, setBulkRemove] = useState<string[] | null>(null);
  const [progress, setProgress] = useState<number | null>(null);
  const [dragging, setDragging] = useState(false);
  const picker = useRef<HTMLInputElement>(null);

  const { data, error, mutate, isLoading } = useApi<FileEntry[]>(`/servers/${serverId}/files?path=${encodeURIComponent(path)}`);

  const entries = [...(data ?? [])].sort((a, b) => Number(b.dir) - Number(a.dir) || a.name.localeCompare(b.name, "de", { numeric: true }));
  const crumbs = path.split("/").filter(Boolean);
  const chosen = entries.filter((entry) => selected.has(entry.name)).map((entry) => entry.name);
  const allChosen = entries.length > 0 && chosen.length === entries.length;
  const toggle = (name: string) =>
    setSelected((current) => {
      const next = new Set(current);
      if (!next.delete(name)) next.add(name);
      return next;
    });
  const done = () => {
    setSelected(new Set());
    mutate();
  };
  const fullPaths = (names: string[]) => names.map((name) => joinPath(path, name));

  useEffect(() => setSelected(new Set()), [path]);

  const send = useCallback(
    async (files: File[]) => {
      if (files.length === 0) return;
      setProgress(0);
      try {
        await upload(`/servers/${serverId}/files/upload?path=${encodeURIComponent(path)}&overwrite=1`, files, setProgress);
        toast.success(files.length === 1 ? `${files[0].name} hochgeladen` : `${files.length} Dateien hochgeladen`);
        mutate();
      } catch (failure) {
        toast.error(failure instanceof Error ? failure.message : "Upload fehlgeschlagen");
      } finally {
        setProgress(null);
      }
    },
    [serverId, path, mutate, toast],
  );

  if (editing !== null) {
    return (
      <FileEditor
        serverId={serverId}
        path={editing}
        canWrite={canWrite}
        onClose={() => {
          setEditing(null);
          mutate();
        }}
      />
    );
  }

  return (
    <Card
      className={dragging ? "border-accent/60" : undefined}
      onDragOver={(event) => {
        if (!canWrite) return;
        event.preventDefault();
        setDragging(true);
      }}
      onDragLeave={() => setDragging(false)}
      onDrop={(event) => {
        event.preventDefault();
        setDragging(false);
        if (canWrite) send(Array.from(event.dataTransfer.files));
      }}
    >
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-white/[0.06] px-4 py-3">
        <nav className="flex min-w-0 flex-wrap items-center gap-1 text-sm">
          <button className="rounded px-1.5 py-0.5 text-muted hover:bg-white/[0.06] hover:text-ink" onClick={() => setPath("")}>
            Server
          </button>
          {crumbs.map((part, index) => (
            <span key={index} className="flex items-center gap-1">
              <ChevronRight className="h-3.5 w-3.5 text-muted/60" />
              <button className="rounded px-1.5 py-0.5 hover:bg-white/[0.06]" onClick={() => setPath(crumbs.slice(0, index + 1).join("/"))}>
                {part}
              </button>
            </span>
          ))}
        </nav>
        <div className="flex items-center gap-2">
          {path && (
            <Button size="sm" variant="ghost" onClick={() => setPath(parentPath(path))}>
              <ArrowLeft className="h-3.5 w-3.5" />
              Zurück
            </Button>
          )}
          <Button size="sm" variant="ghost" onClick={() => mutate()} aria-label="Aktualisieren">
            <RefreshCw className="h-3.5 w-3.5" />
          </Button>
          {canWrite && (
            <>
              <Button size="sm" onClick={() => setCreating("file")}>
                <FilePlus className="h-3.5 w-3.5" />
                Datei
              </Button>
              <Button size="sm" onClick={() => setCreating("folder")}>
                <FolderPlus className="h-3.5 w-3.5" />
                Ordner
              </Button>
              <Button size="sm" variant="primary" onClick={() => picker.current?.click()}>
                <Upload className="h-3.5 w-3.5" />
                Hochladen
              </Button>
              <input
                ref={picker}
                type="file"
                multiple
                className="hidden"
                onChange={(event) => {
                  send(Array.from(event.target.files ?? []));
                  event.target.value = "";
                }}
              />
            </>
          )}
        </div>
      </div>

      {canWrite && chosen.length > 0 && (
        <div className="flex flex-wrap items-center gap-2 border-b border-white/[0.06] bg-accent/10 px-4 py-2 text-sm">
          <span className="mr-auto">{chosen.length} ausgewählt</span>
          <Button size="sm" onClick={() => setMoving(chosen)}>
            <FolderInput className="h-3.5 w-3.5" />
            Verschieben
          </Button>
          <Button size="sm" onClick={() => setArchiving(chosen)}>
            <Archive className="h-3.5 w-3.5" />
            Archivieren
          </Button>
          <Button size="sm" variant="danger" onClick={() => setBulkRemove(chosen)}>
            <Trash2 className="h-3.5 w-3.5" />
            Löschen
          </Button>
          <Button size="sm" variant="ghost" onClick={() => setSelected(new Set())}>
            Aufheben
          </Button>
        </div>
      )}

      {progress !== null && (
        <div className="h-1 bg-white/[0.06]">
          <div className="h-full bg-accent transition-all" style={{ width: `${Math.round(progress * 100)}%` }} />
        </div>
      )}

      {error ? (
        <div className="p-4">
          <ErrorState error={error} retry={() => mutate()} />
        </div>
      ) : isLoading && !data ? (
        <Loading label="Dateien werden geladen" />
      ) : entries.length === 0 ? (
        <EmptyState icon={<Folder className="h-6 w-6" />} title="Dieser Ordner ist leer" text={canWrite ? "Ziehe Dateien hierher, um sie hochzuladen." : undefined} />
      ) : (
        <div className="divide-y divide-white/[0.04]">
          {canWrite && (
            <div className="flex items-center gap-3 px-4 py-1.5 text-xs text-muted">
              <Checkbox checked={allChosen} onChange={(value) => setSelected(value ? new Set(entries.map((entry) => entry.name)) : new Set())} label="Alle auswählen" />
              <span>Alle auswählen</span>
            </div>
          )}
          {entries.map((entry) => {
            const full = joinPath(path, entry.name);
            const editable = !entry.dir && isEditable(entry.name, entry.size);
            return (
              <div
                key={entry.name}
                className="row-hover flex cursor-pointer items-center gap-3 px-4 py-2.5"
                onClick={() => (entry.dir ? setPath(full) : editable ? setEditing(full) : window.open(downloadUrl(`/servers/${serverId}/files/download?path=${encodeURIComponent(full)}`)))}
              >
                {canWrite && <Checkbox checked={selected.has(entry.name)} onChange={() => toggle(entry.name)} label={`${entry.name} auswählen`} />}
                {entry.dir ? <Folder className="h-4 w-4 shrink-0 text-sky-400" /> : <FileIcon className="h-4 w-4 shrink-0 text-muted" />}
                <span className="min-w-0 flex-1 truncate text-sm">{entry.name}</span>
                {entry.symlink && <Badge>Link</Badge>}
                <span className="hidden w-24 text-right text-xs text-muted sm:block">{entry.dir ? "" : bytes(entry.size)}</span>
                <span className="hidden w-32 text-right text-xs text-muted md:block">{dateTime(entry.modified)}</span>
                <RowMenu
                  items={[
                    ...(editable ? [{ label: canWrite ? "Bearbeiten" : "Ansehen", icon: <Pencil className="h-3.5 w-3.5" />, onClick: () => setEditing(full) }] : []),
                    ...(!entry.dir ? [{ label: "Herunterladen", icon: <Download className="h-3.5 w-3.5" />, onClick: () => window.open(downloadUrl(`/servers/${serverId}/files/download?path=${encodeURIComponent(full)}`)) }] : []),
                    ...(canWrite ? ["separator" as const, { label: "Umbenennen", icon: <Pencil className="h-3.5 w-3.5" />, onClick: () => setRename(entry) }, { label: "Verschieben", icon: <FolderInput className="h-3.5 w-3.5" />, onClick: () => setMoving([entry.name]) }, { label: "Archivieren", icon: <Archive className="h-3.5 w-3.5" />, onClick: () => setArchiving([entry.name]) }, ...(!entry.dir && /\.(zip|tar|tar\.gz|tgz)$/i.test(entry.name) ? [{ label: "Entpacken", icon: <PackageOpen className="h-3.5 w-3.5" />, onClick: () => setExtracting(entry) }] : []), { label: "Löschen", icon: <Trash2 className="h-3.5 w-3.5" />, danger: true, onClick: () => setRemove(entry) }] : []),
                  ]}
                />
              </div>
            );
          })}
        </div>
      )}

      <NameDialog
        open={creating !== null}
        title={creating === "folder" ? "Neuer Ordner" : "Neue Datei"}
        label="Name"
        confirm="Anlegen"
        onClose={() => setCreating(null)}
        onSubmit={async (name) => {
          const full = joinPath(path, name);
          if (creating === "folder") await toast.run(() => api.post(`/servers/${serverId}/files/mkdir`, { path: full }), "Ordner angelegt");
          else if (await toast.run(() => putText(`/servers/${serverId}/files/content?path=${encodeURIComponent(full)}&create=1`, ""))) setEditing(full);
          mutate();
        }}
      />
      <NameDialog
        open={!!rename}
        title="Umbenennen"
        label="Neuer Name"
        confirm="Umbenennen"
        initial={rename?.name}
        onClose={() => setRename(null)}
        onSubmit={async (name) => {
          if (!rename) return;
          await toast.run(() => api.post(`/servers/${serverId}/files/rename`, { from: joinPath(path, rename.name), to: joinPath(path, name) }), "Umbenannt");
          mutate();
        }}
      />
      <NameDialog
        open={!!moving}
        title={`${moving?.length ?? 0} Eintrag/Einträge verschieben`}
        label="Zielordner (relativ zum Serverordner, leer = Wurzel)"
        confirm="Verschieben"
        initial={path}
        allowSlash
        allowEmpty
        onClose={() => setMoving(null)}
        onSubmit={async (dest) => {
          if (!moving) return;
          await toast.run(() => api.post(`/servers/${serverId}/files/move`, { paths: fullPaths(moving), dest }), "Verschoben");
          done();
        }}
      />
      <NameDialog
        open={!!archiving}
        title="Archivieren"
        label="Archivname (.zip oder .tar.gz)"
        confirm="Archiv erstellen"
        initial={archiving && archiving.length === 1 ? `${archiving[0]}.zip` : "archiv.zip"}
        onClose={() => setArchiving(null)}
        onSubmit={async (name) => {
          if (!archiving) return;
          await toast.run(() => api.post(`/servers/${serverId}/files/archive`, { paths: fullPaths(archiving), dest: joinPath(path, name) }), "Archiv erstellt");
          done();
        }}
      />
      <NameDialog
        open={!!extracting}
        title={`„${extracting?.name}“ entpacken`}
        label="Zielordner (relativ zum Serverordner)"
        confirm="Entpacken"
        initial={path}
        allowSlash
        allowEmpty
        onClose={() => setExtracting(null)}
        onSubmit={async (dest) => {
          if (!extracting) return;
          await toast.run(() => api.post(`/servers/${serverId}/files/extract`, { path: joinPath(path, extracting.name), dest }), "Entpackt");
          done();
        }}
      />
      <ConfirmDialog
        open={!!bulkRemove}
        onClose={() => setBulkRemove(null)}
        title={`${bulkRemove?.length ?? 0} Einträge löschen?`}
        text="Ordner werden samt Inhalt gelöscht."
        confirm="Löschen"
        danger
        onConfirm={async () => {
          if (!bulkRemove) return;
          await toast.run(() => api.post(`/servers/${serverId}/files/delete`, { paths: fullPaths(bulkRemove) }), "Gelöscht");
          done();
        }}
      />
      <ConfirmDialog
        open={!!remove}
        onClose={() => setRemove(null)}
        title={`„${remove?.name}“ löschen?`}
        text={remove?.dir ? "Der Ordner wird samt Inhalt gelöscht." : "Die Datei wird gelöscht."}
        confirm="Löschen"
        danger
        onConfirm={async () => {
          if (!remove) return;
          await toast.run(() => api.delete(`/servers/${serverId}/files?path=${encodeURIComponent(joinPath(path, remove.name))}&recursive=1`), "Gelöscht");
          mutate();
        }}
      />
    </Card>
  );
}

function NameDialog({ open, title, label, confirm, initial, allowSlash, allowEmpty, onClose, onSubmit }: { open: boolean; title: string; label: string; confirm: string; initial?: string; allowSlash?: boolean; allowEmpty?: boolean; onClose: () => void; onSubmit: (name: string) => Promise<void> }) {
  const invalid = (value: string) => (!allowEmpty && !value.trim()) || (!allowSlash && value.includes("/"));
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (open) setName(initial ?? "");
  }, [open, initial]);
  async function submit() {
    if (invalid(name)) return;
    setBusy(true);
    try {
      await onSubmit(name.trim());
      onClose();
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={title}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Abbrechen
          </Button>
          <Button variant="primary" loading={busy} disabled={invalid(name)} onClick={submit}>
            {confirm}
          </Button>
        </>
      }
    >
      <div>
        <label className="label">{label}</label>
        <input className="field" value={name} onChange={(event) => setName(event.target.value)} onKeyDown={(event) => event.key === "Enter" && submit()} autoFocus />
      </div>
    </Dialog>
  );
}

// ---------------------------------------------------------------- Editor

function FileEditor({ serverId, path, canWrite, onClose }: { serverId: string; path: string; canWrite: boolean; onClose: () => void }) {
  const toast = useToast();
  const [original, setOriginal] = useState<string | null>(null);
  const [text, setText] = useState("");
  const [failure, setFailure] = useState<Error | null>(null);
  const [saving, setSaving] = useState(false);
  const [confirmClose, setConfirmClose] = useState(false);
  const name = path.split("/").pop() ?? path;
  const dirty = original !== null && text !== original;

  useEffect(() => {
    fetchText(`/servers/${serverId}/files/content?path=${encodeURIComponent(path)}`)
      .then((content) => {
        setOriginal(content);
        setText(content);
      })
      .catch((error: Error) => setFailure(error));
  }, [serverId, path]);

  const save = useCallback(async () => {
    setSaving(true);
    try {
      await putText(`/servers/${serverId}/files/content?path=${encodeURIComponent(path)}`, text);
      setOriginal(text);
      toast.success(`${name} gespeichert`);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Speichern fehlgeschlagen");
    } finally {
      setSaving(false);
    }
  }, [serverId, path, text, name, toast]);

  useEffect(() => {
    if (!dirty) return;
    const warn = (event: BeforeUnloadEvent) => event.preventDefault();
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);

  return (
    <Card className="flex flex-col overflow-hidden">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-white/[0.06] px-4 py-2.5">
        <div className="flex min-w-0 items-center gap-2">
          <FileIcon className="h-4 w-4 shrink-0 text-muted" />
          <span className="truncate font-mono text-sm">{path}</span>
          {dirty && <Badge tone="warn">Ungespeichert</Badge>}
          {!canWrite && <Badge>Nur lesen</Badge>}
        </div>
        <div className="flex items-center gap-2">
          {canWrite && (
            <Button size="sm" variant="primary" onClick={save} loading={saving} disabled={!dirty}>
              <Save className="h-3.5 w-3.5" />
              Speichern
            </Button>
          )}
          <Button size="sm" variant="ghost" onClick={() => (dirty ? setConfirmClose(true) : onClose())}>
            <X className="h-3.5 w-3.5" />
            Schließen
          </Button>
        </div>
      </div>
      <div className="h-[min(68vh,720px)] min-h-[360px] bg-black/30">
        {failure ? (
          <div className="p-4">
            <ErrorState error={failure} />
          </div>
        ) : original === null ? (
          <Loading label="Datei wird geladen" />
        ) : (
          <CodeEditor key={path} value={original} language={languageFor(name)} onChange={setText} onSave={() => canWrite && save()} readOnly={!canWrite} />
        )}
      </div>
      <ConfirmDialog open={confirmClose} onClose={() => setConfirmClose(false)} title="Änderungen verwerfen?" text="Die Datei hat ungespeicherte Änderungen." confirm="Verwerfen" danger onConfirm={onClose} />
    </Card>
  );
}
