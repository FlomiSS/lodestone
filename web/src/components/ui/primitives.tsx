"use client";

import { Check, Loader2, Search } from "lucide-react";
import { useEffect, useRef, useState, type ButtonHTMLAttributes, type HTMLAttributes, type InputHTMLAttributes, type ReactNode } from "react";

export function cx(...parts: (string | false | null | undefined)[]): string {
  return parts.filter(Boolean).join(" ");
}

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "secondary" | "ghost" | "danger";
  size?: "sm" | "md" | "lg" | "icon" | "icon-sm";
  loading?: boolean;
};

export function Button({ variant = "secondary", size = "md", loading, className, children, disabled, ...rest }: ButtonProps) {
  const variants = {
    primary: "bg-accent text-white hover:bg-accent/90 shadow-glow",
    secondary: "border border-white/[0.09] bg-white/[0.03] text-ink hover:border-white/[0.16] hover:bg-white/[0.06]",
    ghost: "text-muted hover:bg-white/[0.05] hover:text-ink",
    danger: "border border-bad/30 bg-bad/10 text-bad hover:bg-bad/20",
  };
  const sizes = {
    sm: "h-8 px-3 text-xs",
    md: "h-9 px-4 text-sm",
    lg: "h-10 px-4 text-sm",
    icon: "h-10 w-10",
    "icon-sm": "h-8 w-8",
  };
  return (
    <button
      {...rest}
      disabled={disabled || loading}
      className={cx(
        "inline-flex select-none items-center justify-center gap-2 rounded-lg font-medium transition duration-100 ease-out active:scale-[0.95] active:brightness-90 disabled:cursor-not-allowed disabled:opacity-50 disabled:active:scale-100 disabled:active:brightness-100",
        variants[variant],
        sizes[size],
        className,
      )}
    >
      {loading && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
      {children}
    </button>
  );
}

export function Card({ className, children, ...rest }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div {...rest} className={cx("rounded-xl border border-white/[0.07] bg-surface shadow-panel", className)}>
      {children}
    </div>
  );
}

export function CardHeader({ title, description, action }: { title: string; description?: string; action?: ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-4 border-b border-white/[0.06] px-5 py-4">
      <div>
        <h2 className="text-[13px] font-semibold tracking-tight text-ink">{title}</h2>
        {description && <p className="mt-0.5 text-[11px] text-muted">{description}</p>}
      </div>
      {action}
    </div>
  );
}

type Tone = "neutral" | "good" | "warn" | "bad" | "info" | "accent";

const tones: Record<Tone, string> = {
  neutral: "border-white/[0.08] bg-white/[0.04] text-muted",
  good: "border-good/25 bg-good/10 text-good",
  warn: "border-warn/25 bg-warn/10 text-warn",
  bad: "border-bad/25 bg-bad/10 text-bad",
  info: "border-info/25 bg-info/10 text-info",
  accent: "border-accent/25 bg-accent/10 text-accent",
};

export function Badge({ tone = "neutral", children, className }: { tone?: Tone; children: ReactNode; className?: string }) {
  return (
    <span className={cx("inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[10.5px] font-medium", tones[tone], className)}>
      {children}
    </span>
  );
}

export function Dot({ tone }: { tone: "good" | "bad" | "warn" | "neutral" }) {
  const colors = { good: "bg-good", bad: "bg-bad", warn: "bg-warn", neutral: "bg-muted" };
  return <span className={cx("inline-block h-1.5 w-1.5 rounded-full", colors[tone], tone === "good" && "shadow-[0_0_8px] shadow-good/60")} />;
}

export function Switch({ checked, onChange, disabled }: { checked: boolean; onChange: (value: boolean) => void; disabled?: boolean }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={cx(
        "group relative h-6 w-11 shrink-0 rounded-full border transition duration-100 active:scale-95 disabled:opacity-50",
        checked ? "border-accent/60 bg-accent" : "border-white/[0.08] bg-white/[0.07]",
      )}
    >
      <span
        className={cx(
          "absolute top-0.5 h-5 w-5 rounded-full bg-white shadow transition-all duration-150 group-active:w-6",
          checked ? "left-[22px] group-active:left-[18px]" : "left-0.5",
        )}
      />
    </button>
  );
}

export function PageHeader({ title, description, action }: { title: string; description?: string; action?: ReactNode }) {
  return (
    <div className="mb-6 flex flex-wrap items-center justify-between gap-4">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">{title}</h1>
        {description && <p className="mt-1 text-[13px] text-muted">{description}</p>}
      </div>
      {action}
    </div>
  );
}

export type StatTone = "green" | "blue" | "violet" | "amber" | "red" | "neutral";

// Farbig getönte Karten mit passendem Rand, wie in modernen Cloud-Dashboards
const statTones: Record<StatTone, { card: string; icon: string; value: string }> = {
  green: {
    card: "border-emerald-500/40 bg-gradient-to-br from-emerald-500/[0.14] via-emerald-500/[0.05] to-emerald-500/[0.02]",
    icon: "text-emerald-400",
    value: "text-emerald-400",
  },
  blue: {
    card: "border-sky-500/40 bg-gradient-to-br from-sky-500/[0.14] via-sky-500/[0.05] to-sky-500/[0.02]",
    icon: "text-sky-400",
    value: "text-ink",
  },
  violet: {
    card: "border-violet-500/40 bg-gradient-to-br from-violet-500/[0.16] via-violet-500/[0.05] to-violet-500/[0.02]",
    icon: "text-violet-400",
    value: "text-ink",
  },
  amber: {
    card: "border-amber-500/40 bg-gradient-to-br from-amber-500/[0.14] via-amber-500/[0.05] to-amber-500/[0.02]",
    icon: "text-amber-400",
    value: "text-ink",
  },
  red: {
    card: "border-red-500/40 bg-gradient-to-br from-red-500/[0.14] via-red-500/[0.05] to-red-500/[0.02]",
    icon: "text-red-400",
    value: "text-red-400",
  },
  neutral: {
    card: "border-white/[0.08] bg-gradient-to-br from-white/[0.05] to-white/[0.01]",
    icon: "text-muted",
    value: "text-ink",
  },
};

export function StatCard({
  label,
  value,
  hint,
  icon,
  tone = "neutral",
}: {
  label: string;
  value: ReactNode;
  hint?: ReactNode;
  icon: ReactNode;
  tone?: StatTone;
}) {
  const style = statTones[tone];
  return (
    <div className={cx("rounded-xl border p-5", style.card)}>
      <div className="flex items-center justify-between gap-3">
        <p className="text-[13px] text-ink/80">{label}</p>
        <span className={style.icon}>{icon}</span>
      </div>
      <p className={cx("mt-5 text-[26px] font-semibold leading-none tracking-tight", style.value)}>{value}</p>
      {hint && <p className="mt-3 truncate text-xs text-ink/55">{hint}</p>}
    </div>
  );
}

export function EmptyState({ icon, title, text }: { icon: ReactNode; title: string; text?: string }) {
  return (
    <div className="flex flex-col items-center justify-center px-6 py-14 text-center">
      <div className="mb-3 rounded-xl border border-white/[0.07] bg-white/[0.03] p-3 text-muted">{icon}</div>
      <p className="text-sm font-medium">{title}</p>
      {text && <p className="mt-1 max-w-sm text-xs text-muted">{text}</p>}
    </div>
  );
}

export function Spinner() {
  return <Loader2 className="h-5 w-5 animate-spin text-muted" />;
}

export function Tabs<T extends string>({ value, onChange, items }: { value: T; onChange: (value: T) => void; items: { id: T; label: string; count?: number }[] }) {
  return (
    <div className="inline-flex rounded-lg border border-white/[0.07] bg-white/[0.025] p-0.5">
      {items.map((item) => (
        <button
          key={item.id}
          onClick={() => onChange(item.id)}
          className={cx(
            "rounded-md px-3 py-1.5 text-xs font-medium transition duration-100 active:scale-95",
            value === item.id ? "bg-white/[0.09] text-ink shadow-sm" : "text-muted hover:text-ink",
          )}
        >
          {item.label}
          {item.count !== undefined && <span className="ml-1.5 text-muted">{item.count}</span>}
        </button>
      ))}
    </div>
  );
}

export function Meter({ value, max, tone = "accent" }: { value: number; max: number; tone?: "accent" | "good" | "warn" | "bad" }) {
  const percent = max <= 0 ? 0 : Math.min(100, Math.max(0, (value / max) * 100));
  const colors = { accent: "bg-accent", good: "bg-good", warn: "bg-warn", bad: "bg-bad" };
  return (
    <div className="h-1.5 overflow-hidden rounded-full bg-white/[0.07]">
      <div className={cx("h-full rounded-full transition-all", colors[tone])} style={{ width: `${percent}%` }} />
    </div>
  );
}

/** Farbe nach Auslastung: grün, ab 75 % gelb, ab 90 % rot. */
export function loadTone(percent: number): "good" | "warn" | "bad" {
  return percent >= 90 ? "bad" : percent >= 75 ? "warn" : "good";
}

export function SearchInput({ className, ...rest }: Omit<InputHTMLAttributes<HTMLInputElement>, "type">) {
  return (
    <div
      className={cx(
        "flex items-center gap-2.5 rounded-lg border border-white/[0.08] bg-white/[0.025] px-3 transition focus-within:border-accent/60 focus-within:ring-2 focus-within:ring-accent/20",
        className,
      )}
    >
      <Search className="h-4 w-4 shrink-0 text-muted" />
      <input
        {...rest}
        type="search"
        className="h-10 w-full bg-transparent text-sm text-ink outline-none placeholder:text-muted/70 [&::-webkit-search-cancel-button]:hidden"
      />
    </div>
  );
}

export function Checkbox({ checked, onChange, label }: { checked: boolean; onChange: (value: boolean) => void; label?: string }) {
  return (
    <button
      type="button"
      role="checkbox"
      aria-checked={checked}
      aria-label={label}
      onClick={(event) => {
        event.stopPropagation();
        onChange(!checked);
      }}
      className={cx(
        "grid h-4 w-4 shrink-0 place-items-center rounded border transition duration-100 active:scale-75",
        checked ? "border-accent bg-accent text-white" : "border-white/[0.18] bg-transparent hover:border-white/40",
      )}
    >
      {checked && <Check className="h-3 w-3" strokeWidth={3} />}
    </button>
  );
}

export interface MenuItem {
  label: string;
  icon?: ReactNode;
  onClick: () => void;
  danger?: boolean;
  disabled?: boolean;
  hint?: string;
}

/** Kleines Aufklappmenü (⋯) für Zeilenaktionen. */
export function RowMenu({ items, label = "Aktionen" }: { items: (MenuItem | "separator")[]; label?: string }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const close = (event: MouseEvent) => {
      if (!ref.current?.contains(event.target as Node)) setOpen(false);
    };
    const escape = (event: KeyboardEvent) => event.key === "Escape" && setOpen(false);
    window.addEventListener("mousedown", close);
    window.addEventListener("keydown", escape);
    return () => {
      window.removeEventListener("mousedown", close);
      window.removeEventListener("keydown", escape);
    };
  }, [open]);

  return (
    <div ref={ref} className="relative" onClick={(event) => event.stopPropagation()}>
      <button
        type="button"
        aria-label={label}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
        className="grid h-8 w-8 place-items-center rounded-lg text-muted transition duration-100 hover:bg-white/[0.06] hover:text-ink active:scale-90 active:bg-white/[0.1]"
      >
        <span className="text-lg leading-none tracking-widest">···</span>
      </button>
      {open && (
        <div
          role="menu"
          className="absolute right-0 top-9 z-30 min-w-[210px] animate-fade-in rounded-lg border border-white/[0.09] bg-raised p-1 shadow-2xl shadow-black/60"
        >
          {items.map((item, index) =>
            item === "separator" ? (
              <div key={`separator-${index}`} className="my-1 h-px bg-white/[0.07]" />
            ) : (
              <button
                key={item.label}
                role="menuitem"
                type="button"
                disabled={item.disabled}
                title={item.hint}
                onClick={() => {
                  setOpen(false);
                  item.onClick();
                }}
                className={cx(
                  "flex w-full items-center gap-2.5 rounded-md px-2.5 py-2 text-left text-[13px] transition duration-100 active:scale-[0.98] active:bg-white/[0.1] disabled:cursor-not-allowed disabled:opacity-40 disabled:active:scale-100",
                  item.danger ? "text-bad hover:bg-bad/10" : "text-ink hover:bg-white/[0.06]",
                )}
              >
                {item.icon && <span className="text-muted">{item.icon}</span>}
                {item.label}
              </button>
            ),
          )}
        </div>
      )}
    </div>
  );
}
