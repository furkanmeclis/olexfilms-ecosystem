"use client";

import type { ColumnDef, FilterFn } from "@tanstack/react-table";
import { Eye } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  CLIENT_SIDE_MANUAL,
  EntityRowActions,
  EntityTable,
  clientDateRangeFilter,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { routes } from "@/config/routes";
import {
  customerName,
  serviceStatusTone,
} from "@/features/services/lib/detail";
import { SERVICE_STATUSES } from "@/features/services/lib/list-filters";
import type { Service } from "@/features/services/services/service-wizard.service";
import {
  CLAIM_STATUSES,
  claimStatusTone,
  type WarrantyClaim,
} from "@/features/warranty-claims/lib/claims";
import { WarrantyProgressBar } from "@/features/warranty/components/warranty-progress";
import {
  WARRANTY_STATUSES,
  warrantyStatusTone,
  type Warranty,
} from "@/features/warranty/lib/warranty-list";
import { useLocale } from "@/providers/locale-provider";

export const VEHICLE_SERVICES_PERSIST_KEY = "tenant-vehicle-services-v1";
export const VEHICLE_WARRANTIES_PERSIST_KEY = "tenant-vehicle-warranties-v1";
export const VEHICLE_CLAIMS_PERSIST_KEY = "tenant-vehicle-claims-v1";

/** Distinct option list of a text column (faceted filter in the browser). */
function optionsOf<T>(rows: readonly T[], pick: (row: T) => string) {
  const seen = new Set<string>();
  for (const r of rows) {
    const v = pick(r);
    if (v) seen.add(v);
  }
  return [...seen]
    .sort((a, b) => a.localeCompare(b))
    .map((value) => ({ value, label: value }));
}

type TableProps<T> = {
  slug: string;
  rows: T[];
  isLoading?: boolean;
};

/**
 * Services of one vehicle (TEC-863): GET /v1/services?vehicle_uuid in the
 * services.read scope, one page, sorted / searched / filtered in the browser
 * (nested client-side DataTable, like the customer's vehicles).
 */
export function VehicleServicesTable({
  slug,
  rows,
  isLoading,
}: TableProps<Service>) {
  const { t, format } = useLocale();
  const router = useRouter();
  const orgOptions = useMemo(
    () => optionsOf(rows, (r) => r.organization.name),
    [rows],
  );

  const columns = useMemo(
    () =>
      [
        createColumn<Service>({
          accessorKey: "service_no",
          labelKey: "services.list.columns.service_no",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <Link
              href={routes.tenant.services.detail(slug, row.original.uuid)}
              className="font-mono font-medium whitespace-nowrap hover:underline"
              dir="ltr"
              data-testid="vehicle-service-row"
              onClick={(event) => event.stopPropagation()}
            >
              {row.original.service_no}
            </Link>
          ),
        }),
        createColumn<Service>({
          accessorKey: "status",
          labelKey: "services.list.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: SERVICE_STATUSES.map((value) => ({
            value,
            label: value,
            labelKey: `services.status.${value}`,
          })),
          cell: ({ row }) => (
            <StatusChip
              label={row.original.status_label}
              tone={serviceStatusTone(row.original.status)}
            />
          ),
        }),
        createColumn<Service>({
          id: "package",
          accessorFn: (row) => row.package ?? "",
          labelKey: "services.detail.package",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => row.original.package || "—",
        }),
        createColumn<Service>({
          id: "organization",
          accessorFn: (row) => row.organization.name,
          labelKey: "services.list.columns.organization",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: orgOptions,
          enableColumnFilter: orgOptions.length > 1,
          cell: ({ row }) => row.original.organization.name,
        }),
        createColumn<Service>({
          id: "customer",
          accessorFn: (row) => customerName(row),
          labelKey: "services.list.columns.customer",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => customerName(row.original) || "—",
        }),
        createColumn<Service>({
          accessorKey: "created_at",
          labelKey: "services.list.columns.created_at",
          enableSorting: true,
          filterVariant: "date-range",
          filterFn: clientDateRangeFilter as FilterFn<Service>,
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {format.date(row.original.created_at)}
            </span>
          ),
        }),
        createColumn<Service>({
          id: "completed_at",
          accessorFn: (row) => row.completed_at ?? "",
          labelKey: "services.list.columns.completed_at",
          enableSorting: true,
          sortUndefined: "last",
          filterVariant: "date-range",
          filterFn: clientDateRangeFilter as FilterFn<Service>,
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {row.original.completed_at
                ? format.date(row.original.completed_at)
                : "—"}
            </span>
          ),
        }),
        createColumn<Service>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "open",
                  label: t("common.open"),
                  icon: Eye,
                  onSelect: () =>
                    router.push(
                      routes.tenant.services.detail(slug, row.original.uuid),
                    ),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<Service, unknown>[],
    [format, orgOptions, router, slug, t],
  );

  return (
    <div data-testid="vehicle-services">
      <EntityTable
        columns={columns}
        data={rows}
        getRowId={(row) => row.uuid}
        manual={CLIENT_SIDE_MANUAL}
        isLoading={isLoading}
        onRowClick={(s) =>
          router.push(routes.tenant.services.detail(slug, s.uuid))
        }
        initialState={{
          pagination: { pageIndex: 0, pageSize: 10 },
          sorting: [{ id: "created_at", desc: true }],
        }}
        emptyTitle={t("vehicles.detail.services_empty")}
        emptyDescription=""
        features={{ persistKey: VEHICLE_SERVICES_PERSIST_KEY }}
      />
    </div>
  );
}

/**
 * Warranties of one vehicle (TEC-863): GET /v1/warranties?vehicle_uuid in
 * the warranties.read scope, client-side table.
 */
export function VehicleWarrantiesTable({
  slug,
  rows,
  isLoading,
}: TableProps<Warranty>) {
  const { t, format } = useLocale();
  const router = useRouter();
  const orgOptions = useMemo(
    () => optionsOf(rows, (r) => r.organization.name),
    [rows],
  );

  const columns = useMemo(
    () =>
      [
        createColumn<Warranty>({
          accessorKey: "public_code",
          labelKey: "warranty.list.columns.code",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <Link
              href={routes.tenant.warranties.detail(slug, row.original.uuid)}
              className="font-mono font-medium whitespace-nowrap hover:underline"
              dir="ltr"
              data-testid="vehicle-warranty-row"
              onClick={(event) => event.stopPropagation()}
            >
              {row.original.public_code}
            </Link>
          ),
        }),
        createColumn<Warranty>({
          accessorKey: "status",
          labelKey: "warranty.list.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: WARRANTY_STATUSES.map((value) => ({
            value,
            label: value,
            labelKey: `warranty.status.${value}`,
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(`warranty.status.${row.original.status}`)}
              tone={warrantyStatusTone(row.original.status)}
            />
          ),
        }),
        createColumn<Warranty>({
          id: "product",
          accessorFn: (row) => row.product.name,
          labelKey: "warranty.list.columns.product",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => row.original.product.name,
        }),
        createColumn<Warranty>({
          id: "service_no",
          accessorFn: (row) => row.service.service_no,
          labelKey: "warranty.list.columns.service_no",
          enableSorting: true,
          cell: ({ row }) => (
            <Link
              href={routes.tenant.services.detail(
                slug,
                row.original.service.uuid,
              )}
              className="font-mono text-xs hover:underline"
              dir="ltr"
              onClick={(event) => event.stopPropagation()}
            >
              {row.original.service.service_no}
            </Link>
          ),
        }),
        createColumn<Warranty>({
          id: "organization",
          accessorFn: (row) => row.organization.name,
          labelKey: "warranty.list.columns.organization",
          enableSorting: true,
          defaultHidden: true,
          filterVariant: "faceted",
          filterOptions: orgOptions,
          enableColumnFilter: orgOptions.length > 1,
          cell: ({ row }) => row.original.organization.name,
        }),
        createColumn<Warranty>({
          id: "expiry",
          accessorFn: (row) => row.end_at,
          labelKey: "warranty.list.columns.days_left",
          enableSorting: true,
          cell: ({ row }) => (
            <div className="min-w-[10rem]">
              <WarrantyProgressBar warranty={row.original} compact />
            </div>
          ),
        }),
        createColumn<Warranty>({
          accessorKey: "start_at",
          labelKey: "warranty.detail.start",
          enableSorting: true,
          filterVariant: "date-range",
          filterFn: clientDateRangeFilter as FilterFn<Warranty>,
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {format.date(row.original.start_at)}
            </span>
          ),
        }),
        createColumn<Warranty>({
          accessorKey: "end_at",
          labelKey: "warranty.detail.end",
          enableSorting: true,
          filterVariant: "date-range",
          filterFn: clientDateRangeFilter as FilterFn<Warranty>,
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {format.date(row.original.end_at)}
            </span>
          ),
        }),
        createColumn<Warranty>({
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
                  onSelect: () =>
                    router.push(
                      routes.tenant.warranties.detail(slug, row.original.uuid),
                    ),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<Warranty, unknown>[],
    [format, orgOptions, router, slug, t],
  );

  return (
    <div data-testid="vehicle-warranties">
      <EntityTable
        columns={columns}
        data={rows}
        getRowId={(row) => row.uuid}
        manual={CLIENT_SIDE_MANUAL}
        isLoading={isLoading}
        onRowClick={(w) =>
          router.push(routes.tenant.warranties.detail(slug, w.uuid))
        }
        initialState={{
          pagination: { pageIndex: 0, pageSize: 10 },
          sorting: [{ id: "end_at", desc: true }],
        }}
        emptyTitle={t("vehicles.detail.warranties_empty")}
        emptyDescription=""
        features={{ persistKey: VEHICLE_WARRANTIES_PERSIST_KEY }}
      />
    </div>
  );
}

/**
 * Warranty claims of one vehicle (TEC-863): GET
 * /v1/warranty-claims?vehicle_uuid in the warranty_claims.read scope.
 */
export function VehicleClaimsTable({
  slug,
  rows,
  isLoading,
}: TableProps<WarrantyClaim>) {
  const { t, format } = useLocale();
  const router = useRouter();

  const columns = useMemo(
    () =>
      [
        createColumn<WarrantyClaim>({
          accessorKey: "claim_no",
          labelKey: "warranty.claims.columns.claim_no",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <Link
              href={routes.tenant.warrantyClaims.detail(
                slug,
                row.original.uuid,
              )}
              className="font-mono text-xs font-medium whitespace-nowrap hover:underline"
              dir="ltr"
              data-testid="vehicle-claim-row"
              onClick={(event) => event.stopPropagation()}
            >
              #{row.original.claim_no}
            </Link>
          ),
        }),
        createColumn<WarrantyClaim>({
          accessorKey: "status",
          labelKey: "warranty.claims.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: CLAIM_STATUSES.map((value) => ({
            value,
            label: value,
            labelKey: `warranty.claims.status.${value}`,
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(`warranty.claims.status.${row.original.status}`)}
              tone={claimStatusTone(row.original.status)}
            />
          ),
        }),
        createColumn<WarrantyClaim>({
          id: "product",
          accessorFn: (row) => row.product_name ?? "",
          labelKey: "warranty.claims.columns.product",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => row.original.product_name || "—",
        }),
        createColumn<WarrantyClaim>({
          id: "warranty_no",
          accessorFn: (row) => row.warranty_no ?? "",
          labelKey: "warranty.claims.columns.warranty_no",
          enableSorting: true,
          cell: ({ row }) => (
            <Link
              href={routes.tenant.warranties.detail(
                slug,
                row.original.warranty_uuid,
              )}
              className="font-mono text-xs hover:underline"
              dir="ltr"
              onClick={(event) => event.stopPropagation()}
            >
              {row.original.warranty_no || "—"}
            </Link>
          ),
        }),
        createColumn<WarrantyClaim>({
          id: "organization",
          accessorFn: (row) => row.organization_name ?? "",
          labelKey: "warranty.claims.columns.organization",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => row.original.organization_name || "—",
        }),
        createColumn<WarrantyClaim>({
          accessorKey: "created_at",
          labelKey: "warranty.claims.columns.created_at",
          enableSorting: true,
          filterVariant: "date-range",
          filterFn: clientDateRangeFilter as FilterFn<WarrantyClaim>,
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {format.date(row.original.created_at)}
            </span>
          ),
        }),
        createColumn<WarrantyClaim>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "open",
                  label: t("common.open"),
                  icon: Eye,
                  onSelect: () =>
                    router.push(
                      routes.tenant.warrantyClaims.detail(
                        slug,
                        row.original.uuid,
                      ),
                    ),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<WarrantyClaim, unknown>[],
    [format, router, slug, t],
  );

  return (
    <div data-testid="vehicle-claims">
      <EntityTable
        columns={columns}
        data={rows}
        getRowId={(row) => row.uuid}
        manual={CLIENT_SIDE_MANUAL}
        isLoading={isLoading}
        onRowClick={(c) =>
          router.push(routes.tenant.warrantyClaims.detail(slug, c.uuid))
        }
        initialState={{
          pagination: { pageIndex: 0, pageSize: 10 },
          sorting: [{ id: "created_at", desc: true }],
        }}
        emptyTitle={t("vehicles.detail.claims_empty")}
        emptyDescription=""
        features={{ persistKey: VEHICLE_CLAIMS_PERSIST_KEY }}
      />
    </div>
  );
}
