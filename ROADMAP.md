# Roadmap

Was als Nächstes kommt, nach Wichtigkeit geordnet. Aufwand: **S** (Stunden), **M** (ein bis zwei Tage), **L** (mehrere Tage oder mehr).
Stand: Version 0.1 (Oktober 2026).

## P0 – Vor dem Veröffentlichen

Das sollte erledigt sein, bevor das Repository öffentlich wird oder jemand Fremdes Lodestone installiert.

- [x] **Auto-Update reparieren und automatisieren (S).** Das mitgelieferte Compose startete früher *Watchtower* mit Zugriff auf `docker.sock`. 
  - *Erledigt:* Auto-Update für Panel und Nodes über die Weboberfläche unter Admin-Panel → Updates (Ein-Klick-Update für Panel, einzelne Nodes oder alle, Release-Notizen, Prüfung gegen GitHub Releases alle 6 Stunden, optionales automatisches Update; das Panel tauscht den Container mit Rückweg bei Fehlern). Die Version wird beim Release-Build ins Binary eingebaut. Update-Kanäle **Stabil** (Releases) und **Dev** (jeder gebaute Commit auf `main`, Tag `dev-<Commit>`), umschaltbar im Panel. Offen: ein Lauf gegen echtes Docker und echte Images. Watchtower ist als optionales Compose-Profil (`autoupdate`) weiterhin möglich, aber das Panel-interne Update ist der empfohlene Weg.
- [ ] **Tests für die neuen Panel-Endpunkte (M).** Es gibt keine Tests für Modpack-Server anlegen, Addon-Details und Versionswechsel, öffentliche Adresse und playit-Verwaltung. Außerdem ist die Modpack-Installation und der playit-Agent noch nie gegen ein echtes Modpack bzw. einen echten Schlüssel gelaufen. Zusätzlich in der CI `go test -race` (Linux) laufen lassen. 
  - *Teilweise erledigt:* Integrationstests mit gespieltem Modrinth/Node für Modpack, Addon-Details/Versionswechsel, öffentliche Adresse und playit. Offen: `go test -race` in der CI und ein Lauf gegen ein echtes Modpack / einen echten playit-Schlüssel.
- [ ] **Doku auf den aktuellen Stand bringen (S).** README, `docs/architecture.md` und Website sagen noch „Gruppen kommen später“ und „Voicechat pro Server“. Gruppen, Statistiken und Voicechat über den Proxy fehlen auf der Website. Screenshots der neuen Oberfläche (Server-Seite, Gruppen, Statistiken) einbauen. Eine Seite „Modpacks, playit und Gruppen“ unter `docs/`.
- [ ] **Go-Modulpfad angleichen (S).** Er heißt `github.com/lodestone-mc/lodestone`, das Repository `github.com/Henny263462/lodestone`. Entweder das Modul umbenennen (29 Dateien) oder die Organisation `lodestone-mc` anlegen.
- [ ] **Sichere Voreinstellungen (M).**
  - Passwortwechsel beim ersten Login erzwingen (der Installer zeigt das Start-Passwort im Terminal).
  - Optionales Compose-Profil mit **Caddy/Traefik für HTTPS** (Let's Encrypt) und `SECURE_COOKIES` automatisch an.
  - Dokumentieren, was Zugriff auf `docker.sock` bedeutet; optional ein Socket-Proxy.
  - Der Installer fragt `ifconfig.me` nach der öffentlichen IP – das gehört in die Beschreibung (oder abschaltbar).
- [ ] **Aufräumen im Arbeitsordner (S).** `lodestone-images.tar` und `loadstone_fuer_florian/` (zusammen ~30 MB, nicht im Git, aber im Projektordner) sowie `bin/` und `dist/` löschen. Eine `SECURITY.md` mit echter Kontaktadresse.

## P1 – Hoher Nutzen, bald

Ohne diese Punkte wählen Fremde eher Pelican oder Pterodactyl.

- [ ] **Englische Oberfläche (M).** Die Oberfläche ist komplett deutsch. Texte in Sprachdateien auslagern (Deutsch und Englisch), Sprache pro Benutzer wählbar, Browser-Sprache als Vorgabe. Danach Website und README zweisprachig. Das ist der größte Hebel für andere Nutzer.
- [ ] **Geplante und automatische Backups (M).** Zeitplan pro Server (täglich, wöchentlich), Aufbewahrung nach Alter und Anzahl, **Ziel außerhalb der Maschine** (S3/MinIO, SFTP, `restic`), Backup vor Updates, Versionswechsel und Modpack-Update automatisch. 
  - *Teilweise erledigt:* Zeitpläne können Backups auslösen (Aufbewahrung nach Anzahl). Offen: Ziel außerhalb der Maschine (S3/SFTP/restic) und automatische Backups vor Updates.
- [x] **Zeitpläne und Aufgaben (M).** Neustart um 04:00, Befehl senden (z. B. `say Neustart in 5 Minuten`), Ankündigungen vor dem Neustart, Aufgabenketten („sichern, dann stoppen, dann starten“). 
  - *Erledigt:* Aufgabenketten (Backup, Befehl, Neustart, Start, Stopp, Warten), Intervall/täglich/wöchentlich, Vorwarnung, Nachholen, Zeitzone.
- [x] **Absturzerkennung und Auto-Restart (S).** Pro Server wählbar, mit Pause und Obergrenze (ähnlich wie bei den Gruppen), Hinweis mit letzten Log-Zeilen. Dazu **Benachrichtigungen** per Discord-Webhook, E-Mail und Telegram: Absturz, Backup fehlgeschlagen, wenig Speicher, Update verfügbar. 
  - *Erledigt:* Absturz-Erkennung über den Exit-Code, Auto-Restart (3 in 15 Minuten), Projektverlauf, Webhooks (Discord, allgemein). Offen: E-Mail und Telegram.
- [x] **Server importieren und Vorlagen (M).** Vorhandenen Server aus einem ZIP (oder von einer Pelican-/Pterodactyl-Instanz) übernehmen, Software und Version automatisch erkennen. Dazu Ein-Klick-Vorlagen: „Survival“, „Lobby + Survival-Netzwerk“, „Modpack-Server“. 
  - *Erledigt:* ZIP-/tar.gz-Import beim Anlegen und vier Vorlagen. Offen: Übernahme von Pelican/Pterodactyl und automatische Erkennung von Software und Version.
- [x] **Speicher-Planer (S).** Anzeige pro Node: vergebener RAM, wirklich freier RAM, Warnung *vor* dem Start statt Fehlermeldung danach. Grundlage ist die Prüfung, die jetzt im Node steckt (sie hätte den Neustart des Home-Servers verhindert). 
  - *Erledigt:* Speicher-Prüfung vor dem Start im Node, Zusage-gegen-Belegung auf der Nodes-Seite, Warnung bei wenig freiem RAM.
- [x] **Zwei-Faktor-Anmeldung, API-Schlüssel, Audit-Log (M).** TOTP, persönliche Zugriffsschlüssel (für Skripte und die CLI), Protokoll „wer hat wann was geändert“, Liste der aktiven Sitzungen mit Abmelden. 
  - *Erledigt:* TOTP mit Wiederherstellungscodes, API-Schlüssel, Sitzungsliste, Protokoll (90 Tage).
- [ ] **Node per Code koppeln (M).** Statt URL und Token zu kopieren: Einmal-Code im Panel erzeugen, auf der neuen Maschine `curl … | bash -s CODE`. Danach die Verbindung **vom Node nach außen** (Websocket), damit Nodes hinter NAT und ohne offenen Port funktionieren.
- [ ] **Netzwerke über mehrere Nodes (L).** Heute liegt ein Netzwerk auf *einem* Node. Mit öffentlichen oder VPN-Adressen (WireGuard/Tailscale) lassen sich Backends auf anderen Maschinen anbinden.

## P2 – Komfort und Alleinstellung

- [ ] **Anbindung an Observer, Comparator und Torch (M).** Dein eigenes Ökosystem: Beim Anlegen eines Netzwerks Comparator (Proxy) und Torch (Server) installieren und verbinden, LuckPerms, Strafen und Spieler direkt im Panel anzeigen, Anmeldung gemeinsam. Das hat kein anderes Panel.
- [x] **Befehlspalette (S).** `Strg+K` zum Springen zu Projekt, Server, Datei und zum Ausführen von Aktionen (gab es in Observer, fehlt hier). 
  - *Erledigt:* Strg+K springt zu Projekten, Servern und Seiten. Offen: Dateien und Aktionen.
- [ ] **Mehr Modpack- und Plugin-Quellen (L).** Forge-, NeoForge- und Quilt-Modpacks (eigene Loader-Installer, andere Java-Versionen), **CurseForge**-Packs, Hangar (Paper-Plugins), GitHub-Releases. Modpack **aktualisieren** mit automatischem Backup und Konfliktanzeige. „Alle Server aktualisieren“ als Auftrag.
- [x] **Spieler-Verwaltung (M).** Whitelist, Operatoren und Sperren als Oberfläche, Kick/Ban/Nachricht, Spielerseite mit Spielzeit über das ganze Netzwerk, Köpfe und Skins. 
  - *Erledigt:* Whitelist, Operatoren, Sperren, IP-Sperren, Kick/Nachricht, Köpfe. Offen: Spielerseite über das ganze Netzwerk.
- [ ] **Konsole und Logs (M).** Suche und Filter, Log-Dateien durchblättern und herunterladen, gespeicherte Befehle/Makros, Teilen über mclo.gs, einfache Absturzanalyse (häufige Fehler erklären). 
  - *Teilweise erledigt:* Suche, Stufenfilter, Log-Download, Erklärung häufiger Fehler. Offen: ältere Logs durchblättern, Makros, mclo.gs.
- [ ] **Dateimanager (M).** Verlauf und Rückgängig bei Konfigurationsänderungen (Diff), Suche in Dateien, Upload mit Wiederaufnahme, **SFTP-Zugang** pro Server.
- [ ] **Welten (M).** Welt hoch- und herunterladen, zurücksetzen, wechseln; Chunky-Vorberechnung mit einem Klick; BlueMap oder Dynmap über das Panel erreichbar.
- [ ] **Öffentliche Statusseite (M).** Pro Projekt: online/offline, Spielerzahl, MOTD, einbettbar (Widget) – plus Hilfe für **Domain und SRV-Eintrag** (Cloudflare-DNS-Anbindung: `mc.deine-domain.de` automatisch). 
  - *Teilweise erledigt:* Seite, Einbettung und JSON-Schnittstelle. Offen: Domain- und SRV-Hilfe / DNS-Anbindung.
- [ ] **playit weiterdenken (M).** Eine Tunnel-Schnittstelle für Alternativen (Tailscale, frp).
  - *Erledigt:* Tunnel für Minecraft Java und Voicechat werden über die playit-Schnittstelle automatisch angelegt, übernommen und gelöscht (Server → Einstellungen → playit-Tunnel). Ebenso erledigt: Region und eigene Domain beim Anlegen wählbar, Premium-Erkennung über `/agents/rundata` (Region per `alloc: region` im Anlegen, Domain über `/v1/tunnels/create` mit `hostname`). Offen: ein Lauf gegen ein echtes playit-Konto (bisher nur gegen eine gespielte Schnittstelle getestet; besonders Domain-Zuweisung und Domainliste mit dem Agenten-Schlüssel), Region/Domain eines bestehenden Tunnels ändern (heute: Tunnel löschen und neu anlegen), Bedrock/Geyser-Tunnel (UDP) und der eigene Tunnel-Typ „MC: Simple Voice Chat“ des Dashboards, falls die Schnittstelle ihn anbietet (heute: gewöhnlicher UDP-Tunnel).
- [ ] **Leistungsanalyse (M).** TPS-Verlauf, spark-Berichte verlinken, Festplattenverbrauch pro Server, Netzwerkverkehr als Diagramm, JVM-Voreinstellungen (Aikar-Flags) mit Erklärung.
- [ ] **Proxy-Extras (M).** Geyser/Floodgate (Bedrock-Spieler) mit einem Klick, ViaVersion, Wartungsmodus für das ganze Netzwerk (MOTD, Whitelist), MOTD-Editor mit Vorschau. 
  - *Teilweise erledigt:* MOTD-Editor mit Vorschau. Offen: Geyser/Floodgate, ViaVersion, Wartungsmodus.
- [ ] **Kontingente für Mehrbenutzer-Betrieb (M).** Höchstzahl an Servern, RAM und Festplatte pro Benutzer oder Projekt – nötig, wenn andere auf deiner Maschine hosten.

## P3 – Langfristig

- [ ] **Gruppen 2.0 (L).** Vorgewärmte Reserve-Instanzen, Skalierung nach TPS statt nur Spielerzahl, Welt-Vorlagen (bei jedem Start frisch aus der Vorlage), Party- und Warteschlangen-System, Lobby-Verteilung nach Auslastung.
- [ ] **Neue Server-Arten über Position-Packs (L).** Software als modulare ZIP-Pakete („Position-Packs“) mit getrennter Metadatei (`meta.json`), Autokonfigurationsregeln (`autoconfig.json`) und Skripten (`autoconfig.sh`): Paper, Fabric, Purpur, Velocity, NeoForge etc. sowie Upload, Bearbeitung und Export eigener Packs im Admin-Panel.
  - *Teilweise erledigt:* ZIP-Paketformat, Autokonfigurations-Engine für Velocity & Backend, Panel-Verwaltung und Node-Synchronisation implementiert. *Offen:* E2E-Verifikation von Custom-Packs auf physischer Node-Hardware mit echten Docker-Containern und dedizierter Quilt-Provider.
- [ ] **Offene Schnittstelle (M).** Dokumentierte REST-API (OpenAPI), Webhooks, ein Kommandozeilenprogramm `lodestonectl`, ein Terraform-Provider.
- [ ] **Betrieb im größeren Maßstab (L).** Prometheus-Metriken und fertiges Grafana-Dashboard, PostgreSQL statt SQLite wählbar, Sicherung der Panel-Datenbank (Litestream), Hochverfügbarkeit des Panels.
- [ ] **Anmeldung über Dritte (M).** OIDC/SSO (Authentik, Keycloak, Discord), LDAP.
- [ ] **Mobil (M).** Installierbare Web-App (PWA), Push-Benachrichtigungen, Konsole auf kleinen Bildschirmen.
- [ ] **Sicherheitsprüfung von außen (L).** Unabhängige Prüfung vor der Öffnung für fremde Benutzer (Rechte, Dateizugriff, Docker-Isolation), danach ein Bug-Bounty-Hinweis in `SECURITY.md`.

## Empfohlene Reihenfolge

1. Alles aus **P0** (rund zwei Tage), dann das Repository öffentlich machen.
2. **Englische Oberfläche** und **geplante Backups** – damit lässt sich Lodestone Fremden guten Gewissens empfehlen.
3. **Absturzerkennung mit Benachrichtigungen**, **Zeitpläne** und **Speicher-Planer** – das macht den Dauerbetrieb ruhig.
4. **Observer-Anbindung** und **Server-Import** – hebt Lodestone von Pelican ab und erleichtert den Umstieg.
5. Danach nach Bedarf aus P2.
