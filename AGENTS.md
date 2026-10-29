# AGENTS.md – Pflichtlektüre für jeden Agenten

**Diese Datei gilt für jeden KI-Agenten und jedes Modell, das in diesem Repository arbeitet – ohne Ausnahme, egal wie klein
die Aufgabe ist.** Lies sie vollständig, bevor du die erste Datei änderst. (Claude Code liest zusätzlich `CLAUDE.md`, das
auf diese Datei verweist.)

## Pflicht, in dieser Reihenfolge

1. **Lies `agents/00-kurzfassung.md`.** Das sind die harten Regeln auf einer Seite. Sie gelten immer.
2. **Lies die Dokumente, die zu deiner Aufgabe gehören** (Tabelle unten). Du musst sie **benutzen**, nicht nur überfliegen:
   Farben, Komponenten, Abläufe, Dateiorte und Muster stehen dort. Erfinde nichts, was dort schon festgelegt ist.
3. **Suche vor dem Schreiben ein Vorbild** im Code (eine ähnliche Seite, einen ähnlichen Handler) und folge ihm.
   `agents/06-rezepte.md` nennt für die häufigen Aufgaben das passende Vorbild.
4. **Prüfe am Ende** mit der Liste in `agents/05-arbeitsablauf-und-qualitaet.md` („Fertig heißt“). Behaupte nie, etwas sei
   getestet, was du nicht ausgeführt hast.

## Welches Dokument für welche Aufgabe

| Aufgabe | Pflicht |
| --- | --- |
| Alles | `agents/00-kurzfassung.md`, `agents/05-arbeitsablauf-und-qualitaet.md` |
| Aussehen der Weboberfläche (Farben, Abstände, Komponenten, Texte) | `agents/01-visuelles-design.md` |
| Neue Seite, neuer Dialog, Bedienablauf, Fehler- und Leerzustände | `agents/02-oberflaeche-aufbau-und-ablauf.md` |
| Code in `web/` | `agents/03-frontend-architektur.md` |
| Code in `internal/` oder `cmd/` (Panel, Node, Datenbank, Rechte) | `agents/04-backend-architektur.md` |
| Neue Funktion von der Datenbank bis zur Oberfläche | `agents/06-rezepte.md` |
| Commit, Push, Release, Deployment | `agents/05-arbeitsablauf-und-qualitaet.md`, Abschnitt „Veröffentlichen“ |

Ein Agent, der eine Aufgabe in `web/` ohne `01`, `02` und `03` bearbeitet oder in `internal/` ohne `04`, hat sie nicht
ordentlich bearbeitet. Wenn du unsicher bist, welches Dokument gilt: alle lesen. Sie sind kurz.

## Wenn Dokumentation und Code sich widersprechen

Der **Code in `web/src/components/ui/`** (für das Aussehen) und die **Tests** (für das Verhalten) sind die Wahrheit. Ändere dann
die Dokumentation mit, im selben Commit, und erwähne es in deiner Antwort. Ändere nie stillschweigend einen Standard, nur
weil er dir nicht gefällt: Das Design ist mit dem Besitzer abgestimmt.

## Unverhandelbar (Auszug, vollständig in `agents/00-kurzfassung.md`)

- Sprache der Oberfläche, Kommentare und Fehlermeldungen: **Deutsch**, Anrede **du**. Bezeichner im Code: Englisch.
- Nur die Bausteine aus `web/src/components/ui/` und die Farben aus den Design-Variablen benutzen. Keine neuen Farben,
  keine neuen UI-Bibliotheken, keine eigenen Button- oder Karten-Varianten.
- Rechte werden **im Backend bei jeder Anfrage** geprüft, nie nur in der Oberfläche versteckt.
- Nichts an den Pelican/Wings-Servern des Besitzers (Ports 25565–25575 und 25595) anfassen.
- Nichts committen oder pushen, ohne dass der Besitzer es verlangt hat. Keine Geheimnisse in Dateien.
