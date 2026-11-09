"use client";

import { useQuery } from "@tanstack/react-query";
import { Boxes, ChartLine, Clock, Timer, Warehouse } from "lucide-react";
import Link from "next/link";
import { useState, type ReactNode } from "react";

import { AppChart } from "@/components/charts";
import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { PageHeader } from "@/components/layout";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { orderStatusTone } from "@/features/orders/lib/form";
import {
  orderKeys,
  ordersService,
  type Order,
  type OrderListQuery,
} from "@/features/orders/services/orders.service";
import { performanceService } from "@/features/performance/services/performance.service";
import {
  stockKeys,
  stockService,
} from "@/features/stock/services/stock.service";
import { WarehouseKpis } from "@/features/warehouse/components/warehouse-kpis";
import { PREPARE_STATUSES } from "@/features/warehouse/lib/kpis";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** Rows of the pending and recent order lists. */
export const DASHBOARD_ORDER_ROWS = 8;
export const ORDER_CHART_PERIODS = ["7d", "30d", "90d", "12m"] as const;
export type OrderChartPeriod = (typeof ORDER_CHART_PERIODS)[number];

/**
 * TEC-1066: the warehouse home (legacy olexfilms-warehouse "Kontrol
 * paneli": KPIs, orders and stock packages). Live counters, in-stock totals
 * per warehouse, the preparation queue, the latest orders and the order
 * count over a chosen period. Nothing here shows a price.
 */
export function WarehouseDashboard({ slug }: { slug: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const canOrders = can(Permission.OrdersRead);
  return (
    <div className="flex flex-col gap-6" data-testid="warehouse-dashboard">
      <PageHeader
        title={t("warehouse.home.title")}
        description={t("warehouse.home.description")}
        icon={<Warehouse className="size-6" />}
      />
      <WarehouseKpis slug={slug} can={can} orgType={org?.type} />
      <div className="grid gap-6 xl:grid-cols-2">
        {can(Permission.StockRead) ? <WarehouseStockCard slug={slug} /> : null}
        {canOrders ? <OrderCountChart /> : null}
        {canOrders ? (
          <OrderListCard
            slug={slug}
            testId="warehouse-pending-orders"
            icon={<Timer className="size-4" />}
            title={t("warehouse.home.pending_title")}
            description={t("warehouse.home.pending_description")}
            empty={t("warehouse.home.pending_empty")}
            query={{
              side: "seller",
              status: PREPARE_STATUSES,
              sort: "created_at",
              limit: DASHBOARD_ORDER_ROWS,
              offset: 0,
            }}
            href={`${routes.tenant.orders.list(slug)}?status=${PREPARE_STATUSES}`}
          />
        ) : null}
        {canOrders ? (
          <OrderListCard
            slug={slug}
            testId="warehouse-recent-orders"
            icon={<Clock className="size-4" />}
            title={t("warehouse.home.recent_title")}
            description={t("warehouse.home.recent_description")}
            empty={t("warehouse.home.recent_empty")}
            query={{
              side: "seller",
              sort: "-created_at",
              limit: DASHBOARD_ORDER_ROWS,
              offset: 0,
            }}
            href={routes.tenant.orders.list(slug)}
          />
        ) : null}
      </div>
    </div>
  );
}

function WarehouseStockCard({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const query = useQuery({
    queryKey: stockKeys.warehouseSummary,
    queryFn: () => stockService.warehouseSummary(),
    staleTime: 60_000,
  });
  const rows = query.data?.items ?? [];
  return (
    <Card data-testid="warehouse-stock-card">
      <CardHeader className="flex flex-row items-start justify-between gap-2">
        <div className="space-y-1">
          <CardTitle className="flex items-center gap-2">
            <Boxes className="size-4" />
            {t("warehouse.home.stock_title")}
          </CardTitle>
          <CardDescription>
            {t("warehouse.home.stock_description")}
          </CardDescription>
        </div>
        <Button asChild variant="outline" size="sm">
          <Link href={routes.tenant.stock.root(slug)}>
            {t("warehouse.home.view_all")}
          </Link>
        </Button>
      </CardHeader>
      <CardContent>
        {query.isError ? (
          <ErrorState
            title={t("common.error_generic")}
            onRetry={() => void query.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : query.isPending ? (
          <Skeleton className="h-24" />
        ) : rows.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("warehouse.home.stock_empty")}
          </p>
        ) : (
          <ul className="divide-y" data-testid="warehouse-stock-rows">
            {rows.map((row) => (
              <li
                key={row.warehouse?.uuid ?? "unplaced"}
                className="space-y-2 py-3 first:pt-0 last:pb-0"
                data-testid="warehouse-stock-row"
              >
                <div className="flex items-center gap-2 font-medium">
                  <span className="truncate">
                    {row.warehouse?.name ?? t("warehouse.home.stock_unplaced")}
                  </span>
                  {row.warehouse?.is_primary ? (
                    <Badge variant="secondary">
                      {t("warehouse.home.stock_primary")}
                    </Badge>
                  ) : null}
                </div>
                <dl className="grid grid-cols-2 gap-2 sm:grid-cols-4">
                  <Stat
                    label={t("warehouse.home.stock_products")}
                    value={format.number(row.products)}
                  />
                  <Stat
                    label={t("warehouse.home.stock_pieces")}
                    value={format.number(row.pieces)}
                  />
                  <Stat
                    label={t("warehouse.home.stock_rolls")}
                    value={format.number(row.rolls)}
                  />
                  <Stat
                    label={t("warehouse.home.stock_meters")}
                    value={format.number(Number(row.meters))}
                  />
                </dl>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="bg-muted/40 rounded-md border px-2.5 py-1.5">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="font-semibold tabular-nums">{value}</dd>
    </div>
  );
}

function OrderListCard({
  slug,
  testId,
  icon,
  title,
  description,
  empty,
  query: params,
  href,
}: {
  slug: string;
  testId: string;
  icon: ReactNode;
  title: string;
  description: string;
  empty: string;
  query: OrderListQuery;
  href: string;
}) {
  const { t, format } = useLocale();
  const query = useQuery({
    queryKey: orderKeys.list(params),
    queryFn: () => ordersService.list(params),
    staleTime: 30_000,
    refetchInterval: 60_000,
  });
  const items: Order[] = query.data?.items ?? [];
  return (
    <Card data-testid={testId}>
      <CardHeader className="flex flex-row items-start justify-between gap-2">
        <div className="space-y-1">
          <CardTitle className="flex items-center gap-2">
            {icon}
            {title}
            {typeof query.data?.total === "number" ? (
              <Badge variant="outline" className="tabular-nums">
                {format.number(query.data.total)}
              </Badge>
            ) : null}
          </CardTitle>
          <CardDescription>{description}</CardDescription>
        </div>
        <Button asChild variant="outline" size="sm">
          <Link href={href}>{t("warehouse.home.view_all")}</Link>
        </Button>
      </CardHeader>
      <CardContent>
        {query.isError ? (
          <ErrorState
            title={t("common.error_generic")}
            onRetry={() => void query.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : query.isPending ? (
          <Skeleton className="h-32" />
        ) : items.length === 0 ? (
          <p className="text-muted-foreground text-sm">{empty}</p>
        ) : (
          <ul className="divide-y text-sm">
            {items.map((o) => (
              <li key={o.uuid}>
                <Link
                  href={routes.tenant.orders.detail(slug, o.uuid)}
                  className="hover:bg-accent/50 -mx-2 flex items-center justify-between gap-3 rounded-md px-2 py-2"
                  data-testid="warehouse-order-row"
                >
                  <div className="min-w-0">
                    <div className="truncate font-medium">{o.buyer.name}</div>
                    <div className="text-muted-foreground flex gap-2 text-xs">
                      <span className="font-mono" dir="ltr">
                        {o.order_no}
                      </span>
                      <span>{format.dateTime(o.created_at)}</span>
                    </div>
                  </div>
                  <StatusChip
                    label={o.status_label}
                    tone={orderStatusTone(o.status)}
                  />
                </Link>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

/** Order counts per day/week/month (GET /v1/reports/orders.trend; counts only). */
function OrderCountChart() {
  const { t, format } = useLocale();
  const [period, setPeriod] = useState<OrderChartPeriod>("30d");
  const query = useQuery({
    queryKey: ["warehouse", "home", "orders-trend", period] as const,
    queryFn: () => performanceService.report("orders.trend", { period }),
    staleTime: 60_000,
  });
  const series = query.data?.series ?? [];
  const first = series[0];
  const rows =
    first?.points.map((p, i) => {
      const row: Record<string, string | number | null> = { label: p.label };
      for (const s of series) row[s.key] = s.points[i]?.value ?? null;
      return row;
    }) ?? [];
  return (
    <Card data-testid="warehouse-order-chart">
      <CardHeader className="space-y-3">
        <div className="space-y-1">
          <CardTitle className="flex items-center gap-2">
            <ChartLine className="size-4" />
            {t("warehouse.home.chart_title")}
          </CardTitle>
          <CardDescription>
            {t("warehouse.home.chart_description")}
          </CardDescription>
        </div>
        <div className="flex flex-wrap gap-1" role="group">
          {ORDER_CHART_PERIODS.map((p) => (
            <Button
              key={p}
              type="button"
              size="sm"
              variant={p === period ? "default" : "outline"}
              className={cn("h-7 px-2.5 text-xs")}
              aria-pressed={p === period}
              data-testid="warehouse-chart-period"
              data-period={p}
              onClick={() => setPeriod(p)}
            >
              {t(`warehouse.home.period.${p}`)}
            </Button>
          ))}
        </div>
      </CardHeader>
      <CardContent>
        {query.isError ? (
          <ErrorState
            title={t("common.error_generic")}
            onRetry={() => void query.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : query.isPending ? (
          <Skeleton className="h-[220px]" />
        ) : (
          <AppChart
            type="bar"
            data={rows}
            categoryKey="label"
            config={Object.fromEntries(
              series.map((s) => [s.key, { label: s.label }]),
            )}
            series={series.map((s) => s.key)}
            valueFormatter={(v) => format.number(v)}
            height={220}
            emptyTitle={t("warehouse.home.chart_empty")}
          />
        )}
      </CardContent>
    </Card>
  );
}
