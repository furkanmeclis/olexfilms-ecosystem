import type { ServerListParams } from "@/components/entity";

export const rolesKeys = {
  all: ["platform", "roles"] as const,
  lists: () => [...rolesKeys.all, "list"] as const,
  list: (params: ServerListParams) => [...rolesKeys.lists(), params] as const,
  meta: () => [...rolesKeys.all, "meta"] as const,
  details: () => [...rolesKeys.all, "detail"] as const,
  detail: (uuid: string) => [...rolesKeys.details(), uuid] as const,
  permissions: () => ["platform", "permissions"] as const,
};
