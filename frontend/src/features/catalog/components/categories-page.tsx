"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, Trash2 } from "lucide-react";
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
import { createColumn } from "@/components/tables";
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
} from "@/features/catalog/services/catalog.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Tenant > Catalog > Categories (TEC-147). Writes are center only (K4). */
export function CategoriesPage({ slug }: { slug: string }) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const { catalog } = useCatalogAccess(slug);
  const listState = useServerListState({ initialSort: "sort" });
  const [editing, setEditing] = useState<CatalogCategory | null>(null);
  const [formOpen, setFormOpen] = useState(false);
  const [deleting, setDeleting] = useState<CatalogCategory | null>(null);

  const params = {
    limit: listState.params.limit,
    offset: listState.params.offset,
    q: listState.params.q,
  };
  const list = useQuery({
    queryKey: catalogKeys.categories(params),
    queryFn: () => catalogService.listCategories(params),
    enabled: catalog.canRead,
  });

  const onError = (error: unknown) =>
    appToast.error(
      isApiError(error) ? error.message : t("catalog.toast.failed"),
    );
  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: catalogKeys.all });

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

  const columns = useMemo(() => {
    const cols = [
      createColumn<CatalogCategory>({
        accessorKey: "name",
        labelKey: "catalog.fields.name",
        enableSorting: false,
        gridPrimary: true,
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
        enableSorting: false,
      }),
      createColumn<CatalogCategory>({
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
  }, [catalog.canWrite, t]);

  const total = list.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );

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
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "tenant-catalog-categories-v1",
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void list.refetch()}
            refreshDisabled={list.isFetching}
          />
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
