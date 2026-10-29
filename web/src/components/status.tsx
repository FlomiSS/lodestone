"use client";

import { Play, RotateCw, Skull, Square } from "lucide-react";
import { useState } from "react";
import { Badge, Button, Dot } from "@/components/ui/primitives";
import { ConfirmDialog, useToast } from "@/components/ui/feedback";
import { api } from "@/lib/api";
import { stateLabel, stateTone } from "@/lib/format";
import type { Server } from "@/lib/types";

export function StateBadge({ server }: { server: Server }) {
  if (!server.online) return <Badge tone="bad">Node offline</Badge>;
  const state = server.view?.state;
  const tone = stateTone(state);
  return (
    <Badge tone={tone === "neutral" ? "neutral" : tone}>
      <Dot tone={tone} />
      {state ? (stateLabel[state] ?? state) : "Unbekannt"}
    </Badge>
  );
}

/** Start, Stopp, Neustart und Kill für einen Server. */
export function PowerButtons({ server, onDone, size = "sm", enabled = true }: { server: Server; onDone: () => void; size?: "sm" | "md"; enabled?: boolean }) {
  const toast = useToast();
  const [busy, setBusy] = useState<string | null>(null);
  const [killing, setKilling] = useState(false);
  const state = server.view?.state;
  const usable = enabled && server.online && !!state;

  async function power(action: string, label: string) {
    setBusy(action);
    await toast.run(() => api.post(`/servers/${server.id}/power`, { action }), label);
    setBusy(null);
    onDone();
  }

  const canStart = usable && state === "offline";
  const canStop = usable && (state === "running" || state === "starting");
  const canRestart = usable && state === "running";

  return (
    <div className="flex items-center gap-1.5">
      <Button size={size} variant="secondary" disabled={!canStart} loading={busy === "start"} onClick={() => power("start", "Server startet")}>
        <Play className="h-3.5 w-3.5 text-good" />
        Start
      </Button>
      <Button size={size} variant="secondary" disabled={!canRestart} loading={busy === "restart"} onClick={() => power("restart", "Server startet neu")}>
        <RotateCw className="h-3.5 w-3.5" />
        Neustart
      </Button>
      <Button size={size} variant="secondary" disabled={!canStop} loading={busy === "stop"} onClick={() => power("stop", "Server wird gestoppt")}>
        <Square className="h-3.5 w-3.5 text-bad" />
        Stopp
      </Button>
      {usable && (state === "running" || state === "starting" || state === "stopping") && (
        <Button size={size} variant="ghost" onClick={() => setKilling(true)} title="Beendet den Server sofort, ungespeicherte Daten gehen verloren">
          <Skull className="h-3.5 w-3.5" />
        </Button>
      )}
      <ConfirmDialog
        open={killing}
        onClose={() => setKilling(false)}
        title="Server beenden erzwingen?"
        text="Der Server wird sofort beendet, ohne zu speichern. Nutze das nur, wenn „Stopp“ nicht mehr hilft."
        confirm="Sofort beenden"
        danger
        onConfirm={() => power("kill", "Server beendet")}
      />
    </div>
  );
}

export function ServerAddress({ server }: { server: Server }) {
  if (!server.address) return <span className="text-muted">–</span>;
  return <span className="font-mono text-xs">{server.address}</span>;
}
