import type { Metadata, Viewport } from "next";
import type { ReactNode } from "react";
import { ToastProvider } from "@/components/ui/feedback";
import "./globals.css";

export const metadata: Metadata = {
  title: { default: "Lodestone", template: "%s · Lodestone" },
  description: "Hosting-Panel für Minecraft-Server und -Netzwerke",
};

export const viewport: Viewport = {
  themeColor: "#111111",
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="de">
      <body>
        <ToastProvider>{children}</ToastProvider>
      </body>
    </html>
  );
}
