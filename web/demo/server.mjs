// Demo: liefert die gebaute Oberfläche (web/out) mit einem kleinen Schein-Backend voller Beispieldaten aus.
// Es gibt keine echten Server; Aktionen verändern nur den Zustand im Speicher. Start: npm run demo
import http from "node:http";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { WebSocketServer } from "ws";

const root = path.join(path.dirname(fileURLToPath(import.meta.url)), "..", "out");
const port = Number(process.env.PORT ?? 3100);
const now = () => Date.now();
const MIN = 60_000;
const HOUR = 60 * MIN;

if (!fs.existsSync(path.join(root, "index.html"))) {
  console.error("Die Oberfläche ist noch nicht gebaut. Erst: npm run build");
  process.exit(1);
}

// ---------------------------------------------------------------- Beispieldaten

const user = { id: "u1", username: "admin", isAdmin: true, disabled: false, createdAt: now() - 40 * 24 * HOUR, totpEnabled: true, sessionCount: 2 };
const users = [
  user,
  { id: "u2", username: "anna", isAdmin: false, disabled: false, createdAt: now() - 12 * 24 * HOUR, totpEnabled: true, sessionCount: 1 },
  { id: "u3", username: "ben", isAdmin: false, disabled: false, createdAt: now() - 3 * 24 * HOUR, totpEnabled: false, sessionCount: 0 },
];

const nodes = [
  {
    id: "n1", name: "home", url: "http://node:8480", publicHost: "mc.example.org", online: true, version: "0.1.0", serverCount: 6,
    updateAvailable: true,
    system: { dockerVersion: "29.6.2", os: "Ubuntu 24.04.4 LTS", architecture: "x86_64", cpus: 8, memoryBytes: 16 * 1024 ** 3 },
    memoryAvailable: 5.2 * 1024 ** 3, memoryCommitted: 9.5 * 1024 ** 3,
  },
  { id: "n2", name: "hetzner-1", url: "https://node.example.com:8480", publicHost: "mc2.example.org", online: false, serverCount: 0, error: "der Node ist nicht erreichbar (hetzner-1): connection refused" },
];

let updateStatus = {
  channel: "stable",
  panelVersion: "0.1.0-demo",
  latestVersion: "0.2.0",
  panelUpdateAvailable: true,
  updateAvailable: true,
  checkFailed: false,
  releaseUrl: "https://github.com/lodestone-mc/lodestone/releases/tag/v0.2.0",
  releaseTitle: "Lodestone 0.2.0",
  releaseNotes: "Neu in diesem Release: Auto-Update für Panel und Nodes, verbesserte UI.",
  publishedAt: new Date(now() - 2 * 24 * HOUR).toISOString(),
  autoUpdate: true,
  lastChecked: now() - 10 * MIN,
  nodes: [
    { id: "n1", name: "home", version: "0.1.0", updateAvailable: true, online: true },
    { id: "n2", name: "hetzner-1", version: "", updateAvailable: false, online: false },
  ],
};

let seq = 100;
const id = () => (seq++).toString(16).padStart(12, "0");

function ports(software, extra = []) {
  return [...extra];
}

function makeServer(projectId, name, software, version, memoryMB, state, extra = {}) {
  const sid = extra.id ?? id();
  const role = extra.role ?? "standalone";
  const running = state === "running";
  const stats = running ? { cpuPercent: 8 + Math.random() * 40, memoryBytes: memoryMB * 1024 ** 2 * (0.55 + Math.random() * 0.3), memoryLimit: memoryMB * 1024 ** 2, netRx: 4_000_000, netTx: 9_000_000 } : undefined;
  const hostPort = 28000 + (seq % 90);
  const server = {
    id: sid, projectId, nodeId: "n1", name, slug: name.toLowerCase().replace(/[^a-z0-9]+/g, "-"), role, software,
    inTry: extra.inTry ?? false, tryPosition: extra.tryPosition ?? 0, forcedHost: extra.forcedHost ?? "", voicechat: extra.voicechat ?? false,
    groupId: extra.groupId, nodeName: "home", publicHost: "mc.example.org", online: true,
    address: role === "backend" ? undefined : `mc.example.org:${hostPort}`, voiceAddress: extra.voiceAddress,
    view: {
      id: sid, name, software, version, memoryMB, cpu: 0, eula: true, backupKeep: 5, installed: true, state,
      ports: role === "backend" ? [] : ports(software, [{ container: 25565, host: hostPort, protocol: "tcp" }]),
      stats, startedAt: running ? now() - (extra.uptimeH ?? 5) * HOUR : undefined,
    },
  };
  return server;
}

const proxyId = id();
const survivalId = id();
const lobby1Id = id();
const lobby2Id = id();
const minigamesGroupId = id();
const lobbyGroupId = id();

const projects = [
  {
    id: "p1", name: "Survival-Netz", kind: "network", description: "Hauptnetzwerk mit Lobby, Survival und Creative", ownerId: "u1", createdAt: now() - 30 * 24 * HOUR,
    myRole: "owner", voicechat: true,
    members: [
      { projectId: "p1", userId: "u1", username: "admin", role: "owner" },
      { projectId: "p1", userId: "u2", username: "anna", role: "admin" },
      { projectId: "p1", userId: "u3", username: "ben", role: "viewer" },
    ],
    servers: [
      makeServer("p1", "Proxy", "velocity", "3.5.1", 512, "running", { id: proxyId, role: "proxy", voicechat: true, voiceAddress: "mc.example.org:28000", uptimeH: 71 }),
      makeServer("p1", "Lobby-1", "paper", "1.21.4", 1024, "running", { id: lobby1Id, role: "backend", inTry: true, tryPosition: 0, groupId: lobbyGroupId, voicechat: true, uptimeH: 26 }),
      makeServer("p1", "Lobby-2", "paper", "1.21.4", 1024, "running", { id: lobby2Id, role: "backend", inTry: true, tryPosition: 1, groupId: lobbyGroupId, voicechat: true, uptimeH: 2 }),
      makeServer("p1", "Survival", "paper", "1.21.4", 4096, "running", { id: survivalId, role: "backend", inTry: false, tryPosition: 2, forcedHost: "survival.example.org", voicechat: true, uptimeH: 48 }),
      makeServer("p1", "Creative", "fabric", "1.21.4", 2048, "offline", { role: "backend", inTry: false, tryPosition: 3, voicechat: true }),
    ],
    groups: [
      {
        id: lobbyGroupId, projectId: "p1", name: "Lobby", slug: "lobby", software: "paper", version: "1.21.4", memoryMB: 1024, minInstances: 1, maxInstances: 4, playersPerInstance: 20,
        idleMinutes: 5, inTry: true, dynamic: true, plugins: ["luckperms", "essentialsx"], instances: [lobby1Id, lobby2Id], active: 2, players: 27,
      },
      {
        id: minigamesGroupId, projectId: "p1", name: "Minigames", slug: "minigames", software: "purpur", version: "1.21.4", memoryMB: 2048, minInstances: 0, maxInstances: 6, playersPerInstance: 12,
        idleMinutes: 10, inTry: false, dynamic: true, plugins: [], instances: [], active: 0, players: 0,
        note: "Zu wenig Arbeitsspeicher auf dem Node (13926 von 14212 MB für Server vergeben)",
      },
    ],
    problems: [],
  },
  {
    id: "p2", name: "Test-Server", kind: "server", description: "Zum Ausprobieren von Plugins", ownerId: "u1", createdAt: now() - 6 * 24 * HOUR,
    myRole: "owner", voicechat: false,
    members: [{ projectId: "p2", userId: "u1", username: "admin", role: "owner" }],
    servers: [makeServer("p2", "Test", "paper", "1.21.4", 2048, "running", { uptimeH: 9 })],
    groups: [],
  },
];

projects[0].settings = { statusPublic: true, statusSlug: "survival-netz", motd: "<gradient:#09add3:#22c55e>Survival-Netz</gradient>\n<gray>Neu: Voicechat!" };
projects[1].settings = { statusPublic: false, statusSlug: "", motd: "" };
projects[0].servers.forEach((server) => { server.autoRestart = server.role !== "proxy"; });
projects[1].servers.forEach((server) => { server.autoRestart = false; });
projects[0].servers[4].modpack = { projectId: "P7dR8mSH", slug: "fabulously-optimized", title: "Fabulously Optimized", versionId: "v9", versionNumber: "10.0.0", minecraft: "1.21.4" };
projects[0].servers[3].publicAddress = "survival.playit.gg";

const findServer = (sid) => {
  for (const project of projects) {
    const server = project.servers.find((entry) => entry.id === sid);
    if (server) return { project, server };
  }
  return null;
};

function refreshGroups(project) {
  for (const group of project.groups) {
    const members = project.servers.filter((server) => server.groupId === group.id);
    group.instances = members.map((server) => server.id);
    group.active = members.filter((server) => server.view?.state === "running" || server.view?.state === "starting").length;
  }
}

// ---------------------------------------------------------------- Messreihen

const ranges = { "1h": HOUR, "6h": 6 * HOUR, "24h": 24 * HOUR, "7d": 7 * 24 * HOUR };

function hash(text) {
  let h = 2166136261;
  for (const ch of text) h = Math.imul(h ^ ch.charCodeAt(0), 16777619);
  return (h >>> 0) / 2 ** 32;
}

/** Tageskurve mit Abendspitze und Rauschen, je Server leicht anders. */
function players(serverId, time, scale) {
  const hour = new Date(time).getHours() + new Date(time).getMinutes() / 60;
  const curve = 0.25 + 0.75 * Math.exp(-((hour - 20.5) ** 2) / 14) + 0.25 * Math.exp(-((hour - 14) ** 2) / 20);
  const noise = 0.85 + 0.3 * hash(serverId + Math.floor(time / (5 * MIN)));
  return Math.max(0, Math.round(scale * curve * noise));
}

function series(rangeName, serverIds, scale = 14, memoryMB = 2048, startedHoursAgo = 30) {
  const duration = ranges[rangeName] ?? ranges["6h"];
  const count = 120;
  const start = now() - duration;
  const width = duration / count;
  const times = [];
  const cpu = [];
  const memory = [];
  const playerSeries = [];
  let peakPlayers = 0, sumPlayers = 0, peakCpu = 0, sumCpu = 0, peakMemory = 0, samples = 0;
  for (let i = 0; i < count; i++) {
    const t = start + i * width + width / 2;
    times.push(t);
    if (t < now() - startedHoursAgo * HOUR) {
      cpu.push(null); memory.push(null); playerSeries.push(null);
      continue;
    }
    const p = players(serverIds[0], t, scale);
    const c = Math.min(95, 6 + p * 2.2 + 8 * hash(serverIds[0] + "c" + i));
    const age = (t - (now() - startedHoursAgo * HOUR)) / HOUR;
    const m = memoryMB * 1024 ** 2 * Math.min(0.9, 0.38 + age * 0.012 + 0.02 * hash(serverIds[0] + "m" + i));
    cpu.push(c); memory.push(m); playerSeries.push(p);
    peakPlayers = Math.max(peakPlayers, p); sumPlayers += p; peakCpu = Math.max(peakCpu, c); sumCpu += c; peakMemory = Math.max(peakMemory, m); samples++;
  }
  return { range: rangeName, times, cpu, memory, players: playerSeries, peakPlayers, avgPlayers: samples ? sumPlayers / samples : 0, avgCpu: samples ? sumCpu / samples : 0, peakCpu, peakMemory, samples };
}

const names = ["Alex", "Steve", "Notch", "Herobrine", "Dream", "Technoblade", "Kiwi", "Luna", "Mika", "Finn", "Jonas", "Lea", "Tom", "Nora", "Ole", "Sam"];

// ---------------------------------------------------------------- Dateien

const files = {
  "": [
    { name: "plugins", dir: true, size: 0, modified: now() - 2 * HOUR, mode: "drwxr-xr-x" },
    { name: "world", dir: true, size: 0, modified: now() - 5 * MIN, mode: "drwxr-xr-x" },
    { name: "config", dir: true, size: 0, modified: now() - 26 * HOUR, mode: "drwxr-xr-x" },
    { name: "logs", dir: true, size: 0, modified: now() - 1 * MIN, mode: "drwxr-xr-x" },
    { name: "server.properties", dir: false, size: 1280, modified: now() - 3 * 24 * HOUR, mode: "-rw-r--r--" },
    { name: "bukkit.yml", dir: false, size: 980, modified: now() - 3 * 24 * HOUR, mode: "-rw-r--r--" },
    { name: "eula.txt", dir: false, size: 12, modified: now() - 3 * 24 * HOUR, mode: "-rw-r--r--" },
    { name: "server.jar", dir: false, size: 52_000_000, modified: now() - 3 * 24 * HOUR, mode: "-rw-r--r--" },
  ],
  plugins: [
    { name: "LuckPerms-Bukkit-5.4.150.jar", dir: false, size: 1_600_000, modified: now() - 3 * 24 * HOUR, mode: "-rw-r--r--" },
    { name: "voicechat-bukkit-2.6.27.jar", dir: false, size: 2_100_000, modified: now() - 2 * 24 * HOUR, mode: "-rw-r--r--" },
  ],
};
const fileTexts = {
  "server.properties": "#Minecraft server properties\nmotd=Willkommen im Survival-Netz\nonline-mode=false\nserver-port=25565\nmax-players=20\nview-distance=10\ndifficulty=normal\nenable-rcon=false\n",
  "bukkit.yml": "settings:\n  allow-end: true\n  warn-on-overload: true\n  permissions-file: permissions.yml\nspawn-limits:\n  monsters: 70\n  animals: 10\n",
  "eula.txt": "eula=true\n",
  "velocity.toml": "bind = '0.0.0.0:25565'\nmotd = '<#09add3>Survival-Netz'\nplayer-info-forwarding-mode = 'modern'\n\n[servers]\nlobby-1 = 'lodestone-000000000066:25565'\nlobby-2 = 'lodestone-000000000067:25565'\nsurvival = 'lodestone-000000000068:25565'\ntry = ['lobby-1', 'lobby-2']\n\n[forced-hosts]\n'survival.example.org' = ['survival']\n",
};

// ---------------------------------------------------------------- Anfragen

function send(res, status, body) {
  const text = body === undefined ? "" : JSON.stringify(body);
  res.writeHead(status, { "Content-Type": "application/json; charset=utf-8", "Cache-Control": "no-store" });
  res.end(text);
}

function readBody(req) {
  return new Promise((resolve) => {
    const chunks = [];
    req.on("data", (chunk) => chunks.push(chunk));
    req.on("end", () => {
      const text = Buffer.concat(chunks).toString();
      try {
        resolve(text ? JSON.parse(text) : {});
      } catch {
        resolve({});
      }
    });
  });
}


// ---------------------------------------------------------------- Neue Bereiche (Zeitpläne, Ereignisse, Protokoll ...)

let schedules = [
  { id: "sc1", projectId: "p1", serverId: survivalId, serverName: "Survival", name: "Nächtlicher Neustart", steps: [{ action: "backup" }, { action: "restart" }], kind: "daily", everyMinutes: 0, atMinute: 4 * 60, weekdays: 127, warnMinutes: 5, enabled: true, lastRun: now() - 8 * HOUR, lastStatus: "ok", lastMessage: "2 Schritte ausgeführt", nextRun: now() + 16 * HOUR },
  { id: "sc2", projectId: "p1", serverId: lobby1Id, serverName: "Lobby-1", name: "Hinweis zur vollen Stunde", steps: [{ action: "command", value: "say Vergiss nicht, abzustimmen!" }], kind: "interval", everyMinutes: 60, atMinute: 0, weekdays: 127, warnMinutes: 0, enabled: true, lastRun: now() - 25 * MIN, lastStatus: "ok", lastMessage: "1 Schritt ausgeführt", nextRun: now() + 35 * MIN },
  { id: "sc3", projectId: "p1", serverId: survivalId, serverName: "Survival", name: "Wochenend-Backup", steps: [{ action: "backup" }], kind: "weekly", everyMinutes: 0, atMinute: 3 * 60, weekdays: 0b1100000, warnMinutes: 0, enabled: false, lastRun: 0, lastStatus: "", lastMessage: "", nextRun: 0 },
];
const events = [
  { id: 9, time: now() - 6 * MIN, serverId: survivalId, kind: "backup", level: "info", message: "Backup von „Survival“ ist fertig (208 MB)" },
  { id: 8, time: now() - 2 * HOUR, serverId: lobby2Id, kind: "start", level: "info", message: "„Lobby-2“ wurde gestartet" },
  { id: 7, time: now() - 5 * HOUR, serverId: survivalId, kind: "crash", level: "error", message: "„Survival“ ist abgestürzt (Exit-Code 137). Auto-Restart startet den Server neu." },
  { id: 6, time: now() - 5 * HOUR + 40_000, serverId: survivalId, kind: "restart", level: "warn", message: "„Survival“ wird automatisch neu gestartet (Versuch 1 von 3)" },
  { id: 5, time: now() - 9 * HOUR, kind: "lowmemory", level: "warn", message: "Auf dem Node „home“ sind nur noch 1380 MB Arbeitsspeicher frei." },
  { id: 4, time: now() - 26 * HOUR, kind: "schedule", level: "info", message: "Zeitplan „Nächtlicher Neustart“ ist durchgelaufen" },
];
let webhooks = [
  { id: "w1", name: "Discord #server", url: "https://discord.com/api/webhooks/…/•••", kind: "discord", events: ["crash", "restart", "lowmemory"], enabled: true, lastStatus: "ok" },
];
const audit = [
  ["POST /servers/{id}/power", "Survival", 200, 2], ["PUT /projects/{id}/motd", "Survival-Netz", 200, 40], ["POST /projects/{id}/schedules", "Survival-Netz", 201, 130],
  ["POST /me/keys", "", 201, 300], ["POST /projects/{id}/servers", "Survival-Netz", 201, 700], ["PATCH /users/{id}", "anna", 200, 1500], ["POST /auth/login", "", 200, 2200],
].map(([action, target, status, minutes], index) => ({ id: 100 - index, time: now() - minutes * MIN, userId: "u1", username: index % 3 === 2 ? "anna" : "admin", action, target, status, ip: index % 2 ? "192.168.1.20" : "203.0.113.7" }));
let keys = [{ id: "k1", name: "Skript Backup", prefix: "lds_8fK3", createdAt: now() - 6 * 24 * HOUR, lastUsed: now() - 3 * HOUR, expiresAt: 0 }];
const sessions = [
  { id: "s1", createdAt: now() - 2 * HOUR, expiresAt: now() + 29 * 24 * HOUR, userAgent: "Mozilla/5.0 (Windows NT 10.0) Chrome/130", ip: "192.168.1.20", current: true },
  { id: "s2", createdAt: now() - 4 * 24 * HOUR, expiresAt: now() + 26 * 24 * HOUR, userAgent: "Mozilla/5.0 (iPhone) Safari/604", ip: "203.0.113.7", current: false },
];
let totpEnabled = true;
const me = () => ({ ...user, totpEnabled, recoveryCodes: totpEnabled ? 8 : 0 });
const playerLists = {
  whitelist: [{ uuid: "1", name: "Alex" }, { uuid: "2", name: "Luna" }, { uuid: "3", name: "Mika" }],
  ops: [{ uuid: "1", name: "Alex", level: 4 }],
  banned: [{ uuid: "9", name: "Griefer99", reason: "Griefing", created: "2026-10-20" }],
  bannedIps: [{ ip: "198.51.100.23", reason: "Spam" }],
};
let playit = { configured: false, running: false, image: "ghcr.io/playit-cloud/playit-agent:0.15", logs: [] };
let claimPolls = null; // Abfragen seit dem Start der Verbindung mit playit (null = keine läuft)

const synced = { applied: ["Proxy", "Lobby-1", "Lobby-2", "Survival"], pending: [], failed: [] };

async function api(req, res, url) {
  const parts = url.pathname.replace(/^\/api\//, "").split("/").filter(Boolean);
  const method = req.method;
  const body = ["POST", "PUT", "PATCH"].includes(method) ? await readBody(req) : {};
  const [a, b, c, d] = parts;

  if (a === "auth") {
    if (b === "status") return send(res, 200, { setupRequired: false, version: "0.1.0-demo", user, timezone: "Europe/Berlin" });
    if (b === "totp") return send(res, 200, user);
    if (b === "logout") return send(res, 204);
    return send(res, 200, user);
  }
  if (a === "me") {
    if (!b) return send(res, 200, me());
    if (b === "password") return send(res, 204);
    if (b === "keys") {
      if (method === "POST") {
        const key = { id: id(), name: body.name || "Schlüssel", prefix: "lds_Demo", createdAt: now(), lastUsed: 0, expiresAt: body.days ? now() + body.days * 24 * HOUR : 0 };
        keys.push(key);
        return send(res, 201, { ...key, key: "lds_DemoSchluesselNurImDemoGueltig0123456789" });
      }
      if (c) { keys = keys.filter((key) => key.id !== c); return send(res, 204); }
      return send(res, 200, keys);
    }
    if (b === "sessions") return send(res, c ? 204 : 200, c ? undefined : sessions);
    if (b === "totp") {
      if (c === "setup") return send(res, 200, { secret: "JBSWY3DPEHPK3PXP", uri: "otpauth://totp/Lodestone:admin?secret=JBSWY3DPEHPK3PXP&issuer=Lodestone" });
      if (c === "enable") { totpEnabled = true; return send(res, 200, { recoveryCodes: ["a1b2-c3d4", "e5f6-a7b8", "c9d0-e1f2", "a3b4-c5d6", "e7f8-a9b0", "c1d2-e3f4", "a5b6-c7d8", "e9f0-a1b2"] }); }
      if (c === "disable") { totpEnabled = false; return send(res, 204); }
    }
    return send(res, 204);
  }
  if (a === "audit") {
    const who = url.searchParams.get("user");
    return send(res, 200, audit.filter((entry) => !who || entry.username.includes(who)));
  }
  if (a === "public" && b === "status") {
    if (c !== "survival-netz") return send(res, 404, { error: "nicht gefunden" });
    return send(res, 200, { name: "Survival-Netz", description: "Hauptnetzwerk mit Lobby, Survival und Creative", online: true, address: "mc.example.org:28000", players: players("proxy", now(), 38), maxPlayers: 500, version: "Velocity 3.5.1", servers: [{ name: "Lobby", online: true, players: 12, maxPlayers: 40 }, { name: "Survival", online: true, players: 9, maxPlayers: 20 }, { name: "Creative", online: false, players: 0 }], updated: now() });
  }
  if (a === "schedules") {
    const schedule = schedules.find((entry) => entry.id === b);
    if (!schedule) return send(res, 404, { error: "nicht gefunden" });
    if (method === "DELETE") { schedules = schedules.filter((entry) => entry !== schedule); return send(res, 204); }
    if (c === "run") { schedule.lastRun = now(); schedule.lastStatus = "ok"; schedule.lastMessage = "von Hand ausgeführt"; return send(res, 200, schedule); }
    Object.assign(schedule, body);
    return send(res, 200, schedule);
  }
  if (a === "webhooks") {
    const hook = webhooks.find((entry) => entry.id === b);
    if (!hook) return send(res, 404, { error: "nicht gefunden" });
    if (method === "DELETE") { webhooks = webhooks.filter((entry) => entry !== hook); return send(res, 204); }
    if (c === "test") return send(res, 200, { ok: true, status: "204 No Content" });
    Object.assign(hook, body);
    return send(res, 200, hook);
  }
  if (a === "admin") {
    if (b === "overview") {
      const serverCount = projects.reduce((acc, p) => acc + (p.servers ? p.servers.length : 0), 0);
      const memberCount = projects.reduce((acc, p) => acc + (p.members ? p.members.length : 0), 0);
      const sessionCount = users.reduce((acc, u) => acc + (u.id === "u1" ? sessions.length : (u.sessionCount ?? 1)), 0);
      return send(res, 200, {
        users: users.length,
        admins: users.filter((u) => u.isAdmin).length,
        disabledUsers: users.filter((u) => u.disabled).length,
        projects: projects.length,
        servers: serverCount,
        nodes: nodes.length,
        members: memberCount,
        sessions: sessionCount,
      });
    }
    if (b === "users") {
      if (!c) {
        return send(
          res,
          200,
          users.map((u) => {
            const userProjects = projects.filter((p) => p.members?.some((m) => m.userId === u.id));
            return {
              id: u.id,
              username: u.username,
              isAdmin: !!u.isAdmin,
              disabled: !!u.disabled,
              createdAt: u.createdAt,
              totpEnabled: u.id === "u1" ? !!totpEnabled : !!u.totpEnabled,
              projectCount: userProjects.length,
              sessionCount: u.id === "u1" ? sessions.length : (u.sessionCount ?? 1),
            };
          }),
        );
      }
      const target = users.find((entry) => entry.id === c);
      if (!target) return send(res, 404, { error: "nicht gefunden" });
      if (d === "sessions" && method === "DELETE") {
        if (target.id === "u1") sessions.splice(0, sessions.length);
        target.sessionCount = 0;
        return send(res, 204);
      }
      const userProjects = projects.flatMap((p) => {
        const m = p.members?.find((member) => member.userId === target.id);
        if (!m) return [];
        return [{ id: p.id, name: p.name, kind: p.kind, role: m.role }];
      });
      return send(res, 200, {
        user: {
          id: target.id,
          username: target.username,
          isAdmin: !!target.isAdmin,
          disabled: !!target.disabled,
          createdAt: target.createdAt,
          totpEnabled: target.id === "u1" ? !!totpEnabled : !!target.totpEnabled,
          projectCount: userProjects.length,
          sessionCount: target.id === "u1" ? sessions.length : (target.sessionCount ?? 1),
        },
        projects: userProjects,
      });
    }
    if (b === "projects") {
      return send(
        res,
        200,
        projects.map((p) => {
          const owner = users.find((u) => u.id === p.ownerId);
          return {
            id: p.id,
            name: p.name,
            kind: p.kind,
            description: p.description || "",
            createdAt: p.createdAt,
            ownerId: p.ownerId,
            ownerName: owner ? owner.username : "admin",
            memberCount: p.members ? p.members.length : 0,
            serverCount: p.servers ? p.servers.length : 0,
          };
        }),
      );
    }
    return send(res, 404, { error: "nicht gefunden" });
  }
  if (a === "users") {
    if (c === "totp" && method === "DELETE") {
      const target = users.find((u) => u.id === b);
      if (target) {
        target.totpEnabled = false;
        if (target.id === "u1") totpEnabled = false;
      }
      return send(res, 204);
    }
    if (c === "password" && method === "POST") {
      return send(res, 204);
    }
    if (method === "POST") {
      const newUser = { id: id(), username: body.username, isAdmin: !!body.isAdmin, disabled: false, createdAt: now(), totpEnabled: false, sessionCount: 0 };
      users.push(newUser);
      return send(res, 201, newUser);
    }
    if (method === "PATCH" && b) {
      const target = users.find((u) => u.id === b);
      if (target) Object.assign(target, body);
      return send(res, 200, target);
    }
    if (method === "DELETE" && b) {
      const idx = users.findIndex((u) => u.id === b);
      if (idx !== -1) users.splice(idx, 1);
      return send(res, 204);
    }
    return send(res, method === "GET" ? 200 : 204, method === "GET" ? users : undefined);
  }
  if (a === "updates") {
    if (b === "check" && method === "POST") {
      updateStatus.lastChecked = now();
      return send(res, 200, updateStatus);
    }
    if (b === "settings" && method === "PUT") {
      if (body.channel !== undefined && body.channel !== "stable" && body.channel !== "dev") return send(res, 400, { error: "Der Update-Kanal muss „stable“ oder „dev“ sein." });
      if (body.autoUpdate !== undefined) updateStatus.autoUpdate = !!body.autoUpdate;
      if (body.channel) {
        updateStatus.channel = body.channel;
        const dev = body.channel === "dev";
        updateStatus.latestVersion = dev ? "dev-1a2b3c4" : "0.2.0";
        updateStatus.releaseTitle = dev ? "Spielerliste im Dashboard" : "Lodestone 0.2.0";
        updateStatus.releaseUrl = dev ? "https://github.com/lodestone-mc/lodestone/commit/1a2b3c4" : "https://github.com/lodestone-mc/lodestone/releases/tag/v0.2.0";
      }
      return send(res, 200, { autoUpdate: updateStatus.autoUpdate, channel: updateStatus.channel });
    }
    return send(res, 200, updateStatus);
  }
  if (a === "panel" && b === "update") {
    return send(res, 200, { ok: true, message: "Panel-Update wird durchgeführt" });
  }
  if (a === "nodes" && b === "update-all") {
    return send(res, 200, { ok: true, message: "Alle Nodes werden aktualisiert" });
  }
  if (a === "nodes" && c === "update") {
    return send(res, 200, { ok: true, message: "Node-Update wird durchgeführt" });
  }
  if (a === "nodes" && c === "playit" && d === "claim") {
    // Die Bestätigung auf playit.gg gilt nach dem dritten Abfragen als erfolgt
    if (method === "POST") {
      claimPolls = 0;
      return send(res, 201, { state: "waiting", url: "https://playit.gg/claim/0123456789" });
    }
    if (claimPolls === null) return send(res, 200, { state: "none" });
    if (++claimPolls < 3) return send(res, 200, { state: "waiting", url: "https://playit.gg/claim/0123456789" });
    claimPolls = null;
    playit = { configured: true, running: true, image: playit.image, logs: ["[playit] agent started", "[playit] tunnel ready: survival.playit.gg"] };
    return send(res, 200, { state: "accepted" });
  }
  if (a === "nodes" && c === "playit") {
    if (method === "PUT") playit = { configured: true, running: true, image: playit.image, logs: ["[playit] agent started", "[playit] tunnel ready: survival.playit.gg"] };
    else if (method === "DELETE") playit = { configured: false, running: false, image: playit.image, logs: [] };
    return send(res, method === "GET" ? 200 : 202, playit);
  }
  if (a === "nodes") return send(res, method === "GET" ? 200 : 201, method === "GET" ? nodes : nodes[0]);
  if (a === "packs") {
    if (!b) {
      return send(res, 200, [
        { id: "paper", name: "Paper", description: "Hochperformante Minecraft-Server-Software mit Plugin-Unterstützung.", version: "1.0.0", author: "PaperMC", kind: "server", extensions: "plugins", defaultPort: 25565, custom: false, zipSize: 4200, hasAutoconfig: true, hasScript: false, canBeBackend: true, canVoice: true },
        { id: "fabric", name: "Fabric", description: "Leichtgewichtige, modulare Modding-Toolchain für Minecraft.", version: "1.0.0", author: "FabricMC", kind: "server", extensions: "mods", defaultPort: 25565, custom: false, zipSize: 3800, hasAutoconfig: true, hasScript: false, canBeBackend: true, canVoice: true },
        { id: "purpur", name: "Purpur", description: "Paper-Abspaltung mit Fokus auf Konfigurierbarkeit.", version: "1.0.0", author: "PurpurMC", kind: "server", extensions: "plugins", defaultPort: 25565, custom: false, zipSize: 4100, hasAutoconfig: true, hasScript: false, canBeBackend: true, canVoice: true },
        { id: "velocity", name: "Velocity", description: "Moderner, extrem schneller Minecraft-Proxy von PaperMC.", version: "1.0.0", author: "PaperMC", kind: "proxy", extensions: "plugins", defaultPort: 25565, custom: false, zipSize: 4500, hasAutoconfig: true, hasScript: false, canBeBackend: false, canVoice: false },
        { id: "minestom", name: "Minestom", description: "Schnelle Reimplementierung ohne Vanilla-Code.", version: "1.0.0", author: "Minestom Project", kind: "server", extensions: "none", defaultPort: 25565, custom: true, zipSize: 3900, hasAutoconfig: true, hasScript: true, canBeBackend: true, canVoice: false },
      ]);
    }
    if (b === "upload" && method === "POST") {
      return send(res, 201, { id: "custom-pack", name: "Eigenes Pack", kind: "server", extensions: "plugins", defaultPort: 25565, custom: true, zipSize: 2048, hasAutoconfig: true, hasScript: false, canBeBackend: true, canVoice: false });
    }
    if (b && c === "export") {
      res.writeHead(200, { "Content-Type": "application/zip", "Content-Disposition": `attachment; filename="${b}.positionpack.zip"` });
      return res.end(Buffer.from("PK...mockzip"));
    }
    if (b && method === "DELETE") {
      return send(res, 204);
    }
    if (b && method === "PUT") {
      return send(res, 200, { id: b, name: body.meta?.name || b, kind: body.meta?.kind || "server", extensions: body.meta?.extensions || "plugins", defaultPort: body.meta?.defaultPort || 25565, custom: true, zipSize: 2500, hasAutoconfig: true, hasScript: true, canBeBackend: true, canVoice: true });
    }
    return send(res, 200, {
      meta: { id: b, name: b === "paper" ? "Paper" : b === "fabric" ? "Fabric" : "Position-Pack", kind: b === "velocity" ? "proxy" : "server", extensions: b === "fabric" ? "mods" : "plugins", defaultPort: 25565, startup: "exec java -jar {{JAR}} nogui", stopCommand: "stop", readyPattern: "Done \\(", backend: { supported: b !== "velocity", voiceConfigFile: "plugins/voicechat/voicechat-server.properties" } },
      autoconfig: { backend: { edits: [{ file: "server.properties", format: "properties", set: [{ path: ["online-mode"], value: false }] }] } },
      script: "#!/bin/sh\necho 'Autoconfig ausgeführt'\n",
      custom: b === "minestom",
      zipSize: 4200,
    });
  }
  if (a === "software") {
    if (b && c === "versions") {
      return send(res, 200, b === "velocity" ? ["3.5.1", "3.4.0", "4.3.1"] : b === "fabric" ? ["1.21.4", "1.21.3", "1.20.6"] : b === "neoforge" ? ["1.21.4", "1.21.3", "1.21.1", "1.20.4"] : b === "forge" ? ["1.21.11", "1.21.4", "1.20.1", "1.18.2"] : ["26.3", "26.2", "1.21.11", "1.21.10", "1.21.4"]);
    }
    return send(res, 200, [
      { id: "paper", name: "Paper", kind: "server", extensions: "plugins", defaultPort: 25565, canBeBackend: true, canVoice: true },
      { id: "purpur", name: "Purpur", kind: "server", extensions: "plugins", defaultPort: 25565, canBeBackend: true, canVoice: true },
      { id: "folia", name: "Folia", kind: "server", extensions: "plugins", defaultPort: 25565, canBeBackend: true, canVoice: true },
      { id: "leaf", name: "Leaf", kind: "server", extensions: "plugins", defaultPort: 25565, canBeBackend: true, canVoice: true },
      { id: "fabric", name: "Fabric", kind: "server", extensions: "mods", defaultPort: 25565, canBeBackend: true, canVoice: true },
      { id: "neoforge", name: "NeoForge", kind: "server", extensions: "mods", defaultPort: 25565, canBeBackend: true, canVoice: true },
      { id: "forge", name: "Forge", kind: "server", extensions: "mods", defaultPort: 25565, canBeBackend: false, canVoice: false },
      { id: "vanilla", name: "Vanilla", kind: "server", extensions: "none", defaultPort: 25565, canBeBackend: false, canVoice: false },
      { id: "velocity", name: "Velocity", kind: "proxy", extensions: "plugins", defaultPort: 25565, canBeBackend: false, canVoice: false },
    ]);
  }

  if (a === "projects") {
    if (!b) {
      if (method === "POST") {
        const project = { id: id(), name: body.name || "Neu", kind: body.kind || "server", description: body.description || "", ownerId: "u1", createdAt: now(), myRole: "owner", voicechat: false, members: [{ projectId: "x", userId: "u1", username: "admin", role: "owner" }], servers: [], groups: [], problems: [] };
        projects.push(project);
        return send(res, 201, project);
      }
      projects.forEach(refreshGroups);
      return send(res, 200, projects);
    }
    const project = projects.find((entry) => entry.id === b);
    if (!project) return send(res, 404, { error: "nicht gefunden" });
    refreshGroups(project);
    if (!c) {
      if (method === "DELETE") { projects.splice(projects.indexOf(project), 1); return send(res, 204); }
      if (method === "PATCH") Object.assign(project, { name: body.name ?? project.name, description: body.description ?? project.description });
      return send(res, 200, project);
    }
    if (c === "metrics") {
      const ids = project.servers.map((server) => server.id);
      const range = url.searchParams.get("range") ?? "6h";
      const s = series(range, ids.length ? [ids[0]] : ["x"], project.kind === "network" ? 38 : 9, 4096, 70);
      return send(res, 200, { series: s, players: project.kind === "network" ? 38 : 9 });
    }
    if (c === "servers" && method === "POST") {
      const server = makeServer(project.id, body.name, body.software, body.version, body.memoryMB, "installing", { role: project.kind === "network" ? (body.software === "velocity" ? "proxy" : "backend") : "standalone", inTry: true });
      project.servers.push(server);
      setTimeout(() => { server.view.state = "offline"; }, 4000);
      return send(res, 201, server);
    }
    if (c === "events") return send(res, 200, events);
    if (c === "webhooks") {
      if (method === "POST") { const hook = { id: id(), name: body.name, url: body.url, kind: body.kind, events: body.events ?? [], enabled: true, lastStatus: "" }; webhooks.push(hook); return send(res, 201, hook); }
      return send(res, 200, webhooks);
    }
    if (c === "schedules") {
      if (method === "POST") {
        const target = project.servers.find((server) => server.id === body.serverId);
        const schedule = { id: id(), projectId: project.id, serverName: target?.name ?? "", lastRun: 0, lastStatus: "", lastMessage: "", nextRun: now() + HOUR, enabled: true, everyMinutes: 0, atMinute: 0, weekdays: 127, warnMinutes: 0, ...body };
        schedules.push(schedule);
        return send(res, 201, schedule);
      }
      return send(res, 200, schedules.filter((entry) => entry.projectId === project.id));
    }
    if (c === "status") {
      project.settings = { ...project.settings, statusPublic: !!body.public, statusSlug: body.slug || project.settings?.statusSlug || project.name.toLowerCase().replace(/[^a-z0-9]+/g, "-") };
      return send(res, 200, project.settings);
    }
    if (c === "motd") { project.settings = { ...project.settings, motd: body.motd ?? "" }; return send(res, 200, synced); }
    if (c === "network") return send(res, 200, { ...synced, warnings: [] });
    if (c === "voicechat") {
      project.voicechat = !!body.enabled;
      project.servers.forEach((server) => { server.voicechat = !!body.enabled; });
      return send(res, 200, synced);
    }
    if (c === "groups" && method === "POST") {
      const group = { id: id(), projectId: project.id, name: body.name, slug: String(body.name).toLowerCase(), software: body.software, version: body.version, memoryMB: body.memoryMB, minInstances: body.minInstances, maxInstances: body.maxInstances, playersPerInstance: body.playersPerInstance, idleMinutes: body.idleMinutes, inTry: body.inTry, dynamic: body.dynamic, plugins: body.plugins ?? [], instances: [], active: 0, players: 0 };
      project.groups.push(group);
      return send(res, 201, group);
    }
    if (c === "members") return send(res, d ? 204 : 200, d ? undefined : project.members);
    return send(res, 204);
  }

  if (a === "groups") {
    const project = projects.find((entry) => entry.groups.some((group) => group.id === b));
    const group = project?.groups.find((entry) => entry.id === b);
    if (!group) return send(res, 404, { error: "nicht gefunden" });
    if (c === "instances") {
      const number = project.servers.filter((server) => server.groupId === group.id).length + 1;
      const server = makeServer(project.id, `${group.name}-${number}`, group.software, group.version, group.memoryMB, "installing", { role: "backend", inTry: group.inTry, groupId: group.id });
      project.servers.push(server);
      setTimeout(() => { server.view.state = "running"; server.view.startedAt = now(); }, 5000);
      return send(res, 201, server);
    }
    if (method === "DELETE") {
      project.groups.splice(project.groups.indexOf(group), 1);
      project.servers = project.servers.filter((server) => server.groupId !== group.id);
      return send(res, 204);
    }
    Object.assign(group, body);
    return send(res, 200, group);
  }

  if (a === "servers") {
    const found = findServer(b);
    if (!found) return send(res, 404, { error: "nicht gefunden" });
    const { project, server } = found;
    if (!c) {
      if (method === "DELETE") { project.servers.splice(project.servers.indexOf(server), 1); return send(res, 204); }
      if (method === "PATCH") return send(res, 200, server);
      return send(res, 200, { server, myRole: "owner" });
    }
    if (c === "power") {
      const action = body.action;
      if (action === "start") { server.view.state = "starting"; setTimeout(() => { server.view.state = "running"; server.view.startedAt = now(); server.view.stats = { cpuPercent: 22, memoryBytes: server.view.memoryMB * 1024 ** 2 * 0.5, memoryLimit: server.view.memoryMB * 1024 ** 2, netRx: 0, netTx: 0 }; }, 2500); }
      else if (action === "stop" || action === "kill") { server.view.state = "stopping"; setTimeout(() => { server.view.state = "offline"; server.view.stats = undefined; }, 1800); }
      else if (action === "restart") { server.view.state = "stopping"; setTimeout(() => { server.view.state = "starting"; setTimeout(() => { server.view.state = "running"; server.view.startedAt = now(); }, 2000); }, 1500); }
      return send(res, 202, server.view);
    }
    if (c === "command") return send(res, 204);
    if (c === "auto-restart") { server.autoRestart = !!body.enabled; return send(res, 200, server); }
    if (c === "public-address") { server.publicAddress = body.address || undefined; return send(res, 200, server); }
    if (c === "tunnels") {
      server.tunnels ??= [];
      if (method === "GET" && d === "options") {
        return send(res, 200, {
          premium: true,
          regions: [{ id: "europe", name: "Europa" }, { id: "germany", name: "Deutschland" }, { id: "north-america", name: "Nordamerika" }, { id: "asia", name: "Asien" }],
          domains: [{ id: "dddddddd-0000-0000-0000-000000000001", name: "play.beispiel.de", external: true, used: false }, { id: "dddddddd-0000-0000-0000-000000000002", name: "alt.beispiel.de", external: true, used: true }],
        });
      }
      if (method === "GET") {
        const udp = !!server.view.ports?.some((port) => port.protocol === "udp");
        return send(res, 200, { configured: playit.configured, tunnels: server.tunnels, available: { java: !!server.view.ports?.some((port) => port.protocol === "tcp"), voicechat: udp } });
      }
      if (method === "POST") {
        if (!playit.configured) return send(res, 409, { error: "playit ist auf diesem Node nicht eingerichtet. Ein Administrator hinterlegt den Schlüssel unter Nodes." });
        let tunnel = server.tunnels.find((entry) => entry.kind === body.kind);
        if (!tunnel) {
          tunnel = body.kind === "voicechat"
            ? { kind: "voicechat", id: id(), address: "demo.gl.at.ply.gg:41234", state: "allocated" }
            : { kind: "java", id: id(), address: "demo-server.joinmc.link", state: "allocated" };
          if (body.region) tunnel.region = { europe: "Europa", germany: "Deutschland", "north-america": "Nordamerika", asia: "Asien" }[body.region] ?? body.region;
          if (body.domainId && body.kind === "java") tunnel.domain = tunnel.address = "play.beispiel.de";
          server.tunnels.push(tunnel);
        }
        if (tunnel.kind === "java") server.publicAddress = tunnel.address;
        else server.publicVoiceAddress = tunnel.address;
        return send(res, 201, tunnel);
      }
      if (method === "DELETE") {
        server.tunnels = server.tunnels.filter((entry) => entry.kind !== d);
        if (d === "java") server.publicAddress = undefined;
        else server.publicVoiceAddress = undefined;
        return send(res, 204);
      }
    }
    if (c === "reinstall") return send(res, 202, server.view);
    if (c === "player-lists") return send(res, 200, playerLists);
    if (c === "player-action") {
      const name = body.name;
      const edit = (list, add) => {
        if (add && !list.some((entry) => entry.name === name)) list.push({ uuid: id(), name, reason: body.reason });
        if (!add) list.splice(0, list.length, ...list.filter((entry) => entry.name !== name));
      };
      if (body.action === "whitelist-add") edit(playerLists.whitelist, true);
      if (body.action === "whitelist-remove") edit(playerLists.whitelist, false);
      if (body.action === "op") edit(playerLists.ops, true);
      if (body.action === "deop") edit(playerLists.ops, false);
      if (body.action === "ban") edit(playerLists.banned, true);
      if (body.action === "pardon") edit(playerLists.banned, false);
      if (body.action === "ban-ip") playerLists.bannedIps.push({ ip: name, reason: body.reason });
      if (body.action === "pardon-ip") playerLists.bannedIps = playerLists.bannedIps.filter((entry) => entry.ip !== name);
      return send(res, 200, { ok: true });
    }
    if (c === "modpack") return send(res, 200, { state: "done", done: 148, total: 148, name: "Fabulously Optimized", version: "10.0.0", minecraft: "1.21.4", loader: "fabric" });
    if (c === "logs") return send(res, 200, []);
    if (c === "metrics") {
      const range = url.searchParams.get("range") ?? "6h";
      const scale = server.role === "proxy" ? 38 : server.name.startsWith("Survival") ? 16 : 9;
      const live = server.view.state === "running" ? players(server.id, now(), scale) : 0;
      return send(res, 200, { series: series(range, [server.id], scale, server.view.memoryMB, server.view.startedAt ? (now() - server.view.startedAt) / HOUR : 0.2), players: live, maxPlayers: server.role === "proxy" ? 500 : 20, names: names.slice(0, Math.min(live, 8)) });
    }
    if (c === "players") {
      if (server.view.state !== "running") return send(res, 409, { error: "der Server läuft nicht" });
      const online = players(server.id, now(), server.role === "proxy" ? 38 : 9);
      return send(res, 200, { online, max: server.role === "proxy" ? 500 : 20, names: server.role === "proxy" ? [] : names.slice(0, Math.min(online, 10)), version: server.software === "velocity" ? "Velocity 1.7.2-26.2" : "Paper 1.21.4", motd: "Willkommen im Survival-Netz", latencyMs: 4 });
    }
    if (c === "player-history") {
      const list = names.map((name, index) => ({ name, firstSeen: now() - (20 - index) * 24 * HOUR, lastSeen: now() - index * 37 * MIN, seconds: Math.round((60 - index * 3) * 3600 * hash(name)), online: index < 4 }));
      return send(res, 200, { players: list, online: 4, max: 20 });
    }
    if (c === "files") {
      if (d === "content") {
        const file = url.searchParams.get("path") ?? "";
        res.writeHead(200, { "Content-Type": "text/plain; charset=utf-8" });
        return res.end(fileTexts[file] ?? fileTexts[file.split("/").pop()] ?? "# leer\n");
      }
      if (d) return send(res, 204);
      const dir = (url.searchParams.get("path") ?? "").replace(/^\/+|\/+$/g, "");
      return send(res, 200, files[dir] ?? []);
    }
    if (c === "addons") {
      const isNeoForge = server.software === "neoforge";
      if (d === "check") return send(res, 200, isNeoForge ? [
        { file: "jei-1.21.4-neoforge-19.21.0.246.jar", dir: "mods", name: "Just Enough Items", current: "19.21.0.246", latest: "19.21.0.250", latestFile: "jei-1.21.4-neoforge-19.21.0.250.jar", known: true, updateAvailable: true },
        { file: "voicechat-neoforge-1.21.4-2.6.27.jar", dir: "mods", name: "Simple Voice Chat", current: "2.6.27", known: true, updateAvailable: false },
        { file: "Eigenbau-mod.jar", dir: "mods", name: "Eigenbau Mod", current: "1.0", known: false, updateAvailable: false },
      ] : [
        { file: "LuckPerms-Bukkit-5.4.150.jar", dir: "plugins", name: "LuckPerms", current: "5.4.150", latest: "5.5.0", latestFile: "LuckPerms-Bukkit-5.5.0.jar", known: true, updateAvailable: true },
        { file: "voicechat-bukkit-2.6.27.jar", dir: "plugins", name: "Simple Voice Chat", current: "2.6.27", known: true, updateAvailable: false },
        { file: "Eigenbau.jar", dir: "plugins", name: "Eigenbau", current: "1.0", known: false, updateAvailable: false },
      ]);
      if (d === "update") return send(res, 200, isNeoForge
        ? [{ file: "jei-1.21.4-neoforge-19.21.0.246.jar", updated: true, to: "19.21.0.250" }]
        : [{ file: "LuckPerms-Bukkit-5.4.150.jar", updated: true, to: "5.5.0" }]);
      if (d === "details") return send(res, 200, isNeoForge ? [
        { file: "jei-1.21.4-neoforge-19.21.0.246.jar", dir: "mods", known: true, projectId: "u6dDxAtlas", slug: "jei", title: "Just Enough Items (JEI)", description: "Zeigt Gegenstände und Rezepte an.", projectType: "mod", link: "https://modrinth.com/mod/jei", downloads: 80_000_000, sourceUrl: "https://github.com/mezz/JustEnoughItems", issuesUrl: "https://github.com/mezz/JustEnoughItems/issues", license: "MIT", versionId: "v-jei-1", versionNumber: "19.21.0.246", versionName: "19.21.0.246", versionType: "release", published: "2026-08-12T10:00:00Z" },
        { file: "voicechat-neoforge-1.21.4-2.6.27.jar", dir: "mods", known: true, projectId: "9eGKb6K1", slug: "simple-voice-chat", title: "Simple Voice Chat", description: "Proximity-Voicechat in Minecraft.", projectType: "mod", link: "https://modrinth.com/plugin/simple-voice-chat", downloads: 25_000_000, license: "Custom", versionId: "v2", versionNumber: "2.6.27", versionName: "2.6.27", versionType: "release", published: "2026-09-30T10:00:00Z" },
        { file: "Eigenbau-mod.jar", dir: "mods", known: false },
      ] : [
        { file: "LuckPerms-Bukkit-5.4.150.jar", dir: "plugins", known: true, projectId: "Vebnzrzj", slug: "luckperms", title: "LuckPerms", description: "Ein Rechtesystem für Minecraft-Server.", projectType: "plugin", link: "https://modrinth.com/plugin/luckperms", downloads: 12_400_000, sourceUrl: "https://github.com/LuckPerms/LuckPerms", issuesUrl: "https://github.com/LuckPerms/LuckPerms/issues", license: "MIT", versionId: "v1", versionNumber: "5.4.150", versionName: "v5.4.150", versionType: "release", published: "2026-08-12T10:00:00Z" },
        { file: "voicechat-bukkit-2.6.27.jar", dir: "plugins", known: true, projectId: "9eGKb6K1", slug: "simple-voice-chat", title: "Simple Voice Chat", description: "Proximity-Voicechat in Minecraft.", projectType: "plugin", link: "https://modrinth.com/plugin/simple-voice-chat", downloads: 25_000_000, license: "Custom", versionId: "v2", versionNumber: "2.6.27", versionName: "2.6.27", versionType: "release", published: "2026-09-30T10:00:00Z" },
        { file: "Eigenbau.jar", dir: "plugins", known: false },
      ]);
      if (d === "switch") return send(res, 200, isNeoForge
        ? { file: "jei-1.21.4-neoforge-19.21.0.250.jar", from: "19.21.0.246", to: body.versionId }
        : { file: "LuckPerms-Bukkit-5.5.0.jar", from: "5.4.150", to: body.versionId });
      return send(res, 200, isNeoForge ? [
        { file: "jei-1.21.4-neoforge-19.21.0.246.jar", dir: "mods", size: 1_200_000, modified: now() - 3 * 24 * HOUR, enabled: true, name: "Just Enough Items", version: "19.21.0.246", kind: "neoforge", sha1: "a" },
        { file: "voicechat-neoforge-1.21.4-2.6.27.jar", dir: "mods", size: 2_100_000, modified: now() - 2 * 24 * HOUR, enabled: true, name: "Simple Voice Chat", version: "2.6.27", kind: "neoforge", sha1: "b" },
        { file: "Eigenbau-mod.jar", dir: "mods", size: 90_000, modified: now() - 24 * HOUR, enabled: false, name: "Eigenbau Mod", version: "1.0", kind: "unknown", sha1: "c" },
      ] : [
        { file: "LuckPerms-Bukkit-5.4.150.jar", dir: "plugins", size: 1_600_000, modified: now() - 3 * 24 * HOUR, enabled: true, name: "LuckPerms", version: "5.4.150", kind: "paper", sha1: "a" },
        { file: "voicechat-bukkit-2.6.27.jar", dir: "plugins", size: 2_100_000, modified: now() - 2 * 24 * HOUR, enabled: true, name: "Simple Voice Chat", version: "2.6.27", kind: "paper", sha1: "b" },
        { file: "Eigenbau.jar", dir: "plugins", size: 90_000, modified: now() - 24 * HOUR, enabled: false, name: "Eigenbau", version: "1.0", kind: "unknown", sha1: "c" },
      ]);
    }
    if (c === "modrinth") {
      const isNeoForge = server.software === "neoforge";
      if (d === "search") return send(res, 200, isNeoForge ? { total_hits: 3, hits: [
        { project_id: "u6dDxAtlas", slug: "jei", title: "Just Enough Items (JEI)", description: "Zeigt Gegenstände und Rezepte an.", author: "mezz", downloads: 80_000_000, icon_url: "", categories: ["neoforge"] },
        { project_id: "lhGA9TYQ", slug: "ferrite-core", title: "FerriteCore", description: "Speicheroptimierungen für Minecraft.", author: "malte0811", downloads: 18_000_000, icon_url: "", categories: ["neoforge"] },
        { project_id: "9eGKb6K1", slug: "simple-voice-chat", title: "Simple Voice Chat", description: "Proximity-Voicechat in Minecraft.", author: "henkelmax", downloads: 25_000_000, icon_url: "", categories: ["neoforge"] },
      ] } : { total_hits: 3, hits: [
        { project_id: "Vebnzrzj", slug: "luckperms", title: "LuckPerms", description: "Ein Rechtesystem für Minecraft-Server.", author: "Luck", downloads: 12_400_000, icon_url: "", categories: [] },
        { project_id: "hXiIvTyT", slug: "essentialsx", title: "EssentialsX", description: "Die Essentials für Server: Homes, Warps, Kits und mehr.", author: "EssentialsX", downloads: 8_100_000, icon_url: "", categories: [] },
        { project_id: "9eGKb6K1", slug: "simple-voice-chat", title: "Simple Voice Chat", description: "Proximity-Voicechat in Minecraft.", author: "henkelmax", downloads: 25_000_000, icon_url: "", categories: [] },
      ] });
      if (d === "install") return send(res, 200, { installed: [isNeoForge ? "jei-1.21.4-neoforge-19.21.0.250.jar" : "Beispiel.jar"] });
      return send(res, 200, isNeoForge ? {
        project: { id: "u6dDxAtlas", title: "Just Enough Items (JEI)", description: "Zeigt Gegenstände und Rezepte an.", icon_url: "", downloads: 80_000_000, slug: "jei" },
        versions: [{ id: "v-jei-1", name: "v19.21.0.250", versionNumber: "19.21.0.250", type: "release", published: new Date().toISOString(), gameVersions: ["1.21.4"] }],
      } : {
        project: { id: "Vebnzrzj", title: "LuckPerms", description: "Ein Rechtesystem.", icon_url: "", downloads: 1, slug: "luckperms" },
        versions: [{ id: "v1", name: "v5.5.0", versionNumber: "5.5.0", type: "release", published: new Date().toISOString(), gameVersions: ["1.21.4"] }],
      });
    }
    if (c === "backups") {
      if (method === "GET" && !d) return send(res, 200, [
        { name: "20261029-080135-vor-update.tar.gz", label: "vor-update", size: 217_245_440, created: now() - 3 * HOUR },
        { name: "20261028-030000.tar.gz", size: 201_000_000, created: now() - 29 * HOUR },
        { name: "20261027-030000.tar.gz", size: 198_000_000, created: now() - 53 * HOUR },
      ]);
      return send(res, 202, {});
    }
  }
  return send(res, 404, { error: "unbekannte Adresse (Demo)" });
}

// ---------------------------------------------------------------- Dateien der Oberfläche

const types = { ".html": "text/html; charset=utf-8", ".js": "text/javascript", ".css": "text/css", ".json": "application/json", ".svg": "image/svg+xml", ".png": "image/png", ".ico": "image/x-icon", ".woff2": "font/woff2", ".txt": "text/plain" };

function staticFile(res, pathname) {
  const clean = path.normalize(decodeURIComponent(pathname)).replace(/^([/\\])+/, "");
  const candidates = [clean || "index.html", clean + ".html", path.join(clean, "index.html")];
  for (const candidate of candidates) {
    const file = path.join(root, candidate);
    if (!file.startsWith(root)) continue;
    if (fs.existsSync(file) && fs.statSync(file).isFile()) {
      res.writeHead(200, { "Content-Type": types[path.extname(file)] ?? "application/octet-stream", "Cache-Control": "no-cache" });
      return fs.createReadStream(file).pipe(res);
    }
  }
  // SPA-Fallback für saubere Pfade wie /project/<id>/..., /server/<id>/..., /status/<slug>/... und /admin/...
  const first = clean.split(/[\\/]/)[0];
  if (first === "project" || first === "server" || first === "status" || first === "admin") {
    const isTxt = clean.endsWith(".txt");
    const fallback = path.join(root, first, isTxt ? "index.txt" : "index.html");
    if (fs.existsSync(fallback) && fs.statSync(fallback).isFile()) {
      res.writeHead(200, { "Content-Type": types[path.extname(fallback)] ?? "text/html; charset=utf-8", "Cache-Control": "no-cache" });
      return fs.createReadStream(fallback).pipe(res);
    }
  }

  const missing = path.join(root, "404.html");
  res.writeHead(404, { "Content-Type": "text/html; charset=utf-8" });
  res.end(fs.existsSync(missing) ? fs.readFileSync(missing) : "nicht gefunden");
}

const server = http.createServer((req, res) => {
  const url = new URL(req.url, `http://${req.headers.host}`);
  if (url.pathname.startsWith("/api/")) {
    api(req, res, url).catch((error) => send(res, 500, { error: String(error) }));
    return;
  }
  staticFile(res, url.pathname);
});

// ---------------------------------------------------------------- Schein-Konsole

const sockets = new WebSocketServer({ noServer: true });
server.on("upgrade", (req, socket, head) => {
  const match = /^\/api\/servers\/([^/]+)\/ws/.exec(new URL(req.url, "http://x").pathname);
  if (!match) return socket.destroy();
  sockets.handleUpgrade(req, socket, head, (ws) => sockets.emit("connection", ws, match[1]));
});

const esc = "\u001b[";
const history = (name) => [
  `[lodestone] Server gestartet`,
  `[08:00:11] [main/INFO]: Starting minecraft server version 1.21.4`,
  `[08:00:12] [Server thread/INFO]: Loading properties`,
  `[08:00:13] [Server thread/INFO]: Default game type: SURVIVAL`,
  `[08:00:14] [Server thread/WARN]: ${esc}33mDie Konfiguration ist veraltet${esc}0m`,
  `[08:00:15] [Server thread/INFO]: Preparing level "world"`,
  `[08:00:16] [Server thread/INFO]: Done (3.412s)! For help, type "help"`,
  `[08:02:41] [Server thread/INFO]: Alex joined the game`,
  `[08:03:02] [Server thread/INFO]: <Alex> hallo zusammen`,
  `[08:05:44] [Server thread/INFO]: Luna joined the game`,
  `[08:06:10] [Server thread/ERROR]: ${esc}31mCould not pass event PlayerMoveEvent to ${name}${esc}0m`,
];

sockets.on("connection", (ws, serverId) => {
  const found = findServer(serverId);
  const lines = history(found?.server.name ?? "Server").map((text, index) => ({ seq: index + 1, time: now() - (20 - index) * 1000, text }));
  let seqNo = lines.length;
  ws.send(JSON.stringify({ type: "hello", state: found?.server.view.state ?? "offline", lines, stats: found?.server.view.stats }));
  const timer = setInterval(() => {
    if (ws.readyState !== 1) return;
    const stats = found?.server.view.stats;
    if (stats) ws.send(JSON.stringify({ type: "stats", stats: { ...stats, cpuPercent: 8 + Math.random() * 40 } }));
    if (Math.random() < 0.5) ws.send(JSON.stringify({ type: "line", line: { seq: ++seqNo, time: now(), text: `[${new Date().toLocaleTimeString("de-DE")}] [Server thread/INFO]: ${names[Math.floor(Math.random() * names.length)]} hat sich bewegt` } }));
  }, 2000);
  ws.on("message", (data) => {
    try {
      const message = JSON.parse(data.toString());
      if (message.type === "command") ws.send(JSON.stringify({ type: "line", line: { seq: ++seqNo, time: now(), text: `> ${message.data}\n[Server thread/INFO]: (Demo) Befehl empfangen` } }));
    } catch {}
  });
  ws.on("close", () => clearInterval(timer));
});

server.listen(port, "127.0.0.1", () => console.log(`Lodestone-Demo läuft auf http://localhost:${port}`));
