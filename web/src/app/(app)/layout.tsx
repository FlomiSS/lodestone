import { Suspense, type ReactNode } from "react";
import { NavigationProvider } from "@/components/layout/navigation";
import { AuthGate } from "@/components/layout/shell";

export default function AppLayout({ children }: { children: ReactNode }) {
  return (
    <Suspense fallback={null}>
      <NavigationProvider>
        <AuthGate>{children}</AuthGate>
      </NavigationProvider>
    </Suspense>
  );
}
