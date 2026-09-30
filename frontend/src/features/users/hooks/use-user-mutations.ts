"use client";

import { useQueryClient } from "@tanstack/react-query";

import { usersKeys } from "@/features/users/hooks/query-keys";
import {
  usersService,
  type CreatePlatformUserRequest,
  type PatchPlatformUserRequest,
  type PlatformUserDetail,
  type PublicUser,
  type SetPlatformUserPasswordRequest,
  type UserListResult,
} from "@/features/users/services/users.service";
import { useAppMutation } from "@/lib/query/mutation";
import { appToast } from "@/providers/toast-provider";
import { useLocale } from "@/providers/locale-provider";
import { useAuth } from "@/providers/auth-provider";
import { useStepUp } from "@/features/step-up-engine";
import { defaultHomeForUser } from "@/lib/auth/types";
import { userFullName } from "@/features/users/lib/user-display";

function patchUserInLists(
  queryClient: ReturnType<typeof useQueryClient>,
  uuid: string,
  patch: Partial<PublicUser>,
) {
  queryClient.setQueriesData<UserListResult>(
    { queryKey: usersKeys.lists() },
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

function mergeUserDetail(
  queryClient: ReturnType<typeof useQueryClient>,
  user: PublicUser,
) {
  queryClient.setQueryData<PlatformUserDetail>(
    usersKeys.detail(user.uuid),
    (prev) =>
      prev ? { ...prev, ...user } : { ...user, roles: [], auth_methods: [] },
  );
}

export function useCreateUser() {
  const queryClient = useQueryClient();
  const { t } = useLocale();

  return useAppMutation({
    mutationFn: (body: CreatePlatformUserRequest) => usersService.create(body),
    onSuccess: (user) => {
      queryClient.setQueryData(usersKeys.detail(user.uuid), {
        ...user,
        roles: [],
        auth_methods: [],
      } satisfies PlatformUserDetail);
      void queryClient.invalidateQueries({ queryKey: usersKeys.lists() });
      appToast.success(t("users.toast.created"));
    },
  });
}

export function useUpdateUser() {
  const queryClient = useQueryClient();
  const { t } = useLocale();

  return useAppMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: PatchPlatformUserRequest;
    }) => usersService.update(uuid, body),
    onMutate: async ({ uuid, body }) => {
      await queryClient.cancelQueries({ queryKey: usersKeys.lists() });
      patchUserInLists(queryClient, uuid, body);
      return { uuid };
    },
    onError: (_error, variables) => {
      void queryClient.invalidateQueries({
        queryKey: usersKeys.detail(variables.uuid),
      });
      void queryClient.invalidateQueries({ queryKey: usersKeys.lists() });
    },
    onSuccess: (user) => {
      mergeUserDetail(queryClient, user);
      patchUserInLists(queryClient, user.uuid, user);
      appToast.success(t("users.toast.updated"));
    },
    onSettled: (_data, _error, variables) => {
      void queryClient.invalidateQueries({
        queryKey: usersKeys.detail(variables.uuid),
      });
      void queryClient.invalidateQueries({ queryKey: usersKeys.lists() });
    },
  });
}

export function useEnableUser() {
  const queryClient = useQueryClient();
  const { t } = useLocale();

  return useAppMutation({
    mutationFn: (uuid: string) =>
      usersService.update(uuid, { status: "active" }),
    onMutate: async (uuid) => {
      await queryClient.cancelQueries({ queryKey: usersKeys.lists() });
      patchUserInLists(queryClient, uuid, { status: "active" });
      const prev = queryClient.getQueryData<PlatformUserDetail>(
        usersKeys.detail(uuid),
      );
      if (prev) {
        queryClient.setQueryData(usersKeys.detail(uuid), {
          ...prev,
          status: "active",
        });
      }
    },
    onError: (_error, uuid) => {
      void queryClient.invalidateQueries({
        queryKey: usersKeys.detail(uuid),
      });
      void queryClient.invalidateQueries({ queryKey: usersKeys.lists() });
    },
    onSuccess: (user) => {
      mergeUserDetail(queryClient, user);
      patchUserInLists(queryClient, user.uuid, user);
      appToast.success(t("users.toast.enabled"));
    },
    onSettled: (_data, _error, uuid) => {
      void queryClient.invalidateQueries({
        queryKey: usersKeys.detail(uuid),
      });
      void queryClient.invalidateQueries({ queryKey: usersKeys.lists() });
    },
  });
}

export function useDisableUser() {
  const queryClient = useQueryClient();
  const { t } = useLocale();

  return useAppMutation({
    mutationFn: (uuid: string) =>
      usersService.update(uuid, { status: "disabled" }),
    onMutate: async (uuid) => {
      await queryClient.cancelQueries({ queryKey: usersKeys.lists() });
      patchUserInLists(queryClient, uuid, { status: "disabled" });
      const prev = queryClient.getQueryData<PlatformUserDetail>(
        usersKeys.detail(uuid),
      );
      if (prev) {
        queryClient.setQueryData(usersKeys.detail(uuid), {
          ...prev,
          status: "disabled",
        });
      }
    },
    onError: (_error, uuid) => {
      void queryClient.invalidateQueries({
        queryKey: usersKeys.detail(uuid),
      });
      void queryClient.invalidateQueries({ queryKey: usersKeys.lists() });
    },
    onSuccess: (user) => {
      mergeUserDetail(queryClient, user);
      patchUserInLists(queryClient, user.uuid, user);
      appToast.success(t("users.toast.disabled"));
    },
    onSettled: (_data, _error, uuid) => {
      void queryClient.invalidateQueries({
        queryKey: usersKeys.detail(uuid),
      });
      void queryClient.invalidateQueries({ queryKey: usersKeys.lists() });
    },
  });
}

export function useSetUserPassword() {
  const { t } = useLocale();

  return useAppMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: SetPlatformUserPasswordRequest;
    }) => usersService.setPassword(uuid, body),
    onSuccess: () => {
      appToast.success(t("users.toast.password_set"));
    },
  });
}

export function useImpersonateUser() {
  const { t } = useLocale();
  const { ensure } = useStepUp();
  const { hydrateProfile } = useAuth();

  return useAppMutation({
    mutationFn: async (user: PublicUser) => {
      await ensure();
      return usersService.impersonate(user.uuid);
    },
    onSuccess: async (_result, user) => {
      const refreshed = await hydrateProfile();
      appToast.success(
        t("users.toast.impersonated", { name: userFullName(user) }),
      );
      if (refreshed) {
        window.location.assign(defaultHomeForUser(refreshed));
      } else {
        window.location.reload();
      }
    },
  });
}
