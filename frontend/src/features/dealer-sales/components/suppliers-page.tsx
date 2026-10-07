"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Pencil } from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityCreateButton,
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { SupplierFormDialog } from "@/features/dealer-sales/components/supplier-form-dialog";
import { useDealerSalesAccess } from "@/features/dealer-sales/hooks/use-dealer-sales-access";
import {
  dealerSalesKeys,
  dealerSalesService,
  type ListQuery,
  type Supplier,
  type SupplierRequest,
} from "@/features/dealer-sales/services/dealer-sales.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const SUPPLIERS_PERSIST_KEY = "tenant-dealer-suppliers-v1";

const INITIAL_FILTERS = [{ id: "active", value: true }];

/**
 * Tenant > Dealer sales > Suppliers (TEC-348): the dealer's external
 * suppliers. Server DataTable: sort (name, created, updated), `q` (name,
 * tax no, phone, email), active and created filters; create and edit in a
 * dialog (phone stored as E.164).
 */
export function SuppliersPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const access = useDealerSalesAccess(slug);
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<Supplier | null>(null);
  const [formOpen, setFormOpen] = useState(false);

  const openForm = (supplier: Supplier | null) => {
    setEditing(supplier);
    setFormOpen(true);
  };

  const columns = useMemo(
    () =>
      [
        createColumn<Supplier>({
          accessorKey: "name",
          labelKey: "dealer_sales.fields.name",
          enableSorting: true,
          enableHiding: false,
          enableColumnFilter: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.name}</span>
          ),
        }),
        createColumn<Supplier>({
          accessorKey: "tax_no",
          labelKey: "dealer_sales.fields.tax_no",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <span dir="ltr">{row.original.tax_no ?? "—"}</span>
          ),
        }),
        createColumn<Supplier>({
          accessorKey: "phone_e164",
          labelKey: "dealer_sales.fields.phone",
          enableSorting: false,
          enableColumnFilter: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span dir="ltr">{row.original.phone_e164 ?? "—"}</span>
          ),
        }),
        createColumn<Supplier>({
          accessorKey: "email",
          labelKey: "dealer_sales.fields.email",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) => <span dir="ltr">{row.original.email ?? "—"}</span>,
        }),
        createColumn<Supplier>({
          accessorKey: "active",
          labelKey: "dealer_sales.fields.status",
          enableSorting: false,
          filterVariant: "boolean",
          param: "active",
          cell: ({ row }) => (
            <StatusChip
              label={
                row.original.active
                  ? t("dealer_sales.status.active")
                  : t("dealer_sales.status.inactive")
              }
              tone={row.original.active ? "success" : "default"}
            />
          ),
        }),
        createColumn<Supplier>({
          accessorKey: "created_at",
          labelKey: "dealer_sales.fields.created_at",
          enableSorting: true,
          filterVariant: "date-range",
          param: "created",
          defaultHidden: true,
          cell: ({ row }) => format.dateTime(row.original.created_at),
        }),
        createColumn<Supplier>({
          accessorKey: "updated_at",
          labelKey: "dealer_sales.fields.updated_at",
          enableSorting: true,
          enableColumnFilter: false,
          defaultHidden: true,
          cell: ({ row }) => format.dateTime(row.original.updated_at),
        }),
        createColumn<Supplier>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "edit",
                  label: t("common.edit"),
                  icon: Pencil,
                  onSelect: () => openForm(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<Supplier, unknown>[],
    [format, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "name",
    persistKey: SUPPLIERS_PERSIST_KEY,
    initialColumnFilters: INITIAL_FILTERS,
  });
  const params: ListQuery = listState.params;

  const list = useQuery({
    queryKey: dealerSalesKeys.suppliers(access.orgUuid, params),
    queryFn: () => dealerSalesService.listSuppliers(params),
    enabled: access.canSuppliers && Boolean(access.orgUuid),
  });

  const save = useMutation({
    mutationFn: (body: SupplierRequest) =>
      editing
        ? dealerSalesService.updateSupplier(editing.uuid, body)
        : dealerSalesService.createSupplier(body),
    onSuccess: async () => {
      setFormOpen(false);
      await queryClient.invalidateQueries({
        queryKey: dealerSalesKeys.all(access.orgUuid),
      });
      appToast.success(t("dealer_sales.suppliers.saved"));
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
      title={t("dealer_sales.suppliers.title")}
      description={t("dealer_sales.suppliers.description")}
      permission={permissions.dealerSales.suppliersManage}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("dealer_sales.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("dealer_sales.nav") },
        { label: t("dealer_sales.suppliers.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => openForm(null)}
          label={t("dealer_sales.suppliers.create")}
          permission={permissions.dealerSales.suppliersManage}
        />
      }
    >
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) => openForm(row)}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("dealer_sales.suppliers.empty_title")}
        emptyDescription={t("dealer_sales.suppliers.empty_description")}
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: SUPPLIERS_PERSIST_KEY,
          rowSelection: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void list.refetch()}
            refreshDisabled={list.isFetching}
          />
        }
      />
      <SupplierFormDialog
        open={formOpen}
        supplier={editing}
        pending={save.isPending}
        onOpenChange={setFormOpen}
        onSubmit={(body) => save.mutateAsync(body)}
      />
    </EntityPage>
  );
}
