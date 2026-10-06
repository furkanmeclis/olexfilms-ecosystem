"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Eye, Gavel } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo } from "react";

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
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  Money,
  useCategoryLabels,
} from "@/features/accounting/components/shared";
import {
  accountingKeys,
  useAccountingAccess,
} from "@/features/accounting/hooks/use-accounting-access";
import {
  canResolveDispute,
  disputeStatusTone,
} from "@/features/accounting/lib/disputes";
import {
  accountingService,
  DISPUTE_STATUSES,
  type AccountingDispute,
  type AccountingDisputeStatus,
  type ListDisputesParams,
} from "@/features/accounting/services/accounting.service";
import { useLocale } from "@/providers/locale-provider";

export const DISPUTES_PERSIST_KEY = "tenant-accounting-disputes-v2";

/** Open is the default filter: the parent panel starts on the work to do. */
const INITIAL_FILTERS = [{ id: "status", value: ["open"] }];

export function DisputeStatusChip({
  status,
}: {
  status: AccountingDisputeStatus;
}) {
  const { t } = useLocale();
  return (
    <span data-testid="dispute-status" data-status={status}>
      <StatusChip
        label={t(`accounting.disputes.statuses.${status}`)}
        tone={disputeStatusTone(status)}
      />
    </span>
  );
}

/**
 * Tenant > Accounting > Disputes (K24, TEC-380). The backend lists what the
 * read scope reaches: a child sees the disputes it opened, the parent the
 * ones addressed to it (a distributor: its dealers'). Server DataTable with
 * sort (opened, resolved, status, amount, organization), `q` (reason or
 * organization names), status facet (default open), organization facets
 * from the book's cari counterparties and the opened range.
 */
export function DisputesPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const router = useRouter();
  const access = useAccountingAccess(slug);
  const enabled = access.canRead && Boolean(access.orgUuid);
  const categories = useCategoryLabels(access.orgUuid, enabled);

  // Disputing and addressed organizations are cari counterparties of the
  // active book (its parent and its children).
  const cari = useQuery({
    queryKey: accountingKeys.cariList(access.orgUuid, { limit: 100 }),
    queryFn: () => accountingService.listCari({ limit: 100 }),
    enabled,
  });
  const orgOptions = useMemo(() => {
    const seen = new Map<string, string>();
    for (const c of cari.data?.items ?? []) {
      const { type, uuid, name } = c.counterparty;
      if (type === "organization" && uuid) seen.set(uuid, name);
    }
    return Array.from(seen, ([value, label]) => ({ value, label }));
  }, [cari.data]);

  const columns = useMemo(
    () =>
      [
        createColumn<AccountingDispute>({
          accessorKey: "created_at",
          labelKey: "accounting.disputes.fields.opened_at",
          enableSorting: true,
          filterVariant: "date-range",
          param: "created",
          cell: ({ row }) => (
            <Link
              href={routes.tenant.accounting.disputeDetail(
                slug,
                row.original.uuid,
              )}
              className="whitespace-nowrap hover:underline"
              onClick={(event) => event.stopPropagation()}
            >
              {format.dateTime(row.original.created_at)}
            </Link>
          ),
        }),
        createColumn<AccountingDispute>({
          id: "organization",
          accessorFn: (row) => row.organization.name,
          labelKey: "accounting.disputes.fields.organization",
          enableSorting: true,
          gridPrimary: true,
          filterVariant: "faceted",
          filterOptions: orgOptions,
          enableColumnFilter: orgOptions.length > 0,
          param: "organization_uuid",
          cell: ({ row }) => row.original.organization.name,
        }),
        createColumn<AccountingDispute>({
          id: "counterparty",
          accessorFn: (row) => row.counterparty_organization.name,
          labelKey: "accounting.disputes.fields.counterparty",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: orgOptions,
          enableColumnFilter: orgOptions.length > 0,
          param: "counterparty_organization_uuid",
          cell: ({ row }) => row.original.counterparty_organization.name,
        }),
        createColumn<AccountingDispute>({
          id: "entry",
          accessorFn: (row) => row.entry.category,
          labelKey: "accounting.disputes.fields.entry",
          enableSorting: false,
          enableColumnFilter: false,
          gridSecondary: true,
          cell: ({ row }) =>
            categories.get(row.original.entry.category) ??
            row.original.entry.category,
        }),
        createColumn<AccountingDispute>({
          id: "amount",
          accessorFn: (row) => row.entry.amount,
          labelKey: "accounting.fields.amount",
          enableSorting: true,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <div className="text-end">
              <Money
                amount={row.original.entry.orig_amount}
                currency={row.original.entry.orig_currency}
              />
            </div>
          ),
        }),
        createColumn<AccountingDispute>({
          accessorKey: "status",
          labelKey: "accounting.fields.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: DISPUTE_STATUSES.map((value) => ({
            value,
            label: value,
            labelKey: `accounting.disputes.statuses.${value}`,
          })),
          param: "status",
          cell: ({ row }) => <DisputeStatusChip status={row.original.status} />,
        }),
        createColumn<AccountingDispute>({
          accessorKey: "resolved_at",
          labelKey: "accounting.disputes.fields.resolved_at",
          enableSorting: true,
          enableColumnFilter: false,
          cell: ({ row }) =>
            row.original.resolved_at ? (
              <span className="whitespace-nowrap">
                {format.dateTime(row.original.resolved_at)}
              </span>
            ) : (
              "—"
            ),
        }),
        createColumn<AccountingDispute>({
          accessorKey: "reason",
          labelKey: "accounting.disputes.fields.reason",
          enableSorting: false,
          enableColumnFilter: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="line-clamp-2">{row.original.reason}</span>
          ),
        }),
        createColumn<AccountingDispute>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          enableColumnFilter: false,
          cell: ({ row }) => {
            const href = routes.tenant.accounting.disputeDetail(
              slug,
              row.original.uuid,
            );
            const actions: EntityRowAction[] = [
              {
                id: "open",
                label: t("common.open"),
                icon: Eye,
                onSelect: () => router.push(href),
              },
            ];
            if (
              canResolveDispute(row.original, access.orgUuid, access.canResolve)
            ) {
              actions.push({
                id: "resolve",
                label: t("accounting.disputes.resolve.title"),
                icon: Gavel,
                onSelect: () => router.push(href),
              });
            }
            return <EntityRowActions actions={actions} />;
          },
        }),
      ] as ColumnDef<AccountingDispute, unknown>[],
    [
      access.canResolve,
      access.orgUuid,
      categories,
      format,
      orgOptions,
      router,
      slug,
      t,
    ],
  );

  // Column meta drives the params: facets (CSV), created (_from / _to).
  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    persistKey: DISPUTES_PERSIST_KEY,
    initialColumnFilters: INITIAL_FILTERS,
  });
  const params: ListDisputesParams = listState.params;

  const list = useQuery({
    queryKey: accountingKeys.disputes(access.orgUuid, params),
    queryFn: () => accountingService.listDisputes(params),
    enabled,
  });

  return (
    <EntityPage
      title={t("accounting.disputes.title")}
      description={t("accounting.disputes.description")}
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
        { label: t("accounting.disputes.title") },
      ]}
    >
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(routes.tenant.accounting.disputeDetail(slug, row.uuid))
        }
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("accounting.disputes.empty_title")}
        emptyDescription={t("accounting.disputes.empty_description")}
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: DISPUTES_PERSIST_KEY,
          rowSelection: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void list.refetch()}
            refreshDisabled={list.isFetching}
          />
        }
      />
    </EntityPage>
  );
}
