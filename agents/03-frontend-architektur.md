# 03 – Frontend-Architektur (`web/`)

## 1. Technik (fest)

Next.js 15 (App Router) mit `output: "export"` + `trailingSlash: true` → **statische Dateien**, die das Go-Panel aus dem Binary ausliefert
(`internal/panel/webui/dist`). React 19, TypeScript (strikt), Tailwind 3, SWR, lucide-react, CodeMirror 6, qrcode.
Das hat Folgen:

- Es gibt **keinen Next-Server**: keine API-Routen, keine Server Actions, keine Serverkomponenten mit Daten, kein `next/image`-Optimizer,
  keine Middleware, keine dynamischen Segmente (`[id]`). IDs reisen als **Suchparameter**: `/server/?id=…&tab=…`, `/project/?id=…`.
- Jede Seite beginnt mit `"use client"`. Seiten, die `useSearchParams` nutzen, stecken in `<Suspense>` (siehe `server/page.tsx`).
- Links zwischen Seiten: `next/link` mit abschließendem Schrägstrich (`/nodes/`, `/project/?id=…`).
- Entwicklung: Oberfläche auf Port 3100, Panel auf 8480; `apiBase()` in `lib/api.ts` kümmert sich darum. Nichts hartkodieren.

## 2. Ordner

```
web/src/
  app/
    layout.tsx, globals.css         Wurzel, Design-Variablen
    login/page.tsx                  ohne Shell
    status/page.tsx                 öffentlich, ohne Shell
    (app)/                          alles mit Anmeldung + Shell (layout.tsx prüft die Anmeldung)
      page.tsx                      Übersicht
      project/page.tsx, server/page.tsx, nodes/, users/, audit/, account/
  components/
    ui/                             Grundbausteine (primitives, feedback, async, chart, mc)  ← hier KEINE Fachlogik
    layout/                         shell.tsx (Seitenleisten), palette.tsx (Strg+K)
    server/                         Bereiche eines Servers (console, files, addons, backups, players, settings, stats …)
    *.tsx                           Fachkomponenten auf Projektebene (add-server, groups, schedules, events, motd, status-settings, status)
  lib/
    api.ts                          request/api/useApi/upload/downloadUrl/fetchText/putText, wsBase
    types.ts                        Typen der Schnittstelle + roleRank
    format.ts                       bytes, megabytes, dateTime, ago, uptime, compact, stateTone …
    audit.ts, crash.ts              reine Hilfen (Beschriftungen, Fehlererkennung)
web/demo/server.mjs                 Schein-Backend für die Demo (Port 3100)
```

Regeln für Orte:
- Eine **Seite** (`app/**/page.tsx`) ist dünn: Daten holen, Zustände (`Loading`/`ErrorState`), Aufbau, Fachkomponenten einsetzen.
  Wird sie länger als ~250 Zeilen, wandert ein Block in eine Komponente unter `components/`.
- **Fachkomponenten** liegen bei dem, was sie betreffen: Serverbereich → `components/server/`, Projekt → `components/`.
- **`components/ui/`** enthält nur Fachfreies. Nichts importiert dort aus `lib/types` Fachtypen (außer Tonnamen).
- **Reine Logik ohne React** (Parsen, Erkennen, Beschriften) → `lib/`.
- Pfadalias `@/` zeigt auf `web/src/`. Immer so importieren, nie `../../..`. (Innerhalb eines Ordners ist `./` ok.)

## 3. Daten laden und senden

```tsx
import { api, useApi } from "@/lib/api";
import type { Schedule } from "@/lib/types";

const { data, error, mutate } = useApi<Schedule[]>(`/projects/${project.id}/schedules`, { refreshInterval: 15000 });
// Pfad ohne "/api"-Präfix. Mit `null` als Pfad wird nichts geladen (bedingte Abfrage).

const toast = useToast();
await toast.run(() => api.post(`/projects/${project.id}/schedules`, body), "Zeitplan angelegt");
mutate();
```

- `useApi` = SWR mit `credentials: include`; `api.get/post/put/patch/delete` für Aktionen; `ApiError` trägt `status` und die deutsche Meldung.
- 401 leitet automatisch zur Anmeldung. 204 liefert `undefined`.
- Dateien: `upload()` (mit Fortschritt), `downloadUrl(pfad)` für Download-Links, `fetchText/putText` für Textdateien.
- Konsole: Websocket über `wsBase()`; nur dort.
- Kein globaler Store (kein Redux/Zustand). Serverdaten = SWR-Cache, Formularzustand = `useState` in der Komponente, URL = Tabs/IDs.
- Eigene Hooks nur, wenn sie mindestens zweimal gebraucht werden (Beispiel `usePlayerActions` in `components/server/player-admin.tsx`).

## 4. Typen

- `lib/types.ts` spiegelt die JSON-Antworten **Feld für Feld** (camelCase wie im Go-`json:"…"`-Tag). Neue Antwort im Backend → derselbe
  Commit ergänzt den Typ. Optionale Felder (`omitempty` im Go) sind `?:` im Typ.
- Kein `any`. Kein `as`-Raten bei Antworten; Typ an `useApi<T>`/`api.get<T>` übergeben.
- Zustandsnamen sind Vereinigungstypen (`"running" | "offline" | …`), keine freien Strings.

## 5. Komponenten schreiben

- Funktionskomponenten, benannte Exporte (Seiten: `export default`). Props als Objekt mit Typ direkt an der Funktion.
- Klassen mit `cx(...)` aus `ui/primitives` zusammensetzen, keine Klassen-Strings mit Template-Verkettung aus Nutzereingaben.
- Kein `dangerouslySetInnerHTML`. Nutzer- und Servertexte werden als Text gerendert. MOTD-Vorschau parst MiniMessage selbst (`motd.tsx`).
- Ein Effekt pro Zweck, Abhängigkeiten vollständig. Aufräumen (Timer, Websocket, Listener) im Rückgabewert.
- Zugriff auf `window`/`document`/`localStorage` nur in Effekten oder Ereignissen (die Seite wird beim Bauen einmal vorgerendert).
- Listen mit stabilem `key` (ID, nicht Index, wenn sich die Liste ändert).
- Kommentare auf Deutsch, nur für das **Warum** („Der Kontext hängt an globalThis, weil …“). Kein Kommentar, der den Code wiederholt.

## 6. Rezept: neue Seite

1. Datei `web/src/app/(app)/<name>/page.tsx`, `"use client"`.
2. Typen in `lib/types.ts`, Daten mit `useApi`.
3. Aufbau nach `02` (PageHeader → Zustände → Karten). Rechte: `const user = useUser();` (aus `components/layout/shell`) für `isAdmin`.
4. Link in der Seitenleiste (`shell.tsx`, im passenden Abschnitt, mit lucide-Icon) **und** Eintrag in `palette.tsx`.
5. Demo: Endpunkte in `web/demo/server.mjs` ergänzen, damit die Seite dort funktioniert.
6. Prüfen: `npx tsc --noEmit`, `npx next build`.

## 7. Rezept: neuer Bereich im Server oder neuer Tab im Projekt

Vorbild `components/server/backups.tsx` (einfach) oder `players.tsx` (mit Diagrammen). Komponente `XyzTab({ server | project, canWrite })`,
Eintrag im `entries`-Array von `server/page.tsx` (Rang beachten) bzw. im Tab-Array von `project/page.tsx`, Anzeige per `{active === "xyz" && <XyzTab … />}`.

## 8. Nicht tun

- Keine neue Bibliothek (UI-Kits, Formularbibliotheken, Icon-Sets, Datumsbibliotheken, Zustandsverwaltung, CSS-in-JS).
- Kein `fetch` außerhalb von `lib/api.ts`. Keine API-Adressen mit festem Host.
- Kein Inline-`style`, außer für berechnete Werte (Breite eines Balkens, Pixelgrößen von Avataren).
- Keine Server-Komponenten, `generateStaticParams`, `useRouter().push` auf dynamische Pfade.
- Kein Umbenennen oder Verschieben bestehender Routen (Lesezeichen und die öffentliche Statusadresse `/status/?p=` müssen bleiben).
- Keine Texte in Englisch, keine Zahlenformatierung von Hand (`lib/format`).
