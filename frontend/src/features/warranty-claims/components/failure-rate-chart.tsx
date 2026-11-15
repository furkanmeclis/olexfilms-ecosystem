"use client";

import { useMemo } from "react";

import { BarChart, type ChartSeriesDef } from "@/components/analytics";
import { toFailureRateBars } from "@/features/warranty-claims/lib/claim-reports";
import type { FailureRateRow } from "@/features/warranty-claims/services/claim-reports.service";
import { useLocale } from "@/providers/locale-provider";

/**
 * Highest claim / approved failure rates (percent points) as grouped bars
 * of the analytics kit (TEC-1212: kit palette, tooltip, legend, table view
 * and RTL handling instead of a hand-made recharts chart).
 */
export function FailureRateChart({
  rows,
}: {
  rows: readonly FailureRateRow[];
}) {
  const { t } = useLocale();
  const bars = useMemo(() => toFailureRateBars(rows), [rows]);
  const series = useMemo<ChartSeriesDef[]>(
    () => [
      {
        key: "claimPercent",
        label: t("warranty.claim_reports.columns.claim_rate"),
      },
      {
        key: "approvedPercent",
        label: t("warranty.claim_reports.columns.approved_rate"),
      },
    ],
    [t],
  );
  if (bars.length === 0) return null;
  return (
    <div data-testid="claim-report-chart">
      <BarChart
        bare
        data={bars}
        series={series}
        format={{ kind: "percent", maximumFractionDigits: 1 }}
        height={300}
      />
    </div>
  );
}
