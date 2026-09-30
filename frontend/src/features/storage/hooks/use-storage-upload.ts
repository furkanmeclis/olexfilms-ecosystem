"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";

import { storageKeys } from "@/features/storage/hooks/query-keys";
import { storageService } from "@/features/storage/services/storage.service";
import { appToast } from "@/providers/toast-provider";
import { useLocale } from "@/providers/locale-provider";

export type UploadJob = {
  id: string;
  name: string;
  size: number;
  progress: number;
  status: "queued" | "uploading" | "done" | "error" | "cancelled";
  error?: string;
  speedBps?: number;
  etaSeconds?: number;
  file: File;
  controller: AbortController;
};

export function useStorageUpload(prefix: string) {
  const queryClient = useQueryClient();
  const { t } = useLocale();
  const [jobs, setJobs] = useState<UploadJob[]>([]);

  const update = useCallback((id: string, patch: Partial<UploadJob>) => {
    setJobs((current) =>
      current.map((job) => (job.id === id ? { ...job, ...patch } : job)),
    );
  }, []);

  const start = useCallback(
    async (files: File[]) => {
      const created: UploadJob[] = files.map((file) => ({
        id: `${file.name}-${file.size}-${crypto.randomUUID()}`,
        name: file.name,
        size: file.size,
        progress: 0,
        status: "queued",
        file,
        controller: new AbortController(),
      }));
      setJobs((current) => [...created, ...current].slice(0, 20));

      for (const job of created) {
        update(job.id, { status: "uploading" });
        const started = Date.now();
        try {
          await storageService.uploadFile({
            file: job.file,
            prefix,
            signal: job.controller.signal,
            onProgress: ({ loaded, total }) => {
              const elapsed = Math.max(0.5, (Date.now() - started) / 1000);
              const speed = loaded / elapsed;
              const remaining = total > loaded ? (total - loaded) / speed : 0;
              update(job.id, {
                progress: total ? Math.round((loaded / total) * 100) : 0,
                speedBps: speed,
                etaSeconds: remaining,
              });
            },
          });
          update(job.id, { status: "done", progress: 100 });
        } catch (error) {
          if (job.controller.signal.aborted) {
            update(job.id, { status: "cancelled" });
            continue;
          }
          update(job.id, {
            status: "error",
            error: error instanceof Error ? error.message : "failed",
          });
        }
      }
      void queryClient.invalidateQueries({ queryKey: storageKeys.all });
      if (created.length === 1) {
        appToast.success(t("storage.toast.uploaded"));
      } else {
        appToast.success(
          t("storage.toast.uploaded_many", { count: created.length }),
        );
      }
    },
    [prefix, queryClient, t, update],
  );

  const cancel = useCallback((id: string) => {
    setJobs((current) => {
      const job = current.find((item) => item.id === id);
      job?.controller.abort();
      return current.map((item) =>
        item.id === id ? { ...item, status: "cancelled" } : item,
      );
    });
  }, []);

  const retry = useCallback(
    (id: string) => {
      const job = jobs.find((item) => item.id === id);
      if (!job) return;
      void start([job.file]);
    },
    [jobs, start],
  );

  const clearDone = useCallback(() => {
    setJobs((current) =>
      current.filter(
        (job) => job.status === "uploading" || job.status === "queued",
      ),
    );
  }, []);

  const dismiss = useCallback(() => {
    setJobs((current) =>
      current.filter(
        (job) =>
          (job.status === "uploading" && job.progress < 100) ||
          job.status === "queued",
      ),
    );
  }, []);

  useEffect(() => {
    if (jobs.length === 0) return;

    const hasActive = jobs.some(
      (job) => job.status === "uploading" || job.status === "queued",
    );
    if (hasActive) return;

    const hasError = jobs.some((job) => job.status === "error");
    if (hasError) return;

    const timer = window.setTimeout(() => {
      setJobs([]);
    }, 1200);

    return () => window.clearTimeout(timer);
  }, [jobs]);

  const active = useMemo(
    () =>
      jobs.filter(
        (job) => job.status === "uploading" || job.status === "queued",
      ),
    [jobs],
  );

  return { jobs, active, start, cancel, retry, clearDone, dismiss };
}
