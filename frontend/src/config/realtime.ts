/**
 * Centrifugo realtime — infrastructure kept; disable/stub if upstream unavailable.
 */
export const realtimeConfig = {
  wsUrl:
    process.env.NEXT_PUBLIC_CENTRIFUGO_URL ??
    process.env.NEXT_PUBLIC_WS_URL ??
    "ws://127.0.0.1:8000/connection/websocket",
  reconnect: {
    minDelayMs: 500,
    maxDelayMs: 10_000,
  },
  /** Roles that may subscribe to system notification channels. */
  platformRoles: ["super_admin"] as const,
  channels: {
    systemNotifications: "system.notifications",
    /** WhatsApp inbox events (platform admins only, TEC-395/398). */
    systemConversations: "system.conversations",
    tenant: (tenantUuid: string) => `tenant:${tenantUuid}`,
    user: (userId: number | string) => `user:${userId}`,
    workspace: (workspaceUuid: string) => `workspace:${workspaceUuid}`,
    conversation: (conversationUuid: string) =>
      `conversation:${conversationUuid}`,
  },
  presenceEnabled: true,
  /** When false, RealtimeProvider stays idle (no connect attempts). */
  enabled: process.env.NEXT_PUBLIC_REALTIME_ENABLED !== "false",
} as const;
