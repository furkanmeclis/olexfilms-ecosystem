"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Ban } from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { Money } from "@/features/accounting/components/shared";
import { QuickSaleForm } from "@/features/dealer-sales/components/quick-sale-form";
import { useDealerSalesAccess } from "@/features/dealer-sales/hooks/use-dealer-sales-access";
import { canVoidSale } from "@/features/dealer-sales/lib/sales";
import {
  dealerSalesKeys,
  dealerSalesService,
  SALE_PAYMENT_METHODS,
  type ListQuery,
  type ProductSaleListItem,
} from "@/features/dealer-sales/services/dealer-sales.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const PRODUCT_SALES_PERSIST_KEY = "tenant-dealer-product-sales-v1";

/** Fallback display currency until a sale or a line names the dealer's. */
const DEFAULT_CURRENCY = "TRY";

/**
 * "Void" of a sale row: enabled only on the sale day for a sale that is
 * not voided yet (the backend refuses later voids, TEC-344).
 */
export function VoidSaleButton({
  sale,
  now,
  onVoid,
}: {
  sale: ProductSaleListItem;
  now?: Date;
  onVoid: (sale: ProductSaleListItem) => void;
}) {
  const { t } = useLocale();
  const enabled = canVoidSale(sale, now);
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      disabled={!enabled}
      title={
        enabled
          ? undefined
          : sale.voided
            ? t("dealer_sales.sales.already_voided")
            : t("dealer_sales.sales.void_window")
      }
      onClick={(event) => {
        event.stopPropagation();
        onVoid(sale);
      }}
      data-testid={`void-sale-${sale.uuid}`}
    >
      <Ban className="size-4" />
      {t("dealer_sales.sales.void")}
    </Button>
  );
}

function VoidSaleDialog({
  sale,
  pending,
  onCancel,
  onConfirm,
}: {
  sale: ProductSaleListItem | null;
  pending: boolean;
  onCancel: () => void;
  onConfirm: (reason: string) => void;
}) {
  const { t } = useLocale();
  const [reason, setReason] = useState("");
  return (
    <Dialog
      open={Boolean(sale)}
      onOpenChange={(open) => {
        if (!open) {
          setReason("");
          onCancel();
        }
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("dealer_sales.sales.void_title")}</DialogTitle>
          <DialogDescription>
            {t("dealer_sales.sales.void_description")}
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-1.5">
          <Label htmlFor="void-reason">{t("dealer_sales.fields.reason")}</Label>
          <Input
            id="void-reason"
            value={reason}
            maxLength={500}
            onChange={(e) => setReason(e.target.value)}
          />
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onCancel}>
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={pending}
            onClick={() => onConfirm(reason.trim())}
          >
            {t("dealer_sales.sales.void")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Tenant > Dealer sales > Quick sale (TEC-348): the scan cart on top and
 * the sale list below (server DataTable: sort, `q`, payment method facet,
 * sale date and total ranges, voided filter). A sale is voided only on its
 * sale day.
 */
export function QuickSalePage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const access = useDealerSalesAccess(slug);
  const queryClient = useQueryClient();
  const [voiding, setVoiding] = useState<ProductSaleListItem | null>(null);

  const columns = useMemo(
    () =>
      [
        createColumn<ProductSaleListItem>({
          accessorKey: "sold_at",
          labelKey: "dealer_sales.fields.sold_at",
          enableSorting: true,
          enableHiding: false,
          filterVariant: "date-range",
          param: "sold",
          gridPrimary: true,
          cell: ({ row }) => format.dateTime(row.original.sold_at),
        }),
        createColumn<ProductSaleListItem>({
          accessorKey: "products",
          labelKey: "dealer_sales.fields.products",
          enableSorting: false,
          enableColumnFilter: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="line-clamp-2">{row.original.products}</span>
          ),
        }),
        createColumn<ProductSaleListItem>({
          accessorKey: "customer_name",
          labelKey: "dealer_sales.fields.customer",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) => row.original.customer_name ?? "—",
        }),
        createColumn<ProductSaleListItem>({
          accessorKey: "payment_method",
          labelKey: "dealer_sales.fields.payment_method",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: SALE_PAYMENT_METHODS.map((value) => ({
            value,
            label: value,
            labelKey: `dealer_sales.payment_methods.${value}`,
          })),
          param: "payment_method",
          cell: ({ row }) =>
            t(`dealer_sales.payment_methods.${row.original.payment_method}`),
        }),
        createColumn<ProductSaleListItem>({
          accessorKey: "total",
          labelKey: "dealer_sales.fields.total",
          enableSorting: true,
          filterVariant: "number-range",
          param: "total",
          cell: ({ row }) => (
            <Money
              amount={row.original.total}
              currency={row.original.currency}
            />
          ),
        }),
        ...(access.canSeePurchasePrice
          ? [
              createColumn<ProductSaleListItem>({
                accessorKey: "profit",
                labelKey: "dealer_sales.fields.profit",
                enableSorting: false,
                enableColumnFilter: false,
                cell: ({ row }) =>
                  row.original.profit ? (
                    <Money
                      amount={row.original.profit}
                      currency={row.original.currency}
                    />
                  ) : (
                    "—"
                  ),
              }),
            ]
          : []),
        createColumn<ProductSaleListItem>({
          accessorKey: "voided",
          labelKey: "dealer_sales.fields.status",
          enableSorting: false,
          filterVariant: "boolean",
          param: "voided",
          cell: ({ row }) => (
            <StatusChip
              label={
                row.original.voided
                  ? t("dealer_sales.sales.status_voided")
                  : t("dealer_sales.sales.status_completed")
              }
              tone={row.original.voided ? "danger" : "success"}
            />
          ),
        }),
        createColumn<ProductSaleListItem>({
          accessorKey: "note",
          labelKey: "dealer_sales.fields.note",
          enableSorting: false,
          enableColumnFilter: false,
          defaultHidden: true,
        }),
        createColumn<ProductSaleListItem>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <VoidSaleButton sale={row.original} onVoid={setVoiding} />
          ),
        }),
      ] as ColumnDef<ProductSaleListItem, unknown>[],
    [access.canSeePurchasePrice, format, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "-sold_at",
    persistKey: PRODUCT_SALES_PERSIST_KEY,
  });
  const params: ListQuery = listState.params;

  const list = useQuery({
    queryKey: dealerSalesKeys.sales(access.orgUuid, params),
    queryFn: () => dealerSalesService.listSales(params),
    enabled: access.canSales && Boolean(access.orgUuid),
  });

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: dealerSalesKeys.all(access.orgUuid),
    });

  const voidSale = useMutation({
    mutationFn: (v: { uuid: string; reason: string }) =>
      dealerSalesService.voidSale(v.uuid, v.reason),
    onSuccess: async () => {
      setVoiding(null);
      await invalidate();
      appToast.success(t("dealer_sales.sales.voided"));
    },
    onError: (error) =>
      appToast.error(
        isApiError(error) && error.code === "VOID_WINDOW_EXPIRED"
          ? t("dealer_sales.sales.void_window")
          : isApiError(error)
            ? error.message
            : t("dealer_sales.toast.failed"),
      ),
  });

  return (
    <EntityPage
      title={t("dealer_sales.quick_sale.title")}
      description={t("dealer_sales.quick_sale.description")}
      permission={permissions.dealerSales.salesWrite}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("dealer_sales.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("dealer_sales.nav") },
        { label: t("dealer_sales.quick_sale.title") },
      ]}
    >
      <QuickSaleForm
        currency={list.data?.items[0]?.currency ?? DEFAULT_CURRENCY}
        canSeePurchasePrice={access.canSeePurchasePrice}
        onSaved={() => void invalidate()}
      />
      <h2 className="pt-4 text-lg font-semibold">
        {t("dealer_sales.sales.title")}
      </h2>
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("dealer_sales.sales.empty_title")}
        emptyDescription={t("dealer_sales.sales.empty_description")}
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: PRODUCT_SALES_PERSIST_KEY,
          rowSelection: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void list.refetch()}
            refreshDisabled={list.isFetching}
          />
        }
      />
      <VoidSaleDialog
        key={voiding?.uuid ?? "none"}
        sale={voiding}
        pending={voidSale.isPending}
        onCancel={() => setVoiding(null)}
        onConfirm={(reason) =>
          voiding && voidSale.mutate({ uuid: voiding.uuid, reason })
        }
      />
    </EntityPage>
  );
}
