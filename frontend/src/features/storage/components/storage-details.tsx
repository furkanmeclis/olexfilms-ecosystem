"use client";

import { Copy, Download, RotateCcw, Share2 } from "lucide-react";
import { useState } from "react";

import { Timeline } from "@/components/common/timeline";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { StorageAccessBadge } from "@/features/storage/components/storage-access-badge";
import { StorageConfirmDialog } from "@/features/storage/components/storage-confirm-dialog";
import { StorageFileThumbnail } from "@/features/storage/components/storage-file-thumbnail";
import { isPreviewable } from "@/features/storage/lib/preview";
import {
  useStorageActivity,
  useStorageVersions,
  useRestoreVersion,
} from "@/features/storage/hooks/use-storage";
import {
  useStorageLinks,
  useStorageShares,
  useRevokeLink,
} from "@/features/storage/hooks/use-storage-sharing";
import { formatBytes } from "@/features/storage/lib/format";
import { storageService } from "@/features/storage/services/storage.service";
import type { StorageObject } from "@/features/storage/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

function DetailRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="grid grid-cols-[140px_1fr] gap-2 py-1 text-sm">
      <span className="text-muted-foreground">{label}</span>
      <span className="break-all">{value || "—"}</span>
    </div>
  );
}

export function StorageDetailsPanel({
  object,
  open,
  onOpenChange,
  canWrite,
  onShare,
  onPreview,
}: {
  object: StorageObject | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  canWrite: boolean;
  onShare?: (object: StorageObject) => void;
  onPreview?: (object: StorageObject) => void;
}) {
  const { t, locale } = useLocale();
  const [restoreVersionId, setRestoreVersionId] = useState<string | null>(null);
  const versionsQuery = useStorageVersions(
    object?.kind === "file" ? object.key : null,
  );
  const activityQuery = useStorageActivity(object?.key ?? null);
  const sharesQuery = useStorageShares(object?.key ?? null);
  const linksQuery = useStorageLinks(object?.key ?? null);
  const restoreVersion = useRestoreVersion();
  const revokeLink = useRevokeLink();

  if (!object) return null;

  const copy = async (value: string) => {
    await navigator.clipboard.writeText(value);
    appToast.success(t("storage.copied"));
  };

  return (
    <>
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent side="right" className="w-full sm:max-w-lg">
          <SheetHeader>
            <SheetTitle>{object.name}</SheetTitle>
          </SheetHeader>
          <Tabs defaultValue="general" className="mt-4">
            <TabsList className="grid w-full grid-cols-3">
              <TabsTrigger value="general">
                {t("storage.details_general")}
              </TabsTrigger>
              <TabsTrigger value="object">
                {t("storage.details_object")}
              </TabsTrigger>
              <TabsTrigger value="more">{t("storage.activity")}</TabsTrigger>
            </TabsList>
            <ScrollArea className="h-[calc(100vh-12rem)] pe-3">
              <TabsContent value="general" className="space-y-4">
                {object.kind === "file" ? (
                  <div className="flex justify-center">
                    <StorageFileThumbnail
                      object={object}
                      className="size-32 rounded-lg"
                      iconClassName="size-10"
                    />
                  </div>
                ) : null}
                <DetailRow
                  label={t("storage.field_name")}
                  value={object.name}
                />
                <DetailRow
                  label={t("storage.field_type")}
                  value={t(
                    `storage.kind_${object.file_kind}` as "storage.kind_unknown",
                  )}
                />
                <DetailRow
                  label={t("storage.field_mime")}
                  value={object.mime_type}
                />
                <DetailRow
                  label={t("storage.field_size")}
                  value={
                    object.kind === "folder" ? "—" : formatBytes(object.size)
                  }
                />
                <DetailRow
                  label={t("storage.field_created")}
                  value={datetime(
                    object.created_at,
                    "dd.MM.yyyy HH:mm",
                    locale,
                  )}
                />
                <DetailRow
                  label={t("storage.field_updated")}
                  value={datetime(
                    object.updated_at,
                    "dd.MM.yyyy HH:mm",
                    locale,
                  )}
                />
                <DetailRow
                  label={t("storage.field_owner")}
                  value={object.owner ?? ""}
                />
                <DetailRow
                  label={t("storage.field_class")}
                  value={object.storage_class ?? ""}
                />
                <div className="flex items-center gap-2">
                  <span className="text-muted-foreground text-sm">
                    {t("storage.details_access")}
                  </span>
                  <StorageAccessBadge access={object.access} />
                </div>
                <div className="flex flex-wrap gap-2 pt-2">
                  {object.kind === "file" ? (
                    <>
                      {isPreviewable(object) && onPreview ? (
                        <Button
                          type="button"
                          size="sm"
                          variant="outline"
                          onClick={() => onPreview(object)}
                        >
                          {t("storage.preview")}
                        </Button>
                      ) : null}
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        onClick={() =>
                          void storageService.downloadFile(
                            object.key,
                            object.name,
                          )
                        }
                      >
                        <Download /> {t("storage.download")}
                      </Button>
                      {canWrite && onShare ? (
                        <Button
                          type="button"
                          size="sm"
                          variant="outline"
                          onClick={() => onShare(object)}
                        >
                          <Share2 /> {t("storage.share")}
                        </Button>
                      ) : null}
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        onClick={() =>
                          copy(storageService.previewUrl(object.key))
                        }
                      >
                        <Copy /> {t("storage.copy_link")}
                      </Button>
                    </>
                  ) : null}
                </div>
              </TabsContent>
              <TabsContent value="object" className="space-y-4">
                <DetailRow label={t("storage.field_key")} value={object.key} />
                <DetailRow
                  label={t("storage.field_bucket")}
                  value={object.bucket}
                />
                <DetailRow
                  label={t("storage.field_etag")}
                  value={object.etag ?? ""}
                />
                <DetailRow
                  label={t("storage.field_version")}
                  value={object.version_id ?? ""}
                />
                <DetailRow
                  label={t("storage.field_disposition")}
                  value={object.content_disposition ?? ""}
                />
                <DetailRow
                  label={t("storage.field_cache")}
                  value={object.cache_control ?? ""}
                />
                <div>
                  <p className="text-muted-foreground mb-2 text-sm">
                    {t("storage.field_metadata")}
                  </p>
                  {Object.keys(object.metadata).length ? (
                    Object.entries(object.metadata).map(([key, value]) => (
                      <DetailRow key={key} label={key} value={value} />
                    ))
                  ) : (
                    <p className="text-muted-foreground text-sm">—</p>
                  )}
                </div>
                {object.kind === "file" ? (
                  <div>
                    <p className="mb-2 font-medium">{t("storage.versions")}</p>
                    <div className="space-y-2">
                      {(versionsQuery.data ?? []).map((version) => (
                        <div
                          key={version.version_id || version.label}
                          className="flex items-center justify-between rounded-md border p-2 text-sm"
                        >
                          <div>
                            <p className="font-medium">
                              {version.label}{" "}
                              {version.is_latest
                                ? `(${t("storage.current")})`
                                : ""}
                            </p>
                            <p className="text-muted-foreground text-xs">
                              {formatBytes(version.size)} ·{" "}
                              {datetime(
                                version.created_at,
                                "dd.MM.yyyy HH:mm",
                                locale,
                              )}
                            </p>
                          </div>
                          {canWrite && !version.is_latest ? (
                            <Button
                              type="button"
                              size="sm"
                              variant="outline"
                              onClick={() =>
                                setRestoreVersionId(version.version_id)
                              }
                            >
                              <RotateCcw className="size-3.5" />
                            </Button>
                          ) : null}
                        </div>
                      ))}
                    </div>
                  </div>
                ) : null}
              </TabsContent>
              <TabsContent value="more" className="space-y-6">
                <div>
                  <p className="mb-2 font-medium">{t("storage.shared_with")}</p>
                  {(sharesQuery.data ?? []).length ? (
                    <div className="space-y-2">
                      {sharesQuery.data?.map((share) => (
                        <div
                          key={share.uuid}
                          className="rounded-md border p-2 text-sm"
                        >
                          <p className="font-medium">
                            {share.name || share.email}
                          </p>
                          <p className="text-muted-foreground text-xs">
                            {t(
                              `storage.role_${share.role}` as "storage.role_viewer",
                            )}
                          </p>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <p className="text-muted-foreground text-sm">—</p>
                  )}
                </div>
                <div>
                  <p className="mb-2 font-medium">
                    {t("storage.details_urls")}
                  </p>
                  {(linksQuery.data ?? []).length ? (
                    <div className="space-y-2">
                      {linksQuery.data?.map((link) => {
                        const url =
                          link.kind === "public" && link.slug
                            ? storageService.publicSharePageUrl(link.slug)
                            : link.url;
                        return (
                          <div
                            key={link.uuid}
                            className="space-y-2 rounded-md border p-2 text-sm"
                          >
                            <div className="flex flex-wrap items-center gap-2">
                              <Badge variant="outline">
                                {link.kind === "signed"
                                  ? t("storage.link_signed")
                                  : t("storage.link_public")}
                              </Badge>
                              <Badge variant="secondary">{link.status}</Badge>
                            </div>
                            {url ? (
                              <p className="text-muted-foreground text-xs break-all">
                                {url}
                              </p>
                            ) : (
                              <p className="text-muted-foreground text-xs">
                                {t("storage.signed_url_hidden")}
                              </p>
                            )}
                            <div className="flex flex-wrap gap-2">
                              {url ? (
                                <Button
                                  type="button"
                                  size="sm"
                                  variant="outline"
                                  onClick={() => void copy(url)}
                                >
                                  <Copy /> {t("storage.copy_url")}
                                </Button>
                              ) : null}
                              {canWrite && link.status === "active" ? (
                                <Button
                                  type="button"
                                  size="sm"
                                  variant="destructive"
                                  onClick={() =>
                                    revokeLink.mutate({
                                      uuid: link.uuid,
                                      key: object.key,
                                    })
                                  }
                                >
                                  {t("storage.revoke_link")}
                                </Button>
                              ) : null}
                            </div>
                          </div>
                        );
                      })}
                    </div>
                  ) : (
                    <p className="text-muted-foreground text-sm">—</p>
                  )}
                </div>
                <div>
                  <p className="mb-2 font-medium">{t("storage.activity")}</p>
                  <Timeline
                    items={(activityQuery.data?.items ?? []).map((item) => ({
                      id: item.uuid,
                      title: item.action,
                      description: item.actor,
                      meta: datetime(item.created_at, "dd.MM HH:mm", locale),
                    }))}
                  />
                </div>
              </TabsContent>
            </ScrollArea>
          </Tabs>
        </SheetContent>
      </Sheet>
      <StorageConfirmDialog
        open={Boolean(restoreVersionId)}
        onOpenChange={(next) => !next && setRestoreVersionId(null)}
        title={t("storage.restore_version_title")}
        description={t("storage.restore_version_description")}
        onConfirm={() => {
          if (!restoreVersionId) return;
          restoreVersion.mutate({
            key: object.key,
            version_id: restoreVersionId,
          });
        }}
      />
    </>
  );
}
