"use client";

import { useQuery } from "@tanstack/react-query";

import { usersKeys } from "@/features/users/hooks/query-keys";
import { usersService } from "@/features/users/services/users.service";

const NAV_STATS_STALE_MS = 60_000;
const NAV_STATS_LIMIT = 1;

export type UsersNavStats = {
  total: number;
  active: number;
  pending: number;
  disabled: number;
};

async function fetchUsersNavStats(): Promise<UsersNavStats> {
  const [all, active, pending, disabled] = await Promise.all([
    usersService.list({ limit: NAV_STATS_LIMIT, offset: 0 }),
    usersService.list({
      limit: NAV_STATS_LIMIT,
      offset: 0,
      status: "active",
    }),
    usersService.list({
      limit: NAV_STATS_LIMIT,
      offset: 0,
      status: "pending",
    }),
    usersService.list({
      limit: NAV_STATS_LIMIT,
      offset: 0,
      status: "disabled",
    }),
  ]);

  return {
    total: all.total,
    active: active.total,
    pending: pending.total,
    disabled: disabled.total,
  };
}

export function useUsersNavStats(enabled = true) {
  return useQuery({
    queryKey: usersKeys.navStats(),
    queryFn: fetchUsersNavStats,
    enabled,
    staleTime: NAV_STATS_STALE_MS,
  });
}
