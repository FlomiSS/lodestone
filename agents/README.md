# Agenten-Leitfaden für Lodestone

Diese Dokumente sorgen dafür, dass jeder Agent – auch ein kleines Modell – Ergebnisse liefert, die zum Rest von Lodestone passen:
gleiches Aussehen, gleicher Ablauf, gleiche Architektur. Der Einstieg ist [`../AGENTS.md`](../AGENTS.md); **dort steht, was Pflicht ist.**

| Datei | Inhalt |
| --- | --- |
| [`00-kurzfassung.md`](00-kurzfassung.md) | Die harten Regeln auf einer Seite. Immer lesen. |
| [`01-visuelles-design.md`](01-visuelles-design.md) | Farben, Schrift, Abstände, Rundungen, Komponenten, Textstil, Verbotenes |
| [`02-oberflaeche-aufbau-und-ablauf.md`](02-oberflaeche-aufbau-und-ablauf.md) | Seitenaufbau, Navigation, Abläufe (Anlegen, Löschen, Laden, Fehler), Leerzustände, Formulare |
| [`03-frontend-architektur.md`](03-frontend-architektur.md) | Ordner, Routing, Daten laden, Typen, Zustand, Rezept für eine neue Seite |
| [`04-backend-architektur.md`](04-backend-architektur.md) | Panel, Node, Software-Katalog, Handler-Muster, Rechte, Datenbank, Sicherheit, Tests |
| [`05-arbeitsablauf-und-qualitaet.md`](05-arbeitsablauf-und-qualitaet.md) | Vorgehen, „Fertig heißt“, Befehle, Demo, Doku, Veröffentlichen |
| [`06-rezepte.md`](06-rezepte.md) | Schritt für Schritt: Endpunkt + Oberfläche, Datenbankspalte, Einstellung, Ereignis, Software |

## Warum es diese Dokumente gibt

Kleine Modelle erfinden gern neue Farben, eigene Buttons, neue Bibliotheken oder verteilen Logik falsch. Das Ergebnis sieht dann
zusammengewürfelt aus und ist schwer zu warten. Diese Dokumente nehmen die Entscheidungen vorweg: Es gibt für jede
typische Aufgabe **genau eine** richtige Lösung, ein Vorbild im Code und eine Prüfliste.

## So arbeitest du damit

1. Aufgabe lesen, in einen Satz fassen („Ich baue …“).
2. Passende Dokumente lesen (Tabelle in `AGENTS.md`).
3. Vorbild im Code suchen und öffnen. Nachbauen, nicht neu erfinden.
4. Klein bauen, oft prüfen (`npx tsc --noEmit`, `go test`).
5. Mit der Liste „Fertig heißt“ abschließen und ehrlich berichten.

## Diese Dokumente pflegen

Ändert sich ein Standard (neue Farbe, neue Komponente, neue Regel), wird das **im selben Commit** hier eingetragen. Die Dokumente
beschreiben den Ist-Zustand, nicht Wünsche. Wünsche stehen in [`../ROADMAP.md`](../ROADMAP.md).
