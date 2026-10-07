"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Eye, FileDown, Wand2 } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo } from "react";

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
import { useServiceMoney } from "@/features/services/components/service-income";
import { downloadServicePdf } from "@/features/services/components/service-pdf-button";
import {
  canContinueWizard,
  resolveServiceListAccess,
} from "@/features/services/lib/access";
import {
  customerName,
  serviceStatusTone,
  vehicleTitle,
} from "@/features/services/lib/detail";
import { SERVICE_STATUSES } from "@/features/services/lib/list-filters";
import { useScopeOrganizationOptions } from "@/features/services/lib/use-scope-organizations";
import {
  SERVICES_EXPORT_PATH,
  serviceWizardKeys,
  serviceWizardService,
  type Service,
  type ServiceListQuery,
} from "@/features/services/services/service-wizard.service";
import { ExportMenu } from "@/features/io/components/export-menu";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const SERVICE_PAGE_SIZE = 20;
export const SERVICES_PERSIST_KEY = "tenant-services-v1";

/**
 * Tenant > Services (TEC-183, TEC-378): server DataTable over
 * GET /v1/services with single-field sort, status facet, created and
 * completed date ranges, organization filter (center / distributor), `q`
 * (number, plate, VIN, customer name or phone), row actions (open, continue
 * a draft in the wizard, PDF) and the list export with the same query.
 * Mobile shows cards.
 */
export function ServicesListPage({ slug }: { slug: string }) {
  const { t, format, locale } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const access = resolveServiceListAccess(can);
  const orgFilter = useScopeOrganizationOptions(slug, access.canRead);
  const money = useServiceMoney();
  // TEC-343: the API sends income / profit only to accounting readers.
  const showIncome =
    can(permissions.accounting.read) || can(permissions.accounting.write);

  const open = (s: Service) =>
    router.push(routes.tenant.services.detail(slug, s.uuid));

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
              data-testid="service-row"
              data-uuid={row.original.uuid}
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
          param: "status",
          cell: ({ row }) => (
            <StatusChip
              label={row.original.status_label}
              tone={serviceStatusTone(row.original.status)}
            />
          ),
        }),
        createColumn<Service>({
          id: "customer",
          accessorFn: (row) => customerName(row),
          labelKey: "services.list.columns.customer",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => customerName(row.original) || "—",
        }),
        createColumn<Service>({
          id: "plate",
          accessorFn: (row) => row.plate ?? "",
          labelKey: "services.list.columns.vehicle",
          enableSorting: true,
          cell: ({ row }) => (
            <div className="min-w-0">
              <div>{vehicleTitle(row.original) || "—"}</div>
              {row.original.plate ? (
                <div
                  className="text-muted-foreground font-mono text-xs"
                  dir="ltr"
                >
                  {row.original.plate}
                </div>
              ) : null}
            </div>
          ),
        }),
        createColumn<Service>({
          id: "organization",
          accessorFn: (row) => row.organization.name,
          labelKey: "services.list.columns.organization",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: orgFilter.options,
          enableColumnFilter: orgFilter.enabled,
          param: "organization_uuid",
          cell: ({ row }) => row.original.organization.name,
        }),
        createColumn<Service>({
          accessorKey: "package",
          labelKey: "services.detail.package",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.package || "—",
        }),
        createColumn<Service>({
          accessorKey: "created_at",
          labelKey: "services.list.columns.created_at",
          enableSorting: true,
          filterVariant: "date-range",
          param: "created",
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {format.dateTime(row.original.created_at)}
            </span>
          ),
        }),
        createColumn<Service>({
          accessorKey: "completed_at",
          labelKey: "services.list.columns.completed_at",
          enableSorting: true,
          defaultHidden: true,
          filterVariant: "date-range",
          param: "completed",
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {row.original.completed_at
                ? format.dateTime(row.original.completed_at)
                : "—"}
            </span>
          ),
        }),
        createColumn<Service>({
          accessorKey: "updated_at",
          labelKey: "services.list.columns.updated_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {format.dateTime(row.original.updated_at)}
            </span>
          ),
        }),
        ...(showIncome
          ? [
              createColumn<Service>({
                id: "income_amount",
                accessorFn: (row) => row.income_amount ?? "",
                labelKey: "services.list.columns.income",
                enableSorting: false,
                enableColumnFilter: false,
                defaultHidden: true,
                cell: ({ row }) => (
                  <span className="tabular-nums" dir="ltr">
                    {money(row.original.income_amount)}
                  </span>
                ),
              }),
              createColumn<Service>({
                id: "gross_profit",
                accessorFn: (row) => row.profit?.gross_profit ?? "",
                labelKey: "services.list.columns.profit",
                enableSorting: false,
                enableColumnFilter: false,
                defaultHidden: true,
                cell: ({ row }) => (
                  <span className="tabular-nums" dir="ltr">
                    {money(row.original.profit?.gross_profit)}
                  </span>
                ),
              }),
              createColumn<Service>({
                id: "margin_pct",
                accessorFn: (row) => row.profit?.margin_pct ?? "",
                labelKey: "services.list.columns.margin",
                enableSorting: false,
                enableColumnFilter: false,
                defaultHidden: true,
                cell: ({ row }) =>
                  row.original.profit?.margin_pct ? (
                    <span className="tabular-nums" dir="ltr">
                      %{money(row.original.profit.margin_pct)}
                    </span>
                  ) : (
                    "—"
                  ),
              }),
            ]
          : []),
        createColumn<Service>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const s = row.original;
            const items: EntityRowAction[] = [
              {
                id: "view",
                label: t("common.view"),
                icon: Eye,
                onSelect: () =>
                  router.push(routes.tenant.services.detail(slug, s.uuid)),
              },
            ];
            if (canContinueWizard(can, s)) {
              items.push({
                id: "wizard",
                label: t("services.detail.continue_wizard"),
                icon: Wand2,
                onSelect: () =>
                  router.push(routes.tenant.services.wizard(slug, s.uuid)),
              });
            }
            items.push({
              id: "pdf",
              label: t("services.detail.pdf"),
              icon: FileDown,
              onSelect: () =>
                void downloadServicePdf({
                  serviceUuid: s.uuid,
                  serviceNo: s.service_no,
                  locale,
                  t,
                }),
            });
            return <EntityRowActions actions={items} />;
          },
        }),
      ] as ColumnDef<Service, unknown>[],
    [
      can,
      format,
      locale,
      money,
      orgFilter.enabled,
      orgFilter.options,
      router,
      showIncome,
      slug,
      t,
    ],
  );

  // Column meta drives the params: status / organization_uuid (CSV),
  // created_* / completed_* (days); sort is one backend field.
  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: SERVICE_PAGE_SIZE,
    persistKey: SERVICES_PERSIST_KEY,
  });
  const params: ServiceListQuery = listState.params;

  const list = useQuery({
    queryKey: serviceWizardKeys.list(params),
    queryFn: () => serviceWizardService.listServices(params),
    enabled: access.canRead,
  });
  const total = list.data?.total ?? 0;

  // The export runs on the same filters, search and sort as the list.
  const exportQuery = useMemo(
    () => ({ ...listState.filterParams, q: params.q, sort: params.sort }),
    [listState.filterParams, params.q, params.sort],
  );
  const filtered = listState.columnFilters.length > 0 || Boolean(params.q);

  const title = t("services.list.title");
  return (
    <EntityPage
      title={title}
      description={t("services.list.description")}
      permission={permissions.services.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("services.list.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: title },
      ]}
      actions={
        access.canCreate ? (
          <EntityCreateButton
            label={t("services.nav_new")}
            onClick={() => router.push(routes.tenant.services.create(slug))}
          />
        ) : null
      }
    >
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={open}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("services.list.empty_title")}
        emptyDescription={
          filtered
            ? t("services.list.empty_filtered")
            : t("services.list.empty_description")
        }
        rowCount={total}
        state={listState.tableState}
        features={{ persistKey: SERVICES_PERSIST_KEY }}
        renderGridItem={(s) => (
          <div className="space-y-2" data-testid="service-card">
            <div className="flex items-start justify-between gap-2">
              <Link
                href={routes.tenant.services.detail(slug, s.uuid)}
                className="font-mono font-medium hover:underline"
                dir="ltr"
                onClick={(event) => event.stopPropagation()}
              >
                {s.service_no}
              </Link>
              <StatusChip
                label={s.status_label}
                tone={serviceStatusTone(s.status)}
              />
            </div>
            <p className="text-sm">
              {vehicleTitle(s) || "—"}
              {s.plate ? (
                <span className="ms-2 font-mono" dir="ltr">
                  {s.plate}
                </span>
              ) : null}
            </p>
            <p className="text-muted-foreground text-xs">
              {customerName(s) || "—"} · {s.organization.name}
            </p>
            <p className="text-muted-foreground text-xs">
              {format.dateTime(s.created_at)}
            </p>
          </div>
        )}
        toolbarExtra={
          <>
            <ExportMenu
              exportPath={SERVICES_EXPORT_PATH}
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
    </EntityPage>
  );
}
