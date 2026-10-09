"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { useMemo, useState, type FormEvent } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityCreateButton,
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
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  FieldError,
  Money,
  FormSelect,
} from "@/features/accounting/components/shared";
import { useDealerSalesAccess } from "@/features/dealer-sales/hooks/use-dealer-sales-access";
import { normalizeMoney, toCents } from "@/features/dealer-sales/lib/sales";
import {
  dealerSalesKeys,
  dealerSalesService,
  PURCHASE_PAYMENT_METHODS,
  type ListQuery,
  type PurchaseListItem,
  type PurchasePaymentMethod,
  type PurchaseRequest,
  type Supplier,
} from "@/features/dealer-sales/services/dealer-sales.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const PURCHASES_PERSIST_KEY = "tenant-dealer-purchases-v1";

type PurchaseFormValues = {
  supplier_uuid: string;
  amount: string;
  payment_method: PurchasePaymentMethod;
  description: string;
  note: string;
};

function PurchaseForm({
  suppliers,
  pending,
  onCancel,
  onSubmit,
}: {
  suppliers: Supplier[];
  pending?: boolean;
  onCancel: () => void;
  onSubmit: (body: PurchaseRequest) => Promise<unknown>;
}) {
  const { t } = useLocale();
  const [values, setValues] = useState<PurchaseFormValues>({
    supplier_uuid: "",
    amount: "",
    payment_method: "cash",
    description: "",
    note: "",
  });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const set = <K extends keyof PurchaseFormValues>(
    key: K,
    value: PurchaseFormValues[K],
  ) => setValues((v) => ({ ...v, [key]: value }));

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const next: Record<string, string> = {};
    if (!values.supplier_uuid) {
      next.supplier_uuid = t("dealer_sales.validation.supplier");
    }
    const amount = normalizeMoney(values.amount);
    if (amount === null || (toCents(amount) ?? 0) <= 0) {
      next.amount = t("dealer_sales.validation.amount");
    }
    setErrors(next);
    if (Object.keys(next).length > 0 || amount === null) return;
    try {
      await onSubmit({
        supplier_uuid: values.supplier_uuid,
        amount,
        payment_method: values.payment_method,
        description: values.description.trim() || undefined,
        note: values.note.trim() || undefined,
      });
    } catch (error) {
      if (isApiError(error) && error.isValidation) {
        setErrors(error.fieldErrors());
      }
    }
  };

  return (
    <form
      onSubmit={submit}
      noValidate
      className="space-y-4"
      data-testid="purchase-form"
    >
      <FormSelect
        id="purchase-supplier"
        name="supplier_uuid"
        label={t("dealer_sales.fields.supplier")}
        value={values.supplier_uuid}
        placeholder={t("dealer_sales.purchases.pick_supplier")}
        error={errors.supplier_uuid}
        onChange={(v) => set("supplier_uuid", v)}
        options={suppliers.map((s) => ({ value: s.uuid, label: s.name }))}
      />
      {suppliers.length === 0 ? (
        <p className="text-muted-foreground text-xs">
          {t("dealer_sales.purchases.no_suppliers")}
        </p>
      ) : null}
      <div className="grid gap-1.5">
        <Label htmlFor="purchase-amount">
          {t("dealer_sales.fields.amount")}
        </Label>
        <Input
          id="purchase-amount"
          name="amount"
          inputMode="decimal"
          dir="ltr"
          value={values.amount}
          placeholder="0.00"
          aria-invalid={errors.amount ? true : undefined}
          aria-describedby={errors.amount ? "purchase-amount-error" : undefined}
          onChange={(e) => set("amount", e.target.value)}
        />
        <FieldError id="purchase-amount-error" message={errors.amount} />
      </div>
      <FormSelect
        id="purchase-payment"
        name="payment_method"
        label={t("dealer_sales.fields.payment_method")}
        value={values.payment_method}
        onChange={(v) => set("payment_method", v as PurchasePaymentMethod)}
        options={PURCHASE_PAYMENT_METHODS.map((m) => ({
          value: m,
          label: t(`dealer_sales.payment_methods.${m}`),
        }))}
      />
      <div className="grid gap-1.5">
        <Label htmlFor="purchase-description">
          {t("dealer_sales.fields.description")}
        </Label>
        <Input
          id="purchase-description"
          name="description"
          value={values.description}
          maxLength={1000}
          onChange={(e) => set("description", e.target.value)}
        />
      </div>
      <div className="grid gap-1.5">
        <Label htmlFor="purchase-note">{t("dealer_sales.fields.note")}</Label>
        <Textarea
          id="purchase-note"
          value={values.note}
          maxLength={5000}
          rows={2}
          onChange={(e) => set("note", e.target.value)}
        />
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCancel}>
          {t("common.cancel")}
        </Button>
        <Button
          type="submit"
          disabled={pending}
          data-testid="purchase-form-submit"
        >
          {t("common.save")}
        </Button>
      </DialogFooter>
    </form>
  );
}

/**
 * Tenant > Dealer sales > Purchases (TEC-348): external purchases (an
 * expense on the dealer's book, no stock movement, K12). Server DataTable:
 * sort (date, amount, supplier, payment method), `q`, supplier and
 * payment method facets, date and amount ranges; a new purchase in a
 * dialog.
 */
export function PurchasesPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const access = useDealerSalesAccess(slug);
  const queryClient = useQueryClient();
  const [formOpen, setFormOpen] = useState(false);

  const suppliers = useQuery({
    queryKey: dealerSalesKeys.supplierOptions(access.orgUuid),
    queryFn: () =>
      dealerSalesService.listSuppliers({
        active: true,
        limit: 100,
        sort: "name",
      }),
    enabled:
      access.canPurchases && access.canSuppliers && Boolean(access.orgUuid),
  });
  const supplierItems = useMemo(
    () => suppliers.data?.items ?? [],
    [suppliers.data],
  );

  const columns = useMemo(
    () =>
      [
        createColumn<PurchaseListItem>({
          accessorKey: "purchased_on",
          labelKey: "dealer_sales.fields.purchased_on",
          enableSorting: true,
          enableHiding: false,
          filterVariant: "date-range",
          param: "purchased",
          gridPrimary: true,
          cell: ({ row }) => format.date(row.original.purchased_on),
        }),
        createColumn<PurchaseListItem>({
          accessorKey: "supplier_name",
          labelKey: "dealer_sales.fields.supplier",
          enableSorting: true,
          gridSecondary: true,
          ...(supplierItems.length > 0
            ? {
                filterVariant: "faceted" as const,
                filterOptions: supplierItems.map((s) => ({
                  value: s.uuid,
                  label: s.name,
                })),
                param: "supplier_uuid",
              }
            : { enableColumnFilter: false }),
          cell: ({ row }) => (
            <span className="font-medium">{row.original.supplier_name}</span>
          ),
        }),
        createColumn<PurchaseListItem>({
          accessorKey: "description",
          labelKey: "dealer_sales.fields.description",
          enableSorting: false,
          enableColumnFilter: false,
        }),
        createColumn<PurchaseListItem>({
          accessorKey: "amount",
          labelKey: "dealer_sales.fields.amount",
          enableSorting: true,
          filterVariant: "number-range",
          param: "amount",
          cell: ({ row }) => (
            <Money
              amount={row.original.amount}
              currency={row.original.currency}
            />
          ),
        }),
        createColumn<PurchaseListItem>({
          accessorKey: "payment_method",
          labelKey: "dealer_sales.fields.payment_method",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: PURCHASE_PAYMENT_METHODS.map((value) => ({
            value,
            label: value,
            labelKey: `dealer_sales.payment_methods.${value}`,
          })),
          param: "payment_method",
          cell: ({ row }) =>
            t(`dealer_sales.payment_methods.${row.original.payment_method}`),
        }),
        createColumn<PurchaseListItem>({
          accessorKey: "note",
          labelKey: "dealer_sales.fields.note",
          enableSorting: false,
          enableColumnFilter: false,
          defaultHidden: true,
        }),
        createColumn<PurchaseListItem>({
          accessorKey: "created_at",
          labelKey: "dealer_sales.fields.created_at",
          enableSorting: false,
          enableColumnFilter: false,
          defaultHidden: true,
          cell: ({ row }) => format.dateTime(row.original.created_at),
        }),
      ] as ColumnDef<PurchaseListItem, unknown>[],
    [format, supplierItems, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "-purchased_on",
    persistKey: PURCHASES_PERSIST_KEY,
  });
  const params: ListQuery = listState.params;

  const list = useQuery({
    queryKey: dealerSalesKeys.purchases(access.orgUuid, params),
    queryFn: () => dealerSalesService.listPurchases(params),
    enabled: access.canPurchases && Boolean(access.orgUuid),
  });

  const create = useMutation({
    mutationFn: (body: PurchaseRequest) =>
      dealerSalesService.createPurchase(body),
    onSuccess: async () => {
      setFormOpen(false);
      await queryClient.invalidateQueries({
        queryKey: dealerSalesKeys.all(access.orgUuid),
      });
      appToast.success(t("dealer_sales.purchases.saved"));
    },
    onError: (error) => {
      if (isApiError(error) && error.isValidation) return;
      appToast.error(
        isApiError(error) ? error.message : t("dealer_sales.toast.failed"),
      );
    },
  });

  return (
    <EntityPage
      title={t("dealer_sales.purchases.title")}
      description={t("dealer_sales.purchases.description")}
      permission={permissions.dealerSales.purchasesWrite}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("dealer_sales.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("dealer_sales.nav") },
        { label: t("dealer_sales.purchases.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => setFormOpen(true)}
          label={t("dealer_sales.purchases.create")}
          permission={permissions.dealerSales.purchasesWrite}
        />
      }
    >
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("dealer_sales.purchases.empty_title")}
        emptyDescription={t("dealer_sales.purchases.empty_description")}
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: PURCHASES_PERSIST_KEY,
          rowSelection: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void list.refetch()}
            refreshDisabled={list.isFetching}
          />
        }
      />
      <Dialog open={formOpen} onOpenChange={setFormOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>
              {t("dealer_sales.purchases.create_title")}
            </DialogTitle>
            <DialogDescription>
              {t("dealer_sales.purchases.form_description")}
            </DialogDescription>
          </DialogHeader>
          {formOpen ? (
            <PurchaseForm
              suppliers={supplierItems}
              pending={create.isPending}
              onCancel={() => setFormOpen(false)}
              onSubmit={(body) => create.mutateAsync(body)}
            />
          ) : null}
        </DialogContent>
      </Dialog>
    </EntityPage>
  );
}
