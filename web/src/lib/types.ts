export type Role = "viewer" | "operator" | "admin" | "owner";
export const roleRank: Record<Role, number> = { viewer: 1, operator: 2, admin: 3, owner: 4 };
export const roleLabel: Record<Role, string> = { viewer: "Betrachter", operator: "Operator", admin: "Admin", owner: "Eigentümer" };

export interface User {
  id: string;
  username: string;
  isAdmin: boolean;
  disabled: boolean;
  createdAt: number;
}

export interface AuthStatus {
  setupRequired: boolean;
  version: string;
  user?: User;
}

export interface NodeInfo {
  id: string;
  name: string;
  url?: string;
  publicHost: string;
  online: boolean;
  error?: string;
  version?: string;
  serverCount: number;
  memoryAvailable?: number;
  memoryCommitted?: number;
  memoryUsed?: number;
  memoryTotal?: number;
  pingMs?: number;
  updateAvailable?: boolean;
  system?: { dockerVersion: string; os: string; architecture: string; cpus: number; memoryBytes: number };
}

export interface NodeDetail extends NodeInfo {
  servers: Server[];
  projects?: Record<string, string>;
}

export interface NodeUpdateStatus {
  id: string;
  name: string;
  version: string;
  updateAvailable: boolean;
  online: boolean;
}

export interface UpdateStatus {
  panelVersion: string;
  latestVersion: string;
  /** Nur das Panel ist veraltet; `updateAvailable` gilt für Panel oder irgendeinen Node. */
  panelUpdateAvailable: boolean;
  updateAvailable: boolean;
  /** Update-Kanal: `stable` bietet Releases an, `dev` den neuesten gebauten Stand von main. */
  channel: "stable" | "dev";
  /** Die neueste Version ließ sich nicht ermitteln: „kein Update“ heißt dann nur „unbekannt“. */
  checkFailed: boolean;
  releaseUrl?: string;
  releaseTitle?: string;
  releaseNotes?: string;
  publishedAt?: string;
  autoUpdate: boolean;
  lastChecked: number;
  nodes: NodeUpdateStatus[];
}

export type ServerState = "installing" | "offline" | "starting" | "running" | "stopping" | "restoring";

export interface Stats {
  cpuPercent: number;
  memoryBytes: number;
  memoryLimit: number;
  netRx: number;
  netTx: number;
}

export interface PortMapping {
  container: number;
  host: number;
  protocol: "tcp" | "udp";
}

export interface NodeView {
  id: string;
  name: string;
  software: string;
  version: string;
  memoryMB: number;
  cpu: number;
  ports: PortMapping[] | null;
  env?: Record<string, string>;
  startup?: string;
  image?: string;
  eula: boolean;
  noPublish?: boolean;
  backupKeep: number;
  installed: boolean;
  state: ServerState;
  lastError?: string;
  stats?: Stats;
  startedAt?: number;
  autoconfigPending?: boolean;
}

export type ServerRole = "standalone" | "proxy" | "backend";

export interface Server {
  id: string;
  projectId: string;
  nodeId: string;
  name: string;
  slug: string;
  role: ServerRole;
  software: string;
  inTry: boolean;
  tryPosition: number;
  forcedHost: string;
  voicechat: boolean;
  groupId?: string;
  nodeName: string;
  publicHost: string;
  online: boolean;
  address?: string;
  voiceAddress?: string;
  publicAddress?: string;
  publicVoiceAddress?: string;
  modpack?: ModpackInfo;
  autoRestart: boolean;
  view: NodeView | null;
}

export interface ModpackInfo {
  projectId: string;
  slug: string;
  title: string;
  iconUrl?: string;
  versionId: string;
  versionNumber: string;
  minecraft: string;
}

export interface ModpackStatus {
  state: "idle" | "running" | "done" | "failed";
  phase?: string;
  done: number;
  total: number;
  name?: string;
  version?: string;
  minecraft?: string;
  loader?: string;
  error?: string;
}

export interface ModpackVersion {
  id: string;
  name: string;
  versionNumber: string;
  type: string;
  published: string;
  gameVersions: string[];
}

export interface PlayitStatus {
  configured: boolean;
  running: boolean;
  image: string;
  logs: string[];
  error?: string;
  since?: number;
}

/** Stand der Verbindung eines Nodes mit playit (den Schlüssel bekommt der Browser nie). */
export interface PlayitClaim {
  state: "none" | "waiting" | "accepted" | "rejected" | "expired";
  url?: string;
}

export type TunnelKind = "java" | "voicechat";

export interface Tunnel {
  kind: TunnelKind;
  id: string;
  address: string;
  state: "allocated" | "pending" | "disabled";
  /** Name der gewählten Region; fehlt bei „Automatisch“. */
  region?: string;
  /** Eigene Domain des Tunnels; fehlt ohne eigene Domain. */
  domain?: string;
}

/** Was das playit-Konto beim Anlegen eines Tunnels erlaubt. */
export interface TunnelOptions {
  /** Regionen und eigene Domains gehören zu playit Premium. */
  premium: boolean;
  regions: { id: string; name: string }[];
  domains: { id: string; name: string; external: boolean; used: boolean }[];
  domainsProblem?: string;
}

export interface TunnelList {
  configured: boolean;
  tunnels: Tunnel[];
  available: { java: boolean; voicechat: boolean };
  problem?: string;
}

export interface PlayitNodeOverview {
  nodeId: string;
  nodeName: string;
  online: boolean;
  publicHost: string;
  status: PlayitStatus;
  error?: string;
}

export interface PlayitTunnelOverview {
  serverId: string;
  serverName: string;
  projectId: string;
  projectName: string;
  nodeId: string;
  nodeName: string;
  tunnel: Tunnel;
}

export interface PlayitOverview {
  nodes: PlayitNodeOverview[];
  tunnels: PlayitTunnelOverview[];
}

export interface AddonDetail {
  file: string;
  dir: string;
  known: boolean;
  projectId?: string;
  slug?: string;
  title?: string;
  description?: string;
  iconUrl?: string;
  projectType?: string;
  link?: string;
  downloads?: number;
  sourceUrl?: string;
  issuesUrl?: string;
  license?: string;
  versionId?: string;
  versionNumber?: string;
  versionName?: string;
  versionType?: string;
  published?: string;
  versionLink?: string;
}

export interface Member {
  projectId: string;
  userId: string;
  username: string;
  role: Role;
}

export interface Project {
  id: string;
  name: string;
  kind: "server" | "network";
  description: string;
  ownerId: string;
  createdAt: number;
  myRole: Role;
  voicechat: boolean;
  members?: Member[];
  servers: Server[];
  groups: Group[];
  settings?: ProjectSettings;
  problems?: string[];
}

export interface ProjectSettings {
  statusPublic: boolean;
  statusSlug: string;
  motd: string;
}

export interface Group {
  id: string;
  projectId: string;
  name: string;
  slug: string;
  software: string;
  version: string;
  memoryMB: number;
  minInstances: number;
  maxInstances: number;
  playersPerInstance: number;
  idleMinutes: number;
  inTry: boolean;
  dynamic: boolean;
  plugins: string[];
  instances: string[];
  active: number;
  players: number;
  note?: string;
}

export type Range = "1h" | "6h" | "24h" | "7d";

export interface Series {
  range: Range;
  times: number[];
  cpu: (number | null)[];
  memory: (number | null)[];
  players: (number | null)[];
  peakPlayers: number;
  avgPlayers: number;
  avgCpu: number;
  peakCpu: number;
  peakMemory: number;
  samples: number;
}

export interface Metrics {
  series: Series;
  players: number;
  maxPlayers?: number;
  names?: string[];
}

export interface SeenPlayer {
  name: string;
  firstSeen: number;
  lastSeen: number;
  seconds: number;
  online: boolean;
}

export interface SoftwareInfo {
  id: string;
  name: string;
  kind: "server" | "proxy";
  extensions: "plugins" | "mods" | "none";
  defaultPort: number;
  startup?: string;
  stopCommand?: string;
  readyPattern?: string;
  description?: string;
  canBeBackend?: boolean;
  canVoice?: boolean;
  custom?: boolean;
}

export interface PackSetOp {
  path: string[];
  value: unknown;
}

export interface PackEdit {
  file: string;
  format: "properties" | "yaml" | "toml";
  create?: boolean;
  set?: PackSetOp[];
}

export interface PackWrite {
  path: string;
  content: string;
}

export interface PackAutoconfigTarget {
  edits?: PackEdit[];
  writes?: PackWrite[];
  restart?: boolean;
  reload?: string;
}

export interface PackAutoconfigVoice {
  file: string;
  format: "properties" | "yaml" | "toml" | string;
  set?: PackSetOp[];
}

export interface PositionPackSummary {
  id: string;
  name: string;
  description?: string;
  version?: string;
  author?: string;
  kind: "server" | "proxy";
  extensions: "plugins" | "mods" | "none";
  defaultPort: number;
  custom: boolean;
  overridden?: boolean;
  zipSize: number;
  hasAutoconfig: boolean;
  hasScript: boolean;
  canBeBackend: boolean;
  canVoice: boolean;
}

export interface PositionPackDetail {
  meta: {
    id: string;
    name: string;
    description?: string;
    version?: string;
    author?: string;
    kind: "server" | "proxy";
    extensions: "plugins" | "mods" | "none";
    defaultPort: number;
    startup: string;
    stopCommand: string;
    readyPattern: string;
    java?: {
      strategy?: string;
      major?: number;
    };
    provider?: {
      type?: string;
      versions?: string[];
      downloadUrl?: string;
      fileName?: string;
    };
    backend?: {
      supported?: boolean;
      proxyMod?: string;
      voiceConfigFile?: string;
    };
  };
  autoconfig: {
    backend?: PackAutoconfigTarget;
    proxy?: PackAutoconfigTarget;
    standalone?: PackAutoconfigTarget;
    voicechat?: PackAutoconfigVoice;
  };
  script?: string;
  custom: boolean;
  overridden?: boolean;
  zipSize: number;
}

export interface FileEntry {
  name: string;
  dir: boolean;
  symlink?: boolean;
  size: number;
  modified: number;
  mode: string;
}

export interface Addon {
  file: string;
  dir: string;
  size: number;
  modified: number;
  enabled: boolean;
  name: string;
  id?: string;
  version: string;
  kind: string;
  sha1: string;
}

export interface Backup {
  name: string;
  label?: string;
  size: number;
  created: number;
  creating?: boolean;
}

export interface PlayerInfo {
  online: number;
  max: number;
  names: string[];
  version: string;
  motd: string;
  latencyMs: number;
}

export interface SyncResult {
  applied: string[];
  pending: string[];
  failed: string[];
  warnings?: string[];
}

export interface ModrinthHit {
  project_id: string;
  slug: string;
  title: string;
  description: string;
  author: string;
  downloads: number;
  icon_url: string;
}

export interface UpdateEntry {
  file: string;
  dir: string;
  name: string;
  current: string;
  latest?: string;
  latestFile?: string;
  known: boolean;
  updateAvailable: boolean;
}

export interface LogLine {
  seq: number;
  time: number;
  text: string;
}

export interface Me extends User {
  totpEnabled: boolean;
  recoveryCodes: number;
}

export type StepAction = "backup" | "command" | "restart" | "start" | "stop" | "wait";

export interface ScheduleStep {
  action: StepAction;
  value?: string;
}

export interface Schedule {
  id: string;
  projectId: string;
  serverId: string;
  serverName: string;
  name: string;
  steps: ScheduleStep[];
  kind: "interval" | "daily" | "weekly";
  everyMinutes: number;
  atMinute: number;
  weekdays: number;
  warnMinutes: number;
  enabled: boolean;
  lastRun: number;
  lastStatus: "" | "ok" | "failed" | "skipped";
  lastMessage: string;
  nextRun: number;
}

export interface ProjectEvent {
  id: number;
  time: number;
  serverId?: string;
  kind: string;
  level: "info" | "warn" | "error";
  message: string;
}

export interface Webhook {
  id: string;
  name: string;
  url: string;
  kind: "discord" | "generic";
  events: string[];
  enabled: boolean;
  lastStatus: string;
}

export interface AuditEntry {
  id: number;
  time: number;
  userId?: string;
  username: string;
  action: string;
  target?: string;
  status: number;
  ip?: string;
}

export interface ApiKeyInfo {
  id: string;
  name: string;
  prefix: string;
  createdAt: number;
  lastUsed: number;
  expiresAt: number;
}

export interface SessionInfo {
  id: string;
  createdAt: number;
  expiresAt: number;
  userAgent: string;
  ip: string;
  current: boolean;
}

export interface ListedPlayer {
  uuid: string;
  name: string;
  level?: number;
  reason?: string;
  created?: string;
  source?: string;
  expires?: string;
}

export interface PlayerLists {
  whitelist: ListedPlayer[];
  ops: ListedPlayer[];
  banned: ListedPlayer[];
  bannedIps: { ip: string; reason?: string; created?: string }[];
}

export interface PublicStatus {
  name: string;
  description?: string;
  online: boolean;
  address?: string;
  players: number;
  maxPlayers?: number;
  version?: string;
  servers: { name: string; online: boolean; players: number; maxPlayers?: number }[];
  updated: number;
}

export interface AdminOverview {
  users: number;
  admins: number;
  disabledUsers: number;
  projects: number;
  servers: number;
  nodes: number;
  members: number;
  sessions: number;
}

export interface AdminUser {
  id: string;
  username: string;
  isAdmin: boolean;
  disabled: boolean;
  createdAt: number;
  totpEnabled: boolean;
  projectCount: number;
  sessionCount: number;
}

export interface AdminUserProject {
  id: string;
  name: string;
  kind: "server" | "network";
  role: Role;
}

export interface AdminUserDetail {
  user: AdminUser;
  projects: AdminUserProject[];
}

export interface AdminProject {
  id: string;
  name: string;
  kind: "server" | "network";
  description: string;
  createdAt: number;
  ownerId: string;
  ownerName: string;
  memberCount: number;
  serverCount: number;
}

