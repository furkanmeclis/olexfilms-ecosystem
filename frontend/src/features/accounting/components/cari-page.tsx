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
  BalanceLabel,
  Money,
  NativeSelect,
} from "@/features/accounting/components/shared";
import {
  accountingKeys,
  useAccountingAccess,
} from "@/features/accounting/hooks/use-accounting-access";
import {
  accountingService,
  type CariAccount,
  type ListCariParams,
} from "@/features/accounting/services/accounting.service";
import { useLocale } from "@/providers/locale-provider";

/** Counterparty label: organization type or "customer" for a user cari. */
export function counterpartyKind(c: CariAccount): string {
  if (c.counterparty.type === "user") return "customer";
  return c.counterparty.org_type ?? "organization";
}

/**
 * Tenant > Accounting > Cari accounts (TEC-176). The backend lists the
 * active book's cari only; a distributor sees its parent and its own
 * dealers, never another distributor's tree.
 */
export function CariPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const router = useRouter();
  const access = useAccountingAccess(slug);
  const listState = useServerListState({ initialSort: "name" });
  const [active, setActive] = useState("true");

  const params = useMemo<ListCariParams>(
    () => ({
      limit: listState.params.limit,
      offset: listState.params.offset,
      q: listState.params.q,
      active: active === "" ? undefined : active === "true",
    }),
    [active, listState.params],
  );

  const list = useQuery({
    queryKey: accountingKeys.cariList(access.orgUuid, params),
    queryFn: () => accountingService.listCari(params),
    enabled: access.canRead && Boolean(access.orgUuid),
  });

  const columns = useMemo(
    () =>
      [
        createColumn<CariAccount>({
          id: "name",
          accessorFn: (row) => row.counterparty.name,
          labelKey: "accounting.fields.cari",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <Link
              href={routes.tenant.accounting.cariDetail(
                slug,
                row.original.uuid,
              )}
              className="font-medium hover:underline"
              onClick={(event) => event.stopPropagation()}
            >
              {row.original.counterparty.name}
            </Link>
          ),
        }),
        createColumn<CariAccount>({
          id: "kind",
          accessorFn: (row) => counterpartyKind(row),
          labelKey: "accounting.fields.counterparty_type",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) =>
            t(
              `accounting.counterparty_types.${counterpartyKind(row.original)}`,
            ),
        }),
        createColumn<CariAccount>({
          accessorKey: "balance",
          labelKey: "accounting.fields.balance",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="flex flex-wrap items-center justify-end gap-2">
              <Money
                amount={row.original.balance}
                currency={row.original.currency}
              />
              <BalanceLabel balance={row.original.balance} />
            </div>
          ),
        }),
        createColumn<CariAccount>({
          accessorKey: "last_entry_at",
          labelKey: "accounting.fields.last_entry",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.last_entry_at
              ? format.dateTime(row.original.last_entry_at)
              : "—",
        }),
        createColumn<CariAccount>({
          accessorKey: "active",
          labelKey: "accounting.fields.status",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <StatusChip
              label={
                row.original.active
                  ? t("accounting.status.active")
                  : t("accounting.status.inactive")
              }
              tone={row.original.active ? "success" : "default"}
            />
          ),
        }),
      ] as ColumnDef<CariAccount, unknown>[],
    [format, slug, t],
  );

  const total = list.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );

  return (
    <EntityPage
      title={t("accounting.cari.title")}
      description={t("accounting.cari.description")}
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
        { label: t("accounting.cari.title") },
      ]}
    >
      <div className="flex flex-wrap items-end gap-2">
        <NativeSelect
          id="cari-filter-active"
          label={t("accounting.fields.status")}
          value={active}
          className="w-48"
          onChange={(v) => {
            setActive(v);
            listState.setPagination((p) => ({ ...p, pageIndex: 0 }));
          }}
          options={[
            { value: "", label: t("accounting.filters.all_statuses") },
            { value: "true", label: t("accounting.status.active") },
            { value: "false", label: t("accounting.status.inactive") },
          ]}
        />
      </div>
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(routes.tenant.accounting.cariDetail(slug, row.uuid))
        }
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("accounting.cari.empty_title")}
        emptyDescription={t("accounting.cari.empty_description")}
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "tenant-accounting-cari-v1",
          rowSelection: false,
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
