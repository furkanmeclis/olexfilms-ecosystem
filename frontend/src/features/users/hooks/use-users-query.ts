"use client";

import { useQuery } from "@tanstack/react-query";

import { usersKeys } from "@/features/users/hooks/query-keys";
import {
  usersService,
  type ListUsersParams,
} from "@/features/users/services/users.service";

export function useUsersList(params: ListUsersParams, enabled = true) {
  return useQuery({
    queryKey: usersKeys.list(params),
    queryFn: () => usersService.list(params),
    enabled,
    placeholderData: (previous) => previous,
  });
}

export function useUser(uuid: string | null, enabled = true) {
  return useQuery({
    queryKey: usersKeys.detail(uuid ?? ""),
    queryFn: () => usersService.get(uuid!),
    enabled: Boolean(uuid) && enabled,
  });
}

export function useUsersMeta(enabled = true) {
  return useQuery({
    queryKey: usersKeys.meta(),
    queryFn: () => usersService.meta(),
    enabled,
    staleTime: 5 * 60_000,
  });
}
