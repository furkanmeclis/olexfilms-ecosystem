export const notificationsKeys = {
  all: ["notifications"] as const,
  platformLists: () => [...notificationsKeys.all, "platform-list"] as const,
  platformList: (params: Record<string, unknown>) =>
    [...notificationsKeys.platformLists(), params] as const,
  platformMeta: () => [...notificationsKeys.all, "platform-meta"] as const,
  inbox: () => [...notificationsKeys.all, "inbox"] as const,
  unreadCount: () => [...notificationsKeys.all, "unread-count"] as const,
  details: () => [...notificationsKeys.all, "detail"] as const,
  detail: (uuid: string) => [...notificationsKeys.details(), uuid] as const,
  inboxMeta: () => [...notificationsKeys.all, "inbox-meta"] as const,
};
