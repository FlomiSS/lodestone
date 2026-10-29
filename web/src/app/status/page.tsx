"use client";

import { Check, Copy } from "lucide-react";
import { useSearchParams } from "next/navigation";
import { Suspense, useEffect, useState } from "react";
import { LodestoneLogo } from "@/components/ui/mc";
import { Dot } from "@/components/ui/primitives";
import { api } from "@/lib/api";
import type { PublicStatus } from "@/lib/types";

/** Öffentliche Statusseite eines Projekts, ohne Anmeldung. Aufruf: /status/?p=<Kürzel>, mit &embed=1 ohne Rahmen. */
export default function StatusPage() {
  return (
    <Suspense fallback={null}>
      <StatusView />
    </Suspense>
  );
}

function StatusView() {
  const params = useSearchParams();
  const slug = params.get("p");
  const embed = params.get("embed") === "1";
  const [status, setStatus] = useState<PublicStatus | null>(null);
  const [missing, setMissing] = useState(false);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!slug) {
      setMissing(true);
      return;
    }
    let stop = false;
    const load = () =>
      api
        .get<PublicStatus>(`/public/status/${encodeURIComponent(slug)}`)
        .then((data) => {
          if (!stop) {
            setStatus(data);
            setMissing(false);
          }
        })
        .catch(() => !stop && setStatus((current) => (current ? current : (setMissing(true), null))));
    load();
    const timer = setInterval(load, 20000);
    return () => {
      stop = true;
      clearInterval(timer);
    };
  }, [slug]);

  async function copy() {
    if (!status?.address) return;
    await navigator.clipboard.writeText(status.address).catch(() => null);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }

  return (
    <main className={embed ? "p-3" : "grid min-h-screen place-items-center px-4 py-10"}>
      <div className="w-full max-w-md">
        {missing && !status ? (
          <div className="rounded-xl border border-white/[0.07] bg-surface p-8 text-center">
            <p className="text-sm text-muted">404</p>
            <h1 className="mt-1 text-xl font-semibold">Diese Statusseite gibt es nicht</h1>
            <p className="mt-2 text-sm text-muted">Sie ist nicht veröffentlicht oder der Link stimmt nicht.</p>
          </div>
        ) : !status ? (
          <p className="text-center text-sm text-muted">Wird geladen</p>
        ) : (
          <div className="rounded-xl border border-white/[0.07] bg-surface">
            <div className="flex items-center gap-3 border-b border-white/[0.06] p-5">
              <LodestoneLogo size={32} />
              <div className="min-w-0 flex-1">
                <h1 className="truncate text-lg font-semibold tracking-tight">{status.name}</h1>
                {status.description && <p className="truncate text-xs text-muted">{status.description}</p>}
              </div>
              <span className={`inline-flex items-center gap-2 rounded-full border px-3 py-1 text-xs ${status.online ? "border-good/30 bg-good/10 text-good" : "border-bad/30 bg-bad/10 text-bad"}`}>
                <Dot tone={status.online ? "good" : "bad"} />
                {status.online ? "Online" : "Offline"}
              </span>
            </div>

            <div className="grid grid-cols-2 gap-px bg-white/[0.05]">
              <div className="bg-surface p-5">
                <p className="text-xs text-muted">Spieler</p>
                <p className="mt-1 text-2xl font-semibold tabular-nums">
                  {status.online ? status.players : "–"}
                  {status.online && status.maxPlayers ? <span className="text-base font-normal text-muted"> / {status.maxPlayers}</span> : null}
                </p>
              </div>
              <div className="bg-surface p-5">
                <p className="text-xs text-muted">Version</p>
                <p className="mt-1 truncate text-2xl font-semibold">{status.version || "–"}</p>
              </div>
            </div>

            {status.address && (
              <div className="flex items-center gap-2 border-t border-white/[0.06] p-4">
                <code className="min-w-0 flex-1 truncate rounded-md bg-black/30 px-3 py-2 font-mono text-sm">{status.address}</code>
                <button onClick={copy} className="inline-flex items-center gap-1.5 rounded-lg border border-white/[0.1] px-3 py-2 text-xs text-muted transition hover:bg-white/[0.06] hover:text-ink">
                  {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
                  {copied ? "Kopiert" : "Kopieren"}
                </button>
              </div>
            )}

            {status.servers.length > 1 && (
              <ul className="divide-y divide-white/[0.05] border-t border-white/[0.06]">
                {status.servers.map((server) => (
                  <li key={server.name} className="flex items-center gap-3 px-5 py-2.5 text-sm">
                    <Dot tone={server.online ? "good" : "bad"} />
                    <span className="min-w-0 flex-1 truncate">{server.name}</span>
                    <span className="text-xs tabular-nums text-muted">{server.online ? `${server.players}${server.maxPlayers ? ` / ${server.maxPlayers}` : ""} Spieler` : "offline"}</span>
                  </li>
                ))}
              </ul>
            )}
          </div>
        )}
        {!embed && status && <p className="mt-4 text-center text-[11px] text-muted/70">Angetrieben von Lodestone</p>}
      </div>
    </main>
  );
}
