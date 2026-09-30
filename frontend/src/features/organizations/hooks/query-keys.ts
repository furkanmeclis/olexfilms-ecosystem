export const organizationsKeys = {
  all: ["organizations"] as const,
  lists: () => [...organizationsKeys.all, "list"] as const,
  list: (params: Record<string, unknown>) =>
    [...organizationsKeys.lists(), params] as const,
  details: () => [...organizationsKeys.all, "detail"] as const,
  detail: (uuid: string) => [...organizationsKeys.details(), uuid] as const,
  meta: () => [...organizationsKeys.all, "meta"] as const,
};
