"use client";

import { Warehouse } from "lucide-react";

import { OverviewShell } from "@/features/overview/components/overview-shell";
import {
  ReportKpiStrip,
  ReportTablePanel,
  ReportTrendPanel,
} from "@/features/reports/components/report-panels";
import { WarehouseDashboardBody } from "@/features/warehouse";
import { useLocale } from "@/providers/locale-provider";

/**
 * TEC-1212: warehouse overview. orders.fulfillment (shipped / received,
 * preparation time, open and late orders, per warehouse) and
 * stock.movement (in / out) under one period, then the warehouse home
 * counters and order queues.
 */
export function WarehouseOverview({ slug }: { slug: string }) {
  const { t } = useLocale();
  return (
    <OverviewShell
      testId="warehouse-overview"
      title={t("warehouse.home.title")}
      description={t("warehouse.home.description")}
      icon={<Warehouse className="size-6" />}
    >
      {(query) => (
        <>
          <ReportKpiStrip
            slug={slug}
            reportKey="orders.fulfillment"
            query={query}
          />
          <div className="grid gap-6 xl:grid-cols-2">
            <ReportTrendPanel
              slug={slug}
              reportKey="orders.fulfillment"
              query={query}
              title={t("dashboard.role.fulfillment_trend")}
            />
            <ReportTrendPanel
              slug={slug}
              reportKey="stock.movement"
              query={query}
              title={t("dashboard.role.stock_movement")}
            />
          </div>
          <ReportTablePanel
            slug={slug}
            reportKey="orders.fulfillment"
            query={query}
            height={280}
            title={t("dashboard.role.fulfillment_by_warehouse")}
          />
          <WarehouseDashboardBody slug={slug} showOrderChart={false} />
        </>
      )}
    </OverviewShell>
  );
}
