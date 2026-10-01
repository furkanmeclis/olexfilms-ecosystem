"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { DeleteDialog } from "@/components/dialogs/delete-dialog";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { PriceDialog } from "@/features/catalog/components/price-dialog";
import { catalogKeys } from "@/features/catalog/hooks/use-catalog-access";
import type { PricingAccess } from "@/features/catalog/lib/access";
import { priceNumber } from "@/features/catalog/lib/prices";
import {
  pricingService,
  type DistributorPrice,
} from "@/features/catalog/services/pricing.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type Editing = { row: DistributorPrice | null } | null;

/**
 * Center only: distributor-specific prices of one product (TEC-146). Such a
 * price replaces the list sale price as that distributor's purchase price.
 */
export function DistributorPricesCard({
  productUuid,
  access,
}: {
  productUuid: string;
  access: PricingAccess;
}) {
  const { t, format } = useLocale();
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<Editing>(null);
  const [deleting, setDeleting] = useState<DistributorPrice | null>(null);
  const canWrite = access.canWriteDistributorPrices;

  const list = useQuery({
    queryKey: catalogKeys.distributorPrices(productUuid),
    queryFn: () => pricingService.listDistributorPrices(productUuid),
    enabled: access.canReadDistributorPrices,
  });
  const distributors = useQuery({
    queryKey: catalogKeys.distributors,
    queryFn: () => pricingService.listDistributors(),
    enabled: canWrite && Boolean(editing),
    staleTime: 60_000,
  });

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: catalogKeys.distributorPrices(productUuid),
    });
  const onError = (error: unknown) =>
    appToast.error(
      isApiError(error) ? error.message : t("catalog.toast.failed"),
    );

  const save = useMutation({
    mutationFn: (v: Record<string, string>) =>
      pricingService.setDistributorPrice(
        productUuid,
        v.distributor_uuid ?? "",
        v.currency ?? "",
        v.price ?? "",
      ),
    onSuccess: async () => {
      await invalidate();
      setEditing(null);
      appToast.success(t("catalog.toast.saved"));
    },
    onError,
  });
  const remove = useMutation({
    mutationFn: (row: DistributorPrice) =>
      pricingService.deleteDistributorPrice(
        productUuid,
        row.distributor_uuid,
        row.currency,
      ),
    onSuccess: async () => {
      await invalidate();
      setDeleting(null);
      appToast.success(t("catalog.toast.deleted"));
    },
    onError,
  });

  if (!access.canReadDistributorPrices) return null;

  const row = editing?.row ?? null;
  const options = row
    ? [{ value: row.distributor_uuid, label: row.distributor_name }]
    : (distributors.data ?? []).map((d) => ({ value: d.uuid, label: d.name }));
  const items = list.data?.items ?? [];

  return (
    <Card data-testid="distributor-prices-card">
      <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-2">
        <div className="space-y-1">
          <CardTitle>{t("catalog.distributor_prices.title")}</CardTitle>
          <CardDescription>
            {t("catalog.distributor_prices.description")}
          </CardDescription>
        </div>
        {canWrite ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => setEditing({ row: null })}
          >
            <Plus className="size-4" />
            {t("catalog.distributor_prices.add")}
          </Button>
        ) : null}
      </CardHeader>
      <CardContent>
        {list.isLoading ? (
          <Loading />
        ) : list.isError ? (
          <ErrorState
            title={t("common.error_generic")}
            onRetry={() => void list.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : !items.length ? (
          <p className="text-muted-foreground text-sm">
            {t("catalog.distributor_prices.empty")}
          </p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-muted-foreground border-b">
                  <th className="py-2 pe-4 text-start font-medium">
                    {t("catalog.distributor_prices.distributor")}
                  </th>
                  <th className="py-2 pe-4 text-start font-medium">
                    {t("catalog.prices.currency")}
                  </th>
                  <th className="py-2 pe-4 text-end font-medium">
                    {t("catalog.distributor_prices.price")}
                  </th>
                  <th className="py-2 pe-4 text-start font-medium">
                    {t("catalog.distributor_prices.updated_at")}
                  </th>
                  {canWrite ? (
                    <th className="py-2">
                      <span className="sr-only">{t("common.actions")}</span>
                    </th>
                  ) : null}
                </tr>
              </thead>
              <tbody>
                {items.map((item) => (
                  <tr
                    key={`${item.distributor_uuid}-${item.currency}`}
                    className="border-b last:border-0"
                  >
                    <td className="py-2 pe-4">{item.distributor_name}</td>
                    <td className="py-2 pe-4 font-mono">{item.currency}</td>
                    <td className="py-2 pe-4 text-end tabular-nums">
                      {format.currency(priceNumber(item.price), item.currency)}
                    </td>
                    <td className="text-muted-foreground py-2 pe-4">
                      {format.dateTime(item.updated_at)}
                    </td>
                    {canWrite ? (
                      <td className="py-2 text-end whitespace-nowrap">
                        <Button
                          type="button"
                          size="icon"
                          variant="ghost"
                          className="size-8"
                          aria-label={t("common.edit")}
                          onClick={() => setEditing({ row: item })}
                        >
                          <Pencil className="size-4" />
                        </Button>
                        <Button
                          type="button"
                          size="icon"
                          variant="ghost"
                          className="text-destructive size-8"
                          aria-label={t("common.delete")}
                          onClick={() => setDeleting(item)}
                        >
                          <Trash2 className="size-4" />
                        </Button>
                      </td>
                    ) : null}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </CardContent>

      {canWrite ? (
        <>
          <PriceDialog
            open={Boolean(editing)}
            title={t("catalog.distributor_prices.edit_title")}
            fields={[
              {
                name: "price",
                labelKey: "catalog.distributor_prices.price",
                required: true,
              },
            ]}
            picker={{
              name: "distributor_uuid",
              labelKey: "catalog.distributor_prices.distributor",
              options,
              locked: Boolean(row),
            }}
            defaults={{
              distributor_uuid: row?.distributor_uuid ?? "",
              currency: row?.currency ?? "",
              price: row?.price ?? "",
            }}
            currencyLocked={Boolean(row)}
            pending={save.isPending}
            onOpenChange={(open) => {
              if (!open) setEditing(null);
            }}
            onSubmit={(values) => save.mutateAsync(values)}
          />
          <DeleteDialog
            open={Boolean(deleting)}
            title={t("catalog.distributor_prices.delete_title", {
              name: deleting?.distributor_name ?? "",
              currency: deleting?.currency ?? "",
            })}
            description={t("catalog.prices.delete_description")}
            isPending={remove.isPending}
            onConfirm={() => deleting && remove.mutate(deleting)}
            onCancel={() => setDeleting(null)}
          />
        </>
      ) : null}
    </Card>
  );
}
