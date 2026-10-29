"use client";

import { CalendarClock, Pencil, Play, Plus, Trash2, X } from "lucide-react";
import { useState } from "react";
import { Badge, Button, Card, EmptyState, Switch, cx } from "@/components/ui/primitives";
import { ConfirmDialog, Dialog, useToast } from "@/components/ui/feedback";
import { api, useApi } from "@/lib/api";
import { ago, dateTime } from "@/lib/format";
import { roleRank, type AuthStatus, type Project, type Schedule, type ScheduleStep, type StepAction } from "@/lib/types";

const actionLabel: Record<StepAction, string> = {
  backup: "Backup erstellen",
  command: "Befehl senden",
  restart: "Neu starten",
  stop: "Stoppen",
  start: "Starten",
  wait: "Warten (Sekunden)",
};

const days = ["Mo", "Di", "Mi", "Do", "Fr", "Sa", "So"];

function clock(minute: number): string {
  return `${String(Math.floor(minute / 60)).padStart(2, "0")}:${String(minute % 60).padStart(2, "0")}`;
}

export function scheduleText(schedule: Pick<Schedule, "kind" | "everyMinutes" | "atMinute" | "weekdays">): string {
  if (schedule.kind === "interval") {
    const m = schedule.everyMinutes;
    return m % 1440 === 0 ? `alle ${m / 1440} Tage` : m % 60 === 0 ? `alle ${m / 60} Std` : `alle ${m} Min`;
  }
  if (schedule.kind === "daily" || schedule.weekdays === 127) return `täglich um ${clock(schedule.atMinute)}`;
  const picked = days.filter((_, index) => schedule.weekdays & (1 << index));
  return `${picked.join(", ")} um ${clock(schedule.atMinute)}`;
}

function stepText(step: ScheduleStep): string {
  if (step.action === "command") return `Befehl: ${step.value}`;
  if (step.action === "wait") return `${step.value} s warten`;
  if (step.action === "backup" && step.value) return `Backup „${step.value}“`;
  return actionLabel[step.action];
}

const presets: { label: string; build: () => Partial<Draft> }[] = [
  { label: "Täglicher Neustart um 04:00", build: () => ({ name: "Täglicher Neustart", kind: "daily", atMinute: 240, warnMinutes: 5, steps: [{ action: "restart" }] }) },
  { label: "Nächtliches Backup um 03:30", build: () => ({ name: "Nächtliches Backup", kind: "daily", atMinute: 210, warnMinutes: 0, steps: [{ action: "backup", value: "nachts" }] }) },
  { label: "Backup, dann Neustart", build: () => ({ name: "Backup und Neustart", kind: "daily", atMinute: 240, warnMinutes: 5, steps: [{ action: "backup", value: "vor-neustart" }, { action: "restart" }] }) },
  { label: "Ansage alle 30 Minuten", build: () => ({ name: "Ansage", kind: "interval", everyMinutes: 30, warnMinutes: 0, steps: [{ action: "command", value: "say Besuche unsere Website!" }] }) },
];

interface Draft {
  serverId: string;
  name: string;
  kind: "interval" | "daily" | "weekly";
  everyMinutes: number;
  atMinute: number;
  weekdays: number;
  warnMinutes: number;
  steps: ScheduleStep[];
}

const emptyDraft = (serverId: string): Draft => ({ serverId, name: "", kind: "daily", everyMinutes: 60, atMinute: 240, weekdays: 127, warnMinutes: 0, steps: [{ action: "backup" }] });

export function SchedulesTab({ project }: { project: Project }) {
  const toast = useToast();
  const canManage = roleRank[project.myRole] >= roleRank.admin;
  const canRun = roleRank[project.myRole] >= roleRank.operator;
  const { data, mutate } = useApi<Schedule[]>(`/projects/${project.id}/schedules`, { refreshInterval: 15000 });
  const [editing, setEditing] = useState<Schedule | "new" | null>(null);
  const [removing, setRemoving] = useState<Schedule | null>(null);
  const { data: status } = useApi<AuthStatus & { timezone?: string }>("/auth/status");

  return (
    <div className="space-y-5">
      <Card className="flex flex-wrap items-center justify-between gap-4 p-5">
        <div className="max-w-2xl">
          <p className="text-sm font-semibold">Zeitpläne</p>
          <p className="mt-1 text-xs leading-relaxed text-muted">
            Backups, Neustarts und Befehle zu festen Zeiten. Ein Zeitplan kann mehrere Schritte nacheinander ausführen (z. B. erst sichern, dann neu starten); schlägt einer fehl,
            stoppt der Rest und du bekommst eine Meldung. Zeiten gelten in der Zeitzone <b className="text-ink/80">{status?.timezone ?? "des Servers"}</b>.
          </p>
        </div>
        {canManage && (
          <Button variant="primary" onClick={() => setEditing("new")} disabled={project.servers.length === 0}>
            <Plus className="h-4 w-4" />
            Zeitplan anlegen
          </Button>
        )}
      </Card>

      {!data ? null : data.length === 0 ? (
        <Card>
          <EmptyState icon={<CalendarClock className="h-6 w-6" />} title="Noch kein Zeitplan" text={project.servers.length ? "Lege z. B. ein nächtliches Backup oder einen täglichen Neustart an." : "Lege zuerst einen Server an."} />
        </Card>
      ) : (
        data.map((schedule) => (
          <Card key={schedule.id} className={cx(!schedule.enabled && "opacity-60")}>
            <div className="flex flex-wrap items-start justify-between gap-4 px-5 py-4">
              <div className="min-w-0">
                <p className="truncate text-sm font-semibold">{schedule.name}</p>
                <p className="mt-0.5 text-xs text-muted">
                  {schedule.serverName || "Server gelöscht"} · {scheduleText(schedule)}
                  {schedule.warnMinutes > 0 && ` · Vorwarnung ${schedule.warnMinutes} Min`}
                </p>
                <div className="mt-2.5 flex flex-wrap items-center gap-1.5">
                  {schedule.steps.map((step, index) => (
                    <span key={index} className="inline-flex items-center gap-1.5 rounded-md border border-white/[0.07] bg-white/[0.03] px-2 py-1 text-[11px]">
                      <span className="text-muted">{index + 1}</span>
                      {stepText(step)}
                    </span>
                  ))}
                </div>
              </div>
              <div className="flex items-center gap-2">
                {canManage && (
                  <Switch
                    checked={schedule.enabled}
                    onChange={async (value) => {
                      await toast.run(() => api.patch(`/schedules/${schedule.id}`, { enabled: value }));
                      mutate();
                    }}
                  />
                )}
                {canRun && (
                  <Button
                    size="sm"
                    onClick={async () => {
                      await toast.run(() => api.post(`/schedules/${schedule.id}/run`), "Zeitplan läuft");
                      setTimeout(() => mutate(), 3000);
                    }}
                  >
                    <Play className="h-3.5 w-3.5" />
                    Jetzt
                  </Button>
                )}
                {canManage && (
                  <>
                    <Button variant="ghost" size="icon-sm" onClick={() => setEditing(schedule)} aria-label="Bearbeiten">
                      <Pencil className="h-3.5 w-3.5" />
                    </Button>
                    <Button variant="ghost" size="icon-sm" onClick={() => setRemoving(schedule)} aria-label="Löschen">
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  </>
                )}
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-x-5 gap-y-1 border-t border-white/[0.05] px-5 py-2.5 text-xs text-muted">
              <span>{schedule.enabled && schedule.nextRun ? `Nächster Lauf: ${dateTime(schedule.nextRun)}` : "Pausiert"}</span>
              {schedule.lastRun > 0 && (
                <span className="inline-flex items-center gap-2">
                  Zuletzt {ago(schedule.lastRun)}:
                  <Badge tone={schedule.lastStatus === "ok" ? "good" : schedule.lastStatus === "failed" ? "bad" : "warn"}>
                    {schedule.lastStatus === "ok" ? "erfolgreich" : schedule.lastStatus === "failed" ? "fehlgeschlagen" : "übersprungen"}
                  </Badge>
                </span>
              )}
              {schedule.lastStatus === "failed" && <span className="text-bad">{schedule.lastMessage}</span>}
            </div>
          </Card>
        ))
      )}

      <ScheduleDialog project={project} schedule={editing} onClose={() => setEditing(null)} onDone={() => mutate()} />
      <ConfirmDialog
        open={!!removing}
        onClose={() => setRemoving(null)}
        title={`„${removing?.name}“ löschen?`}
        text="Der Zeitplan läuft danach nicht mehr."
        confirm="Löschen"
        danger
        onConfirm={async () => {
          if (!removing) return;
          await toast.run(() => api.delete(`/schedules/${removing.id}`), "Gelöscht");
          mutate();
        }}
      />
    </div>
  );
}

function ScheduleDialog({ project, schedule, onClose, onDone }: { project: Project; schedule: Schedule | "new" | null; onClose: () => void; onDone: () => void }) {
  const toast = useToast();
  const existing = schedule && schedule !== "new" ? schedule : null;
  const open = schedule !== null;
  const [draft, setDraft] = useState<Draft>(emptyDraft(project.servers[0]?.id ?? ""));
  const [seen, setSeen] = useState<Schedule | "new" | null>(null);
  const [busy, setBusy] = useState(false);

  if (schedule !== seen) {
    setSeen(schedule);
    setDraft(existing ? { serverId: existing.serverId, name: existing.name, kind: existing.kind, everyMinutes: existing.everyMinutes || 60, atMinute: existing.atMinute, weekdays: existing.weekdays || 127, warnMinutes: existing.warnMinutes, steps: existing.steps } : emptyDraft(project.servers[0]?.id ?? ""));
  }

  const patch = (change: Partial<Draft>) => setDraft((current) => ({ ...current, ...change }));
  const setStep = (index: number, change: Partial<ScheduleStep>) => patch({ steps: draft.steps.map((step, position) => (position === index ? { ...step, ...change } : step)) });
  const stops = draft.steps.some((step) => step.action === "restart" || step.action === "stop");
  const valid =
    draft.name.trim() &&
    draft.serverId &&
    draft.steps.length > 0 &&
    draft.steps.every((step) => (step.action === "command" ? !!step.value?.trim() : step.action === "wait" ? Number(step.value) >= 1 : true)) &&
    (draft.kind !== "weekly" || draft.weekdays > 0) &&
    (draft.kind !== "interval" || draft.everyMinutes >= 5);

  async function save() {
    setBusy(true);
    const body = { name: draft.name, steps: draft.steps, kind: draft.kind, everyMinutes: draft.kind === "interval" ? draft.everyMinutes : 0, atMinute: draft.atMinute, weekdays: draft.kind === "weekly" ? draft.weekdays : 127, warnMinutes: stops ? draft.warnMinutes : 0 };
    const ok = await toast.run(() => (existing ? api.patch(`/schedules/${existing.id}`, body) : api.post(`/projects/${project.id}/schedules`, { ...body, serverId: draft.serverId })), existing ? "Gespeichert" : "Zeitplan angelegt");
    setBusy(false);
    if (ok) {
      onClose();
      onDone();
    }
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={existing ? `„${existing.name}“ bearbeiten` : "Zeitplan anlegen"}
      wide
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Abbrechen
          </Button>
          <Button variant="primary" loading={busy} disabled={!valid} onClick={save}>
            {existing ? "Speichern" : "Anlegen"}
          </Button>
        </>
      }
    >
      {!existing && (
        <div className="flex flex-wrap gap-2">
          {presets.map((preset) => (
            <button key={preset.label} type="button" onClick={() => patch(preset.build())} className="rounded-full border border-white/[0.09] bg-white/[0.03] px-3 py-1 text-xs text-muted transition hover:border-white/[0.18] hover:text-ink">
              {preset.label}
            </button>
          ))}
        </div>
      )}
      <div className="grid gap-3 sm:grid-cols-2">
        <div>
          <label className="label">Name</label>
          <input className="field" value={draft.name} onChange={(event) => patch({ name: event.target.value })} maxLength={48} placeholder="Nächtliches Backup" />
        </div>
        <div>
          <label className="label">Server</label>
          <select className="field" value={draft.serverId} disabled={!!existing} onChange={(event) => patch({ serverId: event.target.value })}>
            {project.servers.map((server) => (
              <option key={server.id} value={server.id}>
                {server.name}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className="label">Wann</label>
          <select className="field" value={draft.kind} onChange={(event) => patch({ kind: event.target.value as Draft["kind"] })}>
            <option value="daily">Täglich</option>
            <option value="weekly">An bestimmten Wochentagen</option>
            <option value="interval">In festem Abstand</option>
          </select>
        </div>
        {draft.kind === "interval" ? (
          <div>
            <label className="label">Alle … Minuten (mindestens 5)</label>
            <input className="field" inputMode="numeric" value={draft.everyMinutes} onChange={(event) => patch({ everyMinutes: Number(event.target.value.replace(/\D/g, "")) || 0 })} />
          </div>
        ) : (
          <div>
            <label className="label">Uhrzeit</label>
            <input
              className="field"
              type="time"
              value={clock(draft.atMinute)}
              onChange={(event) => {
                const [h, m] = event.target.value.split(":").map(Number);
                if (!Number.isNaN(h)) patch({ atMinute: h * 60 + (m || 0) });
              }}
            />
          </div>
        )}
      </div>
      {draft.kind === "weekly" && (
        <div className="flex flex-wrap gap-1.5">
          {days.map((day, index) => {
            const on = (draft.weekdays & (1 << index)) !== 0;
            return (
              <button key={day} type="button" onClick={() => patch({ weekdays: draft.weekdays ^ (1 << index) })} className={cx("h-8 w-10 rounded-md border text-xs font-medium transition active:scale-95", on ? "border-accent/60 bg-accent/15 text-ink" : "border-white/[0.08] text-muted hover:text-ink")}>
                {day}
              </button>
            );
          })}
        </div>
      )}

      <div>
        <label className="label">Schritte (laufen nacheinander)</label>
        <div className="space-y-2">
          {draft.steps.map((step, index) => (
            <div key={index} className="flex items-center gap-2">
              <span className="w-5 text-center text-xs text-muted">{index + 1}</span>
              <select className="field w-48" value={step.action} onChange={(event) => setStep(index, { action: event.target.value as StepAction, value: "" })}>
                {(Object.keys(actionLabel) as StepAction[]).map((action) => (
                  <option key={action} value={action}>
                    {actionLabel[action]}
                  </option>
                ))}
              </select>
              {(step.action === "command" || step.action === "wait" || step.action === "backup") && (
                <input
                  className="field flex-1"
                  value={step.value ?? ""}
                  onChange={(event) => setStep(index, { value: event.target.value })}
                  placeholder={step.action === "command" ? "say Hallo" : step.action === "wait" ? "30" : "Bezeichnung (optional)"}
                  inputMode={step.action === "wait" ? "numeric" : undefined}
                  maxLength={200}
                />
              )}
              <Button variant="ghost" size="icon-sm" disabled={draft.steps.length === 1} onClick={() => patch({ steps: draft.steps.filter((_, position) => position !== index) })} aria-label="Schritt entfernen">
                <X className="h-3.5 w-3.5" />
              </Button>
            </div>
          ))}
        </div>
        {draft.steps.length < 10 && (
          <Button size="sm" variant="ghost" className="mt-2" onClick={() => patch({ steps: [...draft.steps, { action: "command", value: "" }] })}>
            <Plus className="h-3.5 w-3.5" />
            Schritt hinzufügen
          </Button>
        )}
      </div>

      {stops && (
        <div>
          <label className="label">Vorwarnung im Spiel (Minuten vorher, 0 = keine)</label>
          <input className="field w-32" inputMode="numeric" value={draft.warnMinutes} onChange={(event) => patch({ warnMinutes: Math.min(60, Number(event.target.value.replace(/\D/g, "")) || 0) })} />
          <p className="mt-1 text-xs text-muted">Das Panel schreibt dann zur gewählten Zeit und noch einmal eine Minute vor dem Lauf „Der Server wird in … neu gestartet“ in den Chat.</p>
        </div>
      )}
    </Dialog>
  );
}
