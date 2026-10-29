# 04 – Backend-Architektur (Go)

Gesamtbild und Begründungen: [`../docs/architecture.md`](../docs/architecture.md). Hier stehen die **Regeln für den Code**.

## 1. Bausteine

```
Browser ──HTTP──▶ Panel (cmd/lodestone-panel, internal/panel) ──HTTP+Bearer-Token──▶ Node (cmd/lodestone-node, internal/node) ──▶ Docker
                       │ SQLite (Benutzer, Projekte, Server-Zeilen, Zeitpläne …)            │ Dateien der Server, Backups, Container
                       └ liefert die Oberfläche (internal/panel/webui, eingebettet)         └ internal/software (Paper, Velocity …: Versionen, Downloads, Java)
```

| Paket | Aufgabe | Darf nicht |
| --- | --- | --- |
| `internal/panel` | Benutzer, Rechte, Projekte, Netzwerke, Gruppen, Zeitpläne, Ereignisse, Weboberfläche; spricht **nur über `NodeClient`** mit Nodes | Docker oder Server-Dateien direkt anfassen |
| `internal/node` | Docker-Container, Server-Dateien (`os.Root`), Backups, Modpacks, playit, Speicherprüfung; kennt weder Benutzer noch Projekte | auf die Panel-Datenbank zugreifen, Rechte entscheiden |
| `internal/software` | Software-Katalog, Versionslisten, Prüfsummen, Java-Wahl | von `panel` oder `node` importieren |
| `cmd/*` | Start, Umgebungsvariablen, Verdrahtung | Fachlogik |

Abhängigkeiten laufen nur in eine Richtung: `cmd` → `panel`/`node` → `software`. `panel` importiert aus `node` nur **Datentypen** der Schnittstelle
(`node.View`, `node.PlayitStatus` …), nie Logik.

## 2. Dateien im Panel (wo gehört was hin)

| Datei | Inhalt |
| --- | --- |
| `panel.go` | `Config`, `Panel`, **alle Routen** (`routes()`), Middleware, `writeJSON`/`writeError`/`fail`/`decode`, Rechte (`rank*`, `requireProject/Server/Admin`) |
| `auth.go`, `security.go`, `handlers_users.go` | Anmeldung, Sitzungen, Zwei-Faktor, API-Schlüssel, Benutzer |
| `handlers_projects.go`, `handlers_servers.go`, `handlers_nodes.go`, `handlers_addons.go`, `handlers_extra.go`, `handlers_more.go` | HTTP-Handler nach Thema |
| `store.go` (Kern + Schema), `store_ext.go`, `store_more.go` | Datenbank: Tabellen, Abfragen. **Nur hier steht SQL.** |
| `nodeclient.go` | HTTP-Client zum Node (`Do`, `Raw`, `Proxy`, `DialWS`) |
| `network.go` | Autoconfig-Erzeugung für Velocity/Paper/Fabric (reine Funktionen, gut testbar) |
| `groups.go`, `scheduler.go`, `events.go`, `metrics.go`, `maintain.go` | Hintergrundschleifen und ihre Logik |
| `modrinth.go` | Modrinth-Client (mit Cache) |
| `*_test.go` | Tests, siehe Abschnitt 8 |

Neue Datei nur, wenn ein neues **Thema** entsteht; sonst in die passende vorhandene Datei. Handler und Store eines Themas liegen in
verschiedenen Dateien (Handler `handlers_*.go`/Themendatei, SQL `store*.go`).

## 3. Das Handler-Muster (immer so)

Vorbild: `putAutoRestart` in `events.go`.

```go
func (p *Panel) putAutoRestart(w http.ResponseWriter, r *http.Request) {
    row, _, _, err := p.requireServer(r, rankAdmin)      // 1. Rechte (immer zuerst!)
    if err != nil { writeError(w, err); return }
    var body struct{ Enabled bool `json:"enabled"` }       // 2. Eingabe, strikt (unbekannte Felder = 400)
    if err := decode(r, &body); err != nil { writeError(w, err); return }
    // 3. prüfen: Länge, Muster, Bereiche → fail(http.StatusBadRequest, "deutsche Meldung")
    if err := p.store.SetAutoRestart(row.ID, body.Enabled); err != nil { writeError(w, err); return }  // 4. handeln
    writeJSON(w, http.StatusOK, …)                          // 5. Antwort (DTO, nie die Datenbankzeile mit Geheimnissen)
}
```

Route in `panel.go`: `mux.HandleFunc("PUT /api/servers/{id}/auto-restart", p.authenticate(p.putAutoRestart))`.

Regeln:
- **Jede** Route außer `healthz`, `auth/status|setup|login|totp` und `public/status/*` läuft durch `p.authenticate(...)`. Routen, die Konto
  oder Zugangsdaten betreffen, zusätzlich durch `p.sessionOnly(...)` (API-Schlüssel dürfen dort nicht hin).
- Mindeststufe wählen: Lesen `rankViewer`, Steuern/Dateien `rankOperator`, Einstellungen `rankAdmin`, Löschen `rankOwner`,
  Systemweites `requireAdmin(r)`. Im Zweifel die höhere Stufe.
- Fremde Projekte „gibt es nicht“: `errNotFound` (404), nicht 403 (macht `requireProject` schon).
- Fehler immer mit `fail(status, "deutscher Satz mit Ursache")` oder einem vorhandenen Fehler (`ErrNotFound`, `ErrExists`, `errForbidden`).
  Fehler vom Node reichen `writeError`/`NodeError` durch. Keine rohen `err.Error()` aus Bibliotheken an Benutzer, wenn sie Pfade oder
  Geheimnisse enthalten könnten.
- Antworten sind JSON mit camelCase-Tags. Listen als `[]` (nie `null`: leere Slices anlegen).
- Jede **verändernde** Route erscheint automatisch im Protokoll (`authenticate` schreibt es). Nichts Persönliches in den Pfad legen.
- Durchgereichte Node-Routen (`/api/servers/{id}/…`) müssen in `requiredRank` (handlers_servers.go) mit Stufe und erlaubter Methode eingetragen
  sein, sonst sind sie gesperrt. Neue Node-Funktion → dort eintragen, **und** `fakeNode` im Test erweitern.

## 4. Datenbank (SQLite, `modernc.org/sqlite`)

- Schema steht als `schema`/`schemaMore` (Strings mit `CREATE TABLE IF NOT EXISTS`). **Neue Tabelle** → in `schemaMore`.
- **Neue Spalte** an einer bestehenden Tabelle → Eintrag in `moreColumns` (`{table, name, definition}`), sie wird beim Start per `ensureColumn`
  nachgezogen. Die Definition braucht einen Standardwert (`INTEGER NOT NULL DEFAULT 0`).
- Nie Spalten löschen, umbenennen oder Typen ändern; nie Daten verwerfen. Ältere Datenbanken müssen weiter starten.
- IDs: `newID()` (Zufall, Hex). Zeit: Unix-Millisekunden (`time.Now().UnixMilli()`), Spaltenname `…_at`/`time`.
- SQL nur mit Platzhaltern (`?`), nie mit `fmt.Sprintf` aus Eingaben. Fehler `sql.ErrNoRows` → `ErrNotFound` übersetzen (Muster in `store.go`).
- Geheimnisse (Passwort, Sitzung, API-Schlüssel, Node-Token, TOTP): Passwörter argon2id, Sitzungs-/Schlüssel-Token nur als SHA-256-Hash,
  TOTP-Geheimnis mit AES-GCM (`secret.key`). Nie im Klartext speichern, nie in Antworten oder Logs.

## 5. Node

- Schnittstelle in `internal/node/api.go` und `api_extra.go`; jede Route prüft das Bearer-Token (konstante Zeit).
- **Dateien**: ausschließlich über `FS` (`files.go`, basiert auf `os.Root`). Nie `filepath.Join(serverDir, nutzerpfad)` mit direktem `os.*`.
  Entpacken verwirft Symlinks, Pfade nach außen und überschreibt nichts (`archive.go`).
- **Docker**: nur über das `Runtime`-Interface (`runtime.go`, Umsetzung `docker.go`), damit Tests eine Attrappe (`fake_test.go`) nehmen können.
  Container laufen als Nicht-Root, ohne Capabilities, mit `no-new-privileges`, Speicher-/CPU-/Prozess-Limit.
- **Externe Downloads** (Modrinth, Modpack-Dateien, Software): nur von erlaubten Hosts, mit Größenlimit, Prüfsumme (sha1/sha256/sha512),
  Zeitlimit. Siehe `modpack.go` (`safeModpackPath`, Host-Liste).
- **Arbeitsspeicher**: Start eines Servers geht nur nach `checkMemory` (Reserve `LODESTONE_NODE_MEMORY_RESERVE_MB`). Neue speicherhungrige
  Funktionen müssen diese Prüfung benutzen.
- Konfiguration über Umgebungsvariablen `LODESTONE_NODE_*` (`config.go`); jede neue Variable dort **und** in README/`.env.example` eintragen.

## 6. Hintergrundschleifen (Panel)

`maintain.go` startet: `sampleLoop` (Messwerte), `scaleLoop` (Gruppen), `watchLoop` (Abstürze, Auto-Restart, Node-Zustand, wenig RAM),
`scheduleLoop` (Zeitpläne). Regeln: jede Schleife endet mit dem `ctx`, fängt Fehler und loggt sie (`p.log.Warn`) statt abzustürzen,
hält Sperren (`sync.Mutex`) kurz und nie über Netzwerkaufrufe. Neue Hinweise an Benutzer laufen über `p.emit(projectID, serverID, kind, level, message)`
(→ Verlauf + Webhooks).

## 7. Sicherheit (nicht verhandelbar)

1. Rechte bei **jeder** Anfrage im Backend (Abschnitt 3). Die Oberfläche versteckt nur.
2. Eingaben, die in Befehle, Dateinamen, Konfigurationsdateien oder Adressen wandern, werden **streng** geprüft (Whitelist-Regex, wie `playerName`,
   `ipAddress`, `Slugify`) und nie in eine Shell eingesetzt. Konsolenbefehle: Zeilenumbrüche entfernen.
3. Ausgehende Anfragen auf Wunsch von Benutzern (Webhooks) nur über `newHookClient` (sperrt interne Adressen, keine Weiterleitungen).
4. Schreibende Browser-Anfragen brauchen den Header `X-Lodestone: 1` (CSRF); der Websocket prüft die Herkunft. Cookies `HttpOnly`, `SameSite=Strict`.
5. Antworten setzen `Cache-Control: no-store`; nichts Geheimes in URLs.
6. Anmeldeversuche sind gedrosselt; Registrierung gibt es nicht (nur Einrichtung beim ersten Start oder Admin legt an).
7. Öffentliche Routen (`/api/public/*`) liefern nur Unkritisches, sind ratenbegrenzt und gecacht.

## 8. Tests

- Jede neue Funktion bekommt Tests in der passenden `*_test.go`. Fehler- und Rechte-Fälle gehören dazu (z. B. „Betrachter bekommt 403“,
  „fremdes Projekt 404“, „ungültige Eingabe 400“).
- Panel-Tests: `setupPanel(t)` startet Panel + `fakeNode` (HTTP-Attrappe des Nodes in `panel_test.go`); `newClient(t, url)` mit
  `expect(status, method, path, body)`. Neuer Node-Endpunkt → `fakeNode` in `panel_test.go` ergänzen.
- Node-Tests: `fake_test.go` ersetzt Docker. Echte Docker-Tests liegen getrennt (`docker_integration_test.go`) und laufen nicht standardmäßig.
- Externe Dienste (Modrinth, Mojang, Paper-API) werden mit `httptest`-Servern gespielt, nie im Test über das Internet angesprochen.
- Zeit-abhängiges (Zeitpläne, Fenster) mit festen Zeitstempeln testen, nicht mit `Sleep`.
- Läufe: `go test -count=1 ./...`. Auf Windows kann der **erste** Lauf wegen Loopback-Zeitüberschreitungen scheitern; Wiederholen. Schlägt es
  danach noch fehl, ist es ein echter Fehler.

## 9. Stil

- `gofmt` (Pflicht), `go vet ./...` sauber. Kommentare auf Deutsch, an jedem exportierten Namen ein Satz, der mit dem Namen beginnt
  („Schedule ist …“). Bezeichner Englisch, Fehlermeldungen Deutsch.
- Fehler mit `fmt.Errorf("…: %w", err)` weitergeben; Kontext (`context.Context`) an Netzwerk-/Docker-Aufrufe durchreichen.
- Keine globalen veränderlichen Variablen; Zustand hängt am `Panel`/`Manager`.
- Neue Abhängigkeiten (`go.mod`) nur auf Wunsch des Besitzers. Standardbibliothek zuerst.
