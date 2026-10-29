// Übersetzt die technische Aktion aus dem Protokoll (z. B. "POST /api/projects/{id}/servers") in lesbaren Text.

const rules: [RegExp, string][] = [
  [/^POST \/api\/projects$/, "Projekt angelegt"],
  [/^PATCH \/api\/projects\/\{id\}$/, "Projekt geändert"],
  [/^DELETE \/api\/projects\/\{id\}$/, "Projekt gelöscht"],
  [/^PUT \/api\/projects\/\{id\}\/members/, "Mitglied geändert"],
  [/^DELETE \/api\/projects\/\{id\}\/members/, "Mitglied entfernt"],
  [/^POST \/api\/projects\/\{id\}\/servers/, "Server angelegt"],
  [/^PUT \/api\/projects\/\{id\}\/network/, "Netzwerk gespeichert"],
  [/^POST \/api\/projects\/\{id\}\/network\/sync/, "Netzwerk abgeglichen"],
  [/^PUT \/api\/projects\/\{id\}\/voicechat/, "Voicechat umgestellt"],
  [/^PUT \/api\/projects\/\{id\}\/motd/, "MOTD geändert"],
  [/^PUT \/api\/projects\/\{id\}\/status/, "Statusseite geändert"],
  [/^POST \/api\/projects\/\{id\}\/groups/, "Gruppe angelegt"],
  [/^(PATCH|POST) \/api\/groups/, "Gruppe geändert"],
  [/^DELETE \/api\/groups/, "Gruppe gelöscht"],
  [/^POST \/api\/projects\/\{id\}\/schedules/, "Zeitplan angelegt"],
  [/^PATCH \/api\/schedules/, "Zeitplan geändert"],
  [/^DELETE \/api\/schedules/, "Zeitplan gelöscht"],
  [/^POST \/api\/schedules\/\{id\}\/run/, "Zeitplan ausgeführt"],
  [/^POST \/api\/projects\/\{id\}\/webhooks/, "Webhook angelegt"],
  [/^(PATCH|POST) \/api\/webhooks/, "Webhook geändert"],
  [/^DELETE \/api\/webhooks/, "Webhook gelöscht"],
  [/^PATCH \/api\/servers\/\{id\}$/, "Server-Einstellungen geändert"],
  [/^DELETE \/api\/servers\/\{id\}$/, "Server gelöscht"],
  [/^PUT \/api\/servers\/\{id\}\/public-address/, "Öffentliche Adresse geändert"],
  [/^PUT \/api\/servers\/\{id\}\/auto-restart/, "Auto-Restart umgestellt"],
  [/^POST \/api\/servers\/\{id\}\/modrinth\/install/, "Modrinth: installiert"],
  [/^POST \/api\/servers\/\{id\}\/addons/, "Plugins/Mods aktualisiert"],
  [/^POST \/api\/servers\/\{id\}\/player-action/, "Spieleraktion"],
  [/^POST \/api\/users$/, "Benutzer angelegt"],
  [/^PATCH \/api\/users/, "Benutzer geändert"],
  [/^DELETE \/api\/users\/\{id\}\/totp/, "2FA eines Benutzers zurückgesetzt"],
  [/^DELETE \/api\/users/, "Benutzer gelöscht"],
  [/^POST \/api\/users\/\{id\}\/password/, "Passwort zurückgesetzt"],
  [/^POST \/api\/me\/password/, "Eigenes Passwort geändert"],
  [/^POST \/api\/me\/keys/, "API-Schlüssel angelegt"],
  [/^DELETE \/api\/me\/keys/, "API-Schlüssel widerrufen"],
  [/^DELETE \/api\/me\/sessions/, "Sitzung beendet"],
  [/^POST \/api\/nodes$/, "Node hinzugefügt"],
  [/^PATCH \/api\/nodes/, "Node geändert"],
  [/^DELETE \/api\/nodes\/\{id\}$/, "Node entfernt"],
  [/^(PUT|DELETE) \/api\/nodes\/\{id\}\/playit/, "playit geändert"],
  [/^POST \/api\/auth\/logout/, "Abgemeldet"],
];

// Aktionen an einem Server laufen über eine gemeinsame Route; der Pfad sagt, was gemeint war.
const serverParts: [RegExp, string][] = [
  [/\/power$/, "Server gesteuert (Start/Stopp)"],
  [/\/command$/, "Konsolenbefehl gesendet"],
  [/\/files\/upload/, "Datei hochgeladen"],
  [/\/files\/(content|mkdir|rename|move|delete|archive|extract|pull)/, "Dateien geändert"],
  [/\/files/, "Dateien geändert"],
  [/\/backups\/[^/]+\/restore/, "Backup wiederhergestellt"],
  [/\/backups\/[^/]+$/, "Backup gelöscht"],
  [/\/backups/, "Backup erstellt"],
  [/\/reinstall/, "Version gewechselt"],
];

export function auditLabel(action: string, target?: string): string {
  const base = action.replace(" (API-Schlüssel)", "");
  const viaKey = action.includes("(API-Schlüssel)") ? " · API-Schlüssel" : "";
  for (const [pattern, label] of rules) {
    if (pattern.test(base)) return label + viaKey;
  }
  if (base.startsWith("POST /api/servers/{id}/") || base.startsWith("DELETE /api/servers/{id}/")) {
    for (const [pattern, label] of serverParts) {
      if (pattern.test(target ?? "")) return label + viaKey;
    }
  }
  return base + viaKey;
}
