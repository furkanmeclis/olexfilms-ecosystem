"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import {
  ArrowDownLeft,
  ArrowUpRight,
  BookOpen,
  Plus,
  Undo2,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { DisputeButton } from "@/features/accounting/components/dispute-dialog";
import { EntriesExportMenu } from "@/features/accounting/components/entries-export";
import { ManualEntryDialog } from "@/features/accounting/components/manual-entry-dialog";
import { SettlementDialog } from "@/features/accounting/components/settlement-dialog";
import {
  EntryAmount,
  EntryStatus,
  useCategoryLabels,
} from "@/features/accounting/components/shared";
import {
  ReadOnlyNotice,
  useOpenDisputeEntries,
} from "@/features/accounting/components/statement-page";
import { VoidEntryDialog } from "@/features/accounting/components/void-entry-dialog";
import {
  accountingKeys,
  useAccountingAccess,
} from "@/features/accounting/hooks/use-accounting-access";
import { isEntryVoidable } from "@/features/accounting/lib/access";
import { isEntryDisputable } from "@/features/accounting/lib/disputes";
import {
  ACCOUNTING_DIRECTIONS,
  accountingService,
  ENTRY_SOURCE_TYPES,
  type FinanceEntry,
  type ListEntriesParams,
  type SettlementKind,
} from "@/features/accounting/services/accounting.service";
import { useLocale } from "@/providers/locale-provider";

export const ENTRIES_PERSIST_KEY = "tenant-accounting-entries-v2";

/** Dispute controls of a child organization's ledger rows (TEC-195). */
export type EntryDisputeOptions = {
  orgUuid: string;
  parentUuid: string | null;
  /** Entry uuids that already carry an open dispute. */
  openDisputes: ReadonlySet<string>;
};

/**
 * Status cell: reversal markers, an open dispute marker, or the "dispute"
 * button of a disputable row (only passed with accounting.dispute).
 */
export function EntryStatusCell({
  entry,
  dispute,
}: {
  entry: FinanceEntry;
  dispute?: EntryDisputeOptions | null;
}) {
  const { t, format } = useLocale();
  const disputed = Boolean(dispute?.openDisputes.has(entry.uuid));
  return (
    <div className="flex flex-wrap items-center gap-1">
      <EntryStatus entry={entry} />
      {disputed ? (
        <span data-testid="entry-disputed">
          <StatusChip
            label={t("accounting.disputes.statuses.open")}
            tone="warning"
          />
        </span>
      ) : dispute && isEntryDisputable(entry, dispute.parentUuid) ? (
        <DisputeButton
          orgUuid={dispute.orgUuid}
          entryUuid={entry.uuid}
          summary={`${format.dateTime(entry.created_at)} · ${entry.category}`}
        />
      ) : null}
    </div>
  );
}

type FilterOption = { value: string; label: string };

/** Filter options of the ledger columns (catalogs of the active book). */
export type EntryColumnOptions = {
  categories: Map<string, string>;
  cari: FilterOption[];
  accounts: FilterOption[];
  dispute?: EntryDisputeOptions | null;
  /** Row action: open the cari account of the row. */
  onOpenCari?: (cariUuid: string) => void;
  /**
   * Row action: reverse an open manual entry (only passed with write
   * access). Rows another module or the parent posted never get it; a
   * child organization disputes them from the status cell.
   */
  onVoid?: (entry: FinanceEntry) => void;
};

function enumOptions(values: readonly string[], prefix: string) {
  return values.map((value) => ({
    value,
    label: value,
    labelKey: `${prefix}.${value}`,
  }));
}

/**
 * Ledger columns (TEC-380): sort on date, type, category and amount; facets
 * (type, category, source), cari / account selects, date and amount ranges.
 */
export function useEntryColumns({
  categories,
  cari,
  accounts,
  dispute,
  onOpenCari,
  onVoid,
}: EntryColumnOptions) {
  const { t, format } = useLocale();
  return useMemo(
    () =>
      [
        createColumn<FinanceEntry>({
          accessorKey: "created_at",
          labelKey: "accounting.fields.date",
          enableSorting: true,
          filterVariant: "date-range",
          param: "created",
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {format.dateTime(row.original.created_at)}
            </span>
          ),
        }),
        createColumn<FinanceEntry>({
          accessorKey: "direction",
          labelKey: "accounting.fields.direction",
          enableSorting: true,
          gridSecondary: true,
          filterVariant: "faceted",
          filterOptions: enumOptions(
            ACCOUNTING_DIRECTIONS,
            "accounting.directions",
          ),
          param: "direction",
          cell: ({ row }) =>
            t(`accounting.directions.${row.original.direction}`),
        }),
        createColumn<FinanceEntry>({
          accessorKey: "category",
          labelKey: "accounting.fields.category",
          enableSorting: true,
          gridPrimary: true,
          filterVariant: "faceted",
          filterOptions: Array.from(categories, ([value, label]) => ({
            value,
            label,
          })),
          enableColumnFilter: categories.size > 0,
          param: "category",
          cell: ({ row }) =>
            categories.get(row.original.category) ?? row.original.category,
        }),
        createColumn<FinanceEntry>({
          id: "counterparty",
          accessorFn: (row) => row.counterparty_organization?.name ?? "",
          labelKey: "accounting.fields.cari",
          enableSorting: false,
          filterVariant: "select",
          filterOptions: cari,
          enableColumnFilter: cari.length > 0,
          param: "cari_uuid",
          cell: ({ row }) =>
            row.original.counterparty_organization?.name ?? "—",
        }),
        createColumn<FinanceEntry>({
          id: "account",
          accessorFn: (row) => row.account?.name ?? "",
          labelKey: "accounting.fields.account",
          enableSorting: false,
          filterVariant: "select",
          filterOptions: accounts,
          enableColumnFilter: accounts.length > 0,
          param: "account_uuid",
          cell: ({ row }) => row.original.account?.name ?? "—",
        }),
        createColumn<FinanceEntry>({
          accessorKey: "source_type",
          labelKey: "accounting.fields.source",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: enumOptions(ENTRY_SOURCE_TYPES, "accounting.sources"),
          param: "source_type",
          cell: ({ row }) => {
            const s = row.original.source_type;
            if (!s) return "—";
            return (ENTRY_SOURCE_TYPES as readonly string[]).includes(s)
              ? t(`accounting.sources.${s}`)
              : s;
          },
        }),
        createColumn<FinanceEntry>({
          accessorKey: "amount",
          labelKey: "accounting.fields.amount",
          enableSorting: true,
          filterVariant: "number-range",
          param: "amount",
          cell: ({ row }) => <EntryAmount entry={row.original} />,
        }),
        createColumn<FinanceEntry>({
          id: "status",
          accessorFn: (row) => (row.reversal_of_uuid ? 2 : row.voided ? 1 : 0),
          labelKey: "accounting.fields.status",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <EntryStatusCell entry={row.original} dispute={dispute} />
          ),
        }),
        createColumn<FinanceEntry>({
          accessorKey: "description",
          labelKey: "accounting.fields.description",
          enableSorting: false,
          enableColumnFilter: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.description ?? "—",
        }),
        createColumn<FinanceEntry>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          enableColumnFilter: false,
          cell: ({ row }) => {
            const entry = row.original;
            const cariUuid = entry.cari_uuid;
            const actions: EntityRowAction[] = [];
            if (cariUuid && onOpenCari) {
              actions.push({
                id: "cari",
                label: t("accounting.cari.detail_title"),
                icon: BookOpen,
                onSelect: () => onOpenCari(cariUuid),
              });
            }
            if (onVoid && isEntryVoidable(entry)) {
              actions.push({
                id: "void",
                label: t("accounting.void.action"),
                icon: Undo2,
                variant: "destructive",
                onSelect: () => onVoid(entry),
              });
            }
            return actions.length ? (
              <EntityRowActions actions={actions} />
            ) : null;
          },
        }),
      ] as ColumnDef<FinanceEntry, unknown>[],
    [accounts, cari, categories, dispute, format, onOpenCari, onVoid, t],
  );
}

/**
 * Tenant > Accounting > Entries (TEC-176, TEC-380): server DataTable over
 * GET /v1/accounting/entries with sort, `q`, column filters (date, type,
 * category, cari, account, source, amount) and the list export.
 */
export function EntriesPage({
  slug,
  initialCari = "",
}: {
  slug: string;
  initialCari?: string;
}) {
  const { t } = useLocale();
  const router = useRouter();
  const access = useAccountingAccess(slug);
  const [settle, setSettle] = useState<SettlementKind | null>(null);
  const [manual, setManual] = useState(false);
  const [voiding, setVoiding] = useState<FinanceEntry | null>(null);
  const enabled = access.canRead && Boolean(access.orgUuid);
  const categories = useCategoryLabels(access.orgUuid, enabled);
  const openDisputes = useOpenDisputeEntries(
    access.orgUuid,
    enabled && access.canDispute,
  );
  const disputeKey = Array.from(openDisputes).sort().join(",");
  const dispute = useMemo<EntryDisputeOptions | null>(
    () =>
      access.canDispute
        ? {
            orgUuid: access.orgUuid,
            parentUuid: access.parentUuid,
            openDisputes: new Set(disputeKey ? disputeKey.split(",") : []),
          }
        : null,
    [access.canDispute, access.orgUuid, access.parentUuid, disputeKey],
  );

  const cari = useQuery({
    queryKey: accountingKeys.cariList(access.orgUuid, { limit: 100 }),
    queryFn: () => accountingService.listCari({ limit: 100 }),
    enabled,
  });
  const accounts = useQuery({
    queryKey: accountingKeys.accounts(access.orgUuid, {}),
    queryFn: () => accountingService.listAccounts(),
    enabled,
  });
  const cariOptions = useMemo(
    () =>
      (cari.data?.items ?? []).map((c) => ({
        value: c.uuid,
        label: c.counterparty.name,
      })),
    [cari.data],
  );
  const accountOptions = useMemo(
    () =>
      (accounts.data?.items ?? []).map((a) => ({
        value: a.uuid,
        label: a.name,
      })),
    [accounts.data],
  );
  const openCari = useMemo(
    () => (uuid: string) =>
      router.push(routes.tenant.accounting.cariDetail(slug, uuid)),
    [router, slug],
  );
  const columns = useEntryColumns({
    categories,
    cari: cariOptions,
    accounts: accountOptions,
    dispute,
    onOpenCari: openCari,
    onVoid: access.canWrite ? setVoiding : undefined,
  });

  const initialFilters = useMemo(
    () => (initialCari ? [{ id: "counterparty", value: initialCari }] : []),
    [initialCari],
  );
  // Column meta drives the params: facets (CSV), selects, created / amount.
  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    persistKey: ENTRIES_PERSIST_KEY,
    initialColumnFilters: initialFilters,
  });
  const params: ListEntriesParams = listState.params;

  const list = useQuery({
    queryKey: accountingKeys.entries(access.orgUuid, params),
    queryFn: () => accountingService.listEntries(params),
    enabled,
  });

  // The export takes the list filters, search and sort.
  const exportQuery = useMemo(() => {
    const query: Record<string, string> = {};
    for (const [key, value] of Object.entries(listState.filterParams)) {
      if (value) query[key] = String(value);
    }
    if (params.q) query.q = String(params.q);
    if (params.sort) query.sort = String(params.sort);
    return query;
  }, [listState.filterParams, params.q, params.sort]);

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
            <Button
              onClick={() => setManual(true)}
              data-testid="manual-entry-open"
            >
              <Plus className="size-4" />
              {t("accounting.manual.new")}
            </Button>
            <Button variant="outline" onClick={() => setSettle("collection")}>
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
      {access.readOnlyDealer ? <ReadOnlyNotice /> : null}
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("accounting.entries.empty_title")}
        emptyDescription={t("accounting.entries.empty_description")}
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: ENTRIES_PERSIST_KEY,
          rowSelection: false,
        }}
        toolbarExtra={
          <>
            {enabled ? (
              <EntriesExportMenu orgUuid={access.orgUuid} query={exportQuery} />
            ) : null}
            <EntityToolbar
              onRefresh={() => void list.refetch()}
              refreshDisabled={list.isFetching}
            />
          </>
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
      {manual ? (
        <ManualEntryDialog
          orgUuid={access.orgUuid}
          open
          onOpenChange={setManual}
        />
      ) : null}
      {access.canWrite ? (
        <VoidEntryDialog
          orgUuid={access.orgUuid}
          entry={voiding}
          onOpenChange={(open) => {
            if (!open) setVoiding(null);
          }}
        />
      ) : null}
    </EntityPage>
  );
}
