"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Archive, BarChart3, ChevronLeft, FolderOpen, Puzzle, Settings, Terminal, Users, type LucideIcon } from "lucide-react";
import { Suspense, useState } from "react";
import { Badge, Meter, cx, loadTone } from "@/components/ui/primitives";
import { ErrorState, Loading } from "@/components/ui/async";
import { PowerButtons, StateBadge } from "@/components/status";
import { SecondarySidebar } from "@/components/layout/shell";
import { AddonsTab } from "@/components/server/addons";
import { BackupsTab } from "@/components/server/backups";
import { ConsoleTab } from "@/components/server/console";
import { FilesTab } from "@/components/server/files";
import { ModpackBanner } from "@/components/server/modpack";
import { PlayersTab } from "@/components/server/players";
import { SettingsTab } from "@/components/server/settings";
import { StatsTab } from "@/components/server/stats";
import { useApi } from "@/lib/api";
import { megabytes } from "@/lib/format";
import { roleRank, type Project, type Role, type Server } from "@/lib/types";

export default function ServerPage() {
  return (
    <Suspense fallback={<Loading />}>
      <ServerView />
    </Suspense>
  );
}

type Tab = "console" | "stats" | "files" | "addons" | "players" | "backups" | "settings";

interface Entry {
  id: Tab;
  label: string;
  icon: LucideIcon;
}

function ServerView() {
  const router = useRouter();
  const params = useSearchParams();
  const id = params.get("id");
  const { data, error, mutate } = useApi<{ server: Server; myRole: Role }>(id ? `/servers/${id}` : null, { refreshInterval: 3000 });
  const { data: project } = useApi<Project>(data ? `/projects/${data.server.projectId}` : null);
  const [tab, setTabState] = useState<Tab>((params.get("tab") as Tab) || "console");

  function setTab(next: Tab) {
    setTabState(next);
    // Der Bereich steht in der Adresse, damit sich ein Link oder Neuladen merkt, wo man war
    const url = new URL(window.location.href);
    url.searchParams.set("tab", next);
    window.history.replaceState(null, "", url);
  }

  if (!id) return <ErrorState error={new Error("Kein Server angegeben")} />;
  if (error) return <ErrorState error={error} retry={() => mutate()} />;
  if (!data) return <Loading />;

  const { server, myRole } = data;
  const rank = roleRank[myRole];
  const view = server.view;
  const state = view?.state;
  const extensions = server.software === "fabric" || server.software === "neoforge" ? "Mods" : server.software === "vanilla" ? null : "Plugins";

  const entries: Entry[] = [
    { id: "console", label: "Konsole", icon: Terminal },
    { id: "stats", label: "Statistiken", icon: BarChart3 },
    ...(rank >= roleRank.operator ? [{ id: "files" as const, label: "Dateien", icon: FolderOpen }] : []),
    ...(extensions ? [{ id: "addons" as const, label: extensions, icon: Puzzle }] : []),
    { id: "players", label: "Spieler", icon: Users },
    { id: "backups", label: "Backups", icon: Archive },
    ...(rank >= roleRank.admin ? [{ id: "settings" as const, label: "Einstellungen", icon: Settings }] : []),
  ];
  const active = entries.some((entry) => entry.id === tab) ? tab : "console";
  const stats = view?.stats;
  const memoryPercent = stats && stats.memoryLimit > 0 ? (stats.memoryBytes / stats.memoryLimit) * 100 : 0;

  return (
    <>
      <SecondarySidebar>
        <div className="flex flex-col gap-4 p-4 lg:min-h-full lg:p-5">
          <Link href={`/project/?id=${server.projectId}`} className="inline-flex items-center gap-1 text-xs text-muted transition hover:text-ink">
            <ChevronLeft className="h-3.5 w-3.5" />
            {project?.name ?? "Projekt"}
          </Link>

          <div>
            <p className="truncate text-[15px] font-semibold tracking-tight">{server.name}</p>
            <p className="mt-0.5 truncate text-xs text-muted">
              {server.software} {view?.version}
            </p>
            <div className="mt-2.5 flex flex-wrap items-center gap-1.5">
              <StateBadge server={server} />
              {server.role === "proxy" && <Badge tone="accent">Proxy</Badge>}
              {server.role === "backend" && <Badge>Backend</Badge>}
            </div>
          </div>

          <nav className="flex gap-1 overflow-x-auto lg:flex-col lg:overflow-visible">
            {entries.map((entry) => (
              <button
                key={entry.id}
                onClick={() => setTab(entry.id)}
                className={cx(
                  "flex shrink-0 items-center gap-3 whitespace-nowrap rounded-lg border px-3 py-2 text-left text-[13px] transition duration-100 active:scale-[0.97]",
                  active === entry.id ? "border-white/[0.07] bg-white/[0.07] text-ink" : "border-transparent text-muted hover:bg-white/[0.04] hover:text-ink",
                )}
              >
                <entry.icon className={cx("h-4 w-4 shrink-0", active === entry.id ? "text-ink" : "text-muted")} />
                {entry.label}
              </button>
            ))}
          </nav>

          {state === "running" && stats && (
            <div className="mt-auto hidden space-y-3 rounded-lg border border-white/[0.06] bg-white/[0.02] p-3 lg:block">
              <div>
                <div className="mb-1.5 flex justify-between text-[11px] text-muted">
                  <span>CPU</span>
                  <span className="tabular-nums text-ink/80">{stats.cpuPercent.toFixed(0)} %</span>
                </div>
                <Meter value={stats.cpuPercent} max={100} tone={loadTone(stats.cpuPercent)} />
              </div>
              <div>
                <div className="mb-1.5 flex justify-between text-[11px] text-muted">
                  <span>Speicher</span>
                  <span className="tabular-nums text-ink/80">{Math.round(stats.memoryBytes / 1024 ** 2)} MB</span>
                </div>
                <Meter value={memoryPercent} max={100} tone={loadTone(memoryPercent)} />
              </div>
            </div>
          )}
        </div>
      </SecondarySidebar>

      <div className="mb-6 flex flex-wrap items-center justify-between gap-4">
        <div className="min-w-0">
          <h1 className="truncate text-xl font-semibold tracking-tight">{entries.find((entry) => entry.id === active)?.label}</h1>
          <p className="mt-1 text-[13px] text-muted">
            {server.name} · {view ? megabytes(view.memoryMB) : "–"} · Node {server.nodeName}
            {server.address && (
              <>
                {" · "}
                <span className="font-mono text-ink/80">{server.address}</span>
              </>
            )}
          </p>
        </div>
        <PowerButtons server={server} onDone={() => mutate()} size="md" enabled={rank >= roleRank.operator} />
      </div>

      <ModpackBanner server={server} />
      {view?.lastError && state === "offline" && <p className="mb-5 rounded-lg border border-bad/25 bg-bad/10 px-4 py-3 text-xs text-bad">{view.lastError}</p>}
      {state === "installing" && <p className="mb-5 rounded-lg border border-info/25 bg-info/10 px-4 py-3 text-xs text-info">Die Server-Datei wird heruntergeladen. Das dauert einen Moment.</p>}

      {active === "console" && <ConsoleTab serverId={server.id} canCommand={rank >= roleRank.operator} state={state} />}
      {active === "stats" && <StatsTab server={server} />}
      {active === "files" && <FilesTab serverId={server.id} canWrite={rank >= roleRank.operator} />}
      {active === "addons" && extensions && <AddonsTab serverId={server.id} canWrite={rank >= roleRank.operator} noun={extensions as "Plugins" | "Mods"} running={state === "running" || state === "starting"} />}
      {active === "players" && <PlayersTab server={server} canOperate={rank >= roleRank.operator} />}
      {active === "backups" && <BackupsTab serverId={server.id} canWrite={rank >= roleRank.operator} offline={state === "offline"} />}
      {active === "settings" && <SettingsTab server={server} onChanged={() => mutate()} onDeleted={() => router.replace(`/project/?id=${server.projectId}`)} />}
    </>
  );
}
