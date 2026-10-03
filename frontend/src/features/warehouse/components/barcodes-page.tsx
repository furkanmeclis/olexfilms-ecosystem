"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Barcode, ChevronLeft, ChevronRight } from "lucide-react";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Permission } from "@/config/permissions";
import { GenerateForm } from "@/features/warehouse/components/generate-form";
import { LabelButton } from "@/features/warehouse/components/label-button";
import {
  useWarehouseAccess,
  WarehouseShell,
} from "@/features/warehouse/components/warehouse-shell";
import { pageCount } from "@/features/warehouse/lib/errors";
import {
  warehouseKeys,
  warehouseService,
  type BarcodeBatch,
} from "@/features/warehouse/services/warehouse.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const PAGE_SIZE = 20;

/**
 * Warehouse > Barcodes (TEC-202, center only, K14): reserve N barcodes of
 * a product (units in status printed, they enter stock through a stock
 * entry) and print the batch label sheet.
 */
export function BarcodesPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const base = useWarehouseAccess(slug, Permission.StockRead);
  const access = { ...base, allowed: base.allowed && base.isCenter };
  const canWrite = access.can(Permission.StockWrite);
  const qc = useQueryClient();
  const [page, setPage] = useState(0);
  const [last, setLast] = useState<BarcodeBatch | null>(null);

  const params = { limit: PAGE_SIZE, offset: page * PAGE_SIZE };
  const list = useQuery({
    queryKey: warehouseKeys.batches(params),
    queryFn: () => warehouseService.listBatches(params),
    enabled: access.allowed,
    placeholderData: keepPreviousData,
  });
  const rows = list.data?.items ?? [];
  const total = list.data?.total ?? 0;
  const pages = pageCount(total, PAGE_SIZE);

  const create = useMutation({
    mutationFn: warehouseService.createBatch,
    onSuccess: async (batch) => {
      setLast(batch);
      appToast.success(
        t("warehouse.barcodes.created", { count: batch.quantity }),
      );
      await qc.invalidateQueries({ queryKey: ["warehouse", "batches"] });
    },
  });

  return (
    <WarehouseShell
      slug={slug}
      access={access}
      title={t("warehouse.barcodes.title")}
      description={t("warehouse.barcodes.description")}
      icon={<Barcode className="size-6" />}
      forbiddenKey="warehouse.barcodes.forbidden"
    >
      {canWrite ? (
        <Card>
          <CardHeader>
            <CardTitle>{t("warehouse.barcodes.new_title")}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <GenerateForm
              submitLabel={t("warehouse.barcodes.generate")}
              pending={create.isPending}
              onSubmit={(body) => create.mutateAsync(body)}
            />
            {last ? (
              <div
                className="bg-muted/50 flex flex-wrap items-center justify-between gap-2 rounded-md p-3 text-sm"
                data-testid="batch-created"
              >
                <span>
                  {t("warehouse.barcodes.range", {
                    first: last.first_barcode,
                    last: last.last_barcode,
                  })}
                </span>
                <LabelButton
                  path={last.labels_url}
                  filename={`${last.first_barcode}.pdf`}
                />
              </div>
            ) : null}
          </CardContent>
        </Card>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle>{t("warehouse.barcodes.batches")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {list.isError ? (
            <ErrorState
              title={t("common.error_generic")}
              onRetry={() => void list.refetch()}
              retryLabel={t("common.retry")}
            />
          ) : list.isLoading ? (
            <p className="text-muted-foreground text-sm">
              {t("warehouse.list.loading")}
            </p>
          ) : rows.length === 0 ? (
            <p
              className="text-muted-foreground text-sm"
              data-testid="batches-empty"
            >
              {t("warehouse.barcodes.empty")}
            </p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm" data-testid="batches-table">
                <thead>
                  <tr className="text-muted-foreground border-b text-xs">
                    <th className="p-2 text-start font-medium">
                      {t("warehouse.fields.product")}
                    </th>
                    <th className="p-2 text-start font-medium">
                      {t("warehouse.barcodes.columns.range")}
                    </th>
                    <th className="p-2 text-start font-medium">
                      {t("warehouse.fields.quantity")}
                    </th>
                    <th className="p-2 text-start font-medium">
                      {t("warehouse.barcodes.columns.printed")}
                    </th>
                    <th className="p-2 text-start font-medium">
                      {t("warehouse.entries.columns.created")}
                    </th>
                    <th className="p-2" />
                  </tr>
                </thead>
                <tbody>
                  {rows.map((b) => (
                    <tr
                      key={b.uuid}
                      className="border-b last:border-0"
                      data-testid="batch-row"
                    >
                      <td className="p-2">
                        {b.product.name}
                        <div
                          className="text-muted-foreground font-mono text-xs"
                          dir="ltr"
                        >
                          {b.product.sku}
                        </div>
                      </td>
                      <td className="p-2 font-mono text-xs" dir="ltr">
                        {b.first_barcode} – {b.last_barcode}
                      </td>
                      <td className="p-2">
                        {format.number(b.quantity)}
                        {b.meters ? ` × ${b.meters} m` : ""}
                      </td>
                      <td className="p-2">{format.number(b.print_count)}</td>
                      <td className="text-muted-foreground p-2 text-xs whitespace-nowrap">
                        {format.dateTime(b.created_at)}
                      </td>
                      <td className="p-2 text-end">
                        <LabelButton
                          path={b.labels_url}
                          filename={`${b.first_barcode}.pdf`}
                          size="icon-sm"
                          variant="ghost"
                        />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <div
            className={cn(
              "flex flex-wrap items-center justify-between gap-2",
              rows.length === 0 && page === 0 && "hidden",
            )}
          >
            <p className="text-muted-foreground text-sm">
              {t("warehouse.list.page", { page: page + 1, pages, total })}
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
                {t("warehouse.list.prev")}
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={page + 1 >= pages || list.isFetching}
                onClick={() => setPage((p) => p + 1)}
              >
                {t("warehouse.list.next")}
                <ChevronRight className="size-4 rtl:rotate-180" />
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>
    </WarehouseShell>
  );
}
