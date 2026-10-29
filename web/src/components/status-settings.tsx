"use client";

import { Copy, ExternalLink } from "lucide-react";
import { useState } from "react";
import { Button, Card, CardHeader, Switch } from "@/components/ui/primitives";
import { useToast } from "@/components/ui/feedback";
import { api } from "@/lib/api";
import type { Project, ProjectSettings } from "@/lib/types";

/** Einstellungen der öffentlichen Statusseite eines Projekts. */
export function StatusCard({ project, refresh }: { project: Project; refresh: () => void }) {
  const toast = useToast();
  const settings = project.settings ?? { statusPublic: false, statusSlug: "", motd: "" };
  const [slug, setSlug] = useState(settings.statusSlug);
  const [busy, setBusy] = useState(false);
  const origin = typeof window === "undefined" ? "" : window.location.origin;
  const link = `${origin}/status/?p=${settings.statusSlug}`;

  async function save(patch: { public?: boolean; slug?: string }) {
    setBusy(true);
    try {
      const result = await api.put<ProjectSettings>(`/projects/${project.id}/status`, { public: patch.public ?? settings.statusPublic, slug: patch.slug ?? slug });
      setSlug(result.statusSlug);
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
        title="Öffentliche Statusseite"
        description="Eine Seite ohne Anmeldung, die zeigt, ob das Projekt online ist und wie viele Spieler da sind. Sie zeigt keine internen Angaben."
        action={<Switch checked={settings.statusPublic} disabled={busy} onChange={(value) => save({ public: value })} />}
      />
      {settings.statusPublic && (
        <div className="space-y-3 px-5 py-4 text-xs">
          <div className="flex flex-wrap items-end gap-3">
            <div>
              <label className="label">Adresse</label>
              <input className="field w-56" value={slug} onChange={(event) => setSlug(event.target.value.toLowerCase())} maxLength={40} />
            </div>
            <Button size="sm" disabled={busy || slug === settings.statusSlug} onClick={() => save({ slug })}>
              Ändern
            </Button>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <code className="break-all rounded bg-black/30 px-2 py-1">{link}</code>
            <Button size="sm" variant="ghost" onClick={() => navigator.clipboard.writeText(link).then(() => toast.success("Kopiert"))}>
              <Copy className="h-3.5 w-3.5" />
            </Button>
            <a href={link} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-accent hover:underline">
              <ExternalLink className="h-3.5 w-3.5" />
              Öffnen
            </a>
          </div>
          <p className="text-muted">
            Einbinden in eine Webseite: <code className="rounded bg-black/30 px-1.5 py-0.5">{`<iframe src="${link}&embed=1" width="360" height="150" style="border:0"></iframe>`}</code> · Die Rohdaten (JSON) liegen unter{" "}
            <code className="rounded bg-black/30 px-1.5 py-0.5">/api/public/status/{settings.statusSlug}</code>.
          </p>
        </div>
      )}
    </Card>
  );
}
