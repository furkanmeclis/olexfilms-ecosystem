"use client";

import { X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import type { UploadJob } from "@/features/storage/hooks/use-storage-upload";
import { formatBytes, formatSpeed } from "@/features/storage/lib/format";
import { useLocale } from "@/providers/locale-provider";

function panelTitle(
  jobs: UploadJob[],
  active: UploadJob[],
  t: (key: string, params?: Record<string, string | number>) => string,
) {
  if (active.length > 0) {
    return t("storage.uploading", { count: active.length });
  }

  const errors = jobs.filter((job) => job.status === "error").length;
  if (errors > 0) {
    return t("storage.upload_failed", { count: errors });
  }

  const done = jobs.filter((job) => job.status === "done").length;
  return t("storage.upload_complete", { count: done || jobs.length });
}

export function StorageUploadManager({
  jobs,
  onCancel,
  onRetry,
  onDismiss,
}: {
  jobs: UploadJob[];
  onCancel: (id: string) => void;
  onRetry: (id: string) => void;
  onDismiss: () => void;
}) {
  const { t } = useLocale();
  if (!jobs.length) return null;

  const active = jobs.filter(
    (job) => job.status === "uploading" || job.status === "queued",
  );

  return (
    <div className="bg-card fixed end-4 bottom-4 z-40 w-full max-w-sm rounded-lg border p-4 shadow-lg">
      <div className="mb-3 flex items-center justify-between gap-2">
        <p className="text-sm font-medium">{panelTitle(jobs, active, t)}</p>
        <Button type="button" size="sm" variant="ghost" onClick={onDismiss}>
          {t("storage.close")}
        </Button>
      </div>
      <div className="max-h-64 space-y-3 overflow-y-auto">
        {jobs.map((job) => (
          <div key={job.id} className="space-y-1">
            <div className="flex items-center justify-between gap-2 text-sm">
              <span className="truncate">{job.name}</span>
              <span className="text-muted-foreground shrink-0">
                {job.status === "done"
                  ? "100%"
                  : job.status === "error"
                    ? "!"
                    : `${job.progress}%`}
              </span>
            </div>
            <Progress value={job.progress} className="h-1.5" />
            <div className="text-muted-foreground flex items-center justify-between text-xs">
              <span>{formatBytes(job.size)}</span>
              {job.status === "uploading" ? (
                <span>
                  {job.progress >= 100
                    ? t("storage.upload_finalizing")
                    : formatSpeed(job.speedBps)}
                </span>
              ) : null}
              {job.status === "error" ? (
                <Button
                  type="button"
                  size="sm"
                  variant="link"
                  className="h-auto p-0"
                  onClick={() => onRetry(job.id)}
                >
                  {t("storage.retry_upload")}
                </Button>
              ) : null}
              {job.status === "uploading" || job.status === "queued" ? (
                <Button
                  type="button"
                  size="icon"
                  variant="ghost"
                  className="size-6"
                  onClick={() => onCancel(job.id)}
                >
                  <X className="size-3.5" />
                </Button>
              ) : null}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
