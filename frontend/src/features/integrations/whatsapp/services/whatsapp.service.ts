import type { components } from "@/generated/api";
import { apiClient, unwrap } from "@/lib/api";

export type WhatsAppOverview = components["schemas"]["WhatsAppOverview"];
export type WhatsAppQR = components["schemas"]["WhatsAppQR"];
export type WhatsAppSettingsRequest =
  components["schemas"]["WhatsAppSettingsRequest"];
export type WhatsAppStatus = WhatsAppOverview["status"];

export const whatsappService = {
  async overview() {
    return unwrap<WhatsAppOverview>(
      await apiClient.GET("/v1/platform/whatsapp"),
    );
  },

  async connect() {
    return unwrap(await apiClient.POST("/v1/platform/whatsapp/connect"));
  },

  async qr() {
    return unwrap<WhatsAppQR>(await apiClient.GET("/v1/platform/whatsapp/qr"));
  },

  async pairPhone(phone: string) {
    return unwrap<components["schemas"]["WhatsAppPairPhone"]>(
      await apiClient.POST("/v1/platform/whatsapp/pair-phone", {
        body: { phone },
      }),
    );
  },

  async logout() {
    return unwrap(await apiClient.POST("/v1/platform/whatsapp/logout"));
  },

  async testMessage(phone: string, body: string) {
    return unwrap<components["schemas"]["WhatsAppTestMessage"]>(
      await apiClient.POST("/v1/platform/whatsapp/test-message", {
        body: { phone, body },
      }),
    );
  },

  async saveSettings(body: WhatsAppSettingsRequest) {
    return unwrap<WhatsAppOverview>(
      await apiClient.PUT("/v1/platform/whatsapp/settings", { body }),
    );
  },
};
