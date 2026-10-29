"use client";

import Link from "next/link";
import { usePathname, useSearchParams } from "next/navigation";
import { Suspense, createContext, useCallback, useContext, useEffect, useMemo, useState, type ComponentType, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { Cpu, LayoutDashboard, LogOut, Menu, Network, PanelLeftClose, PanelLeftOpen, Server, UserCog, Users, X } from "lucide-react";
import { cx } from "@/components/ui/primitives";
import { LodestoneLogo, PixelAvatar, Wordmark } from "@/components/ui/mc";
import { Loading } from "@/components/ui/async";
import { api, useApi } from "@/lib/api";
import type { AuthStatus, Project, User } from "@/lib/types";

type Icon = ComponentType<{ className?: string }>;

const SessionContext = createContext<User | null>(null);
export const useUser = () => useContext(SessionContext) as User;

// ---------------------------------------------------------------- Zweite Seitenleiste

interface SecondaryApi {
  slot: HTMLElement | null;
  setActive: (active: boolean) => void;
}

const SecondaryContext = createContext<SecondaryApi>({ slot: null, setActive: () => {} });

/**
 * Eine zweite Seitenleiste, die neben der Hauptleiste erscheint (z. B. die Bereiche eines Servers). Der Inhalt wird in
 * den Platz der Shell eingeblendet; verschwindet die Seite, verschwindet auch die Leiste.
 */
export function SecondarySidebar({ children }: { children: ReactNode }) {
  const { slot, setActive } = useContext(SecondaryContext);
  useEffect(() => {
    setActive(true);
    return () => setActive(false);
  }, [setActive]);
  return slot ? createPortal(children, slot) : null;
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

function Shell({ user, children }: { user: User; children: ReactNode }) {
  const pathname = normalize(usePathname() ?? "/");
  const params = useSearchParams();
  const [open, setOpen] = useState(false);
  const [folded, setFolded] = useState(false);
  const [slot, setSlot] = useState<HTMLElement | null>(null);
  const [secondary, setSecondary] = useState(0);
  const { data: projects } = useApi<Project[]>("/projects", { refreshInterval: 15000 });

  useEffect(() => setOpen(false), [pathname, params]);
  useEffect(() => setFolded(window.localStorage.getItem(FOLD_KEY) === "1"), []);

  function toggleFold() {
    setFolded((value) => {
      window.localStorage.setItem(FOLD_KEY, value ? "0" : "1");
      return !value;
    });
  }

  // Strg+B klappt die Seitenleiste ein und aus
  useEffect(() => {
    function onKey(event: KeyboardEvent) {
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

  const currentProject = pathname === "/project/" ? params.get("id") : null;

  function link(href: string, label: string, Icon: Icon, selected: boolean, compact: boolean) {
    return (
      <Link
        key={href}
        href={href}
        title={compact ? label : undefined}
        className={cx(
          "flex items-center rounded-lg border text-[13px] transition duration-100 active:scale-[0.97]",
          compact ? "justify-center px-0 py-2.5" : "gap-3 px-3 py-2",
          selected ? "border-white/[0.07] bg-white/[0.07] text-ink" : "border-transparent text-muted hover:bg-white/[0.04] hover:text-ink",
        )}
      >
        <Icon className={cx("h-4 w-4 shrink-0", selected ? "text-ink" : "text-muted")} />
        {!compact && <span className="flex-1 truncate">{label}</span>}
      </Link>
    );
  }

  function sidebar(compact: boolean) {
    const heading = (text: string) =>
      compact ? <div className="mx-3 my-3 h-px bg-white/[0.06]" /> : <p className="px-3 pb-1 pt-4 text-[10.5px] font-medium uppercase tracking-wider text-muted/70">{text}</p>;
    return (
      <div className="flex h-full flex-col">
        <div className={cx("pb-3 pt-4", compact ? "px-2" : "px-3")}>
          <div className={cx("flex items-center rounded-lg py-1.5", compact ? "justify-center" : "gap-2.5 px-2")}>
            <LodestoneLogo size={28} />
            {!compact && <Wordmark className="min-w-0 flex-1 truncate text-[12.5px]" />}
            {!compact && (
              <button onClick={() => setOpen(false)} className="rounded-md p-1.5 text-muted hover:bg-white/[0.06] hover:text-ink lg:hidden" aria-label="Menü schließen">
                <X className="h-4 w-4" />
              </button>
            )}
          </div>
        </div>

        <nav className={cx("no-scrollbar flex-1 space-y-0.5 overflow-y-auto pb-4", compact ? "px-2" : "px-3")}>
          {link("/", "Übersicht", LayoutDashboard, pathname === "/", compact)}

          {heading("Projekte")}
          {(projects ?? []).map((project) =>
            link(
              `/project/?id=${project.id}`,
              project.name,
              project.kind === "network" ? Network : Server,
              currentProject === project.id || (pathname === "/server/" && project.servers.some((server) => server.id === params.get("id"))),
              compact,
            ),
          )}
          {projects && projects.length === 0 && !compact && <p className="px-3 py-1 text-xs text-muted/70">Noch keine Projekte</p>}

          {heading("Verwaltung")}
          {link("/nodes/", "Nodes", Cpu, pathname === "/nodes/", compact)}
          {user.isAdmin && link("/users/", "Benutzer", Users, pathname === "/users/", compact)}
          {link("/account/", "Konto", UserCog, pathname === "/account/", compact)}
        </nav>

        <div className="border-t border-white/[0.06] p-3">
          <div className={cx("flex items-center rounded-lg py-1.5", compact ? "flex-col gap-2 px-0" : "gap-3 px-2")}>
            <span title={compact ? user.username : undefined}>
              <PixelAvatar name={user.username} size={28} />
            </span>
            {!compact && (
              <div className="min-w-0 flex-1">
                <p className="truncate text-[13px] font-medium">{user.username}</p>
                {user.isAdmin && <p className="text-[10.5px] text-muted">Administrator</p>}
              </div>
            )}
            <button onClick={logout} className="rounded-md p-1.5 text-muted transition hover:bg-white/[0.06] hover:text-ink" title="Abmelden" aria-label="Abmelden">
              <LogOut className="h-4 w-4" />
            </button>
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

  // Stabil halten: Die Leiste meldet sich in einem Effekt an und ab, ein neuer Wert würde das endlos wiederholen
  const setActive = useCallback((active: boolean) => setSecondary((count) => Math.max(0, count + (active ? 1 : -1))), []);
  const secondaryApi = useMemo<SecondaryApi>(() => ({ slot, setActive }), [slot, setActive]);

  return (
    <SecondaryContext.Provider value={secondaryApi}>
      <div className="min-h-screen bg-bg lg:h-screen lg:overflow-hidden">
        <aside className={cx("fixed inset-y-0 left-0 z-30 hidden transition-[width] duration-200 lg:block", folded ? "w-[68px]" : "w-[240px]")}>{sidebar(folded)}</aside>

        {open && (
          <div className="fixed inset-0 z-40 lg:hidden">
            <div className="absolute inset-0 bg-black/70 backdrop-blur-sm" onClick={() => setOpen(false)} />
            <aside className="relative h-full w-[264px] border-r border-white/[0.07] bg-bg">{sidebar(false)}</aside>
          </div>
        )}

        <div className={cx("min-h-screen p-0 transition-[padding] duration-200 lg:h-screen lg:min-h-0 lg:p-2", folded ? "lg:pl-[68px]" : "lg:pl-[240px]")}>
          <header className="sticky top-0 z-20 flex h-12 items-center gap-3 border-b border-white/[0.06] bg-bg/90 px-4 backdrop-blur-xl lg:hidden">
            <button onClick={() => setOpen(true)} className="rounded-lg border border-white/[0.08] p-1.5 text-muted hover:bg-white/[0.06] hover:text-ink" aria-label="Menü öffnen">
              <Menu className="h-4 w-4" />
            </button>
            <LodestoneLogo size={22} />
            <Wordmark className="truncate text-[12px]" />
          </header>

          <main className="min-h-[calc(100vh-3rem)] border-white/[0.06] bg-[#151515] lg:flex lg:h-full lg:min-h-0 lg:overflow-hidden lg:rounded-2xl lg:border">
            {/* Platz für die zweite Seitenleiste; sie erscheint nur, solange eine Seite sie einblendet */}
            <aside
              className={cx(
                "no-scrollbar shrink-0 border-white/[0.06] lg:h-full lg:w-[232px] lg:overflow-y-auto lg:border-r",
                secondary > 0 ? "border-b lg:border-b-0" : "hidden",
              )}
            >
              <div ref={setSlot} className="lg:h-full" />
            </aside>
            {/* Nur dieser Bereich scrollt, die Seitenleisten bleiben stehen */}
            <div className="min-w-0 flex-1 lg:h-full lg:overflow-y-auto">
              <div className="mx-auto w-full max-w-[1480px] animate-fade-in px-4 py-6 lg:px-10 lg:py-9">{children}</div>
            </div>
          </main>
        </div>
      </div>
    </SecondaryContext.Provider>
  );
}
