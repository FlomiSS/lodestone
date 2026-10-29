"use client";

import { ArrowRight, Eye, EyeOff, Loader2, Lock, User } from "lucide-react";
import { useEffect, useState, type FormEvent } from "react";
import { LodestoneLogo, Wordmark } from "@/components/ui/mc";
import { api } from "@/lib/api";
import type { AuthStatus } from "@/lib/types";

export default function LoginPage() {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [visible, setVisible] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [setup, setSetup] = useState(false);

  useEffect(() => {
    api
      .get<AuthStatus>("/auth/status")
      .then((status) => {
        if (status.user) window.location.replace(nextTarget());
        setSetup(status.setupRequired);
      })
      .catch(() => null);
  }, []);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await api.post(setup ? "/auth/setup" : "/auth/login", { username, password });
      window.location.replace(nextTarget());
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : "Anmeldung fehlgeschlagen");
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="relative flex min-h-screen items-center justify-center overflow-hidden px-4 py-10">
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_center,rgba(255,255,255,0.04)_1px,transparent_1px)] [background-size:26px_26px] [mask-image:radial-gradient(ellipse_at_center,black,transparent_70%)]" />

      <div className="relative w-full max-w-[400px]">
        <div className="mb-7 flex flex-col items-center text-center">
          <LodestoneLogo size={56} className="mb-4" />
          <Wordmark className="text-xs text-muted" />
          <h1 className="mt-1.5 text-2xl font-semibold tracking-tight">{setup ? "Einrichtung" : "Willkommen zurück"}</h1>
          <p className="mt-1.5 text-sm text-muted">
            {setup ? "Lege den ersten Administrator an." : "Melde dich an, um deine Server zu verwalten."}
          </p>
        </div>

        <form onSubmit={submit} className="rounded-xl border border-white/[0.08] bg-surface p-6 shadow-panel">
          <div className="space-y-4">
            <div>
              <label className="label" htmlFor="username">
                Benutzername
              </label>
              <div className="relative">
                <User className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted/70" />
                <input id="username" className="field pl-9" autoComplete="username" autoFocus value={username} onChange={(event) => setUsername(event.target.value)} placeholder="admin" required />
              </div>
            </div>

            <div>
              <label className="label" htmlFor="password">
                Passwort
              </label>
              <div className="relative">
                <Lock className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted/70" />
                <input
                  id="password"
                  className="field pl-9 pr-10"
                  type={visible ? "text" : "password"}
                  autoComplete={setup ? "new-password" : "current-password"}
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  placeholder={setup ? "mindestens 8 Zeichen" : "••••••••"}
                  required
                />
                <button
                  type="button"
                  onClick={() => setVisible((value) => !value)}
                  className="absolute right-2 top-1/2 -translate-y-1/2 rounded-md p-1.5 text-muted transition hover:bg-white/[0.06] hover:text-ink"
                  aria-label={visible ? "Passwort verbergen" : "Passwort anzeigen"}
                >
                  {visible ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </button>
              </div>
            </div>

            {error && <p className="rounded-lg border border-bad/25 bg-bad/10 px-3 py-2 text-xs leading-relaxed text-bad">{error}</p>}

            <button
              type="submit"
              disabled={busy}
              className="group inline-flex h-10 w-full items-center justify-center gap-2 rounded-lg bg-accent text-sm font-semibold text-white transition duration-100 hover:bg-accent/90 active:scale-[0.97] active:brightness-90 disabled:cursor-not-allowed disabled:opacity-60 disabled:active:scale-100"
            >
              {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
              {busy ? "Einen Moment" : setup ? "Administrator anlegen" : "Anmelden"}
              {!busy && <ArrowRight className="h-4 w-4 transition group-hover:translate-x-0.5" />}
            </button>
          </div>
        </form>
      </div>
    </main>
  );
}

function nextTarget(): string {
  const next = new URLSearchParams(window.location.search).get("next");
  return next && next.startsWith("/") && !next.startsWith("//") ? next : "/";
}
