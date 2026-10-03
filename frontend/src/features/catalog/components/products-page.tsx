"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef, Table } from "@tanstack/react-table";
import { CircleCheck, CircleOff, FolderTree } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityCreateButton,
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import {
  createColumn,
  createSelectColumnDef,
  type DataTableBulkAction,
} from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  catalogKeys,
  useCatalogAccess,
} from "@/features/catalog/hooks/use-catalog-access";
import { useCategoryOptions } from "@/features/catalog/hooks/use-category-options";
import {
  BULK_ACTIVE_MAX,
  CATALOG_UNIT_TYPES,
  catalogService,
  type CatalogProduct,
  type CatalogUnitType,
  type ListProductsParams,
} from "@/features/catalog/services/catalog.service";
import { useBulkMutation } from "@/features/bulk-engine/hooks/use-bulk-mutation";
import { ExportMenu } from "@/features/io/components/export-menu";
import { ImportButton } from "@/features/io/components/import-button";
import { useLocale } from "@/providers/locale-provider";

const ALL = "all";
const RESOURCE = "tenant.catalog.products" as const;

type FilterOption = { value: string; label: string };

function FilterSelect({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: string;
  options: FilterOption[];
  onChange: (value: string) => void;
}) {
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger className="h-8 w-auto min-w-36" aria-label={label}>
        <SelectValue placeholder={label} />
      </SelectTrigger>
      <SelectContent>
        {options.map((option) => (
          <SelectItem key={option.value} value={option.value}>
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

/**
 * Tenant > Catalog > Products (TEC-147): search, filters, bulk
 * activate / deactivate and I/O. Every level reads; writes are center only.
 */
export function ProductsPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const router = useRouter();
  const queryClient = useQueryClient();
  const { catalog } = useCatalogAccess(slug);
  const listState = useServerListState({ initialSort: "name" });
  const [active, setActive] = useState<string>(ALL);
  const [category, setCategory] = useState<string>(ALL);
  const [unitType, setUnitType] = useState<string>(ALL);
  const categories = useCategoryOptions(catalog.canRead);

  const params = useMemo<ListProductsParams>(
    () => ({
      limit: listState.params.limit,
      offset: listState.params.offset,
      q: listState.params.q,
      active: active === ALL ? undefined : active === "true",
      category_uuid: category === ALL ? undefined : category,
      unit_type: unitType === ALL ? undefined : (unitType as CatalogUnitType),
    }),
    [active, category, listState.params, unitType],
  );

  const list = useQuery({
    queryKey: catalogKeys.products(params),
    queryFn: () => catalogService.listProducts(params),
    enabled: catalog.canRead,
  });

  // TEC-212: activate / deactivate run through the bulk engine, which logs
  // the operation and offers "undo" on the result toast.
  const bulk = useBulkMutation();

  const baseColumns = useMemo(
    () =>
      [
        createColumn<CatalogProduct>({
          accessorKey: "sku",
          labelKey: "catalog.fields.sku",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <code className="text-xs">{row.original.sku}</code>
          ),
        }),
        createColumn<CatalogProduct>({
          accessorKey: "name",
          labelKey: "catalog.fields.name",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <Link
              href={routes.tenant.catalog.product(slug, row.original.uuid)}
              className="font-medium hover:underline"
              onClick={(event) => event.stopPropagation()}
            >
              {row.original.name}
            </Link>
          ),
        }),
        createColumn<CatalogProduct>({
          id: "category",
          accessorFn: (row) => row.category.name,
          labelKey: "catalog.fields.category",
          enableSorting: false,
        }),
        createColumn<CatalogProduct>({
          accessorKey: "unit_type",
          labelKey: "catalog.fields.unit_type",
          enableSorting: false,
          cell: ({ row }) => t(`catalog.unit_types.${row.original.unit_type}`),
        }),
        createColumn<CatalogProduct>({
          accessorKey: "uses_fixed_barcode",
          labelKey: "catalog.fields.uses_fixed_barcode",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.uses_fixed_barcode ? (
              <Badge variant="outline">{t("common.yes")}</Badge>
            ) : (
              t("common.no")
            ),
        }),
        createColumn<CatalogProduct>({
          accessorKey: "warranty_duration_months",
          labelKey: "catalog.fields.warranty_duration_months",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.warranty_duration_months === null
              ? "—"
              : format.number(row.original.warranty_duration_months),
        }),
        createColumn<CatalogProduct>({
          accessorKey: "micron_thickness",
          labelKey: "catalog.fields.micron_thickness",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.micron_thickness === null
              ? "—"
              : format.number(row.original.micron_thickness),
        }),
        createColumn<CatalogProduct>({
          accessorKey: "active",
          labelKey: "catalog.fields.active",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={
                row.original.active
                  ? t("catalog.status.active")
                  : t("catalog.status.inactive")
              }
              tone={row.original.active ? "success" : "default"}
            />
          ),
        }),
      ] as ColumnDef<CatalogProduct, unknown>[],
    [format, slug, t],
  );

  const columns = useMemo(
    () =>
      catalog.canWrite
        ? [createSelectColumnDef<CatalogProduct>(), ...baseColumns]
        : baseColumns,
    [baseColumns, catalog.canWrite],
  );

  const bulkActions = useMemo<DataTableBulkAction<CatalogProduct>[]>(() => {
    if (!catalog.canWrite) return [];
    const run =
      (value: boolean) =>
      (selected: CatalogProduct[], table: Table<CatalogProduct>) => {
        const uuids = selected.map((p) => p.uuid).slice(0, BULK_ACTIVE_MAX);
        if (!uuids.length) return;
        bulk.mutate({
          resource: "catalog.products",
          action: value ? "activate" : "deactivate",
          target: { scope: "ids", ids: uuids },
          onComplete: () => {
            table.resetRowSelection();
            void queryClient.invalidateQueries({ queryKey: catalogKeys.all });
          },
        });
      };
    return [
      {
        id: "activate",
        label: t("catalog.products.bulk_activate"),
        icon: CircleCheck,
        onClick: run(true),
        disabled: () => bulk.isPending,
      },
      {
        id: "deactivate",
        label: t("catalog.products.bulk_deactivate"),
        icon: CircleOff,
        onClick: run(false),
        disabled: () => bulk.isPending,
      },
    ];
  }, [bulk, catalog.canWrite, queryClient, t]);

  const categoryOptions: FilterOption[] = [
    { value: ALL, label: t("catalog.filters.all_categories") },
    ...categories.options.map((c) => ({ value: c.value, label: c.label })),
  ];
  const exportQuery = {
    q: params.q,
    active: active === ALL ? undefined : active,
    category_uuid: params.category_uuid,
    unit_type: params.unit_type,
  };
  const total = list.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );

  return (
    <EntityPage
      title={t("catalog.products.title")}
      description={
        catalog.canWrite
          ? t("catalog.products.description")
          : t("catalog.read_only")
      }
      permission={permissions.catalog.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("catalog.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("catalog.products.title") },
      ]}
      actions={
        <div className="flex flex-wrap items-center gap-2">
          <Button asChild variant="outline">
            <Link href={routes.tenant.catalog.categories(slug)}>
              <FolderTree className="size-4" />
              {t("catalog.categories.title")}
            </Link>
          </Button>
          {catalog.canWrite ? (
            <EntityCreateButton
              label={t("catalog.products.create")}
              onClick={() =>
                router.push(routes.tenant.catalog.productCreate(slug))
              }
            />
          ) : null}
        </div>
      }
    >
      <div
        className="flex flex-wrap items-center gap-2"
        data-testid="catalog-product-filters"
      >
        <FilterSelect
          label={t("catalog.filters.status")}
          value={active}
          onChange={(v) => {
            setActive(v);
            listState.setPagination((p) => ({ ...p, pageIndex: 0 }));
          }}
          options={[
            { value: ALL, label: t("catalog.filters.all_statuses") },
            { value: "true", label: t("catalog.status.active") },
            { value: "false", label: t("catalog.status.inactive") },
          ]}
        />
        <FilterSelect
          label={t("catalog.fields.category")}
          value={category}
          onChange={(v) => {
            setCategory(v);
            listState.setPagination((p) => ({ ...p, pageIndex: 0 }));
          }}
          options={categoryOptions}
        />
        <FilterSelect
          label={t("catalog.fields.unit_type")}
          value={unitType}
          onChange={(v) => {
            setUnitType(v);
            listState.setPagination((p) => ({ ...p, pageIndex: 0 }));
          }}
          options={[
            { value: ALL, label: t("catalog.filters.all_units") },
            ...CATALOG_UNIT_TYPES.map((u) => ({
              value: u,
              label: t(`catalog.unit_types.${u}`),
            })),
          ]}
        />
      </div>
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(routes.tenant.catalog.product(slug, row.uuid))
        }
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("catalog.products.empty_title")}
        emptyDescription={t("catalog.products.empty_description")}
        pageCount={pageCount}
        state={listState.tableState}
        bulkActions={catalog.canWrite ? bulkActions : undefined}
        features={{
          persistKey: "tenant-catalog-products-v1",
          rowSelection: catalog.canWrite,
          columnFilters: false,
          facetedFilters: false,
        }}
        toolbarExtra={
          <>
            <ExportMenu
              resource={RESOURCE}
              query={exportQuery}
              jobsHref={routes.tenant.exports.root(slug)}
            />
            {catalog.canWrite ? (
              <ImportButton
                resource={RESOURCE}
                scope="tenant"
                jobsHref={routes.tenant.imports.root(slug)}
                onComplete={() => void list.refetch()}
              />
            ) : null}
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
