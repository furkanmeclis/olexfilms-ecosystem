"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";
import { useRouter } from "next/navigation";
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
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  Money,
  NativeSelect,
  useCategoryLabels,
} from "@/features/accounting/components/shared";
import {
  accountingKeys,
  useAccountingAccess,
} from "@/features/accounting/hooks/use-accounting-access";
import { disputeStatusTone } from "@/features/accounting/lib/disputes";
import {
  accountingService,
  DISPUTE_STATUSES,
  type AccountingDispute,
  type AccountingDisputeStatus,
  type ListDisputesParams,
} from "@/features/accounting/services/accounting.service";
import { useLocale } from "@/providers/locale-provider";

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
 * Tenant > Accounting > Disputes (K24). The backend lists what the read
 * scope reaches: a child sees the disputes it opened, the parent the ones
 * addressed to it (a distributor: its dealers'). Open is the default filter
 * so the parent panel starts on the work to do.
 */
export function DisputesPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const router = useRouter();
  const access = useAccountingAccess(slug);
  const listState = useServerListState({ initialSort: "-created_at" });
  const [status, setStatus] = useState<AccountingDisputeStatus | "">("open");
  const enabled = access.canRead && Boolean(access.orgUuid);
  const categories = useCategoryLabels(access.orgUuid, enabled);

  const params = useMemo<ListDisputesParams>(
    () => ({
      limit: listState.params.limit,
      offset: listState.params.offset,
      ...(status ? { status } : {}),
    }),
    [listState.params.limit, listState.params.offset, status],
  );

  const list = useQuery({
    queryKey: accountingKeys.disputes(access.orgUuid, params),
    queryFn: () => accountingService.listDisputes(params),
    enabled,
  });

  const columns = useMemo(
    () =>
      [
        createColumn<AccountingDispute>({
          accessorKey: "created_at",
          labelKey: "accounting.disputes.fields.opened_at",
          enableSorting: false,
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
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => row.original.organization.name,
        }),
        createColumn<AccountingDispute>({
          id: "counterparty",
          accessorFn: (row) => row.counterparty_organization.name,
          labelKey: "accounting.disputes.fields.counterparty",
          enableSorting: false,
          cell: ({ row }) => row.original.counterparty_organization.name,
        }),
        createColumn<AccountingDispute>({
          id: "entry",
          accessorFn: (row) => row.entry.category,
          labelKey: "accounting.disputes.fields.entry",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) =>
            categories.get(row.original.entry.category) ??
            row.original.entry.category,
        }),
        createColumn<AccountingDispute>({
          id: "amount",
          accessorFn: (row) => row.entry.amount,
          labelKey: "accounting.fields.amount",
          enableSorting: false,
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
          enableSorting: false,
          cell: ({ row }) => <DisputeStatusChip status={row.original.status} />,
        }),
        createColumn<AccountingDispute>({
          accessorKey: "reason",
          labelKey: "accounting.disputes.fields.reason",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="line-clamp-2">{row.original.reason}</span>
          ),
        }),
      ] as ColumnDef<AccountingDispute, unknown>[],
    [categories, format, slug],
  );

  const total = list.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );

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
      <div className="flex flex-wrap items-end gap-2">
        <NativeSelect
          id="dispute-filter-status"
          label={t("accounting.fields.status")}
          value={status}
          className="w-56"
          onChange={(v) => {
            setStatus(v as AccountingDisputeStatus | "");
            listState.setPagination((p) => ({ ...p, pageIndex: 0 }));
          }}
          options={[
            { value: "", label: t("accounting.filters.all_statuses") },
            ...DISPUTE_STATUSES.map((s) => ({
              value: s,
              label: t(`accounting.disputes.statuses.${s}`),
            })),
          ]}
        />
      </div>
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
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "tenant-accounting-disputes-v1",
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
    </EntityPage>
  );
}
