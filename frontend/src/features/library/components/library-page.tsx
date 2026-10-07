"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import {
  Archive,
  Download,
  Eye,
  FileText,
  FolderPlus,
  Languages,
  Pencil,
  Plus,
  Upload,
} from "lucide-react";
import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { LOCALE_NAMES } from "@/config/i18n";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  LibraryArchiveDialog,
  LibraryDeleteFolderDialog,
  LibraryFolderDialog,
  LibraryItemDialog,
  LibraryUploadDialog,
  librarySelectClass,
} from "@/features/library/components/library-dialogs";
import { LibraryFolderTree } from "@/features/library/components/library-folder-tree";
import {
  LibraryItemDetail,
  useLibraryDownload,
} from "@/features/library/components/library-item-detail";
import {
  buildFolderTree,
  formatBytes,
  LIBRARY_ACCESS_LEVELS,
  LIBRARY_LOCALES,
  libraryLocale,
  localeShortLabel,
  type FolderNode,
} from "@/features/library/lib/library";
import {
  libraryKeys,
  libraryService,
  type LibraryFolder,
  type LibraryItem,
  type LibraryListQuery,
} from "@/features/library/services/library.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const LIBRARY_PERSIST_KEY = "tenant-library-items-v1";

type FolderDialogState =
  | { mode: "create"; parent: string | null }
  | { mode: "rename"; folder: LibraryFolder }
  | null;

type ItemDialogState = { item: LibraryItem | null } | null;

/**
 * Document library (TEC-333): folder tree + item list (server DataTable),
 * tag filter, search, language picker (default: the user's language),
 * item drawer with version history and, with `library.manage`, folder /
 * item / version management.
 */
export function LibraryPage({ slug }: { slug: string }) {
  const { t, format, locale: uiLocale } = useLocale();
  const { can } = usePermission();
  const canRead = can(Permission.LibraryRead);
  const canManage = can(Permission.LibraryManage);
  const download = useLibraryDownload();

  const [locale, setLocale] = useState(() => libraryLocale(uiLocale));
  const [folder, setFolder] = useState<string | null>(null);
  const [detail, setDetail] = useState<LibraryItem | null>(null);
  const [folderDialog, setFolderDialog] = useState<FolderDialogState>(null);
  const [deleteFolder, setDeleteFolder] = useState<LibraryFolder | null>(null);
  const [itemDialog, setItemDialog] = useState<ItemDialogState>(null);
  const [uploadItem, setUploadItem] = useState<LibraryItem | null>(null);
  const [archiveItem, setArchiveItem] = useState<LibraryItem | null>(null);

  const foldersQuery = useQuery({
    queryKey: libraryKeys.folders(),
    queryFn: () => libraryService.listFolders(),
    enabled: canRead,
  });
  const folders = useMemo(
    () => foldersQuery.data?.folders ?? [],
    [foldersQuery.data?.folders],
  );
  const tree = useMemo(() => buildFolderTree(folders), [folders]);
  const folderNames = useMemo(
    () => new Map(folders.map((f) => [f.uuid, f.name])),
    [folders],
  );

  const downloadOne = download.mutate;
  const downloading = download.isPending;

  const columns = useMemo<ColumnDef<LibraryItem, unknown>[]>(
    () => [
      createColumn<LibraryItem>({
        accessorKey: "name",
        labelKey: "library.fields.name",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="flex min-w-0 items-start gap-2">
            <FileText className="text-muted-foreground mt-0.5 size-4 shrink-0" />
            <span className="min-w-0">
              <span className="block truncate font-medium">
                {row.original.name}
              </span>
              {row.original.description ? (
                <span className="text-muted-foreground line-clamp-1 text-xs">
                  {row.original.description}
                </span>
              ) : null}
            </span>
          </span>
        ),
      }),
      createColumn<LibraryItem>({
        accessorKey: "tags",
        labelKey: "library.fields.tags",
        enableSorting: false,
        filterVariant: "text",
        param: "tag",
        cell: ({ row }) =>
          row.original.tags.length ? (
            <span className="flex flex-wrap gap-1">
              {row.original.tags.map((tag) => (
                <Badge key={tag} variant="secondary">
                  {tag}
                </Badge>
              ))}
            </span>
          ) : (
            "—"
          ),
      }),
      createColumn<LibraryItem>({
        accessorKey: "access_level",
        labelKey: "library.fields.access_level",
        enableSorting: true,
        filterVariant: "faceted",
        param: "access_level",
        gridSecondary: true,
        filterOptions: LIBRARY_ACCESS_LEVELS.map((value) => ({
          value,
          label: value,
          labelKey: `library.access.${value}`,
        })),
        cell: ({ row }) => t(`library.access.${row.original.access_level}`),
      }),
      createColumn<LibraryItem>({
        id: "latest_version",
        accessorFn: (row) => row.latest_version?.version_no ?? null,
        labelKey: "library.fields.latest_version",
        enableSorting: false,
        cell: ({ row }) => {
          const v = row.original.latest_version;
          if (!v) {
            return (
              <span className="text-muted-foreground text-xs">
                {t("library.detail.no_versions")}
              </span>
            );
          }
          const fallback = v.locale !== locale;
          return (
            <span
              className="flex items-center gap-1.5 text-xs tabular-nums"
              data-testid="library-latest-version"
              data-locale={v.locale}
            >
              <Badge variant={fallback ? "outline" : "default"}>
                {localeShortLabel(v.locale)}
              </Badge>
              {`v${v.version_no} · ${formatBytes(v.size_bytes)}`}
              {fallback ? (
                <span className="text-muted-foreground">
                  {t("library.detail.fallback")}
                </span>
              ) : null}
            </span>
          );
        },
      }),
      createColumn<LibraryItem>({
        id: "folder",
        accessorFn: (row) => row.folder_uuid ?? "",
        labelKey: "library.fields.folder",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) =>
          (row.original.folder_uuid &&
            folderNames.get(row.original.folder_uuid)) ||
          "—",
      }),
      createColumn<LibraryItem>({
        accessorKey: "updated_at",
        labelKey: "library.fields.updated_at",
        enableSorting: true,
        filterVariant: "date-range",
        param: "updated",
        cell: ({ row }) => format.dateTime(row.original.updated_at),
      }),
      createColumn<LibraryItem>({
        accessorKey: "created_at",
        labelKey: "library.fields.created_at",
        enableSorting: true,
        defaultHidden: true,
        cell: ({ row }) => format.dateTime(row.original.created_at),
      }),
      createColumn<LibraryItem>({
        id: "actions",
        labelKey: "common.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => {
          const item = row.original;
          const actions: EntityRowAction[] = [
            {
              id: "view",
              label: t("library.actions.view"),
              icon: Eye,
              onSelect: () => setDetail(item),
            },
          ];
          if (item.latest_version) {
            const versionUuid = item.latest_version.uuid;
            actions.push({
              id: "download",
              label: t("library.actions.download"),
              icon: Download,
              disabled: downloading,
              onSelect: () => downloadOne(versionUuid),
            });
          }
          if (canManage) {
            actions.push(
              {
                id: "edit",
                label: t("library.actions.edit"),
                icon: Pencil,
                onSelect: () => setItemDialog({ item }),
              },
              {
                id: "upload",
                label: t("library.actions.upload"),
                icon: Upload,
                onSelect: () => setUploadItem(item),
              },
              {
                id: "archive",
                label: t("library.actions.archive"),
                icon: Archive,
                variant: "destructive",
                onSelect: () => setArchiveItem(item),
              },
            );
          }
          return <EntityRowActions actions={actions} />;
        },
      }),
    ],
    [canManage, downloadOne, downloading, folderNames, format, locale, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "name",
    initialPageSize: 20,
    persistKey: LIBRARY_PERSIST_KEY,
  });
  const { setPagination } = listState;
  const params = useMemo<LibraryListQuery>(
    () => ({
      ...(listState.params as LibraryListQuery),
      folder: folder ?? undefined,
      locale,
    }),
    [folder, listState.params, locale],
  );

  const list = useQuery({
    queryKey: libraryKeys.list(params),
    queryFn: () => libraryService.listItems(params),
    placeholderData: keepPreviousData,
    enabled: canRead,
  });

  const selectFolder = useCallback(
    (uuid: string | null) => {
      setFolder(uuid);
      setPagination((p) => ({ ...p, pageIndex: 0 }));
    },
    [setPagination],
  );

  const title = t("library.title");
  if (!canRead) {
    return (
      <ErrorState
        title={t("common.error_forbidden")}
        description={t("library.forbidden")}
      />
    );
  }

  return (
    <div className="space-y-4" data-testid="library-page">
      <PageHeader
        title={title}
        icon={<FileText className="size-6" />}
        description={t("library.description")}
        breadcrumbs={[
          {
            label: t("layout.breadcrumb_home"),
            href: routes.tenant.home(slug),
          },
          { label: title },
        ]}
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <label className="flex items-center gap-2 text-sm">
              <Languages className="text-muted-foreground size-4" />
              <span className="sr-only">{t("library.fields.locale")}</span>
              <select
                id="library-locale"
                data-testid="library-locale"
                className={librarySelectClass}
                value={locale}
                onChange={(e) => setLocale(e.target.value)}
              >
                {LIBRARY_LOCALES.map((l) => (
                  <option key={l} value={l}>
                    {LOCALE_NAMES[l]}
                  </option>
                ))}
              </select>
            </label>
            {canManage ? (
              <>
                <Button
                  type="button"
                  variant="outline"
                  data-testid="library-new-folder"
                  onClick={() =>
                    setFolderDialog({ mode: "create", parent: folder })
                  }
                >
                  <FolderPlus className="size-4" />
                  {t("library.folders.create")}
                </Button>
                <Button
                  type="button"
                  data-testid="library-new-item"
                  onClick={() => setItemDialog({ item: null })}
                >
                  <Plus className="size-4" />
                  {t("library.items.create")}
                </Button>
              </>
            ) : null}
          </div>
        }
      />

      <div className="grid gap-4 md:grid-cols-[16rem_minmax(0,1fr)]">
        <Card className="h-fit">
          <CardContent className="p-2">
            <LibraryFolderTree
              tree={tree}
              selected={folder}
              onSelect={selectFolder}
              canManage={canManage}
              onRename={(f: FolderNode) =>
                setFolderDialog({ mode: "rename", folder: f })
              }
              onDelete={(f: FolderNode) => setDeleteFolder(f)}
            />
          </CardContent>
        </Card>
        <div className="min-w-0">
          <EntityTable
            columns={columns}
            data={list.data?.items ?? []}
            getRowId={(row) => row.uuid}
            onRowClick={(row) => setDetail(row)}
            isLoading={list.isLoading}
            isError={list.isError}
            onRetry={() => void list.refetch()}
            emptyTitle={t("library.empty_title")}
            emptyDescription={t("library.empty_description")}
            rowCount={list.data?.total ?? 0}
            state={listState.tableState}
            features={{
              persistKey: LIBRARY_PERSIST_KEY,
              // No bulk / export endpoints for the library.
              rowSelection: false,
              viewMode: true,
            }}
            toolbarExtra={
              <EntityToolbar
                onRefresh={() => void list.refetch()}
                refreshDisabled={list.isFetching}
              />
            }
          />
        </div>
      </div>

      <LibraryItemDetail
        item={detail}
        locale={locale}
        onLocaleChange={setLocale}
        onOpenChange={(open) => {
          if (!open) setDetail(null);
        }}
        canManage={canManage}
        onUpload={(item) => setUploadItem(item)}
      />

      {canManage ? (
        <>
          {folderDialog ? (
            <LibraryFolderDialog
              key={
                folderDialog.mode === "rename"
                  ? folderDialog.folder.uuid
                  : "new"
              }
              open
              onOpenChange={(open) => {
                if (!open) setFolderDialog(null);
              }}
              tree={tree}
              folder={
                folderDialog.mode === "rename" ? folderDialog.folder : null
              }
              defaultParent={
                folderDialog.mode === "create" ? folderDialog.parent : null
              }
            />
          ) : null}
          <LibraryDeleteFolderDialog
            key={`delete-${deleteFolder?.uuid ?? ""}`}
            folder={deleteFolder}
            onOpenChange={(open) => {
              if (!open) setDeleteFolder(null);
            }}
            onDeleted={(uuid) => {
              if (folder === uuid) selectFolder(null);
            }}
          />
          {itemDialog ? (
            <LibraryItemDialog
              key={itemDialog.item?.uuid ?? "new"}
              open
              onOpenChange={(open) => {
                if (!open) setItemDialog(null);
              }}
              tree={tree}
              item={itemDialog.item}
              defaultFolder={folder}
              onCreated={(item) => setUploadItem(item)}
            />
          ) : null}
          <LibraryUploadDialog
            key={`upload-${uploadItem?.uuid ?? ""}-${locale}`}
            item={uploadItem}
            defaultLocale={locale}
            onOpenChange={(open) => {
              if (!open) setUploadItem(null);
            }}
          />
          <LibraryArchiveDialog
            item={archiveItem}
            onOpenChange={(open) => {
              if (!open) setArchiveItem(null);
            }}
            onArchived={(uuid) => {
              if (detail?.uuid === uuid) setDetail(null);
            }}
          />
        </>
      ) : null}
    </div>
  );
}
