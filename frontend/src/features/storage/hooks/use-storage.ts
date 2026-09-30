"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";

import { storageKeys } from "@/features/storage/hooks/query-keys";
import { storageService } from "@/features/storage/services/storage.service";
import type { ListStorageParams } from "@/features/storage/types";
import { useAppMutation } from "@/lib/query/mutation";
import { appToast } from "@/providers/toast-provider";
import { useLocale } from "@/providers/locale-provider";

export function useStorageFiles(params: ListStorageParams, enabled = true) {
  return useQuery({
    queryKey: storageKeys.list(params),
    queryFn: () => storageService.listFiles(params),
    enabled,
    placeholderData: (previous) => previous,
  });
}

export function useStorageFile(key: string | null, enabled = true) {
  return useQuery({
    queryKey: storageKeys.detail(key ?? ""),
    queryFn: () => storageService.getFile(key!),
    enabled: Boolean(key) && enabled,
  });
}

export function useStorageUsage(enabled = true) {
  return useQuery({
    queryKey: storageKeys.usage(),
    queryFn: () => storageService.getUsage(),
    enabled,
    staleTime: 30_000,
  });
}

export function useStorageVersions(key: string | null) {
  return useQuery({
    queryKey: storageKeys.versions(key ?? ""),
    queryFn: () => storageService.getVersions(key!),
    enabled: Boolean(key),
  });
}

export function useStorageActivity(key: string | null) {
  return useQuery({
    queryKey: storageKeys.activity(key ?? ""),
    queryFn: () => storageService.getActivity(key!, { limit: 50 }),
    enabled: Boolean(key),
  });
}

function invalidateAll(queryClient: ReturnType<typeof useQueryClient>) {
  void queryClient.invalidateQueries({ queryKey: storageKeys.all });
}

export function useCreateFolder() {
  const queryClient = useQueryClient();
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: (input: { prefix: string; name: string }) =>
      storageService.createFolder(input),
    onSuccess: () => {
      invalidateAll(queryClient);
      appToast.success(t("storage.toast.folder_created"));
    },
  });
}

export function useDeleteFiles() {
  const queryClient = useQueryClient();
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: (keys: string[]) => storageService.deleteFile(keys),
    onSuccess: () => {
      invalidateAll(queryClient);
      appToast.success(t("storage.toast.deleted"));
    },
  });
}

export function useRestoreFiles() {
  const queryClient = useQueryClient();
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: (keys: string[]) => storageService.restoreFile(keys),
    onSuccess: () => {
      invalidateAll(queryClient);
      appToast.success(t("storage.toast.restored"));
    },
  });
}

export function usePurgeFiles() {
  const queryClient = useQueryClient();
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: (keys: string[]) => storageService.purgeFile(keys),
    onSuccess: () => {
      invalidateAll(queryClient);
      appToast.success(t("storage.toast.purged"));
    },
  });
}

export function useRenameFile() {
  const queryClient = useQueryClient();
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: (input: { key: string; name: string }) =>
      storageService.renameFile(input),
    onSuccess: () => {
      invalidateAll(queryClient);
      appToast.success(t("storage.toast.renamed"));
    },
  });
}

export function useMoveFile() {
  const queryClient = useQueryClient();
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: (input: { source_key: string; dest_key: string }) =>
      storageService.moveFile(input),
    onSuccess: () => {
      invalidateAll(queryClient);
      appToast.success(t("storage.toast.moved"));
    },
  });
}

export function useCopyFile() {
  const queryClient = useQueryClient();
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: (input: { source_key: string; dest_key: string }) =>
      storageService.copyFile(input),
    onSuccess: () => {
      invalidateAll(queryClient);
      appToast.success(t("storage.toast.copied"));
    },
  });
}

export function useStarFile() {
  const queryClient = useQueryClient();
  return useAppMutation({
    mutationFn: ({ key, starred }: { key: string; starred: boolean }) =>
      starred ? storageService.starFile(key) : storageService.unstarFile(key),
    onSuccess: () => invalidateAll(queryClient),
  });
}

export function useRestoreVersion() {
  const queryClient = useQueryClient();
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: (input: { key: string; version_id: string }) =>
      storageService.restoreVersion(input),
    onSuccess: () => {
      invalidateAll(queryClient);
      appToast.success(t("storage.toast.version_restored"));
    },
  });
}
