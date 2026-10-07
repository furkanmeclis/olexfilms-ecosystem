"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef, FilterFn } from "@tanstack/react-table";
import { HandCoins, Pencil } from "lucide-react";
import { useMemo, useState, type ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  CLIENT_SIDE_MANUAL,
  clientDateRangeFilter,
  EntityPage,
  EntityRowActions,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { Money } from "@/features/accounting/components/shared";
import { useCancelPaymentAction } from "@/features/staff-reports/components/planned-payments";
import { StaffFormDialog } from "@/features/staff-reports/components/staff-form-dialog";
import { StaffPaymentDialog } from "@/features/staff-reports/components/staff-payment-dialog";
import { useStaffAccess } from "@/features/staff-reports/hooks/use-staff-access";
import {
  paymentStatusTone,
  periodDate,
  staffPatchInput,
  type StaffFormValues,
} from "@/features/staff-reports/lib/staff";
import {
  STAFF_PAYMENT_STATUSES,
  STAFF_PAYMENT_TYPES,
  staffReportsKeys,
  staffReportsService,
  type StaffPayment,
} from "@/features/staff-reports/services/staff-reports.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const STAFF_PAYMENTS_PERSIST_KEY = "tenant-staff-payments-v1";

const NUMERIC = { headerClassName: "text-end", cellClassName: "text-end" };

const facetFilter: FilterFn<StaffPayment> = (row, columnId, value) =>
  !Array.isArray(value) ||
  value.length === 0 ||
  value.includes(row.getValue(columnId));

/**
 * Tenant > Staff > card (TEC-349): the card summary, edit, "Add payment"
 * and the payment history (GET /v1/staff-profiles/{uuid}/payments, every
 * page; the DataTable sorts and filters in the browser). Each payment
 * shows its state (TEC-381): planned ones (payment day ahead, not booked
 * yet) can be cancelled.
 */
export function StaffDetailPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t, format } = useLocale();
  const queryClient = useQueryClient();
  const access = useStaffAccess(slug);
  const [formOpen, setFormOpen] = useState(false);
  const [paying, setPaying] = useState(false);
  const { action: cancelAction, dialog: cancelDialog } = useCancelPaymentAction(
    access.orgUuid,
  );
  const enabled = access.canManage && Boolean(access.orgUuid);

  // There is no single-card read: the card comes from the (cached) list.
  const staffList = useQuery({
    queryKey: staffReportsKeys.staff(access.orgUuid),
    queryFn: () => staffReportsService.listStaff(),
    enabled,
  });
  const staff = staffList.data?.items.find((s) => s.uuid === uuid) ?? null;

  const payments = useQuery({
    queryKey: staffReportsKeys.payments(access.orgUuid, uuid),
    queryFn: () => staffReportsService.listPayments(uuid),
    enabled: enabled && Boolean(staff),
  });

  const save = useMutation({
    mutationFn: (values: StaffFormValues) =>
      staffReportsService.updateStaff(uuid, staffPatchInput(values)),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: staffReportsKeys.all(access.orgUuid),
      });
      appToast.success(t("staff_reports.staff.saved"));
      setFormOpen(false);
    },
    onError: (error: unknown) =>
      appToast.error(
        isApiError(error) ? error.message : t("accounting.toast.failed"),
      ),
  });

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
          filterFn: clientDateRangeFilter as FilterFn<StaffPayment>,
          cell: ({ row }) => format.date(row.original.paid_on),
        }),
        createColumn<StaffPayment>({
          accessorKey: "type",
          labelKey: "staff_reports.fields.payment_type",
          enableSorting: true,
          gridSecondary: true,
          filterVariant: "faceted",
          filterFn: facetFilter,
          filterOptions: STAFF_PAYMENT_TYPES.map((value) => ({
            value,
            label: value,
            labelKey: `staff_reports.payment_types.${value}`,
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(`staff_reports.payment_types.${row.original.type}`)}
              tone={
                row.original.type === "salary"
                  ? "success"
                  : row.original.type === "bonus"
                    ? "default"
                    : "warning"
              }
            />
          ),
        }),
        createColumn<StaffPayment>({
          accessorKey: "status",
          labelKey: "staff_reports.fields.payment_status",
          enableSorting: true,
          filterVariant: "faceted",
          filterFn: facetFilter,
          filterOptions: STAFF_PAYMENT_STATUSES.map((value) => ({
            value,
            label: value,
            labelKey: `staff_reports.payment_statuses.${value}`,
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(`staff_reports.payment_statuses.${row.original.status}`)}
              tone={paymentStatusTone(row.original.status)}
            />
          ),
        }),
        createColumn<StaffPayment>({
          accessorKey: "period",
          labelKey: "staff_reports.fields.period",
          enableSorting: true,
          filterVariant: "text",
          cell: ({ row }) =>
            format.dateParts(periodDate(row.original.period), {
              year: "numeric",
              month: "long",
            }),
        }),
        createColumn<StaffPayment>({
          id: "amount",
          accessorFn: (row) => Number(row.amount),
          labelKey: "staff_reports.fields.amount",
          enableSorting: true,
          filterVariant: "number-range",
          meta: NUMERIC,
          cell: ({ row }) => (
            <Money
              amount={row.original.amount}
              currency={row.original.currency}
            />
          ),
        }),
        createColumn<StaffPayment>({
          id: "period_advances",
          accessorFn: (row) => Number(row.period_advances),
          labelKey: "staff_reports.fields.period_advances",
          enableSorting: true,
          defaultHidden: true,
          meta: NUMERIC,
          cell: ({ row }) => (
            <Money
              amount={row.original.period_advances}
              currency={row.original.currency}
            />
          ),
        }),
        createColumn<StaffPayment>({
          id: "description",
          accessorFn: (row) => row.description ?? "",
          labelKey: "staff_reports.fields.description",
          enableSorting: false,
          filterVariant: "text",
          cell: ({ row }) => (
            <span className="line-clamp-2">
              {row.original.description || "—"}
            </span>
          ),
        }),
        createColumn<StaffPayment>({
          id: "target_note",
          accessorFn: (row) => row.target_note ?? "",
          labelKey: "staff_reports.fields.target_note",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.target_note || "—",
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
            const action = access.canPay ? cancelAction(row.original) : null;
            return action ? <EntityRowActions actions={[action]} /> : null;
          },
        }),
      ] as ColumnDef<StaffPayment, unknown>[],
    [access.canPay, cancelAction, format, t],
  );

  const title = staff?.name ?? t("staff_reports.staff.title");
  const missing = staffList.isSuccess && !staff;

  return (
    <EntityPage
      title={title}
      description={staff?.title ?? undefined}
      permission={permissions.staff.manage}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("staff_reports.staff.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("staff_reports.staff.title"),
          href: routes.tenant.staff.list(slug),
        },
        { label: title },
      ]}
      actions={
        staff && access.canManage ? (
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              data-testid="staff-edit"
              onClick={() => setFormOpen(true)}
            >
              <Pencil className="size-4" />
              {t("staff_reports.staff.edit")}
            </Button>
            {access.canPay ? (
              <Button data-testid="staff-pay" onClick={() => setPaying(true)}>
                <HandCoins className="size-4" />
                {t("staff_reports.payment.action")}
              </Button>
            ) : null}
          </div>
        ) : null
      }
    >
      {staffList.isError ? (
        <ErrorState
          title={t("staff_reports.load_failed")}
          onRetry={() => void staffList.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : missing ? (
        <ErrorState title={t("staff_reports.staff.not_found")} />
      ) : (
        <div className="space-y-6">
          {staff ? (
            <Card data-testid="staff-summary">
              <CardContent className="grid gap-4 pt-6 sm:grid-cols-4">
                <Summary label={t("staff_reports.fields.monthly_salary")}>
                  {staff.monthly_salary ? (
                    <Money
                      amount={staff.monthly_salary}
                      currency={staff.currency}
                      className="font-semibold"
                    />
                  ) : (
                    "—"
                  )}
                </Summary>
                <Summary label={t("staff_reports.fields.hired_on")}>
                  {staff.hired_on ? format.date(staff.hired_on) : "—"}
                </Summary>
                <Summary label={t("staff_reports.fields.status")}>
                  <StatusChip
                    label={
                      staff.active
                        ? t("accounting.status.active")
                        : t("accounting.status.inactive")
                    }
                    tone={staff.active ? "success" : "default"}
                  />
                </Summary>
                <Summary label={t("staff_reports.fields.currency")}>
                  {staff.currency}
                </Summary>
              </CardContent>
            </Card>
          ) : null}
          <Card>
            <CardHeader>
              <CardTitle className="text-base">
                {t("staff_reports.staff.history")}
              </CardTitle>
            </CardHeader>
            <CardContent data-testid="staff-payments">
              <EntityTable
                columns={columns}
                data={payments.data?.items ?? []}
                getRowId={(row) => row.uuid}
                manual={CLIENT_SIDE_MANUAL}
                isLoading={staffList.isLoading || payments.isLoading}
                isError={payments.isError}
                errorTitle={t("staff_reports.load_failed")}
                onRetry={() => void payments.refetch()}
                emptyTitle={t("staff_reports.history.empty_title")}
                emptyDescription={t("staff_reports.history.empty_description")}
                initialState={{ sorting: [{ id: "paid_on", desc: true }] }}
                features={{
                  persistKey: STAFF_PAYMENTS_PERSIST_KEY,
                  rowSelection: false,
                }}
              />
            </CardContent>
          </Card>
        </div>
      )}
      <StaffFormDialog
        open={formOpen}
        staff={staff}
        pending={save.isPending}
        onOpenChange={setFormOpen}
        onSubmit={(values) => save.mutateAsync(values)}
      />
      {cancelDialog}
      <StaffPaymentDialog
        orgUuid={access.orgUuid}
        staff={paying ? staff : null}
        onOpenChange={(v) => {
          if (!v) setPaying(false);
        }}
      />
    </EntityPage>
  );
}

function Summary({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-1">
      <p className="text-muted-foreground text-xs">{label}</p>
      <div className="text-sm">{children}</div>
    </div>
  );
}
