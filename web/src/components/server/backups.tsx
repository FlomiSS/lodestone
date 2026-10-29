"use client";

import { Archive, ArchiveRestore, Download, Loader2, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { Badge, Button, Card, CardHeader, EmptyState } from "@/components/ui/primitives";
import { ConfirmDialog, Dialog, useToast } from "@/components/ui/feedback";
import { ErrorState, Loading } from "@/components/ui/async";
import { api, downloadUrl, useApi } from "@/lib/api";
import { ago, bytes, dateTime } from "@/lib/format";
import type { Backup } from "@/lib/types";

export function BackupsTab({ serverId, canWrite, offline }: { serverId: string; canWrite: boolean; offline: boolean }) {
  const toast = useToast();
  const { data, error, mutate } = useApi<Backup[]>(`/servers/${serverId}/backups`, {
    // Solange eines entsteht, regelmäßig nachsehen
    refreshInterval: (latest) => (latest?.some((backup) => backup.creating) ? 2000 : 0),
  });
  const [creating, setCreating] = useState(false);
  const [label, setLabel] = useState("");
  const [restore, setRestore] = useState<Backup | null>(null);
  const [remove, setRemove] = useState<Backup | null>(null);

  if (error) return <ErrorState error={error} retry={() => mutate()} />;
  if (!data) return <Loading />;

  return (
    <Card>
      <CardHeader
        title="Backups"
        description="Archive des gesamten Serverordners. Läuft der Server, wird er für die Sicherung kurz am Speichern gehindert, damit sie stimmig ist."
        action={
          canWrite && (
            <Button size="sm" variant="primary" onClick={() => setCreating(true)}>
              <Plus className="h-3.5 w-3.5" />
              Backup erstellen
            </Button>
          )
        }
      />
      {data.length === 0 ? (
        <EmptyState icon={<Archive className="h-6 w-6" />} title="Noch keine Backups" />
      ) : (
        <div className="divide-y divide-white/[0.05]">
          {data.map((backup) => (
            <div key={backup.name} className="flex flex-wrap items-center gap-x-4 gap-y-2 px-5 py-3">
              {backup.creating ? <Loader2 className="h-4 w-4 animate-spin text-muted" /> : <Archive className="h-4 w-4 text-muted" />}
              <div className="min-w-[180px] flex-1">
                <p className="text-sm font-medium">{backup.label || dateTime(backup.created)}</p>
                <p className="text-xs text-muted">
                  {backup.label && `${dateTime(backup.created)} · `}
                  {ago(backup.created)} · {backup.creating ? "wird erstellt …" : bytes(backup.size)}
                </p>
              </div>
              {backup.creating && <Badge tone="warn">Läuft</Badge>}
              {!backup.creating && (
                <div className="flex items-center gap-1">
                  {canWrite && (
                    <Button size="sm" disabled={!offline} title={offline ? undefined : "Der Server muss aus sein"} onClick={() => setRestore(backup)}>
                      <ArchiveRestore className="h-3.5 w-3.5" />
                      Wiederherstellen
                    </Button>
                  )}
                  {canWrite && (
                    <Button variant="ghost" size="icon-sm" aria-label="Herunterladen" onClick={() => window.open(downloadUrl(`/servers/${serverId}/backups/${backup.name}/download`))}>
                      <Download className="h-3.5 w-3.5" />
                    </Button>
                  )}
                  {canWrite && (
                    <Button variant="ghost" size="icon-sm" aria-label="Löschen" onClick={() => setRemove(backup)}>
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  )}
                </div>
              )}
            </div>
          ))}
        </div>
      )}

      <Dialog
        open={creating}
        onClose={() => setCreating(false)}
        title="Backup erstellen"
        footer={
          <>
            <Button variant="ghost" onClick={() => setCreating(false)}>
              Abbrechen
            </Button>
            <Button
              variant="primary"
              onClick={async () => {
                if (await toast.run(() => api.post(`/servers/${serverId}/backups`, { label }), "Backup wird erstellt")) {
                  setCreating(false);
                  setLabel("");
                  mutate();
                }
              }}
            >
              Erstellen
            </Button>
          </>
        }
      >
        <div>
          <label className="label">Bezeichnung (optional)</label>
          <input className="field" value={label} onChange={(event) => setLabel(event.target.value)} maxLength={40} placeholder="vor-dem-update" autoFocus />
        </div>
      </Dialog>

      <ConfirmDialog
        open={!!restore}
        onClose={() => setRestore(null)}
        title="Backup wiederherstellen?"
        text="Alle Dateien des Servers werden durch den Stand des Backups ersetzt. Aktuelle Welten und Einstellungen gehen verloren – erstelle vorher ein Backup, wenn du den heutigen Stand behalten willst."
        confirm="Wiederherstellen"
        danger
        onConfirm={async () => {
          if (!restore) return;
          await toast.run(() => api.post(`/servers/${serverId}/backups/${restore.name}/restore`), "Wiederherstellung läuft");
          mutate();
        }}
      />
      <ConfirmDialog
        open={!!remove}
        onClose={() => setRemove(null)}
        title="Backup löschen?"
        text="Das Archiv wird unwiderruflich gelöscht."
        confirm="Löschen"
        danger
        onConfirm={async () => {
          if (!remove) return;
          await toast.run(() => api.delete(`/servers/${serverId}/backups/${remove.name}`), "Gelöscht");
          mutate();
        }}
      />
    </Card>
  );
}
