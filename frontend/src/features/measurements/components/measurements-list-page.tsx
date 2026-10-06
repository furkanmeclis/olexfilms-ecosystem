"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Cpu, Eye, FileDown, Wrench } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  MeasurementStatusBadge,
  MeasurementVin,
} from "@/features/measurements/components/measurement-status-badge";
import { useMeasurementPdf } from "@/features/measurements/hooks/use-measurement-pdf";
import {
  measurementKeys,
  measurementsService,
  type MeasurementListQuery,
  type MeasurementSummary,
} from "@/features/measurements/services/measurements.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const MEASUREMENTS_PERSIST_KEY = "tenant-measurements-v1";

const dash = (v: string | null | undefined) => (v ? v : "—");

/**
 * Tenant > Measurements (TEC-299): the NexPTG paint measurements in the
 * reader's scope (GET /v1/measurements) as a server DataTable: sort, `q`
 * (VIN, plate, device serial), date range, status and device facets and
 * the linked / unlinked filter. A row opens the detail page.
 */
export function MeasurementsListPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const canRead = can(permissions.measurements.read);
  const canDevices = can(permissions.measurements.devicesManage);
  const canServices = can(permissions.services.read);
  const pdf = useMeasurementPdf();

  // The device facet needs the device registry (measurement_devices.manage).
  const devices = useQuery({
    queryKey: measurementKeys.devices,
    queryFn: () => measurementsService.listDevices(),
    enabled: canRead && canDevices,
    staleTime: 5 * 60_000,
  });
  const deviceOptions = useMemo(
    () =>
      (devices.data ?? []).map((d) => ({
        value: d.uuid,
        label: d.label ? `${d.label} (${d.serial})` : d.serial,
      })),
    [devices.data],
  );

  const columns = useMemo(
    () =>
      [
        createColumn<MeasurementSummary>({
          id: "measured_at",
          accessorFn: (row) => row.measured_at ?? row.created_at,
          labelKey: "measurements.columns.measured_at",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          filterVariant: "date-range",
          param: "measured",
          cell: ({ row }) => (
            <Link
              href={routes.tenant.measurements.detail(slug, row.original.uuid)}
              className="font-medium whitespace-nowrap hover:underline"
              data-testid="measurement-row"
              onClick={(event) => event.stopPropagation()}
            >
              {format.dateTime(
                row.original.measured_at ?? row.original.created_at,
              )}
            </Link>
          ),
        }),
        createColumn<MeasurementSummary>({
          accessorKey: "vin",
          labelKey: "measurements.columns.vin",
          enableSorting: true,
          cell: ({ row }) => <MeasurementVin vin={row.original.vin} />,
        }),
        createColumn<MeasurementSummary>({
          accessorKey: "plate",
          labelKey: "measurements.columns.plate",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="font-mono whitespace-nowrap" dir="ltr">
              {dash(row.original.plate)}
            </span>
          ),
        }),
        createColumn<MeasurementSummary>({
          id: "device",
          accessorFn: (row) => row.device?.uuid ?? "",
          labelKey: "measurements.columns.device",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: deviceOptions,
          enableColumnFilter: deviceOptions.length > 0,
          param: "device_uuid",
          cell: ({ row }) => {
            const d = row.original.device;
            const serial = d?.serial ?? row.original.device_serial;
            return (
              <div className="flex min-w-0 flex-col">
                <span className="font-mono text-xs" dir="ltr">
                  {dash(serial)}
                </span>
                {d?.label ? (
                  <span className="text-muted-foreground text-xs">
                    {d.label}
                  </span>
                ) : null}
              </div>
            );
          },
        }),
        createColumn<MeasurementSummary>({
          accessorKey: "status",
          labelKey: "measurements.columns.status",
          enableSorting: true,
          gridSecondary: true,
          filterVariant: "faceted",
          filterOptions: [
            {
              value: "accepted",
              label: t("measurements.status.accepted"),
            },
            {
              value: "vin_pending",
              label: t("measurements.status.vin_pending"),
            },
          ],
          param: "status",
          cell: ({ row }) => (
            <MeasurementStatusBadge status={row.original.status} />
          ),
        }),
        createColumn<MeasurementSummary>({
          id: "service",
          accessorFn: (row) => (row.service ? "true" : "false"),
          labelKey: "measurements.columns.service",
          enableSorting: false,
          filterVariant: "boolean",
          param: "linked",
          cell: ({ row }) => {
            const s = row.original.service;
            if (!s) {
              return (
                <span className="text-muted-foreground text-xs">
                  {t("measurements.service.unlinked")}
                </span>
              );
            }
            const label = (
              <span className="flex items-center gap-2 whitespace-nowrap">
                <span className="font-mono text-xs" dir="ltr">
                  {s.service_no}
                </span>
                <Badge variant="outline">
                  {t(`measurements.phase.${s.phase}`)}
                </Badge>
              </span>
            );
            return canServices ? (
              <Link
                href={routes.tenant.services.detail(slug, s.uuid)}
                className="hover:underline"
                onClick={(event) => event.stopPropagation()}
              >
                {label}
              </Link>
            ) : (
              label
            );
          },
        }),
        createColumn<MeasurementSummary>({
          id: "organization",
          accessorFn: (row) => row.organization.name,
          labelKey: "measurements.columns.organization",
          enableSorting: false,
          defaultHidden: true,
        }),
        createColumn<MeasurementSummary>({
          accessorKey: "created_at",
          labelKey: "measurements.columns.created_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {format.dateTime(row.original.created_at)}
            </span>
          ),
        }),
        createColumn<MeasurementSummary>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const m = row.original;
            const items: EntityRowAction[] = [
              {
                id: "open",
                label: t("common.open"),
                icon: Eye,
                onSelect: () =>
                  router.push(routes.tenant.measurements.detail(slug, m.uuid)),
              },
            ];
            if (m.status === "accepted") {
              items.push({
                id: "pdf",
                label: t("measurements.pdf.download"),
                icon: FileDown,
                onSelect: () => pdf.mutate(m.uuid),
              });
            }
            if (m.service && canServices) {
              items.push({
                id: "service",
                label: t("measurements.service.open"),
                icon: Wrench,
                onSelect: () =>
                  router.push(
                    routes.tenant.services.detail(slug, m.service!.uuid),
                  ),
              });
            }
            return <EntityRowActions actions={items} />;
          },
        }),
      ] as ColumnDef<MeasurementSummary, unknown>[],
    [canServices, deviceOptions, format, pdf, router, slug, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "-measured_at",
    persistKey: MEASUREMENTS_PERSIST_KEY,
  });
  const params: MeasurementListQuery = listState.params;

  const list = useQuery({
    queryKey: measurementKeys.list(params),
    queryFn: () => measurementsService.list(params),
    enabled: canRead,
  });

  const title = t("measurements.list.title");
  return (
    <EntityPage
      title={title}
      description={t("measurements.list.description")}
      permission={permissions.measurements.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("measurements.list.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: title },
      ]}
      actions={
        canDevices ? (
          <Button asChild variant="outline">
            <Link href={routes.tenant.measurements.devices(slug)}>
              <Cpu className="size-4" />
              {t("measurements.devices.title")}
            </Link>
          </Button>
        ) : null
      }
    >
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(routes.tenant.measurements.detail(slug, row.uuid))
        }
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("measurements.list.empty_title")}
        emptyDescription={t("measurements.list.empty_description")}
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{ persistKey: MEASUREMENTS_PERSIST_KEY }}
        renderGridItem={(m) => (
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0 space-y-1">
              <MeasurementVin vin={m.vin} />
              <p className="font-mono text-sm" dir="ltr">
                {dash(m.plate)}
              </p>
              <p className="text-muted-foreground text-xs">
                {format.dateTime(m.measured_at ?? m.created_at)}
              </p>
            </div>
            <MeasurementStatusBadge status={m.status} />
          </div>
        )}
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
