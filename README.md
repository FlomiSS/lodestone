# Lodestone

[![CI](https://github.com/Henny263462/lodestone/actions/workflows/ci.yml/badge.svg)](https://github.com/Henny263462/lodestone/actions/workflows/ci.yml)
[![Lizenz: MIT](https://img.shields.io/badge/Lizenz-MIT-blue.svg)](LICENSE)

Ein Hosting-Panel für Minecraft, zum Selbsthosten. Es verwaltet Server und ganze Netzwerke als **Projekte** auf einer
oder mehreren Maschinen (Nodes), mit Docker als Isolation, mehreren Benutzern und Rollen pro Projekt.

> **Stand: Version 0.1.** Fertig sind Node-Daemon, Panel-Backend, Weboberfläche und die Netzwerk-Autoconfig. Gruppen mit
> Autoscaling und die Verbindung der Node „nach außen“ kommen später, siehe [docs/architecture.md](docs/architecture.md).

```
Browser ──►  Panel (Go, SQLite, eingebettete Next.js-Oberfläche)  ──HTTP + Token──►  Node-Daemon (Go)  ──►  Docker  ──►  Server
```

## Was geht

- **Projekte:** ein einzelner Server oder ein **Netzwerk** (Velocity + Server dahinter). Mitglieder mit Rollen
  *Betrachter, Operator, Admin, Eigentümer* pro Projekt; globale Administratoren sehen alles
- **Server-Arten:** Paper, Purpur, Folia, Fabric, Vanilla, Velocity – Version, Download mit Prüfsumme und Java-Version
  werden automatisch bestimmt
- **Konsole live** (Websocket, mit Verlauf und ANSI-Farben), Start/Stopp/Neustart/Kill, CPU-/RAM-Anzeige
- **Dateien** mit Editor und Syntax-Highlight (JSON, YAML, TOML, XML, Properties, Markdown …), Upload per Drag & Drop,
  Verschieben, Archivieren (`.zip`, `.tar.gz`), Entpacken und Mehrfachauswahl (Verschieben, Archivieren, Löschen)
- **Plugins und Mods:** installierte anzeigen (Name, Version), von **Modrinth** suchen und installieren (nur passende
  zu Software und Version, inklusive benötigter Abhängigkeiten), Updates prüfen und einspielen, Jar hochladen,
  aktivieren/deaktivieren. Zu jeder installierten Datei zeigt das Panel **Modrinth-Seite, Version, Lizenz und
  Quellcode** und lässt die **Version wechseln**
- **Modpacks:** Beim Anlegen eines Servers ein **Modrinth-Modpack** (Fabric, `.mrpack`) suchen und Version wählen. Mods
  und Konfiguration werden mit Prüfsummen geladen, der Fortschritt steht im Server
- **playit.gg:** Der Node startet den playit-Agenten (Schlüssel unter **Nodes** eintragen), die Tunnel-Adresse
  speichert man beim Server unter *Einstellungen → Öffentliche Adresse*. Keine Portfreigabe im Router nötig
- **Backups** (tar.gz, bei laufendem Server mit `save-off`), Wiederherstellen (sicher, nur bei ausgeschaltetem Server),
  Aufbewahrungsanzahl, Download
- **Spieler** online (Serverlisten-Abfrage), Server-Einstellungen, Versionswechsel ohne Datenverlust
- **Netzwerk-Autoconfig:** Beim Hinzufügen schreibt das Panel alles selbst – Velocity *modern forwarding* samt
  Secret, Serverliste, **Beitrittsreihenfolge (Try) mit Reihenfolge und Schalter pro Server**, **Forced Hosts**,
  `online-mode=false` und Forwarding bei Paper/Purpur/Folia, FabricProxy-Lite bei Fabric (wird installiert)
- **Simple Voice Chat** pro Server einschaltbar: Plugin bzw. Mod wird installiert, der UDP-Port veröffentlicht und
  `voice_host` gesetzt
- Server hinter dem Proxy sind **nicht** nach außen veröffentlicht, nur der Proxy (und der Voicechat-UDP-Port)

## Schnellstart mit fertigen Images

```bash
curl -LO https://github.com/Henny263462/lodestone/releases/latest/download/docker-compose.release.yml
curl -L  https://github.com/Henny263462/lodestone/releases/latest/download/.env.example -o .env
# .env ausfüllen (siehe unten), Datenordner anlegen: mkdir -p /srv/lodestone && chown 1000:1000 /srv/lodestone
docker compose -f docker-compose.release.yml up -d
```

## Schnellstart (Docker Compose, selbst bauen)

Voraussetzung: Docker mit Compose. Panel und ein Node laufen auf derselben Maschine.

```bash
cp .env.example .env
# .env ausfüllen: LODESTONE_ADMIN_PASSWORD, LODESTONE_NODE_TOKEN (openssl rand -hex 32), LODESTONE_PUBLIC_HOST,
# DOCKER_GID (stat -c %g /var/run/docker.sock) und LODESTONE_DATA (Ordner für die Serverdaten, Besitzer 1000:1000)
mkdir -p /srv/lodestone && chown 1000:1000 /srv/lodestone
docker compose up -d --build
```

Dann `http://<Maschine>:8480` öffnen und mit dem Administrator aus der `.env` anmelden. Die Weboberfläche hat kein
eigenes TLS: Stelle bei Zugriff aus dem Internet einen Reverse-Proxy mit HTTPS davor und setze
`LODESTONE_SECURE_COOKIES=true` und `LODESTONE_TRUST_PROXY=true`.

Wichtig bei Compose: Der Datenordner wird **mit demselben Pfad** im Host und im Node-Container eingebunden, weil Docker
die Serverordner über den Pfad des Hosts in die Minecraft-Container bindet. Die Node-API (`:8480` im Compose-Netz) wird
nicht veröffentlicht. Die Minecraft-Ports kommen aus `LODESTONE_PORT_RANGE` – wähle sie so, dass sie keine anderen
Dienste stören, und öffne sie in der Firewall.

Weitere Maschinen: Node-Container (`Dockerfile.node`) dort starten, die API mit TLS oder über VPN erreichbar machen und
im Panel unter **Nodes** mit Adresse und Token eintragen. Alle Server eines Netzwerks liegen auf *einem* Node.

### Ohne Docker bauen

```bash
sh scripts/build-web.sh                          # Oberfläche bauen und einbetten (Node.js 22)
go build -o bin/lodestone-panel ./cmd/lodestone-panel
go build -o bin/lodestone-node  ./cmd/lodestone-node
```

Für schlanke Maschinen: `sh scripts/package.sh` baut Linux-Binaries nach `dist/`, daraus bauen die
`deploy/Dockerfile.*.prebuilt` kleine Images.

## Konfiguration des Panels (Umgebung)

| Variable | Bedeutung |
| --- | --- |
| `LODESTONE_LISTEN` | Adresse, Standard `:8480` |
| `LODESTONE_DATA_DIR` | Ordner der Datenbank (`lodestone.db`), Standard `data` |
| `LODESTONE_ADMIN_USER` / `_PASSWORD` | legt beim ersten Start den Administrator an (ohne Passwort wird eins erzeugt und einmalig ins Log geschrieben). Ohne diese Variablen fragt die Oberfläche beim ersten Aufruf nach dem Administrator |
| `LODESTONE_NODE_URL` / `_TOKEN` / `_NAME` / `_PUBLIC_HOST` | trägt beim Start einen Node ein bzw. aktualisiert ihn |
| `LODESTONE_SECURE_COOKIES`, `LODESTONE_TRUST_PROXY` | hinter HTTPS-Proxy auf `true` |

Node-Daemon: `LODESTONE_NODE_TOKEN`, `_LISTEN`, `_DATA_DIR`, `_PUBLIC_HOST`, `_PORT_RANGE` (z. B. `28000-28099`),
`_CONTAINER_USER`, `_NETWORK` – oder `config.json` (siehe unten).

## Sicherheit

- Anmeldung mit **argon2id**-Passwörtern, Sitzungen als HttpOnly-/SameSite=Strict-Cookie (nur der Hash liegt in der
  Datenbank), zusätzlicher Header gegen CSRF, Herkunftsprüfung beim Websocket, Drosselung nach Fehlversuchen
- Rechte werden bei **jeder** Anfrage serverseitig geprüft (Betrachter lesen Konsole/Spieler, Operatoren steuern
  Server und Dateien, Admins ändern Einstellungen, Eigentümer löschen Projekte); Projekte, in denen man nicht
  Mitglied ist, „gibt es nicht“ (404)
- Das Token des Nodes verlässt das Panel nie; die Node-API gehört nur ins interne Netz oder hinter TLS/VPN
- Container laufen ohne Root-Rechte des Hosts, mit `no-new-privileges`, abgeschalteten Capabilities, Prozess-,
  Speicher- und CPU-Limit; Dateizugriffe bleiben im Serverordner (`os.Root`, auch über Symlinks hinweg)
- Ein Daemon mit Zugriff auf `docker.sock` hat auf der Maschine weitreichende Rechte. Vor dem Öffnen für fremde
  Benutzer: unabhängige Sicherheitsprüfung (siehe docs/architecture.md)

## Der Node-Daemon allein

Der Daemon lässt sich auch ohne Panel nutzen (eigene HTTP-Schnittstelle, Token).

### Voraussetzungen

- [Go](https://go.dev/dl/) 1.26 oder neuer zum Bauen
- [Docker](https://docs.docker.com/get-docker/) (Linux oder Docker Desktop mit WSL)

### Bauen und starten

```bash
go build -o bin/lodestone-node ./cmd/lodestone-node
./bin/lodestone-node -config config.json
```

Beim ersten Start wird `config.json` mit einem zufälligen Token angelegt und ausgegeben. Die Schnittstelle lauscht
standardmäßig nur auf `127.0.0.1:8480`.

```json
{
  "listen": "127.0.0.1:8480",
  "token": "…",
  "dataDir": "data",
  "containerUser": "",
  "portRangeStart": 25565,
  "portRangeEnd": 25999,
  "maxUploadMB": 512
}
```

Läuft Docker nicht, startet der Daemon trotzdem und versucht es alle 10 Sekunden erneut.

### Schnittstelle ausprobieren

Alle Aufrufe brauchen `Authorization: Bearer <token>`.

```bash
T=…   # Token aus config.json
H="Authorization: Bearer $T"

# Server anlegen (die Minecraft-EULA bestätigst du mit "eula": true)
curl -H "$H" -X POST localhost:8480/api/servers -d '{"name":"Lobby","software":"paper","version":"1.21.8","memoryMB":2048,"eula":true}'

curl -H "$H" localhost:8480/api/servers                                   # Liste
curl -H "$H" -X POST localhost:8480/api/servers/<id>/power -d '{"action":"start"}'
curl -H "$H" -X POST localhost:8480/api/servers/<id>/command -d '{"command":"say hallo"}'
curl -H "$H" "localhost:8480/api/servers/<id>/logs?lines=100"
curl -H "$H" "localhost:8480/api/servers/<id>/files?path=plugins"
curl -H "$H" "localhost:8480/api/software/paper/versions"
```

| Methode | Pfad | Zweck |
| --- | --- | --- |
| GET | `/api/system` | Daemon-Version, Docker-Zustand |
| GET | `/api/software`, `/api/software/{id}/versions` | Server-Arten und Versionen |
| GET, POST | `/api/servers` | Liste, Server anlegen |
| GET, PATCH, DELETE | `/api/servers/{id}` | ansehen, ändern (nur offline), löschen |
| POST | `/api/servers/{id}/reinstall` | Software oder Version wechseln |
| POST | `/api/servers/{id}/power` | `start`, `stop`, `restart`, `kill` |
| POST | `/api/servers/{id}/command` | Konsolenbefehl |
| GET | `/api/servers/{id}/logs`, `/ws` | Verlauf, Live-Konsole (Websocket, Token auch als `?token=`) |
| GET, PUT, DELETE | `/api/servers/{id}/files`, `/files/content` | Dateien listen, lesen, schreiben, löschen |
| POST | `/api/servers/{id}/files/mkdir`, `/rename`, `/upload` | Ordner, Umbenennen, Upload |
| GET | `/api/servers/{id}/files/download` | Herunterladen |

Websocket: Die erste Nachricht ist `hello` mit Zustand und Verlauf, danach `line`, `state` und `stats`. Zum Senden
schickt der Client `{"type":"command","data":"say hi"}`.

## Tests

```bash
go test ./...                              # ohne Docker (Docker wird durch eine Attrappe ersetzt)
go test -tags docker ./internal/node -run Docker -v   # gegen ein echtes Docker, überspringt sich ohne
```

## Lizenz

[MIT](LICENSE). Beiträge sind willkommen, siehe [CONTRIBUTING.md](CONTRIBUTING.md). Sicherheitslücken bitte privat
melden ([SECURITY.md](SECURITY.md)).
