"use client";

import { useQueries } from "@tanstack/react-query";
import { RefreshCw } from "lucide-react";
import Link from "next/link";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { ordersService } from "@/features/orders/services/orders.service";
import { visibleKpis, type WarehouseKpi } from "@/features/warehouse/lib/kpis";
import { warehouseService } from "@/features/warehouse/services/warehouse.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/** One `limit=1` list call; the count is the page total. */
async function countOf(kpi: WarehouseKpi): Promise<number> {
  const query = { ...kpi.params, limit: 1, offset: 0 };
  switch (kpi.source) {
    case "orders": {
      const page = await ordersService.list({
        ...query,
        side: kpi.params.side === "buyer" ? "buyer" : "seller",
      });
      return page.total;
    }
    case "entries":
      return (await warehouseService.listEntries(query)).total;
    case "transfers":
      return (await warehouseService.listTransfers(query)).total;
    case "counts":
      return (await warehouseService.listCounts(query)).total;
  }
}

/**
 * Live counters of the warehouse hub (TEC-1032): the preparation queue,
 * ready / shipped and open orders, draft entries, open transfers and open
 * counts of the organization. Each tile links to the filtered list.
 */
export function WarehouseKpis({
  slug,
  can,
  orgType,
}: {
  slug: string;
  can: (permission: string) => boolean;
  orgType: string | null | undefined;
}) {
  const { t, format } = useLocale();
  const kpis = visibleKpis(can, orgType);
  const results = useQueries({
    queries: kpis.map((kpi) => ({
      queryKey: ["warehouse", "kpi", kpi.id] as const,
      queryFn: () => countOf(kpi),
      staleTime: 30_000,
      refetchInterval: 60_000,
    })),
  });
  if (kpis.length === 0) return null;
  const refreshing = results.some((r) => r.isFetching);

  return (
    <section className="space-y-2" data-testid="warehouse-kpis">
      <div className="flex items-center justify-between gap-2">
        <h2 className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
          {t("warehouse.kpis.title")}
        </h2>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => results.forEach((r) => void r.refetch())}
          disabled={refreshing}
          aria-label={t("entity.refresh")}
          data-testid="warehouse-kpis-refresh"
        >
          <RefreshCw className={cn("size-4", refreshing && "animate-spin")} />
        </Button>
      </div>
      <div className="grid gap-3 sm:grid-cols-3 xl:grid-cols-6">
        {kpis.map((kpi, i) => {
          const r = results[i];
          const Icon = kpi.icon;
          const value = r?.data;
          const hot = kpi.attention && typeof value === "number" && value > 0;
          return (
            <Link
              key={kpi.id}
              href={kpi.href(slug)}
              data-testid="warehouse-kpi"
              data-kpi={kpi.id}
              data-count={typeof value === "number" ? value : undefined}
              className={cn(
                "bg-card hover:bg-accent/50 rounded-lg border p-3 transition-colors",
                hot && "border-primary/50",
              )}
            >
              <div className="text-muted-foreground flex items-center gap-1.5 text-xs">
                <Icon className="size-3.5 shrink-0" />
                <span className="truncate">{t(kpi.key)}</span>
              </div>
              {r?.isError ? (
                <div
                  className="text-destructive mt-1 text-sm"
                  data-testid="warehouse-kpi-error"
                >
                  {t("warehouse.kpis.error")}
                </div>
              ) : typeof value === "number" ? (
                <div
                  className={cn(
                    "mt-1 text-2xl font-semibold tabular-nums",
                    hot && "text-primary",
                  )}
                >
                  {format.number(value)}
                </div>
              ) : (
                <Skeleton className="mt-1.5 h-7 w-12" />
              )}
            </Link>
          );
        })}
      </div>
    </section>
  );
}
