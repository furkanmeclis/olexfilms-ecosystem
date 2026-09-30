"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";

import { storageKeys } from "@/features/storage/hooks/query-keys";
import { storageService } from "@/features/storage/services/storage.service";
import type {
  CreateStorageLinkInput,
  CreateStorageShareInput,
} from "@/features/storage/types";
import { useAppMutation } from "@/lib/query/mutation";
import { appToast } from "@/providers/toast-provider";
import { useLocale } from "@/providers/locale-provider";

export function useStorageShares(key: string | null) {
  return useQuery({
    queryKey: storageKeys.shares(key ?? ""),
    queryFn: () => storageService.listShares(key!),
    enabled: Boolean(key),
  });
}

export function useStorageLinks(key: string | null) {
  return useQuery({
    queryKey: storageKeys.links(key ?? ""),
    queryFn: () => storageService.listLinks(key!),
    enabled: Boolean(key),
  });
}

export function useCreatePublicLink() {
  const queryClient = useQueryClient();
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: (input: CreateStorageLinkInput) =>
      storageService.createPublicLink(input),
    onSuccess: (_data, input) => {
      void queryClient.invalidateQueries({
        queryKey: storageKeys.links(input.key),
      });
      void queryClient.invalidateQueries({ queryKey: storageKeys.lists() });
      appToast.success(t("storage.toast.public_link"));
    },
  });
}

export function useCreateSignedUrl() {
  const queryClient = useQueryClient();
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: (input: CreateStorageLinkInput) =>
      storageService.createSignedUrl(input),
    onSuccess: (_data, input) => {
      void queryClient.invalidateQueries({
        queryKey: storageKeys.links(input.key),
      });
      appToast.success(t("storage.toast.signed_link"));
    },
  });
}

export function useRevokeLink() {
  const queryClient = useQueryClient();
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: (input: { uuid: string; key: string }) =>
      storageService.revokeLink(input.uuid),
    onSuccess: (_data, input) => {
      void queryClient.invalidateQueries({
        queryKey: storageKeys.links(input.key),
      });
      void queryClient.invalidateQueries({ queryKey: storageKeys.lists() });
      appToast.success(t("storage.toast.link_revoked"));
    },
  });
}

export function useShareFile() {
  const queryClient = useQueryClient();
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: (input: CreateStorageShareInput) =>
      storageService.shareFile(input),
    onSuccess: (_data, input) => {
      void queryClient.invalidateQueries({
        queryKey: storageKeys.shares(input.key),
      });
      appToast.success(t("storage.toast.shared"));
    },
  });
}

export function useUnshareFile() {
  const queryClient = useQueryClient();
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: (input: { uuid: string; key: string }) =>
      storageService.unshareFile(input.uuid),
    onSuccess: (_data, input) => {
      void queryClient.invalidateQueries({
        queryKey: storageKeys.shares(input.key),
      });
      appToast.success(t("storage.toast.unshared"));
    },
  });
}
