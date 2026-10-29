# 06 – Rezepte: so baust du die häufigen Dinge

Jedes Rezept nennt **Vorbilder** (öffne sie zuerst) und die Schritte in fester Reihenfolge. Die Standards dazu stehen in `01`–`04`.

## A. Neue Einstellung für einen Server (Schalter oder Wert)

Vorbild: Auto-Restart – Backend `putAutoRestart` (`internal/panel/events.go`), Store `SetAutoRestart` (`store_more.go`),
Oberfläche `components/server/settings.tsx`.

1. **Spalte:** Eintrag in `moreColumns` (`store_more.go`), z. B. `{"servers", "quiet_mode", "INTEGER NOT NULL DEFAULT 0"}`.
2. **Zeile/DTO:** Feld in `ServerRow` (`store.go`) lesen/schreiben (SELECT-Liste und `Scan` beachten) und in `ServerDTO`, falls die Oberfläche es braucht.
3. **Store:** `SetQuietMode(serverID string, on bool) error`.
4. **Handler:** `putQuietMode` nach dem Handler-Muster (`04` §3), Stufe `rankAdmin`. Route in `panel.go`.
5. **Test:** in `internal/panel/*_test.go` — Erfolg, Betrachter/Operator bekommt 403, fremdes Projekt 404, unbekanntes Feld 400.
6. **Typ:** Feld in `Server` (`web/src/lib/types.ts`).
7. **Oberfläche:** Karte/Zeile in `settings.tsx` mit `Switch` + `toast.run(() => api.put(...), "…")` + `onDone()`.
8. **Demo:** Zweig in `web/demo/server.mjs` (`if (c === "quiet-mode") …`), Beispielwert in `makeServer`/`projects`.
9. Doku (README „Was geht“), dann „Fertig heißt“ (`05` §2).

## B. Neuer Endpunkt + Liste in der Oberfläche (z. B. neue Art Eintrag mit Anlegen/Löschen)

Vorbild: Webhooks – `events.go` (Handler `listWebhooks`, `createWebhook`, `updateWebhook`, `deleteWebhook`), `store_more.go` (CRUD),
`web/src/components/events.tsx` (`WebhooksCard`).

1. Tabelle in `schemaMore`, Struct mit `json`-Tags, Store-Methoden (Create, List, ByID, Update, Delete).
2. Handler je Aktion; Liste `rankViewer`/`rankOperator`, Ändern `rankAdmin`; Hilfsfunktion `requireXyz` (wie `requireWebhook`), die das Objekt
   lädt **und** die Projektrechte prüft. Eingaben streng validieren (Länge, Muster, Anzahl pro Projekt begrenzen).
3. Routen in `panel.go` (REST: `GET/POST /api/projects/{id}/xyz`, `PATCH/DELETE /api/xyz/{id}`).
4. Tests inkl. Rechte und Grenzen.
5. Typ in `types.ts`; Komponente `XyzCard` mit den vier Zuständen, Zeilen mit `RowMenu`, Anlegen/Bearbeiten im `Dialog`, Löschen im `ConfirmDialog`.
6. In `project/page.tsx` als Tab/Karte einhängen (Rang beachten).
7. Demo-Endpunkte + Beispieldaten; Doku.

## C. Neue Aktion an einem Server, die der Node ausführen muss

Vorbild: Modpack – Node `internal/node/modpack.go` + Routen in `api_extra.go`, Panel-Handler in `handlers_extra.go`, Rang in `requiredRank`,
Oberfläche `components/server/modpack.tsx`.

1. **Node:** Logik in eigener Datei (`internal/node/xyz.go`), nur über `FS`/`Runtime`; Route in `api.go`/`api_extra.go`; Test mit Attrappen.
2. **Panel:** Rang in `requiredRank` eintragen **oder** eigener Handler, der `NodeClient.Do` aufruft (dann Rechte selbst prüfen).
3. `fakeNode` in `panel_test.go` um die Route erweitern; Panel-Test.
4. Oberfläche und Demo wie in A/B.
5. Dauert die Aktion: Fortschritt als Status-Endpunkt (`state: idle|running|done|failed`, `done`, `total`) und in der Oberfläche per `refreshInterval` abfragen.

## D. Neue Hauptseite

Siehe `03` §6. Vorbild: `app/(app)/audit/page.tsx` (Liste mit Filter) oder `nodes/page.tsx` (Karten).

## E. Neue Ereignisart (Verlauf + Webhook)

Vorbild: `kind: "lowmemory"` in `events.go`.

1. An der Stelle, wo es passiert: `p.emit(projectID, serverID, "meinkind", "warn", "Deutscher Satz mit Namen und Zahl")`.
2. Kennung in `eventKinds` (Validierung) und die Beschriftungstabelle in `events.go` (ca. Zeile 100) aufnehmen, bei Bedarf in `defaultHookEvents`, und in `web/src/components/events.tsx` (Auswahl + Beschriftung).
3. Test in `events_test.go`; Demo-Beispiel.

## F. Neue Server-Software (z. B. weiterer Fork)

Vorbild: `purpur` in `internal/software/software.go` und `software_test.go`.

1. `ID`-Konstante, Eintrag im Katalog (Name, Art `server|proxy`, Erweiterungen `plugins|mods|none`, Standardport), Versionsliste, Download-URL,
   Prüfsumme, Java-Regel. Alles aus der offiziellen Quelle des Herstellers, mit Prüfsumme.
2. Test mit `httptest`-Attrappe der Herkunft.
3. Autoconfig prüfen (`network.go`, `internal/node/autoconfig.go`): braucht die Software eigene Weiterleitungs-Einstellungen?
4. Oberfläche: erscheint automatisch über `/api/software`; Beschriftung der Erweiterungen in `server/page.tsx` (`extensions`) prüfen; Demo-Liste ergänzen.

## G. Neue Karte im Konto oder in den Einstellungen

Vorbild: `app/(app)/account/page.tsx` (`KeysCard`). Eigene Komponente in derselben Datei oder daneben, `Card` + `CardHeader`, eigene `useApi`-Abfrage,
Aktionen mit `toast.run`. Backend-Routen für Konto-Dinge hinter `sessionOnly`.

## H. Text, Farbe oder Abstand korrigieren

Nur `01` und `02` befragen, die vorhandene Komponente ändern (dort an **einer** Stelle), nicht jede Seite einzeln. Danach `next build` und alle
Seiten, die die Komponente nutzen, gedanklich prüfen (`Grep` nach dem Komponentennamen).

## Mini-Checkliste vor dem Absenden jeder Antwort

- Habe ich ein Vorbild geöffnet und befolgt?
- Nur vorhandene Bausteine/Farben/Bibliotheken? Deutsch, du?
- Rechte im Backend? Test geschrieben? Datenbank additiv?
- Vier Zustände? Rückmeldung per Toast? Gefährliches mit Rückfrage?
- Typen, Demo, Doku mitgezogen? Befehle wirklich ausgeführt und Ergebnis korrekt berichtet?
