import type { components } from "@/generated/api";
import { apiClient, unwrap } from "@/lib/api";

export type AuthSettings = components["schemas"]["AuthSettings"];
export type AuthSettingsPatch =
  components["schemas"]["PatchAuthSettingsRequest"];

export const authSettingsService = {
  async getSettings() {
    return unwrap<AuthSettings>(
      await apiClient.GET("/v1/platform/auth/settings"),
    );
  },

  async patchSettings(body: AuthSettingsPatch) {
    return unwrap<AuthSettings>(
      await apiClient.PATCH("/v1/platform/auth/settings", { body }),
    );
  },
};
