"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Eye } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useScopeOrganizationOptions } from "@/features/services/lib/use-scope-organizations";
import {
  CLAIM_STATUSES,
  claimStatusTone,
  type WarrantyClaim,
} from "@/features/warranty-claims/lib/claims";
import {
  claimKeys,
  claimsService,
  type WarrantyClaimServerQuery,
} from "@/features/warranty-claims/services/claims.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const CLAIMS_PAGE_SIZE = 20;
export const CLAIMS_PERSIST_KEY = "tenant-warranty-claims-v1";
/** Backend default (TEC-377): newest first. */
export const CLAIMS_DEFAULT_SORT = "-created_at";

/**
 * Tenant > Warranty claims (TEC-339): server DataTable over
 * GET /v1/warranty-claims with single-field sort (claim no, status flow,
 * created / updated), status facet, organization (center / distributor),
 * created date range and `q` (description, service no, claim no).
 * Mobile shows cards. The list has no export or bulk endpoint.
 */
export function WarrantyClaimsListPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const canRead = can(Permission.WarrantyClaimsRead);
  const orgFilter = useScopeOrganizationOptions(slug, canRead);

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
              data-testid="claim-row"
              data-uuid={row.original.uuid}
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
          param: "status",
          cell: ({ row }) => (
            <StatusChip
              label={t(`warranty.claims.status.${row.original.status}`)}
              tone={claimStatusTone(row.original.status)}
            />
          ),
        }),
        createColumn<WarrantyClaim>({
          id: "organization",
          accessorFn: (row) => row.organization_name ?? "",
          labelKey: "warranty.claims.columns.organization",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: orgFilter.options,
          enableColumnFilter: orgFilter.enabled,
          param: "organization_uuid",
          cell: ({ row }) => row.original.organization_name || "—",
        }),
        createColumn<WarrantyClaim>({
          id: "warranty_no",
          accessorFn: (row) => row.warranty_no ?? "",
          labelKey: "warranty.claims.columns.warranty_no",
          enableSorting: false,
          cell: ({ row }) => (
            <Link
              href={routes.tenant.warranties.detail(
                slug,
                row.original.warranty_uuid,
              )}
              className="font-mono text-xs whitespace-nowrap hover:underline"
              dir="ltr"
              onClick={(event) => event.stopPropagation()}
            >
              {row.original.warranty_no || "—"}
            </Link>
          ),
        }),
        createColumn<WarrantyClaim>({
          id: "product",
          accessorFn: (row) => row.product_name ?? "",
          labelKey: "warranty.claims.columns.product",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => row.original.product_name || "—",
        }),
        createColumn<WarrantyClaim>({
          id: "service_no",
          accessorFn: (row) => row.service_no ?? "",
          labelKey: "warranty.claims.columns.service_no",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="font-mono text-xs" dir="ltr">
              {row.original.service_no || "—"}
              {row.original.plate ? ` · ${row.original.plate}` : ""}
            </span>
          ),
        }),
        createColumn<WarrantyClaim>({
          accessorKey: "created_at",
          labelKey: "warranty.claims.columns.created_at",
          enableSorting: true,
          filterVariant: "date-range",
          param: "created",
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {format.date(row.original.created_at)}
            </span>
          ),
        }),
        createColumn<WarrantyClaim>({
          accessorKey: "updated_at",
          labelKey: "warranty.claims.columns.updated_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {format.date(row.original.updated_at)}
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
                  id: "view",
                  label: t("common.view"),
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
    [format, orgFilter.enabled, orgFilter.options, router, slug, t],
  );

  // Column meta drives the params: status / organization_uuid (CSV),
  // created_from / created_to (days); sort is one backend field.
  const listState = useServerListState({
    columns,
    initialSort: CLAIMS_DEFAULT_SORT,
    initialPageSize: CLAIMS_PAGE_SIZE,
    persistKey: CLAIMS_PERSIST_KEY,
  });
  const params: WarrantyClaimServerQuery = listState.params;
  const list = useQuery({
    queryKey: claimKeys.list(params),
    queryFn: () => claimsService.list(params),
    enabled: canRead,
  });
  const filtered =
    listState.tableState.columnFilters.length > 0 || Boolean(params.q);
  const open = (c: WarrantyClaim) =>
    router.push(routes.tenant.warrantyClaims.detail(slug, c.uuid));

  const title = t("warranty.claims.title");
  return (
    <EntityPage
      title={title}
      description={t("warranty.claims.description")}
      permission={Permission.WarrantyClaimsRead}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("warranty.claims.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: title },
      ]}
    >
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={open}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("warranty.claims.empty_title")}
        emptyDescription={
          filtered
            ? t("warranty.claims.empty_filtered")
            : t("warranty.claims.empty_description")
        }
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{ persistKey: CLAIMS_PERSIST_KEY }}
        renderGridItem={(c) => (
          <div className="space-y-2" data-testid="claim-card">
            <div className="flex items-start justify-between gap-2">
              <div className="min-w-0">
                <span className="font-mono text-xs font-medium" dir="ltr">
                  #{c.claim_no} · {c.warranty_no}
                </span>
                <p className="font-medium">{c.product_name}</p>
              </div>
              <StatusChip
                label={t(`warranty.claims.status.${c.status}`)}
                tone={claimStatusTone(c.status)}
              />
            </div>
            <p className="text-muted-foreground text-xs">
              {format.date(c.created_at)} · {c.organization_name}
            </p>
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
