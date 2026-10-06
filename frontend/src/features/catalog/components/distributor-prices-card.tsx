"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";

import { DeleteDialog } from "@/components/dialogs/delete-dialog";
import {
  EntityRowActions,
  EntityTable,
  useServerListState,
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
import { PriceDialog } from "@/features/catalog/components/price-dialog";
import { catalogKeys } from "@/features/catalog/hooks/use-catalog-access";
import type { PricingAccess } from "@/features/catalog/lib/access";
import { priceNumber } from "@/features/catalog/lib/prices";
import {
  pricingService,
  type DistributorPrice,
  type DistributorPriceListParams,
} from "@/features/catalog/services/pricing.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type Editing = { row: DistributorPrice | null } | null;

export const DISTRIBUTOR_PRICES_PERSIST_KEY =
  "tenant-catalog-distributor-prices-v1";

/** Currency filter text → `currency` CSV (ISO-4217, upper case). */
function currencyParam(value: unknown) {
  const codes = String(value ?? "")
    .split(/[\s,]+/)
    .map((code) => code.trim().toUpperCase())
    .filter(Boolean);
  return { currency: codes.length ? codes.join(",") : undefined };
}

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

  const columns = useMemo<ColumnDef<DistributorPrice, unknown>[]>(() => {
    const cols = [
      createColumn<DistributorPrice>({
        id: "distributor",
        accessorKey: "distributor_name",
        labelKey: "catalog.distributor_prices.distributor",
        enableSorting: true,
        gridPrimary: true,
      }),
      createColumn<DistributorPrice>({
        accessorKey: "currency",
        labelKey: "catalog.prices.currency",
        enableSorting: true,
        filterVariant: "text",
        param: "currency",
        paramFormat: currencyParam,
        cell: ({ row }) => (
          <span className="font-mono">{row.original.currency}</span>
        ),
      }),
      createColumn<DistributorPrice>({
        accessorKey: "price",
        labelKey: "catalog.distributor_prices.price",
        enableSorting: true,
        gridSecondary: true,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {format.currency(
              priceNumber(row.original.price),
              row.original.currency,
            )}
          </span>
        ),
      }),
      createColumn<DistributorPrice>({
        accessorKey: "updated_at",
        labelKey: "catalog.distributor_prices.updated_at",
        enableSorting: true,
        cell: ({ row }) => (
          <span className="text-muted-foreground">
            {format.dateTime(row.original.updated_at)}
          </span>
        ),
      }),
    ] as ColumnDef<DistributorPrice, unknown>[];
    if (canWrite) {
      cols.push(
        createColumn<DistributorPrice>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "edit",
                  label: t("common.edit"),
                  icon: Pencil,
                  onSelect: () => setEditing({ row: row.original }),
                },
                {
                  id: "delete",
                  label: t("common.delete"),
                  icon: Trash2,
                  variant: "destructive",
                  onSelect: () => setDeleting(row.original),
                },
              ]}
            />
          ),
        }) as ColumnDef<DistributorPrice, unknown>,
      );
    }
    return cols;
  }, [canWrite, format, t]);

  // Nested server table (TEC-369): sort, q (SKU / product / distributor),
  // currency CSV, paging.
  const listState = useServerListState({
    columns,
    initialSort: "distributor",
    persistKey: DISTRIBUTOR_PRICES_PERSIST_KEY,
  });
  const listParams: DistributorPriceListParams = listState.params;
  const list = useQuery({
    queryKey: catalogKeys.distributorPrices(productUuid, listParams),
    queryFn: () =>
      pricingService.listDistributorPrices(productUuid, listParams),
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
        <EntityTable
          columns={columns}
          data={list.data?.items ?? []}
          getRowId={(row) => `${row.distributor_uuid}-${row.currency}`}
          isLoading={list.isLoading}
          isError={list.isError}
          onRetry={() => void list.refetch()}
          emptyTitle={t("catalog.distributor_prices.empty")}
          emptyDescription=""
          rowCount={list.data?.total ?? 0}
          state={listState.tableState}
          features={{
            persistKey: DISTRIBUTOR_PRICES_PERSIST_KEY,
            rowSelection: false,
          }}
        />
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
