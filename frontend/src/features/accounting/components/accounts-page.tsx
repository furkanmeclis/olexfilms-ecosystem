"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef, FilterFn } from "@tanstack/react-table";
import { Landmark, Pencil, Scale, Wallet } from "lucide-react";
import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  CLIENT_SIDE_MANUAL,
  EntityCreateButton,
  EntityPage,
  EntityRowActions,
  EntityTable,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { AccountFormDialog } from "@/features/accounting/components/account-form-dialog";
import { AccountOpeningDialog } from "@/features/accounting/components/account-opening-dialog";
import { Money } from "@/features/accounting/components/shared";
import {
  accountingKeys,
  useAccountingAccess,
} from "@/features/accounting/hooks/use-accounting-access";
import {
  accountCreateInput,
  accountUpdateInput,
  type AccountFormValues,
} from "@/features/accounting/lib/form";
import {
  ACCOUNT_TYPES,
  accountingService,
  type FinanceAccount,
  type FinanceAccountOpeningInput,
} from "@/features/accounting/services/accounting.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const ACCOUNTS_PERSIST_KEY = "tenant-accounting-accounts-v1";

/** Client-side boolean filter (undefined = all). */
const booleanFilter: FilterFn<FinanceAccount> = (row, columnId, value) =>
  value === undefined || row.getValue(columnId) === value;

/** Account icon by type. */
function AccountIcon({ type }: { type: FinanceAccount["type"] }) {
  return type === "bank" ? (
    <Landmark className="size-4 shrink-0" />
  ) : (
    <Wallet className="size-4 shrink-0" />
  );
}

/**
 * Tenant > Accounting > Cash and bank accounts (TEC-176, TEC-380).
 * `GET /v1/accounting/accounts` returns the full array, so the DataTable
 * sorts, filters (type, active) and searches in the browser; the card
 * grid stays the default view and the row menu edits or books the opening
 * balance.
 */
export function AccountsPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const queryClient = useQueryClient();
  const access = useAccountingAccess(slug);
  const [editing, setEditing] = useState<FinanceAccount | null>(null);
  const [open, setOpen] = useState(false);
  const [opening, setOpening] = useState<FinanceAccount | null>(null);

  const list = useQuery({
    queryKey: accountingKeys.accounts(access.orgUuid, {}),
    queryFn: () => accountingService.listAccounts(),
    enabled: access.canRead && Boolean(access.orgUuid),
  });

  const save = useMutation({
    mutationFn: (values: AccountFormValues) =>
      editing
        ? accountingService.updateAccount(
            editing.uuid,
            accountUpdateInput(values),
          )
        : accountingService.createAccount(accountCreateInput(values)),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: accountingKeys.all(access.orgUuid),
      });
      appToast.success(t("accounting.toast.saved"));
      setOpen(false);
    },
    onError: (error: unknown) =>
      appToast.error(
        isApiError(error) ? error.message : t("accounting.toast.failed"),
      ),
  });

  // TEC-198: one-off opening balance (step-up via platformRequest).
  const book = useMutation({
    mutationFn: (body: FinanceAccountOpeningInput) =>
      accountingService.createAccountOpening(opening?.uuid ?? "", body),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: accountingKeys.all(access.orgUuid),
      });
      appToast.success(t("accounting.opening.done"));
      setOpening(null);
    },
    onError: (error: unknown) =>
      appToast.error(
        isApiError(error) && error.code === "OPENING_BALANCE_EXISTS"
          ? t("accounting.opening.exists")
          : isApiError(error)
            ? error.message
            : t("accounting.toast.failed"),
      ),
  });

  const openForm = useCallback((account: FinanceAccount | null) => {
    setEditing(account);
    setOpen(true);
  }, []);

  const columns = useMemo(
    () =>
      [
        createColumn<FinanceAccount>({
          accessorKey: "name",
          labelKey: "accounting.fields.name",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex items-center gap-2 font-medium">
              <AccountIcon type={row.original.type} />
              <span className="truncate">{row.original.name}</span>
            </span>
          ),
        }),
        createColumn<FinanceAccount>({
          accessorKey: "type",
          labelKey: "accounting.fields.type",
          enableSorting: true,
          gridSecondary: true,
          filterVariant: "faceted",
          filterOptions: ACCOUNT_TYPES.map((value) => ({
            value,
            label: value,
            labelKey: `accounting.account_types.${value}`,
          })),
          cell: ({ row }) => t(`accounting.account_types.${row.original.type}`),
        }),
        createColumn<FinanceAccount>({
          id: "iban",
          accessorFn: (row) => row.iban ?? "",
          labelKey: "accounting.fields.iban",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.iban ? (
              <span dir="ltr" className="font-mono text-xs">
                {row.original.iban}
              </span>
            ) : (
              "—"
            ),
        }),
        createColumn<FinanceAccount>({
          id: "balance",
          accessorFn: (row) => Number(row.balance),
          labelKey: "accounting.fields.balance",
          enableSorting: true,
          cell: ({ row }) => (
            <div className="text-end">
              <Money
                amount={row.original.balance}
                currency={row.original.currency}
              />
            </div>
          ),
        }),
        createColumn<FinanceAccount>({
          accessorKey: "entry_count",
          labelKey: "accounting.fields.entry_count",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {format.number(row.original.entry_count)}
            </span>
          ),
        }),
        createColumn<FinanceAccount>({
          accessorKey: "last_entry_at",
          labelKey: "accounting.fields.last_entry",
          enableSorting: true,
          sortUndefined: "last",
          cell: ({ row }) =>
            row.original.last_entry_at
              ? format.dateTime(row.original.last_entry_at)
              : "—",
        }),
        createColumn<FinanceAccount>({
          accessorKey: "active",
          labelKey: "accounting.fields.status",
          enableSorting: false,
          filterVariant: "boolean",
          filterFn: booleanFilter,
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
        createColumn<FinanceAccount>({
          accessorKey: "created_at",
          labelKey: "accounting.fields.created_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => format.dateTime(row.original.created_at),
        }),
        createColumn<FinanceAccount>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const a = row.original;
            const actions: EntityRowAction[] = [];
            if (access.canWrite) {
              actions.push({
                id: "edit",
                label: t("accounting.accounts.edit"),
                icon: Pencil,
                onSelect: () => openForm(a),
              });
            }
            if (access.canWrite && a.active) {
              actions.push({
                id: "opening",
                label: t("accounting.opening.action"),
                icon: Scale,
                onSelect: () => setOpening(a),
              });
            }
            return actions.length ? (
              <EntityRowActions actions={actions} />
            ) : null;
          },
        }),
      ] as ColumnDef<FinanceAccount, unknown>[],
    [access.canWrite, format, openForm, t],
  );

  return (
    <EntityPage
      title={t("accounting.accounts.title")}
      description={
        access.canWrite
          ? t("accounting.accounts.description")
          : t("accounting.read_only")
      }
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
        { label: t("accounting.accounts.title") },
      ]}
      actions={
        access.canWrite ? (
          <EntityCreateButton
            label={t("accounting.accounts.create")}
            onClick={() => openForm(null)}
          />
        ) : null
      }
    >
      <div data-testid="account-list">
        <EntityTable
          columns={columns}
          data={list.data?.items ?? []}
          getRowId={(row) => row.uuid}
          manual={CLIENT_SIDE_MANUAL}
          isLoading={list.isLoading}
          isError={list.isError}
          errorTitle={t("accounting.load_failed")}
          onRetry={() => void list.refetch()}
          emptyTitle={t("accounting.accounts.empty_title")}
          emptyDescription={t("accounting.accounts.empty_description")}
          initialState={{
            viewMode: "grid",
            pagination: { pageIndex: 0, pageSize: 24 },
          }}
          features={{
            persistKey: ACCOUNTS_PERSIST_KEY,
            rowSelection: false,
            viewMode: true,
          }}
          renderGridItem={(a) => (
            <div className="space-y-1" data-testid="account-card">
              <div className="flex items-start justify-between gap-2">
                <span className="flex min-w-0 items-center gap-2 font-medium">
                  <AccountIcon type={a.type} />
                  <span className="truncate">{a.name}</span>
                </span>
                {access.canWrite ? (
                  <Button
                    size="icon"
                    variant="ghost"
                    className="size-8"
                    aria-label={t("accounting.accounts.edit")}
                    onClick={(event) => {
                      event.stopPropagation();
                      openForm(a);
                    }}
                  >
                    <Pencil className="size-4" />
                  </Button>
                ) : null}
              </div>
              <p className="text-muted-foreground flex flex-wrap items-center gap-2 text-sm">
                {t(`accounting.account_types.${a.type}`)}
                {a.active ? null : (
                  <StatusChip label={t("accounting.status.inactive")} />
                )}
              </p>
              <Money
                amount={a.balance}
                currency={a.currency}
                className="text-2xl font-semibold"
              />
              {a.iban ? (
                <p
                  dir="ltr"
                  className="text-muted-foreground text-start font-mono text-xs"
                >
                  {a.iban}
                </p>
              ) : null}
              <p className="text-muted-foreground text-xs">
                {t("accounting.accounts.entry_count", {
                  n: format.number(a.entry_count),
                })}
                {a.last_entry_at
                  ? ` · ${format.dateTime(a.last_entry_at)}`
                  : ""}
              </p>
              {access.canWrite && a.active ? (
                <Button
                  size="sm"
                  variant="outline"
                  className="mt-2"
                  data-testid="account-opening-action"
                  onClick={(event) => {
                    event.stopPropagation();
                    setOpening(a);
                  }}
                >
                  {t("accounting.opening.action")}
                </Button>
              ) : null}
            </div>
          )}
        />
      </div>
      <AccountFormDialog
        open={open}
        account={editing}
        pending={save.isPending}
        onOpenChange={setOpen}
        onSubmit={(values) => save.mutateAsync(values)}
      />
      <AccountOpeningDialog
        account={opening}
        pending={book.isPending}
        onOpenChange={(v) => {
          if (!v) setOpening(null);
        }}
        onSubmit={(body) => book.mutateAsync(body)}
      />
    </EntityPage>
  );
}
