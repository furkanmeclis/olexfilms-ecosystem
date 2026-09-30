"use client";

import { Download, RotateCw, Share2, ZoomIn, ZoomOut } from "lucide-react";
import { useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { StorageMediaPreview } from "@/features/storage/components/storage-media-preview";
import { StorageSpreadsheetPreviewPanel } from "@/features/storage/components/storage-spreadsheet-preview";
import { useStorageSpreadsheetPreview } from "@/features/storage/hooks/use-storage-spreadsheet-preview";
import { isPreviewable, previewKind } from "@/features/storage/lib/preview";
import { storageService } from "@/features/storage/services/storage.service";
import type { StorageObject } from "@/features/storage/types";
import { useLocale } from "@/providers/locale-provider";

export function StoragePreviewDialog({
  object,
  open,
  onOpenChange,
  onShare,
  canWrite,
}: {
  object: StorageObject | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onShare?: (object: StorageObject) => void;
  canWrite?: boolean;
}) {
  const { t } = useLocale();
  const [zoom, setZoom] = useState(1);
  const [rotation, setRotation] = useState(0);

  const url = useMemo(() => {
    if (!object || object.kind === "folder") return "";
    return storageService.previewUrl(object.key, object.version_id);
  }, [object]);

  const kind = object ? previewKind(object) : "none";
  const spreadsheetPreview = useStorageSpreadsheetPreview(
    object?.key ?? null,
    object?.name ?? "",
    object?.mime_type ?? "",
    open && kind === "spreadsheet",
    object?.version_id,
  );

  if (!object) return null;

  const previewable = isPreviewable(object);
  const isImage = kind === "image";
  const isSpreadsheet = kind === "spreadsheet";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] max-w-5xl overflow-hidden">
        <DialogHeader>
          <DialogTitle>{object.name}</DialogTitle>
        </DialogHeader>
        <div className="flex flex-wrap items-center gap-2">
          {isImage ? (
            <>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setZoom((z) => Math.min(3, z + 0.25))}
              >
                <ZoomIn /> {t("storage.zoom_in")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setZoom((z) => Math.max(0.5, z - 0.25))}
              >
                <ZoomOut /> {t("storage.zoom_out")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setRotation((r) => r + 90)}
              >
                <RotateCw /> {t("storage.rotate")}
              </Button>
            </>
          ) : null}
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() =>
              void storageService.downloadFile(object.key, object.name)
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
          {url ? (
            <Button type="button" size="sm" variant="outline" asChild>
              <a href={url} target="_blank" rel="noreferrer">
                {t("storage.open_original")}
              </a>
            </Button>
          ) : null}
        </div>
        <div className="bg-muted/30 mt-4 flex max-h-[65vh] min-h-64 items-center justify-center overflow-auto rounded-lg border p-4">
          {isSpreadsheet ? (
            <StorageSpreadsheetPreviewPanel
              sheet={spreadsheetPreview.data}
              isLoading={spreadsheetPreview.isLoading}
              isError={spreadsheetPreview.isError}
              onRetry={() => void spreadsheetPreview.refetch()}
            />
          ) : previewable ? (
            isImage ? (
              // eslint-disable-next-line @next/next/no-img-element
              <img
                src={url}
                alt={object.name}
                className="max-h-full max-w-full object-contain transition-transform"
                style={{
                  transform: `scale(${zoom}) rotate(${rotation}deg)`,
                }}
              />
            ) : (
              <StorageMediaPreview url={url} name={object.name} kind={kind} />
            )
          ) : (
            <StorageMediaPreview url={url} name={object.name} kind="none" />
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
