"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { ArrowUpCircle, Cable, Cpu, FolderKanban, LayoutDashboard, Package, ScrollText, Users, type LucideIcon } from "lucide-react";
import { SecondarySidebar } from "@/components/layout/shell";
import { cx } from "@/components/ui/primitives";

interface AdminEntry {
  href: string;
  label: string;
  icon: LucideIcon;
  isActive: (path: string) => boolean;
}

const entries: AdminEntry[] = [
  { href: "/admin/", label: "Übersicht", icon: LayoutDashboard, isActive: (p) => p === "/admin/" },
  { href: "/users/", label: "Benutzer", icon: Users, isActive: (p) => p.startsWith("/users/") },
  { href: "/admin/projects/", label: "Projekte", icon: FolderKanban, isActive: (p) => p.startsWith("/admin/projects/") },
  { href: "/nodes/", label: "Nodes", icon: Cpu, isActive: (p) => p.startsWith("/nodes/") },
  { href: "/admin/packs/", label: "Position-Packs", icon: Package, isActive: (p) => p.startsWith("/admin/packs/") },
  { href: "/admin/playit/", label: "playit.gg", icon: Cable, isActive: (p) => p.startsWith("/admin/playit/") },
  { href: "/admin/updates/", label: "Updates", icon: ArrowUpCircle, isActive: (p) => p.startsWith("/admin/updates/") },
  { href: "/audit/", label: "Protokoll", icon: ScrollText, isActive: (p) => p.startsWith("/audit/") },
];

function normalize(path: string): string {
  const p = path.split("?")[0].replace(/\/+$/, "");
  return p ? p + "/" : "/";
}

export function AdminSidebar() {
  const rawPath = usePathname() ?? "/";
  const path = normalize(rawPath);

  return (
    <SecondarySidebar>
      <div className="flex flex-col gap-4 p-4 lg:min-h-full lg:p-5">
        <div>
          <p className="truncate text-[15px] font-semibold tracking-tight">Admin-Panel</p>
          <p className="mt-0.5 truncate text-xs text-muted">Systemverwaltung</p>
        </div>

        <nav className="flex gap-1 overflow-x-auto lg:flex-col lg:overflow-visible">
          {entries.map((entry) => {
            const active = entry.isActive(path);
            const Icon = entry.icon;
            return (
              <Link
                key={entry.href}
                href={entry.href}
                className={cx(
                  "flex shrink-0 items-center gap-3 whitespace-nowrap rounded-lg border px-3 py-2 text-left text-[13px] transition duration-100 active:scale-[0.97]",
                  active ? "border-white/[0.07] bg-white/[0.07] text-ink" : "border-transparent text-muted hover:bg-white/[0.04] hover:text-ink",
                )}
              >
                <Icon className={cx("h-4 w-4 shrink-0", active ? "text-ink" : "text-muted")} />
                <span className="flex-1 truncate">{entry.label}</span>
              </Link>
            );
          })}
        </nav>
      </div>
    </SecondarySidebar>
  );
}
