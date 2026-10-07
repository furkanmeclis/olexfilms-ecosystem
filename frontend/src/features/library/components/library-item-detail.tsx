"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Download, Upload } from "lucide-react";
import { useMemo } from "react";
import { toast } from "sonner";

import {
  CLIENT_SIDE_MANUAL,
  EntityRowActions,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { LOCALE_NAMES, type AppLocale } from "@/config/i18n";
import {
  availableLocales,
  formatBytes,
  localeShortLabel,
  versionsForLocale,
} from "@/features/library/lib/library";
import {
  libraryKeys,
  libraryService,
  type LibraryItem,
  type LibraryVersion,
} from "@/features/library/services/library.service";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

export const LIBRARY_VERSIONS_PERSIST_KEY = "tenant-library-versions-v1";

/** Opens a short-lived presigned URL of one version. */
export function useLibraryDownload() {
  const { t } = useLocale();
  return useMutation({
    mutationFn: (versionUuid: string) => libraryService.download(versionUuid),
    onSuccess: (out) => {
      window.open(out.url, "_blank", "noopener,noreferrer");
    },
    onError: (err) =>
      toast.error(
        isApiError(err) ? err.message : t("library.detail.download_error"),
      ),
  });
}

/**
 * Item drawer: description, languages with a version, version history of
 * the selected language and presigned downloads.
 */
export function LibraryItemDetail({
  item,
  locale,
  onLocaleChange,
  onOpenChange,
  canManage,
  onUpload,
}: {
  item: LibraryItem | null;
  locale: string;
  onLocaleChange: (locale: string) => void;
  onOpenChange: (open: boolean) => void;
  canManage: boolean;
  onUpload: (item: LibraryItem) => void;
}) {
  const { t, format } = useLocale();
  const download = useLibraryDownload();
  const versionsQuery = useQuery({
    queryKey: libraryKeys.versions(item?.uuid ?? ""),
    queryFn: () => libraryService.listVersions(item!.uuid),
    enabled: Boolean(item),
  });
  const versions = useMemo(
    () => versionsQuery.data?.versions ?? [],
    [versionsQuery.data?.versions],
  );
  const languages = useMemo(() => availableLocales(versions), [versions]);
  const history = useMemo(
    () => versionsForLocale(versions, locale),
    [versions, locale],
  );
  const downloadOne = download.mutate;
  const downloading = download.isPending;

  const columns = useMemo<ColumnDef<LibraryVersion, unknown>[]>(
    () => [
      createColumn<LibraryVersion>({
        accessorKey: "version_no",
        labelKey: "library.detail.version_no",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => `v${row.original.version_no}`,
      }),
      createColumn<LibraryVersion>({
        accessorKey: "created_at",
        labelKey: "library.detail.uploaded_at",
        enableSorting: true,
        gridSecondary: true,
        cell: ({ row }) => format.dateTime(row.original.created_at),
      }),
      createColumn<LibraryVersion>({
        accessorKey: "size_bytes",
        labelKey: "library.detail.size",
        enableSorting: true,
        cell: ({ row }) => formatBytes(row.original.size_bytes),
      }),
      createColumn<LibraryVersion>({
        accessorKey: "mime",
        labelKey: "library.detail.mime",
        enableSorting: false,
        defaultHidden: true,
      }),
      createColumn<LibraryVersion>({
        id: "actions",
        labelKey: "common.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => (
          <EntityRowActions
            actions={[
              {
                id: "download",
                label: t("library.actions.download"),
                icon: Download,
                disabled: downloading,
                onSelect: () => downloadOne(row.original.uuid),
              },
            ]}
          />
        ),
      }),
    ],
    [downloadOne, downloading, format, t],
  );

  return (
    <Sheet open={Boolean(item)} onOpenChange={onOpenChange}>
      <SheetContent
        className="w-full overflow-y-auto sm:max-w-xl"
        data-testid="library-item-detail"
      >
        {item ? (
          <div className="space-y-5">
            <SheetHeader>
              <SheetTitle>{item.name}</SheetTitle>
              <SheetDescription>
                {t(`library.access.${item.access_level}`)}
                {item.role_slug ? ` · ${item.role_slug}` : ""}
              </SheetDescription>
            </SheetHeader>
            <section className="space-y-2 px-4">
              <h3 className="text-sm font-medium">
                {t("library.fields.description")}
              </h3>
              <p className="text-muted-foreground text-sm whitespace-pre-wrap">
                {item.description || t("library.detail.no_description")}
              </p>
              {item.tags.length ? (
                <div className="flex flex-wrap gap-1">
                  {item.tags.map((tag) => (
                    <Badge key={tag} variant="secondary">
                      {tag}
                    </Badge>
                  ))}
                </div>
              ) : null}
            </section>
            <section className="space-y-2 px-4">
              <h3 className="text-sm font-medium">
                {t("library.detail.languages")}
              </h3>
              {languages.length ? (
                <div
                  className="flex flex-wrap gap-1"
                  data-testid="library-detail-languages"
                >
                  {languages.map((l) => (
                    <button
                      key={l}
                      type="button"
                      data-locale={l}
                      title={LOCALE_NAMES[l as AppLocale] ?? l}
                      aria-pressed={l === locale}
                      onClick={() => onLocaleChange(l)}
                      className={cn(
                        "rounded-md border px-2 py-0.5 text-xs font-medium",
                        l === locale
                          ? "bg-primary text-primary-foreground border-primary"
                          : "hover:bg-muted",
                      )}
                    >
                      {localeShortLabel(l)}
                    </button>
                  ))}
                </div>
              ) : (
                <p className="text-muted-foreground text-sm">
                  {t("library.detail.no_versions")}
                </p>
              )}
            </section>
            <section className="space-y-2 px-4 pb-4">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <h3 className="text-sm font-medium">
                  {t("library.detail.history", {
                    language: LOCALE_NAMES[locale as AppLocale] ?? locale,
                  })}
                </h3>
                <div className="flex gap-2">
                  {history[0] ? (
                    <Button
                      type="button"
                      size="sm"
                      data-testid="library-detail-download"
                      disabled={downloading}
                      onClick={() => downloadOne(history[0].uuid)}
                    >
                      <Download className="size-4" />
                      {t("library.actions.download")}
                    </Button>
                  ) : null}
                  {canManage ? (
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      data-testid="library-detail-upload"
                      onClick={() => onUpload(item)}
                    >
                      <Upload className="size-4" />
                      {t("library.actions.upload")}
                    </Button>
                  ) : null}
                </div>
              </div>
              <div data-testid="library-version-history">
                <EntityTable
                  columns={columns}
                  data={history}
                  getRowId={(row) => row.uuid}
                  manual={CLIENT_SIDE_MANUAL}
                  isLoading={versionsQuery.isLoading}
                  isError={versionsQuery.isError}
                  onRetry={() => void versionsQuery.refetch()}
                  emptyTitle={t("library.detail.no_versions_locale")}
                  emptyDescription=""
                  initialState={{
                    sorting: [{ id: "version_no", desc: true }],
                    pagination: { pageIndex: 0, pageSize: 10 },
                  }}
                  features={{
                    persistKey: LIBRARY_VERSIONS_PERSIST_KEY,
                    rowSelection: false,
                    viewMode: false,
                    globalFilter: false,
                  }}
                />
              </div>
            </section>
          </div>
        ) : null}
      </SheetContent>
    </Sheet>
  );
}
