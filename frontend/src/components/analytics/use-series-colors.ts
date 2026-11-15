"use client";

import { useState } from "react";

import { assignColorSlots, categoricalColor } from "./palette";
import type { ChartSeriesDef } from "./types";

function sameSlots(a: Record<string, number>, b: Record<string, number>) {
  const ka = Object.keys(a);
  if (ka.length !== Object.keys(b).length) return false;
  return ka.every((k) => a[k] === b[k]);
}

/**
 * Colour follows identity: a series keeps the slot it first got for the
 * lifetime of the chart, even when filters hide other series. Explicit
 * `colorSlot` wins.
 */
export function useSeriesColors(
  series: readonly Pick<ChartSeriesDef, "key" | "colorSlot">[],
): Record<string, string> {
  const fixed: Record<string, number> = {};
  for (const s of series) {
    if (s.colorSlot !== undefined) fixed[s.key] = s.colorSlot;
  }
  const keys = series.map((s) => s.key);
  const [slots, setSlots] = useState<Record<string, number>>(() =>
    assignColorSlots(keys, fixed),
  );
  const next = assignColorSlots(keys, { ...slots, ...fixed });
  // Derived state: remember newly seen keys (React re-renders at once).
  if (!sameSlots(next, slots)) setSlots(next);
  const out: Record<string, string> = {};
  for (const s of series) out[s.key] = categoricalColor(next[s.key] ?? -1);
  return out;
}
