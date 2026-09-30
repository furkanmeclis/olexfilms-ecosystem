"use client";

import { useQueryClient } from "@tanstack/react-query";

import { rolesKeys } from "@/features/roles/hooks/query-keys";
import {
  rolesService,
  type RoleDetail,
  type RoleListResult,
} from "@/features/roles/services/roles.service";
import { useAppMutation } from "@/lib/query/mutation";
import { appToast } from "@/providers/toast-provider";
import { useLocale } from "@/providers/locale-provider";

export function useCreateRole() {
  const queryClient = useQueryClient();
  const { t } = useLocale();

  return useAppMutation({
    mutationFn: rolesService.create,
    onSuccess: (role) => {
      queryClient.setQueryData(rolesKeys.detail(role.uuid), role);
      void queryClient.invalidateQueries({ queryKey: rolesKeys.lists() });
      appToast.success(t("roles.toast.created"));
    },
  });
}

export function useUpdateRole() {
  const queryClient = useQueryClient();
  const { t } = useLocale();

  return useAppMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: Parameters<typeof rolesService.update>[1];
    }) => rolesService.update(uuid, body),
    onSuccess: (role) => {
      queryClient.setQueryData<RoleDetail>(rolesKeys.detail(role.uuid), role);
      void queryClient.invalidateQueries({ queryKey: rolesKeys.lists() });
      appToast.success(t("roles.toast.updated"));
    },
  });
}

export function useDeleteRole() {
  const queryClient = useQueryClient();
  const { t } = useLocale();

  return useAppMutation({
    mutationFn: (uuid: string) => rolesService.remove(uuid),
    onSuccess: (_data, uuid) => {
      queryClient.removeQueries({ queryKey: rolesKeys.detail(uuid) });
      queryClient.setQueriesData<RoleListResult>(
        { queryKey: rolesKeys.lists() },
        (current) => {
          if (!current) return current;
          return {
            ...current,
            items: current.items.filter((item) => item.uuid !== uuid),
            total: Math.max(0, current.total - 1),
          };
        },
      );
      void queryClient.invalidateQueries({ queryKey: rolesKeys.lists() });
      appToast.success(t("roles.toast.deleted"));
    },
  });
}
