"use client";

import { useQueryClient } from "@tanstack/react-query";

import { organizationsKeys } from "@/features/organizations/hooks/query-keys";
import {
  organizationsService,
  type CreatePlatformOrganizationRequest,
  type Organization,
  type OrganizationDetail,
  type OrganizationListResult,
  type PatchPlatformOrganizationRequest,
} from "@/features/organizations/services/organizations.service";
import { useAppMutation } from "@/lib/query/mutation";
import { appToast } from "@/providers/toast-provider";
import { useLocale } from "@/providers/locale-provider";

function patchOrganizationInLists(
  queryClient: ReturnType<typeof useQueryClient>,
  uuid: string,
  patch: Partial<Organization>,
) {
  queryClient.setQueriesData<OrganizationListResult>(
    { queryKey: organizationsKeys.lists() },
    (current) => {
      if (!current) return current;
      return {
        ...current,
        items: current.items.map((item) =>
          item.uuid === uuid ? { ...item, ...patch } : item,
        ),
      };
    },
  );
}

function mergeOrganizationDetail(
  queryClient: ReturnType<typeof useQueryClient>,
  organization: Organization,
) {
  queryClient.setQueryData<OrganizationDetail>(
    organizationsKeys.detail(organization.uuid),
    (prev) =>
      prev ? { ...prev, organization } : { organization, members: [] },
  );
}

export function useCreateOrganization() {
  const queryClient = useQueryClient();
  const { t } = useLocale();

  return useAppMutation({
    mutationFn: (body: CreatePlatformOrganizationRequest) =>
      organizationsService.create(body),
    onSuccess: (organization) => {
      queryClient.setQueryData(organizationsKeys.detail(organization.uuid), {
        organization,
        members: [],
      } satisfies OrganizationDetail);
      void queryClient.invalidateQueries({
        queryKey: organizationsKeys.lists(),
      });
      appToast.success(t("organizations.toast.created"));
    },
  });
}

export function useUpdateOrganization() {
  const queryClient = useQueryClient();
  const { t } = useLocale();

  return useAppMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: PatchPlatformOrganizationRequest;
    }) => organizationsService.update(uuid, body),
    onMutate: async ({ uuid, body }) => {
      await queryClient.cancelQueries({ queryKey: organizationsKeys.lists() });
      patchOrganizationInLists(queryClient, uuid, body);
      return { uuid };
    },
    onError: (_error, variables) => {
      void queryClient.invalidateQueries({
        queryKey: organizationsKeys.detail(variables.uuid),
      });
      void queryClient.invalidateQueries({
        queryKey: organizationsKeys.lists(),
      });
    },
    onSuccess: (organization) => {
      mergeOrganizationDetail(queryClient, organization);
      patchOrganizationInLists(queryClient, organization.uuid, organization);
      appToast.success(t("organizations.toast.updated"));
    },
    onSettled: (_data, _error, variables) => {
      void queryClient.invalidateQueries({
        queryKey: organizationsKeys.detail(variables.uuid),
      });
      void queryClient.invalidateQueries({
        queryKey: organizationsKeys.lists(),
      });
    },
  });
}

export function useUploadOrganizationLogo() {
  const queryClient = useQueryClient();
  const { t } = useLocale();

  return useAppMutation({
    mutationFn: ({ uuid, file }: { uuid: string; file: File }) =>
      organizationsService.uploadLogo(uuid, file),
    onSuccess: (organization) => {
      mergeOrganizationDetail(queryClient, organization);
      patchOrganizationInLists(queryClient, organization.uuid, organization);
      appToast.success(t("organizations.toast.logo_updated"));
    },
    onError: () => appToast.error(t("organizations.toast.logo_failed")),
  });
}

export function useDeleteOrganizationLogo() {
  const queryClient = useQueryClient();
  const { t } = useLocale();

  return useAppMutation({
    mutationFn: (uuid: string) => organizationsService.deleteLogo(uuid),
    onSuccess: (organization) => {
      mergeOrganizationDetail(queryClient, organization);
      patchOrganizationInLists(queryClient, organization.uuid, organization);
      appToast.success(t("organizations.toast.logo_removed"));
    },
    onError: () => appToast.error(t("organizations.toast.logo_failed")),
  });
}

export function useAddOrganizationMember() {
  const queryClient = useQueryClient();
  const { t } = useLocale();

  return useAppMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: { user_uuid: string; role?: string };
    }) => organizationsService.addMember(uuid, body),
    onSuccess: (_result, variables) => {
      void queryClient.invalidateQueries({
        queryKey: organizationsKeys.detail(variables.uuid),
      });
      appToast.success(t("organizations.toast.member_added"));
    },
    onError: () => appToast.error(t("organizations.toast.member_failed")),
  });
}
