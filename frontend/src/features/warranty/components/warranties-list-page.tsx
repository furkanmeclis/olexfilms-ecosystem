"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { ChevronLeft, ChevronRight, ShieldCheck } from "lucide-react";
import Link from "next/link";
import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { PageHeader } from "@/components/layout/page-header";
import { AsyncCombobox } from "@/components/ui/async-combobox";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { catalogService } from "@/features/catalog/services/catalog.service";
import { WarrantyFilterBar } from "@/features/warranty/components/warranty-filter-bar";
import { WarrantyProgressBar } from "@/features/warranty/components/warranty-progress";
import {
  buildWarrantyListQuery,
  EMPTY_WARRANTY_FILTERS,
  hasWarrantyFilters,
  pageCount,
  warrantyHolderName,
  warrantyStatusTone,
  warrantyVehicleTitle,
  type WarrantyListFilters,
} from "@/features/warranty/lib/warranty-list";
import {
  warrantyKeys,
  warrantyService,
} from "@/features/warranty/services/warranty.service";
import { useDebounce } from "@/hooks/use-debounce";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const WARRANTY_PAGE_SIZE = 20;

/**
 * Tenant > Warranties (TEC-191): warranties of the warranties.read scope
 * (dealer: own, distributor: subtree, center: brand) with status, "ends
 * within" and search filters, days left and the elapsed-period bar.
 */
export function WarrantiesListPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const canRead = can(Permission.WarrantiesRead);
  const canPickProduct = can(Permission.CatalogRead);
  const [filters, setFilters] = useState<WarrantyListFilters>(
    EMPTY_WARRANTY_FILTERS,
  );
  const [page, setPage] = useState(0);
  const q = useDebounce(filters.q, 300);

  const query = useMemo(
    () =>
      buildWarrantyListQuery(
        { ...filters, q },
        { limit: WARRANTY_PAGE_SIZE, offset: page * WARRANTY_PAGE_SIZE },
      ),
    [filters, q, page],
  );

  const list = useQuery({
    queryKey: warrantyKeys.list(query),
    queryFn: () => warrantyService.list(query),
    enabled: canRead,
    placeholderData: keepPreviousData,
  });

  const loadProducts = useCallback(async (term: string) => {
    const res = await catalogService.listProducts({ q: term, limit: 20 });
    return res.items.map((p) => ({
      value: p.uuid,
      label: p.name,
      description: p.sku,
    }));
  }, []);

  const change = (patch: Partial<WarrantyListFilters>) => {
    setFilters((f) => ({ ...f, ...patch }));
    setPage(0);
  };

  const title = t("warranty.list.title");
  const header = (
    <PageHeader
      title={title}
      icon={<ShieldCheck className="size-6" />}
      description={t("warranty.list.description")}
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
          description={t("warranty.list.forbidden")}
        />
      </div>
    );
  }

  const total = list.data?.total ?? 0;
  const pages = pageCount(total, WARRANTY_PAGE_SIZE);
  const rows = list.data?.items ?? [];
  const filtered = hasWarrantyFilters(filters);

  return (
    <div className="space-y-6">
      {header}
      <Card>
        <CardContent className="pt-6">
          <WarrantyFilterBar
            filters={filters}
            onChange={change}
            searchPlaceholder={t("warranty.list.search_placeholder")}
            extra={
              canPickProduct ? (
                <div className="space-y-1.5">
                  <Label htmlFor="warranty-product">
                    {t("warranty.list.product")}
                  </Label>
                  <AsyncCombobox
                    id="warranty-product"
                    value={filters.productUuid}
                    clearable
                    onValueChange={(value) => change({ productUuid: value })}
                    loadOptions={loadProducts}
                    placeholder={t("warranty.list.product_placeholder")}
                    searchPlaceholder={t("warranty.list.product_search")}
                    emptyText={t("warranty.list.product_none")}
                  />
                </div>
              ) : null
            }
          />
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
                {t("warranty.list.loading")}
              </p>
            ) : rows.length === 0 ? (
              <div className="py-8 text-center" data-testid="warranties-empty">
                <p className="font-medium">{t("warranty.list.empty_title")}</p>
                <p className="text-muted-foreground text-sm">
                  {filtered
                    ? t("warranty.list.empty_filtered")
                    : t("warranty.list.empty_description")}
                </p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <table
                  className="w-full text-sm"
                  data-testid="warranties-table"
                >
                  <thead>
                    <tr className="text-muted-foreground border-b text-xs">
                      <th className="p-2 text-start font-medium">
                        {t("warranty.list.columns.code")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {t("warranty.list.columns.status")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {t("warranty.list.columns.product")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {t("warranty.list.columns.vehicle")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {t("warranty.list.columns.holder")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {t("warranty.list.columns.organization")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {t("warranty.list.columns.period")}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((w) => (
                      <tr
                        key={w.uuid}
                        className="hover:bg-accent/50 border-b align-top last:border-0"
                        data-testid="warranty-row"
                        data-uuid={w.uuid}
                      >
                        <td className="p-2">
                          <Link
                            href={routes.tenant.warranties.detail(slug, w.uuid)}
                            className="font-mono text-xs font-medium hover:underline"
                            dir="ltr"
                          >
                            {w.public_code}
                          </Link>
                          <div
                            className="text-muted-foreground font-mono text-xs"
                            dir="ltr"
                          >
                            {w.service.service_no}
                          </div>
                        </td>
                        <td className="p-2">
                          <StatusChip
                            label={t(`warranty.status.${w.status}`)}
                            tone={warrantyStatusTone(w.status)}
                          />
                        </td>
                        <td className="p-2">{w.product.name}</td>
                        <td className="p-2">
                          <div>{warrantyVehicleTitle(w)}</div>
                          {w.vehicle.plate ? (
                            <div
                              className="text-muted-foreground font-mono text-xs"
                              dir="ltr"
                            >
                              {w.vehicle.plate}
                            </div>
                          ) : null}
                        </td>
                        <td className="p-2">{warrantyHolderName(w)}</td>
                        <td className="p-2">{w.organization.name}</td>
                        <td className="p-2">
                          <WarrantyProgressBar warranty={w} compact />
                          <div className="text-muted-foreground mt-1 text-xs whitespace-nowrap">
                            {format.date(w.start_at)} – {format.date(w.end_at)}
                          </div>
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
                {t("warranty.list.page", { page: page + 1, pages, total })}
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
                  {t("warranty.list.prev")}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  data-testid="page-next"
                  disabled={page + 1 >= pages || list.isFetching}
                  onClick={() => setPage((p) => p + 1)}
                >
                  {t("warranty.list.next")}
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
