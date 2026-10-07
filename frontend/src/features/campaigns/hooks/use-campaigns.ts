"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  campaignKeys,
  campaignsService,
  type Campaign,
  type CampaignApprovalQuery,
  type CampaignListQuery,
  type CampaignRecipientQuery,
  type CampaignContentInput,
  type LocaleCode,
} from "@/features/campaigns/services/campaigns.service";

export function useCampaignsList(params: CampaignListQuery, enabled = true) {
  return useQuery({
    queryKey: campaignKeys.list(params),
    queryFn: () => campaignsService.list(params),
    enabled,
    placeholderData: (previous) => previous,
  });
}

export function useCampaignApprovals(
  params: CampaignApprovalQuery,
  enabled = true,
) {
  return useQuery({
    queryKey: campaignKeys.approvals(params),
    queryFn: () => campaignsService.approvals(params),
    enabled,
    placeholderData: (previous) => previous,
  });
}

export function useCampaign(uuid: string, enabled = true) {
  return useQuery({
    queryKey: campaignKeys.detail(uuid),
    queryFn: () => campaignsService.get(uuid),
    enabled: enabled && Boolean(uuid),
  });
}

export function useCampaignPreview(uuid: string, enabled = true) {
  return useQuery({
    queryKey: campaignKeys.preview(uuid),
    queryFn: () => campaignsService.preview(uuid),
    enabled: enabled && Boolean(uuid),
  });
}

export function useCampaignRecipients(
  uuid: string,
  params: CampaignRecipientQuery,
  enabled = true,
) {
  return useQuery({
    queryKey: campaignKeys.recipients(uuid, params),
    queryFn: () => campaignsService.recipients(uuid, params),
    enabled: enabled && Boolean(uuid),
    placeholderData: (previous) => previous,
  });
}

export function useCampaignMutations() {
  const qc = useQueryClient();
  const invalidate = async (campaign?: Campaign) => {
    await qc.invalidateQueries({ queryKey: campaignKeys.lists });
    await qc.invalidateQueries({ queryKey: ["campaigns", "approvals"] });
    if (campaign?.uuid) {
      await qc.invalidateQueries({
        queryKey: campaignKeys.detail(campaign.uuid),
      });
      await qc.invalidateQueries({
        queryKey: campaignKeys.preview(campaign.uuid),
      });
    }
  };

  return {
    create: useMutation({
      mutationFn: campaignsService.create,
      onSuccess: invalidate,
    }),
    update: useMutation({
      mutationFn: ({
        uuid,
        body,
      }: {
        uuid: string;
        body: Parameters<typeof campaignsService.update>[1];
      }) => campaignsService.update(uuid, body),
      onSuccess: invalidate,
    }),
    putContent: useMutation({
      mutationFn: ({
        uuid,
        locale,
        body,
      }: {
        uuid: string;
        locale: LocaleCode;
        body: CampaignContentInput;
      }) => campaignsService.putContent(uuid, locale, body),
      onSuccess: async (_content, vars) => {
        await qc.invalidateQueries({
          queryKey: campaignKeys.detail(vars.uuid),
        });
        await qc.invalidateQueries({
          queryKey: campaignKeys.preview(vars.uuid),
        });
      },
    }),
    submit: useMutation({
      mutationFn: campaignsService.submit,
      onSuccess: invalidate,
    }),
    schedule: useMutation({
      mutationFn: ({
        uuid,
        scheduledAt,
      }: {
        uuid: string;
        scheduledAt: string;
      }) => campaignsService.schedule(uuid, scheduledAt),
      onSuccess: invalidate,
    }),
    approve: useMutation({
      mutationFn: ({ uuid, reason }: { uuid: string; reason?: string }) =>
        campaignsService.approve(uuid, reason),
      onSuccess: invalidate,
    }),
    reject: useMutation({
      mutationFn: ({ uuid, reason }: { uuid: string; reason: string }) =>
        campaignsService.reject(uuid, reason),
      onSuccess: invalidate,
    }),
    requestChanges: useMutation({
      mutationFn: ({ uuid, reason }: { uuid: string; reason: string }) =>
        campaignsService.requestChanges(uuid, reason),
      onSuccess: invalidate,
    }),
    cancel: useMutation({
      mutationFn: campaignsService.cancel,
      onSuccess: invalidate,
    }),
    remove: useMutation({
      mutationFn: campaignsService.remove,
      onSuccess: async () => {
        await qc.invalidateQueries({ queryKey: campaignKeys.lists });
      },
    }),
  };
}
