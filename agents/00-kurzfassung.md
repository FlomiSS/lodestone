# 00 – Kurzfassung: die harten Regeln

Eine Seite. Jede Regel hat einen Grund und steht in den anderen Dokumenten genauer.

## Sprache und Ton
1. Alles, was ein Mensch liest (Oberfläche, Fehlermeldungen, Kommentare, Commit-Texte, Doku), ist **Deutsch** mit der Anrede **du**.
   Bezeichner im Code (Variablen, Funktionen, Typen, Dateinamen) sind **Englisch**.
2. Kurze, konkrete Sätze. Fehlermeldungen sagen, **was los ist und was man tun kann**: „Der Port ist schon belegt. Prüfe unter
   Einstellungen die Anschlüsse.“ Nie „Fehler“, „Ein Fehler ist aufgetreten“ oder englische Systemtexte zeigen.
3. Keine Emojis in der Oberfläche. Symbole kommen aus `lucide-react`.

## Aussehen (Details: `01`)
4. **Dunkles Design, ein Akzent (Blau).** Farben nur über die Tailwind-Namen `bg`, `surface`, `raised`, `line`, `muted`, `ink`,
   `accent`, `good`, `warn`, `bad`, `info` oder die festen Weiß-Töne `white/[0.0x]`. **Keine** Hex-Werte, **keine** neuen
   Tailwind-Farben (wie `indigo-500`) außerhalb von `StatCard` und Diagrammen.
5. Karten sind `Card` (+ `CardHeader`), Knöpfe `Button`, Eingaben die Klasse `field`, Beschriftungen die Klasse `label`, Zustände
   `Badge`/`Dot`, Tabs `Tabs`, leere Listen `EmptyState`, Laden `Loading`, Fehler `ErrorState`, Dialoge `Dialog`/`ConfirmDialog`.
   **Baue keine eigene Variante davon.**
6. Eckig-weich: Karten `rounded-xl`, Knöpfe/Eingaben `rounded-lg`, Abzeichen `rounded-full`. Rahmen immer `border-white/[0.07]`
   (Karten) bzw. `border-white/[0.08]` (Eingaben). **Keine Schatten außer** `shadow-panel` und dem `shadow-glow` des primären Knopfs.
7. Keine Verläufe, Leuchteffekte oder Animationen außer dem, was `primitives.tsx` schon mitbringt.

## Ablauf (Details: `02`)
8. Jede Aktion bekommt eine Rückmeldung: `toast.run(() => api…, "Erledigt")`. Gefährliches (Löschen, Zurücksetzen, Überschreiben)
   läuft **immer** über `ConfirmDialog` mit rotem Knopf und benennt, was verloren geht.
9. Jede Datenansicht hat vier Zustände: **lädt** (`Loading`), **Fehler** (`ErrorState` mit „Erneut versuchen“), **leer**
   (`EmptyState` mit Erklärung und, wenn möglich, dem Knopf zum Anlegen), **voll**. Alle vier bauen, nie nur den vollen.
10. Rechte sichtbar machen: Was ein Benutzer nicht darf, wird **ausgeblendet oder deaktiviert** (Rang prüfen), aber die echte
    Prüfung macht das Backend.

## Frontend (Details: `03`)
11. Next.js als **statischer Export**: keine Serverkomponenten, keine API-Routen, keine dynamischen Pfade wie `/server/[id]`.
    Seiten haben `"use client"` und lesen IDs aus Suchparametern (`/server/?id=…`).
12. Daten nur über `useApi<T>(pfad)` (lesen) und `api.get/post/put/patch/delete` (schreiben) aus `@/lib/api`. Nie `fetch` direkt,
    außer in `lib/api.ts`. Typen der Schnittstelle stehen in `lib/types.ts` und spiegeln die JSON-Namen des Backends.
13. Keine neue npm-Abhängigkeit, ohne dass es der Besitzer verlangt hat. Vorhanden sind: next, react, swr, lucide-react,
    tailwind, CodeMirror, qrcode.

## Backend (Details: `04`)
14. Eine Schicht pro Aufgabe: **Handler** (`handlers_*.go`) prüft Rechte und Eingabe, **Store** (`store*.go`) spricht mit SQLite,
    **NodeClient** (`nodeclient.go`) spricht mit dem Node. Kein SQL im Handler, kein HTTP im Store.
15. Jeder Handler: Rang prüfen (`requireProject`/`requireServer`/`requireAdmin`) → Eingabe lesen (`decode`, strikt) → prüfen →
    handeln → `writeJSON` oder `writeError`. Fehler als `fail(status, "deutsche Meldung")`.
16. Datenbank nur **additiv** ändern (`ensureColumn`, neue Tabellen in `schemaMore`). Nie Spalten löschen oder umbenennen.
17. Sicherheit zuerst: Pfade nur über `os.Root`/die vorhandenen Prüfungen, Namen und Adressen mit strenger Regex, nie Eingaben in
    Shell-Befehle oder Dateinamen einsetzen, nie Geheimnisse loggen oder an den Browser schicken.
18. Jede neue Backend-Funktion bekommt einen Test (`*_test.go`, Docker wird durch Attrappen ersetzt).

## Arbeit und Ehrlichkeit (Details: `05`)
19. Erst ein Vorbild im Code suchen, dann bauen. Kleinste Änderung, die die Aufgabe löst. Keine Umbauten „nebenbei“.
20. Fertig heißt: `gofmt`, `go vet ./...`, `go test ./...`, `npx tsc --noEmit` (in `web/`) laufen sauber; bei UI-Änderungen
    `npx next build`. Demo-Backend (`web/demo/server.mjs`) für neue Endpunkte ergänzen. Doku mitziehen.
21. Berichte ehrlich: was gebaut, was **getestet**, was **nicht** getestet, was offen ist. Nie „sollte funktionieren“ als „funktioniert“.
22. **Nicht anfassen:** Pelican/Wings-Server des Besitzers (Ports 25565–25575, 25595), Geheimnisse, Dokploy-Umgebungswerte.
    **Nicht tun ohne Auftrag:** committen, pushen, taggen, auf den Server deployen.
