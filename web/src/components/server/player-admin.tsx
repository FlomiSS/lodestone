"use client";

import { Ban, Gavel, MessageSquare, ShieldCheck, ShieldOff, Trash2, UserCheck, UserMinus, UserPlus, Users } from "lucide-react";
import { useState } from "react";
import { Badge, Button, Card, CardHeader, EmptyState, RowMenu, Tabs } from "@/components/ui/primitives";
import { Dialog, useToast } from "@/components/ui/feedback";
import { api, useApi } from "@/lib/api";
import type { PlayerLists, Server } from "@/lib/types";

export type PlayerAction = "kick" | "ban" | "pardon" | "op" | "deop" | "whitelist-add" | "whitelist-remove" | "whitelist-on" | "whitelist-off" | "message" | "ban-ip" | "pardon-ip";

const nameOk = /^\.?[A-Za-z0-9_]{1,16}$/;

/** Führt eine Spieleraktion aus (der Server schickt dazu einen Konsolenbefehl). Gibt zurück, ob es geklappt hat. */
export function usePlayerActions(server: Server, onDone?: () => void) {
  const toast = useToast();
  return async (action: PlayerAction, name = "", extra: { reason?: string; text?: string } = {}, success?: string) => {
    const ok = await toast.run(() => api.post(`/servers/${server.id}/player-action`, { action, name, ...extra }), success ?? "Erledigt");
    if (ok) onDone?.();
    return ok;
  };
}

/** Menü hinter einem Spielernamen: Nachricht, Kicken, Sperren, Operator, Whitelist. */
export function PlayerMenu({ server, name, onDone }: { server: Server; name: string; onDone?: () => void }) {
  const act = usePlayerActions(server, onDone);
  const [dialog, setDialog] = useState<"ban" | "kick" | "message" | null>(null);
  const [text, setText] = useState("");
  return (
    <>
      <RowMenu
        label={`Aktionen für ${name}`}
        items={[
          { label: "Nachricht senden", icon: <MessageSquare className="h-3.5 w-3.5" />, onClick: () => setDialog("message") },
          { label: "Kicken", icon: <UserMinus className="h-3.5 w-3.5" />, onClick: () => setDialog("kick") },
          { label: "Sperren", icon: <Ban className="h-3.5 w-3.5" />, danger: true, onClick: () => setDialog("ban") },
          "separator",
          { label: "Zum Operator machen", icon: <ShieldCheck className="h-3.5 w-3.5" />, onClick: () => act("op", name, {}, `${name} ist jetzt Operator`) },
          { label: "Operator entziehen", icon: <ShieldOff className="h-3.5 w-3.5" />, onClick: () => act("deop", name, {}, `${name} ist kein Operator mehr`) },
          { label: "Auf die Whitelist", icon: <UserCheck className="h-3.5 w-3.5" />, onClick: () => act("whitelist-add", name, {}, `${name} steht auf der Whitelist`) },
        ]}
      />
      <Dialog
        open={dialog !== null}
        onClose={() => setDialog(null)}
        title={dialog === "message" ? `Nachricht an ${name}` : dialog === "ban" ? `${name} sperren` : `${name} kicken`}
        footer={
          <>
            <Button variant="ghost" onClick={() => setDialog(null)}>
              Abbrechen
            </Button>
            <Button
              variant={dialog === "ban" ? "danger" : "primary"}
              disabled={dialog === "message" && !text.trim()}
              onClick={async () => {
                const ok = await act(dialog as PlayerAction, name, dialog === "message" ? { text } : { reason: text }, dialog === "message" ? "Gesendet" : dialog === "ban" ? `${name} ist gesperrt` : `${name} wurde gekickt`);
                if (ok) {
                  setDialog(null);
                  setText("");
                }
              }}
            >
              {dialog === "message" ? "Senden" : dialog === "ban" ? "Sperren" : "Kicken"}
            </Button>
          </>
        }
      >
        <div>
          <label className="label">{dialog === "message" ? "Nachricht" : "Grund (optional)"}</label>
          <input className="field" value={text} onChange={(event) => setText(event.target.value)} maxLength={dialog === "message" ? 200 : 100} autoFocus />
        </div>
      </Dialog>
    </>
  );
}

type List = "whitelist" | "ops" | "banned" | "ips";

/** Whitelist, Operatoren und Sperren des Servers (aus den Dateien des Servers) mit Hinzufügen und Entfernen. */
export function PlayerAdmin({ server }: { server: Server }) {
  const running = server.view?.state === "running";
  const { data, mutate } = useApi<PlayerLists>(`/servers/${server.id}/player-lists`, { refreshInterval: running ? 15000 : 0 });
  const [list, setList] = useState<List>("whitelist");
  const [name, setName] = useState("");
  const act = usePlayerActions(server, () => setTimeout(() => mutate(), 1200));

  const items = !data
    ? []
    : list === "whitelist"
      ? data.whitelist.map((p) => ({ key: p.uuid || p.name, label: p.name, note: "" }))
      : list === "ops"
        ? data.ops.map((p) => ({ key: p.uuid || p.name, label: p.name, note: p.level ? `Stufe ${p.level}` : "" }))
        : list === "banned"
          ? data.banned.map((p) => ({ key: p.uuid || p.name, label: p.name, note: p.reason ?? "" }))
          : data.bannedIps.map((p) => ({ key: p.ip, label: p.ip, note: p.reason ?? "" }));

  const add = { whitelist: "whitelist-add", ops: "op", banned: "ban", ips: "ban-ip" }[list] as PlayerAction;
  const remove = { whitelist: "whitelist-remove", ops: "deop", banned: "pardon", ips: "pardon-ip" }[list] as PlayerAction;
  const valid = list === "ips" ? /^[0-9A-Fa-f:.]{3,45}$/.test(name) : nameOk.test(name);

  return (
    <Card>
      <CardHeader
        title="Verwaltung"
        description={running ? "Änderungen gelten sofort. Die Listen stammen aus den Dateien des Servers." : "Der Server muss laufen, um Spieler zu verwalten. Die Listen siehst du trotzdem."}
        action={
          <div className="flex gap-2">
            <Button size="sm" disabled={!running} onClick={() => act("whitelist-on", "", {}, "Whitelist ist an")}>
              Whitelist an
            </Button>
            <Button size="sm" disabled={!running} onClick={() => act("whitelist-off", "", {}, "Whitelist ist aus")}>
              aus
            </Button>
          </div>
        }
      />
      <div className="space-y-4 px-5 py-4">
        <div className="overflow-x-auto">
          <Tabs
            value={list}
            onChange={setList}
            items={[
              { id: "whitelist", label: "Whitelist", count: data?.whitelist.length },
              { id: "ops", label: "Operatoren", count: data?.ops.length },
              { id: "banned", label: "Gesperrt", count: data?.banned.length },
              { id: "ips", label: "IP-Sperren", count: data?.bannedIps.length },
            ]}
          />
        </div>
        <form
          className="flex gap-2"
          onSubmit={async (event) => {
            event.preventDefault();
            if (await act(add, name, {}, "Hinzugefügt")) setName("");
          }}
        >
          <input className="field max-w-xs" value={name} onChange={(event) => setName(event.target.value)} placeholder={list === "ips" ? "IP-Adresse" : "Spielername"} disabled={!running} />
          <Button type="submit" disabled={!running || !valid}>
            {list === "banned" || list === "ips" ? <Gavel className="h-4 w-4" /> : <UserPlus className="h-4 w-4" />}
            {list === "banned" || list === "ips" ? "Sperren" : "Hinzufügen"}
          </Button>
        </form>
        {items.length === 0 ? (
          <EmptyState icon={<Users className="h-6 w-6" />} title="Die Liste ist leer" />
        ) : (
          <div className="divide-y divide-white/[0.05] rounded-lg border border-white/[0.06]">
            {items.map((item) => (
              <div key={item.key} className="flex items-center gap-3 px-4 py-2">
                {list !== "ips" && (
                  // eslint-disable-next-line @next/next/no-img-element
                  <img src={`https://mc-heads.net/avatar/${encodeURIComponent(item.label)}/48`} alt="" width={24} height={24} className="rounded bg-raised [image-rendering:pixelated]" loading="lazy" />
                )}
                <span className="min-w-0 flex-1 truncate text-sm">{item.label}</span>
                {item.note && <Badge>{item.note}</Badge>}
                <Button variant="ghost" size="icon-sm" disabled={!running} aria-label="Entfernen" onClick={() => act(remove, item.label, {}, "Entfernt")}>
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </div>
            ))}
          </div>
        )}
      </div>
    </Card>
  );
}
