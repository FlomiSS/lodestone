"use client";

import Link from "next/link";
import { Activity, ArrowRight, ArrowUpCircle, Cpu, FolderKanban, Package, ScrollText, Server, ShieldCheck, Users } from "lucide-react";
import { AdminSidebar } from "@/components/admin-sidebar";
import { useUser } from "@/components/layout/shell";
import { ErrorState, Loading } from "@/components/ui/async";
import { Badge, Card, CardHeader, EmptyState, PageHeader, StatCard } from "@/components/ui/primitives";
import { useApi } from "@/lib/api";
import { auditLabel } from "@/lib/audit";
import { dateTime } from "@/lib/format";
import type { AdminOverview, AuditEntry, UpdateStatus } from "@/lib/types";

export default function AdminOverviewPage() {
  const user = useUser();
  const { data: overview, error: overviewError, mutate: mutateOverview } = useApi<AdminOverview>(
    user.isAdmin ? "/admin/overview" : null,
    { refreshInterval: 15000 },
  );
  const { data: audit, error: auditError, mutate: mutateAudit } = useApi<AuditEntry[]>(
    user.isAdmin ? "/audit?limit=8" : null,
    { refreshInterval: 15000 },
  );

  const { data: updates } = useApi<UpdateStatus>(user.isAdmin ? "/updates" : null, { refreshInterval: 60000 });

  if (!user.isAdmin) {
    return <ErrorState error={new Error("Dafür braucht es einen Administrator.")} />;
  }

  if (overviewError) {
    return (
      <>
        <AdminSidebar />
        <ErrorState error={overviewError} retry={() => mutateOverview()} />
      </>
    );
  }

  if (!overview) {
    return (
      <>
        <AdminSidebar />
        <Loading />
      </>
    );
  }

  const quickLinks = [
    {
      href: "/users/",
      title: "Benutzerverwaltung",
      description: "Benutzerkonten anlegen, Kennwörter setzen, sperren und Rollen prüfen",
      icon: Users,
    },
    {
      href: "/admin/projects/",
      title: "Projekte",
      description: "Alle Projekte im System mit Eigentümern, Mitgliedern und Servern einsehen",
      icon: FolderKanban,
    },
    {
      href: "/nodes/",
      title: "Nodes",
      description: "Host-Maschinen verwalten, Speicherauslastung prüfen und playit verbinden",
      icon: Cpu,
    },
    {
      href: "/admin/packs/",
      title: "Position-Packs",
      description: "Server-Software-Vorlagen, Autokonfigurationen und Skripte als ZIP verwalten",
      icon: Package,
    },
    {
      href: "/admin/updates/",
      title: "Updates",
      description: updates?.updateAvailable
        ? `Version ${updates.latestVersion} ist verfügbar`
        : "Versionsstand von Panel und Nodes prüfen und Updates einspielen",
      icon: ArrowUpCircle,
    },
    {
      href: "/audit/",
      title: "Protokoll",
      description: "Audit-Log aller administrativen Aktionen und Sicherheitsereignisse durchsuchen",
      icon: ScrollText,
    },
  ];

  return (
    <>
      <AdminSidebar />
      <PageHeader
        title="Admin-Panel"
        description="Zentrale Übersicht über das System, Kennzahlen und letzte Aktivitäten."
      />

      {updates?.updateAvailable && (
        <Link
          href="/admin/updates/"
          className="mb-4 flex items-center justify-between gap-3 rounded-lg border border-warn/25 bg-warn/10 px-3 py-2 text-xs text-warn transition duration-100 hover:bg-warn/20"
        >
          <span>Eine neue Lodestone-Version ({updates.latestVersion}) ist verfügbar.</span>
          <span className="inline-flex items-center gap-1 font-medium">
            Zu den Updates
            <ArrowRight className="h-3.5 w-3.5" />
          </span>
        </Link>
      )}

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        <StatCard
          label="Benutzer"
          value={overview.users}
          hint={overview.disabledUsers > 0 ? `${overview.disabledUsers} gesperrt` : "registrierte Konten"}
          icon={<Users className="h-4 w-4" />}
          tone="blue"
        />
        <StatCard
          label="Administratoren"
          value={overview.admins}
          hint="mit vollem Zugriff"
          icon={<ShieldCheck className="h-4 w-4" />}
          tone="violet"
        />
        <StatCard
          label="Projekte"
          value={overview.projects}
          hint={`${overview.members} Mitgliedschaften`}
          icon={<FolderKanban className="h-4 w-4" />}
          tone="green"
        />
        <StatCard
          label="Server"
          value={overview.servers}
          hint="in allen Projekten"
          icon={<Server className="h-4 w-4" />}
          tone="amber"
        />
        <StatCard
          label="Nodes"
          value={overview.nodes}
          hint="Host-Maschinen"
          icon={<Cpu className="h-4 w-4" />}
          tone="neutral"
        />
        <StatCard
          label="Aktive Sitzungen"
          value={overview.sessions}
          hint="angemeldete Browser"
          icon={<Activity className="h-4 w-4" />}
          tone="neutral"
        />
      </div>

      <div className="mt-8 grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader
            title="Letzte Aktivität"
            description="Die neuesten Aktionen im System"
            action={
              <Link href="/audit/" className="text-xs text-accent hover:underline">
                Alle anzeigen
              </Link>
            }
          />
          {auditError ? (
            <div className="p-5">
              <ErrorState error={auditError} retry={() => mutateAudit()} />
            </div>
          ) : !audit ? (
            <div className="py-8">
              <Loading />
            </div>
          ) : audit.length === 0 ? (
            <EmptyState
              icon={<ScrollText className="h-6 w-6" />}
              title="Keine Einträge"
              text="Bisher wurden keine Aktionen protokolliert."
            />
          ) : (
            <div className="divide-y divide-white/[0.05]">
              {audit.map((entry) => (
                <div key={entry.id} className="flex flex-wrap items-center gap-x-4 gap-y-1 px-5 py-2.5 text-sm">
                  <span className="w-28 shrink-0 text-xs tabular-nums text-muted">{dateTime(entry.time)}</span>
                  <span className="w-24 shrink-0 truncate font-medium">{entry.username || "–"}</span>
                  <span className="min-w-[140px] flex-1 truncate">
                    {auditLabel(entry.action, entry.target)}
                    {entry.target && <span className="ml-1.5 font-mono text-[11px] text-muted">{entry.target}</span>}
                  </span>
                  {entry.status > 0 && (
                    <Badge tone={entry.status < 300 ? "good" : entry.status < 500 ? "warn" : "bad"}>
                      {entry.status}
                    </Badge>
                  )}
                </div>
              ))}
            </div>
          )}
        </Card>

        <Card>
          <CardHeader
            title="Schnellzugriff"
            description="Direkte Links zu den Verwaltungsbereichen"
          />
          <div className="divide-y divide-white/[0.05]">
            {quickLinks.map((item) => (
              <Link
                key={item.href}
                href={item.href}
                className="flex items-center justify-between gap-4 px-5 py-3.5 transition duration-100 hover:bg-white/[0.03]"
              >
                <div className="flex items-center gap-3">
                  <span className="grid h-8 w-8 place-items-center rounded-lg border border-white/[0.08] bg-white/[0.04] text-muted">
                    <item.icon className="h-4 w-4" />
                  </span>
                  <div>
                    <p className="text-sm font-medium text-ink">{item.title}</p>
                    <p className="text-xs text-muted">{item.description}</p>
                  </div>
                </div>
                <ArrowRight className="h-4 w-4 text-muted" />
              </Link>
            ))}
          </div>
        </Card>
      </div>
    </>
  );
}
