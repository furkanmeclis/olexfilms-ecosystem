export const usersKeys = {
  all: ["users"] as const,
  lists: () => [...usersKeys.all, "list"] as const,
  list: (params: Record<string, unknown>) =>
    [...usersKeys.lists(), params] as const,
  details: () => [...usersKeys.all, "detail"] as const,
  detail: (uuid: string) => [...usersKeys.details(), uuid] as const,
  meta: () => [...usersKeys.all, "meta"] as const,
  navStats: () => [...usersKeys.all, "nav-stats"] as const,
};
