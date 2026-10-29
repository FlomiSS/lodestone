"use client";

import { useId, useMemo, useState } from "react";
import { cx } from "./primitives";

/** Farben für Datenreihen; die letzte (grau) ist für "Weitere" reserviert. */
export const seriesColors = ["#3b82f6", "#f97316", "#10b981", "#eab308", "#ec4899", "#8b5cf6"];
export const otherColor = "#a3a3a3";

export interface ChartSeries {
  name: string;
  color: string;
  /** Ein Wert pro Zeitpunkt; null = keine Messung. */
  values: (number | null)[];
}

const VIEW_WIDTH = 1000;

/** Rundet einen Höchstwert auf eine Achsenobergrenze, deren Viertel glatte Zahlen sind (1, 2, 4, 8, 10 · 10ⁿ). */
export function niceMax(value: number): number {
  if (!isFinite(value) || value <= 0) return 1;
  const magnitude = 10 ** Math.floor(Math.log10(value));
  for (const step of [1, 2, 4, 8, 10]) {
    if (value <= step * magnitude) return step * magnitude;
  }
  return 10 * magnitude;
}

function clock(time: number): string {
  return new Date(time).toLocaleTimeString("de-DE", { hour: "2-digit", minute: "2-digit" });
}

function dayAndClock(time: number): string {
  return new Date(time).toLocaleString("de-DE", { day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" });
}

/** Weicher Linienzug ohne Überschwinger (waagerechte Tangenten an den Messpunkten). */
function smooth(points: { x: number; y: number }[]): string {
  let path = `M ${points[0].x.toFixed(2)} ${points[0].y.toFixed(2)}`;
  for (let index = 1; index < points.length; index++) {
    const a = points[index - 1];
    const b = points[index];
    const mid = (a.x + b.x) / 2;
    path += ` C ${mid.toFixed(2)} ${a.y.toFixed(2)}, ${mid.toFixed(2)} ${b.y.toFixed(2)}, ${b.x.toFixed(2)} ${b.y.toFixed(2)}`;
  }
  return path;
}

export function ChartLegend({ items }: { items: { name: string; color: string }[] }) {
  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-1.5">
      {items.map((item) => (
        <span key={item.name} className="inline-flex items-center gap-1.5 text-xs text-ink/80">
          <span className="h-2 w-2 rounded-[2px]" style={{ background: item.color }} />
          {item.name}
        </span>
      ))}
    </div>
  );
}

/**
 * Liniendiagramm mit mehreren Reihen, Achsenbeschriftung und Hover-Tooltip, ohne zusätzliche Bibliothek.
 * Mit `stacked` werden die Reihen aufeinander gestapelt (Summe = oberste Linie).
 */
export function SeriesChart({
  times,
  series,
  height = 220,
  stacked = false,
  fill = stacked || series.length === 1,
  yMax,
  long = false,
  format = (value: number) => String(Math.round(value * 10) / 10),
}: {
  times: number[];
  series: ChartSeries[];
  height?: number;
  stacked?: boolean;
  /** Flächen unter den Linien; bei mehreren überlappenden, nicht gestapelten Reihen aus, damit sie lesbar bleiben. */
  fill?: boolean;
  /** Feste Obergrenze der Y-Achse (z. B. 100 für Prozent); sonst automatisch. */
  yMax?: number;
  /** Zeitraum über mehrere Tage: Achse und Tooltip zeigen auch das Datum. */
  long?: boolean;
  format?: (value: number) => string;
}) {
  const id = useId().replace(/:/g, "");
  const [hover, setHover] = useState<number | null>(null);
  const count = times.length;

  const model = useMemo(() => {
    const raw = series.map((entry) => entry.values.map((value) => value ?? 0));
    const levels = raw.map((values, seriesIndex) =>
      values.map((value, index) => (stacked ? raw.slice(0, seriesIndex + 1).reduce((sum, row) => sum + row[index], 0) : value)),
    );
    const highest = Math.max(0, ...levels.flat());
    const top = yMax ?? niceMax(highest * 1.05);
    const x = (index: number) => (count < 2 ? 0 : (index / (count - 1)) * VIEW_WIDTH);
    const y = (value: number) => height - Math.min(1, Math.max(0, value / top)) * height;
    const paths = levels.map((level, seriesIndex) => {
      const points = level.map((value, index) => ({ x: x(index), y: y(value) }));
      const line = smooth(points);
      const floor = stacked && seriesIndex > 0 ? levels[seriesIndex - 1].map((value, index) => ({ x: x(index), y: y(value) })).reverse() : null;
      const area = floor
        ? `${line} L ${floor[0].x} ${floor[0].y} ${floor
            .slice(1)
            .map((point) => `L ${point.x.toFixed(2)} ${point.y.toFixed(2)}`)
            .join(" ")} Z`
        : `${line} L ${VIEW_WIDTH} ${height} L 0 ${height} Z`;
      return { line, area };
    });
    return { levels, raw, top, paths };
  }, [series, stacked, yMax, height, count]);

  const yTicks = [0, 0.25, 0.5, 0.75, 1].map((fraction) => ({ fraction, value: model.top * fraction }));
  const xTicks = useMemo(() => {
    if (count < 2) return [];
    const wanted = Math.min(6, count);
    return Array.from({ length: wanted }, (_, index) => Math.round((index / (wanted - 1)) * (count - 1)));
  }, [count]);

  // Zeichnen von oben nach unten, damit tiefere Reihen ihre Fläche über die darüberliegenden legen
  const order = stacked ? [...series.keys()].reverse() : [...series.keys()];

  function track(event: React.MouseEvent<HTMLDivElement>) {
    if (count < 2) return;
    const rect = event.currentTarget.getBoundingClientRect();
    const fraction = (event.clientX - rect.left) / rect.width;
    setHover(Math.min(count - 1, Math.max(0, Math.round(fraction * (count - 1)))));
  }

  const hoverLeft = hover === null || count < 2 ? 0 : (hover / (count - 1)) * 100;
  const hoverTotal = hover === null ? 0 : model.raw.reduce((sum, row) => sum + row[hover], 0);

  return (
    <div className="flex pt-2" style={{ height: height + 36 }}>
      <div className="relative w-12 shrink-0 text-right text-[11px] tabular-nums text-muted" style={{ height }}>
        {yTicks.map((tick) => (
          <span key={tick.fraction} className="absolute right-2 -translate-y-1/2" style={{ top: `${(1 - tick.fraction) * 100}%` }}>
            {format(tick.value)}
          </span>
        ))}
      </div>

      <div className="relative min-w-0 flex-1">
        <div className="relative cursor-crosshair" style={{ height }} onMouseMove={track} onMouseLeave={() => setHover(null)}>
          <svg viewBox={`0 0 ${VIEW_WIDTH} ${height}`} preserveAspectRatio="none" className="absolute inset-0 h-full w-full overflow-visible" role="img">
            <defs>
              {series.map((entry, index) => (
                <linearGradient key={entry.name} id={`${id}-${index}`} x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor={entry.color} stopOpacity={stacked ? 0.28 : 0.3} />
                  <stop offset="100%" stopColor={entry.color} stopOpacity={0.02} />
                </linearGradient>
              ))}
            </defs>
            {yTicks.map((tick) => (
              <line
                key={tick.fraction}
                x1={0}
                x2={VIEW_WIDTH}
                y1={height - tick.fraction * height}
                y2={height - tick.fraction * height}
                stroke="rgba(255,255,255,0.06)"
                strokeDasharray={tick.fraction === 0 ? undefined : "3 5"}
                vectorEffect="non-scaling-stroke"
              />
            ))}
            {count >= 2 &&
              order.map((index) => (
                <g key={series[index].name}>
                  {fill && <path d={model.paths[index].area} fill={`url(#${id}-${index})`} />}
                  <path
                    d={model.paths[index].line}
                    fill="none"
                    stroke={series[index].color}
                    strokeWidth={1.75}
                    strokeLinejoin="round"
                    strokeLinecap="round"
                    vectorEffect="non-scaling-stroke"
                  />
                </g>
              ))}
          </svg>

          {hover !== null && (
            <>
              <div className="pointer-events-none absolute inset-y-0 w-px bg-white/20" style={{ left: `${hoverLeft}%` }} />
              {series.map((entry, index) => (
                <span
                  key={entry.name}
                  className="pointer-events-none absolute h-2 w-2 -translate-x-1/2 -translate-y-1/2 rounded-full ring-2 ring-surface"
                  style={{ left: `${hoverLeft}%`, top: `${(1 - Math.min(1, model.levels[index][hover] / model.top)) * 100}%`, background: entry.color }}
                />
              ))}
              <div
                className={cx(
                  "pointer-events-none absolute top-2 z-10 min-w-[150px] rounded-lg border border-white/[0.1] bg-raised/95 px-3 py-2 text-xs shadow-xl backdrop-blur",
                  hoverLeft > 55 ? "-translate-x-[calc(100%+12px)]" : "translate-x-3",
                )}
                style={{ left: `${hoverLeft}%` }}
              >
                <p className="mb-1.5 font-medium text-ink">{long ? dayAndClock(times[hover]) : clock(times[hover])}</p>
                {series.map((entry, index) => (
                  <p key={entry.name} className="flex items-center justify-between gap-4 py-0.5">
                    <span className="inline-flex items-center gap-1.5 text-muted">
                      <span className="h-2 w-2 rounded-[2px]" style={{ background: entry.color }} />
                      {entry.name}
                    </span>
                    <span className="tabular-nums text-ink">{entry.values[hover] === null ? "—" : format(model.raw[index][hover])}</span>
                  </p>
                ))}
                {stacked && series.length > 1 && (
                  <p className="mt-1 flex justify-between gap-4 border-t border-white/[0.08] pt-1.5 font-medium">
                    <span className="text-muted">Gesamt</span>
                    <span className="tabular-nums">{format(hoverTotal)}</span>
                  </p>
                )}
              </div>
            </>
          )}
        </div>

        <div className="relative h-7 text-[11px] tabular-nums text-muted">
          {xTicks.map((index) => (
            <span key={index} className="absolute top-2 -translate-x-1/2" style={{ left: `${(index / (count - 1)) * 100}%` }}>
              {long ? new Date(times[index]).toLocaleDateString("de-DE", { day: "2-digit", month: "2-digit" }) + " " + clock(times[index]) : clock(times[index])}
            </span>
          ))}
        </div>
      </div>
    </div>
  );
}
