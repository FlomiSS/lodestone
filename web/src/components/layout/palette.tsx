"use client";

import { useRouter } from "next/navigation";
import { useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Cpu, LayoutDashboard, Network, Search, Server, UserCog, Users, ScrollText, type LucideIcon } from "lucide-react";
import { cx } from "@/components/ui/primitives";
import type { Project, User } from "@/lib/types";

interface Entry {
  key: string;
  label: string;
  hint: string;
  href: string;
  icon: LucideIcon;
}

/** Strg+K: springt zu Projekten, Servern und Seiten. */
export function CommandPalette({ projects, user, open, onClose }: { projects: Project[] | undefined; user: User; open: boolean; onClose: () => void }) {
  const router = useRouter();
  const [query, setQuery] = useState("");
  const [index, setIndex] = useState(0);
  const input = useRef<HTMLInputElement>(null);

  const entries = useMemo<Entry[]>(() => {
    const list: Entry[] = [{ key: "home", label: "Übersicht", hint: "Seite", href: "/", icon: LayoutDashboard }];
    for (const project of projects ?? []) {
      list.push({ key: `p-${project.id}`, label: project.name, hint: project.kind === "network" ? "Netzwerk" : "Projekt", href: `/project/?id=${project.id}`, icon: project.kind === "network" ? Network : Server });
      for (const server of project.servers) {
        list.push({ key: `s-${server.id}`, label: server.name, hint: `Server in ${project.name}`, href: `/server/?id=${server.id}`, icon: Server });
      }
    }
    list.push({ key: "nodes", label: "Nodes", hint: "Seite", href: "/nodes/", icon: Cpu });
    if (user.isAdmin) {
      list.push({ key: "users", label: "Benutzer", hint: "Seite", href: "/users/", icon: Users });
      list.push({ key: "audit", label: "Protokoll", hint: "Seite", href: "/audit/", icon: ScrollText });
    }
    list.push({ key: "account", label: "Konto, Zwei-Faktor, API-Schlüssel", hint: "Seite", href: "/account/", icon: UserCog });
    return list;
  }, [projects, user.isAdmin]);

  const shown = useMemo(() => {
    const words = query.toLowerCase().split(/\s+/).filter(Boolean);
    return entries.filter((entry) => words.every((word) => `${entry.label} ${entry.hint}`.toLowerCase().includes(word))).slice(0, 12);
  }, [entries, query]);

  useEffect(() => {
    if (open) {
      setQuery("");
      setIndex(0);
      setTimeout(() => input.current?.focus(), 0);
    }
  }, [open]);
  useEffect(() => setIndex(0), [query]);

  if (!open || typeof document === "undefined") return null;

  function go(entry: Entry | undefined) {
    if (!entry) return;
    onClose();
    router.push(entry.href);
  }

  return createPortal(
    <div className="fixed inset-0 z-[70] grid place-items-start bg-black/60 px-4 pt-[15vh] backdrop-blur-sm" onMouseDown={onClose}>
      <div className="mx-auto w-full max-w-lg overflow-hidden rounded-xl border border-white/[0.1] bg-surface shadow-2xl" role="dialog" aria-label="Schnellsuche" onMouseDown={(event) => event.stopPropagation()}>
        <div className="flex items-center gap-3 border-b border-white/[0.07] px-4">
          <Search className="h-4 w-4 text-muted" />
          <input
            ref={input}
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "ArrowDown") {
                event.preventDefault();
                setIndex((value) => Math.min(shown.length - 1, value + 1));
              } else if (event.key === "ArrowUp") {
                event.preventDefault();
                setIndex((value) => Math.max(0, value - 1));
              } else if (event.key === "Enter") {
                event.preventDefault();
                go(shown[index]);
              } else if (event.key === "Escape") {
                onClose();
              }
            }}
            placeholder="Projekt, Server oder Seite suchen"
            className="h-12 flex-1 bg-transparent text-sm outline-none placeholder:text-muted"
          />
          <kbd className="rounded border border-white/[0.1] px-1.5 py-0.5 text-[10px] text-muted">Esc</kbd>
        </div>
        <div className="max-h-80 overflow-y-auto p-1.5">
          {shown.length === 0 && <p className="px-3 py-6 text-center text-sm text-muted">Nichts gefunden</p>}
          {shown.map((entry, position) => (
            <button
              key={entry.key}
              onMouseMove={() => setIndex(position)}
              onClick={() => go(entry)}
              className={cx("flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left text-sm", position === index ? "bg-white/[0.08] text-ink" : "text-muted")}
            >
              <entry.icon className="h-4 w-4 shrink-0" />
              <span className="min-w-0 flex-1 truncate">{entry.label}</span>
              <span className="shrink-0 text-xs text-muted/80">{entry.hint}</span>
            </button>
          ))}
        </div>
      </div>
    </div>,
    document.body,
  );
}
