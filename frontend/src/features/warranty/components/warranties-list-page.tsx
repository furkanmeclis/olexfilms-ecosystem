"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Eye, ShieldCheck, ShieldOff } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
  type FilterParamSpecs,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { AsyncCombobox } from "@/components/ui/async-combobox";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { catalogService } from "@/features/catalog/services/catalog.service";
import { ExportMenu } from "@/features/io/components/export-menu";
import { useScopeOrganizationOptions } from "@/features/services/lib/use-scope-organizations";
import { downloadWarrantyCertificate } from "@/features/warranty/components/warranty-certificate-button";
import { VoidDialog } from "@/features/warranty/components/warranty-detail-page";
import { WarrantyProgressBar } from "@/features/warranty/components/warranty-progress";
import {
  WARRANTY_STATUSES,
  warrantyHolderName,
  warrantyStatusTone,
  warrantyVehicleTitle,
  type Warranty,
} from "@/features/warranty/lib/warranty-list";
import { panelCertificateClient } from "@/features/warranty/services/certificate.service";
import {
  WARRANTIES_EXPORT_PATH,
  warrantyKeys,
  warrantyService,
  type WarrantyServerQuery,
} from "@/features/warranty/services/warranty.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const WARRANTY_PAGE_SIZE = 20;
export const WARRANTIES_PERSIST_KEY = "tenant-warranties-v1";
/** Backend default: active by the soonest end, then the rest (TEC-377). */
export const WARRANTY_DEFAULT_SORT = "expiry";

/**
 * The product filter is the async catalog picker in the toolbar (the
 * catalog is too large for a fixed option list); it writes the `product`
 * column filter, sent as the single `product_uuid`.
 */
const PRODUCT_FILTER: FilterParamSpecs = {
  product: { param: "product_uuid", filterVariant: "select" },
};

/**
 * Tenant > Warranties (TEC-191, TEC-378): server DataTable over
 * GET /v1/warranties with single-field sort (default `expiry`), status
 * facet, days-left range, start / end date ranges, organization (center /
 * distributor) and product filters, `q` (code, service no, product, plate),
 * row actions (open, certificate PDF, void) and the list export with the
 * same query. Mobile shows cards with the elapsed-period bar.
 */
export function WarrantiesListPage({ slug }: { slug: string }) {
  const { t, format, locale } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const canRead = can(Permission.WarrantiesRead);
  const canPickProduct = can(Permission.CatalogRead);
  const canVoid = can(Permission.WarrantiesVoid);
  const orgFilter = useScopeOrganizationOptions(slug, canRead);
  const [voidTarget, setVoidTarget] = useState<Warranty | null>(null);

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
              className="font-mono text-xs font-medium whitespace-nowrap hover:underline"
              dir="ltr"
              data-testid="warranty-row"
              data-uuid={row.original.uuid}
              onClick={(event) => event.stopPropagation()}
            >
              {row.original.public_code}
            </Link>
          ),
        }),
        createColumn<Warranty>({
          id: "service_no",
          accessorFn: (row) => row.service.service_no,
          labelKey: "warranty.list.columns.service_no",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="font-mono text-xs" dir="ltr">
              {row.original.service.service_no}
            </span>
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
          param: "status",
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
          id: "vehicle",
          accessorFn: (row) => row.vehicle.plate ?? "",
          labelKey: "warranty.list.columns.vehicle",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="min-w-0">
              <div>{warrantyVehicleTitle(row.original) || "—"}</div>
              {row.original.vehicle.plate ? (
                <div
                  className="text-muted-foreground font-mono text-xs"
                  dir="ltr"
                >
                  {row.original.vehicle.plate}
                </div>
              ) : null}
            </div>
          ),
        }),
        createColumn<Warranty>({
          id: "holder",
          accessorFn: (row) => warrantyHolderName(row),
          labelKey: "warranty.list.columns.holder",
          enableSorting: false,
          cell: ({ row }) => warrantyHolderName(row.original) || "—",
        }),
        createColumn<Warranty>({
          id: "organization",
          accessorFn: (row) => row.organization.name,
          labelKey: "warranty.list.columns.organization",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: orgFilter.options,
          enableColumnFilter: orgFilter.enabled,
          param: "organization_uuid",
          cell: ({ row }) => row.original.organization.name,
        }),
        createColumn<Warranty>({
          id: "expiry",
          accessorFn: (row) => row.end_at,
          labelKey: "warranty.list.columns.days_left",
          enableSorting: true,
          filterVariant: "number-range",
          param: "days_left",
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
          defaultHidden: true,
          filterVariant: "date-range",
          param: "start",
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
          param: "end",
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {format.date(row.original.end_at)}
            </span>
          ),
        }),
        createColumn<Warranty>({
          accessorKey: "created_at",
          labelKey: "warranty.list.columns.created_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {format.date(row.original.created_at)}
            </span>
          ),
        }),
        createColumn<Warranty>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const w = row.original;
            const items: EntityRowAction[] = [
              {
                id: "view",
                label: t("common.view"),
                icon: Eye,
                onSelect: () =>
                  router.push(routes.tenant.warranties.detail(slug, w.uuid)),
              },
            ];
            // TEC-188: the certificate PDF of the service (active only).
            if (w.status === "active") {
              items.push({
                id: "certificate",
                label: t("warranty.certificate.download"),
                icon: ShieldCheck,
                onSelect: () =>
                  void downloadWarrantyCertificate({
                    client: panelCertificateClient(w.service.uuid),
                    locale,
                    t,
                  }),
              });
            }
            if (w.can_void && canVoid) {
              items.push({
                id: "void",
                label: t("warranty.void.open"),
                icon: ShieldOff,
                variant: "destructive",
                onSelect: () => setVoidTarget(w),
              });
            }
            return <EntityRowActions actions={items} />;
          },
        }),
      ] as ColumnDef<Warranty, unknown>[],
    [
      canVoid,
      format,
      locale,
      orgFilter.enabled,
      orgFilter.options,
      router,
      slug,
      t,
    ],
  );

  // Column meta drives the params: status / organization_uuid (CSV),
  // days_left_min / _max, start_* / end_* (days); product_uuid comes from
  // the toolbar picker; sort is one backend field.
  const listState = useServerListState({
    columns,
    filterParams: PRODUCT_FILTER,
    initialSort: WARRANTY_DEFAULT_SORT,
    initialPageSize: WARRANTY_PAGE_SIZE,
    persistKey: WARRANTIES_PERSIST_KEY,
  });
  const params: WarrantyServerQuery = listState.params;
  const { columnFilters, onColumnFiltersChange } = listState.tableState;
  const productUuid = String(
    columnFilters.find((f) => f.id === "product")?.value ?? "",
  );
  const setProduct = (value: string) =>
    onColumnFiltersChange((prev) => [
      ...prev.filter((f) => f.id !== "product"),
      ...(value ? [{ id: "product", value }] : []),
    ]);

  const list = useQuery({
    queryKey: warrantyKeys.list(params),
    queryFn: () => warrantyService.list(params),
    enabled: canRead,
  });
  const total = list.data?.total ?? 0;

  const loadProducts = useCallback(async (term: string) => {
    const res = await catalogService.listProducts({ q: term, limit: 20 });
    return res.items.map((p) => ({
      value: p.uuid,
      label: p.name,
      description: p.sku,
    }));
  }, []);

  // The export runs on the same filters, search and sort as the list.
  const exportQuery = useMemo(
    () => ({ ...listState.filterParams, q: params.q, sort: params.sort }),
    [listState.filterParams, params.q, params.sort],
  );
  const filtered = columnFilters.length > 0 || Boolean(params.q);

  const title = t("warranty.list.title");
  return (
    <EntityPage
      title={title}
      description={t("warranty.list.description")}
      permission={Permission.WarrantiesRead}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("warranty.list.forbidden")}
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
        onRowClick={(w) =>
          router.push(routes.tenant.warranties.detail(slug, w.uuid))
        }
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("warranty.list.empty_title")}
        emptyDescription={
          filtered
            ? t("warranty.list.empty_filtered")
            : t("warranty.list.empty_description")
        }
        rowCount={total}
        state={listState.tableState}
        features={{ persistKey: WARRANTIES_PERSIST_KEY }}
        renderGridItem={(w) => (
          <div className="space-y-2" data-testid="warranty-card">
            <div className="flex items-start justify-between gap-2">
              <div className="min-w-0">
                <Link
                  href={routes.tenant.warranties.detail(slug, w.uuid)}
                  className="font-mono text-xs font-medium hover:underline"
                  dir="ltr"
                  onClick={(event) => event.stopPropagation()}
                >
                  {w.public_code}
                </Link>
                <p className="font-medium">{w.product.name}</p>
              </div>
              <StatusChip
                label={t(`warranty.status.${w.status}`)}
                tone={warrantyStatusTone(w.status)}
              />
            </div>
            <p className="text-muted-foreground text-xs">
              {warrantyVehicleTitle(w)}
              {w.vehicle.plate ? (
                <span className="ms-2 font-mono" dir="ltr">
                  {w.vehicle.plate}
                </span>
              ) : null}
            </p>
            <WarrantyProgressBar warranty={w} compact />
            <p className="text-muted-foreground text-xs">
              {format.date(w.start_at)} – {format.date(w.end_at)} ·{" "}
              {w.organization.name}
            </p>
          </div>
        )}
        toolbarExtra={
          <>
            {canPickProduct ? (
              <AsyncCombobox
                id="warranty-product"
                className="h-8 w-56"
                value={productUuid}
                clearable
                onValueChange={setProduct}
                loadOptions={loadProducts}
                placeholder={t("warranty.list.product_placeholder")}
                searchPlaceholder={t("warranty.list.product_search")}
                emptyText={t("warranty.list.product_none")}
              />
            ) : null}
            <ExportMenu
              exportPath={WARRANTIES_EXPORT_PATH}
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
      {voidTarget ? (
        <VoidDialog
          key={voidTarget.uuid}
          warranty={voidTarget}
          open
          onOpenChange={(open) => {
            if (!open) setVoidTarget(null);
          }}
        />
      ) : null}
    </EntityPage>
  );
}
