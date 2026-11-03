"use client";

import Link from "next/link";
import { usePathname, useSearchParams } from "next/navigation";
import {
  Suspense,
  createContext,
  useContext,
  useEffect,
  useRef,
  useState,
  type ComponentType,
  type ReactNode,
} from "react";
import {
  Archive,
  ArrowUpCircle,
  BarChart3,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Cpu,
  FolderKanban,
  FolderOpen,
  LogOut,
  Menu,
  Network,
  PanelLeftClose,
  PanelLeftOpen,
  Puzzle,
  ScrollText,
  Search,
  Server,
  Settings,
  ShieldCheck,
  Terminal,
  UserCog,
  Users,
  X,
} from "lucide-react";
import { Meter, cx, loadTone } from "@/components/ui/primitives";
import { LodestoneLogo, PixelAvatar, Wordmark } from "@/components/ui/mc";
import { Loading } from "@/components/ui/async";
import { StateBadge } from "@/components/status";
import { CommandPalette } from "./palette";
import { api, useApi } from "@/lib/api";
import { roleRank, type AuthStatus, type Project, type User } from "@/lib/types";

type Icon = ComponentType<{ className?: string }>;

const SessionContext = createContext<User | null>(null);
export const useUser = () => useContext(SessionContext) as User;

/** Rückwärtskompatibler Export (wird von der dynamischen Seitenleiste abgelöst). */
export function SecondarySidebar({ children }: { children?: ReactNode }) {
  return null;
}

// ---------------------------------------------------------------- Anmeldung

/** Prüft die Anmeldung und zeigt erst dann die Oberfläche. */
export function AuthGate({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);

  useEffect(() => {
    api
      .get<AuthStatus>("/auth/status")
      .then((status) => {
        if (status.setupRequired || !status.user) {
          const next = encodeURIComponent(window.location.pathname + window.location.search);
          window.location.replace(`/login/?next=${next}`);
          return;
        }
        setUser(status.user);
      })
      .catch(() => window.location.replace("/login/"));
  }, []);

  if (!user) {
    return (
      <div className="grid min-h-screen place-items-center">
        <Loading label="Anmeldung wird geprüft" />
      </div>
    );
  }
  return (
    <SessionContext.Provider value={user}>
      <Suspense fallback={null}>
        <Shell user={user}>{children}</Shell>
      </Suspense>
    </SessionContext.Provider>
  );
}

function normalize(path: string): string {
  const p = path.split("?")[0].replace(/\/+$/, "");
  return p ? p + "/" : "/";
}

const FOLD_KEY = "lodestone.sidebar.folded";

// Alle Seiten des Admin-Panels; sie teilen sich den dynamischen Admin-Bereich der Seitenleiste.
const ADMIN_PATHS = ["/admin/", "/users/", "/audit/", "/nodes/"];

type NavView = "root" | "project" | "server" | "admin";

function getNavView(pathname: string): NavView {
  if (pathname.startsWith("/server")) return "server";
  if (pathname.startsWith("/project")) return "project";
  if (ADMIN_PATHS.some((path) => pathname.startsWith(path))) return "admin";
  return "root";
}

function getDepth(view: NavView): number {
  switch (view) {
    case "server":
      return 2;
    case "project":
    case "admin":
      return 1;
    case "root":
    default:
      return 0;
  }
}

function Shell({ user, children }: { user: User; children: ReactNode }) {
  const pathname = normalize(usePathname() ?? "/");
  const params = useSearchParams();
  const [open, setOpen] = useState(false);
  const [folded, setFolded] = useState(false);
  const [palette, setPalette] = useState(false);
  const [projectsExpanded, setProjectsExpanded] = useState(true);
  const [showAllProjects, setShowAllProjects] = useState(false);
  const [profileMenuOpen, setProfileMenuOpen] = useState(false);
  const profileRef = useRef<HTMLDivElement>(null);

  const { data: projects } = useApi<Project[]>("/projects", { refreshInterval: 15000 });

  const activeNavView = getNavView(pathname);
  const [viewState, setViewState] = useState<{
    current: NavView;
    previous: NavView | null;
    direction: "forward" | "backward" | "none";
  }>({
    current: activeNavView,
    previous: null,
    direction: "none",
  });

  useEffect(() => setOpen(false), [pathname, params]);
  useEffect(() => setProfileMenuOpen(false), [pathname, params]);
  useEffect(() => setFolded(window.localStorage.getItem(FOLD_KEY) === "1"), []);

  // Animiertes Ein- und Ausgleiten je nach Unterseite / Navigationstiefe
  useEffect(() => {
    if (activeNavView !== viewState.current) {
      const prevDepth = getDepth(viewState.current);
      const nextDepth = getDepth(activeNavView);
      const direction = nextDepth >= prevDepth ? "forward" : "backward";
      setViewState({
        current: activeNavView,
        previous: viewState.current,
        direction,
      });
      const timer = setTimeout(() => {
        setViewState((state) => ({ ...state, previous: null, direction: "none" }));
      }, 220);
      return () => clearTimeout(timer);
    }
  }, [activeNavView, viewState.current]);

  // Klick außerhalb des Profilmenüs schließt dieses
  useEffect(() => {
    function onMouseDown(e: MouseEvent) {
      if (profileRef.current && !profileRef.current.contains(e.target as Node)) {
        setProfileMenuOpen(false);
      }
    }
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") setProfileMenuOpen(false);
    }
    if (profileMenuOpen) {
      document.addEventListener("mousedown", onMouseDown);
      document.addEventListener("keydown", onKeyDown);
      return () => {
        document.removeEventListener("mousedown", onMouseDown);
        document.removeEventListener("keydown", onKeyDown);
      };
    }
  }, [profileMenuOpen]);

  function toggleFold() {
    setFolded((value) => {
      window.localStorage.setItem(FOLD_KEY, value ? "0" : "1");
      return !value;
    });
  }

  // Strg+B klappt die Seitenleiste ein und aus, Strg+K öffnet die Schnellsuche
  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setPalette((value) => !value);
      }
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "b") {
        event.preventDefault();
        toggleFold();
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  async function logout() {
    await api.post("/auth/logout").catch(() => null);
    window.location.replace("/login/");
  }

  const segments = pathname.split("/").filter(Boolean);
  const currentProjectId =
    segments[0] === "project" && segments[1]
      ? segments[1]
      : pathname === "/project/"
        ? params.get("id")
        : null;
  const currentServerId =
    segments[0] === "server" && segments[1]
      ? segments[1]
      : pathname === "/server/"
        ? params.get("id")
        : null;

  const activeProject = (projects ?? []).find((p) => p.id === currentProjectId);
  const activeServerProject = (projects ?? []).find((p) =>
    p.servers.some((s) => s.id === currentServerId),
  );
  const activeServer = activeServerProject?.servers.find((s) => s.id === currentServerId);

  function link(href: string, label: string, Icon: Icon, selected: boolean, compact: boolean) {
    return (
      <Link
        key={href}
        href={href}
        title={compact ? label : undefined}
        className={cx(
          "flex items-center rounded-lg border text-[13px] transition duration-100 active:scale-[0.97]",
          compact ? "justify-center px-0 py-2.5" : "gap-3 px-3 py-2",
          selected
            ? "border-white/[0.07] bg-white/[0.07] font-medium text-ink"
            : "border-transparent text-muted hover:bg-white/[0.04] hover:text-ink",
        )}
      >
        <Icon className={cx("h-4 w-4 shrink-0", selected ? "text-ink" : "text-muted")} />
        {!compact && <span className="flex-1 truncate">{label}</span>}
      </Link>
    );
  }

  function renderView(view: NavView, compact: boolean) {
    if (view === "server") {
      const server = activeServer;
      const project = activeServerProject;
      const rank = project ? roleRank[project.myRole] : roleRank.viewer;
      const routeTab =
        segments[0] === "server" && segments[2]
          ? segments[2]
          : params.get("tab") || "console";
      const extensions =
        server?.software === "fabric" ||
        server?.software === "neoforge" ||
        server?.software === "forge"
          ? "Mods"
          : server?.software === "vanilla"
            ? null
            : "Plugins";

      const entries = [
        { id: "console", label: "Konsole", icon: Terminal },
        { id: "stats", label: "Statistiken", icon: BarChart3 },
        ...(rank >= roleRank.operator ? [{ id: "files", label: "Dateien", icon: FolderOpen }] : []),
        ...(extensions ? [{ id: "addons", label: extensions, icon: Puzzle }] : []),
        { id: "players", label: "Spieler", icon: Users },
        { id: "backups", label: "Backups", icon: Archive },
        ...(rank >= roleRank.admin ? [{ id: "settings", label: "Einstellungen", icon: Settings }] : []),
      ];

      const stats = server?.view?.stats;
      const memoryPercent =
        stats && stats.memoryLimit > 0 ? (stats.memoryBytes / stats.memoryLimit) * 100 : 0;

      const projectHref = project ? `/project/${project.id}/` : "/";

      return (
        <div className="space-y-3 pb-4">
          <Link
            href={projectHref}
            className={cx(
              "flex items-center gap-2 rounded-lg py-1.5 text-xs text-muted transition hover:bg-white/[0.04] hover:text-ink",
              compact ? "justify-center px-0" : "px-2.5",
            )}
            title={compact ? project?.name ?? "Projekt" : undefined}
          >
            <ChevronLeft className="h-4 w-4 shrink-0" />
            {!compact && <span className="truncate">{project?.name ?? "Projekt"}</span>}
          </Link>

          {!compact && server && (
            <div className="rounded-lg border border-white/[0.07] bg-white/[0.02] p-2.5">
              <div className="flex items-center justify-between gap-2">
                <p className="truncate text-[13px] font-semibold">{server.name}</p>
                <StateBadge server={server} />
              </div>
              <p className="mt-0.5 truncate text-[11px] text-muted">
                {server.software} {server.view?.version}
              </p>
            </div>
          )}

          <div className="space-y-0.5">
            {entries.map((entry) => {
              const target =
                entry.id === "console"
                  ? `/server/${server?.id ?? currentServerId}/`
                  : `/server/${server?.id ?? currentServerId}/${entry.id}/`;
              const isSelected = routeTab === entry.id;
              return (
                <Link
                  key={entry.id}
                  href={target}
                  className={cx(
                    "flex items-center rounded-lg border text-[13px] transition duration-100 active:scale-[0.97]",
                    compact ? "justify-center px-0 py-2.5" : "gap-3 px-3 py-2",
                    isSelected
                      ? "border-white/[0.07] bg-white/[0.07] font-medium text-ink"
                      : "border-transparent text-muted hover:bg-white/[0.04] hover:text-ink",
                  )}
                  title={compact ? entry.label : undefined}
                >
                  <entry.icon
                    className={cx("h-4 w-4 shrink-0", isSelected ? "text-ink" : "text-muted")}
                  />
                  {!compact && <span className="flex-1 truncate">{entry.label}</span>}
                </Link>
              );
            })}
          </div>

          {!compact && server?.view?.state === "running" && stats && (
            <div className="mt-4 space-y-2.5 rounded-lg border border-white/[0.06] bg-white/[0.02] p-2.5 text-xs">
              <div>
                <div className="mb-1 flex justify-between text-[11px] text-muted">
                  <span>CPU</span>
                  <span className="tabular-nums text-ink/80">{stats.cpuPercent.toFixed(0)} %</span>
                </div>
                <Meter value={stats.cpuPercent} max={100} tone={loadTone(stats.cpuPercent)} />
              </div>
              <div>
                <div className="mb-1 flex justify-between text-[11px] text-muted">
                  <span>Speicher</span>
                  <span className="tabular-nums text-ink/80">
                    {Math.round(stats.memoryBytes / 1024 ** 2)} MB
                  </span>
                </div>
                <Meter value={memoryPercent} max={100} tone={loadTone(memoryPercent)} />
              </div>
            </div>
          )}
        </div>
      );
    }

    if (view === "project") {
      const project = activeProject;
      const rank = project ? roleRank[project.myRole] : roleRank.viewer;
      const canManage = rank >= roleRank.admin;
      const network = project?.kind === "network";
      const routeTab =
        segments[0] === "project" && segments[2]
          ? segments[2]
          : params.get("tab") || "servers";

      const projectNavItems = [
        { id: "servers", label: "Server", icon: Server, count: project?.servers.length },
        ...(network
          ? [
              { id: "network", label: "Netzwerk", icon: Network },
              { id: "groups", label: "Gruppen", icon: FolderKanban, count: project?.groups.length },
            ]
          : []),
        { id: "schedules", label: "Zeitpläne", icon: ScrollText },
        { id: "events", label: "Ereignisse", icon: BarChart3 },
        { id: "members", label: "Mitglieder", icon: Users, count: project?.members?.length },
        ...(canManage ? [{ id: "settings", label: "Einstellungen", icon: Settings }] : []),
      ];

      return (
        <div className="space-y-3 pb-4">
          <Link
            href="/"
            className={cx(
              "flex items-center gap-2 rounded-lg py-1.5 text-xs text-muted transition hover:bg-white/[0.04] hover:text-ink",
              compact ? "justify-center px-0" : "px-2.5",
            )}
            title={compact ? "Alle Projekte" : undefined}
          >
            <ChevronLeft className="h-4 w-4 shrink-0" />
            {!compact && <span>Alle Projekte</span>}
          </Link>

          {!compact && project && (
            <div className="rounded-lg border border-white/[0.07] bg-white/[0.02] p-2.5">
              <div className="flex items-center gap-2.5">
                <span className="grid h-7 w-7 shrink-0 place-items-center rounded-lg border border-white/[0.08] bg-white/[0.04] text-muted">
                  {project.kind === "network" ? (
                    <Network className="h-3.5 w-3.5" />
                  ) : (
                    <Server className="h-3.5 w-3.5" />
                  )}
                </span>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-[13px] font-semibold">{project.name}</p>
                  <p className="text-[11px] text-muted">
                    {project.kind === "network" ? "Netzwerk" : "Einzelner Server"}
                  </p>
                </div>
              </div>
            </div>
          )}

          <div className="space-y-0.5">
            {projectNavItems.map((item) => {
              const target =
                item.id === "servers"
                  ? `/project/${project?.id ?? currentProjectId}/`
                  : `/project/${project?.id ?? currentProjectId}/${item.id}/`;
              const isSelected = routeTab === item.id;
              return (
                <Link
                  key={item.id}
                  href={target}
                  className={cx(
                    "flex items-center rounded-lg border text-[13px] transition duration-100 active:scale-[0.97]",
                    compact ? "justify-center px-0 py-2.5" : "gap-3 px-3 py-2",
                    isSelected
                      ? "border-white/[0.07] bg-white/[0.07] font-medium text-ink"
                      : "border-transparent text-muted hover:bg-white/[0.04] hover:text-ink",
                  )}
                  title={compact ? item.label : undefined}
                >
                  <item.icon
                    className={cx("h-4 w-4 shrink-0", isSelected ? "text-ink" : "text-muted")}
                  />
                  {!compact && (
                    <>
                      <span className="flex-1 truncate">{item.label}</span>
                      {item.count !== undefined && item.count > 0 && (
                        <span className="rounded-full bg-white/[0.06] px-1.5 py-0.2 text-[10px] text-muted">
                          {item.count}
                        </span>
                      )}
                    </>
                  )}
                </Link>
              );
            })}
          </div>

          {!compact && project && project.servers.length > 0 && (
            <div className="pt-2">
              <p className="px-3 pb-1.5 text-[10.5px] font-medium uppercase tracking-wider text-muted/70">
                Server
              </p>
              <div className="space-y-0.5">
                {project.servers.map((server) => (
                  <Link
                    key={server.id}
                    href={`/server/${server.id}/`}
                    className="flex items-center gap-2.5 rounded-lg px-3 py-1.5 text-[12.5px] text-muted transition hover:bg-white/[0.04] hover:text-ink"
                  >
                    <Server className="h-3.5 w-3.5 shrink-0 text-muted" />
                    <span className="truncate flex-1">{server.name}</span>
                    <span className="text-[10px] text-muted">
                      {server.role === "proxy" ? "Proxy" : server.software}
                    </span>
                  </Link>
                ))}
              </div>
            </div>
          )}
        </div>
      );
    }

    if (view === "admin") {
      return (
        <div className="space-y-3 pb-4">
          <Link
            href="/"
            className={cx(
              "flex items-center gap-2 rounded-lg py-1.5 text-xs text-muted transition hover:bg-white/[0.04] hover:text-ink",
              compact ? "justify-center px-0" : "px-2.5",
            )}
            title={compact ? "Zu den Projekten" : undefined}
          >
            <ChevronLeft className="h-4 w-4 shrink-0" />
            {!compact && <span>Zu den Projekten</span>}
          </Link>

          {!compact && (
            <div className="rounded-lg border border-white/[0.07] bg-white/[0.02] p-2.5">
              <div className="flex items-center gap-2">
                <ShieldCheck className="h-4 w-4 text-accent shrink-0" />
                <p className="text-[13px] font-semibold">Admin-Bereich</p>
              </div>
              <p className="mt-0.5 text-[11px] text-muted">Verwaltung & System</p>
            </div>
          )}

          <div className="space-y-0.5">
            {user.isAdmin && link("/admin/", "Admin-Panel", ShieldCheck, pathname === "/admin/", compact)}
            {link("/nodes/", "Nodes", Cpu, pathname.startsWith("/nodes/"), compact)}
            {user.isAdmin && link("/users/", "Benutzer", Users, pathname === "/users/", compact)}
            {user.isAdmin &&
              link(
                "/admin/projects/",
                "Alle Projekte",
                FolderKanban,
                pathname === "/admin/projects/",
                compact,
              )}
            {user.isAdmin &&
              link("/admin/updates/", "Updates", ArrowUpCircle, pathname === "/admin/updates/", compact)}
            {user.isAdmin &&
              link("/admin/packs/", "Packs", FolderOpen, pathname === "/admin/packs/", compact)}
            {user.isAdmin &&
              link("/audit/", "Protokoll", ScrollText, pathname === "/audit/", compact)}
          </div>
        </div>
      );
    }

    // Standardansicht (Root): Suche, Ausklappbare Projekte (3-4 Einträge + mehr anzeigen), Konto
    const allProjects = projects ?? [];
    const displayedProjects = showAllProjects ? allProjects : allProjects.slice(0, 4);
    const hasMoreProjects = allProjects.length > 4;

    return (
      <div className="space-y-1 pb-4">
        <button
          onClick={() => setPalette(true)}
          title={compact ? "Suchen (Strg K)" : undefined}
          className={cx(
            "mb-2 flex w-full items-center rounded-lg border border-white/[0.07] bg-white/[0.03] text-[13px] text-muted transition hover:bg-white/[0.06] hover:text-ink",
            compact ? "justify-center py-2.5" : "gap-3 px-3 py-2",
          )}
        >
          <Search className="h-4 w-4 shrink-0" />
          {!compact && (
            <>
              <span className="flex-1 text-left">Suchen</span>
              <kbd className="rounded border border-white/[0.1] px-1.5 text-[10px]">Strg K</kbd>
            </>
          )}
        </button>

        {/* Projekte: Ausklappbar 3-4 Einträge + mehr anzeigen */}
        <div>
          <div
            className={cx(
              "flex items-center rounded-lg border text-[13px] transition duration-100",
              pathname === "/"
                ? "border-white/[0.07] bg-white/[0.07] text-ink font-medium"
                : "border-transparent text-muted hover:bg-white/[0.04] hover:text-ink",
              compact ? "justify-center px-0 py-2.5" : "gap-2 px-3 py-2",
            )}
          >
            <Link
              href="/"
              className="flex flex-1 items-center gap-3 min-w-0"
              title={compact ? "Projekte" : undefined}
            >
              <FolderKanban
                className={cx("h-4 w-4 shrink-0", pathname === "/" ? "text-ink" : "text-muted")}
              />
              {!compact && <span className="flex-1 truncate">Projekte</span>}
            </Link>
            {!compact && (
              <button
                type="button"
                onClick={(e) => {
                  e.preventDefault();
                  e.stopPropagation();
                  setProjectsExpanded((v) => !v);
                }}
                className="rounded p-1 text-muted hover:bg-white/[0.08] hover:text-ink"
                aria-label={projectsExpanded ? "Projekte einklappen" : "Projekte ausklappen"}
              >
                {projectsExpanded ? (
                  <ChevronDown className="h-3.5 w-3.5" />
                ) : (
                  <ChevronRight className="h-3.5 w-3.5" />
                )}
              </button>
            )}
          </div>

          {!compact && projectsExpanded && (
            <div className="mt-1 space-y-0.5 border-l border-white/[0.06] ml-4 pl-3">
              {displayedProjects.map((project) => (
                <Link
                  key={project.id}
                  href={`/project/${project.id}/`}
                  className="flex items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-[12.5px] text-muted transition hover:bg-white/[0.04] hover:text-ink"
                >
                  {project.kind === "network" ? (
                    <Network className="h-3.5 w-3.5 shrink-0 text-muted" />
                  ) : (
                    <Server className="h-3.5 w-3.5 shrink-0 text-muted" />
                  )}
                  <span className="truncate flex-1">{project.name}</span>
                </Link>
              ))}

              {hasMoreProjects && (
                <button
                  type="button"
                  onClick={() => setShowAllProjects((v) => !v)}
                  className="flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-[11.5px] text-muted hover:text-ink hover:bg-white/[0.04] transition text-left"
                >
                  {showAllProjects ? (
                    <span>Weniger anzeigen</span>
                  ) : (
                    <span>+ {allProjects.length - 4} mehr anzeigen</span>
                  )}
                </button>
              )}

              {projects && allProjects.length === 0 && (
                <p className="px-2.5 py-1 text-xs text-muted/70">Noch keine Projekte</p>
              )}
            </div>
          )}
        </div>

        {/* Konto */}
        <div className="mt-1">
          {link("/account/", "Konto", UserCog, pathname === "/account/", compact)}
        </div>
      </div>
    );
  }

  function sidebar(compact: boolean) {
    return (
      <div className="flex h-full flex-col">
        <div className={cx("pb-3 pt-4", compact ? "px-2" : "px-3")}>
          <div
            className={cx(
              "flex items-center rounded-lg py-1.5",
              compact ? "justify-center" : "gap-2.5 px-2",
            )}
          >
            <LodestoneLogo size={28} />
            {!compact && <Wordmark className="min-w-0 flex-1 truncate text-[12.5px]" />}
            {!compact && (
              <button
                onClick={() => setOpen(false)}
                className="rounded-md p-1.5 text-muted hover:bg-white/[0.06] hover:text-ink lg:hidden"
                aria-label="Menü schließen"
              >
                <X className="h-4 w-4" />
              </button>
            )}
          </div>
        </div>

        {/* Dynamischer, animierter Inhaltsbereich der Leiste */}
        <div className={cx("relative flex-1 overflow-hidden", compact ? "px-2" : "px-3")}>
          {viewState.previous && (
            <div
              key={`prev-${viewState.previous}`}
              className={cx(
                "absolute inset-0 no-scrollbar overflow-y-auto",
                viewState.direction === "forward" ? "animate-slide-out-left" : "animate-slide-out-right",
              )}
            >
              {renderView(viewState.previous, compact)}
            </div>
          )}
          <div
            key={`curr-${viewState.current}`}
            className={cx(
              "h-full no-scrollbar overflow-y-auto",
              viewState.previous && "absolute inset-0",
              viewState.direction === "forward" && "animate-slide-in-right",
              viewState.direction === "backward" && "animate-slide-in-left",
            )}
          >
            {renderView(viewState.current, compact)}
          </div>
        </div>

        {/* Fußzeile mit Profil, Dropdown (inkl. Admin-Bereich), Einstellungs-/Kontoknopf und Abmelden */}
        <div className="relative border-t border-white/[0.06] p-3" ref={profileRef}>
          {profileMenuOpen && (
            <div className="absolute bottom-full left-2 right-2 mb-2 z-50 rounded-xl border border-white/[0.08] bg-surface p-1.5 shadow-2xl">
              <div className="border-b border-white/[0.06] px-3 py-2 mb-1">
                <p className="truncate text-xs font-medium text-ink">{user.username}</p>
                <p className="text-[10px] text-muted">
                  {user.isAdmin ? "Administrator" : "Benutzer"}
                </p>
              </div>

              {user.isAdmin && (
                <Link
                  href="/admin/"
                  onClick={() => setProfileMenuOpen(false)}
                  className="flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-left text-[12.5px] text-muted transition hover:bg-white/[0.06] hover:text-ink"
                >
                  <ShieldCheck className="h-4 w-4 text-accent shrink-0" />
                  <span>Zum Admin-Bereich</span>
                </Link>
              )}

              <Link
                href="/account/"
                onClick={() => setProfileMenuOpen(false)}
                className="flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-left text-[12.5px] text-muted transition hover:bg-white/[0.06] hover:text-ink"
              >
                <UserCog className="h-4 w-4 shrink-0" />
                <span>Konto</span>
              </Link>

              <div className="my-1 h-px bg-white/[0.06]" />

              <button
                onClick={() => {
                  setProfileMenuOpen(false);
                  logout();
                }}
                className="flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-left text-[12.5px] text-bad/90 transition hover:bg-bad/10 hover:text-bad"
              >
                <LogOut className="h-4 w-4 shrink-0" />
                <span>Abmelden</span>
              </button>
            </div>
          )}

          <div
            className={cx(
              "flex items-center rounded-lg py-1.5",
              compact ? "flex-col gap-2 px-0" : "gap-2 px-1",
            )}
          >
            <button
              onClick={() => setProfileMenuOpen((v) => !v)}
              className={cx(
                "flex min-w-0 flex-1 items-center rounded-lg text-left transition hover:bg-white/[0.04]",
                compact ? "justify-center p-1" : "gap-2.5 p-1.5",
                profileMenuOpen && "bg-white/[0.06]",
              )}
              title={compact ? user.username : "Profilmenü öffnen"}
              aria-haspopup="true"
              aria-expanded={profileMenuOpen}
            >
              <PixelAvatar name={user.username} size={28} />
              {!compact && (
                <div className="min-w-0 flex-1">
                  <p className="truncate text-[13px] font-medium leading-snug">{user.username}</p>
                  {user.isAdmin && (
                    <p className="text-[10.5px] text-muted leading-tight">Administrator</p>
                  )}
                </div>
              )}
            </button>

            <div className={cx("flex items-center shrink-0", compact ? "flex-col gap-1" : "gap-0.5")}>
              <Link
                href="/account/"
                className={cx(
                  "rounded-md p-1.5 transition",
                  pathname === "/account/"
                    ? "bg-white/[0.08] text-ink"
                    : "text-muted hover:bg-white/[0.06] hover:text-ink",
                )}
                title="Konto"
                aria-label="Konto"
              >
                <Settings className="h-4 w-4" />
              </Link>
              <button
                onClick={logout}
                className="rounded-md p-1.5 text-muted transition hover:bg-white/[0.06] hover:text-ink"
                title="Abmelden"
                aria-label="Abmelden"
              >
                <LogOut className="h-4 w-4" />
              </button>
            </div>
          </div>

          <button
            onClick={toggleFold}
            className={cx(
              "mt-1 hidden w-full items-center rounded-lg py-2 text-[12px] text-muted transition hover:bg-white/[0.05] hover:text-ink lg:flex",
              compact ? "justify-center" : "gap-3 px-3",
            )}
            title={folded ? "Seitenleiste ausklappen (Strg B)" : "Seitenleiste einklappen (Strg B)"}
            aria-label={folded ? "Seitenleiste ausklappen" : "Seitenleiste einklappen"}
          >
            {folded ? <PanelLeftOpen className="h-4 w-4" /> : <PanelLeftClose className="h-4 w-4" />}
            {!compact && <span>Einklappen</span>}
          </button>
        </div>
      </div>
    );
  }

  return (
    <>
      <CommandPalette projects={projects} user={user} open={palette} onClose={() => setPalette(false)} />
      <div className="min-h-screen bg-bg lg:h-screen lg:overflow-hidden">
        <aside
          className={cx(
            "fixed inset-y-0 left-0 z-30 hidden transition-[width] duration-200 lg:block",
            folded ? "w-[68px]" : "w-[240px]",
          )}
        >
          {sidebar(folded)}
        </aside>

        {open && (
          <div className="fixed inset-0 z-40 lg:hidden">
            <div className="absolute inset-0 bg-black/70 backdrop-blur-sm" onClick={() => setOpen(false)} />
            <aside className="relative h-full w-[264px] border-r border-white/[0.07] bg-bg">{sidebar(false)}</aside>
          </div>
        )}

        <div
          className={cx(
            "min-h-screen p-0 transition-[padding] duration-200 lg:h-screen lg:min-h-0 lg:p-2",
            folded ? "lg:pl-[68px]" : "lg:pl-[240px]",
          )}
        >
          <header className="sticky top-0 z-20 flex h-12 items-center gap-3 border-b border-white/[0.06] bg-bg/90 px-4 backdrop-blur-xl lg:hidden">
            <button
              onClick={() => setOpen(true)}
              className="rounded-lg border border-white/[0.08] p-1.5 text-muted hover:bg-white/[0.06] hover:text-ink"
              aria-label="Menü öffnen"
            >
              <Menu className="h-4 w-4" />
            </button>
            <LodestoneLogo size={22} />
            <Wordmark className="truncate text-[12px]" />
          </header>

          <main className="min-h-[calc(100vh-3rem)] border-white/[0.06] bg-[#151515] lg:h-full lg:min-h-0 lg:overflow-hidden lg:rounded-2xl lg:border">
            <div className="min-w-0 h-full overflow-y-auto">
              <div className="mx-auto w-full max-w-[1480px] px-4 py-6 lg:px-10 lg:py-9">{children}</div>
            </div>
          </main>
        </div>
      </div>
    </>
  );
}
