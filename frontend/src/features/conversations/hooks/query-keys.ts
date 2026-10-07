export const conversationsKeys = {
  all: ["conversations"] as const,
  lists: () => [...conversationsKeys.all, "list"] as const,
  list: (params: Record<string, unknown>) =>
    [...conversationsKeys.lists(), params] as const,
  meta: () => [...conversationsKeys.all, "meta"] as const,
  detail: (uuid: string) => [...conversationsKeys.all, "detail", uuid] as const,
  messages: (uuid: string) =>
    [...conversationsKeys.all, "messages", uuid] as const,
  unread: () => [...conversationsKeys.all, "unread"] as const,
  admins: () => [...conversationsKeys.all, "admins"] as const,
};
