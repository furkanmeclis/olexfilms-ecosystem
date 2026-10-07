"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";

import { useAppMutation } from "@/lib/query/mutation";
import {
  mcpService,
  type ClientListParams,
  type GrantListParams,
  type PendingActionListParams,
  type SessionRealm,
} from "@/features/mcp/services/mcp.service";

export const mcpKeys = {
  all: ["mcp"] as const,
  grants: (realm: SessionRealm, params: Record<string, unknown>) =>
    [...mcpKeys.all, "grants", realm, params] as const,
  grantsAll: (realm: SessionRealm) =>
    [...mcpKeys.all, "grants", realm] as const,
  clients: (params: Record<string, unknown>) =>
    [...mcpKeys.all, "clients", params] as const,
  clientsAll: () => [...mcpKeys.all, "clients"] as const,
  pending: (params: Record<string, unknown>) =>
    [...mcpKeys.all, "pending", params] as const,
  pendingAll: () => [...mcpKeys.all, "pending"] as const,
  pendingCount: () => [...mcpKeys.all, "pending", "count"] as const,
};

export function useGrants(
  realm: SessionRealm,
  params: GrantListParams,
  enabled = true,
) {
  return useQuery({
    queryKey: mcpKeys.grants(realm, params),
    queryFn: () => mcpService.grants(realm, params),
    enabled,
    placeholderData: (previous) => previous,
  });
}

export function useRevokeGrant(realm: SessionRealm) {
  const client = useQueryClient();
  return useAppMutation({
    mutationFn: (uuid: string) => mcpService.revokeGrant(realm, uuid),
    onSettled: () =>
      client.invalidateQueries({ queryKey: mcpKeys.grantsAll(realm) }),
  });
}

export function useMcpClients(params: ClientListParams, enabled = true) {
  return useQuery({
    queryKey: mcpKeys.clients(params),
    queryFn: () => mcpService.clients(params),
    enabled,
    placeholderData: (previous) => previous,
  });
}

export function useRevokeMcpClient() {
  const client = useQueryClient();
  return useAppMutation({
    mutationFn: (uuid: string) => mcpService.revokeClient(uuid),
    onSettled: () =>
      client.invalidateQueries({ queryKey: mcpKeys.clientsAll() }),
  });
}

export function usePendingActions(
  params: PendingActionListParams,
  enabled = true,
) {
  return useQuery({
    queryKey: mcpKeys.pending(params),
    queryFn: () => mcpService.pendingActions(params),
    enabled,
    placeholderData: (previous) => previous,
    refetchInterval: 30_000,
  });
}

/** Badge count: `total` of a one-row page. */
export function usePendingActionCount(enabled = true) {
  return useQuery({
    queryKey: mcpKeys.pendingCount(),
    queryFn: async () =>
      (await mcpService.pendingActions({ limit: 1, offset: 0 })).total,
    enabled,
    refetchInterval: 30_000,
  });
}

export function useDecidePendingAction() {
  const client = useQueryClient();
  return useAppMutation({
    mutationFn: ({ uuid, confirm }: { uuid: string; confirm: boolean }) =>
      confirm ? mcpService.confirmAction(uuid) : mcpService.cancelAction(uuid),
    onSettled: () =>
      client.invalidateQueries({ queryKey: mcpKeys.pendingAll() }),
  });
}
