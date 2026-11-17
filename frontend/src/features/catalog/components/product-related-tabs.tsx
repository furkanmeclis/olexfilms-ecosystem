"use client";

import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityLink,
  EntityTable,
  RelatedList,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Card, CardContent } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Permission } from "@/config/permissions";
import { RecommendedHistory } from "@/features/pricing/components/recommended-history";
import {
  stockKeys,
  stockService,
  type StockUnitRow,
} from "@/features/stock/services/stock.service";
import type { components } from "@/generated/api";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type OrderStatus = components["schemas"]["OrderStatus"];

/** Orders still moving (not delivered / received / cancelled). */
export const OPEN_ORDER_STATUSES: readonly OrderStatus[] = [
  "draft",
  "submitted",
  "approved",
  "preparing",
  "ready",
  "processing",
  "shipped",
];

const STOCK_STATUSES = [
  "reserved",
  "printed",
  "available",
  "placed",
  "in_transit",
  "used",
  "void",
] as const;

type ProductTab = "stock" | "orders" | "warranties" | "prices";

/**
 * TEC-1218: stock units of one product in the active organization (the
 * unit list endpoint with `product_uuid`). No price column: warehouse
 * roles must not see prices (K8, TEC-1050).
 */
function ProductStockUnits({
  productUuid,
  orgUuid,
}: {
  productUuid: string;
  orgUuid: string;
}) {
  const { t, format } = useLocale();
  const columns = useMemo(
    () => [
      createColumn<StockUnitRow>({
        accessorKey: "barcode",
        labelKey: "stock.columns.unit",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <EntityLink
            kind="stock_unit"
            entity={{
              barcode: row.original.barcode,
              name: row.original.barcode,
            }}
            className="font-mono"
          />
        ),
      }),
      createColumn<StockUnitRow>({
        accessorKey: "status",
        labelKey: "stock.columns.status",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: STOCK_STATUSES.map((value) => ({
          value,
          label: value,
          labelKey: `stock.status.${value}`,
        })),
        param: "status",
        cell: ({ row }) => (
          <StatusChip label={t(`stock.status.${row.original.status}`)} />
        ),
      }),
      createColumn<StockUnitRow>({
        accessorKey: "quantity",
        labelKey: "stock.columns.quantity",
        enableSorting: true,
        meta: { cellClassName: "text-end", headerClassName: "text-end" },
        cell: ({ row }) => format.number(row.original.quantity),
      }),
      createColumn<StockUnitRow>({
        id: "meters",
        accessorFn: (row) => row.remaining_meters ?? "",
        labelKey: "stock.columns.meters",
        enableSorting: true,
        meta: { cellClassName: "text-end", headerClassName: "text-end" },
        cell: ({ row }) =>
          row.original.remaining_meters
            ? format.number(Number(row.original.remaining_meters))
            : "—",
      }),
      createColumn<StockUnitRow>({
        id: "location",
        accessorFn: (row) => row.location?.code ?? "",
        labelKey: "stock.columns.location",
        enableSorting: false,
        cell: ({ row }) =>
          row.original.location
            ? `${row.original.location.code} · ${row.original.location.name}`
            : "—",
      }),
      createColumn<StockUnitRow>({
        accessorKey: "updated_at",
        labelKey: "stock.columns.updated",
        enableSorting: true,
        cell: ({ row }) => format.date(row.original.updated_at),
      }),
    ],
    [format, t],
  );
  const persistKey = "catalog-product-stock-units-v1";
  const list = useServerListState({
    columns,
    initialSort: "-updated_at",
    initialPageSize: 10,
    persistKey,
  });
  const params = useMemo(
    () => ({ ...list.params, product_uuid: productUuid }),
    [list.params, productUuid],
  );
  const result = useQuery({
    queryKey: stockKeys.units(orgUuid, params),
    queryFn: () => stockService.listUnits(orgUuid, params),
  });
  return (
    <EntityTable
      columns={columns}
      data={result.data?.items ?? []}
      getRowId={(row) => row.uuid}
      isLoading={result.isLoading}
      isError={result.isError}
      onRetry={() => void result.refetch()}
      emptyTitle={t("catalog.related.empty_stock")}
      rowCount={result.data?.total ?? 0}
      state={list.tableState}
      features={{ persistKey }}
    />
  );
}

/**
 * TEC-1218: reverse relations of a product — its stock units, open orders,
 * warranties and recommended price history. Each tab shows only with the
 * module's read permission; the price tab only with the recommended price
 * read permission in the center (the endpoint's scope).
 */
export function ProductRelatedTabs({
  slug,
  productUuid,
}: {
  slug: string;
  productUuid: string;
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const tabs = useMemo(() => {
    const list: ProductTab[] = [];
    if (can(Permission.StockRead) && org?.uuid) list.push("stock");
    if (can(Permission.OrdersRead)) list.push("orders");
    if (can(Permission.WarrantiesRead)) list.push("warranties");
    if (can(Permission.PricingRecommendedRead) && org?.type === "center") {
      list.push("prices");
    }
    return list;
  }, [can, org?.type, org?.uuid]);
  const [tab, setTab] = useState<ProductTab | null>(null);
  const active = tab && tabs.includes(tab) ? tab : tabs[0];
  const orderFilter = useMemo(
    () => ({
      product_uuid: productUuid,
      status: OPEN_ORDER_STATUSES.join(","),
    }),
    [productUuid],
  );
  const productFilter = useMemo(
    () => ({ product_uuid: productUuid }),
    [productUuid],
  );
  if (!active) return null;

  return (
    <Card data-testid="product-related-tabs">
      <CardContent className="pt-6">
        <Tabs value={active} onValueChange={(v) => setTab(v as ProductTab)}>
          <TabsList>
            {tabs.map((value) => (
              <TabsTrigger key={value} value={value}>
                {t(`catalog.related.tabs.${value}`)}
              </TabsTrigger>
            ))}
          </TabsList>
          {tabs.includes("stock") && org?.uuid ? (
            <TabsContent value="stock" className="pt-4">
              <ProductStockUnits productUuid={productUuid} orgUuid={org.uuid} />
            </TabsContent>
          ) : null}
          <TabsContent value="orders" className="pt-4">
            <RelatedList
              kind="order"
              filter={orderFilter}
              persistKey="catalog-product-orders-v1"
              enabled={active === "orders"}
            />
          </TabsContent>
          <TabsContent value="warranties" className="pt-4">
            <RelatedList
              kind="warranty"
              filter={productFilter}
              persistKey="catalog-product-warranties-v1"
              hideColumns={["product"]}
              enabled={active === "warranties"}
            />
          </TabsContent>
          {tabs.includes("prices") ? (
            <TabsContent value="prices" className="pt-4">
              <RecommendedHistory productUuid={productUuid} />
            </TabsContent>
          ) : null}
        </Tabs>
      </CardContent>
    </Card>
  );
}
