"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { CalendarPlus, Eye, Receipt } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityCreateButton,
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { NewFleetDialog } from "@/features/fleets/components/new-fleet-dialog";
import {
  FLEET_LINK_STATUSES,
  fleetKeys,
  fleetsService,
  type FleetLinkStatus,
  type FleetListItem,
} from "@/features/fleets/services/fleets.service";
import { ExportMenu } from "@/features/io/components/export-menu";
import { useFeature } from "@/features/modules/hooks/use-features";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const FLEETS_PERSIST_KEY = "tenant-fleets-v1";
export const FLEETS_EXPORT_PATH = "/v1/fleets/export";

export function linkTone(status: FleetLinkStatus | string) {
  if (status === "active") return "success" as const;
  if (status === "pending") return "warning" as const;
  return "default" as const;
}

/**
 * Tenant > Fleets (TEC-477): server DataTable over GET /v1/fleets (one row
 * per link): name, VKN, vehicle count (range filter), last service, link
 * status (faceted), `q`, single-field sort, row actions and the list
 * export. "New fleet" looks the VKN up first (link request or opening
 * form).
 */
export function FleetsListPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const [newOpen, setNewOpen] = useState(false);
  const appointments = useFeature(slug, "appointments");
  const canPlan = can(permissions.fleets.plan) && appointments.enabled;

  const openCard = (uuid: string, tab?: string) =>
    router.push(routes.tenant.fleets.detail(slug, uuid, tab));

  const columns = useMemo(
    () =>
      [
        createColumn<FleetListItem>({
          accessorKey: "name",
          labelKey: "fleets.columns.name",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="min-w-0">
              <Link
                href={routes.tenant.fleets.detail(slug, row.original.uuid)}
                className="font-medium hover:underline"
                data-testid="fleet-row"
                onClick={(event) => event.stopPropagation()}
              >
                {row.original.name}
              </Link>
              {row.original.legal_name !== row.original.name ? (
                <div className="text-muted-foreground truncate text-xs">
                  {row.original.legal_name}
                </div>
              ) : null}
            </div>
          ),
        }),
        createColumn<FleetListItem>({
          accessorKey: "tax_number",
          labelKey: "fleets.columns.tax_number",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="font-mono" dir="ltr">
              {row.original.tax_number}
            </span>
          ),
        }),
        createColumn<FleetListItem>({
          accessorKey: "vehicle_count",
          labelKey: "fleets.columns.vehicle_count",
          enableSorting: true,
          filterVariant: "number-range",
          param: "vehicle_count",
          cell: ({ row }) => (
            <span className="tabular-nums">
              {format.number(row.original.vehicle_count)}
            </span>
          ),
        }),
        createColumn<FleetListItem>({
          accessorKey: "last_service_at",
          labelKey: "fleets.columns.last_service_at",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.last_service_at
              ? format.date(row.original.last_service_at)
              : "—",
        }),
        createColumn<FleetListItem>({
          id: "status",
          accessorFn: (row) => row.link.status,
          labelKey: "fleets.columns.link_status",
          enableSorting: false,
          filterVariant: "faceted",
          param: "status",
          filterOptions: FLEET_LINK_STATUSES.map((value) => ({
            value,
            label: value,
            labelKey: `fleets.link_status.${value}`,
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(`fleets.link_status.${row.original.link.status}`)}
              tone={linkTone(row.original.link.status)}
            />
          ),
        }),
        createColumn<FleetListItem>({
          id: "dealer",
          accessorFn: (row) => row.link.dealer_name,
          labelKey: "fleets.columns.dealer",
          enableSorting: false,
          defaultHidden: true,
        }),
        createColumn<FleetListItem>({
          id: "created_at",
          accessorFn: (row) => row.link.created_at,
          labelKey: "fleets.columns.linked_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => format.date(row.original.link.created_at),
        }),
        createColumn<FleetListItem>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const f = row.original;
            const active = f.link.status === "active";
            const items: EntityRowAction[] = [
              {
                id: "view",
                label: t("common.view"),
                icon: Eye,
                disabled: !active,
                onSelect: () => openCard(f.uuid),
              },
              {
                id: "statement",
                label: t("fleets.tabs.statement"),
                icon: Receipt,
                disabled: !active,
                onSelect: () => openCard(f.uuid, "statement"),
              },
            ];
            if (canPlan) {
              items.push({
                id: "plan",
                label: t("fleets.plan.create"),
                icon: CalendarPlus,
                disabled: !active,
                onSelect: () =>
                  router.push(routes.tenant.fleets.newPlan(slug, f.uuid)),
              });
            }
            return <EntityRowActions actions={items} />;
          },
        }),
      ] as ColumnDef<FleetListItem, unknown>[],
    // openCard only closes over router and slug.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [canPlan, format, router, slug, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "name",
    initialPageSize: 20,
    persistKey: FLEETS_PERSIST_KEY,
  });
  const params = listState.params;
  const list = useQuery({
    queryKey: fleetKeys.list(params),
    queryFn: () => fleetsService.list(params),
    enabled: can(permissions.fleets.read),
  });
  const exportQuery = useMemo(
    () => ({ ...listState.filterParams, q: params.q, sort: params.sort }),
    [listState.filterParams, params.q, params.sort],
  );
  const filtered = listState.columnFilters.length > 0 || Boolean(params.q);

  const title = t("fleets.title");
  return (
    <EntityPage
      title={title}
      description={t("fleets.description")}
      permission={permissions.fleets.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("fleets.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: title },
      ]}
      actions={
        <EntityCreateButton
          label={t("fleets.new.title")}
          permission={permissions.fleets.manage}
          onClick={() => setNewOpen(true)}
        />
      }
    >
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.link.uuid}
        onRowClick={(row) => {
          if (row.link.status === "active") openCard(row.uuid);
        }}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("fleets.empty_title")}
        emptyDescription={
          filtered ? t("fleets.empty_filtered") : t("fleets.empty_description")
        }
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{ persistKey: FLEETS_PERSIST_KEY }}
        renderGridItem={(f) => (
          <div className="space-y-1" data-testid="fleet-card">
            <div className="flex items-start justify-between gap-2">
              <span className="font-medium">{f.name}</span>
              <StatusChip
                label={t(`fleets.link_status.${f.link.status}`)}
                tone={linkTone(f.link.status)}
              />
            </div>
            <p className="text-muted-foreground font-mono text-xs" dir="ltr">
              {f.tax_number}
            </p>
            <p className="text-muted-foreground text-xs">
              {t("fleets.vehicle_count", { count: f.vehicle_count })}
              {f.last_service_at
                ? ` · ${format.date(f.last_service_at)}`
                : null}
            </p>
          </div>
        )}
        toolbarExtra={
          <>
            <ExportMenu
              exportPath={FLEETS_EXPORT_PATH}
              query={exportQuery}
              formats={["xlsx", "csv", "pdf"]}
              jobsHref={routes.tenant.exports.root(slug)}
            />
            <EntityToolbar
              onRefresh={() => void list.refetch()}
              refreshDisabled={list.isFetching}
            />
          </>
        }
      />
      <NewFleetDialog
        open={newOpen}
        onOpenChange={setNewOpen}
        onOpened={(uuid) => openCard(uuid)}
      />
    </EntityPage>
  );
}
