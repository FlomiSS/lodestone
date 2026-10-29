"use client";

import useSWR, { type SWRConfiguration } from "swr";

/** Fehler der Schnittstelle mit der Meldung des Panels. */
export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message);
  }
}

/** Im Entwicklungsmodus (Port 3100) läuft das Panel auf 8480, sonst liegt alles auf derselben Adresse. */
export function apiBase(): string {
  if (typeof window === "undefined") return "";
  return window.location.port === "3100" ? `http://${window.location.hostname}:8480` : "";
}

export function wsBase(): string {
  const base = apiBase() || window.location.origin;
  return base.replace(/^http/, "ws");
}

function toLogin() {
  if (window.location.pathname.startsWith("/login")) return;
  const next = encodeURIComponent(window.location.pathname + window.location.search);
  window.location.replace(`/login/?next=${next}`);
}

async function failure(response: Response): Promise<ApiError> {
  const data = (await response.json().catch(() => null)) as { error?: string } | null;
  return new ApiError(data?.error ?? `Fehler ${response.status}`, response.status);
}

export async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { "X-Lodestone": "1" };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const response = await fetch(`${apiBase()}/api${path}`, {
    method,
    headers,
    credentials: "include",
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (response.status === 401 && !path.startsWith("/auth/")) {
    toLogin();
    throw new ApiError("Nicht angemeldet", 401);
  }
  if (!response.ok) throw await failure(response);
  if (response.status === 204) return undefined as T;
  const text = await response.text();
  return (text ? JSON.parse(text) : undefined) as T;
}

export const api = {
  get: <T,>(path: string) => request<T>("GET", path),
  post: <T,>(path: string, body?: unknown) => request<T>("POST", path, body ?? {}),
  put: <T,>(path: string, body?: unknown) => request<T>("PUT", path, body ?? {}),
  patch: <T,>(path: string, body?: unknown) => request<T>("PATCH", path, body ?? {}),
  delete: <T,>(path: string) => request<T>("DELETE", path),
};

/** Text einer Datei (kein JSON). */
export async function fetchText(path: string): Promise<string> {
  const response = await fetch(`${apiBase()}/api${path}`, { credentials: "include" });
  if (response.status === 401) {
    toLogin();
    throw new ApiError("Nicht angemeldet", 401);
  }
  if (!response.ok) throw await failure(response);
  return response.text();
}

export async function putText(path: string, text: string): Promise<void> {
  const response = await fetch(`${apiBase()}/api${path}`, {
    method: "PUT",
    headers: { "X-Lodestone": "1", "Content-Type": "text/plain; charset=utf-8" },
    credentials: "include",
    body: text,
  });
  if (!response.ok) throw await failure(response);
}

/** Lädt Dateien hoch und meldet den Fortschritt (0..1). */
export function upload(path: string, files: File[], onProgress?: (fraction: number) => void): Promise<void> {
  return new Promise((resolve, reject) => {
    const form = new FormData();
    for (const file of files) form.append("file", file, file.name);
    const xhr = new XMLHttpRequest();
    xhr.open("POST", `${apiBase()}/api${path}`);
    xhr.withCredentials = true;
    xhr.setRequestHeader("X-Lodestone", "1");
    xhr.upload.onprogress = (event) => event.lengthComputable && onProgress?.(event.loaded / event.total);
    xhr.onerror = () => reject(new ApiError("Der Upload ist fehlgeschlagen", 0));
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) return resolve();
      let message = `Fehler ${xhr.status}`;
      try {
        message = (JSON.parse(xhr.responseText) as { error?: string }).error ?? message;
      } catch {}
      reject(new ApiError(message, xhr.status));
    };
    xhr.send(form);
  });
}

export function downloadUrl(path: string): string {
  return `${apiBase()}/api${path}`;
}

export function useApi<T>(path: string | null, config?: SWRConfiguration<T, ApiError>) {
  return useSWR<T, ApiError>(path, (key: string) => api.get<T>(key), {
    revalidateOnFocus: false,
    shouldRetryOnError: false,
    ...config,
  });
}
