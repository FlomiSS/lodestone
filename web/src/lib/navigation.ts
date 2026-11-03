/**
 * Bereiche, deren Unterseiten es im statischen Export nicht als eigene Route gibt.
 * Die Seiten lesen ID und Reiter aus dem Pfad, ausgeliefert wird aber immer die Basisroute.
 */
const DEEP_SECTIONS = new Set(["project", "server", "nodes"]);

export interface DeepTarget {
  /** Erster Pfadabschnitt, z. B. "project". */
  section: string;
  /** Echte Route des statischen Exports, z. B. "/project/". */
  base: string;
  /** Suchparameter für die echte Route, z. B. "?id=…&tab=…". */
  search: string;
  /** Adresse, die im Browser sichtbar bleibt, z. B. "/project/<id>/<tab>/". */
  pretty: string;
}

/**
 * Zerlegt einen schönen Pfad wie /project/<id>/<tab>/ in die Basisroute, die Next.js
 * tatsächlich kennt. Liefert null für alle anderen Adressen (auch fremde Hosts).
 */
export function parseDeepTarget(href: string): DeepTarget | null {
  const url = new URL(href, "http://lodestone.invalid");
  if (url.origin !== "http://lodestone.invalid") return null;

  const segments = url.pathname.split("/").filter(Boolean);
  if (segments.length < 2) return null;

  const [section, id, slot] = segments;
  if (!DEEP_SECTIONS.has(section)) return null;

  const params = new URLSearchParams();
  for (const [key, value] of url.searchParams) {
    if (key !== "id" && key !== "tab") params.append(key, value);
  }
  params.set("id", id);

  const tab = url.searchParams.get("tab") ?? slot;
  if (tab) params.set("tab", tab);

  const search = params.toString();
  return {
    section,
    base: `/${section}/`,
    search: search ? `?${search}` : "",
    pretty: `${url.pathname}${url.search}${url.hash}`,
  };
}
