export function bytes(value?: number | null): string {
  if (!value || isNaN(value)) return "0 MB";
  if (value >= 1024 ** 3) return `${(value / 1024 ** 3).toFixed(1).replace(".", ",")} GB`;
  if (value >= 1024 ** 2) return `${(value / 1024 ** 2).toFixed(value >= 100 * 1024 ** 2 ? 0 : 1).replace(".", ",")} MB`;
  if (value >= 1024) return `${Math.round(value / 1024)} KB`;
  return `${value} B`;
}

export function megabytes(value: number): string {
  return value >= 1024 ? `${(value / 1024).toFixed(value % 1024 === 0 ? 0 : 1).replace(".", ",")} GB` : `${value} MB`;
}

export function dateTime(timestamp?: number | null): string {
  if (!timestamp || isNaN(timestamp)) return "–";
  return new Date(timestamp).toLocaleString("de-DE", { dateStyle: "short", timeStyle: "short" });
}

export function ago(timestamp?: number | null): string {
  if (!timestamp || isNaN(timestamp)) return "nie";
  const seconds = Math.max(0, Math.round((Date.now() - timestamp) / 1000));
  if (seconds < 10) return "gerade eben";
  if (seconds < 60) return `vor ${seconds} Sek`;
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `vor ${minutes} Min`;
  const hours = Math.round(minutes / 60);
  if (hours < 48) return `vor ${hours} Std`;
  return `vor ${Math.round(hours / 24)} Tagen`;
}

export function uptime(since?: number | null): string {
  if (!since) return "–";
  const minutes = Math.floor((Date.now() - since) / 60_000);
  if (minutes < 1) return "gerade gestartet";
  if (minutes < 60) return `${minutes} Min`;
  const hours = Math.floor(minutes / 60);
  if (hours < 48) return `${hours} Std ${minutes % 60} Min`;
  return `${Math.floor(hours / 24)} Tage`;
}

export function compact(value: number): string {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1).replace(".", ",")} Mio.`;
  if (value >= 1_000) return `${(value / 1_000).toFixed(1).replace(".", ",")} Tsd.`;
  return String(value);
}

export const stateLabel: Record<string, string> = {
  installing: "Wird installiert",
  offline: "Aus",
  starting: "Startet",
  running: "Läuft",
  stopping: "Stoppt",
  restoring: "Wird wiederhergestellt",
};

export function stateTone(state?: string): "good" | "bad" | "warn" | "neutral" {
  switch (state) {
    case "running":
      return "good";
    case "starting":
    case "stopping":
    case "installing":
    case "restoring":
      return "warn";
    default:
      return "neutral";
  }
}

export function ansiStrip(text: string): string {
  // eslint-disable-next-line no-control-regex
  return text.replace(/\u001b\[[0-9;]*[A-Za-z]/g, "");
}

export function joinPath(dir: string, name: string): string {
  return dir === "/" || dir === "" ? name : `${dir.replace(/\/+$/, "")}/${name}`;
}

export function parentPath(path: string): string {
  const parts = path.split("/").filter(Boolean);
  parts.pop();
  return parts.join("/");
}
