"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef, RowSelectionState } from "@tanstack/react-table";
import { CheckCircle2, ClipboardCheck, Loader2, XCircle } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { CLIENT_SIDE_MANUAL, EntityTable } from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import {
  createColumn,
  createSelectColumnDef,
  type DataTableBulkAction,
} from "@/components/tables";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  fleetKeys,
  fleetsService,
  type FleetPlan,
  type FleetPlanIntakeRow,
} from "@/features/fleets/services/fleets.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

type PlanAppointment = FleetPlan["appointments"][number];

export const FLEET_PLAN_APPOINTMENTS_PERSIST_KEY =
  "tenant-fleet-plan-appointments-v1";

/** Appointments that can still be taken into intake. */
export function intakeable(a: PlanAppointment) {
  return (
    !a.service_uuid &&
    (a.status === "scheduled" ||
      a.status === "confirmed" ||
      a.status === "arrived")
  );
}

function appointmentTone(status: string) {
  if (status === "cancelled" || status === "no_show") return "danger" as const;
  if (status === "arrived") return "success" as const;
  return "default" as const;
}

/** Summary of a row-wise intake run. */
export function intakeSummary(results: FleetPlanIntakeRow[]) {
  const ok = results.filter((r) => r.ok).length;
  return { ok, failed: results.length - ok };
}

/**
 * Fleet plan detail (TEC-477): the plan's appointments (plate, day, status,
 * draft service); "Take selected into intake" calls start-intake for the
 * selected rows and shows the result row by row (one failure does not stop
 * the others). The plan can be cancelled.
 */
export function FleetPlanDetailPage({
  slug,
  fleetUuid,
  planUuid,
}: {
  slug: string;
  fleetUuid: string;
  planUuid: string;
}) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const qc = useQueryClient();
  const [selection, setSelection] = useState<RowSelectionState>({});
  const [results, setResults] = useState<Map<string, FleetPlanIntakeRow>>(
    new Map(),
  );
  const canPlan = can(permissions.fleets.plan);

  const plan = useQuery({
    queryKey: fleetKeys.plan(fleetUuid, planUuid),
    queryFn: () => fleetsService.getPlan(fleetUuid, planUuid),
    enabled: canPlan,
  });
  const card = useQuery({
    queryKey: fleetKeys.card(fleetUuid),
    queryFn: () => fleetsService.card(fleetUuid),
    enabled: canPlan,
  });

  const selectedUuids = Object.keys(selection).filter((k) => selection[k]);

  const intake = useMutation({
    mutationFn: (uuids: string[]) =>
      fleetsService.startIntake(fleetUuid, planUuid, uuids),
    onSuccess: async (out) => {
      setResults(new Map(out.results.map((r) => [r.appointment_uuid, r])));
      setSelection({});
      const { ok, failed } = intakeSummary(out.results);
      if (failed === 0) {
        appToast.success(t("fleets.intake.done", { ok }));
      } else {
        appToast.warning(t("fleets.intake.partial", { ok, failed }));
      }
      await qc.invalidateQueries({
        queryKey: fleetKeys.plan(fleetUuid, planUuid),
      });
      await qc.invalidateQueries({ queryKey: ["fleets", fleetUuid, "plans"] });
    },
    onError: (error) =>
      appToast.error(
        isApiError(error) ? error.message : t("fleets.intake.failed"),
      ),
  });

  const cancel = useMutation({
    mutationFn: () => fleetsService.cancelPlan(fleetUuid, planUuid),
    onSuccess: async (updated) => {
      qc.setQueryData(fleetKeys.plan(fleetUuid, planUuid), updated);
      appToast.success(t("fleets.plan.cancelled"));
      await qc.invalidateQueries({ queryKey: ["fleets", fleetUuid, "plans"] });
    },
  });

  const columns = useMemo(
    () =>
      [
        createSelectColumnDef<PlanAppointment>(),
        createColumn<PlanAppointment>({
          id: "plate",
          accessorFn: (row) => row.vehicle_plate ?? "",
          labelKey: "fleets.fields.plate",
          enableSorting: true,
          enableHiding: false,
          cell: ({ row }) => (
            <div className="min-w-0">
              <div className="font-mono font-medium" dir="ltr">
                {row.original.vehicle_plate ?? "—"}
              </div>
              {row.original.vehicle_label ? (
                <div className="text-muted-foreground truncate text-xs">
                  {row.original.vehicle_label}
                </div>
              ) : null}
            </div>
          ),
        }),
        createColumn<PlanAppointment>({
          accessorKey: "starts_at",
          labelKey: "fleets.intake.starts_at",
          enableSorting: true,
          cell: ({ row }) => format.dateTime(row.original.starts_at),
        }),
        createColumn<PlanAppointment>({
          accessorKey: "status",
          labelKey: "fleets.intake.status",
          enableSorting: true,
          cell: ({ row }) => (
            <StatusChip
              label={t(`appointments.status.${row.original.status}`)}
              tone={appointmentTone(row.original.status)}
            />
          ),
        }),
        createColumn<PlanAppointment>({
          id: "service",
          accessorFn: (row) => row.service_uuid ?? "",
          labelKey: "fleets.intake.service",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.service_uuid ? (
              <Link
                href={routes.tenant.services.detail(
                  slug,
                  row.original.service_uuid,
                )}
                className="hover:underline"
              >
                {t("fleets.intake.open_service")}
              </Link>
            ) : (
              "—"
            ),
        }),
        createColumn<PlanAppointment>({
          id: "result",
          accessorFn: (row) => results.get(row.uuid)?.ok ?? null,
          labelKey: "fleets.intake.result",
          enableSorting: false,
          cell: ({ row }) => {
            const r = results.get(row.original.uuid);
            if (!r) return null;
            return r.ok ? (
              <span
                className="flex items-center gap-1 text-sm text-emerald-700 dark:text-emerald-400"
                data-testid="intake-result-ok"
                data-appointment={r.appointment_uuid}
              >
                <CheckCircle2 className="size-4" />
                {t("fleets.intake.ok")}
              </span>
            ) : (
              <span
                className="text-destructive flex items-center gap-1 text-sm"
                data-testid="intake-result-error"
                data-appointment={r.appointment_uuid}
                title={r.message}
              >
                <XCircle className="size-4" />
                {t(`fleets.intake.codes.${r.code ?? "ERROR"}`)}
              </span>
            );
          },
        }),
      ] as ColumnDef<PlanAppointment, unknown>[],
    [format, results, slug, t],
  );

  const bulkActions = useMemo<DataTableBulkAction<PlanAppointment>[]>(
    () => [
      {
        id: "start_intake",
        label: t("bulk.actions.fleet_plan_appointments.start_intake"),
        icon: ClipboardCheck,
        disabled: (selected) => !selected.some(intakeable),
        onClick: (selected) =>
          intake.mutate(selected.filter(intakeable).map((a) => a.uuid)),
      },
    ],
    [intake, t],
  );

  const data = plan.data;
  const fleetName = card.data?.name ?? t("fleets.card.title");
  const title = data
    ? t("fleets.plan.detail_title", {
        service: data.service_type,
      })
    : t("fleets.plan.detail_title", { service: "" });
  const header = (
    <PageHeader
      title={title}
      description={data?.note || undefined}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("fleets.title"), href: routes.tenant.fleets.list(slug) },
        {
          label: fleetName,
          href: routes.tenant.fleets.detail(slug, fleetUuid, "plans"),
        },
        { label: title },
      ]}
      actions={
        data && data.status === "scheduled" ? (
          <Button
            type="button"
            variant="outline"
            disabled={cancel.isPending}
            onClick={() => {
              if (window.confirm(t("fleets.plan.cancel_confirm"))) {
                cancel.mutate();
              }
            }}
          >
            {t("fleets.plan.cancel")}
          </Button>
        ) : null
      }
    />
  );

  if (!canPlan) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("fleets.forbidden")}
        />
      </div>
    );
  }
  if (plan.isError) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_generic")}
          retryLabel={t("common.retry")}
          onRetry={() => void plan.refetch()}
        />
      </div>
    );
  }

  const appointments = data?.appointments ?? [];
  const selectedIntakeable = appointments.filter(
    (a) => selectedUuids.includes(a.uuid) && intakeable(a),
  );
  const summary = results.size ? intakeSummary([...results.values()]) : null;

  return (
    <div className="space-y-6">
      {header}
      {data ? (
        <div className="flex flex-wrap items-center gap-3 text-sm">
          <StatusChip
            label={t(`fleets.plan.status_${data.status}`)}
            tone={data.status === "scheduled" ? "success" : "default"}
          />
          <span className="text-muted-foreground">
            {t("fleets.plan.appointment_count")}: {appointments.length}
          </span>
        </div>
      ) : null}
      {summary ? (
        <Alert
          variant={summary.failed ? "destructive" : "default"}
          data-testid="intake-summary"
        >
          <AlertTitle>
            {t("fleets.intake.summary", {
              ok: summary.ok,
              failed: summary.failed,
            })}
          </AlertTitle>
          {summary.failed ? (
            <AlertDescription>
              {t("fleets.intake.summary_hint")}
            </AlertDescription>
          ) : null}
        </Alert>
      ) : null}
      <EntityTable
        columns={columns}
        data={appointments}
        getRowId={(row) => row.uuid}
        manual={CLIENT_SIDE_MANUAL}
        isLoading={plan.isLoading}
        emptyTitle={t("fleets.intake.empty")}
        emptyDescription=""
        initialState={{ pagination: { pageIndex: 0, pageSize: 50 } }}
        state={{ rowSelection: selection, onRowSelectionChange: setSelection }}
        bulkActions={bulkActions}
        features={{
          persistKey: FLEET_PLAN_APPOINTMENTS_PERSIST_KEY,
          rowSelection: data?.status === "scheduled",
        }}
        toolbarExtra={
          data?.status === "scheduled" ? (
            <Button
              type="button"
              size="sm"
              data-testid="intake-selected"
              disabled={selectedIntakeable.length === 0 || intake.isPending}
              onClick={() =>
                intake.mutate(selectedIntakeable.map((a) => a.uuid))
              }
            >
              {intake.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <ClipboardCheck className="size-4" />
              )}
              {t("fleets.intake.start_selected", {
                count: selectedIntakeable.length,
              })}
            </Button>
          ) : null
        }
      />
    </div>
  );
}
