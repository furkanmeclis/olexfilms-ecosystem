import type { ReactNode } from "react";

import { DeltaBadge } from "@/components/analytics/delta-badge";
import { Sparkline } from "@/components/analytics/sparkline";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/common/card";

type StatsCardProps = {
  title: string;
  value: ReactNode;
  hint?: string;
  /**
   * TEC-1211: optional trend context. For new screens prefer
   * `KpiTile` (@/components/analytics), which formats the value too.
   */
  deltaPct?: number | null;
  /** Which direction of deltaPct is good (default up). */
  good?: "up" | "down";
  spark?: readonly number[];
};

export function StatsCard({
  title,
  value,
  hint,
  deltaPct,
  good,
  spark,
}: StatsCardProps) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-muted-foreground text-sm font-medium">
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex items-end justify-between gap-3">
          <div className="min-w-0">
            <div className="text-2xl font-semibold tracking-tight">{value}</div>
            {deltaPct !== undefined ? (
              <DeltaBadge deltaPct={deltaPct} good={good} className="mt-1" />
            ) : null}
            {hint ? (
              <p className="text-muted-foreground mt-1 text-xs">{hint}</p>
            ) : null}
          </div>
          {spark && spark.length > 1 ? (
            <Sparkline values={spark} className="shrink-0" />
          ) : null}
        </div>
      </CardContent>
    </Card>
  );
}
