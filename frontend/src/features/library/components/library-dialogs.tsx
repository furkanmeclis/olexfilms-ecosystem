"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Progress } from "@/components/ui/progress";
import { Textarea } from "@/components/ui/textarea";
import { LOCALE_NAMES } from "@/config/i18n";
import {
  flattenFolderTree,
  folderSubtreeIds,
  formatBytes,
  isFolderNotEmptyError,
  LIBRARY_ACCESS_LEVELS,
  LIBRARY_LOCALES,
  LIBRARY_MAX_UPLOAD_BYTES,
  LIBRARY_UPLOAD_ACCEPT,
  parseTags,
  uploadFileError,
  type FolderNode,
} from "@/features/library/lib/library";
import {
  libraryKeys,
  libraryService,
  type LibraryAccessLevel,
  type LibraryFolder,
  type LibraryItem,
} from "@/features/library/services/library.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

export const librarySelectClass =
  "border-input bg-background focus-visible:ring-ring flex h-9 w-full rounded-md border px-3 py-1 text-sm focus-visible:ring-2 focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50";

function useErrorToast() {
  const { t } = useLocale();
  return (err: unknown) =>
    toast.error(isApiError(err) ? err.message : t("common.error_generic"));
}

function FolderOptions({
  tree,
  exclude,
}: {
  tree: FolderNode[];
  exclude?: Set<string>;
}) {
  return flattenFolderTree(tree)
    .filter((f) => !exclude?.has(f.uuid))
    .map((f) => (
      <option key={f.uuid} value={f.uuid}>
        {`${"— ".repeat(f.depth)}${f.name}`}
      </option>
    ));
}

/** Create a folder (optionally under a parent) or rename / move one. */
export function LibraryFolderDialog({
  open,
  onOpenChange,
  tree,
  folder,
  defaultParent,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  tree: FolderNode[];
  /** Folder to rename; omitted → create. */
  folder?: LibraryFolder | null;
  defaultParent?: string | null;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const onError = useErrorToast();
  const [name, setName] = useState(folder?.name ?? "");
  const [parent, setParent] = useState(
    folder ? (folder.parent_uuid ?? "") : (defaultParent ?? ""),
  );
  const save = useMutation({
    mutationFn: () => {
      const body = {
        name: name.trim(),
        parent_uuid: parent || null,
        sort_order: folder?.sort_order ?? 0,
      };
      return folder
        ? libraryService.updateFolder(folder.uuid, body)
        : libraryService.createFolder(body);
    },
    onSuccess: () => {
      toast.success(
        t(folder ? "library.folders.renamed" : "library.folders.created"),
      );
      void qc.invalidateQueries({ queryKey: libraryKeys.folders() });
      onOpenChange(false);
    },
    onError,
  });
  const exclude = folder ? folderSubtreeIds(tree, folder.uuid) : undefined;

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    save.mutate();
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent data-testid="library-folder-dialog">
        <form onSubmit={submit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>
              {t(folder ? "library.folders.rename" : "library.folders.create")}
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="library-folder-name">
              {t("library.folders.name")}
            </Label>
            <Input
              id="library-folder-name"
              value={name}
              maxLength={255}
              required
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="library-folder-parent">
              {t("library.folders.parent")}
            </Label>
            <select
              id="library-folder-parent"
              className={librarySelectClass}
              value={parent}
              onChange={(e) => setParent(e.target.value)}
            >
              <option value="">{t("library.folders.root")}</option>
              <FolderOptions tree={tree} exclude={exclude} />
            </select>
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={!name.trim() || save.isPending}>
              {t("common.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Deletes an empty folder. The backend answers 409 LIBRARY_FOLDER_NOT_EMPTY
 * for a folder with items or subfolders; the dialog explains it inline.
 */
export function LibraryDeleteFolderDialog({
  folder,
  onOpenChange,
  onDeleted,
}: {
  folder: LibraryFolder | null;
  onOpenChange: (open: boolean) => void;
  onDeleted: (uuid: string) => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const onError = useErrorToast();
  const [notEmpty, setNotEmpty] = useState(false);
  const remove = useMutation({
    mutationFn: (uuid: string) => libraryService.deleteFolder(uuid),
    onSuccess: (_, uuid) => {
      toast.success(t("library.folders.deleted"));
      void qc.invalidateQueries({ queryKey: libraryKeys.folders() });
      onDeleted(uuid);
      onOpenChange(false);
    },
    onError: (err) => {
      if (isFolderNotEmptyError(err)) {
        setNotEmpty(true);
        toast.error(t("library.folders.not_empty"));
        return;
      }
      onError(err);
    },
  });

  return (
    <Dialog
      open={Boolean(folder)}
      onOpenChange={(open) => {
        if (!open) setNotEmpty(false);
        onOpenChange(open);
      }}
    >
      <DialogContent data-testid="library-delete-folder-dialog">
        <DialogHeader>
          <DialogTitle>{t("library.folders.delete")}</DialogTitle>
          <DialogDescription>
            {t("library.folders.delete_confirm", { name: folder?.name ?? "" })}
          </DialogDescription>
        </DialogHeader>
        {notEmpty ? (
          <p
            role="alert"
            data-testid="library-folder-not-empty"
            className="border-destructive/40 bg-destructive/10 text-destructive rounded-md border px-3 py-2 text-sm"
          >
            {t("library.folders.not_empty")}
          </p>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            data-testid="library-delete-folder-confirm"
            disabled={!folder || remove.isPending || notEmpty}
            onClick={() => folder && remove.mutate(folder.uuid)}
          >
            {t("library.folders.delete")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** Create or edit an item: folder, access level, role, tags. */
export function LibraryItemDialog({
  open,
  onOpenChange,
  tree,
  item,
  defaultFolder,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  tree: FolderNode[];
  item?: LibraryItem | null;
  defaultFolder?: string | null;
  onCreated?: (item: LibraryItem) => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const onError = useErrorToast();
  const [name, setName] = useState(item?.name ?? "");
  const [description, setDescription] = useState(item?.description ?? "");
  const [folder, setFolder] = useState(
    item ? (item.folder_uuid ?? "") : (defaultFolder ?? ""),
  );
  const [access, setAccess] = useState<LibraryAccessLevel>(
    item?.access_level ?? "all_network",
  );
  const [role, setRole] = useState(item?.role_slug ?? "");
  const [tags, setTags] = useState((item?.tags ?? []).join(", "));
  const save = useMutation({
    mutationFn: () => {
      const body = {
        name: name.trim(),
        description: description.trim() || null,
        folder_uuid: folder || null,
        access_level: access,
        role_slug: role.trim() || null,
        tags: parseTags(tags),
      };
      return item
        ? libraryService.updateItem(item.uuid, body)
        : libraryService.createItem(body);
    },
    onSuccess: (saved) => {
      toast.success(
        t(item ? "library.items.updated" : "library.items.created"),
      );
      void qc.invalidateQueries({ queryKey: libraryKeys.items() });
      onOpenChange(false);
      if (!item) onCreated?.(saved);
    },
    onError,
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    save.mutate();
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent data-testid="library-item-dialog" className="sm:max-w-lg">
        <form onSubmit={submit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>
              {t(item ? "library.items.edit" : "library.items.create")}
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="library-item-name">
              {t("library.fields.name")}
            </Label>
            <Input
              id="library-item-name"
              value={name}
              maxLength={255}
              required
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="library-item-description">
              {t("library.fields.description")}
            </Label>
            <Textarea
              id="library-item-description"
              value={description}
              rows={3}
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="library-item-folder">
                {t("library.fields.folder")}
              </Label>
              <select
                id="library-item-folder"
                className={librarySelectClass}
                value={folder}
                onChange={(e) => setFolder(e.target.value)}
              >
                <option value="">{t("library.folders.root")}</option>
                <FolderOptions tree={tree} />
              </select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="library-item-access">
                {t("library.fields.access_level")}
              </Label>
              <select
                id="library-item-access"
                className={librarySelectClass}
                value={access}
                onChange={(e) =>
                  setAccess(e.target.value as LibraryAccessLevel)
                }
              >
                {LIBRARY_ACCESS_LEVELS.map((level) => (
                  <option key={level} value={level}>
                    {t(`library.access.${level}`)}
                  </option>
                ))}
              </select>
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="library-item-role">
              {t("library.fields.role_slug")}
            </Label>
            <Input
              id="library-item-role"
              value={role}
              placeholder={t("library.fields.role_slug_hint")}
              onChange={(e) => setRole(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="library-item-tags">
              {t("library.fields.tags")}
            </Label>
            <Input
              id="library-item-tags"
              value={tags}
              placeholder={t("library.fields.tags_hint")}
              onChange={(e) => setTags(e.target.value)}
            />
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={!name.trim() || save.isPending}>
              {t("common.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Uploads a new version of one language. Files over 50 MB are rejected
 * before any request; the bar follows the XHR upload progress.
 */
export function LibraryUploadDialog({
  item,
  defaultLocale,
  onOpenChange,
}: {
  item: LibraryItem | null;
  defaultLocale: string;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const onError = useErrorToast();
  const [locale, setLocale] = useState(defaultLocale);
  const [file, setFile] = useState<File | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [progress, setProgress] = useState<number | null>(null);
  const upload = useMutation({
    mutationFn: (input: { uuid: string; file: File; locale: string }) =>
      libraryService.uploadVersion(
        input.uuid,
        { file: input.file, locale: input.locale },
        (p) =>
          setProgress(p.total ? Math.round((p.loaded / p.total) * 100) : 0),
      ),
    onSuccess: (_, input) => {
      toast.success(t("library.upload.done"));
      void qc.invalidateQueries({ queryKey: libraryKeys.items() });
      void qc.invalidateQueries({ queryKey: libraryKeys.versions(input.uuid) });
      close(false);
    },
    onError: (err) => {
      setProgress(null);
      onError(err);
    },
  });

  function close(open: boolean) {
    if (!open) {
      setFile(null);
      setError(null);
      setProgress(null);
    }
    onOpenChange(open);
  }

  const pick = (next: File | null) => {
    setFile(next);
    setError(next ? uploadFileError(next) : null);
  };

  const submit = (e: FormEvent) => {
    e.preventDefault();
    const problem = uploadFileError(file);
    setError(problem);
    if (problem || !item || !file) return;
    setProgress(0);
    upload.mutate({ uuid: item.uuid, file, locale });
  };

  return (
    <Dialog open={Boolean(item)} onOpenChange={close}>
      <DialogContent data-testid="library-upload-dialog">
        <form onSubmit={submit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>{t("library.upload.title")}</DialogTitle>
            <DialogDescription>
              {t("library.upload.description", {
                name: item?.name ?? "",
                max: formatBytes(LIBRARY_MAX_UPLOAD_BYTES),
              })}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="library-upload-locale">
              {t("library.fields.locale")}
            </Label>
            <select
              id="library-upload-locale"
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
          </div>
          <div className="space-y-2">
            <Label htmlFor="library-upload-file">
              {t("library.fields.file")}
            </Label>
            <Input
              id="library-upload-file"
              type="file"
              accept={LIBRARY_UPLOAD_ACCEPT}
              aria-invalid={Boolean(error)}
              onChange={(e) => pick(e.target.files?.[0] ?? null)}
            />
            {file && !error ? (
              <p className="text-muted-foreground text-xs">
                {file.name} · {formatBytes(file.size)}
              </p>
            ) : null}
            {error ? (
              <p
                role="alert"
                data-testid="library-upload-error"
                className="text-destructive text-sm"
              >
                {t(error, { max: formatBytes(LIBRARY_MAX_UPLOAD_BYTES) })}
              </p>
            ) : null}
          </div>
          {progress !== null ? (
            <div className="space-y-1" data-testid="library-upload-progress">
              <Progress value={progress} className="rtl:-scale-x-100" />
              <p className="text-muted-foreground text-xs tabular-nums">
                {t("library.upload.progress", { percent: progress })}
              </p>
            </div>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => close(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button
              type="submit"
              data-testid="library-upload-submit"
              disabled={!file || Boolean(error) || upload.isPending}
            >
              {t("library.upload.submit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/** Archives (soft-deletes) an item. */
export function LibraryArchiveDialog({
  item,
  onOpenChange,
  onArchived,
}: {
  item: LibraryItem | null;
  onOpenChange: (open: boolean) => void;
  onArchived: (uuid: string) => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const onError = useErrorToast();
  const archive = useMutation({
    mutationFn: (uuid: string) => libraryService.archiveItem(uuid),
    onSuccess: (_, uuid) => {
      toast.success(t("library.items.archived"));
      void qc.invalidateQueries({ queryKey: libraryKeys.items() });
      onArchived(uuid);
      onOpenChange(false);
    },
    onError,
  });

  return (
    <Dialog open={Boolean(item)} onOpenChange={onOpenChange}>
      <DialogContent data-testid="library-archive-dialog">
        <DialogHeader>
          <DialogTitle>{t("library.items.archive")}</DialogTitle>
          <DialogDescription>
            {t("library.items.archive_confirm", { name: item?.name ?? "" })}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={!item || archive.isPending}
            onClick={() => item && archive.mutate(item.uuid)}
          >
            {t("library.items.archive")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
