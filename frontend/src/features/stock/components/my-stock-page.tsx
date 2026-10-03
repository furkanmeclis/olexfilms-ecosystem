"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { Boxes, ChevronLeft, ChevronRight } from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  ALL,
  buildUnitsQuery,
  EMPTY_STOCK_FILTERS,
  hasActiveFilters,
  pageCount,
  showPurchasePrice,
  STOCK_FILTER_STATUSES,
  STOCK_TABS,
  unitStatusTone,
  type StockListFilters,
  type StockTab,
} from "@/features/stock/lib/stock";
import {
  stockKeys,
  stockService,
  type StockUnitRow,
} from "@/features/stock/services/stock.service";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useDebounce } from "@/hooks/use-debounce";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const STOCK_PAGE_SIZE = 20;

/** The active organization itself in the dealer picker. */
const OWN = "own";

const selectClass =
  "border-input bg-background h-9 w-full rounded-md border px-2 text-sm";

/**
 * Tenant > My stock (TEC-224, K12): the units held by the active dealer
 * (or distributor) and, on the second tab, the units consumed in services
 * (status `used`). A distributor picks one dealer of its subtree and reads
 * that dealer's stock as is (stock.read at scope subtree, TEC-216). The
 * purchase price column shows only when the API answers a price (K8).
 */
export function MyStockPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const canRead = can(Permission.StockRead);
  const isDistributor = org?.type === "distributor";

  const [tab, setTab] = useState<StockTab>("stock");
  const [dealer, setDealer] = useState<string>(OWN);
  const [filters, setFilters] = useState<StockListFilters>(EMPTY_STOCK_FILTERS);
  const [page, setPage] = useState(0);
  const q = useDebounce(filters.q, 300);
  const barcode = useDebounce(filters.barcode, 300);

  const targetUuid = dealer === OWN ? (org?.uuid ?? "") : dealer;

  const dealers = useQuery({
    queryKey: stockKeys.dealers,
    queryFn: () => stockService.listDealers(),
    enabled: canRead && isDistributor,
    staleTime: 60_000,
  });

  const productsQuery = useMemo(() => ({ limit: 200, offset: 0 }) as const, []);
  const products = useQuery({
    queryKey: stockKeys.products(targetUuid, productsQuery),
    queryFn: () => stockService.listProducts(targetUuid, productsQuery),
    enabled: canRead && targetUuid !== "",
    staleTime: 60_000,
  });

  const query = useMemo(
    () =>
      buildUnitsQuery(
        tab,
        { ...filters, q, barcode },
        { limit: STOCK_PAGE_SIZE, offset: page * STOCK_PAGE_SIZE },
      ),
    [tab, filters, q, barcode, page],
  );

  const list = useQuery({
    queryKey: stockKeys.units(targetUuid, query),
    queryFn: () => stockService.listUnits(targetUuid, query),
    enabled: canRead && targetUuid !== "",
    placeholderData: keepPreviousData,
  });

  const change = (patch: Partial<StockListFilters>) => {
    setFilters((prev) => ({ ...prev, ...patch }));
    setPage(0);
  };

  const title = t("stock.page.title");
  const header = (
    <PageHeader
      title={title}
      icon={<Boxes className="size-6" />}
      description={t("stock.page.description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: title },
      ]}
    />
  );

  if (!canRead) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("stock.page.forbidden")}
        />
      </div>
    );
  }

  const rows = list.data?.items ?? [];
  const total = list.data?.total ?? 0;
  const pages = pageCount(total, STOCK_PAGE_SIZE);
  const priceColumn = showPurchasePrice(rows);
  const viewingDealer = isDistributor && dealer !== OWN;
  const dealerName = dealers.data?.find((d) => d.uuid === dealer)?.name ?? "";

  return (
    <div className="space-y-6">
      {header}

      {isDistributor ? (
        <Card>
          <CardContent className="space-y-2 pt-6">
            <Label htmlFor="stock-dealer">{t("stock.dealer.label")}</Label>
            <select
              id="stock-dealer"
              data-testid="stock-dealer"
              className={cn(selectClass, "max-w-md")}
              value={dealer}
              onChange={(e) => {
                setDealer(e.target.value);
                setFilters(EMPTY_STOCK_FILTERS);
                setPage(0);
              }}
            >
              <option value={OWN}>
                {t("stock.dealer.own", { name: org?.name ?? "" })}
              </option>
              {(dealers.data ?? []).map((d) => (
                <option key={d.uuid} value={d.uuid}>
                  {d.name}
                </option>
              ))}
            </select>
            <p className="text-muted-foreground text-xs">
              {viewingDealer
                ? t("stock.dealer.read_only", { name: dealerName })
                : t("stock.dealer.hint")}
            </p>
          </CardContent>
        </Card>
      ) : null}

      <div
        className="flex flex-wrap gap-2"
        role="tablist"
        aria-label={t("stock.tabs.label")}
      >
        {STOCK_TABS.map((value) => {
          const active = tab === value;
          return (
            <Button
              key={value}
              type="button"
              role="tab"
              size="sm"
              variant={active ? "default" : "outline"}
              aria-selected={active}
              data-testid={`stock-tab-${value}`}
              onClick={() => {
                setTab(value);
                setPage(0);
              }}
            >
              {t(`stock.tabs.${value}`)}
            </Button>
          );
        })}
      </div>

      <Card>
        <CardContent className="pt-6">
          <div className="grid gap-3 md:grid-cols-4">
            <div className="space-y-1.5">
              <Label htmlFor="stock-search">{t("stock.filters.search")}</Label>
              <Input
                id="stock-search"
                type="search"
                value={filters.q}
                maxLength={100}
                placeholder={t("stock.filters.search_placeholder")}
                onChange={(e) => change({ q: e.target.value })}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="stock-barcode">
                {t("stock.filters.barcode")}
              </Label>
              <Input
                id="stock-barcode"
                value={filters.barcode}
                maxLength={64}
                dir="ltr"
                placeholder={t("stock.filters.barcode_placeholder")}
                onChange={(e) => change({ barcode: e.target.value })}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="stock-product">
                {t("stock.filters.product")}
              </Label>
              <select
                id="stock-product"
                className={selectClass}
                value={filters.product}
                onChange={(e) => change({ product: e.target.value })}
              >
                <option value={ALL}>{t("stock.filters.all_products")}</option>
                {(products.data?.items ?? []).map((p) => (
                  <option key={p.product.uuid} value={p.product.uuid}>
                    {p.product.name} · {p.product.sku}
                  </option>
                ))}
              </select>
            </div>
            {tab === "stock" ? (
              <div className="space-y-1.5">
                <Label htmlFor="stock-status">
                  {t("stock.filters.status")}
                </Label>
                <select
                  id="stock-status"
                  className={selectClass}
                  value={filters.status}
                  onChange={(e) =>
                    change({
                      status: e.target.value as StockListFilters["status"],
                    })
                  }
                >
                  <option value={ALL}>{t("stock.filters.all_statuses")}</option>
                  {STOCK_FILTER_STATUSES.map((s) => (
                    <option key={s} value={s}>
                      {t(`stock.status.${s}`)}
                    </option>
                  ))}
                </select>
              </div>
            ) : null}
          </div>
          {hasActiveFilters(filters) ? (
            <div className="mt-3">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                data-testid="stock-clear-filters"
                onClick={() => {
                  setFilters(EMPTY_STOCK_FILTERS);
                  setPage(0);
                }}
              >
                {t("stock.filters.clear")}
              </Button>
            </div>
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
                {t("stock.list.loading")}
              </p>
            ) : rows.length === 0 ? (
              <div className="py-8 text-center" data-testid="stock-empty">
                <p className="font-medium">
                  {t(
                    tab === "consumed"
                      ? "stock.list.empty_consumed_title"
                      : "stock.list.empty_title",
                  )}
                </p>
                <p className="text-muted-foreground text-sm">
                  {t(
                    hasActiveFilters(filters)
                      ? "stock.list.empty_filtered"
                      : tab === "consumed"
                        ? "stock.list.empty_consumed_description"
                        : "stock.list.empty_description",
                  )}
                </p>
              </div>
            ) : (
              <StockUnitsTable
                rows={rows}
                priceColumn={priceColumn}
                t={t}
                format={format}
              />
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
                {t("stock.list.page", { page: page + 1, pages, total })}
              </p>
              <div className="flex gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={page === 0 || list.isFetching}
                  onClick={() => setPage((p) => Math.max(0, p - 1))}
                >
                  <ChevronLeft className="size-4 rtl:rotate-180" />
                  {t("stock.list.prev")}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={page + 1 >= pages || list.isFetching}
                  onClick={() => setPage((p) => p + 1)}
                >
                  {t("stock.list.next")}
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

type Locale = ReturnType<typeof useLocale>;

function StockUnitsTable({
  rows,
  priceColumn,
  t,
  format,
}: {
  rows: StockUnitRow[];
  priceColumn: boolean;
  t: Locale["t"];
  format: Locale["format"];
}) {
  const th = "p-2 text-start font-medium";
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm" data-testid="stock-table">
        <thead>
          <tr className="text-muted-foreground border-b text-xs">
            <th className={th}>{t("stock.columns.unit")}</th>
            <th className={th}>{t("stock.columns.product")}</th>
            <th className={th}>{t("stock.columns.status")}</th>
            <th className={cn(th, "text-end")}>
              {t("stock.columns.quantity")}
            </th>
            <th className={cn(th, "text-end")}>{t("stock.columns.meters")}</th>
            <th className={th}>{t("stock.columns.location")}</th>
            {priceColumn ? (
              <th
                className={cn(th, "text-end")}
                data-testid="stock-price-header"
              >
                {t("stock.columns.purchase_price")}
              </th>
            ) : null}
            <th className={th}>{t("stock.columns.updated")}</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr
              key={r.uuid}
              className="hover:bg-accent/50 border-b align-top last:border-0"
              data-testid="stock-row"
              data-uuid={r.uuid}
            >
              <td className="p-2">
                <span className="font-mono text-xs font-medium" dir="ltr">
                  {r.barcode}
                </span>
                <div className="text-muted-foreground text-xs">
                  {t(`stock.unit_kind.${r.unit_kind}`)}
                </div>
              </td>
              <td className="p-2">
                <div className="font-medium">{r.product.name}</div>
                <div className="text-muted-foreground font-mono text-xs">
                  {r.product.sku}
                </div>
              </td>
              <td className="p-2">
                <StatusChip
                  label={t(`stock.status.${r.status}`)}
                  tone={unitStatusTone(r.status)}
                />
              </td>
              <td className="p-2 text-end tabular-nums">
                {format.number(r.quantity)}
              </td>
              <td className="p-2 text-end whitespace-nowrap tabular-nums">
                {r.product.unit_type === "roll_meter" && r.remaining_meters
                  ? t("stock.list.meters_of", {
                      remaining: r.remaining_meters,
                      initial: r.initial_meters ?? r.remaining_meters,
                    })
                  : "—"}
              </td>
              <td className="p-2">
                {r.location ? (
                  <>
                    <span className="font-mono text-xs" dir="ltr">
                      {r.location.code}
                    </span>
                    <div className="text-muted-foreground text-xs">
                      {r.location.name}
                    </div>
                  </>
                ) : (
                  <span className="text-muted-foreground">—</span>
                )}
              </td>
              {priceColumn ? (
                <td
                  className="p-2 text-end whitespace-nowrap tabular-nums"
                  data-testid="stock-price"
                >
                  {r.purchase_price
                    ? format.currency(
                        Number(r.purchase_price.amount),
                        r.purchase_price.currency,
                      )
                    : "—"}
                </td>
              ) : null}
              <td className="text-muted-foreground p-2 text-xs whitespace-nowrap">
                {format.dateTime(r.updated_at)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
