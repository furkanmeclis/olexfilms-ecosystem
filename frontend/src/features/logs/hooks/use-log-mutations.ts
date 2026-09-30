"use client";

import { useQueryClient } from "@tanstack/react-query";

import { logsKeys } from "@/features/logs/hooks/query-keys";
import {
  logsService,
  type PurgeLogsInput,
  type RuleInput,
} from "@/features/logs/services/logs.service";
import { useAppMutation } from "@/lib/query/mutation";
import { appToast } from "@/providers/toast-provider";
import { useLocale } from "@/providers/locale-provider";

export function useDeleteLog() {
  const queryClient = useQueryClient();
  const { t } = useLocale();

  return useAppMutation({
    mutationFn: (uuid: string) => logsService.remove(uuid),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: logsKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: logsKeys.stats() });
      appToast.success(t("logs.toast.deleted"));
    },
  });
}

export function usePurgeLogs() {
  const queryClient = useQueryClient();
  const { t } = useLocale();

  return useAppMutation({
    mutationFn: (body: PurgeLogsInput) => logsService.purge(body),
    onSuccess: (result) => {
      void queryClient.invalidateQueries({ queryKey: logsKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: logsKeys.stats() });
      if (result.dry_run) {
        appToast.info(t("logs.toast.purge_preview", { count: result.deleted }));
      } else {
        appToast.success(t("logs.toast.purged", { count: result.deleted }));
      }
    },
  });
}

export function useCreatePurgeRule() {
  const queryClient = useQueryClient();
  const { t } = useLocale();

  return useAppMutation({
    mutationFn: (body: RuleInput) => logsService.createRule(body),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: logsKeys.rules() });
      appToast.success(t("logs.toast.rule_created"));
    },
  });
}

export function useUpdatePurgeRule() {
  const queryClient = useQueryClient();
  const { t } = useLocale();

  return useAppMutation({
    mutationFn: ({ uuid, body }: { uuid: string; body: Partial<RuleInput> }) =>
      logsService.updateRule(uuid, body),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: logsKeys.rules() });
      appToast.success(t("logs.toast.rule_updated"));
    },
  });
}

export function useDeletePurgeRule() {
  const queryClient = useQueryClient();
  const { t } = useLocale();

  return useAppMutation({
    mutationFn: (uuid: string) => logsService.deleteRule(uuid),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: logsKeys.rules() });
      appToast.success(t("logs.toast.rule_deleted"));
    },
  });
}

export function useRunPurgeRule() {
  const queryClient = useQueryClient();
  const { t } = useLocale();

  return useAppMutation({
    mutationFn: (uuid: string) => logsService.runRule(uuid),
    onSuccess: (result) => {
      void queryClient.invalidateQueries({ queryKey: logsKeys.rules() });
      void queryClient.invalidateQueries({ queryKey: logsKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: logsKeys.stats() });
      appToast.success(t("logs.toast.rule_ran", { count: result.deleted }));
    },
  });
}
