"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import {
  CircleCheck,
  CircleOff,
  Eye,
  FolderTree,
  ImageOff,
  Pencil,
} from "lucide-react";
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
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  catalogKeys,
  useCatalogAccess,
} from "@/features/catalog/hooks/use-catalog-access";
import { useCategoryOptions } from "@/features/catalog/hooks/use-category-options";
import {
  CATALOG_UNIT_TYPES,
  catalogService,
  productImageUrl,
  type CatalogProduct,
  type ListProductsParams,
} from "@/features/catalog/services/catalog.service";
import {
  BulkActionMenu,
  useBulkSelection,
  type BulkActionDef,
} from "@/features/bulk-engine";
import { ExportMenu } from "@/features/io/components/export-menu";
import { ImportButton } from "@/features/io/components/import-button";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const RESOURCE = "tenant.catalog.products" as const;
export const PRODUCTS_PERSIST_KEY = "tenant-catalog-products-v2";

/**
 * Bulk actions of `POST /v1/catalog/products/bulk` (TEC-212, TEC-369):
 * selected ids only; activate / deactivate / set_category are undoable.
 */
export const PRODUCT_BULK_ACTIONS: BulkActionDef[] = [
  {
    id: "activate",
    label_key: "bulk.actions.catalog_products.activate",
    permission: permissions.catalog.write,
    reversible: true,
    icon: CircleCheck,
  },
  {
    id: "deactivate",
    label_key: "bulk.actions.catalog_products.deactivate",
    permission: permissions.catalog.write,
    reversible: true,
    confirm_key: "bulk.confirm.catalog_products.deactivate",
    icon: CircleOff,
  },
  {
    id: "set_category",
    label_key: "bulk.actions.catalog_products.set_category",
    permission: permissions.catalog.write,
    reversible: true,
    params: [
      {
        key: "category_uuid",
        kind: "uuid",
        required: true,
        label_key: "bulk.params.category",
      },
    ],
    icon: FolderTree,
  },
];

function ProductCover({ product }: { product: CatalogProduct }) {
  const first = [...(product.images ?? [])].sort((a, b) => a.sort - b.sort)[0];
  const src = first ? productImageUrl(first.key) : null;
  if (!src) {
    return (
      <div className="bg-muted text-muted-foreground flex aspect-video w-full items-center justify-center rounded-lg">
        <ImageOff className="size-6" aria-hidden />
      </div>
    );
  }
  return (
    // eslint-disable-next-line @next/next/no-img-element -- public, cacheable product image route
    <img
      src={src}
      alt={product.name}
      loading="lazy"
      decoding="async"
      className="bg-muted aspect-video w-full rounded-lg object-cover"
    />
  );
}

/**
 * Tenant > Catalog > Products (TEC-147, TEC-370): server DataTable with
 * sort, column filters, bulk activate / deactivate / set category, row
 * actions, export / import and a grid view with the cover image. Every
 * level reads; writes are center only.
 */
export function ProductsPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const router = useRouter();
  const queryClient = useQueryClient();
  const { catalog } = useCatalogAccess(slug);
  const categories = useCategoryOptions(catalog.canRead);
  const categoryOptions = useMemo(
    () => categories.options.map((c) => ({ value: c.value, label: c.label })),
    [categories.options],
  );

  const toggleActive = useMutation({
    mutationFn: (product: CatalogProduct) =>
      catalogService.bulkActive([product.uuid], !product.active),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: catalogKeys.all });
      appToast.success(t("catalog.toast.saved"));
    },
    onError: (error) =>
      appToast.error(
        isApiError(error) ? error.message : t("catalog.toast.failed"),
      ),
  });
  const toggle = toggleActive.mutate;
  const togglePending = toggleActive.isPending;

  const baseColumns = useMemo(
    () =>
      [
        createColumn<CatalogProduct>({
          accessorKey: "sku",
          labelKey: "catalog.fields.sku",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <code className="text-xs">{row.original.sku}</code>
          ),
        }),
        createColumn<CatalogProduct>({
          accessorKey: "name",
          labelKey: "catalog.fields.name",
          enableSorting: true,
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
          accessorFn: (row) => row.category.uuid,
          labelKey: "catalog.fields.category",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: categoryOptions,
          enableColumnFilter: categoryOptions.length > 0,
          param: "category_uuid",
          cell: ({ row }) => row.original.category.name,
        }),
        createColumn<CatalogProduct>({
          accessorKey: "unit_type",
          labelKey: "catalog.fields.unit_type",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: CATALOG_UNIT_TYPES.map((value) => ({
            value,
            label: value,
            labelKey: `catalog.unit_types.${value}`,
          })),
          param: "unit_type",
          cell: ({ row }) => t(`catalog.unit_types.${row.original.unit_type}`),
        }),
        createColumn<CatalogProduct>({
          accessorKey: "uses_fixed_barcode",
          labelKey: "catalog.fields.uses_fixed_barcode",
          enableSorting: false,
          filterVariant: "boolean",
          param: "uses_fixed_barcode",
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
          enableSorting: true,
          filterVariant: "number-range",
          param: "warranty_duration_months",
          cell: ({ row }) =>
            row.original.warranty_duration_months === null
              ? "—"
              : format.number(row.original.warranty_duration_months),
        }),
        createColumn<CatalogProduct>({
          accessorKey: "micron_thickness",
          labelKey: "catalog.fields.micron_thickness",
          enableSorting: true,
          defaultHidden: true,
          filterVariant: "number-range",
          param: "micron_thickness",
          cell: ({ row }) =>
            row.original.micron_thickness === null
              ? "—"
              : format.number(row.original.micron_thickness),
        }),
        createColumn<CatalogProduct>({
          accessorKey: "active",
          labelKey: "catalog.fields.active",
          enableSorting: true,
          filterVariant: "boolean",
          param: "active",
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
        createColumn<CatalogProduct>({
          accessorKey: "created_at",
          labelKey: "catalog.fields.created_at",
          enableSorting: true,
          defaultHidden: true,
          filterVariant: "date-range",
          param: "created",
          cell: ({ row }) => format.dateTime(row.original.created_at),
        }),
        createColumn<CatalogProduct>({
          accessorKey: "updated_at",
          labelKey: "catalog.fields.updated_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => format.dateTime(row.original.updated_at),
        }),
        createColumn<CatalogProduct>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const product = row.original;
            const actions: EntityRowAction[] = [
              {
                id: "view",
                label: t("common.view"),
                icon: Eye,
                onSelect: () =>
                  router.push(
                    routes.tenant.catalog.product(slug, product.uuid),
                  ),
              },
            ];
            if (catalog.canWrite) {
              actions.push({
                id: "edit",
                label: t("common.edit"),
                icon: Pencil,
                onSelect: () =>
                  router.push(
                    routes.tenant.catalog.productEdit(slug, product.uuid),
                  ),
              });
              if (!(product.locked_fields ?? []).includes("active")) {
                actions.push({
                  id: "toggle_active",
                  label: product.active
                    ? t("catalog.products.bulk_deactivate")
                    : t("catalog.products.bulk_activate"),
                  icon: product.active ? CircleOff : CircleCheck,
                  disabled: togglePending,
                  onSelect: () => toggle(product),
                });
              }
            }
            return <EntityRowActions actions={actions} />;
          },
        }),
      ] as ColumnDef<CatalogProduct, unknown>[],
    [
      catalog.canWrite,
      categoryOptions,
      format,
      router,
      slug,
      t,
      toggle,
      togglePending,
    ],
  );

  const columns = useMemo(
    () =>
      catalog.canWrite
        ? [createSelectColumnDef<CatalogProduct>(), ...baseColumns]
        : baseColumns,
    [baseColumns, catalog.canWrite],
  );

  // Column meta drives the params: category / unit (CSV), active and fixed
  // barcode (boolean), warranty / micron (_min/_max), created (_from/_to).
  const listState = useServerListState({
    columns,
    initialSort: "name",
    persistKey: PRODUCTS_PERSIST_KEY,
  });
  const params: ListProductsParams = listState.params;

  const list = useQuery({
    queryKey: catalogKeys.products(params),
    queryFn: () => catalogService.listProducts(params),
    enabled: catalog.canRead,
  });
  const total = list.data?.total ?? 0;

  // Export and bulk work on the same filters, search and sort as the list.
  const exportQuery = useMemo(
    () => ({
      ...listState.filterParams,
      q: params.q,
      sort: params.sort,
    }),
    [listState.filterParams, params.q, params.sort],
  );
  const bulkSelection = useBulkSelection({
    listQueryKey: params,
    bulkQuery: exportQuery,
    total,
  });

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
        rowCount={total}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: PRODUCTS_PERSIST_KEY,
          rowSelection: catalog.canWrite,
          viewMode: true,
        }}
        renderGridItem={(product) => (
          <div className="space-y-3">
            <ProductCover product={product} />
            <div className="space-y-1">
              <Link
                href={routes.tenant.catalog.product(slug, product.uuid)}
                className="font-display leading-snug font-semibold hover:underline"
                onClick={(event) => event.stopPropagation()}
              >
                {product.name}
              </Link>
              <p className="text-muted-foreground text-xs">
                <code>{product.sku}</code> · {product.category.name}
              </p>
            </div>
            <div className="flex flex-wrap items-center justify-between gap-2">
              <StatusChip
                label={
                  product.active
                    ? t("catalog.status.active")
                    : t("catalog.status.inactive")
                }
                tone={product.active ? "success" : "default"}
              />
              <span className="text-muted-foreground text-xs">
                {t(`catalog.unit_types.${product.unit_type}`)}
              </span>
            </div>
          </div>
        )}
        toolbarExtra={
          <>
            {catalog.canWrite ? (
              <BulkActionMenu
                resource="catalog.products"
                actions={PRODUCT_BULK_ACTIONS}
                scope={bulkSelection.scope}
                selectedCount={bulkSelection.selectedCount}
                paramOptions={{ category_uuid: categoryOptions }}
                onComplete={() => {
                  bulkSelection.clearSelection();
                  void queryClient.invalidateQueries({
                    queryKey: catalogKeys.all,
                  });
                }}
              />
            ) : null}
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
