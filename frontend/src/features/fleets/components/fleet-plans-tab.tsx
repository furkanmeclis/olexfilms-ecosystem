"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { CalendarPlus, Eye } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import {
  FLEET_PLAN_STATUSES,
  fleetKeys,
  fleetsService,
  type FleetCard,
  type FleetPlanSummary,
} from "@/features/fleets/services/fleets.service";
import { useLocale } from "@/providers/locale-provider";

export const FLEET_PLANS_PERSIST_KEY = "tenant-fleet-plans-v1";

/**
 * Fleet card > Plans (TEC-477): the caller's bulk service plans of the
 * fleet (GET /v1/fleets/{uuid}/service-plans: status facet, created /
 * start date sort); a row opens the plan detail (row-wise intake).
 */
export function FleetPlansTab({
  slug,
  fleet,
}: {
  slug: string;
  fleet: FleetCard;
}) {
  const { t, format } = useLocale();
  const router = useRouter();
  const openPlan = (plan: FleetPlanSummary) =>
    router.push(routes.tenant.fleets.plan(slug, fleet.uuid, plan.uuid));

  const columns = useMemo(
    () =>
      [
        createColumn<FleetPlanSummary>({
          accessorKey: "start_date",
          labelKey: "fleets.plan.start_date",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => format.date(row.original.start_date),
        }),
        createColumn<FleetPlanSummary>({
          accessorKey: "service_type",
          labelKey: "fleets.plan.service_type",
          enableSorting: false,
          gridSecondary: true,
        }),
        createColumn<FleetPlanSummary>({
          accessorKey: "status",
          labelKey: "fleets.plan.status",
          enableSorting: false,
          filterVariant: "faceted",
          param: "status",
          filterOptions: FLEET_PLAN_STATUSES.map((value) => ({
            value,
            label: value,
            labelKey: `fleets.plan.status_${value}`,
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(`fleets.plan.status_${row.original.status}`)}
              tone={row.original.status === "scheduled" ? "success" : "default"}
            />
          ),
        }),
        createColumn<FleetPlanSummary>({
          accessorKey: "appointment_count",
          labelKey: "fleets.plan.appointment_count",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {format.number(row.original.appointment_count)}
            </span>
          ),
        }),
        createColumn<FleetPlanSummary>({
          accessorKey: "intake_count",
          labelKey: "fleets.plan.intake_count",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {format.number(row.original.intake_count)}
            </span>
          ),
        }),
        createColumn<FleetPlanSummary>({
          accessorKey: "daily_vehicle_limit",
          labelKey: "fleets.plan.daily_max",
          enableSorting: false,
          defaultHidden: true,
        }),
        createColumn<FleetPlanSummary>({
          accessorKey: "created_at",
          labelKey: "fleets.plan.created_at",
          enableSorting: true,
          cell: ({ row }) => format.dateTime(row.original.created_at),
        }),
        createColumn<FleetPlanSummary>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "view",
                  label: t("common.view"),
                  icon: Eye,
                  onSelect: () => openPlan(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<FleetPlanSummary, unknown>[],
    // openPlan only closes over router, slug and the fleet uuid.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [fleet.uuid, format, router, slug, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: 20,
    persistKey: FLEET_PLANS_PERSIST_KEY,
  });
  const params = listState.params;
  const list = useQuery({
    queryKey: fleetKeys.plans(fleet.uuid, params),
    queryFn: () => fleetsService.listPlans(fleet.uuid, params),
  });

  return (
    <EntityTable
      columns={columns}
      data={list.data?.items ?? []}
      getRowId={(row) => row.uuid}
      onRowClick={openPlan}
      isLoading={list.isLoading}
      isError={list.isError}
      onRetry={() => void list.refetch()}
      emptyTitle={t("fleets.plan.empty_title")}
      emptyDescription={t("fleets.plan.empty_description")}
      rowCount={list.data?.total ?? 0}
      state={listState.tableState}
      features={{ persistKey: FLEET_PLANS_PERSIST_KEY, globalFilter: false }}
      toolbarExtra={
        <>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() =>
              router.push(routes.tenant.fleets.newPlan(slug, fleet.uuid))
            }
          >
            <CalendarPlus className="size-4" />
            {t("fleets.plan.create")}
          </Button>
          <EntityToolbar
            onRefresh={() => void list.refetch()}
            refreshDisabled={list.isFetching}
          />
        </>
      }
    />
  );
}
