"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus } from "lucide-react";
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
import { catalogKeys } from "@/features/catalog/hooks/use-catalog-access";
import type { PricingAccess } from "@/features/catalog/lib/access";
import {
  PriceDialog,
  type PriceDialogField,
  type PriceDialogValues,
} from "@/features/catalog/components/price-dialog";
import { PriceTable } from "@/features/catalog/components/price-table";
import { listPriceBody } from "@/features/catalog/lib/prices";
import { useDeviationThreshold } from "@/features/pricing/hooks/use-deviation-threshold";
import {
  pricingService,
  type EffectivePrice,
  type PriceViewer,
} from "@/features/catalog/services/pricing.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const DESCRIPTION_KEYS: Record<PriceViewer, string> = {
  center: "catalog.prices.center_description",
  distributor: "catalog.prices.distributor_description",
  dealer: "catalog.prices.dealer_description",
};

/** Edit target: an existing currency row, or a new currency (row null). */
type Editing = { row: EffectivePrice | null } | null;

/**
 * Product detail > Prices (TEC-147). Center edits its list price,
 * distributor its price to dealers, dealer only reads its purchase price.
 * Writes go through platformRequest, which opens the step-up dialog when the
 * API answers STEP_UP_REQUIRED and retries.
 */
export function ProductPricesCard({
  productUuid,
  access,
}: {
  productUuid: string;
  access: PricingAccess;
}) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<Editing>(null);
  const [deleting, setDeleting] = useState<EffectivePrice | null>(null);

  const view = useQuery({
    queryKey: catalogKeys.prices(productUuid),
    queryFn: () => pricingService.getProduct(productUuid),
    enabled: access.canView,
  });
  const viewer = view.data?.viewer;
  const showRecommended =
    access.canReadRecommended && Boolean(viewer) && viewer !== "center";
  const threshold = useDeviationThreshold(showRecommended);
  const editable =
    (viewer === "center" && access.canWriteList) ||
    (viewer === "distributor" && access.canWriteDealerPrice);

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: catalogKeys.prices(productUuid),
    });
  const onError = (error: unknown) =>
    appToast.error(
      isApiError(error) ? error.message : t("catalog.toast.failed"),
    );

  const save = useMutation({
    mutationFn: async (values: PriceDialogValues) => {
      const currency = values.currency ?? "";
      if (viewer === "distributor") {
        return pricingService.setDealerPrice(
          productUuid,
          currency,
          values.sale_price ?? "",
        );
      }
      const original = editing?.row ?? null;
      const body = listPriceBody(values, original, access.canWriteRecommended);
      return pricingService.setListPrice(productUuid, currency, body);
    },
    onSuccess: async () => {
      await invalidate();
      setEditing(null);
      appToast.success(t("catalog.toast.saved"));
    },
    onError,
  });

  const remove = useMutation({
    mutationFn: (row: EffectivePrice) =>
      viewer === "distributor"
        ? pricingService.deleteDealerPrice(productUuid, row.currency)
        : pricingService.deleteListPrice(productUuid, row.currency),
    onSuccess: async () => {
      await invalidate();
      setDeleting(null);
      appToast.success(t("catalog.toast.deleted"));
    },
    onError,
  });

  if (!access.canView) return null;

  const fields: PriceDialogField[] =
    viewer === "distributor"
      ? [
          {
            name: "sale_price",
            labelKey: "catalog.prices.distributor_sale",
            required: true,
          },
        ]
      : [
          {
            name: "purchase_price",
            labelKey: "catalog.prices.center_purchase",
          },
          {
            name: "sale_to_distributor_price",
            labelKey: "catalog.prices.center_sale",
          },
          ...(access.canWriteRecommended
            ? [
                {
                  name: "recommended_sale_price",
                  labelKey: "catalog.prices.recommended",
                },
              ]
            : []),
        ];

  const row = editing?.row ?? null;
  const defaults: PriceDialogValues =
    viewer === "distributor"
      ? {
          currency: row?.currency ?? view.data?.prices[0]?.currency ?? "",
          sale_price: row?.sale_price ?? "",
        }
      : {
          currency: row?.currency ?? "",
          purchase_price: row?.purchase_price ?? "",
          sale_to_distributor_price: row?.sale_price ?? "",
          recommended_sale_price: row?.recommended_sale_price ?? "",
        };

  return (
    <Card data-testid="product-prices-card">
      <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-2">
        <div className="space-y-1">
          <CardTitle>{t("catalog.prices.title")}</CardTitle>
          {viewer ? (
            <CardDescription>{t(DESCRIPTION_KEYS[viewer])}</CardDescription>
          ) : null}
        </div>
        {editable ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => setEditing({ row: null })}
          >
            <Plus className="size-4" />
            {viewer === "distributor"
              ? t("catalog.prices.set_dealer_price")
              : t("catalog.prices.add_currency")}
          </Button>
        ) : null}
      </CardHeader>
      <CardContent>
        {view.isLoading ? (
          <Loading />
        ) : view.isError || !view.data ? (
          <ErrorState
            title={t("common.error_generic")}
            onRetry={() => void view.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : (
          <PriceTable
            view={view.data}
            canEdit={editable}
            onEdit={(r) => setEditing({ row: r })}
            onDelete={setDeleting}
            canDelete={(r) =>
              viewer === "distributor" ? r.sale_price !== undefined : true
            }
            showRecommended={showRecommended}
            threshold={threshold}
          />
        )}
      </CardContent>

      {editable ? (
        <>
          <PriceDialog
            open={Boolean(editing)}
            title={
              viewer === "distributor"
                ? t("catalog.prices.set_dealer_price")
                : t("catalog.prices.edit_list_price")
            }
            fields={fields}
            defaults={defaults}
            currencyLocked={Boolean(row)}
            pending={save.isPending}
            onOpenChange={(open) => {
              if (!open) setEditing(null);
            }}
            onSubmit={(values) => save.mutateAsync(values)}
          />
          <DeleteDialog
            open={Boolean(deleting)}
            title={t("catalog.prices.delete_title", {
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
