"use client";

import { AlertTriangle } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Money } from "@/features/accounting/components/shared";
import { isOverThreshold, toNumber } from "@/features/pricing/lib/recommended";
import type { RecommendedPriceRef } from "@/features/pricing/services/recommended.service";
import { useLocale } from "@/providers/locale-provider";

/**
 * Deviation from the recommended price in percent (TEC-507): neutral within
 * the threshold, a warning badge at or above it (either way).
 */
export function DeviationBadge({
  value,
  threshold,
}: {
  value: string | number | null | undefined;
  threshold: number;
}) {
  const { t, format } = useLocale();
  const dev = toNumber(value);
  if (dev === null) return null;
  const over = isOverThreshold(dev, threshold);
  const text = `${dev > 0 ? "+" : ""}${format.percent(dev / 100, 2)}`;
  return (
    <Badge
      variant={over ? "warning" : "outline"}
      data-testid="deviation-badge"
      data-over-threshold={over ? "true" : "false"}
      title={
        over
          ? t("catalog.recommended.deviation_over", { threshold })
          : t("catalog.recommended.deviation_hint")
      }
    >
      {over ? <AlertTriangle className="size-3" aria-hidden /> : null}
      <span dir="ltr" className="tabular-nums">
        {text}
      </span>
    </Badge>
  );
}

/**
 * "Recommended" cell of the distributor / dealer price screens: the price
 * in force (country, else currency-wide) and the own price's deviation.
 */
export function RecommendedPriceCell({
  recommended,
  deviationPct,
  threshold,
}: {
  recommended?: RecommendedPriceRef | null;
  deviationPct?: string | null;
  threshold: number;
}) {
  const { t } = useLocale();
  if (!recommended) {
    return <span className="text-muted-foreground">—</span>;
  }
  return (
    <div
      className="flex flex-wrap items-center justify-end gap-2"
      data-testid="recommended-cell"
    >
      <Money amount={recommended.price} currency={recommended.currency} />
      {recommended.scope === "currency" ? (
        <span
          className="text-muted-foreground text-xs"
          title={t("catalog.recommended.scope_currency_hint")}
        >
          {t("catalog.recommended.scope_currency")}
        </span>
      ) : null}
      <DeviationBadge value={deviationPct} threshold={threshold} />
    </div>
  );
}
