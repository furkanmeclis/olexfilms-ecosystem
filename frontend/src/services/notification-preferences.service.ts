import type { components } from "@/generated/api";
import { apiClient, unwrap } from "@/lib/api";

export type NotificationPreferences =
  components["schemas"]["NotificationPreferences"];

export const notificationPreferencesService = {
  async get() {
    return unwrap<NotificationPreferences>(
      await apiClient.GET("/v1/notification-preferences"),
    );
  },

  async update(body: NotificationPreferences) {
    return unwrap<NotificationPreferences>(
      await apiClient.PUT("/v1/notification-preferences", { body }),
    );
  },
};
