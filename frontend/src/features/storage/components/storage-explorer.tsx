"use client";

import { Menu } from "lucide-react";
import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { EntityPage } from "@/components/entity";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { permissions } from "@/config/permissions";
import { StorageBreadcrumb } from "@/features/storage/components/storage-breadcrumb";
import { StorageConfirmDialog } from "@/features/storage/components/storage-confirm-dialog";
import type { StorageAction } from "@/features/storage/components/storage-context-menu";
import { StorageDetailsPanel } from "@/features/storage/components/storage-details";
import { StorageEmptyState } from "@/features/storage/components/storage-empty-state";
import { StorageFileGrid } from "@/features/storage/components/storage-file-grid";
import { StorageFileList } from "@/features/storage/components/storage-file-list";
import {
  StorageInputDialog,
  type InputMode,
} from "@/features/storage/components/storage-input-dialog";
import {
  StorageShareDialog,
  type ShareTab,
} from "@/features/storage/components/storage-share-dialog";
import { StorageQrDialog } from "@/features/storage/components/storage-link-dialogs";
import { StoragePreviewDialog } from "@/features/storage/components/storage-preview";
import { StorageSidebar } from "@/features/storage/components/storage-sidebar";
import { StorageToolbar } from "@/features/storage/components/storage-toolbar";
import { StorageUploadManager } from "@/features/storage/components/storage-upload-manager";
import { StorageUploadZone } from "@/features/storage/components/storage-upload-zone";
import {
  useCopyFile,
  useCreateFolder,
  useDeleteFiles,
  useMoveFile,
  usePurgeFiles,
  useRenameFile,
  useRestoreFiles,
  useStarFile,
  useStorageFiles,
  useStorageUsage,
} from "@/features/storage/hooks/use-storage";
import { useStorageUpload } from "@/features/storage/hooks/use-storage-upload";
import { joinKey } from "@/features/storage/lib/format";
import { isPreviewable } from "@/features/storage/lib/preview";
import { storageService } from "@/features/storage/services/storage.service";
import type { StorageObject, StorageView } from "@/features/storage/types";
import { usePermission } from "@/providers/permission-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export function StorageExplorer() {
  const { t } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.storage.write);

  const [view, setView] = useState<StorageView>("all");
  const [prefix, setPrefix] = useState("");
  const [query, setQuery] = useState("");
  const [sort, setSort] = useState("name");
  const [viewMode, setViewMode] = useState<"list" | "grid">("list");
  const [kind, setKind] = useState("");
  const [access, setAccess] = useState("");
  const [modifiedFrom, setModifiedFrom] = useState("");
  const [modifiedTo, setModifiedTo] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [detail, setDetail] = useState<StorageObject | null>(null);
  const [preview, setPreview] = useState<StorageObject | null>(null);
  const [inputMode, setInputMode] = useState<InputMode>(null);
  const [inputTarget, setInputTarget] = useState<StorageObject | null>(null);
  const [confirm, setConfirm] = useState<"delete" | "purge" | null>(null);
  const [share, setShare] = useState<{
    object: StorageObject;
    tab: ShareTab;
  } | null>(null);
  const [qr, setQr] = useState<{ object: StorageObject; url: string } | null>(
    null,
  );
  const [mobileNav, setMobileNav] = useState(false);

  const listParams = useMemo(
    () => ({
      prefix: view === "all" ? prefix : undefined,
      view,
      q: query || undefined,
      kind: kind || undefined,
      access: access || undefined,
      sort,
      modified_from: modifiedFrom || undefined,
      modified_to: modifiedTo || undefined,
      limit: 100,
      offset: 0,
      recursive: Boolean(query),
    }),
    [prefix, view, query, kind, access, sort, modifiedFrom, modifiedTo],
  );

  const listQuery = useStorageFiles(listParams);
  const usageQuery = useStorageUsage();
  const upload = useStorageUpload(prefix);
  const createFolder = useCreateFolder();
  const deleteFiles = useDeleteFiles();
  const restoreFiles = useRestoreFiles();
  const purgeFiles = usePurgeFiles();
  const renameFile = useRenameFile();
  const moveFile = useMoveFile();
  const copyFile = useCopyFile();
  const starFile = useStarFile();

  const items = useMemo(
    () => listQuery.data?.items ?? [],
    [listQuery.data?.items],
  );
  const trash = view === "trash";

  const toggleSelect = useCallback((key: string, checked: boolean) => {
    setSelected((current) => {
      const next = new Set(current);
      if (checked) next.add(key);
      else next.delete(key);
      return next;
    });
  }, []);

  const openItem = useCallback(
    (item: StorageObject) => {
      if (item.kind === "folder" && view === "all") {
        setPrefix(item.key.endsWith("/") ? item.key : `${item.key}/`);
        setSelected(new Set());
        return;
      }
      if (item.kind === "file" && isPreviewable(item)) {
        setPreview(item);
        return;
      }
      setDetail(item);
    },
    [view],
  );

  const openShare = useCallback(
    (item: StorageObject, tab: ShareTab = "signed") => {
      setShare({ object: item, tab });
    },
    [],
  );

  const selectedObjects = useMemo(
    () => items.filter((item) => selected.has(item.key)),
    [items, selected],
  );

  const handleAction = useCallback(
    (action: StorageAction, item: StorageObject) => {
      switch (action) {
        case "open":
          openItem(item);
          break;
        case "preview":
          setPreview(item);
          break;
        case "download":
          void storageService.downloadFile(item.key, item.name);
          break;
        case "copy-link":
          void navigator.clipboard
            .writeText(storageService.previewUrl(item.key))
            .then(() => appToast.success(t("storage.copied")));
          break;
        case "public":
          openShare(item, "public");
          break;
        case "signed":
          openShare(item, "signed");
          break;
        case "qr":
          setQr({
            object: item,
            url: storageService.previewUrl(item.key),
          });
          break;
        case "rename":
          setInputTarget(item);
          setInputMode("rename");
          break;
        case "move":
          setInputTarget(item);
          setInputMode("move");
          break;
        case "copy":
          setInputTarget(item);
          setInputMode("copy");
          break;
        case "share":
          openShare(item, "signed");
          break;
        case "star":
          starFile.mutate({ key: item.key, starred: !item.is_starred });
          break;
        case "details":
          setDetail(item);
          break;
        case "versions":
          setDetail(item);
          break;
        case "delete":
          setSelected(new Set([item.key]));
          setConfirm("delete");
          break;
        case "restore":
          restoreFiles.mutate([item.trash_uuid ?? item.key]);
          break;
        case "purge":
          setSelected(new Set([item.trash_uuid ?? item.key]));
          setConfirm("purge");
          break;
      }
    },
    [openItem, openShare, restoreFiles, starFile, t],
  );

  const handleInputSubmit = useCallback(
    (value: string) => {
      if (inputMode === "folder") {
        createFolder.mutate({ prefix, name: value });
        return;
      }
      if (!inputTarget) return;
      if (inputMode === "rename") {
        renameFile.mutate({ key: inputTarget.key, name: value });
        return;
      }
      if (inputMode === "move") {
        const dest = value.endsWith("/")
          ? `${value}${inputTarget.name}`
          : joinKey(value, inputTarget.name);
        moveFile.mutate({ source_key: inputTarget.key, dest_key: dest });
        return;
      }
      if (inputMode === "copy") {
        const dest = value.endsWith("/")
          ? `${value}${inputTarget.name}`
          : joinKey(value, inputTarget.name);
        copyFile.mutate({ source_key: inputTarget.key, dest_key: dest });
      }
    },
    [
      copyFile,
      createFolder,
      inputMode,
      inputTarget,
      moveFile,
      prefix,
      renameFile,
    ],
  );

  const bulkKeys = useMemo(
    () =>
      trash
        ? selectedObjects.map((item) => item.trash_uuid ?? item.key)
        : [...selected],
    [selected, selectedObjects, trash],
  );

  return (
    <EntityPage
      title={t("storage.title")}
      description={t("storage.description")}
      permission={permissions.storage.read}
      forbiddenFallback={<ErrorState title={t("storage.forbidden")} />}
    >
      <div className="flex min-h-[70vh] overflow-hidden rounded-xl border">
        <div className="hidden lg:block">
          <StorageSidebar
            view={view}
            onViewChange={(next) => {
              setView(next);
              setPrefix("");
              setSelected(new Set());
            }}
            usage={usageQuery.data}
          />
        </div>
        <div className="flex min-w-0 flex-1 flex-col">
          <div className="space-y-3 border-b p-4">
            <div className="flex items-center gap-2">
              <Button
                type="button"
                size="icon"
                variant="outline"
                className="lg:hidden"
                onClick={() => setMobileNav(true)}
              >
                <Menu />
              </Button>
              {view === "all" ? (
                <StorageBreadcrumb
                  prefix={prefix}
                  onNavigate={(next) => {
                    setPrefix(next);
                    setSelected(new Set());
                  }}
                />
              ) : (
                <p className="text-sm font-medium">
                  {t(`storage.${view}` as "storage.all_files")}
                </p>
              )}
            </div>
            <StorageToolbar
              view={view}
              query={query}
              sort={sort}
              viewMode={viewMode}
              kind={kind}
              access={access}
              modifiedFrom={modifiedFrom}
              modifiedTo={modifiedTo}
              onQueryChange={setQuery}
              onSortChange={setSort}
              onViewModeChange={setViewMode}
              onKindChange={setKind}
              onAccessChange={setAccess}
              onModifiedFromChange={setModifiedFrom}
              onModifiedToChange={setModifiedTo}
              onNewFolder={() => setInputMode("folder")}
              onUpload={() =>
                document.getElementById("storage-file-input")?.click()
              }
            />
            {selected.size > 0 ? (
              <div className="bg-muted/40 flex flex-wrap items-center gap-2 rounded-md border px-3 py-2 text-sm">
                <span>{t("storage.selected", { count: selected.size })}</span>
                {trash ? (
                  <>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() => restoreFiles.mutate(bulkKeys)}
                    >
                      {t("storage.restore")}
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant="destructive"
                      onClick={() => setConfirm("purge")}
                    >
                      {t("storage.purge")}
                    </Button>
                  </>
                ) : (
                  <>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() =>
                        selectedObjects.forEach(
                          (item) =>
                            void storageService.downloadFile(
                              item.key,
                              item.name,
                            ),
                        )
                      }
                    >
                      {t("storage.download")}
                    </Button>
                    {canWrite &&
                    selected.size === 1 &&
                    selectedObjects[0]?.kind === "file" ? (
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        onClick={() => openShare(selectedObjects[0]!, "signed")}
                      >
                        {t("storage.share")}
                      </Button>
                    ) : null}
                    {canWrite ? (
                      <Button
                        type="button"
                        size="sm"
                        variant="destructive"
                        onClick={() => setConfirm("delete")}
                      >
                        {t("storage.delete")}
                      </Button>
                    ) : null}
                  </>
                )}
              </div>
            ) : null}
          </div>
          <div className="flex-1 space-y-4 overflow-auto p-4">
            {canWrite && view === "all" && !query ? (
              <StorageUploadZone
                onFiles={(files) => void upload.start(files)}
              />
            ) : null}
            {listQuery.isError ? (
              <ErrorState
                title={t("storage.error_title")}
                onRetry={() => void listQuery.refetch()}
                retryLabel={t("storage.retry")}
              />
            ) : items.length === 0 && !listQuery.isLoading ? (
              <StorageEmptyState
                view={view}
                canWrite={canWrite}
                onUpload={() =>
                  document.getElementById("storage-file-input")?.click()
                }
              />
            ) : viewMode === "grid" ? (
              <StorageFileGrid
                items={items}
                loading={listQuery.isLoading}
                selected={selected}
                canWrite={canWrite}
                trash={trash}
                onToggle={toggleSelect}
                onOpen={openItem}
                onAction={handleAction}
              />
            ) : (
              <StorageFileList
                items={items}
                loading={listQuery.isLoading}
                selected={selected}
                canWrite={canWrite}
                trash={trash}
                onToggle={toggleSelect}
                onOpen={openItem}
                onAction={handleAction}
              />
            )}
          </div>
        </div>
      </div>

      <input
        id="storage-file-input"
        type="file"
        multiple
        className="hidden"
        onChange={(event) => {
          const files = event.target.files;
          if (files?.length) void upload.start(Array.from(files));
          event.target.value = "";
        }}
      />

      <StorageUploadManager
        jobs={upload.jobs}
        onCancel={upload.cancel}
        onRetry={upload.retry}
        onDismiss={upload.dismiss}
      />

      <StorageDetailsPanel
        object={detail}
        open={Boolean(detail)}
        onOpenChange={(open) => !open && setDetail(null)}
        canWrite={canWrite}
        onShare={(item) => openShare(item, "signed")}
        onPreview={(item) => {
          setDetail(null);
          setPreview(item);
        }}
      />
      <StoragePreviewDialog
        object={preview}
        open={Boolean(preview)}
        onOpenChange={(open) => !open && setPreview(null)}
        canWrite={canWrite}
        onShare={(item) => {
          setPreview(null);
          openShare(item, "signed");
        }}
      />
      <StorageInputDialog
        mode={inputMode}
        object={inputTarget}
        prefix={prefix}
        open={Boolean(inputMode)}
        onOpenChange={(open) => {
          if (!open) {
            setInputMode(null);
            setInputTarget(null);
          }
        }}
        onSubmit={handleInputSubmit}
      />
      <StorageShareDialog
        object={share?.object ?? null}
        tab={share?.tab ?? "signed"}
        open={Boolean(share)}
        canWrite={canWrite}
        onOpenChange={(open) => !open && setShare(null)}
        onQr={(url) => {
          if (!share?.object) return;
          setQr({ object: share.object, url });
        }}
      />
      <StorageQrDialog
        object={qr?.object ?? null}
        url={qr?.url ?? ""}
        open={Boolean(qr)}
        onOpenChange={(open) => !open && setQr(null)}
      />
      <StorageConfirmDialog
        open={confirm === "delete"}
        onOpenChange={(open) => !open && setConfirm(null)}
        title={t("storage.delete_title")}
        description={t("storage.delete_description")}
        destructive
        onConfirm={() => {
          deleteFiles.mutate(bulkKeys);
          setSelected(new Set());
        }}
      />
      <StorageConfirmDialog
        open={confirm === "purge"}
        onOpenChange={(open) => !open && setConfirm(null)}
        title={t("storage.purge_title")}
        description={t("storage.purge_description")}
        destructive
        confirmLabel={t("storage.purge")}
        onConfirm={() => {
          purgeFiles.mutate(bulkKeys);
          setSelected(new Set());
        }}
      />

      <Sheet open={mobileNav} onOpenChange={setMobileNav}>
        <SheetContent side="left" className="w-72 p-0">
          <SheetHeader className="p-4">
            <SheetTitle>{t("storage.mobile_menu")}</SheetTitle>
          </SheetHeader>
          <StorageSidebar
            view={view}
            onViewChange={(next) => {
              setView(next);
              setPrefix("");
              setSelected(new Set());
              setMobileNav(false);
            }}
            usage={usageQuery.data}
          />
        </SheetContent>
      </Sheet>
    </EntityPage>
  );
}
