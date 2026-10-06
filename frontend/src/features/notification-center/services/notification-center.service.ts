import type { components } from "@/generated/api";
import { apiClient, unwrap } from "@/lib/api";

export type NotificationEvent = components["schemas"]["NotificationEvent"];
export type NotificationTemplate =
  components["schemas"]["NotificationTemplate"];
export type NotificationTemplateInput =
  components["schemas"]["NotificationTemplateInput"];
export type NotificationRendered =
  components["schemas"]["NotificationRendered"];
export type NotificationChannelSetting =
  components["schemas"]["NotificationChannelSetting"];
export type NotificationDelivery =
  components["schemas"]["NotificationDelivery"];
export type NotificationDeliveryPage =
  components["schemas"]["NotificationDeliveryPage"];
export type NotificationChannel = components["schemas"]["NotificationChannel"];
export type NotificationTemplateRole =
  components["schemas"]["NotificationTemplateRole"];
export type NotificationDeliveryStatus =
  components["schemas"]["NotificationDeliveryStatus"];

export type DeliveryFilter = {
  limit?: number;
  offset?: number;
  sort?: string;
  q?: string;
  /** CSV of NotificationDeliveryStatus */
  status?: string;
  /** CSV of NotificationChannel */
  channel?: string;
  created_from?: string;
  created_to?: string;
  event_code?: string;
  event_id?: string;
  user_uuid?: string;
};

export const notificationCenterService = {
  async events() {
    const data = await unwrap<{ items: NotificationEvent[] }>(
      await apiClient.GET("/v1/notification-events"),
    );
    return data.items;
  },

  async templates(code?: string) {
    const data = await unwrap<{ items: NotificationTemplate[] }>(
      await apiClient.GET("/v1/platform/notification-templates", {
        params: { query: code ? { code } : {} },
      }),
    );
    return data.items;
  },

  async saveTemplate(body: NotificationTemplateInput) {
    return unwrap<NotificationTemplate>(
      await apiClient.PUT("/v1/platform/notification-templates", { body }),
    );
  },

  async preview(body: NotificationTemplateInput) {
    return unwrap<NotificationRendered>(
      await apiClient.POST("/v1/platform/notification-templates/preview", {
        body,
      }),
    );
  },

  async channels() {
    const data = await unwrap<{ items: NotificationChannelSetting[] }>(
      await apiClient.GET("/v1/platform/notification-channels"),
    );
    return data.items;
  },

  async setChannel(channel: NotificationChannel, enabled: boolean) {
    return unwrap<NotificationChannelSetting>(
      await apiClient.PUT("/v1/platform/notification-channels/{channel}", {
        params: { path: { channel } },
        body: { enabled },
      }),
    );
  },

  async deliveries(filter: DeliveryFilter) {
    return unwrap<NotificationDeliveryPage>(
      await apiClient.GET("/v1/platform/notification-deliveries", {
        params: { query: filter },
      }),
    );
  },
};
