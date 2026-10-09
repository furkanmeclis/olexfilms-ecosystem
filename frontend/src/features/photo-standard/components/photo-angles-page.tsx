"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { ImageIcon, Pencil, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  CLIENT_SIDE_MANUAL,
  EntityCreateButton,
  EntityDeleteDialog,
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { SUPPORTED_LOCALES } from "@/config/i18n";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  PhotoAngleDialog,
  angleBody,
  type PhotoAngleSubmit,
} from "@/features/photo-standard/components/photo-angle-dialog";
import {
  angleInput,
  angleName,
  nextSortOrder,
  reorderPatches,
} from "@/features/photo-standard/lib/photo-standard";
import {
  photoSrc,
  photoStandardKeys,
  photoStandardService,
  type PhotoAngle,
} from "@/features/photo-standard/services/photo-standard.service";
import { isApiError } from "@/lib/api";
import { isFeatureDisabledError } from "@/lib/api/limit-events";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const T = "photo_standard.angles";
export const PHOTO_ANGLES_PERSIST_KEY = "platform-photo-angles-v1";

type AngleList = { items: PhotoAngle[] };

/**
 * Platform > Fotoğraf açıları (TEC-500): the central intake angles of the
 * photo standard. Client-side DataTable (the API returns the whole list)
 * with search, required / active filters and inline toggles, drag-to-
 * reorder (sort_order PUTs) and a 13-language form with the example image.
 */
export function PhotoAnglesPage() {
  const { t, locale } = useLocale();
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<PhotoAngle | null>(null);
  const [formOpen, setFormOpen] = useState(false);
  const [deleting, setDeleting] = useState<PhotoAngle | null>(null);
  const listKey = photoStandardKeys.angles;

  const list = useQuery({
    queryKey: listKey,
    queryFn: () => photoStandardService.listAngles(),
    retry: false,
  });
  const items = useMemo(() => list.data?.items ?? [], [list.data]);

  const onError = (error: unknown) =>
    appToast.error(isApiError(error) ? error.message : t(`${T}.toast.failed`));
  const invalidate = () => queryClient.invalidateQueries({ queryKey: listKey });

  const save = useMutation({
    mutationFn: async ({
      values,
      example,
      removeExample,
    }: PhotoAngleSubmit) => {
      const body = angleBody(values, {
        sort_order: editing?.sort_order ?? nextSortOrder(items),
        example_storage_key: removeExample
          ? null
          : (editing?.example_storage_key ?? null),
      });
      const saved = editing
        ? await photoStandardService.updateAngle(editing.uuid, body)
        : await photoStandardService.createAngle(body);
      if (example)
        await photoStandardService.uploadExample(saved.uuid, example);
      return saved;
    },
    onSuccess: () => {
      setFormOpen(false);
      appToast.success(t(`${T}.toast.saved`));
    },
    onError,
    // A failed example upload still leaves the angle saved: always refetch.
    onSettled: () => invalidate(),
  });

  const patch = useMutation({
    mutationFn: ({
      angle,
      change,
    }: {
      angle: PhotoAngle;
      change: { required?: boolean; active?: boolean };
    }) =>
      photoStandardService.updateAngle(angle.uuid, angleInput(angle, change)),
    onSuccess: async () => {
      await invalidate();
      appToast.success(t("table.cell_saved"));
    },
    onError,
  });

  const reorder = useMutation({
    mutationFn: async (rows: PhotoAngle[]) => {
      for (const p of reorderPatches(rows)) {
        await photoStandardService.updateAngle(
          p.angle.uuid,
          angleInput(p.angle, { sort_order: p.sort_order }),
        );
      }
    },
    onMutate: async (rows) => {
      await queryClient.cancelQueries({ queryKey: listKey });
      const previous = queryClient.getQueryData<AngleList>(listKey);
      queryClient.setQueryData<AngleList>(listKey, { items: rows });
      return { previous };
    },
    onSuccess: () => appToast.success(t("table.reorder_saved")),
    onError: (error, _rows, context) => {
      if (context?.previous)
        queryClient.setQueryData(listKey, context.previous);
      onError(error);
    },
    onSettled: () => invalidate(),
  });

  const remove = useMutation({
    mutationFn: (angle: PhotoAngle) =>
      photoStandardService.deleteAngle(angle.uuid),
    onSuccess: () => {
      setDeleting(null);
      appToast.success(t(`${T}.toast.deleted`));
    },
    onError,
    onSettled: () => invalidate(),
  });

  const columns = useMemo(
    () =>
      [
        createColumn<PhotoAngle>({
          accessorKey: "sort_order",
          labelKey: `${T}.fields.sort_order`,
          enableSorting: true,
          size: 90,
        }),
        createColumn<PhotoAngle>({
          id: "example",
          labelKey: `${T}.fields.example`,
          enableSorting: false,
          size: 90,
          cell: ({ row }) =>
            row.original.example_url ? (
              // eslint-disable-next-line @next/next/no-img-element
              <img
                src={photoSrc(row.original.example_url)}
                alt={angleName(row.original, locale)}
                loading="lazy"
                className="bg-muted size-10 rounded border object-contain"
                data-testid="angle-example"
              />
            ) : (
              <span className="bg-muted flex size-10 items-center justify-center rounded border">
                <ImageIcon className="text-muted-foreground size-4" />
              </span>
            ),
        }),
        createColumn<PhotoAngle>({
          id: "name",
          accessorFn: (row) => angleName(row, locale),
          labelKey: `${T}.fields.name`,
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">
              {angleName(row.original, locale)}
            </span>
          ),
        }),
        createColumn<PhotoAngle>({
          accessorKey: "key",
          labelKey: `${T}.fields.key`,
          enableSorting: true,
          cell: ({ row }) => (
            <span className="text-muted-foreground font-mono text-xs" dir="ltr">
              {row.original.key}
            </span>
          ),
        }),
        createColumn<PhotoAngle>({
          accessorKey: "required",
          labelKey: `${T}.fields.required`,
          enableSorting: true,
          filterVariant: "boolean",
          editVariant: "boolean",
          gridSecondary: true,
          cell: ({ row }) =>
            row.original.required ? t(`${T}.required`) : t(`${T}.optional`),
        }),
        createColumn<PhotoAngle>({
          accessorKey: "active",
          labelKey: `${T}.fields.active`,
          enableSorting: true,
          filterVariant: "boolean",
          editVariant: "boolean",
          cell: ({ row }) => (
            <StatusChip
              label={
                row.original.active ? t(`${T}.active`) : t(`${T}.inactive`)
              }
              tone={row.original.active ? "success" : "default"}
            />
          ),
        }),
        createColumn<PhotoAngle>({
          id: "translations",
          accessorFn: (row) => Object.keys(row.name ?? {}).length,
          labelKey: `${T}.fields.translations`,
          enableSorting: true,
          cell: ({ row }) =>
            `${Object.keys(row.original.name ?? {}).length}/${SUPPORTED_LOCALES.length}`,
        }),
        createColumn<PhotoAngle>({
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
      ] as ColumnDef<PhotoAngle, unknown>[],
    [locale, t],
  );

  const moduleOff = isFeatureDisabledError(list.error);

  return (
    <EntityPage
      title={t(`${T}.title`)}
      description={t(`${T}.description`)}
      permission={permissions.photoStandard.manage}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t(`${T}.forbidden`)}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t(`${T}.title`) },
      ]}
      actions={
        moduleOff ? null : (
          <EntityCreateButton
            label={t(`${T}.create`)}
            onClick={() => {
              setEditing(null);
              setFormOpen(true);
            }}
          />
        )
      }
    >
      {moduleOff ? (
        <ErrorState
          title={t(`${T}.module_off_title`)}
          description={t(`${T}.module_off`)}
        />
      ) : (
        <EntityTable
          columns={columns}
          data={items}
          getRowId={(row) => row.uuid}
          manual={CLIENT_SIDE_MANUAL}
          isLoading={list.isLoading}
          isError={list.isError}
          onRetry={() => void list.refetch()}
          emptyTitle={t(`${T}.empty_title`)}
          emptyDescription={t(`${T}.empty_description`)}
          initialState={{
            sorting: [{ id: "sort_order", desc: false }],
            pagination: { pageIndex: 0, pageSize: 50 },
          }}
          pageSizeOptions={[20, 50, 100]}
          features={{
            persistKey: PHOTO_ANGLES_PERSIST_KEY,
            rowSelection: false,
            rowReorder: true,
            inlineEdit: true,
          }}
          onRowReorder={(rows) => reorder.mutate(rows)}
          onCellEdit={({ row, columnId, value }) => {
            if (columnId !== "required" && columnId !== "active") return;
            const next = Boolean(value);
            if (next === row[columnId]) return;
            patch.mutate({ angle: row, change: { [columnId]: next } });
          }}
          toolbarExtra={
            <EntityToolbar
              onRefresh={() => void list.refetch()}
              refreshDisabled={list.isFetching}
            />
          }
        />
      )}

      <PhotoAngleDialog
        open={formOpen}
        angle={editing}
        pending={save.isPending}
        onOpenChange={setFormOpen}
        onSubmit={(submit) => save.mutateAsync(submit)}
      />
      <EntityDeleteDialog
        open={Boolean(deleting)}
        entityLabel={deleting ? angleName(deleting, locale) : ""}
        softDelete={false}
        isPending={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting)}
        onCancel={() => setDeleting(null)}
      />
    </EntityPage>
  );
}
