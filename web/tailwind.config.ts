import type { Config } from "tailwindcss";

const withAlpha = (variable: string) => `rgb(var(${variable}) / <alpha-value>)`;

const config: Config = {
  content: ["./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        bg: withAlpha("--bg"),
        surface: withAlpha("--surface"),
        raised: withAlpha("--raised"),
        line: withAlpha("--line"),
        muted: withAlpha("--muted"),
        ink: withAlpha("--ink"),
        accent: withAlpha("--accent"),
        good: withAlpha("--good"),
        warn: withAlpha("--warn"),
        bad: withAlpha("--bad"),
        info: withAlpha("--info"),
      },
      fontFamily: {
        sans: ["Geist", "Inter", "ui-sans-serif", "system-ui", "-apple-system", "Segoe UI", "Roboto", "sans-serif"],
        mono: ["ui-monospace", "SFMono-Regular", "Menlo", "Consolas", "monospace"],
      },
      boxShadow: {
        glow: "0 6px 20px -8px rgb(var(--accent) / 0.55)",
        panel: "0 1px 0 0 rgb(255 255 255 / 0.02) inset",
      },
      keyframes: {
        "fade-in": {
          from: { opacity: "0", transform: "translateY(4px)" },
          to: { opacity: "1", transform: "translateY(0)" },
        },
      },
      animation: {
        "fade-in": "fade-in 0.25s ease-out backwards",
      },
    },
  },
  plugins: [],
};

export default config;
