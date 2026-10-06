"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { ArrowUpCircle, Download, Eye, Pencil, ShieldOff } from "lucide-react";
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
import {
  AnonymizeDialog,
  DataExportDialog,
  UpgradeDialog,
  type CustomerActionKind,
} from "@/features/customers/components/customer-actions";
import { CustomerListExportButton } from "@/features/customers/components/customer-list-export";
import {
  resolveCustomerDetailAccess,
  resolveCustomerListAccess,
} from "@/features/customers/lib/access";
import { customerDisplayName } from "@/features/customers/lib/form";
import {
  customerKeys,
  customersService,
  type CustomerListQuery,
  type CustomerStatus,
  type CustomerSummary,
  type ListExportQuery,
} from "@/features/customers/services/customers.service";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const CUSTOMER_PAGE_SIZE = 20;
export const CUSTOMERS_PERSIST_KEY = "tenant-customers-v1";
const STATUSES: CustomerStatus[] = [
  "active",
  "pending",
  "disabled",
  "anonymized",
];
const TYPES = ["individual", "corporate"] as const;

export function customerStatusTone(
  status: CustomerStatus,
): "default" | "success" | "warning" | "danger" {
  switch (status) {
    case "active":
      return "success";
    case "pending":
      return "warning";
    case "disabled":
      return "danger";
    default:
      return "default";
  }
}

/**
 * The list row has no `editable` flag: anonymized and disabled accounts
 * are not editable (merged accounts leave the list), like the detail.
 */
function summaryAccessInput(c: CustomerSummary) {
  return {
    editable:
      !c.anonymized && c.status !== "anonymized" && c.status !== "disabled",
    anonymized: c.anonymized,
    status: c.status,
  };
}

/**
 * Tenant > Customers (TEC-163, TEC-372): server DataTable over
 * GET /v1/customers with sort, status / type facets, linked date range,
 * organization filter (center / distributor), `q`, the detail actions per
 * row and the list export with the same filters and sort. Mobile shows
 * cards.
 */
export function CustomersListPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const qc = useQueryClient();
  const org = useActiveOrganization(slug);
  const access = resolveCustomerListAccess(can);
  const [action, setAction] = useState<{
    kind: CustomerActionKind;
    customer: CustomerSummary;
  } | null>(null);

  // Center and distributor see several organizations; a dealer only its own.
  const canFilterOrg =
    access.canRead &&
    can(permissions.organizations.tenantRead) &&
    (org?.type === "center" || org?.type === "distributor");
  const organizations = useQuery({
    queryKey: customerKeys.organizations,
    queryFn: () => customersService.listOrganizations(),
    enabled: canFilterOrg,
    staleTime: 5 * 60_000,
  });
  const orgOptions = useMemo(
    () =>
      (organizations.data ?? []).map((o) => ({ value: o.uuid, label: o.name })),
    [organizations.data],
  );

  const orgType = org?.type;
  const columns = useMemo(
    () =>
      [
        createColumn<CustomerSummary>({
          id: "name",
          accessorFn: (row) => customerDisplayName(row),
          labelKey: "customers.fields.name",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="min-w-0" data-testid="customer-row">
              <Link
                href={routes.tenant.customers.detail(slug, row.original.uuid)}
                className="font-medium hover:underline"
                onClick={(event) => event.stopPropagation()}
              >
                {customerDisplayName(row.original)}
              </Link>
              {row.original.type === "corporate" &&
              row.original.company_name ? (
                <p className="text-muted-foreground text-xs">
                  {row.original.company_name}
                </p>
              ) : null}
            </div>
          ),
        }),
        createColumn<CustomerSummary>({
          accessorKey: "phone",
          labelKey: "customers.fields.phone",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span dir="ltr" className="whitespace-nowrap">
              {row.original.phone ?? "—"}
            </span>
          ),
        }),
        createColumn<CustomerSummary>({
          accessorKey: "email",
          labelKey: "customers.fields.email",
          enableSorting: true,
          cell: ({ row }) => row.original.email ?? "—",
        }),
        createColumn<CustomerSummary>({
          accessorKey: "type",
          labelKey: "customers.fields.type",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: TYPES.map((value) => ({
            value,
            label: value,
            labelKey: `customers.type.${value}`,
          })),
          param: "type",
          cell: ({ row }) => t(`customers.type.${row.original.type}`),
        }),
        createColumn<CustomerSummary>({
          accessorKey: "status",
          labelKey: "customers.fields.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: STATUSES.map((value) => ({
            value,
            label: value,
            labelKey: `customers.status.${value}`,
          })),
          param: "status",
          cell: ({ row }) => (
            <StatusChip
              label={t(`customers.status.${row.original.status}`)}
              tone={customerStatusTone(row.original.status)}
            />
          ),
        }),
        createColumn<CustomerSummary>({
          accessorKey: "linked_at",
          labelKey: "customers.fields.linked_at",
          enableSorting: true,
          filterVariant: "date-range",
          param: "linked",
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {row.original.linked_at
                ? format.date(row.original.linked_at)
                : "—"}
            </span>
          ),
        }),
        createColumn<CustomerSummary>({
          accessorKey: "first_service_at",
          labelKey: "customers.fields.first_service_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {row.original.first_service_at
                ? format.date(row.original.first_service_at)
                : "—"}
            </span>
          ),
        }),
        // Filter only: the row carries no organization (TEC-371 matches
        // customers linked to the chosen organizations, inside the scope).
        createColumn<CustomerSummary>({
          id: "organization",
          accessorFn: () => "",
          labelKey: "customers.fields.organization",
          enableSorting: false,
          enableHiding: false,
          defaultHidden: true,
          filterVariant: "faceted",
          filterOptions: orgOptions,
          enableColumnFilter: canFilterOrg && orgOptions.length > 0,
          param: "organization_uuid",
          cell: () => null,
        }),
        createColumn<CustomerSummary>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const customer = row.original;
            const rowAccess = resolveCustomerDetailAccess(
              can,
              orgType,
              summaryAccessInput(customer),
            );
            const items: EntityRowAction[] = [
              {
                id: "view",
                label: t("common.view"),
                icon: Eye,
                onSelect: () =>
                  router.push(
                    routes.tenant.customers.detail(slug, customer.uuid),
                  ),
              },
            ];
            if (rowAccess.canEdit) {
              items.push({
                id: "edit",
                label: t("customers.actions.edit"),
                icon: Pencil,
                onSelect: () =>
                  router.push(
                    routes.tenant.customers.edit(slug, customer.uuid),
                  ),
              });
            }
            if (rowAccess.canUpgrade) {
              items.push({
                id: "upgrade",
                label: t("customers.actions.upgrade.button"),
                icon: ArrowUpCircle,
                onSelect: () => setAction({ kind: "upgrade", customer }),
              });
            }
            if (rowAccess.canExport) {
              items.push({
                id: "export",
                label: t("customers.actions.export.button"),
                icon: Download,
                onSelect: () => setAction({ kind: "export", customer }),
              });
            }
            if (rowAccess.canAnonymize) {
              items.push({
                id: "anonymize",
                label: t("customers.actions.anonymize.button"),
                icon: ShieldOff,
                variant: "destructive",
                onSelect: () => setAction({ kind: "anonymize", customer }),
              });
            }
            return <EntityRowActions actions={items} />;
          },
        }),
      ] as ColumnDef<CustomerSummary, unknown>[],
    [can, canFilterOrg, format, orgOptions, orgType, router, slug, t],
  );

  // Column meta drives the params: status / type / organization (CSV),
  // linked (_from / _to); sort is one of the backend fields.
  const listState = useServerListState({
    columns,
    initialSort: "-linked_at",
    initialPageSize: CUSTOMER_PAGE_SIZE,
    persistKey: CUSTOMERS_PERSIST_KEY,
  });
  const params: CustomerListQuery = listState.params;

  const list = useQuery({
    queryKey: customerKeys.list(params),
    queryFn: () => customersService.list(params),
    enabled: access.canRead,
  });
  const total = list.data?.total ?? 0;

  // The export runs on the same filters, search and sort as the list.
  const exportQuery = useMemo<ListExportQuery>(() => {
    const query: Record<string, string> = {};
    for (const [key, value] of Object.entries(listState.filterParams)) {
      if (value) query[key] = String(value);
    }
    if (params.q) query.q = params.q;
    if (params.sort) query.sort = params.sort;
    return query as ListExportQuery;
  }, [listState.filterParams, params.q, params.sort]);

  const done = () => {
    setAction(null);
    void qc.invalidateQueries({ queryKey: customerKeys.all });
  };

  const title = t("customers.list.title");
  return (
    <EntityPage
      title={title}
      description={t("customers.list.description")}
      permission={permissions.customers.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("customers.list.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: title },
      ]}
      actions={
        access.canExport || access.canCreate ? (
          <div className="flex flex-wrap gap-2">
            {access.canExport ? (
              <CustomerListExportButton filters={exportQuery} />
            ) : null}
            {access.canCreate ? (
              <EntityCreateButton
                label={t("customers.nav_new")}
                onClick={() =>
                  router.push(routes.tenant.customers.create(slug))
                }
              />
            ) : null}
          </div>
        ) : null
      }
    >
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(routes.tenant.customers.detail(slug, row.uuid))
        }
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("customers.list.empty_title")}
        emptyDescription={
          listState.columnFilters.length > 0 || params.q
            ? t("customers.list.empty_filtered")
            : t("customers.list.empty_description")
        }
        rowCount={total}
        state={listState.tableState}
        features={{ persistKey: CUSTOMERS_PERSIST_KEY }}
        renderGridItem={(c) => (
          <div className="space-y-2" data-testid="customer-card">
            <div className="flex items-start justify-between gap-2">
              <div className="min-w-0">
                <Link
                  href={routes.tenant.customers.detail(slug, c.uuid)}
                  className="font-medium hover:underline"
                  onClick={(event) => event.stopPropagation()}
                >
                  {customerDisplayName(c)}
                </Link>
                {c.type === "corporate" && c.company_name ? (
                  <p className="text-muted-foreground text-xs">
                    {c.company_name}
                  </p>
                ) : null}
              </div>
              <StatusChip
                label={t(`customers.status.${c.status}`)}
                tone={customerStatusTone(c.status)}
              />
            </div>
            <p className="text-muted-foreground text-xs">
              <span dir="ltr">{c.phone ?? "—"}</span>
              {c.email ? ` · ${c.email}` : ""}
            </p>
            <p className="text-muted-foreground text-xs">
              {t("customers.fields.linked_at")}:{" "}
              {c.linked_at ? format.date(c.linked_at) : "—"}
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

      {action?.kind === "anonymize" ? (
        <AnonymizeDialog
          customer={action.customer}
          open
          onClose={() => setAction(null)}
          onDone={done}
        />
      ) : null}
      {action?.kind === "export" ? (
        <DataExportDialog
          customer={action.customer}
          open
          onClose={() => setAction(null)}
        />
      ) : null}
      {action?.kind === "upgrade" ? (
        <UpgradeDialog
          customer={action.customer}
          open
          onClose={() => setAction(null)}
          onDone={done}
        />
      ) : null}
    </EntityPage>
  );
}
