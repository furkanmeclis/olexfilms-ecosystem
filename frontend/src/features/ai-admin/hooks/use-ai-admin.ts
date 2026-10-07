"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { aiAdminKeys } from "@/features/ai-admin/hooks/query-keys";
import {
  aiAdminService,
  type AIOrgQuotaListParams,
  type AIOrgQuotaUpdate,
  type AISettingsUpdate,
  type AIUsageListParams,
  type AIUsageScope,
} from "@/features/ai-admin/services/ai-admin.service";

export function useAISettings(enabled = true) {
  return useQuery({
    queryKey: aiAdminKeys.settings(),
    queryFn: () => aiAdminService.settings(),
    enabled,
  });
}

export function useUpdateAISettings() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: AISettingsUpdate) => aiAdminService.updateSettings(body),
    onSuccess: (settings) => {
      queryClient.setQueryData(aiAdminKeys.settings(), settings);
      // The platform default quota feeds every row without an override.
      void queryClient.invalidateQueries({
        queryKey: aiAdminKeys.orgQuotas(),
      });
    },
  });
}

export function useAIOrgQuotas(params: AIOrgQuotaListParams, enabled = true) {
  return useQuery({
    queryKey: aiAdminKeys.orgQuotaList(params),
    queryFn: () => aiAdminService.orgQuotas(params),
    enabled,
    placeholderData: (previous) => previous,
  });
}

export function useUpdateAIOrgQuota() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ uuid, body }: { uuid: string; body: AIOrgQuotaUpdate }) =>
      aiAdminService.updateOrgQuota(uuid, body),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: aiAdminKeys.orgQuotas() }),
  });
}

export function useAIUsage(
  scope: AIUsageScope,
  params: AIUsageListParams,
  enabled = true,
) {
  return useQuery({
    queryKey: aiAdminKeys.usageList(scope, params),
    queryFn: () => aiAdminService.usage(scope, params),
    enabled,
    placeholderData: (previous) => previous,
  });
}

export function useAIUsageSummary(period: string, enabled = true) {
  return useQuery({
    queryKey: aiAdminKeys.summary(period),
    queryFn: () => aiAdminService.summary(period),
    enabled,
  });
}
