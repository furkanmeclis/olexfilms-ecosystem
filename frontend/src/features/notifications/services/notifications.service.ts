import type { components } from "@/generated/api";
import { apiClient, unwrap } from "@/lib/api";
import type { ServerListParams } from "@/components/entity";

export type Notification = components["schemas"]["Notification"];
export type NotificationPage = components["schemas"]["NotificationPage"];
export type UnreadCount = components["schemas"]["UnreadCount"];
export type ResourceMeta = components["schemas"]["ResourceMeta"];

export type NotificationListResult = NotificationPage;

export type ListPlatformNotificationsParams = ServerListParams & {
  /** CSV */
  status?: string;
  /** CSV */
  channel?: string;
  /** CSV */
  priority?: string;
  created_from?: string;
  created_to?: string;
  scope?: "me" | "all";
  user_uuid?: string;
};

export type ListInboxNotificationsParams = ServerListParams & {
  status?: string;
  channel?: string;
  unread?: boolean;
};

export const notificationsService = {
  async listPlatform(params: ListPlatformNotificationsParams) {
    return unwrap<NotificationListResult>(
      await apiClient.GET("/v1/platform/notifications", {
        params: {
          query: {
            limit: params.limit,
            offset: params.offset,
            sort: params.sort,
            q: params.q,
            ...(params.status ? { status: params.status } : {}),
            ...(params.channel ? { channel: params.channel } : {}),
            ...(params.priority ? { priority: params.priority } : {}),
            ...(params.created_from
              ? { created_from: params.created_from }
              : {}),
            ...(params.created_to ? { created_to: params.created_to } : {}),
            ...(params.scope ? { scope: params.scope } : {}),
            ...(params.user_uuid ? { user_uuid: params.user_uuid } : {}),
          },
        },
      }),
    );
  },

  async platformMeta() {
    return unwrap<ResourceMeta>(
      await apiClient.GET("/v1/platform/notifications/meta"),
    );
  },

  async listInbox(params: ListInboxNotificationsParams) {
    return unwrap<NotificationListResult>(
      await apiClient.GET("/v1/notifications", {
        params: {
          query: {
            limit: params.limit,
            offset: params.offset,
            sort: params.sort,
            q: params.q,
            status: params.status,
            channel: params.channel,
            unread: params.unread,
          },
        },
      }),
    );
  },

  async inboxMeta() {
    return unwrap<ResourceMeta>(await apiClient.GET("/v1/notifications/meta"));
  },

  async unreadCount() {
    return unwrap<UnreadCount>(
      await apiClient.GET("/v1/notifications/unread-count"),
    );
  },

  async get(uuid: string) {
    return unwrap<Notification>(
      await apiClient.GET("/v1/notifications/{uuid}", {
        params: { path: { uuid } },
      }),
    );
  },

  async markRead(uuid: string) {
    return unwrap<Notification>(
      await apiClient.POST("/v1/notifications/{uuid}/read", {
        params: { path: { uuid } },
      }),
    );
  },

  async markAllRead() {
    return unwrap<{ status: string }>(
      await apiClient.POST("/v1/notifications/read-all"),
    );
  },

  async redeemSignedAction(source: {
    uuid?: string;
    signed_action_url?: string | null;
  }) {
    try {
      const first = await redeemParsed(source.signed_action_url);
      if (first) return first;
    } catch {
      // Expired or stale signature — refresh from the API.
    }
    if (!source.uuid) return null;
    const full = await notificationsService.get(source.uuid);
    return redeemParsed(full.signed_action_url);
  },
};

async function redeemParsed(signed: string | null | undefined) {
  const parsed = parseSignedActionUrl(signed);
  if (!parsed) return null;
  return unwrap<{
    destination: string;
    notification: Notification;
  }>(
    await apiClient.GET("/v1/notifications/{uuid}/action", {
      params: {
        path: { uuid: parsed.uuid },
        query: { exp: parsed.exp, sig: parsed.sig },
      },
    }),
  );
}

function parseSignedActionUrl(href?: string | null) {
  if (!href) return null;
  try {
    const url = new URL(href, "https://action.invalid");
    const parts = url.pathname.split("/").filter(Boolean);
    if (
      parts.length < 4 ||
      parts[0] !== "v1" ||
      parts[1] !== "notifications" ||
      parts[3] !== "action"
    ) {
      return null;
    }
    const exp = Number(url.searchParams.get("exp"));
    const sig = url.searchParams.get("sig") ?? "";
    if (!Number.isFinite(exp) || !sig) return null;
    return { uuid: parts[2], exp, sig };
  } catch {
    return null;
  }
}
