# 01 – Visuelles Design der Weboberfläche

Quelle der Wahrheit: `web/src/app/globals.css` (Variablen), `web/tailwind.config.ts` (Farbnamen) und
`web/src/components/ui/*.tsx` (Bausteine). Dieses Dokument beschreibt sie und sagt, **wie** man sie benutzt.

## 1. Grundhaltung

Lodestone sieht aus wie ein ruhiges, modernes Entwickler-Werkzeug (Linear, Vercel, Observer): **dunkel, flach, dicht, ein
Akzent**. Nichts blinkt, nichts glänzt. Inhalte stehen in Karten auf dunklem Grund, Trennung geschieht über feine Linien
(`white/[0.06]`) und kleine Helligkeitsunterschiede, nicht über Farbe oder Schatten. Farbe bedeutet immer etwas (Zustand, Aktion).
Minecraft kommt nur über das **Pixel-Logo** und die **Pixel-Avatare** vor (`components/ui/mc.tsx`), nie über Texturen oder Schriften.

## 2. Farben

Variablen in `:root` (RGB-Tripel, werden mit Tailwind-Alpha benutzt, z. B. `bg-accent/10`):

| Name (Tailwind) | Wert | Wofür |
| --- | --- | --- |
| `bg` | 17 17 17 (#111111) | Seitenhintergrund |
| `surface` | 24 24 24 | Karten, Dialoge |
| `raised` | 32 32 32 | Erhöhte Flächen (Menüs, Avatar-Hintergrund) |
| `line` | 40 40 40 | Standard-Rahmenfarbe (`*` hat sie) |
| `ink` | 237 237 239 | Haupttext |
| `muted` | 140 140 146 | Nebentext, Beschriftungen, Icons |
| `accent` | 59 130 246 (Blau) | Primäre Aktion, Auswahl, Fokus, Links |
| `good` | 34 197 94 | Läuft, Erfolg |
| `warn` | 245 158 11 | Wartet, Warnung, wenig Ressourcen |
| `bad` | 239 68 68 | Fehler, Absturz, Gefahr |
| `info` | 96 165 250 | Hinweis, Installation läuft |

Weiße Überlagerungen für Flächen und Linien (das ist die Art, wie Tiefe entsteht):

- Fläche: `bg-white/[0.025]` (Eingaben), `bg-white/[0.03]`–`[0.04]` (Knöpfe, Tabs), `hover:bg-white/[0.05–0.06]`, aktiv `bg-white/[0.07–0.09]`.
- Linie: `border-white/[0.06]` (innen, zwischen Zeilen), `border-white/[0.07]` (Karten), `border-white/[0.08–0.09]` (Eingaben, Knöpfe).

**Regeln**
- Ein Zustand hat **immer** dieselbe Farbe: läuft = `good`, startet/installiert = `info`, wartet/Warnung = `warn`, aus = `muted`
  (neutral), Fehler = `bad`. Benutze `Badge tone="…"` oder `Dot tone="…"`, nicht eigene Farbklassen.
- Text ist `text-ink` (Standard, muss nicht gesetzt werden) oder `text-muted`. Dazwischen `text-ink/80`, `text-ink/55` für
  Abstufungen. Nie reines Weiß für Fließtext, nie graue Hex-Werte.
- Der Akzent erscheint **sparsam**: ein primärer Knopf pro Ansicht, Auswahlzustände, Fokusringe, Links (`text-accent hover:underline`).
- Gefahr = `bad` (rot) nur für Löschen/Zurücksetzen/Abstürze, nicht für gewöhnliche Hinweise.
- Verboten: Hex-Farben, `bg-blue-500`, `text-red-600` und ähnliche Tailwind-Paletten im normalen Code. Ausnahmen, die es schon gibt:
  die getönten Kennzahlkarten (`StatCard`, dort ist die Palette fest) und die Diagrammfarben (`seriesColors` in `ui/chart.tsx`).

## 3. Schrift

Schrift: Geist (Fallback Inter/System), Code `ui-monospace`. Größen (feste Stufen, keine anderen benutzen):

| Zweck | Klassen |
| --- | --- |
| Seitentitel | `text-xl font-semibold tracking-tight` (macht `PageHeader`) |
| Kartentitel | `text-[13px] font-semibold tracking-tight` (macht `CardHeader`) |
| Dialogtitel | `text-base font-semibold` (macht `Dialog`) |
| Fließtext, Eingaben | `text-sm` |
| Beschreibung unter Titeln | `text-[13px] text-muted` (Seite), `text-[11px] text-muted` (Karte), `text-xs text-muted` (Dialog) |
| Beschriftung über Eingaben | Klasse `label` (`text-xs font-medium text-muted`) |
| Abzeichen | `text-[10.5px] font-medium` (macht `Badge`) |
| Kennzahlen | `text-[26px] font-semibold leading-none tracking-tight` (macht `StatCard`) |
| Adressen, Befehle, Dateinamen, Logs | `font-mono` (Konsole: Klasse `console-text`) |

Zahlen in Tabellen und Anzeigen: `tabular-nums`. Lange Texte kürzen mit `truncate` (und `min-w-0` am Flex-Kind).

## 4. Abstände, Rundung, Rahmen

- Abstandsstufen (Tailwind): 1, 1.5, 2, 3, 4, 5, 6. Karteninnenabstand `px-5 py-4`, Raster `gap-4` (Karten) / `gap-5` (Bereiche),
  vertikale Gruppen `space-y-4` bzw. `space-y-5`. Keine Werte wie `p-[13px]`.
- Rundung: Karten/Dialoge `rounded-xl`, Knöpfe/Eingaben/Zeilen `rounded-lg`, Kleinteile `rounded-md`, Abzeichen/Schalter `rounded-full`.
- Rahmen: immer 1 px, immer halbtransparentes Weiß (siehe oben). Nie farbige Rahmen außer bei Zustandskarten (`border-bad/25`,
  `border-good/25` mit gleichfarbiger `bg-…/10`-Fläche).
- Seitenbreite: Inhalt höchstens `max-w-[1480px]`, Seitenrand `px-4 lg:px-10` (macht die Shell – nicht selbst setzen).

## 5. Bausteine (alle in `web/src/components/ui/`) und ihr Einsatz

| Baustein | Einsatz | Wichtig |
| --- | --- | --- |
| `Button` (`variant`: primary, secondary, ghost, danger; `size`: sm, md, lg, icon, icon-sm; `loading`) | Jede Aktion | Primär = höchstens einer pro Ansicht. Aus Tabellenzeilen: `size="sm"` oder `ghost`. Nur-Icon-Knöpfe **brauchen** `aria-label`. |
| `Card`, `CardHeader` (`title`, `description`, `action`) | Jeder Inhaltsblock | Kartenkopf immer mit `CardHeader`, Inhalt darunter mit `px-5 py-4`. |
| `PageHeader` (`title`, `description`, `action`) | Oben auf jeder Seite | Der Hauptknopf der Seite kommt in `action`. |
| `StatCard` (`tone`: green, blue, violet, amber, red, neutral) | Kennzahlen im Raster `grid gap-4 sm:grid-cols-2 xl:grid-cols-4` | Ton nach Bedeutung, nicht nach Geschmack. |
| `Badge`, `Dot` | Zustände, Rollen, kleine Etiketten | Tonnamen: neutral, good, warn, bad, info, accent. |
| `Tabs` | Umschalten zwischen 2–6 Ansichten derselben Sache | Mit `count` für Anzahlen. Auf Seiten mit vielen Bereichen (Server) stattdessen die zweite Seitenleiste. |
| `Switch` | Ein/Aus mit sofortiger Wirkung | Mit Beschriftung daneben, in `rounded-lg border border-white/[0.07] bg-white/[0.02] px-3 py-2.5`. |
| `Checkbox`, `SearchInput` | Auswahl, Suche | Suche immer mit `SearchInput`. |
| `RowMenu` | „⋯“-Menü für Zeilenaktionen | Gefährliches mit `danger: true`, durch `"separator"` abgetrennt. |
| `Meter`, `loadTone` | Auslastung als Balken | Farbe aus `loadTone(prozent)` (ab 75 % gelb, ab 90 % rot). |
| `EmptyState` | Leere Liste/Karte | Icon + kurzer Titel + ein Satz, was zu tun ist. |
| `Loading`, `ErrorState` (`ui/async.tsx`) | Laden, Fehler | `ErrorState` bekommt `retry`. |
| `Dialog`, `ConfirmDialog`, `useToast` (`ui/feedback.tsx`) | Eingaben im Dialog, Rückfragen, Rückmeldungen | `wide` nur für große Inhalte. |
| `SeriesChart`, `ChartLegend` (`ui/chart.tsx`) | Zeitreihen | Farben aus `seriesColors`, nicht eigene. |
| `LodestoneLogo`, `Wordmark`, `PixelAvatar` (`ui/mc.tsx`) | Marke, Benutzer-/Projekt-Avatare | `PixelAvatar name=…` ist deterministisch. |
| Klassen `field`, `label`, `panel`, `row-hover` (`globals.css`) | Eingabefelder, Beschriftungen, Zeilen-Hover | `<input className="field">`, `<label className="label">`. |

Fehlt dir ein Baustein: erst prüfen, ob sich der Bedarf mit den vorhandenen lösen lässt (meist ja). Wenn wirklich nötig, ergänze ihn in
`ui/primitives.tsx` im selben Stil (gleiche Tonnamen, `cx`, Tastaturbedienung), nicht in der Seite, und trage ihn hier ein.

## 6. Zustände und Interaktion

- Jedes klickbare Element hat Hover (`hover:bg-white/[0.05]`) und Druckeffekt (`active:scale-[0.95–0.97]`) – wie die Bausteine.
- Fokus: Eingaben zeigen `focus:border-accent/60 focus:ring-2 focus:ring-accent/20` (Klasse `field`). Nie `outline-none` ohne Ersatz.
- Deaktiviert: `disabled:opacity-50` (macht `Button`), plus kurzer Grund als Text oder `title`, wenn nicht offensichtlich.
- Übergänge: `transition duration-100` bis `200`. Seiten blenden mit `animate-fade-in` ein (Shell). Keine weiteren Animationen.
- Mindest-Klickfläche: Icon-Knöpfe `h-8 w-8` (`icon-sm`) oder `h-10 w-10` (`icon`).

## 7. Layout-Muster

- **Shell** (`components/layout/shell.tsx`): linke Hauptleiste (240 px, einklappbar auf 68 px), optional zweite Leiste (232 px) über
  `SecondarySidebar`, Inhalt in einer abgerundeten Fläche `#151515`. Die Seitenleisten scrollen **nie** mit, nur der Inhalt.
- **Raster**: `grid gap-4 lg:grid-cols-2` für Karten nebeneinander, `grid gap-4 sm:grid-cols-2 xl:grid-cols-4` für Kennzahlen.
  Mobil immer eine Spalte, nichts darf seitlich überlaufen.
- **Listenzeile**: `flex items-center gap-3 px-5 py-2.5`, Trenner `divide-y divide-white/[0.05]`, Name `min-w-0 flex-1 truncate`,
  Aktionen rechts.
- **Formular im Dialog**: `Dialog` mit `<div><label className="label">…</label><input className="field" /></div>` je Feld, zwei Felder
  nebeneinander mit `grid gap-3 sm:grid-cols-2`, Hilfetext darunter `text-xs text-muted`.
- **Hinweis-/Fehlerkasten**: `rounded-lg border border-bad/25 bg-bad/10 px-4 py-3 text-xs text-bad` (analog `warn`, `info`, `good`).

## 8. Barrierefreiheit und Mobil

- Kontrast: Text auf Flächen nur `ink` oder `muted`, nie dunkleres Grau. Zustand nie **nur** durch Farbe: immer Text oder Icon dazu.
- Alle Icon-Knöpfe `aria-label` (deutsch), Dialoge schließen mit Esc, Menüs ebenso.
- Unter `lg` klappt die Hauptleiste in ein Menü; prüfe jede neue Seite gedanklich bei 360 px Breite (Tabellen: `overflow-x-auto`).

## 9. Typische Fehler schwacher Modelle (und die Lösung)

| Falsch | Richtig |
| --- | --- |
| `className="bg-blue-600 text-white rounded px-4 py-2"` | `<Button variant="primary">` |
| `<div className="bg-gray-800 rounded p-4 shadow-lg">` | `<Card>` mit `CardHeader` |
| `<span className="text-green-500">läuft</span>` | `<Badge tone="good">läuft</Badge>` |
| `alert("Gespeichert")`, `confirm("Sicher?")` | `toast.run(...)`, `ConfirmDialog` |
| Eigener Lade-Kringel oder „Loading…“ | `<Loading />` |
| Leere Fläche, wenn es nichts gibt | `<EmptyState icon title text />` |
| Englische Texte („Save“, „Delete“, „Error“) | „Speichern“, „Löschen“, Meldung mit Ursache und Lösung |
| Neues Icon-Set, Emoji | `lucide-react` |
| `style={{ color: "#3b82f6" }}` | `text-accent` |
| Heller Hintergrund, Dark/Light-Umschalter | Es gibt nur das dunkle Design |
