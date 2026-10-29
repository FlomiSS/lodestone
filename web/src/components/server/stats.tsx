"use client";

import { Cpu, MemoryStick, Timer, Users } from "lucide-react";
import { useState } from "react";
import { Card, CardHeader, StatCard, Tabs } from "@/components/ui/primitives";
import { SeriesChart } from "@/components/ui/chart";
import { ErrorState, Loading } from "@/components/ui/async";
import { useApi } from "@/lib/api";
import { megabytes, uptime } from "@/lib/format";
import type { Metrics, Range, Server } from "@/lib/types";

export const ranges: { id: Range; label: string }[] = [
  { id: "1h", label: "1 Std" },
  { id: "6h", label: "6 Std" },
  { id: "24h", label: "24 Std" },
  { id: "7d", label: "7 Tage" },
];

const MB = 1024 * 1024;

export function RangeTabs({ value, onChange }: { value: Range; onChange: (range: Range) => void }) {
  return <Tabs value={value} onChange={onChange} items={ranges} />;
}

/** Ein Diagramm mit Titel und Zusammenfassung; fehlen Messwerte, steht ein Hinweis statt einer leeren Fläche. */
export function ChartCard({
  title,
  description,
  empty,
  children,
  action,
}: {
  title: string;
  description?: string;
  empty: boolean;
  children: React.ReactNode;
  action?: React.ReactNode;
}) {
  return (
    <Card>
      <CardHeader title={title} description={description} action={action} />
      <div className="px-3 pb-3 pt-1">
        {empty ? <p className="px-4 py-14 text-center text-sm text-muted">Für diesen Zeitraum gibt es noch keine Messwerte. Sie werden alle 30 Sekunden erfasst, solange der Server läuft.</p> : children}
      </div>
    </Card>
  );
}

export function StatsTab({ server }: { server: Server }) {
  const [range, setRange] = useState<Range>("6h");
  const { data, error, mutate } = useApi<Metrics>(`/servers/${server.id}/metrics?range=${range}`, { refreshInterval: 30000 });
  const view = server.view;
  const stats = view?.stats;
  const running = view?.state === "running";

  if (error) return <ErrorState error={error} retry={() => mutate()} />;
  if (!data) return <Loading />;

  const series = data.series;
  const long = range === "24h" || range === "7d";
  const limitMB = view?.memoryMB ?? 0;
  const empty = series.samples === 0;
  const memoryMB = series.memory.map((value) => (value === null ? null : value / MB));
  const memoryPercent = stats && stats.memoryLimit > 0 ? (stats.memoryBytes / stats.memoryLimit) * 100 : 0;

  return (
    <div className="space-y-5">
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard label="Laufzeit" value={running ? uptime(view?.startedAt) : "–"} icon={<Timer className="h-4 w-4" />} tone={running ? "green" : "neutral"} hint={running ? "seit dem letzten Start" : "Der Server läuft nicht"} />
        <StatCard
          label="CPU"
          value={running && stats ? `${stats.cpuPercent.toFixed(0)} %` : "–"}
          icon={<Cpu className="h-4 w-4" />}
          tone="blue"
          hint={series.samples ? `Ø ${series.avgCpu.toFixed(0)} % · Spitze ${series.peakCpu.toFixed(0)} %` : undefined}
        />
        <StatCard
          label="Arbeitsspeicher"
          value={running && stats ? `${Math.round(stats.memoryBytes / MB)} MB` : "–"}
          icon={<MemoryStick className="h-4 w-4" />}
          tone={memoryPercent >= 90 ? "red" : memoryPercent >= 75 ? "amber" : "violet"}
          hint={limitMB ? `von ${megabytes(limitMB)}${running && stats ? ` (${memoryPercent.toFixed(0)} %)` : ""}` : undefined}
        />
        <StatCard
          label="Spieler"
          value={running ? `${data.players}${data.maxPlayers ? ` / ${data.maxPlayers}` : ""}` : "–"}
          icon={<Users className="h-4 w-4" />}
          tone="amber"
          hint={series.samples ? `Spitze ${series.peakPlayers} · Ø ${series.avgPlayers.toFixed(1).replace(".", ",")}` : undefined}
        />
      </div>

      <div className="flex justify-end">
        <RangeTabs value={range} onChange={setRange} />
      </div>

      <ChartCard title="Spieler" description="Gleichzeitig online" empty={empty}>
        <SeriesChart times={series.times} series={[{ name: "Spieler", color: "#f59e0b", values: series.players }]} long={long} format={(value) => String(Math.round(value))} />
      </ChartCard>
      <div className="grid gap-5 xl:grid-cols-2">
        <ChartCard title="CPU" description="Auslastung des Containers" empty={empty}>
          <SeriesChart times={series.times} series={[{ name: "CPU", color: "#3b82f6", values: series.cpu }]} long={long} format={(value) => `${Math.round(value)} %`} />
        </ChartCard>
        <ChartCard title="Arbeitsspeicher" description={limitMB ? `Limit ${megabytes(limitMB)}` : undefined} empty={empty}>
          <SeriesChart
            times={series.times}
            series={[{ name: "Speicher", color: "#8b5cf6", values: memoryMB }]}
            yMax={limitMB || undefined}
            long={long}
            format={(value) => `${Math.round(value)} MB`}
          />
        </ChartCard>
      </div>
    </div>
  );
}
