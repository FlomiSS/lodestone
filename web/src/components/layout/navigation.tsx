"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { createContext, useCallback, useContext, useEffect, useRef, type ReactNode } from "react";
import { parseDeepTarget, type DeepTarget } from "@/lib/navigation";

interface AppNavigation {
  push: (href: string) => void;
  replace: (href: string) => void;
}

const NavigationContext = createContext<AppNavigation | null>(null);

/** Navigation, die schöne Pfade wie /project/<id>/<tab>/ clientseitig hält. */
export function useAppNavigation(): AppNavigation {
  const navigation = useContext(NavigationContext);
  if (!navigation) throw new Error("useAppNavigation braucht den NavigationProvider");
  return navigation;
}

function normalize(path: string): string {
  const p = path.split("?")[0].replace(/\/+$/, "");
  return p ? p + "/" : "/";
}

function paramsMatch(actual: URLSearchParams, targetSearch: string): boolean {
  const target = new URLSearchParams(targetSearch);
  for (const [key, value] of target.entries()) {
    if (actual.get(key) !== value) return false;
  }
  return true;
}

/**
 * Next.js kennt im statischen Export nur die Basisrouten (/project/, /server/, /nodes/).
 * Ein Klick auf /project/<id>/<tab>/ wäre deshalb ein vollständiger Seitenaufruf. Dieser
 * Provider navigiert intern auf die Basisroute mit Suchparametern und schreibt danach den
 * schönen Pfad zurück.
 */
export function NavigationProvider({ children }: { children: ReactNode }) {
  const router = useRouter();
  const pathname = usePathname() ?? "/";
  const searchParams = useSearchParams();
  const pending = useRef<DeepTarget | null>(null);

  const push = useCallback(
    (href: string) => {
      const target = parseDeepTarget(href);
      if (!target) {
        pending.current = null;
        router.push(href);
        return;
      }
      const isSameBase = normalize(pathname) === normalize(target.base);
      if (isSameBase && paramsMatch(searchParams, target.search)) {
        window.history.replaceState({ ...window.history.state, __NA: true }, "", target.pretty);
        return;
      }
      pending.current = target;
      router.push(target.base + target.search);
    },
    [pathname, router, searchParams],
  );

  const replace = useCallback(
    (href: string) => {
      const target = parseDeepTarget(href);
      if (!target) {
        pending.current = null;
        router.replace(href);
        return;
      }
      const isSameBase = normalize(pathname) === normalize(target.base);
      if (isSameBase && paramsMatch(searchParams, target.search)) {
        window.history.replaceState({ ...window.history.state, __NA: true }, "", target.pretty);
        return;
      }
      pending.current = target;
      router.replace(target.base + target.search);
    },
    [pathname, router, searchParams],
  );

  // Sobald Next.js auf der Basisroute angekommen ist, wird die Adresse wieder schön.
  useEffect(() => {
    const target = pending.current;
    if (!target) return;
    if (normalize(pathname) !== normalize(target.base)) return;
    if (!paramsMatch(searchParams, target.search)) return;
    pending.current = null;
    window.history.replaceState({ ...window.history.state, __NA: true }, "", target.pretty);
  }, [pathname, searchParams]);

  // Synchronisiert den Next.js-Router bei Vor- und Zurück-Navigation im Browser.
  useEffect(() => {
    function onPopState() {
      const target = parseDeepTarget(window.location.pathname + window.location.search);
      if (target) {
        pending.current = target;
        router.replace(target.base + target.search);
      }
    }
    window.addEventListener("popstate", onPopState);
    return () => window.removeEventListener("popstate", onPopState);
  }, [router]);

  // Fängt Klicks auf schöne Pfade ab, bevor next/link sie als Seitenaufruf behandelt.
  useEffect(() => {
    function onClick(event: MouseEvent) {
      if (event.defaultPrevented || event.button !== 0) return;
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
      if (!(event.target instanceof Element)) return;
      const anchor = event.target.closest("a");
      if (!anchor) return;
      if (anchor.target && anchor.target !== "_self") return;
      if (anchor.hasAttribute("download")) return;
      const href = anchor.getAttribute("href");
      if (!href || !parseDeepTarget(href)) return;
      event.preventDefault();
      push(href);
    }
    document.addEventListener("click", onClick, true);
    return () => document.removeEventListener("click", onClick, true);
  }, [push]);

  return <NavigationContext.Provider value={{ push, replace }}>{children}</NavigationContext.Provider>;
}
