# Lodestone

[![CI](https://github.com/Henny263462/lodestone/actions/workflows/ci.yml/badge.svg)](https://github.com/Henny263462/lodestone/actions/workflows/ci.yml)
[![Lizenz: MIT](https://img.shields.io/badge/Lizenz-MIT-blue.svg)](LICENSE)

Ein Hosting-Panel für Minecraft, zum Selbsthosten. Es verwaltet Server und ganze Netzwerke als **Projekte** auf einer
oder mehreren Maschinen (Nodes), mit Docker als Isolation, mehreren Benutzern und Rollen pro Projekt.

> **Stand: Version 0.1.** Fertig sind Node-Daemon, Panel-Backend, Weboberfläche und die Netzwerk-Autoconfig. Gruppen mit
> Autoscaling und die Verbindung der Node „nach außen“ kommen später, siehe [docs/architecture.md](docs/architecture.md). Das Panel sollte noch nicht zum hosten von Wichtiegen Severn verwendet werde, da es aktuell noch sehr instabil ist und eventuell Probleme verursachen kann.

```
Browser ──►  Panel (Go, SQLite, eingebettete Next.js-Oberfläche)  ──HTTP + Token──►  Node-Daemon (Go)  ──►  Docker  ──►  Server
```

## Was geht

- **Projekte:** ein einzelner Server oder ein **Netzwerk** (Velocity + Server dahinter). Mitglieder mit Rollen
  *Betrachter, Operator, Admin, Eigentümer* pro Projekt; globale Administratoren sehen alles
- **Server-Arten:** Paper, Purpur, Folia, Leaf, Fabric, NeoForge, Forge, Vanilla, Velocity – Version, Download mit Prüfsumme und Java-Version
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
- **playit.gg:** Der Node startet den playit-Agenten. Unter **Nodes** auf *Mit playit verbinden* klicken und einmal auf playit.gg
  bestätigen (ein Schlüssel aus dem playit-Dashboard darf nur lesen und kann keine Tunnel anlegen). Tunnel legt Lodestone selbst an:
  beim Server unter *Einstellungen → playit-Tunnel* ein Klick für **Minecraft Java** (TCP) und **Voicechat** (UDP). Die Adresse
  wird beim Server eingetragen, bei Voicechat auch als `voice_host`. Keine Portfreigabe im Router nötig. Mit **playit Premium**
  wählst du beim Anlegen eine **Region** (z. B. Deutschland, New York oder Japan) oder eine **eigene Domain** aus
  deinem playit-Konto; ohne Premium zeigt der Dialog einen Hinweis und legt den Tunnel automatisch an
- **Backups** (tar.gz, bei laufendem Server mit `save-off`), Wiederherstellen (sicher, nur bei ausgeschaltetem Server),
  Aufbewahrungsanzahl, Download
- **Spieler** online (Serverlisten-Abfrage), Server-Einstellungen, Versionswechsel ohne Datenverlust
- **Netzwerk-Autoconfig:** Beim Hinzufügen schreibt das Panel alles selbst – Velocity *modern forwarding* samt
  Secret, Serverliste, **Beitrittsreihenfolge (Try) mit Reihenfolge und Schalter pro Server**, **Forced Hosts**,
  `online-mode=false` und Forwarding bei Paper/Purpur/Folia/Leaf, Proxy Compatible Forge bei NeoForge (wird installiert), FabricProxy-Lite bei Fabric (wird installiert)
- **Simple Voice Chat** pro Server einschaltbar: Plugin bzw. Mod wird installiert, der UDP-Port veröffentlicht und
  `voice_host` gesetzt
- Server hinter dem Proxy sind **nicht** nach außen veröffentlicht, nur der Proxy (und der Voicechat-UDP-Port)
- **Zeitpläne:** Ketten aus Backup, Befehl, Neustart, Start, Stopp und Warten – alle N Minuten, täglich oder an
  gewählten Wochentagen, mit Vorwarnung im Chat und Nachholen nach einem Panel-Neustart (Zeitzone über `LODESTONE_TZ`)
- **Ereignisse und Webhooks:** Abstürze, automatische Neustarts, Node offline/online, wenig Arbeitsspeicher und
  Zeitpläne landen im Projektverlauf und gehen auf Wunsch an **Discord** oder eine beliebige Adresse (Schutz vor
  Zugriffen auf das interne Netz). **Auto-Restart** startet abgestürzte Server neu (höchstens 3-mal in 15 Minuten)
- **Spielerverwaltung:** Kicken, Sperren, Operator, Whitelist und IP-Sperren per Klick (Listen aus den Dateien des
  Servers); **MOTD** des Netzwerks mit MiniMessage-Vorschau
- **Öffentliche Statusseite** pro Projekt (`/status/<Kürzel>/` oder `/status/?p=<Kürzel>`, mit `&embed=1` zum Einbetten) und JSON unter
  `/api/public/status/<Kürzel>`; zeigt nur Online-Status, Spielerzahl, Version und Adresse
- **Konsole:** Suche, Filter nach Stufe, Log-Download und Hinweise zu bekannten Fehlern (zu wenig RAM, EULA, Port belegt,
  falsche Java-Version, Plugin- und Mod-Konflikte)
- **Arbeitsspeicher-Planer:** Der Node prüft vor dem Start, ob genug RAM frei ist (Reserve einstellbar), und die
  Nodes-Seite zeigt zugesagt gegen tatsächlich belegt
- **Updates über das Panel:** Unter **Admin-Panel → Updates** sehen Administratoren den Versionsstand von Panel und Nodes und aktualisieren sie per Klick (Panel, einzelner Node oder alle Nodes). Optional prüft das Panel alle 6 Stunden auf ein neues GitHub-Release und installiert es selbst, erst die Nodes, zuletzt das Panel
- **Konto-Sicherheit:** Zwei-Faktor-Anmeldung (TOTP mit QR-Code und Wiederherstellungscodes), **API-Schlüssel** für
  Skripte (`Authorization: Bearer lds_…`), Sitzungsliste mit Abmelden und ein **Protokoll** aller Änderungen
  (90 Tage, nur für Administratoren)
- **Admin-Panel:** Für Administratoren gibt es einen eigenen Bereich mit Übersicht (Kennzahlen, letzte Aktivität),
  Benutzerverwaltung (Suche, Zwei-Faktor zurücksetzen, Sitzungen beenden, Projekte je Benutzer), Liste aller Projekte,
  Nodes und Protokoll – mit eigener Seitenleiste
- **Anlegen:** Vorlagen (Survival, Lobby, Creative, Mods), **ZIP-Import** einer bestehenden Welt oder eines Servers
  und Strg+K als Schnellsuche

## Schnellstart (1-Klick Installer für Linux)

Auf einem Linux-Server (Ubuntu, Debian, Fedora, Arch etc.) als Root oder mit `sudo` ausführen:

```bash
curl -fsSL https://henny263462.github.io/lodestone/install.sh | sudo bash
```

Das Skript prüft/installiert Docker und Docker Compose, ermittelt die `DOCKER_GID`, generiert sichere Tokens & Passwörter, erstellt das Datenverzeichnis `/srv/lodestone` (1000:1000) und startet den Stack.

## Updates & Automatisierung

Lodestone-Images werden bei jedem Push auf GitHub automatisch per GitHub Actions gebaut und auf der **GitHub Container Registry (ghcr.io)** bereitgestellt. `latest` und die Versions-Tags (`v0.3.0`) gibt es nur für Releases (**stabil**). Pushes auf `main` erscheinen als **Dev-Build** mit den Tags `dev-<Commit>` (sieben Zeichen, z. B. `dev-1a2b3c4`), `main` und dem vollen Commit-Hash; er meldet sich mit genau dieser Version:
- `ghcr.io/henny263462/lodestone-panel:latest`
- `ghcr.io/henny263462/lodestone-node:latest`

### Auto-Update über das Panel (empfohlen)

Administratoren sehen unter **Admin-Panel → Updates** den Versionsstand von Panel und Nodes, prüfen auf neue Releases und starten Updates per Klick. Dort lässt sich auch das automatische Update einschalten (standardmäßig aus): Das Panel prüft alle 6 Stunden auf ein neues GitHub-Release und tauscht die Container aus, ohne die laufenden Minecraft-Server zu unterbrechen. Auf der Nodes-Seite steht bei jedem veralteten Node ein eigener Knopf.

**Update-Kanal:** Unter Admin-Panel → Updates wählst du zwischen **Stabil** (nur veröffentlichte Releases) und **Dev** (immer der neueste *erfolgreich gebaute* Stand von `main`, also der letzte Commit). Der Wechsel gilt für Panel und Nodes, wirkt sofort auf das, was angeboten wird, und ändert nichts, bis du auf „Aktualisieren“ klickst (oder das automatische Update an ist). Ohne Wahl bleibt ein Dev-Build im Dev-Kanal und ein Release im stabilen. Das Panel fragt im Dev-Kanal den letzten grünen Lauf des Workflows *Docker Autobuild & Release* auf `main` bei GitHub ab; so wird nie ein Image angeboten, das noch gebaut wird.

So funktioniert es: Der Node (er hat Zugriff auf `docker.sock`) lädt das Image der Zielversion, zum Beispiel `ghcr.io/henny263462/lodestone-node:v0.3.0` oder `…:dev-1a2b3c4`, und ersetzt den Container mit derselben Konfiguration. Der alte Container bleibt unter anderem Namen erhalten, bis der neue läuft, und wird bei einem Fehler zurückgeholt. Das Panel wird von dem Node aktualisiert, auf dessen Maschine es als Docker-Container läuft; dabei ist es etwa 10 bis 30 Sekunden nicht erreichbar.

Voraussetzungen: Die Images müssen aus dem Workflow stammen (Release-Tag `vX.Y.Z` oder Push auf `main`); nur dann tragen sie ihre Versionsnummer. Eigene Builds melden sich als `dev` und gelten im Standardkanal nie als veraltet; erst wenn du den Kanal ausdrücklich wählst, bietet das Panel auch ihnen einen Wechsel an. Die Nodes müssen den Kanalwechsel selbst können: Ein Node mit einer Version vor 0.3.1 lehnt `dev-…`-Versionen ab und muss einmal von Hand auf eine neuere Version gehoben werden. Beim Selbstbauen die Version mit `--build-arg VERSION=0.3.0` (Docker) bzw. `VERSION=0.3.0 sh scripts/package.sh` mitgeben.

### Automatische Updates per Watchtower (optional)

In der Release-Compose-Datei (`docker-compose.release.yml`) liegt **Watchtower** bereit, ist aber standardmäßig **aus**
(`--profile autoupdate` schaltet es ein). Es prüft stündlich auf neue Images. Watchtower von containrrr ist archiviert und
läuft mit neuen Docker-Versionen (29+) nicht zuverlässig; das manuelle Update unten ist die sichere Wahl.

### Manuelles Update (1-Klick)

```bash
curl -fsSL https://henny263462.github.io/lodestone/update.sh | sudo bash
```
Oder direkt auf dem Server:
```bash
cd /opt/lodestone && sudo ./update.sh
```

## Schnellstart mit fertigen Images

```bash
curl -LO https://github.com/Henny263462/lodestone/releases/latest/download/docker-compose.release.yml
curl -L  https://github.com/Henny263462/lodestone/releases/latest/download/default.env.example -o .env
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
| `LODESTONE_TZ` | Zeitzone für Zeitpläne, z. B. `Europe/Berlin` (Standard: Zeitzone des Panels, sonst UTC) |
| `LODESTONE_WEBHOOK_ALLOW_INTERNAL` | `true` erlaubt Webhooks an Adressen im internen Netz (Standard: gesperrt) |
| `LODESTONE_UPDATE_URL` | Adresse, unter der das Panel nach dem neuesten Release fragt (Standard: GitHub-API dieses Projekts); nützlich für Forks und Tests |
| `LODESTONE_DEV_UPDATE_URL` | Dasselbe für den Dev-Kanal: Adresse der Workflow-Läufe auf `main` (Standard: GitHub-API dieses Projekts) |

Node-Daemon: `LODESTONE_NODE_TOKEN`, `_LISTEN`, `_DATA_DIR`, `_PUBLIC_HOST`, `_PORT_RANGE` (z. B. `28000-28099`),
`_CONTAINER_USER`, `_NETWORK`, `_MEMORY_RESERVE_MB` (RAM, der für das System frei bleibt, Standard 1024), `_PLAYIT_IMAGE`
– oder `config.json` (siehe unten).

## Sicherheit

- Anmeldung mit **argon2id**-Passwörtern, Sitzungen als HttpOnly-/SameSite=Strict-Cookie (nur der Hash liegt in der
  Datenbank), zusätzlicher Header gegen CSRF, Herkunftsprüfung beim Websocket, Drosselung nach Fehlversuchen
- Rechte werden bei **jeder** Anfrage serverseitig geprüft (Betrachter lesen Konsole/Spieler, Operatoren steuern
  Server und Dateien, Admins ändern Einstellungen, Eigentümer löschen Projekte); Projekte, in denen man nicht
  Mitglied ist, „gibt es nicht“ (404)
- Optional **Zwei-Faktor** (TOTP, TOTP-Geheimnisse mit AES-GCM verschlüsselt abgelegt), API-Schlüssel nur als Hash
  gespeichert und ohne Zugriff auf Konto-Einstellungen; Änderungen werden im **Protokoll** festgehalten
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
