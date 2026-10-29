"use client";

import { AlertTriangle, Loader2, RefreshCw } from "lucide-react";
import { Button, Card } from "./primitives";

export function Loading({ label = "Daten werden geladen" }: { label?: string }) {
  return (
    <div className="flex items-center justify-center gap-2 py-16 text-sm text-muted">
      <Loader2 className="h-4 w-4 animate-spin" />
      {label}
    </div>
  );
}

export function ErrorState({ error, retry }: { error: Error; retry?: () => void }) {
  return (
    <Card className="flex flex-col items-center gap-3 px-6 py-12 text-center">
      <span className="rounded-xl border border-bad/30 bg-bad/10 p-3 text-bad">
        <AlertTriangle className="h-5 w-5" />
      </span>
      <div>
        <p className="text-sm font-medium">Daten konnten nicht geladen werden</p>
        <p className="mx-auto mt-1 max-w-md text-xs text-muted">{error.message}</p>
      </div>
      {retry && (
        <Button size="sm" onClick={retry}>
          <RefreshCw className="h-3.5 w-3.5" />
          Erneut versuchen
        </Button>
      )}
    </Card>
  );
}
