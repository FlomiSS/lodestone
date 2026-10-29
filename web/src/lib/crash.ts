// Erkennt bekannte Fehlerbilder in den letzten Zeilen der Konsole und erklärt, was zu tun ist.

export interface Hint {
  id: string;
  level: "warn" | "bad";
  title: string;
  text: string;
}

const rules: { id: string; level: Hint["level"]; pattern: RegExp; title: string; text: string }[] = [
  {
    id: "oom",
    level: "bad",
    pattern: /OutOfMemoryError|Java heap space|Exit-Code 137|GC overhead limit/,
    title: "Zu wenig Arbeitsspeicher",
    text: "Der Server hat mehr gebraucht, als sein Limit erlaubt (Exit-Code 137 heißt: vom System beendet). Gib ihm unter Einstellungen mehr Arbeitsspeicher. Paper 26 und Modpacks brauchen schnell 2 bis 4 GB.",
  },
  {
    id: "eula",
    level: "bad",
    pattern: /agree to the EULA|eula=false/i,
    title: "Die Minecraft-EULA ist nicht akzeptiert",
    text: "Öffne unter Dateien die Datei eula.txt und setze eula=true. Danach starten.",
  },
  {
    id: "port",
    level: "bad",
    pattern: /Address already in use|Failed to bind to port|BindException/,
    title: "Der Port ist schon belegt",
    text: "Ein anderes Programm nutzt denselben Port. Prüfe unter Einstellungen die Anschlüsse oder stoppe den anderen Server.",
  },
  {
    id: "java",
    level: "bad",
    pattern: /UnsupportedClassVersionError|class file version|requires Java|Unsupported Java/,
    title: "Falsche Java-Version",
    text: "Software und Java passen nicht zusammen. Wechsle unter Einstellungen die Version oder leg das Docker-Image fest, das zur Software passt.",
  },
  {
    id: "plugin",
    level: "warn",
    pattern: /Could not load '[^']+\.jar'|Error occurred while enabling|Unsupported API version|Plugin .* requires/,
    title: "Ein Plugin lässt sich nicht laden",
    text: "Meist passt die Plugin-Version nicht zur Server-Version, oder eine Abhängigkeit fehlt. Schau unter Plugins nach Updates oder deaktiviere das Plugin.",
  },
  {
    id: "mods",
    level: "bad",
    pattern: /Incompatible mod set|Missing or unsupported mandatory dependencies|Mod resolution failed/,
    title: "Mods passen nicht zusammen",
    text: "Eine Mod fehlt oder ist für eine andere Version gebaut. Die Zeilen darüber nennen, welche. Entferne oder aktualisiere sie unter Mods.",
  },
  {
    id: "forwarding",
    level: "warn",
    pattern: /enable it in your BungeeCord config|Unable to verify player details|ip_forward|Velocity forwarding|modern forwarding/i,
    title: "Der Server erwartet den Proxy",
    text: "Spieler müssen über den Proxy beitreten. Im Projekt unter Netzwerk „Neu abgleichen“ drücken und den Server neu starten, damit die Weiterleitung eingerichtet ist.",
  },
  {
    id: "permission",
    level: "bad",
    pattern: /AccessDeniedException|Permission denied/,
    title: "Fehlende Dateirechte",
    text: "Der Server darf in einen Ordner nicht schreiben. Der Datenordner des Nodes muss dem Benutzer 1000:1000 gehören (chown -R 1000:1000).",
  },
  {
    id: "lag",
    level: "warn",
    pattern: /Can't keep up!|Is the server overloaded/,
    title: "Der Server kommt nicht hinterher",
    text: "Die Tick-Rate sinkt. Gib ihm mehr Arbeitsspeicher oder CPU, senke die Sichtweite (view-distance) oder entferne aufwendige Plugins.",
  },
];

/** Prüft die letzten Zeilen und liefert höchstens drei Hinweise, die schlimmsten zuerst. */
export function analyse(lines: string[]): Hint[] {
  const text = lines.slice(-400).join("\n");
  const found = rules.filter((rule) => rule.pattern.test(text)).map(({ id, level, title, text: body }) => ({ id, level, title, text: body }));
  found.sort((a, b) => Number(b.level === "bad") - Number(a.level === "bad"));
  return found.slice(0, 3);
}
