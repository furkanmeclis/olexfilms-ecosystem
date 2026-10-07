"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef, FilterFn } from "@tanstack/react-table";
import { CalendarCheck, HandCoins, History, Pencil } from "lucide-react";
import { useRouter } from "next/navigation";
import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  CLIENT_SIDE_MANUAL,
  clientDateRangeFilter,
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
import { Money } from "@/features/accounting/components/shared";
import { PayrollDialog } from "@/features/staff-reports/components/payroll-dialog";
import { StaffFormDialog } from "@/features/staff-reports/components/staff-form-dialog";
import { StaffPaymentDialog } from "@/features/staff-reports/components/staff-payment-dialog";
import { useStaffAccess } from "@/features/staff-reports/hooks/use-staff-access";
import {
  salaryOf,
  staffCreateInput,
  staffPatchInput,
  type StaffFormValues,
} from "@/features/staff-reports/lib/staff";
import {
  staffReportsKeys,
  staffReportsService,
  type StaffProfile,
} from "@/features/staff-reports/services/staff-reports.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const STAFF_PERSIST_KEY = "tenant-staff-profiles-v1";

const booleanFilter: FilterFn<StaffProfile> = (row, columnId, value) =>
  value === undefined || row.getValue(columnId) === value;

/**
 * Tenant > Staff (TEC-349 on the TEC-345 endpoints). The list endpoint
 * pages without sort or search, so every page is read and the DataTable
 * sorts, filters and searches in the browser. Row actions edit the card,
 * add a payment or open the payment history; the header opens the
 * month-end payroll.
 */
export function StaffPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const router = useRouter();
  const queryClient = useQueryClient();
  const access = useStaffAccess(slug);
  const [editing, setEditing] = useState<StaffProfile | null>(null);
  const [formOpen, setFormOpen] = useState(false);
  const [paying, setPaying] = useState<StaffProfile | null>(null);
  const [payrollOpen, setPayrollOpen] = useState(false);

  const list = useQuery({
    queryKey: staffReportsKeys.staff(access.orgUuid),
    queryFn: () => staffReportsService.listStaff(),
    enabled: access.canManage && Boolean(access.orgUuid),
  });
  const items = useMemo(() => list.data?.items ?? [], [list.data]);

  const save = useMutation({
    mutationFn: (values: StaffFormValues) =>
      editing
        ? staffReportsService.updateStaff(editing.uuid, staffPatchInput(values))
        : staffReportsService.createStaff(staffCreateInput(values)),
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

  const openForm = useCallback((staff: StaffProfile | null) => {
    setEditing(staff);
    setFormOpen(true);
  }, []);

  const columns = useMemo(
    () =>
      [
        createColumn<StaffProfile>({
          accessorKey: "name",
          labelKey: "staff_reports.fields.name",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.name}</span>
          ),
        }),
        createColumn<StaffProfile>({
          id: "title",
          accessorFn: (row) => row.title ?? "",
          labelKey: "staff_reports.fields.title",
          enableSorting: true,
          gridSecondary: true,
          filterVariant: "text",
          cell: ({ row }) => row.original.title || "—",
        }),
        createColumn<StaffProfile>({
          id: "monthly_salary",
          accessorFn: (row) => salaryOf(row),
          labelKey: "staff_reports.fields.monthly_salary",
          enableSorting: true,
          filterVariant: "number-range",
          meta: { headerClassName: "text-end", cellClassName: "text-end" },
          cell: ({ row }) =>
            row.original.monthly_salary ? (
              <Money
                amount={row.original.monthly_salary}
                currency={row.original.currency}
              />
            ) : (
              "—"
            ),
        }),
        createColumn<StaffProfile>({
          id: "hired_on",
          accessorFn: (row) => row.hired_on ?? "",
          labelKey: "staff_reports.fields.hired_on",
          enableSorting: true,
          sortUndefined: "last",
          filterVariant: "date-range",
          filterFn: clientDateRangeFilter as FilterFn<StaffProfile>,
          cell: ({ row }) =>
            row.original.hired_on ? format.date(row.original.hired_on) : "—",
        }),
        createColumn<StaffProfile>({
          accessorKey: "active",
          labelKey: "staff_reports.fields.status",
          enableSorting: true,
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
        createColumn<StaffProfile>({
          id: "linked_user",
          accessorFn: (row) => Boolean(row.user_uuid),
          labelKey: "staff_reports.fields.linked_user",
          enableSorting: false,
          defaultHidden: true,
          filterVariant: "boolean",
          filterFn: booleanFilter,
          cell: ({ row }) =>
            row.original.user_uuid ? t("common.yes") : t("common.no"),
        }),
        createColumn<StaffProfile>({
          accessorKey: "created_at",
          labelKey: "accounting.fields.created_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => format.dateTime(row.original.created_at),
        }),
        createColumn<StaffProfile>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const s = row.original;
            const actions: EntityRowAction[] = [
              {
                id: "history",
                label: t("staff_reports.staff.history"),
                icon: History,
                onSelect: () =>
                  router.push(routes.tenant.staff.detail(slug, s.uuid)),
              },
              {
                id: "edit",
                label: t("staff_reports.staff.edit"),
                icon: Pencil,
                onSelect: () => openForm(s),
              },
            ];
            if (access.canPay) {
              actions.push({
                id: "pay",
                label: t("staff_reports.payment.action"),
                icon: HandCoins,
                onSelect: () => setPaying(s),
              });
            }
            return <EntityRowActions actions={actions} />;
          },
        }),
      ] as ColumnDef<StaffProfile, unknown>[],
    [access.canPay, format, openForm, router, slug, t],
  );

  const forbidden = access.loaded && !access.canManage;

  return (
    <EntityPage
      title={t("staff_reports.staff.title")}
      description={t("staff_reports.staff.description")}
      permission={permissions.staff.manage}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("staff_reports.staff.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("staff_reports.nav") },
        { label: t("staff_reports.staff.title") },
      ]}
      actions={
        access.canManage ? (
          <div className="flex flex-wrap gap-2">
            {access.canPay ? (
              <Button
                variant="outline"
                data-testid="payroll-open"
                onClick={() => setPayrollOpen(true)}
              >
                <CalendarCheck className="size-4" />
                {t("staff_reports.payroll.title")}
              </Button>
            ) : null}
            <EntityCreateButton
              label={t("staff_reports.staff.create")}
              onClick={() => openForm(null)}
            />
          </div>
        ) : null
      }
    >
      {forbidden ? (
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("staff_reports.staff.module_off")}
        />
      ) : (
        <div data-testid="staff-list">
          <EntityTable
            columns={columns}
            data={items}
            getRowId={(row) => row.uuid}
            manual={CLIENT_SIDE_MANUAL}
            isLoading={list.isLoading}
            isError={list.isError}
            errorTitle={t("staff_reports.load_failed")}
            onRetry={() => void list.refetch()}
            emptyTitle={t("staff_reports.staff.empty_title")}
            emptyDescription={t("staff_reports.staff.empty_description")}
            onRowClick={(row) =>
              router.push(routes.tenant.staff.detail(slug, row.uuid))
            }
            initialState={{ sorting: [{ id: "name", desc: false }] }}
            features={{
              persistKey: STAFF_PERSIST_KEY,
              rowSelection: false,
              viewMode: true,
            }}
            renderGridItem={(s) => (
              <div className="space-y-1" data-testid="staff-card">
                <div className="flex items-start justify-between gap-2">
                  <span className="truncate font-medium">{s.name}</span>
                  {s.active ? null : (
                    <StatusChip label={t("accounting.status.inactive")} />
                  )}
                </div>
                <p className="text-muted-foreground text-sm">
                  {s.title || "—"}
                </p>
                {s.monthly_salary ? (
                  <Money
                    amount={s.monthly_salary}
                    currency={s.currency}
                    className="text-xl font-semibold"
                  />
                ) : null}
              </div>
            )}
          />
        </div>
      )}
      <StaffFormDialog
        open={formOpen}
        staff={editing}
        pending={save.isPending}
        onOpenChange={setFormOpen}
        onSubmit={(values) => save.mutateAsync(values)}
      />
      <StaffPaymentDialog
        orgUuid={access.orgUuid}
        staff={paying}
        onOpenChange={(v) => {
          if (!v) setPaying(null);
        }}
      />
      <PayrollDialog
        orgUuid={access.orgUuid}
        open={payrollOpen}
        staff={items}
        onOpenChange={setPayrollOpen}
      />
    </EntityPage>
  );
}
