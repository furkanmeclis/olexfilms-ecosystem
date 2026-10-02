"use client";

import { useQuery } from "@tanstack/react-query";
import { Boxes } from "lucide-react";
import Link from "next/link";
import { useMemo } from "react";

import { ErrorState } from "@/components/common/error-state";
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
import { useFeature } from "@/features/modules/hooks/use-features";
import { summarizeProducts } from "@/features/stock/lib/stock";
import {
  stockKeys,
  stockService,
} from "@/features/stock/services/stock.service";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** Rows of the product projection read for the totals and the top list. */
export const WIDGET_LIMIT = 200;
const TOP_ROWS = 5;

/**
 * TEC-224: stock summary of the tenant home (dealer and distributor):
 * products in stock, pieces and remaining meters, with the top products by
 * quantity. Rendered only with stock.read and the stock module on.
 */
export function StockSummaryWidget({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const feature = useFeature(slug, "stock");
  const orgUuid = org?.uuid ?? "";
  const visible =
    can(Permission.StockRead) &&
    feature.enabled &&
    (org?.type === "dealer" || org?.type === "distributor");

  const params = useMemo(
    () => ({ status: "in_stock" as const, limit: WIDGET_LIMIT, offset: 0 }),
    [],
  );
  const query = useQuery({
    queryKey: stockKeys.products(orgUuid, params),
    queryFn: () => stockService.listProducts(orgUuid, params),
    enabled: visible && orgUuid !== "",
    staleTime: 60_000,
  });

  const items = useMemo(() => query.data?.items ?? [], [query.data]);
  const summary = useMemo(() => summarizeProducts(items), [items]);
  const top = useMemo(
    () =>
      [...items]
        .sort(
          (a, b) =>
            b.quantity - a.quantity || Number(b.meters) - Number(a.meters),
        )
        .slice(0, TOP_ROWS),
    [items],
  );

  if (!visible) return null;

  return (
    <Card data-testid="stock-summary-widget">
      <CardHeader className="flex flex-row items-start justify-between gap-2">
        <div className="space-y-1">
          <CardTitle className="flex items-center gap-2">
            <Boxes className="size-4" />
            {t("stock.widget.title")}
          </CardTitle>
          <CardDescription>{t("stock.widget.description")}</CardDescription>
        </div>
        <Button asChild variant="outline" size="sm">
          <Link
            href={routes.tenant.stock.root(slug)}
            data-testid="stock-widget-link"
          >
            {t("stock.widget.open")}
          </Link>
        </Button>
      </CardHeader>
      <CardContent className="space-y-4">
        {query.isError ? (
          <ErrorState
            title={t("common.error_generic")}
            onRetry={() => void query.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : query.isPending ? (
          <div className="grid gap-3 sm:grid-cols-3">
            <Skeleton className="h-16" />
            <Skeleton className="h-16" />
            <Skeleton className="h-16" />
          </div>
        ) : (
          <>
            <dl className="grid gap-3 sm:grid-cols-3">
              <Stat
                label={t("stock.widget.products")}
                value={format.number(summary.products)}
                testId="stock-widget-products"
              />
              <Stat
                label={t("stock.widget.quantity")}
                value={format.number(summary.quantity)}
                testId="stock-widget-quantity"
              />
              <Stat
                label={t("stock.widget.meters")}
                value={format.number(summary.meters)}
                testId="stock-widget-meters"
              />
            </dl>
            {items.length === 0 ? (
              <p
                className="text-muted-foreground text-sm"
                data-testid="stock-widget-empty"
              >
                {t("stock.widget.empty")}
              </p>
            ) : (
              <ul className="divide-y text-sm" data-testid="stock-widget-top">
                {top.map((row) => (
                  <li
                    key={row.product.uuid}
                    className="flex items-center justify-between gap-3 py-1.5"
                  >
                    <div className="min-w-0">
                      <div className="truncate font-medium">
                        {row.product.name}
                      </div>
                      <div className="text-muted-foreground font-mono text-xs">
                        {row.product.sku}
                      </div>
                    </div>
                    <div className="text-end whitespace-nowrap tabular-nums">
                      {row.product.unit_type === "roll_meter"
                        ? t("stock.widget.meters_value", {
                            meters: row.meters,
                          })
                        : t("stock.widget.quantity_value", {
                            quantity: format.number(row.quantity),
                          })}
                    </div>
                  </li>
                ))}
              </ul>
            )}
            {(query.data?.total ?? 0) > WIDGET_LIMIT ? (
              <p className="text-muted-foreground text-xs">
                {t("stock.widget.truncated", { limit: WIDGET_LIMIT })}
              </p>
            ) : null}
          </>
        )}
      </CardContent>
    </Card>
  );
}

function Stat({
  label,
  value,
  testId,
}: {
  label: string;
  value: string;
  testId: string;
}) {
  return (
    <div className="bg-muted/40 rounded-lg border p-3">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-xl font-semibold tabular-nums" data-testid={testId}>
        {value}
      </dd>
    </div>
  );
}
