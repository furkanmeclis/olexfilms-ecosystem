"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { CircleCheck, CircleOff, Pencil, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityCreateButton,
  EntityDeleteDialog,
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { CategoryFormDialog } from "@/features/catalog/components/category-form-dialog";
import {
  catalogKeys,
  useCatalogAccess,
} from "@/features/catalog/hooks/use-catalog-access";
import {
  catalogService,
  type CatalogCategory,
  type CatalogCategoryInput,
  type CatalogPage,
  type ListCategoriesParams,
} from "@/features/catalog/services/catalog.service";
import {
  BulkActionMenu,
  useBulkSelection,
  type BulkActionDef,
} from "@/features/bulk-engine";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const CATEGORIES_PERSIST_KEY = "tenant-catalog-categories-v2";

/** `POST /v1/catalog/categories/bulk` (TEC-369): center only, ids scope. */
export const CATEGORY_BULK_ACTIONS: BulkActionDef[] = [
  {
    id: "activate",
    label_key: "bulk.actions.catalog_categories.activate",
    permission: permissions.catalog.write,
    reversible: true,
    icon: CircleCheck,
  },
  {
    id: "deactivate",
    label_key: "bulk.actions.catalog_categories.deactivate",
    permission: permissions.catalog.write,
    reversible: true,
    confirm_key: "bulk.confirm.catalog_categories.deactivate",
    icon: CircleOff,
  },
  {
    id: "delete",
    label_key: "bulk.actions.catalog_categories.delete",
    permission: permissions.catalog.write,
    destructive: true,
    confirm_key: "bulk.confirm.catalog_categories.delete",
    icon: Trash2,
  },
];

/**
 * Tenant > Catalog > Categories (TEC-147, TEC-370). Server DataTable with
 * sort, active filter, bulk actions, drag-to-reorder (sort order) and
 * inline edit of name / active. Writes are center only (K4).
 */
export function CategoriesPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const queryClient = useQueryClient();
  const { catalog } = useCatalogAccess(slug);
  const [editing, setEditing] = useState<CatalogCategory | null>(null);
  const [formOpen, setFormOpen] = useState(false);
  const [deleting, setDeleting] = useState<CatalogCategory | null>(null);

  const onError = (error: unknown) =>
    appToast.error(
      isApiError(error) ? error.message : t("catalog.toast.failed"),
    );
  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: catalogKeys.all });

  const baseColumns = useMemo(() => {
    const cols = [
      createColumn<CatalogCategory>({
        accessorKey: "name",
        labelKey: "catalog.fields.name",
        enableSorting: true,
        gridPrimary: true,
        editVariant: "text",
        cell: ({ row }) => (
          <span className="font-medium">{row.original.name}</span>
        ),
      }),
      createColumn<CatalogCategory>({
        id: "available_parts",
        labelKey: "catalog.fields.available_parts",
        enableSorting: false,
        cell: ({ row }) => {
          const parts = row.original.available_parts;
          if (!parts.length) return "—";
          return (
            <div className="flex flex-wrap gap-1">
              {parts.slice(0, 6).map((part) => (
                <Badge key={part} variant="outline">
                  {part}
                </Badge>
              ))}
              {parts.length > 6 ? (
                <Badge variant="secondary">+{parts.length - 6}</Badge>
              ) : null}
            </div>
          );
        },
      }),
      createColumn<CatalogCategory>({
        accessorKey: "sort",
        labelKey: "catalog.fields.sort",
        enableSorting: true,
      }),
      createColumn<CatalogCategory>({
        accessorKey: "active",
        labelKey: "catalog.fields.active",
        enableSorting: true,
        filterVariant: "boolean",
        param: "active",
        editVariant: "boolean",
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
      createColumn<CatalogCategory>({
        accessorKey: "created_at",
        labelKey: "catalog.fields.created_at",
        enableSorting: true,
        defaultHidden: true,
        cell: ({ row }) => format.dateTime(row.original.created_at),
      }),
      createColumn<CatalogCategory>({
        accessorKey: "updated_at",
        labelKey: "catalog.fields.updated_at",
        enableSorting: true,
        defaultHidden: true,
        cell: ({ row }) => format.dateTime(row.original.updated_at),
      }),
    ];
    if (catalog.canWrite) {
      cols.push(
        createColumn<CatalogCategory>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "edit",
                  label: t("common.edit"),
                  icon: Pencil,
                  onSelect: () => {
                    setEditing(row.original);
                    setFormOpen(true);
                  },
                },
                {
                  id: "delete",
                  label: t("common.delete"),
                  icon: Trash2,
                  variant: "destructive",
                  onSelect: () => setDeleting(row.original),
                },
              ]}
            />
          ),
        }),
      );
    }
    return cols as ColumnDef<CatalogCategory, unknown>[];
  }, [catalog.canWrite, format, t]);

  const columns = useMemo(
    () =>
      catalog.canWrite
        ? [createSelectColumnDef<CatalogCategory>(), ...baseColumns]
        : baseColumns,
    [baseColumns, catalog.canWrite],
  );

  const listState = useServerListState({
    columns,
    initialSort: "sort",
    persistKey: CATEGORIES_PERSIST_KEY,
  });
  const params: ListCategoriesParams = listState.params;
  const listKey = catalogKeys.categories(params);
  const list = useQuery({
    queryKey: listKey,
    queryFn: () => catalogService.listCategories(params),
    enabled: catalog.canRead,
  });
  const total = list.data?.total ?? 0;

  const bulkQuery = useMemo(
    () => ({ ...listState.filterParams, q: params.q, sort: params.sort }),
    [listState.filterParams, params.q, params.sort],
  );
  const bulkSelection = useBulkSelection({
    listQueryKey: params,
    bulkQuery,
    total,
  });

  const save = useMutation({
    mutationFn: (input: CatalogCategoryInput) =>
      editing
        ? catalogService.updateCategory(editing.uuid, input)
        : catalogService.createCategory(input),
    onSuccess: async () => {
      await invalidate();
      setFormOpen(false);
      appToast.success(t("catalog.toast.saved"));
    },
    onError,
  });
  const remove = useMutation({
    mutationFn: (uuid: string) => catalogService.deleteCategory(uuid),
    onSuccess: async () => {
      await invalidate();
      setDeleting(null);
      appToast.success(t("catalog.toast.deleted"));
    },
    onError,
  });
  // Inline edit (double-click) of name / active → PATCH with that field.
  const patch = useMutation({
    mutationFn: ({
      uuid,
      input,
    }: {
      uuid: string;
      input: CatalogCategoryInput;
    }) => catalogService.updateCategory(uuid, input),
    onSuccess: async () => {
      await invalidate();
      appToast.success(t("table.cell_saved"));
    },
    onError,
  });
  // Drag-and-drop → PUT /v1/catalog/categories/order with the page's uuids
  // (the backend rearranges them within the positions they hold).
  const reorder = useMutation({
    mutationFn: (rows: CatalogCategory[]) =>
      catalogService.reorderCategories(rows.map((row) => row.uuid)),
    onMutate: async (rows) => {
      await queryClient.cancelQueries({ queryKey: listKey });
      const previous =
        queryClient.getQueryData<CatalogPage<CatalogCategory>>(listKey);
      if (previous) {
        queryClient.setQueryData(listKey, { ...previous, items: rows });
      }
      return { previous };
    },
    onSuccess: async () => {
      await invalidate();
      appToast.success(t("table.reorder_saved"));
    },
    onError: (error, _rows, context) => {
      if (context?.previous)
        queryClient.setQueryData(listKey, context.previous);
      onError(error);
    },
  });

  // Order only makes sense on the sort-order view.
  const canReorder = catalog.canWrite && params.sort === "sort";

  return (
    <EntityPage
      title={t("catalog.categories.title")}
      description={
        catalog.canWrite
          ? t("catalog.categories.description")
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
        {
          label: t("catalog.title"),
          href: routes.tenant.catalog.products(slug),
        },
        { label: t("catalog.categories.title") },
      ]}
      actions={
        catalog.canWrite ? (
          <EntityCreateButton
            label={t("catalog.categories.create")}
            onClick={() => {
              setEditing(null);
              setFormOpen(true);
            }}
          />
        ) : null
      }
    >
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("catalog.categories.empty_title")}
        emptyDescription={t("catalog.categories.empty_description")}
        rowCount={total}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: CATEGORIES_PERSIST_KEY,
          rowSelection: catalog.canWrite,
          rowReorder: canReorder,
          inlineEdit: catalog.canWrite,
        }}
        onRowReorder={canReorder ? (rows) => reorder.mutate(rows) : undefined}
        onCellEdit={
          catalog.canWrite
            ? ({ row, columnId, value }) => {
                if (columnId === "name") {
                  const name = String(value ?? "").trim();
                  if (!name || name === row.name) return;
                  patch.mutate({ uuid: row.uuid, input: { name } });
                } else if (columnId === "active") {
                  const active = Boolean(value);
                  if (active === row.active) return;
                  patch.mutate({ uuid: row.uuid, input: { active } });
                }
              }
            : undefined
        }
        toolbarExtra={
          <>
            {catalog.canWrite ? (
              <BulkActionMenu
                resource="catalog.categories"
                actions={CATEGORY_BULK_ACTIONS}
                scope={bulkSelection.scope}
                selectedCount={bulkSelection.selectedCount}
                onComplete={() => {
                  bulkSelection.clearSelection();
                  void invalidate();
                }}
              />
            ) : null}
            <EntityToolbar
              onRefresh={() => void list.refetch()}
              refreshDisabled={list.isFetching}
            />
          </>
        }
      />

      {catalog.canWrite ? (
        <>
          <CategoryFormDialog
            open={formOpen}
            category={editing}
            pending={save.isPending}
            onOpenChange={setFormOpen}
            onSubmit={(input) => save.mutateAsync(input)}
          />
          <EntityDeleteDialog
            open={Boolean(deleting)}
            entityLabel={deleting?.name ?? ""}
            softDelete={false}
            isPending={remove.isPending}
            onConfirm={() => deleting && remove.mutate(deleting.uuid)}
            onCancel={() => setDeleting(null)}
          />
        </>
      ) : null}
    </EntityPage>
  );
}
