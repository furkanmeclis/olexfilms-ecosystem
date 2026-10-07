"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import {
  ArrowDownLeft,
  ArrowUpRight,
  Eye,
  FileText,
  UserPlus,
} from "lucide-react";
import Link from "next/link";
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
import { CustomerCariDialog } from "@/features/accounting/components/customer-cari-dialog";
import { SettlementDialog } from "@/features/accounting/components/settlement-dialog";
import { BalanceLabel, Money } from "@/features/accounting/components/shared";
import { ReadOnlyNotice } from "@/features/accounting/components/statement-page";
import {
  accountingKeys,
  useAccountingAccess,
} from "@/features/accounting/hooks/use-accounting-access";
import {
  accountingService,
  CARI_COUNTERPARTY_KINDS,
  type CariAccount,
  type ListCariParams,
  type SettlementKind,
} from "@/features/accounting/services/accounting.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const CARI_PERSIST_KEY = "tenant-accounting-cari-v2";

/** Active cari accounts first, as before. */
const INITIAL_FILTERS = [{ id: "active", value: true }];

/** Counterparty label: organization type or "customer" for a user cari. */
export function counterpartyKind(c: CariAccount): string {
  if (c.counterparty.type === "user") return "customer";
  return c.counterparty.org_type ?? "organization";
}

/**
 * Tenant > Accounting > Cari accounts (TEC-176, TEC-380). The backend lists
 * the active book's cari only; a distributor sees its parent and its own
 * dealers, never another distributor's tree. Server DataTable with sort
 * (name, balance, entries, last entry, created), `q`, counterparty kind
 * facet, balance range and the active filter; rows open the detail, the
 * statement or a collection / payment.
 */
export function CariPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const router = useRouter();
  const access = useAccountingAccess(slug);
  const { can } = usePermission();
  // Customer cari (TEC-342): a write gate plus the customer picker's read.
  const canOpenCustomerCari =
    access.canWrite && can(permissions.customers.read);
  const [customerCari, setCustomerCari] = useState(false);
  const [settle, setSettle] = useState<{
    kind: SettlementKind;
    cari: CariAccount;
  } | null>(null);

  const columns = useMemo(
    () =>
      [
        createColumn<CariAccount>({
          id: "name",
          accessorFn: (row) => row.counterparty.name,
          labelKey: "accounting.fields.cari",
          enableSorting: true,
          enableHiding: false,
          enableColumnFilter: false,
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
          filterVariant: "faceted",
          filterOptions: CARI_COUNTERPARTY_KINDS.map((value) => ({
            value,
            label: value,
            labelKey: `accounting.counterparty_types.${value}`,
          })),
          param: "counterparty_kind",
          cell: ({ row }) =>
            t(
              `accounting.counterparty_types.${counterpartyKind(row.original)}`,
            ),
        }),
        createColumn<CariAccount>({
          accessorKey: "balance",
          labelKey: "accounting.fields.balance",
          enableSorting: true,
          filterVariant: "number-range",
          param: "balance",
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
          accessorKey: "entry_count",
          labelKey: "accounting.fields.entry_count",
          enableSorting: true,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {format.number(row.original.entry_count)}
            </span>
          ),
        }),
        createColumn<CariAccount>({
          accessorKey: "last_entry_at",
          labelKey: "accounting.fields.last_entry",
          enableSorting: true,
          enableColumnFilter: false,
          cell: ({ row }) =>
            row.original.last_entry_at
              ? format.dateTime(row.original.last_entry_at)
              : "—",
        }),
        createColumn<CariAccount>({
          accessorKey: "active",
          labelKey: "accounting.fields.status",
          enableSorting: false,
          filterVariant: "boolean",
          param: "active",
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
        createColumn<CariAccount>({
          accessorKey: "created_at",
          labelKey: "accounting.fields.created_at",
          enableSorting: true,
          enableColumnFilter: false,
          defaultHidden: true,
          cell: ({ row }) => format.dateTime(row.original.created_at),
        }),
        createColumn<CariAccount>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          enableColumnFilter: false,
          cell: ({ row }) => {
            const cari = row.original;
            const actions: EntityRowAction[] = [
              {
                id: "open",
                label: t("common.open"),
                icon: Eye,
                onSelect: () =>
                  router.push(
                    routes.tenant.accounting.cariDetail(slug, cari.uuid),
                  ),
              },
              {
                id: "statement",
                label: t("accounting.cari.statement"),
                icon: FileText,
                onSelect: () =>
                  router.push(
                    routes.tenant.accounting.statement(slug, cari.uuid),
                  ),
              },
            ];
            if (access.canWrite && cari.active) {
              actions.push(
                {
                  id: "collection",
                  label: t("accounting.settlement.new_collection"),
                  icon: ArrowDownLeft,
                  onSelect: () => setSettle({ kind: "collection", cari }),
                },
                {
                  id: "payment",
                  label: t("accounting.settlement.new_payment"),
                  icon: ArrowUpRight,
                  onSelect: () => setSettle({ kind: "payment", cari }),
                },
              );
            }
            return <EntityRowActions actions={actions} />;
          },
        }),
      ] as ColumnDef<CariAccount, unknown>[],
    [access.canWrite, format, router, slug, t],
  );

  // Column meta drives the params: kind (CSV), balance range, active.
  const listState = useServerListState({
    columns,
    initialSort: "name",
    persistKey: CARI_PERSIST_KEY,
    initialColumnFilters: INITIAL_FILTERS,
  });
  const params: ListCariParams = listState.params;

  const list = useQuery({
    queryKey: accountingKeys.cariList(access.orgUuid, params),
    queryFn: () => accountingService.listCari(params),
    enabled: access.canRead && Boolean(access.orgUuid),
  });

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
      actions={
        canOpenCustomerCari ? (
          <Button
            onClick={() => setCustomerCari(true)}
            data-testid="customer-cari-open"
          >
            <UserPlus className="size-4" />
            {t("accounting.customer_cari.new")}
          </Button>
        ) : null
      }
    >
      {access.readOnlyDealer ? <ReadOnlyNotice /> : null}
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
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: CARI_PERSIST_KEY,
          rowSelection: false,
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
          kind={settle.kind}
          fixedCari={settle.cari}
          open
          onOpenChange={(open) => {
            if (!open) setSettle(null);
          }}
        />
      ) : null}
      {customerCari ? (
        <CustomerCariDialog
          orgUuid={access.orgUuid}
          open
          onOpenChange={setCustomerCari}
          onOpened={(cari) =>
            router.push(routes.tenant.accounting.cariDetail(slug, cari.uuid))
          }
        />
      ) : null}
    </EntityPage>
  );
}
