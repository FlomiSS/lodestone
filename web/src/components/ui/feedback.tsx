"use client";

import { AlertTriangle, CheckCircle2, X } from "lucide-react";
import { createPortal } from "react-dom";
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type Context, type ReactNode } from "react";
import { ApiError } from "@/lib/api";
import { Button, cx } from "./primitives";

// ---------------------------------------------------------------- Meldungen

interface Toast {
  id: number;
  tone: "good" | "bad";
  text: string;
}

interface ToastApi {
  success: (text: string) => void;
  error: (text: string) => void;
  /** Führt eine Aktion aus und zeigt Erfolg oder die Fehlermeldung an. Gibt zurück, ob sie funktioniert hat. */
  run: <T,>(action: () => Promise<T>, success?: string) => Promise<boolean>;
}

// Der Kontext hängt an globalThis. Dadurch teilen sich alle Kopien dieses Moduls denselben Kontext, auch wenn ein
// offener Tab nach einem Update gemischte Dateien alter und neuer Versionen geladen hat.
const CONTEXT_KEY = "__lodestoneToastContext";
const holder = globalThis as unknown as Record<string, Context<ToastApi | null> | undefined>;
const ToastContext = (holder[CONTEXT_KEY] ??= createContext<ToastApi | null>(null));

/** Notlösung ohne Anzeige: die Seite soll weiter funktionieren, auch wenn der Provider fehlt. */
const fallback: ToastApi = {
  success: (text) => console.info(text),
  error: (text) => console.error(text),
  run: async (action, success) => {
    try {
      await action();
      if (success) console.info(success);
      return true;
    } catch (error) {
      console.error(error);
      return false;
    }
  },
};

export function useToast(): ToastApi {
  return useContext(ToastContext) ?? fallback;
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const push = useCallback((tone: Toast["tone"], text: string) => {
    const id = Date.now() + Math.random();
    setToasts((current) => [...current, { id, tone, text }]);
    setTimeout(() => setToasts((current) => current.filter((toast) => toast.id !== id)), 4500);
  }, []);

  const api = useMemo<ToastApi>(
    () => ({
      success: (text) => push("good", text),
      error: (text) => push("bad", text),
      run: async (action, success) => {
        try {
          await action();
          if (success) push("good", success);
          return true;
        } catch (error) {
          push("bad", error instanceof ApiError || error instanceof Error ? error.message : "Unbekannter Fehler");
          return false;
        }
      },
    }),
    [push],
  );

  return (
    <ToastContext.Provider value={api}>
      {children}
      <div className="pointer-events-none fixed bottom-5 right-5 z-[60] flex w-80 flex-col gap-2">
        {toasts.map((toast) => (
          <div
            key={toast.id}
            className={cx(
              "pointer-events-auto flex items-start gap-3 rounded-lg border bg-raised px-4 py-3 text-sm shadow-2xl shadow-black/50",
              toast.tone === "good" ? "border-good/40" : "border-bad/40",
            )}
          >
            {toast.tone === "good" ? (
              <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-good" />
            ) : (
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-bad" />
            )}
            <span className="flex-1">{toast.text}</span>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

// ---------------------------------------------------------------- Dialoge

export function Dialog({
  open,
  onClose,
  title,
  description,
  children,
  footer,
  wide,
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  description?: string;
  children: ReactNode;
  footer?: ReactNode;
  wide?: boolean;
}) {
  useEffect(() => {
    if (!open) return;
    const handler = (event: KeyboardEvent) => event.key === "Escape" && onClose();
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [open, onClose]);

  if (!open) return null;
  // Über ein Portal direkt unter body, damit weder Animationen noch Scroll-Container die Position verschieben
  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" onClick={onClose} />
      <div className={cx("relative max-h-[calc(100vh-2rem)] w-full animate-fade-in overflow-y-auto rounded-xl border border-white/[0.09] bg-surface shadow-2xl shadow-black/60", wide ? "max-w-2xl" : "max-w-md")}>
        <div className="flex items-start justify-between gap-4 border-b px-5 py-4">
          <div>
            <h3 className="text-base font-semibold">{title}</h3>
            {description && <p className="mt-0.5 text-xs text-muted">{description}</p>}
          </div>
          <button onClick={onClose} className="rounded-md p-1 text-muted hover:bg-raised hover:text-ink" aria-label="Schließen">
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="space-y-4 px-5 py-4">{children}</div>
        {footer && <div className="flex justify-end gap-2 border-t px-5 py-3">{footer}</div>}
      </div>
    </div>,
    document.body,
  );
}

export function ConfirmDialog({
  open,
  onClose,
  onConfirm,
  title,
  text,
  confirm = "Bestätigen",
  danger,
}: {
  open: boolean;
  onClose: () => void;
  onConfirm: () => Promise<void> | void;
  title: string;
  text: string;
  confirm?: string;
  danger?: boolean;
}) {
  const [busy, setBusy] = useState(false);
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={title}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Abbrechen
          </Button>
          <Button
            variant={danger ? "danger" : "primary"}
            loading={busy}
            onClick={async () => {
              setBusy(true);
              try {
                await onConfirm();
              } finally {
                setBusy(false);
                onClose();
              }
            }}
          >
            {confirm}
          </Button>
        </>
      }
    >
      <p className="text-sm text-muted">{text}</p>
    </Dialog>
  );
}
