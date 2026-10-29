import path from "node:path";
import { fileURLToPath } from "node:url";

/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  poweredByHeader: false,
  // Die Oberfläche wird als statische Dateien gebaut und vom Panel aus dem Binary ausgeliefert.
  output: "export",
  trailingSlash: true,
  images: { unoptimized: true },
  outputFileTracingRoot: path.dirname(fileURLToPath(import.meta.url)),
};

export default nextConfig;
