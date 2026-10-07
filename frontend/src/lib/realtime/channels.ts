import { realtimeConfig } from "@/config/realtime";

/**
 * Channel name builders — never hardcode channel strings in features.
 */
export const RealtimeChannels = {
  systemNotifications: () => realtimeConfig.channels.systemNotifications,
  systemConversations: () => realtimeConfig.channels.systemConversations,
  tenant: (tenantUuid: string) => realtimeConfig.channels.tenant(tenantUuid),
  user: (userId: number | string) => realtimeConfig.channels.user(userId),
  workspace: (workspaceUuid: string) =>
    realtimeConfig.channels.workspace(workspaceUuid),
  conversation: (conversationUuid: string) =>
    realtimeConfig.channels.conversation(conversationUuid),
  /** @deprecated Prefer `tenant` — kept for transitional imports */
  dealer: (dealerId: number | string) =>
    realtimeConfig.channels.tenant(String(dealerId)),
} as const;

export type RealtimeChannelBuilder = typeof RealtimeChannels;

export function isAuthorizedChannelShape(channel: string): boolean {
  if (channel === realtimeConfig.channels.systemNotifications) return true;
  if (channel === realtimeConfig.channels.systemConversations) return true;
  if (/^tenant:[0-9a-fA-F-]+$/.test(channel)) return true;
  if (/^user:[0-9a-fA-F-]+$/.test(channel)) return true;
  if (/^user:\d+$/.test(channel)) return true;
  if (/^workspace:[0-9a-fA-F-]+$/.test(channel)) return true;
  if (/^conversation:[0-9a-fA-F-]+$/.test(channel)) return true;
  return false;
}
