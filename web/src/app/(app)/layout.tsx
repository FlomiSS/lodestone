import type { ReactNode } from "react";
import { AuthGate } from "@/components/layout/shell";

export default function AppLayout({ children }: { children: ReactNode }) {
  return <AuthGate>{children}</AuthGate>;
}
