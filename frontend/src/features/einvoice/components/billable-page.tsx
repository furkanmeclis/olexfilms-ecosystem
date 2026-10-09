"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { FilePlus } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo } from "react";

import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  BulkActionMenu,
  SelectionBanner,
  useBulkSelection,
  type BulkActionDef,
} from "@/features/bulk-engine";
import {
  EinvoiceShell,
  useEinvoiceAccess,
} from "@/features/einvoice/components/einvoice-shell";
import {
  EINVOICE_SOURCE_TYPES,
  amount,
} from "@/features/einvoice/lib/einvoice";
import {
  einvoiceKeys,
  einvoiceService,
  type EinvoiceBillable,
  type ListQuery,
} from "@/features/einvoice/services/einvoice.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const BILLABLE_PERSIST_KEY = "tenant-einvoice-billable-v1";

/**
 * `POST /v1/einvoices/billable/bulk` (TEC-504): one draft per selected
 * source, the single draft rules per item (a source that cannot be
 * drafted fails alone). Not reversible: a draft is released by voiding it.
 */
export const BILLABLE_BULK_ACTIONS: BulkActionDef[] = [
  {
    id: "create_draft",
    label_key: "bulk.actions.einvoices.create_draft",
    permission: permissions.einvoice.manage,
    confirm_key: "bulk.confirm.einvoices.create_draft",
    icon: FilePlus,
  },
];

function BillableTable({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const router = useRouter();
  const qc = useQueryClient();
  const { canManage } = useEinvoiceAccess(slug);

  const draft = useMutation({
    mutationFn: (row: EinvoiceBillable) =>
      einvoiceService.createDraft(row.source_type, row.source_uuid),
    onSuccess: (invoice) => {
      void qc.invalidateQueries({ queryKey: einvoiceKeys.all });
      appToast.success(t("einvoice.billable.draft_created"));
      router.push(routes.tenant.einvoices.detail(slug, invoice.uuid));
    },
    onError: (err) => {
      // The API error toast names the code; the missing buyer fields are
      // listed on top.
      if (
        isApiError(err) &&
        err.code === "EINVOICE_BUYER_PROFILE_INCOMPLETE" &&
        err.details.length > 0
      ) {
        appToast.warning(
          t("einvoice.billable.missing_fields", {
            fields: err.details
              .map((d) => t(`einvoice.profile.fields.${d.field ?? ""}`))
              .join(", "),
          }),
        );
      }
    },
  });

  const columns = useMemo(() => {
    const base = [
      createColumn<EinvoiceBillable>({
        id: "source_type",
        accessorKey: "source_type",
        labelKey: "einvoice.billable.columns.source_type",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: EINVOICE_SOURCE_TYPES.map((value) => ({
          value,
          label: value,
          labelKey: `einvoice.source_types.${value}`,
        })),
        param: "source_type",
        cell: ({ row }) =>
          t(`einvoice.source_types.${row.original.source_type}`),
      }),
      createColumn<EinvoiceBillable>({
        id: "source_no",
        accessorKey: "source_no",
        labelKey: "einvoice.billable.columns.source_no",
        enableSorting: true,
        enableHiding: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <div className="min-w-0">
            <div className="font-medium" dir="auto">
              {row.original.source_no}
            </div>
            {row.original.period_start ? (
              <div className="text-muted-foreground text-xs">
                {format.date(row.original.period_start)} –{" "}
                {format.date(row.original.period_end ?? "")}
              </div>
            ) : null}
          </div>
        ),
      }),
      createColumn<EinvoiceBillable>({
        id: "buyer_name",
        accessorFn: (row) => row.buyer_organization.name,
        labelKey: "einvoice.billable.columns.buyer",
        enableSorting: true,
        gridSecondary: true,
        cell: ({ row }) => row.original.buyer_organization.name,
      }),
      createColumn<EinvoiceBillable>({
        id: "billable_at",
        accessorKey: "billable_at",
        labelKey: "einvoice.billable.columns.billable_at",
        enableSorting: true,
        filterVariant: "date-range",
        param: "billable_at",
        cell: ({ row }) => (
          <span className="whitespace-nowrap">
            {row.original.billable_at
              ? format.date(row.original.billable_at)
              : "—"}
          </span>
        ),
      }),
      createColumn<EinvoiceBillable>({
        id: "tax_total",
        accessorKey: "tax_total",
        labelKey: "einvoice.columns.tax_total",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => (
          <span className="whitespace-nowrap tabular-nums" dir="ltr">
            {format.currency(
              amount(row.original.tax_total),
              row.original.currency,
            )}
          </span>
        ),
      }),
      createColumn<EinvoiceBillable>({
        id: "payable",
        accessorKey: "payable",
        labelKey: "einvoice.columns.payable",
        enableSorting: true,
        filterVariant: "number-range",
        param: "payable",
        cell: ({ row }) => (
          <span
            className="font-medium whitespace-nowrap tabular-nums"
            dir="ltr"
          >
            {format.currency(
              amount(row.original.payable),
              row.original.currency,
            )}
          </span>
        ),
      }),
      createColumn<EinvoiceBillable>({
        id: "actions",
        labelKey: "common.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => {
          const items: EntityRowAction[] = [
            {
              id: "draft",
              label: t("einvoice.billable.create_draft"),
              icon: FilePlus,
              permission: permissions.einvoice.manage,
              disabled: draft.isPending,
              onSelect: () => draft.mutate(row.original),
            },
          ];
          return <EntityRowActions actions={items} />;
        },
      }),
    ] as ColumnDef<EinvoiceBillable, unknown>[];
    return canManage
      ? [createSelectColumnDef<EinvoiceBillable>(), ...base]
      : base;
  }, [canManage, draft, format, t]);

  // Column meta drives the params: source_type (CSV), billable_at_from /
  // _to, payable_min / _max; sort is one backend field.
  const listState = useServerListState({
    columns,
    initialSort: "-billable_at",
    initialPageSize: 20,
    persistKey: BILLABLE_PERSIST_KEY,
  });
  const params: ListQuery = listState.params;
  const list = useQuery({
    queryKey: einvoiceKeys.billable(params),
    queryFn: () => einvoiceService.billable(params),
  });
  const total = list.data?.total ?? 0;

  // "Select all matching" runs on the list filters, search and sort.
  const bulkQuery = useMemo(
    () => ({ ...listState.filterParams, q: params.q, sort: params.sort }),
    [listState.filterParams, params.q, params.sort],
  );
  const bulkSelection = useBulkSelection({
    listQueryKey: params,
    bulkQuery,
    total,
  });
  const filtered =
    listState.tableState.columnFilters.length > 0 || Boolean(params.q);

  return (
    <>
      <SelectionBanner
        selectedCount={bulkSelection.selectedCount}
        total={total}
        showSelectAll={bulkSelection.showSelectAllBanner}
        allMatchingSelected={bulkSelection.scope.mode === "all"}
        onSelectAllMatching={bulkSelection.selectAllMatching}
        onClearSelection={bulkSelection.clearSelection}
      />
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.source_uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("einvoice.billable.empty_title")}
        emptyDescription={
          filtered
            ? t("einvoice.billable.empty_filtered")
            : t("einvoice.billable.empty_description")
        }
        rowCount={total}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{ persistKey: BILLABLE_PERSIST_KEY, rowSelection: canManage }}
        renderGridItem={(row) => (
          <div className="space-y-1" data-testid="billable-card">
            <div className="flex items-start justify-between gap-2">
              <p className="font-medium" dir="auto">
                {row.source_no}
              </p>
              <span className="font-medium tabular-nums" dir="ltr">
                {format.currency(amount(row.payable), row.currency)}
              </span>
            </div>
            <p className="text-muted-foreground text-xs">
              {t(`einvoice.source_types.${row.source_type}`)} ·{" "}
              {row.buyer_organization.name}
            </p>
          </div>
        )}
        toolbarExtra={
          <>
            <BulkActionMenu
              resource="einvoices.billable"
              actions={BILLABLE_BULK_ACTIONS}
              scope={bulkSelection.scope}
              selectedCount={bulkSelection.selectedCount}
              onComplete={() => {
                bulkSelection.clearSelection();
                void qc.invalidateQueries({ queryKey: einvoiceKeys.all });
              }}
            />
            <EntityToolbar
              onRefresh={() => void list.refetch()}
              refreshDisabled={list.isFetching}
            />
          </>
        }
      />
    </>
  );
}

/**
 * Muhasebe > e-Fatura > Faturalanabilir kayıtlar (TEC-504): received
 * orders to distributors and posted service catalog periods without an
 * active invoice, over GET /v1/einvoices/billable (source type facet,
 * billable date and amount ranges, `q`), a draft per row and the bulk
 * "create draft".
 */
export function BillablePage({ slug }: { slug: string }) {
  const { t } = useLocale();
  return (
    <EinvoiceShell
      slug={slug}
      section="billable"
      title={t("einvoice.billable.title")}
      description={t("einvoice.billable.description")}
    >
      <BillableTable slug={slug} />
    </EinvoiceShell>
  );
}
