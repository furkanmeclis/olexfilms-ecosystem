"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { AlertTriangle } from "lucide-react";
import { useMemo } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { Money } from "@/features/accounting/components/shared";
import { useDealerSalesAccess } from "@/features/dealer-sales/hooks/use-dealer-sales-access";
import { isBelowCost, normalizeMoney } from "@/features/dealer-sales/lib/sales";
import {
  dealerSalesKeys,
  dealerSalesService,
  type ListQuery,
  type PriceCatalogItem,
} from "@/features/dealer-sales/services/dealer-sales.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const SALE_PRICES_PERSIST_KEY = "tenant-dealer-sale-prices-v1";

function Dash() {
  return <span className="text-muted-foreground">—</span>;
}

/**
 * The dealer's own sale price; a warning badge when it is under the
 * purchase price (selling at a loss).
 */
export function SalePriceCell({ item }: { item: PriceCatalogItem }) {
  const { t } = useLocale();
  if (!item.sale_price) {
    return (
      <span className="text-muted-foreground text-xs">
        {t("dealer_sales.prices.not_set")}
      </span>
    );
  }
  return (
    <div className="flex flex-wrap items-center justify-end gap-2">
      <Money amount={item.sale_price} currency={item.currency} />
      {isBelowCost(item.sale_price, item.purchase_price) ? (
        <Badge
          variant="warning"
          data-testid="below-cost-badge"
          title={t("dealer_sales.prices.below_cost_hint")}
        >
          <AlertTriangle className="size-3" />
          {t("dealer_sales.prices.below_cost")}
        </Badge>
      ) : null}
    </div>
  );
}

/**
 * Tenant > Dealer sales > Sale prices (TEC-348). The brand's piece
 * products with the purchase price (pricing.purchase.read), the
 * recommended price, the dealer's own sale price (double-click to edit)
 * and the estimated profit. Server DataTable: sort, `q` (name, SKU) and
 * the priced filter.
 */
export function SalePricesPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const access = useDealerSalesAccess(slug);
  const queryClient = useQueryClient();

  const columns = useMemo(
    () =>
      [
        createColumn<PriceCatalogItem>({
          accessorKey: "name",
          labelKey: "dealer_sales.fields.product",
          enableSorting: true,
          enableHiding: false,
          enableColumnFilter: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.name}</span>
          ),
        }),
        createColumn<PriceCatalogItem>({
          accessorKey: "sku",
          labelKey: "dealer_sales.fields.sku",
          enableSorting: true,
          enableColumnFilter: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="font-mono text-xs" dir="ltr">
              {row.original.sku}
            </span>
          ),
        }),
        ...(access.canSeePurchasePrice
          ? [
              createColumn<PriceCatalogItem>({
                accessorKey: "purchase_price",
                labelKey: "dealer_sales.fields.purchase_price",
                enableSorting: false,
                enableColumnFilter: false,
                cell: ({ row }) =>
                  row.original.purchase_price ? (
                    <Money
                      amount={row.original.purchase_price}
                      currency={row.original.currency}
                    />
                  ) : (
                    <Dash />
                  ),
              }),
            ]
          : []),
        createColumn<PriceCatalogItem>({
          accessorKey: "recommended_sale_price",
          labelKey: "dealer_sales.fields.recommended_price",
          enableSorting: true,
          enableColumnFilter: false,
          cell: ({ row }) =>
            row.original.recommended_sale_price ? (
              <Money
                amount={row.original.recommended_sale_price}
                currency={row.original.currency}
              />
            ) : (
              <Dash />
            ),
        }),
        createColumn<PriceCatalogItem>({
          id: "sale_price",
          accessorFn: (row) => row.sale_price ?? "",
          labelKey: "dealer_sales.fields.sale_price",
          enableSorting: true,
          enableColumnFilter: false,
          editVariant: "text",
          cell: ({ row }) => <SalePriceCell item={row.original} />,
        }),
        ...(access.canSeePurchasePrice
          ? [
              createColumn<PriceCatalogItem>({
                accessorKey: "estimated_profit",
                labelKey: "dealer_sales.fields.estimated_profit",
                enableSorting: false,
                enableColumnFilter: false,
                cell: ({ row }) =>
                  row.original.estimated_profit ? (
                    <Money
                      amount={row.original.estimated_profit}
                      currency={row.original.currency}
                    />
                  ) : (
                    <Dash />
                  ),
              }),
            ]
          : []),
        createColumn<PriceCatalogItem>({
          id: "priced",
          accessorFn: (row) => Boolean(row.sale_price),
          labelKey: "dealer_sales.fields.priced",
          enableSorting: false,
          filterVariant: "boolean",
          param: "priced",
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.sale_price ? t("table.true") : t("table.false"),
        }),
        createColumn<PriceCatalogItem>({
          accessorKey: "updated_at",
          labelKey: "dealer_sales.fields.updated_at",
          enableSorting: true,
          enableColumnFilter: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.updated_at ? (
              format.dateTime(row.original.updated_at)
            ) : (
              <Dash />
            ),
        }),
      ] as ColumnDef<PriceCatalogItem, unknown>[],
    [access.canSeePurchasePrice, format, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "name",
    persistKey: SALE_PRICES_PERSIST_KEY,
  });
  const params: ListQuery = listState.params;

  const list = useQuery({
    queryKey: dealerSalesKeys.prices(access.orgUuid, params),
    queryFn: () => dealerSalesService.listPrices(params),
    enabled: access.canPrices && Boolean(access.orgUuid),
  });

  const setPrice = useMutation({
    mutationFn: (v: { item: PriceCatalogItem; price: string }) =>
      dealerSalesService.setPrice(v.item.product_uuid, v.price),
    onSuccess: async (_saved, v) => {
      await queryClient.invalidateQueries({
        queryKey: dealerSalesKeys.all(access.orgUuid),
      });
      if (isBelowCost(v.price, v.item.purchase_price)) {
        appToast.warning(t("dealer_sales.prices.below_cost_hint"));
      } else {
        appToast.success(t("dealer_sales.prices.saved"));
      }
    },
    onError: (error) =>
      appToast.error(
        isApiError(error) ? error.message : t("dealer_sales.toast.failed"),
      ),
  });

  return (
    <EntityPage
      title={t("dealer_sales.prices.title")}
      description={t("dealer_sales.prices.description")}
      permission={permissions.dealerSales.pricingWrite}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("dealer_sales.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("dealer_sales.nav") },
        { label: t("dealer_sales.prices.title") },
      ]}
    >
      <p className="text-muted-foreground text-sm">
        {t("dealer_sales.prices.edit_hint")}
      </p>
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.product_uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("dealer_sales.prices.empty_title")}
        emptyDescription={t("dealer_sales.prices.empty_description")}
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: SALE_PRICES_PERSIST_KEY,
          rowSelection: false,
          inlineEdit: access.canPrices,
        }}
        onCellEdit={({ row, columnId, value }) => {
          if (columnId !== "sale_price") return;
          const raw = String(value ?? "").trim();
          if (!raw || raw === row.sale_price) return;
          const price = normalizeMoney(raw);
          if (price === null) {
            appToast.error(t("dealer_sales.validation.price"));
            return;
          }
          if (price === row.sale_price) return;
          setPrice.mutate({ item: row, price });
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void list.refetch()}
            refreshDisabled={list.isFetching}
          />
        }
      />
    </EntityPage>
  );
}
