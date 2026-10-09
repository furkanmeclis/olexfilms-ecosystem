"use client";

import { useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Send, Trash2 } from "lucide-react";
import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  CLIENT_SIDE_MANUAL,
  EntityPage,
  EntityRowActions,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { Money } from "@/features/accounting/components/shared";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { DeviationBadge } from "@/features/pricing/components/deviation-badge";
import { PublishDialog } from "@/features/pricing/components/publish-dialog";
import { RecommendedEditor } from "@/features/pricing/components/recommended-editor";
import { RecommendedHistory } from "@/features/pricing/components/recommended-history";
import { useDeviationThreshold } from "@/features/pricing/hooks/use-deviation-threshold";
import {
  basketKey,
  toNumber,
  type BasketRow,
} from "@/features/pricing/lib/recommended";
import { pricingKeys } from "@/features/pricing/services/recommended.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const RECOMMENDED_BASKET_PERSIST_KEY = "tenant-recommended-basket-v1";

/** (new - previous) / previous in percent; null without a previous price. */
function changePct(row: BasketRow) {
  const prev = toNumber(row.previous);
  const next = toNumber(row.price);
  if (prev === null || next === null || prev === 0) return null;
  return ((next - prev) / prev) * 100;
}

/**
 * Tenant > Catalog > Recommended prices (TEC-507). Brand center only: the
 * price list editor feeding a publication basket, the "Publish" dialog
 * (effective day, note, step-up) and the version history.
 */
export function RecommendedPricesPage({ slug }: { slug: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const queryClient = useQueryClient();
  const isCenter = org?.type === "center";
  const canWrite = isCenter && can(permissions.pricing.recommendedWrite);
  const threshold = useDeviationThreshold(isCenter);

  const [basket, setBasket] = useState<ReadonlyMap<string, BasketRow>>(
    () => new Map(),
  );
  const [publishOpen, setPublishOpen] = useState(false);
  const basketRows = useMemo(() => [...basket.values()], [basket]);

  const changeBasket = useCallback((rows: BasketRow[], remove?: string[]) => {
    setBasket((prev) => {
      const next = new Map(prev);
      for (const key of remove ?? []) next.delete(key);
      for (const row of rows) next.set(basketKey(row), row);
      return next;
    });
  }, []);

  const basketColumns = useMemo(
    () =>
      [
        createColumn<BasketRow>({
          accessorKey: "productName",
          labelKey: "catalog.fields.name",
          enableSorting: true,
          gridPrimary: true,
        }),
        createColumn<BasketRow>({
          accessorKey: "productSku",
          labelKey: "catalog.fields.sku",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="font-mono text-xs" dir="ltr">
              {row.original.productSku}
            </span>
          ),
        }),
        createColumn<BasketRow>({
          accessorKey: "country",
          labelKey: "catalog.recommended.fields.country",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.country || t("catalog.recommended.scope_currency"),
        }),
        createColumn<BasketRow>({
          accessorKey: "currency",
          labelKey: "catalog.recommended.fields.currency",
          enableSorting: true,
        }),
        createColumn<BasketRow>({
          id: "previous",
          accessorFn: (row) => toNumber(row.previous),
          labelKey: "catalog.recommended.fields.current_price",
          enableSorting: true,
          sortUndefined: "last",
          cell: ({ row }) =>
            row.original.previous ? (
              <Money
                amount={row.original.previous}
                currency={row.original.currency}
              />
            ) : (
              "—"
            ),
        }),
        createColumn<BasketRow>({
          id: "price",
          accessorFn: (row) => toNumber(row.price),
          labelKey: "catalog.recommended.fields.new_price",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="flex flex-wrap items-center gap-2">
              <Money
                amount={row.original.price}
                currency={row.original.currency}
              />
              <DeviationBadge
                value={changePct(row.original)}
                threshold={threshold}
              />
            </span>
          ),
        }),
        createColumn<BasketRow>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "remove",
                  label: t("catalog.recommended.bulk.remove"),
                  icon: Trash2,
                  variant: "destructive",
                  onSelect: () => changeBasket([], [basketKey(row.original)]),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<BasketRow, unknown>[],
    [changeBasket, t, threshold],
  );

  const title = t("catalog.recommended.title");
  return (
    <EntityPage
      title={title}
      description={t("catalog.recommended.description")}
      permission={permissions.pricing.recommendedRead}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("catalog.recommended.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("layout.section_catalog"),
          href: routes.tenant.catalog.products(slug),
        },
        { label: title },
      ]}
      actions={
        canWrite ? (
          <Button
            type="button"
            data-testid="open-publish"
            disabled={basketRows.length === 0}
            onClick={() => setPublishOpen(true)}
          >
            <Send className="size-4" />
            {t("catalog.recommended.publish.open", {
              count: basketRows.length,
            })}
          </Button>
        ) : null
      }
    >
      {org && !isCenter ? (
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("catalog.recommended.center_only")}
        />
      ) : (
        <Tabs defaultValue="prices" className="space-y-4">
          <TabsList>
            <TabsTrigger value="prices">
              {t("catalog.recommended.tabs.prices")}
            </TabsTrigger>
            <TabsTrigger value="history">
              {t("catalog.recommended.tabs.history")}
            </TabsTrigger>
          </TabsList>
          <TabsContent value="prices" className="space-y-4">
            {basketRows.length > 0 ? (
              <Card data-testid="publish-basket">
                <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-2">
                  <div className="space-y-1">
                    <CardTitle>
                      {t("catalog.recommended.basket.title")}
                    </CardTitle>
                    <CardDescription>
                      {t("catalog.recommended.basket.description", {
                        count: basketRows.length,
                      })}
                    </CardDescription>
                  </div>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => setBasket(new Map())}
                  >
                    {t("catalog.recommended.basket.clear")}
                  </Button>
                </CardHeader>
                <CardContent>
                  <EntityTable
                    columns={basketColumns}
                    data={basketRows}
                    getRowId={(row) => basketKey(row)}
                    manual={CLIENT_SIDE_MANUAL}
                    initialState={{
                      pagination: { pageIndex: 0, pageSize: 10 },
                    }}
                    features={{
                      persistKey: RECOMMENDED_BASKET_PERSIST_KEY,
                      rowSelection: false,
                      columnFilters: false,
                      facetedFilters: false,
                    }}
                  />
                </CardContent>
              </Card>
            ) : null}
            <RecommendedEditor
              slug={slug}
              canWrite={canWrite}
              basket={basket}
              onBasketChange={changeBasket}
            />
          </TabsContent>
          <TabsContent value="history">
            <RecommendedHistory />
          </TabsContent>
        </Tabs>
      )}
      {canWrite ? (
        <PublishDialog
          open={publishOpen}
          rows={basketRows}
          onOpenChange={setPublishOpen}
          onPublished={() => {
            setPublishOpen(false);
            setBasket(new Map());
            void queryClient.invalidateQueries({ queryKey: pricingKeys.all });
          }}
        />
      ) : null}
    </EntityPage>
  );
}
