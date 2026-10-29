"use client";

import { useMemo } from "react";
import { cx } from "./primitives";

// Alles in diesem Stil: Pixelgrafiken aus Rechtecken, ohne Weichzeichnen, wie Texturen im Spiel.

function hash(text: string): number {
  let h = 2166136261;
  for (let i = 0; i < text.length; i++) h = Math.imul(h ^ text.charCodeAt(i), 16777619);
  return h >>> 0;
}

/** Zufallszahlen aus einem Startwert, damit dieselbe Eingabe immer dasselbe Bild ergibt. */
function random(seed: number) {
  let state = seed || 1;
  return () => {
    state ^= state << 13;
    state ^= state >>> 17;
    state ^= state << 5;
    return (state >>> 0) / 4294967296;
  };
}

// ---------------------------------------------------------------- Logo

const STONE = ["#9b9b9b", "#8c8c8c", "#7d7d7d"];

/** Ein Lodestone-Block mit Kompassnadel: 16 × 16 Pixel Stein, in der Mitte der Kompass. */
function logoPixels(): { x: number; y: number; color: string }[] {
  const next = random(0x10de57);
  const pixels: { x: number; y: number; color: string }[] = [];
  for (let y = 0; y < 16; y++) {
    for (let x = 0; x < 16; x++) {
      let color = STONE[Math.floor(next() * STONE.length)];
      if (x === 0 || y === 0) color = "#b4b4b4"; // Licht von oben links
      if (x === 15 || y === 15) color = "#4d4d4d"; // Schatten unten rechts
      const dx = x - 7.5;
      const dy = y - 7.5;
      const distance = Math.hypot(dx, dy);
      if (distance < 5.6 && distance >= 4.4) color = "#2b2b2b"; // dunkler Ring
      else if (distance < 4.4) color = "#cfcfcf"; // Kompassfläche
      // Nadel: rote Spitze oben rechts, weiße unten links
      if (distance < 4.4 && Math.abs(dx + dy) < 0.9) color = dx - dy > 0 ? "#d63b3b" : "#f4f4f4";
      if (Math.abs(dx) < 0.6 && Math.abs(dy) < 0.6) color = "#2b2b2b"; // Mittelpunkt
      pixels.push({ x, y, color });
    }
  }
  return pixels;
}

export function LodestoneLogo({ size = 28, className }: { size?: number; className?: string }) {
  const pixels = useMemo(logoPixels, []);
  return (
    <svg viewBox="0 0 16 16" width={size} height={size} shapeRendering="crispEdges" className={cx("shrink-0 rounded-[3px]", className)} role="img" aria-label="Lodestone">
      {pixels.map((pixel) => (
        <rect key={`${pixel.x}-${pixel.y}`} x={pixel.x} y={pixel.y} width={1} height={1} fill={pixel.color} />
      ))}
    </svg>
  );
}

/** Schriftzug mit dem Schlagschatten, den Minecraft unter Texten zeichnet. */
export function Wordmark({ className }: { className?: string }) {
  return (
    <span className={cx("font-bold uppercase tracking-[0.14em]", className)} style={{ textShadow: "2px 2px 0 rgba(0,0,0,0.55)" }}>
      Lodestone
    </span>
  );
}

// ---------------------------------------------------------------- Avatar

const SKIN = ["#f1c9a5", "#e2b08a", "#c68b61", "#9a643d", "#fbdcc0"];
const HAIR = ["#3a2a1a", "#6b4423", "#c9a24a", "#1d1d1d", "#8a3b1c", "#d8d8d8", "#3a3f9e"];
const EYES = ["#3b5bdb", "#2f9e44", "#5c3a21", "#1c7ed6", "#6741d9"];
const SHIRT = ["#00a8a8", "#b02e2e", "#2f9e44", "#7048e8", "#e8890c", "#3b5bdb"];

/** 8 × 8 Pixel, wie der Kopf eines Minecraft-Skins; Haut, Haare, Augen und Hemd ergeben sich aus dem Namen. */
export function PixelAvatar({ name, size = 28, className }: { name: string; size?: number; className?: string }) {
  const cells = useMemo(() => {
    const seed = hash(name.toLowerCase());
    const next = random(seed);
    const pick = <T,>(list: T[]) => list[Math.floor(next() * list.length)];
    const skin = pick(SKIN);
    const hair = pick(HAIR);
    const eyes = pick(EYES);
    const shirt = pick(SHIRT);
    const fringe = next() < 0.5 ? 1 : 2; // Haarlänge an der Stirn
    const sideburns = next() < 0.6;
    const mouth = shade(skin, -0.28);
    const result: { x: number; y: number; color: string }[] = [];
    for (let y = 0; y < 8; y++) {
      for (let x = 0; x < 8; x++) {
        let color = skin;
        if (y < fringe + 1) color = hair; // Haar oben
        else if (y < 5 && (x === 0 || x === 7) && sideburns) color = hair;
        if (y === 4 && (x === 1 || x === 2 || x === 5 || x === 6)) {
          color = x === 2 || x === 5 ? eyes : "#ffffff";
        }
        if (y === 6 && x >= 2 && x <= 5) color = mouth;
        if (y === 7) color = shirt; // Kragen
        result.push({ x, y, color });
      }
    }
    return result;
  }, [name]);
  return (
    <svg viewBox="0 0 8 8" width={size} height={size} shapeRendering="crispEdges" className={cx("shrink-0 rounded-[3px] ring-1 ring-black/40", className)} role="img" aria-label={name}>
      {cells.map((cell) => (
        <rect key={`${cell.x}-${cell.y}`} x={cell.x} y={cell.y} width={1} height={1} fill={cell.color} />
      ))}
    </svg>
  );
}

function shade(hex: string, amount: number): string {
  const value = parseInt(hex.slice(1), 16);
  const channel = (shift: number) => Math.max(0, Math.min(255, Math.round(((value >> shift) & 255) * (1 + amount))));
  return `#${[16, 8, 0].map((shift) => channel(shift).toString(16).padStart(2, "0")).join("")}`;
}
