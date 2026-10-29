"use client";

import { ScrollText } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { Badge, Button, Card, EmptyState, PageHeader, SearchInput } from "@/components/ui/primitives";
import { ErrorState, Loading } from "@/components/ui/async";
import { useUser } from "@/components/layout/shell";
import { api } from "@/lib/api";
import { auditLabel } from "@/lib/audit";
import { dateTime } from "@/lib/format";
import type { AuditEntry } from "@/lib/types";

const PAGE = 100;

export default function AuditPage() {
  const user = useUser();
  const [entries, setEntries] = useState<AuditEntry[] | null>(null);
  const [filter, setFilter] = useState("");
  const [applied, setApplied] = useState("");
  const [more, setMore] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const load = useCallback(async (append: boolean, name: string, before = 0) => {
    setBusy(true);
    try {
      const query = new URLSearchParams({ limit: String(PAGE) });
      if (name) query.set("user", name);
      if (before) query.set("before", String(before));
      const page = await api.get<AuditEntry[]>(`/audit?${query}`);
      setEntries((current) => (append ? [...(current ?? []), ...page] : page));
      setMore(page.length === PAGE);
      setError(null);
    } catch (failure) {
      setError(failure instanceof Error ? failure : new Error("Fehler"));
    } finally {
      setBusy(false);
    }
  }, []);

  useEffect(() => {
    if (user.isAdmin) load(false, applied);
  }, [user.isAdmin, applied, load]);

  // Filter nach kurzer Pause anwenden
  useEffect(() => {
    const timer = setTimeout(() => setApplied(filter.trim()), 400);
    return () => clearTimeout(timer);
  }, [filter]);

  if (!user.isAdmin) return <ErrorState error={new Error("Dafür braucht es einen Administrator.")} />;
  if (error && !entries) return <ErrorState error={error} retry={() => load(false, applied)} />;
  if (!entries) return <Loading />;

  return (
    <>
      <PageHeader title="Protokoll" description="Wer hat wann was geändert. Einträge werden 90 Tage aufbewahrt." />
      <SearchInput value={filter} onChange={(event) => setFilter(event.target.value)} placeholder="Nach Benutzername filtern" className="mb-5 max-w-sm" />
      <Card>
        {entries.length === 0 ? (
          <EmptyState icon={<ScrollText className="h-6 w-6" />} title="Keine Einträge" />
        ) : (
          <div className="divide-y divide-white/[0.05]">
            {entries.map((entry) => (
              <div key={entry.id} className="flex flex-wrap items-center gap-x-5 gap-y-1 px-5 py-2.5 text-sm">
                <span className="w-32 shrink-0 text-xs tabular-nums text-muted">{dateTime(entry.time)}</span>
                <span className="w-28 shrink-0 truncate font-medium">{entry.username || "–"}</span>
                <span className="min-w-[200px] flex-1">
                  {auditLabel(entry.action, entry.target)}
                  {entry.target && <span className="ml-2 break-all font-mono text-[11px] text-muted">{entry.target}</span>}
                </span>
                {entry.status > 0 && <Badge tone={entry.status < 300 ? "good" : entry.status < 500 ? "warn" : "bad"}>{entry.status}</Badge>}
                <span className="w-28 shrink-0 text-right font-mono text-[11px] text-muted">{entry.ip}</span>
              </div>
            ))}
          </div>
        )}
      </Card>
      {more && (
        <div className="mt-4 flex justify-center">
          <Button loading={busy} onClick={() => load(true, applied, entries[entries.length - 1].id)}>
            Ältere laden
          </Button>
        </div>
      )}
    </>
  );
}
