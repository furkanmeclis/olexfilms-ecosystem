"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Ban } from "lucide-react";
import Link from "next/link";
import { useCallback, useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityRowActions,
  EntityTable,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { routes } from "@/config/routes";
import { accountingKeys } from "@/features/accounting/hooks/use-accounting-access";
import { Money } from "@/features/accounting/components/shared";
import {
  canCancelPayment,
  NOT_PLANNED_CODE,
  paymentStatusTone,
  periodDate,
} from "@/features/staff-reports/lib/staff";
import {
  STAFF_PAYMENT_TYPES,
  staffReportsKeys,
  staffReportsService,
  type StaffPayment,
} from "@/features/staff-reports/services/staff-reports.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const PLANNED_PAYMENTS_PERSIST_KEY = "tenant-staff-planned-payments-v1";

const NUMERIC = { headerClassName: "text-end", cellClassName: "text-end" };

/**
 * Cancels a planned payment (POST /v1/staff-payments/{uuid}/cancel) and
 * refreshes the staff lists and the reports.
 */
export function useCancelStaffPayment(orgUuid: string) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payment: StaffPayment) =>
      staffReportsService.cancelPayment(payment.uuid),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: staffReportsKeys.all(orgUuid),
        }),
        queryClient.invalidateQueries({
          queryKey: accountingKeys.all(orgUuid),
        }),
      ]);
      appToast.success(t("staff_reports.planned.cancelled"));
    },
    onError: (error: unknown) =>
      appToast.error(
        isApiError(error) && error.code === NOT_PLANNED_CODE
          ? t("staff_reports.planned.not_planned")
          : isApiError(error)
            ? error.message
            : t("accounting.toast.failed"),
      ),
  });
}

/** "Cancel payment" row action plus its confirmation. */
export function useCancelPaymentAction(orgUuid: string) {
  const { t, format } = useLocale();
  const cancel = useCancelStaffPayment(orgUuid);
  const [target, setTarget] = useState<StaffPayment | null>(null);

  const action = useCallback(
    (payment: StaffPayment): EntityRowAction | null =>
      canCancelPayment(payment)
        ? {
            id: "cancel",
            label: t("staff_reports.planned.cancel"),
            icon: Ban,
            variant: "destructive",
            onSelect: () => setTarget(payment),
          }
        : null,
    [t],
  );

  const dialog = (
    <ConfirmDialog
      open={Boolean(target)}
      title={t("staff_reports.planned.cancel_title")}
      description={
        target
          ? t("staff_reports.planned.cancel_description", {
              name: target.staff_name,
              date: format.date(target.paid_on),
            })
          : undefined
      }
      confirmLabel={t("staff_reports.planned.cancel")}
      cancelLabel={t("common.close")}
      variant="destructive"
      isPending={cancel.isPending}
      onCancel={() => setTarget(null)}
      onConfirm={() => {
        if (!target) return;
        cancel.mutate(target, { onSettled: () => setTarget(null) });
      }}
    />
  );

  return { action, dialog };
}

/**
 * Staff > Planned payments (TEC-381): salaries, advances and bonuses whose
 * payment day is still ahead (GET /v1/staff-payments?status=planned). They
 * have no ledger row yet; the worker books each on its day. The card shows
 * the total still to go out; a planned payment can be cancelled.
 */
export function PlannedPayments({
  slug,
  orgUuid,
  canPay,
}: {
  slug: string;
  orgUuid: string;
  canPay: boolean;
}) {
  const { t, format } = useLocale();
  const { action: cancelAction, dialog: cancelDialog } =
    useCancelPaymentAction(orgUuid);

  const columns = useMemo(
    () =>
      [
        createColumn<StaffPayment>({
          accessorKey: "paid_on",
          labelKey: "staff_reports.fields.paid_on",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          filterVariant: "date-range",
          param: "paid_on",
          cell: ({ row }) => format.date(row.original.paid_on),
        }),
        createColumn<StaffPayment>({
          accessorKey: "staff_name",
          labelKey: "staff_reports.fields.name",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <Link
              className="hover:underline"
              href={routes.tenant.staff.detail(slug, row.original.staff_uuid)}
            >
              {row.original.staff_name}
            </Link>
          ),
        }),
        createColumn<StaffPayment>({
          accessorKey: "type",
          labelKey: "staff_reports.fields.payment_type",
          enableSorting: true,
          filterVariant: "faceted",
          param: "type",
          filterOptions: STAFF_PAYMENT_TYPES.map((value) => ({
            value,
            label: value,
            labelKey: `staff_reports.payment_types.${value}`,
          })),
          cell: ({ row }) =>
            t(`staff_reports.payment_types.${row.original.type}`),
        }),
        createColumn<StaffPayment>({
          accessorKey: "period",
          labelKey: "staff_reports.fields.period",
          enableSorting: true,
          cell: ({ row }) =>
            format.dateParts(periodDate(row.original.period), {
              year: "numeric",
              month: "long",
            }),
        }),
        createColumn<StaffPayment>({
          accessorKey: "amount",
          labelKey: "staff_reports.fields.amount",
          enableSorting: true,
          meta: NUMERIC,
          cell: ({ row }) => (
            <Money
              amount={row.original.amount}
              currency={row.original.currency}
            />
          ),
        }),
        createColumn<StaffPayment>({
          accessorKey: "status",
          labelKey: "staff_reports.fields.payment_status",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <StatusChip
              label={t(`staff_reports.payment_statuses.${row.original.status}`)}
              tone={paymentStatusTone(row.original.status)}
            />
          ),
        }),
        createColumn<StaffPayment>({
          id: "description",
          accessorFn: (row) => row.description ?? "",
          labelKey: "staff_reports.fields.description",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="line-clamp-2">
              {row.original.description || "—"}
            </span>
          ),
        }),
        createColumn<StaffPayment>({
          accessorKey: "created_at",
          labelKey: "accounting.fields.created_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => format.dateTime(row.original.created_at),
        }),
        createColumn<StaffPayment>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const action = canPay ? cancelAction(row.original) : null;
            return action ? <EntityRowActions actions={[action]} /> : null;
          },
        }),
      ] as ColumnDef<StaffPayment, unknown>[],
    [canPay, cancelAction, format, slug, t],
  );

  const list = useServerListState({
    columns,
    initialSort: "paid_on",
    persistKey: PLANNED_PAYMENTS_PERSIST_KEY,
  });
  const query = useMemo(
    () => ({ ...list.params, status: "planned" }),
    [list.params],
  );
  const payments = useQuery({
    queryKey: staffReportsKeys.planned(orgUuid, query),
    queryFn: () => staffReportsService.listBookPayments(query),
    enabled: Boolean(orgUuid),
    placeholderData: keepPreviousData,
  });
  const page = payments.data;

  return (
    <Card data-testid="planned-payments">
      <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <CardTitle className="text-base">
            {t("staff_reports.planned.title")}
          </CardTitle>
          <p className="text-muted-foreground text-sm">
            {t("staff_reports.planned.description")}
          </p>
        </div>
        {page ? (
          <div className="text-end" data-testid="planned-total">
            <p className="text-muted-foreground text-xs">
              {t("staff_reports.planned.total", {
                n: format.number(page.total),
              })}
            </p>
            <Money
              amount={page.total_amount}
              currency={page.currency}
              className="text-lg font-semibold"
            />
          </div>
        ) : null}
      </CardHeader>
      <CardContent>
        <EntityTable
          columns={columns}
          data={page?.items ?? []}
          getRowId={(row) => row.uuid}
          isLoading={payments.isLoading}
          isError={payments.isError}
          errorTitle={t("staff_reports.load_failed")}
          onRetry={() => void payments.refetch()}
          emptyTitle={t("staff_reports.planned.empty_title")}
          emptyDescription={t("staff_reports.planned.empty_description")}
          rowCount={page?.total ?? 0}
          state={list.tableState}
          features={{
            persistKey: PLANNED_PAYMENTS_PERSIST_KEY,
            rowSelection: false,
            globalFilter: false,
          }}
        />
        {cancelDialog}
      </CardContent>
    </Card>
  );
}

/**
 * Reports > Staff cost: the payments still to go out (planned, not in the
 * report yet) and the next ones (TEC-381).
 */
export function UpcomingStaffPayments({
  org,
  slug,
}: {
  org: string;
  slug: string;
}) {
  const { t, format } = useLocale();
  const upcoming = useQuery({
    queryKey: staffReportsKeys.upcoming(org),
    queryFn: () => staffReportsService.upcomingPayments(),
    enabled: Boolean(org),
  });
  const page = upcoming.data;
  if (!page) return null;

  return (
    <div className="space-y-3 rounded-md border p-4" data-testid="upcoming">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <p className="text-sm font-medium">
            {t("staff_reports.planned.upcoming_title")}
          </p>
          <p className="text-muted-foreground text-xs">
            {t("staff_reports.planned.upcoming_description")}
          </p>
        </div>
        <div className="text-end" data-testid="upcoming-total">
          <p className="text-muted-foreground text-xs">
            {t("staff_reports.planned.total", {
              n: format.number(page.total),
            })}
          </p>
          <Money
            amount={page.total_amount}
            currency={page.currency}
            className="text-lg font-semibold"
          />
        </div>
      </div>
      {page.items.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("staff_reports.planned.empty_title")}
        </p>
      ) : (
        <ul className="divide-y text-sm">
          {page.items.map((p) => (
            <li
              key={p.uuid}
              className="flex items-center justify-between gap-3 py-2"
              data-testid="upcoming-row"
            >
              <span className="min-w-0 truncate">
                <span className="text-muted-foreground tabular-nums">
                  {format.date(p.paid_on)}
                </span>{" "}
                {p.staff_name} · {t(`staff_reports.payment_types.${p.type}`)}
              </span>
              <Money amount={p.amount} currency={p.currency} />
            </li>
          ))}
        </ul>
      )}
      {page.total > page.items.length ? (
        <Link
          className="text-primary text-sm hover:underline"
          href={routes.tenant.staff.list(slug)}
        >
          {t("staff_reports.planned.show_all")}
        </Link>
      ) : null}
    </div>
  );
}
