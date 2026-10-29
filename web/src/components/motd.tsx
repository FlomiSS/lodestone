"use client";

import { Save } from "lucide-react";
import { useState, type CSSProperties, type ReactNode } from "react";
import { Button, Card, CardHeader } from "@/components/ui/primitives";
import { useToast } from "@/components/ui/feedback";
import { api } from "@/lib/api";
import type { Project, SyncResult } from "@/lib/types";

const colors: Record<string, string> = {
  black: "#000000",
  dark_blue: "#0000aa",
  dark_green: "#00aa00",
  dark_aqua: "#00aaaa",
  dark_red: "#aa0000",
  dark_purple: "#aa00aa",
  gold: "#ffaa00",
  gray: "#aaaaaa",
  dark_gray: "#555555",
  blue: "#5555ff",
  green: "#55ff55",
  aqua: "#55ffff",
  red: "#ff5555",
  light_purple: "#ff55ff",
  yellow: "#ffff55",
  white: "#ffffff",
};

/** Zeigt eine MOTD im MiniMessage-Format so, wie Minecraft sie in der Serverliste darstellt (Farben und Formate; Verläufe nur als Text). */
export function renderMotd(text: string): ReactNode[] {
  const parts: ReactNode[] = [];
  let style: CSSProperties = {};
  const stack: CSSProperties[] = [];
  const pattern = /<(\/?)([#\w:]+)[^>]*>/g;
  let last = 0;
  let match: RegExpExecArray | null;
  let key = 0;
  const flush = (chunk: string) => {
    if (!chunk) return;
    chunk.split("\n").forEach((line, index) => {
      if (index > 0) parts.push(<br key={`br${key++}`} />);
      if (line)
        parts.push(
          <span key={key++} style={style}>
            {line}
          </span>,
        );
    });
  };
  while ((match = pattern.exec(text))) {
    flush(text.slice(last, match.index));
    last = match.index + match[0].length;
    const name = match[2].toLowerCase();
    if (match[1] === "/") {
      style = stack.pop() ?? {};
      continue;
    }
    if (name === "reset") {
      stack.length = 0;
      style = {};
      continue;
    }
    const next: CSSProperties = { ...style };
    if (colors[name]) next.color = colors[name];
    else if (/^#[0-9a-f]{6}$/.test(name)) next.color = name;
    else if (name === "bold" || name === "b") next.fontWeight = 700;
    else if (name === "italic" || name === "i" || name === "em") next.fontStyle = "italic";
    else if (name === "underlined" || name === "u") next.textDecoration = "underline";
    else if (name === "strikethrough" || name === "st") next.textDecoration = "line-through";
    else continue; // unbekannte Tags (Verlauf, Klick …) werden einfach nicht dargestellt
    stack.push(style);
    style = next;
  }
  flush(text.slice(last));
  return parts;
}

export function MotdCard({ project, canManage, refresh }: { project: Project; canManage: boolean; refresh: () => void }) {
  const toast = useToast();
  const saved = project.settings?.motd ?? "";
  const [text, setText] = useState(saved);
  const [busy, setBusy] = useState(false);
  const lines = text.split("\n").length;

  return (
    <Card>
      <CardHeader
        title="MOTD (Serverliste)"
        description="Die zwei Zeilen unter dem Namen in der Minecraft-Serverliste. Format: MiniMessage, z. B. <gold>Willkommen <gray>im Netzwerk. Leer lassen für den Standardtext."
        action={
          canManage && (
            <Button
              size="sm"
              variant="primary"
              loading={busy}
              disabled={text === saved || lines > 2}
              onClick={async () => {
                setBusy(true);
                try {
                  const result = await api.put<SyncResult>(`/projects/${project.id}/motd`, { motd: text });
                  toast.success(result.failed.length ? "Gespeichert, aber nicht überall angewendet" : "MOTD gespeichert – der Proxy lädt sie neu");
                  refresh();
                } catch (error) {
                  toast.error(error instanceof Error ? error.message : "Fehler");
                } finally {
                  setBusy(false);
                }
              }}
            >
              <Save className="h-3.5 w-3.5" />
              Speichern
            </Button>
          )
        }
      />
      <div className="space-y-3 px-5 py-4">
        <textarea className="field font-mono text-xs" rows={2} value={text} disabled={!canManage} onChange={(event) => setText(event.target.value)} placeholder={"<gold><bold>Mein Netzwerk</bold>\n<gray>Jetzt beitreten!"} spellCheck={false} />
        {lines > 2 && <p className="text-xs text-bad">Höchstens zwei Zeilen.</p>}
        <div className="rounded-lg border border-white/[0.06] bg-[#1b1b1b] p-3">
          <div className="flex items-center gap-3">
            <span className="grid h-12 w-12 shrink-0 place-items-center rounded bg-black/50 text-[10px] text-muted">64×64</span>
            <div className="min-w-0 font-mono text-[13px] leading-tight text-[#aaaaaa]">
              <div className="mb-1 text-white">{project.name}</div>
              {text.trim() ? renderMotd(text) : <span className="text-[#555]">A Velocity Server</span>}
            </div>
          </div>
        </div>
      </div>
    </Card>
  );
}
