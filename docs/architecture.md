# Architektur und Plan

Diese Datei hält die Entscheidungen fest, die beim Planen von Lodestone gefallen sind, damit sie nicht verloren gehen.

## Bausteine

```
Panel (Next.js)  ──HTTPS/WebSocket──►  Panel-Backend (Go) ── Datenbank (SQLite → Postgres)
                                              ▲
                                              │ Websocket, die Node verbindet nach außen
                                       Node-Daemon (Go) ──► Docker ──► Minecraft-Server (Torch optional)
```

- **Node-Daemon** (`cmd/lodestone-node`): läuft auf jeder Maschine, besitzt die Server-Prozesse. *Fertig.*
- **Panel-Backend** (`cmd/lodestone-panel`, `internal/panel`): Benutzer, Rollen, Projekte, Nodes, Netzwerk-Autoconfig,
  Modrinth, Proxy zu den Nodes, SQLite. *Fertig (v0.1).*
- **Panel-Oberfläche** (`web/`, Next.js als statischer Export, ins Panel-Binary eingebettet; Konsole mit ANSI-Farben,
  Datei-Editor mit CodeMirror 6, Plugins/Mods, Backups, Spieler). *Fertig (v0.1).*
- **Verbindung Panel ↔ Node (v0.1):** Das Panel ruft die HTTP-Schnittstelle des Nodes direkt auf (Bearer-Token); die
  Node-API gehört deshalb ins interne Netz oder hinter TLS/VPN. **Später:** Die Node verbindet **nach außen** zum Panel
  (eine Websocket-Verbindung, über die alles läuft). Dann braucht sie keine offenen Ports und funktioniert hinter NAT.
  Der Daemon-Handler ist ein normaler `http.Handler`, deshalb lässt sich die jetzige Schnittstelle unverändert durch
  diese Verbindung tunneln.

## Datenmodell

- **Projekt**: Einzelserver oder Netzwerk; hat Mitglieder mit Rollen pro Projekt.
- **Server** gehört zu einem Projekt und läuft auf einer Node. Er hat eine **Software** (Paper, Purpur, Folia, Fabric,
  Vanilla, Velocity, später Forge/NeoForge), Version, Java-Version, Startzeile und Limits. Ein Server ist entweder
  *statisch* (behält seine Daten) oder Instanz einer *Gruppe*.
- **Netzwerk** = ein oder mehrere Proxys + beliebig viele statische Server + optional Gruppen. Auch ein kleines
  Netzwerk nur aus statischen Servern ist möglich.
- **Gruppe**: Vorlage (Software, Version, Plugins, Welt, Configs) plus Regeln (min./max. Instanzen, Spieler pro
  Instanz, Benennung `name-1 …`, RAM/CPU, Nodes, Try-Liste). Instanzen sind dynamisch (aus der Vorlage kopiert,
  beim Stoppen gelöscht) oder statisch.

## Autoconfig für Netzwerke

Beim Hinzufügen eines Servers zu einem Netzwerk schreibt das Panel alles selbst:

- Velocity: Forwarding-Secret, Server-Liste und **Try-Liste** (pro Server wählbar, ob und an welcher Position, per
  Drag & Drop), Forced-Hosts
- Paper/Purpur/Folia: Velocity-Forwarding mit Secret, `online-mode=false`, Port
- Fabric: Proxy-Mod (z. B. FabricProxy-Lite) samt Config
- **Gruppen:** Instanzen melden sich im laufenden Proxy an und beim Stoppen wieder ab (Proxy-Plugin, ähnlich
  Comparator), ohne Neustart. Das Plugin kann auch die Lobby nach Auslastung verteilen, denn Velocity probiert die
  Try-Liste nur der Reihe nach.
- **Bausteine als Daten (YAML)** statt im Code, wie die „Configurators“ in SimpleCloud. Beispiele:
  - *Simple Voice Chat*: UDP-Port je Server, Config-Dateien (Paper/Fabric unterschiedlich), Port-Weiterleitung auf dem
    Proxy, Firewall-Hinweis. Die genauen Optionen vor dem Bau gegen die Doku prüfen.
  - später: Geyser/Floodgate, geteilte LuckPerms-Datenbank, ViaVersion, BlueMap

## Benutzer und Rechte

Von Anfang an: Konten, Teams, Rollen pro Projekt (Besitzer, Admin, Operator, Betrachter) mit feinen Rechten
(Konsole, Dateien, Backups, Plugins ...), API-Schlüssel, Audit-Log, möglichst früh Zwei-Faktor-Anmeldung.

## Selbsthosten

Panel und Datenbank per `docker compose`; die Node als einzelnes Binary oder Container mit einem Einmal-Befehl, der
sie mit dem Panel verbindet. Datenbank hinter einer Schnittstelle, SQLite für kleine, Postgres für größere Setups.

## Reihenfolge

1. ✅ **Node-Daemon**: Server anlegen, starten, stoppen, Konsole live, Dateien, Limits, Wiederübernahme
2. ✅ Panel-Backend: Anmeldung, Benutzer, Datenbank, Nodes, Server-Verwaltung
3. ✅ Projekte (Einzelserver und Netzwerk), Next.js-Oberfläche
4. ✅ Datei-Editor mit Syntax-Highlight, Plugins/Mods (Modrinth, Updates), Logs, Backups, Spieler
5. ✅ Netzwerk-Autoconfig (Try-Reihenfolge, Forced Hosts, statische Server, Voicechat)
6. **Nächste Stufe:** Gruppen mit Scaling und Proxy-Plugin, Verbindung der Node nach außen, Netzwerke über mehrere
   Nodes, geplante Backups, Zwei-Faktor, Audit-Log
7. Forge/NeoForge, Geyser/Floodgate und weitere Bausteine, Spieler-Verwaltung über Rechte (Torch optional)

## Grenzen in v0.1

- Ein Netzwerk liegt auf **einem** Node (die Server erreichen sich über das Docker-Netz `lodestone`).
- Das Panel greift per HTTP auf die Nodes zu (kein Tunnel nach außen).
- Backups sind manuell (begrenzte Aufbewahrung, kein Zeitplan); Gruppen/Autoscaling fehlen.
- Die Weboberfläche liefert kein TLS selbst – Reverse-Proxy davor setzen.

## Offene Entscheidungen

- Lizenz (MIT oder AGPL-3.0), bevor das Projekt veröffentlicht wird
- Unabhängige Sicherheitsprüfung vor dem Öffnen für fremde Benutzer
