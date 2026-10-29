"use client";

import { AlertTriangle, ArrowDownToLine, Download, Search, Send, X } from "lucide-react";
import { Fragment, useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { Badge, Button, Card, Dot } from "@/components/ui/primitives";
import { downloadUrl, wsBase } from "@/lib/api";
import { analyse } from "@/lib/crash";
import { ansiStrip, bytes, stateLabel, stateTone } from "@/lib/format";
import type { LogLine, ServerState, Stats } from "@/lib/types";

const MAX_LINES = 1500;

// ---------------------------------------------------------------- ANSI

const palette = ["#3f3f46", "#ef4444", "#22c55e", "#eab308", "#60a5fa", "#d946ef", "#22d3ee", "#e4e4e7"];
const bright = ["#71717a", "#f87171", "#4ade80", "#facc15", "#93c5fd", "#e879f9", "#67e8f9", "#fafafa"];

interface Style {
  color?: string;
  bold?: boolean;
  dim?: boolean;
}

function renderAnsi(text: string): ReactNode {
  // eslint-disable-next-line no-control-regex
  const parts = text.split(/(\u001b\[[0-9;]*m)/);
  let style: Style = {};
  const out: ReactNode[] = [];
  parts.forEach((part, index) => {
    // eslint-disable-next-line no-control-regex
    const match = /^\u001b\[([0-9;]*)m$/.exec(part);
    if (!match) {
      // andere Steuerzeichen (Cursor, Löschen) entfernen
      // eslint-disable-next-line no-control-regex
      const clean = part.replace(/\u001b\[[0-9;?]*[A-Za-ln-z]/g, "").replace(/\r/g, "");
      if (clean) {
        out.push(
          <span key={index} style={{ color: style.color, fontWeight: style.bold ? 600 : undefined, opacity: style.dim ? 0.65 : undefined }}>
            {clean}
          </span>,
        );
      }
      return;
    }
    const codes = match[1] === "" ? [0] : match[1].split(";").map(Number);
    for (let i = 0; i < codes.length; i++) {
      const code = codes[i];
      if (code === 0) style = {};
      else if (code === 1) style = { ...style, bold: true };
      else if (code === 2) style = { ...style, dim: true };
      else if (code === 22) style = { ...style, bold: false, dim: false };
      else if (code >= 30 && code <= 37) style = { ...style, color: palette[code - 30] };
      else if (code >= 90 && code <= 97) style = { ...style, color: bright[code - 90] };
      else if (code === 39) style = { ...style, color: undefined };
      else if (code === 38 && codes[i + 1] === 2) {
        style = { ...style, color: `rgb(${codes[i + 2]},${codes[i + 3]},${codes[i + 4]})` };
        i += 4;
      } else if (code === 38 && codes[i + 1] === 5) {
        i += 2; // 256-Farben: nicht umgesetzt
      }
    }
  });
  return out;
}

function tone(text: string): string {
  if (/\b(ERROR|SEVERE|Exception)\b/.test(text) || text.includes("[lodestone] Fehler")) return "text-bad";
  if (/\bWARN(ING)?\b/.test(text)) return "text-warn";
  return "";
}

// ---------------------------------------------------------------- Konsole

export function ConsoleTab({ serverId, canCommand, state: initialState, onState }: { serverId: string; canCommand: boolean; state?: ServerState; onState?: (state: ServerState) => void }) {
  const [filter, setFilter] = useState("");
  const [levels, setLevels] = useState({ warn: true, error: true, info: true });
  const [dismissed, setDismissed] = useState<string[]>([]);
  const [lines, setLines] = useState<LogLine[]>([]);
  const [connected, setConnected] = useState(false);
  const [state, setState] = useState<ServerState | undefined>(initialState);
  const [stats, setStats] = useState<Stats | undefined>();
  const [input, setInput] = useState("");
  const [follow, setFollow] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const socket = useRef<WebSocket | null>(null);
  const viewport = useRef<HTMLDivElement>(null);
  const history = useRef<string[]>([]);
  const cursor = useRef(-1);
  const onStateRef = useRef(onState);
  onStateRef.current = onState;

  useEffect(() => {
    let closed = false;
    let retry: ReturnType<typeof setTimeout>;

    function connect() {
      const ws = new WebSocket(`${wsBase()}/api/servers/${serverId}/ws`);
      socket.current = ws;
      ws.onopen = () => setConnected(true);
      ws.onclose = () => {
        setConnected(false);
        if (!closed) retry = setTimeout(connect, 2000);
      };
      ws.onmessage = (message) => {
        const event = JSON.parse(message.data as string) as { type: string; lines?: LogLine[]; line?: LogLine; state?: ServerState; stats?: Stats; error?: string };
        switch (event.type) {
          case "hello":
            setLines((event.lines ?? []).slice(-MAX_LINES));
            setState(event.state);
            setStats(event.stats);
            if (event.state) onStateRef.current?.(event.state);
            break;
          case "line":
            if (event.line) {
              const line = event.line;
              setLines((current) => (current.length && current[current.length - 1].seq >= line.seq ? current : [...current, line].slice(-MAX_LINES)));
            }
            break;
          case "state":
            setState(event.state);
            if (event.state) onStateRef.current?.(event.state);
            break;
          case "stats":
            setStats(event.stats);
            break;
          case "error":
            setError(event.error ?? "Fehler");
            setTimeout(() => setError(null), 5000);
            break;
        }
      };
    }
    connect();
    return () => {
      closed = true;
      clearTimeout(retry);
      socket.current?.close();
    };
  }, [serverId]);

  useEffect(() => {
    if (follow && viewport.current) viewport.current.scrollTop = viewport.current.scrollHeight;
  }, [lines, follow]);

  const send = useCallback(() => {
    const text = input.trim();
    if (!text || !socket.current || socket.current.readyState !== WebSocket.OPEN) return;
    socket.current.send(JSON.stringify({ type: "command", data: text }));
    history.current = [text, ...history.current.filter((entry) => entry !== text)].slice(0, 100);
    cursor.current = -1;
    setInput("");
  }, [input]);

  function onKey(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Enter") send();
    if (event.key === "ArrowUp" && history.current.length) {
      event.preventDefault();
      cursor.current = Math.min(cursor.current + 1, history.current.length - 1);
      setInput(history.current[cursor.current]);
    }
    if (event.key === "ArrowDown") {
      event.preventDefault();
      cursor.current = Math.max(cursor.current - 1, -1);
      setInput(cursor.current === -1 ? "" : history.current[cursor.current]);
    }
  }

  const visible = useMemo(() => {
    const needle = filter.trim().toLowerCase();
    return lines.filter((line) => {
      const level = /(ERROR|SEVERE|FATAL)|Exception/.test(line.text) ? "error" : /WARN(ING)?/.test(line.text) ? "warn" : "info";
      return levels[level] && (!needle || ansiStrip(line.text).toLowerCase().includes(needle));
    });
  }, [lines, filter, levels]);

  const rendered = useMemo(
    () =>
      visible.map((line) => (
        <div key={line.seq} className={tone(line.text)}>
          {renderAnsi(line.text) || " "}
        </div>
      )),
    [visible],
  );

  // Bekannte Fehlerbilder in den letzten Zeilen erklären (nur wenn der Server nicht läuft oder gerade gescheitert ist)
  const hints = useMemo(() => analyse(lines.map((line) => ansiStrip(line.text))).filter((hint) => !dismissed.includes(hint.id)), [lines, dismissed]);

  const tn = stateTone(state);
  const running = state === "running" || state === "starting";

  return (
    <Card className="flex flex-col overflow-hidden">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-white/[0.06] px-4 py-2.5">
        <div className="flex items-center gap-2">
          <Badge tone={tn === "neutral" ? "neutral" : tn}>
            <Dot tone={tn} />
            {state ? stateLabel[state] : "…"}
          </Badge>
          {!connected && <Badge tone="warn">Verbindung wird aufgebaut</Badge>}
        </div>
        {stats && running && (
          <div className="flex gap-4 text-xs text-muted">
            <span>
              CPU <b className="font-medium text-ink">{stats.cpuPercent.toFixed(0)} %</b>
            </span>
            <span>
              RAM{" "}
              <b className="font-medium text-ink">
                {bytes(stats.memoryBytes)}
                {stats.memoryLimit > 0 && ` / ${bytes(stats.memoryLimit)}`}
              </b>
            </span>
            <span className="hidden sm:inline">
              ↓ {bytes(stats.netRx)} ↑ {bytes(stats.netTx)}
            </span>
          </div>
        )}
      </div>

      {hints.length > 0 && (
        <div className="space-y-2 border-b border-white/[0.06] bg-white/[0.015] px-4 py-3">
          {hints.map((hint) => (
            <div key={hint.id} className={`flex items-start gap-3 rounded-lg border px-3 py-2.5 text-xs ${hint.level === "bad" ? "border-bad/25 bg-bad/10" : "border-warn/25 bg-warn/10"}`}>
              <AlertTriangle className={`mt-0.5 h-4 w-4 shrink-0 ${hint.level === "bad" ? "text-bad" : "text-warn"}`} />
              <div className="min-w-0 flex-1">
                <p className="font-medium text-ink">{hint.title}</p>
                <p className="mt-0.5 leading-relaxed text-ink/70">{hint.text}</p>
              </div>
              <button onClick={() => setDismissed((current) => [...current, hint.id])} className="rounded p-1 text-muted hover:bg-white/[0.08] hover:text-ink" aria-label="Ausblenden">
                <X className="h-3.5 w-3.5" />
              </button>
            </div>
          ))}
        </div>
      )}

      <div className="flex flex-wrap items-center gap-2 border-b border-white/[0.06] px-4 py-2">
        <div className="flex min-w-[180px] flex-1 items-center gap-2 rounded-md border border-white/[0.08] bg-white/[0.02] px-2.5">
          <Search className="h-3.5 w-3.5 shrink-0 text-muted" />
          <input className="h-8 w-full bg-transparent text-xs outline-none placeholder:text-muted/60" value={filter} onChange={(event) => setFilter(event.target.value)} placeholder="In der Konsole suchen" />
          {filter && (
            <button onClick={() => setFilter("")} className="text-muted hover:text-ink" aria-label="Suche löschen">
              <X className="h-3.5 w-3.5" />
            </button>
          )}
        </div>
        {(["info", "warn", "error"] as const).map((level) => (
          <button
            key={level}
            onClick={() => setLevels((current) => ({ ...current, [level]: !current[level] }))}
            className={`h-8 rounded-md border px-2.5 text-[11px] font-medium uppercase transition active:scale-95 ${levels[level] ? "border-white/[0.14] bg-white/[0.07] text-ink" : "border-white/[0.06] text-muted/60 line-through"}`}
          >
            {level}
          </button>
        ))}
        {canCommand && (
          <a href={downloadUrl(`/servers/${serverId}/files/download?path=logs/latest.log`)} className="inline-flex h-8 items-center gap-1.5 rounded-md border border-white/[0.09] px-3 text-xs text-muted transition hover:border-white/[0.18] hover:text-ink" title="Die Datei logs/latest.log des Servers herunterladen">
            <Download className="h-3.5 w-3.5" />
            Log
          </a>
        )}
        {filter && (
          <span className="text-[11px] text-muted">
            {visible.length} von {lines.length} Zeilen
          </span>
        )}
      </div>

      <div className="relative">
        <div
          ref={viewport}
          onScroll={(event) => {
            const element = event.currentTarget;
            setFollow(element.scrollHeight - element.scrollTop - element.clientHeight < 40);
          }}
          className="console-text h-[min(62vh,640px)] min-h-[320px] overflow-y-auto bg-black/40 px-4 py-3 text-zinc-200"
        >
          {lines.length === 0 ? <p className="text-muted">Noch keine Ausgabe. Starte den Server, um die Konsole zu sehen.</p> : <Fragment>{rendered}</Fragment>}
        </div>
        {!follow && (
          <button
            onClick={() => setFollow(true)}
            className="absolute bottom-3 right-5 inline-flex items-center gap-1.5 rounded-full border border-white/[0.12] bg-raised px-3 py-1.5 text-xs shadow-lg transition hover:bg-white/[0.08]"
          >
            <ArrowDownToLine className="h-3.5 w-3.5" />
            Nach unten
          </button>
        )}
      </div>

      <div className="flex items-center gap-2 border-t border-white/[0.06] p-3">
        <span className="font-mono text-sm text-muted">&gt;</span>
        <input
          className="h-9 flex-1 bg-transparent font-mono text-sm outline-none placeholder:text-muted/60 disabled:opacity-50"
          placeholder={!canCommand ? "Nur lesen – Befehle brauchen die Rolle Operator" : running ? "Befehl eingeben und Enter drücken" : "Der Server läuft nicht"}
          value={input}
          disabled={!canCommand || !running}
          onChange={(event) => setInput(event.target.value)}
          onKeyDown={onKey}
          spellCheck={false}
          autoComplete="off"
        />
        <Button size="sm" variant="primary" onClick={send} disabled={!canCommand || !running || !input.trim()}>
          <Send className="h-3.5 w-3.5" />
          Senden
        </Button>
      </div>
      {error && <p className="border-t border-bad/25 bg-bad/10 px-4 py-2 text-xs text-bad">{error}</p>}
    </Card>
  );
}
