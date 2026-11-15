"use client";

import { ArrowDownRight, ArrowUpRight, Minus } from "lucide-react";

import { cn } from "@/lib/utils";
import { useFormatter, useLocale } from "@/providers/locale-provider";

import { deltaTone, formatDeltaPct, isNumber, type DeltaTone } from "./format";

const TONE_COLOR: Record<DeltaTone, string> = {
  good: "var(--viz-delta-good)",
  bad: "var(--viz-delta-bad)",
  neutral: "var(--viz-muted)",
};

/** Compact signed delta for tooltips / cells (arrow + text, colour = tone). */
export function DeltaText({
  deltaPct,
  good = "up",
  className,
}: {
  deltaPct: number | null | undefined;
  good?: "up" | "down";
  className?: string;
}) {
  const fmt = useFormatter();
  if (!isNumber(deltaPct)) return <span className={className} />;
  const tone = deltaTone(deltaPct, good);
  return (
    <span
      className={cn("tabular-nums", className)}
      style={{ color: TONE_COLOR[tone] }}
    >
      {formatDeltaPct(deltaPct, fmt)}
    </span>
  );
}

/**
 * Delta badge: icon + signed percent + screen-reader reading (better /
 * worse). Meaning never rests on colour alone.
 */
export function DeltaBadge({
  deltaPct,
  good = "up",
  className,
}: {
  deltaPct: number | null | undefined;
  good?: "up" | "down";
  className?: string;
}) {
  const fmt = useFormatter();
  const { t } = useLocale();
  if (!isNumber(deltaPct)) {
    return (
      <span
        className={cn("text-muted-foreground text-xs", className)}
        data-slot="delta-badge"
        data-tone="none"
      >
        {t("chart.kpi.no_previous")}
      </span>
    );
  }
  const tone = deltaTone(deltaPct, good);
  const Icon =
    deltaPct > 0 ? ArrowUpRight : deltaPct < 0 ? ArrowDownRight : Minus;
  const reading =
    tone === "good"
      ? t("chart.kpi.better")
      : tone === "bad"
        ? t("chart.kpi.worse")
        : t("chart.kpi.no_change");
  return (
    <span
      className={cn(
        "inline-flex items-center gap-0.5 rounded-md px-1.5 py-0.5 text-xs font-medium tabular-nums",
        className,
      )}
      style={{
        color: TONE_COLOR[tone],
        background: `color-mix(in oklab, ${TONE_COLOR[tone]} 12%, transparent)`,
      }}
      data-slot="delta-badge"
      data-tone={tone}
    >
      <Icon className="size-3.5" aria-hidden />
      {formatDeltaPct(deltaPct, fmt)}
      <span className="sr-only">
        {" "}
        ({reading}, {t("chart.kpi.vs_previous")})
      </span>
    </span>
  );
}
