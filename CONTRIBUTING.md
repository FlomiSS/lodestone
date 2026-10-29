# Zu Lodestone beitragen

Danke, dass du mithelfen möchtest! Fehler, Ideen und Pull Requests sind willkommen.

## Fehler und Ideen

Eröffne ein [Issue](../../issues/new/choose). Bei Fehlern helfen: Lodestone-Version, Betriebssystem, Docker-Version und die Meldung aus dem Panel- oder Node-Log. **Sicherheitslücken bitte nicht öffentlich melden**, siehe [SECURITY.md](SECURITY.md).

## Entwicklung

Voraussetzungen: Go 1.26, Node.js 22, Docker (nur für die Docker-Tests und zum Ausprobieren).

```bash
go test ./...                          # Backend-Tests (Docker wird durch eine Attrappe ersetzt)
cd web && npm ci && npx tsc --noEmit   # Oberfläche prüfen
sh scripts/build-web.sh                # Oberfläche bauen und ins Panel einbetten
go build ./cmd/...                     # Panel und Node bauen
```

Die Oberfläche läuft zur Entwicklung auf Port 3100 und spricht mit einem Panel auf Port 8480 (`npm run dev` in `web/`).

## Pull Requests

- Eine Änderung pro Pull Request, mit kurzer Beschreibung *warum*.
- Neue Backend-Funktionen bekommen Tests, siehe `internal/node/*_test.go` und `internal/panel/*_test.go`.
- `go vet ./...`, `go test ./...` und `npx tsc --noEmit` müssen durchlaufen (das prüft auch die CI).
- Sicherheitsrelevantes (Dateizugriff, Rechte, Docker) besonders sorgfältig testen: Pfade dürfen den Serverordner nie verlassen, Rechte werden serverseitig geprüft.
- Texte der Oberfläche und Kommentare sind auf Deutsch, Bezeichner im Code auf Englisch.

Mit deinem Beitrag stimmst du zu, dass er unter der [MIT-Lizenz](LICENSE) veröffentlicht wird.