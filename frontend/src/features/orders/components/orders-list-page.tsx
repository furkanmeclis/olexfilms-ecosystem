"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { ChevronLeft, ChevronRight, Plus, ShoppingCart } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { DatePicker } from "@/components/ui/date-picker";
import { Label } from "@/components/ui/label";
import { routes } from "@/config/routes";
import { resolveOrderListAccess } from "@/features/orders/lib/access";
import { amountNumber, orderStatusTone } from "@/features/orders/lib/form";
import {
  ALL_STATUSES,
  EMPTY_ORDER_FILTERS,
  ORDER_FILTER_STATUSES,
  buildOrderListQuery,
  hasActiveFilters,
  invalidDateRange,
  pageCount,
  type OrderListFilters,
} from "@/features/orders/lib/list-filters";
import {
  orderKeys,
  ordersService,
  type OrderSide,
} from "@/features/orders/services/orders.service";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const ORDER_PAGE_SIZE = 20;

/**
 * Tenant > Orders (TEC-170): incoming orders (the organization sells) and
 * outgoing orders (it buys) with status and created date filters, paged.
 */
export function OrdersListPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const access = resolveOrderListAccess(can, org?.type);
  const [pickedSide, setSide] = useState<OrderSide | null>(null);
  const side: OrderSide =
    pickedSide && access.sides.includes(pickedSide)
      ? pickedSide
      : access.sides[0];
  const [filters, setFilters] = useState<OrderListFilters>(EMPTY_ORDER_FILTERS);
  const [page, setPage] = useState(0);

  const query = buildOrderListQuery(side, filters, {
    limit: ORDER_PAGE_SIZE,
    offset: page * ORDER_PAGE_SIZE,
  });

  const list = useQuery({
    queryKey: orderKeys.list(query),
    queryFn: () => ordersService.list(query),
    enabled: access.canRead,
    placeholderData: keepPreviousData,
  });

  const change = (patch: Partial<OrderListFilters>) => {
    setFilters((f) => ({ ...f, ...patch }));
    setPage(0);
  };

  const title = t("orders.list.title");
  const header = (
    <PageHeader
      title={title}
      icon={<ShoppingCart className="size-6" />}
      description={t("orders.list.description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: title },
      ]}
      actions={
        access.canCreate ? (
          <Button asChild>
            <Link
              href={routes.tenant.orders.create(slug)}
              data-testid="new-order"
            >
              <Plus className="size-4" />
              {t("orders.nav_new")}
            </Link>
          </Button>
        ) : null
      }
    />
  );

  if (!access.canRead) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("orders.list.forbidden")}
        />
      </div>
    );
  }

  const total = list.data?.total ?? 0;
  const pages = pageCount(total, ORDER_PAGE_SIZE);
  const rows = list.data?.items ?? [];
  const badRange = invalidDateRange(filters.from, filters.to);
  const filtered = hasActiveFilters(filters);

  return (
    <div className="space-y-6">
      {header}
      {access.sides.length > 1 ? (
        <div
          className="flex gap-2"
          role="tablist"
          aria-label={t("orders.list.title")}
        >
          {access.sides.map((s) => (
            <Button
              key={s}
              type="button"
              role="tab"
              aria-selected={side === s}
              variant={side === s ? "default" : "outline"}
              data-testid={`side-${s}`}
              onClick={() => {
                setSide(s);
                setPage(0);
              }}
            >
              {s === "seller"
                ? t("orders.list.incoming")
                : t("orders.list.outgoing")}
            </Button>
          ))}
        </div>
      ) : null}
      <Card>
        <CardContent className="space-y-4 pt-6" data-testid="order-filters">
          <div
            className="flex flex-wrap gap-2"
            role="group"
            aria-label={t("orders.list.status")}
          >
            {[ALL_STATUSES, ...ORDER_FILTER_STATUSES].map((status) => {
              const active = filters.status === status;
              return (
                <Button
                  key={status}
                  type="button"
                  size="sm"
                  variant={active ? "default" : "outline"}
                  aria-pressed={active}
                  data-status={status}
                  onClick={() =>
                    change({ status: status as OrderListFilters["status"] })
                  }
                >
                  {status === ALL_STATUSES
                    ? t("orders.list.all_statuses")
                    : t(`orders.status.${status}`)}
                </Button>
              );
            })}
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="order-from">{t("orders.list.date_from")}</Label>
              <DatePicker
                id="order-from"
                value={filters.from}
                placeholder={t("orders.list.date_from")}
                onChange={(from) => change({ from })}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="order-to">{t("orders.list.date_to")}</Label>
              <DatePicker
                id="order-to"
                value={filters.to}
                placeholder={t("orders.list.date_to")}
                aria-invalid={badRange || undefined}
                onChange={(to) => change({ to })}
              />
            </div>
          </div>
          {badRange ? (
            <p className="text-destructive text-sm" data-testid="date-error">
              {t("orders.list.date_invalid")}
            </p>
          ) : null}
          {filtered ? (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              data-testid="clear-filters"
              onClick={() => {
                setFilters(EMPTY_ORDER_FILTERS);
                setPage(0);
              }}
            >
              {t("orders.list.clear_filters")}
            </Button>
          ) : null}
        </CardContent>
      </Card>

      {list.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void list.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : (
        <Card>
          <CardContent className="pt-6">
            {list.isLoading ? (
              <p className="text-muted-foreground text-sm">
                {t("orders.loading")}
              </p>
            ) : rows.length === 0 ? (
              <div className="py-8 text-center" data-testid="orders-empty">
                <p className="font-medium">{t("orders.list.empty_title")}</p>
                <p className="text-muted-foreground text-sm">
                  {filtered
                    ? t("orders.list.empty_filtered")
                    : t("orders.list.empty_description")}
                </p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm" data-testid="orders-table">
                  <thead>
                    <tr className="text-muted-foreground border-b text-start text-xs">
                      <th className="p-2 text-start font-medium">
                        {t("orders.list.columns.order_no")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {t("orders.list.columns.status")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {side === "seller"
                          ? t("orders.list.columns.buyer")
                          : t("orders.list.columns.seller")}
                      </th>
                      <th className="p-2 text-end font-medium">
                        {t("orders.list.columns.total")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {t("orders.list.columns.created_at")}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((o) => (
                      <tr
                        key={o.uuid}
                        className="hover:bg-accent/50 border-b last:border-0"
                        data-testid="order-row"
                        data-uuid={o.uuid}
                      >
                        <td className="p-2">
                          <Link
                            href={routes.tenant.orders.detail(slug, o.uuid)}
                            className="font-mono font-medium hover:underline"
                            dir="ltr"
                          >
                            {o.order_no}
                          </Link>
                        </td>
                        <td className="p-2">
                          <StatusChip
                            label={o.status_label}
                            tone={orderStatusTone(o.status)}
                          />
                        </td>
                        <td className="p-2">
                          {side === "seller" ? o.buyer.name : o.seller.name}
                        </td>
                        <td className="p-2 text-end whitespace-nowrap">
                          {format.currency(amountNumber(o.total), o.currency)}
                        </td>
                        <td className="p-2 whitespace-nowrap">
                          {format.dateTime(o.created_at)}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
            <div
              className={cn(
                "mt-4 flex flex-wrap items-center justify-between gap-2",
                rows.length === 0 && page === 0 && "hidden",
              )}
            >
              <p
                className="text-muted-foreground text-sm"
                data-testid="page-info"
              >
                {t("orders.list.page", { page: page + 1, pages, total })}
              </p>
              <div className="flex gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  data-testid="page-prev"
                  disabled={page === 0 || list.isFetching}
                  onClick={() => setPage((p) => Math.max(0, p - 1))}
                >
                  <ChevronLeft className="size-4 rtl:rotate-180" />
                  {t("orders.list.prev")}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  data-testid="page-next"
                  disabled={page + 1 >= pages || list.isFetching}
                  onClick={() => setPage((p) => p + 1)}
                >
                  {t("orders.list.next")}
                  <ChevronRight className="size-4 rtl:rotate-180" />
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
