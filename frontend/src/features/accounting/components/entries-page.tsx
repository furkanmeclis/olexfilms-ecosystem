"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { ArrowDownLeft, ArrowUpRight } from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { EntryFilters } from "@/features/accounting/components/entry-filters";
import { SettlementDialog } from "@/features/accounting/components/settlement-dialog";
import {
  EntryAmount,
  EntryStatus,
  useCategoryLabels,
} from "@/features/accounting/components/shared";
import {
  accountingKeys,
  useAccountingAccess,
} from "@/features/accounting/hooks/use-accounting-access";
import {
  EMPTY_ENTRY_FILTERS,
  entryFilterParams,
  type EntryFilterValues,
} from "@/features/accounting/lib/form";
import {
  accountingService,
  type FinanceEntry,
  type SettlementKind,
} from "@/features/accounting/services/accounting.service";
import { useLocale } from "@/providers/locale-provider";

/** Columns shared by the ledger list and the cari detail's recent rows. */
export function useEntryColumns(categories: Map<string, string>) {
  const { t, format } = useLocale();
  return useMemo(
    () =>
      [
        createColumn<FinanceEntry>({
          accessorKey: "created_at",
          labelKey: "accounting.fields.date",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {format.dateTime(row.original.created_at)}
            </span>
          ),
        }),
        createColumn<FinanceEntry>({
          accessorKey: "direction",
          labelKey: "accounting.fields.direction",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) =>
            t(`accounting.directions.${row.original.direction}`),
        }),
        createColumn<FinanceEntry>({
          accessorKey: "category",
          labelKey: "accounting.fields.category",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) =>
            categories.get(row.original.category) ?? row.original.category,
        }),
        createColumn<FinanceEntry>({
          id: "counterparty",
          accessorFn: (row) => row.counterparty_organization?.name ?? "",
          labelKey: "accounting.fields.cari",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.counterparty_organization?.name ?? "—",
        }),
        createColumn<FinanceEntry>({
          id: "account",
          accessorFn: (row) => row.account?.name ?? "",
          labelKey: "accounting.fields.account",
          enableSorting: false,
          cell: ({ row }) => row.original.account?.name ?? "—",
        }),
        createColumn<FinanceEntry>({
          accessorKey: "source_type",
          labelKey: "accounting.fields.source",
          enableSorting: false,
          cell: ({ row }) => {
            const s = row.original.source_type;
            if (!s) return "—";
            return (
              ["manual", "order", "service", "transfer"] as string[]
            ).includes(s)
              ? t(`accounting.sources.${s}`)
              : s;
          },
        }),
        createColumn<FinanceEntry>({
          accessorKey: "amount",
          labelKey: "accounting.fields.amount",
          enableSorting: false,
          cell: ({ row }) => <EntryAmount entry={row.original} />,
        }),
        createColumn<FinanceEntry>({
          id: "status",
          accessorFn: (row) => (row.reversal_of_uuid ? 2 : row.voided ? 1 : 0),
          labelKey: "accounting.fields.status",
          enableSorting: false,
          cell: ({ row }) => <EntryStatus entry={row.original} />,
        }),
        createColumn<FinanceEntry>({
          accessorKey: "description",
          labelKey: "accounting.fields.description",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.description ?? "—",
        }),
      ] as ColumnDef<FinanceEntry, unknown>[],
    [categories, format, t],
  );
}

/** Tenant > Accounting > Entries: filters, pagination, reversal markers. */
export function EntriesPage({
  slug,
  initialCari = "",
}: {
  slug: string;
  initialCari?: string;
}) {
  const { t } = useLocale();
  const access = useAccountingAccess(slug);
  const listState = useServerListState({ initialSort: "-created_at" });
  const [filters, setFilters] = useState<EntryFilterValues>({
    ...EMPTY_ENTRY_FILTERS,
    cari_uuid: initialCari,
  });
  const [settle, setSettle] = useState<SettlementKind | null>(null);
  const enabled = access.canRead && Boolean(access.orgUuid);
  const categories = useCategoryLabels(access.orgUuid, enabled);
  const columns = useEntryColumns(categories);

  const params = useMemo(
    () =>
      entryFilterParams(filters, {
        limit: listState.params.limit,
        offset: listState.params.offset,
      }),
    [filters, listState.params.limit, listState.params.offset],
  );

  const list = useQuery({
    queryKey: accountingKeys.entries(access.orgUuid, params),
    queryFn: () => accountingService.listEntries(params),
    enabled,
  });
  const cari = useQuery({
    queryKey: accountingKeys.cariList(access.orgUuid, { limit: 100 }),
    queryFn: () => accountingService.listCari({ limit: 100 }),
    enabled,
  });

  const total = list.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );

  return (
    <EntityPage
      title={t("accounting.entries.title")}
      description={t("accounting.entries.description")}
      permission={permissions.accounting.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("accounting.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("accounting.nav") },
        { label: t("accounting.entries.title") },
      ]}
      actions={
        access.canWrite ? (
          <div className="flex flex-wrap gap-2">
            <Button onClick={() => setSettle("collection")}>
              <ArrowDownLeft className="size-4 rtl:-scale-x-100" />
              {t("accounting.settlement.new_collection")}
            </Button>
            <Button variant="outline" onClick={() => setSettle("payment")}>
              <ArrowUpRight className="size-4 rtl:-scale-x-100" />
              {t("accounting.settlement.new_payment")}
            </Button>
          </div>
        ) : null
      }
    >
      <EntryFilters
        value={filters}
        cariOptions={(cari.data?.items ?? []).map((c) => ({
          value: c.uuid,
          label: c.counterparty.name,
        }))}
        onChange={(next) => {
          setFilters(next);
          listState.setPagination((p) => ({ ...p, pageIndex: 0 }));
        }}
      />
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("accounting.entries.empty_title")}
        emptyDescription={t("accounting.entries.empty_description")}
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "tenant-accounting-entries-v1",
          rowSelection: false,
          globalFilter: false,
          columnFilters: false,
          facetedFilters: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void list.refetch()}
            refreshDisabled={list.isFetching}
          />
        }
      />
      {settle ? (
        <SettlementDialog
          orgUuid={access.orgUuid}
          kind={settle}
          open
          onOpenChange={(open) => {
            if (!open) setSettle(null);
          }}
        />
      ) : null}
    </EntityPage>
  );
}
