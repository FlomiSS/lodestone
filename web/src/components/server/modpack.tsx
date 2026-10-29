"use client";

import { ExternalLink, Package } from "lucide-react";
import { Badge, Meter } from "@/components/ui/primitives";
import { useApi } from "@/lib/api";
import type { ModpackStatus, Server } from "@/lib/types";

/** Zeigt, aus welchem Modrinth-Modpack der Server stammt, und den Fortschritt der Installation. */
export function ModpackBanner({ server }: { server: Server }) {
  const pack = server.modpack;
  const { data } = useApi<ModpackStatus>(pack ? `/servers/${server.id}/modpack` : null, {
    refreshInterval: (latest) => (latest?.state === "running" || latest?.state === "idle" ? 2000 : 15000),
  });
  if (!pack) return null;

  const running = data?.state === "running" || data?.state === "idle";
  const percent = data && data.total > 0 ? (data.done / data.total) * 100 : 0;

  return (
    <div className="mb-5 flex flex-wrap items-center gap-3 rounded-lg border border-white/[0.07] bg-white/[0.02] px-4 py-3">
      {/* eslint-disable-next-line @next/next/no-img-element */}
      {pack.iconUrl ? <img src={pack.iconUrl} alt="" className="h-10 w-10 rounded-lg object-cover" /> : <Package className="h-6 w-6 text-muted" />}
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium">{pack.title}</p>
        <p className="truncate text-xs text-muted">
          Version {pack.versionNumber} · Minecraft {pack.minecraft}
        </p>
      </div>
      {data?.state === "failed" && <Badge tone="bad">Installation fehlgeschlagen</Badge>}
      {data?.state === "done" && <Badge tone="good">Installiert</Badge>}
      <a className="inline-flex items-center gap-1 text-xs text-accent hover:underline" href={`https://modrinth.com/modpack/${pack.slug}/version/${pack.versionId}`} target="_blank" rel="noreferrer">
        <ExternalLink className="h-3 w-3" />
        Auf Modrinth
      </a>
      {running && (
        <div className="w-full">
          <div className="mb-1.5 flex justify-between text-[11px] text-muted">
            <span>{data?.phase || "Wartet auf die Server-Datei"}</span>
            {data && data.total > 0 && (
              <span className="tabular-nums">
                {data.done} / {data.total}
              </span>
            )}
          </div>
          <Meter value={percent} max={100} />
        </div>
      )}
      {data?.state === "failed" && data.error && <p className="w-full text-xs text-bad">{data.error}</p>}
    </div>
  );
}
