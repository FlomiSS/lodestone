# 05 – Arbeitsablauf und Qualität

## 1. Vorgehen bei jeder Aufgabe

1. **Verstehen.** Fasse die Aufgabe in einem Satz zusammen. Ist etwas unklar, das du nicht aus Code oder diesen Dokumenten klären kannst,
   frage kurz nach; sonst entscheide nach den Standards hier und sage, was du entschieden hast.
2. **Vorbild suchen.** Öffne die ähnlichste vorhandene Datei (`06-rezepte.md` nennt sie). Lies sie ganz. Übernimm Aufbau, Namen, Muster.
3. **Klein planen.** Liste die Dateien, die du ändern wirst, und in welcher Reihenfolge (meist: Typen/Datenbank → Backend → Test →
   Oberfläche → Demo → Doku).
4. **Bauen und oft prüfen.** Nach jedem Schritt: `npx tsc --noEmit` bzw. `go build ./...`. Fehler sofort beheben, nicht stapeln.
5. **Abschluss** mit der Liste unten, dann kurz und ehrlich berichten.

Grundsätze:
- **Kleinste Änderung**, die die Aufgabe löst. Kein Umbenennen, Umsortieren, Umformatieren fremden Codes nebenbei.
- **Nichts erfinden, was es gibt:** erst suchen (`Grep`), ob es einen Baustein/eine Funktion schon gibt.
- **Lies, bevor du schreibst:** Eine Datei, die du nicht gelesen hast, änderst du nicht.
- **Kein toter Code, keine auskommentierten Reste, keine TODO ohne Grund.** Keine Dateien „zum Ausprobieren“ im Repository lassen.
- Kommt eine Aufgabe zu groß vor: in Schritte teilen, den ersten vollständig (inkl. Test) liefern, die weiteren benennen.

## 2. Fertig heißt (Pflicht-Liste)

Hake ab, was zutrifft; was du nicht ausgeführt hast, **meldest du als nicht ausgeführt**.

Backend (`internal/`, `cmd/`):
- [ ] `gofmt -l cmd internal` gibt nichts aus
- [ ] `go vet ./...` sauber
- [ ] `go test -count=1 ./...` grün (bei einem Fehler auf Windows einmal wiederholen)
- [ ] Neue Funktion hat Tests (Erfolg, Fehler, Rechte)
- [ ] Rechteprüfung in jedem neuen Handler; Route in `panel.go`; durchgereichte Node-Route in `requiredRank`
- [ ] Datenbank nur additiv (`schemaMore` / `moreColumns`)

Oberfläche (`web/`):
- [ ] `cd web && npx tsc --noEmit` sauber
- [ ] `npx next build` läuft durch (erzeugt `out/`)
- [ ] Vier Zustände (lädt, Fehler, leer, voll) und Rechte beachtet
- [ ] Nur Bausteine aus `components/ui` und Design-Farben; Texte deutsch
- [ ] Typen in `lib/types.ts` ergänzt, `web/demo/server.mjs` kennt die neuen Endpunkte
- [ ] Bei neuer Hauptseite: Seitenleiste (`shell.tsx`) und Strg+K (`palette.tsx`)

Dokumentation:
- [ ] `README.md` („Was geht“, Umgebungsvariablen), `ROADMAP.md` (Punkt abhaken/ergänzen), `docs/architecture.md` bei Strukturänderung
- [ ] `agents/*.md`, wenn du einen Standard geändert oder ergänzt hast
- [ ] `.env.example`/Compose-Dateien, wenn es neue Umgebungsvariablen gibt

## 3. Befehle

```bash
# Backend (im Repo-Wurzelordner)
gofmt -l cmd internal
go vet ./...
go test -count=1 ./...
go build ./...

# Oberfläche
cd web
npm ci                      # nur wenn node_modules fehlt
npx tsc --noEmit
npx next build              # Ausgabe in web/out
node demo/server.mjs        # Demo auf http://localhost:3100 (nutzt web/out), vorher bauen; alten Prozess beenden

# Oberfläche ins Panel einbetten (kopiert nach internal/panel/webui/dist)
sh scripts/build-web.sh
```

Unter Windows (Git Bash): mehrzeilige Heredocs mit vielen Anführungszeichen brechen leicht. Lege längere Skripte als Datei an und führe sie aus,
statt sie inline zu schreiben.

## 4. Testen, ohne etwas zu gefährden

- Entwickle und teste **lokal** (Attrappen, Demo). Der Heimserver des Besitzers hat wenig freien Arbeitsspeicher und betreibt fremde Minecraft-Server
  (Pelican/Wings, Ports 25565–25575 und 25595). **Nie** deren Ports, Container oder Daten anfassen, nie speicherhungrige Tests dort starten.
- Docker-Integrationstests und echte Server nur, wenn der Besitzer es verlangt.
- Die Oberfläche wird nicht im Browser „abgeklickt“, sondern über Typprüfung, Build und die Demo (`web/demo/server.mjs`) belegt. Was du so nicht
  prüfen konntest (Aussehen, echte Dienste wie Discord, playit, Modrinth, Authenticator-App), nennst du ausdrücklich **ungetestet**.

## 5. Berichten (Format der Abschlussantwort)

Kurz, deutsch, ehrlich, in dieser Reihenfolge:
1. **Was jetzt geht** (1–3 Sätze, aus Sicht des Benutzers).
2. **Was geändert wurde** (wichtigste Dateien).
3. **Was geprüft wurde** (Befehle + Ergebnis) und **was nicht** (mit Grund).
4. **Offen / Risiken / Entscheidungen**, die der Besitzer treffen muss.

Verboten: „sollte funktionieren“ als Erfolgsmeldung; Tests erwähnen, die nicht liefen; Fehler verschweigen oder wegerklären;
Umfang größer darstellen, als er ist.

## 6. Veröffentlichen (nur auf ausdrücklichen Auftrag)

Standardmäßig wird **nichts** committet, gepusht, getaggt oder deployt. Auf Auftrag:

1. **Vorher prüfen:** alles aus „Fertig heißt“, `git status` ansehen, dass keine Geheimnisse (`.env`, `config.json`, Schlüssel, Tokens, `data/`,
   `*.tar`, Build-Ergebnisse) und keine Fremddateien in den Commit geraten. `.gitignore` einhalten.
2. **Commit:** eine zusammenhängende Änderung pro Commit, Titelzeile im Imperativ/Perfekt auf Deutsch oder Englisch wie in `git log`,
   darunter ein kurzer Absatz *warum*. Nie `--no-verify`, nie Verlauf umschreiben (`--force`) auf `main`.
3. **Push** nach `origin main` (Repository `Henny263462/lodestone`). Das stößt den Workflow *Docker Autobuild & Release* an, der die Images
   `ghcr.io/henny263462/lodestone-{panel,node}` baut (Tags `latest`, `main`, Commit-Hash). CI (`ci.yml`) prüft `go vet`, `go test`, `tsc`, `next build`
   und die Docker-Builds; **warte auf grün**, bevor du „fertig“ sagst, und melde Rot mit der Ursache.
4. **Release:** Versionsnummer nach SemVer (`vMAJOR.MINOR.PATCH`; neue Funktionen = MINOR, Fehlerbehebungen = PATCH). Tag auf dem geprüften
   Commit setzen (`git tag -a vX.Y.Z -m "…"`, `git push origin vX.Y.Z`). Der Workflow baut dann die Images mit dem Tag und legt das **GitHub-Release
   selbst an** (automatische Notizen, Dateien `deploy/docker-compose.release.yml`, `.env.example`, `install.sh`, `update.sh`). Warte, bis der Lauf grün ist,
   und ersetze die Notizen danach durch gute deutsche (`gh release edit vX.Y.Z --notes-file …`), gegliedert in *Neu*, *Verbessert*, *Behoben*,
   *Hinweise zum Update* (z. B. neue Umgebungsvariablen). Ein bereits veröffentlichtes Tag wird nie verschoben.
5. **Deployment** auf den Heimserver nur auf ausdrücklichen Auftrag: Dokploy-Compose neu ausrollen (Images werden mit `pull_policy: always` geholt).
   Die Zugangsdaten stehen nur in der Dokploy-Umgebung, nie in Dateien oder Antworten.

## 7. Häufige Stolperfallen

- Windows: Der erste Testlauf kann an Loopback-Zeitüberschreitungen scheitern. Zweiter Lauf entscheidet.
- `next build` braucht eine vorhandene `node_modules`; `internal/panel/webui/dist/.gitkeep` muss im Repo bleiben, der Rest von `dist/` ist Build-Ergebnis.
- Statischer Export: Eine Seite, die zur Bauzeit `window` benutzt, bricht den Build. Zugriff nur in Effekten.
- Das Panel erwartet für schreibende Anfragen den Header `X-Lodestone: 1` (macht `lib/api.ts`); Skripte und Tests müssen ihn setzen.
- Datenbank: Neue Spalten ohne Standardwert lassen ältere Datenbanken beim Start scheitern.
- Neue Node-Route ohne Eintrag in `requiredRank` ist für das Panel unsichtbar (404).
